# External Control Verification

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.3 |
| Verification status | AUTHENTICATED_EVIDENCE_NOT_YET_COLLECTED |
| Review cadence | Quarterly and after material configuration change |

This checklist separates repository control design from settings that exist only in GitHub, AWS, the production host, and provider consoles. A checked-in workflow or Terraform resource is not proof that the corresponding external control is enabled or operating.

## Current constraint

The readiness review could not authenticate to GitHub or AWS. The configured GitHub CLI credential is invalid and the `arbion-admin` AWS SSO session is expired. No external setting is marked effective on that basis. Reauthenticate both CLIs, run the read-only collector described below, remediate every failed or unavailable item, and retain the reviewed output in the restricted evidence repository.

Public repository metadata observed during the review indicated that the repository is public and that web commit signoff is disabled. Public visibility is not itself a SOC 2 failure, but it raises the importance of secret scanning, push protection, protected changes, and rapid credential revocation. Authenticated evidence must establish the current state before the observation period begins.

## Collection procedure

1. Authenticate `gh` to the Arbion repository with read access to administration and security settings.
2. Authenticate the AWS CLI to the production account with read-only security-audit access.
3. Choose a new directory outside this Git repository on an encrypted workstation volume.
4. Export exact approved `ARBION_SECURITY_ALARM_TOPIC_ARN` and `ARBION_OPERATIONS_ALARM_TOPIC_ARN` selections where they differ from the documented names, then run `bash scripts/collect-soc2-external-evidence.sh <parent-directory>`. The collector records each role separately and never substitutes the operational topic for unavailable security controls. See the schema 1.1 contract in `EVIDENCE_RUNBOOK.md`.
5. Run `scripts/verify-soc2-evidence-snapshot.sh <snapshot-directory>`. Do not review or transfer a package that fails this internal-consistency boundary.
6. Run `scripts/review-soc2-external-evidence.py <snapshot-directory> <outside-directory>/external-control-review.json`. The output must remain outside the repository and snapshot. It is a deterministic `REVIEW_DRAFT_NOT_OPERATING_EVIDENCE`, not a control approval.
7. Independently compare every draft assertion with its cited JSON source and digest. Record `PASS`, `FAIL`, or `UNAVAILABLE`, the reviewer, UTC review time, and an exception for every failed or unavailable result. An `INCOMPLETE` collection or `INCOMPLETE_REVIEW_REQUIRED` draft is a control exception, not successful evidence.
8. Collect the current Lightsail host snapshot without copying its secret environment: `ssh <restricted-host> 'sudo bash /opt/arbion/scripts/collect-soc2-host-evidence.sh' > <outside-repository>/host.json`. A nonzero result or `INCOMPLETE` status is an exception. Run `scripts/verify-soc2-host-evidence.sh <outside-repository>/host.json`, independently review the result, and retain it promptly in restricted immutable or versioned storage. The embedded digest proves only internal payload consistency, not collector identity or authenticity.
9. Move the reviewed snapshots and draft to the restricted evidence repository, verify their SHA-256 identities, and retain the signed reviewer conclusion and exception links with them.

The collectors omit credential material, secret-scanning alert payloads, notification endpoints, customer data, application/database records, environment values, and CloudTrail event bodies. They do not change any setting. The host collector returns only the release marker, container hardening/health metadata, monitoring-timer state, sensitive-file ownership/modes, backup marker metadata, and results of existing read-only health checks. The external review draft makes no API call and evaluates only the verified saved snapshot; it preserves exact file digests but intentionally omits raw source values.

## Required GitHub state

The collector reads Actions defaults from `/actions/permissions/workflow`; the parent permissions endpoint is not evidence of token defaults. S3 Object Lock is evaluated inside the provider's `ObjectLockConfiguration` wrapper. GuardDuty inventory is paired with each detector's explicit `get-detector` status; missing or unpaired status remains unavailable, and a disabled or absent detector does not pass. Lightsail alarm evidence includes the monitored resource and must match the exact running `arbion-production-host` name and ARN. Older snapshots missing these fields remain unavailable; do not modify or infer values into saved history. See the [GitHub permissions API](https://docs.github.com/en/rest/actions/permissions#get-default-workflow-permissions-for-a-repository), [S3 Object Lock response](https://docs.aws.amazon.com/cli/latest/reference/s3api/get-object-lock-configuration.html), and [GuardDuty detector status](https://docs.aws.amazon.com/cli/latest/reference/guardduty/get-detector.html).

- `main` requires a pull request, code-owner review, dismissal of stale approvals, successful required checks, resolved conversations, and linear history; direct pushes, force pushes, branch deletion, and routine administrator bypass are blocked.
- The `production` environment allows deployment only from `main`, requires an authorized reviewer who cannot routinely bypass the gate, and retains deployment history.
- CodeQL/code scanning, Dependabot alerts and updates, secret scanning, and push protection are enabled and monitored. Open alerts have owners and due dates or time-bounded risk acceptance.
- GitHub Actions default permissions are read-only, workflows use OIDC rather than long-lived AWS keys, third-party actions remain commit-pinned, and untrusted workflow changes cannot obtain production credentials.
- Repository and organization administrators use unique identities and MFA. The quarterly access population is reviewed for least privilege and timely removal.

## Required AWS and production state

- The CLI identity, account, region, and collection time match the approved production boundary.
- Multi-region CloudTrail is logging, includes global management events, validates log files, exports to CloudWatch, and writes to the private object-locked audit bucket.
- AWS Config is recording supported resources and delivering snapshots; GuardDuty and account-level Access Analyzer are enabled.
- Medium-or-higher GuardDuty findings and CloudTrail security alarms route to the approved security topic, with at least one confirmed destination and a retained delivery test. Existing operational notifications are assessed separately. Exact configuration binding is not proof of delivery: the read-only review leaves delivery testing `UNAVAILABLE`. Topic policy permissions, encryption/key permissions, publication, receipt, and response require separate review and an authorized exercise before effectiveness is claimed. The collector uses the documented [SNS topic attributes](https://docs.aws.amazon.com/cli/latest/reference/sns/get-topic-attributes.html) and bounded [subscription inventory](https://docs.aws.amazon.com/cli/latest/reference/sns/list-subscriptions-by-topic.html), without endpoint or policy payloads.
- Audit and backup buckets block public access, require encryption and TLS, enable versioning, and retain protected objects for the approved period.
- Lightsail instance, external alarms, patch/reboot state, container health, public health, backup freshness, restore exercise, and restricted host access are reviewed separately because current production is owner-operated on Lightsail.
- The Terraform ECS/Fargate architecture is treated only as target design until a reviewed apply and post-apply evidence snapshot prove it is the production runtime.

## Required reviewer conclusion

The reviewer records one of `PASS`, `FAIL`, `UNAVAILABLE`, or `NOT_APPLICABLE` for every requirement, cites the source file and immutable identifier, and opens an exception for every result other than `PASS` or approved `NOT_APPLICABLE`. The review may not change the overall program status from `READINESS_BASELINE_NOT_CERTIFIED`; only management completion of the readiness program and an independent CPA examination can support an external SOC 2 assertion.
