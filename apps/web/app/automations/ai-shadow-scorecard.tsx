type RawScore = Record<string, unknown>;

function read(record: RawScore, key: string, legacy: string) {
  return record[key] ?? record[legacy];
}

function number(record: RawScore, key: string, legacy: string) {
  const value = Number(read(record, key, legacy) ?? 0);
  return Number.isFinite(value) ? value : 0;
}

function decimal(value: unknown) {
  if (typeof value !== "string" || !/^-?\d+(\.\d+)?$/.test(value)) return "—";
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function signedPercent(value: unknown) {
  const formatted = decimal(value);
  if (formatted === "—") return formatted;
  if (formatted.startsWith("-") || formatted === "0") return `${formatted}%`;
  return `+${formatted}%`;
}

function signedUSD(value: unknown) {
  const formatted = decimal(value);
  if (formatted === "—") return formatted;
  if (formatted.startsWith("-")) return `-$${formatted.slice(1)}`;
  if (formatted === "0") return "$0";
  return `+$${formatted}`;
}

function percent(value: unknown) {
  const formatted = decimal(value);
  return formatted === "—" ? formatted : `${formatted}%`;
}

function horizonLabel(value: unknown) {
  return value === "TWENTY_FOUR_HOURS" ? "24-hour horizon" : "1-hour horizon";
}

function object(record: RawScore | undefined, key: string, legacy: string) {
  const value = record ? read(record, key, legacy) : undefined;
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as RawScore)
    : undefined;
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function blockerLabel(value: string) {
  const labels: Record<string, string> = {
    ONE_HOUR_SAMPLE_INCOMPLETE: "Collect more 1-hour outcome marks",
    TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE: "Collect more 24-hour outcome marks",
    EVIDENCE_WINDOW_INCOMPLETE: "Observe the mandate across a longer window",
    SCHEDULE_NOT_VERIFIED: "Complete a healthy scheduled cycle",
    SCHEDULE_UNHEALTHY: "Resolve the current scheduler failure",
  };
  return labels[value] ?? "Resolve an unrecognized evidence blocker";
}

function providerLabel(value: unknown) {
  const labels: Record<string, string> = {
    openai: "OpenAI",
    anthropic: "Claude",
    gemini: "Gemini",
  };
  const provider = String(value ?? "").toLowerCase();
  return labels[provider] ?? String(value ?? "AI");
}

export function AIShadowScorecard({ scorecard }: { scorecard?: RawScore }) {
  const horizons = Array.isArray(scorecard?.horizons)
    ? (scorecard.horizons as RawScore[])
    : Array.isArray(scorecard?.Horizons)
      ? (scorecard.Horizons as RawScore[])
      : [];
  const totalMarks = scorecard
    ? number(scorecard, "total_marks", "TotalMarks")
    : 0;
  const evidenceGate = object(scorecard, "evidence_gate", "EvidenceGate");
  const behavior = object(scorecard, "behavior", "Behavior");
  const routes = Array.isArray(behavior?.routes)
    ? (behavior.routes as RawScore[])
    : Array.isArray(behavior?.Routes)
      ? (behavior.Routes as RawScore[])
      : [];
  const symbols = Array.isArray(behavior?.symbols)
    ? (behavior.symbols as RawScore[])
    : Array.isArray(behavior?.Symbols)
      ? (behavior.Symbols as RawScore[])
      : [];
  const evidenceStatus = String(
    evidenceGate
      ? (read(evidenceGate, "status", "Status") ?? "COLLECTING_EVIDENCE")
      : "COLLECTING_EVIDENCE",
  );
  const minimumSample = evidenceGate
    ? number(
        evidenceGate,
        "minimum_sample_per_horizon",
        "MinimumSamplePerHorizon",
      ) || 20
    : 20;
  const minimumWindow = evidenceGate
    ? number(
        evidenceGate,
        "minimum_evidence_window_hours",
        "MinimumEvidenceWindowHours",
      ) || 168
    : 168;
  const gateBlockers = strings(
    evidenceGate ? read(evidenceGate, "blockers", "Blockers") : undefined,
  );

  return (
    <section className="shadow-scorecard" aria-label="AI shadow scorecard">
      <p className="eyebrow">SHADOW EVIDENCE SCORECARD</p>
      <h2>How hypothetical decisions moved</h2>
      <p>
        Favorable means the market later moved in the proposal&apos;s direction.
        Horizons stay separate, and every figure comes from immutable SHADOW
        marks.
      </p>
      <p className="shadow-scorecard-total">
        <strong>{totalMarks}</strong> total horizon mark
        {totalMarks === 1 ? "" : "s"}
      </p>

      {behavior && (
        <section
          className="shadow-behavior-scorecard"
          aria-label="AI behavior and reliability"
        >
          <header>
            <div>
              <p className="eyebrow">AI BEHAVIOR &amp; RELIABILITY</p>
              <h3>How the engine is deciding</h3>
            </div>
            <span>SHADOW ONLY</span>
          </header>
          <p>
            Completed evaluations are counted from the immutable journal for
            this automation&apos;s connected account. Missing history is never
            estimated.
          </p>
          <dl className="shadow-behavior-summary">
            <div>
              <dt>Completed evaluations</dt>
              <dd>
                {number(behavior, "total_ai_decisions", "TotalAIDecisions")}
              </dd>
            </div>
            <div>
              <dt>Abstained</dt>
              <dd>
                {number(behavior, "abstentions", "Abstentions")}
                <small>
                  {percent(
                    read(
                      behavior,
                      "abstention_rate_percent",
                      "AbstentionRatePercent",
                    ),
                  )}
                </small>
              </dd>
            </div>
            <div>
              <dt>Proposed</dt>
              <dd>
                {number(behavior, "proposed_decisions", "ProposedDecisions")}
                <small>
                  {percent(
                    read(
                      behavior,
                      "proposal_rate_percent",
                      "ProposalRatePercent",
                    ),
                  )}
                </small>
              </dd>
            </div>
            <div>
              <dt>Held by risk controls</dt>
              <dd>
                {number(behavior, "risk_held_decisions", "RiskHeldDecisions")}
                <small>
                  {number(
                    behavior,
                    "repeat_action_cooldown_holds",
                    "RepeatActionCooldownHolds",
                  )}{" "}
                  repeat-action hold
                  {number(
                    behavior,
                    "repeat_action_cooldown_holds",
                    "RepeatActionCooldownHolds",
                  ) === 1
                    ? ""
                    : "s"}
                </small>
              </dd>
            </div>
            <div>
              <dt>Would have submitted</dt>
              <dd>
                {number(
                  behavior,
                  "would_have_submitted_decisions",
                  "WouldHaveSubmittedDecisions",
                )}
              </dd>
            </div>
          </dl>

          <div className="shadow-reliability-strip">
            <p>
              <strong>Scheduler</strong>
              {evidenceGate &&
              read(evidenceGate, "schedule_healthy", "ScheduleHealthy")
                ? "Healthy"
                : "Not verified"}
            </p>
            <p>
              <strong>Consecutive failures</strong>
              {evidenceGate
                ? number(
                    evidenceGate,
                    "consecutive_schedule_failures",
                    "ConsecutiveScheduleFailures",
                  )
                : 0}
            </p>
            <p>
              <strong>Average cycle interval</strong>
              {decimal(
                read(
                  behavior,
                  "average_decision_interval_minutes",
                  "AverageDecisionIntervalMins",
                ),
              )}{" "}
              min
            </p>
            <p>
              <strong>Explicit route records</strong>
              {number(
                behavior,
                "attributed_decisions",
                "AttributedDecisions",
              )}{" "}
              / {number(behavior, "total_ai_decisions", "TotalAIDecisions")}
            </p>
          </div>

          {routes.length === 0 ? (
            <p className="security-note">
              Route behavior will appear after the first completed AI cycle.
            </p>
          ) : (
            <div className="shadow-route-grid">
              {routes.map((route, index) => {
                const legacy =
                  read(route, "provenance_status", "ProvenanceStatus") ===
                  "UNATTRIBUTED_LEGACY";
                const measuredLatency = number(
                  route,
                  "measured_latency_decisions",
                  "MeasuredLatencyDecisions",
                );
                const meteredUsage = number(
                  route,
                  "metered_usage_decisions",
                  "MeteredUsageDecisions",
                );
                return (
                  <article
                    key={`${String(read(route, "model_id", "ModelID"))}-${index}`}
                  >
                    <header>
                      <div>
                        <h4>
                          {legacy
                            ? "Earlier route"
                            : `${providerLabel(read(route, "ai_provider", "AIProvider"))} · ${String(read(route, "model_id", "ModelID"))}`}
                        </h4>
                        <p>
                          {legacy
                            ? "Provenance not recorded"
                            : `${String(read(route, "profile", "Profile"))} profile`}
                        </p>
                      </div>
                      <span>
                        {number(route, "total_decisions", "TotalDecisions")}{" "}
                        cycle
                        {number(route, "total_decisions", "TotalDecisions") ===
                        1
                          ? ""
                          : "s"}
                      </span>
                    </header>
                    <dl>
                      <div>
                        <dt>Abstained</dt>
                        <dd>{number(route, "abstentions", "Abstentions")}</dd>
                      </div>
                      <div>
                        <dt>Risk held</dt>
                        <dd>
                          {number(
                            route,
                            "risk_held_decisions",
                            "RiskHeldDecisions",
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>Repeat cooldown</dt>
                        <dd>
                          {number(
                            route,
                            "repeat_action_cooldown_holds",
                            "RepeatActionCooldownHolds",
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>Shadow cleared</dt>
                        <dd>
                          {number(
                            route,
                            "would_have_submitted_decisions",
                            "WouldHaveSubmittedDecisions",
                          )}
                        </dd>
                      </div>
                      <div>
                        <dt>1h / 24h marks</dt>
                        <dd>
                          {number(
                            route,
                            "one_hour_outcome_marks",
                            "OneHourOutcomeMarks",
                          )}{" "}
                          /{" "}
                          {number(
                            route,
                            "twenty_four_hour_outcome_marks",
                            "TwentyFourHourOutcomeMarks",
                          )}
                        </dd>
                      </div>
                    </dl>
                    {legacy ? (
                      <p className="security-note">
                        Arbion does not guess which provider or model produced
                        older entries.
                      </p>
                    ) : measuredLatency > 0 || meteredUsage > 0 ? (
                      <p className="shadow-route-telemetry">
                        {measuredLatency > 0 && (
                          <span>
                            Avg response{" "}
                            {decimal(
                              read(
                                route,
                                "average_latency_milliseconds",
                                "AverageLatencyMilliseconds",
                              ),
                            )}{" "}
                            ms across {measuredLatency} cycle
                            {measuredLatency === 1 ? "" : "s"}
                          </span>
                        )}
                        {meteredUsage > 0 && (
                          <span>
                            Recorded tokens{" "}
                            {number(
                              route,
                              "recorded_input_tokens",
                              "RecordedInputTokens",
                            )}{" "}
                            in /{" "}
                            {number(
                              route,
                              "recorded_output_tokens",
                              "RecordedOutputTokens",
                            )}{" "}
                            out
                          </span>
                        )}
                      </p>
                    ) : (
                      <p className="security-note">
                        Response-time and token telemetry starts with newly
                        recorded cycles.
                      </p>
                    )}
                  </article>
                );
              })}
            </div>
          )}

          {symbols.length > 0 && (
            <div className="shadow-symbol-behavior">
              <h4>Proposal behavior by asset</h4>
              <div role="table" aria-label="Proposal behavior by asset">
                <div role="row">
                  <span role="columnheader">Asset</span>
                  <span role="columnheader">Proposed</span>
                  <span role="columnheader">Risk held</span>
                  <span role="columnheader">Shadow cleared</span>
                  <span role="columnheader">1h / 24h marks</span>
                </div>
                {symbols.map((symbol) => (
                  <div
                    role="row"
                    key={String(read(symbol, "symbol", "Symbol"))}
                  >
                    <strong role="cell">
                      {String(read(symbol, "symbol", "Symbol"))}
                    </strong>
                    <span role="cell">
                      {number(
                        symbol,
                        "proposed_decisions",
                        "ProposedDecisions",
                      )}
                    </span>
                    <span role="cell">
                      {number(
                        symbol,
                        "risk_held_decisions",
                        "RiskHeldDecisions",
                      )}
                    </span>
                    <span role="cell">
                      {number(
                        symbol,
                        "would_have_submitted_decisions",
                        "WouldHaveSubmittedDecisions",
                      )}
                    </span>
                    <span role="cell">
                      {number(
                        symbol,
                        "one_hour_outcome_marks",
                        "OneHourOutcomeMarks",
                      )}{" "}
                      /{" "}
                      {number(
                        symbol,
                        "twenty_four_hour_outcome_marks",
                        "TwentyFourHourOutcomeMarks",
                      )}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
          <p className="security-note">
            These are behavior and operations counts—not accuracy, profit, or
            permission to place an order. Broker execution remains unavailable.
          </p>
        </section>
      )}

      {evidenceGate && (
        <section
          className="shadow-evidence-gate"
          aria-label="Autonomy evidence gate"
        >
          <header>
            <div>
              <p className="eyebrow">AUTONOMY EVIDENCE GATE</p>
              <h3>Is the shadow record mature enough to review?</h3>
            </div>
            <span
              className={
                evidenceStatus === "EVIDENCE_REVIEWABLE" ? "reviewable" : ""
              }
            >
              {evidenceStatus === "EVIDENCE_REVIEWABLE"
                ? "Reviewable evidence"
                : "Collecting evidence"}
            </span>
          </header>
          <dl>
            <div>
              <dt>1-hour marks</dt>
              <dd>
                {number(
                  evidenceGate,
                  "one_hour_sample_size",
                  "OneHourSampleSize",
                )}{" "}
                / {minimumSample}
              </dd>
            </div>
            <div>
              <dt>24-hour marks</dt>
              <dd>
                {number(
                  evidenceGate,
                  "twenty_four_hour_sample_size",
                  "TwentyFourHourSampleSize",
                )}{" "}
                / {minimumSample}
              </dd>
            </div>
            <div>
              <dt>Evidence window</dt>
              <dd>
                {number(
                  evidenceGate,
                  "evidence_window_hours",
                  "EvidenceWindowHours",
                )}{" "}
                / {minimumWindow} hours
              </dd>
            </div>
            <div>
              <dt>Scheduler</dt>
              <dd>
                {read(evidenceGate, "schedule_healthy", "ScheduleHealthy")
                  ? "Healthy"
                  : "Not verified"}
              </dd>
            </div>
          </dl>
          {gateBlockers.length > 0 && (
            <div className="shadow-evidence-blockers">
              <strong>Still needed</strong>
              <ul>
                {gateBlockers.map((blocker) => (
                  <li key={blocker}>{blockerLabel(blocker)}</li>
                ))}
              </ul>
            </div>
          )}
          <p className="security-note">
            Passing this gate makes evidence reviewable. It does not authorize
            live trading, claim profitability, or add a broker-write path.
          </p>
        </section>
      )}

      {horizons.length === 0 ? (
        <p className="security-note">
          No outcome scorecard is available yet. Arbion never estimates missing
          marks.
        </p>
      ) : (
        <div className="shadow-scorecard-grid">
          {horizons.map((horizon, index) => {
            const sampleSize = number(horizon, "sample_size", "SampleSize");
            const minimum = number(
              horizon,
              "minimum_sample_for_observational_label",
              "MinimumSampleForObservationalLabel",
            );
            const interpretation = String(
              read(horizon, "interpretation", "Interpretation") ??
                "INSUFFICIENT_SAMPLE",
            );
            const favorableRate = read(
              horizon,
              "favorable_rate_percent",
              "FavorableRatePercent",
            );
            const averageChange = read(
              horizon,
              "average_directional_change_percent",
              "AverageDirectionalChangePercent",
            );
            const medianChange = read(
              horizon,
              "median_directional_change_percent",
              "MedianDirectionalChangePercent",
            );
            const bestChange = read(
              horizon,
              "best_directional_change_percent",
              "BestDirectionalChangePercent",
            );
            const worstChange = read(
              horizon,
              "worst_directional_change_percent",
              "WorstDirectionalChangePercent",
            );
            const averageUSD = read(
              horizon,
              "average_directional_change_usd",
              "AverageDirectionalChangeUSD",
            );
            const cumulativeUSD = read(
              horizon,
              "cumulative_directional_change_usd",
              "CumulativeDirectionalChangeUSD",
            );
            const horizonValue = read(horizon, "horizon", "Horizon");
            return (
              <article key={String(horizonValue ?? index)}>
                <header>
                  <h3>{horizonLabel(horizonValue)}</h3>
                  <span>
                    {interpretation === "OBSERVATIONAL"
                      ? "Observational only"
                      : "Early evidence"}
                  </span>
                </header>
                <p className="shadow-scorecard-movement">
                  <strong>{signedPercent(averageChange)}</strong>
                  <small>average directional movement</small>
                </p>
                {sampleSize === 0 ? (
                  <p>Awaiting the first fresh mark at this horizon.</p>
                ) : (
                  <dl>
                    <div>
                      <dt>Samples</dt>
                      <dd>{sampleSize}</dd>
                    </div>
                    <div>
                      <dt>Favorable</dt>
                      <dd>
                        {number(horizon, "favorable_marks", "FavorableMarks")}
                      </dd>
                    </div>
                    <div>
                      <dt>Unfavorable</dt>
                      <dd>
                        {number(
                          horizon,
                          "unfavorable_marks",
                          "UnfavorableMarks",
                        )}
                      </dd>
                    </div>
                    <div>
                      <dt>Flat</dt>
                      <dd>{number(horizon, "flat_marks", "FlatMarks")}</dd>
                    </div>
                    <div>
                      <dt>Observed favorable rate</dt>
                      <dd>{signedPercent(favorableRate).replace("+", "")}</dd>
                    </div>
                    <div>
                      <dt>Median mark</dt>
                      <dd>{signedPercent(medianChange)}</dd>
                    </div>
                    <div>
                      <dt>Best observed mark</dt>
                      <dd>{signedPercent(bestChange)}</dd>
                    </div>
                    <div>
                      <dt>Worst observed mark</dt>
                      <dd>{signedPercent(worstChange)}</dd>
                    </div>
                    <div>
                      <dt>Average marked value</dt>
                      <dd>{signedUSD(averageUSD)}</dd>
                    </div>
                    <div>
                      <dt>Sum of independent marks</dt>
                      <dd>{signedUSD(cumulativeUSD)}</dd>
                    </div>
                  </dl>
                )}
                {sampleSize > 0 && (
                  <p>
                    Dollar marks scale each stored price move by its
                    hypothetical quantity. Their sum treats every mark
                    independently and does not reconstruct a portfolio.
                  </p>
                )}
                <p>
                  {sampleSize} of {minimum || 20} marks before Arbion changes
                  the label from early evidence to observational.
                </p>
              </article>
            );
          })}
        </div>
      )}

      <p className="security-note">
        This is hypothetical directional evidence—not prediction accuracy, a
        fill, realized return, account P&amp;L, or permission to trade.
      </p>
    </section>
  );
}
