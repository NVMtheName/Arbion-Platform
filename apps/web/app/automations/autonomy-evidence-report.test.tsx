import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  AutonomyEvidenceReport,
  buildAutonomyEvidenceReport,
} from "./autonomy-evidence-report";

const base = {
  generatedAt: "2026-08-28T04:00:00Z",
  mandateId: "c15b9d4d-2e7e-4698-b7de-3bfaf51bcdcf",
  currentVersion: 7,
  mandateStatus: "READY",
  automationType: "AI_AUTONOMOUS",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  financialAccountId: "account-1",
  financialProvider: "coinbase",
  financialAccount: { id: "account-1", status: "active" },
  financialConnection: {
    id: "financial-1",
    status: "active",
    credential_storage: "vault://must-not-export",
  },
  aiConnection: {
    id: "ai-1",
    provider: "openai",
    status: "active",
    enabled: true,
    credential_hint: "sk-never-export",
  },
  modelId: "gpt-5.6-sol",
  capitalBucketId: "bucket-1",
  capitalBucket: {
    id: "bucket-1",
    status: "ACTIVE",
    allocation_type: "FIXED_AMOUNT",
    allocation_value: "1000.0000000000",
    currency: "USD",
    protected_amount: "0",
    is_reserve: false,
  },
  instance: {
    id: "1d17ec38-4aa3-46a7-9b89-59b2bfce725c",
    status: "ACTIVE",
    current_state: "AI_MONITORING",
    execution_mode: "SHADOW",
    mandate_version: 7,
    last_evaluated_at: "2026-08-28T03:13:54Z",
  },
  schedule: {
    strategy_instance_id: "1d17ec38-4aa3-46a7-9b89-59b2bfce725c",
    enabled: true,
    interval_minutes: 60,
    session: "CONTINUOUS",
    last_status: "SUCCEEDED",
    consecutive_failures: 0,
    last_completed_at: "2026-08-28T03:14:02Z",
    next_run_at: "2026-08-28T04:13:54Z",
  },
  scheduleRuns: [
    {
      id: "run-1",
      mandate_version: 7,
      execution_mode: "SHADOW",
      strategy_state: "AI_MONITORING",
      scheduled_for: "2026-08-28T03:13:54Z",
      started_at: "2026-08-28T03:13:55Z",
      completed_at: "2026-08-28T03:14:02Z",
      next_run_at: "2026-08-28T04:13:54Z",
      status: "SUCCEEDED",
      ai_decision: "ABSTAIN",
      execution_status: "CANCELED",
      duplicate_recovered: false,
      reconciliation_id: "reconciliation-1",
      reconciliation_review_required: false,
      consecutive_failures: 0,
    },
  ],
  schedulerEnabled: true,
  evidenceGate: {
    status: "COLLECTING_EVIDENCE",
    blockers: ["TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE"],
    one_hour_sample_size: 21,
    twenty_four_hour_sample_size: 4,
    minimum_sample_per_horizon: 20,
    evidence_window_hours: 48,
    minimum_evidence_window_hours: 168,
    schedule_healthy: true,
    last_schedule_status: "SUCCEEDED",
    consecutive_schedule_failures: 0,
    execution_boundary: "SHADOW_ONLY",
    live_execution_available: false,
  },
  scorecard: {
    total_marks: 25,
    horizons: [
      {
        horizon: "ONE_HOUR",
        sample_size: 21,
        favorable_marks: 12,
        unfavorable_marks: 8,
        flat_marks: 1,
        favorable_rate_percent: "57.1428571429",
        average_directional_change_percent: "0.2500000000",
        median_directional_change_percent: "0.2000000000",
        best_directional_change_percent: "2.0000000000",
        worst_directional_change_percent: "-1.5000000000",
        average_directional_change_usd: "0.0250000000",
        cumulative_directional_change_usd: "0.5250000000",
        interpretation: "OBSERVATIONAL",
        minimum_sample_for_observational_label: 20,
      },
    ],
    behavior: {
      total_ai_decisions: 31,
      abstentions: 19,
      proposed_decisions: 12,
      risk_held_decisions: 3,
      repeat_action_cooldown_holds: 3,
      would_have_submitted_decisions: 9,
      attributed_decisions: 29,
      unattributed_legacy_decisions: 2,
      routes: [
        {
          ai_provider: "openai",
          model_id: "gpt-5.6-sol",
          profile: "deep",
          provenance_status: "EXPLICIT",
          total_decisions: 29,
          one_hour_outcome_marks: 20,
          twenty_four_hour_outcome_marks: 4,
        },
      ],
      symbols: [
        {
          symbol: "BTC",
          proposed_decisions: 12,
          one_hour_outcome_marks: 21,
          twenty_four_hour_outcome_marks: 4,
          horizons: [
            {
              horizon: "ONE_HOUR",
              sample_size: 21,
              favorable_marks: 12,
              unfavorable_marks: 8,
              flat_marks: 1,
              favorable_rate_percent: "57.1428571429",
              average_directional_change_percent: "0.2500000000",
              average_directional_change_usd: "0.0250000000",
            },
            {
              horizon: "TWENTY_FOUR_HOURS",
              sample_size: 4,
              favorable_marks: 3,
              unfavorable_marks: 1,
              flat_marks: 0,
              favorable_rate_percent: "75.0000000000",
              average_directional_change_percent: "0.5000000000",
              average_directional_change_usd: "0.0500000000",
            },
          ],
        },
      ],
    },
  },
  reconciliation: {
    id: "reconciliation-1",
    provider: "coinbase",
    comparison_status: "MATCHED",
    balances_status: "READY",
    positions_status: "READY",
    autonomy_signal: "CLEAR",
    autonomy_enforcement_active: true,
    blocks_new_actions: false,
    observed_position_count: 3,
    change_count: 0,
    blocking_change_count: 0,
    observed_at: "2026-08-28T03:13:40Z",
    balances: { cash: "private" },
    positions: [{ symbol: "PRIVATE", quantity: "99" }],
  },
  automationBreaker: null,
  breakerObserved: true,
};

