package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

var ErrVPCNotFound = errors.New("VPC not found")

var ErrDuplicateRecord = errors.New("record exists")
var ErrRecordNotFound = errors.New("record not found")

type AWSCreds struct {
	AccessKeyID, SecretAccessKey, SessionToken string
	Expiration                                 time.Time
}

type AWSAccount struct {
	ID, Name, ProjectName string
	IsGovCloud            bool
	IsApprover            bool

	// May or may not be present
	Creds *AWSCreds `json:"-"`
}

type RouteInfo struct {
	Destination         string
	NATGatewayID        string
	InternetGatewayID   string
	TransitGatewayID    string
	PeeringConnectionID string
	VPCEndpointID       string
}

type TransitGatewayAttachment struct {
	ManagedTransitGatewayAttachmentIDs []uint64 `json:"-"` // stored in created_managed_transit_gateway_attachment table
	TransitGatewayID                   string
	TransitGatewayAttachmentID         string
	SubnetIDs                          []string
}

type ResolverRuleAssociation struct {
	ResolverRuleID            string
	ResolverRuleAssociationID string
}

type SecurityGroup struct {
	TemplateID      uint64 `json:"-"` // stored in created_security_group table
	SecurityGroupID string
	Rules           []*SecurityGroupRule
}

type RouteTableInfo struct {
	RouteTableID        string `json:"-"` // filled in from keys of RouteTables map on VPCState
	Routes              []*RouteInfo
	SubnetType          SubnetType          // exactly one of SubnetType or EdgeAssociationType should be non-empty
	EdgeAssociationType EdgeAssociationType // exactly one of SubnetType or EdgeAssociationType should be non-empty
}

type EdgeAssociationType string

const (
	EdgeAssociationTypeIGW EdgeAssociationType = "IGW"
)

func (t *EdgeAssociationType) UnmarshalJSON(input []byte) error {
	var str string
	err := json.Unmarshal(input, &str) // Remove quotes
	if err != nil {
		return err
	}
	str = strings.ToLower(str)
	if str == "" {
		*t = ""
		return nil
	}
	if str == strings.ToLower(string(EdgeAssociationTypeIGW)) {
		*t = EdgeAssociationTypeIGW
		return nil
	}
	return fmt.Errorf("Unknown edge association type %q", str)
}

type NATGatewayInfo struct {
	NATGatewayID string
	EIPID        string
}

type SubnetInfo struct {
	SubnetID                string
	GroupName               string
	RouteTableAssociationID string
	CustomRouteTableID      string
}

type RequestType int

const (
	RequestTypeNewVPC RequestType = iota
	RequestTypeNewSubnet
)

func (rt *RequestType) UnmarshalJSON(b []byte) error {
	var i int
	if err := json.Unmarshal(b, &i); err != nil {
		return err
	}
	switch i {
	case 0:
		*rt = RequestTypeNewVPC
	case 1:
		*rt = RequestTypeNewSubnet
	default:
		return fmt.Errorf("Invalid request type %q", i)
	}
	return nil
}

type SubnetType string

const (
	SubnetTypePrivate    SubnetType = "Private"
	SubnetTypePublic     SubnetType = "Public"
	SubnetTypeApp        SubnetType = "App"
	SubnetTypeData       SubnetType = "Data"
	SubnetTypeWeb        SubnetType = "Web"
	SubnetTypeTransport  SubnetType = "Transport"
	SubnetTypeTransitive SubnetType = "Transitive"
	SubnetTypeSecurity   SubnetType = "Security"
	SubnetTypeManagement SubnetType = "Management"
	SubnetTypeShared     SubnetType = "Shared"
	SubnetTypeSharedOC   SubnetType = "Shared-OC"
	SubnetTypeUnroutable SubnetType = "Unroutable"
	SubnetTypeFirewall   SubnetType = "Firewall"
)

func AllSubnetTypes() []SubnetType {
	return []SubnetType{
		SubnetTypePrivate,
		SubnetTypePublic,
		SubnetTypeApp,
		SubnetTypeData,
		SubnetTypeWeb,
		SubnetTypeTransport,
		SubnetTypeTransitive,
		SubnetTypeSecurity,
		SubnetTypeManagement,
		SubnetTypeShared,
		SubnetTypeSharedOC,
		SubnetTypeUnroutable,
		SubnetTypeFirewall,
	}
}

func (t SubnetType) IsDefaultType() bool {
	return t == SubnetTypePrivate || t == SubnetTypePublic || t == SubnetTypeFirewall
}

// IP Space split into "Lower" and "Prod"
func (t SubnetType) HasSplitIPSpace() bool {
	return t == SubnetTypeApp || t == SubnetTypeData || t == SubnetTypeWeb || t == SubnetTypeShared || t == SubnetTypeSharedOC
}

func (t SubnetType) VRFName(stack string) string {
	if t == SubnetTypeManagement {
		return "vpn_edc_mgmt"
	}
	if t == SubnetTypeSecurity {
		return "vpn_security"
	}
	if t == SubnetTypeTransport {
		return "vpn_transport"
	}
	// S.M.
	// Update to add mgmt to VRF production.
	/*
		if strings.ToLower(stack) == "prod" {
			return map[SubnetType]string{
				SubnetTypeApp:  "vpn_app_unix",
				SubnetTypeData: "vpn_data_unix",
				SubnetTypeWeb:  "vpn_pres_unix",
			}[t]
		}
	*/
	if strings.ToLower(stack) == "prod" || strings.ToLower(stack) == "mgmt" {
		return map[SubnetType]string{
			SubnetTypeApp:  "vpn_app_unix",
			SubnetTypeData: "vpn_data_unix",
			SubnetTypeWeb:  "vpn_pres_unix",
		}[t]
	}
	return map[SubnetType]string{
		SubnetTypeApp:  "vpn_app_unix_imp",
		SubnetTypeData: "vpn_data_unix_imp",
		SubnetTypeWeb:  "vpn_pres_unix_imp",
	}[t]
}

func (t *SubnetType) UnmarshalJSON(input []byte) error {
	var str string
	err := json.Unmarshal(input, &str) // Remove quotes
	if err != nil {
		return err
	}
	str = strings.ToLower(str)
	if str == "" {
		*t = ""
		return nil
	}
	for _, choice := range []SubnetType{
		SubnetTypePrivate,
		SubnetTypePublic,
		SubnetTypeApp,
		SubnetTypeData,
		SubnetTypeWeb,
		SubnetTypeTransport,
		SubnetTypeTransitive,
		SubnetTypeSecurity,
		SubnetTypeManagement,
		SubnetTypeShared,
		SubnetTypeSharedOC,
		SubnetTypeUnroutable,
		SubnetTypeFirewall,
	} {
		if str == strings.ToLower(string(choice)) {
			*t = choice
			return nil
		}
	}
	return fmt.Errorf("Unknown subnet type %q", str)
}

func (t *SubnetType) Scan(src interface{}) error {
	s, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("Invalid data for SubnetType")
	}
	*t = SubnetType(s)
	return nil
}

type AvailabilityZoneInfra struct {
	Subnets             map[SubnetType][]*SubnetInfo
	PrivateRouteTableID string
	PublicRouteTableID  string // "" for V1
	NATGateway          NATGatewayInfo
}

type InternetGatewayInfo struct {
	InternetGatewayID         string
	IsInternetGatewayAttached bool
	RouteTableID              string
	RouteTableAssociationID   string
}

type PeeringConnection struct {
	RequesterVPCID      string
	RequesterRegion     Region
	AccepterVPCID       string
	AccepterRegion      Region
	PeeringConnectionID string
	IsAccepted          bool
}

type Firewall struct {
	AssociatedSubnetIDs []string
}

type VPCType int

func (t VPCType) IsV1Variant() bool {
	return t == VPCTypeV1 || t == VPCTypeV1Firewall
}

func (t VPCType) IsMigrating() bool {
	return t == VPCTypeMigratingV1ToV1Firewall || t == VPCTypeMigratingV1FirewallToV1
}

func (t VPCType) CanModifyAvailabilityZones() bool {
	return t.IsV1Variant()
}

func (t VPCType) CanUpdateZonedSubnets() bool {
	return t.IsV1Variant() || t.IsMigrating()
}

func (t VPCType) CanImportVPC() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanDeleteVPC() bool {
	return t.IsV1Variant() || t.IsMigrating()
}

func (t VPCType) CanUpdateSecurityGroups() bool {
	return t.IsV1Variant()
}

func (t VPCType) CanUpdateCMSNet() bool {
	return t.IsV1Variant()
}

func (t VPCType) HasFirewall() bool {
	return t == VPCTypeV1Firewall || t == VPCTypeMigratingV1ToV1Firewall
}

func (t VPCType) CanUpdateNetworkFirewall() bool {
	return t.IsV1Variant() || t.IsMigrating()
}

func (t VPCType) CanUpdateVPCType() bool {
	return t.IsV1Variant() || t.IsMigrating()
}

func (t VPCType) CanUpdateVPCName() bool {
	return t.IsV1Variant()
}

func (t VPCType) CanDeleteUnusedResources() bool {
	return t.IsMigrating()
}

func (t VPCType) CanVerifyVPC() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanRepairVPC() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanSynchronizeRouteTable() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanUpdateResolverRules() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanUpdateMTGAs() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanUpdatePeering() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

func (t VPCType) CanUpdateLogging() bool {
	return t.IsV1Variant() || t == VPCTypeLegacy
}

// this needs to be updated if new types are added below
func (t VPCType) String() string {
	switch t {
	case VPCTypeV1:
		return "VPCTypeV1"
	case VPCTypeLegacy:
		return "VPCTypeLegacy"
	case VPCTypeException:
		return "VPCTypeException"
	case VPCTypeV1Firewall:
		return "VPCTypeV1Firewall"
	case VPCTypeMigratingV1ToV1Firewall:
		return "VPCTypeMigratingV1ToV1Firewall"
	case VPCTypeMigratingV1FirewallToV1:
		return "VPCTypeMigratingV1FirewallToV1"
	default:
		return "Unknown"
	}
}

func GetVPCTypes() []VPCType {
	return []VPCType{
		VPCTypeV1,
		VPCTypeLegacy,
		VPCTypeException,
		VPCTypeV1Firewall,
		VPCTypeMigratingV1ToV1Firewall,
		VPCTypeMigratingV1FirewallToV1,
	}
}

// V1 VPCs are greenfield VPCs created by VPC Conf.
// Legacy VPCs are VPCs not created by us, for which we support only:
// - Import
// - Manage transit gateway attachments and associated routes
// - Manage peering connections
// - Manage resolver rule sharing
// Exception VPCs are VPCs which don't meet current spec https://docs.google.com/document/d/1XPsZPiUMtvnq9GTJ9eX9i54vGvahp4Bs27-usdf2Y6s/edit#
// and are managed by the EVM.
// V1Firewall VPCs have feature parity with V1 VPCs, but follow the reference architecture: https://confluenceent.cms.gov/display/ITOPS/Network+Firewall+VPC+Design+Doc#NetworkFirewallVPCDesignDoc-Architecture
// Migrating{start_type}To{end_type} VPCs are in the middle of a migration.  The only operations supported for these types are those that implement/revert the migration.

const (
	VPCTypeV1 VPCType = iota
	VPCTypeLegacy
	VPCTypeException
	VPCTypeV1Firewall
	VPCTypeMigratingV1ToV1Firewall
	VPCTypeMigratingV1FirewallToV1
)

type AZMap map[string]*AvailabilityZoneInfra
type NamedAvailabilityZoneInfra struct {
	Name string
	*AvailabilityZoneInfra
}

func (m AZMap) InOrder() []*NamedAvailabilityZoneInfra {
	azNames := []string{}
	for azName := range m {
		azNames = append(azNames, azName)
	}
	sort.Strings(azNames)
	azs := []*NamedAvailabilityZoneInfra{}
	for _, azName := range azNames {
		azs = append(azs, &NamedAvailabilityZoneInfra{azName, m[azName]})
	}
	return azs
}

type VPCState struct {
	VPCType                         VPCType                    // default type is V1
	PublicRouteTableID              string                     // "" for V1Firewall
	RouteTables                     map[string]*RouteTableInfo // RT id -> info
	InternetGateway                 InternetGatewayInfo
	AvailabilityZones               AZMap // AZ name -> info
	TransitGatewayAttachments       []*TransitGatewayAttachment
	ResolverRuleAssociations        []*ResolverRuleAssociation
	PeeringConnections              []*PeeringConnection `json:"-"` // stored in created_peering_connection table
	SecurityGroups                  []*SecurityGroup
	S3FlowLogID                     string
	CloudWatchLogsFlowLogID         string
	ResolverQueryLogConfigurationID string
	ResolverQueryLogAssociationID   string
	Firewall                        *Firewall // nil for V1
	FirewallRouteTableID            string    // "" for V1
}

func (infra *VPCState) GetAvailabilityZoneInfo(azName string) *AvailabilityZoneInfra {
	az := infra.AvailabilityZones[azName]
	if az == nil {
		az = &AvailabilityZoneInfra{
			Subnets: map[SubnetType][]*SubnetInfo{},
		}
		infra.AvailabilityZones[azName] = az
	}
	return az
}

type PeeringConnectionConfig struct {
	IsRequester                 bool
	OtherVPCID                  string
	OtherVPCRegion              Region
	OtherVPCAccountID           string
	ConnectPrivate              bool
	ConnectSubnetGroups         []string
	OtherVPCConnectPrivate      bool
	OtherVPCConnectSubnetGroups []string
}

