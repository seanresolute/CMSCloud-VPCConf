provider "aws" {
  region  = "us-east-1"
  version = "~> 3.0"

  allowed_account_ids = ["546085968493"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "vpc-conf-automation-prod-east-tfstate"
    key            = "state"
    region         = "us-east-1"
    dynamodb_table = "vpc-conf-automation-prod-east-lock-table"
  }
}

variable "private_subnets" {
  default = ["subnet-0e716c85b161a0b4b", "subnet-0711d67cabcba685f", "subnet-0748880e05e95863b"]
}

variable "public_subnets" {
  default = ["subnet-0c1a6a8512760855a", "subnet-090ff87a8670a7a4d", "subnet-0908840df2a331b8b"]
}

variable "vpc_id" {
  default = "vpc-0e8a39fbb69113f27"
}

variable "task_role_arn" {
  default = "arn:aws:iam::546085968493:role/vpc-conf-prod"
}

variable "execution_role_arn" {
  default = "arn:aws:iam::546085968493:role/vpc-conf-prod"
}

variable "health_check_iam_role_arn" {
  default = "arn:aws:iam::546085968493:role/health-check-vpc-conf-prod"
}

variable "redeploy_iam_role_arn" {
  default = "arn:aws:iam::546085968493:role/redeploy-vpc-conf-prod"
}

variable main_cert_arn {
  default = "arn:aws:acm:us-east-1:546085968493:certificate/53d9d41b-6c29-4651-9803-0270fb270a11"
}

variable "cloudwatch_logs_arn_prefix" {
  default = "arn:aws:logs:us-east-1:546085968493:"
}

variable "refresh_ipcontrol_lambda_name" {
  default = "cms-cloud-vpc-conf-refresh-ipcontrol"
}

variable "ipcontrol_metrics_to_cloudwatch_lambda_name" {
  default = "cms-cloud-vpc-conf-ipcontrol-metrics-to-cloudwatch"
}

module "vpc_conf_shared" {
  source                            = "../../modules/vpc_conf_shared"
  env                               = "prod"
  vpc_id                            = var.vpc_id
  region                            = "us-east-1"
  private_subnets                   = var.private_subnets
  rds_multi_az                      = true
  manage_rds_snapshots_iam_role_arn = "arn:aws:iam::546085968493:role/manage-rds-snapshots-prod"
  cloudtamer_host                   = "https://cloudtamer.cms.gov/api"
  cloudtamer_idms_id                = "2"
  cloudtamer_admin_group_id         = "901"
}

module "vpc_conf" {
  source = "../../modules/vpc_conf"

  env                       = "prod"
  region                    = "us-east-1"
  vpc_id                    = var.vpc_id
  redeploy_iam_role_arn     = var.redeploy_iam_role_arn
  health_check_iam_role_arn = var.health_check_iam_role_arn
  ipam_host                 = "ipcontrol.awscloud.cms.local:8443"
  jira_config               = file("../../modules/vpc_conf/jira_config.json")
  jira_issue_labels         = file("../../modules/vpc_conf/jira_issue_labels_prod.json")

  update_aws_accounts_container_defs = data.template_file.update_aws_accounts_container_defs.rendered

  ipam_username   = "vpc-conf-prod"
  private_subnets = var.private_subnets

  cloudwatch_logs_arn_prefix = var.cloudwatch_logs_arn_prefix

  refresh_ipcontrol_lambda_name               = var.refresh_ipcontrol_lambda_name
  ipcontrol_metrics_to_cloudwatch_lambda_name = var.ipcontrol_metrics_to_cloudwatch_lambda_name

  ecs_cluster_name       = module.vpc_conf_common.ecs_cluster_name
  ecs_cluster_id         = module.vpc_conf_common.ecs_cluster_id
  every_minute_rule_arn  = module.vpc_conf_common.every_minute_rule_arn
  every_minute_rule_name = module.vpc_conf_common.every_minute_rule_name
  alb_security_group_id  = module.vpc_conf_common.alb_security_group_id
  alarm_sns_topic_arn    = module.vpc_conf_common.alarm_sns_topic_arn
  app_security_group_id  = module.vpc_conf_common.app_security_group_id
  rds_security_group_id  = module.vpc_conf_shared.rds_security_group_id
  azure_ad_client_id     = "e1d7a274-c3c7-436f-be89-ac622eba019f"
  azure_ad_redirect_url  = "https://vpc-conf.actually-east.west.cms.gov/provision/oauth/callback"
  orchestration_base_url = "https://orchestration-api.actually-east.west.cms.gov/"
}

module "vpc_conf_common" {
  source = "../../modules/ecs_app_common"

  appname        = "vpc-conf"
  component_name = "vpcconf"
  env            = "prod"
  vpc_id         = var.vpc_id

  cert_arn             = var.main_cert_arn
  additional_cert_arns = []

  private_subnets = var.private_subnets
  public_subnets  = var.public_subnets
}

data "template_file" "update_aws_accounts_container_defs" {
  template = file("../../update-aws-accounts.json")

  vars = {
    image  = "546085968493.dkr.ecr.us-east-1.amazonaws.com/update-aws-accounts:7e3caab9"
    env    = "prod"
    region = "us-east-1"
  }
}

output "role_arn" {
  value = var.task_role_arn
}

output "vpc_conf_listener_arn" {
  value = module.vpc_conf_common.https_listener_arn
}

output "vpc_conf_blue_target_group_arn" {
  value = module.vpc_conf_common.app_blue_target_group_arn
}

output "vpc_conf_green_target_group_arn" {
  value = module.vpc_conf_common.app_green_target_group_arn
}

output "vpc_conf_app_security_group" {
  value = module.vpc_conf_common.app_security_group_id
}

output "app_subnets" {
  value = var.private_subnets
}
