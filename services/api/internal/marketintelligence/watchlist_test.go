package marketintelligence

import (
	"context"
	"errors"
	"testing"
	"time"
)

type watchlistStoreFake struct {
	items         []WatchlistItem
	createdSymbol string
	deletedID     string
	err           error
}

func (fake *watchlistStoreFake) ListWatchlist(context.Context, string) ([]WatchlistItem, error) {
	return fake.items, fake.err
}

func (fake *watchlistStoreFake) CreateWatchlistItem(_ context.Context, _ string, symbol string) (WatchlistItem, error) {
	fake.createdSymbol = symbol
	return WatchlistItem{ID: "3f6ab43c-8abd-4056-a858-ccb8051a045f", AssetClass: Crypto, Symbol: symbol, QuoteCurrency: "USD", CreatedAt: time.Now().UTC()}, fake.err
}

func (fake *watchlistStoreFake) DeleteWatchlistItem(_ context.Context, _, itemID string) error {
	fake.deletedID = itemID
	return fake.err
}

func TestWatchlistCanonicalizesCryptoSymbolsAndRejectsUnsafeInput(t *testing.T) {
	store := &watchlistStoreFake{}
	service, err := NewService(ServiceConfig{Watchlists: store})
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.CreateWatchlistItem(context.Background(), "user-1", " btc ")
	if err != nil || item.Symbol != "BTC" || store.createdSymbol != "BTC" {
		t.Fatalf("watchlist symbol was not canonicalized: item=%+v stored=%q err=%v", item, store.createdSymbol, err)
	}
	for _, invalid := range []string{"", "BTC-USD", "../../BTC", "BTC/USD", "123"} {
		if _, err = service.CreateWatchlistItem(context.Background(), "user-1", invalid); !errors.Is(err, ErrWatchlistInvalid) {
			t.Fatalf("unsafe watchlist symbol %q was accepted: %v", invalid, err)
		}
	}
}

func TestWatchlistFailsClosedOnInvalidStoredRowsAndIDs(t *testing.T) {
	store := &watchlistStoreFake{items: []WatchlistItem{{ID: "item", AssetClass: Crypto, Symbol: "BTC-USD", QuoteCurrency: "USD", CreatedAt: time.Now().UTC()}}}
	service, err := NewService(ServiceConfig{Watchlists: store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ListWatchlist(context.Background(), "user-1"); !errors.Is(err, ErrWatchlistUnavailable) {
		t.Fatalf("invalid durable row was exposed: %v", err)
	}
	if err = service.DeleteWatchlistItem(context.Background(), "user-1", "not-a-uuid"); !errors.Is(err, ErrWatchlistInvalid) || store.deletedID != "" {
		t.Fatalf("invalid watchlist identity reached storage: id=%q err=%v", store.deletedID, err)
	}
}

func TestWatchlistRejectsMoreThanThePublishedBound(t *testing.T) {
	items := make([]WatchlistItem, MaxWatchlistItems+1)
	for index := range items {
		items[index] = WatchlistItem{ID: "3f6ab43c-8abd-4056-a858-ccb8051a045f", AssetClass: Crypto, Symbol: "BTC", QuoteCurrency: "USD", CreatedAt: time.Now().UTC()}
	}
	service, err := NewService(ServiceConfig{Watchlists: &watchlistStoreFake{items: items}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ListWatchlist(context.Background(), "user-1"); !errors.Is(err, ErrWatchlistUnavailable) {
		t.Fatalf("oversized watchlist was exposed: %v", err)
	}
}
