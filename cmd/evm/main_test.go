package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

type mockEC2 struct {
	ec2iface.EC2API
	createRouteSuccess      bool
	createdRoutes           map[string][]string
	routeType               RouteType
	hasTGWAttachment        bool
	hasAnotherTGWAttachment bool
}

const (
	numRouteTables int = 6
	numRoutes      int = 4
)

type RouteType int

const (
	Matching RouteType = iota
	Mock
	Partial
	Conflicting
)

var (
	origin    string   = "CreateRoute"
	ownerID   string   = "12345678910"
	tgwRoutes []string = []string{
		fakeAWSID("pl"),
		fakeAWSID("pl"),
		fakeAWSID("pl"),
		fakeAWSID("pl"),
	}
	state            string = "active"
	transitGatewayID string = fakeAWSID("tgw")
	partialRouteLen  int
	vpcMockID        string   = fakeAWSID("vpc")
	mockDestinations []string = []string{"0.0.0.0/16", "0.0.1.0/16", fakeAWSID("pl"), fakeAWSID("pl")}
)

var TGWTemplate vpcconfapi.TGWTemplate = vpcconfapi.TGWTemplate{
	TransitGatewayID: transitGatewayID,
}

func init() {
	TGWTemplate.Routes = make([]string, len(tgwRoutes))
	copy(TGWTemplate.Routes, tgwRoutes)
}

func getRoutes(m mockEC2) []*ec2.Route {
	switch m.routeType {
	case Matching:
		return generateMatchingRoutes()
	case Mock:
		return generateMockRoutes()
	case Partial:
		return generatePartialRoutes()
	case Conflicting:
		return generateConflictingRoutes()
	default:
		log.Panicf("Unknown RouteType %v", m.routeType)
	}
	return nil
}

func generateRouteTables(m mockEC2, input *ec2.DescribeRouteTablesInput) []*ec2.RouteTable {
	routeTables := []*ec2.RouteTable{}

	for i := 0; i < numRouteTables; i++ {
		rtbID := fakeAWSID("rtb")
		routeTables = append(routeTables, &ec2.RouteTable{
			OwnerId:      &ownerID,
			Routes:       getRoutes(m),
			RouteTableId: &rtbID,
			VpcId:        input.Filters[0].Values[0],
		})
	}

	return routeTables
}

func generateMockRoutes() []*ec2.Route {
	routes := []*ec2.Route{}

	for i := 0; i < numRoutes; i++ {
		var destinationCIDR *string
		var destinationPLID *string

		if isPrefixListID(mockDestinations[i]) {
			destinationPLID = &mockDestinations[i]
		} else {
			destinationCIDR = &mockDestinations[i]
		}
		routes = append(routes, &ec2.Route{
			DestinationCidrBlock:    destinationCIDR,
			DestinationPrefixListId: destinationPLID,
			Origin:                  &origin,
			State:                   &state,
			TransitGatewayId:        &transitGatewayID,
		})
	}

	return routes
}

func generateMatchingRoutes() []*ec2.Route {
	routes := []*ec2.Route{}

	for i := 0; i < len(tgwRoutes); i++ {
		routes = append(routes, &ec2.Route{
			DestinationPrefixListId: &tgwRoutes[i],
			Origin:                  &origin,
			State:                   &state,
			TransitGatewayId:        &transitGatewayID,
		})
	}

	return routes
}

func getPartialRouteLen() int {
	partialRouteLen = len(tgwRoutes)

	if partialRouteLen == 0 {
		log.Fatal("tgwRoutes length should not be 0")
	} else if partialRouteLen > 2 {
		partialRouteLen = partialRouteLen / 2
	}

	return partialRouteLen
}

func generatePartialRoutes() []*ec2.Route {
	routes := []*ec2.Route{}
	partialRouteLen = getPartialRouteLen()

	for i := 0; i < partialRouteLen; i++ {
		routes = append(routes, &ec2.Route{
			DestinationPrefixListId: &tgwRoutes[i],
			Origin:                  &origin,
			State:                   &state,
			TransitGatewayId:        &transitGatewayID,
		})
	}

	return routes
}

