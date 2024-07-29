package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/jira"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

var mySession = session.Must(session.NewSession())

var ssmSvc = ssm.New(mySession)

var ssmParameterName = os.Getenv("SSM_PARAMETER_NAME")

func jiraOauthConfig() *jira.OauthConfig {
	out, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(ssmParameterName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		panic(fmt.Sprintf("Error getting parameter from SSM: %s\n", err))
	}
	jiraOauthConfig := jira.OauthConfig{}
	err = json.Unmarshal([]byte(aws.StringValue(out.Parameter.Value)), &jiraOauthConfig)
	if err != nil {
		panic(fmt.Sprintf("Error parsing JIRA oauth config: %s\n", err))
	}
	return &jiraOauthConfig
}

func main() {
	jiraUsername := os.Getenv("JIRA_USERNAME")
	if ssmParameterName == "" {
		fmt.Fprintf(os.Stderr, "SSM_PARAMETER_NAME is required\n")
		os.Exit(2)
	}
	if jiraUsername == "" {
		fmt.Fprintf(os.Stderr, "JIRA_USERNAME is required\n")
		os.Exit(2)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s", r.URL.Path)
	})
	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := jira.GenerateOauth1Config(jiraOauthConfig())
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to configure authentication: %s", err), http.StatusInternalServerError)
			return
		}
		cfg.CallbackURL = "http://" + r.Host + "/callback"
		requestToken, _, err := cfg.RequestToken()
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to get request token: %s", err), http.StatusInternalServerError)
			return
		}
		authorizationURL, err := cfg.AuthorizationURL(requestToken)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to get auth URL: %s", err), http.StatusInternalServerError)
			return
		}
		w.Header().Add("Cache-control", "private, no-store")
		http.Redirect(w, r, authorizationURL.String(), http.StatusFound)
	})
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		jiraConfig := jiraOauthConfig()
		cfg, err := jira.GenerateOauth1Config(jiraOauthConfig())
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to configure authentication: %s", err), http.StatusInternalServerError)
			return
		}
		requestToken := r.FormValue("oauth_token")
		verifier := r.FormValue("oauth_verifier")
		accessToken, _, err := cfg.AccessToken(requestToken, "", verifier)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to authorize: %s", err), http.StatusInternalServerError)
			return
		}
		jiraConfig.Token = accessToken
		jiraClient := jira.Client{}
		err = jiraClient.AddAuthentication(jiraConfig)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to configure authentication: %s", err), http.StatusInternalServerError)
			return
		}
		user, err := jiraClient.GetCurrentUsername()
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to verify access: %s", err), http.StatusInternalServerError)
			return
		}
		if user != jiraUsername {
			http.Error(w, fmt.Sprintf("You must authorize as %q but you authorized as %q", jiraUsername, user), http.StatusBadRequest)
			return
		}

		buf, err := json.Marshal(jiraConfig)
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to marshal config: %s", err), http.StatusInternalServerError)
			return
		}

		_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
			Name:      aws.String(ssmParameterName),
			Value:     aws.String(string(buf)),
			Type:      aws.String(ssm.ParameterTypeSecureString),
			Overwrite: aws.Bool(true),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to update parameter store: %s", err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Success!")
	})

	listenAddr := ":7878"
	log.Printf("Listen on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
