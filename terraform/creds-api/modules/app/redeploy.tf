// Lambda function for redeploying every 72 hours

// Function definition
data "archive_file" "redeploy_zip" {
  type        = "zip"
  source_file = "${path.module}/redeploy.py"
  output_path = "redeploy.zip"
}

data "aws_region" "current" {
}

resource "aws_lambda_function" "redeploy" {
  function_name = "redeploy-creds-api-${var.env}"

  filename         = data.archive_file.redeploy_zip.output_path
  source_code_hash = data.archive_file.redeploy_zip.output_base64sha256

  role    = var.redeploy_iam_role_arn
  handler = "redeploy.lambda_handler"
  runtime = "python3.8"
  timeout = 30

  environment {
    variables = {
      region           = data.aws_region.current.name
      cluster          = var.ecs_cluster_name
      service          = aws_ecs_service.creds_api.name
      redeploy_seconds = 60 * 60 * 71 // 71 hours
    }
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_event_rule" "every_hour" {
  name                = "creds-api-${var.env}-every-hour"
  description         = "Fires every hour"
  schedule_expression = "rate(60 minutes)"
  tags                = merge(module.tagging.common_tags)
}

// Schedule

resource "aws_cloudwatch_event_target" "run_redeploy_every_hour" {
  rule      = aws_cloudwatch_event_rule.every_hour.name
  target_id = "redeploy-creds-api-${var.env}"
  arn       = aws_lambda_function.redeploy.arn
}

// Logging permissions

resource "aws_cloudwatch_log_group" "redeploy" {
  name = "/aws/lambda/${aws_lambda_function.redeploy.function_name}"
  tags = merge(module.tagging.common_tags)
}

// Cloudwatch permissions (enabling scheduled triggers)

resource "aws_lambda_permission" "allow_cloudwatch_to_call_redeploy" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.redeploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.every_hour.arn
}

