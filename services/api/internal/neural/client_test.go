package neural

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAnalyzeSendsBoundedFieldsAndNormalizesInsight(t *testing.T) {
	const secret = "secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/internal/neural/insight" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["credential"] != secret || body["profile"] != "fast" || body["model"] != "" || body["prompt"] != "Explain diversification" || body["safety_identifier"] != strings.Repeat("a", 64) {
			t.Fatalf("unexpected request fields: %#v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"insight":{"summary":"Diversification reduces concentration risk.","key_points":["Spread exposure."],"risk_flags":["Loss remains possible."],"limitations":["No live data."],"requires_current_data":false,"metadata":{"provider":"openai","model":"gpt-5.6-luna","profile":"fast","input_usage":30,"output_usage":45,"request_id":"resp-safe","latency_ms":120}}}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	result, err := client.Analyze(context.Background(), "openai", "fast", []byte(secret), "Explain diversification", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary == "" || result.Metadata.Profile != "fast" || result.Metadata.RequestID != "resp-safe" || result.Metadata.InputUsage == nil || *result.Metadata.InputUsage != 30 {
		t.Fatalf("unexpected normalized insight: %#v", result)
	}
}

func TestVerifyKeepsRequestBodyUntilTransportReadsIt(t *testing.T) {
	const secret = "secret-value"
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), secret) {
			t.Fatalf("credential was cleared before the request body was sent: %q", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"valid":true,"provider":"openai"}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	if err := client.Verify(context.Background(), "openai", []byte(secret)); err != nil {
		t.Fatal(err)
	}
}

func TestProposeTradeSendsOnlyBoundedNormalizedFacts(t *testing.T) {
	const secret = "secret-value"
	observedAt := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/internal/neural/trade-proposal" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["credential"] != secret || body["provider"] != "openai" || body["profile"] != "core" || body["symbol"] != "BTC" || body["side"] != "BUY" || body["max_size"] != "50" || body["max_size_unit"] != "USD" || body["available_cash"] != "200" || body["position_quantity"] != "0.012" || body["position_available_quantity"] != "0.01" || body["observed_at"] != observedAt.Format(time.RFC3339Nano) || body["safety_identifier"] != strings.Repeat("a", 64) {
			t.Fatalf("unexpected proposal request fields: %#v", body)
		}
		if _, exposed := body["account_id"]; exposed {
			t.Fatalf("account identifier escaped the API trust boundary: %#v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"proposal":{"decision":"PROPOSE","requested_size":"25.50","confidence":"LOW","thesis":"Bounded proposal.","risk_flags":["Volatility"],"limitations":["No news feed"],"metadata":{"provider":"openai","model":"gpt-5.6-terra","profile":"core","request_id":"resp-proposal"}}}`)),
		}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	proposal, err := client.ProposeTrade(context.Background(), "openai", []byte(secret), TradeProposalRequest{
		Profile: "core", Objective: "Keep risk low.", Symbol: "BTC", Side: "BUY", MaxSize: "50", MaxSizeUnit: "USD",
		AvailableCash: "200", PositionQuantity: "0.012", PositionAvailableQuantity: "0.01", ObservedAt: observedAt,
	}, strings.Repeat("a", 64))
	if err != nil || proposal.Decision != "PROPOSE" || proposal.Metadata.RequestID != "resp-proposal" {
		t.Fatalf("unexpected proposal response: %#v %v", proposal, err)
	}
}

func TestProposeShadowExcludesAccountIdentifiersAndNormalizesDecision(t *testing.T) {
	observedAt := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/internal/neural/shadow-decision" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, exposed := body["account_id"]; exposed || body["max_proposal_notional"] != "1" || body["objective"] != "Preserve capital." {
			t.Fatalf("unsafe or incomplete shadow request: %#v", body)
		}
		markets, ok := body["markets"].([]any)
		if !ok || len(markets) != 1 || markets[0].(map[string]any)["symbol"] != "BTC" || markets[0].(map[string]any)["change_percent_1h"] != "1.25" || markets[0].(map[string]any)["history_status"] != "COMPLETE" {
			t.Fatalf("normalized markets missing: %#v", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"decision":{"decision":"ABSTAIN","symbol":"NONE","side":"NONE","proposed_notional":"0","confidence":"LOW","thesis":"No cautious edge.","risk_flags":["Volatility"],"limitations":["No news"],"metadata":{"provider":"openai","model":"gpt-5.6-sol","profile":"deep","request_id":"resp-shadow"}}}`))}, nil
	})
	client := NewHTTPClient("http://ai.internal", "internal-token", &http.Client{Transport: transport})
	decision, err := client.ProposeShadow(context.Background(), "openai", []byte("secret-value"), ShadowDecisionRequest{Profile: "deep", Objective: "Preserve capital.", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "1", AvailableCashUSD: "100", BuyingPowerUSD: "100", Positions: []ShadowPositionFact{}, Markets: []ShadowMarketFact{{Symbol: "BTC", AssetClass: "CRYPTO", Currency: "USD", Bid: "99", Ask: "101", Mark: "100", Last: "100", ChangePercent1H: "1.25", Feed: "exchange", Quality: "REAL_TIME_SINGLE_VENUE", ObservedAt: observedAt, HistoryStatus: "COMPLETE", HistoryGranularitySeconds: 900, HistoryContiguousIntervals: 96, HistoryExpectedIntervals: 96, HistoryFeed: "rest_candles", HistoryQuality: "REAL_TIME_SINGLE_VENUE", HistoryObservedAt: &observedAt}}, ObservedAt: observedAt}, strings.Repeat("a", 64))
	if err != nil || decision.Decision != "ABSTAIN" || decision.Metadata.RequestID != "resp-shadow" {
		t.Fatalf("unexpected shadow response: %#v %v", decision, err)
	}
}
