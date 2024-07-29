package testmocks

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/networkfirewall/networkfirewalliface"
)

func (m *MockNetworkFirewall) vpcEndpointName(endpointID int) string {
	// Override with PreDefined IDs if they exist
	if len(m.PreDefinedVPCEQueue) > endpointID-1 && endpointID > 0 {
		return m.PreDefinedVPCEQueue[endpointID-1]
	}
	return fmt.Sprintf("vpce-%d", endpointID)
}

const TestARN = "test-arn"

type MockNetworkFirewall struct {
	networkfirewalliface.NetworkFirewallAPI

	Region    string
	AccountID string

	vpcEndpointID int
	SubnetIDToAZ  map[string]string // subnetID -> AZ

	FirewallPolicies             []*networkfirewall.FirewallPolicyMetadata
	FirewallPoliciesCreated      []string          // policy name
	Firewalls                    map[string]string // firewall name -> vpcID
	FirewallsCreated             map[string]string // firewall name -> vpcID
	RuleGroups                   []*networkfirewall.RuleGroupMetadata
	CreatedPolicyToRuleGroupARNs map[string][]string // policy name -> [configured rule group ARNs]

	SubnetAssociationsAdded    []string          // subnet id
	SubnetAssociationsRemoved  []string          // subnet id
	AssociatedSubnetToEndpoint map[string]string // subnetID -> endpointID

	PreDefinedVPCEQueue []string // VPCE ID
}

func (m *MockNetworkFirewall) ListFirewalls(input *networkfirewall.ListFirewallsInput) (*networkfirewall.ListFirewallsOutput, error) {
	firewalls := []*networkfirewall.FirewallMetadata{}
	for firewallName := range m.Firewalls {
		arn := fmt.Sprintf("arn:aws:network-firewall:%s:%s:firewall/%s", m.Region, m.AccountID, firewallName)
		firewalls = append(firewalls, &networkfirewall.FirewallMetadata{
			FirewallName: &firewallName,
			FirewallArn:  aws.String(arn),
		})
	}

	fwOutput := networkfirewall.ListFirewallsOutput{Firewalls: firewalls}
	return &fwOutput, nil
}

func (m *MockNetworkFirewall) ListFirewallPolicies(*networkfirewall.ListFirewallPoliciesInput) (*networkfirewall.ListFirewallPoliciesOutput, error) {
	output := &networkfirewall.ListFirewallPoliciesOutput{
		FirewallPolicies: m.FirewallPolicies,
	}
	return output, nil
}

func (m *MockNetworkFirewall) ListRuleGroupsPages(input *networkfirewall.ListRuleGroupsInput, fn func(*networkfirewall.ListRuleGroupsOutput, bool) bool) error {
	output := &networkfirewall.ListRuleGroupsOutput{
		RuleGroups: m.RuleGroups,
	}
	fn(output, true)
	return nil
}

func (m *MockNetworkFirewall) CreateFirewallPolicy(input *networkfirewall.CreateFirewallPolicyInput) (*networkfirewall.CreateFirewallPolicyOutput, error) {
	m.FirewallPoliciesCreated = append(m.FirewallPoliciesCreated, aws.StringValue(input.FirewallPolicyName))
	data := &networkfirewall.FirewallPolicyMetadata{
		Name: input.FirewallPolicyName,
		Arn:  aws.String(TestARN),
	}
	m.FirewallPolicies = append(m.FirewallPolicies, data)
	if m.CreatedPolicyToRuleGroupARNs == nil {
		m.CreatedPolicyToRuleGroupARNs = make(map[string][]string)
	}
	firewallName := aws.StringValue(input.FirewallPolicyName)
	for _, rg := range input.FirewallPolicy.StatefulRuleGroupReferences {
		m.CreatedPolicyToRuleGroupARNs[firewallName] = append(m.CreatedPolicyToRuleGroupARNs[firewallName], aws.StringValue(rg.ResourceArn))
	}
	for _, rg := range input.FirewallPolicy.StatelessRuleGroupReferences {
		m.CreatedPolicyToRuleGroupARNs[firewallName] = append(m.CreatedPolicyToRuleGroupARNs[firewallName], aws.StringValue(rg.ResourceArn))
	}
	return &networkfirewall.CreateFirewallPolicyOutput{}, nil
}

