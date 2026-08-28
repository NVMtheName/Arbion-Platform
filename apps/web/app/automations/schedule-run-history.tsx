"use client";

import { useState } from "react";

export type ScheduleRunRecord = {
  id: string;
  mandate_version: number;
  execution_mode: "PAPER" | "SHADOW";
  strategy_state: string;
  scheduled_for: string;
  started_at: string;
  completed_at: string;
  next_run_at: string;
  status: "SUCCEEDED" | "FAILED" | "SKIPPED";
  error_code?: string;
  ai_decision?: "ABSTAIN" | "PROPOSE";
  execution_status?: string;
  duplicate_recovered: boolean;
  reconciliation_id?: string;
  reconciliation_review_required: boolean;
  consecutive_failures: number;
};

function label(value?: string) {
  if (!value) return "Not recorded";
  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function timestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
    timeZone: "UTC",
  }).format(parsed);
}

function elapsed(start: string, end: string) {
  const startTime = new Date(start).valueOf();
  const endTime = new Date(end).valueOf();
  if (!Number.isFinite(startTime) || !Number.isFinite(endTime)) return "—";
  const milliseconds = Math.max(0, endTime - startTime);
  if (milliseconds < 1000) return "<1 sec";
  if (milliseconds < 60_000) return `${Math.round(milliseconds / 1000)} sec`;
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1000);
  return `${minutes}m${seconds ? ` ${seconds}s` : ""}`;
}

function startLag(run: ScheduleRunRecord) {
  const scheduled = new Date(run.scheduled_for).valueOf();
  const started = new Date(run.started_at).valueOf();
  if (!Number.isFinite(scheduled) || !Number.isFinite(started)) return "—";
  const seconds = Math.max(0, Math.round((started - scheduled) / 1000));
  return seconds < 5 ? "On time" : `${seconds} sec after due time`;
}

function statusLabel(status: ScheduleRunRecord["status"]) {
  if (status === "SUCCEEDED") return "Completed";
  if (status === "SKIPPED") return "Skipped safely";
  return "Failed closed";
}

function outcomeLabel(run: ScheduleRunRecord) {
  if (run.duplicate_recovered) return "Recovered prior completed evaluation";
  if (run.ai_decision === "ABSTAIN") return "AI abstained safely";
  switch (run.execution_status) {
    case "RISK_DENIED":
      return "Proposal held by deterministic controls";
    case "WOULD_HAVE_SUBMITTED":
      return "Shadow proposal recorded";
    case "SIMULATED_FILLED":
      return "Paper simulation recorded";
    case "SIMULATED_REJECTED":
      return "Paper simulation rejected safely";
    case "CANCELED":
      return "No non-live action recorded";
    default:
      return run.status === "SUCCEEDED"
        ? "Evaluation completed"
        : label(run.error_code);
  }
}

export function ScheduleRunHistory({
  instanceId,
  initialRuns,
  initialCursor = "",
}: {
  instanceId: string;
  initialRuns: ScheduleRunRecord[];
  initialCursor?: string;
}) {
  const [runs, setRuns] = useState(initialRuns);
  const [cursor, setCursor] = useState(initialCursor);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function loadEarlier() {
    if (!cursor || busy) return;
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(instanceId)}/schedule-runs?limit=12&cursor=${encodeURIComponent(cursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        runs?: ScheduleRunRecord[];
        next_cursor?: string;
      } | null;
      if (!response.ok || !body || !Array.isArray(body.runs)) {
        setMessage("Earlier scheduler evidence could not be loaded.");
        return;
      }
      setRuns((current) => [
        ...current,
        ...body.runs!.filter(
          (candidate) => !current.some((run) => run.id === candidate.id),
        ),
      ]);
      setCursor(body.next_cursor ?? "");
    } catch {
      setMessage("Earlier scheduler evidence could not be loaded.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="schedule-run-history" aria-label="Scheduler run ledger">
      <header>
        <div>
          <p className="eyebrow">IMMUTABLE SCHEDULER LEDGER</p>
          <h2>Every scheduled cycle, preserved.</h2>
          <p>
            Successes, safe skips, and fail-closed results remain visible after
            later cycles complete. Records begin with this production release.
          </p>
        </div>
        <span>NON-LIVE ONLY</span>
      </header>

      {runs.length === 0 ? (
        <div className="schedule-run-empty">
          <strong>No completed scheduler runs recorded yet.</strong>
          <p>The next eligible cycle will create the first immutable record.</p>
        </div>
      ) : (
        <ol className="schedule-run-list">
          {runs.map((run) => (
            <li className={`is-${run.status.toLowerCase()}`} key={run.id}>
              <header>
                <div>
                  <time dateTime={run.scheduled_for}>
                    Due {timestamp(run.scheduled_for)} UTC
                  </time>
                  <h3>{outcomeLabel(run)}</h3>
                </div>
                <div className="schedule-run-badges">
                  <span>{run.execution_mode}</span>
                  <strong>{statusLabel(run.status)}</strong>
                </div>
              </header>
              <dl>
                <div>
                  <dt>Mandate</dt>
                  <dd>Version {run.mandate_version}</dd>
                </div>
                <div>
                  <dt>Starting state</dt>
                  <dd>{label(run.strategy_state)}</dd>
                </div>
                <div>
                  <dt>Started</dt>
                  <dd>{startLag(run)}</dd>
                </div>
                <div>
                  <dt>Duration</dt>
                  <dd>{elapsed(run.started_at, run.completed_at)}</dd>
                </div>
                <div>
                  <dt>Next due</dt>
                  <dd>{timestamp(run.next_run_at)} UTC</dd>
                </div>
                <div>
                  <dt>Failure streak</dt>
                  <dd>{run.consecutive_failures}</dd>
                </div>
              </dl>
              {run.error_code && (
                <p className="schedule-run-code">
                  Safe result code: <strong>{run.error_code}</strong>
                </p>
              )}
              {run.reconciliation_id && (
                <p
                  className={
                    run.reconciliation_review_required
                      ? "schedule-run-reconciliation needs-review"
                      : "schedule-run-reconciliation"
                  }
                >
                  {run.reconciliation_review_required
                    ? "Portfolio inventory review required"
                    : "Portfolio reconciliation recorded"}
                  <small>Evidence {run.reconciliation_id.slice(0, 8)}…</small>
                </p>
              )}
              <footer>No broker order was sent.</footer>
            </li>
          ))}
        </ol>
      )}

      {cursor && (
        <button disabled={busy} onClick={loadEarlier} type="button">
          {busy ? "Loading…" : "Load earlier runs"}
        </button>
      )}
      {message && <p role="status">{message}</p>}
      <footer>
        This ledger contains safe control-plane status only—no credentials,
        provider payloads, broker instructions, or execution authority.
      </footer>
    </section>
  );
}
