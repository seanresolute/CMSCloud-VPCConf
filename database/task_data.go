package database

import "fmt"

type AddZonedSubnetsTaskData struct {
	VPCID               string
	Region              Region
	SubnetType          SubnetType
	SubnetSize          int
	GroupName           string
	JIRAIssueForComment string
	BeIdempotent        bool
}

type RemoveZonedSubnetsTaskData struct {
	VPCID        string
	Region       Region
	GroupName    string
	SubnetType   SubnetType
	BeIdempotent bool
}

type AddAvailabilityZoneTaskData struct {
	VPCID               string
	Region              Region
	AZName              string
	RequestID           int
	JIRAIssueForComment string
}

type RemoveAvailabilityZoneTaskData struct {
	VPCID               string
	Region              Region
	AZName              string
	JIRAIssueForComment string
}

type ImportVPCTaskData struct {
	VPCID     string
	VPCType   VPCType
	Region    Region
	AccountID string
}

type EstablishExceptionVPCTaskData struct {
	VPCID     string
	Region    Region
	AccountID string
}

type UnimportVPCTaskData struct {
	VPCID  string
	Region Region
}

type RepairVPCTaskData struct {
	VPCID  string
	Region Region
	Spec   VerifySpec
}

type VerifyVPCTaskData struct {
	VPCID  string
	Region Region
	Spec   VerifySpec
}

type DeleteVPCTaskData struct {
	AccountID string
	VPCID     string
	Region    Region
}

type CreateVPCTaskData struct {
	AllocateConfig
	VPCRequestID        *uint64
	JIRAIssueForComment string
}

type UpdateNetworkingTaskData struct {
	VPCID     string
	AWSRegion Region
	NetworkingConfig
	SkipVerify bool
}

type UpdateLoggingTaskData struct {
	VPCID  string
	Region Region
}

type NetworkingConfig struct {
	ConnectPublic                      bool
	ConnectPrivate                     bool
	ManagedTransitGatewayAttachmentIDs []uint64
	PeeringConnections                 []*PeeringConnectionConfig
}

type UpdateSecurityGroupsTaskData struct {
	VPCID     string
	AWSRegion Region
	SecurityGroupConfig
}

type SecurityGroupConfig struct {
	SecurityGroupSetIDs []uint64
}

type UpdateResolverRulesTaskData struct {
	VPCID     string
	AWSRegion Region
	ResolverRulesConfig
}
type UpdateVPCTypeTaskData struct {
	VPCID     string
	AWSRegion Region
	VPCType   VPCType
}

type UpdateVPCNameTaskData struct {
	VPCID     string
	AWSRegion Region
	VPCName   string
}

type DeleteUnusedResourcesTaskData struct {
	VPCID     string
	AWSRegion Region
	VPCType   VPCType
}

type ResolverRulesConfig struct {
	ManagedResolverRuleSetIDs []uint64
}

type ProvisionDNSTLSTaskData struct {
	DNSTLSRequestID uint64
}

type DeleteDNSTLSTaskData struct {
	DeleteRequestID uint64
}

type SynchronizeRouteTableStateFromAWSTaskData struct {
	VPCID  string
	Region Region
}

// TODO: rename BatchTaskTypes?
type TaskTypes uint64

const (
	TaskTypeNetworking     TaskTypes = 1 << iota
	TaskTypeRepair         TaskTypes = 1 << iota
	TaskTypeSecurityGroups TaskTypes = 1 << iota
	TaskTypeResolverRules  TaskTypes = 1 << iota
	TaskTypeVerifyState    TaskTypes = 1 << iota
	TaskTypeLogging        TaskTypes = 1 << iota
	TaskTypeSyncRoutes     TaskTypes = 1 << iota
)

func (t TaskTypes) Includes(sub TaskTypes) bool {
	return bitmapIncludes(uint64(t), uint64(sub))
}

type VerifySpec struct {
	VerifyNetworking     bool
	VerifyLogging        bool
	VerifyResolverRules  bool
	VerifySecurityGroups bool
	VerifyCIDRs          bool
	VerifyCMSNet         bool
}

func VerifyAllSpec() VerifySpec {
	return VerifySpec{
		VerifyNetworking:     true,
		VerifyLogging:        true,
		VerifyResolverRules:  true,
		VerifySecurityGroups: true,
		VerifyCIDRs:          true,
		VerifyCMSNet:         true,
	}
}

func (vs VerifySpec) VerifyTypes() VerifyTypes {
	var v VerifyTypes
	if vs.VerifyNetworking {
		v |= VerifyNetworking
	}
	if vs.VerifyLogging {
		v |= VerifyLogging
	}
	if vs.VerifyResolverRules {
		v |= VerifyResolverRules
	}
	if vs.VerifySecurityGroups {
		v |= VerifySecurityGroups
	}
	if vs.VerifyCIDRs {
		v |= VerifyCIDRs
	}
	if vs.VerifyCMSNet {
		v |= VerifyCMSNet
	}
	return v
}

