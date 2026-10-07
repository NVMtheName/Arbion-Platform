-- +goose Up
-- Inert fresh LIVE intake only. Existing non-live runs/proposals are not inputs
-- and no scheduler, model, provider or runtime composition is activated here.
-- The immutable server-set marker distinguishes old unbound history from new
-- autonomous orders, which can commit only with their source counterpart.
ALTER TABLE execution_orders ADD COLUMN scheduled_proposal_required boolean NOT NULL DEFAULT false;
CREATE TABLE execution_scheduled_proposals (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  capital_bucket_id uuid NOT NULL,
  mandate_approval_id uuid NOT NULL,
  mandate_id uuid NOT NULL,
  mandate_version integer NOT NULL CHECK(mandate_version>0),
  scheduled_for timestamptz NOT NULL CHECK(isfinite(scheduled_for) AND scheduled_for=date_trunc('second',scheduled_for)),
  source_mode text NOT NULL CHECK(source_mode='LIVE'),
  ai_provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  ai_model_id text NOT NULL CHECK(ai_model_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
  request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
  client_order_id uuid NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL CHECK(isfinite(expires_at) AND expires_at>created_at AND expires_at<=scheduled_for+interval '2 minutes'),
  UNIQUE(capital_bucket_id,mandate_id,mandate_version,scheduled_for),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_approval_id,owner_id,financial_account_id,capital_bucket_id)
    REFERENCES execution_mandate_approvals(id,owner_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);
CREATE TRIGGER execution_scheduled_proposal_immutable BEFORE UPDATE OR DELETE ON execution_scheduled_proposals FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_scheduled_proposal_no_truncate BEFORE TRUNCATE ON execution_scheduled_proposals FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();

-- A historical exact slot replay must remain readable after expiry, including
-- after waiting for a concurrent winner. Acquire the standard lock prefix, but
-- do not renew or validate historical authority before that second saved read.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_scheduled_proposal_account(owner uuid, account uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'scheduled intake requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  PERFORM 1 FROM users WHERE id=owner FOR SHARE;
  PERFORM 1 FROM user_entitlements WHERE user_id=owner AND entitlement_key='founder' FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(owner::text||':'||account::text||':manual-order-reservations',0));
  PERFORM 1 FROM financial_accounts WHERE id=account AND user_id=owner FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'scheduled proposal account mismatch' USING ERRCODE='23514',CONSTRAINT='execution_current_controls'; END IF;
END $$;
-- +goose StatementEnd

-- Custom UUIDv8 using the identical application domain and UTC second format.
-- Renewable consent, proposal terms and caller-generated IDs are not slot keys.
-- +goose StatementBegin
CREATE FUNCTION execution_scheduled_client_id(bucket uuid, mandate uuid, version integer, scheduled timestamptz) RETURNS uuid LANGUAGE sql IMMUTABLE STRICT AS $$
  SELECT (substr(h,1,8)||'-'||substr(h,9,4)||'-8'||substr(h,14,3)||'-a'||substr(h,18,3)||'-'||substr(h,21,12))::uuid
  FROM (SELECT encode(sha256(convert_to('arbion-live-scheduled-proposal-v1|'||bucket::text||'|'||mandate::text||'|'||version::text||'|'||
    to_char(scheduled AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),'UTF8')),'hex') AS h) x
$$;
-- +goose StatementEnd

-- Source intake and every later authorization independently require the same
-- current consent and immutable enabled CONTINUOUS schedule. Slot alignment is
-- anchored to effective_from rounded UP to the next UTC second, never to now.
-- +goose StatementBegin
CREATE FUNCTION execution_scheduled_proposal_deadline(consent_id uuid, owner uuid, bucket uuid, mandate uuid, version integer,
  scheduled timestamptz, ai_connection uuid, model text) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE consent execution_mandate_approvals%ROWTYPE; m automation_mandates%ROWTYPE; p execution_pilot_allocations%ROWTYPE;
  consent_until timestamptz; until timestamptz; anchor timestamptz; checked_at timestamptz; cadence integer; schedule jsonb; notifications jsonb;
BEGIN
  consent_until=check_execution_mandate_consent(consent_id,owner,bucket);
  SELECT * INTO STRICT consent FROM execution_mandate_approvals WHERE id=consent_id;
  SELECT * INTO STRICT m FROM automation_mandates WHERE id=mandate;
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=bucket;
  schedule=m.schedule_conditions;
  notifications=COALESCE(schedule->'notifications','{}'::jsonb);
  IF consent.mandate_id<>mandate OR consent.mandate_version<>version OR m.current_version<>version
    OR m.user_id<>owner OR m.capital_bucket_id<>bucket OR m.financial_account_id<>p.financial_account_id
    OR m.execution_mode<>'LIVE' OR m.ai_provider_connection_id IS DISTINCT FROM ai_connection OR m.ai_model_id IS DISTINCT FROM model
    OR jsonb_typeof(schedule) IS DISTINCT FROM 'object' OR schedule->'enabled' IS DISTINCT FROM 'true'::jsonb
    OR schedule->>'session' IS DISTINCT FROM 'CONTINUOUS' OR jsonb_typeof(schedule->'interval_minutes') IS DISTINCT FROM 'number'
    OR schedule->>'interval_minutes' !~ '^[0-9]{2,4}$'
    OR schedule-ARRAY['enabled','interval_minutes','session','notifications']<>'{}'::jsonb
    OR jsonb_typeof(notifications) IS DISTINCT FROM 'object'
    OR notifications-ARRAY['evaluation_completed','lifecycle_required','first_failure','reconciliation_review_required']<>'{}'::jsonb THEN
    RAISE EXCEPTION 'fresh LIVE scheduled source unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF EXISTS(SELECT 1 FROM jsonb_each(notifications) AS x WHERE jsonb_typeof(x.value)<>'boolean') THEN
    RAISE EXCEPTION 'invalid scheduled notification policy' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  cadence=(schedule->>'interval_minutes')::integer;
  anchor=date_trunc('second',m.effective_from AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
  IF anchor<m.effective_from THEN anchor=anchor+interval '1 second'; END IF;
  checked_at=clock_timestamp();
  IF cadence<30 OR cadence>1440 OR NOT isfinite(scheduled) OR scheduled<>date_trunc('second',scheduled)
    OR scheduled<anchor OR scheduled>checked_at OR mod(extract(epoch FROM (scheduled-anchor)),cadence*60)<>0 THEN
    RAISE EXCEPTION 'scheduled source is not a current canonical slot' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  until=LEAST(scheduled+interval '2 minutes',scheduled+make_interval(mins=>cadence),consent_until,m.effective_until,p.expires_at);
  IF until<=checked_at THEN
    RAISE EXCEPTION 'scheduled source expired' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN until;
END $$;
-- +goose StatementEnd

-- Enforce normal financial lock order even for direct SQL that inserts an
-- autonomous order before its source. Existing owner/manual preparation is
-- unchanged. This trigger sorts before the existing order/pilot insert guards.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_scheduled_order() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.scheduled_proposal_required=NEW.request ? 'MandateApprovalID';
  IF NEW.scheduled_proposal_required THEN
    PERFORM lock_execution_consent_controls(NEW.owner_id,NEW.capital_bucket_id);
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_00_scheduled_order BEFORE INSERT ON execution_orders FOR EACH ROW EXECUTE FUNCTION lock_execution_scheduled_order();

-- A source can bind only the exact order created in its OWN transaction, never
-- attach a fresh LIVE label to a historical unbound or non-live proposal.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_scheduled_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; consent execution_mandate_approvals%ROWTYPE; until timestamptz;
BEGIN
  until=execution_scheduled_proposal_deadline(NEW.mandate_approval_id,NEW.owner_id,NEW.capital_bucket_id,NEW.mandate_id,
    NEW.mandate_version,NEW.scheduled_for,NEW.ai_provider_connection_id,NEW.ai_model_id);
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO STRICT consent FROM execution_mandate_approvals WHERE id=NEW.mandate_approval_id;
  NEW.created_at=clock_timestamp();
  IF NOT o.scheduled_proposal_required OR o.owner_id<>NEW.owner_id OR o.financial_account_id<>NEW.financial_account_id
    OR o.provider_connection_id<>NEW.provider_connection_id OR o.capital_bucket_id<>NEW.capital_bucket_id
    OR o.client_order_id<>NEW.client_order_id OR o.request_digest<>NEW.request_digest
    OR o.request->>'MandateApprovalID' IS DISTINCT FROM NEW.mandate_approval_id::text
    OR consent.provider_connection_id<>NEW.provider_connection_id OR consent.mandate_id<>NEW.mandate_id OR consent.mandate_version<>NEW.mandate_version
    OR NEW.client_order_id<>execution_scheduled_client_id(NEW.capital_bucket_id,NEW.mandate_id,NEW.mandate_version,NEW.scheduled_for)
    OR NEW.source_mode<>'LIVE' OR NEW.expires_at IS DISTINCT FROM until OR NEW.expires_at<=NEW.created_at
    OR o.created_at<NEW.scheduled_for OR EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id) THEN
    RAISE EXCEPTION 'scheduled source does not bind a fresh exact order' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_scheduled_proposal_guard BEFORE INSERT ON execution_scheduled_proposals FOR EACH ROW EXECUTE FUNCTION guard_execution_scheduled_proposal();

-- No row locks on a new source precede the existing account controls. Immutable
-- source identity and deadline are rechecked at claim and final synchronous send.
-- +goose StatementBegin
CREATE FUNCTION check_execution_scheduled_proposal(target uuid) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE s execution_scheduled_proposals%ROWTYPE; o execution_orders%ROWTYPE; until timestamptz;
BEGIN
  SELECT * INTO STRICT o FROM execution_orders WHERE id=target;
  SELECT * INTO s FROM execution_scheduled_proposals WHERE order_id=target;
  IF s.order_id IS NULL THEN
    RAISE EXCEPTION 'autonomous scheduled source required' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  until=execution_scheduled_proposal_deadline(s.mandate_approval_id,s.owner_id,s.capital_bucket_id,s.mandate_id,
    s.mandate_version,s.scheduled_for,s.ai_provider_connection_id,s.ai_model_id);
  IF NOT o.scheduled_proposal_required OR s.owner_id<>o.owner_id OR s.financial_account_id<>o.financial_account_id OR s.provider_connection_id<>o.provider_connection_id
    OR s.capital_bucket_id<>o.capital_bucket_id OR s.request_digest<>o.request_digest OR s.client_order_id<>o.client_order_id
    OR o.request->>'MandateApprovalID' IS DISTINCT FROM s.mandate_approval_id::text OR s.source_mode<>'LIVE'
    OR s.client_order_id<>execution_scheduled_client_id(s.capital_bucket_id,s.mandate_id,s.mandate_version,s.scheduled_for)
    OR s.expires_at<=clock_timestamp() THEN
    RAISE EXCEPTION 'autonomous scheduled source changed or expired' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN LEAST(s.expires_at,until);
END $$;
-- +goose StatementEnd

-- Only INSERTs after this migration require the new counterpart. Old execution
-- history remains intact and readable, but cannot gain a new autonomous claim.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_scheduled_order_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.scheduled_proposal_required THEN PERFORM check_execution_scheduled_proposal(NEW.id); END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER execution_scheduled_order_commit AFTER INSERT ON execution_orders DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION guard_execution_scheduled_order_commit();

-- Preserve the existing authorization commit guard and add only the mandatory
-- autonomous source/deadline condition. Manual owner authority is unchanged.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_scheduled_authorization() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE until timestamptz;
BEGIN
  IF NEW.mandate_approval_id IS NOT NULL THEN
    until=check_execution_scheduled_proposal(NEW.order_id);
    IF NEW.expires_at>until THEN
      RAISE EXCEPTION 'authorization outlives scheduled source' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_scheduled_authorization_guard BEFORE INSERT ON execution_authorizations FOR EACH ROW EXECUTE FUNCTION guard_execution_scheduled_authorization();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'scheduled execution source history is forward-only'; END $$;
-- +goose StatementEnd
