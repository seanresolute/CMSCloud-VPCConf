package deploy

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

// An Artifact is stored in ArtifactDB and represents a build that can
// be deployed.
type Artifact struct {
	BuildNumber int64
	Value       string
	Tags        map[string]string
}

// A DeployStatus is stored with the deploy lock to show what is being
// deployed and how far along in the process we have made it.
type DeployStatus struct {
	Artifact       Artifact
	ConfigSHA      string
	NewTaskSetID   string
	NewActiveColor Color
	Done           bool
	Step           int
}

func (s *DeployStatus) Bytes() []byte {
	buf, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return buf
}

type Color string

const (
	Blue  Color = "blue"
	Green Color = "green"
)

func (c Color) Other() Color {
	if c == Blue {
		return Green
	} else if c == Green {
		return Blue
	} else {
		return Color("???")
	}
}

const LockTableName = "DeployLock"

func LockKey(app, env string) string {
	return fmt.Sprintf("%s-%s", app, env)
}

// A DeployConfig specifies the configuration for a group of
// apps with shared ArtifactDB and Lock tables.
type DeployConfig struct {
	// Account ID for ArtifactDB and Lock tables
	ConfigAccountID string
	ConfigRegion    string
	Apps            map[string]AppConfig
}

type AppConfig struct {
	ArtifactDBProjectName string
	Environments          map[string]EnvironmentConfig
	// string values will be passed through text/template with the following
	// top-level values available:
	//  - Environment
	//  - Region
	//  - Account
	//  - Artifact
	//    - BuildNumber
	//    - Value
	//    - Tags
	ContainerDefinitionTemplates []*ecs.ContainerDefinition
	// PreDeployCommandTemplates provides templates for commands to
	// run before the deploy starts. They will have the same
	// variables available as ContainerDefinitionTemplates.
	PreDeployCommandTemplates []*Command
	// PostDeployCommandTemplates provides templates for commands to run
	// after a successful deploy or a rollback from an unsuccessful
	// deploy. They will additionally have "TaskDefinition"
	// provided, which will be the task definition of the currently
	// active (after the deploy or rollback) task set.
	PostDeployCommandTemplates []*Command
}

type EnvironmentConfig struct {
	AccountID string
	Region    string

	RequiresPreviousEnvDeploy string

	Cluster          string
	Family           string
	CPU, Memory      string
	TaskRoleArn      string
	ExecutionRoleArn string
	SecurityGroups   []string
	Subnets          []string
	ListenerARN      string
	TargetGroupARNs  map[Color]string
}

type Command struct {
	Program   string
	Arguments []string
}

// ALBState represents the rule and priroity corresponding to each Color.
type ALBState struct {
	GreenPriority, BluePriority int64
	GreenRule, BlueRule         *elbv2.Rule
}

// GetALBState determines the ALBState from AWS. The rules corresponding to each
// Color will be determined by looking at the target group specified for that color
// in envConfig and finding the rule forwarding to that target group.
// TODO: support multiple rules for each color, e.g. by grouping
// into RulePairs, where each RulePair has the same fields as
// this struct below but represents a pair of rules with the
// same Condition.
func GetALBState(elbv2svc elbv2iface.ELBV2API, envConfig *EnvironmentConfig) (*ALBState, error) {
	albState := &ALBState{
		GreenPriority: -1,
		BluePriority:  -1,
	}
	rulesOut, err := elbv2svc.DescribeRules(&elbv2.DescribeRulesInput{
		ListenerArn: &envConfig.ListenerARN,
	})
	if err != nil {
		return nil, fmt.Errorf("Error describing ALB rules: %s", err)
	}
	for _, rule := range rulesOut.Rules {
		if len(rule.Actions) < 1 {
			continue
		}
		action := rule.Actions[len(rule.Actions)-1]
		if aws.StringValue(action.Type) == elbv2.ActionTypeEnumForward {
			priorityStr := aws.StringValue(rule.Priority)
			if priorityStr == "default" {
				continue
			}
			priority, err := strconv.ParseInt(priorityStr, 10, 64)
			if err != nil {
				log.Printf("Error parsing priority %q: %s", priorityStr, err)
			}
			if aws.StringValue(action.TargetGroupArn) == envConfig.TargetGroupARNs[Blue] {
				if albState.BlueRule != nil {
					return nil, fmt.Errorf("Found multiple rules for blue target group: %q and %q", aws.StringValue(albState.BlueRule.Actions[len(albState.BlueRule.Actions)-1].TargetGroupArn), aws.StringValue(rule.Actions[len(rule.Actions)-1].TargetGroupArn))
				}
				albState.BlueRule = rule
				albState.BluePriority = priority
			} else if aws.StringValue(action.TargetGroupArn) == envConfig.TargetGroupARNs[Green] {
				if albState.GreenRule != nil {
					return nil, fmt.Errorf("Found multiple rules for green target group: %q and %q", aws.StringValue(albState.GreenRule.Actions[len(albState.GreenRule.Actions)-1].TargetGroupArn), aws.StringValue(rule.Actions[len(rule.Actions)-1].TargetGroupArn))
				}
				albState.GreenRule = rule
				albState.GreenPriority = priority
			}
		}
	}
	if albState.GreenPriority == -1 {
		return nil, fmt.Errorf("Unable to determine green priority")
	}
	if albState.BluePriority == -1 {
		return nil, fmt.Errorf("Unable to determine blue priority")
	}
	return albState, nil
}
