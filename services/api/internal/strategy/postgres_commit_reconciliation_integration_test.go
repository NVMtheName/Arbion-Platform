package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Construct only new synthetic immutable evidence in the isolated strategy DB.
// The report is already classified; tests do not mutate or bypass its guards.
func commitReconciliationReport(f paperBindingFixture, status string, observedAt time.Time) financialconnection.PortfolioReconciliation {
	r := financialconnection.PortfolioReconciliation{
		FinancialAccountID: f.instance.FinancialAccountID, Provider: "coinbase",
		ComparisonStatus: status, BalancesStatus: "READY", PositionsStatus: "READY",
		PerformanceStatus: "UNAVAILABLE", RealizedPerformanceStatus: "UNAVAILABLE",
		AutonomySignal: "CLEAR", AutonomyEnforcementActive: true,
		Changes: []financialconnection.ReconciliationChange{}, Positions: []financialconnection.ReconciliationPosition{},
		ObservedAt: observedAt,
	}
	switch status {
	case "DRIFT_DETECTED":
		r.AutonomySignal, r.BlocksNewActions = "REVIEW_RECOMMENDED", true
		r.ChangeCount, r.BlockingChangeCount = 1, 1
		r.Changes = []financialconnection.ReconciliationChange{{Symbol: "BTC", InstrumentType: "CRYPTO", Direction: "long", ChangeType: "POSITION_DISAPPEARED", ControlImpact: "TRADABLE_INVENTORY", PreviousQuantity: "1"}}
	case "INCOMPLETE":
		r.PositionsStatus, r.AutonomySignal, r.BlocksNewActions = "UNAVAILABLE", "INSUFFICIENT_EVIDENCE", true
	case "BASELINE":
		r.AutonomySignal, r.BlocksNewActions = "INSUFFICIENT_EVIDENCE", true
	}
	return r
}

func saveCommitReconciliation(ctx context.Context, pool *pgxpool.Pool, f paperBindingFixture, status string, observedAt time.Time) error {
	_, err := financialconnection.NewPostgresStore(pool).CreateReconciliation(ctx, f.instance.UserID, commitReconciliationReport(f, status, observedAt), make([]byte, 32))
	return err
}

func assertReconciliationCommitEmpty(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	f.assertEmpty(t, ctx)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND state_version=1 AND current_state='AI_MONITORING' AND status='ACTIVE' AND last_evaluated_at IS NULL`, 1)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_capital_reservations WHERE strategy_instance_id='`+f.instance.ID+`' AND execution_mode='SHADOW' AND reservation_amount=1000 AND released_at IS NULL`, 1)
}

