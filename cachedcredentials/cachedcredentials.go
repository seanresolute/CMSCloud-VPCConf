package cachedcredentials

import (
	"fmt"
	"log"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/session"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
)

// CachedCredentials satisfies the CredentialsProvider interface in credentialservice.go
type CachedCredentials struct {
	CredentialsProvider credentialservice.CredentialsProvider
	SessionStore        *session.SQLSessionStore
}

func (cc *CachedCredentials) GetAWSCredentials(accountID, region, asUser string) (*credentialservice.Credentials, error) {
	if accountID == "" || region == "" || asUser == "" {
		return nil, fmt.Errorf("accountID, region, and asUser must be defined")
	}

	creds, err := cc.SessionStore.GetAWSCredentials(asUser, accountID)
	if err != nil {
		return nil, err
	}
	if creds == nil {
		freshCreds, err := cc.CredentialsProvider.GetAWSCredentials(accountID, region, asUser)
		if err != nil {
			return nil, err
		}
		err = cc.SessionStore.UpdateAWSCredentials(asUser, accountID, *freshCreds)
		if err != nil {
			log.Printf("failed to update credentials cache for user %s and account %s", asUser, accountID)
		}

		return freshCreds, nil
	}

	return creds, nil
}

func (cc *CachedCredentials) GetAWSSession(accountID, region, asUser string) (*awssession.Session, error) {
	creds, err := cc.GetAWSCredentials(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	awsSession := awssession.Must(awssession.NewSession(&aws.Config{
		Region:      &region,
		Credentials: credentials.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
	}))
	return awsSession, nil
}
