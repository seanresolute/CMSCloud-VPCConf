package cmsnet

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type RegionConfig struct {
	BaseURL      string
	APIID        string
	TGWAccountID string
}
type Config map[database.Region]*RegionConfig

type ClientInterface interface {
	GetBrokenActivations(accountID string, region database.Region, vpcID string, asUser string) ([]Activation, error)
	GetAllConnectionRequests(accountID string, region database.Region, vpcID string, asUser string) (*ConnectionData, error)
	DeleteActivation(requestID, accountID string, region database.Region, vpcID string, asUser string) (*ConnectionData, error)
	MakeConnectionRequest(accountID string, region database.Region, vpcID string, params *ConnectionRequestParams, asUser string) (*ConnectionData, error)
	GetAllNATRequests(accountID string, region database.Region, vpcID string, asUser string) (*NATData, error)
	DeleteNAT(requestID, accountID string, region database.Region, vpcID string, keepIPReserved bool, asUser string) (*NATData, error)
	MakeNATRequest(accountID string, region database.Region, vpcID string, params *NATRequestParams, asUser string) (*NATData, error)
	SupportsRegion(region database.Region) bool
}

func NewClient(config Config, limitToAWSAccountIDs []string, credsProvider credentialservice.CredentialsProvider) ClientInterface {
	return &client{
		Config:               config,
		LimitToAWSAccountIDs: limitToAWSAccountIDs,
		CredentialsProvider:  credsProvider,
	}
}

type client struct {
	Config               Config
	LimitToAWSAccountIDs []string // nil means "all accounts allowed"
	CredentialsProvider  credentialservice.CredentialsProvider
}

// TODO: create constants and validate from list when unmarshaling JSON
type ConnectionStatus string
type ConnectionState int
type DeletionStatus string
type DeletionState int

const (
	ConnectionStatusInProgress ConnectionStatus = "IN_PROGRESS"
	ConnectionStatusRequested  ConnectionStatus = "REQUESTED"
	ConnectionStatusSuccessful ConnectionStatus = "SUCCESSFUL"
	DeletionStatusSuccessful   DeletionStatus   = "SUCCESSFUL"
)

func (s ConnectionStatus) BlocksDeletion() bool {
	return s == ConnectionStatusInProgress || s == ConnectionStatusRequested
}

type Activation struct {
	RequestID       string `json:"sha256_hash"`
	SubnetID        string `json:"subnet_id"`
	DestinationCIDR string `json:"destination_cidr"`
	VRF             string `json:"vrf"`
}

type AWSCred struct {
	AccessKeyID     string `json:"aws_access_key_id"`
	SecretAccessKey string `json:"aws_secret_access_key"`
	SessionToken    string `json:"aws_session_token"`
}
type AWSCreds struct {
	Central *AWSCred `json:"central"`
	Target  *AWSCred `json:"target"`
}
type ConnectionRequestParams struct {
	SubnetID            string   `json:"subnet_id"`
	DestinationCIDR     string   `json:"destination_cidr"`
	VRF                 string   `json:"vrf"`
	AttachmentSubnetIDs []string `json:"attachment_subnet_ids"`
}
type ConnectionRequest struct {
	ID                       string                  `json:"sha256_hash"`
	ConnectionState          ConnectionState         `json:"deployment_state"`
	ConnectionStatus         ConnectionStatus        `json:"deployment_status"`
	ConnectionMessages       []string                `json:"deployment_progress"`
	DeletionState            DeletionState           `json:"deletion_state"`
	DeletionStatus           DeletionStatus          `json:"deletion_status"`
	DeletionMessages         []string                `json:"deletion_progress"`
	ConnectionFailureMessage string                  `json:"deployment_failure_message"`
	DeletionFailureMessage   string                  `json:"deletion_failure_message"`
	Params                   ConnectionRequestParams `json:"wan_request_body"`
}

type ConnectionData struct {
	Requests    []ConnectionRequest `json:"wan_requests"`
	Activations []Activation        `json:"wan_activations"`
}
type NATRequestParams struct {
	VRF            string `json:"vrf"`
	InsideNetwork  string `json:"inside_network"`
	OutsideNetwork string `json:"outside_network,omitempty"`
}
type DeleteNATParams struct {
	KeepIPReserved bool `json:"keep_nat_reserved"`
}
type WANActivationResponse struct {
	DatabaseResults ConnectionData `json:"database_results"`
	MissingInAWS    ConnectionData `json:"missing_in_aws"`
}
type NATRequest struct {
	ID                       string           `json:"sha256_hash"`
	ConnectionState          ConnectionState  `json:"deployment_state"`
	ConnectionStatus         ConnectionStatus `json:"deployment_status"`
	ConnectionMessages       []string         `json:"deployment_progress"`
	DeletionState            DeletionState    `json:"deletion_state"`
	DeletionStatus           DeletionStatus   `json:"deletion_status"`
	DeletionMessages         []string         `json:"deletion_progress"`
	ConnectionFailureMessage string           `json:"deployment_failure_message"`
	DeletionFailureMessage   string           `json:"deletion_failure_message"`
	Params                   NATRequestParams `json:"nat_request_body"`
}
type NAT struct {
	RequestID      string `json:"sha256_hash"`
	InsideNetwork  string `json:"inside_network"`
	OutsideNetwork string `json:"outside_network"`
	VRF            string `json:"vrf"`
}
type NATData struct {
	Requests []NATRequest `json:"nat_requests"`
	NATs     []NAT        `json:"nats"`
}
type NATResponse struct {
	DatabaseResults NATData `json:"database_results"`
	MissingInAWS    NATData `json:"missing_in_aws"`
}
type RouteData struct {
	CMSNetCIDR       string `json:"cmsnet_cidr"`
	VRF              string `json:"vrf"`
	MatchedIPAddress string `json:"matched_ip_address"`
}
type RouteResponse struct {
	CMSNetRoutes []*RouteData `json:"cmsnet_routes"`
}

