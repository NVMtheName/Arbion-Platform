package marketintelligence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/arbion/platform/services/api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestPostgresWatchlistIsOwnerScopedUniqueAndHardBounded(t *testing.T) {
	databaseURL := os.Getenv("STRATEGY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("STRATEGY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.Files)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var firstUser, secondUser string
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, "watchlist-owner-a").Scan(&firstUser); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, firstUser)
	if err = pool.QueryRow(ctx, `INSERT INTO users(external_id) VALUES($1) RETURNING id::text`, "watchlist-owner-b").Scan(&secondUser); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, secondUser)

	store := NewPostgresWatchlistStore(pool)
	first, err := store.CreateWatchlistItem(ctx, firstUser, "BTC")
	if err != nil || first.Symbol != "BTC" || first.AssetClass != Crypto || first.QuoteCurrency != "USD" || first.ID == "" || first.CreatedAt.IsZero() {
		t.Fatalf("unexpected first watchlist item: item=%+v err=%v", first, err)
	}
	if _, err = store.CreateWatchlistItem(ctx, firstUser, "BTC"); !errors.Is(err, ErrWatchlistConflict) {
		t.Fatalf("duplicate watchlist symbol did not conflict: %v", err)
	}
	for index := 1; index < MaxWatchlistItems; index++ {
		if _, err = store.CreateWatchlistItem(ctx, firstUser, fmt.Sprintf("ASSET%d", index)); err != nil {
			t.Fatalf("bounded watchlist insert %d failed: %v", index, err)
		}
	}
	if _, err = store.CreateWatchlistItem(ctx, firstUser, "OVERFLOW"); !errors.Is(err, ErrWatchlistLimit) {
		t.Fatalf("watchlist exceeded its hard bound: %v", err)
	}
	items, err := store.ListWatchlist(ctx, firstUser)
	if err != nil || len(items) != MaxWatchlistItems {
		t.Fatalf("owner watchlist mismatch: count=%d err=%v", len(items), err)
	}

	second, err := store.CreateWatchlistItem(ctx, secondUser, "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteWatchlistItem(ctx, firstUser, second.ID); !errors.Is(err, ErrWatchlistNotFound) {
		t.Fatalf("one owner deleted another owner's item: %v", err)
	}
	secondItems, err := store.ListWatchlist(ctx, secondUser)
	if err != nil || len(secondItems) != 1 || secondItems[0].ID != second.ID {
		t.Fatalf("second owner watchlist was not isolated: items=%+v err=%v", secondItems, err)
	}
	if err = store.DeleteWatchlistItem(ctx, firstUser, first.ID); err != nil {
		t.Fatal(err)
	}
	items, err = store.ListWatchlist(ctx, firstUser)
	if err != nil || len(items) != MaxWatchlistItems-1 {
		t.Fatalf("owner delete did not persist: count=%d err=%v", len(items), err)
	}
}
