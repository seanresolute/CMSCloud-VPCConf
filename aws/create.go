package aws

import (
	"fmt"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/ipcontrol"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/waiter"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/aws/aws-sdk-go/service/route53resolver"
)

const ForbidEC2Tag string = "forbid_ec2"
const FirewallTypeKey string = "VPC Conf VPC Type"
const FirewallTypeValue string = "V1Firewall"
const NoLoggingConfigChanges string = "Given logging configuration has no changes."
const firewallKey string = "Purpose"
const firewallValue string = "cms-cloud-net-fw"
const ManagedStatefulRuleGroupName = "cms-cloud-stateful-rg"
const ManagedStatelessRuleGroupName = "cms-cloud-stateless-rg"

// Resource types within Resource Access Manager
const (
	ResourceTypeUndefined    = "test:Undefined"
	ResourceTypeResolverRule = "route53resolver:ResolverRule"
	ResourceTypePrefixList   = "ec2:PrefixList"
)

// per SecOps
const forwardToSFEAction string = "aws:forward_to_sfe"
const statelessRulePriority int64 = 2

type destructor func() error

func isSubnetID(id string) bool {
	return strings.HasPrefix(id, "subnet-")
}

func IsPrefixListID(dest string) bool {
	return strings.HasPrefix(dest, "pl-")
}

func IsSecurityGroupID(dest string) bool {
	return strings.HasPrefix(dest, "sg-")
}

func IsInternetGatewayID(id string) bool {
	return strings.HasPrefix(id, "igw-")
}

func IsVPCEndpointID(id string) bool {
	return strings.HasPrefix(id, "vpce-")
}

func GenerateSubnetMappings(ids []string) (m []*networkfirewall.SubnetMapping) {
	for _, id := range ids {
		m = append(m, &networkfirewall.SubnetMapping{SubnetId: aws.String(id)})
	}
	return m
}

func (ctx *Context) defaultFirewallPolicyName() string {
	return fmt.Sprintf("cms-cloud-%s-default-fp", ctx.VPCID)
}

func (ctx *Context) FirewallName() string {
	return fmt.Sprintf("cms-cloud-%s-net-fw", ctx.VPCID)
}

func (ctx *Context) SetNameAndAutomated(id string, name string) error {
	return ctx.Tag(id, map[string]string{
		"Name":      name,
		"Automated": "true",
	})
}

func (ctx *Context) Tag(id string, tags map[string]string) error {
	ec2Tags := []*ec2.Tag{}
	for k, v := range tags {
		ec2Tags = append(ec2Tags, &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	_, err := ctx.EC2().CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{&id},
		Tags:      ec2Tags,
	})
	return err
}

func (ctx *Context) GetAvailabilityZones() (map[string]string, error) {
	out, err := ctx.EC2().DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return nil, err
	}
	azList := make(map[string]string)
	for _, az := range out.AvailabilityZones {
		azList[aws.StringValue(az.ZoneName)] = aws.StringValue(az.ZoneId)
	}
	return azList, nil
}

func (ctx *Context) DeleteTags(id string, tags map[string]string) error {
	ec2Tags := []*ec2.Tag{}
	for k, v := range tags {
		ec2Tags = append(ec2Tags, &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	_, err := ctx.EC2().DeleteTags(&ec2.DeleteTagsInput{
		Resources: []*string{&id},
		Tags:      ec2Tags,
	})
	return err
}

func (ctx *Context) SubnetExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Subnets) > 0, nil
}

func (ctx *Context) CreateSubnets(info *ipcontrol.VPCInfo) error {
	for _, subnet := range info.NewSubnets {
		use := strings.ToLower(string(subnet.Type))
		tags := []*ec2.Tag{
			{Key: aws.String("Name"), Value: aws.String(subnet.Name)},
			{Key: aws.String("GroupName"), Value: aws.String(subnet.GroupName)},
			{Key: aws.String("use"), Value: aws.String(use)},
			{Key: aws.String("stack"), Value: aws.String(info.Stack)},
			{Key: aws.String("Automated"), Value: aws.String("true")},
		}
		if subnet.Type == database.SubnetTypeUnroutable || subnet.Type == database.SubnetTypeFirewall {
			tags = append(tags, &ec2.Tag{Key: aws.String(ForbidEC2Tag), Value: aws.String("true")})
		}
		tagSpecification := &ec2.TagSpecification{
			ResourceType: aws.String("subnet"),
			Tags:         tags,
		}
		createSubnetOut, err := ctx.EC2().CreateSubnet(&ec2.CreateSubnetInput{
			AvailabilityZone:  &subnet.AvailabilityZone,
			CidrBlock:         &subnet.CIDR,
			VpcId:             &info.ResourceID,
			TagSpecifications: []*ec2.TagSpecification{tagSpecification},
		})
		if err != nil {
			return fmt.Errorf("Error creating subnet: %s", err)
		}
		ctx.destructors = append(ctx.destructors, func() error {
			_, err := ctx.EC2().DeleteSubnet(&ec2.DeleteSubnetInput{SubnetId: createSubnetOut.Subnet.SubnetId})
			if err == nil {
				ctx.Logger.Log("Deleted subnet %s", *createSubnetOut.Subnet.SubnetId)
			}
			return err
		})
		subnet.ResourceID = *createSubnetOut.Subnet.SubnetId
		err = ctx.WaitForExistence(*createSubnetOut.Subnet.SubnetId, ctx.SubnetExists)
		if err != nil {
			return err
		}
		ctx.Logger.Log("Created subnet %s", subnet.ResourceID)
	}
	return nil
}

func (awsctx *Context) GetCIDRsForPeers() ([]string, error) {
	peeringCIDRs := []string{}

	for _, side := range []string{"requester-vpc-info.vpc-id", "accepter-vpc-info.vpc-id"} {
		peering, err := awsctx.EC2().DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("status-code"),
					Values: []*string{aws.String("active")},
				},
				{
					Name:   aws.String(side),
					Values: []*string{aws.String(awsctx.VPCID)},
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("Failed to describe peering connections for %s", awsctx.VPCID)
		}

		for _, vpcPeeringConnections := range peering.VpcPeeringConnections {
			acceptCIDRs, err := FilterUnroutableCIDRBlocks(vpcPeeringConnections.AccepterVpcInfo.CidrBlockSet)
			if err != nil {
				return nil, fmt.Errorf("Failed to get accepter peering connections for %s", awsctx.VPCID)
			}
			requestCIDRs, err := FilterUnroutableCIDRBlocks(vpcPeeringConnections.RequesterVpcInfo.CidrBlockSet)
			if err != nil {
				return nil, fmt.Errorf("Failed to get requester peering connections for %s", awsctx.VPCID)
			}
			peeringCIDRs = append(peeringCIDRs, acceptCIDRs...)
			peeringCIDRs = append(peeringCIDRs, requestCIDRs...)
		}
	}
	return peeringCIDRs, nil
}

func (ctx *Context) CIDRBlockAssociationExists(assocID string) (bool, error) {
	out, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("cidr-block-association.association-id"),
				Values: []*string{aws.String(assocID)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Vpcs) > 0, nil
}

func (ctx *Context) AddCIDRBlock(cidr string) error {
	assoc, err := ctx.EC2().AssociateVpcCidrBlock(&ec2.AssociateVpcCidrBlockInput{
		VpcId:     &ctx.VPCID,
		CidrBlock: &cidr,
	})
	if err != nil {
		return fmt.Errorf("Error associating new CIDR block: %s", err)
	}
	ctx.destructors = append(ctx.destructors, func() error {
		_, err := ctx.EC2().DisassociateVpcCidrBlock(&ec2.DisassociateVpcCidrBlockInput{
			AssociationId: assoc.CidrBlockAssociation.AssociationId,
		})
		if err == nil {
			ctx.Logger.Log("Dissasociated CIDR block %s", cidr)
		}
		return err
	})
	err = ctx.WaitForExistence(*assoc.CidrBlockAssociation.AssociationId, ctx.CIDRBlockAssociationExists)
	if err != nil {
		return err
	}
	ctx.Logger.Log("Associated CIDR block %s", cidr)
	return nil
}

