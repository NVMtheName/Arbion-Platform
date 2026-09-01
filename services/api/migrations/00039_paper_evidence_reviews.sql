-- +goose Up
CREATE TABLE paper_evidence_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL CHECK (mandate_version > 0),
  evidence_hash bytea NOT NULL CHECK (octet_length(evidence_hash) = 32),
  gate_status text NOT NULL CHECK (gate_status = 'EVIDENCE_REVIEWABLE'),
  evidence_started_at timestamptz NOT NULL,
  evidence_eligible_at timestamptz NOT NULL,
  evidence_as_of timestamptz NOT NULL,
  evidence_window_hours bigint NOT NULL CHECK (evidence_window_hours >= 168),
  decision_count integer NOT NULL CHECK (decision_count >= 20),
  portfolio_version bigint NOT NULL CHECK (portfolio_version >= 0),
  portfolio_updated_at timestamptz NOT NULL,
  latest_checkpoint_run_id uuid NOT NULL REFERENCES nonlive_schedule_runs(id) ON DELETE RESTRICT,
  latest_checkpoint_as_of timestamptz NOT NULL,
  scheduler_sample_count integer NOT NULL CHECK (scheduler_sample_count >= 20),
  scheduler_success_count integer NOT NULL CHECK (scheduler_success_count >= 20),
  scheduler_failure_count integer NOT NULL CHECK (scheduler_failure_count >= 0),
  last_schedule_status text NOT NULL CHECK (last_schedule_status = 'SUCCEEDED'),
  consecutive_schedule_failures integer NOT NULL CHECK (consecutive_schedule_failures = 0),
  route_continuity_status text NOT NULL CHECK (route_continuity_status IN ('STABLE','CONTEXT_CHANGED')),
  input_coverage_status text NOT NULL CHECK (input_coverage_status = 'COMPLETE'),
  input_freshness_status text NOT NULL CHECK (input_freshness_status = 'CURRENT_AT_DECISION'),
  ledger_contract_status text NOT NULL CHECK (ledger_contract_status = 'RECONCILED'),
  no_live_safety_status text NOT NULL CHECK (no_live_safety_status = 'CLEAR'),
  execution_boundary text NOT NULL CHECK (execution_boundary = 'PAPER_SIMULATION_ONLY'),
  review_scope text NOT NULL CHECK (review_scope = 'PAPER_NON_LIVE_EVIDENCE_ONLY'),
  grants_authority boolean NOT NULL CHECK (grants_authority = false),
  live_promotion_available boolean NOT NULL CHECK (live_promotion_available = false),
  mfa_method text NOT NULL CHECK (mfa_method = 'totp'),
  reviewed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id,strategy_instance_id,evidence_hash),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT,
  CHECK (evidence_eligible_at = evidence_started_at + interval '168 hours'),
  CHECK (evidence_as_of >= evidence_eligible_at),
  CHECK (latest_checkpoint_as_of = evidence_as_of),
  CHECK (scheduler_success_count + scheduler_failure_count <= scheduler_sample_count)
);

CREATE INDEX paper_evidence_reviews_owner_instance_time_idx
  ON paper_evidence_reviews(user_id,strategy_instance_id,reviewed_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_paper_evidence_review_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM strategy_instances i
    WHERE i.id=NEW.strategy_instance_id
      AND i.user_id=NEW.user_id
      AND i.financial_account_id=NEW.financial_account_id
      AND i.automation_mandate_id=NEW.mandate_id
      AND i.mandate_version=NEW.mandate_version
      AND i.strategy_identifier='ai_shadow'
      AND i.execution_mode='PAPER'
      AND i.current_state='AI_MONITORING'
  ) THEN
    RAISE EXCEPTION 'Paper evidence review does not match its AI Paper strategy instance';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM nonlive_schedule_runs r
    WHERE r.id=NEW.latest_checkpoint_run_id
      AND r.user_id=NEW.user_id
      AND r.strategy_instance_id=NEW.strategy_instance_id
      AND r.mandate_id=NEW.mandate_id
      AND r.mandate_version=NEW.mandate_version
      AND r.execution_mode='PAPER'
      AND r.status='SUCCEEDED'
      AND r.consecutive_failures=0
      AND r.completed_at=NEW.latest_checkpoint_as_of
  ) THEN
    RAISE EXCEPTION 'Paper evidence review checkpoint is not the owner-bound successful Paper run';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM nonlive_schedule_runs r
    WHERE r.user_id=NEW.user_id
      AND r.strategy_instance_id=NEW.strategy_instance_id
      AND (r.completed_at,r.id) > (NEW.latest_checkpoint_as_of,NEW.latest_checkpoint_run_id)
  ) THEN
    RAISE EXCEPTION 'Paper evidence review checkpoint is stale';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM paper_portfolios p
    WHERE p.user_id=NEW.user_id
      AND p.strategy_instance_id=NEW.strategy_instance_id
      AND p.version=NEW.portfolio_version
      AND p.updated_at=NEW.portfolio_updated_at
  ) THEN
    RAISE EXCEPTION 'Paper evidence review portfolio snapshot changed';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER paper_evidence_review_source_guard
  BEFORE INSERT ON paper_evidence_reviews
  FOR EACH ROW EXECUTE FUNCTION enforce_paper_evidence_review_source();
CREATE TRIGGER paper_evidence_reviews_immutable
  BEFORE UPDATE OR DELETE ON paper_evidence_reviews
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM paper_evidence_reviews) THEN
    RAISE EXCEPTION 'cannot remove immutable Paper evidence review history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS paper_evidence_reviews_immutable ON paper_evidence_reviews;
DROP TRIGGER IF EXISTS paper_evidence_review_source_guard ON paper_evidence_reviews;
DROP FUNCTION IF EXISTS enforce_paper_evidence_review_source();
DROP TABLE IF EXISTS paper_evidence_reviews;
