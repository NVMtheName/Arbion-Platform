import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  reconciliationFreshWithinTwentyFourHours,
  StrategyFleet,
  type StrategyFleetItem,
} from "./strategy-fleet";

const coinbaseEngine: StrategyFleetItem = {
  id: "ai-mandate",
  financialAccountID: "coinbase-account",
  title: "AI Shadow Engine",
  accountName: "Coinbase Portfolio ••••a5d0",
  provider: "coinbase",
  accountStatus: "active",
  financialConnectionAvailable: true,
  financialConnectionContextAvailable: true,
  financialConnectionStatus: "active",
  capitalContextAvailable: true,
  capitalBindingValid: true,
  capitalBucketName: "Coinbase AI Shadow",
  capitalBucketStatus: "ACTIVE",
  capitalAllocationType: "FIXED_AMOUNT",
  capitalAllocationValue: "1000.0000000000",
  capitalCurrency: "USD",
  capitalProtectedAmount: "0.0000000000",
  capitalAllocationLimit: "1000.0000000000",
  capitalReservationStatus: "ACTIVE",
  capitalReservationAmount: "1000.0000000000",
  capitalReservationCurrency: "USD",
  capitalReservationBasis: "BUCKET_FIXED_CAPACITY",
  capitalReservationAccountLimit: "1000.0000000000",
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
  evidenceAvailable: true,
  evidenceStatus: "COLLECTING_EVIDENCE",
  oneHourSampleSize: 12,
  twentyFourHourSampleSize: 4,
  minimumSamplePerHorizon: 20,
  evidenceWindowHours: 48,
  minimumEvidenceWindowHours: 168,
  evidenceScheduleHealthy: true,
  evidenceBlockers: [
    "ONE_HOUR_SAMPLE_INCOMPLETE",
    "TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE",
    "EVIDENCE_WINDOW_INCOMPLETE",
  ],
  currentEvidenceReviewed: false,
  decisionAvailable: true,
  latestDecisionType: "ABSTAIN",
  latestDecisionAt: "2026-08-26T16:17:39Z",
  latestDecisionSymbol: "NONE",
  latestDecisionAIProvider: "openai",
  latestDecisionAIModelID: "gpt-5.6-sol",
  latestDecisionAIProfile: "deep",
  latestDecisionLatencyMS: 1842,
  latestDecisionInputUsage: 12540,
  latestDecisionOutputUsage: 422,
  reconciliationAvailable: true,
  reconciliationComparisonStatus: "MATCHED",
  reconciliationBalancesStatus: "READY",
  reconciliationPositionsStatus: "READY",
  reconciliationAutonomySignal: "CLEAR",
  reconciliationAutonomyEnforcementActive: true,
  reconciliationBlocksNewActions: false,
  reconciliationBlockingChangeCount: 0,
  reconciliationObservedAt: "2026-08-26T16:10:00Z",
  reconciliationFresh: true,
};