type ExistenceCheck func(id string) (bool, error)

// 10 minutes (increased because network firewalls sometimes take 7-8 minutes)
const MaxExistenceWaitTime = time.Second * 600

func (ctx *Context) WaitForExistence(id string, check ExistenceCheck) error {
	startedWaiting := ctx.clock().Now()
	for {
		exists, err := check(id)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		if ctx.clock().Since(startedWaiting) > MaxExistenceWaitTime {
			return fmt.Errorf("Timed out waiting for resource %s to exist", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
}

func (ctx *Context) VPCExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Vpcs) > 0, nil
}

func (ctx *Context) FirewallExists(id string) (bool, error) {
	out, err := ctx.NetworkFirewall().DescribeFirewall(&networkfirewall.DescribeFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == networkfirewall.ErrCodeResourceNotFoundException {
				return false, nil
			} else {
				return false, fmt.Errorf("Error describing network firewall: %s", err)
			}
		} else {
			return false, fmt.Errorf("Error describing network firewall: %s", err)
		}
	}
	return aws.StringValue(out.FirewallStatus.Status) == networkfirewall.FirewallStatusValueReady, nil
}

func (ctx *Context) defaultFirewallPolicyExists(id string) (bool, error) {
	arn, err := ctx.getDefaultFirewallPolicyARN()
	if err != nil {
		return false, fmt.Errorf("Error checking if default firewall policy exists: %s", err)
	}
	return arn != nil, nil
}

func (ctx *Context) RouteTableAssociationExists(assocID string) (bool, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("association.route-table-association-id"),
				Values: aws.StringSlice([]string{
					assocID,
				}),
			},
		},
	})
	if err != nil {
		return false, err
	}

	if len(out.RouteTables) > 0 {
		for _, rt := range out.RouteTables {
			for _, a := range rt.Associations {
				if aws.StringValue(a.RouteTableAssociationId) == assocID && aws.StringValue(a.AssociationState.State) == ec2.RouteTableAssociationStateCodeAssociated {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (ctx *Context) CreateVPC(info *ipcontrol.VPCInfo) error {
	createVPCOut, err := ctx.EC2().CreateVpc(&ec2.CreateVpcInput{
		CidrBlock:       &info.NewCIDRs[0],
		InstanceTenancy: &info.Tenancy,
	})
	if err != nil {
		return fmt.Errorf("Error creating VPC: %s", err)
	}
	ctx.destructors = append(ctx.destructors, func() error {
		_, err := ctx.EC2().DeleteVpc(&ec2.DeleteVpcInput{VpcId: createVPCOut.Vpc.VpcId})
		if err == nil {
			ctx.Logger.Log("Deleted VPC %s", *createVPCOut.Vpc.VpcId)
		}
		return err
	})
	info.ResourceID = *createVPCOut.Vpc.VpcId
	err = ctx.WaitForExistence(*createVPCOut.Vpc.VpcId, ctx.VPCExists)
	if err != nil {
		return err
	}
	ctx.Logger.Log("Created VPC %s", info.ResourceID)
	for idx := 1; idx < len(info.NewCIDRs); idx++ {
		assoc, err := ctx.EC2().AssociateVpcCidrBlock(&ec2.AssociateVpcCidrBlockInput{
			CidrBlock: &info.NewCIDRs[idx],
			VpcId:     &info.ResourceID,
		})
		if err != nil {
			return fmt.Errorf("Error associating additional CIDR: %s", err)
		}
		err = ctx.WaitForExistence(*assoc.CidrBlockAssociation.AssociationId, ctx.CIDRBlockAssociationExists)
		if err != nil {
			return err
		}
		ctx.Logger.Log("Associated additional CIDR %s", info.NewCIDRs[idx])
	}
	tags := []*ec2.Tag{
		{Key: aws.String("Name"), Value: aws.String(info.Name)},
		{Key: aws.String("stack"), Value: aws.String(info.Stack)},
		{Key: aws.String("Automated"), Value: aws.String("true")},
	}
	_, err = ctx.EC2().CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{createVPCOut.Vpc.VpcId},
		Tags:      tags,
	})
	if err != nil {
		return fmt.Errorf("Error setting VPC tags: %s", err)
	}

	return nil
}

func SplitSubnets(subnets []*ec2.Subnet) (map[database.SubnetType][]*ec2.Subnet, error) {
	split := map[database.SubnetType][]*ec2.Subnet{}
	for _, subnet := range subnets {
		found := false
		var t database.SubnetType
		for _, tag := range subnet.Tags {
			if *tag.Key == "use" {
				found = true
				tier := "\"" + aws.StringValue(tag.Value) + "\""
				err := t.UnmarshalJSON([]byte(tier))
				if err != nil || t == database.SubnetTypeTransitive || t == "" {
					return nil, fmt.Errorf("Invalid use tag %s for subnet %s: %s", tier, *subnet.SubnetId, err)
				}
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("No use tag (e.g. public/private) for subnet %s", *subnet.SubnetId)
		}
		split[t] = append(split[t], subnet)

	}
	return split, nil
}

func (ctx *Context) GetSubnets() (map[database.SubnetType][]*ec2.Subnet, error) {
	out, err := ctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(ctx.VPCID)},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return SplitSubnets(out.Subnets)
}

func splitLegacySubnets(subnets []*ec2.Subnet) (map[database.SubnetType][]*ec2.Subnet, error) {
	split := map[database.SubnetType][]*ec2.Subnet{}
	for _, subnet := range subnets {
		found := false
		layer := ""
		for _, tag := range subnet.Tags {
			key := aws.StringValue(tag.Key)
			if (key == "Layer" && layer == "") || key == "vpc-conf-layer" {
				found = true
				layer = aws.StringValue(tag.Value)
			}
		}
		if !found {
			return nil, fmt.Errorf("No Layer/vpc-conf-layer tag (app/data/etc) for subnet %s", *subnet.SubnetId)
		}
		t, err := matchSubnetType(layer)
		if err != nil {
			return nil, fmt.Errorf("Error determining layer for subnet %s: %s", *subnet.SubnetId, err)
		}
		split[t] = append(split[t], subnet)
	}
	return split, nil
}

func matchSubnetType(layer string) (database.SubnetType, error) {
	var t database.SubnetType
	switch layer {
	case "data":
		t = database.SubnetTypeData
	case "app":
		t = database.SubnetTypeApp
	case "management":
		t = database.SubnetTypeManagement
	case "web":
		t = database.SubnetTypeWeb
	case "dmz":
		t = database.SubnetTypePublic
	case "transitive":
		t = database.SubnetTypeTransitive
	case "transport":
		t = database.SubnetTypeTransport
	case "security":
		t = database.SubnetTypeSecurity
	default:
		return "", fmt.Errorf("Invalid Layer/vpc-conf-layer tag %s", layer)
	}
	return t, nil
}

func (ctx *Context) GetLegacySubnets() (map[database.SubnetType][]*ec2.Subnet, error) {
	out, err := ctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(ctx.VPCID)},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return splitLegacySubnets(out.Subnets)
}

func (ctx *Context) DetachAndDeleteInternetGateway(id string) error {
	err := ctx.DetachInternetGateway(id)
	if err != nil {
		return err
	}
	return ctx.DeleteInternetGateway(id)
}

func (ctx *Context) DeleteInternetGateway(id string) error {
	_, err := ctx.EC2().DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
		InternetGatewayId: &id,
	})
	if err == nil {
		ctx.Logger.Log("Deleted Internet Gateway %s", id)
	}
	return err
}

