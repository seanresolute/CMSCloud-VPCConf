package main

import (
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

type updateNetworkingTestCase struct {
	Name string

	StartState                       database.VPCState
	ExistingRouteTables              []*ec2.RouteTable
	ExistingSubnetCIDRs              map[string]string
	ExistingFirewalls                map[string]string // firewall id -> vpc id
	ExistingFirewallSubnetToEndpoint map[string]string // subnet id -> endpoint id
	ExistingFirewallPolicies         []*networkfirewall.FirewallPolicyMetadata
	ExistingRuleGroups               []*networkfirewall.RuleGroupMetadata

	TaskConfig database.UpdateNetworkingTaskData

	ExpectedTaskStatus                        database.TaskStatus
	ExpectedEndState                          database.VPCState
	ExpectedRouteTablesCreated                []string
	ExpectedRouteTablesDeleted                []string
	ExpectedRouteTableAssociationsCreated     []*ec2.RouteTableAssociation
	ExpectedRouteTableAssociationsReplaced    map[string]string // old assoc id -> new route table id
	ExpectedRouteTableAssociationsRemoved     []string
	ExpectedInternetGatewayWasCreated         bool
	ExpectedInternetGatewaysAttached          map[string]string
	ExpectedInternetGatewaysDetached          map[string]string // gateway id -> vpc id
	ExpectedInternetGatewaysDeleted           []string
	ExpectedRoutesAdded                       map[string][]*database.RouteInfo
	ExpectedRoutesDeleted                     map[string][]string
	ExpectedEIPsAllocated                     []string
	ExpectedEIPsReleased                      []string
	ExpectedNATGatewaysCreated                []*ec2.NatGateway
	ExpectedNATGatewaysDeleted                []string
	ExpectedTagsCreated                       map[string][]string // resource id -> [key=value]
	ExpectedTagsDeleted                       map[string][]string // resource id -> [key=value]
	ExpectedFirewallPoliciesCreated           []string            // policy name
	ExpectedFirewallSubnetAssociationsAdded   []string            // subnet id
	ExpectedFirewallSubnetAssociationsRemoved []string            // subnet id
	ExpectedFirewallsCreated                  map[string]string   // firewall name -> vpc id
	ExpectedCreatedPolicyToRuleGroupARNs      map[string][]string // policy name -> [configured rule group ARNs]
}

func TestPerformUpdateNetworking(t *testing.T) {
	testCases := []updateNetworkingTestCase{
		{
			Name: "Basic case; connect nothing",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-d",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},
			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-123",
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5", "rt-6", "rt-7"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-data-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-private-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-7"),
					RouteTableId:            aws.String("rt-5"),
					SubnetId:                aws.String("subnet-data-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-8"),
					RouteTableId:            aws.String("rt-6"),
					SubnetId:                aws.String("subnet-private-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-9"),
					RouteTableId:            aws.String("rt-7"),
					SubnetId:                aws.String("subnet-data-d"),
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"rt-1": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-3": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-a")},
				"rt-4": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-5": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-b")},
				"rt-6": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
				"rt-7": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-123": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeData,
					},
				},
				PublicRouteTableID: "rt-1",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									CustomRouteTableID:      "rt-3",
									RouteTableAssociationID: "assoc-5",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-4",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									CustomRouteTableID:      "rt-5",
									RouteTableAssociationID: "assoc-7",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-6",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									CustomRouteTableID:      "rt-7",
									RouteTableAssociationID: "assoc-9",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Basic case; connect public only",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-d",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5", "rt-6", "rt-7"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-data-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-private-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-7"),
					RouteTableId:            aws.String("rt-5"),
					SubnetId:                aws.String("subnet-data-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-8"),
					RouteTableId:            aws.String("rt-6"),
					SubnetId:                aws.String("subnet-private-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-9"),
					RouteTableId:            aws.String("rt-7"),
					SubnetId:                aws.String("subnet-data-d"),
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
			},
			ExpectedInternetGatewayWasCreated: true,
			ExpectedInternetGatewaysAttached:  map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedTagsCreated: map[string][]string{
				"igw-abc": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc")},
				"rt-1":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-3":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-a")},
				"rt-4":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-5":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-b")},
				"rt-6":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
				"rt-7":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeData,
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									CustomRouteTableID:      "rt-3",
									RouteTableAssociationID: "assoc-5",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-4",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									CustomRouteTableID:      "rt-5",
									RouteTableAssociationID: "assoc-7",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-6",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									CustomRouteTableID:      "rt-7",
									RouteTableAssociationID: "assoc-9",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Basic case; connect public and private",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-d",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic:  true,
					ConnectPrivate: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5", "rt-6", "rt-7"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-data-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-private-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-7"),
					RouteTableId:            aws.String("rt-5"),
					SubnetId:                aws.String("subnet-data-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-8"),
					RouteTableId:            aws.String("rt-6"),
					SubnetId:                aws.String("subnet-private-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-9"),
					RouteTableId:            aws.String("rt-7"),
					SubnetId:                aws.String("subnet-data-d"),
				},
			},
			ExpectedInternetGatewayWasCreated: true,
			ExpectedInternetGatewaysAttached:  map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedEIPsAllocated:             []string{"alloc-1", "alloc-2", "alloc-3"},
			ExpectedNATGatewaysCreated: []*ec2.NatGateway{
				{
					SubnetId:     aws.String("subnet-public-a"),
					NatGatewayId: aws.String("nat-1"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-1"),
						},
					},
				},
				{
					SubnetId:     aws.String("subnet-public-b"),
					NatGatewayId: aws.String("nat-2"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-2"),
						},
					},
				},
				{
					SubnetId:     aws.String("subnet-public-d"),
					NatGatewayId: aws.String("nat-3"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-3"),
						},
					},
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
				"rt-2": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-3": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-4": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
				"rt-5": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
				"rt-6": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-3",
					},
				},
				"rt-7": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-3",
					},
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"alloc-1": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-a")},
				"alloc-2": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-b")},
				"alloc-3": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-d")},
				"igw-abc": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc")},
				"nat-1":   {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-a")},
				"nat-2":   {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-b")},
				"nat-3":   {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-d")},
				"rt-1":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-3":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-a")},
				"rt-4":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-5":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-b")},
				"rt-6":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
				"rt-7":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-3",
							},
						},
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-3",
							},
						},
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-1",
							EIPID:        "alloc-1",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									RouteTableAssociationID: "assoc-5",
									CustomRouteTableID:      "rt-3",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-4",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-2",
							EIPID:        "alloc-2",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									RouteTableAssociationID: "assoc-7",
									CustomRouteTableID:      "rt-5",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-6",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-3",
							EIPID:        "alloc-3",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									RouteTableAssociationID: "assoc-9",
									CustomRouteTableID:      "rt-7",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Connect public and private with some resources already existing",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				// IGW exists but is not connected
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID: "igw-ggg",
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
						// no routes yet
					},
					"rt-data-b": {
						RouteTableID: "rt-data-b",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-b",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						// private route table exists but is not associated
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-a",
								},
							},
						},
					},
					"us-east-1b": {
						// NAT gateway exists
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-b",
							EIPID:        "alloc-b",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
							// data route table exists and is associated
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									RouteTableAssociationID: "assoc-data-b",
									CustomRouteTableID:      "rt-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID: "subnet-data-d",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-private-a"),
				},
				{
					RouteTableId: aws.String("rt-data-b"),
				},
				// AZ d subnets are already associated with random route tables
				{
					RouteTableId: aws.String("rt-xxx"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("assoc-bad-existing-private"),
							SubnetId:                aws.String("subnet-private-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-yyy"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("assoc-bad-existing-public"),
							SubnetId:                aws.String("subnet-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							RouteTableAssociationId: aws.String("assoc-bad-existing-data"),
							SubnetId:                aws.String("subnet-data-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic:  true,
					ConnectPrivate: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5"},
			ExpectedRouteTableAssociationsReplaced: map[string]string{
				"assoc-bad-existing-public":  "rt-1",
				"assoc-bad-existing-private": "rt-4",
				"assoc-bad-existing-data":    "rt-5",
			},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-private-a"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-data-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-private-b"),
				},
			},
			ExpectedInternetGatewayWasCreated: false,
			ExpectedInternetGatewaysAttached:  map[string]string{"igw-ggg": "vpc-xyz"},
			ExpectedEIPsAllocated:             []string{"alloc-1", "alloc-2"},
			ExpectedNATGatewaysCreated: []*ec2.NatGateway{
				{
					SubnetId:     aws.String("subnet-public-a"),
					NatGatewayId: aws.String("nat-1"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-1"),
						},
					},
				},
				{
					SubnetId:     aws.String("subnet-public-d"),
					NatGatewayId: aws.String("nat-2"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-2"),
						},
					},
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: "igw-ggg",
					},
				},
				"rt-private-a": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-2": { // data-a subnet
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-3": { // private-b subnet
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-b",
					},
				},
				"rt-4": { // private-d subnet
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
				"rt-5": { // data-d subnet
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"alloc-1": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-a")},
				"alloc-2": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-d")},
				"nat-1":   {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-a")},
				"nat-2":   {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-d")},
				"rt-1":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-a")},
				"rt-3":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-4":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
				"rt-5":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-data-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: "igw-ggg",
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-b",
							},
						},
					},
					"rt-data-b": {
						RouteTableID: "rt-data-b",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-b",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         "igw-ggg",
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-1",
							EIPID:        "alloc-1",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									RouteTableAssociationID: "assoc-5",
									CustomRouteTableID:      "rt-2",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-3",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-b",
							EIPID:        "alloc-b",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									RouteTableAssociationID: "assoc-data-b",
									CustomRouteTableID:      "rt-data-b",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-4",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-2",
							EIPID:        "alloc-2",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-7",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									RouteTableAssociationID: "assoc-8",
									CustomRouteTableID:      "rt-5",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Disconnect everything",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-3",
							},
						},
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-3",
							},
						},
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-1",
							EIPID:        "alloc-1",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									RouteTableAssociationID: "assoc-5",
									CustomRouteTableID:      "rt-3",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-4",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-2",
							EIPID:        "alloc-2",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									RouteTableAssociationID: "assoc-7",
									CustomRouteTableID:      "rt-5",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-6",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-3",
							EIPID:        "alloc-3",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									RouteTableAssociationID: "assoc-9",
									CustomRouteTableID:      "rt-7",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-1"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("assoc-1"),
							SubnetId:                aws.String("subnet-public-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							RouteTableAssociationId: aws.String("assoc-2"),
							SubnetId:                aws.String("subnet-public-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							RouteTableAssociationId: aws.String("assoc-3"),
							SubnetId:                aws.String("subnet-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-2"),
				},
				{
					RouteTableId: aws.String("rt-3"),
				},
				{
					RouteTableId: aws.String("rt-4"),
				},
				{
					RouteTableId: aws.String("rt-5"),
				},
				{
					RouteTableId: aws.String("rt-6"),
				},
				{
					RouteTableId: aws.String("rt-7"),
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID:            "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{},
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,

			ExpectedRoutesDeleted: map[string][]string{
				"rt-1": {"0.0.0.0/0"},
				"rt-2": {"0.0.0.0/0"},
				"rt-3": {"0.0.0.0/0"},
				"rt-4": {"0.0.0.0/0"},
				"rt-5": {"0.0.0.0/0"},
				"rt-6": {"0.0.0.0/0"},
				"rt-7": {"0.0.0.0/0"},
			},
			ExpectedInternetGatewaysDetached: map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedInternetGatewaysDeleted:  []string{testmocks.NewIGWID},
			ExpectedEIPsReleased:             []string{"alloc-1", "alloc-2", "alloc-3"},
			ExpectedNATGatewaysDeleted:       []string{"nat-1", "nat-2", "nat-3"},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes:       []*database.RouteInfo{},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       []*database.RouteInfo{},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeData,
						Routes:       []*database.RouteInfo{},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       []*database.RouteInfo{},
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypeData,
						Routes:       []*database.RouteInfo{},
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       []*database.RouteInfo{},
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeData,
						Routes:       []*database.RouteInfo{},
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway:    database.InternetGatewayInfo{},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						NATGateway:          database.NATGatewayInfo{},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-a",
									RouteTableAssociationID: "assoc-5",
									CustomRouteTableID:      "rt-3",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-4",
						NATGateway:          database.NATGatewayInfo{},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-b",
									RouteTableAssociationID: "assoc-7",
									CustomRouteTableID:      "rt-5",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-6",
						NATGateway:          database.NATGatewayInfo{},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:                "subnet-data-d",
									RouteTableAssociationID: "assoc-9",
									CustomRouteTableID:      "rt-7",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Multiple public subnet groups; some already associated with RT",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				// Public RT already exists
				PublicRouteTableID: "rt-public",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public": {
						RouteTableID: "rt-public",
						SubnetType:   database.SubnetTypePublic,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
								{
									// already associated
									SubnetID:                "subnet-public2-a",
									RouteTableAssociationID: "assoc-public2-a",
									GroupName:               "public2",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
								{
									SubnetID:  "subnet-public2-b",
									GroupName: "public2",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
								{
									// already associated
									SubnetID:                "subnet-public2-d",
									RouteTableAssociationID: "assoc-public-d",
									GroupName:               "public2",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-public"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("assoc-public2-a"),
							SubnetId:                aws.String("subnet-public2-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							RouteTableAssociationId: aws.String("assoc-public2-d"),
							SubnetId:                aws.String("subnet-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic: true,
				},
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			// private per-AZ route tables get created even if there are no private subnets
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-public"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-public"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-public"),
					SubnetId:                aws.String("subnet-public2-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-public"),
					SubnetId:                aws.String("subnet-public2-d"),
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-public": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
			},
			ExpectedInternetGatewayWasCreated: true,
			ExpectedInternetGatewaysAttached:  map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedTagsCreated: map[string][]string{
				"igw-abc": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc")},
				"rt-1":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-2":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-3":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public": {
						RouteTableID: "rt-public",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					// private per-AZ route tables get created even if there are no private subnets
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				PublicRouteTableID: "rt-public",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-1",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
								{
									// previously associated
									SubnetID:                "subnet-public2-a",
									RouteTableAssociationID: "assoc-public2-a",
									GroupName:               "public2",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
								{
									SubnetID:                "subnet-public2-b",
									RouteTableAssociationID: "assoc-3",
									GroupName:               "public2",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-3",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									// previously associated
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-public2-d",
								},
								{
									SubnetID:                "subnet-public2-d",
									RouteTableAssociationID: "assoc-4",
									GroupName:               "public2",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Multiple private subnet groups",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
								{
									SubnetID:  "subnet-myapp-a",
									GroupName: "myapp",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
								{
									SubnetID:  "subnet-myapp-b",
									GroupName: "myapp",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
								{
									SubnetID:  "subnet-myapp-d",
									GroupName: "myapp",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},
			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-myapp-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-private-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-myapp-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-private-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-myapp-d"),
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"rt-1": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-3": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-4": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					// Public route table still gets created if there are no public subnets
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
					},
					// Each AZ only gets one private route table
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				PublicRouteTableID: "rt-1",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-1",
								},
								{
									SubnetID:                "subnet-myapp-a",
									RouteTableAssociationID: "assoc-2",
									GroupName:               "myapp",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-3",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-3",
								},
								{
									SubnetID:                "subnet-myapp-b",
									RouteTableAssociationID: "assoc-4",
									GroupName:               "myapp",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-4",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-5",
								},
								{
									SubnetID:                "subnet-myapp-d",
									RouteTableAssociationID: "assoc-6",
									GroupName:               "myapp",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Multiple app subnets",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-app1-a",
									GroupName: "app1",
								},
								{
									SubnetID:  "subnet-app2-a",
									GroupName: "app2",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:  "subnet-app1-b",
									GroupName: "app1",
								},
								{
									SubnetID:  "subnet-app2-b",
									GroupName: "app2",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic:  true,
					ConnectPrivate: true,
				},
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			// Private route tables are still created even with no subnets
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5", "rt-6", "rt-7"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				// Each zoned subnet gets its own route table
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-app1-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-app2-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-6"),
					SubnetId:                aws.String("subnet-app1-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-7"),
					SubnetId:                aws.String("subnet-app2-b"),
				},
			},
			ExpectedInternetGatewayWasCreated: true,
			ExpectedInternetGatewaysAttached:  map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedEIPsAllocated:             []string{"alloc-1", "alloc-2"},
			ExpectedNATGatewaysCreated: []*ec2.NatGateway{
				{
					SubnetId:     aws.String("subnet-public-a"),
					NatGatewayId: aws.String("nat-1"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-1"),
						},
					},
				},
				{
					SubnetId:     aws.String("subnet-public-b"),
					NatGatewayId: aws.String("nat-2"),
					NatGatewayAddresses: []*ec2.NatGatewayAddress{
						{
							AllocationId: aws.String("alloc-2"),
						},
					},
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
				"rt-2": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-3": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-4": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-1",
					},
				},
				"rt-5": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
				"rt-6": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
				"rt-7": {
					{
						Destination:  "0.0.0.0/0",
						NATGatewayID: "nat-2",
					},
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"alloc-1":          {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-a")},
				"alloc-2":          {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-nat-gateway-b")},
				testmocks.NewIGWID: {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc")},
				"nat-1":            {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-a")},
				"nat-2":            {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-b")},
				"rt-1":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
				"rt-2":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-3":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-app1-a")},
				"rt-4":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-app2-a")},
				"rt-5":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-6":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-app1-b")},
				"rt-7":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-app2-b")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-1",
							},
						},
					},
					"rt-5": {
						RouteTableID: "rt-5",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:  "0.0.0.0/0",
								NATGatewayID: "nat-2",
							},
						},
					},
				},
				PublicRouteTableID: "rt-1",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-2",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-1",
							EIPID:        "alloc-1",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:                "subnet-app1-a",
									GroupName:               "app1",
									CustomRouteTableID:      "rt-3",
									RouteTableAssociationID: "assoc-3",
								},
								{
									SubnetID:                "subnet-app2-a",
									GroupName:               "app2",
									CustomRouteTableID:      "rt-4",
									RouteTableAssociationID: "assoc-4",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-5",
						NATGateway: database.NATGatewayInfo{
							NATGatewayID: "nat-2",
							EIPID:        "alloc-2",
						},
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:                "subnet-app1-b",
									GroupName:               "app1",
									CustomRouteTableID:      "rt-6",
									RouteTableAssociationID: "assoc-5",
								},
								{
									SubnetID:                "subnet-app2-b",
									GroupName:               "app2",
									CustomRouteTableID:      "rt-7",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
						},
					},
				},
			},
		},

		{
			// TODO: this should have custom route tables
			Name: "Legacy VPC",

			StartState: database.VPCState{
				VPCType: database.VPCTypeLegacy,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-private-a": {
						RouteTableID: "rt-private-a",
					},
					"rt-private-c": {
						RouteTableID: "rt-private-c",
					},
					"rt-public-a": {
						RouteTableID: "rt-public-a",
					},
					"rt-public-c": {
						RouteTableID: "rt-public-c",
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:           "subnet-private-a",
									CustomRouteTableID: "rt-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:           "subnet-public-a",
									CustomRouteTableID: "rt-public-a",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:           "subnet-private-c",
									CustomRouteTableID: "rt-private-c",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:           "subnet-public-c",
									CustomRouteTableID: "rt-public-c",
								},
							},
						},
					},
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					// ConnectPublic and ConnectPrivate should be ignored
					ConnectPublic:  true,
					ConnectPrivate: true,
				},
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeLegacy,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-private-a": {
						RouteTableID: "rt-private-a",
					},
					"rt-private-c": {
						RouteTableID: "rt-private-c",
					},
					"rt-public-a": {
						RouteTableID: "rt-public-a",
					},
					"rt-public-c": {
						RouteTableID: "rt-public-c",
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:           "subnet-private-a",
									CustomRouteTableID: "rt-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:           "subnet-public-a",
									CustomRouteTableID: "rt-public-a",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:           "subnet-private-c",
									CustomRouteTableID: "rt-private-c",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:           "subnet-public-c",
									CustomRouteTableID: "rt-public-c",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Firewall VPC, create new (no existing RTs/assocs)",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-public-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-d",
								},
							},
						},
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{},
			},
			ExistingSubnetCIDRs: map[string]string{
				"subnet-public-a": "10.147.152.96/27",
				"subnet-public-b": "10.147.152.128/27",
				"subnet-public-d": "10.147.152.160/27",
			},
			ExistingRuleGroups: []*networkfirewall.RuleGroupMetadata{
				{
					Name: aws.String(awsp.ManagedStatefulRuleGroupName),
					Arn:  aws.String("managed-stateful-arn"),
				},
				{
					Name: aws.String(awsp.ManagedStatelessRuleGroupName),
					Arn:  aws.String("managed-stateless-arn"),
				},
				{
					Name: aws.String("cms-cloud-stateful-rg-1"),
					Arn:  aws.String("other-stateful-arn"),
				},
				{
					Name: aws.String("cms-cloud-stateless-rg-1"),
					Arn:  aws.String("other-stateless-arn"),
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5", "rt-6", "rt-7", "rt-8"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-5"),
					GatewayId:               aws.String(testmocks.NewIGWID),
				},
				{
					RouteTableAssociationId: aws.String("assoc-5"),
					RouteTableId:            aws.String("rt-1"),
					SubnetId:                aws.String("subnet-public-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-6"),
					RouteTableId:            aws.String("rt-2"),
					SubnetId:                aws.String("subnet-public-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-7"),
					RouteTableId:            aws.String("rt-3"),
					SubnetId:                aws.String("subnet-public-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-8"),
					RouteTableId:            aws.String("rt-6"),
					SubnetId:                aws.String("subnet-private-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-9"),
					RouteTableId:            aws.String("rt-7"),
					SubnetId:                aws.String("subnet-private-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-10"),
					RouteTableId:            aws.String("rt-8"),
					SubnetId:                aws.String("subnet-private-d"),
				},
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-1",
					},
				},
				"rt-2": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-2",
					},
				},
				"rt-3": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-3",
					},
				},
				"rt-4": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
				"rt-5": {
					{
						Destination:   "10.147.152.96/27",
						VPCEndpointID: "vpce-1",
					},
					{
						Destination:   "10.147.152.128/27",
						VPCEndpointID: "vpce-2",
					},
					{
						Destination:   "10.147.152.160/27",
						VPCEndpointID: "vpce-3",
					},
				},
			},
			ExpectedInternetGatewayWasCreated: true,
			ExpectedInternetGatewaysAttached:  map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedTagsCreated: map[string][]string{
				"vpc-xyz":          {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
				testmocks.NewIGWID: {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc")},
				"rt-1":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-a")},
				"rt-2":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-b")},
				"rt-3":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-d")},
				"rt-4":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-firewall")},
				"rt-5":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-igw")},
				"rt-6":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-a")},
				"rt-7":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-b")},
				"rt-8":             {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-private-d")},
			},
			ExpectedFirewallPoliciesCreated: []string{"cms-cloud-vpc-xyz-default-fp"},
			ExpectedCreatedPolicyToRuleGroupARNs: map[string][]string{
				"cms-cloud-vpc-xyz-default-fp": {
					"managed-stateful-arn",
					"managed-stateless-arn",
				},
			},
			ExpectedFirewallSubnetAssociationsAdded: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
			ExpectedFirewallsCreated:                map[string]string{"cms-cloud-vpc-xyz-net-fw": "vpc-xyz"},
			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-1",
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-2",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypeFirewall,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-5": {
						RouteTableID:        "rt-5",
						EdgeAssociationType: database.EdgeAssociationTypeIGW,
						Routes: []*database.RouteInfo{
							{
								Destination:   "10.147.152.96/27",
								VPCEndpointID: "vpce-1",
							},
							{
								Destination:   "10.147.152.128/27",
								VPCEndpointID: "vpce-2",
							},
							{
								Destination:   "10.147.152.160/27",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-6": {
						RouteTableID: "rt-6",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-7": {
						RouteTableID: "rt-7",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-8": {
						RouteTableID: "rt-8",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
					RouteTableID:              "rt-5",
					RouteTableAssociationID:   "assoc-4",
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
				},
				FirewallRouteTableID: "rt-4",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PublicRouteTableID:  "rt-1",
						PrivateRouteTableID: "rt-6",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-5",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-8",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
						},
					},
					"us-east-1b": {
						PublicRouteTableID:  "rt-2",
						PrivateRouteTableID: "rt-7",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-9",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
						},
					},
					"us-east-1d": {
						PublicRouteTableID:  "rt-3",
						PrivateRouteTableID: "rt-8",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-7",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-10",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Migrating V1 -> V1Firewall",

			StartState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1ToV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public": {
						RouteTableID: "rt-public",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				PublicRouteTableID: "rt-public",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-public-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-public-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-public-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID: "subnet-firewall-d",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-public"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-a"),
							RouteTableAssociationId: aws.String("assoc-public-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-public-b"),
							RouteTableAssociationId: aws.String("assoc-public-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-public-d"),
							RouteTableAssociationId: aws.String("assoc-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-a"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-a"),
							RouteTableAssociationId: aws.String("assoc-private-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-b"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-b"),
							RouteTableAssociationId: aws.String("assoc-private-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
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
			ExistingSubnetCIDRs: map[string]string{
				"subnet-public-a": "10.147.152.96/27",
				"subnet-public-b": "10.147.152.128/27",
				"subnet-public-d": "10.147.152.160/27",
			},
			ExistingRuleGroups: []*networkfirewall.RuleGroupMetadata{
				{
					Name: aws.String(awsp.ManagedStatefulRuleGroupName),
					Arn:  aws.String("managed-stateful-arn"),
				},
				{
					Name: aws.String(awsp.ManagedStatelessRuleGroupName),
					Arn:  aws.String("managed-stateless-arn"),
				},
				{
					Name: aws.String("cms-cloud-stateful-rg-1"),
					Arn:  aws.String("other-stateful-arn"),
				},
				{
					Name: aws.String("cms-cloud-stateless-rg-1"),
					Arn:  aws.String("other-stateless-arn"),
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1", "rt-2", "rt-3", "rt-4", "rt-5"},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-a"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-2"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-b"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-3"),
					RouteTableId:            aws.String("rt-4"),
					SubnetId:                aws.String("subnet-firewall-d"),
				},
				{
					RouteTableAssociationId: aws.String("assoc-4"),
					RouteTableId:            aws.String("rt-5"),
					GatewayId:               aws.String(testmocks.NewIGWID),
				},
			},
			ExpectedRouteTableAssociationsReplaced: map[string]string{
				"assoc-public-a": "rt-1",
				"assoc-public-b": "rt-2",
				"assoc-public-d": "rt-3",
			},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-1",
					},
				},
				"rt-2": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-2",
					},
				},
				"rt-3": {
					{
						Destination:   "0.0.0.0/0",
						VPCEndpointID: "vpce-3",
					},
				},
				"rt-4": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
				"rt-5": {
					{
						Destination:   "10.147.152.96/27",
						VPCEndpointID: "vpce-1",
					},
					{
						Destination:   "10.147.152.128/27",
						VPCEndpointID: "vpce-2",
					},
					{
						Destination:   "10.147.152.160/27",
						VPCEndpointID: "vpce-3",
					},
				},
			},
			ExpectedTagsCreated: map[string][]string{
				"rt-1":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-a")},
				"rt-2":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-b")},
				"rt-3":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public-d")},
				"rt-4":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-firewall")},
				"rt-5":    {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-igw")},
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedFirewallPoliciesCreated: []string{"cms-cloud-vpc-xyz-default-fp"},
			ExpectedCreatedPolicyToRuleGroupARNs: map[string][]string{
				"cms-cloud-vpc-xyz-default-fp": {
					"managed-stateful-arn",
					"managed-stateless-arn",
				},
			},
			ExpectedFirewallSubnetAssociationsAdded: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
			ExpectedFirewallsCreated:                map[string]string{"cms-cloud-vpc-xyz-net-fw": "vpc-xyz"},
			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1ToV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					// removed later by deleteIncompleteResources
					"rt-public": {
						RouteTableID: "rt-public",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-1",
							},
						},
					},
					"rt-2": {
						RouteTableID: "rt-2",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-2",
							},
						},
					},
					"rt-3": {
						RouteTableID: "rt-3",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-4": {
						RouteTableID: "rt-4",
						SubnetType:   database.SubnetTypeFirewall,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-5": {
						RouteTableID:        "rt-5",
						EdgeAssociationType: database.EdgeAssociationTypeIGW,
						Routes: []*database.RouteInfo{
							{
								Destination:   "10.147.152.96/27",
								VPCEndpointID: "vpce-1",
							},
							{
								Destination:   "10.147.152.128/27",
								VPCEndpointID: "vpce-2",
							},
							{
								Destination:   "10.147.152.160/27",
								VPCEndpointID: "vpce-3",
							},
						},
					},
				},
				// removed later by deleteUnusedResources
				PublicRouteTableID: "rt-public",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
					RouteTableID:              "rt-5",
					RouteTableAssociationID:   "assoc-4",
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
				},
				FirewallRouteTableID: "rt-4",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PublicRouteTableID:  "rt-1",
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-5",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
						},
					},
					"us-east-1b": {
						PublicRouteTableID:  "rt-2",
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-6",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
						},
					},
					"us-east-1d": {
						PublicRouteTableID:  "rt-3",
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-7",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Migrating V1Firewall -> V1",

			StartState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1FirewallToV1,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public-a": {
						RouteTableID: "rt-public-a",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-1",
							},
						},
					},
					"rt-public-b": {
						RouteTableID: "rt-public-b",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-2",
							},
						},
					},
					"rt-public-d": {
						RouteTableID: "rt-public-d",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						SubnetType:   database.SubnetTypeFirewall,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-igw": {
						RouteTableID:        "rt-igw",
						EdgeAssociationType: database.EdgeAssociationTypeIGW,
						Routes: []*database.RouteInfo{
							{
								Destination:   "10.147.152.96/27",
								VPCEndpointID: "vpce-1",
							},
							{
								Destination:   "10.147.152.128/27",
								VPCEndpointID: "vpce-2",
							},
							{
								Destination:   "10.147.152.160/27",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
					RouteTableID:              "rt-igw",
					RouteTableAssociationID:   "assoc-igw",
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
				},
				FirewallRouteTableID: "rt-firewall",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PublicRouteTableID:  "rt-public-a",
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-public-a",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						PublicRouteTableID:  "rt-public-b",
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-public-b",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						PublicRouteTableID:  "rt-public-d",
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-public-d",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-firewall-d",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-public-a"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-a"),
							RouteTableAssociationId: aws.String("assoc-public-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-public-b"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-b"),
							RouteTableAssociationId: aws.String("assoc-public-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-public-d"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-d"),
							RouteTableAssociationId: aws.String("assoc-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-firewall"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-firewall-a"),
							RouteTableAssociationId: aws.String("assoc-firewall-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-firewall-b"),
							RouteTableAssociationId: aws.String("assoc-firewall-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-firewall-d"),
							RouteTableAssociationId: aws.String("assoc-firewall-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-igw"),
					Associations: []*ec2.RouteTableAssociation{
						{
							GatewayId:               aws.String(testmocks.NewIGWID),
							RouteTableAssociationId: aws.String("assoc-4"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-a"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-a"),
							RouteTableAssociationId: aws.String("assoc-private-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-b"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-b"),
							RouteTableAssociationId: aws.String("assoc-private-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
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

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
				NetworkingConfig: database.NetworkingConfig{
					ConnectPublic: true,
				},
			},

			ExpectedTaskStatus:         database.TaskStatusSuccessful,
			ExpectedRouteTablesCreated: []string{"rt-1"},
			ExpectedRoutesAdded: map[string][]*database.RouteInfo{
				"rt-1": {
					{
						Destination:       "0.0.0.0/0",
						InternetGatewayID: testmocks.NewIGWID,
					},
				},
			},
			ExpectedRouteTableAssociationsReplaced: map[string]string{
				"assoc-public-a": "rt-1",
				"assoc-public-b": "rt-1",
				"assoc-public-d": "rt-1",
			},
			ExpectedRouteTableAssociationsRemoved: []string{"assoc-igw"},
			ExpectedTagsCreated: map[string][]string{
				"rt-1": {testmocks.FormatTag("Automated", "true"), testmocks.FormatTag("Name", "test-vpc-public")},
			},
			ExpectedTagsDeleted: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeMigratingV1FirewallToV1,
				RouteTables: map[string]*database.RouteTableInfo{
					// removed later by deleteIncompleteResources
					"rt-public-a": {
						RouteTableID: "rt-public-a",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-1",
							},
						},
					},
					// removed later by deleteIncompleteResources
					"rt-public-b": {
						RouteTableID: "rt-public-b",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-2",
							},
						},
					},
					// removed later by deleteIncompleteResources
					"rt-public-d": {
						RouteTableID: "rt-public-d",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-1": {
						RouteTableID: "rt-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					// removed later by deleteIncompleteResources
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						SubnetType:   database.SubnetTypeFirewall,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					// removed later by deleteIncompleteResources
					"rt-igw": {
						RouteTableID:        "rt-igw",
						EdgeAssociationType: database.EdgeAssociationTypeIGW,
						Routes: []*database.RouteInfo{
							{
								Destination:   "10.147.152.96/27",
								VPCEndpointID: "vpce-1",
							},
							{
								Destination:   "10.147.152.128/27",
								VPCEndpointID: "vpce-2",
							},
							{
								Destination:   "10.147.152.160/27",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				PublicRouteTableID: "rt-1",
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
					// removed later by deleteIncompleteResources
					RouteTableID: "rt-igw",
				},
				// removed later by deleteIncompleteResources
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
				},
				// removed later by deleteIncompleteResources
				FirewallRouteTableID: "rt-firewall",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						// removed later by deleteIncompleteResources
						PublicRouteTableID:  "rt-public-a",
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-1",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							// removed later by deleteIncompleteResources
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						// removed later by deleteIncompleteResources
						PublicRouteTableID:  "rt-public-b",
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-2",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							// removed later by deleteIncompleteResources
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						// removed later by deleteIncompleteResources
						PublicRouteTableID:  "rt-public-d",
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-3",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							// removed later by deleteIncompleteResources
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-firewall-d",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "Firewall VPC: disconnect public. Also, fix incorrect firewall subnet associations",

			StartState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public-a": {
						RouteTableID: "rt-public-a",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-1",
							},
						},
					},
					"rt-public-b": {
						RouteTableID: "rt-public-b",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-2",
							},
						},
					},
					"rt-public-d": {
						RouteTableID: "rt-public-d",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:   "0.0.0.0/0",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						SubnetType:   database.SubnetTypeFirewall,
						Routes: []*database.RouteInfo{
							{
								Destination:       "0.0.0.0/0",
								InternetGatewayID: testmocks.NewIGWID,
							},
						},
					},
					"rt-igw": {
						RouteTableID:        "rt-igw",
						EdgeAssociationType: database.EdgeAssociationTypeIGW,
						Routes: []*database.RouteInfo{
							{
								Destination:   "10.147.152.96/27",
								VPCEndpointID: "vpce-1",
							},
							{
								Destination:   "10.147.152.128/27",
								VPCEndpointID: "vpce-2",
							},
							{
								Destination:   "10.147.152.160/27",
								VPCEndpointID: "vpce-3",
							},
						},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         testmocks.NewIGWID,
					IsInternetGatewayAttached: true,
					RouteTableID:              "rt-igw",
					RouteTableAssociationID:   "assoc-igw",
				},
				Firewall: &database.Firewall{
					// private-b is incorrect, firewall-d is missing
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-private-b"},
				},
				FirewallRouteTableID: "rt-firewall",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PublicRouteTableID:  "rt-public-a",
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-public-a",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						PublicRouteTableID:  "rt-public-b",
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-public-b",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						PublicRouteTableID:  "rt-public-d",
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-public-d",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-firewall-d",
								},
							},
						},
					},
				},
			},
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rt-public-a"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-a"),
							RouteTableAssociationId: aws.String("assoc-public-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-public-b"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-b"),
							RouteTableAssociationId: aws.String("assoc-public-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-public-d"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-public-d"),
							RouteTableAssociationId: aws.String("assoc-public-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-firewall"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-firewall-a"),
							RouteTableAssociationId: aws.String("assoc-firewall-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-firewall-b"),
							RouteTableAssociationId: aws.String("assoc-firewall-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
						{
							SubnetId:                aws.String("subnet-firewall-d"),
							RouteTableAssociationId: aws.String("assoc-firewall-d"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-igw"),
					Associations: []*ec2.RouteTableAssociation{
						{
							GatewayId:               aws.String(testmocks.NewIGWID),
							RouteTableAssociationId: aws.String("assoc-igw"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-a"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-a"),
							RouteTableAssociationId: aws.String("assoc-private-a"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rt-private-b"),
					Associations: []*ec2.RouteTableAssociation{
						{
							SubnetId:                aws.String("subnet-private-b"),
							RouteTableAssociationId: aws.String("assoc-private-b"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
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
			ExistingFirewalls: map[string]string{"cms-cloud-vpc-xyz-net-fw": "vpc-xyz"},
			ExistingFirewallSubnetToEndpoint: map[string]string{
				"subnet-firewall-a": "vpce-existing",
				"subnet-private-b":  "vpce-incorrect",
			},
			ExistingFirewallPolicies: []*networkfirewall.FirewallPolicyMetadata{
				{
					Arn:  aws.String(testmocks.TestARN),
					Name: aws.String("cms-cloud-vpc-xyz-default-fp"),
				},
			},

			TaskConfig: database.UpdateNetworkingTaskData{
				VPCID: "vpc-xyz",
			},

			ExpectedTaskStatus: database.TaskStatusSuccessful,
			ExpectedRoutesDeleted: map[string][]string{
				"rt-public-a": {"0.0.0.0/0"},
				"rt-public-b": {"0.0.0.0/0"},
				"rt-public-d": {"0.0.0.0/0"},
				"rt-firewall": {"0.0.0.0/0"},
			},
			ExpectedRouteTableAssociationsRemoved: []string{"assoc-igw"},
			ExpectedRouteTablesDeleted:            []string{"rt-igw"},
			ExpectedInternetGatewaysDetached:      map[string]string{testmocks.NewIGWID: "vpc-xyz"},
			ExpectedInternetGatewaysDeleted:       []string{testmocks.NewIGWID},
			ExpectedTagsCreated: map[string][]string{
				"vpc-xyz": {testmocks.FormatTag(awsp.FirewallTypeKey, awsp.FirewallTypeValue)},
			},
			ExpectedFirewallSubnetAssociationsAdded:   []string{"subnet-firewall-b", "subnet-firewall-d"},
			ExpectedFirewallSubnetAssociationsRemoved: []string{"subnet-private-b"},
			ExpectedEndState: database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-public-a": {
						RouteTableID: "rt-public-a",
						SubnetType:   database.SubnetTypePublic,
						Routes:       []*database.RouteInfo{},
					},
					"rt-public-b": {
						RouteTableID: "rt-public-b",
						SubnetType:   database.SubnetTypePublic,
						Routes:       []*database.RouteInfo{},
					},
					"rt-public-d": {
						RouteTableID: "rt-public-d",
						SubnetType:   database.SubnetTypePublic,
						Routes:       []*database.RouteInfo{},
					},
					"rt-firewall": {
						RouteTableID: "rt-firewall",
						SubnetType:   database.SubnetTypeFirewall,
						Routes:       []*database.RouteInfo{},
					},
					"rt-private-a": {
						RouteTableID: "rt-private-a",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-b": {
						RouteTableID: "rt-private-b",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-private-d": {
						RouteTableID: "rt-private-d",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				InternetGateway: database.InternetGatewayInfo{
					InternetGatewayID:         "",
					IsInternetGatewayAttached: false,
					RouteTableID:              "",
					RouteTableAssociationID:   "",
				},
				Firewall: &database.Firewall{
					AssociatedSubnetIDs: []string{"subnet-firewall-a", "subnet-firewall-b", "subnet-firewall-d"},
				},
				FirewallRouteTableID: "rt-firewall",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"us-east-1a": {
						PublicRouteTableID:  "rt-public-a",
						PrivateRouteTableID: "rt-private-a",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-a",
									RouteTableAssociationID: "assoc-public-a",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-a",
									RouteTableAssociationID: "assoc-private-a",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-a",
									RouteTableAssociationID: "assoc-firewall-a",
								},
							},
						},
					},
					"us-east-1b": {
						PublicRouteTableID:  "rt-public-b",
						PrivateRouteTableID: "rt-private-b",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-b",
									RouteTableAssociationID: "assoc-public-b",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-b",
									RouteTableAssociationID: "assoc-private-b",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-b",
									RouteTableAssociationID: "assoc-firewall-b",
								},
							},
						},
					},
					"us-east-1d": {
						PublicRouteTableID:  "rt-public-d",
						PrivateRouteTableID: "rt-private-d",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID:                "subnet-public-d",
									RouteTableAssociationID: "assoc-public-d",
								},
							},
							database.SubnetTypePrivate: {
								{
									SubnetID:                "subnet-private-d",
									RouteTableAssociationID: "assoc-private-d",
								},
							},
							database.SubnetTypeFirewall: {
								{
									SubnetID:                "subnet-firewall-d",
									RouteTableAssociationID: "assoc-firewall-d",
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
			task := &testmocks.MockTask{
				ID: 5567,
			}
			vpcKey := string(tc.TaskConfig.AWSRegion) + tc.TaskConfig.VPCID
			mm := &testmocks.MockModelsManager{
				VPCs: map[string]*database.VPC{
					vpcKey: {
						AccountID: "99445",
						ID:        tc.TaskConfig.VPCID,
						Name:      "test-vpc",
						State:     &tc.StartState,
						Region:    tc.TaskConfig.AWSRegion,
					},
				},
			}
			ipcontrol := &testmocks.MockIPControl{}
			ec2svc := &testmocks.MockEC2{
				RouteTables: tc.ExistingRouteTables,
				SubnetCIDRs: tc.ExistingSubnetCIDRs,
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
				RuleGroups:                 tc.ExistingRuleGroups,
			}

			taskContext := &TaskContext{
				Task:          task,
				ModelsManager: mm,
				LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.TaskConfig.VPCID), database.TargetIPControlWrite),
				IPAM:          ipcontrol,
				BaseAWSAccountAccess: &awsp.AWSAccountAccess{
					EC2svc: ec2svc,
					NFsvc:  nfsvc,
				},
				CMSNet: &testmocks.MockCMSNet{},
			}
			tc.TaskConfig.SkipVerify = true // not testing verify for now

			taskContext.performUpdateNetworkingTask(&tc.TaskConfig)

			// Overall task status
			if task.Status != tc.ExpectedTaskStatus {
				t.Fatalf("Incorrect task status. Expected %s but got %s. Last log message: %s", tc.ExpectedTaskStatus, task.Status, task.LastLoggedMessage)
			}

			// Expected AWS calls
			if diff := cmp.Diff(tc.ExpectedRouteTablesCreated, ec2svc.RouteTablesCreated); diff != "" {
				t.Errorf("Expected route tables created did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRouteTablesDeleted, ec2svc.RouteTablesDeleted); diff != "" {
				t.Errorf("Expected route tables deleted did not match actual: \n%s", diff)
			}
			// save some boilerplate in test case specification
			for _, assoc := range tc.ExpectedRouteTableAssociationsCreated {
				assoc.AssociationState = &ec2.RouteTableAssociationState{
					State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
				}
			}
			if diff := cmp.Diff(tc.ExpectedRouteTableAssociationsCreated, ec2svc.RouteTableAssociationsCreated); diff != "" {
				t.Errorf("Expected route table associations created did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRouteTableAssociationsReplaced, ec2svc.RouteTableAssociationsReplaced); diff != "" {
				t.Errorf("Expected route table associations replaced did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRouteTableAssociationsRemoved, ec2svc.RouteTableAssociationsRemoved); diff != "" {
				t.Errorf("Expected route table associations removed did not match actual: \n%s", diff)
			}
			if tc.ExpectedInternetGatewayWasCreated != ec2svc.InternetGatewayWasCreated {
				t.Errorf("Expected InternetGatewayWasCreated=%v but got %v", tc.ExpectedInternetGatewayWasCreated, ec2svc.InternetGatewayWasCreated)
			}
			if diff := cmp.Diff(tc.ExpectedInternetGatewaysAttached, ec2svc.InternetGatewaysAttached); diff != "" {
				t.Errorf("Expected internet gateway attachments did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedEIPsAllocated, ec2svc.EIPsAllocated); diff != "" {
				t.Errorf("Expected EIPs allocated did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedNATGatewaysCreated, ec2svc.NATGatewaysCreated); diff != "" {
				t.Errorf("Expected NAT gateways created did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRoutesAdded, ec2svc.RoutesAdded); diff != "" {
				t.Errorf("Expected routes added did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedRoutesDeleted, ec2svc.RoutesDeleted); diff != "" {
				t.Errorf("Expected routes deleted did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedInternetGatewaysDetached, ec2svc.InternetGatewaysDetached); diff != "" {
				t.Errorf("Expected internet gateways detached did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedInternetGatewaysDeleted, ec2svc.InternetGatewaysDeleted); diff != "" {
				t.Errorf("Expected internet gateways deleted did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedEIPsReleased, ec2svc.EIPsReleased); diff != "" {
				t.Errorf("Expected eips released did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedNATGatewaysDeleted, ec2svc.NATGatewaysDeleted); diff != "" {
				t.Errorf("Expected nat gateways deleted did not match actual: \n%s", diff)
			}

			sortStringsOption := cmpopts.SortSlices(func(x string, y string) bool { return x < y })

			if diff := cmp.Diff(tc.ExpectedTagsCreated, ec2svc.TagsCreated, sortStringsOption); diff != "" {
				t.Errorf("Expected tags created did not match actual: \n%s", diff)
			}
			if diff := cmp.Diff(tc.ExpectedTagsDeleted, ec2svc.TagsDeleted, sortStringsOption); diff != "" {
				t.Errorf("Expected tags deleted did not match actual: \n%s", diff)
			}

			if tc.StartState.VPCType.HasFirewall() {
				if diff := cmp.Diff(tc.ExpectedFirewallPoliciesCreated, nfsvc.FirewallPoliciesCreated, sortStringsOption); diff != "" {
					t.Errorf("Expected firewall policies created did not match actual: \n%s", diff)
				}
				if diff := cmp.Diff(tc.ExpectedCreatedPolicyToRuleGroupARNs, nfsvc.CreatedPolicyToRuleGroupARNs); diff != "" {
					t.Errorf("Expected rule group ARNs configured for created firewall policies did not match actual: \n%s", diff)
				}
				if diff := cmp.Diff(tc.ExpectedFirewallSubnetAssociationsAdded, nfsvc.SubnetAssociationsAdded, sortStringsOption); diff != "" {
					t.Errorf("Expected firewall subnet associations added did not match actual: \n%s", diff)
				}
				if diff := cmp.Diff(tc.ExpectedFirewallSubnetAssociationsRemoved, nfsvc.SubnetAssociationsRemoved, sortStringsOption); diff != "" {
					t.Errorf("Expected firewall subnet associations removed did not match actual: \n%s", diff)
				}
				if diff := cmp.Diff(tc.ExpectedFirewallsCreated, nfsvc.FirewallsCreated, sortStringsOption); diff != "" {
					t.Errorf("Expected firewalls created did not match actual: \n%s", diff)
				}
			}

			// Expected database updates
			if diff := cmp.Diff(&tc.ExpectedEndState, mm.VPCs[vpcKey].State); diff != "" {
				t.Fatalf("Expected end state did not match state saved to database: \n%s", diff)
			}
		})
	}
}
