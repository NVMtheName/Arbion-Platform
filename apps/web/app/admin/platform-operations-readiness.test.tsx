import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  PlatformOperationsReadiness,
  type PlatformOperationsOverview,
} from "./platform-operations-readiness";

const nominal: PlatformOperationsOverview = {
  generated_at: "2026-08-27T23:30:00Z",
  operational_status: "NOMINAL",
  active_ai_shadow_instances: 2,
  unhealthy_ai_schedules: 0,
  unhealthy_ai_reconciliations: 0,
  unavailable_financial_connections: 0,
  open_global_breakers: 0,
  open_scoped_breakers: 0,
  execution_boundary: {
    live_mandates: 0,
    non_shadow_ai_instances: 0,
    non_shadow_ai_executions: 0,
    executable_risk_evaluations: 0,
    non_executing_ai_proposals: 0,
    reviewed_non_executing_proposals: 0,
  },
  signals: [
    {
      code: "SHADOW_EXECUTION_BOUNDARY",
      state: "PASS",
      count: 0,
      summary: "No incompatible execution record exists.",
    },
  ],
  live_execution_available: false,
  broker_action_requested: false,
};

describe("PlatformOperationsReadiness", () => {
  afterEach(cleanup);

  it("renders credential-free Shadow operations evidence without action controls", () => {
    render(<PlatformOperationsReadiness operations={nominal} />);
    expect(screen.getByText("NOMINAL")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(
      screen.getByText(/shadow execution boundary/i).closest("li"),
    ).toHaveTextContent(/pass/i);
    expect(
      screen.getByText(/live execution remains unavailable/i),
    ).toBeVisible();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("makes stopped and unhealthy evidence explicit", () => {
    render(
      <PlatformOperationsReadiness
        operations={{
          ...nominal,
          operational_status: "STOPPED",
          unhealthy_ai_schedules: 1,
          open_global_breakers: 1,
          signals: [
            {
              code: "CIRCUIT_BREAKERS",
              state: "STOPPED",
              count: 1,
              summary: "The platform-wide emergency stop is active.",
            },
          ],
        }}
      />,
    );
    expect(screen.getByText("STOPPED")).toBeInTheDocument();
    expect(
      screen.getByText(/circuit breakers/i).closest("li"),
    ).toHaveTextContent(/stopped \(1\)/i);
    expect(
      screen.getByText(/platform-wide emergency stop is active/i),
    ).toBeVisible();
  });
});
