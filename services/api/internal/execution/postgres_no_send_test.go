package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func assertNoSendClosed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) NoSendResolution {
	t.Helper()
	s := NewPostgresStore(pool)
	r, err := s.ReadNoSendResolution(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil {
		t.Fatal("missing durable no-send proof", err)
	}
	a, err := s.ReadAttempt(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || a.ProviderOrderID != "" || r.OrderID != f.order.ID || r.OwnerID != f.order.Request.OwnerID || r.AccountID != f.order.Request.AccountID || r.CapitalBucketID != f.order.Request.CapitalBucketID || r.AuthorizationID != a.AuthorizationID || r.RequestDigest != f.order.RequestDigest || r.CredentialGeneration != a.CredentialGeneration || !r.ClaimedAt.Equal(a.ClaimedAt) || r.RecordedAt.Before(r.ClaimedAt) {
		t.Fatal("no-send proof changed immutable claim identity", err, r, a)
	}
	reservation, err := s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
	kind, asset, quantity := "CASH", "USD", f.order.Request.MaximumDebitUSD
	if f.order.Request.Side == "SELL" {
		kind, asset, quantity = "ASSET", strings.TrimSuffix(f.order.Request.ProductID, "-USD"), f.order.Request.BaseSize
	}
	if err != nil || reservation.OrderID != r.OrderID || reservation.AccountID != r.AccountID || reservation.CapitalBucketID != r.CapitalBucketID || reservation.ResourceType != kind || reservation.Asset != asset || !sameAmount(reservation.Quantity, quantity) || reservation.ReleasedAt == nil || !reservation.ReleasedAt.Equal(r.RecordedAt) {
		t.Fatal("no-send resolution changed or retained the exact capital fence", err, reservation)
	}
	var held, brokerFacts bool
	var receipts int
	err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_account_holds WHERE order_id=$1),
	 EXISTS(SELECT 1 FROM execution_broker_acknowledgements WHERE order_id=$1 UNION ALL SELECT 1 FROM execution_submission_rejections WHERE order_id=$1 UNION ALL SELECT 1 FROM execution_fills WHERE order_id=$1 UNION ALL SELECT 1 FROM execution_order_terminals WHERE order_id=$1 UNION ALL SELECT 1 FROM execution_cancellation_attempts WHERE order_id=$1 UNION ALL SELECT 1 FROM execution_account_settlements WHERE order_id=$1),
	 (SELECT count(*) FROM execution_no_send_resolutions WHERE order_id=$1)`, f.order.ID).Scan(&held, &brokerFacts, &receipts)
	if err != nil || held || brokerFacts || receipts != 1 {
		t.Fatal("no-send closure fabricated broker evidence or did not close once", err, held, brokerFacts, receipts)
	}
	if _, err = s.ReadNoSendResolution(ctx, f.order.Request.AccountID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-owner no-send receipt exposed", err)
	}
	// Recovery consumes positive local evidence, without credentials, broker
	// queries, or a newly issued Claim. An absence-only restart cannot do this.
	vault := fixturePreflightVault{retrieve: func(context.Context, credential.Locator) ([]byte, error) {
		t.Error("known no-send recovery read credentials")
		return nil, ErrInvalid
	}}
	_, err = NewPostgresStore(pool).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
		t.Error("known no-send recovery contacted provider")
		return f.ack, nil
	}))
	if !errors.Is(err, ErrSubmissionNotSent) {
		t.Fatal("restart lost positive no-send evidence", err)
	}
	assertSendCannotRetry(t, ctx, pool, f)
	return r
}

func TestPostgresNoSendExactWinningClaimReleaseAndImmutableHistory(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, side)
			denied := errors.New("synthetic final guard unavailable")
			var before CapitalReservation
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n != 3 {
					return nil
				}
				var err error
				before, err = NewPostgresStore(pool).ReadCapitalReservation(c, f.order.Request.OwnerID, f.order.ID)
				if err != nil {
					return err
				}
				return denied
			}}
			_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				t.Error("denied invocation entered sender")
				return f.ack, nil
			}))
			if !errors.Is(err, denied) || !errors.Is(err, ErrSubmissionNotSent) || errors.Is(err, ErrNoSendResolutionUnknown) {
				t.Fatal("no-send closure lost original denial", err)
			}
			proof := assertNoSendClosed(t, ctx, pool, f)
			after, err := NewPostgresStore(pool).ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
			after.ReleasedAt = nil
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("closure erased or rewrote original reservation", err)
			}
			if again, err := NewPostgresStore(pool).ReadNoSendResolution(ctx, f.order.Request.OwnerID, f.order.ID); err != nil || !reflect.DeepEqual(again, proof) {
				t.Fatal("restart changed immutable no-send receipt", err)
			}
			for _, query := range []string{
				`UPDATE execution_no_send_resolutions SET request_digest='changed' WHERE order_id=$1`,
				`DELETE FROM execution_no_send_resolutions WHERE order_id=$1`,
				`INSERT INTO execution_broker_acknowledgements(order_id,owner_id,provider_order_id) SELECT order_id,owner_id,gen_random_uuid() FROM execution_no_send_resolutions WHERE order_id=$1`,
				`INSERT INTO execution_submission_rejections(order_id,owner_id,error_code) SELECT order_id,owner_id,'REJECTED' FROM execution_no_send_resolutions WHERE order_id=$1`,
				`INSERT INTO execution_account_holds(order_id,owner_id,financial_account_id) SELECT order_id,owner_id,financial_account_id FROM execution_no_send_resolutions WHERE order_id=$1`,
				`UPDATE execution_capital_reservations SET released_at=NULL WHERE order_id=$1`,
			} {
				if _, err := pool.Exec(ctx, query, f.order.ID); err == nil {
					t.Fatal("SQL changed or contradicted positive no-send evidence")
				}
			}
		})
	}
}

type failedNoSendCommitTx struct{ pgx.Tx }

func (tx failedNoSendCommitTx) Commit(ctx context.Context) error {
	_ = tx.Tx.Rollback(ctx)
	return errors.New("synthetic closure commit failure")
}

func TestPostgresNoSendClosureCommitFailureVersusResponseLoss(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit response lost"}[committed], func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			denied := errors.New("synthetic pre-callback failure")
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(_ context.Context, n int) error {
				if n == 3 {
					return denied
				}
				return nil
			}, wrap: func(n int, tx pgx.Tx) pgx.Tx {
				if n == 4 {
					if committed {
						return lostCommitTx{tx}
					}
					return failedNoSendCommitTx{tx}
				}
				return tx
			}}
			_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				t.Error("closure uncertainty entered sender")
				return f.ack, nil
			}))
			if !errors.Is(err, denied) || !errors.Is(err, ErrNoSendResolutionUnknown) || errors.Is(err, ErrSubmissionNotSent) {
				t.Fatal("ambiguous closure reported certainty or erased denial", err)
			}
			if committed {
				assertNoSendClosed(t, ctx, pool, f)
			} else {
				assertSendHeld(t, ctx, pool, f.order, "")
				assertSendCannotRetry(t, ctx, pool, f)
			}
		})
	}
}

func TestPostgresNoSendPositiveClaimSurvivesCallerCancellation(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	caller, cancel := context.WithCancel(ctx)
	defer cancel()
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(_ context.Context, n int) error {
		if n == 3 {
			cancel()
			return caller.Err()
		}
		return nil
	}}
	_, err := NewPostgresStore(db).SendConfirmed(caller, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		t.Error("canceled caller entered sender")
		return f.ack, nil
	}))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrSubmissionNotSent) {
		t.Fatal("caller cancellation erased positive no-send evidence", err)
	}
	assertNoSendClosed(t, ctx, pool, f)
}

func TestPostgresNoSendConcurrentOriginalCallsCloseOnce(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	var calls atomic.Int32
	start, results := make(chan struct{}), make(chan error, 4)
	for i := 0; i < cap(results); i++ {
		go func() {
			<-start
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(_ context.Context, n int) error {
				if n == 3 {
					return ErrNotAuthorized
				}
				return nil
			}}
			_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				calls.Add(1)
				return f.ack, nil
			}))
			results <- err
		}()
	}
	close(start)
	closed := 0
	for i := 0; i < cap(results); i++ {
		err := <-results
		if errors.Is(err, ErrSubmissionNotSent) && errors.Is(err, ErrNotAuthorized) {
			closed++
		} else if !errors.Is(err, ErrAlreadyAttempted) {
			t.Fatal("unexpected concurrent no-send outcome", err)
		}
	}
	if closed != 1 || calls.Load() != 0 {
		t.Fatal("multiple closure winners or a sender entered", closed, calls.Load())
	}
	assertNoSendClosed(t, ctx, pool, f)
}

func TestPostgresNoSendRestartCannotInferProofFromUnusedClaim(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	s := NewPostgresStore(pool)
	// Simulate a process crash after a genuine positively committed claim but
	// before the winning invocation can establish or record no-send evidence.
	if _, err := s.Claim(ctx, f.order.Request.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(f.evidence))); err != nil {
		t.Fatal(err)
	}
	assertSendCannotRetry(t, ctx, pool, f)
	called := false
	_, err := NewPostgresStore(pool).RecoverSubmission(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, lookupFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (SubmissionAcknowledgement, error) {
		called = true
		return SubmissionAcknowledgement{}, nil // No broker history is not proof.
	}))
	if !errors.Is(err, ErrSubmissionUnknown) || !called {
		t.Fatal("restart inferred non-submission from missing evidence", err)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
}

type uncertainNoSendRollbackTx struct {
	pgx.Tx
	observed *bool
}

func (tx uncertainNoSendRollbackTx) Rollback(ctx context.Context) error {
	*tx.observed = true
	_ = tx.Tx.Rollback(ctx) // Clean up the fixture, but report no confirmed cleanup.
	return errors.New("synthetic final transaction rollback uncertainty")
}

func TestPostgresNoSendUncertainFinalRollbackNeverStartsClosure(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	rolledBack, called := false, false
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
		if n == 3 {
			return NewPostgresStore(pool).RevokeOwnerApproval(c, f.order.Request.OwnerID, f.order.ID)
		}
		return nil
	}, wrap: func(n int, tx pgx.Tx) pgx.Tx {
		if n == 3 {
			return uncertainNoSendRollbackTx{tx, &rolledBack}
		}
		return tx
	}}
	_, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	}))
	if !errors.Is(err, ErrNotAuthorized) || !errors.Is(err, ErrNoSendResolutionUnknown) || errors.Is(err, ErrSubmissionNotSent) || called || !rolledBack || db.begins != 3 {
		t.Fatal("uncertain final cleanup manufactured no-send proof", err, called, rolledBack, db.begins)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
	assertSendCannotRetry(t, ctx, pool, f)
}

type panicNoSendGuardTx struct {
	pgx.Tx
	reason string
}

func (tx panicNoSendGuardTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic(tx.reason)
}

func TestPostgresNoSendPanicNeverCreatesProof(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, beforeSender := range []bool{true, false} {
		t.Run(map[bool]string{true: "final guard", false: "inside sender"}[beforeSender], func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			const reason = "synthetic execution panic"
			db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
				if n == 3 && beforeSender {
					return panicNoSendGuardTx{tx, reason}
				}
				return tx
			}}
			called := false
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				_, _ = NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
					called = true
					panic(reason)
				}))
			}()
			if recovered != reason || called == beforeSender || db.begins != 3 {
				t.Fatal("panic boundary did not retain uncertain original claim", recovered, called, db.begins)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}
