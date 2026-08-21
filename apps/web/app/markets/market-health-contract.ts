export type MarketHealthBucket = {
  source_id: string;
  capability: string;
  interval_started_at: string;
  last_observed_at: string;
  completed_attempts: number;
  successes: number;
  failures: number;
  last_state: "VERIFIED" | "DEGRADED";
  failure_category?: string;
};

export type MarketHealthHistory = {
  buckets: MarketHealthBucket[];
  window_started_at: string;
  window_ended_at: string;
  window_hours: 24;
  interval_minutes: 60;
};

type HealthHistoryResponse = Partial<MarketHealthHistory> & {
  history_semantics?: string;
  subject_dimensions_exposed?: boolean;
  raw_provider_errors_exposed?: boolean;
  live_execution_available?: boolean;
};

const historySemantics =
  "DURABLE_PROVIDER_OUTCOMES_5_MINUTE_STORAGE_HOURLY_VIEW";
const safeFailureCategories = new Set([
  "TIMEOUT",
  "STALE_DATA",
  "FUTURE_DATED_DATA",
  "MISSING_PROVENANCE",
  "INVALID_DATA",
  "UPSTREAM_FAILURE",
]);
const responseKeys = new Set([
  "buckets",
  "window_started_at",
  "window_ended_at",
  "window_hours",
  "interval_minutes",
  "history_semantics",
  "subject_dimensions_exposed",
  "raw_provider_errors_exposed",
  "live_execution_available",
]);
const bucketKeys = new Set([
  "source_id",
  "capability",
  "interval_started_at",
  "last_observed_at",
  "completed_attempts",
  "successes",
  "failures",
  "last_state",
  "failure_category",
]);

function validBucket(value: unknown): value is MarketHealthBucket {
  if (!value || typeof value !== "object") return false;
  const bucket = value as Record<string, unknown>;
  if (!Object.keys(bucket).every((key) => bucketKeys.has(key))) return false;
  if (
    typeof bucket.source_id !== "string" ||
    bucket.source_id.length < 1 ||
    bucket.source_id.length > 80 ||
    typeof bucket.capability !== "string" ||
    bucket.capability.length < 1 ||
    bucket.capability.length > 80 ||
    typeof bucket.interval_started_at !== "string" ||
    !Number.isFinite(Date.parse(bucket.interval_started_at)) ||
    typeof bucket.last_observed_at !== "string" ||
    !Number.isFinite(Date.parse(bucket.last_observed_at)) ||
    !Number.isSafeInteger(bucket.completed_attempts) ||
    !Number.isSafeInteger(bucket.successes) ||
    !Number.isSafeInteger(bucket.failures) ||
    (bucket.last_state !== "VERIFIED" && bucket.last_state !== "DEGRADED")
  ) {
    return false;
  }
  const attempts = bucket.completed_attempts as number;
  const successes = bucket.successes as number;
  const failures = bucket.failures as number;
  if (
    attempts < 1 ||
    successes < 0 ||
    failures < 0 ||
    attempts !== successes + failures
  ) {
    return false;
  }
  if (
    bucket.failure_category !== undefined &&
    (typeof bucket.failure_category !== "string" ||
      !safeFailureCategories.has(bucket.failure_category))
  ) {
    return false;
  }
  return (
    (bucket.last_state === "VERIFIED" &&
      bucket.failure_category === undefined) ||
    (bucket.last_state === "DEGRADED" &&
      typeof bucket.failure_category === "string")
  );
}

export function safeMarketHealthHistory(
  value: unknown,
): MarketHealthHistory | undefined {
  if (!value || typeof value !== "object") return undefined;
  const record = value as Record<string, unknown>;
  if (!Object.keys(record).every((key) => responseKeys.has(key))) {
    return undefined;
  }
  const response = value as HealthHistoryResponse;
  if (
    response.history_semantics !== historySemantics ||
    response.subject_dimensions_exposed !== false ||
    response.raw_provider_errors_exposed !== false ||
    response.live_execution_available !== false ||
    response.window_hours !== 24 ||
    response.interval_minutes !== 60 ||
    typeof response.window_started_at !== "string" ||
    typeof response.window_ended_at !== "string" ||
    !Array.isArray(response.buckets) ||
    !response.buckets.every(validBucket)
  ) {
    return undefined;
  }
  const start = Date.parse(response.window_started_at);
  const end = Date.parse(response.window_ended_at);
  if (!Number.isFinite(start) || end - start !== 24 * 60 * 60 * 1_000) {
    return undefined;
  }
  if (
    !response.buckets.every((bucket) => {
      const interval = Date.parse(bucket.interval_started_at);
      const observed = Date.parse(bucket.last_observed_at);
      return (
        interval >= start &&
        interval < end &&
        observed >= interval &&
        observed < interval + 60 * 60 * 1_000
      );
    })
  ) {
    return undefined;
  }
  return {
    buckets: response.buckets,
    window_started_at: response.window_started_at,
    window_ended_at: response.window_ended_at,
    window_hours: 24,
    interval_minutes: 60,
  };
}
