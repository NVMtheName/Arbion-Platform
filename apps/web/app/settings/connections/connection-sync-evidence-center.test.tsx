import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { AccountSyncHistoryResult } from "../../accounts/[id]/account-sync-history";
import {
  ConnectionSyncEvidenceCenter,
  type ConnectionSyncEvidenceInput,
  projectConnectionSyncEvidence,
} from "./connection-sync-evidence-center";

const viewedAt = "2026-09-07T06:00:00Z";

const ids = {
  account: "00000000-0000-4000-8000-000000000001",
  connection: "00000000-0000-4000-8000-000000000002",
  checkpoint: "00000000-0000-4000-8000-000000000003",
  operation: "00000000-0000-4000-8000-000000000004",
  reconciliation: "00000000-0000-4000-8000-000000000005",
  instanceAI: "00000000-0000-4000-8000-000000000006",
  mandateAI: "00000000-0000-4000-8000-000000000007",
  bucketAI: "00000000-0000-4000-8000-000000000008",
  instanceRules: "00000000-0000-4000-8000-000000000009",
  mandateRules: "00000000-0000-4000-8000-00000000000a",
  bucketRules: "00000000-0000-4000-8000-00000000000b",
  failureAttempt: "00000000-0000-4000-8000-00000000000c",
};

function history(
  overrides: Partial<AccountSyncHistoryResult> = {},
): AccountSyncHistoryResult {
  return {
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
        observedAt: "2026-09-07T05:50:00.000Z",
        completedAt: "2026-09-07T05:50:02.000Z",
        createdAt: "2026-09-07T05:50:02.000Z",
        durationMilliseconds: 2000,
      },
    ],
    ...overrides,
  };
}

function reconciliation(overrides: Record<string, unknown> = {}) {
  return {
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
      observed_position_count: 2,
      observed_at: "2026-09-07T05:51:00Z",
      created_at: "2026-09-07T05:51:01Z",
      evidence_hash: "a".repeat(64),
      blocks_new_actions: false,
      positions: [
        {
          symbol: "BTC",
          instrument_type: "CRYPTO",
          direction: "long",
          quantity: "1.00000000",
          available_quantity: "0.40000000",
          unavailable_to_trade_quantity: "0.60000000",
        },
        {
          symbol: "USDC",
          instrument_type: "CRYPTO",
          direction: "long",
          quantity: "10",
          available_quantity: "0",
          unavailable_to_trade_quantity: "10.0000",
        },
      ],
      ...overrides,
    },
  };
}

function input(
  overrides: Partial<ConnectionSyncEvidenceInput> = {},
): ConnectionSyncEvidenceInput {
  return {
    account: {
      id: ids.account,
      provider_connection_id: ids.connection,
      provider: "coinbase",
      display_name: "Coinbase Portfolio",
      status: "active",
      last_synced_at: "2026-09-07T05:50:02Z",
    },
    syncHistory: history(),
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
          observedAt: "2026-09-07T05:50:00.000Z",
          completedAt: "2026-09-07T05:50:02.000Z",
          createdAt: "2026-09-07T05:50:02.000Z",
          durationMilliseconds: 2000,
        },
      ],
    },
    reconciliationPayload: reconciliation(),
    bindings: [
      {
        instance_id: ids.instanceAI,
        mandate_id: ids.mandateAI,
        account_id: ids.account,
        capital_bucket_id: ids.bucketAI,
        strategy_identifier: "ai_shadow",
        execution_mode: "SHADOW",
        current_state: "AI_MONITORING",
        status: "ACTIVE",
      },
      {
        instance_id: ids.instanceRules,
        mandate_id: ids.mandateRules,
        account_id: ids.account,
        capital_bucket_id: ids.bucketRules,
        strategy_identifier: "wheel",
        execution_mode: "PAPER",
        current_state: "READY_FOR_PUT",
        status: "ACTIVE",
      },
    ],
    ...overrides,
  };
}

