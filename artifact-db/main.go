package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var globalOptions struct {
	region string
	table  string
}

func NewCmdRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact-db",
		Short: "Interact with ArtifactDB",
	}

	cmd.PersistentFlags().StringVar(&globalOptions.region, "region", "us-east-1", "region")
	cmd.PersistentFlags().StringVar(&globalOptions.table, "table", "ArtifactDB", "region")
	cmd.AddCommand(NewCmdGet())
	cmd.AddCommand(NewCmdLatest())
	cmd.AddCommand(NewCmdCreate())
	cmd.AddCommand(NewCmdTag())
	return cmd
}

func main() {
	if err := NewCmdRoot().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
