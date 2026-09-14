import type { ScheduleRunRecord } from "./schedule-run-history";

type Evidence = {
  schema_version: 1;
  provider: "schwab";
  financial_account_id: string;
  requested_symbol: string;
  quote_type: "NBBO" | "NFL" | "UNAVAILABLE" | "UNRECOGNIZED";
  realtime: boolean | null;
  provider_observed_at: string | null;
  evaluated_at: string;
  rejection_code: string;
  before_model: true;
};

const keys = [
  "schema_version",
  "provider",
  "financial_account_id",
  "requested_symbol",
  "quote_type",
  "realtime",
  "provider_observed_at",
  "evaluated_at",
  "rejection_code",
  "before_model",
];
const codes = [
  "MARKET_DATA_INVALID",
  "MARKET_DATA_STALE",
  "MARKET_DATA_DELAYED",
  "MARKET_DATA_REALTIME_UNCONFIRMED",
];

function savedTime(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
  );
}

function validated(run: ScheduleRunRecord, provider?: string): Evidence | null {
  const value = run.quote_rejection;
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const e = value as Record<string, unknown>;
  if (
    Object.keys(e).length !== keys.length ||
    !keys.every((key) => key in e) ||
    e.schema_version !== 1 ||
    e.provider !== "schwab" ||
    provider !== "schwab" ||
    e.before_model !== true ||
    run.status !== "FAILED" ||
    !["PAPER", "SHADOW"].includes(run.execution_mode) ||
    run.strategy_state !== "AI_MONITORING" ||
    run.ai_decision ||
    run.execution_status ||
    run.duplicate_recovered ||
    e.rejection_code !== run.error_code ||
    !codes.includes(String(e.rejection_code)) ||
    typeof e.financial_account_id !== "string" ||
    !/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/.test(
      e.financial_account_id,
    ) ||
    typeof e.requested_symbol !== "string" ||
    !/^[A-Z0-9][A-Z0-9./$^-]{0,31}$/.test(e.requested_symbol) ||
    !["NBBO", "NFL", "UNAVAILABLE", "UNRECOGNIZED"].includes(
      String(e.quote_type),
    ) ||
    (e.realtime !== null && typeof e.realtime !== "boolean") ||
    (e.rejection_code === "MARKET_DATA_DELAYED" && e.realtime !== false) ||
    (e.rejection_code === "MARKET_DATA_REALTIME_UNCONFIRMED" &&
      e.realtime !== null) ||
    !savedTime(e.evaluated_at) ||
    !savedTime(run.started_at) ||
    !savedTime(run.completed_at) ||
    Date.parse(e.evaluated_at) < Date.parse(run.started_at) ||
    Date.parse(e.evaluated_at) > Date.parse(run.completed_at) ||
    (e.provider_observed_at !== null && !savedTime(e.provider_observed_at))
  )
    return null;
  return e as Evidence;
}

export function QuoteRejectionEvidence({
  run,
  financialProvider,
}: {
  run: ScheduleRunRecord;
  financialProvider?: string;
}) {
  if (!run.quote_rejection && !codes.includes(run.error_code ?? ""))
    return null;
  const e = validated(run, financialProvider);
  return (
    <details className="schedule-quote-evidence" open>
      <summary>Saved quote rejection detail</summary>
      {!e ? (
        <p>
          Quote detail UNAVAILABLE for this saved run. Earlier records are not
          reconstructed.
        </p>
      ) : (
        <>
          <p>
            Schwab returned this metadata for the requested {e.requested_symbol}{" "}
            quote. The engine stopped before calling the model. A recent
            timestamp alone does not satisfy the real-time check.
          </p>
          <dl>
            <div>
              <dt>Provider / quote type</dt>
              <dd>Schwab / {e.quote_type}</dd>
            </div>
            <div>
              <dt>Provider real-time flag</dt>
              <dd>
                {e.realtime === null
                  ? "UNAVAILABLE (not supplied)"
                  : String(e.realtime)}
              </dd>
            </div>
            <div>
              <dt>Provider observation (UTC)</dt>
              <dd>{e.provider_observed_at ?? "UNAVAILABLE"}</dd>
            </div>
            <div>
              <dt>Evaluation time (UTC)</dt>
              <dd>{e.evaluated_at}</dd>
            </div>
          </dl>
          {e.provider_observed_at &&
            Date.parse(e.provider_observed_at) > Date.parse(e.evaluated_at) && (
              <p>
                The provider timestamp is ahead of the evaluation time; it is
                preserved as rejected evidence, not a freshness claim.
              </p>
            )}
          <p>
            These are saved provider fields, not proof of a specific delay,
            subscription, or entitlement. No balances, prices, or credentials
            are included.
          </p>
          <small>Immutable scheduler record {run.id}</small>
        </>
      )}
    </details>
  );
}
