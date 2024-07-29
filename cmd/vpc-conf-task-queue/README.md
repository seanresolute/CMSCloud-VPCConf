# vpc-conf-task-queue

## Installation
```
go install ./cmd/vpc-conf-task-queue
```

## Prerequisites
Set `VPC_CONF_API_KEYS` env variable and be on VPN. 

Example: `export VPC_CONF_API_KEYS='{"dev":"xxxxxxxxxxxxxxxx", "prod": "xxxxxxxxxxxxxxxx"}'`

An API key for each environment can be found in Parameter Store under `vpc-conf-{env}-api-keys`.

## Example usage
```
vpc-conf-task-queue  -env dev stop
# Do a deploy; wait until no old tasks are running
vpc-conf-task-queue  -env dev -allow-all start
```

## More details
```
vpc-conf-task-queue  -help
```
