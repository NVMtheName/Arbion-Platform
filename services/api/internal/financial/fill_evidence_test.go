package financial

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testFillEvidence() FillEvidence {
	now := time.Date(2026, 9, 1, 0, 0, 0, 123456789, time.UTC)
	return FillEvidence{AccountReference: strings.Repeat("a", 64), EntryReference: strings.Repeat("b", 64), TradeReference: strings.Repeat("c", 64), OrderReference: strings.Repeat("d", 64), SequenceTime: now, CommissionCurrencyStatus: "UNAVAILABLE", Fill: TradeFill{ProductID: "BTC-USD", BaseAsset: "BTC", QuoteCurrency: "USD", Side: "BUY", Price: "60000.000", Size: "0.0000100", SizeUnit: "BTC", Commission: Money{Amount: "0.00100"}, TradeTime: now, Liquidity: "MAKER"}}
}

func TestFillEvidenceCanonicalMoneyIdentityAndPrivacy(t *testing.T) {
	e := testFillEvidence()
	now := e.SequenceTime.Add(time.Hour)
	one, err := FillEvidenceDigest(e, now)
	if err != nil {
		t.Fatal(err)
	}
	e.Fill.Price, e.Fill.Size, e.Fill.Commission.Amount = "60000", "0.00001", "0.001"
	two, err := FillEvidenceDigest(e, now.Add(time.Hour))
	if err != nil || one != two {
		t.Fatal("equivalent decimals changed identity", err)
	}
	e.Fill.Commission.Amount = "0.002"
	three, err := FillEvidenceDigest(e, now)
	if err != nil || three == one {
		t.Fatal("changed fee did not change evidence", err)
	}
	for _, value := range []any{e, FillEvidencePage{Fills: []FillEvidence{e}}, FillEvidenceScan{Page: FillEvidencePage{Fills: []FillEvidence{e}}}} {
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != "{}" {
			t.Fatal("private evidence escaped JSON", string(encoded), err)
		}
	}
	a, err := CorrelationReference("coinbase", "portfolio:a", "entry", "entry-one")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := CorrelationReference("coinbase", "portfolio:b", "entry", "entry-one")
	c, _ := CorrelationReference("schwab", "portfolio:a", "entry", "entry-one")
	d, _ := CorrelationReference("coinbase", "portfolio:a", "order", "entry-one")
	if a == b || a == c || a == d || len(a) != 64 {
		t.Fatal("identity is not account/provider/type scoped")
	}
}

func TestFillEvidenceRejectsIncompleteAndInexactFacts(t *testing.T) {
	for name, mutate := range map[string]func(*FillEvidence){
		"missing identity":   func(e *FillEvidence) { e.EntryReference = "" },
		"missing account":    func(e *FillEvidence) { e.AccountReference = "" },
		"wrong unit":         func(e *FillEvidence) { e.Fill.SizeUnit = "ETH" },
		"wrong fee currency": func(e *FillEvidence) { e.Fill.Commission.Currency = "USDC" },
		"wrong product":      func(e *FillEvidence) { e.Fill.ProductID = "ETH-USD" },
		"negative fee":       func(e *FillEvidence) { e.Fill.Commission.Amount = "-0.01" },
		"missing fee":        func(e *FillEvidence) { e.Fill.Commission.Amount = "" },
		"zero size":          func(e *FillEvidence) { e.Fill.Size = "0.00" },
		"exponent":           func(e *FillEvidence) { e.Fill.Price = "1e5" },
		"too precise":        func(e *FillEvidence) { e.Fill.Size = Decimal("0." + strings.Repeat("1", 33)) },
		"future trade":       func(e *FillEvidence) { e.Fill.TradeTime = e.SequenceTime.Add(2 * time.Hour) },
		"missing sequence":   func(e *FillEvidence) { e.SequenceTime = time.Time{} },
		"inferred fee unit":  func(e *FillEvidence) { e.Fill.Commission.Currency = e.Fill.QuoteCurrency },
	} {
		t.Run(name, func(t *testing.T) {
			e := testFillEvidence()
			now := e.SequenceTime.Add(time.Hour)
			mutate(&e)
			if _, err := NormalizeFillEvidence(e, now); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
