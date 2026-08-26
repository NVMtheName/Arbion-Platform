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
    expect(screen.getByText("Healthy")).toBeInTheDocument();
    expect(
      screen.getByText("Collect more 24-hour outcome marks"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/does not authorize live trading/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/not prediction accuracy/i)).toBeInTheDocument();
    expect(screen.getByText(/account P&L/i)).toBeInTheDocument();
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
});
