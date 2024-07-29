package testmocks

import (
	"fmt"
	"log"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

// All newly created internet gateways will have this ID
const NewIGWID = "igw-abc"

func TestAttachmentName(transitGatewayID string) string {
	return fmt.Sprintf("tgw-attach-%s", transitGatewayID)
}

func FormatTag(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

func (m *MockEC2) testPeeringConnectionID() string {
	m.pcxID++
	return fmt.Sprintf("pcx-%d", m.pcxID)
}

type MockEC2 struct {
	AccountID, Region string
	ec2iface.EC2API

	TransitGatewayStatus           map[string]string // id -> status
	TransitGatewayAttachmentStatus map[string]string // id -> status
	TransitGatewaysAutoAccept      bool

	AttachmentsCreated   []*ec2.TransitGatewayVpcAttachment
	AttachmentsModified  map[string][]string // id -> subnets
	AttachmentIDsDeleted []string

	RoutesAdded   map[string][]*database.RouteInfo
	RoutesDeleted map[string][]string

	PeeringConnectionsCreated   *[]*ec2.VpcPeeringConnection
	PeeringConnections          *[]*ec2.VpcPeeringConnection
	PeeringConnectionStatus     map[string]string // id -> status
	PeeringConnectionIDsDeleted []string

	SubnetCIDRs map[string]string

	PrimaryCIDR             *string
	CIDRBlockAssociationSet []*ec2.VpcCidrBlockAssociation

	SubnetsCreated          []*ec2.Subnet
	VPCCIDRBlocksAssociated map[string][]string // vpc id -> [blocks associated]

	SubnetsDeleted          []string
	RouteTablesDeleted      []string
	CIDRBlocksDisassociated []string

	RouteTables                    []*ec2.RouteTable
	RouteTablesCreated             []string
	RouteTableAssociationsCreated  []*ec2.RouteTableAssociation
	RouteTableAssociationsRemoved  []string
	RouteTableAssociationsReplaced map[string]string // old assoc id -> new route table id

	InternetGatewayWasCreated bool
	InternetGatewaysAttached  map[string]string // gateway id -> vpc id
	InternetGatewaysDetached  map[string]string // gateway id -> vpc id
	InternetGatewaysDeleted   []string

	EIPsAllocated      []string
	EIPsReleased       []string
	NATGatewaysCreated []*ec2.NatGateway
	NATGatewaysDeleted []string

	TagsCreated map[string][]string // resource id -> [key=value]
	TagsDeleted map[string][]string // resource id -> [key=value]

	PreDefinedSubnetIDQueue                map[string][]string // AZ -> subnetID
	PreDefinedNatGatewayIDQueue            []string
	PreDefinedRouteTableIDQueue            []string
	PreDefinedRouteTableAssociationIDQueue []string
	PreDefinedEIPQueue                     []string

	pcxID, rtID, assocID, allocID, natID int
}

func (m *MockEC2) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	subnetIDs := input.SubnetIds
	if len(subnetIDs) == 0 && len(input.Filters) == 1 && *input.Filters[0].Name == "subnet-id" {
		subnetIDs = input.Filters[0].Values
	}
	if len(subnetIDs) > 1 {
		return nil, fmt.Errorf("specifying %d subnet IDs not supported", len(input.SubnetIds))
	}
	if len(subnetIDs) == 1 {
		id := aws.StringValue(subnetIDs[0])
		// SubnetsCreated are new subnets created by API calls
		for _, subnet := range m.SubnetsCreated {
			if *subnet.SubnetId == id {
				return &ec2.DescribeSubnetsOutput{
					Subnets: []*ec2.Subnet{subnet},
				}, nil
			}
		}
		// SubnetCIDRs are existing subnets
		if m.SubnetCIDRs != nil {
			if cidr, ok := m.SubnetCIDRs[id]; ok {
				return &ec2.DescribeSubnetsOutput{
					Subnets: []*ec2.Subnet{
						{
							CidrBlock:        &cidr,
							AvailabilityZone: aws.String("us-east-1a"),
						},
					},
				}, nil
			}
		}
		return nil, fmt.Errorf("Unknown subnetID %s", id)
	}
	// Return all subnets
	out := &ec2.DescribeSubnetsOutput{}
	for id, cidr := range m.SubnetCIDRs {
		out.Subnets = append(out.Subnets, &ec2.Subnet{
			SubnetId:  aws.String(id),
			CidrBlock: aws.String(cidr),
		})
	}
	return out, nil
}

func (m *MockEC2) DescribeTransitGateways(input *ec2.DescribeTransitGatewaysInput) (*ec2.DescribeTransitGatewaysOutput, error) {
	id := *input.TransitGatewayIds[0]
	oldStatus, ok := m.TransitGatewayStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown transit gateway %q", id)
	}
	m.TransitGatewayStatus[id] = "available"
	return &ec2.DescribeTransitGatewaysOutput{
		TransitGateways: []*ec2.TransitGateway{
			{
				State: &oldStatus,
			},
		},
	}, nil
}

