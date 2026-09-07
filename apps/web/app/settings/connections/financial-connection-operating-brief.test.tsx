import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { ConnectionSyncEvidenceInput } from "./connection-sync-evidence-center";
import {
  FinancialConnectionOperatingWorkspace,
  projectFinancialConnectionOperatingBrief,
} from "./financial-connection-operating-brief";
import type { FinancialContinuityEngine } from "./financial-continuity-center";
import type { FinancialAccount, FinancialConnection } from "./page";

const ids = {
  connection: "10000000-0000-4000-8000-000000000001",
  account: "10000000-0000-4000-8000-000000000002",
  instance: "10000000-0000-4000-8000-000000000003",
  mandate: "10000000-0000-4000-8000-000000000004",
  bucket: "10000000-0000-4000-8000-000000000005",
  run: "10000000-0000-4000-8000-000000000006",
  checkpoint: "10000000-0000-4000-8000-000000000007",
  operation: "10000000-0000-4000-8000-000000000008",
  reconciliation: "10000000-0000-4000-8000-000000000009",
};

const observedAt = "2026-09-07T08:00:00Z";

const connection: FinancialConnection = {
  id: ids.connection,
  provider: "coinbase",
  display_name: "Coinbase Production",
  status: "active",
  runtime_protected: true,
  protected_mandate_count: 1,
  active_strategy_count: 1,
  last_synced_at: "2026-09-07T07:40:00Z",
};

const account: FinancialAccount = {
  id: ids.account,
  provider_connection_id: ids.connection,
  provider: "coinbase",
  display_name: "Coinbase Portfolio",
  status: "active",
  last_synced_at: "2026-09-07T07:42:00Z",
};

const engine: FinancialContinuityEngine = {
  mandate_id: ids.mandate,
  instance_id: ids.instance,
  connection_id: ids.connection,
  account_id: ids.account,
  account_name: account.display_name,
  provider: "coinbase",
  execution_mode: "PAPER",
  instance_status: "ACTIVE",
  current_state: "AI_MONITORING",
  schedule_available: true,
  schedule_history_available: true,
  schedule_status: "SUCCEEDED",
  schedule_completed_at: "2026-09-07T07:45:10Z",
  schedule_next_run_at: "2026-09-07T08:45:00Z",
  schedule_timing_status: "ON_SCHEDULE",
  consecutive_failures: 0,
  recent_runs: [
    {
      id: ids.run,
      scheduled_for: "2026-09-07T07:45:00Z",
      completed_at: "2026-09-07T07:45:10Z",
      next_run_at: "2026-09-07T08:45:00Z",
      status: "SUCCEEDED",
      error_code: null,
      duplicate_recovered: false,
      consecutive_failures: 0,
    },
  ],
};

function syncInput(
  overrides: Partial<ConnectionSyncEvidenceInput> = {},
): ConnectionSyncEvidenceInput {
  return {
    account,
    syncHistory: {
      state: "CURRENT",
      unauthorized: false,
      checkpoints: [
        {
          id: ids.checkpoint,
          operationID: ids.operation,
          financialAccountID: ids.account,
          providerConnectionID: ids.connection,
          provider: "coinbase",
          sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
          outcome: "SAVED",
          accountCount: 1,
          observedAt: "2026-09-07T07:41:59Z",
          completedAt: "2026-09-07T07:42:00Z",
          createdAt: "2026-09-07T07:42:00Z",
          durationMilliseconds: 1000,
        },
      ],
    },
    attemptHistory: {
      state: "CURRENT",
      unauthorized: false,
      attempts: [
        {
          id: ids.operation,
          providerConnectionID: ids.connection,
          provider: "coinbase",
          sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
          outcome: "SAVED",
          accountCount: 1,
          observedAt: "2026-09-07T07:41:59Z",
          completedAt: "2026-09-07T07:42:00Z",
          createdAt: "2026-09-07T07:42:00Z",
          durationMilliseconds: 1000,
        },
      ],
    },
    reconciliationPayload: {
      evidence_semantics: "SAVED_PORTFOLIO_RECONCILIATION",
      provider_read_performed: false,
      broker_action_available: false,
      live_execution_available: false,
      reconciliation: {
        id: ids.reconciliation,
        financial_account_id: ids.account,
        provider: "coinbase",
        comparison_status: "MATCHED",
        balances_status: "READY",
        positions_status: "READY",
        observed_position_count: 1,
        observed_at: "2026-09-07T07:42:00Z",
        created_at: "2026-09-07T07:42:01Z",
        evidence_hash: "a".repeat(64),
        blocks_new_actions: false,
        positions: [
          {
            symbol: "BTC",
            instrument_type: "CRYPTO",
            direction: "long",
            quantity: "1",
            available_quantity: "1.0",
            unavailable_to_trade_quantity: "0",
          },
        ],
      },
    },
    bindings: [
      {
        instance_id: ids.instance,
        mandate_id: ids.mandate,
        account_id: ids.account,
        capital_bucket_id: ids.bucket,
        strategy_identifier: "ai_shadow",
        execution_mode: "PAPER",
        current_state: "AI_MONITORING",
        status: "ACTIVE",
      },
    ],
    ...overrides,
  };
}

