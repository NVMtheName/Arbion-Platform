package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests deliberately pass inconsistent internal values to persistence.
// They are not evidence of a production incident: the normal Shadow adapter
// currently converts non-ALLOW verdicts into RISK_DENIED before this boundary.
func testAIShadowCommitRiskBinding(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	tests := []struct {
		name   string
		mutate func(*paperBindingFixture)
	}{
		{"DENY verdict", func(f *paperBindingFixture) { f.evaluation.Decision = risk.Deny }},
		{"WARN approval-required verdict", func(f *paperBindingFixture) {
			f.evaluation.Decision, f.evaluation.ApprovalRequired = risk.Warn, true
		}},
		{"ALLOW still requires approval", func(f *paperBindingFixture) { f.evaluation.ApprovalRequired = true }},
		{"Paper risk mode", func(f *paperBindingFixture) { f.evaluation.Mode = "PAPER" }},
		{"live risk mode", func(f *paperBindingFixture) { f.evaluation.Mode = "LIVE" }},
		{"missing risk mode", func(f *paperBindingFixture) { f.evaluation.Mode = "" }},
		{"platform execution flag", func(f *paperBindingFixture) { f.evaluation.PlatformExecutionAvailable = true }},
		{"missing risk mandate", func(f *paperBindingFixture) { f.evaluation.MandateID = nil }},
		{"different risk mandate", func(f *paperBindingFixture) {
			other := f.instance.UserID
			f.evaluation.MandateID = &other
		}},
		{"missing risk version", func(f *paperBindingFixture) { f.evaluation.MandateVersion = nil }},
		{"different risk version", func(f *paperBindingFixture) {
			other := f.instance.MandateVersion + 1
			f.evaluation.MandateVersion = &other
		}},
		{"missing risk timestamp", func(f *paperBindingFixture) { f.evaluation.Timestamp = time.Time{} }},
		{"different risk timestamp", func(f *paperBindingFixture) { f.evaluation.Timestamp = f.now.Add(-time.Second) }},
		// REQUIRE_APPROVAL is not an enum value in this platform. The schema
		// already refuses it; preserve that boundary beside the supported WARN.
		{"unsupported REQUIRE_APPROVAL spelling", func(f *paperBindingFixture) { f.evaluation.Decision = risk.Decision("REQUIRE_APPROVAL") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			tc.mutate(&f)
			if err := commitShadowBindingFixture(ctx, f); err == nil {
				t.Fatal("inconsistent risk verdict produced accepted Shadow evidence")
			}
			assertShadowRiskBindingEmpty(t, ctx, f)
		})
	}
	t.Run("Shadow cannot persist a Paper simulated-fill disposition", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: SimulatedFilled, ExpectedState: AIMonitoring}, f.now); err == nil {
			t.Fatal("Shadow accepted a simulated-fill status without its Paper ledger")
		}
		assertShadowRiskBindingEmpty(t, ctx, f)
	})
	t.Run("complete matching ALLOW commits once", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal("valid Shadow verdict was rejected", err)
		}
		if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrDuplicate) {
			t.Fatal("exact committed delivery was not a duplicate", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM risk_evaluations WHERE user_id='`+f.instance.UserID+`' AND decision='ALLOW' AND NOT approval_required AND execution_mode='SHADOW' AND NOT platform_execution_available`, 1)
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='WOULD_HAVE_SUBMITTED' AND mode='SHADOW'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`' AND decision_type='ALLOW_WOULD_HAVE_SUBMITTED'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+f.instance.UserID+`'`, 0)
		assertCount(t, pool, `SELECT count(*) FROM order_intents WHERE user_id='`+f.instance.UserID+`'`, 0)
	})
	for _, verdict := range []risk.Decision{risk.Deny, risk.Warn} {
		t.Run("legitimate "+string(verdict)+" history remains immutable and duplicate-safe", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			f.evaluation.Decision = verdict
			f.evaluation.ApprovalRequired = verdict == risk.Warn
			result := ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring, Reason: "risk_not_allowed"}
			if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, result, f.now); err != nil {
				t.Fatal("denial evidence was lost", err)
			}
			if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, result, f.now); !errors.Is(err, ErrDuplicate) {
				t.Fatal("denied delivery was not duplicate-safe", err)
			}
			assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='RISK_DENIED' AND mode='SHADOW'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='WOULD_HAVE_SUBMITTED'`, 0)
			assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+f.instance.UserID+`'`, 0)
			assertCount(t, pool, `SELECT count(*) FROM order_intents WHERE user_id='`+f.instance.UserID+`'`, 0)
		})
	}
}

func assertShadowRiskBindingEmpty(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	f.assertEmpty(t, ctx)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND state_version=1 AND current_state='AI_MONITORING' AND status='ACTIVE' AND last_evaluated_at IS NULL`, 1)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_capital_reservations WHERE strategy_instance_id='`+f.instance.ID+`' AND execution_mode='SHADOW' AND reservation_amount=1000 AND released_at IS NULL`, 1)
}
