package neural

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func livePilotInput() LivePilotDecisionRequest {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	return LivePilotDecisionRequest{BudgetScope: "internal-slot-identity", Profile: "deep", Objective: "Consider the exact pilot after costs.", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "24", AvailableCashUSD: "100", BuyingPowerUSD: "100", Positions: []ShadowPositionFact{}, Markets: []ShadowMarketFact{{Symbol: "BTC", AssetClass: "CRYPTO", Currency: "USD", Bid: "59999", Ask: "60001", Feed: "Coinbase", Quality: "LIVE", ObservedAt: now, HistoryStatus: "UNAVAILABLE"}}, ObservedAt: now}
}

const livePilotJSON = `{"purpose":"LIVE_PILOT","decision":{"decision":"ABSTAIN","symbol":"NONE","side":"NONE","proposed_notional":"0","confidence":"LOW","thesis":"Insufficient current evidence after costs.","risk_flags":[],"limitations":["No history"],"metadata":{"provider":"openai","model":"gpt-5.6-sol","profile":"deep","request_id":"synthetic-response"}}}`

func TestLivePilotClientFreshFixedBoundaryAndNoRuntimeIdentifiers(t *testing.T) {
	calls := 0
	c := NewHTTPClient("http://ai.internal", "synthetic-service-token", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/internal/neural/live-pilot-decision" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-service-token" {
			t.Fatal("wrong live transport boundary")
		}
		if r.GetBody != nil || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Fatal("live generation request was made transport-replayable")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		if json.Unmarshal(body, &v) != nil {
			t.Fatal("invalid body")
		}
		if len(v) != 15 || v["credential"] != "synthetic-secret" || v["profile"] != "deep" || v["provider"] != "openai" || v["safety_identifier"] != strings.Repeat("a", 64) {
			t.Fatalf("unexpected transport field set: %d", len(v))
		}
		for _, private := range []string{"internal-slot-identity", "account_id", "financial_credentials", "budget_scope", "mandate_id", "tools", "purpose"} {
			if strings.Contains(string(body), private) {
				t.Fatal("private or authoritative field crossed boundary", private)
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(livePilotJSON))}, nil
	})})
	got, err := c.ProposeLivePilotDecision(context.Background(), "openai", []byte("synthetic-secret"), livePilotInput(), strings.Repeat("a", 64))
	if err != nil || got.Decision != "ABSTAIN" || calls != 1 {
		t.Fatal("fresh request failed", got, err, calls)
	}
}

func TestLivePilotClientNeverFollowsSameOrCrossHostRedirects(t *testing.T) {
	for _, status := range []int{307, 308} {
		for _, crossHost := range []bool{false, true} {
			t.Run(http.StatusText(status)+map[bool]string{false: " same host", true: " cross host"}[crossHost], func(t *testing.T) {
				calls, targetCalls := 0, 0
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					targetCalls++
					w.WriteHeader(200)
					_, _ = io.WriteString(w, livePilotJSON)
				}))
				defer target.Close()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Path == "/redirected" {
						targetCalls++
						_, _ = io.WriteString(w, livePilotJSON)
						return
					}
					location := "/redirected"
					if crossHost {
						location = target.URL + "/redirected"
					}
					w.Header().Set("Location", location)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, livePilotJSON)
				}))
				defer server.Close()
				client := NewHTTPClient(server.URL, "synthetic-token", &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }})
				_, err := client.ProposeLivePilotDecision(context.Background(), "openai", []byte("synthetic-secret"), livePilotInput(), strings.Repeat("a", 64))
				if Code(err) != ProviderUnavailable || calls != 1 || targetCalls != 0 {
					t.Fatal("redirect replayed credential-bearing paid request", err, calls, targetCalls)
				}
			})
		}
	}
}

