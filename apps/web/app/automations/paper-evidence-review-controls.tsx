"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

import type { PaperPortfolio } from "./paper-portfolio-summary";

export type PaperEvidenceReviewRecord = {
  id: string;
  financial_account_id: string;
  mandate_version: number;
  evidence_fingerprint: string;
  gate_status: "EVIDENCE_REVIEWABLE";
  evidence_started_at: string;
  evidence_eligible_at: string;
  evidence_as_of: string;
  evidence_window_hours: number;
  decision_count: number;
  latest_checkpoint_run_id: string;
  latest_checkpoint_as_of: string;
  scheduler_sample_count: number;
  scheduler_success_count: number;
  scheduler_failure_count: number;
  last_schedule_status: "SUCCEEDED";
  consecutive_schedule_failures: 0;
  route_continuity_status: "STABLE" | "CONTEXT_CHANGED";
  input_coverage_status: "COMPLETE";
  input_freshness_status: "CURRENT_AT_DECISION";
  ledger_contract_status: "RECONCILED";
  no_live_safety_status: "CLEAR";
  execution_boundary: "PAPER_SIMULATION_ONLY";
  review_scope: "PAPER_NON_LIVE_EVIDENCE_ONLY";
  grants_authority: false;
  live_promotion_available: false;
  mfa_method: "totp";
  reviewed_at: string;
};

type Props = {
  strategyInstanceId: string;
  portfolio?: PaperPortfolio;
  initialReviews?: PaperEvidenceReviewRecord[];
  initialCursor?: string;
  historyAvailable?: boolean;
};

