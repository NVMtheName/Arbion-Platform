-- +goose Up
-- Preserve original reservations and release only against an exact immutable
-- account-settlement receipt. This migration does not activate any sender.
ALTER TABLE execution_capital_reservations ADD COLUMN released_at timestamptz;
ALTER TABLE execution_capital_reservations DROP CONSTRAINT execution_capital_reservations_financial_account_id_key;
CREATE UNIQUE INDEX execution_capital_active_account ON execution_capital_reservations(financial_account_id) WHERE released_at IS NULL;

CREATE TABLE execution_account_settlements (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL,
  capital_bucket_id uuid NOT NULL,
  provider_order_id uuid NOT NULL,
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  portfolio_id uuid NOT NULL,
  terminal_status text NOT NULL CHECK(terminal_status IN ('FILLED','CANCELLED','EXPIRED','REJECTED')),
  fill_count bigint NOT NULL CHECK(fill_count BETWEEN 0 AND 1000),
  base_quantity numeric NOT NULL CHECK(base_quantity>=0 AND base_quantity<1e18 AND scale(base_quantity)<=18),
  gross_usd numeric NOT NULL CHECK(gross_usd>=0 AND gross_usd<1e36 AND scale(gross_usd)<=36),
  fee_usd numeric NOT NULL CHECK(fee_usd>=0 AND fee_usd<1e18 AND scale(fee_usd)<=18),
  opening_cash_usd numeric NOT NULL CHECK(opening_cash_usd>=0 AND opening_cash_usd<1e18 AND scale(opening_cash_usd)<=18),
  opening_base numeric NOT NULL CHECK(opening_base>=0 AND opening_base<1e18 AND scale(opening_base)<=18),
  closing_cash_usd numeric NOT NULL CHECK(closing_cash_usd>=0 AND closing_cash_usd<1e18 AND scale(closing_cash_usd)<=18),
  closing_base numeric NOT NULL CHECK(closing_base>=0 AND closing_base<1e18 AND scale(closing_base)<=18),
  started_at timestamptz NOT NULL,
  observed_at timestamptz NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=2097152),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id,provider_order_id) REFERENCES execution_broker_acknowledgements(order_id,owner_id,provider_order_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(provider_connection_id) REFERENCES provider_connections(id) ON DELETE RESTRICT
);
CREATE TRIGGER execution_account_settlement_immutable BEFORE UPDATE OR DELETE ON execution_account_settlements
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Same global order as claim. Safe accounting is deliberately independent of
-- expired owner approvals, disabled allocation policies and circuit breakers.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_settlement_controls(target uuid, portfolio uuid) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; a financial_accounts%ROWTYPE; c provider_connections%ROWTYPE;
  e user_entitlements%ROWTYPE; owner_status text; checked_at timestamptz;
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'settlement requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  SELECT * INTO STRICT o FROM execution_orders WHERE id=target;
  SELECT status INTO STRICT owner_status FROM users WHERE id=o.owner_id FOR SHARE;
  SELECT * INTO e FROM user_entitlements WHERE user_id=o.owner_id AND entitlement_key='founder' FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text||':'||o.financial_account_id::text||':manual-order-reservations',0));
  SELECT * INTO STRICT a FROM financial_accounts WHERE id=o.financial_account_id AND user_id=o.owner_id FOR UPDATE;
  SELECT * INTO STRICT c FROM provider_connections WHERE id=o.provider_connection_id AND user_id=o.owner_id FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text||':'||o.financial_account_id::text||':strategy-capital-reservations',0));
  PERFORM lock_execution_capital_fence(o.financial_account_id);
  checked_at=clock_timestamp();
  IF owner_status<>'active' OR e.id IS NULL OR e.status<>'active' OR e.starts_at>checked_at
    OR (e.expires_at IS NOT NULL AND e.expires_at<=checked_at)
    OR a.status<>'active' OR a.provider_name<>'coinbase' OR a.base_currency<>'USD'
    OR a.provider_connection_id<>o.provider_connection_id OR a.provider_account_id IS DISTINCT FROM 'portfolio:'||portfolio::text
    OR c.status<>'active' OR c.provider_category<>'financial' OR c.provider_name<>'coinbase'
    OR c.encrypted_credential_payload IS NULL OR c.credential_reference IS NOT NULL
    OR (c.authorization_expires_at IS NOT NULL AND c.authorization_expires_at<=checked_at) THEN
    RAISE EXCEPTION 'current settlement access unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN c.credential_generation;
