package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

type MarketIntelligence interface {
	Sources() []marketintelligence.Source
	LatestEquityQuote(context.Context, string) (marketintelligence.QuoteObservation, bool, error)
	TopCryptoMarkets(context.Context, string, int) ([]marketintelligence.CryptoMarketObservation, bool, error)
	RecentInsiderFilings(context.Context, string, int) ([]marketintelligence.InsiderFilingObservation, bool, error)
}

type BrokerMarketData interface {
	ListAccounts(context.Context, authorization.Principal) ([]financial.FinancialAccount, error)
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
	GetQuote(context.Context, authorization.Principal, string, string) (financial.Quote, error)
	GetOptionChain(context.Context, authorization.Principal, string, financial.OptionChainRequest) (financial.OptionChain, error)
}

func (h *authHandler) listMarketSources(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	sources := h.marketSources
	if h.markets != nil {
		sources = h.markets.Sources()
	}
	sources = append([]marketintelligence.Source(nil), sources...)
	if h.marketFinancial != nil {
		accounts, err := h.marketFinancial.ListAccounts(request.Context(), principal(request))
		if err == nil {
			for _, account := range accounts {
				if account.Provider == "schwab" && strings.EqualFold(account.Status, "active") {
					setMarketSourceAvailable(sources, "schwab_broker_market_data")
					break
				}
			}
		}
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"sources":                  sources,
		"live_execution_available": false,
	})
}

func setMarketSourceAvailable(sources []marketintelligence.Source, sourceID string) {
	for index := range sources {
		if sources[index].ID == sourceID {
			sources[index].Enabled = true
			sources[index].Healthy = true
			return
		}
	}
}

func (h *authHandler) latestEquityQuote(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	quote, cached, err := h.markets.LatestEquityQuote(request.Context(), request.PathValue("symbol"))
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"quote": quote, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) topCryptoMarkets(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	currency := request.URL.Query().Get("currency")
	if currency == "" {
		currency = "usd"
	}
	limit, ok := boundedMarketLimit(request.URL.Query().Get("limit"), 8)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	markets, cached, err := h.markets.TopCryptoMarkets(request.Context(), currency, limit)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"markets": markets, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) recentInsiderFilings(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.markets == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	limit, ok := boundedMarketLimit(request.URL.Query().Get("limit"), 10)
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	filings, cached, err := h.markets.RecentInsiderFilings(request.Context(), request.PathValue("cik"), limit)
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"filings": filings, "cached": cached, "live_execution_available": false})
}

func (h *authHandler) brokerEquityQuote(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	quote, err := h.marketFinancial.GetQuote(request.Context(), principal(request), account.ID, request.PathValue("symbol"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	observation, err := marketintelligence.NormalizeBrokerQuote(account.Provider, account.BaseCurrency, quote, time.Now().UTC(), brokerFreshnessPolicy())
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"quote": observation, "live_execution_available": false})
}

func (h *authHandler) brokerOptionChain(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h.marketFinancial == nil {
		h.marketUnavailable(writer, marketintelligence.ErrNoEligibleSource)
		return
	}
	query, ok := brokerOptionQuery(request, time.Now().UTC())
	if !ok {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The option-chain query is invalid.")
		return
	}
	account, err := h.marketFinancial.GetAccount(request.Context(), principal(request), request.PathValue("id"))
	if err != nil {
		h.financialError(writer, err)
		return
	}
	chain, err := h.marketFinancial.GetOptionChain(request.Context(), principal(request), account.ID, query)
	if err != nil {
		h.financialError(writer, err)
		return
	}
	observation, err := marketintelligence.NormalizeBrokerOptionChain(account.Provider, chain, time.Now().UTC(), brokerFreshnessPolicy())
	if err != nil {
		h.marketUnavailable(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{"chain": observation, "live_execution_available": false})
}

func brokerOptionQuery(request *stdhttp.Request, now time.Time) (financial.OptionChainRequest, bool) {
	query := request.URL.Query()
	symbol := strings.ToUpper(strings.TrimSpace(query.Get("symbol")))
	contractType := strings.ToUpper(strings.TrimSpace(query.Get("contract_type")))
	if contractType == "" {
		contractType = "PUT"
	}
	strikeCount, ok := boundedMarketLimit(query.Get("strike_count"), 12)
	if !ok || strikeCount > 25 || symbol == "" || (contractType != "PUT" && contractType != "CALL") {
		return financial.OptionChainRequest{}, false
	}
	fromDate, toDate := startOfUTCDay(now), startOfUTCDay(now).AddDate(0, 0, 60)
	if value := strings.TrimSpace(query.Get("from_date")); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return financial.OptionChainRequest{}, false
		}
		fromDate = parsed
	}
	if value := strings.TrimSpace(query.Get("to_date")); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return financial.OptionChainRequest{}, false
		}
		toDate = parsed
	}
	if toDate.Before(fromDate) || toDate.Sub(fromDate) > 90*24*time.Hour || fromDate.Before(startOfUTCDay(now).AddDate(0, 0, -1)) {
		return financial.OptionChainRequest{}, false
	}
	return financial.OptionChainRequest{Symbol: symbol, ContractType: contractType, StrikeCount: strikeCount, FromDate: fromDate, ToDate: toDate}, true
}

func startOfUTCDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func brokerFreshnessPolicy() marketintelligence.FreshnessPolicy {
	return marketintelligence.FreshnessPolicy{MaxAge: 120 * time.Hour, MaxFutureSkew: 2 * time.Minute}
}

func (h *authHandler) marketUnavailable(writer stdhttp.ResponseWriter, err error) {
	if errors.Is(err, marketintelligence.ErrInvalidObservation) {
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_MARKET_QUERY", "The market query is invalid.")
		return
	}
	if errors.Is(err, marketintelligence.ErrNoEligibleSource) {
		writeError(writer, stdhttp.StatusServiceUnavailable, "MARKET_SOURCE_UNAVAILABLE", "The requested market source is not configured.")
		return
	}
	writeError(writer, stdhttp.StatusBadGateway, "MARKET_DATA_UNAVAILABLE", "The market provider is temporarily unavailable.")
}

func boundedMarketLimit(value string, fallback int) (int, bool) {
	if value == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	return limit, err == nil && limit >= 1 && limit <= 100
}
