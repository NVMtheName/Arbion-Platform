import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  PaperToLiveReadinessDossier,
  projectPaperToLiveReadinessDossier,
} from "./paper-live-readiness-dossier";
import type { StrategyFleetItem } from "./strategy-fleet";

const IDs = {
  paperMandate: "11111111-1111-4111-8111-111111111111",
  paperInstance: "22222222-2222-4222-8222-222222222222",
  paperBucket: "33333333-3333-4333-8333-333333333333",
  paperReservation: "44444444-4444-4444-8444-444444444444",
  shadowMandate: "55555555-5555-4555-8555-555555555555",
  shadowInstance: "66666666-6666-4666-8666-666666666666",
  shadowBucket: "77777777-7777-4777-8777-777777777777",
  shadowReservation: "88888888-8888-4888-8888-888888888888",
  account: "99999999-9999-4999-8999-999999999999",
  connection: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  decision: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  review: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
};

const fingerprint = "d".repeat(64);

function paperGate(
  status: "EVIDENCE_REVIEWABLE" | "COLLECTING_EVIDENCE" = "EVIDENCE_REVIEWABLE",
): NonNullable<StrategyFleetItem["paperEvidenceReadiness"]> {
  return {
    status,
    calculation_method: "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE",
    as_of: "2026-09-07T13:30:00Z",
    review_scope: "OWNER_REVIEW_EVIDENCE_ONLY",
    execution_boundary: "PAPER_SIMULATION_ONLY",
    minimum_decision_count: 20,
    minimum_evidence_window_hours: 168,
    evidence_window_hours: status === "EVIDENCE_REVIEWABLE" ? 192 : 96,
    decision_count: status === "EVIDENCE_REVIEWABLE" ? 30 : 12,
    abstention_count: 20,
    proposal_count: 10,
    deterministic_deny_count: 2,
    simulated_fill_count: 8,
    consecutive_schedule_failures: 0,
    attributed_decision_count: 30,
    telemetry_complete_count: 30,
    bounded_memory_count: 30,
    routes: [],
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
      grants_authority: false,
      live_promotion_available: false,
    } as NonNullable<
      StrategyFleetItem["paperEvidenceReadiness"]
    >["review_packet"],
    blockers: [],
    live_execution_available: false,
  };
}

function paperReview(): NonNullable<
  StrategyFleetItem["paperLatestEvidenceReview"]
> {
  return {
    id: IDs.review,
    strategy_instance_id: IDs.paperInstance,
    financial_account_id: IDs.account,
    mandate_id: IDs.paperMandate,
    mandate_version: 3,
    evidence_fingerprint: fingerprint,
    gate_status: "EVIDENCE_REVIEWABLE",
    evidence_started_at: "2026-08-30T13:30:00Z",
    evidence_eligible_at: "2026-09-06T13:30:00Z",
    evidence_as_of: "2026-09-07T13:30:00Z",
    evidence_window_hours: 192,
    decision_count: 30,
    portfolio_version: 51,
    portfolio_updated_at: "2026-09-07T13:26:00Z",
    latest_checkpoint_run_id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
    latest_checkpoint_as_of: "2026-09-07T13:30:00Z",
    scheduler_sample_count: 30,
    scheduler_success_count: 29,
    scheduler_failure_count: 1,
    last_schedule_status: "SUCCEEDED",
    consecutive_schedule_failures: 0,
    route_continuity_status: "STABLE",
    input_coverage_status: "COMPLETE",
    input_freshness_status: "CURRENT_AT_DECISION",
    ledger_contract_status: "RECONCILED",
    no_live_safety_status: "CLEAR",
    execution_boundary: "PAPER_SIMULATION_ONLY",
    review_scope: "PAPER_NON_LIVE_EVIDENCE_ONLY",
    grants_authority: false,
    live_promotion_available: false,
    mfa_method: "totp",
    reviewed_at: "2026-09-07T13:32:00Z",
    created_at: "2026-09-07T13:32:00Z",
  };
}

