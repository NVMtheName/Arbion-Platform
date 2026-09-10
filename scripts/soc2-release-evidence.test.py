#!/usr/bin/env python3
"""Release/PR/CI/backup bindings, unavailable history, and safe I/O regressions."""

from __future__ import annotations

import copy
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import soc2_release_evidence as e

REPO = {"id": 123, "full_name": "example/arbion"}
SHA, BASE, HEAD = "a" * 40, "b" * 40, "c" * 40
SAVED = "2026-09-09T16:05:00Z"


def epoch(value: str) -> int:
    return int(
        datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ")
        .replace(tzinfo=timezone.utc)
        .timestamp()
    )


def host() -> dict:
    value = {
        "schema_version": "1.2",
        "collection_id": "arbion-soc2-host-20260909T160000Z",
        "status": "COMPLETE_REVIEW_REQUIRED",
        "collected_at": "2026-09-09T16:00:00Z",
        "host": {
            "name": "arbion-production",
            "os": "ubuntu",
            "os_version": "24.04",
            "kernel": "6.8.0",
        },
        "release_sha": SHA,
        "summary": dict.fromkeys(
            [
                "services",
                "application_hardening",
                "network_exposure",
                "monitoring_timers",
                "sensitive_file_permissions",
                "checks",
            ],
            "PASS",
        )
        | {"backup": "CURRENT"},
        "services": [],
        "monitoring_timers": [],
        "sensitive_file_permissions": [
            {
                "file": f,
                "owner": "root",
                "group": "root",
                "mode": "600",
                "status": "AVAILABLE",
            }
            for f in [
                "production-environment",
                "backup-environment",
                "alert-environment",
            ]
        ],
        "backup": {
            "status": "CURRENT",
            "completed_epoch": epoch("2026-09-09T15:55:00Z"),
            "object_key": "postgres/daily/arbion-fixture.dump",
            "age_seconds": 300,
            "maximum_age_seconds": 129600,
        },
        "read_only_checks": [
            {"check": c, "result": "PASS"}
            for c in [
                "public-smoke",
                "container-health",
                "host-capacity",
                "memory-pressure",
                "tls-certificate",
                "backup-freshness",
            ]
        ],
        "limitations": [
            "Point-in-time read-only host snapshot",
            "Contains no environment values, credentials, customer records, application records, database content, or logs",
            "Requires reviewer evaluation and separate alert-delivery, restore, access, and patch evidence",
            "Does not by itself establish operating effectiveness or SOC 2 certification",
        ],
    }
    for i, service in enumerate(["proxy", "postgres", "redis", "api", "ai", "web"], 1):
        app = service in ("api", "ai", "web")
        value["services"].append(
            {
                "service": service,
                "container_id": f"{i:x}" * 64,
                "configured_image": "fixture/" + service,
                "image_id": "sha256:" + f"{i + 6:x}" * 64,
                "runtime_user": "arbion" if app else "IMAGE_DEFAULT",
                "status": "running",
                "health": "healthy",
                "started_at": "2026-09-09T15:57:00.123456789Z",
                "restart_count": 0,
                "read_only_root": app,
                "capability_drop": ["ALL"] if app else [],
                "security_options": ["no-new-privileges:true"] if app else [],
                "process_limit": 256 if app else None,
                "published_ports": {
                    p: [{"HostIp": "0.0.0.0", "HostPort": p.split("/")[0]}]
                    for p in ("80/tcp", "443/tcp", "443/udp")
                }
                if service == "proxy"
                else {},
            }
        )
    for name in [
        "postgres-backup",
        "postgres-backup-freshness",
        "host-capacity",
        "memory-pressure",
        "production-containers",
        "production-health",
        "reboot-required",
        "tls-certificate",
        "docker-build-cache-prune",
    ]:
        value["monitoring_timers"].append(
            {
                "unit": f"arbion-{name}.timer",
                "load_state": "loaded",
                "active_state": "active",
                "sub_state": "waiting",
                "unit_file_state": "enabled",
                "next_run": "Wed 2026-09-09 16:10:00 UTC",
                "next_run_clock": "REALTIME",
            }
        )
    value["integrity"] = {
        "algorithm": "SHA-256",
        "canonicalization": "JQ_SORTED_COMPACT_UTF8_V1",
        "covers": "ENTIRE_DOCUMENT_EXCLUDING_INTEGRITY",
        "payload_sha256": e.digest(e.canonical(value)),
    }
    return value


