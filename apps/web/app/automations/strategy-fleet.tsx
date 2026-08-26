import Link from "next/link";

export type StrategyFleetItem = {
  id: string;
  title: string;
  accountName: string;
  provider: string;
  automationType: string;
  mandateStatus: string;
  autonomyLevel: string;
  executionMode: string;
  modelID?: string;
  symbols: string[];
  instanceStatus?: string;
  currentState?: string;
  lastEvaluatedAt?: string;
  scheduleAvailable?: boolean;
  scheduleEnabled?: boolean;
  scheduleStatus?: string;
  consecutiveFailures: number;
  nextRunAt?: string;
  accountContextAvailable?: boolean;
  instanceContextAvailable?: boolean;
};

type StrategyFleetProps = {
  items: StrategyFleetItem[];
  inventoryAvailable?: boolean;
  contextWarnings?: string[];
};

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider === "connected_account" ? "Connected account" : provider;
}

function providerInitial(provider: string) {
  if (provider === "coinbase") return "C";
  if (provider === "schwab") return "S";
  return provider.slice(0, 1).toUpperCase() || "A";
}

function readable(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function readableTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(date);
}

function isAI(item: StrategyFleetItem) {
  return item.automationType === "AI_AUTONOMOUS";
}

function needsReview(item: StrategyFleetItem) {
  return (
    item.instanceStatus === "ERROR" ||
    item.currentState === "ERROR" ||
    item.accountContextAvailable === false ||
    item.instanceContextAvailable === false ||
    item.scheduleAvailable === false ||
    item.scheduleStatus === "FAILED" ||
    item.consecutiveFailures > 0
  );
}

function statusLabel(item: StrategyFleetItem) {
  if (item.instanceContextAvailable === false) return "Status unavailable";
  if (item.accountContextAvailable === false) return "Context unavailable";
  if (needsReview(item)) return "Needs review";
  if (item.instanceStatus === "PAUSED") return "Paused";
  if (item.instanceStatus === "ACTIVE" && item.currentState === "AI_MONITORING")
    return "Monitoring";
  if (item.instanceStatus === "ACTIVE") return "Active";
  if (item.mandateStatus === "DRAFT") return "Draft";
  if (item.mandateStatus === "READY") return "Ready to initialize";
  return readable(item.mandateStatus || "Unknown");
}

function healthLabel(item: StrategyFleetItem) {
  if (item.instanceContextAvailable === false)
    return "Engine state unavailable";
  if (item.accountContextAvailable === false)
    return "Account context unavailable";
  if (item.scheduleAvailable === false) return "Schedule status unavailable";
  if (item.scheduleStatus === "FAILED" || item.consecutiveFailures > 0)
    return "Schedule needs review";
  if (item.instanceStatus === "ERROR" || item.currentState === "ERROR")
    return "Engine needs review";
  if (item.instanceStatus === "PAUSED") return "Paused by owner";
  if (item.scheduleEnabled && item.nextRunAt) return "Healthy schedule";
  if (item.instanceStatus === "ACTIVE") return "Manual cycles only";
  if (item.mandateStatus === "READY") return "Ready to initialize";
  return "Draft configuration";
}

function healthClass(item: StrategyFleetItem) {
  if (needsReview(item)) return "needs-review";
  return item.instanceStatus === "ACTIVE" ? "healthy" : "neutral";
}

function modelLabel(item: StrategyFleetItem) {
  if (isAI(item)) return item.modelID ?? "Model not recorded";
  return "Deterministic rules";
}

function coverageLabel(symbols: string[]) {
  if (symbols.length === 0) return "Not defined";
  if (symbols.length <= 3) return symbols.join(" · ");
  return `${symbols.slice(0, 3).join(" · ")} +${symbols.length - 3}`;
}

