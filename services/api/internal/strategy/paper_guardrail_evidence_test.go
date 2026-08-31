package strategy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func guardrailJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func guardrailRationale(symbol, side, notional string, observedAt time.Time) json.RawMessage {
	return guardrailJSON(map[string]any{
		"decision": "PROPOSE", "symbol": symbol, "side": side, "proposed_notional": notional,
		"input_evidence": map[string]any{"provider": "coinbase", "markets": []map[string]any{{
			"symbol": symbol, "feed": "rest_ticker", "quality": "REAL_TIME_SINGLE_VENUE", "observed_at": observedAt,
		}}},
	})
}

func guardrailCadence(start, end time.Time) PaperActivityCadence {
	return PaperActivityCadence{AsOf: &end, DispositionFunnel: PaperDispositionFunnel{
		Status:          PaperActivityCadenceAvailable,
		TwentyFourHours: PaperDispositionFunnelWindow{Status: PaperActivityCadenceAvailable, HorizonHours: 24, WindowStartedAt: &start, WindowEndedAt: &end, ProposalCount: 2, DeterministicDenyCount: 1, SimulatedFillCount: 1},
		SevenDays:       PaperDispositionFunnelWindow{Status: PaperActivityCadenceUnavailable, HorizonHours: 168},
	}}
}

func guardrailPassChecks() json.RawMessage {
	checks := []map[string]string{}
	for _, stage := range risk.AIAutonomousPaperCheckPlan() {
		checks = append(checks, map[string]string{"code": string(stage.CanonicalCode), "result": "PASS", "message": "The deterministic stage passed."})
	}
	return guardrailJSON(checks)
}

func guardrailDenyChecks(stageIndex int, failureCode string) json.RawMessage {
	checks := []map[string]string{}
	for index, stage := range risk.AIAutonomousPaperCheckPlan() {
		if index > stageIndex {
			break
		}
		code, result := string(stage.CanonicalCode), "PASS"
		if index == stageIndex {
			code, result = failureCode, "FAIL"
		}
		checks = append(checks, map[string]string{"code": code, "result": result, "message": "The deterministic stage was saved."})
	}
	return guardrailJSON(checks)
}

