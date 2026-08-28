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
        aiEngines={[
          {
            id: "engine-1",
            mandateID: "mandate-1",
            accountName: "Coinbase Advanced",
            provider: "coinbase",
            status: "ACTIVE",
            currentState: "AI_MONITORING",
            executionMode: "SHADOW",
            modelID: "gpt-5.6-sol",
            lastEvaluatedAt: "2026-08-26T15:17:14Z",
            nextRunAt: "2026-08-26T16:17:14Z",
            scheduleStatus: "SUCCEEDED",
            scheduleAvailable: true,
            journalAvailable: true,
            consecutiveFailures: 0,
            lastDecision: "ALLOW_WOULD_HAVE_SUBMITTED",
            lastDecisionSymbol: "XRP",
            lastDecisionAt: "2026-08-26T15:17:14Z",
            evidenceAvailable: true,
            evidenceStatus: "COLLECTING_EVIDENCE",
            evidenceBlockers: [
              "ONE_HOUR_SAMPLE_INCOMPLETE",
              "TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE",
              "EVIDENCE_WINDOW_INCOMPLETE",
            ],
            oneHourSampleSize: 8,
            twentyFourHourSampleSize: 4,
            minimumSamplePerHorizon: 20,
            evidenceWindowHours: 72,
            minimumEvidenceWindowHours: 168,
          },
        ]}
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
    expect(screen.getAllByText("Coinbase Advanced")).toHaveLength(2);
    expect(screen.getByText("Provider data partial")).toBeInTheDocument();
    expect(screen.getByText("gpt-5")).toBeInTheDocument();
    const cockpit = screen.getByRole("region", {
      name: "AI oversight at a glance.",
    });
    expect(cockpit).toHaveTextContent("gpt-5.6-sol");
    expect(cockpit).toHaveTextContent("Would have submitted · XRP");
    expect(cockpit).toHaveTextContent("Healthy schedule");
    expect(cockpit).toHaveTextContent("Shadow only");
    expect(cockpit).toHaveTextContent("Collecting evidence");
    expect(cockpit).toHaveTextContent(
      "8/20 one-hour · 4/20 24-hour · 72h/168h window",
    );
    expect(cockpit).toHaveTextContent("Collect more 1-hour outcome marks");
    expect(cockpit).toHaveTextContent(
      "Observe the mandate across a longer window",
    );
    expect(cockpit).toHaveTextContent("No broker order can be sent");
    expect(
      screen.getByRole("link", { name: "Review journal →" }),
    ).toHaveAttribute("href", "/automations/mandate-1");
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
    expect(
      screen.getByText("No AI Shadow Engine is monitoring yet."),
    ).toBeInTheDocument();
  });

  it("surfaces unavailable AI evidence instead of implying a healthy engine", () => {
    render(
      <CommandCenterDashboard
        accounts={[]}
        connectionCount={1}
        modelConfigured
        aiEngines={[
          {
            id: "engine-1",
            mandateID: "mandate-1",
            accountName: "Schwab Brokerage",
            provider: "schwab",
            status: "ACTIVE",
            currentState: "AI_MONITORING",
            executionMode: "SHADOW",
            scheduleAvailable: false,
            journalAvailable: false,
            consecutiveFailures: 0,
          },
        ]}
        user={{
          email: "owner@example.com",
          display_name: "Nick Maya",
          entitlement: "founder",
          role: "superadmin",
        }}
      />,
    );

    const cockpit = screen.getByRole("region", {
      name: "AI oversight at a glance.",
    });
    expect(cockpit).toHaveTextContent("Decision journal unavailable");
    expect(cockpit).not.toHaveTextContent("Healthy schedule");
    expect(cockpit).toHaveTextContent("Evidence unavailable");
  });
});
