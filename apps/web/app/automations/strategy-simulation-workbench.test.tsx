import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: navigation.refresh }),
}));

import { StrategySimulationWorkbench } from "./strategy-simulation-workbench";

const versions = [
  {
    VersionNumber: 1,
    CreatedAt: "2026-08-26T14:00:00Z",
    Source: "UI",
    Snapshot: {
      status: "DRAFT",
      automation_type: "AI_AUTONOMOUS",
      ai_model_id: "gpt-5.6-sol",
      capital_bucket_id: "bucket-old",
      autonomy_level: "FULL_AUTONOMOUS",
      execution_mode: "SHADOW",
      allowed_universe: { symbols: ["SPY"] },
      strategy_parameters: {
        objective: "Observe SPY conservatively.",
        max_proposal_notional: "1",
      },
      risk_parameters: { max_trades_per_day: 1 },
      schedule_conditions: {
        enabled: false,
        interval_minutes: 60,
        session: "US_EQUITIES_REGULAR",
      },
    },
  },
  {
    VersionNumber: 2,
    CreatedAt: "2026-08-27T14:00:00Z",
    Source: "UI",
    Snapshot: {
      status: "READY",
      automation_type: "AI_AUTONOMOUS",
      ai_model_id: "gpt-5.6-sol",
      capital_bucket_id: "bucket-current",
      autonomy_level: "FULL_AUTONOMOUS",
      execution_mode: "SHADOW",
      allowed_universe: { symbols: ["SPY"] },
      strategy_parameters: {
        objective: "Observe SPY with a one-share-capable envelope.",
        max_proposal_notional: "1000",
      },
      risk_parameters: { max_trades_per_day: 1 },
      schedule_conditions: {
        enabled: true,
        interval_minutes: 60,
        session: "US_EQUITIES_REGULAR",
      },
    },
  },
];

const decisions = [
  {
    Source: "AI",
    StructuredRationale: {
      input_evidence: {
        markets: [
          {
            symbol: "SPY",
            mark: "500",
            feed: "schwab_market_data",
            quality: "broker_realtime",
            observed_at: "2026-08-27T15:59:00Z",
          },
        ],
      },
    },
  },
];

