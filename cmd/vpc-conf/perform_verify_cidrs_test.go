package main

import (
	"fmt"
	"reflect"
	"testing"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestVerifyCIDRs(t *testing.T) {
	testCases := []struct {
		Name                    string
		VPC                     *database.VPC
		fix                     bool
		awsPrimaryCIDR          *string
		dbPrimaryCIDR           *string
		dbSecondaryCIDRs        []string
		cidrBlockAssociationSet []*ec2.VpcCidrBlockAssociation
		issuesExpected          []*database.Issue
	}{
		{
			Name: "no CIDRs present in database, only primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Primary CIDR 10.242.1.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "no CIDRs present in database, primary and secondary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Primary CIDR 10.242.1.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
				{
					Description: "Secondary CIDR 10.242.2.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "primary CIDR present in database, primary and secondary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:  aws.String("10.242.1.2/25"),
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Secondary CIDR 10.242.2.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "primary, secondary CIDRs present in database, primary and two secondaries in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:    aws.String("10.242.1.2/25"),
			dbSecondaryCIDRs: []string{"10.242.2.2/25"},
			awsPrimaryCIDR:   aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.3.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Secondary CIDR 10.242.3.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "db primary CIDR doesn't match primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:  aws.String("10.255.255.23/27"),
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Primary CIDR should be 10.242.1.2/25 instead of 10.255.255.23/27",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "Primary and secondary CIDRs present in database, only primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR:   aws.String("10.242.1.2/25"),
			dbPrimaryCIDR:    aws.String("10.242.1.2/25"),
			dbSecondaryCIDRs: []string{"100.100.100.100/28"},
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Extra secondary CIDR 100.100.100.100/28 in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "no CIDRs present in database, primary in AWS, secondary is disassociated",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("disassociated"),
					},
				},
			},
			issuesExpected: []*database.Issue{
				{
					Description: "Primary CIDR 10.242.1.2/25 is missing in vpc-conf",
					IsFixable:   true,
					Type:        database.VerifyCIDRs,
				},
			},
		},
		{
			Name: "FIX - no CIDRs present in database, only primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
		{
			Name: "FIX - no CIDRs present in database, primary and secondary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
		{
			Name: "FIX - primary CIDR present in database, primary and secondary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:  aws.String("10.242.1.2/25"),
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
		{
			Name: "FIX - primary, secondary CIDRs present in database, primary and two secondaries in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:    aws.String("10.242.1.2/25"),
			dbSecondaryCIDRs: []string{"10.242.2.2/25"},
			awsPrimaryCIDR:   aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.2.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
				{
					CidrBlock: aws.String("10.242.3.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
		{
			Name: "FIX - db primary CIDR doesn't match primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			dbPrimaryCIDR:  aws.String("10.255.255.23/27"),
			awsPrimaryCIDR: aws.String("10.242.1.2/25"),
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
		{
			Name: "FIX - Primary and secondary CIDRs present in database, only primary in AWS",
			VPC: &database.VPC{
				ID:     "vpc-abcdef01234567890",
				Region: testRegion,
			},
			awsPrimaryCIDR:   aws.String("10.242.1.2/25"),
			dbPrimaryCIDR:    aws.String("10.242.1.2/25"),
			dbSecondaryCIDRs: []string{"100.100.100.100/28"},
			cidrBlockAssociationSet: []*ec2.VpcCidrBlockAssociation{
				{
					CidrBlock: aws.String("10.242.1.2/25"),
					CidrBlockState: &ec2.VpcCidrBlockState{
						State: aws.String("associated"),
					},
				},
			},
			fix: true,
		},
	}

	for _, test := range testCases {
		ec2svc := &testmocks.MockEC2{
			PrimaryCIDR:             test.awsPrimaryCIDR,
			CIDRBlockAssociationSet: test.cidrBlockAssociationSet,
		}

		ctx := &awsp.Context{
			AWSAccountAccess: &awsp.AWSAccountAccess{
				EC2svc: ec2svc,
			},
			Clock: testClock,
		}

		mm := &testmocks.MockModelsManager{
			VPC:                test.VPC,
			VPCsPrimaryCIDR:    map[string]*string{testRegion + test.VPC.ID: test.dbPrimaryCIDR},
			VPCsSecondaryCIDRs: map[string][]string{testRegion + test.VPC.ID: test.dbSecondaryCIDRs},
			TestRegion:         testRegion,
		}

		t.Run(test.Name, func(t *testing.T) {
			issues, err := verifyCIDRs(ctx, mm, test.VPC, test.fix)
			if err != nil {
				t.Error(err)
				return
			}
			if len(issues) > 0 && len(test.issuesExpected) == 0 {
				t.Errorf("Expected no issues, but got %#v", issues)
				return
			}
			if !reflect.DeepEqual(test.issuesExpected, issues) {
				t.Errorf("Issues returned %#v do not match expected issues %#v", issues, test.issuesExpected)
				return
			}
			if test.fix {
				awsSecondaryCIDRs := []string{}
				for i, associationSet := range test.cidrBlockAssociationSet {
					if i == 0 { // skip primary cidr in the set
						continue
					}
					awsSecondaryCIDRs = append(awsSecondaryCIDRs, aws.StringValue(associationSet.CidrBlock))
				}
				if *mm.VPCsPrimaryCIDR[testRegion+test.VPC.ID] != *test.awsPrimaryCIDR {
					t.Errorf("Fixed primary CIDR %s doesn't match expected %s", *mm.PrimaryCIDR, *test.awsPrimaryCIDR)
					return
				}

				if diff := cmp.Diff(mm.VPCsSecondaryCIDRs[testRegion+test.VPC.ID], awsSecondaryCIDRs, cmpopts.EquateEmpty()); diff != "" {
					fmt.Printf("DB secondary CIDRs: %+v\n", mm.VPCsSecondaryCIDRs[testRegion+test.VPC.ID])
					fmt.Printf("AWS secondary CIDRs: %+v\n", awsSecondaryCIDRs)
					t.Errorf("DB Secondary CIDRs don't match AWS Secondary CIDRs: \n %s", diff)
				}
			}
		})
	}
}
