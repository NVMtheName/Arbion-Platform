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
  evidenceAvailable?: boolean;
  evidenceStatus?: string;
  oneHourSampleSize?: number;
  twentyFourHourSampleSize?: number;
  minimumSamplePerHorizon?: number;
  evidenceWindowHours?: number;
  minimumEvidenceWindowHours?: number;
  evidenceScheduleHealthy?: boolean;
  evidenceBlockers?: string[];
  currentEvidenceReviewed?: boolean;
  decisionAvailable?: boolean;
  latestDecisionType?: string;
  latestDecisionAt?: string;
  latestDecisionSymbol?: string;
  latestDecisionSide?: string;
  latestDecisionQuantity?: string;
  latestDecisionRiskDecision?: string;
  latestDecisionRiskReasons?: string[];
  latestDecisionExecutionStatus?: string;
  latestDecisionAIProvider?: string;
  latestDecisionAIModelID?: string;
  latestDecisionAIProfile?: string;
  latestDecisionLatencyMS?: number;
  latestDecisionInputUsage?: number;
  latestDecisionOutputUsage?: number;
  accountContextAvailable?: boolean;
  instanceContextAvailable?: boolean;
};

type StrategyFleetProps = {
  items: StrategyFleetItem[];
  inventoryAvailable?: boolean;
  contextWarnings?: string[];
};

