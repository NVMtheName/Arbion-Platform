package executionsim

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"
)

// RunBookScenario settles synthetic depth-derived fills through the existing
// durable lifecycle. The book is fictional and stateless; it is not a provider
// snapshot, reusable liquidity pool, exchange adapter, or execution approval.
func RunBookScenario(path, provider string, now time.Time) (Snapshot, error) {
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, ErrJournal
	}
	symbol := "XRP"
	if provider == "schwab" {
		symbol = "SPY"
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	config := Config{Scope: Scope{SimulationOnly: true, Owner: "fixture-owner", Account: "fixture-" + provider, Provider: provider, Run: "depth-sweep-v1"}, StartedAt: start, StartingCash: "1000", CashReserve: "200", OrderCashCeiling: "110", BuySpendCeiling: "250", Symbols: []string{symbol}}
	j, err := OpenJournal(path, config, now)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() {
		if j != nil {
			_ = j.Close()
		}
	}()
	reopen := func() error {
		before, err := j.Snapshot()
		if err != nil {
			return err
		}
		if err = j.Close(); err != nil {
			return err
		}
		j, err = OpenJournal(path, config, now)
		if err != nil {
			return err
		}
		after, err := j.Snapshot()
		if err != nil || !reflect.DeepEqual(before, after) {
			return ErrJournal
		}
		return nil
	}
	appendEvent := func(v Event) error {
		applied, err := j.Append(v, now)
		if err != nil {
			return err
		}
		if !applied {
			return ErrInvalid
		}
		return nil
	}
	for i, side := range []string{"BUY", "SELL"} {
		at := start.Add(time.Duration(i+1) * time.Minute)
		input := BookSweepInput{Source: BookSweepSyntheticSource, Scope: config.Scope, Symbol: symbol, Side: side, LimitPrice: "41", RemainingQuantity: "2.5", Bids: []BookLevel{{Price: "39", Quantity: "0.5"}, {Price: "38", Quantity: "0.5"}}, Asks: []BookLevel{{Price: "40", Quantity: "1"}, {Price: "41", Quantity: "0.5"}, {Price: "42", Quantity: "5"}}, ObservedAt: at, EvaluatedAt: at, MaxAge: time.Second, FeeBasisPoints: 50}
		if side == "SELL" {
			input.LimitPrice, input.RemainingQuantity = "38", "1.5"
		}
		plan, err := SweepBook(input)
		if err != nil {
			return Snapshot{}, err
		}
		if len(plan.Fills) != 2 || (side == "BUY" && plan.StopReason != BookSweepLimitReached) || (side == "SELL" && plan.StopReason != BookSweepDepthExhausted) {
			return Snapshot{}, ErrInvalid
		}
		event := func(id, kind string, version uint64) Event {
			return Event{Scope: config.Scope, ID: side + "-" + id, Kind: kind, OrderID: side, OrderVersion: version, At: at}
		}
		opened := event("open", OpenOrder, 1)
		opened.Symbol, opened.Side, opened.Quantity, opened.Price, opened.CashCeiling = symbol, side, input.RemainingQuantity, input.LimitPrice, "110"
		if err = appendEvent(opened); err != nil {
			return Snapshot{}, err
		}
		if err = appendEvent(event("send", Send, 2)); err != nil {
			return Snapshot{}, err
		}
		ack := event("ack", Acknowledge, 3)
		ack.SimulatedOrderID = "synthetic-" + side
		if err = appendEvent(ack); err != nil {
			return Snapshot{}, err
		}
		for level, f := range plan.Fills {
			v := event(fmt.Sprintf("level-%d", level), Fill, uint64(4+level))
			v.SimulatedOrderID, v.TransactionID = ack.SimulatedOrderID, v.ID
			v.Quantity, v.Price, v.Fee = f.Quantity, f.Price, f.Fee
			if err = appendEvent(v); err != nil {
				return Snapshot{}, err
			}
			if err = reopen(); err != nil {
				return Snapshot{}, err
			}
			// Recomputing the same hypothetical sweep gives the same events,
			// not fresh liquidity or a second financial effect.
			if applied, err := j.Append(v, now); applied || err != nil {
				return Snapshot{}, ErrConflict
			}
		}
		pending, err := j.Snapshot()
		if err != nil {
			return Snapshot{}, err
		}
		order := pending.Orders[side]
		if order.State != "PARTIALLY_FILLED" || order.Filled != plan.FilledQuantity || order.FilledNotional != plan.TotalGrossNotional || order.FeesPaid != plan.TotalFees || (side == "BUY" && order.ReservedCash == zero()) || (side == "SELL" && order.ReservedQuantity != plan.RemainingQuantity) {
			return Snapshot{}, ErrSettlement
		}
		if err = appendEvent(event("cancel-request", RequestCancel, 6)); err != nil {
			return Snapshot{}, err
		}
		cancel := event("cancel-confirm", ConfirmCancel, 7)
		cancel.SimulatedOrderID = ack.SimulatedOrderID
		cancel.TerminalSettlement = &SettlementTotals{FilledQuantity: plan.FilledQuantity, GrossNotional: plan.TotalGrossNotional, Fees: plan.TotalFees}
		if err = appendEvent(cancel); err != nil {
			return Snapshot{}, err
		}
		if err = reopen(); err != nil {
			return Snapshot{}, err
		}
	}
	s, err := j.Snapshot()
	if err != nil {
		return Snapshot{}, err
	}
	if s.Cash != "977.5050000000" || s.BuySpent != "60.8025000000" || s.ReservedCash != zero() || s.Positions[symbol] != "0.5000000000" || s.MatchedTransactions != 4 || s.AppliedEvents != 14 || s.Orders["BUY"].State != "CANCELLED" || s.Orders["SELL"].State != "CANCELLED" || s.FundingReviewRequired {
		return Snapshot{}, ErrInvalid
	}
	return s, nil
}
