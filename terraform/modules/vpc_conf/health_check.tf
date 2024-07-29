// Lambda function for running health checks and reporting results to CloudWatch

// Function definition
data "archive_file" "health_check_zip" {
  type        = "zip"
  source_file = "${path.module}/../vpc_conf_shared/health_check.py"
  output_path = "health_check.zip"
}

resource "aws_lambda_function" "health_check" {
  function_name = "health-check-vpc-conf-${var.region}-${var.env}"

  filename         = data.archive_file.health_check_zip.output_path
  source_code_hash = data.archive_file.health_check_zip.output_base64sha256

  role    = var.health_check_iam_role_arn
  handler = "health_check.lambda_handler"
  runtime = "python3.8"
  timeout = 60

  vpc_config {
    subnet_ids         = var.private_subnets
    security_group_ids = [aws_security_group.vpc_conf_health_check.id]
  }

  environment {
    variables = {
      region           = data.aws_region.current.name
      cluster          = var.ecs_cluster_name
      service          = aws_ecs_service.vpc_conf.name
      metric_namespace = "VPCConf.${var.env}"
    }
  }
  tags = merge(module.tagging.common_tags)
}

// Schedule

resource "aws_cloudwatch_event_target" "run_health_check_every_minute" {
  rule      = var.every_minute_rule_name
  target_id = "health-check-vpc-conf-${var.env}"
  arn       = aws_lambda_function.health_check.arn
}

// Logging permissions

resource "aws_cloudwatch_log_group" "health_check" {
  name = "/aws/lambda/${aws_lambda_function.health_check.function_name}"
  tags = merge(module.tagging.common_tags)
}

// Cloudwatch permissions (enabling scheduled triggers)

resource "aws_lambda_permission" "allow_cloudwatch_to_call_health_check" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.health_check.function_name
  principal     = "events.amazonaws.com"
  source_arn    = var.every_minute_rule_arn
}

// Security group allowing direct access to backends
resource "aws_security_group" "vpc_conf_health_check" {
  name        = "vpc-conf-${var.env}-health-check"
  description = "vpc-conf-${var.env}-health-check"
  vpc_id      = var.vpc_id
  tags        = merge(module.tagging.common_tags)
}

resource "aws_security_group_rule" "allow_health_check_to_app" {
  type                     = "ingress"
  from_port                = 0
  to_port                  = 0
  protocol                 = "-1"
  source_security_group_id = aws_security_group.vpc_conf_health_check.id

  security_group_id = var.app_security_group_id
}

resource "aws_security_group_rule" "allow_health_check_to_internet" {
  type        = "egress"
  from_port   = 0
  to_port     = 0
  protocol    = "-1"
  cidr_blocks = ["0.0.0.0/0"]

  security_group_id = aws_security_group.vpc_conf_health_check.id
}

