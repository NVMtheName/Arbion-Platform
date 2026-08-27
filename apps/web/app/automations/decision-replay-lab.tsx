"use client";

import { useState, type CSSProperties } from "react";

type RawRecord = Record<string, unknown>;
type ReplayFilter = "ALL" | "PROPOSALS" | "ABSTENTIONS" | "CONTROL_HOLDS";

type Rationale = {
  ai_provider?: string;
  model_id?: string;
  profile?: string;
  decision?: string;
  symbol?: string;
  side?: string;
  proposed_notional?: string;
  confidence?: string;
  thesis?: string;
  market_observed_at?: string;
  input_evidence?: RawRecord;
};

function read(entry: RawRecord, key: string, legacy: string) {
  return entry[key] ?? entry[legacy];
}

function text(entry: RawRecord, key: string, legacy = key) {
  const value = read(entry, key, legacy);
  return typeof value === "string" ? value : undefined;
}

function rationale(entry: RawRecord): Rationale {
  const value = read(entry, "structured_rationale", "StructuredRationale");
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Rationale)
    : {};
}

function records(value: unknown) {
  return Array.isArray(value)
    ? value.filter(
        (item): item is RawRecord =>
          Boolean(item) && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

function strings(value: unknown) {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function label(value: string | undefined) {
  if (!value) return "Unavailable";
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function providerLabel(value: string | undefined) {
  if (value === "openai") return "OpenAI";
  if (value === "anthropic") return "Claude";
  if (value === "gemini") return "Gemini";
  return value ? label(value) : "Unattributed legacy route";
}

function routeLabel(facts: Rationale) {
  if (!facts.ai_provider || !facts.model_id || !facts.profile) {
    return "Unattributed legacy route";
  }
  return `${providerLabel(facts.ai_provider)} · ${facts.model_id} · ${label(facts.profile)}`;
}

function timestamp(value: unknown, compact = false) {
  if (typeof value !== "string") return "Unavailable";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    ...(compact ? {} : { year: "numeric" }),
  }).format(parsed);
}

function decimal(value: string | undefined) {
  if (!value || !/^-?\d+(\.\d+)?$/.test(value)) return "Unavailable";
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function dollars(value: string | undefined) {
  const formatted = decimal(value);
  if (formatted === "Unavailable") return formatted;
  return formatted.startsWith("-")
    ? `-$${formatted.slice(1)}`
    : `$${formatted}`;
}

function signedPercent(value: string | undefined) {
  const formatted = decimal(value);
  if (formatted === "Unavailable") return formatted;
  if (formatted === "0" || formatted.startsWith("-")) return `${formatted}%`;
  return `+${formatted}%`;
}

function replayKind(entry: RawRecord): Exclude<ReplayFilter, "ALL"> {
  if (text(entry, "risk_decision", "RiskDecision") === "DENY") {
    return "CONTROL_HOLDS";
  }
  return rationale(entry).decision === "PROPOSE" ? "PROPOSALS" : "ABSTENTIONS";
}

function disposition(entry: RawRecord) {
  const kind = replayKind(entry);
  if (kind === "CONTROL_HOLDS") return "CONTROL HOLD";
  if (kind === "PROPOSALS") return "SHADOW PROPOSAL";
  return "SAFE ABSTENTION";
}

function marketPrice(market: RawRecord | undefined) {
  if (!market) return undefined;
  return ["mark", "last", "bid", "ask"]
    .map((key) => text(market, key))
    .find((value) => value && value !== "0");
}

function outcomePercent(outcome: RawRecord) {
  const raw = text(
    outcome,
    "directional_change_percent",
    "DirectionalChangePercent",
  );
  const parsed = raw ? Number(raw) : Number.NaN;
  return Number.isFinite(parsed) ? parsed : undefined;
}

function OutcomeMark({ outcome }: { outcome?: RawRecord }) {
  if (!outcome) {
    return (
      <article className="decision-replay-outcome is-pending">
        <span>PENDING</span>
        <strong>Awaiting fresh market evidence</strong>
        <small>
          No value is estimated before the exact horizon is observed.
        </small>
      </article>
    );
  }
  const horizon = text(outcome, "horizon", "Horizon");
  const percent = outcomePercent(outcome);
  const direction =
    percent === undefined || percent === 0
      ? "flat"
      : percent > 0
        ? "positive"
        : "negative";
  const magnitude =
    percent === undefined
      ? 0
      : Math.max(2, Math.min(Math.abs(percent), 5) * 10);
  const style = { "--replay-magnitude": `${magnitude}%` } as CSSProperties;
  return (
    <article className={`decision-replay-outcome is-${direction}`}>
      <span>{horizon === "ONE_HOUR" ? "1-HOUR MARK" : "24-HOUR MARK"}</span>
      <strong>
        {signedPercent(
          text(
            outcome,
            "directional_change_percent",
            "DirectionalChangePercent",
          ),
        )}
      </strong>
      <div className="decision-replay-meter" aria-hidden="true">
        <i style={style} />
      </div>
      <small>
        {dollars(
          text(outcome, "directional_change_usd", "DirectionalChangeUSD"),
        )}{" "}
        · {label(text(outcome, "pricing_basis", "PricingBasis"))}
      </small>
    </article>
  );
}

export function DecisionReplayLab({
  decisions,
  outcomes = [],
}: {
  decisions: RawRecord[];
  outcomes?: RawRecord[];
}) {
  const entries = decisions
    .filter((entry) => read(entry, "source", "Source") === "AI")
    .slice(0, 24);
  const [filter, setFilter] = useState<ReplayFilter>("ALL");
  const [selectedID, setSelectedID] = useState<string | undefined>(
    entries[0] ? String(read(entries[0], "id", "ID") ?? "") : undefined,
  );
  const visibleEntries = entries.filter(
    (entry) => filter === "ALL" || replayKind(entry) === filter,
  );
  const selected =
    visibleEntries.find(
      (entry) => String(read(entry, "id", "ID") ?? "") === selectedID,
    ) ?? visibleEntries[0];

  const counts = {
    ALL: entries.length,
    PROPOSALS: entries.filter((entry) => replayKind(entry) === "PROPOSALS")
      .length,
    ABSTENTIONS: entries.filter((entry) => replayKind(entry) === "ABSTENTIONS")
      .length,
    CONTROL_HOLDS: entries.filter(
      (entry) => replayKind(entry) === "CONTROL_HOLDS",
    ).length,
  };

  const facts = selected ? rationale(selected) : {};
  const input = facts.input_evidence;
  const positions = records(input?.positions);
  const markets = records(input?.markets);
  const recentDecisions = records(input?.recent_decisions);
  const symbol =
    facts.symbol && facts.symbol !== "NONE" ? facts.symbol : "No action";
  const selectedMarket =
    markets.find((market) => text(market, "symbol") === facts.symbol) ??
    markets[0];
  const executionRecordID = selected
    ? text(selected, "execution_record_id", "ExecutionRecordID")
    : undefined;
  const selectedOutcomes = executionRecordID
    ? outcomes.filter(
        (outcome) =>
          text(outcome, "execution_record_id", "ExecutionRecordID") ===
          executionRecordID,
      )
    : [];
  const oneHour = selectedOutcomes.find(
    (outcome) => text(outcome, "horizon", "Horizon") === "ONE_HOUR",
  );
  const twentyFourHour = selectedOutcomes.find(
    (outcome) => text(outcome, "horizon", "Horizon") === "TWENTY_FOUR_HOURS",
  );
  const riskDecision = selected
    ? text(selected, "risk_decision", "RiskDecision")
    : undefined;
  const executionStatus = selected
    ? text(selected, "execution_status", "ExecutionStatus")
    : undefined;
  const riskReasons = selected
    ? strings(read(selected, "risk_reason_codes", "RiskReasonCodes"))
    : [];

  return (
    <section
      className="decision-replay-lab"
      aria-label="AI Decision Replay Lab"
    >
      <header>
        <div>
          <p className="eyebrow">DECISION REPLAY LAB</p>
          <h2>Reconstruct every autonomous choice.</h2>
          <p>
            Inspect the exact immutable evidence path from model output through
            deterministic controls and hypothetical outcome marks. Replay never
            reruns the model, changes history, or contacts a broker.
          </p>
        </div>
        <span>IMMUTABLE · READ ONLY</span>
      </header>

      {entries.length === 0 ? (
        <div className="decision-replay-empty">
          <strong>No completed AI decisions yet.</strong>
          <p>The lab will populate after the first successful Shadow cycle.</p>
        </div>
      ) : (
        <>
          <div className="decision-replay-filters" aria-label="Replay filters">
            {(
              ["ALL", "PROPOSALS", "ABSTENTIONS", "CONTROL_HOLDS"] as const
            ).map((value) => (
              <button
                aria-pressed={filter === value}
                className={filter === value ? "is-active" : ""}
                key={value}
                onClick={() => setFilter(value)}
                type="button"
              >
                {label(value)} <span>{counts[value]}</span>
              </button>
            ))}
          </div>

          {visibleEntries.length === 0 ? (
            <div className="decision-replay-empty">
              <strong>No decisions match this view.</strong>
              <p>Choose another evidence filter to continue the replay.</p>
            </div>
          ) : (
            <div className="decision-replay-workspace">
              <nav aria-label="Recorded AI decisions">
                {visibleEntries.map((entry) => {
                  const entryFacts = rationale(entry);
                  const entryID = String(read(entry, "id", "ID") ?? "");
                  const active =
                    String(read(selected ?? {}, "id", "ID") ?? "") === entryID;
                  return (
                    <button
                      aria-current={active ? "true" : undefined}
                      className={active ? "is-active" : ""}
                      key={entryID}
                      onClick={() => setSelectedID(entryID)}
                      type="button"
                    >
                      <time>
                        {timestamp(
                          read(entry, "created_at", "CreatedAt"),
                          true,
                        )}
                      </time>
                      <strong>
                        {entryFacts.symbol && entryFacts.symbol !== "NONE"
                          ? entryFacts.symbol
                          : "No action"}
                      </strong>
                      <span>{disposition(entry)}</span>
                    </button>
                  );
                })}
              </nav>

              {selected && (
                <article className="decision-replay-detail">
                  <header>
                    <div>
                      <p className="eyebrow">SELECTED IMMUTABLE DECISION</p>
                      <h3>
                        {label(facts.decision)} · {symbol}
                      </h3>
                      <p>{facts.thesis ?? "No bounded thesis was recorded."}</p>
                    </div>
                    <span
                      className={`is-${replayKind(selected).toLowerCase()}`}
                    >
                      {disposition(selected)}
                    </span>
                  </header>

                  <div className="decision-replay-route">
                    <div>
                      <span>AI ROUTE</span>
                      <strong>{routeLabel(facts)}</strong>
                    </div>
                    <div>
                      <span>RECORDED</span>
                      <strong>
                        {timestamp(read(selected, "created_at", "CreatedAt"))}{" "}
                        UTC
                      </strong>
                    </div>
                    <div>
                      <span>CONFIDENCE</span>
                      <strong>{label(facts.confidence)}</strong>
                    </div>
                    <div>
                      <span>MAX PROPOSED NOTIONAL</span>
                      <strong>{dollars(facts.proposed_notional ?? "0")}</strong>
                    </div>
                  </div>

                  <ol className="decision-replay-pipeline">
                    <li className="is-complete">
                      <span>01</span>
                      <strong>Model decision</strong>
                      <small>{label(facts.decision)}</small>
                    </li>
                    <li className={riskDecision ? "is-complete" : "is-stopped"}>
                      <span>02</span>
                      <strong>Risk controls</strong>
                      <small>
                        {riskDecision
                          ? `${label(riskDecision)}${riskReasons.length ? ` · ${riskReasons.length} reason${riskReasons.length === 1 ? "" : "s"}` : ""}`
                          : "Not reached after abstention"}
                      </small>
                    </li>
                    <li
                      className={
                        executionStatus === "WOULD_HAVE_SUBMITTED"
                          ? "is-complete"
                          : "is-stopped"
                      }
                    >
                      <span>03</span>
                      <strong>Shadow adapter</strong>
                      <small>
                        {executionStatus
                          ? label(executionStatus)
                          : "No hypothetical action"}
                      </small>
                    </li>
                    <li
                      className={
                        selectedOutcomes.length > 0
                          ? "is-complete"
                          : "is-pending"
                      }
                    >
                      <span>04</span>
                      <strong>Outcome evidence</strong>
                      <small>
                        {selectedOutcomes.length > 0
                          ? `${selectedOutcomes.length} immutable mark${selectedOutcomes.length === 1 ? "" : "s"}`
                          : "No eligible mark"}
                      </small>
                    </li>
                  </ol>

                  <div className="decision-replay-context">
                    <section>
                      <p className="eyebrow">INPUT SNAPSHOT</p>
                      <dl>
                        <div>
                          <dt>Financial source</dt>
                          <dd>{label(text(input ?? {}, "provider"))}</dd>
                        </div>
                        <div>
                          <dt>Available cash</dt>
                          <dd>
                            {dollars(text(input ?? {}, "available_cash_usd"))}
                          </dd>
                        </div>
                        <div>
                          <dt>Buying power</dt>
                          <dd>
                            {dollars(text(input ?? {}, "buying_power_usd"))}
                          </dd>
                        </div>
                        <div>
                          <dt>Coverage</dt>
                          <dd>
                            {positions.length} holding
                            {positions.length === 1 ? "" : "s"} ·{" "}
                            {markets.length} market
                            {markets.length === 1 ? "" : "s"}
                          </dd>
                        </div>
                        <div>
                          <dt>Recent memory</dt>
                          <dd>
                            {recentDecisions.length} bounded decision
                            {recentDecisions.length === 1 ? "" : "s"}
                          </dd>
                        </div>
                        <div>
                          <dt>Context assembled</dt>
                          <dd>
                            {timestamp(text(input ?? {}, "observed_at"))} UTC
                          </dd>
                        </div>
                      </dl>
                    </section>

                    <section>
                      <p className="eyebrow">MARKET SNAPSHOT</p>
                      {selectedMarket ? (
                        <>
                          <div className="decision-replay-market-price">
                            <strong>
                              {text(selectedMarket, "symbol") ?? "—"}
                            </strong>
                            <span>{dollars(marketPrice(selectedMarket))}</span>
                          </div>
                          <dl>
                            <div>
                              <dt>1-hour change</dt>
                              <dd>
                                {signedPercent(
                                  text(selectedMarket, "change_percent_1h"),
                                )}
                              </dd>
                            </div>
                            <div>
                              <dt>6-hour change</dt>
                              <dd>
                                {signedPercent(
                                  text(selectedMarket, "change_percent_6h"),
                                )}
                              </dd>
                            </div>
                            <div>
                              <dt>24-hour change</dt>
                              <dd>
                                {signedPercent(
                                  text(selectedMarket, "change_percent_24h"),
                                )}
                              </dd>
                            </div>
                            <div>
                              <dt>Provenance</dt>
                              <dd>
                                {label(text(selectedMarket, "feed"))} ·{" "}
                                {label(text(selectedMarket, "quality"))}
                              </dd>
                            </div>
                          </dl>
                        </>
                      ) : (
                        <p className="decision-replay-unavailable">
                          Market evidence was unavailable for this decision and
                          is not inferred during replay.
                        </p>
                      )}
                    </section>
                  </div>

                  {executionStatus === "WOULD_HAVE_SUBMITTED" && (
                    <section className="decision-replay-outcomes">
                      <header>
                        <div>
                          <p className="eyebrow">HYPOTHETICAL MARKS</p>
                          <h4>
                            Directional observation—not account performance
                          </h4>
                        </div>
                        <span>
                          {text(selected, "side", "Side")
                            ? label(text(selected, "side", "Side"))
                            : "—"}{" "}
                          {decimal(text(selected, "quantity", "Quantity"))}{" "}
                          {text(selected, "symbol", "Symbol") ?? ""}
                        </span>
                      </header>
                      <div>
                        <OutcomeMark outcome={oneHour} />
                        <OutcomeMark outcome={twentyFourHour} />
                      </div>
                    </section>
                  )}

                  {riskReasons.length > 0 && (
                    <section className="decision-replay-control-hold">
                      <strong>Deterministic control evidence</strong>
                      <ul>
                        {riskReasons.map((reason) => (
                          <li key={reason}>{label(reason)}</li>
                        ))}
                      </ul>
                    </section>
                  )}
                </article>
              )}
            </div>
          )}
        </>
      )}

      <footer>
        <strong>NO BROKER ORDER WAS SENT</strong>
        <span>
          This lab reads historical Shadow evidence only. It has no submission,
          replacement, cancellation, approval, or live-execution capability.
        </span>
      </footer>
    </section>
  );
}
