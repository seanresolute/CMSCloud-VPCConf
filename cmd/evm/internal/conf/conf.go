package conf

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

// Conf is populated via env variables
type Conf struct {
	CloudTamerAdminGroupID  int
	CloudTamerBaseURL       string
	CloudTamerIDMSID        int
	DryRun                  bool
	Password                string
	Username                string
	problems                []string
	TransitGatewayAccountID string
	Stacks                  []string
	VPCConfBaseURL          string
	VPCConfTemplateName     string
	TGWResourceShareName    string
}

// AllowedStack checks if the given stack is in Stacks
func (conf *Conf) AllowedStack(stack string) bool {
	for _, allowed := range conf.Stacks {
		if stack == allowed {
			return true
		}
	}
	return false
}

// Load variables from env and confirm file paths
func (conf *Conf) Load() bool {
	var err error
	conf.problems = []string{}

	cloudTamerAdminGroupID := os.Getenv("CLOUDTAMER_ADMIN_GROUP_ID")
	if cloudTamerAdminGroupID == "" {
		conf.problems = append(conf.problems, "CLOUDTAMER_ADMIN_GROUP_ID is not defined or empty")
	} else {
		conf.CloudTamerAdminGroupID, err = strconv.Atoi(cloudTamerAdminGroupID)
		if err != nil {
			conf.problems = append(conf.problems, fmt.Sprintf("CLOUDTAMER_ADMIN_GROUP_ID: %s", err))
		}
	}

	conf.CloudTamerBaseURL = os.Getenv("CLOUDTAMER_BASE_URL")
	if conf.CloudTamerBaseURL == "" {
		conf.problems = append(conf.problems, "CLOUDTAMER_BASE_URL is not defined or empty")
	}

	cloudTamerIDMSID := os.Getenv("CLOUDTAMER_IDMS_ID")
	if cloudTamerIDMSID == "" {
		conf.problems = append(conf.problems, "CLOUDTAMER_IDMS_ID is not defined or empty")
	} else {
		conf.CloudTamerIDMSID, err = strconv.Atoi(cloudTamerIDMSID)
		if err != nil {
			conf.problems = append(conf.problems, fmt.Sprintf("CLOUDTAMER_IDMS_ID: %s", err))
		}
	}

	conf.Username = os.Getenv("EUA_USERNAME")
	if conf.Username == "" {
		conf.problems = append(conf.problems, "EUA_USERNAME is not defined or empty")
	}

	conf.Password = os.Getenv("EUA_PASSWORD")
	if conf.Password == "" {
		conf.problems = append(conf.problems, "EUA_PASSWORD is not defined or empty")
	}

	// dry run is optional and defaults to false
	dryRun := os.Getenv("EVM_DRY_RUN")
	if dryRun != "" {
		conf.DryRun, err = strconv.ParseBool(dryRun)
		if err != nil {
			conf.problems = append(conf.problems, fmt.Sprintf("EVM_DRY_RUN: %s", err))
		}
	}

	stacks := os.Getenv("STACKS")
	if stacks == "" {
		conf.problems = append(conf.problems, "STACKS is not defined or empty")
	} else {
		conf.Stacks = strings.Split(stacks, ",")
	}

	conf.TransitGatewayAccountID = os.Getenv("TRANSIT_GATEWAY_ACCOUNT_ID")
	if conf.TransitGatewayAccountID == "" {
		conf.problems = append(conf.problems, "TRANSIT_GATEWAY_ACCOUNT_ID is not defined or empty")
	}

	conf.VPCConfBaseURL = os.Getenv("VPC_CONF_BASE_URL")
	if conf.VPCConfBaseURL == "" {
		conf.problems = append(conf.problems, "VPC_CONF_BASE_URL is not defined or empty")
	}

	conf.VPCConfTemplateName = os.Getenv("VPC_CONF_TEMPLATE_NAME")
	if conf.VPCConfTemplateName == "" {
		conf.problems = append(conf.problems, "VPC_CONF_TEMPLATE_NAME is not defined or empty")
	}

	conf.TGWResourceShareName = os.Getenv("TGW_RESOURCE_SHARE_NAME")
	if conf.TGWResourceShareName == "" {
		conf.problems = append(conf.problems, "TGW_RESOURCE_SHARE_NAME is not defined or empty")
	}

	return len(conf.problems) == 0
}

// GetProblems if there is a need to manually parse them
func (conf *Conf) GetProblems() []string {
	return conf.problems
}

// LogProblems to log
func (conf *Conf) LogProblems() {
	if len(conf.problems) > 0 {
		log.Println("There are problems with the environment variables:")
		for _, problem := range conf.problems {
			log.Print(problem)
		}
	}
}
