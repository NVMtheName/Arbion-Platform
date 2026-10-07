-- +goose Up
-- Inert opt-in accounting, not autonomous authority or a funding transfer.
-- Registration is permanent and precedes every order on the account. Existing
-- unregistered accounts retain the legacy individually confirmed order path.
CREATE TABLE execution_pilot_allocations (
  capital_bucket_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL UNIQUE,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  provider_account_id text NOT NULL CHECK(provider_account_id ~ '^portfolio:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'),
  product_id text NOT NULL CHECK(product_id ~ '^[A-Z][A-Z0-9]{0,15}-USD$' AND product_id<>'USD-USD'),
  initial_cash_usd text NOT NULL CHECK(initial_cash_usd ~ '^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$' AND initial_cash_usd::numeric>0),
  maximum_order_usd text NOT NULL CHECK(maximum_order_usd ~ '^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$' AND maximum_order_usd::numeric>0 AND maximum_order_usd::numeric<=initial_cash_usd::numeric),
  expires_at timestamptz NOT NULL CHECK(isfinite(expires_at) AND expires_at>='0001-01-01 UTC' AND expires_at<'10000-01-01 UTC'),
  registered_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE(owner_id,provider_account_id),
  FOREIGN KEY(financial_account_id,owner_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT
);
CREATE TRIGGER execution_pilot_allocation_immutable BEFORE UPDATE OR DELETE ON execution_pilot_allocations
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_pilot_allocation_no_truncate BEFORE TRUNCATE ON execution_pilot_allocations
  FOR EACH STATEMENT EXECUTE FUNCTION keep_execution_evidence();

-- Used before BOTH registration and EVERY order insert, including unregistered
-- accounts. A waiter must see the winner's commit, never a stale snapshot.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_pilot_account(owner uuid, account uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'pilot accounting requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  PERFORM 1 FROM users WHERE id=owner FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(owner::text||':'||account::text||':manual-order-reservations',0));
  PERFORM 1 FROM financial_accounts WHERE id=account AND user_id=owner FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'execution account binding mismatch' USING ERRCODE='23514';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION guard_execution_pilot_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a financial_accounts%ROWTYPE; c provider_connections%ROWTYPE; b capital_buckets%ROWTYPE;
BEGIN
  PERFORM lock_execution_pilot_account(NEW.owner_id,NEW.financial_account_id);
  SELECT * INTO STRICT a FROM financial_accounts WHERE id=NEW.financial_account_id;
  SELECT * INTO c FROM provider_connections WHERE id=NEW.provider_connection_id AND user_id=NEW.owner_id FOR SHARE;
  SELECT * INTO b FROM capital_buckets WHERE id=NEW.capital_bucket_id AND user_id=NEW.owner_id AND financial_account_id=NEW.financial_account_id FOR SHARE;
  IF EXISTS(SELECT 1 FROM execution_orders WHERE financial_account_id=NEW.financial_account_id)
    OR a.status<>'active' OR a.provider_name<>'coinbase' OR a.base_currency<>'USD'
    OR a.provider_connection_id<>NEW.provider_connection_id
    OR c.id IS NULL OR c.status<>'active' OR c.provider_category<>'financial' OR c.provider_name<>'coinbase'
    OR (c.authorization_expires_at IS NOT NULL AND c.authorization_expires_at<=clock_timestamp())
    OR b.id IS NULL OR b.status<>'ACTIVE' OR b.is_reserve OR b.currency<>'USD' OR b.allocation_type<>'FIXED_AMOUNT'
    OR NEW.initial_cash_usd::numeric>LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))-b.protected_amount
    OR NEW.expires_at<=clock_timestamp() THEN
    RAISE EXCEPTION 'pilot registration unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  NEW.provider_account_id=a.provider_account_id;
  NEW.registered_at=clock_timestamp();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_pilot_allocation_guard BEFORE INSERT ON execution_pilot_allocations
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_allocation();

-- The existing request digest already pins all these scope fields. Permanent
-- registration before any order makes attribution unambiguous without importing
-- legacy fills or changing historical request digests.
-- +goose StatementBegin
CREATE FUNCTION check_execution_pilot_scope(account uuid, owner uuid, connection uuid, bucket uuid, request jsonb) RETURNS void LANGUAGE plpgsql AS $$
DECLARE p execution_pilot_allocations%ROWTYPE; provider_account text;
BEGIN
  SELECT * INTO p FROM execution_pilot_allocations WHERE financial_account_id=account;
  SELECT provider_account_id INTO provider_account FROM financial_accounts WHERE id=account;
  IF p.financial_account_id IS NULL THEN
    -- A replacement internal account cannot take the legacy fallback for a
    -- real portfolio whose allocation already exists permanently elsewhere.
    IF EXISTS(SELECT 1 FROM execution_pilot_allocations WHERE owner_id=owner AND provider_account_id=provider_account) THEN
      RAISE EXCEPTION 'pilot portfolio already bound' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
    END IF;
    RETURN;
  END IF;
  IF p.owner_id IS DISTINCT FROM owner OR p.provider_connection_id IS DISTINCT FROM connection
    OR p.capital_bucket_id IS DISTINCT FROM bucket OR p.product_id IS DISTINCT FROM request->>'ProductID'
    OR p.provider_account_id IS DISTINCT FROM provider_account
    OR p.maximum_order_usd IS DISTINCT FROM request->'PilotLimits'->>'MaximumOrderUSD'
    OR p.expires_at IS DISTINCT FROM execution_pilot_deadline(request) THEN
    RAISE EXCEPTION 'pilot scope mismatch' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION guard_execution_order_allocation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM lock_execution_pilot_account(NEW.owner_id,NEW.financial_account_id);
  PERFORM check_execution_pilot_scope(NEW.financial_account_id,NEW.owner_id,NEW.provider_connection_id,NEW.capital_bucket_id,NEW.request);
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_order_allocation_guard BEFORE INSERT ON execution_orders
  FOR EACH ROW EXECUTE FUNCTION guard_execution_order_allocation();