func generateConflictingRoutes() []*ec2.Route {
	routes := []*ec2.Route{}
	var pcxID = fakeAWSID("pcx")

	for i := 0; i < len(tgwRoutes); i++ {
		route := &ec2.Route{
			DestinationPrefixListId: &tgwRoutes[i],
			Origin:                  &origin,
			State:                   &state,
			VpcPeeringConnectionId:  &pcxID,
		}
		routes = append(routes, route)
	}

	return routes
}

func fakeAWSID(prefix string) string {
	bytes := make([]byte, 9)
	rand.Read(bytes)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes)[:16])
}

func generateTransitGatewayAttachments(m mockEC2) (*ec2.DescribeTransitGatewayAttachmentsOutput, error) {
	output := &ec2.DescribeTransitGatewayAttachmentsOutput{
		TransitGatewayAttachments: []*ec2.TransitGatewayAttachment{},
	}
	if m.hasTGWAttachment && !m.hasAnotherTGWAttachment {
		output.TransitGatewayAttachments = append(output.TransitGatewayAttachments,
			&ec2.TransitGatewayAttachment{
				TransitGatewayId: aws.String(transitGatewayID),
			})
	} else if !m.hasTGWAttachment && m.hasAnotherTGWAttachment {
		output.TransitGatewayAttachments = append(output.TransitGatewayAttachments,
			&ec2.TransitGatewayAttachment{
				TransitGatewayId: aws.String(fakeAWSID("tgw")),
			})
	} else {
		return nil, fmt.Errorf("Expected either TGWAttachment (%v) or WrongTWGAttachment (%v) to be set to true", m.hasTGWAttachment, m.hasAnotherTGWAttachment)
	}

	return output, nil
}

func (m mockEC2) DescribeTransitGatewayAttachments(input *ec2.DescribeTransitGatewayAttachmentsInput) (*ec2.DescribeTransitGatewayAttachmentsOutput, error) {
	validInput := &ec2.DescribeTransitGatewayAttachmentsInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("resource-id"),
			Values: []*string{aws.String(vpcMockID)},
		},
	}}

	if !reflect.DeepEqual(input, validInput) {
		return nil, fmt.Errorf("Expected input to match %#v, but got %#v", validInput, input)
	}

	return generateTransitGatewayAttachments(m)
}

func (m mockEC2) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	validInput := &ec2.DescribeRouteTablesInput{Filters: []*ec2.Filter{
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{aws.String(vpcMockID)},
		},
	}}

	if !reflect.DeepEqual(input, validInput) {
		return nil, fmt.Errorf("Expected input to match %#v, but got %#v", validInput, input)
	}

	output := &ec2.DescribeRouteTablesOutput{
		RouteTables: generateRouteTables(m, input),
	}

	return output, nil
}

func (m *mockEC2) CreateRoute(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	if m.createRouteSuccess {
		if aws.StringValue(input.DestinationCidrBlock) != "" {
			return nil, fmt.Errorf("A CIDR route %s was passed to CreateRoute when only prefix list routes were expected", aws.StringValue(input.DestinationCidrBlock))
		}

		m.createdRoutes[aws.StringValue(input.RouteTableId)] = append(m.createdRoutes[aws.StringValue(input.RouteTableId)], aws.StringValue(input.DestinationPrefixListId))
		return &ec2.CreateRouteOutput{
			Return: aws.Bool(true),
		}, nil
	}

	return nil, fmt.Errorf("Failed to create route: %s -> %s -> %s", aws.StringValue(input.RouteTableId), aws.StringValue(input.DestinationPrefixListId), aws.StringValue(input.TransitGatewayId))
}

