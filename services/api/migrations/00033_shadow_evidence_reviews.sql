-- +goose Up
CREATE TABLE shadow_evidence_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL CHECK (mandate_version > 0),
  evidence_hash bytea NOT NULL CHECK (octet_length(evidence_hash) = 32),
  gate_status text NOT NULL CHECK (gate_status = 'EVIDENCE_REVIEWABLE'),
  one_hour_sample_size integer NOT NULL CHECK (one_hour_sample_size >= 20),
  twenty_four_hour_sample_size integer NOT NULL CHECK (twenty_four_hour_sample_size >= 20),
  evidence_window_hours bigint NOT NULL CHECK (evidence_window_hours >= 168),
  schedule_healthy boolean NOT NULL CHECK (schedule_healthy = true),
  last_schedule_status text NOT NULL CHECK (last_schedule_status = 'SUCCEEDED'),
  consecutive_schedule_failures integer NOT NULL CHECK (consecutive_schedule_failures = 0),
  execution_boundary text NOT NULL CHECK (execution_boundary = 'SHADOW_ONLY'),
  live_execution_available boolean NOT NULL CHECK (live_execution_available = false),
  review_scope text NOT NULL CHECK (review_scope = 'NON_LIVE_EVIDENCE_ONLY'),
  mfa_method text NOT NULL CHECK (mfa_method = 'totp'),
  reviewed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id,strategy_instance_id,evidence_hash),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);

CREATE INDEX shadow_evidence_reviews_owner_instance_time_idx
  ON shadow_evidence_reviews(user_id,strategy_instance_id,reviewed_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_shadow_evidence_review_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM strategy_instances i
    WHERE i.id=NEW.strategy_instance_id
      AND i.user_id=NEW.user_id
      AND i.automation_mandate_id=NEW.mandate_id
      AND i.mandate_version=NEW.mandate_version
      AND i.strategy_identifier='ai_shadow'
      AND i.execution_mode='SHADOW'
      AND i.current_state='AI_MONITORING'
  ) THEN
    RAISE EXCEPTION 'shadow evidence review does not match its AI Shadow strategy instance';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER shadow_evidence_review_source_guard
  BEFORE INSERT ON shadow_evidence_reviews
  FOR EACH ROW EXECUTE FUNCTION enforce_shadow_evidence_review_source();
CREATE TRIGGER shadow_evidence_reviews_immutable
  BEFORE UPDATE OR DELETE ON shadow_evidence_reviews
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM shadow_evidence_reviews) THEN
    RAISE EXCEPTION 'cannot remove immutable Shadow evidence review history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS shadow_evidence_reviews_immutable ON shadow_evidence_reviews;
DROP TRIGGER IF EXISTS shadow_evidence_review_source_guard ON shadow_evidence_reviews;
DROP FUNCTION IF EXISTS enforce_shadow_evidence_review_source();
DROP TABLE IF EXISTS shadow_evidence_reviews;
