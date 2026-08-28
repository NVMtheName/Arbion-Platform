type Entity = Record<string, unknown> | null | undefined;

const maximumRoutes = 20;
const maximumSymbols = 100;
const maximumHorizons = 4;
const maximumBlockers = 20;
const maximumScheduleRuns = 12;
const maximumTextLength = 160;

type Props = {
  generatedAt: string;
  mandateId: string;
  currentVersion: number;
  mandateStatus: string;
  automationType: string;
  autonomyLevel: string;
  executionMode: string;
  financialAccountId: string;
  financialProvider: string;
  financialAccount?: Entity;
  financialConnection?: Entity;
  aiConnection?: Entity;
  modelId: string;
  capitalBucketId: string;
  capitalBucket?: Entity;
  instance?: Entity;
  schedule?: Entity;
  scheduleRuns?: unknown;
  schedulerEnabled: boolean;
  scorecard?: Entity;
  evidenceGate?: Entity;
  reconciliation?: Entity;
  automationBreaker?: Entity;
  breakerObserved: boolean;
};

type SafeRecord = Record<string, unknown>;

function value(entity: Entity, key: string, legacy: string) {
  return entity?.[key] ?? entity?.[legacy];
}

function text(input: unknown): string | null {
  if (typeof input !== "string") return null;
  const normalized = input.trim();
  if (!normalized || normalized.length > maximumTextLength) return null;
  return normalized;
}

function entityText(entity: Entity, key: string, legacy: string) {
  return text(value(entity, key, legacy));
}

function integer(input: unknown): number | null {
  return typeof input === "number" && Number.isSafeInteger(input) && input >= 0
    ? input
    : null;
}

function entityInteger(entity: Entity, key: string, legacy: string) {
  return integer(value(entity, key, legacy));
}

function entityFlag(entity: Entity, key: string, legacy: string) {
  const input = value(entity, key, legacy);
  return typeof input === "boolean" ? input : null;
}

function timestamp(input: unknown): string | null {
  const normalized = text(input);
  if (!normalized) return null;
  const parsed = new Date(normalized);
  return Number.isNaN(parsed.valueOf()) ? null : parsed.toISOString();
}

function entityTimestamp(entity: Entity, key: string, legacy: string) {
  return timestamp(value(entity, key, legacy));
}

function decimal(input: unknown): string | null {
  const normalized = text(input);
  return normalized && /^-?(0|[1-9]\d*)(\.\d+)?$/.test(normalized)
    ? normalized
    : null;
}

function entityDecimal(entity: Entity, key: string, legacy: string) {
  return decimal(value(entity, key, legacy));
}

function records(input: unknown, limit: number): SafeRecord[] {
  if (!Array.isArray(input)) return [];
  return input
    .slice(0, limit)
    .filter(
      (item): item is SafeRecord =>
        typeof item === "object" && item !== null && !Array.isArray(item),
    );
}

function strings(input: unknown, limit: number): string[] {
  if (!Array.isArray(input)) return [];
  return input
    .slice(0, limit)
    .map(text)
    .filter((item): item is string => item !== null);
}

function reportTimestamp(generatedAt: string) {
  return timestamp(generatedAt) ?? new Date(0).toISOString();
}

function safeInputText(input: string) {
  return text(input);
}

function normalizeHorizons(scorecard: Entity) {
  const source = value(scorecard, "horizons", "Horizons");
  return records(source, maximumHorizons).map((item) => ({
    horizon: entityText(item, "horizon", "Horizon"),
    sample_size: entityInteger(item, "sample_size", "SampleSize"),
    favorable_marks: entityInteger(item, "favorable_marks", "FavorableMarks"),
    unfavorable_marks: entityInteger(
      item,
      "unfavorable_marks",
      "UnfavorableMarks",
    ),
    flat_marks: entityInteger(item, "flat_marks", "FlatMarks"),
    favorable_rate_percent: entityDecimal(
      item,
      "favorable_rate_percent",
      "FavorableRatePercent",
    ),
    average_directional_change_percent: entityDecimal(
      item,
      "average_directional_change_percent",
      "AverageDirectionalChangePercent",
    ),
    first_evaluated_at: entityTimestamp(
      item,
      "first_evaluated_at",
      "FirstEvaluatedAt",
    ),
    last_evaluated_at: entityTimestamp(
      item,
      "last_evaluated_at",
      "LastEvaluatedAt",
    ),
    interpretation: entityText(item, "interpretation", "Interpretation"),
    minimum_sample_for_observational_label: entityInteger(
      item,
      "minimum_sample_for_observational_label",
      "MinimumSampleForObservationalLabel",
    ),
  }));
}

