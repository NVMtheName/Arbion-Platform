# SOC 2 Evidence Collection Runbook

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.4 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | Quarterly and after evidence-source change |

## Evidence repository

Use an access-controlled, encrypted repository with version history and retention covering the complete examination period plus legal and contractual requirements. Grant access by role, require MFA, review access quarterly, and log export/deletion. Do not put secrets, customer financial details, raw provider payloads, recovery codes, or private model content in Git.

Each evidence item must include control ID, UTC collection time, collector, source system, scoped population or sample, immutable identifier/checksum when available, result, exception link, and reviewer.

## Collection schedule

| Cadence | Evidence |
| --- | --- |
| Every change | Pull request, approvals, required checks, dependency review, CodeQL result, commit SHA, release artifact hash, deployment approval/result, migration result, smoke/readiness result, and rollback reference |
| Continuous/daily | Production health and alert status, security/audit events, scheduler outcomes, backup completion and freshness, vulnerability alerts, and cloud configuration/security findings |
| Monthly | Open vulnerability aging and remediation, patch status, incident/exception register, backup sample, capacity trend, and evidence completeness review |
| Quarterly | GitHub/AWS/production/database/provider access review, privileged-role review, vendor review delta, restore test, key/secret age review, and control-owner certification |
| Annually | Risk assessment, policy review/approval, incident tabletop, business-continuity exercise, vendor reassessment, system description review, and security training |

## Minimum evidence procedures

### External configuration snapshot

Authenticate the GitHub CLI to the Arbion repository and AWS CLI to the production account, then run `scripts/collect-soc2-external-evidence.sh` with an output directory outside the repository. The collector performs read-only API calls, omits subscription endpoints and secrets, writes restrictive local permissions, and creates a deterministically ordered SHA-256 manifest.

Before review or transfer, run `scripts/verify-soc2-evidence-snapshot.sh <snapshot-directory>`. The verifier fails closed on missing, extra, duplicate, reordered, malformed, oversized, symlinked, secret-like, checksum-mismatched, or internally inconsistent evidence.

After verification, create the bounded reviewer aid outside both the repository and snapshot with `scripts/review-soc2-external-evidence.py <snapshot-directory> <outside-directory>/external-control-review.json`. The collector paginates the GitHub collaborator population before this review. The reviewer aid re-runs package verification, rejects duplicate JSON keys, recomputes every source digest, refuses overwrite, writes mode `0600`, and maps 15 narrow saved-field assertions to the control catalog. Its deterministic output is always labeled `REVIEW_DRAFT_NOT_OPERATING_EVIDENCE`; `PASS` means only that a stated saved-field condition matched. A `FAIL` requires an exception and remediation review, while `UNAVAILABLE` requires recollection or a documented exception. The report cannot approve a control, establish operation over time, or support a compliance or certification claim by itself.

Keep the snapshot, manifest, and review draft together; independently review every source and assertion, record exceptions, and move the approved package into immutable or versioned storage in the restricted evidence repository. Successful verification establishes only internal package consistency at that point in time. A party able to alter both evidence and its local manifest can create a new internally consistent package, so authenticity depends on authenticated collection, independent review, access control, and immutable external retention. Collection, verification, and draft generation do not prove operating effectiveness or SOC 2 certification.

### Production host snapshot

Collect the current host snapshot through the restricted, authenticated administration channel described in `docs/compliance/EXTERNAL_CONTROL_VERIFICATION.md` and save the JSON outside the repository. The root-only collector reads control status but never environment values, credentials, logs, customer or application records, database content, or trading data. It reports a canonical collection identity, exact release marker, the six-service inventory, application-container hardening, the reverse-proxy-only network boundary, nine required monitoring timers, three sensitive-file permission records, backup freshness, six existing read-only checks, and an embedded SHA-256 digest over the canonical payload. A snapshot is `COMPLETE_REVIEW_REQUIRED` only when its release marker is valid, services, application hardening, network exposure, and timers pass, all three environment files are root-owned mode `0600`, read-only checks pass, and the backup marker is current.

Run `scripts/verify-soc2-host-evidence.sh <saved-host-evidence.json>` before review or retention. The verifier fails closed on malformed, oversized, symlinked, secret-like, future-dated, checksum-mismatched, duplicated, missing, or internally inconsistent host evidence. An `INCOMPLETE` snapshot can verify only as an internally consistent exception record; it does not pass the underlying controls and is never upgraded. The embedded digest detects accidental or unreconciled changes but is not a signature: anyone able to alter both payload and digest can reseal the file. Authenticity therefore depends on authenticated SSH collection, independent review, restricted access, and prompt immutable or versioned external retention. Verification never establishes operating effectiveness or SOC 2 certification.

### Change sample

Export the PR metadata, exact diff/commit, code-owner review, required checks, security scans, deployment environment approval, production release marker, backup identifier, and post-deploy health result. Preserve failed attempts and emergency classification.

### Access review

Export active users and roles from GitHub, AWS IAM/Identity Center, production hosts, database administration, DNS/domain, email, monitoring, and material provider portals. The reviewer confirms business need, least privilege, MFA, unique identity, and timely removal. Record exceptions and remediation dates.

### Vulnerability review

Export CodeQL, Dependabot, npm audit, govulncheck, pip-audit, image scanning, host patching, and cloud findings. Deduplicate by identifier, record severity and exposure, assign an owner and due date, and document remediation or risk acceptance.

### Backup and recovery

Retain backup job status, encrypted object identifier/version, checksum, freshness result, restore command output, restored database integrity checks, elapsed recovery time, and reviewer. Use isolated infrastructure and destroy the restored copy after evidence collection under the approved disposal process.

### Incident exercise

Use a plausible scenario involving credential exposure, unauthorized financial-data access, provider compromise, or production outage. Record participants, timeline, classification, containment, communication decision, recovery, evidence preservation, lessons, owners, and due dates.

### Vendor review

Record service, data accessed, criticality, hosting regions, subprocessors, security/privacy terms, breach notification, deletion/return terms, continuity dependency, latest independent assurance, exceptions, approval, and next review.

## Exceptions

Every failed or unavailable control produces an exception with risk, affected scope, compensating control, accountable owner, approval, due date, and expiry. Exceptions cannot silently change a catalog status or weaken the non-live execution boundary.
