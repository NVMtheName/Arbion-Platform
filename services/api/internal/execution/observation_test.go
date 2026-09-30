package execution

import (
	"testing"
	"time"
)

func observationFixture() (ConfirmedSubmission, Attempt, BrokerObservation, time.Time) {
	o, a, f, terminal, now := reconciliationFixture()
	o.RequestDigest, _ = requestDigest(o.Request)
	o.CreatedAt = a.ClaimedAt.Add(-time.Second)
	p := providerPreflightFixture(o, o.Request.AccountID, a.ClaimedAt)
	s := ConfirmedSubmission{Order: o, PortfolioID: p.PortfolioID, PreviewID: p.PreviewID, Preflight: p}
	a.OrderID, a.ClientOrderID, a.RequestDigest = o.ID, o.Request.ClientOrderID, o.RequestDigest
	a.AuthorizationID, a.CredentialGeneration, a.ExpiresAt = o.Request.CapitalBucketID, 1, a.ClaimedAt.Add(5*time.Second)
	f.ProviderEvidence = &FillProviderEvidence{EntryID: "entry-1", SequenceAt: f.TradedAt.Add(-time.Millisecond), Size: "24", SizeInQuote: true, FeeCurrency: "USD", FeeCurrencyBasis: "COINBASE_ADVANCED_QUOTE_ASSET_1_91"}
	b := BrokerObservation{BrokerIdentity: f.BrokerIdentity, Status: terminal.Status, CompleteFills: true, Totals: Totals{1, f.BaseQuantity, f.GrossUSD, f.FeeUSD}, Fills: []Fill{f}, StartedAt: a.ClaimedAt, ObservedAt: now}
	return s, a, b, now
}

func TestBrokerObservationRequiresCompleteExactBoundedEvidence(t *testing.T) {
	s, a, b, now := observationFixture()
	if err := ValidateBrokerObservation(s, a, b, now); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"OPEN", "PENDING", "QUEUED", "CANCEL_QUEUED", "EDIT_QUEUED", "EXPIRED", "CANCELLED"} {
		v := b
		v.Status = status
		if err := ValidateBrokerObservation(s, a, v, now); err != nil {
			t.Fatal(status, err)
		}
	}
	for name, mutate := range map[string]func(*BrokerObservation){
		"wrong order":         func(v *BrokerObservation) { v.OrderID = s.Order.Request.OwnerID },
		"incomplete":          func(v *BrokerObservation) { v.CompleteFills = false },
		"missing fills":       func(v *BrokerObservation) { v.Fills = nil },
		"count":               func(v *BrokerObservation) { v.FillCount++ },
		"quantity":            func(v *BrokerObservation) { v.BaseQuantity = "0.000400000000000001" },
		"fees":                func(v *BrokerObservation) { v.FeeUSD = "0.240000000000000001" },
		"gross":               func(v *BrokerObservation) { v.GrossUSD = "24.0000000000000000001" },
		"false filled":        func(v *BrokerObservation) { v.Status = "FILLED" },
		"rejected with fills": func(v *BrokerObservation) { v.Status = "REJECTED" },
		"unknown status":      func(v *BrokerObservation) { v.Status = "UNKNOWN" },
		"duplicate":           func(v *BrokerObservation) { v.Fills = append(v.Fills, v.Fills[0]) },
		"duplicate entry":     func(v *BrokerObservation) { f := v.Fills[0]; f.TradeID = "other"; v.Fills = append(v.Fills, f) },
		"future":              func(v *BrokerObservation) { v.ObservedAt = now.Add(time.Nanosecond) },
		"before claim":        func(v *BrokerObservation) { v.StartedAt = a.ClaimedAt.Add(-time.Nanosecond) },
		"negative duration":   func(v *BrokerObservation) { v.StartedAt = now.Add(time.Nanosecond) },
		"oversized":           func(v *BrokerObservation) { v.Fills = make([]Fill, 1001) },
		"mixed scan":          func(v *BrokerObservation) { v.Fills[0].ObservedAt = now.Add(-time.Nanosecond) },
		"missing provenance":  func(v *BrokerObservation) { v.Fills[0].ProviderEvidence = nil },
		"wrong unit":          func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.SizeInQuote = false },
		"wrong currency":      func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.FeeCurrency = "BTC" },
		"unknown fee basis":   func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.FeeCurrencyBasis = "ASSUMED" },
		"future sequence":     func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.SequenceAt = now.Add(time.Nanosecond) },
		"missing sequence":    func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.SequenceAt = time.Time{} },
		"bad entry":           func(v *BrokerObservation) { v.Fills[0].ProviderEvidence.EntryID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			v := b
			v.Fills = append([]Fill(nil), b.Fills...)
			e := *v.Fills[0].ProviderEvidence
			v.Fills[0].ProviderEvidence = &e
			mutate(&v)
			if ValidateBrokerObservation(s, a, v, now) == nil {
				t.Fatal("unprovable collection accepted")
			}
		})
	}
	long := b
	long.ObservedAt = long.StartedAt.Add(15*time.Second + time.Nanosecond)
	long.Fills = append([]Fill(nil), b.Fills...)
	long.Fills[0].ObservedAt = long.ObservedAt
	if ValidateBrokerObservation(s, a, long, long.ObservedAt) == nil {
		t.Fatal("unbounded scan accepted")
	}
}

func TestProviderProvenanceAndTerminalObservationReplay(t *testing.T) {
	s, a, b, now := observationFixture()
	f, err := normalizeFill(s.Order, a, b.Fills[0], now)
	if err != nil {
		t.Fatal(err)
	}
	copy := f
	e := *f.ProviderEvidence
	e.Size = "24.000"
	copy.ProviderEvidence = &e
	copy, err = normalizeFill(s.Order, a, copy, now)
	if err != nil || !sameFill(f, copy) {
		t.Fatal("canonical provenance replay failed", err)
	}
	copy.ProviderEvidence.EntryID = "changed"
	if sameFill(f, copy) {
		t.Fatal("changed source identity ignored")
	}
	terminal := b.terminal()
	for _, delta := range []time.Duration{-time.Millisecond, time.Millisecond} {
		replay := terminal
		replay.CompletedAt, replay.ObservedAt = now.Add(delta), now.Add(delta)
		if _, err := normalizeTerminal(s.Order, a, replay, now.Add(time.Second)); err != nil || !sameTerminal(terminal, replay) {
			t.Fatal("poll completion order changed exact terminal replay", err)
		}
	}
	bad := terminal
	bad.CompletedAt = bad.CompletedAt.Add(-time.Nanosecond)
	if _, err := normalizeTerminal(s.Order, a, bad, now); err == nil {
		t.Fatal("invented completion timestamp accepted")
	}
	legacy := terminal
	legacy.CompletionTimeBasis = ""
	if sameTerminal(legacy, terminal) {
		t.Fatal("changed timestamp meaning ignored")
	}
}
