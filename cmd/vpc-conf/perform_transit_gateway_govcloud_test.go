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
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/google/go-cmp/cmp"
)

type transitGatewayAttachmentGovCloudTestCase struct {
	Name   string
	Region *string

	// In
	StartState                     *database.VPCState
	TaskData                       *database.UpdateNetworkingTaskData
	AllMTGAs                       []*database.ManagedTransitGatewayAttachment
	ResourceShares                 []*database.TransitGatewayResourceShare
	PLResourceShares               []*PLResourceShare
	ResourceSharePrincipals        map[string]map[string][]string // account ID -> share ID -> account IDs
	TransitGatewayStatus           map[string]string
	TransitGatewayAttachmentStatus map[string]string
	TransitGatewaysAutoAccept      bool
	UnsharedPLID                   string
	ExpectedError                  error

	// Expected calls
	RoutesAdded               map[string][]*database.RouteInfo // route table ID -> new route
	RoutesDeleted             map[string][]string              // route table ID -> removed destinations
	AttachmentIDsDeleted      []string
	AttachmentsUpdated        map[string][]string // attachment ID -> subnet IDs
	AttachmentsCreated        []*ec2.TransitGatewayVpcAttachment
	SharesUpdated             map[string]map[string][]string // account ID -> share id -> principals added
	TransitGatewayTagsCreated map[string][]string            // attachment ID -> [key=value]

	// Out
	EndState *database.VPCState
}

//type StringPointerSlice []*string

//func (p StringPointerSlice) Len() int           { return len(p) }
//func (p StringPointerSlice) Less(i, j int) bool { return *p[i] < *p[j] }
//func (p StringPointerSlice) Swap(i, j int)      { *p[i], *p[j] = *p[j], *p[i] }

//type Routes []*database.RouteInfo

//func (p Routes) Len() int           { return len(p) }
//func (p Routes) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
//func (p Routes) Less(i, j int) bool { return p[i].Destination < p[j].Destination }

//func subnetTypeInSlice(t database.SubnetType, slice []database.SubnetType) bool {
//	for _, st := range slice {
//		if t == st {
//			return true
//		}
//	}
//	return false
//}