func (m *MockEC2) DescribeTransitGatewayVpcAttachments(input *ec2.DescribeTransitGatewayVpcAttachmentsInput) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	id := *input.TransitGatewayAttachmentIds[0]
	oldStatus, ok := m.TransitGatewayAttachmentStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown transit gateway attachment %q", id)
	}
	if oldStatus == "deleting" || oldStatus == "deleted" {
		m.TransitGatewayAttachmentStatus[id] = "deleted"
	} else if oldStatus != "pendingAcceptance" {
		m.TransitGatewayAttachmentStatus[id] = "available"
	}
	return &ec2.DescribeTransitGatewayVpcAttachmentsOutput{
		TransitGatewayVpcAttachments: []*ec2.TransitGatewayVpcAttachment{
			{
				State: &oldStatus,
			},
		},
	}, nil
}

func (m *MockEC2) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{
		Vpcs: []*ec2.Vpc{
			{
				CidrBlock:               m.PrimaryCIDR,
				CidrBlockAssociationSet: m.CIDRBlockAssociationSet,
			},
		},
	}, nil
}

func (m *MockEC2) CreateTransitGatewayVpcAttachment(input *ec2.CreateTransitGatewayVpcAttachmentInput) (*ec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	tgid := *input.TransitGatewayId
	if m.TransitGatewayStatus[tgid] != "available" {
		return nil, fmt.Errorf("Transit Gateway %s is not available yet", tgid)
	}
	name := TestAttachmentName(tgid)
	if m.TransitGatewaysAutoAccept {
		m.TransitGatewayAttachmentStatus[name] = "pending"
	} else {
		m.TransitGatewayAttachmentStatus[name] = "pendingAcceptance"
	}
	m.AttachmentsCreated = append(m.AttachmentsCreated, &ec2.TransitGatewayVpcAttachment{
		TransitGatewayId: input.TransitGatewayId,
		SubnetIds:        input.SubnetIds,
		VpcId:            input.VpcId,
	})
	return &ec2.CreateTransitGatewayVpcAttachmentOutput{
		TransitGatewayVpcAttachment: &ec2.TransitGatewayVpcAttachment{
			TransitGatewayAttachmentId: &name,
		},
	}, nil
}

func (m *MockEC2) CreateVpcPeeringConnection(input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	id := m.testPeeringConnectionID()
	m.PeeringConnectionStatus[id] = "initiating-request"
	pc := &ec2.VpcPeeringConnection{
		VpcPeeringConnectionId: &id,
		AccepterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
			VpcId:   input.PeerVpcId,
			OwnerId: input.PeerOwnerId,
			Region:  input.PeerRegion,
		},
		RequesterVpcInfo: &ec2.VpcPeeringConnectionVpcInfo{
			VpcId:   input.VpcId,
			OwnerId: &m.AccountID,
			Region:  &m.Region,
		},
	}
	*m.PeeringConnectionsCreated = append(*m.PeeringConnectionsCreated, pc)
	*m.PeeringConnections = append(*m.PeeringConnections, pc)
	return &ec2.CreateVpcPeeringConnectionOutput{
		VpcPeeringConnection: &ec2.VpcPeeringConnection{
			VpcPeeringConnectionId: &id,
		},
	}, nil
}

func (m *MockEC2) DeleteVpcPeeringConnection(input *ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	id := *input.VpcPeeringConnectionId
	_, ok := m.PeeringConnectionStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown peering connection %q", id)
	}
	m.PeeringConnectionStatus[id] = "deleting"
	m.PeeringConnectionIDsDeleted = append(m.PeeringConnectionIDsDeleted, id)
	return nil, nil
}

