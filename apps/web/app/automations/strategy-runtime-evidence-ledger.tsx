"use client";

import { useState } from "react";

type RawRecord = Record<string, unknown>;

type Props = {
  strategyInstanceId: string;
  initialTransitions?: RawRecord[];
  initialExecutions?: RawRecord[];
  initialTransitionCursor?: string;
  initialExecutionCursor?: string;
  transitionHistoryAvailable?: boolean;
  executionHistoryAvailable?: boolean;
  loadedDecisionCount?: number;
};

function read(record: RawRecord, key: string, legacy = key) {
  return record[key] ?? record[legacy];
}

function text(record: RawRecord, key: string, legacy = key) {
  const value = read(record, key, legacy);
  return typeof value === "string" ? value : "";
}

function recordID(record: RawRecord) {
  return text(record, "id", "ID");
}

function appendUnique(current: RawRecord[], incoming: RawRecord[]) {
  const seen = new Set(current.map(recordID).filter(Boolean));
  return [
    ...current,
    ...incoming.filter((record) => {
      const id = recordID(record);
      if (!id || seen.has(id)) return false;
      seen.add(id);
      return true;
    }),
  ];
}

function label(value: string) {
  if (!value) return "Unavailable";
  return value
    .toLowerCase()
    .split("_")
    .map((part) =>
      ["ai", "api", "usd"].includes(part)
        ? part.toUpperCase()
        : part.charAt(0).toUpperCase() + part.slice(1),
    )
    .join(" ");
}

function timestamp(value: unknown) {
  if (typeof value !== "string") return "Time unavailable";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Time unavailable";
  return `${new Intl.DateTimeFormat("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
  }).format(parsed)} UTC`;
}

function decimal(value: string) {
  if (!/^-?\d+(\.\d+)?$/.test(value)) return "Unavailable";
  const [whole, fraction = ""] = value.split(".");
  const trimmed = fraction.replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole;
}

function dollars(value: string) {
  const formatted = decimal(value);
  if (formatted === "Unavailable") return "Price unavailable";
  return formatted.startsWith("-")
    ? `-$${formatted.slice(1)}`
    : `$${formatted}`;
}

function statusClass(value: string) {
  if (value === "WOULD_HAVE_SUBMITTED" || value === "SIMULATED_FILLED") {
    return "is-complete";
  }
  if (value === "RISK_DENIED" || value === "SIMULATED_REJECTED") {
    return "is-held";
  }
  return "is-neutral";
}

