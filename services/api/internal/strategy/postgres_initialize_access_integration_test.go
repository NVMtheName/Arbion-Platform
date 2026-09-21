package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A captured request principal is not transactionally current authority. These
// tests exercise real InstanceService/automation/persistence paths against legal
// stored states. Revocation writes are synthetic defense-in-depth fixtures, not
// a claim that the public administrative service allows founder downgrades.
func testInitializeCurrentAccess(t *testing.T, parent context.Context, pool *pgxpool.Pool) {
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		for _, tc := range []struct{ name, query string }{
			{"disabled owner", `UPDATE users SET status='disabled' WHERE id=$1`},
			{"unverified owner", `UPDATE users SET status='pending_verification' WHERE id=$1`},
			{"revoked founder", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`},
			{"deleted founder", `DELETE FROM user_entitlements WHERE user_id=$1 AND entitlement_key='founder'`},
			{"future founder", `UPDATE user_entitlements SET starts_at=clock_timestamp()+interval '1 hour' WHERE user_id=$1 AND entitlement_key='founder'`},
		} {
			t.Run(string(mode)+" captured principal cannot bypass "+tc.name, func(t *testing.T) {
				f := newInitializeConnectionFixture(t, parent, pool, mode)
				initialize := capturedFounderInitializer(f)
				if _, err := pool.Exec(parent, tc.query, f.userID); err != nil {
					t.Fatal(err)
				}
				if err := initialize(parent); !errors.Is(err, ErrForbidden) {
					t.Error("stale captured founder initialized an engine", err)
				}
				f.assertArtifacts(t, 0)
			})
		}
		for _, tc := range []struct{ name, query string }{
			{"owner", `UPDATE users SET status='disabled' WHERE id=$1`},
			{"founder", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1 AND entitlement_key='founder'`},
		} {
			t.Run(string(mode)+" "+tc.name+" revocation wins initialization wait", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newInitializeConnectionFixture(t, ctx, pool, mode)
				initialize := capturedFounderInitializer(f)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, tc.query, f.userID); err != nil {
					t.Fatal(err)
				}
				initialized := start(func() error { return initialize(ctx) })
				blocked, initializeErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), initialized)
				if !blocked {
					t.Error("initialization passed the unfinished authority writer", initializeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if blocked {
					initializeErr = <-initialized
				}
				if !errors.Is(initializeErr, ErrForbidden) {
					t.Error("initialization did not refuse the current revoked authority", initializeErr)
				}
				f.assertArtifacts(t, 0)
			})
			t.Run(string(mode)+" initialized authority defers "+tc.name+" revocation", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				f := newInitializeConnectionFixture(t, ctx, pool, mode)
				initialize := capturedFounderInitializer(f)
				gate, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer gate.Rollback(ctx)
				if _, err = gate.Exec(ctx, `LOCK TABLE strategy_state_transitions IN SHARE MODE`); err != nil {
					t.Fatal(err)
				}
				initialized := start(func() error { return initialize(ctx) })
				waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
				initializerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
				revoked := start(func() error {
					_, err := pool.Exec(ctx, tc.query, f.userID)
					return err
				})
				blocked, revokeErr := waitForResumeLockOrCompletion(t, ctx, pool, initializerPID, revoked)
				if !blocked {
					t.Error("revocation committed while initialization was still unfinished", revokeErr)
				}
				if err = gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				initializeErr := <-initialized
				if blocked {
					revokeErr = <-revoked
				}
				if initializeErr != nil || revokeErr != nil {
					t.Fatal("ordered initialization and later revocation did not finish", initializeErr, revokeErr)
				}
				// The engine was initialized before revocation. Later execution
				// still requires its independent current-access commit guard.
				f.assertArtifacts(t, 1)
			})
		}
		t.Run(string(mode)+" founder grant effective after transaction start is current after wait", func(t *testing.T) {
			ctx, start := initializeRaceWorkers(t, parent)
			f := newInitializeConnectionFixture(t, ctx, pool, mode)
			initialize := capturedFounderInitializer(f)
			gate, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Rollback(ctx)
			if _, err = gate.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, f.userID); err != nil {
				t.Fatal(err)
			}
			initialized := start(func() error { return initialize(ctx) })
			waitForPaperBindingLock(t, ctx, pool, gate.Conn().PgConn().PID())
			initializerPID := resumeBlockedPID(t, ctx, pool, gate.Conn().PgConn().PID())
			var transactionStartedAt, startsAt time.Time
			if err = pool.QueryRow(ctx, `SELECT xact_start FROM pg_stat_activity WHERE pid=$1`, int32(initializerPID)).Scan(&transactionStartedAt); err != nil {
				t.Fatal("could not observe blocked initialization transaction start", err)
			}
			// Permanent founder grants cannot have expires_at by schema. Do
			// not weaken that constraint to fabricate an expiry fixture. A
			// grant made effective after the observed transaction start proves
			// the post-wait wall-clock rule without a scheduling-sensitive delay.
			if err = gate.QueryRow(ctx, `UPDATE user_entitlements SET starts_at=clock_timestamp() WHERE user_id=$1 AND entitlement_key='founder' RETURNING starts_at`, f.userID).Scan(&startsAt); err != nil || !startsAt.After(transactionStartedAt) {
				t.Fatal("fixture did not place effective grant after transaction start", err)
			}
			if err = gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-initialized; err != nil {
				t.Fatal("currently effective founder was checked against stale transaction time", err)
			}
			f.assertArtifacts(t, 1)
		})
	}
}

func capturedFounderInitializer(f initializeConnectionFixture) func(context.Context) error {
	principal := authorization.Principal{UserID: f.userID, Entitlement: authorization.EntitlementFounder}
	service := NewInstanceService(NewPostgresStore(f.pool), automation.NewService(automation.NewPostgresStore(f.pool), nil))
	return func(ctx context.Context) error {
		_, err := service.Initialize(ctx, principal, f.mandate.ID, "1000")
		return err
	}
}