def pr() -> dict:
    return {
        "id": 1000,
        "number": 9,
        "html_url": "https://github.com/example/arbion/pull/9",
        "state": "closed",
        "draft": False,
        "merged": True,
        "merged_at": "2026-09-09T15:50:00Z",
        "merge_commit_sha": SHA,
        "base": {"ref": "main", "sha": BASE, "repo": REPO},
        "head": {"ref": "codex/test", "sha": HEAD, "repo": REPO},
    }


def runs(main: bool = False) -> dict:
    commit, event, branch = (
        (SHA, "push", "main") if main else (HEAD, "pull_request", "codex/test")
    )
    rows = [
        {
            "id": i + (10 if main else 1),
            "path": path,
            "event": event,
            "head_sha": commit,
            "head_branch": branch,
            "run_attempt": 1,
            "created_at": "2026-09-09T15:51:00Z" if main else "2026-09-09T15:40:00Z",
            "updated_at": "2026-09-09T15:53:00Z" if main else "2026-09-09T15:45:00Z",
            "status": "completed",
            "conclusion": "success",
            "html_url": f"https://github.com/example/arbion/actions/runs/{i + (10 if main else 1)}",
            "repository": REPO,
            "head_repository": REPO,
        }
        for i, path in enumerate(e.WORKFLOWS)
    ]
    return {
        "endpoint": f"repos/example/arbion/actions/runs?head_sha={commit}&event={event}&per_page=100",
        "page_counts": [2],
        "total_count": 2,
        "runs": rows,
    }


def receipt() -> dict:
    return {
        "schema_version": "1.0",
        "source": "LIGHTSAIL_DEPLOY_SCRIPT",
        "host": "arbion-production",
        "release_sha": SHA,
        "previous_release_sha": BASE,
        "archive_sha256": "d" * 64,
        "started_epoch": epoch("2026-09-09T15:54:00Z"),
        "backup": {
            "completed_epoch": epoch("2026-09-09T15:55:00Z"),
            "object_key": "postgres/daily/arbion-fixture.dump",
        },
        "replacement_started_epoch": epoch("2026-09-09T15:56:00Z"),
        "replacement_completed_epoch": epoch("2026-09-09T15:57:00Z"),
        "completed_epoch": epoch("2026-09-09T15:58:00Z"),
        "rollback": {
            "path": f"/opt/arbion/.rollback/release-pre-{BASE}-20260909T155505Z.tar.gz",
            "sha256": "e" * 64,
        },
        "checks": {"readiness": "PASS", "public_smoke": "PASS", "containers": "PASS"},
    }


def fixture() -> dict:
    prefix = "https://api.github.com/repos/example/arbion/git/commits/"
    return {
        "schema_version": "1.0",
        "artifact_status": "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE",
        "collected_at": SAVED,
        "repository": REPO,
        "host_evidence": host(),
        "host_canonical_sha256": e.digest(e.canonical(host())),
        "deployment_receipt": receipt(),
        "github": {
            "method": "GET",
            "api_version": e.API_VERSION,
            "repository": REPO,
            "pr": pr(),
            "merge_commit": {
                "sha": SHA,
                "url": prefix + SHA,
                "parents": [{"sha": s, "url": prefix + s} for s in [BASE, HEAD]],
            },
            "pr_runs": runs(),
            "main_runs": runs(True),
        },
        "assertions": {},
        "limitations": e.LIMITATIONS,
    }


def seal(value: dict) -> dict:
    value["assertions"] = e.evaluate(value, validate_host=False)
    return value | {"payload_sha256": e.digest(e.canonical(value))}