func TestHandleTransitGatewayGovCloudAttachments(t *testing.T) {
	vpcName := "test-vpc"
	vpcID := "vpc-123abc"
	accountID := "123789"
	var logger = &testLogger{}

	testCases := []*transitGatewayAttachmentGovCloudTestCase{
		{
			Name: "Basic add without resource share",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-pub": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without resource share: firewall VPC",
			StartState: &database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub-1": {
						RouteTableID: "rt-pub-1",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-pub-2": {
						RouteTableID: "rt-pub-2",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						PublicRouteTableID:  "rt-pub-1",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						PublicRouteTableID:  "rt-pub-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub-1": {
						RouteTableID: "rt-pub-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-pub-2": {
						RouteTableID: "rt-pub-2",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PublicRouteTableID:  "rt-pub-1",
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PublicRouteTableID:  "rt-pub-2",
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-pub-1": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-pub-2": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without TGW share and with PL not already shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without TGW share and with a mix of CIDR and PL not already shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn", "10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without TGW share and with multiple PLs on the same share, not already shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "pl-opq",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "pl-opq",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn", "pl-opq"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn", "pl-opq"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "pl-opq",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "pl-opq",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without TGW share and with a configured PL lacking a manually created RAM share",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			UnsharedPLID: "pl-lmn",
		},
		{
			Name: "Basic add without TGW share and with one shared PL and another lacking a manually created RAM share",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			UnsharedPLID: "pl-lmn",
		},
		{
			Name: "Basic add without TGW share and with PL missing from an existing share",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn", "pl-qrs"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-qrs"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			UnsharedPLID: "pl-lmn",
		},
		{
			Name: "Basic add without TGW share and with PL already shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add without TGW share and with multiple PL resource shares not yet shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "pl-qrs",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn", "pl-qrs"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn"},
					ResourceShareID: "rs-456789",
				},
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-qrs"},
					ResourceShareID: "rs-987654",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {},
					"rs-987654": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
					"rs-987654": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "pl-qrs",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "pl-qrs",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic Legacy add, shared RT and no resource share",
			StartState: &database.VPCState{
				VPCType: database.VPCTypeLegacy,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-trans": {
						RouteTableID: "rt-trans",
						SubnetType:   database.SubnetTypeTransitive,
					},
					"rt-data": {
						RouteTableID: "rt-data",
						SubnetType:   database.SubnetTypeData,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID:           "subnet-abc",
									CustomRouteTableID: "rt-trans",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:           "subnet-def",
									CustomRouteTableID: "rt-data",
								},
								{
									SubnetID:           "subnet-ghi",
									CustomRouteTableID: "rt-data",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID:           "subnet-123",
									CustomRouteTableID: "rt-trans",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				VPCType: database.VPCTypeLegacy,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-trans": {
						RouteTableID: "rt-trans",
						SubnetType:   database.SubnetTypeTransitive,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-data": {
						RouteTableID: "rt-data",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID:           "subnet-abc",
									CustomRouteTableID: "rt-trans",
								},
							},
							database.SubnetTypeData: {
								{
									SubnetID:           "subnet-def",
									CustomRouteTableID: "rt-data",
								},
								{
									SubnetID:           "subnet-ghi",
									CustomRouteTableID: "rt-data",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID:           "subnet-123",
									CustomRouteTableID: "rt-trans",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypeTransitive, database.SubnetTypeData},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-trans": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-data": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add with resource share already shared",
			StartState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        "987",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-010203",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				"987": {
					"rs-010203": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
		},
		{
			Name: "Basic add with resource share not already shared and PL not already shared",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-lmn",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-xyz",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-789",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"pl-lmn"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        "987",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-010203",
				},
			},
			PLResourceShares: []*PLResourceShare{
				{
					AccountID:       prefixListAccountIDGovCloud,
					Region:          testGovRegion,
					PLIDs:           []string{"pl-lmn"},
					ResourceShareID: "rs-456789",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				"987": {
					"rs-010203": {"666"},
				},
				prefixListAccountIDGovCloud: {
					"rs-456789": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				"987": {
					"rs-010203": {accountID},
				},
				prefixListAccountIDGovCloud: {
					"rs-456789": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-xyz": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "pl-lmn",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add with resource share not already shared",
			StartState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        "987",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-010203",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				"987": {
					"rs-010203": {"666"},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				"987": {
					"rs-010203": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
		},
		{
			Name: "Legacy Basic add with resource share not already shared",
			StartState: &database.VPCState{
				VPCType: database.VPCTypeLegacy,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				VPCType: database.VPCTypeLegacy,
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypeTransitive: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypeTransitive},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        "987",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-010203",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				"987": {
					"rs-010203": {"666"},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				"987": {
					"rs-010203": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
		},
		{
			Name: "Missing subnet",
			StartState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
					"az3": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-xyz",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz"},
					},
				},
			},
			EndState: &database.VPCState{
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
					"az3": {
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-xyz",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz", "subnet-123"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsUpdated: map[string][]string{
				testmocks.TestAttachmentName("tgw-123"): {"subnet-123"},
			},
		},
		{
			Name: "Removed from config",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz"},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: nil,
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: nil,
				},
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-123")},
			RoutesDeleted: map[string][]string{
				"rt-123": {"10.0.0.0/8"},
				"rt-abc": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Removed from config: firewall VPC with public TGW routes",
			StartState: &database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub-1": {
						RouteTableID: "rt-pub-1",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-pub-2": {
						RouteTableID: "rt-pub-2",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PublicRouteTableID: "rt-pub-1",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-pub-1",
								},
							},
						},
					},
					"az2": {
						PublicRouteTableID: "rt-pub-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-pub-2",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz"},
					},
				},
			},
			EndState: &database.VPCState{
				VPCType: database.VPCTypeV1Firewall,
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub-1": {
						RouteTableID: "rt-pub-1",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-pub-2": {
						RouteTableID: "rt-pub-2",
						SubnetType:   database.SubnetTypePublic,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PublicRouteTableID: "rt-pub-1",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-pub-1",
								},
							},
						},
					},
					"az2": {
						PublicRouteTableID: "rt-pub-2",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-pub-2",
								},
							},
						},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: nil,
				},
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePublic},
				},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-123")},
			RoutesDeleted: map[string][]string{
				"rt-pub-1": {"10.0.0.0/8"},
				"rt-pub-2": {"10.0.0.0/8"},
			},
		},
		{
			Name: "MTGA with PL route removed from config",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-efg",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "pl-efg",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz"},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: nil,
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: nil,
				},
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-123")},
			RoutesDeleted: map[string][]string{
				"rt-123": {"pl-efg"},
				"rt-abc": {"pl-efg"},
			},
		},
		{
			Name: "Deleted from MTGAs",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-abc", "subnet-xyz"},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: nil,
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: nil, // Deleting from MTGAs requires removing from config
				},
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs:                  nil,
			AttachmentIDsDeleted:      []string{testmocks.TestAttachmentName("tgw-123")},
			RoutesDeleted: map[string][]string{
				"rt-123": {"10.0.0.0/8"},
				"rt-abc": {"10.0.0.0/8"},
			},
		},
		{
			Name: "MTGA changed transit gateway ID so attachment must be recreated",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-xyz",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-xyz"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
				"tgw-xyz": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-xyz",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-xyz"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-123")},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-xyz",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-xyz",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-123": {"10.0.0.0/8"},
				"rt-abc": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Edit routes",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.1.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.1.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/16", "10.1.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.1.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.1.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-123": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Basic add with TGW in same account",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes:       nil,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.2.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.2.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.2.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        accountID,
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-010203",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				accountID: {
					"rs-010203": {"666"},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.2.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.2.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Basic add with multiple subnet types",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-def": {
						RouteTableID: "rt-def",
						SubnetType:   database.SubnetTypeApp,
					},
					"rt-ghi": {
						RouteTableID: "rt-ghi",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypeTransport,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-456": {
						RouteTableID: "rt-456",
						SubnetType:   database.SubnetTypeData,
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypeApp,
					},
					"rt-000": {
						RouteTableID: "rt-000",
						SubnetType:   database.SubnetTypeSecurity,
					},
				},
				PublicRouteTableID: "rt-pub",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-def",
									SubnetID:           "subnet-def",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-ghi",
									SubnetID:           "subnet-ghi",
								},
							},
							database.SubnetTypeTransport: {
								{
									CustomRouteTableID: "rt-xyz",
									SubnetID:           "subnet-xyz",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-456",
									SubnetID:           "subnet-456",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-789",
									SubnetID:           "subnet-789",
								},
							},
							database.SubnetTypeSecurity: {
								{
									CustomRouteTableID: "rt-000",
									SubnetID:           "subnet-000",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-def": {
						RouteTableID: "rt-def",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-ghi": {
						RouteTableID: "rt-ghi",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypeTransport,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-456": {
						RouteTableID: "rt-456",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-000": {
						RouteTableID: "rt-000",
						SubnetType:   database.SubnetTypeSecurity,
					},
				},
				PublicRouteTableID: "rt-pub",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-def",
									SubnetID:           "subnet-def",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-ghi",
									SubnetID:           "subnet-ghi",
								},
							},
							database.SubnetTypeTransport: {
								{
									CustomRouteTableID: "rt-xyz",
									SubnetID:           "subnet-xyz",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-456",
									SubnetID:           "subnet-456",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-789",
									SubnetID:           "subnet-789",
								},
							},
							database.SubnetTypeSecurity: {
								{
									CustomRouteTableID: "rt-000",
									SubnetID:           "subnet-000",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes: []database.SubnetType{
						database.SubnetTypePrivate,
						database.SubnetTypeApp,
						database.SubnetTypeData,
						database.SubnetTypeShared,
					},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-def": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-ghi": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-456": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-789": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
		},
		{
			Name: "Multiple subnet types - remove one type",
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-def": {
						RouteTableID: "rt-def",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-ghi": {
						RouteTableID: "rt-ghi",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypeTransport,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-456": {
						RouteTableID: "rt-456",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-000": {
						RouteTableID: "rt-000",
						SubnetType:   database.SubnetTypeSecurity,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				PublicRouteTableID: "rt-pub",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-def",
									SubnetID:           "subnet-def",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-ghi",
									SubnetID:           "subnet-ghi",
								},
							},
							database.SubnetTypeTransport: {
								{
									CustomRouteTableID: "rt-xyz",
									SubnetID:           "subnet-xyz",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-456",
									SubnetID:           "subnet-456",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-789",
									SubnetID:           "subnet-789",
								},
							},
							database.SubnetTypeSecurity: {
								{
									CustomRouteTableID: "rt-000",
									SubnetID:           "subnet-000",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-def": {
						RouteTableID: "rt-def",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-ghi": {
						RouteTableID: "rt-ghi",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-xyz": {
						RouteTableID: "rt-xyz",
						SubnetType:   database.SubnetTypeTransport,
						Routes:       nil,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-456": {
						RouteTableID: "rt-456",
						SubnetType:   database.SubnetTypeData,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-789": {
						RouteTableID: "rt-789",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-000": {
						RouteTableID: "rt-000",
						SubnetType:   database.SubnetTypeSecurity,
						Routes:       nil,
					},
				},
				PublicRouteTableID: "rt-pub",
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-def",
									SubnetID:           "subnet-def",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-ghi",
									SubnetID:           "subnet-ghi",
								},
							},
							database.SubnetTypeTransport: {
								{
									CustomRouteTableID: "rt-xyz",
									SubnetID:           "subnet-xyz",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeData: {
								{
									CustomRouteTableID: "rt-456",
									SubnetID:           "subnet-456",
								},
							},
							database.SubnetTypeApp: {
								{
									CustomRouteTableID: "rt-789",
									SubnetID:           "subnet-789",
								},
							},
							database.SubnetTypeSecurity: {
								{
									CustomRouteTableID: "rt-000",
									SubnetID:           "subnet-000",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes: []database.SubnetType{
						database.SubnetTypePrivate,
						database.SubnetTypeApp,
						database.SubnetTypeData,
						database.SubnetTypeShared,
					},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-xyz": {"10.0.0.0/8"},
				"rt-000": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Complex case with many TGWs",
			// tgw-123: mtga 1, same account, 10.0.0.0/8
			// tgw-456: mtga 2, account "456", no changes, 10.1.0.0/8 (missing from rt-123 and not attached to subnet-abc)
			// tgw-456-2: mtga 3, account "456", dropped from config, 10.2.0.0/8 (missing from rt-abc)
			// tgw-000: mtga 4, account "000", routes changed, 10.3.0.0/8 and 10.4.0.0/8 -> 10.3.0.0/8 and 10.14.0.0/8
			// tgw-111: mtga 6, account "111", added, 10.5.0.0/8
			StartState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.1.0.0/8",
								TransitGatewayID: "tgw-456",
							},
							{
								Destination:      "10.3.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.4.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							// Missing the route for tgw-456-2
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.2.0.0/8",
								TransitGatewayID: "tgw-456-2",
							},
							{
								Destination:      "10.3.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.4.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							// Missing the route for tgw-456
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-456",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-456"),
						SubnetIDs:                          []string{"subnet-123"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{3},
						TransitGatewayID:                   "tgw-456-2",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-456-2"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{4},
						TransitGatewayID:                   "tgw-000",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-000"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.1.0.0/8",
								TransitGatewayID: "tgw-456",
							},
							{
								Destination:      "10.3.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.14.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.5.0.0/8",
								TransitGatewayID: "tgw-111",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.3.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.1.0.0/8",
								TransitGatewayID: "tgw-456",
							},
							{
								Destination:      "10.14.0.0/8",
								TransitGatewayID: "tgw-000",
							},
							{
								Destination:      "10.5.0.0/8",
								TransitGatewayID: "tgw-111",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-456",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-456"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{4},
						TransitGatewayID:                   "tgw-000",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-000"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{6},
						TransitGatewayID:                   "tgw-111",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-111"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1, 2, 4, 6},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123":   "pending",
				"tgw-456":   "available",
				"tgw-456-2": "pending",
				"tgw-000":   "pending",
				"tgw-111":   "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-456"):   "pending",
				testmocks.TestAttachmentName("tgw-456-2"): "available",
				testmocks.TestAttachmentName("tgw-000"):   "available",
			},
			TransitGatewaysAutoAccept: false,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               2,
					TransitGatewayID: "tgw-456",
					Routes:           []string{"10.1.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               3,
					TransitGatewayID: "tgw-456-2",
					Routes:           []string{"10.2.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               4,
					TransitGatewayID: "tgw-000",
					Routes:           []string{"10.3.0.0/8", "10.14.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               6,
					TransitGatewayID: "tgw-111",
					Routes:           []string{"10.5.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			ResourceShares: []*database.TransitGatewayResourceShare{
				{
					AccountID:        accountID,
					Region:           testGovRegion,
					TransitGatewayID: "tgw-123",
					ResourceShareID:  "rs-123",
				},
				{
					AccountID:        "456",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-456",
					ResourceShareID:  "rs-456",
				},
				{
					AccountID:        "456",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-456-2",
					ResourceShareID:  "rs-456-2",
				},
				{
					AccountID:        "000",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-000",
					ResourceShareID:  "rs-000",
				},
				{
					AccountID:        "111",
					Region:           testGovRegion,
					TransitGatewayID: "tgw-111",
					ResourceShareID:  "rs-111",
				},
			},
			ResourceSharePrincipals: map[string]map[string][]string{
				"456": {
					"rs-456":   {"6666"},
					"rs-456-2": {"9999", "8888", "7777"},
				},
				"000": {
					"rs-000": {"5555", accountID, "6666"},
				},
				"111": {
					"rs-111": {},
				},
			},
			SharesUpdated: map[string]map[string][]string{
				"456": {
					"rs-456": {accountID},
				},
				"111": {
					"rs-111": {accountID},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-111"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			AttachmentsUpdated: map[string][]string{
				testmocks.TestAttachmentName("tgw-456"): {"subnet-abc"},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-456-2")},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.14.0.0/8",
						TransitGatewayID: "tgw-000",
					},
					{
						Destination:      "10.5.0.0/8",
						TransitGatewayID: "tgw-111",
					},
				},
				"rt-123": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.1.0.0/8",
						TransitGatewayID: "tgw-456",
					},
					{
						Destination:      "10.14.0.0/8",
						TransitGatewayID: "tgw-000",
					},
					{
						Destination:      "10.5.0.0/8",
						TransitGatewayID: "tgw-111",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-abc": {"10.4.0.0/8"},
				"rt-123": {"10.2.0.0/8", "10.4.0.0/8"},
			},
		},
		{
			Name: "Basic add without resource share, drop one MTGA and add another identical template",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{2},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
				{
					ID:               2,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
			},
		},
		{
			Name: "Basic add without resource share, drop one MTGA and add another with same TGW id but different routes and subnet types",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes:       nil,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{2},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
				{
					ID:               2,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.232.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-abc": {"10.0.0.0/8"},
				"rt-123": {"10.0.0.0/8"},
				"rt-pub": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Basic add without resource share, keep one template and swap two others with the same TGW id",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-def",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-ghi",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{3},
						TransitGatewayID:                   "tgw-xyz",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-xyz"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-def",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-ghi",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{3},
						TransitGatewayID:                   "tgw-xyz",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-xyz"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{2, 3},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
				"tgw-xyz": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
				testmocks.TestAttachmentName("tgw-xyz"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
				{
					ID:               2,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.232.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               3,
					TransitGatewayID: "tgw-xyz",
					Routes:           []string{"10.232.123.0/24"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-abc": {"10.0.0.0/8"},
				"rt-123": {"10.0.0.0/8"},
				"rt-pub": {"10.0.0.0/8"},
			},
		},
		{
			Name: "Basic add without resource share, drop one template and swap two others with the same TGW id",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.232.123.0/24",
								TransitGatewayID: "tgw-xyz",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-def",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-ghi",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{3},
						TransitGatewayID:                   "tgw-xyz",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-xyz"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes:       nil,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.232.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-def",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypePublic: {
								{
									SubnetID: "subnet-ghi",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{2},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
				"tgw-xyz": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				testmocks.TestAttachmentName("tgw-123"): "available",
				testmocks.TestAttachmentName("tgw-xyz"): "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
				{
					ID:               2,
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.232.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
				{
					ID:               3,
					TransitGatewayID: "tgw-xyz",
					Routes:           []string{"10.232.123.0/24"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypePublic},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.232.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			AttachmentIDsDeleted: []string{testmocks.TestAttachmentName("tgw-xyz")},
			RoutesDeleted: map[string][]string{
				"rt-abc": {"10.0.0.0/8", "10.232.123.0/24"},
				"rt-123": {"10.0.0.0/8", "10.232.123.0/24"},
				"rt-pub": {"10.0.0.0/8", "10.232.123.0/24"},
			},
		},
		{
			Name: "Basic add without resource share, two configured templates sharing a TGW id, no existing attachment",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1, 2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1, 2},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "pending",
			},
			TransitGatewayAttachmentStatus: map[string]string{},
			TransitGatewaysAutoAccept:      true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					Name:             "Security",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePublic},
				},
				{
					ID:               2,
					Name:             "Shared-services",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.53.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			AttachmentsCreated: []*ec2.TransitGatewayVpcAttachment{
				{
					VpcId:            &vpcID,
					TransitGatewayId: aws.String("tgw-123"),
					SubnetIds:        aws.StringSlice([]string{"subnet-123", "subnet-abc"}),
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.53.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.53.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-pub": {
					{
						Destination:      "10.0.0.0/8",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			TransitGatewayTagsCreated: map[string][]string{
				testmocks.TestAttachmentName("tgw-123"): {
					testmocks.FormatTag("Name", fmt.Sprintf("%s:Security/Shared-services", vpcName)),
					testmocks.FormatTag("Automated", "true"),
					testmocks.FormatTag(mtgaIDTagKey, "1,2"),
				},
			},
		},
		{
			Name: "Basic add without resource share, add template to existing attachment",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1, 2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{1, 2},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				"tgw-attach-tgw-123": "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					Name:             "Security",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePublic},
				},
				{
					ID:               2,
					Name:             "Shared-services",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.53.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.53.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.53.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			TransitGatewayTagsCreated: map[string][]string{
				testmocks.TestAttachmentName("tgw-123"): {
					testmocks.FormatTag(mtgaIDTagKey, "1,2"),
					testmocks.FormatTag("Name", fmt.Sprintf("%s:Security/Shared-services", vpcName)),
				},
			},
		},
		{
			Name: "Basic add without resource share, remove template from and add template to existing attachment",
			StartState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.0.0.0/8",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-app-1": {
						RouteTableID: "rt-app-1",
						SubnetType:   database.SubnetTypeApp,
					},
					"rt-app-2": {
						RouteTableID: "rt-app-2",
						SubnetType:   database.SubnetTypeApp,
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-app-abc",
									CustomRouteTableID: "rt-app-1",
									GroupName:          "app",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-app-123",
									CustomRouteTableID: "rt-app-2",
									GroupName:          "app",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{1, 2},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			EndState: &database.VPCState{
				PublicRouteTableID: "rt-pub",
				RouteTables: map[string]*database.RouteTableInfo{
					"rt-pub": {
						RouteTableID: "rt-pub",
						SubnetType:   database.SubnetTypePublic,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-abc": {
						RouteTableID: "rt-abc",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.88.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.89.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-123": {
						RouteTableID: "rt-123",
						SubnetType:   database.SubnetTypePrivate,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.53.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.54.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.88.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.89.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-app-1": {
						RouteTableID: "rt-app-1",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.88.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.89.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
					"rt-app-2": {
						RouteTableID: "rt-app-2",
						SubnetType:   database.SubnetTypeApp,
						Routes: []*database.RouteInfo{
							{
								Destination:      "10.55.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.88.0.0/16",
								TransitGatewayID: "tgw-123",
							},
							{
								Destination:      "10.89.0.0/16",
								TransitGatewayID: "tgw-123",
							},
						},
					},
				},
				AvailabilityZones: map[string]*database.AvailabilityZoneInfra{
					"az1": {
						PrivateRouteTableID: "rt-abc",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-abc",
								},
							},
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-app-abc",
									CustomRouteTableID: "rt-app-1",
									GroupName:          "app",
								},
							},
						},
					},
					"az2": {
						PrivateRouteTableID: "rt-123",
						Subnets: map[database.SubnetType][]*database.SubnetInfo{
							database.SubnetTypePrivate: {
								{
									SubnetID: "subnet-123",
								},
							},
							database.SubnetTypeApp: {
								{
									SubnetID:           "subnet-app-123",
									CustomRouteTableID: "rt-app-2",
									GroupName:          "app",
								},
							},
						},
					},
				},
				TransitGatewayAttachments: []*database.TransitGatewayAttachment{
					{
						ManagedTransitGatewayAttachmentIDs: []uint64{2, 3},
						TransitGatewayID:                   "tgw-123",
						TransitGatewayAttachmentID:         testmocks.TestAttachmentName("tgw-123"),
						SubnetIDs:                          []string{"subnet-123", "subnet-abc"},
					},
				},
			},
			TaskData: &database.UpdateNetworkingTaskData{
				VPCID:     vpcID,
				AWSRegion: testGovRegion,
				NetworkingConfig: database.NetworkingConfig{
					ManagedTransitGatewayAttachmentIDs: []uint64{2, 3},
				},
			},
			TransitGatewayStatus: map[string]string{
				"tgw-123": "available",
			},
			TransitGatewayAttachmentStatus: map[string]string{
				"tgw-attach-tgw-123": "available",
			},
			TransitGatewaysAutoAccept: true,
			AllMTGAs: []*database.ManagedTransitGatewayAttachment{
				{
					ID:               1,
					Name:             "Security",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.0.0.0/8"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePublic},
				},
				{
					ID:               2,
					Name:             "Shared-services",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.53.0.0/16", "10.54.0.0/16", "10.55.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePublic, database.SubnetTypePrivate},
				},
				{
					ID:               3,
					Name:             "LDAP",
					TransitGatewayID: "tgw-123",
					Routes:           []string{"10.55.0.0/16", "10.88.0.0/16", "10.89.0.0/16"},
					SubnetTypes:      []database.SubnetType{database.SubnetTypePrivate, database.SubnetTypeApp},
				},
			},
			RoutesAdded: map[string][]*database.RouteInfo{
				"rt-abc": {
					{
						Destination:      "10.88.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.89.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-123": {
					{
						Destination:      "10.88.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.89.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-app-1": {
					{
						Destination:      "10.55.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.88.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.89.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
				"rt-app-2": {
					{
						Destination:      "10.55.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.88.0.0/16",
						TransitGatewayID: "tgw-123",
					},
					{
						Destination:      "10.89.0.0/16",
						TransitGatewayID: "tgw-123",
					},
				},
			},
			RoutesDeleted: map[string][]string{
				"rt-pub": {"10.0.0.0/8"},
			},
			TransitGatewayTagsCreated: map[string][]string{
				testmocks.TestAttachmentName("tgw-123"): {
					testmocks.FormatTag(mtgaIDTagKey, "2,3"),
					testmocks.FormatTag("Name", fmt.Sprintf("%s:LDAP/Shared-services", vpcName)),
				},
			},
		},
	}

	// Speed time up 1000x
	speedUpTime()
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			log.Printf("\n---------- Running test case %q ----------", tc.Name)
			resourceSharesByTGID := make(map[string]*database.TransitGatewayResourceShare)
			for _, rs := range tc.ResourceShares {
				resourceSharesByTGID[rs.TransitGatewayID] = rs
			}
			mm := &testmocks.MockModelsManager{
				ResourceShares: resourceSharesByTGID,
				VPCs:           make(map[string]*database.VPC),
				TestRegion:     testGovRegion,
			}
			ec2svc := &testmocks.MockEC2{
				TransitGatewayStatus:           tc.TransitGatewayStatus,
				TransitGatewayAttachmentStatus: tc.TransitGatewayAttachmentStatus,
				TransitGatewaysAutoAccept:      tc.TransitGatewaysAutoAccept,
			}
			for _, rt := range tc.StartState.RouteTables {
				ec2svc.RouteTables = append(ec2svc.RouteTables, &ec2.RouteTable{
					RouteTableId: aws.String(rt.RouteTableID),
				})
			}
			region := testGovRegion
			if tc.Region != nil {
				region = *tc.Region
			}
			ramsvc := &testmocks.MockRAM{
				Region:     region,
				TestRegion: testGovRegion,
			}
			ctx := &awsp.Context{
				AWSAccountAccess: &awsp.AWSAccountAccess{
					EC2svc: ec2svc,
					RAMsvc: ramsvc,
				},
				VPCName: vpcName,
				VPCID:   vpcID,
				Logger:  logger,
				Clock:   testClock,
			}
			vpc := &database.VPC{
				AccountID: accountID,
				Name:      vpcName,
				ID:        vpcID,
				Region:    testGovRegion,
				State:     tc.StartState,
			}
			vpcWriter := &testmocks.MockVPCWriter{
				MM:     mm,
				VPCID:  vpc.ID,
				Region: vpc.Region,
			}
			shareEC2s := make(map[string]*testmocks.MockEC2)
			shareRAMs := make(map[string]*testmocks.MockRAM)
			for _, rs := range tc.ResourceShares {
				ec2svc := &testmocks.MockEC2{
					TransitGatewayAttachmentStatus: tc.TransitGatewayAttachmentStatus,
				}
				region := testGovRegion
				if tc.Region != nil {
					region = string(rs.Region)
				}
				ramsvc := &testmocks.MockRAM{
					AccountID:               rs.AccountID,
					Region:                  region,
					ResourceSharePrincipals: tc.ResourceSharePrincipals[rs.AccountID],
					ResourceShareStatuses: map[string]string{
						rs.ResourceShareID: ram.ResourceShareStatusActive,
					},
					TestRegion: testGovRegion,
				}
				shareEC2s[rs.AccountID] = ec2svc
				shareRAMs[rs.AccountID] = ramsvc
			}
			managedAttachmentsByID := make(map[uint64]*database.ManagedTransitGatewayAttachment)
			for _, ma := range tc.AllMTGAs {
				managedAttachmentsByID[ma.ID] = ma
			}
			var getAccountCredentials = func(accountID string) (ec2iface.EC2API, ramiface.RAMAPI, error) {
				if _, ok := shareEC2s[accountID]; ok {
					return shareEC2s[accountID], shareRAMs[accountID], nil
				}
				return nil, nil, fmt.Errorf("Unexpected request for account %s credentials", accountID)
			}
			err := handleTransitGatewayAttachments(
				ctx,
				vpc,
				vpcWriter,
				tc.TaskData,
				mm,
				managedAttachmentsByID,
				getAccountCredentials)
			if err != nil {
				if tc.ExpectedError != nil && tc.ExpectedError.Error() == err.Error() {
					log.Printf("Got expected error for handleTransitGatewayAttachments: %s", err)
					return
				} else {
					t.Fatalf("TGW test case %q failed to handle attachments: %s", tc.Name, err)
				}
			} else if tc.ExpectedError != nil {
				t.Fatalf("TGW test case %q expected an error, but none was returned: %s", tc.Name, err)
			}

			resources := make(map[string][]testmocks.MockResource)
			statuses := make(map[string]string)
			for _, plrs := range tc.PLResourceShares {
				for _, id := range plrs.PLIDs {
					resources[plrs.ResourceShareID] = append(resources[plrs.ResourceShareID], testmocks.MockResource{
						ID:   prefixListARN(testGovRegion, prefixListAccountIDGovCloud, id),
						Type: awsp.ResourceTypePrefixList,
					})
					statuses[plrs.ResourceShareID] = ram.ResourceShareStatusActive
				}
			}
			plRAM := &testmocks.MockRAM{
				AccountID:               prefixListAccountIDGovCloud,
				Region:                  region,
				ResourceSharePrincipals: tc.ResourceSharePrincipals[prefixListAccountIDGovCloud],
				ResourceShareResources:  resources,
				ResourceShareStatuses:   statuses,
				TestRegion:              testGovRegion,
			}
			shareRAMs[prefixListAccountIDGovCloud] = plRAM
			getPrefixListRAM := func(region database.Region) (ramiface.RAMAPI, error) {
				return shareRAMs[prefixListAccountIDGovCloud], nil
			}

			var expectedErrorString string
			if tc.UnsharedPLID != "" {
				expectedErrorString = fmt.Sprintf("Error ensuring configured Prefix Lists are shared via RAM: The configured Prefix List %s is not yet shared. You must share the Prefix List from account %s in the AWS Console first.", tc.UnsharedPLID, prefixListAccountIDGovCloud)
			}
			var affectedSubnetTypes []database.SubnetType
			for _, mtga := range tc.AllMTGAs {
				if stringInSlice(tc.UnsharedPLID, mtga.Routes) {
					affectedSubnetTypes = append(affectedSubnetTypes, mtga.SubnetTypes...)
				}
			}

			publicRTs := []*database.RouteTableInfo{}
			if tc.StartState.VPCType == database.VPCTypeV1 || tc.StartState.VPCType == database.VPCTypeLegacy {
				publicRT, ok := tc.StartState.RouteTables[tc.StartState.PublicRouteTableID]
				if !ok {
					publicRT = &database.RouteTableInfo{}
				}
				publicRTs = append(publicRTs, publicRT)
			} else if tc.StartState.VPCType == database.VPCTypeV1Firewall {
				for _, az := range tc.StartState.AvailabilityZones {
					publicRT, ok := tc.StartState.RouteTables[az.PublicRouteTableID]
					if !ok {
						publicRT = &database.RouteTableInfo{}
					}
					publicRTs = append(publicRTs, publicRT)
				}
			}
			for _, publicRT := range publicRTs {
				err = updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, managedAttachmentsByID, publicRT, database.SubnetTypePublic, database.Region(region), getPrefixListRAM)
				if err != nil {
					if expectedErrorString != "" && err.Error() == expectedErrorString {
						log.Printf("Got expected error for updateTransitGatewayRoutesForSubnet public: %s", err)
						return
					} else {
						t.Fatalf("TGW test case %q failed to update routes for public subnet route table: %s", tc.Name, err)
					}
				} else if tc.UnsharedPLID != "" && subnetTypeInSlice(database.SubnetTypePublic, affectedSubnetTypes) {
					t.Fatalf("TGW test case %q expected an error, but none was returned: %s", tc.Name, err)
				}
				if err != nil {
					t.Fatalf("TGW test case %q failed to update routes for public subnet route table: %s", tc.Name, err)
				}
			}

			for azName, az := range tc.StartState.AvailabilityZones {
				for subnetType, subnets := range az.Subnets {
					if subnetType == database.SubnetTypePrivate {
						privateRT, ok := tc.StartState.RouteTables[az.PrivateRouteTableID]
						if !ok {
							privateRT = &database.RouteTableInfo{}
						}
						err := updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, managedAttachmentsByID, privateRT, database.SubnetTypePrivate, database.Region(region), getPrefixListRAM)
						if err != nil {
							if expectedErrorString != "" && err.Error() == expectedErrorString {
								log.Printf("Got expected error for updateTransitGatewayRoutesForSubnet private: %s", err)
							} else {
								t.Fatalf("TGW Test case %q failed to update routes for private subnet route table in AZ %s: %s", tc.Name, azName, err)
							}
						} else if tc.UnsharedPLID != "" && subnetTypeInSlice(database.SubnetTypePrivate, affectedSubnetTypes) {
							t.Fatalf("TGW test case %q expected an error, but none was returned: %s", tc.Name, err)
						}
					} else if subnetType == database.SubnetTypePublic {
						continue
					} else {
						for _, subnet := range subnets {
							customRT, ok := tc.StartState.RouteTables[subnet.CustomRouteTableID]
							if !ok {
								customRT = &database.RouteTableInfo{}
							}
							err := updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, tc.TaskData, managedAttachmentsByID, customRT, subnetType, database.Region(region), getPrefixListRAM)
							if err != nil {
								if expectedErrorString != "" && err.Error() == expectedErrorString {
									log.Printf("Got expected error for updateTransitGatewayRoutesForSubnet other: %s", err)
								} else {
									t.Fatalf("TGW Test case %q failed to update routes for subnet %s: %s", tc.Name, subnet.SubnetID, err)
								}
							} else if tc.UnsharedPLID != "" && subnetTypeInSlice(subnetType, affectedSubnetTypes) {
								t.Fatalf("TGW test case %q expected an error, but none was returned: %s", tc.Name, err)
							}
							if err != nil {
								t.Fatalf("TGW Test case %q failed to update routes for subnet %s: %s", tc.Name, subnet.SubnetID, err)
							}
						}
					}
				}
			}
			for idx, a1 := range ec2svc.AttachmentsCreated {
				if idx < len(tc.AttachmentsCreated) {
					a2 := tc.AttachmentsCreated[idx]
					sort.Sort(StringPointerSlice(a1.SubnetIds))
					sort.Sort(StringPointerSlice(a2.SubnetIds))
				}
			}
			sort.Slice(ec2svc.AttachmentsCreated, func(i, j int) bool {
				return *ec2svc.AttachmentsCreated[i].TransitGatewayId > *ec2svc.AttachmentsCreated[j].TransitGatewayId
			})
			sort.Slice(tc.AttachmentsCreated, func(i, j int) bool {
				return *tc.AttachmentsCreated[i].TransitGatewayId > *tc.AttachmentsCreated[j].TransitGatewayId
			})

			if !reflect.DeepEqual(tc.AttachmentsCreated, ec2svc.AttachmentsCreated) {
				t.Fatalf("TGW Test case %q: Wrong attachments created. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.AttachmentsCreated, ec2svc.AttachmentsCreated)
			}
			for _, attachment := range tc.AttachmentsCreated {
				status := tc.TransitGatewayAttachmentStatus[testmocks.TestAttachmentName(*attachment.TransitGatewayId)]
				if status != "available" {
					t.Fatalf("TGW Test case %q: Incorrect attachment status %q for transit gateway %q. May have failed to accept or wait.", tc.Name, status, *attachment.TransitGatewayId)
				}
			}
			for idx, subnets1 := range ec2svc.AttachmentsModified {
				if subnets2, ok := tc.AttachmentsUpdated[idx]; ok {
					sort.Strings(subnets1)
					sort.Strings(subnets2)
				}
			}
			if !reflect.DeepEqual(tc.AttachmentsUpdated, ec2svc.AttachmentsModified) {
				t.Fatalf("TGW Test case %q: Wrong attachment updates. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.AttachmentsUpdated, ec2svc.AttachmentsModified)
			}
			for id := range tc.AttachmentsUpdated {
				status := tc.TransitGatewayAttachmentStatus[id]
				if status != "available" {
					t.Fatalf("TGW Test case %q: Incorrect attachment status %q for transit gateway attachment %q. May have failed to wait after updating.", tc.Name, status, id)
				}
			}
			if !reflect.DeepEqual(tc.AttachmentIDsDeleted, ec2svc.AttachmentIDsDeleted) {
				t.Fatalf("TGW Test case %q: Wrong attachments deleted. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.AttachmentIDsDeleted, ec2svc.AttachmentIDsDeleted)
			}

			for _, id := range tc.AttachmentIDsDeleted {
				status := tc.TransitGatewayAttachmentStatus[id]
				if status != "deleted" {
					t.Fatalf("TGW Test case %q: Incorrect attachment status %q for transit gateway attachment %q. May have failed to wait after deleting.", tc.Name, status, id)
				}
			}
			for accountID, ram := range shareRAMs {
				var expectedSharesUpdated map[string][]string
				if len(tc.SharesUpdated[accountID]) > 0 {
					expectedSharesUpdated = make(map[string][]string)
				}
				for shareID, principals := range tc.SharesUpdated[accountID] {
					expectedSharesUpdated[resourceShareARN(testGovRegion, accountID, shareID)] = principals
				}
				if !reflect.DeepEqual(expectedSharesUpdated, ram.PrincipalsAdded) {
					t.Fatalf("TGW Test case %q: Wrong principals added to shares for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedSharesUpdated, ram.PrincipalsAdded)
				}
			}
			var expectedARNs []string
			for accountID, acctShares := range tc.SharesUpdated {
				for shareID := range acctShares {
					expectedARNs = append(expectedARNs, resourceShareARN(testGovRegion, accountID, shareID))
				}
			}

			sort.Strings(expectedARNs)
			sort.Strings(ramsvc.InvitationsAcceptedShareARN)
			if !reflect.DeepEqual(expectedARNs, ramsvc.InvitationsAcceptedShareARN) {
				t.Fatalf("TGW Test case %q: Wrong shares accepted. Expected:\n%#v\nbut got:\n%#v", tc.Name, expectedARNs, ramsvc.InvitationsAcceptedShareARN)
			}

			for _, routes := range tc.RoutesAdded {
				sort.Slice(routes, func(i, j int) bool { return routes[i].Destination < routes[j].Destination })
			}
			for _, routes := range ec2svc.RoutesAdded {
				sort.Slice(routes, func(i, j int) bool { return routes[i].Destination < routes[j].Destination })
			}
			if !reflect.DeepEqual(tc.RoutesAdded, ec2svc.RoutesAdded) {
				t.Fatalf("TGW Test case %q: Wrong routes added. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.RoutesAdded, ec2svc.RoutesAdded)
			}

			for _, r := range tc.RoutesDeleted {
				sort.Strings(r)
			}
			for _, r := range ec2svc.RoutesDeleted {
				sort.Strings(r)
			}
			if !reflect.DeepEqual(tc.RoutesDeleted, ec2svc.RoutesDeleted) {
				t.Fatalf("TGW Test case %q: Wrong routes deleted. Expected:\n%#v\nbut got:\n%#v", tc.Name, tc.RoutesDeleted, ec2svc.RoutesDeleted)
			}

			for id, expectedTags := range tc.TransitGatewayTagsCreated {
				createdTags, ok := ec2svc.TagsCreated[id]
				if ok {
					sort.Strings(expectedTags)
					sort.Strings(createdTags)
					if !reflect.DeepEqual(expectedTags, createdTags) {
						t.Fatalf("TGW Test case %q: Created transit gateway attachment tags did not match expected tags. Expected :\n%v\nbut got:\n%v", tc.Name, expectedTags, createdTags)
					}
				} else {
					t.Fatalf("TGW Test case %q: Expected a tag to be created for transit gateway attachment %s but none was created.", tc.Name, id)
				}
			}

			endVPC, err := mm.GetVPC(testGovRegion, vpcID)
			if err != nil {
				t.Fatalf("TGW Test case %q: Error getting end state: %s", tc.Name, err)
			}

			sort.Slice(tc.EndState.TransitGatewayAttachments, func(i, j int) bool {
				return tc.EndState.TransitGatewayAttachments[i].TransitGatewayID < tc.EndState.TransitGatewayAttachments[j].TransitGatewayID
			})
			sort.Slice(endVPC.State.TransitGatewayAttachments, func(i, j int) bool {
				return endVPC.State.TransitGatewayAttachments[i].TransitGatewayID < endVPC.State.TransitGatewayAttachments[j].TransitGatewayID
			})

			for _, rtInfo := range tc.EndState.RouteTables {
				sort.Slice(rtInfo.Routes, func(i, j int) bool {
					return rtInfo.Routes[i].Destination < rtInfo.Routes[j].Destination
				})
			}
			for _, rtInfo := range endVPC.State.RouteTables {
				sort.Slice(rtInfo.Routes, func(i, j int) bool {
					return rtInfo.Routes[i].Destination < rtInfo.Routes[j].Destination
				})
			}

			if diff := cmp.Diff(tc.EndState, endVPC.State); diff != "" {
				t.Fatalf("Expected end state did not match state in database: \n%s", diff)
			}
		})
	}
}
