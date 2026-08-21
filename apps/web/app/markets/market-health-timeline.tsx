"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import {
  safeMarketHealthHistory,
  type MarketHealthBucket,
  type MarketHealthHistory,
} from "./market-health-contract";
import type { MarketSource } from "./market-source-grid";

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function timestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Unknown time";
  return new Intl.DateTimeFormat("en-US", {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone: "UTC",
    timeZoneName: "short",
  }).format(parsed);
}

type TimelineRow = {
  key: string;
  source: string;
  capability: string;
  slots: Array<MarketHealthBucket | undefined>;
  lastObservedAt: string;
};

function timelineRows(
  history: MarketHealthHistory,
  sources: MarketSource[],
): TimelineRow[] {
  const start = Date.parse(history.window_started_at);
  const interval = history.interval_minutes * 60 * 1_000;
  const labels = new Map(sources.map((source) => [source.id, source.label]));
  const rows = new Map<string, TimelineRow>();
  for (const bucket of history.buckets) {
    const index = Math.floor(
      (Date.parse(bucket.interval_started_at) - start) / interval,
    );
    if (index < 0 || index >= history.window_hours) continue;
    const key = `${bucket.source_id}:${bucket.capability}`;
    const row = rows.get(key) ?? {
      key,
      source: labels.get(bucket.source_id) ?? readable(bucket.source_id),
      capability: readable(bucket.capability),
      slots: Array.from({ length: history.window_hours }),
      lastObservedAt: bucket.last_observed_at,
    };
    row.slots[index] = bucket;
    if (Date.parse(bucket.last_observed_at) > Date.parse(row.lastObservedAt)) {
      row.lastObservedAt = bucket.last_observed_at;
    }
    rows.set(key, row);
  }
  return [...rows.values()].sort(
    (left, right) =>
      left.source.localeCompare(right.source) ||
      left.capability.localeCompare(right.capability),
  );
}

function bucketClass(bucket: MarketHealthBucket | undefined) {
  if (!bucket) return "unobserved";
  if (bucket.failures === 0) return "successful";
  if (bucket.successes > 0) return "mixed";
  return "failed";
}

function bucketLabel(
  bucket: MarketHealthBucket | undefined,
  slotStart: number,
) {
  const period = timestamp(new Date(slotStart).toISOString());
  if (!bucket) return `${period}: no completed provider outcome recorded`;
  const category = bucket.failure_category
    ? `; latest safe category ${readable(bucket.failure_category)}`
    : "";
  return `${period}: ${bucket.successes} successful and ${bucket.failures} failed completed provider outcomes${category}`;
}

export function MarketHealthTimeline({
  sources,
  initialHistory,
}: {
  sources: MarketSource[];
  initialHistory?: MarketHealthHistory;
}) {
  const [history, setHistory] = useState(initialHistory);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState("");

  const refresh = useCallback(async () => {
    setRefreshing(true);
    setRefreshError("");
    try {
      const response = await fetch("/api/markets/source-history", {
        cache: "no-store",
      });
      const body = safeMarketHealthHistory(await response.json());
      if (!response.ok || !body) {
        setRefreshError(
          "Durable source history is temporarily unavailable. Current runtime status remains separate.",
        );
        return;
      }
      setHistory(body);
    } catch {
      setRefreshError(
        "Durable source history is temporarily unavailable. Current runtime status remains separate.",
      );
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => void refresh(), 5 * 60_000);
    return () => window.clearInterval(interval);
  }, [refresh]);

  const rows = useMemo(
    () => (history ? timelineRows(history, sources) : []),
    [history, sources],
  );
  const summary = useMemo(() => {
    const buckets = history?.buckets ?? [];
    const attempts = buckets.reduce(
      (total, bucket) => total + bucket.completed_attempts,
      0,
    );
    const successes = buckets.reduce(
      (total, bucket) => total + bucket.successes,
      0,
    );
    return {
      attempts,
      successes,
      failures: buckets.reduce((total, bucket) => total + bucket.failures, 0),
      capabilities: rows.length,
      successRate:
        attempts === 0
          ? "No outcomes"
          : `${Math.round((successes / attempts) * 100)}%`,
    };
  }, [history, rows.length]);
  const start = history ? Date.parse(history.window_started_at) : 0;

  return (
    <section
      className="market-health-history"
      aria-label="Source health history"
    >
      <header>
        <div>
          <p className="eyebrow">24-HOUR SOURCE MEMORY</p>
          <h3>Provider outcomes that survive a restart.</h3>
          <p>
            Five-minute records roll into an hourly view. It contains no user,
            account, symbol, request, credential, or raw provider-error data.
          </p>
        </div>
        <button
          className="secondary"
          disabled={refreshing}
          onClick={() => void refresh()}
          type="button"
        >
          {refreshing ? "Refreshing…" : "Refresh 24-hour history"}
        </button>
      </header>

      {history ? (
        <>
          <div
            className="market-health-summary"
            aria-label="24-hour health summary"
          >
            <span>
              <strong>{summary.capabilities}</strong> observed capabilities
            </span>
            <span>
              <strong>{summary.attempts}</strong> completed outcomes
            </span>
            <span>
              <strong>{summary.successRate}</strong> successful
            </span>
            <span className={summary.failures > 0 ? "degraded" : ""}>
              <strong>{summary.failures}</strong> failed
            </span>
          </div>
          {rows.length > 0 ? (
            <div className="market-health-timelines">
              {rows.map((row) => (
                <article key={row.key}>
                  <div className="market-health-row-label">
                    <strong>{row.source}</strong>
                    <span>{row.capability}</span>
                    <small>Last outcome {timestamp(row.lastObservedAt)}</small>
                  </div>
                  <div
                    className="market-health-strip"
                    aria-label={`${row.source} ${row.capability} hourly provider outcomes`}
                  >
                    {row.slots.map((bucket, index) => (
                      <span
                        aria-label={bucketLabel(
                          bucket,
                          start + index * 60 * 60 * 1_000,
                        )}
                        className={bucketClass(bucket)}
                        key={`${row.key}:${index}`}
                        role="img"
                        title={bucketLabel(
                          bucket,
                          start + index * 60 * 60 * 1_000,
                        )}
                      />
                    ))}
                  </div>
                </article>
              ))}
              <div
                className="market-health-legend"
                aria-label="Health history legend"
              >
                <span className="successful">Successful</span>
                <span className="mixed">Mixed</span>
                <span className="failed">Failed</span>
                <span className="unobserved">No outcome</span>
              </div>
            </div>
          ) : (
            <p className="market-health-empty">
              No completed provider outcomes have been recorded in this window.
              History begins with this production capability; nothing is
              backfilled or inferred.
            </p>
          )}
          <footer>
            Empty hours are neutral. They mean no provider outcome was recorded,
            not that a source was down. This history is operational evidence,
            never an executable quote or provider quota.
          </footer>
        </>
      ) : (
        <p className="market-health-empty">
          Durable source history is not available yet. Current process status
          remains visible below and no historical state is invented.
        </p>
      )}
      {refreshError && (
        <p className="unavailable" role="status">
          {refreshError}
        </p>
      )}
    </section>
  );
}