func (ctx *Context) DetachInternetGateway(id string) error {
	_, err := ctx.EC2().DetachInternetGateway(&ec2.DetachInternetGatewayInput{
		InternetGatewayId: &id,
		VpcId:             &ctx.VPCID,
	})
	if err == nil {
		ctx.Logger.Log("Detached Internet Gateway %s", id)
	}
	return err
}

func (ctx *Context) DisassociateRouteTable(id string) error {
	_, err := ctx.EC2().DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
		AssociationId: &id,
	})
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "InvalidAssociationID.NotFound" {
			ctx.Logger.Log("Association %s not found", id)
			return nil
		}
	}
	if err == nil {
		ctx.Logger.Log("Removed Route Table association %s", id)
	}
	return err
}

func (ctx *Context) DeleteRouteTable(id string) error {
	_, err := ctx.EC2().DeleteRouteTable(&ec2.DeleteRouteTableInput{
		RouteTableId: &id,
	})
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "InvalidRouteTableID.NotFound" {
			ctx.Logger.Log("Route Table %s not found", id)
			return nil
		}
	}
	if err == nil {
		ctx.Logger.Log("Deleted Route Table %s", id)
	}
	return err
}

func (ctx *Context) InternetGatewayExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("internet-gateway-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.InternetGateways) > 0, nil
}

func (ctx *Context) CreateInternetGateway(name string) (string, error) {
	createGWOut, err := ctx.EC2().CreateInternetGateway(&ec2.CreateInternetGatewayInput{})
	if err != nil {
		return "", err
	}
	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DeleteInternetGateway(*createGWOut.InternetGateway.InternetGatewayId)
	})
	return *createGWOut.InternetGateway.InternetGatewayId, nil
}

func (ctx *Context) AttachInternetGateway(internetGatewayID string) error {
	_, err := ctx.EC2().AttachInternetGateway(&ec2.AttachInternetGatewayInput{
		InternetGatewayId: &internetGatewayID,
		VpcId:             &ctx.VPCID,
	})
	if err != nil {
		return err
	}
	ctx.Logger.Log("Attached Internet Gateway %s", internetGatewayID)
	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DetachInternetGateway(internetGatewayID)
	})

	return nil
}

func (ctx *Context) RouteTableExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("route-table-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.RouteTables) > 0, nil
}

func (ctx *Context) CreateRouteTable(name string) (*ec2.RouteTable, error) {
	createRTOut, err := ctx.EC2().CreateRouteTable(&ec2.CreateRouteTableInput{
		VpcId: &ctx.VPCID,
	})
	if err != nil {
		return nil, err
	}
	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DeleteRouteTable(*createRTOut.RouteTable.RouteTableId)
	})
	return createRTOut.RouteTable, nil
}

func (ctx *Context) GetAttachedInternetGateway() (*ec2.InternetGateway, error) {
	out, err := ctx.EC2().DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.InternetGateways) > 1 {
		return nil, fmt.Errorf("Found %d internet gateways attached to VPC", len(out.InternetGateways))
	}
	if len(out.InternetGateways) == 0 {
		return nil, nil
	}
	return out.InternetGateways[0], nil
}

func (ctx *Context) GetInternetGatewayByName(name string) (*ec2.InternetGateway, error) {
	out, err := ctx.EC2().DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{&name},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.InternetGateways) > 1 {
		return nil, fmt.Errorf("Found %d internet gateways with name %s in VPC", len(out.InternetGateways), name)
	}
	if len(out.InternetGateways) == 0 {
		return nil, nil
	}
	return out.InternetGateways[0], nil
}

func (ctx *Context) GetRouteTableAssociatedWithSubnet(subnetID string) (*ec2.RouteTable, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []*string{&subnetID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.RouteTables) > 1 {
		return nil, fmt.Errorf("Found %d route tables associated with %s", len(out.RouteTables), subnetID)
	}
	if len(out.RouteTables) == 0 {
		return nil, nil
	}
	return out.RouteTables[0], nil
}

func (ctx *Context) GetRouteTableAssociatedWithIGW(igwID string) (*ec2.RouteTable, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, rt := range out.RouteTables {
		for _, a := range rt.Associations {
			if aws.StringValue(a.GatewayId) == igwID {
				return rt, nil
			}
		}
	}
	return nil, nil
}

func (ctx *Context) GetRouteTableByName(name string) (*ec2.RouteTable, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{&name},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.RouteTables) > 1 {
		return nil, fmt.Errorf("Found %d route tables named %s in VPC", len(out.RouteTables), name)
	}
	if len(out.RouteTables) == 0 {
		return nil, nil
	}
	return out.RouteTables[0], nil
}

func (ctx *Context) LocalRouteWithDestinationExistsOnRouteTable(destination, rtID string) (bool, error) {
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		RouteTableIds: aws.StringSlice([]string{rtID}),
	})
	if err != nil {
		return false, err
	}

	if len(out.RouteTables) != 1 {
		return false, fmt.Errorf("Expected 1 route table for ID %s but got %d", rtID, len(out.RouteTables))
	}

	found := false
	for _, route := range out.RouteTables[0].Routes {
		for _, dest := range []*string{route.DestinationCidrBlock, route.DestinationPrefixListId} {
			if dest != nil && aws.StringValue(dest) == destination && aws.StringValue(route.GatewayId) == "local" {
				found = true
			}
		}
	}

	return found, nil
}

func (ctx *Context) GetNATGatewayInSubnet(publicSubnetID string) (*ec2.NatGateway, error) {
	out, err := ctx.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
		Filter: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
			{
				Name:   aws.String("subnet-id"),
				Values: []*string{&publicSubnetID},
			},
			{
				Name:   aws.String("state"),
				Values: []*string{aws.String("pending"), aws.String("available")},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.NatGateways) > 1 {
		return nil, fmt.Errorf("Found %d NAT gateways in subnet %s", len(out.NatGateways), publicSubnetID)
	}
	if len(out.NatGateways) == 0 {
		return nil, nil
	}
	return out.NatGateways[0], nil
}

func (ctx *Context) GetEIPByAllocationID(allocationID string) (*ec2.Address, error) {
	out, err := ctx.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("allocation-id"),
				Values: []*string{&allocationID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Addresses) > 1 {
		return nil, fmt.Errorf("Found %d EIPs with allocation ID %s", len(out.Addresses), allocationID)
	}
	if len(out.Addresses) == 0 {
		return nil, nil
	}
	return out.Addresses[0], nil
}

func (ctx *Context) GetEIPByName(name string) (*ec2.Address, error) {
	out, err := ctx.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []*string{&name},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Addresses) > 1 {
		return nil, fmt.Errorf("Found %d EIPs named %s", len(out.Addresses), name)
	}
	if len(out.Addresses) == 0 {
		return nil, nil
	}
	return out.Addresses[0], nil
}