export function StrategyFleet({
  items,
  inventoryAvailable = true,
  contextWarnings = [],
}: StrategyFleetProps) {
  const ordered = [...items].sort((left, right) => {
    const aiOrder = Number(isAI(right)) - Number(isAI(left));
    if (aiOrder !== 0) return aiOrder;
    const activeOrder =
      Number(right.instanceStatus === "ACTIVE") -
      Number(left.instanceStatus === "ACTIVE");
    if (activeOrder !== 0) return activeOrder;
    return left.accountName.localeCompare(right.accountName);
  });
  const monitoring = items.filter(
    (item) =>
      isAI(item) &&
      item.instanceStatus === "ACTIVE" &&
      item.currentState === "AI_MONITORING",
  ).length;
  const healthySchedules = items.filter(
    (item) =>
      item.scheduleAvailable === true &&
      item.scheduleEnabled &&
      Boolean(item.nextRunAt) &&
      item.scheduleStatus !== "FAILED" &&
      item.consecutiveFailures === 0,
  ).length;
  const attention = items.filter(needsReview).length;
  const drafts = items.filter((item) => item.mandateStatus === "DRAFT").length;

  return (
    <>
      <section className="strategy-fleet-metrics" aria-label="Fleet summary">
        <article>
          <span>Monitoring</span>
          <strong>{inventoryAvailable ? monitoring : "—"}</strong>
          <small>AI shadow engines</small>
        </article>
        <article>
          <span>Scheduled</span>
          <strong>{inventoryAvailable ? healthySchedules : "—"}</strong>
          <small>healthy automatic cycles</small>
        </article>
        <article className={attention > 0 ? "needs-review" : undefined}>
          <span>Attention</span>
          <strong>{inventoryAvailable ? attention : "—"}</strong>
          <small>engine health signals</small>
        </article>
        <article>
          <span>Drafts</span>
          <strong>{inventoryAvailable ? drafts : "—"}</strong>
          <small>not initialized</small>
        </article>
      </section>

      {inventoryAvailable && contextWarnings.length > 0 && (
        <section className="strategy-fleet-warning" role="status">
          <strong>Some live strategy context is unavailable.</strong>
          <p>{contextWarnings.join(" ")}</p>
        </section>
      )}

      {!inventoryAvailable ? (
        <section className="strategy-fleet-empty is-unavailable" role="status">
          <p className="eyebrow">LIVE STATUS UNAVAILABLE</p>
          <h2>Strategies could not be loaded.</h2>
          <p>
            Existing mandates and schedules were not changed. Return to the
            dashboard while Arbion restores this read-only view.
          </p>
          <Link className="button-link" href="/dashboard">
            Return to dashboard
          </Link>
        </section>
      ) : ordered.length === 0 ? (
        <section className="strategy-fleet-empty">
          <p className="eyebrow">START IN SHADOW</p>
          <h2>No strategies yet.</h2>
          <p>
            Connect one account and one AI model, define a bounded objective and
            budget, then begin with non-live observation.
          </p>
          <Link className="button-link" href="/automations/new">
            Launch an AI Shadow Engine
          </Link>
        </section>
      ) : (
        <section className="strategy-fleet-grid" aria-label="Strategy fleet">
          {ordered.map((item) => (
            <article
              className={isAI(item) ? "is-ai-engine" : "is-rules-engine"}
              key={item.id}
            >
              <header>
                <div className="strategy-fleet-account">
                  <span
                    className={`provider-mark provider-${item.provider}`}
                    aria-hidden="true"
                  >
                    {providerInitial(item.provider)}
                  </span>
                  <div>
                    <small>{providerLabel(item.provider)}</small>
                    <strong>{item.accountName}</strong>
                  </div>
                </div>
                <span
                  className={`strategy-fleet-status ${
                    needsReview(item)
                      ? "needs-review"
                      : item.instanceStatus === "ACTIVE"
                        ? "is-active"
                        : ""
                  }`}
                >
                  {statusLabel(item)}
                </span>
              </header>

              <div className="strategy-fleet-identity">
                <p className="eyebrow">
                  {isAI(item) ? "AI AUTONOMOUS" : "RULES-BASED"}
                </p>
                <h2>{item.title}</h2>
                <p>
                  {isAI(item)
                    ? "Model-led analysis inside deterministic risk controls."
                    : "A defined playbook operating inside a non-live account sandbox."}
                </p>
              </div>

              <dl>
                <div>
                  <dt>{isAI(item) ? "Model" : "Decision system"}</dt>
                  <dd>{modelLabel(item)}</dd>
                </div>
                <div>
                  <dt>Coverage</dt>
                  <dd>{coverageLabel(item.symbols)}</dd>
                </div>
                <div>
                  <dt>Mode</dt>
                  <dd>{readable(item.executionMode)}</dd>
                </div>
                <div>
                  <dt>Last cycle</dt>
                  <dd>{readableTime(item.lastEvaluatedAt)}</dd>
                </div>
                <div>
                  <dt>Autonomy</dt>
                  <dd>{readable(item.autonomyLevel)}</dd>
                </div>
                <div>
                  <dt>Next cycle</dt>
                  <dd>
                    {item.nextRunAt
                      ? readableTime(item.nextRunAt)
                      : "Not scheduled"}
                  </dd>
                </div>
              </dl>

              <footer>
                <span className={healthClass(item)}>
                  <i /> {healthLabel(item)}
                </span>
                <Link
                  aria-label={`Open ${item.title} for ${item.accountName}`}
                  href={`/automations/${item.id}`}
                >
                  Open strategy →
                </Link>
              </footer>
            </article>
          ))}
        </section>
      )}

      <section
        className="strategy-fleet-safety"
        aria-label="Execution boundary"
      >
        <strong>Production execution remains locked.</strong>
        <p>
          Shadow records what an engine would consider. Paper simulates a
          position lifecycle. Neither mode can submit a broker order.
        </p>
      </section>
    </>
  );
}
