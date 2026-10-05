package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type preflightFunc func(context.Context, pgx.Tx, Order, int64) (VerifiedPreflight, error)

func (f preflightFunc) VerifyDispatchPreflight(c context.Context, tx pgx.Tx, o Order, g int64) (VerifiedPreflight, error) {
	return f(c, tx, o, g)
}

type stepUpFunc func(context.Context, string, string) (string, time.Time, error)

func (f stepUpFunc) VerifyExecutionStepUp(c context.Context, u, code string) (string, time.Time, error) {
	return f(c, u, code)
}

type authorityFixture struct {
	order     Order
	principal authorization.Principal
	approval  OwnerApproval
	proof     VerifiedPreflight
}

func newAuthorityFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, side string, limits ...OwnerPilotLimits) authorityFixture {
	t.Helper()
	r := newExecutionFixture(t, ctx, pool)
	if len(limits) != 0 {
		copy := limits[0]
		r.PilotLimits = &copy
	}
	r.Side = side
	if side == "SELL" {
		r.MaximumDebitUSD = "0"
	}
	s := NewPostgresStore(pool)
	o, err := s.Prepare(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit test-only encrypted bytes. Never obtain or decrypt a real key.
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('11',32),'hex') WHERE id=$1`, r.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO auth_totp_factors(user_id,secret_ciphertext,enabled_at) VALUES($1,decode(repeat('22',32),'hex'),clock_timestamp()-interval '1 minute')`, r.OwnerID); err != nil {
		t.Fatal(err)
	}
	f := authorityFixture{order: o, principal: authorization.Principal{UserID: r.OwnerID, Entitlement: authorization.EntitlementFounder}}
	verifier := stepUpFunc(func(c context.Context, u, code string) (string, time.Time, error) {
		var now time.Time
		err := pool.QueryRow(c, `UPDATE auth_totp_factors SET updated_at=clock_timestamp(),last_used_step=floor(extract(epoch FROM clock_timestamp())/30) WHERE user_id=$1 RETURNING updated_at`, u).Scan(&now)
		return "totp", now, err
	})
	f.approval, err = s.ApproveOrder(ctx, f.principal, o.ID, o.RequestDigest, "test-only-code", verifier)
	if err != nil {
		t.Fatal(err)
	}
	var rec string
	var now time.Time
	err = pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,observed_position_count,performance_position_count,change_count,evidence_hash,observed_at,cash_amount,cash_currency,available_cash_amount,available_cash_currency)
		VALUES($1,$2,'coinbase','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',1,0,0,decode(repeat('33',32),'hex'),clock_timestamp(),1000,'USD',60.60,'USD') RETURNING id::text,observed_at`, r.OwnerID, r.AccountID).Scan(&rec, &now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'BTC','CRYPTO','long',1,0.001,'UNAVAILABLE')`, rec, r.OwnerID, r.AccountID); err != nil {
		t.Fatal(err)
	}
	f.proof = VerifiedPreflight{EvidenceID: r.ClientOrderID, RequestDigest: o.RequestDigest, AccountID: r.AccountID, ConnectionID: r.ConnectionID, ReconciliationID: rec, CredentialGeneration: f.approval.CredentialGeneration, ObservedAt: now, ExpiresAt: now.Add(30 * time.Second), CashUSD: "1000", AvailableCashUSD: "60.60", TotalBase: "1", AvailableBase: "0.001"}
	return f
}
func (f authorityFixture) authority() *OwnerAuthority {
	return NewOwnerAuthority(preflightFunc(func(context.Context, pgx.Tx, Order, int64) (VerifiedPreflight, error) { return f.proof, nil }))
}

