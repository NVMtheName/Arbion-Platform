import { compareExactDecimals, formatExactMoney } from "../exact-money";
import type { PaperPortfolio } from "./paper-portfolio-summary";

type Entity = Record<string, unknown> | null | undefined;
type SignalState = "READY" | "COLLECTING" | "BLOCKED" | "UNAVAILABLE";

type ReadinessSignal = {
  label: string;
  state: SignalState;
  detail: string;
};

type Props = {
  provider: string;
  modelID: string;
  mandateStatus: string;
  currentVersion: number;
  automationType: string;
  autonomyLevel: string;
  executionMode: string;
  financialAccount?: Entity;
  financialConnection?: Entity;
  aiConnection?: Entity;
  capitalBucket?: Entity;
  capitalReservation?: Entity;
  instance?: Entity;
  schedule?: Entity;
  paperPortfolio?: PaperPortfolio;
  evidenceReadinessContractAvailable: boolean;
  automationBreaker?: Entity;
  schedulerEnabled: boolean;
  decisions?: Entity[];
  allowedSymbols?: string[];
};

function read(entity: Entity, key: string, legacy: string, fallback = "") {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "string" ? value : fallback;
}

function number(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function flag(entity: Entity, key: string, legacy: string) {
  const value = entity?.[key] ?? entity?.[legacy];
  return typeof value === "boolean" ? value : false;
}

function object(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function records(value: unknown) {
  return Array.isArray(value)
    ? value.filter(
        (item): item is Record<string, unknown> =>
          Boolean(item) && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function metric(entity: Record<string, unknown>, key: string) {
  const value = entity[key];
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function sameSymbols(actual: string[], expected: string[]) {
  if (actual.length !== expected.length) return false;
  const left = [...actual].map((value) => value.toUpperCase()).sort();
  const right = [...expected].map((value) => value.toUpperCase()).sort();
  return left.every((value, index) => value === right[index]);
}

function providerLabel(provider: string) {
  if (provider.toLowerCase() === "coinbase") return "Coinbase";
  if (provider.toLowerCase() === "schwab") return "Charles Schwab";
  return provider || "Financial provider";
}

function money(value: string, currency: string) {
  return formatExactMoney(
    { amount: value, currency },
    { maximumFractionDigits: 4 },
  );
}

function positive(value: string) {
  return compareExactDecimals(value, "0") === 1;
}

function nonNegative(value: string) {
  const comparison = compareExactDecimals(value, "0");
  return comparison !== undefined && comparison >= 0;
}

function equal(left: string, right: string) {
  return compareExactDecimals(left, right) === 0;
}

function readableTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "time unavailable";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function buildSignals(props: Props): ReadinessSignal[] {
  const accountID = read(props.financialAccount, "id", "ID");
  const accountStatus = read(props.financialAccount, "status", "Status");
  const financialConnectionID = read(props.financialConnection, "id", "ID");
  const financialStatus = read(props.financialConnection, "status", "Status");
  const financialPresent = Boolean(accountID && financialConnectionID);
  const financialReady =
    financialPresent &&
    accountStatus === "active" &&
    financialStatus === "active";

  const aiConnectionID = read(props.aiConnection, "id", "ID");
  const aiPresent = Boolean(aiConnectionID);
  const aiReady =
    aiPresent &&
    read(props.aiConnection, "status", "Status") === "active" &&
    flag(props.aiConnection, "enabled", "Enabled");

  const mandateReady =
    props.automationType === "AI_AUTONOMOUS" &&
    props.autonomyLevel === "FULL_AUTONOMOUS" &&
    props.executionMode === "PAPER" &&
    props.mandateStatus === "READY" &&
    props.currentVersion > 0;

  const bucketID = read(props.capitalBucket, "id", "ID");
  const bucketStatus = read(props.capitalBucket, "status", "Status");
  const bucketAllocation = read(
    props.capitalBucket,
    "allocation_value",
    "AllocationValue",
  );
  const bucketCurrency = read(
    props.capitalBucket,
    "currency",
    "Currency",
    "USD",
  );
  const bucketReady =
    Boolean(bucketID) &&
    bucketStatus === "ACTIVE" &&
    !flag(props.capitalBucket, "is_reserve", "IsReserve") &&
    positive(bucketAllocation) &&
    bucketCurrency === "USD";

  const instanceID = read(props.instance, "id", "ID");
  const instanceStatus = read(props.instance, "status", "Status");
  const instanceState = read(props.instance, "current_state", "CurrentState");
  const instanceMode = read(props.instance, "execution_mode", "ExecutionMode");
  const instancePresent = Boolean(instanceID);
  const instanceReady =
    instancePresent &&
    instanceStatus === "ACTIVE" &&
    instanceState === "AI_MONITORING" &&
    instanceMode === "PAPER";

  const scheduleEnabled = flag(props.schedule, "enabled", "Enabled");
  const scheduleStatus = read(props.schedule, "last_status", "LastStatus");
  const scheduleError = read(
    props.schedule,
    "last_error_code",
    "LastErrorCode",
  );
  const failures = number(
    props.schedule,
    "consecutive_failures",
    "ConsecutiveFailures",
  );
  const nextRun = read(props.schedule, "next_run_at", "NextRunAt");
  const scheduleReady =
    props.schedulerEnabled &&
    scheduleEnabled &&
    scheduleStatus === "SUCCEEDED" &&
    failures === 0 &&
    Boolean(nextRun);
  const scheduleCollecting =
    props.schedulerEnabled &&
    scheduleEnabled &&
    failures === 0 &&
    (!scheduleStatus || scheduleStatus === "SKIPPED");

  const portfolio = props.paperPortfolio;
  const reservationID = read(props.capitalReservation, "id", "ID");
  const reservationAmount = read(
    props.capitalReservation,
    "reservation_amount",
    "ReservationAmount",
  );
  const reservationCurrency = read(
    props.capitalReservation,
    "currency",
    "Currency",
  );
  const portfolioPresent = Boolean(portfolio);
  const reservationPresent = Boolean(reservationID);
  const ledgerReady =
    portfolio !== undefined &&
    reservationPresent &&
    instancePresent &&
    portfolio.strategy_instance_id === instanceID &&
    read(
      props.capitalReservation,
      "strategy_instance_id",
      "StrategyInstanceID",
    ) === instanceID &&
    read(
      props.capitalReservation,
      "financial_account_id",
      "FinancialAccountID",
    ) === accountID &&
    read(props.capitalReservation, "capital_bucket_id", "CapitalBucketID") ===
      bucketID &&
    read(props.capitalReservation, "execution_mode", "ExecutionMode") ===
      "PAPER" &&
    read(props.capitalReservation, "status", "Status") === "ACTIVE" &&
    read(props.capitalReservation, "reservation_basis", "ReservationBasis") ===
      "PAPER_STARTING_CASH" &&
    portfolio.currency === "USD" &&
    reservationCurrency === portfolio.currency &&
    positive(portfolio.starting_cash) &&
    nonNegative(portfolio.cash) &&
    equal(portfolio.starting_cash, reservationAmount) &&
    equal(portfolio.starting_cash, bucketAllocation) &&
    Number.isInteger(portfolio.version) &&
    portfolio.version >= 1 &&
    Array.isArray(portfolio.positions);

  const breakerOpen =
    read(props.automationBreaker, "state", "State") === "OPEN";

  const latestDecision = (props.decisions ?? []).find(
    (entry) => read(entry, "source", "Source") === "AI",
  );
  const rationale = object(
    latestDecision?.structured_rationale ?? latestDecision?.StructuredRationale,
  );
  const evidence = object(rationale.input_evidence);
  const markets = records(evidence.markets);
  const marketSymbols = markets
    .map((market) => market.symbol)
    .filter((symbol): symbol is string => typeof symbol === "string");
  const recentDecisions = records(evidence.recent_decisions);
  const expectedAIProvider = read(props.aiConnection, "provider", "Provider");
  const decisionType = read(latestDecision, "decision_type", "DecisionType");
  const rationaleDecision =
    typeof rationale.decision === "string" ? rationale.decision : "";
  const decisionProvider =
    typeof rationale.ai_provider === "string" ? rationale.ai_provider : "";
  const decisionModel =
    typeof rationale.model_id === "string" ? rationale.model_id : "";
  const decisionProfile =
    typeof rationale.profile === "string" ? rationale.profile : "";
  const financialInputProvider =
    typeof evidence.provider === "string" ? evidence.provider : "";
  const latency = metric(rationale, "latency_ms");
  const inputUsage = metric(rationale, "input_usage");
  const outputUsage = metric(rationale, "output_usage");
  const decisionProvenanceReady =
    Boolean(latestDecision) &&
    decisionProvider === expectedAIProvider &&
    decisionModel === props.modelID &&
    Boolean(decisionProfile) &&
    financialInputProvider === props.provider &&
    props.executionMode === "PAPER" &&
    sameSymbols(marketSymbols, props.allowedSymbols ?? []) &&
    recentDecisions.length <= 6 &&
    latency !== undefined &&
    inputUsage !== undefined &&
    outputUsage !== undefined;

  const proposedActionID = read(
    latestDecision,
    "proposed_action_id",
    "ProposedActionID",
  );
  const riskEvaluationID = read(
    latestDecision,
    "risk_evaluation_id",
    "RiskEvaluationID",
  );
  const executionRecordID = read(
    latestDecision,
    "execution_record_id",
    "ExecutionRecordID",
  );
  const riskDecision = read(latestDecision, "risk_decision", "RiskDecision");
  const executionStatus = read(
    latestDecision,
    "execution_status",
    "ExecutionStatus",
  );
  const riskFlags = strings(rationale.risk_flags);
  const thesis = typeof rationale.thesis === "string" ? rationale.thesis : "";
  const proposedNotional =
    typeof rationale.proposed_notional === "string"
      ? rationale.proposed_notional
      : "";
  const abstentionBoundaryReady =
    decisionType === "ABSTAIN" &&
    rationaleDecision === "ABSTAIN" &&
    equal(proposedNotional, "0") &&
    Boolean(thesis) &&
    riskFlags.length > 0 &&
    !proposedActionID &&
    !riskEvaluationID &&
    !executionRecordID;
  const proposalBoundaryReady =
    decisionType === "PROPOSE" &&
    rationaleDecision === "PROPOSE" &&
    positive(proposedNotional) &&
    Boolean(proposedActionID) &&
    Boolean(riskEvaluationID) &&
    ((riskDecision === "DENY" && !executionRecordID) ||
      (executionStatus === "SIMULATED_FILLED" && Boolean(executionRecordID)));
  const actionBoundaryReady =
    Boolean(latestDecision) &&
    (abstentionBoundaryReady || proposalBoundaryReady);

  return [
    {
      label: `${providerLabel(props.provider)} connection`,
      state: !financialPresent
        ? "UNAVAILABLE"
        : financialReady
          ? "READY"
          : "BLOCKED",
      detail: financialReady
        ? "Account inventory and credential connection are active."
        : "The connected account is not fully active.",
    },
    {
      label: "Decision-model route",
      state: !aiPresent ? "UNAVAILABLE" : aiReady ? "READY" : "BLOCKED",
      detail: aiReady
        ? `${props.modelID || "Saved model"} is bound to an active, enabled AI connection.`
        : "The saved AI connection is not both active and enabled.",
    },
    {
      label: "Immutable Paper mandate",
      state: mandateReady ? "READY" : "BLOCKED",
      detail: `Version ${props.currentVersion || "—"} · ${props.mandateStatus || "UNKNOWN"} · ${props.autonomyLevel || "UNKNOWN"} · ${props.executionMode || "UNKNOWN"}.`,
    },
    {
      label: "Capital policy",
      state: props.capitalBucket
        ? bucketReady
          ? "READY"
          : "BLOCKED"
        : "UNAVAILABLE",
      detail: bucketReady
        ? `${money(bucketAllocation, bucketCurrency)} active, non-reserve Paper allocation.`
        : "An exact, active, positive, non-reserve USD allocation is required.",
    },
    {
      label: "Paper AI engine",
      state: !instancePresent
        ? "UNAVAILABLE"
        : instanceReady
          ? "READY"
          : "BLOCKED",
      detail: instancePresent
        ? `${instanceStatus || "UNKNOWN"} · ${instanceState || "UNKNOWN"} · ${instanceMode || "UNKNOWN"}.`
        : "The isolated Paper strategy instance is not initialized.",
    },
    {
      label: "Automatic schedule",
      state: scheduleReady
        ? "READY"
        : scheduleCollecting
          ? "COLLECTING"
          : "BLOCKED",
      detail: scheduleReady
        ? `${scheduleStatus} · ${failures} consecutive failures · next ${readableTime(nextRun)}.`
        : scheduleCollecting
          ? scheduleStatus === "SKIPPED"
            ? `Skipped safely${scheduleError ? ` · ${scheduleError}` : ""} · next ${readableTime(nextRun)}.`
            : "Enabled and waiting for its first completed cycle."
          : `Scheduler ${props.schedulerEnabled ? "enabled" : "disabled"} · schedule ${scheduleEnabled ? "enabled" : "disabled"} · ${failures} consecutive failures${scheduleError ? ` · ${scheduleError}` : ""}.`,
    },
    {
      label: "Isolated Paper ledger",
      state:
        !portfolioPresent || !reservationPresent
          ? "UNAVAILABLE"
          : ledgerReady
            ? "READY"
            : "BLOCKED",
      detail: ledgerReady
        ? `${money(portfolio!.starting_cash, portfolio!.currency)} starting cash · ${money(portfolio!.cash, portfolio!.currency)} remaining · version ${portfolio!.version}.`
        : "The Paper portfolio, capital reservation, account, bucket, and strategy identities do not form one exact isolated ledger.",
    },
    {
      label: "Mandate emergency stop",
      state: breakerOpen ? "BLOCKED" : "READY",
      detail: breakerOpen
        ? "The owner emergency stop is engaged; new simulated actions fail closed."
        : "Clear. The owner emergency stop remains immediately available.",
    },
    {
      label: "Latest decision provenance",
      state: !latestDecision
        ? "COLLECTING"
        : decisionProvenanceReady
          ? "READY"
          : "BLOCKED",
      detail: !latestDecision
        ? "Waiting for the first automatic AI decision."
        : decisionProvenanceReady
          ? `${decisionProvider} · ${decisionModel} · ${decisionProfile} · ${financialInputProvider} inputs · ${markets.length} exact markets · ${recentDecisions.length} prior decisions · ${latency} ms · ${inputUsage}/${outputUsage} tokens.`
          : "The newest immutable decision does not match the saved model route, financial provider, exact allowed universe, bounded memory, or complete telemetry contract.",
    },
    {
      label: "Deterministic action boundary",
      state: !latestDecision
        ? "COLLECTING"
        : actionBoundaryReady
          ? "READY"
          : "BLOCKED",
      detail: !latestDecision
        ? "Waiting for a decision to prove the abstention or simulated-action boundary."
        : abstentionBoundaryReady
          ? `Evidence-based abstention stopped before risk evaluation or simulation; ${riskFlags.length} explicit risk flags were preserved.`
          : proposalBoundaryReady
            ? riskDecision === "DENY"
              ? "The proposal reached deterministic risk evaluation and was denied before simulation."
              : "The proposal passed deterministic risk evaluation and produced only an isolated simulated fill."
            : "The latest decision does not prove a valid abstention or deterministic Paper proposal path.",
    },
  ];
}

function stateLabel(state: SignalState) {
  if (state === "READY") return "Verified";
  if (state === "COLLECTING") return "Monitoring";
  if (state === "BLOCKED") return "Blocked";
  return "Unavailable";
}

export function PaperAutonomyReadinessControlPlane(props: Props) {
  const signals = buildSignals(props);
  const verified = signals.filter((signal) => signal.state === "READY").length;
  const blocked = signals.some((signal) => signal.state === "BLOCKED");
  const collecting = signals.some(
    (signal) => signal.state === "COLLECTING" || signal.state === "UNAVAILABLE",
  );
  const overall = blocked
    ? "REVIEW REQUIRED"
    : collecting
      ? "PAPER MONITORING"
      : "PAPER VERIFIED";
  const evidenceGate = props.paperPortfolio?.evidence_readiness;
  const evidenceGateAvailable =
    props.evidenceReadinessContractAvailable && Boolean(evidenceGate);
  const evidenceGateAttention =
    !evidenceGateAvailable ||
    evidenceGate?.status === "REVIEW_REQUIRED" ||
    evidenceGate?.status === "UNAVAILABLE";

  return (
    <section
      className="autonomy-readiness-control-plane paper-autonomy-readiness-control-plane"
      aria-labelledby="paper-autonomy-readiness-heading"
    >
      <header>
        <div>
          <p className="eyebrow">PAPER AUTONOMY CONTROL PLANE</p>
          <h2 id="paper-autonomy-readiness-heading">
            Simulation authority, identity, and health in one view.
          </h2>
          <p>
            Arbion verifies the connected account, model route, immutable
            mandate, exact capital claim, isolated Paper ledger, schedule, and
            emergency stop, then proves the newest decision used the expected
            providers and stopped at a valid simulation boundary.
          </p>
        </div>
        <div
          className="autonomy-readiness-score"
          aria-label="Paper readiness summary"
        >
          <span>{overall}</span>
          <strong>
            {verified}/{signals.length}
          </strong>
          <small>Paper controls verified</small>
        </div>
      </header>

      <div className="autonomy-readiness-grid">
        {signals.map((signal) => (
          <article
            className={`is-${signal.state.toLowerCase()}`}
            key={signal.label}
          >
            <span>{stateLabel(signal.state)}</span>
            <h3>{signal.label}</h3>
            <p>{signal.detail}</p>
          </article>
        ))}
      </div>

      <details
        className={`strategy-fleet-exposure-outcomes is-paper${evidenceGateAttention ? " is-attention" : ""}`}
        open={evidenceGateAttention}
      >
        <summary>
          <span>
            <strong>Paper autonomy evidence gate</strong>
            <small>Seven days and 20 automatic decisions</small>
          </span>
          <span>
            {!evidenceGateAvailable
              ? "Evidence unavailable"
              : evidenceGate!.status === "COLLECTING_EVIDENCE"
                ? "Collecting evidence"
                : evidenceGate!.status === "EVIDENCE_REVIEWABLE"
                  ? "Owner reviewable"
                  : evidenceGate!.status === "REVIEW_REQUIRED"
                    ? "Review required"
                    : "Unavailable"}
          </span>
        </summary>
        {!evidenceGateAvailable ? (
          <p>
            The exact immutable decision, scheduler, ledger, and no-live
            contract could not be verified. Arbion does not infer readiness.
          </p>
        ) : (
          <div>
            <dl>
              <div>
                <dt>Saved evidence window</dt>
                <dd>
                  {evidenceGate!.evidence_window_hours} /{" "}
                  {evidenceGate!.minimum_evidence_window_hours} hours
                </dd>
              </div>
              <div>
                <dt>Automatic decisions</dt>
                <dd>
                  {evidenceGate!.decision_count} /{" "}
                  {evidenceGate!.minimum_decision_count}
                </dd>
                <small>
                  {evidenceGate!.abstention_count} abstain ·{" "}
                  {evidenceGate!.proposal_count} propose
                </small>
              </div>
              <div>
                <dt>Provenance</dt>
                <dd>
                  {evidenceGate!.attributed_decision_count} /{" "}
                  {evidenceGate!.decision_count} attributable
                </dd>
                <small>
                  {evidenceGate!.telemetry_complete_count} telemetry complete ·{" "}
                  {evidenceGate!.bounded_memory_count} bounded-memory
                </small>
              </div>
              <div>
                <dt>Current scheduler</dt>
                <dd>
                  {evidenceGate!.last_schedule_status || "UNAVAILABLE"} ·{" "}
                  {evidenceGate!.consecutive_schedule_failures} failures
                </dd>
              </div>
              <div>
                <dt>Isolated ledger</dt>
                <dd>
                  {evidenceGate!.ledger_contracts_reconciled
                    ? "Reconciled"
                    : "Review required"}
                </dd>
              </div>
              <div>
                <dt>No-live invariant</dt>
                <dd>{evidenceGate!.safety.status}</dd>
                <small>
                  {evidenceGate!.safety.live_mandate_count} live mandates ·{" "}
                  {evidenceGate!.safety.ai_order_intent_count} AI order intents
                  · {evidenceGate!.safety.non_simulation_fill_count}{" "}
                  non-simulation fills
                </small>
              </div>
            </dl>
            {evidenceGate!.blockers.length > 0 && (
              <ol>
                {evidenceGate!.blockers.map((blocker) => (
                  <li key={`${blocker.category}:${blocker.code}`}>
                    <strong>{blocker.code.replaceAll("_", " ")}</strong>
                    <span>{blocker.detail}</span>
                    <small>{blocker.category}</small>
                  </li>
                ))}
              </ol>
            )}
            <p>
              Collection is normal until both thresholds are met. Proposals,
              fills, and profit are not required. Reviewability does not grant
              live authority or enable promotion.
            </p>
          </div>
        )}
      </details>

      <footer>
        <div>
          <span>PAPER SIMULATION BOUNDARY</span>
          <strong>Broker-write capability remains absent</strong>
        </div>
        <p>
          A verified Paper engine may update only Arbion’s isolated simulated
          cash, positions, and immutable fill ledger. It cannot submit, replace,
          cancel, or route an order and cannot change connected-account
          holdings.
        </p>
      </footer>
    </section>
  );
}
