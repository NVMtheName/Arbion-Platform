"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type StrategyScheduleConditions = {
  enabled?: boolean;
  interval_minutes?: number;
  session?: string;
  notifications?: {
    evaluation_completed?: boolean;
    lifecycle_required?: boolean;
    first_failure?: boolean;
  };
};

export type StrategyScheduleStatus = {
  enabled?: boolean;
  interval_minutes?: number;
  session?: string;
  next_run_at?: string;
  last_started_at?: string;
  last_completed_at?: string;
  last_status?: string;
  last_error_code?: string;
  consecutive_failures?: number;
};

type Props = {
  automationId: string;
  currentVersion: number;
  automationType: string;
  autonomyLevel: string;
  executionMode: string;
  instanceId: string;
  conditions: StrategyScheduleConditions;
  runtime?: StrategyScheduleStatus;
  schedulerEnabled: boolean;
  emailDeliveryAvailable: boolean;
};

function readableTime(value?: string) {
  if (!value) return "—";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? "—" : parsed.toLocaleString();
}

function scheduleFailureGuidance(code: string) {
  switch (code) {
    case "AUTHORIZATION_FAILED":
    case "AUTHORIZATION_EXPIRED":
      return "Reconnect Schwab before the next scheduled evaluation.";
    case "PROVIDER":
    case "PROVIDER_UNAVAILABLE":
    case "RATE_LIMITED":
    case "TIMEOUT":
      return "Schwab market data was unavailable. Arbion will remain fail-closed and can try again at the next eligible run.";
    case "MARKET_DATA_STALE":
      return "Schwab's quote or option-chain timestamp was not current. Arbion can try again after market data refreshes.";
    case "NO_ELIGIBLE_OPTION_CONTRACTS":
      return "No option contract matched the saved expiration, delta, and premium filters. Review those filters if this repeats.";
    case "STRATEGY_NOT_ACTIVE":
      return "Resume the non-live strategy if you want scheduled evaluations to continue.";
    case "STRATEGY_CONFIGURATION_CHANGED":
      return "The initialized strategy no longer matches its current mandate, capital bucket, or account. Review the automation before continuing.";
    case "STRATEGY_PARAMETERS_INVALID":
      return "Review and save valid deterministic strategy parameters before continuing.";
    case "PAPER_STATE_UNAVAILABLE":
      return "The PAPER portfolio state needed for evaluation is unavailable. Keep the schedule paused until the state is reviewed.";
    case "WAITING_FOR_LIFECYCLE":
      return "Record the explicit PAPER option lifecycle outcome before another evaluation can run.";
    case "OUTSIDE_SESSION":
      return "No action is needed. Arbion will wait for the next supported U.S. equities session.";
    case "SESSION_CALENDAR_UNAVAILABLE":
      return "The verified market-session calendar does not cover this date. The operator must extend it before scheduling resumes.";
    case "CANCELED":
      return "The run was canceled before completion. Arbion can try again at the next eligible run.";
    case "CONFLICT":
      return "The strategy state changed during the run. Refresh this page and review its latest state.";
    case "FORBIDDEN":
    case "NOT_FOUND":
      return "The schedule can no longer access its required owner-scoped records. Review the automation before continuing.";
    default:
      return "The run failed closed. Review the automation and recent activity before continuing.";
  }
}

