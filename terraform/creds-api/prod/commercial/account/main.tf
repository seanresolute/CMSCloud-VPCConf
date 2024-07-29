provider "aws" {
  region  = "us-west-2"
  version = "~> 2.70.0"

  allowed_account_ids = ["546085968493"]
}

terraform {
  required_version = ">= 0.12.23"

  backend "s3" {
    bucket         = "creds-api-prod-tfstate-546085968493-us-east-1"
    key            = "account/state"
    region         = "us-east-1"
    dynamodb_table = "creds-api-prod-lock-table"
  }
}

module "tagging" {
  source         = "../../../../modules/tagging"
  component_name = "creds-api"
  environment    = "prod"
}

module "common_account" {
  source         = "../../../../modules/ecs_app_common/account/"
  env            = "prod"
  appname        = "creds-api"
  component_name = "creds-api"
  use_rds        = false
}

output "aws_iam_role_app_arn" {
  value = module.common_account.aws_iam_role_app_arn
}

output "aws_iam_role_redeploy_arn" {
  value = module.common_account.aws_iam_role_redeploy_arn
}
