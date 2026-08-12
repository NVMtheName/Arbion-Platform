-- +goose Up
ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'user'
  CONSTRAINT users_role_check CHECK (role IN ('user','admin','superadmin'));

CREATE TABLE user_entitlements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entitlement_key text NOT NULL CHECK (entitlement_key IN ('free','pro','premium','founder','internal_comped')),
  source text NOT NULL DEFAULT 'system' CHECK (source IN ('system','admin','bootstrap','billing_provider')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
  billing_required boolean NOT NULL DEFAULT false,
  starts_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT founder_is_permanent_and_comped CHECK (entitlement_key <> 'founder' OR (billing_required = false AND expires_at IS NULL)),
  UNIQUE (user_id, entitlement_key)
);
CREATE INDEX user_entitlements_effective_idx ON user_entitlements(user_id, status, expires_at);
INSERT INTO user_entitlements(user_id, entitlement_key, source, billing_required)
SELECT id, 'free', 'system', false FROM users;

-- +goose Down
DROP TABLE IF EXISTS user_entitlements;
ALTER TABLE users DROP COLUMN IF EXISTS role;
