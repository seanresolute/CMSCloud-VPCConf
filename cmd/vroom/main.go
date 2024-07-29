package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	awsc "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cloudtamer"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/connection"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/lib"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/networkfirewall/networkfirewalliface"
	"github.com/aws/aws-sdk-go/service/servicequotas"
	"github.com/go-test/deep"

	"github.com/go-redis/redis"
)

const (
	redisHost        = "127.0.0.1:6379"
	cacheCredentials = "credentials"
	cacheGlobal      = "global"
	cacheVPC         = "vpc"
	cacheVPCData     = "vpc-data"
	cacheBackup      = "backup"
)

var accountLocks map[string]*sync.Mutex
var cacheLocks map[string]*sync.Mutex

var globalAccountLock *sync.Mutex
var globalCacheLock *sync.Mutex

func LockCache(accountID string) {
	globalCacheLock.Lock()
	defer globalCacheLock.Unlock()
	if _, ok := cacheLocks[accountID]; !ok {
		cacheLocks[accountID] = &sync.Mutex{}
	}
	cacheLocks[accountID].Lock()
}

func UnlockCache(accountID string) {
	globalCacheLock.Lock()
	defer globalCacheLock.Unlock()
	if _, ok := cacheLocks[accountID]; !ok {
		cacheLocks[accountID] = &sync.Mutex{}
		fmt.Printf("Call to Unlock(%s) on uninitialized cache lock!", accountID)
		return
	}
	cacheLocks[accountID].Unlock()
}

func LockAccount(accountID string) {
	func() {
		globalAccountLock.Lock()
		defer globalAccountLock.Unlock()
		if _, ok := accountLocks[accountID]; !ok {
			accountLocks[accountID] = &sync.Mutex{}
		}
	}()
	accountLocks[accountID].Lock()
}

func UnlockAccount(accountID string) {
	func() {
		globalAccountLock.Lock()
		defer globalAccountLock.Unlock()
		if _, ok := accountLocks[accountID]; !ok {
			accountLocks[accountID] = &sync.Mutex{}
			fmt.Printf("Call to Unlock(%s) on uninitialized acccount lock!", accountID)
			return
		}
	}()
	accountLocks[accountID].Unlock()
}

const (
	durationWeek     = 7 * 24 * time.Hour
	durationInfinity = 0
)

const (
	defaultMaxRouteTableSize = 50
	routeTableSizeBuffer     = 0
	defaultPrefixListSize    = 32
)

const (
	routesPerRouteTableQuotaCode = "L-93826ACB"
)

const (
	regionAll     = "all"
	regionEast    = "us-east-1"
	regionWest    = "us-west-2"
	regionGovWest = "us-gov-west-1"
)
const minArgs = 1

const (
	subnetTypeUnroutable = "unroutable"
	subnetTypePrivate    = "Private"
	subnetTypeUnknown    = "unknown"
)

const (
	errTextDryRun               = "DryRunOperation"
	errTextInvalidVpcIDNotFound = "InvalidVpcID.NotFound"
)

const (
	exitCodeSuccess = iota
	exitCodeUsage
	exitCodeInvalidEnvironment
	exitCodeFatal
)

var (
	errCodeRouteExists = errors.New("route exists")
)

type AccountInfo struct {
	Name string
	ID   string
	VPCs []*database.VPC
}

type VPCConfig struct {
	ConnectPublic                      bool
	ConnectPrivate                     bool
	ManagedTransitGatewayAttachmentIDs []uint64
	ManagedResolverRuleSetIDs          []uint64
	SecurityGroupSetIDs                []uint64
	PeeringConnections                 []*database.PeeringConnectionConfig
}

type version string
type stack string

const (
	versionLegacy     version = "legacy"
	versionGreenfield version = "greenfield"
	stackShared       stack   = "shared"
	stackDev          stack   = "dev"
	stackImp          stack   = "impl"
	//S.M.
	stackNonProd stack = "nonprod"
	stackMgmt    stack = "mgmt"
	stackQA      stack = "qa"
	stackProd    stack = "prod"
	stackDefault stack = "default"
)

type testEndpointDefinition struct {
	Hostname *string
	IP       string
	Port     int16
}

var (
	sharedServiceTestEndpoints = map[string]testEndpointDefinition{
		"AD-1": {
			IP:   "10.244.112.49",
			Port: 636,
		},
		// "TrendMicro": {
		// 	Hostname: aws.String("internal-dsm-prod-elb-us-east-1-1932432501.us-east-1.elb.amazonaws.com"),
		// 	IP:       "10.223.126.6",
		// 	Port:     4120,
		// },
	}
)

const defaultTestEndpoint = "AD-1"

var (
	currentRegion = database.Region(regionEast)
	creds         = &awsc.CloudTamerAWSCreds{}
)

// List of available commands
var (
	commands = map[string]func(*credentials.Credentials, *task) ([]string, error){
		"audit":          auditDispatcher,
		"test":           testSharedServiceFromVPC,
		"cleanup-test":   cleanupTestInfrastructure,
		"refresh":        refreshVPCData,
		"info":           showVPCData,
		"routes":         showRouteInfo,
		"backup-routes":  backupRoutes,
		"restore-routes": restoreRoutes,
		"enable-dns":     enableDnsSupport,
		"show-backup":    showBackupRoutes,
		"backup-diff":    showBackupDiff,
		"quota-increase": requestQuotaIncrease,
		"list":           listVPCs,
		"account-audit":  auditAccounts,
		"count":          countVPCs,
		"comment":        generateComment,
		"find-external":  findIP,
		"egress":         showEgressIPs,
		"update-routes":  updateRoutes,
		"create-route":   createCustomRoute,
		"remove-route":   removeCustomRoute,
		"usage":          showIPUsage,
	}
	commandNames       = []string{}
	auditCommandNames  = []string{}
	offlineCommands    = []string{"list", "show-backup", "info"}
	accountCommands    = []string{"account-audit", "find-external", "block", "count", "usage"}
	tgwRelatedCommands = []string{"audit", "update-routes"}
	sgsRelatedCommands = []string{"audit"}

	layerTags = []string{"use", "vpc-conf-layer", "layer", "zone", "epl:zone"}

	auditCommands = map[string]func(*credentials.Credentials, *task) ([]string, error){
		"routes":          auditRoutes,
		"tables":          auditRouteTables,
		"public-routes":   auditPublics,
		"peers":           auditPeers,
		"vpcdns":          auditVpcDns,
		"unroutables":     auditUnroutables,
		"zones":           auditZones,
		"firewall":        auditFirewall,
		"security-groups": auditSecurityGroups,
	}
)

type tgwInfo struct {
	ID, AccountID string
}

var centralInfo = map[version]map[database.Region]tgwInfo{
	versionGreenfield: {
		regionEast: {
			ID:        "tgw-0f08888a7e2f3e38c",
			AccountID: "921617238787",
		},
		regionWest: {
			ID:        "tgw-085252a1e3587e1c7",
			AccountID: "921617238787",
		},
		regionGovWest: {
			ID:        "tgw-0e4a61bd21933eff6",
			AccountID: "849804945443",
		},
	},
}

var commercialRegions = []string{
	regionEast,
	regionWest,
}

var regions = []string{
	regionEast,
	regionWest,
	regionGovWest,
}

const (
	routeTableTypeUnknown = "unknown"
)

const (
	routeDescriptionUnknown = "unknown"
)

var (
	routeDescriptions = map[string]string{
		"10.128.0.0/16":   "AWS-OC east",
		"10.129.0.0/16":   "AWS-OC west",
		"10.131.125.0/24": "eLDAP impl",
		"10.138.1.0/24":   "eLDAP prod",
		"10.138.132.0/22": "shared lower",
		"10.220.120.0/22": "Greenfield Lower West",
		"10.220.126.0/23": "Greenfield Prod West",
		"10.220.128.0/20": "Greenfield Prod West",
		"10.223.120.0/22": "Greenfield Lower East",
		"10.223.126.0/23": "Greenfield Prod East",
		"10.223.128.0/20": "Greenfield Prod East",
		"10.235.58.0/24":  "eLDAP dev",
		"10.232.32.0/19":  "VPN",
		"10.240.120.0/22": "Greenfield Lower GovWest",
		"10.240.126.0/23": "Greenfield Prod GovWest",
		"10.240.128.0/20": "Greenfield Prod GovWest",
		"10.244.96.0/19":  "Shared entv3",
		"10.252.0.0/16":   "old legacy shared services - unneeded",
		"10.252.0.0/22":   "dev sec ops prod",
	}
	prefixLists = map[database.VPCType]map[database.Region]map[stack]string{
		database.VPCTypeLegacy: {
			regionEast: {
				stackShared: "",
				// stackProd:   "pl-0ffc092c126b667ff", // Unified-InterVPC-East-Prod
				// stackImp:    "pl-0a969cb30f03c3499", // Unified-InterVPC-East-Impl
				// stackDev:    "pl-0d490b86ca2360ddd", // Unified-InterVPC-East-Dev
				stackDefault: "pl-0b5dc9454db9407e9",
			},
		},
		database.VPCTypeV1: {
			regionEast: {
				stackDefault: "pl-0b5dc9454db9407e9",
			},
			regionGovWest: {
				stackDefault: "pl-fake",
			},
		},
	}
	oldPrefixLists = []string{
		"pl-0d490b86ca2360ddd", // Unified-InterVPC-East-Dev
		"pl-0a969cb30f03c3499", // Unified-InterVPC-East-Impl
		"pl-0ffc092c126b667ff", // Unified-InterVPC-East-Prod
	}
	allManagedRoutes = map[database.Region]map[stack][]string{
		regionEast: {
			stackShared: {},
			stackProd: {
				"10.244.96.0/19",
				"10.128.0.0/16",
				"10.232.32.0/19",
				"10.223.126.0/23",
				"10.223.128.0/20",
				"10.252.0.0/22",
				"10.138.1.0/24",
			},
			stackImp: {
				"10.244.96.0/19",
				"10.128.0.0/16",
				"10.232.32.0/19",
				"10.223.126.0/23",
				"10.138.132.0/22",
				"10.131.125.0/24",
				"10.223.120.0/22",
				"10.138.1.0/24",
				"10.235.58.0/24",
				"10.223.128.0/20",
				"10.252.0.0/22",
			},
			stackDev: {},
		},
	}
)

type Job struct {
	command string
	task    *task
}

type JobResult struct {
	job    Job
	output []string
	err    error
}

var (
	jobs    = make(chan Job)
	results = make(chan JobResult)
)

type redisCache struct {
	c *redis.Client
}

var cache = &redisCache{}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func generateCacheKey(scope, name string) string {
	return fmt.Sprintf("vroom/%s/%s", scope, name)
}

func initRedis(host string) *redisCache {
	c := redis.NewClient(&redis.Options{
		Addr: host,
	})

	if err := c.Ping().Err(); err != nil {
		panic("Unable to connect to redis " + err.Error())
	}
	client := &redisCache{
		c: c,
	}
	return client
}

func (client *redisCache) get(scope, name string, src interface{}) error {
	val, err := client.c.Get(generateCacheKey(scope, name)).Result()
	if err == redis.Nil || err != nil {
		return err
	}
	err = json.Unmarshal([]byte(val), &src)
	if err != nil {
		return err
	}
	return nil
}

func (client *redisCache) set(scope, name string, value interface{}, expiration time.Duration) error {
	cacheEntry, err := json.Marshal(value)
	if err != nil {
		return err
	}
	err = client.c.Set(generateCacheKey(scope, name), cacheEntry, expiration).Err()
	if err != nil {
		return err
	}
	return nil
}

type VROOMConfig struct {
	CloudTamerBaseURL      string
	VPCConfBaseURL         string
	UserName               string
	Password               string
	CloudTamerIDMSID       int
	CloudTamerAdminGroupID int
}

type routeTableEntry struct {
	CIDR        *string
	PLID        *string
	TGWID       *string
	OtherTarget *string
}

func (r *routeTableEntry) nextHop() string {
	if r.TGWID != nil {
		return *r.TGWID
	} else if r.OtherTarget != nil {
		return *r.OtherTarget
	}
	return "Unknown Destination"
}

func (r *routeTableEntry) destination() string {
	if r.CIDR != nil {
		return *r.CIDR
	} else if r.PLID != nil {
		return *r.PLID
	}
	return "Unknown Target"
}

type routeTable struct {
	Type    string
	Subnets []string
	Routes  []*routeTableEntry
}

type securityGroup struct {
	Name  string
	Rules []*securityGroupRule
}

type securityGroupRule struct {
	*database.SecurityGroupRule
}

type VPCInfo struct {
	*database.VPC
	IsDefault   bool
	CIDRs       []string
	RouteTables map[string]*routeTable
	Type        database.VPCType
}

type ip struct {
	IP        net.IP
	PrivateIP net.IP
}

type task struct {
	AccountID         *string
	APIRegion         *string
	VPC               VPCInfo
	EC2               ec2iface.EC2API
	NetworkFirewall   networkfirewalliface.NetworkFirewallAPI
	dryRun            bool
	config            tgwInfo
	version           version
	api               *VPCConfAPI
	ctx               context.Context
	exceptionVPCList  []string
	tgwTemplates      map[uint64]*database.ManagedTransitGatewayAttachment
	sgsTemplates      map[uint64]*database.SecurityGroupSet
	quiet             bool
	verbose           bool
	shared            bool
	testEndpoint      string
	routesToFilter    []string
	args              []string
	usePrivilegedRole bool
	routeTables       map[string]*ec2.RouteTable
	securityGroups    map[string]*ec2.SecurityGroup
	routeBlocks       []*net.IPNet
	ips               map[string]*ip
	limitVPC          string
	filterCIDR        string
}

func (t *task) LogString(msg string, args ...interface{}) string {
	str := fmt.Sprintf(msg, args...)
	drs := ""
	if t.dryRun {
		drs = " DRY-RUN"
	}
	context := func() string {
		if t.AccountID != nil {
			return aws.StringValue(t.AccountID)
		}
		return fmt.Sprintf("%s/%s", t.VPC.AccountID, t.VPC.ID)
	}()
	return fmt.Sprintf("[%s%s] %s", context, drs, str)
}

func failTask(s string, a ...interface{}) ([]string, error) {
	return []string{fmt.Sprintf(s, a...)}, fmt.Errorf(s, a...)
}

func (t *task) Debug(msg string, args ...interface{}) {
	if t.verbose {
		log.Printf("%s", t.LogString(fmt.Sprintf("DEBUG %s", msg), args...))
	}
}

func (t *task) Log(msg string, args ...interface{}) {
	log.Printf("%s", t.LogString(fmt.Sprintf("INFO %s", msg), args...))
}

func (t *task) Warn(msg string, args ...interface{}) {
	log.Printf("%s", t.LogString(fmt.Sprintf("WARN %s", msg), args...))
}

func (t *task) Error(msg string, args ...interface{}) {
	log.Printf("%s", t.LogString(fmt.Sprintf("ERROR %s", msg), args...))
}

func (t *task) getRawRouteTables() error {
	t.Debug("getRawRouteTables()")
	VPC := t.VPC
	rto, err := t.EC2.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(VPC.ID)},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, rt := range rto.RouteTables {
		t.routeTables[aws.StringValue(rt.RouteTableId)] = rt
	}
	return nil
}

func (t *task) getRawSecurityGroups() error {
	t.Debug("getRawSecurityGroups()")
	VPC := t.VPC
	sgo, err := t.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(VPC.ID)},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, sg := range sgo.SecurityGroups {
		t.securityGroups[aws.StringValue(sg.GroupId)] = sg
	}
	return nil
}

func (t *task) getIPs() (map[string]*ip, error) {
	ao, err := t.EC2.DescribeAddresses(&ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}
	for _, addr := range ao.Addresses {
		ip := &ip{
			PrivateIP: net.ParseIP(aws.StringValue(addr.PrivateIpAddress)),
		}
		// This has an external IP
		if addr.PublicIp != nil {
			ip.IP = net.ParseIP(aws.StringValue(addr.PublicIp))
		}
		t.ips[aws.StringValue(addr.NetworkInterfaceId)] = ip
	}

	nio, err := t.EC2.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{})
	if err != nil {
		return nil, err
	}
	for _, ni := range nio.NetworkInterfaces {
		if _, ok := t.ips[aws.StringValue(ni.NetworkInterfaceId)]; ok {
			continue
		}
		ip := &ip{
			PrivateIP: net.ParseIP(aws.StringValue(ni.PrivateIpAddress)),
		}
		// This has an external IP
		if ni.Association != nil && ni.Association.PublicIp != nil {
			ip.IP = net.ParseIP(aws.StringValue(ni.Association.PublicIp))
		}
		t.ips[aws.StringValue(ni.NetworkInterfaceId)] = ip
	}
	return t.ips, nil
}

func (t *task) getVPCs() ([]*ec2.Vpc, error) {
	vo, err := t.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: aws.StringSlice([]string{"false"}),
			},
		},
	})
	return vo.Vpcs, err
}

func (t *task) countVPCs() (int, error) {
	vpcs, err := t.getVPCs()
	if err != nil {
		return 0, err
	}

	return len(vpcs), nil
}

type PeeringConnection struct {
	RequesterVPCID      string
	RequesterRegion     database.Region
	RequesterAccount    string
	AccepterVPCID       string
	AccepterRegion      database.Region
	AccepterAccount     string
	PeeringConnectionID string
	IsAccepted          bool
}

type peeringConnection struct {
	State *PeeringConnection
	// For routes:
	SubnetIDs     []string
	OtherVPCCIDRs []string
}

