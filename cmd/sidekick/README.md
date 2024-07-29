# Sidekick!

Curently Sidekick launches the "verify state" batch task in VPC Conf based on the configured regions and stacks, waits for the task to finish, and records the success or failure status.

This initial version will be run locally until other blockers, like service accounts, are removed.

## ENV Configuration
```
EUA_USERNAME=
EUA_PASSWORD=
VPC_CONF_BASE_URL=http://127.0.0.1:2020/provision
SIDEKICK_POSTGRES_CONNECTION_STRING="postgresql://postgres:XXXXXXX@:5432/sidekick?sslmode=disable"
```

## Usage

Executing sidekick without any flags will include targets for every region and stack.

### Regions
The full list of regions is provided by vpc-conf so this list will automatically grow if new regions are added

`us-east-1, us-west-2, us-gov-west-1`

### Stacks:
Stacks are non-dynamic, but are unlikely to change

`sandbox, dev, test, impl, prod`

`# ./sidekick`

You may limit the target regions and/or stacks with the corrosponding flags.

`# sidekick -regions=us-east-1,use-west-2 -stacks=sandbox,dev,test`

## Future Additions
- Deployable in ECS once service account authentication is sorted out
- Launched via ECS Scheduled Tasks
- Email notification for any non-succeful runs for manual review
- Reporting UI, or generator, to show compliance over time
- Archiving of VPC Conf batch task run data/logs (maybe)