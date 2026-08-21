package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

type fakeMarketIntelligence struct {
	sources []marketintelligence.Source
	err     error
}

func (fake fakeMarketIntelligence) Sources() []marketintelligence.Source { return fake.sources }
func (fake fakeMarketIntelligence) LatestEquityQuote(_ context.Context, symbol string) (marketintelligence.QuoteObservation, bool, error) {
	if fake.err != nil {
		return marketintelligence.QuoteObservation{}, false, fake.err
	}
	bid := marketintelligence.Decimal("101.25")
	return marketintelligence.QuoteObservation{Symbol: symbol, AssetClass: marketintelligence.Equity, Currency: "USD", Bid: &bid}, true, nil
}
func (fake fakeMarketIntelligence) TopCryptoMarkets(_ context.Context, currency string, _ int) ([]marketintelligence.CryptoMarketObservation, bool, error) {
	if fake.err != nil {
		return nil, false, fake.err
	}
	return []marketintelligence.CryptoMarketObservation{{ID: "bitcoin", Symbol: "BTC", Currency: currency, CurrentPrice: marketintelligence.Decimal("60000")}}, false, nil
}
func (fake fakeMarketIntelligence) RecentInsiderFilings(_ context.Context, cik string, _ int) ([]marketintelligence.InsiderFilingObservation, bool, error) {
	if fake.err != nil {
		return nil, false, fake.err
	}
	return []marketintelligence.InsiderFilingObservation{{IssuerCIK: cik, Form: "4"}}, false, nil
}

func TestMarketSourcesAreNoStoreReadOnlyMetadata(t *testing.T) {
	handler := &authHandler{marketSources: marketintelligence.DefaultSources()}
	recorder := httptest.NewRecorder()
	handler.listMarketSources(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/sources", nil))

	var response struct {
		Sources                []marketintelligence.Source `json:"sources"`
		LiveExecutionAvailable bool                        `json:"live_execution_available"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response metadata: status=%d cache=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if len(response.Sources) != 6 || response.LiveExecutionAvailable {
		t.Fatalf("unexpected market source response: %+v", response)
	}
	for _, source := range response.Sources {
		if source.Enabled || source.Healthy {
			t.Fatalf("unwired source exposed as available: %+v", source)
		}
	}
}

func TestMarketObservationRoutesAreNoStoreAndReadOnly(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{}}
	for name, testCase := range map[string]struct {
		request *stdhttp.Request
		call    func(stdhttp.ResponseWriter, *stdhttp.Request)
	}{
		"equity":  {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/equities/AAPL/quote", nil), handler.latestEquityQuote},
		"crypto":  {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto?currency=usd&limit=8", nil), handler.topCryptoMarkets},
		"filings": {httptest.NewRequest(stdhttp.MethodGet, "/api/markets/insiders/0000320193?limit=10", nil), handler.recentInsiderFilings},
	} {
		t.Run(name, func(t *testing.T) {
			switch name {
			case "equity":
				testCase.request.SetPathValue("symbol", "AAPL")
			case "filings":
				testCase.request.SetPathValue("cik", "0000320193")
			}
			recorder := httptest.NewRecorder()
			testCase.call(recorder, testCase.request)
			if recorder.Code != stdhttp.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected market response: status=%d cache=%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"live_execution_available":false`) {
				t.Fatalf("market boundary missing from response: %s", recorder.Body.String())
			}
		})
	}
}

func TestMarketRoutesFailClosedWithoutConfiguredSource(t *testing.T) {
	handler := &authHandler{markets: fakeMarketIntelligence{err: marketintelligence.ErrNoEligibleSource}}
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto", nil)
	recorder := httptest.NewRecorder()
	handler.topCryptoMarkets(recorder, request)
	if recorder.Code != stdhttp.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "MARKET_SOURCE_UNAVAILABLE") {
		t.Fatalf("unconfigured source did not fail closed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequest(stdhttp.MethodGet, "/api/markets/crypto?limit=1000", nil)
	recorder = httptest.NewRecorder()
	handler.topCryptoMarkets(recorder, request)
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("unbounded market query was accepted: status=%d", recorder.Code)
	}
}
