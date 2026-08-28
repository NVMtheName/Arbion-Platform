"use client";

import { useState } from "react";

import { AIDecisionJournal } from "./ai-decision-journal";
import { DecisionReplayLab } from "./decision-replay-lab";

type RawRecord = Record<string, unknown>;

type Props = {
  strategyInstanceId: string;
  initialDecisions?: RawRecord[];
  initialOutcomes?: RawRecord[];
  initialCursor?: string;
  historyAvailable?: boolean;
};

function recordID(record: RawRecord) {
  const value = record.id ?? record.ID;
  return typeof value === "string" ? value : "";
}

function appendUnique(current: RawRecord[], incoming: RawRecord[]) {
  const seen = new Set(current.map(recordID).filter(Boolean));
  return [
    ...current,
    ...incoming.filter((record) => {
      const id = recordID(record);
      if (!id || seen.has(id)) return false;
      seen.add(id);
      return true;
    }),
  ];
}

export function AIShadowDecisionWorkspace({
  strategyInstanceId,
  initialDecisions = [],
  initialOutcomes = [],
  initialCursor = "",
  historyAvailable = true,
}: Props) {
  const [decisions, setDecisions] = useState(initialDecisions);
  const [outcomes, setOutcomes] = useState(initialOutcomes);
  const [cursor, setCursor] = useState(initialCursor);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function loadEarlier() {
    if (!cursor || busy) return;
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(strategyInstanceId)}/decisions?limit=24&cursor=${encodeURIComponent(cursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        decisions?: RawRecord[];
        outcomes?: RawRecord[];
        next_cursor?: string;
        decision_history_semantics?: string;
      } | null;
      if (
        !response.ok ||
        !body ||
        body.decision_history_semantics !==
          "IMMUTABLE_OWNER_STRATEGY_DECISION_HISTORY" ||
        !Array.isArray(body.decisions) ||
        !Array.isArray(body.outcomes)
      ) {
        setMessage("Earlier immutable decision evidence could not be loaded.");
        return;
      }
      const earlierDecisions = body.decisions;
      const matchedOutcomes = body.outcomes;
      setDecisions((current) => appendUnique(current, earlierDecisions));
      setOutcomes((current) => appendUnique(current, matchedOutcomes));
      setCursor(body.next_cursor ?? "");
    } catch {
      setMessage("Earlier immutable decision evidence could not be loaded.");
    } finally {
      setBusy(false);
    }
  }

  if (!historyAvailable) {
    return (
      <section
        className="ai-shadow-decision-history-unavailable"
        aria-label="AI decision history unavailable"
        role="status"
      >
        <p className="eyebrow">AI DECISION HISTORY</p>
        <h2>Immutable decision evidence is temporarily unavailable.</h2>
        <p>
          Arbion will not infer an empty history. Refresh after the durable
          journal service recovers; no model, provider, or broker action is
          triggered by this view.
        </p>
      </section>
    );
  }

  return (
    <>
      <DecisionReplayLab decisions={decisions} outcomes={outcomes} />
      <div className="ai-shadow-decision-history-controls">
        <div>
          <strong>{decisions.length} immutable decisions loaded</strong>
          <p>
            Replay includes every loaded page with only its matching outcome
            marks. The summary below stays focused on the five newest choices.
          </p>
        </div>
        {cursor ? (
          <button type="button" onClick={loadEarlier} disabled={busy}>
            {busy ? "Loading…" : "Load earlier decisions"}
          </button>
        ) : (
          <span>All available decisions loaded</span>
        )}
      </div>
      {message && (
        <p className="ai-shadow-decision-history-message" role="status">
          {message}
        </p>
      )}
      <AIDecisionJournal decisions={decisions} outcomes={outcomes} />
    </>
  );
}