function reportFromLink() {
  const link = screen.getByRole("link", { name: /save evidence report/i });
  const href = link.getAttribute("href") ?? "";
  const encoded = href.replace("data:application/json;charset=utf-8,", "");
  return {
    link,
    json: decodeURIComponent(encoded),
    report: JSON.parse(decodeURIComponent(encoded)) as Record<string, unknown>,
  };
}

describe("autonomy evidence report", () => {
  afterEach(cleanup);

  it("creates a complete, bounded owner snapshot without execution authority", () => {
    render(<AutonomyEvidenceReport {...base} />);

    expect(screen.getByText("Complete snapshot")).toBeInTheDocument();
    expect(screen.getByText("25")).toBeInTheDocument();
    expect(screen.getByText("31")).toBeInTheDocument();

    const { link, json, report } = reportFromLink();
    expect(link).toHaveAttribute(
      "download",
      "arbion-autonomy-evidence-c15b9d4d-2026-08-28.json",
    );
    expect(report).toMatchObject({
      schema_version: "arbion.autonomy-evidence-report.v2",
      report_status: "COMPLETE",
      generation_boundary: {
        read_only: true,
        provider_called: false,
        model_called: false,
        strategy_mutated: false,
        broker_action_requested: false,
        order_created: false,
        live_execution_available: false,
        trade_authority_granted: false,
      },
    });
    expect(json).toContain('"provenance_status": "EXPLICIT"');
    expect(json).toContain('"symbol": "BTC"');
    expect(json).toContain(
      '"median_directional_change_percent": "0.2000000000"',
    );
    expect(json).toContain('"average_directional_change_usd": "0.0250000000"');
    expect(json).toContain('"horizon": "TWENTY_FOUR_HOURS"');
    expect(json).toContain('"run_id": "run-1"');
    expect(json).not.toContain("sk-never-export");
    expect(json).not.toContain("vault://must-not-export");
    expect(json).not.toContain('"balances"');
    expect(json).not.toContain('"positions"');
    expect(json).not.toContain('"quantity"');
  });

  it("exports missing evidence as an explicit partial snapshot", () => {
    render(
      <AutonomyEvidenceReport
        {...base}
        scorecard={undefined}
        evidenceGate={undefined}
        reconciliation={undefined}
        breakerObserved={false}
      />,
    );

    expect(screen.getByText("Partial snapshot")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    const { report } = reportFromLink();
    expect(report).toMatchObject({
      report_status: "PARTIAL",
      missing_sources: ["reconciliation", "shadow_scorecard", "emergency_stop"],
    });
    expect(report).toMatchObject({ emergency_stop: { state: "UNAVAILABLE" } });
  });

  it("caps repeated scorecard fields and rejects malformed values", () => {
    const report = buildAutonomyEvidenceReport({
      ...base,
      generatedAt: "invalid",
      scorecard: {
        total_marks: -4,
        horizons: Array.from({ length: 8 }, (_, index) => ({
          horizon: `H${index}`,
        })),
        behavior: {
          total_ai_decisions: Number.POSITIVE_INFINITY,
          routes: Array.from({ length: 30 }, (_, index) => ({
            provenance_status: `R${index}`,
          })),
          symbols: Array.from({ length: 130 }, (_, index) => ({
            symbol: `S${index}`,
          })),
        },
      },
    });

    expect(report.generated_at).toBe("1970-01-01T00:00:00.000Z");
    expect(report.shadow_evidence.total_marks).toBeNull();
    expect(report.shadow_evidence.behavior.total_ai_decisions).toBeNull();
    expect(report.shadow_evidence.horizons).toHaveLength(4);
    expect(report.shadow_evidence.behavior.routes).toHaveLength(20);
    expect(report.shadow_evidence.behavior.symbols).toHaveLength(100);
  });
});
