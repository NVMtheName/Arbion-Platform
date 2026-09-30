package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresObservationFunc func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (BrokerObservation, error)

func (f postgresObservationFunc) CollectExecutionObservation(c context.Context, cr *financial.Credentials, s ConfirmedSubmission, a Attempt) (BrokerObservation, error) {
	return f(c, cr, s, a)
}

func newPostgresObservationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (sendFixture, BrokerObservation) {
	t.Helper()
	f := newSendFixture(t, ctx, pool, "BUY")
	a, err := NewPostgresStore(pool).SendConfirmed(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
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
	fill := Fill{BrokerIdentity: id, TradeID: "observation-trade", BaseQuantity: "0.001", PriceUSD: "60000", GrossUSD: "60", FeeUSD: "0.6", TradedAt: a.ClaimedAt, ObservedAt: now,
		ProviderEvidence: &FillProviderEvidence{EntryID: "observation-entry", SequenceAt: a.ClaimedAt, Size: "0.001", FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}}
	return f, BrokerObservation{BrokerIdentity: id, Status: "FILLED", CompleteFills: true, Totals: Totals{1, "0.001", "60", "0.6"}, Fills: []Fill{fill}, StartedAt: now, ObservedAt: now}
}

func savedObservation(b BrokerObservation) postgresObservationFunc {
	return func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (BrokerObservation, error) {
		return b, nil
	}
}

func TestPostgresObservationRequiresCurrentAccessBeforeAndAfterCollection(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	mutations := map[string]string{
		"owner":      `UPDATE users SET status='disabled' WHERE id=$1`,
		"connection": `UPDATE provider_connections SET status='revoked' WHERE user_id=$1`,
		"key":        `UPDATE provider_connections SET encrypted_credential_payload=decode(repeat('66',32),'hex') WHERE user_id=$1`,
		"portfolio":  `UPDATE financial_accounts SET provider_account_id='portfolio:'||gen_random_uuid()::text WHERE user_id=$1`,
	}
	for name, query := range mutations {
		for _, after := range []bool{false, true} {
			t.Run(name+map[bool]string{false: " before", true: " after"}[after], func(t *testing.T) {
				f, b := newPostgresObservationFixture(t, ctx, pool)
				mutate := func() {
					if _, err := pool.Exec(ctx, query, f.order.Request.OwnerID); err != nil {
						t.Fatal(err)
					}
				}
				if !after {
					mutate()
				}
				called := false
				provider := postgresObservationFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission, Attempt) (BrokerObservation, error) {
					called = true
					mutate() // Acquires another connection: collection holds no access locks.
					return b, nil
				})
				_, err := NewPostgresStore(pool).ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, provider)
				if !errors.Is(err, ErrNotAuthorized) || called != after {
					t.Fatal("stale collection persisted evidence", err, called)
				}
				assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			})
		}
	}
}

func TestPostgresObservationConcurrentReverseReplayIsExact(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f, earlier := newPostgresObservationFixture(t, ctx, pool)
	later := earlier
	later.Fills = append([]Fill(nil), earlier.Fills...)
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&later.ObservedAt); err != nil {
		t.Fatal(err)
	}
	later.Fills[0].ObservedAt = later.ObservedAt
	s := NewPostgresStore(pool)
	if _, err := s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(later)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := 0; i < 4; i++ {
		b := earlier
		if i%2 == 1 {
			b = later
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := NewPostgresStore(pool).ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("exact reverse/concurrent replay failed", err)
		}
	}
	assertPostgresObservationSettled(t, ctx, pool, f, later.ObservedAt)
}

