package artifact

import (
	"fmt"
	"strconv"
	"strings"
)

type Tag struct {
	Key   string
	Value string
}

type Artifact struct {
	ProjectName        string
	BuildNumber        int64
	Value              string
	ContainedArtifacts []string
	Tags               map[string]string
}

func ParseArtifactID(artifactID string) (project string, build int64, err error) {
	// Parse ArtifactID string in the form of project:buildnumber
	pieces := strings.Split(artifactID, ":")
	if len(pieces) != 2 {
		return "", -1, fmt.Errorf("Invalid artifact %q", artifactID)
	}
	project = pieces[0]
	build, err = strconv.ParseInt(pieces[1], 10, 64)
	if err != nil {
		return "", -1, fmt.Errorf("Invalid build number %q", pieces[1])
	}
	return
}

type NestedArtifact struct {
	Artifact
	ContainedArtifacts []*NestedArtifact
}

func (a Artifact) String() string {
	return fmt.Sprintf("Project: %s, BuildNumber: %d, Value: %s", a.ProjectName, a.BuildNumber, a.Value)
}

// Sort By build number, highest to lowest
type ByBuildNumber []*Artifact

func (a ByBuildNumber) Len() int           { return len(a) }
func (a ByBuildNumber) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByBuildNumber) Less(i, j int) bool { return a[i].BuildNumber > a[j].BuildNumber }
