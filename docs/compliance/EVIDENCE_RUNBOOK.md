# SOC 2 Evidence Collection Runbook

| Field | Value |
| --- | --- |
| Document owner | Security and Compliance Owner |
| Version | 0.4 |
| Approval status | PENDING_MANAGEMENT_APPROVAL |
| Review cadence | Quarterly and after evidence-source change |

## Evidence repository

Use an access-controlled, encrypted repository with version history and retention covering the complete examination period plus legal and contractual requirements. Grant access by role, require MFA, review access quarterly, and log export/deletion. Do not put secrets, customer financial details, raw provider payloads, recovery codes, or private model content in Git.

Each evidence item must include control ID, UTC collection time, collector, source system, scoped population or sample, immutable identifier/checksum when available, result, exception link, and reviewer.

## Collection schedule

| Cadence | Evidence |
| --- | --- |
| Every change | Pull request, approvals, required checks, dependency review, CodeQL result, commit SHA, release artifact hash, deployment approval/result, migration result, smoke/readiness result, and rollback reference |
| Continuous/daily | Production health and alert status, security/audit events, scheduler outcomes, backup completion and freshness, vulnerability alerts, and cloud configuration/security findings |
| Monthly | Open vulnerability aging and remediation, patch status, incident/exception register, backup sample, capacity trend, and evidence completeness review |
| Quarterly | GitHub/AWS/production/database/provider access review, privileged-role review, vendor review delta, restore test, key/secret age review, and control-owner certification |
| Annually | Risk assessment, policy review/approval, incident tabletop, business-continuity exercise, vendor reassessment, system description review, and security training |

## Minimum evidence procedures

### External configuration snapshot

Authenticate the GitHub CLI to the Arbion repository and AWS CLI to the production account, then run `scripts/collect-soc2-external-evidence.sh` with an output directory outside the repository. The collector performs read-only API calls, omits subscription endpoints and secrets, writes restrictive local permissions, and creates a deterministically ordered SHA-256 manifest.

Before review or transfer, run `scripts/verify-soc2-evidence-snapshot.sh <snapshot-directory>`. The verifier fails closed on missing, extra, duplicate, reordered, malformed, oversized, symlinked, secret-like, checksum-mismatched, or internally inconsistent evidence.

After verification, create the bounded reviewer aid outside both the repository and snapshot with `scripts/review-soc2-external-evidence.py <snapshot-directory> <outside-directory>/external-control-review.json`. The collector paginates the GitHub collaborator population before this review. The reviewer aid re-runs package verification, rejects duplicate JSON keys, recomputes every source digest, refuses overwrite, writes mode `0600`, and maps 22 narrow saved-field assertions to the control catalog. Its deterministic output is always labeled `REVIEW_DRAFT_NOT_OPERATING_EVIDENCE`; `PASS` means only that a stated saved-field condition matched. A `FAIL` requires an exception and remediation review, while `UNAVAILABLE` requires recollection or a documented exception. The report cannot approve a control, establish operation over time, or support a compliance or certification claim by itself.

External schema 1.2 adds `github-vulnerability-alerts.json` and `github-code-scanning.json` and retains all schema 1.1 topic evidence. New packages require this exact inventory; schemas 1.0 and 1.1 remain readable without adding new evidence to their saved history. Individual Dependabot security-update, secret-scanning, and push-protection states are assessed only when explicitly returned. The legacy four-field aggregate is preserved: a missing `advanced_security` field remains unavailable, never an inferred license or disabled-scanning claim.

The vulnerability-alert read records only a successful HTTP 204, exact repository identity, endpoint, pinned API version, method, and collection time. A 403, 404, unexpected success code, conflicting status lines, or read failure remains unavailable; no alert bodies are retrieved. CodeQL evidence records projected analysis metadata from the exact repository on `refs/heads/main`, with 100 results per page, a maximum 500 results, and a required terminating short or empty page. Capped, partial, duplicated, malformed, future, or out-of-order evidence is unavailable. The head is read before and after collection and must remain identical. The latest saved analysis in each of `/language:actions`, `/language:go`, `/language:javascript-typescript`, and `/language:python` must identify the same pinned main commit and `.github/workflows/security.yml:codeql`. Missing current categories, old-head results, unknown error/warning state, or an ambiguous latest result remain unavailable; known errors, warnings, or zero reported rules do not pass.

