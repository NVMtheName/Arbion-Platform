package executionsim

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func cancellation(version uint64) Event {
	v := fixtureEvent("cancel-confirmed", ConfirmCancel, version)
	v.SimulatedOrderID = "sim-order-a"
	v.TerminalSettlement = &SettlementTotals{FilledQuantity: "1", GrossNotional: "40", Fees: "0.2"}
	return v
}

func pendingCancel(t *testing.T) *Engine {
	t.Helper()
	e := acknowledged(t)
	apply(t, e, filling())
	apply(t, e, fixtureEvent("cancel-requested", RequestCancel, 5))
	return e
}

func TestCancellationRequiresExactTerminalQuantityGrossAndFees(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Event)
		err  error
	}{
		{"missing-witness", func(v *Event) { v.TerminalSettlement = nil }, ErrSettlement},
		{"missing-quantity", func(v *Event) { v.TerminalSettlement.FilledQuantity = "" }, ErrSettlement},
		{"missing-gross", func(v *Event) { v.TerminalSettlement.GrossNotional = "" }, ErrSettlement},
		{"missing-fees", func(v *Event) { v.TerminalSettlement.Fees = "" }, ErrSettlement},
		{"unseen-fill", func(v *Event) { v.TerminalSettlement.FilledQuantity = "1.5" }, ErrSettlement},
		{"underreported-fill", func(v *Event) { v.TerminalSettlement.FilledQuantity = "0.5" }, ErrSettlement},
		{"gross-mismatch", func(v *Event) { v.TerminalSettlement.GrossNotional = "40.0000000001" }, ErrSettlement},
		{"fee-mismatch", func(v *Event) { v.TerminalSettlement.Fees = "0.2000000001" }, ErrSettlement},
		{"negative-fee", func(v *Event) { v.TerminalSettlement.Fees = "-0.2" }, ErrSettlement},
		{"exponent", func(v *Event) { v.TerminalSettlement.GrossNotional = "4e1" }, ErrSettlement},
		{"excess-precision", func(v *Event) { v.TerminalSettlement.Fees = "0.20000000001" }, ErrSettlement},
		{"foreign-order", func(v *Event) { v.SimulatedOrderID = "other" }, ErrTransition},
		{"foreign-account", func(v *Event) { v.Scope.Account = "other" }, ErrInvalid},
		{"foreign-owner", func(v *Event) { v.Scope.Owner = "other" }, ErrInvalid},
		{"foreign-provider", func(v *Event) { v.Scope.Provider = "schwab" }, ErrInvalid},
		{"foreign-run", func(v *Event) { v.Scope.Run = "other" }, ErrInvalid},
		{"mixed-transaction", func(v *Event) { v.TransactionID = "unrelated" }, ErrInvalid},
		{"mixed-deposit", func(v *Event) { v.Amount = "20" }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pendingCancel(t)
			v := cancellation(6)
			tc.edit(&v)
			unchanged(t, e, v, tc.err)
			if e.Snapshot().ReservedCash != "44.8000000000" {
				t.Fatal("terminal mismatch released buy cash")
			}
			// A rejected input did not consume its identity or revision.
			apply(t, e, cancellation(6))
		})
	}
}

func TestTerminalMatchingUsesExactDecimalsAndDoesNotSettleAgain(t *testing.T) {
	e := pendingCancel(t)
	before := e.Snapshot()
	v := cancellation(6)
	v.TerminalSettlement = &SettlementTotals{FilledQuantity: "1.0000000000", GrossNotional: "40.000", Fees: "0.2000"}
	apply(t, e, v)
	after := e.Snapshot()
	o := after.Orders["order-a"]
	if after.Cash != before.Cash || after.BuySpent != before.BuySpent || !reflect.DeepEqual(after.Positions, before.Positions) || after.MatchedTransactions != before.MatchedTransactions || after.ReservedCash != zero() || o.State != "CANCELLED" || o.FilledNotional != "40.0000000000" || o.FeesPaid != "0.2000000000" {
		t.Fatal("terminal witness settled money or lost exact totals", after)
	}
	if applied, err := e.Apply(v, fixtureTime); applied || err != nil || !reflect.DeepEqual(after, e.Snapshot()) {
		t.Fatal("terminal redelivery changed settled state", applied, err)
	}
	// Same delivery identity with different terminal facts is a conflict.
	v.TerminalSettlement.Fees = "0.3"
	unchanged(t, e, v, ErrConflict)
	// The caller's mutation cannot change the stored identity/projection.
	v.TerminalSettlement.Fees = "0.2000"
	if applied, err := e.Apply(v, fixtureTime); applied || err != nil {
		t.Fatal("caller mutation changed saved terminal witness", applied, err)
	}
}

