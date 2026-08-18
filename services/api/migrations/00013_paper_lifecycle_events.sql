-- +goose Up
CREATE TABLE strategy_lifecycle_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  strategy_instance_id uuid NOT NULL,
  user_id uuid NOT NULL,
  event_id text NOT NULL CHECK (length(event_id) BETWEEN 1 AND 200),
  event_type text NOT NULL CHECK (event_type IN ('EXPIRE_WORTHLESS','ASSIGNED','CALLED_AWAY')),
  previous_state text NOT NULL,
  new_state text NOT NULL,
  state_version integer NOT NULL CHECK (state_version > 0),
  metadata jsonb NOT NULL CHECK (jsonb_typeof(metadata) = 'object'),
  occurred_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (strategy_instance_id,event_id),
  UNIQUE (strategy_instance_id,state_version),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT
);

CREATE INDEX strategy_lifecycle_events_owner_idx
  ON strategy_lifecycle_events(user_id,occurred_at DESC,id DESC);

CREATE TRIGGER strategy_lifecycle_event_immutable
  BEFORE UPDATE OR DELETE ON strategy_lifecycle_events
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS strategy_lifecycle_event_immutable ON strategy_lifecycle_events;
DROP TABLE IF EXISTS strategy_lifecycle_events;
