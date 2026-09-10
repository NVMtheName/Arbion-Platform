variable "name" {
  type = string
}
variable "region" {
  type = string
}
variable "account_id" {
  type = string
}
variable "retention_days" {
  type    = number
  default = 30
}
variable "alarm_email" {
  type    = string
  default = null
}
variable "cluster_name" {
  type = string
}
variable "service_names" {
  type = set(string)
}
variable "alb_arn_suffix" {
  type    = string
  default = ""
}
variable "db_identifier" {
  type    = string
  default = ""
}
variable "cache_id" {
  type    = string
  default = ""
}

data "aws_iam_policy_document" "alarm_key" {
  statement {
    sid       = "EnableAccountAdministration"
    effect    = "Allow"
    actions   = ["kms:*"]
    resources = ["*"]

    principals {
      type        = "AWS"
      identifiers = ["arn:aws:iam::${var.account_id}:root"]
    }
  }

  statement {
    sid       = "AllowCloudWatchAlarmPublishing"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey*"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["cloudwatch.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [var.account_id]
    }

    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:cloudwatch:${var.region}:${var.account_id}:alarm:${var.name}-*"]
    }
  }

  # AWS does not support confused-deputy condition keys for EventBridge publishing to an
  # encrypted SNS topic. This dedicated key is therefore limited to the single alarm topic.
  statement {
    sid       = "AllowEventBridgePublishing"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey*"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

resource "aws_kms_key" "alarms" {
  description             = "Arbion encrypted operational and security alarms"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy                  = data.aws_iam_policy_document.alarm_key.json

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_kms_alias" "alarms" {
  name          = "alias/${var.name}-alarms"
  target_key_id = aws_kms_key.alarms.key_id
}

resource "aws_cloudwatch_log_group" "this" {

  for_each          = toset(["web", "api", "ai", "migrations"])
  name              = "/ecs/${var.name}/${each.key}"
  retention_in_days = var.retention_days
}
resource "aws_sns_topic" "alarms" {
  name              = "${var.name}-alarms"
  kms_master_key_id = aws_kms_key.alarms.arn
}
resource "aws_sns_topic_subscription" "email" {

  count     = var.alarm_email == null ? 0 : 1
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}
resource "aws_cloudwatch_metric_alarm" "ecs_cpu" {

  for_each    = var.service_names
  alarm_name  = "${var.name}-${each.key}-high-cpu"
  namespace   = "AWS/ECS"
  metric_name = "CPUUtilization"
  dimensions = {
    ClusterName = var.cluster_name, ServiceName = each.key
  }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 2
  threshold           = 85
  comparison_operator = "GreaterThanThreshold"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  treat_missing_data  = "notBreaching"
}
resource "aws_cloudwatch_metric_alarm" "rds_cpu" {

  alarm_name  = "${var.name}-rds-high-cpu"
  namespace   = "AWS/RDS"
  metric_name = "CPUUtilization"
  dimensions = {
    DBInstanceIdentifier = var.db_identifier
  }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 2
  threshold           = 80
  comparison_operator = "GreaterThanThreshold"
  alarm_actions       = [aws_sns_topic.alarms.arn]
}
resource "aws_cloudwatch_metric_alarm" "rds_storage" {

  alarm_name  = "${var.name}-rds-storage-pressure"
  namespace   = "AWS/RDS"
  metric_name = "FreeStorageSpace"
  dimensions = {
    DBInstanceIdentifier = var.db_identifier
  }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 2
  threshold           = 10737418240
  comparison_operator = "LessThanThreshold"
  alarm_actions       = [aws_sns_topic.alarms.arn]
}
output "log_group_names" {
  value = {
    for k, v in aws_cloudwatch_log_group.this : k => v.name
  }
}
output "alarm_topic_arn" {
  value = aws_sns_topic.alarms.arn
}
output "alarm_kms_key_arn" {
  value = aws_kms_key.alarms.arn
}
