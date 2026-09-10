"""Read-only, bounded GitHub security metadata; no findings or license inference."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
import re
import subprocess
from pathlib import Path
from typing import Any


API_VERSION = "2022-11-28"
MAX_ANALYSES = 500
PAGE_SIZE = 100
CATEGORIES = [
    "/language:actions",
    "/language:go",
    "/language:javascript-typescript",
    "/language:python",
]
ANALYSIS_KEYS = {
    "id",
    "ref",
    "commit_sha",
    "category",
    "analysis_key",
    "created_at",
    "url",
    "tool",
    "rules_count",
    "error_state",
    "warning_state",
}
ANALYSIS_PROJECTION = """[.[] | {id,ref,commit_sha,category,analysis_key,created_at,url,rules_count,
  tool:{name:.tool.name,version:.tool.version},
  error_state:(if (.error|type)!="string" then "UNAVAILABLE" elif .error=="" then "NONE" else "PRESENT" end),
  warning_state:(if (.warning|type)!="string" then "UNAVAILABLE" elif .warning=="" then "NONE" else "PRESENT" end)}]"""
HEAD_PROJECTION = "{ref,url,object:{sha:.object.sha,type:.object.type}}"


class EvidenceUnavailable(ValueError):
    """Safe classification with no raw API body, credentials, or finding text."""


def positive_integer(value: Any) -> bool:
    return type(value) is int and value > 0


def text_field(value: Any) -> bool:
    return isinstance(value, str) and 0 < len(value) <= 512


def saved_time(value: Any) -> datetime | None:
    if not isinstance(value, str) or not re.fullmatch(
        r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", value
    ):
        return None
    try:
        return datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=timezone.utc
        )
    except ValueError:
        return None


def repository_identity(record: Any, repository: str) -> dict[str, Any]:
    if not (
        isinstance(record, dict)
        and positive_integer(record.get("id"))
        and record.get("full_name") == repository
        and re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository)
    ):
        raise EvidenceUnavailable("REPOSITORY_IDENTITY_UNAVAILABLE")
    return {"id": record["id"], "full_name": repository}


def binding(value: Any, record: Any, summary: Any) -> bool:
    try:
        identity = repository_identity(record, summary["repository"])
        observed = saved_time(value["collected_at"])
        return bool(
            value["schema_version"] == "1.0"
            and value["repository"] == identity
            and value["collected_at"] == summary["collected_at"]
            and observed
            and observed <= datetime.now(timezone.utc)
        )
    except (EvidenceUnavailable, KeyError, TypeError):
        return False


def valid_alerts(value: Any, record: Any, summary: Any) -> bool:
    return bool(
        isinstance(value, dict)
        and set(value)
        == {
            "schema_version",
            "repository",
            "collected_at",
            "endpoint",
            "method",
            "api_version",
            "http_status",
        }
        and binding(value, record, summary)
        and value["endpoint"] == f"repos/{record['full_name']}/vulnerability-alerts"
        and value["method"] == "GET"
        and value["api_version"] == API_VERSION
        and type(value["http_status"]) is int
        and value["http_status"] == 204
    )


def valid_head(value: Any, repository: str) -> bool:
    return bool(
        isinstance(value, dict)
        and set(value) == {"ref", "url", "object"}
        and value["ref"] == "refs/heads/main"
        and value["url"]
        == f"https://api.github.com/repos/{repository}/git/refs/heads/main"
        and isinstance(value["object"], dict)
        and set(value["object"]) == {"sha", "type"}
        and value["object"]["type"] == "commit"
        and isinstance(value["object"]["sha"], str)
        and re.fullmatch(r"[0-9a-f]{40}", value["object"]["sha"])
    )


def valid_scanning(value: Any, record: Any, summary: Any) -> bool:
    if not (
        isinstance(value, dict)
        and set(value)
        == {
            "schema_version",
            "repository",
            "collected_at",
            "endpoint",
            "method",
            "api_version",
            "scope",
            "head_before",
            "head_after",
            "page_counts",
            "analyses",
        }
        and binding(value, record, summary)
    ):
        return False
    repository = record["full_name"]
    pages = value["page_counts"]
    rows = value["analyses"]
    if not (
        value["endpoint"] == f"repos/{repository}/code-scanning/analyses"
        and value["method"] == "GET"
        and value["api_version"] == API_VERSION
        and value["scope"]
        == {
            "ref": "refs/heads/main",
            "tool": "CodeQL",
            "categories": CATEGORIES,
            "maximum_analyses": MAX_ANALYSES,
            "page_size": PAGE_SIZE,
        }
        and valid_head(value["head_before"], repository)
        and value["head_before"] == value["head_after"]
        and isinstance(pages, list)
        and 1 <= len(pages) <= 6
        and all(type(count) is int and 0 <= count <= PAGE_SIZE for count in pages)
        and all(count == PAGE_SIZE for count in pages[:-1])
        and pages[-1] < PAGE_SIZE
        and isinstance(rows, list)
        and len(rows) == sum(pages) <= MAX_ANALYSES
    ):
        return False
    previous = saved_time(value["collected_at"])
    ids: set[int] = set()
    for row in rows:
        if not (
            isinstance(row, dict)
            and set(row) == ANALYSIS_KEYS
            and positive_integer(row["id"])
            and row["id"] not in ids
            and row["url"]
            == f"https://api.github.com/repos/{repository}/code-scanning/analyses/{row['id']}"
            and row["ref"] == "refs/heads/main"
            and isinstance(row["commit_sha"], str)
            and re.fullmatch(r"[0-9a-f]{40}", row["commit_sha"])
            and text_field(row["category"])
            and text_field(row["analysis_key"])
            and isinstance(row["tool"], dict)
            and set(row["tool"]) == {"name", "version"}
            and row["tool"]["name"] == "CodeQL"
            and text_field(row["tool"]["version"])
            and type(row["rules_count"]) is int
            and row["rules_count"] >= 0
            and row["error_state"] in ("NONE", "PRESENT", "UNAVAILABLE")
            and row["warning_state"] in ("NONE", "PRESENT", "UNAVAILABLE")
        ):
            return False
        observed = saved_time(row["created_at"])
        if not observed or not previous or observed > previous:
            return False
        previous = observed
        ids.add(row["id"])
    return True


def current_codeql(values: list[Any]) -> tuple[bool, bool]:
    value, record, summary = values
    if not valid_scanning(value, record, summary):
        return False, False
    latest: dict[str, Any] = {}
    for row in value["analyses"]:
        category = row["category"]
        if category not in CATEGORIES:
            continue
        if category in latest:
            if latest[category]["created_at"] == row["created_at"]:
                return (
                    False,
                    False,
                )  # No tie-breaking guess about which result is latest.
        else:
            latest[category] = row
    if set(latest) != set(CATEGORIES) or any(
        row["commit_sha"] != value["head_before"]["object"]["sha"]
        or row["analysis_key"] != ".github/workflows/security.yml:codeql"
        or "UNAVAILABLE" in (row["error_state"], row["warning_state"])
        for row in latest.values()
    ):
        return False, False
    return True, all(
        row["rules_count"] > 0
        and row["error_state"] == "NONE"
        and row["warning_state"] == "NONE"
        for row in latest.values()
    )


def no_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise EvidenceUnavailable("AMBIGUOUS_JSON")
        result[key] = value
    return result


def decode(raw: str) -> Any:
    if not raw or len(raw.encode("utf-8")) > 2_097_152:
        raise EvidenceUnavailable("UNBOUNDED_RESPONSE")
    try:
        return json.loads(raw, object_pairs_hook=no_duplicates)
    except json.JSONDecodeError as error:
        raise EvidenceUnavailable("MALFORMED_RESPONSE") from error


def request(
    endpoint: str, projection: str | None = None
) -> subprocess.CompletedProcess[str]:
    args = [
        "gh",
        "api",
        "--hostname",
        "github.com",
        "--method",
        "GET",
        "-H",
        f"X-GitHub-Api-Version: {API_VERSION}",
        endpoint,
    ]
    args += ["--jq", projection] if projection else ["--include", "--silent"]
    try:
        return subprocess.run(
            args, capture_output=True, text=True, timeout=45, check=False
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        raise EvidenceUnavailable("READ_UNAVAILABLE") from error


def read_json(endpoint: str, projection: str) -> Any:
    result = request(endpoint, projection)
    if result.returncode != 0:
        raise EvidenceUnavailable("READ_UNAVAILABLE")
    return decode(result.stdout)


def collect(
    kind: str, record: Any, repository: str, collected_at: str
) -> dict[str, Any]:
    identity = repository_identity(record, repository)
    common = {
        "schema_version": "1.0",
        "repository": identity,
        "collected_at": collected_at,
        "method": "GET",
        "api_version": API_VERSION,
    }
    summary = {"repository": repository, "collected_at": collected_at}
    if kind == "alerts":
        endpoint = f"repos/{repository}/vulnerability-alerts"
        result = request(endpoint)
        statuses = re.findall(
            r"^HTTP/[0-9.]+ ([0-9]{3})(?:[ \r\n]|$)", result.stdout, re.MULTILINE
        )
        if result.returncode != 0 or statuses != ["204"]:
            # 404 can also mask access: never convert it into a disabled-control claim.
            raise EvidenceUnavailable("ALERT_ENABLEMENT_NOT_CONFIRMED")
        value = {**common, "endpoint": endpoint, "http_status": 204}
        if not valid_alerts(value, record, summary):
            raise EvidenceUnavailable("ALERT_BINDING_INVALID")
        return value
    endpoint = f"repos/{repository}/code-scanning/analyses"
    head_endpoint = f"repos/{repository}/git/ref/heads/main"
    before = read_json(head_endpoint, HEAD_PROJECTION)
    if not valid_head(before, repository):
        raise EvidenceUnavailable("HEAD_IDENTITY_UNAVAILABLE")
    rows: list[Any] = []
    pages: list[int] = []
    for page in range(1, 7):
        batch = read_json(
            f"{endpoint}?ref=refs%2Fheads%2Fmain&tool_name=CodeQL&per_page=100&page={page}",
            ANALYSIS_PROJECTION,
        )
        if not isinstance(batch, list) or len(batch) > PAGE_SIZE:
            raise EvidenceUnavailable("ANALYSIS_PAGE_INVALID")
        pages.append(len(batch))
        rows.extend(batch)
        if len(rows) > MAX_ANALYSES:
            raise EvidenceUnavailable("ANALYSIS_INVENTORY_CAPPED")
        if len(batch) < PAGE_SIZE:
            break
    value = {
        **common,
        "endpoint": endpoint,
        "scope": {
            "ref": "refs/heads/main",
            "tool": "CodeQL",
            "categories": CATEGORIES,
            "maximum_analyses": MAX_ANALYSES,
            "page_size": PAGE_SIZE,
        },
        "head_before": before,
        "head_after": read_json(head_endpoint, HEAD_PROJECTION),
        "page_counts": pages,
        "analyses": rows,
    }
    if not valid_scanning(value, record, summary):
        raise EvidenceUnavailable("ANALYSIS_IDENTITY_TIME_OR_INVENTORY_INVALID")
    return value


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=("alerts", "scanning"))
    parser.add_argument("--repository", required=True)
    parser.add_argument("--repository-record", required=True)
    parser.add_argument("--collected-at", required=True)
    args = parser.parse_args()
    try:
        with Path(args.repository_record).open(encoding="utf-8") as handle:
            record = decode(handle.read(2_097_153))
        value = collect(args.kind, record, args.repository, args.collected_at)
    except (OSError, ValueError, TypeError) as error:
        reason = (
            str(error)
            if isinstance(error, EvidenceUnavailable)
            else "SOURCE_UNAVAILABLE"
        )
        value = {
            "status": "UNAVAILABLE",
            "system": "GITHUB",
            "collected_at": args.collected_at,
            "reason": reason,
        }
    print(json.dumps(value, sort_keys=True, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