// Ensure that an association exists between the provided route table and resource, and return the association ID and true if an association was created/replaced
// If an incorrect association exists, replace it with a correct one.
// If no association exists, create one.
func (ctx *Context) EnsureRouteTableAssociationExists(desiredRouteTableID, resourceID string) (string, error) {
	var subnetID *string
	var igwID *string
	var associatedRouteTable *ec2.RouteTable

	// get the RT associated with the resource ID
	// if the resource ID isn't for a subnet or IGW, error
	if isSubnetID(resourceID) {
		rt, err := ctx.GetRouteTableAssociatedWithSubnet(resourceID)
		if err != nil {
			return "", fmt.Errorf("Error getting route table for subnet %s: %s", resourceID, err)
		}
		subnetID = &resourceID
		associatedRouteTable = rt
	} else if IsInternetGatewayID(resourceID) {
		rt, err := ctx.GetRouteTableAssociatedWithIGW(resourceID)
		if err != nil {
			return "", fmt.Errorf("Error getting route table for IGW %s: %s", resourceID, err)
		}
		igwID = &resourceID
		associatedRouteTable = rt
	} else {
		return "", fmt.Errorf("Unknown resource ID %s", resourceID)
	}

	var newAssociationID string

	if associatedRouteTable != nil {
		// Either update an incorrect association or no-op for a correct one

		var currentAssociationID string
		for _, assoc := range associatedRouteTable.Associations {
			if (subnetID != nil && aws.StringValue(assoc.SubnetId) == *subnetID) || (igwID != nil && aws.StringValue(assoc.GatewayId) == *igwID) {
				if currentAssociationID != "" {
					// shouldn't be possible since a given resource can have at most one active association
					return "", fmt.Errorf("Multiple route table associations found for %s", resourceID)
				}
				currentAssociationID = aws.StringValue(assoc.RouteTableAssociationId)
			}
		}
		if currentAssociationID == "" {
			// shouldn't be possible since all associations have an association ID
			return "", fmt.Errorf("Route table %s is associated with resource %s, but no association ID was found", desiredRouteTableID, resourceID)
		}

		if aws.StringValue(associatedRouteTable.RouteTableId) == desiredRouteTableID {
			// the association between resource and route table is as requested (although the association ID could have changed if the association was destroyed and recreated manually)
			return currentAssociationID, nil
		} else {
			// replace the existing, incorrect association with a new association that targets the desired route table
			replaceAssnOut, err := ctx.EC2().ReplaceRouteTableAssociation(&ec2.ReplaceRouteTableAssociationInput{
				AssociationId: aws.String(currentAssociationID),
				RouteTableId:  aws.String(desiredRouteTableID),
			})
			if err != nil {
				return "", fmt.Errorf("Error updating route table association for resource %s: %s", resourceID, err)
			}
			ctx.Log("Updated route table association for resource %s from route table %s to route table %s", resourceID, aws.StringValue(associatedRouteTable.RouteTableId), desiredRouteTableID)
			newAssociationID = aws.StringValue(replaceAssnOut.NewAssociationId)
		}
	} else {
		// Create a new association

		createAssnOut, err := ctx.EC2().AssociateRouteTable(&ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(desiredRouteTableID),
			SubnetId:     subnetID,
			GatewayId:    igwID,
		})
		if err != nil {
			return "", fmt.Errorf("Error associating route table %s with resource %s: %s", desiredRouteTableID, resourceID, err)
		}
		ctx.Logger.Log("Created Route Table association %s (%s to %s)", aws.StringValue(createAssnOut.AssociationId), desiredRouteTableID, resourceID)
		newAssociationID = aws.StringValue(createAssnOut.AssociationId)
	}

	return newAssociationID, nil
}

func stringInSlice(s string, a []string) bool {
	for _, t := range a {
		if s == t {
			return true
		}
	}
	return false
}

