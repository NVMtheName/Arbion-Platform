import { afterEach, describe, expect, it, vi } from "vitest";

import {
  loadConnectionSyncAttemptHistory,
  projectConnectionSyncAttemptHistory,
} from "./connection-sync-attempt-history";

const account = {
  id: "00000000-0000-4000-8000-000000000001",
  provider_connection_id: "00000000-0000-4000-8000-000000000002",
  provider: "coinbase",
};
const viewedAt = new Date("2026-09-07T12:00:00Z");

function payload(attempts: Array<Record<string, unknown>>) {
  return {
    history_semantics: "IMMUTABLE_FINANCIAL_CONNECTION_SYNC_ATTEMPTS",
    provider_read_performed: false,
    broker_action_available: false,
    live_execution_available: false,
    history: { attempts },
  };
}

function success() {
  return {
    id: "00000000-0000-4000-8000-000000000003",
    provider_connection_id: account.provider_connection_id,
    provider: "coinbase",
    source_operation: "PROVIDER_ACCOUNT_DISCOVERY",
    outcome: "SAVED",
    account_count: 1,
    observed_at: "2026-09-07T11:30:00Z",
    completed_at: "2026-09-07T11:30:02Z",
    created_at: "2026-09-07T11:30:02Z",
  };
}

function failure() {
  return {
    id: "00000000-0000-4000-8000-000000000004",
    provider_connection_id: account.provider_connection_id,
    provider: "coinbase",
    source_operation: "PROVIDER_ACCOUNT_DISCOVERY",
    outcome: "FAILED",
    failure_stage: "ACCOUNT_DISCOVERY",
    error_code: "RATE_LIMITED",
    observed_at: "2026-09-07T10:30:00Z",
    completed_at: "2026-09-07T10:30:03Z",
    created_at: "2026-09-07T10:30:03Z",
  };
}

describe("connection sync attempt history", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("accepts an exact saved recovery after a credential-free failure", () => {
    const result = projectConnectionSyncAttemptHistory(
      payload([success(), failure()]),
      account,
      viewedAt,
    );
    expect(result.state).toBe("CURRENT");
    expect(result.attempts).toMatchObject([
      { outcome: "SAVED", accountCount: 1, durationMilliseconds: 2000 },
      {
        outcome: "FAILED",
        failureStage: "ACCOUNT_DISCOVERY",
        errorCode: "RATE_LIMITED",
        durationMilliseconds: 3000,
      },
    ]);
  });

  it("fails closed on raw error fields or ambiguous success and failure facts", () => {
    expect(
      projectConnectionSyncAttemptHistory(
        payload([
          { ...failure(), raw_error: "provider secret", account_count: 1 },
        ]),
        account,
        viewedAt,
      ).state,
    ).toBe("UNAVAILABLE");
    expect(
      projectConnectionSyncAttemptHistory(
        payload([{ ...success(), error_code: "RATE_LIMITED" }]),
        account,
        viewedAt,
      ).state,
    ).toBe("UNAVAILABLE");
  });

  it("loads only saved evidence and handles authorization separately", async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => payload([success()]),
    });
    vi.stubGlobal("fetch", fetcher);
    const result = await loadConnectionSyncAttemptHistory({
      account,
      base: "http://arbion-api",
      headers: { cookie: "session=safe" },
      viewedAt,
    });
    expect(result.state).toBe("CURRENT");
    expect(fetcher).toHaveBeenCalledWith(
      `http://arbion-api/api/accounts/${account.id}/sync-attempts?limit=12`,
      { headers: { cookie: "session=safe" }, cache: "no-store" },
    );

    fetcher.mockResolvedValue({ ok: false, status: 401 });
    expect(
      await loadConnectionSyncAttemptHistory({
        account,
        base: "http://arbion-api",
        headers: { cookie: "session=safe" },
        viewedAt,
      }),
    ).toMatchObject({ state: "UNAVAILABLE", unauthorized: true });
  });
});
