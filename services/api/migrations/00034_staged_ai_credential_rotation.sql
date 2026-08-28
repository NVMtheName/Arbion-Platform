-- +goose Up
ALTER TABLE provider_connections
  ADD COLUMN credential_generation bigint NOT NULL DEFAULT 1 CHECK (credential_generation > 0),
  ADD COLUMN pending_encrypted_credential_payload bytea,
  ADD COLUMN pending_credential_token text,
  ADD CONSTRAINT provider_pending_credential_pair_check CHECK (
    (pending_encrypted_credential_payload IS NULL) = (pending_credential_token IS NULL)
    AND (pending_encrypted_credential_payload IS NULL OR octet_length(pending_encrypted_credential_payload) >= 28)
  ),
  ADD CONSTRAINT provider_pending_credential_token_check CHECK (
    pending_credential_token IS NULL OR pending_credential_token ~ '^[0-9a-f]{64}$'
  );

COMMENT ON COLUMN provider_connections.credential_generation IS
  'Monotonic guard preventing stale verification from changing a newer credential state.';
COMMENT ON COLUMN provider_connections.pending_encrypted_credential_payload IS
  'Encrypted replacement candidate; never selected by runtime credential reads.';
COMMENT ON COLUMN provider_connections.pending_credential_token IS
  'Random concurrency token binding verification to the exact staged candidate.';

-- +goose Down
ALTER TABLE provider_connections
  DROP CONSTRAINT IF EXISTS provider_pending_credential_token_check,
  DROP CONSTRAINT IF EXISTS provider_pending_credential_pair_check,
  DROP COLUMN IF EXISTS pending_credential_token,
  DROP COLUMN IF EXISTS pending_encrypted_credential_payload,
  DROP COLUMN IF EXISTS credential_generation;
