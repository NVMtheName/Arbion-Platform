variable "name" {
  type = string
}

variable "region" {
  type = string
}

variable "account_id" {
  type = string
}

variable "alarm_topic_arn" {
  type = string
}

variable "cloudwatch_retention_days" {
  type = number
}

variable "object_lock_retention_days" {
  type = number
}

locals {
  bucket_name              = "${var.name}-${var.account_id}-${var.region}-audit"
  cloudtrail_name          = "${var.name}-management"
  cloudtrail_arn           = "arn:aws:cloudtrail:${var.region}:${var.account_id}:trail/${local.cloudtrail_name}"
  cloudtrail_log_group     = "/aws/cloudtrail/${var.name}"
  cloudtrail_log_group_arn = "arn:aws:logs:${var.region}:${var.account_id}:log-group:${local.cloudtrail_log_group}"
}

data "aws_iam_policy_document" "audit_key" {
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
    sid       = "AllowCloudTrailEncryption"
    effect    = "Allow"
    actions   = ["kms:GenerateDataKey*"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceArn"
      values   = [local.cloudtrail_arn]
    }

    condition {
      test     = "StringLike"
      variable = "kms:EncryptionContext:aws:cloudtrail:arn"
      values   = ["arn:aws:cloudtrail:*:${var.account_id}:trail/*"]
    }
  }

  statement {
    sid       = "AllowCloudTrailKeyInspectionAndBucketKeyUse"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:DescribeKey"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceArn"
      values   = [local.cloudtrail_arn]
    }
  }

  statement {
    sid       = "AllowConfigDelivery"
    effect    = "Allow"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["config.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceAccount"
      values   = [var.account_id]
    }

    condition {
      test     = "ArnLike"
      variable = "AWS:SourceArn"
      values   = ["arn:aws:config:${var.region}:${var.account_id}:*"]
    }
  }

  statement {
    sid       = "AllowCloudWatchLogsEncryption"
    effect    = "Allow"
    actions   = ["kms:Encrypt", "kms:Decrypt", "kms:ReEncrypt*", "kms:GenerateDataKey*", "kms:DescribeKey"]
    resources = ["*"]

    principals {
      type        = "Service"
      identifiers = ["logs.${var.region}.amazonaws.com"]
    }

    condition {
      test     = "ArnEquals"
      variable = "kms:EncryptionContext:aws:logs:arn"
      values   = [local.cloudtrail_log_group_arn]
    }
  }
}

resource "aws_kms_key" "audit" {
  description             = "Arbion audit, configuration, and security-log evidence"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy                  = data.aws_iam_policy_document.audit_key.json

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_kms_alias" "audit" {
  name          = "alias/${var.name}-audit"
  target_key_id = aws_kms_key.audit.key_id
}

resource "aws_s3_bucket" "audit" {
  bucket              = local.bucket_name
  object_lock_enabled = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_ownership_controls" "audit" {
  bucket = aws_s3_bucket.audit.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_versioning" "audit" {
  bucket = aws_s3_bucket.audit.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "audit" {
  bucket = aws_s3_bucket.audit.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.audit.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_public_access_block" "audit" {
  bucket                  = aws_s3_bucket.audit.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_object_lock_configuration" "audit" {
  bucket = aws_s3_bucket.audit.id

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = var.object_lock_retention_days
    }
  }

  depends_on = [aws_s3_bucket_versioning.audit]
}

resource "aws_s3_bucket_lifecycle_configuration" "audit" {
  bucket = aws_s3_bucket.audit.id

  rule {
    id     = "retain-audit-evidence"
    status = "Enabled"

    filter {}

    transition {
      days          = 90
      storage_class = "GLACIER"
    }

    expiration {
      days = var.object_lock_retention_days
    }

    noncurrent_version_expiration {
      noncurrent_days = var.object_lock_retention_days
    }
  }

  depends_on = [aws_s3_bucket_object_lock_configuration.audit]
}

