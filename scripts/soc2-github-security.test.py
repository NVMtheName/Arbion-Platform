#!/usr/bin/env python3
"""Exact security metadata, bounded pagination, and no-inference regressions."""

from __future__ import annotations

import copy
import json
import subprocess
import unittest
from unittest.mock import patch

import soc2_github_security_evidence as evidence


REPOSITORY = "example/arbion"
RECORD = {"id": 123, "full_name": REPOSITORY}
COLLECTED_AT = "2026-09-09T16:00:00Z"
SUMMARY = {"repository": REPOSITORY, "collected_at": COLLECTED_AT}
HEAD = {
    "ref": "refs/heads/main",
    "url": f"https://api.github.com/repos/{REPOSITORY}/git/refs/heads/main",
    "object": {"type": "commit", "sha": "a" * 40},
}


def analysis(identifier: int, category: str = "/language:actions") -> dict:
    return {
        "id": identifier,
        "ref": "refs/heads/main",
        "commit_sha": "a" * 40,
        "url": f"https://api.github.com/repos/{REPOSITORY}/code-scanning/analyses/{identifier}",
        "category": category,
        "analysis_key": ".github/workflows/security.yml:codeql",
        "created_at": "2026-09-09T15:00:00Z",
        "tool": {"name": "CodeQL", "version": "2.27.0"},
        "rules_count": 20,
        "error_state": "NONE",
        "warning_state": "NONE",
    }


def scanning() -> dict:
    return {
        "schema_version": "1.0",
        "repository": RECORD,
        "collected_at": COLLECTED_AT,
        "endpoint": f"repos/{REPOSITORY}/code-scanning/analyses",
        "method": "GET",
        "api_version": evidence.API_VERSION,
        "scope": {
            "ref": "refs/heads/main",
            "tool": "CodeQL",
            "categories": evidence.CATEGORIES,
            "maximum_analyses": 500,
            "page_size": 100,
        },
        "head_before": HEAD,
        "head_after": HEAD,
        "page_counts": [4],
        "analyses": [
            analysis(i + 1, category) for i, category in enumerate(evidence.CATEGORIES)
        ],
    }


def response(value: object, code: int = 0) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess([], code, json.dumps(value), "")


