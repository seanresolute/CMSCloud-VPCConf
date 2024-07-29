package testmocks

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type MockVPCWriter struct {
	MM     *MockModelsManager
	Region database.Region
	VPCID  string
}

func (w *MockVPCWriter) UpdateState(state *database.VPCState) error {
	toUpdate := w.MM.VPCs[string(w.Region)+w.VPCID]
	if toUpdate == nil {
		toUpdate = &database.VPC{}
		w.MM.VPCs[string(w.Region)+w.VPCID] = toUpdate
	}

	toUpdate.State = state
	w.MM.VPCs[string(w.Region)+w.VPCID] = toUpdate
	return nil
}

func (w *MockVPCWriter) UpdateName(name string) error {
	return fmt.Errorf("Not implemented yet")
}

func (w *MockVPCWriter) MarkAsDeleted() error {
	return fmt.Errorf("Not implemented yet")
}

func (w *MockVPCWriter) UpdateIssues(issues []*database.Issue) error {
	toUpdate := w.MM.VPCs[string(w.Region)+w.VPCID]
	if toUpdate != nil {
		toUpdate.Issues = append(toUpdate.Issues, issues...)
		return nil
	}
	return fmt.Errorf("UpdateIssues Mock Error")
}

type MockModelsManager struct {
	VPC                              *database.VPC                                    // Should be removed once all tests use VPCs
	VPCs                             map[string]*database.VPC                         // region+VPC ID -> VPC State
	ResourceShares                   map[string]*database.TransitGatewayResourceShare // TG ID -> share
	ResolverRuleSets                 map[uint64]*database.ManagedResolverRuleSet
	VPCsPrimaryCIDR                  map[string]*string  // region+VPC ID -> CIDR
	VPCsSecondaryCIDRs               map[string][]string // region+VPC ID -> CIDRs
	PrimaryCIDR                      *string             // Should be removed once all tests use VPCsPrimaryCIDR
	SecondaryCIDRs                   []string            // Should be removed once all tests use VPCsSecondaryCIDRs
	TestRegion                       database.Region
	ManagedTransitGatewayAttachments []*database.ManagedTransitGatewayAttachment
}

