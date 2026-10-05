-- +goose Up
-- Inert per-order owner limits, never an activation switch or autonomous mandate.
-- Historical requests are untouched. Missing limits deny NEW admissions only;
-- reads, original-identity recovery, cancellation and settlement remain possible.

-- Pure validation is shared by the BEFORE order insert and saved-order checks.
-- Keep exact decimal strings (including trailing zeroes) and UTC microsecond
-- timestamps aligned with the Go request contract. Do not round either boundary.
-- +goose StatementBegin
CREATE FUNCTION execution_pilot_deadline(request jsonb) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE limits jsonb; field text; cap numeric; quantity numeric; price numeric;
  fee numeric; debit numeric; deadline timestamptz; raw_deadline text;
BEGIN
  limits=request->'PilotLimits';
  IF jsonb_typeof(limits) IS DISTINCT FROM 'object' THEN
    RAISE EXCEPTION 'execution pilot limits unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF NOT (limits ?& ARRAY['MaximumOrderUSD','ExpiresAt'])
    OR limits-ARRAY['MaximumOrderUSD','ExpiresAt']<>'{}'::jsonb
    OR jsonb_typeof(limits->'MaximumOrderUSD') IS DISTINCT FROM 'string'
    OR (limits->>'MaximumOrderUSD') !~ '^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$'
    OR jsonb_typeof(limits->'ExpiresAt') IS DISTINCT FROM 'string' THEN
    RAISE EXCEPTION 'execution pilot limits unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  raw_deadline=limits->>'ExpiresAt';
  IF raw_deadline !~ '^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\.[0-9]{1,6})?Z$'
    OR left(raw_deadline,4)='0000' THEN
    RAISE EXCEPTION 'execution pilot deadline invalid' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  deadline=raw_deadline::timestamptz;
  cap=(limits->>'MaximumOrderUSD')::numeric;
  FOREACH field IN ARRAY ARRAY['BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD'] LOOP
    IF jsonb_typeof(request->field) IS DISTINCT FROM 'string'
      OR (request->>field) !~ '^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$' THEN
      RAISE EXCEPTION 'execution pilot terms invalid' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
    END IF;
  END LOOP;
  quantity=(request->>'BaseSize')::numeric;
  price=(request->>'LimitPrice')::numeric;
  fee=(request->>'FeeAllowanceUSD')::numeric;
  debit=(request->>'MaximumDebitUSD')::numeric;
  IF cap<=0 OR quantity<=0 OR price<=0
    OR jsonb_typeof(request->'Side') IS DISTINCT FROM 'string'
    OR request->>'Side' NOT IN ('BUY','SELL')
    OR (request->>'Side'='BUY' AND (debit<quantity*price+fee OR debit>cap))
    -- SELL is a limit-price reference-notional bound plus fees. It is NOT a
    -- maximum realized proceeds claim; better execution prices can exceed it.
    OR (request->>'Side'='SELL' AND (debit<>0 OR quantity*price+fee>cap)) THEN
    RAISE EXCEPTION 'execution pilot order cap exceeded' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN deadline;
EXCEPTION WHEN invalid_datetime_format OR datetime_field_overflow OR numeric_value_out_of_range THEN
  RAISE EXCEPTION 'execution pilot limits invalid' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION check_execution_pilot_limits(target uuid) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE request jsonb; deadline timestamptz;
BEGIN
  SELECT o.request INTO request FROM execution_orders o WHERE o.id=target;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'execution pilot request unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  deadline=execution_pilot_deadline(request);
  IF deadline<=clock_timestamp() THEN
    RAISE EXCEPTION 'execution pilot expired' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN deadline;
END $$;
-- +goose StatementEnd

