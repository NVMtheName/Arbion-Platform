import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  projectStrategyFleetAccountIsolation,
  projectStrategyFleetDecisionEvidence,
  projectStrategyFleetIdentityIsolation,
  projectStrategyFleetInputCoverageChangeLedger,
  projectStrategyFleetInputCoverageMatrix,
  projectStrategyFleetOperatingBrief,
  projectStrategyFleetProvenanceDigest,
  projectStrategyFleetScheduleRecovery,
  projectStrategyFleetScheduleReliability,
  reconciliationFreshWithinTwentyFourHours,
  scheduledRunTimingStatus,
  StrategyFleet,
  type StrategyFleetItem,
} from "./strategy-fleet";

const coinbaseEngine: StrategyFleetItem = {
  id: "ai-mandate",
  strategyInstanceID: "coinbase-shadow-instance",
  financialAccountID: "coinbase-account",
  capitalBucketID: "coinbase-shadow-bucket",
  capitalReservationID: "coinbase-shadow-reservation",
  title: "AI Shadow Engine",
  accountName: "Coinbase Portfolio ••••a5d0",
  provider: "coinbase",
  accountStatus: "active",
  financialConnectionAvailable: true,
  financialConnectionContextAvailable: true,
  financialConnectionStatus: "active",
  capitalContextAvailable: true,
  capitalBindingValid: true,
  capitalBucketName: "Coinbase AI Shadow",
  capitalBucketStatus: "ACTIVE",
  capitalAllocationType: "FIXED_AMOUNT",
  capitalAllocationValue: "1000.0000000000",
  capitalCurrency: "USD",
  capitalProtectedAmount: "0.0000000000",
  capitalAllocationLimit: "1000.0000000000",
  capitalReservationStatus: "ACTIVE",
  capitalReservationAmount: "1000.0000000000",
  capitalReservationCurrency: "USD",
  capitalReservationBasis: "BUCKET_FIXED_CAPACITY",
  capitalReservationAccountLimit: "1000.0000000000",
  runtimeVersionContextAvailable: true,
  runtimeBindingValid: true,
  runtimeScheduleBindingValid: true,
  runtimeMandateVersion: 6,
  currentMandateVersion: 6,
  runtimeSnapshotStatus: "READY",
  newerDraftAvailable: false,
  runtimeMaxProposalNotional: "1.0000000000",
  runtimeMaxTradesPerDay: 1,
  runtimeLegacyDailyActionLimitMissing: false,
  runtimeScheduleEnabled: true,
  runtimeScheduleIntervalMinutes: 60,
  runtimeScheduleSession: "CONTINUOUS",
  automationType: "AI_AUTONOMOUS",
  mandateStatus: "READY",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  modelID: "gpt-5.6-sol",
  symbols: ["BTC", "ETH", "XRP", "SOL"],
  instanceStatus: "ACTIVE",
  currentState: "AI_MONITORING",
  lastEvaluatedAt: "2026-08-26T16:17:39Z",
  scheduleAvailable: true,
  scheduleEnabled: true,
  scheduleStatus: "SUCCEEDED",
  scheduleLastCompletedAt: "2026-08-26T16:17:39Z",
  scheduleTimingStatus: "ON_SCHEDULE",
  consecutiveFailures: 0,
  nextRunAt: "2026-08-26T17:17:39Z",
  scheduleHistoryAvailable: true,
  scheduleRecentRuns: [
    {
      id: "schedule-run-current",
      scheduledFor: "2026-08-26T16:17:00Z",
      completedAt: "2026-08-26T16:17:39Z",
      nextRunAt: "2026-08-26T17:17:39Z",
      status: "SUCCEEDED",
      aiDecision: "ABSTAIN",
      executionStatus: "CANCELED",
      duplicateRecovered: false,
      consecutiveFailures: 0,
    },
  ],
  evidenceAvailable: true,
  evidenceStatus: "COLLECTING_EVIDENCE",
  oneHourSampleSize: 12,
  twentyFourHourSampleSize: 4,
  minimumSamplePerHorizon: 20,
  evidenceWindowHours: 48,
  minimumEvidenceWindowHours: 168,
  evidenceScheduleHealthy: true,
  evidenceBlockers: [
    "ONE_HOUR_SAMPLE_INCOMPLETE",
    "TWENTY_FOUR_HOUR_SAMPLE_INCOMPLETE",
    "EVIDENCE_WINDOW_INCOMPLETE",
  ],
  currentEvidenceReviewed: false,
  decisionAvailable: true,
  latestDecisionID: "decision-abstain",
  latestDecisionType: "ABSTAIN",
  latestDecisionAt: "2026-08-26T16:17:39Z",
  latestDecisionSymbol: "NONE",
  latestDecisionSide: "NONE",
  latestDecisionAIProvider: "openai",
  latestDecisionAIModelID: "gpt-5.6-sol",
  latestDecisionAIProfile: "deep",
  latestDecisionLatencyMS: 1842,
  latestDecisionInputUsage: 12540,
  latestDecisionOutputUsage: 422,
  latestDecisionProposedNotional: "0",
  latestDecisionFinancialContextComplete: true,
  latestDecisionFinancialProvider: "coinbase",
  latestDecisionMarketSymbols: ["BTC", "ETH", "XRP", "SOL"],
  latestDecisionMarketFeeds: ["rest_ticker"],
  latestDecisionMarketQualities: ["REAL_TIME_SINGLE_VENUE"],
  latestDecisionMarketObservedAt: "2026-08-26T16:17:38Z",
  latestDecisionInputCoverageComplete: true,
  latestDecisionHistoryLiquidityEvidenceComplete: true,
  latestDecisionHistoryStatuses: ["COMPLETE"],
  latestDecisionHistoryFeeds: ["coinbase_candles"],
  latestDecisionHistoryQualities: ["REAL_TIME_SINGLE_VENUE"],
  latestDecisionLiquidityStatuses: ["AVAILABLE"],
  latestDecisionPositionEvidenceComplete: true,
  latestDecisionPositionCount: 2,
  latestDecisionPositionPerformanceStatuses: ["UNAVAILABLE"],
  latestDecisionMarketEventEvidenceComplete: true,
  latestDecisionMarketEventCoverageCount: 0,
  latestDecisionMarketEventCoverageStatuses: [],
  latestDecisionMarketEventProviders: [],
  latestDecisionMarketEventFeeds: [],
  latestDecisionMarketEventQualities: [],
  latestDecisionMarketEventCount: 0,
  priorDecisionID: "decision-abstain-prior",
  priorDecisionType: "ABSTAIN",
  priorDecisionAt: "2026-08-26T15:17:39Z",
  priorDecisionSymbol: "NONE",
  priorDecisionSide: "NONE",
  priorDecisionProposedNotional: "0.0000000000",
  priorDecisionFinancialContextComplete: true,
  priorDecisionFinancialProvider: "coinbase",
  priorDecisionMarketSymbols: ["BTC", "ETH", "XRP", "SOL"],
  priorDecisionMarketFeeds: ["rest_ticker"],
  priorDecisionMarketQualities: ["REAL_TIME_SINGLE_VENUE"],
  priorDecisionMarketObservedAt: "2026-08-26T15:17:38Z",
  priorDecisionInputCoverageComplete: true,
  priorDecisionHistoryLiquidityEvidenceComplete: true,
  priorDecisionHistoryStatuses: ["COMPLETE"],
  priorDecisionHistoryFeeds: ["coinbase_candles"],
  priorDecisionHistoryQualities: ["REAL_TIME_SINGLE_VENUE"],
  priorDecisionLiquidityStatuses: ["AVAILABLE"],
  priorDecisionPositionEvidenceComplete: true,
  priorDecisionPositionCount: 2,
  priorDecisionPositionPerformanceStatuses: ["UNAVAILABLE"],
  priorDecisionMarketEventEvidenceComplete: true,
  priorDecisionMarketEventCoverageCount: 0,
  priorDecisionMarketEventCoverageStatuses: [],
  priorDecisionMarketEventProviders: [],
  priorDecisionMarketEventFeeds: [],
  priorDecisionMarketEventQualities: [],
  priorDecisionMarketEventCount: 0,
  reconciliationAvailable: true,
  reconciliationComparisonStatus: "MATCHED",
  reconciliationBalancesStatus: "READY",
  reconciliationPositionsStatus: "READY",
  reconciliationAutonomySignal: "CLEAR",
  reconciliationAutonomyEnforcementActive: true,
  reconciliationBlocksNewActions: false,
  reconciliationBlockingChangeCount: 0,
  reconciliationObservedAt: "2026-08-26T16:10:00Z",
  reconciliationFresh: true,
};