type VPCConfig struct {
	ConnectPublic                      bool
	ConnectPrivate                     bool
	ManagedTransitGatewayAttachmentIDs []uint64                   `json:"-"` // stored in configured_managed_transit_gateway_attachment table
	SecurityGroupSetIDs                []uint64                   `json:"-"` // stored in configured_security_group_set table
	ManagedResolverRuleSetIDs          []uint64                   `json:"-"` // stored in configured_managed_resolver_rule_set table
	PeeringConnections                 []*PeeringConnectionConfig `json:"-"` // stored in configured_peering_connection table
}

type VerifyTypes uint64

const (
	VerifyNetworking     VerifyTypes = 1 << iota
	VerifyLogging        VerifyTypes = 1 << iota
	VerifyResolverRules  VerifyTypes = 1 << iota
	VerifySecurityGroups VerifyTypes = 1 << iota
	VerifyCIDRs          VerifyTypes = 1 << iota
	VerifyCMSNet         VerifyTypes = 1 << iota
)

func bitmapIncludes(superset, subset uint64) bool {
	return superset&subset == subset
}

func (t VerifyTypes) Includes(sub VerifyTypes) bool {
	return bitmapIncludes(uint64(t), uint64(sub))
}

type Issue struct {
	Description       string
	AffectedSubnetIDs []string
	Type              VerifyTypes
	IsFixable         bool
}

type VPC struct {
	AccountID, ID, Name, Stack string
	Region                     Region
	State                      *VPCState // null means "not automated"
	Issues                     []*Issue
	Config                     *VPCConfig
}

type VPCLabel struct {
	Region Region
	ID     string
	Label  string
}

type Session struct {
	Key                string
	UserID             int
	IsAdmin            bool
	CloudTamerToken    string
	AuthorizedAccounts []*AWSAccount
	Username           string
}

type SessionUser struct {
	Name    string
	Email   string
	LoginID string
}

type Region string

func (r Region) IsGovCloud() bool {
	return strings.HasPrefix(string(r), "us-gov-")
}

type ManagedResolverRuleSet struct {
	ID              uint64
	IsGovCloud      bool
	Name            string
	Region          Region
	ResourceShareID string
	AccountID       string
	Rules           []*ResolverRule
	InUseVPCs       []string
	IsDefault       bool
}

type ResolverRule struct {
	ID          uint64
	AWSID       string
	Description string
}

type Label struct {
	ID   uint64
	Name string
}

type ManagedTransitGatewayAttachment struct {
	ID               uint64
	IsGovCloud       bool
	Name             string
	TransitGatewayID string
	Region           Region
	Routes           []string
	SubnetTypes      []SubnetType
	InUseVPCs        []string
	IsDefault        bool
}

type SecurityGroupRule struct {
	Description    string
	IsEgress       bool
	Protocol       string
	FromPort       int64
	ToPort         int64
	Source         string
	SourceIPV6CIDR string
}

type SecurityGroupTemplate struct {
	ID          uint64
	Name        string
	Description string
	Rules       []*SecurityGroupRule
}

type SecurityGroupSet struct {
	ID         uint64
	Name       string
	Groups     []*SecurityGroupTemplate
	InUseVPCs  []string
	IsDefault  bool
	Region     Region
	IsGovCloud bool
}

type TransitGatewayResourceShare struct {
	AccountID        string
	Region           Region
	TransitGatewayID string
	ResourceShareID  string
}

type ResolverRuleSetResourceShare struct {
	AccountID       string
	Region          Region
	Name            string
	ResourceShareID string
}

type VPCRequestStatus int

func (rs *VPCRequestStatus) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	case "submitted":
		*rs = StatusSubmitted
	case "cancelledbyrequester":
		*rs = StatusCancelledByRequester
	case "rejected":
		*rs = StatusRejected
	case "approved":
		*rs = StatusApproved
	case "inprogress":
		*rs = StatusInProgress
	case "done":
		*rs = StatusDone
	case "unknown":
		*rs = StatusUnknown
	default:
		return fmt.Errorf("Invalid VPCRequest value %q", s)
	}
	return nil
}

const (
	StatusSubmitted VPCRequestStatus = iota
	StatusCancelledByRequester
	StatusRejected
	StatusApproved
	StatusInProgress
	StatusDone

	StatusUnknown = -1
)

const (
	WaitForMissingAWSStatus = "Fake AWS status to look for when waiting for a resource to disappear"
)

type VPCRequest struct {
	ID                                          uint64
	AddedAt                                     time.Time
	AccountID, AccountName, ProjectName         string
	RequesterUID, RequesterName, RequesterEmail string
	RequestType                                 RequestType
	RelatedIssues                               []string
	Comment                                     string
	IPJustification                             string
	Status                                      VPCRequestStatus
	HasJIRAErrors                               bool
	JIRAIssue                                   *string
	RequestedConfig                             AllocateConfig
	ApprovedConfig                              *AllocateConfig
	TaskID                                      *uint64
	TaskStatus                                  *int // TaskStatus
	DependentTasks                              []struct {
		Description string
		ID          uint64
		Status      TaskStatus
	}
	ProvisionedVPC *struct {
		ID     string
		Region Region
	}
}

type VPCRequestLog struct {
	ID            uint64    `json:"id"`
	VPCRequestID  uint64    `json:"vpc_request_id"`
	AddedAt       time.Time `json:"added_at"`
	RetryAttempts int       `json:"retry_attempts"`
	Message       string    `json:"message"`
}

type AllocateConfig struct {
	ParentContainer string
	Stack           string

	AccountID            string
	VPCName              string
	VPCID                string
	AvailabilityZones    []string
	IsDefaultDedicated   bool
	PrivateSize          int
	PublicSize           int
	NumPrivateSubnets    int
	NumPublicSubnets     int
	AddContainersSubnets bool
	AddFirewall          bool

	SubnetType string
	SubnetSize int
	GroupName  string

	AWSRegion     string
	AutoProvision bool
}

type JIRAHealth struct {
	NumErrors        int
	OldestNumRetries int
	OldestAddedAt    *time.Time
}

type DNSTLSResourceType int

func (rt *DNSTLSResourceType) String() string {
	r := *rt
	switch r {
	case DNSTLSResourceTypeDomainName:
		return "DNS"
	case DNSTLSResourceTypeCertificate:
		return "TLS"
	}
	return "Unknown"
}

const (
	DNSTLSResourceTypeUnknown DNSTLSResourceType = iota
	DNSTLSResourceTypeDomainName
	DNSTLSResourceTypeCertificate
)

type RequestAction int

const (
	RequestActionUnknown RequestAction = iota
	RequestActionProvision
	RequestActionDelete
)

type DNSTLSRequestStatus int

func (rs *DNSTLSRequestStatus) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch strings.ToLower(s) {
	case "submitted":
		*rs = DNSTLSStatusSubmitted
	case "cancelled":
		*rs = DNSTLSStatusCancelled
	case "rejected":
		*rs = DNSTLSStatusRejected
	case "approved":
		*rs = DNSTLSStatusApproved
	case "inprogress":
		*rs = DNSTLSStatusInProgress
	case "done":
		*rs = DNSTLSStatusDone
	case "unknown":
		*rs = DNSTLSStatusUnknown
	default:
		return fmt.Errorf("Invalid DNSTLSRequest value %q", s)
	}
	return nil
}

const (
	DNSTLSStatusSubmitted DNSTLSRequestStatus = iota
	DNSTLSStatusCancelled
	DNSTLSStatusRejected
	DNSTLSStatusApproved
	DNSTLSStatusInProgress
	DNSTLSStatusDone

	DNSTLSStatusUnknown = -1
)

type DNSRecordType string

const (
	DNSRecordTypeCNAME DNSRecordType = "CNAME"
	DNSRecordTypeTXT   DNSRecordType = "TXT"
)

type DomainName struct {
	ID                                uint64
	AccountID, Zone, Name, RecordType string
	Rdata                             []string
	TTL                               int
}

type DomainNameInfo struct {
	Zone       string
	Names      []string
	Target     string
	RecordType DNSRecordType
}

type CertificateRequestStatus int

const (
	CertificateStatusSubmitted CertificateRequestStatus = iota
	CertificateStatusCancelledByRequester
	CertificateStatusRejected
	CertificateStatusApproved
)

type ValidationRecord struct {
	Subject    string
	Challenge  string
	Response   string
	RecordType string
}

type Certificate struct {
	ID             uint64
	AccountID      string
	Region         Region
	AWSARN         string
	Name           string
	ValidationInfo ValidationRecord
	AlternateNames []*ValidationRecord
}

type CertificateInfo struct {
	AccountID      string
	Name           string
	AlternateNames []*string
	Region         Region
}

type DeleteRequestStatus int

const (
	DeleteStatusSubmitted DeleteRequestStatus = iota
	DeleteStatusCancelledByRequester
	DeleteStatusRejected
	DeleteStatusApproved
)

type DeleteInfo struct {
	AccountID    string
	ResourceType DNSTLSResourceType
	RecordID     uint64
	DomainName   *DomainName
	Certificate  *Certificate
}

type DNSTLSInfo struct {
	CertificateInfo *CertificateInfo
	DeleteInfo      *DeleteInfo
	DomainNameInfo  *DomainNameInfo
}

type DNSTLSRequest struct {
	ID                                          uint64
	AddedAt                                     time.Time
	AccountID, AccountName, ProjectName         string
	RequesterUID, RequesterName, RequesterEmail string
	RequestAction                               RequestAction
	ResourceType                                DNSTLSResourceType
	Status                                      DNSTLSRequestStatus
	RequestedInfo                               DNSTLSInfo
	ApprovedInfo                                *DNSTLSInfo
	JIRAIssue                                   *string
	TaskID                                      *uint64
	TaskStatus                                  *int // task.Status
}

type DNSTLSRecord struct {
	Subject           string
	RecordType        DNSTLSResourceType
	CreatedAt         time.Time
	DeletedAt         *time.Time
	CertificateRecord *Certificate
	DomainNameRecord  *DomainName
}

type VPCRequestTransaction struct {
	requestID uint64
	tx        *sqlx.Tx
}

func (tx *VPCRequestTransaction) Commit() error {
	return tx.tx.Commit()
}

func (tx *VPCRequestTransaction) Rollback() error {
	return tx.tx.Rollback()
}

func (tx *VPCRequestTransaction) SetVPCRequestJIRAIssue(issue string) error {
	q := "UPDATE vpc_request SET jira_issue=:issue WHERE id=:id"
	_, err := tx.tx.NamedExec(q, map[string]interface{}{
		"id":    tx.requestID,
		"issue": issue,
	})
	return err
}

// A VPCWriter gives you the ability to update the name, state, issues,
// and deleted status of a VPC.
type VPCWriter interface {
	MarkAsDeleted() error
	UpdateName(name string) error
	UpdateState(state *VPCState) error
	UpdateIssues(issues []*Issue) error
}

type sqlVPCWriter struct {
	mm     *SQLModelsManager
	region Region
	vpcID  string
}

