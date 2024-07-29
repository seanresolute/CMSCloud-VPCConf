package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/apikey"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
)

type health struct {
	Healthy bool
}

type authConfig []struct {
	Principal string   `json:"principal"`
	Roles     []string `json:"roles"`
}

func (a authConfig) principalHasRole(principal string, role string) bool {
	for _, entry := range a {
		if entry.Principal != principal {
			continue
		}
		for _, entryRole := range entry.Roles {
			if role == entryRole {
				return true
			}
		}
	}

	return false
}

func getAuthConfigFromEnvJSON() (authConfig, error) {
	config := authConfig{}

	envConfig := strings.TrimSpace(os.Getenv(apikey.APIKeyEnvVarName))
	if envConfig == "" {
		return nil, fmt.Errorf("environment variable %s must be set", apikey.APIKeyEnvVarName)
	}

	err := json.Unmarshal([]byte(envConfig), &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %s", apikey.APIKeyEnvVarName, err)
	}

	errStrings := []string{}
	for _, entry := range config {
		if strings.TrimSpace(entry.Principal) == "" {
			errStrings = append(errStrings, fmt.Sprintf("%s entry principal string cannot be empty", apikey.APIKeyEnvVarName))
		}
		if len(entry.Roles) == 0 {
			errStrings = append(errStrings, fmt.Sprintf("%s entry roles array cannot be empty for principal %s", apikey.APIKeyEnvVarName, entry.Principal))
		}
		for _, role := range entry.Roles {
			r := strings.TrimSpace(role)
			if r == "" {
				errStrings = append(errStrings, fmt.Sprintf("%s role string cannot be empty for principal %s", apikey.APIKeyEnvVarName, entry.Principal))
				continue
			}
		}
	}

	if len(errStrings) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errStrings, ", "))
	}

	return config, nil
}

type context struct {
	APIKey       apikey.APIKey
	AuthConfig   authConfig
	SSMClient    ssmiface.SSMAPI
	STSClient    stsiface.STSAPI
	ARNContainer string
}

type credsRequest struct {
	AccountID   string
	SessionName string
	Role        string
}

type credsResponse struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

func (r *credsRequest) Validate() error {
	missing := []string{}
	if r.AccountID == "" {
		missing = append(missing, "AccountID")
	}
	if r.SessionName == "" {
		missing = append(missing, "SessionName")
	}
	if r.Role == "" {
		missing = append(missing, "Role")
	}
	if len(missing) > 0 {
		return fmt.Errorf("The following required fields are missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func newContext(apiKey apikey.APIKey, authConf authConfig, arnContainer string) context {
	sess := session.Must(session.NewSession())
	return context{
		SSMClient:    ssm.New(sess),
		STSClient:    sts.New(sess),
		APIKey:       apiKey,
		AuthConfig:   authConf,
		ARNContainer: arnContainer,
	}
}

func logRequestError(r *http.Request, statusCode int, err error) {
	log.Printf("%s %q %d %s", r.RemoteAddr, r.UserAgent(), statusCode, err)
}

func logProcessError(r *http.Request, principal string, statusCode int, err error) {
	log.Printf("%s %q %d %s %s", r.RemoteAddr, r.UserAgent(), statusCode, principal, err)
}

func (c *context) handleCreds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logRequestError(r, http.StatusBadRequest, fmt.Errorf("expected a POST request but got a %s request", r.Method))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	reqData := credsRequest{}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&reqData)
	if err != nil {
		e := fmt.Errorf("error parsing request: %s", err)
		logRequestError(r, http.StatusBadRequest, e)
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	err = reqData.Validate()
	if err != nil {
		e := fmt.Errorf("Invalid request: %s", err)
		logRequestError(r, http.StatusBadRequest, e)
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}

	result := c.APIKey.Validate(r)
	if !result.IsValid() {
		logRequestError(r, result.StatusCode, result.Error)
		http.Error(w, result.Error.Error(), result.StatusCode)
		return
	}

	if !c.AuthConfig.principalHasRole(result.Principal, reqData.Role) {
		logProcessError(r, result.Principal, http.StatusInternalServerError, fmt.Errorf("API key is not authorized for the requested role %s", reqData.Role))
		http.Error(w, "API key is not authorized for the requested role", http.StatusUnauthorized)
		return
	}

	roleARN := fmt.Sprintf("arn:%s:iam::%s:role/%s", c.ARNContainer, reqData.AccountID, reqData.Role)
	out, err := c.STSClient.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         &roleARN,
		RoleSessionName: &reqData.SessionName,
	})
	if err != nil {
		errMsg := fmt.Errorf("AWS error when assuming role %s: %s", reqData.Role, err)
		logProcessError(r, result.Principal, http.StatusUnprocessableEntity, errMsg)
		http.Error(w, errMsg.Error(), http.StatusUnprocessableEntity)
		return
	}

	resp := credsResponse{
		AccessKeyID:     aws.StringValue(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.StringValue(out.Credentials.SecretAccessKey),
		SessionToken:    aws.StringValue(out.Credentials.SessionToken),
		Expiration:      aws.TimeValue(out.Credentials.Expiration),
	}
	b, err := json.Marshal(resp)
	if err != nil {
		errMsg := fmt.Errorf("Error marshaling creds response: %s", err)
		logProcessError(r, result.Principal, http.StatusInternalServerError, errMsg)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("%s %q %d %s %s %s", r.RemoteAddr, r.UserAgent(), http.StatusOK, result.Principal, reqData.Role, reqData.SessionName)

	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	apiHealth := health{
		Healthy: true,
	}
	b, err := json.Marshal(apiHealth)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshaling health response: %s", err), http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func main() {
	apiKeyConfig, errs := apikey.GetConfigFromEnvJSON()
	if errs != nil {
		errs.(apikey.ConfigErrors).LogErrors()
		os.Exit(1)
	}
	apiKey := apikey.APIKey{Config: apiKeyConfig}

	authConfig, err := getAuthConfigFromEnvJSON()
	if err != nil {
		log.Fatalf("Error parsing %s: %s", apikey.APIKeyEnvVarName, err)
	}

	arnContainer := "aws"
	isGovCloud, err := strconv.ParseBool(os.Getenv("IS_GOVCLOUD"))
	if err != nil {
		log.Fatalf("Error parsing IS_GOVCLOUD environment variable: %s", err)
	}
	if isGovCloud {
		arnContainer = "aws-us-gov"
	}

	ctx := newContext(apiKey, authConfig, arnContainer)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/creds", ctx.handleCreds)

	addr := ":2023"
	log.Printf("Listening on port %s", addr)
	log.Fatal(http.ListenAndServe(addr, apiKey.ValidateHandler(mux)))
}
