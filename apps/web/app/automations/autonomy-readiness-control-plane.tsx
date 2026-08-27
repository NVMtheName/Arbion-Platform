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
  instance?: Entity;
  schedule?: Entity;
  evidenceGate?: Entity;
  reconciliation?: Entity;
  automationBreaker?: Entity;
  schedulerEnabled: boolean;
  observedAt: string;
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

function providerLabel(provider: string) {
  if (provider.toLowerCase() === "coinbase") return "Coinbase";
  if (provider.toLowerCase() === "schwab") return "Charles Schwab";
  return provider || "Financial provider";
}

function conciseDecimal(value: string) {
  if (!/^\d+(\.\d+)?$/.test(value)) return value;
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
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

function snapshotAge(observedAt: string, now: string) {
  const observed = new Date(observedAt);
  const current = new Date(now);
  if (
    Number.isNaN(observed.valueOf()) ||
    Number.isNaN(current.valueOf()) ||
    observed > current
  ) {
    return { fresh: false, label: "timestamp invalid" };
  }
  const hours = Math.floor(
    (current.valueOf() - observed.valueOf()) / 3_600_000,
  );
  return {
    fresh: hours <= 24,
    label: hours < 1 ? "less than 1 hour old" : `${hours} hours old`,
  };
}

function buildSignals(props: Props): ReadinessSignal[] {
  const financialStatus = read(props.financialConnection, "status", "Status");
  const accountStatus = read(props.financialAccount, "status", "Status");
  const financialPresent = Boolean(
    read(props.financialAccount, "id", "ID") &&
      read(props.financialConnection, "id", "ID"),
  );
  const financialReady =
    financialPresent &&
    financialStatus === "active" &&
    accountStatus === "active";

  const aiStatus = read(props.aiConnection, "status", "Status");
  const aiEnabled = flag(props.aiConnection, "enabled", "Enabled");
  const aiPresent = Boolean(read(props.aiConnection, "id", "ID"));
  const aiReady = aiPresent && aiStatus === "active" && aiEnabled;

  const mandateReady =
    props.automationType === "AI_AUTONOMOUS" &&
    props.autonomyLevel === "FULL_AUTONOMOUS" &&
    props.executionMode === "SHADOW" &&
    props.mandateStatus === "READY" &&
    props.currentVersion > 0;

  const bucketStatus = read(props.capitalBucket, "status", "Status");
  const bucketAllocation = read(
    props.capitalBucket,
    "allocation_value",
    "AllocationValue",
  );
  const allocation = Number(bucketAllocation);
  const bucketReady =
    Boolean(read(props.capitalBucket, "id", "ID")) &&
    bucketStatus === "ACTIVE" &&
    !flag(props.capitalBucket, "is_reserve", "IsReserve") &&
    Number.isFinite(allocation) &&
    allocation > 0;

  const instanceStatus = read(props.instance, "status", "Status");
  const instanceState = read(props.instance, "current_state", "CurrentState");
  const instanceMode = read(props.instance, "execution_mode", "ExecutionMode");
  const instancePresent = Boolean(read(props.instance, "id", "ID"));
  const instanceReady =
    instancePresent &&
    instanceStatus === "ACTIVE" &&
    instanceState === "AI_MONITORING" &&
    instanceMode === "SHADOW";

  const scheduleEnabled = flag(props.schedule, "enabled", "Enabled");
  const scheduleStatus = read(props.schedule, "last_status", "LastStatus");
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
    !scheduleStatus &&
    failures === 0;

  const evidenceStatus = read(props.evidenceGate, "status", "Status");
  const oneHourMarks = number(
    props.evidenceGate,
    "one_hour_sample_size",
    "OneHourSampleSize",
  );
  const twentyFourHourMarks = number(
    props.evidenceGate,
    "twenty_four_hour_sample_size",
    "TwentyFourHourSampleSize",
  );
  const minimumMarks = number(
    props.evidenceGate,
    "minimum_sample_per_horizon",
    "MinimumSamplePerHorizon",
  );
  const evidenceWindow = number(
    props.evidenceGate,
    "evidence_window_hours",
    "EvidenceWindowHours",
  );
  const minimumWindow = number(
    props.evidenceGate,
    "minimum_evidence_window_hours",
    "MinimumEvidenceWindowHours",
  );
  const evidencePresent = Boolean(props.evidenceGate);
  const evidenceReady =
    evidencePresent && evidenceStatus === "EVIDENCE_REVIEWABLE";

  const comparison = read(
    props.reconciliation,
    "comparison_status",
    "ComparisonStatus",
  );
  const balanceCoverage = read(
    props.reconciliation,
    "balances_status",
    "BalancesStatus",
  );
  const positionCoverage = read(
    props.reconciliation,
    "positions_status",
    "PositionsStatus",
  );
  const autonomySignal = read(
    props.reconciliation,
    "autonomy_signal",
    "AutonomySignal",
  );
  const reconciliationObservedAt = read(
    props.reconciliation,
    "observed_at",
    "ObservedAt",
  );
  const age = snapshotAge(reconciliationObservedAt, props.observedAt);
  const reconciliationPresent = Boolean(props.reconciliation);
  const reconciliationReady =
    reconciliationPresent &&
    comparison === "MATCHED" &&
    balanceCoverage === "READY" &&
    positionCoverage === "READY" &&
    autonomySignal === "CLEAR" &&
    flag(
      props.reconciliation,
      "autonomy_enforcement_active",
      "AutonomyEnforcementActive",
    ) &&
    !flag(props.reconciliation, "blocks_new_actions", "BlocksNewActions") &&
    age.fresh;

  const breakerOpen =
    read(props.automationBreaker, "state", "State") === "OPEN";

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
        : financialPresent
          ? `Connection ${financialStatus || "unknown"} · account ${accountStatus || "unknown"}.`
          : "Connected-account status could not be verified.",
    },
    {
      label: "Decision-model route",
      state: !aiPresent ? "UNAVAILABLE" : aiReady ? "READY" : "BLOCKED",
      detail: aiReady
        ? `${props.modelID || "Saved model"} is bound to an active, enabled AI connection.`
        : aiPresent
          ? "The saved AI connection is not both active and enabled."
          : "The saved AI connection could not be verified.",
    },
    {
      label: "Immutable mandate",
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
        ? `${conciseDecimal(bucketAllocation)} USD allocation · active · non-reserve.`
        : "An active, positive, non-reserve capital allocation is required.",
    },
    {
      label: "Shadow engine",
      state: !instancePresent
        ? "UNAVAILABLE"
        : instanceReady
          ? "READY"
          : "BLOCKED",
      detail: instancePresent
        ? `${instanceStatus || "UNKNOWN"} · ${instanceState || "UNKNOWN"} · ${instanceMode || "UNKNOWN"}.`
        : "The AI strategy instance is not initialized.",
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
          ? "Enabled and waiting for its first completed cycle."
          : `Scheduler ${props.schedulerEnabled ? "enabled" : "disabled"} · schedule ${scheduleEnabled ? "enabled" : "disabled"} · ${failures} consecutive failures.`,
    },
    {
      label: "Broker reconciliation",
      state: !reconciliationPresent
        ? "COLLECTING"
        : reconciliationReady
          ? "READY"
          : "BLOCKED",
      detail: reconciliationPresent
        ? `${comparison || "UNKNOWN"} · ${autonomySignal || "UNKNOWN"} · ${age.label}.`
        : "Two complete matching provider snapshots are required.",
    },
    {
      label: "Shadow outcome evidence",
      state: !evidencePresent
        ? "COLLECTING"
        : evidenceReady
          ? "READY"
          : "COLLECTING",
      detail: evidencePresent
        ? `${oneHourMarks}/${minimumMarks} one-hour · ${twentyFourHourMarks}/${minimumMarks} twenty-four-hour · ${evidenceWindow}/${minimumWindow} window hours.`
        : "No immutable outcome scorecard is available yet.",
    },
    {
      label: "Mandate emergency stop",
      state: breakerOpen ? "BLOCKED" : "READY",
      detail: breakerOpen
        ? "The owner emergency stop is engaged; new actions fail closed."
        : "Clear. The owner emergency stop remains immediately available.",
    },
  ];
}