func testAICommitReconciliation(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, status := range []string{"DRIFT_DETECTED", "INCOMPLETE", "BASELINE"} {
		t.Run("latest "+status+" rejects prepared Shadow action", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			// f contains an already-prepared ALLOW decision. A later snapshot
			// must be checked at persistence, not hidden by the earlier match.
			if err := saveCommitReconciliation(ctx, pool, f, status, f.now.Add(-30*time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := commitShadowBindingFixture(ctx, f); err == nil {
				t.Fatal("accepted Shadow action after newer blocking reconciliation")
			}
			assertReconciliationCommitEmpty(t, ctx, f)
			assertCount(t, pool, `SELECT count(*) FROM portfolio_reconciliations WHERE financial_account_id='`+f.instance.FinancialAccountID+`'`, 2)
		})
	}
	t.Run("missing current report rejects prepared Shadow action", func(t *testing.T) {
		f := newNonLiveBindingFixtureWithReconciliation(t, ctx, pool, Shadow, json.RawMessage(`{}`), nil, false)
		if err := commitShadowBindingFixture(ctx, f); err == nil {
			t.Fatal("accepted Shadow action without enforced reconciliation")
		}
		assertReconciliationCommitEmpty(t, ctx, f)
	})
	for _, kind := range []string{"stale", "future", "legacy advisory"} {
		t.Run(kind+" report rejects prepared Shadow action", func(t *testing.T) {
			f := newNonLiveBindingFixtureWithReconciliation(t, ctx, pool, Shadow, json.RawMessage(`{}`), nil, false)
			report := commitReconciliationReport(f, "MATCHED", f.now.Add(-time.Minute))
			switch kind {
			case "stale":
				report.ObservedAt = f.now.Add(-risk.AutonomousReconciliationMaxAge - time.Minute)
			case "future":
				report.ObservedAt = f.now.Add(time.Hour)
			case "legacy advisory":
				report.AutonomyEnforcementActive = false
			}
			if _, err := financialconnection.NewPostgresStore(pool).CreateReconciliation(ctx, f.instance.UserID, report, make([]byte, 32)); err != nil {
				t.Fatal(err)
			}
			if err := commitShadowBindingFixture(ctx, f); err == nil {
				t.Fatal("accepted Shadow action without current enforced evidence")
			}
			assertReconciliationCommitEmpty(t, ctx, f)
		})
	}
	t.Run("latest complete match remains eligible", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if err := saveCommitReconciliation(ctx, pool, f, "MATCHED", f.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal("benign latest matched snapshot blocked Shadow", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND mode='SHADOW' AND status='WOULD_HAVE_SUBMITTED'`, 1)
	})
	t.Run("reconciliation writer wins account lock", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM financial_accounts WHERE id=$1 FOR NO KEY UPDATE`, f.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,autonomy_enforcement_active,blocks_new_actions,observed_position_count,performance_position_count,change_count,blocking_change_count,changes,evidence_hash,observed_at) VALUES($1,$2,'coinbase','INCOMPLETE','READY','UNAVAILABLE','UNAVAILABLE','UNAVAILABLE','INSUFFICIENT_EVIDENCE',true,true,0,0,0,0,'[]',decode(repeat('ab',32),'hex'),$3)`, f.instance.UserID, f.instance.FinancialAccountID, f.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- commitShadowBindingFixture(ctx, f) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err == nil {
			t.Fatal("prepared Shadow action ignored report committed during lock wait")
		}
		assertReconciliationCommitEmpty(t, ctx, f)
	})
	t.Run("another account reconciliation does not interrupt this account", func(t *testing.T) {
		blocked := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		active := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`), blocked.instance.UserID)
		if err := saveCommitReconciliation(ctx, pool, blocked, "DRIFT_DETECTED", blocked.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, blocked); err == nil {
			t.Fatal("blocked account accepted a prepared action")
		}
		assertReconciliationCommitEmpty(t, ctx, blocked)
		if err := commitShadowBindingFixture(ctx, active); err != nil {
			t.Fatal("unrelated account was interrupted", err)
		}
	})
	t.Run("Paper stays isolated from real account reconciliation", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if err := saveCommitReconciliation(ctx, pool, f, "DRIFT_DETECTED", f.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); err != nil {
			t.Fatal("broker reconciliation interrupted isolated Paper ledger", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
	t.Run("denied and abstained evidence survives latest drift", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if err := saveCommitReconciliation(ctx, pool, f, "DRIFT_DETECTED", f.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		f.evaluation.Decision = risk.Deny
		if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now); err != nil {
			t.Fatal(err)
		}
		if err := f.store.CommitAIAbstention(ctx, f.instance, "abstain:"+f.instance.ID, json.RawMessage(`{"decision":"ABSTAIN"}`), f.now); err != nil {
			t.Fatal(err)
		}
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 2)
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='WOULD_HAVE_SUBMITTED'`, 0)
	})
	t.Run("completed exact duplicate survives later drift", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal(err)
		}
		if err := saveCommitReconciliation(ctx, pool, f, "DRIFT_DETECTED", f.now.Add(-30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrDuplicate) {
			t.Fatal("later drift hid exact committed duplicate", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
}
