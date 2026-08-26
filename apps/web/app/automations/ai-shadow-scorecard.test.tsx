import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AIShadowScorecard } from "./ai-shadow-scorecard";

describe("AI shadow scorecard", () => {
  afterEach(cleanup);

  it("separates horizons and labels the evidence conservatively", () => {
    render(
      <AIShadowScorecard
        scorecard={{
          strategy_instance_id: "instance-1",
          total_marks: 1,
          evidence_gate: {
            status: "COLLECTING_EVIDENCE",
            blockers: [
              "ONE_HOUR_SAMPLE_INCOMPLETE",
              "TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE",
              "EVIDENCE_WINDOW_INCOMPLETE",
            ],
            one_hour_sample_size: 1,
            twenty_four_hour_sample_size: 0,
            minimum_sample_per_horizon: 20,
            evidence_window_hours: 0,
            minimum_evidence_window_hours: 168,
            schedule_healthy: true,
            execution_boundary: "SHADOW_ONLY",
            live_execution_available: false,
          },
          behavior: {
            total_ai_decisions: 3,
            abstentions: 1,
            proposed_decisions: 2,
            risk_held_decisions: 1,
            repeat_action_cooldown_holds: 1,
            would_have_submitted_decisions: 1,
            attributed_decisions: 2,
            unattributed_legacy_decisions: 1,
            abstention_rate_percent: "33.3333333333",
            proposal_rate_percent: "66.6666666667",
            average_decision_interval_minutes: "60.00",
            routes: [
              {
                ai_provider: "openai",
                model_id: "gpt-5.6-sol",
                profile: "deep",
                provenance_status: "EXPLICIT",
                total_decisions: 2,
                abstentions: 1,
                proposed_decisions: 1,
                risk_held_decisions: 0,
                repeat_action_cooldown_holds: 0,
                would_have_submitted_decisions: 1,
                one_hour_outcome_marks: 1,
                twenty_four_hour_outcome_marks: 0,
                measured_latency_decisions: 1,
                average_latency_milliseconds: "120.00",
                metered_usage_decisions: 1,
                recorded_input_tokens: 30,
                recorded_output_tokens: 45,
              },
              {
                provenance_status: "UNATTRIBUTED_LEGACY",
                total_decisions: 1,
                abstentions: 0,
                proposed_decisions: 1,
                risk_held_decisions: 1,
                repeat_action_cooldown_holds: 1,
                would_have_submitted_decisions: 0,
                one_hour_outcome_marks: 0,
                twenty_four_hour_outcome_marks: 0,
                measured_latency_decisions: 0,
                metered_usage_decisions: 0,
              },
            ],
            symbols: [
              {
                symbol: "BTC",
                proposed_decisions: 2,
                risk_held_decisions: 1,
                would_have_submitted_decisions: 1,
                one_hour_outcome_marks: 1,
                twenty_four_hour_outcome_marks: 0,
              },
            ],
          },
          horizons: [
            {
              horizon: "ONE_HOUR",
              sample_size: 1,
              favorable_marks: 0,
              unfavorable_marks: 1,
              flat_marks: 0,
              favorable_rate_percent: "0.0000000000",
              average_directional_change_percent: "-1.0349650350",
              interpretation: "INSUFFICIENT_SAMPLE",
              minimum_sample_for_observational_label: 20,
            },
            {
              horizon: "TWENTY_FOUR_HOURS",
              sample_size: 0,
              favorable_marks: 0,
              unfavorable_marks: 0,
              flat_marks: 0,
              interpretation: "INSUFFICIENT_SAMPLE",
              minimum_sample_for_observational_label: 20,
            },
          ],
        }}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "How hypothetical decisions moved" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1-hour horizon")).toBeInTheDocument();
    expect(screen.getByText("24-hour horizon")).toBeInTheDocument();
    expect(screen.getByText("-1.034965035%")).toBeInTheDocument();
    expect(screen.getAllByText("Early evidence")).toHaveLength(2);
    expect(screen.getByText(/1 of 20 marks/)).toBeInTheDocument();
    expect(screen.getByText("Collecting evidence")).toBeInTheDocument();
    expect(screen.getByText("1 / 20")).toBeInTheDocument();
    expect(screen.getByText("0 / 20")).toBeInTheDocument();
    expect(screen.getByText("0 / 168 hours")).toBeInTheDocument();
    expect(screen.getAllByText("Healthy")).toHaveLength(2);
    expect(
      screen.getByText("Collect more 24-hour outcome marks"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/does not authorize live trading/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/not prediction accuracy/i)).toBeInTheDocument();
    expect(screen.getByText(/account P&L/i)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "How the engine is deciding" }),
    ).toBeInTheDocument();
    expect(screen.getByText("33.3333333333%")).toBeInTheDocument();
    expect(screen.getByText(/1 repeat-action hold$/)).toBeInTheDocument();
    expect(screen.getByText(/60 min/)).toBeInTheDocument();
    expect(screen.getByText("OpenAI · gpt-5.6-sol")).toBeInTheDocument();
    expect(screen.getByText("Earlier route")).toBeInTheDocument();
    expect(
      screen.getByText(/does not guess which provider/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/Avg response 120 ms/)).toBeInTheDocument();
    expect(
      screen.getByText(/Recorded tokens 30 in \/ 45 out/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: "Proposal behavior by asset" }),
    ).toBeInTheDocument();
    expect(screen.getByText("BTC")).toBeInTheDocument();
    expect(
      screen.getByText(/Broker execution remains unavailable/i),
    ).toBeInTheDocument();
  });

  it("labels mature evidence as reviewable without granting trading authority", () => {
    render(
      <AIShadowScorecard
        scorecard={{
          total_marks: 40,
          horizons: [],
          evidence_gate: {
            status: "EVIDENCE_REVIEWABLE",
            blockers: [],
            one_hour_sample_size: 20,
            twenty_four_hour_sample_size: 20,
            minimum_sample_per_horizon: 20,
            evidence_window_hours: 192,
            minimum_evidence_window_hours: 168,
            schedule_healthy: true,
            execution_boundary: "SHADOW_ONLY",
            live_execution_available: false,
          },
        }}
      />,
    );

    expect(screen.getByText("Reviewable evidence")).toBeInTheDocument();
    expect(screen.queryByText("Still needed")).not.toBeInTheDocument();
    expect(
      screen.getByText(/does not authorize live trading/i),
    ).toBeInTheDocument();
  });

  it("keeps an empty behavior record explicit without inventing rates", () => {
    render(
      <AIShadowScorecard
        scorecard={{
          total_marks: 0,
          horizons: [],
          behavior: {
            total_ai_decisions: 0,
            abstentions: 0,
            proposed_decisions: 0,
            risk_held_decisions: 0,
            repeat_action_cooldown_holds: 0,
            would_have_submitted_decisions: 0,
            attributed_decisions: 0,
            unattributed_legacy_decisions: 0,
            routes: [],
            symbols: [],
          },
        }}
      />,
    );

    expect(
      screen.getByText(
        /Route behavior will appear after the first completed AI cycle/i,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/0 repeat-action holds$/)).toBeInTheDocument();
    expect(screen.getByText(/0 \/ 0/)).toBeInTheDocument();
    expect(document.body.textContent).not.toContain("NaN");
  });
});
