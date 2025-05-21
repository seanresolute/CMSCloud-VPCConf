# VPC Conf

## New build and deploy simplified.

```
GOOS=windows GOARCH=amd64 go build -o vpc-conf.exe ./cmd/vpc-conf/
```

Use NSSM on Windows: https://nssm.cc/download

```
nssm install vpc-conf
nssm start vpc-conf
```

Paths below are relative to the `vpc-automation` directory.

## Setup

### Set Environment Variables

Set AWS credentials for the VPC Conf dev account ("[VPC Automation Dev](https://vpc-conf.actually-east.west.cms.gov/provision/accounts/346570397073)" in VPC Conf), then:


```bash
source env-dev
```

To make this work faster and eliminate the need for AWS credentials, replace the `ssm_param` calls with literals. Retrieve the values from Parameter Store in AWS Systems Manager or ask a teammate for them.

Be sure that the `AZURE_AD_REDIRECT_URL` uses *localhost* and not *127.0.0.1* or development authentication will not work. Access VPC Conf via *localhost* or authentication will not work. The redirect URL must match a configured option in Azure AD.

### Run Postgres

```
docker run -d -e POSTGRES_PASSWORD=password -p 5432:5432 postgres
```

## Running

With environment variable `IPAM_DEV_MODE=1`, running programs from `cmd/<program>` enables templates to be reloaded from disk on each request.

### VPC Conf

```
cd cmd/vpc-conf
go run .
```


## Using Docker

### Setup

