package aws_test

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
)

var testClock = clock.NewMock()

// singletonMockEC2 represents an EC2 that has up to a single resource of
// each type, and all you can do is Describe and filter by ID
type singletonMockEC2 struct {
	ids  map[string]string // resource type -> id of singleton
	errs map[string]error  // resource type -> error when Describing
	ec2iface.EC2API
}

func getFilter(filters []*ec2.Filter, name string) (string, error) {
	if len(filters) != 1 {
		return "", fmt.Errorf("Mocks only support 1 filter")
	}
	filter := filters[0]
	if aws.StringValue(filter.Name) != name {
		return "", fmt.Errorf("Mock only supports filtering on %q", name)
	}
	if len(filter.Values) != 1 {
		return "", fmt.Errorf("Mocks only support 1 filter value")
	}
	return aws.StringValue(filter.Values[0]), nil
}

func (m *singletonMockEC2) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	if len(input.VpcIds) != 0 {
		return nil, fmt.Errorf("DescribeVpcs mock does not support VpcIds")
	}
	output := &ec2.DescribeVpcsOutput{}
	for _, filter := range input.Filters {
		if aws.StringValue(filter.Name) == "vpc-id" {
			vpcID, err := getFilter(input.Filters, "vpc-id")
			if err != nil {
				return nil, err
			}
			if expectedVPCErr, ok := m.errs["vpc"]; ok {
				return nil, expectedVPCErr
			}
			if expectedVPCID, ok := m.ids["vpc"]; ok && expectedVPCID == vpcID {
				output.Vpcs = append(output.Vpcs, &ec2.Vpc{})
			}

		} else if aws.StringValue(filter.Name) == "cidr-block-association.association-id" {
			assocID, err := getFilter(input.Filters, "cidr-block-association.association-id")
			if err != nil {
				return nil, err
			}
			if expectedAssocErr, ok := m.errs["vpc-cidr-assoc"]; ok {
				return nil, expectedAssocErr
			}
			if expectedAssocID, ok := m.ids["vpc-cidr-assoc"]; ok && expectedAssocID == assocID {
				output.Vpcs = append(output.Vpcs, &ec2.Vpc{})
			}
		}
	}
	return output, nil
}

func (m *singletonMockEC2) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	if len(input.GroupIds) != 0 {
		return nil, fmt.Errorf("DescribeSecurityGroups mock does not support GroupIds")
	}
	sgID, err := getFilter(input.Filters, "group-id")
	if err != nil {
		return nil, err
	}
	if expectedErr, ok := m.errs["sg"]; ok {
		return nil, expectedErr
	}
	output := &ec2.DescribeSecurityGroupsOutput{}
	if expectedSGID, ok := m.ids["sg"]; ok && expectedSGID == sgID {
		output.SecurityGroups = append(output.SecurityGroups, &ec2.SecurityGroup{})
	}
	return output, nil
}

func (m *singletonMockEC2) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	if len(input.SubnetIds) != 0 {
		return nil, fmt.Errorf("DescribeSubnets mock does not support SubnetIds")
	}
	subnetID, err := getFilter(input.Filters, "subnet-id")
	if err != nil {
		return nil, err
	}
	if expectedErr, ok := m.errs["subnet"]; ok {
		return nil, expectedErr
	}
	output := &ec2.DescribeSubnetsOutput{}
	if expectedsSubnetID, ok := m.ids["subnet"]; ok && expectedsSubnetID == subnetID {
		output.Subnets = append(output.Subnets, &ec2.Subnet{})
	}
	return output, nil
}

func (m *singletonMockEC2) DescribeInternetGateways(input *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error) {
	if len(input.InternetGatewayIds) != 0 {
		return nil, fmt.Errorf("DescribeInternetGateways mock does not support InternetGatewayIds")
	}
	igwID, err := getFilter(input.Filters, "internet-gateway-id")
	if err != nil {
		return nil, err
	}
	if expectedErr, ok := m.errs["igw"]; ok {
		return nil, expectedErr
	}
	output := &ec2.DescribeInternetGatewaysOutput{}
	if expectedIGWID, ok := m.ids["igw"]; ok && expectedIGWID == igwID {
		output.InternetGateways = append(output.InternetGateways, &ec2.InternetGateway{})
	}
	return output, nil
}

