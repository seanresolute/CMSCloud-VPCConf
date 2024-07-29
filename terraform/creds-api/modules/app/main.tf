module "tagging" {
  source         = "../../../modules/tagging/"
  component_name = var.component_name
  environment    = var.env
}

resource "aws_security_group_rule" "allow_lb_to_app" {
  type                     = "ingress"
  from_port                = 0
  to_port                  = 0
  protocol                 = "-1"
  source_security_group_id = var.alb_security_group_id

  security_group_id = var.creds_api_security_group_id
}

resource "aws_security_group_rule" "allow_app_to_internet" {
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = var.creds_api_security_group_id
}

resource "aws_ecs_cluster" "creds_api" {
  name = "creds-api-${var.env}"
  tags = merge(module.tagging.common_tags)
}

resource "aws_ecs_cluster_capacity_providers" "creds_api" {
  cluster_name = aws_ecs_cluster.creds_api.name

  capacity_providers = ["FARGATE"]

  default_capacity_provider_strategy {
    base              = 1
    weight            = 100
    capacity_provider = "FARGATE"
  }
}

resource "aws_ecs_service" "creds_api" {
  name                = "creds-api-${var.env}"
  cluster             = aws_ecs_cluster.creds_api.id
  desired_count       = var.replicas
  launch_type         = "FARGATE"
  scheduling_strategy = "REPLICA"
  propagate_tags      = "TASK_DEFINITION"

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

resource "aws_cloudwatch_log_group" "creds_api" {
  name = "/aws/ecs/creds-api-${var.env}"
}

resource "aws_ssm_parameter" "api_key_config" {
  name  = "creds-api-${var.env}-api-key-config"
  type  = "SecureString"
  value = "-"
  tags  = merge(module.tagging.common_tags)
  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "is_govcloud" {
  name  = "creds-api-${var.env}-is-govcloud"
  type  = "String"
  value = var.is_govcloud
  tags  = merge(module.tagging.common_tags)
}
