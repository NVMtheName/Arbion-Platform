# Data Classification, Retention, and Disposal Policy

| Field | Value |
| --- | --- |
| Policy owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | At least annually and after data-use change |

## Classes

- **Restricted:** credentials, secret keys, session/MFA material, recovery codes, database backups, sensitive security evidence, and raw authentication artifacts.
- **Confidential:** customer identity, account/portfolio data, holdings, model inputs tied to an owner, non-public operations, contracts, and incident records.
- **Internal:** non-public procedures, risk registers, access reviews, vendor assessments, and architecture details not approved for publication.
- **Public:** approved website content, public policies, and deliberately published source/documentation that contains no higher-class data.

## Handling

Collect only data necessary for an approved service or control. Restricted and Confidential data requires TLS in transit, approved encryption at rest, owner-scoped authorization, least-privilege service access, and secret-free logs/support bundles. Production data must not be copied to development or AI training. Model/provider requests use minimized, purpose-bound fields and provider storage is disabled where supported.

## Retention schedule

Management and legal must approve exact periods before the audit window. At minimum, the schedule must define customer account data, credentials, application audit/security events, immutable trading evidence, email/security tokens, sessions, operational logs, backups, incident records, access/change/vendor evidence, and legal holds. Automated expiry and deletion must be tested against the approved schedule; backup expiry must be considered separately from primary deletion.

## Disposal and subject requests

Expired data is securely deleted or cryptographically rendered inaccessible using provider-supported methods, unless a documented legal hold applies. Disposal evidence records scope, authority, time, result, and reviewer without reproducing deleted content. Customer deletion/access requests are authenticated, tracked, reviewed for financial/legal retention duties, and completed within applicable commitments.

The repository and CI artifacts must contain synthetic data only. Any suspected committed secret is treated as an incident and rotated; deleting Git history alone is not sufficient containment.