func (m *MockEC2) AcceptVpcPeeringConnection(input *ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	pcxID := aws.StringValue(input.VpcPeeringConnectionId)
	for _, pc := range *m.PeeringConnections {
		if aws.StringValue(pc.VpcPeeringConnectionId) == pcxID {
			if aws.StringValue(pc.AccepterVpcInfo.OwnerId) != m.AccountID {
				return nil, fmt.Errorf("Owner ID for peering connection %s is %s but Accept called from account %s", pcxID,
					aws.StringValue(pc.AccepterVpcInfo.OwnerId),
					m.AccountID)
			}
			if aws.StringValue(pc.AccepterVpcInfo.Region) != m.Region {
				return nil, fmt.Errorf("Owner region for peering connection %s is %s but Accept called from region %s", pcxID,
					aws.StringValue(pc.AccepterVpcInfo.Region),
					m.Region)
			}
			if m.PeeringConnectionStatus[pcxID] != "pending-acceptance" {
				return nil, fmt.Errorf("Peering connection %s cannot be accepted because its status is %s", pcxID, m.PeeringConnectionStatus[pcxID])
			}
			m.PeeringConnectionStatus[pcxID] = "provisioning"
			return nil, nil
		}
	}
	return nil, fmt.Errorf("No peering connection for ID %s", pcxID)
}

func contains(list []*string, target string) bool {
	for _, item := range list {
		if *item == target {
			return true
		}
	}
	return false
}

func (m *MockEC2) DescribeVpcPeeringConnections(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	if len(input.VpcPeeringConnectionIds) == 1 {
		id := aws.StringValue(input.VpcPeeringConnectionIds[0])
		oldStatus := m.PeeringConnectionStatus[id]
		if oldStatus == "deleting" || oldStatus == "deleted" {
			m.PeeringConnectionStatus[id] = "deleted"
		} else if oldStatus == "initiating-request" {
			m.PeeringConnectionStatus[id] = "pending-acceptance"
		} else if oldStatus != "pending-acceptance" {
			m.PeeringConnectionStatus[id] = "active"
		}
		log.Printf("%s -> %s", oldStatus, m.PeeringConnectionStatus[id])
		return &ec2.DescribeVpcPeeringConnectionsOutput{
			VpcPeeringConnections: []*ec2.VpcPeeringConnection{
				{
					Status: &ec2.VpcPeeringConnectionStateReason{
						Code: &oldStatus,
					},
				},
			},
		}, nil
	}

	candidates := []*ec2.VpcPeeringConnection{}
	if m.PeeringConnections != nil {
		candidates = append(candidates, *m.PeeringConnections...)
	}
	if m.PeeringConnectionsCreated != nil {
		candidates = append(candidates, *m.PeeringConnectionsCreated...)
	}

	for _, filter := range input.Filters {
		switch *filter.Name {
		case "vpc-peering-connection-id":
			matches := []*ec2.VpcPeeringConnection{}
			for _, candidate := range candidates {
				if candidate.VpcPeeringConnectionId == nil {
					continue
				}
				if contains(filter.Values, *candidate.VpcPeeringConnectionId) {
					matches = append(matches, candidate)
				}
			}
			candidates = matches

		case "status-code":
			matches := []*ec2.VpcPeeringConnection{}
			for _, candidate := range candidates {
				if candidate.Status == nil {
					fmt.Printf("Warning: nil value in ec2 mock for peering connection status: %+v", candidate)
					continue
				}
				if contains(filter.Values, *candidate.Status.Code) {
					matches = append(matches, candidate)
				}
			}
			candidates = matches
		case "requester-vpc-info.vpc-id":
			matches := []*ec2.VpcPeeringConnection{}
			for _, candidate := range candidates {
				if candidate.RequesterVpcInfo == nil || candidate.RequesterVpcInfo.VpcId == nil {
					fmt.Printf("Warning: nil value in ec2 mock for peering connection accepter: %+v", candidate)
					continue
				}
				if contains(filter.Values, *candidate.RequesterVpcInfo.VpcId) {
					matches = append(matches, candidate)
				}
			}
			candidates = matches
		case "accepter-vpc-info.vpc-id":
			matches := []*ec2.VpcPeeringConnection{}
			for _, candidate := range candidates {
				if candidate.AccepterVpcInfo == nil || candidate.AccepterVpcInfo.VpcId == nil {
					fmt.Printf("Warning: nil value in ec2 mock for peering connection requester: %+v", candidate)
					continue
				}
				if contains(filter.Values, *candidate.AccepterVpcInfo.VpcId) {
					matches = append(matches, candidate)
				}
			}
			candidates = matches
		default:
			// Error wasn't printed by calling code, so adding an additional print to ease debugging should this happen
			fmt.Printf("ERROR! EC2Mock has not implemented DescribeVpcPeeringConnections filter %s\n", *filter.Name)
			return nil, fmt.Errorf("EC2Mock has not implemented DescribeVpcPeeringConnections filter %s", *filter.Name)
		}
	}

	out := &ec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: candidates}
	return out, nil
}

