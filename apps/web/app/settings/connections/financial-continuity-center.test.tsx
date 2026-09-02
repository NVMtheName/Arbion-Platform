import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { FinancialAccount, FinancialConnection } from "./page";
import {
  FinancialContinuityCenter,
  SchwabMarketDataReadinessView,
  type FinancialContinuityEngine,
  type FinancialContinuityRun,
  projectFinancialContinuityCenter,
  projectSchwabMarketDataReadiness,
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

function schwabReadinessEngine(
  overrides: Partial<FinancialContinuityEngine> = {},
): FinancialContinuityEngine {
  const latest = run({
    id: "run-schwab-market-data",
    scheduled_for: "2026-09-01T15:35:00Z",
    completed_at: "2026-09-01T15:35:12Z",
    next_run_at: "2026-09-01T16:35:00Z",
    status: "FAILED",
    error_code: "MARKET_DATA_NOT_REALTIME",
    consecutive_failures: 2,
  });
  return engine({
    mandate_id: "mandate-schwab",
    instance_id: "instance-schwab",
    connection_id: "schwab-connection",
    account_id: "schwab-account",
    account_name: "Schwab Brokerage",
    provider: "schwab",
    execution_mode: "SHADOW",
    schedule_status: "FAILED",
    schedule_completed_at: latest.completed_at ?? undefined,
    schedule_next_run_at: latest.next_run_at ?? undefined,
    consecutive_failures: 2,
    recent_runs: [latest],
    market_scope_available: true,
    market_symbols: ["SPY"],
    required_market_quality: "BROKER_REALTIME",
    ...overrides,
  });
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

  it("separates active Schwab authorization from a saved market-data readiness stop", () => {
    const schwabConnection = connection({
      id: "schwab-connection",
      provider: "schwab",
      display_name: "Charles Schwab",
      authorization_expires_at: "2026-09-08T16:00:00Z",
    });
    const schwabAccount = account({
      id: "schwab-account",
      provider_connection_id: "schwab-connection",
      provider: "schwab",
      display_name: "Schwab Brokerage",
    });
    const failed = run({
      id: "run-schwab-market-data",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_NOT_REALTIME",
      consecutive_failures: 1,
    });
    const schwabEngine = engine({
      mandate_id: "mandate-schwab",
      instance_id: "instance-schwab",
      connection_id: "schwab-connection",
      account_id: "schwab-account",
      account_name: "Schwab Brokerage",
      provider: "schwab",
      execution_mode: "SHADOW",
      schedule_status: "FAILED",
      schedule_completed_at: failed.completed_at ?? undefined,
      schedule_next_run_at: failed.next_run_at ?? undefined,
      consecutive_failures: 1,
      recent_runs: [failed],
    });

    const result = projectFinancialContinuityCenter({
      connections: [schwabConnection],
      accounts: [schwabAccount],
      engines: [schwabEngine],
      observedAt,
    });
    expect(result.connections[0]).toMatchObject({
      state: "SCHEDULER_AFFECTED",
      label: "Authorization active; dependent engine paused safely",
      schedulerErrorCodes: ["MARKET_DATA_NOT_REALTIME"],
    });

    render(
      <FinancialContinuityCenter
        connections={[schwabConnection]}
        accounts={[schwabAccount]}
        engines={[schwabEngine]}
        observedAt={observedAt}
      />,
    );
    expect(
      screen.getByText("Authorization active; dependent engine paused safely"),
    ).toBeVisible();
    expect(
      screen.getByText(/Schwab did not explicitly mark the saved quote/),
    ).toHaveTextContent(/stopped before the AI model/i);
    expect(
      screen.getByText(/Connection authorization and non-live/),
    ).toBeVisible();
  });

  it("projects and renders active authorization separately from quote entitlement", () => {
    const schwabConnection = connection({
      id: "schwab-connection",
      provider: "schwab",
      display_name: "Charles Schwab",
    });
    const schwabEngine = schwabReadinessEngine();
    const result = projectSchwabMarketDataReadiness({
      connections: [schwabConnection],
      engines: [schwabEngine],
    });

    expect(result).toMatchObject({
      status: "ATTENTION",
      engineCount: 1,
      readyCount: 0,
      attentionCount: 1,
      unavailableCount: 0,
    });
    expect(result.engines[0]).toMatchObject({
      state: "ENTITLEMENT_REVIEW",
      authorizationProof: "VERIFIED",
      entitlementProof: "REVIEW",
      quoteProof: "LEGACY_UNRESOLVED",
      connectionStatus: "active",
      symbols: ["SPY"],
      requiredQuality: "BROKER_REALTIME",
      errorCode: "MARKET_DATA_NOT_REALTIME",
      consecutiveFailures: 2,
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[schwabConnection]}
        engines={[schwabEngine]}
      />,
    );
    expect(
      screen.getByRole("region", {
        name: "Schwab is connected, but its quote gate needs review.",
      }),
    ).toBeVisible();
    expect(
      screen.getByText("Connected; real-time quote not confirmed"),
    ).toBeVisible();
    expect(screen.getAllByText("SPY")).toHaveLength(2);
    expect(screen.getByText(/passes BROKER_REALTIME/)).toBeVisible();
    expect(screen.getByText("VERIFIED")).toBeVisible();
    expect(screen.getByText("REVIEW")).toBeVisible();
    expect(screen.getByText("LEGACY_UNRESOLVED")).toBeVisible();
    expect(screen.getByText("2")).toBeVisible();
    expect(
      screen.getByText(/reconnecting alone does not prove/i),
    ).toBeVisible();
    expect(screen.getByText(/Reconnecting does not prove/)).toBeVisible();
    expect(
      screen.getByRole("link", { name: /Open immutable scheduler evidence/ }),
    ).toHaveAttribute("href", "/automations/mandate-schwab#runtime-evidence");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(
      screen
        .getByText("Why connected can still stop safely")
        .closest("details"),
    ).toHaveAttribute("open");
    expect(screen.getByText("What to verify in Schwab")).toBeVisible();
    expect(
      screen.getByText(/Trader API – Individual production market data/),
    ).toBeVisible();
    expect(
      screen.getByText(/Never send a client secret or refresh token/),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: /Open Schwab developer apps/ }),
    ).toHaveAttribute("href", "https://developer.schwab.com/dashboard/apps");
  });

  it("shows the Schwab owner checklist only for quote-entitlement review", () => {
    const successfulRun = run({
      id: "run-broker-realtime",
      status: "SUCCEEDED",
      error_code: null,
      consecutive_failures: 0,
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[
          connection({ id: "schwab-connection", provider: "schwab" }),
        ]}
        engines={[
          schwabReadinessEngine({
            schedule_status: "SUCCEEDED",
            schedule_completed_at: successfulRun.completed_at ?? undefined,
            schedule_next_run_at: successfulRun.next_run_at ?? undefined,
            consecutive_failures: 0,
            recent_runs: [successfulRun],
          }),
        ]}
      />,
    );

    expect(
      screen.queryByText("What to verify in Schwab"),
    ).not.toBeInTheDocument();
  });

  it.each([
    ["AUTHORIZATION_EXPIRED", "AUTHORIZATION_REVIEW"],
    ["PROVIDER_UNAVAILABLE", "AUTOMATIC_RETRY"],
    ["MARKET_DATA_INVALID", "OPERATOR_REVIEW"],
  ] as const)(
    "classifies %s without conflating it with entitlement",
    (code, state) => {
      const latest = run({
        id: `run-${code}`,
        status: "FAILED",
        error_code: code,
        consecutive_failures: 1,
      });
      const result = projectSchwabMarketDataReadiness({
        connections: [
          connection({ id: "schwab-connection", provider: "schwab" }),
        ],
        engines: [
          schwabReadinessEngine({
            schedule_completed_at: latest.completed_at ?? undefined,
            schedule_next_run_at: latest.next_run_at ?? undefined,
            consecutive_failures: 1,
            recent_runs: [latest],
          }),
        ],
      });
      expect(result.engines[0].state).toBe(state);
    },
  );

  it.each([
    ["SUCCEEDED", null, "PROVEN", "BROKER_REALTIME"],
    ["FAILED", "MARKET_DATA_DELAYED", "REVIEW", "DELAYED"],
    [
      "FAILED",
      "MARKET_DATA_REALTIME_UNCONFIRMED",
      "REVIEW",
      "REALTIME_UNCONFIRMED",
    ],
    ["SKIPPED", "OUTSIDE_SESSION", "NOT_TESTED", "SESSION_WAIT"],
    ["FAILED", "PROVIDER_UNAVAILABLE", "NOT_TESTED", "NOT_EVALUATED"],
  ] as const)(
    "separates authorization, entitlement, and quote proof for %s %s",
    (status, errorCode, entitlementProof, quoteProof) => {
      const latest = run({
        id: `run-proof-${status}-${errorCode ?? "none"}`,
        status,
        error_code: errorCode,
        consecutive_failures: status === "FAILED" ? 1 : 0,
      });
      const result = projectSchwabMarketDataReadiness({
        connections: [
          connection({ id: "schwab-connection", provider: "schwab" }),
        ],
        engines: [
          schwabReadinessEngine({
            schedule_status: status,
            schedule_completed_at: latest.completed_at ?? undefined,
            schedule_next_run_at: latest.next_run_at ?? undefined,
            consecutive_failures: status === "FAILED" ? 1 : 0,
            recent_runs: [latest],
          }),
        ],
      });

      expect(result.engines[0]).toMatchObject({
        authorizationProof: "VERIFIED",
        entitlementProof,
        quoteProof,
      });
    },
  );

  it("preserves a bounded exact Schwab quote-quality history", () => {
    const newest = run({
      id: "run-delayed",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });
    const priorUnconfirmed = run({
      id: "run-unconfirmed",
      scheduled_for: "2026-09-01T14:35:00Z",
      completed_at: "2026-09-01T14:35:11Z",
      next_run_at: "2026-09-01T15:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_REALTIME_UNCONFIRMED",
      consecutive_failures: 1,
    });
    const priorSuccess = run({
      id: "run-realtime",
      scheduled_for: "2026-09-01T13:35:00Z",
      completed_at: "2026-09-01T13:35:10Z",
      next_run_at: "2026-09-01T14:35:00Z",
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_completed_at: newest.completed_at ?? undefined,
          schedule_next_run_at: newest.next_run_at ?? undefined,
          consecutive_failures: 1,
          recent_runs: [newest, priorUnconfirmed, priorSuccess],
        }),
      ],
    });

    expect(result.engines[0]).toMatchObject({
      quoteHistoryStatus: "AVAILABLE",
      quoteHistorySampleCount: 3,
      quoteHistoryCapped: false,
      quoteIncidentStatus: "AVAILABLE",
      quoteIncidentWindow: "COMPLETE_LOADED_HISTORY",
      quoteSafeWaitCount: 0,
      quoteIncidents: [
        {
          state: "OPEN",
          recurringAfterRecovery: false,
          failureCount: 2,
          durationMilliseconds: 3601000,
          errorCodes: [
            "MARKET_DATA_REALTIME_UNCONFIRMED",
            "MARKET_DATA_DELAYED",
          ],
        },
      ],
      quoteHistory: [
        { id: "run-delayed", state: "DELAYED" },
        { id: "run-unconfirmed", state: "REALTIME_UNCONFIRMED" },
        { id: "run-realtime", state: "BROKER_REALTIME" },
      ],
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[
          connection({ id: "schwab-connection", provider: "schwab" }),
        ]}
        engines={[
          schwabReadinessEngine({
            schedule_completed_at: newest.completed_at ?? undefined,
            schedule_next_run_at: newest.next_run_at ?? undefined,
            consecutive_failures: 1,
            recent_runs: [newest, priorUnconfirmed, priorSuccess],
          }),
        ]}
      />,
    );
    expect(screen.getByText("Delayed quote rejected")).toBeVisible();
    expect(screen.getByText("Real-time status missing")).toBeVisible();
    expect(screen.getByText("Broker real-time check passed")).toBeVisible();
    expect(screen.getByText(/3 immutable scheduler results/)).toBeVisible();
    expect(screen.getByText("Open quote-quality incident")).toBeVisible();
    expect(screen.getByText("1.000 hours saved span")).toBeVisible();
    expect(screen.getByText("Safe Schwab support reference")).toBeVisible();
    expect(screen.getByText("NO CREDENTIALS")).toBeVisible();
    expect(
      screen.getByText("Complete loaded history; first saved failure is exact"),
    ).toBeVisible();
  });

  it("proves automatic Schwab quote recovery from bounded saved history", () => {
    const recovered = run({
      id: "run-recovered",
      scheduled_for: "2026-09-02T15:35:00Z",
      completed_at: "2026-09-02T15:35:08Z",
      next_run_at: "2026-09-02T16:35:00Z",
      status: "SUCCEEDED",
      error_code: null,
      consecutive_failures: 0,
    });
    const sessionWait = run({
      id: "run-session-wait-before-recovery",
      scheduled_for: "2026-09-02T14:35:00Z",
      completed_at: "2026-09-02T14:35:02Z",
      next_run_at: "2026-09-02T15:35:00Z",
      status: "SKIPPED",
      error_code: "OUTSIDE_SESSION",
      consecutive_failures: 0,
    });
    const delayed = run({
      id: "run-delayed-before-recovery",
      scheduled_for: "2026-09-02T13:35:00Z",
      completed_at: "2026-09-02T13:35:06Z",
      next_run_at: "2026-09-02T14:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });
    const engine = schwabReadinessEngine({
      schedule_status: "SUCCEEDED",
      schedule_completed_at: recovered.completed_at ?? undefined,
      schedule_next_run_at: recovered.next_run_at ?? undefined,
      consecutive_failures: 0,
      recent_runs: [recovered, sessionWait, delayed],
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [engine],
    });

    expect(result.engines[0]).toMatchObject({
      state: "READY",
      quoteIncidentStatus: "AVAILABLE",
      quoteIncidentWindow: "COMPLETE_LOADED_HISTORY",
      quoteSafeWaitCount: 1,
      quoteIncidents: [
        {
          state: "RECOVERED",
          recurringAfterRecovery: false,
          failureCount: 1,
          safeWaitCount: 1,
          durationMilliseconds: 7202000,
          errorCodes: ["MARKET_DATA_DELAYED"],
          startedAt: delayed.completed_at,
          recoveredAt: recovered.completed_at,
        },
      ],
      quoteRecovery: {
        errorCode: "MARKET_DATA_DELAYED",
        blockedAt: delayed.completed_at,
        recoveredAt: recovered.completed_at,
      },
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[
          connection({ id: "schwab-connection", provider: "schwab" }),
        ]}
        engines={[engine]}
      />,
    );
    expect(screen.getByText("Automatic recovery proven")).toBeVisible();
    expect(
      screen.getByText(/MARKET_DATA_DELAYED.*BROKER_REALTIME/),
    ).toBeVisible();
    expect(screen.getByText(/Failure streak reset to 0/)).toBeVisible();
    expect(screen.getByText("Recovered automatically")).toBeInTheDocument();
    expect(
      screen.getByText(/1 exact OUTSIDE_SESSION wait kept separate/),
    ).toBeInTheDocument();
  });

  it("shows when a later automatic quote check still blocks after connection verification", () => {
    const latest = run({
      id: "run-after-connection-verification",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 2,
    });
    const schwabConnection = connection({
      id: "schwab-connection",
      provider: "schwab",
      last_synced_at: "2026-09-01T15:30:00Z",
    });
    const schwabEngine = schwabReadinessEngine({
      schedule_completed_at: latest.completed_at ?? undefined,
      schedule_next_run_at: latest.next_run_at ?? undefined,
      consecutive_failures: 2,
      recent_runs: [latest],
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [schwabConnection],
      engines: [schwabEngine],
    });

    expect(result.engines[0].connectionQuoteCheckpoint).toEqual({
      state: "QUOTE_STILL_BLOCKED",
      label: "A later quote check still stopped safely",
      guidance:
        "The saved connection verification predates the newest automatic quote evaluation, which recorded MARKET_DATA_DELAYED. This proves the sequence only; it does not prove provider cause or account entitlement.",
      connectionVerifiedAt: "2026-09-01T15:30:00Z",
      latestEvaluationAt: "2026-09-01T15:35:12Z",
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[schwabConnection]}
        engines={[schwabEngine]}
      />,
    );
    expect(
      screen.getByText("Connection verification → quote check"),
    ).toBeVisible();
    expect(screen.getByText("STILL BLOCKED")).toBeVisible();
    expect(
      screen.getByText(/proves the sequence only.*does not prove provider/i),
    ).toBeVisible();
  });

  it("waits for the next automatic quote check when connection verification is newer", () => {
    const latest = run({
      id: "run-before-newer-connection-verification",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({
          id: "schwab-connection",
          provider: "schwab",
          last_synced_at: "2026-09-01T15:40:00Z",
        }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_completed_at: latest.completed_at ?? undefined,
          schedule_next_run_at: latest.next_run_at ?? undefined,
          consecutive_failures: 1,
          recent_runs: [latest],
        }),
      ],
    });

    expect(result.engines[0].connectionQuoteCheckpoint).toMatchObject({
      state: "WAITING_FOR_QUOTE_CHECK",
      label: "Newer connection verification is waiting for a quote check",
      connectionVerifiedAt: "2026-09-01T15:40:00Z",
      latestEvaluationAt: "2026-09-01T15:35:12Z",
    });
  });

  it("proves a broker-real-time quote passed after connection verification", () => {
    const latest = run({
      id: "run-passed-after-connection-verification",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "SUCCEEDED",
      error_code: null,
      consecutive_failures: 0,
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({
          id: "schwab-connection",
          provider: "schwab",
          last_synced_at: "2026-09-01T15:30:00Z",
        }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_status: "SUCCEEDED",
          schedule_completed_at: latest.completed_at ?? undefined,
          schedule_next_run_at: latest.next_run_at ?? undefined,
          consecutive_failures: 0,
          recent_runs: [latest],
        }),
      ],
    });

    expect(result.engines[0].connectionQuoteCheckpoint).toMatchObject({
      state: "QUOTE_PASSED",
      label: "A later automatic quote check passed",
      connectionVerifiedAt: "2026-09-01T15:30:00Z",
      latestEvaluationAt: "2026-09-01T15:35:12Z",
    });
  });

  it("does not infer connection-to-quote ordering without an exact verification time", () => {
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({
          id: "schwab-connection",
          provider: "schwab",
          last_synced_at: null,
        }),
      ],
      engines: [schwabReadinessEngine()],
    });

    expect(result.engines[0].connectionQuoteCheckpoint).toMatchObject({
      state: "UNAVAILABLE",
      label: "Connection-to-quote order unavailable",
      connectionVerifiedAt: "",
    });
  });

  it("does not infer recovery when saved quote history is not contiguous", () => {
    const recovered = run({
      id: "run-recovered-after-unknown",
      scheduled_for: "2026-09-02T15:35:00Z",
      completed_at: "2026-09-02T15:35:08Z",
      next_run_at: "2026-09-02T16:35:00Z",
      status: "SUCCEEDED",
      error_code: null,
      consecutive_failures: 0,
    });
    const unavailable = run({
      id: "run-provider-unavailable",
      scheduled_for: "2026-09-02T14:35:00Z",
      completed_at: "2026-09-02T14:35:02Z",
      next_run_at: "2026-09-02T15:35:00Z",
      status: "FAILED",
      error_code: "PROVIDER_UNAVAILABLE",
      consecutive_failures: 1,
    });
    const delayed = run({
      id: "run-delayed-before-unknown",
      scheduled_for: "2026-09-02T13:35:00Z",
      completed_at: "2026-09-02T13:35:06Z",
      next_run_at: "2026-09-02T14:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });

    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_status: "SUCCEEDED",
          schedule_completed_at: recovered.completed_at ?? undefined,
          schedule_next_run_at: recovered.next_run_at ?? undefined,
          consecutive_failures: 0,
          recent_runs: [recovered, unavailable, delayed],
        }),
      ],
    });

    expect(result.engines[0]).toMatchObject({
      quoteRecovery: undefined,
      quoteIncidentStatus: "UNAVAILABLE",
      quoteIncidentWindow: "UNAVAILABLE",
      quoteSafeWaitCount: 0,
      quoteIncidents: [],
    });
  });

  it("distinguishes a recurring quote incident after exact saved recovery", () => {
    const recurring = run({
      id: "run-recurring-unconfirmed",
      scheduled_for: "2026-09-02T15:35:00Z",
      completed_at: "2026-09-02T15:35:08Z",
      next_run_at: "2026-09-02T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_REALTIME_UNCONFIRMED",
      consecutive_failures: 1,
    });
    const recovered = run({
      id: "run-prior-recovery",
      scheduled_for: "2026-09-02T14:35:00Z",
      completed_at: "2026-09-02T14:35:08Z",
      next_run_at: "2026-09-02T15:35:00Z",
      status: "SUCCEEDED",
      error_code: null,
      consecutive_failures: 0,
    });
    const firstFailure = run({
      id: "run-first-delayed",
      scheduled_for: "2026-09-02T13:35:00Z",
      completed_at: "2026-09-02T13:35:08Z",
      next_run_at: "2026-09-02T14:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });
    const engine = schwabReadinessEngine({
      schedule_completed_at: recurring.completed_at ?? undefined,
      schedule_next_run_at: recurring.next_run_at ?? undefined,
      consecutive_failures: 1,
      recent_runs: [recurring, recovered, firstFailure],
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [engine],
    });

    expect(result.engines[0]).toMatchObject({
      quoteIncidentStatus: "AVAILABLE",
      quoteIncidentWindow: "COMPLETE_LOADED_HISTORY",
      quoteIncidents: [
        {
          state: "OPEN",
          recurringAfterRecovery: true,
          durationMilliseconds: 0,
          errorCodes: ["MARKET_DATA_REALTIME_UNCONFIRMED"],
        },
        {
          state: "RECOVERED",
          recurringAfterRecovery: false,
          durationMilliseconds: 3600000,
          errorCodes: ["MARKET_DATA_DELAYED"],
          recoveredAt: recovered.completed_at,
        },
      ],
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[
          connection({ id: "schwab-connection", provider: "schwab" }),
        ]}
        engines={[engine]}
      />,
    );
    expect(screen.getByText("Recurring after recovery")).toBeVisible();
  });

  it("marks the Schwab support reference as bounded when older history is capped", () => {
    const runs = Array.from({ length: 7 }, (_, index) =>
      run({
        id: `run-capped-${index}`,
        scheduled_for: `2026-09-02T${String(16 - index).padStart(2, "0")}:35:00Z`,
        completed_at: `2026-09-02T${String(16 - index).padStart(2, "0")}:35:08Z`,
        next_run_at: `2026-09-02T${String(17 - index).padStart(2, "0")}:35:00Z`,
        status: "FAILED",
        error_code: "MARKET_DATA_DELAYED",
        consecutive_failures: 7 - index,
      }),
    );
    const engine = schwabReadinessEngine({
      schedule_completed_at: runs[0].completed_at ?? undefined,
      schedule_next_run_at: runs[0].next_run_at ?? undefined,
      consecutive_failures: 7,
      recent_runs: runs,
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [engine],
    });

    expect(result.engines[0]).toMatchObject({
      quoteHistorySampleCount: 7,
      quoteHistoryCapped: true,
      quoteIncidentWindow: "BOUNDED_SAVED_WINDOW",
      quoteSupportReference: {
        accountName: "Schwab Brokerage",
        symbols: ["SPY"],
        errorCodes: ["MARKET_DATA_DELAYED"],
        firstObservedAt: runs[5].completed_at,
        latestObservedAt: runs[0].completed_at,
        nextRunAt: runs[0].next_run_at,
        durationMilliseconds: 18000000,
        window: "BOUNDED_SAVED_WINDOW",
      },
    });

    render(
      <SchwabMarketDataReadinessView
        connections={[
          connection({ id: "schwab-connection", provider: "schwab" }),
        ]}
        engines={[engine]}
      />,
    );
    expect(
      screen.getByText(
        /Bounded newest 6-check window.*may have started earlier/,
      ),
    ).toBeVisible();
  });

  it("fails the Schwab quote-quality history closed on duplicate rows", () => {
    const newest = run({
      id: "run-duplicate",
      scheduled_for: "2026-09-01T15:35:00Z",
      completed_at: "2026-09-01T15:35:12Z",
      next_run_at: "2026-09-01T16:35:00Z",
      status: "FAILED",
      error_code: "MARKET_DATA_DELAYED",
      consecutive_failures: 1,
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_completed_at: newest.completed_at ?? undefined,
          schedule_next_run_at: newest.next_run_at ?? undefined,
          consecutive_failures: 1,
          recent_runs: [newest, newest],
        }),
      ],
    });

    expect(result.engines[0]).toMatchObject({
      quoteHistoryStatus: "UNAVAILABLE",
      quoteHistorySampleCount: 2,
      quoteHistory: [],
    });
  });

  it.each([
    [
      "MARKET_DATA_DELAYED",
      "Connected; provider quote is delayed",
      /explicitly marked.*delayed.*reconnecting alone does not change/i,
    ],
    [
      "MARKET_DATA_REALTIME_UNCONFIRMED",
      "Connected; quote quality unconfirmed",
      /omitted a real-time status.*without guessing/i,
    ],
  ] as const)(
    "explains the exact Schwab quote-quality state %s",
    (code, label, guidance) => {
      const latest = run({
        id: `run-${code}`,
        status: "FAILED",
        error_code: code,
        consecutive_failures: 1,
      });
      const result = projectSchwabMarketDataReadiness({
        connections: [
          connection({ id: "schwab-connection", provider: "schwab" }),
        ],
        engines: [
          schwabReadinessEngine({
            schedule_completed_at: latest.completed_at ?? undefined,
            schedule_next_run_at: latest.next_run_at ?? undefined,
            consecutive_failures: 1,
            recent_runs: [latest],
          }),
        ],
      });
      expect(result.engines[0]).toMatchObject({
        state: "ENTITLEMENT_REVIEW",
        label,
        guidance: expect.stringMatching(guidance),
        errorCode: code,
      });
    },
  );

  it("keeps a supported market-session wait on course", () => {
    const latest = run({
      id: "run-session-wait",
      status: "SKIPPED",
      error_code: "OUTSIDE_SESSION",
      consecutive_failures: 0,
    });
    const result = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_status: "SKIPPED",
          schedule_completed_at: latest.completed_at ?? undefined,
          schedule_next_run_at: latest.next_run_at ?? undefined,
          consecutive_failures: 0,
          recent_runs: [latest],
        }),
      ],
    });
    expect(result).toMatchObject({ status: "READY", readyCount: 1 });
    expect(result.engines[0].state).toBe("SESSION_WAIT");
  });

  it("fails closed when the mandate market scope or latest run chain is incomplete", () => {
    const missingScope = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [schwabReadinessEngine({ market_scope_available: false })],
    });
    const mismatchedRun = projectSchwabMarketDataReadiness({
      connections: [
        connection({ id: "schwab-connection", provider: "schwab" }),
      ],
      engines: [
        schwabReadinessEngine({
          schedule_next_run_at: "2026-09-01T17:35:00Z",
        }),
      ],
    });
    expect(missingScope.status).toBe("UNAVAILABLE");
    expect(missingScope.engines[0].state).toBe("UNAVAILABLE");
    expect(mismatchedRun.status).toBe("UNAVAILABLE");
    expect(mismatchedRun.engines[0].state).toBe("UNAVAILABLE");
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