func getPeeringConnections(e ec2iface.EC2API, vpcId string) (map[string]*peeringConnection, error) {
	peeringConnections := make(map[string]*peeringConnection)
	pcro, err := e.DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("requester-vpc-info.vpc-id"),
				Values: aws.StringSlice([]string{vpcId}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	pcao, err := e.DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("accepter-vpc-info.vpc-id"),
				Values: aws.StringSlice([]string{vpcId}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	for _, out := range []*ec2.DescribeVpcPeeringConnectionsOutput{pcro, pcao} {
		for _, pc := range out.VpcPeeringConnections {
			peeringObject := &peeringConnection{
				State: &PeeringConnection{
					RequesterVPCID:      aws.StringValue(pc.RequesterVpcInfo.VpcId),
					RequesterRegion:     database.Region(aws.StringValue(pc.RequesterVpcInfo.Region)),
					RequesterAccount:    aws.StringValue(pc.RequesterVpcInfo.OwnerId),
					AccepterVPCID:       aws.StringValue(pc.AccepterVpcInfo.VpcId),
					AccepterRegion:      database.Region(aws.StringValue(pc.AccepterVpcInfo.Region)),
					AccepterAccount:     aws.StringValue(pc.AccepterVpcInfo.OwnerId),
					PeeringConnectionID: aws.StringValue(pc.VpcPeeringConnectionId),
				},
			}
			if aws.StringValue(pc.Status.Code) == ec2.VpcPeeringConnectionStateReasonCodeActive {
				peeringObject.State.IsAccepted = true
			}
			peeringConnections[aws.StringValue(pc.VpcPeeringConnectionId)] = peeringObject
		}
	}
	return peeringConnections, nil
}

func (t *task) getPeeringConnections() (map[string]*peeringConnection, error) {
	return getPeeringConnections(t.EC2, t.VPC.ID)
}

func getRouteTables(e ec2iface.EC2API, input map[string]*ec2.RouteTable) (map[string]*routeTable, error) {
	routeTables := make(map[string]*routeTable)
	for id, rtb := range input {
		routes := []*routeTableEntry{}
		tableType := routeTableTypeUnknown
		for _, tag := range rtb.Tags {
			if aws.StringValue(tag.Key) == "Type" {
				tableType = aws.StringValue(tag.Value)
			}
		}
		// Couldn't find a Type tag on the route table itself, so walk the subnets for "use" tags (a la greenfield)
		if tableType == routeTableTypeUnknown {
			subnets := []*string{}
			for _, association := range rtb.Associations {
				if association.SubnetId != nil && *association.SubnetId != "" {
					subnets = append(subnets, association.SubnetId)
				}
			}
			if len(subnets) > 0 {
				out, err := e.DescribeSubnets(&ec2.DescribeSubnetsInput{
					SubnetIds: subnets,
				})
				if err != nil {
					return nil, err
				} else {
					for _, subnetInfo := range out.Subnets {
						for _, tag := range subnetInfo.Tags {
							if aws.StringValue(tag.Key) == "use" {
								tableType = aws.StringValue(tag.Value)
							}
						}
					}
				}
			}
		}
		for _, r := range rtb.Routes {
			route := &routeTableEntry{
				CIDR: r.DestinationCidrBlock,
				PLID: r.DestinationPrefixListId,
			}
			if r.DestinationCidrBlock != nil || r.DestinationPrefixListId != nil {
				if r.TransitGatewayId != nil {
					route.TGWID = r.TransitGatewayId
				} else if r.EgressOnlyInternetGatewayId != nil {
					route.OtherTarget = r.EgressOnlyInternetGatewayId
				} else if r.GatewayId != nil {
					route.OtherTarget = r.GatewayId
				} else if r.InstanceId != nil {
					route.OtherTarget = r.InstanceId
				} else if r.LocalGatewayId != nil {
					route.OtherTarget = r.LocalGatewayId
				} else if r.NatGatewayId != nil {
					route.OtherTarget = r.NatGatewayId
				} else if r.NetworkInterfaceId != nil {
					route.OtherTarget = r.NetworkInterfaceId
				} else if r.VpcPeeringConnectionId != nil {
					route.OtherTarget = r.VpcPeeringConnectionId
				} else {
					route.OtherTarget = aws.String("blackhole?")
				}
			}
			routes = append(routes, route)
		}
		subnets := []string{}
		for _, a := range rtb.Associations {
			if a.SubnetId == nil {
				continue
			}
			subnets = append(subnets, aws.StringValue(a.SubnetId))
		}
		routeTables[id] = &routeTable{
			Type:    tableType,
			Routes:  routes,
			Subnets: subnets,
		}
	}

	return routeTables, nil
}

func (t *task) getRouteTables() (map[string]*routeTable, error) {
	err := t.getRawRouteTables()
	if err != nil {
		t.Error("fetching raw route tables")
		return nil, err
	}
	return getRouteTables(t.EC2, t.routeTables)
}

func (t *task) getMaximumRouteTableSize() (int, error) {
	awscreds, err := getCredsByAccountID(creds, t.VPC.AccountID)
	if err != nil {
		log.Fatalf("Error getting credentials for %s: %s", t.VPC.AccountID, err)
	}
	quotas := servicequotas.New(getSessionFromCreds(awscreds, string(t.VPC.Region)))
	out, err := quotas.GetServiceQuota(&servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(routesPerRouteTableQuotaCode),
		ServiceCode: aws.String("vpc"),
	})
	if err != nil {
		t.Error("Fetching quotas for vpcs: %s\n", err)
		return defaultMaxRouteTableSize, err
	}
	return int(*out.Quota.Value), nil
}

func (t *task) checkForSupernets(table *routeTable) ([]string, error) {
	lessSpecificRoutes := []string{}
	for _, mr := range allManagedRoutes[t.VPC.Region][stack(t.VPC.Stack)] {
		for _, r := range table.Routes {
			if strings.HasPrefix(r.nextHop(), "igw-") || strings.HasPrefix(r.nextHop(), "nat-") {
				// Don't worry about igw or nat destinations, those are assumed to go off-net
				continue
			}
			if !(strings.HasPrefix(r.destination(), "pl-")) {
				within, err := cidrIsWithin(mr, r.destination())
				if err != nil {
					return []string{}, fmt.Errorf("cidrIsWithin: %s", err)
				}
				if within {
					lessSpecificRoutes = append(lessSpecificRoutes, r.destination())
				}
			}
		}
	}
	return lessSpecificRoutes, nil
}

func (t *task) getPrefixListSize(prefixList *string) int {
	if prefixList != nil {
		out, err := t.EC2.DescribeManagedPrefixLists(
			&ec2.DescribeManagedPrefixListsInput{
				PrefixListIds: []*string{prefixList},
			},
		)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == "InvalidPrefixListID.NotFound" && !t.quiet {
					t.Error("Prefix list isn't shared with this vpc? Not found")
				}
			} else {
				t.Error("DescribeManagedPrefixLists(): %s", err)
			}
			return 0
		}
		for _, pl := range out.PrefixLists {
			if aws.StringValue(pl.PrefixListId) == aws.StringValue(prefixList) {
				return int(aws.Int64Value(pl.MaxEntries))
			}
		}
	} else {
		return 0
	}
	return 0
}

func (t *task) checkOrUpdatePLRoutes(nextHop *string, performUpdates bool) error {
	prefixList := t.getPrefixList()
	return t.checkOrUpdateRoutes(prefixList, nextHop, performUpdates, nil)
}

// checkOrUpdateRoutes: Check or update routes, but limit functionality to only the listed limitToRouteTables (nil: act on all tables)
func (t *task) checkOrUpdateRoutes(destination *string, nextHop *string, performUpdates bool, limitToRouteTables []string) error {
	additionalSize := 0
	isPrefixList := false
	if strings.HasPrefix(aws.StringValue(destination), "pl-") {
		prefixListSize := t.getPrefixListSize(destination)
		if prefixListSize == 0 {
			prefixListSize = defaultPrefixListSize
		}
		additionalSize = additionalSize + prefixListSize
		isPrefixList = true
	} else {
		additionalSize = 1
	}
	maxRouteTableSize, _ := t.getMaximumRouteTableSize()
	err := t.getRawRouteTables()
	if err != nil {
		return err
	}
	tableIsTooLarge := false
	for routeTableId, table := range t.routeTables {
		if len(limitToRouteTables) > 0 && !stringInSlice(routeTableId, limitToRouteTables) {
			continue
		}
		existingRouteCount := 0
		blackholeRouteCount := 0
		actualGrowth := additionalSize
		for _, r := range table.Routes {
			incr := 1
			if r.DestinationCidrBlock == nil {
				incr = t.getPrefixListSize(r.DestinationPrefixListId)
			}
			if (r.DestinationCidrBlock != nil && aws.StringValue(r.DestinationCidrBlock) == aws.StringValue(destination)) || (r.DestinationPrefixListId != nil && aws.StringValue(r.DestinationPrefixListId) == aws.StringValue(destination)) {
				actualGrowth = 0
			}
			switch aws.StringValue(r.State) {
			case ec2.RouteStateActive:
				existingRouteCount = existingRouteCount + incr
			case ec2.RouteStateBlackhole:
				blackholeRouteCount = blackholeRouteCount + incr
			default:
				t.Error("Unknown route state: %s\n", aws.StringValue(r.State))
			}
		}
		if existingRouteCount+actualGrowth+blackholeRouteCount > maxRouteTableSize-routeTableSizeBuffer {
			tableIsTooLarge = true
			if existingRouteCount+actualGrowth < maxRouteTableSize-routeTableSizeBuffer {
				t.Warn("%s Too many routes on %s: %d + %d + %d blackholes = %d (> %d, with %d route buffer)", t.VPC.AccountID, routeTableId, existingRouteCount, actualGrowth, blackholeRouteCount, existingRouteCount+actualGrowth+blackholeRouteCount, maxRouteTableSize, routeTableSizeBuffer)
			} else {
				t.Error("%s Too many routes on %s: %d + %d + %d blackholes = %d (> %d, with %d route buffer)", t.VPC.AccountID, routeTableId, existingRouteCount, actualGrowth, blackholeRouteCount, existingRouteCount+actualGrowth+blackholeRouteCount, maxRouteTableSize, routeTableSizeBuffer)
			}
		} else if !t.quiet && !performUpdates {
			t.Log("Route count on %s: %d + %d = %d", routeTableId, existingRouteCount, actualGrowth, existingRouteCount+actualGrowth)
		}
	}
	if tableIsTooLarge {
		return fmt.Errorf("Route table is too large to apply route updates")
	}
	if destination != nil {
		routeTables, err := t.getRouteTables()
		if err != nil {
			return fmt.Errorf("Getting route tables")
		}
		for routeTableId, table := range routeTables {
			if len(limitToRouteTables) > 0 && !stringInSlice(routeTableId, limitToRouteTables) {
				continue
			}
			supernetsExist := false
			if isPrefixList {
				lessSpecificRoutes, err := t.checkForSupernets(table)
				if err != nil {
					return fmt.Errorf("Checking for supernets: %s", err)
				}
				if len(lessSpecificRoutes) > 0 {
					t.Error("Route table %s has less specific routes (%d): %s", routeTableId, len(lessSpecificRoutes), strings.Join(lessSpecificRoutes, ", "))
					supernetsExist = true
				}
			}
			if !supernetsExist {
				if nextHop != nil {
					err := t.createNewRouteToTGW(routeTableId, aws.StringValue(destination), aws.StringValue(nextHop))
					if err != nil {
						if err != errCodeRouteExists {
							t.Error("Failed to create route for %s -> %s on %s: %s\n", aws.StringValue(destination), nextHop, routeTableId, err)
						}
					} else {
						t.Log("Route created: %s -> %s on %s\n", aws.StringValue(destination), aws.StringValue(nextHop), routeTableId)
					}
				} else {
					t.Warn("nextHop is nil for %s / %s", routeTableId, aws.StringValue(destination))
				}
			}
		}
	}
	return nil
}

func (t *task) createNewRouteToTGW(routeTableId, destination, tgwId string) error {
	for _, checkRoute := range t.routeTables[routeTableId].Routes {
		if (aws.StringValue(checkRoute.DestinationCidrBlock) == destination) || (aws.StringValue(checkRoute.DestinationPrefixListId) == destination) {
			t.Log("%s -> %s pre-exists on %s\n", destination, tgwId, routeTableId)
			if aws.StringValue(checkRoute.TransitGatewayId) != tgwId {
				t.Warn("%s points incorrectly\n", destination)
			}
			return errCodeRouteExists
		}
	}

	startedTrying := time.Now()
	maxTryTime := 30 * time.Second

	for {
		input := &ec2.CreateRouteInput{
			RouteTableId:     aws.String(routeTableId),
			TransitGatewayId: aws.String(tgwId),
			DryRun:           aws.Bool(t.dryRun),
		}
		if isValidCIDR(destination) {
			input.DestinationCidrBlock = aws.String(destination)
		} else {
			input.DestinationPrefixListId = aws.String(destination)
		}
		if _, err := t.EC2.CreateRoute(input); err == nil {
			break
		} else {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == errTextDryRun {
					break
				}
			}
			t.Error("CreateRoute(): %s\n", err)

		}
		if time.Since(startedTrying) > maxTryTime {
			return fmt.Errorf("timeout")
		}
		time.Sleep(5 * time.Second)
	}
	routeToAdd := &ec2.Route{
		TransitGatewayId: aws.String(tgwId),
	}
	if isValidCIDR(destination) {
		routeToAdd.DestinationCidrBlock = aws.String(destination)
	} else {
		routeToAdd.DestinationPrefixListId = aws.String(destination)
	}
	t.routeTables[routeTableId].Routes = append(t.routeTables[routeTableId].Routes, routeToAdd)
	return nil
}