func TestProjectPaperGuardrailEvidenceAttributesExactProposalDispositions(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	allowAt, denyAt := end.Add(-time.Hour), end.Add(-2*time.Hour)
	fill := exactExecutionFill("allow", "BTC", "BUY", "0.001", "100", "100.25", "100.25", "0.50125", allowAt)
	rows := []paperGuardrailProposalRow{
		{
			DecisionJournalEntryID: "decision-deny", CreatedAt: denyAt, DecisionType: "DENY_RISK_DENIED", ProposedActionID: "action-deny", RiskEvaluationID: "risk-deny", ExecutionRecordID: "execution-deny",
			Rationale: guardrailRationale("ETH", "SELL", "50", denyAt.Add(-time.Second)), RiskDecision: "DENY", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"INSUFFICIENT_POSITION"}),
			Checks:        guardrailDenyChecks(9, "INSUFFICIENT_POSITION"),
			ExecutionMode: "PAPER", ExecutionStatus: "RISK_DENIED", Symbol: "ETH", Instrument: "CRYPTO", Side: "SELL", ExecutionNotional: "50.0000000000",
		},
		{
			DecisionJournalEntryID: "decision-allow", CreatedAt: allowAt, DecisionType: "ALLOW_SIMULATED_FILLED", ProposedActionID: fill.ProposedActionID, RiskEvaluationID: fill.RiskEvaluationID, ExecutionRecordID: fill.ExecutionRecordID,
			Rationale: guardrailRationale("BTC", "BUY", "100", fill.MarketObservedAt), RiskDecision: "ALLOW", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"ALLOWED"}),
			Checks:        guardrailPassChecks(),
			ExecutionMode: "PAPER", ExecutionStatus: "SIMULATED_FILLED", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", ExecutionNotional: "100.2500000000",
		},
	}
	result := projectPaperGuardrailEvidence(guardrailCadence(start, end), rows, []paperRealizedFill{fill})
	if result.Status != PaperActivityCadenceAvailable || result.CalculationMethod != PaperGuardrailEvidenceMethod || result.AsOf == nil || result.SevenDays.Status != PaperActivityCadenceUnavailable {
		t.Fatalf("guardrail contract unavailable: %#v", result)
	}
	window := result.TwentyFourHours
	if window.Status != PaperActivityCadenceAvailable || window.ProposalCount != 2 || window.AllowCount != 1 || window.DenyCount != 1 || window.SimulatedFillCount != 1 ||
		window.CoverageStatus != paperGuardrailCoverageComplete || window.FullyEvaluatedCount != 1 || window.FailClosedPrefixCount != 1 || window.CheckSetDriftCount != 0 || len(window.ExpectedCheckCodes) != 14 ||
		window.MinimumProposedNotional != "50.0000000000" || window.MedianProposedNotional != "75.0000000000" || window.MaximumProposedNotional != "100.0000000000" ||
		len(window.DenialReasonCodes) != 1 || window.DenialReasonCodes[0].Code != "INSUFFICIENT_POSITION" || window.DenialReasonCodes[0].Count != 1 ||
		len(window.FailedCheckCodes) != 1 || window.FailedCheckCodes[0].Code != "INSUFFICIENT_POSITION" || len(window.Symbols) != 2 || len(window.Proposals) != 2 {
		t.Fatalf("exact guardrail attribution changed: %#v", window)
	}
	if window.Symbols[0].Symbol != "BTC" || window.Symbols[0].AllowCount != 1 || window.Symbols[0].ProposedNotional != "100.0000000000" ||
		window.Symbols[1].Symbol != "ETH" || window.Symbols[1].DenyCount != 1 || window.Proposals[0].DecisionJournalEntryID != "decision-deny" ||
		window.Proposals[0].CoverageStatus != paperGuardrailCoverageFailClosedPrefix || window.Proposals[0].TerminalCheckStage != "INSUFFICIENT_POSITION" ||
		window.Proposals[1].CoverageStatus != paperGuardrailCoverageFullEvaluation || len(window.Proposals[1].Checks) != 14 || window.Proposals[1].FinancialProvider != "coinbase" {
		t.Fatalf("symbol or immutable evidence attribution changed: %#v", window)
	}
}

