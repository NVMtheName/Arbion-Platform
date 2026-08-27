import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  PortfolioReconciliationPanel,
  type PortfolioReconciliation,
} from "./portfolio-reconciliation-panel";

const baseline: PortfolioReconciliation = {
  id: "reconciliation-1",
  financial_account_id: "account-1",
  provider: "coinbase",
  comparison_status: "BASELINE",
  balances_status: "READY",
  positions_status: "READY",
  performance_status: "UNAVAILABLE",
  realized_performance_status: "UNAVAILABLE",
  autonomy_signal: "INSUFFICIENT_EVIDENCE",
  autonomy_enforcement_active: true,
  blocks_new_actions: true,
  observed_position_count: 3,
  performance_position_count: 0,
  change_count: 0,
  blocking_change_count: 0,
  changes: [],
  evidence_hash: "a".repeat(64),
  observed_at: "2026-08-26T18:00:00Z",
};

describe("PortfolioReconciliationPanel", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("captures the first immutable baseline without implying realized P&L", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ reconciliation: baseline }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <PortfolioReconciliationPanel
        accountID="account-1"
        accountName="Coinbase Portfolio"
      />,
    );

    expect(screen.getByText("No immutable baseline yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Reconcile now" }));
    await waitFor(() =>
      expect(screen.getByText("Baseline captured")).toBeInTheDocument(),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/accounts/account-1/reconciliations",
      expect.objectContaining({ method: "POST" }),
    );
    expect(screen.getByText("Realized P&L")).toBeInTheDocument();
    expect(
      screen.getByText("Never inferred from partial history"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/cannot submit an order, change a holding, or/i),
    ).toBeInTheDocument();
    expect(screen.getByText("AI proposals held")).toBeInTheDocument();
  });

  it("surfaces exact quantity drift and holds new AI proposals", () => {
    render(
      <PortfolioReconciliationPanel
        accountID="account-1"
        accountName="Coinbase Portfolio"
        initialReport={{
          ...baseline,
          id: "reconciliation-2",
          comparison_status: "DRIFT_DETECTED",
          autonomy_signal: "REVIEW_RECOMMENDED",
          blocks_new_actions: true,
          change_count: 1,
          blocking_change_count: 1,
          changes: [
            {
              symbol: "BTC",
              instrument_type: "CRYPTO",
              direction: "long",
              change_type: "QUANTITY_CHANGED",
              control_impact: "TRADABLE_INVENTORY",
              previous_quantity: "0.1",
              current_quantity: "0.2",
            },
          ],
        }}
      />,
    );

    expect(screen.getByText("Position change detected")).toBeInTheDocument();
    expect(screen.getByText("Review recommended")).toBeInTheDocument();
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(screen.getByText("0.1 → 0.2")).toBeInTheDocument();
    expect(
      screen.getByText(/This gate can hold new AI proposals/i),
    ).toBeInTheDocument();
  });

  it("shows exact unavailable-only movement without presenting it as tradable drift", async () => {
    const report: PortfolioReconciliation = {
      ...baseline,
      id: "reconciliation-3",
      comparison_status: "MATCHED",
      autonomy_signal: "CLEAR",
      blocks_new_actions: false,
      change_count: 1,
      blocking_change_count: 0,
      changes: [
        {
          symbol: "USDC",
          instrument_type: "CRYPTO",
          direction: "long",
          change_type: "QUANTITY_CHANGED",
          control_impact: "NON_TRADABLE_QUANTITY_ONLY",
          previous_quantity: "17548.529979",
          current_quantity: "17548.557979",
          previous_available_quantity: "10.705979",
          current_available_quantity: "10.705979",
          previous_unavailable_quantity: "17537.824",
          current_unavailable_quantity: "17537.852",
        },
      ],
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ reconciliation: report }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <PortfolioReconciliationPanel
        accountID="account-1"
        accountName="Coinbase Portfolio"
        initialReport={baseline}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Reconcile now" }));
    await waitFor(() =>
      expect(screen.getByText("AI proposal gate clear")).toBeInTheDocument(),
    );
    expect(screen.getByText("Trading inventory matched")).toBeInTheDocument();
    expect(
      screen.getByText(/Available to trade unchanged at 10\.705979/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/Recorded, non-blocking/i)).toBeInTheDocument();
    expect(
      screen.getByText(/An exact unavailable-to-trade movement was recorded/i),
    ).toBeInTheDocument();
  });
});