describe("ConnectionSyncEvidenceCenter", () => {
  afterEach(cleanup);

  it("proves saved sync, complete holdings, restricted inventory, and every active runtime binding", () => {
    const result = projectConnectionSyncEvidence({
      inputs: [input()],
      observedAt: viewedAt,
      expectedBindingCount: 2,
    });

    expect(result).toMatchObject({
      status: "VERIFIED",
      currentCount: 1,
      collectingCount: 0,
      reviewCount: 0,
      unavailableCount: 0,
    });
    expect(result.accounts[0]).toMatchObject({
      state: "CURRENT",
      syncSampleCount: 1,
      holdingsStatus: "COMPLETE",
      holdingCount: 2,
      restrictedInventoryStatus: "AVAILABLE",
      restrictedHoldingCount: 2,
      aiEngineCount: 1,
      tradeLifecycleCount: 1,
      providerEventTimeStatus: "UNAVAILABLE",
      syncFailureHistoryStatus: "AVAILABLE",
      syncAttemptCount: 1,
      syncFailureCount: 0,
      recoveredFailureCount: 0,
      currentFailureCount: 0,
    });
  });

  it("keeps saved holdings while a first forward sync receipt is pending", () => {
    const result = projectConnectionSyncEvidence({
      inputs: [
        input({
          syncHistory: history({
            state: "FORWARD_COLLECTION_PENDING",
            checkpoints: [],
          }),
          attemptHistory: {
            state: "FORWARD_COLLECTION_PENDING",
            unauthorized: false,
            attempts: [],
          },
        }),
      ],
      observedAt: viewedAt,
      expectedBindingCount: 2,
    });

    expect(result.status).toBe("ATTENTION");
    expect(result.accounts[0]).toMatchObject({
      state: "COLLECTING",
      holdingsStatus: "COMPLETE",
      holdingCount: 2,
    });
  });

  it("shows a preserved failure as followed by a newer exact success", () => {
    const recovered = input();
    recovered.attemptHistory.attempts.push({
      id: ids.failureAttempt,
      providerConnectionID: ids.connection,
      provider: "coinbase",
      sourceOperation: "PROVIDER_ACCOUNT_DISCOVERY",
      outcome: "FAILED",
      failureStage: "ACCOUNT_DISCOVERY",
      errorCode: "RATE_LIMITED",
      observedAt: "2026-09-07T04:50:00.000Z",
      completedAt: "2026-09-07T04:50:02.000Z",
      createdAt: "2026-09-07T04:50:02.000Z",
      durationMilliseconds: 2000,
    });
    const result = projectConnectionSyncEvidence({
      inputs: [recovered],
      observedAt: viewedAt,
      expectedBindingCount: 2,
    });
    expect(result.accounts[0]).toMatchObject({
      state: "CURRENT",
      syncFailureCount: 1,
      recoveredFailureCount: 1,
      currentFailureCount: 0,
      latestFailureStage: "ACCOUNT_DISCOVERY",
      latestFailureCode: "RATE_LIMITED",
    });

    const current = input({
      attemptHistory: {
        ...recovered.attemptHistory,
        attempts: [
          {
            ...recovered.attemptHistory.attempts[1],
            observedAt: "2026-09-07T05:55:00.000Z",
            completedAt: "2026-09-07T05:55:02.000Z",
            createdAt: "2026-09-07T05:55:02.000Z",
          },
          recovered.attemptHistory.attempts[0],
        ],
      },
    });
    const currentResult = projectConnectionSyncEvidence({
      inputs: [current],
      observedAt: viewedAt,
      expectedBindingCount: 2,
    });
    expect(currentResult.accounts[0]).toMatchObject({
      state: "REVIEW",
      currentFailureCount: 1,
      latestAttemptOutcome: "FAILED",
    });
  });

  it("fails closed when the portfolio contract or restricted quantity split is inconsistent", () => {
    const result = projectConnectionSyncEvidence({
      inputs: [
        input({
          reconciliationPayload: reconciliation({
            positions: [
              {
                symbol: "BTC",
                instrument_type: "CRYPTO",
                direction: "long",
                quantity: "1",
                available_quantity: "0.4",
                unavailable_to_trade_quantity: "0.7",
              },
              {
                symbol: "USDC",
                instrument_type: "CRYPTO",
                direction: "long",
                quantity: "10",
                available_quantity: "0",
                unavailable_to_trade_quantity: "10",
              },
            ],
          }),
        }),
      ],
      observedAt: viewedAt,
      expectedBindingCount: 2,
    });

    expect(result.status).toBe("UNAVAILABLE");
    expect(result.accounts[0]).toMatchObject({
      state: "UNAVAILABLE",
      holdingsStatus: "UNAVAILABLE",
      restrictedInventoryStatus: "UNAVAILABLE",
    });
  });

  it("fails closed when a runtime identity is duplicated or omitted from the account fleet", () => {
    const duplicateAccountID = "00000000-0000-4000-8000-000000000010";
    const duplicateConnectionID = "00000000-0000-4000-8000-000000000011";
    const duplicate = input({
      account: {
        id: duplicateAccountID,
        provider_connection_id: duplicateConnectionID,
        provider: "schwab",
        display_name: "Schwab Brokerage",
        status: "active",
        last_synced_at: "2026-09-07T05:50:02Z",
      },
      syncHistory: {
        state: "FORWARD_COLLECTION_PENDING",
        unauthorized: false,
        checkpoints: [],
      },
      reconciliationPayload: {
        ...reconciliation(),
        reconciliation: {
          ...reconciliation().reconciliation,
          financial_account_id: duplicateAccountID,
          provider: "schwab",
          positions: [],
          observed_position_count: 0,
        },
      },
      bindings: [
        {
          ...input().bindings[0],
          account_id: duplicateAccountID,
        },
      ],
    });
    const result = projectConnectionSyncEvidence({
      inputs: [input(), duplicate],
      observedAt: viewedAt,
      expectedBindingCount: 3,
    });

    expect(result.status).toBe("UNAVAILABLE");
    expect(result.unavailableCount).toBe(2);
  });

  it("renders concise owner guidance and immutable evidence actions without implying a refresh", () => {
    render(
      <ConnectionSyncEvidenceCenter
        inputs={[input()]}
        observedAt={viewedAt}
        expectedBindingCount={2}
      />,
    );

    expect(
      screen.getByRole("region", {
        name: "Every account has an attributable saved input chain.",
      }),
    ).toBeVisible();
    expect(
      screen.getByText("Every account has an attributable saved input chain."),
    ).toBeVisible();
    expect(
      screen.getByText("2 exact positions at Sep 7, 2026, 5:51:00 AM UTC"),
    ).toBeVisible();
    expect(
      screen.getByText("2 positions include unavailable-to-trade quantity"),
    ).toBeVisible();
    expect(screen.getByText("1 AI · 1 rules")).toBeVisible();
    expect(screen.getAllByText(/Open immutable runtime evidence/)).toHaveLength(
      2,
    );
    expect(screen.getByText(/no provider refresh/)).toBeVisible();
    expect(
      screen.getByText(/Staking \/ earn classification: UNAVAILABLE/),
    ).toBeInTheDocument();
  });
});
