package http

import (
	"errors"
	stdhttp "net/http"
	"sort"
	"time"

	"github.com/arbion/platform/services/api/internal/marketintelligence"
)

type marketWatchlistObservation struct {
	marketintelligence.WatchlistItem
	Observation *marketintelligence.CryptoMarketObservation `json:"observation,omitempty"`
}

func registerMarketWatchlistRoutes(mux *stdhttp.ServeMux, handler *authHandler) {
	if handler.markets == nil {
		return
	}
	mux.Handle("GET /api/markets/watchlist", handler.require(stdhttp.HandlerFunc(handler.listMarketWatchlist)))
	mux.Handle("POST /api/markets/watchlist", handler.require(stdhttp.HandlerFunc(handler.createMarketWatchlistItem)))
	mux.Handle("DELETE /api/markets/watchlist/{id}", handler.require(stdhttp.HandlerFunc(handler.deleteMarketWatchlistItem)))
}

func (handler *authHandler) listMarketWatchlist(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	items, err := handler.markets.ListWatchlist(request.Context(), principal(request).UserID)
	if err != nil {
		handler.marketWatchlistError(writer, err)
		return
	}

	responseItems := make([]marketWatchlistObservation, len(items))
	symbols := make([]string, len(items))
	for index, item := range items {
		responseItems[index].WatchlistItem = item
		symbols[index] = item.Symbol
	}
	state := "EMPTY"
	message := "Add a crypto asset to begin a durable, read-only venue watchlist."
	cached := false
	unavailable := make([]string, 0)
	if len(items) > 0 {
		state = "READY"
		message = "Current Coinbase Exchange observations are shown with exact source and venue evidence."
		batch, batchCached, batchErr := handler.markets.CryptoMarkets(request.Context(), "USD", symbols)
		cached = batchCached
		if batchErr != nil {
			state = "UNAVAILABLE"
			message = "The watchlist is saved, but current venue observations are temporarily unavailable. No values were substituted."
			unavailable = append(unavailable, symbols...)
		} else {
			observations := make(map[string]marketintelligence.CryptoMarketObservation, len(batch.Markets))
			for _, observation := range batch.Markets {
				observations[observation.Symbol] = observation
			}
			for index := range responseItems {
				observation, ok := observations[responseItems[index].Symbol]
				if !ok {
					unavailable = append(unavailable, responseItems[index].Symbol)
					continue
				}
				copyValue := observation
				responseItems[index].Observation = &copyValue
			}
			if len(unavailable) > 0 {
				state = "PARTIAL"
				message = "Some saved assets have no approved Coinbase USD observation. They remain visible without estimated values."
			}
		}
	}
	sort.Strings(unavailable)
	writeJSON(writer, stdhttp.StatusOK, map[string]any{
		"items":                    responseItems,
		"market_state":             state,
		"message":                  message,
		"unavailable_symbols":      unavailable,
		"cached":                   cached,
		"generated_at":             time.Now().UTC(),
		"max_items":                marketintelligence.MaxWatchlistItems,
		"provider_errors_exposed":  false,
		"provider_write_available": false,
		"order_actions_available":  false,
		"live_execution_available": false,
	})
}

func (handler *authHandler) createMarketWatchlistItem(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.csrf(request) {
		writeError(writer, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	var input struct {
		Symbol string `json:"symbol"`
	}
	if !decode(writer, request, &input) {
		return
	}
	item, err := handler.markets.CreateWatchlistItem(request.Context(), principal(request).UserID, input.Symbol)
	if err != nil {
		handler.marketWatchlistError(writer, err)
		return
	}
	writeJSON(writer, stdhttp.StatusCreated, map[string]any{
		"item":                     item,
		"provider_write_available": false,
		"order_actions_available":  false,
		"live_execution_available": false,
	})
}

func (handler *authHandler) deleteMarketWatchlistItem(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if !handler.csrf(request) {
		writeError(writer, stdhttp.StatusForbidden, "csrf_rejected", "Request origin is not allowed.")
		return
	}
	if err := handler.markets.DeleteWatchlistItem(request.Context(), principal(request).UserID, request.PathValue("id")); err != nil {
		handler.marketWatchlistError(writer, err)
		return
	}
	writer.WriteHeader(stdhttp.StatusNoContent)
}

func (handler *authHandler) marketWatchlistError(writer stdhttp.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketintelligence.ErrWatchlistInvalid):
		writeError(writer, stdhttp.StatusBadRequest, "INVALID_WATCHLIST_ITEM", "Use a supported crypto asset symbol.")
	case errors.Is(err, marketintelligence.ErrWatchlistLimit):
		writeError(writer, stdhttp.StatusConflict, "WATCHLIST_LIMIT_REACHED", "The watchlist is at its 12-asset safety limit.")
	case errors.Is(err, marketintelligence.ErrWatchlistConflict):
		writeError(writer, stdhttp.StatusConflict, "WATCHLIST_ITEM_EXISTS", "That asset is already on the watchlist.")
	case errors.Is(err, marketintelligence.ErrWatchlistNotFound):
		writeError(writer, stdhttp.StatusNotFound, "WATCHLIST_ITEM_NOT_FOUND", "The watchlist item was not found.")
	default:
		writeError(writer, stdhttp.StatusServiceUnavailable, "WATCHLIST_UNAVAILABLE", "The market watchlist is temporarily unavailable.")
	}
}
