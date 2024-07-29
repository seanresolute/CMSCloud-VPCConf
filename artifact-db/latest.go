package main

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/dynamo"
	"github.com/spf13/cobra"
)

func NewCmdLatest() *cobra.Command {
	var project, tagsToCheckString string
	var printJSON bool

	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Finds latest artifact value",
		Long: `
Finds the latest artifact, ordered by build number. Any artifacts missing an optional
key value pair, will be omitted from the results.
`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return errors.New("--project [string] is a required flag")
			}
			return nil
		},

		Run: func(cmd *cobra.Command, args []string) {
			var tagsToCheck map[string]string
			err := json.Unmarshal([]byte(tagsToCheckString), &tagsToCheck)
			if tagsToCheckString != "" {
				if err != nil {
					log.Panicf("Failed to unmarshal tags:\n%v\n", err)
				}
			}

			artifact, err := dynamo.LatestArtifact(globalOptions.region, globalOptions.table, project, &tagsToCheck)
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
	cmd.Flags().StringVarP(&tagsToCheckString, "tags", "t", "", "Tags that must be set in artifact to be considered latest as JSON string of key value pairs")
	cmd.Flags().BoolVarP(&printJSON, "json", "j", false, "Print all artifact atributes as JSON")

	return cmd
}
