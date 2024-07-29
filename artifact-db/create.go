package main

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/artifact"
	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/dynamo"
	"github.com/spf13/cobra"
)

func NewCmdCreate() *cobra.Command {
	var project, value, tagsString string
	var containedArtifacts []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new artifact",
		Long:  "Creates a new artifact with a buildNumber that is set to the max buildNumber + 1",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return errors.New("--project [string] is a required flag")
			}
			if value == "" {
				return errors.New("--value [string] is a required flag")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			var tags map[string]string
			if tagsString != "" {
				err := json.Unmarshal([]byte(tagsString), &tags)
				if err != nil {
					log.Panicf("Failed to unmarshal tags:\n%v\n", err)
				}
			}

			for _, artifactID := range containedArtifacts {
				project, build, err := artifact.ParseArtifactID(artifactID)
				if err != nil {
					log.Panicf("Error parsing contained artifact ID %q: %s", artifactID, err)
				}

				_, err = dynamo.GetArtifact(globalOptions.region, globalOptions.table, project, build)
				if err != nil {
					log.Panic(err)
				}
			}

			var nextBuildNumber int64 = 1
			maxArtifact, err := dynamo.LatestArtifact(globalOptions.region, globalOptions.table, project, &map[string]string{})
			if err != nil {
				log.Panicf("Error getting artifact with max BuildNumber: %v", err)
			}
			if maxArtifact != nil {
				nextBuildNumber = maxArtifact.BuildNumber + 1
			}

			artifact := &artifact.Artifact{
				ProjectName:        project,
				BuildNumber:        nextBuildNumber,
				ContainedArtifacts: containedArtifacts,
				Value:              value,
				Tags:               tags,
			}

			err = dynamo.CreateArtifact(globalOptions.region, globalOptions.table, artifact)
			if err != nil {
				log.Panicf("Failed to put item, does the item already exist?:\n%v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", "", "ArtifactDB project name")
	cmd.Flags().Int64VarP(&build, "build", "b", -1, "Artifact build number (optional; ignored if provided)")
	cmd.Flags().StringVarP(&value, "value", "v", "", "Artifact value")
	cmd.Flags().StringArrayVarP(&containedArtifacts, "contained-artifact", "c", []string{}, "Artifact IDs contained within this artifact")
	cmd.Flags().StringVarP(&tagsString, "tags", "t", "", "JSON string of key value pairs")

	return cmd
}
