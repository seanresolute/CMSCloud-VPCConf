package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cirello.io/dynamolock"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/deploy"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config file")
	app := flag.String("app", "", "app to check")

	flag.Usage = func() {
		fmt.Printf("Usage: %s -config </path/to/config> -app <app>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}

	flag.Parse()

	if *configPath == "" || *app == "" {
		flag.Usage()
	}

	// Set up credentials
	cloudTamerBaseURL := os.Getenv("CLOUDTAMER_BASE_URL")
	cloudTamerAdminGroupIDStr := os.Getenv("CLOUDTAMER_ADMIN_GROUP_ID")
	if cloudTamerBaseURL == "" || cloudTamerAdminGroupIDStr == "" {
		log.Fatalf("CLOUDTAMER_BASE_URL and CLOUDTAMER_ADMIN_GROUP_ID env variables are required")
	}
	cloudTamerAdminGroupID, err := strconv.Atoi(cloudTamerAdminGroupIDStr)
	if err != nil {
		log.Fatalf("Invalid CLOUDTAMER_ADMIN_GROUP_ID")
	}
	cloudTamerServiceAccountUsername := os.Getenv("CLOUDTAMER_USERNAME")
	cloudTamerServiceAccountPassword := os.Getenv("CLOUDTAMER_PASSWORD")
	cloudTamerServiceAccountIDMSIDStr := os.Getenv("CLOUDTAMER_IDMS_ID")
	if cloudTamerServiceAccountUsername == "" || cloudTamerServiceAccountPassword == "" || cloudTamerServiceAccountIDMSIDStr == "" {
		log.Fatalf("CLOUDTAMER_USERNAME, CLOUDTAMER_PASSWORD, and CLOUDTAMER_IDMS_ID env variables are required")
	}
	cloudTamerServiceAccountIDMSID, err := strconv.Atoi(cloudTamerServiceAccountIDMSIDStr)
	if err != nil {
		log.Fatalf("Invalid CLOUDTAMER_IDMS_ID")
	}
	tokenProvider := &cloudtamer.TokenProvider{
		Config: cloudtamer.CloudTamerConfig{
			BaseURL:      cloudTamerBaseURL,
			IDMSID:       cloudTamerServiceAccountIDMSID,
			AdminGroupID: cloudTamerAdminGroupID,
		},
		Username: cloudTamerServiceAccountUsername,
		Password: cloudTamerServiceAccountPassword,
	}
	log.Printf("Logging in to CloudTamer")
	token, err := tokenProvider.GetToken()
	if err != nil {
		log.Fatalf("Error getting CloudTamer token: %s", err)
	}
	creds := &awscreds.CloudTamerAWSCreds{
		Token:   token,
		BaseURL: cloudTamerBaseURL,
	}
	absConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		log.Fatalf("Error determining config file path: %s", err)
	}
	configBytes, err := ioutil.ReadFile(absConfigPath)
	if err != nil {
		log.Fatalf("Error reading config file %q: %s", absConfigPath, err)
	}
	config := &deploy.DeployConfig{}
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		log.Fatalf("Error parsing config: %s", err)
	}

	log.Printf("Getting app status")
	status, err := GetAppStatus(creds, config, *app)
	if err != nil {
		log.Fatalf("Error getting %q stats: %s", *app, err)
	}

	// Show environments in a consistent order.
	envs := []string{}
	for env := range status.Environments {
		envs = append(envs, env)
	}
	sort.Strings(envs)

	fmt.Printf("%s:\n", *app)
	fmt.Printf("  Latest: %s\n", status.LatestAvailable.Value)
	for _, env := range envs {
		envStatus := status.Environments[env]
		plural := "s"
		if envStatus.ServingTaskSet.NumTasksRunning == 1 {
			plural = ""
		}
		fmt.Printf("  %s:\n", env)
		fmt.Printf("    Serving:   %s/%s (taskdef=%s, status=%s, %d task%s running)\n", envStatus.ServingTaskSet.Artifact.Value, envStatus.ServingTaskSet.ConfigSHA, envStatus.ServingTaskSet.TaskDefinition, envStatus.ServingTaskSet.Status, envStatus.ServingTaskSet.NumTasksRunning, plural)
		if envStatus.CurrentDeploy.Done {
			fmt.Printf("    No deploy in progress\n")
		} else {
			fmt.Printf("    Deploying: %s/%s (step %d)\n", envStatus.CurrentDeploy.Artifact.Value, envStatus.CurrentDeploy.ConfigSHA, envStatus.CurrentDeploy.Step)
		}
		if envStatus.SwappedTaskSet != nil {
			plural := "s"
			if envStatus.SwappedTaskSet.NumTasksRunning == 1 {
				plural = ""
			}
			fmt.Printf("    Swapped:   %s/%s (taskdef=%s, status=%s, %d task%s running)\n", envStatus.SwappedTaskSet.Artifact.Value, envStatus.SwappedTaskSet.ConfigSHA, envStatus.SwappedTaskSet.TaskDefinition, envStatus.SwappedTaskSet.Status, envStatus.SwappedTaskSet.NumTasksRunning, plural)
		}
	}
}

