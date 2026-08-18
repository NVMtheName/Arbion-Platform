-- +goose Up
CREATE TABLE auth_email_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (consumed_at IS NULL OR consumed_at >= created_at)
);
CREATE UNIQUE INDEX auth_email_tokens_one_active_idx
  ON auth_email_tokens(user_id, purpose)
  WHERE consumed_at IS NULL;
CREATE INDEX auth_email_tokens_expiry_idx
  ON auth_email_tokens(expires_at)
  WHERE consumed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth_email_tokens;
