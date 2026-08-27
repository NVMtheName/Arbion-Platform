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
  changes: PortfolioReconciliationChange[];
  evidence_hash: string;
  observed_at: string;
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

export function PortfolioReconciliationPanel({
  accountID,
  accountName,
  initialReport,
}: {
  accountID: string;
  accountName: string;
  initialReport?: PortfolioReconciliation;
}) {
  const [report, setReport] = useState(initialReport);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function reconcile() {
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/accounts/${encodeURIComponent(accountID)}/reconciliations`,
        { method: "POST" },
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
        <button disabled={busy} onClick={reconcile} type="button">
          {busy ? "Reconciling…" : "Reconcile now"}
        </button>
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
      {message && (
        <p className="reconciliation-message" aria-live="polite">
          {message}
        </p>
      )}
    </section>
  );
}
