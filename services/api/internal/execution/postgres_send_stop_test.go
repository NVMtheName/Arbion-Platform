package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Close only this test's breaker, including GLOBAL, so later isolated-database
// cases do not inherit a stop. Keep the historical breaker record intact.
func newSendStopID(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanup, `UPDATE risk_circuit_breakers SET state='CLOSED',released_at=clock_timestamp() WHERE id=$1 AND state='OPEN'`, id); err != nil {
			t.Error("close only the synthetic stop", err)
		}
	})
	return id
}

func TestPostgresSendConfirmedFirstStopAfterClaimDeniesCallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, scope := range []string{"GLOBAL", "USER", "ACCOUNT"} {
		t.Run(scope, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			id := newSendStopID(t, ctx, pool)
			var scopeID any
			if scope == "USER" {
				scopeID = f.order.Request.OwnerID
			} else if scope == "ACCOUNT" {
				scopeID = f.order.Request.AccountID
			}
			stopped := false
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n != 3 {
					return nil
				}
				// This separate read proves Claim committed before the first OPEN
				// row appears; the scope fence must protect previously absent rows.
				a, err := NewPostgresStore(pool).ReadAttempt(c, f.order.Request.OwnerID, f.order.ID)
				if err != nil || a.OrderID != f.order.ID {
					t.Error("stop did not follow a durable claim", err)
					return ErrInvalid
				}
				_, err = pool.Exec(c, `INSERT INTO risk_circuit_breakers(id,scope,scope_id,state,reason,source) VALUES($1,$2,$3,'OPEN','synthetic send boundary stop','SYSTEM')`, id, scope, scopeID)
				stopped = err == nil
				return err
			}}
			called := false
			a, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				return f.ack, nil
			}))
			if !errors.Is(err, ErrNotAuthorized) || !stopped || called || a.OrderID != f.order.ID {
				t.Fatal("committed stop crossed the final send boundary", err, stopped, called, a)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSendConfirmedFirstGlobalStopWaitsForCallbackAndAllowsCancellation(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	id := newSendStopID(t, ctx, pool)
	sendConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sendConn.Release()
	stopConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stopConn.Release()
	stopTx, err := stopConn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(stopTx)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	sent := make(chan error, 1)
	go func() {
		_, e := NewPostgresStore(sendConn).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
			close(entered)
			select {
			case <-release:
				return f.ack, nil
			case <-c.Done():
				return SubmissionAcknowledgement{}, c.Err()
			}
		}))
		sent <- e
	}()
	select {
	case <-entered:
	case e := <-sent:
		t.Fatal("send stopped before callback", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopped := make(chan error, 1)
	go func() {
		_, e := stopTx.Exec(ctx, `INSERT INTO risk_circuit_breakers(id,scope,state,reason,source) VALUES($1,'GLOBAL','OPEN','synthetic in-flight stop','SYSTEM')`, id)
		if e == nil {
			e = stopTx.Commit(ctx)
		}
		stopped <- e
	}()
	stopPID, sendPID := stopConn.Conn().PgConn().PID(), sendConn.Conn().PgConn().PID()
	waitExecutionLock(t, ctx, pool, stopPID)
	var advisoryWait, blockedBySend, visible bool
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted),$2::int=ANY(pg_blocking_pids($1)),EXISTS(SELECT 1 FROM risk_circuit_breakers WHERE id=$3)`, stopPID, sendPID, id).Scan(&advisoryWait, &blockedBySend, &visible)
	if err != nil || !advisoryWait || !blockedBySend || visible {
		t.Fatal("first stop did not wait on the synchronous send scope fence", err, advisoryWait, blockedBySend, visible)
	}
	select {
	case e := <-stopped:
		t.Fatal("stop committed before callback ended", e)
	default:
	}
	once.Do(func() { close(release) })
	if err = <-sent; err != nil {
		t.Fatal("later stop falsely retracted an admitted send", err)
	}
	if err = <-stopped; err != nil {
		t.Fatal("stop did not commit after the send guard ended", err)
	}
	if err = pool.QueryRow(ctx, `SELECT state='OPEN' FROM risk_circuit_breakers WHERE id=$1`, id).Scan(&visible); err != nil || !visible {
		t.Fatal("committed stop missing", err)
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	assertSendCannotRetry(t, ctx, pool, f)
	// The stop denies new risk, not the risk-reducing cancellation of this
	// known original order. An accepted cancel still is not fill finality.
	calls := 0
	a, err := NewPostgresStore(pool).CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, cancellationFunc(func(_ context.Context, _ *financial.Credentials, s ConfirmedSubmission, original Attempt) (CancellationAcknowledgement, error) {
		calls++
		if s.Order.ID != f.order.ID || original.ProviderOrderID != f.ack.ProviderOrderID {
			t.Error("stop cancellation changed original order identity")
		}
		return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
	}))
	if err != nil || calls != 1 || a.Outcome != "ACCEPTED" || a.ReceivedAt == nil {
		t.Fatal("stop blocked known-order cancellation", err, calls, a)
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	assertCancellationReplay(t, ctx, NewPostgresStore(pool), f, "ACCEPTED", nil)
}
