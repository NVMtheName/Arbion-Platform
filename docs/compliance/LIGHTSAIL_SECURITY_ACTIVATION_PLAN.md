# Standalone Lightsail security activation plan

Status: **PLAN_ONLY — NOT APPLIED — NOT OPERATING EVIDENCE**. Prepared 2026-09-10 UTC. This is an implementation/review candidate, not spending approval, retention approval, an IAM access grant, or a SOC 2 assertion.

## Owner summary

The next AWS milestone is management-event auditing, protected audit storage, scoped configuration history, foundational threat detection, and external-access analysis. It does not migrate the application. The existing Lightsail instance, database, containers, DNS, network, backup bucket, host monitoring, and `arbion-production-alerts` topic remain outside this state and unchanged.

Authenticated read-only checks on 2026-09-10 found no returned CloudTrail trails, Config recorders, GuardDuty detectors, or account analyzers in the checked `us-east-1` inventory. The existing operations topic had a confirmed subscription; its policy was inspected without mutation. These dated observations are not exhaustive multi-region assurance. Retain restricted evidence separately; never commit operator endpoints, credentials, state, or plan JSON.

## Exact proposed scope

Use `infrastructure/terraform/environments/lightsail-security`, not `environments/production`. The latter is the larger ECS/Fargate target design; its apply workflow must not activate this plan. The new root has no apply workflow. CI validates and uses mocked-provider **plan** tests without AWS credentials.

| Proposed control       | Boundary                                                                                                                                                                                           |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Management CloudTrail  | Multi-region management/global events, integrity validation, encrypted S3 and CloudWatch delivery. No data events, Insights, or Lake.                                                              |
| Audit archive          | One new private versioned customer-key-encrypted bucket, explicit governance retention, archive transition after 90 days, expiration at configured retention. Backup storage untouched.            |
| Security notifications | New encrypted `arbion-production-security-alarms` topic and dedicated key; six security metric alarms and medium-or-higher GuardDuty event routing. No copied subscriber or existing topic policy. |
| Config                 | Only IAM role/user/group/policy, S3 bucket, KMS key, CloudTrail trail, and EC2 security group classes; continuous recording and six-hour delivery. Not all-resource or host/container coverage.    |
| GuardDuty              | Foundational detection in `us-east-1`; eight optional feature settings disabled; no runtime agent installation. Not comprehensive Lightsail workload or multi-region coverage.                     |
| Access Analyzer        | Account external-access analyzer, not paid internal/unused-access analysis.                                                                                                                        |
| Identity               | Two new CloudTrail/Config service roles, delivery policies, and AWS's Config read policy. AWS may create GuardDuty/Access Analyzer service-linked roles. No application/GitHub identity expansion. |

The initial authenticated preview proposed **48 Terraform resource creations, zero updates, zero replacements, and zero deletions**. Eight entries configure GuardDuty features, not eight new detectors. This used an empty temporary local state and illustrative 365-day archive/log retention. It is **not the production backend plan and must not be applied**. It does not prove absence of remote name collisions or sufficient create permissions. Fresh inventory, backend reconciliation, policy review, approved values, and a new exact plan are required.

## Spending and retention decisions

There is **no measured total monthly estimate or hard spending cap yet**. Account activity, resource changes, accumulated logs, retention, and notifications determine the bill. Do not present this bundle as free or flat-price.

Pricing checked against AWS documentation on 2026-09-10:

