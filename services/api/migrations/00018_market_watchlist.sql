-- +goose Up
CREATE TABLE market_watchlist_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  asset_class text NOT NULL DEFAULT 'CRYPTO' CHECK (asset_class = 'CRYPTO'),
  symbol text NOT NULL CHECK (
    symbol = upper(btrim(symbol)) AND
    length(symbol) BETWEEN 1 AND 12 AND
    symbol ~ '^[A-Z0-9]+$' AND
    symbol ~ '[A-Z]'
  ),
  quote_currency char(3) NOT NULL DEFAULT 'USD' CHECK (quote_currency = 'USD'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id,asset_class,symbol,quote_currency)
);

CREATE INDEX market_watchlist_owner_order_idx
  ON market_watchlist_items(user_id,created_at,id);

-- +goose Down
DROP TABLE IF EXISTS market_watchlist_items;
