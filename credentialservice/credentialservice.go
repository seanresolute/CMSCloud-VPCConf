package credentialservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
)

type cloudConfiguration struct {
	APIKey string
	Host   string
	Role   string
}

func (c *cloudConfiguration) IsValid() bool {
	return c.APIKey != "" && c.Host != "" && c.Role != ""
}

type CredentialServiceConfig struct {
	Commercial cloudConfiguration
	GovCloud   cloudConfiguration
}

type CredentialService struct {
	Config *CredentialServiceConfig
}

type CredentialServiceRequest struct {
	AccountID   string
	SessionName string
	Role        string
}

type CredentialServiceHealth struct {
	Commercial bool
	GovCloud   bool
}

type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

type CredentialsProvider interface {
	GetAWSCredentials(accountID, region, asUser string) (*Credentials, error)
	GetAWSSession(accountID, region, asUser string) (*awssession.Session, error)
}

func GetConfigFromENV() (*CredentialServiceConfig, error) {
	invalid := []string{}

	readFromENV := func(dest *string, key string) {
		*dest = os.Getenv(key)
		if *dest == "" {
			invalid = append(invalid, fmt.Sprintf("environment variable %q cannot be empty", key))
		}
	}

	config := &CredentialServiceConfig{}
	jsonString := ""

	readFromENV(&jsonString, "CREDS_SVC_CONFIG")

	if len(invalid) > 0 {
		return nil, fmt.Errorf(strings.Join(invalid, ", "))
	}

	err := json.Unmarshal([]byte(jsonString), &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (cs *CredentialService) GetServiceHealth() CredentialServiceHealth {
	health := CredentialServiceHealth{}

	getHealthResponse := func(host string) bool {
		if host == "" {
			return false
		}

		endpoint := host + "/health"
		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := client.Get(endpoint)
		if err != nil {
			log.Printf("Health Check Error talking to CredentialsService: %s", err)
			return false
		}
		if resp.StatusCode == http.StatusOK {
			// no need to parse the body as it is hardcoded to Healthy: true
			return true
		}
		log.Printf("CredentialsService Health Check Unexpected status %d from %s", resp.StatusCode, endpoint)
		return false
	}

	health.Commercial = getHealthResponse(cs.Config.Commercial.Host)
	health.GovCloud = getHealthResponse(cs.Config.GovCloud.Host)

	return health
}

func (cs *CredentialService) GetAWSCredentials(accountID, region, asUser string) (*Credentials, error) {
	if accountID == "" || asUser == "" || region == "" {
		return nil, fmt.Errorf("accountID, region, and asUser must be defined")
	}

	var cloudConfig *cloudConfiguration
	if strings.HasPrefix(region, "us-gov-") {
		cloudConfig = &cs.Config.GovCloud
	} else {
		cloudConfig = &cs.Config.Commercial
	}
	if !cloudConfig.IsValid() {
		return nil, fmt.Errorf("invalid credentials configuration for %s", region)
	}

	reqBody := &CredentialServiceRequest{
		AccountID:   accountID,
		SessionName: asUser,
		Role:        cloudConfig.Role,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, cloudConfig.Host+"/creds", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cloudConfig.APIKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading Creds API error message: %s", err)
		}
		return nil, fmt.Errorf("Failed to fetch credentials: %s. Creds API Error: %s", resp.Status, buf)
	}

	defer resp.Body.Close()

	response := &Credentials{}

	err = json.NewDecoder(resp.Body).Decode(response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (cs *CredentialService) GetAWSSession(accountID, region, asUser string) (*awssession.Session, error) {
	creds, err := cs.GetAWSCredentials(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	awsSession := awssession.Must(awssession.NewSession(&aws.Config{
		Region:      &region,
		Credentials: credentials.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken),
	}))
	return awsSession, nil
}
