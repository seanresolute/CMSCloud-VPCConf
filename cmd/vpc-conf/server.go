package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/apikey"
	awsp "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/aws"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/awscreds"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/azure"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cachedcredentials"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmsnet"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/credentialservice"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/ipcontrol"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/jira"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/lib"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/orchestration"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/search"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/session"
	"github.com/lib/pq"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/sts"
)

type taskAndLockSet struct {
	task    *database.Task
	lockSet database.LockSet
}

type IssueLabels struct {
	NewRequest     string
	NewSubnets     string
	ManualApproval string
}

type updateNetworkFirewallRequest struct {
	AddNetworkFirewall bool
}
type Server struct {
	APIKey         *apikey.APIKey
	apiKeyRWMu     sync.RWMutex
	apiKeySessions map[string]string
	AzureAD        *azure.AzureAD
	IPAM           client.Client
	CMSNetConfig   cmsnet.Config
	// for when you need fresh credentials
	CredentialService *credentialservice.CredentialService
	// for when you want to use the user's cache
	CachedCredentials *cachedcredentials.CachedCredentials
	PathPrefix        string
	*database.TaskDatabase
	session.SessionStore
	JIRAClient           *jira.Client
	JIRAIssueLabels      IssueLabels
	ModelsManager        database.ModelsManager
	TaskParallelism      int
	LimitToAWSAccountIDs []string              // nil means "all accounts allowed"
	Orchestration        *orchestration.Client // optional

	ReparseTemplates bool

	stopNow         chan struct{} // channel closes when "stop" command sent
	shouldStop      bool          // true means stop command has been set
	taskMu          sync.Mutex    // gates access to shouldStop and tasksInProgress
	tasksInProgress []*taskAndLockSet
	checkForTasks   chan struct{}
	taskSlots       chan struct{}
}

func (s *Server) StopTaskQueue() {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.shouldStop = true
	if s.stopNow != nil {
		close(s.stopNow)
		s.stopNow = nil
	}
}

func (s *Server) FailTasksNow() {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	s.shouldStop = true
	if s.stopNow != nil {
		close(s.stopNow)
		s.stopNow = nil
	}
	for _, tls := range s.tasksInProgress {
		err := tls.task.Fail("Server is shutting down")
		if err != nil {
			log.Printf("Error failing task: %s", err)
		}
		err = s.TaskDatabase.ReleaseTask(tls.task.ID)
		if err != nil {
			log.Printf("Error releasing task: %s", err)
		}
		tls.lockSet.ReleaseAll()
	}
	s.tasksInProgress = nil
}

func (s *Server) createJIRAIssue(requestID uint64) error {
	req, tx, err := s.ModelsManager.LockVPCRequest(requestID)
	if err != nil {
		return fmt.Errorf("Error locking VPC Request: %s", err)
	}
	committed := false
	defer func() {
		if !committed {
			err := tx.Rollback()
			if err != nil {
				log.Printf("Error rolling back: %s", err)
			}
		}
	}()
	// Check again now that we have the lock
	if req.JIRAIssue != nil {
		return nil
	}
	var summary string
	buf := new(bytes.Buffer)
	var issueLabels []string

	if req.RequestType == database.RequestTypeNewSubnet {
		summary = fmt.Sprintf("New Additional Subnets request for project %s: %s", req.ProjectName, req.RequestedConfig.VPCName)
		issueLabels = append(issueLabels, s.JIRAIssueLabels.NewRequest, s.JIRAIssueLabels.NewSubnets)
		err = tplAdditionalSubnetsRequestTicket.Execute(buf, req)
	} else if req.RequestType == database.RequestTypeNewVPC {
		summary = fmt.Sprintf("New VPC Request for project %s: %s", req.ProjectName, req.RequestedConfig.VPCName)
		issueLabels = append(issueLabels, s.JIRAIssueLabels.NewRequest)
		if !req.RequestedConfig.AutoProvision {
			issueLabels = append(issueLabels, s.JIRAIssueLabels.ManualApproval)
		}
		err = tplVPCRequestTicket.Execute(buf, req)
	} else {
		return fmt.Errorf("Invalid request type: %q", req.RequestType)
	}

	if err != nil {
		return fmt.Errorf("Error preparing JIRA issue description: %s", err)
	}

	issueID, err := s.JIRAClient.CreateIssue(&jira.IssueDetails{
		Summary:     summary,
		Description: buf.String(),
		Reporter:    req.RequesterUID,
		Labels:      issueLabels,
	})
	if err != nil {
		// Done outside the transaction, but that's okay
		err = s.ModelsManager.VPCRequestLogInsert(req.ID, err.Error())
		if err != nil {
			log.Printf("Error inserting request log: %s", err)
		}
		return fmt.Errorf("Error creating JIRA issue: %s", err)
	}

	err = tx.SetVPCRequestJIRAIssue(issueID)
	if err != nil {
		return fmt.Errorf("Error setting JIRA issue %s: %s", issueID, err)
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("Error committing: %s", err)
	}
	committed = true
	return nil
}

func (s *Server) SyncVPCRequestStatuses() {
	sync := func() {
		reqs, err := s.ModelsManager.GetAllVPCRequests()
		if err != nil {
			log.Printf("Error getting VPC requests: %s", err)
			return
		}
		for _, req := range reqs {
			if req.JIRAIssue == nil {
				err := s.createJIRAIssue(req.ID)
				if err != nil {
					log.Printf("Error creating JIRA issue: %s", err)
				}
			} else if req.TaskID == nil {
				// skip old JIRA project and cancelled/denied tasks
				jiraIssue := *req.JIRAIssue
				if strings.HasPrefix(jiraIssue, s.JIRAClient.Config.Project+"-") && req.Status != database.StatusCancelledByRequester && req.Status != database.StatusRejected {
					// Check for approval
					status, err := s.JIRAClient.GetIssueStatus(jiraIssue)
					if err != nil {
						log.Printf("Error getting status for %q: %s", jiraIssue, err)
					} else if status != req.Status {
						s.ModelsManager.SetVPCRequestStatus(req.ID, status)
					}
				}
			}
		}
	}
	go func() {
		sync()
		for range time.Tick(time.Minute) {
			sync()
		}
	}()
}

func (s *Server) suggestCheckForTasks() {
	select {
	case s.checkForTasks <- struct{}{}:
	default:
	}
}

func (s *Server) listenForNewTasks(postgresConnectionString string) {
	s.checkForTasks = make(chan struct{}, 1)
	listener := pq.NewListener(postgresConnectionString, time.Second, 30*time.Second, func(t pq.ListenerEventType, err error) {
		eventType := map[pq.ListenerEventType]string{
			pq.ListenerEventConnected:               "connected",
			pq.ListenerEventDisconnected:            "disconnected",
			pq.ListenerEventReconnected:             "reconnected",
			pq.ListenerEventConnectionAttemptFailed: "connection failed",
		}[t]
		if eventType == "" {
			eventType = fmt.Sprintf("Unknown: (%d)", t)
		}
		log.Printf("Postgres listener: got event type %q with err %v", eventType, err)
	})
	go func() {
		channelName := "new_task"
		err := listener.Listen(channelName)
		if err != nil {
			log.Fatalf("Error listening to %q channel: %s", channelName, err)
		}
		for {
			select {
			case n := <-listener.Notify:
				if n == nil {
					// This happens after reconnecting
					log.Printf("received unknown notification from postgres")
				} else {
					log.Printf("received notification %q from postgres", n.Extra)
				}
				s.suggestCheckForTasks()
			case <-time.After(60 * time.Second):
				go listener.Ping() // make the client library notice if the connection dropped
			}
		}
	}()
}

func (s *Server) performNextTask() (bool, error) {
	t, lockSet, err := s.TaskDatabase.ReserveNextQueuedTask(s.ModelsManager)
	if err != nil {
		return false, err
	}
	if t == nil {
		return false, nil
	}
	s.taskMu.Lock()
	tls := &taskAndLockSet{
		task: t, lockSet: lockSet,
	}
	s.tasksInProgress = append(s.tasksInProgress, tls)
	s.taskMu.Unlock()

	go func() {
		(func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Unexpected error: %s", r)
					debug.PrintStack()
					err := t.Fail("Unexpected error: %s", r)
					if err != nil {
						log.Printf("Error failing task: %s", err)
					}
				}
			}()
			s.performTask(t, lockSet)
		})()
		s.taskMu.Lock()
		log.Printf("Finished task %d", tls.task.ID)
		for idx, tls2 := range s.tasksInProgress {
			if tls == tls2 {
				s.tasksInProgress = append(s.tasksInProgress[:idx], s.tasksInProgress[idx+1:]...)
				break
			}
		}
		log.Printf("%d tasks in progress", len(s.tasksInProgress))
		s.taskMu.Unlock()
		err = s.TaskDatabase.ReleaseTask(t.ID)
		if err != nil {
			log.Printf("Error releasing task: %s", err)
		}
		lockSet.ReleaseAll()
		// Return the task slot
		s.taskSlots <- struct{}{}
	}()
	return true, nil
}

// Closes returned channel when it has stopped because of StopTaskQueue()
func (s *Server) DoTasks() chan struct{} {
	done := make(chan struct{})
	s.stopNow = make(chan struct{})
	s.taskSlots = make(chan struct{}, s.TaskParallelism)
	for i := 0; i < s.TaskParallelism; i++ {
		s.taskSlots <- struct{}{}
	}
	go func() {
		for {
			<-s.taskSlots // grab a free task slot
			s.taskMu.Lock()
			stop := s.shouldStop
			s.taskMu.Unlock()
			if stop {
				s.taskSlots <- struct{}{}
				break
			}
			foundTask, err := s.performNextTask()
			if !foundTask {
				// Return the task slot
				s.taskSlots <- struct{}{}
			}
			if err != nil {
				log.Printf("Error getting next task: %s", err)
				time.Sleep(time.Second)
				continue
			}
			if !foundTask {
				// Wait until we get a notification that there might be a task available.
				select {
				case <-s.checkForTasks:
				case <-s.stopNow:
				}
			}
		}
		// Wait for all workers to return their slots
		log.Printf("Waiting for workers to finish")
		for i := 0; i < s.TaskParallelism; i++ {
			<-s.taskSlots
			log.Printf("%d workers done", i+1)
		}
		close(done)
	}()
	return done
}

type CMSNetConnection struct {
	CIDR, VRF   string
	Status      string
	LastMessage string
}
type CMSNetNAT struct {
	InsideNetwork, OutsideNetwork string
	Status                        string
	LastMessage                   string
}
type SubnetInfo struct {
	SubnetID                                  string
	Name                                      string
	GroupName                                 string
	CIDR                                      string
	InPrimaryCIDR                             bool
	IsManaged                                 bool
	Type                                      string
	IsConnectedToInternet                     bool
	ConnectedManagedTransitGatewayAttachments []uint64
	CMSNetConnections                         []*CMSNetConnection
	CMSNetNATs                                []*CMSNetNAT
}
type VPCConfig struct {
	ConnectPublic                      bool
	ConnectPrivate                     bool
	ManagedTransitGatewayAttachmentIDs []uint64
	ManagedResolverRuleSetIDs          []uint64
	SecurityGroupSetIDs                []uint64
	PeeringConnections                 []*database.PeeringConnectionConfig
}
type VPCInfo struct {
	AccountID   string
	AccountName string
	ProjectName string
	IsGovCloud  bool
	IsLegacy    bool
	VPCType     *database.VPCType

	VPCID              string
	Name               string
	AvailabilityZones  map[string]string
	Stack              string
	PrimaryCIDR        string
	SecondaryCIDRs     []string
	IsDefault          bool
	IsAutomated        bool
	IsMissing          bool
	IsException        bool
	Subnets            []SubnetInfo
	Tasks              []TaskInfo
	IsMoreTasks        bool
	Issues             []*database.Issue
	Config             VPCConfig
	SubnetGroups       []SubnetGroupInfo
	CMSNetSupported    bool
	CMSNetError        string
	CustomPublicRoutes string
}
type SubnetGroupInfo struct {
	Name       string
	SubnetType database.SubnetType
}
type BatchTaskInfo struct {
	ID          uint64
	Description string
	Tasks       []TaskInfo
	AddedAt     time.Time
}
type TaskInfo struct {
	ID          uint64
	AccountID   string
	VPCID       *string
	VPCRegion   *string
	Description string
	Status      string
}
type TransitGatewayInfo struct {
	TransitGatewayID   string
	TransitGatewayName string
	ResourceShareID    string
	ResourceShareName  string
}
type RegionInfo struct {
	Name            string
	VPCs            []VPCInfo
	TransitGateways []TransitGatewayInfo
}
type AccountPageInfo struct {
	AccountID   string
	AccountName string
	ProjectName string
	IsGovCloud  bool
	Regions     []RegionInfo
	Tasks       []TaskInfo
	IsMoreTasks bool

	DefaultRegion string
	ServerPrefix  string
}
type PrefixListInfo struct {
	Name string
	ID   string
}

const prefixListAccountIDCommercial = "921617238787"
const prefixListAccountIDGovCloud = "849804945443"

var invalidLabelCharacters = regexp.MustCompile("([^a-z0-9 ]+)")

type route struct {
	regexp       *regexp.Regexp
	handler      *func(*Server, http.ResponseWriter, *http.Request, ...string)
	method       string
	requiresAuth bool
}

