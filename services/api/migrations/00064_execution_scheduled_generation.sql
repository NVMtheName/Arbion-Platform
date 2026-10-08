-- +goose Up
-- Inert once-only fresh LIVE model generation. A durable claim is not a lease,
-- result, order, approval or permission to submit. No runtime is activated.
CREATE TABLE execution_generation_claims (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  capital_bucket_id uuid NOT NULL,
  mandate_approval_id uuid NOT NULL,
  mandate_id uuid NOT NULL,
  mandate_version integer NOT NULL CHECK(mandate_version>0),
  scheduled_for timestamptz NOT NULL CHECK(isfinite(scheduled_for) AND scheduled_for=date_trunc('second',scheduled_for)),
  ai_provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  ai_model_id text NOT NULL CHECK(ai_model_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'),
  facts jsonb NOT NULL CHECK(jsonb_typeof(facts)='object' AND octet_length(facts::text)<=8192),
  market jsonb NOT NULL CHECK(jsonb_typeof(market)='object' AND octet_length(market::text)<=8192),
  model_input text NOT NULL CHECK(octet_length(model_input)<=16384 AND jsonb_typeof(model_input::jsonb)='object'),
  input_digest text NOT NULL CHECK(input_digest=encode(sha256(convert_to(model_input,'UTF8')),'hex')),
  claimed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL CHECK(isfinite(expires_at) AND expires_at>claimed_at AND expires_at<=scheduled_for+interval '2 minutes'),
  UNIQUE(capital_bucket_id,mandate_id,mandate_version,scheduled_for),
  UNIQUE(owner_id,mandate_id,mandate_version,scheduled_for),
  UNIQUE(id,owner_id),
  FOREIGN KEY(financial_account_id,owner_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_approval_id,owner_id,financial_account_id,capital_bucket_id)
    REFERENCES execution_mandate_approvals(id,owner_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);
CREATE TABLE execution_generation_results (
  claim_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  outcome text NOT NULL CHECK(outcome IN ('PROPOSE','ABSTAIN','FAILED','UNKNOWN')),
  result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=24576),
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(claim_id,owner_id) REFERENCES execution_generation_claims(id,owner_id) ON DELETE RESTRICT,
  CHECK(result->>'Outcome' IS NOT DISTINCT FROM outcome)
);
CREATE TRIGGER execution_generation_claim_immutable BEFORE UPDATE OR DELETE ON execution_generation_claims FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_generation_claim_no_truncate BEFORE TRUNCATE ON execution_generation_claims FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_generation_result_immutable BEFORE UPDATE OR DELETE ON execution_generation_results FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_generation_result_no_truncate BEFORE TRUNCATE ON execution_generation_results FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();

-- The existing account/capital locks serialize claims with settlement and all
-- competing reservation writers. No initial capital or external holding is
-- invented; only the permanent pilot ledger supplies model budget facts.
-- +goose StatementBegin
CREATE FUNCTION check_execution_generation_funding(owner uuid, account uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE cash numeric; base numeric;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM execution_pilot_allocations WHERE owner_id=owner AND financial_account_id=account) THEN
    RAISE EXCEPTION 'generation pilot unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  SELECT cash_usd,base_quantity INTO cash,base FROM execution_pilot_balance(account);
  IF cash IS NULL OR base IS NULL OR cash<0 OR base<0
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=account)
    OR EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=account)
    OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=account AND released_at IS NULL)
    OR EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=account AND expires_at>clock_timestamp())
    OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=account AND execution_mode<>'PAPER' AND released_at IS NULL) THEN
    RAISE EXCEPTION 'generation capital unresolved' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
END $$;
-- +goose StatementEnd

-- Scope and current consent are independently enforced for direct SQL. Only
-- an exact enabled LIVE slot and immutable pilot projection may acquire a row.
-- Positive application commit acknowledgement, not row existence, admits the
-- one callback. There is intentionally no expiry deletion or reclaim path.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_generation_claim() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p execution_pilot_allocations%ROWTYPE; a execution_mandate_approvals%ROWTYPE;
  snapshot jsonb; until timestamptz; cash numeric; base numeric;
