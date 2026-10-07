package execution

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newPilotAllocationRequest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, initial string) Request {
	t.Helper()
	r := newExecutionFixture(t, ctx, pool)
	r.PilotLimits = &OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := pool.Exec(ctx, `UPDATE financial_accounts SET provider_account_id='portfolio:'||id::text WHERE id=$1`, r.AccountID); err != nil {
		t.Fatal(err)
	}
	_, err := NewPostgresStore(pool).RegisterPilotAllocation(ctx, PilotAllocation{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID,
		CapitalBucketID: r.CapitalBucketID, ProductID: r.ProductID, InitialCashUSD: initial, Limits: *r.PilotLimits})
	if err != nil {
		t.Fatal(err)
	}
	assertPilotAllocationBalance(t, ctx, pool, r, initial, "0", 0, false)
	return r
}

func pilotAllocationOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, r Request, side, base, debit string) sendFixture {
	t.Helper()
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
		t.Fatal(err)
	}
	r.Side, r.BaseSize, r.MaximumDebitUSD, r.FeeAllowanceUSD = side, base, debit, "1"
	f, p := newSavedPreflightForAuthority(t, ctx, pool, newAuthorityFixtureForRequest(t, ctx, pool, r))
	quantity, _ := decimal(base, true)
	price, _ := decimal(r.LimitPrice, true)
	gross := new(big.Rat).Mul(quantity, price)
	fee, _ := decimal(r.FeeAllowanceUSD, false)
	total := new(big.Rat).Add(gross, fee)
	if side == "SELL" {
		total.Sub(gross, fee)
	}
	p.PreviewBaseSize, p.PreviewQuoteSize = base, canonical(gross)
	p.PreviewFeeUSD, p.PreviewTotalUSD = r.FeeAllowanceUSD, canonical(total)
	// The provider fixture deliberately has 1000 USD and 1 BTC, much more than
	// the allocation. Availability is evidence, never permission to spend it.
	return newSendFixtureForPreflight(t, ctx, pool, f, p)
}

func assertPilotAllocationBalance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, r Request, cash, base string, count int64, pending bool) {
	t.Helper()
	got, err := NewPostgresStore(pool).ReadPilotBalance(ctx, r.OwnerID, r.AccountID)
	if err != nil || got.OwnerID != r.OwnerID || got.AccountID != r.AccountID || got.ConnectionID != r.ConnectionID ||
		got.CapitalBucketID != r.CapitalBucketID || got.ProductID != r.ProductID || !sameAmount(got.CashUSD, cash) ||
		!sameAmount(got.BaseQuantity, base) || got.SettledOrderCount != count || got.Pending != pending || got.RegisteredAt.IsZero() {
		t.Fatalf("incorrect receipt-derived pilot balance: %#v, %v", got, err)
	}
}

func sendPilotAllocationOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) {
	t.Helper()
	calls := 0
	_, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		calls++
		return f.ack, nil
	}))
	if err != nil || calls != 1 {
		t.Fatal("allocated synthetic order did not submit exactly once", err, calls)
	}
}

func denyPilotAllocationOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture, want error) {
	t.Helper()
	called := false
	s := NewPostgresStore(pool)
	_, err := s.SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return f.ack, nil
	}))
	if !errors.Is(err, want) || called {
		t.Fatal("unallocated or pending funds reached the sender", err, called)
	}
	if _, err = s.ReadAttempt(ctx, f.order.Request.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("denied allocation created a durable attempt", err)
	}
}

