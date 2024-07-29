provider "aws" {
  region  = "us-west-2"
  version = "~> 2.70.0"

  allowed_account_ids = ["346570397073"]
}

terraform {
  required_version = ">= 0.12.23"

  backend "s3" {
    bucket         = "creds-api-dev-tfstate"
    key            = "account/state"
    region         = "us-west-2"
    dynamodb_table = "creds-api-dev-lock-table"
  }
}

module "tagging" {
  source         = "../../../../modules/tagging"
  component_name = "creds-api"
  environment    = "dev"
}

module "common_account" {
  source         = "../../../../modules/ecs_app_common/account/"
  env            = "dev"
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