func (ctx *Context) WaitForTransitGatewayStatus(id, waitingFor string, validWhileWaiting ...string) error {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.EC2().DescribeTransitGateways(&ec2.DescribeTransitGatewaysInput{TransitGatewayIds: []*string{aws.String(id)}})
		if err == nil {
			if len(out.TransitGateways) != 1 {
				return fmt.Errorf("Expected 1 Transit Gateway for id %s but found %d", id, len(out.TransitGateways))
			}
			state := *out.TransitGateways[0].State
			if state == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %s for Transit Gateway %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "InvalidTransitGatewayID.NotFound" {
				// Wait some more for it to be found
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for Transit Gateway %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
	return nil
}

func (ctx *Context) WaitForQueryLogConfigStatus(id, waitingFor string, validWhileWaiting ...string) error {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.R53R().GetResolverQueryLogConfig(&route53resolver.GetResolverQueryLogConfigInput{ResolverQueryLogConfigId: aws.String(id)})
		if err == nil {
			state := aws.StringValue(out.ResolverQueryLogConfig.Status)
			if state == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %s for Query Log config %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == route53resolver.ErrCodeResourceNotFoundException {
				if waitingFor == database.WaitForMissingAWSStatus {
					// It is waiting to be deleted, but doesn't exist - success!
					return nil
				}
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for Resolver Rule %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
	return nil
}

func (ctx *Context) WaitForQueryLogConfigAssociationStatus(id, waitingFor string, validWhileWaiting ...string) error {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.R53R().GetResolverQueryLogConfigAssociation(&route53resolver.GetResolverQueryLogConfigAssociationInput{ResolverQueryLogConfigAssociationId: aws.String(id)})
		if err == nil {
			state := aws.StringValue(out.ResolverQueryLogConfigAssociation.Status)
			if state == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %s for Query Log config %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == route53resolver.ErrCodeResourceNotFoundException {
				if waitingFor == database.WaitForMissingAWSStatus {
					// It is waiting to be deleted, but doesn't exist - success!
					break
				}
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for Resolver Rule %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
	return nil
}

func (ctx *Context) WaitForResolverRuleStatus(id, waitingFor string, validWhileWaiting ...string) error {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	status := "FAILED TO GET STATUS"
	for {
		out, err := ctx.R53R().GetResolverRule(&route53resolver.GetResolverRuleInput{ResolverRuleId: aws.String(id)})
		if err == nil {
			status = *out.ResolverRule.Status
			if status == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(status, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %q for Resolver Rule %q", status, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == route53resolver.ErrCodeResourceNotFoundException {
				// Wait some more for it to be found
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for Resolver Rule: %q Desired Status: %q Current Status: %q", id, waitingFor, status)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
	return nil
}

func (ctx *Context) WaitForResolverRuleVpcAttachmentStatus(id, waitingFor string, validWhileWaiting ...string) error {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.R53R().GetResolverRuleAssociation(
			&route53resolver.GetResolverRuleAssociationInput{
				ResolverRuleAssociationId: aws.String(id),
			},
		)
		if err == nil {
			state := *out.ResolverRuleAssociation.Status
			if state == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %s for Resolver Rule Association %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if waitingFor == database.WaitForMissingAWSStatus && aerr.Code() == route53resolver.ErrCodeResourceNotFoundException {
				// It is waiting to be deleted, but doesn't exist - success!
				return nil
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for Resolver Rule Association %s", id)
		}
		ctx.clock().Sleep(5 * time.Second)
	}
	return nil
}

func (ctx *Context) waitForNATGatewayStatus(id, waitingFor string, validWhileWaiting ...string) error {
	// Wait for NAT Gateway
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{NatGatewayIds: []*string{aws.String(id)}})
		if err == nil {
			if len(out.NatGateways) != 1 {
				return fmt.Errorf("Expected 1 NAT Gateway for id %s but found %d", id, len(out.NatGateways))
			}
			state := *out.NatGateways[0].State
			if state == waitingFor {
				break
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return fmt.Errorf("Unexpected state %s for NAT Gateway %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "NatGatewayNotFound" {
				// Wait some more for it to be found
			} else {
				return err
			}
		} else {
			return err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return fmt.Errorf("Timed out waiting for NAT Gateway %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
	return nil
}

func (ctx *Context) DeleteNATGateway(id string) error {
	_, err := ctx.EC2().DeleteNatGateway(&ec2.DeleteNatGatewayInput{
		NatGatewayId: &id,
	})
	if err != nil {
		return err
	}

	ctx.Logger.Log("Deleting NAT Gateway %s", id)
	err = ctx.waitForNATGatewayStatus(id, "deleted")
	if err != nil {
		return err
	}
	ctx.Logger.Log("Deleted NAT Gateway %s", id)
	return nil
}

func (ctx *Context) ReleaseEIP(id string) error {
	_, err := ctx.EC2().ReleaseAddress(&ec2.ReleaseAddressInput{
		AllocationId: &id,
	})
	if err == nil {
		ctx.Logger.Log("Released EIP %s", id)
	}
	return err
}

func (ctx *Context) EIPExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("allocation-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.Addresses) > 0, nil
}

func (ctx *Context) CreateEIP(name string) (string, error) {
	allocOut, err := ctx.EC2().AllocateAddress(&ec2.AllocateAddressInput{
		Domain: aws.String("vpc"),
	})
	if err != nil {
		return "", err
	}
	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.ReleaseEIP(*allocOut.AllocationId)
	})
	return *allocOut.AllocationId, nil
}

func (ctx *Context) CreateNATGateway(name string, eipID string, publicSubnetID string) (*ec2.NatGateway, error) {
	out, err := ctx.EC2().CreateNatGateway(&ec2.CreateNatGatewayInput{
		SubnetId:     &publicSubnetID,
		AllocationId: &eipID,
	})
	if err != nil {
		return nil, err
	}
	ctx.Logger.Log("Created NAT Gateway %s", *out.NatGateway.NatGatewayId)
	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DeleteNATGateway(*out.NatGateway.NatGatewayId)
	})
	ctx.Logger.Log("Waiting for NAT Gateway %s to become available", *out.NatGateway.NatGatewayId)
	err = ctx.waitForNATGatewayStatus(*out.NatGateway.NatGatewayId, "available", "pending")
	if err != nil {
		return nil, err
	}
	err = ctx.SetNameAndAutomated(*out.NatGateway.NatGatewayId, name)
	if err != nil {
		return nil, err
	}

	return out.NatGateway, nil
}

func (ctx *Context) GetTransitGatewayVPCAttachment(transitGatewayID string) (*ec2.TransitGatewayVpcAttachment, error) {
	out, err := ctx.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
			{
				Name:   aws.String("transit-gateway-id"),
				Values: []*string{&transitGatewayID},
			},
			{
				Name: aws.String("state"),
				Values: []*string{
					aws.String("pending"),
					aws.String("available"),
					aws.String("modifying"),
					aws.String("pendingAcceptance"),
					aws.String("rollingBack"),
					aws.String("rejected"),
					aws.String("rejecting"),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.TransitGatewayVpcAttachments) > 1 {
		return nil, fmt.Errorf("Found %d Transit Gateway VPC Attachments attached to %s in VPC", len(out.TransitGatewayVpcAttachments), transitGatewayID)
	}
	if len(out.TransitGatewayVpcAttachments) == 0 {
		return nil, nil
	}
	return out.TransitGatewayVpcAttachments[0], nil
}

func (ctx *Context) WaitForPeeringConnectionStatus(id string, waitingFor, validWhileWaiting []string) (string, error) {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.EC2().DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{VpcPeeringConnectionIds: []*string{aws.String(id)}})
		if err == nil {
			if len(out.VpcPeeringConnections) != 1 {
				return "", fmt.Errorf("Expected 1 peering connection for id %s but found %d", id, len(out.VpcPeeringConnections))
			}
			state := *out.VpcPeeringConnections[0].Status.Code
			if stringInSlice(state, waitingFor) {
				return state, nil
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return state, fmt.Errorf("Unexpected state %s for peering connection %s", state, id)
			}
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "InvalidVpcPeeringConnectionID.NotFound" {
				// Wait some more for it to be found
			} else {
				return "", err
			}
		} else {
			return "", err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return "", fmt.Errorf("Timed out waiting for peering connection %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
}

func (ctx *Context) WaitForTransitGatewayVpcAttachmentStatus(id string, waitingFor, validWhileWaiting []string) (string, error) {
	startedWaiting := ctx.clock().Now()
	maxWaitTime := time.Second * 300
	for {
		out, err := ctx.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{TransitGatewayAttachmentIds: []*string{aws.String(id)}})
		if err == nil {
			if len(out.TransitGatewayVpcAttachments) != 1 {
				return "", fmt.Errorf("Expected 1 Transit Gateway VPC Attachment for id %s but found %d", id, len(out.TransitGatewayVpcAttachments))
			}
			state := *out.TransitGatewayVpcAttachments[0].State
			if stringInSlice(state, waitingFor) {
				return state, nil
			} else if len(validWhileWaiting) == 0 || stringInSlice(state, validWhileWaiting) {
				// Wait for status change
			} else {
				return state, fmt.Errorf("Unexpected state %s for Transit Gateway VPC Attachment %s", state, id)
			}
		} else {
			return "", err
		}
		if ctx.clock().Since(startedWaiting) > maxWaitTime {
			return "", fmt.Errorf("Timed out waiting for Transit Gateway VPC Attachment %s", id)
		}
		ctx.clock().Sleep(1 * time.Second)
	}
}

func (ctx *Context) CreateTransitGatewayVPCAttachment(name, transitGatewayID string, subnetIDs []string) (string, error) {
	createTGWOut, err := ctx.EC2().CreateTransitGatewayVpcAttachment(&ec2.CreateTransitGatewayVpcAttachmentInput{
		SubnetIds:        aws.StringSlice(subnetIDs),
		TransitGatewayId: &transitGatewayID,
		VpcId:            &ctx.VPCID,
	})
	if err != nil {
		return "", err
	}

	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DeleteTransitGatewayVPCAttachment(*createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)
	})

	ctx.Logger.Log("Waiting for Transit Gateway VPC Attachment %s to become available", *createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)
	_, err = ctx.WaitForTransitGatewayVpcAttachmentStatus(*createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId, []string{"available", "pendingAcceptance"}, []string{"pending"})
	if err != nil {
		return "", err
	}

	err = ctx.SetNameAndAutomated(*createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId, name)
	if err != nil {
		return "", err
	}

	ctx.Logger.Log("Created Transit Gateway Attachment ID: %s", *createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)

	return *createTGWOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId, nil
}

func (ctx *Context) DeleteTransitGatewayVPCAttachment(id string) error {
	_, err := ctx.EC2().DeleteTransitGatewayVpcAttachment(&ec2.DeleteTransitGatewayVpcAttachmentInput{
		TransitGatewayAttachmentId: &id,
	})
	if err != nil {
		return err
	}
	ctx.Logger.Log("Deleting Transit Gateway Attachment %s", id)

	_, err = ctx.WaitForTransitGatewayVpcAttachmentStatus(id, []string{"deleted"}, []string{"deleting"})
	if err != nil {
		return err
	}
	ctx.Logger.Log("Deleted Transit Gateway Attachment %s", id)
	return err
}

func (ctx *Context) QueryLogConfigName() string {
	return ctx.VPCID + "-querylogs-to-cloudwatch"
}

func (ctx *Context) CreateResolverQueryLogConfig(destinationArn string) (string, error) {
	out, err := ctx.R53R().CreateResolverQueryLogConfig(&route53resolver.CreateResolverQueryLogConfigInput{
		CreatorRequestId: aws.String(ctx.QueryLogConfigName()),
		Name:             aws.String(ctx.QueryLogConfigName()),
		DestinationArn:   aws.String(destinationArn),
		Tags: []*route53resolver.Tag{
			{
				Key:   aws.String("Automated"),
				Value: aws.String("true"),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("Error creating CloudWatch Logs querylogs configuration: %s", err)
	}
	queryLogConfigId := aws.StringValue(out.ResolverQueryLogConfig.Id)
	err = ctx.WaitForQueryLogConfigStatus(queryLogConfigId, route53resolver.ResolverQueryLogConfigStatusCreated, route53resolver.ResolverQueryLogConfigStatusCreating)
	if err != nil {
		return "", err
	}
	return queryLogConfigId, nil
}

func (ctx *Context) DeleteResolverQueryLogConfig(queryLogConfigId string) error {
	_, err := ctx.R53R().DeleteResolverQueryLogConfig(&route53resolver.DeleteResolverQueryLogConfigInput{
		ResolverQueryLogConfigId: aws.String(queryLogConfigId),
	})
	if err != nil {
		return fmt.Errorf("Error deleting CloudWatch Logs querylogs configuration: %s", err)
	}
	err = ctx.WaitForQueryLogConfigStatus(queryLogConfigId, database.WaitForMissingAWSStatus, route53resolver.ResolverQueryLogConfigStatusDeleting)
	if err != nil {
		return err
	}
	return nil
}

func (ctx *Context) CheckLogGroupExists(logGroupName string) (*string, error) {
	logsOut, err := ctx.CloudWatchLogs().DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
	})
	if err != nil {
		return nil, fmt.Errorf("Error listing CloudWatch Logs groups: %s", err)
	}
	for _, logGroup := range logsOut.LogGroups {
		if aws.StringValue(logGroup.LogGroupName) == logGroupName {
			return logGroup.Arn, nil
		}
	}
	return nil, nil
}

func (ctx *Context) EnsureCloudWatchLogsGroupExists(logGroupName string) (string, error) {
	arn, err := ctx.CheckLogGroupExists(logGroupName)
	if err != nil {
		return "", err
	}
	if arn != nil {
		return aws.StringValue(arn), nil
	}
	// Make the log group if it doesn't exist
	_, err = ctx.CloudWatchLogs().CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(logGroupName),
		Tags: map[string]*string{
			"Automated": aws.String("true"),
		},
	})
	if err != nil {
		return "", fmt.Errorf("Error creating CloudWatch log group %s: %s", logGroupName, err)
	}
	ctx.Log("Created CloudWatch log group %s", logGroupName)

	// After we create the log group, grab the arn via CheckLogGroupExists(), this way we don't have to worry about constructing arns ourselves
	arn, err = ctx.CheckLogGroupExists(logGroupName)
	if err != nil {
		return "", err
	}
	if arn == nil {
		return "", fmt.Errorf("Nil ARN for log group after creation")
	}
	return aws.StringValue(arn), nil
}

func (ctx *Context) AssociateResolverQueryLogConfig(configId string) (string, error) {
	out, err := ctx.R53R().AssociateResolverQueryLogConfig(&route53resolver.AssociateResolverQueryLogConfigInput{
		ResolverQueryLogConfigId: aws.String(configId),
		ResourceId:               aws.String(ctx.VPCID),
	})
	if err != nil {
		return "", err
	}
	return aws.StringValue(out.ResolverQueryLogConfigAssociation.Id), nil
}

func (ctx *Context) CreateResolverRuleVPCAttachment(name, resolverRuleId string) (string, error) {
	createRRAOut, err := ctx.R53R().AssociateResolverRule(&route53resolver.AssociateResolverRuleInput{
		ResolverRuleId: aws.String(resolverRuleId),
		Name:           aws.String(name),
		VPCId:          &ctx.VPCID,
	})
	if err != nil {
		return "", fmt.Errorf("associating resolver rule: %w", err)
	}

	ctx.destructors = append(ctx.destructors, func() error {
		return ctx.DeleteResolverRuleVPCAssociation(resolverRuleId, *createRRAOut.ResolverRuleAssociation.Id)
	})

	ctx.Logger.Log("Waiting for Resolver Rule VPC Attachment %s to become available", *createRRAOut.ResolverRuleAssociation.Id)
	err = ctx.WaitForResolverRuleVpcAttachmentStatus(
		*createRRAOut.ResolverRuleAssociation.Id,
		route53resolver.ResolverRuleAssociationStatusComplete,
		route53resolver.ResolverRuleAssociationStatusCreating,
	)
	if err != nil {
		return "", fmt.Errorf("waiting for association: %w", err)
	}

	ctx.Logger.Log("Associated resolver rule: %s", *createRRAOut.ResolverRuleAssociation.ResolverRuleId)
	return *createRRAOut.ResolverRuleAssociation.Id, nil
}

func (ctx *Context) DeleteResolverRuleVPCAssociation(resolverRuleId, associationId string) error {
	ctx.Logger.Log("Detaching resolver rule %s", resolverRuleId)
	_, err := ctx.R53R().DisassociateResolverRule(&route53resolver.DisassociateResolverRuleInput{
		ResolverRuleId: &resolverRuleId,
		VPCId:          &ctx.VPCID,
	})
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	ctx.Logger.Log("Waiting for resolver rule VPC attachment %s to be deleted", resolverRuleId)
	err = ctx.WaitForResolverRuleVpcAttachmentStatus(
		associationId,
		database.WaitForMissingAWSStatus,
		route53resolver.ResolverRuleAssociationStatusDeleting,
	)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	ctx.Logger.Log("Resolver rule %s detached", resolverRuleId)
	return nil
}

func (ctx *Context) DeleteSecurityGroup(id string) error {
	_, err := ctx.EC2().DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupId: &id,
	})
	if err != nil {
		return err
	}
	ctx.Logger.Log("Deleted security group %s", id)
	return nil
}

func (ctx *Context) SecurityGroupExists(id string) (bool, error) {
	out, err := ctx.EC2().DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("group-id"),
				Values: []*string{aws.String(id)},
			},
		},
	})
	if err != nil {
		return false, err
	}
	return len(out.SecurityGroups) > 0, nil
}

