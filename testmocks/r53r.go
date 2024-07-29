package testmocks

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/route53resolver"
	"github.com/aws/aws-sdk-go/service/route53resolver/route53resolveriface"
)

type MockR53R struct {
	route53resolveriface.Route53ResolverAPI
	AccountID string

	ResolverRuleAssociationStatus map[string]string            // resolver rule association id -> status
	ResolverRulesById             map[string]string            // resolver rule id -> description
	ResolverRuleAssociations      map[string]map[string]string // vpcID -> resolver rule -> association ID

	AssociationsAdded   map[string]map[string][]string // vpcID -> ruleID -> association IDs
	AssociationsDeleted map[string]map[string][]string // vpcID -> ruleID -> association IDs
	Errors              []error
}

func (m *MockR53R) newError(e error) error {
	m.Errors = append(m.Errors, e)
	return e
}

func (m *MockR53R) ListResolverRuleAssociations(input *route53resolver.ListResolverRuleAssociationsInput) (*route53resolver.ListResolverRuleAssociationsOutput, error) {
	vpcIDs := make([]string, 0)
	rrIDs := make([]string, 0)
	output := make([]*route53resolver.ResolverRuleAssociation, 0)

	for _, filter := range input.Filters {
		if aws.StringValue(filter.Name) == "VPCId" {
			vpcIDs = append(vpcIDs, aws.StringValueSlice(filter.Values)...)
		}
		if aws.StringValue(filter.Name) == "ResolverRuleId" {
			rrIDs = append(rrIDs, aws.StringValueSlice(filter.Values)...)
		}
	}
	if len(vpcIDs) == 0 {
		for vpc := range m.ResolverRuleAssociations {
			vpcIDs = append(vpcIDs, vpc)
		}
	}
	for _, _vpcID := range vpcIDs {
		vpcID := aws.String(_vpcID)
		if vpcAssociations, ok := m.ResolverRuleAssociations[_vpcID]; ok {
			for resolverRuleID, associationID := range vpcAssociations {
				if stringInSlice(resolverRuleID, rrIDs) {
					output = append(output, &route53resolver.ResolverRuleAssociation{
						Id:             aws.String(associationID),
						Name:           aws.String(fmt.Sprintf("%s-%s", _vpcID, resolverRuleID)),
						ResolverRuleId: aws.String(resolverRuleID),
						VPCId:          vpcID,
					})
				}
			}
		}
	}
	return &route53resolver.ListResolverRuleAssociationsOutput{
		ResolverRuleAssociations: output,
	}, nil
}

func (m *MockR53R) AssociateResolverRule(input *route53resolver.AssociateResolverRuleInput) (*route53resolver.AssociateResolverRuleOutput, error) {
	vpcID := aws.StringValue(input.VPCId)
	name := aws.StringValue(input.Name)
	rrID := aws.StringValue(input.ResolverRuleId)
	rraID := strings.Replace(rrID, "rslvr-rr-", "rslvr-rrassoc-", 1)
	if _, ok := m.ResolverRuleAssociations[vpcID]; !ok {
		m.ResolverRuleAssociations[vpcID] = make(map[string]string)
	}
	if _, ok := m.ResolverRulesById[rrID]; !ok {
		return nil, m.newError(fmt.Errorf("Invalid rule ID: %s", rrID))
	}
	if m.ResolverRuleAssociations[vpcID][rrID] == rraID {
		return nil, m.newError(fmt.Errorf("unable to add %s: %w", rrID, ErrTestDuplicate))
	}
	if m.AssociationsAdded == nil {
		m.AssociationsAdded = make(map[string]map[string][]string)
	}
	if m.AssociationsAdded[vpcID] == nil {
		m.AssociationsAdded[vpcID] = make(map[string][]string)
	}
	m.AssociationsAdded[vpcID][rrID] = append(m.AssociationsAdded[vpcID][rrID], rraID)
	m.ResolverRuleAssociations[vpcID][rrID] = rraID
	m.ResolverRuleAssociationStatus[rraID] = route53resolver.ResolverRuleAssociationStatusCreating
	return &route53resolver.AssociateResolverRuleOutput{
		ResolverRuleAssociation: &route53resolver.ResolverRuleAssociation{
			Id:             aws.String(rraID),
			Name:           aws.String(name),
			ResolverRuleId: aws.String(rrID),
			Status:         aws.String("CREATING"),
		},
	}, nil
}

