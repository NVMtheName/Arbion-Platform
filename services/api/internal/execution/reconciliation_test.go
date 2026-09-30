package execution

import (
	"errors"
	"testing"
	"time"
)

func reconciliationFixture() (Order, Attempt, Fill, TerminalReport, time.Time) {
	now := time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC)
	o := Order{ID: "66666666-6666-4666-8666-666666666666", Request: requestFixture()}
	a := Attempt{ClaimedAt: now.Add(-time.Second), ProviderOrderID: "77777777-7777-4777-8777-777777777777"}
	r := o.Request
	b := BrokerIdentity{r.OwnerID, o.ID, r.AccountID, r.ConnectionID, r.ClientOrderID, a.ProviderOrderID, r.ProductID, r.Side}
	f := Fill{b, "trade-1", "0.0004", "60000", "24", "0.24", now.Add(-500 * time.Millisecond), now, nil}
	t := TerminalReport{b, "CANCELLED", true, 1, "0.0004", "24", "0.24", now, now, ""}
	return o, a, f, t, now
}

func TestFillValidationAndEconomicReplay(t *testing.T) {
	o, a, f, _, now := reconciliationFixture()
	x, err := normalizeFill(o, a, f, now)
	if err != nil {
		t.Fatal(err)
	}
	f.BaseQuantity = "0.000400"
	f.PriceUSD = "60000.00"
	f.GrossUSD = "24.0"
	f.FeeUSD = "0.240"
	f.ObservedAt = now.Add(time.Second)
	y, err := normalizeFill(o, a, f, now.Add(time.Second))
	if err != nil || !sameFill(x, y) {
		t.Fatalf("equivalent economic fill became new trade: %v", err)
	}
	for name, edit := range map[string]func(*Fill){
		"account": func(f *Fill) { f.AccountID = o.ID }, "broker order": func(f *Fill) { f.ProviderOrderID = o.ID },
		"side": func(f *Fill) { f.Side = "SELL" }, "product": func(f *Fill) { f.ProductID = "ETH-USD" },
		"trade id": func(f *Fill) { f.TradeID = "" }, "negative fee": func(f *Fill) { f.FeeUSD = "-1" },
		"gross":                 func(f *Fill) { f.GrossUSD = "24.000000000000000001" },
		"limit":                 func(f *Fill) { f.PriceUSD = "60001"; f.GrossUSD = "24.0004" },
		"future":                func(f *Fill) { f.ObservedAt = now.Add(time.Minute) },
		"predates attempt":      func(f *Fill) { f.TradedAt = a.ClaimedAt.Add(-time.Microsecond) },
		"observed before trade": func(f *Fill) { f.ObservedAt = a.ClaimedAt },
	} {
		t.Run(name, func(t *testing.T) {
			v := x
			edit(&v)
			if _, e := normalizeFill(o, a, v, now); !errors.Is(e, ErrInvalid) {
				t.Fatalf("invalid fill: %v", e)
			}
		})
	}
	for _, totals := range []Totals{{1, "0.0011", "66", "0.1"}, {1, "0.001", "60", "0.600000000000000001"}, {0, "0", "1", "0"}} {
		if withinBounds(o.Request, totals) {
			t.Fatal("accepted overflowing totals", totals)
		}
	}
}

func TestTerminalRequiresExactCompleteDisposition(t *testing.T) {
	o, a, _, v, now := reconciliationFixture()
	if _, err := normalizeTerminal(o, a, v, now); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"REJECTED", "EXPIRED", "CANCELLED"} {
		zero := v
		zero.Status = status
		zero.FillCount = 0
		zero.BaseQuantity = "0"
		zero.GrossUSD = "0"
		zero.FeeUSD = "0"
		if _, err := normalizeTerminal(o, a, zero, now); err != nil {
			t.Fatal(status, err)
		}
	}
	for name, edit := range map[string]func(*TerminalReport){
		"incomplete":         func(v *TerminalReport) { v.CompleteFills = false },
		"filled but partial": func(v *TerminalReport) { v.Status = "FILLED" },
		"rejected with fill": func(v *TerminalReport) { v.Status = "REJECTED" },
		"open":               func(v *TerminalReport) { v.Status = "OPEN" },
		"future":             func(v *TerminalReport) { v.ObservedAt = now.Add(time.Second) },
		"fee limit":          func(v *TerminalReport) { v.FeeUSD = "0.61" },
	} {
		t.Run(name, func(t *testing.T) {
			x := v
			edit(&x)
			if _, err := normalizeTerminal(o, a, x, now); err == nil {
				t.Fatal("unsafe terminal accepted")
			}
		})
	}
	r := o.Request
	r.Side = "SELL"
	r.MaximumDebitUSD = "0"
	r.FeeAllowanceUSD = "30"
	if withinBounds(r, Totals{1, "0.0004", "24", "25"}) {
		t.Fatal("negative sale proceeds accepted")
	}
}