function exactPaper(): StrategyFleetItem {
  return {
    id: IDs.paperMandate,
    freshnessObservedAt: "2026-09-07T14:00:00Z",
    strategyInstanceID: IDs.paperInstance,
    financialAccountID: IDs.account,
    financialConnectionID: IDs.connection,
    capitalBucketID: IDs.paperBucket,
    capitalReservationID: IDs.paperReservation,
    title: "Coinbase AI Paper",
    accountName: "Coinbase Portfolio",
    provider: "coinbase",
    financialConnectionContextAvailable: true,
    financialConnectionAvailable: true,
    financialConnectionStatus: "active",
    financialConnectionLastVerifiedAt: "2026-09-07T13:20:00Z",
    capitalContextAvailable: true,
    capitalBindingValid: true,
    capitalReservationStatus: "ACTIVE",
    capitalReservationAmount: "1000.0000000000",
    capitalReservationBasis: "PAPER_STARTING_CASH",
    runtimeMandateVersion: 3,
    runtimeScheduleBindingValid: true,
    runtimeMaxProposalNotional: "100.0000000000",
    automationType: "AI_AUTONOMOUS",
    mandateStatus: "READY",
    autonomyLevel: "FULL_AUTONOMOUS",
    executionMode: "PAPER",
    modelID: "gpt-5.6-sol",
    symbols: ["BTC", "ETH", "XRP"],
    instanceStatus: "ACTIVE",
    currentState: "AI_MONITORING",
    scheduleAvailable: true,
    scheduleEnabled: true,
    scheduleStatus: "SUCCEEDED",
    scheduleLastCompletedAt: "2026-09-07T13:26:30Z",
    scheduleTimingStatus: "ON_SCHEDULE",
    consecutiveFailures: 0,
    nextRunAt: "2026-09-07T14:26:30Z",
    scheduleHistoryAvailable: true,
    paperPortfolioAvailable: true,
    paperStartingCash: "1000.0000000000",
    paperCashHeadroom: "528.6131948560",
    paperExposureHeadroom: "527.0000000000",
    paperProposalHeadroom: "50.0000000000",
    paperRealizedContractAvailable: true,
    paperRealizedOutcomeStatus: "AVAILABLE",
    paperExecutionCostsContractAvailable: true,
    paperExecutionCostsStatus: "AVAILABLE",
    paperExecutionFillCount: 51,
    paperActivityCadenceContractAvailable: true,
    paperActivityCadence: {
      status: "AVAILABLE",
    } as NonNullable<StrategyFleetItem["paperActivityCadence"]>,
    paperOutcomeReconciliationStatus: "RECONCILED_EXACT",
    paperEvidenceReadinessContractAvailable: true,
    paperEvidenceReadiness: paperGate(),
    paperEvidenceReviewContractAvailable: true,
    paperEvidenceReviewFingerprint: fingerprint,
    paperLatestEvidenceReview: paperReview(),
    paperCurrentEvidenceReviewed: true,
    decisionAvailable: true,
    latestDecisionID: IDs.decision,
    latestDecisionType: "ABSTAIN",
    latestDecisionAt: "2026-09-07T13:26:20Z",
    latestDecisionProposedNotional: "0",
    latestDecisionAIProvider: "openai",
    latestDecisionAIModelID: "gpt-5.6-sol",
    latestDecisionAIProfile: "deep",
    latestDecisionLatencyMS: 12185,
    latestDecisionInputUsage: 2176,
    latestDecisionOutputUsage: 426,
    latestDecisionFinancialContextComplete: true,
    latestDecisionFinancialProvider: "coinbase",
    latestDecisionMarketSymbols: ["BTC", "ETH", "XRP"],
    latestDecisionMarketFeeds: ["rest_ticker"],
    latestDecisionMarketQualities: ["REAL_TIME_SINGLE_VENUE"],
    latestDecisionMarketObservedAt: "2026-09-07T13:26:19Z",
  };
}

function exactShadow(): StrategyFleetItem {
  return {
    id: IDs.shadowMandate,
    freshnessObservedAt: "2026-09-07T14:00:00Z",
    strategyInstanceID: IDs.shadowInstance,
    financialAccountID: IDs.account,
    financialConnectionID: IDs.connection,
    capitalBucketID: IDs.shadowBucket,
    capitalReservationID: IDs.shadowReservation,
    title: "Coinbase AI Shadow",
    accountName: "Coinbase Portfolio",
    provider: "coinbase",
    automationType: "AI_AUTONOMOUS",
    mandateStatus: "READY",
    autonomyLevel: "FULL_AUTONOMOUS",
    executionMode: "SHADOW",
    symbols: ["BTC", "ETH", "XRP"],
    instanceStatus: "ACTIVE",
    currentState: "AI_MONITORING",
    consecutiveFailures: 0,
    evidenceAvailable: true,
    evidenceStatus: "EVIDENCE_REVIEWABLE",
    oneHourSampleSize: 24,
    twentyFourHourSampleSize: 20,
    minimumSamplePerHorizon: 20,
    evidenceWindowHours: 240,
    minimumEvidenceWindowHours: 168,
    evidenceScheduleHealthy: true,
  };
}

