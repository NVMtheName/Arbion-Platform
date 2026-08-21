package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strconv"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

type MarketIntelligence interface {
	Sources() []marketintelligence.Source
	LatestEquityQuote(context.Context, string) (marketintelligence.QuoteObservation, bool, error)
	TopCryptoMarkets(context.Context, string, int) ([]marketintelligence.CryptoMarketObservation, bool, error)
	RecentInsiderFilings(context.Context, string, int) ([]marketintelligence.InsiderFilingObservation, bool, error)
}

func (h *authHandler) listMarketSources(writer stdhttp.ResponseWriter, _ *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	sources := h.marketSources
	if h.markets != nil {
		sources = h.markets.Sources()
	}
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"sources":                  sources,
		"live_execution_available": false,
	})
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
