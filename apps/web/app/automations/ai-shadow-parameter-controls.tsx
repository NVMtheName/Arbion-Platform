"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

import type { AIShadowParameters } from "./ai-shadow-evaluation-controls";

type Props = {
  automationId: string;
  currentVersion: number;
  status: string;
  hasActiveInstance: boolean;
  parameters: AIShadowParameters;
};

export function AIShadowParameterControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/ai-shadow-parameters`,
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: props.currentVersion,
          strategy_parameters: {
            objective: String(form.get("objective") ?? "").trim(),
            max_proposal_notional: String(
              form.get("max_proposal_notional") ?? "",
            ).trim(),
          },
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "VERSION_CONFLICT"
          ? "The mandate changed while you were reviewing it. Refresh and try again."
          : "The AI Shadow controls were not accepted. Use a positive USD ceiling no greater than $1,000,000,000.",
      );
      return;
    }
    setMessage(
      "Controls saved as a new immutable DRAFT. Review and mark it READY; no AI cycle or broker order was sent.",
    );
    router.refresh();
  }

  return (
    <section
      id="configuration-controls"
      className="mandate-controls"
      aria-label="AI shadow controls"
    >
      <p className="eyebrow">AI SHADOW DECISION ENVELOPE</p>
      <h2>Objective and proposal ceiling</h2>
      <p>
        This ceiling limits a hypothetical proposal recorded in the Shadow
        journal. It never grants broker-write access or enables live execution.
      </p>
      <form onSubmit={save}>
        <label>
          Trading objective
          <textarea
            name="objective"
            maxLength={500}
            defaultValue={props.parameters.objective ?? ""}
            required
          />
        </label>
        <label>
          Maximum hypothetical proposal (USD)
          <input
            name="max_proposal_notional"
            inputMode="decimal"
            min="0.0000000001"
            max="1000000000"
            step="any"
            defaultValue={props.parameters.max_proposal_notional ?? ""}
            required
          />
          <span className="field-hint">
            Shadow-only. A saved change creates a new DRAFT version and stops
            the current instance from evaluating until the reviewed version is
            initialized.
          </span>
        </label>
        <button type="submit" disabled={busy || props.status === "ARCHIVED"}>
          {busy ? "Saving immutable draft…" : "Save AI Shadow controls"}
        </button>
      </form>
      {props.hasActiveInstance && (
        <p className="security-note">
          After saving, finish the existing non-live instance and initialize the
          reviewed replacement. Historical decisions remain immutable.
        </p>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