func TestInspectRouteTablesAllRoutesNeeded(t *testing.T) {
	routeTables, tableErrors := inspectRouteTables(&mockEC2{routeType: Mock}, vpcMockID, fakeAWSID("tgw"), tgwRoutes)

	if len(tableErrors) > 0 {
		t.Errorf("There should have not been any table errors but there were %d", len(tableErrors))
	}
	if len(routeTables) != numRouteTables {
		t.Errorf("Expected %d route tables but got %d", numRouteTables, len(routeTables))
	}

	for _, rt := range routeTables {
		if !reflect.DeepEqual(rt.routesNeeded, tgwRoutes) {
			t.Errorf("Expected routes to match %#v, but got %#v", tgwRoutes, rt.routesNeeded)
		}
	}
}

func TestInspectRouteTablesSomeRoutesNeeded(t *testing.T) {
	routeTables, tableErrors := inspectRouteTables(&mockEC2{routeType: Partial}, vpcMockID, transitGatewayID, tgwRoutes)

	if len(tableErrors) > 0 {
		t.Errorf("There should have not been any table errors but there were %d", len(tableErrors))
	}
	if len(routeTables) != numRouteTables {
		t.Errorf("Expected %d route tables but got %d", numRouteTables, len(routeTables))
	}
	routeLen := len(tgwRoutes) / 2 // this operation is validated in generatePartialRoutes()
	expectedRoutes := tgwRoutes[routeLen:]
	for _, rt := range routeTables {
		if !reflect.DeepEqual(rt.routesNeeded, expectedRoutes) {
			t.Errorf("Expected the last half (rounded up) of routes to match %#v, but got %#v", expectedRoutes, rt.routesNeeded)
		}
	}
}

func TestRouteTableMethods(t *testing.T) {
	rt := &routeTable{id: "rt-abcdef0123456789", routesNeeded: []string{"10.232.32.0/19", "10.244.96.0/19", "10.223.120.0/22"}}
	postRemoval := &routeTable{id: "rt-abcdef0123456789", routesNeeded: []string{"10.232.32.0/19", "10.223.120.0/22"}}

	idx := rt.indexOf("10.244.96.0/19")
	if idx != 1 {
		t.Errorf("Expected index to be 1 but got %d", idx)
	}

	err := rt.removeAt(idx)
	if err != nil {
		t.Errorf("Expected no error when removing element at index 1 but got %s", err)
	}

	idx = rt.indexOf("10.244.96.0/19")
	if idx != -1 {
		t.Errorf("Expected index to be -1 but got %d", idx)
	}

	if !reflect.DeepEqual(rt, postRemoval) {
		t.Errorf("Expected route table and routes to match %#v, but got %#v", postRemoval, rt)
	}
}

type inspectTestCase struct {
	// initial state of a VPC
	name               string
	vpc                *vpcconfapi.VPC
	TGWIsShared        bool
	hasAnotherTGWShare bool
	// return values of inspect
	errorIsExpected bool
	// generated route configuration
	numRouteTables int
	routesExpected []string
	routeType      RouteType
}

var inspectTestCases = []inspectTestCase{
	{
		name:               "Wrong TransitGateway Attachment",
		vpc:                &vpcconfapi.VPC{AccountID: ownerID, ID: vpcMockID},
		TGWIsShared:        false,
		hasAnotherTGWShare: true,
		numRouteTables:     numRouteTables,
		routesExpected:     tgwRoutes,
		routeType:          Mock,
		errorIsExpected:    false,
	},
	{
		name:               "Wrong TransitGateway Attachment and Conflicting Prefix List Targets",
		vpc:                &vpcconfapi.VPC{AccountID: ownerID, ID: vpcMockID},
		TGWIsShared:        false,
		hasAnotherTGWShare: true,
		numRouteTables:     0,
		routesExpected:     []string{},
		routeType:          Conflicting,
		errorIsExpected:    true,
	},
	{
		name:               "Correct TransitGateway Attachment and Partial Routes",
		vpc:                &vpcconfapi.VPC{AccountID: ownerID, ID: vpcMockID},
		TGWIsShared:        true,
		hasAnotherTGWShare: false,
		numRouteTables:     numRouteTables,
		routesExpected:     tgwRoutes[getPartialRouteLen():],
		routeType:          Partial,
		errorIsExpected:    false,
	},
	{
		name:               "Correct TransitGateway Attachment and All Routes",
		vpc:                &vpcconfapi.VPC{AccountID: ownerID, ID: vpcMockID},
		TGWIsShared:        true,
		hasAnotherTGWShare: false,
		numRouteTables:     0,
		routesExpected:     []string{},
		routeType:          Matching,
		errorIsExpected:    false,
	},
}

