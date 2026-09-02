import { afterEach, describe, expect, it, vi } from "vitest";

import {
  loadAccountSyncHistory,
  projectAccountSyncHistory,
  type SyncHistoryAccount,
} from "./account-sync-history";

const account: SyncHistoryAccount = {
  id: "c6f5c59c-aae3-4b29-812c-d3cf3b99e9ab",
  provider_connection_id: "3f6ab43c-8abd-4056-a858-ccb8051a045f",
  provider: "coinbase",
  display_name: "Coinbase Portfolio",
};
const viewedAt = new Date("2026-09-02T23:00:00Z");

function checkpoint(id: string, operationID: string, completedAt: string) {
  const completed = new Date(completedAt);
  return {
    id,
    operation_id: operationID,
    financial_account_id: account.id,
    provider_connection_id: account.provider_connection_id,
    provider: account.provider,
    source_operation: "PROVIDER_ACCOUNT_DISCOVERY",
    outcome: "SAVED",
    account_count: 1,
    observed_at: new Date(completed.valueOf() - 1250).toISOString(),
    completed_at: completed.toISOString(),
    created_at: completed.toISOString(),
  };
}

function payload(checkpoints: unknown[], nextCursor?: string) {
  return {
    history: {
      checkpoints,
      ...(nextCursor ? { next_cursor: nextCursor } : {}),
    },
    history_semantics: "IMMUTABLE_FINANCIAL_ACCOUNT_SYNC_CHECKPOINTS",
    provider_read_performed: false,
    broker_action_available: false,
    live_execution_available: false,
  };
}

describe("Account saved sync history", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("projects bounded newest-first immutable account evidence", () => {
    const firstID = "33333333-3333-4333-8333-333333333333";
    const secondID = "22222222-2222-4222-8222-222222222222";
    const result = projectAccountSyncHistory(
      payload(
        [
          checkpoint(
            firstID,
            "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "2026-09-02T22:30:00Z",
          ),
          checkpoint(
            secondID,
            "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
            "2026-09-02T21:30:00Z",
          ),
        ],
        secondID,
      ),
      account,
      viewedAt,
    );

    expect(result).toMatchObject({
      state: "CURRENT",
      unauthorized: false,
      nextCursor: secondID,
      checkpoints: [
        {
          id: firstID,
          provider: "coinbase",
          outcome: "SAVED",
          accountCount: 1,
          durationMilliseconds: 1250,
        },
        { id: secondID },
      ],
    });
  });

  it("distinguishes an empty forward-only window from connection health", () => {
    expect(projectAccountSyncHistory(payload([]), account, viewedAt)).toEqual({
      state: "FORWARD_COLLECTION_PENDING",
      unauthorized: false,
      checkpoints: [],
    });
  });

  it.each([
    [
      "provider read",
      {
        ...payload([]),
        provider_read_performed: true,
      },
    ],
    [
      "wrong account",
      payload([
        {
          ...checkpoint(
            "33333333-3333-4333-8333-333333333333",
            "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "2026-09-02T22:30:00Z",
          ),
          financial_account_id: "44444444-4444-4444-8444-444444444444",
        },
      ]),
    ],
    [
      "wrong provider",
      payload([
        {
          ...checkpoint(
            "33333333-3333-4333-8333-333333333333",
            "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "2026-09-02T22:30:00Z",
          ),
          provider: "schwab",
        },
      ]),
    ],
    [
      "non-saved outcome",
      payload([
        {
          ...checkpoint(
            "33333333-3333-4333-8333-333333333333",
            "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            "2026-09-02T22:30:00Z",
          ),
          outcome: "FAILED",
        },
      ]),
    ],
    [
      "future evidence",
      payload([
        checkpoint(
          "33333333-3333-4333-8333-333333333333",
          "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
          "2026-09-03T00:30:00Z",
        ),
      ]),
    ],
    [
      "out-of-order evidence",
      payload([
        checkpoint(
          "33333333-3333-4333-8333-333333333333",
          "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
          "2026-09-02T21:30:00Z",
        ),
        checkpoint(
          "22222222-2222-4222-8222-222222222222",
          "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
          "2026-09-02T22:30:00Z",
        ),
      ]),
    ],
  ])("fails closed on %s", (_label, input) => {
    expect(projectAccountSyncHistory(input, account, viewedAt)).toEqual({
      state: "UNAVAILABLE",
      unauthorized: false,
      checkpoints: [],
    });
  });

  it("loads only the saved account endpoint and propagates authentication", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(payload([])), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);

    const first = await loadAccountSyncHistory({
      account,
      base: "http://arbion-api",
      headers: { cookie: "session=owner" },
      viewedAt,
    });
    expect(first.state).toBe("FORWARD_COLLECTION_PENDING");
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "http://arbion-api/api/accounts/c6f5c59c-aae3-4b29-812c-d3cf3b99e9ab/sync-checkpoints?limit=6",
      { headers: { cookie: "session=owner" }, cache: "no-store" },
    );

    const second = await loadAccountSyncHistory({
      account,
      base: "http://arbion-api",
      headers: { cookie: "session=owner" },
      viewedAt,
    });
    expect(second).toEqual({
      state: "UNAVAILABLE",
      unauthorized: true,
      checkpoints: [],
    });
  });
});
