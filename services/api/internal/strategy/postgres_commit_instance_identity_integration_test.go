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

// These synthetic callers deliberately disagree with the stored engine. Normal
// evaluation reloads its instance before dispatch; these are persistence-boundary
// regressions, not evidence of a production incident or a live execution path.
func testCommitInstanceIdentity(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("supplied wheel label cannot bypass stored AI Shadow identity", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		candidate := cloneInstanceIdentityFixture(f)
		candidate.instance.StrategyIdentifier = "wheel"
		candidate.decision.Source, candidate.decision.QuoteReference = "STRATEGY", nil
		candidate.decision.ProposedAction.Source = risk.SourceStrategy
		candidate.evaluation.Decision = risk.Deny
		if err := commitShadowBindingFixture(ctx, candidate); !errors.Is(err, ErrConflict) {
			t.Fatal("supplied label escaped the persisted instance identity", err)
		}
		assertShadowRiskBindingEmpty(t, ctx, f)
	})
	for _, path := range []string{"denied", "abstained"} {
		for _, field := range []string{"definition", "mode", "account", "capital", "empty capital", "mandate", "mandate version"} {
			t.Run(path+" rejects supplied "+field+" mismatch", func(t *testing.T) {
				f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
				candidate := cloneInstanceIdentityFixture(f)
				switch field {
				case "definition":
					candidate.instance.DefinitionVersion++
				case "mode":
					candidate.instance.ExecutionMode = Paper
				case "account", "capital", "mandate":
					// Existing same-owner objects ensure refusal is the identity
					// predicate, not a nonexistent foreign key or syntax error.
					other := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`), f.instance.UserID)
					switch field {
					case "account":
						candidate.instance.FinancialAccountID = other.instance.FinancialAccountID
					case "capital":
						candidate.instance.CapitalBucketID = other.instance.CapitalBucketID
					case "mandate":
						candidate.instance.AutomationMandateID = other.instance.AutomationMandateID
					}
				case "empty capital":
					candidate.instance.CapitalBucketID = ""
				case "mandate version":
					candidate.instance.MandateVersion++
					if _, err := pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
						t.Fatal(err)
					}
				}
				bindInstanceIdentityFixture(&candidate)
				if err := commitInstanceIdentityPath(ctx, candidate, path); !errors.Is(err, ErrConflict) {
					t.Fatal("inconsistent saved-engine identity was accepted", err)
				}
				assertShadowRiskBindingEmpty(t, ctx, f)
			})
		}
	}
	t.Run("accepted Shadow definition mismatch rolls back all evidence", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		candidate := f
		candidate.instance.DefinitionVersion++
		if err := commitShadowBindingFixture(ctx, candidate); !errors.Is(err, ErrConflict) {
			t.Fatal("accepted Shadow definition mismatch was persisted", err)
		}
		assertShadowRiskBindingEmpty(t, ctx, f)
	})
	t.Run("dedicated Paper definition mismatch rolls back cash position and evidence", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		candidate := f
		candidate.instance.DefinitionVersion++
		if err := candidate.commit(ctx); !errors.Is(err, ErrConflict) {
			t.Fatal("accepted Paper definition mismatch was persisted", err)
		}
		f.assertEmpty(t, ctx)
		assertCount(t, pool, `SELECT count(*) FROM paper_portfolios WHERE strategy_instance_id='`+f.instance.ID+`' AND version=1 AND cash=1000`, 1)
		assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND strategy_definition_version=1 AND state_version=1 AND last_evaluated_at IS NULL`, 1)
	})
	for _, path := range []string{"accepted", "denied", "abstained"} {
		t.Run(path+" matching identity commits and remains duplicate after identity changes", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			if err := commitInstanceIdentityPath(ctx, f, path); err != nil {
				t.Fatal("matching instance was refused", err)
			}
			if _, err := pool.Exec(ctx, `UPDATE strategy_instances SET strategy_definition_version=2 WHERE id=$1`, f.instance.ID); err != nil {
				t.Fatal(err)
			}
			if err := commitInstanceIdentityPath(ctx, f, path); !errors.Is(err, ErrDuplicate) {
				t.Fatal("completed immutable delivery lost duplicate precedence", err)
			}
			assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND strategy_definition_version=2 AND state_version=1 AND current_state='AI_MONITORING' AND last_evaluated_at IS NOT NULL`, 1)
			wantActions := 1
			if path == "abstained" {
				wantActions = 0
			}
			assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, wantActions)
			assertCount(t, pool, `SELECT count(*) FROM risk_evaluations WHERE user_id='`+f.instance.UserID+`'`, wantActions)
			assertCount(t, pool, `SELECT count(*) FROM order_intents WHERE user_id='`+f.instance.UserID+`'`, 0)
		})
		t.Run(path+" rechecks identity after concurrent change wins row lock", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback(ctx)
			if _, err = writer.Exec(ctx, `UPDATE strategy_instances SET strategy_definition_version=2 WHERE id=$1`, f.instance.ID); err != nil {
				t.Fatal(err)
			}
			committed := make(chan error, 1)
			go func() { committed <- commitInstanceIdentityPath(ctx, f, path) }()
			waitForPaperBindingLock(t, ctx, pool, writer.Conn().PgConn().PID())
			if err = writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-committed; !errors.Is(err, ErrConflict) {
				t.Fatal("identity committed during row-lock wait was ignored", err)
			}
			assertShadowRiskBindingEmpty(t, ctx, f)
			assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND strategy_definition_version=2`, 1)
		})
	}
	t.Run("accepted Shadow lock wins before later identity writer", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `SELECT strategy_instance_id FROM strategy_capital_reservations WHERE strategy_instance_id=$1 FOR NO KEY UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		committed := make(chan error, 1)
		go func() { committed <- commitShadowBindingFixture(ctx, f) }()
		waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
		var commitPID uint32
		if err = pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid))`, int32(gate.Conn().PgConn().PID())).Scan(&commitPID); err != nil {
			t.Fatal(err)
		}
		changed := make(chan error, 1)
		go func() {
			_, err := pool.Exec(ctx, `UPDATE strategy_instances SET strategy_definition_version=2 WHERE id=$1`, f.instance.ID)
			changed <- err
		}()
		// The accepted commit locks its runtime before its reservation. Prove
		// that the later writer is waiting on that exact commit, not our gate.
		waitForPaperBindingLock(t, ctx, pool, commitPID)
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-committed; err != nil {
			t.Fatal("winning accepted commit failed", err)
		}
		if err = <-changed; err != nil {
			t.Fatal("later identity writer failed", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='WOULD_HAVE_SUBMITTED' AND mode='SHADOW'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND strategy_definition_version=2 AND last_evaluated_at IS NOT NULL`, 1)
		assertCount(t, pool, `SELECT count(*) FROM order_intents WHERE user_id='`+f.instance.UserID+`'`, 0)
	})
	t.Run("distinct concurrent abstentions preserve both without key-lock upgrade", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `SELECT id FROM strategy_instances WHERE id=$1 FOR NO KEY UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		committed := make(chan error, 2)
		for _, suffix := range []string{"first", "second"} {
			go func(suffix string) {
				committed <- f.store.CommitAIAbstention(ctx, f.instance, "abstain:"+f.instance.ID+":"+suffix, json.RawMessage(`{"decision":"ABSTAIN"}`), f.now)
			}(suffix)
		}
		// Both event claims retain compatible FK KEY SHARE locks. Wait until
		// both final writes are blocked before releasing the runtime gate.
		waitForInstanceIdentityWaiters(t, ctx, pool, gate.Conn().PgConn().PID(), 2)
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if first, second := <-committed, <-committed; first != nil || second != nil {
			t.Fatal("distinct abstentions were lost or deadlocked", first, second)
		}
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`' AND decision_type='ABSTAIN'`, 2)
		assertCount(t, pool, `SELECT count(*) FROM strategy_evaluation_events WHERE strategy_instance_id='`+f.instance.ID+`'`, 2)
		assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND state_version=1 AND current_state='AI_MONITORING'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
		assertCount(t, pool, `SELECT count(*) FROM order_intents WHERE user_id='`+f.instance.UserID+`'`, 0)
	})
}

