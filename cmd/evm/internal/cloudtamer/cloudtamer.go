package cloudtamer

import (
	"fmt"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/evm/internal/conf"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	awsc "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
)

var (
	sessionCache = map[string]session.Session{}
)

// CloudTamer core
type CloudTamer struct {
	Config      *conf.Conf
	credentials *awsc.CloudTamerAWSCreds
	expiry      *time.Time
	Region      string
}

// ValidateToken ensures the CloudTamer token is valid
func (ct *CloudTamer) ValidateToken() error {
	if ct.expiry == nil || time.Now().After(*ct.expiry) || ct.credentials == nil {
		tokenProvider := &cloudtamer.TokenProvider{
			Config: cloudtamer.CloudTamerConfig{
				BaseURL:      ct.Config.CloudTamerBaseURL,
				IDMSID:       ct.Config.CloudTamerIDMSID,
				AdminGroupID: ct.Config.CloudTamerAdminGroupID,
			},
			Username: ct.Config.Username,
			Password: ct.Config.Password,
		}

		token, err := tokenProvider.GetToken()
		if err != nil {
			return fmt.Errorf("Error getting CloudTamer token: %s", err)
		}
		t := time.Now().Add(time.Minute * 105)
		ct.expiry = &t
		ct.credentials = &awsc.CloudTamerAWSCreds{Token: token, BaseURL: ct.Config.CloudTamerBaseURL}
	}

	return nil
}

// GetAWSCredsForAccountID returns the credentials for the given account ID
func (ct *CloudTamer) GetAWSCredsForAccountID(accountID string) (*credentials.Credentials, error) {
	err := ct.ValidateToken()
	if err != nil {
		return nil, err
	}

	awsCredentials, err := ct.credentials.GetCredentialsForAccount(accountID)

	if err != nil {
		return nil, fmt.Errorf("Failed to get AWS credentials for account %s: %s", accountID, err)
	}

	return awsCredentials, nil
}

// GetAWSSessionForAccountID returns a session from the cache or caches the new session for the given account ID
func (ct *CloudTamer) GetAWSSessionForAccountID(accountID string) (*session.Session, error) {
	if accountID == "" {
		return nil, fmt.Errorf("accountID is empty")
	}

	cachedSession, ok := sessionCache[accountID]
	if ok {
		return &cachedSession, nil
	}

	creds, err := ct.GetAWSCredsForAccountID(accountID)
	if err != nil {
		return nil, err
	}

	awsSession := session.Must(session.NewSession(&aws.Config{
		Region:      &ct.Region,
		Credentials: creds,
	}))

	sessionCache[accountID] = *awsSession

	return awsSession, nil
}
