package main

import (
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
)

type removeZonedSubnetsTestCase struct {
	Name string

	StartState            database.VPCState
	ExistingContainers    testmocks.ContainerTree
	ExistingSubnetCIDRs   map[string]string
	ExistingVPCCIDRBlocks []*ec2.VpcCidrBlockAssociation
	ExistingDatabaseCIDRs []string

	TaskConfig database.RemoveZonedSubnetsTaskData

	ExpectedTaskStatus database.TaskStatus

	ExpectedSubnetsDeleted                   []string
	ExpectedRouteTablesDeleted               []string
	ExpectedCIDRBlocksDisassociated          []string
	ExpectedContainersDeletedWithTheirBlocks []string
	ExpectedBlocksDeleted                    []string
	ExpectedDatabaseCIDRs                    []string
	ExpectedEndState                         database.VPCState

	// TODO: add a CMSNet mock and test CMSNet calls
}

func TestPerformRemoveZonedSubnets(t *testing.T) {
	testCases := []removeZonedSubnetsTestCase{
		{
			Name: "Remove the only web group",

			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1a",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1b",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-b",
								},
							},
						},
					},
				},
			},
			ExistingDatabaseCIDRs: []string{"10.1.0.0/16", "10.100.123.0/24"},

			ExistingContainers: testmocks.ContainerTree{
				Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test",
				ResourceID: "vpc-aaa",
				Blocks: []testmocks.BlockSpec{
					{
						Address: "10.100.123.0",
						Size:    24,
					},
				},
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
						ResourceID: "subnet-1a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.100.123.0",
								Size:    25,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
						ResourceID: "subnet-1b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.100.123.128",
								Size:    25,
							},
						},
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:      "vpc-aaa",
				Region:     "us-gov-west-1",
				GroupName:  "frontend",
				SubnetType: database.SubnetTypeWeb,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedSubnetsDeleted: []string{
				"subnet-1a",
				"subnet-1b",
			},
			ExpectedRouteTablesDeleted: []string{
				"rt-a",
				"rt-b",
			},
			ExpectedContainersDeletedWithTheirBlocks: []string{
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test",
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
			},
			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {},
						},
					},
				},
			},
			ExpectedDatabaseCIDRs: []string{"10.1.0.0/16"},
		},

		{
			Name: "Remove one of two web groups - group covers whole CIDR",

			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1a",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1a",
								},
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1b",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1b",
								},
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExistingDatabaseCIDRs: []string{"10.1.0.0/16", "10.2.0.0/16"},

			ExistingContainers: testmocks.ContainerTree{
				Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test",
				ResourceID: "vpc-aaa",
				Blocks: []testmocks.BlockSpec{
					// Parent of frontend subnets
					{
						Address: "10.1.0.0",
						Size:    16,
					},
					// Parent of web2 subnets
					{
						Address: "10.2.0.0",
						Size:    16,
					},
				},
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
						ResourceID: "subnet-1a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.0.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
						ResourceID: "subnet-1b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.128.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-a",
						ResourceID: "subnet-2a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.0.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-b",
						ResourceID: "subnet-2b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.128.0",
								Size:    17,
							},
						},
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:      "vpc-aaa",
				Region:     "us-gov-west-1",
				GroupName:  "frontend",
				SubnetType: database.SubnetTypeWeb,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedSubnetsDeleted: []string{
				"subnet-1a",
				"subnet-1b",
			},
			ExpectedRouteTablesDeleted: []string{
				"rt-1a",
				"rt-1b",
			},
			ExpectedContainersDeletedWithTheirBlocks: []string{
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
			},
			ExpectedBlocksDeleted: []string{"10.1.0.0/16"},
			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExpectedDatabaseCIDRs: []string{"10.2.0.0/16"},
		},

		{
			Name: "Remove one of two web groups - group does not cover whole CIDR but other block is free",

			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1a",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1a",
								},
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1b",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1b",
								},
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExistingDatabaseCIDRs: []string{"10.1.0.0/16", "10.2.0.0/16"},

			ExistingContainers: testmocks.ContainerTree{
				Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test",
				ResourceID: "vpc-aaa",
				Blocks: []testmocks.BlockSpec{
					// Parent of frontend subnets
					{
						Address: "10.1.0.0",
						Size:    16,
					},
					// Parent of web2 subnets
					{
						Address: "10.2.0.0",
						Size:    16,
					},
					// Free space
					{
						Address: "10.1.128.0",
						Size:    17,
						Status:  "free",
					},
				},
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
						ResourceID: "subnet-1a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.0.0",
								Size:    18,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
						ResourceID: "subnet-1b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.64.0",
								Size:    18,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-a",
						ResourceID: "subnet-2a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.0.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-b",
						ResourceID: "subnet-2b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.128.0",
								Size:    17,
							},
						},
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:      "vpc-aaa",
				Region:     "us-gov-west-1",
				GroupName:  "frontend",
				SubnetType: database.SubnetTypeWeb,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedSubnetsDeleted: []string{
				"subnet-1a",
				"subnet-1b",
			},
			ExpectedRouteTablesDeleted: []string{
				"rt-1a",
				"rt-1b",
			},
			ExpectedContainersDeletedWithTheirBlocks: []string{
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
			},
			ExpectedBlocksDeleted: []string{"10.1.0.0/16"},
			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExpectedDatabaseCIDRs: []string{"10.2.0.0/16"},
		},

		{
			Name: "Remove one of two web groups - group does not cover whole CIDR and other block is not free",

			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1a",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1a",
								},
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-1b",
									GroupName:          "frontend",
									CustomRouteTableID: "rt-1b",
								},
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExistingDatabaseCIDRs: []string{"10.1.0.0/16", "10.2.0.0/16"},

			ExistingContainers: testmocks.ContainerTree{
				Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test",
				ResourceID: "vpc-aaa",
				Blocks: []testmocks.BlockSpec{
					// Parent of frontend subnets
					{
						Address: "10.1.0.0",
						Size:    16,
					},
					// Parent of web2 subnets
					{
						Address: "10.2.0.0",
						Size:    16,
					},
				},
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
						ResourceID: "subnet-1a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.0.0",
								Size:    18,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
						ResourceID: "subnet-1b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.1.64.0",
								Size:    18,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-a",
						ResourceID: "subnet-2a",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.0.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/web2-b",
						ResourceID: "subnet-2b",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.2.128.0",
								Size:    17,
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/xxx",
						ResourceID: "subnet-2b",
						Blocks: []testmocks.BlockSpec{
							// this block prevents deletion of 10.1.0.0/16 parent block
							{
								Address: "10.1.128.0",
								Size:    17,
								Status:  "aggregate",
							},
						},
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:      "vpc-aaa",
				Region:     "us-gov-west-1",
				GroupName:  "frontend",
				SubnetType: database.SubnetTypeWeb,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedSubnetsDeleted: []string{
				"subnet-1a",
				"subnet-1b",
			},
			ExpectedRouteTablesDeleted: []string{
				"rt-1a",
				"rt-1b",
			},
			ExpectedContainersDeletedWithTheirBlocks: []string{
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-a",
				"/Global/AWS/V4/GovCloud/West/Lower-Web/99445-chris-gov-west-test/frontend-b",
			},
			ExpectedBlocksDeleted: nil,
			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2a",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeWeb: {
								{
									SubnetID:           "subnet-2b",
									GroupName:          "web2",
									CustomRouteTableID: "rt-2b",
								},
							},
						},
					},
				},
			},
			ExpectedDatabaseCIDRs: []string{"10.1.0.0/16", "10.2.0.0/16"},
		},
		{
			Name: "Remove the only unroutable group",

			StartState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:           "subnet-1a",
									GroupName:          "eks",
									CustomRouteTableID: "rt-a",
								},
							},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {
								{
									SubnetID:           "subnet-1b",
									GroupName:          "eks",
									CustomRouteTableID: "rt-b",
								},
							},
						},
					},
				},
			},
			ExistingDatabaseCIDRs: []string{"100.77.0.0/16", "10.1.0.0/16"},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-1a":  "100.77.0.0/17",
				"subnet-1b":  "100.77.0.128/17",
				"subnet-xxx": "10.1.2.0/24",
			},
			ExistingVPCCIDRBlocks: []*ec2.VpcCidrBlockAssociation{
				{
					AssociationId: aws.String("assoc-1"),
					CidrBlock:     aws.String("100.77.0.0/16"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					AssociationId: aws.String("assoc-2"),
					CidrBlock:     aws.String("10.1.0.0/16"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:      "vpc-aaa",
				Region:     "us-gov-west-1",
				GroupName:  "eks",
				SubnetType: database.SubnetTypeUnroutable,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedSubnetsDeleted: []string{
				"subnet-1a",
				"subnet-1b",
			},
			ExpectedRouteTablesDeleted: []string{
				"rt-a",
				"rt-b",
			},
			ExpectedCIDRBlocksDisassociated: []string{"assoc-1"},
			ExpectedEndState: database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-gov-west-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {},
						},
					},
					"us-gov-west-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeUnroutable: {},
						},
					},
				},
			},
			ExpectedDatabaseCIDRs: []string{"10.1.0.0/16"},
		},

		{
			Name: "Remove firewall subnets for migration: subnets already removed",

			StartState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1FirewallToV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-west-2a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-west-2b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},

			TaskConfig: database.RemoveZonedSubnetsTaskData{
				VPCID:        "vpc-aaa",
				Region:       "us-west-2",
				GroupName:    "firewall",
				SubnetType:   database.SubnetTypeFirewall,
				BeIdempotent: true,
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1FirewallToV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-west-2a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
					"us-west-2b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			task := &testmocks.MockTask{
				ID: 5567,
			}
			vpcKey := string(tc.TaskConfig.Region) + tc.TaskConfig.VPCID
			mm := &testmocks.MockModelsManager{
				VPCs: map[string]*database.VPC{
					vpcKey: {
						AccountID: "99445",
						ID:        tc.TaskConfig.VPCID,
						State:     &tc.StartState,
						Region:    tc.TaskConfig.Region,
					},
				},
				SecondaryCIDRs: tc.ExistingDatabaseCIDRs,
			}
			ipcontrol := &testmocks.MockIPControl{
				ExistingContainers: tc.ExistingContainers,
			}
			ec2 := &testmocks.MockEC2{
				PrimaryCIDR:             aws.String("127.0.0.1/32"),
				SubnetCIDRs:             tc.ExistingSubnetCIDRs,
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
				CMSNet: &testmocks.MockCMSNet{},
			}

			taskContext.performRemoveZonedSubnetsTask(&tc.TaskConfig)

			// Overall task status
			if task.Status != tc.ExpectedTaskStatus {
				t.Fatalf("Incorrect task status. Expected %s but got %s", tc.ExpectedTaskStatus, task.Status)
			}

			// Expected IPControl calls
			if diff := cmp.Diff(tc.ExpectedContainersDeletedWithTheirBlocks, ipcontrol.ContainersDeletedWithTheirBlocks); diff != "" {
				t.Fatalf("Expected deleted containers in IPControl did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedBlocksDeleted, ipcontrol.BlocksDeleted); diff != "" {
				t.Fatalf("Expected deleted blocks in IPControl did not match actual: \n%s", diff)
			}

			// Expected AWS calls
			if diff := cmp.Diff(tc.ExpectedSubnetsDeleted, ec2.SubnetsDeleted); diff != "" {
				t.Fatalf("Expected deleted subnets did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRouteTablesDeleted, ec2.RouteTablesDeleted); diff != "" {
				t.Fatalf("Expected deleted route tables did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedCIDRBlocksDisassociated, ec2.CIDRBlocksDisassociated); diff != "" {
				t.Fatalf("Expected disassociated CIDR blocks did not match actual: \n%s", diff)
			}

			// Expected database updates
			if diff := cmp.Diff(&tc.ExpectedEndState, mm.VPCs[vpcKey].State); diff != "" {
				t.Fatalf("Expected end state did not match state saved to database: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedDatabaseCIDRs, mm.SecondaryCIDRs); diff != "" {
				t.Fatalf("Expected secondary CIDRs did not match CIDRs saved to database: \n%s", diff)
			}
		})
	}
}
