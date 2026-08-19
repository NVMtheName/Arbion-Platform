"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Props = {
  automationId: string;
  currentVersion: number;
  automationType: string;
  autonomyLevel: string;
  executionMode: string;
  hasActiveInstance: boolean;
};

export function StrategyAutonomyControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const eligible =
    props.automationType === "STRATEGY" &&
    props.autonomyLevel === "RESEARCH_ONLY" &&
    (props.executionMode === "PAPER" || props.executionMode === "SHADOW");

  if (!eligible) return null;

  async function authorize(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/automations/${props.automationId}/autonomy`,
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: props.currentVersion,
          autonomy_level: "STRATEGY_AUTONOMOUS",
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      setMessage(
        "The autonomy change was not saved. Refresh and review the latest mandate version.",
      );
      return;
    }
    setMessage(
      `Strategy autonomy saved as a new DRAFT version in ${props.executionMode} mode. Scheduling remains off and no broker order was sent.`,
    );
    router.refresh();
  }

  return (
    <section className="mandate-controls" aria-label="Non-live autonomy">
      <p className="eyebrow">NON-LIVE AUTONOMY</p>
      <h2>Authorize the deterministic strategy</h2>
      <p>
        This creates a new DRAFT mandate version that may evaluate only the
        saved deterministic strategy. Execution stays {props.executionMode};
        scheduling remains off unless you separately enable it, and Arbion has
        no broker-order route.
      </p>
      {props.hasActiveInstance ? (
        <p className="security-note">
          Finish the active or paused non-live simulation before changing this
          mandate.
        </p>
      ) : (
        <form onSubmit={authorize}>
          <label className="checkbox-row">
            <input type="checkbox" required />I authorize this saved strategy to
            evaluate autonomously in {props.executionMode} mode only.
          </label>
          <button type="submit" disabled={busy}>
            {busy ? "Creating Draft…" : "Create Strategy-Autonomous Draft"}
          </button>
        </form>
      )}
      {message && <p role="status">{message}</p>}
    </section>
  );
}
