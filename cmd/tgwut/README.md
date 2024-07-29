# tgwut - Transit GateWay Unification Tool

## Concept
The concept behind this tool is to allow for automated handling of all of the routing entries on the unmanaged TGW and the corresponding routing tables and routes.

## Cache Usage
To help streamline operations across executions, there are some pieces of information that we cache in a redis instance:
1. AWS Credential Keys (55 minute TTL)
2. VPC Information (to include routes, subnets, stack, etc) (1 week TTL)
3. TGW Attachments within the main account (30 minute TTL)
4. Route Table Backup (no TTL)

## Routing Config
The following are routes that we care about. The primary route along with the bifurcated routes that we wish to add to replace it are:
* 10.128.0.0/16
    * 10.128.0.0/17
    * 10.128.128.0/17
* 10.131.125.0/24
    * 10.131.125.0/25
    * 10.131.125.128/25
* 10.138.1.0/24
    * 10.138.1.0/25
    * 10.138.1.128/25
* 10.223.120.0/22
    * 10.223.120.0/23
    * 10.223.122.0/23
* 10.223.126.0/23
    * 10.223.126.0/24
    * 10.223.127.0/24
* 10.232.32.0/19
    * 10.232.32.0/20
    * 10.232.48.0/20
* 10.235.58.0/24
    * 10.235.58.0/25
    * 10.235.58.128/25
* 10.244.96.0/19
    * 10.244.96.0/20
    * 10.244.112.0/20

## Usage
The general usage of the app is as such:
```
tgwut [flags] <command>
```
To execute within docker (once you've built the image), ensure you use the env file to bring your needed environment variables in:
```
docker run -it --rm --env-file tgwut.env tgwut [flags] <command>
```

### Global flags
The global flags for selecting VPCs are additive. You can choose to only work with VPCs in "dev" and in us-east-1

#### -threads
Run extra threads - very useful for `refresh`

#### -vpc
Only handle the VPC provided

#### -stack
Only handle VPCs that are marked with the given stack (dev, prod, etc)

#### -region
Only handle VPCs within the given region

##### --greenfield
Perform some functions against the greenfield TGW (useful for post-migration checking)

#### -n
Dry Run - Do not actually update any resources on AWS

### Sub-Commands
#### audit
Check the current state of routes to see how VPCs match with what we expect their state to be

#### create
Create the bifurcated routes based on the config listed above. Any primary route that doesn't already exist will have the bifurcated routes created

#### remove
Remove the bifurcated routes

#### cleanup
Remove the primary routes

#### verify
Verify that the bifurcated routes exist

#### refresh
Refresh the cached state of AWS resources (useful with -threads 20)

#### info
Show details about a VPC (cached info; refresh if you want live data)

#### routes
Show pared down output of current routes that point at the TGW

#### backup-routes
Take a live snapshot of the route-tables in a VPC for possible later restore

#### restore-routes
Restore the TGW specific routes from the backup

#### show-backup
Show the current backup snapshot

#### backup-diff
Show the difference between the current backup and the existing state

#### list
Show a brief one-line output for each VPC that would match the filters

#### fix-dso
Simple function to add the DSO /22 in lieu of the /16

## Local Environment
My current running local environment has a docker container with redis, and I run the app itself within docker to leverage my VPN connection.
To build the image, I use a Dockerfile:
```
# For dev only, reuse dependency layer for faster building

FROM alpine:latest as certs
RUN apk --update add ca-certificates

FROM golang:1.14 as build
COPY ./go.mod /build/go.mod
WORKDIR /build
RUN go mod download
RUN go get github.com/mjibson/esc

COPY . /build
RUN go generate ./cmd/tgwut
RUN CGO_ENABLED=0 GOBIN=/bin/ go install ./cmd/tgwut

FROM scratch
COPY --from=build /bin/tgwut /bin/tgwut
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bin/tgwut"]
```
`./vpc-automation$ docker build .  -f tgwut.Dockerfile -t tgwut:dev`

### redis
`docker run --network tgwut-dev --name redis-host -d redis`

### tgwut
`docker run --rm --env-file tgwut.env --network tgwut-dev tgwut:dev list`

### tgwut.env
The .env file referenced in the `docker run` command can be constructed similar to this:
```
TGWUT_USERNAME
TGWUT_PASSWORD
CLOUDTAMER_BASE_URL=https://cloudtamer.cms.gov/api
VPCCONF_BASE_URL=https://vpc-conf.actually-east.west.cms.gov/provision
CLOUDTAMER_ADMIN_GROUP_ID=956
CLOUDTAMER_IDMS_ID=2
AWS_REGION=us-east-1 
```
Before you run the docker command, make sure you set the username and password variables to your own credentials in your local environment so docker can pick them up:
```
TGWUT_USERNAME=<EUA ID>
TGWUT_PASSWORD=<VPN PASSWORD>
```

## TODO
