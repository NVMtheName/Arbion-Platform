import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { FinancialAccount, FinancialConnection } from "./page";
import {
  FinancialContinuityCenter,
  type FinancialContinuityEngine,
  type FinancialContinuityRun,
  projectFinancialContinuityCenter,
} from "./financial-continuity-center";

const observedAt = "2026-09-01T16:00:00Z";

function connection(
  overrides: Partial<FinancialConnection> = {},
): FinancialConnection {
  return {
    id: "coinbase-connection",
    provider: "coinbase",
    display_name: "Coinbase Production",
    status: "active",
    runtime_protected: true,
    protected_mandate_count: 2,
    active_strategy_count: 2,
    last_synced_at: "2026-09-01T15:50:00Z",
    ...overrides,
  };
}

function account(overrides: Partial<FinancialAccount> = {}): FinancialAccount {
  return {
    id: "coinbase-account",
    provider_connection_id: "coinbase-connection",
    provider: "coinbase",
    display_name: "Coinbase Portfolio",
    status: "active",
    last_synced_at: "2026-09-01T15:51:00Z",
    ...overrides,
  };
}

function run(
  overrides: Partial<FinancialContinuityRun> = {},
): FinancialContinuityRun {
  return {
    id: "run-paper-latest",
    scheduled_for: "2026-09-01T15:45:00Z",
    completed_at: "2026-09-01T15:45:20Z",
    next_run_at: "2026-09-01T16:45:00Z",
    status: "SUCCEEDED",
    error_code: null,
    duplicate_recovered: false,
    consecutive_failures: 0,
    ...overrides,
  };
}

function engine(
  overrides: Partial<FinancialContinuityEngine> = {},
): FinancialContinuityEngine {
  const latest = run();
  return {
    mandate_id: "mandate-paper",
    instance_id: "instance-paper",
    connection_id: "coinbase-connection",
    account_id: "coinbase-account",
    account_name: "Coinbase Portfolio",
    provider: "coinbase",
    execution_mode: "PAPER",
    instance_status: "ACTIVE",
    current_state: "AI_MONITORING",
    schedule_available: true,
    schedule_history_available: true,
    schedule_status: latest.status ?? undefined,
    schedule_completed_at: latest.completed_at ?? undefined,
    schedule_next_run_at: latest.next_run_at ?? undefined,
    schedule_timing_status: "ON_SCHEDULE",
    consecutive_failures: 0,
    recent_runs: [latest],
    ...overrides,
  };
}

