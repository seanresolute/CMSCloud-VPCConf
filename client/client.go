package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	ipc_client "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/client/block"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/client/child_block"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/client/container"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/client/login"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
)

const containerInformationTemplateName = "Container Data"
const awsAccountFieldName = "Account"
const jiraTicketFieldName = "JiraTicket"
const jiraTicketFieldValue = "none"

type BlockType string

const (
	BlockTypeGlobal BlockType = "Global CIDR Block"
	BlockTypeEnv    BlockType = "Environment CIDR Block"
	BlockTypeVPC    BlockType = "VPC CIDR Block"
	BlockTypeSubnet BlockType = "Subnet CIDR Block"
)

var ErrorNoBlockAvailable = errors.New("No free block available")

func getParentTypes(ct BlockType) []string {
	return map[BlockType][]string{
		BlockTypeEnv:    {string(BlockTypeGlobal)},
		BlockTypeVPC:    {string(BlockTypeEnv)},
		BlockTypeSubnet: {string(BlockTypeVPC)},
	}[ct]
}

func newString(s string) *string {
	return &s
}

type token struct {
	token     string
	expiresAt time.Time
}

type Client interface {
	ListContainersForVPC(accountID, vpcID string) ([]*models.WSContainer, error)
	ListContainersForAccount(accountID string) ([]*models.WSContainer, error)
	GetContainersByName(name string) ([]*models.WSContainer, error)
	AddContainer(parentName, name string, bt BlockType, awsAccountID string) error
	ContainerHasAvailableSpace(parentContainer string, size int) (bool, error)
	AllocateBlock(parentContainer, container string, bt BlockType, size int, status string) (*models.WSChildBlock, error)
	UpdateContainerCloudID(containerName, cloudID string) error
	DeleteContainersAndBlocksForVPC(accountID, vpcID string, logger Logger) error
	DeleteContainersAndBlocks(containers []string, logger Logger) error
	DeleteContainerAndBlocks(containerPath string, logger Logger) error
	DeleteContainer(containerPath string, logger Logger) error
	DeleteBlock(name, container string, logger Logger) error
	ListBlocks(container string, onlyFree bool, recursive bool) ([]*models.WSChildBlock, error)
	GetSubnetContainer(subnetID string) (*models.WSContainer, error)
	GetIPUsage() (*database.IPUsage, error)
}

type RESTClient struct {
	*token
	apiClient  *ipc_client.IPControlREST
	httpClient *http.Client
	opTimeout  time.Duration
	username   string
	password   string

	// All public methods should acquire this before making any
	// requests to IPControl, and keep it until any Init/Export
	// sequence is complete, because the Init/Export calls appear
	// to fail sometimes if they are interleaved (even though the
	// Init calls return a context object which is reused).
	mu *sync.Mutex
}

func (c *RESTClient) AuthenticateRequest(req runtime.ClientRequest, reg strfmt.Registry) error {
	if c.token == nil || c.token.expiresAt.Before(time.Now().Add(60*time.Second)) {
		err := c.refreshToken()
		if err != nil {
			return err
		}
	}
	return req.SetHeaderParam("Authorization", "Bearer "+c.token.token)
}

type Logger interface {
	Log(string, ...interface{})
}

func (c *RESTClient) listContainers(query string) ([]*models.WSContainer, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	body := container.InitExportContainerBody{
		Options: []string{"ParentContainerFullPath"},
		Query:   query,
	}
	iParams := &container.InitExportContainerParams{
		Context:    ctx,
		HTTPClient: c.httpClient,

		ExportParameters: body,
	}
	iOK, err := c.apiClient.Container.InitExportContainer(iParams, c)
	if err != nil {
		return nil, err
	}

	defer func() {
		endParams := &container.EndExportContainerParams{
			Context:    ctx,
			HTTPClient: c.httpClient,
			Wscontext: container.EndExportContainerBody{
				Context: iOK.Payload,
			},
		}
		c.apiClient.Container.EndExportContainer(endParams, c)
	}()

	results := make([]*models.WSContainer, 0)
	eParams := &container.ExportContainerParams{
		Context:    ctx,
		HTTPClient: c.httpClient,
		Wscontext: container.ExportContainerBody{
			Context: iOK.Payload,
		},
	}
	for {
		eOK, err := c.apiClient.Container.ExportContainer(eParams, c)
		if err != nil {
			return nil, err
		}
		results = append(results, eOK.Payload...)
		if int64(len(eOK.Payload)) < iOK.Payload.MaxResults {
			break
		} else {
			iOK.Payload.ResultCount += iOK.Payload.MaxResults
		}
	}
	return results, nil
}

