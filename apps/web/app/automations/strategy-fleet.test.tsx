import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  projectStrategyFleetAccountIsolation,
  projectStrategyFleetAutomaticCycleIncidents,
  projectStrategyFleetAutomaticCycleFailureTaxonomy,
  projectStrategyFleetAutomaticRecoveryRTO,
  projectStrategyFleetAutomaticCycleSLOHistory,
  projectStrategyFleetReliabilityCenter,
  projectStrategyFleetDecisionEvidence,
  projectStrategyFleetEvidenceFreshnessBoard,
  projectStrategyFleetIdentityIsolation,
  projectStrategyFleetInputCoverageChangeLedger,
  projectStrategyFleetInputCoverageMatrix,
  projectStrategyFleetPersistentInputGapRegister,
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
  freshnessObservedAt: "2026-08-26T16:30:00Z",
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
  recentDecisionInputCoverage: [
    {
      decisionID: "decision-abstain",
      decisionAt: "2026-08-26T16:17:39Z",
      financialProvider: "coinbase",
      financialContextComplete: true,
      inputCoverageComplete: true,
      historyLiquidityEvidenceComplete: true,
      marketSymbols: ["BTC", "ETH", "XRP", "SOL"],
      marketFeeds: ["rest_ticker"],
      marketQualities: ["REAL_TIME_SINGLE_VENUE"],
      marketObservedAt: "2026-08-26T16:17:38Z",
      historyStatuses: ["COMPLETE"],
      historyFeeds: ["coinbase_candles"],
      historyQualities: ["REAL_TIME_SINGLE_VENUE"],
      liquidityStatuses: ["AVAILABLE"],
      positionEvidenceComplete: true,
      positionCount: 2,
      positionPerformanceStatuses: ["UNAVAILABLE"],
      marketEventEvidenceComplete: true,
      marketEventCoverageCount: 0,
      marketEventCoverageStatuses: [],
      marketEventProviders: [],
      marketEventFeeds: [],
      marketEventQualities: [],
      marketEventCount: 0,
    },
    {
      decisionID: "decision-abstain-prior",
      decisionAt: "2026-08-26T15:17:39Z",
      financialProvider: "coinbase",
      financialContextComplete: true,
      inputCoverageComplete: true,
      historyLiquidityEvidenceComplete: true,
      marketSymbols: ["BTC", "ETH", "XRP", "SOL"],
      marketFeeds: ["rest_ticker"],
      marketQualities: ["REAL_TIME_SINGLE_VENUE"],
      marketObservedAt: "2026-08-26T15:17:38Z",
      historyStatuses: ["COMPLETE"],
      historyFeeds: ["coinbase_candles"],
      historyQualities: ["REAL_TIME_SINGLE_VENUE"],
      liquidityStatuses: ["AVAILABLE"],
      positionEvidenceComplete: true,
      positionCount: 2,
      positionPerformanceStatuses: ["UNAVAILABLE"],
      marketEventEvidenceComplete: true,
      marketEventCoverageCount: 0,
      marketEventCoverageStatuses: [],
      marketEventProviders: [],
      marketEventFeeds: [],
      marketEventQualities: [],
      marketEventCount: 0,
    },
    {
      decisionID: "decision-abstain-oldest",
      decisionAt: "2026-08-26T14:17:39Z",
      financialProvider: "coinbase",
      financialContextComplete: true,
      inputCoverageComplete: true,
      historyLiquidityEvidenceComplete: true,
      marketSymbols: ["BTC", "ETH", "XRP", "SOL"],
      marketFeeds: ["rest_ticker"],
      marketQualities: ["REAL_TIME_SINGLE_VENUE"],
      marketObservedAt: "2026-08-26T14:17:38Z",
      historyStatuses: ["COMPLETE"],
      historyFeeds: ["coinbase_candles"],
      historyQualities: ["REAL_TIME_SINGLE_VENUE"],
      liquidityStatuses: ["AVAILABLE"],
      positionEvidenceComplete: true,
      positionCount: 2,
      positionPerformanceStatuses: ["UNAVAILABLE"],
      marketEventEvidenceComplete: true,
      marketEventCoverageCount: 0,
      marketEventCoverageStatuses: [],
      marketEventProviders: [],
      marketEventFeeds: [],
      marketEventQualities: [],
      marketEventCount: 0,
    },
  ],
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
      hidden: true,
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

  it("registers repeated gaps over a bounded immutable decision window", () => {
    const register = projectStrategyFleetPersistentInputGapRegister([
      coinbaseEngine,
    ]);

    expect(register).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        attributableCount: 1,
        persistentCount: 2,
        intermittentCount: 0,
        newlyMissingCount: 0,
        resolvedCount: 0,
      }),
    );
    expect(register.engines[0]).toEqual(
      expect.objectContaining({
        sampleCount: 3,
        windowStartedAt: "2026-08-26T14:17:39Z",
        windowEndedAt: "2026-08-26T16:17:39Z",
        attributable: true,
      }),
    );
    expect(register.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "HISTORY",
          state: "CLEAR",
          gapSampleCount: 0,
        }),
        expect.objectContaining({
          key: "POSITION",
          state: "PERSISTENT",
          gapSampleCount: 3,
          firstGapAt: "2026-08-26T14:17:39Z",
          latestGapAt: "2026-08-26T16:17:39Z",
        }),
        expect.objectContaining({
          key: "EVENTS",
          state: "PERSISTENT",
          gapSampleCount: 3,
        }),
      ]),
    );
  });

  it("distinguishes intermittent, newly missing, resolved, and context-changed gaps", () => {
    const snapshots = coinbaseEngine.recentDecisionInputCoverage!.map(
      (snapshot, index) => ({
        ...snapshot,
        historyStatuses: index === 0 ? ["UNAVAILABLE"] : ["COMPLETE"],
        historyFeeds: index === 0 ? [] : ["coinbase_candles"],
        historyQualities: index === 0 ? [] : ["REAL_TIME_SINGLE_VENUE"],
        liquidityStatuses: index === 1 ? ["AVAILABLE"] : ["UNAVAILABLE"],
        positionCount: index === 0 ? 2 : index === 1 ? 0 : 2,
        positionPerformanceStatuses:
          index === 0 ? ["AVAILABLE"] : index === 1 ? [] : ["UNAVAILABLE"],
      }),
    );
    const register = projectStrategyFleetPersistentInputGapRegister([
      { ...coinbaseEngine, recentDecisionInputCoverage: snapshots },
    ]);

    expect(register.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "HISTORY", state: "NEWLY_MISSING" }),
        expect.objectContaining({ key: "LIQUIDITY", state: "INTERMITTENT" }),
        expect.objectContaining({
          key: "POSITION",
          state: "CONTEXT_CHANGED",
        }),
      ]),
    );

    const resolved = projectStrategyFleetPersistentInputGapRegister([
      {
        ...coinbaseEngine,
        recentDecisionInputCoverage: snapshots.map((snapshot, index) => ({
          ...snapshot,
          positionCount: 2,
          positionPerformanceStatuses:
            index === 0 ? ["AVAILABLE"] : ["UNAVAILABLE"],
        })),
      },
    ]);
    expect(resolved.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "POSITION",
          state: "CURRENTLY_RESOLVED",
        }),
      ]),
    );
  });

  it("fails the persistent gap register closed on a short or mismatched window", () => {
    const short = projectStrategyFleetPersistentInputGapRegister([
      {
        ...coinbaseEngine,
        recentDecisionInputCoverage:
          coinbaseEngine.recentDecisionInputCoverage!.slice(0, 1),
      },
    ]);
    expect(short).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", attributableCount: 0 }),
    );

    const mismatched = projectStrategyFleetPersistentInputGapRegister([
      {
        ...coinbaseEngine,
        recentDecisionInputCoverage:
          coinbaseEngine.recentDecisionInputCoverage!.map((snapshot, index) =>
            index === 1
              ? { ...snapshot, financialProvider: "schwab" }
              : snapshot,
          ),
      },
    ]);
    expect(mismatched).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", attributableCount: 0 }),
    );
    expect(mismatched.engines[0].categories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ state: "UNAVAILABLE", gapSampleCount: 0 }),
      ]),
    );
  });

  it("measures exact saved evidence freshness against the pinned cycle", () => {
    const board = projectStrategyFleetEvidenceFreshnessBoard([coinbaseEngine]);

    expect(board).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        currentCount: 1,
        nearingStaleCount: 0,
        staleCount: 0,
        safeWaitCount: 0,
        unavailableCount: 0,
      }),
    );
    expect(board.engines[0]).toEqual(
      expect.objectContaining({
        state: "CURRENT",
        intervalMinutes: 60,
        ageThresholdMinutes: 65,
        nextDueGraceMinutes: 5,
      }),
    );
    expect(board.engines[0].metrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "DECISION",
          state: "CURRENT",
          ageMinutes: 12,
          thresholdMinutes: 65,
        }),
        expect.objectContaining({
          key: "NEXT_DUE",
          state: "CURRENT",
          minutesUntilDue: 47,
          thresholdMinutes: 5,
        }),
      ]),
    );
  });

  it("uses exact nearing-stale, stale, and overdue-grace boundaries", () => {
    const nearing = projectStrategyFleetEvidenceFreshnessBoard([
      {
        ...coinbaseEngine,
        freshnessObservedAt: "2026-08-26T17:12:39Z",
        nextRunAt: "2026-08-26T17:07:39Z",
      },
    ]);
    expect(nearing.engines[0]).toEqual(
      expect.objectContaining({ state: "NEARING_STALE" }),
    );
    expect(nearing.engines[0].metrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "DECISION",
          state: "NEARING_STALE",
          ageMinutes: 55,
        }),
        expect.objectContaining({
          key: "NEXT_DUE",
          state: "NEARING_STALE",
          minutesUntilDue: -5,
        }),
      ]),
    );

    const stale = projectStrategyFleetEvidenceFreshnessBoard([
      {
        ...coinbaseEngine,
        freshnessObservedAt: "2026-08-26T17:22:39.001Z",
        nextRunAt: "2026-08-26T17:17:39Z",
      },
    ]);
    expect(stale).toEqual(
      expect.objectContaining({ status: "ATTENTION", staleCount: 1 }),
    );
    expect(stale.engines[0].metrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "DECISION", state: "STALE" }),
        expect.objectContaining({ key: "NEXT_DUE", state: "STALE" }),
      ]),
    );
  });

  it("distinguishes an exact market-session safe wait from stale evidence", () => {
    const board = projectStrategyFleetEvidenceFreshnessBoard([
      {
        ...coinbaseEngine,
        id: "schwab-shadow",
        title: "Schwab AI Shadow Engine",
        provider: "schwab",
        accountName: "Schwab Brokerage ••••1000",
        freshnessObservedAt: "2026-08-30T17:47:56Z",
        latestDecisionAt: "2026-08-28T19:35:12Z",
        latestDecisionMarketObservedAt: "2026-08-28T19:35:10Z",
        scheduleLastCompletedAt: "2026-08-28T19:35:13Z",
        scheduleStatus: "SKIPPED",
        scheduleErrorCode: "OUTSIDE_SESSION",
        nextRunAt: "2026-08-31T13:35:00Z",
      },
    ]);

    expect(board).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        safeWaitCount: 1,
        staleCount: 0,
      }),
    );
    expect(board.engines[0]).toEqual(
      expect.objectContaining({
        state: "SESSION_SAFE_WAIT",
        followUp: expect.stringContaining("next configured market session"),
      }),
    );
    expect(board.engines[0].metrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "DECISION",
          state: "SESSION_SAFE_WAIT",
        }),
        expect.objectContaining({
          key: "NEXT_DUE",
          state: "SESSION_SAFE_WAIT",
        }),
      ]),
    );
  });

  it("fails freshness closed for missing or future saved timestamps", () => {
    const board = projectStrategyFleetEvidenceFreshnessBoard([
      {
        ...coinbaseEngine,
        latestDecisionAt: "2026-08-26T16:30:00.001Z",
        latestDecisionMarketObservedAt: undefined,
      },
    ]);

    expect(board).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        unavailableCount: 1,
      }),
    );
    expect(board.engines[0].metrics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "DECISION", state: "UNAVAILABLE" }),
        expect.objectContaining({ key: "MARKET", state: "UNAVAILABLE" }),
      ]),
    );
  });

  it("measures exact automatic-cycle SLO history and preserves recovery", () => {
    const history = projectStrategyFleetAutomaticCycleSLOHistory([
      {
        ...coinbaseEngine,
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
      },
    ]);

    expect(history).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        verifiedCount: 1,
        recoveredCount: 1,
        totalSampleCount: 2,
        totalFailureCount: 1,
        totalSafeWaitCount: 0,
      }),
    );
    expect(history.engines[0]).toEqual(
      expect.objectContaining({
        state: "RECOVERED",
        sampleCount: 2,
        successCount: 1,
        successRatePercent: 50,
        sloAttainmentCount: 2,
        sloAttainmentPercent: 100,
        latestLatencySeconds: 39,
        averageLatencySeconds: 22,
        maximumLatencySeconds: 39,
        latestBreachAt: "2026-08-26T15:17:05Z",
        latestRecoveryAt: "2026-08-26T16:17:39Z",
      }),
    );
  });

  it("consolidates stable, recovered, and safe-wait evidence without dropping exact counts", () => {
    const recovered: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleRecentRuns: [
        ...coinbaseEngine.scheduleRecentRuns!,
        {
          id: "schedule-run-failed",
          scheduledFor: "2026-08-26T15:17:00Z",
          completedAt: "2026-08-26T15:17:05Z",
          nextRunAt: "2026-08-26T16:17:00Z",
          status: "FAILED",
          errorCode: "INTERNAL",
          duplicateRecovered: false,
          consecutiveFailures: 1,
        },
      ],
    };
    const safeWait: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      capitalBucketID: "schwab-shadow-bucket",
      capitalReservationID: "schwab-shadow-reservation",
      provider: "schwab",
      accountName: "Schwab Brokerage ••••1000",
      scheduleStatus: "SKIPPED",
      scheduleErrorCode: "OUTSIDE_SESSION",
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "SKIPPED",
          errorCode: "OUTSIDE_SESSION",
        },
      ],
    };

    const center = projectStrategyFleetReliabilityCenter([recovered, safeWait]);

    expect(center).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 2,
        stableCount: 0,
        recoveredCount: 1,
        safeWaitCount: 1,
        attentionCount: 0,
        unavailableCount: 0,
        savedFailureCount: 1,
        recoveredFailureCount: 1,
        currentFailureCount: 0,
        recoveredIncidentCount: 1,
        currentIncidentCount: 0,
        medianRecoverySeconds: 3634,
        maximumRecoverySeconds: 3634,
      }),
    );
    expect(center.engines).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "ai-mandate",
          state: "RECOVERED",
          failureCount: 1,
          recoveredIncidentCount: 1,
        }),
        expect.objectContaining({
          id: "schwab-shadow",
          state: "SAFE_WAIT",
          safeWaitCount: 1,
        }),
      ]),
    );
  });

  it("elevates current and incomplete reliability evidence for owner review", () => {
    const attention = projectStrategyFleetReliabilityCenter([
      {
        ...coinbaseEngine,
        scheduleStatus: "FAILED",
        scheduleErrorCode: "STRUCTURED_OUTPUT_MISSING",
        consecutiveFailures: 1,
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            status: "FAILED",
            errorCode: "STRUCTURED_OUTPUT_MISSING",
            consecutiveFailures: 1,
          },
        ],
      },
    ]);
    expect(attention).toEqual(
      expect.objectContaining({
        status: "ATTENTION",
        attentionCount: 1,
        currentFailureCount: 1,
        currentIncidentCount: 1,
      }),
    );
    expect(attention.engines[0]).toEqual(
      expect.objectContaining({ state: "ATTENTION" }),
    );

    const unavailable = projectStrategyFleetReliabilityCenter([
      {
        ...coinbaseEngine,
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            id: undefined,
          },
        ],
      },
    ]);
    expect(unavailable).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        unavailableCount: 1,
      }),
    );
    expect(unavailable.engines[0]).toEqual(
      expect.objectContaining({ state: "UNAVAILABLE" }),
    );
  });

  it("uses the exact five-minute completion boundary", () => {
    const atBoundary: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          scheduledFor: "2026-08-26T16:12:39Z",
        },
      ],
    };
    const inside = projectStrategyFleetAutomaticCycleSLOHistory([atBoundary]);
    expect(inside.engines[0]).toEqual(
      expect.objectContaining({
        state: "STABLE",
        latestLatencySeconds: 300,
        sloAttainmentPercent: 100,
      }),
    );

    const breached = projectStrategyFleetAutomaticCycleSLOHistory([
      {
        ...atBoundary,
        scheduleRecentRuns: [
          {
            ...atBoundary.scheduleRecentRuns![0],
            scheduledFor: "2026-08-26T16:12:38.999Z",
          },
        ],
      },
    ]);
    expect(breached).toEqual(
      expect.objectContaining({ status: "ATTENTION", attentionCount: 1 }),
    );
    expect(breached.engines[0]).toEqual(
      expect.objectContaining({
        state: "ATTENTION",
        latestLatencySeconds: 300.001,
        sloAttainmentPercent: 0,
      }),
    );
  });

  it("classifies an exact session wait and fails future history closed", () => {
    const safeWait = projectStrategyFleetAutomaticCycleSLOHistory([
      {
        ...coinbaseEngine,
        scheduleStatus: "SKIPPED",
        scheduleErrorCode: "OUTSIDE_SESSION",
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            status: "SKIPPED",
            errorCode: "OUTSIDE_SESSION",
          },
        ],
      },
    ]);
    expect(safeWait).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        totalSafeWaitCount: 1,
      }),
    );
    expect(safeWait.engines[0]).toEqual(
      expect.objectContaining({ state: "SAFE_WAIT", successRatePercent: 0 }),
    );

    const unavailable = projectStrategyFleetAutomaticCycleSLOHistory([
      {
        ...coinbaseEngine,
        scheduleLastCompletedAt: "2026-08-26T16:30:00.001Z",
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            completedAt: "2026-08-26T16:30:00.001Z",
          },
        ],
      },
    ]);
    expect(unavailable).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", verifiedCount: 0 }),
    );
    expect(unavailable.engines[0].state).toBe("UNAVAILABLE");
  });

  it("counts exact saved failure codes and automatic recovery", () => {
    const taxonomy = projectStrategyFleetAutomaticCycleFailureTaxonomy([
      {
        ...coinbaseEngine,
        scheduleRecentRuns: [
          ...coinbaseEngine.scheduleRecentRuns!,
          {
            id: "schedule-run-internal-latest",
            scheduledFor: "2026-08-26T15:17:00Z",
            completedAt: "2026-08-26T15:17:05Z",
            nextRunAt: "2026-08-26T16:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 2,
          },
          {
            id: "schedule-run-internal-first",
            scheduledFor: "2026-08-26T14:17:00Z",
            completedAt: "2026-08-26T14:17:04Z",
            nextRunAt: "2026-08-26T15:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(taxonomy).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        engineCount: 1,
        verifiedCount: 1,
        totalFailureCount: 2,
        recoveredFailureCount: 2,
        currentFailureCount: 0,
        safeWaitCount: 0,
      }),
    );
    expect(taxonomy.codes).toEqual([
      expect.objectContaining({
        code: "INTERNAL",
        count: 2,
        recoveredCount: 2,
        currentCount: 0,
        affectedEngineCount: 1,
        firstFailureAt: "2026-08-26T14:17:04Z",
        latestFailureAt: "2026-08-26T15:17:05Z",
        executionModes: ["SHADOW"],
        providers: ["coinbase"],
      }),
    ]);
    expect(taxonomy.engines[0]).toEqual(
      expect.objectContaining({
        state: "RECOVERED",
        failureCount: 2,
        recoveredFailureCount: 2,
        currentFailureCount: 0,
      }),
    );
  });

  it("separates a current failure and exact session wait", () => {
    const currentFailure: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleStatus: "FAILED",
      scheduleErrorCode: "AI_STRUCTURED_OUTPUT_MISSING",
      scheduleLastCompletedAt: "2026-08-26T16:17:39Z",
      consecutiveFailures: 1,
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "FAILED",
          errorCode: "AI_STRUCTURED_OUTPUT_MISSING",
          consecutiveFailures: 1,
        },
      ],
    };
    const safeWait: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      capitalBucketID: "schwab-shadow-bucket",
      capitalReservationID: "schwab-shadow-reservation",
      provider: "schwab",
      accountName: "Schwab Brokerage ••••1000",
      scheduleStatus: "SKIPPED",
      scheduleErrorCode: "OUTSIDE_SESSION",
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "SKIPPED",
          errorCode: "OUTSIDE_SESSION",
        },
      ],
    };
    const taxonomy = projectStrategyFleetAutomaticCycleFailureTaxonomy([
      currentFailure,
      safeWait,
    ]);

    expect(taxonomy).toEqual(
      expect.objectContaining({
        status: "ATTENTION",
        currentFailureCount: 1,
        recoveredFailureCount: 0,
        safeWaitCount: 1,
      }),
    );
    expect(taxonomy.engines).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "ai-mandate",
          state: "ATTENTION",
          currentErrorCode: "AI_STRUCTURED_OUTPUT_MISSING",
        }),
        expect.objectContaining({
          id: "schwab-shadow",
          state: "SAFE_WAIT",
          failureCount: 0,
          safeWaitCount: 1,
        }),
      ]),
    );
  });

  it("fails the taxonomy closed when a failed run has no classification", () => {
    const taxonomy = projectStrategyFleetAutomaticCycleFailureTaxonomy([
      {
        ...coinbaseEngine,
        scheduleStatus: "FAILED",
        consecutiveFailures: 1,
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            status: "FAILED",
            errorCode: undefined,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(taxonomy).toEqual(
      expect.objectContaining({
        status: "UNAVAILABLE",
        verifiedCount: 0,
        totalFailureCount: 0,
      }),
    );
    expect(taxonomy.engines[0]).toEqual(
      expect.objectContaining({ state: "UNAVAILABLE", verified: false }),
    );
  });

  it("measures exact first-success recovery time for saved failures", () => {
    const recovery = projectStrategyFleetAutomaticRecoveryRTO([
      {
        ...coinbaseEngine,
        scheduleRecentRuns: [
          ...coinbaseEngine.scheduleRecentRuns!,
          {
            id: "schedule-run-internal-latest",
            scheduledFor: "2026-08-26T15:17:00Z",
            completedAt: "2026-08-26T15:17:05Z",
            nextRunAt: "2026-08-26T16:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 2,
          },
          {
            id: "schedule-run-internal-first",
            scheduledFor: "2026-08-26T14:17:00Z",
            completedAt: "2026-08-26T14:17:04Z",
            nextRunAt: "2026-08-26T15:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(recovery).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        recoveredFailureCount: 2,
        currentFailureCount: 0,
        medianRecoverySeconds: 5434.5,
        maximumRecoverySeconds: 7235,
        latestRecoveryAt: "2026-08-26T16:17:39Z",
      }),
    );
    expect(recovery.codes).toEqual([
      expect.objectContaining({
        code: "INTERNAL",
        recoveredCount: 2,
        currentCount: 0,
        medianRecoverySeconds: 5434.5,
        maximumRecoverySeconds: 7235,
        affectedEngineCount: 1,
        executionModes: ["SHADOW"],
        providers: ["coinbase"],
      }),
    ]);
    expect(recovery.engines[0]).toEqual(
      expect.objectContaining({ state: "RECOVERED", sampleCount: 3 }),
    );
  });

  it("keeps unrecovered age and exact session waits separate", () => {
    const failed: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleStatus: "FAILED",
      scheduleErrorCode: "AI_STRUCTURED_OUTPUT_MISSING",
      consecutiveFailures: 1,
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "FAILED",
          errorCode: "AI_STRUCTURED_OUTPUT_MISSING",
          consecutiveFailures: 1,
        },
      ],
    };
    const safeWait: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      provider: "schwab",
      accountName: "Schwab Brokerage ••••1000",
      scheduleStatus: "SKIPPED",
      scheduleErrorCode: "OUTSIDE_SESSION",
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "SKIPPED",
          errorCode: "OUTSIDE_SESSION",
        },
      ],
    };
    const recovery = projectStrategyFleetAutomaticRecoveryRTO([
      failed,
      safeWait,
    ]);

    expect(recovery).toEqual(
      expect.objectContaining({
        status: "ATTENTION",
        currentFailureCount: 1,
        recoveredFailureCount: 0,
        safeWaitCount: 1,
        maximumCurrentUnrecoveredAgeSeconds: 741,
      }),
    );
    expect(recovery.engines).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "ai-mandate",
          state: "ATTENTION",
          maximumCurrentUnrecoveredAgeSeconds: 741,
        }),
        expect.objectContaining({
          id: "schwab-shadow",
          state: "SAFE_WAIT",
          safeWaitCount: 1,
        }),
      ]),
    );
  });

  it("fails recovery timing closed when a later cycle completed before its failure", () => {
    const recovery = projectStrategyFleetAutomaticRecoveryRTO([
      {
        ...coinbaseEngine,
        scheduleLastCompletedAt: "2026-08-26T16:18:00Z",
        scheduleRecentRuns: [
          {
            ...coinbaseEngine.scheduleRecentRuns![0],
            completedAt: "2026-08-26T16:18:00Z",
          },
          {
            id: "schedule-run-impossible-failure",
            scheduledFor: "2026-08-26T15:17:00Z",
            completedAt: "2026-08-26T16:20:00Z",
            nextRunAt: "2026-08-26T16:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(recovery).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", verifiedCount: 0 }),
    );
    expect(recovery.engines[0]).toEqual(
      expect.objectContaining({ state: "UNAVAILABLE", verified: false }),
    );
  });

  it("groups consecutive saved failures into one recovered incident", () => {
    const timeline = projectStrategyFleetAutomaticCycleIncidents([
      {
        ...coinbaseEngine,
        scheduleRecentRuns: [
          ...coinbaseEngine.scheduleRecentRuns!,
          {
            id: "schedule-run-contract-failure",
            scheduledFor: "2026-08-26T15:17:00Z",
            completedAt: "2026-08-26T15:17:05Z",
            nextRunAt: "2026-08-26T16:17:00Z",
            status: "FAILED",
            errorCode: "AI_REQUEST_INVALID",
            duplicateRecovered: false,
            consecutiveFailures: 2,
          },
          {
            id: "schedule-run-internal-failure",
            scheduledFor: "2026-08-26T14:17:00Z",
            completedAt: "2026-08-26T14:17:04Z",
            nextRunAt: "2026-08-26T15:17:00Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(timeline).toEqual(
      expect.objectContaining({
        status: "VERIFIED",
        incidentCount: 1,
        recoveredIncidentCount: 1,
        currentIncidentCount: 0,
        latestRecoveryAt: "2026-08-26T16:17:39Z",
      }),
    );
    expect(timeline.engines[0]).toEqual(
      expect.objectContaining({ state: "RECOVERED", sampleCount: 3 }),
    );
    expect(timeline.engines[0].incidents).toEqual([
      expect.objectContaining({
        failureRunIDs: [
          "schedule-run-internal-failure",
          "schedule-run-contract-failure",
        ],
        failureCount: 2,
        failureStages: ["INTERNAL", "AI_REQUEST_INVALID"],
        startedAt: "2026-08-26T14:17:04Z",
        latestFailureAt: "2026-08-26T15:17:05Z",
        recoveredAt: "2026-08-26T16:17:39Z",
        recoverySeconds: 7235,
        state: "RECOVERED",
      }),
    ]);
  });

  it("keeps recovered and current incidents distinct while separating safe waits", () => {
    const current: StrategyFleetItem = {
      ...coinbaseEngine,
      scheduleStatus: "FAILED",
      scheduleErrorCode: "STRUCTURED_OUTPUT_MISSING",
      consecutiveFailures: 1,
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "FAILED",
          errorCode: "STRUCTURED_OUTPUT_MISSING",
          consecutiveFailures: 1,
        },
        {
          id: "schedule-run-prior-success",
          scheduledFor: "2026-08-26T15:17:00Z",
          completedAt: "2026-08-26T15:17:39Z",
          nextRunAt: "2026-08-26T16:17:00Z",
          status: "SUCCEEDED",
          duplicateRecovered: false,
          consecutiveFailures: 0,
        },
        {
          id: "schedule-run-prior-failure",
          scheduledFor: "2026-08-26T14:17:00Z",
          completedAt: "2026-08-26T14:17:04Z",
          nextRunAt: "2026-08-26T15:17:00Z",
          status: "FAILED",
          errorCode: "INTERNAL",
          duplicateRecovered: false,
          consecutiveFailures: 1,
        },
      ],
    };
    const safeWait: StrategyFleetItem = {
      ...coinbaseEngine,
      id: "schwab-shadow",
      strategyInstanceID: "schwab-shadow-instance",
      provider: "schwab",
      accountName: "Schwab Brokerage ••••1000",
      scheduleStatus: "SKIPPED",
      scheduleErrorCode: "OUTSIDE_SESSION",
      scheduleRecentRuns: [
        {
          ...coinbaseEngine.scheduleRecentRuns![0],
          status: "SKIPPED",
          errorCode: "OUTSIDE_SESSION",
        },
      ],
    };
    const timeline = projectStrategyFleetAutomaticCycleIncidents([
      current,
      safeWait,
    ]);

    expect(timeline).toEqual(
      expect.objectContaining({
        status: "ATTENTION",
        incidentCount: 2,
        recoveredIncidentCount: 1,
        currentIncidentCount: 1,
        safeWaitCount: 1,
      }),
    );
    expect(timeline.engines[0].incidents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ state: "RECOVERED", recoverySeconds: 3635 }),
        expect.objectContaining({ state: "CURRENT", currentAgeSeconds: 741 }),
      ]),
    );
    expect(timeline.engines[1]).toEqual(
      expect.objectContaining({ state: "SAFE_WAIT", safeWaitCount: 1 }),
    );
  });

  it("fails the incident timeline closed on ambiguous scheduled timestamps", () => {
    const timeline = projectStrategyFleetAutomaticCycleIncidents([
      {
        ...coinbaseEngine,
        scheduleRecentRuns: [
          ...coinbaseEngine.scheduleRecentRuns!,
          {
            id: "schedule-run-duplicate-time",
            scheduledFor: "2026-08-26T16:17:00Z",
            completedAt: "2026-08-26T16:17:20Z",
            nextRunAt: "2026-08-26T17:17:39Z",
            status: "FAILED",
            errorCode: "INTERNAL",
            duplicateRecovered: false,
            consecutiveFailures: 1,
          },
        ],
      },
    ]);

    expect(timeline).toEqual(
      expect.objectContaining({ status: "UNAVAILABLE", verifiedCount: 0 }),
    );
    expect(timeline.engines[0]).toEqual(
      expect.objectContaining({ state: "UNAVAILABLE", incidentCount: 0 }),
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

  it("automatically exposes reliability diagnostics that need review", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            scheduleStatus: "FAILED",
            scheduleErrorCode: "STRUCTURED_OUTPUT_MISSING",
            consecutiveFailures: 1,
            scheduleRecentRuns: [
              {
                ...coinbaseEngine.scheduleRecentRuns![0],
                status: "FAILED",
                errorCode: "STRUCTURED_OUTPUT_MISSING",
                consecutiveFailures: 1,
              },
            ],
          },
        ]}
      />,
    );

    const center = screen.getByRole("region", {
      name: "1 engine needs review while automatic recovery stays in control.",
    });
    expect(center).toHaveTextContent("0 stable · 0 recovered · 0 safe wait");
    expect(center).toHaveTextContent("1 attention · 0 unavailable");
    expect(center).toHaveTextContent("1 total · 0 recovered · 1 current");
    expect(
      within(center).getByText("Scheduler SLO History").closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(center).getByText("Failure Taxonomy").closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(center).getByText("Recovery Time Objective").closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(center).getByText("Incident Timeline").closest("details"),
    ).toHaveAttribute("open");
  });

  it("automatically exposes unavailable AI operations evidence", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            latestDecisionFinancialProvider: "schwab",
          },
        ]}
      />,
    );

    const workspace = screen.getByRole("region", {
      name: "Some current AI evidence is unavailable.",
    });
    expect(workspace).toHaveTextContent("1 visible · 3 unavailable");
    expect(
      within(workspace)
        .getByText("Decision route + conclusion")
        .closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(workspace)
        .getByText("Current AI input coverage")
        .closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(workspace).getByText("Input coverage changes").closest("details"),
    ).toHaveAttribute("open");
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
    const commandDeck = screen.getByRole("region", {
      name: "Every AI engine has one clear operating view.",
    });
    expect(commandDeck).toHaveTextContent("FLEET COMMAND DECK");
    expect(commandDeck).toHaveTextContent("1 active · 0 review");
    expect(commandDeck).toHaveTextContent("Coinbase Portfolio ••••a5d0");
    expect(commandDeck).toHaveTextContent("Newest immutable conclusion");
    expect(commandDeck).toHaveTextContent("Abstained");
    expect(commandDeck).toHaveTextContent("No action proposed · $0");
    expect(commandDeck).toHaveTextContent("OpenAI · gpt-5.6-sol · Deep");
    expect(commandDeck).toHaveTextContent(
      "$1,000 Shadow claim · $1 proposal ceiling",
    );
    expect(commandDeck).toHaveTextContent("Succeeded · 0 failures");
    expect(
      within(commandDeck)
        .getByText("Advanced engine evidence")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(commandDeck).getByRole("link", {
        name: /Open immutable evidence/i,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#runtime-evidence");
    expect(commandDeck).toHaveTextContent(
      "Paper and Shadow stay non-live · no model rerun",
    );
    const aiOperations = screen.getByRole("region", {
      name: "Your AI engines are operating with visible evidence limits.",
    });
    expect(aiOperations).toHaveTextContent("AI OPERATIONS");
    expect(aiOperations).toHaveTextContent("1 total · 0 Paper · 1 Shadow");
    expect(aiOperations).toHaveTextContent("1/1 current routes");
    expect(aiOperations).toHaveTextContent("3 available · 2 limited");
    expect(aiOperations).toHaveTextContent(
      "1 current · 0 safe wait · 0 review",
    );
    expect(aiOperations).toHaveTextContent("2 visible · 0 unavailable");
    expect(
      within(aiOperations)
        .getByText("Decision route + conclusion")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(aiOperations)
        .getByText("Current AI input coverage")
        .closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(aiOperations)
        .getByText("Input coverage changes")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(aiOperations)
        .getByText("Persistent input gaps")
        .closest("details"),
    ).toHaveAttribute("open");
    expect(
      within(aiOperations).getByText("Evidence freshness").closest("details"),
    ).not.toHaveAttribute("open");
    const provenanceDigest = screen.getByRole("region", {
      name: "1 current AI decision is fully attributable.",
      hidden: true,
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
        hidden: true,
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
      hidden: true,
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
        hidden: true,
      }),
    ).toHaveAttribute("href", "/activity");
    const inputGapRegister = screen.getByRole("region", {
      name: "1 active AI engine has a bounded input history.",
    });
    expect(inputGapRegister).toHaveTextContent("PERSISTENT AI INPUT GAPS");
    expect(inputGapRegister).toHaveTextContent("Attributable engines1 / 1");
    expect(inputGapRegister).toHaveTextContent("Persistent gaps2");
    expect(inputGapRegister).toHaveTextContent("3 immutable decision samples");
    expect(inputGapRegister).toHaveTextContent(
      "Position performancePersistent",
    );
    expect(inputGapRegister).toHaveTextContent("gap samples 3/3");
    expect(inputGapRegister).toHaveTextContent(
      "they never explain or grade an AI conclusion",
    );
    expect(
      within(inputGapRegister).getByRole("link", {
        name: /Review immutable window/i,
      }),
    ).toHaveAttribute("href", "/activity");
    const freshnessBoard = screen.getByRole("region", {
      name: "1 active AI engine is current or safely waiting.",
      hidden: true,
    });
    expect(freshnessBoard).toHaveTextContent("AI EVIDENCE FRESHNESS SLA");
    expect(freshnessBoard).toHaveTextContent("Current1");
    expect(freshnessBoard).toHaveTextContent("60-minute pinned cycle");
    expect(freshnessBoard).toHaveTextContent("65-minute evidence threshold");
    expect(freshnessBoard).toHaveTextContent(
      "Newest AI decisionCurrentAge 12m · stale after 65m",
    );
    expect(freshnessBoard).toHaveTextContent(
      "Next guarded cycleCurrentDue in 47m · 5m overdue grace",
    );
    expect(freshnessBoard).toHaveTextContent(
      "Future or missing evidence fails closed",
    );
    expect(
      within(freshnessBoard).getByRole("link", {
        name: /Open engine evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#runtime-evidence");
    expect(
      within(freshnessBoard).getByRole("link", {
        name: /Decision journal/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/activity");
    const reliabilityCenter = screen.getByRole("region", {
      name: "Every active AI engine is stable, recovered, or safely waiting.",
    });
    expect(reliabilityCenter).toHaveTextContent("FLEET RELIABILITY CENTER");
    expect(reliabilityCenter).toHaveTextContent(
      "1 stable · 0 recovered · 0 safe wait",
    );
    expect(reliabilityCenter).toHaveTextContent("0 attention · 0 unavailable");
    expect(reliabilityCenter).toHaveTextContent(
      "0 total · 0 recovered · 0 current",
    );
    expect(reliabilityCenter).toHaveTextContent("Advanced diagnostics");
    expect(
      within(reliabilityCenter)
        .getByText("Scheduler SLO History")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(reliabilityCenter)
        .getByText("Failure Taxonomy")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(reliabilityCenter)
        .getByText("Recovery Time Objective")
        .closest("details"),
    ).not.toHaveAttribute("open");
    expect(
      within(reliabilityCenter)
        .getByText("Incident Timeline")
        .closest("details"),
    ).not.toHaveAttribute("open");
    const sloHistory = screen.getByRole("region", {
      name: "1 active AI engine has verified cycle history.",
      hidden: true,
    });
    expect(sloHistory).toHaveTextContent("AUTOMATIC CYCLE SLO HISTORY");
    expect(sloHistory).toHaveTextContent("Saved samples1");
    expect(sloHistory).toHaveTextContent("Scheduler success100%");
    expect(sloHistory).toHaveTextContent("Five-minute SLO100%");
    expect(sloHistory).toHaveTextContent(
      "Completion latency39sLatest · average 39s · maximum 39s",
    );
    expect(sloHistory).toHaveTextContent(
      "no manual cycle · no model rerun · no provider refresh",
    );
    expect(
      within(sloHistory).getByRole("link", {
        name: /Scheduler evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    const failureTaxonomy = screen.getByRole("region", {
      name: "0 saved failures remain visible after recovery.",
      hidden: true,
    });
    expect(failureTaxonomy).toHaveTextContent(
      "AUTOMATIC CYCLE FAILURE TAXONOMY",
    );
    expect(failureTaxonomy).toHaveTextContent("Total failures0");
    expect(failureTaxonomy).toHaveTextContent("Session-safe waits0");
    expect(failureTaxonomy).toHaveTextContent(
      "No saved failure classification in this bounded window",
    );
    expect(failureTaxonomy).toHaveTextContent(
      "safe waits stay separate · no inferred provider output or causality",
    );
    expect(
      within(failureTaxonomy).getByRole("link", {
        name: /Scheduler evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    const recoveryRTO = screen.getByRole("region", {
      name: "No saved failure requires a recovery measurement.",
      hidden: true,
    });
    expect(recoveryRTO).toHaveTextContent("AUTOMATIC RECOVERY TIME OBJECTIVE");
    expect(recoveryRTO).toHaveTextContent("Recovered samples0");
    expect(recoveryRTO).toHaveTextContent("Current failures0");
    expect(recoveryRTO).toHaveTextContent("Median recoveryUnavailable");
    expect(recoveryRTO).toHaveTextContent(
      "No recovery classification in this bounded window",
    );
    expect(recoveryRTO).toHaveTextContent(
      "First later saved success only · safe-session waits stay separate",
    );
    expect(
      within(recoveryRTO).getByRole("link", {
        name: /Scheduler evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    const incidentTimeline = screen.getByRole("region", {
      name: "No saved automatic cycle incident is open.",
      hidden: true,
    });
    expect(incidentTimeline).toHaveTextContent(
      "AUTOMATIC CYCLE INCIDENT TIMELINE",
    );
    expect(incidentTimeline).toHaveTextContent("Incidents0");
    expect(incidentTimeline).toHaveTextContent("Current0");
    expect(incidentTimeline).toHaveTextContent(
      "No failed-cycle incident in this bounded window",
    );
    expect(incidentTimeline).toHaveTextContent(
      "first later SUCCEEDED cycle ends an incident",
    );
    expect(
      within(incidentTimeline).getByRole("link", {
        name: /Scheduler evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    expect(screen.getAllByText("Coinbase Portfolio ••••a5d0")).toHaveLength(1);
    expect(screen.getAllByText("gpt-5.6-sol")).toHaveLength(1);
    expect(screen.getAllByText("BTC · ETH · XRP +1")).toHaveLength(2);
    expect(screen.getByText("Healthy schedule")).toBeInTheDocument();
    const watchtower = screen.getByRole("region", {
      name: "1 guarded schedule is on course.",
      hidden: true,
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
      hidden: true,
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
      within(recovery).getByRole("link", {
        name: /Review evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#schedule-controls");
    const decisionTrails = screen.getByRole("region", {
      name: "1 latest AI decision has a complete evidence trail.",
      hidden: true,
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
        hidden: true,
      }),
    ).toHaveAttribute("href", "/activity");
    const runtimeContract = screen.getByRole("region", {
      name: "AI Shadow Engine immutable runtime contract",
      hidden: true,
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
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#configuration-controls");
    const dataHealth = screen.getByRole("region", {
      name: "AI Shadow Engine account data health",
      hidden: true,
    });
    expect(dataHealth).toHaveTextContent("Verified");
    expect(dataHealth).toHaveTextContent("Active account · Active connection");
    expect(dataHealth).toHaveTextContent("Balances ready · Positions ready");
    expect(dataHealth).toHaveTextContent("Fresh ≤24h");
    expect(dataHealth).toHaveTextContent("no provider read or order action");
    expect(
      within(dataHealth).getByRole("link", {
        name: /Account evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute(
      "href",
      "/accounts/coinbase-account#reconciliation-title",
    );
    const capitalAuthority = screen.getByRole("region", {
      name: "AI Shadow Engine capital authority",
      hidden: true,
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
      within(capitalAuthority).getByRole("link", {
        name: /Capital center/i,
        hidden: true,
      }),
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
      within(
        screen.getByRole("region", {
          name: "Every AI engine has one clear operating view.",
        }),
      ).getByRole("link", { name: /Open immutable evidence/i }),
    ).toHaveAttribute("href", "/automations/ai-mandate#runtime-evidence");
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

    const commandDeck = screen.getByRole("region", {
      name: "Every AI engine has one clear operating view.",
    });
    expect(commandDeck).toHaveTextContent("1 active · 0 review");
    expect(
      within(commandDeck).getByText("Healthy schedule"),
    ).toBeInTheDocument();
    expect(
      within(commandDeck).getByText("Succeeded · 0 failures"),
    ).toBeInTheDocument();
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

  it("shows exact Paper exposure, headroom, and immutable realized outcomes", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            title: "AI Paper Engine",
            executionMode: "PAPER",
            evidenceAvailable: undefined,
            evidenceStatus: undefined,
            paperPortfolioAvailable: true,
            paperPerformanceStatus: "AVAILABLE",
            paperCurrency: "USD",
            paperStartingCash: "1000.0000000000",
            paperCash: "850.0000000000",
            paperSimulatedEquity: "995.0000000000",
            paperInvestedExposure: "145.0000000000",
            paperTotalProfitLoss: "-5.0000000000",
            paperTotalReturnPercent: "-0.5",
            paperValuedAt: "2026-08-31T02:35:57Z",
            paperCashReserve: "200.0000000000",
            paperCashHeadroom: "650.0000000000",
            paperExposureCeiling: "800.0000000000",
            paperExposureHeadroom: "655.0000000000",
            paperSymbolCeiling: "300.0000000000",
            paperProposalHeadroom: "100.0000000000",
            paperRealizedContractAvailable: true,
            paperRealizedOutcomeStatus: "AVAILABLE",
            paperRealizedProfitLoss: "-2.0000000000",
            paperRealizedFillCount: 3,
            paperRealizedSellFillCount: 1,
            paperRealizedFirstFillAt: "2026-08-30T12:00:00Z",
            paperRealizedLastFillAt: "2026-08-30T14:00:00Z",
            paperRealizedSymbolOutcomes: [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                realizedProfitLoss: "-2.0000000000",
                buyFillCount: 2,
                sellFillCount: 1,
                totalFees: "2.8500000000",
                endingPositionQuantity: "1.5000000000",
                endingAverageCost: "111.1000000000",
              },
            ],
            paperExecutionCostsContractAvailable: true,
            paperExecutionCostsStatus: "AVAILABLE",
            paperExecutionTotalFees: "2.8500000000",
            paperExecutionTotalAdverseSlippage: "0.7500000000",
            paperExecutionTotalExplicitCost: "3.6000000000",
            paperExecutionProviderReferenceNotional: "300.0000000000",
            paperExecutionGrossNotional: "285.0000000000",
            paperExecutionAllInCostRateBPS: "120.0000000000",
            paperExecutionFillNotionalResidual: "0.0000000000",
            paperExecutionMaximumAbsoluteFillResidual: "0.0000000000",
            paperExecutionResidualBoundPerFill: "0.0000000001",
            paperExecutionFillCount: 3,
            paperExecutionBuyFillCount: 2,
            paperExecutionSellFillCount: 1,
            paperExecutionFirstFillAt: "2026-08-30T12:00:00Z",
            paperExecutionLastFillAt: "2026-08-30T14:00:00Z",
            paperExecutionMarketProviders: ["coinbase"],
            paperExecutionMarketFeeds: ["rest_ticker"],
            paperExecutionMarketQualities: ["REAL_TIME_SINGLE_VENUE"],
            paperExecutionTimelineSampleCount: 3,
            paperExecutionTimelineCapped: false,
            paperActivityCadenceContractAvailable: true,
            paperActivityCadence: {
              status: "AVAILABLE",
              calculation_method:
                "IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY",
              as_of: "2026-08-31T15:10:00Z",
              schedule_interval_minutes: 60,
              twenty_four_hours: {
                status: "AVAILABLE",
                horizon_hours: 24,
                window_started_at: "2026-08-30T15:10:00Z",
                window_ended_at: "2026-08-31T15:10:00Z",
                scheduled_cycle_count: 24,
                succeeded_cycle_count: 24,
                failed_cycle_count: 0,
                safe_wait_cycle_count: 0,
                abstention_count: 20,
                deterministic_deny_count: 1,
                simulated_fill_count: 3,
                other_succeeded_count: 0,
              },
              seven_days: {
                status: "UNAVAILABLE",
                horizon_hours: 168,
                scheduled_cycle_count: 48,
                succeeded_cycle_count: 48,
                failed_cycle_count: 0,
                safe_wait_cycle_count: 0,
                abstention_count: 44,
                deterministic_deny_count: 1,
                simulated_fill_count: 3,
                other_succeeded_count: 0,
              },
              disposition_funnel: {
                status: "AVAILABLE",
                calculation_method:
                  "IMMUTABLE_PAPER_EVALUATION_DISPOSITION_FUNNEL",
                twenty_four_hours: {
                  status: "AVAILABLE",
                  horizon_hours: 24,
                  window_started_at: "2026-08-30T15:10:00Z",
                  window_ended_at: "2026-08-31T15:10:00Z",
                  scheduled_cycle_count: 24,
                  completed_cycle_count: 24,
                  succeeded_evaluation_count: 24,
                  failed_cycle_count: 0,
                  safe_wait_cycle_count: 0,
                  decision_count: 24,
                  abstention_count: 20,
                  proposal_count: 4,
                  deterministic_deny_count: 1,
                  simulated_fill_count: 3,
                  other_proposal_outcome_count: 0,
                  completion_rate_percent: "100.0000000000",
                  succeeded_evaluation_rate_percent: "100.0000000000",
                  decision_rate_percent: "100.0000000000",
                  abstention_rate_percent: "83.3333333333",
                  proposal_rate_percent: "16.6666666667",
                  deterministic_deny_rate_percent: "25.0000000000",
                  simulated_fill_rate_percent: "75.0000000000",
                  other_proposal_outcome_rate_percent: "0.0000000000",
                },
                seven_days: {
                  status: "UNAVAILABLE",
                  horizon_hours: 168,
                  scheduled_cycle_count: 0,
                  completed_cycle_count: 0,
                  succeeded_evaluation_count: 0,
                  failed_cycle_count: 0,
                  safe_wait_cycle_count: 0,
                  decision_count: 0,
                  abstention_count: 0,
                  proposal_count: 0,
                  deterministic_deny_count: 0,
                  simulated_fill_count: 0,
                  other_proposal_outcome_count: 0,
                },
              },
              fill_timing: {
                status: "AVAILABLE",
                historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
                fill_count: 3,
                first_fill_at: "2026-08-30T13:00:00Z",
                last_fill_at: "2026-08-30T15:00:00Z",
                minimum_inter_fill_seconds: "3600.0000000000",
                median_inter_fill_seconds: "3600.0000000000",
                maximum_inter_fill_seconds: "3600.0000000000",
                symbols: [
                  {
                    status: "AVAILABLE",
                    symbol: "BTC",
                    instrument: "CRYPTO",
                    fill_count: 3,
                    first_fill_at: "2026-08-30T13:00:00Z",
                    last_fill_at: "2026-08-30T15:00:00Z",
                    minimum_inter_fill_seconds: "3600.0000000000",
                    median_inter_fill_seconds: "3600.0000000000",
                    maximum_inter_fill_seconds: "3600.0000000000",
                  },
                ],
              },
              longest_no_fill_interval: {
                status: "AVAILABLE",
                cycle_count: 6,
                interval_seconds: "18600.0000000000",
                scheduled_started_at: "2026-08-31T02:00:00Z",
                completed_ended_at: "2026-08-31T07:10:00Z",
              },
            },
            paperTradeSequence: {
              status: "AVAILABLE",
              calculation_method: "COMPLETE_IMMUTABLE_FILL_SEQUENCE",
              historical_coverage: "COMPLETE_FROM_PORTFOLIO_GENESIS",
              starting_cash: "1000.0000000000",
              provider_reference_turnover_to_starting_cash_bps:
                "3000.0000000000",
              explicit_cost_to_starting_cash_bps: "36.0000000000",
              fill_count: 3,
              same_side_transition_count: 1,
              opposite_side_transition_count: 1,
              buy_to_sell_reversal_count: 1,
              sell_to_buy_reversal_count: 0,
              symbols: [
                {
                  symbol: "BTC",
                  instrument: "CRYPTO",
                  fill_count: 3,
                  buy_fill_count: 2,
                  sell_fill_count: 1,
                  same_side_transition_count: 1,
                  opposite_side_transition_count: 1,
                  buy_to_sell_reversal_count: 1,
                  sell_to_buy_reversal_count: 0,
                  longest_same_side_streak: 2,
                  first_side: "BUY",
                  last_side: "SELL",
                  first_fill_at: "2026-08-30T13:00:00Z",
                  last_fill_at: "2026-08-30T15:00:00Z",
                },
              ],
            },
            paperExecutionTimeline: [
              {
                sequence: 1,
                fillID: "fill-first",
                symbol: "BTC",
                side: "BUY",
                explicitCost: "1.2500000000",
                fee: "1.0000000000",
                adverseSlippage: "0.2500000000",
                providerReferenceNotional: "100.0000000000",
                cumulativeExplicitCost: "1.2500000000",
                cumulativeProviderReferenceNotional: "100.0000000000",
                cumulativeAllInCostRateBPS: "125.0000000000",
                cumulativeRateChange: "FIRST",
                symbolSequence: 1,
                sameSideStreak: 1,
                sideTransition: "FIRST",
                marketProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                marketObservedAt: "2026-08-30T12:59:59Z",
                simulatedAt: "2026-08-30T13:00:00Z",
              },
              {
                sequence: 2,
                fillID: "fill-prior",
                symbol: "BTC",
                side: "BUY",
                explicitCost: "1.2500000000",
                fee: "1.0000000000",
                adverseSlippage: "0.2500000000",
                providerReferenceNotional: "100.0000000000",
                cumulativeExplicitCost: "2.5000000000",
                cumulativeProviderReferenceNotional: "200.0000000000",
                cumulativeAllInCostRateBPS: "125.0000000000",
                cumulativeRateChange: "HELD",
                symbolSequence: 2,
                sameSideStreak: 2,
                sideTransition: "SAME_SIDE",
                marketProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                marketObservedAt: "2026-08-30T13:59:59Z",
                simulatedAt: "2026-08-30T14:00:00Z",
              },
              {
                sequence: 3,
                fillID: "fill-current",
                symbol: "BTC",
                side: "SELL",
                explicitCost: "1.1000000000",
                fee: "0.8500000000",
                adverseSlippage: "0.2500000000",
                providerReferenceNotional: "100.0000000000",
                cumulativeExplicitCost: "3.6000000000",
                cumulativeProviderReferenceNotional: "300.0000000000",
                cumulativeAllInCostRateBPS: "120.0000000000",
                cumulativeRateChange: "FELL",
                symbolSequence: 3,
                sameSideStreak: 1,
                sideTransition: "BUY_TO_SELL",
                oppositeSideElapsedSeconds: "3600.0000000000",
                marketProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                marketObservedAt: "2026-08-30T14:59:59Z",
                simulatedAt: "2026-08-30T15:00:00Z",
              },
            ],
            paperExecutionSideCosts: [
              {
                side: "BUY",
                totalFees: "2.0000000000",
                adverseSlippage: "0.5000000000",
                totalExplicitCost: "2.5000000000",
                providerReferenceNotional: "200.0000000000",
                grossNotional: "190.0000000000",
                allInCostRateBPS: "125.0000000000",
                fillCount: 2,
              },
              {
                side: "SELL",
                totalFees: "0.8500000000",
                adverseSlippage: "0.2500000000",
                totalExplicitCost: "1.1000000000",
                providerReferenceNotional: "100.0000000000",
                grossNotional: "95.0000000000",
                allInCostRateBPS: "110.0000000000",
                fillCount: 1,
              },
            ],
            paperExecutionSymbolCosts: [
              {
                symbol: "BTC",
                instrument: "CRYPTO",
                totalFees: "2.8500000000",
                adverseSlippage: "0.7500000000",
                totalExplicitCost: "3.6000000000",
                providerReferenceNotional: "300.0000000000",
                grossNotional: "285.0000000000",
                allInCostRateBPS: "120.0000000000",
                fillCount: 3,
                buyFillCount: 2,
                sellFillCount: 1,
              },
            ],
            paperOutcomeReconciliationStatus: "RECONCILED_EXACT",
            paperReconciledRealizedProfitLoss: "-2.0000000000",
            paperReconciledUnrealizedProfitLoss: "-3.0000000000",
            paperReconciledClassifiedProfitLoss: "-5.0000000000",
            paperReconciledTotalProfitLoss: "-5.0000000000",
            paperOutcomeResidual: "0.0000000000",
            paperReconciledSimulatedEquity: "995.0000000000",
            paperReconciledCashPlusExposure: "995.0000000000",
            paperEquityResidual: "0.0000000000",
            paperOutcomeResidualLimit: "0.000001",
            paperOutcomeReconciliationProvider: "coinbase",
            paperOutcomeReconciliationFeeds: ["rest_ticker"],
            paperOutcomeReconciliationQualities: ["REAL_TIME_SINGLE_VENUE"],
            paperOutcomeReconciliationValuedAt: "2026-08-31T02:35:57Z",
            paperPositionOutcomes: [
              {
                symbol: "BTC",
                marketValue: "95.0000000000",
                unrealizedProfitLoss: "-4.0000000000",
                unrealizedProfitLossPercent: "-4.0404",
              },
              {
                symbol: "ETH",
                marketValue: "50.0000000000",
                unrealizedProfitLoss: "1.0000000000",
                unrealizedProfitLossPercent: "2",
              },
            ],
          },
        ]}
      />,
    );

    const exposureSummary = screen.getByText("Exposure + outcomes");
    expect(exposureSummary.closest("details")).not.toHaveAttribute("open");
    expect(screen.getByText("$850 cash · $145 marked")).toBeInTheDocument();
    expect(
      screen.getByText("$650 reserve headroom · $200 floor"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("$655 ceiling headroom · $800 limit"),
    ).toBeInTheDocument();
    expect(screen.getByText("Next proposal headroom")).toBeInTheDocument();
    expect(screen.getByText("−$5 · −0.5%")).toBeInTheDocument();
    expect(screen.getAllByText("BTC")).toHaveLength(5);
    expect(screen.getByText("$95")).toBeInTheDocument();
    expect(screen.getByText("Unrealized −$4 · −4.0404%")).toBeInTheDocument();
    expect(
      screen.getByText("Exact simulated realized outcome"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("−$2")).toHaveLength(2);
    expect(
      screen.getByText("1 simulated sale · 2 simulated buys"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /Exact average-cost replay from all 3 immutable simulated fills/i,
      ),
    ).toBeInTheDocument();
    const executionCosts = screen.getByRole("region", {
      name: /AI Paper Engine exact Paper execution costs/i,
      hidden: true,
    });
    expect(
      within(executionCosts).getAllByText(/\$3.6 all-in · 120 bps/i),
    ).toHaveLength(2);
    expect(
      within(executionCosts).getByText(
        /\$300 provider-reference turnover\. Explicit cost = \$2.85 fees \+ \$0.75 adverse slippage/i,
      ),
    ).toBeInTheDocument();
    expect(
      within(executionCosts).getByText(/not broker-reported costs/i),
    ).toBeInTheDocument();
    const costTimeline = within(executionCosts).getByLabelText(
      /AI Paper Engine immutable Paper cost and turnover timeline/i,
    );
    expect(
      within(costTimeline).getByText(/fell vs prior/i),
    ).toBeInTheDocument();
    expect(within(costTimeline).getByText(/3,600 sec/i)).toBeInTheDocument();
    expect(
      within(costTimeline).getByRole("link", {
        name: /Fill #3/i,
        hidden: true,
      }),
    ).toHaveAttribute(
      "href",
      "/automations/ai-mandate#paper-fill-fill-current",
    );
    const tradeSequence = within(executionCosts).getByLabelText(
      /AI Paper Engine exact immutable Paper trade sequence and churn evidence/i,
    );
    expect(within(tradeSequence).getByText(/3,000 bps/i)).toBeInTheDocument();
    expect(
      within(tradeSequence).getByText(/longest same-side streak 2/i),
    ).toBeInTheDocument();
    const cadence = screen.getByLabelText(
      /AI Paper Engine exact Paper activity cadence/i,
    );
    expect(
      within(cadence).getByText(/20 abstain · 4 propose/i),
    ).toBeInTheDocument();
    expect(
      within(cadence).getByText(/75% \/ 25% of proposals/i),
    ).toBeInTheDocument();
    expect(
      within(cadence).getByText(/does not establish conversion quality/i),
    ).toBeInTheDocument();
    const reconciliation = screen.getByRole("region", {
      name: /AI Paper Engine exact Paper outcome reconciliation/i,
      hidden: true,
    });
    expect(within(reconciliation).getByText("Exact match")).toBeInTheDocument();
    expect(
      within(reconciliation).getByText(/−\$2 \+ −\$3/i),
    ).toBeInTheDocument();
    expect(
      within(reconciliation).getByText(
        /immutable fill replay and saved market valuation reconcile/i,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", {
        name: /Open Paper evidence/i,
        hidden: true,
      }),
    ).toHaveAttribute("href", "/automations/ai-mandate#runtime-evidence");
  });

  it("fails closed and opens Paper outcome evidence when exact marks are unavailable", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            title: "AI Paper Engine",
            executionMode: "PAPER",
            evidenceAvailable: undefined,
            evidenceStatus: undefined,
            paperPortfolioAvailable: true,
            paperPerformanceStatus: "UNAVAILABLE",
            paperCurrency: "USD",
            paperCash: "850.0000000000",
          },
        ]}
      />,
    );

    const exposureSummary = screen.getByText("Exposure + outcomes");
    expect(exposureSummary.closest("details")).toHaveAttribute("open");
    expect(screen.getByText("Evidence unavailable")).toBeInTheDocument();
    expect(
      screen.getByText("Exact Paper outcome evidence is unavailable"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Missing values are never inferred/i),
    ).toBeInTheDocument();
    expect(screen.queryByText("$850 cash")).not.toBeInTheDocument();
  });

  it("automatically exposes a material Paper outcome reconciliation mismatch", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            title: "AI Paper Engine",
            executionMode: "PAPER",
            paperPortfolioAvailable: true,
            paperPerformanceStatus: "AVAILABLE",
            paperCurrency: "USD",
            paperStartingCash: "1000",
            paperCash: "850",
            paperSimulatedEquity: "995",
            paperInvestedExposure: "145",
            paperTotalProfitLoss: "-5",
            paperTotalReturnPercent: "-0.5",
            paperCashReserve: "200",
            paperCashHeadroom: "650",
            paperExposureCeiling: "800",
            paperExposureHeadroom: "655",
            paperSymbolCeiling: "300",
            paperProposalHeadroom: "100",
            paperPositionOutcomes: [],
            paperRealizedContractAvailable: true,
            paperRealizedOutcomeStatus: "AVAILABLE",
            paperRealizedProfitLoss: "-2",
            paperRealizedFillCount: 2,
            paperRealizedSellFillCount: 1,
            paperRealizedSymbolOutcomes: [],
            paperOutcomeReconciliationStatus: "MISMATCH",
            paperReconciledRealizedProfitLoss: "-2",
            paperReconciledUnrealizedProfitLoss: "-3",
            paperReconciledClassifiedProfitLoss: "-5",
            paperReconciledTotalProfitLoss: "-4",
            paperOutcomeResidual: "1",
            paperReconciledSimulatedEquity: "995",
            paperReconciledCashPlusExposure: "995",
            paperEquityResidual: "0",
            paperOutcomeResidualLimit: "0.000001",
            paperOutcomeReconciliationProvider: "coinbase",
            paperOutcomeReconciliationFeeds: ["rest_ticker"],
            paperOutcomeReconciliationQualities: ["REAL_TIME_SINGLE_VENUE"],
            paperOutcomeReconciliationValuedAt: "2026-08-31T02:35:57Z",
          },
        ]}
      />,
    );

    const exposureSummary = screen.getByText("Exposure + outcomes");
    expect(exposureSummary.closest("details")).toHaveAttribute("open");
    expect(screen.getByText("Outcome mismatch")).toBeInTheDocument();
    expect(
      screen.getByText(/differ beyond Arbion's strict decimal bound/i),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Review required").length).toBeGreaterThan(0);
  });

  it("compares bounded exact Paper outcome snapshots without claiming realized performance", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            title: "AI Paper Engine",
            executionMode: "PAPER",
            evidenceAvailable: undefined,
            evidenceStatus: undefined,
            paperPortfolioAvailable: true,
            paperPerformanceStatus: "AVAILABLE",
            paperCurrency: "USD",
            paperStartingCash: "1000",
            paperCash: "850",
            paperSimulatedEquity: "995",
            paperInvestedExposure: "145",
            paperTotalProfitLoss: "-5",
            paperTotalReturnPercent: "-0.5",
            paperValuedAt: "2026-08-31T03:35:57Z",
            paperCashReserve: "200",
            paperCashHeadroom: "650",
            paperExposureCeiling: "800",
            paperExposureHeadroom: "655",
            paperSymbolCeiling: "300",
            paperProposalHeadroom: "100",
            paperPositionOutcomes: [],
            outcomeHistoryAvailable: true,
            outcomeHistory: [
              {
                id: "paper-now",
                observedAt: "2026-08-31T03:35:58Z",
                marketObservedAt: "2026-08-31T03:35:57Z",
                financialProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                paperCash: "850",
                paperMarkedExposure: "145",
                paperSimulatedEquity: "995",
                paperUnrealizedProfitLoss: "-5",
                paperCashHeadroom: "650",
                paperExposureHeadroom: "655",
                paperFillDisposition: "RISK_DENIED",
                paperPositions: [
                  {
                    symbol: "BTC",
                    marketValue: "145",
                    unrealizedProfitLoss: "-5",
                  },
                ],
              },
              {
                id: "paper-prior",
                observedAt: "2026-08-31T02:35:58Z",
                marketObservedAt: "2026-08-31T02:35:57Z",
                financialProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                paperCash: "848",
                paperMarkedExposure: "140",
                paperSimulatedEquity: "988",
                paperUnrealizedProfitLoss: "-12",
                paperCashHeadroom: "648",
                paperExposureHeadroom: "660",
                paperFillDisposition: "SIMULATED_FILLED",
                paperPositions: [
                  {
                    symbol: "BTC",
                    marketValue: "140",
                    unrealizedProfitLoss: "-12",
                  },
                ],
              },
            ],
          },
        ]}
      />,
    );

    expect(screen.getByText("Outcome change timeline")).toBeInTheDocument();
    expect(screen.getByText("2 saved points")).toBeInTheDocument();
    expect(screen.getByText("$995")).toBeInTheDocument();
    expect(screen.getByText("−$5")).toBeInTheDocument();
    expect(
      screen.getByText("$145 marked · −$5 unrealized"),
    ).toBeInTheDocument();
    expect(screen.getByText("Reserve headroom $650")).toBeInTheDocument();
    expect(screen.getByText("Cycle result Risk Denied")).toBeInTheDocument();
    expect(screen.getAllByText("Improved")).toHaveLength(2);
    expect(screen.getAllByText(/coinbase · rest_ticker/i)).toHaveLength(2);
    expect(
      screen.getByText(/do not establish decision quality/i),
    ).toHaveTextContent("realized performance");
  });

  it("shows exact Shadow mark maturity and fails closed without two saved points", () => {
    const { rerender } = render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            outcomeHistoryAvailable: true,
            outcomeHistory: [
              {
                id: "shadow-now",
                observedAt: "2026-08-31T03:49:28Z",
                marketObservedAt: "2026-08-31T03:49:27Z",
                financialProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                shadowOneHourSamples: 13,
                shadowTwentyFourHourSamples: 5,
                shadowNewMarks: [
                  {
                    id: "mark-now",
                    horizon: "ONE_HOUR",
                    symbol: "XRP",
                    directionalChangePercent: "1.25",
                  },
                ],
              },
              {
                id: "shadow-prior",
                observedAt: "2026-08-31T02:49:28Z",
                marketObservedAt: "2026-08-31T02:49:27Z",
                financialProvider: "coinbase",
                marketFeed: "rest_ticker",
                marketQuality: "REAL_TIME_SINGLE_VENUE",
                shadowOneHourSamples: 12,
                shadowTwentyFourHourSamples: 5,
                shadowNewMarks: [
                  {
                    id: "mark-prior",
                    horizon: "TWENTY_FOUR_HOURS",
                    symbol: "BTC",
                    directionalChangePercent: "-0.75",
                  },
                ],
              },
            ],
          },
        ]}
      />,
    );

    expect(screen.getByText("13 one-hour · 5 24-hour")).toBeInTheDocument();
    expect(screen.getByText("XRP One Hour +1.25%")).toBeInTheDocument();
    expect(screen.getByText("Improved")).toBeInTheDocument();

    rerender(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            outcomeHistoryAvailable: false,
            outcomeHistory: [],
          },
        ]}
      />,
    );
    expect(
      screen.getByText(/needs at least two complete, attributable immutable/i),
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

    const fleet = screen.getByRole("region", {
      name: "1 AI engine has a clear review signal.",
    });
    expect(fleet).toHaveTextContent("1 active · 1 review");
    expect(
      within(fleet).getByText("Schedule evidence unavailable"),
    ).toBeInTheDocument();
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
    ).toHaveLength(1);

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
    ).toHaveLength(1);
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

  it("surfaces exact Paper guardrail attribution in the command deck", () => {
    render(
      <StrategyFleet
        items={[
          {
            ...coinbaseEngine,
            id: "paper-mandate",
            title: "Coinbase AI Paper",
            executionMode: "PAPER",
            paperPortfolioAvailable: true,
            paperPerformanceStatus: "AVAILABLE",
            paperCurrency: "USD",
            paperStartingCash: "1000.0000000000",
            paperCash: "900.0000000000",
            paperSimulatedEquity: "1000.0000000000",
            paperInvestedExposure: "100.0000000000",
            paperTotalProfitLoss: "0.0000000000",
            paperTotalReturnPercent: "0.0000000000",
            paperCashReserve: "200.0000000000",
            paperCashHeadroom: "700.0000000000",
            paperExposureCeiling: "800.0000000000",
            paperExposureHeadroom: "700.0000000000",
            paperSymbolCeiling: "300.0000000000",
            paperProposalHeadroom: "100.0000000000",
            paperPositionOutcomes: [],
            paperGuardrailEvidenceContractAvailable: true,
            paperGuardrailEvidence: {
              status: "AVAILABLE",
              calculation_method:
                "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION",
              as_of: "2026-08-31T15:00:00Z",
              twenty_four_hours: {
                status: "AVAILABLE",
                horizon_hours: 24,
                window_started_at: "2026-08-30T15:00:00Z",
                window_ended_at: "2026-08-31T15:00:00Z",
                proposal_count: 1,
                allow_count: 0,
                deny_count: 1,
                simulated_fill_count: 0,
                minimum_proposed_notional: "50.0000000000",
                median_proposed_notional: "50.0000000000",
                maximum_proposed_notional: "50.0000000000",
                denial_reason_codes: [
                  { code: "INSUFFICIENT_POSITION", count: 1 },
                ],
                failed_check_codes: [
                  { code: "INSUFFICIENT_POSITION", count: 1 },
                ],
                symbols: [
                  {
                    symbol: "ETH",
                    instrument: "CRYPTO",
                    proposal_count: 1,
                    allow_count: 0,
                    deny_count: 1,
                    simulated_fill_count: 0,
                    proposed_notional: "50.0000000000",
                  },
                ],
                proposals: [
                  {
                    decision_journal_entry_id: "decision-denied",
                    proposed_action_id: "action-denied",
                    risk_evaluation_id: "risk-denied",
                    execution_record_id: "execution-denied",
                    created_at: "2026-08-31T14:00:00Z",
                    symbol: "ETH",
                    instrument: "CRYPTO",
                    side: "SELL",
                    proposed_notional: "50.0000000000",
                    risk_decision: "DENY",
                    execution_status: "RISK_DENIED",
                    denial_reason_codes: ["INSUFFICIENT_POSITION"],
                    failed_check_codes: ["INSUFFICIENT_POSITION"],
                    financial_provider: "coinbase",
                    market_feed: "rest_ticker",
                    market_quality: "REAL_TIME_SINGLE_VENUE",
                    market_observed_at: "2026-08-31T13:59:59Z",
                  },
                ],
              },
              seven_days: {
                status: "UNAVAILABLE",
                horizon_hours: 168,
                proposal_count: 0,
                allow_count: 0,
                deny_count: 0,
                simulated_fill_count: 0,
                denial_reason_codes: [],
                failed_check_codes: [],
                symbols: [],
                proposals: [],
              },
            },
          },
        ]}
      />,
    );

    const guardrails = screen.getByLabelText(
      /coinbase ai paper exact paper guardrail disposition/i,
    );
    expect(within(guardrails).getAllByText(/1 denied/i).length).toBeGreaterThan(
      0,
    );
    expect(
      within(guardrails).getByText(/insufficient position \(1\)/i),
    ).toBeInTheDocument();
    expect(
      within(guardrails).getByRole("link", {
        name: /review immutable evidence/i,
      }),
    ).toHaveAttribute("href", "/automations/paper-mandate#decision-journal");
  });
});
