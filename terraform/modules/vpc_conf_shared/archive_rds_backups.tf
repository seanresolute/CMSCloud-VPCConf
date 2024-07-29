// Lambda function for running health checks and reporting results to CloudWatch

// Function definition
data "archive_file" "archive_rds_backups_zip" {
  type        = "zip"
  source_file = "${path.module}/archive_rds_backups.py"
  output_path = "archive_rds_backups.zip"
}

resource "aws_lambda_function" "archive_rds_backups" {
  function_name = "archive-rds-backups-vpc-conf-${var.region}-${var.env}"

  filename         = data.archive_file.archive_rds_backups_zip.output_path
  source_code_hash = data.archive_file.archive_rds_backups_zip.output_base64sha256

  role    = var.manage_rds_snapshots_iam_role_arn
  handler = "archive_rds_backups.lambda_handler"
  runtime = "python3.8"
  timeout = 60

  environment {
    variables = {
      region = var.region
      db     = aws_db_instance.vpc_conf.identifier
    }
  }
  tags = merge(module.tagging.common_tags)
}

// Schedule

resource "aws_cloudwatch_event_target" "run_archive_rds_backups_every_12_hours" {
  rule      = aws_cloudwatch_event_rule.every_12_hours.name
  target_id = "archive-rds-backups-vpc-conf-${var.env}"
  arn       = aws_lambda_function.archive_rds_backups.arn
}

resource "aws_cloudwatch_event_rule" "every_12_hours" {
  name                = "every-12-hours"
  description         = "Fires every 12 hours"
  schedule_expression = "rate(12 hours)"
  tags                = merge(module.tagging.common_tags)
}

// Logging group

resource "aws_cloudwatch_log_group" "archive_rds_backups" {
  name = "/aws/lambda/${aws_lambda_function.archive_rds_backups.function_name}"
  tags = merge(module.tagging.common_tags)
}

// Cloudwatch permissions (enabling scheduled triggers)

resource "aws_lambda_permission" "allow_cloudwatch_to_call_archive_rds_backups" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.archive_rds_backups.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.every_12_hours.arn
}

// Alarms
resource "aws_sns_topic" "rds_backup" {
  name = "vpc-conf-${var.env}-archive-rds-backups-alarm"
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_metric_alarm" "archive_rds_backups_alarm" {
  alarm_name          = "vpc-conf-${var.env}-archive-rds-backups-alarm"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "1"
  metric_name         = "Errors"

  dimensions = {
    FunctionName = aws_lambda_function.archive_rds_backups.function_name
  }

  namespace          = "AWS/Lambda"
  period             = "86400" // 24 hours
  statistic          = "Maximum"
  threshold          = "0"
  alarm_description  = "[vpc-conf-${var.region}-${var.env}] RDS backup archiving"
  treat_missing_data = "missing"

  alarm_actions             = [aws_sns_topic.rds_backup.arn]
  insufficient_data_actions = [aws_sns_topic.rds_backup.arn]
  ok_actions                = [aws_sns_topic.rds_backup.arn]
  tags                      = merge(module.tagging.common_tags)
}
