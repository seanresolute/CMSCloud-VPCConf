package aws

import (
	"fmt"
	"math/rand"
	"net"
	"sort"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/ipcontrol"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	unroutableStart uint = 64
	unroutableEnd   uint = 127
)

var unroutableNetwork = mustParseCIDR("100.64.0.0/10")

func mustParseCIDR(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse CIDR %q", cidr))
	}

	return network
}

func getVPCUnroutableCIDRs(output *ec2.DescribeVpcsOutput) ([]string, error) {
	unroutableCIDRS := []string{}

	for _, association := range output.Vpcs[0].CidrBlockAssociationSet {
		if aws.StringValue(association.CidrBlockState.State) == "disassociated" {
			continue
		}
		cidr := aws.StringValue(association.CidrBlock)
		if cidr == "" {
			continue
		}
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return unroutableCIDRS, err
		}
		if unroutableNetwork.Contains(ip) {
			unroutableCIDRS = append(unroutableCIDRS, cidr)
		}
	}

	return unroutableCIDRS, nil
}

// return a random unroutable CIDR to reduce the chance of collision for peered VPCs
func getRandomUnroutableCIDR(inUseCIDRs []string) (string, uint, error) {
	availableCIDRs := []string{}

	for octet := unroutableStart; octet <= unroutableEnd; octet++ {
		cidr := fmt.Sprintf("100.%d.0.0/16", octet)
		if !stringInSlice(cidr, inUseCIDRs) {
			availableCIDRs = append(availableCIDRs, cidr)
		}
	}

	if len(availableCIDRs) == 0 {
		return "", 0, fmt.Errorf("No unroutable subnets available")
	}

	idx := rand.Intn(len(availableCIDRs))

	_, network, err := net.ParseCIDR(availableCIDRs[idx])
	if err != nil {
		return "", 0, err
	}

	return availableCIDRs[idx], uint(network.IP[1]), nil
}

func getSubnetSize(numberOfAZs int) (int, error) {
	subnetSize := 16

	if numberOfAZs > 8 {
		return 0, fmt.Errorf("More than 8 CIDRs is not supported - asked for %d AZs", numberOfAZs)
	} else if numberOfAZs > 4 {
		subnetSize += 3
	} else if numberOfAZs > 2 {
		subnetSize += 2
	} else if numberOfAZs > 1 {
		subnetSize += 1
	}

	return subnetSize, nil
}

func getSubnetCIDRs(numberOfAZs int, octet uint) ([]string, error) {
	cidrs := []string{}

	subnetSize, err := getSubnetSize(numberOfAZs)
	if err != nil {
		return cidrs, err
	}

	// Subnet IPs: 2^(32 - subnetSize)
	// You get 2^8 from the last octet
	// So you need 2^(32 - subnetSize) / 2^8 = 2^(24 - subnetSize) values in the third octet for each subnet
	// So the subnets are 2^(24 - subnetSize) = 1 << (24 - subnetSize) values apart
	increment := 1 << (24 - subnetSize)

	for i := 0; i < 256; i += increment {
		cidrs = append(cidrs, fmt.Sprintf("100.%d.%d.0/%d", octet, i, subnetSize))
	}

	return cidrs, nil
}

// AddUnroutableSubnets adds the new CIDR and subnets to the given vpcInfo instance
func AddUnroutableSubnets(vpcInfo *ipcontrol.VPCInfo, groupName string, output *ec2.DescribeVpcsOutput, peeringCIDRs []string) error {
	numAZs := len(vpcInfo.AvailabilityZones)
	if numAZs > 8 {
		return fmt.Errorf("More than 8 CIDRs is not supported - asked for %d AZs", numAZs)
	}

	unroutableCIDRS, err := getVPCUnroutableCIDRs(output)
	if err != nil {
		return err
	}

	nextUnroutableCIDR, octet, err := getRandomUnroutableCIDR(append(unroutableCIDRS, peeringCIDRs...))
	if err != nil {
		return err
	}

	vpcInfo.NewCIDRs = []string{nextUnroutableCIDR}
	vpcInfo.NewSubnets = []*ipcontrol.SubnetInfo{}

	subnetCIDRs, err := getSubnetCIDRs(numAZs, octet)
	if err != nil {
		return err
	}

	sort.Strings(vpcInfo.AvailabilityZones) // keep the subnets in order per AZ

	for i, az := range vpcInfo.AvailabilityZones {
		if len(az) < 1 {
			return fmt.Errorf("Invalid empty AZ name")
		}
		if az[len(az)-1] < 'a' || az[len(az)-1] > 'z' {
			return fmt.Errorf("Unexpected AZ name %s", az)
		}
		for j := 0; j < i; j++ {
			az2 := vpcInfo.AvailabilityZones[j]
			if az[len(az)-1] == az2[len(az2)-1] {
				return fmt.Errorf("Conflicting AZ names %s and %s", az, az2)
			}
		}

		vpcInfo.NewSubnets = append(vpcInfo.NewSubnets, &ipcontrol.SubnetInfo{
			Name:             fmt.Sprintf("%s-%s-%c", vpcInfo.Name, groupName, az[len(az)-1]),
			Type:             database.SubnetTypeUnroutable,
			AvailabilityZone: az,
			CIDR:             subnetCIDRs[i],
			GroupName:        groupName,
		})
	}

	return nil
}

func FilterUnroutableCIDRBlocks(block []*ec2.CidrBlock) ([]string, error) {
	unroutableCIDRs := []string{}

	for _, set := range block {
		cidr := aws.StringValue(set.CidrBlock)
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return []string{}, fmt.Errorf("Unable to parse CIDR %q", cidr)
		}
		if unroutableNetwork.Contains(ip) {
			unroutableCIDRs = append(unroutableCIDRs, cidr)
		}
	}

	return unroutableCIDRs, nil
}

// GetUnroutableSupernet returns the /16 supernet of the given CIDR if it is within the unorutable network range
func GetUnroutableSupernet(cidr string) (*net.IPNet, error) {
	_, subnetwork, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	if !unroutableNetwork.Contains(subnetwork.IP) {
		return nil, fmt.Errorf("SubnetType of Unroutable is incorrect, %s does not belong to %s", subnetwork.IP, unroutableNetwork)
	}

	_, supernet, err := net.ParseCIDR(fmt.Sprintf("%d.%d.0.0/16", subnetwork.IP[0], subnetwork.IP[1]))
	if err != nil {
		return nil, err
	}

	return supernet, nil
}
