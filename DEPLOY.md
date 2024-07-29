# Deploying new code to vpc-conf
## Prerequisites
* `ecs-deploy`, `ecs-app-status`, and `vpc-conf-task-queue` from this repo installed
* [artifact-db](https://github.cms.gov/superbrilliant/artifact-db) installed
* Environment variables set per [ecs-deploy/README.md](https://github.com/CMSgov/CMS-AWS-West-Network-Architecture/tree/master/vpc-automation/cmd/ecs-deploy#environment-variables)
## Step by step
1. Get an up-to-date master branch:
   ```
   git checkout master
   git pull origin master
   SHA=$(git rev-parse --short=8 HEAD)
   APP=vpc-conf
   ```
   Note: for Dev only (not Prod) you can follow this process from a branch as well if you want to show or demonstrate a change and you can't do it locally. You must push your code to a remote branch with "build-and-push" in the name to have it built and pushed to ECR automatically.
2. Verify that each environment is serving a previous build or config (you will be deploying `$SHA/$SHA`), that no environments are in the middle of a deploy, and that the "Latest" is what you are planning to deploy:
   ```
   ecs-app-status -app $APP -config deploy-config.json
   ```
   If `$SHA` is something other than the latest you can still deploy it, but make sure that you are not confused about what you are doing.
## Dev deploy
3. Do the deploy
   ```
   ecs-deploy -app $APP -env dev -config deploy-config.json -sha $SHA
   ```
4. Verify that dev is now serving `$SHA/$SHA`:
   ```
   ecs-app-status -app $APP -config deploy-config.json
   ```
5. Go to https://dev.vpc-conf.actually-east.west.cms.gov/provision/  and verify that the service is running. Visit https://dev.vpc-conf.actually-east.west.cms.gov/health and https://dev.vpc-request.actually-east.west.cms.gov/health and verify that the services are healthy and can connect to all their backends.
6. If it is possible to test the feature you deployed on dev, do so.
## Prod deploy
7. Do the deploy
   ```
   ecs-deploy -app $APP -env prod -config deploy-config.json -sha $SHA
   ```
8. Verify that prod is now serving `$SHA/$SHA`:
   ```
   ecs-app-status -app $APP -config deploy-config.json
   ```
9. Go to https://vpc-conf.actually-east.west.cms.gov/provision/ and verify that the service is running. Visit https://vpc-conf.actually-east.west.cms.gov/health and https://vpc-request.cloud.cms.gov/health and verify that the services are healthy and can connect to all their backends.
10. If it is possible to test the feature you deployed, do so.

# Update AWS Accounts Microservice deploy
update-aws-accounts is a microservice that syncs accounts from cloudtamer to VPC Conf's DB. Because it only runs every 2 minutes and VPC Conf generally only interacts with accounts many minutes after it is created in cloudtamer, there is no need for a no downtime deploy like the other services.

This service simply uses terraform to swap the container running in ECR.

1. In `./terraform/prod/east/main.tf` update `image` variable in the `update_aws_accounts_container_defs` resource to point to the new container in ECR.

2. `cd ./terraform/prod/east` or `./terraform/dev/east`

3. `terraform init` (only needed if this is the first time applying this terraform)

4. `terraform plan` and verify that only resources related to the update-aws-accounts microservice are being modified.

4. `terraform apply` and verify that the new service is running in ECS.

