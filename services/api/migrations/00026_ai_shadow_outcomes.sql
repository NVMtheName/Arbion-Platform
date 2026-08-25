-- +goose Up
ALTER TABLE nonlive_execution_records
  ADD CONSTRAINT nonlive_execution_records_outcome_owner_unique
  UNIQUE (id,user_id,strategy_instance_id);

CREATE TABLE shadow_execution_outcomes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  execution_record_id uuid NOT NULL,
  horizon text NOT NULL CHECK (horizon IN ('ONE_HOUR','TWENTY_FOUR_HOURS')),
  symbol text NOT NULL CHECK (length(symbol) BETWEEN 1 AND 32),
  side text NOT NULL CHECK (side IN ('BUY','SELL')),
  quantity numeric(30,10) NOT NULL CHECK (quantity > 0),
  entry_price numeric(30,10) NOT NULL CHECK (entry_price > 0),
  observed_price numeric(30,10) NOT NULL CHECK (observed_price > 0),
  directional_change_usd numeric(30,10) NOT NULL,
  directional_change_percent numeric(30,10) NOT NULL,
  pricing_basis text NOT NULL CHECK (pricing_basis IN ('BID_TO_CLOSE','ASK_TO_CLOSE','MARK_FALLBACK','LAST_FALLBACK')),
  market_feed text NOT NULL CHECK (length(market_feed) BETWEEN 1 AND 100),
  market_quality text NOT NULL CHECK (length(market_quality) BETWEEN 1 AND 100),
  market_observed_at timestamptz NOT NULL,
  evaluated_at timestamptz NOT NULL,
  elapsed_seconds bigint NOT NULL CHECK (
    (horizon='ONE_HOUR' AND elapsed_seconds >= 3600) OR
    (horizon='TWENTY_FOUR_HOURS' AND elapsed_seconds >= 86400)
  ),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    market_observed_at >= evaluated_at - interval '15 minutes' AND
    market_observed_at <= evaluated_at + interval '1 minute'
  ),
  CHECK (
    directional_change_usd = round(
      (CASE WHEN side='BUY' THEN observed_price-entry_price ELSE entry_price-observed_price END) * quantity,
      10
    )
  ),
  CHECK (
    directional_change_percent = round(
      ((CASE WHEN side='BUY' THEN observed_price-entry_price ELSE entry_price-observed_price END) / entry_price) * 100,
      10
    )
  ),
  UNIQUE (execution_record_id,horizon),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (execution_record_id,user_id,strategy_instance_id)
    REFERENCES nonlive_execution_records(id,user_id,strategy_instance_id) ON DELETE RESTRICT
);

CREATE INDEX shadow_execution_outcomes_instance_time_idx
  ON shadow_execution_outcomes(strategy_instance_id,evaluated_at DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_ai_shadow_outcome_source() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  execution_created_at timestamptz;
  minimum_age interval;
BEGIN
  IF NEW.horizon='ONE_HOUR' THEN
    minimum_age := interval '1 hour';
  ELSIF NEW.horizon='TWENTY_FOUR_HOURS' THEN
    minimum_age := interval '24 hours';
  ELSE
    RAISE EXCEPTION 'invalid AI shadow outcome horizon';
  END IF;

  SELECT x.created_at INTO execution_created_at
  FROM nonlive_execution_records x
  JOIN strategy_instances i ON i.id=x.strategy_instance_id AND i.user_id=x.user_id
  WHERE x.id=NEW.execution_record_id
    AND x.user_id=NEW.user_id
    AND x.strategy_instance_id=NEW.strategy_instance_id
    AND x.mode='SHADOW'
    AND x.status='WOULD_HAVE_SUBMITTED'
    AND x.symbol=NEW.symbol
    AND x.side=NEW.side
    AND x.quantity=NEW.quantity
    AND x.price=NEW.entry_price
    AND i.strategy_identifier='ai_shadow'
    AND i.execution_mode='SHADOW';

  IF execution_created_at IS NULL OR
     NEW.evaluated_at < execution_created_at + minimum_age OR
     NEW.elapsed_seconds <> floor(extract(epoch FROM (NEW.evaluated_at-execution_created_at)))::bigint THEN
    RAISE EXCEPTION 'invalid AI shadow outcome source or horizon';
  END IF;
  RETURN NEW;
END; $$;
-- +goose StatementEnd

CREATE TRIGGER shadow_execution_outcomes_source_guard
  BEFORE INSERT ON shadow_execution_outcomes
  FOR EACH ROW EXECUTE FUNCTION enforce_ai_shadow_outcome_source();

CREATE TRIGGER shadow_execution_outcomes_immutable
  BEFORE UPDATE OR DELETE ON shadow_execution_outcomes
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM shadow_execution_outcomes) THEN
    RAISE EXCEPTION 'cannot remove immutable AI shadow outcome history';
  END IF;
END $$;

DROP TRIGGER IF EXISTS shadow_execution_outcomes_immutable ON shadow_execution_outcomes;
DROP TRIGGER IF EXISTS shadow_execution_outcomes_source_guard ON shadow_execution_outcomes;
DROP FUNCTION IF EXISTS enforce_ai_shadow_outcome_source();
DROP TABLE IF EXISTS shadow_execution_outcomes;
ALTER TABLE nonlive_execution_records
  DROP CONSTRAINT IF EXISTS nonlive_execution_records_outcome_owner_unique;
