variable "env" {
}

variable "vpc_id" {
}

variable "region" {
}

variable "private_subnets" {
  type = list(string)
}

variable "rds_multi_az" {
}

variable "manage_rds_snapshots_iam_role_arn" {
}

variable "cloudtamer_host" {
}

variable "cloudtamer_idms_id" {
}

variable "cloudtamer_service_account_idms_id" {
  default = 2
}

variable "cloudtamer_admin_group_id" {
}

module "tagging" {
  source         = "../tagging"
  component_name = "vpcconf"
  environment    = var.env
}

resource "aws_db_subnet_group" "default" {
  name       = "vpc-conf-${var.env}"
  subnet_ids = var.private_subnets
  tags       = merge(module.tagging.common_tags)
}

resource "aws_security_group" "vpc_conf_rds" {
  name        = "vpc-conf-${var.env}-rds"
  description = "vpc-conf-${var.env}-rds"
  vpc_id      = var.vpc_id
  tags        = merge(module.tagging.common_tags)
}

output "rds_security_group_id" {
  value = aws_security_group.vpc_conf_rds.id
}

resource "aws_security_group_rule" "allow_rds_to_internet" {
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = aws_security_group.vpc_conf_rds.id
}

data "aws_security_group" "shared_services" {
  name   = "cmscloud-shared-services"
  vpc_id = var.vpc_id
}

resource "aws_db_instance" "vpc_conf" {
  identifier                          = "vpc-conf-${var.env}"
  allocated_storage                   = 20
  storage_type                        = "gp2"
  engine                              = "postgres"
  engine_version                      = "10.6"
  instance_class                      = "db.m5.large"
  username                            = "postgres"
  password                            = "replacewithrealpasswordmanually"
  parameter_group_name                = "default.postgres10"
  iam_database_authentication_enabled = "true"
  db_subnet_group_name                = "vpc-conf-${var.env}"
  vpc_security_group_ids              = [aws_security_group.vpc_conf_rds.id, data.aws_security_group.shared_services.id]
  storage_encrypted                   = "true"
  multi_az                            = var.rds_multi_az

  backup_retention_period = 7

  lifecycle {
    ignore_changes = [
      password,
      snapshot_identifier,
      instance_class,
      engine_version,
    ]
  }
  tags = merge(module.tagging.common_tags)
}


resource "aws_ssm_parameter" "rds_conn_str" {
  name  = "vpc-conf-${var.env}-rds-conn-str"
  type  = "SecureString"
  value = "postgresql://postgres:replacewithrealpasswordmanually@${aws_db_instance.vpc_conf.address}:${aws_db_instance.vpc_conf.port}"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_host" {
  name  = "vpc-conf-${var.env}-cloudtamer-host"
  type  = "String"
  value = var.cloudtamer_host
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_idms_id" {
  name  = "vpc-conf-${var.env}-cloudtamer-idms-id"
  type  = "String"
  value = var.cloudtamer_idms_id
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_admin_group_id" {
  name  = "vpc-conf-${var.env}-cloudtamer-admin-group-id"
  type  = "String"
  value = var.cloudtamer_admin_group_id
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_service_account_idms_id" {
  name  = "vpc-conf-${var.env}-cloudtamer-service-account-idms-id"
  type  = "String"
  value = var.cloudtamer_service_account_idms_id
  tags  = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_service_account_username" {
  name  = "vpc-conf-${var.env}-cloudtamer-service-account-username"
  type  = "SecureString"
  value = "setmanually"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ssm_parameter" "cloudtamer_service_account_password" {
  name  = "vpc-conf-${var.env}-cloudtamer-service-account-password"
  type  = "SecureString"
  value = "setmanually"

  lifecycle {
    ignore_changes = [value]
  }
  tags = merge(module.tagging.common_tags)
}
