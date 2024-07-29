# Exceptional VPC Manager (EVM)

EVM manages VPCs that cannot be _currently_ managed by VPC Conf.

## Features
- Fetches VPCs to manage and the transit gateway template directly from VPC Conf
- Route changes are purely additive
    -  if a prefix list from the target template points to a different resource than the target TGWID it is logged and skipped for manual investigation
- Operations are idempotent - if a route for each prefix list targeting the TGWID already exists we no-op and continue
- All routes in the template are added to all route tables, including main, regardless if a subnet is associated

## Manual management steps
The tool assumes that the target template has been configured in VPC Conf with only prefix list (not CIDR) destinations. 

The tool assumes the following about any prefix lists that are configured for the target TGW template:
- the prefix lists have been created in the Network Architecture Prod account (921617238787)
- the prefix lists have been added to a new or existing resource share
- exception VPCs have been added to the principal list for the resource share


## Configuration

With the exception of the JSON list of VPCs to manage, configuration is provided through environment variables which are present in the ./cmd/evm/.env file.

The key/value entries are standard except for STACKS. STACKS can take a single value: `STACKS=dev`, or a comma-separated list of values (without spaces): `STACKS=dev,impl,prod` 

Make sure to update missing the EUA_ fields before running.

## Running


#### VSCode

Create an entry in launch.json to load the configuration from the env and start EVM.

```
{
    "name": "EVM Launch",
    "type": "go",
    "request": "launch",
    "mode": "auto",
    "program": "${workspaceFolder}/cmd/evm",
    "envFile": "${workspaceFolder}/cmd/evm/.env",
    "args": []
}
```

#### Command Line

From the EVM project directory:
```
export $(grep -v '^#' .env | xargs) && (go build; ./evm)
```

## TODO

- Update mtgas.json in VPC Conf to include region and owning account ID
- Containerize for schedule deployment and stored logs
