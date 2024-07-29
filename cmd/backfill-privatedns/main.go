package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/route53resolver"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/jmoiron/sqlx"
)

func getSessionFromCreds(creds *credentials.Credentials, region string) *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Region:      &region,
		Credentials: creds,
	}))
}

func arnPrefix(region string) string {
	if strings.HasPrefix(region, "us-gov") {
		return "arn:aws-us-gov"
	}
	return "arn:aws"
}

func resourceShareARN(region, accountID, resourceShareID string) string {
	resourceShareID = strings.TrimPrefix(resourceShareID, "rs-")
	return fmt.Sprintf("%s:ram:%s:%s:resource-share/%s", arnPrefix(region), region, accountID, resourceShareID)
}

func getCredsByAccountID(creds *awscreds.CloudTamerAWSCreds, accountID string) (*credentials.Credentials, error) {
	var awsc *credentials.Credentials
	log.Printf("  Getting credentials for account %s", accountID)
	awsc, err := creds.GetCredentialsForAccount(accountID)
	if err != nil {
		log.Printf("ERROR getting credentials for account %s: %s", accountID, err)
	}
	return awsc, err
}

func main() {
	params := &struct {
		CloudTamerUserName       string
		CloudTamerPassword       string
		CloudTamerIDMSID         int
		PostgresConnectionString string
	}{}

	sess := session.Must(session.NewSession())
	ssmsvc := ssm.New(sess)
	out, err := ssmsvc.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String("backfill-privatedns-params"),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		log.Fatalf("Error getting params: %s", err)
	}
	val := aws.StringValue(out.Parameter.Value)
	err = json.Unmarshal([]byte(val), params)
	if err != nil {
		log.Fatalf("Error unmarshaling params: %s", err)
	}

	if os.Getenv("POSTGRES_CONNECTION_STRING") != "" { // local override for development
		params.PostgresConnectionString = os.Getenv("POSTGRES_CONNECTION_STRING")
	}

	db := sqlx.MustConnect("postgres", params.PostgresConnectionString)
	log.Println("db connected")
	modelsManager := &database.SQLModelsManager{
		DB: db,
	}

	cloudTamerBaseURL := os.Getenv("CLOUDTAMER_BASE_URL")
	cloudTamerAdminGroupIDStr := os.Getenv("CLOUDTAMER_ADMIN_GROUP_ID")
	if cloudTamerBaseURL == "" || cloudTamerAdminGroupIDStr == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "CLOUDTAMER_BASE_URL and CLOUDTAMER_ADMIN_GROUP_ID env variables are required")
		os.Exit(2)
	}
	cloudTamerAdminGroupID, err := strconv.Atoi(cloudTamerAdminGroupIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", "Invalid CLOUDTAMER_ADMIN_GROUP_ID")
		os.Exit(2)
	}
	cloudTamerServiceAccountUsername := params.CloudTamerUserName
	cloudTamerServiceAccountPassword := params.CloudTamerPassword
	cloudTamerServiceAccountIDMSID := params.CloudTamerIDMSID
	tokenProvider := &cloudtamer.TokenProvider{
		Config: cloudtamer.CloudTamerConfig{
			BaseURL:      cloudTamerBaseURL,
			IDMSID:       cloudTamerServiceAccountIDMSID,
			AdminGroupID: cloudTamerAdminGroupID,
		},
		Username: cloudTamerServiceAccountUsername,
		Password: cloudTamerServiceAccountPassword,
	}
	token, err := tokenProvider.GetToken()
	if err != nil {
		log.Fatalf("Error getting CloudTamer token: %s", err)
	}
	creds := &awscreds.CloudTamerAWSCreds{
		Token:   token,
		BaseURL: cloudTamerBaseURL,
	}
	vpcs, err := modelsManager.ListAutomatedVPCs()
	if err != nil {
		log.Fatalf("Error listing automated VPCs: %s", err)
	}
	credsByAccountID := map[string]*credentials.Credentials{}

	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hostname: %s\n", err)
		os.Exit(1)
	}
	taskDB := &database.TaskDatabase{DB: db, WorkerID: hostname}
	var lockSet database.LockSet
	release := func() {
		if lockSet != nil {
			lockSet.ReleaseAll()
		}
	}
	defer release()
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		for s := range c {
			sig, ok := s.(syscall.Signal)
			if !ok {
				log.Printf("Non-UNIX signal %s", s)
				continue
			}
			if sig == syscall.SIGINT {
				release()
				os.Exit(1)
			} else if sig == syscall.SIGTERM {
				release()
				os.Exit(1)
			}
		}
	}()

	// Get a lock on all VPCs
	targets := []database.Target{}
	for _, vpc := range vpcs {
		targets = append(targets, database.TargetVPC(vpc.ID))
	}
	for {
		lockSet, err = taskDB.AcquireLocks(targets...)
		if err == nil {
			break
		}
		if e, ok := err.(*database.TargetAlreadyLockedError); ok {
			log.Printf("Could not get lock for %q", e.Target)
			time.Sleep(5 * time.Second)
		} else {
			log.Printf("Could not get locks: %s", err)
			os.Exit(1)
		}
	}

	managedRuleSets, err := modelsManager.GetManagedResolverRuleSets()
	if err != nil {
		log.Printf("Error getting list of managed rulesets: %s\n", err)
		os.Exit(1)
	}
	shareRAMs := make(map[uint64]*ram.RAM)
	managedRuleSetByID := make(map[uint64]*database.ManagedResolverRuleSet)
	for _, mrs := range managedRuleSets {
		managedRuleSetByID[mrs.ID] = mrs
		awscreds := credsByAccountID[mrs.AccountID]
		if awscreds == nil {
			awscreds, err := getCredsByAccountID(creds, mrs.AccountID)
			if err != nil {
				os.Exit(1)
			}
			credsByAccountID[mrs.AccountID] = awscreds
		}
		if shareRAMs[mrs.ID] == nil {
			shareRAMs[mrs.ID] = ram.New(getSessionFromCreds(credsByAccountID[mrs.AccountID], string(mrs.Region)))
		}
	}

	for _, vpc := range vpcs {
		vpc, vpcWriter, err := modelsManager.GetOperableVPC(lockSet, vpc.Region, vpc.ID)
		if err != nil {
			log.Println(err)
			log.Printf("ERROR getting VPC %s: %s", vpc.ID, err)
			continue
		}
		log.Printf("VPC %s   Account %s   Region %s", vpc.ID, vpc.AccountID, vpc.Region)
		if vpc.State.ResolverRuleAssociations != nil {
			log.Printf("  VPC %s already has associations recorded", vpc.ID)
			continue
		}
		awscreds := credsByAccountID[vpc.AccountID]
		if awscreds == nil {
			awscreds, err = getCredsByAccountID(creds, vpc.AccountID)
			if err != nil {
				log.Printf("Unable to get credentials for %s: %s\n", vpc.AccountID, err)
				continue
			}
			credsByAccountID[vpc.AccountID] = awscreds
		}
		for id, rs := range managedRuleSetByID {
			if rs.ResourceShareID == "" {
				continue
			}
			if rs.Region != vpc.Region {
				continue
			}
			shareRAM := shareRAMs[id]
			// First check if the ruleset is shared with the account in RAM
			shareARN := resourceShareARN(string(rs.Region), rs.AccountID, rs.ResourceShareID)
			out, err := shareRAM.ListPrincipals(&ram.ListPrincipalsInput{
				ResourceShareArns: []*string{aws.String(shareARN)},
				Principals:        aws.StringSlice([]string{vpc.AccountID}),
				ResourceOwner:     aws.String("SELF"),
			})
			if err != nil {
				log.Printf("Error listing principals for %s: %s\n", shareARN, err)
				continue
			}
			if len(out.Principals) == 0 {
				continue
			}
			log.Printf("AccountID %s is a listed principal on %s, check associations\n", vpc.AccountID, shareARN)
			R53R := route53resolver.New(getSessionFromCreds(awscreds, string(vpc.Region)))
			// We are already shared, so let's check if the rules are associated
			rulesFound := false
			vpc.Config.ManagedResolverRuleSetIDs = make([]uint64, 0)
			for _, rule := range rs.Rules {
				log.Printf("Check for rule %s on VPC %s\n", rule.AWSID, vpc.ID)
				out, err := R53R.ListResolverRuleAssociations(
					&route53resolver.ListResolverRuleAssociationsInput{
						Filters: []*route53resolver.Filter{
							{
								Name:   aws.String("VPCId"),
								Values: aws.StringSlice([]string{vpc.ID}),
							},
							{
								Name:   aws.String("ResolverRuleId"),
								Values: aws.StringSlice([]string{rule.AWSID}),
							},
						},
					},
				)
				if err != nil {
					log.Printf("Error getting associations for VPC=%s/RRID=%s\n", vpc.ID, rule.AWSID)
					continue
				}
				log.Printf("%d associations\n", len(out.ResolverRuleAssociations))
				if len(out.ResolverRuleAssociations) == 1 {
					rulesFound = true
					ruleAssociation := &database.ResolverRuleAssociation{
						ResolverRuleID:            rule.AWSID,
						ResolverRuleAssociationID: aws.StringValue(out.ResolverRuleAssociations[0].Id),
					}
					vpc.State.ResolverRuleAssociations = append(vpc.State.ResolverRuleAssociations, ruleAssociation)
				}
			}
			if rulesFound {
				// Rules found, so add to the config
				log.Printf("Rules found for ruleset %d on vpc %s, add to config\n", rs.ID, vpc.ID)
				vpc.Config.ManagedResolverRuleSetIDs = append(vpc.Config.ManagedResolverRuleSetIDs, rs.ID)
			}
		}
		newState, _ := json.Marshal(vpc.State.ResolverRuleAssociations)
		log.Printf("New State:\n%s\n", newState)
		newConfig, _ := json.Marshal(vpc.Config.ManagedResolverRuleSetIDs)
		log.Printf("New Config:\n%s\n", newConfig)
		log.Printf("UpdateVPCConfig() & UpdateVPCState() now...\n=====")
		err = modelsManager.UpdateVPCConfig(vpc.Region, vpc.ID, *vpc.Config)
		if err != nil {
			log.Printf("Error updating config for VPC %s (%s): %s\n", vpc.ID, vpc.AccountID, err)
			continue
		}
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			log.Printf("Error updating state for VPC %s (%s): %s\n", vpc.ID, vpc.AccountID, err)
		}
	}
}
