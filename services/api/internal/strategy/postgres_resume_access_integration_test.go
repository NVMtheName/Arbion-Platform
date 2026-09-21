package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resume is a persisted activation, not execution. These legal stored-state
// fixtures check the real service with a previously captured founder principal;
// they do not claim that the public admin service permits founder downgrades.
func testResumeCurrentAccess(t *testing.T, parent context.Context, pool *pgxpool.Pool) {
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		for _, tc := range []struct{ name, query string }{
			{"disabled owner", `UPDATE users SET status='disabled' WHERE id=$1`},
			{"unverified owner", `UPDATE users SET status='pending_verification' WHERE id=$1`},
			{"revoked founder", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`},
			{"deleted founder", `DELETE FROM user_entitlements WHERE user_id=$1 AND entitlement_key='founder'`},
			{"future founder", `UPDATE user_entitlements SET starts_at=clock_timestamp()+interval '1 hour' WHERE user_id=$1 AND entitlement_key='founder'`},
		} {
			t.Run(string(mode)+" captured principal cannot resume "+tc.name, func(t *testing.T) {
				f := newPausedResumeFixture(t, parent, pool, mode, json.RawMessage(`{}`))
				resume := capturedFounderResumer(f)
				if _, err := pool.Exec(parent, tc.query, f.instance.UserID); err != nil {
					t.Fatal(err)
				}
				if err := resume(parent); !errors.Is(err, ErrForbidden) {
					t.Error("stale captured founder resumed a paused engine", err)
				}
				assertDeniedResumeAccessUnchanged(t, parent, f)
			})
		}
		for _, tc := range []struct{ name, query string }{
			{"owner", `UPDATE users SET status='disabled' WHERE id=$1`},
			{"founder", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`},
		} {
			t.Run(string(mode)+" "+tc.name+" revocation wins Resume wait", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newPausedResumeFixture(t, ctx, pool, mode, json.RawMessage(`{}`))
				resume := capturedFounderResumer(f)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, tc.query, f.instance.UserID); err != nil {
					t.Fatal(err)
				}
				resumed := start(func() error { return resume(ctx) })
				blocked, resumeErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), resumed)
				if !blocked {
					t.Error("Resume passed the unfinished authority writer", resumeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if blocked {
					resumeErr = <-resumed
				}
				if !errors.Is(resumeErr, ErrForbidden) {
					t.Error("Resume did not refuse the now-current revoked authority", resumeErr)
				}
				assertDeniedResumeAccessUnchanged(t, ctx, f)
			})
			t.Run(string(mode)+" Resume retains authority until "+tc.name+" revocation can commit", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newPausedResumeFixture(t, ctx, pool, mode, json.RawMessage(`{}`))
				resume := capturedFounderResumer(f)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, `LOCK TABLE strategy_state_transitions IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				resumed := start(func() error { return resume(ctx) })
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				resumePID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
				revoked := start(func() error {
					_, err := pool.Exec(ctx, tc.query, f.instance.UserID)
					return err
				})
				blocked, revokeErr := waitForResumeLockOrCompletion(t, ctx, pool, resumePID, revoked)
				if !blocked {
					t.Error("revocation committed while Resume was still unfinished", revokeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				resumeErr := <-resumed
				if blocked {
					revokeErr = <-revoked
				}
				if resumeErr != nil || revokeErr != nil {
					t.Fatal("ordered Resume and later revocation did not finish", resumeErr, revokeErr)
				}
				// Activation won the ordering, but later evaluations retain
				// their independent current-access guards. No action ran here.
				assertResumeCommittedOnce(t, ctx, f)
			})
		}
		t.Run(string(mode)+" founder grant effective after transaction start is current after Resume wait", func(t *testing.T) {
			ctx, start := initializeRaceWorkers(t, parent)
			f := newPausedResumeFixture(t, ctx, pool, mode, json.RawMessage(`{}`))
			resume := capturedFounderResumer(f)
			gate, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Rollback(ctx)
			if _, err = gate.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.instance.UserID); err != nil {
				t.Fatal(err)
			}
			// Before the owner guard exists, Resume reaches its mandate lock
			// first. Keep this baseline control bounded in either code version;
			// the corrected path instead waits at the earlier owner row.
			if _, err = gate.Exec(ctx, `SELECT id FROM automation_mandates WHERE id=$1 FOR UPDATE`, f.instance.AutomationMandateID); err != nil {
				t.Fatal(err)
			}
			resumed := start(func() error { return resume(ctx) })
			waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
			resumePID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
			var transactionStartedAt, startsAt time.Time
			if err = pool.QueryRow(ctx, `SELECT xact_start FROM pg_stat_activity WHERE pid=$1`, int32(resumePID)).Scan(&transactionStartedAt); err != nil {
				t.Fatal("could not observe blocked Resume transaction start", err)
			}
			// Founder expiry is prohibited by the schema. A legal grant made
			// effective after transaction start tests post-wait wall-clock
			// behavior without weakening constraints or using timed sleeps.
			if err = gate.QueryRow(ctx, `UPDATE user_entitlements SET starts_at=clock_timestamp() WHERE user_id=$1 AND entitlement_key='founder' RETURNING starts_at`, f.instance.UserID).Scan(&startsAt); err != nil || !startsAt.After(transactionStartedAt) {
				t.Fatal("fixture did not place effective grant after transaction start", err)
			}
			if err = gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-resumed; err != nil {
				t.Fatal("effective founder was checked against stale transaction time", err)
			}
			assertResumeCommittedOnce(t, ctx, f)
		})
	}
}

func capturedFounderResumer(f paperBindingFixture) func(context.Context) error {
	principal := authorization.Principal{UserID: f.instance.UserID, Entitlement: authorization.EntitlementFounder}
	service := NewInstanceService(f.store, nil)
	return func(ctx context.Context) error {
		_, err := service.Resume(ctx, principal, f.instance.ID, f.instance.StateVersion, true)
		return err
	}
}

func assertDeniedResumeAccessUnchanged(t *testing.T, ctx context.Context, f paperBindingFixture) {
	t.Helper()
	assertResumeUnchanged(t, ctx, f)
	current, err := f.store.Get(ctx, f.instance.UserID, f.instance.ID)
	if err != nil || !reflect.DeepEqual(current, f.instance) {
		t.Fatal("refused Resume changed stored runtime identity, state, or timestamps", err)
	}
	assertCount(t, f.store.db, `SELECT count(*) FROM nonlive_strategy_schedules WHERE user_id='`+f.instance.UserID+`'`, 0)
	assertCount(t, f.store.db, `SELECT count(*) FROM nonlive_schedule_runs WHERE user_id='`+f.instance.UserID+`'`, 0)
	assertCount(t, f.store.db, `SELECT count(*) FROM automation_mandate_versions WHERE mandate_id='`+f.instance.AutomationMandateID+`'`, 1)
}
