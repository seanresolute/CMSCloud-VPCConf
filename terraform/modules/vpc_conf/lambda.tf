// Shared resources for all Lambda functions

resource "aws_cloudwatch_event_rule" "every_hour" {
  name                = "vpc-conf-${var.env}-every-hour"
  description         = "Fires every hour"
  schedule_expression = "rate(60 minutes)"
  tags                = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_event_rule" "every_six_hours" {
  name                = "vpc-conf-${var.env}-every-six-hours"
  description         = "Fires every six hours"
  schedule_expression = "rate(6 hours)"
  tags                = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_event_rule" "every_ten_minutes" {
  name                = "vpc-conf-${var.env}-every-ten-minutes"
  description         = "Fires every 10 minutes"
  schedule_expression = "rate(10 minutes)"
  tags                = merge(module.tagging.common_tags)
}

data "aws_region" "current" {
}