var routes = []*route{
	{
		regexp:       regexp.MustCompile(`^dashboard.json$`),
		handler:      &handleDashboard,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^ipusage.json$`),
		handler:      &handleIPUsageList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^ipusage/refresh$`),
		handler:      &handleRefreshIPUsage,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/accounts.json$`),
		handler:      &handleAccountList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+).json$`),
		handler:      &handleAccountDetails,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+)/console$`),
		handler:      &handleConsoleLogin,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+)/creds$`),
		handler:      &handleCreds,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^accounts/([^/]+)$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^accounts$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:       regexp.MustCompile(`^task/([^/]+)/([0-9]+).json$`),
		handler:      &handleAccountTask,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^task/([0-9]+).json$`),
		handler:      &handleGetTask,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^task/([^/]+)(?:/([^/]+))?/$`),
		handler:      &handleTasks,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^task/cancel$`),
		handler:      &handleCancelTasks,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^task/([^/]+)/([^/]+)/([0-9]+).json$`),
		handler:      &handleVPCTask,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^subtasks/([^/]+)/([^/]+).json$`),
		handler:      &handleVPCLastSubTasks,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)$`),
		handler:      &handleDeleteVPC,
		method:       http.MethodDelete,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/network$`),
		handler:      &handleVPCNetwork,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/flowlogs$`),
		handler:      &handleVPCFlowLogs,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/sgs$`),
		handler:      &handleVPCSecurityGroups,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/resolverRules$`),
		handler:      &handleVPCResolverRules,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/networkFirewall$`),
		handler:      &handleVPCNetworkFirewall,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/import$`),
		handler:      &handleVPCImport,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/except$`),
		handler:      &handleVPCEstablishException,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/unimport$`),
		handler:      &handleVPCUnimport,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/repair$`),
		handler:      &handleVPCRepair,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/addAvailabilityZone$`),
		handler:      &handleAddAvailabilityZone,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/removeAvailabilityZone$`),
		handler:      &handleRemoveAvailabilityZone,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/addZonedSubnets$`),
		handler:      &handleAddZonedSubnets,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/removeZonedSubnets$`),
		handler:      &handleRemoveZonedSubnets,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/connectCMSNet$`),
		handler:      &handleConnectCMSNet,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/disconnectCMSNet$`),
		handler:      &handleDisconnectCMSNet,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/addCMSNetNAT$`),
		handler:      &handleAddCMSNetNAT,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/deleteCMSNetNAT$`),
		handler:      &handleDeleteCMSNetNAT,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/syncRouteState$`),
		handler:      &handleSyncRouteState,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/renameVPC$`),
		handler:      &handleRenameVPC,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/verify$`),
		handler:      &handleVPCVerify,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/$`),
		handler:      &handleNewVPC,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+)/vpc/([^/]+)/([^/]+).json$`),
		handler:      &handleVPCDetails,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+)/vpc/([^/]+)/([^/]+)/state.json$`),
		handler:      &handleGetVPCState,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^accounts/([^/]+)/user.json$`),
		handler:      &handleUserDetails,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^exception.json$`),
		handler:      &handleExceptionVPCList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/vpc/([^/]+)/([^/]+)/tgas.json$`),
		handler:      &handleVPCTransitGatewayAttachments,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/rs/([^/]+)/([^/]+)/$`),
		handler:      &handleSetResourceShare,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/rs/([^/]+)/([^/]+)/$`),
		handler:      &handleDeleteResourceShare,
		method:       http.MethodDelete,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^mtgas$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^sgs$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^mrrs$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^batch$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^usage$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:       regexp.MustCompile(`^batch$`),
		handler:      &handleBatchTask,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^batch/task/$`),
		handler:      &handleBatchTasks,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^batch/task/([0-9]+)$`),
		handler:      &handleBatchTaskByID,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^batch/vpcs.json$`),
		handler:      &handleListVPCs,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^batch/labels.json$`),
		handler:      &handleListBatchVPCLabels,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels.json$`),
		handler:      &handleGetLabels,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)/([^/]+)$`),
		handler:      &handleGetVPCLabels,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)/([^/]+)/([^/]+)$`),
		handler:      &handleSetVPCLabel,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)/([^/]+)/([^/]+)$`),
		handler:      &handleDeleteVPCLabel,
		method:       http.MethodDelete,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)$`),
		handler:      &handleGetAccountLabels,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)/([^/]+)$`),
		handler:      &handleSetAccountLabel,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^labels/([^/]+)/([^/]+)$`),
		handler:      &handleDeleteAccountLabel,
		method:       http.MethodDelete,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^mtgas/$`),
		handler: &handleCreateManagedTransitGatewayAttachment,
		method:  http.MethodPost,
	},
	{
		regexp:  regexp.MustCompile(`^mtgas/([0-9]+)$`),
		handler: &handleUpdateManagedTransitGatewayAttachment,
		method:  http.MethodPatch,
	},
	{
		regexp:  regexp.MustCompile(`^mtgas/([0-9]+)$`),
		handler: &handleDeleteManagedTransitGatewayAttachment,
		method:  http.MethodDelete,
	},
	{
		regexp:       regexp.MustCompile(`^mtgas.json$`),
		handler:      &handleManagedTransitGatewayAttachmentList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^mrrs/$`),
		handler: &handleCreateManagedResolverRuleSet,
		method:  http.MethodPost,
	},
	{
		regexp:  regexp.MustCompile(`^mrrs/([0-9]+)$`),
		handler: &handleUpdateManagedResolverRuleSet,
		method:  http.MethodPatch,
	},
	{
		regexp:  regexp.MustCompile(`^mrrs/([0-9]+)$`),
		handler: &handleDeleteManagedResolverRuleSet,
		method:  http.MethodDelete,
	},
	{
		regexp:       regexp.MustCompile(`^mrrs.json$`),
		handler:      &handleManagedResolverRuleSetList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^sgs/$`),
		handler: &handleCreateSecurityGroupSet,
		method:  http.MethodPost,
	},
	{
		regexp:  regexp.MustCompile(`^sgs/([0-9]+)$`),
		handler: &handleUpdateSecurityGroupSet,
		method:  http.MethodPatch,
	},
	{
		regexp:  regexp.MustCompile(`^sgs/([0-9]+)$`),
		handler: &handleDeleteSecurityGroupSet,
		method:  http.MethodDelete,
	},
	{
		regexp:       regexp.MustCompile(`^sgs.json$`),
		handler:      &handleSecurityGroupSetList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/pl.json$`),
		handler:      &handleListManagedPrefixLists,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^([^/]+)/pl/([^/]+).json$`),
		handler:      &handleManagedPrefixListDetails,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^accounts/([^/]+)/vpc/([^/]+)/([^/]+)$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^vpcreqs/?([0-9]+)?$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:       regexp.MustCompile(`^vpcreq/([0-9]+)/provision$`),
		handler:      &handleProvisionRequest,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^vpcreqs\.json$`),
		handler:      &handleVPCRequestList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^vpcreqs/([0-9]+)\.json$`),
		handler:      &handleGetVPCRequest,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^vpcreqs/([0-9]+)/jiraErrors`), // [0-9] is the VPCRequest.ID
		handler:      &handleVPCRequestJIRAIssueErrorList,
		method:       http.MethodGet,
		requiresAuth: true,
	},
	{
		regexp:       regexp.MustCompile(`^allowWorkers$`),
		handler:      &handleAllowWorkers,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^logout$`),
		handler: &handleLogout,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^search$`),
		handler: &handleIndex,
		method:  http.MethodGet,
	},
	{
		regexp:       regexp.MustCompile(`^search/do$`),
		handler:      &handleSearch,
		method:       http.MethodPost,
		requiresAuth: true,
	},
	{
		regexp:  regexp.MustCompile(`^oauth/callback$`),
		handler: &handleOauthCallback,
		method:  http.MethodGet,
	},
	{
		regexp:  regexp.MustCompile(`^oauth/validate$`),
		handler: &handleOauthValidate,
		method:  http.MethodPost,
	},
}

func setStatus(t database.TaskInterface, s database.TaskStatus) {
	err := t.SetStatus(s)
	if err != nil {
		log.Printf("error setting status: %s", err)
	}
}

var handleAllowWorkers = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleAllowWorkers but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	r.ParseForm()
	allowAll := r.FormValue("allowAll") == "1"
	nameSpecified := len(r.Form["allow"]) != 0
	if allowAll && nameSpecified {
		http.Error(w, `Cannot specify both "allow" and "allowAll"`, http.StatusBadRequest)
		return
	}
	var err error
	if allowAll {
		err = s.TaskDatabase.AllowAllWorkers()
	} else if !nameSpecified {
		err = s.TaskDatabase.AllowNoWorkers()
	} else {
		err = s.TaskDatabase.AllowOnlyWorkersWithName(r.FormValue("allow"))
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting value: %s", err), http.StatusInternalServerError)
		return
	}
}

var handleDeleteResourceShare = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleDeleteResourceShare but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	transitGatewayID := args[2]

	err := s.ModelsManager.DeleteTransitGatewayResourceShare(database.Region(region), transitGatewayID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting resource share: %s", err), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

var handleSetResourceShare = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleSetResourceShare but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	transitGatewayID := args[2]

	resourceShareID := r.FormValue("ResourceShareID")
	if resourceShareID == "" {
		http.Error(w, "ResourceShareID is required", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(resourceShareID, "rs-") {
		http.Error(w, "Resource Share ID must begin with \"rs-\"", http.StatusBadRequest)
		return
	}

	asUser := s.getSession(r).Username
	sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ramsvc := ram.New(sess)

	shareARN := resourceShareARN(region, accountID, resourceShareID)
	out, err := ramsvc.ListResources(&ram.ListResourcesInput{
		ResourceShareArns: []*string{aws.String(shareARN)},
		ResourceOwner:     aws.String("SELF"),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error describing resource share: %s", err), http.StatusInternalServerError)
		return
	}

	tgwARN := transitGatewayARN(region, accountID, transitGatewayID)
	found := false
	for _, resource := range out.Resources {
		if *resource.Arn == tgwARN {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, fmt.Sprintf("Resource share %s does not include resource %s. You must add the Transit Gateway to the Resource Share in the AWS Console first.", shareARN, tgwARN), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.CreateOrUpdateTransitGatewayResourceShare(&database.TransitGatewayResourceShare{
		Region:           database.Region(region),
		TransitGatewayID: transitGatewayID,
		ResourceShareID:  resourceShareID,
		AccountID:        accountID,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting resource share: %s", err), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

var handleDeleteVPC = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleDeleteVPC but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	taskData := &database.TaskData{
		DeleteVPCTaskData: &database.DeleteVPCTaskData{
			AccountID: accountID,
			Region:    database.Region(region),
			VPCID:     vpcID,
		},
		AsUser: s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddAccountTask(accountID, "Delete VPC "+vpcID, taskBytes, database.TaskStatusQueued)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleNewVPC = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleNewVPC but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]

	allocateConfig := new(database.AllocateConfig)
	err := json.NewDecoder(r.Body).Decode(allocateConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	allocateConfig.AWSRegion = region

	taskData := &database.TaskData{
		CreateVPCTaskData: &database.CreateVPCTaskData{
			AllocateConfig: *allocateConfig,
		},
		AsUser: s.getSession(r).Username,
	}

	taskData.CreateVPCTaskData.JIRAIssueForComment = r.URL.Query().Get("issue")

	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddAccountTask(allocateConfig.AccountID, "Create new VPC "+allocateConfig.VPCName, taskBytes, database.TaskStatusQueued)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleTasks = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleTasks but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]
	vpcID := args[1]
	beforeID := uint64(0)
	beforeIDStr := r.FormValue("beforeID")
	if beforeIDStr != "" {
		n, err := strconv.Atoi(beforeIDStr)
		if err == nil && n > 0 {
			beforeID = uint64(n)
		}
	}

	var tasks []*database.Task
	var isMore bool
	var err error
	if vpcID == "" {
		if beforeID > 0 {
			tasks, isMore, err = s.TaskDatabase.GetAccountTasksBefore(accountID, beforeID)
		} else {
			tasks, isMore, err = s.TaskDatabase.GetAccountTasks(accountID)
		}
	} else {
		if beforeID > 0 {
			tasks, isMore, err = s.TaskDatabase.GetVPCTasksBefore(accountID, vpcID, beforeID)
		} else {
			tasks, isMore, err = s.TaskDatabase.GetVPCTasks(accountID, vpcID)
		}
	}
	result := &struct {
		IsMoreTasks bool
		Tasks       []TaskInfo
	}{}

	result.IsMoreTasks = isMore
	if err != nil {
		log.Printf("Error getting tasks: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, t := range tasks {
		result.Tasks = append(result.Tasks, TaskInfo{
			ID:          t.ID,
			AccountID:   t.AccountID,
			VPCID:       t.VPCID,
			VPCRegion:   t.VPCRegion,
			Description: t.Description,
			Status:      t.Status.String(),
		})
	}
	buf, err := json.Marshal(result)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleGetTask = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleGetTask but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	var t *database.Task
	taskIdx, err := strconv.ParseUint(args[0], 10, 64)
	if err == nil {
		t, err = s.TaskDatabase.GetTask(taskIdx)
		if err != nil {
			t = nil
			log.Printf("Error getting task %d: %s", taskIdx, err)
		}
	}
	if t == nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}
	data := struct {
		ID          uint64
		Description string
		Status      database.TaskStatus
		Log         []*database.LogEntry
	}{
		ID:          t.ID,
		Description: t.Description,
		Status:      t.Status,
		Log:         t.LogEntries(),
	}
	buf, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

// deprecated; use handleGetTask
var handleAccountTask = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleAccountTask but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	handleGetTask(s, w, r, args[1])
}

// deprecated; use handleGetTask
var handleVPCTask = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional args to handleVPCTask but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	handleGetTask(s, w, r, args[2])
}

func (s *Server) getRegions(isGovCloud bool) []string {
	if isGovCloud {
		return []string{"us-gov-west-1"}
	} else {
		return []string{"us-west-2", "us-east-1"}
	}
}

func (s *Server) getPreferredRegions(r *http.Request, accountID string) ([]string, error) {
	for _, account := range s.getSession(r).AuthorizedAccounts {
		if account.ID == accountID {
			return s.getRegions(account.IsGovCloud), nil
		}
	}
	return nil, fmt.Errorf("Not authorized for account %s", accountID)
}

func arnPrefix(region string) string {
	if strings.HasPrefix(region, "us-gov") {
		return "arn:aws-us-gov"
	}
	return "arn:aws"
}

func resolverRuleARN(region, accountID, resolverRuleID string) string {
	return fmt.Sprintf("%s:route53resolver:%s:%s:resolver-rule/%s", arnPrefix(region), region, accountID, resolverRuleID)
}

func resourceShareARN(region, accountID, resourceShareID string) string {
	resourceShareID = strings.TrimPrefix(resourceShareID, "rs-")
	return fmt.Sprintf("%s:ram:%s:%s:resource-share/%s", arnPrefix(region), region, accountID, resourceShareID)
}

func transitGatewayARN(region, accountID, transitGatewayID string) string {
	return fmt.Sprintf("%s:ec2:%s:%s:transit-gateway/%s", arnPrefix(region), region, accountID, transitGatewayID)
}

func prefixListARN(region database.Region, accountID, prefixListID string) string {
	return fmt.Sprintf("%s:ec2:%s:%s:prefix-list/%s", arnPrefix(string(region)), region, accountID, prefixListID)
}

func roleARN(region database.Region, accountID, roleName string) string {
	return fmt.Sprintf("%s:iam::%s:role/%s", arnPrefix(string(region)), accountID, roleName)
}

var handleListVPCs = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected no additional args to handleListVPCs but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcs, err := s.ModelsManager.ListAutomatedVPCs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPCs from database: %s", err), http.StatusInternalServerError)
		return
	}

	vpcTypes := database.GetVPCTypes()
	vpcTypesResponse := []struct {
		Name         string
		ID           int
		IsVerifiable bool
		IsRepairable bool
	}{}
	for _, vpcType := range vpcTypes {
		// Skip migrating VPC types since they are only temporarily in this state
		if !vpcType.IsMigrating() {
			vpcTypesResponse = append(vpcTypesResponse, struct {
				Name         string
				ID           int
				IsVerifiable bool
				IsRepairable bool
			}{
				Name:         vpcType.String(),
				ID:           int(vpcType),
				IsVerifiable: vpcType.CanVerifyVPC(),
				IsRepairable: vpcType.CanRepairVPC(),
			})
		}
	}

	buf, err := json.Marshal(map[string]interface{}{
		"VPCs":     vpcs,
		"Regions":  append(s.getRegions(false), s.getRegions(true)...),
		"VPCTypes": vpcTypesResponse,
	})
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleListBatchVPCLabels = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected no additional args to handleListBatchVPCLabels but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcLabels, err := s.ModelsManager.ListBatchVPCLabels()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC Labels from database: %s", err), http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(vpcLabels)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleAccountDetails = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleAccount but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]
	account, err := s.ModelsManager.GetAccount(accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading account from database: %s", err), http.StatusInternalServerError)
		return
	}

	regions, err := s.getPreferredRegions(r, accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting region list: %s", err), http.StatusInternalServerError)
		return
	}

	defaultRegion := regions[0]

	accountPageInfo := &AccountPageInfo{
		AccountID:     accountID,
		AccountName:   account.Name,
		ProjectName:   account.ProjectName,
		IsGovCloud:    account.IsGovCloud,
		ServerPrefix:  s.PathPrefix,
		DefaultRegion: defaultRegion,
	}
	tasks, isMore, err := s.TaskDatabase.GetAccountTasks(accountID)
	accountPageInfo.IsMoreTasks = isMore
	if err != nil {
		log.Printf("Error getting tasks: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, t := range tasks {
		accountPageInfo.Tasks = append(accountPageInfo.Tasks, TaskInfo{
			ID:          t.ID,
			AccountID:   t.AccountID,
			VPCID:       t.VPCID,
			VPCRegion:   t.VPCRegion,
			Description: t.Description,
			Status:      t.Status.String(),
		})
	}

	type regionResult struct {
		RegionInfo
		err error
	}

	infos := make(chan *regionResult, len(regions))

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	asUser := s.getSession(r).Username
	for _, region := range regions {
		go func(regionName string) {
			regionResult := &regionResult{
				RegionInfo: RegionInfo{
					Name: regionName,
				},
			}
			defer func() { infos <- regionResult }()

			sess, err := s.CachedCredentials.GetAWSSession(accountID, regionName, asUser)
			if err != nil {
				regionResult.err = fmt.Errorf("Error connecting to AWS: %s", err)
				return
			}
			ec2svc := ec2.New(sess)
			ramsvc := ram.New(sess)

			// TODO: may need multiple requests if there are many VPCs
			out, err := ec2svc.DescribeVpcsWithContext(ctx, &ec2.DescribeVpcsInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("owner-id"),
						Values: []*string{&accountID},
					},
				},
			})
			if err != nil {
				regionResult.err = fmt.Errorf("Error getting VPC list: %s", err)
				return
			}

			dbVPCs, err := s.ModelsManager.GetAutomatedVPCsForAccount(database.Region(regionName), accountID)
			if err != nil {
				regionResult.err = fmt.Errorf("Error loading VPCs for account: %s", err)
			}

			var found bool
			for _, dbVpc := range dbVPCs {
				found = false
				for _, awsVpc := range out.Vpcs {
					if aws.StringValue(awsVpc.VpcId) == dbVpc.ID {
						found = true
					}
				}
				if found {
					continue
				} else {
					vpcInfo := VPCInfo{
						VPCID:       dbVpc.ID,
						Name:        dbVpc.Name,
						IsAutomated: true,
						IsDefault:   false,
						IsMissing:   true,
						IsGovCloud:  account.IsGovCloud,
						VPCType:     &dbVpc.State.VPCType,
					}
					regionResult.RegionInfo.VPCs = append(regionResult.RegionInfo.VPCs, vpcInfo)
				}
			}

			for _, vpc := range out.Vpcs {
				name := ""
				for _, tag := range vpc.Tags {
					if *tag.Key == "Name" {
						name = *tag.Value
					}
				}
				vpcInfo := VPCInfo{
					VPCID:       *vpc.VpcId,
					Name:        name,
					IsAutomated: false,
					IsDefault:   *vpc.IsDefault,
					Issues:      make([]*database.Issue, 0),
					IsGovCloud:  account.IsGovCloud,
				}
				vpcModel, err := s.ModelsManager.GetVPC(database.Region(regionName), *vpc.VpcId)
				if err == database.ErrVPCNotFound {
					// no-op
				} else if err != nil {
					regionResult.err = fmt.Errorf("Error loading VPC state: %s", err)
					return
				} else {
					vpcInfo.Issues = vpcModel.Issues
					if vpcModel.State != nil {
						vpcInfo.VPCType = &vpcModel.State.VPCType
						if vpcModel.State.VPCType == database.VPCTypeException {
							vpcInfo.IsException = true
							vpcInfo.IsAutomated = false
						} else {
							vpcInfo.IsAutomated = true
						}
					}
					vpcInfo.Stack = vpcModel.Stack
				}
				regionResult.RegionInfo.VPCs = append(regionResult.RegionInfo.VPCs, vpcInfo)

			}

			tgOut, err := ec2svc.DescribeTransitGatewaysWithContext(ctx, &ec2.DescribeTransitGatewaysInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("owner-id"),
						Values: []*string{&accountID},
					},
				},
			})
			if err != nil {
				regionResult.err = fmt.Errorf("Error getting Transit Gateway list: %s", err)
				return
			}
			for _, tg := range tgOut.TransitGateways {
				tgInfo := TransitGatewayInfo{
					TransitGatewayID: *tg.TransitGatewayId,
				}
				for _, tag := range tg.Tags {
					if *tag.Key == "Name" {
						tgInfo.TransitGatewayName = *tag.Value
						break
					}
				}
				share, err := s.ModelsManager.GetTransitGatewayResourceShare(database.Region(regionName), tgInfo.TransitGatewayID)
				if err != nil {
					regionResult.err = fmt.Errorf("Error getting Resource Share info: %s", err)
					return
				}
				if share != nil {
					tgInfo.ResourceShareID = share.ResourceShareID
					arn := resourceShareARN(regionName, accountID, tgInfo.ResourceShareID)
					out, err := ramsvc.GetResourceShares(&ram.GetResourceSharesInput{
						ResourceShareArns: []*string{aws.String(arn)},
						ResourceOwner:     aws.String("SELF"),
					})
					if err == nil && len(out.ResourceShares) > 0 {
						tgInfo.ResourceShareName = *out.ResourceShares[0].Name
					}
				}
				regionResult.RegionInfo.TransitGateways = append(regionResult.RegionInfo.TransitGateways, tgInfo)
			}
		}(region)
	}
	for range regions {
		info := <-infos
		if info.err != nil {
			http.Error(w, info.err.Error(), http.StatusInternalServerError)
			return
		}
		accountPageInfo.Regions = append(accountPageInfo.Regions, info.RegionInfo)
	}

	buf, err := json.Marshal(accountPageInfo)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleCancelTasks = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleCancelTasks but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	type cancelRequest struct {
		TaskIDs []uint64
	}
	req := new(cancelRequest)
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	if len(req.TaskIDs) == 0 {
		http.Error(w, "No task IDs specified", http.StatusBadRequest)
		return
	}

	err = s.TaskDatabase.CancelTasks(req.TaskIDs)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error cancelling tasks: %s", err), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

type BatchTaskRequest struct {
	TaskTypes  database.TaskTypes
	VerifySpec database.VerifySpec
	VPCs       []struct {
		Name, ID, AccountID, Region string
	}
	AddSecurityGroupSets    []uint64
	RemoveSecurityGroupSets []uint64

	AddManagedTransitGatewayAttachments    []uint64
	RemoveManagedTransitGatewayAttachments []uint64

	AddResolverRuleSets    []uint64
	RemoveResolverRuleSets []uint64
}

var handleBatchTask = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleBatchTask but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	req := new(BatchTaskRequest)
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	if !req.TaskTypes.Includes(database.TaskTypeNetworking) &&
		!req.TaskTypes.Includes(database.TaskTypeSecurityGroups) &&
		!req.TaskTypes.Includes(database.TaskTypeResolverRules) &&
		!req.TaskTypes.Includes(database.TaskTypeRepair) &&
		!req.TaskTypes.Includes(database.TaskTypeVerifyState) &&
		!req.TaskTypes.Includes(database.TaskTypeSyncRoutes) {
		http.Error(w, "Invalid task type specification", http.StatusBadRequest)
		return
	}

	if req.TaskTypes.Includes(database.TaskTypeVerifyState) && req.TaskTypes != database.TaskTypeVerifyState {
		http.Error(w, "Verify VPC State must be submitted as a single task", http.StatusBadRequest)
		return
	}

	if req.TaskTypes.Includes(database.TaskTypeRepair) {
		req.TaskTypes |= req.VerifySpec.FollowUpTasks()
	}

	descriptionPieces := []string{}
	if req.TaskTypes.Includes(database.TaskTypeRepair) {
		descriptionPieces = append(descriptionPieces, "sync state and repair tags")
	}
	if req.TaskTypes.Includes(database.TaskTypeNetworking) {
		descriptionPieces = append(descriptionPieces, "update networking")
	}
	if req.TaskTypes.Includes(database.TaskTypeSecurityGroups) {
		descriptionPieces = append(descriptionPieces, "update security groups")
	}
	if req.TaskTypes.Includes(database.TaskTypeResolverRules) {
		descriptionPieces = append(descriptionPieces, "update resolver rules")
	}
	if req.TaskTypes.Includes(database.TaskTypeVerifyState) {
		descriptionPieces = append(descriptionPieces, "verify state")
	}
	if req.TaskTypes.Includes(database.TaskTypeLogging) {
		descriptionPieces = append(descriptionPieces, "update logging")
	}
	if req.TaskTypes.Includes(database.TaskTypeSyncRoutes) {
		descriptionPieces = append(descriptionPieces, "sync routes")
	}

	var batchDescription string
	length := len(descriptionPieces)

	if length <= 2 {
		batchDescription = strings.Join(descriptionPieces, " and ")
	} else {
		batchDescription = strings.Join(descriptionPieces[:length-1], ", ") + " and " + descriptionPieces[length-1]
	}
	if len(batchDescription) > 0 {
		batchDescription = strings.ToUpper(batchDescription[:1]) + batchDescription[1:]
	}

	batchTaskID, err := s.TaskDatabase.AddBatchTask(batchDescription)
	if err != nil {
		log.Printf("Error adding batch task to database: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcs := []*database.VPC{}
	for _, vpcInfo := range req.VPCs {
		vpc, err := s.ModelsManager.GetVPC(database.Region(vpcInfo.Region), vpcInfo.ID)
		if err != nil {
			log.Printf("Error loading info for VPC %q: %s", vpcInfo.ID, err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if vpc.State == nil {
			http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcInfo.ID), http.StatusBadRequest)
			return
		}
		vpcs = append(vpcs, vpc)
	}

	validMTGAs := []uint64{}
	mtgas, err := s.ModelsManager.GetManagedTransitGatewayAttachments()
	if err != nil {
		log.Printf("Error fetching valid managed TGW ids")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, mtga := range mtgas {
		if !uint64InSlice(mtga.ID, validMTGAs) {
			validMTGAs = append(validMTGAs, mtga.ID)
		}
	}
	for _, id := range append(req.AddManagedTransitGatewayAttachments, req.RemoveManagedTransitGatewayAttachments...) {
		if !uint64InSlice(id, validMTGAs) {
			log.Printf("Invalid mtga ID supplied: %d", id)
			http.Error(w, "Invalid id", http.StatusBadRequest)
			return
		}
	}

	validSGs := []uint64{}
	sgs, err := s.ModelsManager.GetSecurityGroupSets()
	if err != nil {
		log.Printf("Error fetching valid security group ids")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, sg := range sgs {
		if !uint64InSlice(sg.ID, validSGs) {
			validSGs = append(validSGs, sg.ID)
		}
	}
	for _, id := range append(req.AddSecurityGroupSets, req.RemoveSecurityGroupSets...) {
		if !uint64InSlice(id, validSGs) {
			log.Printf("Invalid security group ID supplied: %d", id)
			http.Error(w, "Invalid id", http.StatusBadRequest)
			return
		}
	}

	validMRRs := []uint64{}
	mrrRegions := map[uint64]database.Region{}
	mrrs, err := s.ModelsManager.GetManagedResolverRuleSets()
	if err != nil {
		log.Printf("Error fetching valid managed resolver rule ids")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, mrr := range mrrs {
		if !uint64InSlice(mrr.ID, validMRRs) {
			validMRRs = append(validMRRs, mrr.ID)
			mrrRegions[mrr.ID] = mrr.Region
		}
	}
	for _, id := range append(req.AddResolverRuleSets, req.RemoveResolverRuleSets...) {
		if !uint64InSlice(id, validMRRs) {
			log.Printf("Invalid managed resolver rule ID supplied: %d", id)
			http.Error(w, "Invalid id", http.StatusBadRequest)
			return
		}
	}

	if len(req.AddManagedTransitGatewayAttachments) > 0 || len(req.RemoveManagedTransitGatewayAttachments) > 0 {
		for _, vpc := range vpcs {
			if !vpc.State.VPCType.CanUpdateMTGAs() {
				continue
			}
			newMTGAs := []uint64{}
			updated := false
			for _, mtgaID := range vpc.Config.ManagedTransitGatewayAttachmentIDs {
				if uint64InSlice(mtgaID, req.RemoveManagedTransitGatewayAttachments) {
					updated = true
				} else {
					newMTGAs = append(newMTGAs, mtgaID)
				}
			}
			for _, mtgaID := range req.AddManagedTransitGatewayAttachments {
				if !uint64InSlice(mtgaID, newMTGAs) {
					updated = true
					newMTGAs = append(newMTGAs, mtgaID)
				}
			}
			if updated {
				vpc.Config.ManagedTransitGatewayAttachmentIDs = newMTGAs
				err := s.ModelsManager.UpdateVPCConfig(vpc.Region, vpc.ID, *vpc.Config)
				if err != nil {
					log.Printf("Error saving config for VPC %q: %s", vpc.ID, err)
					http.Error(w, "Internal error", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	if len(req.AddSecurityGroupSets) > 0 || len(req.RemoveSecurityGroupSets) > 0 {
		for _, vpc := range vpcs {
			if !vpc.State.VPCType.CanUpdateSecurityGroups() {
				continue
			}
			newSGSs := []uint64{}
			updated := false
			for _, sgsID := range vpc.Config.SecurityGroupSetIDs {
				if uint64InSlice(sgsID, req.RemoveSecurityGroupSets) {
					updated = true
				} else {
					newSGSs = append(newSGSs, sgsID)
				}
			}
			for _, sgsID := range req.AddSecurityGroupSets {
				if !uint64InSlice(sgsID, newSGSs) {
					updated = true
					newSGSs = append(newSGSs, sgsID)
				}
			}
			if updated {
				vpc.Config.SecurityGroupSetIDs = newSGSs
				err := s.ModelsManager.UpdateVPCConfig(vpc.Region, vpc.ID, *vpc.Config)
				if err != nil {
					log.Printf("Error saving config for VPC %q: %s", vpc.ID, err)
					http.Error(w, "Internal error", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	if len(req.AddResolverRuleSets) > 0 || len(req.RemoveResolverRuleSets) > 0 {
		for _, vpc := range vpcs {
			if !vpc.State.VPCType.CanUpdateResolverRules() {
				continue
			}
			newMRRSs := []uint64{}
			updated := false
			for _, mrrID := range vpc.Config.ManagedResolverRuleSetIDs {
				if uint64InSlice(mrrID, req.RemoveResolverRuleSets) {
					updated = true
				} else {
					newMRRSs = append(newMRRSs, mrrID)
				}
			}
			for _, mrrID := range req.AddResolverRuleSets {
				if mrrRegions[mrrID] != vpc.Region {
					continue
				}
				if !uint64InSlice(mrrID, newMRRSs) {
					updated = true
					newMRRSs = append(newMRRSs, mrrID)
				}
			}
			if updated {
				vpc.Config.ManagedResolverRuleSetIDs = newMRRSs
				err := s.ModelsManager.UpdateVPCConfig(vpc.Region, vpc.ID, *vpc.Config)
				if err != nil {
					log.Printf("Error saving config for VPC %q: %s", vpc.ID, err)
					http.Error(w, "Internal error", http.StatusInternalServerError)
					return
				}
			}
		}
	}

	sess := s.getSession(r)
	errors := []string{}
	for _, vpc := range vpcs {
		_, err := scheduleVPCTasks(s.ModelsManager, s.TaskDatabase, database.Region(vpc.Region), vpc.AccountID, vpc.ID, sess.Username, req.TaskTypes, req.VerifySpec, nil, &batchTaskID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error scheduling task for %s: %s", vpc.Name, err))
		}
	}

	if len(errors) > 0 {
		http.Error(w, strings.Join(errors, ". "), http.StatusInternalServerError)
		return
	}

	result := map[string]uint64{
		"BatchTaskID": batchTaskID,
	}
	buf, err := json.Marshal(result)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-type", "application/json")
	fmt.Fprintf(w, "%s", buf)
}

var handleBatchTaskByID = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleBatchTaskByID but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	id, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid ID: %s", err), http.StatusBadRequest)
		return
	}

	bt, err := s.TaskDatabase.GetBatchTaskByID(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s", err), http.StatusBadRequest)
		return
	}

	batchTaskInfos := s.getBatchTaskInfo([]*database.BatchTask{bt})

	if len(batchTaskInfos) != 1 {
		http.Error(w, fmt.Sprintf("Unable to get BatchTaskInfo for ID %d", id), http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(batchTaskInfos[0])
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-type", "application/json")
	fmt.Fprintf(w, "%s", buf)
}

func (s *Server) getBatchTaskInfo(batchTasks []*database.BatchTask) []BatchTaskInfo {
	batchTaskInfos := []BatchTaskInfo{}

	for _, bt := range batchTasks {
		bti := BatchTaskInfo{
			ID:          bt.ID,
			Description: bt.Description,
			AddedAt:     bt.AddedAt,
		}
		for _, t := range bt.Tasks {
			bti.Tasks = append(bti.Tasks, TaskInfo{
				ID:          t.ID,
				AccountID:   t.AccountID,
				VPCID:       t.VPCID,
				VPCRegion:   t.VPCRegion,
				Description: t.Description,
				Status:      t.Status.String(),
			})
		}
		batchTaskInfos = append(batchTaskInfos, bti)
	}

	return batchTaskInfos
}

var handleBatchTasks = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleBatchTasks but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	beforeID := uint64(0)
	beforeIDStr := r.FormValue("beforeID")
	if beforeIDStr != "" {
		n, err := strconv.Atoi(beforeIDStr)
		if err == nil && n > 0 {
			beforeID = uint64(n)
		}
	}

	var tasks []*database.BatchTask
	var isMore bool
	var err error
	if beforeID > 0 {
		tasks, isMore, err = s.TaskDatabase.GetBatchTasksBefore(beforeID)
	} else {
		tasks, isMore, err = s.TaskDatabase.GetBatchTasks()
	}
	result := &struct {
		IsMoreTasks bool
		Tasks       []BatchTaskInfo
	}{}

	result.IsMoreTasks = isMore
	if err != nil {
		log.Printf("Error getting tasks: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	result.Tasks = s.getBatchTaskInfo(tasks)

	buf, err := json.Marshal(result)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleGetLabels = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleGetLabels but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	labels, err := s.ModelsManager.GetLabels()
	if err != nil {
		log.Printf("Error getting labels: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(labels)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleGetVPCLabels = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 3 additional args to handleGetVPCLabels but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	region := args[0]
	vpcID := args[1]

	labels, err := s.ModelsManager.GetVPCLabels(region, vpcID)
	if err != nil {
		log.Printf("Error getting vpc labels: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(labels)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleSetVPCLabel = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional args to handleSetVPCLabel but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	region := args[0]
	vpcID := args[1]
	label := args[2]

	if label == "" {
		http.Error(w, "Label cannot be empty", http.StatusBadRequest)
		return
	}

	label = strings.ToLower(label)
	label = invalidLabelCharacters.ReplaceAllString(label, "")
	label = strings.TrimSpace(label)

	err := s.ModelsManager.SetVPCLabel(region, vpcID, label)
	if err != nil {
		log.Printf("Error setting vpc label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleDeleteVPCLabel = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional args to handleDeleteVPCLabel but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	region := args[0]
	vpcID := args[1]
	label := args[2]

	if label == "" {
		http.Error(w, "Label cannot be empty", http.StatusBadRequest)
		return
	}
	label = strings.ToLower(label)

	err := s.ModelsManager.DeleteVPCLabel(region, vpcID, label)
	if err != nil {
		log.Printf("Error deleting vpc label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	err = s.ModelsManager.DeleteLabel(label)
	if err != nil {
		log.Printf("Error deleting label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleGetAccountLabels = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional args to handleGetAccountLabels but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	accountID := args[0]

	labels, err := s.ModelsManager.GetAccountLabels(accountID)
	if err != nil {
		log.Printf("Error getting account labels: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(labels)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleSetAccountLabel = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleSetAccountLabel but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	accountID := args[0]
	label := args[1]

	label = strings.ToLower(label)
	label = invalidLabelCharacters.ReplaceAllString(label, "")
	label = strings.TrimSpace(label)

	if label == "" {
		http.Error(w, "Label cannot be empty", http.StatusBadRequest)
		return
	}
	label = strings.ToLower(label)

	err := s.ModelsManager.SetAccountLabel(accountID, label)
	if err != nil {
		log.Printf("Error setting account label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleDeleteAccountLabel = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleDeleteAccountLabel but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	accountID := args[0]
	label := args[1]

	if label == "" {
		http.Error(w, "Label cannot be empty", http.StatusBadRequest)
		return
	}
	label = strings.ToLower(label)

	err := s.ModelsManager.DeleteAccountLabel(accountID, label)
	if err != nil {
		log.Printf("Error deleting account label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	err = s.ModelsManager.DeleteLabel(label)
	if err != nil {
		log.Printf("Error deleting label: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleCreateManagedTransitGatewayAttachment = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleCreateManagedTransitGatewayAttachment but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	mtga := new(database.ManagedTransitGatewayAttachment)
	err := json.NewDecoder(r.Body).Decode(mtga)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.CreateManagedTransitGatewayAttachment(mtga)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating transit gateway attachment: %s", err), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "%d", mtga.ID)
}

var handleDeleteManagedTransitGatewayAttachment = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleDeleteManagedTransitGatewayAttachment but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.DeleteManagedTransitGatewayAttachment(uint64(id))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting transit gateway attachment: %s", err), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleUpdateManagedTransitGatewayAttachment = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleUpdateManagedTransitGatewayAttachment but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	mtga := new(database.ManagedTransitGatewayAttachment)
	err = json.NewDecoder(r.Body).Decode(mtga)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.UpdateManagedTransitGatewayAttachment(uint64(id), mtga)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error updating transit gateway attachment: %s", err), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleCreateSecurityGroupSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleCreateSecurityGroupSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sgs := new(database.SecurityGroupSet)
	err := json.NewDecoder(r.Body).Decode(sgs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.CreateSecurityGroupSet(sgs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating security group set: %s", err), http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(sgs)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleDeleteSecurityGroupSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleDeleteSecurityGroupSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.DeleteSecurityGroupSet(uint64(id))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting transit gateway attachment: %s", err), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleUpdateSecurityGroupSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleUpdateSecurityGroupSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	sgs := new(database.SecurityGroupSet)
	err = json.NewDecoder(r.Body).Decode(sgs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.UpdateSecurityGroupSet(uint64(id), sgs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error updating transit gateway attachment: %s", err), http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(sgs)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleManagedTransitGatewayAttachmentList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleManagedTransitGatewayAttachmentList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	mas, err := s.ModelsManager.GetManagedTransitGatewayAttachments()
	if err != nil {
		log.Printf("Error getting managed transit gateway attachments: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(mas)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleExceptionVPCList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleExceptionVPCList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcs, err := s.ModelsManager.ListExceptionVPCs()
	if err != nil {
		log.Printf("Error getting exception VPCs: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(vpcs)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleCreateManagedResolverRuleSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleCreateManagedResolverRuleSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	mrr := new(database.ManagedResolverRuleSet)
	err := json.NewDecoder(r.Body).Decode(mrr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.CreateManagedResolverRuleSet(mrr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating resolver ruleset: %s", err), http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(mrr)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleDeleteManagedResolverRuleSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleDeleteManagedResolverRuleSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.DeleteManagedResolverRuleSet(uint64(id))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error deleting resolver ruleset: %s", err), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleUpdateManagedResolverRuleSet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleUpdateManagedResolverRuleSet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid id: %s", err), http.StatusBadRequest)
		return
	}
	if id < 0 {
		http.Error(w, fmt.Sprintf("Invalid id %d", id), http.StatusBadRequest)
		return
	}

	mrr := new(database.ManagedResolverRuleSet)
	err = json.NewDecoder(r.Body).Decode(mrr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	err = s.ModelsManager.UpdateManagedResolverRuleSet(uint64(id), mrr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error updating managed resolver ruleset: %s", err), http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(mrr)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleManagedResolverRuleSetList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleManagedResolverRuleSetList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	mrrs, err := s.ModelsManager.GetManagedResolverRuleSets()
	if err != nil {
		log.Printf("Error getting managed resolver rulesets: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(mrrs)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleSecurityGroupSetList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleSecurityGroupSetList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sets, err := s.ModelsManager.GetSecurityGroupSets()
	if err != nil {
		log.Printf("Error getting security group sets: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(sets)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleListManagedPrefixLists = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleListManagedPrefixLists but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	region := args[0]

	asUser := s.getSession(r).Username
	prefixListAccountID := prefixListAccountIDCommercial
	if strings.HasPrefix(region, "us-gov-") {
		prefixListAccountID = prefixListAccountIDGovCloud
	}
	sess, err := s.CachedCredentials.GetAWSSession(prefixListAccountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ec2svc := ec2.New(sess)

	out, err := ec2svc.DescribeManagedPrefixLists(&ec2.DescribeManagedPrefixListsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: []*string{aws.String(prefixListAccountID)},
			},
		},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error listing managed prefix lists: %s", err), http.StatusInternalServerError)
		return
	}

	prefixLists := []PrefixListInfo{}
	for _, pl := range out.PrefixLists {
		plInfo := PrefixListInfo{
			Name: aws.StringValue(pl.PrefixListName),
			ID:   aws.StringValue(pl.PrefixListId),
		}
		prefixLists = append(prefixLists, plInfo)
	}

	buf, err := json.Marshal(prefixLists)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleManagedPrefixListDetails = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleManagedPrefixListDetails but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	plID := args[1]

	asUser := s.getSession(r).Username
	prefixListAccountID := prefixListAccountIDCommercial
	if region == "us-gov-west-1" {
		prefixListAccountID = prefixListAccountIDGovCloud
	}
	sess, err := s.CachedCredentials.GetAWSSession(prefixListAccountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ec2svc := ec2.New(sess)

	out, err := ec2svc.GetManagedPrefixListEntries(&ec2.GetManagedPrefixListEntriesInput{
		PrefixListId: &plID,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting managed prefix list details: %s", err), http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(out.Entries)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}

	fmt.Fprintf(w, "%s", buf)
}

// Takes the first URL that looks like https://jiraent.cms.gov/browse/IA-1801
// and stores the issue ID (IA-1801) in the provided location.
func extractJIRAIssue(relatedIssueURLs []string, issueID *string) {
	for _, url := range relatedIssueURLs {
		if strings.HasPrefix(url, jira.BaseURL) {
			pieces := strings.Split(url, "/")
			*issueID = pieces[len(pieces)-1]
			break
		}
	}
}

var handleProvisionRequest = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleProvisionRequest but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	reqID, err := strconv.Atoi(args[0])
	requestID := uint64(reqID)
	var req *database.VPCRequest
	if reqID >= 0 && err == nil {
		req, err = s.ModelsManager.GetVPCRequest(requestID)
		if err != nil {
			req = nil
			log.Printf("Error getting request %d: %s", requestID, err)
		}
	}
	if req == nil {
		http.Error(w, "Invalid request ID", http.StatusBadRequest)
		return
	}

	allocateConfig := new(database.AllocateConfig)
	err = json.NewDecoder(r.Body).Decode(allocateConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing config: %s", err), http.StatusBadRequest)
		return
	}
	allocateConfig.AccountID = req.AccountID

	err = s.ModelsManager.SetVPCRequestApprovedConfig(requestID, allocateConfig)
	if err != nil {
		log.Printf("Error updating request %d status: %s", requestID, err)
		http.Error(w, "Error updating request status", http.StatusInternalServerError)
		return
	}

	var response map[string]uint64
	asUser := s.getSession(r).Username

	if req.RequestType == database.RequestTypeNewSubnet {
		taskData := &database.AddZonedSubnetsTaskData{
			VPCID:      allocateConfig.VPCID,
			Region:     database.Region(allocateConfig.AWSRegion),
			SubnetType: database.SubnetType(allocateConfig.SubnetType),
			SubnetSize: allocateConfig.SubnetSize,
			GroupName:  allocateConfig.GroupName,
		}

		if taskData.GroupName == "" {
			taskData.GroupName = strings.ToLower(string(taskData.SubnetType))
		}

		prereq := &database.TaskData{
			AddZonedSubnetsTaskData: taskData,
			AsUser:                  asUser,
		}
		extractJIRAIssue(req.RelatedIssues, &prereq.AddZonedSubnetsTaskData.JIRAIssueForComment)
		taskBytes, err := json.Marshal(prereq)
		if err != nil {
			log.Printf("Error marshalling: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		taskName := fmt.Sprintf("Adding %s subnets to %s", taskData.SubnetType, allocateConfig.VPCID)
		t, err := s.TaskDatabase.AddVPCTask(allocateConfig.AccountID, allocateConfig.VPCID, taskName, taskBytes, database.TaskStatusQueued, nil)
		if err != nil {
			log.Printf("Error adding task: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		id, err := scheduleVPCTasks(s.ModelsManager, s.TaskDatabase, database.Region(allocateConfig.AWSRegion), allocateConfig.AccountID, allocateConfig.VPCID, asUser, database.TaskTypeNetworking, database.VerifySpec{}, t, nil)
		if err != nil {
			log.Printf("Error scheduling add-zoned-subnets task: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		err = s.ModelsManager.SetVPCRequestTaskID(uint64(requestID), t.ID)
		if err != nil {
			log.Printf("Error updating request %d task ID: %s", requestID, err)
		}

		response = map[string]uint64{
			"TaskID": id,
		}
	} else if req.RequestType == database.RequestTypeNewVPC {
		taskData := &database.TaskData{
			CreateVPCTaskData: &database.CreateVPCTaskData{
				AllocateConfig: *allocateConfig,
				VPCRequestID:   &requestID,
			},
			AsUser: asUser,
		}
		extractJIRAIssue(req.RelatedIssues, &taskData.CreateVPCTaskData.JIRAIssueForComment)
		taskBytes, err := json.Marshal(taskData)
		if err != nil {
			log.Printf("Error marshaling: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		t, err := s.TaskDatabase.AddAccountTask(allocateConfig.AccountID, "Create new VPC "+allocateConfig.VPCName, taskBytes, database.TaskStatusQueued)
		if err != nil {
			log.Printf("Error adding task: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		err = s.ModelsManager.SetVPCRequestTaskID(uint64(requestID), t.ID)
		if err != nil {
			log.Printf("Error updating request %d task ID: %s", requestID, err)
		}
		response = map[string]uint64{
			"TaskID": t.ID,
		}
	} else {
		log.Printf("Unknown request type: %d", req.RequestType)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCRequestList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleVPCRequestList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	mas, err := s.ModelsManager.GetAllVPCRequests()
	if err != nil {
		log.Printf("Error getting VPC requests: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(map[string]interface{}{
		"Requests": mas,
		"Regions":  append(s.getRegions(false), s.getRegions(true)...),
	})
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleGetVPCRequest = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 argument to handleGetVPCRequest but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcRequestID, _ := strconv.ParseUint(args[0], 10, 64)
	req, err := s.ModelsManager.GetVPCRequest(vpcRequestID)
	if err != nil {
		log.Printf("Error getting VPC Request: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(req)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCRequestJIRAIssueErrorList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 argument to handleVPCRequestJiraIssueList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpcRequestID, _ := strconv.ParseUint(args[0], 10, 64)
	jiraErrors, err := s.ModelsManager.GetVPCRequestLogs(vpcRequestID)
	if err != nil {
		log.Printf("Error executing GetVPCRequestLogs in handleVPCRequestJiraIssueList: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(jiraErrors)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleAccountList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleAccountList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	accounts := s.getSession(r).AuthorizedAccounts

	buf, err := json.Marshal(accounts)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleIndex = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	err := s.templateIndex().Execute(w, staticAssetsVersion)
	if err != nil {
		log.Printf("Error executing templateIndex: %s", err)
	}
}

const internetRoute = "0.0.0.0/0"

var handleVPCImport = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCImport but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]
	isLegacy := r.FormValue("legacy") == "1"

	var vpcType database.VPCType
	if isLegacy {
		vpcType = database.VPCTypeLegacy
	} else {
		vpcType = database.VPCTypeV1
	}

	importData := &database.ImportVPCTaskData{
		VPCType:   vpcType,
		VPCID:     vpcID,
		Region:    database.Region(region),
		AccountID: accountID,
	}

	taskData := &database.TaskData{
		ImportVPCTaskData: importData,
		AsUser:            s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddAccountTask(accountID, "Import VPC "+vpcID, taskBytes, database.TaskStatusQueued)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCEstablishException = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCEstablishException but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	importData := &database.EstablishExceptionVPCTaskData{
		VPCID:     vpcID,
		Region:    database.Region(region),
		AccountID: accountID,
	}

	taskData := &database.TaskData{
		EstablishExceptionVPCTaskData: importData,
		AsUser:                        s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddAccountTask(accountID, "Establish exception VPC "+vpcID, taskBytes, database.TaskStatusQueued)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCUnimport = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCUnimport but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	unimportData := &database.UnimportVPCTaskData{
		VPCID:  vpcID,
		Region: database.Region(region),
	}

	taskData := &database.TaskData{
		UnimportVPCTaskData: unimportData,
		AsUser:              s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddAccountTask(accountID, "Unimport VPC "+vpcID, taskBytes, database.TaskStatusQueued)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

func scheduleVPCTasks(mm database.ModelsManager, taskDB *database.TaskDatabase, region database.Region, accountID, vpcID string, asUser string, taskTypes database.TaskTypes, verifySpec database.VerifySpec, prereq database.TaskInterface, batchTaskID *uint64) (uint64, error) {
	vpc, err := mm.GetVPC(database.Region(region), vpcID)
	if err != nil {
		return 0, fmt.Errorf("Error fetching VPC configuration: %s", err)
	}
	if vpc.State == nil {
		return 0, fmt.Errorf("VPC %s is not automated", vpcID)
	}

	var prereqID *uint64

	type taskInfo struct {
		name   string
		data   *database.TaskData
		taskID uint64
	}
	tasks := []*taskInfo{}

	addTask := func(t *taskInfo) (uint64, error) {
		t.data.AsUser = asUser
		taskBytes, err := json.Marshal(t.data)
		if err != nil {
			return 0, fmt.Errorf("Error marshaling: %s", err)
		}
		var newTask *database.Task
		if prereqID == nil {
			newTask, err = taskDB.AddVPCTask(accountID, vpcID, t.name, taskBytes, database.TaskStatusQueued, batchTaskID)
			if err != nil {
				return 0, fmt.Errorf("Error adding task: %s", err)
			}
		} else {
			newTask, err = taskDB.AddDependentVPCTask(accountID, vpcID, t.name, taskBytes, database.TaskStatusQueued, *prereqID, batchTaskID)
			if err != nil {
				return 0, fmt.Errorf("Error adding task: %s", err)
			}
		}
		t.taskID = newTask.ID
		tasks = append(tasks, t)
		return t.taskID, nil
	}

	if prereq != nil {
		id := prereq.GetID()
		prereqID = &id
	}

	if taskTypes&database.TaskTypeRepair != 0 {
		id, err := addTask(&taskInfo{
			name: "Sync VPC " + vpcID + " state and repair tags",
			data: &database.TaskData{
				RepairVPCTaskData: &database.RepairVPCTaskData{
					VPCID:  vpcID,
					Region: database.Region(region),
					Spec:   verifySpec,
				},
				AsUser: asUser,
			},
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
		prereqID = &id
	}

	if taskTypes&database.TaskTypeNetworking != 0 {
		networkConfig := &database.UpdateNetworkingTaskData{NetworkingConfig: database.NetworkingConfig{
			ConnectPublic:                      vpc.Config.ConnectPublic,
			ConnectPrivate:                     vpc.Config.ConnectPrivate,
			ManagedTransitGatewayAttachmentIDs: vpc.Config.ManagedTransitGatewayAttachmentIDs,
			PeeringConnections:                 vpc.Config.PeeringConnections,
		}}
		networkConfig.AWSRegion = region
		networkConfig.VPCID = vpcID
		networkTaskData := &database.TaskData{
			UpdateNetworkingTaskData: networkConfig,
			AsUser:                   asUser,
		}
		_, err := addTask(&taskInfo{
			name: "Update VPC " + vpcID + " networking",
			data: networkTaskData,
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if taskTypes&database.TaskTypeLogging != 0 {
		config := &database.UpdateLoggingTaskData{
			Region: region,
			VPCID:  vpcID,
		}
		networkTaskData := &database.TaskData{
			UpdateLoggingTaskData: config,
			AsUser:                asUser,
		}
		_, err := addTask(&taskInfo{
			name: "Update VPC " + vpcID + " logging",
			data: networkTaskData,
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if taskTypes&database.TaskTypeSecurityGroups != 0 && vpc.State.VPCType.CanUpdateSecurityGroups() {
		taskData := &database.TaskData{
			UpdateSecurityGroupsTaskData: &database.UpdateSecurityGroupsTaskData{
				VPCID:     vpc.ID,
				AWSRegion: vpc.Region,
				SecurityGroupConfig: database.SecurityGroupConfig{
					SecurityGroupSetIDs: vpc.Config.SecurityGroupSetIDs,
				},
			},
			AsUser: asUser,
		}
		_, err := addTask(&taskInfo{
			name: "Update VPC " + vpcID + " security groups",
			data: taskData,
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if taskTypes&database.TaskTypeResolverRules != 0 && vpc.State.VPCType.CanUpdateResolverRules() {
		taskData := &database.TaskData{
			UpdateResolverRulesTaskData: &database.UpdateResolverRulesTaskData{
				VPCID:               vpc.ID,
				AWSRegion:           vpc.Region,
				ResolverRulesConfig: database.ResolverRulesConfig{ManagedResolverRuleSetIDs: vpc.Config.ManagedResolverRuleSetIDs},
			},
			AsUser: asUser,
		}
		_, err := addTask(&taskInfo{
			name: "Update VPC " + vpcID + " resolver rules",
			data: taskData,
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if taskTypes&database.TaskTypeVerifyState != 0 {
		_, err := addTask(&taskInfo{
			name: "Verify VPC " + vpcID,
			data: &database.TaskData{
				VerifyVPCTaskData: &database.VerifyVPCTaskData{
					VPCID:  vpcID,
					Region: database.Region(region),
					Spec:   verifySpec,
				},
				AsUser: asUser,
			},
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if taskTypes&database.TaskTypeSyncRoutes != 0 {
		_, err := addTask(&taskInfo{
			name: "Sync Routes from " + vpcID,
			data: &database.TaskData{
				SynchronizeRouteTableStateFromAWSTaskData: &database.SynchronizeRouteTableStateFromAWSTaskData{
					VPCID:  vpcID,
					Region: database.Region(region),
				},
				AsUser: asUser,
			},
		})
		if err != nil {
			return 0, fmt.Errorf("Error adding task: %s", err)
		}
	}

	if len(tasks) == 0 {
		return 0, nil
	}
	return tasks[len(tasks)-1].taskID, nil
}

var handleVPCRepair = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCRepair but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	spec := &database.VerifySpec{}
	err := json.NewDecoder(r.Body).Decode(spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	id, err := scheduleVPCTasks(s.ModelsManager, s.TaskDatabase, database.Region(region), accountID, vpcID, s.getSession(r).Username, database.TaskTypeRepair|spec.FollowUpTasks(), *spec, nil, nil)
	if err != nil {
		log.Printf("Error scheduling repair task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": id,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleAddAvailabilityZone = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleAddAvailabilityZone but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	addData := &database.AddAvailabilityZoneTaskData{}
	err := json.NewDecoder(r.Body).Decode(addData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	if addData.AZName == "" {
		http.Error(w, "No AZ name specified", http.StatusBadRequest)
		return
	}
	addData.VPCID = vpcID
	addData.Region = database.Region(region)

	prereq := &database.TaskData{
		AddAvailabilityZoneTaskData: addData,
	}
	prereq.AsUser = s.getSession(r).Username
	taskBytes, err := json.Marshal(prereq)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	taskName := fmt.Sprintf("Adding AZ %s to %s", addData.AZName, vpcID)
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := scheduleVPCTasks(s.ModelsManager, s.TaskDatabase, database.Region(region), accountID, vpcID, s.getSession(r).Username, database.TaskTypeNetworking, database.VerifySpec{}, t, nil)
	if err != nil {
		log.Printf("Error scheduling add-availability-zone task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	requestID := addData.RequestID
	if requestID != 0 {
		err = s.ModelsManager.SetVPCRequestTaskID(uint64(requestID), t.ID)
		if err != nil {
			log.Printf("Error updating request %d task ID: %s", requestID, err)
		}
	}

	response := map[string]uint64{
		"TaskID": id,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleRemoveAvailabilityZone = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleRemoveAvailabilityZone but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	removeData := &database.RemoveAvailabilityZoneTaskData{}
	err := json.NewDecoder(r.Body).Decode(removeData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	if removeData.AZName == "" {
		http.Error(w, "No AZ name specified", http.StatusBadRequest)
		return
	}
	removeData.VPCID = vpcID
	removeData.Region = database.Region(region)

	data := &database.TaskData{
		RemoveAvailabilityZoneTaskData: removeData,
	}
	data.AsUser = s.getSession(r).Username
	taskBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	taskName := fmt.Sprintf("Removing AZ %q", removeData.AZName)
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleAddZonedSubnets = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleAddZonedSubnets but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	addData := &database.AddZonedSubnetsTaskData{}
	err := json.NewDecoder(r.Body).Decode(addData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	addData.VPCID = vpcID
	addData.Region = database.Region(region)

	if addData.GroupName == "" {
		addData.GroupName = strings.ToLower(string(addData.SubnetType))
	}

	prereq := &database.TaskData{
		AddZonedSubnetsTaskData: addData,
	}
	prereq.AsUser = s.getSession(r).Username
	taskBytes, err := json.Marshal(prereq)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	taskName := fmt.Sprintf("Adding %s subnets to %s", addData.SubnetType, vpcID)
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	id, err := scheduleVPCTasks(s.ModelsManager, s.TaskDatabase, database.Region(region), accountID, vpcID, s.getSession(r).Username, database.TaskTypeNetworking, database.VerifySpec{}, t, nil)
	if err != nil {
		log.Printf("Error scheduling add-zoned-subnets task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": id,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleConnectCMSNet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleConnectCMSNet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	params := &struct {
		GroupName       string
		DestinationCIDR string
	}{}
	err := json.NewDecoder(r.Body).Decode(params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateCMSNet() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}

	privateSubnetIDs := []string{}
	reqParams := []*cmsnet.ConnectionRequestParams{}
	for _, az := range vpc.State.AvailabilityZones {
		for subnetType, subnets := range az.Subnets {
			if subnetType == database.SubnetTypePrivate && len(subnets) > 0 {
				privateSubnetIDs = append(privateSubnetIDs, subnets[0].SubnetID)
			}
			vrfName := subnetType.VRFName(vpc.Stack)
			for _, subnet := range subnets {
				if subnet.GroupName == params.GroupName {
					if subnetType == database.SubnetTypeTransitive || subnetType == database.SubnetTypePublic || subnetType == database.SubnetTypePrivate || subnetType == database.SubnetTypeUnroutable || subnetType == database.SubnetTypeFirewall {
						http.Error(w, "Not available for Public, Private, Transitive, Unroutable, or Firewall type subnets", http.StatusBadRequest)
						return
					}
					reqParams = append(reqParams, &cmsnet.ConnectionRequestParams{
						SubnetID:        subnet.SubnetID,
						DestinationCIDR: params.DestinationCIDR,
						VRF:             vrfName,
						// attachment subnet IDs filled in later
					})
				}
			}
		}
	}
	errs := []string{}
	asUser := s.getSession(r).Username
	for _, p := range reqParams {
		p.AttachmentSubnetIDs = privateSubnetIDs
		_, err := s.getCMSNetClient(s.CredentialService).MakeConnectionRequest(accountID, database.Region(region), vpcID, p, asUser)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		http.Error(w, strings.Join(errs, ". "), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

func (s *Server) getCMSNetClient(credsProvider credentialservice.CredentialsProvider) cmsnet.ClientInterface {
	return cmsnet.NewClient(s.CMSNetConfig, s.LimitToAWSAccountIDs, credsProvider)
}

var handleAddCMSNetNAT = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleAddCMSNetNAT but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	params := &struct {
		PrivateIP, PublicIP string
	}{}
	err := json.NewDecoder(r.Body).Decode(params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	internalIP := net.ParseIP(params.PrivateIP)
	if internalIP == nil {
		http.Error(w, fmt.Sprintf("Unable to parse IP %s", params.PrivateIP), http.StatusBadRequest)
		return
	}

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateCMSNet() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}
	asUser := s.getSession(r).Username
	sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ec2svc := ec2.New(sess)

	out, err := ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
		},
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error listing subnets: %s", err), http.StatusInternalServerError)
		return
	}
	subnetIDToIPNet := map[string]*net.IPNet{}
	for _, subnet := range out.Subnets {
		_, ipNet, err := net.ParseCIDR(aws.StringValue(subnet.CidrBlock))
		if err == nil {
			subnetIDToIPNet[aws.StringValue(subnet.SubnetId)] = ipNet
		} else {
			log.Printf("Error parsing CIDR %s: %s", aws.StringValue(subnet.CidrBlock), err)
		}
	}
	cmsNetClient := s.getCMSNetClient(s.CredentialService)
	data, err := cmsNetClient.GetAllConnectionRequests(vpc.AccountID, vpc.Region, vpc.ID, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error listing existing CMSNet connections: %s", err), http.StatusInternalServerError)
		return
	}
	var subnetID *string
	var subnetType database.SubnetType
SubnetLoop:
	for _, az := range vpc.State.AvailabilityZones {
		for st, subnets := range az.Subnets {
			for _, subnet := range subnets {
				ipNet := subnetIDToIPNet[subnet.SubnetID]
				if ipNet != nil && ipNet.Contains(internalIP) {
					subnetID = &subnet.SubnetID
					subnetType = st
					break SubnetLoop
				}
			}
		}
	}
	if subnetID == nil {
		http.Error(w, fmt.Sprintf("Could not find a subnet containing IP %s", internalIP), http.StatusBadRequest)
		return
	}
	if subnetType != database.SubnetTypeTransport {
		http.Error(w, fmt.Sprintf("Subnet %s is not a Transport subnet", *subnetID), http.StatusBadRequest)
		return
	}

	foundConnectedSubnet := false
	desiredVRF := database.SubnetTypeTransport.VRFName(vpc.Stack)
	for _, activation := range data.Activations {
		if *subnetID == activation.SubnetID && activation.VRF == desiredVRF {
			foundConnectedSubnet = true
			break
		}
	}
	if !foundConnectedSubnet {
		http.Error(w, fmt.Sprintf("Subnet %s is not connected to %s VRF on CMSNet", *subnetID, desiredVRF), http.StatusBadRequest)
		return
	}
	reqParams := &cmsnet.NATRequestParams{
		InsideNetwork: fmt.Sprintf("%s/32", params.PrivateIP),
		VRF:           desiredVRF,
	}
	if params.PublicIP != "" {
		reqParams.OutsideNetwork = fmt.Sprintf("%s/32", params.PublicIP)
	}
	_, err = cmsNetClient.MakeNATRequest(accountID, database.Region(region), vpcID, reqParams, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating NAT: %s", err), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

var handleDeleteCMSNetNAT = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleDisconnectCMSNet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	params := &struct {
		InsideNetwork, OutsideNetwork string
		KeepIPReserved                bool
	}{}
	err := json.NewDecoder(r.Body).Decode(params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateCMSNet() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}

	asUser := s.getSession(r).Username
	reqs, err := s.getCMSNetClient(s.CredentialService).GetAllNATRequests(accountID, database.Region(region), vpcID, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting CMSNet connection info: %s", err), http.StatusInternalServerError)
		return
	}
	errs := []string{}
	found := false
	for _, a := range reqs.NATs {
		if a.InsideNetwork == params.InsideNetwork && a.OutsideNetwork == params.OutsideNetwork {
			found = true
			_, err := s.getCMSNetClient(s.CredentialService).DeleteNAT(a.RequestID, accountID, database.Region(region), vpcID, params.KeepIPReserved, asUser)
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		http.Error(w, strings.Join(errs, ". "), http.StatusInternalServerError)
		return
	}
	if !found {
		http.Error(w, fmt.Sprintf("No NAT %s / %s found", params.InsideNetwork, params.OutsideNetwork), http.StatusBadRequest)
		return
	}

	fmt.Fprintf(w, "null")
}

var handleDisconnectCMSNet = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleDisconnectCMSNet but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	params := &struct {
		GroupName       string
		DestinationCIDR string
	}{}
	err := json.NewDecoder(r.Body).Decode(params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateCMSNet() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}
	matchesSubnetID := map[string]bool{}
	for _, az := range vpc.State.AvailabilityZones {
		for _, subnets := range az.Subnets {
			for _, subnet := range subnets {
				if subnet.GroupName == params.GroupName {
					matchesSubnetID[subnet.SubnetID] = true
				}
			}
		}
	}
	asUser := s.getSession(r).Username
	reqs, err := s.getCMSNetClient(s.CredentialService).GetAllConnectionRequests(accountID, database.Region(region), vpcID, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting CMSNet connection info: %s", err), http.StatusInternalServerError)
		return
	}
	errs := []string{}
	for _, a := range reqs.Activations {
		if a.DestinationCIDR == params.DestinationCIDR && matchesSubnetID[a.SubnetID] {
			_, err := s.getCMSNetClient(s.CredentialService).DeleteActivation(a.RequestID, accountID, database.Region(region), vpcID, asUser)
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) > 0 {
		http.Error(w, strings.Join(errs, ". "), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "null")
}

var handleSyncRouteState = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleSyncRouteState but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}

	synchronizeRouteTableStateFromAWSData := &database.SynchronizeRouteTableStateFromAWSTaskData{
		VPCID:  vpc.ID,
		Region: vpc.Region,
	}
	data := &database.TaskData{
		SynchronizeRouteTableStateFromAWSTaskData: synchronizeRouteTableStateFromAWSData,
		AsUser: s.getSession(r).Username,
	}

	taskBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	taskName := "Synchronize AWS route tables to internal state"
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}

	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleRenameVPC = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleRenameVPC but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, "Account ID does not match VPC account ID", http.StatusBadRequest)
		return
	}

	updateVPCNameData := &database.UpdateVPCNameTaskData{
		VPCID:     vpc.ID,
		AWSRegion: vpc.Region,
	}
	err = json.NewDecoder(r.Body).Decode(&updateVPCNameData.VPCName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	data := &database.TaskData{
		UpdateVPCNameTaskData: updateVPCNameData,
		AsUser:                s.getSession(r).Username,
	}

	taskBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Println(string(taskBytes))

	taskName := "Rename VPC"
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}

	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleRemoveZonedSubnets = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleRemoveZonedSubnets but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	removeData := &database.RemoveZonedSubnetsTaskData{}
	err := json.NewDecoder(r.Body).Decode(removeData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	if removeData.GroupName == "" {
		http.Error(w, "No group name specified", http.StatusBadRequest)
		return
	}
	removeData.VPCID = vpcID
	removeData.Region = database.Region(region)

	data := &database.TaskData{
		RemoveZonedSubnetsTaskData: removeData,
	}
	data.AsUser = s.getSession(r).Username
	taskBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	taskName := fmt.Sprintf("Removing %q subnets", removeData.GroupName)
	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCVerify = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCVerify but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	verifyData := &database.VerifyVPCTaskData{
		VPCID:  vpcID,
		Region: database.Region(region),
	}

	err := json.NewDecoder(r.Body).Decode(&verifyData.Spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	taskData := &database.TaskData{
		VerifyVPCTaskData: verifyData,
		AsUser:            s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, "Verify VPC "+vpcID, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCFlowLogs = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCFlowLogs but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if vpc.State.VPCType == database.VPCTypeException {
		http.Error(w, "Not available for Exception VPCs", http.StatusBadRequest)
		return
	}

	taskData := &database.TaskData{
		UpdateLoggingTaskData: &database.UpdateLoggingTaskData{
			VPCID:  vpcID,
			Region: database.Region(region),
		},
		AsUser: s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, "Update VPC "+vpcID+" logging", taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCNetwork = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCNetwork but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	networkConfig := &database.UpdateNetworkingTaskData{}
	err := json.NewDecoder(r.Body).Decode(&networkConfig.NetworkingConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	networkConfig.AWSRegion = database.Region(region)
	networkConfig.VPCID = vpcID

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if vpc.State.VPCType == database.VPCTypeException {
		http.Error(w, "Not available for Exception VPCs", http.StatusBadRequest)
		return
	}

	if networkConfig.ConnectPrivate && !networkConfig.ConnectPublic {
		http.Error(w, "You cannot connect private subnets to the internet without connecting public subnets.", http.StatusBadRequest)
		return
	}

	taskData := &database.TaskData{
		UpdateNetworkingTaskData: networkConfig,
		AsUser:                   s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, "Update VPC "+vpcID+" networking", taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpc.Config.ConnectPublic = networkConfig.ConnectPublic
	vpc.Config.ConnectPrivate = networkConfig.ConnectPrivate
	vpc.Config.ManagedTransitGatewayAttachmentIDs = networkConfig.ManagedTransitGatewayAttachmentIDs
	vpc.Config.PeeringConnections = networkConfig.PeeringConnections
	err = s.ModelsManager.UpdateVPCConfig(database.Region(region), vpcID, *vpc.Config)
	if err != nil {
		log.Printf("Error updating VPC config: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCResolverRules = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCResolverRules but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	config := &database.UpdateResolverRulesTaskData{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	config.AWSRegion = database.Region(region)
	config.VPCID = vpcID

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateResolverRules() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}

	taskData := &database.TaskData{
		UpdateResolverRulesTaskData: config,
		AsUser:                      s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, "Update VPC "+vpcID+" resolver rules", taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpc.Config.ManagedResolverRuleSetIDs = config.ManagedResolverRuleSetIDs
	err = s.ModelsManager.UpdateVPCConfig(database.Region(region), vpcID, *vpc.Config)
	if err != nil {
		log.Printf("Error updating VPC config: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCNetworkFirewall = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional args to handleVPCNetworkFirewall but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	req := updateNetworkFirewallRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateNetworkFirewall() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}

	asUser := s.getSession(r).Username
	sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ec2svc := ec2.New(sess)
	customRoutes, err := s.findCustomRoutesOnPublicRouteTables(ec2svc, vpc)
	if err != nil {
		log.Printf("Error checking for custom routes on public route tables: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if len(customRoutes) > 0 {
		msg := "The VPC has routes on public route tables are not managed by VPC Conf and would be destroyed by migrating. Remove the routes manually before proceeding."
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	var taskID uint64

	if req.AddNetworkFirewall {
		taskID, err = s.scheduleV1ToV1FirewallTasks(vpcID, accountID, asUser, database.Region(region), vpc)
		if err != nil {
			log.Printf("Error scheduling tasks V1 to V1Firewall: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	} else {
		taskID, err = s.scheduleV1FirewallToV1Tasks(vpcID, accountID, asUser, database.Region(region), vpc)
		if err != nil {
			log.Printf("Error scheduling tasks for V1Firewall to V1: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	response := map[string]uint64{
		"TaskID": taskID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCSecurityGroups = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCSecurityGroups but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	config := &database.UpdateSecurityGroupsTaskData{}
	err := json.NewDecoder(r.Body).Decode(&config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}
	config.AWSRegion = database.Region(region)
	config.VPCID = vpcID

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}
	if !vpc.State.VPCType.CanUpdateSecurityGroups() {
		http.Error(w, "Not available for this type of VPC", http.StatusBadRequest)
		return
	}

	taskData := &database.TaskData{
		UpdateSecurityGroupsTaskData: config,
		AsUser:                       s.getSession(r).Username,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		log.Printf("Error marshaling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	t, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, "Update VPC "+vpcID+" security groups", taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		log.Printf("Error adding task: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	vpc.Config.SecurityGroupSetIDs = config.SecurityGroupSetIDs
	err = s.ModelsManager.UpdateVPCConfig(database.Region(region), vpcID, *vpc.Config)
	if err != nil {
		log.Printf("Error updating VPC config: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response := map[string]uint64{
		"TaskID": t.ID,
	}
	buf, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleGetVPCState = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleGetVPCState but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]
	region := args[1]
	vpcID := args[2]

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.AccountID != accountID {
		http.Error(w, fmt.Sprintf("Invalid Account ID: %s", accountID), http.StatusBadRequest)
		return
	}
	if vpc.State == nil {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}

	// Convert the vpc state to a map of interfaces first so that JSON tags are ignored and all fields are serialized
	buf, err := json.Marshal(lib.ObjectToMap(vpc.State))
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCTransitGatewayAttachments = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional arg to handleVPCTransitGatewayAttachments but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	region := args[0]
	accountID := args[1]
	vpcID := args[2]

	account, err := s.ModelsManager.GetAccount(accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading account from database: %s", err), http.StatusInternalServerError)
		return
	}

	asUser := s.getSession(r).Username
	sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}
	ec2svc := ec2.New(sess)

	out, err := ec2svc.DescribeTransitGatewayVpcAttachments(&ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
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
		http.Error(w, fmt.Sprintf("Error getting transit gateway attachments: %s", err), http.StatusInternalServerError)
		return
	}

	results := []*database.ManagedTransitGatewayAttachment{}
	for _, attachment := range out.TransitGatewayVpcAttachments {
		mtga := &database.ManagedTransitGatewayAttachment{
			TransitGatewayID: *attachment.TransitGatewayId,
			IsGovCloud:       account.IsGovCloud,
		}
		for _, tag := range attachment.Tags {
			if *tag.Key == "Name" {
				mtga.Name = *tag.Value
			}
		}

		if len(attachment.SubnetIds) > 0 {
			inp := &ec2.DescribeRouteTablesInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("route.transit-gateway-id"),
						Values: []*string{attachment.TransitGatewayId},
					},
					{
						Name:   aws.String("association.subnet-id"),
						Values: attachment.SubnetIds,
					},
				},
			}
			out, err := ec2svc.DescribeRouteTables(inp)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error gettings: %s", err), http.StatusInternalServerError)
				return
			}
			for _, rt := range out.RouteTables {
				for _, route := range rt.Routes {
					var dest string

					if aws.StringValue(route.DestinationCidrBlock) != "" {
						dest = aws.StringValue(route.DestinationCidrBlock)
					} else if aws.StringValue(route.DestinationPrefixListId) != "" {
						dest = aws.StringValue(route.DestinationPrefixListId)
					}

					if dest == "" {
						continue
					}
					if aws.StringValue(route.TransitGatewayId) != *attachment.TransitGatewayId {
						continue
					}
					found := false
					for _, d := range mtga.Routes {
						if d == dest {
							found = true
							break
						}
					}
					if !found {
						mtga.Routes = append(mtga.Routes, dest)
					}
				}
			}
		}

		results = append(results, mtga)
	}

	buf, err := json.Marshal(results)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleCreds = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleAccountLogin but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]
	r.ParseForm()
	region := r.FormValue("region")
	if region == "" {
		regions, err := s.getPreferredRegions(r, accountID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting region list: %s", err), http.StatusInternalServerError)
			return
		}
		region = regions[0]
	}
	asUser := s.getSession(r).Username
	// Get fresh creds instead of cached creds because these are being returned to the user
	creds, err := s.CredentialService.GetAWSCredentials(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(creds)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Cache-Control", "private, max-age=600")
	fmt.Fprintf(w, "%s", buf)
}

var handleConsoleLogin = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleAccountLogin but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]
	r.ParseForm()
	region := r.FormValue("region")
	if region == "" {
		regions, err := s.getPreferredRegions(r, accountID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting region list: %s", err), http.StatusInternalServerError)
			return
		}
		region = regions[0]
	}
	vpcID := r.FormValue("vpc")
	asUser := s.getSession(r).Username
	creds, err := s.CachedCredentials.GetAWSCredentials(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
		return
	}

	isGovCloud := false
	for _, account := range s.getSession(r).AuthorizedAccounts {
		if account.ID == accountID {
			isGovCloud = account.IsGovCloud
			break
		}
	}

	destinationURL := fmt.Sprintf("https://console.aws.amazon.com/vpc/home?region=%s", region)
	if isGovCloud {
		destinationURL = fmt.Sprintf("https://console.amazonaws-us-gov.com/vpc/home?region=%s", region)
	}
	if vpcID != "" {
		destinationURL += fmt.Sprintf("#vpcs:search=%s;sort=VpcId", vpcID)
	}
	// This is the link shown to the user when their session expires.
	issuerURL := (&url.URL{
		Scheme: "https",
		Host:   r.Host,
		Path:   fmt.Sprintf("%saccount/%s", s.PathPrefix, accountID),
	}).String()
	url, err := awscreds.CreateSignInURL(credentials.NewStaticCredentials(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken), destinationURL, issuerURL, isGovCloud)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting sign-in URL: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Location", url)
	// The login link is supposed to be good for 15 minutes so tell browsers to cache it for up to 10.
	w.Header().Add("Cache-Control", "private, max-age=600")
	w.WriteHeader(http.StatusFound)
}

var handleUserDetails = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 1 {
		log.Printf("Expected 1 additional arg to handleUserDetails but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	accountID := args[0]

	account, err := s.ModelsManager.GetAccount(accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading account from database: %s", err), http.StatusInternalServerError)
		return
	}

	r.ParseForm()
	region := r.Form.Get("region")
	if region == "" {
		region = s.getRegions(account.IsGovCloud)[0]
	}

	asUser := s.getSession(r).Username
	sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting AWS Session: %s", err), http.StatusInternalServerError)
		return
	}
	out, err := sts.New(sess).GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting caller identity: %s", err), http.StatusInternalServerError)
		return
	}
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error marshalling: %s", err), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleVPCDetails = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 3 {
		log.Printf("Expected 3 additional args to handleVPCDetails but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	accountID := args[0]
	region := args[1]
	vpcID := args[2]
	r.ParseForm()

	vpc, err := s.ModelsManager.GetVPC(database.Region(region), vpcID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading VPC from database: %s", err), http.StatusInternalServerError)
		return
	}
	if vpc.State == nil || vpc.State.VPCType == database.VPCTypeException {
		http.Error(w, fmt.Sprintf("VPC %s is not automated", vpcID), http.StatusBadRequest)
		return
	}

	if accountID != vpc.AccountID {
		http.Error(w, fmt.Sprintf("VPC %s account ID %s does not match the provided account ID %s", vpc.ID, vpc.AccountID, accountID), http.StatusBadRequest)
		return
	}

	account, err := s.ModelsManager.GetAccount(accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading account from database: %s", err), http.StatusInternalServerError)
		return
	}

	vpcInfo := &VPCInfo{
		AccountID:         account.ID,
		AccountName:       account.Name,
		ProjectName:       account.ProjectName,
		IsGovCloud:        account.IsGovCloud,
		IsLegacy:          vpc.State.VPCType == database.VPCTypeLegacy,
		VPCType:           &vpc.State.VPCType,
		AvailabilityZones: make(map[string]string),

		VPCID:  vpc.ID,
		Name:   vpc.Name,
		Stack:  vpc.Stack,
		Issues: vpc.Issues,
		Config: VPCConfig{
			ConnectPublic:                      vpc.Config.ConnectPublic,
			ConnectPrivate:                     vpc.Config.ConnectPrivate,
			ManagedTransitGatewayAttachmentIDs: vpc.Config.ManagedTransitGatewayAttachmentIDs,
			ManagedResolverRuleSetIDs:          vpc.Config.ManagedResolverRuleSetIDs,
			PeeringConnections:                 vpc.Config.PeeringConnections,
			SecurityGroupSetIDs:                vpc.Config.SecurityGroupSetIDs,
		},
	}
	subnetIDToName := make(map[string]string)
	var primaryIPNet *net.IPNet
	subnetIDToSubnet := make(map[string]*ec2.Subnet)
	var cmsnetConnections *cmsnet.ConnectionData
	var cmsnetNATs *cmsnet.NATData
	firewallEndpointByAZ := make(map[string]string)

	asUser := s.getSession(r).Username

	if len(r.Form["dbOnly"]) == 0 {
		sess, err := s.CachedCredentials.GetAWSSession(accountID, region, asUser)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error connecting to AWS: %s", err), http.StatusInternalServerError)
			return
		}
		ec2svc := ec2.New(sess)

		out, err := ec2svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: []*string{aws.String(vpcID)},
				},
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Error listing subnets: %s", err), http.StatusInternalServerError)
			return
		}

		vpcOut, err := ec2svc.DescribeVpcs(&ec2.DescribeVpcsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("vpc-id"),
					Values: []*string{aws.String(vpcID)},
				},
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Error describing VPC: %s", err), http.StatusInternalServerError)
			return
		}
		if len(vpcOut.Vpcs) > 0 {
			vpcInfo.PrimaryCIDR = aws.StringValue(vpcOut.Vpcs[0].CidrBlock)
			_, primaryIPNet, err = net.ParseCIDR(vpcInfo.PrimaryCIDR)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error parsing primary CIDR %q: %s", vpcInfo.PrimaryCIDR, err), http.StatusInternalServerError)
				return
			}

			vpcInfo.SecondaryCIDRs = []string{}
			for _, assn := range vpcOut.Vpcs[0].CidrBlockAssociationSet {
				if assn.CidrBlockState != nil && aws.StringValue(assn.CidrBlockState.State) == ec2.SubnetCidrBlockStateCodeAssociated {
					if aws.StringValue(assn.CidrBlock) != vpcInfo.PrimaryCIDR {
						vpcInfo.SecondaryCIDRs = append(vpcInfo.SecondaryCIDRs, aws.StringValue(assn.CidrBlock))
					}
				}
			}
		}

		cmsNetClient := s.getCMSNetClient(s.CachedCredentials)
		vpcInfo.CMSNetSupported = cmsNetClient.SupportsRegion(database.Region(region))
		if vpcInfo.CMSNetSupported {
			var err error
			asUser := s.getSession(r).Username
			cmsnetConnections, err = cmsNetClient.GetAllConnectionRequests(accountID, database.Region(region), vpcID, asUser)
			if err != nil {
				vpcInfo.CMSNetError = fmt.Sprintf("Error getting CMSNet connections: %s", err)
				cmsnetConnections = nil
			} else {
				var err error
				cmsnetNATs, err = cmsNetClient.GetAllNATRequests(accountID, database.Region(region), vpcID, asUser)
				if err != nil {
					vpcInfo.CMSNetError = fmt.Sprintf("Error getting CMSNet connections: %s", err)
					cmsnetConnections = nil
					cmsnetNATs = nil
				}
			}
		}
		for _, subnet := range out.Subnets {
			subnetIDToSubnet[*subnet.SubnetId] = subnet
			for _, tag := range subnet.Tags {
				if *tag.Key == "Name" {
					subnetIDToName[*subnet.SubnetId] = *tag.Value
					break
				}
			}
		}

		if vpc.State.VPCType.HasFirewall() {
			ctx := &awsp.Context{
				VPCID: vpc.ID,
				AWSAccountAccess: &awsp.AWSAccountAccess{
					Session: sess,
				},
			}
			result, err := ctx.GetFirewallEndpointIDByAZ()
			if err != nil {
				if aerr, ok := err.(awserr.Error); ok {
					if aerr.Code() == networkfirewall.ErrCodeResourceNotFoundException {
						// no-op: either the firewall is still being created, or it was manually deleted
					} else {
						http.Error(w, fmt.Sprintf("Error getting firewall endpoint by AZ: %s", err), http.StatusInternalServerError)
						return
					}
				} else {
					http.Error(w, fmt.Sprintf("Error getting firewall endpoint by AZ: %s", err), http.StatusInternalServerError)
					return
				}
			} else {
				firewallEndpointByAZ = result
			}
		}
		azOut, err := ec2svc.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("region-name"),
					Values: aws.StringSlice([]string{string(vpc.Region)}),
				},
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("Error describing availability zones: %s", err), http.StatusInternalServerError)
			return
		}
		for _, az := range azOut.AvailabilityZones {
			vpcInfo.AvailabilityZones[aws.StringValue(az.ZoneName)] = aws.StringValue(az.ZoneId)
		}

		customRoutes, err := s.findCustomRoutesOnPublicRouteTables(ec2svc, vpc)
		if err != nil {
			log.Printf("Error checking for custom routes on public route tables: %s", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		if len(customRoutes) > 0 {
			routeJSON, err := json.MarshalIndent(customRoutes, "", "  ")
			if err != nil {
				log.Printf("Error marshalling custom routes: %s", err)
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}
			vpcInfo.CustomPublicRoutes = string(routeJSON)
		}
	}

	uniqueGroups := []SubnetGroupInfo{}
	for azName, az := range vpc.State.AvailabilityZones {
		attachedMTGAs := map[uint64]string{} // mtga id -> tgw id
		for _, subnet := range az.Subnets[database.SubnetTypePrivate] {
			for _, tga := range vpc.State.TransitGatewayAttachments {
				for _, subnetID := range tga.SubnetIDs {
					if subnet.SubnetID == subnetID {
						for _, managedID := range tga.ManagedTransitGatewayAttachmentIDs {
							attachedMTGAs[managedID] = tga.TransitGatewayID
						}
					}
				}
			}
		}
		var firewallEndpoint string
		if vpc.State.VPCType.HasFirewall() {
			firewallEndpoint = firewallEndpointByAZ[azName]
		}
		for subnetType, subnets := range az.Subnets {
			for _, subnet := range subnets {
				var rtID string
				if subnetType == database.SubnetTypePublic {
					if vpc.State.VPCType.HasFirewall() {
						rtID = az.PublicRouteTableID
					} else if vpc.State.VPCType == database.VPCTypeV1 {
						rtID = vpc.State.PublicRouteTableID
					}
				} else if subnet.CustomRouteTableID != "" {
					rtID = subnet.CustomRouteTableID
				} else {
					rtID = az.PrivateRouteTableID
				}

				unique := true
				for _, subnetInfo := range uniqueGroups {
					if subnetInfo.Name == subnet.GroupName {
						unique = false
					}
				}
				if unique {
					uniqueGroups = append(uniqueGroups, SubnetGroupInfo{Name: subnet.GroupName, SubnetType: subnetType})
				}

				subnetInfo := SubnetInfo{
					SubnetID:  subnet.SubnetID,
					Name:      subnetIDToName[subnet.SubnetID],
					GroupName: subnet.GroupName,
					Type:      string(subnetType),
					IsManaged: true,
				}
				delete(subnetIDToName, subnet.SubnetID)

				rtInfo, ok := vpc.State.RouteTables[rtID]
				if ok {
					for _, route := range rtInfo.Routes {
						if route.Destination == "0.0.0.0/0" {
							if route.NATGatewayID != "" && route.NATGatewayID == az.NATGateway.NATGatewayID {
								subnetInfo.IsConnectedToInternet = true
							} else if route.InternetGatewayID != "" && route.InternetGatewayID == vpc.State.InternetGateway.InternetGatewayID {
								subnetInfo.IsConnectedToInternet = true
							} else if vpc.State.VPCType.HasFirewall() && route.VPCEndpointID != "" && route.VPCEndpointID == firewallEndpoint {
								subnetInfo.IsConnectedToInternet = true
							}
						}
						if route.TransitGatewayID != "" {
							for mtgaID, tgwID := range attachedMTGAs {
								if route.TransitGatewayID == tgwID && !uint64InSlice(mtgaID, subnetInfo.ConnectedManagedTransitGatewayAttachments) {
									subnetInfo.ConnectedManagedTransitGatewayAttachments = append(subnetInfo.ConnectedManagedTransitGatewayAttachments, mtgaID)
								}
							}
						}
					}
				}

				if primaryIPNet != nil {
					sn := subnetIDToSubnet[subnet.SubnetID]
					if sn != nil {
						subnetCIDR := aws.StringValue(sn.CidrBlock)
						subnetInfo.CIDR = subnetCIDR
						_, subnetIPNet, err := net.ParseCIDR(subnetCIDR)
						if err != nil {
							http.Error(w, fmt.Sprintf("Error parsing subnet CIDR %q: %s", subnetCIDR, err), http.StatusInternalServerError)
							return
						}
						subnetInfo.InPrimaryCIDR = cidrIsWithin(subnetIPNet, primaryIPNet)
					}
				}
				if cmsnetConnections != nil {
					activationsCreated := map[string]*CMSNetConnection{}
					for _, activation := range cmsnetConnections.Activations {
						if activation.SubnetID == subnet.SubnetID {
							info := &CMSNetConnection{
								CIDR:   activation.DestinationCIDR,
								VRF:    activation.VRF,
								Status: "Connected",
							}
							subnetInfo.CMSNetConnections = append(subnetInfo.CMSNetConnections, info)
							activationsCreated[activation.RequestID] = info
						}
					}
					for _, req := range cmsnetConnections.Requests {
						activation := activationsCreated[req.ID]
						creationIsInProgress := req.ConnectionStatus != cmsnet.ConnectionStatusSuccessful
						deleteWasRequested := req.DeletionStatus != ""
						needsUpdates := activation == nil || creationIsInProgress || deleteWasRequested
						if req.Params.SubnetID == subnet.SubnetID && needsUpdates {
							if activation == nil {
								// Request that hasn't been fulfilled yet
								activation = &CMSNetConnection{
									CIDR: req.Params.DestinationCIDR,
									VRF:  req.Params.VRF,
								}
								subnetInfo.CMSNetConnections = append(subnetInfo.CMSNetConnections, activation)
							}
							messages := req.ConnectionMessages
							status := "Connecting: " + string(req.ConnectionStatus)
							failureMessage := req.ConnectionFailureMessage
							if req.DeletionStatus == cmsnet.DeletionStatusSuccessful {
								status = "Deleted"
								messages = []string{}
							} else if deleteWasRequested {
								status = "Deleting: " + string(req.DeletionStatus)
								messages = req.DeletionMessages
								failureMessage = req.DeletionFailureMessage
							}
							activation.Status = status
							if failureMessage != "" {
								activation.LastMessage = failureMessage
							} else if len(messages) > 0 {
								activation.LastMessage = messages[len(messages)-1]
							}
						}
					}
				}
				_, ipNet, err := net.ParseCIDR(subnetInfo.CIDR)
				if err != nil {
					vpcInfo.CMSNetError = fmt.Sprintf("Unable to parse subnet CIDR %q", subnetInfo.CIDR)
				} else {
					if cmsnetNATs != nil {
						natsCreated := map[string]*CMSNetNAT{}
						for _, nat := range cmsnetNATs.NATs {
							ip, _, err := net.ParseCIDR(nat.InsideNetwork)
							if err != nil {
								vpcInfo.CMSNetError = fmt.Sprintf("Unable to parse insideNetwork CIDR %q", nat.InsideNetwork)
							} else if ipNet.Contains(ip) {
								info := &CMSNetNAT{
									InsideNetwork:  nat.InsideNetwork,
									OutsideNetwork: nat.OutsideNetwork,
									Status:         "Connected",
								}
								subnetInfo.CMSNetNATs = append(subnetInfo.CMSNetNATs, info)
								natsCreated[nat.RequestID] = info
							}
						}
						for _, req := range cmsnetNATs.Requests {
							nat := natsCreated[req.ID]
							creationIsInProgress := req.ConnectionStatus != cmsnet.ConnectionStatusSuccessful
							deleteWasRequested := req.DeletionStatus != ""
							needsUpdates := nat == nil || creationIsInProgress || deleteWasRequested
							if !needsUpdates {
								// No need to gather progress information from the request object
								continue
							}
							ip, _, err := net.ParseCIDR(req.Params.InsideNetwork)
							if err != nil {
								vpcInfo.CMSNetError = fmt.Sprintf("Unable to parse insideNetwork CIDR %q", req.Params.InsideNetwork)
							} else if ipNet.Contains(ip) {
								if nat == nil {
									// Request that hasn't been fulfilled yet
									nat = &CMSNetNAT{
										InsideNetwork:  req.Params.InsideNetwork,
										OutsideNetwork: req.Params.OutsideNetwork,
									}
									subnetInfo.CMSNetNATs = append(subnetInfo.CMSNetNATs, nat)
								}
								messages := req.ConnectionMessages
								status := "Connecting: " + string(req.ConnectionStatus)
								failureMessage := req.ConnectionFailureMessage
								if req.DeletionStatus == cmsnet.DeletionStatusSuccessful {
									status = "Deleted"
									messages = []string{}
								} else if deleteWasRequested {
									status = "Deleting: " + string(req.DeletionStatus)
									messages = req.DeletionMessages
									failureMessage = req.DeletionFailureMessage
								}
								nat.Status = status
								if failureMessage != "" {
									nat.LastMessage = failureMessage
								} else if len(messages) > 0 {
									nat.LastMessage = messages[len(messages)-1]
								}
							}
						}
					}
				}

				vpcInfo.Subnets = append(vpcInfo.Subnets, subnetInfo)
			}
		}
	}

	// Any remaining subnets are unmanaged
	for ID, Name := range subnetIDToName {
		vpcInfo.Subnets = append(vpcInfo.Subnets, SubnetInfo{
			SubnetID: ID,
			Name:     Name,
		})
	}

	vpcInfo.SubnetGroups = uniqueGroups

	tasks, isMore, err := s.TaskDatabase.GetVPCTasks(accountID, vpcID)
	vpcInfo.IsMoreTasks = isMore
	if err != nil {
		log.Printf("Error getting tasks: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, t := range tasks {
		vpcInfo.Tasks = append(vpcInfo.Tasks, TaskInfo{
			ID:          t.ID,
			AccountID:   t.AccountID,
			VPCID:       t.VPCID,
			VPCRegion:   t.VPCRegion,
			Description: t.Description,
			Status:      t.Status.String(),
		})
	}

	buf, err := json.Marshal(vpcInfo)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

var handleLogout = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	sessionKey := ""
	for _, cookie := range r.Cookies() {
		if cookie.Name == cookieSessionKey {
			sessionKey = cookie.Value
			break
		}
	}

	if sessionKey != "" {
		err := s.SessionStore.Delete(sessionKey)
		if err != nil {
			log.Printf("Error deleting session: %s", err)
		}
	}
	http.Redirect(w, r, s.PathPrefix, http.StatusFound)
}

var handleOauthCallback = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	err := s.templateAuth().Execute(w, s.AzureAD.AzureADConfig)
	if err != nil {
		log.Printf("Error executing templateAuth: %s", err)
	}
}

type UserInfo = struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"isAdmin"`
}