BEGIN
  until=execution_scheduled_proposal_deadline(NEW.mandate_approval_id,NEW.owner_id,NEW.capital_bucket_id,NEW.mandate_id,
    NEW.mandate_version,NEW.scheduled_for,NEW.ai_provider_connection_id,NEW.ai_model_id);
  PERFORM check_execution_generation_funding(NEW.owner_id,NEW.financial_account_id);
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=NEW.capital_bucket_id;
  SELECT * INTO STRICT a FROM execution_mandate_approvals WHERE id=NEW.mandate_approval_id;
  SELECT v.snapshot INTO STRICT snapshot FROM automation_mandate_versions v WHERE v.mandate_id=NEW.mandate_id AND v.version_number=NEW.mandate_version;
  SELECT cash_usd,base_quantity INTO cash,base FROM execution_pilot_balance(NEW.financial_account_id);
  NEW.claimed_at=clock_timestamp();
  IF p.owner_id<>NEW.owner_id OR p.financial_account_id<>NEW.financial_account_id OR p.provider_connection_id<>NEW.provider_connection_id
    OR a.mandate_id<>NEW.mandate_id OR a.mandate_version<>NEW.mandate_version OR a.provider_connection_id<>NEW.provider_connection_id
    OR NEW.expires_at IS DISTINCT FROM until OR NEW.expires_at<=NEW.claimed_at
    OR NEW.facts->'Pilot'-'Limits' IS DISTINCT FROM jsonb_build_object('OwnerID',p.owner_id,'AccountID',p.financial_account_id,
      'ConnectionID',p.provider_connection_id,'CapitalBucketID',p.capital_bucket_id,'ProductID',p.product_id,'InitialCashUSD',p.initial_cash_usd)
    OR NEW.facts->'Pilot'->'Limits'-'ExpiresAt' IS DISTINCT FROM jsonb_build_object('MaximumOrderUSD',p.maximum_order_usd)
    OR (NEW.facts->'Pilot'->'Limits'->>'ExpiresAt')::timestamptz IS DISTINCT FROM p.expires_at
    OR (NEW.facts->>'ExpiresAt')::timestamptz IS DISTINCT FROM until
    OR (NEW.facts->>'ObservedAt')::timestamptz IS NULL OR (NEW.facts->>'ObservedAt')::timestamptz>NEW.claimed_at
    OR (NEW.facts->>'ObservedAt')::timestamptz<NEW.claimed_at-interval '30 seconds'
    OR (NEW.facts->>'CashUSD')::numeric IS DISTINCT FROM cash OR (NEW.facts->>'AcquiredBase')::numeric IS DISTINCT FROM base
    OR NEW.facts->>'Objective' IS DISTINCT FROM snapshot->'strategy_parameters'->>'objective'
    OR NEW.facts->>'MaxProposalNotional' IS DISTINCT FROM snapshot->'strategy_parameters'->>'max_proposal_notional'
    OR NEW.facts->>'AIConnectionID' IS DISTINCT FROM NEW.ai_provider_connection_id::text
    OR NEW.facts->>'AIModelID' IS DISTINCT FROM NEW.ai_model_id
    OR NEW.facts->>'Profile' IS NULL OR NEW.facts->>'Profile' NOT IN ('fast','core','deep')
    OR NEW.model_input::jsonb->>'profile' IS DISTINCT FROM NEW.facts->>'Profile'
    OR NEW.model_input::jsonb->>'objective' IS DISTINCT FROM NEW.facts->>'Objective'
    OR NEW.model_input::jsonb->>'max_proposal_notional' IS DISTINCT FROM NEW.facts->>'MaxProposalNotional'
    OR (NEW.model_input::jsonb->>'available_cash_usd')::numeric IS DISTINCT FROM cash
    OR (NEW.model_input::jsonb->>'buying_power_usd')::numeric IS DISTINCT FROM cash
    OR (NEW.model_input::jsonb->>'observed_at')::timestamptz IS DISTINCT FROM (NEW.facts->>'ObservedAt')::timestamptz
    OR NEW.model_input::jsonb->'allowed_symbols' IS DISTINCT FROM jsonb_build_array(left(p.product_id,length(p.product_id)-4))
    OR EXISTS(SELECT 1 FROM execution_scheduled_proposals WHERE capital_bucket_id=NEW.capital_bucket_id AND mandate_id=NEW.mandate_id
      AND mandate_version=NEW.mandate_version AND scheduled_for=NEW.scheduled_for) THEN
    RAISE EXCEPTION 'generation claim does not match frozen controls' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_generation_claim_guard BEFORE INSERT ON execution_generation_claims FOR EACH ROW EXECUTE FUNCTION guard_execution_generation_claim();

