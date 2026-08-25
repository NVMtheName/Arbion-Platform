"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

export type AIShadowParameters = {
  objective?: string;
  max_proposal_notional?: string;
};

type Props = {
  status: string;
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
      setMessage(
        `The AI cycle failed closed${body.error?.code ? ` (${body.error.code})` : ""}. No Coinbase or Schwab order was sent.`,
      );
      return;
    }
    const result = body.evaluation;
    setMessage(
      result.ai_decision === "ABSTAIN"
        ? `Arbion abstained (${result.confidence ?? "unknown"} confidence). The reason is in the Decision Journal; no order was sent.`
        : `Shadow proposal recorded: ${result.risk_decision} · ${result.execution?.status}. No order was sent.`,
    );
    router.refresh();
  }

  return (
    <section className="mandate-controls" aria-label="AI shadow engine">
      <p className="eyebrow">AI SHADOW COMMAND LOOP</p>
      <h2>Observe → reason → risk-check → journal</h2>
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
          <strong>Broker access</strong>Read-only
        </p>
        <p>
          <strong>Execution result</strong>Shadow journal only
        </p>
      </div>
      <button disabled={!canEvaluate || busy} onClick={evaluate}>
        {busy ? "Running guarded cycle…" : "Run AI Shadow cycle now"}
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
