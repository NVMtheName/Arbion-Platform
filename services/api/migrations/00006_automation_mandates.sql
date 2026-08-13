-- +goose Up
CREATE TABLE capital_buckets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  financial_account_id uuid NOT NULL,
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
  allocation_type text NOT NULL CHECK (allocation_type IN ('FIXED_AMOUNT','PERCENT_OF_AVAILABLE_CASH','PERCENT_OF_BUYING_POWER')),
  allocation_value numeric(30,10) NOT NULL CHECK (allocation_value > 0),
  currency char(3) NOT NULL DEFAULT 'USD',
  is_reserve boolean NOT NULL DEFAULT false,
  protected_amount numeric(30,10) NOT NULL DEFAULT 0 CHECK (protected_amount >= 0),
  allocation_limit numeric(30,10),
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','ARCHIVED')),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  CHECK (allocation_type='FIXED_AMOUNT' OR allocation_value <= 100),
  CHECK (allocation_limit IS NULL OR allocation_limit > 0)
);
CREATE INDEX capital_buckets_account_idx ON capital_buckets(user_id,financial_account_id) WHERE status='ACTIVE';

-- Evolve the one early automation concept in place. The migration fails closed if a
-- legacy connection has no discovered account rather than silently changing its scope.
ALTER TABLE automation_configs RENAME TO automation_mandates;
ALTER INDEX automation_configs_user_id_idx RENAME TO automation_mandates_user_id_idx;
DROP INDEX automation_configs_enabled_idx;
ALTER TABLE automation_mandates ADD COLUMN financial_account_id uuid;
UPDATE automation_mandates m SET financial_account_id=(SELECT f.id FROM financial_accounts f WHERE f.user_id=m.user_id AND f.provider_connection_id=m.financial_provider_connection_id ORDER BY f.created_at LIMIT 1);
DO $$ BEGIN IF EXISTS (SELECT 1 FROM automation_mandates WHERE financial_account_id IS NULL) THEN RAISE EXCEPTION 'legacy automation config has no financial account; connect/sync its account before migration'; END IF; END $$;
ALTER TABLE automation_mandates ADD COLUMN capital_bucket_id uuid;
INSERT INTO capital_buckets(user_id,financial_account_id,name,allocation_type,allocation_value,currency)
SELECT m.user_id,m.financial_account_id,'Migrated automation allocation','FIXED_AMOUNT',0.0000000001,f.base_currency FROM automation_mandates m JOIN financial_accounts f ON f.id=m.financial_account_id;
UPDATE automation_mandates m SET capital_bucket_id=(SELECT b.id FROM capital_buckets b WHERE b.user_id=m.user_id AND b.financial_account_id=m.financial_account_id AND b.name='Migrated automation allocation' ORDER BY b.created_at LIMIT 1);
ALTER TABLE automation_mandates DROP CONSTRAINT automation_configs_financial_provider_connection_id_fkey;
ALTER TABLE automation_mandates DROP COLUMN financial_provider_connection_id;
ALTER TABLE automation_mandates DROP COLUMN enabled;
ALTER TABLE automation_mandates DROP COLUMN last_run_at;
ALTER TABLE automation_mandates RENAME COLUMN mode TO execution_mode;
ALTER TABLE automation_mandates DROP CONSTRAINT automation_configs_mode_check;
ALTER TABLE automation_mandates ALTER COLUMN execution_mode SET DEFAULT 'PAPER';
UPDATE automation_mandates SET execution_mode=upper(execution_mode);
ALTER TABLE automation_mandates ADD CHECK (execution_mode IN ('BACKTEST','PAPER','SHADOW','LIVE'));
ALTER TABLE automation_mandates RENAME COLUMN strategy_config TO strategy_parameters;
ALTER TABLE automation_mandates RENAME COLUMN risk_config TO risk_parameters;
ALTER TABLE automation_mandates ALTER COLUMN strategy_identifier DROP NOT NULL;
ALTER TABLE automation_mandates ADD COLUMN automation_type text NOT NULL DEFAULT 'STRATEGY' CHECK (automation_type IN ('AI_AUTONOMOUS','STRATEGY','HYBRID'));
ALTER TABLE automation_mandates ADD COLUMN ai_model_id text;
ALTER TABLE automation_mandates ADD COLUMN autonomy_level text NOT NULL DEFAULT 'CONFIRM_EACH' CHECK (autonomy_level IN ('RESEARCH_ONLY','SUGGEST','CONFIRM_EACH','STRATEGY_AUTONOMOUS','FULL_AUTONOMOUS'));
ALTER TABLE automation_mandates ADD COLUMN status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','READY','PAUSED','DISABLED','ARCHIVED'));
ALTER TABLE automation_mandates ADD COLUMN current_version integer NOT NULL DEFAULT 0 CHECK (current_version >= 0);
ALTER TABLE automation_mandates ADD COLUMN allowed_universe jsonb NOT NULL DEFAULT '{"symbols":[],"universe_ids":[]}'::jsonb CHECK (jsonb_typeof(allowed_universe)='object');
ALTER TABLE automation_mandates ADD COLUMN prohibited_universe jsonb NOT NULL DEFAULT '{"symbols":[]}'::jsonb CHECK (jsonb_typeof(prohibited_universe)='object');
ALTER TABLE automation_mandates ADD COLUMN margin_allowed boolean NOT NULL DEFAULT false;
ALTER TABLE automation_mandates ADD COLUMN options_allowed boolean NOT NULL DEFAULT false;
ALTER TABLE automation_mandates ADD COLUMN schedule_conditions jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(schedule_conditions)='object');
ALTER TABLE automation_mandates ADD COLUMN capability_unverified boolean NOT NULL DEFAULT false;
ALTER TABLE automation_mandates ADD COLUMN effective_from timestamptz NOT NULL DEFAULT now();
ALTER TABLE automation_mandates ADD COLUMN effective_until timestamptz;
ALTER TABLE automation_mandates ALTER COLUMN financial_account_id SET NOT NULL;
ALTER TABLE automation_mandates ALTER COLUMN capital_bucket_id SET NOT NULL;
ALTER TABLE automation_mandates ADD FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT;
ALTER TABLE automation_mandates ADD FOREIGN KEY (capital_bucket_id) REFERENCES capital_buckets(id) ON DELETE RESTRICT;
ALTER TABLE automation_mandates ADD CHECK (effective_until IS NULL OR effective_until > effective_from);
CREATE INDEX automation_mandates_status_idx ON automation_mandates(user_id,status);

