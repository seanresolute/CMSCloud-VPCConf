package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/dynamo"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/spf13/cobra"
)

var (
	key, value, project string
	build               int64
)

func NewCmdTag() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manipulate tags on an artifact",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return errors.New("--project [string] is a required flag")
			}
			if build < 0 {
				return errors.New("--build [nonnegative int] is a required flag")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&project, "project", "p", "", "ArtifactDB project name")
	cmd.PersistentFlags().Int64VarP(&build, "build", "b", -1, "Artifact build number")

	cmd.AddCommand(NewCmdTagSet())
	cmd.AddCommand(NewCmdTagRm())

	return cmd
}

func NewCmdTagSet() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set tag on an artifact",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if value == "" {
				return errors.New("--value [string] is a required flag")
			}
			if key == "" {
				return errors.New("--key [string] is a required flag")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := dynamo.SetTag(globalOptions.region, globalOptions.table, project, key, value, build)
			if err != nil {
				err := err.(awserr.Error)
				if err.Code() == "ValidationException" {
					fmt.Println("Does the artifact Exist?: ", err.Message())
					os.Exit(1)
				} else {
					log.Panicf("Failed to set tag:\n%v\n", err)
				}
			}
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Key to set")
	cmd.Flags().StringVarP(&value, "value", "v", "", "Value to set")

	return cmd
}

func NewCmdTagRm() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove tag from an artifact",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if key == "" {
				return errors.New("--key [string] is a required flag")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := dynamo.RemoveTag(globalOptions.region, globalOptions.table, project, key, build)
			if err != nil {
				log.Panicf("Failed to remove tag:\n%v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Key to set")

	return cmd
}
