-- +goose Up
-- Inert owner consent only. No route, scheduler, model or provider is activated.
CREATE TABLE execution_mandate_approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  capital_bucket_id uuid NOT NULL REFERENCES execution_pilot_allocations(capital_bucket_id) ON DELETE RESTRICT,
  mandate_id uuid NOT NULL,
  mandate_version integer NOT NULL CHECK(mandate_version>0),
  snapshot_digest text NOT NULL CHECK(snapshot_digest ~ '^[0-9a-f]{64}$'),
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  mfa_method text NOT NULL CHECK(mfa_method='totp'),
  mfa_verified_at timestamptz NOT NULL,
  mfa_enabled_at timestamptz NOT NULL,
  approved_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK(isfinite(expires_at) AND expires_at>approved_at AND expires_at<=approved_at+interval '24 hours'),
  UNIQUE(id,owner_id,financial_account_id,capital_bucket_id),
  UNIQUE(owner_id,mfa_verified_at),
  FOREIGN KEY(financial_account_id,owner_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);
CREATE TABLE execution_mandate_revocations (
  approval_id uuid PRIMARY KEY REFERENCES execution_mandate_approvals(id) ON DELETE RESTRICT,
  revoked_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER execution_mandate_approval_immutable BEFORE UPDATE OR DELETE ON execution_mandate_approvals FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_mandate_approval_no_truncate BEFORE TRUNCATE ON execution_mandate_approvals FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_mandate_revocation_immutable BEFORE UPDATE OR DELETE ON execution_mandate_revocations FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_mandate_revocation_no_truncate BEFORE TRUNCATE ON execution_mandate_revocations FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_mandate_versions_no_truncate BEFORE TRUNCATE ON automation_mandate_versions FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();

-- Compare every policy-bearing current field with the immutable version. Time
-- spellings can differ between Go and PostgreSQL; instants cannot differ.
-- +goose StatementBegin
CREATE FUNCTION execution_mandate_snapshot_matches(m automation_mandates, snapshot jsonb) RETURNS boolean LANGUAGE plpgsql STABLE AS $$
BEGIN
  RETURN COALESCE(jsonb_typeof(snapshot)='object'
    AND snapshot ?& ARRAY['effective_from','effective_until']
    AND snapshot-ARRAY['effective_from','effective_until']=jsonb_build_object(
      'financial_account_id',m.financial_account_id,'automation_type',m.automation_type,
      'strategy_identifier',m.strategy_identifier,'ai_provider_connection_id',m.ai_provider_connection_id,
      'ai_model_id',m.ai_model_id,'capital_bucket_id',m.capital_bucket_id,'autonomy_level',m.autonomy_level,
      'execution_mode',m.execution_mode,'status',m.status,'strategy_parameters',m.strategy_parameters,
      'risk_parameters',m.risk_parameters,'allowed_universe',m.allowed_universe,'prohibited_universe',m.prohibited_universe,
      'margin_allowed',m.margin_allowed,'options_allowed',m.options_allowed,'schedule_conditions',m.schedule_conditions,
      'capability_unverified',m.capability_unverified,'paper_options_simulation_attested',m.paper_options_simulation_attested,
      'execution_capable',false)
    AND jsonb_typeof(snapshot->'effective_from')='string'
    AND (snapshot->>'effective_from')::timestamptz=m.effective_from
    AND ((m.effective_until IS NULL AND snapshot->'effective_until'='null'::jsonb)
      OR (jsonb_typeof(snapshot->'effective_until')='string' AND (snapshot->>'effective_until')::timestamptz=m.effective_until)),false);
EXCEPTION WHEN invalid_datetime_format OR datetime_field_overflow THEN RETURN false;
END $$;
-- +goose StatementEnd

-- Same owner/account/provider/bucket and breaker ordering as claim. Founder
-- entitlement supplies both financial and automation access in this pilot.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_consent_controls(owner uuid, bucket uuid) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE p execution_pilot_allocations%ROWTYPE; a financial_accounts%ROWTYPE; c provider_connections%ROWTYPE;
  b capital_buckets%ROWTYPE; e user_entitlements%ROWTYPE; owner_status text; checked_at timestamptz;
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'consent requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=bucket AND owner_id=owner;
  SELECT status INTO STRICT owner_status FROM users WHERE id=owner FOR SHARE;
  SELECT * INTO e FROM user_entitlements WHERE user_id=owner AND entitlement_key='founder' FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(owner::text||':'||p.financial_account_id::text||':manual-order-reservations',0));
  SELECT * INTO STRICT a FROM financial_accounts WHERE id=p.financial_account_id AND user_id=owner FOR UPDATE;
  SELECT * INTO STRICT c FROM provider_connections WHERE id=p.provider_connection_id AND user_id=owner FOR SHARE;
  SELECT * INTO STRICT b FROM capital_buckets WHERE id=bucket AND user_id=owner AND financial_account_id=a.id FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(owner::text||':'||a.id::text||':strategy-capital-reservations',0));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('GLOBAL',NULL));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('USER',owner));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('ACCOUNT',a.id));
  checked_at=clock_timestamp();
  IF owner_status<>'active' OR e.id IS NULL OR e.status<>'active' OR e.starts_at>checked_at OR (e.expires_at IS NOT NULL AND e.expires_at<=checked_at)
    OR a.status<>'active' OR a.provider_name<>'coinbase' OR a.base_currency<>'USD'
    OR a.provider_connection_id<>p.provider_connection_id OR a.provider_account_id IS DISTINCT FROM p.provider_account_id
    OR c.status<>'active' OR c.provider_name<>'coinbase' OR c.provider_category<>'financial'
    OR c.encrypted_credential_payload IS NULL OR c.credential_reference IS NOT NULL
    OR (c.authorization_expires_at IS NOT NULL AND c.authorization_expires_at<=checked_at)
    OR b.status<>'ACTIVE' OR b.is_reserve OR b.currency<>'USD' OR b.allocation_type<>'FIXED_AMOUNT'
    OR LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))-b.protected_amount<p.initial_cash_usd::numeric
    OR p.expires_at<=checked_at OR EXISTS(SELECT 1 FROM risk_circuit_breakers WHERE state='OPEN' AND
      (scope='GLOBAL' OR (scope='USER' AND scope_id=owner) OR (scope='ACCOUNT' AND scope_id=a.id))) THEN
    RAISE EXCEPTION 'current consent controls unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN c.credential_generation;
