package marketintelligence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresWatchlistStore struct{ db *pgxpool.Pool }

func NewPostgresWatchlistStore(db *pgxpool.Pool) *PostgresWatchlistStore {
	return &PostgresWatchlistStore{db: db}
}

const watchlistColumns = `id::text,asset_class,symbol,quote_currency,created_at`

func scanWatchlistItem(row pgx.Row) (item WatchlistItem, err error) {
	err = row.Scan(&item.ID, &item.AssetClass, &item.Symbol, &item.QuoteCurrency, &item.CreatedAt)
	return
}

func (store *PostgresWatchlistStore) ListWatchlist(ctx context.Context, userID string) ([]WatchlistItem, error) {
	if store == nil || store.db == nil {
		return nil, ErrWatchlistUnavailable
	}
	rows, err := store.db.Query(ctx, `SELECT `+watchlistColumns+` FROM market_watchlist_items WHERE user_id=$1 ORDER BY created_at,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WatchlistItem, 0)
	for rows.Next() {
		item, scanErr := scanWatchlistItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (store *PostgresWatchlistStore) CreateWatchlistItem(ctx context.Context, userID, symbol string) (WatchlistItem, error) {
	if store == nil || store.db == nil {
		return WatchlistItem{}, ErrWatchlistUnavailable
	}
	tx, err := store.db.Begin(ctx)
	if err != nil {
		return WatchlistItem{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, userID); err != nil {
		return WatchlistItem{}, err
	}
	var count int
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT count(*),COALESCE(bool_or(symbol=$2),false) FROM market_watchlist_items WHERE user_id=$1`, userID, symbol).Scan(&count, &exists); err != nil {
		return WatchlistItem{}, err
	}
	if exists {
		return WatchlistItem{}, ErrWatchlistConflict
	}
	if count >= MaxWatchlistItems {
		return WatchlistItem{}, ErrWatchlistLimit
	}
	item, err := scanWatchlistItem(tx.QueryRow(ctx, `INSERT INTO market_watchlist_items(user_id,symbol) VALUES($1,$2) RETURNING `+watchlistColumns, userID, symbol))
	if err != nil {
		var providerError *pgconn.PgError
		if errors.As(err, &providerError) && providerError.Code == "23505" {
			return WatchlistItem{}, ErrWatchlistConflict
		}
		return WatchlistItem{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return WatchlistItem{}, err
	}
	return item, nil
}

func (store *PostgresWatchlistStore) DeleteWatchlistItem(ctx context.Context, userID, itemID string) error {
	if store == nil || store.db == nil {
		return ErrWatchlistUnavailable
	}
	tag, err := store.db.Exec(ctx, `DELETE FROM market_watchlist_items WHERE id=$1 AND user_id=$2`, itemID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrWatchlistNotFound
	}
	return err
}
