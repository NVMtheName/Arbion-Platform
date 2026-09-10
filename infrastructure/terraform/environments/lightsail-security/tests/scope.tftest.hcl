# These tests plan against a mocked provider. They cannot create AWS resources.
mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{}" }
  }
  mock_resource "aws_kms_key" {
    defaults = { arn = "arn:aws:kms:us-east-1:123456789012:key/11111111-1111-1111-1111-111111111111" }
  }
  mock_resource "aws_s3_bucket" {
    defaults = {
      id  = "arbion-production-123456789012-us-east-1-audit"
      arn = "arn:aws:s3:::arbion-production-123456789012-us-east-1-audit"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-east-1:123456789012:log-group:/aws/cloudtrail/arbion-production" }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::123456789012:role/arbion-production-cloudtrail" }
  }
  mock_resource "aws_sns_topic" {
    defaults = { arn = "arn:aws:sns:us-east-1:123456789012:arbion-production-security-alarms" }
  }
}

variables {
  expected_account_id           = "123456789012"
  audit_evidence_retention_days = 365
  security_log_retention_days   = 365
}

run "isolated_security_root" {
  command = plan
  assert {
    condition     = aws_sns_topic.security.name == "arbion-production-security-alarms"
    error_message = "The new security topic must never reuse the existing production-alerts topic."
  }
  assert {
    condition     = aws_kms_key.notifications.enable_key_rotation && aws_kms_key.notifications.deletion_window_in_days == 30
    error_message = "The separate notification key must retain rotation and deletion safety."
  }
  assert {
    condition     = output.review_boundary.status == "PLAN_ONLY_NOT_OPERATING_EVIDENCE" && !output.review_boundary.application_managed && !output.review_boundary.subscription_managed
    error_message = "A plan is not operating evidence and cannot manage application services or subscribers."
  }
  assert {
    condition     = !module.audit.planned_scope.insights_enabled && !module.audit.planned_scope.config_all_supported && toset(module.audit.planned_scope.config_resource_types) == toset(["AWS::IAM::Role", "AWS::IAM::User", "AWS::IAM::Group", "AWS::IAM::Policy", "AWS::S3::Bucket", "AWS::KMS::Key", "AWS::CloudTrail::Trail", "AWS::EC2::SecurityGroup"])
    error_message = "The root must actually wire the bounded Config scope and exclude paid Insights."
  }
  assert {
    condition     = length(module.audit.planned_scope.guardduty_features) == 8 && alltrue([for status in values(module.audit.planned_scope.guardduty_features) : status == "DISABLED"])
    error_message = "All eight planned optional GuardDuty features must be explicitly disabled."
  }
}

run "reject_short_retention" {
  command = plan
  variables { audit_evidence_retention_days = 30 }
  expect_failures = [var.audit_evidence_retention_days]
}

run "reject_fractional_retention" {
  command = plan
  variables { audit_evidence_retention_days = 365.5 }
  expect_failures = [var.audit_evidence_retention_days]
}

run "reject_implicit_account" {
  command = plan
  variables { expected_account_id = "" }
  expect_failures = [var.expected_account_id]
}

run "audit_module_scoped_controls" {
  command = plan
  module { source = "../../modules/audit" }
  variables {
    name                       = "arbion-production"
    region                     = "us-east-1"
    account_id                 = "123456789012"
    alarm_topic_arn            = "arn:aws:sns:us-east-1:123456789012:arbion-production-security-alarms"
    cloudwatch_retention_days  = 365
    object_lock_retention_days = 365
    enable_cloudtrail_insights = false
    config_resource_types      = ["AWS::S3::Bucket", "AWS::IAM::Role"]
    guardduty_feature_status   = { S3_DATA_EVENTS = "DISABLED" }
  }
  assert {
    condition     = length(aws_cloudtrail.management.insight_selector) == 0 && aws_cloudtrail.management.is_multi_region_trail && aws_cloudtrail.management.enable_log_file_validation && aws_cloudtrail.management.enable_logging
    error_message = "Scope excludes paid Insights but must keep the multi-region validated management trail."
  }
  assert {
    condition     = !aws_config_configuration_recorder.this.recording_group[0].all_supported && toset(aws_config_configuration_recorder.this.recording_group[0].resource_types) == toset(["AWS::S3::Bucket", "AWS::IAM::Role"])
    error_message = "Config must record only the explicit approved resource classes."
  }
  assert {
    condition     = aws_guardduty_detector.this.enable && aws_guardduty_detector_feature.explicit["S3_DATA_EVENTS"].status == "DISABLED"
    error_message = "Foundational detection and explicit add-on statuses must remain distinct."
  }
  assert {
    condition     = aws_s3_bucket_object_lock_configuration.audit.rule[0].default_retention[0].mode == "GOVERNANCE" && aws_s3_bucket_object_lock_configuration.audit.rule[0].default_retention[0].days == 365
    error_message = "The plan may not silently choose irreversible compliance retention or a different duration."
  }
  assert {
    condition = length([for statement in data.aws_iam_policy_document.audit_key.statement : statement if statement.sid == "AllowCloudWatchLogsEncryption"]) == 1 && alltrue([
      for statement in data.aws_iam_policy_document.audit_key.statement :
      statement.sid != "AllowCloudWatchLogsEncryption" || (
        !contains(statement.actions, "kms:DescribeKey") &&
        length(statement.condition) == 1 &&
        alltrue([for condition in statement.condition : condition.test == "StringEquals" && condition.variable == "kms:EncryptionContext:aws:logs:arn" && toset(condition.values) == toset(["arn:aws:logs:us-east-1:123456789012:log-group:/aws/cloudtrail/arbion-production"])])
      )
    ])
    error_message = "Log encryption must remain exact-context bound without an unsupported metadata action or ARN operator."
  }
  assert {
    condition     = length([for statement in data.aws_iam_policy_document.audit_key.statement : statement if statement.sid == "AllowCloudWatchLogsKeyInspection" && toset(statement.actions) == toset(["kms:DescribeKey"]) && length(statement.condition) == 0 && length(statement.principals) == 1 && alltrue([for principal in statement.principals : principal.type == "Service" && toset(principal.identifiers) == toset(["logs.us-east-1.amazonaws.com"])])]) == 1
    error_message = "Only metadata inspection may be separated from the encryption-context condition."
  }
}

run "existing_target_defaults_unchanged" {
  command = plan
  module { source = "../../modules/audit" }
  variables {
    name                       = "arbion-production"
    region                     = "us-east-1"
    account_id                 = "123456789012"
    alarm_topic_arn            = "arn:aws:sns:us-east-1:123456789012:arbion-production-alarms"
    cloudwatch_retention_days  = 365
    object_lock_retention_days = 2555
  }
  assert {
    condition     = length(aws_cloudtrail.management.insight_selector) == 1 && aws_config_configuration_recorder.this.recording_group[0].all_supported && length(aws_guardduty_detector_feature.explicit) == 0
    error_message = "Introducing a separate Lightsail root must preserve existing target-design defaults and resource identities."
  }
}
