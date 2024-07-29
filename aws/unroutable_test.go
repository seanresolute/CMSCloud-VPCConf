package aws

import (
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/ipcontrol"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var mustParseCIDRTests = []struct {
	name        string
	cidr        string
	expectPanic bool
}{
	{
		name:        "Parse valid CIDR",
		cidr:        "100.64.0.0/16",
		expectPanic: false,
	},
	{
		name:        "Parse invalid CIDR and panic",
		cidr:        "0/100",
		expectPanic: true,
	},
}

func TestMustParseCIDR(t *testing.T) {
	for _, test := range mustParseCIDRTests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r != nil && !test.expectPanic {
					t.Errorf("mustParseCIDR panicked with valid input")
				}
				if r == nil && test.expectPanic {
					t.Error("mustParseCIDR did not panic with invalid input")
				}
			}()

			mustParseCIDR(test.cidr)
		})
	}
}

var unroutableCIDRTests = []struct {
	name       string
	inUseCIDRs []string
}{
	{
		name:       "First unroutable allocation",
		inUseCIDRs: []string{},
	},
	{
		name:       "Third unroutable allocation",
		inUseCIDRs: []string{"100.64.0.0/16", "100.65.0.0/16"},
	},
	{
		name:       "Fragmented unroutable allocation",
		inUseCIDRs: []string{"100.64.0.0/16", "100.96.0.0/16", "100.114.0.0/16", "100.127.0.0/16"},
	},
}

func TestRandomUnroutable(t *testing.T) {
	for _, test := range unroutableCIDRTests {
		t.Run(test.name, func(t *testing.T) {
			cidr, octet, err := getRandomUnroutableCIDR(test.inUseCIDRs)
			if err != nil {
				t.Error(err)
			}

			if !strings.Contains(cidr, strconv.Itoa(int(octet))) {
				t.Errorf("Octet %d was not present in cidr %s", octet, cidr)
			}
			if stringInSlice(cidr, test.inUseCIDRs) {
				t.Errorf("Random CIDR %s was present in already used CIDRs %#v", cidr, test.inUseCIDRs)
			}
		})
	}
}

func TestRandomUnroutableExpansion(t *testing.T) {
	inUseCIDRs := []string{}
	uniqueCIDRs := map[string]struct{}{}

	for i := unroutableStart; i <= unroutableEnd; i++ {
		cidr, _, err := getRandomUnroutableCIDR(inUseCIDRs)
		if err != nil {
			t.Errorf("Expected no error, but got %s", err)
		}
		inUseCIDRs = append(inUseCIDRs, cidr)
		uniqueCIDRs[cidr] = struct{}{}
	}

	if len(inUseCIDRs) != 64 {
		t.Errorf("Expected inUseCIDRs to have 64 entries, but got %d", len(inUseCIDRs))
	}

	if len(uniqueCIDRs) != 64 {
		t.Errorf("Expected uniqueCIDRs to have 64 entries (no duplicates), but got %d", len(uniqueCIDRs))
	}

	_, _, err := getRandomUnroutableCIDR(inUseCIDRs)
	if err == nil {
		t.Errorf("Expected no available unroutable CIDR, but got %s", err)
	}
}

var getSubnetCIDRsTests = []struct {
	name          string
	numberOfAZs   int
	octet         uint
	expectedCIDRs []string
}{
	{
		name:          "1 AZ, no CIDRs in use",
		numberOfAZs:   1,
		octet:         64,
		expectedCIDRs: []string{"100.64.0.0/16"},
	},
	{
		name:          "2 AZs, 1 CIDR in use",
		numberOfAZs:   2,
		octet:         65,
		expectedCIDRs: []string{"100.65.0.0/17", "100.65.128.0/17"},
	},
	{
		name:          "4 AZs, 2 CIDRs in use",
		numberOfAZs:   4,
		octet:         66,
		expectedCIDRs: []string{"100.66.0.0/18", "100.66.64.0/18", "100.66.128.0/18", "100.66.192.0/18"},
	},
	{
		name:          "8 AZs, 3 CIDRs in use",
		numberOfAZs:   8,
		octet:         67,
		expectedCIDRs: []string{"100.67.0.0/19", "100.67.32.0/19", "100.67.64.0/19", "100.67.96.0/19", "100.67.128.0/19", "100.67.160.0/19", "100.67.192.0/19", "100.67.224.0/19"},
	},
}