function normalizeRoutes(behavior: Entity) {
  const source = value(behavior, "routes", "Routes");
  return records(source, maximumRoutes).map((item) => ({
    ai_provider: entityText(item, "ai_provider", "AIProvider"),
    model_id: entityText(item, "model_id", "ModelID"),
    profile: entityText(item, "profile", "Profile"),
    provenance_status: entityText(
      item,
      "provenance_status",
      "ProvenanceStatus",
    ),
    total_decisions: entityInteger(item, "total_decisions", "TotalDecisions"),
    abstentions: entityInteger(item, "abstentions", "Abstentions"),
    proposed_decisions: entityInteger(
      item,
      "proposed_decisions",
      "ProposedDecisions",
    ),
    risk_held_decisions: entityInteger(
      item,
      "risk_held_decisions",
      "RiskHeldDecisions",
    ),
    repeat_action_cooldown_holds: entityInteger(
      item,
      "repeat_action_cooldown_holds",
      "RepeatActionCooldownHolds",
    ),
    would_have_submitted_decisions: entityInteger(
      item,
      "would_have_submitted_decisions",
      "WouldHaveSubmittedDecisions",
    ),
    one_hour_outcome_marks: entityInteger(
      item,
      "one_hour_outcome_marks",
      "OneHourOutcomeMarks",
    ),
    twenty_four_hour_outcome_marks: entityInteger(
      item,
      "twenty_four_hour_outcome_marks",
      "TwentyFourHourOutcomeMarks",
    ),
    measured_latency_decisions: entityInteger(
      item,
      "measured_latency_decisions",
      "MeasuredLatencyDecisions",
    ),
    average_latency_milliseconds: entityDecimal(
      item,
      "average_latency_milliseconds",
      "AverageLatencyMilliseconds",
    ),
    metered_usage_decisions: entityInteger(
      item,
      "metered_usage_decisions",
      "MeteredUsageDecisions",
    ),
    recorded_input_tokens: entityInteger(
      item,
      "recorded_input_tokens",
      "RecordedInputTokens",
    ),
    recorded_output_tokens: entityInteger(
      item,
      "recorded_output_tokens",
      "RecordedOutputTokens",
    ),
  }));
}

function normalizeSymbols(behavior: Entity) {
  const source = value(behavior, "symbols", "Symbols");
  return records(source, maximumSymbols).map((item) => ({
    symbol: entityText(item, "symbol", "Symbol"),
    proposed_decisions: entityInteger(
      item,
      "proposed_decisions",
      "ProposedDecisions",
    ),
    risk_held_decisions: entityInteger(
      item,
      "risk_held_decisions",
      "RiskHeldDecisions",
    ),
    would_have_submitted_decisions: entityInteger(
      item,
      "would_have_submitted_decisions",
      "WouldHaveSubmittedDecisions",
    ),
    one_hour_outcome_marks: entityInteger(
      item,
      "one_hour_outcome_marks",
      "OneHourOutcomeMarks",
    ),
    twenty_four_hour_outcome_marks: entityInteger(
      item,
      "twenty_four_hour_outcome_marks",
      "TwentyFourHourOutcomeMarks",
    ),
  }));
}

function normalizeScheduleRuns(input: unknown) {
  return records(input, maximumScheduleRuns).map((item) => ({
    run_id: entityText(item, "id", "ID"),
    mandate_version: entityInteger(item, "mandate_version", "MandateVersion"),
    execution_mode: entityText(item, "execution_mode", "ExecutionMode"),
    strategy_state: entityText(item, "strategy_state", "StrategyState"),
    scheduled_for: entityTimestamp(item, "scheduled_for", "ScheduledFor"),
    started_at: entityTimestamp(item, "started_at", "StartedAt"),
    completed_at: entityTimestamp(item, "completed_at", "CompletedAt"),
    next_run_at: entityTimestamp(item, "next_run_at", "NextRunAt"),
    status: entityText(item, "status", "Status"),
    error_code: entityText(item, "error_code", "ErrorCode"),
    ai_decision: entityText(item, "ai_decision", "AIDecision"),
    execution_status: entityText(item, "execution_status", "ExecutionStatus"),
    duplicate_recovered: entityFlag(
      item,
      "duplicate_recovered",
      "DuplicateRecovered",
    ),
    reconciliation_id: entityText(
      item,
      "reconciliation_id",
      "ReconciliationID",
    ),
    reconciliation_review_required: entityFlag(
      item,
      "reconciliation_review_required",
      "ReconciliationReviewRequired",
    ),
    consecutive_failures: entityInteger(
      item,
      "consecutive_failures",
      "ConsecutiveFailures",
    ),
  }));
}

