package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type lookupFunc func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error)

func (f lookupFunc) LookupSubmission(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (SubmissionAcknowledgement, error) {
	return f(c, cr, s, a)
}

func unknownSend(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) {
	t.Helper()
	_, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		return SubmissionAcknowledgement{}, errors.New("synthetic lost response")
	}))
	if !errors.Is(err, ErrSubmissionUnknown) {
		t.Fatal(err)
	}
}

func TestPostgresSubmissionRejectionIsDistinctImmutableAndHeld(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, code := range []string{"INSUFFICIENT_FUND", "secret provider message", "", strings.Repeat("A", 129)} {
		f := newSendFixture(t, ctx, pool, "BUY")
		s := NewPostgresStore(pool)
		_, err := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
			return SubmissionAcknowledgement{}, &SubmissionRejectedError{Code: code}
		}))
		want := ErrSubmissionUnknown
		if code == "INSUFFICIENT_FUND" {
			want = ErrSubmissionRejected
		}
		if !errors.Is(err, want) {
			t.Fatal(code, err)
		}
		r, err := s.ReadSubmissionRejection(ctx, f.order.Request.OwnerID, f.order.ID)
		if want == ErrSubmissionRejected {
			if err != nil || r.Code != code || r.OrderID != f.order.ID || r.ReceivedAt.IsZero() {
				t.Fatal(r, err)
			}
			if err = s.RecordAcknowledgement(ctx, f.order.Request.OwnerID, f.order.ID, f.ack.ProviderOrderID); !errors.Is(err, ErrConflict) {
				t.Fatal("contradictory ack allowed", err)
			}
			for _, q := range []string{`UPDATE execution_submission_rejections SET error_code='CHANGED' WHERE order_id=$1`, `DELETE FROM execution_submission_rejections WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, q, f.order.ID); err == nil {
					t.Fatal("mutable rejection")
				}
			}
			called := false
			_, err = s.RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
				called = true
				return f.ack, nil
			}))
			if !errors.Is(err, ErrSubmissionRejected) || called {
				t.Fatal("reported rejection lost", err, called)
			}
		} else if !errors.Is(err, ErrNotFound) {
			t.Fatal("malformed code saved", err)
		}
		if _, err = s.ReadSubmissionRejection(ctx, f.order.Request.AccountID, f.order.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("cross-owner receipt", err)
		}
		assertSendHeld(t, ctx, pool, f.order, "")
		assertSendCannotRetry(t, ctx, pool, f)
	}
}

func TestPostgresRecoveryRetainsIdentityAfterRevocationAndStops(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	unknownSend(t, ctx, pool, f)
	s := NewPostgresStore(pool)
	if err := s.RevokeOwnerApproval(ctx, f.order.Request.OwnerID, f.order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('USER',$1,'OPEN','fixture','SYSTEM')`, f.order.Request.OwnerID); err != nil {
		t.Fatal(err)
	}
	// A replacement key for the SAME portfolio is usable for read recovery, not
	// for replaying the old claim's send authorization.
	if _, err := pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('77',32),'hex') WHERE id=$1`, f.order.Request.ConnectionID); err != nil {
		t.Fatal(err)
	}
	f.vault.generation++
	lookup := lookupFunc(func(_ context.Context, cr *financial.Credentials, sub ConfirmedSubmission, a Attempt) (SubmissionAcknowledgement, error) {
		if sub.Order.Request != f.order.Request || sub.PreviewID != f.preflight.PreviewID || sub.PortfolioID != cr.PortfolioID || a.CredentialGeneration != f.approval.CredentialGeneration {
			return SubmissionAcknowledgement{}, ErrConflict
		}
		return f.ack, nil
	})
	// Two restarted workers may read concurrently, but can only persist the
	// same acknowledgement. Neither has a sender or changes either hold.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := NewPostgresStore(pool).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookup)
			if err == nil && a.ProviderOrderID != f.ack.ProviderOrderID {
				err = ErrConflict
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
		t.Error("known ack caused provider request")
		return SubmissionAcknowledgement{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	assertSendCannotRetry(t, ctx, pool, f)
	if _, err = pool.Exec(ctx, `INSERT INTO execution_submission_rejections(order_id,owner_id,error_code) VALUES($1,$2,'FAILED')`, f.order.ID, f.order.Request.OwnerID); err == nil {
		t.Fatal("rejection contradicted ack")
	}
}

func TestPostgresRecoveryRequiresCurrentAccessBeforeAndAfterLookup(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	mutations := map[string]string{
		"owner":              `UPDATE users SET status='disabled' WHERE id=$1`,
		"entitlement":        `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`,
		"connection":         `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"connection expired": `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1`,
		"portfolio":          `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE user_id=$1`,
		"credential":         `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('66',32),'hex') WHERE user_id=$1`,
	}
	for name, q := range mutations {
		for _, after := range []bool{false, true} {
			t.Run(name+map[bool]string{false: " before", true: " after"}[after], func(t *testing.T) {
				f := newSendFixture(t, ctx, pool, "BUY")
				unknownSend(t, ctx, pool, f)
				mutate := func() {
					if _, err := pool.Exec(ctx, q, f.order.Request.OwnerID); err != nil {
						t.Fatal(err)
					}
				}
				if !after {
					mutate()
				}
				called := false
				_, err := NewPostgresStore(pool).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
					called = true
					mutate()
					return f.ack, nil
				}))
				if !errors.Is(err, ErrNotAuthorized) || called != after {
					t.Fatal("stale access accepted", err, called)
				}
				assertSendHeld(t, ctx, pool, f.order, "")
			})
		}
	}
}

