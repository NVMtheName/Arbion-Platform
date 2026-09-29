package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type settlementProviderFunc func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error)

func (f settlementProviderFunc) CollectAccountSettlement(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (AccountSettlementEvidence, error) {
	return f(c, cr, s, a)
}

func savedSettlement(e AccountSettlementEvidence) settlementProviderFunc {
	return func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
		return e, nil
	}
}

func newSettlementFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, side, kind string) (sendFixture, AccountSettlementEvidence) {
	t.Helper()
	f := newSendFixture(t, ctx, pool, side)
	s := NewPostgresStore(pool)
	a, err := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		return f.ack, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	r := f.order.Request
	id := BrokerIdentity{r.OwnerID, f.order.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, a.ProviderOrderID, r.ProductID, r.Side}
	b := BrokerObservation{BrokerIdentity: id, Status: "FILLED", CompleteFills: true, Totals: Totals{1, "0.001", "60", "0.6"}, StartedAt: now, ObservedAt: now}
	if kind == "partial" {
		b.Status, b.Totals = "CANCELLED", Totals{1, "0.0004", "24", "0.24"}
	}
	b.Fills = []Fill{{BrokerIdentity: id, TradeID: "settlement-trade", BaseQuantity: b.BaseQuantity, PriceUSD: "60000", GrossUSD: b.GrossUSD, FeeUSD: b.FeeUSD, TradedAt: a.ClaimedAt, ObservedAt: now,
		ProviderEvidence: &FillProviderEvidence{EntryID: "settlement-entry", SequenceAt: a.ClaimedAt, Size: b.BaseQuantity, FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}}}
	if kind == "zero" {
		b.Status, b.Totals, b.Fills = "CANCELLED", Totals{0, "0", "0", "0"}, []Fill{}
	}
	if _, err = s.ReconcileBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	b.StartedAt, b.ObservedAt = now, now
	for i := range b.Fills {
		b.Fills[i].ObservedAt = now
	}
	closeCash, closeBase := "1000", "1"
	if kind == "full" && side == "BUY" {
		closeCash, closeBase = "939.4", "1.001"
	}
	if kind == "full" && side == "SELL" {
		closeCash, closeBase = "1059.4", "0.999"
	}
	if kind == "partial" && side == "BUY" {
		closeCash, closeBase = "975.76", "1.0004"
	}
	if kind == "partial" && side == "SELL" {
		closeCash, closeBase = "1023.76", "0.9996"
	}
	return f, AccountSettlementEvidence{PortfolioID: f.preflight.PortfolioID, Observation: b, CashUSD: closeCash, AvailableCashUSD: closeCash, TotalBase: closeBase, AvailableBase: closeBase, Complete: true, NoOpenOrders: true, StartedAt: now, CompletedAt: now}
}

func assertSettlementHeld(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) {
	t.Helper()
	r, err := NewPostgresStore(pool).ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
	var count int
	if err != nil || r.ReleasedAt != nil {
		t.Fatal("unverified account evidence released capital", err, r)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_account_settlements WHERE order_id=$1`, f.order.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("unverified settlement receipt persisted", err, count)
	}
}

func assertSettlementReceipt(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture, e AccountSettlementEvidence, got AccountSettlement) {
	t.Helper()
	if got.BrokerIdentity != e.Observation.BrokerIdentity || got.PortfolioID != e.PortfolioID || got.CredentialGeneration != f.approval.CredentialGeneration || got.TerminalStatus != e.Observation.Status || got.Totals != e.Observation.Totals || got.OpeningCashUSD != "1000" || got.OpeningBase != "1" || got.ClosingCashUSD != e.CashUSD || got.ClosingBase != e.TotalBase || got.RecordedAt.IsZero() {
		t.Fatal("incorrect exact account settlement receipt", got)
	}
	r, err := NewPostgresStore(pool).ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || r.ReleasedAt == nil || !r.ReleasedAt.Equal(got.RecordedAt) || r.OrderID != f.order.ID || r.Quantity == "0" {
		t.Fatal("reservation history was not preserved with exact receipt release", err, r)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_account_settlements WHERE order_id=$1`, f.order.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("settlement receipt is not once-only", err, count)
	}
}