func (m *singletonMockEC2) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	if len(input.RouteTableIds) != 0 {
		return nil, fmt.Errorf("DescribeRouteTables mock does not support RouteTableIds")
	}
	rtID, err := getFilter(input.Filters, "route-table-id")
	if err != nil {
		return nil, err
	}
	if expectedErr, ok := m.errs["rtb"]; ok {
		return nil, expectedErr
	}
	output := &ec2.DescribeRouteTablesOutput{}
	if expectedRTID, ok := m.ids["rtb"]; ok && expectedRTID == rtID {
		output.RouteTables = append(output.RouteTables, &ec2.RouteTable{})
	}
	return output, nil
}

func (m *singletonMockEC2) DescribeAddresses(input *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
	if len(input.AllocationIds) != 0 {
		return nil, fmt.Errorf("DescribeAddresses mock does not support AllocationIds")
	}
	allocID, err := getFilter(input.Filters, "allocation-id")
	if err != nil {
		return nil, err
	}
	if expectedErr, ok := m.errs["eipalloc"]; ok {
		return nil, expectedErr
	}
	output := &ec2.DescribeAddressesOutput{}
	if expectedAllocID, ok := m.ids["eipalloc"]; ok && expectedAllocID == allocID {
		output.Addresses = append(output.Addresses, &ec2.Address{})
	}
	return output, nil
}

type testLogger struct{}

