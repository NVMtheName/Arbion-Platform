-- +goose Up
-- Forward-only, credential-free failure evidence for provider account discovery.
-- Successful attempts remain in financial_account_sync_operations.
CREATE TABLE financial_connection_sync_failures (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  provider_name text NOT NULL CHECK (provider_name ~ '^[a-z][a-z0-9_]{0,31}$'),
  source_operation text NOT NULL CHECK (source_operation = 'PROVIDER_ACCOUNT_DISCOVERY'),
  outcome text NOT NULL CHECK (outcome = 'FAILED'),
  failure_stage text NOT NULL CHECK (failure_stage IN ('CREDENTIAL_ACCESS','ACCOUNT_DISCOVERY','PERSISTENCE')),
  error_code text NOT NULL CHECK (error_code IN (
    'AUTHORIZATION_FAILED','INVALID_CREDENTIAL_FORMAT','AUTHORIZATION_EXPIRED',
    'PROVIDER_UNAVAILABLE','RATE_LIMITED','TIMEOUT','ACCOUNT_NOT_FOUND',
    'PERMISSION_DENIED','INVALID_PROVIDER_RESPONSE','CONNECTION_DISABLED',
    'CREDENTIAL_UNAVAILABLE','INTERNAL_ERROR'
  )),
  observed_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (completed_at >= observed_at),
  CHECK (created_at >= observed_at)
);

CREATE INDEX financial_connection_sync_failures_owner_connection_time_idx
  ON financial_connection_sync_failures(user_id,provider_connection_id,completed_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_financial_connection_sync_failure_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM provider_connections p
    WHERE p.id=NEW.provider_connection_id
      AND p.user_id=NEW.user_id
      AND p.provider_category='financial'
      AND p.provider_name=NEW.provider_name
  ) THEN
    RAISE EXCEPTION 'financial connection sync failure identity mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION reject_financial_connection_sync_failure_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'financial connection sync failure history is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER financial_connection_sync_failure_source_guard
  BEFORE INSERT ON financial_connection_sync_failures
  FOR EACH ROW EXECUTE FUNCTION enforce_financial_connection_sync_failure_source();
CREATE TRIGGER financial_connection_sync_failures_immutable
  BEFORE UPDATE OR DELETE ON financial_connection_sync_failures
  FOR EACH ROW EXECUTE FUNCTION reject_financial_connection_sync_failure_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM financial_connection_sync_failures) THEN
    RAISE EXCEPTION 'cannot remove immutable financial connection sync failure history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS financial_connection_sync_failures_immutable ON financial_connection_sync_failures;
DROP TRIGGER IF EXISTS financial_connection_sync_failure_source_guard ON financial_connection_sync_failures;
DROP FUNCTION IF EXISTS reject_financial_connection_sync_failure_mutation();
DROP FUNCTION IF EXISTS enforce_financial_connection_sync_failure_source();
DROP TABLE IF EXISTS financial_connection_sync_failures;
