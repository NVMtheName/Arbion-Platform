-- +goose Up
CREATE TABLE strategy_instances (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  automation_mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL, financial_account_id uuid NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
  strategy_identifier text NOT NULL CHECK (strategy_identifier IN ('wheel','covered_call','cash_secured_put')),
  strategy_definition_version integer NOT NULL DEFAULT 1, execution_mode text NOT NULL CHECK (execution_mode IN ('PAPER','SHADOW')),
  current_state text NOT NULL, state_version integer NOT NULL DEFAULT 1 CHECK (state_version > 0),
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','PAUSED','COMPLETED','ERROR')),
  started_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), paused_at timestamptz,
  completed_at timestamptz, last_evaluated_at timestamptz,
  UNIQUE (id,user_id), UNIQUE (id,state_version),
  FOREIGN KEY (automation_mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX strategy_one_active_mandate_version_idx ON strategy_instances(automation_mandate_id,mandate_version) WHERE status IN ('ACTIVE','PAUSED');

CREATE TABLE strategy_evaluation_events (
  strategy_instance_id uuid NOT NULL REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  event_id text NOT NULL CHECK (length(event_id) BETWEEN 1 AND 200), status text NOT NULL CHECK(status IN ('CLAIMED','COMMITTED','ERROR')),
  created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz, PRIMARY KEY(strategy_instance_id,event_id)
);
CREATE TABLE paper_portfolios (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  strategy_instance_id uuid NOT NULL UNIQUE REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  currency char(3) NOT NULL, starting_cash numeric(30,10) NOT NULL CHECK(starting_cash >= 0),
  cash numeric(30,10) NOT NULL CHECK(cash >= 0), version bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE paper_positions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), paper_portfolio_id uuid NOT NULL REFERENCES paper_portfolios(id) ON DELETE RESTRICT,
  symbol text NOT NULL, instrument text NOT NULL CHECK(instrument IN ('EQUITY','OPTION')),
  option_type text CHECK(option_type IN ('PUT','CALL')), strike numeric(30,10), expiration date,
  quantity numeric(30,10) NOT NULL, average_price numeric(30,10) NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(metadata)='object'), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(paper_portfolio_id,symbol,instrument,option_type,strike,expiration)
);
CREATE TABLE nonlive_execution_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), idempotency_key text NOT NULL UNIQUE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, strategy_instance_id uuid NOT NULL REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT, mandate_version integer NOT NULL,
  proposed_action_id text NOT NULL, risk_evaluation_id uuid NOT NULL REFERENCES risk_evaluations(id) ON DELETE RESTRICT,
  mode text NOT NULL CHECK(mode IN ('PAPER','SHADOW')),
  status text NOT NULL CHECK(status IN ('PROPOSED','RISK_DENIED','SIMULATED_FILLED','SIMULATED_REJECTED','WOULD_HAVE_SUBMITTED','CANCELED','ERROR')),
  symbol text NOT NULL, instrument text NOT NULL, side text NOT NULL, quantity numeric(30,10) NOT NULL,
  price numeric(30,10), notional numeric(30,10), metadata jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(metadata)='object'),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE strategy_state_transitions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), strategy_instance_id uuid NOT NULL REFERENCES strategy_instances(id) ON DELETE RESTRICT,
  previous_state text NOT NULL, new_state text NOT NULL, state_version integer NOT NULL,
  trigger text NOT NULL, proposed_action_id text, risk_evaluation_id uuid REFERENCES risk_evaluations(id) ON DELETE RESTRICT,
  execution_record_id uuid REFERENCES nonlive_execution_records(id) ON DELETE RESTRICT,
  metadata jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(metadata)='object'), occurred_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(strategy_instance_id,state_version)
);
CREATE TABLE decision_journal_entries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT, mandate_version integer NOT NULL,
  strategy_instance_id uuid NOT NULL REFERENCES strategy_instances(id) ON DELETE RESTRICT, strategy_state text NOT NULL,
  source text NOT NULL CHECK(source IN ('STRATEGY','LIFECYCLE')), decision_type text NOT NULL,
  structured_rationale jsonb NOT NULL CHECK(jsonb_typeof(structured_rationale)='object'), proposed_action_id text,
  risk_evaluation_id uuid REFERENCES risk_evaluations(id) ON DELETE RESTRICT,
  execution_record_id uuid REFERENCES nonlive_execution_records(id) ON DELETE RESTRICT,
  resulting_state text, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX strategy_instances_user_idx ON strategy_instances(user_id,updated_at DESC);
CREATE INDEX strategy_transitions_history_idx ON strategy_state_transitions(strategy_instance_id,state_version);
CREATE INDEX strategy_decisions_history_idx ON decision_journal_entries(strategy_instance_id,created_at DESC);
CREATE INDEX strategy_executions_history_idx ON nonlive_execution_records(strategy_instance_id,created_at DESC);

CREATE FUNCTION reject_nonlive_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'non-live strategy history is immutable'; END $$;
CREATE TRIGGER strategy_transition_immutable BEFORE UPDATE OR DELETE ON strategy_state_transitions FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();
CREATE TRIGGER decision_journal_immutable BEFORE UPDATE OR DELETE ON decision_journal_entries FOR EACH ROW EXECUTE FUNCTION reject_nonlive_history_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS decision_journal_immutable ON decision_journal_entries;
DROP TRIGGER IF EXISTS strategy_transition_immutable ON strategy_state_transitions;
DROP FUNCTION IF EXISTS reject_nonlive_history_mutation;
DROP TABLE IF EXISTS decision_journal_entries;
DROP TABLE IF EXISTS strategy_state_transitions;
DROP TABLE IF EXISTS nonlive_execution_records;
DROP TABLE IF EXISTS paper_positions;
DROP TABLE IF EXISTS paper_portfolios;
DROP TABLE IF EXISTS strategy_evaluation_events;
DROP TABLE IF EXISTS strategy_instances;