func GetAppStatus(creds *awscreds.CloudTamerAWSCreds, config *deploy.DeployConfig, app string) (*AppStatus, error) {
	configCreds, err := creds.GetCredentialsForAccount(config.ConfigAccountID)
	if err != nil {
		return nil, fmt.Errorf("Error getting AWS credentials for account %s: %s", config.ConfigAccountID, err)
	}
	configCredsValue, err := configCreds.Get()
	if err != nil {
		return nil, fmt.Errorf("Error getting AWS credentials for account %s: %s", config.ConfigAccountID, err)
	}
	artifactDBEnv := []string{
		"AWS_ACCESS_KEY_ID=" + configCredsValue.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + configCredsValue.SecretAccessKey,
		"AWS_SESSION_TOKEN=" + configCredsValue.SessionToken,
	}
	configSess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(string(config.ConfigRegion)),
		Credentials: configCreds,
	}))
	dynamodbsvc := dynamodb.New(configSess)

	appConfig, appOK := config.Apps[app]
	if !appOK {
		return nil, fmt.Errorf("Unrecognized app %q", app)
	}

	appStatus := &AppStatus{
		Environments: make(map[string]EnvStatus),
	}

	type result struct {
		env    string
		status *EnvStatus
		error  error
	}
	results := make(chan result)

	for env, envConfig := range appConfig.Environments {
		go func(env string, envConfig deploy.EnvironmentConfig) {
			status, err := (func() (*EnvStatus, error) {
				envStatus := &EnvStatus{}
				lockClient, err := dynamolock.New(dynamodbsvc,
					deploy.LockTableName,
				)
				if err != nil {
					return nil, fmt.Errorf("Error setting up lock client: %s", err)
				}
				lock, err := lockClient.Get(deploy.LockKey(app, env))
				if err != nil {
					return nil, fmt.Errorf("Error looking up deploy lock: %s", err)
				}
				status := &deploy.DeployStatus{Done: true}
				lockData := lock.Data()
				if len(lockData) > 0 {
					err := json.Unmarshal(lockData, status)
					if err != nil {
						return nil, fmt.Errorf("Unable to parse deploy lock status info: %s", err)
					}
				}
				envStatus.CurrentDeploy = *status

				awscreds, err := creds.GetCredentialsForAccount(envConfig.AccountID)
				if err != nil {
					return nil, fmt.Errorf("Error getting AWS credentials for account %s: %s", envConfig.AccountID, err)
				}
				sess := session.Must(session.NewSession(&aws.Config{
					Region:      aws.String(string(envConfig.Region)),
					Credentials: awscreds,
				}))
				ecssvc := ecs.New(sess)
				elbv2svc := elbv2.New(sess)

				albState, err := deploy.GetALBState(elbv2svc, &envConfig)
				if err != nil {
					return nil, fmt.Errorf("Error getting ALB state: %s", err)
				}
				servingColor := deploy.Blue
				if albState.GreenPriority < albState.BluePriority {
					servingColor = deploy.Green
				}

				envStatus.ServingColor = servingColor
				servingTargetGroup := envConfig.TargetGroupARNs[servingColor]
				otherTargetGroup := envConfig.TargetGroupARNs[servingColor.Other()]

				serviceName := envConfig.Family
				serviceOut, err := ecssvc.DescribeServices(&ecs.DescribeServicesInput{
					Cluster:  &envConfig.Cluster,
					Services: []*string{&serviceName},
				})
				if err != nil {
					return nil, fmt.Errorf("Error describing service: %s", err)
				}
				if len(serviceOut.Services) != 1 {
					return nil, fmt.Errorf("Got %d services", len(serviceOut.Services))
				}
				for _, taskSet := range serviceOut.Services[0].TaskSets {
					if len(taskSet.LoadBalancers) == 0 {
						continue
					}
					taskStatus := aws.StringValue(taskSet.Status)
					taskSetID := aws.StringValue(taskSet.Id)
					taskSetInfo := TaskSet{
						TaskSetID:       taskSetID,
						NumTasksRunning: aws.Int64Value(taskSet.RunningCount),
						Status:          taskStatus,
					}
					taskDefinitionPieces := strings.Split(aws.StringValue(taskSet.TaskDefinition), "/")
					if len(taskDefinitionPieces) > 0 {
						taskSetInfo.TaskDefinition = taskDefinitionPieces[len(taskDefinitionPieces)-1]
					}
					tagsOut, err := ecssvc.ListTagsForResource(&ecs.ListTagsForResourceInput{
						ResourceArn: taskSet.TaskSetArn,
					})
					if err != nil {
						return nil, fmt.Errorf("Error getting task set tags: %s", err)
					}
					for _, tag := range tagsOut.Tags {
						if aws.StringValue(tag.Key) == "config-sha" {
							taskSetInfo.ConfigSHA = aws.StringValue(tag.Value)
						} else if aws.StringValue(tag.Key) == "artifact-build-number" {
							args := []string{"--region", config.ConfigRegion, "get", "-j", "-p", appConfig.ArtifactDBProjectName, "-b", aws.StringValue(tag.Value)}
							cmd := exec.Command("artifact-db", args...)
							cmd.Env = artifactDBEnv
							artifactDBOut, err := cmd.CombinedOutput()
							if err != nil {
								return nil, fmt.Errorf("Error getting serving artifact from ArtifactDB: %s\nOutput: %s\n", err, artifactDBOut)
							}
							err = json.Unmarshal(artifactDBOut, &taskSetInfo.Artifact)
							if err != nil {
								return nil, fmt.Errorf("Error umarshaling artifact-db output for serving artifact: %s", err)
							}
						}
					}
					if aws.StringValue(taskSet.LoadBalancers[0].TargetGroupArn) == servingTargetGroup {
						envStatus.ServingTaskSet = taskSetInfo
					} else if aws.StringValue(taskSet.LoadBalancers[0].TargetGroupArn) == otherTargetGroup {
						envStatus.SwappedTaskSet = &taskSetInfo
					}
				}
				return envStatus, nil
			})()
			results <- result{
				env:    env,
				status: status,
				error:  err,
			}
		}(env, envConfig)
	}

	args := []string{"--region", config.ConfigRegion, "latest", "-p", appConfig.ArtifactDBProjectName, "-j", "-t"}
	filterTags := map[string]string{"ForAutomatedDeploy": "true"}
	filterTagsBytes, err := json.Marshal(filterTags)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling tags for filtering artifacts: %s", err)
	}
	args = append(args, string(filterTagsBytes))
	cmd := exec.Command("artifact-db", args...)
	cmd.Env = artifactDBEnv
	artifactDBOut, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("Error talking to ArtifactDB for latest artifact: %s\nOutput: %s\n", err, artifactDBOut)
	}
	if len(bytes.TrimSpace(artifactDBOut)) == 0 {
		return nil, fmt.Errorf("Empty response from artifact-db for latest artifact; probably no artifact matched the filter specified")
	}

	err = json.Unmarshal(artifactDBOut, &appStatus.LatestAvailable)
	if err != nil {
		return nil, fmt.Errorf("Error umarshaling artifact-db output for latest artifact: %s", err)
	}
	errs := []error{}
	for range appConfig.Environments {
		result := <-results
		if result.error != nil {
			errs = append(errs, result.error)
		} else {
			appStatus.Environments[result.env] = *result.status
		}
	}

	if len(errs) > 0 {
		combinedError := "Errors encountered: "
		for idx, err := range errs {
			if idx > 0 {
				combinedError += "; "
			}
			combinedError += err.Error()
		}
		return nil, errors.New(combinedError)
	}

	return appStatus, nil
}

type TaskSet struct {
	TaskSetID       string
	TaskDefinition  string
	Artifact        deploy.Artifact
	ConfigSHA       string
	NumTasksRunning int64
	Status          string
}

type EnvStatus struct {
	CurrentDeploy  deploy.DeployStatus
	ServingColor   deploy.Color
	ServingTaskSet TaskSet
	SwappedTaskSet *TaskSet
}

type AppStatus struct {
	Environments    map[string]EnvStatus
	LatestAvailable deploy.Artifact
}