func (m *MockNetworkFirewall) AssociateSubnets(input *networkfirewall.AssociateSubnetsInput) (*networkfirewall.AssociateSubnetsOutput, error) {
	if m.AssociatedSubnetToEndpoint == nil {
		m.AssociatedSubnetToEndpoint = make(map[string]string)
	}
	for _, s := range input.SubnetMappings {
		m.vpcEndpointID++
		m.AssociatedSubnetToEndpoint[aws.StringValue(s.SubnetId)] = m.vpcEndpointName(m.vpcEndpointID)
		m.SubnetAssociationsAdded = append(m.SubnetAssociationsAdded, aws.StringValue(s.SubnetId))
	}
	return &networkfirewall.AssociateSubnetsOutput{}, nil
}

func (m *MockNetworkFirewall) DisassociateSubnets(input *networkfirewall.DisassociateSubnetsInput) (*networkfirewall.DisassociateSubnetsOutput, error) {
	for _, s := range aws.StringValueSlice(input.SubnetIds) {
		_, ok := m.AssociatedSubnetToEndpoint[s]
		if !ok {
			return nil, fmt.Errorf("Subnet ID %s not found", s)
		}
		delete(m.AssociatedSubnetToEndpoint, s)
		m.SubnetAssociationsRemoved = append(m.SubnetAssociationsRemoved, s)
	}
	return &networkfirewall.DisassociateSubnetsOutput{}, nil
}

func (m *MockNetworkFirewall) CreateFirewall(input *networkfirewall.CreateFirewallInput) (*networkfirewall.CreateFirewallOutput, error) {
	if m.FirewallsCreated == nil {
		m.FirewallsCreated = make(map[string]string)
	}
	m.FirewallsCreated[aws.StringValue(input.FirewallName)] = aws.StringValue(input.VpcId)

	if m.Firewalls == nil {
		m.Firewalls = make(map[string]string)
	}
	m.Firewalls[aws.StringValue(input.FirewallName)] = aws.StringValue(input.VpcId)

	if m.AssociatedSubnetToEndpoint == nil {
		m.AssociatedSubnetToEndpoint = make(map[string]string)
	}
	for _, s := range input.SubnetMappings {
		m.vpcEndpointID++
		m.AssociatedSubnetToEndpoint[aws.StringValue(s.SubnetId)] = m.vpcEndpointName(m.vpcEndpointID)
		m.SubnetAssociationsAdded = append(m.SubnetAssociationsAdded, aws.StringValue(s.SubnetId))
	}

	output := &networkfirewall.CreateFirewallOutput{
		Firewall: &networkfirewall.Firewall{},
	}
	return output, nil
}

// supports filtering on firewall name only
// returns a 'ready' attachment state for firewall and all associated subnets
func (m *MockNetworkFirewall) DescribeFirewall(input *networkfirewall.DescribeFirewallInput) (*networkfirewall.DescribeFirewallOutput, error) {
	if aws.StringValue(input.FirewallArn) != "" {
		return nil, fmt.Errorf("Does not support filtering on firewall ARN")
	}
	if _, ok := m.Firewalls[aws.StringValue(input.FirewallName)]; !ok {
		return nil, fmt.Errorf("Firewall %s not found", aws.StringValue(input.FirewallName))
	}

	output := &networkfirewall.DescribeFirewallOutput{
		Firewall: &networkfirewall.Firewall{
			FirewallName: input.FirewallName,
			FirewallArn:  aws.String(fmt.Sprintf("arn:aws:network-firewall:%s:%s:firewall/%s", m.Region, m.AccountID, *input.FirewallName)),
			VpcId:        aws.String(m.Firewalls[aws.StringValue(input.FirewallName)]),
		},
		FirewallStatus: &networkfirewall.FirewallStatus{
			SyncStates: make(map[string]*networkfirewall.SyncState),
			Status:     aws.String(networkfirewall.FirewallStatusValueReady),
		},
	}
	for subnetID, endpointID := range m.AssociatedSubnetToEndpoint {
		az, ok := m.SubnetIDToAZ[subnetID]
		if !ok {
			return nil, fmt.Errorf("Subnet %s not found", subnetID)
		}
		output.FirewallStatus.SyncStates[az] = &networkfirewall.SyncState{
			Attachment: &networkfirewall.Attachment{
				Status:     aws.String(networkfirewall.AttachmentStatusReady),
				EndpointId: aws.String(endpointID),
				SubnetId:   aws.String(subnetID),
			},
		}
	}
	return output, nil
}