// Build observed fills and account reads, then run the production reconciler.
// Only SettleBrokerAccount below may create a receipt; no fixture inserts one.
func pilotAllocationTerminal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture, status, base, gross, fee string) AccountSettlementEvidence {
	t.Helper()
	s := NewPostgresStore(pool)
	r := f.order.Request
	a, err := s.ReadAttempt(ctx, r.OwnerID, f.order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	id := BrokerIdentity{r.OwnerID, f.order.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, a.ProviderOrderID, r.ProductID, r.Side}
	b := BrokerObservation{BrokerIdentity: id, Status: status, CompleteFills: true, Totals: Totals{1, base, gross, fee}, StartedAt: now, ObservedAt: now}
	b.Fills = []Fill{{BrokerIdentity: id, TradeID: "allocation-" + r.ClientOrderID, BaseQuantity: base, PriceUSD: r.LimitPrice, GrossUSD: gross, FeeUSD: fee, TradedAt: a.ClaimedAt, ObservedAt: now,
		ProviderEvidence: &FillProviderEvidence{EntryID: "allocation-" + r.ClientOrderID, SequenceAt: a.ClaimedAt, Size: base, FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}}}
	if _, err = s.ReconcileBrokerOrder(ctx, r.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	b.StartedAt, b.ObservedAt, b.Fills[0].ObservedAt = now, now, now
	cash, _ := decimal(f.preflight.CashUSD, false)
	position, _ := decimal(f.preflight.TotalBase, false)
	quantity, _ := decimal(base, true)
	notional, _ := decimal(gross, true)
	commission, _ := decimal(fee, false)
	if r.Side == "BUY" {
		cash.Sub(cash, notional)
		position.Add(position, quantity)
	} else {
		cash.Add(cash, notional)
		position.Sub(position, quantity)
	}
	cash.Sub(cash, commission)
	return AccountSettlementEvidence{PortfolioID: f.preflight.PortfolioID, Observation: b, CashUSD: canonical(cash), AvailableCashUSD: canonical(cash),
		TotalBase: canonical(position), AvailableBase: canonical(position), Complete: true, NoOpenOrders: true, StartedAt: now, CompletedAt: now}
}

func settlePilotAllocationOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture, evidence AccountSettlementEvidence) AccountSettlement {
	t.Helper()
	r, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(evidence))
	if err != nil {
		t.Fatal(err)
	}
	assertSettlementReceipt(t, ctx, pool, f, evidence, r)
	return r
}

func TestPostgresPilotAllocationExcludesBrokerAssetsAndStopsAfterFourBuys(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r := newPilotAllocationRequest(t, ctx, pool, "100")
	initialSell := pilotAllocationOrder(t, ctx, pool, r, "SELL", "0.0004", "0")
	denyPilotAllocationOrder(t, ctx, pool, initialSell, ErrNotAuthorized)
	assertPilotAllocationBalance(t, ctx, pool, r, "100", "0", 0, false)
	cash := []string{"100", "75", "50", "25", "0"}
	base := []string{"0", "0.0004", "0.0008", "0.0012", "0.0016"}
	for i := 0; i < 4; i++ {
		f := pilotAllocationOrder(t, ctx, pool, r, "BUY", "0.0004", "25")
		sendPilotAllocationOrder(t, ctx, pool, f)
		assertPilotAllocationBalance(t, ctx, pool, r, cash[i], base[i], int64(i), true)
		var next sendFixture
		if i == 0 {
			next = pilotAllocationOrder(t, ctx, pool, r, "BUY", "0.0004", "25")
			denyPilotAllocationOrder(t, ctx, pool, next, ErrAccountHeld)
		}
		e := pilotAllocationTerminal(t, ctx, pool, f, "FILLED", "0.0004", "24", "1")
		assertPilotAllocationBalance(t, ctx, pool, r, cash[i], base[i], int64(i), true)
		if i == 0 {
			denyPilotAllocationOrder(t, ctx, pool, next, ErrCapitalHeld)
		}
		settlePilotAllocationOrder(t, ctx, pool, f, e)
		assertPilotAllocationBalance(t, ctx, pool, r, cash[i+1], base[i+1], int64(i+1), false)
	}
	fifth := pilotAllocationOrder(t, ctx, pool, r, "BUY", "0.0004", "25")
	denyPilotAllocationOrder(t, ctx, pool, fifth, ErrNotAuthorized)
	assertPilotAllocationBalance(t, ctx, pool, r, "0", "0.0016", 4, false)
}

func TestPostgresPilotAllocationReusesOnlyNetSettledProceeds(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r := newPilotAllocationRequest(t, ctx, pool, "25")
	for i, leg := range []struct{ side, base, debit, gross, cash, position string }{
		{"BUY", "0.0004", "25", "24", "0", "0.0004"},
		{"SELL", "0.0004", "0", "24", "23", "0"},
		{"BUY", "0.00036", "22.6", "21.6", "0.4", "0.00036"},
	} {
		f := pilotAllocationOrder(t, ctx, pool, r, leg.side, leg.base, leg.debit)
		sendPilotAllocationOrder(t, ctx, pool, f)
		e := pilotAllocationTerminal(t, ctx, pool, f, "FILLED", leg.base, leg.gross, "1")
		settlePilotAllocationOrder(t, ctx, pool, f, e)
		assertPilotAllocationBalance(t, ctx, pool, r, leg.cash, leg.position, int64(i+1), false)
	}
}