func (m *MockR53R) GetResolverRule(input *route53resolver.GetResolverRuleInput) (*route53resolver.GetResolverRuleOutput, error) {
	id := aws.StringValue(input.ResolverRuleId)
	log.Printf("id = %s\n", id)
	for ruleID := range m.ResolverRulesById {
		log.Printf("GetResolverRule checking %s\n", ruleID)
		if ruleID == id {
			log.Printf("GetResolverRule returning %s\n", id)
			return &route53resolver.GetResolverRuleOutput{
				ResolverRule: &route53resolver.ResolverRule{
					Status: aws.String(route53resolver.ResolverRuleStatusComplete),
				},
			}, nil
		}
	}
	return nil, ErrTestResolverRuleNotFound
}

func (m *MockR53R) GetResolverRuleAssociation(input *route53resolver.GetResolverRuleAssociationInput) (*route53resolver.GetResolverRuleAssociationOutput, error) {
	id := aws.StringValue(input.ResolverRuleAssociationId)
	var vpcID, ruleID string
	for _vpcID := range m.ResolverRuleAssociations {
		for _ruleID, association := range m.ResolverRuleAssociations[_vpcID] {
			if association == id {
				vpcID = _vpcID
				ruleID = _ruleID
			}
		}
	}
	oldStatus := route53resolver.ErrCodeInternalServiceErrorException
	if vpcID != "" && ruleID != "" {
		oldStatus = m.ResolverRuleAssociationStatus[id]
		if oldStatus == route53resolver.ResolverRuleAssociationStatusDeleting {
			delete(m.ResolverRuleAssociationStatus, id)
		} else if oldStatus == route53resolver.ResolverRuleAssociationStatusCreating || oldStatus == route53resolver.ResolverRuleAssociationStatusComplete {
			m.ResolverRuleAssociationStatus[id] = route53resolver.ResolverRuleAssociationStatusComplete
		} else {
			m.ResolverRuleAssociationStatus[id] = route53resolver.ResolverRuleAssociationStatusFailed
		}
	} else {
		delete(m.ResolverRuleAssociationStatus, id)
	}
	if _, ok := m.ResolverRuleAssociationStatus[id]; !ok {
		return nil, awserr.New(route53resolver.ErrCodeResourceNotFoundException, "error getting association", nil)
	}
	log.Printf("%s -> %s", oldStatus, m.ResolverRuleAssociationStatus[id])
	return &route53resolver.GetResolverRuleAssociationOutput{
		ResolverRuleAssociation: &route53resolver.ResolverRuleAssociation{
			Id:             aws.String(id),
			Name:           aws.String(id),
			ResolverRuleId: aws.String(ruleID),
			Status:         aws.String(oldStatus),
			StatusMessage:  aws.String(oldStatus),
			VPCId:          aws.String(vpcID),
		},
	}, nil
}

func (m *MockR53R) DisassociateResolverRule(input *route53resolver.DisassociateResolverRuleInput) (*route53resolver.DisassociateResolverRuleOutput, error) {
	ruleID := aws.StringValue(input.ResolverRuleId)
	vpcID := aws.StringValue(input.VPCId)
	var associationID *string
	if association, ok := m.ResolverRuleAssociations[vpcID][ruleID]; ok {
		delete(m.ResolverRuleAssociations[vpcID], ruleID)
		associationID = &association
	} else {
		return nil, m.newError(fmt.Errorf("error disassociating rule: %w", ErrTestAssociationNotFound))
	}
	if m.AssociationsDeleted == nil {
		m.AssociationsDeleted = make(map[string]map[string][]string)
	}
	if m.AssociationsDeleted[vpcID] == nil {
		m.AssociationsDeleted[vpcID] = make(map[string][]string)
	}
	m.AssociationsDeleted[vpcID][ruleID] = append(m.AssociationsDeleted[vpcID][ruleID], *associationID)
	return &route53resolver.DisassociateResolverRuleOutput{
		ResolverRuleAssociation: &route53resolver.ResolverRuleAssociation{
			Id:             associationID,
			Name:           associationID,
			ResolverRuleId: input.ResolverRuleId,
			Status:         aws.String(route53resolver.ResolverRuleAssociationStatusDeleting),
			StatusMessage:  aws.String(route53resolver.ResolverRuleAssociationStatusDeleting),
			VPCId:          input.VPCId,
		},
	}, nil
}