const bearerPrefix string = "Bearer "

var handleOauthValidate = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	bearerToken := r.Header.Get("Authorization")
	if bearerToken == "" || !strings.HasPrefix(bearerToken, bearerPrefix) {
		http.Error(w, fmt.Sprintf("Error authenticating: %s", fmt.Errorf("missing bearer token")), http.StatusBadRequest)
		return
	}

	token := strings.TrimPrefix(bearerToken, bearerPrefix)
	userDetails, err := s.AzureAD.VerifyToken(token)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error authenticating: %s", err), http.StatusBadRequest)
		return
	}

	accessToken := r.Header.Get("X-Access")
	if accessToken == "" || !strings.HasPrefix(bearerToken, bearerPrefix) {
		http.Error(w, fmt.Sprintf("Error authenticating: %s", fmt.Errorf("missing access token")), http.StatusBadRequest)
		return
	}
	access := strings.TrimPrefix(accessToken, bearerPrefix)

	groups, err := s.AzureAD.GetGraphGroups(access)
	if err != nil {
		http.Error(w, fmt.Sprintf("unable to fetch user groups: %s", err), http.StatusBadRequest)
	}

	isAdmin := groups.Has("ct-gss-network") // this may need to be ct-gss-network-admin or the new AD group
	if !isAdmin {
		isReadOnly := groups.Has("ct-gss-network-vpcconf-viewer") || groups.Has("ct-gss-onboarding-readonly")
		if !isReadOnly {
			http.Error(w, fmt.Sprintf("User %s (%s) is not authorized to use this application", userDetails.PreferredUsername, userDetails.Email), http.StatusForbidden)
			return
		}
	}

	accounts, err := s.ModelsManager.GetAllAWSAccounts()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting authorized accounts: %s", err), http.StatusInternalServerError)
		return
	}

	sessionKey := ""
	for _, cookie := range r.Cookies() {
		if cookie.Name == cookieSessionKey {
			sessionKey = cookie.Value
			break
		}
	}
	if sessionKey == "" {
		http.Error(w, fmt.Sprintf("no session cookie found: %s", err), http.StatusInternalServerError)
		return
	}

	sess := &database.Session{
		Key:                sessionKey,
		UserID:             0, // fill this out or use a different field / no int compatible user IDs for AD auth
		IsAdmin:            isAdmin,
		AuthorizedAccounts: accounts,
		Username:           userDetails.PreferredUsername,
	}

	err = s.SessionStore.Extend(sess)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error extending session: %s", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[LOGIN] %s (%s)", userDetails.PreferredUsername, userDetails.Email)

	userInfo := &UserInfo{
		Name:     userDetails.Name,
		Email:    userDetails.Email,
		Username: userDetails.PreferredUsername,
		IsAdmin:  isAdmin,
	}

	buf, err := json.Marshal(userInfo)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s", buf)
}

