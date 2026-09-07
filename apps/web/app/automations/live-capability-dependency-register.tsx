import { createHash } from "node:crypto";

import type {
  PaperLiveReadinessEngineProjection,
  PaperLiveReadinessSignal,
} from "./paper-live-readiness-dossier";

export type LiveCapabilityDependencyState =
  | "PROVEN_NON_LIVE"
  | "COLLECTING"
  | "BLOCKED"
  | "UNAVAILABLE"
  | "BLOCKED_UNIMPLEMENTED"
  | "REVIEW_NOT_RECORDED";

export type LiveCapabilityDependency = {
  key: string;
  category:
    | "NON_LIVE_EVIDENCE"
    | "PLATFORM_EXECUTION"
    | "PROVIDER_PERMISSION"
    | "SECURITY"
    | "LEGAL_COMPLIANCE"
    | "OWNER_GOVERNANCE";
  label: string;
  state: LiveCapabilityDependencyState;
  structuralBlocker: boolean;
  responsibility: string;
  detail: string;
  evidenceSource: string;
};

type LiveCapabilityPacketCore = {
  schema_version: "arbion.live_capability_dependency_packet.v1";
  fingerprint_scope: "EXACT_SAVED_NON_LIVE_EVIDENCE_AND_PLATFORM_CONTRACT";
  paper_mandate_id: string;
  paper_strategy_instance_id: string;
  companion_shadow_strategy_instance_id: string;
  financial_account_id: string;
  financial_connection_id: string;
  capital_bucket_id: string;
  capital_reservation_id: string;
  latest_decision_id: string;
  latest_decision_at: string;
  account_name: string;
  financial_provider: string;
  observed_at: string;
  dossier_status: PaperLiveReadinessEngineProjection["status"];
  live_execution_available: false;
  live_promotion_available: false;
  grants_authority: false;
  dependencies: Array<{
    key: string;
    category: LiveCapabilityDependency["category"];
    state: LiveCapabilityDependencyState;
    structural_blocker: boolean;
    responsibility: string;
    evidence_source: string;
  }>;
};

export type LiveCapabilityDependencyRegisterProjection = {
  status: "LIVE_BLOCKED";
  dependencyCount: number;
  provenNonLiveCount: number;
  collectingCount: number;
  unavailableCount: number;
  structuralBlockerCount: number;
  separateReviewCount: number;
  attention: boolean;
  ownerGuidance: string;
  dependencies: LiveCapabilityDependency[];
  packetFingerprint: string;
  packetJSON: string;
  packetFilename: string;
};

function aggregateNonLiveState(signals: PaperLiveReadinessSignal[]) {
  const evidence = signals.filter((signal) => signal.key !== "LIVE_CAPABILITY");
  if (evidence.length === 0) return "UNAVAILABLE" as const;
  if (evidence.some((signal) => signal.state === "BLOCKED"))
    return "BLOCKED" as const;
  if (evidence.some((signal) => signal.state === "UNAVAILABLE"))
    return "UNAVAILABLE" as const;
  if (evidence.some((signal) => signal.state === "COLLECTING"))
    return "COLLECTING" as const;
  return "PROVEN_NON_LIVE" as const;
}

function nonLiveDetail(state: LiveCapabilityDependencyState) {
  if (state === "PROVEN_NON_LIVE")
    return "All current non-live dossier controls are proven from saved evidence. This proves only Paper and Shadow operation.";
  if (state === "COLLECTING")
    return "One or more exact Paper or Shadow evidence windows are still collecting through normal automatic cycles.";
  if (state === "BLOCKED")
    return "One or more current non-live controls require review before this evidence case can be relied upon.";
  return "One or more current non-live evidence contracts are unavailable, so readiness is not inferred.";
}

