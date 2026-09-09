# SOC 2 Readiness and Gap Assessment

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Assessment basis | Repository and declared production architecture as of this revision |
| Program status | READINESS_BASELINE_NOT_CERTIFIED |

## Outcome

Arbion has a strong technical-control foundation, but it must not claim SOC 2 compliance or certification yet. A SOC 2 report is issued only after an independent CPA evaluates the control design and, for Type II, operating effectiveness over an agreed observation period.

This baseline targets the Security, Availability, and Confidentiality criteria. The canonical control inventory is `compliance/soc2-controls.json`; CI validates that the inventory, policy set, secure-development gates, and evidence pointers remain present and internally consistent.

## Existing control strengths

- Strong authentication boundaries, Argon2id password hashing, opaque server-side sessions, trusted-origin checks, optional TOTP MFA, rate limits, and owner-scoped authorization.
- Provider credentials encrypted at rest, separated from the Neural Engine, excluded from audit payloads, and retrieved only at the authorized adapter boundary.
- Private application and data tiers in the AWS target architecture, TLS in transit, KMS-backed encryption, least-privilege task roles, immutable image tags, and disabled ECS shell access.
- Multi-AZ database/cache design, service health checks, automatic rollback, capacity alarms, encrypted backups, backup-freshness monitoring, and restore verification tooling.
- Immutable security, decision, scheduler, risk, Paper, and Shadow evidence; no live broker-write adapter; deterministic fail-closed financial controls.
- Automated tests, dependency vulnerability checks, infrastructure validation, manual production deployment gates, release hashing, rollback packages, and production smoke checks.

## Gaps that block an audit-ready claim

| Priority | Gap | Required completion evidence |
| --- | --- | --- |
| P0 | Policies have not yet been approved and named owners have not been assigned | Dated management approvals and owner roster |
| P0 | No completed organization-wide risk assessment | Approved risk register with likelihood, impact, treatment, owner, and due date |
| P0 | GitHub branch/ruleset, review, security-feature, and production-environment settings are not authenticated and archived as evidence | Run the read-only external collector and retain exports showing enforced PR review, required checks, no force push/delete, code scanning, secret scanning/push protection, and production reviewers |
| P0 | Workforce access lifecycle is not evidenced | Joiner/mover/leaver records, MFA evidence, quarterly access reviews, and terminated-access samples |
| P0 | Incident response and disaster recovery have not completed observed exercises | Tabletop report and successful restore/recovery report with follow-up items |
| P0 | Vendor and subservice-organization due diligence is incomplete | Current inventory, risk tier, contract/DPA, security report review, and reassessment dates |
| P1 | Evidence retention location and auditor observation window are not formally established | Restricted evidence repository, retention settings, examination dates, and evidence index |
| P1 | Data retention/deletion schedule needs management and legal approval plus production verification | Approved schedule and sampled deletion/retention evidence |
| P1 | Production cloud configuration has not been independently compared with infrastructure-as-code in this assessment | Run the read-only external collector after authenticating, then retain the dated AWS export, drift review, IAM review, and remediation record |
| P1 | Business commitments, availability objective, and incident notification terms require legal/management approval | Approved customer commitments and control mapping |

## Technical baseline added by this milestone

- Immutable commit pinning for every third-party GitHub Action.
- Immutable registry-digest pinning for application build bases and production service images, with automated update monitoring.
- CodeQL scanning for Go, TypeScript/JavaScript, Python, and workflow code.
- Pull-request dependency review that blocks newly introduced moderate-or-higher known vulnerabilities.
- Reproducible builds and weekly/change-triggered scans that reject fixable high-or-critical operating-system and library vulnerabilities in all three production application images.
- Independent current-tree secret detection and infrastructure-as-code scanning that reject detected secrets and unaccepted high-or-critical configuration findings on every change and weekly.
- Hash-locked runtime and test dependency closures for the Python Neural Engine; the production image and CI reject packages whose downloaded artifacts do not match the reviewed hashes, and CI audits that exact locked set.
- Dependabot coverage for application, AI, API, container, workflow, and Terraform dependencies.
- Code ownership for the repository and heightened ownership of authentication, credentials, risk, live-execution, deployment, infrastructure, and compliance paths.
- A public vulnerability disclosure policy.
- A machine-validated control catalog and CI evidence artifact.
- Read-only filesystems, dropped Linux capabilities, no-new-privileges, bounded process counts, and explicit init handling for production application containers.
- A fail-closed Lightsail deployment gate that completes and verifies a new encrypted off-host PostgreSQL backup before production code is replaced, then emits the prior release, rollback artifact, and deployed release for the change record.
- Target AWS infrastructure definitions for multi-region validated CloudTrail, customer-KMS-encrypted object-locked audit storage, customer-KMS-encrypted CloudTrail and security logs, customer-KMS-encrypted alert delivery, AWS Config recording, GuardDuty, Access Analyzer, CloudTrail security-event alarms, and medium-or-higher GuardDuty finding delivery. These controls remain configuration evidence—not production claims—until an authenticated plan/apply and post-apply verification are retained.
- A credential-free-in-Git, read-only external evidence collector for GitHub and AWS configuration snapshots. Its output must be written outside the repository and retained in the restricted evidence store.
- A read-only production-host evidence snapshot that reports only release identity, container hardening and health, monitoring-timer state, backup-marker freshness, sensitive-file ownership/modes, and existing health-check results; it never reads environment values, logs, or application/database records.

These controls improve readiness but do not replace management operation, evidence collection, auditor scoping, or the independent examination.

## Exit criteria for Type II readiness

1. Management approves the policies, scope, criteria, system description, control owners, and risk register.
2. Every `OPERATIONAL_ACTION_REQUIRED`, `EXTERNAL_CONFIGURATION_REQUIRED`, or `PARTIAL` catalog entry has an approved remediation or auditor-accepted treatment.
3. GitHub, AWS, production host, database, backup, alerting, and vendor configurations are tested and archived without secrets.
4. Access reviews, vulnerability remediation, changes, incidents, backup restores, availability monitoring, and vendor reviews operate for the auditor-agreed period.
5. Exceptions are risk accepted by authorized management with expiry and compensating controls.
6. An independent CPA completes the examination and issues the report.