func TestPostgresSettlementExactBuySellPartialZeroAndRestart(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, side := range []string{"BUY", "SELL"} {
		for _, kind := range []string{"full", "partial", "zero"} {
			t.Run(side+" "+kind, func(t *testing.T) {
				f, e := newSettlementFixture(t, ctx, pool, side, kind)
				assertSettlementHeld(t, ctx, pool, f)
				r, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e))
				if err != nil {
					t.Fatal(err)
				}
				assertSettlementReceipt(t, ctx, pool, f, e, r)
				replay, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, settlementProviderFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
					t.Error("restart of completed settlement made another provider call")
					return AccountSettlementEvidence{}, ErrInvalid
				}))
				if err != nil || !reflect.DeepEqual(replay, r) {
					t.Fatal("restart changed immutable settlement", err, replay)
				}
				assertSendCannotRetry(t, ctx, pool, f)
			})
		}
	}
}

func TestPostgresSettlementRejectsIncompleteDiscrepantStaleAndForeignEvidence(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	cases := map[string]func(*AccountSettlementEvidence){
		"cash discrepancy":            func(e *AccountSettlementEvidence) { e.CashUSD, e.AvailableCashUSD = "940", "940" },
		"cash hold":                   func(e *AccountSettlementEvidence) { e.AvailableCashUSD = "939" },
		"asset discrepancy":           func(e *AccountSettlementEvidence) { e.TotalBase, e.AvailableBase = "1.002", "1.002" },
		"asset hold":                  func(e *AccountSettlementEvidence) { e.AvailableBase = "1" },
		"incomplete":                  func(e *AccountSettlementEvidence) { e.Complete = false },
		"open order":                  func(e *AccountSettlementEvidence) { e.NoOpenOrders = false },
		"foreign portfolio":           func(e *AccountSettlementEvidence) { e.PortfolioID = e.Observation.OwnerID },
		"stale":                       func(e *AccountSettlementEvidence) { e.StartedAt = e.StartedAt.Add(-31 * time.Second) },
		"different terminal":          func(e *AccountSettlementEvidence) { e.Observation.Status = "CANCELLED" },
		"different trade same totals": func(e *AccountSettlementEvidence) { e.Observation.Fills[0].TradeID = "different-trade" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, e := newSettlementFixture(t, ctx, pool, "BUY", "full")
			mutate(&e)
			if _, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e)); !errors.Is(err, ErrUnreconciled) {
				t.Fatal("bad settlement evidence accepted", err)
			}
			assertSettlementHeld(t, ctx, pool, f)
		})
	}
}

