#!/usr/bin/env python3
"""Regression tests for the deterministic external SOC 2 review draft."""

from __future__ import annotations

import hashlib
import json
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path
from typing import Any

from soc2_alert_evidence import DELIVERY, valid_topic
from soc2_github_security_evidence import (
    API_VERSION,
    CATEGORIES,
    MAX_ANALYSES,
    PAGE_SIZE,
)


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
REVIEWER = REPOSITORY_ROOT / "scripts" / "review-soc2-external-evidence.py"
COLLECTION_ID = "arbion-soc2-20260909T160000Z"
COLLECTED_AT = "2026-09-09T16:00:00Z"

GITHUB_NAMES = {
    "github-code-scanning",
    "github-vulnerability-alerts",
    "github-actions-permissions",
    "github-collaborators",
    "github-identity",
    "github-main-protection",
    "github-production-environment",
    "github-repository",
    "github-rulesets",
}

AWS_NAMES = {
    "aws-access-analyzers",
    "aws-alarm-topic-subscriptions",
    "aws-operations-alarm-topic",
    "aws-audit-bucket-encryption",
    "aws-audit-bucket-lifecycle",
    "aws-audit-bucket-object-lock",
    "aws-audit-bucket-public-access",
    "aws-audit-bucket-versioning",
    "aws-backup-bucket-encryption",
    "aws-backup-bucket-lifecycle",
    "aws-backup-bucket-object-lock",
    "aws-backup-bucket-public-access",
    "aws-backup-bucket-versioning",
    "aws-cloudtrail-selectors",
    "aws-cloudtrail-status",
    "aws-cloudtrail-trails",
    "aws-cloudwatch-alarms",
    "aws-config-delivery-channels",
    "aws-config-recorder-status",
    "aws-config-recorders",
    "aws-guardduty-detectors",
    "aws-identity",
    "aws-lightsail-alarms",
    "aws-lightsail-instances",
    "aws-security-event-rule",
    "aws-security-event-targets",
}


def write_json(path: Path, value: Any) -> None:
    path.write_text(
        json.dumps(value, sort_keys=True, indent=2) + "\n",
        encoding="utf-8",
    )


def seal_snapshot(snapshot: Path) -> dict[str, str]:
    manifest: dict[str, str] = {}
    for path in sorted(snapshot.glob("*.json"), key=lambda item: item.name):
        manifest[path.name] = hashlib.sha256(path.read_bytes()).hexdigest()
    rendered = "".join(f"{digest}  {name}\n" for name, digest in manifest.items())
    (snapshot / "SHA256SUMS").write_text(rendered, encoding="ascii")
    return manifest


def bucket_evidence(snapshot: Path, role: str, algorithm: str) -> None:
    prefix = f"aws-{role}-bucket"
    write_json(
        snapshot / f"{prefix}-public-access.json",
        {
            "PublicAccessBlockConfiguration": {
                "BlockPublicAcls": True,
                "IgnorePublicAcls": True,
                "BlockPublicPolicy": True,
                "RestrictPublicBuckets": True,
            }
        },
    )
    write_json(
        snapshot / f"{prefix}-encryption.json",
        {
            "ServerSideEncryptionConfiguration": {
                "Rules": [
                    {"ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": algorithm}}
                ]
            }
        },
    )
    write_json(snapshot / f"{prefix}-versioning.json", {"Status": "Enabled"})
    write_json(
        snapshot / f"{prefix}-object-lock.json",
        {
            "ObjectLockConfiguration": {
                "ObjectLockEnabled": "Enabled",
                "Rule": {"DefaultRetention": {"Mode": "GOVERNANCE", "Days": 35}},
            },
        },
    )
    write_json(
        snapshot / f"{prefix}-lifecycle.json",
        {
            "Rules": [
                {"ID": "retention", "Status": "Enabled", "Expiration": {"Days": 45}}
            ]
        },
    )