Only error/warning presence classifications are saved, never their text, SARIF, finding bodies, or credentials. These narrow checks do not prove future enforcement, license status, alert triage, absence of vulnerabilities, or operation over an observation period. Workflow presence or a successful CI job is not substituted for saved analysis evidence. See the [analysis metadata API](https://docs.github.com/en/rest/code-scanning/code-scanning#list-code-scanning-analyses-for-a-repository) and [vulnerability-alert enablement API](https://docs.github.com/en/rest/repos/repos#check-if-vulnerability-alerts-are-enabled-for-a-repository).

External snapshot schema 1.1 records separate security and operational SNS selections, exact account/region-bound attributes, and an endpoint-free subscription inventory capped at 1,000 entries. Export `ARBION_SECURITY_ALARM_TOPIC_ARN` and `ARBION_OPERATIONS_ALARM_TOPIC_ARN` for an explicitly reviewed selection. Without overrides the recorded defaults are `arbion-<environment>-alarms` (target design only) and `arbion-<environment>-alerts` (operational name only); names never establish existence or delivery. Distinct standard topics in the authenticated account and collection region are required. FIFO and cross-account topics are outside this contract. A missing or inaccessible topic, capped inventory, inconsistent subscription count, malformed identity, or aliased role stays `UNAVAILABLE`. The attributes include KMS configuration without claiming encryption or delivery effectiveness. Schema 1.0 packages retain their original inventory and bytes; missing new binding evidence stays unavailable on review.

The security configuration assertion binds the saved EventBridge rule, exact SNS target, enabled CloudWatch alarm action, and confirmed subscription to the selected security topic. The operational topic is assessed separately, and Lightsail alarm evidence remains a distinct check. No setting is changed, no topic is auto-discovered or substituted, and no notification is published. `AWS_ALERT_DELIVERY_TESTED` always remains `UNAVAILABLE` in this read-only review: an independent, separately authorized exercise must establish actual receipt and response. Recipient addresses, topic policies, message bodies, and credentials are not collected.

Keep the snapshot, manifest, and review draft together; independently review every source and assertion, record exceptions, and move the approved package into immutable or versioned storage in the restricted evidence repository. Successful verification establishes only internal package consistency at that point in time. A party able to alter both evidence and its local manifest can create a new internally consistent package, so authenticity depends on authenticated collection, independent review, access control, and immutable external retention. Collection, verification, and draft generation do not prove operating effectiveness or SOC 2 certification.

### Production host snapshot

Schema 1.2 labels each saved timer deadline with `next_run_clock`: `REALTIME`, `MONOTONIC`, or `UNAVAILABLE`. Interval timers may expose only a monotonic deadline; retain that exact systemd duration, not an invented calendar time. This proves a saved scheduling value, not punctual execution or deadline freshness. Missing deadlines remain incomplete. Schema 1.1 snapshots remain verifiable under their original contract and are never rewritten or relabeled.

Collect the current host snapshot through the restricted, authenticated administration channel described in `docs/compliance/EXTERNAL_CONTROL_VERIFICATION.md` and save the JSON outside the repository. The root-only collector reads control status but never environment values, credentials, logs, customer or application records, database content, or trading data. It reports a canonical collection identity, exact release marker, the six-service inventory, application-container hardening, the reverse-proxy-only network boundary, nine required monitoring timers, three sensitive-file permission records, backup freshness, six existing read-only checks, and an embedded SHA-256 digest over the canonical payload. A snapshot is `COMPLETE_REVIEW_REQUIRED` only when its release marker is valid, services, application hardening, network exposure, and timers pass, all three environment files are root-owned mode `0600`, read-only checks pass, and the backup marker is current.

Run `scripts/verify-soc2-host-evidence.sh <saved-host-evidence.json>` before review or retention. The verifier fails closed on malformed, oversized, symlinked, secret-like, future-dated, checksum-mismatched, duplicated, missing, or internally inconsistent host evidence. An `INCOMPLETE` snapshot can verify only as an internally consistent exception record; it does not pass the underlying controls and is never upgraded. The embedded digest detects accidental or unreconciled changes but is not a signature: anyone able to alter both payload and digest can reseal the file. Authenticity therefore depends on authenticated SSH collection, independent review, restricted access, and prompt immutable or versioned external retention. Verification never establishes operating effectiveness or SOC 2 certification.

### Change sample

Export the PR metadata, exact diff/commit, code-owner review, required checks, security scans, deployment environment approval, production release marker, backup identifier, and post-deploy health result. Preserve failed attempts and emergency classification.

#### Read-only release lineage review

`python3 scripts/soc2_release_evidence.py collect --repository OWNER/REPO --pr NUMBER --host-evidence /outside/host.json --output /outside/release-review.json` collects only projected GitHub repository, merged PR, Git merge-parent, and workflow-run metadata. Add `--deployment-receipt /outside/deployment-receipt.json` only when an actual receipt from that deployment was saved. Outputs must remain outside public Git and synced reference files. The output refuses overwrites, uses mode `0600`, embeds the strict-verified host snapshot and its canonical SHA-256, and seals the complete review draft. Run `python3 scripts/soc2_release_evidence.py verify /outside/release-review.json` for offline internal-consistency verification; it makes no GitHub, provider, or cloud API call.

The current contract supports same-repository two-parent merges into `main`: the exact host release must equal the PR's merged commit, with the PR base and head matching the Git commit's ordered parents. Squash, rebase, fork, malformed, or ambiguous ancestry is not inferred. For that PR head and deployed main SHA, collection reads complete `pull_request` and `push` workflow populations separately, with a 200-run maximum per scope, page size 100, a terminating short/empty page, and repeated complete reads to detect source changes. Every row preserves identity, branch, SHA, workflow path, run attempt, result, and saved timestamps. No logs, review bodies, findings, author emails, customer records, or credentials are retained.

The latest unambiguous saved run for each of `.github/workflows/ci.yml` and `.github/workflows/security.yml` must have an explicit successful completion. PR-head run completion must precede merge; main runs must follow merge. Incomplete or missing populations, pending runs, tied newest timestamps, or rerun attempts with unavailable earlier-attempt history cannot pass. Earlier saved failed runs are not removed. This attributes workflow summaries to the exact recorded head/event; it does not attest which checkout was actually tested, associate an empty GitHub `pull_requests` array with a specific PR number, establish required-check enforcement, or prove an artifact was built from those inputs. See GitHub's [workflow-run API](https://docs.github.com/en/rest/actions/workflow-runs#list-workflow-runs-for-a-repository) and [merged-PR semantics](https://docs.github.com/en/rest/pulls/pulls#get-a-pull-request).

The authorized Lightsail deploy helper now emits one `ARBION_DEPLOYMENT_EVIDENCE_JSON=` line only after readiness, smoke, and container checks pass. Save only the JSON from that exact line in restricted evidence storage; reject missing or duplicate receipt lines. The forward-only receipt records the release and prior release, archive digest, deployment start, fresh backup object/completion, code-replacement interval, final completion, rollback path/digest, host, and successful checks. The backup must complete during this deployment, before replacement, and within five minutes of replacement. Offline review binds the receipt to the saved host identity/release and exact backup marker, validates timestamp ordering and the rollback identity, and never interprets a backup as proof of restore success. A later changed backup marker requires the matching saved host snapshot, not substituting a new marker for the old receipt.

Do not redeploy or run a backup merely to manufacture historical evidence. Releases without a saved contemporaneous receipt keep pre-deploy backup/rollback binding `UNAVAILABLE`; no timestamp is reconstructed from a narrative. A missing or inconsistent receipt cannot be converted into a passing assertion. Local receipt and package hashes are not signatures or independent authentication. Independent review, deployment approval, enforced protection, actual checkout/artifact attestation, and durable retention remain explicitly `UNAVAILABLE` in this narrow reviewer aid regardless of other passing assertions. Preserve these gaps for the separate control-review process; this is not SOC 2 certification or operating-effectiveness evidence.

### Access review

Export active users and roles from GitHub, AWS IAM/Identity Center, production hosts, database administration, DNS/domain, email, monitoring, and material provider portals. The reviewer confirms business need, least privilege, MFA, unique identity, and timely removal. Record exceptions and remediation dates.

### Vulnerability review

Export CodeQL, Dependabot, npm audit, govulncheck, pip-audit, image scanning, host patching, and cloud findings. Deduplicate by identifier, record severity and exposure, assign an owner and due date, and document remediation or risk acceptance.

### Backup and recovery

Retain backup job status, encrypted object identifier/version, checksum, freshness result, restore command output, restored database integrity checks, elapsed recovery time, and reviewer. Use isolated infrastructure and destroy the restored copy after evidence collection under the approved disposal process.

### Incident exercise

Use a plausible scenario involving credential exposure, unauthorized financial-data access, provider compromise, or production outage. Record participants, timeline, classification, containment, communication decision, recovery, evidence preservation, lessons, owners, and due dates.

### Vendor review

Record service, data accessed, criticality, hosting regions, subprocessors, security/privacy terms, breach notification, deletion/return terms, continuity dependency, latest independent assurance, exceptions, approval, and next review.

## Exceptions

Every failed or unavailable control produces an exception with risk, affected scope, compensating control, accountable owner, approval, due date, and expiry. Exceptions cannot silently change a catalog status or weaken the non-live execution boundary.
