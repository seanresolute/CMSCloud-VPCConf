variable "alarm_sns_topic_arn" {
  type = string
}

variable "alarm_definitions" {
  type = map(object({
    Region    = string
    Zone      = string
    Metric    = string
    Threshold = number
  }))
}

variable "metric_namespace" {}

resource "aws_cloudwatch_metric_alarm" "alarm" {
  for_each = var.alarm_definitions

  alarm_name          = "cms-cloud-ipcontrol-${each.key}"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "5"
  metric_name         = each.value.Metric
  namespace           = var.metric_namespace
  period              = "60"
  statistic           = "Average"
  threshold           = each.value.Threshold
  alarm_description   = format("%s for %s/%s", each.value.Metric, each.value.Region, each.value.Zone)
  dimensions = {
    Region = each.value.Region
    Zone   = each.value.Zone
  }

  alarm_actions = [var.alarm_sns_topic_arn]
  ok_actions    = [var.alarm_sns_topic_arn]
}

output "alarms" {
  value = { for k, v in var.alarm_definitions : k => format("%s/%s : %s < %d", v.Region, v.Zone, v.Metric, v.Threshold) }
}