func (m *MockModelsManager) CreateOrUpdateAWSAccount(account *database.AWSAccount) (databaseID uint64, err error) {
	return 0, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetMockState() *map[string]*database.VPC {
	return &m.VPCs
}

func (m *MockModelsManager) GetVPC(region database.Region, vpcID string) (*database.VPC, error) {
	vpc, ok := m.VPCs[string(region)+vpcID]
	if !ok {
		return nil, fmt.Errorf("VPC %q not found in region %s", vpcID, region)
	}
	// Serialize and deserialize to make a deep copy. Use gob because some fields are
	// set to be omitted when JSON-encoding.
	buf := new(bytes.Buffer)
	err := gob.NewEncoder(buf).Encode(vpc)
	if err != nil {
		return nil, fmt.Errorf("Error encoding in MockModelsManager: %s", err)
	}
	var vpc2 database.VPC
	err = gob.NewDecoder(buf).Decode(&vpc2)
	if err != nil {
		return nil, fmt.Errorf("Error decoding in MockModelsManager: %s", err)
	}
	return &vpc2, nil
}

func (m *MockModelsManager) GetDefaultVPCConfig(region database.Region) (*database.VPCConfig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MockModelsManager) GetOperableVPC(lockSet database.LockSet, region database.Region, vpcID string) (*database.VPC, database.VPCWriter, error) {
	if !lockSet.HasLock(database.TargetVPC(vpcID)) {
		return nil, nil, fmt.Errorf("LockSet does not hold a lock for %s", vpcID)
	}
	writer := &MockVPCWriter{
		MM:     m,
		Region: region,
		VPCID:  vpcID,
	}
	vpc, err := m.GetVPC(writer.Region, writer.VPCID)
	return vpc, writer, err
}

func (m *MockModelsManager) GetAutomatedVPCsForAccount(region database.Region, accountID string) ([]*database.VPC, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *MockModelsManager) ListAutomatedVPCs() ([]*database.VPC, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (m *MockModelsManager) ListExceptionVPCs() ([]*database.VPC, error) {
	return nil, fmt.Errorf("Not implemented")
}

// Will not update state, config, or issues
func (m *MockModelsManager) CreateOrUpdateVPC(vpc *database.VPC) (databaseID uint64, err error) {
	return 0, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) CreateVPCRequest(req *database.VPCRequest) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetVPCRequest(id uint64) (*database.VPCRequest, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetVPCRequests(accountID string) ([]*database.VPCRequest, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetAllVPCRequests() ([]*database.VPCRequest, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) SetVPCRequestApprovedConfig(id uint64, approvedConfig *database.AllocateConfig) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) SetVPCRequestTaskID(id uint64, taskID uint64) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) SetVPCRequestStatus(id uint64, status database.VPCRequestStatus) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) UpdateVPCConfig(region database.Region, vpcID string, config database.VPCConfig) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetAccount(accountID string) (*database.AWSAccount, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetManagedTransitGatewayAttachments() ([]*database.ManagedTransitGatewayAttachment, error) {
	return m.ManagedTransitGatewayAttachments, nil
}

func (m *MockModelsManager) GetLabels() ([]*database.Label, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteLabel(label string) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetVPCLabels(accountID string, vpcID string) ([]*database.Label, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) SetVPCLabel(accountID string, vpcID string, label string) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteVPCLabel(accountID string, vpcID string, label string) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetAccountLabels(accountID string) ([]*database.Label, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) SetAccountLabel(accountID string, label string) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteAccountLabel(accountID string, label string) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) ListBatchVPCLabels() ([]*database.VPCLabel, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) CreateManagedTransitGatewayAttachment(*database.ManagedTransitGatewayAttachment) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) UpdateManagedTransitGatewayAttachment(id uint64, mtga *database.ManagedTransitGatewayAttachment) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteManagedTransitGatewayAttachment(id uint64) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetTransitGatewayResourceShare(region database.Region, transitGatewayID string) (*database.TransitGatewayResourceShare, error) {
	if region != m.TestRegion {
		return nil, fmt.Errorf("Wrong region %q", region)
	}
	return m.ResourceShares[transitGatewayID], nil
}
func (m *MockModelsManager) CreateOrUpdateTransitGatewayResourceShare(share *database.TransitGatewayResourceShare) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteTransitGatewayResourceShare(region database.Region, transitGatewayID string) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetSecurityGroupSets() ([]*database.SecurityGroupSet, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) CreateSecurityGroupSet(*database.SecurityGroupSet) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) UpdateSecurityGroupSet(id uint64, set *database.SecurityGroupSet) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) DeleteSecurityGroupSet(id uint64) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetManagedResolverRuleSets() ([]*database.ManagedResolverRuleSet, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) CreateManagedResolverRuleSet(*database.ManagedResolverRuleSet) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) UpdateManagedResolverRuleSet(id uint64, mrr *database.ManagedResolverRuleSet) error {
	for idx, rs := range m.ResolverRuleSets {
		if rs.ID == id {
			m.ResolverRuleSets[idx] = mrr
			return nil
		}
	}
	return fmt.Errorf("Invalid ruleset ID: %d", id)
}
func (m *MockModelsManager) DeleteManagedResolverRuleSet(id uint64) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) VPCRequestLogInsert(vpcRequestID uint64, msg string, args ...interface{}) error {
	return fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetVPCRequestLogs(id uint64) ([]*database.VPCRequestLog, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) GetJIRAHealth() (*database.JIRAHealth, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
func (m *MockModelsManager) LockVPCRequest(id uint64) (*database.VPCRequest, *database.VPCRequestTransaction, error) {
	return nil, nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetDashboard() (*database.Dashboard, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetIPUsage() (*database.IPUsage, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) UpdateIPUsage(*database.IPUsage) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetVPCCIDRs(vpcID string, region database.Region) (*string, []string, error) {
	if _, ok := m.VPCsPrimaryCIDR[string(region)+vpcID]; !ok {
		holder := ""
		return &holder, []string{}, nil
	}
	primary := m.VPCsPrimaryCIDR[string(region)+vpcID]
	return primary, m.VPCsSecondaryCIDRs[string(region)+vpcID], nil
}

func (m *MockModelsManager) GetVPCDBID(vpcID string, region database.Region) (*uint64, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) DeleteVPCCIDR(vpcID string, region database.Region, cidr string) error {
	filteredCIDRs := []string{}
	for _, existing := range m.SecondaryCIDRs {
		if existing != cidr {
			filteredCIDRs = append(filteredCIDRs, existing)
		}
	}
	m.SecondaryCIDRs = filteredCIDRs
	return nil
}

func (m *MockModelsManager) SetVPCRequestProvisionedVPC(requestID uint64, region database.Region, vpcID string) error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) DeleteVPCCIDRs(vpcID string, region database.Region) error {
	m.VPCsPrimaryCIDR[string(region)+vpcID] = nil
	m.VPCsSecondaryCIDRs[string(region)+vpcID] = []string{}

	return nil
}

func (m *MockModelsManager) InsertVPCCIDR(vpcID string, region database.Region, cidr string, isPrimary bool) error {
	fmt.Printf("IncertVPCCIDR -- VPCID: %s REGION: %s CIDR: %s IS PRIMARY: %t\n", vpcID, region, cidr, isPrimary)
	if m.VPCsSecondaryCIDRs == nil {
		m.VPCsSecondaryCIDRs = make(map[string][]string)
	}

	if m.VPCsPrimaryCIDR == nil {
		m.VPCsPrimaryCIDR = make(map[string]*string)
	}

	if isPrimary {
		m.VPCsPrimaryCIDR[string(region)+vpcID] = &cidr
	} else {
		if m.VPCsSecondaryCIDRs[string(region)+vpcID] == nil {
			m.VPCsSecondaryCIDRs[string(region)+vpcID] = []string{}
		}
		for _, existingCIDR := range m.VPCsSecondaryCIDRs[string(region)+vpcID] {
			if existingCIDR == cidr {
				return nil
			}
		}
		m.VPCsSecondaryCIDRs[string(region)+vpcID] = append(m.VPCsSecondaryCIDRs[string(region)+vpcID], cidr)
	}

	return nil
}

func (m *MockModelsManager) GetAllAWSAccounts() ([]*database.AWSAccount, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) RecordUpdateAWSAccountsHeartbeat() error {
	return fmt.Errorf("Not implemented yet")
}

func (m *MockModelsManager) GetAWSAccountsLastSyncedinInterval(minutes int) (bool, error) {
	return false, fmt.Errorf("Not implemented yet")
}
