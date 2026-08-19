"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Props = {
  instanceId: string;
  status: string;
  stateVersion: number;
};

export function StrategyInstanceControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  if (props.status !== "ACTIVE" && props.status !== "PAUSED") return null;

  async function finish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/strategy-instances/${props.instanceId}/finish`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_state_version: props.stateVersion,
          confirm_non_live_finish: true,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "PAPER_POSITION_OPEN"
          ? "This PAPER simulation still has an open option or share position. Resolve its simulated lifecycle before finishing."
          : "The simulation was not finished. Refresh the page and review its latest state.",
      );
      return;
    }
    setMessage(
      "Simulation finished. Its account capital claim is released. No Schwab order or account change was made.",
    );
    router.refresh();
  }

  return (
    <section className="mandate-controls" aria-label="Simulation lifecycle">
      <p className="eyebrow">NON-LIVE CAPITAL CLAIM</p>
      <h2>Finish this simulation</h2>
      <p>
        Active and paused simulations keep the financial account&apos;s Arbion
        capital claim. PAPER must have no open simulated positions. Finishing is
        permanent, preserves all history, and only releases that claim. It never
        changes the Schwab account.
      </p>
      <form onSubmit={finish}>
        <label className="checkbox-row">
          <input type="checkbox" required />I understand this permanently
          finishes only the non-live simulation.
        </label>
        <button type="submit" disabled={busy}>
          {busy ? "Finishing…" : "Finish Non-Live Strategy"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
