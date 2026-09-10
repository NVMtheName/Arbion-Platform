#!/usr/bin/env python3
"""Build a deterministic, non-authoritative review draft from verified SOC 2 evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Iterable, Sequence

from soc2_alert_evidence import valid_topic


MAXIMUM_REPORT_BYTES = 1_048_576
MAXIMUM_SOURCE_BYTES = 2_097_152
MAXIMUM_MANIFEST_BYTES = 65_536
SAFE_OUTPUT_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*\.json$")
SHA256 = re.compile(r"^[0-9a-f]{64}$")
MISSING = object()


class ReviewError(RuntimeError):
    """A safe review-draft precondition failed."""


def no_duplicate_object_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ReviewError(f"duplicate JSON object key: {key}")
        result[key] = value
    return result


def parse_json_bytes(raw: bytes, source: str) -> Any:
    try:
        return json.loads(raw, object_pairs_hook=no_duplicate_object_pairs)
    except ReviewError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ReviewError(
            f"malformed JSON after snapshot verification: {source}"
        ) from exc


def nested(value: Any, *keys: str) -> Any:
    current = value
    for key in keys:
        if not isinstance(current, dict) or key not in current:
            return MISSING
        current = current[key]
    return current


def is_bool(value: Any) -> bool:
    return isinstance(value, bool)


def is_nonempty_string(value: Any) -> bool:
    return isinstance(value, str) and bool(value) and len(value) <= 2_048


def is_string_list(value: Any) -> bool:
    return isinstance(value, list) and all(is_nonempty_string(item) for item in value)


def path_is_within(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
    except ValueError:
        return False
    return True


def read_regular_bytes(path: Path, maximum_bytes: int, label: str) -> bytes:
    flags = os.O_RDONLY | getattr(os, "O_CLOEXEC", 0) | getattr(os, "O_NOFOLLOW", 0)
    try:
        descriptor = os.open(path, flags)
    except OSError as exc:
        raise ReviewError(f"{label} is missing or unsafe") from exc
    try:
        information = os.fstat(descriptor)
        if not stat.S_ISREG(information.st_mode):
            raise ReviewError(f"{label} is missing or unsafe")
        handle = os.fdopen(descriptor, "rb")
        descriptor = -1
        with handle:
            raw = handle.read(maximum_bytes + 1)
    except OSError as exc:
        raise ReviewError(f"{label} could not be read safely") from exc
    finally:
        if descriptor >= 0:
            os.close(descriptor)
    if not raw or len(raw) > maximum_bytes:
        raise ReviewError(f"{label} is empty or exceeds its size limit")
    return raw


@dataclass(frozen=True)
class Source:
    name: str
    digest: str
    data: Any

    @property
    def unavailable(self) -> bool:
        return isinstance(self.data, dict) and self.data.get("status") == "UNAVAILABLE"


@dataclass(frozen=True)
class Assertion:
    assertion_id: str
    title: str
    system: str
    controls: tuple[str, ...]
    files: tuple[str, ...]
    evaluate: Callable[[Sequence[Any]], tuple[bool, bool]]


def github_identity_binding(values: Sequence[Any]) -> tuple[bool, bool]:
    identity, repository, summary = values
    schema = (
        isinstance(identity, dict)
        and is_nonempty_string(identity.get("login"))
        and isinstance(identity.get("id"), int)
        and not isinstance(identity.get("id"), bool)
        and identity["id"] > 0
        and isinstance(repository, dict)
        and is_nonempty_string(repository.get("full_name"))
        and isinstance(summary, dict)
        and is_nonempty_string(summary.get("repository"))
    )
    return schema, schema and repository["full_name"] == summary["repository"]


def github_security_features(values: Sequence[Any]) -> tuple[bool, bool]:
    repository = values[0]
    features = nested(repository, "security_and_analysis")
    names = (
        "advanced_security",
        "dependabot_security_updates",
        "secret_scanning",
        "secret_scanning_push_protection",
    )
    schema = isinstance(features, dict) and all(
        isinstance(features.get(name), dict)
        and isinstance(features[name].get("status"), str)
        for name in names
    )
    return schema, schema and all(
        features[name]["status"] == "enabled" for name in names
    )


def github_main_protection(values: Sequence[Any]) -> tuple[bool, bool]:
    protection = values[0]
    checks = nested(protection, "required_status_checks")
    reviews = nested(protection, "required_pull_request_reviews")
    contexts = checks.get("contexts", MISSING) if isinstance(checks, dict) else MISSING
    check_entries = (
        checks.get("checks", MISSING) if isinstance(checks, dict) else MISSING
    )
    contexts_schema = isinstance(contexts, list) and all(
        is_nonempty_string(context) for context in contexts
    )
    check_entries_schema = isinstance(check_entries, list) and all(
        isinstance(item, dict) and is_nonempty_string(item.get("context"))
        for item in check_entries
    )
    has_check_population = contexts_schema or check_entries_schema
    check_count = (len(contexts) if isinstance(contexts, list) else 0) + (
        len(check_entries) if isinstance(check_entries, list) else 0
    )
    required_booleans = (
        nested(protection, "enforce_admins", "enabled"),
        nested(protection, "required_conversation_resolution", "enabled"),
        nested(protection, "required_linear_history", "enabled"),
        nested(protection, "allow_force_pushes", "enabled"),
        nested(protection, "allow_deletions", "enabled"),
    )
    schema = (
        isinstance(protection, dict)
        and isinstance(checks, dict)
        and is_bool(checks.get("strict"))
        and has_check_population
        and isinstance(reviews, dict)
        and isinstance(reviews.get("required_approving_review_count"), int)
        and not isinstance(reviews.get("required_approving_review_count"), bool)
        and is_bool(reviews.get("dismiss_stale_reviews"))
        and is_bool(reviews.get("require_code_owner_reviews"))
        and all(is_bool(value) for value in required_booleans)
    )
    passed = schema and (
        checks["strict"]
        and check_count > 0
        and reviews["required_approving_review_count"] >= 1
        and reviews["dismiss_stale_reviews"]
        and reviews["require_code_owner_reviews"]
        and required_booleans[0]
        and required_booleans[1]
        and required_booleans[2]
        and not required_booleans[3]
        and not required_booleans[4]
    )
    return schema, passed


def github_production_environment(values: Sequence[Any]) -> tuple[bool, bool]:
    environment = values[0]
    rules = (
        environment.get("protection_rules", MISSING)
        if isinstance(environment, dict)
        else MISSING
    )
    policy = (
        environment.get("deployment_branch_policy", MISSING)
        if isinstance(environment, dict)
        else MISSING
    )
    reviewer_rules = (
        [
            rule
            for rule in rules
            if isinstance(rule, dict) and rule.get("type") == "required_reviewers"
        ]
        if isinstance(rules, list)
        else []
    )
    schema = (
        isinstance(rules, list)
        and isinstance(policy, dict)
        and is_bool(policy.get("protected_branches"))
        and is_bool(policy.get("custom_branch_policies"))
        and all(
            isinstance(rule.get("reviewers"), list)
            and is_bool(rule.get("prevent_self_review"))
            and all(
                isinstance(reviewer, dict)
                and is_nonempty_string(reviewer.get("type"))
                and isinstance(reviewer.get("reviewer"), dict)
                for reviewer in rule["reviewers"]
            )
            for rule in reviewer_rules
        )
    )
    passed = schema and (
        len(reviewer_rules) > 0
        and any(rule["reviewers"] for rule in reviewer_rules)
        and all(rule["prevent_self_review"] for rule in reviewer_rules)
        and policy["protected_branches"]
        and not policy["custom_branch_policies"]
    )
    return schema, passed


def github_actions_permissions(values: Sequence[Any]) -> tuple[bool, bool]:
    permissions = values[0]
    schema = (
        isinstance(permissions, dict)
        and isinstance(permissions.get("default_workflow_permissions"), str)
        and is_bool(permissions.get("can_approve_pull_request_reviews"))
    )
    return schema, bool(
        schema
        and permissions["default_workflow_permissions"] == "read"
        and not permissions["can_approve_pull_request_reviews"]
    )


def github_active_ruleset(values: Sequence[Any]) -> tuple[bool, bool]:
    rulesets = values[0]
    schema = isinstance(rulesets, list) and all(
        isinstance(rule, dict)
        and isinstance(rule.get("target"), str)
        and isinstance(rule.get("enforcement"), str)
        for rule in rulesets
    )
    passed = schema and any(
        rule["target"] == "branch" and rule["enforcement"] == "active"
        for rule in rulesets
    )
    return schema, passed


def github_collaborator_population(values: Sequence[Any]) -> tuple[bool, bool]:
    collaborators = values[0]
    schema = isinstance(collaborators, list) and all(
        isinstance(item, dict)
        and is_nonempty_string(item.get("login"))
        and isinstance(item.get("id"), int)
        and not isinstance(item.get("id"), bool)
        and item["id"] > 0
        and is_nonempty_string(item.get("role_name"))
        and isinstance(item.get("permissions"), dict)
        for item in collaborators
    )
    return schema, schema


def aws_identity(values: Sequence[Any]) -> tuple[bool, bool]:
    identity = values[0]
    schema = (
        isinstance(identity, dict)
        and isinstance(identity.get("Account"), str)
        and re.fullmatch(r"[0-9]{12}", identity["Account"]) is not None
        and is_nonempty_string(identity.get("Arn"))
        and identity["Arn"].startswith("arn:aws:")
        and is_nonempty_string(identity.get("UserId"))
    )
    return schema, schema


def aws_cloudtrail(values: Sequence[Any]) -> tuple[bool, bool]:
    trails, status, selectors = values
    trail_list = (
        trails.get("trailList", MISSING) if isinstance(trails, dict) else MISSING
    )
    event_selectors = (
        selectors.get("EventSelectors", MISSING)
        if isinstance(selectors, dict)
        else MISSING
    )
    advanced_selectors = (
        selectors.get("AdvancedEventSelectors", MISSING)
        if isinstance(selectors, dict)
        else MISSING
    )
    production_trails = (
        [
            trail
            for trail in trail_list
            if isinstance(trail, dict)
            and trail.get("Name") == "arbion-production-management"
        ]
        if isinstance(trail_list, list)
        else []
    )
    basic_schema = isinstance(event_selectors, list) and all(
        isinstance(selector, dict)
        and is_bool(selector.get("IncludeManagementEvents"))
        and isinstance(selector.get("ReadWriteType"), str)
        for selector in event_selectors
    )
    advanced_schema = isinstance(advanced_selectors, list) and all(
        isinstance(selector, dict)
        and isinstance(selector.get("FieldSelectors"), list)
        and all(
            isinstance(field, dict)
            and is_nonempty_string(field.get("Field"))
            and ("Equals" not in field or is_string_list(field.get("Equals")))
            for field in selector["FieldSelectors"]
        )
        for selector in advanced_selectors
    )
    selector_schema = (
        (event_selectors is MISSING or basic_schema)
        and (advanced_selectors is MISSING or advanced_schema)
        and (basic_schema or advanced_schema)
    )
    schema = (
        isinstance(trail_list, list)
        and all(
            isinstance(trail, dict) and is_nonempty_string(trail.get("Name"))
            for trail in trail_list
        )
        and isinstance(status, dict)
        and is_bool(status.get("IsLogging"))
        and selector_schema
        and len(production_trails) <= 1
        and all(
            is_bool(trail.get("IsMultiRegionTrail"))
            and is_bool(trail.get("IncludeGlobalServiceEvents"))
            and is_bool(trail.get("LogFileValidationEnabled"))
            and is_nonempty_string(trail.get("S3BucketName"))
            and is_nonempty_string(trail.get("CloudWatchLogsLogGroupArn"))
            for trail in production_trails
        )
    )
    protected_trail = (
        schema
        and len(production_trails) == 1
        and any(
            trail.get("IsMultiRegionTrail") is True
            and trail.get("IncludeGlobalServiceEvents") is True
            and trail.get("LogFileValidationEnabled") is True
            and is_nonempty_string(trail.get("S3BucketName"))
            and is_nonempty_string(trail.get("CloudWatchLogsLogGroupArn"))
            for trail in production_trails
        )
    )
    basic_management = isinstance(event_selectors, list) and any(
        isinstance(selector, dict)
        and selector.get("IncludeManagementEvents") is True
        and selector.get("ReadWriteType") == "All"
        for selector in event_selectors
    )
    advanced_management = isinstance(advanced_selectors, list) and any(
        isinstance(selector, dict)
        and any(
            isinstance(field, dict)
            and field.get("Field") == "eventCategory"
            and isinstance(field.get("Equals"), list)
            and "Management" in field["Equals"]
            for field in selector.get("FieldSelectors", [])
        )
        for selector in advanced_selectors
    )
    return schema, bool(
        schema
        and protected_trail
        and status["IsLogging"]
        and (basic_management or advanced_management)
    )


def aws_config_recording(values: Sequence[Any]) -> tuple[bool, bool]:
    recorders, statuses, channels = values
    recorder_list = (
        recorders.get("ConfigurationRecorders", MISSING)
        if isinstance(recorders, dict)
        else MISSING
    )
    status_list = (
        statuses.get("ConfigurationRecordersStatus", MISSING)
        if isinstance(statuses, dict)
        else MISSING
    )
    channel_list = (
        channels.get("DeliveryChannels", MISSING)
        if isinstance(channels, dict)
        else MISSING
    )
    schema = (
        isinstance(recorder_list, list)
        and isinstance(status_list, list)
        and isinstance(channel_list, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and isinstance(item.get("recordingGroup"), dict)
            and is_bool(nested(item, "recordingGroup", "allSupported"))
            and is_bool(nested(item, "recordingGroup", "includeGlobalResourceTypes"))
            for item in recorder_list
        )
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and is_bool(item.get("recording"))
            for item in status_list
        )
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and is_nonempty_string(item.get("s3BucketName"))
            for item in channel_list
        )
    )
    recorder_pass = schema and any(
        nested(item, "recordingGroup", "allSupported") is True
        and nested(item, "recordingGroup", "includeGlobalResourceTypes") is True
        for item in recorder_list
    )
    status_pass = schema and any(item.get("recording") is True for item in status_list)
    channel_pass = schema and bool(channel_list)
    return schema, bool(schema and recorder_pass and status_pass and channel_pass)


def aws_threat_detection(values: Sequence[Any]) -> tuple[bool, bool]:
    guardduty, analyzers = values
    detectors = (
        guardduty.get("DetectorIds", MISSING)
        if isinstance(guardduty, dict)
        else MISSING
    )
    statuses = (
        guardduty.get("Detectors", MISSING) if isinstance(guardduty, dict) else MISSING
    )
    analyzer_list = (
        analyzers.get("analyzers", MISSING) if isinstance(analyzers, dict) else MISSING
    )
    schema = (
        is_string_list(detectors)
        and len(set(detectors)) == len(detectors)
        and isinstance(statuses, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("DetectorId"))
            and item.get("Status") in ("ENABLED", "DISABLED")
            for item in statuses
        )
        and sorted(item["DetectorId"] for item in statuses) == sorted(detectors)
        and isinstance(analyzer_list, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and is_nonempty_string(item.get("status"))
            and is_nonempty_string(item.get("type"))
            for item in analyzer_list
        )
    )
    passed = (
        schema
        and bool(detectors)
        and all(item["Status"] == "ENABLED" for item in statuses)
        and any(
            item.get("status") == "ACTIVE" and item.get("type") == "ACCOUNT"
            for item in analyzer_list
        )
    )
    return schema, passed


def aws_security_routing(values: Sequence[Any]) -> tuple[bool, bool]:
    rule, targets, topic, alarms, identity, summary = values
    if not topic_identity_valid(topic, "SECURITY", identity, summary):
        return False, False
    subscriptions = topic["subscriptions"]
    topic_arn = topic["selection"]["topic_arn"]
    arn_prefix = f"arn:aws:events:{summary['aws_region']}:{identity['Account']}:rule/"
    target_list = (
        targets.get("Targets", MISSING) if isinstance(targets, dict) else MISSING
    )
    event_pattern = (
        rule.get("EventPattern", MISSING) if isinstance(rule, dict) else MISSING
    )
    parsed_pattern: Any = MISSING
    if isinstance(event_pattern, str):
        try:
            parsed_pattern = json.loads(
                event_pattern,
                object_pairs_hook=no_duplicate_object_pairs,
            )
        except (json.JSONDecodeError, ReviewError):
            parsed_pattern = MISSING
    detail_types = (
        parsed_pattern.get("detail-type", MISSING)
        if isinstance(parsed_pattern, dict)
        else MISSING
    )
    schema = (
        isinstance(rule, dict)
        and isinstance(rule.get("State"), str)
        and is_nonempty_string(rule.get("Name"))
        and is_nonempty_string(rule.get("Arn"))
        and is_string_list(detail_types)
        and is_string_list(nested(parsed_pattern, "source"))
        and isinstance(target_list, list)
        and targets.get("NextToken") is None
        and isinstance(subscriptions, list)
        and isinstance(alarms, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("Id"))
            and is_nonempty_string(item.get("Arn"))
            for item in target_list
        )
        and len({item["Id"] for item in target_list}) == len(target_list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("SubscriptionArn"))
            and is_nonempty_string(item.get("Protocol"))
            for item in subscriptions
        )
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("AlarmName"))
            and is_nonempty_string(item.get("AlarmArn"))
            and is_bool(item.get("ActionsEnabled"))
            and is_string_list(item.get("AlarmActions"))
            for item in alarms
        )
        and len({item["AlarmArn"] for item in alarms}) == len(alarms)
    )
    confirmed_subscription = schema and any(
        is_nonempty_string(item.get("SubscriptionArn"))
        and item["SubscriptionArn"] != "PendingConfirmation"
        for item in subscriptions
    )
    active_alarm = schema and any(
        item.get("ActionsEnabled") is True
        and item["AlarmArn"]
        == f"arn:aws:cloudwatch:{summary['aws_region']}:{identity['Account']}:alarm:{item['AlarmName']}"
        and topic_arn in item["AlarmActions"]
        for item in alarms
    )
    passed = schema and (
        rule["State"] == "ENABLED"
        and rule["Arn"] == arn_prefix + rule["Name"]
        and "aws.guardduty" in parsed_pattern["source"]
        and "GuardDuty Finding" in detail_types
        and any(item["Arn"] == topic_arn for item in target_list)
        and confirmed_subscription
        and active_alarm
    )
    return schema, passed


def topic_identity_valid(topic: Any, role: str, identity: Any, summary: Any) -> bool:
    account = nested(identity, "Account")
    region = nested(summary, "aws_region")
    arn = nested(identity, "Arn")
    return bool(
        isinstance(account, str)
        and re.fullmatch(r"[0-9]{12}", account)
        and isinstance(region, str)
        and isinstance(arn, str)
        and re.fullmatch(rf"arn:aws:(?:iam|sts)::{account}:.+", arn)
        and valid_topic(topic, role, account, region)
    )


def aws_operations_topic(values: Sequence[Any]) -> tuple[bool, bool]:
    topic, identity, summary = values
    schema = topic_identity_valid(topic, "OPERATIONS", identity, summary)
    return schema, schema and int(topic["attributes"]["SubscriptionsConfirmed"]) > 0


def aws_delivery_unverified(values: Sequence[Any]) -> tuple[bool, bool]:
    # Configuration reads cannot establish publication, receipt, or response.
    # Never upgrade this using a caller-supplied flag or subscription count.
    return False, False


def aws_bucket_protection(
    values: Sequence[Any], require_kms: bool
) -> tuple[bool, bool]:
    public, encryption, versioning, object_lock, lifecycle = values
    public_config = nested(public, "PublicAccessBlockConfiguration")
    encryption_rules = nested(encryption, "ServerSideEncryptionConfiguration", "Rules")
    lock_config = nested(object_lock, "ObjectLockConfiguration")
    lock_rule = nested(lock_config, "Rule", "DefaultRetention")
    lifecycle_rules = (
        lifecycle.get("Rules", MISSING) if isinstance(lifecycle, dict) else MISSING
    )
    schema = (
        isinstance(public_config, dict)
        and all(
            is_bool(public_config.get(key))
            for key in (
                "BlockPublicAcls",
                "IgnorePublicAcls",
                "BlockPublicPolicy",
                "RestrictPublicBuckets",
            )
        )
        and isinstance(encryption_rules, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(
                nested(item, "ApplyServerSideEncryptionByDefault", "SSEAlgorithm")
            )
            for item in encryption_rules
        )
        and isinstance(versioning, dict)
        and isinstance(versioning.get("Status"), str)
        and isinstance(lock_config, dict)
        and isinstance(lock_config.get("ObjectLockEnabled"), str)
        and isinstance(lock_rule, dict)
        and isinstance(lock_rule.get("Mode"), str)
        and isinstance(lock_rule.get("Days"), int)
        and not isinstance(lock_rule.get("Days"), bool)
        and isinstance(lifecycle_rules, list)
        and all(isinstance(item, dict) for item in lifecycle_rules)
    )
    algorithms = (
        [
            nested(item, "ApplyServerSideEncryptionByDefault", "SSEAlgorithm")
            for item in encryption_rules
        ]
        if schema
        else []
    )
    encryption_pass = (
        any(algorithm == "aws:kms" for algorithm in algorithms)
        if require_kms
        else any(algorithm in ("AES256", "aws:kms") for algorithm in algorithms)
    )
    lifecycle_pass = schema and any(
        item.get("Status") == "Enabled"
        and isinstance(nested(item, "Expiration", "Days"), int)
        and not isinstance(nested(item, "Expiration", "Days"), bool)
        and nested(item, "Expiration", "Days") >= 35
        for item in lifecycle_rules
    )
    passed = schema and (
        all(
            public_config[key]
            for key in (
                "BlockPublicAcls",
                "IgnorePublicAcls",
                "BlockPublicPolicy",
                "RestrictPublicBuckets",
            )
        )
        and encryption_pass
        and versioning["Status"] == "Enabled"
        and lock_config["ObjectLockEnabled"] == "Enabled"
        and lock_rule["Mode"] in ("GOVERNANCE", "COMPLIANCE")
        and lock_rule["Days"] >= 35
        and lifecycle_pass
    )
    return schema, passed


def audit_bucket(values: Sequence[Any]) -> tuple[bool, bool]:
    return aws_bucket_protection(values, require_kms=True)


def backup_bucket(values: Sequence[Any]) -> tuple[bool, bool]:
    return aws_bucket_protection(values, require_kms=False)


def aws_lightsail_monitoring(values: Sequence[Any]) -> tuple[bool, bool]:
    instances, alarms = values
    schema = (
        isinstance(instances, list)
        and isinstance(alarms, list)
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and is_nonempty_string(item.get("state"))
            and is_nonempty_string(item.get("arn"))
            for item in instances
        )
        and all(
            isinstance(item, dict)
            and is_nonempty_string(item.get("name"))
            and is_bool(item.get("notificationEnabled"))
            and is_string_list(item.get("contactProtocols"))
            and is_nonempty_string(nested(item, "monitoredResourceInfo", "arn"))
            and is_nonempty_string(nested(item, "monitoredResourceInfo", "name"))
            and nested(item, "monitoredResourceInfo", "resourceType") == "Instance"
            for item in alarms
        )
    )
    required = {
        "arbion-production-status-check-failed",
        "arbion-production-cpu-high",
        "arbion-production-burst-capacity-low",
    }
    production_instances = (
        [
            item
            for item in instances
            if item.get("name") == "arbion-production-host"
            and item.get("state") == "running"
        ]
        if schema
        else []
    )
    configured = (
        {
            item.get("name")
            for item in alarms
            if item.get("notificationEnabled") is True
            and "Email" in item["contactProtocols"]
            and len(production_instances) == 1
            and nested(item, "monitoredResourceInfo", "arn")
            == production_instances[0]["arn"]
            and nested(item, "monitoredResourceInfo", "name")
            == production_instances[0]["name"]
        }
        if schema
        else set()
    )
    passed = schema and len(production_instances) == 1 and required.issubset(configured)
    return schema, passed


ASSERTIONS = (
    Assertion(
        "GITHUB_IDENTITY_REPOSITORY_BOUND",
        "Collector identity is bound to the exact repository",
        "GITHUB",
        ("AC-02", "EVID-01"),
        (
            "github-identity.json",
            "github-repository.json",
            "collection-summary.json",
        ),
        github_identity_binding,
    ),
    Assertion(
        "GITHUB_SECURITY_FEATURES_ENABLED",
        "Repository security features are explicitly enabled",
        "GITHUB",
        ("SDLC-01", "SEC-01", "MON-01"),
        ("github-repository.json",),
        github_security_features,
    ),
    Assertion(
        "GITHUB_MAIN_CHANGE_PROTECTION",
        "Main branch has the documented review and history protections",
        "GITHUB",
        ("CHG-01",),
        ("github-main-protection.json",),
        github_main_protection,
    ),
    Assertion(
        "GITHUB_PRODUCTION_ENVIRONMENT_PROTECTED",
        "Production requires independent review from protected branches",
        "GITHUB",
        ("CHG-01",),
        ("github-production-environment.json",),
        github_production_environment,
    ),
    Assertion(
        "GITHUB_ACTIONS_DEFAULT_READ_ONLY",
        "GitHub Actions defaults to read-only and cannot approve reviews",
        "GITHUB",
        ("SEC-02", "CHG-01"),
        ("github-actions-permissions.json",),
        github_actions_permissions,
    ),
    Assertion(
        "GITHUB_ACTIVE_BRANCH_RULESET_PRESENT",
        "At least one active branch ruleset is saved",
        "GITHUB",
        ("CHG-01",),
        ("github-rulesets.json",),
        github_active_ruleset,
    ),
    Assertion(
        "GITHUB_COLLABORATOR_POPULATION_CAPTURED",
        "Repository collaborator population is structurally captured",
        "GITHUB",
        ("AC-02",),
        ("github-collaborators.json",),
        github_collaborator_population,
    ),
    Assertion(
        "AWS_COLLECTOR_IDENTITY_CAPTURED",
        "AWS collector account and principal identity are captured",
        "AWS",
        ("AC-02", "EVID-01"),
        ("aws-identity.json",),
        aws_identity,
    ),
    Assertion(
        "AWS_CLOUDTRAIL_MANAGEMENT_LOGGING",
        "CloudTrail management logging has the documented protections",
        "AWS",
        ("MON-01", "SEC-02"),
        (
            "aws-cloudtrail-trails.json",
            "aws-cloudtrail-status.json",
            "aws-cloudtrail-selectors.json",
        ),
        aws_cloudtrail,
    ),
    Assertion(
        "AWS_CONFIG_RECORDING_DELIVERY",
        "AWS Config recording and delivery are active",
        "AWS",
        ("MON-01",),
        (
            "aws-config-recorders.json",
            "aws-config-recorder-status.json",
            "aws-config-delivery-channels.json",
        ),
        aws_config_recording,
    ),
    Assertion(
        "AWS_THREAT_DETECTION_ACTIVE",
        "GuardDuty and account Access Analyzer are active",
        "AWS",
        ("MON-01", "SEC-02"),
        ("aws-guardduty-detectors.json", "aws-access-analyzers.json"),
        aws_threat_detection,
    ),
    Assertion(
        "AWS_SECURITY_EVENT_ROUTING",
        "Saved GuardDuty rule and alarm select the exact security topic (configuration only)",
        "AWS",
        ("MON-01", "IR-01"),
        (
            "aws-security-event-rule.json",
            "aws-security-event-targets.json",
            "aws-alarm-topic-subscriptions.json",
            "aws-cloudwatch-alarms.json",
            "aws-identity.json",
            "collection-summary.json",
        ),
        aws_security_routing,
    ),
    Assertion(
        "AWS_OPERATIONS_TOPIC_CONFIGURED",
        "Operational alert topic and confirmed subscription have exact account/region attribution",
        "AWS",
        ("MON-01", "IR-01"),
        (
            "aws-operations-alarm-topic.json",
            "aws-identity.json",
            "collection-summary.json",
        ),
        aws_operations_topic,
    ),
    Assertion(
        "AWS_ALERT_DELIVERY_TESTED",
        "End-to-end alert delivery requires a separately observed and reviewed receipt",
        "AWS",
        ("MON-01", "IR-01"),
        ("aws-alarm-topic-subscriptions.json", "aws-operations-alarm-topic.json"),
        aws_delivery_unverified,
    ),
    Assertion(
        "AWS_AUDIT_BUCKET_PROTECTED",
        "Audit storage has public-access, KMS, versioning, lock, and lifecycle protections",
        "AWS",
        ("EVID-01", "CONF-01"),
        (
            "aws-audit-bucket-public-access.json",
            "aws-audit-bucket-encryption.json",
            "aws-audit-bucket-versioning.json",
            "aws-audit-bucket-object-lock.json",
            "aws-audit-bucket-lifecycle.json",
        ),
        audit_bucket,
    ),
    Assertion(
        "AWS_BACKUP_BUCKET_PROTECTED",
        "Backup storage has public-access, encryption, versioning, lock, and lifecycle protections",
        "AWS",
        ("DR-01", "CONF-01"),
        (
            "aws-backup-bucket-public-access.json",
            "aws-backup-bucket-encryption.json",
            "aws-backup-bucket-versioning.json",
            "aws-backup-bucket-object-lock.json",
            "aws-backup-bucket-lifecycle.json",
        ),
        backup_bucket,
    ),
    Assertion(
        "AWS_LIGHTSAIL_MONITORING_CONFIGURED",
        "Production Lightsail service and external alarm population are configured",
        "AWS",
        ("AV-01", "MON-01"),
        ("aws-lightsail-instances.json", "aws-lightsail-alarms.json"),
        aws_lightsail_monitoring,
    ),
)


def evidence_rows(
    names: Iterable[str], sources: dict[str, Source]
) -> list[dict[str, str]]:
    return [
        {"file": name, "sha256": sources[name].digest} for name in sorted(set(names))
    ]


def result_follow_up(status: str) -> str:
    if status == "PASS":
        return (
            "Independent reviewer must confirm and retain the saved fields; "
            "this draft is not operating-effectiveness evidence."
        )
    if status == "FAIL":
        return (
            "Open a control exception and review or remediate the exact saved "
            "configuration before relying on this control."
        )
    return (
        "Recollect with authorized read-only access or resolve the unavailable "
        "saved fields; do not infer the setting."
    )


def evaluate_assertion(
    assertion: Assertion, sources: dict[str, Source]
) -> dict[str, Any]:
    coarse_name = "github.json" if assertion.system == "GITHUB" else "aws.json"
    if coarse_name in sources:
        selected_names = [coarse_name]
        status = "UNAVAILABLE"
    else:
        selected_names = [name for name in assertion.files if name in sources]
        selected = [sources[name] for name in selected_names]
        if len(selected_names) != len(assertion.files) or any(
            source.unavailable for source in selected
        ):
            status = "UNAVAILABLE"
        else:
            schema_available, passed = assertion.evaluate(
                [source.data for source in selected]
            )
            if not schema_available:
                status = "UNAVAILABLE"
            else:
                status = "PASS" if passed else "FAIL"
    follow_up = result_follow_up(status)
    if assertion.assertion_id == "AWS_ALERT_DELIVERY_TESTED":
        follow_up = "Arrange a separately authorized end-to-end delivery exercise and retain a reviewed receipt. This collector does not publish notifications or test delivery."
    return {
        "assertion_id": assertion.assertion_id,
        "title": assertion.title,
        "system": assertion.system,
        "control_ids": sorted(assertion.controls),
        "status": status,
        "evidence": evidence_rows(selected_names, sources),
        "safe_follow_up": follow_up,
    }


def verify_snapshot(snapshot: Path, verifier: Path) -> None:
    try:
        completed = subprocess.run(
            [str(verifier), str(snapshot)],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
        )
    except OSError as exc:
        raise ReviewError("snapshot verifier could not be executed") from exc
    if completed.returncode != 0:
        classification = (
            completed.stderr.strip().splitlines()[-1]
            if completed.stderr.strip()
            else "snapshot verifier rejected input"
        )
        raise ReviewError(classification)


def read_verified_snapshot(
    snapshot: Path, verifier: Path
) -> tuple[dict[str, Source], str]:
    verify_snapshot(snapshot, verifier)

    manifest_path = snapshot / "SHA256SUMS"
    manifest_raw = read_regular_bytes(
        manifest_path,
        MAXIMUM_MANIFEST_BYTES,
        "checksum manifest",
    )
    manifest_sha256 = hashlib.sha256(manifest_raw).hexdigest()
    if SHA256.fullmatch(manifest_sha256) is None:
        raise ReviewError("checksum manifest digest is invalid")
    manifest: dict[str, str] = {}
    try:
        manifest_lines = manifest_raw.decode("ascii").splitlines()
    except UnicodeDecodeError as exc:
        raise ReviewError("checksum manifest changed after verification") from exc
    for line in manifest_lines:
        match = re.fullmatch(
            r"([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._-]*\.json)", line
        )
        if not match or match.group(2) in manifest:
            raise ReviewError("checksum manifest changed after verification")
        manifest[match.group(2)] = match.group(1)

    github_names = (
        {"github.json"}
        if "github.json" in manifest
        else {
            name
            for assertion in ASSERTIONS
            if assertion.system == "GITHUB"
            for name in assertion.files
        }
    )
    aws_names = (
        {"aws.json"}
        if "aws.json" in manifest
        else {
            name
            for assertion in ASSERTIONS
            if assertion.system == "AWS"
            for name in assertion.files
        }
    )
    expected_names = {"collection-summary.json", *github_names, *aws_names}
    # The shell verifier enforces the exact versioned inventory. Legacy 1.0
    # packages remain immutable; their new topic assertions stay UNAVAILABLE.
    if set(manifest) == expected_names - {"aws-operations-alarm-topic.json"}:
        expected_names.discard("aws-operations-alarm-topic.json")
    if set(manifest) != expected_names:
        raise ReviewError("source inventory changed after verification")

    sources: dict[str, Source] = {}
    for name, claimed_digest in manifest.items():
        path = snapshot / name
        raw = read_regular_bytes(path, MAXIMUM_SOURCE_BYTES, f"evidence source {name}")
        digest = hashlib.sha256(raw).hexdigest()
        if digest != claimed_digest:
            raise ReviewError(f"source checksum changed after verification: {name}")
        sources[name] = Source(
            name=name, digest=digest, data=parse_json_bytes(raw, name)
        )

    if (
        read_regular_bytes(
            manifest_path,
            MAXIMUM_MANIFEST_BYTES,
            "checksum manifest",
        )
        != manifest_raw
    ):
        raise ReviewError("checksum manifest changed while sources were read")
    actual_names = {
        path.name for path in snapshot.iterdir() if path.name != "SHA256SUMS"
    }
    if actual_names != expected_names:
        raise ReviewError("source inventory changed while sources were read")
    for name, source in sources.items():
        path = snapshot / name
        raw = read_regular_bytes(path, MAXIMUM_SOURCE_BYTES, f"evidence source {name}")
        if hashlib.sha256(raw).hexdigest() != source.digest:
            raise ReviewError(f"source changed while snapshot was read: {name}")
    verify_snapshot(snapshot, verifier)
    if (
        read_regular_bytes(
            manifest_path,
            MAXIMUM_MANIFEST_BYTES,
            "checksum manifest",
        )
        != manifest_raw
    ):
        raise ReviewError("checksum manifest changed during final verification")
    return sources, manifest_sha256


def build_report(snapshot: Path, verifier: Path) -> dict[str, Any]:
    sources, manifest_sha256 = read_verified_snapshot(snapshot, verifier)
    summary_source = sources.get("collection-summary.json")
    if summary_source is None or not isinstance(summary_source.data, dict):
        raise ReviewError("verified collection summary is unavailable")
    summary = summary_source.data
    repository_source = sources.get("github-repository.json")
    if repository_source and not repository_source.unavailable:
        full_name = nested(repository_source.data, "full_name")
        if isinstance(full_name, str) and full_name != summary.get("repository"):
            raise ReviewError(
                "repository identity conflicts with the collection summary"
            )

    results = [evaluate_assertion(assertion, sources) for assertion in ASSERTIONS]
    counts = {
        status: sum(result["status"] == status for result in results)
        for status in ("PASS", "FAIL", "UNAVAILABLE")
    }
    if counts["FAIL"]:
        review_state = "REVIEW_REQUIRED"
    elif counts["UNAVAILABLE"]:
        review_state = "INCOMPLETE_REVIEW_REQUIRED"
    else:
        review_state = "DRAFT_PASS_REVIEW_REQUIRED"

    inventory = []
    for name in sorted(sources):
        source = sources[name]
        inventory.append(
            {
                "file": name,
                "sha256": source.digest,
                "status": "UNAVAILABLE" if source.unavailable else "AVAILABLE",
            }
        )

    report: dict[str, Any] = {
        "schema_version": "1.0",
        "artifact_status": "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE",
        "review_state": review_state,
        "review_id": f"arbion-soc2-review-{manifest_sha256}",
        "collection_id": summary["collection_id"],
        "collected_at": summary["collected_at"],
        "snapshot_status": summary["status"],
        "repository": summary["repository"],
        "aws_region": summary["aws_region"],
        "source_manifest_sha256": manifest_sha256,
        "unavailable_sources": sorted(summary["unavailable_sources"]),
        "source_inventory": inventory,
        "summary": {
            "assertion_count": len(results),
            **{status.lower(): count for status, count in counts.items()},
        },
        "results": results,
        "limitations": [
            "Deterministic draft derived only from the verified saved snapshot",
            "PASS means the narrowly stated saved-field condition matched; "
            "it is not a control-effectiveness conclusion",
            "Missing or unexpected fields remain UNAVAILABLE without inference",
            "Requires authorized independent review, exception handling, "
            "and immutable external retention",
            "Does not establish operating effectiveness, SOC 2 compliance, or certification",
        ],
    }
    canonical_payload = (
        json.dumps(
            report,
            sort_keys=True,
            separators=(",", ":"),
            ensure_ascii=False,
        ).encode("utf-8")
        + b"\n"
    )
    report["integrity"] = {
        "algorithm": "SHA-256",
        "canonicalization": "PYTHON_JSON_SORTED_COMPACT_UTF8_V1",
        "covers": "ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY",
        "payload_sha256": hashlib.sha256(canonical_payload).hexdigest(),
    }
    return report


def write_report(report: dict[str, Any], output: Path) -> None:
    rendered = (
        json.dumps(report, sort_keys=True, indent=2, ensure_ascii=False) + "\n"
    ).encode("utf-8")
    if len(rendered) > MAXIMUM_REPORT_BYTES:
        raise ReviewError("review draft exceeds its size limit")
    flags = (
        os.O_WRONLY
        | os.O_CREAT
        | os.O_EXCL
        | getattr(os, "O_CLOEXEC", 0)
        | getattr(os, "O_NOFOLLOW", 0)
    )
    try:
        descriptor = os.open(output, flags, 0o600)
    except OSError as exc:
        raise ReviewError("review draft output could not be created safely") from exc
    try:
        os.fchmod(descriptor, 0o600)
        handle = os.fdopen(descriptor, "wb")
        descriptor = -1
        with handle:
            handle.write(rendered)
            handle.flush()
            os.fsync(handle.fileno())
    except BaseException as exc:
        if descriptor >= 0:
            try:
                os.close(descriptor)
            except OSError:
                pass
        try:
            output.unlink(missing_ok=True)
        except OSError:
            pass
        if isinstance(exc, OSError):
            raise ReviewError(
                "review draft output could not be written safely"
            ) from exc
        raise


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("snapshot", type=Path)
    parser.add_argument("output", type=Path)
    return parser.parse_args(argv)


def main(argv: Sequence[str]) -> int:
    args = parse_args(argv)
    script_path = Path(__file__).resolve()
    repository_root = script_path.parent.parent
    verifier = script_path.parent / "verify-soc2-evidence-snapshot.sh"
    if args.snapshot.is_symlink():
        raise ReviewError("snapshot directory must not be a symlink")
    snapshot = args.snapshot.resolve()
    if not snapshot.is_dir():
        raise ReviewError("snapshot directory is unavailable")
    output_parent = args.output.parent.resolve()
    output = output_parent / args.output.name
    if not output_parent.is_dir():
        raise ReviewError("output parent must already exist")
    if not SAFE_OUTPUT_NAME.fullmatch(output.name):
        raise ReviewError("output file name is unsafe or noncanonical")
    if output.exists() or output.is_symlink():
        raise ReviewError("output file already exists")
    if path_is_within(output, repository_root):
        raise ReviewError("review draft must remain outside the Git repository")
    if path_is_within(output, snapshot):
        raise ReviewError("review draft must not modify the verified snapshot")

    report = build_report(snapshot, verifier)
    write_report(report, output)
    print(f"SOC 2 external-control review draft created: {output}")
    print(
        "Status: REVIEW_DRAFT_NOT_OPERATING_EVIDENCE; "
        "authorized independent review remains required."
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except ReviewError as exc:
        print(f"SOC 2 external-control review failed safely: {exc}", file=sys.stderr)
        raise SystemExit(1)