func TestUnknownRejectionRequiresExplicitZeroSettlement(t *testing.T) {
	for _, totals := range []*SettlementTotals{
		nil, {},
		{FilledQuantity: "1", GrossNotional: "40", Fees: "0.2"},
		{FilledQuantity: "0", GrossNotional: "0.0000000001", Fees: "0"},
		{FilledQuantity: "0", GrossNotional: "0", Fees: "0.0000000001"},
	} {
		e := fixtureEngine(t)
		apply(t, e, opening())
		apply(t, e, fixtureEvent("send", Send, 2))
		v := fixtureEvent("reject", Reject, 3)
		v.TerminalSettlement = totals
		unchanged(t, e, v, ErrSettlement)
		if s := e.Snapshot(); s.ReservedCash != "85.0000000000" || s.Orders["order-a"].State != "OUTCOME_UNKNOWN" {
			t.Fatal("unknown attempt was treated as zero settlement", s)
		}
		v.TerminalSettlement = &SettlementTotals{FilledQuantity: "0.0", GrossNotional: "0.0000", Fees: "0"}
		apply(t, e, v)
		if e.Snapshot().ReservedCash != zero() {
			t.Fatal("exact zero rejection did not release the claim")
		}
	}
}

func TestZeroFillCancellationAndUnrelatedEvents(t *testing.T) {
	e := acknowledged(t)
	apply(t, e, fixtureEvent("cancel", RequestCancel, 4))
	v := cancellation(5)
	v.TerminalSettlement = &SettlementTotals{FilledQuantity: "0", GrossNotional: "0", Fees: "0"}
	apply(t, e, v)
	if e.Snapshot().Cash != "1000.0000000000" || e.Snapshot().ReservedCash != zero() {
		t.Fatal("zero-fill cancellation changed cash")
	}
	for _, original := range []Event{opening(), filling(), fixtureEvent("send", Send, 2)} {
		e := fixtureEngine(t)
		if original.Kind == Fill {
			e = acknowledged(t)
		} else if original.Kind == Send {
			apply(t, e, opening())
		}
		original.TerminalSettlement = &SettlementTotals{FilledQuantity: "0", GrossNotional: "0", Fees: "0"}
		unchanged(t, e, original, ErrInvalid)
	}
}

func TestSaleCancellationRetainsQuantityUntilGrossAndFeesMatch(t *testing.T) {
	e := acknowledged(t)
	v := filling()
	v.Quantity, v.Fee = "2", "0.4"
	apply(t, e, v)
	v = opening()
	v.ID, v.OrderID, v.Side = "sale", "sale", "SELL"
	apply(t, e, v)
	v = fixtureEvent("sale-send", Send, 2)
	v.OrderID = "sale"
	apply(t, e, v)
	v = fixtureEvent("sale-ack", Acknowledge, 3)
	v.OrderID, v.SimulatedOrderID = "sale", "sim-sale"
	apply(t, e, v)
	v = filling()
	v.ID, v.OrderID, v.SimulatedOrderID, v.TransactionID, v.Price, v.Fee = "sale-fill", "sale", "sim-sale", "sale-trade", "42", "0.21"
	apply(t, e, v)
	v = fixtureEvent("sale-cancel", RequestCancel, 5)
	v.OrderID = "sale"
	apply(t, e, v)
	v = cancellation(6)
	v.OrderID, v.SimulatedOrderID = "sale", "sim-sale"
	// Matching quantity alone is insufficient: buy-side price/fee totals
	// are not interchangeable with this sale's settled evidence.
	unchanged(t, e, v, ErrSettlement)
	before := e.Snapshot()
	if before.Orders["sale"].ReservedQuantity != "1.0000000000" {
		t.Fatal("sale units released early")
	}
	v.TerminalSettlement = &SettlementTotals{FilledQuantity: "1", GrossNotional: "42", Fees: "0.21"}
	apply(t, e, v)
	after := e.Snapshot()
	if after.Orders["sale"].ReservedQuantity != zero() || before.Cash != after.Cash || !reflect.DeepEqual(before.Positions, after.Positions) {
		t.Fatal("sale terminal witness moved money or units", after)
	}
}

