# jira-oauth

This program updates the Jira oauth config on SSM Parameter Store with a valid oauth access token for Jira for the given user. It is intended to run in the cloud at some point but for now you can run it locally.

## Local usage

1. Get local credentials for the environment you want to update the token in.
2. Set the `AWS_REGION` env variable.
3. Set `JIRA_USERNAME` env variable to the same username as the `vpc-conf-<env>-jira-username` SSM parameter.
4. Set `SSM_PARAMETER_NAME` to `vpc-conf-<env>-jira-oauth-config`.
5. Start jira-oauth.
6. Visit http://localhost:7878/start in an incognito window or some other browser session that is not already logged in to your personal Jira account.
7. Log in using the credentials for `JIRA_USERNAME` from 1Password and grant access.