-- Preserve the original immutable identity guard; only the new exact-key member
-- and its independent cap/deadline checks are added. Do not backfill old orders.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_order() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'execution evidence is immutable' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM financial_accounts a JOIN provider_connections c ON c.id=a.provider_connection_id
    WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.owner_id AND a.provider_connection_id=NEW.provider_connection_id
      AND a.provider_name='coinbase' AND c.user_id=NEW.owner_id AND c.provider_category='financial' AND c.provider_name='coinbase') THEN
    RAISE EXCEPTION 'execution account binding mismatch' USING ERRCODE='23514';
  END IF;
  IF NOT (NEW.request ?& ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD','PilotLimits'])
    OR NEW.request-ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD','PilotLimits']<>'{}'::jsonb
    OR NEW.request->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.request->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.request->>'ConnectionID' IS DISTINCT FROM NEW.provider_connection_id::text
    OR NEW.request->>'CapitalBucketID' IS DISTINCT FROM NEW.capital_bucket_id::text
    OR NEW.request->>'ClientOrderID' IS DISTINCT FROM NEW.client_order_id::text THEN
    RAISE EXCEPTION 'execution request binding mismatch' USING ERRCODE='23514';
  END IF;
  IF execution_pilot_deadline(NEW.request)<=clock_timestamp() THEN
    RAISE EXCEPTION 'execution pilot expired' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- These guards deliberately do NOT change lock_execution_claim_controls, which
-- is also used by safe post-expiry lifecycle work. Named AFTER existing BEFORE
-- guards, they observe clock time after any existing account/control lock wait.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_pilot_admission() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE deadline timestamptz;
BEGIN
  deadline=check_execution_pilot_limits(NEW.order_id);
  IF TG_TABLE_NAME IN ('execution_owner_approvals','execution_authorizations','execution_dispatch_attempts')
    AND NEW.expires_at>deadline THEN
    RAISE EXCEPTION 'execution authorization exceeds pilot deadline' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_owner_pilot_guard BEFORE INSERT ON execution_owner_approvals
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_admission();
CREATE TRIGGER execution_provider_preflight_pilot_guard BEFORE INSERT ON execution_provider_preflights
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_admission();
CREATE TRIGGER execution_pilot_admission_guard BEFORE INSERT ON execution_dispatch_attempts
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_admission();
CREATE TRIGGER execution_pilot_authorization_guard BEFORE INSERT ON execution_authorizations
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_admission();
-- A transaction that waited after insertion cannot commit fresh send authority
-- beyond the pilot lifetime. This does not expire/release a pre-existing hold.
CREATE CONSTRAINT TRIGGER execution_pilot_commit_guard AFTER INSERT ON execution_dispatch_attempts
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_admission();

-- +goose Down
-- Refuse to remove a control that any immutable order already depends on.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_orders WHERE request ? 'PilotLimits') THEN
    RAISE EXCEPTION 'cannot remove immutable execution pilot limits';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER execution_pilot_commit_guard ON execution_dispatch_attempts;
DROP TRIGGER execution_pilot_authorization_guard ON execution_authorizations;
DROP TRIGGER execution_pilot_admission_guard ON execution_dispatch_attempts;
DROP TRIGGER execution_provider_preflight_pilot_guard ON execution_provider_preflights;
DROP TRIGGER execution_owner_pilot_guard ON execution_owner_approvals;
DROP FUNCTION guard_execution_pilot_admission();
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_order() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'execution evidence is immutable' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM financial_accounts a JOIN provider_connections c ON c.id=a.provider_connection_id
    WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.owner_id AND a.provider_connection_id=NEW.provider_connection_id
      AND a.provider_name='coinbase' AND c.user_id=NEW.owner_id AND c.provider_category='financial' AND c.provider_name='coinbase') THEN
    RAISE EXCEPTION 'execution account binding mismatch' USING ERRCODE='23514';
  END IF;
  IF NOT (NEW.request ?& ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD'])
    OR NEW.request-ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD']<>'{}'::jsonb
    OR NEW.request->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.request->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.request->>'ConnectionID' IS DISTINCT FROM NEW.provider_connection_id::text
    OR NEW.request->>'CapitalBucketID' IS DISTINCT FROM NEW.capital_bucket_id::text
    OR NEW.request->>'ClientOrderID' IS DISTINCT FROM NEW.client_order_id::text THEN
    RAISE EXCEPTION 'execution request binding mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
DROP FUNCTION check_execution_pilot_limits(uuid);
DROP FUNCTION execution_pilot_deadline(jsonb);
