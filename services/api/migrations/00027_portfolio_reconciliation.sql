-- +goose Up
CREATE TABLE portfolio_reconciliations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_name text NOT NULL CHECK (provider_name IN ('coinbase','schwab')),
  comparison_status text NOT NULL CHECK (comparison_status IN ('BASELINE','MATCHED','DRIFT_DETECTED','INCOMPLETE')),
  balances_status text NOT NULL CHECK (balances_status IN ('READY','UNAVAILABLE')),
  positions_status text NOT NULL CHECK (positions_status IN ('READY','UNAVAILABLE')),
  performance_status text NOT NULL CHECK (performance_status IN ('AVAILABLE','PARTIAL','UNAVAILABLE')),
  realized_performance_status text NOT NULL CHECK (realized_performance_status = 'UNAVAILABLE'),
  autonomy_signal text NOT NULL CHECK (autonomy_signal IN ('CLEAR','REVIEW_RECOMMENDED','INSUFFICIENT_EVIDENCE')),
  blocks_new_actions boolean NOT NULL DEFAULT false CHECK (blocks_new_actions = false),
  observed_position_count integer NOT NULL CHECK (observed_position_count BETWEEN 0 AND 1000),
  performance_position_count integer NOT NULL CHECK (performance_position_count BETWEEN 0 AND observed_position_count),
  change_count integer NOT NULL CHECK (change_count BETWEEN 0 AND 1000),
  cash_amount numeric(38,18),
  cash_currency char(3),
  available_cash_amount numeric(38,18),
  available_cash_currency char(3),
  buying_power_amount numeric(38,18),
  buying_power_currency char(3),
  account_value_amount numeric(38,18),
  account_value_currency char(3),
  previous_reconciliation_id uuid,
  changes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(changes) = 'array' AND jsonb_array_length(changes) <= 1000),
  evidence_hash bytea NOT NULL CHECK (octet_length(evidence_hash) = 32),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id,user_id,financial_account_id),
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (previous_reconciliation_id,user_id,financial_account_id)
    REFERENCES portfolio_reconciliations(id,user_id,financial_account_id) ON DELETE RESTRICT,
  CHECK ((comparison_status='DRIFT_DETECTED') = (change_count > 0)),
  CHECK ((comparison_status='INCOMPLETE') = (balances_status='UNAVAILABLE' OR positions_status='UNAVAILABLE')),
  CHECK ((positions_status='UNAVAILABLE' AND observed_position_count=0 AND performance_position_count=0) OR positions_status='READY'),
  CHECK ((cash_amount IS NULL) = (cash_currency IS NULL)),
  CHECK ((available_cash_amount IS NULL) = (available_cash_currency IS NULL)),
  CHECK ((buying_power_amount IS NULL) = (buying_power_currency IS NULL)),
  CHECK ((account_value_amount IS NULL) = (account_value_currency IS NULL)),
  CHECK (balances_status='READY' OR (cash_amount IS NULL AND available_cash_amount IS NULL AND buying_power_amount IS NULL AND account_value_amount IS NULL)),
  CHECK ((performance_status='AVAILABLE' AND observed_position_count > 0 AND performance_position_count=observed_position_count) OR
         (performance_status='PARTIAL' AND performance_position_count > 0 AND performance_position_count < observed_position_count) OR
         (performance_status='UNAVAILABLE' AND performance_position_count=0)),
  CHECK ((comparison_status='MATCHED' AND autonomy_signal='CLEAR') OR
         (comparison_status IN ('BASELINE','INCOMPLETE') AND autonomy_signal='INSUFFICIENT_EVIDENCE') OR
         (comparison_status='DRIFT_DETECTED' AND autonomy_signal='REVIEW_RECOMMENDED'))
);

CREATE INDEX portfolio_reconciliations_owner_account_time_idx
  ON portfolio_reconciliations(user_id,financial_account_id,observed_at DESC,id DESC);

