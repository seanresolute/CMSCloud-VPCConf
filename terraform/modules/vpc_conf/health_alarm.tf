resource "aws_cloudwatch_metric_alarm" "health_alarm" {
  alarm_name          = "vpc-conf-${var.env}-health-alarm"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "5"
  metric_name         = "Backends.Healthy"
  namespace           = "VPCConf.${var.env}"
  period              = "60"
  statistic           = "Maximum"
  threshold           = var.replicas
  alarm_description   = "[vpc-conf-${var.env}] Less than ${var.replicas} instances are healthy"
  treat_missing_data  = "breaching"

  alarm_actions = [var.alarm_sns_topic_arn]
  ok_actions    = [var.alarm_sns_topic_arn]
  tags          = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_metric_alarm" "postgres_alarm" {
  alarm_name          = "vpc-conf-${var.env}-postgres-alarm"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "5"
  threshold           = "0"
  alarm_description   = "[vpc-conf-${var.env}] Some healthy backends cannot connect to Postgres"
  treat_missing_data  = "breaching"

  alarm_actions = [var.alarm_sns_topic_arn]
  ok_actions    = [var.alarm_sns_topic_arn]

  metric_query {
    id          = "healthy_no_postgres"
    label       = "Number of healthy backends that can't connect to Postgres"
    expression  = "healthy - postgres"
    return_data = "true"
  }

  metric_query {
    id    = "healthy"
    label = "Number of healthy backends"

    metric {
      metric_name = "Backends.Healthy"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }

  metric_query {
    id    = "postgres"
    label = "Number of backends that can connect to Postgres"

    metric {
      metric_name = "Backends.Connect.Postgres"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_metric_alarm" "ipcontrol_alarm" {
  alarm_name          = "vpc-conf-${var.env}-ipcontrol-alarm"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "5"
  threshold           = "0"
  alarm_description   = "[vpc-conf-${var.env}] Some healthy backends cannot connect to IPControl"
  treat_missing_data  = "breaching"

  alarm_actions = [var.alarm_sns_topic_arn]
  ok_actions    = [var.alarm_sns_topic_arn]

  metric_query {
    id          = "healthy_no_ipcontrol"
    label       = "Number of healthy backends that can't connect to IPControl"
    expression  = "healthy - ipcontrol"
    return_data = "true"
  }

  metric_query {
    id    = "healthy"
    label = "Number of healthy backends"

    metric {
      metric_name = "Backends.Healthy"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }

  metric_query {
    id    = "ipcontrol"
    label = "Number of backends that can connect to IPControl"

    metric {
      metric_name = "Backends.Connect.IPControl"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_metric_alarm" "jira_alarm" {
  alarm_name          = "vpc-conf-${var.env}-jira-alarm"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "5"
  threshold           = "0"
  alarm_description   = "[vpc-conf-${var.env}] Some healthy backends cannot connect to JIRA"
  treat_missing_data  = "breaching"

  alarm_actions = [var.alarm_sns_topic_arn]
  ok_actions    = [var.alarm_sns_topic_arn]

  metric_query {
    id          = "healthy_no_jira"
    label       = "Number of healthy backends that can't connect to JIRA"
    expression  = "healthy - jira"
    return_data = "true"
  }

  metric_query {
    id    = "healthy"
    label = "Number of healthy backends"

    metric {
      metric_name = "Backends.Healthy"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }

  metric_query {
    id    = "jira"
    label = "Number of backends that can connect to JIRA"

    metric {
      metric_name = "Backends.Connect.JIRA"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }
  tags = merge(module.tagging.common_tags)
}

resource "aws_cloudwatch_metric_alarm" "jira_issue_alarm" {
  alarm_name          = "vpc-conf-${var.env}-jira-issue-alarm"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = "5"
  threshold           = "0"
  alarm_description   = "[vpc-conf-${var.env}] VPC Conf cannot create Jira issues"
  treat_missing_data  = "breaching"

  alarm_actions = ["${var.alarm_sns_topic_arn}"]
  ok_actions    = ["${var.alarm_sns_topic_arn}"]

  metric_query {
    id          = "jira_num_errors"
    label       = "Number of Jira issue create failures"
    return_data = "true"

    metric {
      metric_name = "JiraNumErrors"
      namespace   = "VPCConf.${var.env}"
      period      = "60"
      stat        = "Maximum"
    }
  }
  tags = merge(module.tagging.common_tags)
}
