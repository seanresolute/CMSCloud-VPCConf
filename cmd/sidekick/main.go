package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/sidekick/internal/conf"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/cmd/sidekick/internal/database"
	vpcconf "github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

const (
	exitBadConfiguration = iota + 1
	exitVPCConfAPIError
	exitDBError
)

func observeProgress(sql *database.SQLModels, task *database.Task, vpcConfAPI *vpcconfapi.VPCConfAPI, wg *sync.WaitGroup) {
	defer wg.Done()
	lastResponse := &vpcconfapi.BatchTaskInfo{}

	for range time.Tick(1 * time.Second) {
		batchTaskInfo, err := vpcConfAPI.GetBatchTaskByID(task.BatchTaskID)
		if err != nil {
			log.Printf("Failed to fetch batch task info from vpc-conf: %s", err)
			err = sql.CompleteTask(task.ID, false)
			if err != nil {
				log.Printf("Failed to mark errored Task %d as as complete: %s", task.ID, err)
			}
			return
		}

		if !reflect.DeepEqual(*lastResponse, *batchTaskInfo) {
			*lastResponse = *batchTaskInfo

			progress := batchTaskInfo.GetBatchTaskProgress()

			log.Printf("[Task %d] %s %d - %s", task.ID, task.BatchTaskName, task.BatchTaskID, progress)

			if progress.Remaining() == 0 {
				err := sql.CompleteTask(task.ID, progress.Failed == 0)
				if err != nil {
					log.Printf("Unable to mark Task %d as complete, will attempt on next invocation", task.ID)
				}
				break
			}
		}
	}
}

func resumeIncompleteTasks(sql *database.SQLModels, vpcConfAPI *vpcconfapi.VPCConfAPI, wg *sync.WaitGroup) bool {
	incompleteTasks, err := sql.GetIncompleteTasks()
	if err != nil {
		log.Printf("Unable to fetch incomplete tasks: %s", err)
	}

	if len(incompleteTasks) == 0 {
		return false
	}

	log.Printf("Found %d incomplete tasks", len(incompleteTasks))

	for _, task := range incompleteTasks {
		wg.Add(1)
		go observeProgress(sql, task, vpcConfAPI, wg)
	}

	return true
}

func verifyStateTask(vpcs []vpcconfapi.VPC, sql *database.SQLModels, vpcConfAPI *vpcconfapi.VPCConfAPI, wg *sync.WaitGroup) error {
	batchTaskResult, err := vpcConfAPI.SubmitVerifyBatchTask(vpcs, vpcconf.VerifyAllSpec())
	if err != nil {
		return err
	}

	batchTaskInfo, err := vpcConfAPI.GetBatchTaskByID(batchTaskResult.BatchTaskID)
	if err != nil {
		return err
	}

	taskID, err := sql.CreateTask(batchTaskInfo.ID, batchTaskInfo.Description)
	if err != nil {
		return err
	}

	task, err := sql.GetTaskByID(*taskID)
	if err != nil {
		return err
	}

	if task.ID != *taskID {
		return fmt.Errorf("Failed to fetch Task %d", taskID)
	}

	wg.Add(1)
	go observeProgress(sql, task, vpcConfAPI, wg)

	return nil
}

func contains(needle string, haystack []string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}

func filterVPCs(vpcs []vpcconfapi.VPC, regions []string, stacks []string) []vpcconfapi.VPC {
	filteredVPCs := []vpcconfapi.VPC{}

	for _, vpc := range vpcs {
		regionOK := contains(string(vpc.Region), regions)
		stackOK := contains(vpc.Stack, stacks)
		if regionOK && stackOK {
			filteredVPCs = append(filteredVPCs, vpc)
		}
	}

	return filteredVPCs
}

func validate(input []string, valid []string) error {
	for _, in := range input {
		if !contains(in, valid) {
			return fmt.Errorf("%q is not one of these valid options: %s", in, strings.Join(valid, ", "))
		}
	}
	return nil
}

func main() {
	flag.Usage = func() {
		log.Println("USAGE: sidekick [-regions=] [-stacks=]")
		log.Println("EXAMPLE: sidekick -regions=us-east-1,use-west-2 -stacks=sandbox,dev,test")
	}

	flagStacks := flag.String("stacks", "", "override target stacks - comma separated")
	flagRegions := flag.String("regions", "", "overide target regions - comma separated")

	flag.Parse()

	conf := &conf.Conf{}
	result := conf.Load()
	if !result {
		conf.LogProblems()
		os.Exit(exitBadConfiguration)
	}

	db := sqlx.MustConnect("postgres", conf.DBConnect)
	err := database.Migrate(db)
	if err != nil {
		log.Panicf("Failed to migrate database: %s", err)
	}

	sql := &database.SQLModels{DB: db}

	vpcConfAPI := &vpcconfapi.VPCConfAPI{
		Username: conf.Username,
		Password: conf.Password,
		BaseURL:  conf.VPCConfBaseURL,
	}

	err = vpcConfAPI.VerifySession()
	if err != nil {
		log.Printf("Failed to authenticate to VPC Conf: %s", err)
		os.Exit(exitVPCConfAPIError)
	}

	var wg sync.WaitGroup

	if resumeIncompleteTasks(sql, vpcConfAPI, &wg) {
		wg.Wait()
		os.Exit(0)
	}

	automatedVPCs, automatedRegions, err := vpcConfAPI.GetAutomatedVPCsAndRegions()
	if err != nil {
		log.Println(err)
		os.Exit(exitVPCConfAPIError)
	}
	if len(automatedVPCs) == 0 {
		log.Printf("VPC Conf returned 0 VPCs")
		os.Exit(exitVPCConfAPIError)
	}

	validRegions := []string{"us-east-1", "us-west-2", "us-gov-west-1"}
	validStacks := []string{"sandbox", "dev", "test", "impl", "prod"}
	var regions = automatedRegions
	var stacks = validStacks

	if *flagStacks != "" {
		stacks = strings.Split(*flagStacks, ",")
		err = validate(stacks, validStacks)
		if err != nil {
			log.Fatal(err)
		}
	}
	if *flagRegions != "" {
		regions = strings.Split(*flagRegions, ",")
		err = validate(regions, validRegions)
		if err != nil {
			log.Fatal(err)
		}
	}

	filteredVPCs := filterVPCs(automatedVPCs, regions, stacks)

	if len(filteredVPCs) == 0 {
		log.Println("No VPCs remain after filtering for region and stack:")
		log.Printf("\tRegions: %s", strings.Join(regions, ", "))
		log.Printf("\tStacks: %s", strings.Join(stacks, ", "))
		os.Exit(0)
	}

	err = verifyStateTask(filteredVPCs, sql, vpcConfAPI, &wg)
	if err != nil {
		log.Println(err)
	}

	wg.Wait()
}
