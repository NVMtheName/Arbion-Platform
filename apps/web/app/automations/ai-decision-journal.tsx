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

export function AIDecisionJournal({ decisions }: { decisions: RawDecision[] }) {
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
            const executionRecorded = Boolean(
              read(entry, "execution_record_id", "ExecutionRecordID"),
            );
            const riskFlags = strings(facts.risk_flags);
            const limitations = strings(facts.limitations);
            const symbol =
              facts.symbol && facts.symbol !== "NONE"
                ? facts.symbol
                : "No action";
            const entryID = String(read(entry, "id", "ID") ?? index);

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
                      {label(decisionType)} · {symbol}
                    </h3>
                    <p>{facts.thesis ?? "No thesis was recorded."}</p>
                  </div>
                  <div className="journal-badges">
                    <span className="journal-mode shadow">SHADOW</span>
                    <span
                      className={`journal-decision ${riskReached ? "allow" : "abstain"}`}
                    >
                      {riskReached ? "Risk checked" : "Safe abstention"}
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
                    <dd>{executionRecorded ? "Shadow record" : "None"}</dd>
                  </div>
                </dl>

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
