package executionsim

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

var fixtureTime = time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)

func fixtureConfig() Config {
	return Config{Scope: Scope{SimulationOnly: true, Owner: "owner-a", Account: "account-a", Provider: "coinbase", Run: "test-run"}, StartedAt: fixtureTime.Add(-time.Hour), StartingCash: "1000", CashReserve: "200", OrderCashCeiling: "110", BuySpendCeiling: "250", Symbols: []string{"XRP"}}
}

func fixtureEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := New(fixtureConfig(), fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func fixtureEvent(id, kind string, version uint64) Event {
	return Event{Scope: fixtureConfig().Scope, ID: id, Kind: kind, At: fixtureTime, OrderID: "order-a", OrderVersion: version}
}
func opening() Event {
	v := fixtureEvent("open", OpenOrder, 1)
	v.Symbol, v.Side, v.Quantity, v.Price, v.CashCeiling = "XRP", "BUY", "2", "40", "85"
	return v
}
func apply(t *testing.T, e *Engine, v Event) {
	t.Helper()
	ok, err := e.Apply(v, fixtureTime)
	if err != nil || !ok {
		t.Fatalf("apply %s: %v %v", v.Kind, ok, err)
	}
}
func acknowledged(t *testing.T) *Engine {
	t.Helper()
	e := fixtureEngine(t)
	apply(t, e, opening())
	apply(t, e, fixtureEvent("send", Send, 2))
	v := fixtureEvent("ack", Acknowledge, 3)
	v.SimulatedOrderID = "sim-order-a"
	apply(t, e, v)
	return e
}
func filling() Event {
	v := fixtureEvent("fill", Fill, 4)
	v.SimulatedOrderID, v.TransactionID, v.Quantity, v.Price, v.Fee = "sim-order-a", "trade-a", "1", "40", "0.2"
	return v
}
func unchanged(t *testing.T, e *Engine, v Event, want error) {
	t.Helper()
	before := e.Snapshot()
	ok, err := e.Apply(v, fixtureTime)
	if ok || !errors.Is(err, want) || !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatalf("expected atomic %v for %s, got %v %v", want, v.Kind, ok, err)
	}
}

func TestPartialFillCancellationAndLateFill(t *testing.T) {
	e := acknowledged(t)
	apply(t, e, filling())
	s := e.Snapshot()
	if s.Cash != "959.8000000000" || s.ReservedCash != "44.8000000000" || s.Positions["XRP"] != "1.0000000000" || s.Orders["order-a"].State != "PARTIALLY_FILLED" {
		t.Fatal(s)
	}
	apply(t, e, fixtureEvent("cancel", RequestCancel, 5))
	if e.Snapshot().ReservedCash != s.ReservedCash {
		t.Fatal("cancel request released cash before confirmation")
	}
	v := filling()
	v.ID, v.TransactionID, v.OrderVersion, v.Quantity, v.Fee = "late-fill", "trade-b", 6, "0.5", "0.1"
	apply(t, e, v)
	if e.Snapshot().Orders["order-a"].State != "CANCEL_PENDING" {
		t.Fatal("late fill forgot pending cancellation")
	}
	v = fixtureEvent("cancelled", ConfirmCancel, 7)
	v.SimulatedOrderID = "sim-order-a"
	apply(t, e, v)
	s = e.Snapshot()
	if s.Cash != "939.7000000000" || s.ReservedCash != zero() || s.Positions["XRP"] != "1.5000000000" || s.Orders["order-a"].State != "CANCELLED" {
		t.Fatal(s)
	}
	v = filling()
	v.ID, v.TransactionID, v.OrderVersion = "after-terminal", "trade-c", 8
	unchanged(t, e, v, ErrTransition)
}

func TestFullFillWinsCancellationRace(t *testing.T) {
	e := acknowledged(t)
	apply(t, e, fixtureEvent("cancel", RequestCancel, 4))
	v := filling()
	v.OrderVersion, v.Quantity, v.Fee = 5, "2", "0.4"
	apply(t, e, v)
	s := e.Snapshot()
	if s.Orders["order-a"].State != "FILLED" || s.ReservedCash != zero() || s.Cash != "919.6000000000" {
		t.Fatal(s)
	}
	v = fixtureEvent("cancelled", ConfirmCancel, 6)
	v.SimulatedOrderID = "sim-order-a"
	unchanged(t, e, v, ErrTransition)
}

