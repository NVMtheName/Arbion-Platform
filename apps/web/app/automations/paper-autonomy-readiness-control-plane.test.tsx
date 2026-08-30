import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { PaperAutonomyReadinessControlPlane } from "./paper-autonomy-readiness-control-plane";

const base = {
  provider: "coinbase",
  modelID: "gpt-5.6-sol",
  mandateStatus: "READY",
  currentVersion: 3,
  automationType: "AI_AUTONOMOUS",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "PAPER",
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
    status: "ACTIVE",
    is_reserve: false,
    allocation_value: "1000.0000000000",
    currency: "USD",
  },
  capitalReservation: {
    id: "reservation-1",
    strategy_instance_id: "instance-1",
    financial_account_id: "account-1",
    capital_bucket_id: "bucket-1",
    execution_mode: "PAPER",
    reservation_amount: "1000.0000000000",
    currency: "USD",
    reservation_basis: "PAPER_STARTING_CASH",
    status: "ACTIVE",
  },
  instance: {
    id: "instance-1",
    status: "ACTIVE",
    current_state: "AI_MONITORING",
    execution_mode: "PAPER",
  },
  schedule: {
    enabled: true,
    last_status: "SUCCEEDED",
    consecutive_failures: 0,
    next_run_at: "2026-08-30T04:28:57Z",
  },
  paperPortfolio: {
    strategy_instance_id: "instance-1",
    currency: "USD",
    starting_cash: "1000.0000000000",
    cash: "975.0000000000",
    version: 2,
    positions: [],
    updated_at: "2026-08-29T21:27:00Z",
  },
  automationBreaker: null,
  schedulerEnabled: true,
  allowedSymbols: ["BTC", "ETH", "XRP"],
  decisions: [
    {
      id: "decision-1",
      source: "AI",
      decision_type: "ABSTAIN",
      structured_rationale: {
        ai_provider: "openai",
        model_id: "gpt-5.6-sol",
        profile: "deep",
        decision: "ABSTAIN",
        proposed_notional: "0",
        thesis: "Mixed momentum does not support a cautious entry.",
        risk_flags: ["Negative short-term momentum"],
        latency_ms: 4301,
        input_usage: 1725,
        output_usage: 237,
        input_evidence: {
          provider: "coinbase",
          markets: [{ symbol: "BTC" }, { symbol: "ETH" }, { symbol: "XRP" }],
          recent_decisions: [],
        },
      },
    },
  ],
};

describe("Paper autonomy readiness control plane", () => {
  afterEach(cleanup);

  it("verifies one exact isolated Paper engine without implying broker access", () => {
    render(<PaperAutonomyReadinessControlPlane {...base} />);

    expect(screen.getByText("PAPER VERIFIED")).toBeInTheDocument();
    expect(screen.getByText("10/10")).toBeInTheDocument();
    expect(screen.getByText(/\$1,000\.00 starting cash/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Broker-write capability remains absent/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/cannot submit, replace, cancel, or route/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/4301 ms · 1725\/237 tokens/i)).toBeInTheDocument();
    expect(
      screen.getByText(/stopped before risk evaluation/i),
    ).toBeInTheDocument();
  });

  it("shows a bounded scheduler skip as monitoring instead of an outage", () => {
    render(
      <PaperAutonomyReadinessControlPlane
        {...base}
        schedule={{
          ...base.schedule,
          last_status: "SKIPPED",
          last_error_code: "AI_DECISION_BUDGET_EXHAUSTED",
        }}
      />,
    );

    expect(screen.getByText("PAPER MONITORING")).toBeInTheDocument();
    expect(screen.getByText("9/10")).toBeInTheDocument();
    expect(screen.getByText(/Skipped safely/i)).toBeInTheDocument();
    expect(
      screen.getByText(/AI_DECISION_BUDGET_EXHAUSTED/i),
    ).toBeInTheDocument();
  });

  it("fails closed when the reservation and Paper ledger do not match", () => {
    render(
      <PaperAutonomyReadinessControlPlane
        {...base}
        capitalReservation={{
          ...base.capitalReservation,
          reservation_amount: "999.9999999999",
        }}
      />,
    );

    expect(screen.getByText("REVIEW REQUIRED")).toBeInTheDocument();
    expect(screen.getByText("9/10")).toBeInTheDocument();
    expect(
      screen.getByText(/do not form one exact isolated ledger/i),
    ).toBeInTheDocument();
  });

  it("rejects non-canonical capital evidence that JavaScript would coerce", () => {
    render(
      <PaperAutonomyReadinessControlPlane
        {...base}
        capitalBucket={{
          ...base.capitalBucket,
          allocation_value: "1e3",
        }}
      />,
    );

    expect(screen.getByText("REVIEW REQUIRED")).toBeInTheDocument();
    expect(screen.getByText("8/10")).toBeInTheDocument();
    expect(screen.getByText(/exact, active, positive/i)).toBeInTheDocument();
  });

  it("fails closed when the newest decision route or market universe drifts", () => {
    render(
      <PaperAutonomyReadinessControlPlane
        {...base}
        decisions={[
          {
            ...base.decisions[0],
            structured_rationale: {
              ...base.decisions[0].structured_rationale,
              model_id: "unexpected-model",
              input_evidence: {
                provider: "coinbase",
                markets: [{ symbol: "BTC" }, { symbol: "DOGE" }],
                recent_decisions: [],
              },
            },
          },
        ]}
      />,
    );

    expect(screen.getByText("REVIEW REQUIRED")).toBeInTheDocument();
    expect(screen.getByText("9/10")).toBeInTheDocument();
    expect(
      screen.getByText(/does not match the saved model route/i),
    ).toBeInTheDocument();
  });

  it("monitors safely until the first immutable decision arrives", () => {
    render(<PaperAutonomyReadinessControlPlane {...base} decisions={[]} />);

    expect(screen.getByText("PAPER MONITORING")).toBeInTheDocument();
    expect(screen.getByText("8/10")).toBeInTheDocument();
    expect(
      screen.getByText(/Waiting for the first automatic AI decision/i),
    ).toBeInTheDocument();
  });
});