function dependenciesForEngine(
  engine: PaperLiveReadinessEngineProjection,
): LiveCapabilityDependency[] {
  const nonLiveState = aggregateNonLiveState(engine.signals);
  const liveContract = engine.signals.find(
    (signal) => signal.key === "LIVE_CAPABILITY",
  );
  const authorization = engine.signals.find(
    (signal) => signal.key === "AUTHORIZATION",
  );
  return [
    {
      key: "NON_LIVE_EVIDENCE_CASE",
      category: "NON_LIVE_EVIDENCE",
      label: "Current Paper + Shadow evidence case",
      state: nonLiveState,
      structuralBlocker: false,
      responsibility: "Automatic non-live system and owner review",
      detail: nonLiveDetail(nonLiveState),
      evidenceSource: `${engine.signals.length - 1} saved non-live dossier controls · ${engine.detailHref}`,
    },
    {
      key: "LIVE_EXECUTION_RUNTIME",
      category: "PLATFORM_EXECUTION",
      label: "Live execution runtime",
      state: "BLOCKED_UNIMPLEMENTED",
      structuralBlocker: true,
      responsibility: "Arbion engineering",
      detail:
        "The platform contract explicitly exposes no live execution or promotion capability. A future runtime must be separately designed, reviewed, and tested.",
      evidenceSource:
        liveContract?.evidence ?? "UNAVAILABLE · live remains disabled",
    },
    {
      key: "PROVIDER_WRITE_PERMISSION",
      category: "PROVIDER_PERMISSION",
      label: "Broker write permission and credential scope",
      state: "UNAVAILABLE",
      structuralBlocker: true,
      responsibility: "Financial provider and owner",
      detail:
        "An active saved connection does not prove order-write permission, credential scope, product eligibility, or provider approval. No provider was contacted.",
      evidenceSource: authorization?.evidence ?? "UNAVAILABLE",
    },
    {
      key: "LIVE_ORDER_LIFECYCLE",
      category: "PLATFORM_EXECUTION",
      label: "Idempotent live order lifecycle",
      state: "BLOCKED_UNIMPLEMENTED",
      structuralBlocker: true,
      responsibility: "Arbion engineering",
      detail:
        "The saved non-live proposal, risk, and simulation chain is not a broker submission, acknowledgement, cancel, replace, or terminal-status workflow.",
      evidenceSource:
        "Current order intents are nonexecuting · no broker submission contract",
    },
    {
      key: "LIVE_RECONCILIATION",
      category: "PLATFORM_EXECUTION",
      label: "Live pre-trade and post-trade reconciliation",
      state: "BLOCKED_UNIMPLEMENTED",
      structuralBlocker: true,
      responsibility: "Arbion engineering",
      detail:
        "Paper accounting and connected-account read reconciliation do not prove broker order, fill, position, cash, or partial-fill reconciliation for live execution.",
      evidenceSource: "Paper simulation and read-only account contracts only",
    },
    {
      key: "LIVE_KILL_SWITCH_ENFORCEMENT",
      category: "PLATFORM_EXECUTION",
      label: "Broker-bound kill switch enforcement",
      state: "BLOCKED_UNIMPLEMENTED",
      structuralBlocker: true,
      responsibility: "Arbion engineering and security review",
      detail:
        "Existing non-live circuit breakers do not prove cancellation, submission blocking, or recovery behavior against a live broker order path.",
      evidenceSource: "No live broker path exists to test or enforce",
    },
    {
      key: "SECURITY_REVIEW",
      category: "SECURITY",
      label: "Live security and operational review",
      state: "REVIEW_NOT_RECORDED",
      structuralBlocker: true,
      responsibility: "Security and production operations",
      detail:
        "This dossier contains no immutable approval for live credential handling, least privilege, incident response, or production order operations.",
      evidenceSource: "No saved live security review record",
    },
    {
      key: "LEGAL_COMPLIANCE_REVIEW",
      category: "LEGAL_COMPLIANCE",
      label: "Legal and compliance review",
      state: "REVIEW_NOT_RECORDED",
      structuralBlocker: true,
      responsibility: "Qualified legal and compliance reviewers",
      detail:
        "Paper outcomes and owner acknowledgements do not establish regulatory, contractual, suitability, disclosure, or compliance approval.",
      evidenceSource: "No saved live legal or compliance review record",
    },
    {
      key: "OWNER_PROMOTION_AUTHORIZATION",
      category: "OWNER_GOVERNANCE",
      label: "Owner live-promotion authorization",
      state: "BLOCKED_UNIMPLEMENTED",
      structuralBlocker: true,
      responsibility: "Owner governance after every other prerequisite",
      detail:
        "The current MFA-backed Paper evidence review grants no authority, and Arbion intentionally provides no live-promotion action.",
      evidenceSource: "grants_authority=false · live_promotion_available=false",
    },
  ];
}

