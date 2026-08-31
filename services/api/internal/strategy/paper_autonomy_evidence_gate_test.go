package strategy

import (
	"encoding/json"
	"testing"
	"time"
)

func paperEvidenceFixture(asOf time.Time, count int, startedAt time.Time) ([]ScheduleRun, []paperAutonomyDecisionRow) {
	runs := make([]ScheduleRun, 0, count)
	decisions := make([]paperAutonomyDecisionRow, 0, count)
	for index := 0; index < count; index++ {
		createdAt := asOf.Add(-time.Duration(count-1-index) * time.Hour)
		decision := "ABSTAIN"
		executionStatus := "CANCELED"
		rationale, _ := json.Marshal(map[string]any{
			"decision": decision, "ai_provider": "openai", "model_id": "gpt-5.6-sol", "profile": "deep",
			"execution_mode": "PAPER", "latency_ms": 10, "input_usage": 20, "output_usage": 5,
			"input_evidence": map[string]any{
				"provider": "coinbase", "recent_decisions": []any{},
				"markets": []map[string]any{{"symbol": "BTC", "feed": "rest_ticker", "quality": "REAL_TIME_SINGLE_VENUE", "observed_at": createdAt.Add(-time.Second)}},
			},
		})
		runs = append(runs, ScheduleRun{ID: "run-" + createdAt.Format(time.RFC3339), StrategyInstanceID: "instance", ExecutionMode: Paper,
			ScheduledFor: createdAt.Add(-time.Minute), StartedAt: createdAt.Add(-30 * time.Second), CompletedAt: createdAt,
			NextRunAt: createdAt.Add(time.Hour), Status: "SUCCEEDED", AIDecision: &decision, ExecutionStatus: &executionStatus})
		decisions = append(decisions, paperAutonomyDecisionRow{ID: "decision-" + createdAt.Format(time.RFC3339), DecisionType: "ABSTAIN", Rationale: rationale, CreatedAt: createdAt})
	}
	return runs, decisions
}

func paperEvidencePortfolio() PaperPortfolio {
	return PaperPortfolio{
		RealizedOutcome:   PaperRealizedOutcome{Status: PaperRealizedNoSales, FillCount: 0},
		ExecutionCosts:    PaperExecutionCosts{Status: PaperExecutionCostsNoFills, FillCount: 0},
		ActivityCadence:   PaperActivityCadence{Status: PaperActivityCadenceAvailable},
		GuardrailEvidence: PaperGuardrailEvidence{Status: PaperActivityCadenceAvailable},
	}
}

func TestPaperAutonomyEvidenceGateBecomesReviewableWithoutTrades(t *testing.T) {
	asOf := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-169 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 20, startedAt)
	gate := projectPaperAutonomyEvidenceGate(startedAt, "SUCCEEDED", 0, runs, decisions, paperEvidencePortfolio(), paperAutonomySafetyCounts{})
	if gate.Status != PaperAutonomyEvidenceReviewable || gate.EvidenceWindowHours != 168 || gate.DecisionCount != 20 || gate.AbstentionCount != 20 || gate.ProposalCount != 0 || len(gate.Blockers) != 0 {
		t.Fatalf("abstention-only evidence should be reviewable without trading: %#v", gate)
	}
	if gate.AttributedDecisionCount != 20 || gate.TelemetryCompleteCount != 20 || gate.BoundedMemoryCount != 20 || len(gate.Routes) != 1 || gate.Safety.Status != "CLEAR" || !gate.LedgerContractsReconciled || gate.LiveExecutionAvailable {
		t.Fatalf("reviewable gate lost exact provenance or safety evidence: %#v", gate)
	}
	packet := gate.ReviewPacket
	if packet.Status != PaperAutonomyEvidenceReviewable || !packet.EvidenceReadyForHumanReview || packet.ElapsedSeconds != 169*60*60 || packet.RemainingSeconds != 0 || packet.SchedulerSampleCount != 20 || packet.SchedulerSuccessCount != 20 || packet.SchedulerFailureCount != 0 || packet.RouteContinuityStatus != "STABLE" || packet.InputCoverageStatus != "COMPLETE" || packet.InputFreshnessStatus != "CURRENT_AT_DECISION" || packet.MarketObservationCount != 20 || packet.FreshMarketDecisionCount != 20 || packet.MaximumMarketAgeSeconds != 1 || packet.LedgerContractStatus != "RECONCILED" || packet.NoLiveSafetyStatus != "CLEAR" || packet.GrantsAuthority || packet.LivePromotionAvailable {
		t.Fatalf("review packet lost exact owner-review evidence: %#v", packet)
	}
}