func pendingCancelJournal(t *testing.T, path string) *Journal {
	t.Helper()
	j := openTestJournal(t, path)
	appendTest(t, j, opening())
	appendTest(t, j, fixtureEvent("send", Send, 2))
	v := fixtureEvent("ack", Acknowledge, 3)
	v.SimulatedOrderID = "sim-order-a"
	appendTest(t, j, v)
	appendTest(t, j, filling())
	appendTest(t, j, fixtureEvent("cancel-request", RequestCancel, 5))
	return j
}

func TestPrematureTerminalEvidenceRetainsClaimAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.jsonl")
	j := pendingCancelJournal(t, path)
	before := snapshotTest(t, j)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v := cancellation(6)
	v.TerminalSettlement = &SettlementTotals{FilledQuantity: "1.5", GrossNotional: "60", Fees: "0.3"}
	if applied, err := j.Append(v, fixtureTime); applied || !errors.Is(err, ErrSettlement) {
		t.Fatal("premature terminal witness accepted", applied, err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("rejected terminal input changed accepted journal")
	}
	j = openTestJournal(t, path)
	if !reflect.DeepEqual(before, snapshotTest(t, j)) {
		t.Fatal("restart released unmatched terminal claim")
	}
	late := filling()
	late.ID, late.TransactionID, late.OrderVersion, late.Quantity, late.Fee = "late", "late-trade", 6, "0.5", "0.1"
	appendTest(t, j, late)
	v.OrderVersion = 7
	appendTest(t, j, v)
	settled := snapshotTest(t, j)
	if settled.Cash != "939.7000000000" || settled.ReservedCash != zero() || settled.Orders["order-a"].FeesPaid != "0.3000000000" {
		t.Fatal(settled)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = openTestJournal(t, path)
	late.ID = "late-alias-after-terminal"
	for _, duplicate := range []Event{late, v} {
		if applied, err := j.Append(duplicate, fixtureTime); applied || err != nil || !reflect.DeepEqual(settled, snapshotTest(t, j)) {
			t.Fatal("restart replay duplicated settlement", applied, err)
		}
	}
}

func TestTerminalWitnessConcurrentDeliveryAndFailedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.jsonl")
	j := pendingCancelJournal(t, path)
	before := snapshotTest(t, j)
	// No state publication after a storage failure, even if terminal totals
	// match. Reopening must recover the still-reserved durable state.
	if err := j.file.Close(); err != nil {
		t.Fatal(err)
	}
	if applied, err := j.Append(cancellation(6), fixtureTime); applied || !errors.Is(err, ErrJournal) {
		t.Fatal(applied, err)
	}
	if !reflect.DeepEqual(before, j.engine.Snapshot()) {
		t.Fatal("failed terminal write published a release")
	}
	j = openTestJournal(t, path)
	if !reflect.DeepEqual(before, snapshotTest(t, j)) {
		t.Fatal("failed terminal write released durable reservation")
	}
	var wg sync.WaitGroup
	results := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, err := j.Append(cancellation(6), fixtureTime)
			if err != nil {
				t.Error(err)
			}
			results <- applied
		}()
	}
	wg.Wait()
	close(results)
	count := 0
	for applied := range results {
		if applied {
			count++
		}
	}
	after := snapshotTest(t, j)
	if count != 1 || after.Cash != before.Cash || after.ReservedCash != zero() || after.AppliedEvents != before.AppliedEvents+1 {
		t.Fatal("concurrent terminal delivery duplicated settlement", count, after)
	}
}

func TestLegacyTerminalWithoutWitnessFailsReplayWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jsonl")
	j := pendingCancelJournal(t, path)
	v := cancellation(6)
	v.TerminalSettlement = nil
	// Deliberately construct the old, hash-valid fictional record. Never
	// backfill inferred zero totals or overwrite the original evidence.
	if err := j.persist(record{Event: &v}); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenJournal(path, fixtureConfig(), fixtureTime); !errors.Is(err, ErrJournal) {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatal("legacy terminal replay inferred missing totals", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("legacy evidence was rewritten")
	}
}