func (t *task) deleteRoute(routeTableId, target string) error {
	startedTrying := time.Now()
	maxTryTime := 30 * time.Second

	for {
		input := &ec2.DeleteRouteInput{
			RouteTableId: aws.String(routeTableId),
			DryRun:       aws.Bool(t.dryRun),
		}
		if isValidCIDR(target) {
			input.DestinationCidrBlock = aws.String(target)
		} else if strings.HasPrefix(target, "pl-") {
			input.DestinationPrefixListId = aws.String(target)
		}
		if _, err := t.EC2.DeleteRoute(input); err == nil {
			break
		} else {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == errTextDryRun {
					break
				}
			}
			t.Error("DeleteRoute(): %s\n", err)

		}
		if time.Since(startedTrying) > maxTryTime {
			return fmt.Errorf("timeout")
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (t *task) getRouteDescription(route string) string {
	if desc, ok := routeDescriptions[route]; ok {
		return desc
	}
	if isValidCIDR(route) {
		for haystack, desc := range routeDescriptions {
			within, err := cidrIsWithin(route, haystack)
			if err != nil {
				t.Error("checking cidr: %s\n", route)
				return ""
			}
			if within {
				return fmt.Sprintf("within %s", desc)
			}
		}
	}
	return routeDescriptionUnknown
}

func (t *task) maskCIDRs() []string {
	re := regexp.MustCompile(`^\d+\.\d+\.`)
	maskedCIDRs := make([]string, 0)
	for _, cidr := range t.VPC.CIDRs {
		maskedCIDRs = append(maskedCIDRs, re.ReplaceAllString(cidr, "x.x."))
	}
	return maskedCIDRs
}

func (t *task) getCIDRs() ([]string, error) {
	return getCIDRs(t.EC2, t.VPC.ID)
}

func saveVPC(vpcID string, vpc VPCInfo) {
	cache.set(cacheVPC, vpcID, vpc, durationWeek)
	cache.set(cacheVPCData, fmt.Sprintf("%s/mtga", vpcID), vpc.Config.ManagedTransitGatewayAttachmentIDs, durationWeek)
	cache.set(cacheVPCData, fmt.Sprintf("%s/sgs", vpcID), vpc.Config.SecurityGroupSetIDs, durationWeek)
	cache.set(cacheVPCData, fmt.Sprintf("%s/state", vpcID), lib.ObjectToMap(vpc.State), durationWeek)
}

func (t *task) saveVPC() {
	saveVPC(t.VPC.ID, t.VPC)
}

func (t *task) routeIsSubnetOfRoutes(destination string, cidrs []string) (string, bool, error) {
	for _, block := range cidrs {
		if strings.HasPrefix(block, "pl-") {
			continue
		}
		within, err := cidrIsWithin(destination, block)
		if err != nil {
			return "", false, err
		} else if within {
			return block, true, nil
		}
	}
	return "", false, nil
}

func (t *task) routeIsSupernetOfRoutes(destination string, cidrs []string) (string, bool, error) {
	for _, block := range t.routeBlocks {
		_, within, err := t.routeIsSubnetOfRoutes(block.String(), []string{destination})
		if err != nil {
			return "", false, err
		} else if within {
			return block.String(), true, nil
		}
	}
	return "", false, nil
}

func (t *task) ignoreRouteFromSharedService(route string) bool {
	if t.VPC.Stack == "shared" && stringInSlice(route, t.routesToFilter) {
		return true
	}
	return false
}

func getSecurityGroups(e ec2iface.EC2API, input map[string]*ec2.SecurityGroup) (map[string]*securityGroup, error) {
	securityGroups := make(map[string]*securityGroup)
	for id, sg := range input {
		rules := make([]*securityGroupRule, 0)
		for _, ingressRule := range sg.IpPermissions {
			for _, ipRange := range ingressRule.IpRanges {
				rule := &database.SecurityGroupRule{
					Description:    aws.StringValue(ipRange.Description),
					IsEgress:       false,
					Protocol:       aws.StringValue(ingressRule.IpProtocol),
					FromPort:       aws.Int64Value(ingressRule.FromPort),
					ToPort:         aws.Int64Value(ingressRule.ToPort),
					Source:         aws.StringValue(ipRange.CidrIp),
					SourceIPV6CIDR: "",
				}
				rules = append(rules, &securityGroupRule{rule})
			}
			for _, ipRange := range ingressRule.Ipv6Ranges {
				rule := &database.SecurityGroupRule{
					Description:    aws.StringValue(ipRange.Description),
					IsEgress:       false,
					Protocol:       aws.StringValue(ingressRule.IpProtocol),
					FromPort:       aws.Int64Value(ingressRule.FromPort),
					ToPort:         aws.Int64Value(ingressRule.ToPort),
					Source:         "",
					SourceIPV6CIDR: aws.StringValue(ipRange.CidrIpv6),
				}
				rules = append(rules, &securityGroupRule{rule})
			}
		}

		for _, egressRule := range sg.IpPermissionsEgress {
			for _, ipRange := range egressRule.IpRanges {
				rule := &database.SecurityGroupRule{
					Description:    aws.StringValue(ipRange.Description),
					IsEgress:       true,
					Protocol:       aws.StringValue(egressRule.IpProtocol),
					FromPort:       aws.Int64Value(egressRule.FromPort),
					ToPort:         aws.Int64Value(egressRule.ToPort),
					Source:         aws.StringValue(ipRange.CidrIp),
					SourceIPV6CIDR: "",
				}
				rules = append(rules, &securityGroupRule{rule})
			}
			for _, ipRange := range egressRule.Ipv6Ranges {
				rule := &database.SecurityGroupRule{
					Description:    aws.StringValue(ipRange.Description),
					IsEgress:       true,
					Protocol:       aws.StringValue(egressRule.IpProtocol),
					FromPort:       aws.Int64Value(egressRule.FromPort),
					ToPort:         aws.Int64Value(egressRule.ToPort),
					Source:         "",
					SourceIPV6CIDR: aws.StringValue(ipRange.CidrIpv6),
				}
				rules = append(rules, &securityGroupRule{rule})
			}
		}
		sgo, err := e.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("group-id"),
					Values: []*string{aws.String(id)},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(sgo.SecurityGroups) != 1 {
			return nil, fmt.Errorf("DescribeSecurityGroups(%s) returned %d security groups", id, len(sgo.SecurityGroups))
		}
		securityGroups[id] = &securityGroup{
			Name:  aws.StringValue(sgo.SecurityGroups[0].GroupName),
			Rules: rules,
		}
	}
	return securityGroups, nil
}

func (t *task) getSecurityGroups() (map[string]*securityGroup, error) {
	err := t.getRawSecurityGroups()
	if err != nil {
		t.Error("fetching raw security groups")
		return nil, err
	}
	return getSecurityGroups(t.EC2, t.securityGroups)
}

type VPCConfAPI struct {
	Username, Password string
	APIKey             string
	BaseURL            string
	AutomatedVPCs      map[database.Region][]*database.VPC
	verbose            bool
	Lock               sync.Mutex
}

type BatchTaskRequest struct {
	TaskTypes uint64 // bitmap of taskTypex
	VPCs      []struct {
		ID, Region string
	}
	AddManagedTransitGatewayAttachments    []uint64
	RemoveManagedTransitGatewayAttachments []uint64
}

func (api *VPCConfAPI) Login() error {
	api.Lock.Lock()
	defer api.Lock.Unlock()
	api.APIKey = os.Getenv("VROOM_APIKEY")
	return nil
}

func (api *VPCConfAPI) SubmitBatchTask(request BatchTaskRequest, dryRun bool) error {
	log.Println("SubmitBatchTask()")
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return err
		}
	}
	// Make sure tgwut *only* touches networking
	request.TaskTypes &= 1
	b, err := json.MarshalIndent(request, "", "   ")
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/batch", api.BaseURL)
	if dryRun {
		log.Printf("DRY-RUN: %s\n", url)
		log.Printf("APIKey: %s\n", api.APIKey)
		log.Printf("DRY-RUN: Payload: %s\n", string(b))
	} else {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("ERROR: Submitting batch task: %s\n", resp.Status)
			body, _ := ioutil.ReadAll(resp.Body)
			log.Println(string(body))
			return fmt.Errorf("submitting batch task request")
		}
	}
	return nil
}

func (api *VPCConfAPI) FetchAutomatedVPCsByRegion(region database.Region) ([]*database.VPC, error) {
	if api.AutomatedVPCs[region] != nil {
		return api.AutomatedVPCs[region], nil
	}
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequest("GET", api.BaseURL+"/batch/vpcs.json", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("ERROR: fetching list of automated vpcs: %s\n", resp.Status)
		return nil, fmt.Errorf("fetching list of automated vpcs")
	}
	var info struct {
		Regions []database.Region
		VPCs    []*database.VPC
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, err
	}
	vpcs := make([]*database.VPC, 0)
	for _, vpc := range info.VPCs {
		if vpc.Region == region {
			vpcs = append(vpcs, vpc)
		}
	}
	api.AutomatedVPCs[region] = vpcs
	return vpcs, nil
}

func (api *VPCConfAPI) SearchForEntity(searchString string) ([]byte, error) {
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	request := struct {
		SearchTerm string
	}{
		SearchTerm: searchString,
	}
	b, err := json.MarshalIndent(request, "", "   ")
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/search/do", api.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		log.Println(string(body))
		return nil, fmt.Errorf("submitting vpc search request")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (api *VPCConfAPI) FindVPC(vpcName string) (*database.VPC, error) {
	type vpcFromSearch struct {
		Name string
		URL  string
	}
	var info struct {
		Results []struct {
			VPCs []vpcFromSearch
		}
	}
	body, err := api.SearchForEntity(vpcName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(body, &info)
	if err != nil {
		return nil, err
	}
	// Dumb way to do it, but it'll work for now
	// Returns accountID, vpcID, region
	parseURL := func(url string) (string, string, string) {
		urlParts := strings.Split(url, "/")
		return urlParts[3], urlParts[6], urlParts[5]
	}
	printVPCnames := func(vpcs []vpcFromSearch) {
		for _, vpc := range vpcs {
			_, id, region := parseURL(vpc.URL)
			log.Printf("VPC: %s (%s [%s])\n", id, vpc.Name, region)
		}
	}
	if len(info.Results) != 1 {
		for _, result := range info.Results {
			printVPCnames(result.VPCs)
		}
		return nil, fmt.Errorf("Invalid number of search results for %s (%d)", vpcName, len(info.Results))
	}
	if len(info.Results[0].VPCs) != 1 {
		printVPCnames(info.Results[0].VPCs)
		return nil, fmt.Errorf("Invalid number of VPCs returned for %s (%d)", vpcName, len(info.Results[0].VPCs))
	}
	acctID, vpcID, region := parseURL(info.Results[0].VPCs[0].URL)
	vpc, err := api.FetchVPCDetails(acctID, region, vpcID)
	if err != nil {
		return nil, err
	}
	return vpc, nil
}

func (api *VPCConfAPI) FetchExceptionalVPCs() ([]string, error) {
	if api.verbose {
		log.Println("FetchExceptionalVPCs()")
	}
	exceptions := []string{}
	cacheKeyName := "vpc-conf/exceptional-vpcs"
	err := cache.get(cacheGlobal, cacheKeyName, &exceptions)
	if err != nil {
		if api.APIKey == "" {
			if err := api.Login(); err != nil {
				return exceptions, err
			}
		}
		req, err := http.NewRequest("GET", api.BaseURL+"/exception.json", nil)
		if err != nil {
			return exceptions, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return exceptions, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("ERROR: fetching list of exceptional vpcs: %s\n", resp.Status)
			return exceptions, fmt.Errorf("fetching list of exceptional vpcs")
		}
		var vpcs []*database.VPC
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return exceptions, err
		}
		err = json.Unmarshal(body, &vpcs)
		if err != nil {
			return exceptions, err
		}
		for _, vpc := range vpcs {
			exceptions = append(exceptions, vpc.ID)
		}
		cache.set(cacheGlobal, cacheKeyName, exceptions, 10*time.Minute)
	}
	return exceptions, nil
}

func (api *VPCConfAPI) FetchAutomatedVPCsByAccount(account string) (*AccountInfo, error) {
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/accounts/%s.json", api.BaseURL, account)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("ERROR: fetching vpcs for account %s: %s\n", account, resp.Status)
		return nil, fmt.Errorf("fetching vpc details: %s", resp.Body)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	type VPCInfo struct {
		AccountID string
		Name      string
		VPCID     string

		IsAutomated bool
		IsException bool
		Config      *VPCConfig
	}
	type RegionInfo struct {
		Name string
		VPCs []VPCInfo
	}
	type AccountPageInfo struct {
		AccountID   string
		AccountName string
		ProjectName string
		IsGovCloud  bool
		Regions     []RegionInfo

		DefaultRegion string
		ServerPrefix  string
	}
	accountInfo := &AccountPageInfo{}
	err = json.Unmarshal(body, &accountInfo)
	if err != nil {
		return nil, err
	}
	vpcs := make([]*database.VPC, 0)
	for _, region := range accountInfo.Regions {
		for _, vpc := range region.VPCs {
			if vpc.IsAutomated || vpc.IsException {
				vpcs = append(vpcs, &database.VPC{
					AccountID: vpc.AccountID,
					ID:        vpc.VPCID,
					Name:      vpc.Name,
					Region:    database.Region(region.Name),
				})
			}
		}
	}
	return &AccountInfo{
		VPCs: vpcs,
		Name: accountInfo.AccountName,
		ID:   accountInfo.AccountID,
	}, nil
}

var mapOfInterfaces = reflect.TypeOf(map[string]interface{}{})

func underlyingType(in reflect.Value) reflect.Type {
	return underlyingValue(in).Type()
}

func underlyingValue(in reflect.Value) reflect.Value {
	return func(v reflect.Value) reflect.Value {
		for {
			switch k := v.Kind(); k {
			case reflect.Interface, reflect.Ptr:
				if v.Elem().IsValid() {
					v = v.Elem()
				} else {
					return v
				}
			default:
				return v
			}
		}
	}(in)
}

func getInitializedValueOf(in interface{}) reflect.Value {
	rv := reflect.ValueOf(in)
	rve := reflect.ValueOf(&in).Elem()
	switch k := rv.Kind(); k {
	case reflect.Ptr:
		if rv.IsNil() {
			rve.Set(reflect.New(reflect.TypeOf(in).Elem()))
			return rve
		}
	case reflect.Map:
		if rv.IsNil() {
			rve.Set(reflect.MakeMap(reflect.TypeOf(in)))
			if rve.Kind() == reflect.Interface {
				return rve.Elem()
			}
			return rve
		}
	case reflect.Slice:
		if rv.IsNil() {
			rve.Set(reflect.MakeSlice(reflect.TypeOf(in), 0, 0))
			return rve
		}
	}
	return rv
}

func rehydrateObject(input, output interface{}) error {
	return copyRecursive(reflect.ValueOf(input), reflect.ValueOf(output))
}

const DEBUG = false

// Keep these in case you need them
/*
func debugf(f string, a ...interface{}) {
	if DEBUG {
		fmt.Printf(f, a...)
	}
}

func debugln(a ...interface{}) {
	if DEBUG {
		fmt.Println(a...)
	}
}
*/

func copyRecursive(input, output reflect.Value) error {
	inputReferencedType := underlyingType(input)
	outputReferencedType := underlyingType(output)

	// Check if this is a double pointer
	if outputReferencedType.Kind() == reflect.Ptr && reflect.Indirect(output).Kind() == reflect.Ptr {
		return copyRecursive(input, output.Elem())
	}

	// Simple cases
	if input.Type() == outputReferencedType {
		output.Set(input)
		return nil
	}
	if underlyingValue(input).CanConvert(outputReferencedType) {
		output.Set(underlyingValue(input).Convert(outputReferencedType))
		return nil
	}
	if inputReferencedType.AssignableTo(outputReferencedType) {
		output.Set(input)
		return nil
	}

	switch output.Kind() {

	// Ptr on the output
	case reflect.Ptr:
		if output.IsNil() {
			output.Set(reflect.New(output.Type().Elem()))
		}
		return copyRecursive(underlyingValue(input), output.Elem())

	// Struct as an output
	case reflect.Struct:
		switch inputReferencedKind := inputReferencedType.Kind(); inputReferencedKind {
		// Support a case copying from map to struct
		case reflect.Map:

			// ONLY support map[string]interface{}
			if input.Type() != mapOfInterfaces {
				return fmt.Errorf("unsupported input type: %s", input.Type())
			}
			for i := 0; i < outputReferencedType.NumField(); i++ {
				fieldName := outputReferencedType.Field(i).Name
				iter := input.MapRange()
				for iter.Next() {
					k := iter.Key()
					if fieldName == k.String() {
						v := iter.Value()
						err := copyRecursive(underlyingValue(v), output.Field(i))
						if err != nil {
							return err
						}
					}
				}
			}
		default:
			return fmt.Errorf("skipping struct output with input type %s\n", input.Type())
		}

	// Map as an output
	case reflect.Map:
		switch inputReferencedKind := input.Kind(); inputReferencedKind {

		// Support a case of map on the input
		case reflect.Map:
			// ONLY support map[string]interface{}
			if input.Type() != mapOfInterfaces {
				return fmt.Errorf("unsupported map to map input type: %s", input.Type())
			}
			temporaryMap := getInitializedValueOf(underlyingValue(output).Interface())
			// debugln(temporaryMap.Type(), temporaryMap.Kind(), temporaryMap.CanAddr(), temporaryMap.CanSet())
			iter := input.MapRange()
			for iter.Next() {
				k := iter.Key()
				v := iter.Value()
				newObject := reflect.New(outputReferencedType.Elem())
				err := copyRecursive(v, newObject)
				if err != nil {
					return err
				}
				temporaryMap.SetMapIndex(k, newObject.Elem())
			}
			output.Set(temporaryMap)
		default:
			return fmt.Errorf("skipping map output with input type %s\n", input.Type())
		}

	// Slice as an output
	case reflect.Slice:
		switch inputReferencedKind := inputReferencedType.Kind(); inputReferencedKind {

		// Slice to slice copy
		case reflect.Slice:
			// if input.Kind() == output.Kind() {
			output.Set(reflect.MakeSlice(output.Type(), input.Len(), input.Cap()))
			for i := 0; i < input.Len(); i++ {
				err := copyRecursive(input.Index(i), output.Index(i))
				if err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("skipping slice output with input type %s\n", input.Type())
		}
	default:
		return fmt.Errorf("Unsupported output type %s (%s) %s\n", output.Type(), underlyingType(output).Kind(), underlyingType(output))
	}
	return nil
}

func (api *VPCConfAPI) FetchVPCState(account, region, vpcID string) (*database.VPCState, error) {
	if api.verbose {
		log.Printf("FetchVPCState(%s, %s, %s)\n", account, region, vpcID)
	}
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/accounts/%s/vpc/%s/%s/state.json", api.BaseURL, account, region, vpcID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		log.Printf("ERROR: fetching vpc %s: %s %s\n", vpcID, resp.Status, b)
		return nil, fmt.Errorf("fetching vpc details")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	vpcstatemap := make(map[string]interface{})
	err = json.Unmarshal(body, &vpcstatemap)
	if err != nil {
		return nil, err
	}
	vpcstate := database.VPCState{}
	err = rehydrateObject(vpcstatemap, &vpcstate)
	if err != nil {
		return nil, err
	}
	return &vpcstate, nil
}

func (api *VPCConfAPI) FetchVPCDetails(account, region, vpcID string) (*database.VPC, error) {
	if api.verbose {
		log.Printf("FetchVPCDetails(%s, %s, %s)\n", account, region, vpcID)
	}
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/accounts/%s/vpc/%s/%s.json", api.BaseURL, account, region, vpcID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		log.Printf("ERROR: fetching vpc %s: %s %s\n", vpcID, resp.Status, b)
		return nil, fmt.Errorf("fetching vpc details")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	type VPCInfo struct {
		AccountID string
		Name      string
		VPCID     string

		IsException bool
		Config      *VPCConfig
		VPCType     database.VPCType
	}
	vpcinfo := &VPCInfo{}
	err = json.Unmarshal(body, &vpcinfo)
	if err != nil {
		return nil, err
	}
	vpcs, err := api.FetchAutomatedVPCsByRegion(database.Region(region))
	if err != nil {
		return nil, err
	}
	vpcConfig := &database.VPCConfig{
		ConnectPublic:                      vpcinfo.Config.ConnectPublic,
		ConnectPrivate:                     vpcinfo.Config.ConnectPrivate,
		ManagedTransitGatewayAttachmentIDs: vpcinfo.Config.ManagedTransitGatewayAttachmentIDs,
		SecurityGroupSetIDs:                vpcinfo.Config.SecurityGroupSetIDs,
		ManagedResolverRuleSetIDs:          vpcinfo.Config.ManagedResolverRuleSetIDs,
		PeeringConnections:                 vpcinfo.Config.PeeringConnections,
	}
	for _, vpc := range vpcs {
		if vpc.ID == vpcinfo.VPCID {
			vpc.Config = vpcConfig
			state, err := api.FetchVPCState(account, region, vpcID)
			if err != nil {
				return nil, err
			}
			vpc.State = state
			return vpc, nil
		}
	}
	// Fall-through: the vpc doesn't appear in the automated list
	vpc := &database.VPC{
		ID:        vpcinfo.VPCID,
		AccountID: vpcinfo.AccountID,
		Name:      vpcinfo.Name,
		Region:    database.Region(region),
	}
	vpc.State = &database.VPCState{}
	if vpcinfo.IsException {
		vpc.Config = &database.VPCConfig{
			ManagedTransitGatewayAttachmentIDs: []uint64{},
			SecurityGroupSetIDs:                []uint64{},
		}
	} else {
		vpc.Config = vpcConfig
	}
	return vpc, nil
}

func (api *VPCConfAPI) FetchManagedTGWTemplates() (map[uint64]*database.ManagedTransitGatewayAttachment, error) {
	templates := make(map[uint64]*database.ManagedTransitGatewayAttachment)
	cacheKeyName := "vpc-conf/mtgas"
	err := cache.get(cacheGlobal, cacheKeyName, &templates)
	if err != nil {
		if api.APIKey == "" {
			if err := api.Login(); err != nil {
				return nil, err
			}
		}
		url := fmt.Sprintf("%s/mtgas.json", api.BaseURL)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("ERROR: fetching mtga templates: %s\n", resp.Status)
			return nil, fmt.Errorf("fetching mtga templates")
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		mtgas := make([]*database.ManagedTransitGatewayAttachment, 0)
		err = json.Unmarshal(body, &mtgas)
		if err != nil {
			return nil, err
		}
		for _, mtga := range mtgas {
			templates[mtga.ID] = mtga
		}
		cache.set(cacheGlobal, cacheKeyName, templates, 1*time.Minute)
	}
	return templates, nil
}

func (api *VPCConfAPI) FetchManagedSecurityGroupTemplates() (map[uint64]*database.SecurityGroupSet, error) {
	templates := make(map[uint64]*database.SecurityGroupSet)
	cacheKeyName := "vpc-conf/sgs"
	err := cache.get(cacheGlobal, cacheKeyName, &templates)
	if err != nil {
		if api.APIKey == "" {
			if err := api.Login(); err != nil {
				return nil, err
			}
		}
		url := fmt.Sprintf("%s/sgs.json", api.BaseURL)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("ERROR: fetching sgs templates: %s\n", resp.Status)
			return nil, fmt.Errorf("fetching sgs templates")
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		sgss := make([]*database.SecurityGroupSet, 0)
		err = json.Unmarshal(body, &sgss)
		if err != nil {
			return nil, err
		}
		for _, sgs := range sgss {
			templates[sgs.ID] = sgs
		}
		cache.set(cacheGlobal, cacheKeyName, templates, 1*time.Minute)
	}
	return templates, nil
}

func (api *VPCConfAPI) FetchAllAccountDetails() (map[string]*database.AWSAccount, error) {
	if api.APIKey == "" {
		if err := api.Login(); err != nil {
			return nil, err
		}
	}
	url := fmt.Sprintf("%s/accounts/accounts.json", api.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("ERROR: fetching accounts: %s\n", resp.Status)
		return nil, fmt.Errorf("fetching all account details")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	accountInfo := []*database.AWSAccount{}
	err = json.Unmarshal(body, &accountInfo)
	if err != nil {
		return nil, err
	}
	accounts := make(map[string]*database.AWSAccount)
	for _, a := range accountInfo {
		accounts[a.ID] = a
	}
	return accounts, nil
}

func getCIDRs(e ec2iface.EC2API, vpcID string) ([]string, error) {
	cidrs := make([]string, 1)
	vpco, err := e.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{aws.String(vpcID)},
	})
	if err != nil {
		return []string{}, err
	}
	if len(vpco.Vpcs) != 1 {
		return []string{}, fmt.Errorf("Unable to get cidrs for %s, invalid qty (%d)\n", vpcID, len(vpco.Vpcs))
	}
	cidrs[0] = aws.StringValue(vpco.Vpcs[0].CidrBlock)
	for _, cidrAssociation := range vpco.Vpcs[0].CidrBlockAssociationSet {
		if !stringInSlice(aws.StringValue(cidrAssociation.CidrBlock), cidrs) {
			cidrs = append(cidrs, aws.StringValue(cidrAssociation.CidrBlock))
		}
	}
	return cidrs, nil
}

func getCIDRsForVPC(accountID, vpcID string, region database.Region) ([]string, error) {
	EC2, _, err := getRWAccountCredentials(accountID, region)
	if err != nil {
		return nil, err
	}
	cidrs, err := getCIDRs(EC2, vpcID)
	return cidrs, err
}

func cidrIsWithin(needle, haystack string) (bool, error) {
	needleIP, needleBlock, err := net.ParseCIDR(needle)
	if err != nil {
		return false, err
	}
	_, haystackBlock, err := net.ParseCIDR(haystack)
	if err != nil {
		return false, err
	}
	needleOnes, _ := needleBlock.Mask.Size()
	haystackOnes, _ := haystackBlock.Mask.Size()
	if haystackBlock.Contains(needleIP) && needleOnes > haystackOnes {
		return true, nil
	} else {
		return false, nil
	}
}

func subnetSize(cidr string) (int, error) {
	_, block, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	netSize, totalSize := block.Mask.Size()
	return 1 << (totalSize - netSize), nil
}

func intInSlice(i int, s []int) bool {
	for _, e := range s {
		if i == e {
			return true
		}
	}
	return false
}

func stringInSlice(str string, sl []string) bool {
	for _, s := range sl {
		if s == str {
			return true
		}
	}
	return false
}

func getRWAccountCredentials(accountID string, region database.Region) (*ec2.EC2, *networkfirewall.NetworkFirewall, error) {
	awscreds, err := getCredsByAccountID(creds, accountID)
	if err != nil {
		log.Printf("Error getting credentials for %s: %s", accountID, err)
		return nil, nil, err
	}
	return ec2.New(getSessionFromCreds(awscreds, string(region))), networkfirewall.New(getSessionFromCreds(awscreds, string(region))), nil
}

func getCentralAccountCredentials(version version, region database.Region) (*ec2.EC2, *networkfirewall.NetworkFirewall, error) {
	if _, ok := centralInfo[version][region]; !ok {
		return nil, nil, fmt.Errorf("No account info for %s/%s", version, string(region))
	}
	return getRWAccountCredentials(centralInfo[version][region].AccountID, region)
}

func (t *task) getPrefixListEntries(prefixList *string) ([]string, error) {
	if prefixList != nil {
		out, err := t.EC2.GetManagedPrefixListEntries(
			&ec2.GetManagedPrefixListEntriesInput{
				PrefixListId: prefixList,
			},
		)
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == "InvalidPrefixListID.NotFound" && !t.quiet {
					return nil, err
				}
			}
			return nil, err
		}
		entries := []string{}
		for _, entry := range out.Entries {
			entries = append(entries, aws.StringValue(entry.Cidr))
		}
		return entries, nil
	}
	return nil, nil
}

