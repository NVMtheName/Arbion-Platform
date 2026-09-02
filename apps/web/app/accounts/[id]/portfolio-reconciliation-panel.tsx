"use client";

import { useState } from "react";

export type PortfolioReconciliationChange = {
  symbol: string;
  instrument_type: string;
  direction: string;
  change_type:
    | "POSITION_APPEARED"
    | "POSITION_DISAPPEARED"
    | "QUANTITY_CHANGED";
  control_impact: "TRADABLE_INVENTORY" | "NON_TRADABLE_QUANTITY_ONLY";
  previous_quantity?: string;
  current_quantity?: string;
  previous_available_quantity?: string;
  current_available_quantity?: string;
  previous_unavailable_quantity?: string;
  current_unavailable_quantity?: string;
};

export type PortfolioReconciliation = {
  id: string;
  financial_account_id: string;
  provider: string;
  comparison_status: "BASELINE" | "MATCHED" | "DRIFT_DETECTED" | "INCOMPLETE";
  balances_status: "READY" | "UNAVAILABLE";
  positions_status: "READY" | "UNAVAILABLE";
  performance_status: "AVAILABLE" | "PARTIAL" | "UNAVAILABLE";
  realized_performance_status: "UNAVAILABLE";
  autonomy_signal: "CLEAR" | "REVIEW_RECOMMENDED" | "INSUFFICIENT_EVIDENCE";
  autonomy_enforcement_active: boolean;
  blocks_new_actions: boolean;
  observed_position_count: number;
  performance_position_count: number;
  change_count: number;
  blocking_change_count: number;
  previous_reconciliation_id?: string;
  changes: PortfolioReconciliationChange[];
  evidence_hash: string;
  observed_at: string;
  created_at?: string;
};

export type PortfolioReconciliationHistory = {
  reconciliations: PortfolioReconciliation[];
  next_cursor?: string;
};

function statusLabel(status: PortfolioReconciliation["comparison_status"]) {
  if (status === "MATCHED") return "Trading inventory matched";
  if (status === "DRIFT_DETECTED") return "Position change detected";
  if (status === "INCOMPLETE") return "Coverage incomplete";
  return "Baseline captured";
}

function signalLabel(signal: PortfolioReconciliation["autonomy_signal"]) {
  if (signal === "CLEAR") return "Evidence clear";
  if (signal === "REVIEW_RECOMMENDED") return "Review recommended";
  return "Collecting evidence";
}