func (c *RESTClient) GetContainersByName(name string) ([]*models.WSContainer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listContainers(fmt.Sprintf("Name='%s'", escapeForQuery(name)))
}

func (c *RESTClient) ListContainersForAccount(accountID string) ([]*models.WSContainer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listContainers(fmt.Sprintf("UDF:%s='%s'", awsAccountFieldName, accountID))
}

func (c *RESTClient) GetSubnetContainer(subnetID string) (*models.WSContainer, error) {
	containers, err := c.listContainers(fmt.Sprintf("cloudObjectId='%s'", subnetID))
	if err != nil {
		return nil, err
	}
	if len(containers) != 1 {
		return nil, fmt.Errorf("Got %d containers for subnet %q", len(containers), subnetID)
	}
	return containers[0], err
}

func (c *RESTClient) ListContainersForVPC(accountID, vpcID string) ([]*models.WSContainer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	containers, err := c.listContainers(fmt.Sprintf("cloudObjectId='%s'", vpcID))
	if err != nil {
		return nil, err
	}
	if len(containers) < 1 {
		return nil, fmt.Errorf("Got %d containers for VPC %q", len(containers), vpcID)
	}

	accountContainers, err := c.listContainers(fmt.Sprintf("UDF:%s='%s'", awsAccountFieldName, accountID))
	if err != nil {
		return nil, err
	}

	vpcContainers := []*models.WSContainer{}
	for _, vpcContainer := range containers {
		vpcContainers = append(vpcContainers, vpcContainer)
		vpcContainerFullName := fmt.Sprintf("%s/%s", vpcContainer.ParentName, vpcContainer.ContainerName)
		for _, container := range accountContainers {
			if container.ParentName == vpcContainerFullName || strings.HasPrefix(container.ParentName, vpcContainerFullName+"/") {
				vpcContainers = append(vpcContainers, container)
			}
		}
	}
	return vpcContainers, nil
}

func escapeForQuery(s string) string {
	// TODO: check for boolean operators? apparently their query parser does not
	// understand them, even inside quotes
	return strings.Replace(s, "'", "''", -1)
}

func ipToInt(ip string) uint32 {
	pieces := strings.Split(ip, ".")
	if len(pieces) != 4 {
		return 0
	}
	n := uint32(0)
	for _, piece := range pieces {
		n <<= 8
		i, err := strconv.Atoi(piece)
		if err != nil || i < 0 || i >= 256 {
			return 0
		}
		n += uint32(i)
	}
	return n
}

type Blocks []*models.WSChildBlock

func (blocks Blocks) Len() int {
	return len(blocks)
}
func (blocks Blocks) Less(i, j int) bool {
	addrI := ipToInt(blocks[i].BlockAddr)
	addrJ := ipToInt(blocks[j].BlockAddr)
	if addrI == addrJ {
		return blocks[i].BlockSize < blocks[j].BlockSize
	}
	return addrI < addrJ
}
func (blocks Blocks) Swap(i, j int) {
	b := blocks[i]
	blocks[i] = blocks[j]
	blocks[j] = b
}

func (c *RESTClient) ListBlocks(container string, onlyFree bool, recursive bool) ([]*models.WSChildBlock, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listBlocks(container, onlyFree, recursive)
}

