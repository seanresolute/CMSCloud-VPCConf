provider "aws" {
  region  = "us-gov-west-1"
  version = "~> 2.70.0"

  allowed_account_ids = ["350522771286"]
}

terraform {
  required_version = ">= 0.12.23"

  backend "s3" {
    bucket         = "creds-api-dev-tfstate"
    key            = "state"
    region         = "us-gov-west-1"
    dynamodb_table = "creds-api-dev-lock-table"
  }
}


module "common" {
  source = "../../../../modules/ecs_app_common/"

  appname                  = var.appname
  component_name           = var.appname
  env                      = var.env
  vpc_id                   = var.vpc_id
  private_subnets          = var.private_subnets
  private_lb_ingress_cidrs = var.private_lb_ingress_cidrs
  use_public_alb           = var.use_public_alb
  cert_arn                 = var.cert_arn
}

module "app" {
  source = "../../../modules/app"

  env                         = var.env
  redeploy_iam_role_arn       = var.redeploy_iam_role_arn
  ecs_cluster_id              = module.common.ecs_cluster_id
  ecs_cluster_name            = module.common.ecs_cluster_name
  replicas                    = var.replicas
  alb_security_group_id       = module.common.alb_security_group_id
  creds_api_security_group_id = module.common.app_security_group_id
  component_name              = var.appname
  is_govcloud                 = var.is_govcloud
}

output "listener_arn" {
  value = module.common.https_listener_arn
}

output "blue_target_group_arn" {
  value = module.common.app_blue_target_group_arn
}

output "green_target_group_arn" {
  value = module.common.app_green_target_group_arn
}

output "app_security_group_id" {
  value = module.common.app_security_group_id
}
