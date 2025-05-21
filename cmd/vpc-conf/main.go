package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/apikey"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/azure"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cachedcredentials"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmsnet"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/jira"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/orchestration"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/session"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/static"
)

func main() {
	// Set environment variables from vpc-conf-dev.env
	os.Setenv("API_KEY_CONFIG", `[{"principal":"local-dev","keys":["50m3r4nd0m4P1k3Y"]}]`)
	os.Setenv("CMSNET_CONFIG", `null`)
	os.Setenv("IPAM_DEV_MODE", `0`)
	os.Setenv("POSTGRES_CONNECTION_STRING", `postgresql://postgres:QFO0EAlQq3050VpuKg@vpc-conf-dev.cbkdyavbhvkm.us-east-1.rds.amazonaws.com:5432/vpcconfdev`)
	os.Setenv("WORKER_NAME", `vpc-conf-dev:40`)
	os.Setenv("AZURE_AD_CLIENT_ID", `ca4aae61-e7cf-4f9f-9688-0f6e8f60d8a1`)
	os.Setenv("AZURE_AD_HOST", `https://login.microsoftonline.us`)
	os.Setenv("AZURE_AD_REDIRECT_URL", `https://dev.vpc-conf.actually-east.west.cms.gov/provision/oauth/callback`)
	os.Setenv("AZURE_AD_TENANT_ID", `7c8bb92a-832a-4d12-8e7e-c569d7b232c9`)
	os.Setenv("CLOUDTAMER_ADMIN_GROUP_ID", `901`)
	os.Setenv("CLOUDTAMER_BASE_URL", `https://cloudtamer.cms.gov/api`)
	os.Setenv("CLOUDTAMER_IDMS_ID", `2`)
	os.Setenv("CLOUDTAMER_READ_ONLY_GROUP_IDS", `1526`)
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID", `2`)
	os.Setenv("IPCONTROL_HOST", `internal-ipcontrol-network-ipam-438345226.us-east-1.elb.amazonaws.com:8443`)
	os.Setenv("IPCONTROL_USERNAME", `vpc-conf-prod`)
	os.Setenv("JIRA_ISSUE_LABELS", `null`)
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_PASSWORD", `aknCfdFg129ndI`)
	os.Setenv("CLOUDTAMER_SERVICE_ACCOUNT_USERNAME", `vpcconf-svc`)
	os.Setenv("CREDS_SVC_CONFIG", `{"Commercial":{"APIKey":"BEVFVpM8tzUxhKph","Host":"https://dev.creds-api.west.cms.gov","Role": "cms-cloud-admin/ct-cms-cloud-ia-operations"},"GovCloud":{"APIKey":"QVbzv3ER0CZt63y5","Host":"https://dev.creds-api-gc.west.cms.gov","Role":"cms-cloud-admin/ct-cms-cloud-ia-operations-gov"}}`)
	os.Setenv("IPCONTROL_PASSWORD", `bAZxObrxcbzSM5Lp6Q`)
	os.Setenv("JIRA_CONFIG", `{"Project": "IA","IssueType": "10007","Statuses": {"10102": "Submitted","3": "InProgress","10004": "CancelledByRequester","10103": "Approved","10016": "Done"},"Watchers": [],"Assignee": "LOVP"}`)
	os.Setenv("JIRA_OAUTH_CONFIG", `{"ConsumerKey":"CCG-PROD-HBNL-ACON-SetupD-12022019","PrivateKey":"-----BEGIN RSA PRIVATE KEY-----\nMIICXAIBAAKBgQCurKrQr+J27n36w7K3KHOI78HrvVA82/vXnJSwoMKbX/vie5Rg\nkpwjpw5IurL7g91MLPwT4qMTAet3SeMTCakfw2DFQtPpiIQHT6q2XjfwIaYnJck7\nmWtKqK/J8NQ3GwA82t6k9+5Xoiyfrd2NHCUCjiwzTjQuW62/xXKFRQKaSQIDAQAB\nAoGBAK2ghMqbioid2CwDiwn086MSb7hcnf1gzZ0sz8AijE7VwhMGtB6qnPnzfIde\nzbqlALxPmuJJTb//EIeqskSiPbDm9c3ppm/rjNIpnmrXgcvR7qqOWC986k/Trwxe\nKOxrlAEOq1p0Pdfh+o5sHw00Jgf7PiBUJW/aP/NAu7me+KPhAkEA50+FZMY4h24q\n9CeShDSnZEDHq+iaDjOiEexzchO+pSG9gHE2RKdGqxsu3IP3E8QwQ22iqWa/Yapp\ntzevyYm8ywJBAMFRlYG7+k9Y9WNmo7/utejVZoerAZkzRSB3Ugq5gznaLBhDtbBi\nVpngTSwXDtYosDVHluQYlg4qZFtyF7481rsCQHWwpz1kAaUer6o0bD7qD3VZ5H4a\nRjANo1udRAv58dlRNnsgny0FM1ah6RD38AHVo3zbTpUEm0GVFF7NbZqMg0sCQA9t\nYU7/H1ShtsN992djt2SjUxFUlkYRj1yt6QAuGcjOHmK5VJCE6IBTJBV2qZpxmM5H\nrkT5qU/sFiIuErL9y+0CQHZ+vB4mpEPQnZfwdIhakcK8e/MR9B4P6J9QKKeeUByv\nbBg8JvuBjcImNy2dDePIJDOaRQY7Xg4SYw+/ggtIu0M=\n-----END RSA PRIVATE KEY-----","Token":"P7ujAynN0RJyO006FJjtr7bYLFgjwptJ"}`)
	os.Setenv("JIRA_USERNAME", `west_infra_jira_ent`)
	os.Setenv("ORCHESTRATION_API_KEY", `0_11b7dea6_06903bb620ca14973a2bd05b565ff196fcbbc15fad8f502e38183db6ff679f1c`)
	os.Setenv("ORCHESTRATION_BASE_URL", `https://api.service-provisioning-dev.cloud.internal.cms.gov/api/v1/vpc-conf/`)

	devMode := os.Getenv("IPAM_DEV_MODE") == "1" // if true, make sure to change to ipam-web directory before running ipam-web

	postgresConnectionString := os.Getenv("POSTGRES_CONNECTION_STRING")
	db := sqlx.MustConnect("postgres", postgresConnectionString)
	err := database.Migrate(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error migrating database: %s\n", err)
		os.Exit(1)
	}

	cmsnetConfig := cmsnet.Config{}
	err = json.Unmarshal([]byte(os.Getenv("CMSNET_CONFIG")), &cmsnetConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing CMSNet config: %s\n", err)
		os.Exit(1)
	}

	jiraConfig := jira.Config{}
	err = json.Unmarshal([]byte(os.Getenv("JIRA_CONFIG")), &jiraConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JIRA config: %s\n", err)
		os.Exit(1)
	}
	if os.Getenv("JIRA_USERNAME") == "" || os.Getenv("JIRA_OAUTH_CONFIG") == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "JIRA_USERNAME and JIRA_OAUTH_CONFIG env variables are required")
		os.Exit(2)
	}

	jiraOauthConfig := &jira.OauthConfig{}
	err = json.Unmarshal([]byte(os.Getenv("JIRA_OAUTH_CONFIG")), &jiraOauthConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JIRA oauth config: %s\n", err)
		os.Exit(1)
	}

	apiKeyConfig, errs := apikey.GetConfigFromEnvJSON()
	if errs != nil {
		errs.(apikey.ConfigErrors).LogErrors()
		os.Exit(1)
	}

	jiraIssueLabels := IssueLabels{}
	err = json.Unmarshal([]byte(os.Getenv("JIRA_ISSUE_LABELS")), &jiraIssueLabels)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JIRA issue labels config: %s\n", err)
		os.Exit(1)
	}

	username := os.Getenv("IPCONTROL_USERNAME")
	password := os.Getenv("IPCONTROL_PASSWORD")
	ipcHost := os.Getenv("IPCONTROL_HOST")
	if username == "" || password == "" || ipcHost == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "IPCONTROL_HOST, IPCONTROL_USERNAME and IPCONTROL_PASSWORD env variables are required")
		os.Exit(2)
	}

	c := client.GetClient(ipcHost, username, password, 60*time.Second)
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting hostname: %s\n", err)
		os.Exit(1)
	}

	taskParallelismStr := os.Getenv("NUM_WORKERS")
	if taskParallelismStr == "" {
		taskParallelismStr = "10"
	}
	taskParallelism, err := strconv.Atoi(taskParallelismStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", "Invalid NUM_WORKERS")
		os.Exit(2)
	}

	workerName := os.Getenv("WORKER_NAME")
	if workerName == "" {
		metadata := struct {
			Family, Revision string
		}{}

		metadataURI := os.Getenv("ECS_CONTAINER_METADATA_URI")
		if metadataURI == "" {
			fmt.Fprintf(os.Stderr, "Unable to determine ECS metadata API. If this program is not running on ECS, set the WORKER_NAME env variable.\n")
			os.Exit(2)
		}
		resp, err := http.Get(metadataURI + "/task")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting ECS metadata: %s\n", err)
			os.Exit(2)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading ECS metadata: %s\n", err)
			os.Exit(2)
		}

		err = json.Unmarshal(body, &metadata)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing ECS metadata: %s\n", err)
			os.Exit(2)
		}
		workerName = fmt.Sprintf("%s:%s", metadata.Family, metadata.Revision)
	}
	if workerName == "" {
		fmt.Fprintf(os.Stderr, "Unable to determine worker name\n")
		os.Exit(2)
	} else {
		log.Printf("Worker name: %s", workerName)
	}
	taskDB := &database.TaskDatabase{
		DB:         db,
		WorkerID:   hostname,
		WorkerName: workerName,
	}
	log.Printf("Worker ID: %s", taskDB.WorkerID)

	jiraClient := &jira.Client{
		Config:   jiraConfig,
		Username: os.Getenv("JIRA_USERNAME"),
	}
	err = jiraClient.AddAuthentication(jiraOauthConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error configuring Jira authentication: %s\n", err)
		os.Exit(2)
	}

	azureADConfig, err := azure.GetConfigFromENV()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading Azure AD configuration: %s", err)
		os.Exit(2)
	}
	azureADConfig.StaticAssetsVersion = staticAssetsVersion
	azureAD := &azure.AzureAD{AzureADConfig: azureADConfig}

	sessionStore := &session.SQLSessionStore{DB: db}

	credentialsConfig, err := credentialservice.GetConfigFromENV()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading credentials configuration: %s", err)
		os.Exit(2)
	}
	credentialService := &credentialservice.CredentialService{Config: credentialsConfig}
	cachedCredentials := &cachedcredentials.CachedCredentials{
		CredentialsProvider: credentialService,
		SessionStore:        sessionStore,
	}

	server := &Server{
		APIKey:            &apikey.APIKey{Config: apiKeyConfig},
		AzureAD:           azureAD,
		CredentialService: credentialService,
		CachedCredentials: cachedCredentials,
		PathPrefix:        "/provision/",
		IPAM:              c,
		CMSNetConfig:      cmsnetConfig,
		TaskDatabase:      taskDB,
		SessionStore:      sessionStore,
		ModelsManager: &database.SQLModelsManager{
			DB: db,
		},
		JIRAClient:       jiraClient,
		JIRAIssueLabels:  jiraIssueLabels,
		ReparseTemplates: devMode,
		TaskParallelism:  taskParallelism,
	}

	onlyAccountIDs := strings.TrimSpace(os.Getenv("ONLY_AWS_ACCOUNT_IDS"))
	if onlyAccountIDs != "" {
		split := strings.Split(onlyAccountIDs, ",")
		log.Printf("Only allowing accounts: %s", split)
		server.LimitToAWSAccountIDs = split
	} else {
		log.Printf("Allowing all accounts to be managed")
	}

	orchestrationBaseURL := os.Getenv("ORCHESTRATION_BASE_URL")
	orchestrationAPIKey := os.Getenv("ORCHESTRATION_API_KEY")
	if orchestrationBaseURL != "" {
		server.Orchestration = &orchestration.Client{
			BaseURL: orchestrationBaseURL,
			APIKey:  orchestrationAPIKey,
		}
	}

	cmsnetClient := cmsnet.NewClient(server.CMSNetConfig, nil, server.CachedCredentials.CredentialsProvider)

	server.listenForNewTasks(postgresConnectionString)
	server.SyncVPCRequestStatuses()

	tasksDone := server.DoTasks()

	go func() {
		c := make(chan os.Signal, 3)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		interrupts := 0
		for s := range c {
			sig, ok := s.(syscall.Signal)
			if !ok {
				log.Printf("Non-UNIX signal %s", s)
				continue
			}
			if sig == syscall.SIGINT {
				interrupts += 1
				if interrupts == 1 {
					log.Printf("SIGINT received; waiting for any ongoing tasks to finish")
					go func() {
						server.StopTaskQueue()
						<-tasksDone
						os.Exit(3)
					}()
				} else {
					log.Printf("Second SIGINT received; marking any current tasks as failed before quitting")
					server.FailTasksNow()
					os.Exit(4)
				}
			} else if sig == syscall.SIGTERM {
				log.Printf("SIGTERM received; waiting for any ongoing tasks to finish")
				go func() {
					server.StopTaskQueue()
					select {
					case <-tasksDone:
						os.Exit(5)
					case <-time.After(90 * time.Second):
						log.Printf("Timeout exceeded; marking any current tasks as failed before quitting")
						server.FailTasksNow()
						os.Exit(6)
					}
				}()
			}
		}
	}()

	go func() {
		for {
			err := server.SessionStore.DeleteOldSessions(55)
			if err != nil {
				log.Printf("Error deleting old sessions: %s", err)
			}
			time.Sleep(1 * time.Minute)
		}
	}()

	http.Handle(server.PathPrefix, server)
	http.Handle("/static/", static.FileServer(_escFS(devMode)))
	healthMu := new(sync.RWMutex)
	type Health struct {
		CanConnect struct {
			Postgres    bool
			Credentials struct {
				Commercial bool
				GovCloud   bool
			}
			IPControl     bool
			JIRA          bool
			Groot         bool
			Orchestration bool
		}
		UpdateAwsAccountsSynced bool
		TaskStats               *database.TaskStats
		JiraIssueErrors         *database.JIRAHealth
	}
	var currentHealth Health
	healthInterval := 30 * time.Second
	go func() {
		for range time.Tick(healthInterval) {
			// CredentialsSerivce
			health := credentialService.GetServiceHealth()
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.CanConnect.Credentials.Commercial = health.Commercial
				currentHealth.CanConnect.Credentials.GovCloud = health.GovCloud
			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// IPControl
			_, err := c.ListBlocks("/root/AWS/", false, false)
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.CanConnect.IPControl = false
				if err != nil {
					log.Printf("Health Check Error talking to IPControl: %s", err)
				} else {
					currentHealth.CanConnect.IPControl = true
				}
			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// JIRA
			err = jiraClient.VerifyAccess()
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.CanConnect.JIRA = false
				if err != nil {
					log.Printf("Health Check Error verifying JIRA access: %s", err)
				} else {
					currentHealth.CanConnect.JIRA = true
				}

			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// Groot (CMS Net API) Get connection requests on VPC Automation Prod account
			_, err := cmsnetClient.GetAllConnectionRequests("546085968493", "us-east-1", "vpc-0e8a39fbb69113f27", "VPCConfHealthCheck")
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.CanConnect.Groot = false
				if err != nil {
					log.Printf("Health Check Error verifying Groot connectivity: %s", err)
				} else {
					currentHealth.CanConnect.Groot = true
				}

			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// Status of update-aws-accounts micro service
			upToDate, err := server.ModelsManager.GetAWSAccountsLastSyncedinInterval(2)
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.UpdateAwsAccountsSynced = false
				if err != nil {
					log.Printf("Health Check Error verifying Update AWS Accounts last success: %s", err)
				} else if !upToDate {
					log.Printf("Update AWS Accounts microservice has not run successfully in the allotted time")
				} else {
					currentHealth.UpdateAwsAccountsSynced = true
				}
			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// Orchestration
			err = server.Orchestration.Health()
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.CanConnect.Orchestration = false
				if err != nil {
					log.Printf("Health Check Error verifying Orchestration access: %s", err)
				} else {
					currentHealth.CanConnect.Orchestration = true
				}

			}()
		}
	}()
	go func() {
		for range time.Tick(healthInterval) {
			// Jira Issue Create Problems
			jiraHealth, err := server.ModelsManager.GetJIRAHealth()
			func() {
				healthMu.Lock()
				defer healthMu.Unlock()
				currentHealth.JiraIssueErrors = jiraHealth
				if err != nil {
					log.Printf("Error getting Jira issue health: %s", err)
				}
			}()
		}
	}()
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthMu.RLock()
		health := currentHealth
		healthMu.RUnlock()
		health.CanConnect.Postgres = false
		taskStats, err := taskDB.GetTaskStats()
		taskStats.MaxWorkers = taskParallelism
		health.TaskStats = taskStats
		if err != nil {
			log.Printf("Error getting task stats: %s", err)
		} else {
			health.CanConnect.Postgres = true
		}

		err = json.NewEncoder(w).Encode(health)
		if err != nil {
			log.Printf("Error marshaling health response: %s", err)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, server.PathPrefix, http.StatusFound)
		} else {
			http.NotFound(w, r)
		}
	})

	mainListenAddr := ":2020"
	log.Printf("Interactive login: listen on %s", mainListenAddr)
	mainHTTPServer := &http.Server{
		Addr: mainListenAddr,
		BaseContext: func(net.Listener) context.Context {
			return context.WithValue(context.Background(), authContextKey, useCredentialAuth)
		},
	}
	log.Fatal(mainHTTPServer.ListenAndServe())
}
