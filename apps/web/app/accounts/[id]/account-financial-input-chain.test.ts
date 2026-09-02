import { afterEach, describe, expect, it, vi } from "vitest";

import { loadAccountFinancialInputChain } from "./account-financial-input-chain";

const base = "http://arbion-api";
const headers = { cookie: "session=owner" };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

describe("Account financial input chain loader", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("loads only the selected account's exact active non-live engine chain", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url === `${base}/api/connections/financial`) {
        return json({
          connections: [
            {
              id: "connection-1",
              provider: "coinbase",
              display_name: "Coinbase",
              status: "active",
              runtime_protected: true,
              protected_mandate_count: 1,
              active_strategy_count: 1,
              last_synced_at: "2026-09-02T18:00:00Z",
            },
          ],
        });
      }
      if (url === `${base}/api/accounts`) {
        return json({
          accounts: [
            {
              id: "account-1",
              provider_connection_id: "connection-1",
              provider: "coinbase",
              display_name: "Coinbase Portfolio",
              status: "active",
              last_synced_at: "2026-09-02T18:05:00Z",
            },
            {
              id: "account-2",
              provider_connection_id: "connection-2",
              provider: "schwab",
              display_name: "Schwab Brokerage",
              status: "active",
              last_synced_at: "2026-09-02T18:05:00Z",
            },
          ],
        });
      }
      if (url === `${base}/api/strategy-instances`) {
        return json({
          strategy_instances: [
            {
              id: "engine-1",
              automation_mandate_id: "mandate-1",
              financial_account_id: "account-1",
              strategy_identifier: "ai_shadow",
              execution_mode: "PAPER",
              status: "ACTIVE",
              current_state: "AI_MONITORING",
            },
            {
              id: "engine-2",
              automation_mandate_id: "mandate-2",
              financial_account_id: "account-2",
              strategy_identifier: "ai_shadow",
              execution_mode: "SHADOW",
              status: "ACTIVE",
              current_state: "AI_MONITORING",
            },
          ],
        });
      }
      if (url === `${base}/api/strategy-instances/engine-1/schedule`) {
        return json({
          schedule: {
            last_status: "SUCCEEDED",
            last_completed_at: "2026-09-02T18:10:00Z",
            next_run_at: "2026-09-02T19:10:00Z",
            consecutive_failures: 0,
          },
        });
      }
      if (
        url === `${base}/api/strategy-instances/engine-1/schedule-runs?limit=12`
      ) {
        return json({
          history_semantics: "IMMUTABLE_NONLIVE_SCHEDULER_EVIDENCE",
          broker_action_available: false,
          live_execution_available: false,
          runs: [
            {
              id: "run-1",
              scheduled_for: "2026-09-02T18:09:00Z",
              completed_at: "2026-09-02T18:10:00Z",
              next_run_at: "2026-09-02T19:10:00Z",
              status: "SUCCEEDED",
              error_code: null,
              duplicate_recovered: false,
              consecutive_failures: 0,
            },
          ],
        });
      }
      return json({ error: "unexpected request" }, 500);
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await loadAccountFinancialInputChain({
      accountID: "account-1",
      base,
      headers,
      observedAt: new Date("2026-09-02T18:20:00Z"),
    });

    expect(result.available).toBe(true);
    expect(result.unauthorized).toBe(false);
    expect(result.projection).toMatchObject({
      status: "VERIFIED",
      engineCount: 1,
      currentCount: 1,
      engines: [
        {
          instanceID: "engine-1",
          mandateID: "mandate-1",
          accountName: "Coinbase Portfolio",
          provider: "coinbase",
          executionMode: "PAPER",
          state: "CURRENT",
        },
      ],
    });
    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.stringContaining("engine-2"),
      expect.anything(),
    );
  });

  it("fails closed when the owner inventory is incomplete", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) =>
        String(input).endsWith("/api/accounts")
          ? json({ accounts: [] })
          : json({ connections: [], strategy_instances: [] }),
      ),
    );

    const result = await loadAccountFinancialInputChain({
      accountID: "missing-account",
      base,
      headers,
      observedAt: new Date("2026-09-02T18:20:00Z"),
    });

    expect(result).toEqual({ available: false, unauthorized: false });
  });

  it("propagates expired owner authentication without exposing partial data", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json({ error: "unauthorized" }, 401)),
    );

    const result = await loadAccountFinancialInputChain({
      accountID: "account-1",
      base,
      headers,
      observedAt: new Date("2026-09-02T18:20:00Z"),
    });

    expect(result).toEqual({ available: false, unauthorized: true });
  });
});