describe("StrategyFleet", () => {
  afterEach(cleanup);

  it("uses the exact 24-hour reconciliation freshness boundary", () => {
    const now = new Date("2026-08-28T16:00:00Z");

    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-27T16:00:00Z", now),
    ).toBe(true);
    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-27T15:59:59.999Z", now),
    ).toBe(false);
    expect(
      reconciliationFreshWithinTwentyFourHours("2026-08-28T16:00:00.001Z", now),
    ).toBe(false);
    expect(reconciliationFreshWithinTwentyFourHours("invalid", now)).toBe(
      false,
    );
  });

  it("uses a five-minute grace boundary before a scheduled cycle is overdue", () => {
    const observedAt = new Date("2026-08-30T08:05:00Z");

    expect(scheduledRunTimingStatus("2026-08-30T08:00:00Z", observedAt)).toBe(
      "ON_SCHEDULE",
    );
    expect(
      scheduledRunTimingStatus("2026-08-30T07:59:59.999Z", observedAt),
    ).toBe("OVERDUE");
    expect(scheduledRunTimingStatus("invalid", observedAt)).toBe("UNAVAILABLE");
  });

  it("proves exact fleet scheduler reliability across a completion and safe session wait", () => {
    const schwabWait: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      financialAccountID: "schwab-account",
      capitalBucketID: "schwab-shadow-bucket",
      capitalReservationID: "schwab-shadow-reservation",
      title: "Schwab AI Shadow Engine",
      accountName: "Schwab Brokerage ••••1000",
      provider: "schwab",
      runtimeScheduleSession: "US_EQUITIES_REGULAR",
      scheduleStatus: "SKIPPED",
      scheduleErrorCode: "OUTSIDE_SESSION",
      scheduleLastCompletedAt: "2026-08-29T20:35:30Z",
      nextRunAt: "2026-08-31T13:35:00Z",
    };
    const reliability = projectStrategyFleetScheduleReliability([
      coinbaseEngine,
      schwabWait,
    ]);

    expect(reliability).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 2,
        healthyCount: 2,
        succeededCount: 1,
        safelySkippedCount: 1,
        failureCount: 0,
        overdueCount: 0,
        consecutiveFailures: 0,
        nextRunAt: "2026-08-26T17:17:39Z",
      }),
    );
  });

  it("fails fleet scheduler reliability closed when timing is overdue", () => {
    const reliability = projectStrategyFleetScheduleReliability([
      { ...coinbaseEngine, scheduleTimingStatus: "OVERDUE" },
    ]);

    expect(reliability).toEqual(
      expect.objectContaining({
        status: "ATTENTION",
        healthyCount: 0,
        overdueCount: 1,
      }),
    );
  });

  it("proves automatic recovery while preserving earlier failed cycles", () => {
    const recovered: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleHistoryAvailable: true,
      scheduleRecentRuns: [
        ...coinbaseEngine.scheduleRecentRuns!,
        {
          id: "schedule-run-failed",
          scheduledFor: "2026-08-26T15:17:00Z",
          completedAt: "2026-08-26T15:17:05Z",
          nextRunAt: "2026-08-26T16:17:00Z",
          status: "FAILED",
          errorCode: "AI_PROVIDER_UNAVAILABLE",
          duplicateRecovered: false,
          consecutiveFailures: 1,
        },
      ],
    };
    const proof = projectStrategyFleetScheduleRecovery([recovered]);

    expect(proof).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        verifiedCount: 1,
        recoveredCount: 1,
        attentionCount: 0,
        preservedRunCount: 2,
        preservedFailureCount: 1,
      }),
    );
    expect(proof.engines[0]).toEqual(
      expect.objectContaining({
        state: "RECOVERED",
        recentStatuses: ["SUCCEEDED", "FAILED"],
      }),
    );
  });

  it("fails recovery proof closed when immutable history does not match the schedule", () => {
    const proof = projectStrategyFleetScheduleRecovery([
      {
        ...coinbaseEngine,
        scheduleHistoryAvailable: true,
        scheduleRecentRuns: coinbaseEngine.scheduleRecentRuns?.map((run) => ({
          ...run,
          nextRunAt: "2026-08-26T18:17:39Z",
        })),
      },
    ]);

    expect(proof).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        verifiedCount: 0,
      }),
    );
    expect(proof.engines[0].state).toBe("UNAVAILABLE");
  });

  it("translates a healthy engine into a nontechnical operating brief", () => {
    const brief = projectStrategyFleetOperatingBrief([coinbaseEngine]);

    expect(brief).toEqual(
      expect.objectContaining({
        status: "ON_COURSE",
        engineCount: 1,
        onCourseCount: 1,
        reviewCount: 0,
      }),
    );
    expect(brief.engines[0]).toEqual(
      expect.objectContaining({
        status: "ON_COURSE",
        conclusion: "AI chose to wait.",
        nextStep: "The next guarded cycle runs automatically.",
        nextRunAt: "2026-08-26T17:17:39Z",
        reviewHref: undefined,
      }),
    );
  });

  it("gives an exact review step when scheduler evidence is unavailable", () => {
    const brief = projectStrategyFleetOperatingBrief([
      { ...coinbaseEngine, scheduleHistoryAvailable: false },
    ]);

    expect(brief).toEqual(
      expect.objectContaining({
        status: "REVIEW",
        onCourseCount: 0,
        reviewCount: 1,
      }),
    );
    expect(brief.engines[0]).toEqual(
      expect.objectContaining({
        status: "REVIEW",
        nextStep: "Review AI Shadow Engine schedule health",
        reviewHref: "/automations/ai-mandate#schedule-controls",
        reviewLabel: "Review schedule",
      }),
    );
  });

  it("verifies an attributable decision that held its exact conclusion", () => {
    const digest = projectStrategyFleetProvenanceDigest([coinbaseEngine]);

    expect(digest).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        attributableCount: 1,
        changedCount: 0,
        heldCourseCount: 1,
      }),
    );
    expect(digest.engines[0]).toEqual(
      expect.objectContaining({
        state: "HELD_COURSE",
        attributable: true,
        financialProvider: "coinbase",
        marketSymbols: ["BTC", "ETH", "XRP", "SOL"],
      }),
    );
  });

  it("reports an exact conclusion change without treating it as authority", () => {
    const digest = projectStrategyFleetProvenanceDigest([
      {
        ...coinbaseEngine,
        latestDecisionType: "ALLOW_SIMULATED_FILLED",
        latestDecisionSymbol: "BTC",
        latestDecisionSide: "BUY",
        latestDecisionProposedNotional: "50.0000000000",
      },
    ]);

    expect(digest).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        attributableCount: 1,
        changedCount: 1,
        heldCourseCount: 0,
      }),
    );
    expect(digest.engines[0]).toEqual(
      expect.objectContaining({
        state: "CHANGED",
        latestDecisionSymbol: "BTC",
        latestDecisionSide: "BUY",
        latestDecisionProposedNotional: "50.0000000000",
        priorDecisionSymbol: "NONE",
        priorDecisionSide: "NONE",
        priorDecisionProposedNotional: "0.0000000000",
        followUp: expect.stringContaining("No action is required"),
      }),
    );
  });

  it("renders the exact saved action delta for a changed conclusion", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: "ALLOW_SIMULATED_FILLED",
            latestDecisionSymbol: "BTC",
            latestDecisionSide: "BUY",
            latestDecisionProposedNotional: "50.0000000000",
          },
        ]}
      />,
    );

    const digest = screen.getByRole("region", {
      name: "1 current AI decision is fully attributable.",
    });
    expect(digest).toHaveTextContent("Conclusion changed");
    expect(digest).toHaveTextContent("No action proposed · $0");
    expect(digest).toHaveTextContent("Buy BTC · $50");
    expect(digest).toHaveTextContent("no broker order");
  });

  it("fails the provenance digest closed on a financial-provider mismatch", () => {
    const digest = projectStrategyFleetProvenanceDigest([
      {
        ...coinbaseEngine,
        latestDecisionFinancialProvider: "schwab",
      },
    ]);

    expect(digest).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        attributableCount: 0,
      }),
    );
    expect(digest.engines[0]).toEqual(
      expect.objectContaining({
        state: "UNAVAILABLE",
        followUp: expect.stringContaining("will not rerun the model"),
      }),
    );
  });

  it("projects exact input coverage without inferring missing evidence", () => {
    const schwabEngine: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      financialAccountID: "schwab-account",
      capitalBucketID: "schwab-bucket",
      capitalReservationID: "schwab-reservation",
      accountName: "Schwab Brokerage ••••1000",
      provider: "schwab",
      latestDecisionFinancialProvider: "schwab",
      latestDecisionHistoryStatuses: ["UNAVAILABLE"],
      latestDecisionHistoryFeeds: [],
      latestDecisionHistoryQualities: [],
      latestDecisionLiquidityStatuses: ["UNAVAILABLE"],
      latestDecisionPositionCount: 0,
      latestDecisionPositionPerformanceStatuses: [],
      latestDecisionMarketEventCoverageCount: 1,
      latestDecisionMarketEventCoverageStatuses: ["AVAILABLE"],
      latestDecisionMarketEventProviders: ["sec_edgar"],
      latestDecisionMarketEventFeeds: ["company_tickers"],
      latestDecisionMarketEventQualities: ["AGGREGATED_REFERENCE"],
      latestDecisionMarketEventCount: 0,
    };
    const matrix = projectStrategyFleetInputCoverageMatrix([
      coinbaseEngine,
      schwabEngine,
    ]);

    expect(matrix).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 2,
        attributableCount: 2,
        availableCategoryCount: 5,
        partialCategoryCount: 0,
        unavailableCategoryCount: 4,
      }),
    );
    expect(matrix.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "POSITION", status: "UNAVAILABLE" }),
        expect.objectContaining({ key: "EVENTS", status: "UNAVAILABLE" }),
      ]),
    );
    expect(matrix.engines[1].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "POSITION",
          status: "NOT_APPLICABLE",
        }),
        expect.objectContaining({
          key: "EVENTS",
          status: "AVAILABLE",
          evidence: expect.stringContaining("0 saved events"),
        }),
      ]),
    );
  });

  it("fails the input coverage matrix closed when attribution is incomplete", () => {
    const matrix = projectStrategyFleetInputCoverageMatrix([
      {
        ...coinbaseEngine,
        latestDecisionInputCoverageComplete: false,
      },
    ]);

    expect(matrix).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", attributableCount: 0 }),
    );
  });

  it("compares two immutable input snapshots without inferring causality", () => {
    const ledger = projectStrategyFleetInputCoverageChangeLedger([
      coinbaseEngine,
    ]);

    expect(ledger).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        comparableCount: 1,
        improvedCategoryCount: 0,
        regressedCategoryCount: 0,
        unchangedCategoryCount: 5,
        contextChangedCategoryCount: 0,
      }),
    );
    expect(ledger.engines[0]).toEqual(
      expect.objectContaining({
        comparable: true,
        followUp: expect.stringContaining("No owner action is required"),
      }),
    );
  });

  it("reports exact improvements and regressions in saved input coverage", () => {
    const ledger = projectStrategyFleetInputCoverageChangeLedger([
      {
        ...coinbaseEngine,
        latestDecisionPositionPerformanceStatuses: ["AVAILABLE"],
        latestDecisionMarketEventCoverageCount: 0,
        latestDecisionMarketEventCoverageStatuses: [],
        latestDecisionMarketEventProviders: [],
        latestDecisionMarketEventFeeds: [],
        latestDecisionMarketEventQualities: [],
        priorDecisionMarketEventCoverageCount: 1,
        priorDecisionMarketEventCoverageStatuses: ["AVAILABLE"],
        priorDecisionMarketEventProviders: ["sec_edgar"],
        priorDecisionMarketEventFeeds: ["company_tickers"],
        priorDecisionMarketEventQualities: ["AGGREGATED_REFERENCE"],
        priorDecisionMarketEventCount: 0,
      },
    ]);

    expect(ledger).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        improvedCategoryCount: 1,
        regressedCategoryCount: 1,
        unchangedCategoryCount: 3,
      }),
    );
    expect(ledger.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "POSITION", change: "IMPROVED" }),
        expect.objectContaining({ key: "EVENTS", change: "REGRESSED" }),
      ]),
    );
    expect(ledger.engines[0].followUp).toContain(
      "Review the exact regressed categories",
    );
  });

  it("fails the change ledger closed when prior provider attribution differs", () => {
    const ledger = projectStrategyFleetInputCoverageChangeLedger([
      {
        ...coinbaseEngine,
        priorDecisionFinancialProvider: "schwab",
      },
    ]);

    expect(ledger).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", comparableCount: 0 }),
    );
    expect(ledger.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ change: "UNAVAILABLE" }),
      ]),
    );
  });

  it("proves complete immutable trails for abstentions and linked non-live outcomes", () => {
    const paperFill: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "paper-mandate",
      strategyInstanceID: "paper-instance",
      capitalBucketID: "paper-bucket",
      capitalReservationID: "paper-reservation",
      title: "AI Paper Engine",
      executionMode: "PAPER",
      latestDecisionID: "paper-decision",
      latestDecisionType: "ALLOW_SIMULATED_FILLED",
      latestDecisionProposedActionID: "paper-action",
      latestDecisionRiskEvaluationID: "paper-risk",
      latestDecisionExecutionRecordID: "paper-execution",
      latestDecisionRiskDecision: "ALLOW",
      latestDecisionExecutionStatus: "SIMULATED_FILLED",
    };

    expect(
      projectStrategyFleetDecisionEvidence([coinbaseEngine, paperFill]),
    ).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 2,
        verifiedCount: 2,
        abstentionCount: 1,
        riskEvaluationCount: 1,
        nonLiveRecordCount: 1,
      }),
    );
  });

  it("fails immutable trail proof closed when a linked record identity is missing", () => {
    const incomplete = {
      ...coinbaseEngine,
      latestDecisionType: "ALLOW_WOULD_HAVE_SUBMITTED",
      latestDecisionProposedActionID: "shadow-action",
      latestDecisionRiskEvaluationID: "shadow-risk",
      latestDecisionExecutionRecordID: undefined,
      latestDecisionRiskDecision: "ALLOW",
      latestDecisionExecutionStatus: "WOULD_HAVE_SUBMITTED",
    };

    expect(projectStrategyFleetDecisionEvidence([incomplete])).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        engineCount: 1,
        verifiedCount: 0,
        nonLiveRecordCount: 0,
      }),
    );
  });

  it("projects exact account-level isolation for compatible Paper and Shadow claims", () => {
    const paperEngine: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "paper-mandate",
      strategyInstanceID: "coinbase-paper-instance",
      capitalBucketID: "coinbase-paper-bucket",
      capitalReservationID: "coinbase-paper-reservation",
      title: "AI Paper Engine",
      executionMode: "PAPER",
      capitalReservationAmount: "1000.0000000000",
      capitalReservationBasis: "PAPER_STARTING_CASH",
      capitalReservationAccountLimit: "3000.0000000000",
    };
    const shadowEngine: StrategyFleetItem = {
      ...coinbaseEngine,
      capitalReservationAmount: "750.0000000000",
      capitalReservationAccountLimit: "3000.0000000000",
    };

    expect(
      projectStrategyFleetAccountIsolation([paperEngine, shadowEngine]),
    ).toEqual([
      expect.objectContaining({
        accountID: "coinbase-account",
        status: "VERIFIED",
        engineCount: 2,
        modes: ["PAPER", "SHADOW"],
        currency: "USD",
        paperClaimedAmount: "1000",
        shadowClaimedAmount: "750",
        accountLimit: "3000",
      }),
    ]);
  });

  it("fails the isolation projection closed for duplicate policy identities", () => {
    const duplicate = {
      ...coinbaseEngine,
      id: "second-mandate",
      capitalReservationAccountLimit: "3000.0000000000",
    };
    const first = {
      ...coinbaseEngine,
      capitalReservationAccountLimit: "3000.0000000000",
    };

    expect(projectStrategyFleetAccountIsolation([first, duplicate])).toEqual([
      expect.objectContaining({
        status: "UNAVAILABLE",
        engineCount: 2,
        paperClaimedAmount: undefined,
        shadowClaimedAmount: undefined,
        accountLimit: undefined,
      }),
    ]);
  });

  it("fails the fleet identity proof closed for a cross-account identity collision", () => {
    const schwabCollision: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-mandate",
      financialAccountID: "schwab-account",
      accountName: "Schwab Brokerage ••9555",
      provider: "schwab",
      capitalBucketID: "schwab-bucket",
      capitalReservationID: "schwab-reservation",
    };

    expect(
      projectStrategyFleetIdentityIsolation([coinbaseEngine, schwabCollision]),
    ).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        engineCount: 2,
        accountCount: 2,
        uniqueInstanceCount: 1,
        crossAccountCollisionCount: 1,
      }),
    );
    expect(
      projectStrategyFleetAccountIsolation([coinbaseEngine, schwabCollision]),
    ).toEqual([
      expect.objectContaining({ status: "UNAVAILABLE" }),
      expect.objectContaining({ status: "UNAVAILABLE" }),
    ]);
  });

  it("renders the read-only account isolation map", () => {
    render(<StrategyFleet items={[coinbaseEngine]} />);

    const isolation = screen.getByRole("region", {
      name: "Every engine stays inside its own policy claim.",
    });
    expect(isolation).toHaveTextContent("ACCOUNT ISOLATION MAP");
    expect(isolation).toHaveTextContent("Coinbase Portfolio ••••a5d0");
    expect(isolation).toHaveTextContent("Verified");
    expect(isolation).toHaveTextContent("$1,000");
    expect(isolation).toHaveTextContent("Shadow account ceiling$1,000");
    expect(isolation).toHaveTextContent(
      "Paper claims never consume broker cash · no broker hold · no order action",
    );
    const identity = screen.getByRole("region", {
      name: /Fleet identity isolation proof/i,
    });
    expect(identity).toHaveTextContent("Verified");
    expect(identity).toHaveTextContent("1 engines · 1 budgets · 1 claims");
    expect(identity).toHaveTextContent("Cross-account collisions0");
  });

  it("shows an owner-facing fleet summary with account and engine context", () => {
    render(
      <StrategyFleet
        items={[
          coinbaseEngine,
          {
            id: "rules-mandate",
            title: "Covered Call",
            accountName: "Schwab Brokerage ••9555",
            provider: "schwab",
            automationType: "RULES_BASED",
            mandateStatus: "DRAFT",
            autonomyLevel: "CONFIRM_EACH",
            executionMode: "PAPER",
            symbols: ["AAPL"],
            consecutiveFailures: 0,
          },
        ]}
      />,
    );

    const summary = screen.getByRole("region", { name: "Fleet summary" });
    expect(summary).toHaveTextContent("Monitoring1AI non-live engines");
    expect(summary).toHaveTextContent("Scheduled1healthy automatic cycles");
    expect(summary).toHaveTextContent("Attention0engine health signals");
    expect(summary).toHaveTextContent("Drafts1not initialized");
    const operatingBrief = screen.getByRole("region", {
      name: "Your AI engines are continuing safely.",
    });
    expect(operatingBrief).toHaveTextContent("No owner action");
    expect(operatingBrief).toHaveTextContent("1 of 1 on course");
    expect(operatingBrief).toHaveTextContent("AI chose to wait.");
    expect(operatingBrief).toHaveTextContent("Why this is okay");
    expect(operatingBrief).toHaveTextContent(
      "The next guarded cycle runs automatically.",
    );
    expect(operatingBrief).toHaveTextContent(
      "Paper and Shadow remain isolated · no broker order · no live path",
    );
    expect(
      within(operatingBrief).getByRole("link", {
        name: /Open engine evidence/i,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#runtime-evidence");
    expect(
      within(operatingBrief).getByRole("link", {
        name: /Open decision journal/i,
      }),
    ).toHaveAttribute("href", "/activity");
    const provenanceDigest = screen.getByRole("region", {
      name: "1 current AI decision is fully attributable.",
    });
    expect(provenanceDigest).toHaveTextContent("DECISION CHANGE + PROVENANCE");
    expect(provenanceDigest).toHaveTextContent("Held course");
    expect(
      within(provenanceDigest).getAllByText("No action proposed · $0"),
    ).toHaveLength(2);
    expect(provenanceDigest).toHaveTextContent("OpenAI · gpt-5.6-sol · Deep");
    expect(provenanceDigest).toHaveTextContent("Financial sourceCoinbase");
    expect(provenanceDigest).toHaveTextContent("BTC · ETH · XRP · SOL");
    expect(provenanceDigest).toHaveTextContent(
      "Rest Ticker · Real Time Single Venue",
    );
    expect(provenanceDigest).toHaveTextContent("no model rerun");
    expect(
      within(provenanceDigest).getByRole("link", {
        name: /Compare immutable records/i,
      }),
    ).toHaveAttribute("href", "/activity");
    const inputMatrix = screen.getByRole("region", {
      name: "Every current engine input is exactly attributable.",
    });
    expect(inputMatrix).toHaveTextContent("AI INPUT COVERAGE");
    expect(inputMatrix).toHaveTextContent("Financial input providerCoinbase");
    expect(inputMatrix).toHaveTextContent("Market priceAvailable");
    expect(inputMatrix).toHaveTextContent("Price historyAvailable");
    expect(inputMatrix).toHaveTextContent("LiquidityAvailable");
    expect(inputMatrix).toHaveTextContent("Position performanceUnavailable");
    expect(inputMatrix).toHaveTextContent("Market eventsUnavailable");
    expect(inputMatrix).toHaveTextContent(
      "it does not prove why the model chose its conclusion",
    );
    expect(
      within(inputMatrix).getByRole("link", {
        name: /Open immutable evidence/i,
      }),
    ).toHaveAttribute("href", "/activity");
    const inputChangeLedger = screen.getByRole("region", {
      name: "1 current AI engine has an exact input comparison.",
    });
    expect(inputChangeLedger).toHaveTextContent("INPUT COVERAGE CHANGE LEDGER");
    expect(inputChangeLedger).toHaveTextContent("Comparable engines1 / 1");
    expect(inputChangeLedger).toHaveTextContent("Improved inputs0");
    expect(inputChangeLedger).toHaveTextContent("Regressed inputs0");
    expect(inputChangeLedger).toHaveTextContent("Unchanged inputs5");
    expect(inputChangeLedger).toHaveTextContent("Coinbase");
    expect(inputChangeLedger).toHaveTextContent(
      "describe evidence coverage only—not decision quality or causality",
    );
    expect(
      within(inputChangeLedger).getByRole("link", {
        name: /Compare immutable records/i,
      }),
    ).toHaveAttribute("href", "/activity");
    expect(screen.getAllByText("Coinbase Portfolio ••••a5d0")).toHaveLength(2);
    expect(screen.getAllByText("gpt-5.6-sol")).toHaveLength(2);
    expect(screen.getAllByText("BTC · ETH · XRP +1")).toHaveLength(2);
    expect(screen.getByText("Healthy schedule")).toBeInTheDocument();
    const watchtower = screen.getByRole("region", {
      name: "1 guarded schedule is on course.",
    });
    expect(watchtower).toHaveTextContent("Verified");
    expect(watchtower).toHaveTextContent("Healthy schedules1 / 1");
    expect(watchtower).toHaveTextContent("Failure streaks0");
    expect(watchtower).toHaveTextContent("Completed safely");
    expect(watchtower).toHaveTextContent("On schedule");
    expect(watchtower).toHaveTextContent(
      "Paper or Shadow only · no manual cycle · no broker order",
    );
    const recovery = screen.getByRole("region", {
      name: "1 guarded engine has a verified recent path.",
    });
    expect(recovery).toHaveTextContent("Verified engines1 / 1");
    expect(recovery).toHaveTextContent("Recent records1");
    expect(recovery).toHaveTextContent("Recovered paths0");
    expect(recovery).toHaveTextContent("Open failures0");
    expect(recovery).toHaveTextContent("Stable");
    expect(recovery).toHaveTextContent("Completed");
    expect(recovery).toHaveTextContent(
      "No action needed. Recent guarded cycles are preserved",
    );
    expect(recovery).toHaveTextContent(
      "no automatic replay · no manual cycle · no broker order",
    );
    expect(
      within(recovery).getByRole("link", { name: /Review evidence/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    const decisionTrails = screen.getByRole("region", {
      name: "1 latest AI decision has a complete evidence trail.",
    });
    expect(decisionTrails).toHaveTextContent("Verified trails1 / 1");
    expect(decisionTrails).toHaveTextContent("Safe abstentions1");
    expect(decisionTrails).toHaveTextContent("Risk evaluations0");
    expect(decisionTrails).toHaveTextContent("Non-live records0");
    expect(decisionTrails).toHaveTextContent("None by design");
    expect(decisionTrails).toHaveTextContent("No proposal to evaluate");
    expect(decisionTrails).toHaveTextContent("Terminal safe decision");
    expect(decisionTrails).toHaveTextContent(
      "no model rerun · no provider call · no live execution path",
    );
    expect(
      within(decisionTrails).getByRole("link", {
        name: /Open decision journal/i,
      }),
    ).toHaveAttribute("href", "/activity");
    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
    });
    expect(runtimeContract).toHaveTextContent("PINNED v6");
    expect(runtimeContract).toHaveTextContent("Ready · immutable");
    expect(runtimeContract).toHaveTextContent("$1");
    expect(runtimeContract).toHaveTextContent("1 action / UTC day");
    expect(runtimeContract).toHaveTextContent("60 min · Continuous");
    expect(runtimeContract).toHaveTextContent("v6 · Matches runtime");
    expect(runtimeContract).toHaveTextContent(
      "Version and schedule identities match",
    );
    expect(
      within(runtimeContract).getByRole("link", {
        name: /Exact configuration/i,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#configuration-controls");
    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Verified");
    expect(dataHealth).toHaveTextContent("Active account · Active connection");
    expect(dataHealth).toHaveTextContent("Balances ready · Positions ready");
    expect(dataHealth).toHaveTextContent("Fresh ≤24h");
    expect(dataHealth).toHaveTextContent("no provider read or order action");
    expect(
      within(dataHealth).getByRole("link", { name: /Account evidence/i }),
    ).toHaveAttribute(
      "href",
      "/accounts/coinbase-account#reconciliation-title",
    );
    const capitalAuthority = screen.getByRole("region", {
      name: "AI Shadow Engine capital authority",
    });
    expect(capitalAuthority).toHaveTextContent("Bounded");
    expect(capitalAuthority).toHaveTextContent("Coinbase AI Shadow");
    expect(capitalAuthority).toHaveTextContent("$1,000");
    expect(capitalAuthority).toHaveTextContent("$0");
    expect(capitalAuthority).toHaveTextContent("Fixed budget capacity");
    expect(capitalAuthority).toHaveTextContent("Shadow");
    expect(capitalAuthority).toHaveTextContent(
      "no broker custody or execution authority",
    );
    expect(
      within(capitalAuthority).getByRole("link", { name: /Capital center/i }),
    ).toHaveAttribute("href", "/capital");
    expect(screen.getByText("Covered Call")).toBeInTheDocument();
    expect(screen.getByText("Deterministic rules")).toBeInTheDocument();
    expect(screen.getByText("Draft configuration")).toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("1 owner step");
    expect(
      within(queue).getByText("Finish reviewing Covered Call"),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Review draft/i }),
    ).toHaveAttribute(
      "href",
      "/automations/rules-mandate#mandate-lifecycle-controls",
    );
    expect(
      screen.getByRole("region", { name: "Execution boundary" }),
    ).toHaveTextContent("Neither mode can submit a broker order");
    expect(
      screen.getByRole("link", {
        name: "Open AI Shadow Engine for Coinbase Portfolio ••••a5d0",
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
  });

  it("treats AI Paper as an isolated healthy runtime without Shadow evidence or reconciliation", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            title: "AI Paper Engine",
            executionMode: "PAPER",
            capitalBucketName: "Coinbase AI Paper",
            capitalReservationAmount: "900.0000000000",
            capitalReservationBasis: "PAPER_STARTING_CASH",
            latestDecisionType: "ALLOW_SIMULATED_FILLED",
            latestDecisionSymbol: "BTC",
            latestDecisionSide: "BUY",
            latestDecisionQuantity: "0.0010000000",
            latestDecisionRiskDecision: "ALLOW",
            latestDecisionExecutionStatus: "SIMULATED_FILLED",
            evidenceAvailable: undefined,
            evidenceStatus: undefined,
            reconciliationAvailable: undefined,
            reconciliationComparisonStatus: undefined,
            reconciliationBalancesStatus: undefined,
            reconciliationPositionsStatus: undefined,
            reconciliationAutonomySignal: undefined,
            reconciliationAutonomyEnforcementActive: undefined,
            reconciliationBlocksNewActions: undefined,
            reconciliationObservedAt: undefined,
            reconciliationFresh: undefined,
          },
        ]}
      />,
    );

    expect(
      within(screen.getByRole("region", { name: "Strategy fleet" })).getByText(
        "Monitoring",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Healthy schedule")).toBeInTheDocument();
    const dataHealth = screen.getByRole("region", {
      name: "AI Paper Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("MARKET PRICE SOURCE");
    expect(dataHealth).toHaveTextContent("Broker portfolioNot used by Paper");
    expect(dataHealth).toHaveTextContent("Isolated Arbion simulation");
    expect(dataHealth).toHaveTextContent("no broker positions, cash, or order");
    const decisionPulse = screen.getByRole("region", {
      name: "AI Paper Engine latest AI decision",
    });
    expect(
      within(decisionPulse).getByText("Simulated fill"),
    ).toBeInTheDocument();
    expect(
      within(decisionPulse).getByText("Paper simulated fill only"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("region", {
        name: /AI Paper Engine Shadow evidence/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("surfaces a schedule outage instead of presenting healthy automation", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            scheduleAvailable: false,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
        ]}
      />,
    );

    const fleet = screen.getByRole("region", { name: "Strategy fleet" });
    expect(within(fleet).getByText("Needs review")).toBeInTheDocument();
    expect(
      within(fleet).getByText("Schedule status unavailable"),
    ).toBeInTheDocument();
    expect(
      within(fleet).queryByText("Healthy schedule"),
    ).not.toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByText(/Review AI Shadow Engine schedule health/i),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Review schedule/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
  });

  it("fails closed and hides partial capital values when the binding is invalid", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            capitalBindingValid: false,
          },
        ]}
      />,
    );

    const capitalAuthority = screen.getByRole("region", {
      name: "AI Shadow Engine capital authority",
    });
    expect(capitalAuthority).toHaveTextContent("Review required");
    expect(capitalAuthority).toHaveTextContent(
      "hidden until the complete owner-scoped capital binding can be verified",
    );
    expect(capitalAuthority).toHaveTextContent(
      "Database controls remain enforced · no provider funds moved",
    );
    expect(capitalAuthority).not.toHaveTextContent("$1,000");
    expect(
      within(capitalAuthority).getByRole("link", {
        name: /Review capital control/i,
      }),
    ).toHaveAttribute("href", "/capital");

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent(
      "Review AI Shadow Engine capital authority",
    );
    expect(queue).toHaveTextContent("no broker funds moved");
  });

  it("fails closed when the pinned version or schedule identity cannot be verified", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            runtimeBindingValid: false,
            runtimeScheduleBindingValid: false,
          },
        ]}
      />,
    );

    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
    });
    expect(runtimeContract).toHaveTextContent("Review required");
    expect(runtimeContract).toHaveTextContent(
      "hidden until the exact pinned mandate version and schedule binding can be verified",
    );
    expect(runtimeContract).not.toHaveTextContent("PINNED v6");
    expect(runtimeContract).not.toHaveTextContent("$1");
    expect(screen.getByText("Pinned model unavailable")).toBeInTheDocument();
    expect(screen.getByText("Pinned universe unavailable")).toBeInTheDocument();
    expect(
      within(runtimeContract).getByRole("link", {
        name: /Review runtime contract/i,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#configuration-controls");

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("Review AI Shadow Engine runtime contract");
    expect(queue).toHaveTextContent("no runtime setting or broker action");
  });

  it("keeps a newer editable draft separate from the immutable running version", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            mandateStatus: "DRAFT",
            runtimeMandateVersion: 6,
            currentMandateVersion: 7,
            newerDraftAvailable: true,
          },
        ]}
      />,
    );

    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
    });
    expect(runtimeContract).toHaveTextContent("PINNED v6");
    expect(runtimeContract).toHaveTextContent("v7 · Draft separate");
    expect(runtimeContract).toHaveTextContent(
      "Version and schedule identities match",
    );
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("Finish reviewing AI Shadow Engine");
  });

  it("does not label a newer ready configuration as the running version", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            mandateStatus: "READY",
            runtimeMandateVersion: 6,
            currentMandateVersion: 7,
            newerDraftAvailable: false,
          },
        ]}
      />,
    );

    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
    });
    expect(runtimeContract).toHaveTextContent("PINNED v6");
    expect(runtimeContract).toHaveTextContent("v7 · Ready separate");
    expect(runtimeContract).not.toHaveTextContent("v7 · Matches runtime");
  });

  it("shows a legacy missing daily ceiling without inventing a value", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            runtimeMaxTradesPerDay: undefined,
            runtimeLegacyDailyActionLimitMissing: true,
          },
        ]}
      />,
    );

    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
    });
    expect(runtimeContract).toHaveTextContent("Not recorded · legacy");
    expect(runtimeContract).toHaveTextContent(
      "Legacy daily ceiling is absent and not inferred",
    );
    expect(runtimeContract).not.toHaveTextContent("1 action / UTC day");
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("surfaces exact blocking portfolio drift ahead of later lifecycle work", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationComparisonStatus: "DRIFT_DETECTED",
            reconciliationAutonomySignal: "BLOCKED",
            reconciliationBlocksNewActions: true,
            reconciliationBlockingChangeCount: 2,
          },
        ]}
      />,
    );

    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Review required");
    expect(dataHealth).toHaveTextContent("Drift Detected");
    expect(dataHealth).toHaveTextContent(
      "New AI proposals are held by portfolio evidence",
    );
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(queue).toHaveTextContent("2 blocking changes recorded");
    expect(
      within(queue).getByRole("link", {
        name: /Review portfolio evidence/i,
      }),
    ).toHaveAttribute(
      "href",
      "/accounts/coinbase-account#reconciliation-title",
    );
    expect(
      screen.getByText("Portfolio drift blocks proposals"),
    ).toBeInTheDocument();
  });

  it("fails closed when connection state or reconciliation freshness is not current", () => {
    const { rerender } = render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            financialConnectionAvailable: false,
            financialConnectionStatus: undefined,
          },
        ]}
      />,
    );

    expect(
      screen.getByText("Connection status unavailable"),
    ).toBeInTheDocument();
    expect(
      screen
        .getAllByRole("link", { name: /Review connection/i })
        .every(
          (link) =>
            link.getAttribute("href") === "/connections#financial-accounts",
        ),
    ).toBe(true);

    rerender(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationFresh: false,
            reconciliationObservedAt: "2026-08-24T16:10:00Z",
          },
        ]}
      />,
    );

    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
    });
    expect(dataHealth).toHaveTextContent("Stale or invalid");
    expect(screen.getByText("Portfolio evidence is stale")).toBeInTheDocument();
    expect(
      screen.getAllByText(/older than the 24-hour autonomy threshold/i),
    ).toHaveLength(2);

    rerender(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            reconciliationAvailable: false,
            reconciliationComparisonStatus: undefined,
            reconciliationBalancesStatus: undefined,
            reconciliationPositionsStatus: undefined,
            reconciliationAutonomySignal: undefined,
            reconciliationAutonomyEnforcementActive: undefined,
            reconciliationBlocksNewActions: undefined,
            reconciliationObservedAt: undefined,
            reconciliationFresh: false,
          },
        ]}
      />,
    );

    expect(
      screen.getByText("Portfolio evidence unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(
        /will not infer balances, positions, or proposal readiness/i,
      ),
    ).toHaveLength(2);
  });

  it("shows a clear owner queue for a healthy scheduled fleet", () => {
    render(<StrategyFleet items={[coinbaseEngine]} />);

    const queue = screen.getByRole("region", {
      name: "No owner action right now.",
    });
    expect(queue).toHaveTextContent("0 owner steps");
    expect(queue).toHaveTextContent("healthy next cycle");
    expect(queue).toHaveTextContent(
      "opening a control does not run a cycle or authorize an order",
    );
    expect(within(queue).queryByRole("list")).not.toBeInTheDocument();
    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(evidence).toHaveTextContent("Collecting");
    expect(evidence).toHaveTextContent("12 / 20");
    expect(evidence).toHaveTextContent("4 / 20");
    expect(evidence).toHaveTextContent("48 / 168h");
    expect(evidence).toHaveTextContent("3 remaining conditions");
    expect(evidence).toHaveTextContent("Collect more 1-hour outcome marks");
    expect(evidence).toHaveTextContent(
      "Observe the mandate across a longer window",
    );
    expect(evidence).toHaveTextContent("never live authority");
    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Abstained");
    expect(pulse).toHaveTextContent("No action proposed");
    expect(pulse).toHaveTextContent("Risk gate not reached");
    expect(pulse).toHaveTextContent("No execution record");
    expect(pulse).toHaveTextContent("OpenAI · gpt-5.6-sol · Deep");
    expect(pulse).toHaveTextContent("1,842 ms · 12,540 in / 422 out");
    expect(
      within(pulse).getByRole("link", { name: /Decision journal/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#decision-journal");
  });

  it("shows a deterministic risk hold without presenting an order", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: "DENY_RISK_DENIED",
            latestDecisionSymbol: "BTC",
            latestDecisionSide: "BUY",
            latestDecisionQuantity: "0.001",
            latestDecisionRiskDecision: "DENY",
            latestDecisionRiskReasons: [
              "REPEAT_ACTION_COOLDOWN_ACTIVE",
              "MAX_TRADES_PER_DAY_REACHED",
            ],
            latestDecisionExecutionStatus: "RISK_DENIED",
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Held by controls");
    expect(pulse).toHaveTextContent("Buy · 0.001 · BTC");
    expect(pulse).toHaveTextContent("Repeat Action Cooldown Active +1");
    expect(pulse).toHaveTextContent("Risk Denied · non-live");
    expect(pulse).not.toHaveTextContent(/order submitted/i);
  });

  it("labels allowed evidence as Shadow-only would-have-submitted", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: "ALLOW_WOULD_HAVE_SUBMITTED",
            latestDecisionSymbol: "ETH",
            latestDecisionSide: "SELL",
            latestDecisionQuantity: "0.25",
            latestDecisionRiskDecision: "ALLOW",
            latestDecisionExecutionStatus: "WOULD_HAVE_SUBMITTED",
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Would have submitted");
    expect(pulse).toHaveTextContent("Sell · 0.25 · ETH");
    expect(pulse).toHaveTextContent("Allowed by deterministic controls");
    expect(pulse).toHaveTextContent("Shadow record only");
  });

  it("fails closed when the bounded decision journal is unavailable", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            decisionAvailable: false,
            latestDecisionType: undefined,
          },
        ]}
      />,
    );

    expect(screen.getByText("Latest decision unavailable")).toBeInTheDocument();
    expect(screen.queryByText("No action proposed")).not.toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /Refresh decision pulse/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
    expect(queue).toHaveTextContent("will not infer a recent AI action");
  });

  it("states when the bounded journal has no completed AI entry", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionType: undefined,
            latestDecisionAt: undefined,
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Awaiting a completed AI decision");
    expect(pulse).toHaveTextContent(
      "No AI entry appears in the latest 10 immutable journal records",
    );
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("keeps missing route provenance explicitly unattributed", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionAIProvider: undefined,
            latestDecisionAIModelID: undefined,
            latestDecisionAIProfile: undefined,
            latestDecisionLatencyMS: undefined,
            latestDecisionInputUsage: undefined,
            latestDecisionOutputUsage: undefined,
          },
        ]}
      />,
    );

    const pulse = screen.getByRole("region", {
      name: "AI Shadow Engine latest AI decision",
    });
    expect(pulse).toHaveTextContent("Unattributed legacy route");
    expect(pulse).toHaveTextContent("Telemetry unavailable");
    expect(pulse).not.toHaveTextContent("OpenAI");
  });

  it("surfaces an exact reviewable snapshot without granting authority", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceStatus: "EVIDENCE_REVIEWABLE",
            oneHourSampleSize: 22,
            twentyFourHourSampleSize: 20,
            evidenceWindowHours: 171,
            evidenceBlockers: [],
          },
        ]}
      />,
    );

    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(evidence).toHaveTextContent("Reviewable");
    expect(evidence).toHaveTextContent("22 / 20");
    expect(evidence).toHaveTextContent("20 / 20");
    expect(evidence).toHaveTextContent("171 / 168h");
    expect(evidence).toHaveTextContent("exact gate complete");
    expect(
      within(evidence).getByRole("progressbar", {
        name: "1-hour Shadow outcome sample progress",
      }),
    ).toHaveAttribute("value", "20");
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /evidence review/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#shadow-evidence-review");
    expect(queue).toHaveTextContent("grants no trading authority");
  });

  it("does not keep a currently reviewed snapshot in the owner queue", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceStatus: "EVIDENCE_REVIEWABLE",
            oneHourSampleSize: 20,
            twentyFourHourSampleSize: 20,
            evidenceWindowHours: 168,
            evidenceBlockers: [],
            currentEvidenceReviewed: true,
          },
        ]}
      />,
    );

    expect(screen.getByText("Current snapshot reviewed")).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "No owner action right now." }),
    ).toBeInTheDocument();
  });

  it("fails closed when the immutable evidence scorecard is unavailable", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            evidenceAvailable: false,
            evidenceStatus: undefined,
          },
        ]}
      />,
    );

    const evidence = screen.getByRole("region", {
      name: "AI Shadow Engine Shadow evidence",
    });
    expect(
      within(evidence).getByText("Evidence status unavailable"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/12 \/ 20/)).not.toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /Refresh evidence/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate");
    expect(queue).toHaveTextContent(
      "will not infer its sample or review status",
    );
  });

  it("orders failed schedules before draft and paused owner choices", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            id: "paused",
            accountName: "Paused account",
            instanceStatus: "PAUSED",
            scheduleEnabled: false,
            nextRunAt: undefined,
          },
          {
            ...coinbaseEngine,
            id: "draft",
            accountName: "Draft account",
            mandateStatus: "DRAFT",
            instanceStatus: undefined,
            currentState: undefined,
            scheduleAvailable: undefined,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
          {
            ...coinbaseEngine,
            id: "failed",
            accountName: "Failed account",
            scheduleStatus: "FAILED",
            consecutiveFailures: 2,
          },
        ]}
      />,
    );

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    const actions = within(queue).getAllByRole("listitem");
    expect(actions).toHaveLength(3);
    expect(actions[0]).toHaveTextContent(
      "Review AI Shadow Engine schedule health",
    );
    expect(actions[1]).toHaveTextContent("Finish reviewing AI Shadow Engine");
    expect(actions[2]).toHaveTextContent(
      "Decide when to resume AI Shadow Engine",
    );
  });

  it("does not mislabel a completed immutable version as ready to initialize", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            instanceStatus: "COMPLETED",
            currentState: "AI_MONITORING",
            scheduleAvailable: undefined,
            scheduleEnabled: undefined,
            scheduleStatus: undefined,
            nextRunAt: undefined,
          },
        ]}
      />,
    );

    expect(screen.getByText("New version required")).toBeInTheDocument();
    expect(screen.getByText("Historical version complete")).toBeInTheDocument();
    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(
      within(queue).getByRole("link", { name: /version controls/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#configuration-controls");
  });

  it("collapses repeated partial-context warnings into one fail-closed action", () => {
    render(
      <StrategyFleet
        contextWarnings={["Current engine state could not be refreshed."]}
        items={[
          {
            ...coinbaseEngine,
            id: "first",
            instanceContextAvailable: false,
          },
          {
            ...coinbaseEngine,
            id: "second",
            accountName: "Second account",
            instanceContextAvailable: false,
          },
        ]}
      />,
    );

    const queue = screen.getByRole("region", {
      name: "Your clearest path forward.",
    });
    expect(within(queue).getAllByRole("listitem")).toHaveLength(1);
    expect(
      within(queue).getByText("Refresh the current fleet context"),
    ).toBeInTheDocument();
    expect(
      within(queue).getByRole("link", { name: /Refresh automations/i }),
    ).toHaveAttribute("href", "/automations");
    expect(queue).toHaveTextContent("No mandate or schedule was changed");
  });

  it("keeps the empty state focused on a bounded non-live launch", () => {
    render(<StrategyFleet items={[]} />);

    expect(screen.getByText("No strategies yet.")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Launch an AI Engine" }),
    ).toHaveAttribute("href", "/automations/new");
  });

  it("does not present an inventory outage as an empty fleet", () => {
    render(
      <StrategyFleet
        contextWarnings={["Current engine state could not be refreshed."]}
        inventoryAvailable={false}
        items={[]}
      />,
    );

    expect(
      screen.getByText("Strategies could not be loaded."),
    ).toBeInTheDocument();
    expect(screen.queryByText("No strategies yet.")).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Return to dashboard" }),
    ).toHaveAttribute("href", "/dashboard");
  });
});
