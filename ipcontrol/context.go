package ipcontrol

import (
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type Context struct {
	IPAM client.Client
	database.AllocateConfig
	VPCInfo
	Logger
	LockSet database.LockSet

	containersCreated   []string
	blocksCreated       []*Block
	newVPCContainerPath string
}

type Block struct {
	name      string
	container string
}

type Logger interface {
	Log(string, ...interface{})
}

type SubnetConfig struct {
	SubnetType      string
	SubnetSize      int
	GroupName       string
	ParentContainer string
}

type SubnetInfo struct {
	Name             string
	Type             database.SubnetType
	ContainerPath    string
	AvailabilityZone string
	ResourceID       string
	CIDR             string
	GroupName        string
}

type VPCInfo struct {
	Name              string
	Stack             string
	Tenancy           string
	ResourceID        string
	NewCIDRs          []string
	AvailabilityZones []string
	NewSubnets        []*SubnetInfo
}
