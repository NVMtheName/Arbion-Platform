# SOC 2 Evidence Collection Runbook

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
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

Authenticate the GitHub CLI to the Arbion repository and AWS CLI to the production account, then run `scripts/collect-soc2-external-evidence.sh` with an output directory outside the repository. The collector performs read-only API calls, omits subscription endpoints and secrets, writes restrictive local permissions, and creates a SHA-256 manifest. Review every collected file, record exceptions, and move the approved snapshot to the restricted evidence repository. A successful collection proves only what the saved APIs reported at that time; it does not prove operating effectiveness by itself.

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
