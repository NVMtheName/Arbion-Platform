package strategy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Invoked by the mandatory isolated PostgreSQL Paper binding test.
func testPaperCommitEmergencyStops(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, scope := range []string{"GLOBAL", "USER", "ACCOUNT", "AUTOMATION"} {
		t.Run(scope+" stop wins before fill", func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			var stopID string
			err = tx.QueryRow(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source,engaged_by_user_id) VALUES($1,$2,'OPEN','test stop','SYSTEM',$3) RETURNING id::text`, scope, stopFixtureScope(scope, f), f.instance.UserID).Scan(&stopID)
			if err != nil {
				t.Fatal(err)
			}
			cleanupFixtureStop(t, pool, stopID)
			result := make(chan error, 1)
			go func() { result <- f.commit(ctx) }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, risk.ErrCommitCircuitBreakerActive) {
				t.Fatal("committed stop not observed after waiting", err)
			}
			f.assertEmpty(t, ctx)
		})
		t.Run(scope+" stop waits for guarded commit", func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			if scope == "GLOBAL" {
				if _, err := pool.Exec(ctx, `UPDATE users SET role='superadmin' WHERE id=$1`, f.instance.UserID); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err = risk.LockCircuitBreakersForCommit(ctx, tx, f.instance.UserID, f.instance.FinancialAccountID, f.instance.AutomationMandateID); err != nil {
				t.Fatal(err)
			}
			type response struct {
				breaker risk.CircuitBreaker
				err     error
			}
			result := make(chan response, 1)
			go func() { b, e := engageFixtureStop(ctx, scope, f); result <- response{b, e} }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			r := <-result
			if r.err != nil {
				t.Fatal(r.err)
			}
			cleanupFixtureStop(t, pool, r.breaker.ID)
			if err = f.commit(ctx); !errors.Is(err, risk.ErrCommitCircuitBreakerActive) {
				t.Fatal("acknowledged stop bypassed", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("unrelated accounts remain independent", func(t *testing.T) {
		f, other := newPaperBindingFixture(t, ctx, pool), newPaperBindingFixture(t, ctx, pool)
		for _, scope := range []string{"USER", "ACCOUNT", "AUTOMATION"} {
			b, err := engageFixtureStop(ctx, scope, other)
			if err != nil {
				t.Fatal(err)
			}
			cleanupFixtureStop(t, pool, b.ID)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `UPDATE risk_circuit_breakers SET reason='still stopped' WHERE scope='ACCOUNT' AND scope_id=$1`, other.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err = f.commit(bounded); err != nil {
			t.Fatal("foreign stop disrupted account", err)
		}
	})
	t.Run("same owner accounts share read locks but not account stops", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		other := newPaperBindingFixture(t, ctx, pool, f.instance.UserID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = risk.LockCircuitBreakersForCommit(ctx, tx, f.instance.UserID, f.instance.FinancialAccountID, f.instance.AutomationMandateID); err != nil {
			t.Fatal(err)
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		b, err := engageFixtureStop(bounded, "ACCOUNT", other)
		if err != nil {
			t.Fatal("same-owner account stop was serialized fleet-wide", err)
		}
		cleanupFixtureStop(t, pool, b.ID)
		if err = f.commit(bounded); err != nil {
			t.Fatal("same-owner foreign stop blocked fill", err)
		}
	})
	t.Run("release waits and duplicates remain safe", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		b, err := engageFixtureStop(ctx, "ACCOUNT", f)
		if err != nil {
			t.Fatal(err)
		}
		cleanupFixtureStop(t, pool, b.ID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = risk.LockCircuitBreakersForCommit(ctx, tx, f.instance.UserID, f.instance.FinancialAccountID, f.instance.AutomationMandateID); !errors.Is(err, risk.ErrCommitCircuitBreakerActive) {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, e := risk.NewPostgresBreakerStore(pool).ReleaseAccountBreaker(ctx, f.instance.UserID, f.instance.FinancialAccountID, f.now)
			result <- e
		}()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
		if err = f.commit(ctx); err != nil {
			t.Fatal("released stop blocked commit", err)
		}
		b, err = engageFixtureStop(ctx, "ACCOUNT", f)
		if err != nil {
			t.Fatal(err)
		}
		cleanupFixtureStop(t, pool, b.ID)
		if err = f.commit(ctx); !errors.Is(err, ErrDuplicate) {
			t.Fatal("stop broke completed-event recovery", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
	t.Run("rolled back stop permits fill", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('ACCOUNT',$1,'OPEN','rolled back','SYSTEM')`, f.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- f.commit(ctx) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("canceled fill cannot escape stop lock", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('ACCOUNT',$1,'OPEN','pending stop','SYSTEM')`, f.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		pending, cancel := context.WithCancel(ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- f.commit(pending) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		cancel()
		if err = <-result; !errors.Is(err, context.Canceled) {
			t.Fatal("canceled commit was not rejected", err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("canonical UUID and immutable scope identity", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		b, err := engageFixtureStop(ctx, "ACCOUNT", f)
		if err != nil {
			t.Fatal(err)
		}
		cleanupFixtureStop(t, pool, b.ID)
		if _, err = pool.Exec(ctx, `UPDATE risk_circuit_breakers SET scope_id=$2 WHERE id=$1`, b.ID, f.instance.UserID); err == nil {
			t.Fatal("scope identity changed")
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = risk.LockCircuitBreakersForCommit(ctx, tx, strings.ToUpper(f.instance.UserID), strings.ToUpper(f.instance.FinancialAccountID), strings.ToUpper(f.instance.AutomationMandateID)); !errors.Is(err, risk.ErrCommitCircuitBreakerActive) {
			t.Fatal("UUID spelling evaded stop", err)
		}
	})
	t.Run("repeatable read fails closed", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = risk.LockCircuitBreakersForCommit(ctx, tx, f.instance.UserID, f.instance.FinancialAccountID, f.instance.AutomationMandateID); !errors.Is(err, risk.ErrCommitGuardUnavailable) {
			t.Fatal("stale isolation accepted", err)
		}
	})
	t.Run("generic accepted-action path and denial evidence", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		b, err := engageFixtureStop(ctx, "AUTOMATION", f)
		if err != nil {
			t.Fatal(err)
		}
		cleanupFixtureStop(t, pool, b.ID)
		err = f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
		if !errors.Is(err, risk.ErrCommitCircuitBreakerActive) {
			t.Fatal("generic writer bypassed stop", err)
		}
		f.assertEmpty(t, ctx)
		f.evaluation.Decision = risk.Deny
		f.evaluation.ReasonCodes = []risk.ReasonCode{risk.CircuitBreakerActive}
		err = f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now)
		if err != nil {
			t.Fatal("immutable denial blocked", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
	})
}

func stopFixtureScope(scope string, f paperBindingFixture) any {
	switch scope {
	case "GLOBAL":
		return nil
	case "USER":
		return f.instance.UserID
	case "ACCOUNT":
		return f.instance.FinancialAccountID
	default:
		return f.instance.AutomationMandateID
	}
}

func engageFixtureStop(ctx context.Context, scope string, f paperBindingFixture) (risk.CircuitBreaker, error) {
	s := risk.NewPostgresBreakerStore(f.store.db)
	switch scope {
	case "GLOBAL":
		return s.EngageGlobalBreaker(ctx, f.instance.UserID, "test stop", f.now)
	case "USER":
		return s.EngageUserBreaker(ctx, f.instance.UserID, "test stop", f.now)
	case "ACCOUNT":
		return s.EngageAccountBreaker(ctx, f.instance.UserID, f.instance.FinancialAccountID, "test stop", f.now)
	default:
		return s.EngageAutomationBreaker(ctx, f.instance.UserID, f.instance.AutomationMandateID, "test stop", f.now)
	}
}

func cleanupFixtureStop(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, `UPDATE risk_circuit_breakers SET state='CLOSED',released_at=now() WHERE id=$1 AND state='OPEN'`, id); err != nil {
			t.Error("fixture cleanup", err)
		}
	})
}
