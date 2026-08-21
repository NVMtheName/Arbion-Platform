-- +goose Up
CREATE TABLE market_source_health_buckets (
  source_id text NOT NULL,
  capability text NOT NULL,
  bucket_started_at timestamptz NOT NULL,
  completed_attempts bigint NOT NULL CHECK (completed_attempts > 0),
  successes bigint NOT NULL CHECK (successes >= 0),
  failures bigint NOT NULL CHECK (failures >= 0),
  last_state text NOT NULL CHECK (last_state IN ('VERIFIED','DEGRADED')),
  failure_category text CHECK (failure_category IN ('TIMEOUT','STALE_DATA','FUTURE_DATED_DATA','MISSING_PROVENANCE','INVALID_DATA','UPSTREAM_FAILURE')),
  last_observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (source_id,capability,bucket_started_at),
  CHECK (completed_attempts = successes + failures),
  CHECK (bucket_started_at = date_bin('5 minutes',bucket_started_at,timestamptz '1970-01-01 00:00:00+00')),
  CHECK (last_observed_at >= bucket_started_at AND last_observed_at < bucket_started_at + interval '5 minutes'),
  CHECK (
    (last_state = 'VERIFIED' AND failure_category IS NULL) OR
    (last_state = 'DEGRADED' AND failure_category IS NOT NULL)
  ),
  CHECK (
    (source_id = 'schwab_broker_market_data' AND capability IN ('EQUITY_QUOTE','OPTION_DATA')) OR
    (source_id = 'alpaca_iex' AND capability IN ('EQUITY_QUOTE','EQUITY_BARS')) OR
    (source_id = 'alpaca_sip' AND capability IN ('EQUITY_QUOTE','EQUITY_BARS')) OR
    (source_id = 'alpaca_options_indicative' AND capability = 'OPTION_DATA') OR
    (source_id = 'alpaca_opra' AND capability = 'OPTION_DATA') OR
    (source_id = 'coingecko_rest' AND capability = 'CRYPTO_MARKETS') OR
    (source_id = 'coinbase_exchange' AND capability IN ('CRYPTO_MARKETS','CRYPTO_CANDLES','CRYPTO_LIQUIDITY','CRYPTO_TRADES','CRYPTO_VENUE_STATS')) OR
    (source_id = 'sec_edgar' AND capability = 'INSIDER_FILING')
  )
);

CREATE INDEX market_source_health_time_idx
  ON market_source_health_buckets(bucket_started_at DESC,source_id,capability);

-- +goose Down
DROP TABLE IF EXISTS market_source_health_buckets;
