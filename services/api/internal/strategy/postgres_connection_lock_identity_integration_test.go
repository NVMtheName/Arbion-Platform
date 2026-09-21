package strategy

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/connectionguard"
	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testConnectionLockIdentity(t *testing.T, parent context.Context, pool *pgxpool.Pool) {
	stores := []struct {
		name string
		lock func(context.Context, string, func() error) error
	}{
		{"financial", financialconnection.NewPostgresStore(pool).WithLock},
		{"AI", aiconnection.NewPostgresStore(pool, aiconnection.DefaultRegistry()).WithLock},
	}
	for _, store := range stores {
		for _, spelling := range connectionLockUUIDSpellings("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11") {
			t.Run(store.name+" "+spelling.name+" shares canonical lock", func(t *testing.T) {
				ctx, start := initializeRaceWorkers(t, parent)
				canonical := confirmConnectionLockUUID(t, ctx, pool, spelling.id)
				gate := holdConnectionIdentityLock(t, ctx, pool, canonical)
				defer gate.Rollback(ctx)
				var callbacks atomic.Int32
				done := start(func() error {
					return store.lock(ctx, spelling.id, func() error { callbacks.Add(1); return nil })
				})
				blocked, lockErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), done)
				if !blocked || callbacks.Load() != 0 {
					t.Error("equivalent UUID entered callback before canonical lock was released", lockErr)
				}
				if err := gate.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if blocked {
					lockErr = <-done
				}
				if lockErr != nil || callbacks.Load() != 1 {
					t.Fatal("serialized callback did not complete exactly once", lockErr, callbacks.Load())
				}
			})
		}
		t.Run(store.name+" distinct UUID does not contend", func(t *testing.T) {
			ctx, start := initializeRaceWorkers(t, parent)
			gate := holdConnectionIdentityLock(t, ctx, pool, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
			defer gate.Rollback(ctx)
			var callbacks atomic.Int32
			done := start(func() error {
				return store.lock(ctx, "{A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A12}", func() error { callbacks.Add(1); return nil })
			})
			blocked, lockErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), done)
			if blocked {
				t.Error("different UUID waited for unrelated connection")
			}
			if err := gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if blocked {
				lockErr = <-done
			}
			if lockErr != nil || callbacks.Load() != 1 {
				t.Fatal("independent callback did not complete exactly once", lockErr, callbacks.Load())
			}
		})
		for _, cancelCallback := range []bool{false, true} {
			name := "callback error releases lock"
			if cancelCallback {
				name = "callback cancellation releases lock"
			}
			t.Run(store.name+" "+name, func(t *testing.T) {
				// Pin an independent session before the operation. A probe on
				// the original session could reacquire its own leaked lock.
				probe, err := pool.Begin(parent)
				if err != nil {
					t.Fatal(err)
				}
				defer probe.Rollback(parent)
				ctx, cancel := context.WithCancel(parent)
				defer cancel()
				wantErr := errors.New("synthetic callback failure")
				callbacks := 0
				err = store.lock(ctx, "{A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11}", func() error {
					callbacks++
					if cancelCallback {
						cancel()
						wantErr = context.Canceled
					}
					return wantErr
				})
				if !errors.Is(err, wantErr) || callbacks != 1 {
					t.Fatal("callback outcome was not preserved", err, callbacks)
				}
				assertConnectionIdentityReleased(t, parent, probe, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
			})
		}
	}
	t.Run("financial composite key stays byte-identical", func(t *testing.T) {
		ctx, start := initializeRaceWorkers(t, parent)
		// Uppercase within this non-UUID domain must not be normalized.
		key := "portfolio-reconciliation:A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11:a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
		gate := holdConnectionIdentityLock(t, ctx, pool, key)
		defer gate.Rollback(ctx)
		probe, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer probe.Rollback(ctx)
		var callbacks atomic.Int32
		done := start(func() error {
			return stores[0].lock(ctx, key, func() error { callbacks.Add(1); return nil })
		})
		blocked, lockErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), done)
		if !blocked || callbacks.Load() != 0 {
			t.Error("financial composite key identity changed", lockErr)
		}
		if err := gate.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if blocked {
			lockErr = <-done
		}
		if lockErr != nil || callbacks.Load() != 1 {
			t.Fatal("composite-key callback did not complete exactly once", lockErr, callbacks.Load())
		}
		assertConnectionIdentityReleased(t, ctx, probe, key)
	})
	for _, variant := range []struct {
		name  string
		index int
	}{{"uppercase", 1}, {"braced mixed hyphens", 5}} {
		t.Run("guard AI spelling "+variant.name+" waits and rechecks", func(t *testing.T) {
			ctx, start := initializeRaceWorkers(t, parent)
			f := newInitializeConnectionFixture(t, ctx, pool, Shadow)
			spelling := connectionLockUUIDSpellings(f.aiID)[variant.index].id
			if canonical := confirmConnectionLockUUID(t, ctx, pool, spelling); canonical != f.aiID {
				t.Fatal("fixture UUID spelling changed identity", canonical)
			}
			gate := holdConnectionIdentityLock(t, ctx, pool, f.aiID)
			defer gate.Rollback(ctx)
			done := start(func() error {
				tx, err := pool.Begin(ctx)
				if err != nil {
					return err
				}
				defer tx.Rollback(ctx)
				return connectionguard.LockActive(ctx, tx, f.userID, f.accountID, &spelling)
			})
			blocked, guardErr := waitForResumeLockOrCompletion(t, ctx, pool, gate.Conn().PgConn().PID(), done)
			if !blocked {
				t.Error("guard passed canonical AI lifecycle lock using another UUID spelling", guardErr)
			}
			if _, err := gate.Exec(ctx, `UPDATE provider_connections SET status='disabled' WHERE id=$1 AND user_id=$2 AND provider_category='ai'`, f.aiID, f.userID); err != nil {
				t.Fatal(err)
			}
			if err := gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if blocked {
				guardErr = <-done
			}
			if !errors.Is(guardErr, connectionguard.ErrUnavailable) {
				t.Error("guard accepted provider disabled while it waited", guardErr)
			}
			f.assertArtifacts(t, 0)
		})
	}
}