class ReleaseEvidenceTests(unittest.TestCase):
    def test_complete_exact_chain_and_no_approval_inference(self) -> None:
        value = seal(fixture())
        e.verify(value)  # Real strict host verifier, not mocked.
        self.assertEqual(list(value["assertions"].values()).count("PASS"), 5)
        for key in (
            "INDEPENDENT_REVIEW",
            "DEPLOYMENT_APPROVAL",
            "ARTIFACT_AUTHENTICITY",
            "CHECKOUT_ATTESTATION",
            "DURABLE_RETENTION",
            "ENFORCED_PROTECTION",
        ):
            self.assertEqual(value["assertions"][key], "UNAVAILABLE")

    def test_legacy_no_receipt_remains_unavailable(self) -> None:
        value = fixture()
        value["deployment_receipt"] = None
        self.assertEqual(
            e.evaluate(value, False)["PRE_DEPLOY_BACKUP_AND_ROLLBACK_BOUND"],
            "UNAVAILABLE",
        )

    def test_identity_and_time_mutations_fail_closed(self) -> None:
        mutations = [
            lambda v: v["github"].update(method="POST"),
            lambda v: v["github"]["repository"].update(id=999),
            lambda v: v["github"]["pr"].update(merged=False),
            lambda v: v["github"]["pr"].update(draft=True),
            lambda v: v["github"]["pr"].update(merge_commit_sha=HEAD),
            lambda v: v["github"]["pr"].update(number=True),
            lambda v: v["github"]["pr"].update(merged_at="2099-01-01T00:00:00Z"),
            lambda v: v["github"]["merge_commit"]["parents"].reverse(),
            lambda v: v["github"]["merge_commit"].update(parents=[]),
            lambda v: v["github"]["pr"]["base"].update(ref="wrong"),
            lambda v: v["github"]["pr"]["head"].update(
                repo={"id": 999, "full_name": "other/repo"}
            ),
            lambda v: v["github"]["main_runs"]["runs"][0].update(head_sha=HEAD),
            lambda v: v["github"]["main_runs"]["runs"][0].update(event="pull_request"),
            lambda v: v["github"]["main_runs"]["runs"][0].update(
                html_url="https://example.com/forged"
            ),
            lambda v: v["github"]["main_runs"]["runs"][0].update(
                created_at="2026-02-30T01:00:00Z"
            ),
            lambda v: v["github"]["main_runs"]["runs"][0].update(conclusion=None),
            lambda v: v["github"]["main_runs"]["runs"][0].update(raw_output="private"),
            lambda v: v["github"]["main_runs"].update(page_counts=[1, 1]),
            lambda v: v["github"]["main_runs"]["runs"][1].update(id=10),
            lambda v: v.update(collected_at="2026-01-01T00:00:00Z"),
        ]
        for i, mutate in enumerate(mutations):
            with self.subTest(i=i):
                value = json.loads(json.dumps(fixture()))  # Detach aliases.
                mutate(value)
                with self.assertRaises(ValueError):
                    e.evaluate(value, False)

    def test_receipt_rejects_wrong_backup_late_backup_and_rollback(self) -> None:
        mutations = [
            lambda r: r.update(host="other"),
            lambda r: r.update(release_sha=HEAD),
            lambda r: r.update(started_epoch=0),
            lambda r: r.update(started_epoch=True),
            lambda r: r["backup"].update(completed_epoch=epoch("2026-09-09T15:57:00Z")),
            lambda r: r["backup"].update(object_key="postgres/daily/other.dump"),
            lambda r: r["rollback"].update(
                path=f"/opt/arbion/.rollback/release-pre-{HEAD}-20260909T155505Z.tar.gz"
            ),
            lambda r: r["rollback"].update(
                path=f"/opt/arbion/.rollback/release-pre-{BASE}-20260909T160505Z.tar.gz"
            ),
            lambda r: r["rollback"].update(sha256="invalid"),
            lambda r: r.update(completed_epoch=epoch("2026-09-09T16:01:00Z")),
            lambda r: r["checks"].update(readiness="FAIL"),
            lambda r: r.update(secret="not allowed"),
        ]
        for i, mutate in enumerate(mutations):
            with self.subTest(i=i):
                value = fixture()
                mutate(value["deployment_receipt"])
                with self.assertRaises(ValueError):
                    e.evaluate(value, False)

    def test_pending_failed_ambiguous_rerun_and_late_pr_checks(self) -> None:
        for case, expected in [
            ("pending", "UNAVAILABLE"),
            ("failed", "FAIL"),
            ("rerun", "UNAVAILABLE"),
            ("late", "UNAVAILABLE"),
            ("missing", "UNAVAILABLE"),
            ("tie", "UNAVAILABLE"),
        ]:
            with self.subTest(case=case):
                value = runs()
                if case == "pending":
                    value["runs"][0].update(status="in_progress", conclusion=None)
                elif case == "failed":
                    value["runs"][0]["conclusion"] = "failure"
                elif case == "rerun":
                    value["runs"][0]["run_attempt"] = 2
                elif case == "late":
                    value["runs"][0]["updated_at"] = "2026-09-09T15:51:00Z"
                elif case == "missing":
                    value["runs"].pop()
                else:
                    value["runs"].append(copy.deepcopy(value["runs"][0]))
                self.assertEqual(
                    e.workflow_result(value, e.saved_time(pr()["merged_at"])), expected
                )

    def test_full_pagination_requires_terminating_page_and_stable_total(self) -> None:
        batch = {"total_count": 100, "workflow_runs": [{}] * 100}
        with patch.object(
            e,
            "read_json",
            side_effect=[batch, {"total_count": 100, "workflow_runs": []}],
        ):
            self.assertEqual(
                e.collect_runs("example/arbion", HEAD, "pull_request")["page_counts"],
                [100, 0],
            )
        for pages in [
            [{"total_count": 201, "workflow_runs": []}],
            [batch, {"total_count": 99, "workflow_runs": []}],
            [{"total_count": 2, "workflow_runs": [{}]}],
        ]:
            with (
                patch.object(e, "read_json", side_effect=pages),
                self.assertRaises(ValueError),
            ):
                e.collect_runs("example/arbion", HEAD, "pull_request")

    def test_preserved_failure_does_not_hide_later_recovery(self) -> None:
        value = runs()
        old = copy.deepcopy(value["runs"][0])
        old.update(
            id=99,
            html_url="https://github.com/example/arbion/actions/runs/99",
            created_at="2026-09-09T15:30:00Z",
            updated_at="2026-09-09T15:35:00Z",
            conclusion="failure",
        )
        value["runs"].append(old)
        value.update(total_count=3, page_counts=[3])
        e.validate_runs(
            value, REPO, HEAD, "pull_request", "codex/test", e.saved_time(SAVED)
        )
        self.assertEqual(
            e.workflow_result(value, e.saved_time(pr()["merged_at"])), "PASS"
        )
        self.assertEqual(value["runs"][-1]["conclusion"], "failure")

    def test_cross_scope_identity_canonical_host_and_freshness(self) -> None:
        value = fixture()
        value["github"]["main_runs"]["runs"][0].update(
            id=1, html_url="https://github.com/example/arbion/actions/runs/1"
        )
        with self.assertRaises(ValueError):
            e.evaluate(value, False)
        value = fixture()
        value["host_canonical_sha256"] = "0" * 64
        with self.assertRaises(ValueError):
            e.evaluate(value, False)
        value = fixture()
        value["deployment_receipt"]["replacement_started_epoch"] = epoch(
            "2026-09-09T16:00:01Z"
        )
        value["deployment_receipt"]["replacement_completed_epoch"] = epoch(
            "2026-09-09T16:00:01Z"
        )
        value["deployment_receipt"]["completed_epoch"] = epoch("2026-09-09T16:00:01Z")
        # Freshness has its own boundary, independent of the post-deploy host cutoff.
        later_host = host()
        later_host["collected_at"] = "2026-09-09T16:05:00Z"
        with self.assertRaisesRegex(ValueError, "PRE_DEPLOY_BACKUP_NOT_FRESH"):
            e.validate_receipt(
                value["deployment_receipt"],
                later_host,
                e.saved_time(pr()["merged_at"]),
                e.saved_time(SAVED),
            )

    def test_boolean_identity_and_fifo_are_rejected(self) -> None:
        bad_pr = pr()
        bad_pr["base"] = copy.deepcopy(bad_pr["base"])
        bad_pr["base"]["repo"] = {"full_name": REPO["full_name"], "id": True}
        with self.assertRaises(ValueError):
            e.validate_pr(
                bad_pr,
                {"id": 1, "full_name": REPO["full_name"]},
                SHA,
                e.saved_time(SAVED),
            )
        import os

        with tempfile.TemporaryDirectory(prefix="arbion-release-fifo-") as directory:
            path = Path(directory) / "fifo.json"
            os.mkfifo(path)
            with self.assertRaises(ValueError):
                e.read_file(path)

    def test_safe_get_only_collector_and_changed_population(self) -> None:
        value = fixture()
        calls = []

        def reader(endpoint, projection):
            calls.append((endpoint, projection))
            if endpoint == "repos/example/arbion":
                return REPO
            if "/pulls/" in endpoint:
                return pr()
            if "/git/commits/" in endpoint:
                return value["github"]["merge_commit"]
            return {
                "total_count": 2,
                "workflow_runs": runs("event=push" in endpoint)["runs"],
            }

        with (
            patch.object(e, "read_json", side_effect=reader),
            patch.object(e, "verify_host"),
        ):
            result = e.collect("example/arbion", 9, host(), receipt())
        self.assertEqual(result["assertions"]["MERGED_PR_RELEASE_BOUND"], "PASS")
        self.assertTrue(
            all(
                "logs" not in url and "rerun" not in url and "cancel" not in url
                for url, _ in calls
            )
        )
        with (
            patch.object(e, "read_json", side_effect=reader),
            patch.object(e, "verify_host"),
            patch.object(
                e, "collect_runs", side_effect=[runs(), runs(True), runs(True)]
            ),
        ):
            result = e.collect("example/arbion", 9, host(), None)
        self.assertEqual(
            result["github"],
            {"status": "UNAVAILABLE", "reason": "GITHUB_LINEAGE_UNAVAILABLE"},
        )

    def test_checksum_and_assertion_tampering_and_host_validation(self) -> None:
        value = seal(fixture())
        value["assertions"]["INDEPENDENT_REVIEW"] = "PASS"
        with self.assertRaises(ValueError):
            e.verify(value)
        payload = {k: v for k, v in value.items() if k != "payload_sha256"}
        value["payload_sha256"] = e.digest(e.canonical(payload))
        with self.assertRaises(ValueError):
            e.verify(value)
        bad_host = host()
        bad_host["summary"]["services"] = "FAIL"
        with self.assertRaises(ValueError):
            e.verify_host(bad_host)

    def test_offline_cli_safe_files_and_no_overwrite(self) -> None:
        with tempfile.TemporaryDirectory(prefix="arbion-release-test-") as directory:
            path = Path(directory) / "release.json"
            e.write_file(path, seal(fixture()))
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            result = subprocess.run(
                [
                    "python3",
                    str(e.ROOT / "scripts/soc2_release_evidence.py"),
                    "verify",
                    str(path),
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stdout)
            with self.assertRaises(FileExistsError):
                e.write_file(path, {})
            link = Path(directory) / "link.json"
            link.symlink_to(path)
            with self.assertRaises(ValueError):
                e.read_file(link)
            path.write_text('{"x":1,"x":2}')
            with self.assertRaises(ValueError):
                e.read_file(path)
            with self.assertRaises(ValueError):
                e.write_file(e.ROOT / "unsafe.json", {})


if __name__ == "__main__":
    unittest.main()
