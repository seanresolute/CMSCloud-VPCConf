package jira

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dghubble/oauth1"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type Config struct {
	Project        string
	IssueType      string
	Statuses       map[string]database.VPCRequestStatus
	DNSTLSStatuses map[string]database.DNSTLSRequestStatus
	Watchers       []string
	Assignee       string
}

type ClientInterface interface {
	CreateIssue(details *IssueDetails) (string, error)
	VerifyAccess() error
	GetIssueStatus(id string) (database.VPCRequestStatus, error)
	SetIssueStatus(issueID string, status database.VPCRequestStatus) error
	GetDNSTLSIssueStatus(id string) (database.DNSTLSRequestStatus, error)
	SetDNSTLSIssueStatus(issueID string, status database.DNSTLSRequestStatus) error
	PostComment(issueID, comment string) error
}

type OauthConfig struct {
	ConsumerKey, PrivateKey string
	Token                   string
}

type Client struct {
	Config
	Username string

	oauth1Config *oauth1.Config
	oauthToken   string
}

func (c *Client) AddAuthentication(oauthConfig *OauthConfig) error {
	oauth1Config, err := GenerateOauth1Config(oauthConfig)
	if err != nil {
		return err
	}
	c.oauth1Config = oauth1Config
	c.oauthToken = oauthConfig.Token
	return nil
}

type IssueDetails struct {
	Reporter    string
	Summary     string
	Description string
	Labels      []string
}

const BaseURL = "https://jiraent.cms.gov/"

// JiraURL is a helper method to decide if the issue is an URL (starter box) or
// only an issue ID (QuickVPC) - fix QuickVPC so that it sends a full URL
// then this extra function can be removed
func JiraURL(issueURL string) string {
	if strings.HasPrefix(issueURL, "https://") {
		return issueURL
	}

	return BaseURL + "browse/" + issueURL
}

func GenerateOauth1Config(c *OauthConfig) (*oauth1.Config, error) {
	keyDERBlock, _ := pem.Decode([]byte(c.PrivateKey))
	if keyDERBlock == nil {
		return nil, fmt.Errorf("unable to decode key PEM block")
	}
	if !(keyDERBlock.Type == "PRIVATE KEY" || strings.HasSuffix(keyDERBlock.Type, " PRIVATE KEY")) {
		return nil, fmt.Errorf("unexpected key DER block type: %s", keyDERBlock.Type)
	}

	pk, err := x509.ParsePKCS1PrivateKey(keyDERBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unable to parse PKCS1 private key. %v", err)
	}

	server, err := url.Parse(BaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse server URL '%v': %v", BaseURL, err)
	}

	return &oauth1.Config{
		ConsumerKey: c.ConsumerKey,
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: server.String() + "plugins/servlet/oauth/request-token",
			AuthorizeURL:    server.String() + "plugins/servlet/oauth/authorize",
			AccessTokenURL:  server.String() + "plugins/servlet/oauth/access-token",
		},
		Signer: &oauth1.RSASigner{
			PrivateKey: pk,
		},
	}, nil
}

func (c *Client) httpClient() *http.Client {
	token := oauth1.NewToken(c.oauthToken, "") // token secret is ignored for RSA signatures
	client := c.oauth1Config.Client(context.Background(), token)
	client.Timeout = 30 * time.Second
	return client
}

func (c *Client) setIssueStatus(issueID string, statusID string) error {
	// Find transition ID for status
	url := BaseURL + "rest/api/2/issue/" + issueID + "/transitions"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		}
		log.Printf("JIRA response: %s", buf)
		return fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
	}
	var respData struct {
		Transitions []struct {
			ID string `json:"id"`
			To struct {
				ID string `json:"id"`
			} `json:"to"`
		} `json:"transitions"`
	}
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return fmt.Errorf("Error reading response from JIRA: %s", err)
	}
	for _, transition := range respData.Transitions {
		if transition.To.ID == statusID {
			// Found transition ID; now do it
			data := map[string]interface{}{
				"transition": map[string]interface{}{
					"id": transition.ID,
				},
			}
			buf, err := json.Marshal(data)
			if err != nil {
				return fmt.Errorf("Error marshalling: %s", err)
			}
			req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(buf))
			req.Header.Set("Content-Type", "application/json")
			resp, err := c.httpClient().Do(req)
			if err != nil {
				return fmt.Errorf("Error talking to JIRA: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 204 {
				buf, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("Error reading JIRA response: %s", err)
				}
				log.Printf("JIRA response: %s", buf)
				return fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
			}
			return nil
		}
	}
	return fmt.Errorf("No transition for status %q", statusID)
}