// idempotent for identical tags
func (m *MockEC2) CreateTags(input *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	if m.TagsCreated == nil {
		m.TagsCreated = make(map[string][]string)
	}
	resourceID := aws.StringValue(input.Resources[0])
	for _, tag := range input.Tags {
		key := aws.StringValue(tag.Key)
		value := aws.StringValue(tag.Value)
		formattedTag := FormatTag(key, value)

		found := false
		for _, tag := range m.TagsCreated[resourceID] {
			if tag == formattedTag {
				found = true
			}
		}
		if !found {
			m.TagsCreated[resourceID] = append(m.TagsCreated[resourceID], FormatTag(key, value))
		}
	}
	return nil, nil
}

func (m *MockEC2) AcceptTransitGatewayVpcAttachment(input *ec2.AcceptTransitGatewayVpcAttachmentInput) (*ec2.AcceptTransitGatewayVpcAttachmentOutput, error) {
	id := *input.TransitGatewayAttachmentId
	_, ok := m.TransitGatewayAttachmentStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown transit gateway attachment %q", id)
	}
	m.TransitGatewayAttachmentStatus[id] = "pending"
	return nil, nil
}

func (m *MockEC2) ModifyTransitGatewayVpcAttachment(input *ec2.ModifyTransitGatewayVpcAttachmentInput) (*ec2.ModifyTransitGatewayVpcAttachmentOutput, error) {
	id := *input.TransitGatewayAttachmentId
	_, ok := m.TransitGatewayAttachmentStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown transit gateway attachment %q", id)
	}
	if input.RemoveSubnetIds != nil {
		return nil, fmt.Errorf("Did not expect any subnets to be removed")
	}
	m.TransitGatewayAttachmentStatus[id] = "modifying"
	if m.AttachmentsModified == nil {
		m.AttachmentsModified = make(map[string][]string)
	}
	m.AttachmentsModified[id] = aws.StringValueSlice(input.AddSubnetIds)
	return nil, nil
}

func (m *MockEC2) DeleteTransitGatewayVpcAttachment(input *ec2.DeleteTransitGatewayVpcAttachmentInput) (*ec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	id := *input.TransitGatewayAttachmentId
	_, ok := m.TransitGatewayAttachmentStatus[id]
	if !ok {
		return nil, fmt.Errorf("Unknown transit gateway attachment %q", id)
	}
	m.TransitGatewayAttachmentStatus[id] = "deleting"
	m.AttachmentIDsDeleted = append(m.AttachmentIDsDeleted, id)
	return nil, nil
}

func (m *MockEC2) CreateRoute(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	if m.RoutesAdded == nil {
		m.RoutesAdded = make(map[string][]*database.RouteInfo)
	}

	var destination *string
	if aws.StringValue(input.DestinationCidrBlock) != "" {
		destination = input.DestinationCidrBlock
	} else {
		destination = input.DestinationPrefixListId
	}

	m.RoutesAdded[*input.RouteTableId] = append(m.RoutesAdded[*input.RouteTableId], &database.RouteInfo{
		Destination:         aws.StringValue(destination),
		NATGatewayID:        aws.StringValue(input.NatGatewayId),
		InternetGatewayID:   aws.StringValue(input.GatewayId),
		VPCEndpointID:       aws.StringValue(input.VpcEndpointId),
		TransitGatewayID:    aws.StringValue(input.TransitGatewayId),
		PeeringConnectionID: aws.StringValue(input.VpcPeeringConnectionId),
	})
	return nil, nil
}

