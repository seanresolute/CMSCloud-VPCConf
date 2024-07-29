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
	"net/url"
	"os"
	"os/signal"
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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/servicequotas"

	"github.com/go-redis/redis"
)

const (
	redisHost        = "redis-host:6379"
	cacheCredentials = "credentials"
	cacheGlobal      = "global"
	cacheVPC         = "vpc"
	cacheBackup      = "backup"
)

var accountLocks map[string]*sync.Mutex
var cacheLock map[string]*sync.Mutex

const (
	durationWeek     = 7 * 24 * time.Hour
	durationInfinity = 0
)

const (
	defaultMaxRouteTableSize = 50
	routeTableSizeBuffer     = 0
	defaultPrefixListSize    = 16
)

const routesPerRouteTableQuotaCode = "L-93826ACB"

const (
	regionAll     = "all"
	regionEast    = "us-east-1"
	regionWest    = "us-west-2"
	regionGovWest = "us-gov-west-1"
)
const minArgs = 1

const (
	errTextDryRun = "DryRunOperation"
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

type version string
type stack string

const (
	versionLegacy     version = "legacy"
	versionGreenfield version = "greenfield"
	stackShared       stack   = "shared"
	stackDev          stack   = "dev"
	stackImp          stack   = "impl"
	stackProd         stack   = "prod"
)

type testEndpointDefinition struct {
	Hostname *string
	IP       string
	Port     int16
}

var (
	sharedServiceTestEndpoints = map[string]testEndpointDefinition{
		"TrendMicro": {
			Hostname: aws.String("internal-dsm-prod-elb-us-east-1-1932432501.us-east-1.elb.amazonaws.com"),
			IP:       "10.223.126.6",
			Port:     4120,
		},
	}
)

const defaultTestEndpoint = "TrendMicro"

var (
	currentRegion = database.Region(regionEast)
	creds         = &awsc.CloudTamerAWSCreds{}
)

var (
	commands = map[string]func(*credentials.Credentials, *task) ([]string, error){
		"audit":            auditPrimaryRoutes,
		"create":           createRoute,
		"cleanup":          cleanupPrimaryRoutes,
		"inspect":          inspectPrimaryRoutes,
		"remove":           removePrefixListRoute,
		"verify":           verifyPrefixList,
		"import":           importVPC,
		"test":             testSharedServiceFromVPC,
		"cleanup-test":     cleanupTestInfrastructure,
		"check":            checkTableSizes,
		"refresh":          refreshVPCData,
		"info":             showVPCData,
		"routes":           showRouteInfo,
		"backup-routes":    backupRoutes,
		"restore-routes":   restoreRoutes,
		"show-backup":      showBackupRoutes,
		"backup-diff":      showBackupDiff,
		"list":             listVPCs,
		"accounts":         listAccounts,
		"find-external":    findIP,
		"block":            blockTraffic,
		"attach-templates": gatherRouteInformation,
	}
	commandNames    = []string{}
	offlineCommands = []string{"list", "show-backup", "info"}
	accountCommands = []string{"accounts", "find-external", "block"}
)

var tgwRouteTables = map[version]map[database.Region]map[stack]string{
	versionLegacy: {
		regionEast: {
			stackDev:    "tgw-rtb-06ba03207e513ac94",
			stackImp:    "tgw-rtb-0253e0a641de3a804",
			stackProd:   "tgw-rtb-0608d3040f9b1c5f2",
			stackShared: "tgw-rtb-04c4d9a3d9dc9479d",
		},
		regionWest: {
			stackDev:    "tgw-rtb-06ba03207e513ac94",
			stackImp:    "tgw-rtb-0253e0a641de3a804",
			stackProd:   "tgw-rtb-0608d3040f9b1c5f2",
			stackShared: "tgw-rtb-04c4d9a3d9dc9479d",
		},
	},
}

type tgwInfo struct {
	ID, AccountID string
}

var centralInfo = map[version]map[database.Region]tgwInfo{
	versionLegacy: {
		regionEast: {
			ID:        "tgw-0f9f90319b91f8ee4",
			AccountID: "842420567215",
		},
		regionWest: {},
	},
	versionGreenfield: {
		regionEast: {
			ID:        "tgw-0f08888a7e2f3e38c",
			AccountID: "921617238787",
		},
		regionWest: {
			ID:        "tgw-085252a1e3587e1c7",
			AccountID: "921617238787",
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

var (
	routeDescriptions = map[string]string{
		"10.128.0.0/16":   "AWS-OC east",
		"10.131.125.0/24": "eLDAP impl",
		"10.138.1.0/24":   "eLDAP prod",
		"10.138.132.0/22": "shared lower",
		"10.223.120.0/22": "Greenfield Lower SS",
		"10.223.126.0/23": "Greenfield Prod SS",
		"10.223.128.0/20": "Greenfield Prod SS",
		"10.235.58.0/24":  "eLDAP dev",
		"10.232.32.0/19":  "VPN",
		"10.244.96.0/19":  "Shared entv3",
		"10.252.0.0/16":   "old legacy shared services - unneeded",
		"10.252.0.0/22":   "dev sec ops prod",
	}
	prefixLists = map[database.Region]map[stack]string{
		regionEast: {
			stackShared: "",
			stackProd:   "pl-0ffc092c126b667ff", // Unified-InterVPC-East-Prod
			stackImp:    "pl-0a969cb30f03c3499", // Unified-InterVPC-East-Impl
			stackDev:    "pl-0d490b86ca2360ddd", // Unified-InterVPC-East-Dev
		},
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
	mtgaTemplatesByStack = map[stack]map[string]uint64{
		stackDev: {
			"base": 20,
		},
		stackImp: {
			"base": 28,
		},
		stackProd: {
			"base": 29,
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

func generateCacheKey(scope, name string) string {
	return fmt.Sprintf("tgwut/%s/%s", scope, name)
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

type TGWUTConfig struct {
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

type vpcInfo struct {
	*database.VPC
	CIDRs       []string
	RouteTables map[string]*routeTable
}

type accountDetails struct {
	VPCs []*database.VPC
}

type ip struct {
	IP        net.IP
	PrivateIP net.IP
}

type task struct {
	AccountID         *string
	APIRegion         *string
	VPC               vpcInfo
	EC2               ec2iface.EC2API
	dryRun            bool
	config            tgwInfo
	version           version
	api               *VPCConfAPI
	ctx               context.Context
	exceptionVPCList  []string
	quiet             bool
	shared            bool
	skipMissing       bool
	testEndpoint      string
	managedRoutes     []string
	routesToFilter    []string
	args              []string
	usePrivilegedRole bool
	routeTables       map[string]*ec2.RouteTable
	routeBlocks       []*net.IPNet
	ips               map[string]*ip
	hasLegacy252      bool
	limitVPC          string
	filterCIDR        string
	filterEnv         string
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
		return t.VPC.ID
	}()
	return fmt.Sprintf("[%s%s] %s", context, drs, str)
}

func failTask(s string, a ...interface{}) ([]string, error) {
	return []string{fmt.Sprintf(s, a...)}, fmt.Errorf(s, a...)
}

func (t *task) Debug(msg string, args ...interface{}) {
	log.Printf("%s", t.LogString(fmt.Sprintf("DEBUG %s", msg), args...))
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

func (t *task) getRouteTables() (map[string]*routeTable, error) {
	routeTables := make(map[string]*routeTable)
	err := t.getRawRouteTables()
	if err != nil {
		t.Error("fetching raw route tables")
		return nil, err
	}
	for id, rtb := range t.routeTables {
		routes := []*routeTableEntry{}
		tableType := "unknown"
		for _, tag := range rtb.Tags {
			if aws.StringValue(tag.Key) == "Type" {
				tableType = aws.StringValue(tag.Value)
			}
		}
		// Couldn't find a Type tag on the route table itself, so walk the subnets for "use" tags (a la greenfield)
		if tableType == "unknown" {
			subnets := []*string{}
			for _, association := range rtb.Associations {
				if association.SubnetId != nil && *association.SubnetId != "" {
					subnets = append(subnets, association.SubnetId)
				}
			}
			if len(subnets) > 0 {
				out, err := t.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
					SubnetIds: subnets,
				})
				if err != nil {
					t.Warn("Unable to pull subnets for greenfield type checking: %s\n", err)
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

func (t *task) routeExists(route string) bool {
	if len(t.routeTables) == 0 {
		t.getRawRouteTables()
	}
	for _, rt := range t.routeTables {
		for _, r := range rt.Routes {
			if r.TransitGatewayId != nil && aws.StringValue(r.TransitGatewayId) == t.config.ID && aws.StringValue(r.DestinationCidrBlock) == route {
				return true
			}
		}
	}
	return false
}

func (t *task) getMaximumRouteTableSize() int {
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
		return defaultMaxRouteTableSize
	}
	return int(*out.Quota.Value)
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

func (t *task) checkOrUpdateRoutes(performUpdates bool) error {
	prefixList := t.getPrefixList()
	prefixListSize := t.getPrefixListSize(prefixList)
	if prefixListSize == 0 {
		prefixListSize = defaultPrefixListSize
	}
	maxRouteTableSize := t.getMaximumRouteTableSize()
	err := t.getRawRouteTables()
	if err != nil {
		return err
	}
	tableIsTooLarge := false
	for routeTableId, table := range t.routeTables {
		existingRouteCount := 0
		blackholeRouteCount := 0
		for _, r := range table.Routes {
			incr := 1
			if r.DestinationCidrBlock == nil {
				incr = t.getPrefixListSize(r.DestinationPrefixListId)
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
		if existingRouteCount+prefixListSize+blackholeRouteCount > maxRouteTableSize-routeTableSizeBuffer {
			tableIsTooLarge = true
			if existingRouteCount+prefixListSize < maxRouteTableSize-routeTableSizeBuffer {
				t.Warn("%s Too many routes on %s: %d + %d + %d blackholes = %d (> %d, with %d route buffer)", t.VPC.AccountID, routeTableId, existingRouteCount, prefixListSize, blackholeRouteCount, existingRouteCount+prefixListSize+blackholeRouteCount, maxRouteTableSize, routeTableSizeBuffer)
			} else {
				t.Error("%s Too many routes on %s: %d + %d + %d blackholes = %d (> %d, with %d route buffer)", t.VPC.AccountID, routeTableId, existingRouteCount, prefixListSize, blackholeRouteCount, existingRouteCount+prefixListSize+blackholeRouteCount, maxRouteTableSize, routeTableSizeBuffer)
			}
		} else if !t.quiet && !performUpdates {
			t.Log("Route count on %s: %d + %d = %d", routeTableId, existingRouteCount, prefixListSize, existingRouteCount+prefixListSize)
		}
	}
	if performUpdates {
		if tableIsTooLarge {
			return fmt.Errorf("Route table is too large to apply route updates")
		}
		if prefixLists != nil {
			routeTables, err := t.getRouteTables()
			if err != nil {
				return fmt.Errorf("Getting route tables")
			}
			for routeTableId, table := range routeTables {
				supernetsExist := false
				lessSpecificRoutes, err := t.checkForSupernets(table)
				if err != nil {
					return fmt.Errorf("Checking for supernets: %s", err)
				}
				if len(lessSpecificRoutes) > 0 {
					t.Error("Route table %s has less specific routes (%d): %s", routeTableId, len(lessSpecificRoutes), strings.Join(lessSpecificRoutes, ", "))
					supernetsExist = true
				}
				if !supernetsExist {
					err := t.createNewRouteToTGW(routeTableId, aws.StringValue(prefixList), t.config.ID)
					if err != nil {
						if err != errCodeRouteExists {
							t.Error("Failed to create route for %s -> %s on %s: %s\n", aws.StringValue(prefixList), t.config.ID, routeTableId, err)
						}
					} else {
						t.Log("Route created: %s -> %s on %s\n", aws.StringValue(prefixList), t.config.ID, routeTableId)
					}
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
			if aws.StringValue(checkRoute.TransitGatewayId) != t.config.ID {
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

	if t.dryRun {
		t.Log("delete route for %s on %s", target, routeTableId)
	}
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
	return "unknown"
}

func (t *task) getCIDRs() ([]string, error) {
	return getCIDRs(t.EC2, t.VPC.ID)
}

func (t *task) saveVPC() {
	cache.set(cacheVPC, t.VPC.ID, t.VPC, durationWeek)
}

func (t *task) routeIsSubnetOfManagedRoutes(destination string) (string, bool, error) {
	for _, block := range t.routeBlocks {
		within, err := cidrIsWithin(destination, block.String())
		if err != nil {
			return "", false, err
		} else if within {
			return block.String(), true, nil
		}
	}
	return "", false, nil
}

func (t *task) routeIsSupernetOfManagedRoutes(destination string) (string, bool, error) {
	for _, block := range t.routeBlocks {
		within, err := cidrIsWithin(block.String(), destination)
		if err != nil {
			return "", false, err
		} else if within {
			return block.String(), true, nil
		}
	}
	return "", false, nil
}

func (t *task) generateCIDRBlocks() {
	routeBlocks := make([]*net.IPNet, 0)
	for _, primary := range t.managedRoutes {
		_, block, err := net.ParseCIDR(primary)
		if err != nil {
			t.Error("parsing cidr %s\n", primary)
		}
		routeBlocks = append(routeBlocks, block)
	}
	t.routeBlocks = routeBlocks
}

func (t *task) ignoreRouteFromSharedService(route string) bool {
	if t.VPC.Stack == "shared" && stringInSlice(route, t.routesToFilter) {
		return true
	}
	return false
}

type VPCConfAPI struct {
	Username, Password string
	SessionID          string
	BaseURL            string
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
	form := url.Values{
		"username": []string{api.Username},
		"password": []string{api.Password},
	}
	client := &http.Client{}
	resp, err := client.PostForm(api.BaseURL+"/login", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("ERROR: Login to VPC-Conf failed: %s\n", resp.Status)
		return fmt.Errorf("logging in to vpcconf")
	}
	for _, c := range resp.Cookies() {
		if c.Name == "sessionID" {
			api.SessionID = c.Value
		}
	}
	return nil
}

func (api *VPCConfAPI) SubmitBatchTask(request BatchTaskRequest, dryRun bool) error {
	log.Println("SubmitBatchTask()")
	if api.SessionID == "" {
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
		log.Printf("Cookie: %s\n", api.SessionID)
		log.Printf("DRY-RUN: Payload: %s\n", string(b))
	} else {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{
			Name:  "sessionID",
			Value: api.SessionID,
			Path:  "/",
		})

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

func (api *VPCConfAPI) ImportVPC(vpc *database.VPC, dryRun bool) error {
	log.Println("ImportVPC()")
	if api.SessionID == "" {
		if err := api.Login(); err != nil {
			return err
		}
	}
	url := fmt.Sprintf("%s/%s/vpc/%s/%s/import?legacy=1", api.BaseURL, vpc.Region, vpc.AccountID, vpc.ID)
	if dryRun {
		log.Printf("DRY-RUN: %s\n", url)
		log.Printf("Cookie: %s\n", api.SessionID)
	} else {
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return err
		}

		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{
			Name:  "sessionID",
			Value: api.SessionID,
			Path:  "/",
		})

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Printf("ERROR: Importing VPC: %s\n", resp.Status)
			return fmt.Errorf("importing vpc")
		}
	}
	return nil
}

func (api *VPCConfAPI) FetchExceptionalVPCs() ([]string, error) {
	log.Println("FetchExceptionalVPCs()")
	exceptions := []string{}
	if api.SessionID == "" {
		if err := api.Login(); err != nil {
			return exceptions, err
		}
	}
	req, err := http.NewRequest("GET", api.BaseURL+"/exception.json", nil)
	if err != nil {
		return exceptions, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  "sessionID",
		Value: api.SessionID,
		Path:  "/",
	})

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
	return exceptions, nil
}

func (api *VPCConfAPI) FetchVPCDetails(account, region, vpcID string) (*database.VPC, error) {
	if api.SessionID == "" {
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
	req.AddCookie(&http.Cookie{
		Name:  "sessionID",
		Value: api.SessionID,
		Path:  "/",
	})

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("ERROR: fetching vpc %s: %s\n", vpcID, resp.Status)
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

		IsAutomated bool
		Config      struct {
			ManagedTransitGatewayAttachmentIDs []uint64
		}
	}
	vpcinfo := &VPCInfo{}
	err = json.Unmarshal(body, &vpcinfo)
	if err != nil {
		return nil, err
	}
	vpc := &database.VPC{
		ID:        vpcinfo.VPCID,
		AccountID: vpcinfo.AccountID,
		Name:      vpcinfo.Name,
		Region:    database.Region(region),
	}
	if vpcinfo.IsAutomated {
		vpc.State = &database.VPCState{}
		vpc.Config = &database.VPCConfig{
			ManagedTransitGatewayAttachmentIDs: vpcinfo.Config.ManagedTransitGatewayAttachmentIDs,
		}
	}
	return vpc, nil
}

func (api *VPCConfAPI) FetchAllAccountDetails() (map[string]*database.AWSAccount, error) {
	if api.SessionID == "" {
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
	req.AddCookie(&http.Cookie{
		Name:  "sessionID",
		Value: api.SessionID,
		Path:  "/",
	})

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
	EC2, err := getRWAccountCredentials(accountID, region)
	if err != nil {
		return nil, err
	}
	cidrs, err := getCIDRs(EC2, vpcID)
	return cidrs, err
}

func cidrIsInEnv(cidr, env string) bool {
	for _, byRegion := range allManagedRoutes {
		for stack, byStack := range byRegion {
			if string(stack) != env {
				continue
			}
			for _, block := range byStack {
				if cidr == block {
					return true
				}
			}
		}
	}
	return false
}

func cidrIsManaged(cidr string) bool {
	for _, byRegion := range allManagedRoutes {
		for _, byStack := range byRegion {
			for _, block := range byStack {
				if cidr == block {
					return true
				}
			}
		}
	}
	return false
}

func cidrIsWithinManaged(cidr string) bool {
	for _, byRegion := range allManagedRoutes {
		for _, byStack := range byRegion {
			for _, block := range byStack {
				if ok, _ := cidrIsWithin(cidr, block); ok {
					return true
				}
			}
		}
	}
	return false
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

func uint64InSlice(n uint64, sl []uint64) bool {
	for _, i := range sl {
		if i == n {
			return true
		}
	}
	return false
}

func getRWAccountCredentials(accountID string, region database.Region) (*ec2.EC2, error) {
	awscreds, err := getCredsByAccountID(creds, accountID)
	if err != nil {
		log.Printf("Error getting credentials for %s: %s", accountID, err)
		return nil, err
	}
	return ec2.New(getSessionFromCreds(awscreds, string(region))), nil
}

func getCentralAccountCredentials(version version, region database.Region) (*ec2.EC2, error) {
	if _, ok := centralInfo[version][region]; !ok {
		return nil, fmt.Errorf("No account info for %s/%s", version, string(region))
	}
	if _, ok := cacheLock[centralInfo[version][region].AccountID]; !ok {
		cacheLock[centralInfo[version][region].AccountID] = &sync.Mutex{}
	}
	return getRWAccountCredentials(centralInfo[version][region].AccountID, region)
}

func getLegacyVPCs(region database.Region) []*database.VPC {
	EC2, err := getCentralAccountCredentials(versionLegacy, region)
	if err != nil {
		log.Printf("Getting central credentials: %s\n", err)
		return nil
	}
	vpcs, err := getVPCsAttachedToTGW(EC2, centralInfo[versionLegacy][region])
	if err != nil {
		log.Printf("Error getting legacy VPC list: %s", err)
		return nil
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
	return vpcs
}

func getPrefixListEntries(region database.Region, plid string) ([]string, error) {
	rv := []string{}
	EC2, err := getCentralAccountCredentials(versionGreenfield, region)
	if err != nil {
		return []string{}, err
	}
	out, err := EC2.GetManagedPrefixListEntries(
		&ec2.GetManagedPrefixListEntriesInput{
			PrefixListId: aws.String(plid),
		},
	)
	if err != nil {
		return []string{}, err
	}
	for _, entry := range out.Entries {
		rv = append(rv, aws.StringValue(entry.Cidr))
	}
	return rv, nil
}

func generateManagedRouteLists(region database.Region) error {
	if _, ok := prefixLists[region]; !ok {
		return fmt.Errorf("Invalid region %s, no prefix lists defined", region)
	}
	for stack, plid := range prefixLists[region] {
		var err error
		if plid == "" {
			continue
		}
		allManagedRoutes[region][stack], err = getPrefixListEntries(region, plid)
		if err != nil {
			fmt.Printf("Error fetching PL %s from master account: %s", plid, err)
		}
	}
	return nil
}

func isValidCIDR(in string) bool {
	_, _, err := net.ParseCIDR(in)
	return (err == nil)
}

func (t *task) getPrefixList() *string {
	return getPrefixList(t.VPC.Region, stack(t.VPC.Stack))
}

func getPrefixList(region database.Region, requestedStack stack) *string {
	if _, ok := prefixLists[region]; !ok {
		log.Printf("WARN: Unsupported region: %s\n", region)
		return nil
	}
	if pl, ok := prefixLists[region][requestedStack]; ok {
		return &pl
	} else {
		log.Printf("Unsupported stack: %s\n", requestedStack)
		return nil
	}
}

func getRoutesForStack(region database.Region, requestedStack stack) []string {
	typesToCopy := []stack{}
	returnedRoutes := []string{}
	if _, ok := allManagedRoutes[region]; !ok {
		log.Printf("Unsupported region: %s\n", requestedStack)
		return returnedRoutes
	}
	if _, ok := allManagedRoutes[region][requestedStack]; ok {
		typesToCopy = append(typesToCopy, requestedStack)
		for _, s := range typesToCopy {
			returnedRoutes = append(returnedRoutes, allManagedRoutes[region][s]...)
		}
	} else {
		log.Printf("Unsupported stack: %s\n", requestedStack)
	}
	return returnedRoutes
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
	cacheLock[accountID].Lock()
	defer cacheLock[accountID].Unlock()
	cachedCreds := &credentials.Value{}
	err := cache.get(cacheCredentials, accountID, cachedCreds)
	if err != nil {
		log.Printf("  Getting credentials for account %s\n", accountID)
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
				cache.set(cacheCredentials, accountID, cachedCreds, 55*time.Minute)
				break
			}
		}
	} else {
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

func fetchTGWRTBAssociations(e ec2iface.EC2API, rtbID, vpcID string) ([]*ec2.TransitGatewayRouteTableAssociation, error) {
	startedTrying := time.Now()
	maxTryTime := 30 * time.Second

	for {
		out, err := e.GetTransitGatewayRouteTableAssociations(&ec2.GetTransitGatewayRouteTableAssociationsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("resource-id"),
					Values: []*string{aws.String(vpcID)},
				},
			},
			TransitGatewayRouteTableId: aws.String(rtbID),
		})
		if err == nil {
			return out.Associations, nil
		}
		log.Printf("Error fetching TGW attachments: %s\n", err)
		if time.Since(startedTrying) > maxTryTime {
			return nil, fmt.Errorf("timeout")
		}
		time.Sleep(5 * time.Second)
	}
}

// gatherRouteInformation: gathers the routing information for use in attach-templates
func gatherRouteInformation(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.getRawRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRawRouteTables(): %w\n", err)
	}
	if t.routeExists("10.252.0.0/16") {
		t.hasLegacy252 = true
	}
	v, err := t.api.FetchVPCDetails(t.VPC.AccountID, string(t.VPC.Region), t.VPC.ID)
	if err != nil {
		t.Error("Fetching details: SKIP!")
		return nil, err
	}
	t.VPC.Config = v.Config
	return nil, nil
}

// auditPrimaryRoutes: Audit the list of routes currently on the route tables within the current VPC
// If there are any extra routes we don't care about, or if any of the ones we do care about are missing, flag it as an issue
func auditPrimaryRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRouteTables(): %w\n", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	allVerified := true
	for routeTableId, table := range routeTables {
		extraRoutes := []string{}
		notes := []string{}
		misdirectedRoutes := map[string]*routeTableEntry{}
		routeVerified := make(map[string]bool, len(t.managedRoutes))
		for _, k := range t.managedRoutes {
			routeVerified[k] = false
		}
		tableHasTGWTarget := false
		for _, route := range table.Routes {
			if route.CIDR == nil {
				continue
			}
			destination := aws.StringValue(route.CIDR)
			supernet, within, err := t.routeIsSubnetOfManagedRoutes(destination)
			if err != nil {
				t.Error("checking if %s is longer prefix: %s\n", destination, err)
			} else {
				if route.nextHop() == t.config.ID {
					tableHasTGWTarget = true
					if stringInSlice(destination, t.managedRoutes) {
						if _, ok := routeVerified[destination]; ok {
							routeVerified[destination] = true
						}
						continue
					}
					if within {
						notes = append(notes, fmt.Sprintf("More specific route %s -> %s is longer prefix within %s\n", destination, route.nextHop(), supernet))
						if !t.ignoreRouteFromSharedService(destination) {
							extraRoutes = append(extraRoutes, destination)
						}
					} else {
						subnet, contains, err := t.routeIsSupernetOfManagedRoutes(destination)
						if err != nil {
							t.Error("checking if %s is shorter prefix: %s\n", destination, err)
							continue
						}
						if contains {
							notes = append(notes, fmt.Sprintf("Less specific route %s -> %s contains longer prefix %s\n", destination, route.nextHop(), subnet))
						}
					}
				} else {
					if _, ok := routeVerified[destination]; ok {
						misdirectedRoutes[destination] = route
					}
					if within && destination != supernet {
						notes = append(notes, fmt.Sprintf("Mispointed route %s -> %s is longer prefix within %s\n", destination, route.nextHop(), supernet))
					}
				}
			}
		}
		if !tableHasTGWTarget {
			continue
		}
		routesVerified := true
		for _, exists := range routeVerified {
			if !exists && !t.skipMissing {
				routesVerified = false
			}
		}
		if routesVerified && len(extraRoutes) == 0 && len(notes) == 0 {
			t.Log("route table %s GOOD", routeTableId)
		} else {
			allVerified = false
			if !t.quiet {
				t.Warn("route table %s not matching desired state", routeTableId)
			}
			sort.Strings(extraRoutes)
			if stringInSlice("10.131.125.0/24", extraRoutes) && stringInSlice("10.138.1.0/24", extraRoutes) {
				t.Log("NOTE %s: eLDAP YES impl + YES prod on %s env", routeTableId, t.VPC.Stack)
			}
			if stringInSlice("10.131.125.0/24", extraRoutes) && !stringInSlice("10.138.1.0/24", extraRoutes) {
				t.Log("NOTE %s: eLDAP YES impl + NO prod on %s env", routeTableId, t.VPC.Stack)
			}
			if !stringInSlice("10.131.125.0/24", extraRoutes) && stringInSlice("10.138.1.0/24", extraRoutes) {
				t.Log("NOTE %s: eLDAP NO impl + YES prod on %s env", routeTableId, t.VPC.Stack)
			}
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
			if len(table.Subnets) > 0 {
				if !t.skipMissing {
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
						}
					}
				}
			} else {
				t.Warn("%s has routes to TGW, but no subnets associated", routeTableId)
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
	}
	if allVerified {
		t.Log("All routes match")
	} else if !t.quiet {
		t.Warn("Some routes mismatch")
	}
	return nil, nil
}

// inspectPrimaryRoutes: Grab the list of routes and if there are any still pointing at the legacy TGW, flag it as an issue
func inspectPrimaryRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRouteTables(): %w\n", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	allVerified := true
	for routeTableId, table := range routeTables {
		extraRoutes := []string{}
		tableHasTGWTarget := false
		for _, route := range table.Routes {
			if route.nextHop() == t.config.ID {
				tableHasTGWTarget = true
				extraRoutes = append(extraRoutes, route.destination())
			}
		}
		if !tableHasTGWTarget {
			continue
		}
		if len(extraRoutes) == 0 {
			if !t.quiet {
				t.Log("route table %s GOOD", routeTableId)
			}
		} else {
			allVerified = false
			for _, route := range extraRoutes {
				t.Warn("route remains on %s post migration: %s", routeTableId, route)
			}
		}
	}
	if allVerified {
		t.Log("All routes match")
	} else if !t.quiet {
		t.Warn("Some routes remain")
	}
	return nil, nil
}

// checkTableSizes: Check the existing route tables to make sure our work won't exceed the 50 limit
func checkTableSizes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.checkOrUpdateRoutes(false)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// createRoute: Create the route to the prefix list, pointing to the new TGW
func createRoute(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.checkOrUpdateRoutes(true)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// removePrefixListRoute: Remove the route pointing to the prefix list
func removePrefixListRoute(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("removeRoute(): %w\n", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	currentPrefixList := t.getPrefixList()
	if currentPrefixList != nil {
		for routeTableId, table := range routeTables {
			routeDeletedFromTable := false
			for _, route := range table.Routes {
				destination := route.destination()
				if destination == aws.StringValue(currentPrefixList) {
					err := t.deleteRoute(routeTableId, destination)
					if err != nil {
						t.Error("DeleteRoute(%s, %s): %s", routeTableId, destination, err)
						routeDeletedFromTable = true
						continue
					}
				}
			}
			if !routeDeletedFromTable {
				t.Warn("No prefix list route for %s on %s", *currentPrefixList, routeTableId)
			}
		}
	} else {
		t.Error("Unspecified target prefix list")
	}
	return nil, nil
}

// verifyPrefixList: Verify the prefix list is on the route table and pointing to the new TGW
func verifyPrefixList(awscreds *credentials.Credentials, t *task) ([]string, error) {
	routeTables, err := t.getRouteTables()
	if err != nil {
		return nil, fmt.Errorf("verifyPrefixList(): %w\n", err)
	}
	if len(routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	allVerified := true
	currentPrefixList := t.getPrefixList()
	if currentPrefixList == nil {
		t.Warn("No prefix list configured, unable to verify")
		return nil, fmt.Errorf("No prefix list configured")
	}
	for routeTableId, table := range routeTables {
		extraRoutes := []string{}
		notes := []string{}
		primaryRoutes := []string{}
		routeVerified := make(map[string]bool, len(t.managedRoutes)+1)
		routeVerified[*currentPrefixList] = false
		tableVerified := true
		tableHasTGWTargets := false
		for _, route := range table.Routes {
			destination := route.destination()
			if !isValidCIDR(destination) {
				routeVerified[destination] = true
				if aws.StringValue(route.TGWID) == centralInfo[versionGreenfield][t.VPC.Region].ID {
					tableHasTGWTargets = true
				}
				continue
			}
			if aws.StringValue(route.TGWID) == t.config.ID {
				tableHasTGWTargets = true
				if _, ok := routeVerified[destination]; ok {
					routeVerified[destination] = true
				} else {
					if !stringInSlice(destination, primaryRoutes) && !cidrIsManaged(destination) && !cidrIsWithinManaged(destination) && !t.ignoreRouteFromSharedService(destination) {
						extraRoutes = append(extraRoutes, destination)
						if !t.quiet {
							t.Warn("Extra route %s in %s to tgw\n", destination, routeTableId)
						}
					}
				}
			} else {
				if supernet, within, err := t.routeIsSubnetOfManagedRoutes(destination); err != nil {
					t.Error("checking if %s is longer prefix: %s\n", destination, err)
				} else if within && destination != supernet {
					notes = append(notes, fmt.Sprintf("Mispointed route %s -> %s is longer prefix within %s\n", destination, route.nextHop(), supernet))
				}
			}
		}
		if !tableHasTGWTargets {
			continue
		}
		for _, extraRoute := range extraRoutes {
			if supernet, within, err := t.routeIsSubnetOfManagedRoutes(extraRoute); err != nil {
				t.Error("checking if extra route %s is longer prefix: %s\n", extraRoute, err)
			} else if within {
				notes = append(notes, fmt.Sprintf("Extra route %s is longer prefix within %s\n", extraRoute, supernet))
			}
		}
		missingCount := 0
		missing := []string{}
		for route, exists := range routeVerified {
			if !exists {
				missingCount++
				missing = append(missing, route)
			}
		}
		if missingCount > 0 {
			if len(table.Subnets) > 0 {
				t.Error("Route Table %s (type: %s) missing routes (%d): %s\n", routeTableId, table.Type, missingCount, strings.Join(missing, ", "))
				tableVerified = false
			} else {
				if !t.quiet {
					t.Warn("Route Table %s (type: %s) missing routes (%d): %s\n", routeTableId, table.Type, missingCount, strings.Join(missing, ", "))
				}
			}
		}
		for _, note := range notes {
			t.Warn(note)
		}
		if len(extraRoutes) > 0 && len(table.Subnets) > 0 {
			tableVerified = false
		}
		if !tableVerified {
			allVerified = false
			if !t.quiet {
				t.Log("%s on subnets [%s]\n", routeTableId, strings.Join(table.Subnets, ","))
			}
		}
	}
	if allVerified {
		t.Log("All routes verified")
	}
	return nil, nil
}

// showRouteInfo: Show a trimmed version of the current route tables that reflect all routes that point at the current TGW
func showRouteInfo(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.getRawRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRawRouteTables(): %w\n", err)
	}
	if len(t.routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	for routeTableId, table := range t.routeTables {
		for _, route := range table.Routes {
			destination := aws.StringValue(route.DestinationCidrBlock)
			if aws.StringValue(route.TransitGatewayId) == t.config.ID {
				if len(table.Associations) > 0 && aws.BoolValue(table.Associations[0].Main) {
					t.Log("%s [MAIN] route %s -> %s (%s)\n", routeTableId, destination, aws.StringValue(route.TransitGatewayId), t.getRouteDescription(destination))
				} else {
					t.Log("%s route %s -> %s (%s)\n", routeTableId, destination, aws.StringValue(route.TransitGatewayId), t.getRouteDescription(destination))
				}
			}
		}
	}
	return nil, nil
}

// cleanupPrimaryRoutes: Remove all of the "primary" routes - used for cleanup
func cleanupPrimaryRoutes(awscreds *credentials.Credentials, t *task) ([]string, error) {
	err := t.getRawRouteTables()
	if err != nil {
		return nil, fmt.Errorf("getRawRouteTables(): %w\n", err)
	}
	if len(t.routeTables) == 0 {
		t.Error("No route tables found with routes for %s/%s\n", t.config.AccountID, t.config.ID)
		return nil, fmt.Errorf("No routes")
	}
	currentPrefixList := t.getPrefixList()
	if currentPrefixList == nil {
		return nil, fmt.Errorf("Unable to check if prefix list is attached, currentPrefixList is nil")
	}
	for routeTableId, table := range t.routeTables {
		hasPrefixListRoute := false
		for _, route := range table.Routes {
			if route.DestinationPrefixListId != nil && aws.StringValue(route.DestinationPrefixListId) == aws.StringValue(currentPrefixList) {
				hasPrefixListRoute = true
			}
		}
		isMainTable := false
		for _, association := range table.Associations {
			isMainTable = isMainTable || aws.BoolValue(association.Main)
		}
		associations := len(table.Associations)
		if !hasPrefixListRoute && ((associations == 1 && !isMainTable) || (associations > 1)) {
			t.Log("Route table %s has no prefix list route, but is associated with subnets (%d): SKIP!", routeTableId, len(table.Associations))
			continue
		}
		cleanedRoutes := make(map[string]bool, len(t.managedRoutes))
		for _, route := range t.managedRoutes {
			cleanedRoutes[route] = false
		}
		for _, route := range table.Routes {
			destination := aws.StringValue(route.DestinationCidrBlock)
			if err != nil {
				t.Log("Error marshaling route: %s\n", err)
			}
			if aws.StringValue(route.TransitGatewayId) == t.config.ID {
				if cidrIsManaged(destination) {
					t.Log("Found %s on %s, delete\n", destination, routeTableId)
					err := t.deleteRoute(routeTableId, aws.StringValue(route.DestinationCidrBlock))
					if err != nil {
						t.Error("DeleteRoute(%s, %s): %s", routeTableId, aws.StringValue(route.DestinationCidrBlock), err)
						continue
					}
					cleanedRoutes[destination] = true
				}
			}
		}
	}
	return nil, nil
}

// refreshVPCData: Force a read of the current state of the route tables on the selected VPCs
func refreshVPCData(awscreds *credentials.Credentials, t *task) ([]string, error) {
	matchedStack := false
	if _, ok := tgwRouteTables[t.version]; !ok {
		return nil, fmt.Errorf("Invalid route table version: %s\n", t.version)
	}
	if _, ok := tgwRouteTables[t.version][t.VPC.Region]; !ok {
		return nil, fmt.Errorf("No route table for region %s in %s\n", t.VPC.Region, t.version)
	}
	for stack, rtb := range tgwRouteTables[t.version][t.VPC.Region] {
		centralEC2, err := getCentralAccountCredentials(t.version, t.VPC.Region)
		if err != nil {
			return []string{}, err
		}
		associations, err := fetchTGWRTBAssociations(centralEC2, rtb, t.VPC.ID)
		if err != nil {
			t.Error("fetching RTB associations for %s\n", rtb)
			continue
		}
		if len(associations) > 0 {
			if matchedStack {
				t.Log("Multiple RTB associations, shared?")
				t.VPC.Stack = "unknown"
			} else {
				t.VPC.Stack = string(stack)
			}
			matchedStack = true
		}
	}
	if t.VPC.Stack == "" {
		t.Log("Unknown stack")
	} else {
		t.Log("Stack set to %s\n", t.VPC.Stack)
	}
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
	t.VPC.RouteTables, err = t.getRouteTables()
	if err != nil {
		t.Error("fetching route tables\n")
		return nil, err
	}
	cidrs, err := t.getCIDRs()
	if err != nil {
		t.Error("Unable to fetch CIDRs: %s\n", err)
	}
	t.VPC.CIDRs = cidrs
	t.saveVPC()
	return nil, nil
}

// importVPC: Import a VPC to VPC-Conf
func importVPC(awscreds *credentials.Credentials, t *task) ([]string, error) {
	if !stringInSlice(t.VPC.ID, t.exceptionVPCList) {
		vpc, err := t.api.FetchVPCDetails(t.VPC.AccountID, string(t.VPC.Region), t.VPC.ID)
		if err != nil {
			t.Error("fetching details (maybe doesn't exist?): %s", err)
		}
		if vpc != nil && vpc.State != nil {
			t.Log("already imported")
			return nil, nil
		}
		err = t.api.ImportVPC(t.VPC.VPC, t.dryRun)
		if err != nil {
			t.Error("importing: %s", err)
			return nil, err
		}
	} else {
		t.Log("Exception VPC, SKIP!")
	}
	return nil, nil
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

// blockTraffic: One-off add NACLs to block any traffic related to solarwinds hack (tcp/17778 and udp/17778)
func blockTraffic(awscreds *credentials.Credentials, t *task) ([]string, error) {
	output := []string{}
	vo, err := t.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}
	vpcs := make([]string, 0)
	for _, v := range vo.Vpcs {
		if (t.limitVPC != "" && t.limitVPC == aws.StringValue(v.VpcId)) || (t.limitVPC == "") {
			vpcs = append(vpcs, aws.StringValue(v.VpcId))
		}
	}
	for _, vpc := range vpcs {
		sno, err := t.EC2.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{vpc}),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		subnets := make([]*string, 0)
		updatableAcls := make([]string, 0)
		for _, sn := range sno.Subnets {
			subnets = append(subnets, sn.SubnetId)
		}
		do, err := t.EC2.DescribeNetworkAcls(&ec2.DescribeNetworkAclsInput{})
		if err != nil {
			return nil, err
		}
		for _, o := range do.NetworkAcls {
			for _, a := range o.Associations {
				if stringInSlice(aws.StringValue(a.SubnetId), aws.StringValueSlice(subnets)) {
					t.Log("Subnet %s is already associated with a NACL, modify existing in place", aws.StringValue(a.SubnetId))
					if !stringInSlice(aws.StringValue(o.NetworkAclId), updatableAcls) {
						updatableAcls = append(updatableAcls, aws.StringValue(o.NetworkAclId))
					}
				}
			}
			for _, e := range o.Entries {
				if !t.quiet {
					t.Log("Existing entry: %s\n", e.String())
				}
			}
		}
		naclIDs := updatableAcls
		protocols := map[string]*string{
			"tcp": aws.String("6"),
			"udp": aws.String("17"),
		}
		for _, naclID := range naclIDs {
			for _, direction := range []bool{true} {
				for pname, pid := range protocols {
					failed := func() error {
						_, err = t.EC2.CreateNetworkAclEntry(&ec2.CreateNetworkAclEntryInput{
							CidrBlock:    aws.String("0.0.0.0/0"),
							Egress:       aws.Bool(direction),
							DryRun:       aws.Bool(t.dryRun),
							NetworkAclId: aws.String(naclID),
							Protocol:     pid,
							PortRange: &ec2.PortRange{
								From: aws.Int64(17778),
								To:   aws.Int64(17778),
							},
							RuleAction: aws.String("deny"),
							RuleNumber: aws.Int64(99),
						})
						if err != nil {
							if aerr, ok := err.(awserr.Error); ok {
								if aerr.Code() == errTextDryRun {
									return nil
								} else {
									return err
								}
							} else {
								return err
							}
						}
						return nil
					}()
					if failed != nil {
						return nil, failed
					}
					t.Log("Create NACL blocking %s/17778 egress on %s", pname, naclID)
				}
			}
			for _, direction := range []bool{true, false} {
				failed := func() error {
					_, err = t.EC2.CreateNetworkAclEntry(&ec2.CreateNetworkAclEntryInput{
						CidrBlock:    aws.String("0.0.0.0/0"),
						Egress:       aws.Bool(direction),
						DryRun:       aws.Bool(t.dryRun),
						NetworkAclId: aws.String(naclID),
						Protocol:     aws.String("-1"),
						RuleAction:   aws.String("allow"),
						RuleNumber:   aws.Int64(100),
					})
					if err != nil {
						if aerr, ok := err.(awserr.Error); ok {
							if aerr.Code() == errTextDryRun {
								return nil
							} else {
								return err
							}
						} else {
							return err
						}
					}
					return nil
				}()
				if failed != nil {
					return nil, failed
				}
			}
		}
		t.Log("Ensured default NACL rule 100 is in place")
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
				}
			} else {
				// Route exists in current state, make sure it points to the right target
				for _, c := range current[rtbid].Routes {
					if route.destination() == c.destination() && route.nextHop() != c.nextHop() {
						t.Log("Route %s mispointing at %s, want %s\n", c.destination(), c.nextHop(), route.nextHop())
						if fix && route.TGWID != nil {
							time.Sleep(5 * time.Second)
							t.deleteRoute(rtbid, route.destination())
							if err := t.createNewRouteToTGW(rtbid, route.destination(), *route.TGWID); err != nil {
								t.Warn("unable to create route %s -> %s (%s)\n", route.destination(), *route.TGWID, rtbid)
							} else {
								t.Log("route created %s -> %s (%s)\n", route.destination(), *route.TGWID, rtbid)
							}
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
	}
	j, err := json.Marshal(info)
	if err != nil {
		t.Error("marshal error\n")
		return nil, err
	}
	t.Log("%s\n", string(j))
	return nil, nil
}

func (t *task) getAccountDetails() (*accountDetails, error) {
	output, err := t.EC2.DescribeVpcs(&ec2.DescribeVpcsInput{})
	if err != nil {
		return nil, err
	}

	details := &accountDetails{}
	for _, vpc := range output.Vpcs {
		details.VPCs = append(details.VPCs, &database.VPC{
			AccountID: aws.StringValue(t.AccountID),
			ID:        aws.StringValue(vpc.VpcId),
			Region:    currentRegion,
		})
	}
	return details, nil
}

func listAccounts(awscreds *credentials.Credentials, t *task) ([]string, error) {
	accountDetails, err := t.getAccountDetails()
	if err != nil {
		return nil, err
	}
	j, err := json.Marshal(accountDetails)
	if err != nil {
		t.Error("marshal error\n")
		return nil, err
	}
	t.Log("%s\n", string(j))
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
	test := &connection.NetworkConnectionTest{
		Context:     t.ctx,
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

func getVPC(vpcID string) *vpcInfo {
	cachedVPC := &vpcInfo{}
	if err := cache.get(cacheVPC, vpcID, &cachedVPC); err == nil {
		return cachedVPC
	}
	return nil
}

func (t *task) getRequestedRoles(account string) error {
	workingRegion := func() database.Region {
		if t.APIRegion != nil {
			return database.Region(aws.StringValue(t.APIRegion))
		}
		return database.Region("us-west-2")
	}()
	var err error
	t.EC2, err = getRWAccountCredentials(account, workingRegion)
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
			select {
			case <-j.task.ctx.Done():
				return
			default:
			}
			if j.task.AccountID != nil {
				acctID := aws.StringValue(j.task.AccountID)
				accountLocks[acctID].Lock()
				defer accountLocks[acctID].Unlock()
				if !stringInSlice(j.command, offlineCommands) {
					err = taskWithLogging.getRequestedRoles(acctID)
					if err != nil {
						results <- JobResult{job: j, err: err, output: nil}
						return
					}
				}
			} else {
				if j.command != "refresh" && j.command != "list" {
					taskWithLogging.managedRoutes = getRoutesForStack(taskWithLogging.VPC.Region, stack(taskWithLogging.VPC.Stack))
					log.Printf("VPC %s / %s / %s / %s", taskWithLogging.VPC.ID, taskWithLogging.VPC.Stack, taskWithLogging.VPC.AccountID, taskWithLogging.VPC.Region)
				}
				// We need to filter the routes to work on based on --cidr and/or --env flags
				if taskWithLogging.VPC.Stack == "shared" {
					for id, route := range taskWithLogging.managedRoutes {
						if route == taskWithLogging.filterCIDR || (taskWithLogging.filterEnv != "" && cidrIsInEnv(route, taskWithLogging.filterEnv)) {
							taskWithLogging.managedRoutes = append(taskWithLogging.managedRoutes[:id], taskWithLogging.managedRoutes[id+1:]...)
							taskWithLogging.routesToFilter = append(taskWithLogging.routesToFilter, route)
						}
					}
				}
				taskWithLogging.generateCIDRBlocks()
				if !stringInSlice(j.command, offlineCommands) {
					err = taskWithLogging.getRequestedRoles(taskWithLogging.VPC.AccountID)
					if err != nil {
						results <- JobResult{job: j, err: err, output: nil}
						return
					}
				}
			}
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
	fmt.Println("USAGE: tgwut <command> [flags]")
	fmt.Printf("Commands: %s\n", strings.Join(commandNames, ", "))
}

func captureTGWUTConfig() *TGWUTConfig {
	var err error
	config := &TGWUTConfig{}
	config.UserName = os.Getenv("TGWUT_USERNAME")
	config.Password = os.Getenv("TGWUT_PASSWORD")
	config.CloudTamerBaseURL = os.Getenv("CLOUDTAMER_BASE_URL")
	config.VPCConfBaseURL = os.Getenv("VPCCONF_BASE_URL")
	cloudTamerIDMSIDStr := os.Getenv("CLOUDTAMER_IDMS_ID")
	cloudTamerAdminGroupIDStr := os.Getenv("CLOUDTAMER_ADMIN_GROUP_ID")
	if config.UserName == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "TGWUT_USERNAME env variable required")
		os.Exit(exitCodeInvalidEnvironment)
	}
	if config.Password == "" {
		fmt.Fprintf(os.Stderr, "%s\n", "TGWUT_PASSWORD env variable required")
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

func loginToCloudTamer(config *TGWUTConfig) {
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

func main() {
	accountLocks = make(map[string]*sync.Mutex)
	cacheLock = make(map[string]*sync.Mutex)
	flag.Usage = listCommands
	dryRun := flag.Bool("n", false, "do not write to resources")
	limitVPC := flag.String("vpc", "", "limit to specific vpc")
	limitStack := flag.String("stack", "", "limit to specific stack")
	region := flag.String("region", regionEast, "specify region to work in")
	limitAccount := flag.String("account", "", "limit to specific account ID")
	includeShared := flag.Bool("shared", false, "include shared stack vpcs")
	skipMissing := flag.Bool("skip-missing", false, "skip missing primary routes")
	limitCIDR := flag.String("cidr", "", "limit to a specific cidr block")
	maxConcurrency := flag.Int("threads", 1, "run more threads")
	greenfieldFlag := flag.Bool("greenfield", false, "refer to greenfield tgw config")
	quietFlag := flag.Bool("quiet", false, "suppress some extra info")
	testEndpoint := flag.String("endpoint", defaultTestEndpoint, "endpoint to test")
	usePrivilegedRole := flag.Bool("rw", false, "use privileged roles")
	flag.Parse()

	currentRegion = database.Region(*region)
	if currentRegion != regionAll {
		log.Printf("Limiting region to %s\n", currentRegion)
	}
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
	if *dryRun {
		log.Println("DRY RUN ENABLED")
	}
	cache = initRedis(redisHost)

	tgwutConfig := captureTGWUTConfig()
	loginToCloudTamer(tgwutConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
				log.Printf("Shutting down TGWut")
				cancel()
			}
		}
	}()
	defer cancel()

	api := &VPCConfAPI{
		Username: tgwutConfig.UserName,
		Password: tgwutConfig.Password,
		BaseURL:  tgwutConfig.VPCConfBaseURL,
	}

	config := tgwInfo{}
	if *greenfieldFlag {
		log.Println("TARGETING GREENFIELD")
		config = centralInfo[versionGreenfield][currentRegion]
	} else {
		log.Println("TARGETING LEGACY")
		config = centralInfo[versionLegacy][currentRegion]
	}

	// Create worker pool
	jobs = make(chan Job, *maxConcurrency)
	results = make(chan JobResult, *maxConcurrency)

	exceptionVPCList, err := api.FetchExceptionalVPCs()
	if err != nil {
		log.Printf("Error fetching exception list: %s\n", err)
	}

	processedList := []string{}
	processedByStack := make(map[string][]string)
	jobData := make(map[string]JobResult)

	// Iterate across accounts (to walk all infrastructure)
	if stringInSlice(command, accountCommands) {
		go func() {
			accounts, err := creds.GetAuthorizedAccounts()
			if err != nil {
				log.Fatalf("Error fetching authorized accounts: %s\n", err)
			}
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
			}
			if accountDetails == nil {
				log.Println("unable to fetch account details")
				return
			}
			for _, account := range accounts {
				accountLocks[account.ID] = &sync.Mutex{}
				cacheLock[account.ID] = &sync.Mutex{}
			}
			for _, account := range accounts {
				if OUHierarchy.ProjectIsInOU(account.ProjectID, DECOMISSIONED_OU) {
					log.Printf("Skipping disabled account %s (%s)\n", account.ID, account.Name)
					continue
				}
				if *limitAccount != "" && (*limitAccount != account.ID) {
					continue
				}
				for _, r := range regions {
					if _, ok := accountDetails[account.ID]; !ok {
						log.Printf("Unknown account (not in vpc-conf): %s\n", account.ID)
						//end
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
							VPC:               vpcInfo{},
							dryRun:            *dryRun,
							testEndpoint:      *testEndpoint,
							config:            config,
							api:               api,
							ctx:               ctx,
							exceptionVPCList:  exceptionVPCList,
							shared:            *includeShared,
							version:           versionLegacy,
							quiet:             *quietFlag,
							routeTables:       map[string]*ec2.RouteTable{},
							ips:               map[string]*ip{},
							limitVPC:          *limitVPC,
							filterCIDR:        *limitCIDR,
							skipMissing:       *skipMissing,
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

			generateManagedRouteLists(database.Region(region))

			queueVPC := func(VPCInfo *vpcInfo) {
				queue(command, &task{
					VPC:               *VPCInfo,
					APIRegion:         aws.String(string(VPCInfo.Region)),
					dryRun:            *dryRun,
					testEndpoint:      *testEndpoint,
					config:            config,
					api:               api,
					ctx:               ctx,
					exceptionVPCList:  exceptionVPCList,
					shared:            *includeShared,
					version:           versionLegacy,
					quiet:             *quietFlag,
					routeTables:       map[string]*ec2.RouteTable{},
					ips:               map[string]*ip{},
					filterCIDR:        *limitCIDR,
					limitVPC:          *limitVPC,
					skipMissing:       *skipMissing,
					usePrivilegedRole: *usePrivilegedRole,
					args:              args,
				})
			}
			go func(region database.Region) {
				// We always want to only worry about VPCs currently connected to the legacy TGW
				vpcs := getLegacyVPCs(region)
				accountLocks = map[string]*sync.Mutex{}
				cacheLock = map[string]*sync.Mutex{}
				for _, vpc := range vpcs {
					accountLocks[vpc.AccountID] = &sync.Mutex{}
					cacheLock[vpc.AccountID] = &sync.Mutex{}
				}
				for _, vpc := range vpcs {
					VPCInfo := getVPC(vpc.ID)
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
							if VPCInfo == nil {
								log.Fatalf("Unable to filter by cidr, missing data for vpc: %s\n", vpc.ID)
							}
							if len(VPCInfo.CIDRs) == 0 {
								cidrs, err := getCIDRsForVPC(vpc.AccountID, vpc.ID, region)
								if err != nil {
									log.Printf("Error fetching CIDRs: %s\n", err)
									continue
								}
								VPCInfo.CIDRs = cidrs
							}
							found := false
							for _, cidr := range VPCInfo.CIDRs {
								if found, _ = cidrIsWithin(cidr, *limitCIDR); found {
									break
								}
							}
							if !found {
								continue
							}
						}
					}
					if VPCInfo == nil {
						VPCInfo = &vpcInfo{
							VPC: vpc,
						}
					}
					queueVPC(VPCInfo)
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
						VPCInfo := &vpcInfo{
							VPC: vpc,
						}
						queueVPC(VPCInfo)
						processedList = append(processedList, vpc.ID)
					}
				}
				close(jobs)
			}(database.Region(region))
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
	if command == "attach-templates" {
		for stackString, vpcs := range processedByStack {
			stack := stack(stackString)
			vpcsByTemplateId := make(map[uint64][]string)
			for _, t := range []string{"base"} {
				if _, ok := mtgaTemplatesByStack[stack][t]; !ok {
					log.Printf("ERROR: %s has no %s template defined\n", t, stackString)
				}
			}
			for _, vpc := range vpcs {
				id := mtgaTemplatesByStack[stack]["base"]
				vpcsByTemplateId[id] = append(vpcsByTemplateId[id], vpc)
			}
			for id, vpcs := range vpcsByTemplateId {
				log.Printf("Prepare batch request for stack %s (%d VPCs)\n", stack, len(vpcs))
				request := BatchTaskRequest{
					TaskTypes:                           1,
					AddManagedTransitGatewayAttachments: []uint64{id},
				}
				for _, vpc := range vpcs {
					if stringInSlice(vpc, exceptionVPCList) {
						log.Printf("VPC %s is exception, SKIP!\n", vpc)
						continue
					}
					if jobData[vpc].err != nil {
						log.Printf("Job for %s has err: %s\n", vpc, jobData[vpc].err)
						continue
					}
					v := jobData[vpc].job.task.VPC
					if v.Config != nil && v.Config.ManagedTransitGatewayAttachmentIDs != nil {
						if uint64InSlice(id, v.Config.ManagedTransitGatewayAttachmentIDs) {
							log.Printf("VPC %s already has TGW template id %d attached, SKIP!\n", vpc, id)
							continue
						}
					}
					vs := struct {
						ID, Region string
					}{
						ID:     vpc,
						Region: *region,
					}
					request.VPCs = append(request.VPCs, vs)
				}
				if len(request.VPCs) < 1 {
					log.Printf("No VPCs missing managed TGW attachment config, skip this attachment template id (%d)\n", id)
				} else {
					// Make sure we set the appropriate template IDs
					err := api.SubmitBatchTask(request, *dryRun)
					if err != nil {
						log.Printf("Error submitting batch task request: %s\n", err)
					}
				}
			}
		}
	}
	entity := "vpcs"
	if stringInSlice(command, accountCommands) {
		entity = "accounts"
	}
	for context, jd := range jobData {
		if len(jd.output) > 0 {
			for _, line := range jd.output {
				log.Printf("[%s] %s\n", context, line)
			}
		}
	}
	log.Printf("Task complete: %d %s processed: %s\n", len(processedList), entity, strings.Join(processedList, ", "))
}
