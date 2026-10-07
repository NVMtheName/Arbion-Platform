package execution

import (
	"testing"
	"time"
)

func TestPilotAllocationTermsAreExactAndBounded(t *testing.T) {
	r := requestFixture()
	p := PilotAllocation{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID,
		CapitalBucketID: r.CapitalBucketID, ProductID: "BTC-USD", InitialCashUSD: "100",
		Limits: OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if !validPilotAllocation(p) || !samePilotAllocation(p, p) {
		t.Fatal("valid allocation rejected")
	}
	for name, mutate := range map[string]func(*PilotAllocation){
		"missing owner":   func(p *PilotAllocation) { p.OwnerID = "" },
		"missing account": func(p *PilotAllocation) { p.AccountID = "" },
		"missing key":     func(p *PilotAllocation) { p.ConnectionID = "" },
		"missing bucket":  func(p *PilotAllocation) { p.CapitalBucketID = "" },
		"wrong quote":     func(p *PilotAllocation) { p.ProductID = "BTC-USDT" },
		"same currency":   func(p *PilotAllocation) { p.ProductID = "USD-USD" },
		"zero cash":       func(p *PilotAllocation) { p.InitialCashUSD = "0" },
		"negative cash":   func(p *PilotAllocation) { p.InitialCashUSD = "-100" },
		"rounded cash":    func(p *PilotAllocation) { p.InitialCashUSD = "100.0000000000000000001" },
		"oversized cap":   func(p *PilotAllocation) { p.Limits.MaximumOrderUSD = "100.000000000000000001" },
		"missing expiry":  func(p *PilotAllocation) { p.Limits.ExpiresAt = time.Time{} },
		"rounded expiry":  func(p *PilotAllocation) { p.Limits.ExpiresAt = p.Limits.ExpiresAt.Add(time.Nanosecond) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if validPilotAllocation(changed) {
				t.Fatal("invalid allocation accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*PilotAllocation){
		"cash":    func(p *PilotAllocation) { p.InitialCashUSD = "100.0" },
		"cap":     func(p *PilotAllocation) { p.Limits.MaximumOrderUSD = "25.0" },
		"expiry":  func(p *PilotAllocation) { p.Limits.ExpiresAt = p.Limits.ExpiresAt.Add(time.Second) },
		"product": func(p *PilotAllocation) { p.ProductID = "ETH-USD" },
		"bucket":  func(p *PilotAllocation) { p.CapitalBucketID = p.AccountID },
	} {
		t.Run("changed "+name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if samePilotAllocation(p, changed) {
				t.Fatal("changed immutable terms compare equal")
			}
		})
	}
}