export function buildAutonomyEvidenceReport(props: Props) {
  const behavior = (value(props.scorecard, "behavior", "Behavior") ??
    null) as SafeRecord | null;
  const sourceAvailability = {
    mandate: Boolean(
      safeInputText(props.mandateId) && props.currentVersion > 0,
    ),
    runtime: Boolean(entityText(props.instance, "id", "ID")),
    connections: Boolean(
      entityText(props.financialAccount, "status", "Status") &&
        entityText(props.financialConnection, "status", "Status") &&
        entityText(props.aiConnection, "status", "Status"),
    ),
    capital_policy: Boolean(
      entityText(props.capitalBucket, "id", "ID") &&
        entityText(props.capitalBucket, "status", "Status"),
    ),
    schedule: Boolean(
      entityText(props.schedule, "strategy_instance_id", "StrategyInstanceID"),
    ),
    reconciliation: Boolean(entityText(props.reconciliation, "id", "ID")),
    shadow_scorecard: Boolean(
      props.scorecard && entityText(props.evidenceGate, "status", "Status"),
    ),
    emergency_stop: props.breakerObserved,
  };
  const missingSources = Object.entries(sourceAvailability)
    .filter(([, available]) => !available)
    .map(([source]) => source);

  return {
    schema_version: "arbion.autonomy-evidence-report.v1",
    generated_at: reportTimestamp(props.generatedAt),
    report_status: missingSources.length === 0 ? "COMPLETE" : "PARTIAL",
    source: "OWNER_AUTHENTICATED_CURRENT_STATE",
    source_availability: sourceAvailability,
    missing_sources: missingSources,
    scope: {
      owner_scoped: true,
      mandate_id: safeInputText(props.mandateId),
      financial_account_id: safeInputText(props.financialAccountId),
      strategy_instance_id: entityText(props.instance, "id", "ID"),
    },
    mandate: {
      automation_type: safeInputText(props.automationType),
      version: integer(props.currentVersion),
      status: safeInputText(props.mandateStatus),
      autonomy_level: safeInputText(props.autonomyLevel),
      execution_mode: safeInputText(props.executionMode),
    },
    connections: {
      financial: {
        provider: safeInputText(props.financialProvider),
        account_status: entityText(props.financialAccount, "status", "Status"),
        connection_status: entityText(
          props.financialConnection,
          "status",
          "Status",
        ),
      },
      ai: {
        provider: entityText(props.aiConnection, "provider", "Provider"),
        model_id: safeInputText(props.modelId),
        connection_status: entityText(props.aiConnection, "status", "Status"),
        enabled: entityFlag(props.aiConnection, "enabled", "Enabled"),
      },
    },
    capital_policy: {
      bucket_id: safeInputText(props.capitalBucketId),
      status: entityText(props.capitalBucket, "status", "Status"),
      allocation_type: entityText(
        props.capitalBucket,
        "allocation_type",
        "AllocationType",
      ),
      allocation_value: entityDecimal(
        props.capitalBucket,
        "allocation_value",
        "AllocationValue",
      ),
      currency: entityText(props.capitalBucket, "currency", "Currency"),
      protected_amount: entityDecimal(
        props.capitalBucket,
        "protected_amount",
        "ProtectedAmount",
      ),
      is_reserve: entityFlag(props.capitalBucket, "is_reserve", "IsReserve"),
    },
    runtime: {
      status: entityText(props.instance, "status", "Status"),
      state: entityText(props.instance, "current_state", "CurrentState"),
      execution_mode: entityText(
        props.instance,
        "execution_mode",
        "ExecutionMode",
      ),
      mandate_version: entityInteger(
        props.instance,
        "mandate_version",
        "MandateVersion",
      ),
      last_evaluated_at: entityTimestamp(
        props.instance,
        "last_evaluated_at",
        "LastEvaluatedAt",
      ),
    },
    scheduler: {
      platform_enabled: props.schedulerEnabled,
      schedule_enabled: entityFlag(props.schedule, "enabled", "Enabled"),
      interval_minutes: entityInteger(
        props.schedule,
        "interval_minutes",
        "IntervalMinutes",
      ),
      session: entityText(props.schedule, "session", "Session"),
      last_status: entityText(props.schedule, "last_status", "LastStatus"),
      last_error_code: entityText(
        props.schedule,
        "last_error_code",
        "LastErrorCode",
      ),
      consecutive_failures: entityInteger(
        props.schedule,
        "consecutive_failures",
        "ConsecutiveFailures",
      ),
      last_completed_at: entityTimestamp(
        props.schedule,
        "last_completed_at",
        "LastCompletedAt",
      ),
      next_run_at: entityTimestamp(props.schedule, "next_run_at", "NextRunAt"),
      recent_runs: normalizeScheduleRuns(props.scheduleRuns),
      recent_runs_limit: maximumScheduleRuns,
    },
    broker_reconciliation: {
      reconciliation_id: entityText(props.reconciliation, "id", "ID"),
      provider: entityText(props.reconciliation, "provider", "Provider"),
      comparison_status: entityText(
        props.reconciliation,
        "comparison_status",
        "ComparisonStatus",
      ),
      balances_status: entityText(
        props.reconciliation,
        "balances_status",
        "BalancesStatus",
      ),
      positions_status: entityText(
        props.reconciliation,
        "positions_status",
        "PositionsStatus",
      ),
      autonomy_signal: entityText(
        props.reconciliation,
        "autonomy_signal",
        "AutonomySignal",
      ),
      autonomy_enforcement_active: entityFlag(
        props.reconciliation,
        "autonomy_enforcement_active",
        "AutonomyEnforcementActive",
      ),
      blocks_new_actions: entityFlag(
        props.reconciliation,
        "blocks_new_actions",
        "BlocksNewActions",
      ),
      observed_position_count: entityInteger(
        props.reconciliation,
        "observed_position_count",
        "ObservedPositionCount",
      ),
      change_count: entityInteger(
        props.reconciliation,
        "change_count",
        "ChangeCount",
      ),
      blocking_change_count: entityInteger(
        props.reconciliation,
        "blocking_change_count",
        "BlockingChangeCount",
      ),
      observed_at: entityTimestamp(
        props.reconciliation,
        "observed_at",
        "ObservedAt",
      ),
    },
    shadow_evidence: {
      total_marks: entityInteger(props.scorecard, "total_marks", "TotalMarks"),
      gate: {
        status: entityText(props.evidenceGate, "status", "Status"),
        blockers: strings(
          value(props.evidenceGate, "blockers", "Blockers"),
          maximumBlockers,
        ),
        one_hour_sample_size: entityInteger(
          props.evidenceGate,
          "one_hour_sample_size",
          "OneHourSampleSize",
        ),
        twenty_four_hour_sample_size: entityInteger(
          props.evidenceGate,
          "twenty_four_hour_sample_size",
          "TwentyFourHourSampleSize",
        ),
        minimum_sample_per_horizon: entityInteger(
          props.evidenceGate,
          "minimum_sample_per_horizon",
          "MinimumSamplePerHorizon",
        ),
        evidence_window_hours: entityInteger(
          props.evidenceGate,
          "evidence_window_hours",
          "EvidenceWindowHours",
        ),
        minimum_evidence_window_hours: entityInteger(
          props.evidenceGate,
          "minimum_evidence_window_hours",
          "MinimumEvidenceWindowHours",
        ),
        schedule_healthy: entityFlag(
          props.evidenceGate,
          "schedule_healthy",
          "ScheduleHealthy",
        ),
        last_schedule_status: entityText(
          props.evidenceGate,
          "last_schedule_status",
          "LastScheduleStatus",
        ),
        consecutive_schedule_failures: entityInteger(
          props.evidenceGate,
          "consecutive_schedule_failures",
          "ConsecutiveScheduleFailures",
        ),
        execution_boundary: entityText(
          props.evidenceGate,
          "execution_boundary",
          "ExecutionBoundary",
        ),
        live_execution_available: entityFlag(
          props.evidenceGate,
          "live_execution_available",
          "LiveExecutionAvailable",
        ),
      },
      horizons: normalizeHorizons(props.scorecard),
      behavior: {
        total_ai_decisions: entityInteger(
          behavior,
          "total_ai_decisions",
          "TotalAIDecisions",
        ),
        abstentions: entityInteger(behavior, "abstentions", "Abstentions"),
        proposed_decisions: entityInteger(
          behavior,
          "proposed_decisions",
          "ProposedDecisions",
        ),
        risk_held_decisions: entityInteger(
          behavior,
          "risk_held_decisions",
          "RiskHeldDecisions",
        ),
        repeat_action_cooldown_holds: entityInteger(
          behavior,
          "repeat_action_cooldown_holds",
          "RepeatActionCooldownHolds",
        ),
        would_have_submitted_decisions: entityInteger(
          behavior,
          "would_have_submitted_decisions",
          "WouldHaveSubmittedDecisions",
        ),
        attributed_decisions: entityInteger(
          behavior,
          "attributed_decisions",
          "AttributedDecisions",
        ),
        unattributed_legacy_decisions: entityInteger(
          behavior,
          "unattributed_legacy_decisions",
          "UnattributedLegacyDecisions",
        ),
        routes: normalizeRoutes(behavior),
        symbols: normalizeSymbols(behavior),
      },
    },
    emergency_stop: {
      state: props.breakerObserved
        ? (entityText(props.automationBreaker, "state", "State") ?? "CLOSED")
        : "UNAVAILABLE",
      engaged_at: entityTimestamp(
        props.automationBreaker,
        "engaged_at",
        "EngagedAt",
      ),
    },
    generation_boundary: {
      read_only: true,
      provider_called: false,
      model_called: false,
      strategy_mutated: false,
      broker_action_requested: false,
      order_created: false,
      live_execution_available: false,
      trade_authority_granted: false,
    },
    excluded_data: [
      "credentials and credential hints",
      "provider tokens and raw provider payloads",
      "portfolio balances, holdings, quantities, and position changes",
      "private prompts and model rationale",
      "broker order identifiers",
    ],
    limitations: [
      "This is a point-in-time owner review snapshot, not an execution authorization.",
      "A reviewable evidence gate is descriptive and does not establish profitability or readiness for live trading.",
      "Immutable decisions and outcomes remain authoritative in Arbion's Decision Journal.",
    ],
  };
}

