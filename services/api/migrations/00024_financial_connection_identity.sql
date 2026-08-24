-- +goose Up
-- A provider account is a stable owner-scoped identity. Reauthorization and
-- resync must update that identity rather than attach it to another connection.
CREATE UNIQUE INDEX financial_accounts_owner_provider_identity_idx
  ON financial_accounts(user_id,provider_name,provider_account_id);

-- +goose Down
DROP INDEX IF EXISTS financial_accounts_owner_provider_identity_idx;
