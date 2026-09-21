package risk

import (
	"reflect"
	"testing"
	"time"
)

func TestCheckAutonomousReconciliationPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReconciliationSnapshot, *time.Time)
		code   ReasonCode
		result CheckResult
	}{
		{"current matched", func(*ReconciliationSnapshot, *time.Time) {}, ReconciliationRequired, Pass},
		{"new matched nonblocking changes", func(s *ReconciliationSnapshot, now *time.Time) {
			s.ObservedAt, s.ChangeCount = *now, 12
		}, ReconciliationRequired, Pass},
		{"exact maximum age", func(s *ReconciliationSnapshot, now *time.Time) {
			s.ObservedAt = now.Add(-AutonomousReconciliationMaxAge)
		}, ReconciliationRequired, Pass},
		{"wrong account", func(s *ReconciliationSnapshot, _ *time.Time) { s.AccountID = "other" }, ReconciliationRequired, Fail},
		{"advisory evidence", func(s *ReconciliationSnapshot, _ *time.Time) { s.AutonomyEnforcementActive = false }, ReconciliationRequired, Fail},
		{"zero trusted clock", func(_ *ReconciliationSnapshot, now *time.Time) { *now = time.Time{} }, ReconciliationStale, Fail},
		{"zero observation", func(s *ReconciliationSnapshot, _ *time.Time) { s.ObservedAt = time.Time{} }, ReconciliationStale, Fail},
		{"future observation", func(s *ReconciliationSnapshot, now *time.Time) { s.ObservedAt = now.Add(time.Nanosecond) }, ReconciliationStale, Fail},
		{"expired observation", func(s *ReconciliationSnapshot, now *time.Time) {
			s.ObservedAt = now.Add(-AutonomousReconciliationMaxAge - time.Nanosecond)
		}, ReconciliationStale, Fail},
		{"incomplete comparison", func(s *ReconciliationSnapshot, _ *time.Time) { s.ComparisonStatus = "INCOMPLETE" }, ReconciliationIncomplete, Fail},
		{"missing balance coverage", func(s *ReconciliationSnapshot, _ *time.Time) { s.BalancesStatus = "UNAVAILABLE" }, ReconciliationIncomplete, Fail},
		{"missing position coverage", func(s *ReconciliationSnapshot, _ *time.Time) { s.PositionsStatus = "UNAVAILABLE" }, ReconciliationIncomplete, Fail},
		{"drift comparison", func(s *ReconciliationSnapshot, _ *time.Time) { s.ComparisonStatus = "DRIFT_DETECTED" }, ReconciliationDriftDetected, Fail},
		{"review required", func(s *ReconciliationSnapshot, _ *time.Time) { s.AutonomySignal = "REVIEW_RECOMMENDED" }, ReconciliationDriftDetected, Fail},
		{"blocking change", func(s *ReconciliationSnapshot, _ *time.Time) { s.BlockingChangeCount = 1 }, ReconciliationDriftDetected, Fail},
		{"baseline", func(s *ReconciliationSnapshot, _ *time.Time) { s.ComparisonStatus = "BASELINE" }, ReconciliationRequired, Fail},
		{"unknown comparison", func(s *ReconciliationSnapshot, _ *time.Time) { s.ComparisonStatus = "UNKNOWN" }, ReconciliationRequired, Fail},
		{"insufficient evidence", func(s *ReconciliationSnapshot, _ *time.Time) { s.AutonomySignal = "INSUFFICIENT_EVIDENCE" }, ReconciliationRequired, Fail},
		{"explicit blocker", func(s *ReconciliationSnapshot, _ *time.Time) { s.BlocksNewActions = true }, ReconciliationRequired, Fail},
		{"staleness before drift", func(s *ReconciliationSnapshot, now *time.Time) {
			s.ComparisonStatus, s.ObservedAt = "DRIFT_DETECTED", now.Add(-25*time.Hour)
		}, ReconciliationStale, Fail},
		{"incomplete before drift", func(s *ReconciliationSnapshot, _ *time.Time) {
			s.PositionsStatus, s.ComparisonStatus = "UNAVAILABLE", "DRIFT_DETECTED"
		}, ReconciliationIncomplete, Fail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			context, action := fixture()
			addMatchedReconciliation(&context)
			snapshot, now := context.Reconciliation, context.Now
			tc.mutate(snapshot, &now)
			before := *snapshot
			first := CheckAutonomousReconciliation(snapshot, action.FinancialAccountID, now)
			if first.Code != tc.code || first.Result != tc.result || first.Message == "" {
				t.Fatalf("unexpected shared policy: %#v", first)
			}
			if again := CheckAutonomousReconciliation(snapshot, action.FinancialAccountID, now); again != first || !reflect.DeepEqual(before, *snapshot) {
				t.Fatal("shared policy is not pure and deterministic")
			}
			context.Reconciliation, context.Now = snapshot, now
			context.Mandate.AutomationType, context.Mandate.ExecutionMode, action.Source = "AI_AUTONOMOUS", "SHADOW", SourceAI
			if got := reconciliationRule(&context, action); got != first {
				t.Fatalf("evaluation policy diverged from shared policy: %#v != %#v", got, first)
			}
		})
	}
	context, action := fixture()
	if got := CheckAutonomousReconciliation(nil, action.FinancialAccountID, context.Now); got.Code != ReconciliationRequired || got.Result != Fail {
		t.Fatalf("missing evidence accepted: %#v", got)
	}
}

func TestAutonomousReconciliationRuleApplicabilityPreserved(t *testing.T) {
	for _, mutate := range []func(*EvaluationContext, *ProposedAction){
		func(_ *EvaluationContext, a *ProposedAction) { a.Source = SourceUI },
		func(c *EvaluationContext, _ *ProposedAction) { c.Mandate = nil },
		func(c *EvaluationContext, _ *ProposedAction) { c.Mandate.AutomationType = "STRATEGY" },
		func(c *EvaluationContext, _ *ProposedAction) { c.Mandate.ExecutionMode = "PAPER" },
	} {
		context, action := fixture()
		context.Mandate.AutomationType, context.Mandate.ExecutionMode, action.Source = "AI_AUTONOMOUS", "SHADOW", SourceAI
		mutate(&context, &action)
		if got := reconciliationRule(&context, action); got.Code != ReconciliationRequired || got.Result != Pass || got.Message != "The autonomous reconciliation gate does not apply." {
			t.Fatalf("nonapplicable gate was changed: %#v", got)
		}
	}
}
