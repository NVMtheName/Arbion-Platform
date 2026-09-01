import Link from "next/link";

import type { PaperAutonomyEvidenceThresholdChangeLedger } from "./paper-portfolio-summary";

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function readableTime(value: string) {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(new Date(value));
}

function readableDuration(seconds: number) {
  const hours = Math.floor(seconds / 3600);
  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return days > 0 ? days + "d " + remainingHours + "h" : hours + "h";
}

export function PaperEvidenceThresholdChangeLedger({
  ledger,
  detailHref,
}: {
  ledger: PaperAutonomyEvidenceThresholdChangeLedger;
  detailHref?: string;
}) {
  const available =
    ledger.status === "AVAILABLE" && ledger.checkpoints.length > 0;
  const latest = available ? ledger.checkpoints.at(-1) : undefined;
  const attention =
    !available ||
    latest?.evidence_status === "REVIEW_REQUIRED" ||
    latest?.evidence_status === "UNAVAILABLE";
  return (
    <details
      className={
        "paper-evidence-threshold-ledger" + (attention ? " is-attention" : "")
      }
      open={attention}
    >
      <summary>
        <span>
          <strong>Evidence threshold change ledger</strong>
          <small>Consecutive immutable scheduler checkpoints</small>
        </span>
        <span>
          {!latest
            ? "Evidence unavailable"
            : latest.decision_count +
              " decisions · " +
              latest.evidence_window_hours +
              "/168h"}
        </span>
      </summary>
      {!latest ? (
        <div className="strategy-fleet-exposure-unavailable">
          <strong>Exact checkpoint history is unavailable</strong>
          <p>
            Arbion does not infer progress, regressions, blocker changes, or
            readiness from incomplete immutable scheduler evidence.
          </p>
        </div>
      ) : (
        <div>
          <p>
            {latest.progress_classification === "NORMAL_COLLECTION"
              ? "The evidence window is progressing normally."
              : latest.progress_classification === "RECOVERED"
                ? "The latest saved checkpoint recovered automatically."
                : latest.progress_classification === "REVIEW_REGRESSION"
                  ? "The latest saved checkpoint introduced evidence that requires review."
                  : latest.progress_classification === "CONTEXT_CHANGED"
                    ? "The saved route or input context changed and remains explicitly attributable."
                    : "The newest saved checkpoint held its prior evidence state."}{" "}
            This history describes evidence collection only; it does not score
            performance, authorize promotion, or enable live execution.
          </p>
          <ol className="paper-evidence-threshold-checkpoints">
            {ledger.checkpoints.map((checkpoint) => (
              <li key={checkpoint.schedule_run_id}>
                <header>
                  <span>{readable(checkpoint.progress_classification)}</span>
                  <strong>{readableTime(checkpoint.as_of)} UTC</strong>
                </header>
                <dl>
                  <div>
                    <dt>Threshold progress</dt>
                    <dd>
                      {checkpoint.decision_count} decisions ·{" "}
                      {checkpoint.evidence_window_hours}/168 hours
                    </dd>
                    <small>
                      {checkpoint.decision_delta >= 0 ? "+" : ""}
                      {checkpoint.decision_delta} decision change ·{" "}
                      {checkpoint.remaining_seconds > 0
                        ? readableDuration(checkpoint.remaining_seconds) +
                          " remaining"
                        : "time threshold reached"}
                    </small>
                  </div>
                  <div>
                    <dt>Route + input evidence</dt>
                    <dd>
                      {readable(checkpoint.route_continuity_status)} ·{" "}
                      {readable(checkpoint.input_coverage_status)}
                    </dd>
                    <small>
                      Route {readable(checkpoint.route_continuity_change)} ·
                      input {readable(checkpoint.input_coverage_change)} ·{" "}
                      {readable(checkpoint.input_freshness_status)}
                    </small>
                  </div>
                  <div>
                    <dt>Automatic scheduler</dt>
                    <dd>
                      {readable(checkpoint.scheduler_status)} ·{" "}
                      {readable(checkpoint.scheduler_change)}
                    </dd>
                    <small>
                      {checkpoint.consecutive_failures} consecutive failures
                    </small>
                  </div>
                  <div>
                    <dt>Blocker change</dt>
                    <dd>
                      {checkpoint.added_blocker_codes.length} added ·{" "}
                      {checkpoint.resolved_blocker_codes.length} resolved
                    </dd>
                    <small>
                      {checkpoint.added_blocker_codes.length > 0
                        ? "Added: " +
                          checkpoint.added_blocker_codes
                            .map(readable)
                            .join(", ")
                        : checkpoint.resolved_blocker_codes.length > 0
                          ? "Resolved: " +
                            checkpoint.resolved_blocker_codes
                              .map(readable)
                              .join(", ")
                          : "No blocker change"}
                    </small>
                  </div>
                </dl>
                <p>
                  {checkpoint.routes.length > 0
                    ? checkpoint.routes
                        .map(
                          (route) =>
                            route.ai_provider +
                            " / " +
                            route.model_id +
                            " / " +
                            route.profile +
                            " · " +
                            route.financial_provider,
                        )
                        .join("; ")
                    : "Route attribution unavailable at this checkpoint"}
                </p>
                <code>Scheduler run {checkpoint.schedule_run_id}</code>
              </li>
            ))}
          </ol>
          <footer>
            {detailHref ? (
              <Link href={detailHref}>Open complete Paper evidence →</Link>
            ) : null}
            <Link href="/activity">Open immutable Decision Journal →</Link>
            <span>
              {ledger.checkpoint_count} shown of {ledger.source_run_count} saved
              scheduler rows{ledger.capped ? " · latest bounded view" : ""}
            </span>
          </footer>
        </div>
      )}
    </details>
  );
}
