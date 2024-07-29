package main

import (
	"fmt"
	"log"
	"reflect"
	"sort"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/google/go-cmp/cmp"
)

type peeringConnectionTestCase struct {
	Name string

	// In
	VPCs                    map[string]*database.VPC
	TaskData                *database.UpdateNetworkingTaskData
	PeeringConnections      []*ec2.VpcPeeringConnection
	PeeringConnectionStatus map[string]string
	SubnetCIDRs             map[string]map[string]string // account ID -> (subnet ID -> CIDR)
	ExpectedError           error

	// Expected calls
	RoutesAdded                 map[string][]*database.RouteInfo // route table ID -> new route
	RoutesDeleted               map[string][]string              // route table ID -> removed destinations
	PeeringConnectionIDsDeleted []string
	PeeringConnectionsCreated   []*ec2.VpcPeeringConnection

	// Out
	EndState *database.VPCState
}

func TestHandlePeeringConnections(t *testing.T) {
	vpcID := "vpc-123abc"
	otherVPCID := "vpc-456xyz"
	otherAccountID := "555"
	otherRegion := database.Region("us-west-2")
	otherRegionStr := string(otherRegion)
	accountID := "123789"
	region := database.Region("us-east-1")
	regionStr := string(region)
	var logger = &testLogger{}

	testCases := []*peeringConnectionTestCase{
		{
			Name: "basic add same account",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account, accepter has multiple private subnet groups",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID:  "subnet-p-abc",
											GroupName: "private",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID:  "subnet-p-123",
											GroupName: "private",
										},
										{
											SubnetID:  "subnet-p-234",
											GroupName: "private-2",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID:  "subnet-p-456",
											GroupName: "private",
										},
										{
											SubnetID:  "subnet-p-567",
											GroupName: "private-2",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-234": "10.1.14.0/28",
					"subnet-p-456": "10.1.13.0/28",
					"subnet-p-567": "10.1.15.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.14.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.15.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.14.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.15.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.14.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.15.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.14.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.15.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID:  "subnet-p-abc",
									GroupName: "private",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic Legacy add same account, shared RT",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						VPCType: database.VPCTypeLegacy,
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-app": {
								RouteTableID: "rt-app",
								SubnetType:   database.SubnetTypeApp,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeApp: {
										{
											SubnetID:           "subnet-abc",
											CustomRouteTableID: "rt-app",
											GroupName:          "app1",
										},
									},
								},
							},
							"az2": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeApp: {
										{
											SubnetID:           "subnet-xyz",
											CustomRouteTableID: "rt-app",
											GroupName:          "app1",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-123": {
								RouteTableID: "rt-123",
								SubnetType:   database.SubnetTypeApp,
							},
							"rt-456": {
								RouteTableID: "rt-456",
								SubnetType:   database.SubnetTypeApp,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeApp: {
										{
											SubnetID:           "subnet-123",
											CustomRouteTableID: "rt-123",
											GroupName:          "app2",
										},
									},
								},
							},
							"az2": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeApp: {
										{
											SubnetID:           "subnet-456",
											CustomRouteTableID: "rt-456",
											GroupName:          "app2",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:                 true,
							OtherVPCID:                  otherVPCID,
							OtherVPCRegion:              region,
							ConnectPrivate:              true,
							ConnectSubnetGroups:         []string{"app1"},
							OtherVPCConnectPrivate:      false,
							OtherVPCConnectSubnetGroups: []string{"app2"},
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-abc": "10.1.2.0/28",
					"subnet-xyz": "10.1.3.0/28",
					"subnet-123": "10.1.12.0/28",
					"subnet-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-app": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				VPCType: database.VPCTypeLegacy,
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-app": {
						RouteTableID: "rt-app",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-abc",
									CustomRouteTableID: "rt-app",
									GroupName:          "app1",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-xyz",
									CustomRouteTableID: "rt-app",
									GroupName:          "app1",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account - called by accepter",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            false,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      otherVPCID,
						RequesterRegion:     region,
						AccepterVPCID:       vpcID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account, error from accepter with individual private subnet group configured",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID:  "subnet-p-123",
											GroupName: "private",
										},
										{
											SubnetID:  "subnet-p-234",
											GroupName: "private-2",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID:  "subnet-p-456",
											GroupName: "private",
										},
										{
											SubnetID:  "subnet-p-567",
											GroupName: "private-2",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:                 false,
							OtherVPCID:                  otherVPCID,
							OtherVPCRegion:              region,
							ConnectPrivate:              true,
							OtherVPCConnectSubnetGroups: []string{"private-2"},
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-def": "10.1.4.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-234": "10.1.14.0/28",
					"subnet-p-456": "10.1.13.0/28",
					"subnet-p-567": "10.1.15.0/28",
				},
			},
			ExpectedError: fmt.Errorf("Error validating peering connection subnet groups for %s: Peering connections for Private subnets can't be configured individually", otherVPCID),

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesAdded:               map[string][]*database.RouteInfo{},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account, error from requester with public subnet group configured",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PublicRouteTableID: "rt-pub",
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-pub": {
								RouteTableID: "rt-pub",
								SubnetType:   database.SubnetTypePublic,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
									database.SubnetTypePublic: {
										{
											SubnetID:  "subnet-pub-def",
											GroupName: "public",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
									database.SubnetTypePublic: {
										{
											SubnetID:  "subnet-pub-uvw",
											GroupName: "public",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            false,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							OtherVPCConnectPrivate: true,
							ConnectSubnetGroups:    []string{"public"},
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-def": "10.1.4.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},
			ExpectedError: fmt.Errorf("Error validating peering connection subnet groups for %s: Peering connections for Public subnet groups are not allowed", vpcID),

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesAdded:               map[string][]*database.RouteInfo{},

			// Out
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-123",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-456",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-456",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account, error from requester with firewall subnet group configured",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						VPCType:              database.VPCTypeV1Firewall,
						FirewallRouteTableID: "rt-fw",
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-fw": {
								RouteTableID: "rt-fw",
								SubnetType:   database.SubnetTypeFirewall,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeFirewall: {
										{
											SubnetID:  "subnet-fw-1",
											GroupName: "firewall",
										},
									},
								},
							},
							"az2": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeFirewall: {
										{
											SubnetID:  "subnet-fw-2",
											GroupName: "firewall",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-0-fww=": {
								RouteTableID: "rt-o-fw",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeFirewall: {
										{
											SubnetID:  "subnet-o-fw-1",
											GroupName: "firewall",
										},
									},
								},
							},
							"az2": {
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypeFirewall: {
										{
											SubnetID:  "subnet-o-fw-2",
											GroupName: "firewall",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            false,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							OtherVPCConnectPrivate: false,
							ConnectSubnetGroups:    []string{"firewall"},
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-def": "10.1.4.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},
			ExpectedError: fmt.Errorf("Error validating peering connection subnet groups for %s: Peering connections for Firewall subnet groups are not allowed", vpcID),

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesAdded:               map[string][]*database.RouteInfo{},

			// Out
			EndState: &database.VPCState{
				VPCType:              database.VPCTypeV1Firewall,
				FirewallRouteTableID: "rt-fw",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-fw": {
						RouteTableID: "rt-fw",
						SubnetType:   database.SubnetTypeFirewall,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-fw-1",
									GroupName: "firewall",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeFirewall: {
								{
									SubnetID:  "subnet-fw-2",
									GroupName: "firewall",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account - already created but some routes missing or extra",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      vpcID,
								RequesterRegion:     region,
								AccepterVPCID:       otherVPCID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          true,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
									{
										Destination:         "0.0.0.0/0",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "active",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesDeleted: map[string][]string{
				"rt-p-abc": {"0.0.0.0/0"},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-xyz",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add different account/region - already created but some routes missing - run by accepter",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      otherVPCID,
								RequesterRegion:     otherRegion,
								AccepterVPCID:       vpcID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          true,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(otherRegionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    otherRegion,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            false,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         otherRegion,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &otherRegionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "active",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      otherVPCID,
						RequesterRegion:     otherRegion,
						AccepterVPCID:       vpcID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-xyz",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account - already created but not accepted",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      vpcID,
								RequesterRegion:     region,
								AccepterVPCID:       otherVPCID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          false,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "pending-acceptance",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-xyz",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-xyz",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add same account - reverse direction",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      otherVPCID,
								RequesterRegion:     region,
								AccepterVPCID:       vpcID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          true,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         region,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "active",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionIDsDeleted: []string{"pcx-xyz"},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic remove",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      vpcID,
								RequesterRegion:     region,
								AccepterVPCID:       otherVPCID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          true,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "active",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated:   []*ec2.VpcPeeringConnection{},
			PeeringConnectionIDsDeleted: []string{"pcx-xyz"},
			RoutesDeleted: map[string][]string{
				"rt-p-abc": {"10.1.12.0/28", "10.1.13.0/28"},
				"rt-p-xyz": {"10.1.12.0/28", "10.1.13.0/28"},
			},

			// Out
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic remove: firewall VPC",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						PeeringConnections: []*database.PeeringConnection{
							{
								RequesterVPCID:      vpcID,
								RequesterRegion:     region,
								AccepterVPCID:       otherVPCID,
								AccepterRegion:      region,
								PeeringConnectionID: "pcx-xyz",
								IsAccepted:          true,
							},
						},
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
								Routes: []*database.RouteInfo{
									{
										Destination:         "10.1.12.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
									{
										Destination:         "10.1.13.0/28",
										PeeringConnectionID: "pcx-xyz",
									},
								},
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{},
				},
			},
			PeeringConnections: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-xyz"),
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			PeeringConnectionStatus: map[string]string{
				"pcx-xyz": "active",
			},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated:   []*ec2.VpcPeeringConnection{},
			PeeringConnectionIDsDeleted: []string{"pcx-xyz"},
			RoutesDeleted: map[string][]string{
				"rt-p-abc": {"10.1.12.0/28", "10.1.13.0/28"},
				"rt-p-xyz": {"10.1.12.0/28", "10.1.13.0/28"},
			},

			// Out
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "basic add different account/region",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
								},
							},
						},
					},
				},
				(otherRegionStr + otherVPCID): {
					AccountID: otherAccountID,
					ID:        otherVPCID,
					Region:    otherRegion,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:            true,
							OtherVPCID:             otherVPCID,
							OtherVPCRegion:         otherRegion,
							ConnectPrivate:         true,
							OtherVPCConnectPrivate: true,
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc": "10.1.2.0/28",
					"subnet-p-xyz": "10.1.3.0/28",
				},
				otherAccountID: {
					"subnet-p-123": "10.1.12.0/28",
					"subnet-p-456": "10.1.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &otherAccountID,
						Region:  &otherRegionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.1.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.1.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      otherRegion,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.1.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.1.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
						},
					},
				},
			},
		},

		{
			Name: "same account, with subnet groups",

			// In
			VPCs: map[string]*database.VPC{
				(regionStr + vpcID): {
					AccountID: accountID,
					ID:        vpcID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-abc": {
								RouteTableID: "rt-p-abc",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-app-abc": {
								RouteTableID: "rt-app-abc",
								SubnetType:   database.SubnetTypeApp,
							},
							"rt-data-abc": {
								RouteTableID: "rt-data-abc",
								SubnetType:   database.SubnetTypeData,
							},
							"rt-p-xyz": {
								RouteTableID: "rt-p-xyz",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-app-xyz": {
								RouteTableID: "rt-app-xyz",
								SubnetType:   database.SubnetTypeApp,
							},
							"rt-data-xyz": {
								RouteTableID: "rt-data-xyz",
								SubnetType:   database.SubnetTypeData,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-abc",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-abc",
										},
									},
									database.SubnetTypeApp: {
										{
											CustomRouteTableID: "rt-app-abc",
											GroupName:          "app1",
											SubnetID:           "subnet-app-abc",
										},
									},
									database.SubnetTypeData: {
										{
											CustomRouteTableID: "rt-data-abc",
											GroupName:          "data1",
											SubnetID:           "subnet-data-abc",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-xyz",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-xyz",
										},
									},
									database.SubnetTypeApp: {
										{
											GroupName:          "app1",
											CustomRouteTableID: "rt-app-xyz",
											SubnetID:           "subnet-app-xyz",
										},
									},
									database.SubnetTypeData: {
										{
											GroupName:          "data1",
											CustomRouteTableID: "rt-data-xyz",
											SubnetID:           "subnet-data-xyz",
										},
									},
								},
							},
						},
					},
				},
				(regionStr + otherVPCID): {
					AccountID: accountID,
					ID:        otherVPCID,
					Region:    region,
					State: &database.VPCState{
						RouteTables: map[string]*database.RouteTableInfo{
							"rt-p-123": {
								RouteTableID: "rt-p-123",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-app-123": {
								RouteTableID: "rt-app-123",
								SubnetType:   database.SubnetTypeApp,
							},
							"rt-data-123": {
								RouteTableID: "rt-data-123",
								SubnetType:   database.SubnetTypeData,
							},
							"rt-p-456": {
								RouteTableID: "rt-p-456",
								SubnetType:   database.SubnetTypePrivate,
							},
							"rt-app-456": {
								RouteTableID: "rt-app-456",
								SubnetType:   database.SubnetTypeApp,
							},
							"rt-data-456": {
								RouteTableID: "rt-data-456",
								SubnetType:   database.SubnetTypeData,
							},
						},
						AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
							"az1": {
								PrivateRouteTableID: "rt-p-123",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-123",
										},
									},
									database.SubnetTypeApp: {
										{
											GroupName:          "app2",
											CustomRouteTableID: "rt-app-123",
											SubnetID:           "subnet-app-123",
										},
									},
									database.SubnetTypeData: {
										{
											GroupName:          "data2",
											CustomRouteTableID: "rt-data-123",
											SubnetID:           "subnet-data-123",
										},
									},
								},
							},
							"az2": {
								PrivateRouteTableID: "rt-p-456",
								Subnets: map[database.SubnetType][]*database.SubnetInfo{
									database.SubnetTypePrivate: {
										{
											SubnetID: "subnet-p-456",
										},
									},
									database.SubnetTypeApp: {
										{
											GroupName:          "app2",
											CustomRouteTableID: "rt-app-456",
											SubnetID:           "subnet-app-456",
										},
									},
									database.SubnetTypeData: {
										{
											GroupName:          "data2",
											CustomRouteTableID: "rt-data-456",
											SubnetID:           "subnet-data-456",
										},
									},
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: region,
				NetworkingConfig: database.NetworkingConfig{
					PeeringConnections: []*database.PeeringConnectionConfig{
						{
							IsRequester:                 true,
							OtherVPCID:                  otherVPCID,
							OtherVPCRegion:              region,
							ConnectPrivate:              true,
							ConnectSubnetGroups:         []string{"app1"},
							OtherVPCConnectPrivate:      false,
							OtherVPCConnectSubnetGroups: []string{"data2", "app2"},
						},
					},
				},
			},
			PeeringConnections:      []*ec2.VpcPeeringConnection{},
			PeeringConnectionStatus: map[string]string{},
			SubnetCIDRs: map[string]map[string]string{
				accountID: {
					"subnet-p-abc":    "10.1.2.0/28",
					"subnet-p-xyz":    "10.1.3.0/28",
					"subnet-p-123":    "10.1.12.0/28",
					"subnet-p-456":    "10.1.13.0/28",
					"subnet-app-abc":  "10.2.2.0/28",
					"subnet-app-xyz":  "10.2.3.0/28",
					"subnet-app-123":  "10.2.12.0/28",
					"subnet-app-456":  "10.2.13.0/28",
					"subnet-data-abc": "10.3.2.0/28",
					"subnet-data-xyz": "10.3.3.0/28",
					"subnet-data-123": "10.3.12.0/28",
					"subnet-data-456": "10.3.13.0/28",
				},
			},

			// Expected calls
			PeeringConnectionsCreated: []*ec2.VpcPeeringConnection{
				{
					VpcPeeringConnectionId: aws.String("pcx-1"),
					RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &vpcID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
					AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
						VpcId:   &otherVPCID,
						OwnerId: &accountID,
						Region:  &regionStr,
					},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-p-abc": {
					{
						Destination:         "10.2.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.2.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-app-abc": {
					{
						Destination:         "10.2.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.2.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-p-xyz": {
					{
						Destination:         "10.2.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.2.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
				"rt-app-xyz": {
					{
						Destination:         "10.2.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.2.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.12.0/28",
						PeeringConnectionID: "pcx-1",
					},
					{
						Destination:         "10.3.13.0/28",
						PeeringConnectionID: "pcx-1",
					},
				},
			},

			// Out
			EndState: &database.VPCState{
				PeeringConnections: []*database.PeeringConnection{
					{
						RequesterVPCID:      vpcID,
						RequesterRegion:     region,
						AccepterVPCID:       otherVPCID,
						AccepterRegion:      region,
						PeeringConnectionID: "pcx-1",
						IsAccepted:          true,
					},
				},
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-p-abc": {
						RouteTableID: "rt-p-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.2.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.2.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-app-abc": {
						RouteTableID: "rt-app-abc",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.2.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.2.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-data-abc": {
						RouteTableID: "rt-data-abc",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-p-xyz": {
						RouteTableID: "rt-p-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.2.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.2.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-app-xyz": {
						RouteTableID: "rt-app-xyz",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:         "10.2.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.2.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.12.0/28",
								PeeringConnectionID: "pcx-1",
							},
							{
								Destination:         "10.3.13.0/28",
								PeeringConnectionID: "pcx-1",
							},
						},
					},
					"rt-data-xyz": {
						RouteTableID: "rt-data-xyz",
						SubnetType:   database.SubnetTypeData,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-p-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									GroupName:          "app1",
									SubnetID:           "subnet-app-abc",
									CustomRouteTableID: "rt-app-abc",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "data1",
									SubnetID:           "subnet-data-abc",
									CustomRouteTableID: "rt-data-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-p-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-p-xyz",
								},
							},
							database.SubnetTypeApp: {
								{
									GroupName:          "app1",
									CustomRouteTableID: "rt-app-xyz",
									SubnetID:           "subnet-app-xyz",
								},
							},
							database.SubnetTypeData: {
								{
									GroupName:          "data1",
									SubnetID:           "subnet-data-xyz",
									CustomRouteTableID: "rt-data-xyz",
								},
							},
						},
					},
				},
			},
		},
	}
	speedUpTime()
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			log.Printf("\n---------- Running test case %q ----------", tc.Name)
			mm := &testmocks.MockModelsManager{
				VPCs:       tc.VPCs,
				TestRegion: testRegion,
			}
			routeTablesInEC2 := []*ec2.RouteTable{}
			for _, vpc := range tc.VPCs {
				for _, rt := range vpc.State.RouteTables {
					routeTablesInEC2 = append(routeTablesInEC2, &ec2.RouteTable{
						RouteTableId: aws.String(rt.RouteTableID),
					})
				}
			}
			ec2svc := &testmocks.MockEC2{
				AccountID:                 accountID,
				Region:                    string(region),
				PeeringConnections:        &tc.PeeringConnections,
				PeeringConnectionsCreated: &[]*ec2.VpcPeeringConnection{},
				PeeringConnectionStatus:   tc.PeeringConnectionStatus,
				SubnetCIDRs:               tc.SubnetCIDRs[accountID],
				RouteTables:               routeTablesInEC2,
			}
			ctx := &awsp.Context{
				AWSAccountAccess: &awsp.AWSAccountAccess{
					EC2svc: ec2svc,
				},
				Logger: logger,
				Clock:  testClock,
			}
			vpc := tc.VPCs[string(region)+vpcID]
			vpcWriter := &testmocks.MockVPCWriter{
				MM:     mm,
				VPCID:  vpc.ID,
				Region: vpc.Region,
			}
			targets := []database.Target{}
			for _, vpc := range tc.VPCs {
				targets = append(targets, database.TargetVPC(vpc.ID))
			}
			lockSet := database.GetFakeLockSet(targets...)
			otherContexts := make(map[string]*awsp.Context)
			var getContext = func(region database.Region, accountID string) (*awsp.Context, error) {
				if ctx, ok := otherContexts[accountID]; ok {
					return ctx, nil
				}
				otherEC2 := &testmocks.MockEC2{
					AccountID:                 accountID,
					Region:                    string(region),
					PeeringConnections:        ec2svc.PeeringConnections,
					PeeringConnectionsCreated: ec2svc.PeeringConnectionsCreated,
					PeeringConnectionStatus:   ec2svc.PeeringConnectionStatus,
					SubnetCIDRs:               tc.SubnetCIDRs[accountID],
					RouteTables:               routeTablesInEC2,
				}
				ctx := &awsp.Context{
					AWSAccountAccess: &awsp.AWSAccountAccess{
						EC2svc: otherEC2,
					},
					Logger: logger,
					Clock:  testClock,
				}
				return ctx, nil
			}
			pcs, err := handlePeeringConnections(
				lockSet,
				ctx,
				vpc,
				vpcWriter,
				tc.TaskData,
				mm,
				getContext)
			if err != nil {
				if tc.ExpectedError != nil && tc.ExpectedError.Error() == err.Error() {
					log.Printf("Got expected error: %s", err)
					return
				} else {
					t.Fatalf("PCX Test case %q failed to handle peering connections: %s", tc.Name, err)
				}
			} else if tc.ExpectedError != nil {
				t.Fatalf("TGW test case %q expected an error, but none was returned: %s", tc.Name, err)
			}
			for azName, az := range vpc.State.AvailabilityZones {
				for subnetType, subnets := range az.Subnets {
					for _, pc := range pcs {
						if subnetType == database.SubnetTypePrivate {
							privateRTInfo, ok := vpc.State.RouteTables[az.PrivateRouteTableID]
							if !ok {
								t.Fatalf("Private route table %s missing from state", az.PrivateRouteTableID)
							}
							if pc.Config.ConnectPrivate {
								err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, pc.OtherVPCCIDRs, pc.State.PeeringConnectionID, privateRTInfo)
								if err != nil {
									t.Fatalf("PCX Test case %q failed to update private routes for az %s: %s", tc.Name, azName, err)
								}
							} else {
								err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, []string{}, pc.State.PeeringConnectionID, privateRTInfo)
								if err != nil {
									t.Fatalf("PCX Test case %q failed to update private routes for az %s: %s", tc.Name, azName, err)
								}
							}
						} else {
							for _, subnet := range subnets {
								customRTInfo, ok := vpc.State.RouteTables[subnet.CustomRouteTableID]
								if !ok {
									t.Fatalf("Custom route table %s missing from state", subnet.CustomRouteTableID)
								}
								if stringInSlice(subnet.SubnetID, pc.SubnetIDs) {
									err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, pc.OtherVPCCIDRs, pc.State.PeeringConnectionID, customRTInfo)
									if err != nil {
										t.Fatalf("PCX Test case %q failed to update routes for subnet %s: %s", tc.Name, subnet.SubnetID, err)
									}
								} else {
									err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, []string{}, pc.State.PeeringConnectionID, customRTInfo)
									if err != nil {
										t.Fatalf("PCX Test case %q failed to update routes for subnet %s: %s", tc.Name, subnet.SubnetID, err)
									}
								}
							}
						}
					}
				}
			}
			if !reflect.DeepEqual(tc.PeeringConnectionsCreated, *ec2svc.PeeringConnectionsCreated) {
				t.Fatalf("PCX Test case %q: Wrong peering connections created. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.PeeringConnectionsCreated, ec2svc.PeeringConnectionsCreated)
			}
			for _, pc := range append(tc.PeeringConnections, tc.PeeringConnectionsCreated...) {
				if stringInSlice(*pc.VpcPeeringConnectionId, tc.PeeringConnectionIDsDeleted) {
					continue
				}
				status := tc.PeeringConnectionStatus[*pc.VpcPeeringConnectionId]
				if status != "active" {
					t.Fatalf("PCX Test case %q: Incorrect status %q for peering connection %q. May have failed to accept or wait.", tc.Name, status, *pc.VpcPeeringConnectionId)
				}
			}
			if !reflect.DeepEqual(tc.PeeringConnectionIDsDeleted, ec2svc.PeeringConnectionIDsDeleted) {
				t.Fatalf("PCX Test case %q: Wrong peering connections deleted. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.PeeringConnectionIDsDeleted, ec2svc.PeeringConnectionIDsDeleted)
			}

			for _, id := range tc.PeeringConnectionIDsDeleted {
				status := tc.PeeringConnectionStatus[id]
				if status != "deleted" {
					t.Fatalf("PCX Test case %q: Incorrect status %q for peering connection %q. May have failed to wait after deleting.", tc.Name, status, id)
				}
			}
			for _, routes := range ec2svc.RoutesAdded {
				sort.Sort(Routes(routes))
			}
			if !reflect.DeepEqual(tc.RoutesAdded, ec2svc.RoutesAdded) {
				t.Fatalf("PCX Test case %q: Wrong routes added. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.RoutesAdded, ec2svc.RoutesAdded)
			}
			if !reflect.DeepEqual(tc.RoutesDeleted, ec2svc.RoutesDeleted) {
				t.Fatalf("PCX Test case %q: Wrong routes deleted. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.RoutesDeleted, ec2svc.RoutesDeleted)
			}
			endVPC, err := mm.GetVPC(region, vpcID)
			if err != nil {
				t.Fatalf("PCX Test case %q: Error getting end state: %s", tc.Name, err)
			}
			for _, rt := range endVPC.State.RouteTables {
				sort.Sort(Routes(rt.Routes))
			}

			if diff := cmp.Diff(tc.EndState, endVPC.State); diff != "" {
				t.Fatalf("Expected end state did not match state in database: \n%s", diff)
			}
		})
	}
}
