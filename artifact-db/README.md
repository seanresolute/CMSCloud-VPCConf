### Build

`go build`


### Binary S3 Download

If you don't want to bother with setting up go. The latest version of the
ArtifactDB binary is available on S3.

`aws s3 cp s3://artifact-db/artifact-db .`

### ArtifactDB DynamoDB Schema and setup

ArtifactDB consists of a single DynamoDB table.

DynamoDB supports two types of primary keys, either a single unique attribute
as the partition key, or a composite key consisting of a partition key
attribute + a range key attribute. When using a composite key, hash+range
attributes must produce a unique primary key.

The ArtifactDB table must be created via the AWS WebUi with a partition key of
Project (string) and a range key of BuildNumber (number).

ArtifactDBItem {
    Project: String (partition key)
    BuildNumber: Number (range key)
    Tags: map[string] string (optional for now)
    Value: String (optional for now)
}


### Usage

See `./artifact-db --help`


### High Level Purpose

The intention of ArtifactDB is to organize and attach additional information
to artifacts as well as querying for this information with various criteria.

It is intended to answer questions like "What is the newest binary that is
approved to run in production?" "What are the versions of 3 data sets contained
within this single archive?" "What is the S3 download link for the latest
approved dataset of the service I am installing?"


### Example: GeoAPI AMI

## Standard Lifecycle

The GeoAPI AMI is deployed in 3 places, the test, impl and prod environment.
There is a process to move a new AMI from test to, impl, to prod. It involves
testing, leaving enough time to collect metrics and ensure that problems don't
emerge hours after deployment, and receiving approval from CMS. As these
validations are completed, the AMI is cleared to progress until it reaches the
prod environment.

To represent the GeoAMI we create an artifact with the following structure.

Artifact {
    Project: geo-rh6-east
    BuildNumber: N
    Value: AMI-ID
    Tags: {
              test: string
              impl: string
              prod: string
          }
}

ArtifactDB does not force any tagging constraints, so it is up to the user to
create a tagging structure that represents the lifecycle of the artifact.

In this case, we are creating a tag for each environment and we are defining a
default value of "false" and an approved state of "true".

As there is no criteria to be deployed to test, on initial creation of a GeoAMI
artifact we can set the test tag to "true".

Artifact {
    Project: geo-rh6-east
    BuildNumber: 11
    Value: ami-asd32v5a
    Tags: {
              test: "true"
              impl: "false"
              prod: "false"
          }
}

When executing a deploy to test, we will call:

`./artifact-db latest --project geo-rh6-east --tags '{"test": "true"}'`

This will find the above artifact and return 'ami-asd32v5a'.

Once the developer is ready to move forward with this change after
experimenting in test, they will update the impl tag to "true". When executing
a deploy to impl, we will call:

`./artifact-db latest --project geo-rh6-east --tags '{"impl": "true"}'`

This will find the above artifact and return 'ami-asd32v5a'.

Before a GeoAMI is ready to be deployed to prod, it must spend at least 24
hours in impl, it needs to have smoke testing conducted to attempt to catch any
regressions, and CMS must give their approval.

When this criteria is met, the tag prod is set to "true". On executing a deploy
to prod, we will call:

`./artifact-db latest --project geo-rh6-east --tags '{"prod": "true"}'`

This will find the above artifact and return 'ami-asd32v5a'.


## Abort after test

If we imagine an alternate case where the developer is unhappy with the results
of build 11 at the test phase, they can update the test flag to "false" and
execute a deploy to test.

## TODO: add example of updating a tag when implemented

The engineer may also optionally create a "notes" tag to provide a summary
as to why this image is no longer being considered for the impl environment and
is being removed from the test environment. With the test flag updated,
calling:

`./artifact-db latest --project geo-rh6-east --tags '{"test": "true"}'`

Will NOT return the above artifact, instead artifact DB will return the next
highest BuildNumber that also has the test tag set to "true"

Before ArtifactDB, this process was manual and an engineer would have to
look through a git commit history to figure out a good rollback candidate.
