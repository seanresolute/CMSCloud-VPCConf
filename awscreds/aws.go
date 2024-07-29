package awscreds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
)

type AWSCreds interface {
	GetCredentialsForAccount(accountID string) (*credentials.Credentials, error)
	GetAuthorizedAccounts() ([]*AccountInfo, error)
}

type AccountInfo struct {
	ID, Name, ProjectName string
	ProjectID             int
	IsGovCloud            bool
}

type LocalAWSCreds struct{}

type CloudTamerAWSCreds struct {
	Token                string
	BaseURL              string
	LimitToAWSAccountIDs []string // nil means "all accounts allowed"
}

const accountTypeGovCloud = 2

type cloudTamerAccountsResponse struct {
	Data []*struct {
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
		AccountTypeID int    `json:"account_type_id"`
		ProjectID     int    `json:"project_id"`
	} `json:"data"`
}

type cloudTamerProjectsResponse struct {
	Data []*struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

type cloudTamerCredsResponse struct {
	Data struct {
		AccessKey       string `json:"access_key"`
		SecretAccessKey string `json:"secret_access_key"`
		SessionToken    string `json:"session_token"`
	} `json:"data"`
}

type cloudTamerRolesResponse struct {
	Data struct {
		OUCloudAccessRoles []struct {
			AWSIAMRoleName string `json:"aws_iam_role_name"`
		} `json:"ou_cloud_access_roles"`
	} `json:"data"`
}

// Prefer earlier roles first.
var cloudTamerIAMRoles = []string{
	"ct-gss-network-admin", "ct-gss-network-poweruser", "ct-naworkstream-wsadmin", "ct-naworkstream-networkadmin", "ct-networkarchitecture-admin", "ct-network-architecture-admin", "ct-eastnetworkarchitecture-admin", "network-architecture-workstream-owners", // IA team roles
	"ct-gss-onboarding-readonly",                      // onboard & outreach role
	"ct-gss-network-vpcconf-viewer",                   // corbalt non-engineer read-only role
	"network-architecture-workstream-service-account", //New approach? Check admin permissions before deploy.
}

func selectPreferredIAMRole(ctRoles []struct {
	AWSIAMRoleName string "json:\"aws_iam_role_name\""
}) (string, error) {
	for _, validRole := range cloudTamerIAMRoles {
		for _, role := range ctRoles {
			if role.AWSIAMRoleName == validRole {
				return validRole, nil
			}
		}
	}
	return "", fmt.Errorf("CloudTamer did not return any valid IAM roles to assume")
}

func stringInSlice(s string, a []string) bool {
	for _, t := range a {
		if s == t {
			return true
		}
	}
	return false
}

// TODO: prevent this from running concurrently for the same creds & account ID?
func (c *CloudTamerAWSCreds) GetCredentialsForAccount(accountID string) (*credentials.Credentials, error) {
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	client := &cloudtamer.HTTPClient{Token: c.Token}
	// Determine project ID for this account
	req, _ := http.NewRequest("GET", c.BaseURL+"/v3/account", nil)
	ctar := cloudTamerAccountsResponse{}
	err := client.Do(req, &ctar)
	if err != nil {
		return nil, fmt.Errorf("Error getting account list from CloudTamer: %s", err)
	}
	found := false
	var projectID int
	for _, account := range ctar.Data {
		if account.AccountNumber == accountID {
			found = true
			projectID = account.ProjectID
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("Unknown account number %s", accountID)
	}

	// Determine valid roles for project
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v3/project/%d/cloud-access-role?assumable=true", c.BaseURL, projectID), nil)
	ctrr := cloudTamerRolesResponse{}
	err = client.Do(req, &ctrr)
	if err != nil {
		return nil, fmt.Errorf("Error getting role list from CloudTamer: %s", err)
	}
	iamRole, err := selectPreferredIAMRole(ctrr.Data.OUCloudAccessRoles)
	if err != nil {
		return nil, err
	}

	data := struct {
		AccountNumber string `json:"account_number"`
		IamRoleName   string `json:"iam_role_name"`
	}{
		AccountNumber: accountID,
		IamRoleName:   iamRole,
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling for CloudTamer: %s", err)
	}

	body := bytes.NewReader(payloadBytes)

	req, err = http.NewRequest("POST", c.BaseURL+"/v3/temporary-credentials", body)
	if err != nil {
		return nil, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	creds := cloudTamerCredsResponse{}
	err = client.Do(req, &creds)
	if err != nil {
		return nil, fmt.Errorf("Error getting credentials from CloudTamer: %s", err)
	}
	return credentials.NewStaticCredentials(creds.Data.AccessKey, creds.Data.SecretAccessKey, creds.Data.SessionToken), nil
}

func (c *CloudTamerAWSCreds) GetAuthorizedAccounts() ([]*AccountInfo, error) {
	client := &cloudtamer.HTTPClient{Token: c.Token}
	projectIDToName := make(map[int]string)
	req, err := http.NewRequest("GET", c.BaseURL+"/v3/project", nil)
	if err != nil {
		return nil, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	ctpr := cloudTamerProjectsResponse{}
	err = client.Do(req, &ctpr)
	if err != nil {
		return nil, fmt.Errorf("Error getting project list from CloudTamer: %s", err)
	}
	for _, project := range ctpr.Data {
		projectIDToName[project.ID] = project.Name
	}

	req, err = http.NewRequest("GET", c.BaseURL+"/v3/account", nil)
	if err != nil {
		return nil, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	ctar := cloudTamerAccountsResponse{}
	err = client.Do(req, &ctar)
	if err != nil {
		return nil, fmt.Errorf("Error getting account list from CloudTamer: %s", err)
	}
	seenAccountNumbers := map[string]struct{}{}
	accounts := make([]*AccountInfo, 0)
	for _, account := range ctar.Data {
		// Dedupe in case CloudTamer added the same account multiple times
		if _, ok := seenAccountNumbers[account.AccountNumber]; ok {
			continue
		}
		if c.LimitToAWSAccountIDs != nil && !stringInSlice(account.AccountNumber, c.LimitToAWSAccountIDs) {
			continue
		}
		seenAccountNumbers[account.AccountNumber] = struct{}{}
		accounts = append(accounts, &AccountInfo{
			ID:          account.AccountNumber,
			Name:        account.AccountName,
			ProjectName: projectIDToName[account.ProjectID],
			ProjectID:   account.ProjectID,
			IsGovCloud:  account.AccountTypeID == accountTypeGovCloud,
		})
	}
	return accounts, nil
}

func (c *LocalAWSCreds) GetCredentialsForAccount(accountID string) (*credentials.Credentials, error) {
	return credentials.NewEnvCredentials(), nil
}

const stsRegion = "us-east-1"

func (c *LocalAWSCreds) GetAuthorizedAccounts() ([]*AccountInfo, error) {
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(stsRegion),
	}))
	stssvc := sts.New(sess)
	idOut, err := stssvc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, err
	}
	iamsvc := iam.New(sess)
	aliasOut, err := iamsvc.ListAccountAliases(&iam.ListAccountAliasesInput{})
	if err != nil {
		return nil, err
	}
	alias := "?"
	if len(aliasOut.AccountAliases) > 0 {
		alias = *aliasOut.AccountAliases[0]
	}
	return []*AccountInfo{{ID: *idOut.Account, Name: alias}}, nil
}

// Returns a link that will sign the user in to AWS and forward them to the given destinationURL,
// per the method specified at https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html.
//
// issuerURL will be linked to by AWS when the session expires.
func CreateSignInURL(creds *credentials.Credentials, destinationURL, issuerURL string, isGovCloud bool) (string, error) {
	val, err := creds.Get()
	if err != nil {
		return "", err
	}
	d := map[string]string{
		"sessionId":    val.AccessKeyID,
		"sessionKey":   val.SecretAccessKey,
		"sessionToken": val.SessionToken,
	}
	buf, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("Error marshalling credentials: %s", err)
	}
	u := &url.URL{
		Scheme: "https",
		Host:   "signin.aws.amazon.com",
		Path:   "federation",
	}
	if isGovCloud {
		u.Host = "signin.amazonaws-us-gov.com"
	}
	q := u.Query()
	q.Set("Action", "getSigninToken")
	q.Set("Session", string(buf))
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("Error getting signin token: %s", err)
	}
	data := &struct {
		SigninToken string
	}{}
	err = json.NewDecoder(resp.Body).Decode(data)
	if err != nil {
		return "", fmt.Errorf("Error decoding signin token: %s", err)
	}
	if data.SigninToken == "" {
		return "", fmt.Errorf("No signin token provided by AWS")
	}

	q = url.Values{}
	q.Set("Action", "login")
	q.Set("Issuer", issuerURL)
	q.Set("Destination", destinationURL)
	q.Set("SigninToken", data.SigninToken)
	u.RawQuery = q.Encode()

	return u.String(), nil
}