def make_complete_snapshot(snapshot: Path) -> dict[str, str]:
    snapshot.mkdir(mode=0o700, parents=True)
    write_json(
        snapshot / "collection-summary.json",
        {
            "schema_version": "1.2",
            "collection_id": COLLECTION_ID,
            "collected_at": COLLECTED_AT,
            "status": "COMPLETE_REVIEW_REQUIRED",
            "repository": "example/arbion",
            "aws_region": "us-east-1",
            "unavailable_sources": [],
            "limitations": [
                "Read-only point-in-time configuration snapshot",
                "Requires reviewer evaluation",
                "Does not establish SOC 2 certification",
            ],
        },
    )

    write_json(snapshot / "github-identity.json", {"login": "reviewer", "id": 1})
    write_json(
        snapshot / "github-repository.json",
        {
            "id": 123,
            "full_name": "example/arbion",
            "default_branch": "main",
            "archived": False,
            "security_and_analysis": {
                "advanced_security": {"status": "enabled"},
                "dependabot_security_updates": {"status": "enabled"},
                "secret_scanning": {"status": "enabled"},
                "secret_scanning_push_protection": {"status": "enabled"},
            },
        },
    )
    security_common = {
        "schema_version": "1.0",
        "repository": {"id": 123, "full_name": "example/arbion"},
        "collected_at": COLLECTED_AT,
        "method": "GET",
        "api_version": API_VERSION,
    }
    write_json(
        snapshot / "github-vulnerability-alerts.json",
        {
            **security_common,
            "endpoint": "repos/example/arbion/vulnerability-alerts",
            "http_status": 204,
        },
    )
    head = {
        "ref": "refs/heads/main",
        "url": "https://api.github.com/repos/example/arbion/git/refs/heads/main",
        "object": {"type": "commit", "sha": "a" * 40},
    }
    write_json(
        snapshot / "github-code-scanning.json",
        {
            **security_common,
            "endpoint": "repos/example/arbion/code-scanning/analyses",
            "scope": {
                "ref": "refs/heads/main",
                "tool": "CodeQL",
                "categories": CATEGORIES,
                "maximum_analyses": MAX_ANALYSES,
                "page_size": PAGE_SIZE,
            },
            "head_before": head,
            "head_after": head,
            "page_counts": [4],
            "analyses": [
                {
                    "id": index + 1,
                    "ref": "refs/heads/main",
                    "commit_sha": "a" * 40,
                    "url": f"https://api.github.com/repos/example/arbion/code-scanning/analyses/{index + 1}",
                    "category": category,
                    "analysis_key": ".github/workflows/security.yml:codeql",
                    "created_at": "2026-09-09T15:00:00Z",
                    "tool": {"name": "CodeQL", "version": "2.27.0"},
                    "rules_count": 20,
                    "error_state": "NONE",
                    "warning_state": "NONE",
                }
                for index, category in enumerate(CATEGORIES)
            ],
        },
    )
    write_json(
        snapshot / "github-main-protection.json",
        {
            "required_status_checks": {"strict": True, "contexts": ["CI"]},
            "required_pull_request_reviews": {
                "required_approving_review_count": 1,
                "dismiss_stale_reviews": True,
                "require_code_owner_reviews": True,
            },
            "enforce_admins": {"enabled": True},
            "required_conversation_resolution": {"enabled": True},
            "required_linear_history": {"enabled": True},
            "allow_force_pushes": {"enabled": False},
            "allow_deletions": {"enabled": False},
        },
    )
    write_json(
        snapshot / "github-production-environment.json",
        {
            "protection_rules": [
                {
                    "type": "required_reviewers",
                    "prevent_self_review": True,
                    "reviewers": [{"type": "User", "reviewer": {"login": "approver"}}],
                }
            ],
            "deployment_branch_policy": {
                "protected_branches": True,
                "custom_branch_policies": False,
            },
        },
    )
    write_json(
        snapshot / "github-actions-permissions.json",
        {
            "default_workflow_permissions": "read",
            "can_approve_pull_request_reviews": False,
        },
    )
    write_json(
        snapshot / "github-rulesets.json",
        [{"id": 1, "target": "branch", "enforcement": "active"}],
    )
    write_json(
        snapshot / "github-collaborators.json",
        [
            {
                "login": "reviewer",
                "id": 1,
                "role_name": "admin",
                "permissions": {"admin": True},
            }
        ],
    )

    write_json(
        snapshot / "aws-identity.json",
        {
            "Account": "111122223333",
            "Arn": "arn:aws:iam::111122223333:role/read-only",
            "UserId": "AROATEST",
        },
    )
    write_json(
        snapshot / "aws-cloudtrail-trails.json",
        {
            "trailList": [
                {
                    "Name": "arbion-production-management",
                    "IsMultiRegionTrail": True,
                    "IncludeGlobalServiceEvents": True,
                    "LogFileValidationEnabled": True,
                    "S3BucketName": "arbion-audit",
                    "CloudWatchLogsLogGroupArn": (
                        "arn:aws:logs:us-east-1:111122223333:log-group:audit"
                    ),
                }
            ]
        },
    )
    write_json(snapshot / "aws-cloudtrail-status.json", {"IsLogging": True})
    write_json(
        snapshot / "aws-cloudtrail-selectors.json",
        {"EventSelectors": [{"IncludeManagementEvents": True, "ReadWriteType": "All"}]},
    )
    write_json(
        snapshot / "aws-config-recorders.json",
        {
            "ConfigurationRecorders": [
                {
                    "name": "arbion-production",
                    "recordingGroup": {
                        "allSupported": True,
                        "includeGlobalResourceTypes": True,
                    },
                }
            ]
        },
    )
    write_json(
        snapshot / "aws-config-recorder-status.json",
        {
            "ConfigurationRecordersStatus": [
                {"name": "arbion-production", "recording": True}
            ]
        },
    )
    write_json(
        snapshot / "aws-config-delivery-channels.json",
        {
            "DeliveryChannels": [
                {"name": "arbion-production", "s3BucketName": "arbion-audit"}
            ]
        },
    )
    write_json(
        snapshot / "aws-guardduty-detectors.json",
        {
            "DetectorIds": ["a" * 32],
            "Detectors": [{"DetectorId": "a" * 32, "Status": "ENABLED"}],
        },
    )
    write_json(
        snapshot / "aws-access-analyzers.json",
        {
            "analyzers": [
                {"name": "arbion-production", "status": "ACTIVE", "type": "ACCOUNT"}
            ]
        },
    )
    write_json(
        snapshot / "aws-security-event-rule.json",
        {
            "Name": "arbion-production-guardduty-findings",
            "Arn": "arn:aws:events:us-east-1:111122223333:rule/arbion-production-guardduty-findings",
            "State": "ENABLED",
            "EventPattern": json.dumps(
                {"source": ["aws.guardduty"], "detail-type": ["GuardDuty Finding"]},
                sort_keys=True,
            ),
        },
    )
    write_json(
        snapshot / "aws-security-event-targets.json",
        {
            "Targets": [
                {"Id": "security", "Arn": "arn:aws:sns:us-east-1:111122223333:security"}
            ]
        },
    )
    for role, name, topic, other in (
        ("SECURITY", "aws-alarm-topic-subscriptions", "security", "ops"),
        ("OPERATIONS", "aws-operations-alarm-topic", "ops", "security"),
    ):
        arn = f"arn:aws:sns:us-east-1:111122223333:{topic}"
        write_json(
            snapshot / f"{name}.json",
            {
                "schema_version": "1.0",
                "role": role,
                "selection": {
                    "source": "EXPLICIT_ARN",
                    "topic_arn": arn,
                    "other_role_topic_arn": f"arn:aws:sns:us-east-1:111122223333:{other}",
                },
                "attributes": {
                    "TopicArn": arn,
                    "Owner": "111122223333",
                    "SubscriptionsConfirmed": "1",
                    "SubscriptionsPending": "0",
                    "KmsMasterKeyId": None,
                },
                "subscriptions": [
                    {
                        "SubscriptionArn": arn
                        + ":00000000-0000-0000-0000-000000000001",
                        "TopicArn": arn,
                        "Owner": "111122223333",
                        "Protocol": "email",
                    }
                ],
                "delivery_evidence": DELIVERY,
            },
        )
    write_json(
        snapshot / "aws-cloudwatch-alarms.json",
        [
            {
                "AlarmName": "arbion-production-security",
                "AlarmArn": "arn:aws:cloudwatch:us-east-1:111122223333:alarm:arbion-production-security",
                "ActionsEnabled": True,
                "AlarmActions": ["arn:aws:sns:us-east-1:111122223333:security"],
            }
        ],
    )
    bucket_evidence(snapshot, "audit", "aws:kms")
    bucket_evidence(snapshot, "backup", "AES256")
    write_json(
        snapshot / "aws-lightsail-instances.json",
        [
            {
                "name": "arbion-production-host",
                "state": "running",
                "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/test",
            }
        ],
    )
    write_json(
        snapshot / "aws-lightsail-alarms.json",
        [
            {
                "name": name,
                "notificationEnabled": True,
                "contactProtocols": ["Email"],
                "monitoredResourceInfo": {
                    "name": "arbion-production-host",
                    "resourceType": "Instance",
                    "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/test",
                },
            }
            for name in (
                "arbion-production-status-check-failed",
                "arbion-production-cpu-high",
                "arbion-production-burst-capacity-low",
            )
        ],
    )

    actual = {
        path.stem
        for path in snapshot.glob("*.json")
        if path.name != "collection-summary.json"
    }
    expected = GITHUB_NAMES | AWS_NAMES
    if actual != expected:
        raise AssertionError(
            f"fixture inventory differs: missing={expected - actual}, "
            f"extra={actual - expected}"
        )
    return seal_snapshot(snapshot)


class ExternalReviewTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory(prefix="arbion-soc2-review-test.")
        self.root = Path(self.temporary.name)
        self.snapshot = self.root / "snapshot" / COLLECTION_ID
        self.manifest = make_complete_snapshot(self.snapshot)

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def run_review(
        self,
        output_name: str = "review.json",
    ) -> tuple[subprocess.CompletedProcess[str], Path]:
        output = self.root / output_name
        completed = subprocess.run(
            [str(REVIEWER), str(self.snapshot), str(output)],
            check=False,
            capture_output=True,
            text=True,
        )
        return completed, output

    @staticmethod
    def assertion(report: dict[str, Any], assertion_id: str) -> dict[str, Any]:
        return next(
            item for item in report["results"] if item["assertion_id"] == assertion_id
        )

    def test_complete_snapshot_produces_deterministic_draft_pass(self) -> None:
        first, first_path = self.run_review("first.json")
        second, second_path = self.run_review("second.json")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(first_path.read_bytes(), second_path.read_bytes())
        self.assertEqual(stat.S_IMODE(first_path.stat().st_mode), 0o600)
        rendered = first_path.read_text(encoding="utf-8")
        self.assertNotIn("111122223333", rendered)
        self.assertNotIn("arn:aws:", rendered)
        self.assertNotIn('"login": "reviewer"', rendered)
        self.assertNotIn("approver", rendered)

        report = json.loads(rendered)
        self.assertEqual(
            report["artifact_status"], "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE"
        )
        self.assertEqual(report["review_state"], "INCOMPLETE_REVIEW_REQUIRED")
        self.assertEqual(
            report["summary"],
            {"assertion_count": 22, "pass": 21, "fail": 0, "unavailable": 1},
        )
        self.assertEqual(len(report["source_inventory"]), 36)
        report_inventory = {
            item["file"]: item["sha256"] for item in report["source_inventory"]
        }
        self.assertEqual(report_inventory, self.manifest)
        self.assertTrue(all(result["control_ids"] for result in report["results"]))

        claimed = report.pop("integrity")["payload_sha256"]
        canonical = (
            json.dumps(
                report,
                sort_keys=True,
                separators=(",", ":"),
                ensure_ascii=False,
            ).encode("utf-8")
            + b"\n"
        )
        self.assertEqual(hashlib.sha256(canonical).hexdigest(), claimed)

    def test_explicit_configuration_mismatch_is_fail(self) -> None:
        permissions = self.snapshot / "github-actions-permissions.json"
        value = json.loads(permissions.read_text(encoding="utf-8"))
        value["default_workflow_permissions"] = "write"
        write_json(permissions, value)
        seal_snapshot(self.snapshot)

        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text(encoding="utf-8"))
        result = self.assertion(report, "GITHUB_ACTIONS_DEFAULT_READ_ONLY")
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(report["review_state"], "REVIEW_REQUIRED")

    def test_missing_advanced_security_does_not_erase_explicit_features(self) -> None:
        path = self.snapshot / "github-repository.json"
        value = json.loads(path.read_text())
        del value["security_and_analysis"]["advanced_security"]
        write_json(path, value)
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        self.assertEqual(
            self.assertion(report, "GITHUB_SECURITY_FEATURES_ENABLED")["status"],
            "UNAVAILABLE",
        )
        for name in (
            "GITHUB_DEPENDABOT_UPDATES_ENABLED",
            "GITHUB_SECRET_SCANNING_ENABLED",
            "GITHUB_PUSH_PROTECTION_ENABLED",
            "GITHUB_VULNERABILITY_ALERTS_ENABLED",
            "GITHUB_CURRENT_MAIN_CODEQL_ANALYSES",
        ):
            self.assertEqual(self.assertion(report, name)["status"], "PASS")

    def test_individual_feature_disabled_or_missing_is_not_enabled(self) -> None:
        path = self.snapshot / "github-repository.json"
        original = json.loads(path.read_text())
        for index, (status, expected) in enumerate(
            (("disabled", "FAIL"), (None, "UNAVAILABLE"), ("unknown", "UNAVAILABLE"))
        ):
            value = json.loads(json.dumps(original))
            value["security_and_analysis"]["secret_scanning"]["status"] = status
            write_json(path, value)
            seal_snapshot(self.snapshot)
            completed, output = self.run_review(f"feature-{index}.json")
            self.assertEqual(completed.returncode, 0, completed.stderr)
            report = json.loads(output.read_text())
            self.assertEqual(
                self.assertion(report, "GITHUB_SECRET_SCANNING_ENABLED")["status"],
                expected,
            )
            self.assertEqual(
                self.assertion(report, "GITHUB_PUSH_PROTECTION_ENABLED")["status"],
                "PASS",
            )

    def test_legacy_11_package_has_no_inferred_scanning_or_alert_evidence(self) -> None:
        path = self.snapshot / "collection-summary.json"
        value = json.loads(path.read_text())
        value["schema_version"] = "1.1"
        write_json(path, value)
        for name in ("github-code-scanning.json", "github-vulnerability-alerts.json"):
            (self.snapshot / name).unlink()
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        for name in (
            "GITHUB_CURRENT_MAIN_CODEQL_ANALYSES",
            "GITHUB_VULNERABILITY_ALERTS_ENABLED",
        ):
            self.assertEqual(self.assertion(report, name)["status"], "UNAVAILABLE")

    def test_new_package_requires_exact_github_security_inventory(self) -> None:
        (self.snapshot / "github-code-scanning.json").unlink()
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertFalse(output.exists())

    def test_security_routing_cannot_borrow_unrelated_targets_or_alarms(self) -> None:
        variants = [
            (
                "aws-security-event-targets.json",
                lambda v: v["Targets"][0].update(
                    Arn="arn:aws:sns:us-east-1:111122223333:ops"
                ),
                "FAIL",
            ),
            (
                "aws-cloudwatch-alarms.json",
                lambda v: v[0].update(
                    AlarmActions=["arn:aws:sns:us-east-1:111122223333:ops"]
                ),
                "FAIL",
            ),
            (
                "aws-cloudwatch-alarms.json",
                lambda v: v[0].update(
                    AlarmArn="arn:aws:cloudwatch:us-west-2:111122223333:alarm:arbion-production-security"
                ),
                "FAIL",
            ),
            (
                "aws-security-event-rule.json",
                lambda v: v.update(
                    Arn="arn:aws:events:us-east-1:999922223333:rule/arbion-production-guardduty-findings"
                ),
                "FAIL",
            ),
            (
                "aws-security-event-rule.json",
                lambda v: v.update(
                    EventPattern=json.dumps(
                        {"source": ["aws.ec2"], "detail-type": ["GuardDuty Finding"]}
                    )
                ),
                "FAIL",
            ),
            (
                "aws-security-event-targets.json",
                lambda v: v.update(NextToken="capped"),
                "UNAVAILABLE",
            ),
        ]
        for index, (name, mutate, expected) in enumerate(variants):
            with self.subTest(name=name, index=index):
                path = self.snapshot / name
                original = json.loads(path.read_text())
                changed = json.loads(json.dumps(original))
                mutate(changed)
                write_json(path, changed)
                seal_snapshot(self.snapshot)
                completed, output = self.run_review(f"route-{index}.json")
                self.assertEqual(completed.returncode, 0, completed.stderr)
                report = json.loads(output.read_text())
                self.assertEqual(
                    self.assertion(report, "AWS_SECURITY_EVENT_ROUTING")["status"],
                    expected,
                )
                self.assertEqual(
                    self.assertion(report, "AWS_ALERT_DELIVERY_TESTED")["status"],
                    "UNAVAILABLE",
                )
                write_json(path, original)

    def test_topic_must_match_account_and_region_of_collection(self) -> None:
        path = self.snapshot / "collection-summary.json"
        value = json.loads(path.read_text())
        value["aws_region"] = "us-west-2"
        write_json(path, value)
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        for name in ("AWS_SECURITY_EVENT_ROUTING", "AWS_OPERATIONS_TOPIC_CONFIGURED"):
            self.assertEqual(self.assertion(report, name)["status"], "UNAVAILABLE")

    def test_legacy_package_remains_reviewable_without_invented_topic_binding(
        self,
    ) -> None:
        path = self.snapshot / "collection-summary.json"
        summary = json.loads(path.read_text())
        summary["schema_version"] = "1.0"
        write_json(path, summary)
        (self.snapshot / "aws-operations-alarm-topic.json").unlink()
        (self.snapshot / "github-code-scanning.json").unlink()
        (self.snapshot / "github-vulnerability-alerts.json").unlink()
        write_json(
            self.snapshot / "aws-alarm-topic-subscriptions.json",
            [
                {
                    "SubscriptionArn": "arn:aws:sns:us-east-1:111122223333:ops:legacy",
                    "Protocol": "email",
                }
            ],
        )
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        for name in (
            "AWS_SECURITY_EVENT_ROUTING",
            "AWS_OPERATIONS_TOPIC_CONFIGURED",
            "AWS_ALERT_DELIVERY_TESTED",
        ):
            self.assertEqual(self.assertion(report, name)["status"], "UNAVAILABLE")

    def test_new_package_requires_operations_evidence_inventory(self) -> None:
        (self.snapshot / "aws-operations-alarm-topic.json").unlink()
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertFalse(output.exists())

    def test_topic_contract_fails_closed_on_inconsistent_or_private_fields(
        self,
    ) -> None:
        original = json.loads(
            (self.snapshot / "aws-alarm-topic-subscriptions.json").read_text()
        )
        mutations = [
            lambda v: v["attributes"].update(
                TopicArn=v["selection"]["other_role_topic_arn"]
            ),
            lambda v: v["attributes"].update(Owner="999922223333"),
            lambda v: v["attributes"].update(SubscriptionsConfirmed="2"),
            lambda v: v["attributes"].update(SubscriptionsConfirmed=1),
            lambda v: v["attributes"].pop("KmsMasterKeyId"),
            lambda v: v["selection"].update(
                other_role_topic_arn=v["selection"]["topic_arn"]
            ),
            lambda v: v["selection"].update(source="ASSUMED"),
            lambda v: v["subscriptions"][0].update(
                TopicArn=v["selection"]["other_role_topic_arn"]
            ),
            lambda v: v["subscriptions"][0].update(Owner="999922223333"),
            lambda v: v["subscriptions"][0].update(SubscriptionArn="not-confirmed"),
            lambda v: v["subscriptions"][0].update(SubscriptionArn="Deleted"),
            lambda v: v["subscriptions"].append(v["subscriptions"][0]),
            lambda v: v["subscriptions"][0].update(Protocol="unknown"),
            lambda v: v["subscriptions"][0].update(
                Endpoint="must-not-persist@example.invalid"
            ),
            lambda v: v.update(role="OPERATIONS"),
            lambda v: v.update(delivery_evidence={"status": "PASS"}),
        ]
        self.assertTrue(valid_topic(original, "SECURITY", "111122223333", "us-east-1"))
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                value = json.loads(json.dumps(original))
                mutate(value)
                self.assertFalse(
                    valid_topic(value, "SECURITY", "111122223333", "us-east-1")
                )

    def test_pending_and_empty_topics_are_available_configuration_not_delivery(
        self,
    ) -> None:
        path = self.snapshot / "aws-alarm-topic-subscriptions.json"
        value = json.loads(path.read_text())
        value["attributes"].update(SubscriptionsConfirmed="0", SubscriptionsPending="1")
        value["subscriptions"][0]["SubscriptionArn"] = "PendingConfirmation"
        self.assertTrue(valid_topic(value, "SECURITY", "111122223333", "us-east-1"))
        write_json(path, value)
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        self.assertEqual(
            self.assertion(report, "AWS_SECURITY_EVENT_ROUTING")["status"], "FAIL"
        )
        self.assertEqual(
            self.assertion(report, "AWS_ALERT_DELIVERY_TESTED")["status"], "UNAVAILABLE"
        )
        value["attributes"]["SubscriptionsPending"] = "0"
        value["subscriptions"] = []
        self.assertTrue(valid_topic(value, "SECURITY", "111122223333", "us-east-1"))

    def test_read_only_collector_pins_topics_redacts_and_rejects_partial_inventory(
        self,
    ) -> None:
        fake_bin = self.root / "fake-bin"
        fake_bin.mkdir()
        gh = fake_bin / "gh"
        gh.write_text("#!/bin/sh\nexit 1\n")
        gh.chmod(0o700)
        aws = fake_bin / "aws"
        aws.write_text("""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with Path(os.environ["CALLS"]).open("a") as log:
    log.write(json.dumps(args) + "\\n")
if args[:2] == ["sts", "get-caller-identity"]:
    print(json.dumps({"Account":"111122223333", "Arn":"arn:aws:sts::111122223333:assumed-role/read-only/session", "UserId":"test"}))
    sys.exit(0)
if args[:1] != ["sns"]:
    sys.exit(1)
assert args[1] in ("get-topic-attributes", "list-subscriptions-by-topic")
topic = args[args.index("--topic-arn") + 1]
assert topic in ("arn:aws:sns:us-east-1:111122223333:exact-security", "arn:aws:sns:us-east-1:111122223333:exact-ops")
assert args[args.index("--region") + 1] == "us-east-1"
query = args[args.index("--query") + 1]
assert "Endpoint" not in query and "Policy" not in query
case = os.environ["TOPIC_CASE"]
if case == "denied" and "exact-security" in topic:
    sys.exit(1)
if args[1] == "get-topic-attributes":
    assert "KmsMasterKeyId" in query and "TopicArn" in query
    result = {"TopicArn":topic, "Owner":"111122223333", "SubscriptionsConfirmed":"1", "SubscriptionsPending":"0", "KmsMasterKeyId":None}
    if case == "mismatch": result["Owner"] = "999922223333"
else:
    assert args[args.index("--max-items") + 1] == "1000"
    assert "NextToken" in query and "TopicArn" in query
    result = {"Subscriptions":[{"SubscriptionArn":topic+":00000000-0000-0000-0000-000000000001", "TopicArn":topic, "Owner":"111122223333", "Protocol":"email"}], "NextToken":None}
    if case == "capped": result["NextToken"] = "incomplete"
    if case == "endpoint": result["Subscriptions"][0]["Endpoint"] = "must-not-persist@example.invalid"
print(json.dumps(result))
""")
        aws.chmod(0o700)
        for case in (
            "valid",
            "denied",
            "mismatch",
            "capped",
            "endpoint",
            "cross-account",
            "cross-region",
            "alias",
        ):
            with self.subTest(case=case):
                parent = self.root / case
                parent.mkdir()
                calls = parent / "calls.jsonl"
                security = "arn:aws:sns:us-east-1:111122223333:exact-security"
                operations = "arn:aws:sns:us-east-1:111122223333:exact-ops"
                if case == "cross-account":
                    security = security.replace("111122223333", "999922223333")
                if case == "cross-region":
                    security = security.replace("us-east-1", "us-west-2")
                if case == "alias":
                    security = operations
                env = {
                    **os.environ,
                    "PATH": f"{fake_bin}:{os.environ['PATH']}",
                    "AWS_REGION": "us-east-1",
                    "ARBION_SECURITY_ALARM_TOPIC_ARN": security,
                    "ARBION_OPERATIONS_ALARM_TOPIC_ARN": operations,
                    "TOPIC_CASE": case,
                    "CALLS": str(calls),
                }
                completed = subprocess.run(
                    [
                        str(
                            REPOSITORY_ROOT
                            / "scripts/collect-soc2-external-evidence.sh"
                        ),
                        str(parent),
                    ],
                    env=env,
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(completed.returncode, 2, completed.stderr)
                snapshot = next(parent.glob("arbion-soc2-*"))
                verified = subprocess.run(
                    [
                        str(
                            REPOSITORY_ROOT / "scripts/verify-soc2-evidence-snapshot.sh"
                        ),
                        str(snapshot),
                    ],
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(verified.returncode, 0, verified.stderr)
                security_value = json.loads(
                    (snapshot / "aws-alarm-topic-subscriptions.json").read_text()
                )
                operations_value = json.loads(
                    (snapshot / "aws-operations-alarm-topic.json").read_text()
                )
                if case == "valid":
                    self.assertTrue(
                        valid_topic(
                            security_value, "SECURITY", "111122223333", "us-east-1"
                        )
                    )
                    self.assertTrue(
                        valid_topic(
                            operations_value, "OPERATIONS", "111122223333", "us-east-1"
                        )
                    )
                    self.assertEqual(
                        security_value["selection"]["source"], "EXPLICIT_ARN"
                    )
                else:
                    self.assertEqual(security_value["status"], "UNAVAILABLE")
                if case == "denied":
                    self.assertEqual(security_value["selection"]["topic_arn"], security)
                    self.assertEqual(security_value["role"], "SECURITY")
                    self.assertNotIn("attributes", security_value)
                    self.assertTrue(
                        valid_topic(
                            operations_value, "OPERATIONS", "111122223333", "us-east-1"
                        )
                    )
                if case in ("cross-account", "cross-region", "alias"):
                    self.assertFalse(
                        any(
                            json.loads(line)[0] == "sns"
                            for line in calls.read_text().splitlines()
                        )
                    )
                all_evidence = "".join(path.read_text() for path in snapshot.iterdir())
                self.assertNotIn("must-not-persist", all_evidence)
                self.assertFalse(
                    any(path.name.startswith(".") for path in snapshot.iterdir())
                )

    def test_guardduty_requires_exact_enabled_status_inventory(self) -> None:
        for index, (value, expected) in enumerate(
            [
                ({"DetectorIds": [], "Detectors": []}, "FAIL"),
                ({"DetectorIds": ["a" * 32]}, "UNAVAILABLE"),
                ({"DetectorIds": ["a" * 32], "Detectors": []}, "UNAVAILABLE"),
                (
                    {
                        "DetectorIds": ["a" * 32],
                        "Detectors": [{"DetectorId": "b" * 32, "Status": "ENABLED"}],
                    },
                    "UNAVAILABLE",
                ),
                (
                    {
                        "DetectorIds": ["a" * 32],
                        "Detectors": [{"DetectorId": "a" * 32, "Status": "DISABLED"}],
                    },
                    "FAIL",
                ),
                (
                    {
                        "DetectorIds": ["a" * 32],
                        "Detectors": [{"DetectorId": "a" * 32, "Status": "UNKNOWN"}],
                    },
                    "UNAVAILABLE",
                ),
                (
                    {
                        "DetectorIds": ["a" * 32, "a" * 32],
                        "Detectors": [{"DetectorId": "a" * 32, "Status": "ENABLED"}]
                        * 2,
                    },
                    "UNAVAILABLE",
                ),
            ]
        ):
            with self.subTest(value=value):
                write_json(self.snapshot / "aws-guardduty-detectors.json", value)
                seal_snapshot(self.snapshot)
                completed, output = self.run_review(f"detector-{index}.json")
                self.assertEqual(completed.returncode, 0, completed.stderr)
                report = json.loads(output.read_text())
                self.assertEqual(
                    self.assertion(report, "AWS_THREAT_DETECTION_ACTIVE")["status"],
                    expected,
                )

    def test_object_lock_requires_provider_wrapper(self) -> None:
        path = self.snapshot / "aws-backup-bucket-object-lock.json"
        original = json.loads(path.read_text())
        write_json(path, original["ObjectLockConfiguration"])
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        result = next(
            item
            for item in report["results"]
            if "backup-bucket-object-lock.json" in str(item)
        )
        self.assertEqual(result["status"], "UNAVAILABLE")

    def test_lightsail_alarm_must_target_exact_production_instance(self) -> None:
        path = self.snapshot / "aws-lightsail-alarms.json"
        original = json.loads(path.read_text())
        for index, (resource, expected) in enumerate(
            [
                (
                    {
                        "name": "arbion-production-host",
                        "resourceType": "Instance",
                        "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/other",
                    },
                    "FAIL",
                ),
                ({}, "UNAVAILABLE"),
            ]
        ):
            value = json.loads(json.dumps(original))
            value[0]["monitoredResourceInfo"] = resource
            write_json(path, value)
            seal_snapshot(self.snapshot)
            completed, output = self.run_review(f"lightsail-{index}.json")
            self.assertEqual(completed.returncode, 0, completed.stderr)
            report = json.loads(output.read_text())
            self.assertEqual(
                self.assertion(report, "AWS_LIGHTSAIL_MONITORING_CONFIGURED")["status"],
                expected,
            )

    def test_missing_field_is_unavailable_without_inference(self) -> None:
        permissions = self.snapshot / "github-actions-permissions.json"
        value = json.loads(permissions.read_text(encoding="utf-8"))
        del value["default_workflow_permissions"]
        write_json(permissions, value)
        seal_snapshot(self.snapshot)

        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text(encoding="utf-8"))
        result = self.assertion(report, "GITHUB_ACTIONS_DEFAULT_READ_ONLY")
        self.assertEqual(result["status"], "UNAVAILABLE")
        self.assertEqual(report["review_state"], "INCOMPLETE_REVIEW_REQUIRED")

    def test_declared_unavailable_source_stays_unavailable(self) -> None:
        write_json(
            self.snapshot / "github-actions-permissions.json",
            {
                "status": "UNAVAILABLE",
                "system": "GITHUB",
                "reason": "Authorized read-only collection was unavailable.",
                "collected_at": COLLECTED_AT,
            },
        )
        summary_path = self.snapshot / "collection-summary.json"
        summary = json.loads(summary_path.read_text(encoding="utf-8"))
        summary["status"] = "INCOMPLETE"
        summary["unavailable_sources"] = ["github-actions-permissions"]
        write_json(summary_path, summary)
        seal_snapshot(self.snapshot)

        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertEqual(
            self.assertion(report, "GITHUB_ACTIONS_DEFAULT_READ_ONLY")["status"],
            "UNAVAILABLE",
        )

    def test_coarse_collection_placeholders_make_all_assertions_unavailable(
        self,
    ) -> None:
        for path in self.snapshot.glob("*.json"):
            path.unlink()
        write_json(
            self.snapshot / "collection-summary.json",
            {
                "schema_version": "1.0",
                "collection_id": COLLECTION_ID,
                "collected_at": COLLECTED_AT,
                "status": "INCOMPLETE",
                "repository": "example/arbion",
                "aws_region": "us-east-1",
                "unavailable_sources": ["aws", "github"],
                "limitations": ["Requires reviewer evaluation"],
            },
        )
        for file_name, system in (("aws.json", "AWS"), ("github.json", "GITHUB")):
            write_json(
                self.snapshot / file_name,
                {
                    "status": "UNAVAILABLE",
                    "system": system,
                    "reason": "Authorized read-only authentication was unavailable.",
                    "collected_at": COLLECTED_AT,
                },
            )
        seal_snapshot(self.snapshot)

        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertEqual(report["summary"]["unavailable"], 22)
        self.assertEqual(report["summary"]["pass"], 0)
        self.assertEqual(report["summary"]["fail"], 0)
        self.assertEqual(len(report["source_inventory"]), 3)

    def test_tampered_snapshot_fails_without_output(self) -> None:
        repository = self.snapshot / "github-repository.json"
        repository.write_bytes(repository.read_bytes() + b" ")
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertFalse(output.exists())

    def test_malformed_json_fails_without_output(self) -> None:
        (self.snapshot / "github-repository.json").write_text("{\n", encoding="utf-8")
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertFalse(output.exists())

    def test_repository_identity_conflict_fails_without_output(self) -> None:
        repository = self.snapshot / "github-repository.json"
        value = json.loads(repository.read_text(encoding="utf-8"))
        value["full_name"] = "example/different"
        write_json(repository, value)
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("repository identity conflicts", completed.stderr)
        self.assertFalse(output.exists())

    def test_duplicate_json_key_fails_without_output(self) -> None:
        (self.snapshot / "github-actions-permissions.json").write_text(
            '{"default_workflow_permissions":"read",'
            '"default_workflow_permissions":"write",'
            '"can_approve_pull_request_reviews":false}\n',
            encoding="utf-8",
        )
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("duplicate JSON object key", completed.stderr)
        self.assertFalse(output.exists())

    def test_existing_output_is_not_overwritten(self) -> None:
        output = self.root / "review.json"
        output.write_text("sentinel", encoding="utf-8")
        completed, returned_output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertEqual(returned_output.read_text(encoding="utf-8"), "sentinel")

    def test_output_inside_repository_is_rejected(self) -> None:
        output = REPOSITORY_ROOT / "scripts" / "unsafe-soc2-review-output.json"
        self.assertFalse(output.exists())
        completed = subprocess.run(
            [str(REVIEWER), str(self.snapshot), str(output)],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("outside the Git repository", completed.stderr)
        self.assertFalse(output.exists())

    def test_output_inside_snapshot_is_rejected(self) -> None:
        output = self.snapshot / "review.json"
        completed = subprocess.run(
            [str(REVIEWER), str(self.snapshot), str(output)],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("must not modify the verified snapshot", completed.stderr)
        self.assertFalse(output.exists())

    def test_unsafe_output_name_is_rejected(self) -> None:
        completed, output = self.run_review("review draft.json")
        self.assertNotEqual(completed.returncode, 0)
        self.assertIn("unsafe or noncanonical", completed.stderr)
        self.assertFalse(output.exists())

    def test_summary_unavailable_set_mismatch_fails_without_output(self) -> None:
        write_json(
            self.snapshot / "github-actions-permissions.json",
            {
                "status": "UNAVAILABLE",
                "system": "GITHUB",
                "reason": "Authorized read-only collection was unavailable.",
                "collected_at": COLLECTED_AT,
            },
        )
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertNotEqual(completed.returncode, 0)
        self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main(verbosity=2)
