# Change Management Policy

| Field | Value |
| --- | --- |
| Policy owner | Engineering Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | At least annually and after pipeline change |

## Standard changes

1. Work is linked to a documented objective, risk, or defect and developed on a branch.
2. Automated formatting, static analysis, unit/integration tests, security scans, dependency review, and infrastructure validation pass.
3. A code owner reviews security-sensitive paths. The author may not use an administrative bypass as normal practice.
4. Changes merge through a protected pull request into `main`; force push and deletion of `main` are prohibited.
5. Production deployment uses the protected production environment and an exact immutable commit. Database migration precedes service replacement and failure stops the release.
6. A recoverable backup or verified rollback point exists before a material data or production change.
7. Readiness and smoke checks confirm the release. The exact commit, approvals, checks, deployment, and result are retained.

## Emergency changes

An emergency change is permitted only to contain an active incident or restore a materially unavailable service. It requires incident linkage, minimum necessary scope, available peer or management approval, preserved commands/results, immediate validation, and retrospective review within two business days. Emergency status never authorizes weakening deterministic trading controls or creating a live execution path.

## Segregation and exceptions

Where staffing prevents separate developer, reviewer, and deployer roles, management documents the limitation and applies compensating after-the-fact review of the diff, checks, release, and production result. Every bypass or failed deployment becomes reviewable evidence and cannot be erased from the record.