type contextKey string

const (
	sessionContextKey contextKey = "ipam-web-context-session-key"
	authContextKey    contextKey = "ipam-web-context-auth-key"
	useCredentialAuth string     = "useCredentialAuth"
)

func (s *Server) getSession(r *http.Request) *database.Session {
	return r.Context().Value(sessionContextKey).(*database.Session)
}

const cookieSessionKey = "sessionID"

var readOnlyHandlers = []*func(*Server, http.ResponseWriter, *http.Request, ...string){
	&handleAccountList, &handleAccountDetails, &handleAccountTask,
	&handleBatchTasks,
	&handleConsoleLogin,
	&handleCreds,
	&handleDashboard,
	&handleListVPCs,
	&handleGetLabels,
	&handleGetAccountLabels,
	&handleListBatchVPCLabels,
	&handleGetVPCLabels,
	&handleManagedTransitGatewayAttachmentList,
	&handleManagedResolverRuleSetList,
	&handleSearch,
	&handleSecurityGroupSetList,
	&handleTasks,
	&handleVPCLastSubTasks,
	&handleVPCDetails,
	&handleVPCRequestList,
	&handleGetVPCRequest,
	&handleVPCTask,
	&handleIPUsageList,
	&handleGetTask,
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path[len(s.PathPrefix):]

	apiKeyValid := false

	// Assume this is an APIKey request if there is an Authorization header
	// and the path isn't part of the Azure AD login sequence.
	if req.Header.Get("Authorization") != "" && path != "oauth/validate" {
		result := s.APIKey.Validate(req)
		apiKeyValid = result.IsValid()
		if !apiKeyValid {
			http.Error(w, result.Error.Error(), result.StatusCode)
			return
		}

		var sess *database.Session
		sessionKey := s.getSessionKeyForPrincipal(result.Principal)
		if sessionKey != "" {
			var err error
			sess, err = s.SessionStore.Get(sessionKey)
			if err != nil && err != session.ErrNotFound && err != session.ErrExpired {
				http.Error(w, fmt.Sprintf("Error fetching session: %s", err), http.StatusInternalServerError)
				return
			}
		}
		if sess == nil {
			sess = &database.Session{
				UserID:   -1,
				Username: result.Principal,
				IsAdmin:  true,
			}

			accounts, err := s.ModelsManager.GetAllAWSAccounts()
			if err != nil {
				http.Error(w, fmt.Sprintf("Error getting authorized accounts: %s", err), http.StatusInternalServerError)
				return
			}

			for _, account := range accounts {
				sess.AuthorizedAccounts = append(sess.AuthorizedAccounts, &database.AWSAccount{
					ID:          account.ID,
					Name:        account.Name,
					ProjectName: account.ProjectName,
					IsGovCloud:  account.IsGovCloud,
				})
			}
			err = s.SessionStore.Create(sess)
			if err != nil {
				http.Error(w, fmt.Sprintf("Error creating session: %s", err), http.StatusInternalServerError)
				return
			}
			s.putPrincipalSessionKey(result.Principal, sess.Key)
		}

		req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, sess))
	}

	var matchedRoute *route
	var args []string
	for _, r := range routes {
		if req.Method == r.method {
			match := r.regexp.FindStringSubmatch(path)
			if match != nil {
				matchedRoute = r
				args = match[1:]
				break
			}
		}
	}
	if matchedRoute == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if matchedRoute.requiresAuth && !apiKeyValid {
		sessionKey := ""
		for _, cookie := range req.Cookies() {
			if cookie.Name == cookieSessionKey {
				sessionKey = cookie.Value
				break
			}
		}

		var sess *database.Session
		var err error
		if sessionKey != "" {
			sess, err = s.SessionStore.Get(sessionKey)
			if err == session.ErrExpired {
				// session can be extended
			} else if err == session.ErrNotFound {
				// force a new session to be generated
				sessionKey = ""
			} else if err != nil {
				log.Printf("Failed to fetch session: %s", err)
			}
		}

		if sess == nil {
			// if there is no session key then generate a new one for unauthenticated users
			// this enables the existing tab to know the ID of the authenticated session from
			// the new tab
			if sessionKey == "" {
				newSessionKey, err := s.SessionStore.CreateUnauthenticated()
				if err != nil {
					http.Error(w, "Session ID error", http.StatusInternalServerError)
					return
				}
				http.SetCookie(w, &http.Cookie{
					Name:  cookieSessionKey,
					Value: newSessionKey,
					Path:  "/",
				})
			}
			http.Error(w, "Authorization Required", http.StatusUnauthorized)
			return
		}

		if !sess.IsAdmin {
			allowed := false
			for _, handler := range readOnlyHandlers {
				if handler == matchedRoute.handler {
					allowed = true
				}
			}
			if !allowed {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		req = req.WithContext(context.WithValue(req.Context(), sessionContextKey, sess))
	}

	(*matchedRoute.handler)(s, w, req, args...)
}

var handleDashboard = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleDashboard but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	dashboard, err := s.ModelsManager.GetDashboard()
	if err != nil {
		log.Printf("Error fetching dashboard: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(dashboard)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleIPUsageList = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleIPUsageList but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	usage, err := s.ModelsManager.GetIPUsage()
	if err != nil {
		log.Printf("Error fetching IPUsage: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(usage)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleRefreshIPUsage = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 additional args to handleRefreshIPUsage but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	ipUsage, err := s.IPAM.GetIPUsage()
	if err != nil {
		log.Printf("Error generating IPUsage: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	err = s.ModelsManager.UpdateIPUsage(ipUsage)
	if err != nil {
		log.Printf("Error updating IPUsage: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", "null")
}

var handleSearch = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 0 {
		log.Printf("Expected 0 args to handleSearch but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	searchCriteria := &struct {
		SearchTerm string
	}{}

	err := json.NewDecoder(r.Body).Decode(&searchCriteria)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %s", err), http.StatusBadRequest)
		return
	}

	searchManager := &search.SearchManager{
		DB:         s.DB,
		PathPrefix: s.PathPrefix,
	}

	session := s.getSession(r)

	searchResult, err := search.Search(searchManager, searchCriteria.SearchTerm, session.AuthorizedAccounts)
	if err != nil {
		log.Printf("Error searching: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(searchResult)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%s", buf)
}

var handleVPCLastSubTasks = func(s *Server, w http.ResponseWriter, r *http.Request, args ...string) {
	if len(args) != 2 {
		log.Printf("Expected 2 additional args to handleVPCLastSubTasks but got %d", len(args))
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	region := database.Region(args[0])
	vpcID := args[1]

	statuses, err := s.TaskDatabase.GetLastSubtaskStatuses(region, vpcID)
	if err != nil {
		log.Printf("Error fetching sub task statuses: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	buf, err := json.Marshal(statuses)
	if err != nil {
		log.Printf("Error marshalling: %s", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "%s", buf)
}

func (s *Server) scheduleDependentVPCTask(taskData *database.TaskData, taskName, accountID, vpcID string, prereqID uint64) (uint64, error) {
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		return 0, fmt.Errorf("Error marshaling: %s", err)
	}
	t, err := s.TaskDatabase.AddDependentVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, prereqID, nil)
	if err != nil {
		return 0, fmt.Errorf("Error adding task: %s", err)
	}
	return t.ID, nil
}

func (s *Server) scheduleV1ToV1FirewallTasks(vpcID, accountID, asUser string, region database.Region, vpc *database.VPC) (uint64, error) {
	// Update VPC Type (MigratingV1ToV1Firewall)
	updateTypeData := &database.UpdateVPCTypeTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeMigratingV1ToV1Firewall,
	}
	taskData := &database.TaskData{
		UpdateVPCTypeTaskData: updateTypeData,
		AsUser:                asUser,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		return 0, fmt.Errorf("Error marshaling: %s", err)
	}
	taskName := fmt.Sprintf("Update VPC %s type", vpcID)
	firstTask, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		return 0, fmt.Errorf("Error adding task: %s", err)
	}

	// Add Zoned Subnets
	addSubnetsData := &database.AddZonedSubnetsTaskData{
		VPCID:        vpcID,
		Region:       region,
		SubnetType:   database.SubnetTypeFirewall,
		SubnetSize:   ipcontrol.FirewallSubnetSize,
		GroupName:    "firewall",
		BeIdempotent: true,
	}
	taskData = &database.TaskData{
		AddZonedSubnetsTaskData: addSubnetsData,
		AsUser:                  asUser,
	}
	prereqID, err := s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Adding firewall subnets to %s", vpcID), accountID, vpcID, firstTask.ID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Update Networking
	updateNetworkingData := &database.UpdateNetworkingTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		NetworkingConfig: database.NetworkingConfig{
			ConnectPublic:                      vpc.Config.ConnectPublic,
			ConnectPrivate:                     vpc.Config.ConnectPrivate,
			ManagedTransitGatewayAttachmentIDs: vpc.Config.ManagedTransitGatewayAttachmentIDs,
			PeeringConnections:                 vpc.Config.PeeringConnections,
		},
	}
	taskData = &database.TaskData{
		UpdateNetworkingTaskData: updateNetworkingData,
		AsUser:                   asUser,
	}
	prereqID, err = s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Update VPC %s networking", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Update Logging
	updateLoggingData := &database.UpdateLoggingTaskData{
		VPCID:  vpcID,
		Region: region,
	}
	taskData = &database.TaskData{
		UpdateLoggingTaskData: updateLoggingData,
		AsUser:                asUser,
	}
	prereqID, err = s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Update VPC %s logging", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Delete Unused Resources
	deleteUnusedResourcesData := &database.DeleteUnusedResourcesTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeMigratingV1ToV1Firewall,
	}
	taskData = &database.TaskData{
		DeleteUnusedResourcesTaskData: deleteUnusedResourcesData,
		AsUser:                        asUser,
	}
	prereqID, err = s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Delete unused resources for VPC %s", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Update VPC Type (V1Firewall)
	updateTypeData = &database.UpdateVPCTypeTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeV1Firewall,
	}
	taskData = &database.TaskData{
		UpdateVPCTypeTaskData: updateTypeData,
		AsUser:                asUser,
	}
	lastTaskID, err := s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Update VPC %s type", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	return lastTaskID, nil
}

// Schedule tasks, each dependent on the previous task, and return the task ID of the last task
func (s *Server) scheduleV1FirewallToV1Tasks(vpcID, accountID, asUser string, region database.Region, vpc *database.VPC) (uint64, error) {
	// Update VPC Type (MigratingV1FirewallToV1)
	updateTypeData := &database.UpdateVPCTypeTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeMigratingV1FirewallToV1,
	}
	taskData := &database.TaskData{
		UpdateVPCTypeTaskData: updateTypeData,
		AsUser:                asUser,
	}
	taskBytes, err := json.Marshal(taskData)
	if err != nil {
		return 0, fmt.Errorf("Error marshaling: %s", err)
	}
	taskName := fmt.Sprintf("Update VPC %s type", vpcID)
	firstTask, err := s.TaskDatabase.AddVPCTask(accountID, vpcID, taskName, taskBytes, database.TaskStatusQueued, nil)
	if err != nil {
		return 0, fmt.Errorf("Error adding task: %s", err)
	}

	// Update Networking
	updateNetworkingData := &database.UpdateNetworkingTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		NetworkingConfig: database.NetworkingConfig{
			ConnectPublic:                      vpc.Config.ConnectPublic,
			ConnectPrivate:                     vpc.Config.ConnectPrivate,
			ManagedTransitGatewayAttachmentIDs: vpc.Config.ManagedTransitGatewayAttachmentIDs,
			PeeringConnections:                 vpc.Config.PeeringConnections,
		},
	}
	taskData = &database.TaskData{
		UpdateNetworkingTaskData: updateNetworkingData,
		AsUser:                   asUser,
	}
	prereqID, err := s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Update VPC %s networking", vpcID), accountID, vpcID, firstTask.ID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Delete Unused Resources
	deleteUnusedResourcesData := &database.DeleteUnusedResourcesTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeMigratingV1ToV1Firewall,
	}
	taskData = &database.TaskData{
		DeleteUnusedResourcesTaskData: deleteUnusedResourcesData,
		AsUser:                        asUser,
	}
	prereqID, err = s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Delete unused resources for VPC %s", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Remove Zoned Subnets
	removeZonedSubnetsData := &database.RemoveZonedSubnetsTaskData{
		VPCID:        vpcID,
		Region:       region,
		GroupName:    "firewall",
		SubnetType:   database.SubnetTypeFirewall,
		BeIdempotent: true,
	}
	taskData = &database.TaskData{
		RemoveZonedSubnetsTaskData: removeZonedSubnetsData,
		AsUser:                     asUser,
	}
	prereqID, err = s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Removing firewall subnets from VPC %s", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	// Update VPC Type (V1)
	updateTypeData = &database.UpdateVPCTypeTaskData{
		VPCID:     vpcID,
		AWSRegion: region,
		VPCType:   database.VPCTypeV1,
	}
	taskData = &database.TaskData{
		UpdateVPCTypeTaskData: updateTypeData,
		AsUser:                asUser,
	}
	lastTaskID, err := s.scheduleDependentVPCTask(taskData, fmt.Sprintf("Update VPC %s type", vpcID), accountID, vpcID, prereqID)
	if err != nil {
		return 0, fmt.Errorf("Error scheduling dependent VPC task: %s", err)
	}

	return lastTaskID, nil
}

func (s *Server) findCustomRoutesOnPublicRouteTables(ec2svc *ec2.EC2, vpc *database.VPC) (map[string][]*ec2.Route, error) {
	customRoutes := make(map[string][]*ec2.Route)
	for rtID, rtInfo := range vpc.State.RouteTables {
		if rtInfo.SubnetType == database.SubnetTypePublic {
			out, err := ec2svc.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("route-table-id"),
						Values: []*string{aws.String(rtID)},
					},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("Error describing route table %s: %s", rtID, err)
			}

			if len(out.RouteTables) != 1 {
				// If the VPC is migrating, some RTs still in state might not be found in AWS
				return nil, nil
			}

			for _, route := range out.RouteTables[0].Routes {
				if aws.StringValue(route.Origin) != ec2.RouteOriginCreateRoute {
					continue
				}
				found := false
				for _, info := range rtInfo.Routes {
					if routeInfoIsEqualToRoute(info, route) {
						found = true
					}
				}
				if !found {
					if customRoutes[rtID] == nil {
						customRoutes[rtID] = []*ec2.Route{}
					}
					customRoutes[rtID] = append(customRoutes[rtID], route)
				}
			}
		}
	}

	return customRoutes, nil
}

