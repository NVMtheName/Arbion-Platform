package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financialconnection"
	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testNonLiveCommitAccess(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	tests := []struct {
		name, target, query string
		want                error
	}{
		{"disabled owner", "user", `UPDATE users SET status='disabled' WHERE id=$1`, ErrCommitAccessRevoked},
		{"unverified owner", "user", `UPDATE users SET status='pending_verification' WHERE id=$1`, ErrCommitAccessRevoked},
		{"revoked entitlement", "user", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`, ErrCommitAccessRevoked},
		{"deleted entitlement", "user", `DELETE FROM user_entitlements WHERE user_id=$1`, ErrCommitAccessRevoked},
		{"future entitlement", "user", `UPDATE user_entitlements SET starts_at=clock_timestamp()+interval '1 hour' WHERE user_id=$1`, ErrCommitAccessRevoked},
		{"closed account", "account", `UPDATE financial_accounts SET status='closed' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"missing account", "account", `UPDATE financial_accounts SET status='unavailable' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"disabled financial connection", "financial", `UPDATE provider_connections SET status='disabled' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"revoked financial connection", "financial", `UPDATE provider_connections SET status='revoked' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"pending financial connection", "financial", `UPDATE provider_connections SET status='pending' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"expired financial authorization", "financial", `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"disabled pinned AI connection", "ai", `UPDATE provider_connections SET status='disabled' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"revoked pinned AI connection", "ai", `UPDATE provider_connections SET status='revoked' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"financial provider mismatch", "financial", `UPDATE provider_connections SET provider_name='schwab' WHERE id=$1`, ErrCommitConnectionUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			id := commitAccessFixtureID(t, ctx, f, tc.target)
			if _, err := pool.Exec(ctx, tc.query, id); err != nil {
				t.Fatal(err)
			}
			if err := f.commit(ctx); !errors.Is(err, tc.want) {
				t.Fatal("stale access accepted by Paper", err)
			}
			f.assertEmpty(t, ctx)
			err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
			if !errors.Is(err, tc.want) {
				t.Fatal("stale access accepted by generic writer", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("administrative role does not replace automation entitlement", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if _, err := pool.Exec(ctx, `UPDATE users SET role='superadmin' WHERE id=$1`, f.instance.UserID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM user_entitlements WHERE user_id=$1`, f.instance.UserID); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrCommitAccessRevoked) {
			t.Fatal("role bypassed product access", err)
		}
		f.assertEmpty(t, ctx)
	})
	for _, pinnedActive := range []bool{true, false} {
		t.Run("new draft does not replace pinned AI authority/"+map[bool]string{true: "active", false: "disabled"}[pinnedActive], func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			pinnedID := commitAccessFixtureID(t, ctx, f, "ai")
			draftID := paperBindingUUID(t, ctx, pool)
			draftStatus := "disabled"
			if !pinnedActive {
				draftStatus = "active"
				if _, err := pool.Exec(ctx, `UPDATE provider_connections SET status='disabled' WHERE id=$1`, pinnedID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Exec(ctx, `INSERT INTO provider_connections(id,user_id,provider_category,provider_name,display_name,status) VALUES($1,$2,'ai','openai',$4,$3)`, draftID, f.instance.UserID, draftStatus, "Draft "+draftID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2,ai_provider_connection_id=$2 WHERE id=$1`, f.instance.AutomationMandateID, draftID); err != nil {
				t.Fatal(err)
			}
			err := f.commit(ctx)
			if pinnedActive && err != nil {
				t.Fatal("unrelated disabled draft blocked pinned authority", err)
			}
			if !pinnedActive {
				if !errors.Is(err, ErrCommitConnectionUnavailable) {
					t.Fatal("new draft bypassed pinned connection revocation", err)
				}
				f.assertEmpty(t, ctx)
			}
		})
	}
	for _, tc := range []struct {
		name, target, query string
		want                error
	}{
		{"owner", "user", `UPDATE users SET status='disabled' WHERE id=$1`, ErrCommitAccessRevoked},
		{"entitlement", "user", `UPDATE user_entitlements SET status='revoked' WHERE user_id=$1`, ErrCommitAccessRevoked},
		{"account", "account", `UPDATE financial_accounts SET status='closed' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"financial", "financial", `UPDATE provider_connections SET status='disabled' WHERE id=$1`, ErrCommitConnectionUnavailable},
		{"AI", "ai", `UPDATE provider_connections SET status='disabled' WHERE id=$1`, ErrCommitConnectionUnavailable},
	} {
		t.Run(tc.name+" revocation wins", func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			id := commitAccessFixtureID(t, ctx, f, tc.target)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, tc.query, id); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- f.commit(ctx) }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, tc.want) {
				t.Fatal("revocation lost after lock wait", err)
			}
			f.assertEmpty(t, ctx)
		})
		t.Run(tc.name+" revocation waits for access guard", func(t *testing.T) {
			f := newPaperBindingFixture(t, ctx, pool)
			id := commitAccessFixtureID(t, ctx, f, tc.target)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = lockNonLiveCommitAccess(ctx, tx, f.instance, "coinbase"); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { _, e := pool.Exec(ctx, tc.query, id); result <- e }()
			waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-result; err != nil {
				t.Fatal(err)
			}
			if err = f.commit(ctx); !errors.Is(err, tc.want) {
				t.Fatal("acknowledged revocation bypassed", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
	t.Run("ordinary sync does not interrupt pending fill", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `UPDATE financial_accounts SET status='unavailable' WHERE id=$1`, f.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- f.commit(ctx) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		if _, err = tx.Exec(ctx, `UPDATE financial_accounts SET status='active',last_synced_at=clock_timestamp() WHERE id=$1`, f.instance.FinancialAccountID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; err != nil {
			t.Fatal("uncommitted sync intermediate state rejected", err)
		}
	})
	t.Run("same owner accounts and shared read guards remain independent", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		other := newPaperBindingFixture(t, ctx, pool, f.instance.UserID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = lockNonLiveCommitAccess(ctx, tx, f.instance, "coinbase"); err != nil {
			t.Fatal(err)
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		otherTx, err := pool.Begin(bounded)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = lockNonLiveCommitAccess(bounded, otherTx, other.instance, "coinbase"); err != nil {
			t.Fatal("shared owner lock serialized readers", err)
		}
		if err = otherTx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		otherID := commitAccessFixtureID(t, ctx, other, "financial")
		if err = financialconnection.NewPostgresStore(pool).Retire(bounded, other.instance.UserID, otherID); err != nil {
			t.Fatal("other account retirement blocked", err)
		}
		if err = f.commit(bounded); err != nil {
			t.Fatal("other account retirement disrupted fill", err)
		}
	})
	t.Run("refreshable access token expiry is not permission revocation", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		id := commitAccessFixtureID(t, ctx, f, "financial")
		if _, err := pool.Exec(ctx, `UPDATE provider_connections SET token_expires_at=clock_timestamp()-interval '1 minute',authorization_expires_at=clock_timestamp()+interval '1 hour' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("expiry during final ledger wait rolls back all effects", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		id := commitAccessFixtureID(t, ctx, f, "financial")
		var expiry time.Time
		if err := pool.QueryRow(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING authorization_expires_at`, id).Scan(&expiry); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM paper_portfolios WHERE strategy_instance_id=$1 FOR UPDATE`, f.instance.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- f.commit(ctx) }()
		waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
		for {
			var expired bool
			if err = pool.QueryRow(ctx, `SELECT clock_timestamp()>$1`, expiry).Scan(&expired); err != nil {
				t.Fatal(err)
			}
			if expired {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-result; !errors.Is(err, ErrCommitConnectionUnavailable) {
			t.Fatal("expired authorization survived ledger wait", err)
		}
		f.assertEmpty(t, ctx)
	})
	t.Run("completed duplicates remain safe after revocation", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if err := f.commit(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, f.instance.UserID); err != nil {
			t.Fatal(err)
		}
		if err := f.commit(ctx); !errors.Is(err, ErrDuplicate) {
			t.Fatal("duplicate recovery broken", err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
	})
	t.Run("revocation preserves immutable denial evidence", func(t *testing.T) {
		f := newPaperBindingFixture(t, ctx, pool)
		if _, err := pool.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, f.instance.UserID); err != nil {
			t.Fatal(err)
		}
		f.evaluation.Decision = risk.Deny
		err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now)
		if err != nil {
			t.Fatal(err)
		}
		assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
	})
}

func commitAccessFixtureID(t *testing.T, ctx context.Context, f paperBindingFixture, target string) string {
	t.Helper()
	switch target {
	case "user":
		return f.instance.UserID
	case "account":
		return f.instance.FinancialAccountID
	}
	var id string
	query, source := `SELECT provider_connection_id::text FROM financial_accounts WHERE id=$1`, f.instance.FinancialAccountID
	if target == "ai" {
		query, source = `SELECT ai_provider_connection_id::text FROM automation_mandates WHERE id=$1`, f.instance.AutomationMandateID
	}
	if err := f.store.db.QueryRow(ctx, query, source).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