func TestProjectPaperGuardrailEvidenceComparesCompleteCoverageWindows(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	currentStart, baselineStart := end.Add(-24*time.Hour), end.Add(-168*time.Hour)
	allowAt, denyAt, olderAllowAt := end.Add(-time.Hour), end.Add(-2*time.Hour), end.Add(-48*time.Hour)
	currentFill := exactExecutionFill("allow-current", "BTC", "BUY", "0.001", "100", "100.25", "100.25", "0.50125", allowAt)
	olderFill := exactExecutionFill("allow-older", "BTC", "BUY", "0.001", "100", "100.25", "100.25", "0.50125", olderAllowAt)
	rows := []paperGuardrailProposalRow{
		{
			DecisionJournalEntryID: "decision-allow-older", CreatedAt: olderAllowAt, DecisionType: "ALLOW_SIMULATED_FILLED", ProposedActionID: olderFill.ProposedActionID, RiskEvaluationID: olderFill.RiskEvaluationID, ExecutionRecordID: olderFill.ExecutionRecordID,
			Rationale: guardrailRationale("BTC", "BUY", "100", olderFill.MarketObservedAt), RiskDecision: "ALLOW", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"ALLOWED"}), Checks: guardrailPassChecks(),
			ExecutionMode: "PAPER", ExecutionStatus: "SIMULATED_FILLED", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", ExecutionNotional: "100.2500000000",
		},
		{
			DecisionJournalEntryID: "decision-deny-current", CreatedAt: denyAt, DecisionType: "DENY_RISK_DENIED", ProposedActionID: "action-deny-current", RiskEvaluationID: "risk-deny-current", ExecutionRecordID: "execution-deny-current",
			Rationale: guardrailRationale("ETH", "SELL", "50", denyAt.Add(-time.Second)), RiskDecision: "DENY", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"INSUFFICIENT_POSITION"}), Checks: guardrailDenyChecks(9, "INSUFFICIENT_POSITION"),
			ExecutionMode: "PAPER", ExecutionStatus: "RISK_DENIED", Symbol: "ETH", Instrument: "CRYPTO", Side: "SELL", ExecutionNotional: "50.0000000000",
		},
		{
			DecisionJournalEntryID: "decision-allow-current", CreatedAt: allowAt, DecisionType: "ALLOW_SIMULATED_FILLED", ProposedActionID: currentFill.ProposedActionID, RiskEvaluationID: currentFill.RiskEvaluationID, ExecutionRecordID: currentFill.ExecutionRecordID,
			Rationale: guardrailRationale("BTC", "BUY", "100", currentFill.MarketObservedAt), RiskDecision: "ALLOW", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"ALLOWED"}), Checks: guardrailPassChecks(),
			ExecutionMode: "PAPER", ExecutionStatus: "SIMULATED_FILLED", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", ExecutionNotional: "100.2500000000",
		},
	}
	cadence := guardrailCadence(currentStart, end)
	cadence.DispositionFunnel.SevenDays = PaperDispositionFunnelWindow{
		Status: PaperActivityCadenceAvailable, HorizonHours: 168, WindowStartedAt: &baselineStart, WindowEndedAt: &end,
		ProposalCount: 3, DeterministicDenyCount: 1, SimulatedFillCount: 2,
	}
	result := projectPaperGuardrailEvidence(cadence, rows, []paperRealizedFill{olderFill, currentFill})
	change := result.CoverageChange
	if change.Status != PaperActivityCadenceAvailable || change.CalculationMethod != PaperGuardrailCoverageChangeMethod ||
		change.BaselineProposalCount != 3 || change.CurrentProposalCount != 2 || change.ProposalCountDelta != -1 ||
		len(change.FinancialProviders) != 1 || change.FinancialProviders[0] != "coinbase" ||
		change.FirstEvidenceAt == nil || !change.FirstEvidenceAt.Equal(olderAllowAt) || change.LatestEvidenceAt == nil || !change.LatestEvidenceAt.Equal(allowAt) ||
		change.FirstCheckSetDriftAt != nil || change.LatestCheckSetDriftAt != nil || len(change.CoverageMetrics) != 3 || len(change.CheckChanges) != 14 || len(change.SymbolChanges) != 2 {
		t.Fatalf("coverage window comparison changed: %#v", change)
	}
	if change.CoverageMetrics[0].Metric != "FULL_EVALUATION" || change.CoverageMetrics[0].BaselineCount != 2 || change.CoverageMetrics[0].CurrentCount != 1 || change.CoverageMetrics[0].ShareChange != paperGuardrailShareDecreased ||
		change.CoverageMetrics[1].Metric != "FAIL_CLOSED_PREFIX" || change.CoverageMetrics[1].ShareChange != paperGuardrailShareIncreased ||
		change.CoverageMetrics[2].Metric != "CHECK_SET_DRIFT" || change.CoverageMetrics[2].ShareChange != paperGuardrailShareUnchanged {
		t.Fatalf("coverage metric changes are not exact: %#v", change.CoverageMetrics)
	}
	if change.CheckChanges[0].BaselineEvaluationCount != 3 || change.CheckChanges[0].CurrentEvaluationCount != 2 || change.CheckChanges[0].EvaluationShareChange != paperGuardrailShareUnchanged ||
		change.SymbolChanges[0].Symbol != "BTC" || change.SymbolChanges[0].ProposedNotionalDelta != "-100.0000000000" || change.SymbolChanges[0].ProposalShareChange != paperGuardrailShareDecreased {
		t.Fatalf("check or symbol comparison changed: checks=%#v symbols=%#v", change.CheckChanges, change.SymbolChanges)
	}
}

