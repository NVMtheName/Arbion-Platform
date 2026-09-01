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
  evidenceReadinessContractAvailable: false,
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

  it("shows normal evidence collection without requiring a proposal or fill", () => {
    render(
      <PaperAutonomyReadinessControlPlane
        {...base}
        evidenceReadinessContractAvailable
        paperPortfolio={{
          ...base.paperPortfolio,
          evidence_readiness: {
            status: "COLLECTING_EVIDENCE",
            calculation_method:
              "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE",
            as_of: "2026-08-31T12:00:00Z",
            review_scope: "OWNER_REVIEW_EVIDENCE_ONLY",
            execution_boundary: "PAPER_SIMULATION_ONLY",
            minimum_decision_count: 20,
            minimum_evidence_window_hours: 168,
            evidence_window_hours: 48,
            decision_count: 20,
            abstention_count: 20,
            proposal_count: 0,
            deterministic_deny_count: 0,
            simulated_fill_count: 0,
            consecutive_schedule_failures: 0,
            last_schedule_status: "SUCCEEDED",
            attributed_decision_count: 20,
            telemetry_complete_count: 20,
            bounded_memory_count: 20,
            routes: [
              {
                ai_provider: "openai",
                model_id: "gpt-5.6-sol",
                profile: "deep",
                financial_provider: "coinbase",
                decision_count: 20,
              },
            ],
            ledger_contracts_reconciled: true,
            safety: {
              status: "CLEAR",
              live_mandate_count: 0,
              ai_order_intent_count: 0,
              invalid_strategy_mode_count: 0,
              invalid_execution_mode_count: 0,
              platform_executable_risk_count: 0,
              non_simulation_fill_count: 0,
            },
            review_packet: {
              status: "COLLECTING_EVIDENCE",
              calculation_method:
                "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_REVIEW_PACKET",
              evidence_started_at: "2026-08-29T12:00:00Z",
              evidence_eligible_at: "2026-09-05T12:00:00Z",
              as_of: "2026-08-31T12:00:00Z",
              elapsed_seconds: 172800,
              remaining_seconds: 432000,
              scheduler_sample_count: 20,
              scheduler_success_count: 20,
              scheduler_failure_count: 0,
              scheduler_safe_wait_count: 0,
              route_continuity_status: "STABLE",
              input_coverage_status: "COMPLETE",
              input_freshness_status: "CURRENT_AT_DECISION",
              freshness_threshold_seconds: 300,
              market_observation_count: 60,
              fresh_market_decision_count: 20,
              maximum_market_age_seconds: 2,
              first_market_observed_at: "2026-08-29T12:59:58Z",
              latest_market_observed_at: "2026-08-31T11:59:58Z",
              ledger_contract_status: "RECONCILED",
              no_live_safety_status: "CLEAR",
              evidence_ready_for_human_review: false,
              owner_guidance:
                "No owner action is required while evidence collects.",
              grants_authority: false,
              live_promotion_available: false,
              threshold_change_ledger: {
                status: "AVAILABLE",
                calculation_method:
                  "IMMUTABLE_PAPER_EVIDENCE_THRESHOLD_CHANGE_LEDGER",
                strategy_instance_id: "instance-1",
                execution_mode: "PAPER",
                source_run_count: 1,
                checkpoint_count: 1,
                capped: false,
                checkpoints: [
                  {
                    schedule_run_id: "run-20",
                    as_of: "2026-08-31T12:00:00Z",
                    elapsed_seconds: 172800,
                    remaining_seconds: 432000,
                    evidence_window_hours: 48,
                    decision_count: 20,
                    decision_delta: 0,
                    route_continuity_status: "STABLE",
                    route_continuity_change: "BASELINE",
                    input_coverage_status: "COMPLETE",
                    input_coverage_change: "BASELINE",
                    input_freshness_status: "CURRENT_AT_DECISION",
                    scheduler_status: "SUCCEEDED",
                    consecutive_failures: 0,
                    scheduler_change: "BASELINE",
                    evidence_status: "COLLECTING_EVIDENCE",
                    progress_classification: "BASELINE",
                    routes: [
                      {
                        ai_provider: "openai",
                        model_id: "gpt-5.6-sol",
                        profile: "deep",
                        financial_provider: "coinbase",
                        decision_count: 20,
                      },
                    ],
                    blockers: [
                      {
                        code: "EVIDENCE_WINDOW_INCOMPLETE",
                        category: "COLLECTION",
                        detail: "Seven days have not elapsed yet.",
                      },
                    ],
                    added_blocker_codes: [],
                    resolved_blocker_codes: [],
                  },
                ],
                grants_authority: false,
                live_promotion_available: false,
              },
            },
            blockers: [
              {
                code: "EVIDENCE_WINDOW_INCOMPLETE",
                category: "COLLECTION",
                detail: "Seven days have not elapsed yet.",
              },
            ],
            live_execution_available: false,
          },
        }}
      />,
    );

    expect(screen.getByText("Collecting evidence")).toBeInTheDocument();
    expect(
      screen.getByText("Evidence threshold change ledger"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Automatic decisions").closest("div"),
    ).toHaveTextContent("20 / 20");
    expect(
      screen.getByText(/Proposals, fills, and profit are not required/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("region", {
        name: "Paper autonomy evidence review packet",
      }),
    ).toHaveTextContent("5d 0m remaining");
  });
});
