-- +goose Up
-- Preserve every historical decision exactly: prior DRIFT_DETECTED records
-- remain blocking, while new MATCHED records may carry exact
-- non-tradable-only quantity evidence without blocking autonomy.
ALTER TABLE portfolio_reconciliations
  ADD COLUMN blocking_change_count integer NOT NULL DEFAULT 0;

-- The immutable source evidence is not rewritten. The trigger is suspended
-- only while adding this derived control classification to historical rows.
ALTER TABLE portfolio_reconciliations
  DISABLE TRIGGER portfolio_reconciliations_immutable;
UPDATE portfolio_reconciliations
SET blocking_change_count = CASE
  WHEN comparison_status='DRIFT_DETECTED' THEN change_count
  ELSE 0
END;
ALTER TABLE portfolio_reconciliations
  ENABLE TRIGGER portfolio_reconciliations_immutable;

ALTER TABLE portfolio_reconciliations
  DROP CONSTRAINT portfolio_reconciliations_check1;

ALTER TABLE portfolio_reconciliations
  ADD CONSTRAINT portfolio_reconciliations_blocking_change_count_check CHECK (
    blocking_change_count BETWEEN 0 AND change_count AND
    ((comparison_status='DRIFT_DETECTED') = (blocking_change_count > 0))
  );

-- +goose StatementBegin
CREATE FUNCTION enforce_reconciliation_change_impact() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE TRIGGER portfolio_reconciliation_change_impact_guard
  BEFORE INSERT ON portfolio_reconciliations
  FOR EACH ROW EXECUTE FUNCTION enforce_reconciliation_change_impact();

-- +goose Down
-- MATCHED evidence with exact non-tradable-only changes cannot satisfy the old
-- contract and is immutable, so rollback must refuse instead of rewriting it.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM portfolio_reconciliations
    WHERE comparison_status='MATCHED' AND change_count > 0
  ) THEN
    RAISE EXCEPTION 'cannot remove classified non-tradable reconciliation history';
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE portfolio_reconciliations
  DROP CONSTRAINT IF EXISTS portfolio_reconciliations_blocking_change_count_check;
DROP TRIGGER IF EXISTS portfolio_reconciliation_change_impact_guard ON portfolio_reconciliations;
DROP FUNCTION IF EXISTS enforce_reconciliation_change_impact();
ALTER TABLE portfolio_reconciliations
  DROP COLUMN blocking_change_count;
ALTER TABLE portfolio_reconciliations
  ADD CONSTRAINT portfolio_reconciliations_check1 CHECK (
    (comparison_status='DRIFT_DETECTED') = (change_count > 0)
  );
