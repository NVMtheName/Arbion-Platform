-- +goose Up
-- Forward-only evidence for an existing provider-account discovery operation.
-- These rows intentionally contain only account identity and operation timing.
CREATE TABLE financial_account_sync_operations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  provider_name text NOT NULL CHECK (provider_name ~ '^[a-z][a-z0-9_]{0,31}$'),
  source_operation text NOT NULL CHECK (source_operation = 'PROVIDER_ACCOUNT_DISCOVERY'),
  outcome text NOT NULL CHECK (outcome = 'SAVED'),
  account_count integer NOT NULL CHECK (account_count > 0),
  observed_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (completed_at >= observed_at),
  CHECK (created_at >= observed_at)
);

CREATE TABLE financial_account_sync_checkpoints (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  operation_id uuid NOT NULL REFERENCES financial_account_sync_operations(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_name text NOT NULL CHECK (provider_name ~ '^[a-z][a-z0-9_]{0,31}$'),
  observed_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (operation_id,financial_account_id),
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  CHECK (completed_at >= observed_at),
  CHECK (created_at >= observed_at)
);

CREATE INDEX financial_account_sync_checkpoints_owner_account_time_idx
  ON financial_account_sync_checkpoints(user_id,financial_account_id,completed_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_financial_account_sync_operation_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM provider_connections p
    WHERE p.id=NEW.provider_connection_id
      AND p.user_id=NEW.user_id
      AND p.provider_category='financial'
      AND p.provider_name=NEW.provider_name
  ) THEN
    RAISE EXCEPTION 'financial account sync operation connection identity mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION enforce_financial_account_sync_checkpoint_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM financial_account_sync_operations o
    JOIN financial_accounts a
      ON a.id=NEW.financial_account_id
     AND a.user_id=NEW.user_id
     AND a.provider_connection_id=NEW.provider_connection_id
     AND a.provider_name=NEW.provider_name
    WHERE o.id=NEW.operation_id
      AND o.user_id=NEW.user_id
      AND o.provider_connection_id=NEW.provider_connection_id
      AND o.provider_name=NEW.provider_name
      AND o.observed_at=NEW.observed_at
      AND o.completed_at=NEW.completed_at
  ) THEN
    RAISE EXCEPTION 'financial account sync checkpoint identity mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION reject_financial_account_sync_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'financial account sync history is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER financial_account_sync_operation_source_guard
  BEFORE INSERT ON financial_account_sync_operations
  FOR EACH ROW EXECUTE FUNCTION enforce_financial_account_sync_operation_source();
CREATE TRIGGER financial_account_sync_checkpoint_source_guard
  BEFORE INSERT ON financial_account_sync_checkpoints
  FOR EACH ROW EXECUTE FUNCTION enforce_financial_account_sync_checkpoint_source();
CREATE TRIGGER financial_account_sync_operations_immutable
  BEFORE UPDATE OR DELETE ON financial_account_sync_operations
  FOR EACH ROW EXECUTE FUNCTION reject_financial_account_sync_history_mutation();
CREATE TRIGGER financial_account_sync_checkpoints_immutable
  BEFORE UPDATE OR DELETE ON financial_account_sync_checkpoints
  FOR EACH ROW EXECUTE FUNCTION reject_financial_account_sync_history_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM financial_account_sync_operations) OR
     EXISTS (SELECT 1 FROM financial_account_sync_checkpoints) THEN
    RAISE EXCEPTION 'cannot remove immutable financial account sync history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS financial_account_sync_checkpoints_immutable ON financial_account_sync_checkpoints;
DROP TRIGGER IF EXISTS financial_account_sync_operations_immutable ON financial_account_sync_operations;
DROP TRIGGER IF EXISTS financial_account_sync_checkpoint_source_guard ON financial_account_sync_checkpoints;
DROP TRIGGER IF EXISTS financial_account_sync_operation_source_guard ON financial_account_sync_operations;
DROP FUNCTION IF EXISTS reject_financial_account_sync_history_mutation();
DROP FUNCTION IF EXISTS enforce_financial_account_sync_checkpoint_source();
DROP FUNCTION IF EXISTS enforce_financial_account_sync_operation_source();
DROP TABLE IF EXISTS financial_account_sync_checkpoints;
DROP TABLE IF EXISTS financial_account_sync_operations;
