-- +goose Up
-- Private normalized provider observations. These are NOT Arbion order intents,
-- approval, simulated fills, settlement accounting, or execution authority.
CREATE TABLE financial_fill_observations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_name text NOT NULL CHECK(provider_name='coinbase'),
  entry_reference text NOT NULL CHECK(entry_reference ~ '^[a-f0-9]{64}$'),
  trade_reference text NOT NULL CHECK(trade_reference ~ '^[a-f0-9]{64}$'),
  order_reference text NOT NULL CHECK(order_reference ~ '^[a-f0-9]{64}$'),
  evidence_digest text NOT NULL CHECK(evidence_digest ~ '^[a-f0-9]{64}$'),
  product_id text NOT NULL,
  base_asset text NOT NULL CHECK(base_asset ~ '^[A-Z0-9]{1,20}$'),
  quote_currency text NOT NULL CHECK(quote_currency ~ '^[A-Z0-9]{1,20}$'),
  side text NOT NULL CHECK(side IN ('BUY','SELL')),
  -- Unconstrained numeric avoids rounding before validation; reject NaN too.
  price numeric NOT NULL CHECK(price>0 AND price<1e40 AND scale(price)<=32),
  size numeric NOT NULL CHECK(size>0 AND size<1e40 AND scale(size)<=32),
  size_unit text NOT NULL,
  commission numeric NOT NULL CHECK(commission>=0 AND commission<1e40 AND scale(commission)<=32),
  commission_currency_status text NOT NULL DEFAULT 'UNAVAILABLE' CHECK(commission_currency_status='UNAVAILABLE'),
  liquidity text NOT NULL CHECK(liquidity IN ('MAKER','TAKER','UNKNOWN')),
  -- Preserve exact fractional timestamp evidence, not rounded microseconds.
  trade_time text NOT NULL CHECK(length(trade_time)<=40 AND trade_time ~ 'Z$'),
  sequence_time text NOT NULL CHECK(length(sequence_time)<=40 AND sequence_time ~ 'Z$'),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE(financial_account_id,entry_reference),
  FOREIGN KEY(financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  CHECK(product_id=base_asset||'-'||quote_currency),
  CHECK(size_unit IN (base_asset,quote_currency)),
  CHECK(trade_time::timestamptz <= observed_at AND sequence_time::timestamptz <= observed_at),
  CHECK(observed_at<=created_at)
);
CREATE INDEX financial_fill_observations_owner_account_idx ON financial_fill_observations(user_id,financial_account_id);

CREATE TABLE financial_fill_capture_receipts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_name text NOT NULL CHECK(provider_name='coinbase'),
  source_operation text NOT NULL DEFAULT 'OWNER_HISTORY_READ' CHECK(source_operation='OWNER_HISTORY_READ'),
  observed_at timestamptz NOT NULL,
  reported_count integer NOT NULL CHECK(reported_count BETWEEN 0 AND 50),
  unique_count integer NOT NULL CHECK(unique_count BETWEEN 0 AND reported_count),
  inserted_count integer NOT NULL CHECK(inserted_count>=0),
  matched_count integer NOT NULL CHECK(matched_count>=0),
  has_more boolean NOT NULL,
  complete_account_history boolean NOT NULL DEFAULT false CHECK(NOT complete_account_history),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  CHECK(inserted_count+matched_count=unique_count),
  CHECK(observed_at<=created_at)
);
CREATE INDEX financial_fill_receipts_owner_account_time_idx ON financial_fill_capture_receipts(user_id,financial_account_id,created_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION guard_private_fill_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION 'private fill evidence is immutable';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM financial_accounts a WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.user_id AND a.provider_name=NEW.provider_name) THEN
    RAISE EXCEPTION 'private fill evidence account binding mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER private_fill_observations_guard BEFORE INSERT OR UPDATE OR DELETE ON financial_fill_observations FOR EACH ROW EXECUTE FUNCTION guard_private_fill_evidence();
CREATE TRIGGER private_fill_receipts_guard BEFORE INSERT OR UPDATE OR DELETE ON financial_fill_capture_receipts FOR EACH ROW EXECUTE FUNCTION guard_private_fill_evidence();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM financial_fill_observations) OR EXISTS(SELECT 1 FROM financial_fill_capture_receipts) THEN
    RAISE EXCEPTION 'cannot remove immutable private fill evidence';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE financial_fill_capture_receipts;
DROP TABLE financial_fill_observations;
DROP FUNCTION guard_private_fill_evidence();