func TestDuplicateDeliveriesMatchEconomicIdentity(t *testing.T) {
	e := acknowledged(t)
	v := filling()
	apply(t, e, v)
	before := e.Snapshot()
	for _, id := range []string{"fill", "another-delivery", "another-delivery"} {
		v.ID = id
		ok, err := e.Apply(v, fixtureTime)
		if ok || err != nil || !reflect.DeepEqual(before, e.Snapshot()) {
			t.Fatal("duplicate changed ledger", ok, err)
		}
	}
	v.Fee = "0.3"
	unchanged(t, e, v, ErrConflict)
	v.ID = "new-delivery"
	unchanged(t, e, v, ErrConflict)
	v = filling()
	v.ID, v.TransactionID = "another-delivery", "different-trade"
	unchanged(t, e, v, ErrConflict)
}

func TestUnknownOutcomeCannotResendOrReleaseReservation(t *testing.T) {
	e := fixtureEngine(t)
	apply(t, e, opening())
	v := fixtureEvent("send", Send, 2)
	apply(t, e, v)
	if s := e.Snapshot(); s.Orders["order-a"].State != "OUTCOME_UNKNOWN" || s.ReservedCash != "85.0000000000" {
		t.Fatal(s)
	}
	unchanged(t, e, fixtureEvent("send-again", Send, 3), ErrTransition)
	unchanged(t, e, fixtureEvent("cancel-unknown", RequestCancel, 3), ErrTransition)
	apply(t, e, fixtureEvent("reject", Reject, 3))
	if s := e.Snapshot(); s.ReservedCash != zero() || s.Cash != "1000.0000000000" {
		t.Fatal(s)
	}
}

func TestFillFailuresAreAtomic(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Event)
		err    error
	}{
		{"overfill", func(v *Event) { v.Quantity = "3" }, ErrLimits},
		{"limit-price", func(v *Event) { v.Price = "41" }, ErrLimits},
		{"fee-budget", func(v *Event) { v.Fee = "50" }, ErrLimits},
		{"nonexact-product", func(v *Event) { v.Quantity = "0.0000000001"; v.Price = "0.0000000001" }, ErrInvalid},
		{"negative-fee", func(v *Event) { v.Fee = "-1" }, ErrTransition},
		{"exponent", func(v *Event) { v.Quantity = "1e0" }, ErrTransition},
		{"too-precise", func(v *Event) { v.Price = "40.00000000001" }, ErrTransition},
		{"not-a-number", func(v *Event) { v.Quantity = "NaN" }, ErrTransition},
		{"foreign-order", func(v *Event) { v.SimulatedOrderID = "sim-other" }, ErrTransition},
		{"unknown-order", func(v *Event) { v.OrderID = "other" }, ErrTransition},
		{"gap", func(v *Event) { v.OrderVersion = 5 }, ErrTransition},
		{"old-revision", func(v *Event) { v.OrderVersion = 3 }, ErrTransition},
		{"old-time", func(v *Event) { v.At = fixtureTime.Add(-time.Second) }, ErrTransition},
		{"future-time", func(v *Event) { v.At = fixtureTime.Add(time.Second) }, ErrInvalid},
		{"missing-transaction", func(v *Event) { v.TransactionID = "" }, ErrTransition},
		{"mixed-cash-fact", func(v *Event) { v.Amount = "25" }, ErrInvalid},
		{"foreign-account", func(v *Event) { v.Scope.Account = "account-b" }, ErrInvalid},
		{"foreign-owner", func(v *Event) { v.Scope.Owner = "owner-b" }, ErrInvalid},
		{"foreign-provider", func(v *Event) { v.Scope.Provider = "schwab" }, ErrInvalid},
		{"foreign-run", func(v *Event) { v.Scope.Run = "run-b" }, ErrInvalid},
		{"not-simulation", func(v *Event) { v.Scope.SimulationOnly = false }, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { e := acknowledged(t); v := filling(); tt.mutate(&v); unchanged(t, e, v, tt.err) })
	}
}

