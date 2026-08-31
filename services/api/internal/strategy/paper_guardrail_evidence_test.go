package strategy

import (
	"encoding/json"
	"testing"
	"time"
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

func TestProjectPaperGuardrailEvidenceAttributesExactProposalDispositions(t *testing.T) {
	end := time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	allowAt, denyAt := end.Add(-time.Hour), end.Add(-2*time.Hour)
	fill := exactExecutionFill("allow", "BTC", "BUY", "0.001", "100", "100.25", "100.25", "0.50125", allowAt)
	rows := []paperGuardrailProposalRow{
		{
			DecisionJournalEntryID: "decision-deny", CreatedAt: denyAt, DecisionType: "DENY_RISK_DENIED", ProposedActionID: "action-deny", RiskEvaluationID: "risk-deny", ExecutionRecordID: "execution-deny",
			Rationale: guardrailRationale("ETH", "SELL", "50", denyAt.Add(-time.Second)), RiskDecision: "DENY", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"INSUFFICIENT_POSITION"}),
			Checks:        guardrailJSON([]map[string]string{{"code": "AUTHORIZATION_DENIED", "result": "PASS", "message": "Identity is valid."}, {"code": "INSUFFICIENT_POSITION", "result": "FAIL", "message": "The sale exceeds the saved holding."}}),
			ExecutionMode: "PAPER", ExecutionStatus: "RISK_DENIED", Symbol: "ETH", Instrument: "CRYPTO", Side: "SELL", ExecutionNotional: "50.0000000000",
		},
		{
			DecisionJournalEntryID: "decision-allow", CreatedAt: allowAt, DecisionType: "ALLOW_SIMULATED_FILLED", ProposedActionID: fill.ProposedActionID, RiskEvaluationID: fill.RiskEvaluationID, ExecutionRecordID: fill.ExecutionRecordID,
			Rationale: guardrailRationale("BTC", "BUY", "100", fill.MarketObservedAt), RiskDecision: "ALLOW", RiskExecutionMode: "PAPER", ReasonCodes: guardrailJSON([]string{"ALLOWED"}),
			Checks:        guardrailJSON([]map[string]string{{"code": "AUTHORIZATION_DENIED", "result": "PASS", "message": "Identity is valid."}, {"code": "CAPITAL_LIMIT_EXCEEDED", "result": "PASS", "message": "Capital is within limits."}}),
			ExecutionMode: "PAPER", ExecutionStatus: "SIMULATED_FILLED", Symbol: "BTC", Instrument: "CRYPTO", Side: "BUY", ExecutionNotional: "100.2500000000",
		},
	}
	result := projectPaperGuardrailEvidence(guardrailCadence(start, end), rows, []paperRealizedFill{fill})
	if result.Status != PaperActivityCadenceAvailable || result.CalculationMethod != PaperGuardrailEvidenceMethod || result.AsOf == nil || result.SevenDays.Status != PaperActivityCadenceUnavailable {
		t.Fatalf("guardrail contract unavailable: %#v", result)
	}
	window := result.TwentyFourHours
	if window.Status != PaperActivityCadenceAvailable || window.ProposalCount != 2 || window.AllowCount != 1 || window.DenyCount != 1 || window.SimulatedFillCount != 1 ||
		window.MinimumProposedNotional != "50.0000000000" || window.MedianProposedNotional != "75.0000000000" || window.MaximumProposedNotional != "100.0000000000" ||
		len(window.DenialReasonCodes) != 1 || window.DenialReasonCodes[0].Code != "INSUFFICIENT_POSITION" || window.DenialReasonCodes[0].Count != 1 ||
		len(window.FailedCheckCodes) != 1 || window.FailedCheckCodes[0].Code != "INSUFFICIENT_POSITION" || len(window.Symbols) != 2 || len(window.Proposals) != 2 {
		t.Fatalf("exact guardrail attribution changed: %#v", window)
	}
	if window.Symbols[0].Symbol != "BTC" || window.Symbols[0].AllowCount != 1 || window.Symbols[0].ProposedNotional != "100.0000000000" ||
		window.Symbols[1].Symbol != "ETH" || window.Symbols[1].DenyCount != 1 || window.Proposals[0].DecisionJournalEntryID != "decision-deny" || window.Proposals[1].FinancialProvider != "coinbase" {
		t.Fatalf("symbol or immutable evidence attribution changed: %#v", window)
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