-- Terminal result recording is recovery, not authority: expiry/stops must not
-- prevent preserving the one completed/failed outcome. PROPOSE scope is bound
-- here and deterministic sizing is validated by the narrow Go store boundary.
-- It must separately pass current scheduled intake and eventual send controls.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_generation_result() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c execution_generation_claims%ROWTYPE; proposal jsonb;
BEGIN
  SELECT * INTO STRICT c FROM execution_generation_claims WHERE id=NEW.claim_id AND owner_id=NEW.owner_id FOR UPDATE;
  NEW.recorded_at=clock_timestamp();
  IF NEW.result->>'Outcome' IS DISTINCT FROM NEW.outcome
    OR NEW.result-ARRAY['Outcome','Decision','Proposal','RequestDigest','ClientOrderID','Market']<>'{}'::jsonb THEN
    RAISE EXCEPTION 'generation terminal result mismatch' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  IF NEW.outcome='PROPOSE' THEN
    proposal=NEW.result->'Proposal';
    IF jsonb_typeof(proposal) IS DISTINCT FROM 'object' OR jsonb_typeof(NEW.result->'Market') IS DISTINCT FROM 'object'
      OR c.expires_at<=NEW.recorded_at
      OR (NEW.result->'Market'->>'CompletedAt')::timestamptz IS NULL
      OR (NEW.result->'Market'->>'CompletedAt')::timestamptz>NEW.recorded_at
      OR (NEW.result->'Market'->>'StartedAt')::timestamptz IS NULL
      OR (NEW.result->'Market'->>'StartedAt')::timestamptz<NEW.recorded_at-interval '30 seconds'
      OR (NEW.result->'Market'->>'ObservedAt')::timestamptz IS NULL
      OR (NEW.result->'Market'->>'ObservedAt')::timestamptz<NEW.recorded_at-interval '10 seconds'
      OR proposal->>'MandateApprovalID' IS DISTINCT FROM c.mandate_approval_id::text
      OR proposal->>'MandateID' IS DISTINCT FROM c.mandate_id::text OR (proposal->>'MandateVersion')::integer IS DISTINCT FROM c.mandate_version
      OR (proposal->>'ScheduledFor')::timestamptz IS DISTINCT FROM c.scheduled_for OR proposal->>'SourceMode' IS DISTINCT FROM 'LIVE'
      OR proposal->>'AIConnectionID' IS DISTINCT FROM c.ai_provider_connection_id::text OR proposal->>'AIModelID' IS DISTINCT FROM c.ai_model_id
      OR NEW.result->>'ClientOrderID' IS DISTINCT FROM execution_scheduled_client_id(c.capital_bucket_id,c.mandate_id,c.mandate_version,c.scheduled_for)::text
      OR NEW.result->>'RequestDigest' IS NULL OR NEW.result->>'RequestDigest' !~ '^[0-9a-f]{64}$' THEN
      RAISE EXCEPTION 'generation proposal scope mismatch' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
    END IF;
  ELSIF NEW.result->'Proposal' IS DISTINCT FROM 'null'::jsonb OR NEW.result->'Market' IS DISTINCT FROM 'null'::jsonb
    OR NEW.result->>'RequestDigest' IS DISTINCT FROM '' OR NEW.result->>'ClientOrderID' IS DISTINCT FROM '' THEN
    RAISE EXCEPTION 'non-proposal generation has order terms' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_generation_result_guard BEFORE INSERT ON execution_generation_results FOR EACH ROW EXECUTE FUNCTION guard_execution_generation_result();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'scheduled generation history is forward-only'; END $$;
-- +goose StatementEnd