func isValidCIDR(in string) bool {
	_, _, err := net.ParseCIDR(in)
	return (err == nil)
}

func (t *task) getPrefixList() *string {
	return getPrefixList(func() database.VPCType {
		if t.VPC.State != nil {
			return t.VPC.State.VPCType
		} else {
			return -1
		}
	}(), t.VPC.Region, stack(t.VPC.Stack))
}

func getPrefixList(vpcType database.VPCType, region database.Region, requestedStack stack) *string {
	if _, ok := prefixLists[vpcType]; !ok {
		log.Printf("WARN: Unsupported vpc type: %q\n", vpcType)
		pl := prefixLists[database.VPCTypeV1][region][stackDefault]
		return &pl
	}
	if _, ok := prefixLists[vpcType][region]; !ok {
		log.Printf("WARN: Unsupported region: %s\n", region)
		return nil
	}
	if pl, ok := prefixLists[vpcType][region][requestedStack]; ok {
		return &pl
	} else {
		log.Printf("Unsupported stack: %s\n", requestedStack)
		if pl, ok = prefixLists[vpcType][region][stackDefault]; ok {
			log.Printf("Choosing default PL %s\n", pl)
			return &pl
		}
		return nil
	}
}

func getVPCsAttachedToTGW(EC2 *ec2.EC2, tgw tgwInfo) ([]*database.VPC, error) {
	rv := []*database.VPC{}
	moreAvailable := true
	cacheKeyName := fmt.Sprintf("transit-gateway-attachments/%s", tgw.ID)
	err := cache.get(cacheGlobal, cacheKeyName, &rv)
	var nextToken *string
	if err != nil {
		for moreAvailable {
			in := &ec2.DescribeTransitGatewayAttachmentsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("transit-gateway-id"),
						Values: []*string{aws.String(tgw.ID)},
					},
					{
						Name:   aws.String("resource-type"),
						Values: []*string{aws.String("vpc")},
					},
					{
						Name:   aws.String("state"),
						Values: []*string{aws.String("available")},
					},
				},
			}
			if nextToken != nil {
				in.NextToken = nextToken
			}
			out, describeErr := EC2.DescribeTransitGatewayAttachments(in)
			if describeErr != nil {
				return nil, describeErr
			}
			for _, tga := range out.TransitGatewayAttachments {
				vpc := &database.VPC{
					ID:        aws.StringValue(tga.ResourceId),
					AccountID: aws.StringValue(tga.ResourceOwnerId),
					Region:    currentRegion,
				}
				rv = append(rv, vpc)
			}
			nextToken = out.NextToken
			moreAvailable = (nextToken != nil)
		}
		cache.set(cacheGlobal, cacheKeyName, rv, 30*time.Minute)
	}
	return rv, nil
}

func getSessionFromCreds(creds *credentials.Credentials, region string) *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Region:      &region,
		Credentials: creds,
	}))
}

func getCredsByAccountID(creds *awsc.CloudTamerAWSCreds, accountID string) (*credentials.Credentials, error) {
	var awsc *credentials.Credentials
	LockCache(accountID)
	defer UnlockCache(accountID)
	cachedCreds := &credentials.Value{}
	err := cache.get(cacheCredentials, accountID, cachedCreds)
	if err != nil {
		log.Printf("  Getting new credentials for account %s\n", accountID)
		startedTrying := time.Now()
		maxTryTime := 10 * time.Second

		for {
			awsc, err = creds.GetCredentialsForAccount(accountID)
			if err != nil {
				log.Printf("ERROR getting credentials for account %s: %s", accountID, err)
				if err.Error() == "CloudTamer did not return any valid IAM roles to assume" {
					return nil, err
				}
				if time.Since(startedTrying) > maxTryTime {
					return nil, fmt.Errorf("timeout")
				}
				time.Sleep(2 * time.Second)
			} else {
				storedCreds, err := awsc.Get()
				if err != nil {
					return nil, err
				}
				cachedCreds = &storedCreds
				cache.set(cacheCredentials, accountID, cachedCreds, 25*time.Minute)
				break
			}
		}
	} else {
		log.Printf("  Got cached credentials for account %s\n", accountID)
		awsc = credentials.NewStaticCredentialsFromCreds(*cachedCreds)
	}
	return awsc, err
}

func fetchVPCName(e ec2iface.EC2API, vpcID string) (string, error) {
	startedTrying := time.Now()
	maxTryTime := 30 * time.Second

	for {
		out, err := e.DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: []*string{aws.String(vpcID)},
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == errTextInvalidVpcIDNotFound {
					return "", err
				}
			}
			log.Printf("Error from DescribeVpcs(): %s\n", err)
		} else {
			if len(out.Vpcs) > 0 {
				for _, tag := range out.Vpcs[0].Tags {
					if strings.ToLower(aws.StringValue(tag.Key)) == "name" {
						return aws.StringValue(tag.Value), nil
					}
				}
			}
		}
		if time.Since(startedTrying) > maxTryTime {
			return "", fmt.Errorf("timeout")
		}
		time.Sleep(5 * time.Second)
	}
}

// auditDispatcher: The primary entry point for "audit". Use the subsequent args to pick further functionality
func auditDispatcher(awscreds *credentials.Credentials, t *task) ([]string, error) {
	command := strings.ToLower(t.args[0])
	return auditCommands[command](awscreds, t)
}

// auditUnroutables: Audit the usage of unroutable space
func auditUnroutables(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := make([]string, 0)
	compliant := true
	subnets, err := t.getSubnetsByUses(subnetTypeUnroutable)
	if err != nil {
		return nil, err
	}
	if len(subnets) == 0 {
		t.Log("No unroutable subnets found")
	} else {
		t.Debug("Checking list of subnets: [%s]", strings.Join(subnets, ","))
		output, err := t.EC2.DescribeInstances(&ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{t.VPC.ID}),
				},
				{
					Name:   aws.String("subnet-id"),
					Values: aws.StringSlice(subnets),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		if len(output.Reservations) > 0 {
			for _, reservation := range output.Reservations {
				for _, instance := range reservation.Instances {
					notes = append(notes, fmt.Sprintf("Instance ID %s is in an unroutable subnet (%s)", aws.StringValue(instance.InstanceId), aws.StringValue(instance.SubnetId)))
					compliant = false
				}
			}
		}
		t.Log("Auditing unroutables")
		if !compliant {
			notes = append(notes, "Unroutables have disallowed infrastructure in them")
		} else {
			notes = append(notes, "Unroutables seemingly fully compliant")
		}
	}
	return notes, nil
}

// auditFirewall: Audit the usage of network-firewall (really just to test the API)
func auditFirewall(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := make([]string, 0)
	fwOut, err := t.NetworkFirewall.ListFirewalls(&networkfirewall.ListFirewallsInput{})
	if err != nil {
		fmt.Println("===")
		fmt.Println(err)
		fmt.Println("===")
		return nil, err
	}
	for _, fw := range fwOut.Firewalls {
		notes = append(notes, fmt.Sprintf("Firewall: %s", aws.StringValue(fw.FirewallName)))
	}
	return notes, nil
}

// auditZones: Audit the usage of zoned subnets
func auditZones(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := make([]string, 0)
	rawSubnets, err := t.getRawSubnets()
	if err != nil {
		return nil, err
	}
	subnetsByZone := make(map[string][]string)
	for _, subnet := range rawSubnets {
		// Check if the use is what we want
		for _, tag := range subnet.Tags {
			t.Debug("Found tag %s = %s", aws.StringValue(tag.Key), aws.StringValue(tag.Value))
			if strings.ToLower(aws.StringValue(tag.Key)) == "use" {
				if _, ok := subnetsByZone[aws.StringValue(tag.Value)]; !ok {
					subnetsByZone[aws.StringValue(tag.Value)] = make([]string, 0)
				}
				subnetsByZone[aws.StringValue(tag.Value)] = append(subnetsByZone[aws.StringValue(tag.Value)], aws.StringValue(subnet.SubnetId))
			}
		}
	}
	for zone, subnets := range subnetsByZone {
		notes = append(notes, fmt.Sprintf("%s = %d", zone, len(subnets)))
	}
	return notes, nil
}

// auditVpcDns: Audit the VPC attributes related to DNS
func auditVpcDns(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := make([]string, 0)
	fullyEnabled := true
	for _, attribute := range []string{"enableDnsHostnames", "enableDnsSupport"} {
		o, err := t.EC2.DescribeVpcAttribute(&ec2.DescribeVpcAttributeInput{
			VpcId:     aws.String(t.VPC.ID),
			Attribute: aws.String(attribute),
		})
		if err != nil {
			return nil, err
		}
		if (o.EnableDnsHostnames != nil && !aws.BoolValue(o.EnableDnsHostnames.Value)) || (o.EnableDnsSupport != nil && !aws.BoolValue(o.EnableDnsSupport.Value)) {
			fullyEnabled = false
			t.Warn("%s is disabled", attribute)
		}
	}
	if !fullyEnabled {
		notes = append(notes, "DNS attributes not fully enabled")
	} else {
		notes = append(notes, "All DNS attributes enabled")
	}
	return notes, nil
}

