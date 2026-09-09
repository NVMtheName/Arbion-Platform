# External Control Verification

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
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
4. Run `bash scripts/collect-soc2-external-evidence.sh <parent-directory>`.
5. Review the generated `collection-summary.json` and every JSON source file. An `INCOMPLETE` status is a control exception, not a successful snapshot.
6. Collect the current Lightsail host snapshot without copying its secret environment: `ssh <restricted-host> 'sudo bash /opt/arbion/scripts/collect-soc2-host-evidence.sh' > <outside-repository>/host.json`. A nonzero result or `INCOMPLETE` status is an exception. Hash the file locally.
7. Move the reviewed snapshots to the restricted evidence repository, verify their SHA-256 manifests, and record control ID, reviewer, result, exception, and UTC review time.

The collectors omit credential material, secret-scanning alert payloads, notification endpoints, customer data, application/database records, environment values, and CloudTrail event bodies. They do not change any setting. The host collector returns only the release marker, container hardening/health metadata, monitoring-timer state, sensitive-file ownership/modes, backup marker metadata, and results of existing read-only health checks.

## Required GitHub state

- `main` requires a pull request, code-owner review, dismissal of stale approvals, successful required checks, resolved conversations, and linear history; direct pushes, force pushes, branch deletion, and routine administrator bypass are blocked.
- The `production` environment allows deployment only from `main`, requires an authorized reviewer who cannot routinely bypass the gate, and retains deployment history.
- CodeQL/code scanning, Dependabot alerts and updates, secret scanning, and push protection are enabled and monitored. Open alerts have owners and due dates or time-bounded risk acceptance.
- GitHub Actions default permissions are read-only, workflows use OIDC rather than long-lived AWS keys, third-party actions remain commit-pinned, and untrusted workflow changes cannot obtain production credentials.
- Repository and organization administrators use unique identities and MFA. The quarterly access population is reviewed for least privilege and timely removal.

## Required AWS and production state

- The CLI identity, account, region, and collection time match the approved production boundary.
- Multi-region CloudTrail is logging, includes global management events, validates log files, exports to CloudWatch, and writes to the private object-locked audit bucket.
- AWS Config is recording supported resources and delivering snapshots; GuardDuty and account-level Access Analyzer are enabled.
- Medium-or-higher GuardDuty findings and CloudTrail security alarms route to the approved operations topic, with at least one confirmed destination and a retained delivery test.
- Audit and backup buckets block public access, require encryption and TLS, enable versioning, and retain protected objects for the approved period.
- Lightsail instance, external alarms, patch/reboot state, container health, public health, backup freshness, restore exercise, and restricted host access are reviewed separately because current production is owner-operated on Lightsail.
- The Terraform ECS/Fargate architecture is treated only as target design until a reviewed apply and post-apply evidence snapshot prove it is the production runtime.

## Required reviewer conclusion

The reviewer records one of `PASS`, `FAIL`, `UNAVAILABLE`, or `NOT_APPLICABLE` for every requirement, cites the source file and immutable identifier, and opens an exception for every result other than `PASS` or approved `NOT_APPLICABLE`. The review may not change the overall program status from `READINESS_BASELINE_NOT_CERTIFIED`; only management completion of the readiness program and an independent CPA examination can support an external SOC 2 assertion.
