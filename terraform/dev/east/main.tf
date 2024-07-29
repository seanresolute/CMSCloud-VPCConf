provider "aws" {
  region  = "us-east-1"
  version = "~> 3.0"

  allowed_account_ids = ["346570397073"]
}

terraform {
  required_version = "= 0.12.23"

  backend "s3" {
    bucket         = "vpc-conf-automation-dev-east-tfstate"
    key            = "state"
    region         = "us-east-1"
    dynamodb_table = "vpc-conf-automation-dev-east-lock-table"
  }
}

variable "private_subnets" {
  default = ["subnet-020e69ca13a260ebf", "subnet-0970fe25c0b2b62e9", "subnet-0e84ad7f96523c669"]
}

variable "public_subnets" {
  default = ["subnet-09d1618c65885c696", "subnet-0f7c85224e3d5d1f8", "subnet-09bcd5e26c06deb2f"]
}

variable "vpc_id" {
  default = "vpc-09474408ccf0a53f9"
}

variable "task_role_arn" {
  default = "arn:aws:iam::346570397073:role/vpc-conf-dev"
}

variable "execution_role_arn" {
  default = "arn:aws:iam::346570397073:role/vpc-conf-dev"
}

variable "health_check_iam_role_arn" {
  default = "arn:aws:iam::346570397073:role/health-check-vpc-conf-dev"
}

variable "redeploy_iam_role_arn" {
  default = "arn:aws:iam::346570397073:role/redeploy-vpc-conf-dev"
}

variable main_cert_arn {
  default = "arn:aws:acm:us-east-1:346570397073:certificate/b62f4a2a-1dd5-41c1-aa72-2b6b53864a97"
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

variable additional_cert_arns {
  default = []
}

module "vpc_conf_shared" {
  source                            = "../../modules/vpc_conf_shared"
  env                               = "dev"
  vpc_id                            = var.vpc_id
  region                            = "us-east-1"
  private_subnets                   = var.private_subnets
  rds_multi_az                      = true
  manage_rds_snapshots_iam_role_arn = "arn:aws:iam::346570397073:role/manage-rds-snapshots-dev"
  cloudtamer_host                   = "https://cloudtamer.cms.gov/api"
  cloudtamer_idms_id                = "2"
  cloudtamer_admin_group_id         = "901"
}

module "vpc_conf" {
  source = "../../modules/vpc_conf"

  env                       = "dev"
  region                    = "us-east-1"
  vpc_id                    = var.vpc_id
  redeploy_iam_role_arn     = var.redeploy_iam_role_arn
  health_check_iam_role_arn = var.health_check_iam_role_arn
  ipam_host                 = "ipcontrol.awscloud.cms.local:8443"
  jira_config               = file("../../modules/vpc_conf/jira_config.json")
  jira_issue_labels         = file("../../modules/vpc_conf/jira_issue_labels_dev.json")

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
  only_aws_account_ids   = " "
  azure_ad_client_id     = "ca4aae61-e7cf-4f9f-9688-0f6e8f60d8a1"
  azure_ad_redirect_url  = "https://dev.vpc-conf.actually-east.west.cms.gov/provision/oauth/callback"
  orchestration_base_url = "https://dev.orchestration-api.actually-east.west.cms.gov/"
}

module "vpc_conf_common" {
  source = "../../modules/ecs_app_common"

  appname        = "vpc-conf"
  component_name = "vpcconf"
  env            = "dev"
  vpc_id         = var.vpc_id

  cert_arn             = var.main_cert_arn
  additional_cert_arns = var.additional_cert_arns

  private_subnets = var.private_subnets
  public_subnets  = var.public_subnets
}

data "template_file" "update_aws_accounts_container_defs" {
  template = file("../../update-aws-accounts.json")

  vars = {
    image  = "346570397073.dkr.ecr.us-east-1.amazonaws.com/update-aws-accounts:7e3caab9"
    env    = "dev"
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
