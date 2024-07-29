package ipcontrol

import (
	"fmt"
	"sort"
	"strings"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

const (
	groupNamePrivate = "private"
	groupNamePublic  = "public"
)
const FirewallSubnetSize = 28

func GenerateTopLevelContainerNameBySubnetType(region, stack string, subnetType database.SubnetType) (string, error) {
	regionToContainer := map[string]string{
		"us-east-1":     "Commercial/East",
		"us-west-2":     "Commercial/West",
		"us-gov-west-1": "GovCloud/West",
		"us-gov-east-1": "GovCloud/East",
	}

	regionContainer := regionToContainer[region]
	if regionContainer == "" {
		return "", fmt.Errorf("Error: Region not supported: %s", region)
	}

	parentContainer := ""

	if subnetType.IsDefaultType() {
		stackToContainer := map[string]string{
			"dev":     "Development and Test",
			"sandbox": "Development and Test",
			"test":    "Development and Test",
			//S.M.
			"nonprod": "Development and Test",
			"mgmt":    "Production",
			"impl":    "Implementation",
			"qa":      "Development and Test",
			"prod":    "Production",
		}

		parentContainer = stackToContainer[stack]
		if parentContainer == "" {
			return "", fmt.Errorf("Error: Stack not supported: %q", stack)
		}
	} else {
		typeToContainerSuffix := map[database.SubnetType]string{
			database.SubnetTypeApp:        "App",
			database.SubnetTypeData:       "Data",
			database.SubnetTypeWeb:        "Web",
			database.SubnetTypeManagement: "Management",
			database.SubnetTypeSecurity:   "Security",
			database.SubnetTypeTransport:  "Transport",
			database.SubnetTypeShared:     "Shared",
			database.SubnetTypeSharedOC:   "Shared-OC",
		}
		parentContainer = typeToContainerSuffix[subnetType]
		if parentContainer == "" {
			return "", fmt.Errorf("Error: Subnet Type not supported: %q", subnetType)
		}
		if subnetType.HasSplitIPSpace() {
			prefix := "Lower"
			if stack == "prod" || stack == "mgmt" {
				prefix = "Prod"
			}
			parentContainer = prefix + "-" + parentContainer
		}
	}

	return "/Global/AWS/V4/" + regionContainer + "/" + parentContainer, nil
}

func chooseBlockSize(cfg *database.AllocateConfig, logger Logger) (privateBlockSize, publicBlockSize int) {
	privateIPs := uint64(cfg.NumPrivateSubnets * (1 << uint(32-cfg.PrivateSize)))
	publicIPs := uint64(0)
	if cfg.NumPublicSubnets > 0 {
		publicIPs = uint64(cfg.NumPublicSubnets * (1 << uint(32-cfg.PublicSize)))
	}

	combinedBlockSize := 32
	for uint64(1<<uint(32-combinedBlockSize)) < privateIPs+publicIPs {
		combinedBlockSize -= 1
		if combinedBlockSize < 0 {
			panic("Internal error")
		}
	}
	privateBlockSize = 32
	for uint64(1<<uint(32-privateBlockSize)) < privateIPs {
		privateBlockSize -= 1
		if privateBlockSize < 0 {
			panic("Internal error")
		}
	}
	publicBlockSize = 32
	for uint64(1<<uint(32-publicBlockSize)) < publicIPs {
		publicBlockSize -= 1
		if publicBlockSize < 0 {
			panic("Internal error")
		}
	}

	wastedIfCombined := 1<<uint(32-combinedBlockSize) - privateIPs - publicIPs
	wastedIfSeparate := 1<<uint(32-privateBlockSize) - privateIPs + 1<<uint(32-publicBlockSize) - publicIPs

	logger.Log("Wasted if public and private subnets combined into a /%d: %d IPs", combinedBlockSize, wastedIfCombined)
	logger.Log("Wasted if public and private subnets split into a /%d and a /%d: %d IPs", privateBlockSize, publicBlockSize, wastedIfSeparate)

	if wastedIfCombined <= wastedIfSeparate {
		return combinedBlockSize, -1
	}
	return
}

func (ctx *Context) allocateSubnet(subnet *SubnetConfig, availabilityZone string) (*SubnetInfo, error) {
	cfg := &ctx.AllocateConfig
	info := &ctx.VPCInfo
	info.Stack = cfg.Stack
	info.AvailabilityZones = cfg.AvailabilityZones

	subnetParentContainer := subnet.ParentContainer

	topLevelContainerName, err := GenerateTopLevelContainerNameBySubnetType(cfg.AWSRegion, cfg.Stack, database.SubnetType(subnet.SubnetType))
	if err != nil {
		return nil, err
	}
	vpcContainerPath := fmt.Sprintf("%s/%s-%s", topLevelContainerName, cfg.AccountID, cfg.VPCName)
	// Ensure that the vpc container path we construct is where the parent container defined at the subnet level lives
	if !strings.HasPrefix(subnetParentContainer, vpcContainerPath) {
		return nil, fmt.Errorf("Provided path %q is not within VPC scope: %q", subnetParentContainer, vpcContainerPath)
	}

	subnetName := fmt.Sprintf("%s-%c", subnet.GroupName, availabilityZone[len(availabilityZone)-1])
	err = ctx.IPAM.AddContainer(subnetParentContainer, subnetName, client.BlockTypeSubnet, cfg.AccountID)
	if err != nil {
		return nil, fmt.Errorf("Error creating subnet container: %s", err)
	}
	subnetContainerPath := fmt.Sprintf("%s/%s", subnetParentContainer, subnetName)
	ctx.Logger.Log("Created container %s", subnetContainerPath)
	ctx.containersCreated = append(ctx.containersCreated, subnetContainerPath)

	subnetBlock, err := ctx.IPAM.AllocateBlock(subnetParentContainer, subnetContainerPath, client.BlockTypeSubnet, subnet.SubnetSize, "Deployed")
	if err != nil {
		return nil, fmt.Errorf("Error creating subnet block for /%d: %s", subnet.SubnetSize, err)
	}
	ctx.blocksCreated = append(ctx.blocksCreated, &Block{
		name:      subnetBlock.BlockName,
		container: subnetBlock.Container,
	})
	ctx.Logger.Log("%s CIDR: %s", subnetName, subnetBlock.BlockName)
	newSubnet := &SubnetInfo{
		AvailabilityZone: availabilityZone,
		Type:             database.SubnetType(subnet.SubnetType),
		Name:             fmt.Sprintf("%s-%s", cfg.VPCName, subnetName),
		ContainerPath:    subnetContainerPath,
		CIDR:             subnetBlock.BlockName,
		GroupName:        subnet.GroupName,
	}
	return newSubnet, nil
}

func cidrSize(ips int) int {
	for i := 28; i >= 16; i-- {
		if ips <= (1 << (32 - i)) {
			return i
		}
	}
	return 0
}

func cidrIpCount(cidrLength int) int {
	return (1 << (32 - cidrLength))
}

// findOrCreateFreeSpace will check for a free block that can hold the first element of subnetSizes
// if that smallest allocation won't fit anywhere, allocate a block large enough to fit all of the requested subnetSizes
func (ctx *Context) findOrCreateFreeSpace(parentContainer string, subnetSizes []int) error {
	existingContainerHasSpace, err := ctx.IPAM.ContainerHasAvailableSpace(parentContainer, subnetSizes[0])
	if err != nil {
		return err
	}
	if !existingContainerHasSpace {
		ctx.Logger.Log("No existing free space to utilize, adding new VPC CIDR")
		ipCount := 0
		ctx.Logger.Log(fmt.Sprintf("%v", subnetSizes))
		for _, s := range subnetSizes {
			ipCount += cidrIpCount(s)
		}
		aggregateSize := cidrSize(ipCount)
		// Extrapolate the parent container of *this* container
		containerParts := strings.Split(parentContainer, "/")
		if len(containerParts) < 1 {
			return fmt.Errorf("%s is not a long enough scope", parentContainer)
		}
		myParentContainer := strings.Join(containerParts[:len(containerParts)-1], "/")
		block, err := ctx.IPAM.AllocateBlock(myParentContainer, parentContainer, client.BlockTypeVPC, aggregateSize, "Aggregate")
		if err != nil {
			return fmt.Errorf("Error creating VPC block /%d: %s", aggregateSize, err)
		}
		ctx.blocksCreated = append(ctx.blocksCreated, &Block{
			name:      block.BlockName,
			container: block.Container,
		})
		ctx.NewCIDRs = append(ctx.NewCIDRs, block.BlockName)
	}
	return nil
}

// Makes a subnet of all the given types in the defined AZ
func (ctx *Context) AddAZ(availabilityZone string, subnetConfigs []*SubnetConfig) error {
	if ctx.LockSet == nil || !ctx.LockSet.HasLock(database.TargetIPControlWrite) {
		return fmt.Errorf("We don't have the IPControl lock.  If you see this error in production please alert a developer!")
	}
	cfg := &ctx.AllocateConfig
	info := &ctx.VPCInfo
	info.Stack = cfg.Stack
	info.AvailabilityZones = cfg.AvailabilityZones

	// Check subnet sizes first
	for _, subnet := range subnetConfigs {
		if subnet.SubnetSize < 16 || subnet.SubnetSize > 28 {
			return fmt.Errorf("Invalid subnet size /%d for %s", subnet.SubnetSize, subnet.GroupName)
		}
	}

	subnetsByType := make(map[string][]*SubnetConfig)
	for _, subnet := range subnetConfigs {
		subnetType := subnet.SubnetType
		if subnet.SubnetType == string(database.SubnetTypePrivate) || subnet.SubnetType == string(database.SubnetTypePublic) {
			subnetType = "Unzoned"
		}
		if _, ok := subnetsByType[subnetType]; !ok {
			subnetsByType[subnetType] = make([]*SubnetConfig, 0)
		}
		subnetsByType[subnetType] = append(subnetsByType[subnetType], subnet)
	}

	ctx.Logger.Log("Building out subnets in AZ %s", availabilityZone)
	sortedSubnetTypes := make([]string, 0)
	for subnetType := range subnetsByType {
		sortedSubnetTypes = append(sortedSubnetTypes, subnetType)
	}
	// Sort the subnet types to always run it in order
	sort.SliceStable(sortedSubnetTypes, func(i, j int) bool {
		return sortedSubnetTypes[i] > sortedSubnetTypes[j]
	})
	for _, subnetType := range sortedSubnetTypes {
		subnets := subnetsByType[subnetType]
		if subnetType == string(database.SubnetTypeUnroutable) {
			continue
		}
		// Sort subnets by size first, so we act on the smallest allocations first
		sort.SliceStable(subnets, func(i, j int) bool {
			return subnets[i].SubnetSize > subnets[j].SubnetSize
		})
		for i, subnet := range subnets {
			remainingSubnetSizes := make([]int, 0)
			for _, s := range subnets[i:] {
				remainingSubnetSizes = append(remainingSubnetSizes, s.SubnetSize)
			}
			err := ctx.findOrCreateFreeSpace(subnet.ParentContainer, remainingSubnetSizes)
			if err != nil {
				return err
			}
			subnetInfo, err := ctx.allocateSubnet(subnet, availabilityZone)
			if err != nil {
				return err
			}
			info.NewSubnets = append(info.NewSubnets, subnetInfo)
		}
	}
	if len(ctx.NewCIDRs) > 0 {
		ctx.Logger.Log("New VPC CIDRs: %s", ctx.NewCIDRs)
	}
	return nil
}

// Makes a subnet of the given type in every AZ
func (ctx *Context) AddSubnets(subnetType database.SubnetType, subnetSize int, groupName string) error {
	if ctx.LockSet == nil || !ctx.LockSet.HasLock(database.TargetIPControlWrite) {
		return fmt.Errorf("We don't have the IPControl lock.  If you see this error in production please alert a developer!")
	}
	cfg := &ctx.AllocateConfig
	info := &ctx.VPCInfo
	info.Stack = cfg.Stack
	info.AvailabilityZones = cfg.AvailabilityZones

	if subnetSize < 16 || subnetSize > 29 {
		return fmt.Errorf("Invalid subnet size /%d", subnetSize)
	}

	for i, az := range cfg.AvailabilityZones {
		if len(az) < 1 {
			return fmt.Errorf("Invalid empty AZ name")
		}
		if az[len(az)-1] < 'a' || az[len(az)-1] > 'z' {
			return fmt.Errorf("Unexpected AZ name %s", az)
		}
		for j := 0; j < i; j++ {
			az2 := cfg.AvailabilityZones[j]
			if az[len(az)-1] == az2[len(az2)-1] {
				return fmt.Errorf("Conflicting AZ names %s and %s", az, az2)
			}
		}
	}

	vpcContainerName := fmt.Sprintf("%s-%s", cfg.AccountID, cfg.VPCName)
	topLevelContainer, err := GenerateTopLevelContainerNameBySubnetType(cfg.AWSRegion, cfg.Stack, subnetType)
	if err != nil {
		return err
	}
	vpcContainerPath := fmt.Sprintf("%s/%s", topLevelContainer, vpcContainerName)
	ctx.Logger.Log("Attempting to provision to container: %s", vpcContainerPath)
	existing, err := ctx.IPAM.GetContainersByName(vpcContainerPath)
	if err != nil {
		return fmt.Errorf("Error checking existing containers: %s", err)
	}
	if len(existing) == 0 {
		err = ctx.IPAM.AddContainer(topLevelContainer, vpcContainerName, client.BlockTypeVPC, cfg.AccountID)
		if err != nil {
			return fmt.Errorf("Error creating VPC container: %s", err)
		}
		ctx.Logger.Log("Created container %s", vpcContainerPath)
		ctx.containersCreated = append(ctx.containersCreated, vpcContainerPath)
		ctx.newVPCContainerPath = vpcContainerPath
	}

	// TODO: look for existing space in VPC block
	vpcBlockSize := subnetSize
	if len(cfg.AvailabilityZones) > 8 {
		return fmt.Errorf("More than 8 AZs is not supported")
	} else if len(cfg.AvailabilityZones) > 4 {
		vpcBlockSize -= 3
	} else if len(cfg.AvailabilityZones) > 2 {
		vpcBlockSize -= 2
	} else if len(cfg.AvailabilityZones) > 1 {
		vpcBlockSize -= 1
	}

	block, err := ctx.IPAM.AllocateBlock(topLevelContainer, vpcContainerPath, client.BlockTypeVPC, vpcBlockSize, "Aggregate")
	if err != nil {
		return fmt.Errorf("Error creating VPC block /%d: %s", vpcBlockSize, err)
	}
	ctx.blocksCreated = append(ctx.blocksCreated, &Block{
		name:      block.BlockName,
		container: block.Container,
	})

	info.NewCIDRs = append(info.NewCIDRs, block.BlockName)
	for _, az := range cfg.AvailabilityZones {
		subnetConfig := &SubnetConfig{
			SubnetType:      string(subnetType),
			SubnetSize:      subnetSize,
			GroupName:       groupName,
			ParentContainer: vpcContainerPath,
		}
		subnetInfo, err := ctx.allocateSubnet(subnetConfig, az)
		if err != nil {
			return err
		}
		info.NewSubnets = append(info.NewSubnets, subnetInfo)
	}

	return nil
}

func (ctx *Context) Allocate() error {
	if ctx.LockSet == nil || !ctx.LockSet.HasLock(database.TargetIPControlWrite) {
		return fmt.Errorf("We don't have the IPControl lock.  If you see this error in production please alert a developer!")
	}
	cfg := &ctx.AllocateConfig
	if cfg.NumPrivateSubnets == 0 {
		return fmt.Errorf("Must request at least one private subnet")
	}

	if cfg.NumPublicSubnets > 0 && (cfg.PublicSize < 16 || cfg.PublicSize > 29) {
		return fmt.Errorf("Invalid public subnet size /%d", cfg.PublicSize)
	}
	if cfg.PrivateSize < 16 || cfg.PrivateSize > 29 {
		return fmt.Errorf("Invalid private subnet size /%d", cfg.PrivateSize)
	}

	if cfg.NumPrivateSubnets > len(cfg.AvailabilityZones) {
		return fmt.Errorf("Requested %d private subnets but only %d AZs are available", cfg.NumPrivateSubnets, len(cfg.AvailabilityZones))
	}
	if cfg.NumPublicSubnets > len(cfg.AvailabilityZones) {
		return fmt.Errorf("Requested %d Public subnets but only %d AZs are available", cfg.NumPublicSubnets, len(cfg.AvailabilityZones))
	}
	for i, az := range cfg.AvailabilityZones {
		if len(az) < 1 {
			return fmt.Errorf("Invalid empty AZ name")
		}
		if az[len(az)-1] < 'a' || az[len(az)-1] > 'z' {
			return fmt.Errorf("Unexpected AZ name %s", az)
		}
		for j := 0; j < i; j++ {
			az2 := cfg.AvailabilityZones[j]
			if az[len(az)-1] == az2[len(az2)-1] {
				return fmt.Errorf("Conflicting AZ names %s and %s", az, az2)
			}
		}
	}

	info := &ctx.VPCInfo
	info.Stack = cfg.Stack
	info.AvailabilityZones = cfg.AvailabilityZones

	vpcContainerName := fmt.Sprintf("%s-%s", cfg.AccountID, cfg.VPCName)
	topLevelContainer, err := GenerateTopLevelContainerNameBySubnetType(cfg.AWSRegion, cfg.Stack, database.SubnetTypePrivate)
	if err != nil {
		return err
	}
	ctx.Logger.Log("Attempting to provision to container: %s/%s", topLevelContainer, vpcContainerName)
	err = ctx.IPAM.AddContainer(topLevelContainer, vpcContainerName, client.BlockTypeVPC, cfg.AccountID)
	if err != nil {
		return fmt.Errorf("Error creating VPC container: %s", err)
	}
	ctx.containersCreated = append(ctx.containersCreated, fmt.Sprintf("%s/%s", topLevelContainer, vpcContainerName))
	info.Name = cfg.VPCName
	ctx.newVPCContainerPath = fmt.Sprintf("%s/%s", topLevelContainer, vpcContainerName)

	allocatePrivate, allocatePublic := chooseBlockSize(&ctx.AllocateConfig, ctx.Logger)
	var privateContainerPath, publicContainerPath string
	if allocatePublic == -1 {
		privateContainerPath = fmt.Sprintf("%s/%s", topLevelContainer, vpcContainerName)
		publicContainerPath = privateContainerPath
	} else {
		privateContainerName := "private"
		err := ctx.IPAM.AddContainer(vpcContainerName, privateContainerName, client.BlockTypeVPC, cfg.AccountID)
		if err != nil {
			return fmt.Errorf("Error creating VPC container: %s", err)
		}
		ctx.containersCreated = append(ctx.containersCreated, fmt.Sprintf("%s/%s", vpcContainerName, privateContainerName))
		privateContainerPath = fmt.Sprintf("%s/%s/%s", topLevelContainer, vpcContainerName, privateContainerName)
		publicContainerName := "public"
		err = ctx.IPAM.AddContainer(vpcContainerName, publicContainerName, client.BlockTypeVPC, cfg.AccountID)
		if err != nil {
			return fmt.Errorf("Error creating VPC container: %s", err)
		}
		ctx.containersCreated = append(ctx.containersCreated, fmt.Sprintf("%s/%s", vpcContainerName, publicContainerName))
		publicContainerPath = fmt.Sprintf("%s/%s/%s", topLevelContainer, vpcContainerName, publicContainerName)
	}

	block, err := ctx.IPAM.AllocateBlock(topLevelContainer, privateContainerPath, client.BlockTypeVPC, allocatePrivate, "Aggregate")
	if err != nil {
		return fmt.Errorf("Error creating VPC block /%d: %s", allocatePrivate, err)
	}
	ctx.blocksCreated = append(ctx.blocksCreated, &Block{
		name:      block.BlockName,
		container: block.Container,
	})
	info.NewCIDRs = append(info.NewCIDRs, block.BlockName)
	if allocatePublic > -1 {
		block, err := ctx.IPAM.AllocateBlock(topLevelContainer, publicContainerPath, client.BlockTypeVPC, allocatePublic, "Aggregate")
		if err != nil {
			return fmt.Errorf("Error creating public VPC block /%d: %s", allocatePublic, err)
		}
		ctx.blocksCreated = append(ctx.blocksCreated, &Block{
			name:      block.BlockName,
			container: block.Container,
		})
		info.NewCIDRs = append(info.NewCIDRs, block.BlockName)
	}

	var firewallContainerPath string
	if cfg.AddFirewall {
		firewallContainerName := "firewall"
		firewallContainerPath = fmt.Sprintf("%s/%s/%s", topLevelContainer, vpcContainerName, firewallContainerName)
		err := ctx.IPAM.AddContainer(vpcContainerName, firewallContainerName, client.BlockTypeVPC, cfg.AccountID)
		if err != nil {
			return fmt.Errorf("Error creating firewall container: %s", err)
		}
		ctx.containersCreated = append(ctx.containersCreated, fmt.Sprintf("%s/%s", vpcContainerName, firewallContainerName))

		firewallBlockSize := FirewallSubnetSize
		// one firewall subnet for each public subnet
		if cfg.NumPublicSubnets > 8 {
			return fmt.Errorf("More than 8 AZs is not supported")
		} else if cfg.NumPublicSubnets > 4 {
			firewallBlockSize -= 3
		} else if cfg.NumPublicSubnets > 2 {
			firewallBlockSize -= 2
		} else if cfg.NumPublicSubnets > 1 {
			firewallBlockSize -= 1
		}

		block, err := ctx.IPAM.AllocateBlock(topLevelContainer, firewallContainerPath, client.BlockTypeVPC, firewallBlockSize, "Aggregate")
		if err != nil {
			return fmt.Errorf("Error creating firewall block: %s", err)
		}
		ctx.blocksCreated = append(ctx.blocksCreated, &Block{
			name:      block.BlockName,
			container: block.Container,
		})
		info.NewCIDRs = append(info.NewCIDRs, block.BlockName)
	}

	ctx.Logger.Log("VPC CIDRs: %s", info.NewCIDRs)

	for azIdx := 0; azIdx < cfg.NumPrivateSubnets; azIdx++ {
		subnetConfig := &SubnetConfig{
			SubnetType:      string(database.SubnetTypePrivate),
			SubnetSize:      cfg.PrivateSize,
			GroupName:       groupNamePrivate,
			ParentContainer: privateContainerPath,
		}
		az := cfg.AvailabilityZones[azIdx]
		subnetInfo, err := ctx.allocateSubnet(subnetConfig, az)
		if err != nil {
			return err
		}
		info.NewSubnets = append(info.NewSubnets, subnetInfo)
	}

	for azIdx := 0; azIdx < cfg.NumPublicSubnets; azIdx++ {
		subnetConfig := &SubnetConfig{
			SubnetType:      string(database.SubnetTypePublic),
			SubnetSize:      cfg.PublicSize,
			GroupName:       groupNamePublic,
			ParentContainer: publicContainerPath,
		}
		az := cfg.AvailabilityZones[azIdx]
		subnetInfo, err := ctx.allocateSubnet(subnetConfig, az)
		if err != nil {
			return err
		}
		info.NewSubnets = append(info.NewSubnets, subnetInfo)

		if cfg.AddFirewall {
			// one firewall subnet for each public subnet
			subnetName := fmt.Sprintf("firewall-%c", az[len(az)-1])
			err := ctx.IPAM.AddContainer(firewallContainerPath, subnetName, client.BlockTypeSubnet, cfg.AccountID)
			if err != nil {
				return fmt.Errorf("Error creating subnet container: %s", err)
			}
			ctx.containersCreated = append(ctx.containersCreated, fmt.Sprintf("%s/%s", firewallContainerPath, subnetName))

			subnetContainerPath := fmt.Sprintf("%s/%s", firewallContainerPath, subnetName)
			block, err := ctx.IPAM.AllocateBlock(firewallContainerPath, subnetContainerPath, client.BlockTypeSubnet, FirewallSubnetSize, "Deployed")
			if err != nil {
				return fmt.Errorf("Error creating subnet block: %s", err)
			}
			ctx.blocksCreated = append(ctx.blocksCreated, &Block{
				name:      block.BlockName,
				container: block.Container,
			})
			ctx.Logger.Log("%s CIDR: %s", subnetName, block.BlockName)
			info.NewSubnets = append(info.NewSubnets, &SubnetInfo{
				AvailabilityZone: az,
				Type:             database.SubnetTypeFirewall,
				Name:             fmt.Sprintf("%s-%s", cfg.VPCName, subnetName),
				ContainerPath:    subnetContainerPath,
				CIDR:             block.BlockName,
			})
		}
	}

	return nil
}

func (ctx *Context) AddReferencesToContainers() error {
	if ctx.LockSet == nil || !ctx.LockSet.HasLock(database.TargetIPControlWrite) {
		return fmt.Errorf("We don't have the IPControl lock.  If you see this error in production please alert a developer!")
	}
	info := &ctx.VPCInfo
	if ctx.newVPCContainerPath != "" {
		err := ctx.IPAM.UpdateContainerCloudID(ctx.newVPCContainerPath, info.ResourceID)
		if err != nil {
			return err
		}
	}
	sort.SliceStable(info.NewSubnets, func(i, j int) bool {
		return info.NewSubnets[i].ResourceID > info.NewSubnets[j].ResourceID
	})
	for _, subnet := range info.NewSubnets {
		err := ctx.IPAM.UpdateContainerCloudID(subnet.ContainerPath, subnet.ResourceID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ctx *Context) DeleteIncompleteResources() {
	ctx.Logger.Log("Deleting previously created containers (%d) and blocks (%d)...", len(ctx.containersCreated), len(ctx.blocksCreated))

	for idx := len(ctx.blocksCreated) - 1; idx >= 0; idx-- {
		block := ctx.blocksCreated[idx]
		err := ctx.IPAM.DeleteBlock(block.name, block.container, ctx.Logger)
		if err != nil {
			ctx.Logger.Log("Error deleting block %s: %s", block.name, err)
		}
	}

	if len(ctx.containersCreated) > 0 {
		for idx := len(ctx.containersCreated) - 1; idx >= 0; idx-- {
			container := ctx.containersCreated[idx]
			err := ctx.IPAM.DeleteContainer(container, ctx.Logger)
			if err != nil {
				ctx.Logger.Log("Error deleting container %q: %s", container, err)
			}
		}
	}

}
