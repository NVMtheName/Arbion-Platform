export type ScheduleFailureAction =
  | "AUTOMATIC_RETRY"
  | "OWNER_REVIEW"
  | "OPERATOR_REVIEW"
  | "NO_ACTION";

export type ScheduleFailureGuidance = {
  action: ScheduleFailureAction;
  actionLabel: string;
  message: string;
};

function financialProviderLabel(provider?: string) {
  if (provider === "coinbase") return "Coinbase";
  if (provider === "schwab") return "Schwab";
  return "The financial provider";
}

function guidance(
  action: ScheduleFailureAction,
  message: string,
): ScheduleFailureGuidance {
  const actionLabel = {
    AUTOMATIC_RETRY: "Automatic retry",
    OWNER_REVIEW: "Owner review",
    OPERATOR_REVIEW: "Operator correction",
    NO_ACTION: "No action needed",
  }[action];
  return { action, actionLabel, message };
}

// Maps the scheduler's fixed safe-code allowlist to bounded recovery guidance.
// It never renders raw provider text or changes schedule/execution state.
export function scheduleFailureGuidance(
  code: string,
  financialProvider?: string,
): ScheduleFailureGuidance {
  const provider = financialProviderLabel(financialProvider);
  switch (code) {
    case "COMMIT_ACTION_LIMIT_REACHED":
    case "COMMIT_ACTION_COOLDOWN_ACTIVE":
      return guidance(
        "AUTOMATIC_RETRY",
        "The final save check found that this engine's daily action quota or same-symbol, same-side cooldown did not permit the prepared action. This attempt saved no simulated fill or would-have-submitted action, and no broker order was sent. The next normal scheduled cycle will evaluate current limits; do not rerun the old proposal or increase limits to bypass this hold.",
      );
    case "COMMIT_ACTIVITY_UNAVAILABLE":
      return guidance(
        "OPERATOR_REVIEW",
        "The final save check could not verify current action activity for the prepared evaluation's UTC day. This attempt saved no simulated fill or would-have-submitted action, and no broker order was sent. Review saved activity and timing evidence; do not replay the old proposal or weaken the limits.",
      );
    case "COMMIT_MANDATE_WINDOW_CLOSED":
      return guidance(
        "OWNER_REVIEW",
        "The approved mandate version pinned to this engine was outside its effective window at the final save check. This attempt saved no simulated fill or would-have-submitted action, and no broker order was sent. Review the engine's approved start and end times; a newer draft does not extend its authorization. Do not rerun the old proposal or automatically extend the window.",
      );
    case "COMMIT_MARKET_DATA_STALE":
      return guidance(
        "AUTOMATIC_RETRY",
        "The saved AI quote was outside the permitted time window at the final save check. This attempt saved no simulated fill or would-have-submitted action, and no broker order was sent. The normal schedule can obtain new evidence on its next cycle; do not rerun the old proposal or loosen the quote limits.",
      );
    case "COMMIT_ACCESS_REVOKED":
      return guidance(
        "OWNER_REVIEW",
        "Current owner or automation access did not permit this non-live commit. This attempt saved no simulated fill or would-have-submitted action. Review account access with an administrator; logging in again does not restore revoked permissions. No broker order was sent.",
      );
    case "COMMIT_CONNECTION_UNAVAILABLE":
      return guidance(
        "OWNER_REVIEW",
        "The saved financial account or the provider connection pinned to this engine was unavailable at commit. Review Connections and the engine's saved binding; do not re-enable an intentionally disabled connection. This attempt saved no simulated fill or would-have-submitted action, and no broker order was sent.",
      );
    case "CIRCUIT_BREAKER_ACTIVE":
      return guidance(
        "OWNER_REVIEW",
        "An emergency stop blocked this non-live commit. This attempt saved no simulated fill or would-have-submitted action. Review the active automation, account, user, or platform stop; an intentional stop should remain engaged. No broker order was sent.",
      );
    case "AUTHORIZATION_FAILED":
    case "AUTHORIZATION_EXPIRED":
      return guidance(
        "OWNER_REVIEW",
        `Reconnect ${provider} before the next scheduled evaluation.`,
      );
    case "PROVIDER":
      return guidance(
        "OPERATOR_REVIEW",
        `${provider} read data was unavailable, but this saved result does not identify why. Review connection status and saved evidence; it does not prove a temporary outage or a quote-entitlement problem.`,
      );
    case "INVALID_CREDENTIAL_FORMAT":
      return guidance(
        "OWNER_REVIEW",
        `Review the saved ${provider} connection setup. Its credential format was rejected; do not paste credentials into the AI conversation.`,
      );
    case "ACCOUNT_NOT_FOUND":
      return guidance(
        "OWNER_REVIEW",
        `${provider} could not find the linked account. Review the account and connection before the next evaluation; no holdings or allocation were changed.`,
      );
    case "PERMISSION_DENIED":
      return guidance(
        "OWNER_REVIEW",
        `${provider} denied the requested read access. Review the connection's account and API permissions; this result alone does not establish quote entitlement.`,
      );
    case "INVALID_PROVIDER_RESPONSE":
      return guidance(
        "OPERATOR_REVIEW",
        `${provider} returned evidence that did not satisfy the read contract. Arbion stopped safely; the provider response or adapter needs review.`,
      );
    case "PROVIDER_UNAVAILABLE":
    case "RATE_LIMITED":
    case "TIMEOUT":
      return guidance(
        "AUTOMATIC_RETRY",
        `${provider} read data was unavailable. Arbion remains fail-closed and will try again at the next eligible run.`,
      );
    case "MARKET_DATA_STALE":
      return guidance(
        "AUTOMATIC_RETRY",
        `${provider} market evidence was not current. Arbion will try again after the data refreshes.`,
      );
    case "MARKET_DATA_DELAYED":
      return guidance(
        "OWNER_REVIEW",
        `${provider} explicitly marked the saved quote as delayed. Arbion stopped before the AI model and will not use it. Review real-time market-data access; reconnecting the account alone does not change quote entitlement.`,
      );
    case "MARKET_DATA_REALTIME_UNCONFIRMED":
      return guidance(
        "OWNER_REVIEW",
        `${provider} did not include a real-time status for the saved quote. Arbion stopped before the AI model rather than guessing. Review the provider app's market-data product and entitlement before the next cycle.`,
      );
    case "MARKET_DATA_NOT_REALTIME":
      return guidance(
        "OWNER_REVIEW",
        `${provider} did not explicitly mark the saved quote as real-time. Arbion stopped before the AI model and will not use delayed or ambiguous market prices. Review the provider connection and market-data access before the next cycle.`,
      );
    case "MARKET_DATA_INVALID":
      return guidance(
        "OPERATOR_REVIEW",
        `${provider} returned malformed or unusable market prices. Arbion stopped before the AI model and will not use negative, negative-zero, zero-only, crossed, mismatched, or one-sided quote evidence. The provider response or adapter must be corrected before the next cycle.`,
      );
    case "NO_ELIGIBLE_OPTION_CONTRACTS":
      return guidance(
        "OWNER_REVIEW",
        "No option contract matched the saved expiration, delta, and premium filters. Review those filters if this repeats.",
      );
    case "STRATEGY_NOT_ACTIVE":
      return guidance(
        "OWNER_REVIEW",
        "Resume the non-live strategy if you want scheduled evaluations to continue.",
      );
    case "STRATEGY_CONFIGURATION_CHANGED":
      return guidance(
        "OWNER_REVIEW",
        "The initialized strategy no longer matches its current mandate, capital bucket, or account. Review the automation before continuing.",
      );
    case "STRATEGY_PARAMETERS_INVALID":
      return guidance(
        "OWNER_REVIEW",
        "Review and save valid deterministic strategy parameters before continuing.",
      );
    case "PAPER_STATE_UNAVAILABLE":
      return guidance(
        "OPERATOR_REVIEW",
        "The isolated Paper portfolio needed for evaluation is unavailable. Arbion cannot evaluate until that state is repaired.",
      );
    case "WAITING_FOR_LIFECYCLE":
      return guidance(
        "OWNER_REVIEW",
        "Record the explicit Paper option lifecycle outcome before another evaluation can run.",
      );
    case "OUTSIDE_SESSION":
      return guidance(
        "NO_ACTION",
        "Arbion will wait for the next supported U.S. equities session.",
      );
    case "SESSION_CALENDAR_UNAVAILABLE":
      return guidance(
        "OPERATOR_REVIEW",
        "The verified market-session calendar does not cover this date. The operator must extend it before scheduling resumes.",
      );
    case "RECONCILIATION_REFRESH_FAILED":
      return guidance(
        "AUTOMATIC_RETRY",
        `${provider} inventory evidence could not be refreshed. Arbion held the AI cycle and will retry without clearing prior evidence.`,
      );
    case "AI_DECISION_BUDGET_EXHAUSTED":
      return guidance(
        "AUTOMATIC_RETRY",
        "The bounded AI decision window was exhausted. Arbion will retry after capacity becomes available.",
      );
    case "AI_PROVIDER_RATE_LIMITED":
      return guidance(
        "AUTOMATIC_RETRY",
        "The selected AI provider temporarily limited requests. Arbion will retry at the next scheduled cycle.",
      );
    case "AI_PROVIDER_UNAVAILABLE":
      return guidance(
        "AUTOMATIC_RETRY",
        "The selected AI provider was unavailable or timed out. Arbion will retry at the next scheduled cycle.",
      );
    case "AI_CONNECTION_UNAVAILABLE":
      return guidance(
        "OWNER_REVIEW",
        "The selected AI connection is inactive, disabled, or no longer authorized. Review that connection before the next cycle.",
      );
    case "AI_REQUEST_INVALID":
      return guidance(
        "OPERATOR_REVIEW",
        "The AI request or response failed Arbion's strict schema contract. It was rejected safely and requires an application correction.",
      );
    case "INVALID":
      return guidance(
        "OPERATOR_REVIEW",
        "Runtime evidence failed Arbion's validation contract. The cycle was rejected before any non-live action was recorded.",
      );
    case "UNSUPPORTED_MODE":
    case "UNSUPPORTED_SESSION":
      return guidance(
        "OPERATOR_REVIEW",
        "The saved runtime mode or session is unsupported. Scheduling remains fail-closed until the configuration is corrected.",
      );
    case "CANCELED":
      return guidance(
        "AUTOMATIC_RETRY",
        "The run ended before completion. Arbion can try again at the next eligible cycle.",
      );
    case "CONFLICT":
      return guidance(
        "OWNER_REVIEW",
        "The strategy changed during the run. Refresh the automation and review its latest state.",
      );
    case "FORBIDDEN":
    case "NOT_FOUND":
      return guidance(
        "OWNER_REVIEW",
        "The schedule can no longer access its required owner-scoped records. Review the automation before continuing.",
      );
    default:
      return guidance(
        "OPERATOR_REVIEW",
        "The run failed closed with a bounded internal status. Arbion needs an operator review before repeated failures are ignored.",
      );
  }
}
