#!/usr/bin/env python3
"""Read-only release lineage collection and offline, non-authoritative review."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile
from typing import Any

from soc2_github_security_evidence import (
    API_VERSION,
    EvidenceUnavailable,
    decode,
    positive_integer,
    read_json,
    saved_time,
)

ROOT = Path(__file__).resolve().parent.parent
LIMIT = 4_194_304
WORKFLOWS = [".github/workflows/ci.yml", ".github/workflows/security.yml"]
LIMITATIONS = [
    "Read-only point-in-time release lineage; not operating-effectiveness evidence",
    "PASS means only the stated saved-source binding matched",
    "Workflow head attribution is not an attestation of the actual checkout or build artifact",
    "Receipts are forward-only; missing historical deployment facts are never reconstructed",
    "Local hashes are not signatures or independent source authentication",
    "Requires independent review, approval, enforced protection and durable retention",
    "Does not establish SOC 2 compliance or certification",
]
PR_PROJECTION = """{id,number,html_url,state,draft,merged,merged_at,merge_commit_sha,
base:{ref:.base.ref,sha:.base.sha,repo:{id:.base.repo.id,full_name:.base.repo.full_name}},
head:{ref:.head.ref,sha:.head.sha,repo:{id:.head.repo.id,full_name:.head.repo.full_name}}}"""
RUN_PROJECTION = """{total_count,workflow_runs:[.workflow_runs[]|{id,path,event,head_sha,
head_branch,run_attempt,created_at,updated_at,status,conclusion,html_url,
repository:{id:.repository.id,full_name:.repository.full_name},
head_repository:{id:.head_repository.id,full_name:.head_repository.full_name}}]}"""


def require(value: Any, reason: str = "SOURCE_CONTRACT_INVALID") -> None:
    if not value:
        raise EvidenceUnavailable(reason)


def exact(value: Any, keys: str) -> None:
    require(isinstance(value, dict) and set(value) == set(keys.split()))


def matches(value: Any, pattern: str) -> bool:
    return isinstance(value, str) and bool(re.fullmatch(pattern, value))


def sha(value: Any, length: int = 40) -> bool:
    return matches(value, f"[0-9a-f]{{{length}}}")


def timestamp(value: Any, cutoff: datetime) -> datetime:
    result = saved_time(value)
    require(result is not None and result <= cutoff, "TIMESTAMP_INVALID")
    return result


def canonical(value: Any) -> bytes:
    return (
        json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
        + "\n"
    ).encode()


def digest(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def read_file(path: Path) -> tuple[Any, str]:
    require(
        not path.is_symlink() and not path.resolve().is_relative_to(ROOT),
        "UNSAFE_INPUT_PATH",
    )
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as handle:
        require(stat.S_ISREG(os.fstat(handle.fileno()).st_mode), "UNSAFE_INPUT_PATH")
        raw = handle.read(LIMIT + 1)
    require(0 < len(raw) <= LIMIT, "INPUT_SIZE_INVALID")
    # Shared decoder rejects duplicate keys and oversized source JSON.
    return decode(raw.decode("utf-8")), digest(raw)


def write_file(path: Path, value: Any) -> None:
    require(not path.parent.resolve().is_relative_to(ROOT), "UNSAFE_OUTPUT_PATH")
    require(
        matches(path.name, r"[A-Za-z0-9][A-Za-z0-9._-]*\.json"), "UNSAFE_OUTPUT_PATH"
    )
    raw = json.dumps(value, sort_keys=True, indent=2).encode() + b"\n"
    require(len(raw) <= 2_097_152, "OUTPUT_SIZE_INVALID")
    descriptor = os.open(
        path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600
    )
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(raw)


def verify_host(host: Any) -> None:
    # Validate the exact embedded snapshot, not a path that can change afterwards.
    with tempfile.TemporaryDirectory(prefix="arbion-release-host-") as directory:
        path = Path(directory) / "host.json"
        write_file(path, host)
        result = subprocess.run(
            [str(ROOT / "scripts/verify-soc2-host-evidence.sh"), str(path)],
            capture_output=True,
            timeout=45,
            check=False,
        )
        require(result.returncode == 0, "HOST_EVIDENCE_INVALID")


def identity(value: Any, repository: str) -> None:
    exact(value, "id full_name")
    require(positive_integer(value["id"]) and value["full_name"] == repository)


def validate_pr(pr: Any, repo: dict, release: str, cutoff: datetime) -> None:
    exact(
        pr, "id number html_url state draft merged merged_at merge_commit_sha base head"
    )
    require(positive_integer(pr["id"]) and positive_integer(pr["number"]))
    require(
        pr["html_url"] == f"https://github.com/{repo['full_name']}/pull/{pr['number']}"
    )
    require(pr["state"] == "closed" and pr["merged"] is True and pr["draft"] is False)
    require(pr["merge_commit_sha"] == release and sha(release))
    timestamp(pr["merged_at"], cutoff)
    for side in ("base", "head"):
        exact(pr[side], "ref sha repo")
        identity(pr[side]["repo"], repo["full_name"])
        require(pr[side]["repo"] == repo and sha(pr[side]["sha"]))
        require(matches(pr[side]["ref"], r"[A-Za-z0-9][A-Za-z0-9_./-]{0,199}"))
    require(pr["base"]["ref"] == "main" and pr["head"]["ref"] != "main")
    require(len({release, pr["base"]["sha"], pr["head"]["sha"]}) == 3)


def validate_commit(value: Any, pr: dict, repository: str) -> None:
    exact(value, "sha url parents")
    prefix = f"https://api.github.com/repos/{repository}/git/commits/"
    require(
        value["sha"] == pr["merge_commit_sha"] and value["url"] == prefix + value["sha"]
    )
    require(isinstance(value["parents"], list) and len(value["parents"]) == 2)
    for parent, side in zip(value["parents"], ("base", "head")):
        exact(parent, "sha url")
        require(
            parent["sha"] == pr[side]["sha"] and parent["url"] == prefix + parent["sha"]
        )


def collect_runs(repository: str, commit: str, event: str) -> dict:
    endpoint = (
        f"repos/{repository}/actions/runs?head_sha={commit}&event={event}&per_page=100"
    )
    rows, counts, total = [], [], None
    for page in range(1, 4):
        batch = read_json(f"{endpoint}&page={page}", RUN_PROJECTION)
        exact(batch, "total_count workflow_runs")
        require(
            type(batch["total_count"]) is int and 0 <= batch["total_count"] <= 200,
            "RUN_INVENTORY_CAPPED",
        )
        require(total is None or total == batch["total_count"], "RUN_INVENTORY_CHANGED")
        total = batch["total_count"]
        require(
            isinstance(batch["workflow_runs"], list)
            and len(batch["workflow_runs"]) <= 100
        )
        counts.append(len(batch["workflow_runs"]))
        rows.extend(batch["workflow_runs"])
        if counts[-1] < 100:
            break
    require(
        len(rows) == total and counts[-1] < 100 and len(rows) <= 200,
        "RUN_INVENTORY_INCOMPLETE",
    )
    return {
        "endpoint": endpoint,
        "page_counts": counts,
        "total_count": total,
        "runs": rows,
    }


def validate_runs(
    value: Any, repo: dict, commit: str, event: str, branch: str, cutoff: datetime
) -> None:
    exact(value, "endpoint page_counts total_count runs")
    require(
        value["endpoint"]
        == f"repos/{repo['full_name']}/actions/runs?head_sha={commit}&event={event}&per_page=100"
    )
    require(type(value["total_count"]) is int and 0 <= value["total_count"] <= 200)
    require(
        isinstance(value["runs"], list) and len(value["runs"]) == value["total_count"]
    )
    pages = value["page_counts"]
    require(isinstance(pages, list) and 1 <= len(pages) <= 3)
    require(all(type(n) is int and 0 <= n <= 100 for n in pages))
    require(
        pages[-1] < 100
        and all(n == 100 for n in pages[:-1])
        and sum(pages) == len(value["runs"])
    )
    ids = set()
    for row in value["runs"]:
        exact(
            row,
            "id path event head_sha head_branch run_attempt created_at updated_at status conclusion html_url repository head_repository",
        )
        require(positive_integer(row["id"]) and row["id"] not in ids)
        ids.add(row["id"])
        identity(row["repository"], repo["full_name"])
        identity(row["head_repository"], repo["full_name"])
        require(row["repository"] == repo and row["head_repository"] == repo)
        require(
            row["head_sha"] == commit
            and row["event"] == event
            and row["head_branch"] == branch
        )
        require(
            row["html_url"]
            == f"https://github.com/{repo['full_name']}/actions/runs/{row['id']}"
        )
        require(matches(row["path"], r"\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml"))
        require(positive_integer(row["run_attempt"]))
        require(
            timestamp(row["created_at"], cutoff) <= timestamp(row["updated_at"], cutoff)
        )
        require(
            row["status"]
            in ("queued", "in_progress", "completed", "requested", "waiting", "pending")
        )
        require(
            row["conclusion"]
            in (
                None,
                "success",
                "failure",
                "neutral",
                "cancelled",
                "skipped",
                "timed_out",
                "action_required",
                "stale",
                "startup_failure",
            )
        )
        require((row["status"] == "completed") == (row["conclusion"] is not None))


def workflow_result(value: dict, before: datetime | None = None) -> str:
    for path in WORKFLOWS:
        rows = [r for r in value["runs"] if r["path"] == path]
        if not rows:
            return "UNAVAILABLE"
        newest = max(r["created_at"] for r in rows)
        selected = [r for r in rows if r["created_at"] == newest]
        if len(selected) != 1:
            return "UNAVAILABLE"
        row = selected[0]
        # Prior attempts are not returned by this API. Never silently erase them.
        if row["run_attempt"] != 1 or row["status"] != "completed":
            return "UNAVAILABLE"
        if before and saved_time(row["updated_at"]) > before:
            return "UNAVAILABLE"
        if row["conclusion"] != "success":
            return "FAIL"
    return "PASS"


def validate_receipt(
    receipt: Any, host: dict, merged: datetime, cutoff: datetime
) -> None:
    exact(
        receipt,
        "schema_version source host release_sha previous_release_sha archive_sha256 started_epoch backup replacement_started_epoch replacement_completed_epoch completed_epoch rollback checks",
    )
    require(
        receipt["schema_version"] == "1.0"
        and receipt["source"] == "LIGHTSAIL_DEPLOY_SCRIPT"
    )
    require(
        receipt["host"] == host["host"]["name"]
        and receipt["release_sha"] == host["release_sha"]
    )
    require(sha(receipt["previous_release_sha"]) and sha(receipt["archive_sha256"], 64))
    exact(receipt["backup"], "completed_epoch object_key")
    exact(receipt["rollback"], "path sha256")
    exact(receipt["checks"], "readiness public_smoke containers")
    require(all(v == "PASS" for v in receipt["checks"].values()))
    times = [
        receipt["started_epoch"],
        receipt["backup"]["completed_epoch"],
        receipt["replacement_started_epoch"],
        receipt["replacement_completed_epoch"],
        receipt["completed_epoch"],
    ]
    require(all(type(t) is int and t >= 0 for t in times) and times == sorted(times))
    require(
        int(merged.timestamp()) <= times[0]
        and times[-1] <= int(timestamp(host["collected_at"], cutoff).timestamp())
    )
    require(times[2] - times[1] <= 300, "PRE_DEPLOY_BACKUP_NOT_FRESH")
    require(
        receipt["backup"]["completed_epoch"] == host["backup"]["completed_epoch"]
        and receipt["backup"]["object_key"] == host["backup"]["object_key"]
    )
    require(matches(receipt["backup"]["object_key"], r"postgres/daily/[A-Za-z0-9._-]+"))
    pattern = (
        r"/opt/arbion/\.rollback/release-pre-"
        + receipt["previous_release_sha"]
        + r"-([0-9]{8}T[0-9]{6}Z)\.tar\.gz"
    )
    require(
        matches(receipt["rollback"]["path"], pattern)
        and sha(receipt["rollback"]["sha256"], 64)
    )
    label = re.fullmatch(pattern, receipt["rollback"]["path"]).group(1)
    rollback_epoch = int(
        datetime.strptime(label, "%Y%m%dT%H%M%SZ")
        .replace(tzinfo=timezone.utc)
        .timestamp()
    )
    require(times[1] <= rollback_epoch <= times[2])


def evaluate(value: dict, validate_host: bool = True) -> dict[str, str]:
    exact(
        value,
        "schema_version artifact_status collected_at repository host_evidence host_canonical_sha256 deployment_receipt github assertions limitations",
    )
    require(
        value["schema_version"] == "1.0"
        and value["artifact_status"] == "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE"
    )
    require(
        value["limitations"] == LIMITATIONS
        and sha(value["host_canonical_sha256"], 64)
        and value["host_canonical_sha256"] == digest(canonical(value["host_evidence"]))
    )
    cutoff = timestamp(value["collected_at"], datetime.now(timezone.utc))
    repo = value["repository"]
    require(
        isinstance(repo, dict)
        and matches(repo.get("full_name"), r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")
    )
    identity(repo, repo["full_name"])
    host = value["host_evidence"]
    if validate_host:
        verify_host(host)
    timestamp(host["collected_at"], cutoff)
    result = {
        key: "UNAVAILABLE"
        for key in (
            "MERGED_PR_RELEASE_BOUND",
            "PR_HEAD_WORKFLOWS_BEFORE_MERGE",
            "MAIN_RELEASE_WORKFLOWS",
            "PRE_DEPLOY_BACKUP_AND_ROLLBACK_BOUND",
            "INDEPENDENT_REVIEW",
            "DEPLOYMENT_APPROVAL",
            "ENFORCED_PROTECTION",
            "ARTIFACT_AUTHENTICITY",
            "DURABLE_RETENTION",
            "CHECKOUT_ATTESTATION",
        )
    }
    result["HOST_POST_DEPLOY_CHECKS"] = (
        "PASS" if host["status"] == "COMPLETE_REVIEW_REQUIRED" else "FAIL"
    )
    github = value["github"]
    require(isinstance(github, dict))
    if github.get("status") == "UNAVAILABLE":
        exact(github, "status reason")
        require(github["reason"] == "GITHUB_LINEAGE_UNAVAILABLE")
        require(value["deployment_receipt"] is None, "UNBOUND_DEPLOYMENT_RECEIPT")
        return result
    exact(github, "method api_version repository pr merge_commit pr_runs main_runs")
    identity(github["repository"], repo["full_name"])
    require(
        github["method"] == "GET"
        and github["api_version"] == API_VERSION
        and github["repository"] == repo
    )
    pr = github["pr"]
    validate_pr(pr, repo, host["release_sha"], cutoff)
    validate_commit(github["merge_commit"], pr, repo["full_name"])
    validate_runs(
        github["pr_runs"],
        repo,
        pr["head"]["sha"],
        "pull_request",
        pr["head"]["ref"],
        cutoff,
    )
    validate_runs(
        github["main_runs"], repo, host["release_sha"], "push", "main", cutoff
    )
    require(
        not (
            {row["id"] for row in github["pr_runs"]["runs"]}
            & {row["id"] for row in github["main_runs"]["runs"]}
        )
    )
    merged = timestamp(pr["merged_at"], cutoff)
    require(merged <= timestamp(host["collected_at"], cutoff))
    require(
        all(
            timestamp(row["created_at"], cutoff) >= merged
            for row in github["main_runs"]["runs"]
        )
    )
    result["MERGED_PR_RELEASE_BOUND"] = "PASS"
    result["PR_HEAD_WORKFLOWS_BEFORE_MERGE"] = workflow_result(
        github["pr_runs"], merged
    )
    result["MAIN_RELEASE_WORKFLOWS"] = workflow_result(github["main_runs"])
    if value["deployment_receipt"] is not None:
        validate_receipt(value["deployment_receipt"], host, merged, cutoff)
        result["PRE_DEPLOY_BACKUP_AND_ROLLBACK_BOUND"] = "PASS"
    return result


def collect(repository: str, number: int, host: Any, receipt: Any) -> dict:
    require(
        matches(repository, r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")
        and positive_integer(number)
    )
    verify_host(host)
    repo = read_json(f"repos/{repository}", "{id,full_name}")
    identity(repo, repository)
    pr_endpoint = f"repos/{repository}/pulls/{number}"
    try:
        pr = read_json(pr_endpoint, PR_PROJECTION)
        cutoff = datetime.now(timezone.utc)
        validate_pr(pr, repo, host["release_sha"], cutoff)
        require(pr["number"] == number)
        commit = read_json(
            f"repos/{repository}/git/commits/{host['release_sha']}",
            "{sha,url,parents:[.parents[]|{sha,url}]}",
        )
        validate_commit(commit, pr, repository)
        pr_runs = collect_runs(repository, pr["head"]["sha"], "pull_request")
        main_runs = collect_runs(repository, host["release_sha"], "push")
        # Compare complete projected populations twice, not just a total count.
        require(
            pr_runs == collect_runs(repository, pr["head"]["sha"], "pull_request"),
            "RUN_INVENTORY_CHANGED",
        )
        require(
            main_runs == collect_runs(repository, host["release_sha"], "push"),
            "RUN_INVENTORY_CHANGED",
        )
        require(pr == read_json(pr_endpoint, PR_PROJECTION), "PR_CHANGED")
        github = {
            "method": "GET",
            "api_version": API_VERSION,
            "repository": repo,
            "pr": pr,
            "merge_commit": commit,
            "pr_runs": pr_runs,
            "main_runs": main_runs,
        }
    except (ValueError, TypeError, KeyError):
        # No raw API error or partial payload is persisted.
        github = {"status": "UNAVAILABLE", "reason": "GITHUB_LINEAGE_UNAVAILABLE"}
        receipt = None
    value = {
        "schema_version": "1.0",
        "artifact_status": "REVIEW_DRAFT_NOT_OPERATING_EVIDENCE",
        "collected_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "repository": repo,
        "host_evidence": host,
        "host_canonical_sha256": digest(canonical(host)),
        "deployment_receipt": receipt,
        "github": github,
        "assertions": {},
        "limitations": LIMITATIONS,
    }
    value["assertions"] = evaluate(value)
    return {**value, "payload_sha256": digest(canonical(value))}


def verify(value: Any) -> None:
    require(isinstance(value, dict) and sha(value.get("payload_sha256"), 64))
    payload = {k: v for k, v in value.items() if k != "payload_sha256"}
    require(
        digest(canonical(payload)) == value["payload_sha256"],
        "PAYLOAD_CHECKSUM_MISMATCH",
    )
    require(payload["assertions"] == evaluate(payload), "ASSERTION_MISMATCH")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    collect_parser = sub.add_parser("collect")
    collect_parser.add_argument("--repository", required=True)
    collect_parser.add_argument("--pr", type=int, required=True)
    collect_parser.add_argument("--host-evidence", type=Path, required=True)
    collect_parser.add_argument("--deployment-receipt", type=Path)
    collect_parser.add_argument("--output", type=Path, required=True)
    sub.add_parser("verify").add_argument("evidence", type=Path)
    args = parser.parse_args()
    try:
        if args.command == "verify":
            value, _ = read_file(args.evidence)
            verify(value)
            print(
                "Release evidence verification passed; internal consistency only, independent review still required."
            )
        else:
            host, _ = read_file(args.host_evidence)
            receipt = (
                read_file(args.deployment_receipt)[0]
                if args.deployment_receipt
                else None
            )
            value = collect(args.repository, args.pr, host, receipt)
            write_file(args.output, value)
            print(
                "Release evidence review draft saved; not operating-effectiveness evidence."
            )
        return 0
    except (OSError, ValueError, TypeError, KeyError, subprocess.TimeoutExpired):
        print(
            "Release evidence unavailable: input, source binding, output path, or verification failed. No raw source output is disclosed."
        )
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