export type StrategyFleetNextAction = {
  key: string;
  priority: number;
  tone: "ATTENTION" | "READY" | "PAUSED";
  eyebrow: string;
  title: string;
  detail: string;
  href: string;
  actionLabel: string;
  accountName?: string;
  provider?: string;
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

function isAutonomous(item: StrategyFleetItem) {
  return (
    item.autonomyLevel === "FULL_AUTONOMOUS" ||
    item.autonomyLevel === "STRATEGY_AUTONOMOUS"
  );
}

function historicalInstance(item: StrategyFleetItem) {
  return Boolean(
    item.instanceStatus &&
      !["ACTIVE", "PAUSED", "ERROR"].includes(item.instanceStatus),
  );
}

function needsReview(item: StrategyFleetItem) {
  return (
    item.instanceStatus === "ERROR" ||
    item.currentState === "ERROR" ||
    item.accountContextAvailable === false ||
    item.instanceContextAvailable === false ||
    item.scheduleAvailable === false ||
    item.evidenceAvailable === false ||
    item.decisionAvailable === false ||
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
  if (historicalInstance(item)) return "New version required";
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
  if (item.evidenceAvailable === false) return "Evidence status unavailable";
  if (item.decisionAvailable === false) return "Decision status unavailable";
  if (item.scheduleStatus === "FAILED" || item.consecutiveFailures > 0)
    return "Schedule needs review";
  if (item.instanceStatus === "ERROR" || item.currentState === "ERROR")
    return "Engine needs review";
  if (item.instanceStatus === "PAUSED") return "Paused by owner";
  if (historicalInstance(item)) return "Historical version complete";
  if (item.scheduleEnabled && item.nextRunAt) return "Healthy schedule";
  if (item.instanceStatus === "ACTIVE") return "Manual cycles only";
  if (item.mandateStatus === "READY") return "Ready to initialize";
  return "Draft configuration";
}

export function selectStrategyFleetNextAction(
  item: StrategyFleetItem,
): StrategyFleetNextAction | null {
  const destination = `/automations/${item.id}`;
  const identity = {
    key: item.id,
    accountName: item.accountName,
    provider: item.provider,
  };

  if (
    item.accountContextAvailable === false ||
    item.instanceContextAvailable === false
  ) {
    return {
      key: "CURRENT_FLEET_CONTEXT",
      priority: 0,
      tone: "ATTENTION",
      eyebrow: "CURRENT CONTEXT UNAVAILABLE",
      title: "Refresh the current fleet context",
      detail:
        "Arbion will not infer owner actions from a partial account or strategy inventory. No mandate or schedule was changed.",
      href: "/automations",
      actionLabel: "Refresh automations",
    };
  }
  if (item.instanceStatus === "ERROR" || item.currentState === "ERROR") {
    return {
      ...identity,
      priority: 1,
      tone: "ATTENTION",
      eyebrow: "ENGINE REVIEW REQUIRED",
      title: `Review ${item.title} runtime evidence`,
      detail:
        "The non-live instance is in an error state. Arbion stopped the cycle and did not send a broker order.",
      href: `${destination}#runtime-evidence`,
      actionLabel: "Review runtime evidence",
    };
  }
  if (
    item.scheduleAvailable === false ||
    item.scheduleStatus === "FAILED" ||
    item.consecutiveFailures > 0
  ) {
    return {
      ...identity,
      priority: 2,
      tone: "ATTENTION",
      eyebrow: "SCHEDULE NEEDS REVIEW",
      title: `Review ${item.title} schedule health`,
      detail:
        item.scheduleAvailable === false
          ? "Arbion could not refresh the current schedule record and will not label this engine healthy."
          : `${item.consecutiveFailures} consecutive ${item.consecutiveFailures === 1 ? "failure" : "failures"}. The scheduler failed closed without broker execution.`,
      href: `${destination}#schedule-controls`,
      actionLabel: "Review schedule",
    };
  }
  if (item.evidenceAvailable === false) {
    return {
      ...identity,
      priority: 3,
      tone: "ATTENTION",
      eyebrow: "EVIDENCE STATUS UNAVAILABLE",
      title: `Refresh ${item.title} evidence`,
      detail:
        "Arbion could not refresh the immutable Shadow scorecard and will not infer its sample or review status.",
      href: destination,
      actionLabel: "Refresh evidence",
    };
  }
  if (item.decisionAvailable === false) {
    return {
      ...identity,
      priority: 4,
      tone: "ATTENTION",
      eyebrow: "DECISION STATUS UNAVAILABLE",
      title: `Refresh ${item.title} decision pulse`,
      detail:
        "Arbion could not refresh the bounded immutable journal window and will not infer a recent AI action.",
      href: destination,
      actionLabel: "Refresh decision pulse",
    };
  }
  if (
    item.evidenceStatus === "EVIDENCE_REVIEWABLE" &&
    !item.currentEvidenceReviewed
  ) {
    return {
      ...identity,
      priority: 5,
      tone: "READY",
      eyebrow: "SHADOW EVIDENCE REVIEWABLE",
      title: `Review ${item.title} evidence`,
      detail:
        "The exact non-live sample and time-window gate has passed. Review is an acknowledgment only and grants no trading authority.",
      href: `${destination}#shadow-evidence-review`,
      actionLabel: "Open evidence review",
    };
  }
  if (item.mandateStatus === "DRAFT") {
    return {
      ...identity,
      priority: 10,
      tone: "READY",
      eyebrow: "DRAFT REVIEW",
      title: `Finish reviewing ${item.title}`,
      detail:
        "Review the exact saved version and mark it Ready before any non-live instance can initialize.",
      href: `${destination}#mandate-lifecycle-controls`,
      actionLabel: "Review draft",
    };
  }
  if (historicalInstance(item)) {
    return {
      ...identity,
      priority: 20,
      tone: "READY",
      eyebrow: "HISTORICAL VERSION COMPLETE",
      title: `Create the next ${item.title} version`,
      detail:
        "This immutable version already produced a historical instance. A new bounded version is required before initialization.",
      href: `${destination}#configuration-controls`,
      actionLabel: "Open version controls",
    };
  }
  if (!item.instanceStatus && item.mandateStatus === "READY") {
    return {
      ...identity,
      priority: 30,
      tone: "READY",
      eyebrow: "READY FOR NON-LIVE INITIALIZATION",
      title: `Initialize ${item.title}`,
      detail:
        "The mandate is Ready for its exact initialization checks. This creates no broker order or live authority.",
      href: `${destination}#strategy-readiness-check`,
      actionLabel: "Review initialization",
    };
  }
  if (item.instanceStatus === "PAUSED") {
    return {
      ...identity,
      priority: 40,
      tone: "PAUSED",
      eyebrow: "PAUSED BY OWNER",
      title: `Decide when to resume ${item.title}`,
      detail:
        "The non-live capital claim remains protected while evaluations are stopped. Resume only when you choose.",
      href: `${destination}#strategy-instance-controls`,
      actionLabel: "Open resume controls",
    };
  }
  if (item.instanceStatus === "ACTIVE" && isAutonomous(item)) {
    if (!item.scheduleEnabled || !item.nextRunAt) {
      return {
        ...identity,
        priority: 50,
        tone: "READY",
        eyebrow: "AUTOMATIC CYCLES ARE OFF",
        title: `Configure ${item.title} scheduling`,
        detail:
          "The non-live engine is active, but it has no current automatic cycle. Review and save a bounded schedule.",
        href: `${destination}#schedule-controls`,
        actionLabel: "Open schedule controls",
      };
    }
    return null;
  }
  if (item.instanceStatus === "ACTIVE") {
    return {
      ...identity,
      priority: 60,
      tone: "READY",
      eyebrow: "OWNER-INVOKED STRATEGY",
      title: `Evaluate ${item.title} when ready`,
      detail:
        "This strategy waits for an owner-invoked PAPER or SHADOW evaluation through the deterministic risk gate.",
      href: `${destination}#manual-evaluation`,
      actionLabel: "Open evaluation controls",
    };
  }
  return {
    ...identity,
    priority: 70,
    tone: "ATTENTION",
    eyebrow: "MANDATE REVIEW REQUIRED",
    title: `Review ${item.title}`,
    detail:
      "The current lifecycle state does not map to an automatic owner action. Review the immutable mandate before proceeding.",
    href: destination,
    actionLabel: "Open strategy",
  };
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

function boundedProgress(
  value: number | undefined,
  maximum: number | undefined,
) {
  if (
    value === undefined ||
    maximum === undefined ||
    !Number.isFinite(value) ||
    !Number.isFinite(maximum) ||
    value < 0 ||
    maximum <= 0
  )
    return undefined;
  return { value: Math.min(value, maximum), maximum };
}

function latestDecisionLabel(value: string) {
  if (value === "ABSTAIN") return "Abstained";
  if (value === "DENY_RISK_DENIED") return "Held by controls";
  if (value === "ALLOW_WOULD_HAVE_SUBMITTED") return "Would have submitted";
  return readable(value);
}

function latestActionLabel(item: StrategyFleetItem) {
  const symbol =
    item.latestDecisionSymbol && item.latestDecisionSymbol !== "NONE"
      ? item.latestDecisionSymbol
      : undefined;
  if (!symbol) return "No action proposed";
  return [
    item.latestDecisionSide && readable(item.latestDecisionSide),
    item.latestDecisionQuantity,
    symbol,
  ]
    .filter(Boolean)
    .join(" · ");
}

function latestRiskLabel(item: StrategyFleetItem) {
  if (item.latestDecisionRiskDecision === "ALLOW")
    return "Allowed by deterministic controls";
  if (item.latestDecisionRiskDecision === "DENY") {
    const [first, ...remaining] = item.latestDecisionRiskReasons ?? [];
    if (!first) return "Denied by deterministic controls";
    return `${readable(first)}${remaining.length ? ` +${remaining.length}` : ""}`;
  }
  return "Risk gate not reached";
}

function latestExecutionLabel(value?: string) {
  if (value === "WOULD_HAVE_SUBMITTED") return "Shadow record only";
  if (!value) return "No execution record";
  return `${readable(value)} · non-live`;
}

function aiProviderLabel(value: string) {
  if (value === "openai") return "OpenAI";
  if (value === "anthropic") return "Claude";
  if (value === "gemini") return "Gemini";
  return readable(value);
}

function latestRouteLabel(item: StrategyFleetItem) {
  if (
    !item.latestDecisionAIProvider ||
    !item.latestDecisionAIModelID ||
    !item.latestDecisionAIProfile
  )
    return "Unattributed legacy route";
  return `${aiProviderLabel(item.latestDecisionAIProvider)} · ${item.latestDecisionAIModelID} · ${readable(item.latestDecisionAIProfile)}`;
}

function latestTelemetryLabel(item: StrategyFleetItem) {
  const values = [
    item.latestDecisionLatencyMS,
    item.latestDecisionInputUsage,
    item.latestDecisionOutputUsage,
  ];
  if (
    values.some(
      (value) => value === undefined || !Number.isFinite(value) || value < 0,
    )
  )
    return "Telemetry unavailable";
  const integer = new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 });
  return `${integer.format(item.latestDecisionLatencyMS ?? 0)} ms · ${integer.format(item.latestDecisionInputUsage ?? 0)} in / ${integer.format(item.latestDecisionOutputUsage ?? 0)} out`;
}

function StrategyFleetDecision({ item }: { item: StrategyFleetItem }) {
  if (!isAI(item)) return null;
  if (!item.instanceStatus) {
    return (
      <section
        className="strategy-fleet-decision is-pending"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Decision pulse begins after initialization.</strong>
        <p>No model activity is inferred before an instance exists.</p>
      </section>
    );
  }
  if (item.decisionAvailable === false) {
    return (
      <section
        className="strategy-fleet-decision is-unavailable"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Latest decision unavailable</strong>
        <p>No recent AI action is inferred from a failed journal refresh.</p>
      </section>
    );
  }
  if (!item.latestDecisionType) {
    return (
      <section
        className="strategy-fleet-decision is-pending"
        aria-label={`${item.title} latest AI decision`}
      >
        <strong>Awaiting a completed AI decision</strong>
        <p>No AI entry appears in the latest 10 immutable journal records.</p>
      </section>
    );
  }

  const riskTone = ["ALLOW", "DENY"].includes(
    item.latestDecisionRiskDecision ?? "",
  )
    ? item.latestDecisionRiskDecision?.toLowerCase()
    : "neutral";
  return (
    <section
      className={`strategy-fleet-decision is-${riskTone}`}
      aria-label={`${item.title} latest AI decision`}
    >
      <header>
        <span>LATEST AI DECISION</span>
        <strong>{latestDecisionLabel(item.latestDecisionType)}</strong>
      </header>
      <dl>
        <div>
          <dt>Action</dt>
          <dd>{latestActionLabel(item)}</dd>
        </div>
        <div>
          <dt>Risk gate</dt>
          <dd>{latestRiskLabel(item)}</dd>
        </div>
        <div>
          <dt>AI route</dt>
          <dd>{latestRouteLabel(item)}</dd>
        </div>
        <div>
          <dt>Route telemetry</dt>
          <dd>{latestTelemetryLabel(item)}</dd>
        </div>
      </dl>
      <p>
        <time dateTime={item.latestDecisionAt}>
          {readableTime(item.latestDecisionAt)}
        </time>
        <span>{latestExecutionLabel(item.latestDecisionExecutionStatus)}</span>
        <Link href={`/automations/${item.id}#decision-journal`}>
          Decision journal →
        </Link>
      </p>
    </section>
  );
}

function StrategyFleetEvidence({ item }: { item: StrategyFleetItem }) {
  if (!isAI(item)) return null;
  if (!item.instanceStatus) {
    return (
      <section
        className="strategy-fleet-evidence is-pending"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Shadow evidence begins after initialization.</strong>
        <p>No outcome samples are inferred before an instance exists.</p>
      </section>
    );
  }
  if (item.evidenceAvailable === false || !item.evidenceStatus) {
    return (
      <section
        className="strategy-fleet-evidence is-unavailable"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Evidence status unavailable</strong>
        <p>Sample counts are hidden until the exact scorecard refreshes.</p>
      </section>
    );
  }

  const oneHour = boundedProgress(
    item.oneHourSampleSize,
    item.minimumSamplePerHorizon,
  );
  const twentyFourHour = boundedProgress(
    item.twentyFourHourSampleSize,
    item.minimumSamplePerHorizon,
  );
  const window = boundedProgress(
    item.evidenceWindowHours,
    item.minimumEvidenceWindowHours,
  );
  const valid = oneHour && twentyFourHour && window;
  if (!valid) {
    return (
      <section
        className="strategy-fleet-evidence is-unavailable"
        aria-label={`${item.title} Shadow evidence`}
      >
        <strong>Evidence contract incomplete</strong>
        <p>Arbion will not estimate a missing sample target or time window.</p>
      </section>
    );
  }

  const reviewable = item.evidenceStatus === "EVIDENCE_REVIEWABLE";
  return (
    <section
      className={`strategy-fleet-evidence${reviewable ? " is-reviewable" : ""}`}
      aria-label={`${item.title} Shadow evidence`}
    >
      <header>
        <span>SHADOW EVIDENCE</span>
        <strong>
          {reviewable
            ? item.currentEvidenceReviewed
              ? "Current snapshot reviewed"
              : "Reviewable"
            : "Collecting"}
        </strong>
      </header>
      <div>
        <label>
          <span>1-hour marks</span>
          <strong>
            {item.oneHourSampleSize} / {item.minimumSamplePerHorizon}
          </strong>
          <progress
            aria-label="1-hour Shadow outcome sample progress"
            max={oneHour.maximum}
            value={oneHour.value}
          />
        </label>
        <label>
          <span>24-hour marks</span>
          <strong>
            {item.twentyFourHourSampleSize} / {item.minimumSamplePerHorizon}
          </strong>
          <progress
            aria-label="24-hour Shadow outcome sample progress"
            max={twentyFourHour.maximum}
            value={twentyFourHour.value}
          />
        </label>
        <label>
          <span>Evidence window</span>
          <strong>
            {item.evidenceWindowHours} / {item.minimumEvidenceWindowHours}h
          </strong>
          <progress
            aria-label="Shadow evidence window progress"
            max={window.maximum}
            value={window.value}
          />
        </label>
      </div>
      <p>
        Gate schedule {item.evidenceScheduleHealthy ? "verified" : "pending"}
        {item.evidenceBlockers?.length
          ? ` · ${item.evidenceBlockers.length} remaining ${item.evidenceBlockers.length === 1 ? "condition" : "conditions"}`
          : " · exact gate complete"}
        <span>Reviewability only · never live authority</span>
      </p>
    </section>
  );
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
  const nextActions = Array.from(
    new Map(
      items
        .map(selectStrategyFleetNextAction)
        .filter((action): action is StrategyFleetNextAction => Boolean(action))
        .sort((left, right) =>
          left.priority !== right.priority
            ? left.priority - right.priority
            : left.title.localeCompare(right.title),
        )
        .map((action) => [action.key, action]),
    ).values(),
  );

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

      {inventoryAvailable && ordered.length > 0 && (
        <section
          className={`strategy-fleet-action-queue${nextActions.length === 0 ? " is-clear" : ""}`}
          aria-labelledby="strategy-fleet-action-heading"
        >
          <header>
            <div>
              <p className="eyebrow">OWNER ACTION QUEUE</p>
              <h2 id="strategy-fleet-action-heading">
                {nextActions.length === 0
                  ? "No owner action right now."
                  : "Your clearest path forward."}
              </h2>
              <p>
                {nextActions.length === 0
                  ? "Every current autonomous engine has a healthy next cycle or no actionable exception."
                  : "Ordered by safety and lifecycle priority from the complete current fleet snapshot."}
              </p>
            </div>
            <span>
              {nextActions.length} owner{" "}
              {nextActions.length === 1 ? "step" : "steps"}
            </span>
          </header>
          {nextActions.length > 0 && (
            <ol>
              {nextActions.map((action, index) => (
                <li
                  className={`is-${action.tone.toLowerCase()}`}
                  key={action.key}
                >
                  <span aria-hidden="true">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  <div>
                    <small>
                      {action.provider && action.accountName
                        ? `${providerLabel(action.provider)} · ${action.accountName}`
                        : action.eyebrow}
                    </small>
                    <h3>{action.title}</h3>
                    <p>{action.detail}</p>
                  </div>
                  <Link href={action.href}>{action.actionLabel} →</Link>
                </li>
              ))}
            </ol>
          )}
          <footer>
            Guidance only · opening a control does not run a cycle or authorize
            an order
          </footer>
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

              <StrategyFleetDecision item={item} />
              <StrategyFleetEvidence item={item} />

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