type ModelsManager interface {
	// ID, AddedAt, AccountName and ProjectName will be ignored
	// FindRequest(id uint64) (ResourceType, interface{}, error)
	// FindRecord(id uint64, resourceType ResourceType) (interface{}, error)

	CreateVPCRequest(req *VPCRequest) error
	GetVPCRequest(id uint64) (*VPCRequest, error)
	// The caller *must* call Commit() or Rollback() on the returned VPCRequestTransaction
	LockVPCRequest(id uint64) (*VPCRequest, *VPCRequestTransaction, error)
	GetVPCRequests(accountID string) ([]*VPCRequest, error)
	GetAllVPCRequests() ([]*VPCRequest, error)
	SetVPCRequestApprovedConfig(id uint64, approvedConfig *AllocateConfig) error
	SetVPCRequestTaskID(id uint64, taskID uint64) error
	SetVPCRequestStatus(id uint64, status VPCRequestStatus) error
	SetVPCRequestProvisionedVPC(requestID uint64, region Region, vpcID string) error

	GetVPCRequestLogs(vpcRequestID uint64) ([]*VPCRequestLog, error)
	// VPCRequestLogInsert inserts an entry for the given vpc_request_id or
	// increments the retry_attempts on conflict (vpc_request_id, message).
	VPCRequestLogInsert(vpcRequestID uint64, msg string, args ...interface{}) error

	GetAccount(accountID string) (*AWSAccount, error)
	GetAllAWSAccounts() ([]*AWSAccount, error)
	CreateOrUpdateAWSAccount(account *AWSAccount) (databaseID uint64, err error)
	RecordUpdateAWSAccountsHeartbeat() error
	GetAWSAccountsLastSyncedinInterval(minutes int) (bool, error)

	GetVPC(region Region, vpcID string) (*VPC, error)
	GetOperableVPC(lockSet LockSet, region Region, vpcID string) (*VPC, VPCWriter, error)
	GetAutomatedVPCsForAccount(region Region, accountID string) ([]*VPC, error)
	// Will only update name and stack
	CreateOrUpdateVPC(vpc *VPC) (databaseID uint64, err error)
	// Non-deleted and non-exception only; state/config/issues are not filled out
	ListAutomatedVPCs() ([]*VPC, error)
	// Region/state/config/issues are not filled out
	ListExceptionVPCs() ([]*VPC, error)

	UpdateVPCConfig(region Region, vpcID string, config VPCConfig) error

	GetLabels() ([]*Label, error)
	DeleteLabel(label string) error
	GetVPCLabels(region string, vpcID string) ([]*Label, error)
	SetVPCLabel(region string, vpcID string, label string) error
	DeleteVPCLabel(region string, vpcID string, label string) error
	GetAccountLabels(accountID string) ([]*Label, error)
	SetAccountLabel(accountID string, label string) error
	DeleteAccountLabel(accountID string, label string) error

	ListBatchVPCLabels() ([]*VPCLabel, error)

	GetManagedTransitGatewayAttachments() ([]*ManagedTransitGatewayAttachment, error)
	// ID field will be set
	CreateManagedTransitGatewayAttachment(*ManagedTransitGatewayAttachment) error
	UpdateManagedTransitGatewayAttachment(id uint64, mtga *ManagedTransitGatewayAttachment) error
	DeleteManagedTransitGatewayAttachment(id uint64) error

	GetTransitGatewayResourceShare(region Region, transitGatewayID string) (*TransitGatewayResourceShare, error)
	// Will be identified by region + transit gateway id
	CreateOrUpdateTransitGatewayResourceShare(share *TransitGatewayResourceShare) error
	DeleteTransitGatewayResourceShare(region Region, transitGatewayID string) error

	GetManagedResolverRuleSets() ([]*ManagedResolverRuleSet, error)

	CreateManagedResolverRuleSet(*ManagedResolverRuleSet) error
	UpdateManagedResolverRuleSet(id uint64, mrr *ManagedResolverRuleSet) error
	DeleteManagedResolverRuleSet(id uint64) error

	GetSecurityGroupSets() ([]*SecurityGroupSet, error)
	// ID fields will be set
	CreateSecurityGroupSet(*SecurityGroupSet) error
	UpdateSecurityGroupSet(id uint64, set *SecurityGroupSet) error
	DeleteSecurityGroupSet(id uint64) error

	// GetJIRAHealth returns a JIRAHealth struct with the number of unresolved issues.
	// If NumErrors is > 0 it adds the timestamp and retries of the oldest entry.
	GetJIRAHealth() (*JIRAHealth, error)

	GetDashboard() (*Dashboard, error)
	GetIPUsage() (*IPUsage, error)
	UpdateIPUsage(ipUsage *IPUsage) error

	GetVPCCIDRs(vpcID string, region Region) (*string, []string, error)
	GetVPCDBID(vpcID string, region Region) (*uint64, error)
	DeleteVPCCIDR(vpcID string, region Region, cidr string) error
	DeleteVPCCIDRs(vpcID string, region Region) error
	InsertVPCCIDR(vpcID string, region Region, cidr string, isPrimary bool) error

	GetDefaultVPCConfig(region Region) (*VPCConfig, error)
}

type SQLModelsManager struct {
	DB *sqlx.DB
}

func (m *SQLModelsManager) DeleteTransitGatewayResourceShare(region Region, transitGatewayID string) error {
	q := "DELETE FROM transit_gateway_resource_share WHERE aws_region=:region AND transit_gateway_id = :transitGatewayID"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"region":           region,
		"transitGatewayID": transitGatewayID,
	})
	return err
}

func (m *SQLModelsManager) CreateOrUpdateTransitGatewayResourceShare(share *TransitGatewayResourceShare) error {
	q := `
		INSERT INTO transit_gateway_resource_share
			(aws_account_id, aws_region, transit_gateway_id, resource_share_id)
		VALUES (
			(SELECT id FROM aws_account WHERE aws_id=:accountID),
			:region,
			:transitGatewayID,
			:resourceShareID)
		ON CONFLICT(aws_region, transit_gateway_id)
			DO UPDATE SET
				aws_account_id=(SELECT id FROM aws_account WHERE aws_id=:accountID),
				resource_share_id=:resourceShareID`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"region":           share.Region,
		"transitGatewayID": share.TransitGatewayID,
		"resourceShareID":  share.ResourceShareID,
		"accountID":        share.AccountID,
	})
	return err
}

func (m *SQLModelsManager) GetTransitGatewayResourceShare(region Region, transitGatewayID string) (share *TransitGatewayResourceShare, err error) {
	share = &TransitGatewayResourceShare{
		Region:           region,
		TransitGatewayID: transitGatewayID,
	}
	q := "SELECT aws_account.aws_id, share.resource_share_id FROM transit_gateway_resource_share share INNER JOIN aws_account ON share.aws_account_id=aws_account.id WHERE aws_region=:region AND transit_gateway_id = :transitGatewayID"
	rewritten, args, err := m.DB.BindNamed(q, map[string]interface{}{
		"transitGatewayID": transitGatewayID,
		"region":           region,
	})
	if err != nil {
		return nil, err
	}
	err = m.DB.QueryRow(rewritten, args...).Scan(&share.AccountID, &share.ResourceShareID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return
}

func (m *SQLModelsManager) GetLabels() ([]*Label, error) {
	q := `SELECT id, name FROM label ORDER BY name ASC`
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	labels := []*Label{}
	for rows.Next() {
		var label Label
		err := rows.Scan(
			&label.ID,
			&label.Name)
		if err != nil {
			return nil, err
		}
		labels = append(labels, &label)
	}
	return labels, nil
}

func (m *SQLModelsManager) DeleteLabel(label string) error {
	q := `
		DELETE FROM label
		WHERE name=:label
			AND NOT EXISTS(SELECT 1 FROM vpc_label WHERE label_id = label.id)
			AND NOT EXISTS(SELECT 1 FROM account_label WHERE label_id = label.id)`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"label": label,
	})
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) GetVPCLabels(region string, vpcID string) ([]*Label, error) {
	q := `
		SELECT label.id, label.name
		FROM vpc_label vlabel
			INNER JOIN label label ON label.id = vlabel.label_id
			INNER JOIN vpc vpc ON vpc.id = vlabel.vpc_id
		WHERE vpc.aws_region=:region AND vpc.aws_id=:vpcID
		ORDER BY label.name ASC`
	rows, err := m.DB.NamedQuery(q, map[string]interface{}{
		"region": region,
		"vpcID":  vpcID,
	})
	if err != nil {
		return nil, err
	}
	labels := []*Label{}
	for rows.Next() {
		var label Label
		err := rows.Scan(
			&label.ID,
			&label.Name)
		if err != nil {
			return nil, err
		}
		labels = append(labels, &label)
	}
	return labels, nil
}

func (m *SQLModelsManager) SetVPCLabel(region string, vpcID string, label string) error {
	q := `
		WITH first_insert as (
			INSERT into label(name) 
			VALUES(:label)
			ON CONFLICT("name") DO UPDATE SET name=EXCLUDED.name RETURNING id
		)
		INSERT INTO vpc_label (vpc_id, label_id) 
		SELECT vpc.id, (SELECT id FROM first_insert) 
		FROM vpc vpc 
		WHERE vpc.aws_region=:region AND vpc.aws_id=:vpcID
		ON CONFLICT DO NOTHING`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"region": region,
		"vpcID":  vpcID,
		"label":  label,
	})
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) DeleteVPCLabel(region string, vpcID string, label string) error {
	q := `
		DELETE FROM vpc_label 
		WHERE vpc_id IN (SELECT id FROM vpc WHERE aws_id=:vpcID AND aws_region=:region) 
			  AND label_id IN (SELECT id FROM label WHERE name=:label)
		`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"region": region,
		"vpcID":  vpcID,
		"label":  label,
	})
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) GetAccountLabels(accountID string) ([]*Label, error) {
	q := `
		SELECT label.id, label.name
		FROM account_label alabel
			INNER JOIN label label ON label.id = alabel.label_id
			INNER JOIN aws_account account on account.id = alabel.account_id
		WHERE account.aws_id = $1
		ORDER BY label.name ASC`
	rows, err := m.DB.Queryx(q, accountID)
	if err != nil {
		return nil, err
	}
	labels := []*Label{}
	for rows.Next() {
		var label Label
		err := rows.Scan(
			&label.ID,
			&label.Name)
		if err != nil {
			return nil, err
		}
		labels = append(labels, &label)
	}
	return labels, nil
}

func (m *SQLModelsManager) SetAccountLabel(accountID string, label string) error {
	q := `
		WITH first_insert as (
			INSERT into label(name) 
			VALUES(:label)
			ON CONFLICT("name") DO UPDATE SET name=EXCLUDED.name RETURNING id
		)
		INSERT INTO account_label (account_id, label_id) 
		SELECT account.id, (SELECT id FROM first_insert) 
		FROM aws_account account
		WHERE account.aws_id=:accountID
		ON CONFLICT DO NOTHING`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"accountID": accountID,
		"label":     label,
	})
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) DeleteAccountLabel(accountID string, label string) error {
	q := `
		DELETE FROM account_label 
		WHERE account_id IN (SELECT id FROM aws_account WHERE aws_id=:accountID) 
			  AND label_id IN (SELECT id FROM label WHERE name=:label)
		`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"accountID": accountID,
		"label":     label,
	})
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) ListBatchVPCLabels() ([]*VPCLabel, error) {
	q := `
		SELECT vpc.aws_region, vpc.aws_id, label.name
		FROM vpc_label vlabel
			INNER JOIN label label ON label.id = vlabel.label_id
			INNER JOIN vpc vpc ON vpc.id = vlabel.vpc_id
		WHERE (vpc.state->>'VPCType')::integer != $1 AND NOT vpc.is_deleted
		ORDER BY vpc.name ASC`
	rows, err := m.DB.Queryx(q, VPCTypeException)
	if err != nil {
		return nil, err
	}
	vpcLabels := []*VPCLabel{}
	for rows.Next() {
		var vpcLabel VPCLabel
		err := rows.Scan(
			&vpcLabel.Region,
			&vpcLabel.ID,
			&vpcLabel.Label)
		if err != nil {
			return nil, err
		}
		vpcLabels = append(vpcLabels, &vpcLabel)
	}
	return vpcLabels, nil
}

func (m *SQLModelsManager) CreateManagedTransitGatewayAttachment(mtga *ManagedTransitGatewayAttachment) error {
	q := "INSERT INTO managed_transit_gateway_attachment (transit_gateway_id, region, is_gov_cloud, name, routes, subnet_types, is_default) VALUES (:transitGatewayID, :region, :isGovCloud, :name, :routes, :subnetTypes, :isDefault) RETURNING id"
	rewritten, args, err := m.DB.BindNamed(q, map[string]interface{}{
		"transitGatewayID": mtga.TransitGatewayID,
		"region":           mtga.Region,
		"isGovCloud":       mtga.IsGovCloud,
		"name":             mtga.Name,
		"routes":           pq.Array(mtga.Routes),
		"subnetTypes":      pq.Array(mtga.SubnetTypes),
		"isDefault":        mtga.IsDefault,
	})
	if err != nil {
		return err
	}
	err = m.DB.Get(&mtga.ID, rewritten, args...)
	if err != nil {
		return err
	}
	return err
}

func (m *SQLModelsManager) UpdateManagedTransitGatewayAttachment(id uint64, mtga *ManagedTransitGatewayAttachment) error {
	if mtga.ID != 0 && mtga.ID != id {
		return errors.New("Updating ID is not supported")
	}
	q := "UPDATE managed_transit_gateway_attachment SET transit_gateway_id=:transitGatewayID, region=:region, is_gov_cloud=:isGovCloud, name=:name, routes=:routes, subnet_types=:subnetTypes, is_default=:isDefault WHERE id=:id"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"transitGatewayID": mtga.TransitGatewayID,
		"region":           mtga.Region,
		"isGovCloud":       mtga.IsGovCloud,
		"name":             mtga.Name,
		"routes":           pq.Array(mtga.Routes),
		"subnetTypes":      pq.Array(mtga.SubnetTypes),
		"isDefault":        mtga.IsDefault,
		"id":               id,
	})
	return err
}

func (m *SQLModelsManager) DeleteManagedTransitGatewayAttachment(id uint64) error {
	q := "DELETE FROM managed_transit_gateway_attachment WHERE id=:id"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"id": id,
	})
	return err
}

func (m *SQLModelsManager) GetSecurityGroupSets() ([]*SecurityGroupSet, error) {
	q := `
	SELECT
		set.id,
		set.name,
		set.is_default,
		set.region,
		set.is_gov_cloud,
		grp.id,
		COALESCE(grp.name, ''),
		COALESCE(grp.description, ''),
		rule.id,
		COALESCE(rule.description, ''),
		COALESCE(rule.is_egress, false),
		COALESCE(rule.protocol, ''),
		COALESCE(rule.from_port, 0),
		COALESCE(rule.to_port, 0),
		COALESCE(rule.source, ''),
		(SELECT
			array_agg(CONCAT(vpc.aws_region, '/', aws_account.aws_id, '/', vpc.aws_id, ' - ', vpc.name))
		 FROM vpc
		 INNER JOIN configured_security_group_set
			ON vpc_id=vpc.id
		 INNER JOIN aws_account
			 ON aws_account.id=vpc.aws_account_id
		 WHERE security_group_set_id=set.id
		)
	FROM
		security_group_set set
	LEFT JOIN
		security_group grp
		ON grp.security_group_set_id=set.id
	LEFT JOIN
		security_group_rule rule
		ON rule.security_group_id = grp.id
	ORDER BY set.id, grp.id, rule.description
	`
	var currSet *SecurityGroupSet
	var currGroup *SecurityGroupTemplate
	sets := []*SecurityGroupSet{}
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		set := &SecurityGroupSet{}
		group := &SecurityGroupTemplate{}
		rule := &SecurityGroupRule{}
		var groupID, ruleID *uint64
		err := rows.Scan(
			&set.ID,
			&set.Name,
			&set.IsDefault,
			&set.Region,
			&set.IsGovCloud,
			&groupID,
			&group.Name,
			&group.Description,
			&ruleID,
			&rule.Description,
			&rule.IsEgress,
			&rule.Protocol,
			&rule.FromPort,
			&rule.ToPort,
			&rule.Source,
			pq.Array(&set.InUseVPCs),
		)
		if err != nil {
			return nil, err
		}
		if currSet == nil || currSet.ID != set.ID {
			sets = append(sets, set)
			currSet = set
		}
		if groupID != nil {
			group.ID = *groupID
			if currGroup == nil || currGroup.ID != group.ID {
				currSet.Groups = append(currSet.Groups, group)
				currGroup = group
			}
		} else {
			currGroup = nil
		}
		if ruleID != nil {
			currGroup.Rules = append(currGroup.Rules, rule)
		}
	}
	return sets, nil
}

