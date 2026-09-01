package strategy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestValidAIProposalQuoteReferenceAcceptsExactSideSpecificEvidence(t *testing.T) {
	now := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, side, basis string
	}{
		{name: "buy ask", side: "BUY", basis: "ASK"},
		{name: "sell bid", side: "SELL", basis: "BID"},
		{name: "buy mark fallback", side: "BUY", basis: "MARK_FALLBACK"},
		{name: "sell mark fallback", side: "SELL", basis: "MARK_FALLBACK"},
		{name: "buy last fallback", side: "BUY", basis: "LAST_FALLBACK"},
		{name: "sell last fallback", side: "SELL", basis: "LAST_FALLBACK"},
	} {
		t.Run(test.name, func(t *testing.T) {
			price := "101.2500000000"
			action := risk.ProposedAction{Instrument: "SPY", Side: test.side, EstimatedPrice: &price}
			reference := AIProposalQuoteReference{
				Symbol: "SPY", Side: test.side, Price: price, Basis: test.basis,
				Provider: "schwab", Feed: "schwab_market_data", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-time.Second),
			}
			if !validAIProposalQuoteReference(reference, action, now) {
				t.Fatalf("exact quote reference was rejected: %#v", reference)
			}
		})
	}
}

func TestValidAIProposalQuoteReferenceFailsClosedOnInconsistentEvidence(t *testing.T) {
	now := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	price := "101.2500000000"
	action := risk.ProposedAction{Instrument: "SPY", Side: "BUY", EstimatedPrice: &price}
	valid := AIProposalQuoteReference{
		Symbol: "SPY", Side: "BUY", Price: price, Basis: "ASK",
		Provider: "schwab", Feed: "schwab_market_data", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-time.Second),
	}

	tests := map[string]func(*AIProposalQuoteReference){
		"wrong symbol":     func(reference *AIProposalQuoteReference) { reference.Symbol = "QQQ" },
		"wrong side":       func(reference *AIProposalQuoteReference) { reference.Side = "SELL" },
		"wrong side basis": func(reference *AIProposalQuoteReference) { reference.Basis = "BID" },
		"wrong price":      func(reference *AIProposalQuoteReference) { reference.Price = "101.25" },
		"negative zero":    func(reference *AIProposalQuoteReference) { reference.Price = "-0.0000" },
		"missing provider": func(reference *AIProposalQuoteReference) { reference.Provider = "" },
		"future timestamp": func(reference *AIProposalQuoteReference) { reference.ObservedAt = now.Add(2 * time.Minute) },
		"stale timestamp":  func(reference *AIProposalQuoteReference) { reference.ObservedAt = now.Add(-16 * time.Minute) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			reference := valid
			mutate(&reference)
			if validAIProposalQuoteReference(reference, action, now) {
				t.Fatalf("inconsistent quote reference was accepted: %#v", reference)
			}
		})
	}
}

func TestValidAIProposalQuoteReferenceEvidenceRequiresExactSavedCopy(t *testing.T) {
	now := time.Date(2026, 9, 1, 19, 0, 0, 0, time.UTC)
	price := "101.2500000000"
	action := risk.ProposedAction{Instrument: "SPY", Side: "BUY", EstimatedPrice: &price}
	reference := AIProposalQuoteReference{
		Symbol: "SPY", Side: "BUY", Price: price, Basis: "ASK",
		Provider: "schwab", Feed: "schwab_market_data", Quality: "BROKER_REALTIME", ObservedAt: now.Add(-time.Second),
	}
	rationale, err := json.Marshal(map[string]any{"decision": "PROPOSE", "quote_reference": reference})
	if err != nil {
		t.Fatal(err)
	}
	decision := Decision{ProposedAction: &action, QuoteReference: &reference, Source: "AI", Rationale: rationale}
	if !validAIProposalQuoteReferenceEvidence(decision, now) {
		t.Fatal("exact saved quote reference was rejected")
	}

	var saved map[string]any
	if err = json.Unmarshal(rationale, &saved); err != nil {
		t.Fatal(err)
	}
	savedReference := saved["quote_reference"].(map[string]any)
	savedReference["basis"] = "LAST_FALLBACK"
	decision.Rationale, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if validAIProposalQuoteReferenceEvidence(decision, now) {
		t.Fatal("a rationale copy inconsistent with the committed quote reference was accepted")
	}
}
