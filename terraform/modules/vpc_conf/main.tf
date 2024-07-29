data "aws_caller_identity" "current" {
}

variable "env" {
}

variable "vpc_id" {
}

variable "ipam_host" {
}

variable "ipam_username" {
}

variable "region" {
}

variable "redeploy_iam_role_arn" {
}

variable "health_check_iam_role_arn" {
}

variable "every_minute_rule_arn" {
}

variable "every_minute_rule_name" {
}

variable "alarm_sns_topic_arn" {
}

variable "ecs_cluster_id" {
}

variable "ecs_cluster_name" {
}

variable "alb_security_group_id" {
}

variable "rds_security_group_id" {
}

variable "app_security_group_id" {
}

variable "cloudtamer_read_only_group_ids" {
  default = "1526,2058" // PROD -> 1526 = ct-onboard-outreach, 2058 = ct-gss-network-vpcconf-viewer
}                       // DEV  -> 2072 = ct-onboard-outreach, 2596 = ct-gss-network-vpcconf-viewer

variable "replicas" {
  default = 1
}

variable "private_subnets" {
  type = list(string)
}

variable "jira_config" {}
variable "jira_issue_labels" {}

variable "only_aws_account_ids" {
  default = " "
}

variable "azure_ad_client_id" {
  default = " "
}

variable "azure_ad_host" {
  default = "https://login.microsoftonline.us"
}

variable "azure_ad_redirect_url" {
}

variable "azure_ad_tenant_id" {
  default = "7c8bb92a-832a-4d12-8e7e-c569d7b232c9"
}

variable "creds_svc_config" {
  default = " "
}

variable "orchestration_base_url" {
}

variable "cloudwatch_logs_arn_prefix" {
  default = ""
}

variable "refresh_ipcontrol_lambda_name" {
  default = ""
}

variable "ipcontrol_metrics_to_cloudwatch_lambda_name" {
  default = ""
}

module "tagging" {
  source         = "../tagging"
  component_name = "vpcconf"
  environment    = var.env
}

resource "aws_security_group_rule" "allow_lb_to_app" {
  type                     = "ingress"
  from_port                = 0
  to_port                  = 0
  protocol                 = "-1"
  source_security_group_id = var.alb_security_group_id

  security_group_id = var.app_security_group_id
}

resource "aws_security_group_rule" "allow_app_to_internet" {
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = var.app_security_group_id
}

resource "aws_security_group_rule" "allow_postgres_to_app" {
  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  source_security_group_id = var.app_security_group_id

  security_group_id = var.rds_security_group_id
}

resource "aws_ecs_service" "vpc_conf" {
  name           = "vpc-conf-${var.env}"
  cluster        = var.ecs_cluster_id
  desired_count  = var.replicas
  launch_type    = "FARGATE"
  propagate_tags = "TASK_DEFINITION"

  deployment_controller {
    type = "EXTERNAL"
  }

  lifecycle {
    ignore_changes = [
      // Managed by task sets
      load_balancer, network_configuration, task_definition,
    ]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_log_group" "vpc-conf" {
  name = "/aws/ecs/vpc-conf-${var.env}"
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "ipam_host" {
  name  = "vpc-conf-${var.env}-ipam-host"
  type  = "String"
  value = var.ipam_host
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "ipam_username" {
  name  = "vpc-conf-${var.env}-ipam-username"
  type  = "String"
  value = var.ipam_username
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "ipam_password" {
  name  = "vpc-conf-${var.env}-ipam-password"
  type  = "SecureString"
  value = "replacewithrealpasswordmanually"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "groot_config" {
  name = "vpc-conf-${var.env}-groot-config"
  type = "String"

  value = <<EOF
{
  "us-gov-west-1": {
    "BaseURL": "https://vpce-0405d231b5e4488c0-6fjr8w8o.execute-api.us-west-2.vpce.amazonaws.com/prod/babygroot/api/",
    "APIID": "dlej7eoe9d",
    "TGWAccountID": "535697577246"
  },
  "us-west-2": {
    "BaseURL": "https://vpce-0405d231b5e4488c0-6fjr8w8o.execute-api.us-west-2.vpce.amazonaws.com/prod/babygroot/api/",
    "APIID": "dlej7eoe9d",
    "TGWAccountID": "464673255361"
  },
  "us-east-1": {
    "BaseURL": "https://vpce-00ed12d44ca2af418-44g1m2v9.execute-api.us-east-1.vpce.amazonaws.com/prod/babygroot/api/",
    "APIID": "pw5smko7ke",
    "TGWAccountID": "842420567215"
  }
}
EOF
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "jira_username" {
  name  = "vpc-conf-${var.env}-jira-username"
  type  = "SecureString"
  value = "setmanually"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "jira_oauth_config" {
  name  = "vpc-conf-${var.env}-jira-oauth-config"
  type  = "SecureString"
  value = "setmanually"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "jira_config" {
  name = "vpc-conf-${var.env}-jira-config"
  type = "String"

  value = var.jira_config
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "jira_issue_labels" {
  name = "vpc-conf-${var.env}-jira-issue-labels"
  type = "String"

  value = var.jira_issue_labels
}

resource "aws_ssm_parameter" "cloudtamer_read_only_group_ids" {
  name  = "vpc-conf-${var.env}-cloudtamer-read-only-group-ids"
  type  = "String"
  value = var.cloudtamer_read_only_group_ids
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "only_aws_account_ids" {
  name  = "vpc-conf-${var.env}-only-aws-account-ids"
  type  = "String"
  value = var.only_aws_account_ids
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "azure_ad_client_id" {
  name  = "vpc-conf-${var.env}-azure-ad-client-id"
  type  = "String"
  value = var.azure_ad_client_id
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "azure_ad_host" {
  name  = "vpc-conf-${var.env}-azure-ad-host"
  type  = "String"
  value = var.azure_ad_host
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "azure_ad_redirect_url" {
  name  = "vpc-conf-${var.env}-azure-ad-redirect-url"
  type  = "String"
  value = var.azure_ad_redirect_url
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "azure_ad_tenant_id" {
  name  = "vpc-conf-${var.env}-azure-ad-tenant-id"
  type  = "String"
  value = var.azure_ad_tenant_id
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "creds_svc_config" {
  name  = "vpc-conf-${var.env}-creds-svc-config"
  type  = "SecureString"
  value = var.creds_svc_config
  tags  = merge(module.tagging.common_tags)
  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "api_key_config" {
  name  = "vpc-conf-${var.env}-api-key-config"
  type  = "SecureString"
  value = " "
  tags  = merge(module.tagging.common_tags)
  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "orchestration_api_key" {
  name  = "vpc-conf-${var.env}-orchestration-api-key"
  type  = "SecureString"
  value = "xxxxxxxxxxxxxxxx"
  tags  = merge(module.tagging.common_tags)
  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "orchestration_base_url" {
  name  = "vpc-conf-${var.env}-orchestration-base-url"
  type  = "String"
  value = var.orchestration_base_url
  tags  = merge(module.tagging.common_tags)
}
