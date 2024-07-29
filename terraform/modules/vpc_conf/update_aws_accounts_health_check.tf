resource "aws_cloudwatch_metric_alarm" "update_aws_accounts_health_alarm" {
  alarm_name          = "update-aws-accounts-${var.env}-health-alarm"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "5"
  metric_name         = "Updates"
  namespace           = "UpdateAWSAccounts.${var.env}"
  period              = "60"
  statistic           = "Maximum"
  threshold           = 1
  alarm_description   = "[update-aws-accounts-${var.env}] No AWS account updates in 5 minutes"
  treat_missing_data  = "breaching"

  alarm_actions = [aws_sns_topic.update_aws_accounts_alarm.arn]
  ok_actions    = [aws_sns_topic.update_aws_accounts_alarm.arn]
}

resource "aws_sns_topic" "update_aws_accounts_alarm" {
  name = "update-aws-accounts-${var.env}-alarm"
  tags = merge(merge(module.tagging.common_tags))
}
