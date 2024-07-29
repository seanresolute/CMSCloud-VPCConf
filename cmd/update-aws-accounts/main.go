package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

const updatesMetricName = "Updates"

func main() {
	os.Setenv("POSTGRES_CONNECTION_STRING", "postgresql://postgres:A1w@ysChng1ng@10.147.152.14:5432/postgres?sslmode=disable")
	os.Setenv("CLOUDTAMER_BASE_URL", "https://cloudtamer.cms.gov/api")
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_USERNAME", "vpcconf-svc")
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_PASSWORD", "aknCfdFg129ndI")
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID", "2")

	postgresConnectionString := os.Getenv("POSTGRES_CONNECTION_STRING")
	db := sqlx.MustConnect("postgres", postgresConnectionString)

	cloudTamerBaseURL := os.Getenv("CLOUDTAMER_BASE_URL")
	cloudTamerServiceAccountUsername := os.Getenv("CLOUDTAMER_SERVICE_ACCOUNT_USERNAME")
	cloudTamerServiceAccountPassword := os.Getenv("CLOUDTAMER_SERVICE_ACCOUNT_PASSWORD")
	cloudTamerServiceAccountIDMSIDStr := os.Getenv("CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID")
	if cloudTamerBaseURL == "" || cloudTamerServiceAccountUsername == "" || cloudTamerServiceAccountPassword == "" || cloudTamerServiceAccountIDMSIDStr == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "CLOUDTAMER_BASE_URL, CLOUDTAMER_SERVICE_ACCOUNT_USERNAME, CLOUDTAMER_SERVICE_ACCOUNT_PASSWORD, and CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID env variables are required")
		os.Exit(2)
	}
	cloudTamerServiceAccountIDMSID, err := strconv.Atoi(cloudTamerServiceAccountIDMSIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", "Invalid CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID")
		os.Exit(2)
	}

	var cloudwatchSvc *cloudwatch.CloudWatch
	metricNamespace := os.Getenv("CLOUDWATCH_METRIC_NAMESPACE")
	if metricNamespace != "" {
		cloudwatchSvc = cloudwatch.New(session.Must(session.NewSession()))
	}

	cloudTamerTokenProvider := &cloudtamer.TokenProvider{
		Config: cloudtamer.CloudTamerConfig{
			BaseURL: cloudTamerBaseURL,
			IDMSID:  cloudTamerServiceAccountIDMSID,
		},
		Username: cloudTamerServiceAccountUsername,
		Password: cloudTamerServiceAccountPassword,
	}
	modelsManager := &database.SQLModelsManager{
		DB: db,
	}

	ticker := time.NewTicker(time.Minute)
	for ; true; <-ticker.C { // idiosyncratic loop to make one iteration happen immediately
		log.Printf("Syncing accounts with CloudTamer")

		token, err := cloudTamerTokenProvider.GetToken()
		if err != nil {
			log.Fatalf("Error authenticating service account: %s", err)
		}
		creds := &awscreds.CloudTamerAWSCreds{
			Token:   token,
			BaseURL: cloudTamerTokenProvider.Config.BaseURL,
		}

		accountsInDatabase, err := modelsManager.GetAllAWSAccounts()
		if err != nil {
			log.Fatalf("Error getting list of active accounts from database: %s", err)
		}
		existingAccountsByID := map[string]*database.AWSAccount{}
		for _, account := range accountsInDatabase {
			existingAccountsByID[account.ID] = account
		}

		accountsInCloudTamer, err := creds.GetAuthorizedAccounts()
		if err != nil {
			log.Fatalf("Error getting list of active accounts from CloudTamer: %s", err)
		}

		for _, account := range accountsInCloudTamer {
			inCloudTamer := &database.AWSAccount{
				ID:          account.ID,
				Name:        account.Name,
				ProjectName: account.ProjectName,
				IsGovCloud:  account.IsGovCloud,
			}
			inDatabase := existingAccountsByID[account.ID]
			if inDatabase == nil || *inDatabase != *inCloudTamer {
				log.Printf("Creating or updating account %s", inCloudTamer.ID)
				_, err := modelsManager.CreateOrUpdateAWSAccount(inCloudTamer)
				if err != nil {
					log.Fatalf("Error updating account: %s", err)
				}
			}
			delete(existingAccountsByID, account.ID)
		}

		for _, account := range existingAccountsByID {
			log.Printf("Marking account %s inactive", account.ID)
			err := modelsManager.MarkAWSAccountInactive(account.ID)
			if err != nil {
				log.Fatalf("Error marking account inactive: %s", err)
			}
		}

		// Record that a successful sync was performed at this time regardless of if anything needed to be updated
		err = modelsManager.RecordUpdateAWSAccountsHeartbeat()
		if err != nil {
			log.Fatalf("Error recording sync")
		}

		if cloudwatchSvc != nil {
			log.Printf("Updating metric")
			_, err := cloudwatchSvc.PutMetricData(&cloudwatch.PutMetricDataInput{
				Namespace: aws.String(metricNamespace),
				MetricData: []*cloudwatch.MetricDatum{
					{
						MetricName: aws.String(updatesMetricName),
						Timestamp:  aws.Time(time.Now()),
						Value:      aws.Float64(1),
					},
				},
			})
			if err != nil {
				log.Printf("Error updating metric: %s", err)
			}
		}

		log.Printf("Done syncing")
	}
}
