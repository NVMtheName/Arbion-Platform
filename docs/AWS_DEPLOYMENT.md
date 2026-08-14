# AWS production deployment foundation

## Status and architecture

This is infrastructure-as-code preparation only: no AWS resource has been applied. AWS ECS/Fargate is Arbion's intended scalable production topology; the supported Caddy/single-host option remains documented in [Deployment](DEPLOYMENT.md).

```text
Internet -> DNS -> ACM -> public ALB (80 redirects to 443)
                           | /api/*, /healthz, /readyz -> private API Fargate
                           | default                    -> private Web Fargate
private API -> RDS PostgreSQL, ElastiCache, ai.arbion.internal
private AI  -> approved AI providers through NAT
```

Only the ALB accepts Internet ingress. Fargate tasks receive no public IP. RDS and ElastiCache occupy isolated data subnets and are never public. The AI service is registered in a private Cloud Map namespace and accepts port 8000 only from the API security group. Its internal bearer token remains mandatory: private networking is not authentication.

## Layout and prerequisites

`infrastructure/terraform/bootstrap` is the one-time administrator stack. Reusable, deliberately small modules live in `modules`; `environments/production` composes them and can later be reused for staging without deploying staging now. Terraform `>= 1.10, < 2.0` and AWS provider `~> 6.0` are pinned. Prerequisites are an AWS account, administrator-operated bootstrap credentials, Terraform, AWS CLI, a GitHub `production` environment restricted to `main`, and the existing repository `NVMtheName/Arbion-Platform`. Configure required reviewers when the repository's GitHub plan supports them; otherwise follow the documented single-operator exception below.

Do not run bootstrap or primary apply from Codex. No permanent AWS key belongs in GitHub.

## Bootstrap, remote state, and OIDC

1. An AWS administrator chooses a globally unique state bucket name, copies `bootstrap/terraform.tfvars.example` to an ignored file, runs `terraform init`, reviews a plan, and manually applies bootstrap once.
2. Bootstrap creates a versioned, public-access-blocked S3 bucket with TLS-only policy and KMS encryption. Both bucket and key have `prevent_destroy`; the key rotates and has a 30-day deletion window.
3. It creates the GitHub OIDC provider and distinct plan, apply, and application-deploy roles. Trust is constrained to this repository; apply/deploy require the `production` GitHub environment while plan uses pull-request subjects. There is no IAM user or static access key.
4. From bootstrap outputs, create an ignored `backend.hcl` containing `bucket`, `region`, and `kms_key_id`. Run `terraform init -migrate-state -backend-config=backend.hcl` in production. The committed backend uses native S3 `use_lockfile = true`; no DynamoDB table exists and no credentials are in backend configuration.

The bootstrap role policies separate state reads, infrastructure management, and release operations. Review CloudTrail and IAM Access Analyzer and narrow the infrastructure policy as the resource set stabilizes; application task roles have no AdministratorAccess.

## Network and outbound access

The VPC creates public, private application, and private data subnets in at least two AZs. Each production application subnet defaults to its same-AZ NAT gateway so API and AI can call Schwab/OpenAI/Anthropic/Gemini over outbound HTTPS without inbound exposure. Data route tables have no Internet default route. `nat_gateway_per_az=true` is the HA default. Setting it false reduces NAT baseline cost for a future non-production environment but introduces a cross-AZ dependency and single failure domain; it is not the production recommendation.

Security groups allow: Internet 80/443 to ALB; ALB to web 3000/API 8080; API to AI 8000, PostgreSQL 5432, Redis 6379, and outbound HTTPS; migration to PostgreSQL; and no other private inbound. Future workers can reuse application/data tiers but none are deployed.

## ALB, ACM, and DNS safety

The ALB uses IP target groups, `/readyz` for API readiness and `/api/health` for the web. HTTP permanently redirects to HTTPS. On HTTPS, apex host traffic redirects to canonical `https://www.arbion.ai`; `/api/*` (including the Schwab callback), `/healthz`, and `/readyz` reach Go; default traffic reaches Next.js. AI, RDS, and cache have no listener rule.

ACM requests `arbion.ai` plus `www.arbion.ai`. Terraform outputs DNS validation records and the ALB DNS name. By default `manage_dns_records=false`, so infrastructure cannot cut over DNS. For an existing Route 53 zone, supply its ID and explicitly enable management only during an approved cutover; Terraform never creates/replaces a hosted zone and records use `allow_overwrite=false`. For external DNS, create the output ACM validation CNAMEs manually, wait for issuance, verify through the ALB/test mapping, and later create apex/`www` ALIAS/ANAME/CNAME records as supported by that provider.

## ECR, ECS, discovery, and scaling

Private `arbion-web`, `arbion-api`, and `arbion-ai` ECR repositories use KMS, push scanning, immutable tags, seven-day untagged cleanup, and a bounded release history. Releases tag all images with the full Git SHA and never rely on `latest`.

The Fargate-only cluster runs web, API, and private AI services with deployment circuit-breaker rollback, 100/200 healthy rollout percentages, CloudWatch logs, read-only root filesystems, no execute-command/SSH, and distinct task roles. The migration task reuses the API image with `/migrate`. Cloud Map resolves `ai.arbion.internal`; the bearer token is still injected into both API and AI. Scaling variables control min/max/CPU/memory. Defaults keep two tasks and modest maxima (6–8), prioritizing AZ availability without unbounded scale.

## PostgreSQL and migrations