describe("StrategyFleet", () => {
  afterEach(cleanup);

  it("uses the exact 24-hour reconciliation freshness boundary", () => {
    const now = new Date("2026-08-28T16:00:00Z");

    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-27T16:00:00Z", now),
    ).toBe(true);
    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-27T15:59:59.999Z", now),
    ).toBe(false);
    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-28T16:00:00.001Z", now),
    ).toBe(false);
    expect(reconciliationFreshWithinTwentyFourHours("invalid", now)).toBe(
      false,
    );
  });

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
    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Verified");
    expect(dataHealth).toHaveTextContent("Active account · Active connection");
    expect(dataHealth).toHaveTextContent("Balances ready · Positions ready");
    expect(dataHealth).toHaveTextContent("Fresh ≤24h");
    expect(dataHealth).toHaveTextContent("no provider read or order action");
    expect(
      within(dataHealth).getByRole("link", { name: /Account evidence/i }),
    ).toHaveAttribute(
      "href",
      "/accounts/coinbase-account#reconciliation-title",
    );
    const capitalAuthority = screen.getByRole("region", {
      name: "AI Shadow Engine capital authority",
    });
    expect(capitalAuthority).toHaveTextContent("Bounded");
    expect(capitalAuthority).toHaveTextContent("Coinbase AI Shadow");
    expect(capitalAuthority).toHaveTextContent("$1,000");
    expect(capitalAuthority).toHaveTextContent("$0");
    expect(capitalAuthority).toHaveTextContent("Fixed budget capacity");
    expect(capitalAuthority).toHaveTextContent("Shadow");
    expect(capitalAuthority).toHaveTextContent(
      "no broker custody or execution authority",
    );
    expect(
      within(capitalAuthority).getByRole("link", { name: /Capital center/i }),
    ).toHaveAttribute("href", "/capital");
    expect(screen.getByText("Covered Call")).toBeInTheDocument();
    expect(screen.getByText("Deterministic rules")).toBeInTheDocument();
    expect(screen.getByText("Draft configuration")).toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("1 owner step");
    expect(
      within(queue).getByText("Finish reviewing Covered Call"),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Review draft/i }),
    ).toHaveAttribute(
      "href",
      "/automations/rules-mandate#mandate-lifecycle-controls",
    );
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
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByText(/Review AI Shadow Engine schedule health/i),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Review schedule/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
  });

  it("fails closed and hides partial capital values when the binding is invalid", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            capitalBindingValid: false,
          },
        ]}
      />,
    );

    const capitalAuthority = screen.getByRole("region", {
      name: "AI Shadow Engine capital authority",
    });
    expect(capitalAuthority).toHaveTextContent("Review required");
    expect(capitalAuthority).toHaveTextContent(
      "hidden until the complete owner-scoped capital binding can be verified",
    );
    expect(capitalAuthority).toHaveTextContent(
      "Database controls remain enforced · no provider funds moved",
    );
    expect(capitalAuthority).not.toHaveTextContent("$1,000");
    expect(
      within(capitalAuthority).getByRole("link", {
        name: /Review capital control/i,
      }),
    ).toHaveAttribute("href", "/capital");

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent(
      "Review AI Shadow Engine capital authority",
    );
    expect(queue).toHaveTextContent("no broker funds moved");
  });

  it("surfaces exact blocking portfolio drift ahead of later lifecycle work", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationComparisonStatus: "DRIFT_DETECTED",
            reconciliationAutonomySignal: "BLOCKED",
            reconciliationBlocksNewActions: true,
            reconciliationBlockingChangeCount: 2,
          },
        ]}
      />,
    );

    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Review required");
    expect(dataHealth).toHaveTextContent("Drift Detected");
    expect(dataHealth).toHaveTextContent(
      "New AI proposals are held by portfolio evidence",
    );
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("2 blocking changes recorded");
    expect(
      within(queue).getByRole("link", {
        name: /Review portfolio evidence/i,
      }),
    ).toHaveAttribute(
      "href",
      "/accounts/coinbase-account#reconciliation-title",
    );
    expect(
      screen.getByText("Portfolio drift blocks proposals"),
    ).toBeInTheDocument();
  });

  it("fails closed when connection state or reconciliation freshness is not current", () => {
    const { rerender } = render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            financialConnectionAvailable: false,
            financialConnectionStatus: undefined,
          },
        ]}
      />,
    );

    expect(
      screen.getByText("Connection status unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Review connection/i }),
    ).toHaveAttribute("href", "/connections#financial-accounts");

    rerender(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationFresh: false,
            reconciliationObservedAt: "2026-08-24T16:10:00Z",
          },
        ]}
      />,
    );

    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Stale or invalid");
    expect(screen.getByText("Portfolio evidence is stale")).toBeInTheDocument();
    expect(
      screen.getByText(/older than the 24-hour autonomy threshold/i),
    ).toBeInTheDocument();

    rerender(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationAvailable: false,
            reconciliationComparisonStatus: undefined,
            reconciliationBalancesStatus: undefined,
            reconciliationPositionsStatus: undefined,
            reconciliationAutonomySignal: undefined,
            reconciliationAutonomyEnforcementActive: undefined,
            reconciliationBlocksNewActions: undefined,
            reconciliationObservedAt: undefined,
            reconciliationFresh: false,
          },
        ]}
      />,
    );

    expect(
      screen.getByText("Portfolio evidence unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /will not infer balances, positions, or proposal readiness/i,
      ),
    ).toBeInTheDocument();
  });

  it("shows a clear owner queue for a healthy scheduled fleet", () => {
    render(<StrategyFleet items={[coinbaseEngine]} />);

    const queue = screen.getByRole("region", {
      name: "No owner action right now.",
    });
    expect(queue).toHaveTextContent("0 owner steps");
    expect(queue).toHaveTextContent("healthy next cycle");
    expect(queue).toHaveTextContent(
      "opening a control does not run a cycle or authorize an order",
    );
    expect(within(queue).queryByRole("list")).not.toBeInTheDocument();
    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(evidence).toHaveTextContent("Collecting");
    expect(evidence).toHaveTextContent("12 / 20");
    expect(evidence).toHaveTextContent("4 / 20");
    expect(evidence).toHaveTextContent("48 / 168h");
    expect(evidence).toHaveTextContent("3 remaining conditions");
    expect(evidence).toHaveTextContent("never live authority");
    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Abstained");
    expect(pulse).toHaveTextContent("No action proposed");
    expect(pulse).toHaveTextContent("Risk gate not reached");
    expect(pulse).toHaveTextContent("No execution record");
    expect(pulse).toHaveTextContent("OpenAI · gpt-5.6-sol · Deep");
    expect(pulse).toHaveTextContent("1,842 ms · 12,540 in / 422 out");
    expect(
      within(pulse).getByRole("link", { name: /Decision journal/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#decision-journal");
  });

  it("shows a deterministic risk hold without presenting an order", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: "DENY_RISK_DENIED",
            latestDecisionSymbol: "BTC",
            latestDecisionSide: "BUY",
            latestDecisionQuantity: "0.001",
            latestDecisionRiskDecision: "DENY",
            latestDecisionRiskReasons: [
              "REPEAT_ACTION_COOLDOWN_ACTIVE",
              "MAX_TRADES_PER_DAY_REACHED",
            ],
            latestDecisionExecutionStatus: "RISK_DENIED",
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Held by controls");
    expect(pulse).toHaveTextContent("Buy · 0.001 · BTC");
    expect(pulse).toHaveTextContent("Repeat Action Cooldown Active +1");
    expect(pulse).toHaveTextContent("Risk Denied · non-live");
    expect(pulse).not.toHaveTextContent(/order submitted/i);
  });

  it("labels allowed evidence as Shadow-only would-have-submitted", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: "ALLOW_WOULD_HAVE_SUBMITTED",
            latestDecisionSymbol: "ETH",
            latestDecisionSide: "SELL",
            latestDecisionQuantity: "0.25",
            latestDecisionRiskDecision: "ALLOW",
            latestDecisionExecutionStatus: "WOULD_HAVE_SUBMITTED",
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Would have submitted");
    expect(pulse).toHaveTextContent("Sell · 0.25 · ETH");
    expect(pulse).toHaveTextContent("Allowed by deterministic controls");
    expect(pulse).toHaveTextContent("Shadow record only");
  });

  it("fails closed when the bounded decision journal is unavailable", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            decisionAvailable: false,
            latestDecisionType: undefined,
          },
        ]}
      />,
    );

    expect(screen.getByText("Latest decision unavailable")).toBeInTheDocument();
    expect(screen.queryByText("No action proposed")).not.toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /Refresh decision pulse/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
    expect(queue).toHaveTextContent("will not infer a recent AI action");
  });

  it("states when the bounded journal has no completed AI entry", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: undefined,
            latestDecisionAt: undefined,
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Awaiting a completed AI decision");
    expect(pulse).toHaveTextContent(
      "No AI entry appears in the latest 10 immutable journal records",
    );
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("keeps missing route provenance explicitly unattributed", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionAIProvider: undefined,
            latestDecisionAIModelID: undefined,
            latestDecisionAIProfile: undefined,
            latestDecisionLatencyMS: undefined,
            latestDecisionInputUsage: undefined,
            latestDecisionOutputUsage: undefined,
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Unattributed legacy route");
    expect(pulse).toHaveTextContent("Telemetry unavailable");
    expect(pulse).not.toHaveTextContent("OpenAI");
  });

  it("surfaces an exact reviewable snapshot without granting authority", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceStatus: "EVIDENCE_REVIEWABLE",
            oneHourSampleSize: 22,
            twentyFourHourSampleSize: 20,
            evidenceWindowHours: 171,
            evidenceBlockers: [],
          },
        ]}
      />,
    );

    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(evidence).toHaveTextContent("Reviewable");
    expect(evidence).toHaveTextContent("22 / 20");
    expect(evidence).toHaveTextContent("20 / 20");
    expect(evidence).toHaveTextContent("171 / 168h");
    expect(evidence).toHaveTextContent("exact gate complete");
    expect(
      within(evidence).getByRole("progressbar", {
        name: "1-hour Shadow outcome sample progress",
      }),
    ).toHaveAttribute("value", "20");
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /evidence review/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#shadow-evidence-review");
    expect(queue).toHaveTextContent("grants no trading authority");
  });

  it("does not keep a currently reviewed snapshot in the owner queue", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceStatus: "EVIDENCE_REVIEWABLE",
            oneHourSampleSize: 20,
            twentyFourHourSampleSize: 20,
            evidenceWindowHours: 168,
            evidenceBlockers: [],
            currentEvidenceReviewed: true,
          },
        ]}
      />,
    );

    expect(screen.getByText("Current snapshot reviewed")).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("fails closed when the immutable evidence scorecard is unavailable", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceAvailable: false,
            evidenceStatus: undefined,
          },
        ]}
      />,
    );

    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(
      within(evidence).getByText("Evidence status unavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/12 \/ 20/)).not.toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /Refresh evidence/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
    expect(queue).toHaveTextContent(
      "will not infer its sample or review status",
    );
  });

  it("orders failed schedules before draft and paused owner choices", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            id: "paused",
            accountName: "Paused account",
            instanceStatus: "PAUSED",
            scheduleEnabled: false,
            nextRunAt: undefined,
          },
          {
            ...coinbaseEngine,
            id: "draft",
            accountName: "Draft account",
            mandateStatus: "DRAFT",
            instanceStatus: undefined,
            currentState: undefined,
            scheduleAvailable: undefined,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
          {
            ...coinbaseEngine,
            id: "failed",
            accountName: "Failed account",
            scheduleStatus: "FAILED",
            consecutiveFailures: 2,
          },
        ]}
      />,
    );

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    const actions = within(queue).getAllByRole("listitem");
    expect(actions).toHaveLength(3);
    expect(actions[0]).toHaveTextContent(
      "Review AI Shadow Engine schedule health",
    );
    expect(actions[1]).toHaveTextContent("Finish reviewing AI Shadow Engine");
    expect(actions[2]).toHaveTextContent(
      "Decide when to resume AI Shadow Engine",
    );
  });

  it("does not mislabel a completed immutable version as ready to initialize", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            instanceStatus: "COMPLETED",
            currentState: "AI_MONITORING",
            scheduleAvailable: undefined,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
        ]}
      />,
    );

    expect(screen.getByText("New version required")).toBeInTheDocument();
    expect(screen.getByText("Historical version complete")).toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /version controls/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#configuration-controls");
  });

  it("collapses repeated partial-context warnings into one fail-closed action", () => {
    render(
      <StrategyFleet
        contextWarnings={["Current engine state could not be refreshed."]}
        items={[
          {
            ...coinbaseEngine,
            id: "first",
            instanceContextAvailable: false,
          },
          {
            ...coinbaseEngine,
            id: "second",
            accountName: "Second account",
            instanceContextAvailable: false,
          },
        ]}
      />,
    );

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(within(queue).getAllByRole("listitem")).toHaveLength(1);
    expect(
      within(queue).getByText("Refresh the current fleet context"),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Refresh automations/i }),
    ).toHaveAttribute("href", "/automations");
    expect(queue).toHaveTextContent("No mandate or schedule was changed");
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
