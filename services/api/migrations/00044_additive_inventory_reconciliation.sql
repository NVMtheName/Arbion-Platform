-- +goose Up
-- Forward-only classification of exact additions. No history, account holdings,
-- mandates, capital claims, order state, or risk limits are rewritten.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_reconciliation_change_impact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  derived_blocking_count integer;
BEGIN
  IF NEW.change_count <> jsonb_array_length(NEW.changes) THEN
    RAISE EXCEPTION 'reconciliation change count mismatch';
  END IF;
  IF EXISTS (
    SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
    WHERE change->>'control_impact' IS NULL
       OR change->>'control_impact' NOT IN ('TRADABLE_INVENTORY','NON_TRADABLE_QUANTITY_ONLY','ADDITIVE_INVENTORY_ONLY')
  ) THEN
    RAISE EXCEPTION 'invalid reconciliation change control impact';
  END IF;
  IF NEW.provider_name <> 'coinbase' AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
    WHERE change->>'control_impact' IN ('NON_TRADABLE_QUANTITY_ONLY','ADDITIVE_INVENTORY_ONLY')
  ) THEN
    RAISE EXCEPTION 'non-tradable-only reconciliation impact requires Coinbase evidence';
  END IF;
  SELECT count(*) INTO derived_blocking_count
  FROM jsonb_array_elements(NEW.changes) AS change
  WHERE change->>'control_impact'='TRADABLE_INVENTORY';
  IF NEW.blocking_change_count <> derived_blocking_count THEN
    RAISE EXCEPTION 'reconciliation blocking change count mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION enforce_additive_reconciliation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  change jsonb;
  field_name text;
  prior portfolio_reconciliations%ROWTYPE;
  prior_position portfolio_reconciliation_positions%ROWTYPE;
  ct numeric; ca numeric; cu numeric;
  pt numeric; pa numeric; pu numeric;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(NEW.changes) AS item WHERE item->>'control_impact'='ADDITIVE_INVENTORY_ONLY') THEN
    RETURN NEW;
  END IF;
  SELECT * INTO prior FROM portfolio_reconciliations
  WHERE id=NEW.previous_reconciliation_id AND user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id;
  IF NOT FOUND OR prior.provider_name <> 'coinbase' OR NEW.provider_name <> 'coinbase'
     OR prior.balances_status <> 'READY' OR prior.positions_status <> 'READY'
     OR NEW.balances_status <> 'READY' OR NEW.positions_status <> 'READY'
     OR NEW.observed_at <= prior.observed_at THEN
    RAISE EXCEPTION 'additive inventory requires complete ordered account-scoped Coinbase snapshots';
  END IF;
  IF (prior.cash_currency IS DISTINCT FROM NEW.cash_currency) OR
     (prior.available_cash_currency IS DISTINCT FROM NEW.available_cash_currency) OR
     (prior.buying_power_currency IS DISTINCT FROM NEW.buying_power_currency) OR
     (prior.cash_amount IS NULL) <> (NEW.cash_amount IS NULL) OR
     (prior.available_cash_amount IS NULL) <> (NEW.available_cash_amount IS NULL) OR
     (prior.buying_power_amount IS NULL) <> (NEW.buying_power_amount IS NULL) OR
     prior.cash_amount < 0 OR NEW.cash_amount < prior.cash_amount OR
     prior.available_cash_amount < 0 OR NEW.available_cash_amount < prior.available_cash_amount OR
     prior.buying_power_amount < 0 OR NEW.buying_power_amount < prior.buying_power_amount THEN
    RAISE EXCEPTION 'additive inventory cannot conceal a cash reduction or changed cash context';
  END IF;
  FOR change IN SELECT * FROM jsonb_array_elements(NEW.changes) AS item WHERE item->>'control_impact'='ADDITIVE_INVENTORY_ONLY' LOOP
    IF change->>'direction' IS DISTINCT FROM 'long' OR change->>'instrument_type' IS DISTINCT FROM 'CRYPTO'
       OR COALESCE(change->>'change_type','') NOT IN ('POSITION_APPEARED','QUANTITY_CHANGED') THEN
      RAISE EXCEPTION 'additive inventory requires long crypto evidence';
    END IF;
    FOREACH field_name IN ARRAY ARRAY['current_quantity','current_available_quantity','current_unavailable_quantity'] LOOP
      IF jsonb_typeof(change->field_name) IS DISTINCT FROM 'string'
         OR length(change->>field_name) > 80
         OR (change->>field_name) !~ '^[+]?[0-9]+([.][0-9]{1,18})?$' THEN
        RAISE EXCEPTION 'invalid additive inventory decimal evidence';
      END IF;
    END LOOP;
    ct := (change->>'current_quantity')::numeric;
    ca := (change->>'current_available_quantity')::numeric;
    cu := (change->>'current_unavailable_quantity')::numeric;
    IF ct <= 0 OR ct <> ca + cu THEN
      RAISE EXCEPTION 'additive inventory current quantities must reconcile exactly';
    END IF;
    SELECT * INTO prior_position FROM portfolio_reconciliation_positions
    WHERE reconciliation_id=prior.id AND user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id
      AND symbol=change->>'symbol' AND instrument_type='CRYPTO' AND direction='long';
    IF change->>'change_type'='POSITION_APPEARED' THEN
      IF FOUND OR change ?| ARRAY['previous_quantity','previous_available_quantity','previous_unavailable_quantity'] THEN
        RAISE EXCEPTION 'additive appeared position must be absent in prior snapshot';
      END IF;
    ELSE
      IF NOT FOUND THEN
        RAISE EXCEPTION 'additive quantity change requires prior immutable position';
      END IF;
      FOREACH field_name IN ARRAY ARRAY['previous_quantity','previous_available_quantity','previous_unavailable_quantity'] LOOP
        IF jsonb_typeof(change->field_name) IS DISTINCT FROM 'string'
           OR length(change->>field_name) > 80
           OR (change->>field_name) !~ '^[+]?[0-9]+([.][0-9]{1,18})?$' THEN
          RAISE EXCEPTION 'invalid prior additive inventory decimal evidence';
        END IF;
      END LOOP;
      pt := (change->>'previous_quantity')::numeric;
      pa := (change->>'previous_available_quantity')::numeric;
      pu := (change->>'previous_unavailable_quantity')::numeric;
      IF pt IS DISTINCT FROM prior_position.quantity OR pa IS DISTINCT FROM prior_position.available_quantity
         OR pu IS DISTINCT FROM prior_position.unavailable_to_trade_quantity
         OR pt <> pa + pu OR ct <= pt OR ca < pa OR cu < pu THEN
        RAISE EXCEPTION 'additive inventory cannot reduce or invent prior quantities';
      END IF;
    END IF;
  END LOOP;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER portfolio_reconciliation_additive_guard
  BEFORE INSERT ON portfolio_reconciliations
  FOR EACH ROW EXECUTE FUNCTION enforce_additive_reconciliation_evidence();

