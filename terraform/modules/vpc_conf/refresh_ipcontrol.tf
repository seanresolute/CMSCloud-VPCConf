// Lambda function for refreshing ipcontrol data every six hours

// Role and policies definitions
resource "aws_iam_role" "refresh_ipcontrol_lambda_role" {
  name               = "${var.refresh_ipcontrol_lambda_name}-role"
  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": "sts:AssumeRole",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Effect": "Allow",
      "Sid": ""
    }
  ]
}
EOF
}

resource "aws_iam_policy" "refresh_ipcontrol_lambda_policy" {
  name        = "${var.refresh_ipcontrol_lambda_name}-policy"
  path        = "/"
  description = "Policy to allow refresh lambda to operate"
  policy      = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "logs:CreateLogGroup",
      "Resource": "${var.cloudwatch_logs_arn_prefix}*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": [
        "${var.cloudwatch_logs_arn_prefix}log-group:/aws/lambda/${var.refresh_ipcontrol_lambda_name}:*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "ec2:CreateNetworkInterface",
        "ec2:DeleteNetworkInterface",
        "ec2:DescribeNetworkInterfaces"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": "ssm:GetParameter",
      "Resource": "arn:aws:ssm:us-east-1:546085968493:parameter/vpc-conf-prod-api-key-config"
    }
  ]
}
EOF
}

resource "aws_iam_role_policy_attachment" "refresh_ipcontrol" {
  role       = aws_iam_role.refresh_ipcontrol_lambda_role.name
  policy_arn = aws_iam_policy.refresh_ipcontrol_lambda_policy.arn
}

// Generate the zip

data "archive_file" "zip_refresh_ipcontrol_js" {
  type        = "zip"
  source_file = "refresh_ipcontrol.js"
  output_path = "refresh_ipcontrol.zip"
}

// Function definition

resource "aws_lambda_function" "refresh_ipcontrol" {
  filename         = "refresh_ipcontrol.zip"
  function_name    = var.refresh_ipcontrol_lambda_name
  role             = aws_iam_role.refresh_ipcontrol_lambda_role.arn
  source_code_hash = data.archive_file.zip_refresh_ipcontrol_js.output_base64sha256
  runtime          = "nodejs14.x"
  handler          = "refresh_ipcontrol.handler"
  timeout          = 15

  vpc_config {
    security_group_ids = [var.app_security_group_id]
    subnet_ids         = var.private_subnets
  }
}

// Schedule

resource "aws_cloudwatch_event_target" "run_refresh_every_six_hours" {
  rule      = aws_cloudwatch_event_rule.every_six_hours.name
  target_id = "vpc-conf-refresh-ipcontrol-${var.env}"
  arn       = aws_lambda_function.refresh_ipcontrol.arn
}

// Cloudwatch permissions (enabling scheduled triggers)

resource "aws_lambda_permission" "allow_cloudwatch_to_call_refresh_ipcontrol" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.refresh_ipcontrol.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.every_six_hours.arn
}
