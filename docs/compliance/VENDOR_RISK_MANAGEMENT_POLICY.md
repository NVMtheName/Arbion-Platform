# Vendor Risk Management Policy

| Field | Value |
| --- | --- |
| Policy owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | Before use, annually for critical vendors, and after material change/incident |

## Inventory and tiering

Maintain an inventory of vendors and subservice organizations with owner, service, data class, access, hosting region, criticality, integration method, contract, subprocessors, assurance reports, incident contacts, renewal/termination date, and next review. AWS, GitHub, financial providers, AI providers, email delivery, DNS/domain, and monitoring/support providers require explicit evaluation.

Vendors are tiered by confidential-data access, privileged/system access, service criticality, financial-action influence, substitutability, and outage impact. A vendor that can materially affect authentication, production, customer financial data, or autonomous decisions is critical even when it cannot execute trades.

## Due diligence and monitoring

Before approval, review security/privacy terms, independent assurance, encryption, access control, vulnerability/incident practices, availability/recovery, retention/deletion, subprocessors, breach notification, data use/training, export/return, and termination assistance. Document exceptions and compensating controls. Reassess critical vendors annually and monitor material incidents or service changes.

## Contract and termination

Contracts must reflect approved confidentiality, security, availability, notification, deletion/return, audit/assurance, and subprocessor expectations. On termination, revoke accounts and tokens, rotate shared dependencies, export required records, request deletion, validate replacement continuity, and preserve closure evidence.

Provider output and availability never replace Arbion authorization. Financial and AI vendors receive only the credentials and minimized data needed for the selected service; failures remain bounded and reviewable.
