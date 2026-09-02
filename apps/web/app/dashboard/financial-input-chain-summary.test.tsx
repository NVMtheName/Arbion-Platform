import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { FinancialInputChainProjection } from "../settings/connections/financial-continuity-center";
import { FinancialInputChainSummary } from "./financial-input-chain-summary";

const projection: FinancialInputChainProjection = {
  status: "ATTENTION",
  engineCount: 2,
  currentCount: 1,
  waitingCount: 0,
  blockedCount: 1,
  unavailableCount: 0,
  engines: [
    {
      instanceID: "coinbase-paper",
      mandateID: "coinbase-paper-mandate",
      provider: "coinbase",
      accountName: "Coinbase Portfolio",
      executionMode: "PAPER",
      state: "CURRENT",
      label: "Saved financial input chain is current",
      guidance: "The newest automatic engine completion follows the sync.",
      connectionVerifiedAt: "2026-09-02T18:00:00Z",
      accountSyncedAt: "2026-09-02T18:05:00Z",
      scheduleCompletedAt: "2026-09-02T18:10:00Z",
      nextRunAt: "2026-09-02T19:10:00Z",
      scheduleStatus: "SUCCEEDED",
    },
    {
      instanceID: "schwab-shadow",
      mandateID: "schwab-shadow-mandate",
      provider: "schwab",
      accountName: "Schwab Brokerage",
      executionMode: "SHADOW",
      state: "SAFE_BLOCKED",
      label: "Current saved input stopped safely",
      guidance: "The separate quote-quality gate stopped before AI.",
      connectionVerifiedAt: "2026-09-02T17:00:00Z",
      accountSyncedAt: "2026-09-02T17:05:00Z",
      scheduleCompletedAt: "2026-09-02T17:10:00Z",
      nextRunAt: "2026-09-02T18:10:00Z",
      scheduleStatus: "FAILED",
      errorCode: "MARKET_DATA_DELAYED",
    },
  ],
};

describe("Dashboard financial input chain", () => {
  afterEach(cleanup);

  it("keeps provider, account, mode, status, and next cycle visible", () => {
    render(<FinancialInputChainSummary projection={projection} available />);

    const region = screen.getByRole("region", {
      name: "1 AI input chain needs attention.",
    });
    expect(region).toHaveTextContent("Coinbase Portfolio");
    expect(region).toHaveTextContent("Coinbase · PAPER");
    expect(region).toHaveTextContent("Current");
    expect(region).toHaveTextContent("Schwab Brokerage");
    expect(region).toHaveTextContent("Charles Schwab · SHADOW");
    expect(region).toHaveTextContent("Stopped safely");
    expect(region).toHaveTextContent("MARKET_DATA_DELAYED");
    expect(region).toHaveTextContent("Next Sep 2, 6:10 PM UTC");
    expect(
      screen.getByRole("link", { name: /Open immutable evidence/ }),
    ).toHaveAttribute(
      "href",
      "/automations/coinbase-paper-mandate#runtime-evidence",
    );
    expect(
      screen.getByRole("link", { name: /Review input chain/ }),
    ).toHaveAttribute("href", "/connections#financial-input-chain");
    expect(
      screen.getByText(/Ordering does not prove provider cause/),
    ).toBeVisible();
  });

  it("automatically opens attention evidence but keeps current detail closed", () => {
    const { container } = render(
      <FinancialInputChainSummary projection={projection} available />,
    );
    const details = container.querySelectorAll("details");
    expect(details).toHaveLength(2);
    expect(details[0]).not.toHaveAttribute("open");
    expect(details[1]).toHaveAttribute("open");
  });

  it("fails visibly closed when the saved chain inventory is unavailable", () => {
    render(<FinancialInputChainSummary available={false} />);

    const region = screen.getByRole("region", {
      name: "Financial input status is unavailable.",
    });
    expect(region).toHaveTextContent("will not infer a healthy chain");
    expect(region).toHaveTextContent("Existing controls remain unchanged");
    expect(region).toHaveTextContent("Read-only saved evidence");
  });

  it("shows an explicit account checkpoint when no active engine is attached", () => {
    render(
      <FinancialInputChainSummary
        available
        projection={{
          status: "VERIFIED",
          engineCount: 0,
          currentCount: 0,
          waitingCount: 0,
          blockedCount: 0,
          unavailableCount: 0,
          engines: [],
        }}
        scope="account"
      />,
    );

    const region = screen.getByRole("region", {
      name: "No active AI engine uses this account.",
    });
    expect(region).toHaveTextContent("No trading activity was started");
    expect(region).toHaveTextContent("active Paper or Shadow engine");
  });

  it("uses an account-specific current-state headline", () => {
    render(
      <FinancialInputChainSummary
        available
        projection={{
          ...projection,
          status: "VERIFIED",
          currentCount: 2,
          blockedCount: 0,
          engines: projection.engines.map((engine) => ({
            ...engine,
            state: "CURRENT",
          })),
        }}
        scope="account"
      />,
    );

    expect(
      screen.getByRole("region", {
        name: "This account’s AI input chain is current.",
      }),
    ).toBeVisible();
  });
});