func (vs VerifySpec) FollowUpTasks() TaskTypes {
	var t TaskTypes
	if vs.VerifyNetworking {
		t |= TaskTypeNetworking
	}
	if vs.VerifyLogging {
		t |= TaskTypeLogging
	}
	if vs.VerifyResolverRules {
		t |= TaskTypeResolverRules
	}
	if vs.VerifySecurityGroups {
		t |= TaskTypeSecurityGroups
	}
	return t
}

type TaskData struct {
	// Exactly one of the *Data fields should be non-nil.
	DeleteVPCTaskData                         *DeleteVPCTaskData
	CreateVPCTaskData                         *CreateVPCTaskData
	UpdateNetworkingTaskData                  *UpdateNetworkingTaskData
	UpdateLoggingTaskData                     *UpdateLoggingTaskData
	UpdateSecurityGroupsTaskData              *UpdateSecurityGroupsTaskData
	UpdateResolverRulesTaskData               *UpdateResolverRulesTaskData
	ImportVPCTaskData                         *ImportVPCTaskData
	EstablishExceptionVPCTaskData             *EstablishExceptionVPCTaskData
	UnimportVPCTaskData                       *UnimportVPCTaskData
	VerifyVPCTaskData                         *VerifyVPCTaskData
	RepairVPCTaskData                         *RepairVPCTaskData
	AddZonedSubnetsTaskData                   *AddZonedSubnetsTaskData
	RemoveZonedSubnetsTaskData                *RemoveZonedSubnetsTaskData
	ProvisionDNSTLSTaskData                   *ProvisionDNSTLSTaskData
	DeleteDNSTLSTaskData                      *DeleteDNSTLSTaskData
	UpdateVPCTypeTaskData                     *UpdateVPCTypeTaskData
	UpdateVPCNameTaskData                     *UpdateVPCNameTaskData
	DeleteUnusedResourcesTaskData             *DeleteUnusedResourcesTaskData
	SynchronizeRouteTableStateFromAWSTaskData *SynchronizeRouteTableStateFromAWSTaskData
	AddAvailabilityZoneTaskData               *AddAvailabilityZoneTaskData
	RemoveAvailabilityZoneTaskData            *RemoveAvailabilityZoneTaskData

	CloudTamerToken string // leave for QuickDNS
	AsUser          string
}

func (t *TaskData) LockTargetsNeeded(mm ModelsManager) ([]Target, error) {
	if t.DeleteVPCTaskData != nil {
		return []Target{TargetVPC(t.DeleteVPCTaskData.VPCID), TargetIPControlWrite}, nil
	} else if t.CreateVPCTaskData != nil {
		return []Target{TargetIPControlWrite}, nil
	} else if t.UpdateNetworkingTaskData != nil {
		// Update Networking might remove existing peering connections from the state or add new peering
		// connections from the config.
		targets := []Target{TargetVPC(t.UpdateNetworkingTaskData.VPCID)}
		vpc, err := mm.GetVPC(t.UpdateNetworkingTaskData.AWSRegion, t.UpdateNetworkingTaskData.VPCID)
		if err != nil {
			return nil, fmt.Errorf("Error getting VPC state: %s", err)
		}
		for _, pcx := range vpc.State.PeeringConnections {
			targets = append(targets, TargetVPC(pcx.AccepterVPCID), TargetVPC(pcx.RequesterVPCID))
		}
		for _, pcx := range t.UpdateNetworkingTaskData.PeeringConnections {
			targets = append(targets, TargetVPC(pcx.OtherVPCID))
		}
		return targets, nil
	} else if t.UpdateLoggingTaskData != nil {
		return []Target{TargetVPC(t.UpdateLoggingTaskData.VPCID)}, nil
	} else if t.UpdateSecurityGroupsTaskData != nil {
		return []Target{TargetVPC(t.UpdateSecurityGroupsTaskData.VPCID)}, nil
	} else if t.UpdateResolverRulesTaskData != nil {
		return []Target{TargetVPC(t.UpdateResolverRulesTaskData.VPCID)}, nil
	} else if t.ImportVPCTaskData != nil {
		return []Target{TargetVPC(t.ImportVPCTaskData.VPCID)}, nil
	} else if t.EstablishExceptionVPCTaskData != nil {
		return []Target{TargetVPC(t.EstablishExceptionVPCTaskData.VPCID)}, nil
	} else if t.UnimportVPCTaskData != nil {
		return []Target{TargetVPC(t.UnimportVPCTaskData.VPCID)}, nil
	} else if t.VerifyVPCTaskData != nil {
		return []Target{TargetVPC(t.VerifyVPCTaskData.VPCID)}, nil
	} else if t.RepairVPCTaskData != nil {
		// Repair might remove existing peering connections from the state.
		targets := []Target{TargetVPC(t.RepairVPCTaskData.VPCID)}
		vpc, err := mm.GetVPC(t.RepairVPCTaskData.Region, t.RepairVPCTaskData.VPCID)
		if err != nil {
			return nil, fmt.Errorf("Error getting VPC state: %s", err)
		}
		for _, pcx := range vpc.State.PeeringConnections {
			targets = append(targets, TargetVPC(pcx.AccepterVPCID), TargetVPC(pcx.RequesterVPCID))
		}
		return targets, nil
	} else if t.AddZonedSubnetsTaskData != nil {
		if t.AddZonedSubnetsTaskData.SubnetType == SubnetTypeUnroutable {
			return []Target{TargetVPC(t.AddZonedSubnetsTaskData.VPCID)}, nil
		}
		return []Target{TargetVPC(t.AddZonedSubnetsTaskData.VPCID), TargetIPControlWrite}, nil
	} else if t.RemoveZonedSubnetsTaskData != nil {
		if t.RemoveZonedSubnetsTaskData.SubnetType == SubnetTypeUnroutable {
			return []Target{TargetVPC(t.RemoveZonedSubnetsTaskData.VPCID)}, nil
		}
		return []Target{TargetVPC(t.RemoveZonedSubnetsTaskData.VPCID), TargetIPControlWrite}, nil
	} else if t.ProvisionDNSTLSTaskData != nil {
		return []Target{TargetFastDNSAPI}, nil
	} else if t.DeleteDNSTLSTaskData != nil {
		return []Target{TargetFastDNSAPI}, nil
	} else if t.AddAvailabilityZoneTaskData != nil {
		return []Target{TargetVPC(t.AddAvailabilityZoneTaskData.VPCID), TargetIPControlWrite}, nil
	} else if t.RemoveAvailabilityZoneTaskData != nil {
		return []Target{TargetVPC(t.RemoveAvailabilityZoneTaskData.VPCID), TargetIPControlWrite}, nil
	} else if t.UpdateVPCTypeTaskData != nil {
		return []Target{TargetVPC(t.UpdateVPCTypeTaskData.VPCID)}, nil
	} else if t.DeleteUnusedResourcesTaskData != nil {
		return []Target{TargetVPC(t.DeleteUnusedResourcesTaskData.VPCID)}, nil
	} else if t.SynchronizeRouteTableStateFromAWSTaskData != nil {
		return []Target{TargetVPC(t.SynchronizeRouteTableStateFromAWSTaskData.VPCID)}, nil
	} else if t.UpdateVPCNameTaskData != nil {
		return []Target{TargetVPC(t.UpdateVPCNameTaskData.VPCID)}, nil
	} else {
		return nil, fmt.Errorf("No target list implemented for this task")
	}
}

