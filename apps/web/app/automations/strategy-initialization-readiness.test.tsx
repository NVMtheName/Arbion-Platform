import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  assessStrategyInitialization,
  StrategyInitializationReadiness,
  type StrategyInitializationInput,
} from "./strategy-initialization-readiness";

const base: StrategyInitializationInput = {
  automationType: "AI_AUTONOMOUS",
  mandateStatus: "READY",
  currentVersion: 4,
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  strategyIdentifier: "—",
  modelID: "gpt-5.6-sol",
  financialProvider: "coinbase",
  financialAccount: { id: "account-1", status: "active" },
  financialConnection: { id: "financial-1", status: "active" },
  aiConnection: {
    id: "ai-1",
    provider: "openai",
    status: "active",
    enabled: true,
  },
  capitalBucket: {
    id: "bucket-1",
    name: "AI Shadow budget",
    allocation_type: "FIXED_AMOUNT",
    allocation_value: "1000.0000000000",
    protected_amount: "100.0000000000",
    currency: "USD",
    status: "ACTIVE",
    is_reserve: false,
  },
  activeReservations: [],
  scheduleConditions: {
    enabled: true,
    interval_minutes: 60,
    session: "CONTINUOUS",
  },
  reconciliation: {
    comparison_status: "MATCHED",
    balances_status: "READY",
    positions_status: "READY",
    autonomy_signal: "CLEAR",
    autonomy_enforcement_active: true,
    blocks_new_actions: false,
    observed_at: "2026-08-28T09:00:00Z",
  },
  observedAt: "2026-08-28T10:00:00Z",
  inventoryAvailable: true,
  reconciliationAvailable: true,
};

describe("StrategyInitializationReadiness", () => {
  afterEach(cleanup);

  it("shows a fully verified AI Shadow initialization without implying execution", () => {
    const assessment = assessStrategyInitialization(base);
    render(<StrategyInitializationReadiness assessment={assessment} />);

    expect(assessment.eligible).toBe(true);
    expect(screen.getByText("READY TO INITIALIZE")).toBeInTheDocument();
    expect(screen.getByText("11/11")).toBeInTheDocument();
    expect(
      screen.getByText(/AI Shadow budget provides \$900/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Broker order submission is unavailable/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/Final admission is atomic/i)).toBeInTheDocument();
  });

  it("admits AI Paper with an exact simulated budget and no broker reconciliation", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      executionMode: "PAPER",
      reconciliation: undefined,
      reconciliationAvailable: false,
    });

    expect(assessment.eligible).toBe(true);
    expect(assessment.paperStartingCashLimit).toBe("900");
    expect(
      assessment.checks.find((check) => check.code === "AI_ROUTE")?.state,
    ).toBe("READY");
    expect(
      assessment.checks.find((check) => check.code === "BROKER_RECONCILIATION"),
    ).toMatchObject({ state: "NOT_APPLICABLE", blocking: false });
    expect(
      assessment.checks.find((check) => check.code === "EXECUTION_BOUNDARY")
        ?.detail,
    ).toMatch(/simulated portfolio state/i);
  });

  it("admits isolated AI Paper alongside an exclusive Shadow reservation", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      executionMode: "PAPER",
      reconciliation: undefined,
      reconciliationAvailable: false,
      activeReservations: [
        {
          id: "existing-shadow",
          status: "ACTIVE",
          execution_mode: "SHADOW",
          financial_account_id: "account-1",
          capital_bucket_id: "other-bucket",
          reservation_amount: "1.0000000000",
          currency: "USD",
        },
      ],
    });

    expect(assessment.eligible).toBe(true);
    expect(assessment.paperStartingCashLimit).toBe("900");
    expect(
      assessment.checks.find((check) => check.code === "ACCOUNT_CAPACITY")
        ?.detail,
    ).toMatch(/isolated from 1 active account reservation/i);
  });

  it("fails AI Paper closed when the selected model has no active provider route", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      executionMode: "PAPER",
      aiConnection: undefined,
      reconciliation: undefined,
    });

    expect(assessment.eligible).toBe(false);
    expect(
      assessment.checks.find((check) => check.code === "AI_ROUTE")?.state,
    ).toBe("BLOCKED");
  });

  it("blocks an exclusive account overlap before initialization", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      activeReservations: [
        {
          id: "existing",
          status: "ACTIVE",
          execution_mode: "SHADOW",
          financial_account_id: "account-1",
          capital_bucket_id: "other-bucket",
          reservation_amount: "1.0000000000",
          currency: "USD",
        },
      ],
    });
    render(<StrategyInitializationReadiness assessment={assessment} />);

    expect(assessment.eligible).toBe(false);
    expect(screen.getByText("BLOCKED")).toBeInTheDocument();
    const capacity = screen
      .getByRole("heading", { name: "Account reservation capacity" })
      .closest("article") as HTMLElement;
    expect(within(capacity).getByText("Blocked")).toBeInTheDocument();
    expect(capacity).toHaveTextContent("no compatible shared account ceiling");
  });

  it("calculates exact Shadow headroom without counting Paper authority", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      automationType: "STRATEGY",
      autonomyLevel: "STRATEGY_AUTONOMOUS",
      executionMode: "SHADOW",
      strategyIdentifier: "wheel",
      modelID: "—",
      aiConnection: undefined,
      financialProvider: "schwab",
      capitalBucket: {
        ...base.capitalBucket,
        allocation_value: "300.0000000000",
        protected_amount: "100.0000000000",
        allocation_limit: "1000.0000000000",
      },
      scheduleConditions: {
        enabled: true,
        interval_minutes: 60,
        session: "US_EQUITIES_REGULAR",
      },
      activeReservations: [
        {
          id: "existing",
          status: "ACTIVE",
          execution_mode: "SHADOW",
          financial_account_id: "account-1",
          capital_bucket_id: "other-bucket",
          reservation_amount: "750.0000000000",
          account_allocation_limit: "1000.0000000000",
          currency: "USD",
        },
      ],
    });

    expect(assessment.eligible).toBe(true);
    expect(assessment.paperStartingCashLimit).toBeUndefined();
    expect(
      assessment.checks.find((check) => check.code === "ACCOUNT_CAPACITY")
        ?.detail,
    ).toContain("$200 fits within $250");
  });

  it("fails closed when the complete current inventory is unavailable", () => {
    const assessment = assessStrategyInitialization({
      ...base,
      inventoryAvailable: false,
      activeReservations: [],
    });
    render(<StrategyInitializationReadiness assessment={assessment} />);

    expect(assessment.eligible).toBe(false);
    expect(screen.getByText("REFRESH REQUIRED")).toBeInTheDocument();
    expect(
      screen.getByText(
        /will not infer initialization readiness from partial data/i,
      ),
    ).toBeInTheDocument();
  });
});