describe("FinancialConnectionOperatingWorkspace", () => {
  afterEach(cleanup);

  it("combines exact current connection, portfolio, sync, and engine evidence", () => {
    const result = projectFinancialConnectionOperatingBrief({
      connections: [connection],
      accounts: [account],
      engines: [engine],
      syncInputs: [syncInput()],
      observedAt,
      contextAvailable: true,
      expectedBindingCount: 1,
    });

    expect(result).toMatchObject({
      status: "VERIFIED",
      onCourseCount: 1,
      collectingCount: 0,
      reviewCount: 0,
      unavailableCount: 0,
    });
    expect(result.connections[0]).toMatchObject({
      state: "ON_COURSE",
      accountCount: 1,
      latestPortfolioObservedAt: "2026-09-07T07:42:00.000Z",
      syncAttemptCount: 1,
      syncSuccessCount: 1,
      paperEngineCount: 1,
      shadowEngineCount: 0,
      nextRunAt: "2026-09-07T08:45:00Z",
    });
  });

  it("labels pre-contract attempt history as collecting without disturbing runtimes", () => {
    const pending = syncInput({
      syncHistory: {
        state: "FORWARD_COLLECTION_PENDING",
        unauthorized: false,
        checkpoints: [],
      },
      attemptHistory: {
        state: "FORWARD_COLLECTION_PENDING",
        unauthorized: false,
        attempts: [],
      },
    });
    const result = projectFinancialConnectionOperatingBrief({
      connections: [connection],
      accounts: [account],
      engines: [engine],
      syncInputs: [pending],
      observedAt,
      contextAvailable: true,
      expectedBindingCount: 1,
    });

    expect(result.status).toBe("ATTENTION");
    expect(result.connections[0]).toMatchObject({
      state: "COLLECTING",
      label: "Sync reliability evidence is collecting forward",
      syncAttemptCount: 0,
      paperEngineCount: 1,
    });
  });

  it("renders one nontechnical brief with advanced saved evidence collapsed", () => {
    render(
      <FinancialConnectionOperatingWorkspace
        connections={[connection]}
        accounts={[account]}
        engines={[engine]}
        syncInputs={[syncInput()]}
        observedAt={observedAt}
        contextAvailable
        expectedBindingCount={1}
      />,
    );

    expect(
      screen.getByRole("region", {
        name: "Every financial connection is operating on course.",
      }),
    ).toBeVisible();
    expect(screen.getByText("Connected and operating on course")).toBeVisible();
    expect(screen.getAllByText("1 of 1 saved")[0]).toBeVisible();
    expect(screen.getByText("1 Paper · 0 Shadow")).toBeVisible();
    expect(
      screen.getByText(
        "Detailed continuity, sync, and market-readiness evidence",
      ),
    ).toBeVisible();
    expect(
      screen.getAllByRole("link", { name: "Open account evidence →" })[0],
    ).toHaveAttribute(
      "href",
      `/accounts/${ids.account}#account-sync-history-title`,
    );
    expect(screen.getAllByText(/no provider refresh/)[1]).toBeVisible();
  });
});
