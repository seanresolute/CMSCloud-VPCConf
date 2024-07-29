// Lambda function for publishing ipcontrol metrics to cloudwatch

// Role and policies definitions
resource "aws_iam_role" "ipcontrol_metrics_to_cloudwatch_lambda_role" {
  name               = "${var.ipcontrol_metrics_to_cloudwatch_lambda_name}-role"
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

resource "aws_iam_policy" "ipcontrol_metrics_to_cloudwatch_lambda_policy" {
  name        = "${var.ipcontrol_metrics_to_cloudwatch_lambda_name}-policy"
  path        = "/"
  description = "Policy to allow update ipcontrol metrics to cloudwatch lambda to operate"
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
        "${var.cloudwatch_logs_arn_prefix}log-group:/aws/lambda/${var.ipcontrol_metrics_to_cloudwatch_lambda_name}:*"
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
    },
    {
      "Effect": "Allow",
      "Action": "cloudwatch:PutMetricData",
      "Resource": "*"
    }
  ]
}
EOF
}

resource "aws_iam_role_policy_attachment" "ipcontrol_metrics_to_cloudwatch" {
  role       = aws_iam_role.ipcontrol_metrics_to_cloudwatch_lambda_role.name
  policy_arn = aws_iam_policy.ipcontrol_metrics_to_cloudwatch_lambda_policy.arn
}

// Generate the zip

data "archive_file" "zip_ipcontrol_metrics_to_cloudwatch_js" {
  type        = "zip"
  source_file = "ipcontrol_metrics_to_cloudwatch.js"
  output_path = "ipcontrol_metrics_to_cloudwatch.zip"
}

// Function definition

resource "aws_lambda_function" "ipcontrol_metrics_to_cloudwatch" {
  filename         = "ipcontrol_metrics_to_cloudwatch.zip"
  function_name    = var.ipcontrol_metrics_to_cloudwatch_lambda_name
  role             = aws_iam_role.ipcontrol_metrics_to_cloudwatch_lambda_role.arn
  source_code_hash = data.archive_file.zip_ipcontrol_metrics_to_cloudwatch_js.output_base64sha256
  runtime          = "nodejs14.x"
  handler          = "ipcontrol_metrics_to_cloudwatch.handler"
  timeout          = 15

  vpc_config {
    security_group_ids = [var.app_security_group_id]
    subnet_ids         = var.private_subnets
  }
}

// Schedule

resource "aws_cloudwatch_event_target" "run_ipcontrol_metrics_to_cloudwatch_every_ten_minutes" {
  rule      = aws_cloudwatch_event_rule.every_ten_minutes.name
  target_id = "vpc-conf-ipcontrol-metrics-to-cloudwatch-${var.env}"
  arn       = aws_lambda_function.ipcontrol_metrics_to_cloudwatch.arn
}

// Cloudwatch permissions (enabling scheduled triggers)

resource "aws_lambda_permission" "allow_cloudwatch_to_call_ipcontrol_metrics_to_cloudwatch" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.ipcontrol_metrics_to_cloudwatch.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.every_ten_minutes.arn
}