export function StrategyRuntimeEvidenceLedger({
  strategyInstanceId,
  initialTransitions = [],
  initialExecutions = [],
  initialTransitionCursor = "",
  initialExecutionCursor = "",
  transitionHistoryAvailable = true,
  executionHistoryAvailable = true,
  loadedDecisionCount = 0,
}: Props) {
  const [transitions, setTransitions] = useState(initialTransitions);
  const [executions, setExecutions] = useState(initialExecutions);
  const [transitionCursor, setTransitionCursor] = useState(
    initialTransitionCursor,
  );
  const [executionCursor, setExecutionCursor] = useState(
    initialExecutionCursor,
  );
  const [transitionBusy, setTransitionBusy] = useState(false);
  const [executionBusy, setExecutionBusy] = useState(false);
  const [transitionMessage, setTransitionMessage] = useState("");
  const [executionMessage, setExecutionMessage] = useState("");

  async function loadEarlierTransitions() {
    if (!transitionCursor || transitionBusy) return;
    setTransitionBusy(true);
    setTransitionMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(strategyInstanceId)}/history?limit=16&cursor=${encodeURIComponent(transitionCursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        transitions?: RawRecord[];
        next_cursor?: string;
        history_semantics?: string;
      } | null;
      if (
        !response.ok ||
        !body ||
        body.history_semantics !== "IMMUTABLE_OWNER_STRATEGY_STATE_HISTORY" ||
        !Array.isArray(body.transitions)
      ) {
        setTransitionMessage(
          "Earlier immutable state evidence could not be loaded.",
        );
        return;
      }
      const earlier = body.transitions;
      setTransitions((current) => appendUnique(current, earlier));
      setTransitionCursor(body.next_cursor ?? "");
    } catch {
      setTransitionMessage(
        "Earlier immutable state evidence could not be loaded.",
      );
    } finally {
      setTransitionBusy(false);
    }
  }

  async function loadEarlierExecutions() {
    if (!executionCursor || executionBusy) return;
    setExecutionBusy(true);
    setExecutionMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(strategyInstanceId)}/executions?limit=16&cursor=${encodeURIComponent(executionCursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        executions?: RawRecord[];
        next_cursor?: string;
        history_semantics?: string;
      } | null;
      if (
        !response.ok ||
        !body ||
        body.history_semantics !==
          "IMMUTABLE_OWNER_NONLIVE_EXECUTION_HISTORY" ||
        !Array.isArray(body.executions)
      ) {
        setExecutionMessage(
          "Earlier immutable execution evidence could not be loaded.",
        );
        return;
      }
      const earlier = body.executions;
      setExecutions((current) => appendUnique(current, earlier));
      setExecutionCursor(body.next_cursor ?? "");
    } catch {
      setExecutionMessage(
        "Earlier immutable execution evidence could not be loaded.",
      );
    } finally {
      setExecutionBusy(false);
    }
  }

  return (
    <section
      className="strategy-runtime-ledger"
      aria-label="Strategy runtime evidence ledger"
    >
      <header>
        <div>
          <p className="eyebrow">RUNTIME EVIDENCE</p>
          <h2>Every non-live state change, bounded and reviewable.</h2>
          <p>
            Inspect immutable state and PAPER or SHADOW execution evidence.
            Loading history never evaluates a model, contacts a provider, or
            requests a broker action.
          </p>
        </div>
        <span>IMMUTABLE · NO BROKER ACTION</span>
      </header>

      <div className="strategy-runtime-ledger-metrics">
        <p>
          <strong>{transitions.length}</strong>
          state changes loaded
        </p>
        <p>
          <strong>{loadedDecisionCount}</strong>
          decisions loaded
        </p>
        <p>
          <strong>{executions.length}</strong>
          non-live results loaded
        </p>
      </div>

      <div className="strategy-runtime-ledger-grid">
        <article>
          <header>
            <div>
              <p className="eyebrow">STATE LEDGER</p>
              <h3>Durable state transitions</h3>
            </div>
            <span>NEWEST FIRST</span>
          </header>
          {!transitionHistoryAvailable ? (
            <p className="strategy-runtime-ledger-unavailable" role="status">
              State history is temporarily unavailable. Arbion will not infer an
              empty ledger.
            </p>
          ) : transitions.length === 0 ? (
            <p className="strategy-runtime-ledger-empty">
              No state transitions are recorded for this instance yet.
            </p>
          ) : (
            <ol className="strategy-runtime-state-list">
              {transitions.map((transition) => {
                const version = Number(
                  read(transition, "state_version", "StateVersion") ?? 0,
                );
                return (
                  <li key={recordID(transition)}>
                    <div>
                      <span>
                        STATE v{Number.isFinite(version) ? version : 0}
                      </span>
                      <time>
                        {timestamp(
                          read(transition, "occurred_at", "OccurredAt"),
                        )}
                      </time>
                    </div>
                    <strong>
                      {label(
                        text(transition, "previous_state", "PreviousState"),
                      )}{" "}
                      {"→"} {label(text(transition, "new_state", "NewState"))}
                    </strong>
                    <small>
                      {label(text(transition, "trigger", "Trigger"))}
                    </small>
                  </li>
                );
              })}
            </ol>
          )}
          {transitionHistoryAvailable && transitionCursor && (
            <button
              type="button"
              onClick={loadEarlierTransitions}
              disabled={transitionBusy}
            >
              {transitionBusy ? "Loading…" : "Load earlier state changes"}
            </button>
          )}
          {transitionMessage && <p role="status">{transitionMessage}</p>}
        </article>

        <article>
          <header>
            <div>
              <p className="eyebrow">EXECUTION EVIDENCE</p>
              <h3>PAPER and SHADOW outcomes</h3>
            </div>
            <span>NON-LIVE ONLY</span>
          </header>
          {!executionHistoryAvailable ? (
            <p className="strategy-runtime-ledger-unavailable" role="status">
              Non-live execution history is temporarily unavailable. Arbion will
              not infer an empty ledger.
            </p>
          ) : executions.length === 0 ? (
            <p className="strategy-runtime-ledger-empty">
              No PAPER or SHADOW execution evidence is recorded yet.
            </p>
          ) : (
            <ol className="strategy-runtime-execution-list">
              {executions.map((execution) => {
                const mode = text(execution, "mode", "Mode");
                const status = text(execution, "status", "Status");
                return (
                  <li key={recordID(execution)}>
                    <header>
                      <span className={statusClass(status)}>
                        {label(status)}
                      </span>
                      <time>
                        {timestamp(read(execution, "created_at", "CreatedAt"))}
                      </time>
                    </header>
                    <strong>
                      {label(text(execution, "side", "Side"))}{" "}
                      {decimal(text(execution, "quantity", "Quantity"))}{" "}
                      {text(execution, "symbol", "Symbol") ||
                        "Symbol unavailable"}
                    </strong>
                    <p>
                      {label(text(execution, "instrument", "Instrument"))} ·{" "}
                      {dollars(text(execution, "price", "Price"))} reference ·{" "}
                      {dollars(text(execution, "notional", "Notional"))}{" "}
                      notional
                    </p>
                    <small>
                      {mode === "PAPER"
                        ? "PAPER simulation only — no broker fill."
                        : "SHADOW hypothetical only — no broker order."}
                    </small>
                  </li>
                );
              })}
            </ol>
          )}
          {executionHistoryAvailable && executionCursor && (
            <button
              type="button"
              onClick={loadEarlierExecutions}
              disabled={executionBusy}
            >
              {executionBusy ? "Loading…" : "Load earlier non-live results"}
            </button>
          )}
          {executionMessage && <p role="status">{executionMessage}</p>}
        </article>
      </div>

      <footer>
        <strong>EXECUTION BOUNDARY: PAPER OR SHADOW ONLY</strong>
        <span>
          These records cannot authorize, submit, replace, cancel, or imply a
          live broker order.
        </span>
      </footer>
    </section>
  );
}
