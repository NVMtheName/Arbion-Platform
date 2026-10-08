package neural

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strings"
)

// The neutral decision grammar is shared, not the request path or provenance.
// A saved Paper/Shadow result cannot supply a fresh LIVE-purpose provider call.
type LivePilotDecisionRequest = ShadowDecisionRequest
type LivePilotDecision = ShadowDecision

type LivePilotDecisionClient interface {
	ProposeLivePilotDecision(context.Context, string, []byte, LivePilotDecisionRequest, string) (LivePilotDecision, error)
}

var livePilotSymbol = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
var livePilotDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})(\.[0-9]{1,32})?$`)
var livePilotNotional = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)

func liveAmount(raw string, positive bool) (*big.Rat, bool) {
	if !livePilotDecimal.MatchString(raw) {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(raw)
	return n, ok && (!positive || n.Sign() > 0)
}

// ValidateLivePilotDecisionRequest accepts only a single normalized spot/USD
// pilot snapshot. Current authorization, provenance and quote freshness remain
// the caller's deterministic responsibility; this adapter grants no authority.
func ValidateLivePilotDecisionRequest(in LivePilotDecisionRequest) error {
	bad := &ProviderError{Code: InvalidRequest}
	if strings.TrimSpace(in.Objective) == "" || len(in.Objective) > 2000 || in.ObservedAt.IsZero() ||
		len(in.AllowedSymbols) != 1 || !livePilotSymbol.MatchString(in.AllowedSymbols[0]) || in.AllowedSymbols[0] == "USD" ||
		len(in.Markets) != 1 || len(in.Positions) > 1 || len(in.MarketEvents) != 0 || len(in.MarketEventCoverage) != 0 || len(in.RecentDecisions) != 0 ||
		!livePilotNotional.MatchString(in.MaxProposalNotional) {
		return bad
	}
	if _, ok := liveAmount(in.MaxProposalNotional, true); !ok {
		return bad
	}
	cash, ok := liveAmount(in.AvailableCashUSD, false)
	if !ok {
		return bad
	}
	power, ok := liveAmount(in.BuyingPowerUSD, false)
	if !ok || power.Cmp(cash) != 0 {
		return bad
	} // No margin or whole-account buying power.
	m := in.Markets[0]
	if m.Symbol != in.AllowedSymbols[0] || m.AssetClass != "CRYPTO" || m.Currency != "USD" || m.ObservedAt.IsZero() || m.ObservedAt.After(in.ObservedAt) ||
		strings.TrimSpace(m.Feed) == "" || len(m.Feed) > 64 || strings.TrimSpace(m.Quality) == "" || len(m.Quality) > 64 {
		return bad
	}
	bid, bidOK := liveAmount(m.Bid, true)
	ask, askOK := liveAmount(m.Ask, true)
	if !bidOK || !askOK || ask.Cmp(bid) < 0 {
		return bad
	}
	// Initial pilot facts intentionally exclude inferred history, performance,
	// depth, issuer prose and non-live decision memory.
	if m.Mark != "" || m.Last != "" || m.ChangePercent1H != "" || m.ChangePercent6H != "" || m.ChangePercent24H != "" || m.Volume24H != "" ||
		m.HistoryStatus != "UNAVAILABLE" || m.HistoryGranularitySeconds != 0 || m.HistoryContiguousIntervals != 0 || m.HistoryExpectedIntervals != 0 ||
		m.HistoryFeed != "" || m.HistoryQuality != "" || m.HistoryObservedAt != nil ||
		(m.LiquidityStatus != "" && m.LiquidityStatus != "UNAVAILABLE") || m.SpreadBPS != "" || m.BidDepthUSD != "" || m.AskDepthUSD != "" ||
		m.BidLevels != 0 || m.AskLevels != 0 || m.LiquidityFeed != "" || m.LiquidityQuality != "" || m.LiquidityObservedAt != nil {
		return bad
	}
	for _, p := range in.Positions {
		quantity, qOK := liveAmount(p.Quantity, true)
		available, aOK := liveAmount(p.AvailableQuantity, false)
		_, valueOK := liveAmount(p.MarketValueUSD, false)
		if p.Symbol != in.AllowedSymbols[0] || p.Instrument != "CRYPTO" || !qOK || !aOK || available.Cmp(quantity) > 0 || !valueOK ||
			p.PerformanceStatus != "UNAVAILABLE" || p.AveragePriceUSD != "" || p.CurrentPriceUSD != "" || p.DayProfitLossUSD != "" || p.DayProfitLossPercent != "" ||
			p.OpenProfitLossUSD != "" || p.OpenProfitLossPercent != "" || p.PriceBasis != "" {
			return bad
		}
	}
	return nil
}

func ValidateLivePilotDecision(out LivePilotDecision, in LivePilotDecisionRequest) error {
	bad := &ProviderError{Code: DecisionContractInvalid}
	if len(in.AllowedSymbols) != 1 || !livePilotNotional.MatchString(out.ProposedNotional) ||
		(out.Confidence != "LOW" && out.Confidence != "MEDIUM" && out.Confidence != "HIGH") ||
		strings.TrimSpace(out.Thesis) == "" || len(out.Thesis) > 1000 {
		return bad
	}
	for _, values := range [][]string{out.RiskFlags, out.Limitations} {
		if values == nil || len(values) > 8 {
			return bad
		}
		seen := map[string]bool{}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 500 || seen[value] {
				return bad
			}
			seen[value] = true
		}
	}
	amount, amountOK := liveAmount(out.ProposedNotional, false)
	maximum, maxOK := liveAmount(in.MaxProposalNotional, true)
	if !amountOK || !maxOK {
		return bad
	}
	if out.Decision == "ABSTAIN" {
		if out.Symbol != "NONE" || out.Side != "NONE" || out.ProposedNotional != "0" {
			return bad
		}
		return nil
	}
	if out.Decision != "PROPOSE" || out.Symbol != in.AllowedSymbols[0] || (out.Side != "BUY" && out.Side != "SELL") || amount.Sign() <= 0 || amount.Cmp(maximum) > 0 {
		return bad
	}
	return nil
}

func (c *HTTPClient) ProposeLivePilotDecision(ctx context.Context, provider string, credential []byte, input LivePilotDecisionRequest, safetyIdentifier string) (LivePilotDecision, error) {
	if provider != "openai" {
		return LivePilotDecision{}, &ProviderError{Code: Unsupported}
	}
	if err := ValidateLivePilotDecisionRequest(input); err != nil {
		return LivePilotDecision{}, err
	}
	if input.Profile != "fast" && input.Profile != "core" && input.Profile != "deep" {
		return LivePilotDecision{}, &ProviderError{Code: InvalidRequest}
	}
	input.Positions = append([]ShadowPositionFact{}, input.Positions...)
	request := shadowDecisionPayload(provider, credential, input, safetyIdentifier)
	var raw json.RawMessage
	if err := c.callMode(ctx, "/internal/neural/live-pilot-decision", request, &raw, true); err != nil {
		return LivePilotDecision{}, err
	}
	return decodeLivePilotDecision(raw, input)
}

func decodeLivePilotDecision(raw []byte, input LivePilotDecisionRequest) (LivePilotDecision, error) {
	bad := &ProviderError{Code: DecisionContractInvalid}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueJSONValue(decoder, 0); err != nil {
		return LivePilotDecision{}, bad
	}
	if _, err := decoder.Token(); err != io.EOF {
		return LivePilotDecision{}, bad
	}
	envelope, ok := liveObject(raw, []string{"purpose", "decision"}, nil)
	if !ok || string(envelope["purpose"]) != `"LIVE_PILOT"` {
		return LivePilotDecision{}, bad
	}
	decision, ok := liveObject(envelope["decision"], []string{"decision", "symbol", "side", "proposed_notional", "confidence", "thesis", "risk_flags", "limitations", "metadata"}, nil)
	if !ok {
		return LivePilotDecision{}, bad
	}
	if _, ok := liveObject(decision["metadata"], []string{"provider", "model", "profile"}, []string{"input_usage", "output_usage", "request_id", "latency_ms"}); !ok {
		return LivePilotDecision{}, bad
	}
	var out LivePilotDecision
	if json.Unmarshal(envelope["decision"], &out) != nil {
		return LivePilotDecision{}, bad
	}
	if err := ValidateLivePilotDecision(out, input); err != nil {
		return LivePilotDecision{}, err
	}
	return out, nil
}

func liveObject(raw []byte, required, optional []string) (map[string]json.RawMessage, bool) {
	var out map[string]json.RawMessage
	if json.Unmarshal(raw, &out) != nil || out == nil {
		return nil, false
	}
	allowed := map[string]bool{}
	for _, name := range required {
		if _, ok := out[name]; !ok {
			return nil, false
		}
		allowed[name] = true
	}
	for _, name := range optional {
		allowed[name] = true
	}
	for name := range out {
		if !allowed[name] {
			return nil, false
		}
	}
	return out, true
}

// JSON's last-key-wins behavior is not acceptable for fresh financial intent.
// The bounded transport already limits bytes; bound recursion separately.
func uniqueJSONValue(d *json.Decoder, depth int) error {
	bad := &ProviderError{Code: DecisionContractInvalid}
	if depth > 16 {
		return bad
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return bad
			}
			seen[name] = true
			if err := uniqueJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return bad
	}
	_, err = d.Token()
	return err
}
