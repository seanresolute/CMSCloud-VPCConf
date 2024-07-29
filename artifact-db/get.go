package main

import (
	"errors"
	"log"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/dynamo"
	"github.com/spf13/cobra"
)

func NewCmdGet() *cobra.Command {
	var project string
	var buildNumber int64
	var printJSON bool

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Gets a specific artifact",
		Long: `
Gets the artifact with the specified build number.
`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return errors.New("--project [string] is a required flag")
			}
			if buildNumber == 0 {
				return errors.New("--build [int64] is a required flag")
			}
			return nil
		},

		Run: func(cmd *cobra.Command, args []string) {
			artifact, err := dynamo.GetArtifact(globalOptions.region, globalOptions.table, project, buildNumber)
			if err != nil {
				log.Panicf("Failed to get all artifacts from DB:\n%v\n", err)
			}

			if artifact != nil {
				err := printArtifact(artifact, printJSON)
				if err != nil {
					log.Fatalf("Error printing: %s", err)
				}
			}
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", "", "ArtifactDB project name")
	cmd.Flags().Int64VarP(&buildNumber, "build", "b", 0, "Build number")
	cmd.Flags().BoolVarP(&printJSON, "json", "j", false, "Print all artifact atributes as JSON")

	return cmd
}