func TestPostgresObservationLostCommitAndConflictingFill(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	t.Run("lost commit recovers once", func(t *testing.T) {
		f, b := newPostgresObservationFixture(t, ctx, pool)
		_, err := NewPostgresStore(lostCommitDB{pool}).ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b))
		if !errors.Is(err, ErrCommitUnknown) {
			t.Fatal("lost commit reported as certain", err)
		}
		if _, err = NewPostgresStore(pool).ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
			t.Fatal("restart could not recover committed observation", err)
		}
		assertPostgresObservationSettled(t, ctx, pool, f, b.ObservedAt)
	})
	t.Run("complete conflicting fill quarantines", func(t *testing.T) {
		f, b := newPostgresObservationFixture(t, ctx, pool)
		b.Status = "OPEN"
		s := NewPostgresStore(pool)
		if _, err := s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
			t.Fatal(err)
		}
		// Still individually valid and within the owner's allowance, but the
		// same trade/entry cannot change its fee after being committed.
		b.FeeUSD, b.Fills[0].FeeUSD = "0.59", "0.59"
		if _, err := s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); !errors.Is(err, ErrReconciliationBlocked) {
			t.Fatal("conflicting complete evidence did not quarantine", err)
		}
		x, err := NewPostgresStore(pool).ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
		if err != nil || !x.AccountBlocked || !x.AccountHeld || x.FillCount != 1 || x.FeeUSD != "0.6" || x.TerminalStatus != "" {
			t.Fatal("quarantine or original economics were not durable", err, x)
		}
		var blocks, terminals int
		err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_reconciliation_blocks WHERE order_id=$1),(SELECT count(*) FROM execution_order_terminals WHERE order_id=$1)`, f.order.ID).Scan(&blocks, &terminals)
		if err != nil || blocks != 1 || terminals != 0 {
			t.Fatal("wrong durable conflict outcome", err, blocks, terminals)
		}
		if _, err = s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID); err != nil {
			t.Fatal("quarantine lost capital reservation", err)
		}
	})
}

func TestPostgresObservationRejectsMalformedOptionalProvenance(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f, b := newPostgresObservationFixture(t, ctx, pool)
	fill := b.Fills[0]
	encoded, err := json.Marshal(fill)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"EntryID", "SizeInQuote", "FeeCurrencyBasis"} {
		var payload map[string]any
		if err = json.Unmarshal(encoded, &payload); err != nil {
			t.Fatal(err)
		}
		payload["ProviderEvidence"].(map[string]any)[field] = nil
		malformed, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO execution_fills(order_id,owner_id,financial_account_id,provider_order_id,trade_id,base_quantity,price_usd,gross_usd,fee_usd,traded_at,observed_at,payload)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, fill.OrderID, fill.OwnerID, fill.AccountID, fill.ProviderOrderID, fill.TradeID, fill.BaseQuantity, fill.PriceUSD, fill.GrossUSD, fill.FeeUSD, fill.TradedAt, fill.ObservedAt, malformed)
		if err == nil {
			t.Fatal("SQL accepted malformed optional provider evidence", field)
		}
	}
	assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
}

type observationExpireAfterConflictTx struct {
	pgx.Tx
	expires time.Time
	waited  *bool
}

func (tx observationExpireAfterConflictTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	tag, err := tx.Tx.Exec(ctx, query, args...)
	if err == nil && strings.HasPrefix(query, "INSERT INTO execution_reconciliation_blocks") {
		*tx.waited = true
		timer := time.NewTimer(time.Until(tx.expires) + 25*time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return tag, ctx.Err()
		}
	}
	return tag, err
}

func TestPostgresObservationConflictCommitRechecksAccessExpiry(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	f, b := newPostgresObservationFixture(t, ctx, pool)
	b.Status = "OPEN"
	s := NewPostgresStore(pool)
	if _, err := s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); err != nil {
		t.Fatal(err)
	}
	b.FeeUSD, b.Fills[0].FeeUSD = "0.59", "0.59"
	var expiry time.Time
	if err := pool.QueryRow(ctx, `UPDATE provider_connections SET authorization_expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1 RETURNING authorization_expires_at`, f.order.Request.ConnectionID).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	waited := false
	db := &sendBoundaryDB{Database: pool, wrap: func(_ int, tx pgx.Tx) pgx.Tx {
		return observationExpireAfterConflictTx{tx, expiry, &waited}
	}}
	_, err := NewPostgresStore(db).ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b))
	if !errors.Is(err, ErrNotAuthorized) || !waited {
		t.Fatal("conflict committed past connection expiry", err, waited)
	}
	x, err := s.ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
	var blocks int
	if err != nil || x.AccountBlocked || !x.AccountHeld || x.FillCount != 1 || x.FeeUSD != "0.6" {
		t.Fatal("expired-access conflict changed durable facts", err, x)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_reconciliation_blocks WHERE order_id=$1`, f.order.ID).Scan(&blocks); err != nil || blocks != 0 {
		t.Fatal("quarantine escaped expired transaction", err, blocks)
	}
	if _, err = pool.Exec(ctx, `UPDATE provider_connections SET authorization_expires_at=NULL WHERE id=$1`, f.order.Request.ConnectionID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReconcileBrokerOrder(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedObservation(b)); !errors.Is(err, ErrReconciliationBlocked) {
		t.Fatal("fresh access did not preserve proven conflict", err)
	}
	x, err = NewPostgresStore(pool).ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || !x.AccountBlocked || !x.AccountHeld || x.FillCount != 1 || x.FeeUSD != "0.6" {
		t.Fatal("fresh conflict was not durably quarantined", err, x)
	}
}

func assertPostgresObservationSettled(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture, first time.Time) {
	t.Helper()
	s := NewPostgresStore(pool)
	x, err := s.ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || x.FillCount != 1 || x.BaseQuantity != "0.001" || x.GrossUSD != "60" || x.FeeUSD != "0.6" || x.TerminalStatus != "FILLED" || x.AccountHeld || x.AccountBlocked {
		t.Fatal("incorrect exact terminal recovery", err, x)
	}
	var count int
	var completed time.Time
	if err = pool.QueryRow(ctx, `SELECT count(*),min(completed_at) FROM execution_order_terminals WHERE order_id=$1`, f.order.ID).Scan(&count, &completed); err != nil || count != 1 || !completed.Equal(first.Round(time.Microsecond)) {
		t.Fatal("replay changed first immutable terminal observation", err, count, completed)
	}
	capital, err := s.ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || capital.ResourceType != "CASH" || !sameAmount(capital.Quantity, f.order.Request.MaximumDebitUSD) {
		t.Fatal("order terminal released exact capital reservation", err, capital)
	}
	assertSendCannotRetry(t, ctx, pool, f)
}
