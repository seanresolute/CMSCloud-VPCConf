provider "aws" {
  region  = "us-east-1"
  version = "~> 2.70.0"

  allowed_account_ids = ["346570397073"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "vpc-conf-automation-dev-tfstate"
    key            = "account/state"
    region         = "us-east-1"
    dynamodb_table = "vpc-conf-automation-dev-lock-table"
  }
}

module "vpc_conf_account" {
  source = "../../modules/ecs_app_common/account"

  env                  = "dev"
  appname              = "vpc-conf"
  component_name       = "vpcconf"
  additional_app_names = []
}

