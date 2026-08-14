-- +goose Up
CREATE TABLE financial_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  provider_name text NOT NULL,
  provider_account_id text NOT NULL,
  display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 160),
  masked_identifier text NOT NULL DEFAULT '',
  account_type text NOT NULL DEFAULT 'unknown',
  base_currency text NOT NULL DEFAULT 'USD' CHECK (length(base_currency) = 3),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled','closed','unavailable')),
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities) = 'object'),
  discovered_at timestamptz NOT NULL DEFAULT now(),
  last_synced_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider_connection_id, provider_account_id),
  UNIQUE(id, user_id)
);
CREATE INDEX financial_accounts_user_idx ON financial_accounts(user_id, provider_name);
CREATE INDEX financial_accounts_connection_idx ON financial_accounts(provider_connection_id);

-- Prevent cross-user or non-financial connection links even if an application bug supplies them.
-- +goose StatementBegin
CREATE FUNCTION enforce_financial_account_connection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM provider_connections p WHERE p.id=NEW.provider_connection_id AND p.user_id=NEW.user_id AND p.provider_category='financial') THEN
    RAISE EXCEPTION 'financial account connection owner/category mismatch';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER financial_accounts_connection_guard BEFORE INSERT OR UPDATE ON financial_accounts FOR EACH ROW EXECUTE FUNCTION enforce_financial_account_connection();

-- +goose Down
DROP TRIGGER IF EXISTS financial_accounts_connection_guard ON financial_accounts;
DROP FUNCTION IF EXISTS enforce_financial_account_connection();
DROP TABLE IF EXISTS financial_accounts;
