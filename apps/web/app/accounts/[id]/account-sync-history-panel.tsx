"use client";

import { useState } from "react";

import {
  type AccountSyncCheckpoint,
  type AccountSyncHistoryResult,
  type SyncHistoryAccount,
  projectAccountSyncHistory,
} from "./account-sync-history";
import type {
  ConnectionSyncAttempt,
  ConnectionSyncAttemptHistoryResult,
} from "../../settings/connections/connection-sync-attempt-history";

function providerLabel(provider: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Charles Schwab";
  return provider;
}

function readableTime(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unavailable";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

function readableDuration(milliseconds: number) {
  if (milliseconds < 1000) return String(milliseconds) + " ms";
  return (milliseconds / 1000).toFixed(3) + " seconds";
}

function isStrictlyOlder(
  newer: AccountSyncCheckpoint,
  older: AccountSyncCheckpoint,
) {
  const newerTime = new Date(newer.completedAt).valueOf();
  const olderTime = new Date(older.completedAt).valueOf();
  return (
    olderTime < newerTime ||
    (olderTime === newerTime && older.id.localeCompare(newer.id) < 0)
  );
}

export function AccountSyncHistoryPanel({
  account,
  initial,
  initialAttempts,
}: {
  account: SyncHistoryAccount;
  initial: AccountSyncHistoryResult;
  initialAttempts: ConnectionSyncAttemptHistoryResult;
}) {
  const [checkpoints, setCheckpoints] = useState(initial.checkpoints);
  const [nextCursor, setNextCursor] = useState(initial.nextCursor);
  const [state, setState] = useState(initial.state);
  const [loading, setLoading] = useState(false);

  async function loadOlder() {
    if (!nextCursor || loading) return;
    setLoading(true);
    try {
      const response = await fetch(
        "/api/accounts/" +
          encodeURIComponent(account.id) +
          "/sync-checkpoints?limit=6&cursor=" +
          encodeURIComponent(nextCursor),
        { cache: "no-store" },
      );
      if (!response.ok) {
        setState("UNAVAILABLE");
        return;
      }
      const page = projectAccountSyncHistory(
        await response.json(),
        account,
        new Date(),
      );
      const existingIDs = new Set(checkpoints.map((item) => item.id));
      const firstOlder = page.checkpoints[0];
      if (
        page.state === "UNAVAILABLE" ||
        page.checkpoints.some((item) => existingIDs.has(item.id)) ||
        (firstOlder &&
          checkpoints.length > 0 &&
          !isStrictlyOlder(checkpoints.at(-1)!, firstOlder))
      ) {
        setState("UNAVAILABLE");
        return;
      }
      setCheckpoints((current) => [...current, ...page.checkpoints]);
      setNextCursor(page.nextCursor);
    } catch {
      setState("UNAVAILABLE");
    } finally {
      setLoading(false);
    }
  }

  const newest = checkpoints[0];
  const newestAttempt = initialAttempts.attempts[0];
  const currentFailureCount = initialAttempts.attempts.filter(
    (attempt) =>
      attempt.outcome === "FAILED" &&
      !initialAttempts.attempts.some(
        (candidate) =>
          candidate.outcome === "SAVED" &&
          new Date(candidate.completedAt).valueOf() >
            new Date(attempt.completedAt).valueOf(),
      ),
  ).length;
  const recoveredFailureCount = initialAttempts.attempts.filter(
    (attempt) =>
      attempt.outcome === "FAILED" &&
      initialAttempts.attempts.some(
        (candidate) =>
          candidate.outcome === "SAVED" &&
          new Date(candidate.completedAt).valueOf() >
            new Date(attempt.completedAt).valueOf(),
      ),
  ).length;
  const attemptAttention =
    initialAttempts.state === "UNAVAILABLE" || currentFailureCount > 0;
  return (
    <section
      className={
        "account-sync-history is-" + state.toLowerCase().replaceAll("_", "-")
      }
      aria-labelledby="account-sync-history-title"
    >
      <header>
        <div>
          <p className="command-kicker">SAVED SYNC HISTORY</p>
          <h2 id="account-sync-history-title">
            {attemptAttention
              ? currentFailureCount > 0
                ? "The latest account sync failed closed."
                : "Saved sync attempt evidence needs review."
              : state === "CURRENT"
                ? "Arbion saved this account’s latest sync."
                : state === "FORWARD_COLLECTION_PENDING"
                  ? "Forward collection has not started yet."
                  : "Saved sync evidence needs review."}
          </h2>
          <p>
            Immutable account-discovery receipts collected only when an existing
            sync completes. This view does not contact the provider.
          </p>
        </div>
        <span>
          {attemptAttention
            ? "REVIEW"
            : state === "CURRENT"
              ? "SAVED"
              : "READ-ONLY"}
        </span>
      </header>

      {state === "UNAVAILABLE" ? (
        <div className="account-sync-history-message is-review" role="status">
          <strong>
            Arbion will not infer missing or inconsistent history.
          </strong>
          <p>
            The account and its trading controls are unchanged. Review the
            connection evidence; this panel cannot refresh or reconnect it.
          </p>
        </div>
      ) : state === "FORWARD_COLLECTION_PENDING" || !newest ? (
        <div className="account-sync-history-message" role="status">
          <strong>No saved checkpoint exists yet.</strong>
          <p>
            Arbion has not observed a normal account sync since forward
            collection was enabled. This does not mean{" "}
            {providerLabel(account.provider)} is disconnected or unhealthy.
          </p>
        </div>
      ) : (
        <>
          <div className="account-sync-history-current">
            <div>
              <small>LATEST SAVED CHECKPOINT</small>
              <strong>
                {providerLabel(newest.provider)} account inventory
              </strong>
              <span>{readableTime(newest.completedAt)}</span>
            </div>
            <dl>
              <div>
                <dt>Account</dt>
                <dd>{account.display_name}</dd>
              </div>
              <div>
                <dt>Save duration</dt>
                <dd>{readableDuration(newest.durationMilliseconds)}</dd>
              </div>
              <div>
                <dt>Accounts in operation</dt>
                <dd>{newest.accountCount}</dd>
              </div>
            </dl>
          </div>
          <details className="account-sync-history-evidence">
            <summary>
              Immutable identifiers and older history
              <span>{checkpoints.length} loaded</span>
            </summary>
            <ol>
              {checkpoints.map((checkpoint, index) => (
                <li key={checkpoint.id}>
                  <header>
                    <strong>
                      {index === 0 ? "Latest saved sync" : "Earlier saved sync"}
                    </strong>
                    <span>{readableTime(checkpoint.completedAt)}</span>
                  </header>
                  <dl>
                    <div>
                      <dt>Provider</dt>
                      <dd>{providerLabel(checkpoint.provider)}</dd>
                    </div>
                    <div>
                      <dt>Duration</dt>
                      <dd>
                        {readableDuration(checkpoint.durationMilliseconds)}
                      </dd>
                    </div>
                    <div>
                      <dt>Outcome</dt>
                      <dd>{checkpoint.outcome}</dd>
                    </div>
                    <div>
                      <dt>Source</dt>
                      <dd>{checkpoint.sourceOperation}</dd>
                    </div>
                    <div>
                      <dt>Account ID</dt>
                      <dd>{checkpoint.financialAccountID}</dd>
                    </div>
                    <div>
                      <dt>Connection ID</dt>
                      <dd>{checkpoint.providerConnectionID}</dd>
                    </div>
                    <div>
                      <dt>Checkpoint ID</dt>
                      <dd>{checkpoint.id}</dd>
                    </div>
                    <div>
                      <dt>Operation ID</dt>
                      <dd>{checkpoint.operationID}</dd>
                    </div>
                  </dl>
                </li>
              ))}
            </ol>
            {nextCursor && (
              <button type="button" onClick={loadOlder} disabled={loading}>
                {loading
                  ? "Loading saved history…"
                  : "Load older saved history"}
              </button>
            )}
          </details>
        </>
      )}

      <details
        className="account-sync-history-evidence"
        open={attemptAttention}
      >
        <summary>
          Sync attempts and recovery evidence
          <span>{initialAttempts.attempts.length} loaded</span>
        </summary>
        {initialAttempts.state === "UNAVAILABLE" ? (
          <div className="account-sync-history-message is-review" role="status">
            <strong>Attempt history is unavailable.</strong>
            <p>
              Arbion will not infer a failure, recovery, or provider cause from
              missing evidence. Existing account and runtime state is unchanged.
            </p>
          </div>
        ) : initialAttempts.state === "FORWARD_COLLECTION_PENDING" ||
          !newestAttempt ? (
          <div className="account-sync-history-message">
            <strong>Forward attempt collection has not started yet.</strong>
            <p>
              A future normal sync can add an immutable outcome. This view does
              not contact {providerLabel(account.provider)}.
            </p>
          </div>
        ) : (
          <>
            <p>
              {initialAttempts.attempts.length} immutable attempts ·{" "}
              {recoveredFailureCount} failures followed by a later saved success
              · {currentFailureCount} current failures. A later success proves
              recovery of this sync path, but does not identify who initiated it
              or why the earlier attempt failed.
            </p>
            <ol>
              {initialAttempts.attempts.map((attempt, index) => (
                <SyncAttemptEvidence
                  attempt={attempt}
                  index={index}
                  key={attempt.id}
                />
              ))}
            </ol>
          </>
        )}
      </details>

      <footer>
        Saved identity and timing only · No holdings · No provider refresh · No
        broker or live-trading path
      </footer>
    </section>
  );
}

function SyncAttemptEvidence({
  attempt,
  index,
}: {
  attempt: ConnectionSyncAttempt;
  index: number;
}) {
  return (
    <li>
      <header>
        <strong>
          {index === 0 ? "Latest sync attempt" : "Earlier sync attempt"}
        </strong>
        <span>{readableTime(attempt.completedAt)}</span>
      </header>
      <dl>
        <div>
          <dt>Outcome</dt>
          <dd>{attempt.outcome}</dd>
        </div>
        <div>
          <dt>Duration</dt>
          <dd>{readableDuration(attempt.durationMilliseconds)}</dd>
        </div>
        <div>
          <dt>Provider</dt>
          <dd>{providerLabel(attempt.provider)}</dd>
        </div>
        <div>
          <dt>Account count</dt>
          <dd>{attempt.accountCount ?? "NOT SAVED ON FAILURE"}</dd>
        </div>
        <div>
          <dt>Failure stage</dt>
          <dd>{attempt.failureStage ?? "NOT APPLICABLE"}</dd>
        </div>
        <div>
          <dt>Safe error code</dt>
          <dd>{attempt.errorCode ?? "NOT APPLICABLE"}</dd>
        </div>
        <div>
          <dt>Connection ID</dt>
          <dd>{attempt.providerConnectionID}</dd>
        </div>
        <div>
          <dt>Attempt ID</dt>
          <dd>{attempt.id}</dd>
        </div>
      </dl>
    </li>
  );
}