func insertOrUpdateGroupsAndRules(tx *sqlx.Tx, set *SecurityGroupSet, allowUpdate bool) error {
	for _, group := range set.Groups {
		if allowUpdate && group.ID > 0 {
			q := `DELETE FROM
					security_group_rule
				WHERE security_group_id = $1`
			_, err := tx.Exec(q, group.ID)
			if err != nil {
				return err
			}
			q = `UPDATE security_group SET name=:name, description=:description WHERE id=:id`
			_, err = tx.NamedExec(q, map[string]interface{}{
				"name":        group.Name,
				"description": group.Description,
				"id":          group.ID,
			})
			if err != nil {
				return err
			}
		} else {
			q := `
			INSERT INTO security_group
				(name, description, security_group_set_id)
			VALUES
				(:name, :description, :setID)
			RETURNING id`
			rewritten, args, err := tx.BindNamed(q, map[string]interface{}{
				"name":        group.Name,
				"description": group.Description,
				"setID":       set.ID,
			})
			if err != nil {
				return err
			}
			err = tx.Get(&group.ID, rewritten, args...)
			if err != nil {
				return err
			}
		}
		for _, rule := range group.Rules {
			q := `
			INSERT INTO security_group_rule
				(
					security_group_id,
					description,
					is_egress,
					protocol,
					from_port,
					to_port,
					source
				)
				VALUES (
					:groupID,
					:description,
					:isEgress,
					:protocol,
					:fromPort,
					:toPort,
					:source
				)
			`
			_, err := tx.NamedExec(q, map[string]interface{}{
				"groupID":     group.ID,
				"description": rule.Description,
				"isEgress":    rule.IsEgress,
				"protocol":    rule.Protocol,
				"fromPort":    rule.FromPort,
				"toPort":      rule.ToPort,
				"source":      rule.Source,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *SQLModelsManager) CreateSecurityGroupSet(set *SecurityGroupSet) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	q := `
		INSERT INTO security_group_set (name, is_default, region, is_gov_cloud) VALUES (:name, :isDefault, :region, :isGovCloud) RETURNING id;
	`
	rewritten, args, err := tx.BindNamed(q, map[string]interface{}{
		"name":       set.Name,
		"isDefault":  set.IsDefault,
		"region":     set.Region,
		"isGovCloud": set.IsGovCloud,
	})
	if err != nil {
		return err
	}
	err = tx.Get(&set.ID, rewritten, args...)
	if err != nil {
		return err
	}
	err = insertOrUpdateGroupsAndRules(tx, set, false)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) UpdateSecurityGroupSet(id uint64, set *SecurityGroupSet) error {
	if set.ID != 0 && set.ID != id {
		return fmt.Errorf("id arg does not match set ID")
	}
	set.ID = id
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	_, err = tx.Exec("SELECT id FROM security_group_set WHERE id=$1 FOR UPDATE", id)
	if err != nil {
		return err
	}
	q := "UPDATE security_group_set SET name=:name, is_default=:isDefault, region=:region, is_gov_cloud=:isGovCloud WHERE id=:id"
	_, err = tx.NamedExec(q, map[string]interface{}{
		"id":         id,
		"name":       set.Name,
		"isDefault":  set.IsDefault,
		"region":     set.Region,
		"isGovCloud": set.IsGovCloud,
	})
	if err != nil {
		return err
	}
	keepGroupIDs := []uint64{}
	for _, group := range set.Groups {
		if group.ID > 0 {
			keepGroupIDs = append(keepGroupIDs, group.ID)
		}
	}
	if len(keepGroupIDs) == 0 {
		err = m.deleteGroupsAndRulesWhere(tx, "security_group_set_id=$1", id)
	} else {
		where, args, whereErr := m.getInQuery(
			"security_group_set_id=:setID AND id NOT IN (:keepGroupIDs)",
			map[string]interface{}{
				"setID":        id,
				"keepGroupIDs": keepGroupIDs,
			})
		if whereErr != nil {
			return whereErr
		}
		err = m.deleteGroupsAndRulesWhere(tx, where, args...)
	}
	if err != nil {
		return err
	}
	err = insertOrUpdateGroupsAndRules(tx, set, true)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) getInQuery(q string, namedArgs map[string]interface{}) (string, []interface{}, error) {
	q, args, err := sqlx.Named(q, namedArgs)
	if err != nil {
		return "", nil, err
	}
	q, args, err = sqlx.In(q, args...)
	if err != nil {
		return "", nil, err
	}
	return m.DB.Rebind(q), args, nil
}

func (m *SQLModelsManager) deleteGroupsAndRules(tx *sqlx.Tx, groupIDs []uint64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	q, args, err := m.getInQuery(`
		DELETE FROM
			security_group_rule
		WHERE security_group_id IN (:groupIDs)`,
		map[string]interface{}{"groupIDs": groupIDs})
	if err != nil {
		panic(err)
	}
	_, err = tx.Exec(q, args...)
	if err != nil {
		return err
	}
	q, args, err = m.getInQuery(`
		DELETE FROM
			security_group
		WHERE id in (:groupIDs)`,
		map[string]interface{}{"groupIDs": groupIDs})
	if err != nil {
		return err
	}
	_, err = tx.Exec(q, args...)
	return err
}

func (m *SQLModelsManager) deleteGroupsAndRulesWhere(tx *sqlx.Tx, where string, args ...interface{}) error {
	q := `SELECT id FROM security_group WHERE ` + where
	rows, err := tx.Queryx(q, args...)
	if err != nil {
		return err
	}
	groupIDs := []uint64{}
	for rows.Next() {
		groupID := uint64(0)
		err := rows.Scan(&groupID)
		if err != nil {
			return err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return m.deleteGroupsAndRules(tx, groupIDs)
}

func updateResolverRules(tx *sqlx.Tx, mrr *ManagedResolverRuleSet) error {
	q := `DELETE FROM
			resolver_rule
		WHERE ruleset_id = $1`
	_, err := tx.Exec(q, mrr.ID)
	if err != nil {
		return err
	}
	for _, rule := range mrr.Rules {
		q := `
		INSERT INTO resolver_rule
			(
				ruleset_id,
				aws_id,
				description
			)
			VALUES (
				:rulesetID,
				:awsID,
				:description
			)
			RETURNING id
		`
		rewritten, args, err := tx.BindNamed(q, map[string]interface{}{
			"rulesetID":   mrr.ID,
			"awsID":       strings.TrimSpace(rule.AWSID),
			"description": rule.Description,
		})
		if err != nil {
			return err
		}
		err = tx.Get(&rule.ID, rewritten, args...)
		if err != nil {
			return fmt.Errorf("Inserting resolver rule fails: %w", err)
		}
	}
	return nil
}

func (m *SQLModelsManager) deleteResolverRulesWhere(tx *sqlx.Tx, where string, args ...interface{}) error {
	q := `DELETE FROM resolver_rule WHERE ` + where
	_, err := tx.Exec(q, args...)
	return err
}

func (m *SQLModelsManager) DeleteSecurityGroupSet(id uint64) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	err = m.deleteGroupsAndRulesWhere(tx, "security_group_set_id=$1", id)
	if err != nil {
		return err
	}
	q := `
		DELETE FROM
			security_group_set
		WHERE id=:id`
	_, err = tx.NamedExec(q, map[string]interface{}{
		"id": id,
	})
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return err
}

func (m *SQLModelsManager) GetManagedResolverRuleSets() ([]*ManagedResolverRuleSet, error) {
	q := `
		SELECT
			set.id, set.aws_region, set.name, set.aws_share_id, set.is_default, rule.id, COALESCE(rule.description, ''), COALESCE(rule.aws_id, ''), account.aws_id, account.is_gov_cloud,
			(SELECT
				array_agg(CONCAT(vpc.aws_region, '/', aws_account.aws_id, '/', vpc.aws_id, ' - ', vpc.name))
			 FROM vpc
			 INNER JOIN configured_managed_resolver_rule_set
				ON vpc_id=vpc.id
			 INNER JOIN aws_account
			 	ON aws_account.id=vpc.aws_account_id
			 WHERE managed_resolver_rule_set_id=set.id
			)
		FROM managed_resolver_rule_set set
		INNER JOIN 
			aws_account account
			ON account.id=set.aws_account_id
		LEFT JOIN
			resolver_rule rule
			ON rule.ruleset_id = set.id
		ORDER BY set.id`
	var currSet *ManagedResolverRuleSet
	sets := []*ManagedResolverRuleSet{}
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		set := &ManagedResolverRuleSet{}
		rule := &ResolverRule{}
		var ruleID *uint64
		err := rows.Scan(
			&set.ID,
			&set.Region,
			&set.Name,
			&set.ResourceShareID,
			&set.IsDefault,
			&ruleID,
			&rule.Description,
			&rule.AWSID,
			&set.AccountID,
			&set.IsGovCloud,
			pq.Array(&set.InUseVPCs))
		if err != nil {
			return nil, err
		}
		if currSet == nil || currSet.ID != set.ID {
			sets = append(sets, set)
			currSet = set
		}
		if ruleID != nil {
			rule.ID = *ruleID
			currSet.Rules = append(currSet.Rules, rule)
		}
	}
	return sets, nil
}

func (m *SQLModelsManager) CreateManagedResolverRuleSet(mrr *ManagedResolverRuleSet) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	q := "INSERT INTO managed_resolver_rule_set (name, aws_region, aws_share_id, is_default, aws_account_id, is_gov_cloud) SELECT :name, :region, :shareId, :isDefault, id, is_gov_cloud FROM aws_account WHERE aws_id=:awsAccountId RETURNING id"
	rewritten, args, err := tx.BindNamed(q, map[string]interface{}{
		"name":         mrr.Name,
		"isGovCloud":   mrr.IsGovCloud,
		"awsAccountId": mrr.AccountID,
		"shareId":      mrr.ResourceShareID,
		"region":       mrr.Region,
		"isDefault":    mrr.IsDefault,
	})
	if err != nil {
		return err
	}
	err = m.DB.Get(&mrr.ID, rewritten, args...)
	if err != nil {
		return err
	}
	err = updateResolverRules(tx, mrr)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) UpdateManagedResolverRuleSet(id uint64, mrr *ManagedResolverRuleSet) error {
	if mrr.ID != 0 && mrr.ID != id {
		return errors.New("Updating ID is not supported")
	}
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	q := "UPDATE managed_resolver_rule_set SET is_gov_cloud=:isGovCloud, is_default=:isDefault, aws_share_id=:shareId, name=:name WHERE id=:id"
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"isGovCloud": mrr.IsGovCloud,
		"name":       mrr.Name,
		"shareId":    mrr.ResourceShareID,
		"id":         id,
		"isDefault":  mrr.IsDefault,
	})
	if err != nil {
		return err
	}
	mrr.ID = id
	err = updateResolverRules(tx, mrr)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) DeleteManagedResolverRuleSet(id uint64) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	err = m.deleteResolverRulesWhere(tx, "ruleset_id=$1", id)
	if err != nil {
		return err
	}
	q := `
		DELETE FROM
			managed_resolver_rule_set
		WHERE id=:id`
	_, err = tx.NamedExec(q, map[string]interface{}{
		"id": id,
	})
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) GetManagedTransitGatewayAttachments() ([]*ManagedTransitGatewayAttachment, error) {
	q := `
		SELECT
			id, transit_gateway_id, region, is_gov_cloud, name, routes, subnet_types, is_default,
			(SELECT
				array_agg(CONCAT(vpc.aws_region, '/', aws_account.aws_id, '/', vpc.aws_id, ' - ', vpc.name))
			 FROM vpc
			 INNER JOIN configured_managed_transit_gateway_attachment
				ON vpc_id=vpc.id
			 INNER JOIN aws_account
			 	ON aws_account.id=vpc.aws_account_id
			 WHERE managed_transit_gateway_attachment_id=managed_transit_gateway_attachment.id
			)
		FROM managed_transit_gateway_attachment`
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	mas := []*ManagedTransitGatewayAttachment{}
	for rows.Next() {
		var ma ManagedTransitGatewayAttachment
		err := rows.Scan(
			&ma.ID,
			&ma.TransitGatewayID,
			&ma.Region,
			&ma.IsGovCloud,
			&ma.Name,
			pq.Array(&ma.Routes),
			pq.Array(&ma.SubnetTypes),
			&ma.IsDefault,
			pq.Array(&ma.InUseVPCs))
		if err != nil {
			return nil, err
		}
		mas = append(mas, &ma)
	}
	return mas, nil
}