// This requires a LockSet with TargetAddResourceShare.
// It is special because RAM shares created are not recorded in any state.
func (ctx *Context) CreateRAMShare(lockSet database.LockSet, sourceRAM ramiface.RAMAPI, name string, resourceArns []string) (string, error) {
	if !lockSet.HasLock(database.TargetAddResourceShare) {
		return "", fmt.Errorf("We don't have the AddResourceShare lock. If you see this error in production please alert a developer!")
	}

	shareInfo, err := sourceRAM.CreateResourceShare(
		&ram.CreateResourceShareInput{
			AllowExternalPrincipals: aws.Bool(true),
			Name:                    aws.String(name),
			ResourceArns:            aws.StringSlice(resourceArns),
		},
	)

	if err != nil {
		return "", err
	}

	arn := aws.StringValue(shareInfo.ResourceShare.ResourceShareArn)
	ss := strings.LastIndexByte(arn, '/') + 1
	if ss == 0 {
		return "", fmt.Errorf("Invalid Share ARN: %s", arn)
	}
	return arn[ss:], nil
}

func principalExistsOnShare(sourceRAM ramiface.RAMAPI, accountID, shareARN string) (bool, error) {
	found := false
	// https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/making-requests.html#:~:text=Using%20Pagination%20Methods
	err := sourceRAM.ListPrincipalsPages(&ram.ListPrincipalsInput{
		ResourceShareArns: []*string{aws.String(shareARN)},
		ResourceOwner:     aws.String("SELF"),
	}, func(page *ram.ListPrincipalsOutput, lastPage bool) bool {
		for _, principal := range page.Principals {
			if *principal.Id == accountID {
				found = true
				return false // Found, no need to look at any more pages
			}
		}
		return true // Not yet found, get next page
	})
	if err != nil {
		return false, fmt.Errorf("Error listing principals for share: %s", err)
	}
	return found, nil
}

