"""Strict, endpoint-free SNS configuration evidence; never a delivery receipt."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any


MAX_SUBSCRIPTIONS = 1000
TOPIC_ARN = re.compile(
    r"arn:aws:sns:([a-z]{2}(?:-[a-z]+)+-[0-9]+):([0-9]{12}):([A-Za-z0-9_-]{1,256})"
)
SUBSCRIPTION_ID = re.compile(r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")
PROTOCOLS = {
    "http",
    "https",
    "email",
    "email-json",
    "sms",
    "sqs",
    "application",
    "lambda",
    "firehose",
}
ATTRIBUTE_KEYS = {
    "TopicArn",
    "Owner",
    "SubscriptionsConfirmed",
    "SubscriptionsPending",
    "KmsMasterKeyId",
}
SUBSCRIPTION_KEYS = {"SubscriptionArn", "TopicArn", "Owner", "Protocol"}
DELIVERY = {
    "status": "UNAVAILABLE",
    "reason": "No end-to-end delivery receipt collected; configuration is not delivery evidence.",
}


def valid_selection(arn: Any, account: Any, region: Any) -> bool:
    match = TOPIC_ARN.fullmatch(arn) if isinstance(arn, str) else None
    return bool(match and match.group(1) == region and match.group(2) == account)


def valid_topic(value: Any, role: str, account: str, region: str) -> bool:
    if not isinstance(value, dict) or set(value) != {
        "schema_version",
        "role",
        "selection",
        "attributes",
        "subscriptions",
        "delivery_evidence",
    }:
        return False
    selection = value["selection"]
    attrs = value["attributes"]
    rows = value["subscriptions"]
    if not (
        value["schema_version"] == "1.0"
        and value["role"] == role
        and value["delivery_evidence"] == DELIVERY
        and isinstance(selection, dict)
        and set(selection) == {"source", "topic_arn", "other_role_topic_arn"}
        and selection["source"]
        in (
            "EXPLICIT_ARN",
            "DEFAULT_TARGET_DESIGN"
            if role == "SECURITY"
            else "DEFAULT_OPERATIONS_NAME",
        )
        and valid_selection(selection["topic_arn"], account, region)
        and valid_selection(selection["other_role_topic_arn"], account, region)
        and selection["topic_arn"] != selection["other_role_topic_arn"]
        and isinstance(attrs, dict)
        and set(attrs) == ATTRIBUTE_KEYS
        and attrs["TopicArn"] == selection["topic_arn"]
        and attrs["Owner"] == account
        and (
            attrs["KmsMasterKeyId"] is None
            or (
                isinstance(attrs["KmsMasterKeyId"], str)
                and 0 < len(attrs["KmsMasterKeyId"]) <= 2048
            )
        )
        and isinstance(rows, list)
        and len(rows) <= MAX_SUBSCRIPTIONS
    ):
        return False
    counts = []
    for key in ("SubscriptionsConfirmed", "SubscriptionsPending"):
        count = attrs[key]
        if not isinstance(count, str) or not re.fullmatch(r"0|[1-9][0-9]{0,3}", count):
            return False
        counts.append(int(count))
    confirmed: set[str] = set()
    pending = 0
    prefix = selection["topic_arn"] + ":"
    for row in rows:
        if not (
            isinstance(row, dict)
            and set(row) == SUBSCRIPTION_KEYS
            and row["TopicArn"] == selection["topic_arn"]
            and row["Owner"] == account
            and isinstance(row["Protocol"], str)
            and row["Protocol"] in PROTOCOLS
            and isinstance(row["SubscriptionArn"], str)
        ):
            return False
        arn = row["SubscriptionArn"]
        if arn == "PendingConfirmation":
            pending += 1
        elif (
            not arn.startswith(prefix)
            or not SUBSCRIPTION_ID.fullmatch(arn[len(prefix) :])
            or arn in confirmed
        ):
            return False
        else:
            confirmed.add(arn)
    return counts == [len(confirmed), pending]


def no_duplicates(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("Ambiguous JSON")
        result[key] = value
    return result


def read_projection(path: str) -> Any:
    with Path(path).open("rb") as handle:
        raw = handle.read(2_097_153)
    if not raw or len(raw) > 2_097_152:
        raise ValueError("Unbounded projection")
    return json.loads(raw, object_pairs_hook=no_duplicates)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--role", choices=("SECURITY", "OPERATIONS"), required=True)
    parser.add_argument("--source", required=True)
    parser.add_argument("--topic", required=True)
    parser.add_argument("--other-topic", required=True)
    parser.add_argument("--account", required=True)
    parser.add_argument("--region", required=True)
    parser.add_argument("--attributes")
    parser.add_argument("--subscriptions")
    args = parser.parse_args()
    if not (
        valid_selection(args.topic, args.account, args.region)
        and valid_selection(args.other_topic, args.account, args.region)
        and args.topic != args.other_topic
    ):
        return 1
    if args.attributes is None and args.subscriptions is None:
        return 0  # Preflight selection before any SNS API request.
    try:
        attrs = read_projection(args.attributes)
        subscriptions = read_projection(args.subscriptions)
        if not isinstance(subscriptions, dict) or set(subscriptions) != {
            "Subscriptions",
            "NextToken",
        }:
            return 1
        if subscriptions["NextToken"] is not None:
            return 1  # A capped or incomplete inventory cannot prove a count.
        value = {
            "schema_version": "1.0",
            "role": args.role,
            "selection": {
                "source": args.source,
                "topic_arn": args.topic,
                "other_role_topic_arn": args.other_topic,
            },
            "attributes": attrs,
            "subscriptions": subscriptions["Subscriptions"],
            "delivery_evidence": DELIVERY,
        }
        if not valid_topic(value, args.role, args.account, args.region):
            return 1
        print(json.dumps(value, sort_keys=True, indent=2))
        return 0
    except (OSError, ValueError, TypeError):
        return 1  # Never emit raw response bodies or notification endpoints.


if __name__ == "__main__":
    sys.exit(main())
