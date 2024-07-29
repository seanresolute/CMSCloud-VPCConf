package dynamo

import (
	"fmt"
	"strconv"

	"github.com/CMS-AWS-West-Network-Architecture/vpc-automation/artifact-db/artifact"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

func dynamo(region string) *dynamodb.DynamoDB {
	// Create the config specifying the Region for the DynamoDB table.
	// If Config.Region is not set the region must come from the shared
	// config or AWS_REGION environment variable.
	awscfg := &aws.Config{}
	if len(region) > 0 {
		awscfg.WithRegion(region)
	}

	// Create the session that the DynamoDB service will use.
	sess := session.Must(session.NewSession(awscfg))

	// Create the DynamoDB service client to make the query request with.
	svc := dynamodb.New(sess)

	return svc
}

func CreateArtifact(region, table string, artifact *artifact.Artifact) error {
	svc := dynamo(region)

	artifactMarshaled, err := dynamodbattribute.MarshalMap(artifact)
	if err != nil {
		return err
	}

	input := &dynamodb.PutItemInput{
		Item:      artifactMarshaled,
		TableName: &table,
	}

	// Put item if something with this primary key doesn't exist
	// see attribute_not_exists Notes on:
	// http://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_PutItem.html
	cond := "attribute_not_exists(ProjectName)"
	input.ConditionExpression = &cond
	_, err = svc.PutItem(input)
	return err
}

func RemoveTag(region, table, project, key string, build int64) error {
	svc := dynamo(region)
	buildStr := strconv.FormatInt(build, 10)

	input := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#key": aws.String(key),
		},
		Key: map[string]*dynamodb.AttributeValue{
			"ProjectName": {
				S: aws.String(project),
			},
			"BuildNumber": {
				N: aws.String(buildStr),
			},
		},
		TableName:        aws.String(table),
		UpdateExpression: aws.String("REMOVE Tags.#key"),
	}

	_, err := svc.UpdateItem(input)
	return err
}

func SetTag(region, table, project, key, value string, build int64) error {
	svc := dynamo(region)
	buildStr := strconv.FormatInt(build, 10)

	input := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: map[string]*string{
			"#key": aws.String(key),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":value": {
				S: aws.String(value),
			},
		},
		Key: map[string]*dynamodb.AttributeValue{
			"ProjectName": {
				S: aws.String(project),
			},
			"BuildNumber": {
				N: aws.String(buildStr),
			},
		},
		TableName:        aws.String(table),
		UpdateExpression: aws.String("SET Tags.#key = :value"),
	}

	_, err := svc.UpdateItem(input)
	return err
}

func GetArtifact(region, table, project string, buildNumber int64) (*artifact.Artifact, error) {
	svc := dynamo(region)

	result, err := svc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key: map[string]*dynamodb.AttributeValue{
			"ProjectName": {
				S: aws.String(project),
			},
			"BuildNumber": {
				N: aws.String(fmt.Sprintf("%d", buildNumber)),
			},
		},
	})
	artifact := artifact.Artifact{}
	if err != nil {
		return nil, err
	}

	if result.Item == nil {
		return nil, fmt.Errorf("Item %s:%d not found", project, buildNumber)
	}

	err = dynamodbattribute.UnmarshalMap(result.Item, &artifact)
	if err != nil {
		return nil, err
	}

	return &artifact, err
}

// Returns all artifacts that are of the given project sorted build number (highest first)
func GetAllProjectArtifacts(region, table, project string) ([]*artifact.Artifact, error) {
	svc := dynamo(region)
	artifacts := []*artifact.Artifact{}

	startKey := map[string]*dynamodb.AttributeValue{}
	pages := []map[string]*dynamodb.AttributeValue{}

	// Keep getting maximum page size (1mb) until we have no remaining keys
	for {
		var queryInput = &dynamodb.QueryInput{
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":project": {
					S: aws.String(project),
				},
			},
			TableName:              aws.String(table),
			KeyConditionExpression: aws.String("ProjectName = :project"),
			// Sort artifacts by highest build number first
			ScanIndexForward: aws.Bool(false),
		}

		// If this isn't the first run and there are still keys to process
		// set startkey previous loop
		if len(startKey) != 0 {
			queryInput.SetExclusiveStartKey(startKey)
		}

		var result, err = svc.Query(queryInput)
		if err != nil {
			return artifacts, err
		}

		// Add results to previous pages
		pages = append(pages, result.Items...)

		// If there is a LastEvaluatedKey set it as the start key for next iteration
		// else we got all the items and we can break
		if len(result.LastEvaluatedKey) != 0 {
			startKey = result.LastEvaluatedKey
		} else {
			break
		}
	}

	// Combine all pages and unmarshal
	err := dynamodbattribute.UnmarshalListOfMaps(pages, &artifacts)
	if err != nil {
		return artifacts, err
	}
	return artifacts, nil
}

// Finds the latest artifact that contains all tags in tagsToCheck
func LatestArtifact(region, table, project string, tagsToCheck *map[string]string) (*artifact.Artifact, error) {
	artifacts, err := GetAllProjectArtifacts(region, table, project)
	if err != nil {
		return nil, err
	}

	// Return first artifact that mataches our criteria
	for _, artifact := range artifacts {
		if hasTags(artifact, tagsToCheck) {
			return artifact, nil
		}
	}
	return nil, nil
}

// Check that potential match has all key value pairs we are looking for
func hasTags(artifact *artifact.Artifact, tagsToCheck *map[string]string) bool {
	for key, value := range *tagsToCheck {
		if value != artifact.Tags[key] {
			return false
		}
	}
	return true
}

// In the backing DB, only ArtifactID strings are stored to represent contained artifacts
// This method builds a complete nested Artifacts structure by recursively looking up
// the ArtifactID strings and merging the results into one nested structure.
func BuildNestedArtifact(region, table string, art *artifact.Artifact) (*artifact.NestedArtifact, error) {
	nestedArtifact := artifact.NestedArtifact{Artifact: *art}
	// For each ArtifactID string get the artifact and convert it to the nested type
	for _, containedArtifactID := range art.ContainedArtifacts {
		project, build, err := artifact.ParseArtifactID(containedArtifactID)
		if err != nil {
			return nil, fmt.Errorf("Error parsing nested artifact ID: %s", err)
		}
		containedArtifact, err := GetArtifact(region, table, project, build)
		if err != nil {
			return nil, err
		}

		// Transform regular Artifact to Nested Form
		containedNestedArtifact, err := BuildNestedArtifact(region, table, containedArtifact)
		if err != nil {
			return nil, err
		}

		nestedArtifact.ContainedArtifacts = append(nestedArtifact.ContainedArtifacts, containedNestedArtifact)
	}
	return &nestedArtifact, nil
}
