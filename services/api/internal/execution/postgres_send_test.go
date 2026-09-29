package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sendFixture struct {
	authorityFixture
	preflight ProviderPreflight
	evidence  string
	vault     fixturePreflightVault
	ack       SubmissionAcknowledgement
}

func newSendFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, side string) sendFixture {
	t.Helper()
	f, p := newSavedPreflightFixture(t, ctx, pool, side)
	id, err := NewPostgresStore(pool).savePreflight(ctx, f.order, f.approval.CredentialGeneration, p.PortfolioID, p)
	if err != nil {
		t.Fatal(err)
	}
	r := f.order.Request
	vault := fixturePreflightVault{generation: f.approval.CredentialGeneration, retrieve: func(_ context.Context, l credential.Locator) ([]byte, error) {
		if l.ConnectionID != r.ConnectionID || l.UserID != r.OwnerID || l.Class != credential.Financial {
			return nil, errors.New("incorrect synthetic credential scope")
		}
		return json.Marshal(financial.Credentials{APIKeyName: "synthetic-send-key", APIPrivateKey: "synthetic-send-private-key-not-a-real-key", PortfolioID: p.PortfolioID})
	}}
	var providerID string
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	return sendFixture{f, p, id, vault, SubmissionAcknowledgement{providerID, r.ClientOrderID, r.ProductID, r.Side}}
}

type sendFunc func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error)

func (f sendFunc) SubmitOnce(c context.Context, cr *financial.Credentials, s ConfirmedSubmission) (SubmissionAcknowledgement, error) {
	return f(c, cr, s)
}

// Each wrapper is local to one invocation: Begin 1 reads private context,
// Begin 2 claims durably, and Begin 3 enters the final send guard. Mutations
// use another connection after Claim commits, never an injected Authority.
type sendBoundaryDB struct {
	Database
	begins      int
	beforeBegin func(context.Context, int) error
	wrap        func(int, pgx.Tx) pgx.Tx
}

func (d *sendBoundaryDB) Begin(ctx context.Context) (pgx.Tx, error) {
	d.begins++
	if d.beforeBegin != nil {
		if err := d.beforeBegin(ctx, d.begins); err != nil {
			return nil, err
		}
	}
	tx, err := d.Database.Begin(ctx)
	if err != nil || d.wrap == nil {
		return tx, err
	}
	return d.wrap(d.begins, tx), nil
}

func assertSendHeld(t *testing.T, ctx context.Context, pool *pgxpool.Pool, o Order, providerID string) Attempt {
	t.Helper()
	s := NewPostgresStore(pool)
	a, err := s.ReadAttempt(ctx, o.Request.OwnerID, o.ID)
	if err != nil || a.OrderID != o.ID || a.ClientOrderID != o.Request.ClientOrderID || a.RequestDigest != o.RequestDigest || a.ProviderOrderID != providerID {
		t.Fatalf("wrong durable send outcome: %#v %v", a, err)
	}
	var held bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_account_holds WHERE order_id=$1 AND owner_id=$2 AND financial_account_id=$3)`, o.ID, o.Request.OwnerID, o.Request.AccountID).Scan(&held); err != nil || !held {
		t.Fatal("send released account hold", err, held)
	}
	hold, err := s.ReadCapitalReservation(ctx, o.Request.OwnerID, o.ID)
	kind, asset, quantity := "CASH", "USD", o.Request.MaximumDebitUSD
	if o.Request.Side == "SELL" {
		kind, asset, quantity = "ASSET", strings.TrimSuffix(o.Request.ProductID, "-USD"), o.Request.BaseSize
	}
	if err != nil || hold.OrderID != o.ID || hold.AccountID != o.Request.AccountID || hold.CapitalBucketID != o.Request.CapitalBucketID || hold.ResourceType != kind || hold.Asset != asset || !sameAmount(hold.Quantity, quantity) {
		t.Fatalf("send changed exact capital hold: %#v %v", hold, err)
	}
	var fills, terminals int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_fills WHERE order_id=$1),(SELECT count(*) FROM execution_order_terminals WHERE order_id=$1)`, o.ID).Scan(&fills, &terminals); err != nil || fills != 0 || terminals != 0 {
		t.Fatal("send manufactured settlement", err, fills, terminals)
	}
	return a
}

func assertSendCannotRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) {
	t.Helper()
	called := false
	sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	})
	// A new store and a recovered Attempt cannot turn the record into authority.
	s := NewPostgresStore(pool)
	if _, err := s.ReadAttempt(ctx, f.order.Request.OwnerID, f.order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender); !errors.Is(err, ErrAlreadyAttempted) || called {
		t.Fatal("recovery allowed a second callback", err, called)
	}
}

func TestPostgresSendConfirmedExactOrderAndAcknowledgement(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, side)
			calls := 0
			sender := sendFunc(func(c context.Context, cr *financial.Credentials, submission ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				calls++
				if cr.APIKeyName != "synthetic-send-key" || cr.APIPrivateKey != "synthetic-send-private-key-not-a-real-key" || cr.PortfolioID != f.preflight.PortfolioID {
					return SubmissionAcknowledgement{}, errors.New("wrong synthetic credentials")
				}
				if submission.Order.ID != f.order.ID || submission.Order.Request != f.order.Request || submission.Order.RequestDigest != f.order.RequestDigest || !submission.Order.CreatedAt.Equal(f.order.CreatedAt) || submission.PortfolioID != f.preflight.PortfolioID || submission.PreviewID != f.preflight.PreviewID {
					return SubmissionAcknowledgement{}, errors.New("changed exact LIMIT_IOC submission")
				}
				deadline, ok := c.Deadline()
				if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
					return SubmissionAcknowledgement{}, errors.New("unbounded send context")
				}
				// Other readers must already see the durable one-shot attempt and
				// both holds before the only mock provider callback is entered.
				assertSendHeld(t, ctx, pool, f.order, "")
				return f.ack, nil
			})
			a, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			if err != nil || calls != 1 || a.ProviderOrderID != f.ack.ProviderOrderID || a.CredentialGeneration != f.approval.CredentialGeneration {
				t.Fatal("exact send failed", err, calls, a)
			}
			assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSendConfirmedConcurrentCallsCannotResend(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	second, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var calls atomic.Int32
	sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		calls.Add(1)
		return f.ack, nil
	})
	start, results := make(chan struct{}), make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		s := NewPostgresStore(pool)
		if i%2 == 1 {
			s = NewPostgresStore(second)
		}
		go func() {
			<-start
			_, e := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			results <- e
		}()
	}
	close(start)
	wins := 0
	for i := 0; i < cap(results); i++ {
		select {
		case err := <-results:
			if err == nil {
				wins++
			} else if !errors.Is(err, ErrAlreadyAttempted) {
				t.Fatal("unexpected concurrent send result", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if wins != 1 || calls.Load() != 1 {
		t.Fatal("multiple send admissions", wins, calls.Load())
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	assertSendCannotRetry(t, ctx, second, f)
}

func TestPostgresSendConfirmedIndeterminateOutcomesRetainHolds(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, name := range []string{"callback error", "missing provider ID", "wrong client", "wrong product", "wrong side"} {
		t.Run(name, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			calls := 0
			sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				calls++
				ack := f.ack
				switch name {
				case "callback error":
					return ack, errors.New("synthetic private provider response must not escape")
				case "missing provider ID":
					ack.ProviderOrderID = ""
				case "wrong client":
					ack.ClientOrderID = f.order.ID
				case "wrong product":
					ack.ProductID = "ETH-USD"
				case "wrong side":
					ack.Side = "SELL"
				}
				return ack, nil
			})
			a, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			if err != ErrSubmissionUnknown || calls != 1 || a.OrderID != f.order.ID || a.ProviderOrderID != "" {
				t.Fatal("indeterminate send escaped", err, calls, a)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSendConfirmedCommitResponseLoss(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, phase := range []int{2, 3} {
		t.Run(fmt.Sprintf("transaction_%d", phase), func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
				if n == phase {
					return lostCommitTx{tx}
				}
				return tx
			}}
			calls := 0
			sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				calls++
				return f.ack, nil
			})
			a, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			if phase == 2 {
				if !errors.Is(err, ErrCommitUnknown) || calls != 0 || a.OrderID != "" || db.begins != 2 {
					t.Fatal("lost Claim commit reached sender", err, calls, a, db.begins)
				}
				assertSendHeld(t, ctx, pool, f.order, "")
			} else {
				if err != ErrSubmissionUnknown || calls != 1 || a.OrderID != f.order.ID || a.ProviderOrderID != "" || db.begins != 3 {
					t.Fatal("lost acknowledgement commit treated as success", err, calls, a, db.begins)
				}
				assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			}
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSendConfirmedRechecksClaimToSendGap(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, name := range []string{"approval revoked", "credential replaced", "MFA replaced", "reconciliation blocked"} {
		t.Run(name, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			r := f.order.Request
			mutated := false
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n != 3 {
					return nil
				}
				mutated = true
				switch name {
				case "approval revoked":
					return NewPostgresStore(pool).RevokeOwnerApproval(c, r.OwnerID, f.order.ID)
				case "credential replaced":
					_, err := pool.Exec(c, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('88',32),'hex') WHERE id=$1`, r.ConnectionID)
					return err
				case "MFA replaced":
					_, err := pool.Exec(c, `UPDATE auth_totp_factors SET secret_ciphertext=decode(repeat('99',32),'hex'),enabled_at=clock_timestamp() WHERE user_id=$1`, r.OwnerID)
					return err
				default:
					_, err := pool.Exec(c, `INSERT INTO execution_reconciliation_blocks(order_id,owner_id,financial_account_id,reason,payload) VALUES($1,$2,$3,'INVALID_FILL','{}')`, f.order.ID, r.OwnerID, r.AccountID)
					return err
				}
			}}
			called := false
			sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				return f.ack, nil
			})
			a, err := NewPostgresStore(db).SendConfirmed(ctx, r.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			want := ErrNotAuthorized
			if name == "reconciliation blocked" {
				want = ErrReconciliationBlocked
			}
			if !errors.Is(err, want) || called || !mutated || a.OrderID != f.order.ID {
				t.Fatal("stale Claim authority reached sender", err, called, mutated, a)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

func TestPostgresSendConfirmedStaleVaultNeverClaims(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	f.vault.generation--
	called := false
	sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	})
	s := NewPostgresStore(pool)
	if _, err := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender); !errors.Is(err, ErrNotAuthorized) || called {
		t.Fatal("stale material reached sender", err, called)
	}
	if _, err := s.ReadAttempt(ctx, f.order.Request.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale vault consumed Claim", err)
	}
	if _, err := s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale vault reserved capital", err)
	}
}

func TestPostgresSendConfirmedSerializesRevocationDuringCallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	sender := sendFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		close(entered)
		select {
		case <-release:
			return f.ack, nil
		case <-c.Done():
			return SubmissionAcknowledgement{}, c.Err()
		}
	})
	sent := make(chan error, 1)
	go func() {
		_, e := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
		sent <- e
	}()
	select {
	case <-entered:
	case e := <-sent:
		t.Fatal("send stopped before callback", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	revoked := make(chan error, 1)
	go func() {
		revoked <- NewPostgresStore(conn).RevokeOwnerApproval(ctx, f.order.Request.OwnerID, f.order.ID)
	}()
	waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
	select {
	case e := <-revoked:
		t.Fatal("revocation escaped synchronous send guard", e)
	default:
	}
	var recorded bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_approval_revocations WHERE approval_id=$1)`, f.approval.ID).Scan(&recorded); err != nil || recorded {
		t.Fatal("revocation committed during callback", err, recorded)
	}
	once.Do(func() { close(release) })
	if err = <-sent; err != nil {
		t.Fatal("guarded send failed", err)
	}
	if err = <-revoked; err != nil {
		t.Fatal("revocation failed after callback completed", err)
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
	assertSendCannotRetry(t, ctx, pool, f)
}

func TestPostgresSendConfirmedExpiryBoundsCallbackAndKeepsUnknown(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, name := range []string{"entitlement", "connection"} {
		t.Run(name, func(t *testing.T) {
			f := newSendFixture(t, ctx, pool, "BUY")
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n != 3 {
					return nil
				}
				statement := `UPDATE user_entitlements SET expires_at=clock_timestamp()+interval '2 seconds' WHERE user_id=$1`
				if name == "connection" {
					statement = `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE user_id=$1`
				}
				_, err := pool.Exec(c, statement, f.order.Request.OwnerID)
				return err
			}}
			called, bounded, timedOut := false, false, false
			sender := sendFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				deadline, ok := c.Deadline()
				bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 2*time.Second
				<-c.Done()
				timedOut = errors.Is(c.Err(), context.DeadlineExceeded)
				// Even a syntactically valid acknowledgement is indeterminate
				// when returned after the synchronously observed deadline.
				return f.ack, nil
			})
			a, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
			if err != ErrSubmissionUnknown || !called || !bounded || !timedOut || a.OrderID != f.order.ID {
				t.Fatal("expiring authority did not bound callback", err, called, bounded, timedOut, a)
			}
			assertSendHeld(t, ctx, pool, f.order, "")
			assertSendCannotRetry(t, ctx, pool, f)
		})
	}
}

type sendDelayedClockTx struct {
	pgx.Tx
	observed *bool
}

func (tx sendDelayedClockTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	row := tx.Tx.QueryRow(ctx, query, args...)
	if strings.Contains(query, "e.expires_at,c.authorization_expires_at,clock_timestamp()") {
		return sendDelayedClockRow{Row: row, ctx: ctx, observed: tx.observed}
	}
	return row
}

type sendDelayedClockRow struct {
	pgx.Row
	ctx      context.Context
	observed *bool
}

func (r sendDelayedClockRow) Scan(dest ...any) error {
	if err := r.Row.Scan(dest...); err != nil {
		return err
	}
	*r.observed = true
	end, now := *(dest[1].(**time.Time)), *(dest[3].(*time.Time))
	if end == nil || !end.After(now) {
		return errors.New("fixture expected positive final authority window")
	}
	// Simulate a stalled database response after it captured its wall clock.
	// Waiting on the known deadline is intentional, not a scheduling assertion.
	timer := time.NewTimer(end.Sub(now) + 100*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

func TestPostgresSendConfirmedDelayedFinalClockCannotExtendAuthority(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	observed, called := false, false
	db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
		if n != 3 {
			return nil
		}
		_, err := pool.Exec(c, `UPDATE user_entitlements SET expires_at=clock_timestamp()+interval '2 seconds' WHERE user_id=$1`, f.order.Request.OwnerID)
		return err
	}, wrap: func(n int, tx pgx.Tx) pgx.Tx {
		if n == 3 {
			return sendDelayedClockTx{tx, &observed}
		}
		return tx
	}}
	sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	})
	a, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
	if !errors.Is(err, ErrNotAuthorized) || called || !observed || a.OrderID != f.order.ID {
		t.Fatal("delayed clock response extended authority", err, called, observed, a)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
	assertSendCannotRetry(t, ctx, pool, f)
}

func TestPostgresSendConfirmedDatabaseSessionLossNeverResends(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f := newSendFixture(t, ctx, pool, "BUY")
	var pid uint32
	db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
		if n == 3 {
			pid = tx.Conn().PgConn().PID()
		}
		return tx
	}}
	calls, terminated := 0, false
	sender := sendFunc(func(c context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		calls++
		if pid == 0 {
			return SubmissionAcknowledgement{}, errors.New("missing final transaction PID")
		}
		err := pool.QueryRow(c, `SELECT pg_terminate_backend($1,1000)`, pid).Scan(&terminated)
		// Database-session loss can release locks while this callback still
		// runs. A valid mock acknowledgement cannot repair that transaction.
		return f.ack, err
	})
	a, err := NewPostgresStore(db).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
	if err != ErrSubmissionUnknown || calls != 1 || !terminated || a.OrderID != f.order.ID || a.ProviderOrderID != "" {
		t.Fatal("database loss permitted success or resend", err, calls, terminated, a)
	}
	assertSendHeld(t, ctx, pool, f.order, "")
	assertSendCannotRetry(t, ctx, pool, f)
}
