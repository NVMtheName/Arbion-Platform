# Information Security Policy

| Field | Value |
| --- | --- |
| Policy owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | At least annually |

## Purpose and scope

Arbion protects the confidentiality, integrity, and availability of production services, customer information, provider credentials, source code, and operational evidence. This policy applies to all personnel, systems, vendors, and environments that can affect the scoped service.

## Requirements

- Management assigns control owners, approves risk treatment, supplies adequate resources, and reviews control performance.
- Access follows least privilege, unique identity, MFA for privileged systems, prompt removal, and periodic review.
- Confidential data is minimized, encrypted in transit and at rest, never committed to source, and retained only for an approved purpose and period.
- Production changes follow the approved change process and leave review, test, release, deployment, and rollback evidence.
- Security events are logged without secrets, monitored, triaged, and retained under the approved evidence schedule.
- Vulnerabilities are inventoried, severity-ranked, assigned, remediated within approved targets, or explicitly risk accepted with expiry.
- Incidents follow the incident response plan. Availability and recovery follow the continuity and disaster-recovery policy.
- Vendors are assessed before use and periodically thereafter according to access and criticality.
- Arbion's model output is untrusted; only deterministic, owner-scoped controls may authorize platform behavior. No live financial execution is permitted until separately designed, approved, tested, and placed inside the assessed boundary.

## Review and exceptions

The owner reviews this policy and related evidence at least annually and after a significant incident or system change. Exceptions require documented risk, compensating controls, management approval, owner, due date, and expiry. Violations may result in access removal and incident handling.
