package deploy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
)

type mockELBV2 struct {
	elbv2iface.ELBV2API

	Rules []*elbv2.Rule
}

func (m *mockELBV2) DescribeRules(input *elbv2.DescribeRulesInput) (*elbv2.DescribeRulesOutput, error) {
	if aws.StringValue(input.ListenerArn) != listenerARN {
		return nil, fmt.Errorf("DescribeRules called on unexpected listener ARN %q", listenerARN)
	}
	return &elbv2.DescribeRulesOutput{
		Rules: m.Rules,
	}, nil
}

var (
	blueTargetGroupARN  = "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/my-targets/blue"
	greenTargetGroupARN = "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/my-targets/green"
	otherTargetGroupARN = "arn:aws:elasticloadbalancing:us-west-2:123456789012:targetgroup/my-targets/other"

	listenerARN = "arn:aws:elasticloadbalancing:us-west-2:123456789012:listener/app/my-load-balancer/50dc6c495c0c9188/f2f7dc8efc522ab2"
)

func TestGetALBState(t *testing.T) {
	type testCase struct {
		Name                  string
		Rules                 []*elbv2.Rule
		ExpectedErrorContains *string
		ExpectedGreenPriority int64
		ExpectedBluePriority  int64
	}

	testCases := []*testCase{
		{
			Name: "Basic test",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedGreenPriority: 2,
			ExpectedBluePriority:  1,
		},
		{
			Name: "Default ignored",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("default"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
			},
			ExpectedGreenPriority: 2,
			ExpectedBluePriority:  3,
		},
		{
			Name: "Other target group ignored",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("5"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &otherTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
			},
			ExpectedGreenPriority: 2,
			ExpectedBluePriority:  3,
		},
		{
			Name: "Blue missing",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedErrorContains: aws.String("Unable to determine blue priority"),
		},
		{
			Name: "Green missing",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("20"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("default"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedErrorContains: aws.String("Unable to determine green priority"),
		},
		{
			Name: "Multiple rules not allowed - blue",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
			},
			ExpectedErrorContains: aws.String("multiple rules"),
		},
		{
			Name: "Multiple rules not allowed - green",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("3"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedErrorContains: aws.String("multiple rules"),
		},
		{
			Name: "Non-final actions ignored",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedGreenPriority: 2,
			ExpectedBluePriority:  1,
		},
		{
			Name: "Non-forward actions ignored",
			Rules: []*elbv2.Rule{
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumFixedResponse),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("1"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &blueTargetGroupARN,
						},
					},
				},
				{
					Priority: aws.String("2"),
					Actions: []*elbv2.Action{
						{
							Type:           aws.String(elbv2.ActionTypeEnumForward),
							TargetGroupArn: &greenTargetGroupARN,
						},
					},
				},
			},
			ExpectedGreenPriority: 2,
			ExpectedBluePriority:  1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			elbsvc := &mockELBV2{
				Rules: tc.Rules,
			}
			state, err := GetALBState(elbsvc, &EnvironmentConfig{
				ListenerARN: listenerARN,
				TargetGroupARNs: map[Color]string{
					Blue:  blueTargetGroupARN,
					Green: greenTargetGroupARN,
				},
			})
			if err != nil {
				if tc.ExpectedErrorContains == nil || !strings.Contains(err.Error(), *tc.ExpectedErrorContains) {
					t.Fatalf("Test case %q returned unexpected error: %s", tc.Name, err)
				}
				return
			} else if tc.ExpectedErrorContains != nil {
				t.Fatalf("Test case %q was expected to return an error but didn't", tc.Name)
			}
			if state.BluePriority != tc.ExpectedBluePriority {
				t.Fatalf("Test case %q has wrong blue priority (%d instead of %d)", tc.Name, state.BluePriority, tc.ExpectedBluePriority)
			}
			if state.GreenPriority != tc.ExpectedGreenPriority {
				t.Fatalf("Test case %q has wrong green priority (%d instead of %d)", tc.Name, state.GreenPriority, tc.ExpectedGreenPriority)
			}
			gotBlueTargetGroupARN := aws.StringValue(state.BlueRule.Actions[len(state.BlueRule.Actions)-1].TargetGroupArn)
			if gotBlueTargetGroupARN != blueTargetGroupARN {
				t.Fatalf("Test case %q has wrong blue target group ARN (%s instead of %s)", tc.Name, gotBlueTargetGroupARN, blueTargetGroupARN)
			}
			gotGreenTargetGroupARN := aws.StringValue(state.GreenRule.Actions[len(state.GreenRule.Actions)-1].TargetGroupArn)
			if gotGreenTargetGroupARN != greenTargetGroupARN {
				t.Fatalf("Test case %q has wrong green target group ARN (%s instead of %s)", tc.Name, gotGreenTargetGroupARN, greenTargetGroupARN)
			}
		})
	}
}
