package testmocks

import (
	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
)

type MockAWSAccountAccessProvider struct {
	EC2 *MockEC2
}

func (m *MockAWSAccountAccessProvider) AccessAccount(accountID, region, asUser string) (*awsp.AWSAccountAccess, error) {
	access := awsp.AWSAccountAccess{
		EC2svc: m.EC2,
	}
	return &access, nil
}