func (w *sqlVPCWriter) MarkAsDeleted() error {
	q := "UPDATE vpc SET is_deleted=true WHERE aws_id=:vpcID AND aws_region=:region"
	_, err := w.mm.DB.NamedExec(q, map[string]interface{}{
		"vpcID":  w.vpcID,
		"region": w.region,
	})
	return err
}

// Selects each row for update and sets the DBID field in each regionAndID
// It's okay if the same row is included multiple times.
func (m *SQLModelsManager) lockVPCRows(tx *sqlx.Tx, vpcs regionAndIDList) error {
	sort.Sort(vpcs) // Must always lock in same order or else deadlock!
	// Do not need to dedupe because "a transaction never conflicts with itself"
	// so we can SELECT FOR UPDATE the same row multiple times in one tx.

	for _, regionAndID := range vpcs {
		err := tx.Get(&regionAndID.DBID, "SELECT id FROM vpc WHERE aws_id=$1 AND aws_region=$2 FOR UPDATE", regionAndID.ID, regionAndID.Region)
		if err != nil {
			return err
		}
	}

	return nil
}

type regionAndID struct {
	Region Region
	ID     string
	DBID   uint64
}
type regionAndIDList []*regionAndID

func (l regionAndIDList) Len() int      { return len(l) }
func (l regionAndIDList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l regionAndIDList) Less(i, j int) bool {
	if l[i].Region < l[j].Region {
		return true
	}
	return l[i].ID < l[j].ID
}

// Must call lockVPCRows first
func findDBID(l regionAndIDList, region Region, vpcID string) uint64 {
	for _, regionAndID := range l {
		if regionAndID.ID == vpcID && regionAndID.Region == region {
			return regionAndID.DBID
		}
	}
	// Panic because internal preconditions (call lockVPCRows first) violated
	panic("No dbid for " + vpcID)
}