RDS PostgreSQL 16 matches the repository's current PostgreSQL generation. It is private, Multi-AZ, GP3 encrypted by KMS, storage-autoscaled, automatically backed up with configurable retention/PITR, log-exporting, deletion-protected, and final-snapshot protected. Terraform uses RDS-managed master credentials in Secrets Manager; no password enters source or tfvars. Applications accept either legacy `DATABASE_URL` or separately injected host/port/name/user/password/SSL mode, preserving Compose. TLS is required on AWS.

Every release registers and runs a one-off Fargate migration revision before changing service traffic, waits for it to stop, checks exit code zero, aborts on failure, then updates services and waits for stability. Runtime schema creation is not used.

## ElastiCache

ElastiCache is a private, Multi-AZ Redis-compatible replication group with TLS, KMS at-rest encryption, automatic failover, and an auth token. Redis remains ephemeral session/coordination state; PostgreSQL is durable truth. To avoid destabilizing the existing URL-based client, this milestone uses a strong operator-populated auth token and a separately populated `rediss://` URL secret. IAM authentication is a future migration path. Redis must never be deployed without the populated token or over plaintext.

## Secrets Manager and KMS

Terraform creates empty protected secret containers for `credential-encryption-key`, `ai-internal-service-token`, Schwab client ID/secret, Redis token, and Redis URL; RDS manages its own credential secret. It never creates application secret values or outputs them. Before primary apply where required, an operator uses an approved workstation and `aws secretsmanager put-secret-value --secret-id <ARN> --secret-string file://...` (or equivalent secure input), avoiding shell history and logs. The Redis URL contains the cache endpoint and URL-escaped token.

Generate `CREDENTIAL_ENCRYPTION_KEY` exactly once as a base64-encoded 32-byte value, back it up under dual control, and populate its existing container. Never regenerate it during deploy: the application's encryption key is distinct from the AWS KMS key. This milestone adds no key-rotation procedure.

## CI/CD and authorization

`terraform.yml` validates formatting/init/validate on infrastructure changes without AWS credentials. Only manual dispatch on `main`, through protected `production`, can assume the apply role and apply a reviewed saved plan. Fork PRs cannot receive AWS credentials. `deploy-aws.yml` is also manual and approval-gated: OIDC, build/push SHA images, migration, service revisions, stable wait, then smoke tests. Infrastructure and application releases remain separate.

Configure non-secret GitHub environment variables for role ARN, region, state identifiers, private subnet IDs, and migration security group. Store production secret values only in Secrets Manager—not GitHub. No workflow applies on merge, destroys infrastructure, changes DNS, or uses access keys.

For a private repository whose GitHub plan does not support required environment reviewers, a single-operator exception may be used temporarily: restrict the `production` environment to `main`, keep infrastructure and deployment workflows manual, leave the apply input disabled by default, and trigger a run only after reviewing its saved plan. This is an explicit reduction in separation of duties, not equivalent to independent approval. Configure required reviewers before granting another operator write access or when the repository moves to a plan that supports reviewer protection for private repositories.

## First deployment and DNS cutover

1. Manually bootstrap and migrate state; review the primary plan without applying from this repository task.
2. Apply only the reviewed foundational `module.secrets` and `module.ecr` targets first, then populate the newly created secret containers outside Terraform/GitHub. Review a new complete plan and create the remaining infrastructure with DNS management disabled. This explicit first-deployment exception breaks the intentional container-before-value/image bootstrap dependency; subsequent infrastructure plans must not use routine targeting.
3. Push initial immutable images, run migrations, verify ECS health/logs and the ALB using a controlled test hostname/host mapping.
4. Validate backups, alarms, RDS TLS, Redis TLS/auth, private service discovery, and API-to-AI token authentication.
5. Validate ACM, then explicitly approve apex/`www` DNS cutover. Confirm apex redirects and canonical site/API health.
6. Only after the canonical callback is reachable, manually test Schwab: Connect, Schwab login/authorization, callback, accounts, balances, positions, and duplicate-safe sync. Never place an order.

The callback remains exactly `https://www.arbion.ai/api/connections/financial/schwab/callback`.

## Observability, rollback, and cost

Log groups for web/API/AI/migrations retain 30 days. Baseline alarms cover ECS CPU plus RDS CPU/storage and feed an SNS topic; extend with ALB unhealthy/5xx, ECS memory/unhealthy deployment, RDS connections, and cache health during operational tuning. Never log environments, URLs containing credentials, provider tokens, cookies, or secret values.

Application rollback selects the previous immutable image/task revision, updates the ECS service, and waits for a healthy rollout. Migration rollback is not assumed: depending on semantics, forward-fix or restore a verified RDS point-in-time backup during an outage. Durable RDS, Secrets Manager, KMS, state bucket, and ALB deletion protections make destroy intentionally difficult. There is no automated destroy workflow.

Material baseline costs are NAT gateways/data processing, ALB/LCUs, continuously running Fargate tasks, Multi-AZ RDS/storage/backups, Multi-AZ ElastiCache, CloudWatch logs/metrics/alarms, KMS/Secrets Manager, ECR storage/scanning, and inter-AZ/Internet transfer. Scaling, retention, instance class, cache node type, and NAT topology are explicit variables for future environments; production security is not weakened for cost.

## Trading safety

AWS production does not imply live trading. This foundation adds no `PlaceOrder`, broker submission, cancellation/replacement, live adapter/switch, or autonomous worker. PAPER simulation and SHADOW intent recording remain the only execution modes; Schwab remains read-only.
