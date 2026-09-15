locals {
  name       = "arbion-production"
  topic_name = "${local.name}-security-alarms"
}

# A NEW topic: the existing arbion-production-alerts topic, policy, subscriptions,
# Lightsail alarms, backup notifications, and host settings are not managed here.
data "aws_iam_policy_document" "notification_key" {
  statement {
    sid       = "AccountKeyAdministration"
    actions   = ["kms:*"]
    resources = ["*"]
    principals {
      type        = "AWS"
      identifiers = ["arn:aws:iam::${var.expected_account_id}:root"]
    }
  }
  statement {
    sid       = "CloudWatchSecurityAlarms"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey*"]
    resources = ["*"]
    principals {
      type        = "Service"
      identifiers = ["cloudwatch.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [var.expected_account_id]
    }
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:cloudwatch:us-east-1:${var.expected_account_id}:alarm:${local.name}-*"]
    }
  }
  # EventBridge-to-encrypted-SNS does not support SourceArn/SourceAccount KMS
  # conditions. This dedicated key is not shared with application/backup data.
  # https://docs.aws.amazon.com/sns/latest/dg/sns-key-management.html
  statement {
    sid       = "EventBridgeSecurityFindings"
    actions   = ["kms:Decrypt", "kms:GenerateDataKey*"]
    resources = ["*"]
    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

resource "aws_kms_key" "notifications" {
  description             = "Arbion standalone security notifications; not production application or backup data"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  policy                  = data.aws_iam_policy_document.notification_key.json
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_kms_alias" "notifications" {
  name          = "alias/${local.topic_name}"
  target_key_id = aws_kms_key.notifications.key_id
}

resource "aws_sns_topic" "security" {
  name              = local.topic_name
  kms_master_key_id = aws_kms_key.notifications.arn
  lifecycle {
    prevent_destroy = true
  }
}

# No subscription endpoint is inferred or copied from another topic. Confirmation
# and delivery testing are separate activation requirements, not a plan PASS.
module "audit" {
  source                     = "../../modules/audit"
  name                       = local.name
  region                     = "us-east-1"
  account_id                 = var.expected_account_id
  alarm_topic_arn            = aws_sns_topic.security.arn
  cloudwatch_retention_days  = var.security_log_retention_days
  object_lock_retention_days = var.audit_evidence_retention_days
  enable_cloudtrail_insights = false

  # No account-wide all-supported Config recording or Config rule evaluations.
  # Lightsail host/container controls still require separate host evidence.
  config_resource_types = [
    "AWS::IAM::Role", "AWS::IAM::User", "AWS::IAM::Group", "AWS::IAM::Policy",
    "AWS::S3::Bucket", "AWS::KMS::Key", "AWS::CloudTrail::Trail", "AWS::EC2::SecurityGroup"
  ]

  # Foundational detection stays enabled. AWS may initially enable protection
  # plans on detector creation; verify all feature states after activation.
  guardduty_feature_status = {
    S3_DATA_EVENTS         = "DISABLED"
    EKS_AUDIT_LOGS         = "DISABLED"
    EBS_MALWARE_PROTECTION = "DISABLED"
    RDS_LOGIN_EVENTS       = "DISABLED"
    LAMBDA_NETWORK_LOGS    = "DISABLED"
    RUNTIME_MONITORING     = "DISABLED"
    AI_PROTECTION          = "DISABLED"
    AI_ANALYST             = "DISABLED"
  }
}

output "review_boundary" {
  value = {
    status               = "PLAN_ONLY_NOT_OPERATING_EVIDENCE"
    region               = "us-east-1"
    account_id           = var.expected_account_id
    notification_topic   = aws_sns_topic.security.arn
    audit_bucket         = module.audit.audit_bucket_name
    cloudtrail           = module.audit.cloudtrail_name
    config_recorder      = module.audit.config_recorder_name
    guardduty_detector   = module.audit.guardduty_detector_id
    access_analyzer      = module.audit.access_analyzer_arn
    subscription_managed = false
    application_managed  = false
  }
}