func TestCreateRoutesSuccess(t *testing.T) {
	for _, tc := range inspectTestCases {
		ex := &exceptionVPC{
			vpc:             *tc.vpc,
			TGWIsAssociated: true,
			routeTables: []routeTable{
				{
					id:           fakeAWSID("rt"),
					routesNeeded: tgwRoutes,
				},
				{
					id:           fakeAWSID("rt"),
					routesNeeded: tgwRoutes[getPartialRouteLen():],
				},
			},
		}

		mock := &mockEC2{createRouteSuccess: true, createdRoutes: map[string][]string{}}

		err := createRoutes(mock, *ex, transitGatewayID, false)
		if err != nil {
			t.Error(err)
		}
		for _, rt := range ex.routeTables {
			if !reflect.DeepEqual(rt.routesNeeded, mock.createdRoutes[rt.id]) {
				t.Errorf("Expected %s routes %v to match requested routes %v", rt.id, mock.createdRoutes[rt.id], rt.routesNeeded)
			}
		}
	}
}

func TestCreateRoutesCIDRError(t *testing.T) {
	for _, tc := range inspectTestCases {
		cidrRoute := "10.0.0.0/16"
		ex := &exceptionVPC{
			vpc:             *tc.vpc,
			TGWIsAssociated: true,
			routeTables: []routeTable{
				{
					id:           fakeAWSID("rt"),
					routesNeeded: []string{cidrRoute},
				},
				{
					id:           fakeAWSID("rt"),
					routesNeeded: []string{cidrRoute},
				},
			},
		}

		mock := &mockEC2{createRouteSuccess: true, createdRoutes: map[string][]string{}}
		expectedError := fmt.Errorf("A CIDR route %s was passed to CreateRoute when only prefix list routes were expected", cidrRoute)

		err := createRoutes(mock, *ex, transitGatewayID, false)
		if err == nil {
			t.Error("CreateRoute returned success when it should have failed.")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("CreateRoute returned the following error instead of the expected error: %s", err.Error())
		}
		if len(mock.createdRoutes) != 0 {
			t.Errorf("Expected number of created routes to be 0, but got %d", len(mock.createdRoutes))
		}
	}
}

func TestCreateRoutesError(t *testing.T) {
	for _, tc := range inspectTestCases {
		errorRT := fakeAWSID("rt")
		ex := &exceptionVPC{
			vpc:             *tc.vpc,
			TGWIsAssociated: true,
			routeTables: []routeTable{
				{
					id:           errorRT,
					routesNeeded: tgwRoutes,
				},
				{
					id:           fakeAWSID("rt"),
					routesNeeded: tgwRoutes[getPartialRouteLen():],
				},
			},
		}

		mock := &mockEC2{createRouteSuccess: false, createdRoutes: map[string][]string{}}
		expectedCreateRouteError := fmt.Errorf("Failed to create route: %s -> %s -> %s", errorRT, tgwRoutes[0], transitGatewayID)
		expectedError := fmt.Errorf("Failed to create route for %s: %s -> %s on %s - %s", vpcMockID, tgwRoutes[0], transitGatewayID, errorRT, expectedCreateRouteError)

		err := createRoutes(mock, *ex, transitGatewayID, false)
		if err == nil {
			t.Error("CreateRoute returned success when it should have failed.")
		}
		if err.Error() != expectedError.Error() {
			t.Errorf("CreateRoute returned the following error instead of the expected error: %s", err.Error())
		}
		if len(mock.createdRoutes) != 0 {
			t.Errorf("Expected number of created routes to be 0, but got %d", len(mock.createdRoutes))
		}
	}
}