export function StrategyScheduleControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [enabled, setEnabled] = useState(Boolean(props.conditions.enabled));

  const eligible =
    props.automationType === "STRATEGY" &&
    props.autonomyLevel === "STRATEGY_AUTONOMOUS" &&
    (props.executionMode === "PAPER" || props.executionMode === "SHADOW");

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const schedule = enabled
      ? {
          enabled: true,
          interval_minutes: Number(form.get("interval_minutes")),
          session: "US_EQUITIES_REGULAR",
          notifications: {
            evaluation_completed:
              form.get("notify_evaluation_completed") === "on",
            lifecycle_required: form.get("notify_lifecycle_required") === "on",
            first_failure: form.get("notify_first_failure") === "on",
          },
        }
      : { enabled: false };
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/schedule`,
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: props.currentVersion,
          schedule_conditions: schedule,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The schedule was not accepted. It requires a strategy-autonomous PAPER or SHADOW mandate.",
      );
      return;
    }
    setMessage(
      enabled
        ? "Schedule saved as a new DRAFT version. Review it, mark it READY, and initialize that version before anything can run."
        : "Schedule disabled in a new DRAFT version. No evaluation was run.",
    );
    router.refresh();
  }

  return (
    <section className="mandate-controls" aria-label="Non-live schedule">
      <p className="eyebrow">GUARDED NON-LIVE SCHEDULE</p>
      <h2>Optional market-session evaluations</h2>
      <p>
        The scheduler can create only PAPER or SHADOW records during the U.S.
        regular market session. It cannot place a live order. A PAPER fill then
        waits for an explicit lifecycle event before another evaluation.
      </p>
      {!eligible && (
        <p className="security-note">
          Scheduling requires STRATEGY_AUTONOMOUS with PAPER or SHADOW mode.
        </p>
      )}
      <form onSubmit={save}>
        <label>
          <input
            name="enabled"
            type="checkbox"
            checked={enabled}
            disabled={!eligible || busy}
            onChange={(event) => setEnabled(event.currentTarget.checked)}
          />{" "}
          Enable guarded non-live schedule
        </label>
        <label>
          Evaluation interval
          <select
            name="interval_minutes"
            disabled={!enabled || !eligible || busy}
            defaultValue={String(props.conditions.interval_minutes ?? 60)}
          >
            <option value="30">Every 30 minutes</option>
            <option value="60">Every hour</option>
            <option value="120">Every 2 hours</option>
            <option value="240">Every 4 hours</option>
          </select>
        </label>
        <p>
          Session: 9:35 a.m.–3:55 p.m. America/New_York, weekdays. Provider
          freshness checks fail closed on market holidays or stale data.
        </p>
        <fieldset
          disabled={
            !enabled || !eligible || busy || !props.emailDeliveryAvailable
          }
        >
          <legend>Informational email</legend>
          <label>
            <input
              name="notify_evaluation_completed"
              type="checkbox"
              defaultChecked={Boolean(
                props.conditions.notifications?.evaluation_completed,
              )}
            />{" "}
            Email after each scheduled evaluation
          </label>
          <label>
            <input
              name="notify_lifecycle_required"
              type="checkbox"
              defaultChecked={Boolean(
                props.conditions.notifications?.lifecycle_required,
              )}
            />{" "}
            Email once when a PAPER option needs lifecycle review
          </label>
          <label>
            <input
              name="notify_first_failure"
              type="checkbox"
              defaultChecked={Boolean(
                props.conditions.notifications?.first_failure,
              )}
            />{" "}
            Email on the first consecutive scheduler failure
          </label>
        </fieldset>
        {props.emailDeliveryAvailable ? (
          <p className="security-note">
            Emails go only to your verified Arbion address. They are
            informational and contain no approval, continuation, or execution
            action.
          </p>
        ) : (
          <p className="security-note">
            Email delivery is not configured, so notification options are
            unavailable.
          </p>
        )}
        <button type="submit" disabled={!eligible || busy}>
          Save Schedule — Creates Draft
        </button>
      </form>
      {props.conditions.enabled && !props.instanceId && (
        <p className="security-note">
          This saved schedule is not running. Mark this exact version READY and
          initialize its non-live strategy instance.
        </p>
      )}
      {props.runtime?.enabled && !props.schedulerEnabled && (
        <p className="security-note">
          This schedule is saved but the production scheduler is currently
          paused by the operator.
        </p>
      )}
      {props.runtime?.enabled && (
        <>
          <div className="review-grid" aria-label="Schedule status">
            <p>
              <strong>Next evaluation</strong>
              {readableTime(props.runtime.next_run_at)}
            </p>
            <p>
              <strong>Last result</strong>
              {props.runtime.last_status ?? "Not run"}
              {props.runtime.last_error_code
                ? ` — ${props.runtime.last_error_code}`
                : ""}
            </p>
            <p>
              <strong>Last completed</strong>
              {readableTime(props.runtime.last_completed_at)}
            </p>
            <p>
              <strong>Consecutive failures</strong>
              {props.runtime.consecutive_failures ?? 0}
            </p>
          </div>
          {props.runtime.last_error_code && (
            <p className="security-note">
              <strong>What to do</strong>{" "}
              {scheduleFailureGuidance(props.runtime.last_error_code)} No broker
              order was sent.
            </p>
          )}
        </>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
