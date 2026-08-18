-- +goose Up
CREATE TABLE nonlive_strategy_schedules (
  strategy_instance_id uuid PRIMARY KEY REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  user_id uuid NOT NULL,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL,
  interval_minutes integer NOT NULL CHECK (interval_minutes BETWEEN 30 AND 1440),
  session text NOT NULL CHECK (session = 'US_EQUITIES_REGULAR'),
  next_run_at timestamptz NOT NULL,
  lease_token uuid,
  lease_expires_at timestamptz,
  last_started_at timestamptz,
  last_completed_at timestamptz,
  last_status text CHECK (last_status IN ('SUCCEEDED','FAILED','SKIPPED')),
  last_error_code text,
  consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT,
  CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL))
);

CREATE INDEX nonlive_strategy_schedules_due_idx
  ON nonlive_strategy_schedules(next_run_at)
  WHERE lease_token IS NULL;

-- +goose Down
DROP TABLE IF EXISTS nonlive_strategy_schedules;