function packetCore(
  engine: PaperLiveReadinessEngineProjection,
  dependencies: LiveCapabilityDependency[],
): LiveCapabilityPacketCore {
  return {
    schema_version: "arbion.live_capability_dependency_packet.v1",
    fingerprint_scope: "EXACT_SAVED_NON_LIVE_EVIDENCE_AND_PLATFORM_CONTRACT",
    paper_mandate_id: engine.id,
    paper_strategy_instance_id: engine.instanceID,
    companion_shadow_strategy_instance_id:
      engine.shadowInstanceID ?? "UNAVAILABLE",
    financial_account_id: engine.financialAccountID ?? "UNAVAILABLE",
    financial_connection_id: engine.financialConnectionID ?? "UNAVAILABLE",
    capital_bucket_id: engine.capitalBucketID ?? "UNAVAILABLE",
    capital_reservation_id: engine.capitalReservationID ?? "UNAVAILABLE",
    latest_decision_id: engine.latestDecisionID ?? "UNAVAILABLE",
    latest_decision_at: engine.latestDecisionAt ?? "UNAVAILABLE",
    account_name: engine.accountName,
    financial_provider: engine.provider,
    observed_at: engine.observedAt ?? "UNAVAILABLE",
    dossier_status: engine.status,
    live_execution_available: false,
    live_promotion_available: false,
    grants_authority: false,
    dependencies: dependencies.map((dependency) => ({
      key: dependency.key,
      category: dependency.category,
      state: dependency.state,
      structural_blocker: dependency.structuralBlocker,
      responsibility: dependency.responsibility,
      evidence_source: dependency.evidenceSource,
    })),
  };
}

export function projectLiveCapabilityDependencyRegister(
  engine: PaperLiveReadinessEngineProjection,
): LiveCapabilityDependencyRegisterProjection {
  const dependencies = dependenciesForEngine(engine);
  const core = packetCore(engine, dependencies);
  const packetFingerprint = createHash("sha256")
    .update(JSON.stringify(core))
    .digest("hex");
  const packetJSON = JSON.stringify(
    { ...core, content_sha256: packetFingerprint },
    null,
    2,
  );
  const unavailableCount = dependencies.filter(
    (dependency) => dependency.state === "UNAVAILABLE",
  ).length;
  const collectingCount = dependencies.filter(
    (dependency) => dependency.state === "COLLECTING",
  ).length;
  const nonLiveEvidence = dependencies[0];
  return {
    status: "LIVE_BLOCKED",
    dependencyCount: dependencies.length,
    provenNonLiveCount: dependencies.filter(
      (dependency) => dependency.state === "PROVEN_NON_LIVE",
    ).length,
    collectingCount,
    unavailableCount,
    structuralBlockerCount: dependencies.filter(
      (dependency) => dependency.structuralBlocker,
    ).length,
    separateReviewCount: dependencies.filter(
      (dependency) => dependency.state === "REVIEW_NOT_RECORDED",
    ).length,
    attention:
      nonLiveEvidence.state === "BLOCKED" ||
      nonLiveEvidence.state === "UNAVAILABLE",
    ownerGuidance:
      nonLiveEvidence.state === "PROVEN_NON_LIVE"
        ? "The saved non-live evidence is complete. Live remains blocked by separate engineering, provider-permission, security, legal/compliance, and owner-governance prerequisites."
        : "Resolve or continue collecting the exact non-live evidence first. Every live-specific dependency remains separately blocked.",
    dependencies,
    packetFingerprint,
    packetJSON,
    packetFilename: `arbion-live-capability-${engine.instanceID}.json`,
  };
}

