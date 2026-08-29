"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export type AIShadowParameters = {
  objective?: string;
  max_proposal_notional?: string;
};

type Props = {
  status: string;
  executionMode?: string;
  instanceId: string;
  instanceStatus: string;
  parameters: AIShadowParameters;
  symbols: string[];
};

export function AIShadowEvaluationControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const canEvaluate =
    props.status === "READY" &&
    Boolean(props.instanceId) &&
    props.instanceStatus === "ACTIVE";
  const paper = props.executionMode === "PAPER";

  async function evaluate() {
    setBusy(true);
    setMessage("");
    const event = globalThis.crypto?.randomUUID?.() ?? String(Date.now());
    const response = await fetch(
      `/api/strategy-instances/${props.instanceId}/evaluate`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ event_id: `manual-ai:${event}` }),
      },
    );
    const body = (await response.json().catch(() => ({}))) as {
      evaluation?: {
        ai_decision?: string;
        confidence?: string;
        risk_decision?: string;
        risk_reason_codes?: string[];
        execution?: { status?: string };
      };
      error?: { code?: string };
    };
    setBusy(false);
    if (!response.ok || !body.evaluation) {
      const code = body.error?.code;
      setMessage(
        code === "AI_DECISION_BUDGET_EXHAUSTED"
          ? "Arbion's hourly AI decision budget is currently used. Wait for the window to reset; the schedule remains safe and no order was sent."
          : code === "AI_PROVIDER_RATE_LIMITED"
            ? "The AI provider is temporarily rate limited. Try again later; the schedule remains safe and no order was sent."
            : `The AI cycle failed closed${code ? ` (${code})` : ""}. No Coinbase or Schwab order was sent.`,
      );
      return;
    }
    const result = body.evaluation;
    const repeatHeld = result.risk_reason_codes?.includes(
      "REPEAT_ACTION_COOLDOWN_ACTIVE",
    );
    setMessage(
      result.ai_decision === "ABSTAIN"
        ? `Arbion abstained (${result.confidence ?? "unknown"} confidence). The reason is in the Decision Journal; no order was sent.`
        : repeatHeld
          ? "The model proposed repeating the same symbol and side inside one hour. Arbion held it at the deterministic control gate and journaled the evidence; no order was sent."
          : `${paper ? "Paper proposal processed" : "Shadow proposal recorded"}: ${result.risk_decision} · ${result.execution?.status}. No order was sent.`,
    );
    router.refresh();
  }

  return (
    <section
      className="mandate-controls"
      aria-label={`AI ${paper ? "paper" : "shadow"} engine`}
    >
      <p className="eyebrow">AI {paper ? "PAPER" : "SHADOW"} COMMAND LOOP</p>
      <h2>
        {paper
          ? "Price → reason → risk-check → simulate → journal"
          : "Observe → reason → risk-check → journal"}
      </h2>
      <p>{props.parameters.objective ?? "No objective saved."}</p>
      <div className="review-grid">
        <p>
          <strong>Allowed universe</strong>
          {props.symbols.join(", ") || "—"}
        </p>
        <p>
          <strong>Per-decision ceiling</strong>$
          {props.parameters.max_proposal_notional ?? "—"}
        </p>
        <p>
          <strong>Broker access</strong>
          {paper ? "Market prices only" : "Read-only"}
        </p>
        <p>
          <strong>Execution result</strong>
          {paper ? "Isolated simulated ledger only" : "Shadow journal only"}
        </p>
        <p>
          <strong>Repeat-action guard</strong>One hour · same symbol and side
        </p>
      </div>
      <button disabled={!canEvaluate || busy} onClick={evaluate}>
        {busy
          ? "Running guarded cycle…"
          : `Run AI ${paper ? "Paper" : "Shadow"} cycle now`}
      </button>
      {!props.instanceId && (
        <p className="security-note">
          Mark the mandate READY and initialize its AI Engine first.
        </p>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
