package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"encoding/json"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testhelpers"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"

	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
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

func ContainsSubnetId(target string, subnets []*ec2.Subnet) bool {
	for _, item := range subnets {
		if target == *item.SubnetId {
			return true
		}
	}
	return false
}

func addRetryHandler(sess *session.Session) *session.Session {
	if sess.Config == nil {
		sess.Config = aws.NewConfig()
	}
	sess.Config.MaxRetries = aws.Int(maxRetries)
	sess.Handlers.Retry.PushBack(handleRetry)
	return sess
}

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

func stringInSlice(s string, a []string) bool {
	for _, t := range a {
		if s == t {
			//fmt.Println("comp: " + s + " " + t + " ture")
			//fmt.Println(" true")
			return true
		}
	}
	//fmt.Println(" false")
	return false
}

// Intermedate containers are between the root (common parrent containers to all VPC containers) and containers returned as directly part of a VPC
// for example:
// Getting VPC containers returns:
// Global/AWS/V4/Commercial/East/Development and Test/346570397073-alc-test3-east-test
// Global/AWS/V4/Commercial/East/Lower-Data/346570397073-alc-test3-east-test
// and all sub containers
// Global/AWS/V4/Commercial/East would be the root container (computed by getting the largest prefix that starts all VPC Containers)
// The following would be the intermediate containers and need to be computed
// Global/AWS/V4/Commercial/East/Development and Test
// Global/AWS/V4/Commercial/East/Lower-Data

func findIntermediateContainers(root string, VPCContainers []string) []string {
	//fmt.Printf("Inputs: %+v\n", VPCContainers)
	min := len(root)
	intermediates := []string{}
	for _, container := range VPCContainers {
		containerSplit := strings.Split(container, "/")
		candidate := "Global"
		for i := 1; i < len(containerSplit)-1; i++ {
			candidate = candidate + "/" + containerSplit[i]
			if len(candidate) <= min {
				continue
			}
			if !stringInSlice(candidate, intermediates) && !stringInSlice(candidate, VPCContainers) {
				intermediates = append(intermediates, candidate)
			}
		}
	}
	return intermediates
}

func findRoot(in []string) string {
	containerSplit := strings.Split(in[0], "/")
	root := "Global/AWS/V4"
	for i := 3; i <= 4; i++ {
		root = root + "/" + containerSplit[i]
	}
	return root
}

