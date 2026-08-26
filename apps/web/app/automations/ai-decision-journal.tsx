type RawDecision = Record<string, unknown>;

type Rationale = {
  decision?: string;
  symbol?: string;
  side?: string;
  proposed_notional?: string;
  confidence?: string;
  thesis?: string;
  risk_flags?: string[];
  limitations?: string[];
  market_observed_at?: string;
  input_evidence?: Record<string, unknown>;
};

function read(entry: RawDecision, key: string, legacy: string) {
  return entry[key] ?? entry[legacy];
}

function label(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function timestamp(value: unknown) {
  if (typeof value !== "string") return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "—";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(parsed);
}

function rationale(entry: RawDecision): Rationale {
  const value = read(entry, "structured_rationale", "StructuredRationale");
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Rationale)
    : {};
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function records(value: unknown) {
  return Array.isArray(value)
    ? value.filter(
        (item): item is Record<string, unknown> =>
          Boolean(item) && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function field(entry: RawDecision, key: string, legacy: string) {
  const value = read(entry, key, legacy);
  return typeof value === "string" ? value : undefined;
}

function decimal(value: string | undefined) {
  if (!value || !/^-?\d+(\.\d+)?$/.test(value)) return "—";
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function dollars(value: string | undefined) {
  const formatted = decimal(value);
  return formatted === "—" ? formatted : `$${formatted}`;
}

function signedDollars(value: string | undefined) {
  const formatted = decimal(value);
  if (formatted === "—") return formatted;
  if (formatted.startsWith("-")) return `-$${formatted.slice(1)}`;
  if (formatted === "0") return "$0";
  return `+$${formatted}`;
}

function signedPercent(value: string | undefined) {
  const formatted = decimal(value);
  if (formatted === "—") return formatted;
  if (formatted.startsWith("-") || formatted === "0") return `${formatted}%`;
  return `+${formatted}%`;
}

function contextValue(entry: Record<string, unknown>, key: string) {
  const value = entry[key];
  return typeof value === "string" ? value : undefined;
}

function contextNumber(entry: Record<string, unknown>, key: string) {
  const value = entry[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function historyCoverage(market: Record<string, unknown>) {
  const status = contextValue(market, "history_status") ?? "UNAVAILABLE";
  if (status === "UNAVAILABLE") return "History unavailable";
  if (status === "STALE") return "History stale — not used";
  const available = contextNumber(market, "history_contiguous_intervals");
  const expected = contextNumber(market, "history_expected_intervals");
  const seconds = contextNumber(market, "history_granularity_seconds");
  const interval = seconds > 0 ? `${seconds / 60}m` : "timed";
  return `${label(status)} history · ${available}/${expected} exact ${interval} candles`;
}

function marketPrice(market: Record<string, unknown>) {
  return ["mark", "last", "bid", "ask"]
    .map((key) => contextValue(market, key))
    .find((value) => value && value !== "0");
}

function elapsedLabel(value: unknown) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0)
    return "Elapsed time unavailable";
  const hours = Math.floor(value / 3600);
  const minutes = Math.floor((value % 3600) / 60);
  return `Observed ${hours}h${minutes ? ` ${minutes}m` : ""} after proposal`;
}

export function AIDecisionJournal({
  decisions,
  outcomes = [],
}: {
  decisions: RawDecision[];
  outcomes?: RawDecision[];
}) {
  const entries = decisions
    .filter((entry) => read(entry, "source", "Source") === "AI")
    .slice(0, 5);

  return (
    <section aria-label="AI Decision Journal">
      <p className="eyebrow">AI DECISION JOURNAL</p>
      <h2>Latest autonomous reasoning</h2>
      <p>
        Every completed model decision is immutable. Abstentions stop before
        risk evaluation; proposals must still pass Arbion&apos;s deterministic
        controls and remain shadow-only.
      </p>

      {entries.length === 0 ? (
        <p className="security-note">
          No AI decision has completed yet. Provider or rate-limit failures do
          not create journal evidence and never create an order.
        </p>
      ) : (
        <div className="journal-timeline">
          {entries.map((entry, index) => {
            const facts = rationale(entry);
            const decisionType = String(
              read(entry, "decision_type", "DecisionType") ?? "UNKNOWN",
            );
            const riskReached = Boolean(
              read(entry, "risk_evaluation_id", "RiskEvaluationID"),
            );
            const executionRecordID = field(
              entry,
              "execution_record_id",
              "ExecutionRecordID",
            );
            const executionRecorded = Boolean(executionRecordID);
            const riskDecision = field(entry, "risk_decision", "RiskDecision");
            const riskDenied = riskDecision === "DENY";
            const controlReasons = strings(
              read(entry, "risk_reason_codes", "RiskReasonCodes"),
            );
            const executionStatus = field(
              entry,
              "execution_status",
              "ExecutionStatus",
            );
            const shadowAttemptRecorded =
              executionRecorded && executionStatus === "WOULD_HAVE_SUBMITTED";
            const executionSymbol = field(entry, "symbol", "Symbol");
            const side = field(entry, "side", "Side");
            const quantity = field(entry, "quantity", "Quantity");
            const price = field(entry, "price", "Price");
            const notional = field(entry, "notional", "Notional");
            const riskFlags = strings(facts.risk_flags);
            const limitations = strings(facts.limitations);
            const inputEvidence = facts.input_evidence;
            const evidencePositions = records(inputEvidence?.positions);
            const evidenceMarkets = records(inputEvidence?.markets);
            const symbol =
              facts.symbol && facts.symbol !== "NONE"
                ? facts.symbol
                : "No action";
            const entryID = String(read(entry, "id", "ID") ?? index);
            const outcomeMarks = executionRecordID
              ? outcomes
                  .filter(
                    (outcome) =>
                      field(
                        outcome,
                        "execution_record_id",
                        "ExecutionRecordID",
                      ) === executionRecordID,
                  )
                  .sort((left, right) =>
                    field(left, "horizon", "Horizon") === "ONE_HOUR"
                      ? -1
                      : field(right, "horizon", "Horizon") === "ONE_HOUR"
                        ? 1
                        : 0,
                  )
              : [];

            return (
              <article className="journal-entry" key={entryID}>
                <header>
                  <div>
                    <time
                      dateTime={String(
                        read(entry, "created_at", "CreatedAt") ?? "",
                      )}
                    >
                      {timestamp(read(entry, "created_at", "CreatedAt"))} UTC
                    </time>
                    <h3>
                      {riskDenied ? "Held by controls" : label(decisionType)} ·{" "}
                      {symbol}
                    </h3>
                    <p>{facts.thesis ?? "No thesis was recorded."}</p>
                  </div>
                  <div className="journal-badges">
                    <span className="journal-mode shadow">SHADOW</span>
                    <span
                      className={`journal-decision ${riskDenied ? "deny" : riskReached ? "allow" : "abstain"}`}
                    >
                      {riskDenied
                        ? "Held by controls"
                        : riskReached
                          ? "Risk checked"
                          : "Safe abstention"}
                    </span>
                  </div>
                </header>

                <dl className="journal-facts">
                  <div>
                    <dt>Model decision</dt>
                    <dd>{label(facts.decision ?? decisionType)}</dd>
                  </div>
                  <div>
                    <dt>Confidence</dt>
                    <dd>{facts.confidence ? label(facts.confidence) : "—"}</dd>
                  </div>
                  <div>
                    <dt>Proposed notional</dt>
                    <dd>${facts.proposed_notional ?? "0"}</dd>
                  </div>
                  <div>
                    <dt>Market observed</dt>
                    <dd>{timestamp(facts.market_observed_at)} UTC</dd>
                  </div>
                  <div>
                    <dt>Risk gate</dt>
                    <dd>{riskReached ? "Evaluated" : "Not reached"}</dd>
                  </div>
                  <div>
                    <dt>Execution evidence</dt>
                    <dd>
                      {riskDenied
                        ? "Control denial"
                        : shadowAttemptRecorded
                          ? "Shadow record"
                          : "None"}
                    </dd>
                  </div>
                </dl>

                {riskDenied && controlReasons.length > 0 && (
                  <div className="journal-reasons">
                    <strong>Control gate</strong>
                    <ul>
                      {controlReasons.map((reason) => (
                        <li key={reason}>{label(reason)}</li>
                      ))}
                    </ul>
                  </div>
                )}

                {inputEvidence && (
                  <details className="journal-input-evidence">
                    <summary>
                      Evidence considered · {evidencePositions.length}{" "}
                      allowlisted holding
                      {evidencePositions.length === 1 ? "" : "s"} ·{" "}
                      {evidenceMarkets.length} market snapshot
                      {evidenceMarkets.length === 1 ? "" : "s"}
                    </summary>
                    <p>
                      Sanitized account and market facts frozen with this
                      decision. Credentials, provider account IDs, unrelated
                      holdings, and raw provider responses are excluded.
                    </p>
                    <dl className="journal-facts journal-input-summary">
                      <div>
                        <dt>Available cash</dt>
                        <dd>
                          {dollars(
                            contextValue(inputEvidence, "available_cash_usd"),
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>Buying power</dt>
                        <dd>
                          {dollars(
                            contextValue(inputEvidence, "buying_power_usd"),
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>Financial source</dt>
                        <dd>
                          {label(
                            contextValue(inputEvidence, "provider") ??
                              "connected_account",
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>Context assembled</dt>
                        <dd>
                          {timestamp(
                            contextValue(inputEvidence, "observed_at"),
                          )}{" "}
                          UTC
                        </dd>
                      </div>
                    </dl>
                    <div className="journal-context-grid">
                      <section>
                        <h4>Allowlisted holdings</h4>
                        {evidencePositions.length === 0 ? (
                          <p>No allowlisted holdings were present.</p>
                        ) : (
                          evidencePositions.map((position, positionIndex) => (
                            <article
                              key={`${contextValue(position, "symbol") ?? "position"}-${positionIndex}`}
                            >
                              <strong>
                                {contextValue(position, "symbol") ?? "—"}
                              </strong>
                              <span>
                                {decimal(contextValue(position, "quantity"))}{" "}
                                held ·{" "}
                                {decimal(
                                  contextValue(position, "available_quantity"),
                                )}{" "}
                                available
                              </span>
                              <small>
                                {dollars(
                                  contextValue(position, "market_value_usd"),
                                )}{" "}
                                observed value
                              </small>
                            </article>
                          ))
                        )}
                      </section>
                      <section>
                        <h4>Market context</h4>
                        {evidenceMarkets.map((market, marketIndex) => (
                          <article
                            key={`${contextValue(market, "symbol") ?? "market"}-${marketIndex}`}
                          >
                            <strong>
                              {contextValue(market, "symbol") ?? "—"}
                            </strong>
                            <span>{dollars(marketPrice(market))}</span>
                            <small>
                              1h{" "}
                              {signedPercent(
                                contextValue(market, "change_percent_1h"),
                              )}{" "}
                              · 6h{" "}
                              {signedPercent(
                                contextValue(market, "change_percent_6h"),
                              )}{" "}
                              · 24h{" "}
                              {signedPercent(
                                contextValue(market, "change_percent_24h"),
                              )}{" "}
                            </small>
                            <small>
                              {historyCoverage(market)} ·{" "}
                              {label(
                                contextValue(market, "quality") ?? "unknown",
                              )}
                            </small>
                            <time
                              dateTime={
                                contextValue(market, "observed_at") ?? ""
                              }
                            >
                              {label(
                                contextValue(market, "feed") ??
                                  "market_observation",
                              )}{" "}
                              · {timestamp(contextValue(market, "observed_at"))}{" "}
                              UTC
                            </time>
                            {contextValue(market, "history_observed_at") && (
                              <time
                                dateTime={
                                  contextValue(market, "history_observed_at") ??
                                  ""
                                }
                              >
                                {label(
                                  contextValue(market, "history_feed") ??
                                    "market_history",
                                )}{" "}
                                ·{" "}
                                {timestamp(
                                  contextValue(market, "history_observed_at"),
                                )}{" "}
                                UTC
                              </time>
                            )}
                          </article>
                        ))}
                      </section>
                    </div>
                  </details>
                )}

                {shadowAttemptRecorded && (
                  <section
                    className="journal-execution-evidence"
                    aria-label="Hypothetical trade evidence"
                  >
                    <h4>Hypothetical trade evidence</h4>
                    <p>
                      Immutable SHADOW evidence of what Arbion would have
                      attempted after deterministic risk checks.
                    </p>
                    <dl className="journal-facts journal-execution-facts">
                      <div>
                        <dt>Side</dt>
                        <dd>{side ? label(side) : "—"}</dd>
                      </div>
                      <div>
                        <dt>Quantity</dt>
                        <dd>
                          {decimal(quantity)} {executionSymbol ?? ""}
                        </dd>
                      </div>
                      <div>
                        <dt>Reference price</dt>
                        <dd>{dollars(price)}</dd>
                      </div>
                      <div>
                        <dt>Recorded notional</dt>
                        <dd>{dollars(notional)}</dd>
                      </div>
                      <div>
                        <dt>Risk decision</dt>
                        <dd>{riskDecision ? label(riskDecision) : "—"}</dd>
                      </div>
                      <div>
                        <dt>Shadow status</dt>
                        <dd>
                          {executionStatus ? label(executionStatus) : "—"}
                        </dd>
                      </div>
                    </dl>
                    <div className="journal-outcomes">
                      <h5>Outcome marks</h5>
                      {outcomeMarks.length === 0 ? (
                        <p>
                          The 1-hour directional mark is pending. Arbion records
                          it after the horizon receives a fresh market
                          observation.
                        </p>
                      ) : (
                        <div className="journal-outcome-grid">
                          {outcomeMarks.map((outcome, outcomeIndex) => {
                            const horizon = field(
                              outcome,
                              "horizon",
                              "Horizon",
                            );
                            const change = field(
                              outcome,
                              "directional_change_usd",
                              "DirectionalChangeUSD",
                            );
                            const changePercent = field(
                              outcome,
                              "directional_change_percent",
                              "DirectionalChangePercent",
                            );
                            return (
                              <article
                                key={String(
                                  read(outcome, "id", "ID") ?? outcomeIndex,
                                )}
                              >
                                <strong>
                                  {horizon === "TWENTY_FOUR_HOURS"
                                    ? "24-hour mark"
                                    : "1-hour mark"}
                                </strong>
                                <span>
                                  {signedDollars(change)} (
                                  {signedPercent(changePercent)})
                                </span>
                                <small>
                                  {dollars(
                                    field(
                                      outcome,
                                      "observed_price",
                                      "ObservedPrice",
                                    ),
                                  )}{" "}
                                  via{" "}
                                  {label(
                                    field(
                                      outcome,
                                      "pricing_basis",
                                      "PricingBasis",
                                    ) ?? "MARK_FALLBACK",
                                  )}
                                </small>
                                <time
                                  dateTime={
                                    field(
                                      outcome,
                                      "market_observed_at",
                                      "MarketObservedAt",
                                    ) ?? ""
                                  }
                                >
                                  {timestamp(
                                    field(
                                      outcome,
                                      "market_observed_at",
                                      "MarketObservedAt",
                                    ),
                                  )}{" "}
                                  UTC
                                </time>
                                <small>
                                  {elapsedLabel(
                                    read(
                                      outcome,
                                      "elapsed_seconds",
                                      "ElapsedSeconds",
                                    ),
                                  )}
                                </small>
                              </article>
                            );
                          })}
                        </div>
                      )}
                      <p>
                        Directional SHADOW evidence only. It excludes fees and
                        slippage and is not a fill, realized return, or account
                        P&amp;L.
                      </p>
                    </div>
                  </section>
                )}

                {riskFlags.length > 0 && (
                  <div className="journal-reasons">
                    <strong>Risk flags</strong>
                    <ul>
                      {riskFlags.map((flag) => (
                        <li key={flag}>{flag}</li>
                      ))}
                    </ul>
                  </div>
                )}

                {limitations.length > 0 && (
                  <details>
                    <summary>Data limitations</summary>
                    <ul>
                      {limitations.map((limitation) => (
                        <li key={limitation}>{limitation}</li>
                      ))}
                    </ul>
                  </details>
                )}

                <footer>
                  <p>
                    No broker order was sent. Arbion has no live execution path
                    for this engine.
                  </p>
                </footer>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