function displayNumber(value: unknown) {
  return typeof value === "number" ? value.toLocaleString("en-US") : "—";
}

export function AutonomyEvidenceReport(props: Props) {
  const report = buildAutonomyEvidenceReport(props);
  const reportJSON = JSON.stringify(report, null, 2);
  const href = `data:application/json;charset=utf-8,${encodeURIComponent(reportJSON)}`;
  const shortMandate =
    safeInputText(props.mandateId)?.slice(0, 8) ?? "snapshot";
  const reportDate = report.generated_at.slice(0, 10);
  const evidence = report.shadow_evidence;
  const status = report.report_status === "COMPLETE" ? "Complete" : "Partial";

  return (
    <section
      className="autonomy-evidence-report"
      aria-labelledby="autonomy-evidence-report-heading"
    >
      <header>
        <div>
          <p className="eyebrow">OWNER EVIDENCE REPORT</p>
          <h2 id="autonomy-evidence-report-heading">
            Save the current autonomy review.
          </h2>
          <p>
            Arbion packages the mandate, control state, scheduler health,
            reconciliation status, and bounded Shadow scorecard into one
            private, point-in-time report.
          </p>
        </div>
        <span
          className={`autonomy-evidence-report-status is-${report.report_status.toLowerCase()}`}
        >
          {status} snapshot
        </span>
      </header>

      <div className="autonomy-evidence-report-facts">
        <div>
          <span>Evidence gate</span>
          <strong>{evidence.gate.status ?? "Unavailable"}</strong>
        </div>
        <div>
          <span>Immutable marks</span>
          <strong>{displayNumber(evidence.total_marks)}</strong>
        </div>
        <div>
          <span>AI decisions</span>
          <strong>{displayNumber(evidence.behavior.total_ai_decisions)}</strong>
        </div>
        <div>
          <span>Missing sources</span>
          <strong>{report.missing_sources.length}</strong>
        </div>
      </div>

      <footer>
        <div>
          <a
            className="button-link autonomy-evidence-download"
            download={`arbion-autonomy-evidence-${shortMandate}-${reportDate}.json`}
            href={href}
          >
            Save evidence report
          </a>
          <small>
            Generated {new Date(report.generated_at).toUTCString()} · keep this
            file private.
          </small>
        </div>
        <p>
          Creating this report does not call a provider or model, change the
          strategy, request a broker action, or grant live-trading authority.
          Credentials, holdings, quantities, raw provider data, and private
          rationale are excluded.
        </p>
      </footer>
    </section>
  );
}