describe("Paper-to-live readiness dossier", () => {
  it("proves every non-live prerequisite while keeping live capability blocked", () => {
    const dossier = projectPaperToLiveReadinessDossier([
      exactPaper(),
      exactShadow(),
    ]);

    expect(dossier).toEqual(
      expect.objectContaining({
        status: "PLATFORM_BLOCKED",
        paperEngineCount: 1,
        provenCount: 8,
        blockedCount: 1,
        collectingCount: 0,
        unavailableCount: 0,
      }),
    );
    expect(dossier.engines[0]).toEqual(
      expect.objectContaining({
        instanceID: IDs.paperInstance,
        shadowInstanceID: IDs.shadowInstance,
        shadowMandateID: IDs.shadowMandate,
      }),
    );
  });

  it("renders a non-authorizing owner view with immutable evidence links", () => {
    render(
      <PaperToLiveReadinessDossier items={[exactPaper(), exactShadow()]} />,
    );

    const dossier = screen.getByRole("region", {
      name: "Live execution remains intentionally unavailable.",
    });
    expect(dossier).toHaveTextContent("8proven controls");
    expect(dossier).toHaveTextContent("Live platform blocked");
    expect(dossier).toHaveTextContent("grants_authority=false");
    expect(dossier).toHaveTextContent(IDs.shadowInstance);
    expect(
      screen.getByRole("link", { name: /Open immutable Paper evidence/i }),
    ).toHaveAttribute(
      "href",
      `/automations/${IDs.paperMandate}#runtime-evidence`,
    );
    expect(
      screen.getByRole("link", { name: /Open companion Shadow evidence/i }),
    ).toHaveAttribute(
      "href",
      `/automations/${IDs.shadowMandate}#runtime-evidence`,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("preserves collecting Paper and Shadow evidence without implying readiness", () => {
    const paper = exactPaper();
    paper.paperEvidenceReadiness = paperGate("COLLECTING_EVIDENCE");
    paper.paperCurrentEvidenceReviewed = false;
    paper.paperLatestEvidenceReview = undefined;
    const shadow = exactShadow();
    shadow.evidenceStatus = "COLLECTING_EVIDENCE";
    shadow.oneHourSampleSize = 12;
    shadow.twentyFourHourSampleSize = 4;
    shadow.evidenceWindowHours = 96;

    const dossier = projectPaperToLiveReadinessDossier([paper, shadow]);

    expect(dossier.status).toBe("EVIDENCE_COLLECTING");
    expect(dossier.collectingCount).toBe(2);
    expect(dossier.blockedCount).toBe(1);
    expect(dossier.engines[0].ownerAction).toContain("next normal Paper");
  });

  it("fails closed when fleet identity evidence collides", () => {
    const shadow = exactShadow();
    shadow.capitalBucketID = IDs.paperBucket;

    const dossier = projectPaperToLiveReadinessDossier([exactPaper(), shadow]);

    expect(dossier.status).toBe("EVIDENCE_UNAVAILABLE");
    expect(
      dossier.engines[0].signals.find((signal) => signal.key === "CAPITAL"),
    ).toEqual(expect.objectContaining({ state: "UNAVAILABLE" }));
  });

  it("surfaces an automatic scheduler failure as a review blocker", () => {
    const paper = exactPaper();
    paper.scheduleStatus = "FAILED";
    paper.consecutiveFailures = 1;

    const dossier = projectPaperToLiveReadinessDossier([paper, exactShadow()]);

    expect(dossier.status).toBe("REVIEW_REQUIRED");
    expect(
      dossier.engines[0].signals.find((signal) => signal.key === "SCHEDULER"),
    ).toEqual(expect.objectContaining({ state: "BLOCKED" }));
  });

  it("does not infer readiness from mismatched model provenance", () => {
    const paper = exactPaper();
    paper.latestDecisionAIModelID = "another-model";

    const dossier = projectPaperToLiveReadinessDossier([paper, exactShadow()]);

    expect(dossier.status).toBe("EVIDENCE_UNAVAILABLE");
    expect(
      dossier.engines[0].signals.find((signal) => signal.key === "PROVENANCE"),
    ).toEqual(expect.objectContaining({ state: "UNAVAILABLE" }));
  });
});