func (t *testLogger) Log(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

func TestExistenceChecks(t *testing.T) {
	ctx := &awsp.Context{
		Logger:           &testLogger{},
		Clock:            testClock,
		AWSAccountAccess: &awsp.AWSAccountAccess{},
	}

	type testCase struct {
		check            awsp.ExistenceCheck
		resourceType, id string
	}
	testCases := []*testCase{
		{
			check:        ctx.VPCExists,
			resourceType: "vpc",
			id:           "vpc-123",
		},
		{
			check:        ctx.SecurityGroupExists,
			resourceType: "sg",
			id:           "sg-0df5f292bc",
		},
		{
			check:        ctx.SubnetExists,
			resourceType: "subnet",
			id:           "subnet-0bb1c79de3",
		},
		{
			check:        ctx.InternetGatewayExists,
			resourceType: "igw",
			id:           "igw-036dde5c85",
		},
		{
			check:        ctx.RouteTableExists,
			resourceType: "rtb",
			id:           "rtb-098abc876",
		},
		{
			check:        ctx.CIDRBlockAssociationExists,
			resourceType: "vpc-cidr-assoc",
			id:           "vpc-cidr-assoc-0b2600f482b51c5e7",
		},
		{
			check:        ctx.EIPExists,
			resourceType: "eipalloc",
			id:           "eipalloc-12345678",
		},
	}

	for _, tc := range testCases {
		// No IDs
		ctx.EC2svc = &singletonMockEC2{}
		exists, err := tc.check(tc.id)
		if err != nil {
			t.Fatalf("%s check got an unexpected error with no IDs present: %s", tc.resourceType, err)
		}
		if exists {
			t.Fatalf("%s check returned true with no IDs present", tc.resourceType)
		}
		// Correct ID
		ctx.EC2svc = &singletonMockEC2{ids: map[string]string{tc.resourceType: tc.id}}
		exists, err = tc.check(tc.id)
		if err != nil {
			t.Fatalf("%s check got an unexpected error with correct ID present: %s", tc.resourceType, err)
		}
		if !exists {
			t.Fatalf("%s check returned false with correct ID present", tc.resourceType)
		}
		// Incorrect ID
		exists, err = tc.check(tc.id + "x")
		if err != nil {
			t.Fatalf("%s check got an unexpected error with incorrect ID present: %s", tc.resourceType, err)
		}
		if exists {
			t.Fatalf("%s check returned true with incorrect ID present", tc.resourceType)
		}
		// Error
		ctx.EC2svc = &singletonMockEC2{
			ids:  map[string]string{tc.resourceType: tc.id},
			errs: map[string]error{tc.resourceType: fmt.Errorf("Error listing %s", tc.resourceType)},
		}
		_, err = tc.check(tc.id)
		if err == nil {
			t.Fatalf("%s check did not pass on error from AWS", tc.resourceType)
		}
	}
}

type check struct {
	id     string
	exists bool
	err    error
	called bool
}

func (c *check) check(id string) (bool, error) {
	c.called = true
	if c.id != id {
		return false, fmt.Errorf("check called with wrong id %q != %q", id, c.id)
	}
	if c.err != nil {
		return false, c.err
	}
	return c.exists, nil
}

func TestWaitForExistence(t *testing.T) {
	ctx := &awsp.Context{
		Logger: &testLogger{},
		Clock:  testClock,
	}
	type returnValue struct {
		exists bool
		err    error
	}
	type testCase struct {
		name                 string
		initialReturn        *returnValue
		finalReturn          *returnValue
		delay                time.Duration
		expectedError        error
		waitForFinalExpected bool
	}
	id := "xyz"
	testCases := []*testCase{
		{
			name:                 "found immediately",
			initialReturn:        &returnValue{true, nil},
			finalReturn:          &returnValue{true, nil},
			delay:                time.Second,
			expectedError:        nil,
			waitForFinalExpected: false,
		},
		{
			name:                 "found after 10 seconds",
			initialReturn:        &returnValue{false, nil},
			finalReturn:          &returnValue{true, nil},
			delay:                10 * time.Second,
			expectedError:        nil,
			waitForFinalExpected: true,
		},
		{
			name:                 "error immediately",
			initialReturn:        &returnValue{false, errors.New("error from AWS")},
			finalReturn:          &returnValue{false, nil},
			delay:                time.Second,
			expectedError:        errors.New("error from AWS"),
			waitForFinalExpected: false,
		},
		{
			name:                 "error after 10 seconds",
			initialReturn:        &returnValue{false, nil},
			finalReturn:          &returnValue{false, errors.New("error from AWS")},
			delay:                10 * time.Second,
			expectedError:        errors.New("error from AWS"),
			waitForFinalExpected: true,
		},
		{
			name:                 "found after too long",
			initialReturn:        &returnValue{false, nil},
			finalReturn:          &returnValue{true, nil},
			delay:                2 * awsp.MaxExistenceWaitTime,
			expectedError:        fmt.Errorf("Timed out waiting for resource %s to exist", id),
			waitForFinalExpected: false,
		},
		{
			name:                 "never found",
			initialReturn:        &returnValue{false, nil},
			finalReturn:          &returnValue{false, nil},
			delay:                time.Second,
			expectedError:        fmt.Errorf("Timed out waiting for resource %s to exist", id),
			waitForFinalExpected: true,
		},
	}
	for _, tc := range testCases {
		initial := &check{id: id, exists: tc.initialReturn.exists, err: tc.initialReturn.err}
		final := &check{id: id, exists: tc.finalReturn.exists, err: tc.finalReturn.err}
		ch := make(chan error)
		currentCheck := initial
		go func() {
			ch <- ctx.WaitForExistence(id, func(id string) (bool, error) { return currentCheck.check(id) })
		}()
		time.Sleep(10 * time.Millisecond) // Give WaitForExistence a chance to do the first check
		start := testClock.Now()
		end := testClock.Now().Add(awsp.MaxExistenceWaitTime + time.Second)
		for testClock.Now().Before(end) {
			if testClock.Now().Sub(start) >= tc.delay {
				currentCheck = final
			}
			testClock.Add(time.Second)
		}
		select {
		case err := <-ch:
			if tc.expectedError == nil && err != nil {
				t.Fatalf("Test case %q: Got unexpected error: %v", tc.name, err)
			} else if tc.expectedError != nil && err == nil {
				t.Fatalf("Test case %q: Did not get expected error: %v", tc.name, tc.expectedError)
			} else if err != nil && err.Error() != tc.expectedError.Error() {
				t.Fatalf("Test case %q: Expected error: %v but got: %v", tc.name, tc.expectedError, err)
			}
		default:
			t.Fatalf("Test case %q: WaitForExistence did not return", tc.name)
		}
		if !initial.called {
			t.Fatalf("Test case %q: initial check never called", tc.name)
		}
		if tc.waitForFinalExpected && !final.called {
			t.Fatalf("Test case %q: final check never called", tc.name)
		}
		if !tc.waitForFinalExpected && final.called {
			t.Fatalf("Test case %q: final check unexpectedly called", tc.name)
		}
	}
}

func TestSplitSubnets(t *testing.T) {
	type subnet struct {
		SubnetID         string
		AvailabilityZone string
		Tags             map[string]string
		SubnetType       database.SubnetType
	}
	type testCase struct {
		Subnets []*subnet
		Error   *string
	}
	testCases := []*testCase{
		// One private, one public
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
			},
		},
		// One private, one public, one firewall
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypeFirewall,
					SubnetID:         "firewall-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "firewall",
					},
				},
			},
		},
		// One public only
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
			},
		},
		// One private only
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-d",
					AvailabilityZone: "us-west-2d",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
			},
		},
		// Two private, two public
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
			},
		},
		// Two private, two public, two data
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypeData,
					SubnetID:         "data-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "data",
					},
				},
				{
					SubnetType:       database.SubnetTypeData,
					SubnetID:         "data-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "data",
					},
				},
			},
		},
		// Two private, three public
		{
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-b",
					AvailabilityZone: "us-west-2b",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-c",
					AvailabilityZone: "us-west-2c",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "public",
					},
				},
			},
		},
		// No use
		{
			Error: aws.String("No use tag"),
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "public",
					},
				},
			},
		},
		// Invalid use
		{
			Error: aws.String("Invalid use tag"),
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "pub",
					},
				},
			},
		},
		// Invalid use: empty string
		{
			Error: aws.String("Invalid use tag"),
			Subnets: []*subnet{
				{
					SubnetType:       database.SubnetTypePrivate,
					SubnetID:         "private-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "private",
					},
				},
				{
					SubnetType:       database.SubnetTypePublic,
					SubnetID:         "public-a",
					AvailabilityZone: "us-west-2a",
					Tags: map[string]string{
						"Automated": "true",
						"use":       "",
					},
				},
			},
		},
	}

	for tidx, tc := range testCases {
		subnets := make([]*ec2.Subnet, len(tc.Subnets))
		for sidx, subnet := range tc.Subnets {
			subnets[sidx] = &ec2.Subnet{
				SubnetId:         aws.String(subnet.SubnetID),
				AvailabilityZone: aws.String(subnet.AvailabilityZone),
				Tags:             []*ec2.Tag{},
			}
			for k, v := range subnet.Tags {
				subnets[sidx].Tags = append(subnets[sidx].Tags, &ec2.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				})
			}
		}
		allSubnets, err := awsp.SplitSubnets(subnets)
		if err != nil {
			if tc.Error == nil {
				t.Errorf("Unexpected error in test case %d: %s", tidx, err)
				continue
			} else if !strings.Contains(err.Error(), *tc.Error) {
				t.Errorf("Unexpected error in test case %d: %s (expected '%s')", tidx, err, *tc.Error)
				continue
			} else {
				// Expected
				continue
			}
		} else if tc.Error != nil {
			t.Errorf("Expected error '%s' in test case %d but there was no error", *tc.Error, tidx)
			continue
		}

		indexes := map[database.SubnetType]int{}
		for sidx, subnet := range tc.Subnets {
			lookIn := allSubnets[subnet.SubnetType]
			idx := indexes[subnet.SubnetType]
			indexes[subnet.SubnetType] += 1
			if idx >= len(lookIn) {
				t.Errorf("Test case %d: '%s' missing from %s list", tidx, subnet.SubnetID, subnet.SubnetType)
			} else if lookIn[idx] != subnets[sidx] {
				t.Errorf("Test case %d: '%s' not in position %d in %s list", tidx, subnet.SubnetID, idx, subnet.SubnetType)
			}
		}
		for subnetType, idx := range indexes {
			if idx < len(allSubnets[subnetType]) {
				t.Errorf("%d extra %s subnets returned", len(allSubnets[subnetType])-idx, subnetType)
			}
		}
	}
}

