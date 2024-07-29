package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/vpcconfapi"
)

type VPCConfAPIKeys struct {
	Dev  string `json:"dev"`
	Prod string `json:"prod"`
}

func main() {
	env := flag.String("env", "", "environment to affect: dev|prod")
	allowOnly := flag.String("allow-only", "", "worker name to allow")
	allowAll := flag.Bool("allow-all", false, "allow all workers")
	noWait := flag.Bool("no-wait", false, "do not wait for the queue to empty after stopping it")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -env <env> [-allow-only <name>] [-allow-all] start|stop\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if len(flag.Args()) != 1 || (flag.Args()[0] != "start" && flag.Args()[0] != "stop") {
		flag.Usage()
		os.Exit(2)
	}

	baseURL := map[string]string{
		"dev":  "https://dev.vpc-conf.actually-east.west.cms.gov/provision",
		"prod": "https://vpc-conf.actually-east.west.cms.gov/provision",
	}[*env]
	if baseURL == "" {
		fmt.Fprintf(os.Stderr, "Invalid environment %q\n", *env)
		os.Exit(2)
	}

	isStop := flag.Args()[0] == "stop"

	if !isStop && *allowAll == (*allowOnly != "") {
		fmt.Fprintf(os.Stderr, "You must specify exactly one of -allow-all and -allow-only\n")
		os.Exit(2)
	}

	var err error

	keysString := os.Getenv("VPC_CONF_API_KEYS")
	if len(keysString) == 0 {
		fmt.Fprintf(os.Stderr, "VPC_CONF_API_KEYS is not defined\n")
		os.Exit(2)
	}

	keys := &VPCConfAPIKeys{}
	err = json.Unmarshal([]byte(keysString), keys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to unmarshal VPC_CONF_API_KEYS: %s", err)
		os.Exit(2)
	}

	var key string
	if *env == "dev" { // baseURL above does valid environment check
		key = keys.Dev
	} else {
		key = keys.Prod
	}
	if key == "" {
		fmt.Fprintf(os.Stderr, "VPC_CONF_API_KEYS entry for %s is empty\n", *env)
		os.Exit(2)
	}

	vpcConfAPI := &vpcconfapi.VPCConfAPI{
		APIKey:  key,
		BaseURL: baseURL,
	}

	if isStop {
		err = vpcConfAPI.AllowNoWorkers()
		if !*noWait {
			log.Printf("Waiting for task queue to empty...")
			for {
				stats, err := vpcConfAPI.GetTaskStats()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error getting task stats while waiting for task queue to empty: %s\n", err)
					os.Exit(1)
				}
				if stats.NumTasksReserved > 0 {
					log.Printf("Still %d task(s) reserved", stats.NumTasksReserved)
					time.Sleep(5 * time.Second)
				} else {
					break
				}
			}
		}
	} else if *allowAll {
		err = vpcConfAPI.AllowAllWorkers()
	} else {
		err = vpcConfAPI.AllowOnlyWorkers(*allowOnly)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating allowed workers: %s\n", err)
		os.Exit(1)
	}
}
