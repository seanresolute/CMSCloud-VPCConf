# update-aws-accounts

This program downloads the complete list of active AWS Accounts from CloudTamer and syncs the VPCConf database to it.

## Environment variables

These are all required, and all should have the same values as VPCConf:
* POSTGRES_CONNECTION_STRING
* CLOUDTAMER_BASE_URL
* CLOUDTAMER_SERVICE_ACCOUNT_USERNAME
* CLOUDTAMER_SERVICE_ACCOUNT_PASSWORD
* CLOUDTAMER_SERVICE_ACCOUNT_IDMS_ID
