package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmsnet"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/ipcontrol"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/jira"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/orchestration"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/ram/ramiface"
	"github.com/aws/aws-sdk-go/service/route53resolver"
)

// These are operation/error code combinations that we want to retry
// because AWS's eventual consistency can result in an error indicating
// a missing prerequisite when in fact it is just not visible yet. See:
// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/query-api-troubleshooting.html#eventual-consistency
//
// map is: service "." operation name -> retryable aws error codes
var retryableAWSErrors = map[string][]string{
	"ec2.CreateFlowLogs": {
		"InvalidVpcId.NotFound",
	},
	"ec2.CreateTags": {
		"InvalidElasticIpID.NotFound",
		"InvalidInternetGatewayID.NotFound",
		"InvalidRouteTableID.NotFound",
		"InvalidVpcID.NotFound",
	},
	"ec2.DescribeRouteTables": {
		"InvalidRouteTableID.NotFound",
	},
}

const maxRetries = 100
const maxRetryTime = 10 * time.Minute

func handleRetry(req *request.Request) {
	if time.Since(req.Time) > maxRetryTime {
		req.Retryable = aws.Bool(false)
	} else if aerr, ok := req.Error.(awserr.Error); ok && req.Operation != nil {
		opID := req.ClientInfo.ServiceName + "." + req.Operation.Name
		if stringInSlice(aerr.Code(), retryableAWSErrors[opID]) {
			req.Retryable = aws.Bool(true)
		}
	}
}

func addRetryHandler(sess *session.Session) *session.Session {
	if sess.Config == nil {
		sess.Config = aws.NewConfig()
	}
	sess.Config.MaxRetries = aws.Int(maxRetries)
	sess.Handlers.Retry.PushBack(handleRetry)
	return sess
}

type AWSAccountAccessProvider interface {
	AccessAccount(accountID, region, asUser string) (*awsp.AWSAccountAccess, error)
}

type CredentialsServiceBackedAWSAccountAccessProvider struct {
	CredentialService credentialservice.CredentialsProvider
}

func (p *CredentialsServiceBackedAWSAccountAccessProvider) AccessAccount(accountID, region, asUser string) (*awsp.AWSAccountAccess, error) {
	sess, err := p.CredentialService.GetAWSSession(accountID, region, asUser)
	if err != nil {
		return nil, fmt.Errorf("Error getting AWS credentials: %s", err)
	}
	return &awsp.AWSAccountAccess{
		Session: addRetryHandler(sess),
	}, nil
}

type TaskContext struct {
	Task                     database.TaskInterface
	LockSet                  database.LockSet
	BaseAWSAccountAccess     *awsp.AWSAccountAccess
	AWSAccountAccessProvider AWSAccountAccessProvider
	ModelsManager            database.ModelsManager
	IPAM                     client.Client
	CMSNet                   cmsnet.ClientInterface
	Orchestration            *orchestration.Client
	TaskDatabase             *database.TaskDatabase
	AsUser                   string
}

func (s *Server) performTask(t *database.Task, lockSet database.LockSet) {
	dependsOn, err := t.DependsOn()
	if err != nil {
		t.Log("Error checking previous task status: %s", err)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	if dependsOn != nil && dependsOn.Status != database.TaskStatusSuccessful {
		t.Log("Prerequisite task \"%s\" did not succeed", dependsOn.Description)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	taskData := new(database.TaskData)
	err = json.Unmarshal(t.Data, taskData)
	if err != nil {
		t.Log("Unmarshalling error: %s", err)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	if s.LimitToAWSAccountIDs != nil && !stringInSlice(t.AccountID, s.LimitToAWSAccountIDs) {
		t.Log("This worker cannot work on AWS account %q", t.AccountID)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	if taskData.AsUser == "" {
		t.Log("AsUser for task %q must be set", t.Description)
		t.SetStatus(database.TaskStatusFailed)
		return
	}

	taskRegion := taskData.GetRegion()
	if taskRegion == "" {
		t.Log("region for task %q must be set", t.Description)
		t.SetStatus(database.TaskStatusFailed)
		return
	}

	sess, err := s.CredentialService.GetAWSSession(t.AccountID, string(taskRegion), taskData.AsUser)
	if err != nil {
		t.Log("Error getting AWS credentials: %s", err)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	awsAccountAccess := &awsp.AWSAccountAccess{
		Session: addRetryHandler(sess),
	}

	ctx := &TaskContext{
		Task:                 t,
		TaskDatabase:         s.TaskDatabase,
		LockSet:              lockSet,
		BaseAWSAccountAccess: awsAccountAccess,
		AWSAccountAccessProvider: &CredentialsServiceBackedAWSAccountAccessProvider{
			CredentialService: s.CredentialService,
		},
		ModelsManager: s.ModelsManager,
		IPAM:          s.IPAM,
		CMSNet:        cmsnet.NewClient(s.CMSNetConfig, s.LimitToAWSAccountIDs, s.CredentialService),
		Orchestration: s.Orchestration,
		AsUser:        taskData.AsUser,
	}

	if taskData.CreateVPCTaskData != nil {
		ctx.performCreateVPCTask(taskData.CreateVPCTaskData)
	} else if taskData.DeleteVPCTaskData != nil {
		ctx.performDeleteVPCTask(taskData.DeleteVPCTaskData)
	} else if taskData.UpdateNetworkingTaskData != nil {
		ctx.performUpdateNetworkingTask(taskData.UpdateNetworkingTaskData)
	} else if taskData.UpdateLoggingTaskData != nil {
		ctx.performUpdateLoggingTask(taskData.UpdateLoggingTaskData)
	} else if taskData.UpdateSecurityGroupsTaskData != nil {
		ctx.performUpdateSecurityGroupsTask(taskData.UpdateSecurityGroupsTaskData)
	} else if taskData.ImportVPCTaskData != nil {
		ctx.performImportVPCTask(taskData.ImportVPCTaskData)
	} else if taskData.EstablishExceptionVPCTaskData != nil {
		ctx.performEstablishExceptionVPCTask(taskData.EstablishExceptionVPCTaskData)
	} else if taskData.UnimportVPCTaskData != nil {
		ctx.performUnimportVPCTask(taskData.UnimportVPCTaskData)
	} else if taskData.VerifyVPCTaskData != nil {
		ctx.performVerifyVPCTask(taskData.VerifyVPCTaskData)
	} else if taskData.RepairVPCTaskData != nil {
		ctx.performRepairVPCTask(taskData.RepairVPCTaskData)
	} else if taskData.AddZonedSubnetsTaskData != nil {
		ctx.performAddZonedSubnetsTask(taskData.AddZonedSubnetsTaskData)
	} else if taskData.RemoveZonedSubnetsTaskData != nil {
		ctx.performRemoveZonedSubnetsTask(taskData.RemoveZonedSubnetsTaskData)
	} else if taskData.AddAvailabilityZoneTaskData != nil {
		ctx.performAddAvailabilityZoneTask(taskData.AddAvailabilityZoneTaskData)
	} else if taskData.RemoveAvailabilityZoneTaskData != nil {
		ctx.performRemoveAvailabilityZoneTask(taskData.RemoveAvailabilityZoneTaskData)
	} else if taskData.UpdateResolverRulesTaskData != nil {
		ctx.performUpdateResolverRulesTaskData(taskData.UpdateResolverRulesTaskData)
	} else if taskData.UpdateVPCTypeTaskData != nil {
		ctx.performUpdateVPCTypeTask(taskData.UpdateVPCTypeTaskData)
	} else if taskData.UpdateVPCNameTaskData != nil {
		ctx.performUpdateVPCNameTask(taskData.UpdateVPCNameTaskData)
	} else if taskData.DeleteUnusedResourcesTaskData != nil {
		ctx.performDeleteUnusedResourcesTask(taskData.DeleteUnusedResourcesTaskData)
	} else if taskData.SynchronizeRouteTableStateFromAWSTaskData != nil {
		ctx.performSynchronizeRouteTableStateFromAWSTask(taskData.SynchronizeRouteTableStateFromAWSTaskData)
	} else {
		t.Log("Unknown task type")
		t.SetStatus(database.TaskStatusFailed)
	}
}

func subnetName(vpcName, availabilityZone, groupName string, subnetType database.SubnetType) string {
	if groupName == "" {
		groupName = strings.ToLower(string(subnetType))
	}
	return fmt.Sprintf("%s-%s-%c", vpcName, groupName, availabilityZone[len(availabilityZone)-1])
}

func sharedPublicRouteTableName(vpcName string) string {
	return vpcName + "-public"
}

func sharedFirewallRouteTableName(vpcName string) string {
	return vpcName + "-firewall"
}

func igwRouteTableName(vpcName string) string {
	return vpcName + "-igw"
}

func routeTableName(vpcName, availabilityZone, groupName string, subnetType database.SubnetType) string {
	return subnetName(vpcName, availabilityZone, groupName, subnetType)
}

func natGatewayName(vpcName, availabilityZone string) string {
	return fmt.Sprintf("%s-%c", vpcName, availabilityZone[len(availabilityZone)-1])
}

func eipName(vpcName, availabilityZone string) string {
	return fmt.Sprintf("%s-nat-gateway-%c", vpcName, availabilityZone[len(availabilityZone)-1])
}

func transitGatewayAttachmentName(vpcName, managedName string) string {
	return fmt.Sprintf("%s:%s", vpcName, managedName)
}

func internetGatewayName(vpcName string) string {
	return vpcName
}

func peeringConnectionName(requesterName, accepterName string) string {
	return fmt.Sprintf("%s-%s", requesterName, accepterName)
}

func firewallAlertLogName(vpcID string) string {
	return fmt.Sprintf("cms-cloud-%s-firewallalertlogs", vpcID)
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func uint64InSlice(n uint64, slice []uint64) bool {
	for _, k := range slice {
		if k == n {
			return true
		}
	}
	return false
}

func sameStringSlice(sl1, sl2 []string) bool {
	sl1Copy := append([]string{}, sl1...)
	sl2Copy := append([]string{}, sl2...)
	sort.Strings(sl1Copy)
	sort.Strings(sl2Copy)
	return reflect.DeepEqual(sl1Copy, sl2Copy)
}

func selectBySubnetID(state *database.VPCState, subnetID string) (*database.SubnetInfo, bool) {
	for _, azInfo := range state.AvailabilityZones {
		for _, sns := range azInfo.Subnets {
			for _, subnet := range sns {
				if subnet.SubnetID == subnetID {
					return subnet, true
				}
			}
		}
	}
	return nil, false
}

func selectBySubnetType(state *database.VPCState, subnetType database.SubnetType) (subnets []*database.SubnetInfo) {
	for _, az := range state.AvailabilityZones {
		subnets = append(subnets, az.Subnets[subnetType]...)
	}
	return subnets
}

func ensurePrefixListSharedWithAccount(ctx *awsp.Context, plRAM ramiface.RAMAPI, plIDs []string, region database.Region, accountID string) error {
	out, err := plRAM.ListResources(&ram.ListResourcesInput{
		ResourceOwner: aws.String("SELF"),
	})
	if err != nil {
		return fmt.Errorf("Error listing shared resources : %s", err)
	}

	prefixListAccountID := prefixListAccountIDCommercial
	if region.IsGovCloud() {
		prefixListAccountID = prefixListAccountIDGovCloud
	}

	addedShares := make(map[string]struct{})
	for _, id := range plIDs {
		plARN := prefixListARN(region, prefixListAccountID, id)
		var shareARN string
		found := false
		for _, resource := range out.Resources {
			if aws.StringValue(resource.Arn) == plARN {
				found = true
				shareARN = aws.StringValue(resource.ResourceShareArn)
				break
			}
		}
		if !found {
			return fmt.Errorf("The configured Prefix List %s is not yet shared. You must share the Prefix List from account %s in the AWS Console first.", id, prefixListAccountID)
		}

		_, ok := addedShares[shareARN]
		if !ok {
			err = ctx.EnsurePrincipalOnShare(plRAM, accountID, shareARN)
			if err != nil {
				return fmt.Errorf("Error ensuring account is principal on share: %s", err)
			}
			addedShares[shareARN] = struct{}{}
		}
	}
	return nil
}

const mtgaIDTagKey = "VPC Conf MTGA IDs"

func addOrUpdateTGAttachmentTags(ctx *awsp.Context, managedAttachmentsByID map[uint64]*database.ManagedTransitGatewayAttachment, managedIDs []uint64, tgaID string) error {
	ids := []string{}
	for _, id := range managedIDs {
		ids = append(ids, strconv.FormatUint(id, 10))
	}
	maName := generateMTGAName(managedIDs, managedAttachmentsByID)
	err := ctx.Tag(tgaID, map[string]string{
		mtgaIDTagKey: strings.Join(ids, ","),
		"Name":       transitGatewayAttachmentName(ctx.VPCName, maName),
	})
	if err != nil {
		return err
	}
	return nil
}

func parseTGAttachmentTag(value string) ([]uint64, error) {
	ids := strings.Split((value), ",")
	managedIDs := []uint64{}
	for _, id := range ids {
		i, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return nil, err
		}
		managedIDs = append(managedIDs, i)
	}
	return managedIDs, nil
}

func generateMTGAName(ids []uint64, managedAttachmentsByID map[uint64]*database.ManagedTransitGatewayAttachment) string {
	names := []string{}
	for _, id := range ids {
		names = append(names, (managedAttachmentsByID[id].Name))
	}
	sort.Strings(names)
	return strings.Join(names, "/")
}

func cidrIsWithin(needle, haystack *net.IPNet) bool {
	needleSize, _ := needle.Mask.Size()
	haystackSize, _ := haystack.Mask.Size()

	if haystack.Contains(needle.IP) && needleSize >= haystackSize {
		return true
	}
	return false
}

func childCIDRsUseAllParentCIDRSpace(parentIPNet *net.IPNet, childIPNets []*net.IPNet) bool {
	parentSize, _ := parentIPNet.Mask.Size()
	parentIPSpace := 1 << (32 - parentSize)

	for _, childIPNet := range childIPNets {
		childSize, _ := childIPNet.Mask.Size()
		childIPSpace := 1 << (32 - childSize)

		parentIPSpace -= childIPSpace
	}

	return parentIPSpace <= 0
}

func blocksToIPNets(blocks []*models.WSChildBlock) ([]*net.IPNet, error) {
	var ipNets []*net.IPNet
	for _, b := range blocks {
		cidr := fmt.Sprintf("%s/%s", b.BlockAddr, b.BlockSize)
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		ipNets = append(ipNets, ipNet)
	}
	if ok, err := ipNetsOverlap(ipNets); ok {
		return nil, err
	}
	return ipNets, nil
}

func ipNetsOverlap(ipNets []*net.IPNet) (bool, error) {
	for i1, n1 := range ipNets {
		for i2 := 0; i2 < i1; i2++ {
			n2 := ipNets[i2]
			if n1.Contains(n2.IP) || n2.Contains(n1.IP) {
				return true, fmt.Errorf("IP %s overlaps with IP %s", n1.IP, n2.IP)
			}
		}
	}
	return false, nil
}

func disassociateCIDRBlocks(ctx *awsp.Context, t database.TaskInterface, VPCID string, CIDRs []string) error {
	vpcOut, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{&VPCID},
	})
	if err != nil {
		return fmt.Errorf("Error listing VPC: %s", err)
	}
	for _, cidr := range CIDRs {
		for _, assoc := range vpcOut.Vpcs[0].CidrBlockAssociationSet {
			if assoc.CidrBlockState != nil && aws.StringValue(assoc.CidrBlockState.State) != "associated" {
				continue
			}
			if aws.StringValue(assoc.CidrBlock) == cidr {
				_, err := ctx.EC2().DisassociateVpcCidrBlock(&ec2.DisassociateVpcCidrBlockInput{
					AssociationId: assoc.AssociationId,
				})
				if err != nil {
					return fmt.Errorf("Error disassociating %s from VPC: %s", cidr, err)
				}
				t.Log("Disassociated %s from VPC", cidr)
			}
		}
	}
	return nil
}

func deleteFirewallResourcesWithinAZ(awsctx *awsp.Context, vpcWriter database.VPCWriter, vpc *database.VPC, azName string, az *database.AvailabilityZoneInfra) error {
	routeTableHasCidr := func(table *database.RouteTableInfo, cidr string) bool {
		for _, route := range table.Routes {
			if route.Destination == cidr {
				return true
			}
		}
		return false
	}
	publicSubnetIDtoCIDR, err := awsctx.GetPublicSubnetIDtoCIDR(database.AZMap{azName: az})
	if err != nil {
		return fmt.Errorf("Error getting cidrs for public subnets: %s", err)
	}

	igwRT, ok := vpc.State.RouteTables[vpc.State.InternetGateway.RouteTableID]
	if !ok {
		return fmt.Errorf("No IGW route table info found for ID %q", vpc.State.InternetGateway.RouteTableID)
	}

	for _, publicSubnet := range az.Subnets[database.SubnetTypePublic] {
		publicCIDR, ok := publicSubnetIDtoCIDR[publicSubnet.SubnetID]
		if !ok {
			return fmt.Errorf("No CIDR found for public subnet ID %s", publicSubnet.SubnetID)
		}
		if routeTableHasCidr(igwRT, publicCIDR) {
			igwRT.Routes, err = setRoute(awsctx, vpc.State.InternetGateway.RouteTableID, publicCIDR, igwRT.Routes, nil)
			if err != nil {
				return fmt.Errorf("Error updating IGW route table: %s", err)
			}
		} else {
			awsctx.Log("CIDR route for public subnet %s (%s) not found on %s, skipping removal", publicSubnet.SubnetID, publicCIDR, vpc.State.InternetGateway.RouteTableID)
		}
	}

	if az.PublicRouteTableID != "" {
		publicRT, ok := vpc.State.RouteTables[az.PublicRouteTableID]
		if !ok {
			return fmt.Errorf("No public route table info found for AZ %s and ID %q", azName, vpc.State.PublicRouteTableID)
		}
		if routeTableHasCidr(publicRT, internetRoute) {
			publicRT.Routes, err = setRoute(awsctx, az.PublicRouteTableID, internetRoute, publicRT.Routes, nil)
			if err != nil {
				return fmt.Errorf("Error deleting default route on %s: %s", az.PublicRouteTableID, err)
			}
		} else {
			awsctx.Log("Internet route not found on %s, skipping removal", az.PublicRouteTableID)
		}
	}

	subnetsToDisassociate := make([]string, 0)
	for _, firewallSubnet := range az.Subnets[database.SubnetTypeFirewall] {
		subnetsToDisassociate = append(subnetsToDisassociate, firewallSubnet.SubnetID)
	}
	newFirewallSubnetIDs := make([]string, 0)
	for _, subnetID := range vpc.State.Firewall.AssociatedSubnetIDs {
		if !stringInSlice(subnetID, subnetsToDisassociate) {
			newFirewallSubnetIDs = append(newFirewallSubnetIDs, subnetID)
		}
	}
	err = updateFirewallSubnetAssociations(awsctx, vpc, vpcWriter, newFirewallSubnetIDs)
	if err != nil {
		return fmt.Errorf("Error updating firewall subnet associations: %s", err)
	}

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		return fmt.Errorf("Error updating state: %s", err)
	}
	return nil
}

func destroyNATGatewayResourcesInAZ(awsctx *awsp.Context, vpcWriter database.VPCWriter, vpc *database.VPC, az *database.AvailabilityZoneInfra) error {
	// Clear the default route first
	if az.PrivateRouteTableID != "" {
		err := setRouteAllNonPublic(awsctx, az, vpc, internetRoute, nil)
		if err != nil {
			return fmt.Errorf("Error updating route tables: %s", err)
		}
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	if az.NATGateway.NATGatewayID != "" {
		err := awsctx.DeleteNATGateway(az.NATGateway.NATGatewayID)
		if err != nil {
			return fmt.Errorf("Error deleting NAT Gateway %s: %s", az.NATGateway.NATGatewayID, err)
		}
		az.NATGateway.NATGatewayID = ""
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	if az.NATGateway.EIPID != "" {
		err := awsctx.ReleaseEIP(az.NATGateway.EIPID)
		if err != nil {
			return fmt.Errorf("Error releasing EIP %s: %s", az.NATGateway.EIPID, err)
		}
		az.NATGateway.EIPID = ""
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	return nil
}

func deleteSubnets(awsctx *awsp.Context, subnetIDs []string) error {
	// Delete subnets from AWS
	for _, subnet := range subnetIDs {
		_, err := awsctx.EC2().DeleteSubnet(&ec2.DeleteSubnetInput{SubnetId: aws.String(subnet)})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == "InvalidSubnetID.NotFound" {
					awsctx.Log("Subnet %q not found", subnet)
					continue
				}
			}
			return fmt.Errorf("Error deleting subnet %s: %s\n", subnet, err)
		} else {
			awsctx.Log("Deleted subnet %s", subnet)
		}
	}
	return nil
}

func deleteRouteTables(awsctx *awsp.Context, routeTableIDs []string) error {
	// Delete route tables now that we've removed the subnets themselves
	for _, routeTableID := range routeTableIDs {
		_, err := awsctx.EC2().DeleteRouteTable(&ec2.DeleteRouteTableInput{RouteTableId: &routeTableID})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == "InvalidRouteTableID.NotFound" {
					awsctx.Log("Route Table %s not found", routeTableID)
					continue
				}
			}
			return fmt.Errorf("Error deleting route table %s: %s\n", routeTableID, err)
		}
		awsctx.Log("Deleted route table: %s", routeTableID)
	}
	return nil
}

func parentWillBeEmptyAfterDeletions(containerTree map[string][]*models.WSContainer, parentName string, containersToDelete []string) bool {
	if children, ok := containerTree[parentName]; ok {
		for _, child := range children {
			childPath := "/" + parentName + "/" + child.ContainerName
			if stringInSlice(childPath, containersToDelete) {
				continue
			}
			return false
		}
	}
	return true
}

func getContainerTree(c client.Client, accountID, vpcID string) (map[string][]*models.WSContainer, error) {
	containers, err := c.ListContainersForVPC(accountID, vpcID)
	if err != nil {
		return nil, err
	}
	tree := make(map[string][]*models.WSContainer)
	for _, c1 := range containers {
		tree[c1.ParentName] = append(tree[c1.ParentName], c1)
	}
	return tree, nil
}

// Pass all subnetIDs to delete everything
func (taskContext *TaskContext) DeleteCMSNetConfigurations(vpc *database.VPC, subnetIDs []string) error {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	asUser := taskContext.AsUser

	setStatus(t, database.TaskStatusInProgress)

	awsctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            vpc.ID,
	}

	if subnetIDs == nil {
		return fmt.Errorf("nil subnetIDs but not intending to delete all CMSnet connections")
	}
	subnetNets := []*net.IPNet{}
	subnetsFound := []string{}
	for _, subnetID := range subnetIDs {
		out, err := awsctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
			SubnetIds: aws.StringSlice([]string{subnetID}),
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); (ok && aerr.Code() != "InvalidSubnetID.NotFound") || !ok {
				return fmt.Errorf("Error describing subnet [%s]: %s\n", subnetID, err)
			}
		}
		for _, subnet := range out.Subnets {
			cidr := aws.StringValue(subnet.CidrBlock)
			_, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				return fmt.Errorf("Could not parse CIDR %q for subnet %s: %s\n", cidr, aws.StringValue(subnet.SubnetId), err)
			}
			subnetsFound = append(subnetsFound, aws.StringValue(subnet.SubnetId))
			subnetNets = append(subnetNets, ipNet)
		}

	}
	if len(subnetsFound) < 1 {
		return fmt.Errorf("Error describing subnets [%s]: got %d subnets\n", strings.Join(subnetIDs, ", "), len(subnetsFound))
	}

	for _, subnet := range subnetIDs {
		if !stringInSlice(subnet, subnetsFound) {
			awsctx.Logger.Log("Subnet %s not found", subnet)
		}
	}

	inSubnets := func(cidr string) (bool, error) {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return false, fmt.Errorf("Could not parse CIDR %q: %s\n", cidr, err)
		}
		for _, ipNet := range subnetNets {
			if ipNet.Contains(ip) {
				return true, nil
			}
		}
		return false, nil
	}

	natResult, err := taskContext.CMSNet.GetAllNATRequests(vpc.AccountID, vpc.Region, vpc.ID, asUser)
	if err != nil {
		return fmt.Errorf("Error getting CMSNet NAT info: %s", err)
	}

	for _, request := range natResult.Requests {
		matches := true
		if subnetIDs != nil {
			matches, err = inSubnets(request.Params.InsideNetwork)
			if err != nil {
				return err
			}
		}
		if matches && request.ConnectionStatus.BlocksDeletion() {
			return fmt.Errorf("A CMSNet NAT is currently pending for %s. Wait for it to succeed or fail and try again.", request.Params.InsideNetwork)
		}
	}
	for _, nat := range natResult.NATs {
		matches := true
		if subnetIDs != nil {
			matches, err = inSubnets(nat.InsideNetwork)
			if err != nil {
				return err
			}
		}
		if matches {
			_, err := taskContext.CMSNet.DeleteNAT(nat.RequestID, vpc.AccountID, vpc.Region, vpc.ID, false, asUser)
			if err != nil {
				return fmt.Errorf("Error deleting CMSNet NAT %q: %s", nat.RequestID, err)
			}
			awsctx.Logger.Log("Deleted CMSNet NAT %q", nat.RequestID)
		}
	}

	connResult, err := taskContext.CMSNet.GetAllConnectionRequests(vpc.AccountID, vpc.Region, vpc.ID, asUser)
	if err != nil {
		return fmt.Errorf("Error getting CMSNet connection info: %s", err)
	}

	for _, request := range connResult.Requests {
		if (subnetIDs == nil || stringInSlice(request.Params.SubnetID, subnetIDs)) && request.ConnectionStatus.BlocksDeletion() {
			return fmt.Errorf("A CMSNet connection is currently pending for subnet %s. Wait for it to succeed or fail and try again.", request.Params.SubnetID)
		}
	}
	for _, activation := range connResult.Activations {
		if subnetIDs == nil || stringInSlice(activation.SubnetID, subnetIDs) {
			_, err := taskContext.CMSNet.DeleteActivation(activation.RequestID, vpc.AccountID, vpc.Region, vpc.ID, asUser)
			if err != nil {
				return fmt.Errorf("Error deleting CMSNet connection %q: %s", activation.RequestID, err)
			}
			awsctx.Logger.Log("Deleted CMSNet activation %q", activation.RequestID)
		}
	}
	return nil
}