// auditPeers: Audit the list of peers currently discovered in the VPC, both against vpc-conf's configurations as well as TRB guidelines and completeness
func auditPeers(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := []string{}
	actualPeers, err := t.getPeeringConnections()
	if err != nil {
		return nil, err
	}
	t.Log("Found %d peer%s", len(actualPeers), pluralS(len(actualPeers)))
	unconfiguredPeers := make([]string, 0)
	for _, peer := range actualPeers {
		isConfigured := false
		for _, configuredPeer := range t.VPC.Config.PeeringConnections {
			if (configuredPeer.IsRequester && peer.State.AccepterVPCID == configuredPeer.OtherVPCID && peer.State.AccepterRegion == configuredPeer.OtherVPCRegion) ||
				(!configuredPeer.IsRequester && peer.State.RequesterVPCID == configuredPeer.OtherVPCID && peer.State.RequesterRegion == configuredPeer.OtherVPCRegion) {
				isConfigured = true
				break
			}
		}
		if !isConfigured {
			t.Warn("Found non-automated peer %s: %s/%s -> %s/%s", peer.State.PeeringConnectionID, peer.State.RequesterVPCID, peer.State.RequesterRegion, peer.State.AccepterVPCID, peer.State.AccepterRegion)
			unconfiguredPeers = append(unconfiguredPeers, peer.State.PeeringConnectionID)
		}
	}

	getTablesWithRoutingToPeer := func(e ec2iface.EC2API, peerId string) (map[string]*routeTable, error) {
		out, err := e.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("route.vpc-peering-connection-id"),
					Values: aws.StringSlice([]string{peerId}),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		routeTables := make(map[string]*ec2.RouteTable)
		for _, rt := range out.RouteTables {
			routeTables[aws.StringValue(rt.RouteTableId)] = rt
		}
		return getRouteTables(e, routeTables)
	}

	// Gather subnet type information from both sides of each peer, then audit the routing
	for _, peer := range actualPeers {
		subnetTypes := make(map[string]string)
		subnetCidrs := make(map[string]string)
		tablesWithRoute, err := getTablesWithRoutingToPeer(t.EC2, peer.State.PeeringConnectionID)
		if err != nil {
			return nil, err
		}
		otherVPCID := func(p *peeringConnection) string {
			if p.State.AccepterVPCID == t.VPC.ID {
				return p.State.RequesterVPCID
			}
			return p.State.AccepterVPCID
		}(peer)
		otherAcctID := func(p *peeringConnection) string {
			if p.State.AccepterVPCID == t.VPC.ID {
				return p.State.RequesterAccount
			}
			return p.State.AccepterAccount
		}(peer)
		otherRegion := func(p *peeringConnection) database.Region {
			if p.State.AccepterVPCID == t.VPC.ID {
				return p.State.RequesterRegion
			}
			return p.State.AccepterRegion
		}(peer)
		remoteCredentials, _, err := getRWAccountCredentials(otherAcctID, otherRegion)
		if err != nil {
			return nil, err
		}
		remoteTablesWithRoute, err := getTablesWithRoutingToPeer(remoteCredentials, peer.State.PeeringConnectionID)
		if err != nil {
			return nil, err
		}
		for _, rt := range tablesWithRoute {
			t.Debug("Local Table says %s", rt.Type)
			for _, subnet := range rt.Subnets {
				localSubnetCidr, localSubnetType, err := getSubnetInfo(t.EC2, t.VPC.ID, subnet)
				if err != nil {
					return nil, err
				}
				if localSubnetType == subnetTypeUnknown {
					t.Warn("Found type %s for local subnet %s", localSubnetType, subnet)
				} else {
					t.Debug("Found type %s for local subnet %s", localSubnetType, subnet)
				}
				if rt.Type != localSubnetType {
					t.Debug("Mismatch between local table tags and subnet tags: %s vs %s", rt.Type, localSubnetType)
				}
				subnetTypes[subnet] = localSubnetType
				subnetCidrs[subnet] = localSubnetCidr
			}
		}
		for _, rt := range remoteTablesWithRoute {
			for _, subnet := range rt.Subnets {
				remoteSubnetCidr, remoteSubnetType, err := getSubnetInfo(remoteCredentials, otherVPCID, subnet)
				if err != nil {
					return nil, err
				}
				if remoteSubnetType == subnetTypeUnknown {
					t.Warn("Found type %s for remote subnet %s in %s/%s", remoteSubnetType, subnet, otherAcctID, otherVPCID)
				} else {
					t.Debug("Found type %s for remote subnet %s in %s/%s", remoteSubnetType, subnet, otherAcctID, otherVPCID)
				}
				if rt.Type != remoteSubnetType {
					t.Debug("Mismatch between remote route table tags and subnet tags: %s vs %s", rt.Type, remoteSubnetType)
				}
				subnetTypes[subnet] = remoteSubnetType
				subnetCidrs[subnet] = remoteSubnetCidr
			}
		}

		// Audit the routing between the two peers
		for _, localRouteTable := range tablesWithRoute {
			for _, localRoute := range localRouteTable.Routes {
				if localRoute.nextHop() == peer.State.PeeringConnectionID {
					t.Log("Subnet%s %s has route to %s via %s", pluralS(len(localRouteTable.Subnets)), strings.Join(localRouteTable.Subnets, ", "), localRoute.destination(), localRoute.nextHop())
					for _, remoteRouteTable := range remoteTablesWithRoute {
						for _, remoteRoute := range remoteRouteTable.Routes {
							if remoteRoute.nextHop() == peer.State.PeeringConnectionID {
								for _, remoteSubnet := range remoteRouteTable.Subnets {
									remoteCidr := subnetCidrs[remoteSubnet]
									routeWithinRemoteCidr, err := cidrIsWithin(localRoute.destination(), remoteCidr)
									if err != nil {
										return nil, err
									}
									if routeWithinRemoteCidr {
										t.Log("Local route to %s is within remote cidr %s [%s -> %s]", localRoute.destination(), remoteCidr, subnetTypes[localRouteTable.Subnets[0]], subnetTypes[remoteSubnet])
									}
									// routeIncludesRemoteCidr, err := cidrIsWithin(remoteCidr, localRoute.destination())
									// if err != nil {
									// 	return nil, err
									// }
								}
							}
						}
					}
				}
			}
		}
	}
	j, _ := json.MarshalIndent(t.VPC.Config.PeeringConnections, "", "   ")
	t.Debug("Configured peering connections: %s", string(j))
	j, _ = json.MarshalIndent(actualPeers, "", "   ")
	t.Debug("Actual peering connections: %s", string(j))
	if len(unconfiguredPeers) > 0 {
		t.Warn("Found unconfigured peer%s (%d): %s", pluralS(len(unconfiguredPeers)), len(unconfiguredPeers), strings.Join(unconfiguredPeers, ", "))
	}
	return notes, nil
}

// auditRoutes: Audit the list of routes currently on the route tables within the current VPC
// If there are any extra routes we don't care about, or if any of the ones we do care about are missing, flag it as an issue
func auditRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	notes := []string{}
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRouteTables(): %w\n", err)
	}
	delta := func(a []string) int {
		if len(a) > 1 {
			d, err := strconv.Atoi(a[1])
			if err != nil {
				t.Warn("Invalid delta (%s) provided: %s", a[1], err)
				return 0
			}
			return d
		}
		return 0
	}(t.args)

	maxRouteTableSize, err := t.getMaximumRouteTableSize()
	if err != nil {
		t.Error("fetching maximum route table size: %s", err)
	}

	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	allVerified := true
	templates, err := t.getConfiguredTGWTemplates()
	if err != nil {
		t.Error("fetching configured tgw templates: %s", err)
	}

	// Keep track of all the tables we've tracked, so later we can check the remaining ones
	checkedTables := make(map[string]bool, len(routeTables))
	for routeTableId := range routeTables {
		checkedTables[routeTableId] = false
	}

	for routeTableId, table := range routeTables {
		extraRoutes := []string{}
		misdirectedRoutes := map[string]*routeTableEntry{}
		missingRoutes := []string{}
		routeVerified := make(map[string]bool)
		tableHasTGWTarget := false
		// Iterate each template this VPC is attached to, and find which map to this current route table
		for mtgaID, template := range templates {
			t.Debug("Checking template ID %d\n", mtgaID)
			// wipe out the inuse vpcs for the display, we don't care about the others
			template.InUseVPCs = []string{t.VPC.ID}
			j, _ := json.MarshalIndent(template, "", "   ")
			t.Debug("Comparing route table with:\n%s\n", string(j))
			applicableTypes := make([]string, 0)
			for _, t := range template.SubnetTypes {
				applicableTypes = append(applicableTypes, strings.ToLower(string(t)))
			}
			if !stringInSlice(strings.ToLower(table.Type), applicableTypes) {
				continue
			}
			for _, k := range template.Routes {
				routeVerified[k] = routeVerified[k] || false
			}
			for _, route := range table.Routes {
				t.Debug("Checking route %s -> %s [%s]\n", route.destination(), route.nextHop(), routeTableId)
				routeHasTGWTarget := route.nextHop() == template.TransitGatewayID
				isManagedRoute := stringInSlice(route.destination(), template.Routes)
				tableHasTGWTarget = tableHasTGWTarget || routeHasTGWTarget
				// Short cut for exact matches
				if isManagedRoute && routeHasTGWTarget {
					routeVerified[route.destination()] = true
					continue
				}
				if !strings.HasPrefix(route.destination(), "pl-") {
					supernet, routeDestinationIsWithinManagedRoute, err := t.routeIsSubnetOfRoutes(route.destination(), template.Routes)
					if err != nil {
						t.Error("checking if %s is longer prefix: %s\n", route.destination(), err)
						continue
					}
					// This route is a subnet of a managed route, extra (or mispointed?)
					if routeDestinationIsWithinManagedRoute {
						if routeHasTGWTarget {
							notes = append(notes, fmt.Sprintf("More specific route %s -> %s is longer prefix within %s\n", route.destination(), route.nextHop(), supernet))
							if !t.ignoreRouteFromSharedService(route.destination()) {
								extraRoutes = append(extraRoutes, route.destination())
							}
						} else {
							notes = append(notes, fmt.Sprintf("Mispointed route %s -> %s is longer prefix within %s\n", route.destination(), route.nextHop(), supernet))
							misdirectedRoutes[route.destination()] = route
						}
					} else if routeHasTGWTarget {
						// Look to see if it's a shorter prefix
						subnet, contains, err := t.routeIsSupernetOfRoutes(route.destination(), template.Routes)
						if err != nil {
							t.Error("checking if %s is shorter prefix: %s\n", route.destination(), err)
							continue
						}
						if contains {
							notes = append(notes, fmt.Sprintf("Less specific route %s -> %s contains longer prefix %s\n", route.destination(), route.nextHop(), subnet))
						}
						extraRoutes = append(extraRoutes, route.destination())
					} else {
						t.Debug("Skipping %s -> %s (not pointing at TGW)", route.destination(), route.nextHop())
					}
				} else {
					// Only prefix lists that aren't managed are here
					if routeHasTGWTarget {
						notes = append(notes, fmt.Sprintf("Extra prefix list route: %s -> %s", route.destination(), route.nextHop()))
						extraRoutes = append(extraRoutes, route.destination())
					}
				}
			}
			if !tableHasTGWTarget {
				continue
			}
		}
		routesVerified := true
		for _, exists := range routeVerified {
			if !exists {
				routesVerified = false
			}
		}
		if routesVerified && len(extraRoutes) == 0 && len(misdirectedRoutes) == 0 {
			if !t.quiet {
				t.Log("route table %s GOOD", routeTableId)
			}
		} else {
			allVerified = false
			if t.quiet {
				t.Warn("route table %s not matching desired state", routeTableId)
			}
			sort.Strings(extraRoutes)
			for _, note := range notes {
				t.Warn("%s %s", routeTableId, note)
			}
			routes := []string{}
			for k := range routeVerified {
				routes = append(routes, k)
			}
			sort.Strings(routes)
			if !t.quiet {
				for _, route := range routes {
					if routeVerified[route] {
						t.Log("%s valid %s\n", routeTableId, route)
					}
				}
			}
			if len(table.Subnets) == 0 {
				t.Warn("%s has routes to TGW, but no subnets associated", routeTableId)
			} else {
				for _, route := range routes {
					missing := false
					if route == "10.252.0.0/22" {
						if !routeVerified[route] && !stringInSlice("10.252.0.0/16", extraRoutes) {
							missing = true
						}
					} else if !routeVerified[route] {
						missing = true
					}
					if missing {
						t.Error("%s missing %s (%s)\n", routeTableId, route, t.getRouteDescription(route))
						missingRoutes = append(missingRoutes, route)
					}
				}
			}
			if !t.quiet {
				for _, route := range extraRoutes {
					t.Warn("%s extra %s (%s)\n", routeTableId, route, t.getRouteDescription(route))
				}
				for block, route := range misdirectedRoutes {
					t.Warn("%s wrong target: %s -> %s\n", routeTableId, block, route.nextHop())
				}
				t.Log("%s on subnets [%s]\n", routeTableId, strings.Join(table.Subnets, ","))
			}
		}
		newSize := len(table.Routes) + delta + len(missingRoutes) - len(extraRoutes)
		t.Log("Deltas %d (+%d/-%d): %d -> %d", delta, len(missingRoutes), len(extraRoutes), len(table.Routes), newSize)
		if newSize > maxRouteTableSize {
			t.Warn("RT_SIZE: New table would exceed existing quota: %d > %d", newSize, maxRouteTableSize)
		} else {
			t.Log("RT_SIZE: Estimated new table size fits within existing quota: %d <= %d", newSize, maxRouteTableSize)
		}
	}
	if allVerified {
		t.Log("All routes match")
	} else if !t.quiet {
		t.Warn("Some routes mismatch")
	}
	return nil, nil
}

// auditPublics: Audit the list of routes currently on the route tables within the current VPC
func auditPublics(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRouteTables(): %w\n", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}

	publicRouteTables := make([]string, 0)
	for rtbid, rtb := range routeTables {
		t.Log("rtbId: %s", rtbid)
		for _, rt := range rtb.Routes {
			if strings.HasPrefix(rt.nextHop(), "igw-") {
				publicRouteTables = append(publicRouteTables, rtbid)
				break
			}
		}
	}

	for _, routeTableId := range publicRouteTables {
		notes := []string{}
		for _, route := range routeTables[routeTableId].Routes {
			t.Debug("Checking route %s -> %s [%s]\n", route.destination(), route.nextHop(), routeTableId)
			if !strings.HasPrefix(route.nextHop(), "igw-") {
				notes = append(notes, fmt.Sprintf("Non-igw route to %s via %s", route.destination(), route.nextHop()))
			}
		}
		if len(notes) == 0 {
			if !t.quiet {
				t.Log("route table %s GOOD", routeTableId)
			}
		} else {
			for _, note := range notes {
				t.Warn("%s %s", routeTableId, note)
			}
		}
	}
	return nil, nil
}

// auditSecurityGroups: Audit the security groups in the current VPC, to check for variance
func auditSecurityGroups(awscreds *credentials.Credentials, t *task) ([]string, error) {
	securityGroups, err := t.getSecurityGroups()
	if err != nil {
		return nil, fmt.Errorf("getSecurityGroups(): %w\n", err)
	}
	if len(securityGroups) == 0 {
		t.Error("No security groups found for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No security groups")
	}
	sgsTemplates, err := t.getConfiguredSecurityGroupTemplates()
	if err != nil {
		return nil, fmt.Errorf("getConfiguredSecurityGroupTemplates(): %w\n", err)
	}
	for _, sg := range t.VPC.State.SecurityGroups {
		t.Debug("SG: %#v", sg)
		t.Debug("Find matching SG for id %d", sg.TemplateID)
		matchingSG := func(id uint64) *database.SecurityGroupTemplate {
			for _, sgsTemplate := range sgsTemplates {
				for _, sgTemplate := range sgsTemplate.Groups {
					if sg.TemplateID == sgTemplate.ID {
						return sgTemplate
					}
				}
			}
			return nil
		}(sg.TemplateID)
		if matchingSG == nil {
			t.Warn("Security Group %s is not configured, but exists: should DELETE", sg.SecurityGroupID)
			continue
		}

		// We found a match, do we have a PL?
		usesPL := false
		for _, rule := range matchingSG.Rules {
			if strings.HasPrefix(rule.Source, "pl-") {
				usesPL = true
				break
			}
		}

		// Final flag for whether the security group would be modified
		securityGroupWouldBeModified := false

		// Walk the AWS state
		if awsSecurityGroup, ok := securityGroups[sg.SecurityGroupID]; ok {

			for _, awsRule := range awsSecurityGroup.Rules {
				// t.Debug("[%s] %#v", sg.SecurityGroupID, awsRule.SecurityGroupRule)
				// }
				// Find a match in the configs - look for rules that *would* be deleted
				// for _, stateRule := range sg.Rules {
				ruleFound := false
				for _, rule := range matchingSG.Rules {
					if diff := deep.Equal(awsRule.SecurityGroupRule, rule); diff == nil {
						ruleFound = true
						break
					}
				}
				if !ruleFound && !usesPL {
					t.Warn("[%s] Rule not in config, DELETE: %s", sg.SecurityGroupID, fmt.Sprintln(awsRule.SecurityGroupRule))
					securityGroupWouldBeModified = true
				}
			}
		} else {
			t.Error("[%s] State has record of non-existent SG", sg.SecurityGroupID)
		}

		// Find a match in the state - look for rules that *would* be added
		for _, rule := range matchingSG.Rules {
			ruleFound := false
			for _, stateRule := range sg.Rules {
				if diff := deep.Equal(stateRule, rule); diff == nil {
					ruleFound = true
					break
				}
			}
			if !ruleFound {
				t.Warn("[%s] Rule not in state, CREATE: %s", sg.SecurityGroupID, fmt.Sprintln(rule))
				securityGroupWouldBeModified = true
			}
		}

		if !securityGroupWouldBeModified {
			t.Log("SG %d (%s) matches with state, no changes", sg.TemplateID, sg.SecurityGroupID)
		}
	}
	return nil, nil
}

// showRouteInfo: Show existing routes on the route tables
func showRouteInfo(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRouteTables(): %w\n", err)
	}
	t.Debug(fmt.Sprintln(routeTables))
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	for routeTableId, table := range t.routeTables {
		t.Log("Working on route table %s", routeTableId)
		for _, route := range routeTables[routeTableId].Routes {
			t.Debug("Compare %s == %s?", route.nextHop(), t.config.ID)
			if route.nextHop() == t.config.ID {
				if len(table.Associations) > 0 && aws.BoolValue(table.Associations[0].Main) {
					t.Log("%s [MAIN] route %s -> %s (%s)\n", routeTableId, route.destination(), route.nextHop(), t.getRouteDescription(route.destination()))
				} else {
					t.Log("%s route %s -> %s (%s)\n", routeTableId, route.destination(), route.nextHop(), t.getRouteDescription(route.destination()))
				}
			}
		}
	}
	return nil, nil
}

