package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/benbjohnson/clock"
)

const testRegion = "us-west-2"
const testGovRegion = "us-gov-west-1"

var testClock = clock.NewMock()

type testLogger struct{}

func (t *testLogger) Log(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

func speedUpTime() {
	// Speed time up 1000x
	go func() {
		for {
			testClock.Add(time.Second)
			time.Sleep(time.Millisecond)
		}
	}()
}

type childParentCIDRTestCase struct {
	Name           string
	ParentCIDR     string
	ChildCIDRs     []string
	ExpectedResult bool
}

func cidrsToIPNets(cidrs []string) ([]*net.IPNet, error) {
	var ipNets []*net.IPNet
	for _, c := range cidrs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, err
		}
		ipNets = append(ipNets, ipNet)
	}
	return ipNets, nil
}

func TestChildCIDRsUseAllParentCIDRSpace(t *testing.T) {
	testCases := []*childParentCIDRTestCase{
		{
			Name:       "/25 --> 4 /27s",
			ParentCIDR: "10.231.25.128/25",
			ChildCIDRs: []string{
				"10.231.25.128/27",
				"10.231.25.160/27",
				"10.231.25.192/27",
				"10.231.25.224/27",
			},
			ExpectedResult: true,
		},
		{
			Name:       "/26 --> 2 /27s",
			ParentCIDR: "10.231.25.192/26",
			ChildCIDRs: []string{
				"10.231.25.192/27",
				"10.231.25.224/27",
			},
			ExpectedResult: true,
		},
		{
			Name:       "/20 --> 1 /21, 1 /22, 1 /23 and and 2 /24s",
			ParentCIDR: "10.231.16.0/20",
			ChildCIDRs: []string{
				"10.231.16.0/21",
				"10.231.24.0/22",
				"10.231.28.0/23",
				"10.231.30.0/24",
				"10.231.31.0/24",
			},
			ExpectedResult: true,
		},
		{
			Name:       "/25 --> 2 /27s",
			ParentCIDR: "10.231.25.128/25",
			ChildCIDRs: []string{
				"10.231.25.128/27",
				"10.231.25.160/27",
				"10.231.25.192/27",
				"10.231.25.224/27",
			},
			ExpectedResult: true,
		},
		{
			Name:       "/26 --> 1 /27",
			ParentCIDR: "10.231.25.192/26",
			ChildCIDRs: []string{
				"10.231.25.192/27",
			},
			ExpectedResult: false,
		},
		{
			Name:       "/26 --> 1 /27 and 1 /28s",
			ParentCIDR: "10.231.25.192/26",
			ChildCIDRs: []string{
				"10.231.25.192/27",
				"10.231.25.240/28",
			},
			ExpectedResult: false,
		},
		{
			Name:       "/25 --> 3 /27s",
			ParentCIDR: "10.231.25.128/25",
			ChildCIDRs: []string{
				"10.231.25.128/27",
				"10.231.25.160/27",
				"10.231.25.192/27",
			},
			ExpectedResult: false,
		},
		{
			Name:           "/25 --> 0 child CIDRs",
			ParentCIDR:     "10.231.25.128/25",
			ChildCIDRs:     []string{},
			ExpectedResult: false,
		},
	}

	for _, tc := range testCases {
		_, parentIPNet, err := net.ParseCIDR(tc.ParentCIDR)
		if err != nil {
			t.Fatalf("For test case '%s', error converting parent CIDR to IPNet: %s", tc.Name, err)
		}
		childIPNets, err := cidrsToIPNets(tc.ChildCIDRs)
		if err != nil {
			t.Fatalf("For test case '%s', error converting child CIDRs to IPNets: %s", tc.Name, err)
		}
		actualResult := childCIDRsUseAllParentCIDRSpace(parentIPNet, childIPNets)
		if tc.ExpectedResult != actualResult {
			t.Fatalf("For test case '%s', ChildCIDRsUseAllParentCIDRSpace returned the wrong result. Expected: %t but got: %t", tc.Name, tc.ExpectedResult, actualResult)
		}
	}
}

