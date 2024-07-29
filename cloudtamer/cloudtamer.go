package cloudtamer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LoginRequest represents a login request.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IDMS     int   `json:"idms"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    struct {
		ID      int    `json:"id"`
		Message string `json:"message"`
		UserID  int    `json:"user_id"`
		Access  struct {
			Token  string    `json:"token"`
			Expiry string `json:"expiry"`

		} `json:"access"`
		Refresh struct {
			Token  string    `json:"token"`
			Expiry string `json:"expiry"`
		} `json:"refresh"`
	} `json:"data"`
}

type GroupResponse struct {
	Data struct {
		Users []struct {
			ID int `json:"id"`
		} `json:"users"`
	} `json:"data"`
}

type UserResponse struct {
	Data struct {
		User struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"user"`
	} `json:"data"`
}

var ErrInvalidUsernameOrPassword = errors.New("Invalid username or password")

type CloudTamerConfig struct {
	BaseURL          string
	IDMSID           int
	AdminGroupID     int
	ReadOnlyGroupIDs []int
}

type TokenProvider struct {
	Config             CloudTamerConfig
	Username, Password string
	token              *string
	tokenMu            sync.Mutex
}

const approverEmailSuffixCMS = "@hhs.cms.gov"
const CloudTamerTokenNotApplicable = "n/a"

func (p *TokenProvider) GetToken() (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.token == nil {
		var err error
		_, token, err := LogIn(p.Config, p.Username, p.Password)
		if err != nil {
			return "", err
		}
		p.token = &token
		go func() {
			time.Sleep(1 * time.Hour)
			p.tokenMu.Lock()
			defer p.tokenMu.Unlock()
			p.token = nil
		}()
	}
	return *p.token, nil
}

func IsApprover(config CloudTamerConfig, userID int, token string) (bool, error) {
	userInfo, err := GetUserInfo(config.BaseURL, userID, token)

	if err != nil {
		log.Printf("Unable to fetch user info for %d", userID)
		return false, fmt.Errorf("Unable to fetch user info for %q", userID)
	}

	if strings.HasSuffix(strings.ToLower(userInfo.Email), approverEmailSuffixCMS) {
		return true, nil
	}
	return InGroup(config.BaseURL, userID, config.AdminGroupID, token)
}

func InGroup(baseURL string, userID int, groupID int, token string) (bool, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v3/user-group/%d", baseURL, groupID), nil)
	if err != nil {
		return false, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	gs := new(GroupResponse)
	err = (&HTTPClient{Token: token}).Do(req, gs)
	if err != nil {
		return false, fmt.Errorf("Error getting group info from CloudTamer: %s", err)
	}
	for _, user := range gs.Data.Users {
		if user.ID == userID {
			return true, nil
		}
	}
	return false, nil
}

func LogIn(config CloudTamerConfig, username, password string) (userID int, token string, err error) {
	b, err := json.Marshal(LoginRequest{
		Username: username,
		Password: password,
		IDMS: config.IDMSID,
	})
	if err != nil {
		return -1, "", err
	}

	req, err := http.NewRequest("POST", config.BaseURL+"/v3/token", bytes.NewBuffer(b))
	if err != nil {
		return -1, "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return -1, "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return -1, "", err
	}

	rs := new(LoginResponse)
	err = json.Unmarshal(body, rs)
	if resp.StatusCode != http.StatusOK {
		if err == nil && rs.Data.Message == "Invalid username or password." {
			return -1, "", ErrInvalidUsernameOrPassword
		}
		return -1, "", fmt.Errorf("%s", body)
	}
	if err != nil {
		return -1, "", fmt.Errorf("Error reading CloudTamer response: %s", err)
	}

	return rs.Data.UserID, rs.Data.Access.Token, nil
}

type UserInfo struct {
	Username, Name, Email string
}

func GetUserInfo(baseURL string, userID int, token string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/v3/user/%d", baseURL, userID), nil)
	if err != nil {
		return nil, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	us := new(UserResponse)
	err = (&HTTPClient{Token: token}).Do(req, us)
	if err != nil {
		return nil, fmt.Errorf("Error getting user info from CloudTamer: %s", err)
	}
	return &UserInfo{
		Username: us.Data.User.Username,
		Name:     us.Data.User.DisplayName,
		Email:    us.Data.User.Email,
	}, nil
}
