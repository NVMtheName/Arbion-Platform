package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise real Resume and mandate transitions against only the isolated test
// database. Table gates stop a writer at a known statement; observed database
// dependencies, not arbitrary delays, determine which writer won its lock.
func testResumeMandateSerialization(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("mandate disable owns lock first and Resume refuses after waiting", func(t *testing.T) {
		f := newPausedResumeFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `LOCK TABLE automation_mandate_versions IN SHARE MODE`); err != nil {
			t.Fatal(err)
		}
		disabled := make(chan error, 1)
		go func() {
			_, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "DISABLED", "UI")
			disabled <- err
		}()
		waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
		writerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
		resumed := make(chan error, 1)
		go func() {
			_, err := f.store.Resume(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now)
			resumed <- err
		}()
		blocked, earlyErr := waitForResumeLockOrCompletion(t, ctx, pool, writerPID, resumed)
		if !blocked {
			t.Error("Resume finished before the in-flight mandate revocation released its lock", earlyErr)
		}
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-disabled; err != nil {
			t.Fatal("mandate revocation failed", err)
		}
		if blocked {
			earlyErr = <-resumed
		}
		if !errors.Is(earlyErr, ErrMandateStale) {
			t.Error("Resume did not refuse the now-revoked mandate", earlyErr)
		}
		assertResumeUnchanged(t, ctx, f)
		assertCount(t, pool, `SELECT count(*) FROM automation_mandates WHERE id='`+f.instance.AutomationMandateID+`' AND status='DISABLED' AND current_version=2`, 1)
		assertCount(t, pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.instance.AutomationMandateID+`'`, 2)
	})
	t.Run("Resume owns mandate lock until its state transition commits", func(t *testing.T) {
		f := newPausedResumeFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `LOCK TABLE strategy_state_transitions IN SHARE MODE`); err != nil {
			t.Fatal(err)
		}
		resumed := make(chan error, 1)
		go func() {
			_, err := f.store.Resume(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now)
			resumed <- err
		}()
		waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
		resumePID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
		disabled := make(chan error, 1)
		go func() {
			_, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "DISABLED", "UI")
			disabled <- err
		}()
		blocked, earlyErr := waitForResumeLockOrCompletion(t, ctx, pool, resumePID, disabled)
		if !blocked {
			t.Error("mandate revocation committed while Resume was still unfinished", earlyErr)
		}
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-resumed; err != nil {
			t.Fatal("winning Resume failed", err)
		}
		if blocked {
			earlyErr = <-disabled
		}
		if earlyErr != nil {
			t.Fatal("later mandate revocation failed", earlyErr)
		}
		// ACTIVE is valid here: the Resume completed before the subsequent
		// revocation. This asserts ordering, not permission for another action.
		assertResumeCommittedOnce(t, ctx, f)
		assertCount(t, pool, `SELECT count(*) FROM automation_mandates WHERE id='`+f.instance.AutomationMandateID+`' AND status='DISABLED' AND current_version=2`, 1)
		assertCount(t, pool, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.instance.AutomationMandateID+`'`, 2)
	})
	for _, snapshot := range []string{
		`{"status":"DRAFT"}`, `{"execution_mode":"PAPER"}`,
		`{"financial_account_id":"00000000-0000-0000-0000-000000000000"}`,
		`{"capital_bucket_id":"00000000-0000-0000-0000-000000000000"}`,
	} {
		t.Run("pinned snapshot mismatch remains refused "+snapshot, func(t *testing.T) {
			f := newPausedResumeFixture(t, ctx, pool, Shadow, json.RawMessage(snapshot))
			if _, err := f.store.Resume(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now); !errors.Is(err, ErrMandateStale) {
				t.Fatal("mismatched immutable mandate binding resumed", err)
			}
			assertResumeUnchanged(t, ctx, f)
		})
	}
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		t.Run(string(mode)+" newer draft preserves approved pinned version", func(t *testing.T) {
			f := newPausedResumeFixture(t, ctx, pool, mode, json.RawMessage(`{}`))
			if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2 WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
				t.Fatal(err)
			}
			resumed, err := f.store.Resume(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now)
			if err != nil || resumed.MandateVersion != 1 || resumed.Status != "ACTIVE" {
				t.Fatal("newer draft displaced the pinned READY version", err)
			}
			assertResumeCommittedOnce(t, ctx, f)
			assertCount(t, pool, `SELECT count(*) FROM automation_mandates WHERE id='`+f.instance.AutomationMandateID+`' AND status='DRAFT' AND current_version=2`, 1)
		})
	}
	t.Run("two same-version Resume requests serialize without a deadlock", func(t *testing.T) {
		f := newPausedResumeFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `SELECT id FROM strategy_instances WHERE id=$1 FOR NO KEY UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		resumed := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := f.store.Resume(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now)
				resumed <- err
			}()
		}
		waitForInstanceIdentityWaiters(t, ctx, pool, gate.Conn().PgConn().PID(), 2)
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		first, second := <-resumed, <-resumed
		if !((first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict))) {
			t.Fatal("expected one Resume and one stale-version conflict, not a database deadlock", first, second)
		}
		assertResumeCommittedOnce(t, ctx, f)
	})
}

func newPausedResumeFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, mode ExecutionMode, snapshot json.RawMessage) paperBindingFixture {
	t.Helper()
	f := newNonLiveBindingFixture(t, ctx, pool, mode, snapshot)
	paused, err := f.store.Pause(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now)
	if err != nil || paused.Status != "PAUSED" || paused.StateVersion != 2 {
		t.Fatal("could not establish paused test instance", err)
	}
	f.instance = paused
	return f
}

func assertResumeUnchanged(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	f.assertEmpty(t, ctx)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND status='PAUSED' AND state_version=2 AND paused_at IS NOT NULL`, 1)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id='`+f.instance.ID+`'`, 2)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id='`+f.instance.ID+`' AND trigger='RESUMED'`, 0)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_capital_reservations WHERE strategy_instance_id='`+f.instance.ID+`' AND reservation_amount=1000 AND released_at IS NULL`, 1)
}

func assertResumeCommittedOnce(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	f.assertEmpty(t, ctx)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND status='ACTIVE' AND state_version=3 AND paused_at IS NULL`, 1)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id='`+f.instance.ID+`'`, 3)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_state_transitions WHERE strategy_instance_id='`+f.instance.ID+`' AND trigger='RESUMED'`, 1)
	assertCount(t, f.store.db, `SELECT count(*) FROM strategy_capital_reservations WHERE strategy_instance_id='`+f.instance.ID+`' AND reservation_amount=1000 AND released_at IS NULL`, 1)
}

func resumeBlockedPID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blocker uint32) uint32 {
	t.Helper()
	var pid uint32
	if err := pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid))`, int32(blocker)).Scan(&pid); err != nil {
		t.Fatal("expected one exact blocked transaction", err)
	}
	return pid
}

// Returns true only after a real lock dependency appears. A premature worker
// completion is returned separately so old code fails promptly rather than
// spending the test budget waiting for a lock it never takes.
func waitForResumeLockOrCompletion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blocker uint32, done <-chan error) (bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		select {
		case err := <-done:
			return false, err
		default:
		}
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid)))`, int32(blocker)).Scan(&blocked); err != nil {
			t.Fatal("worker did not reach the expected lock", err)
		}
		if blocked {
			return true, nil
		}
		select {
		case err := <-done:
			return false, err
		case <-ctx.Done():
			t.Fatal("worker neither completed nor reached the expected lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
