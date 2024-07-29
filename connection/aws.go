package connection

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs/cloudwatchlogsiface"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
)

type cancellableEC2 struct {
	t   *NetworkConnectionTest
	EC2 ec2iface.EC2API
}

type cancellableECS struct {
	t   *NetworkConnectionTest
	ECS ecsiface.ECSAPI
}

type cancellableIAM struct {
	t   *NetworkConnectionTest
	IAM iamiface.IAMAPI
}

type cancellableDynamoDB struct {
	t        *NetworkConnectionTest
	DynamoDB dynamodbiface.DynamoDBAPI
}

type cancellableCloudWatchLogs struct {
	t              *NetworkConnectionTest
	CloudWatchLogs cloudwatchlogsiface.CloudWatchLogsAPI
}

func newCancellableEC2(t *NetworkConnectionTest, p client.ConfigProvider, cfgs ...*aws.Config) *cancellableEC2 {
	return &cancellableEC2{t: t, EC2: ec2.New(p, cfgs...)}
}

func newCancellableECS(t *NetworkConnectionTest, p client.ConfigProvider, cfgs ...*aws.Config) *cancellableECS {
	return &cancellableECS{t: t, ECS: ecs.New(p, cfgs...)}
}

func newCancellableIAM(t *NetworkConnectionTest, p client.ConfigProvider, cfgs ...*aws.Config) *cancellableIAM {
	return &cancellableIAM{t: t, IAM: iam.New(p, cfgs...)}
}

func newCancellableDynamoDB(t *NetworkConnectionTest, p client.ConfigProvider, cfgs ...*aws.Config) *cancellableDynamoDB {
	return &cancellableDynamoDB{t: t, DynamoDB: dynamodb.New(p, cfgs...)}
}

func newCancellableCloudWatchLogs(t *NetworkConnectionTest, p client.ConfigProvider, cfgs ...*aws.Config) *cancellableCloudWatchLogs {
	return &cancellableCloudWatchLogs{t: t, CloudWatchLogs: cloudwatchlogs.New(p, cfgs...)}
}

func (e *cancellableEC2) DescribeNetworkInterfaces(in *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeNetworkInterfaces(in)
}

func (e *cancellableEC2) DescribeSubnets(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeSubnets(in)
}

func (e *cancellableEC2) DescribeNatGateways(in *ec2.DescribeNatGatewaysInput) (*ec2.DescribeNatGatewaysOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeNatGateways(in)
}

func (e *cancellableEC2) GetManagedPrefixListEntries(in *ec2.GetManagedPrefixListEntriesInput) (*ec2.GetManagedPrefixListEntriesOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.GetManagedPrefixListEntries(in)
}

func (e *cancellableEC2) DescribeVpcPeeringConnections(in *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeVpcPeeringConnections(in)
}

func (e *cancellableEC2) RevokeSecurityGroupEgress(in *ec2.RevokeSecurityGroupEgressInput) (*ec2.RevokeSecurityGroupEgressOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.RevokeSecurityGroupEgress(in)
}

func (e *cancellableEC2) CreateSecurityGroup(in *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.CreateSecurityGroup(in)
}

func (e *cancellableEC2) AuthorizeSecurityGroupEgress(in *ec2.AuthorizeSecurityGroupEgressInput) (*ec2.AuthorizeSecurityGroupEgressOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.AuthorizeSecurityGroupEgress(in)
}

func (e *cancellableEC2) DescribeRouteTables(in *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeRouteTables(in)
}

func (e *cancellableEC2) DescribeSecurityGroups(in *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.EC2.DescribeSecurityGroups(in)
}

func (e *cancellableECS) DescribeClusters(in *ecs.DescribeClustersInput) (*ecs.DescribeClustersOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.ECS.DescribeClusters(in)
}

func (e *cancellableECS) CreateCluster(in *ecs.CreateClusterInput) (*ecs.CreateClusterOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.ECS.CreateCluster(in)
}

func (e *cancellableECS) RegisterTaskDefinition(in *ecs.RegisterTaskDefinitionInput) (*ecs.RegisterTaskDefinitionOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.ECS.RegisterTaskDefinition(in)
}

func (e *cancellableECS) RunTask(in *ecs.RunTaskInput) (*ecs.RunTaskOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.ECS.RunTask(in)
}

func (e *cancellableECS) DescribeTasks(in *ecs.DescribeTasksInput) (*ecs.DescribeTasksOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.ECS.DescribeTasks(in)
}

func (e *cancellableIAM) CreateRole(in *iam.CreateRoleInput) (*iam.CreateRoleOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.IAM.CreateRole(in)
}

func (e *cancellableIAM) PutRolePolicy(in *iam.PutRolePolicyInput) (*iam.PutRolePolicyOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.IAM.PutRolePolicy(in)
}

func (e *cancellableIAM) AttachRolePolicy(in *iam.AttachRolePolicyInput) (*iam.AttachRolePolicyOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.IAM.AttachRolePolicy(in)
}

func (e *cancellableDynamoDB) CreateTable(in *dynamodb.CreateTableInput) (*dynamodb.CreateTableOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.DynamoDB.CreateTable(in)
}

func (e *cancellableDynamoDB) DescribeTable(in *dynamodb.DescribeTableInput) (*dynamodb.DescribeTableOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.DynamoDB.DescribeTable(in)
}

func (e *cancellableDynamoDB) UpdateItem(in *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.DynamoDB.UpdateItem(in)
}

func (e *cancellableDynamoDB) GetItem(in *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.DynamoDB.GetItem(in)
}

func (e *cancellableCloudWatchLogs) CreateLogGroup(in *cloudwatchlogs.CreateLogGroupInput) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.CloudWatchLogs.CreateLogGroup(in)
}

func (e *cancellableCloudWatchLogs) PutRetentionPolicy(in *cloudwatchlogs.PutRetentionPolicyInput) (*cloudwatchlogs.PutRetentionPolicyOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.CloudWatchLogs.PutRetentionPolicy(in)
}

func (e *cancellableCloudWatchLogs) DescribeLogGroups(in *cloudwatchlogs.DescribeLogGroupsInput) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	if err := e.t.Context.Err(); err != nil {
		return nil, fmt.Errorf("Test aborted: %w", err)
	}
	return e.CloudWatchLogs.DescribeLogGroups(in)
}