func (m *MockEC2) DeleteRoute(input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	if m.RoutesDeleted == nil {
		m.RoutesDeleted = make(map[string][]string)
	}

	var destination *string
	if aws.StringValue(input.DestinationCidrBlock) != "" {
		destination = input.DestinationCidrBlock
	} else {
		destination = input.DestinationPrefixListId
	}

	m.RoutesDeleted[*input.RouteTableId] = append(m.RoutesDeleted[*input.RouteTableId], *destination)
	return nil, nil
}

func (m *MockEC2) AssociateVpcCidrBlock(input *ec2.AssociateVpcCidrBlockInput) (*ec2.AssociateVpcCidrBlockOutput, error) {
	if m.VPCCIDRBlocksAssociated == nil {
		m.VPCCIDRBlocksAssociated = make(map[string][]string)
	}

	m.VPCCIDRBlocksAssociated[*input.VpcId] = append(
		m.VPCCIDRBlocksAssociated[*input.VpcId],
		*input.CidrBlock)
	return &ec2.AssociateVpcCidrBlockOutput{
		CidrBlockAssociation: &ec2.VpcCidrBlockAssociation{
			AssociationId: aws.String(*input.VpcId + "_" + *input.CidrBlock),
		},
	}, nil
}

func (m *MockEC2) hasSubnet(subnetID string) bool {
	for _, subnet := range m.SubnetsCreated {
		if subnetID == *subnet.SubnetId {
			return true
		}
	}
	return false
}

func (m *MockEC2) CreateSubnet(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
	// Override with PreDefined IDs if they exist
	newID := ""
	if len(m.PreDefinedSubnetIDQueue[*input.AvailabilityZone]) > 0 {
		newID = m.PreDefinedSubnetIDQueue[*input.AvailabilityZone][0]
		m.PreDefinedSubnetIDQueue[*input.AvailabilityZone] = m.PreDefinedSubnetIDQueue[*input.AvailabilityZone][1:]
	} else {
		for n := 0; newID == "" || m.hasSubnet(newID); n++ {
			newID = fmt.Sprintf("subnet-%s-%d", *input.AvailabilityZone, n)
		}
	}

	subnet := &ec2.Subnet{
		AvailabilityZone: input.AvailabilityZone,
		CidrBlock:        input.CidrBlock,
		VpcId:            input.VpcId,
		SubnetId:         &newID,
	}
	for _, spec := range input.TagSpecifications {
		subnet.Tags = append(subnet.Tags, spec.Tags...)
	}
	m.SubnetsCreated = append(m.SubnetsCreated, subnet)
	return &ec2.CreateSubnetOutput{Subnet: subnet}, nil
}

func (m *MockEC2) DeleteSubnet(input *ec2.DeleteSubnetInput) (*ec2.DeleteSubnetOutput, error) {
	m.SubnetsDeleted = append(m.SubnetsDeleted, *input.SubnetId)
	return nil, nil
}

func (m *MockEC2) DeleteRouteTable(input *ec2.DeleteRouteTableInput) (*ec2.DeleteRouteTableOutput, error) {
	m.RouteTablesDeleted = append(m.RouteTablesDeleted, *input.RouteTableId)
	return nil, nil
}

func (m *MockEC2) DisassociateVpcCidrBlock(input *ec2.DisassociateVpcCidrBlockInput) (*ec2.DisassociateVpcCidrBlockOutput, error) {
	m.CIDRBlocksDisassociated = append(m.CIDRBlocksDisassociated, *input.AssociationId)
	return nil, nil
}

func (m *MockEC2) CreateRouteTable(*ec2.CreateRouteTableInput) (*ec2.CreateRouteTableOutput, error) {
	// Override with PreDefined IDs if they exist
	newID := ""
	if len(m.PreDefinedRouteTableIDQueue) > 0 {
		newID = m.PreDefinedRouteTableIDQueue[0]
		m.PreDefinedRouteTableIDQueue = m.PreDefinedRouteTableIDQueue[1:]
	} else {
		m.rtID++
		newID = fmt.Sprintf("rt-%d", m.rtID)
	}

	m.RouteTablesCreated = append(m.RouteTablesCreated, newID)
	rt := &ec2.RouteTable{
		RouteTableId: aws.String(newID),
	}

	m.RouteTables = append(m.RouteTables, rt)
	return &ec2.CreateRouteTableOutput{
		RouteTable: rt,
	}, nil
}