function stateLabel(state: SignalState) {
  if (state === "READY") return "Verified";
  if (state === "COLLECTING") return "Collecting";
  if (state === "BLOCKED") return "Blocked";
  return "Unavailable";
}

export function AutonomyReadinessControlPlane(props: Props) {
  const signals = buildSignals(props);
  const verified = signals.filter((signal) => signal.state === "READY").length;
  const blocked = signals.filter((signal) => signal.state === "BLOCKED").length;
  const collecting = signals.some(
    (signal) => signal.state === "COLLECTING" || signal.state === "UNAVAILABLE",
  );
  const overall =
    blocked > 0
      ? "REVIEW REQUIRED"
      : collecting
        ? "COLLECTING EVIDENCE"
        : "NON-LIVE REVIEWABLE";

  return (
    <section
      className="autonomy-readiness-control-plane"
      aria-labelledby="autonomy-readiness-heading"
    >
      <header>
        <div>
          <p className="eyebrow">AUTONOMY READINESS CONTROL PLANE</p>
          <h2 id="autonomy-readiness-heading">
            Every promotion prerequisite, one view.
          </h2>
          <p>
            These provider-neutral controls are evaluated from Arbion’s current
            account, model, mandate, runtime, reconciliation, schedule, and
            immutable Shadow evidence.
          </p>
        </div>
        <div
          className="autonomy-readiness-score"
          aria-label="Readiness summary"
        >
          <span>{overall}</span>
          <strong>
            {verified}/{signals.length}
          </strong>
          <small>non-live controls verified</small>
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

      <footer>
        <div>
          <span>LIVE SUBMISSION BOUNDARY</span>
          <strong>Physically unavailable in this release</strong>
        </div>
        <p>
          Passing every control makes the non-live evidence reviewable. It does
          not unlock trading, create regulatory approval, or add a broker-write
          adapter. Arbion still cannot submit, replace, cancel, or route an
          order.
        </p>
      </footer>
    </section>
  );
}
