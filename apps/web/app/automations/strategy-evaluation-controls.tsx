"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export type StrategyParameters = {
  symbols?: string[];
  minimum_dte?: number;
  maximum_dte?: number;
  target_delta?: string;
  target_delta_min?: string;
  target_delta_max?: string;
  minimum_premium?: string | null;
  maximum_contracts?: number;
  assignment_handling_policy?: string;
};

type Props = {
  automationId: string;
  currentVersion: number;
  status: string;
  executionMode: string;
  strategyIdentifier: string;
  instanceId: string;
  instanceStatus: string;
  strategyParameters: StrategyParameters;
};

function configured(parameters: StrategyParameters) {
  return (
    Array.isArray(parameters.symbols) &&
    parameters.symbols.length > 0 &&
    Number.isInteger(parameters.minimum_dte) &&
    Number.isInteger(parameters.maximum_dte) &&
    Boolean(parameters.target_delta) &&
    Boolean(parameters.target_delta_min) &&
    Boolean(parameters.target_delta_max) &&
    Number(parameters.maximum_contracts) > 0 &&
    parameters.assignment_handling_policy === "continue_wheel"
  );
}

export function StrategyEvaluationControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const hasParameters = configured(props.strategyParameters);
  const canEvaluate =
    hasParameters &&
    props.status === "READY" &&
    Boolean(props.instanceId) &&
    props.instanceStatus === "ACTIVE" &&
    (props.executionMode === "PAPER" || props.executionMode === "SHADOW");

  async function saveParameters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const symbols = String(form.get("symbols") ?? "")
      .split(",")
      .map((symbol) => symbol.trim().toUpperCase())
      .filter(Boolean);
    const minimumPremium = String(form.get("minimum_premium") ?? "").trim();
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/strategy-parameters`,
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: props.currentVersion,
          strategy_parameters: {
            symbols,
            minimum_dte: Number(form.get("minimum_dte")),
            maximum_dte: Number(form.get("maximum_dte")),
            target_delta: String(form.get("target_delta")),
            target_delta_min: String(form.get("target_delta_min")),
            target_delta_max: String(form.get("target_delta_max")),
            minimum_premium: minimumPremium || null,
            maximum_contracts: Number(form.get("maximum_contracts")),
            assignment_handling_policy: "continue_wheel",
          },
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage("The strategy parameters were not accepted.");
      return;
    }
    setMessage(
      "Parameters saved as a new DRAFT version. Review and mark it READY; no evaluation or broker order was sent.",
    );
    router.refresh();
  }

  async function evaluate() {
    setBusy(true);
    setMessage("");
    const random = globalThis.crypto?.randomUUID?.() ?? String(Date.now());
    const response = await fetch(
      `/api/strategy-instances/${props.instanceId}/evaluate`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ event_id: `manual:${random}` }),
      },
    );
    const body = (await response.json().catch(() => ({}))) as {
      evaluation?: {
        risk_decision?: string;
        risk_reason_codes?: string[];
        execution?: { status?: string };
      };
    };
    setBusy(false);
    if (!response.ok || !body.evaluation) {
      setMessage(
        "The manual evaluation failed closed. Nothing was sent to Schwab.",
      );
      return;
    }
    const reasons = body.evaluation.risk_reason_codes?.join(", ") || "none";
    setMessage(
      `${props.executionMode} evaluation recorded: ${body.evaluation.risk_decision} (${reasons}); ${body.evaluation.execution?.status}. No broker order was sent.`,
    );
    router.refresh();
  }

  const parameters = props.strategyParameters;
  return (
    <section className="mandate-controls" aria-label="Manual evaluation">
      <p className="eyebrow">READ-ONLY MARKET EVALUATION</p>
      <h2>Configure and evaluate manually</h2>
      <p>
        Quotes and option chains are read from Schwab. Every proposal passes the
        deterministic Risk/Control Engine and can create only PAPER or SHADOW
        records. An optional guarded schedule uses this same evaluation path;
        there is no broker-write route.
      </p>
      <form onSubmit={saveParameters}>
        <label>
          Symbols (comma separated)
          <input
            name="symbols"
            defaultValue={parameters.symbols?.join(", ") ?? ""}
            placeholder="AAPL"
            required
          />
        </label>
        <label>
          Minimum days to expiration
          <input
            name="minimum_dte"
            type="number"
            min="0"
            max="730"
            defaultValue={parameters.minimum_dte ?? 20}
            required
          />
        </label>
        <label>
          Maximum days to expiration
          <input
            name="maximum_dte"
            type="number"
            min="0"
            max="730"
            defaultValue={parameters.maximum_dte ?? 60}
            required
          />
        </label>
        <label>
          Target absolute delta
          <input
            name="target_delta"
            inputMode="decimal"
            defaultValue={parameters.target_delta ?? "0.30"}
            required
          />
        </label>
        <label>
          Minimum absolute delta
          <input
            name="target_delta_min"
            inputMode="decimal"
            defaultValue={parameters.target_delta_min ?? "0.20"}
            required
          />
        </label>
        <label>
          Maximum absolute delta
          <input
            name="target_delta_max"
            inputMode="decimal"
            defaultValue={parameters.target_delta_max ?? "0.40"}
            required
          />
        </label>
        <label>
          Minimum premium (optional)
          <input
            name="minimum_premium"
            inputMode="decimal"
            defaultValue={parameters.minimum_premium ?? ""}
          />
        </label>
        <label>
          Maximum contracts
          <input
            name="maximum_contracts"
            type="number"
            min="1"
            max="100"
            defaultValue={parameters.maximum_contracts ?? 1}
            required
          />
        </label>
        <button type="submit" disabled={busy}>
          Save Parameters — Creates Draft
        </button>
      </form>
      {!hasParameters && (
        <p className="security-note">
          Complete and save the deterministic parameters before evaluation.
        </p>
      )}
      {hasParameters && !props.instanceId && (
        <p className="security-note">
          Mark the current version READY and initialize its non-live strategy
          instance before evaluation.
        </p>
      )}
      {hasParameters && props.instanceStatus === "PAUSED" && (
        <p className="security-note">
          This non-live strategy is paused. Resume its exact READY mandate
          instance before running another evaluation.
        </p>
      )}
      {canEvaluate && (
        <button type="button" disabled={busy} onClick={evaluate}>
          Run Manual {props.executionMode} Evaluation — No Broker Order
        </button>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
