"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type MandateControlsProps = {
  automationId: string;
  currentVersion: number;
  status: string;
  automationType: string;
  executionMode: string;
  strategyIdentifier: string;
  instanceExists: boolean;
};

const initializableStrategies = new Set([
  "wheel",
  "covered_call",
  "cash_secured_put",
]);

export function MandateControls(props: MandateControlsProps) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function transition(action: "ready" | "pause" | "disable") {
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/${action}`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ expected_version: props.currentVersion }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage("The mandate changed or did not pass its safety checks.");
      return;
    }
    setMessage(
      action === "ready"
        ? "Mandate marked READY. No strategy was run and no order was sent."
        : action === "pause"
          ? "Mandate paused."
          : "Mandate disabled.",
    );
    router.refresh();
  }

  async function initialize(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/strategy/initialize`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          starting_cash:
            props.executionMode === "PAPER" ? data.get("starting_cash") : "",
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      if (body?.error?.code === "PAPER_CAPITAL_LIMIT") {
        setMessage(
          "Starting simulated cash must fit within the selected capital bucket after protected amounts and absolute limits.",
        );
      } else if (body?.error?.code === "ACCOUNT_CAPITAL_IN_USE") {
        setMessage(
          "This financial account already has an active or paused simulation. Finish that strategy before starting another; paused strategies keep their capital claim.",
        );
      } else {
        setMessage("The non-live strategy could not be initialized.");
      }
      return;
    }
    setMessage(
      `${props.executionMode} strategy initialized. No broker order was sent.`,
    );
    router.refresh();
  }

  const canInitialize =
    props.status === "READY" &&
    props.automationType === "STRATEGY" &&
    (props.executionMode === "PAPER" || props.executionMode === "SHADOW") &&
    initializableStrategies.has(props.strategyIdentifier) &&
    !props.instanceExists;

  return (
    <section className="mandate-controls" aria-label="Mandate controls">
      <p className="eyebrow">SAFE LIFECYCLE CONTROLS</p>
      <h2>Review and initialize</h2>
      <p>
        READY authorizes only this saved configuration. Initialization creates a
        non-live strategy record; it cannot place or prepare a broker order.
      </p>
      <div className="connection-actions">
        {(props.status === "DRAFT" ||
          props.status === "PAUSED" ||
          props.status === "DISABLED") && (
          <button disabled={busy} onClick={() => transition("ready")}>
            Mark Ready — No Execution
          </button>
        )}
        {props.status === "READY" && (
          <button
            className="secondary"
            disabled={busy}
            onClick={() => transition("pause")}
          >
            Pause Mandate
          </button>
        )}
        {props.status !== "DISABLED" && props.status !== "ARCHIVED" && (
          <button
            className="secondary"
            disabled={busy}
            onClick={() => transition("disable")}
          >
            Disable Mandate
          </button>
        )}
      </div>
      {canInitialize && (
        <form onSubmit={initialize}>
          {props.executionMode === "PAPER" && (
            <label>
              Starting simulated cash (USD)
              <input
                name="starting_cash"
                inputMode="decimal"
                min="0.0000000001"
                step="any"
                required
              />
              <span className="field-hint">
                Must fit within this mandate&apos;s capital bucket after
                protected amounts and absolute limits.
              </span>
            </label>
          )}
          <button type="submit" disabled={busy}>
            Initialize {props.executionMode} Strategy
          </button>
        </form>
      )}
      {props.instanceExists && (
        <p className="security-note">
          This mandate already has a durable non-live strategy instance.
        </p>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
