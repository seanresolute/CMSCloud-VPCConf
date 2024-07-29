package main

import (
	"math/rand"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
)

type addZonedSubnetsTestCase struct {
	Name string

	VPCName                    string
	Stack                      string
	StartState                 database.VPCState
	ExistingContainers         testmocks.ContainerTree
	ExistingPeeringConnections []*ec2.VpcPeeringConnection
	ExistingVPCCIDRBlocks      []*ec2.VpcCidrBlockAssociation // only used when adding unroutable

	TaskConfig database.AddZonedSubnetsTaskData

	ExpectedContainersAdded []testmocks.ContainerSpec
	ExpectedBlocksAdded     []testmocks.BlockSpec
	ExpectedCloudIDsUpdated map[string]string

	ExpectedVPCCIDRBlocksAssociated []string
	ExpectedSubnetsAdded            []*ec2.Subnet

	ExpectedTaskStatus database.TaskStatus

	ExpectedEndState database.VPCState
}

func TestPerformAddZonedSubnets(t *testing.T) {
	testCases := []addZonedSubnetsTestCase{
		{
			Name:  "Add app subnets",
			Stack: "dev",

			VPCName: "chris-east-dev",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},

			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East/Lower-App",
			},

			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-east-1",
				SubnetType: database.SubnetTypeApp,
				SubnetSize: 27,
				GroupName:  "foo",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersAdded: []testmocks.ContainerSpec{
				{
					ParentName:   "/Global/AWS/V4/Commercial/East/Lower-App",
					Name:         "123456-chris-east-dev",
					BlockType:    client.BlockTypeVPC,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Name:         "foo-a",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Name:         "foo-b",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Name:         "foo-c",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
			},
			ExpectedBlocksAdded: []testmocks.BlockSpec{
				{
					ParentContainer: "/Global/AWS/V4/Commercial/East/Lower-App",
					Container:       "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					BlockType:       client.BlockTypeVPC,
					Size:            25,
					Status:          "Aggregate",
				},
				{
					ParentContainer: "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Container:       "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-a",
					BlockType:       client.BlockTypeSubnet,
					Size:            27,
					Status:          "Deployed",
				},
				{
					ParentContainer: "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Container:       "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-b",
					BlockType:       client.BlockTypeSubnet,
					Size:            27,
					Status:          "Deployed",
				},
				{
					ParentContainer: "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev",
					Container:       "/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-c",
					BlockType:       client.BlockTypeSubnet,
					Size:            27,
					Status:          "Deployed",
				},
			},
			ExpectedCloudIDsUpdated: map[string]string{
				"/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev":       "vpc-abc",
				"/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-a": "subnet-us-east-1a-0",
				"/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-b": "subnet-us-east-1b-0",
				"/Global/AWS/V4/Commercial/East/Lower-App/123456-chris-east-dev/foo-c": "subnet-us-east-1c-0",
			},

			ExpectedVPCCIDRBlocksAssociated: []string{"10.0.0.0/25"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-east-1a"),
					CidrBlock:        aws.String("10.1.0.0/27"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1b"),
					CidrBlock:        aws.String("10.2.0.0/27"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1b-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-b"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1c"),
					CidrBlock:        aws.String("10.3.0.0/27"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1c-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-c"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-us-east-1a-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-us-east-1b-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-us-east-1c-0",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "Add app subnets - some already exist",

			VPCName: "chris-gov-west-test",
			Stack:   "test",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1a",
									GroupName: "foo",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1b",
									GroupName: "foo",
								},
							},
						},
					},
					"us-gov-west-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1c",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/GovCloud/West/",
				Children: []testmocks.ContainerTree{
					{
						Name: "/Global/AWS/V4/GovCloud/West/Lower-App",
						Children: []testmocks.ContainerTree{{
							Name: "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test"},
						},
					},
				},
			},

			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-gov-west-1",
				SubnetType: database.SubnetTypeApp,
				SubnetSize: 28,
				GroupName:  "bar",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersAdded: []testmocks.ContainerSpec{
				{
					ParentName:   "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Name:         "bar-a",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Name:         "bar-b",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Name:         "bar-c",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
			},
			ExpectedBlocksAdded: []testmocks.BlockSpec{
				{
					ParentContainer: "/Global/AWS/V4/GovCloud/West/Lower-App",
					Container:       "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					BlockType:       client.BlockTypeVPC,
					Size:            26,
					Status:          "Aggregate",
				},
				{
					ParentContainer: "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Container:       "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-a",
					BlockType:       client.BlockTypeSubnet,
					Size:            28,
					Status:          "Deployed",
				},
				{
					ParentContainer: "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Container:       "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-b",
					BlockType:       client.BlockTypeSubnet,
					Size:            28,
					Status:          "Deployed",
				},
				{
					ParentContainer: "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test",
					Container:       "/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-c",
					BlockType:       client.BlockTypeSubnet,
					Size:            28,
					Status:          "Deployed",
				},
			},
			ExpectedCloudIDsUpdated: map[string]string{
				"/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-a": "subnet-us-gov-west-1a-0",
				"/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-b": "subnet-us-gov-west-1b-0",
				"/Global/AWS/V4/GovCloud/West/Lower-App/123456-chris-gov-west-test/bar-c": "subnet-us-gov-west-1c-0",
			},

			ExpectedVPCCIDRBlocksAssociated: []string{"10.0.0.0/26"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-gov-west-1a"),
					CidrBlock:        aws.String("10.1.0.0/28"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-gov-west-1a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-gov-west-test-bar-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("bar"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("test"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-gov-west-1b"),
					CidrBlock:        aws.String("10.2.0.0/28"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-gov-west-1b-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-gov-west-test-bar-b"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("bar"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("test"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-gov-west-1c"),
					CidrBlock:        aws.String("10.3.0.0/28"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-gov-west-1c-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-gov-west-test-bar-c"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("bar"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("app"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("test"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1a",
									GroupName: "foo",
								},
								{
									SubnetID:  "subnet-us-gov-west-1a-0",
									GroupName: "bar",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1b",
									GroupName: "foo",
								},
								{
									SubnetID:  "subnet-us-gov-west-1b-0",
									GroupName: "bar",
								},
							},
						},
					},
					"us-gov-west-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1c",
									GroupName: "foo",
								},
								{
									SubnetID:  "subnet-us-gov-west-1c-0",
									GroupName: "bar",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "Add data subnets - app subnets already exist",

			VPCName: "chris-west-prod",
			Stack:   "prod",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-west-2a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1a",
									GroupName: "foo",
								},
							},
						},
					},
					"us-west-2c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1c",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/West/",
				Children: []testmocks.ContainerTree{
					{
						Name: "/Global/AWS/V4/Commercial/West/Lower-App",
						Children: []testmocks.ContainerTree{{
							Name: "/Global/AWS/V4/Commercial/West/Lower-App/123456-chris-west-prod"},
						},
					},
					{
						Name: "/Global/AWS/V4/Commercial/West/Prod-Data",
					},
				},
			},

			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-west-2",
				SubnetType: database.SubnetTypeData,
				SubnetSize: 28,
				GroupName:  "bar",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersAdded: []testmocks.ContainerSpec{
				{
					ParentName:   "/Global/AWS/V4/Commercial/West/Prod-Data",
					Name:         "123456-chris-west-prod",
					BlockType:    client.BlockTypeVPC,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod",
					Name:         "bar-a",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
				{
					ParentName:   "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod",
					Name:         "bar-c",
					BlockType:    client.BlockTypeSubnet,
					AWSAccountID: "123456",
				},
			},
			ExpectedBlocksAdded: []testmocks.BlockSpec{
				{
					ParentContainer: "/Global/AWS/V4/Commercial/West/Prod-Data",
					Container:       "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod",
					BlockType:       client.BlockTypeVPC,
					Size:            27,
					Status:          "Aggregate",
				},
				{
					ParentContainer: "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod",
					Container:       "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod/bar-a",
					BlockType:       client.BlockTypeSubnet,
					Size:            28,
					Status:          "Deployed",
				},
				{
					ParentContainer: "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod",
					Container:       "/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod/bar-c",
					BlockType:       client.BlockTypeSubnet,
					Size:            28,
					Status:          "Deployed",
				},
			},
			ExpectedCloudIDsUpdated: map[string]string{
				"/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod":       "vpc-abc",
				"/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod/bar-a": "subnet-us-west-2a-0",
				"/Global/AWS/V4/Commercial/West/Prod-Data/123456-chris-west-prod/bar-c": "subnet-us-west-2c-0",
			},

			ExpectedVPCCIDRBlocksAssociated: []string{"10.0.0.0/27"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-west-2a"),
					CidrBlock:        aws.String("10.1.0.0/28"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-west-2a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-west-prod-bar-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("bar"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("data"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("prod"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-west-2c"),
					CidrBlock:        aws.String("10.2.0.0/28"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-west-2c-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-west-prod-bar-c"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("bar"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("data"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("prod"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-west-2a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1a",
									GroupName: "foo",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:  "subnet-us-west-2a-0",
									GroupName: "bar",
								},
							},
						},
					},
					"us-west-2c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-1c",
									GroupName: "foo",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:  "subnet-us-west-2c-0",
									GroupName: "bar",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Add unroutable subnets",

			VPCName: "chris-east-dev",
			Stack:   "dev",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},
			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-east-1",
				SubnetType: database.SubnetTypeUnroutable,
				SubnetSize: 27,
				GroupName:  "foo",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedVPCCIDRBlocksAssociated: []string{"100.107.0.0/16"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-east-1a"),
					CidrBlock:        aws.String("100.107.0.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1b"),
					CidrBlock:        aws.String("100.107.64.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1b-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-b"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1c"),
					CidrBlock:        aws.String("100.107.128.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1c-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-c"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1a-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1b-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1c-0",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Add unroutable subnets - existing conflicts",

			VPCName: "chris-east-dev",
			Stack:   "dev",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},
			ExistingVPCCIDRBlocks: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("100.107.0.0/16"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				// This one is ignored because it's disassociated
				{
					CidrBlock: aws.String("100.125.0.0/16"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("disassociated"),
					},
				},
			},

			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-east-1",
				SubnetType: database.SubnetTypeUnroutable,
				SubnetSize: 27,
				GroupName:  "foo",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedVPCCIDRBlocksAssociated: []string{"100.125.0.0/16"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-east-1a"),
					CidrBlock:        aws.String("100.125.0.0/16"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1a-0",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Add unroutable subnets - peering connection check",

			VPCName: "chris-east-dev",
			Stack:   "dev",
			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},
			ExistingPeeringConnections: []*ec2.VpcPeeringConnection{
				// Accepter matches
				{
					Status: &ec2.VpcPeeringConnectionStateReason{Code: aws.String("active")},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-abc"),
						CidrBlockSet: []*ec2.CidrBlock{
							{
								CidrBlock: aws.String("100.107.0.0/16"),
							},
						},
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-xyz"),
					},
				},
				// Requester matches
				{
					Status: &ec2.VpcPeeringConnectionStateReason{Code: aws.String("active")},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-xyz"),
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-abc"),
						CidrBlockSet: []*ec2.CidrBlock{
							{
								CidrBlock: aws.String("100.125.0.0/16"),
							},
						},
					},
				},
				// No match - 118 will be picked
				{
					Status: &ec2.VpcPeeringConnectionStateReason{Code: aws.String("active")},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-xyz"),
						CidrBlockSet: []*ec2.CidrBlock{
							{
								CidrBlock: aws.String("100.118.0.0/16"),
							},
						},
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId: aws.String("vpc-123"),
						CidrBlockSet: []*ec2.CidrBlock{
							{
								CidrBlock: aws.String("100.118.0.0/16"),
							},
						},
					},
				},
			},

			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:      "vpc-abc",
				Region:     "us-east-1",
				SubnetType: database.SubnetTypeUnroutable,
				SubnetSize: 27,
				GroupName:  "foo",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedVPCCIDRBlocksAssociated: []string{"100.118.0.0/16"},
			ExpectedSubnetsAdded: []*ec2.Subnet{
				{
					AvailabilityZone: aws.String("us-east-1a"),
					CidrBlock:        aws.String("100.118.0.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1a-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-a"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1b"),
					CidrBlock:        aws.String("100.118.64.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1b-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-b"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
				{
					AvailabilityZone: aws.String("us-east-1c"),
					CidrBlock:        aws.String("100.118.128.0/18"),
					VpcId:            aws.String("vpc-abc"),
					SubnetId:         aws.String("subnet-us-east-1c-0"),
					Tags: []*ec2.Tag{
						{
							Key:   aws.String("Name"),
							Value: aws.String("chris-east-dev-foo-c"),
						},
						{
							Key:   aws.String("GroupName"),
							Value: aws.String("foo"),
						},
						{
							Key:   aws.String("use"),
							Value: aws.String("unroutable"),
						},
						{
							Key:   aws.String("stack"),
							Value: aws.String("dev"),
						},
						{
							Key:   aws.String("Automated"),
							Value: aws.String("true"),
						},
						{
							Key:   aws.String("forbid_ec2"),
							Value: aws.String("true"),
						},
					},
				},
			},

			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1a-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1b-0",
									GroupName: "foo",
								},
							},
						},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-us-east-1c-0",
									GroupName: "foo",
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "Add firewall subnets for migration: subnets already exist",

			VPCName: "chris-east-dev",
			Stack:   "dev",
			StartState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1ToV1Firewall,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
						},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
						},
					},
				},
			},
			TaskConfig: database.AddZonedSubnetsTaskData{
				VPCID:        "vpc-abc",
				Region:       "us-east-1",
				SubnetType:   database.SubnetTypeFirewall,
				SubnetSize:   28,
				GroupName:    "firewall",
				BeIdempotent: true,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1ToV1Firewall,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
						},
					},
					"us-east-1c": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			rand.Seed(976)
			task := &testmocks.MockTask{
				ID: 1235,
			}
			vpcKey := string(tc.TaskConfig.Region) + tc.TaskConfig.VPCID
			mm := &testmocks.MockModelsManager{
				VPCs: map[string]*database.VPC{
					vpcKey: {
						AccountID: "123456",
						ID:        tc.TaskConfig.VPCID,
						State:     &tc.StartState,
						Name:      tc.VPCName,
						Stack:     tc.Stack,
						Region:    tc.TaskConfig.Region,
					},
				},
			}
			ipcontrol := &testmocks.MockIPControl{
				ExistingContainers: tc.ExistingContainers,
			}
			ec2 := &testmocks.MockEC2{
				PeeringConnections:      &tc.ExistingPeeringConnections,
				CIDRBlockAssociationSet: tc.ExistingVPCCIDRBlocks,
			}
			taskContext := &TaskContext{
				Task:          task,
				ModelsManager: mm,
				LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.TaskConfig.VPCID), database.TargetIPControlWrite),
				IPAM:          ipcontrol,
				BaseAWSAccountAccess: &awsp.AWSAccountAccess{
					EC2svc: ec2,
				},
			}

			taskContext.performAddZonedSubnetsTask(&tc.TaskConfig)

			// Overall task status
			if task.Status != tc.ExpectedTaskStatus {
				t.Fatalf("Incorrect task status. Expected %s but got %s", tc.ExpectedTaskStatus, task.Status)
			}

			// Expected IPControl calls
			if diff := cmp.Diff(tc.ExpectedContainersAdded, ipcontrol.ContainersAdded); diff != "" {
				t.Fatalf("Expected added containers in IPControl did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedBlocksAdded, ipcontrol.BlocksAdded); diff != "" {
				t.Fatalf("Expected added blocks in IPControl did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedCloudIDsUpdated, ipcontrol.CloudIDsUpdated); diff != "" {
				t.Fatalf("Expected cloud ID updates did not match actual: \n%s", diff)
			}

			// Expected AWS calls
			if diff := cmp.Diff(tc.ExpectedVPCCIDRBlocksAssociated, ec2.VPCCIDRBlocksAssociated[tc.TaskConfig.VPCID]); diff != "" {
				t.Fatalf("Expected CIDR blocks associated did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedSubnetsAdded, ec2.SubnetsCreated); diff != "" {
				t.Fatalf("Expected subnets created did not match actual: \n%s", diff)
			}

			// Saved state
			if diff := cmp.Diff(&tc.ExpectedEndState, mm.VPCs[vpcKey].State); diff != "" {
				t.Fatalf("Expected end state did not match state saved to database: \n%s", diff)
			}
		})
	}
}
