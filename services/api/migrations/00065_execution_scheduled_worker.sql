-- +goose Up
-- Inert scheduler admission only. A run is immutable evidence, not a lease or
-- authority to submit. No runtime, route, timer, or activation is installed.
CREATE TABLE execution_scheduled_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  capital_bucket_id uuid NOT NULL,
  mandate_approval_id uuid NOT NULL,
  mandate_id uuid NOT NULL,
  mandate_version integer NOT NULL CHECK(mandate_version>0),
  scheduled_for timestamptz NOT NULL CHECK(isfinite(scheduled_for) AND scheduled_for=date_trunc('second',scheduled_for)),
  admitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL CHECK(isfinite(expires_at) AND expires_at>admitted_at AND expires_at<=scheduled_for+interval '2 minutes'),
  UNIQUE(owner_id,mandate_id,mandate_version,scheduled_for),
  UNIQUE(capital_bucket_id,mandate_id,mandate_version,scheduled_for),
  FOREIGN KEY(financial_account_id,owner_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_approval_id,owner_id,financial_account_id,capital_bucket_id)
    REFERENCES execution_mandate_approvals(id,owner_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT,
  FOREIGN KEY(mandate_id,mandate_version) REFERENCES automation_mandate_versions(mandate_id,version_number) ON DELETE RESTRICT
);
CREATE TRIGGER execution_scheduled_run_immutable BEFORE UPDATE OR DELETE ON execution_scheduled_runs
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_scheduled_run_no_truncate BEFORE TRUNCATE ON execution_scheduled_runs
  FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();

-- Only the DB clock chooses the most recent slot. The immutable consent-bound
-- version supplies its anchor and cadence, not a caller timestamp or interval.
-- A missed two-minute window is skipped; there is no catchup or lease reclaim.
-- +goose StatementBegin
CREATE FUNCTION execution_scheduled_worker_slot(owner uuid, account uuid, connection uuid, bucket uuid,
  consent_id uuid, mandate uuid, version integer) RETURNS timestamptz LANGUAGE plpgsql AS $$
DECLARE a execution_mandate_approvals%ROWTYPE; p execution_pilot_allocations%ROWTYPE;
  snapshot jsonb; schedule jsonb; notifications jsonb; effective timestamptz; anchor timestamptz;
  checked_at timestamptz; scheduled timestamptz; cadence integer;
