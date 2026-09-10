# Arbion SOC 2 Readiness Program

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | At least annually and after material system or regulatory change |
| Classification | Public control description; never store sensitive audit evidence here |

This directory contains Arbion's auditable control baseline. It is a readiness program, not a representation that Arbion has completed an independent SOC 2 examination or received a SOC 2 report.

## Current scope

The intended examination boundary covers the production website, Go control plane, Python Neural Engine, PostgreSQL and Redis data services, production hosting and monitoring, release pipeline, backup process, and the people and vendors that operate those systems. The initial Trust Services Criteria scope is:

- Security (Common Criteria)
- Availability
- Confidentiality

Processing Integrity and Privacy require a documented scope decision with the independent auditor before an examination starts. Arbion's deterministic trading controls remain in scope as security-sensitive application controls even while live execution is unavailable.

## Canonical artifacts

- [Readiness and gap assessment](SOC2_READINESS.md)
- [System description](SYSTEM_DESCRIPTION.md)
- [Evidence collection runbook](EVIDENCE_RUNBOOK.md)
- [External control verification](EXTERNAL_CONTROL_VERIFICATION.md)
- [Operating evidence workbook](OPERATING_EVIDENCE_WORKBOOK.md)
- [Information security policy](INFORMATION_SECURITY_POLICY.md)
- [Access control policy](ACCESS_CONTROL_POLICY.md)
- [Change management policy](CHANGE_MANAGEMENT_POLICY.md)
- [Incident response plan](INCIDENT_RESPONSE_PLAN.md)
- [Business continuity and disaster recovery policy](BUSINESS_CONTINUITY_AND_DISASTER_RECOVERY.md)
- [Data classification, retention, and disposal policy](DATA_CLASSIFICATION_RETENTION_POLICY.md)
- [Vendor risk management policy](VENDOR_RISK_MANAGEMENT_POLICY.md)
- [Risk management policy](RISK_MANAGEMENT_POLICY.md)
- Machine-readable control catalog: `compliance/soc2-controls.json`

## Evidence rules

1. Evidence must identify the system, control, actor or service, UTC time, result, and immutable source identifier where available.
2. Secrets, customer financial details, MFA material, raw provider payloads, and private model content must never be committed to this repository.
3. Repository evidence is a pointer to a control implementation. Operating evidence must be retained in an access-controlled evidence repository for the auditor-agreed period.
4. Failed checks, incidents, exceptions, and remediations are retained; they are not rewritten into a clean history.
5. A control is not marked effective merely because a policy exists. It needs both suitable design and operating evidence across the examination period.

## Required approvals and external work

Management must approve this policy set, assign named control owners, complete the risk assessment, set the examination period, and engage an independent CPA firm. GitHub and AWS configuration evidence, access reviews, vendor reviews, incident exercises, and restore tests must be collected as described in the evidence runbook. Until those steps and the examination are complete, the only accurate status is **SOC 2 readiness in progress — not certified**.
