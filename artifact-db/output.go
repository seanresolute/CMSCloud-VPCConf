package main

import (
	"encoding/json"
	"fmt"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/artifact"
	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/dynamo"
)

func printArtifact(artifact *artifact.Artifact, full bool) error {
	if full {
		nestedArtifacts, err := dynamo.BuildNestedArtifact(globalOptions.region, globalOptions.table, artifact)
		if err != nil {
			return fmt.Errorf("Failed build Nested Artifacts struct from:\n%v\n", err)
		}

		nestedArtifactsJson, err := json.MarshalIndent(nestedArtifacts, "", "    ")
		if err != nil {
			return fmt.Errorf("Failed to get artifact in JSON from:\n%v\n", err)
		}

		fmt.Println(string(nestedArtifactsJson))
	} else {
		fmt.Println(artifact.Value)
	}
	return nil
}
