package testmocks

import (
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmsnet"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type MockCMSNet struct {
	cmsnet.ClientInterface
}

func (m *MockCMSNet) SupportsRegion(region database.Region) bool {
	return false
}