func TestPaperGuardrailCoverageChangeFailsClosedUntilSevenDayWindowCompletes(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	result := projectPaperGuardrailEvidence(guardrailCadence(start, end), nil, nil)
	if result.CoverageChange.Status != PaperActivityCadenceUnavailable || len(result.CoverageChange.CoverageMetrics) != 0 || len(result.CoverageChange.CheckChanges) != 0 || len(result.CoverageChange.SymbolChanges) != 0 {
		t.Fatalf("incomplete seven-day comparison was inferred: %#v", result.CoverageChange)
	}
}

func TestProjectPaperGuardrailEvidenceExposesExactCheckSetDrift(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	row := paperGuardrailProposalRow{
		DecisionJournalEntryID: "decision", CreatedAt: end.Add(-time.Hour), DecisionType: "DENY_RISK_DENIED", ProposedActionID: "action", RiskEvaluationID: "risk", ExecutionRecordID: "execution",
		Rationale: guardrailRationale("BTC", "BUY", "50", end.Add(-time.Hour-time.Second)), RiskDecision: "DENY", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"CAPITAL_LIMIT_EXCEEDED"}),
		Checks: guardrailJSON([]map[string]string{
			{"code": "AUTHORIZATION_DENIED", "result": "PASS", "message": "Identity is valid."},
			{"code": "CAPITAL_LIMIT_EXCEEDED", "result": "FAIL", "message": "The saved set skipped required stages."},
		}),
		ExecutionMode: "PAPER", ExecutionStatus: "RISK_DENIED", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", ExecutionNotional: "50",
	}
	cadence := guardrailCadence(start, end)
	cadence.DispositionFunnel.TwentyFourHours.ProposalCount = 1
	cadence.DispositionFunnel.TwentyFourHours.SimulatedFillCount = 0
	result := projectPaperGuardrailEvidence(cadence, []paperGuardrailProposalRow{row}, nil)
	window := result.TwentyFourHours
	if result.Status != PaperActivityCadenceAvailable || window.Status != PaperActivityCadenceAvailable || window.CoverageStatus != paperGuardrailCoverageDriftDetected ||
		window.CheckSetDriftCount != 1 || window.FullyEvaluatedCount != 0 || window.FailClosedPrefixCount != 0 || len(window.Proposals) != 1 ||
		window.Proposals[0].CoverageStatus != paperGuardrailCoverageDriftDetected || len(window.CheckResults) != 2 {
		t.Fatalf("exact saved check-set drift was not exposed: %#v", result)
	}
}

func TestProjectPaperGuardrailEvidenceFailsClosedOnInconsistentRiskEvidence(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	row := paperGuardrailProposalRow{
		DecisionJournalEntryID: "decision", CreatedAt: end.Add(-time.Hour), DecisionType: "DENY_RISK_DENIED", ProposedActionID: "action", RiskEvaluationID: "risk", ExecutionRecordID: "execution",
		Rationale: guardrailRationale("BTC", "SELL", "50", end.Add(-time.Hour-time.Second)), RiskDecision: "DENY", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"INSUFFICIENT_POSITION"}),
		Checks:        guardrailJSON([]map[string]string{{"code": "INSUFFICIENT_POSITION", "result": "PASS", "message": "Inconsistent saved check."}}),
		ExecutionMode: "PAPER", ExecutionStatus: "RISK_DENIED", Symbol: "BTC", Instrument: "CRYPTO", Side: "SELL", ExecutionNotional: "50",
	}
	cadence := guardrailCadence(start, end)
	cadence.DispositionFunnel.TwentyFourHours.ProposalCount = 1
	cadence.DispositionFunnel.TwentyFourHours.SimulatedFillCount = 0
	result := projectPaperGuardrailEvidence(cadence, []paperGuardrailProposalRow{row}, nil)
	if result.Status != PaperActivityCadenceUnavailable || result.TwentyFourHours.Status != PaperActivityCadenceUnavailable || len(result.TwentyFourHours.Proposals) != 0 {
		t.Fatalf("inconsistent risk evidence was inferred: %#v", result)
	}
}
