import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountSyncHistoryPanel } from "./account-sync-history-panel";
import type {
  AccountSyncCheckpoint,
  SyncHistoryAccount,
} from "./account-sync-history";

const account: SyncHistoryAccount = {
  id: "c6f5c59c-aae3-4b29-812c-d3cf3b99e9ab",
  provider_connection_id: "3f6ab43c-8abd-4056-a858-ccb8051a045f",
  provider: "coinbase",
  display_name: "Coinbase Portfolio",
};

function saved(
  id: string,
  operationID: string,
  completedAt: string,
): AccountSyncCheckpoint {
  const completed = new Date(completedAt);
  return {
    id,
    operationID,
    financialAccountID: account.id,
    providerConnectionID: account.provider_connection_id,
    provider: "coinbase",
    sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
    outcome: "SAVED",
    accountCount: 1,
    observedAt: new Date(completed.valueOf() - 900).toISOString(),
    completedAt: completed.toISOString(),
    createdAt: completed.toISOString(),
    durationMilliseconds: 900,
  };
}

describe("AccountSyncHistoryPanel", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("explains the forward-only empty state without calling the provider", () => {
    render(
      <AccountSyncHistoryPanel
        account={account}
        initial={{
          state: "FORWARD_COLLECTION_PENDING",
          unauthorized: false,
          checkpoints: [],
        }}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "Forward collection has not started yet.",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/does not mean Coinbase is disconnected or unhealthy/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/No provider refresh · No broker or live-trading path/),
    ).toBeInTheDocument();
  });

  it("shows exact saved evidence and loads only older immutable rows", async () => {
    const current = saved(
      "33333333-3333-4333-8333-333333333333",
      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      "2026-09-02T22:30:00Z",
    );
    const older = saved(
      "22222222-2222-4222-8222-222222222222",
      "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      "2026-09-02T21:30:00Z",
    );
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            history: {
              checkpoints: [
                {
                  id: older.id,
                  operation_id: older.operationID,
                  financial_account_id: older.financialAccountID,
                  provider_connection_id: older.providerConnectionID,
                  provider: older.provider,
                  source_operation: older.sourceOperation,
                  outcome: older.outcome,
                  account_count: older.accountCount,
                  observed_at: older.observedAt,
                  completed_at: older.completedAt,
                  created_at: older.createdAt,
                },
              ],
            },
            history_semantics: "IMMUTABLE_FINANCIAL_ACCOUNT_SYNC_CHECKPOINTS",
            provider_read_performed: false,
            broker_action_available: false,
            live_execution_available: false,
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AccountSyncHistoryPanel
        account={account}
        initial={{
          state: "CURRENT",
          unauthorized: false,
          checkpoints: [current],
          nextCursor: current.id,
        }}
      />,
    );

    expect(screen.getByText("Coinbase account inventory")).toBeInTheDocument();
    expect(screen.getAllByText("900 ms")).toHaveLength(2);
    fireEvent.click(
      screen.getByRole("button", { name: "Load older saved history" }),
    );

    await waitFor(() =>
      expect(screen.getByText("Earlier saved sync")).toBeInTheDocument(),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/accounts/c6f5c59c-aae3-4b29-812c-d3cf3b99e9ab/sync-checkpoints?limit=6&cursor=33333333-3333-4333-8333-333333333333",
      { cache: "no-store" },
    );
  });

  it("automatically exposes a fail-closed evidence state", () => {
    render(
      <AccountSyncHistoryPanel
        account={account}
        initial={{
          state: "UNAVAILABLE",
          unauthorized: false,
          checkpoints: [],
        }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "will not infer missing or inconsistent history",
    );
  });
});
