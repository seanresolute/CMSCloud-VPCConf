package database

import (
	"reflect"
	"testing"
)

func TestBitmapIncludes(t *testing.T) {
	type testCase struct {
		name             string
		superset, subset interface{} // TaskTypes or VerifyTypes
		includes         bool
	}

	testCases := []*testCase{
		{
			name:     "includes self - single",
			superset: VerifyResolverRules,
			subset:   VerifyResolverRules,
			includes: true,
		},
		{
			name:     "includes self - multiple",
			superset: TaskTypeRepair | TaskTypeSecurityGroups | TaskTypeNetworking,
			subset:   TaskTypeNetworking | TaskTypeRepair | TaskTypeSecurityGroups,
			includes: true,
		},
		{
			name:     "self does not include other",
			superset: TaskTypeRepair,
			subset:   TaskTypeLogging,
			includes: false,
		},
		{
			name:     "self does not include superset - self is single",
			superset: VerifyResolverRules,
			subset:   VerifyResolverRules | VerifyNetworking,
			includes: false,
		},
		{
			name:     "self does not include superset - self is multiple",
			superset: VerifyResolverRules | VerifyCIDRs,
			subset:   VerifyResolverRules | VerifyCIDRs | VerifyNetworking,
			includes: false,
		},
		{
			name:     "disjoint sets do not include each other",
			superset: TaskTypeRepair | TaskTypeLogging,
			subset:   TaskTypeNetworking | TaskTypeSecurityGroups,
			includes: false,
		},
		{
			name:     "empty set includes empty set",
			superset: VerifyTypes(0),
			subset:   VerifyTypes(0),
			includes: true,
		},
		{
			name:     "anything includes empty set",
			superset: TaskTypeNetworking | TaskTypeLogging,
			subset:   TaskTypes(0),
			includes: true,
		},
		{
			name:     "empty set does not include something",
			superset: TaskTypes(0),
			subset:   TaskTypeLogging,
			includes: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sup := reflect.ValueOf(tc.superset).Uint()
			sub := reflect.ValueOf(tc.subset).Uint()
			actual := bitmapIncludes(sup, sub)
			if actual != tc.includes {
				t.Errorf("Expected %v but got %v", tc.includes, actual)
			}
		})
	}
}

func TestFollowUpTasks(t *testing.T) {
	type testCase struct {
		name          string
		spec          VerifySpec
		followUpTypes TaskTypes
	}
	testCases := []*testCase{
		{
			name: "No follow-up for CIDRs",
			spec: VerifySpec{VerifyCIDRs: true},
		},
		{
			name:          "Verify all",
			spec:          VerifyAllSpec(),
			followUpTypes: TaskTypeNetworking | TaskTypeLogging | TaskTypeResolverRules | TaskTypeSecurityGroups,
		},
		{
			name:          "Verify some",
			spec:          VerifySpec{VerifyNetworking: true, VerifyLogging: true, VerifyCIDRs: true},
			followUpTypes: TaskTypeNetworking | TaskTypeLogging,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.spec.FollowUpTasks()
			if actual != tc.followUpTypes {
				t.Errorf("Expected %v but got %v", tc.followUpTypes, actual)
			}
		})
	}
}