export function PaperEvidenceReviewControls({
  strategyInstanceId,
  portfolio,
  initialReviews = [],
  initialCursor = "",
  historyAvailable = true,
}: Props) {
  const router = useRouter();
  const gate = portfolio?.evidence_readiness;
  const status = gate?.status ?? "UNAVAILABLE";
  const fingerprint = portfolio?.evidence_review_fingerprint ?? "";
  const latest = portfolio?.latest_evidence_review;
  const currentReviewed = portfolio?.current_evidence_reviewed === true;
  const reviewable =
    status === "EVIDENCE_REVIEWABLE" && /^[0-9a-f]{64}$/.test(fingerprint);
  const [mfaCode, setMFACode] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [reviews, setReviews] = useState(initialReviews);
  const [historyCursor, setHistoryCursor] = useState(initialCursor);
  const [historyBusy, setHistoryBusy] = useState(false);
  const [historyMessage, setHistoryMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(strategyInstanceId)}/paper-evidence-reviews`,
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({
            evidence_fingerprint: fingerprint,
            confirm_paper_review: confirmed,
            mfa_code: mfaCode,
          }),
        },
      );
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as {
          error?: { code?: string };
        } | null;
        const code = body?.error?.code;
        setMessage(
          code === "EVIDENCE_SNAPSHOT_CHANGED"
            ? "The Paper evidence changed while you were reviewing it. Refreshing will load the new immutable checkpoint."
            : code === "EVIDENCE_NOT_REVIEWABLE"
              ? "The Paper evidence gate is not reviewable. No acknowledgment was recorded."
              : code === "EVIDENCE_REVIEW_MFA_REQUIRED"
                ? "Use a fresh six-digit code from your authenticator app."
                : "The Paper evidence acknowledgment was not recorded. Refresh and try again.",
        );
        return;
      }
      setMFACode("");
      setConfirmed(false);
      setMessage(
        "This exact Paper gate and scheduler checkpoint are now recorded as reviewed. No promotion, trading authority, or broker action was created.",
      );
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  async function loadEarlierReviews() {
    if (!historyCursor || historyBusy) return;
    setHistoryBusy(true);
    setHistoryMessage("");
    try {
      const response = await fetch(
        `/api/strategy-instances/${encodeURIComponent(strategyInstanceId)}/paper-evidence-reviews?limit=8&cursor=${encodeURIComponent(historyCursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        evidence_reviews?: PaperEvidenceReviewRecord[];
        next_cursor?: string;
      } | null;
      if (!response.ok || !body || !Array.isArray(body.evidence_reviews)) {
        setHistoryMessage("Earlier Paper review evidence could not be loaded.");
        return;
      }
      setReviews((current) => [
        ...current,
        ...body.evidence_reviews!.filter(
          (candidate) => !current.some((review) => review.id === candidate.id),
        ),
      ]);
      setHistoryCursor(body.next_cursor ?? "");
    } catch {
      setHistoryMessage("Earlier Paper review evidence could not be loaded.");
    } finally {
      setHistoryBusy(false);
    }
  }

  return (
    <section
      id="paper-evidence-review"
      className="shadow-evidence-review"
      aria-label="Paper evidence review"
    >
      <header>
        <div>
          <p className="eyebrow">IMMUTABLE OWNER REVIEW</p>
          <h2>Paper evidence acknowledgment</h2>
          <p>
            Record that you reviewed one exact Paper gate and its latest saved
            scheduler checkpoint. This record is governance evidence only—not
            promotion, approval, profitability, or permission to trade.
          </p>
        </div>
        <span className="shadow-evidence-boundary">PAPER ONLY</span>
      </header>

      {currentReviewed && latest ? (
        <div className="shadow-evidence-review-state is-reviewed">
          <strong>Current Paper checkpoint reviewed</strong>
          <p>
            The MFA-backed record is immutable and was created{" "}
            <time dateTime={latest.reviewed_at}>
              {new Date(latest.reviewed_at).toUTCString()}
            </time>
            . A changed gate or checkpoint requires a separate acknowledgment.
          </p>
        </div>
      ) : reviewable ? (
        <>
          {latest && (
            <div className="shadow-evidence-review-state is-stale" role="note">
              <strong>Paper evidence changed after the prior review</strong>
              <p>
                The earlier record remains immutable but does not cover this
                newer checkpoint.
              </p>
            </div>
          )}
          <form onSubmit={submit}>
            <label>
              Authenticator code
              <input
                required
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]{6}"
                maxLength={6}
                value={mfaCode}
                onChange={(event) => setMFACode(event.target.value)}
                placeholder="6-digit code"
              />
            </label>
            <label className="checkbox-row">
              <input
                required
                type="checkbox"
                checked={confirmed}
                onChange={(event) => setConfirmed(event.target.checked)}
              />
              I reviewed this exact Paper evidence packet and checkpoint. I
              understand this does not approve or enable live trading.
            </label>
            <button type="submit" disabled={busy}>
              {busy ? "Recording…" : "Record Paper evidence review"}
            </button>
          </form>
        </>
      ) : (
        <div className="shadow-evidence-review-state is-collecting">
          <strong>
            {status === "COLLECTING_EVIDENCE"
              ? "Paper evidence is still collecting"
              : "Paper evidence review is unavailable"}
          </strong>
          <p>
            The acknowledgment form appears only when the full seven-day gate,
            decision sample, scheduler, input, ledger, and no-live checks are
            simultaneously reviewable. Arbion will not record a partial or stale
            review.
          </p>
        </div>
      )}

      <footer>
        No mandate, strategy, account, broker order, promotion, or execution
        setting is changed by this record.
      </footer>
      {message && <p role="status">{message}</p>}

      <div className="shadow-evidence-review-ledger">
        <header>
          <div>
            <p className="eyebrow">PAPER REVIEW LEDGER</p>
            <h3>Every exact acknowledgment, preserved.</h3>
          </div>
          <span>{reviews.length} loaded</span>
        </header>
        {!historyAvailable ? (
          <div
            className="shadow-evidence-review-state is-collecting"
            role="status"
          >
            <strong>Paper review history is temporarily unavailable</strong>
            <p>Arbion will not infer an empty immutable ledger.</p>
          </div>
        ) : reviews.length === 0 ? (
          <div className="shadow-evidence-review-ledger-empty">
            <strong>No MFA-backed Paper reviews recorded yet.</strong>
            <p>The ledger begins only after the complete gate is reviewable.</p>
          </div>
        ) : (
          <ol className="shadow-evidence-review-ledger-list">
            {reviews.map((review) => {
              const isCurrent =
                currentReviewed && review.evidence_fingerprint === fingerprint;
              return (
                <li
                  className={isCurrent ? "is-current" : "is-prior"}
                  key={review.id}
                >
                  <header>
                    <div>
                      <time dateTime={review.reviewed_at}>
                        {new Date(review.reviewed_at).toUTCString()}
                      </time>
                      <strong>
                        {isCurrent
                          ? "Current Paper checkpoint"
                          : "Earlier Paper checkpoint"}
                      </strong>
                    </div>
                    <span>{isCurrent ? "CURRENT" : "PRESERVED"}</span>
                  </header>
                  <code title={review.evidence_fingerprint}>
                    {review.evidence_fingerprint}
                  </code>
                  <dl>
                    <div>
                      <dt>Mandate</dt>
                      <dd>Version {review.mandate_version}</dd>
                    </div>
                    <div>
                      <dt>Decisions</dt>
                      <dd>{review.decision_count}</dd>
                    </div>
                    <div>
                      <dt>Evidence window</dt>
                      <dd>{review.evidence_window_hours} hours</dd>
                    </div>
                    <div>
                      <dt>Latest checkpoint</dt>
                      <dd>{review.latest_checkpoint_run_id}</dd>
                    </div>
                    <div>
                      <dt>Schedule</dt>
                      <dd>
                        {review.last_schedule_status} ·{" "}
                        {review.consecutive_schedule_failures} failures
                      </dd>
                    </div>
                    <div>
                      <dt>Boundary</dt>
                      <dd>{review.execution_boundary}</dd>
                    </div>
                  </dl>
                  <p>
                    MFA-backed · {review.review_scope} · grants no authority
                  </p>
                </li>
              );
            })}
          </ol>
        )}
        {historyCursor && historyAvailable && (
          <button
            type="button"
            onClick={loadEarlierReviews}
            disabled={historyBusy}
          >
            {historyBusy ? "Loading…" : "Load earlier reviews"}
          </button>
        )}
        {historyMessage && <p role="status">{historyMessage}</p>}
      </div>
    </section>
  );
}
