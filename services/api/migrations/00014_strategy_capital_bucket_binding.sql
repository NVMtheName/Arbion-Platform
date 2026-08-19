-- +goose Up
ALTER TABLE capital_buckets
  ADD CONSTRAINT capital_buckets_id_user_unique UNIQUE (id,user_id);

ALTER TABLE strategy_instances ADD COLUMN capital_bucket_id uuid;
UPDATE strategy_instances i
SET capital_bucket_id=(v.snapshot->>'capital_bucket_id')::uuid
FROM automation_mandate_versions v
WHERE v.mandate_id=i.automation_mandate_id
  AND v.version_number=i.mandate_version;

-- Existing instances must have an exact immutable-version bucket binding. The
-- migration aborts rather than inventing authority when historical data is incomplete.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM strategy_instances WHERE capital_bucket_id IS NULL) THEN
    RAISE EXCEPTION 'strategy instance has no immutable capital bucket binding';
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE strategy_instances ALTER COLUMN capital_bucket_id SET NOT NULL;
ALTER TABLE strategy_instances
  ADD CONSTRAINT strategy_instances_capital_bucket_owner_fkey
  FOREIGN KEY (capital_bucket_id,user_id)
  REFERENCES capital_buckets(id,user_id) ON DELETE RESTRICT;

CREATE UNIQUE INDEX strategy_one_active_bucket_idx
  ON strategy_instances(user_id,capital_bucket_id)
  WHERE status IN ('ACTIVE','PAUSED');

-- +goose Down
DROP INDEX IF EXISTS strategy_one_active_bucket_idx;
ALTER TABLE strategy_instances DROP CONSTRAINT IF EXISTS strategy_instances_capital_bucket_owner_fkey;
ALTER TABLE strategy_instances DROP COLUMN IF EXISTS capital_bucket_id;
ALTER TABLE capital_buckets DROP CONSTRAINT IF EXISTS capital_buckets_id_user_unique;
