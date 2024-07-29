package orchestration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type NewVPCNotification struct {
	Region    string
	VPCID     string
	JIRAIssue string
}

type cidrsChangedMessage struct {
	Notification *NewVPCNotification
}

type Client struct {
	BaseURL string
	APIKey  string
}

func (api *Client) Health() error {
	buf := new(bytes.Buffer)
	//S.M.
	//url := fmt.Sprintf("%shealth", api.BaseURL)
	url := strings.Replace(fmt.Sprintf("%shealth", api.BaseURL), "api/v1/vpc-conf/", "", 1)

	req, err := http.NewRequest("GET", url, buf)
	if err != nil {
		return fmt.Errorf("Failed to create request for %q - %s", url, err)
	}

	return api.doRequest(req, nil)
}

func (api *Client) NotifyCIDRsChanged(accountID string, notification *NewVPCNotification) error {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(cidrsChangedMessage{Notification: notification})
	if err != nil {
		return fmt.Errorf("Error encoding notifiation details: %s", err)
	}

	url := fmt.Sprintf("%svpc-cidrs/%s", api.BaseURL, accountID)
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return fmt.Errorf("Failed to create request for %q - %s", url, err)
	}

	return api.doRequest(req, nil)
}

func (api *Client) doRequest(req *http.Request, jsonStruct interface{}) error {
	//S.M.
	//req.Header.Add("Authorization", "Bearer "+api.APIKey)
	req.Header.Add("X-API-Key", api.APIKey)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to fetch request for %q - %s", req.URL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("Status %s and error reading details: %s", resp.Status, err)
		}
		return fmt.Errorf("ERROR: response for %q - %s - %s", req.URL, resp.Status, buf)
	}

	if jsonStruct != nil {
		err = json.NewDecoder(resp.Body).Decode(&jsonStruct)
		if err != nil {
			return fmt.Errorf("Failed to decode response for %q - %s", req.RequestURI, err)
		}
	}

	return nil
}