CREATE TABLE portfolio_reconciliation_positions (
  reconciliation_id uuid NOT NULL,
  user_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  symbol text NOT NULL CHECK (length(symbol) BETWEEN 1 AND 64 AND symbol !~ '[[:cntrl:]]'),
  instrument_type text NOT NULL CHECK (length(instrument_type) BETWEEN 1 AND 40),
  direction text NOT NULL CHECK (direction IN ('long','short')),
  quantity numeric(38,18) NOT NULL,
  available_quantity numeric(38,18),
  unavailable_to_trade_quantity numeric(38,18),
  market_value_amount numeric(38,18),
  market_value_currency char(3),
  average_price_amount numeric(38,18),
  average_price_currency char(3),
  current_price_amount numeric(38,18),
  current_price_currency char(3),
  day_profit_loss_amount numeric(38,18),
  day_profit_loss_currency char(3),
  open_profit_loss_amount numeric(38,18),
  open_profit_loss_currency char(3),
  performance_status text NOT NULL CHECK (performance_status IN ('AVAILABLE','UNAVAILABLE')),
  price_basis text NOT NULL DEFAULT '' CHECK (length(price_basis) <= 80),
  PRIMARY KEY (reconciliation_id,symbol,instrument_type,direction),
  FOREIGN KEY (reconciliation_id,user_id,financial_account_id)
    REFERENCES portfolio_reconciliations(id,user_id,financial_account_id) ON DELETE RESTRICT,
  CHECK ((market_value_amount IS NULL) = (market_value_currency IS NULL)),
  CHECK ((average_price_amount IS NULL) = (average_price_currency IS NULL)),
  CHECK ((current_price_amount IS NULL) = (current_price_currency IS NULL)),
  CHECK ((day_profit_loss_amount IS NULL) = (day_profit_loss_currency IS NULL)),
  CHECK ((open_profit_loss_amount IS NULL) = (open_profit_loss_currency IS NULL)),
  CHECK (quantity >= 0),
  CHECK (available_quantity IS NULL OR available_quantity >= 0),
  CHECK (unavailable_to_trade_quantity IS NULL OR unavailable_to_trade_quantity >= 0),
  CHECK ((performance_status='AVAILABLE' AND average_price_amount IS NOT NULL AND current_price_amount IS NOT NULL AND open_profit_loss_amount IS NOT NULL) OR
         (performance_status='UNAVAILABLE' AND (average_price_amount IS NULL OR current_price_amount IS NULL OR open_profit_loss_amount IS NULL)))
);

-- +goose StatementBegin
CREATE FUNCTION enforce_portfolio_reconciliation_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM financial_accounts a
    WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.user_id AND a.provider_name=NEW.provider_name
  ) THEN
    RAISE EXCEPTION 'portfolio reconciliation account/provider mismatch';
  END IF;
  IF NEW.previous_reconciliation_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM portfolio_reconciliations p
    WHERE p.id=NEW.previous_reconciliation_id
      AND p.user_id=NEW.user_id
      AND p.financial_account_id=NEW.financial_account_id
      AND p.observed_at < NEW.observed_at
  ) THEN
    RAISE EXCEPTION 'portfolio reconciliation predecessor is invalid';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER portfolio_reconciliation_source_guard
  BEFORE INSERT ON portfolio_reconciliations
  FOR EACH ROW EXECUTE FUNCTION enforce_portfolio_reconciliation_source();

-- +goose StatementBegin
CREATE FUNCTION reject_portfolio_reconciliation_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'portfolio reconciliation evidence is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER portfolio_reconciliations_immutable
  BEFORE UPDATE OR DELETE ON portfolio_reconciliations
  FOR EACH ROW EXECUTE FUNCTION reject_portfolio_reconciliation_mutation();
CREATE TRIGGER portfolio_reconciliation_positions_immutable
  BEFORE UPDATE OR DELETE ON portfolio_reconciliation_positions
  FOR EACH ROW EXECUTE FUNCTION reject_portfolio_reconciliation_mutation();

-- +goose Down
-- Reconciliation evidence is deliberately retained once present.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM portfolio_reconciliations) THEN
    RAISE EXCEPTION 'cannot remove immutable portfolio reconciliation history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS portfolio_reconciliation_positions_immutable ON portfolio_reconciliation_positions;
DROP TRIGGER IF EXISTS portfolio_reconciliations_immutable ON portfolio_reconciliations;
DROP TRIGGER IF EXISTS portfolio_reconciliation_source_guard ON portfolio_reconciliations;
DROP FUNCTION IF EXISTS reject_portfolio_reconciliation_mutation();
DROP FUNCTION IF EXISTS enforce_portfolio_reconciliation_source();
DROP TABLE IF EXISTS portfolio_reconciliation_positions;
DROP TABLE IF EXISTS portfolio_reconciliations;
