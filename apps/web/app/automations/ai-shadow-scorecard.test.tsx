import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AIShadowScorecard } from "./ai-shadow-scorecard";

describe("AI shadow scorecard", () => {
  it("separates horizons and labels the evidence conservatively", () => {
    render(
      <AIShadowScorecard
        scorecard={{
          strategy_instance_id: "instance-1",
          total_marks: 1,
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
    expect(screen.getByText(/not prediction accuracy/i)).toBeInTheDocument();
    expect(screen.getByText(/account P&L/i)).toBeInTheDocument();
  });
});