class GitHubSecurityEvidenceTests(unittest.TestCase):
    def test_exact_current_main_analysis_contract(self) -> None:
        self.assertEqual(
            evidence.current_codeql([scanning(), RECORD, SUMMARY]), (True, True)
        )

    def test_unavailable_identity_time_inventory_and_private_fields(self) -> None:
        variants = [
            lambda v: v["repository"].update(id=999),
            lambda v: v.update(endpoint="repos/other/repo/code-scanning/analyses"),
            lambda v: v.update(method="POST"),
            lambda v: v.update(api_version="unrecorded"),
            lambda v: v.update(collected_at="2099-01-01T00:00:00Z"),
            lambda v: v.update(collected_at="2026-09-09T16:00:01Z"),
            lambda v: v["head_after"]["object"].update(sha="b" * 40),
            lambda v: v["head_before"].update(
                url="https://api.github.com/repos/other/repo/git/refs/heads/main"
            ),
            lambda v: v["scope"].update(maximum_analyses=999999),
            lambda v: v["scope"].update(categories=["/language:actions"]),
            lambda v: v.update(page_counts=[100]),
            lambda v: v.update(page_counts=[3, 1]),
            lambda v: v["analyses"][0].update(
                url="https://api.github.com/repos/other/repo/code-scanning/analyses/1"
            ),
            lambda v: v["analyses"][0].update(commit_sha="unknown"),
            lambda v: v["analyses"][0].update(ref="refs/pull/1/merge"),
            lambda v: v["analyses"][0].update(created_at="2026-09-09T16:00:01Z"),
            lambda v: v["analyses"][0].update(created_at="2026-02-30T00:00:00Z"),
            lambda v: v["analyses"][0].update(created_at="2026-09-08T00:00:00Z"),
            lambda v: v["analyses"][0].update(id=True),
            lambda v: v["analyses"][0].update(rules_count=True),
            lambda v: v["analyses"][0]["tool"].update(name="Different Scanner"),
            lambda v: v["analyses"][0].update(error="private error body"),
            lambda v: v["analyses"][0].update(warning="private warning body"),
            lambda v: v["analyses"][0].update(error_state="INFERRED_CLEAR"),
            lambda v: v["analyses"].append(v["analyses"][0]),
        ]
        for index, mutate in enumerate(variants):
            with self.subTest(index=index):
                value = copy.deepcopy(scanning())
                # Detach copies to test changed head identities independently.
                value["head_after"] = copy.deepcopy(value["head_after"])
                mutate(value)
                self.assertEqual(
                    evidence.current_codeql([value, RECORD, SUMMARY]), (False, False)
                )

    def test_missing_current_categories_old_commit_or_ambiguous_latest_is_unavailable(
        self,
    ) -> None:
        for case in ("empty", "missing", "old", "key", "tie", "unknown-error"):
            with self.subTest(case=case):
                value = copy.deepcopy(scanning())
                if case == "empty":
                    value["analyses"] = []
                if case == "missing":
                    value["analyses"].pop()
                if case == "old":
                    value["analyses"][0]["commit_sha"] = "b" * 40
                if case == "key":
                    value["analyses"][0]["analysis_key"] = (
                        ".github/workflows/unrelated.yml:codeql"
                    )
                if case == "tie":
                    value["analyses"].append(analysis(10))
                if case == "unknown-error":
                    value["analyses"][0]["error_state"] = "UNAVAILABLE"
                value["page_counts"] = [len(value["analyses"])]
                self.assertTrue(evidence.valid_scanning(value, RECORD, SUMMARY))
                self.assertEqual(
                    evidence.current_codeql([value, RECORD, SUMMARY]), (False, False)
                )

    def test_known_analysis_error_warning_or_no_rules_is_fail_not_pass(self) -> None:
        for field, saved in (
            ("error_state", "PRESENT"),
            ("warning_state", "PRESENT"),
            ("rules_count", 0),
        ):
            value = copy.deepcopy(scanning())
            value["analyses"][0][field] = saved
            self.assertEqual(
                evidence.current_codeql([value, RECORD, SUMMARY]), (True, False)
            )

    def test_vulnerability_alerts_requires_exact_success_response(self) -> None:
        for code, stdout, exit_code in (
            (204, "HTTP/2.0 204 No Content\r\nX-Safe: true\r\n\r\n", 0),
            (404, "HTTP/2.0 404 Not Found\r\n", 1),
            (403, "HTTP/2.0 403 Forbidden\r\n", 1),
            (200, "HTTP/2.0 200 OK\r\n", 0),
            (204, "HTTP/2.0 204 No Content\r\nHTTP/2.0 204 No Content\r\n", 0),
            (204, "", 0),
            (204, "HTTP/2.0 204 No Content\r\n", 1),
        ):
            with self.subTest(code=code, stdout=stdout, exit_code=exit_code):
                with patch.object(
                    evidence,
                    "request",
                    return_value=subprocess.CompletedProcess(
                        [], exit_code, stdout, "private stderr"
                    ),
                ):
                    if code == 204 and stdout.startswith(
                        "HTTP/2.0 204 No Content\r\nX-Safe"
                    ):
                        value = evidence.collect(
                            "alerts", RECORD, REPOSITORY, COLLECTED_AT
                        )
                        self.assertTrue(evidence.valid_alerts(value, RECORD, SUMMARY))
                        self.assertNotIn("private", json.dumps(value))
                        self.assertFalse(
                            evidence.valid_alerts(value, {**RECORD, "id": 999}, SUMMARY)
                        )
                        self.assertFalse(
                            evidence.valid_alerts(
                                {**value, "http_status": 200}, RECORD, SUMMARY
                            )
                        )
                    else:
                        with self.assertRaisesRegex(
                            evidence.EvidenceUnavailable,
                            "ALERT_ENABLEMENT_NOT_CONFIRMED",
                        ):
                            evidence.collect("alerts", RECORD, REPOSITORY, COLLECTED_AT)

    def test_complete_multi_page_inventory_and_stable_head(self) -> None:
        pages = [[analysis(i) for i in range(1, 101)], [analysis(101)]]
        with patch.object(
            evidence,
            "request",
            side_effect=[
                response(HEAD),
                response(pages[0]),
                response(pages[1]),
                response(HEAD),
            ],
        ) as request:
            value = evidence.collect("scanning", RECORD, REPOSITORY, COLLECTED_AT)
        self.assertEqual(value["page_counts"], [100, 1])
        self.assertEqual(len(value["analyses"]), 101)
        self.assertTrue(evidence.valid_scanning(value, RECORD, SUMMARY))
        endpoints = [call.args[0] for call in request.call_args_list]
        self.assertIn("page=1", endpoints[1])
        self.assertIn("page=2", endpoints[2])
        self.assertEqual(endpoints[0], endpoints[-1])

    def test_partial_capped_duplicate_or_changed_head_collection_fails_closed(
        self,
    ) -> None:
        page = [analysis(i) for i in range(1, 101)]
        changed_head = copy.deepcopy(HEAD)
        changed_head["object"]["sha"] = "b" * 40
        cases = [
            [response(HEAD), response(page), response({}, 1)],
            [response(HEAD), response(page), response([page[0]]), response(HEAD)],
            [response(HEAD), response([analysis(1)]), response(changed_head)],
            [response(HEAD)]
            + [
                response([analysis(i + p * 100) for i in range(1, 101)])
                for p in range(6)
            ],
        ]
        for calls in cases:
            with patch.object(evidence, "request", side_effect=calls):
                with self.assertRaises(evidence.EvidenceUnavailable):
                    evidence.collect("scanning", RECORD, REPOSITORY, COLLECTED_AT)

    def test_exact_500_rows_requires_empty_termination_page(self) -> None:
        calls = (
            [response(HEAD)]
            + [
                response([analysis(i + p * 100) for i in range(1, 101)])
                for p in range(5)
            ]
            + [response([]), response(HEAD)]
        )
        with patch.object(evidence, "request", side_effect=calls):
            value = evidence.collect("scanning", RECORD, REPOSITORY, COLLECTED_AT)
        self.assertTrue(evidence.valid_scanning(value, RECORD, SUMMARY))
        self.assertEqual(value["page_counts"], [100, 100, 100, 100, 100, 0])

    def test_api_boundary_is_get_only_pinned_and_projected(self) -> None:
        with patch.object(evidence.subprocess, "run", return_value=response([])) as run:
            evidence.request(
                "repos/example/arbion/code-scanning/analyses",
                evidence.ANALYSIS_PROJECTION,
            )
            args = run.call_args.args[0]
            self.assertEqual(args[args.index("--method") + 1], "GET")
            self.assertEqual(args[args.index("--hostname") + 1], "github.com")
            self.assertIn("--jq", args)
            self.assertIn("error_state", args[-1])
            self.assertNotIn("results_count", args[-1])
            evidence.request("repos/example/arbion/vulnerability-alerts")
            args = run.call_args.args[0]
            self.assertIn("--silent", args)
            self.assertIn("--include", args)
            self.assertEqual(run.call_args.kwargs["timeout"], 45)

    def test_repository_rebinding_or_duplicate_json_cannot_collect(self) -> None:
        with patch.object(evidence, "request") as request:
            with self.assertRaises(evidence.EvidenceUnavailable):
                evidence.collect("alerts", RECORD, "other/repo", COLLECTED_AT)
            request.assert_not_called()
        with self.assertRaises(evidence.EvidenceUnavailable):
            evidence.decode('{"id":1,"id":2}')


if __name__ == "__main__":
    unittest.main(verbosity=2)
