package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func commitShadowBindingFixture(ctx context.Context, f paperBindingFixture) error {
	return f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
}

func testAIShadowCommitBindings(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, status := range []string{"PAUSED", "DISABLED", "ARCHIVED"} {
		t.Run("current mandate "+status+" blocks prepared Shadow action", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, status, "UI"); err != nil {
				t.Fatal(err)
			}
			if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrEvaluationConfigurationChanged) {
				t.Fatal("revoked mandate accepted a prepared Shadow action", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("newer draft preserves pinned authorization and exact retry", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2 WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); err != nil {
			t.Fatal("unrelated draft replaced the approved version", err)
		}
		if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 2, "DISABLED", "UI"); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrDuplicate) {
			t.Fatal("revocation hid committed duplicate", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND mode='SHADOW' AND status='WOULD_HAVE_SUBMITTED'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE user_id='`+f.instance.UserID+`'`, 0)
	})
	for _, snapshot := range []string{
		`{"status":"DRAFT"}`, `{"execution_mode":"PAPER"}`,
		`{"automation_type":"STRATEGY"}`, `{"autonomy_level":"RESEARCH_ONLY"}`,
		`{"financial_account_id":"00000000-0000-0000-0000-000000000000"}`,
		`{"capital_bucket_id":"00000000-0000-0000-0000-000000000000"}`,
	} {
		t.Run("invalid pinned identity "+snapshot, func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(snapshot))
			if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrEvaluationConfigurationChanged) {
				t.Fatal("invalid pinned binding accepted", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	for _, target := range []string{"mandate", "runtime"} {
		t.Run(target+" pause wins its lock", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if target == "mandate" {
				_, err = tx.Exec(ctx, `UPDATE automation_mandates SET status='PAUSED',current_version=2 WHERE id=$1`, f.instance.AutomationMandateID)
			} else {
				_, err = tx.Exec(ctx, `UPDATE strategy_instances SET status='PAUSED',state_version=2 WHERE id=$1`, f.instance.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- commitShadowBindingFixture(ctx, f) }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			want := ErrEvaluationConfigurationChanged
			if target == "runtime" {
				want = ErrConflict
			}
			if err = <-result; !errors.Is(err, want) {
				t.Fatal("pause lost after lock wait", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("winning binding locks hold through transaction completion", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if amount, err := lockAINonLiveCommitBindings(ctx, tx, f.instance, Shadow); err != nil || !sameAIPaperDecimal(amount, "1000") {
			t.Fatal("valid bound claim rejected", amount, err)
		}
		result := make(chan error, 1)
		go func() {
			_, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "DISABLED", "UI")
			result <- err
		}()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("completion releases claim and blocks prepared action", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if _, err := f.store.Finish(ctx, f.instance.UserID, f.instance.ID, f.instance.StateVersion, f.now); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrConflict) {
			t.Fatal("released capital accepted", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("another account pause does not interrupt this account", func(t *testing.T) {
		paused := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		active := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`), paused.instance.UserID)
		if _, err := automation.NewPostgresStore(pool).Transition(ctx, paused.instance.UserID, paused.instance.AutomationMandateID, 1, "PAUSED", "UI"); err != nil {
			t.Fatal(err)
		}
		if err := commitShadowBindingFixture(ctx, paused); !errors.Is(err, ErrEvaluationConfigurationChanged) {
			t.Fatal(err)
		}
		paused.assertEmpty(t, ctx)
		if err := commitShadowBindingFixture(ctx, active); err != nil {
			t.Fatal("unrelated account interrupted", err)
		}
	})
	t.Run("denied and abstained evidence survive mandate pause", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "PAUSED", "UI"); err != nil {
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
	for _, kind := range []string{"FIXED_AMOUNT", "PERCENT_OF_AVAILABLE_CASH", "PERCENT_OF_BUYING_POWER"} {
		t.Run("exact capital basis "+kind, func(t *testing.T) {
			limit := "1000"
			value := "1000"
			if kind != "FIXED_AMOUNT" {
				value = "50"
			}
			f := newNonLiveBindingFixtureWithBucket(t, ctx, pool, Shadow, json.RawMessage(`{}`), &automation.CapitalBucket{AllocationType: kind, AllocationValue: value, Currency: "USD", ProtectedAmount: "100", AllocationLimit: &limit})
			if err := commitShadowBindingFixture(ctx, f); err != nil {
				t.Fatal("exact capped reservation rejected", err)
			}
			claim, err := f.store.CapitalReservation(ctx, f.instance.UserID, f.instance.ID)
			if err != nil || claim.ReservationAmount == nil || !sameAIPaperDecimal(*claim.ReservationAmount, "900") || claim.ExecutionMode != Shadow {
				t.Fatal("claim expanded or converted to Paper cash", err)
			}
			// Existing database policy remains intact; the commit guard must not
			// enable mid-runtime reallocations or manufacture replacement claims.
			if _, err = pool.Exec(ctx, `UPDATE capital_buckets SET allocation_value=allocation_value+1 WHERE id=$1`, f.instance.CapitalBucketID); err == nil {
				t.Fatal("active reservation no longer freezes allocation")
			}
		})
	}
	t.Run("accepted commit finishes before later mandate pause", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		gate, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Rollback(ctx)
		if _, err = gate.Exec(ctx, `SELECT id FROM strategy_instances WHERE id=$1 FOR NO KEY UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		committed := make(chan error, 1)
		go func() { committed <- commitShadowBindingFixture(ctx, f) }()
		waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
		var commitPID uint32
		if err = pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1::int=ANY(pg_blocking_pids(pid))`, int32(gate.Conn().PgConn().PID())).Scan(&commitPID); err != nil {
			t.Fatal(err)
		}
		paused := make(chan error, 1)
		go func() {
			_, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, "PAUSED", "UI")
			paused <- err
		}()
		// The real writer has locked the mandate before waiting on runtime.
		// A later pause must wait on that writer, not acknowledge early.
		waitForPaperBindingLock(t, ctx, pool, commitPID)
		if err = gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-committed; err != nil {
			t.Fatal("winning commit failed", err)
		}
		if err = <-paused; err != nil {
			t.Fatal("waiting pause failed", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`' AND status='WOULD_HAVE_SUBMITTED' AND mode='SHADOW'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM automation_mandates WHERE id='`+f.instance.AutomationMandateID+`' AND status='PAUSED'`, 1)
	})
	t.Run("concurrent duplicate has only one immutable result", func(t *testing.T) {
		f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
		results := make(chan error, 2)
		go func() { results <- commitShadowBindingFixture(ctx, f) }()
		go func() { results <- commitShadowBindingFixture(ctx, f) }()
		first, second := <-results, <-results
		if !((first == nil && errors.Is(second, ErrDuplicate)) || (second == nil && errors.Is(first, ErrDuplicate))) {
			t.Fatal("duplicate created a second outcome or deadlocked", first, second)
		}
		assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
		assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
}
