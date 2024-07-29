package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cirello.io/dynamolock"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/deploy"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/waiter"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func instantiateTemplate(target interface{}, data map[string]interface{}) error {
	tplStr, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("Error remarshaling template: %s", err)
	}
	tpl, err := template.New("defs").Parse(string(tplStr))
	if err != nil {
		return fmt.Errorf("Error parsing template: %s", err)
	}
	buf := new(bytes.Buffer)
	err = tpl.Execute(buf, data)
	if err != nil {
		return fmt.Errorf("Error executing template: %s", err)
	}
	err = json.Unmarshal(buf.Bytes(), &target)
	if err != nil {
		return fmt.Errorf("Error reparsing template after executing: %s", err)
	}
	return nil
}

var statusWaiter = &waiter.Waiter{
	SleepDuration:  time.Second,
	StatusInterval: 10 * time.Second,
	Timeout:        5 * time.Minute,
}

func wrapOutput(r io.Reader, programName string) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		stdoutReader := bufio.NewReader(r)
		for {
			line, err := stdoutReader.ReadBytes('\n')
			if err == nil { // line ends in \n
				fmt.Printf("[%s] %s", programName, line)
			}
			if err != nil {
				if len(line) > 0 { // partial output before error; line does not end in \n
					fmt.Printf("[%s] %s\n", programName, line)
				}
				if err != io.EOF {
					log.Printf("Error reading from %s: %s", programName, err)
				}
				return
			}
		}
	}()
	return done
}

func wireUpIO(cmd *exec.Cmd) (chan struct{}, error) {
	cmd.Stdin = os.Stdin
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	name := cmd.Path
	if len(cmd.Args) > 0 {
		name = cmd.Args[0]
	}
	stdoutDone := wrapOutput(stdoutPipe, name)
	stderrDone := wrapOutput(stderrPipe, name)
	done := make(chan struct{})
	go func() {
		<-stdoutDone
		<-stderrDone
		close(done)
	}()
	return done, nil
}

func runCommand(command *deploy.Command) error {
	description := command.Program
	if len(command.Arguments) > 0 {
		description += " " + strings.Join(command.Arguments, " ")
	}
	log.Printf("Running %q...", description)
	cmd := exec.Command(command.Program, command.Arguments...)
	done, err := wireUpIO(cmd)
	if err != nil {
		return fmt.Errorf("Error running %q: %s", description, err)
	}
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("Error running %q: %s", description, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("Error: still waiting for %q to complete after 5 minutes", description)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("Error running %q: %s", description, err)
	}
	return nil
}

func deployTag(env, configSHA string) string {
	return fmt.Sprintf("deploy-%s-%s", env, configSHA)
}

const (
	deployFailed    = "failure"
	deploySucceeded = "success"
)

type deployOptions struct {
	App       string
	Env       string
	DeploySHA string
	ConfigSHA string

	DoResume    bool // resume a previous failed deploy
	DoStartOver bool // start over after a previous failed deploy

	DoSkipPrePostCommands bool
}

