package execution

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
)

type cancellationFunc func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error)

func (f cancellationFunc) CancelOnce(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (CancellationAcknowledgement, error) {
	return f(c, cr, s, a)
}

func assertCancellationReplay(t *testing.T, ctx context.Context, s *PostgresStore, f sendFixture, outcome string, want error) {
	t.Helper()
	vault := fixturePreflightVault{retrieve: func(context.Context, credential.Locator) ([]byte, error) {
		t.Error("cancellation replay reread credentials")
		return nil, ErrInvalid
	}}
	got, err := s.CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
		t.Error("cancellation replay called provider")
		return CancellationAcknowledgement{}, ErrInvalid
	}))
	if !errors.Is(err, want) || got.Outcome != outcome || got.ProviderOrderID != f.ack.ProviderOrderID || got.OrderID != f.order.ID {
		t.Fatal("incorrect saved cancellation replay", err, got)
	}
}

func TestPostgresCancellationExactOutcomeIsOnceOnlyAndNeverSettlement(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, mode := range []string{"accepted", "not accepted", "network unknown", "wrong identity"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := newPostgresObservationFixture(t, ctx, pool)
			s := NewPostgresStore(pool)
			// Risk-reducing cancellation must remain possible after a stop and
			// revocation of the original order's send approval.
			if err := s.RevokeOwnerApproval(ctx, f.order.Request.OwnerID, f.order.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('USER',$1,'OPEN','cancellation fixture','SYSTEM')`, f.order.Request.OwnerID); err != nil {
				t.Fatal(err)
			}
			calls := 0
			got, err := s.CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, cancellationFunc(func(c context.Context, cr *financial.Credentials, sub ConfirmedSubmission, a Attempt) (CancellationAcknowledgement, error) {
				calls++
				deadline, ok := c.Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
					t.Error("unbounded cancellation deadline")
				}
				if !reflect.DeepEqual(sub.Order, f.order) || sub.PortfolioID != f.preflight.PortfolioID || cr.PortfolioID != sub.PortfolioID || a.ProviderOrderID != f.ack.ProviderOrderID {
					t.Error("cancellation changed original identity")
				}
				saved, e := NewPostgresStore(pool).ReadCancellation(ctx, f.order.Request.OwnerID, f.order.ID)
				if e != nil || saved.Outcome != "UNKNOWN" {
					t.Error("callback entered before durable cancellation claim", e)
				}
				assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
				ack := CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: mode != "not accepted"}
				if mode == "network unknown" {
					return ack, errors.New("synthetic private provider message")
				}
				if mode == "wrong identity" {
					ack.ProviderOrderID = f.order.ID
				}
				return ack, nil
			}))
			outcome, want := "ACCEPTED", error(nil)
			if mode == "not accepted" {
				outcome, want = "NOT_ACCEPTED", ErrCancellationNotAccepted
			} else if mode != "accepted" {
				outcome, want = "UNKNOWN", ErrCancellationUnknown
			}
			if !errors.Is(err, want) || calls != 1 || got.Outcome != outcome {
				t.Fatal("incorrect cancellation outcome", err, calls, got)
			}
			saved, err := s.ReadCancellation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil || saved.Outcome != outcome || saved.ReceivedAt == nil {
				t.Fatal("incorrect immutable cancellation receipt", err, saved)
			}
			assertCancellationReplay(t, ctx, NewPostgresStore(pool), f, outcome, want)
			assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			for _, q := range []string{`UPDATE execution_cancellation_attempts SET expires_at=expires_at+interval '1 second' WHERE order_id=$1`, `DELETE FROM execution_cancellation_attempts WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, q, f.order.ID); err == nil {
					t.Fatal("cancellation attempt mutated")
				}
			}
			for _, q := range []string{`UPDATE execution_cancellation_receipts SET outcome='UNKNOWN' WHERE order_id=$1`, `DELETE FROM execution_cancellation_receipts WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, q, f.order.ID); err == nil {
					t.Fatal("cancellation receipt mutated")
				}
			}
		})
	}
}

func TestPostgresCancellationConcurrencyAndCommitUncertainty(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	t.Run("concurrent workers", func(t *testing.T) {
		f, _ := newPostgresObservationFixture(t, ctx, pool)
		var calls atomic.Int32
		var wg sync.WaitGroup
		results := make(chan error, 6)
		sender := cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
			calls.Add(1)
			return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
		})
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := NewPostgresStore(pool).CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, sender)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil && !errors.Is(err, ErrCancellationUnknown) {
				t.Fatal("unexpected concurrent outcome", err)
			}
		}
		if calls.Load() != 1 {
			t.Fatal("multiple cancellation calls", calls.Load())
		}
		assertCancellationReplay(t, ctx, NewPostgresStore(pool), f, "ACCEPTED", nil)
		assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	})
	for _, phase := range []int{2, 3} {
		t.Run(map[int]string{2: "claim commit unknown", 3: "receipt commit unknown"}[phase], func(t *testing.T) {
			f, _ := newPostgresObservationFixture(t, ctx, pool)
			calls := 0
			db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
				if n == phase {
					return lostCommitTx{tx}
				}
				return tx
			}}
			_, err := NewPostgresStore(db).CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
				calls++
				return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
			}))
			want, outcome, replayErr, wantCalls := ErrCommitUnknown, "UNKNOWN", error(ErrCancellationUnknown), 0
			if phase == 3 {
				want, outcome, replayErr, wantCalls = ErrCancellationUnknown, "ACCEPTED", nil, 1
			}
			if !errors.Is(err, want) || calls != wantCalls {
				t.Fatal("commit uncertainty allowed repeat or unclaimed cancellation", err, calls)
			}
			assertCancellationReplay(t, ctx, NewPostgresStore(pool), f, outcome, replayErr)
			assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
		})
	}
}

func TestPostgresCancellationRequiresExactCurrentAccess(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, q := range map[string]string{
		"owner":      `UPDATE users SET status='disabled' WHERE id=$1`,
		"connection": `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"key":        `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('66',32),'hex') WHERE user_id=$1`,
		"portfolio":  `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE user_id=$1`,
	} {
		for _, afterClaim := range []bool{false, true} {
			t.Run(name+map[bool]string{false: " before", true: " after claim"}[afterClaim], func(t *testing.T) {
				f, _ := newPostgresObservationFixture(t, ctx, pool)
				mutate := func(c context.Context) error { _, err := pool.Exec(c, q, f.order.Request.OwnerID); return err }
				if !afterClaim {
					if err := mutate(ctx); err != nil {
						t.Fatal(err)
					}
				}
				db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
					if afterClaim && n == 3 {
						return mutate(c)
					}
					return nil
				}}
				called := false
				_, err := NewPostgresStore(db).CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
					called = true
					return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
				}))
				if !errors.Is(err, ErrNotAuthorized) || called {
					t.Fatal("stale cancellation access accepted", err, called)
				}
				assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			})
		}
	}
	for _, mode := range []string{"foreign owner", "unknown original broker", "already terminal"} {
		t.Run(mode, func(t *testing.T) {
			var f sendFixture
			s := NewPostgresStore(pool)
			if mode == "unknown original broker" {
				f = newSendFixture(t, ctx, pool, "BUY")
				unknownSend(t, ctx, pool, f)
			} else {
				var b BrokerObservation
				f, b = newPostgresObservationFixture(t, ctx, pool)
				if mode == "already terminal" {
					if _, err := s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
						t.Fatal(err)
					}
				}
			}
			owner := f.order.Request.OwnerID
			if mode == "foreign owner" {
				owner = f.order.Request.AccountID
			}
			before, err := s.ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil {
				t.Fatal(err)
			}
			called := false
			_, err = s.CancelBrokerOrder(ctx, owner, f.order.ID, f.vault, cancellationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (CancellationAcknowledgement, error) {
				called = true
				return CancellationAcknowledgement{}, nil
			}))
			if err == nil || called {
				t.Fatal("unknown foreign or final order was cancelled", err, called)
			}
			if mode == "unknown original broker" && !errors.Is(err, ErrSubmissionUnknown) {
				t.Fatal(err)
			}
			if _, err = s.ReadCancellation(ctx, f.order.Request.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("rejected order claimed cancellation", err)
			}
			after, err := s.ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected cancellation changed reconciliation", err)
			}
		})
	}
}

