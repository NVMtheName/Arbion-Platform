import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { StrategyFleet, type StrategyFleetItem } from "./strategy-fleet";

const coinbaseEngine: StrategyFleetItem = {
  id: "ai-mandate",
  title: "AI Shadow Engine",
  accountName: "Coinbase Portfolio ••••a5d0",
  provider: "coinbase",
  automationType: "AI_AUTONOMOUS",
  mandateStatus: "READY",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  modelID: "gpt-5.6-sol",
  symbols: ["BTC", "ETH", "XRP", "SOL"],
  instanceStatus: "ACTIVE",
  currentState: "AI_MONITORING",
  lastEvaluatedAt: "2026-08-26T16:17:39Z",
  scheduleAvailable: true,
  scheduleEnabled: true,
  scheduleStatus: "SUCCEEDED",
  consecutiveFailures: 0,
  nextRunAt: "2026-08-26T17:17:39Z",
};

describe("StrategyFleet", () => {
  afterEach(cleanup);

  it("shows an owner-facing fleet summary with account and engine context", () => {
    render(
      <StrategyFleet
        items={[
          coinbaseEngine,
          {
            id: "rules-mandate",
            title: "Covered Call",
            accountName: "Schwab Brokerage ••9555",
            provider: "schwab",
            automationType: "RULES_BASED",
            mandateStatus: "DRAFT",
            autonomyLevel: "CONFIRM_EACH",
            executionMode: "PAPER",
            symbols: ["AAPL"],
            consecutiveFailures: 0,
          },
        ]}
      />,
    );

    const summary = screen.getByRole("region", { name: "Fleet summary" });
    expect(summary).toHaveTextContent("Monitoring1AI shadow engines");
    expect(summary).toHaveTextContent("Scheduled1healthy automatic cycles");
    expect(summary).toHaveTextContent("Attention0engine health signals");
    expect(summary).toHaveTextContent("Drafts1not initialized");
    expect(screen.getByText("Coinbase Portfolio ••••a5d0")).toBeInTheDocument();
    expect(screen.getByText("gpt-5.6-sol")).toBeInTheDocument();
    expect(screen.getByText("BTC · ETH · XRP +1")).toBeInTheDocument();
    expect(screen.getByText("Healthy schedule")).toBeInTheDocument();
    expect(screen.getByText("Covered Call")).toBeInTheDocument();
    expect(screen.getByText("Deterministic rules")).toBeInTheDocument();
    expect(screen.getByText("Draft configuration")).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "Execution boundary" }),
    ).toHaveTextContent("Neither mode can submit a broker order");
    expect(
      screen.getByRole("link", {
        name: "Open AI Shadow Engine for Coinbase Portfolio ••••a5d0",
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
  });

  it("surfaces a schedule outage instead of presenting healthy automation", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            scheduleAvailable: false,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
        ]}
      />,
    );

    const fleet = screen.getByRole("region", { name: "Strategy fleet" });
    expect(within(fleet).getByText("Needs review")).toBeInTheDocument();
    expect(
      within(fleet).getByText("Schedule status unavailable"),
    ).toBeInTheDocument();
    expect(
      within(fleet).queryByText("Healthy schedule"),
    ).not.toBeInTheDocument();
  });

  it("keeps the empty state focused on a bounded shadow launch", () => {
    render(<StrategyFleet items={[]} />);

    expect(screen.getByText("No strategies yet.")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Launch an AI Shadow Engine" }),
    ).toHaveAttribute("href", "/automations/new");
  });

  it("does not present an inventory outage as an empty fleet", () => {
    render(
      <StrategyFleet
        contextWarnings={["Current engine state could not be refreshed."]}
        inventoryAvailable={false}
        items={[]}
      />,
    );

    expect(
      screen.getByText("Strategies could not be loaded."),
    ).toBeInTheDocument();
    expect(screen.queryByText("No strategies yet.")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Return to dashboard" }),
    ).toHaveAttribute("href", "/dashboard");
  });
});
