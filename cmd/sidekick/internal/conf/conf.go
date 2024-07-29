package conf

import (
	"log"
	"os"
)

type Conf struct {
	Password       string
	Username       string
	DBConnect      string
	VPCConfBaseURL string
	problems       []string
}

func (conf *Conf) Load() bool {
	conf.problems = []string{}

	conf.Username = os.Getenv("EUA_USERNAME")
	if conf.Username == "" {
		conf.problems = append(conf.problems, "EUA_USERNAME is not defined or empty")
	}

	conf.Password = os.Getenv("EUA_PASSWORD")
	if conf.Password == "" {
		conf.problems = append(conf.problems, "EUA_PASSWORD is not defined or empty")
	}

	conf.VPCConfBaseURL = os.Getenv("VPC_CONF_BASE_URL")
	if conf.VPCConfBaseURL == "" {
		conf.problems = append(conf.problems, "VPC_CONF_BASE_URL is not defined or empty")
	}

	conf.DBConnect = os.Getenv("SIDEKICK_POSTGRES_CONNECTION_STRING")
	if conf.DBConnect == "" {
		conf.problems = append(conf.problems, "SIDEKICK_POSTGRES_CONNECTION_STRING is not defined or empty")
	}
	return len(conf.problems) == 0
}

func (conf *Conf) GetProblems() []string {
	return conf.problems
}

func (conf *Conf) LogProblems() {
	if len(conf.problems) > 0 {
		log.Println("There are problems with the environment variables:")
		for _, problem := range conf.problems {
			log.Print(problem)
		}
	}
}