// auditRouteTables: Check the existing route tables to make sure our work won't exceed the limit
func auditRouteTables(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.checkOrUpdatePLRoutes(nil, false)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func requestQuotaIncrease(awscreds *credentials.Credentials, t *task) ([]string, error) {
	tiers := []float64{65, 80, 100, 120, 150, 200, 300, 400, 500, 750}
	currentMax, err := t.getMaximumRouteTableSize()
	if err != nil {
		return nil, err
	}
	t.Debug("Create session in %s", string(t.VPC.Region))
	fmt.Println(awscreds) // TODO: Do you actually need to re-get the creds on the next line?
	awscreds, err = getCredsByAccountID(creds, t.VPC.AccountID)
	if err != nil {
		log.Fatalf("Error getting credentials for %s: %s", t.VPC.AccountID, err)
	}
	quotas := servicequotas.New(getSessionFromCreds(awscreds, string(t.VPC.Region)))
	fmt.Println(awscreds)
	for _, nextTier := range tiers {
		if float64(currentMax) < nextTier {
			if !t.dryRun {
				_, err := quotas.RequestServiceQuotaIncrease(&servicequotas.RequestServiceQuotaIncreaseInput{

					QuotaCode:    aws.String(routesPerRouteTableQuotaCode),
					ServiceCode:  aws.String("vpc"),
					DesiredValue: aws.Float64(nextTier),
				})
				if err != nil {
					return nil, err
				}
			}
			return []string{fmt.Sprintf("requested quota increase to %0.0f", nextTier)}, nil
		}
	}
	return nil, fmt.Errorf("unable to find appropriate tier to request increase")
}

func updateRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	templates, err := t.getConfiguredTGWTemplates()
	if err != nil {
		t.Error("fetching configured tgw templates: %s", err)
	}
	configuredTGWs := []string{}
	desiredPLs := []string{}

	rts, err := t.getRouteTables()
	if err != nil {
		return nil, err
	}
	// Create route phase
	for _, template := range templates {
		for _, route := range template.Routes {
			for _, templateSubnetType := range template.SubnetTypes {
				lt := strings.ToLower(string(templateSubnetType))
				if strings.HasPrefix(route, "pl-") && !stringInSlice(route, desiredPLs) {
					desiredPLs = append(desiredPLs, route)
				}
				for rtbid, rt := range rts {
					if strings.ToLower(rt.Type) != lt {
						continue
					}
					routeFound := false
					for _, r := range rt.Routes {
						if r.destination() == route {
							if r.nextHop() == template.TransitGatewayID {
								routeFound = true
							} else {
								t.Log("route %s found on %s, mispoint: %s", r.destination(), rtbid, r.nextHop())
							}
						}
					}
					if !routeFound {
						err := t.checkOrUpdateRoutes(aws.String(route), aws.String(template.TransitGatewayID), false, []string{rtbid})
						if err != nil {
							return nil, err
						}
					}
				}
			}
		}
		if !stringInSlice(template.TransitGatewayID, configuredTGWs) {
			configuredTGWs = append(configuredTGWs, template.TransitGatewayID)
		}
	}

	// Clean up route phase
	rts, err = t.getRouteTables()
	if err != nil {
		return nil, err
	}
	for _, pl := range desiredPLs {
		_, err := t.getPrefixListEntries(aws.String(pl))
		if err != nil {
			return nil, err
		}
	}
	for _, pl := range desiredPLs {
		plEntries, _ := t.getPrefixListEntries(aws.String(pl))
		for rtbid, rt := range rts {
			hasTemplatePrefixList := false
			// First check to ensure the route was added here
			for _, r := range rt.Routes {
				if r.destination() == pl && stringInSlice(r.nextHop(), configuredTGWs) {
					hasTemplatePrefixList = true
					break
				}
			}
			if hasTemplatePrefixList {
				t.Debug("%s has template pl, delete routes", rtbid)
				for _, r := range rt.Routes {
					if stringInSlice(r.nextHop(), configuredTGWs) {
						if stringInSlice(r.destination(), oldPrefixLists) || stringInSlice(r.destination(), plEntries) {
							if stringInSlice(r.destination(), desiredPLs) {
								t.Warn("template PL is on old PL list?")
								continue
							}
							err := t.deleteRoute(rtbid, r.destination())
							if err != nil {
								return nil, err
							}
							t.Log("Route deleted: %s -> %s on %s\n", r.destination(), r.nextHop(), rtbid)
						}
					}
				}
			}
		}
	}
	return nil, nil
}

// createCustomRoute: Create the route to the prefix list, pointing to the new TGW
func createCustomRoute(awscreds *credentials.Credentials, t *task) ([]string, error) {
	destination := t.args[0]
	err := t.checkOrUpdateRoutes(aws.String(destination), aws.String(t.config.ID), true, nil)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// removeCustomRoute: Remove the route pointing to the given cidr, but only if it's pointing at the current TGW
func removeCustomRoute(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("removeRoute(): %w", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("no routes")
	}
	deleteTarget := t.args[0]
	for routeTableId, table := range routeTables {
		routeDeletedFromTable := false
		for _, route := range table.Routes {
			destination := route.destination()
			if destination == deleteTarget && route.nextHop() == t.config.ID {
				err := t.deleteRoute(routeTableId, destination)
				if err != nil {
					t.Error("DeleteRoute(%s, %s): %s", routeTableId, destination, err)
				} else {
					routeDeletedFromTable = true
				}
			}
		}
		if !routeDeletedFromTable {
			t.Warn("Route for %s not found on %s", deleteTarget, routeTableId)
		}
	}
	return nil, nil
}

func (t *task) refreshVPCData(force bool) ([]string, error) {
	t.Debug("t.refreshVPCData(%t)", force)
	if t.VPC.VPC == nil {
		t.Warn("Attempting to refresh VPC data for an account level command")
		return nil, nil
	}
	vpcInfo := getVPC(t.VPC.ID)
	if vpcInfo == nil || force {
		vpc, err := t.api.FetchVPCDetails(t.VPC.AccountID, string(t.VPC.Region), t.VPC.ID)
		if err != nil {
			return nil, err
		}
		t.VPC.VPC = vpc
		if t.VPC.Stack == "" {
			t.Log("Unknown stack")
		} else {
			t.Log("Stack set to %s\n", t.VPC.Stack)
		}
		t.Debug("Fetch name")
		name, err := fetchVPCName(t.EC2, t.VPC.ID)
		if err != nil {
			t.Error("fetching name for vpc")
		}
		if name == "" {
			t.Log("Unknown name")
		} else if t.VPC.Name != name {
			t.VPC.Name = name
			t.Log("Name set to %s\n", t.VPC.Name)
		}
		t.Debug("Fetch route tables")
		t.VPC.RouteTables, err = t.getRouteTables()
		if err != nil {
			t.Error("fetching route tables\n")
			return nil, err
		}
		t.Debug("Get CIDRS")
		cidrs, err := t.getCIDRs()
		if err != nil {
			t.Error("Unable to fetch CIDRs: %s\n", err)
		}
		t.VPC.CIDRs = cidrs
		t.Debug("CIDRs fetched")
	} else {
		t.VPC = *vpcInfo
	}
	t.Debug("saveVPC()")
	t.saveVPC()
	t.Debug("success")
	return nil, nil
}

// enableDnsSupport: Enable DNS attributes within the VPC
func enableDnsSupport(awscreds *credentials.Credentials, t *task) ([]string, error) {
	if !t.dryRun {
		t.EC2.ModifyVpcAttribute(&ec2.ModifyVpcAttributeInput{
			VpcId:              aws.String(t.VPC.ID),
			EnableDnsHostnames: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
			EnableDnsSupport:   &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		})
	} else {
		t.Log("EC2.ModifyVpcAttribute()")
	}
	return nil, nil
}

// refreshVPCData: Force a read of the current state of the route tables on the selected VPCs
func refreshVPCData(awscreds *credentials.Credentials, t *task) ([]string, error) {
	return t.refreshVPCData(true)
}

// showVPCData: Dump a raw copy of the cached VPC information, if available, else fetch and show
func showVPCData(awscreds *credentials.Credentials, t *task) ([]string, error) {
	cachedVPC := getVPC(t.VPC.ID)
	if cachedVPC == nil {
		rt, err := t.getRouteTables()
		if err != nil {
			return nil, err
		}
		t.VPC.RouteTables = rt
	} else {
		t.VPC = *cachedVPC
	}
	info := map[string]interface{}{
		"AccountID":   t.VPC.AccountID,
		"ID":          t.VPC.ID,
		"Stack":       t.VPC.Stack,
		"Name":        t.VPC.Name,
		"CIDRs":       t.VPC.CIDRs,
		"RouteTables": t.VPC.RouteTables,
	}
	j, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Error("marshal error\n")
		return nil, err
	}
	t.Log("INFO: %s\n", string(j))
	return nil, nil
}

// findIP: Look through all the accounts to find an IP
func findIP(awscreds *credentials.Credentials, t *task) ([]string, error) {
	output := []string{}
	ips, err := t.getIPs()
	if err != nil {
		return nil, fmt.Errorf("getIPs(): %w\n", err)
	}
	if len(ips) == 0 {
		return nil, nil
	}
	if len(t.args) > 0 {
		for _, ip := range ips {
			if ip.IP != nil && stringInSlice(ip.IP.String(), t.args) {
				output = append(output, fmt.Sprintf("%s -> %s\n", ip.IP.String(), ip.PrivateIP.String()))
			}
		}
	} else {
		j, _ := json.MarshalIndent(ips, "", "   ")
		t.Log(string(j))
	}
	return output, nil
}

// backupRoutes: make a backup of the current route state
func backupRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	out, err := t.getRouteTables()
	if err != nil {
		t.Error("fetching route tables: %s\n", err)
		return nil, err
	}
	cache.set(cacheBackup, t.VPC.ID, out, durationInfinity)
	t.Log("route tables backed up")
	return nil, nil
}

// showBackupRoutes: show the current backup route state
func showBackupRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routes := map[string]*routeTable{}
	err := cache.get(cacheBackup, t.VPC.ID, &routes)
	if err != nil {
		t.Error("Unable to pull route table backup")
		return nil, err
	}
	j, err := json.MarshalIndent(routes, "", "  ")
	if err != nil {
		t.Error("marshal error\n")
		return nil, err
	}
	t.Log("Backup Data: %s\n", string(j))

	return nil, nil
}

// compareBackupToCurrentRouteTables: diff or apply the backup state of routes to AWS
func (t *task) compareBackupToCurrentRouteTables(backup, current map[string]*routeTable, fix bool) {
	stringMissing := "missing from"
	stringExtra := "on"
	if fix {
		stringMissing = "created on"
		stringExtra = "deleted from"
	}
	for rtbid := range current {
		if _, ok := backup[rtbid]; !ok {
			t.Log("New Route Table: %s (won't fix)\n", rtbid)
		}
	}
	for rtbid := range backup {
		if _, ok := current[rtbid]; !ok {
			t.Log("Missing Route Table: %s (won't fix)\n", rtbid)
		}
	}
	for rtbid, backupRouteTable := range backup {
		routeSliceCurrent := []string{}
		for _, route := range current[rtbid].Routes {
			routeSliceCurrent = append(routeSliceCurrent, route.destination())
		}

		for _, route := range backupRouteTable.Routes {
			if route.TGWID == nil {
				continue
			}
			// Route does not exist in current state, CREATE
			if !stringInSlice(route.destination(), routeSliceCurrent) {
				t.Log("Route %s -> %s %s %s\n", route.destination(), route.nextHop(), stringMissing, rtbid)
				if fix && route.nextHop() != "" {
					t.createNewRouteToTGW(rtbid, route.destination(), route.nextHop())
					// t.createNewRouteToTGW(rtbid, route.destination(), route.nextHop())
				}
			} else {
				// Route exists in current state, make sure it points to the right target
				for _, c := range current[rtbid].Routes {
					if route.destination() == c.destination() && route.nextHop() != c.nextHop() {
						t.Log("Route %s mispointing at %s, want %s\n", c.destination(), c.nextHop(), route.nextHop())
						if fix && route.TGWID != nil {
							time.Sleep(5 * time.Second)
							t.deleteRoute(rtbid, route.destination())
							// if err := t.createNewRouteToTGW(rtbid, route.destination(), *route.TGWID); err != nil {
							// 	t.Warn("unable to create route %s -> %s (%s)\n", route.destination(), *route.TGWID, rtbid)
							// } else {
							// 	t.Log("route created %s -> %s (%s)\n", route.destination(), *route.TGWID, rtbid)
							// }
						}
					}
				}
			}
		}
	}
	if fix {
		// Sleep for a little to make sure the routes settle
		time.Sleep(10 * time.Second)
	}
	for rtbid, backupRouteTable := range backup {
		routeSliceBackup := []string{}
		for _, route := range backupRouteTable.Routes {
			routeSliceBackup = append(routeSliceBackup, route.destination())
		}
		for _, route := range current[rtbid].Routes {
			if route.TGWID == nil {
				continue
			}
			// Route exists in current state, but is not in backup, DELETE
			if !stringInSlice(route.destination(), routeSliceBackup) {
				t.Log("Extra route for %s -> %s %s %s\n", route.destination(), route.nextHop(), stringExtra, rtbid)
				if fix {
					t.deleteRoute(rtbid, route.destination())
				}
			}
		}
	}
}

func showBackupDiff(awscreds *credentials.Credentials, t *task) ([]string, error) {
	currentRouteTables, err := t.getRouteTables()
	if err != nil {
		t.Error("fetching route tables: %s\n", err)
		return nil, err
	}
	backupRouteTables := map[string]*routeTable{}
	err = cache.get(cacheBackup, t.VPC.ID, &backupRouteTables)
	if err != nil {
		t.Error("Unable to pull route table backup")
		return nil, err
	}
	t.compareBackupToCurrentRouteTables(backupRouteTables, currentRouteTables, false)
	return nil, nil
}

func restoreRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	currentRouteTables, err := t.getRouteTables()
	if err != nil {
		t.Error("fetching route tables: %s\n", err)
		return nil, err
	}
	backupRouteTables := map[string]*routeTable{}
	err = cache.get(cacheBackup, t.VPC.ID, &backupRouteTables)
	if err != nil {
		t.Error("Unable to pull route table backup, refresh")
		return nil, err
	}
	t.compareBackupToCurrentRouteTables(backupRouteTables, currentRouteTables, true)
	return nil, nil
}

func listVPCs(awscreds *credentials.Credentials, t *task) ([]string, error) {
	if len(t.VPC.CIDRs) == 0 {
		cidrs, err := getCIDRsForVPC(t.VPC.AccountID, t.VPC.ID, t.VPC.Region)
		if err != nil {
			t.Warn("Unable to fetch CIDRs: %s\n", err)
		} else {
			t.VPC.CIDRs = cidrs
			t.saveVPC()
		}
	}
	info := map[string]interface{}{
		"AccountID": t.VPC.AccountID,
		"Stack":     t.VPC.Stack,
		"Name":      t.VPC.Name,
		"CIDRs":     t.VPC.CIDRs,
		"VPCType":   t.VPC.Type,
	}
	j, err := json.Marshal(info)
	if err != nil {
		t.Error("marshal error\n")
		return nil, err
	}
	t.Log("%s\n", string(j))
	return nil, nil
}

func countVPCs(awscreds *credentials.Credentials, t *task) ([]string, error) {
	vpcs, err := t.countVPCs()
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("VPCCOUNT %d", vpcs)}, nil
}

func showEgressIPs(awscreds *credentials.Credentials, t *task) ([]string, error) {
	ngwout, err := t.EC2.DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
		Filter: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{t.VPC.ID}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	egressIPs := make([]string, 0)
	for _, ngw := range ngwout.NatGateways {
		for _, a := range ngw.NatGatewayAddresses {
			egressIPs = append(egressIPs, aws.StringValue(a.PublicIp))
		}
	}
	return []string{fmt.Sprintf("Egress IPs (%d): %s", len(egressIPs), strings.Join(egressIPs, ", "))}, nil
}

func showIPUsage(awscreds *credentials.Credentials, t *task) ([]string, error) {
	vpcs, err := t.getVPCs()
	if err != nil {
		return nil, err
	}

	vpcIds := make([]string, 0)
	for _, v := range vpcs {
		vpcIds = append(vpcIds, aws.StringValue(v.VpcId))
	}

	so, err := t.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice(vpcIds),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	subnets := make([]*ec2.Subnet, 0)
	for _, s := range so.Subnets {
		within, err := cidrIsWithin(aws.StringValue(s.CidrBlock), "10.0.0.0/8")
		if err != nil {
			return nil, err
		}
		if within {
			subnets = append(subnets, s)
		}
	}
	usedIps := 0
	for _, s := range subnets {
		t.Debug("Working in subnet %s in %s", aws.StringValue(s.SubnetId), aws.StringValue(s.VpcId))
		totalIps, err := subnetSize(aws.StringValue(s.CidrBlock))
		if err != nil {
			return nil, err
		}
		t.Debug("Subnet size: %d ips", totalIps)
		usedIps += (totalIps - int(aws.Int64Value(s.AvailableIpAddressCount)) - 5)
	}
	return []string{fmt.Sprintf("COUNTS: %d %d", len(subnets), usedIps)}, nil
}

func (t *task) getRawSubnets() ([]*ec2.Subnet, error) {
	output, err := t.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{t.VPC.ID}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return output.Subnets, nil
}

