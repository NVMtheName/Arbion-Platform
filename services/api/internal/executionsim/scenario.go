package executionsim

import (
	"errors"
	"os"
	"reflect"
	"time"
)

// RunScenario creates synthetic Coinbase- or Schwab-labelled evidence. Prices
// are deliberately fictional, not provider quotes. No provider SDK is used.
// The scenario closes/reopens its journal after an uncertain send, after a
// partial fill, and at completion; it proves replay equality and no resend.
func RunScenario(path, provider string, now time.Time) (Snapshot, error) {
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, ErrJournal
	}
	symbol := "XRP"
	if provider == "schwab" {
		symbol = "SPY"
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	config := Config{Scope: Scope{SimulationOnly: true, Owner: "fixture-owner", Account: "fixture-" + provider, Provider: provider, Run: "lifecycle-v1"}, StartedAt: start, StartingCash: "1000", CashReserve: "200", OrderCashCeiling: "110", BuySpendCeiling: "250", Symbols: []string{symbol}}
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
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return ErrJournal
		}
		return nil
	}
	event := func(id, kind, order string, version uint64, minute int) Event {
		return Event{Scope: config.Scope, ID: id, Kind: kind, OrderID: order, OrderVersion: version, At: start.Add(time.Duration(minute) * time.Minute)}
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
	opened := event("buy-open", OpenOrder, "buy", 1, 1)
	opened.Symbol, opened.Side, opened.Quantity, opened.Price, opened.CashCeiling = symbol, "BUY", "2", "40", "85"
	if err = appendEvent(opened); err != nil {
		return Snapshot{}, err
	}
	sent := event("buy-send", Send, "buy", 2, 2)
	if err = appendEvent(sent); err != nil {
		return Snapshot{}, err
	}
	if err = reopen(); err != nil {
		return Snapshot{}, err
	}
	if applied, err := j.Append(sent, now); err != nil || applied {
		return Snapshot{}, ErrConflict
	}
	if _, err := j.Append(event("unsafe-resend", Send, "buy", 3, 3), now); !errors.Is(err, ErrTransition) {
		return Snapshot{}, ErrTransition
	}
	ack := event("buy-ack", Acknowledge, "buy", 3, 3)
	ack.SimulatedOrderID = "fixture-buy"
	if err = appendEvent(ack); err != nil {
		return Snapshot{}, err
	}
	fill := event("buy-fill-1", Fill, "buy", 4, 4)
	fill.SimulatedOrderID, fill.TransactionID, fill.Quantity, fill.Price, fill.Fee = "fixture-buy", "trade-buy-1", "1", "40", "0.2"
	if err = appendEvent(fill); err != nil {
		return Snapshot{}, err
	}
	if err = reopen(); err != nil {
		return Snapshot{}, err
	}
	fill.ID = "buy-fill-redelivery"
	if applied, err := j.Append(fill, now); err != nil || applied {
		return Snapshot{}, ErrConflict
	}
	if err = appendEvent(event("cancel-request", RequestCancel, "buy", 5, 5)); err != nil {
		return Snapshot{}, err
	}
	late := event("buy-fill-2", Fill, "buy", 6, 6)
	late.SimulatedOrderID, late.TransactionID, late.Quantity, late.Price, late.Fee = "fixture-buy", "trade-buy-2", "0.5", "40", "0.1"
	if err = appendEvent(late); err != nil {
		return Snapshot{}, err
	}
	cancel := event("cancel-confirm", ConfirmCancel, "buy", 7, 7)
	cancel.SimulatedOrderID = "fixture-buy"
	if err = appendEvent(cancel); err != nil {
		return Snapshot{}, err
	}
	deposit := event("deposit", Deposit, "", 0, 8)
	deposit.TransactionID, deposit.Amount = "transfer-in", "100"
	if err = appendEvent(deposit); err != nil {
		return Snapshot{}, err
	}
	deposit.ID = "deposit-redelivery"
	if applied, err := j.Append(deposit, now); err != nil || applied {
		return Snapshot{}, ErrConflict
	}
	withdrawal := event("withdrawal", Withdrawal, "", 0, 9)
	withdrawal.TransactionID, withdrawal.Amount = "transfer-out", "25"
	if err = appendEvent(withdrawal); err != nil {
		return Snapshot{}, err
	}
	sale := event("sell-open", OpenOrder, "sell", 1, 10)
	sale.Symbol, sale.Side, sale.Quantity, sale.Price, sale.CashCeiling = symbol, "SELL", "1.5", "40", "65"
	if err = appendEvent(sale); err != nil {
		return Snapshot{}, err
	}
	if err = appendEvent(event("sell-send", Send, "sell", 2, 11)); err != nil {
		return Snapshot{}, err
	}
	ack = event("sell-ack", Acknowledge, "sell", 3, 12)
	ack.SimulatedOrderID = "fixture-sell"
	if err = appendEvent(ack); err != nil {
		return Snapshot{}, err
	}
	for i, q := range []string{"0.5", "1"} {
		id, fee := "sell-fill-1", "0.105"
		if i == 1 {
			id, fee = "sell-fill-2", "0.21"
		}
		v := event(id, Fill, "sell", uint64(4+i), 13+i)
		v.SimulatedOrderID, v.TransactionID, v.Quantity, v.Price, v.Fee = "fixture-sell", id, q, "42", fee
		if err = appendEvent(v); err != nil {
			return Snapshot{}, err
		}
	}
	opened.ID, opened.OrderID, opened.At = "reject-open", "reject", start.Add(15*time.Minute)
	if err = appendEvent(opened); err != nil {
		return Snapshot{}, err
	}
	if err = appendEvent(event("reject-send", Send, "reject", 2, 16)); err != nil {
		return Snapshot{}, err
	}
	if err = appendEvent(event("rejected", Reject, "reject", 3, 17)); err != nil {
		return Snapshot{}, err
	}
	if err = reopen(); err != nil {
		return Snapshot{}, err
	}
	s, err := j.Snapshot()
	if err != nil {
		return Snapshot{}, err
	}
	if s.Cash != "1077.3850000000" || s.BuySpent != "60.3000000000" || s.ReservedCash != zero() || s.Positions[symbol] != zero() || s.MatchedTransactions != 6 || s.AppliedEvents != 17 || s.Orders["buy"].State != "CANCELLED" || s.Orders["sell"].State != "FILLED" || s.Orders["reject"].State != "REJECTED" || s.FundingReviewRequired {
		return Snapshot{}, ErrInvalid
	}
	return s, nil
}