func (m *MockEC2) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	var filter func(*ec2.RouteTable) bool
	allowEmptyResults := true
	if len(input.Filters) == 1 && aws.StringValue(input.Filters[0].Name) == "route-table-id" && len(input.Filters[0].Values) == 1 {
		rtID := aws.StringValue(input.Filters[0].Values[0])
		filter = func(rt *ec2.RouteTable) bool {
			return aws.StringValue(rt.RouteTableId) == rtID
		}
	} else if len(input.Filters) == 1 && aws.StringValue(input.Filters[0].Name) == "association.subnet-id" && len(input.Filters[0].Values) == 1 {
		subnetID := aws.StringValue(input.Filters[0].Values[0])
		filter = func(rt *ec2.RouteTable) bool {
			for _, assoc := range rt.Associations {
				if subnetID == aws.StringValue(assoc.SubnetId) {
					return true
				}
			}
			return false
		}
	} else if len(input.Filters) == 1 && aws.StringValue(input.Filters[0].Name) == "association.route-table-association-id" && len(input.Filters[0].Values) == 1 {
		assocID := aws.StringValue(input.Filters[0].Values[0])
		filter = func(rt *ec2.RouteTable) bool {
			for _, assoc := range rt.Associations {
				if aws.StringValue(assoc.RouteTableAssociationId) == assocID {
					return true
				}
			}
			return false
		}
	} else if len(input.Filters) == 1 && aws.StringValue(input.Filters[0].Name) == "vpc-id" && len(input.Filters[0].Values) == 1 {
		filter = func(rt *ec2.RouteTable) bool {
			return true
		}
	} else if len(input.RouteTableIds) == 1 {
		rtID := aws.StringValue(input.RouteTableIds[0])
		filter = func(rt *ec2.RouteTable) bool {
			return aws.StringValue(rt.RouteTableId) == rtID
		}
		allowEmptyResults = false
	} else {
		return nil, fmt.Errorf("mock only supports filter on single value for route-table-id or association.subnet-id, association.route-table-association-id, or vpc-id")
	}
	for _, rt := range m.RouteTables {
		if filter(rt) {
			return &ec2.DescribeRouteTablesOutput{
				RouteTables: []*ec2.RouteTable{rt},
			}, nil
		}
	}
	if !allowEmptyResults {
		return nil, fmt.Errorf("No matching route tables")
	}
	return &ec2.DescribeRouteTablesOutput{
		RouteTables: nil,
	}, nil
}

func (m *MockEC2) AssociateRouteTable(input *ec2.AssociateRouteTableInput) (*ec2.AssociateRouteTableOutput, error) {
	// Override with PreDefined IDs if they exist
	newID := ""
	if len(m.PreDefinedRouteTableAssociationIDQueue) > 0 {
		newID = m.PreDefinedRouteTableAssociationIDQueue[0]
		m.PreDefinedRouteTableAssociationIDQueue = m.PreDefinedRouteTableAssociationIDQueue[1:]
	} else {
		m.assocID++
		newID = fmt.Sprintf("assoc-%d", m.assocID)
	}

	assoc := &ec2.RouteTableAssociation{
		RouteTableAssociationId: aws.String(newID),
		RouteTableId:            input.RouteTableId,
		SubnetId:                input.SubnetId,
		GatewayId:               input.GatewayId,
		AssociationState: &ec2.RouteTableAssociationState{
			State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
		},
	}

	m.RouteTableAssociationsCreated = append(m.RouteTableAssociationsCreated, assoc)
	for _, rt := range m.RouteTables {
		if *rt.RouteTableId == *input.RouteTableId {
			rt.Associations = append(rt.Associations, assoc)
		}
	}
	return &ec2.AssociateRouteTableOutput{
		AssociationId: aws.String(newID),
	}, nil
}

func (m *MockEC2) CreateInternetGateway(*ec2.CreateInternetGatewayInput) (*ec2.CreateInternetGatewayOutput, error) {
	if m.InternetGatewayWasCreated {
		return nil, fmt.Errorf("Cannot create two internet gateways")
	}
	m.InternetGatewayWasCreated = true
	return &ec2.CreateInternetGatewayOutput{
		InternetGateway: &ec2.InternetGateway{
			InternetGatewayId: aws.String(NewIGWID),
		},
	}, nil
}

