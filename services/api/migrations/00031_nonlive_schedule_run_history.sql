-- +goose Up
CREATE TABLE nonlive_schedule_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL CHECK (mandate_version > 0),
  execution_mode text NOT NULL CHECK (execution_mode IN ('PAPER','SHADOW')),
  strategy_state text NOT NULL CHECK (length(strategy_state) BETWEEN 1 AND 80),
  scheduled_for timestamptz NOT NULL,
  started_at timestamptz NOT NULL,
  completed_at timestamptz NOT NULL,
  next_run_at timestamptz NOT NULL,
  status text NOT NULL CHECK (status IN ('SUCCEEDED','FAILED','SKIPPED')),
  error_code text CHECK (error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  ai_decision text CHECK (ai_decision IN ('ABSTAIN','PROPOSE')),
  execution_status text CHECK (execution_status IN ('PROPOSED','RISK_DENIED','SIMULATED_FILLED','SIMULATED_REJECTED','WOULD_HAVE_SUBMITTED','CANCELED','ERROR')),
  duplicate_recovered boolean NOT NULL DEFAULT false,
  reconciliation_id uuid REFERENCES portfolio_reconciliations(id) ON DELETE RESTRICT,
  reconciliation_review_required boolean NOT NULL DEFAULT false,
  consecutive_failures integer NOT NULL CHECK (consecutive_failures >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (strategy_instance_id,scheduled_for),
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT,
  CHECK (started_at >= scheduled_for),
  CHECK (completed_at >= started_at),
  CHECK (next_run_at > scheduled_for),
  CHECK ((status='SUCCEEDED' AND error_code IS NULL) OR (status IN ('FAILED','SKIPPED') AND error_code IS NOT NULL)),
  CHECK (duplicate_recovered=false OR status='SUCCEEDED'),
  CHECK (reconciliation_review_required=false OR reconciliation_id IS NOT NULL)
);

CREATE INDEX nonlive_schedule_runs_owner_instance_time_idx
  ON nonlive_schedule_runs(user_id,strategy_instance_id,scheduled_for DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_nonlive_schedule_run_source() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM strategy_instances i
    WHERE i.id=NEW.strategy_instance_id
      AND i.user_id=NEW.user_id
      AND i.automation_mandate_id=NEW.mandate_id
      AND i.mandate_version=NEW.mandate_version
      AND i.execution_mode=NEW.execution_mode
  ) THEN
    RAISE EXCEPTION 'non-live schedule run does not match its strategy instance';
  END IF;
  IF NEW.reconciliation_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
    FROM portfolio_reconciliations r
    JOIN strategy_instances i
      ON i.id=NEW.strategy_instance_id
     AND i.user_id=NEW.user_id
     AND i.financial_account_id=r.financial_account_id
    WHERE r.id=NEW.reconciliation_id
      AND r.user_id=NEW.user_id
  ) THEN
    RAISE EXCEPTION 'non-live schedule run reconciliation does not match its owner account';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION reject_nonlive_schedule_run_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'non-live schedule run history is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER nonlive_schedule_run_source_guard
  BEFORE INSERT ON nonlive_schedule_runs
  FOR EACH ROW EXECUTE FUNCTION enforce_nonlive_schedule_run_source();
CREATE TRIGGER nonlive_schedule_runs_immutable
  BEFORE UPDATE OR DELETE ON nonlive_schedule_runs
  FOR EACH ROW EXECUTE FUNCTION reject_nonlive_schedule_run_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM nonlive_schedule_runs) THEN
    RAISE EXCEPTION 'cannot remove immutable non-live schedule run history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS nonlive_schedule_runs_immutable ON nonlive_schedule_runs;
DROP TRIGGER IF EXISTS nonlive_schedule_run_source_guard ON nonlive_schedule_runs;
DROP FUNCTION IF EXISTS reject_nonlive_schedule_run_mutation();
DROP FUNCTION IF EXISTS enforce_nonlive_schedule_run_source();
DROP TABLE IF EXISTS nonlive_schedule_runs;
