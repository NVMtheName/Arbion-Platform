import Link from "next/link";

export type JournalEntry = {
  id: string;
  created_at: string;
  strategy_instance_id: string;
  financial_account_id: string;
  account_display_name: string;
  mandate_id: string;
  mandate_version: number;
  strategy_identifier: string;
  execution_mode: "PAPER" | "SHADOW";
  strategy_state: string;
  resulting_state?: string;
  source: string;
  decision_type: string;
  structured_rationale: Record<string, unknown>;
  risk_decision?: string;
  approval_required?: boolean;
  risk_reason_codes?: string[];
  execution_status?: string;
  symbol?: string;
  instrument?: string;
  side?: string;
  quantity?: string;
  price?: string;
  notional?: string;
};

function label(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function timestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return value;
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(parsed);
}

function money(value?: string) {
  if (!value) return "—";
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 2,
  }).format(parsed);
}

function rationaleFacts(rationale: Record<string, unknown>) {
  return Object.entries(rationale)
    .filter(([, value]) =>
      ["string", "number", "boolean"].includes(typeof value),
    )
    .slice(0, 6);
}

export function JournalList({ entries }: { entries: JournalEntry[] }) {
  const allowed = entries.filter(
    (entry) => entry.risk_decision === "ALLOW",
  ).length;
  const denied = entries.filter(
    (entry) => entry.risk_decision === "DENY",
  ).length;
  const paper = entries.filter(
    (entry) => entry.execution_mode === "PAPER",
  ).length;
  const shadow = entries.length - paper;

  return (
    <>
      <section className="journal-summary" aria-label="Recent journal summary">
        <article>
          <strong>{entries.length}</strong>
          <span>Recent decisions</span>
        </article>
        <article>
          <strong>{allowed}</strong>
          <span>Risk allowed</span>
        </article>
        <article>
          <strong>{denied}</strong>
          <span>Risk denied</span>
        </article>
        <article>
          <strong>
            {paper} / {shadow}
          </strong>
          <span>Paper / Shadow</span>
        </article>
      </section>

      {entries.length === 0 ? (
        <section className="journal-empty">
          <p className="eyebrow">NO DECISIONS YET</p>
          <h2>Your journal is ready.</h2>
          <p>
            Manually evaluate a READY PAPER or SHADOW automation, or enable its
            guarded non-live schedule, and decision evidence will appear here.
          </p>
          <Link href="/automations">View automations</Link>
        </section>
      ) : (
        <section
          className="journal-timeline"
          aria-label="Decision journal entries"
        >
          {entries.map((entry) => {
            const facts = rationaleFacts(entry.structured_rationale ?? {});
            return (
              <article className="journal-entry" key={entry.id}>
                <header>
                  <div>
                    <time dateTime={entry.created_at}>
                      {timestamp(entry.created_at)} UTC
                    </time>
                    <h2>
                      {label(entry.strategy_identifier)}
                      {entry.symbol ? ` · ${entry.symbol}` : ""}
                    </h2>
                    <p>
                      {entry.account_display_name} · Mandate v
                      {entry.mandate_version}
                    </p>
                  </div>
                  <div className="journal-badges">
                    <span
                      className={`journal-mode ${entry.execution_mode.toLowerCase()}`}
                    >
                      {entry.execution_mode}
                    </span>
                    {entry.risk_decision && (
                      <span
                        className={`journal-decision ${entry.risk_decision.toLowerCase()}`}
                      >
                        Risk {label(entry.risk_decision)}
                      </span>
                    )}
                  </div>
                </header>

                <dl className="journal-facts">
                  <div>
                    <dt>Decision</dt>
                    <dd>{label(entry.decision_type)}</dd>
                  </div>
                  <div>
                    <dt>Execution evidence</dt>
                    <dd>
                      {entry.execution_status
                        ? label(entry.execution_status)
                        : "Not applicable"}
                    </dd>
                  </div>
                  <div>
                    <dt>State</dt>
                    <dd>
                      {label(entry.strategy_state)}
                      {entry.resulting_state &&
                      entry.resulting_state !== entry.strategy_state
                        ? ` → ${label(entry.resulting_state)}`
                        : ""}
                    </dd>
                  </div>
                  <div>
                    <dt>Proposed action</dt>
                    <dd>
                      {[
                        entry.side && label(entry.side),
                        entry.quantity,
                        entry.instrument,
                      ]
                        .filter(Boolean)
                        .join(" · ") || "No action"}
                    </dd>
                  </div>
                  <div>
                    <dt>Price</dt>
                    <dd>{money(entry.price)}</dd>
                  </div>
                  <div>
                    <dt>Notional</dt>
                    <dd>{money(entry.notional)}</dd>
                  </div>
                </dl>

                {(entry.risk_reason_codes?.length ?? 0) > 0 && (
                  <div className="journal-reasons">
                    <strong>Risk reasons</strong>
                    <ul>
                      {entry.risk_reason_codes?.map((reason) => (
                        <li key={reason}>{label(reason)}</li>
                      ))}
                    </ul>
                  </div>
                )}

                {facts.length > 0 && (
                  <details>
                    <summary>Structured rationale</summary>
                    <dl className="journal-rationale">
                      {facts.map(([key, value]) => (
                        <div key={key}>
                          <dt>{label(key)}</dt>
                          <dd>{String(value)}</dd>
                        </div>
                      ))}
                    </dl>
                  </details>
                )}

                <footer>
                  <p>
                    {entry.execution_mode === "PAPER"
                      ? "Simulation evidence only — no real broker order was sent."
                      : "Would-have-submitted evidence only — no real broker order was sent."}
                  </p>
                  <Link href={`/automations/${entry.mandate_id}`}>
                    Review mandate
                  </Link>
                </footer>
              </article>
            );
          })}
        </section>
      )}
    </>
  );
}
