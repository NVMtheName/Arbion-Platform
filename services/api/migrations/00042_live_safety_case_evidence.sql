-- +goose Up
-- This registry is an append-only design-evidence boundary. It deliberately has
-- no order, request, credential, command, approval, or executable payload.
CREATE TABLE live_safety_case_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  assessment_key text NOT NULL CHECK (assessment_key ~ '^[A-Za-z0-9_-]{16,128}$'),
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  provider_name text NOT NULL CHECK (provider_name ~ '^[a-z][a-z0-9_]{0,31}$'),
  strategy_instance_id uuid NOT NULL,
  mandate_id uuid NOT NULL REFERENCES automation_mandates(id) ON DELETE RESTRICT,
  mandate_version integer NOT NULL CHECK (mandate_version > 0),
  capital_bucket_id uuid NOT NULL,
  capital_reservation_id uuid NOT NULL,
  action_digest_sha256 char(64) NOT NULL CHECK (action_digest_sha256 ~ '^[0-9a-f]{64}$'),
  contract_version text NOT NULL CHECK (contract_version = 'live-execution-safety-v1'),
  assessment_state text NOT NULL CHECK (assessment_state IN ('UNAVAILABLE','BLOCKED_UNIMPLEMENTED')),
  structural_blocker boolean NOT NULL CHECK (structural_blocker),
  reason_codes jsonb NOT NULL CHECK (jsonb_typeof(reason_codes) = 'array'),
  evidence_manifest jsonb NOT NULL CHECK (jsonb_typeof(evidence_manifest) = 'object'),
  evidence_sha256 char(64) NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
  content_sha256 char(64) NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id,assessment_key),
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (strategy_instance_id,user_id) REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT,
  FOREIGN KEY (capital_bucket_id,user_id) REFERENCES capital_buckets(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (capital_reservation_id,user_id,strategy_instance_id)
    REFERENCES strategy_capital_reservations(id,user_id,strategy_instance_id) ON DELETE RESTRICT,
  CHECK (created_at >= observed_at)
);

CREATE INDEX live_safety_case_evidence_owner_time_idx
  ON live_safety_case_evidence(user_id,created_at DESC,id DESC);
CREATE INDEX live_safety_case_evidence_instance_time_idx
  ON live_safety_case_evidence(user_id,strategy_instance_id,created_at DESC,id DESC);

-- +goose StatementBegin
CREATE FUNCTION enforce_live_safety_case_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  reason text;
  item jsonb;
  item_id text;
  item_time timestamptz;
  seen_reasons text[] := ARRAY[]::text[];
  seen_items text[] := ARRAY[]::text[];