END $$;
-- +goose StatementEnd

-- Compare economics and provenance exactly; only polling time may change.
-- Decimal spellings are equivalent, but no precision is rounded or inferred.
-- +goose StatementBegin
CREATE FUNCTION execution_settlement_fill_matches(candidate jsonb, saved jsonb, observed jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
  SELECT COALESCE(
    jsonb_typeof(candidate)='object'
    AND candidate ?& ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','TradeID','BaseQuantity','PriceUSD','GrossUSD','FeeUSD','TradedAt','ObservedAt','ProviderEvidence']
    AND candidate-ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','TradeID','BaseQuantity','PriceUSD','GrossUSD','FeeUSD','TradedAt','ObservedAt','ProviderEvidence']='{}'::jsonb
    AND candidate-ARRAY['BaseQuantity','PriceUSD','GrossUSD','FeeUSD','ObservedAt','ProviderEvidence'] = saved-ARRAY['BaseQuantity','PriceUSD','GrossUSD','FeeUSD','ObservedAt','ProviderEvidence']
    AND (candidate->>'BaseQuantity')::numeric=(saved->>'BaseQuantity')::numeric
    AND (candidate->>'PriceUSD')::numeric=(saved->>'PriceUSD')::numeric
    AND (candidate->>'GrossUSD')::numeric=(saved->>'GrossUSD')::numeric
    AND (candidate->>'FeeUSD')::numeric=(saved->>'FeeUSD')::numeric
    AND candidate->'ObservedAt'=observed
    AND jsonb_typeof(candidate->'ProviderEvidence')='object'
    AND candidate->'ProviderEvidence'-'Size'=saved->'ProviderEvidence'-'Size'
    AND (candidate->'ProviderEvidence'->>'Size')::numeric=(saved->'ProviderEvidence'->>'Size')::numeric,
    false)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION guard_execution_account_settlement() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; t execution_order_terminals%ROWTYPE;
  p execution_provider_preflights%ROWTYPE; hold execution_capital_reservations%ROWTYPE;
  b jsonb; item jsonb; saved jsonb; k text; generation bigint; n bigint; q numeric; g numeric; f numeric;
BEGIN
  generation=lock_execution_settlement_controls(NEW.order_id,NEW.portfolio_id);
  NEW.recorded_at=clock_timestamp();
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO STRICT t FROM execution_order_terminals WHERE order_id=o.id;
  SELECT * INTO STRICT hold FROM execution_capital_reservations WHERE order_id=o.id;
  SELECT pf.* INTO STRICT p FROM execution_dispatch_attempts a
    JOIN execution_authorizations z ON z.id=a.authorization_id AND z.order_id=a.order_id AND z.owner_id=a.owner_id
    JOIN execution_provider_preflights pf ON pf.id::text=z.preflight->>'EvidenceID' AND pf.order_id=a.order_id
      AND pf.owner_id=a.owner_id AND pf.financial_account_id=a.financial_account_id
      AND pf.credential_generation=a.credential_generation AND pf.request_digest=z.request_digest
    WHERE a.order_id=o.id AND a.owner_id=o.owner_id;
  SELECT count(*),COALESCE(sum(base_quantity),0),COALESCE(sum(gross_usd),0),COALESCE(sum(fee_usd),0)
    INTO n,q,g,f FROM execution_fills WHERE order_id=o.id;
  IF NEW.owner_id<>o.owner_id OR NEW.financial_account_id<>o.financial_account_id
    OR NEW.provider_connection_id<>o.provider_connection_id OR NEW.capital_bucket_id<>o.capital_bucket_id
    OR NEW.credential_generation<>generation OR p.provider_connection_id<>o.provider_connection_id
    OR p.request_digest<>o.request_digest OR p.portfolio_id<>NEW.portfolio_id
    OR hold.released_at IS NOT NULL OR hold.owner_id<>o.owner_id OR hold.financial_account_id<>o.financial_account_id
    OR NEW.provider_order_id<>t.provider_order_id OR NEW.terminal_status<>t.status
    OR NEW.fill_count<>t.fill_count OR NEW.base_quantity<>t.base_quantity OR NEW.gross_usd<>t.gross_usd OR NEW.fee_usd<>t.fee_usd
    OR NEW.fill_count<>n OR NEW.base_quantity<>q OR NEW.gross_usd<>g OR NEW.fee_usd<>f
    OR NEW.started_at<t.observed_at OR NEW.observed_at<NEW.started_at
    OR NEW.observed_at-NEW.started_at>interval '15 seconds' OR NEW.observed_at>NEW.recorded_at
    OR NEW.recorded_at-NEW.observed_at>interval '30 seconds'
    OR NEW.opening_cash_usd IS DISTINCT FROM (p.evidence->>'CashUSD')::numeric
    OR NEW.opening_base IS DISTINCT FROM (p.evidence->>'TotalBase')::numeric
    OR NEW.opening_cash_usd IS DISTINCT FROM (p.evidence->>'AvailableCashUSD')::numeric
    OR NEW.opening_base IS DISTINCT FROM (p.evidence->>'AvailableBase')::numeric
    OR NEW.closing_cash_usd IS DISTINCT FROM (CASE WHEN o.request->>'Side'='BUY' THEN NEW.opening_cash_usd-g-f ELSE NEW.opening_cash_usd+g-f END)
    OR NEW.closing_base IS DISTINCT FROM (CASE WHEN o.request->>'Side'='BUY' THEN NEW.opening_base+q ELSE NEW.opening_base-q END)
    OR EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=o.financial_account_id)
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=o.financial_account_id)
    OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=o.financial_account_id AND released_at IS NULL AND order_id<>o.id)
    OR EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=o.financial_account_id AND expires_at>NEW.recorded_at)
    OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=o.financial_account_id AND execution_mode<>'PAPER' AND released_at IS NULL) THEN
    RAISE EXCEPTION 'exact account settlement unavailable' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  IF NOT (NEW.evidence ?& ARRAY['PortfolioID','Observation','CashUSD','AvailableCashUSD','TotalBase','AvailableBase','Complete','NoOpenOrders','StartedAt','CompletedAt'])
    OR NEW.evidence-ARRAY['PortfolioID','Observation','CashUSD','AvailableCashUSD','TotalBase','AvailableBase','Complete','NoOpenOrders','StartedAt','CompletedAt']<>'{}'::jsonb
    OR NEW.evidence->>'PortfolioID' IS DISTINCT FROM NEW.portfolio_id::text
    OR NEW.evidence->'Complete' IS DISTINCT FROM 'true'::jsonb OR NEW.evidence->'NoOpenOrders' IS DISTINCT FROM 'true'::jsonb
    OR (NEW.evidence->>'StartedAt')::timestamptz IS DISTINCT FROM NEW.started_at
    OR (NEW.evidence->>'CompletedAt')::timestamptz IS DISTINCT FROM NEW.observed_at THEN
    RAISE EXCEPTION 'account settlement evidence binding mismatch' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  FOREACH k IN ARRAY ARRAY['CashUSD','AvailableCashUSD','TotalBase','AvailableBase'] LOOP
    IF jsonb_typeof(NEW.evidence->k) IS DISTINCT FROM 'string' OR (NEW.evidence->>k)!~'^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$' THEN
      RAISE EXCEPTION 'invalid account settlement amount' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
    END IF;
  END LOOP;
  IF (NEW.evidence->>'CashUSD')::numeric IS DISTINCT FROM NEW.closing_cash_usd
    OR (NEW.evidence->>'AvailableCashUSD')::numeric IS DISTINCT FROM NEW.closing_cash_usd
    OR (NEW.evidence->>'TotalBase')::numeric IS DISTINCT FROM NEW.closing_base
    OR (NEW.evidence->>'AvailableBase')::numeric IS DISTINCT FROM NEW.closing_base THEN
    RAISE EXCEPTION 'account settlement balances mismatch' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  b=NEW.evidence->'Observation';
  IF jsonb_typeof(b) IS DISTINCT FROM 'object'
    OR NOT (b ?& ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','Status','CompleteFills','FillCount','BaseQuantity','GrossUSD','FeeUSD','Fills','StartedAt','ObservedAt'])
    OR b-ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','Status','CompleteFills','FillCount','BaseQuantity','GrossUSD','FeeUSD','Fills','StartedAt','ObservedAt']<>'{}'::jsonb
    OR b->>'OwnerID' IS DISTINCT FROM o.owner_id::text OR b->>'OrderID' IS DISTINCT FROM o.id::text
    OR b->>'AccountID' IS DISTINCT FROM o.financial_account_id::text OR b->>'ConnectionID' IS DISTINCT FROM o.provider_connection_id::text
    OR b->>'ClientOrderID' IS DISTINCT FROM o.client_order_id::text OR b->>'ProviderOrderID' IS DISTINCT FROM NEW.provider_order_id::text
    OR b->>'ProductID' IS DISTINCT FROM o.request->>'ProductID' OR b->>'Side' IS DISTINCT FROM o.request->>'Side'
    OR b->>'Status' IS DISTINCT FROM NEW.terminal_status OR b->'CompleteFills' IS DISTINCT FROM 'true'::jsonb
    OR (b->>'FillCount')::bigint IS DISTINCT FROM n OR (b->>'BaseQuantity')::numeric IS DISTINCT FROM q
    OR (b->>'GrossUSD')::numeric IS DISTINCT FROM g OR (b->>'FeeUSD')::numeric IS DISTINCT FROM f
    OR jsonb_typeof(b->'StartedAt') IS DISTINCT FROM 'string' OR jsonb_typeof(b->'ObservedAt') IS DISTINCT FROM 'string'
    OR (b->>'StartedAt')::timestamptz<NEW.started_at OR (b->>'ObservedAt')::timestamptz<(b->>'StartedAt')::timestamptz
    OR (b->>'ObservedAt')::timestamptz>NEW.observed_at OR jsonb_typeof(b->'Fills') IS DISTINCT FROM 'array' THEN
    RAISE EXCEPTION 'account settlement observation mismatch' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  IF jsonb_array_length(b->'Fills')<>n
    OR (SELECT count(DISTINCT value->>'TradeID') FROM jsonb_array_elements(b->'Fills'))<>n
    OR (SELECT count(DISTINCT value->'ProviderEvidence'->>'EntryID') FROM jsonb_array_elements(b->'Fills'))<>n THEN
    RAISE EXCEPTION 'account settlement fill coverage mismatch' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(b->'Fills') LOOP
    SELECT payload INTO saved FROM execution_fills WHERE order_id=o.id AND trade_id=item->>'TradeID';
    IF NOT FOUND OR NOT execution_settlement_fill_matches(item,saved,b->'ObservedAt') THEN
      RAISE EXCEPTION 'account settlement fill identity mismatch' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
    END IF;
  END LOOP;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_account_settlement_guard BEFORE INSERT ON execution_account_settlements
  FOR EACH ROW EXECUTE FUNCTION guard_execution_account_settlement();

-- +goose StatementBegin
CREATE FUNCTION guard_execution_capital_release() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.released_at IS NOT NULL THEN RAISE EXCEPTION 'capital cannot be born released' USING ERRCODE='23514'; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'capital history is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.released_at IS NOT NULL OR NEW.released_at IS NULL
    OR to_jsonb(NEW)-'released_at' IS DISTINCT FROM to_jsonb(OLD)-'released_at'
    OR NOT EXISTS(SELECT 1 FROM execution_account_settlements s WHERE s.order_id=OLD.order_id
      AND s.owner_id=OLD.owner_id AND s.financial_account_id=OLD.financial_account_id
      AND s.capital_bucket_id=OLD.capital_bucket_id AND s.recorded_at=NEW.released_at) THEN
    RAISE EXCEPTION 'capital release requires exact immutable settlement' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
DROP TRIGGER execution_capital_immutable ON execution_capital_reservations;
CREATE TRIGGER execution_capital_immutable BEFORE INSERT OR UPDATE OR DELETE ON execution_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION guard_execution_capital_release();

-- +goose StatementBegin
CREATE FUNCTION release_settled_execution_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE execution_capital_reservations SET released_at=NEW.recorded_at WHERE order_id=NEW.order_id AND released_at IS NULL;
  IF NOT FOUND THEN RAISE EXCEPTION 'settlement lost original reservation' USING ERRCODE='23514'; END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_account_settlement_release AFTER INSERT ON execution_account_settlements
  FOR EACH ROW EXECUTE FUNCTION release_settled_execution_capital();

-- Default-deferred control-plane transactions recheck access and freshness at
-- commit, in addition to the service's final current-control check. This is not
-- a privilege boundary against SQL callers that force the constraint IMMEDIATE.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_settlement_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF lock_execution_settlement_controls(NEW.order_id,NEW.portfolio_id)<>NEW.credential_generation
    OR clock_timestamp()-NEW.observed_at>interval '30 seconds'
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id)
    OR NOT EXISTS(SELECT 1 FROM execution_capital_reservations WHERE order_id=NEW.order_id AND released_at=NEW.recorded_at) THEN
    RAISE EXCEPTION 'settlement is stale at commit' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER execution_account_settlement_commit AFTER INSERT ON execution_account_settlements
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_execution_settlement_commit();