func cloneInstanceIdentityFixture(f paperBindingFixture) paperBindingFixture {
	action := *f.decision.ProposedAction
	f.decision.ProposedAction = &action
	return f
}

func bindInstanceIdentityFixture(f *paperBindingFixture) {
	f.decision.ProposedAction.FinancialAccountID = f.instance.FinancialAccountID
	f.decision.ProposedAction.MandateID, f.decision.ProposedAction.MandateVersion = &f.instance.AutomationMandateID, &f.instance.MandateVersion
	f.evaluation.AccountID = f.instance.FinancialAccountID
	f.evaluation.MandateID, f.evaluation.MandateVersion, f.evaluation.Mode = &f.instance.AutomationMandateID, &f.instance.MandateVersion, string(f.instance.ExecutionMode)
}

func commitInstanceIdentityPath(ctx context.Context, f paperBindingFixture, path string) error {
	if path == "abstained" {
		return f.store.CommitAIAbstention(ctx, f.instance, "abstain:"+f.instance.ID, json.RawMessage(`{"decision":"ABSTAIN"}`), f.now)
	}
	if path == "denied" {
		f.evaluation.Decision = risk.Deny
		return f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring, Reason: "risk_not_allowed"}, f.now)
	}
	return commitShadowBindingFixture(ctx, f)
}

func waitForInstanceIdentityWaiters(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blocker uint32, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid))`, int32(blocker)).Scan(&count); err != nil {
			t.Fatal("concurrent deliveries did not reach the database lock", err)
		}
		if count == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("concurrent deliveries did not both reach the database lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
