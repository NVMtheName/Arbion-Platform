-- +goose Up
CREATE TABLE neural_engine_preferences (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  model_id text NOT NULL CHECK (length(model_id) BETWEEN 1 AND 255),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX neural_engine_preferences_connection_idx ON neural_engine_preferences(provider_connection_id);

-- +goose Down
DROP TABLE IF EXISTS neural_engine_preferences;