Translate the `env-dev` script to a [Docker env file](https://docs.docker.com/compose/env-file/) (`~/vpc-conf-dev.env`) or ask a teammate for a copy of theirs. `IPAM_DEV_MODE` must not be set when using Docker because standalone templates are not included in the Docker images.

Then:

```
docker network create vpc-conf-dev
docker run --rm --name postgres --network vpc-conf-dev -e POSTGRES_PASSWORD=password -d postgres
```

### VPC Conf

```
docker build . -f vpc-conf.Dockerfile -t vpc-conf:dev

# Postgres host matches name of Postgres Docker container
docker run --rm --env-file ~/vpc-conf-dev.env -e 'POSTGRES_CONNECTION_STRING=postgresql://postgres:password@postgres:5432/postgres?sslmode=disable' --network vpc-conf-dev -p 2020:2020 vpc-conf:dev
```

To get a list of account for your local VPC Conf to opperate on, the update-aws-account micro service needs to be built and run to populate the database.
Once every minute this service gets a list of accounts via the cloudtamer API and populates the VPC Conf database with them.

```
docker build . -f update-aws-accounts.Dockerfile -t update-aws-accounts
docker run --rm --env-file ~/vpc-conf-dev.env -e 'POSTGRES_CONNECTION_STRING=postgresql://postgres:password@postgres:5432/postgres?sslmode=disable' --network vpc-conf-dev update-aws-accounts
```

VPC Conf needs to have been started once to setup the initial postgres tables, but after that initial execution update-aws-accounts can be run at any time as long as the database container is running.

## Templates and Static Files

These are embedded into the binary using [esc](https://github.com/mjibson/esc) in order to make the application a single binary with no dependencies. Anything in the `cmd/vpc-conf/esc` directory will be embedded. Files in the further subdirectory `static` will be served by the web server at `/static`. Templates are defined in `templates.go`.

To regenerate embedded files:

```
go get github.com/mjibson/esc
go generate ./cmd/vpc-conf
```
export PATH=$PATH:/Users/[]/go/bin

To reload on-disk versions of the embedded files with each request instead of using the embedded versions (so that you can see updates without restarting the server) set env variable `IPAM_DEV_MODE=1`. In that case you must run the server from inside the `cmd/vpc-conf` directory.


### Update libraries

```
docker run --name polymer-project -it --entrypoint npm node:12.13.1-alpine install lit-element
DIR=$(mktemp -d /tmp/polymer-project.XXXXXX)
docker cp polymer-project:/node_modules/lit-element $DIR
docker cp polymer-project:/node_modules/lit-html $DIR
docker rm polymer-project
cp $DIR/lit-element/lit-element.js cmd/vpc-conf/esc/static/lit-element/lit-element.js
rm cmd/vpc-conf/esc/static/lit-element/lib/*
cp $DIR/lit-element/lib/*.js cmd/vpc-conf/esc/static/lit-element/lib/
cp $DIR/lit-html/lit-html.js cmd/vpc-conf/esc/static/lit-html/lit-html.js
rm cmd/vpc-conf/esc/static/lit-html/lib/*
cp $DIR/lit-html/lib/*.js cmd/vpc-conf/esc/static/lit-html/lib/
rm cmd/vpc-conf/esc/static/lit-html/directives/*
cp $DIR/lit-html/directives/repeat.js cmd/vpc-conf/esc/static/lit-html/directives/
rm -r $DIR
```

### Fix relative paths

lit-element.js has [unsupported paths](https://v8.dev/features/modules#specifiers) by default to reference lit-html and one of its libraries. This is due to modern web application reliance on bundler applications like webpack and parcel.

To correct this in `vpc-automation/cmd/vpc-conf/esc/static/lit-element/lit-element.js` update:

```
import { render } from 'lit-html/lib/shady-render.js';
export { html, svg, TemplateResult, SVGTemplateResult } from 'lit-html/lit-html.js';
```

to

```
import { render } from '../lit-html/lib/shady-render.js';
export { html, svg, TemplateResult, SVGTemplateResult } from '../lit-html/lit-html.js';
```

### Generate templates

```
go generate ./cmd/vpc-conf
```

## Pushing Docker Images

### Automatic

Images for VPCConf, Credentials API, and Update AWS Account are automatically built and pushed by [Jenkins](https://jenkins-east.cloud.cms.gov/itops-ia/job/CMS-AWS-West-Network-Architecture/job/Build%20and%20push%20images/) and/or [JenkinsGovCloud](https://jenkins-gc-west.cloud.cms.gov/itops-ia/job/CMS-AWS-West-Network-Architecture/job/Build%20and%20push%20images/) to prod and dev ECR whenever a change is merged to master.

If you wish to have Jenkins build an image from your branch for you to test on ECS, push it to a remote branch with "build-and-push" in the name. (The image will be pushed to dev and sandbox only, not prod.)

### Manual (VPC Conf)

With AWS CLI >= 1.17.10:

```
SHA=$(git rev-parse --short=8 HEAD)
docker build -t vpc-conf:$SHA -f vpc-conf.Dockerfile .
for REGION in us-east-1 us-west-2
do
 aws ecr get-login-password --region=$REGION |  docker login --username AWS --password-stdin $(aws sts get-caller-identity --output text --query Account).dkr.ecr.$REGION.amazonaws.com
  REMOTE=$(aws sts get-caller-identity --output text --query Account).dkr.ecr.$REGION.amazonaws.com/vpc-conf:$SHA
  docker tag vpc-conf:$SHA $REMOTE
  docker push $REMOTE
done
```

## Deploying

### Production (first time)

1. Run `terraform apply` in `terraform/init` and `terraform/init_east` to create prerequisite infrastructure.
2. Run `terraform apply` in `terraform/prod/account` to create prerequisite IAM roles and policies.
3. Create the `vpc-conf` repos in ECR.
4. Follow the instructions above to build and push the images.
5. Edit `main.tf` in `terraform/prod/{east,west}` to fill in all the correct arguments like `vpc_id`, etc. These resources will either have been created as part of the VPC creation process or as part of steps 2, 3, and 4. Don't forget about the route table IDs and CIDR blocks for the peering connection routes at the bottom. These routes give access to the VPC endpoint for the groot API.
6. Run `terraform apply` in `terraform/prod/{east,west}`.
7. Manually subscribe to the SNS topics that were created for alerts.

### Production (subsequent deploys)

1. Follow the instructions above to build and push a new image.
2. Edit `main.tf` in `terraform/prod/east` to update the image reference.
3. Run `terraform apply` in `terraform/prod/east`.