func (w *sqlVPCWriter) UpdateState(state *VPCState) error {
	tx, err := w.mm.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	myRegionAndID := &regionAndID{ID: w.vpcID, Region: w.region}
	toLock := regionAndIDList{myRegionAndID}
	if state != nil {
		for _, pc := range state.PeeringConnections {
			toLock = append(toLock,
				&regionAndID{ID: pc.RequesterVPCID, Region: pc.RequesterRegion},
				&regionAndID{ID: pc.AccepterVPCID, Region: pc.AccepterRegion})
		}
	}
	err = w.mm.lockVPCRows(tx, toLock)
	if err != nil {
		return fmt.Errorf("Error getting database ids: %s", err)
	}
	dbID := myRegionAndID.DBID
	var q string
	if state == nil {
		q = "UPDATE vpc SET state=NULL WHERE id=:dbID"
		_, err = tx.NamedExec(q, map[string]interface{}{
			"dbID": dbID,
		})
		if err != nil {
			return err
		}
	} else {
		q = "UPDATE vpc SET state=:state WHERE id=:dbID"
		for id, rt := range state.RouteTables {
			if rt.RouteTableID != "" {
				if rt.RouteTableID != id {
					return fmt.Errorf("RouteTableID %s for RouteTableInfo doesn't match the RouteTables map key id %s", rt.RouteTableID, id)
				}
			}
		}
		data, err := json.Marshal(state)
		if err != nil {
			return err
		}
		_, err = tx.NamedExec(q, map[string]interface{}{
			"dbID":  dbID,
			"state": data,
		})
		if err != nil {
			return err
		}
	}
	managedIDs := []uint64{}
	if state != nil {
		for _, tga := range state.TransitGatewayAttachments {
			for _, managedID := range tga.ManagedTransitGatewayAttachmentIDs {
				q := "INSERT INTO created_managed_transit_gateway_attachment (vpc_id, managed_transit_gateway_attachment_id, transit_gateway_attachment_id) VALUES (:dbID, :managedID, :attachmentID) ON CONFLICT(vpc_id, managed_transit_gateway_attachment_id) DO UPDATE SET transit_gateway_attachment_id=:attachmentID"
				_, err = tx.NamedExec(q, map[string]interface{}{
					"dbID":         dbID,
					"managedID":    managedID,
					"attachmentID": tga.TransitGatewayAttachmentID,
				})
				if err != nil {
					return err
				}
				managedIDs = append(managedIDs, managedID)
			}
		}
	}
	if len(managedIDs) == 0 {
		_, err = tx.Exec("DELETE FROM created_managed_transit_gateway_attachment WHERE vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM created_managed_transit_gateway_attachment WHERE vpc_id = :dbID AND managed_transit_gateway_attachment_id NOT IN (:managedIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":       dbID,
			"managedIDs": managedIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = w.mm.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	otherAccepterDBIDs := []uint64{}
	otherRequesterDBIDs := []uint64{}
	if state != nil {
		for _, pc := range state.PeeringConnections {
			if pc.RequesterVPCID == w.vpcID && pc.RequesterRegion == w.region {
				otherAccepterDBIDs = append(otherAccepterDBIDs, findDBID(toLock, pc.AccepterRegion, pc.AccepterVPCID))
			} else {
				if pc.AccepterVPCID != w.vpcID || pc.AccepterRegion != w.region {
					return fmt.Errorf("State includes a peering connection where neither VPC matches the current one")
				}
				otherRequesterDBIDs = append(otherRequesterDBIDs, findDBID(toLock, pc.RequesterRegion, pc.RequesterVPCID))
			}
		}
	}
	if len(otherRequesterDBIDs) == 0 {
		_, err = tx.Exec("DELETE FROM created_peering_connection WHERE accepter_vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM created_peering_connection WHERE accepter_vpc_id = :dbID AND requester_vpc_id NOT IN (:otherRequesterIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":              dbID,
			"otherRequesterIDs": otherRequesterDBIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = w.mm.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}
	if len(otherAccepterDBIDs) == 0 {
		_, err = tx.Exec("DELETE FROM created_peering_connection WHERE requester_vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM created_peering_connection WHERE requester_vpc_id = :dbID AND accepter_vpc_id NOT IN (:otherAccepterIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":             dbID,
			"otherAccepterIDs": otherAccepterDBIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = w.mm.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	if state != nil {
		for _, pc := range state.PeeringConnections {
			q := `
			INSERT INTO created_peering_connection
				(requester_vpc_id, accepter_vpc_id, peering_connection_id, is_accepted)
			VALUES
				(:requesterDBID, :accepterDBID, :peeringConnectionID, :isAccepted)
			ON CONFLICT(requester_vpc_id, accepter_vpc_id)
				DO UPDATE SET
					peering_connection_id=:peeringConnectionID,
					is_accepted=:isAccepted
			`
			_, err = tx.NamedExec(q, map[string]interface{}{
				"requesterDBID":       findDBID(toLock, pc.RequesterRegion, pc.RequesterVPCID),
				"accepterDBID":        findDBID(toLock, pc.AccepterRegion, pc.AccepterVPCID),
				"peeringConnectionID": pc.PeeringConnectionID,
				"isAccepted":          pc.IsAccepted,
			})
			if err != nil {
				return err
			}
		}
	}

	securityGroupTemplateIDs := []uint64{}
	if state != nil {
		for _, sg := range state.SecurityGroups {
			if sg.TemplateID == 0 {
				// Signals that it no longer corresponds to a template group
				continue
			}
			q := "INSERT INTO created_security_group (vpc_id, security_group_id, aws_id) VALUES (:dbID, :templateID, :awsID) ON CONFLICT(vpc_id, security_group_id) DO UPDATE SET aws_id=:awsID"
			_, err = tx.NamedExec(q, map[string]interface{}{
				"dbID":       dbID,
				"templateID": sg.TemplateID,
				"awsID":      sg.SecurityGroupID,
			})
			if err != nil {
				return err
			}
			securityGroupTemplateIDs = append(securityGroupTemplateIDs, sg.TemplateID)
		}
	}
	if len(securityGroupTemplateIDs) == 0 {
		_, err = tx.Exec("DELETE FROM created_security_group WHERE vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM created_security_group WHERE vpc_id = :dbID AND security_group_id NOT IN (:securityGroupTemplateIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":                     dbID,
			"securityGroupTemplateIDs": securityGroupTemplateIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = w.mm.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (w *sqlVPCWriter) UpdateName(name string) error {
	tx, err := w.mm.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	myRegionAndID := &regionAndID{ID: w.vpcID, Region: w.region}
	toLock := regionAndIDList{myRegionAndID}
	err = w.mm.lockVPCRows(tx, toLock)
	if err != nil {
		return fmt.Errorf("Error getting database ids: %s", err)
	}
	dbID := myRegionAndID.DBID
	q := "UPDATE vpc SET name=:name WHERE id=:dbID"
	_, err = tx.NamedExec(q, map[string]interface{}{
		"name": name,
		"dbID": dbID,
	})
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *SQLModelsManager) UpdateVPCConfig(region Region, vpcID string, config VPCConfig) error {
	tx, err := m.DB.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	myRegionAndID := &regionAndID{ID: vpcID, Region: region}
	toLock := regionAndIDList{myRegionAndID}
	for _, pc := range config.PeeringConnections {
		toLock = append(toLock,
			&regionAndID{ID: pc.OtherVPCID, Region: pc.OtherVPCRegion})
	}
	err = m.lockVPCRows(tx, toLock)
	if err != nil {
		return fmt.Errorf("Error getting database ids: %s", err)
	}
	dbID := myRegionAndID.DBID
	q := "UPDATE vpc SET config=:config WHERE id=:dbID"
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = tx.NamedExec(q, map[string]interface{}{
		"dbID":   dbID,
		"config": data,
	})
	if err != nil {
		return err
	}
	for _, managedID := range config.ManagedTransitGatewayAttachmentIDs {
		q := "INSERT INTO configured_managed_transit_gateway_attachment (vpc_id, managed_transit_gateway_attachment_id) VALUES (:dbID, :managedID) ON CONFLICT(vpc_id, managed_transit_gateway_attachment_id) DO NOTHING"
		_, err = tx.NamedExec(q, map[string]interface{}{
			"dbID":      dbID,
			"managedID": managedID,
		})
		if err != nil {
			return err
		}
	}
	if len(config.ManagedTransitGatewayAttachmentIDs) == 0 {
		_, err = tx.Exec("DELETE FROM configured_managed_transit_gateway_attachment WHERE vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM configured_managed_transit_gateway_attachment WHERE vpc_id = :dbID AND managed_transit_gateway_attachment_id NOT IN (:managedIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":       dbID,
			"managedIDs": config.ManagedTransitGatewayAttachmentIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = m.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	for _, managedID := range config.ManagedResolverRuleSetIDs {
		q := "INSERT INTO configured_managed_resolver_rule_set (vpc_id, managed_resolver_rule_set_id) VALUES (:dbID, :managedID) ON CONFLICT(vpc_id, managed_resolver_rule_set_id) DO NOTHING"
		_, err = tx.NamedExec(q, map[string]interface{}{
			"dbID":      dbID,
			"managedID": managedID,
		})
		if err != nil {
			return err
		}
	}
	if len(config.ManagedResolverRuleSetIDs) == 0 {
		_, err = tx.Exec("DELETE FROM configured_managed_resolver_rule_set WHERE vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM configured_managed_resolver_rule_set WHERE vpc_id = :dbID AND managed_resolver_rule_set_id NOT IN (:managedIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":       dbID,
			"managedIDs": config.ManagedResolverRuleSetIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = m.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	otherAccepterDBIDs := []uint64{}
	otherRequesterDBIDs := []uint64{}
	for _, pc := range config.PeeringConnections {
		if pc.IsRequester {
			otherAccepterDBIDs = append(otherAccepterDBIDs, findDBID(toLock, pc.OtherVPCRegion, pc.OtherVPCID))
		} else {
			otherRequesterDBIDs = append(otherRequesterDBIDs, findDBID(toLock, pc.OtherVPCRegion, pc.OtherVPCID))
		}
	}
	if len(otherRequesterDBIDs) == 0 {
		_, err = tx.Exec("DELETE FROM configured_peering_connection WHERE accepter_vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM configured_peering_connection WHERE accepter_vpc_id = :dbID AND requester_vpc_id NOT IN (:otherRequesterDBIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":                dbID,
			"otherRequesterDBIDs": otherRequesterDBIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = m.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}
	if len(otherAccepterDBIDs) == 0 {
		_, err = tx.Exec("DELETE FROM configured_peering_connection WHERE requester_vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM configured_peering_connection WHERE requester_vpc_id = :dbID AND accepter_vpc_id NOT IN (:otherAccepterDBIDs)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":               dbID,
			"otherAccepterDBIDs": otherAccepterDBIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = m.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	for _, pc := range config.PeeringConnections {
		var q string
		if pc.IsRequester {
			q = `
			INSERT INTO configured_peering_connection
			(
				requester_vpc_id,
				accepter_vpc_id,
				requester_connect_private,
				accepter_connect_private,
				requester_connect_subnet_groups,
				accepter_connect_subnet_groups
			)
			VALUES
			(
				:dbID,
				:otherDBID,
				:connectPrivate,
				:otherVPCConnectPrivate,
				:connectSubnetGroups,
				:otherVPCConnectSubnetGroups
			)
			ON CONFLICT(LEAST(requester_vpc_id, accepter_vpc_id), GREATEST(requester_vpc_id, accepter_vpc_id))
				DO UPDATE SET
					requester_vpc_id=:dbID,
					accepter_vpc_id=:otherDBID,
					requester_connect_private=:connectPrivate,
					accepter_connect_private=:otherVPCConnectPrivate,
					requester_connect_subnet_groups=:connectSubnetGroups,
					accepter_connect_subnet_groups=:otherVPCConnectSubnetGroups
			`
		} else {
			q = `
			INSERT INTO configured_peering_connection
			(
				requester_vpc_id,
				accepter_vpc_id,
				requester_connect_private,
				accepter_connect_private,
				requester_connect_subnet_groups,
				accepter_connect_subnet_groups
			)
			VALUES
			(
				:otherDBID,
				:dbID,
				:otherVPCConnectPrivate,
				:connectPrivate,
				:otherVPCConnectSubnetGroups,
				:connectSubnetGroups
			)
			ON CONFLICT(LEAST(requester_vpc_id, accepter_vpc_id), GREATEST(requester_vpc_id, accepter_vpc_id))
				DO UPDATE SET
					requester_vpc_id=:otherDBID,
					accepter_vpc_id=:dbID,
					requester_connect_private=:otherVPCConnectPrivate,
					accepter_connect_private=:connectPrivate,
					requester_connect_subnet_groups=:otherVPCConnectSubnetGroups,
					accepter_connect_subnet_groups=:connectSubnetGroups
			`
		}
		_, err := tx.NamedExec(q, map[string]interface{}{
			"dbID":                        dbID,
			"otherDBID":                   findDBID(toLock, pc.OtherVPCRegion, pc.OtherVPCID),
			"connectPrivate":              pc.ConnectPrivate,
			"otherVPCConnectPrivate":      pc.OtherVPCConnectPrivate,
			"connectSubnetGroups":         pq.Array(pc.ConnectSubnetGroups),
			"otherVPCConnectSubnetGroups": pq.Array(pc.OtherVPCConnectSubnetGroups),
		})
		if err != nil {
			return err
		}
	}

	for _, sgsID := range config.SecurityGroupSetIDs {
		q := "INSERT INTO configured_security_group_set (vpc_id, security_group_set_id) VALUES (:dbID, :sgsID) ON CONFLICT(vpc_id, security_group_set_id) DO NOTHING"
		_, err = tx.NamedExec(q, map[string]interface{}{
			"dbID":  dbID,
			"sgsID": sgsID,
		})
		if err != nil {
			return err
		}
	}
	if len(config.SecurityGroupSetIDs) == 0 {
		_, err = tx.Exec("DELETE FROM configured_security_group_set WHERE vpc_id = $1", dbID)
	} else {
		q = "DELETE FROM configured_security_group_set WHERE vpc_id = :dbID AND security_group_set_id NOT IN (:sgsID)"
		var args []interface{}
		q, args, err = sqlx.Named(q, map[string]interface{}{
			"dbID":  dbID,
			"sgsID": config.SecurityGroupSetIDs,
		})
		if err != nil {
			return err
		}
		q, args, err = sqlx.In(q, args...)
		if err != nil {
			return err
		}
		q = m.DB.Rebind(q)
		_, err = tx.Exec(q, args...)
	}
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}
	committed = true
	return nil
}

func (w *sqlVPCWriter) UpdateIssues(issues []*Issue) error {
	q := "UPDATE vpc SET issues=:issues WHERE aws_id=:vpcID AND aws_region=:region"
	data, err := json.Marshal(issues)
	if err != nil {
		return err
	}
	_, err = w.mm.DB.NamedExec(q, map[string]interface{}{
		"vpcID":  w.vpcID,
		"region": w.region,
		"issues": data,
	})
	return err
}

func (m *SQLModelsManager) GetAllAWSAccounts() ([]*AWSAccount, error) {
	q := `SELECT aws_id, name, project_name, is_gov_cloud FROM aws_account WHERE NOT is_inactive`
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	awsAccounts := []*AWSAccount{}
	for rows.Next() {
		acct := &AWSAccount{}
		err := rows.Scan(&acct.ID, &acct.Name, &acct.ProjectName, &acct.IsGovCloud)
		if err != nil {
			return nil, err
		}
		awsAccounts = append(awsAccounts, acct)
	}
	return awsAccounts, nil
}

func (m *SQLModelsManager) MarkAWSAccountInactive(awsID string) error {
	q := "UPDATE aws_account SET is_inactive = true WHERE aws_id=:awsID"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"awsID": awsID,
	})
	return err
}

// TODO: determine the performance update of using INSERT ON CONFLICT DO UPDATE
// (because it creates a new row version every time)

func (m *SQLModelsManager) CreateOrUpdateAWSAccount(account *AWSAccount) (uint64, error) {
	var dbAccountID uint64
	q := "INSERT INTO aws_account (aws_id, name, project_name, is_gov_cloud, is_inactive) VALUES (:ID, :name, :projectName, :isGovCloud, false) ON CONFLICT(aws_id) DO UPDATE SET name=:name, project_name=:projectName, is_gov_cloud=:isGovCloud, is_inactive=false RETURNING id"
	rewritten, args, err := m.DB.BindNamed(q, map[string]interface{}{
		"ID":          account.ID,
		"name":        account.Name,
		"projectName": account.ProjectName,
		"isGovCloud":  account.IsGovCloud,
	})
	if err != nil {
		return 0, err
	}
	err = m.DB.Get(&dbAccountID, rewritten, args...)
	if err != nil {
		return 0, err
	}
	return dbAccountID, nil
}

// When the update-aws-accounts microservice runs a successful sync loop, it should finish by calling this method to record the last successful execution
func (m *SQLModelsManager) RecordUpdateAWSAccountsHeartbeat() error {
	q := "INSERT INTO micro_service_heartbeats (service_name, last_success) VALUES ('update-aws-accounts', NOW()) ON CONFLICT (service_name) DO UPDATE SET last_success = NOW()"
	_, err := m.DB.Exec(q)
	return err
}

func (m *SQLModelsManager) GetAWSAccountsLastSyncedinInterval(minutes int) (bool, error) {
	// Count will be 0 if the time stamp of the heartbeat is too old or 1 if the timestamp of the heartbeat is within the interval
	q := fmt.Sprintf("SELECT COUNT(*) FROM micro_service_heartbeats WHERE service_name = 'update-aws-accounts' AND last_success > NOW() - interval '%d minutes'", minutes)
	var heartBeatInRangeCount int
	err := m.DB.QueryRow(q).Scan(&heartBeatInRangeCount)
	if err != nil {
		return false, err
	}
	return heartBeatInRangeCount > 0, nil
}

func (m *SQLModelsManager) GetAccount(accountID string) (*AWSAccount, error) {
	account := &AWSAccount{
		ID: accountID,
	}
	q := "SELECT name, project_name, is_gov_cloud FROM aws_account WHERE aws_id=$1"
	err := m.DB.QueryRowx(q, accountID).Scan(&account.Name, &account.ProjectName, &account.IsGovCloud)
	if err != nil {
		return nil, err
	}
	return account, nil
}

func (m *SQLModelsManager) ListAutomatedVPCs() ([]*VPC, error) {
	q := `
	SELECT
		vpc.aws_id,
		aws_account.aws_id,
		vpc.name,
		vpc.stack,
		vpc.aws_region,
		vpc.state->>'VPCType' as vpc_type
	FROM vpc
	INNER JOIN aws_account ON vpc.aws_account_id=aws_account.id
	WHERE (vpc.state->>'VPCType')::integer != $1 AND NOT vpc.is_deleted
	ORDER BY vpc.name ASC`

	rows, err := m.DB.Queryx(q, VPCTypeException)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vpcs := []*VPC{}
	for rows.Next() {
		vpc := &VPC{
			State: &VPCState{},
		}
		err := rows.Scan(&vpc.ID, &vpc.AccountID, &vpc.Name, &vpc.Stack, &vpc.Region, &vpc.State.VPCType)
		if err != nil {
			return nil, err
		}
		vpcs = append(vpcs, vpc)
	}
	return vpcs, nil
}

func (m *SQLModelsManager) ListExceptionVPCs() ([]*VPC, error) {
	q := `
	SELECT
		vpc.aws_id,
		aws_account.aws_id,
		vpc.name,
		vpc.stack
	FROM vpc
	INNER JOIN aws_account ON vpc.aws_account_id=aws_account.id
	WHERE (vpc.state->>'VPCType')::integer = $1 AND NOT vpc.is_deleted`

	rows, err := m.DB.Queryx(q, VPCTypeException)
	if err != nil {
		return nil, err
	}
	vpcs := []*VPC{}
	for rows.Next() {
		vpc := &VPC{}
		err := rows.Scan(&vpc.ID, &vpc.AccountID, &vpc.Name, &vpc.Stack)
		if err != nil {
			return nil, err
		}
		vpcs = append(vpcs, vpc)
	}
	return vpcs, nil
}

func (m *SQLModelsManager) GetAutomatedVPCsForAccount(region Region, accountID string) ([]*VPC, error) {
	q := `
	SELECT
		vpc.aws_id,
		vpc.name,
		vpc.state
	FROM vpc
	INNER JOIN aws_account ON vpc.aws_account_id=aws_account.id
	WHERE (vpc.state->>'VPCType')::integer != $1 AND NOT vpc.is_deleted
	AND aws_account.aws_id = $2
	AND vpc.aws_region = $3;
`

	rows, err := m.DB.Queryx(q, VPCTypeException, accountID, region)
	if err != nil {
		return nil, err
	}
	vpcs := []*VPC{}
	for rows.Next() {
		vpc := &VPC{
			AccountID: accountID,
			Region:    region,
		}
		var state *[]byte
		err := rows.Scan(&vpc.ID, &vpc.Name, &state)
		if err != nil {
			return nil, err
		}
		if state != nil {
			err := json.Unmarshal(*state, &vpc.State)
			if err != nil {
				return nil, err
			}
		}

		vpcs = append(vpcs, vpc)
	}
	return vpcs, nil
}

func (m *SQLModelsManager) GetOperableVPC(lockSet LockSet, region Region, vpcID string) (*VPC, VPCWriter, error) {
	if !lockSet.HasLock(TargetVPC(vpcID)) {
		return nil, nil, fmt.Errorf("LockSet does not hold a lock for %s", vpcID)
	}
	writer := &sqlVPCWriter{
		mm:     m,
		region: region,
		vpcID:  vpcID,
	}
	vpc, err := m.GetVPC(writer.region, writer.vpcID)
	return vpc, writer, err
}

func (m *SQLModelsManager) GetVPC(region Region, vpcID string) (*VPC, error) {
	vpc := &VPC{
		ID:     vpcID,
		Region: region,
	}
	var state *[]byte
	var issues *[]byte
	var config *[]byte
	var vpcDBID uint64
	q := `
	SELECT
		vpc.id,
		aws_account.aws_id,
		vpc.name,
		vpc.stack,
		vpc.state,
		vpc.issues,
		vpc.config,
		vpc.configured_mid,
		vpc.created_mid,
		vpc.created_id
	FROM (
		SELECT
			vpc.id,
			vpc.aws_account_id,
			vpc.name,
			vpc.stack,
			vpc.state,
			vpc.issues,
			vpc.config,
			configured_tga.managed_transit_gateway_attachment_id AS configured_mid,
			created_tga.managed_transit_gateway_attachment_id AS created_mid,
			created_tga.transit_gateway_attachment_id AS created_id
		FROM vpc
			LEFT JOIN configured_managed_transit_gateway_attachment configured_tga
				ON configured_tga.vpc_id=vpc.id 
			LEFT JOIN created_managed_transit_gateway_attachment created_tga
				ON created_tga.vpc_id=vpc.id
				AND created_tga.managed_transit_gateway_attachment_id=configured_tga.managed_transit_gateway_attachment_id
		WHERE vpc.aws_id = $1 AND vpc.aws_region = $2
	UNION
		SELECT
			vpc.id,
			vpc.aws_account_id,
			vpc.name,
			vpc.stack,
			vpc.state,
			vpc.issues,
			vpc.config,
			configured_tga.managed_transit_gateway_attachment_id AS configured_mid,
			created_tga.managed_transit_gateway_attachment_id AS created_mid,
			created_tga.transit_gateway_attachment_id AS created_id
		FROM vpc
			LEFT JOIN created_managed_transit_gateway_attachment created_tga
				ON created_tga.vpc_id=vpc.id
			LEFT JOIN configured_managed_transit_gateway_attachment configured_tga
				ON configured_tga.vpc_id=vpc.id
				AND created_tga.managed_transit_gateway_attachment_id=configured_tga.managed_transit_gateway_attachment_id
		WHERE vpc.aws_id = $1 AND vpc.aws_region = $2
	) vpc
	INNER JOIN aws_account ON vpc.aws_account_id=aws_account.id`
	rows, err := m.DB.Queryx(q, vpcID, region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configuredManagedIDs := []uint64{}
	attachmentIDToManagedIDs := make(map[string][]uint64)
	count := 0

	for ; rows.Next(); count++ {
		var configuredManagedID, createdManagedID *uint64
		var transitGatewayAttachmentID *string
		err := rows.Scan(&vpcDBID, &vpc.AccountID, &vpc.Name, &vpc.Stack, &state, &issues, &config, &configuredManagedID, &createdManagedID, &transitGatewayAttachmentID)
		if err != nil {
			return nil, err
		}
		if configuredManagedID != nil {
			configuredManagedIDs = append(configuredManagedIDs, *configuredManagedID)
		}
		if createdManagedID != nil && transitGatewayAttachmentID != nil {
			attachmentIDToManagedIDs[*transitGatewayAttachmentID] = append(attachmentIDToManagedIDs[*transitGatewayAttachmentID], *createdManagedID)
		}
	}
	if count == 0 {
		return nil, ErrVPCNotFound
	}
	if state != nil {
		err := json.Unmarshal(*state, &vpc.State)
		if err != nil {
			return nil, err
		}
		for _, tg := range vpc.State.TransitGatewayAttachments {
			// Does the right thing if the map entry is missing
			tg.ManagedTransitGatewayAttachmentIDs = attachmentIDToManagedIDs[tg.TransitGatewayAttachmentID]
		}
		for id, rt := range vpc.State.RouteTables {
			rt.RouteTableID = id
		}
	}
	if issues != nil {
		err := json.Unmarshal(*issues, &vpc.Issues)
		if err != nil {
			return nil, err
		}
	}
	if config != nil {
		err := json.Unmarshal(*config, &vpc.Config)
		if err != nil {
			return nil, err
		}
		vpc.Config.ManagedTransitGatewayAttachmentIDs = configuredManagedIDs
	}
	q = `
	SELECT
		true AS is_requester,
		other_vpc.aws_id AS other_vpc_id,
		other_vpc.aws_region AS other_vpc_region,
		other_aws_account.aws_id AS other_aws_account_id,
		requester_connect_private AS connect_private,
		accepter_connect_private AS other_vpc_connect_private,
		requester_connect_subnet_groups AS connect_subnet_groups,
		accepter_connect_subnet_groups AS other_vpc_connect_subnet_groups
	FROM
		configured_peering_connection
		INNER JOIN vpc other_vpc ON other_vpc.id = accepter_vpc_id
		INNER JOIN aws_account other_aws_account ON other_aws_account.id = other_vpc.aws_account_id
	WHERE requester_vpc_id = ($1)
	UNION
	SELECT
		false AS is_requester,
		other_vpc.aws_id AS other_vpc_id,
		other_vpc.aws_region AS other_vpc_region,
		other_aws_account.aws_id AS other_aws_account_id,
		accepter_connect_private AS connect_private,
		requester_connect_private AS other_vpc_connect_private,
		accepter_connect_subnet_groups AS connect_subnet_groups,
		requester_connect_subnet_groups AS other_vpc_connect_subnet_groups
	FROM
		configured_peering_connection
		INNER JOIN vpc other_vpc ON other_vpc.id = requester_vpc_id
		INNER JOIN aws_account other_aws_account ON other_aws_account.id = other_vpc.aws_account_id
	WHERE accepter_vpc_id = ($1)
	`
	rows, err = m.DB.Queryx(q, vpcDBID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		pc := &PeeringConnectionConfig{}
		err := rows.Scan(
			&pc.IsRequester,
			&pc.OtherVPCID,
			&pc.OtherVPCRegion,
			&pc.OtherVPCAccountID,
			&pc.ConnectPrivate,
			&pc.OtherVPCConnectPrivate,
			pq.Array(&pc.ConnectSubnetGroups),
			pq.Array(&pc.OtherVPCConnectSubnetGroups),
		)
		if err != nil {
			return nil, err
		}
		vpc.Config.PeeringConnections = append(vpc.Config.PeeringConnections, pc)
	}
	q = `
	SELECT
		requester.aws_id,
		requester.aws_region,
		accepter.aws_id,
		accepter.aws_region,
		peering_connection_id,
		is_accepted
	FROM
		created_peering_connection
		INNER JOIN vpc requester ON requester.id=requester_vpc_id
		INNER JOIN vpc accepter ON accepter.id=accepter_vpc_id
	WHERE
		(requester.aws_id = $1 AND requester.aws_region = $2)
		OR (accepter.aws_id = $1 AND accepter.aws_region = $2)
	`
	rows, err = m.DB.Queryx(q, vpcID, region)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		pc := &PeeringConnection{}
		err := rows.Scan(
			&pc.RequesterVPCID,
			&pc.RequesterRegion,
			&pc.AccepterVPCID,
			&pc.AccepterRegion,
			&pc.PeeringConnectionID,
			&pc.IsAccepted,
		)
		if err != nil {
			return nil, err
		}
		vpc.State.PeeringConnections = append(vpc.State.PeeringConnections, pc)
	}

	q = `SELECT managed_resolver_rule_set_id FROM configured_managed_resolver_rule_set WHERE vpc_id = $1`
	rows, err = m.DB.Queryx(q, vpcDBID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		managedResolverRuleSetID := uint64(0)
		err := rows.Scan(&managedResolverRuleSetID)
		if err != nil {
			return nil, err
		}
		vpc.Config.ManagedResolverRuleSetIDs = append(vpc.Config.ManagedResolverRuleSetIDs, managedResolverRuleSetID)
	}

	q = `SELECT security_group_set_id FROM configured_security_group_set WHERE vpc_id = $1`
	rows, err = m.DB.Queryx(q, vpcDBID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		sgsID := uint64(0)
		err := rows.Scan(&sgsID)
		if err != nil {
			return nil, err
		}
		vpc.Config.SecurityGroupSetIDs = append(vpc.Config.SecurityGroupSetIDs, sgsID)
	}
	q = `SELECT security_group_id, aws_id FROM created_security_group WHERE vpc_id = $1`
	rows, err = m.DB.Queryx(q, vpcDBID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		sgID := uint64(0)
		awsID := ""
		err := rows.Scan(&sgID, &awsID)
		if err != nil {
			return nil, err
		}
		for _, group := range vpc.State.SecurityGroups {
			if group.SecurityGroupID == awsID {
				group.TemplateID = sgID
				break
			}
		}
	}

	// We don't expect CIDRs with a null vpc.State, but need to guard against nil pointer dereferences below
	if vpc.State != nil {
		q = `SELECT vpc_id, cidr, is_primary FROM vpc_cidr WHERE vpc_id = $1 ORDER BY cidr ASC`
		rows, err = m.DB.Queryx(q, vpcDBID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			vpcCIDR := &VPCCIDR{}
			err := rows.Scan(&vpcCIDR.VPCID, &vpcCIDR.CIDR, &vpcCIDR.IsPrimary)
			if err != nil {
				return nil, err
			}
		}
	}

	return vpc, nil
}

// An account for vpc.AccountID must exist *before* calling this.
func (m *SQLModelsManager) CreateOrUpdateVPC(vpc *VPC) (uint64, error) {
	var dbVPCID uint64
	q := `INSERT INTO vpc
			(aws_account_id, aws_region, aws_id, name, stack)
			VALUES ((SELECT id FROM aws_account WHERE aws_id = :accountID), :region, :id, :name, :stack)
		ON CONFLICT(aws_region, aws_id) DO UPDATE SET name=:name, stack=:stack RETURNING id;`
	rewritten, args, err := m.DB.BindNamed(q, map[string]interface{}{
		"id":        vpc.ID,
		"accountID": vpc.AccountID,
		"region":    vpc.Region,
		"name":      vpc.Name,
		"stack":     vpc.Stack,
	})
	if err != nil {
		return 0, err
	}
	err = m.DB.Get(&dbVPCID, rewritten, args...)
	if err != nil {
		return 0, err
	}
	return dbVPCID, nil
}

func (m *SQLModelsManager) SetVPCRequestApprovedConfig(id uint64, approvedConfig *AllocateConfig) error {
	approvedJSON, err := json.Marshal(approvedConfig)
	if err != nil {
		return err
	}
	q := "UPDATE vpc_request SET approved_config=:config WHERE id=:id"
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"id":     id,
		"config": approvedJSON,
	})
	return err
}

func (m *SQLModelsManager) SetVPCRequestTaskID(id uint64, taskID uint64) error {
	q := "UPDATE vpc_request SET task_id=:taskID WHERE id=:id"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"id":     id,
		"taskID": taskID,
	})
	return err
}

func (m *SQLModelsManager) SetVPCRequestStatus(id uint64, status VPCRequestStatus) error {
	q := "UPDATE vpc_request SET status=:status WHERE id=:id"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"id":     id,
		"status": status,
	})
	return err
}

func (m *SQLModelsManager) SetVPCRequestProvisionedVPC(requestID uint64, region Region, vpcID string) error {
	q := "UPDATE vpc_request SET provisioned_vpc_id=(SELECT id FROM vpc WHERE aws_id=:vpcID AND aws_region=:region) WHERE id = :id"
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"id":     requestID,
		"region": region,
		"vpcID":  vpcID,
	})
	return err

}

func (m *SQLModelsManager) CreateVPCRequest(req *VPCRequest) error {
	if req.AccountID != req.RequestedConfig.AccountID {
		return fmt.Errorf("Inconsistent account IDs %q vs %q", req.AccountID, req.RequestedConfig.AccountID)
	}
	if req.ApprovedConfig != nil {
		return errors.New("ApprovedConfig must be null")
	}
	requestedJSON, err := json.Marshal(req.RequestedConfig)
	if err != nil {
		return err
	}
	q := `
		INSERT INTO vpc_request
			(aws_account_id,
			 requester_uid,
			 requester_name,
			 requester_email,
			 request_type,
			 related_issues,
			 comment,
			 ip_justification,
			 status,
			 requested_config,
			 task_id,
			 jira_issue)
		VALUES
			((SELECT id FROM aws_account WHERE aws_id=:accountID),
			 :requesterUID,
			 :requesterName,
			 :requesterEmail,
			 :requestType,
			 :relatedIssues,
			 :comment,
			 :ipJustification,
			 :status,
			 :requestedConfig,
			 :taskID,
			 :jiraIssue)`
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"accountID":       req.AccountID,
		"requesterUID":    req.RequesterUID,
		"requesterName":   req.RequesterName,
		"requesterEmail":  req.RequesterEmail,
		"requestType":     req.RequestType,
		"relatedIssues":   pq.Array(req.RelatedIssues),
		"comment":         req.Comment,
		"ipJustification": req.IPJustification,
		"status":          req.Status,
		"requestedConfig": requestedJSON,
		"taskID":          req.TaskID,
		"jiraIssue":       req.JIRAIssue,
	})
	return err
}

func vpcRequestSelect(where string, forUpdate bool) string {
	sql := `
	WITH RECURSIVE descendant_task(id, description, status, root_id) AS (
		SELECT task.id, task.description, task.status, task.depends_on_task_id
			FROM task
			INNER JOIN vpc_request ON vpc_request.task_id=task.depends_on_task_id
		UNION ALL
		SELECT further_descendant.id, further_descendant.description, further_descendant.status, descendant_task.root_id
			FROM task further_descendant
			INNER JOIN descendant_task ON descendant_task.id=further_descendant.depends_on_task_id
	)
	SELECT
		vpc_request.id,
		vpc_request.added_at,
		vpc_request.requester_uid,
		vpc_request.requester_name,
		vpc_request.requester_email,
		vpc_request.request_type,
		vpc_request.related_issues,
		vpc_request.comment,
		vpc_request.ip_justification,
		vpc_request.status,
		vpc_request.requested_config,
		vpc_request.approved_config,
		vpc_request.task_id,
		vpc_request.jira_issue,
		provisioned_vpc.aws_id,
		provisioned_vpc.aws_region,
		task.status AS task_status,
		aws_account.aws_id AS account_id,
		aws_account.name AS account_name,
		aws_account.project_name AS project_name,
		descendant_task.id AS descendant_task_id,
		descendant_task.description AS descendant_task_description,
		descendant_task.status AS descendant_task_status,
        CASE
           WHEN (vpc_request.jira_issue IS NOT NULL) THEN false
           ELSE EXISTS(SELECT 1 FROM vpc_request_log WHERE vpc_request_id=vpc_request.id)
        END AS jira_errors
	FROM vpc_request
	INNER JOIN aws_account ON aws_account.id = vpc_request.aws_account_id
	LEFT JOIN vpc provisioned_vpc ON provisioned_vpc.id=vpc_request.provisioned_vpc_id
	LEFT JOIN task ON task.id=task_id
	LEFT JOIN descendant_task ON descendant_task.root_id=task.id
	` + where + ` ORDER BY vpc_request.added_at DESC, task.id DESC, descendant_task.id ASC`
	if forUpdate {
		sql += " FOR UPDATE OF vpc_request"
	}
	return sql
}

func getVPCRequests(rows *sqlx.Rows) ([]*VPCRequest, error) {
	reqs := []*VPCRequest{}
	for rows.Next() {
		req := &VPCRequest{}
		var requestedBytes, approvedBytes *[]byte
		var dependentTaskInfo struct {
			Description *string
			ID          *uint64
			Status      *TaskStatus
		}
		var provisionedVPCRegion *Region
		var provisionedVPCID *string
		err := rows.Scan(
			&req.ID,
			&req.AddedAt,
			&req.RequesterUID,
			&req.RequesterName,
			&req.RequesterEmail,
			&req.RequestType,
			pq.Array(&req.RelatedIssues),
			&req.Comment,
			&req.IPJustification,
			&req.Status,
			&requestedBytes,
			&approvedBytes,
			&req.TaskID,
			&req.JIRAIssue,
			&provisionedVPCID,
			&provisionedVPCRegion,
			&req.TaskStatus,
			&req.AccountID,
			&req.AccountName,
			&req.ProjectName,
			&dependentTaskInfo.ID,
			&dependentTaskInfo.Description,
			&dependentTaskInfo.Status,
			&req.HasJIRAErrors)
		if err != nil {
			return nil, err
		}
		if len(reqs) == 0 || req.ID != reqs[len(reqs)-1].ID {
			// New request
			err = json.Unmarshal(*requestedBytes, &req.RequestedConfig)
			if err != nil {
				return nil, fmt.Errorf("Error unmarshaling requested config: %s", err)
			}
			if approvedBytes != nil {
				err := json.Unmarshal(*approvedBytes, &req.ApprovedConfig)
				if err != nil {
					return nil, fmt.Errorf("Error unmarshaling approved config: %s", err)
				}
			}
			if provisionedVPCID != nil && provisionedVPCRegion != nil {
				req.ProvisionedVPC = &struct {
					ID     string
					Region Region
				}{
					ID:     *provisionedVPCID,
					Region: *provisionedVPCRegion,
				}
			}
			reqs = append(reqs, req)
		} else {
			// Another dependent task for the existing task
			req = reqs[len(reqs)-1]
		}
		if dependentTaskInfo.ID != nil {
			req.DependentTasks = append(req.DependentTasks, struct {
				Description string
				ID          uint64
				Status      TaskStatus
			}{
				Description: *dependentTaskInfo.Description,
				ID:          *dependentTaskInfo.ID,
				Status:      *dependentTaskInfo.Status,
			})
		}
	}
	return reqs, nil
}

func (m *SQLModelsManager) GetVPCRequest(id uint64) (*VPCRequest, error) {
	q := vpcRequestSelect("WHERE vpc_request.id = :id", false)
	rows, err := m.DB.NamedQuery(q, map[string]interface{}{
		"id": id,
	})
	if err != nil {
		return nil, err
	}
	results, err := getVPCRequests(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("No VPC request for id %d", id)
	}
	if len(results) > 1 {
		return nil, fmt.Errorf("Multiple VPC requests for id %d", id)
	}
	return results[0], nil
}

func (m *SQLModelsManager) LockVPCRequest(id uint64) (*VPCRequest, *VPCRequestTransaction, error) {
	q := vpcRequestSelect("WHERE vpc_request.id = :id", true)
	tx, err := m.DB.Beginx()
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.NamedQuery(q, map[string]interface{}{
		"id": id,
	})
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	results, err := getVPCRequests(rows)
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	if len(results) == 0 {
		tx.Rollback()
		return nil, nil, fmt.Errorf("No VPC request for id %d", id)
	}
	if len(results) > 1 {
		tx.Rollback()
		return nil, nil, fmt.Errorf("Multiple VPC requests for id %d", id)
	}
	return results[0], &VPCRequestTransaction{requestID: id, tx: tx}, nil
}

func (m *SQLModelsManager) GetVPCRequests(accountID string) ([]*VPCRequest, error) {
	q := vpcRequestSelect("WHERE aws_account.aws_id = :accountID", false)
	rows, err := m.DB.NamedQuery(q, map[string]interface{}{
		"accountID": accountID,
	})
	if err != nil {
		return nil, err
	}
	return getVPCRequests(rows)
}

func (m *SQLModelsManager) GetAllVPCRequests() ([]*VPCRequest, error) {
	q := vpcRequestSelect("", false)
	rows, err := m.DB.Queryx(q)
	if err != nil {
		return nil, err
	}
	return getVPCRequests(rows)
}

// vpcRequestLogSelect returns either the default select statement
// or with the additional criteria via the "where" string.
func vpcRequestLogSelect(where string) string {
	return `SELECT * FROM vpc_request_log ` + where + ` ORDER BY added_at ASC`
}

// parseVPCRequestLogRows parses multiple rows and returns a slice of results
func parseVPCRequestLogRows(rows *sqlx.Rows) ([]*VPCRequestLog, error) {
	logs := []*VPCRequestLog{}
	for rows.Next() {
		var log VPCRequestLog
		err := rows.Scan(
			&log.ID,
			&log.AddedAt,
			&log.Message,
			&log.RetryAttempts,
			&log.VPCRequestID)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}
	return logs, nil
}

func (m *SQLModelsManager) VPCRequestLogInsert(vpcRequestID uint64, msg string, args ...interface{}) error {
	message := fmt.Sprintf(msg, args...)
	q := `INSERT INTO vpc_request_log (vpc_request_id, message, retry_attempts)
		  VALUES (:vpcReqestID, :message, 0)
		  ON CONFLICT (md5(message), vpc_request_id)
		  DO UPDATE SET retry_attempts = vpc_request_log.retry_attempts + 1`
	_, err := m.DB.NamedExec(q, map[string]interface{}{
		"vpcReqestID": vpcRequestID,
		"message":     message,
	})
	return err
}

func (m *SQLModelsManager) GetVPCRequestLogs(vpcRequestID uint64) ([]*VPCRequestLog, error) {
	q := vpcRequestLogSelect("WHERE vpc_request_id = :vpcRequestID")
	rows, err := m.DB.NamedQuery(q, map[string]interface{}{
		"vpcRequestID": vpcRequestID,
	})
	if err != nil {
		return nil, err
	}
	return parseVPCRequestLogRows(rows)
}

func (m *SQLModelsManager) GetJIRAHealth() (*JIRAHealth, error) {
	health := &JIRAHealth{}
	q := `SELECT COUNT(vrl.id)
		  FROM vpc_request_log vrl
		  LEFT JOIN vpc_request vr
		  ON vrl.vpc_request_id = vr.id
		  WHERE vr.jira_issue is null`
	err := m.DB.Get(&health.NumErrors, q)
	if err != nil {
		return nil, err
	}
	if health.NumErrors > 0 {
		q := `SELECT vrl.added_at, vrl.retry_attempts
			  FROM vpc_request_log vrl
			  LEFT JOIN vpc_request vr
			  ON vrl.vpc_request_id = vr.id
			  WHERE vr.jira_issue is null
			  ORDER BY vrl.added_at ASC
			  LIMIT 1`
		row := m.DB.QueryRowx(q)
		err := row.Scan(&health.OldestAddedAt, &health.OldestNumRetries)
		if err != nil {
			return nil, err
		}
	}
	return health, nil
}

type Dashboard struct {
	VPCRequests      []*VPCRequest
	TotalAccounts    int
	TotalVPCs        int
	TotalVPCRequests int
}

func (m *SQLModelsManager) GetDashboard() (*Dashboard, error) {
	dashboard := &Dashboard{}

	q := vpcRequestSelect("", false)
	rows, err := m.DB.Queryx(q + " LIMIT 10")
	if err != nil {
		return nil, err
	}

	vpcRequests, err := getVPCRequests(rows)
	if err != nil {
		return nil, err
	}

	dashboard.VPCRequests = vpcRequests

	err = m.DB.QueryRow("SELECT COUNT(id) FROM aws_account").Scan(&dashboard.TotalAccounts)
	if err != nil {
		return nil, err
	}
	err = m.DB.QueryRow("SELECT COUNT(id) FROM vpc WHERE is_deleted IS FALSE").Scan(&dashboard.TotalVPCs)
	if err != nil {
		return nil, err
	}
	err = m.DB.QueryRow("SELECT COUNT(id) FROM vpc_request").Scan(&dashboard.TotalVPCRequests)
	if err != nil {
		return nil, err
	}

	return dashboard, nil
}

type IPUsage struct {
	LastUpdated time.Time
	Data        []*EnvironmentIPUsage
}

type EnvironmentIPUsage struct {
	Region                     string
	Environment                string
	Zone                       string
	CIDRs                      []*IPUsageCIDR
	IPTotal                    uint64
	IPFree                     uint64
	IPFreePercent              float64
	LargestFreeContiguousBlock string
}

type IPUsageCIDR struct {
	CIDR                       string
	IPTotal                    uint64
	IPFree                     uint64
	IPFreePercent              float64
	LargestFreeContiguousBlock string
}

func (m *SQLModelsManager) GetIPUsage() (*IPUsage, error) {
	var jsonString *string = nil
	ipUsage := &IPUsage{}

	err := m.DB.QueryRow("SELECT usage FROM ip_usage ORDER BY date DESC").Scan(&jsonString)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	decoder := json.NewDecoder(strings.NewReader(*jsonString))
	if err := decoder.Decode(&ipUsage); err != nil {
		return nil, fmt.Errorf("Failed to decode IPUsageData %s", err)
	}

	return ipUsage, nil
}

func (m *SQLModelsManager) UpdateIPUsage(input *IPUsage) error {
	usage, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("Failed to marshal EnvironmentIPUsage %s", err)
	}
	q := `
		INSERT into ip_usage(date, usage) 
		VALUES(:date, :usage)
		ON CONFLICT("date") DO UPDATE SET usage = EXCLUDED.usage
		`
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"date":  input.LastUpdated,
		"usage": usage,
	})

	if err != nil {
		return err
	}

	return nil
}

