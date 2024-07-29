package main

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/google/go-cmp/cmp"
)

type mockResourceShare struct {
	Principals []string
	Resources  []testmocks.MockResource
	Status     string
	Region     *string
}

type resolverRuleAssociationTestCase struct {
	Name   string
	Region *string

	// In
	StartState                    *database.VPCState
	TaskData                      *database.UpdateResolverRulesTaskData
	AllRRSs                       []*database.ManagedResolverRuleSet
	UseAwsOrganizations           bool
	StartResourceShares           map[string]map[string]mockResourceShare // account ID -> share ID -> resource share info
	ResolverRuleAssociationStatus map[string]string                       // association id -> status
	ResolverRuleAssociations      map[string]map[string]string            // vpc id -> rule id -> association id

	// Expected calls
	SharesAdded                  map[string][]string                       // account ID -> shares created
	InvitesAccepted              map[string][]string                       // account ID -> arns
	PrincipalsAdded              map[string]map[string][]string            // account ID -> share id -> principals added
	PrincipalsDeleted            map[string]map[string][]string            // account ID -> share id -> principals deleted
	RulesAssociatedWithVPCs      map[string]map[string]map[string][]string // account ID -> vpc id -> rule id -> association ids
	RulesDisassociatedWithVPCs   map[string]map[string]map[string][]string // account ID -> vpc id -> rule id -> association ids
	RulesAssociatedWithShares    map[string]map[string][]string            // account ID -> vpc id -> rule id
	RulesDisassociatedWithShares map[string]map[string][]string            // account ID -> vpc id -> rule id
	ExpectedError                error
	ExpectedAWSErrors            []error

	// Out
	EndState *database.VPCState
}

func sumErrors(errs []error) string {
	errorSummary := ""
	for _, err := range errs {
		errorSummary += err.Error() + "\n"
	}
	return errorSummary
}

