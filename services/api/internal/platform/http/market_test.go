package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
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

type fakeBrokerMarketData struct {
	accounts []financial.FinancialAccount
	quote    financial.Quote
	chain    financial.OptionChain
	query    financial.OptionChainRequest
}

func (fake *fakeBrokerMarketData) ListAccounts(context.Context, authorization.Principal) ([]financial.FinancialAccount, error) {
	return fake.accounts, nil
}

func (fake *fakeBrokerMarketData) GetAccount(_ context.Context, _ authorization.Principal, id string) (financial.FinancialAccount, error) {
	for _, account := range fake.accounts {
		if account.ID == id {
			return account, nil
		}
	}
	return financial.FinancialAccount{}, errors.New("not found")
}

func (fake *fakeBrokerMarketData) GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error) {
	return fake.quote, nil
}

func (fake *fakeBrokerMarketData) GetOptionChain(_ context.Context, _ authorization.Principal, _ string, query financial.OptionChainRequest) (financial.OptionChain, error) {
	fake.query = query
	return fake.chain, nil
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
	if len(response.Sources) != 8 || response.LiveExecutionAvailable {
		t.Fatalf("unexpected market source response: %+v", response)
	}
	for _, source := range response.Sources {
		if source.Enabled || source.Healthy {
			t.Fatalf("unwired source exposed as available: %+v", source)
		}
	}
}

func TestMarketSourcesExposeOnlyTheCurrentUsersActiveSchwabSource(t *testing.T) {
	broker := &fakeBrokerMarketData{accounts: []financial.FinancialAccount{{ID: "account-1", Provider: "schwab", Status: "active"}}}
	handler := &authHandler{marketSources: marketintelligence.DefaultSources(), marketFinancial: broker}
	recorder := httptest.NewRecorder()
	handler.listMarketSources(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/markets/sources", nil))
	if recorder.Code != stdhttp.StatusOK || !strings.Contains(recorder.Body.String(), `"id":"schwab_broker_market_data"`) {
		t.Fatalf("Schwab source missing: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"enabled":true,"healthy":true`) {
		t.Fatalf("active user source not marked available: %s", recorder.Body.String())
	}
}

func TestBrokerMarketRoutesNormalizeReadOnlyAccountScopedObservations(t *testing.T) {
	now := time.Now().UTC()
	realtime, delayed := true, false
	bid, ask, underlying, strike := financial.Decimal("100.10"), financial.Decimal("100.20"), financial.Decimal("100.15"), financial.Decimal("95")
	broker := &fakeBrokerMarketData{
		accounts: []financial.FinancialAccount{{ID: "account-1", Provider: "schwab", BaseCurrency: "USD", Status: "active"}},
		quote:    financial.Quote{Symbol: "SPY", Bid: &bid, Ask: &ask, ProviderTimestamp: now.Add(-time.Second), Realtime: &realtime},
		chain: financial.OptionChain{
			Symbol: "SPY", UnderlyingPrice: &underlying, ProviderTimestamp: now.Add(-time.Second), Delayed: &delayed,
			Contracts: []financial.OptionContract{{Symbol: "SPY   261016P00095000", Underlying: "SPY", PutCall: "PUT", Expiration: "2026-10-16", Strike: strike, Bid: &bid, ProviderTimestamp: now.Add(-time.Second)}},
		},
	}
	handler := &authHandler{marketFinancial: broker}

	quoteRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/account-1/markets/equities/SPY/quote", nil)
	quoteRequest.SetPathValue("id", "account-1")
	quoteRequest.SetPathValue("symbol", "SPY")
	quoteRecorder := httptest.NewRecorder()
	handler.brokerEquityQuote(quoteRecorder, quoteRequest)
	if quoteRecorder.Code != stdhttp.StatusOK || !strings.Contains(quoteRecorder.Body.String(), `"provider":"schwab"`) || !strings.Contains(quoteRecorder.Body.String(), `"live_execution_available":false`) {
		t.Fatalf("broker quote was not normalized safely: status=%d body=%s", quoteRecorder.Code, quoteRecorder.Body.String())
	}

	optionRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/accounts/account-1/markets/options?symbol=SPY&contract_type=PUT&strike_count=12", nil)
	optionRequest.SetPathValue("id", "account-1")
	optionRecorder := httptest.NewRecorder()
	handler.brokerOptionChain(optionRecorder, optionRequest)
	if optionRecorder.Code != stdhttp.StatusOK || broker.query.Symbol != "SPY" || broker.query.StrikeCount != 12 || !strings.Contains(optionRecorder.Body.String(), `"quality":"REAL_TIME_CONSOLIDATED"`) {
		t.Fatalf("broker option chain was not normalized safely: status=%d query=%+v body=%s", optionRecorder.Code, broker.query, optionRecorder.Body.String())
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