func TestPostgresPilotAllocationNoSendClosureDoesNotDebitOrCredit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, change := range []string{"approval revoked", "bucket shrunk after claim"} {
		t.Run(change, func(t *testing.T) {
			r := newPilotAllocationRequest(t, ctx, pool, "100")
			f := pilotAllocationOrder(t, ctx, pool, r, "BUY", "0.0004", "25")
			changed, called := false, false
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(c context.Context, n int) error {
				if n != 3 {
					return nil
				}
				changed = true
				if change == "approval revoked" {
					return NewPostgresStore(pool).RevokeOwnerApproval(c, r.OwnerID, f.order.ID)
				}
				// 99 still covers this 25 order; only the registered 100 pilot
				// capacity check can reject the change at the final send boundary.
				_, err := pool.Exec(c, `UPDATE capital_buckets SET allocation_value=99 WHERE id=$1`, r.CapitalBucketID)
				return err
			}}
			_, err := NewPostgresStore(db).SendConfirmed(ctx, r.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				return f.ack, nil
			}))
			if !changed || called || !errors.Is(err, ErrSubmissionNotSent) || !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("changed allocation entered sender or lost no-send proof", err, changed, called)
			}
			assertNoSendClosed(t, ctx, pool, f)
			assertPilotAllocationBalance(t, ctx, pool, r, "100", "0", 0, false)
		})
	}
}

func TestPostgresPilotAllocationPartialCancellationLostCommitNeverDoubleCredits(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	r := newPilotAllocationRequest(t, ctx, pool, "100")
	buy := pilotAllocationOrder(t, ctx, pool, r, "BUY", "0.0004", "25")
	sendPilotAllocationOrder(t, ctx, pool, buy)
	partialBuy := pilotAllocationTerminal(t, ctx, pool, buy, "CANCELLED", "0.0002", "12", "0.5")
	settlePilotAllocationOrder(t, ctx, pool, buy, partialBuy)
	assertPilotAllocationBalance(t, ctx, pool, r, "87.5", "0.0002", 1, false)
	sell := pilotAllocationOrder(t, ctx, pool, r, "SELL", "0.0002", "0")
	sendPilotAllocationOrder(t, ctx, pool, sell)
	partialSell := pilotAllocationTerminal(t, ctx, pool, sell, "CANCELLED", "0.0001", "6", "0.25")
	assertPilotAllocationBalance(t, ctx, pool, r, "87.5", "0.0002", 1, true)
	if _, err := NewPostgresStore(lostCommitDB{pool}).SettleBrokerAccount(ctx, r.OwnerID, sell.order.ID, sell.vault, savedSettlement(partialSell)); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("lost settlement response was treated as certain", err)
	}
	assertPilotAllocationBalance(t, ctx, pool, r, "93.25", "0.0001", 2, false)
	var first AccountSettlement
	for i := 0; i < 2; i++ {
		got, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, r.OwnerID, sell.order.ID, sell.vault, settlementProviderFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (AccountSettlementEvidence, error) {
			t.Error("settlement restart repeated a provider read")
			return AccountSettlementEvidence{}, ErrInvalid
		}))
		if err != nil || i > 0 && !reflect.DeepEqual(first, got) {
			t.Fatal("restart changed immutable settlement", err)
		}
		first = got
		assertSettlementReceipt(t, ctx, pool, sell, partialSell, got)
		assertPilotAllocationBalance(t, ctx, pool, r, "93.25", "0.0001", 2, false)
	}
	oversell := pilotAllocationOrder(t, ctx, pool, r, "SELL", "0.0002", "0")
	denyPilotAllocationOrder(t, ctx, pool, oversell, ErrNotAuthorized)
	assertPilotAllocationBalance(t, ctx, pool, r, "93.25", "0.0001", 2, false)
}