func TestMergeIsssues(t *testing.T) {
	type testCase struct {
		name                      string
		existingIssues, newIssues []*database.Issue
		newIssueTypes             database.VerifyTypes
		mergedIssues              []*database.Issue
	}
	testCases := []*testCase{
		{
			name: "replace all",
			existingIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifyCIDRs},
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "existing 3", Type: database.VerifyLogging},
			},
			newIssues: []*database.Issue{
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2", Type: database.VerifyCIDRs},
				{Description: "new 3", Type: database.VerifyResolverRules},
			},
			newIssueTypes: database.VerifyCIDRs | database.VerifyNetworking | database.VerifyLogging | database.VerifyResolverRules,
			mergedIssues: []*database.Issue{
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2", Type: database.VerifyCIDRs},
				{Description: "new 3", Type: database.VerifyResolverRules},
			},
		},
		{
			name: "drop all",
			existingIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifyCIDRs},
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "existing 3", Type: database.VerifyLogging},
			},
			newIssues:     []*database.Issue{},
			newIssueTypes: database.VerifyCIDRs | database.VerifyNetworking | database.VerifyLogging | database.VerifyResolverRules,
			mergedIssues:  []*database.Issue{},
		},
		{
			name: "replace some",
			existingIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifySecurityGroups},
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "existing 3", Type: database.VerifyLogging},
			},
			newIssues: []*database.Issue{
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2", Type: database.VerifyCIDRs},
				{Description: "new 3", Type: database.VerifyNetworking},
			},
			newIssueTypes: database.VerifyCIDRs | database.VerifyNetworking,
			mergedIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifySecurityGroups},
				{Description: "existing 3", Type: database.VerifyLogging},
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2", Type: database.VerifyCIDRs},
				{Description: "new 3", Type: database.VerifyNetworking},
			},
		},
		{
			name: "drop some",
			existingIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifySecurityGroups},
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "existing 3", Type: database.VerifyLogging},
			},
			newIssues:     []*database.Issue{},
			newIssueTypes: database.VerifyCIDRs | database.VerifyNetworking,
			mergedIssues: []*database.Issue{
				{Description: "existing 1", Type: database.VerifySecurityGroups},
				{Description: "existing 3", Type: database.VerifyLogging},
			},
		},
		{
			name: "no-type existing issues always dropped but new ones allowed",
			existingIssues: []*database.Issue{
				{Description: "existing 1"},
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "existing 3"},
			},
			newIssues: []*database.Issue{
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2"},
			},
			newIssueTypes: database.VerifyCIDRs,
			mergedIssues: []*database.Issue{
				{Description: "existing 2", Type: database.VerifyNetworking},
				{Description: "new 1", Type: database.VerifyCIDRs},
				{Description: "new 2"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := mergeIssues(tc.existingIssues, tc.newIssues, tc.newIssueTypes)
			if !reflect.DeepEqual(tc.mergedIssues, actual) {
				t.Errorf("Merged issues %#v don't match expected %#v", actual, tc.mergedIssues)
				return
			}
		})
	}
}

func TestCreateRouteInfos(t *testing.T) {
	type testCase struct {
		Name               string
		RouteTable         *ec2.RouteTable
		RTInfo             *database.RouteTableInfo
		ExpectedRouteInfos []*database.RouteInfo
		ExpectedError      error
	}
	igwID := "igw-123"
	endpointID := "vpce-456"
	natID := "nat-789"

	testCases := []testCase{
		{
			Name: "Gateway ID is IGW ID",
			RouteTable: &ec2.RouteTable{
				Routes: []*ec2.Route{
					{
						GatewayId:            aws.String(igwID),
						Origin:               aws.String(ec2.RouteOriginCreateRoute),
						DestinationCidrBlock: aws.String(internetRoute),
					},
				},
			},
			RTInfo: &database.RouteTableInfo{},
			ExpectedRouteInfos: []*database.RouteInfo{
				{
					Destination:       internetRoute,
					InternetGatewayID: igwID,
				},
			},
		},

		{
			Name: "Gateway ID is empty",
			RouteTable: &ec2.RouteTable{
				Routes: []*ec2.Route{
					{
						NatGatewayId:         aws.String(natID),
						Origin:               aws.String(ec2.RouteOriginCreateRoute),
						DestinationCidrBlock: aws.String(internetRoute),
					},
				},
			},
			RTInfo: &database.RouteTableInfo{},
			ExpectedRouteInfos: []*database.RouteInfo{
				{
					Destination:  internetRoute,
					NATGatewayID: natID,
				},
			},
		},

		{
			Name: "Error: unrecognized gateway ID",
			RouteTable: &ec2.RouteTable{
				Routes: []*ec2.Route{
					{
						GatewayId:            aws.String("foo-123"),
						Origin:               aws.String(ec2.RouteOriginCreateRoute),
						DestinationCidrBlock: aws.String(internetRoute),
					},
				},
			},
			RTInfo:        &database.RouteTableInfo{},
			ExpectedError: fmt.Errorf("Could not assign unrecognized gateway ID foo-123"),
		},

		{
			Name: "Error: both subnet and edge association types",
			RouteTable: &ec2.RouteTable{
				Routes: []*ec2.Route{
					{
						GatewayId:            aws.String(endpointID),
						Origin:               aws.String(ec2.RouteOriginCreateRoute),
						DestinationCidrBlock: aws.String(internetRoute),
					},
				},
			},
			RTInfo: &database.RouteTableInfo{
				SubnetType:          database.SubnetTypePublic,
				EdgeAssociationType: database.EdgeAssociationTypeIGW,
			},
			ExpectedError: fmt.Errorf("Route table info has both an edge association type and subnet type"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			actualRouteInfos, err := createRouteInfos(tc.RouteTable, tc.RTInfo)
			if err != nil {
				if tc.ExpectedError != nil && tc.ExpectedError.Error() == err.Error() {
					log.Printf("Got expected error: %s", err)
					return
				} else {
					t.Fatalf("Got unexpected error: %s", err)
				}
			} else if tc.ExpectedError != nil {
				t.Fatalf("Expected an error, but none was returned: %s", err)
			}

			expected, err := json.MarshalIndent(tc.ExpectedRouteInfos, "", "  ")
			if err != nil {
				t.Fatalf("Could not marshal expected route table info: %s", err)
			}
			actual, err := json.MarshalIndent(actualRouteInfos, "", "  ")
			if err != nil {
				t.Fatalf("Could not marshal actual route table info: %s", err)
			}
			if !reflect.DeepEqual(expected, actual) {
				t.Errorf("Expected route infos don't match actual route infos.\n Expected: %s\n Actual: %s\n", expected, actual)
			}
		})
	}
}