func monitorAndFinish(ecssvc *ecs.ECS, config *deploy.DeployConfig, envConfig *deploy.EnvironmentConfig, appConfig *deploy.AppConfig, options *deployOptions, lockClient *dynamolock.Client, lock *dynamolock.Lock, status *deploy.DeployStatus, artifactDBEnv []string) error {
	serviceName := envConfig.Family

	// 10. Update the new task set to be PRIMARY.
	status.Step = 10
	err := lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	log.Printf("Marking new task set %s as PRIMARY", status.NewTaskSetID)
	_, err = ecssvc.UpdateServicePrimaryTaskSet(&ecs.UpdateServicePrimaryTaskSetInput{
		Cluster:        &envConfig.Cluster,
		Service:        &serviceName,
		PrimaryTaskSet: &status.NewTaskSetID,
	})
	if err != nil {
		return fmt.Errorf("Error marking new task set as PRIMARY: %s", err)
	}

	// 11. Send requests through ALB and monitor stats
	status.Step = 11
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	// TODO: implement this
	// TODO: if this fails, need to swap PRIMARY, swap ALB, restart task queue, and release lock

	// 12. Restart task queue
	if options.DoSkipPrePostCommands {
		log.Printf("Skipping post commands")
	} else {
		status.Step = 12
		err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
		if err != nil {
			return fmt.Errorf("Error updating lock: %s", err)
		}
		out, err := ecssvc.DescribeTaskSets(&ecs.DescribeTaskSetsInput{
			Cluster:  &envConfig.Cluster,
			Service:  &serviceName,
			TaskSets: []*string{&status.NewTaskSetID},
		})
		if err != nil {
			return fmt.Errorf("Error looking up new task set: %s", err)
		}
		if len(out.TaskSets) != 1 {
			return fmt.Errorf("Got %d task sets for id %s", len(out.TaskSets), status.NewTaskSetID)
		}
		// Docs don't say if the task definition is specified as family:version or ARN. In practice
		// it seems to be ARN. This code works with either one.
		taskDefinitionPieces := strings.Split(aws.StringValue(out.TaskSets[0].TaskDefinition), "/")
		if len(taskDefinitionPieces) == 0 {
			return fmt.Errorf("Invalid task definition %q", aws.StringValue(out.TaskSets[0].TaskDefinition))
		}
		taskDefinition := taskDefinitionPieces[len(taskDefinitionPieces)-1]
		templateData := map[string]interface{}{
			"Environment":    options.Env,
			"Region":         envConfig.Region,
			"Account":        envConfig.AccountID,
			"Artifact":       status.Artifact,
			"TaskDefinition": taskDefinition,
		}
		err = instantiateTemplate(&appConfig.PostDeployCommandTemplates, templateData)
		if err != nil {
			return fmt.Errorf("Error instantiating post-deploy templates: %s", err)
		}
		for _, cmd := range appConfig.PostDeployCommandTemplates {
			err := runCommand(cmd)
			if err != nil {
				return fmt.Errorf("Error running post-deploy command: %s", err)
			}
		}
	}

	// 13. Mark the deploy as successful
	status.Step = 13
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	cmd := exec.Command("artifact-db", "--region", config.ConfigRegion, "-p", appConfig.ArtifactDBProjectName, "-b", fmt.Sprintf("%d", status.Artifact.BuildNumber), "tag", "set", "-k", deployTag(options.Env, options.ConfigSHA), "-v", deploySucceeded)
	cmd.Env = artifactDBEnv
	artifactDBOut, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error talking to ArtifactDB: %s\nOutput: %s\n", err, artifactDBOut)
	}

	status.Done = true
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}

	return nil
}

