package main

import (
	"log"
	"math/rand"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type removeAvailabilityZoneTestCase struct {
	TestCaseName string

	VPCName                          string
	Stack                            string
	StartState                       database.VPCState
	ExistingContainers               testmocks.ContainerTree
	ExistingPeeringConnections       []*ec2.VpcPeeringConnection
	ExistingSubnetCIDRs              map[string]string
	ExistingVPCCIDRBlocks            []*ec2.VpcCidrBlockAssociation // only used when there are unroutable subnets
	ExistingRouteTables              []*ec2.RouteTable
	ExistingFirewalls                map[string]string // firewall id -> vpc id
	ExistingFirewallSubnetToEndpoint map[string]string // subnet id -> endpoint id
	ExistingFirewallPolicies         []*networkfirewall.FirewallPolicyMetadata

	TaskConfig database.RemoveAvailabilityZoneTaskData

	ExpectedContainersDeleted []string
	ExpectedBlocksDeleted     []string

	ExpectedCIDRBlocksDisassociated           []string
	ExpectedSubnetsDeleted                    []string
	ExpectedRouteTablesDeleted                []string
	ExpectedNATGatewaysDeleted                []string
	ExpectedFirewallSubnetAssociationsAdded   []string // subnet id
	ExpectedFirewallSubnetAssociationsRemoved []string // subnet id

	ExpectedTaskStatus database.TaskStatus

	ExpectedEndState database.VPCState
}

func TestPerformRemoveAvailabilityZone(t *testing.T) {
	testCases := []removeAvailabilityZoneTestCase{
		{
			TestCaseName: "Remove 1/3 with data",
			Stack:        "dev",

			VPCName: "chris-east-dev",
			StartState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-private-d": {
						RouteTableID: "rt-private-d",
					},
					"rt-data-d": {
						RouteTableID: "rt-data-d",
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName:          "private",
									SubnetID:           "subnet-private-d",
									CustomRouteTableID: "rt-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-d",
									CustomRouteTableID: "rt-data-d",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East",
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.10.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/private-a",
								ResourceID: "subnet-private-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/private-b",
								ResourceID: "subnet-private-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/private-d",
								ResourceID: "subnet-private-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/public-a",
								ResourceID: "subnet-public-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/public-b",
								ResourceID: "subnet-public-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/public-d",
								ResourceID: "subnet-public-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.3.0",
										Size:    18,
									},
								},
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-chris-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.12.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-chris-east-dev/data-a",
								ResourceID: "subnet-data-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-chris-east-dev/data-b",
								ResourceID: "subnet-data-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-chris-east-dev/data-d",
								ResourceID: "subnet-data-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.3.0",
										Size:    18,
									},
								},
							},
						},
					},
				},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-private-a": "10.10.1.0/18",
				"subnet-private-b": "10.10.2.0/18",
				"subnet-private-d": "10.10.3.0/18",
				"subnet-public-a":  "10.11.1.0/18",
				"subnet-public-b":  "10.11.2.0/18",
				"subnet-public-d":  "10.11.3.0/18",
				"subnet-data-a":    "10.12.1.0/18",
				"subnet-data-b":    "10.12.2.0/18",
				"subnet-data-d":    "10.12.3.0/18",
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-private-d"),
				},
				{
					RouteTableId: aws.String("rt-data-d"),
				},
			},

			TaskConfig: database.RemoveAvailabilityZoneTaskData{
				VPCID:  "vpc-abc",
				Region: "us-east-1",
				AZName: "us-east-1d",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersDeleted: []string{
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/private-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-chris-east-dev/public-d",
				"/Global/AWS/V4/Commercial/East/Lower-Data/123456-chris-east-dev/data-d",
			},
			ExpectedBlocksDeleted: []string{},

			ExpectedCIDRBlocksDisassociated: []string{},
			ExpectedSubnetsDeleted: []string{
				"subnet-private-d",
				"subnet-public-d",
				"subnet-data-d",
			},
			ExpectedRouteTablesDeleted: []string{"rt-private-d", "rt-data-d"},
			ExpectedNATGatewaysDeleted: []string{"ng-d"},

			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName: "private",
									SubnetID:  "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-a",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName: "private",
									SubnetID:  "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-b",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
				},
			},
		},
		{
			TestCaseName: "Remove 1/3 with unroutable",
			Stack:        "dev",

			VPCName: "jason-east-dev",
			StartState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-private-d": {
						RouteTableID: "rt-private-d",
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-unroutable-a",
									GroupName: "eks",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-unroutable-b",
									GroupName: "eks",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName:          "private",
									SubnetID:           "subnet-private-d",
									CustomRouteTableID: "rt-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-unroutable-d",
									GroupName: "eks",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East",
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.10.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-a",
								ResourceID: "subnet-private-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-b",
								ResourceID: "subnet-private-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
								ResourceID: "subnet-private-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-a",
								ResourceID: "subnet-public-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-b",
								ResourceID: "subnet-public-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
								ResourceID: "subnet-public-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.3.0",
										Size:    18,
									},
								},
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.12.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-a",
								ResourceID: "subnet-data-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-b",
								ResourceID: "subnet-data-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-d",
								ResourceID: "subnet-data-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.3.0",
										Size:    18,
									},
								},
							},
						},
					},
				},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-private-a":    "10.10.1.0/18",
				"subnet-private-b":    "10.10.2.0/18",
				"subnet-private-d":    "10.10.3.0/18",
				"subnet-public-a":     "10.11.1.0/18",
				"subnet-public-b":     "10.11.2.0/18",
				"subnet-public-d":     "10.11.3.0/18",
				"subnet-unroutable-a": "100.64.0.0/16",
				"subnet-unroutable-b": "100.65.0.0/16",
				"subnet-unroutable-d": "100.66.0.0/16",
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-private-d"),
				},
				{
					RouteTableId: aws.String("rt-data-d"),
				},
			},

			TaskConfig: database.RemoveAvailabilityZoneTaskData{
				VPCID:  "vpc-abc",
				Region: "us-east-1",
				AZName: "us-east-1d",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersDeleted: []string{
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
			},
			ExpectedBlocksDeleted: []string{},

			ExpectedCIDRBlocksDisassociated: []string{},
			ExpectedSubnetsDeleted: []string{
				"subnet-private-d",
				"subnet-public-d",
				"subnet-unroutable-d",
			},
			ExpectedRouteTablesDeleted: []string{"rt-private-d"},
			ExpectedNATGatewaysDeleted: []string{"ng-d"},

			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-unroutable-a",
									GroupName: "eks",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeUnroutable: {
								{
									SubnetID:  "subnet-unroutable-b",
									GroupName: "eks",
								},
							},
						},
					},
				},
			},
		},
		{
			TestCaseName: "Remove 1/4 with firewall",
			Stack:        "dev",

			VPCName: "jason-east-dev",
			StartState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						Routes: []*database.RouteInfo{{
							Destination: "10.11.4.0/18",
						}},
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
					},
					"rt-data-d": {
						RouteTableID: "rt-data-d",
					},
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-c", "subnet-firewall-d"},
				},
				InternetGateway: database.InternetGatewayInfo{
					RouteTableID:              "rt-firewall",
					InternetGatewayID:         "igw",
					RouteTableAssociationID:   "rta-d",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
					"us-east-1c": {
						PrivateRouteTableID: "rt-private-c",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-c",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-c",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-c",
									CustomRouteTableID: "rt-data-c",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-d",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									GroupName: "private",
									SubnetID:  "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-d",
									CustomRouteTableID: "rt-data-d",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East",
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.10.0.0",
								Size:    14,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-a",
								ResourceID: "subnet-private-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-b",
								ResourceID: "subnet-private-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-c",
								ResourceID: "subnet-private-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
								ResourceID: "subnet-private-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.4.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-a",
								ResourceID: "subnet-public-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-b",
								ResourceID: "subnet-public-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-c",
								ResourceID: "subnet-public-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
								ResourceID: "subnet-public-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.4.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-a",
								ResourceID: "subnet-firewall-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.1.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-b",
								ResourceID: "subnet-firewall-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.2.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-c",
								ResourceID: "subnet-firewall-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.3.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-d",
								ResourceID: "subnet-firewall-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.4.0",
										Size:    24,
									},
								},
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.20.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-a",
								ResourceID: "subnet-data-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.20.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-b",
								ResourceID: "subnet-data-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.20.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-c",
								ResourceID: "subnet-data-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.20.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-d",
								ResourceID: "subnet-data-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.20.4.0",
										Size:    18,
									},
								},
							},
						},
					},
				},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-private-a":  "10.10.1.0/18",
				"subnet-private-b":  "10.10.2.0/18",
				"subnet-private-c":  "10.10.3.0/18",
				"subnet-private-d":  "10.10.4.0/18",
				"subnet-public-a":   "10.11.1.0/18",
				"subnet-public-b":   "10.11.2.0/18",
				"subnet-public-c":   "10.11.3.0/18",
				"subnet-public-d":   "10.11.4.0/18",
				"subnet-firewall-a": "10.12.1.0/24",
				"subnet-firewall-b": "10.12.2.0/24",
				"subnet-firewall-c": "10.12.3.0/24",
				"subnet-firewall-d": "10.12.4.0/24",
				"subnet-data-a":     "10.20.1.0/18",
				"subnet-data-b":     "10.20.2.0/18",
				"subnet-data-c":     "10.20.3.0/18",
				"subnet-data-d":     "10.20.4.0/18",
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-firewall"),
				},
				{
					RouteTableId: aws.String("rt-private-d"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-d"),
							RouteTableAssociationId: aws.String("assoc-private-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-data-d"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-data-d"),
							RouteTableAssociationId: aws.String("assoc-data-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			ExistingFirewalls: map[string]string{"cms-cloud-vpc-abc-net-fw": "vpc-abc"},
			ExistingFirewallSubnetToEndpoint: map[string]string{
				"subnet-firewall-d": "vpce-existing",
			},
			ExistingFirewallPolicies: []*networkfirewall.FirewallPolicyMetadata{
				{
					Arn:  aws.String(testmocks.TestARN),
					Name: aws.String("cms-cloud-vpc-abc-default-fp"),
				},
			},
			TaskConfig: database.RemoveAvailabilityZoneTaskData{
				VPCID:  "vpc-abc",
				Region: "us-east-1",
				AZName: "us-east-1d",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersDeleted: []string{
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-d",
				"/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-d",
			},
			ExpectedBlocksDeleted: []string{},

			ExpectedCIDRBlocksDisassociated: []string{},
			ExpectedSubnetsDeleted: []string{
				"subnet-firewall-d",
				"subnet-private-d",
				"subnet-public-d",
				"subnet-data-d",
			},
			ExpectedRouteTablesDeleted:                []string{"rt-private-d", "rt-data-d"},
			ExpectedNATGatewaysDeleted:                []string{"ng-d"},
			ExpectedFirewallSubnetAssociationsRemoved: []string{"subnet-firewall-d"},

			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						Routes:       []*database.RouteInfo{},
					},
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-c"},
				},
				InternetGateway: database.InternetGatewayInfo{
					RouteTableID:              "rt-firewall",
					InternetGatewayID:         "igw",
					RouteTableAssociationID:   "rta-d",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
					"us-east-1c": {
						PrivateRouteTableID: "rt-private-c",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-c",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-c",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-c",
									CustomRouteTableID: "rt-data-c",
								},
							},
						},
					},
				},
			},
		},
		{
			TestCaseName: "Remove previously added AZ with separate ipcontrol container",
			Stack:        "dev",

			VPCName: "jason-east-dev",
			StartState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						Routes: []*database.RouteInfo{{
							Destination: "10.33.4.0/18",
						}},
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
					},
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-c", "subnet-firewall-d"},
				},
				InternetGateway: database.InternetGatewayInfo{
					RouteTableID:              "rt-firewall",
					InternetGatewayID:         "igw",
					RouteTableAssociationID:   "rta-d",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
						},
					},
					"us-east-1c": {
						PrivateRouteTableID: "rt-private-c",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-c",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-c",
									GroupName: "public",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-d",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									GroupName: "private",
									SubnetID:  "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East",
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.10.0.0",
								Size:    14,
							},
							{
								Address: "10.32.0.0",
								Size:    24,
							},
							{
								Address: "10.32.1.0",
								Size:    25,
							},
							{
								Address: "10.32.1.128",
								Size:    25,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
								ResourceID: "subnet-private-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.32.0.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
								ResourceID: "subnet-public-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.32.1.0",
										Size:    25,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-d",
								ResourceID: "subnet-firewall-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.32.1.128",
										Size:    25,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-a",
								ResourceID: "subnet-private-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-b",
								ResourceID: "subnet-private-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-c",
								ResourceID: "subnet-private-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-a",
								ResourceID: "subnet-public-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-b",
								ResourceID: "subnet-public-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-c",
								ResourceID: "subnet-public-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-a",
								ResourceID: "subnet-firewall-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.1.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-b",
								ResourceID: "subnet-firewall-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.2.0",
										Size:    24,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-c",
								ResourceID: "subnet-firewall-c",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.3.0",
										Size:    24,
									},
								},
							},
						},
					},
				},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-private-a":  "10.10.1.0/18",
				"subnet-private-b":  "10.10.2.0/18",
				"subnet-private-c":  "10.10.3.0/18",
				"subnet-private-d":  "10.32.4.0/18",
				"subnet-public-a":   "10.11.1.0/18",
				"subnet-public-b":   "10.11.2.0/18",
				"subnet-public-c":   "10.11.3.0/18",
				"subnet-public-d":   "10.33.4.0/18",
				"subnet-firewall-a": "10.12.1.0/24",
				"subnet-firewall-b": "10.12.2.0/24",
				"subnet-firewall-c": "10.12.3.0/24",
				"subnet-firewall-d": "10.34.4.0/24",
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-firewall"),
				},
				{
					RouteTableId: aws.String("rt-private-d"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-d"),
							RouteTableAssociationId: aws.String("assoc-private-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			ExistingFirewalls: map[string]string{"cms-cloud-vpc-abc-net-fw": "vpc-abc"},
			ExistingFirewallSubnetToEndpoint: map[string]string{
				"subnet-firewall-d": "vpce-existing",
			},
			ExistingFirewallPolicies: []*networkfirewall.FirewallPolicyMetadata{
				{
					Arn:  aws.String(testmocks.TestARN),
					Name: aws.String("cms-cloud-vpc-abc-default-fp"),
				},
			},
			TaskConfig: database.RemoveAvailabilityZoneTaskData{
				VPCID:  "vpc-abc",
				Region: "us-east-1",
				AZName: "us-east-1d",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedContainersDeleted: []string{
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
				"/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/firewall-d",
			},
			ExpectedBlocksDeleted: []string{
				"10.32.0.0/24",
				"10.32.1.0/25",
				"10.32.1.128/25",
			},

			ExpectedCIDRBlocksDisassociated: []string{},
			ExpectedSubnetsDeleted: []string{
				"subnet-firewall-d",
				"subnet-private-d",
				"subnet-public-d",
			},
			ExpectedRouteTablesDeleted:                []string{"rt-private-d"},
			ExpectedNATGatewaysDeleted:                []string{"ng-d"},
			ExpectedFirewallSubnetAssociationsRemoved: []string{"subnet-firewall-d"},

			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						Routes:       []*database.RouteInfo{},
					},
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-c"},
				},
				InternetGateway: database.InternetGatewayInfo{
					RouteTableID:              "rt-firewall",
					InternetGatewayID:         "igw",
					RouteTableAssociationID:   "rta-d",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-a",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-b",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
						},
					},
					"us-east-1c": {
						PrivateRouteTableID: "rt-private-c",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-firewall-c",
									GroupName: "firewall",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-c",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-c",
									GroupName: "public",
								},
							},
						},
					},
				},
			},
		},
		{
			TestCaseName: "Fail to remove non-existent az",
			Stack:        "dev",

			VPCName: "jason-east-dev",
			StartState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName:          "private",
									SubnetID:           "subnet-private-d",
									CustomRouteTableID: "rt-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-d",
									CustomRouteTableID: "rt-data-d",
								},
							},
						},
					},
				},
			},
			ExistingContainers: testmocks.ContainerTree{
				Name: "/Global/AWS/V4/Commercial/East",
				Children: []testmocks.ContainerTree{
					{
						Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.10.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-a",
								ResourceID: "subnet-private-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-b",
								ResourceID: "subnet-private-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/private-d",
								ResourceID: "subnet-private-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.10.3.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-a",
								ResourceID: "subnet-public-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-b",
								ResourceID: "subnet-public-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Development and Test/123456-jason-east-dev/public-d",
								ResourceID: "subnet-public-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.11.3.0",
										Size:    18,
									},
								},
							},
						},
					},
					{
						Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev",
						ResourceID: "vpc-abc",
						Blocks: []testmocks.BlockSpec{
							{
								Address: "10.12.0.0",
								Size:    16,
							},
						},
						Children: []testmocks.ContainerTree{
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-a",
								ResourceID: "subnet-data-a",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.1.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-b",
								ResourceID: "subnet-data-b",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.2.0",
										Size:    18,
									},
								},
							},
							{
								Name:       "/Global/AWS/V4/Commercial/East/Lower-Data/123456-jason-east-dev/data-d",
								ResourceID: "subnet-data-d",
								Blocks: []testmocks.BlockSpec{
									{
										Address: "10.12.3.0",
										Size:    18,
									},
								},
							},
						},
					},
				},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-private-a": "10.10.1.0/18",
				"subnet-private-b": "10.10.2.0/18",
				"subnet-private-d": "10.10.3.0/18",
				"subnet-public-a":  "10.11.1.0/18",
				"subnet-public-b":  "10.11.2.0/18",
				"subnet-public-d":  "10.11.3.0/18",
				"subnet-data-a":    "10.12.1.0/18",
				"subnet-data-b":    "10.12.2.0/18",
				"subnet-data-d":    "10.12.3.0/18",
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-private-d"),
				},
				{
					RouteTableId: aws.String("rt-data-d"),
				},
			},
			TaskConfig: database.RemoveAvailabilityZoneTaskData{
				VPCID:  "vpc-abc",
				Region: "us-east-1",
				AZName: "us-east-1c",
			},

			ExpectedTaskStatus: database.TaskStatusFailed,

			ExpectedBlocksDeleted: []string{},

			ExpectedCIDRBlocksDisassociated: []string{},

			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-a",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-a",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-a",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-a",
									CustomRouteTableID: "rt-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-private-b",
									GroupName: "private",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:  "subnet-public-b",
									GroupName: "public",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-b",
									CustomRouteTableID: "rt-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "ng-d",
						},
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									GroupName:          "private",
									SubnetID:           "subnet-private-d",
									CustomRouteTableID: "rt-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									GroupName: "public",
									SubnetID:  "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "db",
									SubnetID:           "subnet-data-d",
									CustomRouteTableID: "rt-data-d",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		log.Printf("\n---------- Running test case %q ----------", tc.TestCaseName)
		t.Run(tc.TestCaseName, func(t *testing.T) {
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
				BlocksDeleted:      []string{},
			}
			ec2 := &testmocks.MockEC2{
				PeeringConnections:      &tc.ExistingPeeringConnections,
				CIDRBlockAssociationSet: tc.ExistingVPCCIDRBlocks,
				SubnetCIDRs:             tc.ExistingSubnetCIDRs,
				CIDRBlocksDisassociated: []string{},
				RouteTables:             tc.ExistingRouteTables,
			}
			subnetIDToAZ := make(map[string]string)
			for azName, info := range tc.StartState.AvailabilityZones {
				for _, subnets := range info.Subnets {
					for _, subnet := range subnets {
						subnetIDToAZ[subnet.SubnetID] = azName
					}
				}
			}
			nfsvc := &testmocks.MockNetworkFirewall{
				SubnetIDToAZ:               subnetIDToAZ,
				AssociatedSubnetToEndpoint: tc.ExistingFirewallSubnetToEndpoint,
				Firewalls:                  tc.ExistingFirewalls,
				FirewallPolicies:           tc.ExistingFirewallPolicies,
			}
			taskContext := &TaskContext{
				Task:          task,
				ModelsManager: mm,
				LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.TaskConfig.VPCID), database.TargetIPControlWrite),
				IPAM:          ipcontrol,
				CMSNet:        &testmocks.MockCMSNet{},
				BaseAWSAccountAccess: &awsp.AWSAccountAccess{
					EC2svc: ec2,
					NFsvc:  nfsvc,
				},
			}

			taskContext.performRemoveAvailabilityZoneTask(&tc.TaskConfig)

			// Overall task status
			if task.Status != tc.ExpectedTaskStatus {
				t.Fatalf("Incorrect task status. Expected %s but got %s. Last log message: %s", tc.ExpectedTaskStatus, task.Status, task.LastLoggedMessage)
			}

			sortStringsOption := cmpopts.SortSlices(func(x, y string) bool { return x < y })

			// Expected IPControl calls
			if diff := cmp.Diff(tc.ExpectedContainersDeleted, ipcontrol.ContainersDeletedWithTheirBlocks, sortStringsOption); diff != "" {
				t.Fatalf("Expected deleted containers in IPControl did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedBlocksDeleted, ipcontrol.BlocksDeleted, sortStringsOption); diff != "" {
				t.Fatalf("Expected deleted blocks in IPControl did not match actual: \n%s", diff)
			}

			// Expected AWS calls
			if diff := cmp.Diff(tc.ExpectedCIDRBlocksDisassociated, ec2.CIDRBlocksDisassociated, sortStringsOption); diff != "" {
				t.Fatalf("Expected CIDR blocks associated did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRouteTablesDeleted, ec2.RouteTablesDeleted, sortStringsOption); diff != "" {
				t.Fatalf("Expected deleted route tables did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedSubnetsDeleted, ec2.SubnetsDeleted, sortStringsOption); diff != "" {
				t.Fatalf("Expected deleted subnets did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedNATGatewaysDeleted, ec2.NATGatewaysDeleted, sortStringsOption); diff != "" {
				t.Fatalf("Expected deleted NAT Gateways did not match actual: \n%s", diff)
			}

			// Network Firewall Changes
			if tc.StartState.VPCType.HasFirewall() {
				if diff := cmp.Diff(tc.ExpectedFirewallSubnetAssociationsAdded, nfsvc.SubnetAssociationsAdded, sortStringsOption); diff != "" {
					t.Errorf("Expected firewall subnet associations added did not match actual: \n%s", diff)
				}
				if diff := cmp.Diff(tc.ExpectedFirewallSubnetAssociationsRemoved, nfsvc.SubnetAssociationsRemoved, sortStringsOption); diff != "" {
					t.Errorf("Expected firewall subnet associations removed did not match actual: \n%s", diff)
				}
			}

			// Saved state
			if diff := cmp.Diff(&tc.ExpectedEndState, mm.VPCs[vpcKey].State); diff != "" {
				t.Fatalf("Expected end state did not match state saved to database: \n%s", diff)
			}
		})
	}
}