func TestGetSubnetCIDRs(t *testing.T) {
	for _, test := range getSubnetCIDRsTests {
		t.Run(test.name, func(t *testing.T) {
			subnets, err := getSubnetCIDRs(test.numberOfAZs, test.octet)
			if err != nil {
				t.Error(err)
			}
			if !reflect.DeepEqual(test.expectedCIDRs, subnets) {
				t.Errorf("Expected %d AZs to return %#v but got %#v", test.numberOfAZs, test.expectedCIDRs, subnets)
			}
		})
	}
}

var addUnroutableSubnetsTests = []struct {
	name            string
	vpcInfo         *ipcontrol.VPCInfo
	groupName       string
	output          *ec2.DescribeVpcsOutput
	errorExpected   bool
	cidrExpected    []string // this will always be one CIDR for unroutables
	subnetsExpected []*ipcontrol.SubnetInfo
	peeredCIDRS     []string
}{
	{
		name: "First unroutable, 1 AZ",
		vpcInfo: &ipcontrol.VPCInfo{
			Name:              "test-vpc",
			AvailabilityZones: []string{"us-east-1a"},
		},
		groupName: "unroutable",
		output: &ec2.DescribeVpcsOutput{
			Vpcs: []*ec2.Vpc{
				{
					CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
						{
							CidrBlock: aws.String("10.10.10.0/24"), // non-unroutable
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
					},
				},
			},
		},
		peeredCIDRS:  []string{},
		cidrExpected: []string{"100.64.0.0/16"},
		subnetsExpected: []*ipcontrol.SubnetInfo{
			{
				Name:             "test-vpc-unroutable-a",
				CIDR:             "100.64.0.0/16",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1a",
				GroupName:        "unroutable",
			},
		},
	},
	{
		name: "Second unroutable, 2 AZs",
		vpcInfo: &ipcontrol.VPCInfo{
			Name:              "test-vpc",
			AvailabilityZones: []string{"us-east-1a", "us-east-1b"},
		},
		groupName: "unroutable-1",
		output: &ec2.DescribeVpcsOutput{
			Vpcs: []*ec2.Vpc{
				{
					CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
						{
							CidrBlock: aws.String("100.64.0.0/16"), // first unroutable
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
					},
				},
			},
		},
		peeredCIDRS:  []string{"100.90.0.0/16"},
		cidrExpected: []string{"100.69.0.0/16"},
		subnetsExpected: []*ipcontrol.SubnetInfo{
			{
				Name:             "test-vpc-unroutable-1-a",
				CIDR:             "100.69.0.0/17",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1a",
				GroupName:        "unroutable-1",
			},
			{
				Name:             "test-vpc-unroutable-1-b",
				CIDR:             "100.69.128.0/17",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1b",
				GroupName:        "unroutable-1",
			},
		},
	},
	{
		name: "Second unroutable, 3 AZs",
		vpcInfo: &ipcontrol.VPCInfo{
			Name:              "test-vpc",
			AvailabilityZones: []string{"us-east-1b", "us-east-1c", "us-east-1a"},
		},
		groupName: "unroutable",
		output: &ec2.DescribeVpcsOutput{
			Vpcs: []*ec2.Vpc{
				{
					CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
						{
							CidrBlock: aws.String("100.94.0.0/16"), // first unroutable
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("disassociated"), // now disassociated
							},
						},
					},
				},
			},
		},
		peeredCIDRS:  []string{"100.90.0.0/16", "100.65.0.0/16", "100.79.0.0/16", "100.100.0.0/16"},
		cidrExpected: []string{"100.82.0.0/16"},
		subnetsExpected: []*ipcontrol.SubnetInfo{
			{
				Name:             "test-vpc-unroutable-a",
				CIDR:             "100.82.0.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1a",
				GroupName:        "unroutable",
			},
			{
				Name:             "test-vpc-unroutable-b",
				CIDR:             "100.82.64.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1b",
				GroupName:        "unroutable",
			},
			{
				Name:             "test-vpc-unroutable-c",
				CIDR:             "100.82.128.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1c",
				GroupName:        "unroutable",
			},
		},
	},
	{
		name: "4 AZs",
		vpcInfo: &ipcontrol.VPCInfo{
			Name:              "test-vpc",
			AvailabilityZones: []string{"us-east-1d", "us-east-1a", "us-east-1b", "us-east-1c"},
		},
		groupName: "unroutable",
		output: &ec2.DescribeVpcsOutput{
			Vpcs: []*ec2.Vpc{
				{
					CidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
						{
							CidrBlock: aws.String("100.64.0.0/16"),
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
						{
							CidrBlock: aws.String("100.65.0.0/16"),
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
						{
							CidrBlock: aws.String("100.67.0.0/16"),
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
						{
							CidrBlock: aws.String("100.68.0.0/16"),
							CidrBlockState: &ec2.VpcCidrBlockState{
								State: aws.String("associated"),
							},
						},
					},
				},
			},
		},
		peeredCIDRS:  []string{"100.90.0.0/16", "100.79.0.0/16", "100.120.0.0/16"},
		cidrExpected: []string{"100.78.0.0/16"},
		subnetsExpected: []*ipcontrol.SubnetInfo{
			{
				Name:             "test-vpc-unroutable-a",
				CIDR:             "100.78.0.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1a",
				GroupName:        "unroutable",
			},
			{
				Name:             "test-vpc-unroutable-b",
				CIDR:             "100.78.64.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1b",
				GroupName:        "unroutable",
			},
			{
				Name:             "test-vpc-unroutable-c",
				CIDR:             "100.78.128.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1c",
				GroupName:        "unroutable",
			},
			{
				Name:             "test-vpc-unroutable-d",
				CIDR:             "100.78.192.0/18",
				Type:             "Unroutable",
				AvailabilityZone: "us-east-1d",
				GroupName:        "unroutable",
			},
		},
	},
}

func TestAddUnroutableSubnets(t *testing.T) {
	for _, test := range addUnroutableSubnetsTests {
		t.Run(test.name, func(t *testing.T) {
			rand.Seed(31337) // rand.Int31n returns 0
			err := AddUnroutableSubnets(test.vpcInfo, test.groupName, test.output, test.peeredCIDRS)
			if err != nil && !test.errorExpected {
				t.Errorf("No error expected but got %q", err)
			}
			if !reflect.DeepEqual(test.vpcInfo.NewCIDRs, test.cidrExpected) {
				t.Errorf("NewCIDRs returned %#v does not match cidrsExpected %#v", test.vpcInfo.NewCIDRs, test.cidrExpected)
			}
			if !reflect.DeepEqual(test.vpcInfo.NewSubnets, test.subnetsExpected) {
				t.Errorf("NewSubnets returned does not match subnetsExpected")
			}
		})
	}
}

var getUnroutableSuperNetworkTests = []struct {
	name             string
	subnetCIDR       string
	supernetExpected string
	errorExpected    bool
}{
	{
		name:             "Valid subnet CIDR /16",
		subnetCIDR:       "100.64.0.0/16",
		supernetExpected: "100.64.0.0/16",
		errorExpected:    false,
	},
	{
		name:             "Valid subnet CIDR /17",
		subnetCIDR:       "100.65.128.0/17",
		supernetExpected: "100.65.0.0/16",
		errorExpected:    false,
	},
	{
		name:             "Valid subnet CIDR /18",
		subnetCIDR:       "100.66.64.0/18",
		supernetExpected: "100.66.0.0/16",
		errorExpected:    false,
	},
	{
		name:             "Valid subnet CIDR /19",
		subnetCIDR:       "100.67.32.0/19",
		supernetExpected: "100.67.0.0/16",
		errorExpected:    false,
	},
	{
		name:             "Non-unroutable subnet CIDR",
		subnetCIDR:       "10.0.0.0/8",
		supernetExpected: "",
		errorExpected:    true,
	},
	{
		name:             "Before unroutable range subnet CIDR",
		subnetCIDR:       "100.63.0.0/17",
		supernetExpected: "",
		errorExpected:    true,
	},
	{
		name:             "After unroutable range subnet CIDR",
		subnetCIDR:       "100.128.0.0/17",
		supernetExpected: "",
		errorExpected:    true,
	},
}

func TestGetUnroutableSuperNetwork(t *testing.T) {
	for _, test := range getUnroutableSuperNetworkTests {
		t.Run(test.name, func(t *testing.T) {
			supernet, err := GetUnroutableSupernet(test.subnetCIDR)
			if err != nil && !test.errorExpected {
				t.Error(err)
			} else if err != nil && test.errorExpected && !strings.Contains(err.Error(), "does not belong to") {
				t.Errorf("Got unexpected error %q", err.Error())
			} else if supernet != nil && supernet.String() != test.supernetExpected {
				t.Errorf("Expected %s as the parent CIDR, but got %s", test.supernetExpected, supernet.String())
			}
		})
	}
}