func (ctx *Context) CheckResourceShareResourcesInUse(sourceRAM ramiface.RAMAPI, shareARN string) (bool, error) {
	out, err := sourceRAM.ListResources(&ram.ListResourcesInput{
		ResourceShareArns: aws.StringSlice([]string{shareARN}),
		ResourceOwner:     aws.String("SELF"),
	})
	if err != nil {
		return false, fmt.Errorf("Error listing resources on RAM share: %w", err)
	}
	for _, resource := range out.Resources {
		switch aws.StringValue(resource.Type) {
		case ResourceTypeResolverRule:
			resourceID := aws.StringValue(resource.Arn)
			if arn.IsARN(resourceID) {
				a, err := arn.Parse(resourceID)
				if err != nil {
					return false, fmt.Errorf("Unable to parse arn: %w", err)
				}
				resourceID = a.Resource
			}
			if i := strings.LastIndex(resourceID, "/"); i > -1 {
				resourceID = resourceID[i+1:]
			}
			out, err := ctx.R53Rsvc.ListResolverRuleAssociations(&route53resolver.ListResolverRuleAssociationsInput{
				Filters: []*route53resolver.Filter{
					{
						Name:   aws.String("ResolverRuleId"),
						Values: aws.StringSlice([]string{resourceID}),
					},
				},
			})
			if err != nil {
				return false, fmt.Errorf("Error checking resolver rule associations: %w", err)
			}
			for _, association := range out.ResolverRuleAssociations {
				if strings.HasSuffix(aws.StringValue(resource.Arn), aws.StringValue(association.ResolverRuleId)) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (ctx *Context) RemovePrincipalFromShare(sourceRAM ramiface.RAMAPI, accountID, shareARN string) error {
	found, err := principalExistsOnShare(sourceRAM, accountID, shareARN)
	if err != nil {
		return fmt.Errorf("Error checking for principal to remove: %s", err)
	}
	if found {
		ctx.Log("Removing account %s from principal list on share %s", accountID, shareARN)
		_, err = sourceRAM.DisassociateResourceShare(&ram.DisassociateResourceShareInput{
			ResourceShareArn: aws.String(shareARN),
			Principals:       []*string{&accountID},
		})
		if err != nil {
			return fmt.Errorf("Error removing principal: %s", err)
		} else {
			ctx.Log("Account %s removed from principal list", accountID)
		}
	}
	return nil
}

func (ctx *Context) EnsurePrincipalOnShare(sourceRAM ramiface.RAMAPI, accountID, shareARN string) error {
	found, err := principalExistsOnShare(sourceRAM, accountID, shareARN)
	if err != nil {
		return fmt.Errorf("Error checking for principal: %s", err)
	}
	if found {
		return nil
	}
	ctx.Log("Adding account %s to principal list", accountID)
	_, err = sourceRAM.AssociateResourceShare(&ram.AssociateResourceShareInput{
		ResourceShareArn: aws.String(shareARN),
		Principals:       []*string{&accountID},
	})
	if err != nil {
		return fmt.Errorf("Error associating principal: %s", err)
	}
	// Sometimes it takes some time for a pending invitation to appear.
	ctx.Log("Waiting for invitation to accept")
	accepted := false
	startedWaiting := time.Now()
	maxWaitTime := time.Minute * 5
	// If the target account is in the cmscloud AWS organization, then we won't get an invitation
	// Instead, the resource share will just show up in the newly added principal
	// Leave the possibility of an invitation as a fallback method to join the share, though
	for {
		ramOut, err := ctx.RAM().GetResourceShares(&ram.GetResourceSharesInput{
			ResourceOwner:     aws.String(ram.ResourceOwnerOtherAccounts),
			ResourceShareArns: []*string{&shareARN},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() != ram.ErrCodeUnknownResourceException {
					return fmt.Errorf("Error fetching shares: %s", err)
				}
			} else {
				return fmt.Errorf("Error fetching shares: %s", err)
			}
		}
		if ramOut != nil {
			for _, resource := range ramOut.ResourceShares {
				if *resource.Status == ram.ResourceShareStatusActive {
					ctx.Log("Resource share found locally, skipping invite acceptance")
					accepted = true
					break
				}
			}
		}
		if accepted {
			break
		}
		inviteOut, err := ctx.RAM().GetResourceShareInvitations(&ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []*string{&shareARN},
		})
		if err != nil {
			return fmt.Errorf("Error fetching invitations: %s", err)
		}
		for _, invite := range inviteOut.ResourceShareInvitations {
			if *invite.Status != "PENDING" {
				continue
			}
			ctx.Log("Accepting invitation %s", *invite.ResourceShareInvitationArn)
			_, err = ctx.RAM().AcceptResourceShareInvitation(&ram.AcceptResourceShareInvitationInput{
				ResourceShareInvitationArn: invite.ResourceShareInvitationArn,
			})
			if err != nil {
				return fmt.Errorf("Error accepting share: %s", err)
			}
			accepted = true
		}
		if accepted || time.Since(startedWaiting) > maxWaitTime {
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	if !accepted {
		return fmt.Errorf("No pending invitations to accept!")
	}

	// Wait for account to be added to principal list before continuing
	ctx.Log("Waiting for account %s to be added to principal list", accountID)
	startedWaiting = time.Now()
	maxWaitTime = time.Second * 60
	for {
		added, err := principalExistsOnShare(sourceRAM, accountID, shareARN)
		if err != nil {
			return fmt.Errorf("Error checking for principal: %s", err)
		}
		if added {
			ctx.Log("Account %s was added to principal list %s", accountID, shareARN)
			return nil
		}
		if time.Since(startedWaiting) > maxWaitTime {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("Timed out waiting for account %s to be added to principal list %s", accountID, shareARN)
}

// returns the ARN or nil if the policy doesn't exist
func (ctx *Context) getDefaultFirewallPolicyARN() (*string, error) {
	out, err := ctx.NetworkFirewall().ListFirewallPolicies(&networkfirewall.ListFirewallPoliciesInput{})
	if err != nil {
		return nil, fmt.Errorf("Error listing firewall policies: %s", err)
	}
	for _, fp := range out.FirewallPolicies {
		if aws.StringValue(fp.Name) == ctx.defaultFirewallPolicyName() {
			return fp.Arn, nil
		}
	}
	return nil, nil
}

// ARNs for the rule groups managed by
type managedRuleGroupInfo struct {
	statefulARN, statelessARN string
}

// return is non-nil only if both rule group ARNs are found without error
func (ctx *Context) getManagedRuleGroupInfo() (*managedRuleGroupInfo, error) {
	info := &managedRuleGroupInfo{}
	statefulRuleFound := false
	statelessRuleFound := false

	err := ctx.NetworkFirewall().ListRuleGroupsPages(&networkfirewall.ListRuleGroupsInput{},
		func(page *networkfirewall.ListRuleGroupsOutput, lastPage bool) bool {
			for _, rg := range page.RuleGroups {
				if aws.StringValue(rg.Name) == ManagedStatefulRuleGroupName {
					statefulRuleFound = true
					info.statefulARN = aws.StringValue(rg.Arn)
				} else if aws.StringValue(rg.Name) == ManagedStatelessRuleGroupName {
					statelessRuleFound = true
					info.statelessARN = aws.StringValue(rg.Arn)
				}
			}
			// https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/making-requests.html#:~:text=Using%20Pagination%20Methods
			// the callback keeps paginating until it returns false or reaches the last page, so we keep going if we haven't found both ARNs
			return !(statefulRuleFound && statelessRuleFound)
		})
	if err != nil {
		return nil, fmt.Errorf("Error listing firewall rule groups: %s", err)
	}

	if !statefulRuleFound || !statelessRuleFound {
		return nil, nil
	}
	return info, nil
}

func (ctx *Context) waitForManagedRuleGroupInfo() (*managedRuleGroupInfo, error) {
	w := &waiter.Waiter{
		SleepDuration:  time.Second * 5,
		StatusInterval: time.Second * 60,
		Timeout:        time.Minute * 30,
		Logger:         ctx.Logger,
	}

	var err error
	var result *managedRuleGroupInfo

	err = w.Wait(func() waiter.Result {
		result, err = ctx.getManagedRuleGroupInfo()
		if err != nil {
			return waiter.Error(fmt.Errorf("Error checking if managed firewall rule groups are ready: %s", err))
		}
		if result != nil {
			return waiter.DoneWithMessage("Managed firewall rule groups are ready")
		}
		return waiter.Continue("Waiting for managed firewall rule groups to be ready")
	})
	if err != nil {
		return nil, fmt.Errorf("Error waiting for managed firewall rule groups: %s. Please check with SecOps to make sure the rule groups are set up for this account.", err)
	}

	return result, nil
}

func (ctx *Context) EnsureDefaultFirewallPolicyExists(region database.Region) error {
	arn, err := ctx.getDefaultFirewallPolicyARN()
	if err != nil {
		return fmt.Errorf("Error checking if default firewall policy exists: %s", err)
	}

	if arn == nil {
		ctx.Log("Looking for SecOps-managed firewall rule groups")
		info, err := ctx.waitForManagedRuleGroupInfo()
		if err != nil {
			return fmt.Errorf("Error getting managed firewall rule group info: %s", err)
		}
		_, err = ctx.NetworkFirewall().CreateFirewallPolicy(&networkfirewall.CreateFirewallPolicyInput{
			FirewallPolicyName: aws.String(ctx.defaultFirewallPolicyName()),
			FirewallPolicy: &networkfirewall.FirewallPolicy{
				StatefulRuleGroupReferences: []*networkfirewall.StatefulRuleGroupReference{
					{
						ResourceArn: aws.String(info.statefulARN),
					},
				},
				StatelessRuleGroupReferences: []*networkfirewall.StatelessRuleGroupReference{
					{
						ResourceArn: aws.String(info.statelessARN),
						Priority:    aws.Int64(statelessRulePriority),
					},
				},
				StatelessDefaultActions: []*string{
					aws.String(forwardToSFEAction),
				},
				StatelessFragmentDefaultActions: []*string{
					aws.String(forwardToSFEAction),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("Error creating default firewall policy: %s", err)
		}
		ctx.Log("Creating default firewall policy")

		// it's safe to wait here since the policy isn't stored in state
		// we pass an empty id since the policy name is inferred from the VPC name by the existence check
		err = ctx.WaitForExistence("", ctx.defaultFirewallPolicyExists)
		if err != nil {
			return fmt.Errorf("Error waiting for default firewall policy to exist: %s", err)
		}
	}
	return nil
}

func (ctx *Context) CreateFirewall(subnetIDs []string) (*networkfirewall.Firewall, error) {
	firewallPolicyARN, err := ctx.getDefaultFirewallPolicyARN()
	if err != nil {
		return nil, fmt.Errorf("Error getting default firewall policy ARN: %s", err)
	}
	if firewallPolicyARN == nil {
		return nil, fmt.Errorf("Can't create the firewall because the default firewall policy doesn't exist")
	}

	out, err := ctx.NetworkFirewall().CreateFirewall(&networkfirewall.CreateFirewallInput{
		DeleteProtection:               aws.Bool(false),
		FirewallName:                   aws.String(ctx.FirewallName()),
		FirewallPolicyArn:              firewallPolicyARN,
		FirewallPolicyChangeProtection: aws.Bool(false),
		SubnetChangeProtection:         aws.Bool(false),
		SubnetMappings:                 GenerateSubnetMappings(subnetIDs),
		Tags: []*networkfirewall.Tag{
			{
				Key:   aws.String(firewallKey),
				Value: aws.String(firewallValue),
			},
		},
		VpcId: aws.String(ctx.VPCID),
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating network firewall: %s", err)
	}
	ctx.Log("Creating network firewall %s", ctx.FirewallName())

	return out.Firewall, nil
}

func (ctx *Context) GetSubnetAssociationsForFirewall() ([]string, error) {
	out, err := ctx.NetworkFirewall().DescribeFirewall(&networkfirewall.DescribeFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, s := range out.Firewall.SubnetMappings {
		ids = append(ids, aws.StringValue(s.SubnetId))
	}
	return ids, nil
}

func (ctx *Context) FirewallSubnetAssociationsAreReady(subnetIDs []string) (bool, error) {
	out, err := ctx.NetworkFirewall().DescribeFirewall(&networkfirewall.DescribeFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		return false, err
	}
	if out.FirewallStatus != nil {
		for _, id := range subnetIDs {
			found := false
			var syncState *networkfirewall.SyncState

			for _, s := range out.FirewallStatus.SyncStates {
				if aws.StringValue(s.Attachment.SubnetId) == id {
					found = true
					syncState = s
				}
			}
			if !found || aws.StringValue(syncState.Attachment.Status) != networkfirewall.AttachmentStatusReady {
				return false, nil
			}
		}
	}
	return true, nil
}

func (ctx *Context) FirewallSubnetDisassociationsAreDone(subnetIDs []string) (bool, error) {
	out, err := ctx.NetworkFirewall().DescribeFirewall(&networkfirewall.DescribeFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		return false, err
	}
	stillExist := 0
	if out.FirewallStatus != nil {
		for _, id := range subnetIDs {
			for _, s := range out.FirewallStatus.SyncStates {
				if aws.StringValue(s.Attachment.SubnetId) == id {
					stillExist++
				}
			}
		}
	}
	if stillExist > 0 {
		return false, nil
	}
	return true, nil
}

func (ctx *Context) GetFirewallEndpointIDByAZ() (map[string]string, error) {
	fwOut, err := ctx.NetworkFirewall().DescribeFirewall(&networkfirewall.DescribeFirewallInput{
		FirewallName: aws.String(ctx.FirewallName()),
	})
	if err != nil {
		return nil, err
	}
	endpointIDByAZ := make(map[string]string)
	for azName, syncState := range fwOut.FirewallStatus.SyncStates {
		endpointIDByAZ[azName] = aws.StringValue(syncState.Attachment.EndpointId)
	}
	return endpointIDByAZ, nil
}

func (ctx *Context) GetPublicSubnetIDtoCIDR(azs map[string]*database.AvailabilityZoneInfra) (map[string]string, error) {
	publicSubnetIDtoCIDR := make(map[string]string)
	out, err := ctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(ctx.VPCID)},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, az := range azs {
		for _, publicSubnet := range az.Subnets[database.SubnetTypePublic] {
			for _, subnet := range out.Subnets {
				if aws.StringValue(subnet.SubnetId) == publicSubnet.SubnetID {
					publicSubnetIDtoCIDR[publicSubnet.SubnetID] = aws.StringValue(subnet.CidrBlock)
				}
			}
		}
	}
	return publicSubnetIDtoCIDR, nil
}

func (ctx *Context) WaitForFirewallSubnetAssociations(addIDs []string) error {
	w := &waiter.Waiter{
		SleepDuration:  time.Second * 5,
		StatusInterval: time.Second * 60,
		Timeout:        time.Minute * 10,
		Logger:         ctx.Logger,
	}

	err := w.Wait(func() waiter.Result {
		ready, err := ctx.FirewallSubnetAssociationsAreReady(addIDs)
		if err != nil {
			return waiter.Error(fmt.Errorf("Error checking if firewall subnet associations are ready: %s", err))
		}
		if ready {
			return waiter.DoneWithMessage("Firewall subnet associations are ready")
		}
		return waiter.Continue("Waiting for firewall subnet associations to be ready")
	})

	return err
}

func (ctx *Context) WaitForFirewallSubnetDisassociations(removedIDs []string) error {
	w := &waiter.Waiter{
		SleepDuration:  time.Second * 5,
		StatusInterval: time.Second * 60,
		Timeout:        time.Minute * 10,
		Logger:         ctx.Logger,
	}

	err := w.Wait(func() waiter.Result {
		done, err := ctx.FirewallSubnetDisassociationsAreDone(removedIDs)
		if err != nil {
			return waiter.Error(fmt.Errorf("Error checking if firewall subnet disassociations are done: %s", err))
		}
		if done {
			return waiter.DoneWithMessage("Firewall subnet disassociations are done")
		}
		return waiter.Continue("Waiting for firewall subnet disassociations to be done")
	})

	return err
}