func buildTree(containersByName map[string]*models.WSContainer, blocksByContainer map[string][]*models.WSChildBlock, root string) *testmocks.ContainerTree {
	out := &testmocks.ContainerTree{}
	if rootContainer, ok := containersByName[root]; ok {
		out.Name = "/" + root
		out.ResourceID = rootContainer.CloudObjectID
		if blocks, ok := blocksByContainer[root]; ok {
			for _, block := range blocks {
				sizeInt, err := strconv.ParseInt(block.BlockSize, 10, 64)
				if err != nil {
					log.Fatal(err)
				}
				out.Blocks = append(out.Blocks, testmocks.BlockSpec{
					Address:   block.BlockAddr,
					Size:      int(sizeInt),
					BlockType: client.BlockType(block.BlockType),
					Status:    block.BlockStatus,
					Container: block.Container,
				})
			}
		}
	} else {
		out.Name = "Undefined"
	}
	for name, container := range containersByName {
		if container.ParentName != root {
			continue
		}
		child := buildTree(containersByName, blocksByContainer, name)
		out.Children = append(out.Children, *child)
	}
	return out
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter VPC ID: ")
	vpcID, _ := reader.ReadString('\n')
	vpcID = vpcID[:len(vpcID)-1]

	reader = bufio.NewReader(os.Stdin)
	fmt.Print("Enter region (us-east-1): ")
	region, _ := reader.ReadString('\n')
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}

	reader = bufio.NewReader(os.Stdin)
	fmt.Print("Enter Account ID (VPC Dev): ")
	accountID, _ := reader.ReadString('\n')
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		accountID = "346570397073"
	}

	reader = bufio.NewReader(os.Stdin)
	fmt.Print("Enter Test Name: ")
	testCaseName, _ := reader.ReadString('\n')
	testCaseName = testCaseName[:len(testCaseName)-1]

	reader = bufio.NewReader(os.Stdin)
	fmt.Print("Select Task Type:\n\t1. performAddAvailabilityZoneTask\n\t2. performUpdateNetworkingTask\n")
	taskTypeSelection, _ := reader.ReadString('\n')
	taskTypeSelection = taskTypeSelection[:len(taskTypeSelection)-1]

	taskType := ""
	switch taskTypeSelection {
	case "1":
		taskType = "performAddAvailabilityZoneTask"
	case "2":
		taskType = "performUpdateNetworkingTask"
	default:
		log.Fatal("Invalid Task Selection")
	}

	credentialsConfig, err := credentialservice.GetConfigFromENV()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading credentials configuration: %s", err)
		os.Exit(2)
	}

	credentialService := credentialservice.CredentialService{Config: credentialsConfig}
	sess, err := credentialService.GetAWSSession(accountID, string(region), "foo")
	if err != nil {
		fmt.Printf("Error getting AWS credentials %s %s Error: %s\n", accountID, region, err)
		return
	}
	awsAccountAccess := &awsp.AWSAccountAccess{
		Session: addRetryHandler(sess),
	}

	subnetsOutput, err := awsAccountAccess.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)
	if err != nil {
		fmt.Println("Error getting subnets from AWS")
	}

	natGatewaysOutput, err := awsAccountAccess.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{Filter: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)

	if err != nil {
		log.Fatal(err)
	}

	startNatGateways := []string{}
	for _, gateway := range natGatewaysOutput.NatGateways {
		startNatGateways = append(startNatGateways, *gateway.NatGatewayId)
	}

	routeTablesOutput, err := awsAccountAccess.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{Filters: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)

	if err != nil {
		log.Fatal(err)
	}

	startRouteTables := []string{}
	startRouteTableAssociations := []string{}
	for _, routeTable := range routeTablesOutput.RouteTables {
		startRouteTables = append(startRouteTables, *routeTable.RouteTableId)
		for _, association := range routeTable.Associations {
			if association.RouteTableAssociationId != nil {
				startRouteTableAssociations = append(startRouteTableAssociations, *association.RouteTableAssociationId)
			}
		}
	}

	ExistingRouteTablesStr := ""
	for _, rt := range startRouteTables {
		ExistingRouteTablesStr = ExistingRouteTablesStr + fmt.Sprintf(`	{
				RouteTableId: aws.String("%[1]s"),
				VpcId:        aws.String("%[2]s"),
			},
		`, rt, vpcID)
	}

	eIPOutput, err := awsAccountAccess.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{})

	if err != nil {
		log.Fatal(err)
	}

	startEIPs := []string{}
	for _, eIP := range eIPOutput.Addresses {
		startEIPs = append(startEIPs, *eIP.AllocationId)
	}

	vpcsOutput, err := awsAccountAccess.EC2().DescribeVpcs(&ec2.DescribeVpcsInput{Filters: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)
	if err != nil {
		fmt.Println("error getting VPC from AWS")
	}

	VPCName := ""
	PrimaryCIDR := ""
	var ExistingVPCCIDRBlocksList []*ec2.VpcCidrBlockAssociation
	for _, vpc := range vpcsOutput.Vpcs {
		PrimaryCIDR = *vpc.CidrBlock
		ExistingVPCCIDRBlocksList = vpc.CidrBlockAssociationSet
		for _, tag := range vpc.Tags {
			if *tag.Key == "Name" {
				VPCName = *tag.Value
				break
			}
		}
	}

	ExistingVPCCIDRBlocksStr := ""
	for _, block := range ExistingVPCCIDRBlocksList {
		ExistingVPCCIDRBlocksStr = ExistingVPCCIDRBlocksStr + fmt.Sprintf(`		{
			AssociationId: aws.String("%[1]s"),
			CidrBlock:     aws.String("%[2]s"),
			CidrBlockState: &ec2.VpcCidrBlockState{
				State: aws.String("%[3]s"),
			},
		},
`, *block.AssociationId, *block.CidrBlock, *block.CidrBlockState.State)
	}

	taskDataType := ""
	switch taskType {
	case "performAddAvailabilityZoneTask":
		taskDataType = "\tdatabase.AddAvailabilityZoneTaskData"
	case "performUpdateNetworkingTask":
		taskDataType = "\tdatabase.UpdateNetworkingTaskData"
	default:
		log.Fatal("Not Valid task type")
	}

	header := `
package main

import (
	"encoding/json"
	"fmt"
	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testhelpers"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"testing"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type %[1]s struct {
	TestCaseName string

	VPCName string
	Stack   string
	VPCID   string
	Region  database.Region

	StartState                       database.VPCState
	ExistingContainers               testmocks.ContainerTree
	ExistingPeeringConnections       []*ec2.VpcPeeringConnection
	ExistingSubnetCIDRs              map[string]string
	ExistingVPCCIDRBlocks            []*ec2.VpcCidrBlockAssociation // only used when there are unroutable subnets
	ExistingRouteTables              []*ec2.RouteTable
	ExistingFirewalls                map[string]string // firewall id -> vpc id
	ExistingFirewallSubnetToEndpoint map[string]string // subnet id -> endpoint id
	ExistingFirewallPolicies         []*networkfirewall.FirewallPolicyMetadata

	TaskConfig %[2]s

	ExpectedTaskStatus database.TaskStatus
	ExpectedEndState   database.VPCState
}


func TestPerform%[1]s(t *testing.T) {
`
	fmt.Printf(header, testCaseName, taskDataType)

	fmt.Printf("\tExistingSubnetCIDRs := map[string]string{\n")
	for _, subnet := range subnetsOutput.Subnets {
		fmt.Printf("\t\t\"%s\": \"%s\",\n", *subnet.SubnetId, *subnet.CidrBlock)
	}
	fmt.Print("\t}\n")

	postgresConnectionString := os.Getenv("POSTGRES_CONNECTION_STRING")
	db := sqlx.MustConnect("postgres", postgresConnectionString)
	err = database.Migrate(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error migrating database: %s\n", err)
		os.Exit(1)
	}

	username := os.Getenv("IPCONTROL_USERNAME")
	password := os.Getenv("IPCONTROL_PASSWORD")
	ipcHost := os.Getenv("IPCONTROL_HOST")
	if username == "" || password == "" || ipcHost == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "IPCONTROL_HOST, IPCONTROL_USERNAME and IPCONTROL_PASSWORD env variables are required")
		os.Exit(2)
	}

	c := client.GetClient(ipcHost, username, password, 60*time.Second)
	mm := &database.SQLModelsManager{
		DB: db,
	}

	vpc, err := mm.GetVPC(database.Region(region), vpcID)
	if err != nil {
		log.Fatal(err)
	}
	ret, err := json.MarshalIndent(vpc.State, "", " ")
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("\n\nstartStateJson := `")
		fmt.Println(string(ret))
		fmt.Printf("`\n\n")
	}

	containers, err := c.ListContainersForVPC(accountID, vpcID)
	if err != nil {
		log.Fatal(err)
	}

	containersByName := make(map[string]*models.WSContainer)
	containerNames := make([]string, 0)

	// First find the top level container path
	for _, container := range containers {
		fullPath := container.ParentName + "/" + container.ContainerName
		if !stringInSlice(fullPath, containerNames) {
			containerNames = append(containerNames, fullPath)
		}
		//	if !stringInSlice(container.ParentName, containerNames) {
		//		containerNames = append(containerNames, container.ParentName)
		//	}
		containersByName[fullPath] = container
		//fmt.Println("Container name: " + fullPath)
	}

	//fmt.Printf("containerNames: %+v\n", containerNames)
	//topLevelContainerName := findLongestPrefix(containerNames)
	topLevelContainerName := findRoot(containerNames)
	containers, err = c.GetContainersByName("/" + topLevelContainerName)
	if err != nil {
		log.Fatal(err)
	}
	if len(containers) < 1 {
		log.Fatalf("No containers found for top level container: %s", topLevelContainerName)
	}
	containersByName[topLevelContainerName] = containers[0]

	intermediates := findIntermediateContainers(topLevelContainerName, containerNames)
	for _, inter := range intermediates {
		containers, err = c.GetContainersByName("/" + inter)
		if err != nil {
			log.Fatal(err)
		}
		if len(containers) < 1 {
			log.Fatalf("No containers found for intermediate container: %s", inter)
		}
		containersByName[inter] = containers[0]
	}

	blocksByContainer := make(map[string][]*models.WSChildBlock)
	for container := range containersByName {
		freeblocks, err := c.ListBlocks("/"+container, true, false)
		if err != nil {
			log.Print(err)
			continue
		}
		notfreeblocks, err := c.ListBlocks("/"+container, false, false)
		if err != nil {
			log.Print(err)
			continue
		}
		blocks := append(freeblocks, notfreeblocks...)
		blocksByContainer[container] = blocks
	}
	//fmt.Println("Top Level Container: " + topLevelContainerName)

	finalTree := buildTree(containersByName, blocksByContainer, topLevelContainerName)
	ret, err = json.MarshalIndent(finalTree, "", " ")
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("\nexistingContainersJson := `")
		fmt.Println(string(ret))
		fmt.Println("`")
	}

	fmt.Println("TODO Write to files directly for now just delete this, first state collected, make the next change and press enter")
	fmt.Scanln() // wait for Enter Key

	vpc, err = mm.GetVPC(database.Region(region), vpcID)
	if err != nil {
		log.Fatal(err)
	}
	ret, err = json.MarshalIndent(vpc.State, "", " ")
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("\n\nendStateJson := `")
		fmt.Println(string(ret))
		fmt.Printf("`\n\n")
	}

	subnetsPostTaskOutput, err := awsAccountAccess.EC2().DescribeSubnets(&ec2.DescribeSubnetsInput{Filters: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)
	if err != nil {
		fmt.Println("Error getting subnets from AWS")
	}

	natGatewaysOutput, err = awsAccountAccess.EC2().DescribeNatGateways(&ec2.DescribeNatGatewaysInput{Filter: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)

	if err != nil {
		log.Fatal(err)
	}

	endNatGateways := []string{}
	for _, gateway := range natGatewaysOutput.NatGateways {
		endNatGateways = append(endNatGateways, *gateway.NatGatewayId)
	}

	routeTablesOutput, err = awsAccountAccess.EC2().DescribeRouteTables(&ec2.DescribeRouteTablesInput{Filters: []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
	},
	},
	)

	if err != nil {
		log.Fatal(err)
	}

	endRouteTables := []string{}
	endRouteTableAssociations := []string{}
	for _, routeTable := range routeTablesOutput.RouteTables {
		endRouteTables = append(endRouteTables, *routeTable.RouteTableId)
		for _, association := range routeTable.Associations {
			if association.RouteTableAssociationId != nil {
				endRouteTableAssociations = append(endRouteTableAssociations, *association.RouteTableAssociationId)
			}
		}
	}

	eIPOutput, err = awsAccountAccess.EC2().DescribeAddresses(&ec2.DescribeAddressesInput{})
	if err != nil {
		fmt.Println("Error getting addresses from AWS")
	}

	endEIPs := []string{}
	for _, eIP := range eIPOutput.Addresses {
		endEIPs = append(endEIPs, *eIP.AllocationId)
	}

	containers, err = c.ListContainersForVPC(accountID, vpcID)
	if err != nil {
		log.Fatal(err)
	}

	containersByName = make(map[string]*models.WSContainer)
	containerNames = make([]string, 0)

	// First find the top level container path
	for _, container := range containers {
		fullPath := container.ParentName + "/" + container.ContainerName
		if !stringInSlice(fullPath, containerNames) {
			containerNames = append(containerNames, fullPath)
		}
		containersByName[fullPath] = container
	}

	topLevelContainerName = findRoot(containerNames)
	containers, err = c.GetContainersByName("/" + topLevelContainerName)
	if err != nil {
		log.Fatal(err)
	}
	if len(containers) < 1 {
		log.Fatalf("No containers found for top level container: %s", topLevelContainerName)
	}
	containersByName[topLevelContainerName] = containers[0]

	intermediates = findIntermediateContainers(topLevelContainerName, containerNames)
	for _, inter := range intermediates {
		containers, err = c.GetContainersByName("/" + inter)
		if err != nil {
			log.Fatal(err)
		}
		if len(containers) < 1 {
			log.Fatalf("No containers found for intermediate container: %s", inter)
		}
		containersByName[inter] = containers[0]
	}

	blocksByContainer = make(map[string][]*models.WSChildBlock)
	for container := range containersByName {
		freeblocks, err := c.ListBlocks("/"+container, true, false)
		if err != nil {
			log.Print(err)
			continue
		}
		notfreeblocks, err := c.ListBlocks("/"+container, false, false)
		if err != nil {
			log.Print(err)
			continue
		}
		blocks := append(freeblocks, notfreeblocks...)
		blocksByContainer[container] = blocks
	}

	finalTree = buildTree(containersByName, blocksByContainer, topLevelContainerName)
	ret, err = json.MarshalIndent(finalTree, "", " ")
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("\nendContainersJson := `")
		fmt.Println(string(ret))
		fmt.Println("`")
		fmt.Printf(`
	endContainers := testmocks.ContainerTree{}
	err := json.Unmarshal([]byte(endContainersJson), &endContainers)
	if err != nil {
		fmt.Println(err)
	}
`)
	}

	printMap := map[string][]string{}
	for _, subnet := range subnetsPostTaskOutput.Subnets {
		if !ContainsSubnetId(*subnet.SubnetId, subnetsOutput.Subnets) {
			printMap[*subnet.AvailabilityZone] = append(printMap[*subnet.AvailabilityZone], *subnet.SubnetId)
		}
	}

	fmt.Println("CORRECT ORDER AND REMOVE THIS LINE")
	fmt.Printf("\tpreDefinedSubnetIDQueue := map[string][]string{\n")
	for az, subnets := range printMap {
		fmt.Printf("\t\t\"%s\": []string{", az)
		for _, subnet := range subnets {
			fmt.Printf("\"%s\", ", subnet)
		}
		fmt.Print("},\n")
	}
	fmt.Print("\t}\n\n")

	fmt.Println("CORRECT ORDER AND REMOVE THIS LINE")
	fmt.Printf("\tpreDefinedNatGatewayIDQueue := []string{\n")
	for _, gateway := range testhelpers.NewItems(startNatGateways, endNatGateways) {
		fmt.Printf("\"%s\", ", gateway)
	}
	fmt.Print("\t}\n")

	fmt.Println("CORRECT ORDER AND REMOVE THIS LINE")
	fmt.Printf("\tpreDefinedRouteTableIDQueue := []string{\n")
	for _, routeTable := range testhelpers.NewItems(startRouteTables, endRouteTables) {
		fmt.Printf("\"%s\", ", routeTable)
	}
	fmt.Print("\t}\n")

	fmt.Println("CORRECT ORDER AND REMOVE THIS LINE")
	fmt.Printf("\tpreDefinedRouteTableAssociationIDQueue := []string{\n")
	for _, routeTableAssociation := range testhelpers.NewItems(startRouteTableAssociations, endRouteTableAssociations) {
		fmt.Printf("\"%s\", ", routeTableAssociation)
	}
	fmt.Print("\t}\n")

	fmt.Println("CORRECT ORDER AND REMOVE THIS LINE")
	fmt.Printf("\tpreDefinedEIPQueue := []string{\n")
	for _, eIPAssociation := range testhelpers.NewItems(startEIPs, endEIPs) {
		fmt.Printf("\"%s\", ", eIPAssociation)
	}
	fmt.Print("\t}\n")

	taskConfigStr := ""
	switch taskType {
	case "performAddAvailabilityZoneTask":
		taskConfigStr = fmt.Sprintf(`
		TaskConfig := database.AddAvailabilityZoneTaskData {
			VPCID: "%[1]s",
			Region: "%[2]s",
			AZName: "us-east-1f",
		}
		`, vpcID, region)
	case "performUpdateNetworkingTask":
		taskConfigStr = fmt.Sprintf(`
		TaskConfig := database.UpdateNetworkingTaskData{
			VPCID:     "%[1]s",
			AWSRegion: "%[2]s",
			NetworkingConfig: database.NetworkingConfig{
				ConnectPublic:  true,
				ConnectPrivate: true,
			},
		}
		`, vpcID, region)
	default:
		log.Fatal("Not Valid task type")
	}

	fmt.Printf(`

	startState := database.VPCState{}
	err = json.Unmarshal([]byte(startStateJson), &startState)
	if err != nil {
		fmt.Println(err)
	}

	endState := database.VPCState{}
	err = json.Unmarshal([]byte(endStateJson), &endState)
	if err != nil {
		fmt.Println(err)
	}

	existingContainers := testmocks.ContainerTree{}
	err = json.Unmarshal([]byte(existingContainersJson), &existingContainers)
	if err != nil {
		fmt.Println(err)
	}

	%[9]s

	ExistingVPCCIDRBlocks := []*ec2.VpcCidrBlockAssociation{
	%[7]s
	}

	ExistingRouteTables := []*ec2.RouteTable{
%[10]s
	}

	ExistingPeeringConnections := []*ec2.VpcPeeringConnection{}

	tc := %[1]s{
		VPCName: "%[5]s",
		VPCID: "%[4]s",
		Region: "%[3]s",
		Stack: "%[8]s",
		TaskConfig: TaskConfig,
		ExistingSubnetCIDRs: ExistingSubnetCIDRs,
		ExistingVPCCIDRBlocks: ExistingVPCCIDRBlocks,
		StartState: startState,
		ExistingContainers: existingContainers,
		ExistingRouteTables: ExistingRouteTables,
		ExistingPeeringConnections: ExistingPeeringConnections,
		ExpectedTaskStatus:         database.TaskStatusSuccessful,
	}

	taskId := uint64(1235)
	vpcKey := string(tc.Region) + tc.VPCID
	mm := &testmocks.MockModelsManager{
		VPCs: map[string]*database.VPC{
			vpcKey: {
				AccountID: "%[2]s",
				ID:        tc.VPCID,
				State:     &tc.StartState,
				Name:      tc.VPCName,
				Stack:     tc.Stack,
				Region:    tc.Region,
			},
		},
	}

	ipcontrol := &testmocks.MockIPControl{
		ExistingContainers: tc.ExistingContainers,
		BlocksDeleted:      []string{},
	}

	ec2 := &testmocks.MockEC2{
		PeeringConnections:                     &tc.ExistingPeeringConnections,
		PrimaryCIDR:                            aws.String("%[6]s"),
		CIDRBlockAssociationSet:                tc.ExistingVPCCIDRBlocks,
		RouteTables:                            tc.ExistingRouteTables,
		SubnetCIDRs:                            tc.ExistingSubnetCIDRs,
		PreDefinedSubnetIDQueue:                preDefinedSubnetIDQueue,
		PreDefinedNatGatewayIDQueue:            preDefinedNatGatewayIDQueue,
		PreDefinedRouteTableIDQueue:            preDefinedRouteTableIDQueue,
		PreDefinedRouteTableAssociationIDQueue: preDefinedRouteTableAssociationIDQueue,
		PreDefinedEIPQueue:                     preDefinedEIPQueue,
	}
	task := &testmocks.MockTask{
		ID: taskId,
	}
	taskContext := &TaskContext{
		Task:          task,
		ModelsManager: mm,
		LockSet:       database.GetFakeLockSet(database.TargetVPC(tc.VPCID), database.TargetIPControlWrite),
		IPAM:          ipcontrol,
		BaseAWSAccountAccess: &awsp.AWSAccountAccess{
			EC2svc: ec2,
		},
		CMSNet: &testmocks.MockCMSNet{},
	}

`, testCaseName, accountID, region, vpcID, VPCName, PrimaryCIDR, ExistingVPCCIDRBlocksStr, vpc.Stack, taskConfigStr, ExistingRouteTablesStr)

	switch taskType {
	case "performAddAvailabilityZoneTask":
		fmt.Println("\ttaskContext.performAddAvailabilityZoneTask(&tc.TaskConfig)")
	case "performUpdateNetworkingTask":
		fmt.Println("\ttaskContext.performUpdateNetworkingTask(&tc.TaskConfig)")
	default:
		log.Fatal("Not Valid task type")
	}

	fmt.Printf(`
	if task.Status != tc.ExpectedTaskStatus {
		t.Fatalf("Incorrect task status. Expected %%s but got %%s", tc.ExpectedTaskStatus, task.Status)
	}

	testhelpers.SortIpcontrolContainersAndBlocks(&endContainers)
	testhelpers.SortIpcontrolContainersAndBlocks(&ipcontrol.ExistingContainers)
	if diff := cmp.Diff(endContainers, ipcontrol.ExistingContainers, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end containers did not match mock containers: \n%%s\n\nSide By Side Diff:\n%%s", diff, testhelpers.ObjectGoPrintSideBySide(endContainers, ipcontrol.ExistingContainers))
	}

	testStateJson, _ := json.Marshal(*mm.VPCs[vpcKey].State)
	testState := *&database.VPCState{}
	err = json.Unmarshal([]byte(testStateJson), &testState)
	if err != nil {
		fmt.Println(err)
	}

	// Saved state
	if diff := cmp.Diff(endState, testState, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("Expected end state did not match state saved to database: \n%%s\n\nSide By Side Diff:\n%%s", diff, testhelpers.ObjectGoPrintSideBySide(endState, testState))
	}
}
`)
}