func TestPaperAutonomyEvidenceGateCollectsBeforeThresholds(t *testing.T) {
	asOf := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-48 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 12, startedAt)
	gate := projectPaperAutonomyEvidenceGate(startedAt, "SUCCEEDED", 0, runs, decisions, paperEvidencePortfolio(), paperAutonomySafetyCounts{})
	if gate.Status != PaperAutonomyEvidenceCollecting || gate.EvidenceWindowHours != 48 || gate.DecisionCount != 12 || len(gate.Blockers) != 2 {
		t.Fatalf("incomplete normal evidence should remain collecting: %#v", gate)
	}
	for _, blocker := range gate.Blockers {
		if blocker.Category != "COLLECTION" {
			t.Fatalf("normal threshold blocker was misclassified: %#v", gate.Blockers)
		}
	}
	if gate.ReviewPacket.Status != PaperAutonomyEvidenceCollecting || gate.ReviewPacket.ElapsedSeconds != 48*60*60 || gate.ReviewPacket.RemainingSeconds != 120*60*60 || gate.ReviewPacket.EvidenceReadyForHumanReview {
		t.Fatalf("collecting packet must expose the exact remaining window without granting review authority: %#v", gate.ReviewPacket)
	}
}

func TestPaperAutonomyEvidenceGateRequiresReviewForCurrentSafetyFailure(t *testing.T) {
	asOf := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-169 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 20, startedAt)
	gate := projectPaperAutonomyEvidenceGate(startedAt, "FAILED", 1, runs, decisions, paperEvidencePortfolio(), paperAutonomySafetyCounts{LiveMandates: 1})
	if gate.Status != PaperAutonomyEvidenceReviewRequired || gate.Safety.Status != "REVIEW_REQUIRED" || len(gate.Blockers) != 2 {
		t.Fatalf("current scheduler and no-live failures must require review: %#v", gate)
	}
}

func TestPaperAutonomyEvidenceGateFailsClosedOnDecisionScheduleMismatch(t *testing.T) {
	asOf := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-169 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 20, startedAt)
	gate := projectPaperAutonomyEvidenceGate(startedAt, "SUCCEEDED", 0, runs, decisions[:19], paperEvidencePortfolio(), paperAutonomySafetyCounts{})
	if gate.Status != PaperAutonomyEvidenceUnavailable || len(gate.Blockers) == 0 || gate.Blockers[0].Code != "DECISION_EVIDENCE_INCONSISTENT" {
		t.Fatalf("mismatched immutable evidence must fail closed: %#v", gate)
	}
}

func TestPaperAutonomyEvidenceGateRequiresReviewForStaleSavedMarketInput(t *testing.T) {
	asOf := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-169 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 20, startedAt)
	var rationale map[string]any
	if err := json.Unmarshal(decisions[0].Rationale, &rationale); err != nil {
		t.Fatal(err)
	}
	inputEvidence := rationale["input_evidence"].(map[string]any)
	markets := inputEvidence["markets"].([]any)
	market := markets[0].(map[string]any)
	market["observed_at"] = decisions[0].CreatedAt.Add(-10 * time.Minute)
	decisions[0].Rationale, _ = json.Marshal(rationale)

	gate := projectPaperAutonomyEvidenceGate(startedAt, "SUCCEEDED", 0, runs, decisions, paperEvidencePortfolio(), paperAutonomySafetyCounts{})
	if gate.Status != PaperAutonomyEvidenceReviewRequired || gate.ReviewPacket.InputFreshnessStatus != "REVIEW_REQUIRED" || gate.ReviewPacket.FreshMarketDecisionCount != 19 {
		t.Fatalf("stale saved market input must remain exact and require review: %#v", gate)
	}
	found := false
	for _, blocker := range gate.Blockers {
		found = found || blocker.Code == "MARKET_INPUT_FRESHNESS_REVIEW_REQUIRED"
	}
	if !found {
		t.Fatalf("stale saved market input blocker missing: %#v", gate.Blockers)
	}
}