func routeInfoIsEqualToRoute(info *database.RouteInfo, route *ec2.Route) bool {
	var routeDestination string

	if aws.StringValue(route.DestinationCidrBlock) != "" {
		routeDestination = aws.StringValue(route.DestinationCidrBlock)
	} else if aws.StringValue(route.DestinationPrefixListId) != "" {
		routeDestination = aws.StringValue(route.DestinationPrefixListId)
	}

	destIsEqual := info.Destination == routeDestination

	natIsEqual := info.NATGatewayID == aws.StringValue(route.NatGatewayId)
	tgwIsEqual := info.TransitGatewayID == aws.StringValue(route.TransitGatewayId)
	pcxIsEqual := info.PeeringConnectionID == aws.StringValue(route.VpcPeeringConnectionId)

	gatewayIsEqual := false
	gatewayID := aws.StringValue(route.GatewayId)
	if awsp.IsInternetGatewayID(gatewayID) && info.InternetGatewayID == aws.StringValue(route.GatewayId) {
		gatewayIsEqual = true
	} else if awsp.IsVPCEndpointID(gatewayID) && info.VPCEndpointID == aws.StringValue(route.GatewayId) {
		gatewayIsEqual = true
	} else if gatewayID == "" && info.VPCEndpointID == "" && info.InternetGatewayID == "" {
		gatewayIsEqual = true
	}

	return destIsEqual && gatewayIsEqual && natIsEqual && tgwIsEqual && pcxIsEqual
}

func (s *Server) getSessionKeyForPrincipal(principal string) string {
	s.apiKeyRWMu.RLock()
	defer s.apiKeyRWMu.RUnlock()
	return s.apiKeySessions[principal]
}

func (s *Server) putPrincipalSessionKey(principal, key string) {
	s.apiKeyRWMu.Lock()
	defer s.apiKeyRWMu.Unlock()
	if s.apiKeySessions == nil {
		s.apiKeySessions = map[string]string{}
	}

	s.apiKeySessions[principal] = key
}
