package testmocks

import (
	"errors"
	"fmt"
	"strings"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/aws/aws-sdk-go/service/route53resolver"
)

type MockResource struct {
	ID   string
	Type string
}

type MockRAM struct {
	ramiface.RAMAPI
	AccountID string
	Region    string

	ResourceSharePrincipals map[string][]string       // share ID -> account IDs
	ResourceShareResources  map[string][]MockResource // share ID -> resource ARN
	ResourceShareStatuses   map[string]string         // ARN -> share status
	OtherMocks              map[string]*MockRAM       // account ID -> mock

	SharesAdded                 []string
	PrincipalsAdded             map[string][]string // arn -> account IDs
	PrincipalsDeleted           map[string][]string // arn -> account IDs
	ResourcesAdded              map[string][]string // arn -> resource ARN
	ResourcesDeleted            map[string][]string // arn -> resource ARN
	InvitationsAcceptedShareARN []string

	TestRegion string
}

var (
	ErrTestDuplicate            = errors.New("duplicate resource")
	ErrTestAssociationNotFound  = awserr.New(route53resolver.ErrCodeResourceNotFoundException, "", nil)
	ErrTestPrincipalNotFound    = awserr.New(ram.ErrCodeUnknownResourceException, "", nil)
	ErrTestResolverRuleNotFound = awserr.New(route53resolver.ErrCodeResourceNotFoundException, "", nil)
	ErrTestMalformedARN         = awserr.New(ram.ErrCodeMalformedArnException, "", nil)
	ErrTestCrossBoundary        = awserr.New(ram.ErrCodeOperationNotPermittedException, "Cross boundary access forbidden", nil)
)

func guessResourceType(input string) string {
	parts, err := parseARN(input)
	if err != nil {
		fmt.Println(err)
		return awsp.ResourceTypeUndefined
	}
	if strings.HasPrefix(parts.Resource, "resolver-rule/") {
		return awsp.ResourceTypeResolverRule
	}
	if strings.HasPrefix(parts.Resource, "prefix-list/") {
		return awsp.ResourceTypePrefixList
	}
	return awsp.ResourceTypeUndefined
}

func parseARN(input string) (arn.ARN, error) {
	if !arn.IsARN(input) {
		return arn.ARN{}, ErrTestMalformedARN
	}
	parsed, err := arn.Parse(input)
	if err != nil {
		return arn.ARN{}, ErrTestMalformedARN
	}
	return parsed, nil
}

func extractRegionFromARN(input string) (*string, error) {
	parts, err := parseARN(input)
	if err != nil {
		return nil, err
	}
	return aws.String(parts.Region), nil
}

func arnPrefix(region string) string {
	if strings.HasPrefix(region, "us-gov") {
		return "arn:aws-us-gov"
	}
	return "arn:aws"
}

func resourceShareARN(region, accountID, resourceShareID string) string {
	resourceShareID = strings.TrimPrefix(resourceShareID, "rs-")
	return fmt.Sprintf("%s:ram:%s:%s:resource-share/%s", arnPrefix(region), region, accountID, resourceShareID)
}

func resolverRuleARN(region, accountID, resolverRuleID string) string {
	return fmt.Sprintf("%s:route53resolver:%s:%s:resolver-rule/%s", arnPrefix(region), region, accountID, resolverRuleID)
}

func (m *MockRAM) ListPrincipalsPages(input *ram.ListPrincipalsInput, fn func(*ram.ListPrincipalsOutput, bool) bool) error {
	out, err := m.ListPrincipals(input)
	if err != nil {
		return err
	}
	fn(out, true)
	return nil
}

func (m *MockRAM) ListPrincipals(input *ram.ListPrincipalsInput) (*ram.ListPrincipalsOutput, error) {
	desiredARN := *input.ResourceShareArns[0]
	shareRegion, err := extractRegionFromARN(desiredARN)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse ARN: %w", ErrTestMalformedARN)
	}
	if m.Region != *shareRegion {
		return nil, fmt.Errorf("Cross boundary error: %w", ErrTestCrossBoundary)
	}
	principals := []*ram.Principal{}
	for shareID, accountIDs := range m.ResourceSharePrincipals {
		arn := resourceShareARN(m.TestRegion, m.AccountID, shareID)
		if arn == desiredARN {
			for _, accountID := range accountIDs {
				principals = append(principals, &ram.Principal{Id: aws.String(accountID)})
			}
			break
		}
	}
	return &ram.ListPrincipalsOutput{
		Principals: principals,
	}, nil
}

