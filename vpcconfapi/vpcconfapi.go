package vpcconfapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

// VPCConfAPI core
type VPCConfAPI struct {
	APIKey             string
	Username, Password string
	SessionID          string
	BaseURL            string
	Expiry             *time.Time
}

//VPC exceptions from VPC Conf
type VPC struct {
	ID        string
	AccountID string
	Name      string
	Stack     string
	Region    database.Region
}

// VerifyBatchTask container
type VerifyBatchTask struct {
	TaskTypes  database.TaskTypes
	VerifySpec database.VerifySpec
	VPCs       []VPC
}

// BatchTaskResult returns the ID
type BatchTaskResult struct {
	BatchTaskID int
}

// BatchTaskInfo same as vpc-conf
type BatchTaskInfo struct {
	ID          int
	Description string
	AddedAt     time.Time
	Tasks       []*Task
}

// GetBatchTaskProgress returns the current state of this BatchTaskInfo
func (bti *BatchTaskInfo) GetBatchTaskProgress() *BatchTaskProgress {
	progress := &BatchTaskProgress{}

	for _, task := range bti.Tasks {
		if task.Status == database.TaskStatusCancelled.String() {
			progress.Cancelled++
		} else if task.Status == database.TaskStatusFailed.String() {
			progress.Failed++
		} else if task.Status == database.TaskStatusInProgress.String() {
			progress.InProgress++
		} else if task.Status == database.TaskStatusQueued.String() {
			progress.Queued++
		} else if task.Status == database.TaskStatusSuccessful.String() {
			progress.Success++
		}
	}

	return progress
}

// BatchTaskProgress is the current state of BatchTaskInfo
type BatchTaskProgress struct {
	BatchTaskInfo *BatchTaskInfo
	Cancelled     int
	Failed        int
	InProgress    int
	Queued        int
	Success       int
}

func (p BatchTaskProgress) String() string {
	return fmt.Sprintf("Queued: %d, In Progress: %d, Success: %d, Cancelled: %d, Failed: %d",
		p.Queued, p.InProgress, p.Success, p.Cancelled, p.Failed)
}

// Remaining returns the number of remaining items until the task is finished
func (p BatchTaskProgress) Remaining() int {
	return p.Queued + p.InProgress
}

// Task is the subtasks for a batch task
type Task struct {
	ID          uint64
	AccountID   string
	VPCID       string
	VPCRegion   string
	Description string
	Status      string
}

func (api *VPCConfAPI) setHeader(req *http.Request) {
	req.Header.Set("Accepts", "application/json")
	if api.APIKey != "" {
		req.Header.Add("Authorization", "Bearer "+api.APIKey)
	} else if api.SessionID != "" {
		req.AddCookie(&http.Cookie{
			Name:  "sessionID",
			Value: api.SessionID,
			Path:  "/",
		})
	}
}

func (api *VPCConfAPI) doRequest(req *http.Request, jsonStruct interface{}) error {
	err := api.VerifySession()
	if err != nil {
		return err
	}

	api.setHeader(req)
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to fetch request for %q - %s", req.RequestURI, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ERROR: response for %q - %s", req.RequestURI, resp.Status)
	}

	if jsonStruct != nil {
		err = json.NewDecoder(resp.Body).Decode(&jsonStruct)
		if err != nil {
			return fmt.Errorf("Failed to decode response for %q - %s", req.RequestURI, err)
		}
	}

	return nil
}