END $$;
-- +goose StatementEnd

-- Private SQL current-policy check. Caller already owns financial controls.
-- Never acquire connectionguard advisory locks after the financial account.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_mandate_policy(owner uuid, bucket uuid, mandate uuid, version integer) RETURNS text LANGUAGE plpgsql AS $$
DECLARE p execution_pilot_allocations%ROWTYPE; m automation_mandates%ROWTYPE; ai provider_connections%ROWTYPE;
  snapshot jsonb; ai_id uuid; checked_at timestamptz;
BEGIN
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=bucket AND owner_id=owner;
  SELECT ai_provider_connection_id INTO ai_id FROM automation_mandates WHERE id=mandate AND user_id=owner;
  SELECT * INTO ai FROM provider_connections WHERE id=ai_id AND user_id=owner FOR SHARE;
  SELECT * INTO STRICT m FROM automation_mandates WHERE id=mandate AND user_id=owner FOR SHARE;
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('AUTOMATION',mandate));
  SELECT v.snapshot INTO STRICT snapshot FROM automation_mandate_versions v WHERE v.mandate_id=mandate AND v.version_number=version;
  checked_at=clock_timestamp();
  IF m.current_version<>version OR m.financial_account_id<>p.financial_account_id OR m.capital_bucket_id<>bucket
    OR m.status<>'READY' OR m.execution_mode<>'LIVE' OR m.automation_type<>'AI_AUTONOMOUS' OR m.autonomy_level<>'FULL_AUTONOMOUS'
    OR m.strategy_identifier IS NOT NULL OR m.margin_allowed OR m.options_allowed OR m.capability_unverified OR m.paper_options_simulation_attested
    OR m.effective_from>checked_at OR (m.effective_until IS NOT NULL AND m.effective_until<=checked_at)
    OR ai.id IS NULL OR m.ai_provider_connection_id IS DISTINCT FROM ai.id OR ai.status<>'active' OR ai.provider_category<>'ai' OR ai.provider_name<>'openai'
    OR (ai.authorization_expires_at IS NOT NULL AND ai.authorization_expires_at<=checked_at)
    OR m.ai_model_id IS NULL OR m.ai_model_id !~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
    OR NOT execution_mandate_snapshot_matches(m,snapshot)
    OR m.strategy_parameters->>'profile' IS DISTINCT FROM 'COINBASE_SPOT_PILOT_V1'
    OR EXISTS(SELECT 1 FROM risk_circuit_breakers WHERE state='OPEN' AND scope='AUTOMATION' AND scope_id=mandate) THEN
    RAISE EXCEPTION 'current immutable mandate unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN encode(sha256(convert_to(snapshot::text,'UTF8')),'hex');
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION guard_execution_mandate_approval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint; digest text; p execution_pilot_allocations%ROWTYPE; m automation_mandates%ROWTYPE; factor auth_totp_factors%ROWTYPE; ai_expiry timestamptz;
BEGIN
  generation=lock_execution_consent_controls(NEW.owner_id,NEW.capital_bucket_id);
  digest=lock_execution_mandate_policy(NEW.owner_id,NEW.capital_bucket_id,NEW.mandate_id,NEW.mandate_version);
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=NEW.capital_bucket_id;
  SELECT * INTO STRICT m FROM automation_mandates WHERE id=NEW.mandate_id;
  SELECT authorization_expires_at INTO ai_expiry FROM provider_connections WHERE id=m.ai_provider_connection_id;
  -- Exclusive factor lock serializes consumption across BOTH approval kinds.
  SELECT * INTO factor FROM auth_totp_factors WHERE user_id=NEW.owner_id FOR UPDATE;
  IF NEW.financial_account_id<>p.financial_account_id OR NEW.provider_connection_id<>p.provider_connection_id
    OR NEW.snapshot_digest<>digest OR NEW.credential_generation<>generation
    OR factor.enabled_at IS NULL OR factor.enabled_at IS DISTINCT FROM NEW.mfa_enabled_at OR factor.enabled_at>NEW.mfa_verified_at
    OR factor.updated_at IS DISTINCT FROM NEW.mfa_verified_at
    OR factor.last_used_step IS NULL OR factor.last_used_step NOT BETWEEN floor(extract(epoch FROM NEW.mfa_verified_at)/30)-1 AND floor(extract(epoch FROM NEW.mfa_verified_at)/30)+1
    OR NEW.mfa_verified_at<p.registered_at OR NEW.mfa_verified_at<m.updated_at OR NEW.mfa_verified_at>NEW.approved_at
    OR NEW.approved_at-NEW.mfa_verified_at>interval '10 seconds' OR NEW.approved_at>clock_timestamp()
    OR clock_timestamp()-NEW.mfa_verified_at>interval '10 seconds' OR NEW.expires_at<=clock_timestamp()
    OR NEW.expires_at>p.expires_at OR (m.effective_until IS NOT NULL AND NEW.expires_at>m.effective_until)
    OR (ai_expiry IS NOT NULL AND NEW.expires_at>ai_expiry)
    OR EXISTS(SELECT 1 FROM execution_owner_approvals WHERE owner_id=NEW.owner_id AND mfa_verified_at=NEW.mfa_verified_at) THEN
    RAISE EXCEPTION 'fresh exact mandate consent unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_mandate_approval_guard BEFORE INSERT ON execution_mandate_approvals FOR EACH ROW EXECUTE FUNCTION guard_execution_mandate_approval();

