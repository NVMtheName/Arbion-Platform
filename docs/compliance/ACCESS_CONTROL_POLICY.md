# Access Control Policy

| Field | Value |
| --- | --- |
| Policy owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | Quarterly and after material identity change |

## Principles

Access is deny-by-default, uniquely attributable, least-privileged, separated by environment and function, and limited to current business need. Shared interactive accounts are prohibited. Production credentials must not be used for routine development.

## Lifecycle

- An authorized owner approves access before provisioning and records the role, system, reason, scope, and expiry where temporary.
- Privileged GitHub, AWS, production host, database, DNS/domain, email, monitoring, financial-provider, and AI-provider access requires MFA where supported.
- Service workloads use separate non-human identities with the minimum permitted actions; long-lived cloud access keys are avoided in favor of workload identity or OIDC.
- Movers are reviewed before role changes. Terminated or no-longer-needed access is revoked immediately and no later than the same business day.
- Secrets are never sent in tickets, source control, chat, logs, or evidence exports. Emergency access is time-limited and reviewed after use.

## Reviews

Quarterly, an independent reviewer where practical compares active accounts and privileges to the approved owner roster. The review covers dormant accounts, administrators, service identities, deploy permissions, branch/ruleset bypass, production-environment bypass, database access, and third-party portals. Findings receive an owner and due date; completion is rechecked.

Application sessions are owner-scoped, revocable, bounded by configured expiry, and protected by server-side authorization. Sensitive security and future execution actions require appropriate step-up authentication. Authentication failures and security-sensitive changes generate secret-free audit evidence.