describe("FinancialContinuityCenter", () => {
  afterEach(cleanup);

  it("groups exact Paper and Shadow dependencies by provider connection", () => {
    const coinbasePaper = engine();
    const coinbaseShadow = engine({
      mandate_id: "mandate-shadow",
      instance_id: "instance-shadow",
      execution_mode: "SHADOW",
      recent_runs: [
        run({
          id: "run-shadow-latest",
          scheduled_for: "2026-09-01T15:50:00Z",
          completed_at: "2026-09-01T15:50:15Z",
          next_run_at: "2026-09-01T16:50:00Z",
        }),
      ],
      schedule_completed_at: "2026-09-01T15:50:15Z",
      schedule_next_run_at: "2026-09-01T16:50:00Z",
    });
    const schwabConnection = connection({
      id: "schwab-connection",
      provider: "schwab",
      display_name: "Charles Schwab",
      authorization_expires_at: "2026-09-03T16:00:00Z",
    });
    const schwabAccount = account({
      id: "schwab-account",
      provider_connection_id: "schwab-connection",
      provider: "schwab",
      display_name: "Schwab Brokerage",
    });
    const schwabEngine = engine({
      mandate_id: "mandate-schwab",
      instance_id: "instance-schwab",
      connection_id: "schwab-connection",
      account_id: "schwab-account",
      account_name: "Schwab Brokerage",
      provider: "schwab",
      execution_mode: "SHADOW",
      recent_runs: [
        run({
          id: "run-schwab-latest",
          scheduled_for: "2026-09-01T15:35:00Z",
          completed_at: "2026-09-01T15:35:12Z",
          next_run_at: "2026-09-02T13:35:00Z",
        }),
      ],
      schedule_completed_at: "2026-09-01T15:35:12Z",
      schedule_next_run_at: "2026-09-02T13:35:00Z",
    });

    const result = projectFinancialContinuityCenter({
      connections: [connection(), schwabConnection],
      accounts: [account(), schwabAccount],
      engines: [coinbasePaper, coinbaseShadow, schwabEngine],
      observedAt,
    });

    expect(result.status).toBe("VERIFIED");
    expect(result.connectionCount).toBe(2);
    expect(result.engineCount).toBe(3);
    expect(result.connections[0]).toMatchObject({
      provider: "coinbase",
      state: "CURRENT",
      paperEngineCount: 1,
      shadowEngineCount: 1,
    });
    expect(result.connections[1]).toMatchObject({
      provider: "schwab",
      state: "CURRENT",
      paperEngineCount: 0,
      shadowEngineCount: 1,
    });
  });

  it("treats the exact 24-hour boundary as expiring soon and expiry as expired", () => {
    const expiring = projectFinancialContinuityCenter({
      connections: [
        connection({ authorization_expires_at: "2026-09-02T16:00:00Z" }),
      ],
      accounts: [account()],
      engines: [engine()],
      observedAt,
    });
    const expired = projectFinancialContinuityCenter({
      connections: [
        connection({ authorization_expires_at: "2026-09-01T16:00:00Z" }),
      ],
      accounts: [account()],
      engines: [engine()],
      observedAt,
    });

    expect(expiring.connections[0].state).toBe("EXPIRING_SOON");
    expect(expired.connections[0].state).toBe("EXPIRED");
  });

  it("pairs reconnect recovery only with a later success from the same engine", () => {
    const failed = run({
      id: "run-paper-failed",
      scheduled_for: "2026-09-01T14:45:00Z",
      completed_at: "2026-09-01T14:45:20Z",
      next_run_at: "2026-09-01T15:45:00Z",
      status: "FAILED",
      error_code: "RECONCILIATION_REFRESH_FAILED",
      consecutive_failures: 1,
    });
    const recoveredEngine = engine({ recent_runs: [run(), failed] });
    const recovered = projectFinancialContinuityCenter({
      connections: [connection()],
      accounts: [account()],
      engines: [recoveredEngine],
      observedAt,
    });
    expect(recovered.connections[0].state).toBe("RECOVERED");
    expect(recovered.connections[0].priorIncidentAt).toBe(failed.completed_at);

    const stillFailed = engine({
      schedule_status: "FAILED",
      schedule_completed_at: failed.completed_at ?? undefined,
      schedule_next_run_at: failed.next_run_at ?? undefined,
      consecutive_failures: 1,
      recent_runs: [failed],
    });
    const unrelatedSuccess = engine({
      mandate_id: "mandate-shadow",
      instance_id: "instance-shadow",
      execution_mode: "SHADOW",
      recent_runs: [run({ id: "unrelated-success" })],
    });
    const affected = projectFinancialContinuityCenter({
      connections: [connection()],
      accounts: [account()],
      engines: [stillFailed, unrelatedSuccess],
      observedAt,
    });
    expect(affected.connections[0].state).toBe("SCHEDULER_AFFECTED");
    expect(affected.connections[0].recoveredAt).toBeUndefined();
  });

  it("fails closed on incomplete inventory, future evidence, and duplicate identity", () => {
    const noAccounts = projectFinancialContinuityCenter({
      connections: [connection()],
      accounts: [],
      engines: [],
      observedAt,
    });
    const futureSync = projectFinancialContinuityCenter({
      connections: [connection()],
      accounts: [account({ last_synced_at: "2026-09-01T16:00:01Z" })],
      engines: [engine()],
      observedAt,
    });
    const duplicateEngine = engine();
    const duplicateIdentity = projectFinancialContinuityCenter({
      connections: [connection()],
      accounts: [account()],
      engines: [duplicateEngine, { ...duplicateEngine }],
      observedAt,
    });

    expect(noAccounts.connections[0].state).toBe("UNAVAILABLE");
    expect(futureSync.connections[0].state).toBe("UNAVAILABLE");
    expect(duplicateIdentity.connections[0].state).toBe("UNAVAILABLE");
  });

  it("opens attention evidence and preserves a direct immutable engine link", () => {
    render(
      <FinancialContinuityCenter
        connections={[
          connection({ authorization_expires_at: "2026-09-02T12:00:00Z" }),
        ]}
        accounts={[account()]}
        engines={[engine()]}
        observedAt={observedAt}
      />,
    );

    expect(
      screen.getByRole("region", {
        name: "1 financial connection needs review.",
      }),
    ).toBeVisible();
    const review = screen.getByText("OWNER REVIEW").closest("details");
    expect(review).toHaveAttribute("open");
    expect(
      screen.getByRole("link", { name: /Open immutable engine evidence/ }),
    ).toHaveAttribute("href", "/automations/mandate-paper#runtime-evidence");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(
      screen.getByText(/Reconnecting one provider does not rewrite another/),
    ).toBeVisible();
  });

  it("shows an explicit fail-closed state when the inventory cannot be loaded", () => {
    render(
      <FinancialContinuityCenter
        connections={[]}
        accounts={[]}
        engines={[]}
        observedAt={observedAt}
        contextAvailable={false}
      />,
    );

    expect(
      screen.getByRole("region", {
        name: "Financial connection continuity is unavailable.",
      }),
    ).toHaveTextContent("will not infer dependency safety");
  });
});