func (taskContext *TaskContext) performRemoveZonedSubnetsTask(config *database.RemoveZonedSubnetsTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)

	awsctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
	}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.Region, config.VPCID)
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateZonedSubnets() || (vpc.State.VPCType.IsMigrating() && config.SubnetType != database.SubnetTypeFirewall) {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	subnetIDs := []string{}
	routeTableIDs := []string{}
	var matchingSubnetType *database.SubnetType

	vpcOut, err := awsctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(config.VPCID)},
	})
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if len(vpcOut.Vpcs) != 1 {
		t.Log("Error getting VPC info: expected 1 VPC for VPC ID %s but got %d:", config.VPCID, len(vpcOut.Vpcs))
		setStatus(t, database.TaskStatusFailed)
		return
	}

	var primaryIPNet *net.IPNet
	primaryCIDR := aws.StringValue(vpcOut.Vpcs[0].CidrBlock)
	_, primaryIPNet, err = net.ParseCIDR(primaryCIDR)
	if err != nil {
		t.Log("Error parsing primary CIDR %q: %s", primaryCIDR, err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	subnetOut, err := awsctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(config.VPCID)},
			},
		},
	})
	if err != nil {
		t.Log("Error getting subnet info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	subnetCIDRsByID := make(map[string]string)

	for _, s := range subnetOut.Subnets {
		subnetCIDRsByID[aws.StringValue(s.SubnetId)] = aws.StringValue(s.CidrBlock)
	}

	// Find subnets to remove
	for _, az := range vpc.State.AvailabilityZones.InOrder() {
		for subnetType, subnets := range az.Subnets {
			filteredSubnets := []*database.SubnetInfo{}
			for _, subnet := range subnets {
				if subnet.GroupName == config.GroupName {
					if subnetType != config.SubnetType {
						t.Log("Subnet %q of type %q matches the configured group name %q, but not the configured subnet type %q ", subnet.SubnetID, subnetType, config.GroupName, config.SubnetType)
						setStatus(t, database.TaskStatusFailed)
						return
					}

					if matchingSubnetType == nil {
						st := subnetType
						matchingSubnetType = &st
					} else if subnetType != *matchingSubnetType {
						t.Log("Found matching subnets with multiple types: %q and %q", *matchingSubnetType, subnetType)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					subnetIDs = append(subnetIDs, subnet.SubnetID)

					subnetCIDR, inAWS := subnetCIDRsByID[subnet.SubnetID]
					if inAWS {
						_, subnetIPNet, err := net.ParseCIDR(subnetCIDR)
						if err != nil {
							t.Log("Error parsing subnet CIDR %q: %s", subnetCIDR, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						if cidrIsWithin(subnetIPNet, primaryIPNet) {
							t.Log("Cannot remove subnet %s with CIDR %q because it is in the VPC's primary CIDR %q", subnet.SubnetID, subnetCIDR, primaryCIDR)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					} else {
						t.Log("Subnet %s is missing from AWS, removing from VPC Conf state", subnet.SubnetID)
					}

					if subnet.CustomRouteTableID != "" {
						if subnetSharesCustomRouteTable(subnet.SubnetID, subnet.CustomRouteTableID, vpc) {
							t.Log("V4 subnet %s shares custom route table %s", subnet.SubnetID, subnet.CustomRouteTableID)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						routeTableIDs = append(routeTableIDs, subnet.CustomRouteTableID)
					}
				} else {
					filteredSubnets = append(filteredSubnets, subnet)
				}
			}
			az.Subnets[subnetType] = filteredSubnets
		}
	}
	if len(subnetIDs) == 0 {
		t.Log("No subnets matching %q", config.GroupName)

		if config.BeIdempotent {
			setStatus(t, database.TaskStatusSuccessful)
		} else {
			setStatus(t, database.TaskStatusFailed)
		}

		return
	}
	sort.Strings(subnetIDs)
	sort.Strings(routeTableIDs)

	subnetContainers := []*models.WSContainer{}
	deleteParent := true
	parentContainer := ""
	containersToDelete := []string{}

	if *matchingSubnetType != database.SubnetTypeUnroutable {
		// Find corresponding containers
		for _, subnetID := range subnetIDs {
			container, err := taskContext.IPAM.GetSubnetContainer(subnetID)
			if err != nil {
				t.Log("Error finding subnet container: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			subnetContainers = append(subnetContainers, container)
		}

		// Identify parent container
		for _, c := range subnetContainers {
			if parentContainer == "" {
				parentContainer = c.ParentName
			} else if parentContainer != c.ParentName {
				t.Log("Found multiple parent containers: %q and %q", parentContainer, c.ParentName)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		// Check whether the subnet containers are all the children of the parent container.
		// If so we will delete the parent container and disassociate its blocks from the VPC.
		containerTree, err := getContainerTree(taskContext.IPAM, vpc.AccountID, config.VPCID)
		if err != nil {
			t.Log("Error finding VPC containers: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		for _, container := range subnetContainers {
			containersToDelete = append(containersToDelete, fmt.Sprintf("/%s/%s", container.ParentName, container.ContainerName))
		}
		deleteParent = parentWillBeEmptyAfterDeletions(containerTree, parentContainer, containersToDelete)

		// Delete any CMSNet connections for these zones
		if taskContext.CMSNet.SupportsRegion(vpc.Region) {
			err := taskContext.DeleteCMSNetConfigurations(vpc, subnetIDs)
			if err != nil {
				t.Log("Error deleting CMSNet configurations: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}
	err = deleteSubnets(awsctx, subnetIDs)
	if err != nil {
		t.Log("Error deleting subnet resources: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = deleteRouteTables(awsctx, routeTableIDs)
	if err != nil {
		t.Log("Error deleting route tables: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if *matchingSubnetType == database.SubnetTypeUnroutable {
		var cidrsToDisassociate []string
		var supernetCIDR *net.IPNet

		for _, subnetID := range subnetIDs {
			if cidr, ok := subnetCIDRsByID[subnetID]; ok {
				nextSupernetCIDR, err := awsp.GetUnroutableSupernet(cidr)
				if err != nil {
					t.Log("Error getting supernet CIDR for %s: %s", cidr, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				if supernetCIDR == nil {
					supernetCIDR = nextSupernetCIDR
				}

				if nextSupernetCIDR.String() != supernetCIDR.String() {
					t.Log("Multiple supernet CIDRs found in same unroutable group: %s - %s ", supernetCIDR.String(), nextSupernetCIDR.String())
					setStatus(t, database.TaskStatusFailed)
					return
				}

				if !stringInSlice(supernetCIDR.String(), cidrsToDisassociate) {
					cidrsToDisassociate = append(cidrsToDisassociate, supernetCIDR.String())
				}
			}
		}
		// are there any subnets from this /16 in another subnet group?
		for subnetID, subnetCIDR := range subnetCIDRsByID {
			if stringInSlice(subnetID, subnetIDs) {
				continue // already accounted for
			}

			ip, _, err := net.ParseCIDR(subnetCIDR)
			if err != nil {
				t.Log("Error parsing subnet CIDR for %s: %s", subnetCIDR, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			if supernetCIDR.Contains(ip) {
				t.Log("Subnet CIDR %q is assigned to another subnet group", subnetCIDR)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		err = disassociateCIDRBlocks(awsctx, t, config.VPCID, cidrsToDisassociate)
		if err != nil {
			t.Log("Error disassociating CIDR blocks from VPC: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		for _, cidr := range cidrsToDisassociate {
			err = taskContext.ModelsManager.DeleteVPCCIDR(vpc.ID, vpc.Region, cidr)
			if err != nil {
				t.Log("Error deleting CIDR blocks from database: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	} else {
		subnetIPNetsByParent := make(map[*net.IPNet][]*net.IPNet)
		blocksToDelete := make(map[string]string) // CIDR -> container

		parentBlocks, err := taskContext.IPAM.ListBlocks("/"+parentContainer, false, false)
		if err != nil {
			t.Log("Error listing parent container blocks: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		parentIPNets, err := blocksToIPNets(parentBlocks)
		if err != nil {
			t.Log("Error converting parent blocks to IPNets: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		if deleteParent {
			containersToDelete = append(containersToDelete, "/"+parentContainer)

			// Disassociate all of the parent's CIDR blocks before deleting it.
			var parentCIDRs []string
			for _, p := range parentBlocks {
				parentCIDRs = append(parentCIDRs, fmt.Sprintf("%s/%s", p.BlockAddr, p.BlockSize))
			}
			err = disassociateCIDRBlocks(awsctx, t, config.VPCID, parentCIDRs)
			if err != nil {
				t.Log("Error disassociating CIDR blocks from VPC: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = deleteCIDRs(taskContext.ModelsManager, parentCIDRs, vpc)
			if err != nil {
				t.Log("Error deleting CIDR blocks from vpc-conf: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		} else {
			// Find and disassociate the immediate parent block of the subnet blocks
			for _, container := range subnetContainers {
				subnetBlocks, err := taskContext.IPAM.ListBlocks(fmt.Sprintf("/%s/%s", container.ParentName, container.ContainerName), false, false)
				if err != nil {
					t.Log("Error listing subnet container blocks: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				subnetIPNets, err := blocksToIPNets(subnetBlocks)
				if err != nil {
					t.Log("Error converting subnet blocks to IPNets: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

				for _, p := range parentIPNets {
					for _, s := range subnetIPNets {
						if cidrIsWithin(s, p) {
							subnetIPNetsByParent[p] = append(subnetIPNetsByParent[p], s)
						}
					}
				}
			}

			// Make sure any other child blocks of the parent block are free so the block can safely be deleted
			freeParentBlocks, err := taskContext.IPAM.ListBlocks("/"+parentContainer, true, true)
			if err != nil {
				t.Log("Error listing all free parent container blocks: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			freeParentIPNets, err := blocksToIPNets(freeParentBlocks)
			if err != nil {
				t.Log("Error converting free parent blocks to IPNets: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			for _, f := range freeParentIPNets {
				for p := range subnetIPNetsByParent {
					if cidrIsWithin(f, p) {
						subnetIPNetsByParent[p] = append(subnetIPNetsByParent[p], f)
					}
				}
			}

			var cidrsToDisassociate []string
			for p, s := range subnetIPNetsByParent {
				if childCIDRsUseAllParentCIDRSpace(p, s) {
					blocksToDelete[p.String()] = parentContainer
					cidrsToDisassociate = append(cidrsToDisassociate, p.String())
				}
			}

			err = disassociateCIDRBlocks(awsctx, t, config.VPCID, cidrsToDisassociate)
			if err != nil {
				t.Log("Error disassociating CIDR blocks from VPC: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = deleteCIDRs(taskContext.ModelsManager, cidrsToDisassociate, vpc)
			if err != nil {
				t.Log("Error deleting CIDR blocks from vpc-conf: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		// Delete IPControl resources
		err = taskContext.IPAM.DeleteContainersAndBlocks(containersToDelete, t)
		if err != nil {
			t.Log("Error deleting containers and blocks: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		for cidr, container := range blocksToDelete {
			err = taskContext.IPAM.DeleteBlock(cidr, container, t)
			if err != nil {
				t.Log("Error deleting block %s: %s", cidr, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Save state
	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		awsctx.Fail("Error updating VPC state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if taskContext.Orchestration != nil {
		t.Log("Notifying orchestration engine of changed CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(vpc.AccountID, nil)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performAddZonedSubnetsTask(config *database.AddZonedSubnetsTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)

	awsctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
	}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.Region, config.VPCID)
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateZonedSubnets() || (vpc.State.VPCType.IsMigrating() && config.SubnetType != database.SubnetTypeFirewall) {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Name == "" {
		t.Log("VPC is missing a name")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Stack == "" {
		t.Log("VPC is missing a stack")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State.VPCType != database.VPCTypeLegacy && config.SubnetType == database.SubnetTypeTransitive {
		t.Log("Transitive type subnets can only be created in Legacy VPCs")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	// firewall validation
	msgs := []string{}
	if config.GroupName == "firewall" {
		if config.SubnetType != database.SubnetTypeFirewall {
			msgs = append(msgs, "A group name of 'firewall' is only allowed for Firewall subnet types")
		}

		if !(vpc.State.VPCType == database.VPCTypeMigratingV1ToV1Firewall) {
			msgs = append(msgs, "A group name of 'firewall' is not allowed")
		}

		if config.SubnetSize != ipcontrol.FirewallSubnetSize {
			msgs = append(msgs, fmt.Sprintf("Firewall subnets must be sized as a /%d", ipcontrol.FirewallSubnetSize))
		}

	} else {
		if config.SubnetType == database.SubnetTypeFirewall {
			msgs = append(msgs, "Firewall subnet types must have a group name of 'firewall'")
		}
	}
	if len(msgs) > 0 {
		t.Log(strings.Join(msgs, ". "))
		setStatus(t, database.TaskStatusFailed)
		return
	}

	allAZsWithGroupName := []string{}
	allAZs := []string{}

	for _, az := range vpc.State.AvailabilityZones.InOrder() {
		allAZs = append(allAZs, az.Name)
		for _, subnets := range az.Subnets {
			for _, subnet := range subnets {
				if subnet.GroupName == config.GroupName {
					allAZsWithGroupName = append(allAZsWithGroupName, az.Name)
					if !config.BeIdempotent {
						t.Log("The group name %q is already in use by another subnet group", config.GroupName)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				}
			}
		}
	}

	if config.BeIdempotent && len(allAZsWithGroupName) > 0 {
		if len(allAZs) == len(allAZsWithGroupName) {
			t.Log("A subnet with group name %s was found for each AZ", config.GroupName)
			setStatus(t, database.TaskStatusSuccessful)
			return
		} else {
			// we failed partway through adding and didn't roll back completely
			t.Log("BeIdempotent is set, but there isn't a subnet with group name %s in every AZ", config.GroupName)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	if config.GroupName == "public" || config.GroupName == "private" {
		t.Log("A group name of 'public' or 'private' is not allowed")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	azs := []string{}
	for azName := range vpc.State.AvailabilityZones {
		azs = append(azs, azName)
	}
	sort.Strings(azs)

	ctx := &ipcontrol.Context{
		LockSet: lockSet,
		IPAM:    taskContext.IPAM,
		AllocateConfig: database.AllocateConfig{
			VPCName:           vpc.Name,
			AccountID:         vpc.AccountID,
			Stack:             vpc.Stack,
			AWSRegion:         string(vpc.Region),
			AvailabilityZones: azs,
		},
		Logger: t,
	}

	if config.SubnetType == database.SubnetTypeUnroutable {
		ctx.VPCInfo = ipcontrol.VPCInfo{
			Name:              vpc.Name,
			Stack:             vpc.Stack,
			ResourceID:        vpc.ID,
			AvailabilityZones: azs,
		}

		describeVPCsOutput, err := awsctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: aws.StringSlice([]string{awsctx.VPCID}),
		})
		if err != nil {
			t.Log("Failed to describe VPC %s: %q", awsctx.VPCID, err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		if len(describeVPCsOutput.Vpcs) != 1 {
			t.Log("Failed to describe VPC %s", awsctx.VPCID)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		peeringCIDRs, err := awsctx.GetCIDRsForPeers()
		if err != nil {
			t.Log("Error getting CIDRS for peers: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}

		err = awsp.AddUnroutableSubnets(&ctx.VPCInfo, config.GroupName, describeVPCsOutput, peeringCIDRs)
		if err != nil {
			t.Log("Error getting unroutable subnets: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	} else {
		err = ctx.AddSubnets(config.SubnetType, config.SubnetSize, config.GroupName)
		if err != nil {
			t.Log("Error getting IPs for subnets: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	for _, cidr := range ctx.VPCInfo.NewCIDRs {
		err := awsctx.AddCIDRBlock(cidr)
		if err != nil {
			awsctx.Fail("Error associating new CIDR block: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
		err = taskContext.ModelsManager.InsertVPCCIDR(vpc.ID, vpc.Region, cidr, false)
		if err != nil {
			awsctx.Fail("Error inserting secondary CIDR for %s: %s", vpc.ID, err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	ctx.VPCInfo.ResourceID = config.VPCID // to write the VPC ID back to the new container
	err = awsctx.CreateSubnets(&ctx.VPCInfo)
	if err != nil {
		awsctx.Fail("Error creating AWS resources: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if config.SubnetType != database.SubnetTypeUnroutable {
		// Update containers with pointers back to AWS resource IDs
		err = ctx.AddReferencesToContainers()
		if err != nil {
			awsctx.Fail("Error updating IPControl containers with AWS resource IDs: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}
	for _, subnet := range ctx.VPCInfo.NewSubnets {
		az := vpc.State.GetAvailabilityZoneInfo(subnet.AvailabilityZone)
		az.Subnets[subnet.Type] = append(
			az.Subnets[subnet.Type],
			&database.SubnetInfo{
				SubnetID:  subnet.ResourceID,
				GroupName: config.GroupName,
			},
		)
	}
	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		awsctx.Fail("Error updating VPC state: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if taskContext.Orchestration != nil {
		var notification *orchestration.NewVPCNotification
		if config.JIRAIssueForComment != "" {
			notification = &orchestration.NewVPCNotification{
				VPCID:     config.VPCID,
				Region:    string(config.Region),
				JIRAIssue: config.JIRAIssueForComment,
			}
		}
		t.Log("Notifying orchestration engine of changed CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(vpc.AccountID, notification)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func recordNewSubnetsAndCidrs(vpcInfo ipcontrol.VPCInfo, vpc *database.VPC, mm database.ModelsManager) error {
	var err error
	for _, subnet := range vpcInfo.NewSubnets {
		az := vpc.State.GetAvailabilityZoneInfo(subnet.AvailabilityZone)
		az.Subnets[subnet.Type] = append(
			az.Subnets[subnet.Type],
			&database.SubnetInfo{
				SubnetID:  subnet.ResourceID,
				GroupName: subnet.GroupName,
			},
		)
	}
	for _, cidr := range vpcInfo.NewCIDRs {
		err = mm.InsertVPCCIDR(vpc.ID, vpc.Region, cidr, false)
		if err != nil {
			return fmt.Errorf("Error inserting secondary CIDR: %s", err)
		}
	}

	return nil
}

func (taskContext *TaskContext) performAddAvailabilityZoneTask(config *database.AddAvailabilityZoneTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)

	awsctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
	}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.Region, config.VPCID)
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanModifyAvailabilityZones() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Name == "" {
		t.Log("VPC is missing a name")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Stack == "" {
		t.Log("VPC is missing a stack")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	azs := []string{}
	for _, az := range vpc.State.AvailabilityZones.InOrder() {
		azs = append(azs, az.Name)
	}
	if stringInSlice(config.AZName, azs) {
		t.Log("AZ already configured, doing nothing")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	sort.Strings(azs)

	if len(azs) < 1 {
		t.Log("VPC has no AZs in the state")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	// Copy the first az
	firstAZ := azs[0]
	sourceAZ := vpc.State.AvailabilityZones[firstAZ]

	out, err := awsctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{vpc.ID}),
			},
			{
				Name:   aws.String("availability-zone"),
				Values: aws.StringSlice([]string{firstAZ}),
			},
		},
	})
	if err != nil {
		t.Log("Error getting list of subnets in VPC: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	newSubnets := make([]*ipcontrol.SubnetConfig, 0)
	oldSubnets := make(map[string]*ec2.Subnet)
	for _, subnet := range out.Subnets {
		oldSubnets[aws.StringValue(subnet.SubnetId)] = subnet
	}
	ctx := &ipcontrol.Context{
		LockSet: lockSet,
		IPAM:    taskContext.IPAM,
		AllocateConfig: database.AllocateConfig{
			VPCName:   vpc.Name,
			AccountID: vpc.AccountID,
			Stack:     vpc.Stack,
			AWSRegion: string(vpc.Region),
		},
		Logger: t,
	}

	hasUnroutables := false
	for _, subnetType := range database.AllSubnetTypes() {
		subnets, ok := sourceAZ.Subnets[subnetType]
		if !ok {
			continue
		}
		if subnetType == database.SubnetTypeUnroutable {
			hasUnroutables = true
		}
		var subnetTypeParentContainer string
		for _, subnet := range subnets {
			var subnetSize int
			if oldSubnet, ok := oldSubnets[subnet.SubnetID]; ok {
				cidr := aws.StringValue(oldSubnet.CidrBlock)
				_, ipNet, err := net.ParseCIDR(cidr)
				if err != nil {
					t.Log("Unable to parse subnet size from existing cidr %s: %s", cidr, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				subnetSize, _ = ipNet.Mask.Size()
			} else {
				t.Log("Subnet ID %s not found", subnet.SubnetID)
				continue
			}
			if subnetType != database.SubnetTypeUnroutable && subnetTypeParentContainer == "" {
				container, err := ctx.IPAM.GetSubnetContainer(subnet.SubnetID)
				if err != nil {
					t.Log("Unable to get source subnet's parent container: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				subnetTypeParentContainer = "/" + container.ParentName
			}

			// Generate the standard containers to catch the cases where there are sub-containers for private/public (cf. 506921813928)
			targetContainer := subnetTypeParentContainer
			if subnetType != database.SubnetTypeUnroutable {
				vpcContainerName := fmt.Sprintf("%s-%s", vpc.AccountID, vpc.Name)
				topLevelContainer, err := ipcontrol.GenerateTopLevelContainerNameBySubnetType(string(vpc.Region), vpc.Stack, subnetType)
				if err != nil {
					t.Log("Unable to generate top level container name for subnet type %s: %s", subnetType, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				vpcContainerPath := fmt.Sprintf("%s/%s", topLevelContainer, vpcContainerName)
				if strings.HasPrefix(subnetTypeParentContainer, vpcContainerPath) {
					targetContainer = vpcContainerPath
				}
			}

			newSubnet := &ipcontrol.SubnetConfig{
				SubnetType:      string(subnetType),
				SubnetSize:      subnetSize,
				GroupName:       subnet.GroupName,
				ParentContainer: targetContainer,
			}
			newSubnets = append(newSubnets, newSubnet)
		}
	}

	err = ctx.AddAZ(config.AZName, newSubnets)
	if err != nil {
		t.Log("Error creating new AZ subnets: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	for _, cidr := range ctx.VPCInfo.NewCIDRs {
		err := awsctx.AddCIDRBlock(cidr)
		if err != nil {
			awsctx.Fail("Error associating new CIDR block: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	ctx.VPCInfo.ResourceID = config.VPCID // to write the VPC ID back to the new container
	err = awsctx.CreateSubnets(&ctx.VPCInfo)
	if err != nil {
		awsctx.Fail("Error creating AWS resources: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	// Update containers with pointers back to AWS resource IDs
	err = ctx.AddReferencesToContainers()
	if err != nil {
		awsctx.Fail("Error updating IPControl containers with AWS resource IDs: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = recordNewSubnetsAndCidrs(ctx.VPCInfo, vpc, taskContext.ModelsManager)
	if err != nil {
		awsctx.Fail("Error saving VPC data: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if hasUnroutables {
		t.Log("Creating unroutable subnet(s)...")
		peeringCIDRs, err := awsctx.GetCIDRsForPeers()
		if err != nil {
			t.Log("Error getting CIDRs for peers: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
		describeVPCsOutput, err := awsctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: aws.StringSlice([]string{awsctx.VPCID}),
		})
		if err != nil {
			awsctx.Fail("Failed to describe VPC %s: %s", awsctx.VPCID, err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
		if len(describeVPCsOutput.Vpcs) != 1 {
			awsctx.Fail("Failed to describe VPC %s (returned vpcs %d != 1)", awsctx.VPCID, len(describeVPCsOutput.Vpcs))
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}

		for _, subnet := range newSubnets {
			if subnet.SubnetType == string(database.SubnetTypeUnroutable) {
				ctx.VPCInfo.Name = vpc.Name
				ctx.VPCInfo.AvailabilityZones = []string{config.AZName}
				err = awsp.AddUnroutableSubnets(&ctx.VPCInfo, subnet.GroupName, describeVPCsOutput, peeringCIDRs)
				if err != nil {
					awsctx.Fail("Error getting unroutable subnets: %s", err)
					ctx.DeleteIncompleteResources()
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
		for _, cidr := range ctx.VPCInfo.NewCIDRs {
			err := awsctx.AddCIDRBlock(cidr)
			if err != nil {
				awsctx.Fail("Error associating new CIDR block: %s", err)
				ctx.DeleteIncompleteResources()
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
		err = awsctx.CreateSubnets(&ctx.VPCInfo)
		if err != nil {
			awsctx.Fail("Error creating AWS resources: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
		err = recordNewSubnetsAndCidrs(ctx.VPCInfo, vpc, taskContext.ModelsManager)
		if err != nil {
			awsctx.Fail("Error saving VPC data: %s", err)
			ctx.DeleteIncompleteResources()
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		awsctx.Fail("Error updating VPC state: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if taskContext.Orchestration != nil {
		t.Log("Notifying orchestration engine of changed CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(vpc.AccountID, nil)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performRemoveAvailabilityZoneTask(config *database.RemoveAvailabilityZoneTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)

	awsctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
	}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.Region, config.VPCID)
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanModifyAvailabilityZones() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Name == "" {
		t.Log("VPC is missing a name")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Stack == "" {
		t.Log("VPC is missing a stack")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if _, ok := vpc.State.AvailabilityZones[config.AZName]; !ok {
		t.Log("No state for az %s", config.AZName)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	vpcOut, err := awsctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(config.VPCID)},
	})
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if len(vpcOut.Vpcs) != 1 {
		t.Log("Error getting VPC info: expected 1 VPC for VPC ID %s but got %d:", config.VPCID, len(vpcOut.Vpcs))
		setStatus(t, database.TaskStatusFailed)
		return
	}

	az := vpc.State.AvailabilityZones[config.AZName]
	// Find subnets to remove
	for _, subnetInfo := range az.Subnets {
		for _, subnet := range subnetInfo {
			if subnet.CustomRouteTableID != "" {
				if subnetSharesCustomRouteTable(subnet.SubnetID, subnet.CustomRouteTableID, vpc) {
					t.Log("V4 subnet %s shares custom route table %s", subnet.SubnetID, subnet.CustomRouteTableID)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
	}

	subnetOut, err := awsctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(config.VPCID)},
			},
			{
				Name:   aws.String("availability-zone"),
				Values: []*string{aws.String(config.AZName)},
			},
		},
	})
	if err != nil {
		t.Log("Error getting subnet info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	subnetCIDRsByID := make(map[string]string)
	subnetIDs := make([]string, 0)
	routeTableIDs := make([]string, 0)

	for _, s := range subnetOut.Subnets {
		subnetCIDRsByID[aws.StringValue(s.SubnetId)] = aws.StringValue(s.CidrBlock)
	}

	// parent name -> delete?
	deleteParent := make(map[string]bool)
	// subnetType -> subnet -> container
	containersBySubnetType := make(map[database.SubnetType]map[string]*models.WSContainer)
	parentContainerNamesInAZ := []string{}
	containersToDelete := []string{}
	// cidr -> container
	blocksToDelete := make(map[string]string)

	if az.PrivateRouteTableID != "" {
		routeTableIDs = append(routeTableIDs, az.PrivateRouteTableID)
	}
	if az.PublicRouteTableID != "" {
		routeTableIDs = append(routeTableIDs, az.PublicRouteTableID)
	}

	for subnetType, subnetInfo := range az.Subnets {
		containersBySubnetType[subnetType] = make(map[string]*models.WSContainer)
		// Find corresponding containers
		for _, subnet := range subnetInfo {
			subnetIDs = append(subnetIDs, subnet.SubnetID)

			if subnetType != database.SubnetTypeUnroutable {
				container, err := taskContext.IPAM.GetSubnetContainer(subnet.SubnetID)
				if err != nil {
					t.Log("Error finding subnet container: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				containersBySubnetType[subnetType][subnet.SubnetID] = container
				if subnet.CustomRouteTableID != "" && !stringInSlice(subnet.CustomRouteTableID, routeTableIDs) {
					routeTableIDs = append(routeTableIDs, subnet.CustomRouteTableID)
				}
			}
		}
	}
	// Gather quick list for matching
	for _, c1 := range containersBySubnetType {
		for _, c := range c1 {
			if !stringInSlice(c.ParentName, parentContainerNamesInAZ) {
				parentContainerNamesInAZ = append(parentContainerNamesInAZ, c.ParentName)
			}
			containerName := fmt.Sprintf("/%s/%s", c.ParentName, c.ContainerName)
			if !stringInSlice(containerName, containersToDelete) {
				containersToDelete = append(containersToDelete, containerName)
			}
		}
	}
	containerTree, err := getContainerTree(taskContext.IPAM, vpc.AccountID, config.VPCID)
	if err != nil {
		t.Log("Error finding VPC containers: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	for _, parentName := range parentContainerNamesInAZ {
		deleteParent[parentName] = parentWillBeEmptyAfterDeletions(containerTree, parentName, containersToDelete)
	}

	// Delete any CMSNet connections for these zones
	if taskContext.CMSNet.SupportsRegion(vpc.Region) {
		err := taskContext.DeleteCMSNetConfigurations(vpc, subnetIDs)
		if err != nil {
			t.Log("Error deleting CMSNet configurations: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	err = destroyNATGatewayResourcesInAZ(awsctx, vpcWriter, vpc, az)
	if err != nil {
		t.Log("Error destroying NAT gateway resources: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State.VPCType.HasFirewall() {
		err = deleteFirewallResourcesWithinAZ(awsctx, vpcWriter, vpc, config.AZName, az)
		if err != nil {
			t.Log("Error deleting network firewall resources in az %s: %s", config.AZName, err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	err = deleteSubnets(awsctx, subnetIDs)
	if err != nil {
		t.Log("Error deleting subnets: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = deleteRouteTables(awsctx, routeTableIDs)
	if err != nil {
		t.Log("Error deleting route tables: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	for _, routeTableID := range routeTableIDs {
		delete(vpc.State.RouteTables, routeTableID)
	}

	// Pre-flight check of all containers across subnets. This will fail if *any* subnet type has multiple discrete parent containers listed
	// or if there are no containers
	// With a well-formed DB, this should never happen, but we should favor bailing before we do any actual deletions
	for subnetType := range az.Subnets {
		if subnetType == database.SubnetTypeUnroutable {
			continue
		}
		parentContainer := ""
		for _, c := range containersBySubnetType[subnetType] {
			if parentContainer == "" {
				parentContainer = c.ParentName
			} else if parentContainer != c.ParentName {
				t.Log("Found multiple parent containers for subnet type %s: %q and %q", string(subnetType), parentContainer, c.ParentName)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
		if parentContainer == "" {
			t.Log("Unable to find a parent container for subnet type %s", string(subnetType))
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	// Delete containers
	for subnetType, subnetInfo := range az.Subnets {
		parentContainer := ""
		// Identify parent container
		for _, c := range containersBySubnetType[subnetType] {
			parentContainer = c.ParentName
			break
		}
		if subnetType == database.SubnetTypeUnroutable {
			var cidrsToDisassociate []string
			var supernetCIDR *net.IPNet

			for _, subnet := range subnetInfo {
				if cidr, ok := subnetCIDRsByID[subnet.SubnetID]; ok {
					nextSupernetCIDR, err := awsp.GetUnroutableSupernet(cidr)
					if err != nil {
						t.Log("Error getting supernet CIDR for %s: %s", cidr, err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					if supernetCIDR == nil {
						supernetCIDR = nextSupernetCIDR
					}

					if nextSupernetCIDR.String() != supernetCIDR.String() {
						t.Log("Multiple supernet CIDRs found in same unroutable group: %s - %s ", supernetCIDR.String(), nextSupernetCIDR.String())
						setStatus(t, database.TaskStatusFailed)
						return
					}

					if !stringInSlice(supernetCIDR.String(), cidrsToDisassociate) {
						cidrsToDisassociate = append(cidrsToDisassociate, supernetCIDR.String())
					}
				}
			}
			// are there any subnets from this /16 in another subnet group?
			for subnetID, subnetCIDR := range subnetCIDRsByID {
				if stringInSlice(subnetID, subnetIDs) {
					continue // already accounted for
				}

				ip, _, err := net.ParseCIDR(subnetCIDR)
				if err != nil {
					t.Log("Error parsing subnet CIDR for %s: %s", subnetCIDR, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

				if supernetCIDR.Contains(ip) {
					t.Log("Subnet CIDR %q is assigned to another subnet group", subnetCIDR)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}

			err = disassociateCIDRBlocks(awsctx, t, config.VPCID, cidrsToDisassociate)
			if err != nil {
				t.Log("Error disassociating CIDR blocks from VPC: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			for _, cidr := range cidrsToDisassociate {
				err = taskContext.ModelsManager.DeleteVPCCIDR(vpc.ID, vpc.Region, cidr)
				if err != nil {
					t.Log("Error deleting CIDR blocks from database: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		} else {
			subnetIPNetsByParent := make(map[*net.IPNet][]*net.IPNet)

			parentBlocks, err := taskContext.IPAM.ListBlocks("/"+parentContainer, false, false)
			if err != nil {
				t.Log("Error listing parent container blocks: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			parentIPNets, err := blocksToIPNets(parentBlocks)
			if err != nil {
				t.Log("Error converting parent blocks to IPNets: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			for _, n := range parentIPNets {
				subnetIPNetsByParent[n] = make([]*net.IPNet, 0)
			}

			if deleteParent[parentContainer] {
				if !stringInSlice(parentContainer, containersToDelete) {
					containersToDelete = append(containersToDelete, parentContainer)
				}

				// Disassociate all of the parent's CIDR blocks before deleting it.
				var parentCIDRs []string
				for _, p := range parentBlocks {
					parentCIDRs = append(parentCIDRs, fmt.Sprintf("%s/%s", p.BlockAddr, p.BlockSize))
				}
				err = disassociateCIDRBlocks(awsctx, t, config.VPCID, parentCIDRs)
				if err != nil {
					t.Log("Error disassociating CIDR blocks from VPC: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = deleteCIDRs(taskContext.ModelsManager, parentCIDRs, vpc)
				if err != nil {
					t.Log("Error deleting CIDR blocks from vpc-conf: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			} else {
				t.Log("Find blocks that will be deleted")
				// Find and disassociate the immediate parent block of the subnet blocks
				for _, container := range containersBySubnetType[subnetType] {
					subnetBlocks, err := taskContext.IPAM.ListBlocks(fmt.Sprintf("/%s/%s", container.ParentName, container.ContainerName), false, false)
					if err != nil {
						t.Log("Error listing subnet container blocks: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					subnetIPNets, err := blocksToIPNets(subnetBlocks)
					if err != nil {
						t.Log("Error converting subnet blocks to IPNets: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}

					for _, p := range parentIPNets {
						for _, s := range subnetIPNets {
							if cidrIsWithin(s, p) {
								subnetIPNetsByParent[p] = append(subnetIPNetsByParent[p], s)
							}
						}
					}
				}

				// Make sure any other child blocks of the parent block are free so the block can safely be deleted
				freeParentBlocks, err := taskContext.IPAM.ListBlocks("/"+parentContainer, true, true)
				if err != nil {
					t.Log("Error listing all free parent container blocks: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				freeParentIPNets, err := blocksToIPNets(freeParentBlocks)
				if err != nil {
					t.Log("Error converting free parent blocks to IPNets: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				for _, f := range freeParentIPNets {
					for p := range subnetIPNetsByParent {
						if cidrIsWithin(f, p) {
							subnetIPNetsByParent[p] = append(subnetIPNetsByParent[p], f)
						}
					}
				}

				var cidrsToDisassociate []string
				for p, s := range subnetIPNetsByParent {
					if childCIDRsUseAllParentCIDRSpace(p, s) {
						blocksToDelete[p.String()] = parentContainer
						cidrsToDisassociate = append(cidrsToDisassociate, p.String())
					}
				}

				err = disassociateCIDRBlocks(awsctx, t, config.VPCID, cidrsToDisassociate)
				if err != nil {
					t.Log("Error disassociating CIDR blocks from VPC: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = deleteCIDRs(taskContext.ModelsManager, cidrsToDisassociate, vpc)
				if err != nil {
					t.Log("Error deleting CIDR blocks from vpc-conf: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
	}
	// Delete IPControl resources
	err = taskContext.IPAM.DeleteContainersAndBlocks(containersToDelete, t)
	if err != nil {
		t.Log("Error deleting AZ containers and blocks: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	for cidr, container := range blocksToDelete {
		err = taskContext.IPAM.DeleteBlock(cidr, container, t)
		if err != nil {
			t.Log("Error deleting block %s: %s", cidr, err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	delete(vpc.State.AvailabilityZones, config.AZName)
	// Save state
	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		awsctx.Fail("Error updating VPC state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if taskContext.Orchestration != nil {
		var notification *orchestration.NewVPCNotification
		if config.JIRAIssueForComment != "" {
			notification = &orchestration.NewVPCNotification{
				VPCID:     config.VPCID,
				Region:    string(config.Region),
				JIRAIssue: jira.JiraURL(config.JIRAIssueForComment),
			}
		}
		t.Log("Notifying orchestration engine of changed CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(vpc.AccountID, notification)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performImportVPCTask(importConfig *database.ImportVPCTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Loading VPC info")

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            importConfig.VPCID,
	}

	resp, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(importConfig.VPCID)},
	})
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	vpcResp := resp.Vpcs[0]
	name := ""
	stack := ""
	hasFirewallTag := false
	for _, tag := range vpcResp.Tags {
		key := aws.StringValue(tag.Key)
		value := aws.StringValue(tag.Value)
		if key == "Name" {
			name = value
		} else if (key == "stack" && stack == "") || key == "vpc-conf-stack" {
			stack = value
		} else if key == awsp.FirewallTypeKey && value == awsp.FirewallTypeValue {
			hasFirewallTag = true
		}
	}

	vpc := &database.VPC{
		ID:        importConfig.VPCID,
		AccountID: importConfig.AccountID,
		Region:    importConfig.Region,
		Name:      name,
		Stack:     stack,
	}

	if vpc.Name == "" {
		t.Log("VPC must have a name to use this tool")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Stack == "" {
		t.Log("VPC must have a stack tag to use this tool")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	/*
		if vpc.Stack != "sandbox" && vpc.Stack != "test" && vpc.Stack != "dev" && vpc.Stack != "impl" && vpc.Stack != "prod" {
			t.Log("VPC does not comply with spec. VPC Stack is %q. Valid Stacks are: 'sandbox' 'test' 'dev' 'test' or 'prod'", vpc.Stack)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	*/
	//S.M.

	if vpc.Stack != "sandbox" && vpc.Stack != "test" && vpc.Stack != "dev" && vpc.Stack != "impl" && vpc.Stack != "mgmt" && vpc.Stack != "nonprod" && vpc.Stack != "qa" && vpc.Stack != "prod" {
		t.Log("VPC does not comply with spec. VPC Stack is %q. Valid Stacks are: 'sandbox' 'test' 'dev' 'test' 'mgmt' 'nonprod' 'qa' or 'prod'", vpc.Stack)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	_, err = taskContext.ModelsManager.CreateOrUpdateVPC(vpc)
	if err != nil {
		t.Log("Error syncing VPC with database: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, vpc.Region, vpc.ID)
	if err != nil {
		t.Log("Error getting operable VPC: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State != nil && !vpc.State.VPCType.CanImportVPC() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx.VPCName = vpc.Name
	ctx.VPCID = vpc.ID

	vpc.State = &database.VPCState{
		VPCType:           importConfig.VPCType,
		AvailabilityZones: map[string]*database.AvailabilityZoneInfra{},
		RouteTables:       map[string]*database.RouteTableInfo{},
	}

	if importConfig.VPCType == database.VPCTypeV1 && hasFirewallTag {
		vpc.State.VPCType = database.VPCTypeV1Firewall
	}

	config := &database.VPCConfig{}

	var subnetsByType map[database.SubnetType][]*ec2.Subnet
	if vpc.State.VPCType == database.VPCTypeLegacy {
		subnetsByType, err = ctx.GetLegacySubnets()
	} else {
		subnetsByType, err = ctx.GetSubnets()
	}
	if err != nil {
		t.Log("VPC does not comply with spec. %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State.VPCType.IsV1Variant() {
		// IGW and IGW RT
		igw, err := ctx.GetAttachedInternetGateway()
		if err != nil {
			t.Log("Error getting internet gateway: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		if igw == nil {
			igw, err = ctx.GetInternetGatewayByName(internetGatewayName(ctx.VPCName))
			if err != nil {
				t.Log("Error getting internet gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			if igw != nil && len(igw.Attachments) != 0 {
				igw = nil // attached to another VPC; forget it
			}
		}
		if igw != nil {
			vpc.State.InternetGateway.InternetGatewayID = aws.StringValue(igw.InternetGatewayId)
			for _, attachment := range igw.Attachments {
				if *attachment.VpcId == ctx.VPCID {
					vpc.State.InternetGateway.IsInternetGatewayAttached = true
					config.ConnectPublic = true
				}
			}
			if vpc.State.VPCType == database.VPCTypeV1Firewall {
				rt, err := ctx.GetRouteTableAssociatedWithIGW(aws.StringValue(igw.InternetGatewayId))
				if err != nil {
					t.Log("Error getting IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				if rt == nil {
					rt, err = ctx.GetRouteTableByName(igwRouteTableName(ctx.VPCName))
					if err != nil {
						t.Log("Error getting igw route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				}
				if rt != nil {
					vpc.State.InternetGateway.RouteTableID = aws.StringValue(rt.RouteTableId)
					igwRTInfo, err := createRouteTableInfo(rt, "", database.EdgeAssociationTypeIGW, aws.StringValue(rt.RouteTableId))
					if err != nil {
						t.Log("Error creating route table info for IGW route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					vpc.State.RouteTables[aws.StringValue(rt.RouteTableId)] = igwRTInfo

					for _, assn := range rt.Associations {
						gatewayID := aws.StringValue(assn.GatewayId)
						if gatewayID == vpc.State.InternetGateway.InternetGatewayID {
							vpc.State.InternetGateway.RouteTableAssociationID = aws.StringValue(assn.RouteTableAssociationId)
						}
					}
				}
			}
		}

		// Public subnets and RTs
		publicSubnets := subnetsByType[database.SubnetTypePublic]
		for _, subnet := range publicSubnets {
			groupName := "public"
			// If tagged with a group name explicitly, use instead of subnetType
			for _, tag := range subnet.Tags {
				if *tag.Key == "GroupName" {
					groupName = *tag.Value
				}
			}

			az := vpc.State.GetAvailabilityZoneInfo(*subnet.AvailabilityZone)
			az.Subnets[database.SubnetTypePublic] = append(
				az.Subnets[database.SubnetTypePublic],
				&database.SubnetInfo{
					SubnetID:  *subnet.SubnetId,
					GroupName: groupName,
				},
			)
		}

		if len(publicSubnets) > 0 {
			if vpc.State.VPCType == database.VPCTypeV1 {
				rt, err := ctx.GetRouteTableAssociatedWithSubnet(aws.StringValue(publicSubnets[0].SubnetId))
				if err != nil {
					t.Log("Error getting public route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				if rt == nil {
					rt, err = ctx.GetRouteTableByName(sharedPublicRouteTableName(ctx.VPCName))
					if err != nil {
						t.Log("Error getting public route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				}
				if rt != nil {
					rtID := aws.StringValue(rt.RouteTableId)

					if vpc.State.PublicRouteTableID != "" {
						if vpc.State.PublicRouteTableID != rtID {
							t.Log("VPC does not comply with spec. Multiple public subnets with different Route Tables")
							setStatus(t, database.TaskStatusFailed)
							return
						}
					} else {
						vpc.State.PublicRouteTableID = rtID
						publicRTInfo, err := createRouteTableInfo(rt, database.SubnetTypePublic, "", rtID)
						if err != nil {
							t.Log("Error creating route table info for public route table: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						vpc.State.RouteTables[rtID] = publicRTInfo
					}
					for _, assn := range rt.Associations {
						subnetID := aws.StringValue(assn.SubnetId)
						if subnetID == "" {
							// Skip implicit associations
							continue
						}
						publicSubnet, ok := selectBySubnetID(vpc.State, subnetID)
						if !ok {
							t.Log("Subnet %s is on public route table but is not marked as public", *assn.SubnetId)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						publicSubnet.RouteTableAssociationID = *assn.RouteTableAssociationId
					}
				}
			} else if vpc.State.VPCType == database.VPCTypeV1Firewall {
				for _, publicSubnet := range publicSubnets {
					az := aws.StringValue(publicSubnet.AvailabilityZone)
					rt, err := ctx.GetRouteTableAssociatedWithSubnet(aws.StringValue(publicSubnet.SubnetId))
					if err != nil {
						t.Log("Error getting public route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					if rt == nil {
						rt, err = ctx.GetRouteTableByName(routeTableName(ctx.VPCName, az, "", database.SubnetTypePublic))
						if err != nil {
							t.Log("Error getting public route table: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
					if rt != nil {
						rtID := aws.StringValue(rt.RouteTableId)

						if vpc.State.AvailabilityZones[az].PublicRouteTableID != "" {
							if vpc.State.AvailabilityZones[az].PublicRouteTableID != rtID {
								t.Log("VPC does not comply with spec. Multiple public subnets with different Route Tables for AZ %s", az)
								setStatus(t, database.TaskStatusFailed)
								return
							}
						} else {
							vpc.State.AvailabilityZones[az].PublicRouteTableID = rtID
							publicRTInfo, err := createRouteTableInfo(rt, database.SubnetTypePublic, "", rtID)
							if err != nil {
								t.Log("Error creating route table info for public route table for AZ %s: %s", az, err)
								setStatus(t, database.TaskStatusFailed)
								return
							}
							vpc.State.RouteTables[rtID] = publicRTInfo
						}
						for _, assn := range rt.Associations {
							subnetID := aws.StringValue(assn.SubnetId)
							if subnetID == "" {
								// Skip implicit associations
								continue
							}
							publicSubnet, ok := selectBySubnetID(vpc.State, subnetID)
							if !ok {
								t.Log("Subnet %s is on public route table but is not marked as public", *assn.SubnetId)
								setStatus(t, database.TaskStatusFailed)
								return
							}
							publicSubnet.RouteTableAssociationID = *assn.RouteTableAssociationId
						}
					}
				}
			}
		}

		// NAT gateways
		for _, subnet := range subnetsByType[database.SubnetTypePublic] {
			az := vpc.State.GetAvailabilityZoneInfo(*subnet.AvailabilityZone)

			ng, err := ctx.GetNATGatewayInSubnet(aws.StringValue(subnet.SubnetId))
			if err != nil {
				t.Log("Error getting NAT Gateway for AZ %s: %s", *subnet.AvailabilityZone, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			if ng != nil {
				if az.NATGateway.NATGatewayID != "" {
					if az.NATGateway.NATGatewayID != aws.StringValue(ng.NatGatewayId) {
						t.Log("VPC does not comply with spec. Multiple private subnets in AZ %s with different NAT Gateways", *subnet.AvailabilityZone)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				} else {
					az.NATGateway.NATGatewayID = aws.StringValue(ng.NatGatewayId)
					if len(ng.NatGatewayAddresses) == 0 {
						// This shouldn't happen
						setStatus(t, database.TaskStatusFailed)
						t.Log("No addresses on NAT Gateway for AZ %s: %s", aws.StringValue(subnet.AvailabilityZone), err)
						return
					}
					config.ConnectPrivate = true
					var eip *ec2.Address
					if ng != nil {
						eip, err = ctx.GetEIPByAllocationID(*ng.NatGatewayAddresses[0].AllocationId)
						if err != nil {
							t.Log("Error getting EIP for NAT Gateway %s: %s", aws.StringValue(ng.NatGatewayId), err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						if eip == nil {
							t.Log("NO EIP found for NAT Gateway %s: %s", aws.StringValue(ng.NatGatewayId), err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					} else {
						eip, err = ctx.GetEIPByName(eipName(ctx.VPCName, *subnet.AvailabilityZone))
						if err != nil {
							t.Log("Error getting EIP for AZ %s: %s", *subnet.AvailabilityZone, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
					if eip != nil {
						az.NATGateway.EIPID = *eip.AllocationId
					}
				}
			}
		}
	}

	// Firewall subnets and RT
	if vpc.State.VPCType == database.VPCTypeV1Firewall {
		firewallSubnets := subnetsByType[database.SubnetTypeFirewall]
		for _, subnet := range firewallSubnets {
			az := vpc.State.GetAvailabilityZoneInfo(*subnet.AvailabilityZone)
			az.Subnets[database.SubnetTypeFirewall] = append(
				az.Subnets[database.SubnetTypeFirewall],
				&database.SubnetInfo{
					SubnetID:  *subnet.SubnetId,
					GroupName: "firewall",
				},
			)
		}

		if len(firewallSubnets) > 0 {
			rt, err := ctx.GetRouteTableAssociatedWithSubnet(aws.StringValue(firewallSubnets[0].SubnetId))
			if err != nil {
				t.Log("Error getting firewall route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			if rt == nil {
				rt, err = ctx.GetRouteTableByName(sharedFirewallRouteTableName(ctx.VPCName))
				if err != nil {
					t.Log("Error getting firewall route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			if rt != nil {
				rtID := aws.StringValue(rt.RouteTableId)

				if vpc.State.FirewallRouteTableID != "" {
					if vpc.State.FirewallRouteTableID != rtID {
						t.Log("VPC does not comply with spec. Multiple firewall subnets with different Route Tables")
						setStatus(t, database.TaskStatusFailed)
						return
					}
				} else {
					vpc.State.FirewallRouteTableID = rtID
					firewallRTInfo, err := createRouteTableInfo(rt, database.SubnetTypeFirewall, "", rtID)
					if err != nil {
						t.Log("Error creating route table info for firewall route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					vpc.State.RouteTables[rtID] = firewallRTInfo
				}
				for _, assn := range rt.Associations {
					subnetID := aws.StringValue(assn.SubnetId)
					if subnetID == "" {
						// Skip implicit associations
						continue
					}
					firewallSubnet, ok := selectBySubnetID(vpc.State, subnetID)
					if !ok {
						t.Log("Subnet %s is on firewall route table but is not marked as firewall", *assn.SubnetId)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					firewallSubnet.RouteTableAssociationID = *assn.RouteTableAssociationId
				}
			}
		}
	}

	// Private/zoned subnets and RTs
	azHasPublicSubnet := make(map[string]bool)
	for _, subnet := range subnetsByType[database.SubnetTypePublic] {
		azHasPublicSubnet[aws.StringValue(subnet.AvailabilityZone)] = true
	}
	for subnetType, subnets := range subnetsByType {
		if (vpc.State.VPCType != database.VPCTypeLegacy && subnetType == database.SubnetTypePublic) || subnetType == database.SubnetTypeFirewall {
			//  handled separately
			continue
		}
		for _, subnet := range subnets {
			_, ok := azHasPublicSubnet[*subnet.AvailabilityZone]
			if !ok {
				t.Log("VPC does not comply with spec. There is a %s subnet in AZ %s but no public subnet", subnetType, *subnet.AvailabilityZone)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			// Fill in infra
			groupName := string(subnetType)

			// If tagged with a group name explicitly, use instead of subnetType
			for _, tag := range subnet.Tags {
				if *tag.Key == "GroupName" {
					groupName = *tag.Value
				}
			}
			if vpc.State.VPCType.IsV1Variant() {
				groupName = strings.ToLower(groupName)
			}
			pi := &database.SubnetInfo{
				SubnetID:  *subnet.SubnetId,
				GroupName: groupName,
			}
			az := vpc.State.GetAvailabilityZoneInfo(*subnet.AvailabilityZone)
			az.Subnets[subnetType] = append(az.Subnets[subnetType], pi)

			rt, err := ctx.GetRouteTableAssociatedWithSubnet(*subnet.SubnetId)
			if err != nil {
				t.Log("Error getting private route table for AZ %s: %s", *subnet.AvailabilityZone, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			if rt == nil {
				rt, err = ctx.GetRouteTableByName(routeTableName(ctx.VPCName, *subnet.AvailabilityZone, "", subnetType))
				if err != nil {
					t.Log("Error getting private route table for AZ %s: %s", *subnet.AvailabilityZone, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			if rt == nil {
				if vpc.State.VPCType == database.VPCTypeLegacy {
					t.Log("VPC does not comply with spec. No route table found for subnet %s", aws.StringValue(subnet.SubnetId))
					setStatus(t, database.TaskStatusFailed)
					return
				}
			} else {
				rtID := aws.StringValue(rt.RouteTableId)

				if subnetType == database.SubnetTypePrivate {
					if az.PrivateRouteTableID != "" {
						if az.PrivateRouteTableID != rtID {
							t.Log("VPC does not comply with spec. Multiple private subnets in AZ %s with different route tables", *subnet.AvailabilityZone)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					} else {
						az.PrivateRouteTableID = rtID
					}
				} else {
					pi.CustomRouteTableID = rtID
				}
				rtInfo, ok := vpc.State.RouteTables[aws.StringValue(rt.RouteTableId)]
				if ok {
					if subnetType != rtInfo.SubnetType {
						t.Log("VPC does not comply with spec. Subnet %s of type %s cannot share route table %s with subnet(s) of type %s", aws.StringValue(subnet.SubnetId), subnetType, rtID, rtInfo.SubnetType)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				} else {
					if vpc.State.VPCType == database.VPCTypeLegacy {
						// don't add routes
						rtInfo := &database.RouteTableInfo{
							SubnetType:   subnetType,
							RouteTableID: rtID,
						}
						vpc.State.RouteTables[rtID] = rtInfo
					} else {
						rtInfo, err := createRouteTableInfo(rt, subnetType, "", rtID)
						if err != nil {
							t.Log("Error creating route table info for %s route table in AZ %s: %s", subnetType, aws.StringValue(subnet.AvailabilityZone), err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						vpc.State.RouteTables[rtID] = rtInfo
					}
				}
				for _, assn := range rt.Associations {
					subnetID := aws.StringValue(assn.SubnetId)
					if subnetID == "" {
						// Skip implicit associations
						continue
					}
					if subnetID == *subnet.SubnetId {
						pi.RouteTableAssociationID = aws.StringValue(assn.RouteTableAssociationId)
					}
				}
			}
		}
	}

	// Firewall
	if vpc.State.VPCType == database.VPCTypeV1Firewall {
		out, err := ctx.NetworkFirewall().ListFirewalls(&networkfirewall.ListFirewallsInput{
			VpcIds: []*string{aws.String(ctx.VPCID)},
		})
		if err != nil {
			t.Log("Error listing firewalls: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		for _, fw := range out.Firewalls {
			name := aws.StringValue(fw.FirewallName)
			if name == ctx.FirewallName() {
				subnetIDs, err := ctx.GetSubnetAssociationsForFirewall()
				if err != nil {
					t.Log("Error getting subnet associations for firewall: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				vpc.State.Firewall = &database.Firewall{
					AssociatedSubnetIDs: subnetIDs,
				}
			}
		}
	}

	// Logging
	logGroupArn, err := ctx.CheckLogGroupExists(cloudwatchQueryLogDestination)
	if err != nil {
		t.Log("Error checking for query log group: %s", err)
		return
	}

	if logGroupArn != nil {
		qlcaOut, err := ctx.R53R().ListResolverQueryLogConfigAssociations(&route53resolver.ListResolverQueryLogConfigAssociationsInput{
			Filters: []*route53resolver.Filter{
				{
					Name:   aws.String("ResourceId"),
					Values: []*string{&vpc.ID},
				},
			},
		})
		if err != nil {
			t.Log("Error listing QueryLog Associations: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		if len(qlcaOut.ResolverQueryLogConfigAssociations) > 0 {
			for _, association := range qlcaOut.ResolverQueryLogConfigAssociations {
				configOut, err := ctx.R53R().GetResolverQueryLogConfig(&route53resolver.GetResolverQueryLogConfigInput{
					ResolverQueryLogConfigId: association.ResolverQueryLogConfigId,
				})
				if err != nil {
					t.Log("Error getting resolver query log config: %s", err)
					return
				}
				if aws.StringValue(configOut.ResolverQueryLogConfig.DestinationArn)+":*" == aws.StringValue(logGroupArn) {
					t.Log("Found Resolver Query Log Configuration %s", aws.StringValue(association.ResolverQueryLogConfigId))
					vpc.State.ResolverQueryLogConfigurationID = aws.StringValue(association.ResolverQueryLogConfigId)
					t.Log("Found Resolver Query Log Association %s", aws.StringValue(association.Id))
					vpc.State.ResolverQueryLogAssociationID = aws.StringValue(association.Id)
				} else {
					t.Log("Found non-importable query log configuration and association: %s / %s", aws.StringValue(association.ResolverQueryLogConfigId), aws.StringValue(association.Id))
				}
			}
		}
	}

	out, err := ctx.EC2().DescribeFlowLogs(&ec2.DescribeFlowLogsInput{
		Filter: []*ec2.Filter{
			{
				Name:   aws.String("resource-id"),
				Values: []*string{&vpc.ID},
			},
		},
	})
	if err != nil {
		t.Log("Error listing FlowLogs: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	cloudwatchRoleARNs := []string{}
	cloudwatchGroupNames := []string{}
	for _, config := range cloudwatchFlowLogConfigs() {
		cloudwatchRoleARNs = append(cloudwatchRoleARNs, roleARN(vpc.Region, vpc.AccountID, config.role))
		cloudwatchGroupNames = append(cloudwatchGroupNames, config.groupName)
	}
	for _, flowLog := range out.FlowLogs {
		if aws.StringValue(flowLog.LogDestinationType) == ec2.LogDestinationTypeS3 &&
			stringInSlice(aws.StringValue(flowLog.LogDestination), flowLogS3Destinations(vpc.AccountID, vpc.Region)) &&
			aws.StringValue(flowLog.TrafficType) == ec2.TrafficTypeAll {
			t.Log("Found S3 FlowLog %s", aws.StringValue(flowLog.FlowLogId))
			vpc.State.S3FlowLogID = aws.StringValue(flowLog.FlowLogId)
		} else if aws.StringValue(flowLog.LogDestinationType) == ec2.LogDestinationTypeCloudWatchLogs &&
			stringInSlice(aws.StringValue(flowLog.DeliverLogsPermissionArn), cloudwatchRoleARNs) &&
			stringInSlice(aws.StringValue(flowLog.LogGroupName), cloudwatchGroupNames) &&
			aws.StringValue(flowLog.TrafficType) == ec2.TrafficTypeAll {
			t.Log("Found CloudWatch Logs FlowLog %s", aws.StringValue(flowLog.FlowLogId))
			vpc.State.CloudWatchLogsFlowLogID = aws.StringValue(flowLog.FlowLogId)
		}
	}

	// Import TG attachments if they were tagged by VPC Conf
	output, err := ctx.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{&ctx.VPCID},
			},
			{
				Name:   aws.String("state"),
				Values: []*string{aws.String("available")},
			},
		},
	})
	if err != nil {
		t.Log("Error getting transit gateway VPC attachments: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if len(output.TransitGatewayVpcAttachments) > 0 {
		managedAttachmentsByID := make(map[uint64]*database.ManagedTransitGatewayAttachment)
		managedAttachments, err := taskContext.ModelsManager.GetManagedTransitGatewayAttachments()
		if err != nil {
			t.Log("Error getting transit gateway configuration info: %s", err)
			t.SetStatus(database.TaskStatusFailed)
			return
		}
		for _, ma := range managedAttachments {
			managedAttachmentsByID[ma.ID] = ma
		}

		for _, output := range output.TransitGatewayVpcAttachments {
			for _, tag := range output.Tags {
				if aws.StringValue(tag.Key) == mtgaIDTagKey {
					value := aws.StringValue(tag.Value)
					managedIDs, err := parseTGAttachmentTag(value)
					if err != nil {
						t.Log("Error parsing transit gateway attachment tag value %s: %s", value, err)
						t.SetStatus(database.TaskStatusFailed)
						return
					}
					if len(managedIDs) > 0 {
						outputTGID := aws.StringValue(output.TransitGatewayId)

						for _, managedID := range managedIDs {
							mtga, ok := managedAttachmentsByID[managedID]
							if ok {
								if mtga.TransitGatewayID != outputTGID {
									t.Log("Transit gateway ID %s for MTGA template %d doesn't match the tagged transit gateway attachment's transit gateway ID %s ", mtga.TransitGatewayID, managedID, outputTGID)
									t.SetStatus(database.TaskStatusFailed)
									return
								}
							} else {
								t.Log("No corresponding MTGA template found for MTGA tag value %d", managedID)
								t.SetStatus(database.TaskStatusFailed)
								return
							}
						}

						t.Log("Importing transit gateway attachment %s", aws.StringValue(output.TransitGatewayAttachmentId))

						attachment := &database.TransitGatewayAttachment{
							ManagedTransitGatewayAttachmentIDs: managedIDs,
							TransitGatewayID:                   outputTGID,
							TransitGatewayAttachmentID:         aws.StringValue(output.TransitGatewayAttachmentId),
							SubnetIDs:                          aws.StringValueSlice(output.SubnetIds),
						}
						vpc.State.TransitGatewayAttachments = append(vpc.State.TransitGatewayAttachments, attachment)
						err = vpcWriter.UpdateState(vpc.State)
						if err != nil {
							t.Log("Error updating database: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}

						config.ManagedTransitGatewayAttachmentIDs = append(config.ManagedTransitGatewayAttachmentIDs, managedIDs...)
						err = taskContext.ModelsManager.UpdateVPCConfig(importConfig.Region, importConfig.VPCID, *config)
						if err != nil {
							t.Log("Error updating database: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
				}
			}
		}
	}

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		t.Log("Error updating database: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	primaryCIDR := aws.StringValue(vpcResp.CidrBlock)
	for _, associationSet := range vpcResp.CidrBlockAssociationSet {
		if aws.StringValue(associationSet.CidrBlockState.State) != "associated" {
			continue
		}
		cidr := aws.StringValue(associationSet.CidrBlock)
		err = taskContext.ModelsManager.InsertVPCCIDR(vpc.ID, vpc.Region, cidr, cidr == primaryCIDR)
		if err != nil {
			t.Log("Error updating database CIDRs: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	err = taskContext.ModelsManager.UpdateVPCConfig(importConfig.Region, importConfig.VPCID, *config)
	if err != nil {
		t.Log("Error updating database: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifyAllSpec(), false)
	if err != nil {
		t.Log("Error verifying: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Successfully imported VPC %s", importConfig.VPCID)
	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performEstablishExceptionVPCTask(importConfig *database.EstablishExceptionVPCTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Loading VPC info")

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            importConfig.VPCID,
	}

	resp, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(importConfig.VPCID)},
	})
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	vpcResp := resp.Vpcs[0]
	name := ""
	stack := ""
	for _, tag := range vpcResp.Tags {
		key := aws.StringValue(tag.Key)
		value := aws.StringValue(tag.Value)
		if key == "Name" {
			name = value
		} else if (key == "stack" && stack == "") || key == "vpc-conf-stack" {
			stack = value
		}
	}

	vpc := &database.VPC{
		ID:        importConfig.VPCID,
		AccountID: importConfig.AccountID,
		Region:    importConfig.Region,
		Name:      name,
		Stack:     stack,
	}

	if vpc.Name == "" {
		t.Log("VPC must have a name to use this tool")
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.Stack == "" {
		t.Log("VPC must have a stack tag to use this tool")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	_, err = taskContext.ModelsManager.CreateOrUpdateVPC(vpc)
	if err != nil {
		t.Log("Error syncing VPC with database: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, vpc.Region, vpc.ID)
	if err != nil {
		t.Log("Error getting operable VPC: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	vpc.State = &database.VPCState{
		VPCType: database.VPCTypeException,
	}
	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		t.Log("Error updating database: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Successfully established VPC %s as exception", importConfig.VPCID)
	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performUnimportVPCTask(taskData *database.UnimportVPCTaskData) {
	t := taskContext.Task
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, taskData.Region, taskData.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = taskContext.ModelsManager.UpdateVPCConfig(taskData.Region, taskData.VPCID, database.VPCConfig{})
	if err != nil {
		t.Log("Error clearing VPC %s config: %s", taskData.VPCID, err)
	}
	err = vpcWriter.UpdateIssues([]*database.Issue{})
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = taskContext.ModelsManager.DeleteVPCCIDRs(taskData.VPCID, taskData.Region)
	if err != nil {
		t.Log("Error deleting VPC CIDRs: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = vpcWriter.UpdateState(nil)
	if err != nil {
		t.Log("Error clearing VPC %s state: %s", taskData.VPCID, err)
	}

	if vpc.State.VPCType == database.VPCTypeException {
		t.Log("Successfully removed VPC %s as exception", taskData.VPCID)
	} else {
		t.Log("Successfully unimported VPC %s", taskData.VPCID)
	}
	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performVerifyVPCTask(verifyConfig *database.VerifyVPCTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Verifying VPC state")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, database.Region(verifyConfig.Region), verifyConfig.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanVerifyVPC() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            verifyConfig.VPCID,
		VPCName:          vpc.Name,
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, verifyConfig.Spec, false)
	if err != nil {
		t.Log("Error verifying: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Verification successful")
	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performRepairVPCTask(repairConfig *database.RepairVPCTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Syncing state and fixing tags")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, repairConfig.Region, repairConfig.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanRepairVPC() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            repairConfig.VPCID,
		VPCName:          vpc.Name,
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, repairConfig.Spec, true)
	if err != nil {
		t.Log("Error syncing VPC state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Sync successful")
	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performCreateVPCTask(taskConfig *database.CreateVPCTaskData) {
	t := taskContext.Task

	var vpcRequest *database.VPCRequest
	var err error
	if taskConfig.VPCRequestID != nil {
		vpcRequest, err = taskContext.ModelsManager.GetVPCRequest(*taskConfig.VPCRequestID)
		if err != nil {
			t.Log("Error getting VPC request task: %s", err)
		}
	}
	vpcID, provisionErr := taskContext.createVPC(taskConfig)
	if provisionErr != nil {
		t.Log("%s", provisionErr)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpcRequest != nil {
		err = taskContext.ModelsManager.SetVPCRequestProvisionedVPC(vpcRequest.ID, database.Region(taskConfig.AWSRegion), vpcID)
		if err != nil {
			t.Log("Error updating database with provisioned VPC ID: %s", err)
		}
	}

	if taskContext.Orchestration != nil {
		var notification *orchestration.NewVPCNotification
		if taskConfig.JIRAIssueForComment != "" {
			notification = &orchestration.NewVPCNotification{
				VPCID:     vpcID,
				Region:    taskConfig.AWSRegion,
				JIRAIssue: jira.JiraURL(taskConfig.JIRAIssueForComment),
			}
		}
		t.Log("Notifying orchestration engine of new CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(taskConfig.AccountID, notification)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}
}

func (taskContext *TaskContext) createVPC(taskConfig *database.CreateVPCTaskData) (string, error) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet
	asUser := taskContext.AsUser

	setStatus(t, database.TaskStatusInProgress)

	// Check that we aren't asking for more subnets than there are AZs.
	awsctx := awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
	}
	azOut, err := awsctx.EC2().DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
	azs := make([]string, 0)
	if err != nil {
		return "", fmt.Errorf("Error getting list of availability zones in your region: %s\n", err)
	}
	for _, az := range azOut.AvailabilityZones {
		azs = append(azs, *az.ZoneName)
	}
	taskConfig.AvailabilityZones = azs

	if taskConfig.NumPrivateSubnets > len(azs) || taskConfig.NumPublicSubnets > len(azs) {
		return "", fmt.Errorf("Too many AZs requested. This region has only %d AZs available.\n", len(azs))
	}

	if taskConfig.NumPrivateSubnets > taskConfig.NumPublicSubnets {
		return "", fmt.Errorf("There must be at least as many public subnets as private subnets.\n")
	}

	ctx := &ipcontrol.Context{
		LockSet:        lockSet,
		IPAM:           taskContext.IPAM,
		AllocateConfig: taskConfig.AllocateConfig,
		Logger:         t,
	}

	// Allocate IP space from IPControl
	err = ctx.Allocate()
	if err != nil {
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error creating entries in IPControl: %s", err)
	}
	if taskConfig.IsDefaultDedicated {
		ctx.VPCInfo.Tenancy = "dedicated"
	} else {
		ctx.VPCInfo.Tenancy = "default"
	}

	// Create resources in AWS
	err = awsctx.CreateVPC(&ctx.VPCInfo)
	if err != nil {
		awsctx.Fail("Error creating AWS resources: %s", err)
		ctx.DeleteIncompleteResources()
		setStatus(t, database.TaskStatusFailed)
		return "", fmt.Errorf("Error creating AWS resources: %s", err)
	}
	err = awsctx.CreateSubnets(&ctx.VPCInfo)
	if err != nil {
		ctx.DeleteIncompleteResources()
		awsctx.Fail("Error creating AWS resources: %s", err)
		return "", fmt.Errorf("Error creating AWS resources: %s", err)
	}

	// Update containers with pointers back to AWS resource IDs
	err = ctx.AddReferencesToContainers()
	if err != nil {
		awsctx.Fail("Error updating IPControl containers with AWS resource IDs: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error updating IPControl containers with AWS resource IDs: %s", err)
	}

	// Acquire a lock before adding the VPC to the database so that no other tasks attempt
	// to do anything with it before we finish initializing the state.
	err = lockSet.AcquireAdditionalLock(database.TargetVPC(ctx.VPCInfo.ResourceID))
	if err != nil {
		awsctx.Fail("Error getting VPC operations lock: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error getting VPC operations lock: %s", err)
	}

	region := database.Region(taskConfig.AWSRegion)
	vpc := &database.VPC{
		AccountID: taskConfig.AccountID,
		Region:    region,
		ID:        ctx.VPCInfo.ResourceID,
		Name:      ctx.VPCName,
		Stack:     taskConfig.Stack,
	}
	_, err = taskContext.ModelsManager.CreateOrUpdateVPC(vpc)
	if err != nil {
		awsctx.Fail("Error syncing VPC with database: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error syncing VPC with database: %s", err)
	}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, vpc.Region, vpc.ID)
	if err != nil {
		awsctx.Fail("Error getting VPC from database: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error getting VPC from database: %s", err)
	}

	vpc.State = &database.VPCState{}
	// default VPCType is database.VPCTypeV1
	if taskConfig.AddFirewall {
		vpc.State.VPCType = database.VPCTypeV1Firewall
	}
	vpc.State.AvailabilityZones = map[string]*database.AvailabilityZoneInfra{}
	vpc.State.RouteTables = map[string]*database.RouteTableInfo{}
	for _, subnet := range ctx.VPCInfo.NewSubnets {
		az := vpc.State.GetAvailabilityZoneInfo(subnet.AvailabilityZone)
		az.Subnets[subnet.Type] = append(
			az.Subnets[subnet.Type],
			&database.SubnetInfo{
				SubnetID:  subnet.ResourceID,
				GroupName: strings.ToLower(string(subnet.Type)),
			},
		)
	}

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		awsctx.Fail("Error updating VPC state: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error updating VPC state: %s", err)
	}

	// if there are multiple CIDRs during create assume that index 0 is the primary
	primaryCIDR := ctx.VPCInfo.NewCIDRs[0]
	for _, cidr := range ctx.VPCInfo.NewCIDRs {
		err = taskContext.ModelsManager.InsertVPCCIDR(vpc.ID, vpc.Region, cidr, cidr == primaryCIDR)
		if err != nil {
			awsctx.Fail("Failed to insert VPC CIDR: %s", err)
			ctx.DeleteIncompleteResources()
			return "", fmt.Errorf("Failed to insert VPC CIDR: %s", err)
		}
	}

	defaultConfig, err := taskContext.ModelsManager.GetDefaultVPCConfig(vpc.Region)
	if err != nil {
		awsctx.Fail("Error getting default VPC config: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error getting default VPC config: %s", err)
	}
	err = taskContext.ModelsManager.UpdateVPCConfig(vpc.Region, vpc.ID, *defaultConfig)
	if err != nil {
		awsctx.Fail("Error updating VPC config: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error updating VPC config: %s", err)
	}

	// update networking task is dependent on current task
	networkConfig := &database.UpdateNetworkingTaskData{NetworkingConfig: database.NetworkingConfig{
		ConnectPublic:                      defaultConfig.ConnectPublic,
		ConnectPrivate:                     defaultConfig.ConnectPrivate,
		ManagedTransitGatewayAttachmentIDs: defaultConfig.ManagedTransitGatewayAttachmentIDs,
	}}
	networkConfig.AWSRegion = region
	networkConfig.VPCID = vpc.ID
	networkTaskData := &database.TaskData{
		UpdateNetworkingTaskData: networkConfig,
		AsUser:                   asUser,
	}
	taskBytes, err := json.Marshal(networkTaskData)
	if err != nil {
		awsctx.Fail("Error marshalling network task data: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error marshalling network task data: %s", err)
	}
	taskName := fmt.Sprintf("Update VPC %s networking", vpc.ID)
	updateNetworkingTask, err := taskContext.TaskDatabase.AddDependentVPCTask(vpc.AccountID, vpc.ID, taskName, taskBytes, database.TaskStatusQueued, t.GetID(), nil)
	if err != nil {
		awsctx.Fail("Error adding task: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error adding task: %s", err)
	}

	// subsequent tasks are dependent on update networking task
	_, err = scheduleVPCTasks(taskContext.ModelsManager, taskContext.TaskDatabase, vpc.Region, vpc.AccountID, vpc.ID, asUser, database.TaskTypeLogging|database.TaskTypeSecurityGroups|database.TaskTypeResolverRules, database.VerifySpec{}, updateNetworkingTask, nil)
	if err != nil {
		awsctx.Fail("Error scheduling follow-up tasks: %s", err)
		ctx.DeleteIncompleteResources()
		return "", fmt.Errorf("Error scheduling follow-up tasks: %s", err)
	}

	setStatus(t, database.TaskStatusSuccessful)

	return ctx.VPCInfo.ResourceID, nil
}

type cloudwatchFlowLogConfig struct {
	role, groupName string
}

// The first entry is preferred but the second one may be imported,
// or used if the the destination group of the first is not present.
func cloudwatchFlowLogConfigs() []cloudwatchFlowLogConfig {
	return []cloudwatchFlowLogConfig{
		{
			role:      "cms-cloud-logging-flowlogs-cloudwatch-role",
			groupName: "cms-cloud-vpc-flowlogs",
		},
		{
			role:      "cms-cloud-cloudwatch-flowlogs-role",
			groupName: "vpc-flowlogs",
		},
	}
}

// The first entry is preferred but the second one may be imported.
func flowLogS3Destinations(accountID string, region database.Region) []string {
	return []string{
		fmt.Sprintf("%s:s3:::cms-cloud-%s-%s/", arnPrefix(string(region)), accountID, region),
		fmt.Sprintf("%s:s3:::%s/", arnPrefix(string(region)), accountID),
	}
}

// The desired flow log format
const flowLogFormat = "${version} ${account-id} ${action} ${bytes} ${dstaddr} ${dstport} ${end} ${instance-id} ${interface-id} ${log-status} ${packets} ${pkt-dstaddr} ${pkt-srcaddr} ${protocol} ${srcaddr} ${srcport} ${start} ${subnet-id} ${tcp-flags} ${type} ${version} ${vpc-id} ${az-id} ${flow-direction} ${pkt-dst-aws-service} ${pkt-src-aws-service} ${region} ${sublocation-id} ${sublocation-type} ${traffic-path} ${type}"

// Where to send VPC query logs
const cloudwatchQueryLogDestination = "cms-cloud-vpc-querylogs"

func (taskContext *TaskContext) performDeleteVPCTask(taskData *database.DeleteVPCTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, taskData.Region, taskData.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanDeleteVPC() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            taskData.VPCID,
	}
	subnets, err := ctx.GetSubnets()
	if err != nil {
		t.Log("Error listing subnets: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	subnetIDs := func(in map[database.SubnetType][]*ec2.Subnet) []string {
		out := make([]string, 0)
		for _, subnetsByType := range in {
			for _, subnet := range subnetsByType {
				if !stringInSlice(*subnet.SubnetId, out) {
					out = append(out, *subnet.SubnetId)
				}
			}
		}
		return out
	}(subnets)
	if taskContext.CMSNet.SupportsRegion(taskData.Region) {
		err := taskContext.DeleteCMSNetConfigurations(vpc, subnetIDs)
		if err != nil {
			t.Log("Error deleting CMSNet configurations: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	// Delete route53resolver query log configurations and associations
	err = ctx.DeleteAllResolverRuleQueryLogs()
	if err != nil {
		t.Log("Error deleting query log configurations: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	// Delete networking
	// TODO: unattached EIPs or IGWs will not get deleted.
	err = ctx.DeleteAllRouteTables()
	if err != nil {
		t.Log("Error deleting route tables: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = ctx.DeleteAllSecurityGroups()
	if err != nil {
		t.Log("Error deleting security groups: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = ctx.DeleteAllNATGateways()
	if err != nil {
		t.Log("Error deleting NAT Gateways: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = ctx.DeleteAllInternetGateways()
	if err != nil {
		t.Log("Error deleting Internet Gateways: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = ctx.DeleteAllTransitGatewayVPCAttachments()
	if err != nil {
		t.Log("Error deleting Transit Gateway Attachments: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = ctx.DeleteAllPeeringConnections()
	if err != nil {
		t.Log("Error deleting Peering Connections: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State.VPCType.HasFirewall() {
		err = ctx.DeleteFirewallResources()
		if err != nil {
			t.Log("Error deleting firewall resources: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}
	ec2svc := ctx.EC2()
	for _, subnets := range subnets {
		for _, subnet := range subnets {
			subnetID := aws.StringValue(subnet.SubnetId)
			_, err := ec2svc.DeleteSubnet(&ec2.DeleteSubnetInput{SubnetId: &subnetID})
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == "InvalidSubnetID.NotFound" {
						t.Log("Subnet %s not found", subnetID)
						continue
					}
				}
				t.Log("Error deleting subnet %s: %s\n", subnetID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			} else {
				t.Log("Deleted subnet %s", subnetID)
			}
		}
	}
	_, err = ec2svc.DeleteVpc(&ec2.DeleteVpcInput{VpcId: &taskData.VPCID})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "InvalidVpcID.NotFound" {
			t.Log("VPC %s not found", taskData.VPCID)
		} else {
			t.Log("Error deleting VPC %s: %s\n", taskData.VPCID, err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	} else {
		t.Log("Deleted VPC %s", taskData.VPCID)
	}

	err = taskContext.IPAM.DeleteContainersAndBlocksForVPC(taskData.AccountID, taskData.VPCID, t)
	if err != nil {
		t.Log("Error deleting containers: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = taskContext.ModelsManager.DeleteVPCCIDRs(taskData.VPCID, taskData.Region)
	if err != nil {
		t.Log("Error deleting CIDRs: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	err = taskContext.ModelsManager.UpdateVPCConfig(taskData.Region, taskData.VPCID, database.VPCConfig{})
	if err != nil {
		t.Log("Error clearing VPC %s config: %s", taskData.VPCID, err)
	}
	err = vpcWriter.UpdateState(&database.VPCState{})
	if err != nil {
		t.Log("Error clearing VPC %s state: %s", taskData.VPCID, err)
	}
	err = vpcWriter.MarkAsDeleted()
	if err != nil {
		t.Log("Error marking VPC %s as deleted: %s", taskData.VPCID, err)
	}

	if taskContext.Orchestration != nil {
		t.Log("Notifying orchestration engine of changed CIDRs")
		err := taskContext.Orchestration.NotifyCIDRsChanged(taskData.AccountID, nil)
		if err != nil {
			t.Log("Error notifying orchestration engine of new CIDRs: %s", err)
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

// Given a route table ID and list of routes for that route table, add a new route or
// update the existing route for the given destination.
// If desired is nil then any existing route for the destination will be deleted.
func setRoute(ctx *awsp.Context, routeTableID, destination string, current []*database.RouteInfo, desired *database.RouteInfo) ([]*database.RouteInfo, error) {
	var destinationCIDR *string
	var destinationPLID *string

	if awsp.IsPrefixListID(destination) {
		destinationPLID = &destination
	} else {
		destinationCIDR = &destination
	}

	if desired != nil {
		desired.Destination = destination
	}

	foundInAWS, err := ctx.LocalRouteWithDestinationExistsOnRouteTable(destination, routeTableID)
	if err != nil {
		return nil, fmt.Errorf("Error checking if route with destination %s exists on route table %s: %s", destination, routeTableID, err)
	}

	foundInState := false
	for idx, info := range current {
		if info.Destination == destination {
			foundInState = true
			if desired == nil {
				current = append(current[:idx], current[idx+1:]...)
				ctx.Log("Deleting route for %s on route table %s", destination, routeTableID)
				_, err := ctx.EC2().DeleteRoute(&ec2.DeleteRouteInput{
					RouteTableId:            &routeTableID,
					DestinationCidrBlock:    destinationCIDR,
					DestinationPrefixListId: destinationPLID,
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "InvalidRoute.NotFound" { // this is an okay error - route shouldn't exist when we're through
						ctx.Log("Route %s was not found, continuing", destination)
						continue
					}
					return nil, err
				}
			} else if info.InternetGatewayID != desired.InternetGatewayID || info.NATGatewayID != desired.NATGatewayID || info.TransitGatewayID != desired.TransitGatewayID || info.PeeringConnectionID != desired.PeeringConnectionID || info.VPCEndpointID != desired.VPCEndpointID {
				ctx.Log("Updating route %s -> %s%s%s%s%s on route table %s", destination, desired.NATGatewayID, desired.InternetGatewayID, desired.TransitGatewayID, desired.PeeringConnectionID, desired.VPCEndpointID, routeTableID)
				input := &ec2.ReplaceRouteInput{
					RouteTableId:            &routeTableID,
					DestinationCidrBlock:    destinationCIDR,
					DestinationPrefixListId: destinationPLID,
					NatGatewayId:            &desired.NATGatewayID,
					GatewayId:               &desired.InternetGatewayID,
					TransitGatewayId:        &desired.TransitGatewayID,
					VpcPeeringConnectionId:  &desired.PeeringConnectionID,
				}
				// AWS errors if this field is specified in input but empty
				if desired.VPCEndpointID != "" {
					input.VpcEndpointId = &desired.VPCEndpointID
				}
				_, err := ctx.EC2().ReplaceRoute(input)
				if err != nil {
					return nil, err
				}
				current[idx] = desired
			}
		}
	}

	if !foundInState && desired != nil {
		if foundInAWS {
			// a route with the desired destination exists already, but it's not in VPC Conf state.
			// This happens when we are setting routes for firewall endpoints on the IGW route table and the desired destination matches an existing 'local' route that AWS creates by default when creating the route table
			ctx.Log("Updating route %s -> %s%s%s%s%s on route table %s", destination, desired.NATGatewayID, desired.InternetGatewayID, desired.TransitGatewayID, desired.PeeringConnectionID, desired.VPCEndpointID, routeTableID)
			input := &ec2.ReplaceRouteInput{
				RouteTableId:            &routeTableID,
				DestinationCidrBlock:    destinationCIDR,
				DestinationPrefixListId: destinationPLID,
				NatGatewayId:            &desired.NATGatewayID,
				GatewayId:               &desired.InternetGatewayID,
				TransitGatewayId:        &desired.TransitGatewayID,
				VpcPeeringConnectionId:  &desired.PeeringConnectionID,
			}
			// AWS errors if this field is specified in input but empty
			if desired.VPCEndpointID != "" {
				input.VpcEndpointId = &desired.VPCEndpointID
			}
			// AWS errors if these fields are empty string
			if desired.NATGatewayID == "" {
				input.NatGatewayId = nil
			}
			if desired.InternetGatewayID == "" {
				input.GatewayId = nil
			}
			if desired.TransitGatewayID == "" {
				input.TransitGatewayId = nil
			}
			if desired.PeeringConnectionID == "" {
				input.VpcPeeringConnectionId = nil
			}
			// AWS errors if these fields are empty string ends
			_, err := ctx.EC2().ReplaceRoute(input)
			if err != nil {
				return nil, err
			}
		} else {
			ctx.Log("Creating route %s -> %s%s%s%s%s on route table %s", destination, desired.NATGatewayID, desired.InternetGatewayID, desired.TransitGatewayID, desired.PeeringConnectionID, desired.VPCEndpointID, routeTableID)
			input := &ec2.CreateRouteInput{
				RouteTableId:            &routeTableID,
				DestinationCidrBlock:    destinationCIDR,
				DestinationPrefixListId: destinationPLID,
				NatGatewayId:            &desired.NATGatewayID,
				GatewayId:               &desired.InternetGatewayID,
				TransitGatewayId:        &desired.TransitGatewayID,
				VpcPeeringConnectionId:  &desired.PeeringConnectionID,
			}
			// AWS errors if this field is specified in input but empty
			if desired.VPCEndpointID != "" {
				input.VpcEndpointId = &desired.VPCEndpointID
			}
			// AWS errors if these fields are empty string
			if desired.NATGatewayID == "" {
				input.NatGatewayId = nil
			}
			if desired.InternetGatewayID == "" {
				input.GatewayId = nil
			}
			if desired.TransitGatewayID == "" {
				input.TransitGatewayId = nil
			}
			if desired.PeeringConnectionID == "" {
				input.VpcPeeringConnectionId = nil
			}
			// AWS errors if these fields are empty string ends
			_, err := ctx.EC2().CreateRoute(input)
			if err != nil {
				return nil, err
			}
		}

		current = append(current, desired)
	}
	return current, nil
}

// Calls setRoute for every private (common or custom) route table
func setRouteAllNonPublic(ctx *awsp.Context, az *database.AvailabilityZoneInfra, vpc *database.VPC, destination string, desired *database.RouteInfo) error {
	var err error
	privateRT, ok := vpc.State.RouteTables[az.PrivateRouteTableID]
	if !ok {
		return fmt.Errorf("Error updating route table %s: no private route table info found", az.PrivateRouteTableID)
	}
	privateRT.Routes, err = setRoute(ctx, az.PrivateRouteTableID, destination, privateRT.Routes, desired)
	if err != nil {
		return fmt.Errorf("Error updating route table %s: %s", az.PrivateRouteTableID, err)
	}
	for _, subnets := range az.Subnets {
		for _, subnet := range subnets {
			if subnet.CustomRouteTableID != "" {
				customRT, ok := vpc.State.RouteTables[subnet.CustomRouteTableID]
				if !ok {
					return fmt.Errorf("Error updating route table %s: no custom route table info found", subnet.CustomRouteTableID)
				}
				customRT.Routes, err = setRoute(ctx, subnet.CustomRouteTableID, destination, customRT.Routes, desired)
				if err != nil {
					return fmt.Errorf("Error updating route table %s: %s", subnet.CustomRouteTableID, err)
				}
			}
		}
	}
	return nil
}

func subnetSharesCustomRouteTable(subnetID string, rtID string, vpc *database.VPC) bool {
	for _, az := range vpc.State.AvailabilityZones {
		for _, subnets := range az.Subnets {
			for _, subnet := range subnets {
				if subnet.SubnetID == subnetID {
					continue
				}
				if subnet.CustomRouteTableID == rtID {
					return true
				}
			}
		}
	}
	return false
}

// Combines information frame state and config with info from EC2 API
type peeringConnection struct {
	State          *database.PeeringConnection
	Config         *database.PeeringConnectionConfig
	OtherVPC       *database.VPC
	OtherVPCWriter database.VPCWriter
	OtherCTX       *awsp.Context
	// For routes:
	SubnetIDs     []string
	OtherVPCCIDRs []string
}

func handlePeeringConnections(
	lockSet database.LockSet,
	ctx *awsp.Context,
	vpc *database.VPC,
	vpcWriter database.VPCWriter,
	networkConfig *database.UpdateNetworkingTaskData,
	modelsManager database.ModelsManager,
	getContext func(region database.Region, accountID string) (*awsp.Context, error)) ([]*peeringConnection, error) {

	peeringConnections := []*peeringConnection{}
	for _, pcState := range vpc.State.PeeringConnections {
		peeringConnections = append(peeringConnections, &peeringConnection{
			State: pcState,
		})
	}
	for _, pcConfig := range networkConfig.NetworkingConfig.PeeringConnections {
		found := false
		for _, pc := range peeringConnections {
			if pc.State == nil {
				continue
			}
			if pcConfig.IsRequester && pc.State.RequesterVPCID == vpc.ID && pc.State.RequesterRegion == vpc.Region && pc.State.AccepterVPCID == pcConfig.OtherVPCID && pc.State.AccepterRegion == pcConfig.OtherVPCRegion {
				found = true
				pc.Config = pcConfig
				break
			}
			if !pcConfig.IsRequester && pc.State.AccepterVPCID == vpc.ID && pc.State.AccepterRegion == vpc.Region && pc.State.RequesterVPCID == pcConfig.OtherVPCID && pc.State.RequesterRegion == pcConfig.OtherVPCRegion {
				found = true
				pc.Config = pcConfig
				break
			}
		}
		if !found {
			peeringConnections = append(peeringConnections, &peeringConnection{
				Config: pcConfig,
			})
		}
	}
	// Delete peering connections in state that are no longer configured
	deleteRoutes := func(ctx *awsp.Context, vpc *database.VPC, vpcWriter database.VPCWriter, pcxID string) error {
		// Delete all routes for the given peering connection
		for _, az := range vpc.State.AvailabilityZones {
			for subnetType, subnets := range az.Subnets {
				for _, subnet := range subnets {
					var rtID string

					if subnet.CustomRouteTableID != "" {
						rtID = subnet.CustomRouteTableID
					} else if subnetType == database.SubnetTypePublic {
						if vpc.State.VPCType.HasFirewall() {
							rtID = az.PublicRouteTableID
						} else {
							rtID = vpc.State.PublicRouteTableID
						}
					} else {
						rtID = az.PrivateRouteTableID
					}
					if rtID == "" {
						return fmt.Errorf("No route table for subnet %s", subnet.SubnetID)
					}

					rt, ok := vpc.State.RouteTables[rtID]
					if !ok {
						return fmt.Errorf("Route table %s missing from state", rtID)
					}
					existingRoutes := append([]*database.RouteInfo{}, rt.Routes...) // copy
					for _, route := range existingRoutes {
						if route.PeeringConnectionID == pcxID {
							var err error
							rt.Routes, err = setRoute(ctx, rtID, route.Destination, rt.Routes, nil)
							if err != nil {
								return fmt.Errorf("Error updating route table %s: %s", rtID, err)
							}
							err = vpcWriter.UpdateState(vpc.State)
							if err != nil {
								return fmt.Errorf("Error updating state: %s", err)
							}
						}
					}
				}
			}
		}
		return nil
	}
	keepPeeringConnections := []*peeringConnection{}
	for _, pc := range peeringConnections {
		var err error
		// Get vpc object from database
		if pc.Config != nil {
			pc.OtherVPC, pc.OtherVPCWriter, err = modelsManager.GetOperableVPC(lockSet, pc.Config.OtherVPCRegion, pc.Config.OtherVPCID)
		} else if pc.State.RequesterVPCID == vpc.ID && pc.State.RequesterRegion == vpc.Region {
			pc.OtherVPC, pc.OtherVPCWriter, err = modelsManager.GetOperableVPC(lockSet, pc.State.AccepterRegion, pc.State.AccepterVPCID)
		} else {
			pc.OtherVPC, pc.OtherVPCWriter, err = modelsManager.GetOperableVPC(lockSet, pc.State.RequesterRegion, pc.State.RequesterVPCID)
		}
		if err != nil {
			return nil, fmt.Errorf("Error looking up VPC for peering connection %#v: %s", pc, err)
		}
		pc.OtherCTX, err = getContext(pc.OtherVPC.Region, pc.OtherVPC.AccountID)
		if err != nil {
			return nil, fmt.Errorf("Error getting context for VPC %s: %s", pc.OtherVPC.ID, err)
		}

		// Delete peering connection if it's no longer configured
		if pc.Config != nil {
			keepPeeringConnections = append(keepPeeringConnections, pc)
		} else if pc.State.PeeringConnectionID != "" {
			// Delete any routes to the peering connection
			err := deleteRoutes(ctx, vpc, vpcWriter, pc.State.PeeringConnectionID)
			if err != nil {
				ctx.Log("Error deleting route to %s: %s", pc.State.PeeringConnectionID, err)
				continue
			}
			err = deleteRoutes(pc.OtherCTX, pc.OtherVPC, pc.OtherVPCWriter, pc.State.PeeringConnectionID)
			if err != nil {
				ctx.Log("Error deleting route to %s: %s", pc.State.PeeringConnectionID, err)
				continue
			}
			// Delete peering connection
			_, err = ctx.EC2().DeleteVpcPeeringConnection(&ec2.DeleteVpcPeeringConnectionInput{
				VpcPeeringConnectionId: &pc.State.PeeringConnectionID,
			})
			if err != nil {
				ctx.Log("Error deleting peering connection %s: %s", pc.State.PeeringConnectionID, err)
				continue
			}
			ctx.Log("Deleted peering connection %s", pc.State.PeeringConnectionID)
			ctx.WaitForPeeringConnectionStatus(pc.State.PeeringConnectionID, []string{ec2.VpcPeeringConnectionStateReasonCodeDeleted}, []string{ec2.VpcPeeringConnectionStateReasonCodeDeleting})
		}
	}
	peeringConnections = keepPeeringConnections
	vpc.State.PeeringConnections = nil
	for _, pc := range keepPeeringConnections {
		if pc.State != nil {
			vpc.State.PeeringConnections = append(vpc.State.PeeringConnections, pc.State)
		}
	}
	err := vpcWriter.UpdateState(vpc.State)
	if err != nil {
		return nil, fmt.Errorf("Error updating state: %s", err)
	}
	// Create peering connections in Config that don't exist or aren't accepted yet
	for _, pc := range peeringConnections {
		err = validatePeeringConnectionSubnetGroups(vpc, pc.Config.ConnectSubnetGroups)
		if err != nil {
			return nil, fmt.Errorf("Error validating peering connection subnet groups for %s: %s", vpc.ID, err)
		}
		err = validatePeeringConnectionSubnetGroups(pc.OtherVPC, pc.Config.OtherVPCConnectSubnetGroups)
		if err != nil {
			return nil, fmt.Errorf("Error validating peering connection subnet groups for %s: %s", pc.OtherVPC.ID, err)
		}
		region := string(vpc.Region)
		otherRegion := string(pc.Config.OtherVPCRegion)
		if pc.State == nil { // Must create
			var out *ec2.CreateVpcPeeringConnectionOutput
			if pc.Config.IsRequester {
				out, err = ctx.EC2().CreateVpcPeeringConnection(&ec2.CreateVpcPeeringConnectionInput{
					VpcId:       &vpc.ID,
					PeerVpcId:   &pc.OtherVPC.ID,
					PeerOwnerId: &pc.OtherVPC.AccountID,
					PeerRegion:  &otherRegion,
				})
				pc.State = &database.PeeringConnection{
					RequesterVPCID:  vpc.ID,
					RequesterRegion: vpc.Region,
					AccepterVPCID:   pc.OtherVPC.ID,
					AccepterRegion:  pc.OtherVPC.Region,
				}
			} else {
				out, err = pc.OtherCTX.EC2().CreateVpcPeeringConnection(&ec2.CreateVpcPeeringConnectionInput{
					VpcId:       &pc.OtherVPC.ID,
					PeerVpcId:   &vpc.ID,
					PeerOwnerId: &vpc.AccountID,
					PeerRegion:  &region,
				})
				pc.State = &database.PeeringConnection{
					RequesterVPCID:  pc.OtherVPC.ID,
					RequesterRegion: pc.OtherVPC.Region,
					AccepterVPCID:   vpc.ID,
					AccepterRegion:  vpc.Region,
				}
			}
			if err != nil {
				return nil, fmt.Errorf("Error creating peering connection to VPC %s: %s", pc.Config.OtherVPCID, err)
			}
			pcxID := aws.StringValue(out.VpcPeeringConnection.VpcPeeringConnectionId)
			pc.State.PeeringConnectionID = pcxID
			vpc.State.PeeringConnections = append(vpc.State.PeeringConnections, pc.State)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				return nil, fmt.Errorf("Error updating state: %s", err)
			}
			ctx.Log("Created peering connection %s to %s", pcxID, pc.OtherVPC.ID)
			ctx.Log("Waiting for peering connection status")
			status, err := ctx.WaitForPeeringConnectionStatus(
				*out.VpcPeeringConnection.VpcPeeringConnectionId,
				[]string{
					ec2.VpcPeeringConnectionStateReasonCodePendingAcceptance, ec2.VpcPeeringConnectionStateReasonCodeActive,
				},
				[]string{
					ec2.VpcPeeringConnectionStateReasonCodeInitiatingRequest,
					ec2.VpcPeeringConnectionStateReasonCodeProvisioning,
				})
			if err != nil {
				return nil, fmt.Errorf("Error waiting for peering connection %s: %s", pcxID, err)
			}
			if status == ec2.VpcPeeringConnectionStateReasonCodeActive {
				pc.State.IsAccepted = true
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					return nil, fmt.Errorf("Error updating state: %s", err)
				}
			}
		}

		if !pc.State.IsAccepted { // Must accept
			if pc.Config.IsRequester {
				_, err = pc.OtherCTX.EC2().AcceptVpcPeeringConnection(&ec2.AcceptVpcPeeringConnectionInput{
					VpcPeeringConnectionId: &pc.State.PeeringConnectionID,
				})
			} else {
				_, err = ctx.EC2().AcceptVpcPeeringConnection(&ec2.AcceptVpcPeeringConnectionInput{
					VpcPeeringConnectionId: &pc.State.PeeringConnectionID,
				})
			}
			if err != nil {
				return nil, fmt.Errorf("Error accepting peering connection %s: %s", pc.State.PeeringConnectionID, err)
			}
			ctx.Log("Accepted peering connection %s", pc.State.PeeringConnectionID)
			pc.State.IsAccepted = true
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				return nil, fmt.Errorf("Error updating state: %s", err)
			}
		}
		_, err = ctx.WaitForPeeringConnectionStatus(
			pc.State.PeeringConnectionID,
			[]string{
				ec2.VpcPeeringConnectionStateReasonCodeActive,
			},
			[]string{
				ec2.VpcPeeringConnectionStateReasonCodePendingAcceptance,
				ec2.VpcPeeringConnectionStateReasonCodeProvisioning,
			})
		if err != nil {
			return nil, fmt.Errorf("Error waiting for peering connection: %s", err)
		}

		var pcxName string
		if pc.Config.IsRequester {
			pcxName = peeringConnectionName(vpc.Name, pc.OtherVPC.Name)
		} else {
			pcxName = peeringConnectionName(pc.OtherVPC.Name, vpc.Name)
		}
		tags := []*ec2.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(pcxName),
			},
			{
				Key:   aws.String("Automated"),
				Value: aws.String("true"),
			},
		}
		ctx.EC2().CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{&pc.State.PeeringConnectionID},
			Tags:      tags,
		})
		pc.OtherCTX.EC2().CreateTags(&ec2.CreateTagsInput{
			Resources: []*string{&pc.State.PeeringConnectionID},
			Tags:      tags,
		})

		// Now identify which subnets to connect on each side (for routes, later)
		pc.SubnetIDs = getSubnetIDsForPeeringConnection(vpc, pc.Config.ConnectPrivate, pc.Config.ConnectSubnetGroups)

		for _, subnetID := range getSubnetIDsForPeeringConnection(pc.OtherVPC, pc.Config.OtherVPCConnectPrivate, pc.Config.OtherVPCConnectSubnetGroups) {
			out, err := pc.OtherCTX.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
				SubnetIds: []*string{&subnetID},
			})
			if err != nil {
				return nil, fmt.Errorf("Error describing subnet %s: %s", subnetID, err)
			}
			pc.OtherVPCCIDRs = append(pc.OtherVPCCIDRs, aws.StringValue(out.Subnets[0].CidrBlock))
		}
	}
	return peeringConnections, nil
}

func handleTransitGatewayAttachments(
	ctx *awsp.Context,
	vpc *database.VPC,
	vpcWriter database.VPCWriter,
	networkConfig *database.UpdateNetworkingTaskData,
	modelsManager database.ModelsManager,
	managedAttachmentsByID map[uint64]*database.ManagedTransitGatewayAttachment,
	getAccountCredentials func(accountID string) (ec2iface.EC2API, ramiface.RAMAPI, error)) error {

	managedIDsByTGID := make(map[string][]uint64)
	for _, managedID := range networkConfig.ManagedTransitGatewayAttachmentIDs {
		ma := managedAttachmentsByID[managedID]
		managedIDsByTGID[ma.TransitGatewayID] = append(managedIDsByTGID[ma.TransitGatewayID], managedID)
	}

	// First delete any managed transit gateway attachments that are no longer in the config.
	transitGatewayAttachmentsByTGID := make(map[string]*database.TransitGatewayAttachment)
	tgas := append([]*database.TransitGatewayAttachment{}, vpc.State.TransitGatewayAttachments...) // copy
	for idx, tga := range tgas {
		found := false
		for _, managedID := range networkConfig.ManagedTransitGatewayAttachmentIDs {
			ma := managedAttachmentsByID[managedID]
			if ma == nil {
				return fmt.Errorf("Invalid managed attachment ID: %d", managedID)
			}
			if ma.TransitGatewayID == tga.TransitGatewayID {
				found = true
				transitGatewayAttachmentsByTGID[tga.TransitGatewayID] = tga
				break
			}
		}
		if !found {
			// Delete any routes to the transit gateway for this attachment
			for _, az := range vpc.State.AvailabilityZones {
				for subnetType, subnets := range az.Subnets {
					if subnetType == database.SubnetTypeFirewall {
						// no TGW routes in firewall subnets
						continue
					}
					for _, subnet := range subnets {
						var rtID string

						if subnet.CustomRouteTableID != "" {
							rtID = subnet.CustomRouteTableID
						} else if subnetType == database.SubnetTypePublic {
							if vpc.State.VPCType.HasFirewall() {
								rtID = az.PublicRouteTableID
							} else {
								rtID = vpc.State.PublicRouteTableID
							}
						} else {
							rtID = az.PrivateRouteTableID
						}
						if rtID == "" {
							return fmt.Errorf("No route table for subnet %s", subnet.SubnetID)
						}

						rt, ok := vpc.State.RouteTables[rtID]
						if !ok {
							return fmt.Errorf("Route table %s missing from state", rtID)
						}
						existingRoutes := append([]*database.RouteInfo{}, rt.Routes...) // copy
						for _, route := range existingRoutes {
							if route.TransitGatewayID == tga.TransitGatewayID {
								var err error
								rt.Routes, err = setRoute(ctx, rtID, route.Destination, rt.Routes, nil)
								if err != nil {
									return fmt.Errorf("Error updating route table %s: %s", rtID, err)
								}
								err = vpcWriter.UpdateState(vpc.State)
								if err != nil {
									return fmt.Errorf("Error updating state: %s", err)
								}
							}
						}
					}
				}
			}
			err := ctx.DeleteTransitGatewayVPCAttachment(tga.TransitGatewayAttachmentID)
			if err != nil {
				return fmt.Errorf("Error deleting transit gateway attachment: %s", err)
			}
			vpc.State.TransitGatewayAttachments = append(vpc.State.TransitGatewayAttachments[:idx], vpc.State.TransitGatewayAttachments[idx+1:]...)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				return fmt.Errorf("Error updating state: %s", err)
			}
		}
	}

	// Now create or update all the managed transit gateway attachments.
	for tgID, managedIDs := range managedIDsByTGID {
		share, err := modelsManager.GetTransitGatewayResourceShare(networkConfig.AWSRegion, tgID)
		if err != nil {
			return fmt.Errorf("Error checking share info: %s", err)
		}
		var shareEC2 ec2iface.EC2API
		var sourceRAM ramiface.RAMAPI
		if share == nil {
			ctx.Log("WARNING: no Resource Share ID specified for Transit Gateway %s. Will only be able to attach this transit gateway if it is from the same account or already shared", tgID)
		} else {
			shareEC2, sourceRAM, err = getAccountCredentials(share.AccountID)
			if err != nil {
				return fmt.Errorf("Error getting AWS credentials for share account: %s", err)
			}
		}
		// First check to see if we need to share it.
		if share != nil && share.AccountID != vpc.AccountID {
			shareARN := resourceShareARN(string(networkConfig.AWSRegion), share.AccountID, share.ResourceShareID)
			ctx.Log("Checking principal list on share %s in account %s", share.ResourceShareID, share.AccountID)
			err = ctx.EnsurePrincipalOnShare(sourceRAM, vpc.AccountID, shareARN)
			if err != nil {
				return err
			}
		}
		// Transit gateway should live in one subnet per AZ
		transitGatewaySubnetIDs := []string{}
		for _, az := range vpc.State.AvailabilityZones {
			subnetType := database.SubnetTypePrivate
			if vpc.State.VPCType == database.VPCTypeLegacy {
				subnetType = database.SubnetTypeTransitive
			}
			if len(az.Subnets[subnetType]) > 0 {
				transitGatewaySubnetIDs = append(transitGatewaySubnetIDs, az.Subnets[subnetType][0].SubnetID)
			}
		}
		attachment, ok := transitGatewayAttachmentsByTGID[tgID]
		if ok {
			attachment.ManagedTransitGatewayAttachmentIDs = managedIDs

			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				return fmt.Errorf("Error updating state: %s", err)
			}
			err = addOrUpdateTGAttachmentTags(ctx, managedAttachmentsByID, managedIDs, attachment.TransitGatewayAttachmentID)
			if err != nil {
				return fmt.Errorf("Error updating tags on transit gateway attachment %s: %s", attachment.TransitGatewayAttachmentID, err)
			}

			missingSubnetIDs := []*string{}
			for _, subnetID := range transitGatewaySubnetIDs {
				foundSubnet := false
				for _, attachedID := range attachment.SubnetIDs {
					if subnetID == attachedID {
						foundSubnet = true
						break
					}
				}
				if !foundSubnet {
					missingSubnetIDs = append(missingSubnetIDs, aws.String(subnetID))
					ctx.Log("Transit gateway attachment %s is missing subnet %s", attachment.TransitGatewayAttachmentID, subnetID)
					attachment.SubnetIDs = append(attachment.SubnetIDs, subnetID) // to be saved later, after success
				}
			}
			if len(missingSubnetIDs) > 0 {
				ctx.Log("Adding missing subnets to transit gateway attachment")
				_, err := ctx.EC2().ModifyTransitGatewayVpcAttachment(&ec2.ModifyTransitGatewayVpcAttachmentInput{
					TransitGatewayAttachmentId: &attachment.TransitGatewayAttachmentID,
					AddSubnetIds:               missingSubnetIDs,
				})
				if err != nil {
					return fmt.Errorf("Error updating transit gateway attachment: %s", err)
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					return fmt.Errorf("Error updating state: %s", err)
				}
				ctx.Log("Waiting for transit gateway attachment to become available")
				ctx.WaitForTransitGatewayVpcAttachmentStatus(attachment.TransitGatewayAttachmentID, []string{"available"}, []string{"modifying"})
			}
		} else {
			// If we just shared it we might need to wait for it to appear
			ctx.Log("Waiting for transit gateway to be available")
			err := ctx.WaitForTransitGatewayStatus(tgID, "available")
			if err != nil {
				return fmt.Errorf("Error waiting for transit gateway status: %s", err)
			}
			attachment = &database.TransitGatewayAttachment{
				ManagedTransitGatewayAttachmentIDs: managedIDs,
				TransitGatewayID:                   tgID,
				SubnetIDs:                          transitGatewaySubnetIDs,
			}

			maName := generateMTGAName(managedIDs, managedAttachmentsByID)
			attachment.TransitGatewayAttachmentID, err = ctx.CreateTransitGatewayVPCAttachment(transitGatewayAttachmentName(vpc.Name, maName), tgID, transitGatewaySubnetIDs)
			if err != nil {
				return fmt.Errorf("Error creating transit gateway attachment %s: %s", transitGatewayAttachmentName(vpc.Name, maName), err)
			}

			vpc.State.TransitGatewayAttachments = append(vpc.State.TransitGatewayAttachments, attachment)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				return fmt.Errorf("Error updating state: %s", err)
			}
			err = addOrUpdateTGAttachmentTags(ctx, managedAttachmentsByID, managedIDs, attachment.TransitGatewayAttachmentID)
			if err != nil {
				return fmt.Errorf("Error updating tags on transit gateway attachment %s: %s", attachment.TransitGatewayAttachmentID, err)
			}
		}
		state, err := ctx.WaitForTransitGatewayVpcAttachmentStatus(attachment.TransitGatewayAttachmentID, []string{"available", "pendingAcceptance"}, []string{"pending"})
		if err != nil {
			return fmt.Errorf("Error waiting for transit gateway attachment status: %s", err)
		}
		if state == "pendingAcceptance" {
			if share == nil {
				return fmt.Errorf("Transit Gateway %s does not automatically accept attachment and no Resource Share is specified. Specify a Resource Share or manually accept the Attachment and retry.", tgID)
			}
			// Accept using owner's EC2 credentials
			ctx.Log("Accepting Transit Gateway VPC Attachment %s", attachment.TransitGatewayAttachmentID)
			_, err := shareEC2.AcceptTransitGatewayVpcAttachment(&ec2.AcceptTransitGatewayVpcAttachmentInput{
				TransitGatewayAttachmentId: &attachment.TransitGatewayAttachmentID,
			})
			if err != nil {
				return fmt.Errorf("Error accepting attachment: %s", err)
			}
			ctx.Log("Waiting for transit gateway attachment to become available")
			_, err = ctx.WaitForTransitGatewayVpcAttachmentStatus(attachment.TransitGatewayAttachmentID, []string{"available"}, []string{"pending"})
			if err != nil {
				return fmt.Errorf("Error waiting for transit gateway attachment status: %s", err)
			}
		}
	}
	return nil
}

var ErrIncompleteResolverRuleProcessing = errors.New("Incomplete resolver rule association processing")

func handleResolverRuleAssociations(
	lockSet database.LockSet,
	ctx *awsp.Context,
	vpc *database.VPC,
	vpcWriter database.VPCWriter,
	config *database.UpdateResolverRulesTaskData,
	modelsManager database.ModelsManager,
	managedResolverRuleSetsByID map[uint64]*database.ManagedResolverRuleSet,
	getAccountCredentials func(accountID string) (ramiface.RAMAPI, error)) error {
	// First delete any managed resolver rule associations that are no longer in the config.
	filteredResolverRuleAssociations := []*database.ResolverRuleAssociation{}

	experiencedFailures := make([]error, 0)

	for _, resolverRuleAssociation := range vpc.State.ResolverRuleAssociations {
		found := false
		for _, managedID := range config.ManagedResolverRuleSetIDs {
			managedRuleSet := managedResolverRuleSetsByID[managedID]
			if managedRuleSet == nil {
				return fmt.Errorf("Invalid managed resolver ruleset ID: %d", managedID)
			}
			if managedRuleSet.Region != vpc.Region {
				return fmt.Errorf("Resolver Ruleset %s passed for invalid region (%s != %s)", managedRuleSet.Name, managedRuleSet.Region, vpc.Region)
			}
			for _, rule := range managedRuleSet.Rules {
				if resolverRuleAssociation.ResolverRuleID == rule.AWSID {
					found = true
					break
				}
			}
		}

		if !found {
			// Delete the association since it isn't in the requested config
			err := ctx.DeleteResolverRuleVPCAssociation(resolverRuleAssociation.ResolverRuleID, resolverRuleAssociation.ResolverRuleAssociationID)
			if err != nil {
				recoverableError := fmt.Errorf("Error deleting resolver rule attachment: %w", err)
				ctx.Logger.Log(recoverableError.Error())
				experiencedFailures = append(experiencedFailures, recoverableError)
				// Because we failed to delete, retain the association in the state
				filteredResolverRuleAssociations = append(filteredResolverRuleAssociations, resolverRuleAssociation)
			}
			continue
		}
		filteredResolverRuleAssociations = append(filteredResolverRuleAssociations, resolverRuleAssociation)
	}

	// Update the state to reflect post rule disassociation
	vpc.State.ResolverRuleAssociations = filteredResolverRuleAssociations
	err := vpcWriter.UpdateState(vpc.State)
	if err != nil {
		return fmt.Errorf("Error updating state: %w", err)
	}

	// First let's see if we need to create any RAM shares (and grab a lock)
	for _, managedRuleSetID := range config.ManagedResolverRuleSetIDs {
		managedRuleSet := managedResolverRuleSetsByID[managedRuleSetID]
		if managedRuleSet.ResourceShareID == "" {
			err := lockSet.AcquireAdditionalLock(database.TargetAddResourceShare)
			if err != nil {
				return fmt.Errorf("Error acquiring lock: %w", err)
			}
			break
		}
	}

	// Now create or update all the configured resolver rule attachments.
	for _, managedRuleSetID := range config.ManagedResolverRuleSetIDs {
		managedRuleSet := managedResolverRuleSetsByID[managedRuleSetID]
		if managedRuleSet == nil {
			return fmt.Errorf("Invalid managed resolver ruleset ID: %d", managedRuleSetID)
		}
		sourceRAM, err := getAccountCredentials(managedRuleSet.AccountID)
		if err != nil {
			return fmt.Errorf("Error getting AWS credentials for share account: %w", err)
		}

		var resourceArns []string
		for _, rule := range managedRuleSet.Rules {
			resourceArns = append(resourceArns, resolverRuleARN(string(managedRuleSet.Region), managedRuleSet.AccountID, rule.AWSID))
		}

		// First check to see if we need to share it
		if managedRuleSet.ResourceShareID == "" {
			ctx.Log("Sharing resolver ruleset %d for [%s]", managedRuleSet.ID, strings.Join(resourceArns, ", "))
			managedRuleSet.ResourceShareID, err = ctx.CreateRAMShare(lockSet, sourceRAM, managedRuleSet.Name, resourceArns)
			if err != nil {
				return fmt.Errorf("unable to share: %w", err)
			}
			err = modelsManager.UpdateManagedResolverRuleSet(managedRuleSet.ID, managedRuleSet)
			if err != nil {
				return fmt.Errorf("Error updating managed ruleset %d record: %w", managedRuleSet.ID, err)
			}
		}

		// Source and destination accounts are different, so share
		if managedRuleSet.AccountID != vpc.AccountID {
			// Ensure all the rules configured in vpcconf are on the share
			shareARN := resourceShareARN(string(managedRuleSet.Region), managedRuleSet.AccountID, managedRuleSet.ResourceShareID)

			if len(resourceArns) > 0 {

				// code to fix more than 100 resolverrules issue
				if len(resourceArns) > 100 {

					var resourceShareARN1 = resourceArns[0:99]
					var resourceShareARN2 = resourceArns[99 : len(resourceArns)-1]
					_, err = sourceRAM.AssociateResourceShare(
						&ram.AssociateResourceShareInput{
							ResourceShareArn: aws.String(shareARN),
							ResourceArns:     aws.StringSlice(resourceShareARN1),
						},
					)

					if err != nil {
						return fmt.Errorf("Unable to associate first 100 configured rules with share %s: %w", shareARN, err)
					}

					_, err = sourceRAM.AssociateResourceShare(
						&ram.AssociateResourceShareInput{
							ResourceShareArn: aws.String(shareARN),
							ResourceArns:     aws.StringSlice(resourceShareARN2),
						},
					)

					if err != nil {
						return fmt.Errorf("Unable to associate next 100 configured rules with share %s: %w", shareARN, err)
					}
				} else {
					_, err = sourceRAM.AssociateResourceShare(
						&ram.AssociateResourceShareInput{
							ResourceShareArn: aws.String(shareARN),
							ResourceArns:     aws.StringSlice(resourceArns),
						},
					)

					if err != nil {
						return fmt.Errorf("Unable to associate all configured rules with share %s: %w", shareARN, err)
					}
				}
				// code to fix more than 100 resolverrules issue ends

			}
			err = ctx.EnsurePrincipalOnShare(sourceRAM, vpc.AccountID, shareARN)
			if err != nil {
				return fmt.Errorf("Unable to ensure principal is on share: %w", err)
			}
		}

		for _, rule := range managedRuleSet.Rules {
			ruleInState := false
			for _, association := range vpc.State.ResolverRuleAssociations {
				if association.ResolverRuleID == rule.AWSID {
					ruleInState = true
					break
				}
			}
			if !ruleInState {
				// If we just shared it we might need to wait for it to appear
				ctx.Log("Waiting for resolver rule to be available")
				err := ctx.WaitForResolverRuleStatus(rule.AWSID, route53resolver.ResolverRuleStatusComplete)
				if err != nil {
					return fmt.Errorf("Error waiting for resolver rule status: %s", err)
				}
				associationID, err := ctx.CreateResolverRuleVPCAttachment(rule.Description, rule.AWSID)
				if err != nil {
					recoverableError := fmt.Errorf("Error associating resolver rule %q (%q): %w", rule.AWSID, rule.Description, err)
					ctx.Logger.Log(recoverableError.Error())
					experiencedFailures = append(experiencedFailures, recoverableError)
					continue
				}
				association := &database.ResolverRuleAssociation{
					ResolverRuleAssociationID: associationID,
					ResolverRuleID:            rule.AWSID,
				}
				vpc.State.ResolverRuleAssociations = append(vpc.State.ResolverRuleAssociations, association)
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					return fmt.Errorf("Error updating VPC state: %w", err)
				}
			}
		}
	}

	// Final pass - remove shares that we manage if there are no more rules associated
	// Have to do two passes to make sure in case a share is re-used
	type shareToDelete struct {
		deletable bool
		id        uint64
	}
	sharesToDelete := make(map[string]*shareToDelete)
	for _, rs := range managedResolverRuleSetsByID {
		if rs.AccountID != vpc.AccountID {
			if rs.ResourceShareID != "" && rs.Region == vpc.Region {
				needed := false
				for _, rule := range rs.Rules {
					for _, resolverRuleAssociation := range vpc.State.ResolverRuleAssociations {
						if resolverRuleAssociation.ResolverRuleID == rule.AWSID {
							needed = true
							break
						}
					}
					if needed {
						break
					}
				}
				if needed {
					if sharesToDelete[rs.ResourceShareID] == nil {
						sharesToDelete[rs.ResourceShareID] = &shareToDelete{}
					}
					sharesToDelete[rs.ResourceShareID].deletable = false
				} else {
					if _, ok := sharesToDelete[rs.ResourceShareID]; !ok {
						sharesToDelete[rs.ResourceShareID] = &shareToDelete{
							deletable: true,
							id:        rs.ID,
						}
					}
				}
			}
		}
	}
	for _, deleteInfo := range sharesToDelete {
		share := managedResolverRuleSetsByID[deleteInfo.id]
		if deleteInfo.deletable {
			sourceRAM, err := getAccountCredentials(share.AccountID)
			if err != nil {
				return fmt.Errorf("Error getting AWS credentials for source account: %w", err)
			}
			shareARN := resourceShareARN(string(share.Region), share.AccountID, share.ResourceShareID)
			inUse, err := ctx.CheckResourceShareResourcesInUse(sourceRAM, shareARN)
			if err != nil {
				return err
			}
			if !inUse {
				err = ctx.RemovePrincipalFromShare(sourceRAM, vpc.AccountID, shareARN)
				if err != nil {
					ctx.Logger.Log(err.Error())
					continue
				}
			}
		}
	}
	if len(experiencedFailures) > 0 {
		ctx.Logger.Log("Experienced %d errors:", len(experiencedFailures))
		for _, e := range experiencedFailures {
			ctx.Logger.Log(e.Error())
		}
		return ErrIncompleteResolverRuleProcessing
	}
	return nil
}

func updatePeeringConnectionRoutesForSubnet(
	ctx *awsp.Context,
	vpc *database.VPC,
	vpcWriter database.VPCWriter,
	networkConfig *database.UpdateNetworkingTaskData,
	cidrs []string,
	peeringConnectionID string,
	routeTable *database.RouteTableInfo) error {
	deleteRoutes := []string{}

	expectedRoutes := make(map[string]bool)
	for _, cidr := range cidrs {
		var err error
		expectedRoutes[cidr] = true
		routeTable.Routes, err = setRoute(ctx, routeTable.RouteTableID, cidr, routeTable.Routes, &database.RouteInfo{
			PeeringConnectionID: peeringConnectionID,
		})
		if err != nil {
			return fmt.Errorf("Error updating private route table %s: %s", routeTable.RouteTableID, err)
		}
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	for _, route := range routeTable.Routes {
		if route.PeeringConnectionID == peeringConnectionID && !expectedRoutes[route.Destination] {
			deleteRoutes = append(deleteRoutes, route.Destination)
		}
	}
	for _, route := range deleteRoutes {
		var err error
		routeTable.Routes, err = setRoute(ctx, routeTable.RouteTableID, route, routeTable.Routes, nil)
		if err != nil {
			return fmt.Errorf("Error updating private route table %s: %s", routeTable.RouteTableID, err)
		}
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	return nil
}

func updateTransitGatewayRoutesForSubnet(
	ctx *awsp.Context,
	vpc *database.VPC,
	vpcWriter database.VPCWriter,
	networkConfig *database.UpdateNetworkingTaskData,
	managedAttachmentsByID map[uint64]*database.ManagedTransitGatewayAttachment,
	routeTable *database.RouteTableInfo,
	subnetType database.SubnetType,
	region database.Region,
	getPrefixListRAM func(region database.Region) (ramiface.RAMAPI, error)) error {
	deleteRoutes := []string{}
	subnetTypesAndRoutesByTGID := make(map[string]map[database.SubnetType]map[string]struct{}) // TG ID --> subnet type --> route --> struct{}

	if subnetType == database.SubnetTypeUnroutable || subnetType == database.SubnetTypeFirewall {
		return nil
	}

	// Get the union of routes by subnet type for MTGAs that share a TG ID
	for _, managedID := range networkConfig.ManagedTransitGatewayAttachmentIDs {
		ma := managedAttachmentsByID[managedID]
		for _, t := range ma.SubnetTypes {
			if subnetTypesAndRoutesByTGID[ma.TransitGatewayID] == nil {
				subnetTypesAndRoutesByTGID[ma.TransitGatewayID] = make(map[database.SubnetType]map[string]struct{})
			}
			if subnetTypesAndRoutesByTGID[ma.TransitGatewayID][t] == nil {
				subnetTypesAndRoutesByTGID[ma.TransitGatewayID][t] = make(map[string]struct{})
			}
			for _, route := range ma.Routes {
				subnetTypesAndRoutesByTGID[ma.TransitGatewayID][t][route] = struct{}{}
			}
		}
	}

	for _, managedID := range networkConfig.ManagedTransitGatewayAttachmentIDs {
		ma := managedAttachmentsByID[managedID]
		routes, appliesToSubnetType := subnetTypesAndRoutesByTGID[ma.TransitGatewayID][subnetType]

		expectedRoutes := make(map[string]bool)
		if appliesToSubnetType {
			for route := range routes {
				var err error

				if awsp.IsPrefixListID(route) {
					plRAM, err := getPrefixListRAM(region)
					if err != nil {
						return fmt.Errorf("Error getting AWS credentials for Prefix List share account: %s", err)
					}
					err = ensurePrefixListSharedWithAccount(ctx, plRAM, []string{route}, region, vpc.AccountID)
					if err != nil {
						return fmt.Errorf("Error ensuring configured Prefix Lists are shared via RAM: %s", err)
					}
				}

				expectedRoutes[route] = true
				routeTable.Routes, err = setRoute(ctx, routeTable.RouteTableID, route, routeTable.Routes, &database.RouteInfo{
					TransitGatewayID: ma.TransitGatewayID,
				})
				if err != nil {
					return fmt.Errorf("Error updating private route table %s: %s", routeTable.RouteTableID, err)
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					return fmt.Errorf("Error updating state: %s", err)
				}
			}
		}
		for _, route := range routeTable.Routes {
			if route.TransitGatewayID == ma.TransitGatewayID && !expectedRoutes[route.Destination] {
				deleteRoutes = append(deleteRoutes, route.Destination)
			}
		}
	}
	for _, route := range deleteRoutes {
		var err error
		routeTable.Routes, err = setRoute(ctx, routeTable.RouteTableID, route, routeTable.Routes, nil)
		if err != nil {
			return fmt.Errorf("Error updating private route table %s: %s", routeTable.RouteTableID, err)
		}
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}
	return nil
}

func getSubnetIDsForPeeringConnection(vpc *database.VPC, connectPrivate bool, connectSubnetGroups []string) []string {
	subnetIDs := []string{}
	for _, az := range vpc.State.AvailabilityZones {
		for subnetType, subnets := range az.Subnets {
			for _, subnet := range subnets {
				connect := (subnetType == database.SubnetTypePrivate && connectPrivate) || (subnet.GroupName != "" && stringInSlice(subnet.GroupName, connectSubnetGroups))
				if connect {
					subnetIDs = append(subnetIDs, subnet.SubnetID)
				}
			}
		}
	}
	return subnetIDs
}

func validatePeeringConnectionSubnetGroups(vpc *database.VPC, connectSubnetGroups []string) error {
	for _, az := range vpc.State.AvailabilityZones {
		for subnetType, subnets := range az.Subnets {
			for _, subnet := range subnets {
				if stringInSlice(subnet.GroupName, connectSubnetGroups) {
					if subnetType == database.SubnetTypePrivate {
						return fmt.Errorf("Peering connections for Private subnets can't be configured individually")
					} else if subnetType == database.SubnetTypePublic {
						return fmt.Errorf("Peering connections for Public subnet groups are not allowed")
					} else if subnetType == database.SubnetTypeUnroutable {
						return fmt.Errorf("Peering connections for Unroutable subnet groups are not allowed")
					} else if subnetType == database.SubnetTypeFirewall {
						return fmt.Errorf("Peering connections for Firewall subnet groups are not allowed")
					}
				}
			}
		}
	}
	return nil
}

const logDestinationLogGroup = "logGroup"

func (taskContext *TaskContext) performUpdateLoggingTask(networkConfig *database.UpdateLoggingTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Updating VPC flow logs")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, networkConfig.Region, networkConfig.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State.VPCType == database.VPCTypeException {
		t.Log("This is not allowed for Exception VPCs")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            networkConfig.VPCID,
		VPCName:          vpc.Name,
	}

	// Flow logs
	flowLogFailed := false

	if vpc.State.CloudWatchLogsFlowLogID == "" {
		found := false
		// Make flowlogs for the first config where we can find the destination log group
		for _, config := range cloudwatchFlowLogConfigs() {
			if found {
				break
			}
			logsOut, err := ctx.CloudWatchLogs().DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
				LogGroupNamePrefix: &config.groupName,
			})
			if err != nil {
				t.Log("Error listing CloudWatch Logs groups: %s", err)
				break
			}
			for _, logGroup := range logsOut.LogGroups {
				if aws.StringValue(logGroup.LogGroupName) == config.groupName {
					found = true
					out, err := ctx.EC2().CreateFlowLogs(&ec2.CreateFlowLogsInput{
						ClientToken:              aws.String(vpc.ID + "-flowlogs-to-cloudwatch"),
						DeliverLogsPermissionArn: aws.String(roleARN(vpc.Region, vpc.AccountID, config.role)),
						LogDestinationType:       aws.String(ec2.LogDestinationTypeCloudWatchLogs),
						LogGroupName:             aws.String(config.groupName),
						LogFormat:                aws.String(flowLogFormat),
						ResourceIds:              []*string{&vpc.ID},
						ResourceType:             aws.String(ec2.FlowLogsResourceTypeVpc),
						TrafficType:              aws.String(ec2.TrafficTypeAll),
					})
					if err != nil {
						t.Log("Error adding CloudWatch Logs flowlogs: %s", err)
						flowLogFailed = true
					} else if len(out.Unsuccessful) > 0 {
						msg := ""
						if out.Unsuccessful[0].Error != nil {
							msg = aws.StringValue(out.Unsuccessful[0].Error.Message)
						}
						t.Log("Error adding CloudWatch Logs flowlogs: %s", msg)
						flowLogFailed = true
					} else if len(out.FlowLogIds) != 1 {
						t.Log("Expected 1 CloudWatch Logs flowlogs but got %d", len(out.FlowLogIds))
						flowLogFailed = true
					} else {
						vpc.State.CloudWatchLogsFlowLogID = aws.StringValue(out.FlowLogIds[0])
						t.Log("Created FlowLogs %s to CloudWatch Logs", vpc.State.CloudWatchLogsFlowLogID)
						err = vpcWriter.UpdateState(vpc.State)
						if err != nil {
							t.Log("Error updating state: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
					break
				}
			}
		}
		if !found {
			t.Log("Could not find any log group for CloudWatch Logs flowlogs")
		}
	}
	if vpc.State.S3FlowLogID == "" {
		out, err := ctx.EC2().CreateFlowLogs(&ec2.CreateFlowLogsInput{
			ClientToken:        aws.String(vpc.ID + "-flowlogs-to-s3"),
			LogDestination:     aws.String(flowLogS3Destinations(vpc.AccountID, vpc.Region)[0]),
			LogDestinationType: aws.String(ec2.LogDestinationTypeS3),
			LogFormat:          aws.String(flowLogFormat),
			ResourceIds:        []*string{&vpc.ID},
			ResourceType:       aws.String(ec2.FlowLogsResourceTypeVpc),
			TrafficType:        aws.String(ec2.TrafficTypeAll),
		})
		if err != nil {
			t.Log("Error adding S3 flowlogs: %s", err)
			flowLogFailed = true
		} else if len(out.Unsuccessful) > 0 {
			msg := ""
			if out.Unsuccessful[0].Error != nil {
				msg = aws.StringValue(out.Unsuccessful[0].Error.Message)
			}
			t.Log("Error adding S3 flowlogs: %s", msg)
			flowLogFailed = true
		} else if len(out.FlowLogIds) != 1 {
			t.Log("Expected 1 S3 flowlogs but got %d", len(out.FlowLogIds))
			flowLogFailed = true
		} else {
			vpc.State.S3FlowLogID = aws.StringValue(out.FlowLogIds[0])
			t.Log("Created FlowLogs %s to S3", vpc.State.S3FlowLogID)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Query logs
	t.Log("Updating VPC query logs")

	queryLogFailed := false

	logGroupArn, err := ctx.CheckLogGroupExists(cloudwatchQueryLogDestination)
	if err != nil {
		t.Log("Error checking for query log group: %s", err)
		return
	}

	if logGroupArn == nil {
		newArn, err := ctx.EnsureCloudWatchLogsGroupExists(cloudwatchQueryLogDestination)
		if err != nil {
			t.Log("Error creating CloudWatch Logs querylogs group: %s", err)
			queryLogFailed = true
		} else {
			logGroupArn = aws.String(newArn)
			t.Log("Created cloudwatch log group for query logs: %s", newArn)
		}
	}

	if !queryLogFailed && vpc.State.ResolverQueryLogConfigurationID == "" {
		configId, err := ctx.CreateResolverQueryLogConfig(aws.StringValue(logGroupArn))
		if err != nil {
			t.Log("Error creating querylog configuration: %s", err)
			queryLogFailed = true
		} else {
			vpc.State.ResolverQueryLogConfigurationID = configId
			t.Log("Created QueryLog Configuration %s ", vpc.State.ResolverQueryLogConfigurationID)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	if !queryLogFailed && vpc.State.ResolverQueryLogConfigurationID != "" && vpc.State.ResolverQueryLogAssociationID == "" {
		associationId, err := ctx.AssociateResolverQueryLogConfig(vpc.State.ResolverQueryLogConfigurationID)
		if err != nil {
			t.Log("Error associating querylog configuration with VPC: %s", err)
			queryLogFailed = true
		} else {
			vpc.State.ResolverQueryLogAssociationID = associationId
			t.Log("Associated QueryLog Configuration %s with VPC (%s)", vpc.State.ResolverQueryLogConfigurationID, associationId)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Firewall logs

	var firewallLogFailed bool

	if vpc.State.VPCType.HasFirewall() {
		t.Log("Updating firewall alert logs")

		firewallLogFailed = false

		logGroupArn, err := ctx.CheckLogGroupExists(firewallAlertLogName(ctx.VPCID))
		if err != nil {
			t.Log("Error checking for firewall alert log group: %s", err)
			return
		}

		if logGroupArn == nil {
			_, err := ctx.EnsureCloudWatchLogsGroupExists(firewallAlertLogName(ctx.VPCID))
			if err != nil {
				t.Log("Error creating CloudWatch Logs firewall alert log group: %s", err)
				firewallLogFailed = true
			}
		}

		if !firewallLogFailed && vpc.State.Firewall != nil {
			_, err := ctx.NetworkFirewall().UpdateLoggingConfiguration(&networkfirewall.UpdateLoggingConfigurationInput{
				FirewallName: aws.String(ctx.FirewallName()),
				LoggingConfiguration: &networkfirewall.LoggingConfiguration{
					LogDestinationConfigs: []*networkfirewall.LogDestinationConfig{
						{
							LogDestination: map[string]*string{
								logDestinationLogGroup: aws.String(firewallAlertLogName(ctx.VPCID)),
							},
							LogDestinationType: aws.String(networkfirewall.LogDestinationTypeCloudWatchLogs),
							LogType:            aws.String(networkfirewall.LogTypeAlert),
						},
					},
				},
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == networkfirewall.ErrCodeInvalidRequestException && aerr.Message() == awsp.NoLoggingConfigChanges {
						// no-op, no changes to config
					} else {
						t.Log("Error updating firewall alert log configuration: %s", err)
						firewallLogFailed = true
					}
				} else {
					t.Log("Error updating firewall alert log configuration: %s", err)
					firewallLogFailed = true
				}
			} else {
				t.Log("Updated firewall alert log configuration")
			}
		}
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifySpec{VerifyLogging: true}, false)
	if err != nil {
		t.Log("Error verifying: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if flowLogFailed || queryLogFailed || firewallLogFailed {
		setStatus(t, database.TaskStatusFailed)
	} else {
		setStatus(t, database.TaskStatusSuccessful)
	}
}

func (taskContext *TaskContext) performUpdateNetworkingTask(networkConfig *database.UpdateNetworkingTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet
	asUser := taskContext.AsUser

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Updating VPC networking")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, networkConfig.AWSRegion, networkConfig.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State.VPCType == database.VPCTypeException {
		t.Log("This is not allowed for Exception VPCs")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            networkConfig.VPCID,
		VPCName:          vpc.Name,
	}

	prefixListAccountID := prefixListAccountIDCommercial
	if networkConfig.AWSRegion.IsGovCloud() {
		prefixListAccountID = prefixListAccountIDGovCloud
	}

	getPrefixListRAM := func(region database.Region) (ramiface.RAMAPI, error) {
		access, err := taskContext.AWSAccountAccessProvider.AccessAccount(prefixListAccountID, string(region), asUser)
		if err != nil {
			return nil, fmt.Errorf("Error getting credentials for account %s: %s", prefixListAccountID, err)
		}
		plRAM := access.RAM()
		return plRAM, nil
	}

	// Firewall resources

	firewallTag := map[string]string{awsp.FirewallTypeKey: awsp.FirewallTypeValue}

	if vpc.State.VPCType.HasFirewall() {
		err = ctx.EnsureDefaultFirewallPolicyExists(vpc.Region)
		if err != nil {
			t.Log("Error ensuring default firewall policy exists: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		firewallSubnetIDs := []string{}
		for _, az := range vpc.State.AvailabilityZones.InOrder() {
			for subnetType, subnets := range az.Subnets {
				if subnetType == database.SubnetTypeFirewall {
					for _, subnet := range subnets {
						firewallSubnetIDs = append(firewallSubnetIDs, subnet.SubnetID)
					}
				}
			}
		}

		if vpc.State.Firewall == nil {
			_, err := ctx.CreateFirewall(firewallSubnetIDs)
			if err != nil {
				t.Log("Error creating network firewall: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			vpc.State.Firewall = &database.Firewall{
				AssociatedSubnetIDs: firewallSubnetIDs,
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			// we pass an empty id since the firewall name is inferred from the VPC name by the existence check
			err = ctx.WaitForExistence("", ctx.FirewallExists)
			if err != nil {
				t.Log("Error waiting for firewall to exist: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		err = updateFirewallSubnetAssociations(ctx, vpc, vpcWriter, firewallSubnetIDs)
		if err != nil {
			t.Log("Error updating firewall subnet associations: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		err := ctx.Tag(ctx.VPCID, firewallTag)
		if err != nil {
			t.Log("Error creating firewall VPC tag: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	} else {
		err := ctx.DeleteTags(ctx.VPCID, firewallTag)
		if err != nil {
			t.Log("Error deleting firewall VPC tag: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	// Transit gateway attachments
	managedAttachmentsByID := make(map[uint64]*database.ManagedTransitGatewayAttachment)
	if len(networkConfig.ManagedTransitGatewayAttachmentIDs) > 0 {
		managedAttachments, err := taskContext.ModelsManager.GetManagedTransitGatewayAttachments()
		if err != nil {
			t.Log("Error getting transit gateway configuration info: %s", err)
			t.SetStatus(database.TaskStatusFailed)
			return
		}
		for _, ma := range managedAttachments {
			managedAttachmentsByID[ma.ID] = ma
		}
	}

	err = handleTransitGatewayAttachments(
		ctx, vpc, vpcWriter, networkConfig, taskContext.ModelsManager, managedAttachmentsByID,
		func(accountID string) (ec2iface.EC2API, ramiface.RAMAPI, error) {
			access, err := taskContext.AWSAccountAccessProvider.AccessAccount(accountID, string(vpc.Region), asUser)
			if err != nil {
				return nil, nil, fmt.Errorf("Error getting credentials for account %s: %s", accountID, err)
			}
			return access.EC2(), access.RAM(), nil
		})
	if err != nil {
		t.Log("%s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	// Peering Connections
	peeringConnections, err := handlePeeringConnections(
		lockSet, ctx, vpc, vpcWriter, networkConfig, taskContext.ModelsManager,
		func(region database.Region, accountID string) (*awsp.Context, error) {
			access, err := taskContext.AWSAccountAccessProvider.AccessAccount(accountID, string(region), asUser)
			if err != nil {
				return nil, fmt.Errorf("Error getting credentials for account %s: %s", accountID, err)
			}
			return &awsp.Context{
				AWSAccountAccess: access,
				Logger:           ctx.Logger,
			}, nil
		})
	if err != nil {
		t.Log("%s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State.VPCType == database.VPCTypeLegacy {
		// We only support transit gateways and peering connections for Legacy VPCs so just add their routes and return.

		for _, az := range vpc.State.AvailabilityZones.InOrder() {
			for subnetType, subnets := range az.Subnets {
				for _, subnet := range subnets {
					subnetID := subnet.SubnetID
					if subnet.CustomRouteTableID == "" {
						t.Log("No custom route table for subnet %s", subnetID)
						continue
					}
					rt, ok := vpc.State.RouteTables[subnet.CustomRouteTableID]
					if !ok {
						t.Log("No custom route table info found for route table %s", subnet.CustomRouteTableID)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					err := updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, managedAttachmentsByID, rt, subnetType, database.Region(networkConfig.AWSRegion), getPrefixListRAM)
					if err != nil {
						t.Log("%s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}

					for _, pc := range peeringConnections {
						var err error
						if stringInSlice(subnet.SubnetID, pc.SubnetIDs) {
							err = updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, pc.OtherVPCCIDRs, pc.State.PeeringConnectionID, rt)
						} else {
							err = updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, []string{}, pc.State.PeeringConnectionID, rt)
						}
						if err != nil {
							t.Log("%s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
				}
			}
		}

		if !networkConfig.SkipVerify {
			issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifySpec{VerifyNetworking: true}, false)
			if err != nil {
				t.Log("Error verifying: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateIssues(issues)
			if err != nil {
				t.Log("Error updating issues: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		setStatus(t, database.TaskStatusSuccessful)

		return
	}

	azToPublicSubnetID := make(map[string]string)
	for azName, az := range vpc.State.AvailabilityZones {
		publicSubnets := az.Subnets[database.SubnetTypePublic]
		if len(publicSubnets) > 0 {
			azToPublicSubnetID[azName] = publicSubnets[0].SubnetID
		}
	}

	// Public route tables
	if vpc.State.VPCType.HasFirewall() {
		for _, az := range vpc.State.AvailabilityZones.InOrder() {
			if az.PublicRouteTableID == "" {
				rtName := routeTableName(ctx.VPCName, az.Name, "", database.SubnetTypePublic)
				rt, err := ctx.CreateRouteTable(rtName)
				if err != nil {
					t.Log("Error creating public route table for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				rtID := aws.StringValue(rt.RouteTableId)

				az.PublicRouteTableID = rtID
				vpc.State.RouteTables[rtID] = &database.RouteTableInfo{RouteTableID: rtID, SubnetType: database.SubnetTypePublic}

				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = ctx.WaitForExistence(rtID, ctx.RouteTableExists)
				if err != nil {
					t.Log("Error creating public route table for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				ctx.Logger.Log("Created Route Table %s ", rtID)
				err = ctx.SetNameAndAutomated(rtID, rtName)
				if err != nil {
					t.Log("Error creating tags for public route table for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
	} else {
		if vpc.State.PublicRouteTableID == "" {
			rtName := sharedPublicRouteTableName(ctx.VPCName)
			rt, err := ctx.CreateRouteTable(rtName)
			if err != nil {
				t.Log("Error creating public route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			rtID := aws.StringValue(rt.RouteTableId)

			vpc.State.PublicRouteTableID = rtID
			vpc.State.RouteTables[rtID] = &database.RouteTableInfo{RouteTableID: rtID, SubnetType: database.SubnetTypePublic}

			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = ctx.WaitForExistence(rtID, ctx.RouteTableExists)
			if err != nil {
				t.Log("Error creating public route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			ctx.Logger.Log("Created Route Table %s", rtID)
			err = ctx.SetNameAndAutomated(rtID, rtName)
			if err != nil {
				t.Log("Error creating tags for public route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Public route table transit gateway routes. This step must be performed before changing subnet associations to avoid downtime when migrating V1 to V1Firewall or vice versa
	publicRTs := []*database.RouteTableInfo{}
	if vpc.State.VPCType.HasFirewall() {
		for _, az := range vpc.State.AvailabilityZones.InOrder() {
			publicRT, ok := vpc.State.RouteTables[az.PublicRouteTableID]
			if !ok {
				t.Log("No public route table info found for AZ %s and RT ID %q", az.Name, az.PublicRouteTableID)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			publicRTs = append(publicRTs, publicRT)
		}
	} else {
		publicRT, ok := vpc.State.RouteTables[vpc.State.PublicRouteTableID]
		if !ok {
			t.Log("No shared public route table info found for RT ID %q", vpc.State.PublicRouteTableID)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		publicRTs = append(publicRTs, publicRT)
	}
	for _, publicRT := range publicRTs {
		err = updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, managedAttachmentsByID, publicRT, database.SubnetTypePublic, database.Region(networkConfig.AWSRegion), getPrefixListRAM)
		if err != nil {
			t.Log("%s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	// Firewall route table
	if vpc.State.VPCType.HasFirewall() {
		if vpc.State.FirewallRouteTableID == "" {
			rtName := sharedFirewallRouteTableName(ctx.VPCName)
			rt, err := ctx.CreateRouteTable(rtName)
			if err != nil {
				t.Log("Error creating firewall route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			rtID := aws.StringValue(rt.RouteTableId)

			vpc.State.FirewallRouteTableID = rtID
			vpc.State.RouteTables[rtID] = &database.RouteTableInfo{RouteTableID: rtID, SubnetType: database.SubnetTypeFirewall}

			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = ctx.WaitForExistence(rtID, ctx.RouteTableExists)
			if err != nil {
				t.Log("Error creating firewall route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			ctx.Logger.Log("Created Route Table %s", rtID)
			err = ctx.SetNameAndAutomated(rtID, rtName)
			if err != nil {
				t.Log("Error creating tags for firewall route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	if networkConfig.ConnectPublic {
		// IGW: Resources
		if vpc.State.InternetGateway.InternetGatewayID == "" {
			igwName := internetGatewayName(ctx.VPCName)
			ctx.Logger.Log("Creating Internet Gateway")
			vpc.State.InternetGateway.InternetGatewayID, err = ctx.CreateInternetGateway(igwName)
			if err != nil {
				t.Log("Error creating Internet Gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = ctx.WaitForExistence(vpc.State.InternetGateway.InternetGatewayID, ctx.InternetGatewayExists)
			if err != nil {
				t.Log("Error creating Internet Gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			ctx.Logger.Log("Created Internet Gateway %s", vpc.State.InternetGateway.InternetGatewayID)
			err = ctx.SetNameAndAutomated(vpc.State.InternetGateway.InternetGatewayID, igwName)
			if err != nil {
				t.Log("Error creating tags for Internet Gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
		if !vpc.State.InternetGateway.IsInternetGatewayAttached {
			ctx.Logger.Log("Attaching Internet Gateway")
			err := ctx.AttachInternetGateway(vpc.State.InternetGateway.InternetGatewayID)
			if err != nil {
				t.Log("Error attaching Internet Gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			vpc.State.InternetGateway.IsInternetGatewayAttached = true
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
		if vpc.State.VPCType.HasFirewall() {
			//  create the IGW edge association RT
			if vpc.State.InternetGateway.RouteTableID == "" {
				rtName := igwRouteTableName(ctx.VPCName)
				rt, err := ctx.CreateRouteTable(rtName)
				if err != nil {
					t.Log("Error creating IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				rtID := aws.StringValue(rt.RouteTableId)

				vpc.State.InternetGateway.RouteTableID = rtID
				vpc.State.RouteTables[rtID] = &database.RouteTableInfo{
					RouteTableID:        rtID,
					EdgeAssociationType: database.EdgeAssociationTypeIGW,
				}

				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = ctx.WaitForExistence(rtID, ctx.RouteTableExists)
				if err != nil {
					t.Log("Error creating IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				ctx.Logger.Log("Created Route Table %s", rtID)
				err = ctx.SetNameAndAutomated(rtID, rtName)
				if err != nil {
					t.Log("Error creating tags for IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}

		// IGW: Routes
		if vpc.State.VPCType.HasFirewall() {
			// shared firewall RT's internet route targets the IGW
			firewallRT, ok := vpc.State.RouteTables[vpc.State.FirewallRouteTableID]
			if !ok {
				t.Log("No firewall route table info found for %q", vpc.State.FirewallRouteTableID)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			firewallRT.Routes, err = setRoute(ctx, vpc.State.FirewallRouteTableID, internetRoute, firewallRT.Routes, &database.RouteInfo{
				InternetGatewayID: vpc.State.InternetGateway.InternetGatewayID,
			})
			if err != nil {
				t.Log("Error updating firewall route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			publicSubnetIDtoCIDR, err := ctx.GetPublicSubnetIDtoCIDR(vpc.State.AvailabilityZones)
			if err != nil {
				t.Log("Error getting public subnet ID to CIDR: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			endpointIDByAZ, err := ctx.GetFirewallEndpointIDByAZ()
			if err != nil {
				t.Log("Error getting endpoints by AZ for firewall: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			igwRT, ok := vpc.State.RouteTables[vpc.State.InternetGateway.RouteTableID]
			if !ok {
				t.Log("No IGW route table info found for ID %q", vpc.State.InternetGateway.RouteTableID)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			for _, az := range vpc.State.AvailabilityZones.InOrder() {
				// each AZ's public RT's internet route targets the firewall endpoint for that AZ
				publicRT, ok := vpc.State.RouteTables[az.PublicRouteTableID]
				if !ok {
					t.Log("No public route table info found for AZ %s and ID %q", az.Name, vpc.State.PublicRouteTableID)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				endpointID, ok := endpointIDByAZ[az.Name]
				if !ok {
					t.Log("No firewall endpoint ID found for AZ %s", az.Name)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				publicRT.Routes, err = setRoute(ctx, az.PublicRouteTableID, internetRoute, publicRT.Routes, &database.RouteInfo{
					VPCEndpointID: endpointID,
				})
				if err != nil {
					t.Log("Error updating public route table for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

				// for each AZ, the IGW RT has a route for the CIDRs of each public subnet in the AZ that targets the firewall endpoint for that AZ
				for _, publicSubnet := range az.Subnets[database.SubnetTypePublic] {
					publicCIDR, ok := publicSubnetIDtoCIDR[publicSubnet.SubnetID]
					if !ok {
						t.Log("No CIDR found for public subnet ID %s", publicSubnet.SubnetID)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					igwRT.Routes, err = setRoute(ctx, vpc.State.InternetGateway.RouteTableID, publicCIDR, igwRT.Routes, &database.RouteInfo{
						VPCEndpointID: endpointID,
					})
					if err != nil {
						t.Log("Error updating IGW route table: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					err = vpcWriter.UpdateState(vpc.State)
					if err != nil {
						t.Log("Error updating state: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				}
			}
		} else {
			// shared public RT's internet route targets the IGW
			publicRT, ok := vpc.State.RouteTables[vpc.State.PublicRouteTableID]
			if !ok {
				t.Log("No public route table info found for %q", vpc.State.PublicRouteTableID)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			publicRT.Routes, err = setRoute(ctx, vpc.State.PublicRouteTableID, internetRoute, publicRT.Routes, &database.RouteInfo{
				InternetGatewayID: vpc.State.InternetGateway.InternetGatewayID,
			})
			if err != nil {
				t.Log("Error updating public route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Route Table Associations

	// Firewall associations
	// We need to do this before changing public associations, or else egress from public RTs won't route correctly when migrating from V1 to V1Firewall
	if vpc.State.VPCType.HasFirewall() {
		for _, az := range vpc.State.AvailabilityZones.InOrder() {
			for _, firewallSubnet := range az.Subnets[database.SubnetTypeFirewall] {
				firewallSubnet.RouteTableAssociationID, err = ctx.EnsureRouteTableAssociationExists(vpc.State.FirewallRouteTableID, firewallSubnet.SubnetID)
				if err != nil {
					t.Log("Error ensuring route table association between subnet %s and firewall route table: %s", firewallSubnet.SubnetID, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err := ctx.WaitForExistence(firewallSubnet.RouteTableAssociationID, ctx.RouteTableAssociationExists)
				if err != nil {
					t.Log("Error waiting for firewall subnet route table association to exist: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

			}
		}
	}

	// IGW association
	// When migrating V1 to V1Firewall or vice versa, the cutover of ingress traffic happens here.
	if vpc.State.VPCType.HasFirewall() {
		if vpc.State.InternetGateway.RouteTableAssociationID == "" {
			vpc.State.InternetGateway.RouteTableAssociationID, err = ctx.EnsureRouteTableAssociationExists(vpc.State.InternetGateway.RouteTableID, vpc.State.InternetGateway.InternetGatewayID)
			if err != nil {
				t.Log("Error ensuring route table association between IGW %s and route table %s: %s", vpc.State.InternetGateway.InternetGatewayID, vpc.State.InternetGateway.RouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err := ctx.WaitForExistence(vpc.State.InternetGateway.RouteTableAssociationID, ctx.RouteTableAssociationExists)
			if err != nil {
				t.Log("Error waiting for IGW route table association to exist: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	} else {
		if vpc.State.InternetGateway.RouteTableAssociationID != "" {
			err := ctx.DisassociateRouteTable(vpc.State.InternetGateway.RouteTableAssociationID)
			if err != nil {
				t.Log("Error disassociating IGW route table: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			vpc.State.InternetGateway.RouteTableAssociationID = ""
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// Public associations
	// When migrating V1 to V1Firewall or vice versa, the cutover of egress traffic happens here, so other firewall routing needs to be done before this step
	for _, az := range vpc.State.AvailabilityZones.InOrder() {
		for _, publicInfra := range az.Subnets[database.SubnetTypePublic] {
			subnetID := publicInfra.SubnetID

			var rtID string
			if vpc.State.VPCType.HasFirewall() {
				rtID = az.PublicRouteTableID
			} else {
				rtID = vpc.State.PublicRouteTableID
			}

			publicInfra.RouteTableAssociationID, err = ctx.EnsureRouteTableAssociationExists(rtID, subnetID)
			if err != nil {
				t.Log("Error ensuring route table association between subnet %s and public route table %s: %s", subnetID, rtID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err := ctx.WaitForExistence(publicInfra.RouteTableAssociationID, ctx.RouteTableAssociationExists)
			if err != nil {
				t.Log("Error waiting for public subnet route table association to exist: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	// NAT Gateways and routes
	for _, az := range vpc.State.AvailabilityZones.InOrder() {
		// Need a standard private route table for the AZ
		if az.PrivateRouteTableID == "" {
			rtName := routeTableName(ctx.VPCName, az.Name, "", database.SubnetTypePrivate)
			rt, err := ctx.CreateRouteTable(rtName)
			if err != nil {
				t.Log("Error creating private route table for AZ %s: %s", az.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			rtID := aws.StringValue(rt.RouteTableId)

			az.PrivateRouteTableID = rtID
			vpc.State.RouteTables[rtID] = &database.RouteTableInfo{RouteTableID: rtID, SubnetType: database.SubnetTypePrivate}

			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = ctx.WaitForExistence(*rt.RouteTableId, ctx.RouteTableExists)
			if err != nil {
				t.Log("Error creating private route table for AZ %s: %s", az.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			ctx.Logger.Log("Created Route Table %s", *rt.RouteTableId)
			err = ctx.SetNameAndAutomated(*rt.RouteTableId, rtName)
			if err != nil {
				t.Log("Error creating tags for private route table for AZ %s: %s", az.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		privateRT, ok := vpc.State.RouteTables[az.PrivateRouteTableID]
		if !ok {
			t.Log("No private route table info found for %s", az.PrivateRouteTableID)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		err := updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, managedAttachmentsByID, privateRT, database.SubnetTypePrivate, database.Region(networkConfig.AWSRegion), getPrefixListRAM)
		if err != nil {
			t.Log("%s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		for _, pc := range peeringConnections {
			if pc.Config.ConnectPrivate {
				err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, pc.OtherVPCCIDRs, pc.State.PeeringConnectionID, privateRT)
				if err != nil {

					t.Log("%s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			} else {
				err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, []string{}, pc.State.PeeringConnectionID, privateRT)
				if err != nil {

					t.Log("%s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}

		for _, subnetType := range database.AllSubnetTypes() {
			subnets := az.Subnets[subnetType]
			if subnetType == database.SubnetTypePublic || subnetType == database.SubnetTypeFirewall {
				// handled separately
				continue
			}
			for _, subnet := range subnets {
				subnetID := subnet.SubnetID
				routeTableID := az.PrivateRouteTableID
				if subnetType != database.SubnetTypePrivate {
					// Should have a custom route table
					if subnet.CustomRouteTableID == "" {
						// Create missing route table
						rtName := routeTableName(ctx.VPCName, az.Name, subnet.GroupName, subnetType)
						rt, err := ctx.CreateRouteTable(rtName)
						if err != nil {
							t.Log("Error creating route table for subnet %s: %s", subnetID, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						rtID := aws.StringValue(rt.RouteTableId)

						subnet.CustomRouteTableID = rtID
						vpc.State.RouteTables[rtID] = &database.RouteTableInfo{RouteTableID: rtID, SubnetType: subnetType}

						err = vpcWriter.UpdateState(vpc.State)
						if err != nil {
							t.Log("Error updating state: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						err = ctx.WaitForExistence(rtID, ctx.RouteTableExists)
						if err != nil {
							t.Log("Error creating route table for subnet %s: %s", subnetID, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						ctx.Logger.Log("Created Route Table %s", rtID)
						err = ctx.SetNameAndAutomated(rtID, rtName)
						if err != nil {
							t.Log("Error creating tags for route table for subnet %s: %s", subnetID, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
					routeTableID = subnet.CustomRouteTableID
					customRT, ok := vpc.State.RouteTables[subnet.CustomRouteTableID]
					if !ok {
						t.Log("No custom route table info found for %s", subnet.CustomRouteTableID)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					err := updateTransitGatewayRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, managedAttachmentsByID, customRT, subnetType, database.Region(networkConfig.AWSRegion), getPrefixListRAM)
					if err != nil {
						t.Log("%s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					for _, pc := range peeringConnections {
						if stringInSlice(subnet.SubnetID, pc.SubnetIDs) {
							err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, pc.OtherVPCCIDRs, pc.State.PeeringConnectionID, customRT)
							if err != nil {

								t.Log("%s", err)
								setStatus(t, database.TaskStatusFailed)
								return
							}
						} else {
							err := updatePeeringConnectionRoutesForSubnet(ctx, vpc, vpcWriter, networkConfig, []string{}, pc.State.PeeringConnectionID, customRT)
							if err != nil {

								t.Log("%s", err)
								setStatus(t, database.TaskStatusFailed)
								return
							}
						}
					}
				}
				if subnet.RouteTableAssociationID == "" {
					subnet.RouteTableAssociationID, err = ctx.EnsureRouteTableAssociationExists(routeTableID, subnetID)
					if err != nil {
						t.Log("Error setting association between subnet %s and private route table %s: %s", subnetID, routeTableID, err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					err = vpcWriter.UpdateState(vpc.State)
					if err != nil {
						t.Log("Error updating state: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					err := ctx.WaitForExistence(subnet.RouteTableAssociationID, ctx.RouteTableAssociationExists)
					if err != nil {
						t.Log("Error waiting for %s subnet route table association to exist: %s", subnetType, err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
				}
			}
		}
		if networkConfig.ConnectPrivate {
			// Need an EIP
			if az.NATGateway.EIPID == "" {
				EIPName := eipName(ctx.VPCName, az.Name)
				az.NATGateway.EIPID, err = ctx.CreateEIP(EIPName)
				if err != nil {
					t.Log("Error creating EIP for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = ctx.WaitForExistence(az.NATGateway.EIPID, ctx.EIPExists)
				if err != nil {
					t.Log("Error creating EIP for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				ctx.Logger.Log("Created EIP %s", az.NATGateway.EIPID)
				err = ctx.SetNameAndAutomated(az.NATGateway.EIPID, EIPName)
				if err != nil {
					t.Log("Error creating tags for EIP for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			if az.NATGateway.NATGatewayID == "" {
				ng, err := ctx.CreateNATGateway(
					natGatewayName(ctx.VPCName, az.Name),
					az.NATGateway.EIPID,
					azToPublicSubnetID[az.Name])
				if err != nil {
					t.Log("Error creating NAT Gateway for AZ %s: %s", az.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				az.NATGateway.NATGatewayID = *ng.NatGatewayId
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}

			// Update routes

			err := setRouteAllNonPublic(ctx, az.AvailabilityZoneInfra, vpc, internetRoute, &database.RouteInfo{
				NATGatewayID: az.NATGateway.NATGatewayID,
			})
			if err != nil {
				t.Log("Error updating private routes for NAT Gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		} else {
			// Delete NAT Gateway and associated route and EIP.
			if az.PrivateRouteTableID != "" {
				err := destroyNATGatewayResourcesInAZ(ctx, vpcWriter, vpc, az.AvailabilityZoneInfra)
				if err != nil {
					t.Log("%s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
	}

	if !networkConfig.ConnectPublic {
		// remove internet route from public RTs
		for _, publicRT := range publicRTs {
			publicRT.Routes, err = setRoute(ctx, publicRT.RouteTableID, internetRoute, publicRT.Routes, nil)
			if err != nil {
				t.Log("Error updating public route table %s: %s", publicRT.RouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		// detach and delete IGW, and for V1Firewall disassociate and delete IGW RT
		if vpc.State.InternetGateway.InternetGatewayID != "" {
			if vpc.State.InternetGateway.IsInternetGatewayAttached {
				err := ctx.DetachInternetGateway(vpc.State.InternetGateway.InternetGatewayID)
				if err != nil {
					t.Log("Error detaching internet gateway: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				vpc.State.InternetGateway.IsInternetGatewayAttached = false
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			if vpc.State.VPCType.HasFirewall() {
				err := ctx.DisassociateRouteTable(vpc.State.InternetGateway.RouteTableAssociationID)
				if err != nil {
					t.Log("Error disassociating IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				vpc.State.InternetGateway.RouteTableAssociationID = ""
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

				err = ctx.DeleteRouteTable(vpc.State.InternetGateway.RouteTableID)
				if err != nil {
					t.Log("Error deleting IGW route table: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				delete(vpc.State.RouteTables, vpc.State.InternetGateway.RouteTableID)
				vpc.State.InternetGateway.RouteTableID = ""
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			err := ctx.DeleteInternetGateway(vpc.State.InternetGateway.InternetGatewayID)
			if err != nil {
				t.Log("Error deleting internet gateway: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			vpc.State.InternetGateway.InternetGatewayID = ""
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		// remove internet route from firewall RT
		if vpc.State.VPCType.HasFirewall() {
			firewallRT, ok := vpc.State.RouteTables[vpc.State.FirewallRouteTableID]
			if !ok {
				if err != nil {
					t.Log("No firewall route table found for RT ID %q: %s", vpc.State.FirewallRouteTableID, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
			firewallRT.Routes, err = setRoute(ctx, firewallRT.RouteTableID, internetRoute, firewallRT.Routes, nil)
			if err != nil {
				t.Log("Error updating firewall route table %s: %s", firewallRT.RouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	if !networkConfig.SkipVerify {
		issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifySpec{VerifyNetworking: true}, false)
		if err != nil {
			t.Log("Error verifying: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		err = vpcWriter.UpdateIssues(issues)
		if err != nil {
			t.Log("Error updating issues: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performUpdateVPCTypeTask(config *database.UpdateVPCTypeTaskData) {
	t := taskContext.Task
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Updating VPC type")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.AWSRegion, config.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateVPCType() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	vpc.State.VPCType = config.VPCType
	t.Log("Updated VPC type to %s", config.VPCType.String())

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		t.Log("Error updating state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performUpdateVPCNameTask(config *database.UpdateVPCNameTaskData) {
	t := taskContext.Task
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Updating VPC name")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.AWSRegion, config.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateVPCName() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	vpc.Name = config.VPCName

	err = vpcWriter.UpdateName(config.VPCName)
	if err != nil {
		t.Log("Error updating name: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Updated VPC name to %s", config.VPCName)

	taskName := fmt.Sprintf("Update VPC %s networking", vpc.ID)

	// Add/schedule sync tags task
	updateTagsTaskData := &database.TaskData{
		RepairVPCTaskData: &database.RepairVPCTaskData{
			VPCID:  vpc.ID,
			Region: vpc.Region,
			Spec:   database.VerifyAllSpec(),
		},
		AsUser: taskContext.AsUser,
	}
	taskBytes, err := json.Marshal(updateTagsTaskData)
	if err != nil {
		t.Log("Error marshalling network task data: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	updateTagsTask, err := taskContext.TaskDatabase.AddDependentVPCTask(vpc.AccountID, vpc.ID, taskName, taskBytes, database.TaskStatusQueued, t.GetID(), nil)
	if err != nil {
		t.Log("Error adding task: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	_, err = scheduleVPCTasks(taskContext.ModelsManager, taskContext.TaskDatabase, vpc.Region, vpc.AccountID, vpc.ID, taskContext.AsUser, database.TaskTypeRepair, database.VerifySpec{}, updateTagsTask, nil)
	if err != nil {
		t.Log("Error scheduling follow-up tasks: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	setStatus(t, database.TaskStatusSuccessful)
}

// only schedule this task after update networking, so there are no dependency exceptions due to routes/associations/etc.
func (taskContext *TaskContext) performDeleteUnusedResourcesTask(config *database.DeleteUnusedResourcesTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Deleting unused resources")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.AWSRegion, config.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanDeleteUnusedResources() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
		VPCName:          vpc.Name,
	}

	if vpc.State.VPCType == database.VPCTypeMigratingV1ToV1Firewall {
		if vpc.State.PublicRouteTableID != "" {
			// already disassociated by UpdateNetworking
			err := ctx.DeleteRouteTable(vpc.State.PublicRouteTableID)
			if err != nil {
				t.Log("Error deleting shared public route table %s: %s", vpc.State.PublicRouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			delete(vpc.State.RouteTables, vpc.State.PublicRouteTableID)
			vpc.State.PublicRouteTableID = ""
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	} else if vpc.State.VPCType == database.VPCTypeMigratingV1FirewallToV1 {
		if vpc.State.InternetGateway.RouteTableID != "" {
			// already disassociated by UpdateNetworking
			err := ctx.DeleteRouteTable(vpc.State.InternetGateway.RouteTableID)
			if err != nil {
				t.Log("Error deleting IGW route table %s: %s", vpc.State.InternetGateway.RouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			delete(vpc.State.RouteTables, vpc.State.InternetGateway.RouteTableID)
			vpc.State.InternetGateway.RouteTableID = ""
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		if vpc.State.FirewallRouteTableID != "" {
			for _, az := range vpc.State.AvailabilityZones {
				for _, subnet := range az.Subnets[database.SubnetTypeFirewall] {
					if subnet.RouteTableAssociationID != "" {
						err := ctx.DisassociateRouteTable(subnet.RouteTableAssociationID)
						if err != nil {
							t.Log("Error disassociating subnet %s from firewall route table %s: %s", subnet.SubnetID, vpc.State.FirewallRouteTableID, err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
						subnet.RouteTableAssociationID = ""
						err = vpcWriter.UpdateState(vpc.State)
						if err != nil {
							t.Log("Error updating state: %s", err)
							setStatus(t, database.TaskStatusFailed)
							return
						}
					}
				}
			}
			err := ctx.DeleteRouteTable(vpc.State.FirewallRouteTableID)
			if err != nil {
				t.Log("Error deleting firewall route table %s: %s", vpc.State.FirewallRouteTableID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}

			delete(vpc.State.RouteTables, vpc.State.FirewallRouteTableID)
			vpc.State.FirewallRouteTableID = ""
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}

		for azName, az := range vpc.State.AvailabilityZones {
			if az.PublicRouteTableID != "" {
				// already disassociated by UpdateNetworking
				err := ctx.DeleteRouteTable(az.PublicRouteTableID)
				if err != nil {
					t.Log("Error deleting public route table %s for AZ %s: %s", az.PublicRouteTableID, azName, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}

				delete(vpc.State.RouteTables, az.PublicRouteTableID)
				az.PublicRouteTableID = ""
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}

		err := ctx.DeleteFirewallResources()
		if err != nil {
			t.Log("Error deleting firewall resources: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		} else {
			vpc.State.Firewall = nil
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performUpdateResolverRulesTaskData(config *database.UpdateResolverRulesTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet
	asUser := taskContext.AsUser

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Updating VPC resolver rules")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.AWSRegion, config.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateResolverRules() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
		VPCName:          vpc.Name,
	}

	// Resolver Rule Set Attachments
	managedResolverRuleSetsByID := make(map[uint64]*database.ManagedResolverRuleSet)
	managedResolverRuleSets, err := taskContext.ModelsManager.GetManagedResolverRuleSets()
	if err != nil {
		t.Log("Error getting resolver rule configuration info: %s", err)
		t.SetStatus(database.TaskStatusFailed)
		return
	}
	for _, rr := range managedResolverRuleSets {
		managedResolverRuleSetsByID[rr.ID] = rr
	}

	err = handleResolverRuleAssociations(
		lockSet, ctx, vpc, vpcWriter, config, taskContext.ModelsManager, managedResolverRuleSetsByID,
		func(accountID string) (ramiface.RAMAPI, error) {
			access, err := taskContext.AWSAccountAccessProvider.AccessAccount(accountID, string(vpc.Region), asUser)
			if err != nil {
				return nil, fmt.Errorf("Error getting credentials for account %s: %s", accountID, err)
			}
			return access.RAM(), nil
		})
	if err != nil {
		t.Log("Error handling resolver rules: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifySpec{VerifyResolverRules: true}, false)
	if err != nil {
		t.Log("Error verifying: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func verifyTags(ctx *awsp.Context, resourceID string, expectedTags map[string]string, tags []*ec2.Tag, verifyType database.VerifyTypes, fix bool) ([]*database.Issue, error) {
	issues := []*database.Issue{}
	expectedTags["Automated"] = "true"
	missingTags := make(map[string]string)
	for k, v := range expectedTags {
		missingTags[k] = v
	}
	for _, tag := range tags {
		expectedValue, ok := expectedTags[*tag.Key]
		if ok {
			if *tag.Value != expectedValue {
				issues = append(issues, &database.Issue{
					Description: fmt.Sprintf("%s should be tagged %q=%q instead of %q=%q", resourceID, *tag.Key, expectedValue, *tag.Key, *tag.Value),
					IsFixable:   true,
					Type:        verifyType,
				})
			}
			delete(missingTags, *tag.Key)
		}
	}
	for k, v := range missingTags {
		issues = append(issues, &database.Issue{
			Description: fmt.Sprintf("%s should be tagged %q=%q but tag is missing", resourceID, k, v),
			IsFixable:   true,
			Type:        verifyType,
		})
	}
	if fix && len(issues) > 0 {
		return nil, ctx.Tag(resourceID, expectedTags)
	}
	return issues, nil
}

func verifyRoutes(info *database.RouteTableInfo, routeTable *ec2.RouteTable, affectedSubnetIDs []string, fix bool) ([]*database.Issue, error) {
	issues := []*database.Issue{}
	existingRoutes := make(map[string]*database.RouteInfo)

	routeInfos, err := createRouteInfos(routeTable, info)
	if err != nil {
		return nil, fmt.Errorf("Error creating route infos: %s", err)
	}
	for _, routeInfo := range routeInfos {
		existingRoutes[routeInfo.Destination] = routeInfo
	}

	updatedRoutes := []*database.RouteInfo{}
	for _, route := range info.Routes {
		existing := existingRoutes[route.Destination]
		if existing == nil {
			if !fix {
				issues = append(issues, &database.Issue{
					AffectedSubnetIDs: affectedSubnetIDs,
					Description: fmt.Sprintf(
						"Route table %s is missing route for %s",
						*routeTable.RouteTableId,
						route.Destination),
					IsFixable: true,
					Type:      database.VerifyNetworking,
				})
			}
		} else if *route != *existing {
			updatedRoutes = append(updatedRoutes, existing)
			if !fix {
				issues = append(issues, &database.Issue{
					AffectedSubnetIDs: affectedSubnetIDs,
					Description: fmt.Sprintf(
						"Route table %s has wrong route for %s",
						*routeTable.RouteTableId,
						route.Destination),
					IsFixable: true,
					Type:      database.VerifyNetworking,
				})
			}
		} else {
			updatedRoutes = append(updatedRoutes, route)
		}
	}
	if fix {
		info.Routes = updatedRoutes
	}
	return issues, nil
}

type checkTags bool

const (
	doCheckTags    checkTags = true
	doNotCheckTags checkTags = false
)

//		if route table is not found in AWS, verifyRouteTable modifies VPC state:
//			- clears the route table ID
//			- deletes the route table from the RouteTables map
//			- for the associatedSubnets:
//	 		- clears the route table ID
//				- clears the association ID
func verifyRouteTable(ctx *awsp.Context, vpc *database.VPC, rtID *string, associatedSubnets []*database.SubnetInfo, expectedName string, ct checkTags, fix bool) (*ec2.RouteTable, []*database.Issue, error) {
	issues := []*database.Issue{}
	associatedSubnetIDs := []string{}
	for _, subnet := range associatedSubnets {
		associatedSubnetIDs = append(associatedSubnetIDs, subnet.SubnetID)
	}
	var routeTable *ec2.RouteTable
	out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("route-table-id"),
				Values: []*string{rtID},
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if len(out.RouteTables) == 0 {
		if fix {
			delete(vpc.State.RouteTables, *rtID)
			*rtID = ""
			for _, info := range associatedSubnets {
				info.CustomRouteTableID = ""
				info.RouteTableAssociationID = ""
			}
		} else {
			issues = append(issues, &database.Issue{
				AffectedSubnetIDs: associatedSubnetIDs,
				Description:       fmt.Sprintf("Route table %s is missing", *rtID),
				IsFixable:         true,
				Type:              database.VerifyNetworking,
			})
		}
	} else {
		routeTable = out.RouteTables[0]
		if ct {
			tagIssues, err := verifyTags(ctx, *routeTable.RouteTableId, map[string]string{"Name": expectedName}, routeTable.Tags, database.VerifyNetworking, fix)
			if err != nil {
				return nil, nil, err
			}
			issues = append(issues, tagIssues...)
		}
	}

	if routeTable != nil {
		info, ok := vpc.State.RouteTables[*rtID]
		if !ok {
			return nil, nil, fmt.Errorf("No route table info found for route table %q", *rtID)
		}
		routeIssues, err := verifyRoutes(info, routeTable, associatedSubnetIDs, fix)
		if err != nil {
			return nil, nil, fmt.Errorf("Error verifying routes: %s", err)
		}
		issues = append(issues, routeIssues...)
	}

	return routeTable, issues, nil
}

// Removes all issues from existingIssues matching updateTypes and then appends newIssues
func mergeIssues(existingIssues, newIssues []*database.Issue, updateTypes database.VerifyTypes) []*database.Issue {
	merged := []*database.Issue{}
	for _, issue := range existingIssues {
		if !updateTypes.Includes(issue.Type) {
			merged = append(merged, issue)
		}
	}
	merged = append(merged, newIssues...)
	for _, issue := range merged {
		if issue.Type == 0 {
			// log warning on server side
			log.Printf("Warning: issue %q has no type", issue.Description)
		}
	}
	return merged
}

// returned issues will be based on vpc.Issues, with only the issue types specified
// in verifySpec replaced with new issues.
func (taskContext *TaskContext) synchronizeRouteTableState(ctx *awsp.Context, vpc *database.VPC, vpcWriter database.VPCWriter) error {
	modelsManager := taskContext.ModelsManager

	managedAttachments, err := modelsManager.GetManagedTransitGatewayAttachments()
	if err != nil {
		return fmt.Errorf("Error getting transit gateway configuration info: %s", err)
	}

	routeDestination := func(r *ec2.Route) string {
		if r.DestinationCidrBlock != nil {
			return aws.StringValue(r.DestinationCidrBlock)
		}
		if r.DestinationPrefixListId != nil {
			return aws.StringValue(r.DestinationPrefixListId)
		}
		return ""
	}

	routeInState := func(routeInAWS *ec2.Route, routesInState []*database.RouteInfo) bool {
		for _, routeInState := range routesInState {
			if routeInState.Destination == routeDestination(routeInAWS) && routeInState.TransitGatewayID == aws.StringValue(routeInAWS.TransitGatewayId) {
				return true
			}
		}
		return false
	}

	for rtbid, rtInState := range vpc.State.RouteTables {
		out, err := ctx.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("route-table-id"),
					Values: []*string{&rtbid},
				},
			},
		})
		if err != nil {
			return err
		}
		if len(out.RouteTables) != 1 {
			taskContext.Task.Log("Unexpected number of route tables matching %s", rtbid)
		}

		routesToAdd := make(map[string]*database.RouteInfo)

		// Use AWS route tables as source of truth - this should only be one loop
		for _, rtInAWS := range out.RouteTables {
			// Route table isn't in state, but exists in aws, we shouldn't import it
			if _, ok := vpc.State.RouteTables[aws.StringValue(rtInAWS.RouteTableId)]; !ok {
				taskContext.Task.Log("Route table %s not in state, but exists in AWS", rtbid)
				break
			}
			// Check all the routes on this table within AWS
			for _, routeInAWS := range rtInAWS.Routes {
				// Check TGWs from managed attachments
				for _, attachment := range managedAttachments {
					// This route points at a TGW we manage
					if routeInAWS.TransitGatewayId != nil && aws.StringValue(routeInAWS.TransitGatewayId) == attachment.TransitGatewayID {
						// This route is configured in a template
						if routeDestination(routeInAWS) != "" && stringInSlice(routeDestination(routeInAWS), attachment.Routes) {
							// Not currently in state
							if !routeInState(routeInAWS, rtInState.Routes) {
								routesToAdd[routeDestination(routeInAWS)] = &database.RouteInfo{
									TransitGatewayID: aws.StringValue(routeInAWS.TransitGatewayId),
									Destination:      routeDestination(routeInAWS),
								}
								break
							}
						}
					}
				}
			}
		}
		for _, routeToAdd := range routesToAdd {
			rtInState.Routes = append(rtInState.Routes, routeToAdd)
		}
	}

	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		return fmt.Errorf("Error updating state: %s", err)
	}
	return nil
}

// returned issues will be based on vpc.Issues, with only the issue types specified
// in verifySpec replaced with new issues.
func (taskContext *TaskContext) verifyState(ctx *awsp.Context, vpc *database.VPC, vpcWriter database.VPCWriter, verifySpec database.VerifySpec, fix bool) ([]*database.Issue, error) {
	modelsManager := taskContext.ModelsManager
	cmsNet := taskContext.CMSNet
	issues := make([]*database.Issue, 0)
	asUser := taskContext.AsUser

	if verifySpec.VerifyResolverRules {
		if vpc.State.VPCType.CanUpdateResolverRules() {
			filteredResolverRuleAssociations := []*database.ResolverRuleAssociation{}
			for _, ruleAssociation := range vpc.State.ResolverRuleAssociations {
				out, err := ctx.R53R().ListResolverRuleAssociations(
					&route53resolver.ListResolverRuleAssociationsInput{
						Filters: []*route53resolver.Filter{
							{
								Name:   aws.String("VPCId"),
								Values: []*string{&ctx.VPCID},
							},
							{
								Name:   aws.String("ResolverRuleId"),
								Values: []*string{&ruleAssociation.ResolverRuleID},
							},
						},
					},
				)
				if err != nil {
					return nil, err
				}
				if len(out.ResolverRuleAssociations) != 1 {
					if !fix {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: nil,
							Description:       fmt.Sprintf("Resolver Rule Association for %s is missing", ruleAssociation.ResolverRuleID),
							IsFixable:         true,
							Type:              database.VerifyResolverRules,
						})
					}
				} else {
					if *out.ResolverRuleAssociations[0].Id != ruleAssociation.ResolverRuleAssociationID {
						if !fix {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nil,
								Description:       fmt.Sprintf("Resolver Rule Association mismatch for %s", ruleAssociation.ResolverRuleID),
								IsFixable:         true,
								Type:              database.VerifyResolverRules,
							})
						}
					} else {
						filteredResolverRuleAssociations = append(filteredResolverRuleAssociations, ruleAssociation)
					}
				}
			}
			if fix {
				vpc.State.ResolverRuleAssociations = filteredResolverRuleAssociations
			}
		} else {
			taskContext.Task.Log("Verification/repair of resolver rules unsupported on this VPC")
		}
	}

	// Verify the recording/state of the CIDRs associated with this VPC
	if verifySpec.VerifyCIDRs {
		cidrIssues, err := verifyCIDRs(ctx, modelsManager, vpc, fix)
		if err != nil {
			return nil, err
		}
		issues = append(issues, cidrIssues...)

		// If orchestration is configured, send a notification so any automation can fire (e.g. VPN API)
		if taskContext.Orchestration != nil && fix {
			taskContext.Task.Log("Notifying orchestration engine of changed CIDRs")
			err := taskContext.Orchestration.NotifyCIDRsChanged(vpc.AccountID, nil)
			if err != nil {
				taskContext.Task.Log("Error notifying orchestration engine of new CIDRs: %s", err)
			}
		}

	}

	// Logging: flow logs and query logs
	if verifySpec.VerifyLogging {
		if vpc.State.VPCType.CanUpdateLogging() {
			out, err := ctx.EC2().DescribeFlowLogs(&ec2.DescribeFlowLogsInput{
				Filter: []*ec2.Filter{
					{
						Name:   aws.String("resource-id"),
						Values: []*string{&vpc.ID},
					},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("Error listing flow logs: %s", err)
			}
			flowLogsByID := map[string]*ec2.FlowLog{}
			for _, flowLog := range out.FlowLogs {
				flowLogsByID[aws.StringValue(flowLog.FlowLogId)] = flowLog
			}
			if vpc.State.CloudWatchLogsFlowLogID != "" {
				if flowLogsByID[vpc.State.CloudWatchLogsFlowLogID] == nil {
					if fix {
						vpc.State.CloudWatchLogsFlowLogID = ""
					} else {
						issues = append(issues, &database.Issue{
							Description: fmt.Sprintf("Missing CloudWatch Logs flow log %s", vpc.State.CloudWatchLogsFlowLogID),
							IsFixable:   true,
							Type:        database.VerifyLogging,
						})
					}
				} else {
					if aws.StringValue(flowLogsByID[vpc.State.CloudWatchLogsFlowLogID].LogFormat) != flowLogFormat {
						if fix {
							del, err := ctx.EC2().DeleteFlowLogs(&ec2.DeleteFlowLogsInput{
								FlowLogIds: []*string{&vpc.State.CloudWatchLogsFlowLogID},
							})
							if err != nil {
								return nil, err
							}
							if len(del.Unsuccessful) > 0 {
								msg := ""
								if del.Unsuccessful[0].Error != nil {
									msg = aws.StringValue(del.Unsuccessful[0].Error.Message)
								}
								return nil, fmt.Errorf("Error deleting CloudWatch Logs flowlogs: %s", msg)
							}
							vpc.State.CloudWatchLogsFlowLogID = ""
						} else {
							issues = append(issues, &database.Issue{
								Description: fmt.Sprintf("Incorrect log format for CloudWatch Logs flow log %s", vpc.State.CloudWatchLogsFlowLogID),
								IsFixable:   true,
								Type:        database.VerifyLogging,
							})
						}
					}
				}
			}
			if vpc.State.S3FlowLogID != "" {
				if flowLogsByID[vpc.State.S3FlowLogID] == nil {
					if fix {
						vpc.State.S3FlowLogID = ""
					} else {
						issues = append(issues, &database.Issue{
							Description: fmt.Sprintf("Missing S3 flow log %s", vpc.State.S3FlowLogID),
							IsFixable:   true,
							Type:        database.VerifyLogging,
						})
					}
				} else {
					if aws.StringValue(flowLogsByID[vpc.State.S3FlowLogID].LogFormat) != flowLogFormat {
						if fix {
							del, err := ctx.EC2().DeleteFlowLogs(&ec2.DeleteFlowLogsInput{
								FlowLogIds: []*string{&vpc.State.S3FlowLogID},
							})
							if err != nil {
								return nil, err
							}
							if len(del.Unsuccessful) > 0 {
								msg := ""
								if del.Unsuccessful[0].Error != nil {
									msg = aws.StringValue(del.Unsuccessful[0].Error.Message)
								}
								return nil, fmt.Errorf("Error deleting CloudWatch Logs flowlogs: %s", msg)
							}
							vpc.State.S3FlowLogID = ""
						} else {
							issues = append(issues, &database.Issue{
								Description: fmt.Sprintf("Incorrect log format for S3 flow log %s", vpc.State.S3FlowLogID),
								IsFixable:   true,
								Type:        database.VerifyLogging,
							})
						}
					}
				}
			}

			if vpc.State.ResolverQueryLogConfigurationID != "" {
				_, err := ctx.R53R().GetResolverQueryLogConfig(&route53resolver.GetResolverQueryLogConfigInput{
					ResolverQueryLogConfigId: aws.String(vpc.State.ResolverQueryLogConfigurationID),
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "ResourceNotFoundException" {
						if fix {
							vpc.State.ResolverQueryLogConfigurationID = ""
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nil,
								Description:       fmt.Sprintf("Resolver Query Log configuration %s is missing", vpc.State.ResolverQueryLogConfigurationID),
								IsFixable:         true,
								Type:              database.VerifyLogging,
							})
						}
					} else {
						return nil, fmt.Errorf("Error getting resolver query log association by id: %s", err)
					}
				}
			}

			if vpc.State.ResolverQueryLogAssociationID != "" {
				out, err := ctx.R53R().GetResolverQueryLogConfigAssociation(&route53resolver.GetResolverQueryLogConfigAssociationInput{
					ResolverQueryLogConfigAssociationId: aws.String(vpc.State.ResolverQueryLogAssociationID),
				})
				if err != nil {
					if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "ResourceNotFoundException" {
						if fix {
							vpc.State.ResolverQueryLogAssociationID = ""
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nil,
								Description:       fmt.Sprintf("Resolver Query Log association %s is missing", vpc.State.ResolverQueryLogAssociationID),
								IsFixable:         true,
								Type:              database.VerifyLogging,
							})
						}
					} else {
						return nil, fmt.Errorf("Error getting resolver query log association by id: %s", err)
					}
				} else {
					if out.ResolverQueryLogConfigAssociation.Error != nil && aws.StringValue(out.ResolverQueryLogConfigAssociation.Error) != "NONE" {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: nil,
							Description:       fmt.Sprintf("Resolver Query Log association %s has an error: %s", vpc.State.ResolverQueryLogAssociationID, aws.StringValue(out.ResolverQueryLogConfigAssociation.ErrorMessage)),
							IsFixable:         false,
							Type:              database.VerifyLogging,
						})
					} else if aws.StringValue(out.ResolverQueryLogConfigAssociation.ResourceId) != vpc.ID {
						if fix {
							vpc.State.ResolverQueryLogAssociationID = ""
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nil,
								Description:       fmt.Sprintf("Resolver Query Log association %s is associated with the wrong VPC: %s", vpc.State.ResolverQueryLogAssociationID, vpc.ID),
								IsFixable:         true,
								Type:              database.VerifyLogging,
							})
						}
					}
				}
			}
		} else {
			taskContext.Task.Log("Verification/repair of logging unsupported on this VPC")
		}
	}

	if verifySpec.VerifyNetworking {
		publicSubnetIDs := []string{}
		nonPublicSubnetIDs := []string{}
		for _, az := range vpc.State.AvailabilityZones {
			for subnetType, subnets := range az.Subnets {
				for _, subnet := range subnets {
					if subnetType == database.SubnetTypePublic {
						publicSubnetIDs = append(publicSubnetIDs, subnet.SubnetID)
					} else {
						nonPublicSubnetIDs = append(nonPublicSubnetIDs, subnet.SubnetID)
					}
				}
			}
		}
		managedAttachmentsByID := make(map[uint64]*database.ManagedTransitGatewayAttachment)
		managedAttachments, err := modelsManager.GetManagedTransitGatewayAttachments()
		if err != nil {
			return nil, fmt.Errorf("Error getting transit gateway configuration info: %s", err)
		}
		for _, ma := range managedAttachments {
			managedAttachmentsByID[ma.ID] = ma
		}

		filteredTransitGatewayAttachments := []*database.TransitGatewayAttachment{}
		for _, tga := range vpc.State.TransitGatewayAttachments {
			out, err := ctx.EC2().DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("transit-gateway-attachment-id"),
						Values: []*string{&tga.TransitGatewayAttachmentID},
					},
					{
						Name: aws.String("state"),
						Values: []*string{
							aws.String("pending"),
							aws.String("available"),
							aws.String("modifying"),
							aws.String("pendingAcceptance"),
							aws.String("rollingBack"),
							aws.String("rejected"),
							aws.String("rejecting"),
						},
					},
				},
			})
			if err != nil {
				return nil, err
			}
			if len(out.TransitGatewayVpcAttachments) == 0 {
				if !fix {
					issues = append(issues, &database.Issue{
						AffectedSubnetIDs: nonPublicSubnetIDs,
						Description:       fmt.Sprintf("Missing transit gateway attachment %s", tga.TransitGatewayAttachmentID),
						IsFixable:         true,
						Type:              database.VerifyNetworking,
					})
				}
			} else {
				filteredTransitGatewayAttachments = append(filteredTransitGatewayAttachments, tga)
				transitGatewayAttachment := out.TransitGatewayVpcAttachments[0]
				// Check subnets
				actualSubnetIDs := aws.StringValueSlice(transitGatewayAttachment.SubnetIds)
				if fix {
					tga.SubnetIDs = actualSubnetIDs
				} else {
					for _, subnetID := range tga.SubnetIDs {
						found := false
						for _, attachedID := range actualSubnetIDs {
							if subnetID == attachedID {
								found = true
								break
							}
						}
						if !found {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: []string{subnetID},
								Description:       fmt.Sprintf("Transit gateway attachment %s does not include subnet %s", tga.TransitGatewayAttachmentID, subnetID),
								IsFixable:         true,
								Type:              database.VerifyNetworking,
							})
						}
					}
				}
				// Check name
				maName := generateMTGAName(tga.ManagedTransitGatewayAttachmentIDs, managedAttachmentsByID)
				expectedName := transitGatewayAttachmentName(vpc.Name, maName)
				tagIssues, err := verifyTags(ctx, *transitGatewayAttachment.TransitGatewayAttachmentId, map[string]string{"Name": expectedName}, transitGatewayAttachment.Tags, database.VerifyNetworking, fix)
				if err != nil {
					return nil, err
				}
				issues = append(issues, tagIssues...)

				// Check sharing
				share, err := modelsManager.GetTransitGatewayResourceShare(vpc.Region, tga.TransitGatewayID)
				if err != nil {
					return nil, err
				}
				if share != nil && share.AccountID != vpc.AccountID {
					shareARN := resourceShareARN(string(vpc.Region), share.AccountID, share.ResourceShareID)
					tgwArn := transitGatewayARN(string(vpc.Region), share.AccountID, tga.TransitGatewayID)
					out, err := ctx.RAM().ListResources(&ram.ListResourcesInput{
						ResourceShareArns: []*string{aws.String(shareARN)},
						ResourceOwner:     aws.String("OTHER-ACCOUNTS"),
					})
					if err != nil {
						return nil, fmt.Errorf("Error listing principals for share: %s", err)
					}
					found := false
					for _, resource := range out.Resources {
						if *resource.Arn == tgwArn {
							found = true
							break
						}
					}
					if !found && !fix {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: actualSubnetIDs,
							Description:       fmt.Sprintf("Transit gateway %s is no longer shared", tga.TransitGatewayID),
							IsFixable:         true,
							Type:              database.VerifyNetworking,
						})
					}
				}
			}
		}
		if fix {
			vpc.State.TransitGatewayAttachments = filteredTransitGatewayAttachments
		}

		if vpc.State.VPCType == database.VPCTypeLegacy {
			// Just check route tables
			for azName, az := range vpc.State.AvailabilityZones {
				for subnetType, subnets := range az.Subnets {
					for _, subnet := range subnets {
						if subnet.CustomRouteTableID != "" {
							_, rtIssues, err := verifyRouteTable(
								ctx,
								vpc,
								&subnet.CustomRouteTableID,
								[]*database.SubnetInfo{subnet},
								routeTableName(ctx.VPCName, azName, subnet.GroupName, subnetType),
								doNotCheckTags,
								fix)
							if err != nil {
								return nil, err
							}
							issues = append(issues, rtIssues...)
						}
					}
				}
			}

			if fix {
				err := vpcWriter.UpdateState(vpc.State)
				if err != nil {
					return nil, fmt.Errorf("Error updating state: %s", err)
				}
			}
		}

		if vpc.State.VPCType.IsV1Variant() {
			// Public route tables

			var sharedPublicRT *ec2.RouteTable
			if vpc.State.PublicRouteTableID != "" {
				rt, rtIssues, err := verifyRouteTable(
					ctx,
					vpc,
					&vpc.State.PublicRouteTableID,
					selectBySubnetType(vpc.State, database.SubnetTypePublic),
					sharedPublicRouteTableName(ctx.VPCName),
					doCheckTags,
					fix)
				if err != nil {
					return nil, err
				}
				issues = append(issues, rtIssues...)
				sharedPublicRT = rt
			}

			azToPublicRT := make(map[string]*ec2.RouteTable)
			for azName, az := range vpc.State.AvailabilityZones {
				if az.PublicRouteTableID != "" {
					rt, rtIssues, err := verifyRouteTable(
						ctx,
						vpc,
						&az.PublicRouteTableID,
						az.Subnets[database.SubnetTypePublic],
						routeTableName(ctx.VPCName, azName, "", database.SubnetTypePublic),
						doCheckTags,
						fix)
					if err != nil {
						return nil, err
					}
					issues = append(issues, rtIssues...)
					azToPublicRT[azName] = rt
				}
			}

			// IGW

			if vpc.State.InternetGateway.InternetGatewayID != "" {
				out, err := ctx.EC2().DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("internet-gateway-id"),
							Values: []*string{&vpc.State.InternetGateway.InternetGatewayID},
						},
					},
				})
				if err != nil {
					return nil, err
				}
				if len(out.InternetGateways) == 0 {
					if fix {
						vpc.State.InternetGateway.InternetGatewayID = ""
						vpc.State.InternetGateway.IsInternetGatewayAttached = false
					} else {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: publicSubnetIDs,
							Description:       fmt.Sprintf("Missing internet gateway %s", vpc.State.InternetGateway.InternetGatewayID),
							IsFixable:         true,
							Type:              database.VerifyNetworking,
						})
					}
				} else {
					expectedName := internetGatewayName(ctx.VPCName)
					internetGateway := out.InternetGateways[0]
					tagIssues, err := verifyTags(ctx, *internetGateway.InternetGatewayId, map[string]string{"Name": expectedName}, internetGateway.Tags, database.VerifyNetworking, fix)
					if err != nil {
						return nil, err
					}
					issues = append(issues, tagIssues...)

					isAttached := false
					for _, attachment := range internetGateway.Attachments {
						if *attachment.VpcId == ctx.VPCID {
							isAttached = true
						}
					}

					if fix {
						vpc.State.InternetGateway.IsInternetGatewayAttached = isAttached
					} else if vpc.State.InternetGateway.IsInternetGatewayAttached && !isAttached {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: publicSubnetIDs,
							Description:       "Internet gateway is not attached",
							IsFixable:         true,
							Type:              database.VerifyNetworking,
						})
					}
				}

				if vpc.State.VPCType.HasFirewall() {
					if vpc.State.InternetGateway.RouteTableID != "" {
						rt, rtIssues, err := verifyRouteTable(
							ctx,
							vpc,
							&vpc.State.InternetGateway.RouteTableID,
							nil,
							igwRouteTableName(ctx.VPCName),
							doCheckTags,
							fix)
						if err != nil {
							return nil, err
						}
						issues = append(issues, rtIssues...)

						associated := false
						if rt != nil {
							for _, assn := range rt.Associations {
								// when removing an IGW edge association, the association ID remains but the association status changes to 'disassociated'
								if aws.StringValue(assn.GatewayId) == vpc.State.InternetGateway.InternetGatewayID && aws.StringValue(assn.AssociationState.State) == ec2.AssociationStatusCodeAssociated {
									associated = true
								}
							}
						}
						if !associated {
							if fix {
								vpc.State.InternetGateway.RouteTableAssociationID = ""
							} else {
								issues = append(issues, &database.Issue{
									Description: fmt.Sprintf(
										"IWG %s is not associated with the IGW route table",
										vpc.State.InternetGateway.InternetGatewayID),
									IsFixable: true,
									Type:      database.VerifyNetworking,
								})
							}
						}
					}
				}
			}

			// Firewall RT

			var firewallRT *ec2.RouteTable

			if vpc.State.FirewallRouteTableID != "" {
				rt, rtIssues, err := verifyRouteTable(
					ctx,
					vpc,
					&vpc.State.FirewallRouteTableID,
					selectBySubnetType(vpc.State, database.SubnetTypeFirewall),
					sharedFirewallRouteTableName(ctx.VPCName),
					doCheckTags,
					fix)
				if err != nil {
					return nil, err
				}
				issues = append(issues, rtIssues...)
				firewallRT = rt
			}

			// Private zone

			for azName, az := range vpc.State.AvailabilityZones {
				var privateRouteTable *ec2.RouteTable
				var rtIssues []*database.Issue
				var err error
				if az.PrivateRouteTableID != "" {
					privateRouteTable, rtIssues, err = verifyRouteTable(
						ctx,
						vpc,
						&az.PrivateRouteTableID,
						az.Subnets[database.SubnetTypePrivate],
						routeTableName(ctx.VPCName, azName, "", database.SubnetTypePrivate),
						doCheckTags,
						fix)
					if err != nil {
						return nil, err
					}
					issues = append(issues, rtIssues...)
				}

				if az.NATGateway.EIPID != "" {
					out, err := ctx.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{
						Filters: []*ec2.Filter{
							{
								Name:   aws.String("allocation-id"),
								Values: []*string{&az.NATGateway.EIPID},
							},
						},
					})
					if err != nil {
						return nil, err
					}
					if len(out.Addresses) == 0 {
						if fix {
							az.NATGateway.EIPID = ""
							// No EIP means no NAT Gateway either.
							az.NATGateway.NATGatewayID = ""
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nonPublicSubnetIDs,
								Description:       fmt.Sprintf("EIP %s is missing", az.NATGateway.EIPID),
								IsFixable:         true,
								Type:              database.VerifyNetworking,
							})
						}
					} else {
						expectedName := eipName(ctx.VPCName, azName)
						eip := out.Addresses[0]
						tagIssues, err := verifyTags(ctx, *eip.AllocationId, map[string]string{"Name": expectedName}, eip.Tags, database.VerifyNetworking, fix)
						if err != nil {
							return nil, err
						}
						issues = append(issues, tagIssues...)
					}
				}

				if az.NATGateway.NATGatewayID != "" {
					out, err := ctx.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
						Filter: []*ec2.Filter{
							{
								Name:   aws.String("nat-gateway-id"),
								Values: []*string{&az.NATGateway.NATGatewayID},
							},
							{
								Name:   aws.String("state"),
								Values: []*string{aws.String("pending"), aws.String("available")},
							},
						},
					})
					if err != nil {
						return nil, err
					}
					if len(out.NatGateways) == 0 {
						if fix {
							az.NATGateway.NATGatewayID = ""
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nonPublicSubnetIDs,
								Description:       fmt.Sprintf("NAT Gateway %s is missing", az.NATGateway.NATGatewayID),
								IsFixable:         true,
								Type:              database.VerifyNetworking,
							})
						}
					} else {
						expectedName := natGatewayName(ctx.VPCName, azName)
						natGateway := out.NatGateways[0]
						tagIssues, err := verifyTags(ctx, *natGateway.NatGatewayId, map[string]string{"Name": expectedName}, natGateway.Tags, database.VerifyNetworking, fix)
						if err != nil {
							return nil, err
						}
						issues = append(issues, tagIssues...)
					}
				}

				for subnetType, subnets := range az.Subnets {
					for _, subnet := range subnets {
						subnetID := subnet.SubnetID
						out, err := ctx.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{
							Filters: []*ec2.Filter{
								{
									Name:   aws.String("subnet-id"),
									Values: []*string{&subnetID},
								},
							},
						})
						if err != nil {
							return nil, err
						}
						if len(out.Subnets) == 0 {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: []string{subnetID},
								Description:       fmt.Sprintf("Missing %s subnet %s", subnetType, subnetID),
								Type:              database.VerifyNetworking,
							})
							continue
						}
						sn := out.Subnets[0]
						if *sn.AvailabilityZone != azName {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: []string{subnetID},
								Description:       fmt.Sprintf("Subnet %s is in incorrect AZ %s", subnetID, *sn.AvailabilityZone),
								Type:              database.VerifyNetworking,
							})
							continue
						}

						expectedName := subnetName(ctx.VPCName, azName, subnet.GroupName, subnetType)
						expectedTags := map[string]string{
							"Name":      expectedName,
							"GroupName": subnet.GroupName,
							"use":       strings.ToLower(string(subnetType)),
							"stack":     vpc.Stack,
						}
						if subnetType == database.SubnetTypeUnroutable || subnetType == database.SubnetTypeFirewall {
							expectedTags[awsp.ForbidEC2Tag] = "true"
						}

						tagIssues, err := verifyTags(ctx, subnetID, expectedTags, sn.Tags, database.VerifyNetworking, fix)
						if err != nil {
							return nil, err
						}
						for _, issue := range tagIssues {
							issue.AffectedSubnetIDs = []string{subnetID}
						}
						issues = append(issues, tagIssues...)

						var routeTable *ec2.RouteTable
						if subnet.CustomRouteTableID != "" {
							routeTable, rtIssues, err = verifyRouteTable(
								ctx,
								vpc,
								&subnet.CustomRouteTableID,
								[]*database.SubnetInfo{subnet},
								routeTableName(ctx.VPCName, azName, subnet.GroupName, subnetType),
								doCheckTags,
								fix)
							if err != nil {
								return nil, err
							}
							issues = append(issues, rtIssues...)
						} else if subnetType == database.SubnetTypePrivate {
							routeTable = privateRouteTable
						} else if subnetType == database.SubnetTypeFirewall {
							routeTable = firewallRT
						} else if subnetType == database.SubnetTypePublic {
							if vpc.State.VPCType.HasFirewall() {
								routeTable = azToPublicRT[azName]
							} else {
								routeTable = sharedPublicRT
							}

						}

						if subnet.RouteTableAssociationID != "" {
							associationFound := false
							if routeTable != nil {
								for _, association := range routeTable.Associations {
									if aws.StringValue(association.SubnetId) == subnetID {
										associationFound = true
										if *association.RouteTableAssociationId != subnet.RouteTableAssociationID {
											if fix {
												subnet.RouteTableAssociationID = *association.RouteTableAssociationId
											} else {
												issues = append(issues, &database.Issue{
													AffectedSubnetIDs: []string{subnetID},
													Description: fmt.Sprintf(
														"Route table association ID (%s) does not match expected (%s)",
														*association.RouteTableAssociationId,
														subnet.RouteTableAssociationID),
													IsFixable: true,
													Type:      database.VerifyNetworking,
												})
											}
										}
									}
								}
							}

							if !associationFound {
								if fix {
									subnet.RouteTableAssociationID = ""
								} else {
									issues = append(issues, &database.Issue{
										AffectedSubnetIDs: []string{subnetID},
										Description: fmt.Sprintf(
											"Subnet %s is not associated with the expected route table",
											subnetID),
										IsFixable: true,
										Type:      database.VerifyNetworking,
									})
								}
							}
						}
					}
				}
			}
		}

		if vpc.State.VPCType.CanUpdatePeering() {
			filteredPeeringConnections := []*database.PeeringConnection{}
			for _, pc := range vpc.State.PeeringConnections {
				if pc.PeeringConnectionID == "" {
					continue
				}
				out, err := ctx.EC2().DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("vpc-peering-connection-id"),
							Values: []*string{&pc.PeeringConnectionID},
						},
						{
							Name: aws.String("status-code"),
							Values: []*string{
								aws.String("provisioning"),
								aws.String("active"),
							},
						},
					},
				})
				if err != nil {
					return nil, err
				}
				if len(out.VpcPeeringConnections) == 0 {
					if !fix {
						issues = append(issues, &database.Issue{
							AffectedSubnetIDs: []string{},
							Description:       fmt.Sprintf("Missing peering connection %s", pc.PeeringConnectionID),
							IsFixable:         true,
							Type:              database.VerifyNetworking,
						})
					}
				} else {
					filteredPeeringConnections = append(filteredPeeringConnections, pc)

					requester, err := modelsManager.GetVPC(pc.RequesterRegion, pc.RequesterVPCID)
					if err != nil {
						return nil, err
					}
					accepter, err := modelsManager.GetVPC(pc.AccepterRegion, pc.AccepterVPCID)
					if err != nil {
						return nil, err
					}
					expectedTags := map[string]string{"Name": peeringConnectionName(requester.Name, accepter.Name)}
					tagIssues, err := verifyTags(ctx, aws.StringValue(out.VpcPeeringConnections[0].VpcPeeringConnectionId), expectedTags, out.VpcPeeringConnections[0].Tags, database.VerifyNetworking, fix)
					if err != nil {
						return nil, err
					}
					issues = append(issues, tagIssues...)

					status := aws.StringValue(out.VpcPeeringConnections[0].Status.Code)
					if pc.IsAccepted && !stringInSlice(
						status,
						[]string{
							ec2.VpcPeeringConnectionStateReasonCodeActive,
							ec2.VpcPeeringConnectionStateReasonCodeProvisioning,
						}) {
						if fix {
							pc.IsAccepted = false
						} else {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nonPublicSubnetIDs,
								Description:       fmt.Sprintf("Peering connection %s has status %s", pc.PeeringConnectionID, status),
								IsFixable:         true,
								Type:              database.VerifyNetworking,
							})
						}
					}
				}
			}
			if fix {
				vpc.State.PeeringConnections = filteredPeeringConnections
			}
		}

		// Firewall

		if vpc.State.VPCType.HasFirewall() {
			if vpc.State.Firewall != nil {
				out, err := ctx.NetworkFirewall().ListFirewalls(&networkfirewall.ListFirewallsInput{
					VpcIds: []*string{aws.String(ctx.VPCID)},
				})
				if err != nil {
					return nil, fmt.Errorf("Error listing firewalls: %s", err)
				}

				found := false
				for _, fw := range out.Firewalls {
					name := aws.StringValue(fw.FirewallName)
					if name == ctx.FirewallName() {
						found = true
					}
				}
				if !found {
					if fix {
						vpc.State.Firewall = nil
					} else {
						issues = append(issues, &database.Issue{
							Description: fmt.Sprintf("Firewall %s is missing", ctx.FirewallName()),
							IsFixable:   true,
							Type:        database.VerifyNetworking,
						})
					}
				} else {
					currentIDs, err := ctx.GetSubnetAssociationsForFirewall()
					if err != nil {
						return nil, fmt.Errorf("Error getting subnet associations for firewall: %s", err)
					}
					if !sameStringSlice(currentIDs, vpc.State.Firewall.AssociatedSubnetIDs) {
						if fix {
							vpc.State.Firewall.AssociatedSubnetIDs = currentIDs
						} else {
							issues = append(issues, &database.Issue{
								Description: fmt.Sprintf("Firewall %s has incorrect subnet associations", ctx.FirewallName()),
								IsFixable:   true,
								Type:        database.VerifyNetworking,
							})
						}
					}
				}
			}
		}

		// VPC tags

		out, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc.ID}),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		expectedTags := map[string]string{
			"Name":  vpc.Name,
			"stack": vpc.Stack,
		}
		if vpc.State.VPCType.HasFirewall() {
			expectedTags[awsp.FirewallTypeKey] = awsp.FirewallTypeValue
		}
		tagIssues, err := verifyTags(ctx, aws.StringValue(out.Vpcs[0].VpcId), expectedTags, out.Vpcs[0].Tags, database.VerifyNetworking, fix)
		if err != nil {
			return nil, err
		}
		issues = append(issues, tagIssues...)
	}

	// CMSNet connections
	if verifySpec.VerifyCMSNet {
		if vpc.State.VPCType.CanUpdateCMSNet() {
			if cmsNet.SupportsRegion(vpc.Region) {
				broken, err := cmsNet.GetBrokenActivations(vpc.AccountID, vpc.Region, vpc.ID, asUser)
				if err != nil {
					return nil, fmt.Errorf("Error getting CMSNet issues: %s", err)
				}
				for _, activation := range broken {
					issues = append(issues, &database.Issue{
						AffectedSubnetIDs: []string{activation.SubnetID},
						Description: fmt.Sprintf(
							"Subnet %s's connection to CMSNet CIDR %s is broken",
							activation.SubnetID,
							activation.DestinationCIDR),
						IsFixable: false,
						Type:      database.VerifyCMSNet,
					})
				}
			}
		} else {
			taskContext.Task.Log("Verification/repair of cmsnet unsupported on this VPC")
		}
	}

	// Security groups
	if verifySpec.VerifySecurityGroups {
		if vpc.State.VPCType.CanUpdateSecurityGroups() {
			templatesByID := map[uint64]*database.SecurityGroupTemplate{}
			sets, err := modelsManager.GetSecurityGroupSets()
			if err != nil {
				return nil, fmt.Errorf("Failed to load security group sets: %s", err)
			}
			for _, set := range sets {
				for _, group := range set.Groups {
					templatesByID[group.ID] = group
				}
			}
			keepSecurityGroups := []*database.SecurityGroup{}
			for _, sg := range vpc.State.SecurityGroups {
				if sg.SecurityGroupID != "" {
					out, err := ctx.EC2().DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
						Filters: []*ec2.Filter{
							{
								Name:   aws.String("group-id"),
								Values: []*string{&sg.SecurityGroupID},
							},
						},
					})
					if err != nil {
						return nil, err
					}
					if len(out.SecurityGroups) == 0 {
						if !fix {
							issues = append(issues, &database.Issue{
								AffectedSubnetIDs: nil,
								Description:       fmt.Sprintf("Security group %s is missing", sg.SecurityGroupID),
								IsFixable:         true,
								Type:              database.VerifySecurityGroups,
							})
						}
					} else {
						keepSecurityGroups = append(keepSecurityGroups, sg)

						config := templatesByID[sg.TemplateID]
						if config != nil {
							expectedName := config.Name

							tagIssues, err := verifyTags(ctx, sg.SecurityGroupID, map[string]string{"Name": expectedName}, out.SecurityGroups[0].Tags, database.VerifySecurityGroups, fix)
							if err != nil {
								return nil, err
							}
							issues = append(issues, tagIssues...)
						}

						existingRules := []*database.SecurityGroupRule{}
						// Convert existing rules to *database.SecurityGroupRule
						permToRules := func(perm *ec2.IpPermission, isEgress bool) []*database.SecurityGroupRule {
							rules := []*database.SecurityGroupRule{}
							for _, p := range perm.UserIdGroupPairs {
								rule := &database.SecurityGroupRule{
									Description: aws.StringValue(p.Description),
									IsEgress:    isEgress,
									Protocol:    aws.StringValue(perm.IpProtocol),
									FromPort:    aws.Int64Value(perm.FromPort),
									ToPort:      aws.Int64Value(perm.ToPort),
									Source:      aws.StringValue(p.GroupId),
								}
								rules = append(rules, rule)
							}
							for _, r := range perm.IpRanges {
								rule := &database.SecurityGroupRule{
									Description: aws.StringValue(r.Description),
									IsEgress:    isEgress,
									Protocol:    aws.StringValue(perm.IpProtocol),
									FromPort:    aws.Int64Value(perm.FromPort),
									ToPort:      aws.Int64Value(perm.ToPort),
									Source:      aws.StringValue(r.CidrIp),
								}
								rules = append(rules, rule)
							}
							for _, r := range perm.Ipv6Ranges {
								rule := &database.SecurityGroupRule{
									Description:    aws.StringValue(r.Description),
									IsEgress:       isEgress,
									Protocol:       aws.StringValue(perm.IpProtocol),
									FromPort:       aws.Int64Value(perm.FromPort),
									ToPort:         aws.Int64Value(perm.ToPort),
									SourceIPV6CIDR: aws.StringValue(r.CidrIpv6),
								}
								rules = append(rules, rule)
							}
							for _, p := range perm.PrefixListIds {
								rule := &database.SecurityGroupRule{
									Description: aws.StringValue(p.Description),
									IsEgress:    isEgress,
									Protocol:    aws.StringValue(perm.IpProtocol),
									FromPort:    aws.Int64Value(perm.FromPort),
									ToPort:      aws.Int64Value(perm.ToPort),
									Source:      aws.StringValue(p.PrefixListId),
								}
								rules = append(rules, rule)
							}
							return rules
						}
						for _, perm := range out.SecurityGroups[0].IpPermissions {
							existingRules = append(existingRules, permToRules(perm, false)...)
						}
						for _, perm := range out.SecurityGroups[0].IpPermissionsEgress {
							existingRules = append(existingRules, permToRules(perm, true)...)
						}
						if fix {
							sg.Rules = existingRules
						} else {
							missingRules := []*database.SecurityGroupRule{}
							for _, desiredRule := range sg.Rules {
								found := false
								for eIdx, existingRule := range existingRules {
									if desiredRule.IsEgress == existingRule.IsEgress &&
										desiredRule.Protocol == existingRule.Protocol &&
										desiredRule.Source == existingRule.Source &&
										desiredRule.SourceIPV6CIDR == existingRule.SourceIPV6CIDR &&
										desiredRule.FromPort == existingRule.FromPort &&
										desiredRule.ToPort == existingRule.ToPort {
										found = true
										existingRules = append(existingRules[:eIdx], existingRules[eIdx+1:]...)
										break
									}
								}
								if !found {
									missingRules = append(missingRules, desiredRule)
								}
							}
							for _, rule := range missingRules {
								issues = append(issues, &database.Issue{
									Description: fmt.Sprintf(
										"Security group %s is missing rule %q",
										sg.SecurityGroupID,
										rule.Description),
									IsFixable: true,
									Type:      database.VerifySecurityGroups,
								})
							}
							if len(existingRules) > 0 {
								issues = append(issues, &database.Issue{
									Description: fmt.Sprintf(
										"Security group %s has extra rules",
										sg.SecurityGroupID),
									IsFixable: true,
									Type:      database.VerifySecurityGroups,
								})
							}
						}
					}
				}
			}
			if fix {
				vpc.State.SecurityGroups = keepSecurityGroups
			}
		} else {
			taskContext.Task.Log("Verification/repair of security groups unsupported on this VPC")
		}
	}

	if fix {
		err := vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return nil, fmt.Errorf("Error updating state: %s", err)
		}
	}

	return mergeIssues(vpc.Issues, issues, verifySpec.VerifyTypes()), nil
}

func (taskContext *TaskContext) performUpdateSecurityGroupsTask(config *database.UpdateSecurityGroupsTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet
	asUser := taskContext.AsUser

	setStatus(t, database.TaskStatusInProgress)
	setsByID := map[uint64]*database.SecurityGroupSet{}
	sets, err := taskContext.ModelsManager.GetSecurityGroupSets()
	if err != nil {
		t.Log("Failed to load security group sets: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	for _, set := range sets {
		setsByID[set.ID] = set
	}

	type securityGroup struct {
		State  *database.SecurityGroup
		Config *database.SecurityGroupTemplate
	}
	securityGroups := []*securityGroup{}

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, config.AWSRegion, config.VPCID)
	if err != nil {
		t.Log("Error getting VPC info: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanUpdateSecurityGroups() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            config.VPCID,
		VPCName:          vpc.Name,
	}

	for _, group := range vpc.State.SecurityGroups {
		securityGroups = append(securityGroups, &securityGroup{State: group})
	}

	configuredPLIDs := []string{}
	for _, setID := range config.SecurityGroupSetIDs {
		set, ok := setsByID[setID]
		if !ok {
			// Should never happen because setID should come from a database join
			t.Log("No security group set for configured ID %d", setID)
			setStatus(t, database.TaskStatusFailed)
			return
		}

		for _, template := range set.Groups {
			for _, r := range template.Rules {
				if awsp.IsPrefixListID(r.Source) {
					configuredPLIDs = append(configuredPLIDs, r.Source)
				}
			}

			found := false
			for _, sg := range securityGroups {
				if sg.State == nil {
					continue
				}
				if sg.State.TemplateID == template.ID {
					sg.Config = template
					found = true
					break
				}
			}
			if !found {
				securityGroups = append(securityGroups, &securityGroup{Config: template})
			}
		}
	}

	if len(configuredPLIDs) > 0 {
		region := string(config.AWSRegion)

		prefixListAccountID := prefixListAccountIDCommercial
		if config.AWSRegion.IsGovCloud() {
			prefixListAccountID = prefixListAccountIDGovCloud
		}

		access, err := taskContext.AWSAccountAccessProvider.AccessAccount(prefixListAccountID, region, asUser)
		if err != nil {
			t.Log("Error getting credentials for Prefix List account %s: %s", prefixListAccountID, err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
		plRAM := access.RAM()
		err = ensurePrefixListSharedWithAccount(ctx, plRAM, configuredPLIDs, config.AWSRegion, vpc.AccountID)
		if err != nil {
			t.Log("Error ensuring configured Prefix Lists are shared via RAM: %s", err)
			setStatus(t, database.TaskStatusFailed)
			return
		}
	}

	keepSecurityGroups := []*securityGroup{}
	// Delete security groups that are no longer configured
	for _, sg := range securityGroups {
		if sg.Config != nil {
			keepSecurityGroups = append(keepSecurityGroups, sg)
		} else if sg.State.SecurityGroupID != "" {
			_, err := ctx.EC2().DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
				GroupId: &sg.State.SecurityGroupID,
			})
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == "DependencyViolation" {
						t.Log("WARNING: Security Group %q is still in use. Please find interfaces using it and switch them to new security groups", sg.State.SecurityGroupID)
						keepSecurityGroups = append(keepSecurityGroups, sg)
						continue
					}
				}
				t.Log("Error deleting security group %q: %s\n", sg.State.SecurityGroupID, err)
				setStatus(t, database.TaskStatusFailed)
				return
			} else {
				t.Log("Deleted security group %q", sg.State.SecurityGroupID)
			}
		}
	}
	securityGroups = keepSecurityGroups
	vpc.State.SecurityGroups = nil
	for _, sg := range keepSecurityGroups {
		if sg.State != nil {
			vpc.State.SecurityGroups = append(vpc.State.SecurityGroups, sg.State)
		}
	}
	err = vpcWriter.UpdateState(vpc.State)
	if err != nil {
		t.Log("Error updating state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	for _, sg := range securityGroups {
		if sg.Config == nil {
			// No longer configured but we can't delete it yet. Don't
			// try to manage it.
			continue
		}
		if sg.State == nil {
			sg.State = &database.SecurityGroup{TemplateID: sg.Config.ID}
			vpc.State.SecurityGroups = append(vpc.State.SecurityGroups, sg.State)
		}
		if sg.State.SecurityGroupID == "" {
			out, err := ctx.EC2().CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
				GroupName:   &sg.Config.Name,
				Description: &sg.Config.Description,
				VpcId:       &config.VPCID,
			})
			if err != nil {
				t.Log("Error creating security group %q: %s", sg.Config.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			sg.State.SecurityGroupID = aws.StringValue(out.GroupId)
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			err = ctx.WaitForExistence(*out.GroupId, ctx.SecurityGroupExists)
			if err != nil {
				t.Log("Error creating security group %q: %s", sg.Config.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			t.Log("Created group %s (%s)", sg.State.SecurityGroupID, sg.Config.Name)
			// Delete default egress rule
			_, err = ctx.EC2().RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
				GroupId: out.GroupId,
				IpPermissions: []*ec2.IpPermission{
					{
						IpProtocol: aws.String("-1"),
						IpRanges: []*ec2.IpRange{
							{
								CidrIp: aws.String("0.0.0.0/0"),
							},
						},
					},
				},
			})
			if err != nil {
				t.Log("Error deleting default egress rule on new security group %s: %s", aws.StringValue(out.GroupId), err)

				_, err := ctx.EC2().DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
					GroupId: out.GroupId,
				})
				if err != nil {
					t.Log("Error deleting bad security group %s. Please delete manually", aws.StringValue(out.GroupId))
				}
				setStatus(t, database.TaskStatusFailed)
				return
			}
			tags := []*ec2.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(sg.Config.Name),
				},
				{
					Key:   aws.String("Automated"),
					Value: aws.String("true"),
				},
			}
			ctx.EC2().CreateTags(&ec2.CreateTagsInput{
				Resources: []*string{&sg.State.SecurityGroupID},
				Tags:      tags,
			})
		}
		existingRules := append([]*database.SecurityGroupRule{}, sg.State.Rules...)

		for _, desiredRule := range sg.Config.Rules {
			found := false
			for eIdx, existingRule := range existingRules {
				if desiredRule.IsEgress == existingRule.IsEgress &&
					desiredRule.Protocol == existingRule.Protocol &&
					desiredRule.Source == existingRule.Source &&
					desiredRule.SourceIPV6CIDR == existingRule.SourceIPV6CIDR &&
					desiredRule.FromPort == existingRule.FromPort &&
					desiredRule.ToPort == existingRule.ToPort {
					found = true
					existingRules = append(existingRules[:eIdx], existingRules[eIdx+1:]...)
					break
				}
			}
			if !found {
				// Create new rule
				perm := &ec2.IpPermission{
					FromPort:   &desiredRule.FromPort,
					ToPort:     &desiredRule.ToPort,
					IpProtocol: &desiredRule.Protocol,
				}

				if awsp.IsPrefixListID(desiredRule.Source) {
					perm.PrefixListIds = []*ec2.PrefixListId{
						{
							PrefixListId: &desiredRule.Source,
							Description:  &desiredRule.Description,
						},
					}
				} else {
					perm.IpRanges = []*ec2.IpRange{
						{
							CidrIp:      &desiredRule.Source,
							Description: &desiredRule.Description,
						},
					}
				}
				if desiredRule.IsEgress {
					_, err = ctx.EC2().AuthorizeSecurityGroupEgress(&ec2.AuthorizeSecurityGroupEgressInput{
						GroupId: &sg.State.SecurityGroupID,
						IpPermissions: []*ec2.IpPermission{
							perm,
						},
					})
				} else {
					_, err = ctx.EC2().AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
						GroupId: &sg.State.SecurityGroupID,
						IpPermissions: []*ec2.IpPermission{
							perm,
						},
					})
				}
				if err != nil {
					t.Log("Error adding rule %q to security group %q: %s", desiredRule.Description, sg.Config.Name, err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
				t.Log("Added %q to security group %s (%s): %s - %s/%d-%d", desiredRule.Description, sg.State.SecurityGroupID, sg.Config.Name, desiredRule.Source, desiredRule.Protocol, desiredRule.FromPort, desiredRule.ToPort)
				sg.State.Rules = append(sg.State.Rules, desiredRule)
				err = vpcWriter.UpdateState(vpc.State)
				if err != nil {
					t.Log("Error updating state: %s", err)
					setStatus(t, database.TaskStatusFailed)
					return
				}
			}
		}
		// Delete all existing rules that didn't match a desired rule
		for _, obsoleteRule := range existingRules {
			perm := &ec2.IpPermission{
				FromPort:   &obsoleteRule.FromPort,
				ToPort:     &obsoleteRule.ToPort,
				IpProtocol: &obsoleteRule.Protocol,
			}
			if awsp.IsPrefixListID(obsoleteRule.Source) {
				perm.PrefixListIds = []*ec2.PrefixListId{
					{
						PrefixListId: &obsoleteRule.Source,
					},
				}
			} else if awsp.IsSecurityGroupID(obsoleteRule.Source) {
				perm.UserIdGroupPairs = []*ec2.UserIdGroupPair{
					{
						GroupId: &obsoleteRule.Source,
					},
				}
			} else if obsoleteRule.SourceIPV6CIDR != "" {
				perm.Ipv6Ranges = []*ec2.Ipv6Range{
					{
						CidrIpv6: &obsoleteRule.SourceIPV6CIDR,
					},
				}
			} else {
				perm.IpRanges = []*ec2.IpRange{
					{
						CidrIp: &obsoleteRule.Source,
					},
				}
			}
			if obsoleteRule.IsEgress {
				_, err = ctx.EC2().RevokeSecurityGroupEgress(&ec2.RevokeSecurityGroupEgressInput{
					GroupId: &sg.State.SecurityGroupID,
					IpPermissions: []*ec2.IpPermission{
						perm,
					},
				})
			} else {
				_, err = ctx.EC2().RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
					GroupId: &sg.State.SecurityGroupID,
					IpPermissions: []*ec2.IpPermission{
						perm,
					},
				})
			}
			if err != nil {
				t.Log("Error removing rule %q from security group %q: %s", obsoleteRule.Description, sg.Config.Name, err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
			t.Log("Removed %q from security group %s (%s): %s - %s/%d-%d", obsoleteRule.Description, sg.State.SecurityGroupID, sg.Config.Name, obsoleteRule.Source, obsoleteRule.Protocol, obsoleteRule.FromPort, obsoleteRule.ToPort)
			for rIdx, rule := range sg.State.Rules {
				if rule == obsoleteRule {
					sg.State.Rules = append(sg.State.Rules[:rIdx], sg.State.Rules[rIdx+1:]...)
					err = vpcWriter.UpdateState(vpc.State)
					if err != nil {
						t.Log("Error updating state: %s", err)
						setStatus(t, database.TaskStatusFailed)
						return
					}
					break
				}
			}
			err = vpcWriter.UpdateState(vpc.State)
			if err != nil {
				t.Log("Error updating state: %s", err)
				setStatus(t, database.TaskStatusFailed)
				return
			}
		}
	}

	issues, err := taskContext.verifyState(ctx, vpc, vpcWriter, database.VerifySpec{VerifySecurityGroups: true}, false)
	if err != nil {
		t.Log("Error verifying: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	err = vpcWriter.UpdateIssues(issues)
	if err != nil {
		t.Log("Error updating issues: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	setStatus(t, database.TaskStatusSuccessful)
}

func (taskContext *TaskContext) performSynchronizeRouteTableStateFromAWSTask(synchronizeConfig *database.SynchronizeRouteTableStateFromAWSTaskData) {
	t := taskContext.Task
	awsAccountAccess := taskContext.BaseAWSAccountAccess
	lockSet := taskContext.LockSet

	setStatus(t, database.TaskStatusInProgress)
	t.Log("Syncing state from AWS")

	vpc, vpcWriter, err := taskContext.ModelsManager.GetOperableVPC(lockSet, synchronizeConfig.Region, synchronizeConfig.VPCID)
	if err != nil {
		t.Log("Error loading state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if vpc.State == nil {
		t.Log("VPC %s is not managed", vpc.ID)
		setStatus(t, database.TaskStatusFailed)
		return
	}
	if !vpc.State.VPCType.CanSynchronizeRouteTable() {
		t.Log("This is not allowed for this type of VPC")
		setStatus(t, database.TaskStatusFailed)
		return
	}

	ctx := &awsp.Context{
		AWSAccountAccess: awsAccountAccess,
		Logger:           t,
		VPCID:            synchronizeConfig.VPCID,
		VPCName:          vpc.Name,
	}

	err = taskContext.synchronizeRouteTableState(ctx, vpc, vpcWriter)
	if err != nil {
		t.Log("Error syncing VPC state: %s", err)
		setStatus(t, database.TaskStatusFailed)
		return
	}

	t.Log("Sync successful")
	setStatus(t, database.TaskStatusSuccessful)
}

func deleteCIDRs(modelsManager database.ModelsManager, cidrs []string, vpc *database.VPC) error {
	for _, cidr := range cidrs {
		err := modelsManager.DeleteVPCCIDR(vpc.ID, vpc.Region, cidr)
		if err != nil {
			return err
		}
	}

	return nil
}

func verifyCIDRs(ctx *awsp.Context, modelsManager database.ModelsManager, vpc *database.VPC, fix bool) ([]*database.Issue, error) {
	primaryCIDR, secondaryCIDRs, err := modelsManager.GetVPCCIDRs(vpc.ID, vpc.Region)
	if err != nil {
		return nil, err
	}

	vpcOutput, err := ctx.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{vpc.ID}),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if len(vpcOutput.Vpcs) != 1 {
		return nil, fmt.Errorf("Expected to get one VPC for %s, but got %d", vpc.ID, len(vpcOutput.Vpcs))
	}

	issues := []*database.Issue{}
	thisVPC := vpcOutput.Vpcs[0]
	awsPrimaryCIDR := aws.StringValue(thisVPC.CidrBlock)
	awsCIDRs := []string{awsPrimaryCIDR}

	if primaryCIDR != nil {
		if *primaryCIDR != awsPrimaryCIDR {
			issues = append(issues, &database.Issue{
				Description: fmt.Sprintf("Primary CIDR should be %s instead of %s", awsPrimaryCIDR, *primaryCIDR),
				IsFixable:   true,
				Type:        database.VerifyCIDRs,
			})
		}
	} else {
		issues = append(issues, &database.Issue{
			Description: fmt.Sprintf("Primary CIDR %s is missing in vpc-conf", awsPrimaryCIDR),
			IsFixable:   true,
			Type:        database.VerifyCIDRs,
		})
	}

	for _, associationSet := range thisVPC.CidrBlockAssociationSet {
		if aws.StringValue(associationSet.CidrBlockState.State) != "associated" {
			continue
		}

		found := false
		cidr := aws.StringValue(associationSet.CidrBlock)

		if cidr == awsPrimaryCIDR {
			continue
		}

		awsCIDRs = append(awsCIDRs, cidr)

		for _, dbCIDR := range secondaryCIDRs {
			if cidr == dbCIDR {
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, &database.Issue{
				Description: fmt.Sprintf("Secondary CIDR %s is missing in vpc-conf", cidr),
				IsFixable:   true,
				Type:        database.VerifyCIDRs,
			})
		}
	}

	for _, cidr := range secondaryCIDRs {
		found := false
		for _, awsCIDR := range awsCIDRs {
			if cidr == awsCIDR {
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, &database.Issue{
				Description: fmt.Sprintf("Extra secondary CIDR %s in vpc-conf", cidr),
				IsFixable:   true,
				Type:        database.VerifyCIDRs,
			})
		}
	}

	if fix {
		err = modelsManager.DeleteVPCCIDRs(vpc.ID, vpc.Region)
		if err != nil {
			return nil, err
		}

		for _, cidr := range awsCIDRs {
			err = modelsManager.InsertVPCCIDR(vpc.ID, vpc.Region, cidr, cidr == awsPrimaryCIDR)
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
	}

	return issues, nil
}

func updateFirewallSubnetAssociations(ctx *awsp.Context, vpc *database.VPC,
	vpcWriter database.VPCWriter, desiredIDs []string) error {
	currentIDs := vpc.State.Firewall.AssociatedSubnetIDs

	addIDs := []string{}
	for _, desired := range desiredIDs {
		if !stringInSlice(desired, vpc.State.Firewall.AssociatedSubnetIDs) {
			addIDs = append(addIDs, desired)
		}
	}
	removeIDs := []string{}
	for _, current := range vpc.State.Firewall.AssociatedSubnetIDs {
		if !stringInSlice(current, desiredIDs) {
			removeIDs = append(removeIDs, current)
		}
	}

	// remove first, since firewall will only allow one subnet association per AZ
	if len(removeIDs) > 0 {
		_, err := ctx.NetworkFirewall().DisassociateSubnets(&networkfirewall.DisassociateSubnetsInput{
			FirewallName: aws.String(ctx.FirewallName()),
			SubnetIds:    aws.StringSlice(removeIDs),
		})
		if err != nil {
			return fmt.Errorf("Error disassociating subnet IDs [%s] from firewall: %s", strings.Join(removeIDs, ", "), err)
		}
		ctx.Log(fmt.Sprintf("Disassociating subnet IDs [%s] from firewall", strings.Join(removeIDs, ", ")))

		keepIDs := []string{}
		for _, id := range currentIDs {
			keep := true
			for _, removeID := range removeIDs {
				if id == removeID {
					keep = false
				}
			}
			if keep {
				keepIDs = append(keepIDs, id)
			}
		}

		currentIDs = keepIDs
		vpc.State.Firewall.AssociatedSubnetIDs = currentIDs
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}

	if len(removeIDs) > 0 {
		err := ctx.WaitForFirewallSubnetDisassociations(removeIDs)
		if err != nil {
			return fmt.Errorf("Error waiting for firewall subnet disassociations to be done: %s", err)
		}
	}

	if len(addIDs) > 0 {
		_, err := ctx.NetworkFirewall().AssociateSubnets(&networkfirewall.AssociateSubnetsInput{
			FirewallName:   aws.String(ctx.FirewallName()),
			SubnetMappings: awsp.GenerateSubnetMappings(addIDs),
		})
		if err != nil {
			return fmt.Errorf("Error associating subnet IDs [%s] with firewall: %s", strings.Join(addIDs, ", "), err)
		}

		ctx.Log(fmt.Sprintf("Associating subnet IDs [%s] with firewall", strings.Join(addIDs, ", ")))
		currentIDs = append(currentIDs, addIDs...)
		vpc.State.Firewall.AssociatedSubnetIDs = currentIDs
		err = vpcWriter.UpdateState(vpc.State)
		if err != nil {
			return fmt.Errorf("Error updating state: %s", err)
		}
	}

	if len(addIDs) > 0 {
		// if we added associations, we need to wait for endpoints in the newly associated subnets since other resources depend on them
		err := ctx.WaitForFirewallSubnetAssociations(addIDs)
		if err != nil {
			return fmt.Errorf("Error waiting for firewall subnet associations to be ready: %s", err)
		}
	}

	return nil
}

func createRouteTableInfo(rt *ec2.RouteTable, subnetType database.SubnetType, edgeAssociationType database.EdgeAssociationType, rtID string) (*database.RouteTableInfo, error) {
	rtInfo := &database.RouteTableInfo{
		SubnetType:          subnetType,
		EdgeAssociationType: edgeAssociationType,
		RouteTableID:        rtID,
	}

	routeInfos, err := createRouteInfos(rt, rtInfo)
	if err != nil {
		return nil, fmt.Errorf("Error creating route infos: %s", err)
	}
	rtInfo.Routes = routeInfos

	return rtInfo, nil
}

func createRouteInfos(rt *ec2.RouteTable, rtInfo *database.RouteTableInfo) (routeInfos []*database.RouteInfo, err error) {
	if rtInfo.SubnetType != "" && rtInfo.EdgeAssociationType != "" {
		// route tables can have either a subnet or edge association but not both
		return nil, fmt.Errorf("Route table info has both an edge association type and subnet type")
	}

	for _, route := range rt.Routes {
		if aws.StringValue(route.Origin) == ec2.RouteOriginCreateRoute {
			var dest string

			if aws.StringValue(route.DestinationCidrBlock) != "" {
				dest = aws.StringValue(route.DestinationCidrBlock)
			} else if aws.StringValue(route.DestinationPrefixListId) != "" {
				dest = aws.StringValue(route.DestinationPrefixListId)
			}

			if dest == "" {
				continue
			}

			routeInfo := &database.RouteInfo{
				Destination:         dest,
				NATGatewayID:        aws.StringValue(route.NatGatewayId),
				TransitGatewayID:    aws.StringValue(route.TransitGatewayId),
				PeeringConnectionID: aws.StringValue(route.VpcPeeringConnectionId),
			}

			gatewayID := aws.StringValue(route.GatewayId)
			if gatewayID != "" {
				if awsp.IsInternetGatewayID(gatewayID) {
					routeInfo.InternetGatewayID = gatewayID
				} else if awsp.IsVPCEndpointID(gatewayID) {
					routeInfo.VPCEndpointID = gatewayID
				} else {
					return nil, fmt.Errorf("Could not assign unrecognized gateway ID %s", gatewayID)
				}
			}

			routeInfos = append(routeInfos, routeInfo)
		}
	}

	return routeInfos, nil
}