func getSubnetInfo(e ec2iface.EC2API, vpcId, subnetId string) (string, string, error) {
	type SubnetInfo struct {
		SubnetType string
		SubnetCidr string
	}
	var subnetInfo *SubnetInfo
	cacheKeyName := fmt.Sprintf("%s/%s/subnetInfo", vpcId, subnetId)
	if err := cache.get(cacheVPCData, cacheKeyName, subnetInfo); err == nil {
		if subnetInfo != nil {
			return subnetInfo.SubnetType, subnetInfo.SubnetCidr, nil
		}
	}
	subnetInfo = &SubnetInfo{}
	out, err := e.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: aws.StringSlice([]string{subnetId}),
			},
		},
	})
	if err != nil {
		return subnetInfo.SubnetCidr, subnetInfo.SubnetType, err
	}
	if len(out.Subnets) != 1 {
		return subnetInfo.SubnetCidr, subnetInfo.SubnetType, fmt.Errorf("Invalid number of subnet%s: %d", pluralS(len(out.Subnets)), len(out.Subnets))
	}
	for _, subnet := range out.Subnets {
		for _, tag := range subnet.Tags {
			for _, name := range layerTags {
				if strings.ToLower(aws.StringValue(tag.Key)) == name {
					subnetInfo.SubnetType = aws.StringValue(tag.Value)
					cache.set(cacheVPCData, cacheKeyName, subnetInfo.SubnetType, time.Minute*60)
					subnetInfo.SubnetCidr = aws.StringValue(subnet.CidrBlock)
					return subnetInfo.SubnetCidr, subnetInfo.SubnetType, nil
				}
			}
		}
	}
	return subnetInfo.SubnetCidr, subnetTypeUnknown, nil
}

func (t *task) getSubnetsByUses(uses ...string) ([]string, error) {
	subnets, err := t.getRawSubnets()
	if err != nil {
		return nil, err
	}
	validSubnets := make([]string, 0)
	for _, subnet := range subnets {
		t.Debug("Check subnet %s for tags [%s]", aws.StringValue(subnet.SubnetId), strings.Join(uses, ","))
		// Check if the use is what we want
		for _, tag := range subnet.Tags {
			t.Debug("Found tag %s = %s", aws.StringValue(tag.Key), aws.StringValue(tag.Value))
			if strings.ToLower(aws.StringValue(tag.Key)) == "use" && stringInSlice(aws.StringValue(tag.Value), uses) {
				validSubnets = append(validSubnets, aws.StringValue(subnet.SubnetId))
			}
		}
	}
	return validSubnets, nil
}

func (t *task) getLiveVPCList() ([]*VPCInfo, error) {
	t.Debug("Get Live VPC list")
	output, err := t.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}

	vpcs := make([]*VPCInfo, 0)
	for _, ec2vpc := range output.Vpcs {
		vpc := &database.VPC{
			AccountID: aws.StringValue(t.AccountID),
			ID:        aws.StringValue(ec2vpc.VpcId),
			Region:    currentRegion,
		}
		for _, tag := range ec2vpc.Tags {
			if strings.ToLower(aws.StringValue(tag.Key)) == "name" {
				vpc.Name = aws.StringValue(tag.Value)
				break
			}
		}
		info := &VPCInfo{
			VPC:       vpc,
			IsDefault: aws.BoolValue(ec2vpc.IsDefault),
		}
		vpcs = append(vpcs, info)
	}
	return vpcs, nil
}

func generateComment(awscreds *credentials.Credentials, t *task) ([]string, error) {
	if len(t.VPC.CIDRs) == 0 {
		cidrs, err := t.getCIDRs()
		if err != nil {
			t.Error("Unable to fetch CIDRs: %s\n", err)
		}
		t.VPC.CIDRs = cidrs
		t.saveVPC()
	}
	maskedCidrs := t.maskCIDRs()
	t.Log("Comment:\nVPC name: %s\nVPC id: %s\nCIDRs: %s", t.VPC.Name, t.VPC.ID, strings.Join(maskedCidrs, ", "))
	return nil, nil
}

func auditAccounts(awscreds *credentials.Credentials, t *task) ([]string, error) {
	t.Debug("Auditing account ID %s", aws.StringValue(t.AccountID))
	notes := make([]string, 0)
	// Get the list of real, existing VPCs from the account via AWS API
	liveVpcs, err := t.getLiveVPCList()
	if err != nil {
		return nil, err
	}
	if len(liveVpcs) == 0 {
		t.Debug("No VPCs")
		return nil, nil
	}
	accountInfo, err := t.api.FetchAutomatedVPCsByAccount(aws.StringValue(t.AccountID))
	if err != nil {
		return nil, err
	}
	automatedVpcs := make(map[string]*database.VPC)
	for _, vpc := range accountInfo.VPCs {
		automatedVpcs[vpc.ID] = vpc
	}
	nonAutomated := 0
	for _, vpc := range liveVpcs {
		t.Debug("Verifying VPC %s is automated", vpc.ID)
		if _, ok := automatedVpcs[vpc.ID]; !ok {
			displayName := func() string {
				if vpc.IsDefault {
					return fmt.Sprintf("%s:default!", vpc.Name)
				}
				return vpc.Name
			}()
			t.Warn("Non-automated VPC %s (%s) [%s]", vpc.ID, displayName, accountInfo.Name)
			nonAutomated = nonAutomated + 1
			continue
		}
	}
	if nonAutomated > 0 {
		notes = append(notes, fmt.Sprintf("%d VPC%s non-automated", nonAutomated, pluralS(nonAutomated)))
		return notes, nil
	}
	return nil, nil
}

const (
	ecsClusterName     = "debug-test-network-connection"
	serverTaskName     = "debug-test-network-connection-server"
	clientTaskName     = "debug-test-network-connection-client"
	ecsTaskRoleName    = "debug-test-network-connection-util"
	dynamodbTableName  = ecsClusterName
	dynamodbPrimaryKey = "task-arn"
	lambdaFunctionName = "debug-test-network-connection"
)

func cleanupTestInfrastructure(awscreds *credentials.Credentials, t *task) ([]string, error) {
	getDynamoDBCredentials := func(accountID string, region database.Region) *dynamodb.DynamoDB {
		awscreds, err := getCredsByAccountID(creds, accountID)
		if err != nil {
			log.Fatalf("Error getting credentials for %s: %s", accountID, err)
		}
		return dynamodb.New(getSessionFromCreds(awscreds, string(region)))
	}

	getIAMCredentials := func(accountID string, region database.Region) *iam.IAM {
		awscreds, err := getCredsByAccountID(creds, accountID)
		if err != nil {
			log.Fatalf("Error getting credentials for %s: %s", accountID, err)
		}
		return iam.New(getSessionFromCreds(awscreds, string(region)))
	}

	getECSCredentials := func(accountID string, region database.Region) *ecs.ECS {
		awscreds, err := getCredsByAccountID(creds, accountID)
		if err != nil {
			log.Fatalf("Error getting credentials for %s: %s", accountID, err)
		}
		return ecs.New(getSessionFromCreds(awscreds, string(region)))
	}

	dynamodbToDelete := []string{dynamodbTableName}
	policiesToDelete := [][2]string{{clientTaskName, ecsTaskRoleName}}
	rolesToDelete := []string{ecsTaskRoleName}
	clustersToDelete := []string{ecsClusterName}
	ddbClient := getDynamoDBCredentials(t.VPC.AccountID, t.VPC.Region)
	iamClient := getIAMCredentials(t.VPC.AccountID, t.VPC.Region)
	ecsClient := getECSCredentials(t.VPC.AccountID, t.VPC.Region)

	for _, d := range dynamodbToDelete {
		if !t.dryRun {
			ddbClient.DeleteTable(&dynamodb.DeleteTableInput{
				TableName: aws.String(d),
			})
		} else {
			t.Log("Delete dynamodb table %s", d)
		}
	}
	for _, p := range policiesToDelete {
		if !t.dryRun {
			iamClient.DeleteRolePolicy(&iam.DeleteRolePolicyInput{
				PolicyName: aws.String(p[0]),
				RoleName:   aws.String(p[1]),
			})
		} else {
			t.Log("Delete Role Policy %s/%s", p[0], p[1])
		}
	}
	for _, r := range rolesToDelete {
		if !t.dryRun {
			iamClient.DeleteRole(&iam.DeleteRoleInput{
				RoleName: aws.String(r),
			})
		} else {
			t.Log("Delete role %s", r)
		}
	}
	for _, c := range clustersToDelete {
		if !t.dryRun {
			ecsClient.DeleteCluster(&ecs.DeleteClusterInput{
				Cluster: aws.String(c),
			})
		} else {
			t.Log("Delete cluster %s", c)
		}
	}
	return nil, nil
}

// testSharedServiceFromVPC: Run a connection test from within the target VPC to a shared service
// TODO: add selector to allow testing of various shared services
func testSharedServiceFromVPC(awscreds *credentials.Credentials, t *task) ([]string, error) {
	var endpoint *testEndpointDefinition
	for name, endpointInfo := range sharedServiceTestEndpoints {
		if name == t.testEndpoint {
			endpoint = &endpointInfo
		}
	}
	if endpoint == nil {
		return failTask("Unknown test endpoint: %s\n", t.testEndpoint)
	}
	// Do a DNS lookup
	if endpoint.Hostname != nil {
		addrs, err := net.LookupHost(*endpoint.Hostname)
		if err != nil {
			return failTask("Unable to lookup hostname %s: %s\n", *endpoint.Hostname, err)
		}
		if len(addrs) > 0 {
			endpoint.IP = addrs[0]
		} else {
			return failTask("No addresses returned for %s\n", *endpoint.Hostname)
		}
	}
	// Check for tenancy
	output, err := t.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []*string{
			aws.String(t.VPC.ID),
		},
	})
	if err != nil {
		return failTask("Describing VPC")
	}
	for _, v := range output.Vpcs {
		if aws.StringValue(v.InstanceTenancy) == ec2.HostTenancyDedicated {
			t.Warn("Tenancy is dedicated, unable to run test")
			return failTask("dedicated tenancy")
		}
	}
	// Find an appropriate subnet
	tables, err := t.getRouteTables()
	if err != nil {
		return failTask("Fetching route tables\n")
	}
	sourceSubnetPrecedence := map[int]string{
		1: "private",
		2: "public",
	}
	chosenSubnet := ""
	for _, subnetType := range sourceSubnetPrecedence {
		for _, rtb := range tables {
			if rtb.Type == subnetType && len(rtb.Subnets) > 0 {
				chosenSubnet = rtb.Subnets[0]
				break
			}
		}
		if chosenSubnet != "" {
			break
		}
	}
	if chosenSubnet == "" {
		return failTask("Unable to find appropriate subnet")
	}
	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute) // DERP
	defer cancel()
	test := &connection.NetworkConnectionTest{
		Context:     ctx,
		Logger:      t,
		Credentials: creds,
	}
	spec := &connection.ConnectionSpec{
		Source: &connection.Endpoint{
			NetworkInterfaceSpec: &connection.NetworkInterfaceSpec{
				AccountID:                 t.VPC.AccountID,
				Region:                    string(t.VPC.Region),
				SubnetID:                  chosenSubnet,
				CreateEgressSecurityGroup: true,
			},
		},
		Destination: &connection.Endpoint{
			IPAddress: &connection.IPAddress{
				IP:     net.ParseIP(endpoint.IP),
				IPType: connection.IPTypeSharedService,
			},
		},
		Port:        int64(endpoint.Port),
		PerformTest: true,
	}
	finalOutput := make([]string, 0)
	verifyErr, rollbackErr := test.Verify(spec)
	if rollbackErr != nil {
		s := fmt.Sprintf("FAILED TO ROLL BACK THE FOLLOWING RESOURCES:\n  %s", strings.Join(rollbackErr.(*connection.RollbackError).ResourcesNotRolledBack, "\n  "))
		t.Error(s)
		finalOutput = append(finalOutput, s)
	}
	if verifyErr != nil {
		return failTask("Test failed: %s", verifyErr)
	}
	return append(finalOutput, "Test succeeded"), nil
}

func getVPC(vpcID string) *VPCInfo {
	cachedVPC := &VPCInfo{}
	if err := cache.get(cacheVPC, vpcID, &cachedVPC); err == nil {
		if cachedVPC.Config == nil {
			cachedVPC.Config = &database.VPCConfig{}
		}
		stateMap := map[string]interface{}{}
		cache.get(cacheVPCData, fmt.Sprintf("%s/mtga", vpcID), &cachedVPC.Config.ManagedTransitGatewayAttachmentIDs)
		cache.get(cacheVPCData, fmt.Sprintf("%s/sgs", vpcID), &cachedVPC.Config.SecurityGroupSetIDs)
		cache.get(cacheVPCData, fmt.Sprintf("%s/state", vpcID), &stateMap)
		// fmt.Printf("%#v\n", stateMap)
		err := rehydrateObject(stateMap, cachedVPC.State)
		if err != nil {
			fmt.Println(err)
		}
		return cachedVPC
	}
	return nil
}

func (t *task) getConfiguredTGWTemplates() (map[uint64]*database.ManagedTransitGatewayAttachment, error) {
	validTGWs := make(map[uint64]*database.ManagedTransitGatewayAttachment)
	if t.VPC.Config == nil {
		return nil, fmt.Errorf("nil VPC Config")
	}
	if t.VPC.Config.ManagedTransitGatewayAttachmentIDs == nil {
		return nil, fmt.Errorf("nil MTGA ids")
	}
	for _, mtgaID := range t.VPC.Config.ManagedTransitGatewayAttachmentIDs {
		if _, ok := t.tgwTemplates[mtgaID]; !ok {
			t.Warn("configured template ID %d does not exist", mtgaID)
		} else {
			if !t.quiet {
				t.Log("Found managed TGW attachment: %s (%d)", t.tgwTemplates[mtgaID].Name, mtgaID)
			}
			validTGWs[mtgaID] = t.tgwTemplates[mtgaID]
		}
	}
	return validTGWs, nil
}

func (t *task) getConfiguredSecurityGroupTemplates() (map[uint64]*database.SecurityGroupSet, error) {
	validSGs := make(map[uint64]*database.SecurityGroupSet)
	if t.VPC.Config == nil {
		return nil, fmt.Errorf("nil VPC Config")
	}
	if t.VPC.Config.SecurityGroupSetIDs == nil {
		return nil, fmt.Errorf("nil SGS ids")
	}
	for _, sgsID := range t.VPC.Config.SecurityGroupSetIDs {
		if _, ok := t.sgsTemplates[sgsID]; !ok {
			t.Warn("configured template ID %d does not exist", sgsID)
		} else {
			if !t.quiet {
				t.Log("Found managed SG set: %s (%d)", t.sgsTemplates[sgsID].Name, sgsID)
			}
			validSGs[sgsID] = t.sgsTemplates[sgsID]
		}
	}
	return validSGs, nil
}

func (t *task) getRequestedRoles(account string) error {
	workingRegion := func() database.Region {
		if t.APIRegion != nil {
			return database.Region(aws.StringValue(t.APIRegion))
		}
		return database.Region("us-west-2")
	}()
	var err error
	t.EC2, t.NetworkFirewall, err = getRWAccountCredentials(account, workingRegion)
	if err != nil {
		return fmt.Errorf("Unable to fetch credentials: %s", err)
	}
	return nil
}

func worker(workerID int, wg *sync.WaitGroup) {
	defer func() {
		log.Printf("Worker %d exiting\n", workerID)
		wg.Done()
	}()
	for job := range jobs {
		func(j Job) {
			var err error
			var output []string
			awscreds := &credentials.Credentials{}
			taskWithLogging := j.task
			acctID := func() string {
				if j.task.AccountID != nil {
					return *j.task.AccountID
				}
				if j.task.VPC.VPC != nil {
					return j.task.VPC.AccountID
				}
				return ""
			}()
			select {
			case <-j.task.ctx.Done():
				taskWithLogging.Warn("Got job for account %s, but exiting...", acctID)
				return
			default:
			}
			taskWithLogging.Debug("Got job for account %s", acctID)
			LockAccount(acctID)
			defer UnlockAccount(acctID)
			if !stringInSlice(j.command, offlineCommands) {
				err = taskWithLogging.getRequestedRoles(acctID)
				if err != nil {
					results <- JobResult{job: j, err: err, output: nil}
					return
				}
			}
			// Online command
			if !stringInSlice(j.command, offlineCommands) {
				output, err = commands[j.command](awscreds, taskWithLogging)
			} else {
				output, err = commands[j.command](nil, taskWithLogging)
			}
			if err != nil {
				log.Printf("Job failed: %s\n", err)
				results <- JobResult{job: j, err: err, output: output}
			} else {
				results <- JobResult{job: j, err: nil, output: output}
			}
		}(job)
	}
}

func createWorkerPool(max int) {
	var wg sync.WaitGroup
	if max > 1 {
		log.Printf("Creating %d workers...\n", max)
	}
	for i := 1; i <= max; i++ {
		wg.Add(1)
		go worker(i, &wg)
	}
	wg.Wait()
	close(results)
}

func listCommands() {
	fmt.Println("USAGE: vroom <command> [flags]")
	fmt.Printf("Commands: %s\n", strings.Join(commandNames, ", "))
	fmt.Printf("Audit Sub-commands: %s\n", strings.Join(auditCommandNames, ", "))
}

func captureVROOMConfig() *VROOMConfig {
	var err error
	config := &VROOMConfig{}
	config.UserName = os.Getenv("VROOM_USERNAME")
	config.Password = os.Getenv("VROOM_PASSWORD")
	config.CloudTamerBaseURL = os.Getenv("CLOUDTAMER_BASE_URL")
	config.VPCConfBaseURL = os.Getenv("VPCCONF_BASE_URL")
	cloudTamerIDMSIDStr := os.Getenv("CLOUDTAMER_IDMS_ID")
	cloudTamerAdminGroupIDStr := os.Getenv("CLOUDTAMER_ADMIN_GROUP_ID")
	if config.UserName == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "VROOM_USERNAME env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if config.Password == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "VROOM_PASSWORD env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if config.CloudTamerBaseURL == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "CLOUDTAMER_BASE_URL env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if config.VPCConfBaseURL == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "VPCCONF_BASE_URL env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if cloudTamerAdminGroupIDStr == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "CLOUDTAMER_ADMIN_GROUP_ID env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if cloudTamerIDMSIDStr == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "CLOUDTAMER_IDMS_ID env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	config.CloudTamerAdminGroupID, err = strconv.Atoi(cloudTamerAdminGroupIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", "Invalid CLOUDTAMER_ADMIN_GROUP_ID")
		os.Exit(exitCodeInvalidEnvironment)
	}
	config.CloudTamerIDMSID, err = strconv.Atoi(cloudTamerIDMSIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", "Invalid CLOUDTAMER_IDMS_ID")
		os.Exit(exitCodeInvalidEnvironment)
	}
	return config
}