func (m *MockRAM) GetResourceShares(input *ram.GetResourceSharesInput) (*ram.GetResourceSharesOutput, error) {
	desiredARN := *input.ResourceShareArns[0]
	shareRegion, err := extractRegionFromARN(desiredARN)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse ARN: %w", ErrTestMalformedARN)
	}
	if m.Region != *shareRegion {
		return nil, fmt.Errorf("Cross boundary error: %w", ErrTestCrossBoundary)
	}
	shares := []*ram.ResourceShare{}
	for arn, status := range m.ResourceShareStatuses {
		if arn == desiredARN {
			shares = append(shares, &ram.ResourceShare{ResourceShareArn: aws.String(arn), Status: aws.String(status)})
			break
		}
	}
	return &ram.GetResourceSharesOutput{
		ResourceShares: shares,
	}, nil
}

func (m *MockRAM) ListResources(input *ram.ListResourcesInput) (*ram.ListResourcesOutput, error) {
	resources := []*ram.Resource{}
	for id, resourceList := range m.ResourceShareResources {
		shareArn := resourceShareARN(m.TestRegion, m.AccountID, id)
		if len(input.ResourceShareArns) == 0 || stringInSlice(shareArn, aws.StringValueSlice(input.ResourceShareArns)) {
			for _, resource := range resourceList {
				resourceArn := resource.ID
				if !arn.IsARN(resourceArn) {
					switch resource.Type {
					case awsp.ResourceTypeResolverRule:
						resourceArn = resolverRuleARN(m.TestRegion, m.AccountID, resource.ID)
					}
				}
				resources = append(resources, &ram.Resource{
					Arn:              aws.String(resourceArn),
					Type:             aws.String(resource.Type),
					ResourceShareArn: aws.String(shareArn),
				})
			}
		}
	}
	return &ram.ListResourcesOutput{
		Resources: resources,
	}, nil
}