func TestPostgresExactOwnerAuthority(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			f := newAuthorityFixture(t, ctx, pool, side)
			o := f.order
			if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, NewOwnerAuthority(nil)); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("missing provider verifier allowed", err)
			}
			if _, err := NewPostgresStore(lostCommitDB{pool}).Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrCommitUnknown) {
				t.Fatal(err)
			}
			a, err := s.ReadAttempt(ctx, o.Request.OwnerID, o.ID)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_authorizations WHERE id=$1 AND order_id=$2 AND approval_id=$3 AND risk_evaluation->>'Decision'='ALLOW' AND risk_evaluation->>'PlatformExecutionAvailable'='false'`, a.AuthorizationID, o.ID, f.approval.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal("missing exact authorization", err, count)
			}
			if _, err = s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrAlreadyAttempted) {
				t.Fatal("replayed claim", err)
			}
			for _, query := range []string{`UPDATE execution_owner_approvals SET expires_at=expires_at+interval '1 second' WHERE order_id=$1`, `DELETE FROM execution_authorizations WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, query, o.ID); err == nil {
					t.Fatal("authority evidence mutable")
				}
			}
			if err = s.RevokeOwnerApproval(ctx, o.Request.OwnerID, o.ID); err != nil {
				t.Fatal(err)
			}
			if err = s.RevokeOwnerApproval(ctx, o.Request.OwnerID, o.ID); err != nil {
				t.Fatal("revocation replay", err)
			}
		})
	}
	for name, mutate := range map[string]func(authorityFixture) authorityFixture{
		"missing cash":         func(f authorityFixture) authorityFixture { f.proof.AvailableCashUSD = "60.59"; return f },
		"stale funds":          func(f authorityFixture) authorityFixture { f.proof.ObservedAt = time.Now().Add(-time.Minute); return f },
		"wrong key":            func(f authorityFixture) authorityFixture { f.proof.CredentialGeneration++; return f },
		"wrong digest":         func(f authorityFixture) authorityFixture { f.proof.RequestDigest = "bad"; return f },
		"wrong reconciliation": func(f authorityFixture) authorityFixture { f.proof.ReconciliationID = f.order.ID; return f },
	} {
		t.Run(name, func(t *testing.T) {
			f := mutate(newAuthorityFixture(t, ctx, pool, "BUY"))
			o := f.order
			if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("bad authorization passed", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_authorizations WHERE order_id=$1`, o.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("denial leaked authority", err, count)
			}
		})
	}
	t.Run("revoked approval", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		if err := s.RevokeOwnerApproval(ctx, o.Request.OwnerID, o.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("revocation ignored", err)
		}
	})
	t.Run("authorization cannot commit without exact attempt", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		if _, err = f.authority().AuthorizeDispatch(ctx, tx, f.order, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); !errors.Is(mapError(err), ErrNotAuthorized) {
			t.Fatal("loose authorization committed", err)
		}
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_authorizations WHERE order_id=$1`, f.order.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("failed commit retained authorization", err, count)
		}
	})
	t.Run("approval is owner and digest specific", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		other := newExecutionFixture(t, ctx, pool)
		o := f.order
		if _, err := s.ReadOwnerApproval(ctx, other.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("cross owner approval read", err)
		}
		if err := s.RevokeOwnerApproval(ctx, other.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("cross owner revocation", err)
		}
		called := false
		verifier := stepUpFunc(func(context.Context, string, string) (string, time.Time, error) {
			called = true
			return "totp", time.Now(), nil
		})
		if _, err := s.ApproveOrder(ctx, f.principal, o.ID, "different digest", "test-only", verifier); !errors.Is(err, ErrConflict) || called {
			t.Fatal("wrong order consumed MFA", err)
		}
	})
	t.Run("MFA disabled", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		if _, err := pool.Exec(ctx, `DELETE FROM auth_totp_factors WHERE user_id=$1`, o.Request.OwnerID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("disabled MFA retained approval", err)
		}
	})
	t.Run("incomplete or additional inventory", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		if _, err := pool.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'ETH','CRYPTO','long',1,1,'UNAVAILABLE')`, f.proof.ReconciliationID, o.Request.OwnerID, o.Request.AccountID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("extra account inventory ignored", err)
		}
	})
	t.Run("position insertion serializes with funding check", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		if _, err = tx.Exec(ctx, `SELECT id FROM financial_accounts WHERE id=$1 FOR UPDATE`, o.Request.AccountID); err != nil {
			t.Fatal(err)
		}
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		result := make(chan error, 1)
		go func() {
			_, e := conn.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'ETH','CRYPTO','long',1,1,'UNAVAILABLE')`, f.proof.ReconciliationID, o.Request.OwnerID, o.Request.AccountID)
			result <- e
		}()
		waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("real credential replacement", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		if _, err := pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('44',32),'hex') WHERE id=$1`, o.Request.ConnectionID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("old approval survived new key", err)
		}
	})
	t.Run("revocation wins lock race", func(t *testing.T) {
		f := newAuthorityFixture(t, ctx, pool, "BUY")
		o := f.order
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rollback(tx)
		if _, err = tx.Exec(ctx, `INSERT INTO execution_approval_revocations(approval_id) VALUES($1)`, f.approval.ID); err != nil {
			t.Fatal(err)
		}
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		result := make(chan error, 1)
		go func() { _, e := NewPostgresStore(conn).Claim(ctx, o.Request.OwnerID, o.ID, f.authority()); result <- e }()
		waitExecutionLock(t, ctx, pool, conn.Conn().PgConn().PID())
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("stale approval survived wait", err)
		}
	})
}