func TestEnsureRouteTableAssociationExists(t *testing.T) {
	type result struct {
		AssocID     string
		ErrorString string
	}

	type testCase struct {
		Name string

		// Start state
		ExistingRouteTables []*ec2.RouteTable

		// Input
		RTID     string
		SubnetID string
		IGWID    string
		VPGID    string

		// Output
		ExpectedRouteTableAssociationsCreated  []*ec2.RouteTableAssociation
		ExpectedRouteTableAssociationsReplaced map[string]string
		ExpectedResult                         result
	}

	vpcID := "vpc-lmnop"

	ctx := &awsp.Context{
		Logger:           &testLogger{},
		VPCID:            vpcID,
		AWSAccountAccess: &awsp.AWSAccountAccess{},
	}

	testCases := []testCase{
		{
			Name: "Correct subnet association already exists",
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rtb-abc"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-existing"),
							SubnetId:                aws.String("subnet-123"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rtb-xyz"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-other"),
							SubnetId:                aws.String("subnet-789"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			RTID:     "rtb-abc",
			SubnetID: "subnet-123",
			ExpectedResult: result{
				AssocID: "rtbassoc-existing",
			},
		},

		{
			Name: "Correct igw association already exists",
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rtb-abc"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-existing"),
							GatewayId:               aws.String("igw-456"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			RTID:  "rtb-abc",
			IGWID: "igw-456",
			ExpectedResult: result{
				AssocID: "rtbassoc-existing",
			},
		},

		{
			Name: "Create new subnet association",
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rtb-xyz"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-other"),
							SubnetId:                aws.String("subnet-789"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			RTID:     "rtb-abc",
			SubnetID: "subnet-123",
			ExpectedResult: result{
				AssocID: "assoc-1",
			},

			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rtb-abc"),
					SubnetId:                aws.String("subnet-123"),
					AssociationState: &ec2.RouteTableAssociationState{
						State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
					},
				},
			},
		},

		{
			Name:  "Create new igw association",
			RTID:  "rtb-abc",
			IGWID: "igw-456",
			ExpectedResult: result{
				AssocID: "assoc-1",
			},
			ExpectedRouteTableAssociationsCreated: []*ec2.RouteTableAssociation{
				{
					RouteTableAssociationId: aws.String("assoc-1"),
					RouteTableId:            aws.String("rtb-abc"),
					GatewayId:               aws.String("igw-456"),
					AssociationState: &ec2.RouteTableAssociationState{
						State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
					},
				},
			},
		},

		{
			Name: "Replace existing subnet association",
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rtb-abc"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-existing"),
							SubnetId:                aws.String("subnet-123"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
				{
					RouteTableId: aws.String("rtb-xyz"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-other"),
							SubnetId:                aws.String("subnet-789"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			RTID:     "rtb-def",
			SubnetID: "subnet-123",
			ExpectedResult: result{
				AssocID: "assoc-1",
			},
			ExpectedRouteTableAssociationsReplaced: map[string]string{
				"rtbassoc-existing": "rtb-def",
			},
		},

		{
			Name: "Replace existing igw association",
			ExistingRouteTables: []*ec2.RouteTable{
				{
					RouteTableId: aws.String("rtb-abc"),
					Associations: []*ec2.RouteTableAssociation{
						{
							RouteTableAssociationId: aws.String("rtbassoc-existing"),
							GatewayId:               aws.String("igw-123"),
							AssociationState: &ec2.RouteTableAssociationState{
								State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
							},
						},
					},
				},
			},
			RTID:  "rtb-def",
			IGWID: "igw-123",
			ExpectedResult: result{
				AssocID: "assoc-1",
			},
			ExpectedRouteTableAssociationsReplaced: map[string]string{
				"rtbassoc-existing": "rtb-def",
			},
		},

		{
			Name:  "Error: unknown resource type",
			RTID:  "rtb-abc",
			VPGID: "vpg-123",
			ExpectedResult: result{
				ErrorString: "Unknown resource ID vpg-123",
			},
		},
	}

	vpcToIGW := make(map[string]string)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var resourceID string

			if tc.SubnetID != "" {
				resourceID = tc.SubnetID
			} else if tc.IGWID != "" {
				resourceID = tc.IGWID
				vpcToIGW[vpcID] = tc.IGWID
			} else if tc.VPGID != "" {
				resourceID = tc.VPGID
			} else {
				t.Fatalf("Test case has no resource ID")
			}

			mock := &testmocks.MockEC2{
				RouteTables: tc.ExistingRouteTables,
			}
			ctx.EC2svc = mock

			assocID, err := ctx.EnsureRouteTableAssociationExists(tc.RTID, resourceID)

			var actualErrorString string
			if err != nil {
				actualErrorString = err.Error()
			}
			actualResult := result{
				AssocID:     assocID,
				ErrorString: actualErrorString,
			}
			if diff := cmp.Diff(tc.ExpectedResult, actualResult); diff != "" {
				t.Fatalf("Expected result did not match actual result: \n%s", diff)
			}

			if diff := cmp.Diff(tc.ExpectedRouteTableAssociationsCreated, mock.RouteTableAssociationsCreated); diff != "" {
				t.Fatalf("Expected created associations did not match created associations: \n%s", diff)
			}

			if diff := cmp.Diff(tc.ExpectedRouteTableAssociationsReplaced, mock.RouteTableAssociationsReplaced); diff != "" {
				t.Fatalf("Expected replaced associations did not match created associations: \n%s", diff)
			}
		})
	}
}