function observedAt(value: string) {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime())
    ? value
    : `${parsed.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

function quantity(value?: string) {
  if (!value) return "none";
  const parsed = Number(value);
  return Number.isFinite(parsed)
    ? new Intl.NumberFormat("en-US", { maximumFractionDigits: 8 }).format(
        parsed,
      )
    : value;
}

function changeDescription(change: PortfolioReconciliationChange) {
  if (change.change_type === "POSITION_APPEARED") {
    return `Appeared at ${quantity(change.current_quantity)}`;
  }
  if (change.change_type === "POSITION_DISAPPEARED") {
    return `No longer reported · previously ${quantity(change.previous_quantity)}`;
  }
  if (change.control_impact === "NON_TRADABLE_QUANTITY_ONLY") {
    return `Unavailable to trade ${quantity(change.previous_unavailable_quantity)} → ${quantity(change.current_unavailable_quantity)} · Available to trade unchanged at ${quantity(change.current_available_quantity)}`;
  }
  return `${quantity(change.previous_quantity)} → ${quantity(change.current_quantity)}`;
}

type ReconciliationResolution = {
  status: "ACTIVE_REVIEW" | "RESOLVED" | "EVIDENCE_REQUIRED" | "CLEAR";
  eyebrow: string;
  label: string;
  detail: string;
  nextStep: string;
  previous?: PortfolioReconciliation;
};

function countLabel(count: number, singular: string) {
  return `${count} ${singular}${count === 1 ? "" : "s"}`;
}

function reconciliationResolution(
  report: PortfolioReconciliation,
  history: PortfolioReconciliation[],
): ReconciliationResolution {
  const previous = report.previous_reconciliation_id
    ? history.find((item) => item.id === report.previous_reconciliation_id)
    : undefined;
  const recordedOnly = Math.max(
    0,
    report.change_count - report.blocking_change_count,
  );

  if (report.comparison_status === "DRIFT_DETECTED") {
    return {
      status: "ACTIVE_REVIEW",
      eyebrow: "ACTIVE HOLD · SAVED EVIDENCE",
      label: "Review active",
      detail: `${countLabel(report.blocking_change_count, "tradable-inventory change")} ${report.blocking_change_count === 1 ? "holds" : "hold"} new AI proposals. ${countLabel(recordedOnly, "additional non-blocking change")} ${recordedOnly === 1 ? "remains" : "remain"} saved as context.`,
      nextStep:
        "Review the exact tradable-inventory change below, acknowledge it, then reconcile. The hold clears only when a later complete snapshot matches the reviewed state.",
      previous,
    };
  }

  if (
    report.comparison_status === "MATCHED" &&
    previous?.comparison_status === "DRIFT_DETECTED"
  ) {
    return {
      status: "RESOLVED",
      eyebrow: "RECORDED RESOLUTION · SAVED EVIDENCE",
      label: "Resolution recorded",
      detail:
        "The current complete snapshot no longer contains a blocking tradable-inventory difference from the reviewed state.",
      nextStep:
        "No owner action is required. New AI proposals remain subject to every other deterministic guardrail.",
      previous,
    };
  }

  if (
    report.comparison_status === "BASELINE" ||
    report.comparison_status === "INCOMPLETE"
  ) {
    return {
      status: "EVIDENCE_REQUIRED",
      eyebrow: "EVIDENCE GATE · SAVED EVIDENCE",
      label:
        report.comparison_status === "BASELINE"
          ? "Comparison pending"
          : "Evidence incomplete",
      detail:
        report.comparison_status === "BASELINE"
          ? "This is the first complete saved snapshot, so Arbion does not yet have a second state to compare."
          : "The latest saved snapshot does not contain complete balance and position evidence.",
      nextStep:
        "Capture another complete provider snapshot. Arbion keeps new AI proposals held until exact comparison evidence is available.",
      previous,
    };
  }

  return {
    status: "CLEAR",
    eyebrow: "CURRENT GATE · SAVED EVIDENCE",
    label: "Gate clear",
    detail:
      "The latest complete snapshot contains no blocking tradable-inventory change.",
    nextStep:
      "No owner action is required. New AI proposals remain subject to every other deterministic guardrail.",
    previous,
  };
}

export function PortfolioReconciliationPanel({
  accountID,
  accountName,
  initialReport,
  initialHistory,
}: {
  accountID: string;
  accountName: string;
  initialReport?: PortfolioReconciliation;
  initialHistory?: PortfolioReconciliationHistory;
}) {
  const [report, setReport] = useState(initialReport);
  const [history, setHistory] = useState<PortfolioReconciliation[]>(
    initialHistory?.reconciliations.length
      ? initialHistory.reconciliations
      : initialReport
        ? [initialReport]
        : [],
  );
  const [historyCursor, setHistoryCursor] = useState(
    initialHistory?.next_cursor ?? "",
  );
  const [historyBusy, setHistoryBusy] = useState(false);
  const [historyMessage, setHistoryMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [driftReviewed, setDriftReviewed] = useState(false);
  const driftReviewRequired = report?.comparison_status === "DRIFT_DETECTED";
  const resolution = report
    ? reconciliationResolution(report, history)
    : undefined;

  async function reconcile() {
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/reconciliations`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            expected_reconciliation_id: report?.id ?? "",
            acknowledge_current_drift: driftReviewRequired && driftReviewed,
          }),
        },
      );
      const body = (await response.json().catch(() => ({}))) as {
        reconciliation?: PortfolioReconciliation;
        error?: { message?: string };
      };
      if (!response.ok || !body.reconciliation) {
        setMessage(
          body.error?.message ??
            "The provider snapshot could not be reconciled safely.",
        );
        return;
      }
      setReport(body.reconciliation);
      setHistory((current) => [
        body.reconciliation!,
        ...current.filter((item) => item.id !== body.reconciliation!.id),
      ]);
      setDriftReviewed(false);
      setMessage(
        body.reconciliation.comparison_status === "DRIFT_DETECTED"
          ? "A broker-reported quantity changed. New AI proposals stay held until a later complete snapshot matches this state."
          : body.reconciliation.comparison_status === "MATCHED"
            ? body.reconciliation.change_count > 0
              ? "An exact unavailable-to-trade movement was recorded. Available inventory is unchanged, so the reconciliation gate is clear for new AI proposals."
              : "Broker state matched the prior complete snapshot. The reconciliation gate is clear for new AI proposals."
            : "A new immutable provider snapshot was recorded. Capture a second matching snapshot to clear new AI proposals.",
      );
    } catch {
      setMessage(
        "The provider snapshot could not be reconciled safely. No account action was taken.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function loadEarlierHistory() {
    if (!historyCursor) return;
    setHistoryBusy(true);
    setHistoryMessage("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/reconciliations?limit=8&cursor=${encodeURIComponent(historyCursor)}`,
        { method: "GET" },
      );
      const body = (await response.json().catch(() => ({}))) as {
        history?: PortfolioReconciliationHistory;
        error?: { message?: string };
      };
      if (!response.ok || !body.history) {
        setHistoryMessage(
          body.error?.message ??
            "Earlier reconciliation evidence could not be loaded.",
        );
        return;
      }
      setHistory((current) => {
        const known = new Set(current.map((item) => item.id));
        return [
          ...current,
          ...body.history!.reconciliations.filter(
            (item) => !known.has(item.id),
          ),
        ];
      });
      setHistoryCursor(body.history.next_cursor ?? "");
    } catch {
      setHistoryMessage(
        "Earlier reconciliation evidence could not be loaded. No provider request was made.",
      );
    } finally {
      setHistoryBusy(false);
    }
  }

  return (
    <section
      className="reconciliation-console"
      aria-labelledby="reconciliation-title"
    >
      <header className="reconciliation-console-header">
        <div>
          <p className="eyebrow">BROKER TRUTH · IMMUTABLE EVIDENCE</p>
          <h2 id="reconciliation-title">Portfolio reconciliation</h2>
          <p>
            Capture {accountName}&apos;s current provider-reported balances and
            compare exact position quantities with its last reliable Arbion
            snapshot.
          </p>
        </div>
        <div className="reconciliation-review-action">
          {driftReviewRequired && (
            <label>
              <input
                checked={driftReviewed}
                disabled={busy}
                onChange={(event) => setDriftReviewed(event.target.checked)}
                type="checkbox"
              />
              <span>
                I reviewed the tradable-inventory changes shown below.
              </span>
            </label>
          )}
          <button
            disabled={busy || (driftReviewRequired && !driftReviewed)}
            onClick={reconcile}
            type="button"
          >
            {busy
              ? "Reconciling…"
              : driftReviewRequired
                ? "Confirm review & reconcile"
                : "Reconcile now"}
          </button>
        </div>
      </header>

      {!report ? (
        <div className="reconciliation-empty">
          <strong>No immutable baseline yet</strong>
          <p>
            Capture the first provider snapshot. A second snapshot can identify
            exact quantity changes without guessing why they happened. New AI
            proposals remain held until two complete snapshots match.
          </p>
        </div>
      ) : (
        <>
          <div className="reconciliation-status-row">
            <span
              className={`reconciliation-status is-${report.comparison_status.toLowerCase()}`}
            >
              {statusLabel(report.comparison_status)}
            </span>
            <span>{signalLabel(report.autonomy_signal)}</span>
            {report.autonomy_enforcement_active && (
              <span>
                {report.blocks_new_actions
                  ? "AI proposals held"
                  : "AI proposal gate clear"}
              </span>
            )}
            <time dateTime={report.observed_at}>
              {observedAt(report.observed_at)}
            </time>
          </div>
          {resolution && (
            <section
              className={`reconciliation-resolution is-${resolution.status.toLowerCase()}`}
              aria-labelledby="reconciliation-resolution-title"
            >
              <header>
                <div>
                  <p className="eyebrow">{resolution.eyebrow}</p>
                  <h3 id="reconciliation-resolution-title">
                    Reconciliation resolution
                  </h3>
                </div>
                <strong>{resolution.label}</strong>
              </header>
              <p>{resolution.detail}</p>
              {resolution.previous && (
                <p>
                  Prior checkpoint:{" "}
                  {statusLabel(resolution.previous.comparison_status)} at{" "}
                  <time dateTime={resolution.previous.observed_at}>
                    {observedAt(resolution.previous.observed_at)}
                  </time>
                  .
                </p>
              )}
              <footer>
                <div>
                  <strong>Next safe step</strong>
                  <span>{resolution.nextStep}</span>
                </div>
                <a href="#reconciliation-history-title">
                  Review immutable snapshot history
                </a>
              </footer>
            </section>
          )}
          <div className="reconciliation-metrics">
            <article>
              <span>Balance feed</span>
              <strong>{report.balances_status}</strong>
            </article>
            <article>
              <span>Position feed</span>
              <strong>{report.positions_status}</strong>
            </article>
            <article>
              <span>Position performance</span>
              <strong>
                {report.performance_position_count}/
                {report.observed_position_count}
              </strong>
              <small>{report.performance_status.toLowerCase()}</small>
            </article>
            <article>
              <span>Realized P&amp;L</span>
              <strong>Unavailable</strong>
              <small>Never inferred from partial history</small>
            </article>
          </div>
          {report.changes.length > 0 && (
            <div className="reconciliation-changes">
              <h3>Changes since the prior snapshot</h3>
              <ul>
                {report.changes.map((change) => (
                  <li
                    key={`${change.symbol}-${change.instrument_type}-${change.direction}`}
                  >
                    <strong>{change.symbol}</strong>
                    <span>
                      {changeDescription(change)}
                      {change.control_impact === "NON_TRADABLE_QUANTITY_ONLY" &&
                        " · Recorded, non-blocking"}
                      {change.control_impact === "TRADABLE_INVENTORY" &&
                        " · Owner review required"}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <footer>
            Evidence {report.evidence_hash.slice(0, 12)}… · This gate can hold
            new AI proposals. It cannot submit an order, change a holding, or
            pause the current shadow engine.
          </footer>
        </>
      )}
      {history.length > 0 && (
        <section
          className="reconciliation-history"
          aria-labelledby="reconciliation-history-title"
        >
          <header>
            <div>
              <p className="eyebrow">ACCOUNT-SCOPED · NEWEST FIRST</p>
              <h3 id="reconciliation-history-title">Reconciliation history</h3>
            </div>
            <span>{history.length} loaded</span>
          </header>
          <ol>
            {history.map((item) => (
              <li key={item.id}>
                <div className="reconciliation-history-row">
                  <div>
                    <span
                      className={`reconciliation-status is-${item.comparison_status.toLowerCase()}`}
                    >
                      {statusLabel(item.comparison_status)}
                    </span>
                    <time dateTime={item.observed_at}>
                      {observedAt(item.observed_at)}
                    </time>
                  </div>
                  <strong>
                    {item.blocking_change_count > 0
                      ? `${item.blocking_change_count} review-required change${item.blocking_change_count === 1 ? "" : "s"}`
                      : item.change_count > 0
                        ? `${item.change_count} recorded non-blocking change${item.change_count === 1 ? "" : "s"}`
                        : "No quantity changes"}
                  </strong>
                </div>
                <p>
                  {item.observed_position_count} positions · Balance feed{" "}
                  {item.balances_status.toLowerCase()} · Position feed{" "}
                  {item.positions_status.toLowerCase()} ·{" "}
                  {item.blocks_new_actions
                    ? "new AI proposals held"
                    : "AI proposal gate clear"}
                </p>
                {item.changes.length > 0 && (
                  <details>
                    <summary>Review exact changes</summary>
                    <ul>
                      {item.changes.map((change) => (
                        <li
                          key={`${item.id}-${change.symbol}-${change.instrument_type}-${change.direction}`}
                        >
                          <strong>{change.symbol}</strong>
                          <span>{changeDescription(change)}</span>
                          <small>
                            {change.control_impact === "TRADABLE_INVENTORY"
                              ? "Owner review required"
                              : "Recorded, non-blocking"}
                          </small>
                        </li>
                      ))}
                    </ul>
                  </details>
                )}
                <footer>
                  Evidence {item.evidence_hash.slice(0, 12)}…
                  {item.previous_reconciliation_id &&
                    ` · Compared with ${item.previous_reconciliation_id.slice(0, 8)}…`}
                </footer>
              </li>
            ))}
          </ol>
          {historyCursor && (
            <button
              disabled={historyBusy}
              onClick={loadEarlierHistory}
              type="button"
            >
              {historyBusy ? "Loading evidence…" : "Load earlier snapshots"}
            </button>
          )}
          {historyMessage && (
            <p className="reconciliation-history-message" aria-live="polite">
              {historyMessage}
            </p>
          )}
          <footer>
            Stored evidence only. Loading history does not contact Coinbase or
            Schwab and cannot acknowledge a change.
          </footer>
        </section>
      )}
      {message && (
        <p className="reconciliation-message" aria-live="polite">
          {message}
        </p>
      )}
    </section>
  );
}