data "aws_iam_policy_document" "audit_bucket" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.audit.arn, "${aws_s3_bucket.audit.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }

  statement {
    sid       = "CloudTrailBucketCheck"
    effect    = "Allow"
    actions   = ["s3:GetBucketAcl", "s3:ListBucket"]
    resources = [aws_s3_bucket.audit.arn]

    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceArn"
      values   = [local.cloudtrail_arn]
    }
  }

  statement {
    sid       = "CloudTrailWrite"
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.audit.arn}/CloudTrail/AWSLogs/${var.account_id}/*"]

    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceArn"
      values   = [local.cloudtrail_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }
  }

  statement {
    sid       = "ConfigBucketCheck"
    effect    = "Allow"
    actions   = ["s3:GetBucketAcl", "s3:ListBucket"]
    resources = [aws_s3_bucket.audit.arn]

    principals {
      type        = "Service"
      identifiers = ["config.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceAccount"
      values   = [var.account_id]
    }
  }

  statement {
    sid       = "ConfigWrite"
    effect    = "Allow"
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.audit.arn}/AWSConfig/AWSLogs/${var.account_id}/Config/*"]

    principals {
      type        = "Service"
      identifiers = ["config.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceAccount"
      values   = [var.account_id]
    }

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }
  }
}

resource "aws_s3_bucket_policy" "audit" {
  bucket = aws_s3_bucket.audit.id
  policy = data.aws_iam_policy_document.audit_bucket.json

  depends_on = [
    aws_s3_bucket_ownership_controls.audit,
    aws_s3_bucket_public_access_block.audit
  ]
}

resource "aws_cloudwatch_log_group" "cloudtrail" {
  name              = local.cloudtrail_log_group
  retention_in_days = var.cloudwatch_retention_days
  kms_key_id        = aws_kms_key.audit.arn
}

data "aws_iam_policy_document" "cloudtrail_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["cloudtrail.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "cloudtrail" {
  name               = "${var.name}-cloudtrail"
  assume_role_policy = data.aws_iam_policy_document.cloudtrail_assume.json
}

resource "aws_iam_role_policy" "cloudtrail" {
  name = "write-cloudtrail-logs"
  role = aws_iam_role.cloudtrail.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
      Resource = "${aws_cloudwatch_log_group.cloudtrail.arn}:*"
    }]
  })
}

resource "aws_cloudtrail" "management" {
  name                          = local.cloudtrail_name
  s3_bucket_name                = aws_s3_bucket.audit.id
  s3_key_prefix                 = "CloudTrail"
  include_global_service_events = true
  is_multi_region_trail         = true
  enable_log_file_validation    = true
  enable_logging                = true
  kms_key_id                    = aws_kms_key.audit.arn
  cloud_watch_logs_group_arn    = "${aws_cloudwatch_log_group.cloudtrail.arn}:*"
  cloud_watch_logs_role_arn     = aws_iam_role.cloudtrail.arn

  event_selector {
    include_management_events = true
    read_write_type           = "All"
  }

  insight_selector {
    insight_type = "ApiCallRateInsight"
  }

  depends_on = [aws_s3_bucket_policy.audit, aws_iam_role_policy.cloudtrail]
}

locals {
  security_metrics = {
    root_account_use = {
      pattern = "{ ($.userIdentity.type = \"Root\") && ($.userIdentity.invokedBy NOT EXISTS) && ($.eventType != \"AwsServiceEvent\") }"
      title   = "Root account use"
    }
    console_login_failure = {
      pattern = "{ ($.eventName = \"ConsoleLogin\") && ($.errorMessage = \"Failed authentication\") }"
      title   = "Console login failure"
    }
    unauthorized_api_call = {
      pattern = "{ ($.errorCode = \"*UnauthorizedOperation\") || ($.errorCode = \"AccessDenied*\") }"
      title   = "Unauthorized API call"
    }
    cloudtrail_change = {
      pattern = "{ ($.eventSource = \"cloudtrail.amazonaws.com\") && (($.eventName = \"CreateTrail\") || ($.eventName = \"UpdateTrail\") || ($.eventName = \"DeleteTrail\") || ($.eventName = \"StartLogging\") || ($.eventName = \"StopLogging\")) }"
      title   = "CloudTrail configuration change"
    }
    iam_policy_change = {
      pattern = "{ ($.eventSource = \"iam.amazonaws.com\") && (($.eventName = \"DeleteGroupPolicy\") || ($.eventName = \"DeleteRolePolicy\") || ($.eventName = \"DeleteUserPolicy\") || ($.eventName = \"PutGroupPolicy\") || ($.eventName = \"PutRolePolicy\") || ($.eventName = \"PutUserPolicy\") || ($.eventName = \"CreatePolicy\") || ($.eventName = \"DeletePolicy\") || ($.eventName = \"CreatePolicyVersion\") || ($.eventName = \"DeletePolicyVersion\") || ($.eventName = \"AttachRolePolicy\") || ($.eventName = \"DetachRolePolicy\")) }"
      title   = "IAM policy change"
    }
    security_group_change = {
      pattern = "{ ($.eventSource = \"ec2.amazonaws.com\") && (($.eventName = \"AuthorizeSecurityGroupIngress\") || ($.eventName = \"AuthorizeSecurityGroupEgress\") || ($.eventName = \"RevokeSecurityGroupIngress\") || ($.eventName = \"RevokeSecurityGroupEgress\") || ($.eventName = \"CreateSecurityGroup\") || ($.eventName = \"DeleteSecurityGroup\")) }"
      title   = "Security group change"
    }
  }
}

resource "aws_cloudwatch_log_metric_filter" "security" {
  for_each       = local.security_metrics
  name           = "${var.name}-${replace(each.key, "_", "-")}"
  pattern        = each.value.pattern
  log_group_name = aws_cloudwatch_log_group.cloudtrail.name

  metric_transformation {
    name      = each.value.title
    namespace = "Arbion/Security"
    value     = "1"
  }
}

resource "aws_cloudwatch_metric_alarm" "security" {
  for_each            = local.security_metrics
  alarm_name          = "${var.name}-${replace(each.key, "_", "-")}"
  alarm_description   = "Arbion security event: ${each.value.title}"
  namespace           = "Arbion/Security"
  metric_name         = each.value.title
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = [var.alarm_topic_arn]
}

data "aws_iam_policy_document" "config_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["config.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "config" {
  name               = "${var.name}-config"
  assume_role_policy = data.aws_iam_policy_document.config_assume.json
}

resource "aws_iam_role_policy_attachment" "config" {
  role       = aws_iam_role.config.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWS_ConfigRole"
}

resource "aws_iam_role_policy" "config_delivery" {
  name = "deliver-config-snapshots"
  role = aws_iam_role.config.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["s3:GetBucketAcl", "s3:ListBucket"]
        Resource = aws_s3_bucket.audit.arn
      },
      {
        Effect   = "Allow"
        Action   = ["s3:PutObject"]
        Resource = "${aws_s3_bucket.audit.arn}/AWSConfig/AWSLogs/${var.account_id}/Config/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl" = "bucket-owner-full-control"
          }
        }
      },
      {
        Effect   = "Allow"
        Action   = ["kms:Decrypt", "kms:GenerateDataKey"]
        Resource = aws_kms_key.audit.arn
      }
    ]
  })
}

resource "aws_config_configuration_recorder" "this" {
  name     = "${var.name}-recorder"
  role_arn = aws_iam_role.config.arn

  recording_group {
    all_supported                 = true
    include_global_resource_types = true
  }
}

resource "aws_config_delivery_channel" "this" {
  name           = "${var.name}-delivery"
  s3_bucket_name = aws_s3_bucket.audit.id
  s3_key_prefix  = "AWSConfig"
  s3_kms_key_arn = aws_kms_key.audit.arn

  snapshot_delivery_properties {
    delivery_frequency = "Six_Hours"
  }

  depends_on = [aws_s3_bucket_policy.audit, aws_iam_role_policy.config_delivery]
}

resource "aws_config_configuration_recorder_status" "this" {
  name       = aws_config_configuration_recorder.this.name
  is_enabled = true

  depends_on = [aws_config_delivery_channel.this]
}

resource "aws_guardduty_detector" "this" {
  enable                       = true
  finding_publishing_frequency = "FIFTEEN_MINUTES"
}

resource "aws_cloudwatch_event_rule" "guardduty_findings" {
  name        = "${var.name}-guardduty-findings"
  description = "Route medium-and-higher GuardDuty findings to the Arbion operations alarm topic."

  event_pattern = jsonencode({
    source      = ["aws.guardduty"]
    detail-type = ["GuardDuty Finding"]
    detail = {
      severity = [{ numeric = [">=", 4] }]
    }
  })
}

resource "aws_cloudwatch_event_target" "guardduty_findings" {
  rule      = aws_cloudwatch_event_rule.guardduty_findings.name
  target_id = "ArbionOperationsAlarmTopic"
  arn       = var.alarm_topic_arn

  depends_on = [aws_sns_topic_policy.security_events]
}

data "aws_iam_policy_document" "alarm_topic" {
  statement {
    sid       = "AccountOwnerAdministration"
    effect    = "Allow"
    actions   = ["SNS:*"]
    resources = [var.alarm_topic_arn]

    principals {
      type        = "AWS"
      identifiers = ["arn:aws:iam::${var.account_id}:root"]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceOwner"
      values   = [var.account_id]
    }
  }

  statement {
    sid       = "GuardDutyEventBridgePublish"
    effect    = "Allow"
    actions   = ["SNS:Publish"]
    resources = [var.alarm_topic_arn]

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }

    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_cloudwatch_event_rule.guardduty_findings.arn]
    }
  }

  statement {
    sid       = "CloudWatchAlarmPublish"
    effect    = "Allow"
    actions   = ["SNS:Publish"]
    resources = [var.alarm_topic_arn]

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
}

resource "aws_sns_topic_policy" "security_events" {
  arn    = var.alarm_topic_arn
  policy = data.aws_iam_policy_document.alarm_topic.json
}

resource "aws_accessanalyzer_analyzer" "this" {
  analyzer_name = "${var.name}-external-access"
  type          = "ACCOUNT"
}

output "audit_bucket_name" {
  value = aws_s3_bucket.audit.id
}

output "audit_kms_key_arn" {
  value = aws_kms_key.audit.arn
}

output "cloudtrail_name" {
  value = aws_cloudtrail.management.name
}

output "config_recorder_name" {
  value = aws_config_configuration_recorder.this.name
}

output "guardduty_detector_id" {
  value = aws_guardduty_detector.this.id
}

output "guardduty_event_rule_arn" {
  value = aws_cloudwatch_event_rule.guardduty_findings.arn
}

output "access_analyzer_arn" {
  value = aws_accessanalyzer_analyzer.this.arn
}