CREATE TABLE automation_mandate_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  version_number integer NOT NULL CHECK (version_number > 0), created_at timestamptz NOT NULL DEFAULT now(),
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  source text NOT NULL CHECK (source IN ('UI','CONVERSATION','ADMIN','SYSTEM')),
  snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot)='object'), change_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(change_summary)='object'),
  UNIQUE(mandate_id,version_number)
);
CREATE INDEX automation_mandate_versions_idx ON automation_mandate_versions(mandate_id,version_number DESC);

-- Existing placeholder configurations become immutable version 1 drafts.
INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
SELECT id,1,user_id,'SYSTEM',jsonb_build_object('financial_account_id',financial_account_id,'automation_type',automation_type,'strategy_identifier',strategy_identifier,'ai_provider_connection_id',ai_provider_connection_id,'ai_model_id',ai_model_id,'capital_bucket_id',capital_bucket_id,'autonomy_level',autonomy_level,'execution_mode',execution_mode,'status',status,'strategy_parameters',strategy_parameters,'risk_parameters',risk_parameters,'allowed_universe',allowed_universe,'prohibited_universe',prohibited_universe,'margin_allowed',margin_allowed,'options_allowed',options_allowed,'schedule_conditions',schedule_conditions,'effective_from',effective_from,'effective_until',effective_until,'execution_capable',false),'{"migration":"automation_configs"}'::jsonb FROM automation_mandates;
UPDATE automation_mandates SET current_version=1 WHERE current_version=0;

-- Immutable means UPDATE and DELETE are rejected even for application mistakes.
CREATE FUNCTION reject_mandate_version_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'automation mandate versions are immutable'; END $$;
CREATE TRIGGER automation_mandate_versions_immutable BEFORE UPDATE OR DELETE ON automation_mandate_versions FOR EACH ROW EXECUTE FUNCTION reject_mandate_version_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS automation_mandate_versions_immutable ON automation_mandate_versions;
DROP FUNCTION IF EXISTS reject_mandate_version_mutation();
DROP TABLE IF EXISTS automation_mandate_versions;
DROP TABLE IF EXISTS automation_mandates;
DROP TABLE IF EXISTS capital_buckets;
