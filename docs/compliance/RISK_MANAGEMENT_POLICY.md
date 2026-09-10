# Risk Management Policy

| Field | Value |
| --- | --- |
| Policy owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Assessment cadence | At least annually and after significant change |

## Method

Identify assets, commitments, threats, vulnerabilities, fraud scenarios, vendor dependencies, legal obligations, and significant changes across the scoped system. Rate inherent likelihood and impact on an approved scale, document existing controls, calculate residual risk, and choose mitigation, transfer, avoidance, or time-bounded acceptance.

Every risk record includes a unique ID, description, affected criteria/system/data, inherent rating, control links, residual rating, treatment, accountable owner, due date, status, evidence, approver, and next review. Critical risks and overdue high risks are reported to management promptly.

## Required scenarios

The assessment includes account takeover, privileged misuse, secret exposure, cross-owner data access, provider compromise, prompt/content injection, inaccurate or stale financial data, unauthorized broker action, deterministic-control bypass, dependency/build compromise, database loss/corruption, backup failure, ransomware, denial of service, vendor outage, insider threat, and regulatory/contractual change.

## Change and exception review

Reassess risk before enabling live trading, adding a write-capable provider, materially changing identity or hosting, expanding data use, onboarding a critical vendor, or changing service commitments. Policy/control exceptions are risk records with compensating controls, approval, and expiry; they are not permanent silent waivers.

## Design exceptions pending management approval

These repository-declared exceptions are inputs to the formal risk register. They are not approved merely because the implementation requires them. Management must accept, replace, or remediate each before its expiry and retain that decision as operating evidence.

| Exception | Design need | Compensating controls | Expiry |
| --- | --- | --- | --- |
| NET-EX-01 | The website, API, and Neural Engine need outbound access to provider APIs and package endpoints whose public addresses are not stable enough for IP allowlisting. | Egress is limited to TCP 443; service-to-service and data-tier paths use separately scoped security-group rules; provider authentication, TLS, audit logging, and credential isolation remain enforced. Review replacement with an authenticated egress proxy or domain-aware firewall. | 2027-09-09 |
| NET-EX-02 | The customer-facing `arbion.ai` application load balancer must be reachable from the public internet. | Public ingress is limited to HTTP redirect and modern-policy HTTPS; only the load balancer is public; application and data tiers remain private; invalid headers are dropped and deletion protection is enabled. | 2027-09-09 |
