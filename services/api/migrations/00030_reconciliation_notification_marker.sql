-- +goose Up
-- A successful informational drift-review delivery is remembered by the
-- immutable reconciliation evidence ID. The marker cannot acknowledge drift,
-- mutate evidence, or grant any execution authority.
ALTER TABLE nonlive_strategy_schedules
  ADD COLUMN last_reconciliation_notification_id uuid REFERENCES portfolio_reconciliations(id) ON DELETE RESTRICT,
  ADD COLUMN last_reconciliation_notification_at timestamptz,
  ADD CONSTRAINT nonlive_strategy_schedules_reconciliation_notification_pair CHECK (
    (last_reconciliation_notification_id IS NULL) = (last_reconciliation_notification_at IS NULL)
  );

-- +goose StatementBegin
CREATE FUNCTION enforce_reconciliation_notification_marker() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.last_reconciliation_notification_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
    FROM portfolio_reconciliations r
    JOIN strategy_instances i
      ON i.id=NEW.strategy_instance_id
     AND i.user_id=NEW.user_id
     AND i.financial_account_id=r.financial_account_id
    WHERE r.id=NEW.last_reconciliation_notification_id
      AND r.user_id=NEW.user_id
      AND r.comparison_status='DRIFT_DETECTED'
      AND r.blocking_change_count > 0
      AND r.blocks_new_actions=true
  ) THEN
    RAISE EXCEPTION 'reconciliation notification marker is not blocking owner-account evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER nonlive_schedule_reconciliation_notification_guard
  BEFORE INSERT OR UPDATE
  ON nonlive_strategy_schedules
  FOR EACH ROW EXECUTE FUNCTION enforce_reconciliation_notification_marker();

-- +goose Down
DROP TRIGGER IF EXISTS nonlive_schedule_reconciliation_notification_guard ON nonlive_strategy_schedules;
DROP FUNCTION IF EXISTS enforce_reconciliation_notification_marker();
ALTER TABLE nonlive_strategy_schedules
  DROP CONSTRAINT IF EXISTS nonlive_strategy_schedules_reconciliation_notification_pair,
  DROP COLUMN IF EXISTS last_reconciliation_notification_at,
  DROP COLUMN IF EXISTS last_reconciliation_notification_id;
