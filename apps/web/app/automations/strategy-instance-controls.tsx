"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Props = {
  instanceId: string;
  status: string;
  stateVersion: number;
  executionMode: string;
};

export function StrategyInstanceControls(props: Props) {
  const router = useRouter();
  const [busy, setBusy] = useState<"pause" | "resume" | "finish" | "">("");
  const [message, setMessage] = useState("");
  if (props.status !== "ACTIVE" && props.status !== "PAUSED") return null;

  async function changeStatus(action: "pause" | "resume") {
    setBusy(action);
    setMessage("");
    const response = await fetch(
      `/api/strategy-instances/${props.instanceId}/${action}`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_state_version: props.stateVersion,
          ...(action === "resume" ? { confirm_non_live_resume: true } : {}),
        }),
      },
    );
    setBusy("");
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      setMessage(
        body?.error?.code === "MANDATE_NOT_READY"
          ? "This strategy cannot resume because its exact mandate version is no longer current and READY. You may still resolve an open PAPER lifecycle while paused, then finish it."
          : `The simulation was not ${action === "pause" ? "paused" : "resumed"}. Refresh the page and review its latest state.`,
      );
      return;
    }
    setMessage(
      action === "pause"
        ? "Simulation paused. Scheduled and manual evaluations are stopped, its capital claim is retained, and no connected-account change was made."
        : "Simulation resumed under its exact READY mandate. It is eligible for non-live evaluation again; no broker order was sent.",
    );
    router.refresh();
  }

  async function finish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy("finish");
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
    setBusy("");
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
      "Simulation finished. Its Arbion policy reservation is released. No broker order or connected-account change was made.",
    );
    router.refresh();
  }

  return (
    <section
      id="strategy-instance-controls"
      className="mandate-controls"
      aria-label="Simulation lifecycle"
    >
      <p className="eyebrow">NON-LIVE CAPITAL CLAIM</p>
      <h2>
        {props.status === "ACTIVE"
          ? "Pause this simulation"
          : "Resume this simulation"}
      </h2>
      <p>
        Pausing stops new manual and scheduled evaluations immediately while
        preserving state, history, and the financial account&apos;s Arbion
        capital claim. A paused PAPER lifecycle can still be resolved without
        resuming.
      </p>
      {props.status === "ACTIVE" ? (
        <button
          type="button"
          disabled={busy !== ""}
          onClick={() => void changeStatus("pause")}
        >
          {busy === "pause" ? "Pausing…" : "Pause Non-Live Strategy"}
        </button>
      ) : (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void changeStatus("resume");
          }}
        >
          <label className="checkbox-row">
            <input type="checkbox" required />I understand resuming makes this
            simulation eligible for non-live evaluation under its exact READY
            mandate.
          </label>
          <button type="submit" disabled={busy !== ""}>
            {busy === "resume" ? "Resuming…" : "Resume Non-Live Strategy"}
          </button>
        </form>
      )}
      <h2>Finish this simulation</h2>
      <p>
        {props.executionMode === "PAPER"
          ? "Active and paused PAPER simulations retain their isolated simulated-cash reservation. PAPER must have no open simulated positions. Finishing is permanent, preserves all history, and releases only that simulated reservation. It never changes the connected account."
          : "Active and paused SHADOW simulations retain their Arbion account-policy reservation. Finishing is permanent, preserves all history, and releases only that policy reservation. It never changes the connected account."}
      </p>
      <form onSubmit={finish}>
        <label className="checkbox-row">
          <input type="checkbox" required />I understand this permanently
          finishes only the non-live simulation.
        </label>
        <button type="submit" disabled={busy !== ""}>
          {busy === "finish" ? "Finishing…" : "Finish Non-Live Strategy"}
        </button>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