func deployAndSwap(elbv2svc *elbv2.ELBV2, ecssvc *ecs.ECS, config *deploy.DeployConfig, envConfig *deploy.EnvironmentConfig, appConfig *deploy.AppConfig, options *deployOptions, lockClient *dynamolock.Client, lock *dynamolock.Lock, status *deploy.DeployStatus) error {
	// 2. Stop task queue
	if options.DoSkipPrePostCommands {
		log.Printf("Skipping pre commands")
	} else {
		status.Step = 2
		err := lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
		if err != nil {
			return fmt.Errorf("Error updating lock: %s", err)
		}
		for _, cmd := range appConfig.PreDeployCommandTemplates {
			err := runCommand(cmd)
			if err != nil {
				return fmt.Errorf("Error running pre-deploy command: %s", err)
			}
		}
	}

	// 3. Read active color from ALB
	status.Step = 3
	err := lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	var oldActiveColor, newActiveColor deploy.Color
	var oldActiveRule *elbv2.Rule

	albState, err := deploy.GetALBState(elbv2svc, envConfig)
	if err != nil {
		return fmt.Errorf("Error getting ALB state: %s", err)
	}

	if albState.GreenPriority < albState.BluePriority {
		oldActiveColor = deploy.Green
		newActiveColor = deploy.Blue
		oldActiveRule = albState.GreenRule
	} else {
		oldActiveColor = deploy.Blue
		newActiveColor = deploy.Green
		oldActiveRule = albState.BlueRule
	}
	log.Printf("Active color is %s", oldActiveColor)
	status.NewActiveColor = newActiveColor
	servingTargetGroupARN := aws.StringValue(oldActiveRule.Actions[len(oldActiveRule.Actions)-1].TargetGroupArn)

	// 4. Stop/delete non-primary task sets
	status.Step = 4
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	serviceName := envConfig.Family
	serviceOut, err := ecssvc.DescribeServices(&ecs.DescribeServicesInput{
		Cluster:  &envConfig.Cluster,
		Services: []*string{&serviceName},
	})
	if err != nil {
		return fmt.Errorf("Error describing task sets: %s", err)
	}
	if len(serviceOut.Services) != 1 {
		return fmt.Errorf("Got %d services", len(serviceOut.Services))
	}
	desiredCount := serviceOut.Services[0].DesiredCount
	activeIDs := []string{}
	for _, taskSet := range serviceOut.Services[0].TaskSets {
		taskStatus := aws.StringValue(taskSet.Status)
		taskSetID := aws.StringValue(taskSet.Id)
		if taskStatus == "ACTIVE" {
			if len(taskSet.LoadBalancers) > 0 && aws.StringValue(taskSet.LoadBalancers[0].TargetGroupArn) == servingTargetGroupARN {
				log.Printf("WARNING: task set %s is not PRIMARY but is associated with the serving target group. We will continue with the deploy but we will not stop this task set.", taskSetID)
			} else {
				activeIDs = append(activeIDs, taskSetID)
			}
		} else if taskStatus == "DRAINING" {
			return fmt.Errorf("Task set %s is still draining", taskSetID)
		}
	}

	if len(activeIDs) > 0 {
		for _, id := range activeIDs {
			log.Printf("Stopping old task set %s", id)
			_, err := ecssvc.UpdateTaskSet(&ecs.UpdateTaskSetInput{
				Cluster: &envConfig.Cluster,
				Service: &serviceName,
				TaskSet: &id,
				Scale: &ecs.Scale{
					Unit:  aws.String(ecs.ScaleUnitPercent),
					Value: aws.Float64(0),
				},
			})
			if err != nil {
				return fmt.Errorf("Error stopping task set: %s", err)
			}
		}
		log.Printf("Waiting for stopped task set to stabilize...")
		err = statusWaiter.Wait(func() waiter.Result {
			out, err := ecssvc.DescribeTaskSets(&ecs.DescribeTaskSetsInput{
				Cluster:  &envConfig.Cluster,
				Service:  &serviceName,
				TaskSets: aws.StringSlice(activeIDs),
			})
			if err != nil {
				return waiter.Error(fmt.Errorf("Error describing task sets: %s", err))
			}
			for _, taskSet := range out.TaskSets {
				status := aws.StringValue(taskSet.StabilityStatus)
				if status != ecs.StabilityStatusSteadyState {
					waiter.Continue(fmt.Sprintf("Task set is still %s with %d task(s) running", status, aws.Int64Value(taskSet.RunningCount)))
				}
			}
			return waiter.Done()
		})
		if err != nil {
			return fmt.Errorf("Error waiting for task set to stabilize: %s", err)
		}
		for _, id := range activeIDs {
			log.Printf("Deleting task set %s", id)
			_, err := ecssvc.DeleteTaskSet(&ecs.DeleteTaskSetInput{
				Cluster: &envConfig.Cluster,
				Service: &serviceName,
				TaskSet: &id,
				Force:   aws.Bool(true),
			})
			if err != nil {
				return fmt.Errorf("Error deleting task set: %s", err)
			}
		}
		// There is a maximum number of task sets; this makes sure that the just-deleted
		// task sets are not counted towards the maximum.
		log.Printf("Waiting for task set deletion to be effective...")
		err = statusWaiter.Wait(func() waiter.Result {
			out, err := ecssvc.DescribeTaskSets(&ecs.DescribeTaskSetsInput{
				Cluster:  &envConfig.Cluster,
				Service:  &serviceName,
				TaskSets: aws.StringSlice(activeIDs),
			})
			if err != nil {
				return waiter.Error(fmt.Errorf("Error describing task sets: %s", err))
			}
			if len(out.TaskSets) > 0 {
				return waiter.Continue(fmt.Sprintf("Still %d task set(s) remaining", len(out.TaskSets)))
			}
			return waiter.Done()
		})
		if err != nil {
			return fmt.Errorf("Error waiting for task set deletion to be effective: %s", err)
		}
	}

	// 5. Create new task definition from template
	status.Step = 5
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	taskTags := []*ecs.Tag{
		{
			Key:   aws.String("config-sha"),
			Value: aws.String(options.ConfigSHA),
		},
		{
			Key:   aws.String("artifact-build-number"),
			Value: aws.String(fmt.Sprintf("%d", status.Artifact.BuildNumber)),
		},
	}
	registerOut, err := ecssvc.RegisterTaskDefinition(&ecs.RegisterTaskDefinitionInput{
		ContainerDefinitions:    appConfig.ContainerDefinitionTemplates,
		Memory:                  &envConfig.Memory,
		Cpu:                     &envConfig.CPU,
		TaskRoleArn:             &envConfig.TaskRoleArn,
		ExecutionRoleArn:        &envConfig.ExecutionRoleArn,
		Family:                  &envConfig.Family,
		RequiresCompatibilities: []*string{aws.String("FARGATE")},
		NetworkMode:             aws.String("awsvpc"),
		Tags:                    taskTags,
	})
	if err != nil {
		return fmt.Errorf("Error registering new task definition: %s", err)
	}
	newTaskDefinition := fmt.Sprintf("%s:%d", aws.StringValue(registerOut.TaskDefinition.Family), aws.Int64Value(registerOut.TaskDefinition.Revision))
	log.Printf("Registered %s", newTaskDefinition)

	// 6. Create a new task set
	status.Step = 6
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	taskSetOut, err := ecssvc.CreateTaskSet(&ecs.CreateTaskSetInput{
		Cluster:        &envConfig.Cluster,
		Service:        &serviceName,
		TaskDefinition: &newTaskDefinition,
		NetworkConfiguration: &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				SecurityGroups: aws.StringSlice(envConfig.SecurityGroups),
				Subnets:        aws.StringSlice(envConfig.Subnets),
			},
		},
		LaunchType: aws.String(ecs.LaunchTypeFargate),
		Scale: &ecs.Scale{
			Unit:  aws.String(ecs.ScaleUnitPercent),
			Value: aws.Float64(100),
		},
		LoadBalancers: []*ecs.LoadBalancer{
			{
				ContainerName:  appConfig.ContainerDefinitionTemplates[0].Name,
				ContainerPort:  appConfig.ContainerDefinitionTemplates[0].PortMappings[0].ContainerPort,
				TargetGroupArn: aws.String(envConfig.TargetGroupARNs[newActiveColor]),
			},
		},
		Tags: taskTags,
	})
	if err != nil {
		return fmt.Errorf("Error creating new task set: %s", err)
	}
	if taskSetOut.TaskSet == nil {
		return fmt.Errorf("Invalid response from CreateTaskSet: no TaskSet")
	}
	log.Printf("Created task set %s", aws.StringValue(taskSetOut.TaskSet.Id))
	status.NewTaskSetID = aws.StringValue(taskSetOut.TaskSet.Id)

	// 7. Wait for rollout to complete
	status.Step = 7
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	log.Printf("Waiting for new task set to stabilize...")
	desired := aws.Int64Value(desiredCount)
	err = statusWaiter.Wait(func() waiter.Result {
		out, err := ecssvc.DescribeTaskSets(&ecs.DescribeTaskSetsInput{
			Cluster:  &envConfig.Cluster,
			Service:  &serviceName,
			TaskSets: []*string{taskSetOut.TaskSet.Id},
		})
		if err != nil {
			return waiter.Error(fmt.Errorf("Error describing task set: %s", err))
		}
		if len(out.TaskSets) != 1 {
			return waiter.Error(fmt.Errorf("Error: got %d task sets", len(out.TaskSets)))
		}
		taskSet := out.TaskSets[0]
		running := aws.Int64Value(taskSet.RunningCount)
		status := aws.StringValue(taskSet.StabilityStatus)
		if status != ecs.StabilityStatusSteadyState {
			return waiter.Continue(fmt.Sprintf("Task set is still %s with %d task(s) running out of %d desired", status, running, desired))
		}
		return waiter.Done()
	})
	if err != nil {
		return fmt.Errorf("Error waiting for new task set to stabilize: %s", err)
	}

	// 8. Do some requests against the new tasks.
	status.Step = 8
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	// TODO: implement this
	// TODO: if this fails, need to restart task queue and release lock

	// 9. Update the ALB to make the new tasks the target for requests.
	status.Step = 9
	err = lockClient.SendHeartbeat(lock, dynamolock.ReplaceHeartbeatData(status.Bytes()))
	if err != nil {
		return fmt.Errorf("Error updating lock: %s", err)
	}
	log.Printf("Swapping active color to %s", newActiveColor)
	_, err = elbv2svc.SetRulePriorities(&elbv2.SetRulePrioritiesInput{
		RulePriorities: []*elbv2.RulePriorityPair{
			{
				RuleArn:  albState.BlueRule.RuleArn,
				Priority: aws.Int64(albState.GreenPriority),
			},
			{
				RuleArn:  albState.GreenRule.RuleArn,
				Priority: aws.Int64(albState.BluePriority),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("Error swapping active color: %s", err)
	}

	return nil
}

func deployApp(creds *awscreds.CloudTamerAWSCreds, config *deploy.DeployConfig, options *deployOptions) error {
	configCreds, err := creds.GetCredentialsForAccount(config.ConfigAccountID)
	if err != nil {
		return fmt.Errorf("Error AWS credentials for account %s: %s", config.ConfigAccountID, err)
	}
	configCredsValue, err := configCreds.Get()
	if err != nil {
		return fmt.Errorf("Error AWS credentials for account %s: %s", config.ConfigAccountID, err)
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

	appConfig, appOK := config.Apps[options.App]
	if !appOK {
		return fmt.Errorf("Unrecognized app %q", options.App)
	}

	args := []string{"--region", config.ConfigRegion, "latest", "-p", appConfig.ArtifactDBProjectName, "-j", "-t"}
	filterTags := map[string]string{"SHA": options.DeploySHA}
	filterTagsBytes, err := json.Marshal(filterTags)
	if err != nil {
		return fmt.Errorf("Error marshalling tags for filtering artifacts: %s", err)
	}
	args = append(args, string(filterTagsBytes))
	cmd := exec.Command("artifact-db", args...)
	cmd.Env = artifactDBEnv
	artifactDBOut, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error talking to ArtifactDB: %s\nOutput: %s\n", err, artifactDBOut)
	}
	if len(bytes.TrimSpace(artifactDBOut)) == 0 {
		return fmt.Errorf("Empty response from artifact-db; probably no artifact matched the filter specified")
	}

	artifact := new(deploy.Artifact)
	err = json.Unmarshal(artifactDBOut, artifact)
	if err != nil {
		return fmt.Errorf("Error umarshaling ArtifactDB output: %s", err)
	}

	envConfig, envOK := appConfig.Environments[options.Env]
	if !envOK {
		return fmt.Errorf("Unrecognized env %q", options.Env)
	}

	if envConfig.RequiresPreviousEnvDeploy != "" {
		prevResult := artifact.Tags[deployTag(envConfig.RequiresPreviousEnvDeploy, options.ConfigSHA)]
		if prevResult != deploySucceeded {
			return fmt.Errorf("Must deploy to %s before %s", envConfig.RequiresPreviousEnvDeploy, options.Env)
		}
	}

	log.Printf("Will deploy %q", artifact.Value)

	templateData := map[string]interface{}{
		"Environment": options.Env,
		"Region":      envConfig.Region,
		"Account":     envConfig.AccountID,
		"Artifact":    artifact,
	}
	err = instantiateTemplate(&appConfig.ContainerDefinitionTemplates, templateData)
	if err != nil {
		return fmt.Errorf("Error instantiating container definition templates: %s", err)
	}
	if len(appConfig.ContainerDefinitionTemplates) == 0 {
		return fmt.Errorf("No containers in container definition templates")
	}
	if len(appConfig.ContainerDefinitionTemplates[0].PortMappings) != 1 {
		return fmt.Errorf("First container definition template must contain exactly one port mapping, to be targeted by the ALB")
	}
	err = instantiateTemplate(&appConfig.PreDeployCommandTemplates, templateData)
	if err != nil {
		return fmt.Errorf("Error instantiating pre-deploy templates: %s", err)
	}

	awscreds, err := creds.GetCredentialsForAccount(envConfig.AccountID)
	if err != nil {
		return fmt.Errorf("Error AWS credentials for account %s: %s", envConfig.AccountID, err)
	}
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(string(envConfig.Region)),
		Credentials: awscreds,
	}))
	ecssvc := ecs.New(sess)
	elbv2svc := elbv2.New(sess)

	// 1. Acquire lock
	leaseDuration := 10 * time.Second
	lockClient, err := dynamolock.New(dynamodbsvc,
		deploy.LockTableName,
		dynamolock.WithLeaseDuration(leaseDuration),
		dynamolock.WithHeartbeatPeriod(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("Error setting up lock client: %s", err)
	}

	opts := []dynamolock.AcquireLockOption{
		dynamolock.WithAdditionalTimeToWaitForLock(leaseDuration),
	}
	log.Printf("Acquiring deploy lock...")
	if options.DoResume || options.DoStartOver {
		log.Printf("This may take up to %s", 2*leaseDuration)
	} else {
		opts = append(opts, dynamolock.FailIfLocked())
	}
	lock, err := lockClient.AcquireLock(deploy.LockKey(options.App, options.Env), opts...)
	if err != nil {
		if _, ok := err.(*dynamolock.LockNotGrantedError); ok {
			if options.DoResume || options.DoStartOver {
				return fmt.Errorf("Failed to acquire the lock because there is another deploy ongoing")
			}
			return fmt.Errorf("A previous deploy has the lock and you did not ask to resume or start over")
		}
		return fmt.Errorf("Error getting lock: %s", err)
	}
	log.Printf("Got deploy lock")

	lockData := lock.Data()
	status := &deploy.DeployStatus{Step: 1, Artifact: *artifact, ConfigSHA: options.ConfigSHA}
	if options.DoResume {
		err := json.Unmarshal(lockData, status)
		if err != nil {
			return fmt.Errorf("Unable to parse status of previous run: %s", err)
		}
		if status.Done {
			lock.Close()
			return fmt.Errorf("Previous deploy succeeded so it cannot be resumed")
		}
		if status.Artifact.BuildNumber != artifact.BuildNumber || status.Artifact.Value != artifact.Value {
			return fmt.Errorf("Artifact previously being deployed (%v) does not match current artifact (%v)", status.Artifact, *artifact)
		}
		if status.ConfigSHA != options.ConfigSHA {
			return fmt.Errorf("Config SHA previously being deployed (%q) does not match current config SHA (%q)", status.ConfigSHA, options.ConfigSHA)
		}
		log.Printf("Previous run was on step %d", status.Step)
	}

	shouldStartFromBeginning := true
	if options.DoResume {
		if status.Step >= 10 {
			shouldStartFromBeginning = false
		} else if status.Step == 9 {
			// If we were at step 9 (swap ALB) then we don't know if we are serving the
			// old or new task set. Find out by checking if the active color matches
			// status.NewActiveColor.
			albState, err := deploy.GetALBState(elbv2svc, &envConfig)
			if err != nil {
				return fmt.Errorf("Error getting ALB state: %s", err)
			}
			var activeColor deploy.Color
			if albState.GreenPriority < albState.BluePriority {
				activeColor = deploy.Green
			} else {
				activeColor = deploy.Blue
			}
			// Only continue if the ALB has already been swapped.
			shouldStartFromBeginning = activeColor != status.NewActiveColor
		}
	}

	if shouldStartFromBeginning {
		if options.DoResume {
			log.Printf("The ALB has not been swapped yet so deploy will start over from the beginning")
		}

		err = deployAndSwap(elbv2svc, ecssvc, config, &envConfig, &appConfig, options, lockClient, lock, status)
		if err != nil {
			return err
		}
	}

	err = monitorAndFinish(ecssvc, config, &envConfig, &appConfig, options, lockClient, lock, status, artifactDBEnv)

	if err != nil {
		return err
	}

	return lock.Close()
}

