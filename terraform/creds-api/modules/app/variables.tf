variable "env" {}

variable "redeploy_iam_role_arn" {}

variable "ecs_cluster_id" {}

variable "ecs_cluster_name" {}

variable "replicas" {
  default = 1
}

variable "alb_security_group_id" {}

variable "creds_api_security_group_id" {}

variable "component_name" {}

variable "is_govcloud" {}