-- +goose StatementBegin
CREATE FUNCTION check_execution_mandate_consent(consent uuid, owner uuid, bucket uuid) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE a execution_mandate_approvals%ROWTYPE; generation bigint; digest text; enabled timestamptz; ai_expiry timestamptz;
BEGIN
  generation=lock_execution_consent_controls(owner,bucket);
  SELECT * INTO STRICT a FROM execution_mandate_approvals WHERE id=consent AND owner_id=owner AND capital_bucket_id=bucket;
  digest=lock_execution_mandate_policy(owner,bucket,a.mandate_id,a.mandate_version);
  SELECT * INTO STRICT a FROM execution_mandate_approvals WHERE id=consent AND owner_id=owner AND capital_bucket_id=bucket FOR SHARE;
  SELECT enabled_at INTO enabled FROM auth_totp_factors WHERE user_id=owner FOR SHARE;
  SELECT c.authorization_expires_at INTO ai_expiry FROM automation_mandates m JOIN provider_connections c ON c.id=m.ai_provider_connection_id WHERE m.id=a.mandate_id;
  IF a.credential_generation<>generation OR a.snapshot_digest<>digest OR enabled IS NULL OR enabled IS DISTINCT FROM a.mfa_enabled_at
    OR a.expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM execution_mandate_revocations WHERE approval_id=consent) THEN
    RAISE EXCEPTION 'mandate consent unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN LEAST(a.expires_at,ai_expiry);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION guard_execution_mandate_revocation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM execution_mandate_approvals WHERE id=NEW.approval_id FOR UPDATE;
  NEW.revoked_at=clock_timestamp();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_mandate_revoke_guard BEFORE INSERT ON execution_mandate_revocations FOR EACH ROW EXECUTE FUNCTION guard_execution_mandate_revocation();

