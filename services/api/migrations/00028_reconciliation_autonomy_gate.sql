-- +goose Up
ALTER TABLE portfolio_reconciliations
  ADD COLUMN autonomy_enforcement_active boolean NOT NULL DEFAULT false;

-- Rows created before this migration remain explicitly advisory. New rows use
-- the enforced contract without rewriting immutable historical evidence.
ALTER TABLE portfolio_reconciliations
  ALTER COLUMN autonomy_enforcement_active SET DEFAULT true;

ALTER TABLE portfolio_reconciliations
  DROP CONSTRAINT portfolio_reconciliations_blocks_new_actions_check;

ALTER TABLE portfolio_reconciliations
  ADD CONSTRAINT portfolio_reconciliations_autonomy_gate_check CHECK (
    (autonomy_enforcement_active=false AND blocks_new_actions=false) OR
    (autonomy_enforcement_active=true AND blocks_new_actions=(comparison_status <> 'MATCHED'))
  );

-- +goose Down
-- Enforced reconciliation evidence is immutable and cannot be made advisory.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM portfolio_reconciliations WHERE autonomy_enforcement_active) THEN
    RAISE EXCEPTION 'cannot remove enforced portfolio reconciliation history';
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE portfolio_reconciliations
  DROP CONSTRAINT IF EXISTS portfolio_reconciliations_autonomy_gate_check;
ALTER TABLE portfolio_reconciliations
  ADD CONSTRAINT portfolio_reconciliations_blocks_new_actions_check CHECK (blocks_new_actions=false);
ALTER TABLE portfolio_reconciliations
  DROP COLUMN autonomy_enforcement_active;
