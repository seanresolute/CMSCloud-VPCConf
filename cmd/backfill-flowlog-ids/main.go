package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/jmoiron/sqlx"
)

const cloudwatchFlowlogsRole = "cms-cloud-cloudwatch-flowlogs-role"
const flowLogGroupName = "vpc-flowlogs"

func flowLogS3Destination(accountID string) string {
	return fmt.Sprintf("arn:aws:s3:::%s/", accountID)
}

func roleARN(accountID, roleName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
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
		Name:           aws.String("backfill-flowlogs-params"),
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

	for _, vpc := range vpcs {
		vpc, vpcWriter, err := modelsManager.GetOperableVPC(lockSet, vpc.Region, vpc.ID)
		if err != nil {
			log.Printf("ERROR getting VPC %s: %s", vpc.ID, err)
			continue
		}
		log.Printf("VPC %s   Account %s   Region %s", vpc.ID, vpc.AccountID, vpc.Region)
		if vpc.State.S3FlowLogID != "" && vpc.State.CloudWatchLogsFlowLogID != "" {
			log.Printf("  VPC %s already has FlowLogs recorded", vpc.ID)
			continue
		}
		awscreds := credsByAccountID[vpc.AccountID]
		if awscreds == nil {
			log.Printf("  Getting credentials for account %s", vpc.AccountID)
			awscreds, err = creds.GetCredentialsForAccount(vpc.AccountID)
			if err != nil {
				log.Printf("ERROR getting credentials for account %s: %s", vpc.AccountID, err)
				continue
			}
			credsByAccountID[vpc.AccountID] = awscreds
		}
		sess := session.Must(session.NewSession(&aws.Config{
			Region:      aws.String(string(vpc.Region)),
			Credentials: awscreds,
		}))
		ec2svc := ec2.New(sess)
		out, err := ec2svc.DescribeFlowLogs(&ec2.DescribeFlowLogsInput{
			Filter: []*ec2.Filter{
				{
					Name:   aws.String("resource-id"),
					Values: []*string{&vpc.ID},
				},
			},
		})
		if err != nil {
			log.Printf("ERROR listing FlowLogs for VPC %s: %s", vpc.ID, err)
			continue
		}
		flowLogsByID := map[string]*ec2.FlowLog{}
		for _, flowLog := range out.FlowLogs {
			flowLogsByID[aws.StringValue(flowLog.FlowLogId)] = flowLog
		}
		if vpc.State.S3FlowLogID == "" {
			for id, flowLog := range flowLogsByID {
				if aws.StringValue(flowLog.LogDestinationType) == ec2.LogDestinationTypeS3 &&
					aws.StringValue(flowLog.LogDestination) == flowLogS3Destination(vpc.AccountID) &&
					aws.StringValue(flowLog.TrafficType) == ec2.TrafficTypeAll {
					log.Printf("  Found S3 FlowLog %s for VPC %s", id, vpc.ID)
					vpc.State.S3FlowLogID = id
					err = vpcWriter.UpdateState(vpc.State)
					if err != nil {
						log.Printf("  Error updating state: %s", err)
					}
					delete(flowLogsByID, id)
					break
				}
			}
			if vpc.State.S3FlowLogID == "" {
				log.Printf("  No S3 FlowLog for VPC %s", vpc.ID)
			}
		}
		if vpc.State.CloudWatchLogsFlowLogID == "" {
			for id, flowLog := range flowLogsByID {
				if aws.StringValue(flowLog.LogDestinationType) == ec2.LogDestinationTypeCloudWatchLogs &&
					aws.StringValue(flowLog.DeliverLogsPermissionArn) == roleARN(vpc.AccountID, cloudwatchFlowlogsRole) &&
					aws.StringValue(flowLog.LogGroupName) == flowLogGroupName &&
					aws.StringValue(flowLog.TrafficType) == ec2.TrafficTypeAll {
					log.Printf("  Found CloudWatch Logs FlowLog %s for VPC %s", id, vpc.ID)
					vpc.State.CloudWatchLogsFlowLogID = id
					err = vpcWriter.UpdateState(vpc.State)
					if err != nil {
						log.Printf("  Error updating state: %s", err)
					}
					delete(flowLogsByID, id)
					break
				}
			}
			if vpc.State.CloudWatchLogsFlowLogID == "" {
				log.Printf("  No CloudWatch Logs FlowLog for VPC %s", vpc.ID)
			}
		}
		for id := range flowLogsByID {
			log.Printf("  Unknown FlowLog %s", id)
		}
	}
}