func TestHandleResolverRuleAssociations(t *testing.T) {
	vpcName := "test-vpc"
	vpcID := "vpc-123abc"
	targetAccountID := "123"
	masterAccountID := "987"
	masterGovAccountID := "987654321"
	var logger = &testLogger{}

	testCases := []*resolverRuleAssociationTestCase{
		{
			Name:       "PDNS Basic add with pre-existing resource share",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic commercial add, with govcloud ruleset",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      true,
					Name:            "gov-resolver-ruleset-test",
					Region:          testGovRegion,
					ResourceShareID: "rs-090807",
					AccountID:       masterGovAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic add without resource share",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "rs-010203",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			SharesAdded: map[string][]string{
				masterAccountID: {
					"rs-010203",
				},
			},
			InvitesAccepted: map[string][]string{
				targetAccountID: {
					"arn:aws:ram:us-west-2:987:resource-share/010203",
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {"rslvr-rr-010203040506"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic add without resource share, use aws orgs",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "rs-010203",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			SharesAdded: map[string][]string{
				masterAccountID: {
					"rs-010203",
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			UseAwsOrganizations: true,
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {},
				targetAccountID: {},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {"rslvr-rr-010203040506"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Add two rulesets, sharing single pre-existing share",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1, 2},
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test-1",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test-6",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{"345", "456"},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
							{
								ID:   "rslvr-rr-060504030201",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			InvitesAccepted: map[string][]string{
				targetAccountID: {
					"arn:aws:ram:us-west-2:987:resource-share/010203",
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic add to same account, pre-existing resource share",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "rs-010203",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       targetAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares:       map[string]map[string]mockResourceShare{},
			RulesAssociatedWithShares: map[string]map[string][]string{},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic add to same account, no resource share",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "rs-010203",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       targetAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{},
			SharesAdded: map[string][]string{
				targetAccountID: {
					"rs-010203",
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				targetAccountID: {
					"rs-010203": {"rslvr-rr-010203040506"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name:       "PDNS Basic add to same account, no resource share, use aws orgs",
			StartState: &database.VPCState{},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "rs-010203",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       targetAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{},
			UseAwsOrganizations: true,
			SharesAdded: map[string][]string{
				targetAccountID: {
					"rs-010203",
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				targetAccountID: {
					"rs-010203": {"rslvr-rr-010203040506"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Basic delete",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			PrincipalsDeleted: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Empty task, existing share",
			StartState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			PrincipalsDeleted: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
		},
		{
			Name: "PDNS Delete with stale state but other associations",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			ExpectedError:     ErrIncompleteResolverRuleProcessing,
			ExpectedAWSErrors: []error{testmocks.ErrTestAssociationNotFound},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-060504030201": "rslvr-rrassoc-060504030201",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
							{
								ID:   "rslvr-rr-060504030201",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
		},
		{
			Name: "PDNS Delete with stale state and no other associations",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			ExpectedError:     ErrIncompleteResolverRuleProcessing,
			ExpectedAWSErrors: []error{testmocks.ErrTestAssociationNotFound},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{},
				},
			},
			ResolverRuleAssociations:      map[string]map[string]string{},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
		},
		{
			Name: "PDNS Associate against pre-existing unmanaged",
			StartState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			ExpectedError:     ErrIncompleteResolverRuleProcessing,
			ExpectedAWSErrors: []error{testmocks.ErrTestDuplicate},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{},
		},
		{
			Name: "PDNS Associate against pre-existing unmanaged and new association",
			StartState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			ExpectedError:     ErrIncompleteResolverRuleProcessing,
			ExpectedAWSErrors: []error{testmocks.ErrTestDuplicate},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1, 2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test-new",
					Region:          testRegion,
					ResourceShareID: "rs-030201",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-02",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
					"rs-030201": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-060504030201",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
		},
		{
			Name: "PDNS Replace old association with new, same ruleset",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Replace old association with new, additional ruleset kept",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
					{
						ResolverRuleID:            "rslvr-rr-090807060504",
						ResolverRuleAssociationID: "rslvr-rrassoc-090807060504",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-090807060504",
						ResolverRuleAssociationID: "rslvr-rrassoc-090807060504",
					},
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1, 2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-030201",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-090807060504",
							Description: "resolver-rule-test-09",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
					"rs-030201": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-090807060504",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Ruleset ID change, use pre-existing resourceshare",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-030201",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {targetAccountID},
				},
			},
			InvitesAccepted: map[string][]string{
				targetAccountID: {
					"arn:aws:ram:us-west-2:987:resource-share/030201",
				},
			},
			PrincipalsDeleted: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
					"rs-030201": {
						Principals: []string{},
						Resources:  []testmocks.MockResource{},
						Status:     ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Ruleset ID change, new resourceshare",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "rs-030201",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			SharesAdded: map[string][]string{
				masterAccountID: {
					"rs-030201",
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {targetAccountID},
				},
			},
			InvitesAccepted: map[string][]string{
				targetAccountID: {
					"arn:aws:ram:us-west-2:987:resource-share/030201",
				},
			},
			PrincipalsDeleted: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Ruleset ID change, new resourceshare, use aws orgs",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "rs-030201",
					Region:          testRegion,
					ResourceShareID: "",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			SharesAdded: map[string][]string{
				masterAccountID: {
					"rs-030201",
				},
			},
			PrincipalsAdded: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {targetAccountID},
				},
			},
			PrincipalsDeleted: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {targetAccountID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			UseAwsOrganizations: true,
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-030201": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Ruleset ID change, re-use resourceshare",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesAssociatedWithShares: map[string]map[string][]string{
				masterAccountID: {
					"rs-010203": {"rslvr-rr-060504030201"},
				},
			},
			RulesAssociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
		{
			Name: "PDNS Ruleset ID change, same underlying rules",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{2},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
		},
		{
			Name: "PDNS Remove rule from existing ruleset",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
					"rslvr-rr-060504030201": "rslvr-rrassoc-060504030201",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
		},
		{
			Name: "PDNS Remove ruleset, keep ruleset, with same backing share",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
					{
						ResolverRuleID:            "rslvr-rr-060504030201",
						ResolverRuleAssociationID: "rslvr-rrassoc-060504030201",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{1},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
					"rslvr-rr-060504030201": "rslvr-rrassoc-060504030201",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs: []*database.ManagedResolverRuleSet{
				{
					ID:              1,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test-01",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          1,
							AWSID:       "rslvr-rr-010203040506",
							Description: "resolver-rule-test-01",
						},
					},
					InUseVPCs: []string{vpcID},
				},
				{
					ID:              2,
					IsGovCloud:      false,
					Name:            "resolver-ruleset-test-06",
					Region:          testRegion,
					ResourceShareID: "rs-010203",
					AccountID:       masterAccountID,
					Rules: []*database.ResolverRule{
						{
							ID:          2,
							AWSID:       "rslvr-rr-060504030201",
							Description: "resolver-rule-test-06",
						},
					},
					InUseVPCs: []string{vpcID},
				},
			},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-060504030201": {"rslvr-rrassoc-060504030201"},
					},
				},
			},
		},
		{
			Name: "PDNS Remove ruleset from database",
			StartState: &database.VPCState{
				ResolverRuleAssociations: []*database.ResolverRuleAssociation{
					{
						ResolverRuleID:            "rslvr-rr-010203040506",
						ResolverRuleAssociationID: "rslvr-rrassoc-010203040506",
					},
				},
			},
			EndState: &database.VPCState{
				ResolverRuleAssociations: nil,
			},
			TaskData: &database.UpdateResolverRulesTaskData{
				VPCID:     vpcID,
				AWSRegion: testRegion,
				ResolverRulesConfig: database.ResolverRulesConfig{
					ManagedResolverRuleSetIDs: []uint64{},
				},
			},
			ResolverRuleAssociations: map[string]map[string]string{
				vpcID: {
					"rslvr-rr-010203040506": "rslvr-rrassoc-010203040506",
				},
			},
			ResolverRuleAssociationStatus: map[string]string{},
			AllRRSs:                       []*database.ManagedResolverRuleSet{},
			StartResourceShares: map[string]map[string]mockResourceShare{
				masterAccountID: {
					"rs-010203": {
						Principals: []string{targetAccountID},
						Resources: []testmocks.MockResource{
							{
								ID:   "rslvr-rr-010203040506",
								Type: awsp.ResourceTypeResolverRule,
							},
						},
						Status: ram.ResourceShareStatusActive,
					},
				},
			},
			RulesDisassociatedWithVPCs: map[string]map[string]map[string][]string{
				targetAccountID: {
					vpcID: {
						"rslvr-rr-010203040506": {"rslvr-rrassoc-010203040506"},
					},
				},
			},
		},
	}

	// Speed time up 1000x
	speedUpTime()
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			log.Printf("\n---------- Running test case %q ----------", tc.Name)
			managedRuleSetsByID := make(map[uint64]*database.ManagedResolverRuleSet)
			resolverRulesById := make(map[string]string)
			resolverRuleAssociations := tc.ResolverRuleAssociations
			if resolverRuleAssociations == nil {
				resolverRuleAssociations = make(map[string]map[string]string)
			}
			resolverRuleAssociationStatus := tc.ResolverRuleAssociationStatus
			if resolverRuleAssociationStatus == nil {
				resolverRuleAssociationStatus = make(map[string]string)
			}
			for _, rs := range tc.AllRRSs {
				managedRuleSetsByID[rs.ID] = rs
				for _, rule := range rs.Rules {
					resolverRulesById[rule.AWSID] = rule.Description
				}
			}
			mm := &testmocks.MockModelsManager{
				ResolverRuleSets: managedRuleSetsByID,
				VPCs:             make(map[string]*database.VPC),
				TestRegion:       testRegion,
			}
			region := testRegion
			if tc.Region != nil {
				region = *tc.Region
			}
			ramsvc := &testmocks.MockRAM{
				Region:     region,
				TestRegion: testRegion,
			}
			r53rsvc := &testmocks.MockR53R{
				AccountID:                     targetAccountID,
				ResolverRuleAssociations:      resolverRuleAssociations,
				ResolverRuleAssociationStatus: resolverRuleAssociationStatus,
				ResolverRulesById:             resolverRulesById,
				AssociationsAdded:             map[string]map[string][]string{},
				AssociationsDeleted:           map[string]map[string][]string{},
				Errors:                        []error{},
			}
			vpc := &database.VPC{
				AccountID: targetAccountID,
				Name:      vpcName,
				ID:        vpcID,
				Region:    testRegion,
				State:     tc.StartState,
			}
			vpcWriter := &testmocks.MockVPCWriter{
				MM:    mm,
				VPCID: vpc.ID,
				Region: vpc.
					Region,
			}
			lockSet := database.GetFakeLockSet(database.TargetVPC(vpc.ID), database.TargetAddResourceShare)
			shareRAMs := make(map[string]*testmocks.MockRAM)
			shareR53Rs := make(map[string]*testmocks.MockR53R)
			accountIDs := make([]string, 0)
			accountIDs = append(accountIDs, targetAccountID)
			shareR53Rs[targetAccountID] = r53rsvc
			for _, rs := range managedRuleSetsByID {
				if !stringInSlice(rs.AccountID, accountIDs) {
					accountIDs = append(accountIDs, rs.AccountID)
				}
			}
			for accountID := range tc.StartResourceShares {
				if !stringInSlice(accountID, accountIDs) {
					accountIDs = append(accountIDs, accountID)
				}
			}
			for _, accountID := range accountIDs {
				resourceSharePrincipals := make(map[string][]string)
				resourceShareResources := make(map[string][]testmocks.MockResource)
				resourceShareStatuses := make(map[string]string)
				region := testRegion
				for shareID, share := range tc.StartResourceShares[accountID] {
					resourceSharePrincipals[shareID] = share.Principals
					resourceShareResources[shareID] = share.Resources
					resourceShareStatuses[resourceShareARN(region, accountID, shareID)] = share.Status
					if share.Region != nil {
						region = *share.Region
					}
				}
				ramsvc := &testmocks.MockRAM{
					AccountID:                   accountID,
					Region:                      region,
					SharesAdded:                 []string{},
					ResourceSharePrincipals:     resourceSharePrincipals,
					ResourceShareResources:      resourceShareResources,
					ResourceShareStatuses:       resourceShareStatuses,
					PrincipalsAdded:             map[string][]string{},
					PrincipalsDeleted:           map[string][]string{},
					ResourcesAdded:              map[string][]string{},
					ResourcesDeleted:            map[string][]string{},
					InvitationsAcceptedShareARN: []string{},
					TestRegion:                  testRegion,
					OtherMocks:                  map[string]*testmocks.MockRAM{},
				}
				shareRAMs[accountID] = ramsvc
			}
			if tc.UseAwsOrganizations {
				for id, share := range shareRAMs {
					t.Logf("Account %s", id)
					for _, accountID := range accountIDs {
						if id != accountID {
							share.OtherMocks[accountID] = shareRAMs[accountID]
						}
					}
				}
				ramsvc.OtherMocks = shareRAMs
			}
			ctx := &awsp.Context{
				AWSAccountAccess: &awsp.AWSAccountAccess{
					RAMsvc:  shareRAMs[targetAccountID],
					R53Rsvc: r53rsvc,
				},
				VPCName: vpcName,
				VPCID:   vpcID,
				Logger:  logger,
				Clock:   testClock,
			}
			var getAccountCredentials = func(accountID string) (ramiface.RAMAPI, error) {
				if _, ok := shareRAMs[accountID]; ok {
					return shareRAMs[accountID], nil
				}
				return nil, fmt.Errorf("Unexpected request for account %s credentials", accountID)
			}
			vpcWriter.UpdateState(tc.StartState)
			err := handleResolverRuleAssociations(
				lockSet,
				ctx,
				vpc,
				vpcWriter,
				tc.TaskData,
				mm,
				managedRuleSetsByID,
				getAccountCredentials)
			if err != nil {
				if tc.ExpectedError != nil {
					if errors.Is(err, tc.ExpectedError) {
						log.Printf("Got expected error: %s", err)
					}
				} else {
					t.Fatalf("Resolver Rule Test case %q failed to handle associations: %s", tc.Name, err)
				}
			} else if tc.ExpectedError != nil {
				t.Fatalf("Resolver Rule Test case %q expected an error, but succeeded: %s", tc.Name, tc.ExpectedError)
			}
			if len(r53rsvc.Errors) > 0 {
				if len(tc.ExpectedAWSErrors) > 0 {
					for _, expectedError := range tc.ExpectedAWSErrors {
						for _, e := range r53rsvc.Errors {
							if errors.Is(e, expectedError) {
								log.Printf("Got expected error: %s", e)
								break
							} else {
								t.Fatalf("Resolver Rule Test case %q failed to handle associations: %s", tc.Name, e)
							}
						}
					}
				} else {
					t.Fatalf("Resolver rule Test case %q failed to handle associations: %s", tc.Name, sumErrors(r53rsvc.Errors))
				}
			}
			for accountID, ram := range shareRAMs {
				expectedSharesAdded := make([]string, 0)
				if _, ok := tc.SharesAdded[accountID]; ok {
					for _, shareID := range tc.SharesAdded[accountID] {
						expectedSharesAdded = append(expectedSharesAdded, resourceShareARN(testRegion, accountID, shareID))
					}
				}
				if !reflect.DeepEqual(expectedSharesAdded, ram.SharesAdded) {
					t.Fatalf("R53RR Test case %q: Wrong shares added to account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedSharesAdded, ram.SharesAdded)
				}
				expectedPrincipalsAdded := make(map[string][]string)
				for shareID, principals := range tc.PrincipalsAdded[accountID] {
					expectedPrincipalsAdded[resourceShareARN(testRegion, accountID, shareID)] = principals
				}
				if !reflect.DeepEqual(expectedPrincipalsAdded, ram.PrincipalsAdded) {
					t.Fatalf("R53RR Test case %q: Wrong principals added to shares for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedPrincipalsAdded, ram.PrincipalsAdded)
				}
				expectedPrincipalsDeleted := make(map[string][]string)
				for shareID, principals := range tc.PrincipalsDeleted[accountID] {
					expectedPrincipalsDeleted[resourceShareARN(testRegion, accountID, shareID)] = principals
				}
				if !reflect.DeepEqual(expectedPrincipalsDeleted, ram.PrincipalsDeleted) {
					t.Fatalf("R53RR Test case %q: Wrong principals deleted from shares for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedPrincipalsDeleted, ram.PrincipalsDeleted)
				}
				expectedARNs := make([]string, 0)
				if v, ok := tc.InvitesAccepted[accountID]; ok {
					expectedARNs = v
				}
				sort.Strings(expectedARNs)
				sort.Strings(ram.InvitationsAcceptedShareARN)
				if !reflect.DeepEqual(expectedARNs, ram.InvitationsAcceptedShareARN) {
					t.Fatalf("R53RR Test case %q: Wrong invites accepted. Expected:\n%#v\nbut got:\n%#v", tc.Name, expectedARNs, ram.InvitationsAcceptedShareARN)
				}

				expectedRulesAssociatedWithShares := make(map[string][]string)
				for shareID, rules := range tc.RulesAssociatedWithShares[accountID] {
					for _, ruleID := range rules {
						expectedRulesAssociatedWithShares[resourceShareARN(testRegion, accountID, shareID)] = append(
							expectedRulesAssociatedWithShares[resourceShareARN(testRegion, accountID, shareID)],
							resolverRuleARN(testRegion, accountID, ruleID),
						)
					}
				}
				if !reflect.DeepEqual(expectedRulesAssociatedWithShares, ram.ResourcesAdded) {
					t.Fatalf("R53RR Test case %q: Wrong rules associated with shares for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedRulesAssociatedWithShares, ram.ResourcesAdded)
				}
				expectedRulesDisassociatedWithShares := make(map[string][]string)
				for shareID, rules := range tc.RulesDisassociatedWithShares[accountID] {
					expectedRulesDisassociatedWithShares[resourceShareARN(testRegion, accountID, shareID)] = rules
				}
				if !reflect.DeepEqual(expectedRulesDisassociatedWithShares, ram.ResourcesDeleted) {
					t.Fatalf("R53RR Test case %q: Wrong rules disassociated with shares for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedRulesDisassociatedWithShares, ram.ResourcesDeleted)
				}
			}

			for accountID, r53 := range shareR53Rs {
				expectedRulesAssociatedWithVPCs := make(map[string]map[string][]string)
				for vpcID, rules := range tc.RulesAssociatedWithVPCs[accountID] {
					expectedRulesAssociatedWithVPCs[vpcID] = rules
				}
				if !reflect.DeepEqual(expectedRulesAssociatedWithVPCs, r53.AssociationsAdded) {
					t.Fatalf("R53RR Test case %q: Wrong rules associated with vpcs for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedRulesAssociatedWithVPCs, r53.AssociationsAdded)
				}
				expectedRulesDisassociatedWithVPCs := make(map[string]map[string][]string)
				for vpcID, rules := range tc.RulesDisassociatedWithVPCs[accountID] {
					expectedRulesDisassociatedWithVPCs[vpcID] = rules
				}
				if !reflect.DeepEqual(expectedRulesDisassociatedWithVPCs, r53.AssociationsDeleted) {
					t.Fatalf("R53RR Test case %q: Wrong rules disassociated with vpcs for account %q. Expected:\n%#v\nbut got:\n%#v", tc.Name, accountID, expectedRulesDisassociatedWithVPCs, r53.AssociationsDeleted)
				}
			}
			endVPC, err := mm.GetVPC(testRegion, vpcID)
			if err != nil {
				t.Fatalf("R53RR Test case %q: Error getting end state: %s", tc.Name, err)
			}
			if diff := cmp.Diff(tc.EndState, endVPC.State); diff != "" {
				t.Fatalf("Expected end state did not match state in database: \n%s", diff)
			}
		})
	}
}
