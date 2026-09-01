package strategy

import (
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func validAIProposalQuoteReferenceEvidence(decision Decision, evaluatedAt time.Time) bool {
	if decision.ProposedAction == nil || decision.QuoteReference == nil || evaluatedAt.IsZero() {
		return false
	}
	if !validAIProposalQuoteReference(*decision.QuoteReference, *decision.ProposedAction, evaluatedAt) {
		return false
	}
	var saved struct {
		QuoteReference *AIProposalQuoteReference `json:"quote_reference"`
	}
	if json.Unmarshal(decision.Rationale, &saved) != nil || saved.QuoteReference == nil {
		return false
	}
	return sameAIProposalQuoteReference(*decision.QuoteReference, *saved.QuoteReference)
}

func validAIProposalQuoteReference(reference AIProposalQuoteReference, action risk.ProposedAction, evaluatedAt time.Time) bool {
	if action.EstimatedPrice == nil || reference.Symbol != action.Instrument || reference.Symbol != strings.ToUpper(strings.TrimSpace(reference.Symbol)) || reference.Side != action.Side {
		return false
	}
	if !plainAIQuoteDecimal(reference.Price) || strings.HasPrefix(reference.Price, "-") || reference.Price != *action.EstimatedPrice {
		return false
	}
	price, ok := new(big.Rat).SetString(reference.Price)
	if !ok || price.Sign() <= 0 || !freshMarketTimestamp(reference.ObservedAt, evaluatedAt) {
		return false
	}
	for _, value := range []string{reference.Provider, reference.Feed, reference.Quality} {
		if value == "" || value != strings.TrimSpace(value) || len(value) > 80 {
			return false
		}
	}
	allowedBasis := map[string]bool{"MARK_FALLBACK": true, "LAST_FALLBACK": true}
	switch reference.Side {
	case "BUY":
		allowedBasis["ASK"] = true
	case "SELL":
		allowedBasis["BID"] = true
	default:
		return false
	}
	return allowedBasis[reference.Basis]
}

func sameAIProposalQuoteReference(left, right AIProposalQuoteReference) bool {
	return left.Symbol == right.Symbol &&
		left.Side == right.Side &&
		left.Price == right.Price &&
		left.Basis == right.Basis &&
		left.Provider == right.Provider &&
		left.Feed == right.Feed &&
		left.Quality == right.Quality &&
		left.ObservedAt.Equal(right.ObservedAt)
}
