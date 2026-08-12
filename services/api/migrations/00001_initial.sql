-- +goose Up
CREATE TABLE users (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), external_id text UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE provider_connections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 provider_category text NOT NULL CHECK (provider_category IN ('ai','financial')), provider_name text NOT NULL, display_name text NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','expired','revoked','error','disabled')),
 scopes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scopes)='array'), credential_storage text NOT NULL DEFAULT 'encrypted_database' CHECK (credential_storage IN ('encrypted_database','managed_reference')),
 encrypted_credential_payload bytea, credential_reference text, credential_metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(credential_metadata)='object'), token_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), last_verified_at timestamptz,
 CONSTRAINT provider_credential_location CHECK ((credential_storage='encrypted_database' AND credential_reference IS NULL) OR (credential_storage='managed_reference' AND encrypted_credential_payload IS NULL)),
 UNIQUE(user_id,provider_category,provider_name,display_name));
CREATE INDEX provider_connections_user_id_idx ON provider_connections(user_id);
CREATE INDEX provider_connections_active_idx ON provider_connections(user_id,provider_category) WHERE status='active';
CREATE TABLE automation_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 financial_provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT, ai_provider_connection_id uuid REFERENCES provider_connections(id) ON DELETE SET NULL,
 enabled boolean NOT NULL DEFAULT false, mode text NOT NULL DEFAULT 'paper' CHECK(mode IN ('paper','live')), strategy_identifier text NOT NULL,
 strategy_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(strategy_config)='object'), risk_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(risk_config)='object'),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), last_run_at timestamptz);
CREATE INDEX automation_configs_user_id_idx ON automation_configs(user_id);
CREATE INDEX automation_configs_enabled_idx ON automation_configs(enabled) WHERE enabled;
CREATE TABLE audit_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid REFERENCES users(id) ON DELETE SET NULL, actor_type text NOT NULL, actor_id text,
 action text NOT NULL, target_type text NOT NULL, target_id text, occurred_at timestamptz NOT NULL DEFAULT now(), metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(metadata)='object'), correlation_id text);
CREATE INDEX audit_events_user_time_idx ON audit_events(user_id,occurred_at DESC);
CREATE INDEX audit_events_correlation_idx ON audit_events(correlation_id) WHERE correlation_id IS NOT NULL;
-- +goose Down
DROP TABLE IF EXISTS audit_events; DROP TABLE IF EXISTS automation_configs; DROP TABLE IF EXISTS provider_connections; DROP TABLE IF EXISTS users;