type VPCCIDR struct {
	VPCID     uint64
	CIDR      string
	IsPrimary bool
}

func (m *SQLModelsManager) GetVPCCIDRs(vpcID string, region Region) (*string, []string, error) {
	vpcDBID, err := m.GetVPCDBID(vpcID, region)
	if err != nil {
		return nil, nil, err
	}

	q := `SELECT vpc_id, cidr, is_primary FROM vpc_cidr WHERE vpc_id = :vpcDBID`
	rows, err := m.DB.NamedQuery(q, map[string]interface{}{
		"vpcDBID": vpcDBID,
	})
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var primary *string
	secondary := []string{}
	for rows.Next() {
		var cidr VPCCIDR
		err := rows.Scan(
			&cidr.VPCID,
			&cidr.CIDR,
			&cidr.IsPrimary)
		if err != nil {
			return nil, nil, err
		}
		if cidr.IsPrimary {
			primary = &cidr.CIDR
			continue
		}
		secondary = append(secondary, cidr.CIDR)
	}

	return primary, secondary, nil
}

func (m *SQLModelsManager) GetVPCDBID(vpcID string, region Region) (*uint64, error) {
	q := `SELECT id FROM vpc WHERE aws_id = $1 and aws_region = $2`
	var dbID *uint64

	err := m.DB.QueryRowx(q, vpcID, region).Scan(&dbID)
	if err != nil {
		return nil, err
	}

	return dbID, nil
}