-- Only unreleased reservations block subsequent allocations. All reciprocal
-- fences and the original current-control checks remain in force.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_capital_claim() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint;
BEGIN
  generation=lock_execution_claim_controls(NEW.order_id);
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF NEW.credential_generation<>generation OR NEW.expires_at<=clock_timestamp() OR NEW.claimed_at>clock_timestamp() THEN
    RAISE EXCEPTION 'execution authorization is stale' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=NEW.financial_account_id AND released_at IS NULL)
    OR EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=NEW.financial_account_id AND expires_at>clock_timestamp())
    OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=NEW.financial_account_id AND execution_mode<>'PAPER' AND released_at IS NULL) THEN
    RAISE EXCEPTION 'account capital is reserved' USING ERRCODE='23514',CONSTRAINT='execution_capital_held';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_other_execution_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME='strategy_capital_reservations' THEN
    IF NEW.execution_mode='PAPER' THEN RETURN NEW; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text||':'||NEW.financial_account_id::text||':strategy-capital-reservations',0));
  ELSE
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text||':'||NEW.financial_account_id::text||':manual-order-reservations',0));
  END IF;
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=NEW.financial_account_id AND released_at IS NULL) THEN
    RAISE EXCEPTION 'account capital is reserved' USING ERRCODE='23514',CONSTRAINT='execution_capital_held';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_account_settlements) OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE released_at IS NOT NULL) THEN
    RAISE EXCEPTION 'cannot remove immutable account settlement history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE execution_account_settlements;