func (t *TaskData) GetRegion() Region {
	if t.DeleteVPCTaskData != nil {
		return t.DeleteVPCTaskData.Region
	} else if t.CreateVPCTaskData != nil {
		return Region(t.CreateVPCTaskData.AllocateConfig.AWSRegion)
	} else if t.UpdateNetworkingTaskData != nil {
		return t.UpdateNetworkingTaskData.AWSRegion
	} else if t.UpdateLoggingTaskData != nil {
		return t.UpdateLoggingTaskData.Region
	} else if t.UpdateSecurityGroupsTaskData != nil {
		return t.UpdateSecurityGroupsTaskData.AWSRegion
	} else if t.UpdateResolverRulesTaskData != nil {
		return t.UpdateResolverRulesTaskData.AWSRegion
	} else if t.ImportVPCTaskData != nil {
		return t.ImportVPCTaskData.Region
	} else if t.EstablishExceptionVPCTaskData != nil {
		return t.EstablishExceptionVPCTaskData.Region
	} else if t.UnimportVPCTaskData != nil {
		return t.UnimportVPCTaskData.Region
	} else if t.VerifyVPCTaskData != nil {
		return t.VerifyVPCTaskData.Region
	} else if t.RepairVPCTaskData != nil {
		return t.RepairVPCTaskData.Region
	} else if t.AddAvailabilityZoneTaskData != nil {
		return t.AddAvailabilityZoneTaskData.Region
	} else if t.RemoveAvailabilityZoneTaskData != nil {
		return t.RemoveAvailabilityZoneTaskData.Region
	} else if t.AddZonedSubnetsTaskData != nil {
		return t.AddZonedSubnetsTaskData.Region
	} else if t.RemoveZonedSubnetsTaskData != nil {
		return t.RemoveZonedSubnetsTaskData.Region
	} else if t.UpdateVPCTypeTaskData != nil {
		return t.UpdateVPCTypeTaskData.AWSRegion
	} else if t.UpdateVPCNameTaskData != nil {
		return t.UpdateVPCNameTaskData.AWSRegion
	} else if t.DeleteUnusedResourcesTaskData != nil {
		return t.DeleteUnusedResourcesTaskData.AWSRegion
	} else if t.SynchronizeRouteTableStateFromAWSTaskData != nil {
		return t.SynchronizeRouteTableStateFromAWSTaskData.Region
	}

	return ""
}