func TestPostgresRecoveryUnknownWrongIdentityAndLostCommit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, mode := range []string{"not found", "wrong client", "lost commit"} {
		t.Run(mode, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "SELL")
			unknownSend(t, ctx, pool, f)
			var db Database = pool
			if mode == "lost commit" {
				db = lostCommitDB{pool}
			}
			_, err := NewPostgresStore(db).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
				if mode == "not found" {
					return SubmissionAcknowledgement{}, ErrNotFound
				}
				ack := f.ack
				if mode == "wrong client" {
					ack.ClientOrderID = f.order.Request.OwnerID
				}
				return ack, nil
			}))
			if !errors.Is(err, ErrSubmissionUnknown) {
				t.Fatal(err)
			}
			provider := ""
			if mode == "lost commit" {
				provider = f.ack.ProviderOrderID
			}
			assertSendHeld(t, ctx, pool, f.order, provider)
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSubmissionReceiptsSerializeAndLostRejectionCommitRecovers(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
		if n == 3 {
			return lostCommitTx{tx}
		}
		return tx
	}}
	_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		return SubmissionAcknowledgement{}, &SubmissionRejectedError{Code: "INSUFFICIENT_FUND"}
	}))
	if !errors.Is(err, ErrSubmissionUnknown) {
		t.Fatal(err)
	}
	if r, err := NewPostgresStore(pool).ReadSubmissionRejection(ctx, f.order.Request.OwnerID, f.order.ID); err != nil || r.Code != "INSUFFICIENT_FUND" {
		t.Fatal(r, err)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
	assertSendCannotRetry(t, ctx, pool, f)

	// Both direct SQL writers must serialize, not permit contradictory evidence
	// after waiting on a stale statement snapshot.
	f = newSendFixture(t, ctx, pool, "BUY")
	unknownSend(t, ctx, pool, f)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `INSERT INTO execution_submission_rejections(order_id,owner_id,error_code) VALUES($1,$2,'REJECTED')`, f.order.ID, f.order.Request.OwnerID); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var pid uint32
	if err = conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := conn.Exec(ctx, `INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id) VALUES($1,$2,$3)`, f.order.ID, f.order.Request.OwnerID, f.ack.ProviderOrderID)
		result <- e
	}()
	waitExecutionLock(t, ctx, pool, pid)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(mapError(err), ErrConflict) {
		t.Fatal("contradictory concurrent ack accepted", err)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
}

func TestPostgresRecoveryRechecksAccessExpiryAfterReceiptWait(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	unknownSend(t, ctx, pool, f)
	var expiry time.Time
	if err := pool.QueryRow(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING authorization_expires_at`, f.order.Request.ConnectionID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(holder)
	var id string
	if err = holder.QueryRow(ctx, `SELECT order_id::text FROM execution_dispatch_attempts WHERE order_id=$1 FOR UPDATE`, f.order.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var pid uint32
	if err = conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := NewPostgresStore(conn).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
			return f.ack, nil
		}))
		result <- e
	}()
	waitExecutionLock(t, ctx, pool, pid)
	time.Sleep(time.Until(expiry) + 20*time.Millisecond)
	if err = holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("expired access accepted after receipt wait", err)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
}