func TestPostgresCancellationExpiryAndDelayedClockDoNotRenewDeadline(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, delayed := range []bool{false, true} {
		t.Run(map[bool]string{false: "callback bounded", true: "delayed final clock"}[delayed], func(t *testing.T) {
			f, _ := newPostgresObservationFixture(t, ctx, pool)
			observed, called, bounded := false, false, false
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n == 3 {
					_, err := pool.Exec(c, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1`, f.order.Request.ConnectionID)
					return err
				}
				return nil
			}, wrap: func(n int, tx pgx.Tx) pgx.Tx {
				if delayed && n == 3 {
					return sendDelayedClockTx{tx, &observed}
				}
				return tx
			}}
			_, err := NewPostgresStore(db).CancelBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, cancellationFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission, _ Attempt) (CancellationAcknowledgement, error) {
				called = true
				deadline, ok := c.Deadline()
				bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
				<-c.Done()
				return CancellationAcknowledgement{ProviderOrderID: f.ack.ProviderOrderID, Accepted: true}, nil
			}))
			want := ErrCancellationUnknown
			if delayed {
				want = ErrNotAuthorized
			}
			if !errors.Is(err, want) || (delayed && (!observed || called)) || (!delayed && (!called || !bounded)) {
				t.Fatal("cancellation extended expired current access", err, delayed, observed, called, bounded)
			}
			assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			a, err := NewPostgresStore(pool).ReadCancellation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil || a.Outcome != "UNKNOWN" {
				t.Fatal("expired operation was not retained as unknown", err, a)
			}
		})
	}
}
