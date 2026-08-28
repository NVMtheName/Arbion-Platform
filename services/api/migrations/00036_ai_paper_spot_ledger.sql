-- +goose Up
ALTER TABLE paper_portfolios
  ADD CONSTRAINT paper_portfolios_ai_paper_owner_unique
  UNIQUE (id,user_id,strategy_instance_id);

CREATE TABLE ai_paper_spot_fills (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL,
  paper_portfolio_id uuid NOT NULL,
  execution_record_id uuid NOT NULL,
  proposed_action_id text NOT NULL CHECK (length(proposed_action_id) BETWEEN 1 AND 300),
  risk_evaluation_id uuid NOT NULL,
  symbol text NOT NULL CHECK (symbol ~ '^[A-Z0-9][A-Z0-9.-]{0,19}$'),
  instrument text NOT NULL CHECK (instrument IN ('EQUITY','CRYPTO')),
  side text NOT NULL CHECK (side IN ('BUY','SELL')),
  quantity numeric(30,10) NOT NULL CHECK (quantity > 0),
  requested_notional numeric(30,10) NOT NULL CHECK (requested_notional > 0),
  reference_price numeric(30,10) NOT NULL CHECK (reference_price > 0),
  fill_price numeric(30,10) NOT NULL CHECK (fill_price > 0),
  gross_notional numeric(30,10) NOT NULL CHECK (gross_notional > 0),
  fee numeric(30,10) NOT NULL CHECK (fee >= 0),
  cash_delta numeric(30,10) NOT NULL,
  position_delta numeric(30,10) NOT NULL,
  previous_cash numeric(30,10) NOT NULL CHECK (previous_cash >= 0),
  previous_position_quantity numeric(30,10) NOT NULL CHECK (previous_position_quantity >= 0),
  resulting_cash numeric(30,10) NOT NULL CHECK (resulting_cash >= 0),
  resulting_position_quantity numeric(30,10) NOT NULL CHECK (resulting_position_quantity >= 0),
  pricing_basis text NOT NULL CHECK (length(pricing_basis) BETWEEN 1 AND 50),
  market_provider text NOT NULL CHECK (length(market_provider) BETWEEN 1 AND 50),
  market_feed text NOT NULL CHECK (length(market_feed) BETWEEN 1 AND 100),
  market_quality text NOT NULL CHECK (length(market_quality) BETWEEN 1 AND 100),
  market_observed_at timestamptz NOT NULL,
  simulated_at timestamptz NOT NULL,
  simulation_only boolean NOT NULL DEFAULT true CHECK (simulation_only),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (market_observed_at >= simulated_at - interval '2 minutes'),
  CHECK (market_observed_at <= simulated_at + interval '5 seconds'),
  CHECK (requested_notional >= quantity * reference_price),
  CHECK (
    (side='BUY' AND fill_price >= reference_price AND cash_delta=-(gross_notional+fee) AND position_delta=quantity) OR
    (side='SELL' AND fill_price <= reference_price AND cash_delta=gross_notional-fee AND position_delta=-quantity)
  ),
  CHECK (resulting_cash=previous_cash+cash_delta),
  CHECK (resulting_position_quantity=previous_position_quantity+position_delta),
  UNIQUE (execution_record_id),
  UNIQUE (proposed_action_id),
  UNIQUE (risk_evaluation_id),
  FOREIGN KEY (strategy_instance_id,user_id)
    REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (paper_portfolio_id,user_id,strategy_instance_id)
    REFERENCES paper_portfolios(id,user_id,strategy_instance_id) ON DELETE RESTRICT,
  FOREIGN KEY (execution_record_id,user_id,strategy_instance_id)
    REFERENCES nonlive_execution_records(id,user_id,strategy_instance_id) ON DELETE RESTRICT,
  FOREIGN KEY (risk_evaluation_id)
    REFERENCES risk_evaluations(id) ON DELETE RESTRICT
);

CREATE INDEX ai_paper_spot_fills_owner_time_idx
  ON ai_paper_spot_fills(user_id,simulated_at DESC,id DESC);
CREATE INDEX ai_paper_spot_fills_instance_time_idx
  ON ai_paper_spot_fills(strategy_instance_id,simulated_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_ai_paper_spot_fill_source() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  source_is_valid boolean;
BEGIN
  SELECT true INTO source_is_valid
  FROM nonlive_execution_records x
  JOIN strategy_instances i
    ON i.id=x.strategy_instance_id AND i.user_id=x.user_id
  JOIN decision_journal_entries j
    ON j.execution_record_id=x.id AND j.user_id=x.user_id AND j.strategy_instance_id=x.strategy_instance_id
  WHERE x.id=NEW.execution_record_id
    AND x.user_id=NEW.user_id
    AND x.strategy_instance_id=NEW.strategy_instance_id
    AND x.mode='PAPER'
    AND x.status='SIMULATED_FILLED'
    AND x.proposed_action_id=NEW.proposed_action_id
    AND x.risk_evaluation_id=NEW.risk_evaluation_id
    AND x.symbol=NEW.symbol
    AND x.instrument=NEW.instrument
    AND x.side=NEW.side
    AND x.quantity=NEW.quantity
    AND x.price=NEW.fill_price
    AND x.notional=NEW.gross_notional
    AND x.created_at=NEW.simulated_at
    AND i.strategy_identifier='ai_shadow'
    AND i.execution_mode='PAPER'
    AND i.current_state='AI_MONITORING'
    AND j.source='AI'
    AND j.proposed_action_id=NEW.proposed_action_id
    AND j.risk_evaluation_id=NEW.risk_evaluation_id
    AND j.created_at=NEW.simulated_at;

  IF source_is_valid IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'invalid AI paper spot fill source';
  END IF;
  RETURN NEW;
END; $$;
-- +goose StatementEnd

CREATE TRIGGER ai_paper_spot_fills_source_guard
  BEFORE INSERT ON ai_paper_spot_fills
  FOR EACH ROW EXECUTE FUNCTION enforce_ai_paper_spot_fill_source();

CREATE TRIGGER ai_paper_spot_fills_immutable
  BEFORE UPDATE OR DELETE ON ai_paper_spot_fills
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM ai_paper_spot_fills) THEN
    RAISE EXCEPTION 'cannot remove immutable AI paper spot fill history';
  END IF;
END $$;

DROP TRIGGER IF EXISTS ai_paper_spot_fills_immutable ON ai_paper_spot_fills;
DROP TRIGGER IF EXISTS ai_paper_spot_fills_source_guard ON ai_paper_spot_fills;
DROP FUNCTION IF EXISTS enforce_ai_paper_spot_fill_source();
DROP TABLE IF EXISTS ai_paper_spot_fills;
ALTER TABLE paper_portfolios
  DROP CONSTRAINT IF EXISTS paper_portfolios_ai_paper_owner_unique;