func (c *Client) CreateIssue(details *IssueDetails) (string, error) {
	data := map[string]interface{}{
		"fields": map[string]interface{}{
			"project": map[string]interface{}{
				"key": c.Project,
			},
			"summary":     details.Summary,
			"description": details.Description,
			"issuetype": map[string]interface{}{
				"id": c.IssueType,
			},
			"labels": details.Labels,
		},
	}

	if c.Assignee != "" {
		data["fields"].(map[string]interface{})["assignee"] = map[string]interface{}{"name": c.Assignee}
		data["fields"].(map[string]interface{})["reporter"] = map[string]interface{}{"name": c.Assignee}
	}

	buf, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("Error marshalling: %s", err)
	}
	req, _ := http.NewRequest(http.MethodPost, BaseURL+"rest/api/2/issue/", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		} else {
			log.Printf("JIRA response: %s", buf)
		}
		return "", fmt.Errorf("Unexpected status %s from JIRA, response: %s", resp.Status, buf)
	}
	var respData struct {
		Key string `json:"key"`
	}
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("Error reading response from JIRA: %s", err)
	}

	for _, watcher := range c.Config.Watchers {
		buf, err := json.Marshal(watcher)
		if err != nil {
			log.Printf("Error marshalling watcher %q: %s", watcher, err)
		} else {
			url := fmt.Sprintf("%srest/api/2/issue/%s/watchers", BaseURL, respData.Key)
			req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(buf))
			req.Header.Set("Content-Type", "application/json")
			resp, err := c.httpClient().Do(req)
			if err != nil {
				log.Printf("Error talking to JIRA: %s", err)
			} else if resp.StatusCode != 204 {
				buf, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("Error reading JIRA response: %s", err)
				} else {
					log.Printf("JIRA response: %s", buf)
				}
				log.Printf("Unexpected status %s from JIRA watch request", resp.Status)
			}
			resp.Body.Close()
		}
	}

	return respData.Key, nil
}

func (c *Client) VerifyAccess() error {
	req, _ := http.NewRequest(http.MethodGet, BaseURL+"rest/api/2/user?username="+c.Username, nil)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		}
		log.Printf("JIRA response: %s", buf)
		return fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
	}
	return nil
}

func (c *Client) GetCurrentUsername() (string, error) {
	req, _ := http.NewRequest(http.MethodGet, BaseURL+"rest/auth/1/session", nil)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		}
		log.Printf("JIRA response: %s", buf)
		return "", fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
	}
	data := struct {
		Name string `json:"name"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", fmt.Errorf("Error reading JIRA response: %s", err)
	}
	return data.Name, nil
}

func (c *Client) getIssueStatus(id string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, BaseURL+"rest/api/2/issue/"+id, nil)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		}
		log.Printf("JIRA response: %s", buf)
		return "", fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
	}

	type issueResponse struct {
		Fields struct {
			Status struct {
				ID string `json:"id"`
			} `json:"status"`
		} `json:"fields"`
	}

	var respData *issueResponse
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("Error reading response from JIRA: %s", err)
	}
	return respData.Fields.Status.ID, nil
}

func (c *Client) GetIssueStatus(id string) (database.VPCRequestStatus, error) {
	statusString, err := c.getIssueStatus(id)
	if err != nil {
		return database.StatusUnknown, fmt.Errorf("Unable to get issue status for %s", id)
	}
	status, ok := c.Config.Statuses[statusString]
	if !ok {
		return database.StatusUnknown, fmt.Errorf("Unkown status id %s", statusString)
	}
	return status, nil
}

func (c *Client) SetIssueStatus(issueID string, status database.VPCRequestStatus) error {
	// Get status ID
	var statusID string
	for id, s := range c.Config.Statuses {
		if s == status {
			statusID = id
			break
		}
	}
	if statusID == "" {
		return fmt.Errorf("No JIRA status ID for status %q", status)
	}
	return c.setIssueStatus(issueID, statusID)
}

func (c *Client) GetDNSTLSIssueStatus(id string) (database.DNSTLSRequestStatus, error) {
	statusString, err := c.getIssueStatus(id)
	if err != nil {
		return database.DNSTLSStatusUnknown, fmt.Errorf("Unable to get issue status for %s", id)
	}
	status, ok := c.Config.DNSTLSStatuses[statusString]
	if !ok {
		return database.DNSTLSStatusUnknown, fmt.Errorf("Unkown status id %s", statusString)
	}
	return status, nil
}

func (c *Client) SetDNSTLSIssueStatus(issueID string, status database.DNSTLSRequestStatus) error {
	// Get status ID
	var statusID string
	for id, s := range c.Config.DNSTLSStatuses {
		if s == status {
			statusID = id
			break
		}
	}
	if statusID == "" {
		return fmt.Errorf("No JIRA status ID for status %q", status)
	}

	return c.setIssueStatus(issueID, statusID)
}

func (c *Client) PostComment(issueID, comment string) error {
	data := map[string]interface{}{
		"body": comment,
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("Error marshalling: %s", err)
	}
	req, _ := http.NewRequest(http.MethodPost, BaseURL+"rest/api/2/issue/"+issueID+"/comment", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("Error talking to JIRA: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading JIRA response: %s", err)
		}
		log.Printf("JIRA response: %s", buf)
		return fmt.Errorf("Unexpected status %s from JIRA", resp.Status)
	}
	return nil
}