func TestCashMatchingKeepsAuthorityAndPendingClaims(t *testing.T) {
	e := acknowledged(t)
	v := Event{Scope: fixtureConfig().Scope, ID: "withdraw", Kind: Withdrawal, TransactionID: "withdraw-a", At: fixtureTime, Amount: "750"}
	apply(t, e, v)
	s := e.Snapshot()
	if s.Cash != "250.0000000000" || s.ReservedCash != "85.0000000000" || !s.FundingReviewRequired {
		t.Fatal(s)
	}
	other := opening()
	other.ID, other.OrderID = "other", "other"
	unchanged(t, e, other, ErrLimits)
	v.ID, v.Kind, v.TransactionID, v.Amount = "deposit", Deposit, "deposit-a", "100"
	apply(t, e, v)
	s = e.Snapshot()
	if s.Cash != "350.0000000000" || s.FundingReviewRequired {
		t.Fatal(s)
	}
	v.ID = "deposit-duplicate"
	ok, err := e.Apply(v, fixtureTime)
	if err != nil || ok || e.Snapshot().Cash != s.Cash {
		t.Fatal("deposit applied twice")
	}
	v.Amount = "101"
	unchanged(t, e, v, ErrConflict)
	// Top ups do not raise the configured per-order or cumulative authority.
	v.ID, v.TransactionID, v.Amount = "large-topup", "large-topup", "10000"
	apply(t, e, v)
	for _, id := range []string{"second", "third"} {
		other.ID, other.OrderID = id, id
		if id == "second" {
			apply(t, e, other)
		} else {
			unchanged(t, e, other, ErrLimits)
		}
	}
	if e.config.BuySpendCeiling != "250" || e.config.OrderCashCeiling != "110" {
		t.Fatal("transfer changed authority")
	}
}

func TestSellReservationsPreventDoubleUseAndOversell(t *testing.T) {
	e := acknowledged(t)
	v := filling()
	v.Quantity, v.Fee = "2", "0.4"
	apply(t, e, v)
	sale := opening()
	sale.ID, sale.OrderID, sale.Side, sale.Quantity = "sale", "sale", "SELL", "1.5"
	apply(t, e, sale)
	second := sale
	second.ID, second.OrderID, second.Quantity = "sale-2", "sale-2", "1"
	unchanged(t, e, second, ErrLimits)
	send := fixtureEvent("sale-send", Send, 2)
	send.OrderID = "sale"
	apply(t, e, send)
	ack := fixtureEvent("sale-ack", Acknowledge, 3)
	ack.OrderID, ack.SimulatedOrderID = "sale", "sim-sale"
	apply(t, e, ack)
	fill := filling()
	fill.ID, fill.OrderID, fill.SimulatedOrderID, fill.TransactionID, fill.Quantity, fill.Price, fill.Fee = "sale-fill", "sale", "sim-sale", "sale-trade", "1", "42", "0.21"
	apply(t, e, fill)
	s := e.Snapshot()
	if s.Cash != "961.3900000000" || s.Positions["XRP"] != "1.0000000000" || s.Orders["sale"].ReservedQuantity != "0.5000000000" {
		t.Fatal(s)
	}
	fill.ID, fill.TransactionID, fill.OrderVersion, fill.Quantity = "sale-overfill", "sale-overfill", 5, "1"
	unchanged(t, e, fill, ErrLimits)
	fill.Quantity, fill.Price = "0.5", "39"
	unchanged(t, e, fill, ErrLimits)
}

func TestInputConfigurationAndSnapshotsAreIsolated(t *testing.T) {
	c := fixtureConfig()
	e, err := New(c, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	c.Symbols[0] = "BTC"
	apply(t, e, opening())
	s := e.Snapshot()
	delete(s.Orders, "order-a")
	s.Positions["XRP"] = "100"
	if len(e.Snapshot().Orders) != 1 || len(e.Snapshot().Positions) != 0 {
		t.Fatal("snapshot mutated engine")
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.Scope.SimulationOnly = false }, func(c *Config) { c.Scope.Provider = "live" }, func(c *Config) { c.StartingCash = "-1" }, func(c *Config) { c.CashReserve = "1001" }, func(c *Config) { c.Symbols = []string{"XRP", "XRP"} }, func(c *Config) { c.StartedAt = fixtureTime.Add(time.Second) }} {
		c := fixtureConfig()
		mutate(&c)
		if _, err := New(c, fixtureTime); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid config accepted", err)
		}
	}
}