func (m *MockEC2) DescribeInternetGateways(input *ec2.DescribeInternetGatewaysInput) (*ec2.DescribeInternetGatewaysOutput, error) {
	if len(input.Filters) != 1 || aws.StringValue(input.Filters[0].Name) != "internet-gateway-id" || len(input.Filters[0].Values) != 1 {
		return nil, fmt.Errorf("only filtering with a single value for internet-gateway-id is supported")
	}
	if !m.InternetGatewayWasCreated || *input.Filters[0].Values[0] != NewIGWID {
		return &ec2.DescribeInternetGatewaysOutput{}, nil
	}
	return &ec2.DescribeInternetGatewaysOutput{
		InternetGateways: []*ec2.InternetGateway{
			{
				InternetGatewayId: aws.String(NewIGWID),
			},
		},
	}, nil
}

func (m *MockEC2) AttachInternetGateway(input *ec2.AttachInternetGatewayInput) (*ec2.AttachInternetGatewayOutput, error) {
	if m.InternetGatewaysAttached == nil {
		m.InternetGatewaysAttached = make(map[string]string)
	}

	m.InternetGatewaysAttached[*input.InternetGatewayId] = *input.VpcId
	return nil, nil
}

func (m *MockEC2) AllocateAddress(*ec2.AllocateAddressInput) (*ec2.AllocateAddressOutput, error) {
	// Override with PreDefined IDs if they exist
	newID := ""
	if len(m.PreDefinedEIPQueue) > 0 {
		newID = m.PreDefinedEIPQueue[0]
		m.PreDefinedEIPQueue = m.PreDefinedEIPQueue[1:]
	} else {
		m.allocID++
		newID = fmt.Sprintf("alloc-%d", m.allocID)
	}

	m.EIPsAllocated = append(m.EIPsAllocated, newID)
	return &ec2.AllocateAddressOutput{
		AllocationId: &newID,
	}, nil
}

func (m *MockEC2) DescribeAddresses(input *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
	if len(input.Filters) != 1 || aws.StringValue(input.Filters[0].Name) != "allocation-id" || len(input.Filters[0].Values) != 1 {
		return nil, fmt.Errorf("only filtering with a single value for allocation-id is supported")
	}
	allocID := *input.Filters[0].Values[0]
	for _, existing := range m.EIPsAllocated {
		if existing == allocID {
			return &ec2.DescribeAddressesOutput{
				Addresses: []*ec2.Address{
					{
						AllocationId: &allocID,
					},
				},
			}, nil
		}
	}
	return &ec2.DescribeAddressesOutput{}, nil
}

func (m *MockEC2) CreateNatGateway(input *ec2.CreateNatGatewayInput) (*ec2.CreateNatGatewayOutput, error) {
	// Override with PreDefined IDs if they exist
	newID := ""
	if len(m.PreDefinedNatGatewayIDQueue) > 0 {
		newID = m.PreDefinedNatGatewayIDQueue[0]
		m.PreDefinedNatGatewayIDQueue = m.PreDefinedNatGatewayIDQueue[1:]
	} else {
		m.natID++
		newID = fmt.Sprintf("nat-%d", m.natID)
	}

	ng := &ec2.NatGateway{
		NatGatewayId: &newID,
		SubnetId:     input.SubnetId,
		NatGatewayAddresses: []*ec2.NatGatewayAddress{
			{
				AllocationId: input.AllocationId,
			},
		},
	}

	m.NATGatewaysCreated = append(m.NATGatewaysCreated, ng)
	return &ec2.CreateNatGatewayOutput{
		NatGateway: ng,
	}, nil
}