func TestLivePilotInputIsOnlyIsolatedSinglePairFacts(t *testing.T) {
	for name, mutate := range map[string]func(*LivePilotDecisionRequest){
		"two symbols":                func(v *LivePilotDecisionRequest) { v.AllowedSymbols = []string{"BTC", "ETH"} },
		"fiat":                       func(v *LivePilotDecisionRequest) { v.AllowedSymbols[0] = "USD" },
		"foreign symbol":             func(v *LivePilotDecisionRequest) { v.Markets[0].Symbol = "ETH" },
		"equity":                     func(v *LivePilotDecisionRequest) { v.Markets[0].AssetClass = "EQUITY" },
		"crossed":                    func(v *LivePilotDecisionRequest) { v.Markets[0].Bid = "60002" },
		"missing price":              func(v *LivePilotDecisionRequest) { v.Markets[0].Ask = "" },
		"future quote":               func(v *LivePilotDecisionRequest) { v.Markets[0].ObservedAt = v.ObservedAt.Add(time.Second) },
		"whole account buying power": func(v *LivePilotDecisionRequest) { v.BuyingPowerUSD = "10000" },
		"old shadow memory":          func(v *LivePilotDecisionRequest) { v.RecentDecisions = []ShadowRecentDecision{{Decision: "ABSTAIN"}} },
		"issuer event":               func(v *LivePilotDecisionRequest) { v.MarketEvents = []ShadowMarketEventFact{{Symbol: "BTC"}} },
		"invented history":           func(v *LivePilotDecisionRequest) { v.Markets[0].ChangePercent1H = "1" },
		"invented depth":             func(v *LivePilotDecisionRequest) { v.Markets[0].BidDepthUSD = "10000" },
		"excess objective":           func(v *LivePilotDecisionRequest) { v.Objective = strings.Repeat("x", 2001) },
		"short": func(v *LivePilotDecisionRequest) {
			v.Positions = []ShadowPositionFact{{Symbol: "BTC", Instrument: "CRYPTO", Quantity: "-1", AvailableQuantity: "0", MarketValueUSD: "0", PerformanceStatus: "UNAVAILABLE"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := livePilotInput()
			mutate(&input)
			if ValidateLivePilotDecisionRequest(input) == nil {
				t.Fatal("unsafe normalized facts accepted")
			}
		})
	}
	input := livePilotInput()
	input.Objective = strings.Repeat("x", 2000)
	input.Positions = []ShadowPositionFact{{Symbol: "BTC", Instrument: "CRYPTO", Quantity: "0.0001", AvailableQuantity: "0.0001", MarketValueUSD: "5.9999", PerformanceStatus: "UNAVAILABLE"}}
	if err := ValidateLivePilotDecisionRequest(input); err != nil {
		t.Fatal("valid exact pilot facts rejected", err)
	}
}

func TestLivePilotResponseRejectsPromotionAndAmbiguousOrExpandedOutput(t *testing.T) {
	for name, raw := range map[string]string{
		"shadow envelope":    strings.Replace(livePilotJSON, `"purpose":"LIVE_PILOT",`, "", 1),
		"wrong purpose":      strings.Replace(livePilotJSON, "LIVE_PILOT", "SHADOW", 1),
		"duplicate":          strings.Replace(livePilotJSON, `"decision":"ABSTAIN"`, `"decision":"PROPOSE","decision":"ABSTAIN"`, 1),
		"duplicate envelope": strings.TrimSuffix(livePilotJSON, "}") + `,"purpose":"LIVE_PILOT"}`,
		"authority":          strings.Replace(livePilotJSON, `"symbol":"NONE"`, `"symbol":"NONE","account_id":"private"`, 1),
		"missing":            strings.Replace(livePilotJSON, `"risk_flags":[],`, "", 1),
		"null":               strings.Replace(livePilotJSON, `"risk_flags":[]`, `"risk_flags":null`, 1),
		"bad abstain":        strings.Replace(livePilotJSON, `"side":"NONE"`, `"side":"BUY"`, 1),
		"extra metadata":     strings.Replace(livePilotJSON, `"profile":"deep"`, `"profile":"deep","authorization":true`, 1),
		"case alias":         strings.Replace(livePilotJSON, `"thesis"`, `"Thesis"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeLivePilotDecision([]byte(raw), livePilotInput()); err == nil {
				t.Fatal("ambiguous or non-live output accepted")
			}
		})
	}
}

func TestLivePilotClientDeniesUnsupportedBeforeTransportAndDoesNotRetry(t *testing.T) {
	calls := 0
	c := NewHTTPClient("http://ai.internal", "token", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })})
	if _, err := c.ProposeLivePilotDecision(context.Background(), "anthropic", []byte("synthetic"), livePilotInput(), ""); Code(err) != Unsupported || calls != 0 {
		t.Fatal("unsupported provider reached transport")
	}
	if _, err := c.ProposeLivePilotDecision(context.Background(), "openai", []byte("synthetic"), livePilotInput(), strings.Repeat("a", 64)); Code(err) != Timeout || calls != 1 {
		t.Fatal("timeout retried or leaked", err, calls)
	}
}