// Login to VPC Conf and store the sessionID
func (api *VPCConfAPI) Login() error {
	loginURL := api.BaseURL + "/login"

	log.Printf("Authenticate as %q at %q", api.Username, loginURL)

	form := url.Values{
		"username": []string{api.Username},
		"password": []string{api.Password},
	}
	client := &http.Client{}
	resp, err := client.PostForm(loginURL, form)
	if err != nil {
		return fmt.Errorf("ERROR: Login to VPC-Conf failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("ERROR: Login to VPC-Conf failed: %s", resp.Status)
	}
	for _, c := range resp.Cookies() {
		if c.Name == "sessionID" {
			api.SessionID = c.Value
			expiry := time.Now().Add(time.Minute * 50)
			api.Expiry = &expiry
		}
	}

	return nil
}

// VerifySession verifies the session is valid
func (api *VPCConfAPI) VerifySession() error {
	if api.APIKey != "" {
		return nil
	}
	if api.Expiry == nil || time.Now().After(*api.Expiry) || api.SessionID == "" {
		return api.Login()
	}

	return nil
}

// GetAutomatedVPCsAndRegions from the batch task endpoint
func (api *VPCConfAPI) GetAutomatedVPCsAndRegions() ([]VPC, []string, error) {
	vpcsURL := api.BaseURL + "/batch/vpcs.json"

	log.Printf("Fetch vpcs from %q", vpcsURL)

	req, err := http.NewRequest("GET", vpcsURL, nil)
	if err != nil {
		return []VPC{}, []string{}, fmt.Errorf("Failed to create request for %q - %s", vpcsURL, err)
	}

	automatedRequest := &struct {
		Regions []string
		VPCs    []VPC
	}{}

	err = api.doRequest(req, automatedRequest)

	return automatedRequest.VPCs, automatedRequest.Regions, err
}

// SubmitVerifyBatchTask ...
func (api *VPCConfAPI) SubmitVerifyBatchTask(vpcs []VPC, verifySpec database.VerifySpec) (*BatchTaskResult, error) {
	verifyTaskURL := api.BaseURL + "/batch"

	batch := &VerifyBatchTask{TaskTypes: database.TaskTypeVerifyState, VPCs: vpcs, VerifySpec: verifySpec}

	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(batch)
	if err != nil {
		return nil, err
	}

	log.Printf("Create verify batch task for %q", verifyTaskURL)

	req, err := http.NewRequest("POST", verifyTaskURL, buf)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request for %q - %s", verifyTaskURL, err)
	}

	result := &BatchTaskResult{}

	err = api.doRequest(req, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetBatchTaskByID retrieves a single batch task instance
func (api *VPCConfAPI) GetBatchTaskByID(batchTaskID int) (*BatchTaskInfo, error) {
	batchTaskURL := fmt.Sprintf("%s/batch/task/%d", api.BaseURL, batchTaskID)

	req, err := http.NewRequest("GET", batchTaskURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request for %q - %s", batchTaskURL, err)
	}

	batchTaskInfo := &BatchTaskInfo{}

	err = api.doRequest(req, batchTaskInfo)
	if err != nil {
		return nil, err
	}

	return batchTaskInfo, nil
}

// TGWTemplate is single entry taken from vpc-conf's TGW template from mtgas.json
type TGWTemplate struct {
	ID               int
	IsGovCloud       bool
	Name             string
	TransitGatewayID string
	Routes           []string
	SubnetTypes      []string
	InUseVPCs        []string
}

func (vpc VPC) String() string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", vpc.AccountID, vpc.ID, vpc.Name, vpc.Stack, vpc.Region)
}

func isPrefixListID(dest string) bool {
	return strings.HasPrefix(dest, "pl-")
}

// GetTGWTemplate returns the template for the given name or an error
func (api *VPCConfAPI) GetTGWTemplate(targetTemplateName string) (*TGWTemplate, error) {
	mtgasURL := api.BaseURL + "/mtgas.json"

	log.Printf("Fetch transit gateway template '%s' from %s", targetTemplateName, mtgasURL)

	req, err := http.NewRequest("GET", mtgasURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request for %s - %s", mtgasURL, err)
	}

	templates := []*TGWTemplate{}

	err = api.doRequest(req, &templates)
	if err != nil {
		return nil, err
	}

	var template *TGWTemplate
	for _, t := range templates {
		if t.Name == targetTemplateName {
			template = t
		}
	}
	if template == nil {
		return nil, fmt.Errorf("No template named %s was found", targetTemplateName)
	}

	var cidrRoutes []string
	for _, r := range template.Routes {
		if !isPrefixListID(r) {
			cidrRoutes = append(cidrRoutes, r)
		}
	}
	if len(cidrRoutes) > 0 {
		return nil, fmt.Errorf("Only prefix list routes are expected on the target template. The target template contains the following CIDR routes: %v", cidrRoutes)
	}

	return template, nil
}

func (api *VPCConfAPI) GetTaskStats() (*database.TaskStats, error) {
	healthURL, err := url.Parse(api.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("Error parsing base URL %q: %s", api.BaseURL, err)
	}
	healthURL.Path = "/health"

	req, err := http.NewRequest(http.MethodGet, healthURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request for %s - %s", healthURL, err)
	}
	health := &struct {
		TaskStats *database.TaskStats
	}{}
	// No authentication needed, but sending header won't hurt
	err = api.doRequest(req, health)
	if err != nil {
		return nil, err
	}
	return health.TaskStats, nil
}

// AllowAllWorkers allows any worker to do jobs on the task queue
func (api *VPCConfAPI) AllowAllWorkers() error {
	return api.allowWorkers(nil, true)
}

// AllowNoWorkers stops the task queue
func (api *VPCConfAPI) AllowNoWorkers() error {
	return api.allowWorkers(nil, false)
}

// AllowOnlyWorkers allows only workers with the given name to do jobs on the task queue
func (api *VPCConfAPI) AllowOnlyWorkers(name string) error {
	return api.allowWorkers(&name, false)
}

func (api *VPCConfAPI) allowWorkers(allow *string, allowAll bool) error {
	allowURL := api.BaseURL + "/allowWorkers"

	formData := url.Values{}
	if allowAll {
		formData.Add("allowAll", "1")
	} else if allow != nil {
		formData.Add("allow", *allow)
	}

	req, err := http.NewRequest(http.MethodPost, allowURL, strings.NewReader(formData.Encode()))
	req.Header.Add("Content-type", "application/x-www-form-urlencoded")
	if err != nil {
		return fmt.Errorf("Failed to create request for %s - %s", allowURL, err)
	}

	return api.doRequest(req, nil)
}

// GetExceptionVPCs returns the VPCs not managed by VPC Conf
func (api *VPCConfAPI) GetExceptionVPCs() ([]VPC, error) {
	exceptionsURL := api.BaseURL + "/exception.json"
	log.Printf("Fetch exception VPCs from %s", exceptionsURL)

	req, err := http.NewRequest("GET", exceptionsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request for %s - %s", exceptionsURL, err)
	}

	vpcs := []VPC{}

	err = api.doRequest(req, &vpcs)

	return vpcs, err
}