BEGIN
  IF NEW.observed_at > now() + interval '5 seconds' OR NEW.created_at > now() + interval '5 seconds' THEN
    RAISE EXCEPTION 'live safety evidence timestamps cannot be in the future';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM financial_accounts a
    JOIN provider_connections p
      ON p.id=a.provider_connection_id AND p.user_id=a.user_id
    WHERE a.id=NEW.financial_account_id
      AND a.user_id=NEW.user_id
      AND p.id=NEW.provider_connection_id
      AND p.provider_category='financial'
      AND p.provider_name=NEW.provider_name
      AND a.provider_name=NEW.provider_name
  ) THEN
    RAISE EXCEPTION 'live safety evidence financial identity mismatch';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM strategy_instances i
    JOIN automation_mandates m
      ON m.id=i.automation_mandate_id AND m.user_id=i.user_id
    WHERE i.id=NEW.strategy_instance_id
      AND i.user_id=NEW.user_id
      AND i.financial_account_id=NEW.financial_account_id
      AND i.capital_bucket_id=NEW.capital_bucket_id
      AND i.automation_mandate_id=NEW.mandate_id
      AND i.mandate_version=NEW.mandate_version
      AND i.execution_mode IN ('PAPER','SHADOW')
      AND m.financial_account_id=NEW.financial_account_id
      AND m.capital_bucket_id=NEW.capital_bucket_id
      AND m.execution_mode=i.execution_mode
  ) THEN
    RAISE EXCEPTION 'live safety evidence strategy identity mismatch';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM strategy_capital_reservations r
    WHERE r.id=NEW.capital_reservation_id
      AND r.user_id=NEW.user_id
      AND r.financial_account_id=NEW.financial_account_id
      AND r.capital_bucket_id=NEW.capital_bucket_id
      AND r.strategy_instance_id=NEW.strategy_instance_id
      AND r.execution_mode IN ('PAPER','SHADOW')
      AND r.released_at IS NULL
  ) THEN
    RAISE EXCEPTION 'live safety evidence capital identity mismatch';
  END IF;

  IF jsonb_typeof(NEW.reason_codes) <> 'array'
     OR jsonb_array_length(NEW.reason_codes) NOT BETWEEN 1 AND 32 THEN
    RAISE EXCEPTION 'live safety evidence requires one to thirty-two reasons';
  END IF;
  FOR reason IN SELECT jsonb_array_elements_text(NEW.reason_codes) LOOP
    IF reason IS NULL OR reason NOT IN (
      'INVALID_EVIDENCE','DUPLICATE_EVIDENCE','FUTURE_EVIDENCE','STALE_EVIDENCE',
      'BINDING_MISMATCH','ACTION_DIGEST_MISMATCH','NON_LIVE_MODE',
      'PROVIDER_CAPABILITY_UNAVAILABLE','TRANSFER_PERMISSION_PRESENT',
      'OWNER_AUTHORIZATION_UNAVAILABLE','DETERMINISTIC_RISK_UNAVAILABLE',
      'RECONCILIATION_UNAVAILABLE','KILL_SWITCH_UNAVAILABLE',
      'IDEMPOTENCY_UNAVAILABLE','LIFECYCLE_CONTRACT_UNAVAILABLE',
      'LIVE_RUNTIME_UNIMPLEMENTED','LIFECYCLE_EVIDENCE_INVALID',
      'POST_TRADE_RECONCILIATION_MISSING'
    ) THEN
      RAISE EXCEPTION 'unsupported live safety evidence reason';
    END IF;
    IF reason = ANY(seen_reasons) THEN
      RAISE EXCEPTION 'duplicate live safety evidence reason';
    END IF;
    seen_reasons := array_append(seen_reasons,reason);
  END LOOP;
  IF NEW.assessment_state='BLOCKED_UNIMPLEMENTED'
     AND NOT ('LIVE_RUNTIME_UNIMPLEMENTED'=ANY(seen_reasons)) THEN
    RAISE EXCEPTION 'blocked live safety evidence requires the runtime blocker';
  END IF;
  IF NEW.assessment_state='UNAVAILABLE'
     AND seen_reasons <@ ARRAY['LIVE_RUNTIME_UNIMPLEMENTED']::text[] THEN
    RAISE EXCEPTION 'unavailable live safety evidence requires an evidence reason';
  END IF;

  IF jsonb_typeof(NEW.evidence_manifest) <> 'object'
     OR NOT (NEW.evidence_manifest ? 'items')
     OR NEW.evidence_manifest - 'items' <> '{}'::jsonb
     OR jsonb_typeof(NEW.evidence_manifest->'items') <> 'array'
     OR jsonb_array_length(NEW.evidence_manifest->'items') NOT BETWEEN 1 AND 32 THEN
    RAISE EXCEPTION 'live safety evidence manifest shape is invalid';
  END IF;
  IF lower(NEW.evidence_manifest::text) ~ '"(api_key|secret|credential|password|private_key|access_token|refresh_token|authorization_header)"[[:space:]]*:' THEN
    RAISE EXCEPTION 'live safety evidence manifest contains a secret-like field';
  END IF;

  FOR item IN SELECT value FROM jsonb_array_elements(NEW.evidence_manifest->'items') LOOP
    IF jsonb_typeof(item) <> 'object'
       OR NOT (item ?& ARRAY['id','kind','digest_sha256','source','observed_at'])
       OR item - ARRAY['id','kind','digest_sha256','source','observed_at'] <> '{}'::jsonb
       OR jsonb_typeof(item->'id') <> 'string'
       OR jsonb_typeof(item->'kind') <> 'string'
       OR jsonb_typeof(item->'digest_sha256') <> 'string'
       OR jsonb_typeof(item->'source') <> 'string'
       OR jsonb_typeof(item->'observed_at') <> 'string'
       OR item->>'id' !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
       OR item->>'kind' !~ '^[A-Z][A-Z0-9_]{2,63}$'
       OR item->>'kind' ~ '(API_KEY|SECRET|CREDENTIAL|PASSWORD|PRIVATE_KEY|ACCESS_TOKEN|REFRESH_TOKEN|AUTHORIZATION_HEADER)'
       OR item->>'digest_sha256' !~ '^[0-9a-f]{64}$'
       OR item->>'source' NOT IN ('DATABASE','PROVIDER_VERIFIED','OWNER_MFA','DETERMINISTIC_CONTROL','DESIGN_CONTRACT') THEN
      RAISE EXCEPTION 'live safety evidence manifest item is invalid';
    END IF;
    item_id := item->>'id';
    IF item_id = ANY(seen_items) THEN
      RAISE EXCEPTION 'duplicate live safety evidence manifest item';
    END IF;
    seen_items := array_append(seen_items,item_id);
    BEGIN
      item_time := (item->>'observed_at')::timestamptz;
    EXCEPTION WHEN others THEN
      RAISE EXCEPTION 'live safety evidence manifest time is invalid';
    END;
    IF item_time > NEW.observed_at OR item_time > now() + interval '5 seconds' THEN
      RAISE EXCEPTION 'live safety evidence manifest time is inconsistent';
    END IF;
  END LOOP;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER live_safety_case_evidence_guard
  BEFORE INSERT ON live_safety_case_evidence
  FOR EACH ROW EXECUTE FUNCTION enforce_live_safety_case_evidence();

-- +goose StatementBegin
CREATE FUNCTION reject_live_safety_case_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'live safety case evidence is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER live_safety_case_evidence_immutable
  BEFORE UPDATE OR DELETE ON live_safety_case_evidence
  FOR EACH ROW EXECUTE FUNCTION reject_live_safety_case_evidence_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM live_safety_case_evidence) THEN
    RAISE EXCEPTION 'cannot remove immutable live safety case evidence';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS live_safety_case_evidence_immutable ON live_safety_case_evidence;
DROP FUNCTION IF EXISTS reject_live_safety_case_evidence_mutation();
DROP TRIGGER IF EXISTS live_safety_case_evidence_guard ON live_safety_case_evidence;
DROP FUNCTION IF EXISTS enforce_live_safety_case_evidence();
DROP TABLE IF EXISTS live_safety_case_evidence;
