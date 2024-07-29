# Deploying new code for creds-api to GovCloud

Currently this is mostly the same as a regular deploy. The differences being that the only app for GovCloud deploy (currently) is the creds-api and you must use `deploy-config-gov.json`.

## Prerequisites
* `ecs-deploy`, and `ecs-app-status` from this repo installed
* [artifact-db](https://github.cms.gov/superbrilliant/artifact-db) installed
* Environment variables set per [ecs-deploy/README.md](https://github.com/CMSgov/CMS-AWS-West-Network-Architecture/tree/master/vpc-automation/cmd/ecs-deploy#environment-variables)
## Step by step
1. Get an up-to-date master branch:
   ```
   git checkout master
   git pull origin master
   SHA=$(git rev-parse --short=8 HEAD)
   APP=creds-api
   ```
   Note: for Dev only (not Prod) you can follow this process from a branch as well if you want to show or demonstrate a change and you can't do it locally. You must push your code to a remote branch with "build-and-push" in the name to have it built and pushed to ECR automatically.
2. Verify that each environment is serving a previous build or config (you will be deploying `$SHA/$SHA`), that no environments are in the middle of a deploy, and that the "Latest" is what you are planning to deploy:
   ```
   ecs-app-status -app $APP -config deploy-config-gov.json
   ```
   If `$SHA` is something other than the latest you can still deploy it, but make sure that you are not confused about what you are doing.
## Dev deploy
3. Do the deploy
   ```
   ecs-deploy -app $APP -env dev -config deploy-config-gov.json -sha $SHA
   ```
4. Verify that dev is now serving `$SHA/$SHA`:
   ```
   ecs-app-status -app $APP -config deploy-config-gov.json
   ```
5. Visit https://dev.creds-api-gc.west.cms.gov/health to verify that the service is healthy. 
 
6. If it is possible to test the feature you deployed on dev, do so.
## Prod deploy
7. Do the deploy
   ```
   ecs-deploy -app $APP -env prod -config deploy-config-gov.json -sha $SHA
   ```
8. Verify that prod is now serving `$SHA/$SHA`:
   ```
   ecs-app-status -app $APP -config deploy-config-gov.json
   ```
9. Visit https://creds-api-gc.west.cms.gov/health to verify that the service is healthy. 

10. If it is possible to test the feature you deployed, do so.
