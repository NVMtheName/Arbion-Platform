package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCurrentClaimControls(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	for name, statement := range map[string]string{
		"owner":                  `UPDATE users SET status='disabled' WHERE id=$1`,
		"entitlement":            `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`,
		"future grant":           `UPDATE user_entitlements SET starts_at=clock_timestamp()+interval '1 hour' WHERE user_id=$1`,
		"account":                `UPDATE financial_accounts SET status='disabled' WHERE user_id=$1`,
		"connection":             `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"authorization deadline": `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE user_id=$1`,
		"rotated credential":     `UPDATE provider_connections SET credential_generation=credential_generation+1 WHERE user_id=$1`,
		"bucket":                 `UPDATE capital_buckets SET status='ARCHIVED' WHERE user_id=$1`,
		"protected cash":         `UPDATE capital_buckets SET protected_amount=50 WHERE user_id=$1`,
		"absolute ceiling":       `UPDATE capital_buckets SET allocation_limit=60 WHERE user_id=$1`,
		"reserve bucket":         `UPDATE capital_buckets SET is_reserve=true WHERE user_id=$1`,
		"percentage allocation":  `UPDATE capital_buckets SET allocation_type='PERCENT_OF_AVAILABLE_CASH' WHERE user_id=$1`,
		"owner stop":             `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('USER',$1,'OPEN','fixture','SYSTEM')`,
	} {
		t.Run(name, func(t *testing.T) {
			r := newExecutionFixture(t, ctx, pool)
			o, err := s.Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, statement, r.OwnerID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Claim(ctx, r.OwnerID, o.ID, fixtureAuthority); !errors.Is(err, ErrNotAuthorized) {
				t.Fatalf("stale control accepted: %v", err)
			}
			if _, err = s.ReadCapitalReservation(ctx, r.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("denial reserved capital", err)
			}
		})
	}
	t.Run("deadline closes during approval", func(t *testing.T) {
		r := newExecutionFixture(t, ctx, pool)
		o, err := s.Prepare(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		authority := authorityFunc(func(c context.Context, tx pgx.Tx, o Order, now time.Time) (Authorization, error) {
			a, err := fixtureAuthority(c, tx, o, now)
			if err != nil {
				return a, err
			}
			_, err = tx.Exec(c, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, r.ConnectionID)
			return a, err
		})
		if _, err = s.Claim(ctx, r.OwnerID, o.ID, authority); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("expired precommit access accepted", err)
		}
	})
}

func TestPostgresPersistentCapitalReservation(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			r := newExecutionFixture(t, ctx, pool)
			r.Side = side
			if side == "SELL" {
				r.MaximumDebitUSD = "0"
			}
			o, err := s.Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			// A lost commit response must leave both the attempt and capital held.
			if _, err = NewPostgresStore(lostCommitDB{pool}).Claim(ctx, r.OwnerID, o.ID, fixtureAuthority); !errors.Is(err, ErrCommitUnknown) {
				t.Fatal(err)
			}
			hold, err := s.ReadCapitalReservation(ctx, r.OwnerID, o.ID)
			if err != nil {
				t.Fatal(err)
			}
			asset, kind, qty := "USD", "CASH", "60.6"
			if side == "SELL" {
				asset, kind, qty = "BTC", "ASSET", "0.001"
			}
			if hold.OrderID != o.ID || hold.AccountID != r.AccountID || hold.CapitalBucketID != r.CapitalBucketID || hold.Asset != asset || hold.ResourceType != kind || hold.Quantity != qty {
				t.Fatalf("wrong capital: %#v", hold)
			}
			other := newExecutionFixture(t, ctx, pool)
			if _, err = s.ReadCapitalReservation(ctx, other.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
				t.Fatal("cross owner read", err)
			}
			for _, statement := range []string{`UPDATE execution_capital_reservations SET quantity=1 WHERE order_id=$1`, `DELETE FROM execution_capital_reservations WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, statement, o.ID); err == nil {
					t.Fatal("capital changed without account proof")
				}
			}
			// Recovering/acknowledging the order cannot reduce the hold.
			if err = s.RecordAcknowledgement(ctx, r.OwnerID, o.ID, r.ClientOrderID); err != nil {
				t.Fatal(err)
			}
			again, err := s.ReadCapitalReservation(ctx, r.OwnerID, o.ID)
			if err != nil || again != hold {
				t.Fatal("ack released capital", err)
			}
			for _, status := range []string{"CANCELLED"} {
				now := time.Now().UTC()
				terminal := TerminalReport{BrokerIdentity: BrokerIdentity{r.OwnerID, o.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, r.ClientOrderID, r.ProductID, side}, Status: status, CompleteFills: true, BaseQuantity: "0", GrossUSD: "0", FeeUSD: "0", CompletedAt: now, ObservedAt: now}
				if err = s.ReconcileTerminal(ctx, terminal); err != nil {
					t.Fatal(err)
				}
			}
			again, err = s.ReadCapitalReservation(ctx, r.OwnerID, o.ID)
			if err != nil || again != hold {
				t.Fatal("zero-fill terminal released capital", err)
			}
			// SQL attempts are subject to the same control/reservation boundary.
			r.ClientOrderID = other.ClientOrderID
			next, err := s.Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `INSERT INTO execution_dispatch_attempts(order_id,owner_id,financial_account_id,authorization_id,credential_generation,claimed_at,expires_at) VALUES($1,$2,$3,gen_random_uuid(),1,clock_timestamp(),clock_timestamp()+interval '30 seconds')`, next.ID, r.OwnerID, r.AccountID)
			if !errors.Is(mapError(err), ErrCapitalHeld) {
				t.Fatal("SQL bypassed capital fence", err)
			}
		})
	}
}

// Observe an actual lock wait, not a scheduler timing assumption.
func waitExecutionLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("claim never waited on control")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func TestPostgresClaimWaitsForCurrentControls(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, statement := range map[string]string{
		"owner revocation":      `UPDATE users SET status='disabled' WHERE id=$1`,
		"connection revocation": `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"bucket reduction":      `UPDATE capital_buckets SET allocation_value=10 WHERE user_id=$1`,
		"first owner stop":      `INSERT INTO risk_circuit_breakers(scope,scope_id,state,reason,source) VALUES('USER',$1,'OPEN','fixture','SYSTEM')`,
	} {
		t.Run(name, func(t *testing.T) {
			r := newExecutionFixture(t, ctx, pool)
			o, err := NewPostgresStore(pool).Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollback(writer)
			if _, err = writer.Exec(ctx, statement, r.OwnerID); err != nil {
				t.Fatal(err)
			}
			conn, err := pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Release()
			result := make(chan error, 1)
			go func() { _, e := NewPostgresStore(conn).Claim(ctx, r.OwnerID, o.ID, fixtureAuthority); result <- e }()
			waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
			if err = writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("stale claim survived wait", err)
			}
		})
	}
}
