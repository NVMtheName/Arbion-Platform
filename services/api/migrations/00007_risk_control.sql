-- +goose Up
CREATE TABLE risk_circuit_breakers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope text NOT NULL CHECK (scope IN ('AUTOMATION','ACCOUNT','USER','GLOBAL')),
  scope_id uuid, state text NOT NULL CHECK (state IN ('OPEN','CLOSED')), reason text NOT NULL, source text NOT NULL,
  engaged_by_user_id uuid REFERENCES users(id) ON DELETE RESTRICT, engaged_at timestamptz NOT NULL DEFAULT now(),
  released_by_user_id uuid REFERENCES users(id) ON DELETE RESTRICT, released_at timestamptz,
  CHECK ((scope='GLOBAL' AND scope_id IS NULL) OR (scope<>'GLOBAL' AND scope_id IS NOT NULL)),
  CHECK ((state='OPEN' AND released_at IS NULL) OR state='CLOSED')
);
CREATE UNIQUE INDEX risk_breaker_one_open_scope_idx ON risk_circuit_breakers(scope,COALESCE(scope_id,'00000000-0000-0000-0000-000000000000')) WHERE state='OPEN';
CREATE TABLE risk_evaluations (
  id uuid PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, financial_account_id uuid NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
  proposed_action_id text NOT NULL, correlation_id text, mandate_id uuid REFERENCES automation_mandates(id) ON DELETE RESTRICT, mandate_version integer,
  decision text NOT NULL CHECK (decision IN ('ALLOW','DENY','WARN')), approval_required boolean NOT NULL, execution_mode text,
  platform_execution_available boolean NOT NULL DEFAULT false CHECK (platform_execution_available=false), reason_codes jsonb NOT NULL, checks jsonb NOT NULL,
  evaluated_at timestamptz NOT NULL DEFAULT now(), CHECK (jsonb_typeof(reason_codes)='array'), CHECK (jsonb_typeof(checks)='array')
);
CREATE INDEX risk_evaluations_user_time_idx ON risk_evaluations(user_id,evaluated_at DESC);
-- +goose Down
DROP TABLE IF EXISTS risk_evaluations;
DROP TABLE IF EXISTS risk_circuit_breakers;
