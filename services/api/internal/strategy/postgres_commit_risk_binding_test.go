package strategy

import (
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestAIShadowCommitRiskBinding(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 30, 0, 123000, time.UTC)
	instance := Instance{UserID: "owner", FinancialAccountID: "account", AutomationMandateID: "mandate", MandateVersion: 2, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow}
	for _, tc := range []struct {
		name   string
		mutate func(*risk.RiskEvaluation, *ExecutionResult)
	}{
		{"deny", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Decision = risk.Deny }},
		{"warn", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Decision = risk.Warn }},
		{"missing verdict", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Decision = "" }},
		{"approval required", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.ApprovalRequired = true }},
		{"executable claim", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.PlatformExecutionAvailable = true }},
		{"Paper verdict", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Mode = "PAPER" }},
		{"live verdict", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Mode = "LIVE" }},
		{"missing mode", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Mode = "" }},
		{"other owner", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.UserID = "other" }},
		{"other account", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.AccountID = "other" }},
		{"missing mandate", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.MandateID = nil }},
		{"other mandate", func(e *risk.RiskEvaluation, _ *ExecutionResult) { value := "other"; e.MandateID = &value }},
		{"missing version", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.MandateVersion = nil }},
		{"other version", func(e *risk.RiskEvaluation, _ *ExecutionResult) { value := 3; e.MandateVersion = &value }},
		{"missing time", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Timestamp = time.Time{} }},
		{"other time", func(e *risk.RiskEvaluation, _ *ExecutionResult) { e.Timestamp = e.Timestamp.Add(time.Nanosecond) }},
		{"simulation result", func(_ *risk.RiskEvaluation, r *ExecutionResult) { r.Status = SimulatedFilled }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := risk.RiskEvaluation{UserID: instance.UserID, AccountID: instance.FinancialAccountID, MandateID: &instance.AutomationMandateID, MandateVersion: &instance.MandateVersion, Decision: risk.Allow, Mode: "SHADOW", Timestamp: now}
			r := ExecutionResult{Status: WouldHaveSubmitted}
			if !validAIShadowCommitRisk(instance, e, r, now) {
				t.Fatal("matching non-live approval rejected")
			}
			tc.mutate(&e, &r)
			if validAIShadowCommitRisk(instance, e, r, now) {
				t.Fatal("inconsistent risk evidence accepted")
			}
		})
	}
	e := risk.RiskEvaluation{UserID: instance.UserID, AccountID: instance.FinancialAccountID, MandateID: &instance.AutomationMandateID, MandateVersion: &instance.MandateVersion, Decision: risk.Allow, Mode: "SHADOW", Timestamp: now.In(time.FixedZone("owner-zone", -4*60*60))}
	r := ExecutionResult{Status: WouldHaveSubmitted}
	if !validAIShadowCommitRisk(instance, e, r, now) || validAIShadowCommitRisk(instance, e, r, time.Time{}) {
		t.Fatal("same instant or missing trusted time handled incorrectly")
	}
	instance.ExecutionMode = Paper
	if validAIShadowCommitRisk(instance, e, r, now) {
		t.Fatal("Shadow contract accepted a Paper instance")
	}
}
