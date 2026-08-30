import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AutonomyReadinessControlPlane } from "./autonomy-readiness-control-plane";

const base = {
  provider: "coinbase",
  modelID: "gpt-5.6-sol",
  mandateStatus: "READY",
  currentVersion: 7,
  automationType: "AI_AUTONOMOUS",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  financialAccount: { id: "account-1", status: "active" },
  financialConnection: { id: "financial-1", status: "active" },
  aiConnection: { id: "ai-1", status: "active", enabled: true },
  capitalBucket: {
    id: "bucket-1",
    status: "ACTIVE",
    is_reserve: false,
    allocation_value: "1000.0000000000",
  },
  instance: {
    id: "instance-1",
    status: "ACTIVE",
    current_state: "AI_MONITORING",
    execution_mode: "SHADOW",
  },
  schedule: {
    enabled: true,
    last_status: "SUCCEEDED",
    consecutive_failures: 0,
    next_run_at: "2026-09-01T01:00:00Z",
  },
  evidenceGate: {
    status: "EVIDENCE_REVIEWABLE",
    one_hour_sample_size: 22,
    twenty_four_hour_sample_size: 20,
    minimum_sample_per_horizon: 20,
    evidence_window_hours: 172,
    minimum_evidence_window_hours: 168,
  },
  reconciliation: {
    comparison_status: "MATCHED",
    balances_status: "READY",
    positions_status: "READY",
    autonomy_signal: "CLEAR",
    autonomy_enforcement_active: true,
    blocks_new_actions: false,
    observed_at: "2026-08-27T16:00:00Z",
  },
  automationBreaker: null,
  schedulerEnabled: true,
  observedAt: "2026-08-27T18:00:00Z",
};

describe("autonomy readiness control plane", () => {
  afterEach(cleanup);

  it("shows a fully verified non-live review without implying execution", () => {
    render(<AutonomyReadinessControlPlane {...base} />);

    expect(screen.getByText("NON-LIVE REVIEWABLE")).toBeInTheDocument();
    expect(screen.getByText("9/9")).toBeInTheDocument();
    expect(screen.getByText(/Coinbase connection/i)).toBeInTheDocument();
    expect(screen.getByText(/22\/20 one-hour/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Physically unavailable in this release/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/cannot submit, replace, cancel/i),
    ).toBeInTheDocument();
  });

  it("keeps incomplete evidence visible without weakening the boundary", () => {
    render(
      <AutonomyReadinessControlPlane
        {...base}
        evidenceGate={{
          ...base.evidenceGate,
          status: "COLLECTING_EVIDENCE",
          one_hour_sample_size: 12,
          twenty_four_hour_sample_size: 8,
          evidence_window_hours: 72,
        }}
      />,
    );

    expect(screen.getByText("COLLECTING EVIDENCE")).toBeInTheDocument();
    expect(screen.getByText("8/9")).toBeInTheDocument();
    expect(screen.getByText(/12\/20 one-hour/i)).toBeInTheDocument();
    expect(screen.getByText(/broker-write adapter/i)).toBeInTheDocument();
  });

  it("fails closed when the scheduler or emergency stop is unhealthy", () => {
    render(
      <AutonomyReadinessControlPlane
        {...base}
        schedule={{
          ...base.schedule,
          last_status: "FAILED",
          consecutive_failures: 2,
        }}
        automationBreaker={{ state: "OPEN" }}
      />,
    );

    expect(screen.getByText("REVIEW REQUIRED")).toBeInTheDocument();
    expect(screen.getAllByText("Blocked")).toHaveLength(2);
    expect(screen.getByText(/emergency stop is engaged/i)).toBeInTheDocument();
  });

  it("rejects non-canonical capital values that JavaScript would coerce", () => {
    render(
      <AutonomyReadinessControlPlane
        {...base}
        capitalBucket={{
          ...base.capitalBucket,
          allocation_value: "1e3",
        }}
      />,
    );

    expect(screen.getByText("REVIEW REQUIRED")).toBeInTheDocument();
    expect(screen.getByText("8/9")).toBeInTheDocument();
    expect(
      screen.getByText(/active, positive, non-reserve capital allocation/i),
    ).toBeInTheDocument();
  });
});
