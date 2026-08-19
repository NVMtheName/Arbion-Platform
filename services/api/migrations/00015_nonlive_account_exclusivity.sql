-- +goose Up
-- Until aggregate reservation accounting is implemented, fail closed by allowing
-- only one active or paused non-live strategy to claim a financial account.
CREATE UNIQUE INDEX strategy_one_active_account_idx
  ON strategy_instances(user_id,financial_account_id)
  WHERE status IN ('ACTIVE','PAUSED');

-- +goose Down
DROP INDEX IF EXISTS strategy_one_active_account_idx;