func loginToCloudTamer(config *VROOMConfig) {
	tokenProvider := &cloudtamer.TokenProvider{
		Config: cloudtamer.CloudTamerConfig{
			BaseURL:      config.CloudTamerBaseURL,
			IDMSID:       config.CloudTamerIDMSID,
			AdminGroupID: config.CloudTamerAdminGroupID,
		},
		Username: config.UserName,
		Password: config.Password,
	}
	token, err := tokenProvider.GetToken()
	if err != nil {
		log.Fatalf("Error getting CloudTamer token: %s", err)
	}
	creds.Token = token
	creds.BaseURL = config.CloudTamerBaseURL
}

const DECOMISSIONED_OU = 323

type ouData struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    int    `json:"parent_ou_id"`
}

type cloudTamerOUsResponse struct {
	Data []*ouData `json:"data"`
}

type ouHierarchy struct {
	ouData map[int]*ouData
}

func ouCrumbs(id int, ous map[int]*ouData) []int {
	parentOf := func(id int) *int {
		if ou, ok := ous[id]; ok && ou.ParentID != id {
			return &ou.ParentID
		} else {
			return nil
		}
	}
	lineage := []int{}
	for current, parent := id, parentOf(id); parent != nil; parent = parentOf(current) {
		current = *parent
		lineage = append(lineage, *parent)
	}
	return lineage
}

func (h *ouHierarchy) ProjectIsInOU(projectID, ouID int) bool {
	return intInSlice(ouID, ouCrumbs(projectID, h.ouData))
}

func getOUTree() (*ouHierarchy, error) {
	ous := make(map[int]*ouData)
	req, err := http.NewRequest("GET", creds.BaseURL+"/v3/ou", nil)
	if err != nil {
		return nil, fmt.Errorf("Error contacting CloudTamer: %s", err)
	}
	ctou := cloudTamerOUsResponse{}
	err = (&cloudtamer.HTTPClient{Token: creds.Token}).Do(req, &ctou)
	if err != nil {
		return nil, fmt.Errorf("Error getting OU tree from CloudTamer: %s", err)
	}
	for _, oudata := range ctou.Data {
		ous[oudata.ID] = oudata
	}
	return &ouHierarchy{
		ouData: ous,
	}, nil
}

func queue(command string, task *task) {
	jobs <- Job{
		command: command,
		task:    task,
	}
}

func monitorQueues() {
	lastStatus := time.Now()
	for {
		if time.Since(lastStatus) > 10*time.Second {
			log.Printf("jobs: %d/%d, results: %d/%d\n", len(jobs), cap(jobs), len(results), cap(results))
			lastStatus = time.Now()
		}
		time.Sleep(time.Second)
	}
}

func main() {
	log.Println("Starting up the VPC Resource & Object Operations Manager...")
	accountLocks = make(map[string]*sync.Mutex)
	cacheLocks = make(map[string]*sync.Mutex)
	flag.Usage = listCommands
	dryRun := flag.Bool("n", false, "do not write to resources")
	limitVPC := flag.String("vpc", "", "limit to specific vpc")
	limitStack := flag.String("stack", "", "limit to specific stack")
	region := flag.String("region", regionEast, "specify region to work in")
	limitAccount := flag.String("account", "", "limit to specific account ID")
	includeShared := flag.Bool("shared", false, "include shared stack vpcs")
	limitCIDR := flag.String("cidr", "", "limit to a specific cidr block")
	maxConcurrency := flag.Int("threads", 1, "run more threads")
	operateByTGWAttachments := flag.Bool("tgw", false, "get list of VPCs by TGW attachments")
	quietFlag := flag.Bool("quiet", false, "suppress some extra info")
	verboseFlag := flag.Bool("v", false, "be verbose with output")
	testEndpoint := flag.String("endpoint", defaultTestEndpoint, "endpoint to test")
	usePrivilegedRole := flag.Bool("rw", false, "use privileged roles")
	flag.Parse()

	globalAccountLock = &sync.Mutex{}
	globalCacheLock = &sync.Mutex{}
	currentRegion = database.Region(*region)
	log.Println(flag.Args())
	if flag.NArg() < minArgs {
		flag.Usage()
		os.Exit(exitCodeUsage)
	}
	args := flag.Args()[1:]
	command := flag.Arg(0)
	if currentRegion == regionAll && !stringInSlice(command, accountCommands) {
		log.Printf("Unable to run in all regions unless running one of the following commands: %s\n", accountCommands)
		flag.Usage()
		os.Exit(exitCodeUsage)
	}
	for k := range commands {
		commandNames = append(commandNames, k)
	}
	if !stringInSlice(command, commandNames) {
		log.Printf("Invalid command: %s", command)
		flag.Usage()
		os.Exit(exitCodeUsage)
	}
	if command == "audit" {
		for k := range auditCommands {
			auditCommandNames = append(auditCommandNames, k)
		}
		if len(args) == 0 {
			log.Printf("Please provide an audit sub-command")
			flag.Usage()
			os.Exit(exitCodeUsage)
		}
		if !stringInSlice(strings.ToLower(args[0]), auditCommandNames) {
			log.Printf("Invalid audit sub-command: %s", args[0])
			flag.Usage()
			os.Exit(exitCodeUsage)
		}
	}
	if *dryRun {
		log.Println("DRY RUN ENABLED")
	}
	cache = initRedis(redisHost)

	vroomConfig := captureVROOMConfig()
	loginToCloudTamer(vroomConfig)

	ctx, cancel := context.WithCancel(context.Background())
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // DERP
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		for s := range c {
			sig, ok := s.(syscall.Signal)
			if !ok {
				log.Printf("Non-UNIX signal %s", s)
				continue
			}
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				log.Printf("Shutting down VROOM")
				cancel()
			}
		}
	}()
	defer cancel()

	api := &VPCConfAPI{
		Username:      vroomConfig.UserName,
		Password:      vroomConfig.Password,
		BaseURL:       vroomConfig.VPCConfBaseURL,
		verbose:       *verboseFlag,
		AutomatedVPCs: make(map[database.Region][]*database.VPC),
		Lock:          sync.Mutex{},
	}

	// Create worker pool
	jobs = make(chan Job, *maxConcurrency)
	results = make(chan JobResult, *maxConcurrency)

	go monitorQueues()

	processedList := []string{}
	processedByStack := make(map[string][]string)
	jobData := make(map[string]JobResult)

	exceptionVPCList := make([]string, 0)
	tgwTemplates := make(map[uint64]*database.ManagedTransitGatewayAttachment)
	sgsTemplates := make(map[uint64]*database.SecurityGroupSet)
	var err error

	if *limitVPC == "" {
		exceptionVPCList, err = api.FetchExceptionalVPCs()
		if err != nil {
			log.Printf("Error fetching exception list: %s\n", err)
		}
	}

	if stringInSlice(command, tgwRelatedCommands) {
		tgwTemplates, err = api.FetchManagedTGWTemplates()
		if err != nil {
			log.Printf("Error fetching managed TGW template list: %s\n", err)
		}
	}

	if stringInSlice(command, sgsRelatedCommands) {
		sgsTemplates, err = api.FetchManagedSecurityGroupTemplates()
		if err != nil {
			log.Printf("Error fetching managed TGW template list: %s\n", err)
		}
	}

	queueVPC := func(vpcinfo *VPCInfo) {
		queue(command, &task{
			VPC:               *vpcinfo,
			APIRegion:         aws.String(string(vpcinfo.Region)),
			dryRun:            *dryRun,
			testEndpoint:      *testEndpoint,
			config:            centralInfo[versionGreenfield][vpcinfo.Region],
			api:               api,
			ctx:               ctx,
			exceptionVPCList:  exceptionVPCList,
			tgwTemplates:      tgwTemplates,
			sgsTemplates:      sgsTemplates,
			shared:            *includeShared,
			version:           versionLegacy,
			quiet:             *quietFlag,
			verbose:           *verboseFlag,
			routeTables:       map[string]*ec2.RouteTable{},
			securityGroups:    map[string]*ec2.SecurityGroup{},
			ips:               map[string]*ip{},
			filterCIDR:        *limitCIDR,
			limitVPC:          *limitVPC,
			usePrivilegedRole: *usePrivilegedRole,
			args:              args,
		})
	}

	// Highly targeted mode: single vpc
	if *limitVPC != "" {
		targetInfo := getVPC(*limitVPC)
		if targetInfo == nil {
			vpc, err := api.FindVPC(*limitVPC)
			if err != nil {
				log.Printf("Error searching for VPC: %s", err)
				os.Exit(exitCodeFatal)
			}
			if vpc == nil {
				log.Printf("Unable to find VPC %s", *limitVPC)
				os.Exit(exitCodeFatal)
			}
			targetInfo = &VPCInfo{
				VPC: vpc,
			}
			saveVPC(vpc.ID, *targetInfo)
		}
		if targetInfo.AccountID == "" {
			if *limitAccount == "" {
				log.Printf("Unable to determine account ID for VPC %s", *limitVPC)
				os.Exit(exitCodeFatal)
			} else {
				targetInfo.AccountID = *limitAccount
			}
		}
		queueVPC(targetInfo)
		processedByStack[targetInfo.Stack] = []string{targetInfo.ID}
		processedList = append(processedList, targetInfo.ID)
		close(jobs)
	} else {
		// Iterate across accounts (to walk all infrastructure)
		if stringInSlice(command, accountCommands) {
			go func() {
				log.Println("Getting list of authorized accounts...")
				accounts, err := creds.GetAuthorizedAccounts()
				if err != nil {
					log.Fatalf("Error fetching authorized accounts: %s\n", err)
				}
				log.Println("Getting the OU tree from cloudtamer...")
				OUHierarchy, err := getOUTree()
				if err != nil {
					log.Fatalf("Error fetching OU tree from CloudTamer: %s\n", err)
				}
				var accountDetails map[string]*database.AWSAccount
				maxTryTime := time.Second * 45
				for startedTrying := time.Now(); time.Since(startedTrying) < maxTryTime; time.Sleep(5 * time.Second) {
					accountDetails, err = api.FetchAllAccountDetails()
					if err != nil {
						log.Printf("error fetching account details: %s\n", err)
						continue
					}
					break
				}
				if accountDetails == nil {
					log.Println("unable to fetch account details")
					return
				}
				for _, account := range accounts {
					if *limitAccount != "" && (*limitAccount != account.ID) {
						continue
					}
					if OUHierarchy.ProjectIsInOU(account.ProjectID, DECOMISSIONED_OU) {
						log.Printf("Skipping disabled account %s (%s)\n", account.ID, account.Name)
						continue
					}
					for _, r := range regions {
						if _, ok := accountDetails[account.ID]; !ok {
							log.Printf("Unknown account (not in vpc-conf): %s\n", account.ID)
							continue
						}
						if (currentRegion != regionAll && r == string(currentRegion)) || currentRegion == regionAll {
							if accountDetails[account.ID].IsGovCloud {
								if r != regionGovWest {
									continue
								}
							} else {
								if !stringInSlice(r, commercialRegions) {
									continue
								}
							}
							queue(command, &task{
								AccountID:         aws.String(account.ID),
								APIRegion:         aws.String(r),
								VPC:               VPCInfo{},
								dryRun:            *dryRun,
								testEndpoint:      *testEndpoint,
								config:            centralInfo[versionGreenfield][currentRegion],
								api:               api,
								ctx:               ctx,
								exceptionVPCList:  exceptionVPCList,
								tgwTemplates:      tgwTemplates,
								shared:            *includeShared,
								version:           versionLegacy,
								quiet:             *quietFlag,
								verbose:           *verboseFlag,
								routeTables:       map[string]*ec2.RouteTable{},
								ips:               map[string]*ip{},
								limitVPC:          *limitVPC,
								filterCIDR:        *limitCIDR,
								usePrivilegedRole: *usePrivilegedRole,
								args:              args,
							})
						}
					}
					processedList = append(processedList, account.ID)
				}
				close(jobs)
			}()
		} else {
			log.Printf("Operating in region '%s'\n", currentRegion)
			for _, region := range regions {
				if region != string(currentRegion) {
					continue
				}
				processedInRegion := 0

				go func(region database.Region) {
					vpcs, err := func() ([]*database.VPC, error) {
						// Grab the list of vpcs by looking at the TGW attachments
						if *operateByTGWAttachments {
							EC2, _, err := getCentralAccountCredentials(versionGreenfield, currentRegion)
							if err != nil {
								return nil, fmt.Errorf("Unable to get central account credentials: %s", err)
							}
							vpcs, err := getVPCsAttachedToTGW(EC2, centralInfo[versionGreenfield][currentRegion])
							if err != nil {
								return nil, fmt.Errorf("Unable to get the list of VPCs attached to the TGW: %s", err)
							}
							for id, vpc := range vpcs {
								cachedVpc := getVPC(vpc.ID)
								if cachedVpc != nil {
									vpc = cachedVpc.VPC
									vpcs[id] = vpc
								} else {
									log.Printf("No cached VPC %s, need refresh!\n", vpc.ID)
									vpc.Region = currentRegion
								}
							}
							return vpcs, nil
						}
						return api.FetchAutomatedVPCsByRegion(region)
					}()
					if err != nil {
						log.Printf("Error fetching VPCs: %s", err)
						return
					}
					accountLocks = map[string]*sync.Mutex{}
					cacheLocks = map[string]*sync.Mutex{}
					for _, vpc := range vpcs {
						vpcinfo := getVPC(vpc.ID)
						if *limitVPC != "" {
							if *limitVPC != vpc.ID {
								continue
							}
						} else {
							if (!*includeShared && (vpc.Stack == string(stackShared))) || (*includeShared && (vpc.Stack != string(stackShared))) {
								continue
							}
							if *limitStack != "" && (*limitStack != vpc.Stack) {
								continue
							}
							if *limitAccount != "" && (*limitAccount != vpc.AccountID) {
								continue
							}
							if *limitCIDR != "" {
								if vpcinfo == nil {
									log.Fatalf("Unable to filter by cidr, missing data for vpc: %s\n", vpc.ID)
								}
								if len(vpcinfo.CIDRs) == 0 {
									cidrs, err := getCIDRsForVPC(vpc.AccountID, vpc.ID, region)
									if err != nil {
										log.Printf("Error fetching CIDRs: %s\n", err)
										continue
									}
									vpcinfo.CIDRs = cidrs
								}
								found := false
								for _, cidr := range vpcinfo.CIDRs {
									if found, _ = cidrIsWithin(cidr, *limitCIDR); found {
										break
									}
								}
								if !found {
									continue
								}
							}
						}
						if vpcinfo == nil {
							vpcinfo = &VPCInfo{
								VPC: vpc,
								Type: func() database.VPCType {
									if vpc.State != nil {
										return vpc.State.VPCType
									} else {
										return -1
									}
								}(),
							}
						}
						queueVPC(vpcinfo)
						if _, ok := processedByStack[vpc.Stack]; !ok {
							processedByStack[vpc.Stack] = []string{}
						}
						processedByStack[vpc.Stack] = append(processedByStack[vpc.Stack], vpc.ID)
						processedList = append(processedList, vpc.ID)
						processedInRegion++
					}
					if processedInRegion == 0 && *limitVPC != "" && *limitAccount != "" {
						vpc, err := api.FetchVPCDetails(*limitAccount, string(region), *limitVPC)
						if err != nil {
							fmt.Printf("Error fetching VPC details for vpc %s\n", *limitVPC)
						} else {
							vpcinfo := &VPCInfo{
								VPC: vpc,
							}
							queueVPC(vpcinfo)
							processedList = append(processedList, vpc.ID)
						}
					}
					close(jobs)
				}(database.Region(region))
			}
		}
	}
	done := make(chan bool)
	go func() {
		ctr := 0
		for result := range results {
			key := func(r *JobResult) string {
				if result.job.task.AccountID != nil {
					return fmt.Sprintf("%s/%s", aws.StringValue(result.job.task.AccountID), aws.StringValue(result.job.task.APIRegion))
				}
				return result.job.task.VPC.ID
			}(&result)
			jobData[key] = result
			if result.err != nil {
				log.Printf("Failed job: '%s' %s: %s\n", result.job.command, key, result.err)
			}
			ctr++
			if ctr%100 == 0 {
				log.Printf("%d jobs complete...\n", ctr)
			}
		}
		done <- true
	}()
	createWorkerPool(*maxConcurrency)
	<-done
	entity := "vpcs"
	if stringInSlice(command, accountCommands) {
		entity = "accounts"
	}
	failedJobs := 0
	for context, jd := range jobData {
		if jd.err != nil {
			failedJobs++
		}
		if len(jd.output) > 0 {
			for _, line := range jd.output {
				log.Printf("[%s] %s\n", context, line)
			}
		}
	}
	log.Printf("Task complete: %d %s processed (w/%d error%s): %s\n", len(processedList), entity, failedJobs, pluralS(failedJobs), strings.Join(processedList, ", "))
}
