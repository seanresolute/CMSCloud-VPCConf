data "aws_caller_identity" "current" {
}

module "tagging" {
  source         = "../../tagging"
  component_name = var.component_name != "" ? var.component_name : var.appname
  environment    = var.env
}

resource "aws_iam_role" "app" {
  name = "${var.appname}-${var.env}"

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

resource "aws_iam_policy" "app_parameter_store" {
  name = "${var.appname}-${var.env}-parameter_store"

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
                "ssm:GetParameters",
                "ssm:GetParameter"
            ],
            "Resource": [
                "arn:${local.arn_container}:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${var.appname}-${var.env}*",
                "arn:${local.arn_container}:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.appname}/*"
            ]
        }
    ]
}
POLICY

}

resource "aws_iam_policy" "additional_app_parameter_store" {
  for_each = toset(var.additional_app_names)
  name     = "${each.key}-${var.env}-parameter_store"

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
                "ssm:GetParameters",
                "ssm:GetParameter"

            ],
            "Resource": [
                "arn:${local.arn_container}:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${each.key}-${var.env}*",
                "arn:${local.arn_container}:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${each.key}/*"
            ]
        }
    ]
}
POLICY

}

resource "aws_iam_policy" "ecs_access" {
  name = "${var.appname}-${var.env}-ecs-access"

  // As of June 2019, resource-level permissions are not supported
  // for ecs:DescribeServices or ecs:UpdateService.
  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Id": "${var.appname}-${var.env}-ecs-access",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecs:DescribeServices",
        "ecs:UpdateService",
        "ecs:ListTasks",
        "ecs:DescribeTasks"
      ],
      "Resource": "*"
    }
  ]
}
POLICY

}

resource "aws_iam_policy" "cloudwatch_put_metrics" {
  name = "${var.appname}-${var.env}-cloudwatch-put-metrics"

  // As of June 2019, resource-level permissions are not supported
  // for CloudWatch permissions.
  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Id": "${var.appname}-${var.env}-ecs-access",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cloudwatch:PutMetricData"
      ],
      "Resource": "*"
    }
  ]
}
POLICY

}

resource "aws_iam_policy" "assume_ia_operations_role" {
  name   = "${var.appname}-${var.env}-assume-ia-operations-role"
  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": {
    "Effect": "Allow",
    "Action": "sts:AssumeRole",
    "Resource": "arn:${local.arn_container}:iam::*:role/${local.ia_operations_role}"
  }
}
POLICY

}

