variable update_aws_accounts_container_defs {}

resource "aws_cloudwatch_log_group" "update_aws_accounts" {
  name = "/aws/ecs/update-aws-accounts-${var.env}"
  tags = merge(module.tagging.common_tags)
}
resource "aws_ecr_repository" "update_aws_accounts" {
  name                 = "update-aws-accounts"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_ecs_cluster" "update_aws_accounts" {
  name = "update-aws-accounts-${var.env}"
  tags = merge(module.tagging.common_tags)
}

resource "aws_security_group" "update_aws_accounts" {
  name        = "update-aws-accounts-${var.env}"
  description = "update-aws-accounts-${var.env}"
  vpc_id      = var.vpc_id
  tags        = merge(module.tagging.common_tags)
}

resource "aws_security_group_rule" "allow_update_aws_accounts_to_internet" {
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = aws_security_group.update_aws_accounts.id
}

resource "aws_security_group_rule" "allow_postgres_to_update_aws_accounts" {
  type                     = "ingress"
  from_port                = 5432
  to_port                  = 5432
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.update_aws_accounts.id

  security_group_id = var.rds_security_group_id
}

resource "aws_ecs_task_definition" "update_aws_accounts" {
  family = "update-aws-accounts-${var.env}"

  container_definitions = var.update_aws_accounts_container_defs

  requires_compatibilities = ["FARGATE"]

  execution_role_arn = aws_iam_role.update_aws_accounts.arn
  task_role_arn      = aws_iam_role.update_aws_accounts.arn
  memory             = 1024
  cpu                = 512
  network_mode       = "awsvpc"
}


resource "aws_iam_role" "update_aws_accounts" {
  name = "update-aws-accounts-${var.env}"

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {
        "Service": "ecs-tasks.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOF
}

resource "aws_iam_policy" "update_aws_accounts_app_parameter_store" {
  name = "update-aws-accounts-${var.env}-parameter_store"

  policy = <<POLICY
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "ssm:DescribeParameters"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "ssm:GetParameters"
            ],
            "Resource": [
                "arn:aws:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/vpc-conf-${var.env}*",
                "arn:aws:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/vpc-conf/*"
            ]
        }
    ]
}
POLICY
}

resource "aws_iam_policy_attachment" "update_aws_accounts_parameter_store" {
  name       = "update-aws-accounts-${var.env}-parameter-store"
  roles      = [aws_iam_role.update_aws_accounts.name]
  policy_arn = aws_iam_policy.update_aws_accounts_app_parameter_store.arn
}

resource "aws_iam_policy" "update_aws_accounts_cloudwatch_put_metrics" {
  name = "${var.env}-update-aws-accounts-cloudwatch-put-metrics"

  policy = <<POLICY
{
    "Version": "2012-10-17",
    "Statement": {
        "Effect": "Allow",
        "Resource": "*",
        "Action": "cloudwatch:PutMetricData",
        "Condition": {
            "StringEquals": {
                "cloudwatch:namespace": "UpdateAWSAccounts.${var.env}"
            }
        }
    }
}
POLICY
}

resource "aws_iam_role_policy_attachment" "health_check_put_metrics" {
  role       = "update-aws-accounts-${var.env}"
  policy_arn = aws_iam_policy.update_aws_accounts_cloudwatch_put_metrics.arn
}

resource "aws_iam_role_policy_attachment" "update_aws_accounts_ecr" {
  role       = aws_iam_role.update_aws_accounts.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_iam_role_policy_attachment" "update_aws_accounts_ecs" {
  role       = aws_iam_role.update_aws_accounts.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_ecs_service" "update_aws_accounts" {
  name           = "update-aws-accounts-${var.env}"
  cluster        = aws_ecs_cluster.update_aws_accounts.id
  desired_count  = 1
  launch_type    = "FARGATE"
  propagate_tags = "TASK_DEFINITION"

  task_definition = aws_ecs_task_definition.update_aws_accounts.arn

  network_configuration {
    security_groups = [aws_security_group.update_aws_accounts.id]
    subnets         = var.private_subnets
  }

  tags = merge(module.tagging.common_tags)
}