func TestPostgresSettlementRequiresCurrentAccessAcrossCollection(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for name, query := range map[string]string{
		"revoked owner":      `UPDATE users SET status='disabled' WHERE id=$1`,
		"revoked connection": `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"changed key":        `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('77',32),'hex') WHERE user_id=$1`,
		"foreign account":    `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE user_id=$1`,
	} {
		for _, after := range []bool{false, true} {
			t.Run(name+map[bool]string{false: " before", true: " after"}[after], func(t *testing.T) {
				f, e := newSettlementFixture(t, ctx, pool, "BUY", "full")
				mutate := func() {
					if _, err := pool.Exec(ctx, query, f.order.Request.OwnerID); err != nil {
						t.Fatal(err)
					}
				}
				if !after {
					mutate()
				}
				called := false
				_, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, settlementProviderFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
					called = true
					mutate()
					return e, nil
				}))
				if !errors.Is(err, ErrNotAuthorized) || called != after {
					t.Fatal("stale settlement access accepted", err, called)
				}
				assertSettlementHeld(t, ctx, pool, f)
			})
		}
	}
}

func TestPostgresSettlementConcurrentAndUnknownCommit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent", true: "unknown commit"}[lost], func(t *testing.T) {
			f, e := newSettlementFixture(t, ctx, pool, "SELL", "partial")
			var first AccountSettlement
			if lost {
				if _, err := NewPostgresStore(lostCommitDB{pool}).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e)); !errors.Is(err, ErrCommitUnknown) {
					t.Fatal("lost commit treated as certain", err)
				}
				var err error
				first, err = NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, settlementProviderFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
					t.Error("committed receipt reran provider")
					return AccountSettlementEvidence{}, ErrInvalid
				}))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				type result struct {
					receipt AccountSettlement
					err     error
				}
				results := make(chan result, 4)
				var wg sync.WaitGroup
				for i := 0; i < 4; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						r, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e))
						results <- result{r, err}
					}()
				}
				wg.Wait()
				close(results)
				for r := range results {
					if r.err != nil {
						t.Fatal(r.err)
					}
					if first.OrderID == "" {
						first = r.receipt
					} else if !reflect.DeepEqual(first, r.receipt) {
						t.Fatal("concurrent receipts differed")
					}
				}
			}
			assertSettlementReceipt(t, ctx, pool, f, e, first)
		})
	}
}

func TestPostgresSettlementOnlyReceiptReleasesCapitalAndHistoryDoesNotBlockNextClaim(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f, e := newSettlementFixture(t, ctx, pool, "BUY", "partial")
	if _, err := pool.Exec(ctx, `UPDATE execution_capital_reservations SET released_at=clock_timestamp() WHERE order_id=$1`, f.order.ID); err == nil {
		t.Fatal("forged release without receipt accepted")
	}
	assertSettlementHeld(t, ctx, pool, f)
	s := NewPostgresStore(pool)
	receipt, err := s.SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e))
	if err != nil {
		t.Fatal(err)
	}
	assertSettlementReceipt(t, ctx, pool, f, e, receipt)
	for _, q := range []string{`UPDATE execution_account_settlements SET closing_cash_usd=1 WHERE order_id=$1`, `DELETE FROM execution_account_settlements WHERE order_id=$1`, `UPDATE execution_capital_reservations SET released_at=NULL WHERE order_id=$1`, `UPDATE execution_capital_reservations SET quantity=1 WHERE order_id=$1`, `DELETE FROM execution_capital_reservations WHERE order_id=$1`} {
		if _, err = pool.Exec(ctx, q, f.order.ID); err == nil {
			t.Fatal("immutable settlement or released history mutated")
		}
	}
	next := f.order.Request
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&next.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	o, err := s.Prepare(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	// Test-only authority proves the released history no longer blocks the
	// account/capital guards. It is not approval or permission for a real send.
	authority := authorityFunc(func(c context.Context, tx pgx.Tx, o Order, now time.Time) (Authorization, error) {
		a, err := fixtureAuthority.AuthorizeDispatch(c, tx, o, now)
		a.CredentialGeneration = f.approval.CredentialGeneration
		return a, err
	})
	if _, err = s.Claim(ctx, next.OwnerID, o.ID, authority); err != nil {
		t.Fatal("released historical reservation blocked fresh claim", err)
	}
	assertSendCannotRetry(t, ctx, pool, f)
	old, err := s.ReadCapitalReservation(ctx, next.OwnerID, f.order.ID)
	if err != nil || old.ReleasedAt == nil {
		t.Fatal("fresh claim erased released history", err)
	}
}

type settlementExpiryTx struct {
	pgx.Tx
	expires time.Time
	waited  *bool
	stage   string
}

func (tx settlementExpiryTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	insert := strings.HasPrefix(query, "INSERT INTO execution_account_settlements")
	if insert && tx.stage == "immediate application recheck" {
		if _, err := tx.Tx.Exec(ctx, `SET CONSTRAINTS execution_account_settlement_commit IMMEDIATE`); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	tag, err := tx.Tx.Exec(ctx, query, args...)
	if err == nil && insert && tx.stage != "deferred commit recheck" {
		err = tx.waitForExpiry(ctx)
	}
	return tag, err
}

func (tx settlementExpiryTx) Commit(ctx context.Context) error {
	if tx.stage == "deferred commit recheck" {
		if err := tx.waitForExpiry(ctx); err != nil {
			return err
		}
	}
	return tx.Tx.Commit(ctx)
}

func (tx settlementExpiryTx) waitForExpiry(ctx context.Context) error {
	*tx.waited = true
	timer := time.NewTimer(time.Until(tx.expires) + 25*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPostgresSettlementExpiryRollsBackReceiptAndRelease(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, stage := range []string{"default application recheck", "immediate application recheck", "deferred commit recheck"} {
		t.Run(stage, func(t *testing.T) {
			f, e := newSettlementFixture(t, ctx, pool, "BUY", "full")
			s := NewPostgresStore(pool)
			before, err := s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil {
				t.Fatal(err)
			}
			var expiry time.Time
			if err = pool.QueryRow(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING authorization_expires_at`, f.order.Request.ConnectionID).Scan(&expiry); err != nil {
				t.Fatal(err)
			}
			waited := false
			db := &sendBoundaryDB{Database: pool, wrap: func(_ int, tx pgx.Tx) pgx.Tx { return settlementExpiryTx{tx, expiry, &waited, stage} }}
			want := ErrNotAuthorized
			if stage == "deferred commit recheck" {
				want = ErrCommitUnknown
			}
			if _, err = NewPostgresStore(db).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e)); !errors.Is(err, want) || !waited {
				t.Fatal("settlement release committed after access expired", err, waited)
			}
			assertSettlementHeld(t, ctx, pool, f)
			after, err := s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatal("failed settlement changed reservation history", err)
			}
		})
	}
}
