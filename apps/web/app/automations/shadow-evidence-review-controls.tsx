"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type Entity = Record<string, unknown>;

type Props = {
  strategyInstanceId: string;
  scorecard?: Entity;
};

function value(entity: Entity | undefined, primary: string, legacy: string) {
  return entity?.[primary] ?? entity?.[legacy];
}

export function ShadowEvidenceReviewControls({
  strategyInstanceId,
  scorecard,
}: Props) {
  const router = useRouter();
  const gate = value(scorecard, "evidence_gate", "EvidenceGate") as
    | Entity
    | undefined;
  const status = String(value(gate, "status", "Status") ?? "UNAVAILABLE");
  const fingerprint = String(
    value(
      scorecard,
      "evidence_review_fingerprint",
      "EvidenceReviewFingerprint",
    ) ?? "",
  );
  const currentReviewed = Boolean(
    value(scorecard, "current_evidence_reviewed", "CurrentEvidenceReviewed"),
  );
  const latest = value(
    scorecard,
    "latest_evidence_review",
    "LatestEvidenceReview",
  ) as Entity | undefined;
  const reviewedAt = String(value(latest, "reviewed_at", "ReviewedAt") ?? "");
  const reviewable =
    status === "EVIDENCE_REVIEWABLE" && /^[0-9a-f]{64}$/.test(fingerprint);
  const staleReview = Boolean(latest) && !currentReviewed;
  const [mfaCode, setMFACode] = useState("");
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    const response = await fetch(
      `/api/strategy-instances/${strategyInstanceId}/shadow-evidence-reviews`,
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          evidence_fingerprint: fingerprint,
          confirm_non_live_review: confirmed,
          mfa_code: mfaCode,
        }),
      },
    );
    setBusy(false);
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as {
        error?: { code?: string };
      } | null;
      const code = body?.error?.code;
      setMessage(
        code === "EVIDENCE_SNAPSHOT_CHANGED"
          ? "The evidence changed while you were reviewing it. Refreshing will load the new immutable snapshot."
          : code === "EVIDENCE_NOT_REVIEWABLE"
            ? "The durable evidence gate is still collecting. No review was recorded."
            : code === "EVIDENCE_REVIEW_MFA_REQUIRED"
              ? "Use a fresh six-digit code from your authenticator app."
              : "The non-live evidence review was not recorded. Refresh and try again.",
      );
      return;
    }
    setMFACode("");
    setConfirmed(false);
    setMessage(
      "This exact Shadow evidence snapshot is now recorded as reviewed. No trading authority or broker action was created.",
    );
    router.refresh();
  }

  return (
    <section
      className="shadow-evidence-review"
      aria-label="Shadow evidence review"
    >
      <header>
        <div>
          <p className="eyebrow">IMMUTABLE OWNER REVIEW</p>
          <h2>Shadow evidence acknowledgment</h2>
          <p>
            Record that you reviewed one exact non-live evidence snapshot. This
            is governance evidence—not approval, profitability, live readiness,
            or permission to trade.
          </p>
        </div>
        <span className="shadow-evidence-boundary">SHADOW ONLY</span>
      </header>

      {currentReviewed && latest ? (
        <div className="shadow-evidence-review-state is-reviewed">
          <strong>Current snapshot reviewed</strong>
          <p>
            The MFA-backed record is immutable
            {reviewedAt ? (
              <>
                {" "}
                and was created{" "}
                <time dateTime={reviewedAt}>
                  {new Date(reviewedAt).toUTCString()}
                </time>
              </>
            ) : null}
            . New evidence will create a new fingerprint and require a separate
            review.
          </p>
        </div>
      ) : reviewable ? (
        <>
          {staleReview && (
            <div className="shadow-evidence-review-state is-stale" role="note">
              <strong>Evidence changed after the prior review</strong>
              <p>
                The previous record remains immutable, but it does not cover
                this newer snapshot.
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
              I reviewed this exact non-live evidence snapshot. I understand
              this does not approve or enable live trading.
            </label>
            <button type="submit" disabled={busy}>
              {busy ? "Recording…" : "Record non-live evidence review"}
            </button>
          </form>
        </>
      ) : (
        <div className="shadow-evidence-review-state is-collecting">
          <strong>Evidence is still collecting</strong>
          <p>
            This control unlocks only after Arbion records the required 1-hour
            and 24-hour samples across the full evidence window with a healthy
            scheduler. No form is available before then.
          </p>
        </div>
      )}

      <footer>
        No broker order is created. Live promotion and live execution remain
        unavailable.
      </footer>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