func (m *SQLModelsManager) DeleteVPCCIDR(vpcID string, region Region, cidr string) error {
	dbID, err := m.GetVPCDBID(vpcID, region)
	if err != nil {
		return err
	}

	q := "DELETE FROM vpc_cidr WHERE vpc_id = :dbID AND cidr = :cidr"
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"dbID": dbID,
		"cidr": cidr,
	})
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	return nil
}

func (m *SQLModelsManager) DeleteVPCCIDRs(vpcID string, region Region) error {
	dbID, err := m.GetVPCDBID(vpcID, region)
	if err != nil {
		return err
	}

	q := "DELETE FROM vpc_cidr WHERE vpc_id = :dbID"
	_, err = m.DB.NamedExec(q, map[string]interface{}{
		"dbID": dbID,
	})
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	return nil
}

func (m *SQLModelsManager) InsertVPCCIDR(vpcID string, region Region, cidr string, isPrimary bool) error {
	dbID, err := m.GetVPCDBID(vpcID, region)
	if err != nil {
		return err
	}

	q := "INSERT INTO vpc_cidr (vpc_id, cidr, is_primary) VALUES (:dbID, :cidr, :isPrimary) ON CONFLICT (vpc_id, cidr) DO NOTHING"
	args := map[string]interface{}{
		"dbID":      dbID,
		"cidr":      cidr,
		"isPrimary": isPrimary,
	}
	_, err = m.DB.NamedExec(q, args)
	if err != nil {
		return err
	}

	return nil
}

func (m *SQLModelsManager) GetDefaultVPCConfig(region Region) (*VPCConfig, error) {
	config := &VPCConfig{
		ConnectPublic:  true,
		ConnectPrivate: true,
	}

	allMTGAs, err := m.GetManagedTransitGatewayAttachments()
	if err != nil {
		return nil, fmt.Errorf("Error loading managed transit gateway attachments: %s", err)
	}
	for _, mtga := range allMTGAs {
		if mtga.Region == region && mtga.IsDefault {
			config.ManagedTransitGatewayAttachmentIDs = append(config.ManagedTransitGatewayAttachmentIDs, mtga.ID)
		}
	}

	allMRRSs, err := m.GetManagedResolverRuleSets()
	if err != nil {
		return nil, fmt.Errorf("Error loading managed resolver rule sets: %s", err)
	}
	for _, mrrs := range allMRRSs {
		if mrrs.Region == region && mrrs.IsDefault {
			config.ManagedResolverRuleSetIDs = append(config.ManagedResolverRuleSetIDs, mrrs.ID)
		}
	}

	allSGSs, err := m.GetSecurityGroupSets()
	if err != nil {
		return nil, fmt.Errorf("Error loading security group sets: %s", err)
	}
	for _, sgs := range allSGSs {
		if sgs.Region == region && sgs.IsDefault {
			config.SecurityGroupSetIDs = append(config.SecurityGroupSetIDs, sgs.ID)
		}
	}

	return config, nil
}