func (c *client) do(req *http.Request, config *RegionConfig, authHeader string, result interface{}) error {
	req.Header.Add("x-apigw-api-id", config.APIID)
	req.Header.Add("Authorization", authHeader)
	req.Header.Add("Content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Error getting connection requests: %s", err)
	}
	if resp.StatusCode >= 300 {
		buf, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Response from %s %s: %s", req.Method, req.URL, buf)
		return fmt.Errorf("CMSNet connection API returned status %s", resp.Status)
	}
	err = json.NewDecoder(resp.Body).Decode(result)
	if err != nil {
		return fmt.Errorf("Error getting connection requests: %s", err)
	}
	return nil
}

func (c *client) GetBrokenActivations(accountID string, region database.Region, vpcID string, asUser string) ([]Activation, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%ssynchronizations/%s/%s/%s", config.BaseURL, region, accountID, vpcID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &WANActivationResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return result.MissingInAWS.Activations, nil
}

func (c *client) SupportsRegion(region database.Region) bool {
	return c.Config[region] != nil
}

func (c *client) GetAllConnectionRequests(accountID string, region database.Region, vpcID string, asUser string) (*ConnectionData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%swan_activations/%s/%s/%s", config.BaseURL, region, accountID, vpcID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &WANActivationResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func (c *client) DeleteActivation(requestID, accountID string, region database.Region, vpcID string, asUser string) (*ConnectionData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%swan_activations/%s/%s/%s/%s", config.BaseURL, region, accountID, vpcID, requestID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &WANActivationResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func stringInSlice(s string, a []string) bool {
	for _, t := range a {
		if s == t {
			return true
		}
	}
	return false
}

func (c *client) MakeConnectionRequest(accountID string, region database.Region, vpcID string, params *ConnectionRequestParams, asUser string) (*ConnectionData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%swan_activations/%s/%s/%s", config.BaseURL, region, accountID, vpcID)
	buf, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("Error encoding JSON: %s", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &WANActivationResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func (c *client) GetAllNATRequests(accountID string, region database.Region, vpcID string, asUser string) (*NATData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%snats/%s/%s/%s", config.BaseURL, region, accountID, vpcID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &NATResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func (c *client) DeleteNAT(requestID, accountID string, region database.Region, vpcID string, keepIPReserved bool, asUser string) (*NATData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	params := &DeleteNATParams{
		KeepIPReserved: keepIPReserved,
	}
	buf, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("Error encoding JSON: %s", err)
	}
	url := fmt.Sprintf("%snats/%s/%s/%s/%s", config.BaseURL, region, accountID, vpcID, requestID)
	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &NATResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func (c *client) MakeNATRequest(accountID string, region database.Region, vpcID string, params *NATRequestParams, asUser string) (*NATData, error) {
	config := c.Config[region]
	if config == nil {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	if c.LimitToAWSAccountIDs != nil && !stringInSlice(accountID, c.LimitToAWSAccountIDs) {
		return nil, fmt.Errorf("Access to account %q is not allowed", accountID)
	}
	url := fmt.Sprintf("%snats/%s/%s/%s", config.BaseURL, region, accountID, vpcID)
	buf, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("Error encoding JSON: %s", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	authHeader, err := c.generateCMSNetAuthorizationHeader(accountID, region, asUser)
	if err != nil {
		return nil, err
	}
	result := &NATResponse{}
	err = c.do(req, config, authHeader, result)
	if err != nil {
		return nil, err
	}
	return &result.DatabaseResults, nil
}

func (c *client) GetConfig(region database.Region) (*RegionConfig, error) {
	if !c.SupportsRegion(region) {
		return nil, fmt.Errorf("Region %q is not supported by CMSNet", region)
	}
	return c.Config[region], nil
}

func (c *client) generateCMSNetAuthorizationHeader(accountID string, region database.Region, asUser string) (string, error) {
	centralConfig, err := c.GetConfig(region)
	if err != nil {
		return "", err
	}
	central, err := c.CredentialsProvider.GetAWSCredentials(centralConfig.TGWAccountID, string(region), asUser)
	if err != nil {
		return "", err
	}
	target, err := c.CredentialsProvider.GetAWSCredentials(accountID, string(region), asUser)
	if err != nil {
		return "", err
	}
	authorizationInfo := &AWSCreds{
		Central: &AWSCred{
			AccessKeyID:     central.AccessKeyID,
			SecretAccessKey: central.SecretAccessKey,
			SessionToken:    central.SessionToken,
		},
		Target: &AWSCred{
			AccessKeyID:     target.AccessKeyID,
			SecretAccessKey: target.SecretAccessKey,
			SessionToken:    target.SessionToken,
		},
	}
	authorizationJson, err := json.MarshalIndent(authorizationInfo, "", "   ")
	if err != nil {
		return "", fmt.Errorf("Error json-encoding authorization: %w", err)
	}
	return fmt.Sprintf("AWS %s", base64.StdEncoding.EncodeToString(authorizationJson)), nil
}
