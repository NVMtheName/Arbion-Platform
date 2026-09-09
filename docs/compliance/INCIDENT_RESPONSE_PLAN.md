# Incident Response Plan

| Field | Value |
| --- | --- |
| Plan owner | Incident Response Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Exercise cadence | At least annually |

## Priorities

Protect people and customer assets, stop unauthorized access, preserve evidence, maintain trustworthy communication, restore service safely, and learn without deleting failed history. A suspected broker-write or unauthorized financial action is always critical even if impact is not yet confirmed.

## Severity

- **SEV-1:** confirmed or credible risk of credential compromise, unauthorized customer-data access, unauthorized financial action, destructive loss, or widespread production outage.
- **SEV-2:** material degradation, exploitable high-risk weakness, or limited confidential-data exposure with functioning containment.
- **SEV-3:** contained low-impact event or control failure with no evidence of customer impact.

## Response

1. **Detect and record:** open a restricted incident record with UTC time, reporter, affected systems, evidence sources, and initial severity. Do not paste secrets or customer holdings into general channels.
2. **Assign:** name an Incident Commander, technical lead, communications owner, and scribe. Escalate SEV-1 immediately.
3. **Contain:** revoke affected sessions/credentials, isolate workloads or accounts, stop unsafe automation, and preserve logs/snapshots. Do not destroy evidence.
4. **Investigate:** establish scope and timeline, identify affected subjects and data, validate audit integrity, and distinguish model/provider output from deterministic platform actions.
5. **Communicate:** legal and management determine regulatory, contractual, vendor, insurer, law-enforcement, and customer notifications. Record the decision and deadlines.
6. **Eradicate and recover:** remove the cause, rotate affected secrets, restore through approved change control, validate data and controls, monitor for recurrence, and document the recovery point/time.
7. **Review:** within five business days for SEV-1/2, document cause, control gaps, response effectiveness, actions, owners, and dates. Track actions to verified closure.

## Contact and evidence appendix

Named contacts, phone numbers, insurer/counsel details, customer notification templates, AWS/GitHub escalation paths, and evidence-repository locations belong in a restricted operational appendix, not this public repository. Test the plan annually and after material system change.