-- Reciprocal cross-kind MFA consumption. Existing unique owner/timestamp
-- constraints continue to reject same-kind reuse and no history is rewritten.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_owner_approval() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint; factor auth_totp_factors%ROWTYPE; o execution_orders%ROWTYPE;
BEGIN
  generation=lock_execution_claim_controls(NEW.order_id);
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO factor FROM auth_totp_factors WHERE user_id=NEW.owner_id FOR UPDATE;
  IF o.request ? 'MandateApprovalID' OR NEW.owner_id<>o.owner_id OR NEW.request_digest<>o.request_digest OR NEW.credential_generation<>generation
    OR factor.enabled_at IS NULL OR factor.enabled_at IS DISTINCT FROM NEW.mfa_enabled_at OR factor.enabled_at>NEW.mfa_verified_at
    OR factor.updated_at IS DISTINCT FROM NEW.mfa_verified_at
    OR factor.last_used_step IS NULL OR factor.last_used_step NOT BETWEEN floor(extract(epoch FROM NEW.mfa_verified_at)/30)-1 AND floor(extract(epoch FROM NEW.mfa_verified_at)/30)+1
    OR NEW.mfa_verified_at<o.created_at OR NEW.mfa_verified_at>NEW.approved_at OR NEW.approved_at-NEW.mfa_verified_at>interval '10 seconds'
    OR NEW.approved_at>clock_timestamp() OR NEW.expires_at<=clock_timestamp()
    OR clock_timestamp()-NEW.mfa_verified_at>interval '10 seconds'
    OR EXISTS(SELECT 1 FROM execution_mandate_approvals WHERE owner_id=NEW.owner_id AND mfa_verified_at=NEW.mfa_verified_at)
    OR EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id)
    OR NOT EXISTS(SELECT 1 FROM provider_connections WHERE id=o.provider_connection_id AND encrypted_credential_payload IS NOT NULL AND credential_reference IS NULL) THEN
    RAISE EXCEPTION 'exact execution approval unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

