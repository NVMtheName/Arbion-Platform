package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newSavedPreflightFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, side string, limits ...OwnerPilotLimits) (authorityFixture, ProviderPreflight) {
	t.Helper()
	f := newAuthorityFixture(t, ctx, pool, side, limits...)
	r := f.order.Request
	if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id=$2 WHERE id=$1`, r.AccountID, "portfolio:"+r.AccountID); err != nil {
		t.Fatal(err)
	}
	var rec string
	err := pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,observed_position_count,performance_position_count,change_count,evidence_hash,observed_at,cash_amount,cash_currency,available_cash_amount,available_cash_currency)
	 VALUES($1,$2,'coinbase','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',1,0,0,decode(repeat('55',32),'hex'),clock_timestamp(),1000,'USD',1000,'USD') RETURNING id::text`, r.OwnerID, r.AccountID).Scan(&rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'BTC','CRYPTO','long',1,1,'UNAVAILABLE')`, rec, r.OwnerID, r.AccountID); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return f, providerPreflightFixture(f.order, r.AccountID, now)
}

type captureProvider func(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error)

func (f captureProvider) CollectExecutionPreflight(c context.Context, cr *financial.Credentials, o Order, p string) (ProviderPreflight, error) {
	return f(c, cr, o, p)
}

type fixturePreflightVault struct {
	retrieve   func(context.Context, credential.Locator) ([]byte, error)
	generation int64
}

func (v fixturePreflightVault) RetrieveFinancialVersion(c context.Context, l credential.Locator) ([]byte, int64, error) {
	raw, err := v.retrieve(c, l)
	return raw, v.generation, err
}

func TestPostgresSavedProviderPreflight(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	s := NewPostgresStore(pool)
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			f, p := newSavedPreflightFixture(t, ctx, pool, side)
			r := f.order.Request
			// The mock read deliberately obtains the account lock in another
			// transaction: capture must release control locks BEFORE network work.
			provider := captureProvider(func(c context.Context, cr *financial.Credentials, o Order, portfolio string) (ProviderPreflight, error) {
				if cr.PortfolioID != portfolio || o.RequestDigest != f.order.RequestDigest {
					t.Fatal("wrong private read context")
				}
				tx, e := pool.Begin(c)
				if e != nil {
					return ProviderPreflight{}, e
				}
				defer rollback(tx)
				_, e = tx.Exec(c, `SELECT id FROM financial_accounts WHERE id=$1 FOR UPDATE NOWAIT`, r.AccountID)
				return p, e
			})
			vault := fixturePreflightVault{generation: f.approval.CredentialGeneration, retrieve: func(_ context.Context, l credential.Locator) ([]byte, error) {
				if l.ConnectionID != r.ConnectionID || l.UserID != r.OwnerID || l.Class != credential.Financial {
					t.Fatal("wrong credential scope")
				}
				return json.Marshal(financial.Credentials{PortfolioID: r.AccountID})
			}}
			id, err := s.CapturePreflight(ctx, r.OwnerID, f.order.ID, vault, provider)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `DELETE FROM execution_provider_preflights WHERE id=$1`, id); err == nil {
				t.Fatal("mutable provider evidence")
			}
			a, err := s.Claim(ctx, r.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(id)))
			if err != nil || a.CredentialGeneration != f.approval.CredentialGeneration {
				t.Fatal("saved evidence did not authorize exact mock order", err)
			}
			if _, err = s.Claim(ctx, r.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(id))); !errors.Is(err, ErrAlreadyAttempted) {
				t.Fatal("duplicate send claim", err)
			}
		})
	}
	t.Run("rotation during provider read discards evidence", func(t *testing.T) {
		f, p := newSavedPreflightFixture(t, ctx, pool, "BUY")
		r := f.order.Request
		vault := fixturePreflightVault{generation: f.approval.CredentialGeneration, retrieve: func(context.Context, credential.Locator) ([]byte, error) {
			return json.Marshal(financial.Credentials{PortfolioID: r.AccountID})
		}}
		provider := captureProvider(func(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error) {
			_, e := pool.Exec(ctx, `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('66',32),'hex') WHERE id=$1`, r.ConnectionID)
			return p, e
		})
		if _, err := s.CapturePreflight(ctx, r.OwnerID, f.order.ID, vault, provider); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("rotated credential evidence accepted", err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_provider_preflights WHERE order_id=$1`, f.order.ID).Scan(&n); err != nil || n != 0 {
			t.Fatal("rejected evidence saved", err, n)
		}
	})
	t.Run("stale vault material never reaches provider", func(t *testing.T) {
		f, _ := newSavedPreflightFixture(t, ctx, pool, "BUY")
		r := f.order.Request
		vault := fixturePreflightVault{generation: f.approval.CredentialGeneration - 1, retrieve: func(context.Context, credential.Locator) ([]byte, error) {
			return json.Marshal(financial.Credentials{PortfolioID: r.AccountID})
		}}
		called := false
		provider := captureProvider(func(context.Context, *financial.Credentials, Order, string) (ProviderPreflight, error) {
			called = true
			return ProviderPreflight{}, nil
		})
		if _, err := s.CapturePreflight(ctx, r.OwnerID, f.order.ID, vault, provider); !errors.Is(err, ErrNotAuthorized) || called {
			t.Fatal("stale material used", err, called)
		}
	})
	t.Run("exact evidence cannot authorize different order", func(t *testing.T) {
		f, p := newSavedPreflightFixture(t, ctx, pool, "BUY")
		id, err := s.savePreflight(ctx, f.order, f.approval.CredentialGeneration, p.PortfolioID, p)
		if err != nil {
			t.Fatal(err)
		}
		other, _ := newSavedPreflightFixture(t, ctx, pool, "BUY")
		if _, err = s.Claim(ctx, other.order.Request.OwnerID, other.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(id))); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("foreign evidence reused", err)
		}
	})
	t.Run("new reconciliation invalidates pinned evidence", func(t *testing.T) {
		f, p := newSavedPreflightFixture(t, ctx, pool, "BUY")
		r := f.order.Request
		id, err := s.savePreflight(ctx, f.order, f.approval.CredentialGeneration, p.PortfolioID, p)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,observed_position_count,performance_position_count,change_count,evidence_hash,observed_at,cash_amount,cash_currency,available_cash_amount,available_cash_currency) VALUES($1,$2,'coinbase','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',0,0,0,decode(repeat('77',32),'hex'),clock_timestamp(),999,'USD',999,'USD')`, r.OwnerID, r.AccountID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Claim(ctx, r.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(id))); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("old funding evidence retained", err)
		}
	})
}
