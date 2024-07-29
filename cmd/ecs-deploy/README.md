# ecs-deploy

## Install

```
go install ./cmd/ecs-deploy
```

## Prerequisites
### Environment Variables
- CLOUDTAMER_BASE_URL
- CLOUDTAMER_ADMIN_GROUP_ID
- CLOUDTAMER_USERNAME
- CLOUDTAMER_PASSWORD
- CLOUDTAMER_IDMS_ID
- VPC_CONF_API_KEYS # see vpc-conf-task-queue README.md
### Configuration file
You will need a configuration file that is a JSON serialization of a `deploy.DeployConfig`. If you are going to use a custom config file that is not in a git repo or that has been modified for testing, you must manually specify some kind of hash ("SHA") that will be used to uniquely identify it.
### Artifact
The image must be registered in ArtifactDB. ArtifactDB records which config SHAs have been used to deploy to which environments; if the `RequiresPreviousEnvDeploy` field in the `EnvConfig` is set then the same artifact must have been deployed to the previous environment using the same config SHA that you are trying to deploy to the current environment.
### AWS setup
You must have one ALB listener serving traffic for the app with two target groups, a "green" and "blue" one. Each target group must have a single listener rule which forwards to it.
### Other tools
#### Git
If you don't manually specify the config SHA then `git` will be called to get it.
#### ArtifactDB
You must have `artifact-db` in your PATH.
#### App-specific
If `AppConfig.PreDeployCommandTemplates` or `AppConfig.PostDeployCommandTemplates` in your deploy config specify any commands to run then those commands must be in your PATH.

## Deploy lock
`ecs-deploy` uses the [dynamolock](https://github.com/cirello-io/dynamolock) API to prevent concurrent deploys to the same app in the same environment. So as to make clock skew irrelevant, this library does not set specific expiration times but rather sends periodic heartbeats. If the previous deploy finished successfully then the lock will be released and the next deploy can acquire it immediately. If the previous deploy failed then the next deploy will have to watch the lock for a period of time to make sure no heartbeats come in; if they do not then it can acquire the lock.

## Doing a deploy
The typical use, say to deploy vpc-conf to dev, will be
```
ecs-deploy -app vpc-conf -env dev -config deploy-config.json -sha abcd1234
```
This will look in ArtifactDB for the latest build in project "vpc-conf" tagged `SHA=abcd1234`. (These artifacts are created and tagged automatically after each merge to master.) After the deploy is completed, ArtifactDB will be updated to record that the artifact was successfully deployed to the dev environment with the config SHA corresponding to the HEAD commit SHA for the repo where `deploy-config.json` lives.

## The deploy process
`ecs-deploy` does the following:
1. Acquire the deploy lock.
2. Run any commands specified in `AppConfig.PreDeployCommandTemplates`.
3. Read the active color from ALB (which target group's rule has lower priority).
4. Stop and delete any non-PRIMARY task sets.
5. Create a new task definition from `AppConfig.ContainerDefinitionTemplates`.
6. Create a new task set with the new task definition.
7. Wait for ECS's rolling deploy of the new task set to complete.
8. [Not implemented yet]
9. Swap the priority of the blue and green ALB rules to send traffic to the new task set.
10. Update the new task set to be PRIMARY.
11. [Not implemented yet]
12. Run any commands specified in `AppConfig.PostDeployCommandTemplates`.

## Resuming and starting over
If a deploy fails then the next time you invoke the application you must choose whether to "resume" or "start over."

To "resume" means to attempt to continue to deploy the same artifact and config SHA: some steps may be repeated but the net effect on AWS state at the end of the deploy will be as if the original deploy succeeded. Specifically, the originally PRIMARY task set (before the first failed deploy) will still be running after the deploy and will now be ACTIVE.

"Start over" means to start the deploy again from the beginning, and may be done with a different artifact or config SHA. No rewinding of the failed deploy(s) will happen. So, for example, if the previous deploy had already swapped the ALB to the new task set, `ecs-deploy` may stop and delete the task set that was originally PRIMARY before the first failed deploy.

## Rollback
Automated rollbacks are not currently supported. If you wish to roll back to a previous deploy you can do one of two things:

**Re-deploy**: just deploy the old artifact/config using a new invocation of `ecs-deploy` (with `-start-over` if the previous deploy never finished). This will generally just work but it is slower because it requires a new task set to be created and rolled out, even if the artifact and config you want are already running on ECS in a non-PRIMARY task set.

**Manually roll back**: swap the ALB rule priorities back so that the old task set is serving, and mark that task set as PRIMARY. This requires that the task set you want to roll back to is still running, in particular that you haven't done any further deploys since it was made non-PRIMARY (which would result in the task set's deletion; see step 4 of the deploy process above). You can continue to use `ecs-deploy` to do further deploys after doing a manual rollback.
