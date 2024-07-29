module "tagging" {
  source         = "../tagging"
  component_name = var.component_name != "" ? var.component_name : var.appname
  environment    = var.env
}

resource "aws_sns_topic" "alarm" {
  name = "${var.appname}-${var.env}-alarm"
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_event_rule" "every_minute" {
  name                = "${var.appname}-${var.env}-every-minute"
  description         = "Fires every minute"
  schedule_expression = "rate(1 minute)"
  tags                = merge(module.tagging.common_tags)
}

resource "aws_ecr_repository" "app" {
  name                 = var.appname
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ecs_cluster" "app" {
  name = "${var.appname}-${var.env}"
  tags = merge(module.tagging.common_tags)
}

resource "aws_security_group" "app" {
  name        = "${var.appname}-${var.env}-app"
  description = "${var.appname}-${var.env}-app"
  vpc_id      = var.vpc_id
  tags        = merge(module.tagging.common_tags)
}

resource "aws_security_group" "app_lb" {
  name        = "${var.appname}-${var.env}-lb"
  description = "${var.appname}-${var.env}-lb"
  vpc_id      = var.vpc_id

  tags = merge(module.tagging.common_tags)

  lifecycle {
    ignore_changes = [name, description]
  }
}

resource "aws_security_group_rule" "allow_private_to_lb" {
  count       = var.use_public_alb ? 0 : 1
  type        = "ingress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = var.private_lb_ingress_cidrs

  security_group_id = aws_security_group.app_lb.id
}

resource "aws_security_group_rule" "allow_lb_to_internet" {
  count       = var.use_public_alb ? 0 : 1
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = aws_security_group.app_lb.id
}

resource "aws_security_group_rule" "allow_vpn_to_public_lb" {
  count     = var.use_public_alb ? 1 : 0
  type      = "ingress"
  from_port = 0
  to_port   = 0
  protocol  = "-1"

  cidr_blocks = [
    "52.20.26.200/32",   # cloudvpn.cms.gov
    "34.196.35.156/32",  # cloudvpn.cms.gov
    "52.5.212.71/32",    # cloudvpn.cms.gov
    "35.160.156.109/32", # cloudwest.cms.gov
    "52.34.232.30/32",   # cloudwest.cms.gov
  ]

  security_group_id = aws_security_group.app_lb.id
}

resource "aws_security_group_rule" "allow_akamai_to_public_lb" {
  count     = var.use_public_alb ? 1 : 0
  type      = "ingress"
  from_port = 0
  to_port   = 0
  protocol  = "-1"

  cidr_blocks = [
    "184.25.157.0/24",
    "184.26.44.0/24",
    "184.27.120.0/24",
    "184.27.45.0/24",
    "184.28.127.0/24",
    "184.51.101.0/24",
    "184.51.151.0/24",
    "184.51.199.0/24",
    "184.84.239.0/24",
    "2.16.218.0/24",
    "2.18.240.0/24",
    "23.205.127.0/24",
    "23.216.10.0/24",
    "23.219.36.0/24",
    "23.220.148.0/24",
    "23.62.239.0/24",
    "23.67.251.0/24",
    "23.79.240.0/24",
    "72.246.150.0/24",
    "72.246.216.0/24",
    "72.246.52.0/24",
    "72.247.190.0/24",
    "96.7.55.0/24",
    "67.220.142.19/32",
    "67.220.142.20/32",
    "67.220.142.21/32",
    "67.220.142.22/32",
    "66.198.8.141/32",
    "66.198.8.142/32",
    "66.198.8.143/32",
    "66.198.8.144/32",
    "23.48.168.0/22",
    "23.50.48.0/20",
    "72.246.0.22/32",
    "72.246.0.10/32",
    "72.246.116.14/32",
    "72.246.116.10/32",
  ]

  security_group_id = aws_security_group.app_lb.id
}

resource "aws_security_group_rule" "allow_public_lb_to_internet" {
  count       = var.use_public_alb ? 1 : 0
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = aws_security_group.app_lb.id
}

data "aws_partition" "current" {}
data "aws_elb_service_account" "main" {}