func connectionLockUUIDSpellings(canonical string) []struct{ name, id string } {
	compact := strings.ReplaceAll(canonical, "-", "")
	groups := make([]string, 0, 8)
	for i := 0; i < len(compact); i += 4 {
		groups = append(groups, compact[i:i+4])
	}
	return []struct{ name, id string }{
		{"canonical", canonical},
		{"uppercase", strings.ToUpper(canonical)},
		{"braces", "{" + canonical + "}"},
		{"hyphenless", compact},
		{"four-digit grouping", strings.Join(groups, "-")},
		{"braced mixed hyphens", "{" + strings.ToUpper(compact[:8]+"-"+compact[8:16]+"-"+compact[16:]) + "}"},
	}
}

func confirmConnectionLockUUID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, spelling string) string {
	t.Helper()
	var canonical string
	// Parameter stays text so PostgreSQL, not a narrower client UUID parser,
	// establishes which spellings address the same persisted identity.
	if err := pool.QueryRow(ctx, `SELECT $1::text::uuid::text`, spelling).Scan(&canonical); err != nil {
		t.Fatal("PostgreSQL rejected test UUID spelling", spelling, err)
	}
	if strings.ReplaceAll(canonical, "-", "") != strings.ToLower(strings.NewReplacer("-", "", "{", "", "}", "").Replace(spelling)) {
		t.Fatal("test spelling did not preserve UUID identity", spelling, canonical)
	}
	return canonical
}

func holdConnectionIdentityLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key string) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text,0))`, key); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	return tx
}

func assertConnectionIdentityReleased(t *testing.T, parent context.Context, probe pgx.Tx, key string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	for {
		var acquired bool
		if err := probe.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1::text,0))`, key).Scan(&acquired); err != nil {
			t.Fatal("could not check advisory lock release", err)
		}
		if acquired {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("callback left its advisory lock held")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