func (m *MockRAM) CreateResourceShare(input *ram.CreateResourceShareInput) (*ram.CreateResourceShareOutput, error) {
	output := &ram.CreateResourceShareOutput{}
	if !*input.AllowExternalPrincipals {
		return output, fmt.Errorf("Must allow external principals")
	}
	newShareARN := resourceShareARN(m.TestRegion, m.AccountID, *input.Name)
	output.ResourceShare = &ram.ResourceShare{
		ResourceShareArn: aws.String(newShareARN),
	}
	m.SharesAdded = append(m.SharesAdded, newShareARN)
	m.ResourceSharePrincipals[*input.Name] = []string{}
	mockResources := make([]MockResource, 0)
	for _, arn := range input.ResourceArns {
		mockResources = append(mockResources, MockResource{
			ID:   aws.StringValue(arn),
			Type: guessResourceType(aws.StringValue(arn)),
		})
	}
	m.ResourceShareResources[*input.Name] = mockResources
	m.ResourcesAdded[newShareARN] = aws.StringValueSlice(input.ResourceArns)
	return output, nil
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (m *MockRAM) setOtherMockStatuses(accountID, arn, status string) {
	if _, ok := m.OtherMocks[accountID]; !ok {
		return
	}
	if m.OtherMocks[accountID].ResourceShareStatuses == nil {
		m.OtherMocks[accountID].ResourceShareStatuses = make(map[string]string)
	}
	m.OtherMocks[accountID].ResourceShareStatuses[arn] = status
}

func (m *MockRAM) AssociateResourceShare(input *ram.AssociateResourceShareInput) (*ram.AssociateResourceShareOutput, error) {
	if m.PrincipalsAdded == nil {
		m.PrincipalsAdded = make(map[string][]string)
	}
	if m.ResourcesAdded == nil {
		m.ResourcesAdded = make(map[string][]string)
	}
	desiredARN := *input.ResourceShareArn
	if len(input.Principals) > 0 {
		for _, principal := range input.Principals {
			found := false
			for shareID, accountIDs := range m.ResourceSharePrincipals {
				arn := resourceShareARN(m.TestRegion, m.AccountID, shareID)
				if arn == desiredARN {
					if stringInSlice(aws.StringValue(principal), accountIDs) {
						return nil, fmt.Errorf("%s", ErrTestDuplicate)
					}
					m.ResourceSharePrincipals[shareID] = append(m.ResourceSharePrincipals[shareID], aws.StringValue(principal))
					m.ResourceShareStatuses[desiredARN] = ram.ResourceShareStatusActive
					m.PrincipalsAdded[desiredARN] = aws.StringValueSlice(input.Principals)
					m.setOtherMockStatuses(aws.StringValue(principal), desiredARN, ram.ResourceShareStatusActive)
					found = true
				}
			}
			if !found {
				prefix := fmt.Sprintf("arn:aws:ram:%s:%s:resource-share/", m.TestRegion, m.AccountID)
				shareID := strings.Replace(desiredARN, prefix, "rs-", 1)
				m.ResourceSharePrincipals[shareID] = []string{aws.StringValue(principal)}
				m.ResourceShareStatuses[desiredARN] = ram.ResourceShareStatusActive
				m.PrincipalsAdded[desiredARN] = aws.StringValueSlice(input.Principals)
				m.setOtherMockStatuses(aws.StringValue(principal), desiredARN, ram.ResourceShareStatusActive)
			}
		}
	}
	if len(input.ResourceArns) > 0 {
		for _, desiredResourceARN := range input.ResourceArns {
			found := false
			for shareID, mockResources := range m.ResourceShareResources {
				arn := resourceShareARN(m.TestRegion, m.AccountID, shareID)
				if arn == desiredARN {
					resourceARNFound := false
					for _, mockResource := range mockResources {
						resourceARN := mockResource.ID
						if strings.HasPrefix(mockResource.ID, "rslvr-rr-") {
							resourceARN = resolverRuleARN(m.TestRegion, m.AccountID, mockResource.ID)
						}
						if resourceARN == aws.StringValue(desiredResourceARN) {
							resourceARNFound = true
						}
					}
					if !resourceARNFound {
						m.ResourceShareResources[shareID] = append(m.ResourceShareResources[shareID], MockResource{
							ID:   aws.StringValue(desiredResourceARN),
							Type: guessResourceType(aws.StringValue(desiredResourceARN)),
						})
						m.ResourceShareStatuses[desiredARN] = ram.ResourceShareStatusActive
						m.ResourcesAdded[desiredARN] = append(m.ResourcesAdded[desiredARN], aws.StringValue(desiredResourceARN))
					}
					found = true
				}
			}
			if !found {
				prefix := fmt.Sprintf("arn:aws:ram:%s:%s:resource-share/", m.TestRegion, m.AccountID)
				shareID := strings.Replace(desiredARN, prefix, "rs-", 1)
				m.ResourceShareResources[shareID] = []MockResource{
					{
						ID:   aws.StringValue(desiredResourceARN),
						Type: guessResourceType(aws.StringValue(desiredResourceARN)),
					},
				}
				m.ResourceShareStatuses[desiredARN] = ram.ResourceShareStatusActive
				m.ResourcesAdded[desiredARN] = aws.StringValueSlice(input.ResourceArns)
			}
		}
	}
	return nil, nil
}

func (m *MockRAM) GetResourceShareInvitations(input *ram.GetResourceShareInvitationsInput) (*ram.GetResourceShareInvitationsOutput, error) {
	return &ram.GetResourceShareInvitationsOutput{
		ResourceShareInvitations: []*ram.ResourceShareInvitation{
			{
				ResourceShareInvitationArn: aws.String("bad-arn-1"),
				Status:                     aws.String("ACCEPTED"),
			},
			{
				ResourceShareInvitationArn: aws.String("invite-" + *input.ResourceShareArns[0]),
				Status:                     aws.String("PENDING"),
			},
			{
				ResourceShareInvitationArn: aws.String("bad-arn-2"),
				Status:                     aws.String("REJECTED"),
			},
		},
	}, nil
}

func (m *MockRAM) AcceptResourceShareInvitation(input *ram.AcceptResourceShareInvitationInput) (*ram.AcceptResourceShareInvitationOutput, error) {
	arn := *input.ResourceShareInvitationArn
	if !strings.HasPrefix(arn, "invite-") {
		return nil, fmt.Errorf("Invalid invitation ARN %q", arn)
	}
	m.InvitationsAcceptedShareARN = append(m.InvitationsAcceptedShareARN, arn[len("invite-"):])

	return nil, nil
}

func (m *MockRAM) DisassociateResourceShare(input *ram.DisassociateResourceShareInput) (*ram.DisassociateResourceShareOutput, error) {
	if m.PrincipalsDeleted == nil {
		m.PrincipalsDeleted = make(map[string][]string)
	}
	desiredARN := *input.ResourceShareArn
	shareFound := false
	for shareID, accountIDs := range m.ResourceSharePrincipals {
		arn := resourceShareARN(m.TestRegion, m.AccountID, shareID)
		if arn == desiredARN {
			shareFound = true
			principals := make([]string, 0)
			for _, accountID := range accountIDs {
				delete := false
				if stringInSlice(accountID, aws.StringValueSlice(input.Principals)) {
					delete = true
				}
				if !delete {
					principals = append(principals, accountID)
				} else {
					m.PrincipalsDeleted[arn] = append(m.PrincipalsDeleted[arn], accountID)
				}
			}
			m.ResourceSharePrincipals[shareID] = principals
		}
	}
	if !shareFound {
		return nil, fmt.Errorf("%w", ErrTestPrincipalNotFound)
	}
	return &ram.DisassociateResourceShareOutput{}, nil
}
