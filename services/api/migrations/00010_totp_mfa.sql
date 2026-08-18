-- +goose Up
CREATE TABLE auth_totp_factors (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret_ciphertext bytea NOT NULL CHECK (octet_length(secret_ciphertext) >= 32),
  pending_expires_at timestamptz,
  enabled_at timestamptz,
  last_used_step bigint NOT NULL DEFAULT -1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (enabled_at IS NULL AND pending_expires_at IS NOT NULL) OR
    (enabled_at IS NOT NULL AND pending_expires_at IS NULL)
  )
);

CREATE TABLE auth_mfa_recovery_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth_totp_factors(user_id) ON DELETE CASCADE,
  code_hash bytea NOT NULL UNIQUE CHECK (octet_length(code_hash) = 32),
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_mfa_recovery_codes_available_idx
  ON auth_mfa_recovery_codes(user_id)
  WHERE used_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS auth_mfa_recovery_codes;
DROP TABLE IF EXISTS auth_totp_factors;