DROP FUNCTION guard_execution_settlement_commit();
DROP FUNCTION guard_execution_account_settlement();
DROP FUNCTION release_settled_execution_capital();
DROP FUNCTION execution_settlement_fill_matches(jsonb,jsonb,jsonb);
DROP FUNCTION lock_execution_settlement_controls(uuid,uuid);
DROP TRIGGER execution_capital_immutable ON execution_capital_reservations;
DROP FUNCTION guard_execution_capital_release();
CREATE TRIGGER execution_capital_immutable BEFORE UPDATE OR DELETE ON execution_capital_reservations FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
DROP INDEX execution_capital_active_account;
ALTER TABLE execution_capital_reservations DROP COLUMN released_at;
ALTER TABLE execution_capital_reservations ADD CONSTRAINT execution_capital_reservations_financial_account_id_key UNIQUE(financial_account_id);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_capital_claim() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint;
BEGIN
  generation=lock_execution_claim_controls(NEW.order_id);
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF NEW.credential_generation<>generation OR NEW.expires_at<=clock_timestamp() OR NEW.claimed_at>clock_timestamp() THEN
    RAISE EXCEPTION 'execution authorization is stale' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  IF EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=NEW.financial_account_id)
    OR EXISTS(SELECT 1 FROM capital_reservations WHERE financial_account_id=NEW.financial_account_id AND expires_at>clock_timestamp())
    OR EXISTS(SELECT 1 FROM strategy_capital_reservations WHERE financial_account_id=NEW.financial_account_id AND execution_mode<>'PAPER' AND released_at IS NULL) THEN
    RAISE EXCEPTION 'account capital is reserved' USING ERRCODE='23514',CONSTRAINT='execution_capital_held';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_other_execution_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME='strategy_capital_reservations' THEN
    IF NEW.execution_mode='PAPER' THEN RETURN NEW; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text||':'||NEW.financial_account_id::text||':strategy-capital-reservations',0));
  ELSE
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text||':'||NEW.financial_account_id::text||':manual-order-reservations',0));
  END IF;
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'account capital is reserved' USING ERRCODE='23514',CONSTRAINT='execution_capital_held';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
