# SOC 2 Operating Evidence Workbook

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.1 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Artifact status | BLANK_TEMPLATE_NOT_OPERATING_EVIDENCE |

Copy this workbook into the restricted evidence repository for each review period. Do not complete it in the public source repository. Remove secrets and customer financial details, link to immutable source evidence, and require a dated reviewer conclusion. A blank or unsigned section is not evidence that a control operated.

## Management approval and scope

| Field | Entry |
| --- | --- |
| Examination type and period |  |
| Trust Services Criteria in scope |  |
| System boundary and material subservice organizations |  |
| Approved policies and versions |  |
| Named executive approver and UTC approval time |  |
| Independent CPA/auditor |  |
| Open scope decisions |  |

## Control-owner register

| Control ID | Accountable person | Operational delegate | Evidence source | Cadence | Last operated | Next due | Reviewer | Result/exception |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |

## Risk register

| Risk ID | Asset/process | Threat and scenario | Inherent likelihood/impact | Existing controls | Residual likelihood/impact | Treatment | Owner | Due date | Approval/expiry | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |  |  |

Include fraud, credential theft, account isolation failure, provider compromise, AI output misuse, unauthorized trading, availability, backup loss, vendor concentration, privacy, regulatory, and significant-change scenarios. Enabling live execution requires a new assessment before implementation.

## Quarterly access review

| System | Population source/time | Identity | Role/access | MFA | Business need | Last use | Decision | Remediation/due date | Reviewer |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |  |

Cover GitHub, AWS IAM/Identity Center, production host, database administration, DNS/domain, email, monitoring, evidence repository, financial providers, and AI providers. Sample same-business-day terminations where applicable.

## Vendor review

| Vendor | Service/data | Criticality | Regions/subprocessors | Contract/DPA | Assurance reviewed | Incident/deletion terms | Exceptions | Owner | Next review |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |  |  |

## Monthly control and vulnerability review

| Source/control | Population and UTC cutoff | Result | Open items by severity | Oldest age | Owner/due date | Risk acceptance/expiry | Reviewer |
| --- | --- | --- | --- | --- | --- | --- | --- |
|  |  |  |  |  |  |  |  |

Include alerts, security events, scheduler reliability, backup freshness, dependency and code findings, image/host patching, cloud findings, capacity, evidence completeness, and expired exceptions.

## Exercise record

| Field | Entry |
| --- | --- |
| Exercise type (`INCIDENT_TABLETOP`, `BACKUP_RESTORE`, or `BUSINESS_CONTINUITY`) |  |
| Date, participants, and approved scenario |  |
| Recovery/response objectives |  |
| Start, detection, containment, recovery, and completion times |  |
| Source evidence and immutable backup/version/checksum |  |
| Actual result versus objective |  |
| Gaps, owners, and due dates |  |
| Reviewer and UTC approval time |  |

## Control exception

| Field | Entry |
| --- | --- |
| Exception ID and affected controls |  |
| Detection time and source |  |
| Scope and evidence |  |
| Risk and customer impact |  |
| Compensating control |  |
| Remediation owner and due date |  |
| Authorized acceptance and expiry |  |
| Closure evidence and independent reviewer |  |

## Period close certification

Record the complete evidence index, missing or late evidence, incidents, changes, access exceptions, vulnerabilities, vendor exceptions, restore results, control failures, remediation status, and management conclusion. The certifier must explicitly state that evidence was reviewed rather than relying on the absence of reported incidents.