func (c *RESTClient) listBlocks(container string, onlyFree bool, recursive bool) ([]*models.WSChildBlock, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()

	body := child_block.InitExportChildBlockBody{
		IncludeFreeBlocks: onlyFree,
	}
	clauses := make([]string, 0)
	if container != "" {
		clauses = append(clauses, fmt.Sprintf("container='%s'", escapeForQuery(container)))
	}
	if onlyFree {
		clauses = append(clauses, "status='free'")
	}
	if recursive {
		clauses = append(clauses, "recursive='container'")
	}
	if len(clauses) > 0 {
		body.Query = strings.Join(clauses, " and ")
	}
	iParams := &child_block.InitExportChildBlockParams{
		ExportParameters: body,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	iOK, err := c.apiClient.ChildBlock.InitExportChildBlock(iParams, c)
	if err != nil {
		return nil, fmt.Errorf("Error calling InitExportChildBlock: %s", err)
	}
	defer func() {
		endParams := &child_block.EndExportChildBlockParams{
			Context:    ctx,
			HTTPClient: c.httpClient,
			Wscontext: child_block.EndExportChildBlockBody{
				Context: iOK.Payload,
			},
		}
		c.apiClient.ChildBlock.EndExportChildBlock(endParams, c)
	}()

	eParams := &child_block.ExportChildBlockParams{
		Context:    ctx,
		HTTPClient: c.httpClient,
		Wscontext: child_block.ExportChildBlockBody{
			Context: iOK.Payload,
		},
	}
	blocks := make(Blocks, 0)
	for {
		eOK, err := c.apiClient.ChildBlock.ExportChildBlock(eParams, c)
		if err != nil {
			return nil, fmt.Errorf("Error calling ExportChildBlock: %s", err)
		}
		for _, cs := range eOK.Payload {
			blocks = append(blocks, cs.ChildBlock)
		}
		if int64(len(eOK.Payload)) < iOK.Payload.MaxResults {
			break
		} else {
			iOK.Payload.ResultCount += iOK.Payload.MaxResults
		}
	}
	sort.Sort(blocks)
	return blocks, nil
}

func chooseBlock(freeBlocks []*models.WSChildBlock, allocationSize int) (*models.WSChildBlock, error) {
	blockSizes := make([]int, len(freeBlocks))
	for idx, block := range freeBlocks {
		bs, err := strconv.Atoi(block.BlockSize)
		if err != nil || idx < 0 {
			return nil, fmt.Errorf("Invalid block size %s", block.BlockSize)
		}
		blockSizes[idx] = int(bs)
	}
	for parentSize := allocationSize; parentSize >= 0; parentSize-- {
		for idx, blockSize := range blockSizes {
			if blockSize == parentSize {
				return freeBlocks[idx], nil
			}
		}
	}
	return nil, ErrorNoBlockAvailable
}

func (c *RESTClient) findCandidateBlock(container string, size int) (*models.WSChildBlock, error) {
	if size > 32 || size < 0 {
		return nil, fmt.Errorf("Invalid requested size %d", size)
	}
	blocks, err := c.listBlocks(container, true, false)
	if err != nil {
		return nil, fmt.Errorf("Error listing blocks; %s", err)
	}
	return chooseBlock(blocks, size)
}

func (c *RESTClient) UpdateContainerCloudID(containerName, cloudID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	getParams := &container.GetContainerByNameParams{
		ContainerName: containerName,
		Context:       ctx,
		HTTPClient:    c.httpClient,
	}
	gOK, err := c.apiClient.Container.GetContainerByName(getParams, c)
	if err != nil {
		return fmt.Errorf("Error getting info on container %q: %s", containerName, err)
	}
	gOK.Payload.CloudObjectID = cloudID

	importBody := container.ImportContainerBody{
		InpContainer: gOK.Payload,
	}
	importParams := &container.ImportContainerParams{
		ImportParameters: importBody,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	_, err = c.apiClient.Container.ImportContainer(importParams, c)
	return err
}

func (c *RESTClient) DeleteBlock(name, container string, logger Logger) error {
	logger.Log("Deleting block %s", name)
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	body := block.DeleteBlockBody{BlockName: name, Container: container}
	params := &block.DeleteBlockParams{
		DeleteParameters: body,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	_, err := c.apiClient.Block.DeleteBlock(params, c)
	return err
}

func (c *RESTClient) DeleteContainersAndBlocksForVPC(accountID, vpcID string, logger Logger) error {
	allContainers, err := c.ListContainersForVPC(accountID, vpcID)
	if err != nil {
		return fmt.Errorf("Error listing containers for VPC %q: %s", vpcID, err)
	}
	containerNames := make([]string, len(allContainers))
	for idx, container := range allContainers {
		containerNames[idx] = fmt.Sprintf("/%s/%s", container.ParentName, container.ContainerName)
	}
	return c.DeleteContainersAndBlocks(containerNames, logger)
}

func (c *RESTClient) DeleteContainersAndBlocks(containerNames []string, logger Logger) error {
	sort.Strings(containerNames)
	for idx := len(containerNames) - 1; idx >= 0; idx-- {
		err := c.DeleteContainerAndBlocks(containerNames[idx], logger)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *RESTClient) DeleteContainerAndBlocks(containerPath string, logger Logger) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	blocks, err := c.listBlocks(containerPath, false, true)
	if err != nil {
		return err
	}
	for idx := len(blocks) - 1; idx >= 0; idx-- {
		block := blocks[idx]
		err := c.DeleteBlock(block.BlockName, block.Container, logger)
		if err != nil {
			return fmt.Errorf("Error deleting block %s: %s", block.BlockName, err)
		}
	}

	logger.Log("Deleting container %s", containerPath)
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	body := container.DeleteContainerBody{FullName: containerPath}
	params := &container.DeleteContainerParams{
		DeleteParameters: body,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	_, err = c.apiClient.Container.DeleteContainer(params, c)
	return err
}

func (c *RESTClient) DeleteContainer(containerPath string, logger Logger) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logger.Log("Deleting container %s", containerPath)
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	body := container.DeleteContainerBody{FullName: containerPath}
	params := &container.DeleteContainerParams{
		DeleteParameters: body,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	_, err := c.apiClient.Container.DeleteContainer(params, c)
	return err
}

func (c *RESTClient) AddContainer(parentName, name string, bt BlockType, awsAccountID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	body := container.ImportContainerBody{
		InpContainer: &models.WSContainer{
			ContainerName:                    name,
			ParentName:                       parentName,
			ContainerType:                    "logical",
			AllowedBlockTypes:                []string{string(bt)},
			AllowedAllocFromParentBlocktypes: getParentTypes(bt),
			CloudType:                        "AWS",
			InformationTemplate:              []string{containerInformationTemplateName},
			UserDefinedFields: []string{
				fmt.Sprintf("%s=%s", awsAccountFieldName, awsAccountID),
				fmt.Sprintf("%s=%s", jiraTicketFieldName, jiraTicketFieldValue),
			},
		},
	}
	params := &container.ImportContainerParams{
		ImportParameters: body,
		Context:          ctx,
		HTTPClient:       c.httpClient,
	}
	_, err := c.apiClient.Container.ImportContainer(params, c)
	return err
}

func (c *RESTClient) ContainerHasAvailableSpace(container string, size int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.findCandidateBlock(container, size)
	if err != nil {
		// We expect ErrorNoBlockAvailable from findCandidateBlock(), and it's not a failure mode, so return an empty container and nil error
		if err == ErrorNoBlockAvailable {
			return false, nil
		}
		return false, fmt.Errorf("Error finding free candidate block: %s", err)
	}
	return true, nil
}

func (c *RESTClient) AllocateBlock(parentContainer, container string, bt BlockType, size int, status string) (*models.WSChildBlock, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if status != "Deployed" && status != "Aggregate" {
		return nil, fmt.Errorf("Invalid status %s. Must be Deployed or Aggregate", status)
	}
	parentBlock, err := c.findCandidateBlock(parentContainer, size)
	if err != nil {
		return nil, fmt.Errorf("Error finding candidate block: %s", err)
	}
	newBlock := &models.WSChildBlock{
		Container:   container,
		BlockSize:   fmt.Sprintf("%d", size),
		BlockAddr:   parentBlock.BlockAddr,
		BlockName:   fmt.Sprintf("%s/%d", parentBlock.BlockAddr, size),
		BlockStatus: status,
		BlockType:   string(bt),
		CloudType:   "AWS",
	}
	body := child_block.ImportChildBlockBody{
		InpChildBlock: newBlock,
	}
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()
	params := &child_block.ImportChildBlockParams{
		ImportParametersInpBlockPolicy: body,
		Context:                        ctx,
		HTTPClient:                     c.httpClient,
	}
	_, err = c.apiClient.ChildBlock.ImportChildBlock(params, c)
	if err != nil {
		return nil, err
	}
	return newBlock, nil
}

// No op authWritter to avoid stack overflow in c.apiClient.Login.AcceptItem attempting to refresh a token when it doesn't use tokens...
type noOpAuthWriter struct {
}

func (noOpAuthWriter) AuthenticateRequest(req runtime.ClientRequest, reg strfmt.Registry) error {
	return nil
}

func (c *RESTClient) refreshToken() error {
	ctx, cancel := context.WithTimeout(context.TODO(), c.opTimeout)
	defer cancel()

	lParams := &login.AcceptItemParams{
		Username:   newString(c.username),
		Password:   newString(c.password),
		Context:    ctx,
		HTTPClient: c.httpClient,
	}
	lOK, err := c.apiClient.Login.AcceptItem(lParams, noOpAuthWriter{})
	if err != nil {
		return fmt.Errorf("Error logging in: %s", err)
	}
	c.token = &token{
		token:     lOK.Payload.AccessToken,
		expiresAt: time.Now().Add(time.Duration(lOK.Payload.ExpiresIn) * time.Second),
	}
	return nil
}

func GetClient(host, username, password string, opTimeout time.Duration) Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	cl := &http.Client{Transport: tr}
	c := &RESTClient{
		mu: new(sync.Mutex),
		apiClient: ipc_client.NewHTTPClientWithConfig(nil, &ipc_client.TransportConfig{
			Host:     host,
			BasePath: ipc_client.DefaultBasePath,
			Schemes:  ipc_client.DefaultSchemes,
		}),
		httpClient: cl,
		opTimeout:  opTimeout,
		username:   username,
		password:   password,
	}
	return c
}

type IPUsageCIDR struct {
	Address                        string
	BlockSize                      int
	IP                             net.IP
	Net                            *net.IPNet
	IPTotal                        uint64
	IPFree                         uint64
	IPFreePercent                  float64
	LargestFreeContiguousBlockSize int
}

func AddressCount(network *net.IPNet) uint64 {
	prefixLen, bits := network.Mask.Size()
	return 1 << (uint64(bits) - uint64(prefixLen))
}

func updateCIDRInfo(CIDRs []*IPUsageCIDR, freeCIDR IPUsageCIDR) {
	for _, CIDR := range CIDRs {
		if CIDR.Net.Contains(freeCIDR.IP) {
			CIDR.IPFree = CIDR.IPFree + freeCIDR.IPFree
			if CIDR.IPTotal != 0 {
				CIDR.IPFreePercent = math.Floor((float64(CIDR.IPFree)/float64(CIDR.IPTotal))*100) / 100
			}
			if CIDR.LargestFreeContiguousBlockSize == 0 || freeCIDR.BlockSize < CIDR.LargestFreeContiguousBlockSize {
				CIDR.LargestFreeContiguousBlockSize = freeCIDR.BlockSize
			}
		}
	}
}

func (c *RESTClient) GetIPUsage() (*database.IPUsage, error) {
	var region = map[string]string{
		"us-east-1":     "Commercial/East",
		"us-west-2":     "Commercial/West",
		"us-gov-west-1": "GovCloud/West",
	}

	var stack = map[string]string{
		"Development and Test": "Lower",
		"Implementation":       "Prod",
		"Production":           "Prod",
		"Management":           "Zone",
		"Security":             "Zone",
		"Transport":            "Zone",
		"Prod-App":             "Prod",
		"Prod-Data":            "Prod",
		"Prod-Web":             "Prod",
		"Prod-Shared":          "Prod",
		"Prod-Shared-OC":       "Prod",
		"Lower-App":            "Lower",
		"Lower-Data":           "Lower",
		"Lower-Web":            "Lower",
		"Lower-Shared":         "Lower",
		"Lower-Shared-OC":      "Lower",
	}

	data := []*database.EnvironmentIPUsage{}

	for _, r := range region {
		regionContainer := r

		for s, environment := range stack {

			d := database.EnvironmentIPUsage{
				Region: r,
				CIDRs:  []*database.IPUsageCIDR{},
			}

			parentContainer := s
			path := "/Global/AWS/V4/" + regionContainer + "/" + parentContainer

			ipUsageCIDRs := []*IPUsageCIDR{}
			var largestFreeBlockSize int = 0
			var totalIPs uint64 = 0
			var totalFreeIPs uint64 = 0

			blocks, err := c.ListBlocks(path, false, false)
			if err != nil {
				return nil, fmt.Errorf("Error listing blocks: %s\n", err)
			}

			for _, block := range blocks {
				blockCIDR := block.BlockAddr
				blockSize := block.BlockSize
				blockIP, blockIPNet, err := net.ParseCIDR(blockCIDR + "/" + blockSize)
				if err != nil {
					return nil, fmt.Errorf("Error parsing CIDR: %s\n", err)
				}
				blockSizeValue, _ := blockIPNet.Mask.Size()
				blockAddressCount := AddressCount(blockIPNet)
				totalIPs = totalIPs + blockAddressCount
				ipUsageCIDR := IPUsageCIDR{
					Address:                        blockCIDR,
					BlockSize:                      blockSizeValue,
					IP:                             blockIP,
					Net:                            blockIPNet,
					IPTotal:                        blockAddressCount,
					IPFree:                         0,
					IPFreePercent:                  0,
					LargestFreeContiguousBlockSize: 0,
				}
				ipUsageCIDRs = append(ipUsageCIDRs, &ipUsageCIDR)
			}

			freeBlocks, err := c.ListBlocks(path, true, false)
			if err != nil {
				return nil, fmt.Errorf("Error listing blocks: %s\n", err)
			}
			for _, block := range freeBlocks {
				blockCIDR := block.BlockAddr
				blockSize := block.BlockSize
				blockIP, blockIPNet, err := net.ParseCIDR(blockCIDR + "/" + blockSize)
				blockAddressCount := AddressCount(blockIPNet)
				if err != nil {
					return nil, fmt.Errorf("Error parsing CIDR: %s\n", err)
				}
				blockSizeValue, _ := blockIPNet.Mask.Size()
				freeIPCIDR := IPUsageCIDR{
					Address:   blockCIDR,
					BlockSize: blockSizeValue,
					IP:        blockIP,
					Net:       blockIPNet,
					IPFree:    blockAddressCount,
				}
				updateCIDRInfo(ipUsageCIDRs, freeIPCIDR)
				totalFreeIPs = totalFreeIPs + blockAddressCount
			}

			for _, ipUsageCIDR := range ipUsageCIDRs {
				largestFreeContiguousBlock := ""
				if ipUsageCIDR.LargestFreeContiguousBlockSize != 0 {
					largestFreeContiguousBlock = "/" + strconv.Itoa(ipUsageCIDR.LargestFreeContiguousBlockSize)
				}
				d.CIDRs = append(d.CIDRs, &database.IPUsageCIDR{
					CIDR:                       ipUsageCIDR.Address + "/" + strconv.Itoa(ipUsageCIDR.BlockSize),
					IPTotal:                    ipUsageCIDR.IPTotal,
					IPFree:                     ipUsageCIDR.IPFree,
					IPFreePercent:              ipUsageCIDR.IPFreePercent,
					LargestFreeContiguousBlock: largestFreeContiguousBlock,
				})
				if largestFreeBlockSize == 0 || (ipUsageCIDR.LargestFreeContiguousBlockSize != 0 && ipUsageCIDR.LargestFreeContiguousBlockSize < largestFreeBlockSize) {
					largestFreeBlockSize = ipUsageCIDR.LargestFreeContiguousBlockSize
				}
			}
			d.Environment = environment
			d.Zone = parentContainer
			d.IPTotal = totalIPs
			d.IPFree = totalFreeIPs
			if totalIPs != 0 {
				d.IPFreePercent = math.Floor((float64(totalFreeIPs)/float64(totalIPs))*100) / 100
			}
			if largestFreeBlockSize != 0 {
				d.LargestFreeContiguousBlock = "/" + strconv.Itoa(largestFreeBlockSize)
			}
			data = append(data, &d)
		}
	}

	ipUsage := database.IPUsage{
		LastUpdated: time.Now(),
		Data:        []*database.EnvironmentIPUsage{},
	}
	ipUsage.Data = data
	return &ipUsage, nil
}