BEGIN
  PERFORM lock_execution_scheduled_proposal_account(owner,account);
  PERFORM check_execution_mandate_consent(consent_id,owner,bucket);
  SELECT * INTO STRICT a FROM execution_mandate_approvals WHERE id=consent_id;
  SELECT * INTO STRICT p FROM execution_pilot_allocations WHERE capital_bucket_id=bucket;
  SELECT v.snapshot INTO STRICT snapshot FROM automation_mandate_versions v WHERE v.mandate_id=mandate AND v.version_number=version;
  schedule=snapshot->'schedule_conditions';
  notifications=COALESCE(schedule->'notifications','{}'::jsonb);
  IF a.owner_id<>owner OR a.financial_account_id<>account OR a.provider_connection_id<>connection OR a.capital_bucket_id<>bucket
    OR a.mandate_id<>mandate OR a.mandate_version<>version OR p.owner_id<>owner OR p.financial_account_id<>account
    OR p.provider_connection_id<>connection OR a.snapshot_digest<>encode(sha256(convert_to(snapshot::text,'UTF8')),'hex')
    OR jsonb_typeof(schedule) IS DISTINCT FROM 'object' OR schedule->'enabled' IS DISTINCT FROM 'true'::jsonb
    OR schedule->>'session' IS DISTINCT FROM 'CONTINUOUS' OR jsonb_typeof(schedule->'interval_minutes') IS DISTINCT FROM 'number'
    OR schedule->>'interval_minutes' !~ '^[0-9]{2,4}$'
    OR schedule-ARRAY['enabled','interval_minutes','session','notifications']<>'{}'::jsonb
    OR jsonb_typeof(notifications) IS DISTINCT FROM 'object'
    OR notifications-ARRAY['evaluation_completed','lifecycle_required','first_failure','reconciliation_review_required']<>'{}'::jsonb THEN
    RAISE EXCEPTION 'scheduled worker scope unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF EXISTS(SELECT 1 FROM jsonb_each(notifications) AS x WHERE jsonb_typeof(x.value)<>'boolean') THEN
    RAISE EXCEPTION 'invalid scheduled notification policy' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  cadence=(schedule->>'interval_minutes')::integer;
  effective=(snapshot->>'effective_from')::timestamptz;
  anchor=date_trunc('second',effective AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
  IF anchor<effective THEN anchor=anchor+interval '1 second'; END IF;
  checked_at=clock_timestamp();
  IF cadence<30 OR cadence>1440 OR effective IS NULL OR NOT isfinite(effective) THEN
    RAISE EXCEPTION 'invalid scheduled worker timing' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF checked_at<anchor THEN RETURN NULL; END IF;
  scheduled=anchor+make_interval(secs=>floor(extract(epoch FROM (checked_at-anchor))/(cadence*60))::double precision*cadence*60);
  IF checked_at>=scheduled+interval '2 minutes' THEN RETURN NULL; END IF;
  -- Existing current authorization independently checks the same canonical
  -- source, notification policy, model identity, and all applicable deadlines.
  PERFORM execution_scheduled_proposal_deadline(consent_id,owner,bucket,mandate,version,scheduled,
    (snapshot->>'ai_provider_connection_id')::uuid,snapshot->>'ai_model_id');
  RETURN scheduled;
END $$;
-- +goose StatementEnd

-- Direct writes retain the same lock order/current controls. Closed financial
-- history requires an exact durable closure; an absent hold is never closure.
-- An exact expired, never-authorized source may remain inert historical data
-- after a denied preview without permanently preventing later fresh slots.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_scheduled_run() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE scheduled timestamptz; snapshot jsonb; until timestamptz;
BEGIN
  scheduled=execution_scheduled_worker_slot(NEW.owner_id,NEW.financial_account_id,NEW.provider_connection_id,
    NEW.capital_bucket_id,NEW.mandate_approval_id,NEW.mandate_id,NEW.mandate_version);
  IF scheduled IS NULL OR NEW.scheduled_for IS DISTINCT FROM scheduled THEN
    RAISE EXCEPTION 'scheduled run is not due' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  PERFORM check_execution_generation_funding(NEW.owner_id,NEW.financial_account_id);
  SELECT v.snapshot INTO STRICT snapshot FROM automation_mandate_versions v WHERE v.mandate_id=NEW.mandate_id AND v.version_number=NEW.mandate_version;
  until=execution_scheduled_proposal_deadline(NEW.mandate_approval_id,NEW.owner_id,NEW.capital_bucket_id,NEW.mandate_id,
    NEW.mandate_version,scheduled,(snapshot->>'ai_provider_connection_id')::uuid,snapshot->>'ai_model_id');
  NEW.admitted_at=clock_timestamp();
  IF NEW.expires_at IS DISTINCT FROM until OR NEW.expires_at<=NEW.admitted_at
    OR EXISTS(SELECT 1 FROM execution_orders o
      LEFT JOIN execution_dispatch_attempts d ON d.order_id=o.id
      LEFT JOIN execution_capital_reservations r ON r.order_id=o.id
      LEFT JOIN execution_no_send_resolutions n ON n.order_id=o.id
      LEFT JOIN execution_account_settlements z ON z.order_id=o.id
      LEFT JOIN execution_order_terminals t ON t.order_id=o.id
      LEFT JOIN execution_scheduled_proposals sp ON sp.order_id=o.id
      WHERE o.financial_account_id=NEW.financial_account_id AND (
        -- Financial closures must bind the immutable original claim and its
        -- exact released reservation; neither expiry nor rejection qualifies.
        (d.order_id IS NOT NULL AND r.owner_id=o.owner_id AND r.financial_account_id=o.financial_account_id
          AND r.capital_bucket_id=o.capital_bucket_id AND r.reserved_at=d.claimed_at AND r.released_at IS NOT NULL
          AND NOT EXISTS(SELECT 1 FROM execution_account_holds h WHERE h.order_id=o.id)
          AND ((n.order_id IS NOT NULL AND z.order_id IS NULL AND t.order_id IS NULL
            AND n.owner_id=o.owner_id AND n.financial_account_id=o.financial_account_id AND n.capital_bucket_id=o.capital_bucket_id
            AND n.authorization_id=d.authorization_id AND n.request_digest=o.request_digest AND n.credential_generation=d.credential_generation
            AND n.claimed_at=d.claimed_at AND n.recorded_at=r.released_at
            AND NOT EXISTS(SELECT 1 FROM execution_broker_acknowledgements b WHERE b.order_id=o.id)
            AND NOT EXISTS(SELECT 1 FROM execution_submission_rejections x WHERE x.order_id=o.id)
            AND NOT EXISTS(SELECT 1 FROM execution_fills f WHERE f.order_id=o.id))
          OR (z.order_id IS NOT NULL AND n.order_id IS NULL AND t.order_id IS NOT NULL
            AND z.owner_id=o.owner_id AND z.financial_account_id=o.financial_account_id AND z.provider_connection_id=o.provider_connection_id
            AND z.capital_bucket_id=o.capital_bucket_id AND z.recorded_at=r.released_at AND z.provider_order_id=t.provider_order_id
            AND z.terminal_status=t.status AND z.fill_count=t.fill_count AND z.base_quantity=t.base_quantity
            AND z.gross_usd=t.gross_usd AND z.fee_usd=t.fee_usd)))
        OR (d.order_id IS NULL AND r.order_id IS NULL AND n.order_id IS NULL AND z.order_id IS NULL AND t.order_id IS NULL
          AND o.scheduled_proposal_required AND o.owner_id=NEW.owner_id AND o.provider_connection_id=NEW.provider_connection_id
          AND o.capital_bucket_id=NEW.capital_bucket_id AND o.request->>'MandateApprovalID'=NEW.mandate_approval_id::text
          AND sp.owner_id=NEW.owner_id AND sp.financial_account_id=NEW.financial_account_id AND sp.provider_connection_id=NEW.provider_connection_id
          AND sp.capital_bucket_id=NEW.capital_bucket_id AND sp.mandate_approval_id=NEW.mandate_approval_id
          AND sp.mandate_id=NEW.mandate_id AND sp.mandate_version=NEW.mandate_version AND sp.source_mode='LIVE'
          AND sp.request_digest=o.request_digest AND sp.client_order_id=o.client_order_id
          AND sp.client_order_id=execution_scheduled_client_id(sp.capital_bucket_id,sp.mandate_id,sp.mandate_version,sp.scheduled_for)
          AND sp.expires_at<=NEW.admitted_at
          AND NOT EXISTS(SELECT 1 FROM execution_authorizations a WHERE a.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_owner_approvals a WHERE a.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_account_holds h WHERE h.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_broker_acknowledgements b WHERE b.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_submission_rejections x WHERE x.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_fills f WHERE f.order_id=o.id)
          AND NOT EXISTS(SELECT 1 FROM execution_cancellation_attempts k WHERE k.order_id=o.id))
      ) IS NOT TRUE) THEN
    RAISE EXCEPTION 'scheduled run has unresolved account history' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_scheduled_run_guard BEFORE INSERT ON execution_scheduled_runs
  FOR EACH ROW EXECUTE FUNCTION guard_execution_scheduled_run();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'scheduled worker admission history is forward-only'; END $$;
-- +goose StatementEnd
