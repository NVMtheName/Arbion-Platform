package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresFillReconciliation(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	store := NewPostgresStore(pool)
	newScenario := func(side string) (Order, Fill, Fill, TerminalReport) {
		t.Helper()
		r := newExecutionFixture(t, ctx, pool)
		r.Side = side
		if side == "SELL" {
			r.MaximumDebitUSD = "0"
		}
		o, err := store.Prepare(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		a, err := store.Claim(ctx, r.OwnerID, o.ID, fixtureAuthority)
		if err != nil {
			t.Fatal(err)
		}
		var brokerID string
		if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&brokerID); err != nil {
			t.Fatal(err)
		}
		if err = store.RecordAcknowledgement(ctx, r.OwnerID, o.ID, brokerID); err != nil {
			t.Fatal(err)
		}
		b := BrokerIdentity{r.OwnerID, o.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, brokerID, r.ProductID, r.Side}
		now := time.Now().UTC()
		f1 := Fill{b, "first", "0.0004", "60000", "24", "0.24", a.ClaimedAt.Add(time.Microsecond), now, nil}
		f2 := Fill{b, "second", "0.0006", "60000", "36", "0.36", a.ClaimedAt.Add(2900 * time.Nanosecond), now, nil}
		return o, f1, f2, TerminalReport{b, "FILLED", true, 2, "0.001", "60", "0.6", now, now, ""}
	}
	assertHeld := func(o Order, want bool) {
		t.Helper()
		x, err := store.ReadReconciliation(ctx, o.Request.OwnerID, o.ID)
		if err != nil || x.AccountHeld != want {
			t.Fatalf("hold=%v want=%v err=%v", x.AccountHeld, want, err)
		}
	}
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			o, f1, f2, terminal := newScenario(side)
			// Terminal before the complete fill chain cannot release the slot.
			if err := store.ReconcileTerminal(ctx, terminal); !errors.Is(err, ErrUnreconciled) {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM execution_account_holds WHERE order_id=$1`, o.ID); err == nil {
				t.Fatal("unsettled hold deleted")
			}
			assertHeld(o, true)
			// Relational settlement facts cannot disagree with replay payloads.
			forged := f2
			forged.GrossUSD = "37"
			payload, _ := json.Marshal(forged)
			_, err := pool.Exec(ctx, `INSERT INTO execution_fills(order_id,owner_id,financial_account_id,provider_order_id,trade_id,base_quantity,price_usd,gross_usd,fee_usd,traded_at,observed_at,payload)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, o.ID, f2.OwnerID, f2.AccountID, f2.ProviderOrderID, f2.TradeID, f2.BaseQuantity, f2.PriceUSD, f2.GrossUSD, f2.FeeUSD, f2.TradedAt.Round(time.Microsecond), f2.ObservedAt.Round(time.Microsecond), payload)
			if err == nil {
				t.Fatal("mismatched fill payload persisted")
			}
			// Out-of-order delivery and concurrent duplicates settle each trade once.
			if err := store.RecordFill(ctx, f2); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); results <- store.RecordFill(ctx, f1) }()
			}
			wg.Wait()
			close(results)
			for err := range results {
				if err != nil {
					t.Fatal(err)
				}
			}
			replay := f1
			replay.BaseQuantity = "0.000400"
			replay.FeeUSD = "0.240"
			replay.ObservedAt = time.Now().UTC()
			if err := store.RecordFill(ctx, replay); err != nil {
				t.Fatal(err)
			}
			for _, edit := range []func(*TerminalReport){func(t *TerminalReport) { t.CompleteFills = false }, func(t *TerminalReport) { t.FeeUSD = "0.59" }, func(t *TerminalReport) { t.FillCount = 1 }, func(t *TerminalReport) { t.CompletedAt = f1.TradedAt }, func(t *TerminalReport) { t.CompletedAt = f2.TradedAt.Add(-time.Nanosecond) }, func(t *TerminalReport) { t.GrossUSD = "59.999999999999999999" }} {
				bad := terminal
				edit(&bad)
				if err := store.ReconcileTerminal(ctx, bad); !errors.Is(err, ErrUnreconciled) {
					t.Fatalf("mismatch: %v", err)
				}
				assertHeld(o, true)
			}
			// A lost acknowledgement of the terminal commit is safely replayable.
			if err := NewPostgresStore(lostCommitDB{pool}).ReconcileTerminal(ctx, terminal); !errors.Is(err, ErrCommitUnknown) {
				t.Fatal(err)
			}
			if err := store.ReconcileTerminal(ctx, terminal); err != nil {
				t.Fatal(err)
			}
			assertHeld(o, false)
			x, err := store.ReadReconciliation(ctx, o.Request.OwnerID, o.ID)
			if err != nil || x.FillCount != 2 || x.BaseQuantity != "0.001" || x.GrossUSD != "60" || x.FeeUSD != "0.6" || x.TerminalStatus != "FILLED" || x.AccountBlocked {
				t.Fatalf("bad totals: %#v %v", x, err)
			}
			// Recovery in a fresh process sees terminal truth and cannot reclaim it.
			body, _ := json.Marshal([]string{o.Request.OwnerID, o.ID})
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostgresReconciliationRecoveryChild$", "-test.count=1")
			child.Env = append(os.Environ(), "ARBION_RECONCILIATION_RECOVERY_FIXTURE="+string(body))
			if out, err := child.CombinedOutput(); err != nil {
				t.Fatalf("recovery: %v %s", err, out)
			}
			if err = store.RecordFill(ctx, f1); err != nil {
				t.Fatal("known fill replay after terminal", err)
			}
			if _, err = store.Claim(ctx, o.Request.OwnerID, o.ID, fixtureAuthority); !errors.Is(err, ErrAlreadyAttempted) {
				t.Fatal("old order reclaimed", err)
			}
			for _, q := range []string{`DELETE FROM execution_fills WHERE order_id=$1`, `UPDATE execution_fills SET fee_usd=0 WHERE order_id=$1`, `DELETE FROM execution_order_terminals WHERE order_id=$1`} {
				if _, err = pool.Exec(ctx, q, o.ID); err == nil {
					t.Fatal("evidence mutated")
				}
			}
			// Terminal order proof frees the slot, but not the capital fence.
			r := o.Request
			if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
				t.Fatal(err)
			}
			next, err := store.Prepare(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.Claim(ctx, r.OwnerID, next.ID, nil); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal(err)
			}
			if _, err = store.Claim(ctx, r.OwnerID, next.ID, fixtureAuthority); !errors.Is(err, ErrCapitalHeld) {
				t.Fatal("order terminal incorrectly freed account capital", err)
			}
			// A correction to the old order blocks the whole account, including the
			// next prepared order. It cannot quietly change totals or free capacity.
			conflict := f1
			conflict.FeeUSD = "0.23"
			if err = store.RecordFill(ctx, conflict); !errors.Is(err, ErrReconciliationBlocked) {
				t.Fatal(err)
			}
			x, err = store.ReadReconciliation(ctx, r.OwnerID, next.ID)
			if err != nil || !x.AccountBlocked || x.AccountHeld {
				t.Fatalf("correction failed to stop account: %#v %v", x, err)
			}
		})
	}
	for _, status := range []string{"CANCELLED", "REJECTED", "EXPIRED"} {
		t.Run(status, func(t *testing.T) {
			o, f1, _, terminal := newScenario("BUY")
			terminal.Status = status
			terminal.FillCount = 0
			terminal.BaseQuantity = "0"
			terminal.GrossUSD = "0"
			terminal.FeeUSD = "0"
			if status == "CANCELLED" {
				if err := store.RecordFill(ctx, f1); err != nil {
					t.Fatal(err)
				}
				terminal.FillCount = 1
				terminal.BaseQuantity = f1.BaseQuantity
				terminal.GrossUSD = f1.GrossUSD
				terminal.FeeUSD = f1.FeeUSD
			}
			if err := store.ReconcileTerminal(ctx, terminal); err != nil {
				t.Fatal(err)
			}
			assertHeld(o, false)
			// New late fill is never silently accepted after terminal settlement.
			f1.TradeID = "late"
			if err := store.RecordFill(ctx, f1); !errors.Is(err, ErrReconciliationBlocked) {
				t.Fatal(err)
			}
			x, err := store.ReadReconciliation(ctx, o.Request.OwnerID, o.ID)
			if err != nil || !x.AccountBlocked {
				t.Fatal("late fill did not block", err)
			}
		})
	}
	// Terminal corrections, including malformed ones, quarantine after release.
	t.Run("terminal correction", func(t *testing.T) {
		o, _, _, tr := newScenario("BUY")
		tr.Status = "REJECTED"
		tr.FillCount = 0
		tr.BaseQuantity = "0"
		tr.GrossUSD = "0"
		tr.FeeUSD = "0"
		if err := store.ReconcileTerminal(ctx, tr); err != nil {
			t.Fatal(err)
		}
		tr.FeeUSD = "-1"
		if err := store.ReconcileTerminal(ctx, tr); !errors.Is(err, ErrReconciliationBlocked) {
			t.Fatal(err)
		}
		r := o.Request
		if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&r.ClientOrderID); err != nil {
			t.Fatal(err)
		}
		next, err := store.Prepare(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.Claim(ctx, r.OwnerID, next.ID, fixtureAuthority); !errors.Is(err, ErrReconciliationBlocked) {
			t.Fatal("blocked account reused", err)
		}
		if _, err = pool.Exec(ctx, `DELETE FROM execution_reconciliation_blocks WHERE order_id=$1`, o.ID); err == nil {
			t.Fatal("block could be cleared")
		}
	})
	t.Run("scope and cumulative limits", func(t *testing.T) {
		o, f1, f2, _ := newScenario("BUY")
		other := newExecutionFixture(t, ctx, pool)
		bad := f1
		bad.OwnerID = other.OwnerID
		if err := store.RecordFill(ctx, bad); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		bad = f1
		bad.AccountID = other.AccountID
		if err := store.RecordFill(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := store.ReadReconciliation(ctx, other.OwnerID, o.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		if err := store.RecordFill(ctx, f1); err != nil {
			t.Fatal(err)
		}
		f2.FeeUSD = "0.37"
		if err := store.RecordFill(ctx, f2); !errors.Is(err, ErrReconciliationBlocked) {
			t.Fatal("fee budget overrun accepted", err)
		}
		assertHeld(o, true)
	})
	t.Run("cancel fill race", func(t *testing.T) {
		o, f1, f2, tr := newScenario("BUY")
		if err := store.RecordFill(ctx, f1); err != nil {
			t.Fatal(err)
		}
		tr.Status = "CANCELLED"
		tr.FillCount = 1
		tr.BaseQuantity = f1.BaseQuantity
		tr.GrossUSD = f1.GrossUSD
		tr.FeeUSD = f1.FeeUSD
		results := make(chan error, 2)
		go func() { results <- store.RecordFill(ctx, f2) }()
		go func() { results <- store.ReconcileTerminal(ctx, tr) }()
		for i := 0; i < 2; i++ {
			err := <-results
			if err != nil && !errors.Is(err, ErrReconciliationBlocked) && !errors.Is(err, ErrUnreconciled) {
				t.Fatal(err)
			}
		}
		x, err := store.ReadReconciliation(ctx, o.Request.OwnerID, o.ID)
		if err != nil || (!x.AccountHeld && !x.AccountBlocked) {
			t.Fatalf("race released account unsafely: %#v %v", x, err)
		}
	})
}

func TestPostgresReconciliationPreservesHalfMicrosecondEvidence(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	// The two conversion rules are observably different; this is not a test
	// that merely replaces one equivalent representation with another.
	fixed := time.Date(2026, time.September, 29, 12, 0, 0, 500, time.UTC)
	var differs bool
	if err := pool.QueryRow(ctx, `SELECT $1::timestamptz IS DISTINCT FROM ($2::text)::timestamptz`, fixed.Round(time.Microsecond), fixed.Format(time.RFC3339Nano)).Scan(&differs); err != nil || !differs {
		t.Fatal("fixture did not expose independent Go/PostgreSQL rounding", err, differs)
	}
	for name, offset := range map[string]time.Duration{"500ns": 500 * time.Nanosecond, "1500ns": 1500 * time.Nanosecond} {
		t.Run(name, func(t *testing.T) {
			f, b := newPostgresObservationFixture(t, ctx, pool)
			origin := nextSettlementPrecisionOrigin(t, ctx, pool)
			exact := origin.Add(offset)
			fill := b.Fills[0]
			fill.TradedAt, fill.ObservedAt = exact, exact
			terminal := TerminalReport{BrokerIdentity: b.BrokerIdentity, Status: "FILLED", CompleteFills: true, FillCount: 1,
				BaseQuantity: fill.BaseQuantity, GrossUSD: fill.GrossUSD, FeeUSD: fill.FeeUSD, CompletedAt: exact, ObservedAt: exact}
			s := NewPostgresStore(pool)
			for i := 0; i < 2; i++ {
				if err := s.RecordFill(ctx, fill); err != nil {
					t.Fatal("half-microsecond fill persistence/replay", err)
				}
				if err := s.ReconcileTerminal(ctx, terminal); err != nil {
					t.Fatal("half-microsecond terminal persistence/replay", err)
				}
			}
			var columnsMatch bool
			var traded, observed, completed, terminalObserved string
			err := pool.QueryRow(ctx, `SELECT f.traded_at=(f.payload->>'TradedAt')::timestamptz
			 AND f.observed_at=(f.payload->>'ObservedAt')::timestamptz
			 AND t.completed_at=(t.payload->>'CompletedAt')::timestamptz
			 AND t.observed_at=(t.payload->>'ObservedAt')::timestamptz,
			 f.payload->>'TradedAt',f.payload->>'ObservedAt',t.payload->>'CompletedAt',t.payload->>'ObservedAt'
			 FROM execution_fills f JOIN execution_order_terminals t ON t.order_id=f.order_id WHERE f.order_id=$1`, f.order.ID).
				Scan(&columnsMatch, &traded, &observed, &completed, &terminalObserved)
			want := exact.Format(time.RFC3339Nano)
			if err != nil || !columnsMatch || traded != want || observed != want || completed != want || terminalObserved != want {
				t.Fatal("relational timestamp disagreed with immutable exact evidence", err, columnsMatch, traded, observed, completed, terminalObserved)
			}
			recovered, err := NewPostgresStore(pool).ReadReconciliation(ctx, f.order.Request.OwnerID, f.order.ID)
			if err != nil || recovered.FillCount != 1 || recovered.TerminalStatus != "FILLED" || recovered.AccountHeld || recovered.AccountBlocked {
				t.Fatal("precision replay changed lifecycle outcome", err, recovered)
			}
		})
	}
}

// A whole-second origin makes the half-microsecond boundary deterministic,
// independent of binary rounding at an arbitrary fractional-second base. The
// database wait is bounded below one second and ensures evidence is not future.
func nextSettlementPrecisionOrigin(t *testing.T, ctx context.Context, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var origin time.Time
	if err := pool.QueryRow(ctx, `SELECT date_trunc('second',clock_timestamp())+interval '1 second'`).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT pg_sleep(GREATEST(0,extract(epoch FROM $1::timestamptz-clock_timestamp()))::double precision)`, origin.Add(2*time.Microsecond)); err != nil {
		t.Fatal(err)
	}
	return origin.UTC()
}

func TestPostgresReconciliationRecoveryChild(t *testing.T) {
	fixture := os.Getenv("ARBION_RECONCILIATION_RECOVERY_FIXTURE")
	if fixture == "" {
		t.Skip("subprocess fixture only")
	}
	var ids []string
	if json.Unmarshal([]byte(fixture), &ids) != nil || len(ids) != 2 {
		t.Fatal("invalid fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("STRATEGY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := NewPostgresStore(pool)
	r, err := s.ReadReconciliation(ctx, ids[0], ids[1])
	if err != nil || r.TerminalStatus != "FILLED" || r.FillCount != 2 || r.AccountHeld || r.AccountBlocked {
		t.Fatalf("lost terminal settlement: %#v %v", r, err)
	}
	if _, err = s.Claim(ctx, ids[0], ids[1], fixtureAuthority); !errors.Is(err, ErrAlreadyAttempted) {
		t.Fatal("restart reclaimed order", err)
	}
}