-- One immutable receipt per order: no raw-fill join or rounded accumulator.
-- Include partial CANCELLED/EXPIRED fills. Unsettled fills and no-send receipts
-- contribute nothing; the existing unresolved capital hold still blocks reuse.
-- +goose StatementBegin
CREATE FUNCTION execution_pilot_balance(account uuid)
RETURNS TABLE(cash_usd numeric,base_quantity numeric,settled_order_count bigint) LANGUAGE sql STABLE AS $$
  SELECT p.initial_cash_usd::numeric+COALESCE(SUM(CASE WHEN o.request->>'Side'='BUY'
      THEN -s.gross_usd-s.fee_usd ELSE s.gross_usd-s.fee_usd END),0),
    COALESCE(SUM(CASE WHEN o.request->>'Side'='BUY' THEN s.base_quantity ELSE -s.base_quantity END),0),COUNT(s.order_id)
  FROM execution_pilot_allocations p
  LEFT JOIN execution_orders o ON o.financial_account_id=p.financial_account_id AND o.owner_id=p.owner_id
    AND o.provider_connection_id=p.provider_connection_id AND o.capital_bucket_id=p.capital_bucket_id
    AND o.request->>'ProductID'=p.product_id
  LEFT JOIN execution_account_settlements s ON s.order_id=o.id
  WHERE p.financial_account_id=account GROUP BY p.capital_bucket_id
$$;
-- +goose StatementEnd

-- Admission only. Do not put this in shared recovery/cancellation/settlement
-- locks: expiry, stops and exhausted pilot cash must not prevent safe accounting.
-- Caller holds current account and capital fences; the dispatch trigger runs
-- after execution_capital_claim_guard, and final OwnerAuthority repeats it.
-- +goose StatementBegin
CREATE FUNCTION check_execution_pilot_capital(target uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; p execution_pilot_allocations%ROWTYPE; cash numeric; base numeric; capacity numeric;
BEGIN
  SELECT * INTO STRICT o FROM execution_orders WHERE id=target;
  PERFORM check_execution_pilot_scope(o.financial_account_id,o.owner_id,o.provider_connection_id,o.capital_bucket_id,o.request);
  SELECT * INTO p FROM execution_pilot_allocations WHERE financial_account_id=o.financial_account_id;
  IF NOT FOUND THEN RETURN; END IF;
  SELECT cash_usd,base_quantity INTO cash,base FROM execution_pilot_balance(o.financial_account_id);
  SELECT LEAST(allocation_value,COALESCE(allocation_limit,allocation_value))-protected_amount INTO capacity
    FROM capital_buckets WHERE id=p.capital_bucket_id;
  IF cash IS NULL OR base IS NULL OR cash<0 OR base<0 OR capacity<p.initial_cash_usd::numeric
    OR p.expires_at<=clock_timestamp()
    OR (o.request->>'Side'='BUY' AND (o.request->>'MaximumDebitUSD')::numeric>cash)
    OR (o.request->>'Side'='SELL' AND (o.request->>'BaseSize')::numeric>base) THEN
    RAISE EXCEPTION 'pilot capital unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION guard_execution_pilot_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM check_execution_pilot_capital(NEW.order_id);
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_pilot_capital_guard BEFORE INSERT ON execution_dispatch_attempts
  FOR EACH ROW EXECUTE FUNCTION guard_execution_pilot_capital();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_pilot_allocations) THEN
    RAISE EXCEPTION 'cannot remove immutable pilot allocations';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER execution_pilot_capital_guard ON execution_dispatch_attempts;
DROP TRIGGER execution_order_allocation_guard ON execution_orders;
DROP FUNCTION guard_execution_pilot_capital();
DROP FUNCTION check_execution_pilot_capital(uuid);
DROP FUNCTION execution_pilot_balance(uuid);
DROP FUNCTION guard_execution_order_allocation();
DROP FUNCTION check_execution_pilot_scope(uuid,uuid,uuid,uuid,jsonb);
DROP TABLE execution_pilot_allocations;
DROP FUNCTION guard_execution_pilot_allocation();
DROP FUNCTION lock_execution_pilot_account(uuid,uuid);