| Component                 | Cost basis, not an approved budget                                                                                                                                                                                                                                                 |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Two customer-managed keys | Initially $2/month total key storage plus requests. First and second rotations add storage charges per key. [KMS pricing](https://aws.amazon.com/kms/pricing/).                                                                                                                    |
| Management trail          | First management-event copy per region has no CloudTrail delivery charge; storage, KMS, CloudWatch, and additional copies can incur charges. Insights excluded. [CloudTrail costs](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-trail-manage-costs.html). |
| Config                    | US East continuous-recording example: $0.003 per item. 1,000 items would be $3; 10,000 would be $30, plus storage/delivery. Scenarios, not measured Arbion volumes. No rules/conformance packs. [Config pricing](https://aws.amazon.com/config/pricing/).                          |
| GuardDuty                 | Usage-priced: AWS's US East management-event example is $4 per million; network/DNS analysis separately metered. Do not assume trial eligibility or free ongoing operation. [GuardDuty pricing](https://aws.amazon.com/guardduty/pricing/).                                        |
| External Access Analyzer  | External-access analysis has no additional charge; paid types excluded. [Access Analyzer pricing](https://aws.amazon.com/iam/access-analyzer/pricing/).                                                                                                                            |
| Other metered usage       | Log ingestion/retention, six custom metrics/alarms, archive storage/transitions/retrieval, requests, and message delivery. [CloudWatch](https://aws.amazon.com/cloudwatch/pricing/), [S3](https://aws.amazon.com/s3/pricing/), [SNS](https://aws.amazon.com/sns/pricing/).         |

Before activation, record the owner's monthly budget/escalation threshold, explicit retention, and notification destination. A budget alert is not a hard cap. Review estimated usage promptly and before any trial ends; do not auto-delete evidence or disable monitoring to meet a guessed budget. Governance permits specially authorized bypass; it is not irreversible compliance-mode retention. Enabling Object Lock and protecting stored objects have retention consequences even though the app is untouched.

## Dependency and policy caveats

AWS's read-only policy validator identified an unsupported encryption-context condition on `kms:DescribeKey` and an operator-type warning in the inherited audit key. The candidate now separates metadata-only inspection for the regional CloudWatch Logs service from data operations; encryption/decryption retain an exact log-group `StringEquals` context restriction. Tests cover that separation. This changes proposed policy design, not a deployed key. [KMS encryption-context conditions](https://docs.aws.amazon.com/kms/latest/developerguide/conditions-kms.html).

- The new topic avoids replacing existing operations policies/subscriptions. It starts without subscribers: notifications are not effective until an approved destination is confirmed and delivery is verified.
- EventBridge-to-encrypted-SNS has a KMS condition-key limitation. The dedicated key grants the required service actions and is not shared with app/backup data. Review key/topic policies together and prove actual delivery, not just valid JSON. [SNS key management](https://docs.aws.amazon.com/sns/latest/dg/sns-key-management.html).
- AWS may enable optional GuardDuty plans on detector creation; explicit feature updates set the planned steady state afterward. These operations are not atomic. Verify actual features and organization policy; investigate any failed update. Removing a Terraform feature resource does not disable its AWS feature. [GuardDuty defaults](https://docs.aws.amazon.com/guardduty/latest/ug/guardduty-pricing.html), [provider behavior](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/guardduty_detector_feature).
- Config permits one delivery channel per account/region and needs a recorder before channel creation. The module now encodes that dependency while preserving existing resource identities/defaults. Never overwrite a recorder/channel discovered during recheck. [PutDeliveryChannel](https://docs.aws.amazon.com/config/latest/APIReference/API_PutDeliveryChannel.html).
- Audit names overlap the target-design module. A future ECS migration needs an independently reviewed state ownership transfer/import; never let both roots manage the same resources. The separate backend key is `lightsail-security/terraform.tfstate`, not `production/terraform.tfstate`.
- Evidence collection currently defaults to the target-design alarm topic. Before post-activation collection, implement/test exact security-topic selection; do not relabel old-topic evidence or mark routing PASS while it is unavailable.

## Review and activation sequence

1. Review the commit and CI. Supply the actual expected account and explicit retention outside Git. The example account is not production. Preserve the existing production lockfile.
2. Resolve budget, retention, notification recipient, and independent reviewer. Review full IAM/KMS/S3/SNS policies, the Config read policy, service-linked-role effects, feature states, and regional gaps. Do not expand a deployment identity just to make apply succeed.
3. Recheck trails/shadow trails, recorder/channel, detectors/organization policy, analyzers, proposed bucket/key aliases/roles/topic, and state ownership. Inaccessible/conflicting resources require review, not automatic replacement/import.
4. Verify the existing encrypted state backend and locking. Initialize only the new key after authority is established. Retain a restricted state backup, generate a fresh saved plan with the reviewed lockfile, and review its digest/actions/policies. Reject runtime, network, database, DNS, backup, existing topic, broker, or trading changes.
5. Obtain approval tied to the exact plan, IAM scope, expected spend, retention, and recipient. No apply, billing activation, subscriber confirmation, or irreversible retention operation is included in this plan-preparation task.
6. After approved activation, verify CloudTrail logging/digest/delivery, Config recording/encrypted delivery, each GuardDuty feature, analyzer state, storage protection, and confirmed recipient. Run controlled delivery tests with an approved recipient, retain receipts, and recheck existing operations alerts/production health. Never put event bodies, credentials, or customer data in Git.
7. Retain plan/source/lock hashes, timestamps, actor, approval, state backup, post-apply snapshot, delivery receipts, and exceptions in restricted evidence storage. Missing checks stay unavailable. This cannot establish operating effectiveness or replace an independent SOC 2 examination.

## Local verification

```sh
terraform -chdir=infrastructure/terraform/environments/lightsail-security init -backend=false -lockfile=readonly
terraform -chdir=infrastructure/terraform/environments/lightsail-security validate
terraform -chdir=infrastructure/terraform/environments/lightsail-security test
```

Mocked tests cannot verify actual service delivery or billing. The initial backend-free disposable preview is not an apply candidate; regenerate against the approved backend before activation.
