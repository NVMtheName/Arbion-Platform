package marketintelligence

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

const MaxWatchlistItems = 12

var (
	ErrWatchlistUnavailable = errors.New("market watchlist unavailable")
	ErrWatchlistInvalid     = errors.New("invalid market watchlist item")
	ErrWatchlistLimit       = errors.New("market watchlist limit reached")
	ErrWatchlistConflict    = errors.New("market watchlist item already exists")
	ErrWatchlistNotFound    = errors.New("market watchlist item not found")
)

var watchlistIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// WatchlistItem is a durable owner preference, not a position, order, signal,
// or authorization to act at a provider.
type WatchlistItem struct {
	ID            string     `json:"id"`
	AssetClass    AssetClass `json:"asset_class"`
	Symbol        string     `json:"symbol"`
	QuoteCurrency string     `json:"quote_currency"`
	CreatedAt     time.Time  `json:"created_at"`
}

type WatchlistStore interface {
	ListWatchlist(context.Context, string) ([]WatchlistItem, error)
	CreateWatchlistItem(context.Context, string, string) (WatchlistItem, error)
	DeleteWatchlistItem(context.Context, string, string) error
}

func (service *Service) ListWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error) {
	if service == nil || service.watchlists == nil || strings.TrimSpace(userID) == "" {
		return nil, ErrWatchlistUnavailable
	}
	items, err := service.watchlists.ListWatchlist(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(items) > MaxWatchlistItems {
		return nil, ErrWatchlistUnavailable
	}
	for _, item := range items {
		if !validWatchlistItem(item) {
			return nil, ErrWatchlistUnavailable
		}
	}
	return append([]WatchlistItem(nil), items...), nil
}

func (service *Service) CreateWatchlistItem(ctx context.Context, userID, symbol string) (WatchlistItem, error) {
	if service == nil || service.watchlists == nil || strings.TrimSpace(userID) == "" {
		return WatchlistItem{}, ErrWatchlistUnavailable
	}
	canonical, ok := canonicalCryptoSymbols("USD", []string{symbol})
	if !ok || len(canonical) != 1 || !watchlistSymbolHasLetter(canonical[0]) {
		return WatchlistItem{}, ErrWatchlistInvalid
	}
	item, err := service.watchlists.CreateWatchlistItem(ctx, userID, canonical[0])
	if err != nil {
		return WatchlistItem{}, err
	}
	if !validWatchlistItem(item) || item.Symbol != canonical[0] {
		return WatchlistItem{}, ErrWatchlistUnavailable
	}
	return item, nil
}

func watchlistSymbolHasLetter(symbol string) bool {
	for _, character := range symbol {
		if character >= 'A' && character <= 'Z' {
			return true
		}
	}
	return false
}

func validWatchlistItem(item WatchlistItem) bool {
	canonical, ok := canonicalCryptoSymbols(item.QuoteCurrency, []string{item.Symbol})
	return watchlistIDPattern.MatchString(strings.ToLower(item.ID)) && item.AssetClass == Crypto && ok && len(canonical) == 1 && canonical[0] == item.Symbol && watchlistSymbolHasLetter(item.Symbol) && item.QuoteCurrency == "USD" && !item.CreatedAt.IsZero()
}

func (service *Service) DeleteWatchlistItem(ctx context.Context, userID, itemID string) error {
	itemID = strings.ToLower(strings.TrimSpace(itemID))
	if service == nil || service.watchlists == nil || strings.TrimSpace(userID) == "" {
		return ErrWatchlistUnavailable
	}
	if !watchlistIDPattern.MatchString(itemID) {
		return ErrWatchlistInvalid
	}
	return service.watchlists.DeleteWatchlistItem(ctx, userID, itemID)
}