resource "aws_s3_bucket" "logs" {
  bucket = "${data.aws_caller_identity.current.account_id}-cms-cloud-lb-logs-${var.appname}-${var.env}"
  acl    = "private"

  versioning {
    enabled = true
  }

  server_side_encryption_configuration {
    rule {
      apply_server_side_encryption_by_default {
        sse_algorithm = "AES256"
      }
    }
  }
  tags = merge(module.tagging.common_tags, {
    Name      = "CMS Cloud Load Balancer logs for ${data.aws_caller_identity.current.account_id}-${var.appname}-${var.env}"
    Automated = "true"
  })

  policy = <<POLICY
{
    "Version": "2012-10-17",
    "Id": "Policy",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "AWS": "${data.aws_elb_service_account.main.arn}"
            },
            "Action": "s3:PutObject",
            "Resource": "arn:${data.aws_partition.current.partition}:s3:::${data.aws_caller_identity.current.account_id}-cms-cloud-lb-logs-${var.appname}-${var.env}/*"
        },
        {
            "Sid": "AllowSSLRequestsOnly",
            "Effect": "Deny",
            "Principal": "*",
            "Action": "s3:*",
            "Resource": [
                "arn:${data.aws_partition.current.partition}:s3:::${data.aws_caller_identity.current.account_id}-cms-cloud-lb-logs-${var.appname}-${var.env}",
                "arn:${data.aws_partition.current.partition}:s3:::${data.aws_caller_identity.current.account_id}-cms-cloud-lb-logs-${var.appname}-${var.env}/*"
            ],
            "Condition": {
                "Bool": {
                    "aws:SecureTransport": "false"
                }
            }
        }
    ]
}
POLICY

}

resource "aws_lb" "app" {
  name               = "${var.appname}-${var.env}"
  internal           = ! var.use_public_alb
  load_balancer_type = "application"
  security_groups    = [aws_security_group.app_lb.id]
  subnets            = var.use_public_alb ? var.public_subnets : var.private_subnets

  enable_deletion_protection = true

  access_logs {
    bucket  = aws_s3_bucket.logs.bucket
    prefix  = "app"
    enabled = var.alb_logs_enabled
  }

  tags = merge(module.tagging.common_tags)

  depends_on = [aws_s3_bucket.logs]
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.app.arn
  port              = "80"
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_302"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.app.arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS-1-2-2017-01"
  certificate_arn   = var.cert_arn

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "Not Found"
      status_code  = "404"
    }
  }
}

resource "aws_lb_target_group" "app_green" {
  name                 = "${var.appname}-${var.env}-green"
  port                 = 80
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = var.vpc_id
  deregistration_delay = 30

  health_check {
    path                = "/health"
    port                = "traffic-port"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    timeout             = 25
    interval            = 30
    matcher             = 200
  }

  lifecycle {
    create_before_destroy = true
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_lb_target_group" "app_blue" {
  name                 = "${var.appname}-${var.env}-blue"
  port                 = 80
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = var.vpc_id
  deregistration_delay = 30

  health_check {
    path                = "/health"
    port                = "traffic-port"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    timeout             = 25
    interval            = 30
    matcher             = 200
  }

  lifecycle {
    create_before_destroy = true
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_lb_listener_rule" "app_green" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 11

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app_green.arn
  }

  lifecycle {
    ignore_changes = [priority] # managed by ecs-deploy
  }

  condition {
    path_pattern {
      values = ["*"]
    }
  }
}

resource "aws_lb_listener_rule" "app_blue" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 12

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app_blue.arn
  }

  lifecycle {
    ignore_changes = [priority] # managed by ecs-deploy
  }

  condition {
    path_pattern {
      values = ["*"]
    }
  }
}

resource "aws_lb_listener_certificate" "internal" {
  for_each        = toset(var.additional_cert_arns)
  listener_arn    = aws_lb_listener.https.arn
  certificate_arn = each.key
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}
locals {
  arn_container = data.aws_region.current.name == "us-gov-west-1" ? "aws-us-gov" : "aws"
}
