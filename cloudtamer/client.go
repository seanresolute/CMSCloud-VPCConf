package cloudtamer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type HTTPClient struct {
	Token string
}

// Do adds the necessary headers and attempts the request, with retries and
// exponential backoff, and then deserializes the result.
func (c *HTTPClient) Do(req *http.Request, result interface{}) error {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	canRetry := true
	var err error
	for tries := 0; canRetry && tries < 5; tries++ {
		time.Sleep(time.Second * (1<<tries - 1))
		canRetry, err = c.doOnce(req, result)
	}

	return err
}

func (c *HTTPClient) doOnce(req *http.Request, result interface{}) (canRetry bool, err error) {
	timedClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := timedClient.Do(req)
	if err != nil {
		return true, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		canRetry := resp.StatusCode >= 500 && resp.StatusCode < 600
		return canRetry, fmt.Errorf("Got status %d from CloudTamer", resp.StatusCode)
	}

	if result != nil {
		err := json.NewDecoder(resp.Body).Decode(result)
		if err != nil {
			return false, fmt.Errorf("Error reading CloudTamer response: %s", err)
		}
	}

	return false, nil
}