describe("Strategy Simulation Workbench", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("compares immutable versions and calculates a local recorded-price scenario", () => {
    render(
      <StrategySimulationWorkbench
        capitalBucket={{
          Name: "Schwab AI allocation",
          AllocationType: "FIXED_AMOUNT",
          AllocationValue: "1000",
          ProtectedAmount: "0",
          Status: "ACTIVE",
        }}
        currentVersion={2}
        decisions={decisions}
        versions={versions}
      />,
    );

    expect(screen.getByText("Version 1 → Version 2")).toBeInTheDocument();
    const comparison = screen.getByRole("table");
    const ceilingRow = within(comparison)
      .getByText("Per-decision ceiling")
      .closest<HTMLElement>('[role="row"]');
    expect(ceilingRow).not.toBeNull();
    expect(within(ceilingRow!).getByText("$1.00")).toBeInTheDocument();
    expect(within(ceilingRow!).getByText("$1,000.00")).toBeInTheDocument();

    const results = screen.getByText("PER-DECISION ENVELOPE").parentElement;
    expect(results).not.toBeNull();
    expect(within(results!).getByText("$1,000.00")).toBeInTheDocument();
    expect(screen.getByText("2 SPY")).toBeInTheDocument();
    expect(screen.getByText("100% of current capacity")).toBeInTheDocument();
    expect(screen.getByText("WITHIN STATIC CAPACITY")).toBeInTheDocument();
    expect(
      screen.getByText(/Schwab Market Data · Broker Realtime/),
    ).toBeInTheDocument();
  });

  it("updates and resets a what-if without mutating the immutable version", () => {
    render(
      <StrategySimulationWorkbench
        capitalBucket={{
          Name: "Schwab AI allocation",
          AllocationType: "FIXED_AMOUNT",
          AllocationValue: "1000",
          ProtectedAmount: "0",
        }}
        currentVersion={2}
        decisions={decisions}
        versions={versions}
      />,
    );

    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario per-decision ceiling",
      }),
      { target: { value: "500" } },
    );
    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario maximum decisions per day",
      }),
      { target: { value: "2" } },
    );

    expect(screen.getByText("1 SPY")).toBeInTheDocument();
    expect(
      screen.getByText(/2 local changes from version 2/),
    ).toBeInTheDocument();
    expect(
      within(
        screen.getByText("DAILY RESEARCH NOTIONAL").closest("article")!,
      ).getByText("$1,000.00", { selector: "strong" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Reset to version 2" }));
    expect(
      screen.getByRole("spinbutton", {
        name: "Scenario per-decision ceiling",
      }),
    ).toHaveValue(1000);
    expect(screen.getByText("2 SPY")).toBeInTheDocument();
  });

  it("does not invent absolute capacity or market evidence", () => {
    render(
      <StrategySimulationWorkbench
        capitalBucket={{
          Name: "Percentage allocation",
          AllocationType: "PERCENT_OF_AVAILABLE_CASH",
          AllocationValue: "10",
          ProtectedAmount: "0",
        }}
        currentVersion={2}
        versions={versions}
      />,
    );

    expect(screen.getByText("Provider-relative")).toBeInTheDocument();
    expect(
      screen.getByText("Cannot calculate an absolute ratio"),
    ).toBeInTheDocument();
    expect(screen.getByText("PROVIDER FACTS REQUIRED")).toBeInTheDocument();
    expect(
      screen.getByText(/does not substitute a live or estimated price/),
    ).toBeInTheDocument();
  });

  it("keeps large exact boundaries out of browser floating-point math", () => {
    const exactVersions = structuredClone(versions);
    exactVersions[1].Snapshot.strategy_parameters.max_proposal_notional =
      "9007199254740993.01";
    render(
      <StrategySimulationWorkbench
        automationId="mandate-exact"
        capitalBucket={{
          Name: "Exact capital policy",
          AllocationType: "FIXED_AMOUNT",
          AllocationValue: "9007199254740993.01",
          ProtectedAmount: "0.01",
        }}
        currentVersion={2}
        decisions={[
          {
            Source: "AI",
            StructuredRationale: {
              input_evidence: {
                markets: [
                  {
                    symbol: "SPY",
                    mark: "0.1",
                    feed: "schwab_market_data",
                    quality: "broker_realtime",
                  },
                ],
              },
            },
          },
        ]}
        status="READY"
        versions={exactVersions}
      />,
    );

    expect(
      screen.getAllByText("$9,007,199,254,740,993.01").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("$9,007,199,254,740,993.00")).toBeInTheDocument();
    expect(
      screen.getByText("90,071,992,547,409,930.1 SPY"),
    ).toBeInTheDocument();
    expect(screen.getByText("EXCEEDS STATIC CAPACITY")).toBeInTheDocument();

    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario per-decision ceiling",
      }),
      { target: { value: "9007199254740993.0100" } },
    );
    expect(
      screen.getByRole("button", { name: "Review as new DRAFT" }),
    ).toBeDisabled();
  });

  it("keeps an honest empty state before the first mandate version", () => {
    render(<StrategySimulationWorkbench currentVersion={0} versions={[]} />);
    expect(
      screen.getByText("No immutable mandate version is available."),
    ).toBeInTheDocument();
  });

  it("creates only a confirmed immutable draft from the two reviewed limits", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategySimulationWorkbench
        automationId="mandate-1"
        capitalBucket={{
          Name: "Schwab AI allocation",
          AllocationType: "FIXED_AMOUNT",
          AllocationValue: "1000",
          ProtectedAmount: "0",
        }}
        currentVersion={2}
        decisions={decisions}
        hasActiveInstance
        status="READY"
        versions={versions}
      />,
    );

    const reviewButton = screen.getByRole("button", {
      name: "Review as new DRAFT",
    });
    expect(reviewButton).toBeDisabled();
    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario per-decision ceiling",
      }),
      { target: { value: "500" } },
    );
    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario maximum decisions per day",
      }),
      { target: { value: "2" } },
    );
    expect(reviewButton).toBeEnabled();
    fireEvent.click(reviewButton);

    const review = screen.getByRole("table", {
      name: "Scenario draft changes",
    });
    expect(within(review).getByText("$500.00")).toBeInTheDocument();
    expect(
      screen.getByText(/continues on that exact schedule/i),
    ).toBeInTheDocument();
    const createButton = screen.getByRole("button", { name: "Create DRAFT" });
    expect(createButton).toBeDisabled();
    fireEvent.click(
      screen.getByLabelText(/I reviewed both changes and understand/i),
    );
    fireEvent.click(createButton);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/ai-shadow-scenario-draft",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: 2,
          max_proposal_notional: "500",
          max_trades_per_day: 2,
          confirm: true,
        }),
      },
    );
    expect(
      await screen.findByText(/Version 3 was created as an immutable DRAFT/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("keeps archived mandates and stale reviewed versions from creating drafts", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: { code: "VERSION_CONFLICT" } }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    const { rerender } = render(
      <StrategySimulationWorkbench
        automationId="mandate-1"
        currentVersion={2}
        status="ARCHIVED"
        versions={versions}
      />,
    );
    fireEvent.change(
      screen.getByRole("spinbutton", {
        name: "Scenario per-decision ceiling",
      }),
      { target: { value: "500" } },
    );
    expect(
      screen.getByRole("button", { name: "Review as new DRAFT" }),
    ).toBeDisabled();

    rerender(
      <StrategySimulationWorkbench
        automationId="mandate-1"
        currentVersion={2}
        status="READY"
        versions={versions}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Review as new DRAFT" }),
    );
    fireEvent.click(
      screen.getByLabelText(/I reviewed both changes and understand/i),
    );
    fireEvent.click(screen.getByRole("button", { name: "Create DRAFT" }));
    expect(
      await screen.findByText(/mandate changed while you were reviewing/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
