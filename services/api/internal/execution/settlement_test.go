package execution

import (
	"testing"
	"time"
)

func settlementEvidenceFixture(side string) (ConfirmedSubmission, Attempt, AccountSettlementEvidence, time.Time) {
	s, a, b, now := observationFixture()
	s.Order.Request.Side = side
	if side == "SELL" {
		s.Order.Request.MaximumDebitUSD = "0"
	}
	s.Order.RequestDigest, _ = requestDigest(s.Order.Request)
	a.RequestDigest = s.Order.RequestDigest
	s.Preflight = providerPreflightFixture(s.Order, s.PortfolioID, a.ClaimedAt)
	s.PreviewID = s.Preflight.PreviewID
	b.Side = side
	b.Fills[0].Side = side
	e := AccountSettlementEvidence{PortfolioID: s.PortfolioID, Observation: b, Complete: true, NoOpenOrders: true, StartedAt: b.StartedAt, CompletedAt: now, CashUSD: "975.76", AvailableCashUSD: "975.76", TotalBase: "1.0004", AvailableBase: "1.0004"}
	if side == "SELL" {
		e.CashUSD, e.AvailableCashUSD, e.TotalBase, e.AvailableBase = "1023.76", "1023.76", "0.9996", "0.9996"
	}
	return s, a, e, now
}

func TestAccountSettlementRequiresExactCashInventoryAndFeeAttribution(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			s, a, e, now := settlementEvidenceFixture(side)
			if err := ValidateAccountSettlementEvidence(s, a, e, now); err != nil {
				t.Fatal(err)
			}
			for name, mutate := range map[string]func(*AccountSettlementEvidence){
				"foreign portfolio":        func(e *AccountSettlementEvidence) { e.PortfolioID = s.Order.Request.OwnerID },
				"incomplete account":       func(e *AccountSettlementEvidence) { e.Complete = false },
				"open orders":              func(e *AccountSettlementEvidence) { e.NoOpenOrders = false },
				"not final":                func(e *AccountSettlementEvidence) { e.Observation.Status = "OPEN" },
				"cash difference":          func(e *AccountSettlementEvidence) { e.CashUSD = "900"; e.AvailableCashUSD = "900" },
				"inventory difference":     func(e *AccountSettlementEvidence) { e.TotalBase = "2"; e.AvailableBase = "2" },
				"cash held":                func(e *AccountSettlementEvidence) { e.AvailableCashUSD = "0" },
				"inventory held":           func(e *AccountSettlementEvidence) { e.AvailableBase = "0" },
				"nondecimal":               func(e *AccountSettlementEvidence) { e.CashUSD = "97576/100" },
				"negative":                 func(e *AccountSettlementEvidence) { e.TotalBase = "-1" },
				"fraction too precise":     func(e *AccountSettlementEvidence) { e.TotalBase = "1.0004000000000000001" },
				"missing fill":             func(e *AccountSettlementEvidence) { e.Observation.Fills = nil },
				"future completion":        func(e *AccountSettlementEvidence) { e.CompletedAt = now.Add(time.Nanosecond) },
				"before order observation": func(e *AccountSettlementEvidence) { e.CompletedAt = now.Add(-time.Nanosecond) },
				"start after observation":  func(e *AccountSettlementEvidence) { e.StartedAt = now },
				"starts before attempt":    func(e *AccountSettlementEvidence) { e.StartedAt = a.ClaimedAt.Add(-time.Nanosecond) },
			} {
				t.Run(name, func(t *testing.T) {
					v := e
					mutate(&v)
					if ValidateAccountSettlementEvidence(s, a, v, now) == nil {
						t.Fatal("unsafe settlement evidence accepted")
					}
				})
			}
			if ValidateAccountSettlementEvidence(s, a, e, now.Add(30*time.Second+time.Nanosecond)) == nil {
				t.Fatal("stale funds accepted")
			}
			long := e
			long.CompletedAt = long.StartedAt.Add(15*time.Second + time.Nanosecond)
			if ValidateAccountSettlementEvidence(s, a, long, long.CompletedAt) == nil {
				t.Fatal("unbounded scan accepted")
			}
			// Zero-fill terminal costs exactly zero, not an estimate/fee allowance.
			zero := e
			zero.Observation.Status = "CANCELLED"
			zero.Observation.Fills = []Fill{}
			zero.Observation.Totals = Totals{0, "0", "0", "0"}
			zero.CashUSD, zero.AvailableCashUSD, zero.TotalBase, zero.AvailableBase = s.Preflight.CashUSD, s.Preflight.CashUSD, s.Preflight.TotalBase, s.Preflight.TotalBase
			if err := ValidateAccountSettlementEvidence(s, a, zero, now); err != nil {
				t.Fatal("zero-fill exact settlement", err)
			}
		})
	}
}