func main() {
	configPath := flag.String("config", "", "path to JSON config file")
	app := flag.String("app", "", "app to deploy")
	env := flag.String("env", "", "env to deploy to")
	deploySHA := flag.String("sha", "", "deploy the artifact tagged SHA=<sha>")
	configSHAOverride := flag.String("config-sha", "", "manual config SHA to use instead of determining automatically")
	doResume := flag.Bool("resume", false, "resume a previous deploy")
	doStartOver := flag.Bool("start-over", false, "start a previous deploy over")
	doSkipPrePostCommands := flag.Bool("skip-pre-post", false, "skip the pre- and post-deploy commands")

	flag.Usage = func() {
		fmt.Printf("Usage: %s -config </path/to/config> -env <env> -app <app> -sha <SHA>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}

	flag.Parse()

	if *configPath == "" || *app == "" || *env == "" || *deploySHA == "" {
		flag.Usage()
	}

	if *doResume && *doStartOver {
		log.Fatalf("Cannot specify both -resume and -start-over")
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

	configSHA := *configSHAOverride
	if configSHA == "" {
		log.Printf("Attempting to determine config SHA")
		cmd := exec.Command("git", "rev-parse", "--short=8", "HEAD")
		configDir := path.Dir(absConfigPath)
		cmd.Dir = configDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Error running git. If the config file is not in git repo, specify -config-sha. Error: %s. Output:\n%s", err, out)
		}
		configSHA = strings.TrimSpace(string(out))
		// Also check to see if the config file has been modified (in which case
		// the commit SHA does not reflect its current contents)
		cmd = exec.Command("git", "diff", "--exit-code", "--shortstat", absConfigPath)
		cmd.Dir = configDir
		out, err = cmd.CombinedOutput()
		if err != nil {
			log.Fatalf("Config file is modified (or 'git diff' failed). Error: %s. Output:\n%s", err, out)
		}
		log.Printf("config SHA: %s", configSHA)
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
	err = deployApp(creds, config, &deployOptions{
		App:                   *app,
		Env:                   *env,
		ConfigSHA:             configSHA,
		DeploySHA:             *deploySHA,
		DoResume:              *doResume,
		DoStartOver:           *doStartOver,
		DoSkipPrePostCommands: *doSkipPrePostCommands,
	})
	if err != nil {
		log.Fatalf("%s", err)
	}
	log.Printf("Success!")
}
