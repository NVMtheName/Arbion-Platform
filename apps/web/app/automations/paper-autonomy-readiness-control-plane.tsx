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
  automationBreaker?: Entity;
  schedulerEnabled: boolean;
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
            emergency stop without implying broker execution.
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
