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


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
REVIEWER = REPOSITORY_ROOT / "scripts" / "review-soc2-external-evidence.py"
COLLECTION_ID = "arbion-soc2-20260909T160000Z"
COLLECTED_AT = "2026-09-09T16:00:00Z"

GITHUB_NAMES = {
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
                    {
                        "ApplyServerSideEncryptionByDefault": {
                            "SSEAlgorithm": algorithm
                        }
                    }
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
        {"Rules": [{"ID": "retention", "Status": "Enabled", "Expiration": {"Days": 45}}]},
    )


def make_complete_snapshot(snapshot: Path) -> dict[str, str]:
    snapshot.mkdir(mode=0o700, parents=True)
    write_json(
        snapshot / "collection-summary.json",
        {
            "schema_version": "1.0",
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
        {"ConfigurationRecordersStatus": [{"name": "arbion-production", "recording": True}]},
    )
    write_json(
        snapshot / "aws-config-delivery-channels.json",
        {"DeliveryChannels": [{"name": "arbion-production", "s3BucketName": "arbion-audit"}]},
    )
    write_json(snapshot / "aws-guardduty-detectors.json", {
        "DetectorIds": ["a" * 32],
        "Detectors": [{"DetectorId": "a" * 32, "Status": "ENABLED"}],
    })
    write_json(
        snapshot / "aws-access-analyzers.json",
        {"analyzers": [{"name": "arbion-production", "status": "ACTIVE", "type": "ACCOUNT"}]},
    )
    write_json(
        snapshot / "aws-security-event-rule.json",
        {
            "Name": "arbion-production-guardduty-findings",
            "State": "ENABLED",
            "EventPattern": json.dumps({"detail-type": ["GuardDuty Finding"]}, sort_keys=True),
        },
    )
    write_json(
        snapshot / "aws-security-event-targets.json",
        {"Targets": [{"Id": "operations", "Arn": "arn:aws:sns:us-east-1:111122223333:ops"}]},
    )
    write_json(
        snapshot / "aws-alarm-topic-subscriptions.json",
        [{"SubscriptionArn": "arn:aws:sns:us-east-1:111122223333:ops:sub", "Protocol": "email"}],
    )
    write_json(
        snapshot / "aws-cloudwatch-alarms.json",
        [
            {
                "AlarmName": "arbion-production-security",
                "ActionsEnabled": True,
                "AlarmActions": ["arn:aws:sns:us-east-1:111122223333:ops"],
            }
        ],
    )
    bucket_evidence(snapshot, "audit", "aws:kms")
    bucket_evidence(snapshot, "backup", "AES256")
    write_json(
        snapshot / "aws-lightsail-instances.json",
        [{"name": "arbion-production-host", "state": "running", "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/test"}],
    )
    write_json(
        snapshot / "aws-lightsail-alarms.json",
        [
            {"name": name, "notificationEnabled": True, "contactProtocols": ["Email"],
             "monitoredResourceInfo": {"name": "arbion-production-host", "resourceType": "Instance",
                                       "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/test"}}
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
        return next(item for item in report["results"] if item["assertion_id"] == assertion_id)

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
        self.assertEqual(report["artifact_status"], "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE")
        self.assertEqual(report["review_state"], "DRAFT_PASS_REVIEW_REQUIRED")
        self.assertEqual(
            report["summary"],
            {"assertion_count": 15, "pass": 15, "fail": 0, "unavailable": 0},
        )
        self.assertEqual(len(report["source_inventory"]), 33)
        report_inventory = {item["file"]: item["sha256"] for item in report["source_inventory"]}
        self.assertEqual(report_inventory, self.manifest)
        self.assertTrue(all(result["control_ids"] for result in report["results"]))

        claimed = report.pop("integrity")["payload_sha256"]
        canonical = json.dumps(
            report,
            sort_keys=True,
            separators=(",", ":"),
            ensure_ascii=False,
        ).encode("utf-8") + b"\n"
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

    def test_guardduty_requires_exact_enabled_status_inventory(self) -> None:
        for index, (value, expected) in enumerate([
            ({"DetectorIds": [], "Detectors": []}, "FAIL"),
            ({"DetectorIds": ["a" * 32]}, "UNAVAILABLE"),
            ({"DetectorIds": ["a" * 32], "Detectors": []}, "UNAVAILABLE"),
            ({"DetectorIds": ["a" * 32], "Detectors": [{"DetectorId": "b" * 32, "Status": "ENABLED"}]}, "UNAVAILABLE"),
            ({"DetectorIds": ["a" * 32], "Detectors": [{"DetectorId": "a" * 32, "Status": "DISABLED"}]}, "FAIL"),
            ({"DetectorIds": ["a" * 32], "Detectors": [{"DetectorId": "a" * 32, "Status": "UNKNOWN"}]}, "UNAVAILABLE"),
            ({"DetectorIds": ["a" * 32, "a" * 32], "Detectors": [{"DetectorId": "a" * 32, "Status": "ENABLED"}] * 2}, "UNAVAILABLE"),
        ]):
            with self.subTest(value=value):
                write_json(self.snapshot / "aws-guardduty-detectors.json", value)
                seal_snapshot(self.snapshot)
                completed, output = self.run_review(f"detector-{index}.json")
                self.assertEqual(completed.returncode, 0, completed.stderr)
                report = json.loads(output.read_text())
                self.assertEqual(self.assertion(report, "AWS_THREAT_DETECTION_ACTIVE")["status"], expected)

    def test_object_lock_requires_provider_wrapper(self) -> None:
        path = self.snapshot / "aws-backup-bucket-object-lock.json"
        original = json.loads(path.read_text())
        write_json(path, original["ObjectLockConfiguration"])
        seal_snapshot(self.snapshot)
        completed, output = self.run_review()
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads(output.read_text())
        result = next(item for item in report["results"] if "backup-bucket-object-lock.json" in str(item))
        self.assertEqual(result["status"], "UNAVAILABLE")

    def test_lightsail_alarm_must_target_exact_production_instance(self) -> None:
        path = self.snapshot / "aws-lightsail-alarms.json"
        original = json.loads(path.read_text())
        for index, (resource, expected) in enumerate([
            ({"name": "arbion-production-host", "resourceType": "Instance", "arn": "arn:aws:lightsail:us-east-1:111122223333:Instance/other"}, "FAIL"),
            ({}, "UNAVAILABLE"),
        ]):
            value = json.loads(json.dumps(original))
            value[0]["monitoredResourceInfo"] = resource
            write_json(path, value)
            seal_snapshot(self.snapshot)
            completed, output = self.run_review(f"lightsail-{index}.json")
            self.assertEqual(completed.returncode, 0, completed.stderr)
            report = json.loads(output.read_text())
            self.assertEqual(self.assertion(report, "AWS_LIGHTSAIL_MONITORING_CONFIGURED")["status"], expected)

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

    def test_coarse_collection_placeholders_make_all_assertions_unavailable(self) -> None:
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
        self.assertEqual(report["summary"]["unavailable"], 15)
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