data "aws_iam_policy_document" "policy" {
  statement {
    sid    = ""
    effect = "Allow"

    principals {
      identifiers = ["lambda.amazonaws.com"]
      type        = "Service"
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "redeploy" {
  name               = "redeploy-${var.appname}-${var.env}"
  assume_role_policy = data.aws_iam_policy_document.policy.json
  tags               = merge(module.tagging.common_tags)
}

resource "aws_iam_role_policy_attachment" "ecs" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "ecr" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_iam_policy_attachment" "parameter_store" {
  name       = "${var.appname}-${var.env}-parameter-store"
  roles      = [aws_iam_role.app.name]
  policy_arn = aws_iam_policy.app_parameter_store.arn
}

resource "aws_iam_policy_attachment" "additional_parameter_store" {
  for_each   = toset(var.additional_app_names)
  name       = "${each.key}-${var.env}-parameter-store"
  roles      = [aws_iam_role.app.name]
  policy_arn = aws_iam_policy.additional_app_parameter_store[each.key].arn
}

resource "aws_iam_role" "health_check" {
  name               = "health-check-${var.appname}-${var.env}"
  assume_role_policy = data.aws_iam_policy_document.policy.json
  tags               = merge(module.tagging.common_tags)
}

resource "aws_iam_role_policy_attachment" "health_check_ecs_access" {
  role       = "health-check-${var.appname}-${var.env}"
  policy_arn = aws_iam_policy.ecs_access.arn
}

resource "aws_iam_role_policy_attachment" "health_check_put_metrics" {
  role       = "health-check-${var.appname}-${var.env}"
  policy_arn = aws_iam_policy.cloudwatch_put_metrics.arn
}

resource "aws_iam_role_policy_attachment" "health_check_cloudwatch_access" {
  role       = "health-check-${var.appname}-${var.env}"
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "health_check_vpc_access" {
  role       = "health-check-${var.appname}-${var.env}"
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

resource "aws_iam_role_policy_attachment" "redeploy_ecs_access" {
  role       = aws_iam_role.redeploy.id
  policy_arn = aws_iam_policy.ecs_access.arn
}

resource "aws_iam_role_policy_attachment" "redeploy_cloudwatch_access" {
  role       = aws_iam_role.redeploy.id
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "assume_ia_operations_role" {
  role       = aws_iam_role.app.name
  policy_arn = aws_iam_policy.assume_ia_operations_role.arn
}

resource "aws_iam_policy" "manage_rds_snapshots" {
  count = var.use_rds ? 1 : 0
  name  = "${var.appname}-${var.env}-manage-rds-snapshots"

  policy = <<POLICY
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "",
            "Effect": "Allow",
            "Action": [
                "rds:CopyDBSnapshot",
                "rds:DeleteDBSnapshot",
                "rds:DescribeDBSnapshots"
            ],
            "Resource": "*"
        }
    ]
}
POLICY

}

resource "aws_iam_role" "manage_rds_snapshots" {
  count              = var.use_rds ? 1 : 0
  name               = "manage-rds-snapshots-${var.env}"
  assume_role_policy = data.aws_iam_policy_document.policy.json
  tags               = merge(module.tagging.common_tags)
}

resource "aws_iam_role_policy_attachment" "manage_rds_snapshots" {
  count      = var.use_rds ? 1 : 0
  role       = aws_iam_role.manage_rds_snapshots[0].id
  policy_arn = aws_iam_policy.manage_rds_snapshots[0].arn
}

resource "aws_iam_role_policy_attachment" "manage_rds_snapshots_cloudwatch_access" {
  count      = var.use_rds ? 1 : 0
  role       = aws_iam_role.manage_rds_snapshots[0].id
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role" "cbc_jenkins" {
  name               = "${var.appname}-${var.env}-cbc-jenkins"
  path               = local.is_govcloud ? "/delegatedadmin/developer/" : null
  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:${local.arn_container}:iam::${local.jenkins_account}:role/cbc-itops-ia"
      },
      "Action": "sts:AssumeRole",
      "Condition": {}
    }
  ]
}
EOF
  tags               = merge(module.tagging.common_tags)
}

resource "aws_iam_role_policy_attachment" "cbc_jenkins_ecr_access" {
  role       = aws_iam_role.cbc_jenkins.id
  policy_arn = "arn:${local.arn_container}:iam::aws:policy/AmazonEC2ContainerRegistryFullAccess"
}

output "aws_iam_role_app_arn" {
  value = aws_iam_role.app.arn
}

output "aws_iam_role_redeploy_arn" {
  value = aws_iam_role.redeploy.arn
}

output "aws_iam_role_health_check_arn" {
  value = aws_iam_role.health_check.arn
}

output "aws_iam_role_manage_rds_snapshots_arn" {
  value = var.use_rds ? aws_iam_role.manage_rds_snapshots[0].arn : null
}

output "aws_iam_role_cbc_jenkins_id" {
  value = aws_iam_role.cbc_jenkins.id
}

data "aws_region" "current" {}
locals {
  is_govcloud        = data.aws_region.current.name == "us-gov-west-1" ? true : false
  arn_container      = local.is_govcloud ? "aws-us-gov" : "aws"
  jenkins_account    = local.is_govcloud ? "020847197482" : "478919403635"
  ia_operations_role = local.is_govcloud ? "cms-cloud-admin/ct-cms-cloud-ia-operations-gov" : "cms-cloud-admin/ct-cms-cloud-ia-operations"
}
