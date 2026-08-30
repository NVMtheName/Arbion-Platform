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

export type DecisionReviewIndexItem = {
  strategyInstanceID: string;
  mandateID: string;
  accountName: string;
  strategyIdentifier: string;
  executionMode: "PAPER" | "SHADOW";
  latestDecisionID: string;
  latestDecisionType: string;
  latestDecisionAt: string;
  previousDecisionType?: string;
  previousDecisionAt?: string;
  comparison: "CHANGED" | "UNCHANGED" | "FIRST_IN_VIEW";
  provenanceStatus: "VERIFIED" | "UNAVAILABLE";
  aiProvider?: string;
  modelID?: string;
  profile?: string;
  financialProvider?: string;
  marketSymbols: string[];
  marketFeeds: string[];
  marketQualities: string[];
  marketObservedAt?: string;
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

function record(value: unknown) {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function exactText(value: unknown) {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function exactMarketProvenance(
  inputEvidence: Record<string, unknown> | undefined,
) {
  if (!Array.isArray(inputEvidence?.markets))
    return { symbols: [], feeds: [], qualities: [] };
  const symbols: string[] = [];
  const feeds: string[] = [];
  const qualities: string[] = [];
  for (const value of inputEvidence.markets) {
    const market = record(value);
    const symbol = exactText(market?.symbol);
    const feed = exactText(market?.feed);
    const quality = exactText(market?.quality);
    const observedAt = exactText(market?.observed_at);
    if (!symbol || !feed || !quality || !observedAt)
      return { symbols: [], feeds: [], qualities: [] };
    if (!symbols.includes(symbol)) symbols.push(symbol);
    if (!feeds.includes(feed)) feeds.push(feed);
    if (!qualities.includes(quality)) qualities.push(quality);
  }
  return { symbols, feeds, qualities };
}

export function projectDecisionReviewIndex(
  entries: JournalEntry[],
): DecisionReviewIndexItem[] {
  const byInstance = new Map<string, JournalEntry[]>();
  for (const entry of entries) {
    if (entry.source !== "AI") continue;
    const current = byInstance.get(entry.strategy_instance_id) ?? [];
    current.push(entry);
    byInstance.set(entry.strategy_instance_id, current);
  }

  return Array.from(byInstance.values())
    .map((instanceEntries) => {
      const ordered = [...instanceEntries].sort((left, right) => {
        const time =
          new Date(right.created_at).valueOf() -
          new Date(left.created_at).valueOf();
        return time !== 0 ? time : right.id.localeCompare(left.id);
      });
      const latest = ordered[0];
      const previous = ordered[1];
      const rationale = latest.structured_rationale ?? {};
      const inputEvidence = record(rationale.input_evidence);
      const aiProvider = exactText(rationale.ai_provider);
      const modelID = exactText(rationale.model_id);
      const profile = exactText(rationale.profile);
      const financialProvider = exactText(inputEvidence?.provider);
      const marketProvenance = exactMarketProvenance(inputEvidence);
      const marketObservedAt =
        exactText(rationale.market_observed_at) ??
        exactText(inputEvidence?.observed_at);
      const provenanceStatus =
        aiProvider &&
        modelID &&
        profile &&
        financialProvider &&
        marketProvenance.symbols.length > 0 &&
        marketObservedAt
          ? "VERIFIED"
          : "UNAVAILABLE";

      return {
        strategyInstanceID: latest.strategy_instance_id,
        mandateID: latest.mandate_id,
        accountName: latest.account_display_name,
        strategyIdentifier: latest.strategy_identifier,
        executionMode: latest.execution_mode,
        latestDecisionID: latest.id,
        latestDecisionType: latest.decision_type,
        latestDecisionAt: latest.created_at,
        previousDecisionType: previous?.decision_type,
        previousDecisionAt: previous?.created_at,
        comparison: !previous
          ? "FIRST_IN_VIEW"
          : previous.decision_type === latest.decision_type
            ? "UNCHANGED"
            : "CHANGED",
        provenanceStatus,
        aiProvider,
        modelID,
        profile,
        financialProvider,
        marketSymbols: marketProvenance.symbols,
        marketFeeds: marketProvenance.feeds,
        marketQualities: marketProvenance.qualities,
        marketObservedAt,
      } satisfies DecisionReviewIndexItem;
    })
    .sort(
      (left, right) =>
        new Date(right.latestDecisionAt).valueOf() -
        new Date(left.latestDecisionAt).valueOf(),
    );
}

function decisionComparisonLabel(item: DecisionReviewIndexItem) {
  if (item.comparison === "CHANGED") return "Conclusion changed";
  if (item.comparison === "UNCHANGED") return "Conclusion held";
  return "First conclusion in view";
}

function providerLabel(value?: string) {
  if (value === "openai") return "OpenAI";
  if (value === "coinbase") return "Coinbase";
  if (value === "schwab") return "Charles Schwab";
  return value ? label(value) : "Unavailable";
}

function DecisionReviewIndex({ entries }: { entries: JournalEntry[] }) {
  const items = projectDecisionReviewIndex(entries);
  if (items.length === 0) return null;
  return (
    <section
      className="decision-review-index"
      aria-labelledby="decision-review-index-heading"
    >
      <header>
        <div>
          <p className="eyebrow">IMMUTABLE EVIDENCE REVIEW</p>
          <h2 id="decision-review-index-heading">
            Compare each AI engine’s newest conclusion.
          </h2>
          <p>
            This index compares only the saved records on this journal page. It
            never reruns a model, calls a financial provider, or infers missing
            evidence.
          </p>
        </div>
        <span>{items.length} AI engines in view</span>
      </header>
      <ol>
        {items.map((item) => (
          <li
            className={`is-${item.executionMode.toLowerCase()}`}
            key={item.strategyInstanceID}
          >
            <header>
              <div>
                <small>{item.accountName}</small>
                <strong>{label(item.strategyIdentifier)}</strong>
              </div>
              <span
                className={`journal-mode ${item.executionMode.toLowerCase()}`}
              >
                {item.executionMode}
              </span>
            </header>
            <div className="decision-review-index-comparison">
              <section>
                <span>Newest conclusion</span>
                <strong>{label(item.latestDecisionType)}</strong>
                <time dateTime={item.latestDecisionAt}>
                  {timestamp(item.latestDecisionAt)} UTC
                </time>
              </section>
              <span aria-label={decisionComparisonLabel(item)}>
                {item.comparison === "CHANGED" ? "Changed" : "→"}
              </span>
              <section>
                <span>Prior conclusion in this page</span>
                <strong>
                  {item.previousDecisionType
                    ? label(item.previousDecisionType)
                    : "Not in current page"}
                </strong>
                {item.previousDecisionAt && (
                  <time dateTime={item.previousDecisionAt}>
                    {timestamp(item.previousDecisionAt)} UTC
                  </time>
                )}
              </section>
            </div>
            <dl>
              <div>
                <dt>AI route</dt>
                <dd>
                  {item.provenanceStatus === "VERIFIED"
                    ? `${providerLabel(item.aiProvider)} · ${item.modelID} · ${label(item.profile ?? "")}`
                    : "Unavailable — not inferred"}
                </dd>
              </div>
              <div>
                <dt>Financial evidence</dt>
                <dd>
                  {item.provenanceStatus === "VERIFIED"
                    ? `${providerLabel(item.financialProvider)} · ${item.marketSymbols.join(" · ")}`
                    : "Unavailable — not inferred"}
                </dd>
              </div>
              <div>
                <dt>Market snapshot</dt>
                <dd>
                  {item.provenanceStatus === "VERIFIED" && item.marketObservedAt
                    ? `${timestamp(item.marketObservedAt)} UTC · ${item.marketFeeds.map(label).join(" + ")} · ${item.marketQualities.map(label).join(" + ")}`
                    : "Unavailable — not inferred"}
                </dd>
              </div>
            </dl>
            <footer>
              <span>{decisionComparisonLabel(item)}</span>
              <div>
                <Link href={`#decision-${item.latestDecisionID}`}>
                  Open newest record ↓
                </Link>
                <Link href={`/automations/${item.mandateID}`}>
                  Open engine evidence →
                </Link>
              </div>
            </footer>
          </li>
        ))}
      </ol>
      <footer>
        Current page only · owner-scoped immutable records · Paper and Shadow
        remain non-live
      </footer>
    </section>
  );
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

      <DecisionReviewIndex entries={entries} />

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
              <article
                className="journal-entry"
                id={`decision-${entry.id}`}
                key={entry.id}
              >
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