func (m *MockEC2) DescribeNatGateways(input *ec2.DescribeNatGatewaysInput) (*ec2.DescribeNatGatewaysOutput, error) {
	if len(input.Filter) > 0 {
		matches := []*ec2.NatGateway{}
		for _, filter := range input.Filter {
			if *filter.Name == "nat-gateway-id" {
				for _, value := range filter.Values {
					for _, ng := range m.NATGatewaysCreated {
						if *ng.NatGatewayId == *value {
							matches = append(matches, &ec2.NatGateway{NatGatewayId: aws.String(*ng.NatGatewayId), State: aws.String("available")})
						}
					}
				}
			}
		}
		return &ec2.DescribeNatGatewaysOutput{
			NatGateways: matches,
		}, nil
	}

	if len(input.NatGatewayIds) != 1 {
		return nil, fmt.Errorf("Must specify 1 NatGatewayId for mock")
	}
	natID := *input.NatGatewayIds[0]
	for _, id := range m.NATGatewaysDeleted {
		if id == natID {
			return &ec2.DescribeNatGatewaysOutput{
				NatGateways: []*ec2.NatGateway{
					{
						State: aws.String("deleted"),
					},
				},
			}, nil
		}
	}
	for _, ng := range m.NATGatewaysCreated {
		if *ng.NatGatewayId == natID {
			return &ec2.DescribeNatGatewaysOutput{
				NatGateways: []*ec2.NatGateway{
					{
						State: aws.String("available"),
					},
				},
			}, nil
		}
	}
	return nil, awserr.New("NatGatewayNotFound", "", nil)
}

func (m *MockEC2) DeleteNatGateway(input *ec2.DeleteNatGatewayInput) (*ec2.DeleteNatGatewayOutput, error) {
	m.NATGatewaysDeleted = append(m.NATGatewaysDeleted, *input.NatGatewayId)
	return nil, nil
}

func (m *MockEC2) ReleaseAddress(input *ec2.ReleaseAddressInput) (*ec2.ReleaseAddressOutput, error) {
	m.EIPsReleased = append(m.EIPsReleased, *input.AllocationId)
	return nil, nil
}

func (m *MockEC2) DetachInternetGateway(input *ec2.DetachInternetGatewayInput) (*ec2.DetachInternetGatewayOutput, error) {
	if m.InternetGatewaysDetached == nil {
		m.InternetGatewaysDetached = make(map[string]string)
	}
	m.InternetGatewaysDetached[*input.InternetGatewayId] = *input.VpcId
	return nil, nil
}

func (m *MockEC2) DeleteInternetGateway(input *ec2.DeleteInternetGatewayInput) (*ec2.DeleteInternetGatewayOutput, error) {
	m.InternetGatewaysDeleted = append(m.InternetGatewaysDeleted, *input.InternetGatewayId)
	return nil, nil
}

func (m *MockEC2) DisassociateRouteTable(input *ec2.DisassociateRouteTableInput) (*ec2.DisassociateRouteTableOutput, error) {
	m.RouteTableAssociationsRemoved = append(m.RouteTableAssociationsRemoved, *input.AssociationId)
	return nil, nil
}

func (m *MockEC2) DeleteTags(input *ec2.DeleteTagsInput) (*ec2.DeleteTagsOutput, error) {
	if m.TagsDeleted == nil {
		m.TagsDeleted = make(map[string][]string)
	}
	for _, resourceID := range input.Resources {
		for _, tag := range input.Tags {
			m.TagsDeleted[*resourceID] = append(m.TagsDeleted[*resourceID], FormatTag(*tag.Key, *tag.Value))
		}
	}
	return nil, nil
}

func (m *MockEC2) ReplaceRoute(input *ec2.ReplaceRouteInput) (*ec2.ReplaceRouteOutput, error) {
	return nil, fmt.Errorf("Replace Route Not Implemented")
}

func (m *MockEC2) ReplaceRouteTableAssociation(input *ec2.ReplaceRouteTableAssociationInput) (*ec2.ReplaceRouteTableAssociationOutput, error) {
	m.assocID++
	newID := fmt.Sprintf("assoc-%d", m.assocID)
	if m.RouteTableAssociationsReplaced == nil {
		m.RouteTableAssociationsReplaced = make(map[string]string)
	}
	m.RouteTableAssociationsReplaced[*input.AssociationId] = *input.RouteTableId
	// find the old association
	for _, rt := range m.RouteTables {
		for _, assoc := range rt.Associations {
			if *assoc.RouteTableAssociationId == *input.AssociationId {
				assoc.RouteTableAssociationId = aws.String(newID)
				assoc.RouteTableId = input.RouteTableId
				assoc.AssociationState = &ec2.RouteTableAssociationState{
					State: aws.String(ec2.RouteTableAssociationStateCodeAssociated),
				}
				return &ec2.ReplaceRouteTableAssociationOutput{
					NewAssociationId: aws.String(newID),
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find association to update")
}
