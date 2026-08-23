import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
}));

import { CommandCenterDashboard } from "./command-center-dashboard";

describe("Portfolio-first command center", () => {
  afterEach(() => {
    cleanup();
    navigation.push.mockReset();
    navigation.refresh.mockReset();
  });

  it("puts connected account value and strategy launch on the home screen", () => {
    render(
      <CommandCenterDashboard
        accounts={[
          {
            id: "schwab-1",
            provider: "schwab",
            displayName: "Schwab Brokerage ••4270",
            status: "active",
            observedValue: { amount: "12500", currency: "USD" },
            cash: { amount: "2500", currency: "USD" },
            positionCount: 4,
            availability: "ready",
          },
          {
            id: "coinbase-1",
            provider: "coinbase",
            displayName: "Coinbase Advanced",
            status: "active",
            observedValue: { amount: "2750", currency: "USD" },
            cash: { amount: "125", currency: "USD" },
            positionCount: 3,
            availability: "partial",
          },
        ]}
        connectionCount={1}
        modelConfigured
        modelID="gpt-5"
        user={{
          email: "owner@example.com",
          display_name: "Nick Maya",
          entitlement: "founder",
          role: "superadmin",
        }}
      />,
    );

    expect(
      screen.getByRole("heading", { name: /your portfolio. one clear view/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("$15,250.00")).toBeInTheDocument();
    expect(screen.getByText("Schwab Brokerage ••4270")).toBeInTheDocument();
    expect(screen.getByText("Coinbase Advanced")).toBeInTheDocument();
    expect(screen.getByText("gpt-5")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /create a strategy/i }),
    ).toHaveAttribute("href", "/automations/new");
    expect(screen.queryByText(/source coverage/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/35-day activity/i)).not.toBeInTheDocument();
  });

  it("keeps the next missing connection in the primary path", () => {
    render(
      <CommandCenterDashboard
        accounts={[]}
        connectionCount={0}
        modelConfigured={false}
        user={{
          email: "owner@example.com",
          display_name: "Nick Maya",
          entitlement: "founder",
          role: "superadmin",
        }}
      />,
    );

    expect(
      screen.getAllByRole("link", { name: /connect a financial account/i })[0],
    ).toHaveAttribute("href", "/connections#financial-accounts");
    expect(
      screen.getByRole("heading", { name: "Connect. Choose. Build." }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/your accounts will appear here/i),
    ).toBeInTheDocument();
  });
});