-- Positions are inserted after their parent report in the same transaction.
-- Validate the actual saved current quantities at commit, not only the JSON.
-- +goose StatementBegin
CREATE FUNCTION enforce_additive_current_positions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM jsonb_array_elements(NEW.changes) AS item WHERE item->>'control_impact'='ADDITIVE_INVENTORY_ONLY') THEN
    RETURN NEW;
  END IF;
  IF NEW.observed_position_count <> (SELECT count(*) FROM portfolio_reconciliation_positions
       WHERE reconciliation_id=NEW.id AND user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id)
     OR EXISTS (
       SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
       GROUP BY change->>'symbol',change->>'instrument_type',change->>'direction' HAVING count(*) > 1
     ) OR EXISTS (
       SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
       WHERE change->>'control_impact'='ADDITIVE_INVENTORY_ONLY' AND NOT EXISTS (
         SELECT 1 FROM portfolio_reconciliation_positions p
         WHERE p.reconciliation_id=NEW.id AND p.user_id=NEW.user_id AND p.financial_account_id=NEW.financial_account_id
           AND p.symbol=change->>'symbol' AND p.instrument_type='CRYPTO' AND p.direction='long'
           AND p.quantity=(change->>'current_quantity')::numeric
           AND p.available_quantity=(change->>'current_available_quantity')::numeric
           AND p.unavailable_to_trade_quantity=(change->>'current_unavailable_quantity')::numeric
       )
     ) THEN
    RAISE EXCEPTION 'additive current position chain must match immutable change evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER portfolio_reconciliation_additive_positions_guard
  AFTER INSERT ON portfolio_reconciliations DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION enforce_additive_current_positions();

-- +goose Down
-- Never rewrite immutable additions to make an older contract accept them.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM portfolio_reconciliations, jsonb_array_elements(changes) AS change
             WHERE change->>'control_impact'='ADDITIVE_INVENTORY_ONLY') THEN
    RAISE EXCEPTION 'cannot remove immutable additive reconciliation history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER portfolio_reconciliation_additive_guard ON portfolio_reconciliations;
DROP FUNCTION enforce_additive_reconciliation_evidence();
DROP TRIGGER portfolio_reconciliation_additive_positions_guard ON portfolio_reconciliations;
DROP FUNCTION enforce_additive_current_positions();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_reconciliation_change_impact() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  derived_blocking_count integer;
BEGIN
  IF NEW.change_count <> jsonb_array_length(NEW.changes) THEN
    RAISE EXCEPTION 'reconciliation change count mismatch';
  END IF;
  IF EXISTS (
    SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
    WHERE change->>'control_impact' IS NULL
       OR change->>'control_impact' NOT IN ('TRADABLE_INVENTORY','NON_TRADABLE_QUANTITY_ONLY')
  ) THEN
    RAISE EXCEPTION 'invalid reconciliation change control impact';
  END IF;
  IF NEW.provider_name <> 'coinbase' AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(NEW.changes) AS change
    WHERE change->>'control_impact'='NON_TRADABLE_QUANTITY_ONLY'
  ) THEN
    RAISE EXCEPTION 'non-tradable-only reconciliation impact requires Coinbase evidence';
  END IF;
  SELECT count(*) INTO derived_blocking_count
  FROM jsonb_array_elements(NEW.changes) AS change
  WHERE change->>'control_impact'='TRADABLE_INVENTORY';
  IF NEW.blocking_change_count <> derived_blocking_count THEN
    RAISE EXCEPTION 'reconciliation blocking change count mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