function stateLabel(state: LiveCapabilityDependencyState) {
  if (state === "PROVEN_NON_LIVE") return "Proven · non-live";
  if (state === "COLLECTING") return "Collecting";
  if (state === "BLOCKED") return "Review required";
  if (state === "UNAVAILABLE") return "Unavailable";
  if (state === "REVIEW_NOT_RECORDED") return "Review not recorded";
  return "Not implemented";
}

export function LiveCapabilityDependencyRegister({
  engine,
}: {
  engine: PaperLiveReadinessEngineProjection;
}) {
  const register = projectLiveCapabilityDependencyRegister(engine);
  const packetHref = `data:application/json;charset=utf-8,${encodeURIComponent(register.packetJSON)}`;
  return (
    <section
      className="live-capability-register"
      aria-label={`${engine.title} live capability dependency register`}
    >
      <header>
        <div>
          <p className="eyebrow">LIVE CAPABILITY DEPENDENCY REGISTER</p>
          <h3>No live promotion path exists.</h3>
          <p>{register.ownerGuidance}</p>
        </div>
        <span>Live blocked</span>
      </header>
      <dl>
        <div>
          <dt>Dependencies</dt>
          <dd>{register.dependencyCount}</dd>
        </div>
        <div>
          <dt>Structural blockers</dt>
          <dd>{register.structuralBlockerCount}</dd>
        </div>
        <div>
          <dt>Separate reviews</dt>
          <dd>{register.separateReviewCount}</dd>
        </div>
        <div>
          <dt>Packet fingerprint</dt>
          <dd title={register.packetFingerprint}>
            {register.packetFingerprint.slice(0, 12)}…
          </dd>
        </div>
      </dl>
      <details open={register.attention}>
        <summary>
          Exact live dependency evidence
          <span>{register.structuralBlockerCount} block live</span>
        </summary>
        <ol>
          {register.dependencies.map((dependency) => (
            <li
              className={`is-${dependency.state.toLowerCase().replaceAll("_", "-")}`}
              key={dependency.key}
            >
              <header>
                <div>
                  <small>{dependency.category.replaceAll("_", " ")}</small>
                  <strong>{dependency.label}</strong>
                </div>
                <span>{stateLabel(dependency.state)}</span>
              </header>
              <p>{dependency.detail}</p>
              <dl>
                <div>
                  <dt>Responsible follow-up</dt>
                  <dd>{dependency.responsibility}</dd>
                </div>
                <div>
                  <dt>Exact evidence source</dt>
                  <dd>{dependency.evidenceSource}</dd>
                </div>
              </dl>
            </li>
          ))}
        </ol>
      </details>
      <details className="live-capability-packet">
        <summary>
          Machine-readable non-executable packet
          <span>SHA-256</span>
        </summary>
        <div>
          <p>
            This deterministic packet contains saved identifiers, states, and
            evidence references only. Its fingerprint is not an approval,
            signature, or authority grant.
          </p>
          <code>{register.packetFingerprint}</code>
          <a download={register.packetFilename} href={packetHref}>
            Download exact JSON packet
          </a>
          <pre>{register.packetJSON}</pre>
        </div>
      </details>
      <footer>
        Saved evidence only · no provider contact · no credential scope
        inference · no live toggle · no order submission · no authority grant
      </footer>
    </section>
  );
}