ALTER TABLE execution_authorizations ALTER COLUMN approval_id DROP NOT NULL;
ALTER TABLE execution_authorizations ADD COLUMN mandate_approval_id uuid;
ALTER TABLE execution_authorizations ADD COLUMN capital_bucket_id uuid;
ALTER TABLE execution_authorizations ADD CONSTRAINT execution_authority_kind CHECK((approval_id IS NULL)<>(mandate_approval_id IS NULL));
ALTER TABLE execution_authorizations ADD CONSTRAINT execution_mandate_authority_bucket CHECK(mandate_approval_id IS NULL OR capital_bucket_id IS NOT NULL);
ALTER TABLE execution_authorizations ADD CONSTRAINT execution_mandate_authority_scope FOREIGN KEY(mandate_approval_id,owner_id,financial_account_id,capital_bucket_id)
  REFERENCES execution_mandate_approvals(id,owner_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_order() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE consent execution_mandate_approvals%ROWTYPE;
BEGIN
  IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'execution evidence is immutable' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM financial_accounts a JOIN provider_connections c ON c.id=a.provider_connection_id
    WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.owner_id AND a.provider_connection_id=NEW.provider_connection_id
      AND a.provider_name='coinbase' AND c.user_id=NEW.owner_id AND c.provider_category='financial' AND c.provider_name='coinbase') THEN
    RAISE EXCEPTION 'execution account binding mismatch' USING ERRCODE='23514';
  END IF;
  IF NOT (NEW.request ?& ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD','PilotLimits'])
    OR NEW.request-ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD','PilotLimits','MandateApprovalID']<>'{}'::jsonb
    OR NEW.request->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text OR NEW.request->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.request->>'ConnectionID' IS DISTINCT FROM NEW.provider_connection_id::text OR NEW.request->>'CapitalBucketID' IS DISTINCT FROM NEW.capital_bucket_id::text
    OR NEW.request->>'ClientOrderID' IS DISTINCT FROM NEW.client_order_id::text THEN
    RAISE EXCEPTION 'execution request binding mismatch' USING ERRCODE='23514';
  END IF;
  IF execution_pilot_deadline(NEW.request)<=clock_timestamp() THEN
    RAISE EXCEPTION 'execution pilot expired' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF NEW.request ? 'MandateApprovalID' THEN
    IF jsonb_typeof(NEW.request->'MandateApprovalID') IS DISTINCT FROM 'string' OR (NEW.request->>'MandateApprovalID') !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
      RAISE EXCEPTION 'invalid mandate consent identity' USING ERRCODE='23514';
    END IF;
    SELECT * INTO STRICT consent FROM execution_mandate_approvals WHERE id=(NEW.request->>'MandateApprovalID')::uuid;
    IF consent.owner_id<>NEW.owner_id OR consent.financial_account_id<>NEW.financial_account_id
      OR consent.provider_connection_id<>NEW.provider_connection_id OR consent.capital_bucket_id<>NEW.capital_bucket_id THEN
      RAISE EXCEPTION 'mandate consent scope mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Preserve every common exact authorization/attempt check; only the authority
-- branch differs. LIVE consent is distinct from legacy per-order confirmation.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_authorization_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; approval execution_owner_approvals%ROWTYPE; consent execution_mandate_approvals%ROWTYPE; until timestamptz;
BEGIN
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  IF NEW.mandate_approval_id IS NULL THEN
    SELECT * INTO STRICT approval FROM execution_owner_approvals WHERE id=NEW.approval_id FOR SHARE;
    IF o.request ? 'MandateApprovalID' OR NEW.request_digest<>approval.request_digest
      OR NEW.credential_generation<>approval.credential_generation OR NEW.expires_at>approval.expires_at
      OR EXISTS(SELECT 1 FROM execution_approval_revocations WHERE approval_id=NEW.approval_id)
      OR NEW.risk_evaluation->>'Mode' IS DISTINCT FROM 'MANUAL_PROPOSAL'
      OR NEW.risk_evaluation->>'ApprovalRequired' IS DISTINCT FROM 'true' THEN
      RAISE EXCEPTION 'exact owner authorization unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
    END IF;
  ELSE
    until=check_execution_mandate_consent(NEW.mandate_approval_id,NEW.owner_id,o.capital_bucket_id);
    SELECT * INTO STRICT consent FROM execution_mandate_approvals WHERE id=NEW.mandate_approval_id;
    IF NEW.capital_bucket_id IS DISTINCT FROM o.capital_bucket_id OR NEW.credential_generation<>consent.credential_generation
      OR NEW.expires_at>until OR o.request->>'MandateApprovalID' IS DISTINCT FROM consent.id::text
      OR NEW.risk_evaluation->>'Mode' IS DISTINCT FROM 'LIVE' OR NEW.risk_evaluation->>'ApprovalRequired' IS DISTINCT FROM 'false'
      OR NEW.risk_evaluation->>'Source' IS DISTINCT FROM 'AI'
      OR NEW.risk_evaluation->>'MandateID' IS DISTINCT FROM consent.mandate_id::text
      OR (NEW.risk_evaluation->>'MandateVersion')::integer IS DISTINCT FROM consent.mandate_version THEN
      RAISE EXCEPTION 'exact mandate authorization unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
    END IF;
  END IF;
  IF NEW.request_digest<>o.request_digest OR NEW.expires_at<=clock_timestamp()
    OR NEW.preflight->>'RequestDigest' IS DISTINCT FROM NEW.request_digest
    OR NEW.preflight->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.preflight->>'ConnectionID' IS DISTINCT FROM o.provider_connection_id::text
    OR NEW.preflight->>'ReconciliationID' IS DISTINCT FROM NEW.reconciliation_id::text
    OR (NEW.preflight->>'CredentialGeneration')::bigint IS DISTINCT FROM NEW.credential_generation
    OR jsonb_typeof(NEW.preflight->'ExpiresAt') IS DISTINCT FROM 'string' OR (NEW.preflight->>'ExpiresAt')::timestamptz<NEW.expires_at
    OR NEW.risk_evaluation->>'ID' IS DISTINCT FROM NEW.id::text OR NEW.risk_evaluation->>'Decision' IS DISTINCT FROM 'ALLOW'
    OR NEW.risk_evaluation->>'UserID' IS DISTINCT FROM NEW.owner_id::text OR NEW.risk_evaluation->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.risk_evaluation->>'PlatformExecutionAvailable' IS DISTINCT FROM 'false'
    OR (NEW.risk_evaluation->>'Timestamp')::timestamptz IS DISTINCT FROM NEW.checked_at
    OR NOT EXISTS(SELECT 1 FROM execution_dispatch_attempts a WHERE a.order_id=NEW.order_id AND a.owner_id=NEW.owner_id
      AND a.financial_account_id=NEW.financial_account_id AND a.authorization_id=NEW.id
      AND a.credential_generation=NEW.credential_generation AND a.expires_at=NEW.expires_at AND a.claimed_at>=NEW.checked_at) THEN
    RAISE EXCEPTION 'execution authorization does not bind exact claim' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- This migration introduces durable consent and an authority-union contract.
-- Removing it requires a separately reviewed forward migration, never erasure.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'execution mandate consent migration is forward-only'; END $$;
-- +goose StatementEnd
