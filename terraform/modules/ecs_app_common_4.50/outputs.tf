output "alarm_sns_topic_arn" {
  value = aws_sns_topic.alarm.arn
}

output "every_minute_rule_arn" {
  value = aws_cloudwatch_event_rule.every_minute.arn
}

output "every_minute_rule_name" {
  value = aws_cloudwatch_event_rule.every_minute.name
}

output "ecs_cluster_id" {
  value = aws_ecs_cluster.app.id
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.app.name
}

output "app_security_group_id" {
  value = aws_security_group.app.id
}

output "alb_security_group_id" {
  value = aws_security_group.app_lb.id
}

output "alb_hostname" {
  value = aws_lb.app.dns_name
}

output "https_listener_arn" {
  value = aws_lb_listener.https.arn
}

output "app_blue_target_group_arn" {
  value = aws_lb_target_group.app_blue.arn
}

output "app_green_target_group_arn" {
  value = aws_lb_target_group.app_green.arn
}