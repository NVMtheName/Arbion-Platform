import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
}));

import { CommandCenterDashboard } from "./command-center-dashboard";

describe("Trading command center dashboard", () => {
  afterEach(() => {
    cleanup();
    navigation.push.mockReset();
    navigation.refresh.mockReset();
  });

  it("renders real source and decision status without claiming execution", () => {
    render(
      <CommandCenterDashboard
        accountCount={1}
        asOf="2026-08-20T18:00:00.000Z"
        connectionCount={2}
        journalEntries={[
          {
            id: "decision-1",
            created_at: "2026-08-20T15:00:00.000Z",
            strategy_instance_id: "instance-1",
            financial_account_id: "account-1",
            account_display_name: "Brokerage",
            mandate_id: "mandate-1",
            mandate_version: 1,
            strategy_identifier: "wheel",
            execution_mode: "PAPER",
            strategy_state: "ACTIVE",
            source: "manual",
            decision_type: "HOLD",
            structured_rationale: {},
            risk_decision: "DENY",
          },
        ]}
        sources={[
          {
            id: "alpaca_iex",
            label: "Alpaca IEX",
            role: "market_observation",
            feed: "iex",
            quality: "real_time_single_venue",
            capabilities: ["equity_quote"],
            enabled: true,
            healthy: true,
          },
          {
            id: "sec_edgar",
            label: "SEC EDGAR",
            role: "primary_filing",
            feed: "ownership_xml",
            quality: "filing",
            capabilities: ["insider_filing"],
            enabled: false,
            healthy: false,
          },
        ]}
        user={{
          email: "owner@example.com",
          display_name: "Nick Maya",
          entitlement: "founder",
          role: "superadmin",
        }}
      >
        <div>Advisory panel</div>
      </CommandCenterDashboard>,
    );

    expect(
      screen.getByRole("heading", { name: /welcome back, nick/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("Alpaca IEX")).toBeInTheDocument();
    expect(screen.getByText("SEC EDGAR")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByText("None")).toBeInTheDocument();
    expect(
      screen.getByRole("img", { name: /1 recent decision loaded/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("Advisory panel")).toBeInTheDocument();
  });
});
