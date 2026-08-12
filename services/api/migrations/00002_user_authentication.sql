-- +goose Up
ALTER TABLE users
  ADD COLUMN email text,
  ADD COLUMN normalized_email text,
  ADD COLUMN password_hash text,
  ADD COLUMN display_name text,
  ADD COLUMN status text NOT NULL DEFAULT 'active',
  ADD COLUMN email_verified_at timestamptz,
  ADD COLUMN last_login_at timestamptz,
  ADD CONSTRAINT users_status_check CHECK (status IN ('active','disabled','pending_verification')),
  ADD CONSTRAINT users_email_pair_check CHECK ((email IS NULL) = (normalized_email IS NULL)),
  ADD CONSTRAINT users_password_with_email_check CHECK (password_hash IS NULL OR normalized_email IS NOT NULL),
  ADD CONSTRAINT users_normalized_email_format_check CHECK (normalized_email IS NULL OR (normalized_email = lower(btrim(normalized_email)) AND length(normalized_email) BETWEEN 3 AND 320));
CREATE UNIQUE INDEX users_normalized_email_unique_idx ON users(normalized_email) WHERE normalized_email IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS users_normalized_email_unique_idx;
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_normalized_email_format_check,
  DROP CONSTRAINT IF EXISTS users_password_with_email_check,
  DROP CONSTRAINT IF EXISTS users_email_pair_check,
  DROP CONSTRAINT IF EXISTS users_status_check,
  DROP COLUMN IF EXISTS last_login_at,
  DROP COLUMN IF EXISTS email_verified_at,
  DROP COLUMN IF EXISTS status,
  DROP COLUMN IF EXISTS display_name,
  DROP COLUMN IF EXISTS password_hash,
  DROP COLUMN IF EXISTS normalized_email,
  DROP COLUMN IF EXISTS email;
