-- +goose Up
-- A terminal order frees a submission slot, NOT the capital/account fence.
-- There is deliberately no release writer until account reconciliation exists.
-- The row write also rejects stale SERIALIZABLE/REPEATABLE READ snapshots after
-- an advisory-lock wait; advisory locking alone does not refresh those snapshots.
CREATE TABLE execution_capital_fences (
  financial_account_id uuid PRIMARY KEY REFERENCES financial_accounts(id) ON DELETE RESTRICT,
  revision bigint NOT NULL DEFAULT 1 CHECK(revision>0)
);

-- +goose StatementBegin
CREATE FUNCTION lock_execution_capital_fence(account_id uuid) RETURNS void LANGUAGE sql AS $$
  INSERT INTO execution_capital_fences(financial_account_id) VALUES(account_id)
  ON CONFLICT(financial_account_id) DO UPDATE SET revision=execution_capital_fences.revision+1
$$;
-- +goose StatementEnd

CREATE TABLE execution_capital_reservations (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL UNIQUE,
  capital_bucket_id uuid NOT NULL,
  resource_type text NOT NULL CHECK(resource_type IN ('CASH','ASSET')),
  resource_asset text NOT NULL,
  quantity numeric NOT NULL CHECK(quantity>0 AND quantity<1e18 AND scale(quantity)<=18),
  reserved_at timestamptz NOT NULL,
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT
);

-- Fail closed on previously reused accounts; never guess which funds are free.
INSERT INTO execution_capital_reservations
SELECT o.id,o.owner_id,o.financial_account_id,o.capital_bucket_id,
  CASE WHEN o.request->>'Side'='BUY' THEN 'CASH' ELSE 'ASSET' END,
  CASE WHEN o.request->>'Side'='BUY' THEN 'USD' ELSE split_part(o.request->>'ProductID','-',1) END,
  CASE WHEN o.request->>'Side'='BUY' THEN (o.request->>'MaximumDebitUSD')::numeric ELSE (o.request->>'BaseSize')::numeric END,a.claimed_at
FROM execution_orders o JOIN execution_dispatch_attempts a ON a.order_id=o.id;

CREATE TRIGGER execution_capital_immutable BEFORE UPDATE OR DELETE ON execution_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- All claim callers (including SQL) use owner -> entitlement -> manual fence ->
-- account -> connection -> bucket -> strategy fence -> breaker scopes. Account
-- sync/retirement and existing non-live commits already use account before provider.
-- Shared breaker locks cover absent rows and serialize the first stop too.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_claim_controls(target uuid) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE
  o execution_orders%ROWTYPE;
  a financial_accounts%ROWTYPE;
  c provider_connections%ROWTYPE;
  b capital_buckets%ROWTYPE;
  e user_entitlements%ROWTYPE;
  owner_status text;
  checked_at timestamptz;
  capacity numeric;
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'execution claim requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  SELECT * INTO STRICT o FROM execution_orders WHERE id=target;
  SELECT status INTO STRICT owner_status FROM users WHERE id=o.owner_id FOR SHARE;
  SELECT * INTO e FROM user_entitlements WHERE user_id=o.owner_id AND entitlement_key='founder' FOR SHARE;
  IF owner_status<>'active' OR e.id IS NULL OR e.status<>'active' THEN
    RAISE EXCEPTION 'execution access unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text || ':' || o.financial_account_id::text || ':manual-order-reservations',0));
  SELECT * INTO STRICT a FROM financial_accounts WHERE id=o.financial_account_id AND user_id=o.owner_id FOR UPDATE;
  SELECT * INTO STRICT c FROM provider_connections WHERE id=o.provider_connection_id AND user_id=o.owner_id FOR SHARE;
  SELECT * INTO STRICT b FROM capital_buckets WHERE id=o.capital_bucket_id AND user_id=o.owner_id AND financial_account_id=o.financial_account_id FOR SHARE;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text || ':' || o.financial_account_id::text || ':strategy-capital-reservations',0));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('GLOBAL',NULL));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('USER',o.owner_id));
  PERFORM pg_advisory_xact_lock_shared(risk_breaker_scope_lock_key('ACCOUNT',o.financial_account_id));
  checked_at=clock_timestamp();
  capacity=LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))-b.protected_amount;
  IF e.starts_at>checked_at OR (e.expires_at IS NOT NULL AND e.expires_at<=checked_at)
    OR a.status<>'active' OR a.provider_name<>'coinbase' OR a.base_currency<>'USD'
    OR a.provider_connection_id<>o.provider_connection_id
    OR c.status<>'active' OR c.provider_name<>'coinbase' OR c.provider_category<>'financial'
    OR (c.authorization_expires_at IS NOT NULL AND c.authorization_expires_at<=checked_at)
    OR b.status<>'ACTIVE' OR b.is_reserve OR b.currency<>'USD' OR b.allocation_type<>'FIXED_AMOUNT'
    OR capacity<=0
    OR (o.request->>'Side'='BUY' AND (o.request->>'MaximumDebitUSD')::numeric>capacity)
    OR (o.request->>'Side'='SELL' AND (o.request->>'BaseSize')::numeric*(o.request->>'LimitPrice')::numeric>capacity)
    OR EXISTS(SELECT 1 FROM risk_circuit_breakers WHERE state='OPEN' AND
      (scope='GLOBAL' OR (scope='USER' AND scope_id=o.owner_id) OR (scope='ACCOUNT' AND scope_id=o.financial_account_id))) THEN
    RAISE EXCEPTION 'execution current controls deny claim' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
  END IF;
  RETURN c.credential_generation;
END $$;
-- +goose StatementEnd

-- This is a necessary storage boundary, not sufficient live authority. The
-- application still requires exact live approval, fresh risk/quote/account proof,
-- product rules and a revocation-safe sender. No production caller exists yet.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_capital_claim() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint;
BEGIN
  generation=lock_execution_claim_controls(NEW.order_id);
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF NEW.credential_generation<>generation OR NEW.expires_at<=clock_timestamp()
    OR NEW.claimed_at>clock_timestamp() THEN
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
CREATE TRIGGER execution_capital_claim_guard BEFORE INSERT ON execution_dispatch_attempts
  FOR EACH ROW EXECUTE FUNCTION guard_execution_capital_claim();

-- +goose StatementBegin
CREATE FUNCTION reserve_execution_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO execution_capital_reservations
  SELECT o.id,o.owner_id,o.financial_account_id,o.capital_bucket_id,
    CASE WHEN o.request->>'Side'='BUY' THEN 'CASH' ELSE 'ASSET' END,
    CASE WHEN o.request->>'Side'='BUY' THEN 'USD' ELSE split_part(o.request->>'ProductID','-',1) END,
    CASE WHEN o.request->>'Side'='BUY' THEN (o.request->>'MaximumDebitUSD')::numeric ELSE (o.request->>'BaseSize')::numeric END,NEW.claimed_at
  FROM execution_orders o WHERE o.id=NEW.order_id;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_capital_reserve AFTER INSERT ON execution_dispatch_attempts
  FOR EACH ROW EXECUTE FUNCTION reserve_execution_capital();

-- Direct inserts cannot forge reservation amounts, scope, or creation time.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_capital_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM execution_orders o JOIN execution_dispatch_attempts a ON a.order_id=o.id
    WHERE o.id=NEW.order_id AND o.owner_id=NEW.owner_id AND o.financial_account_id=NEW.financial_account_id
      AND o.capital_bucket_id=NEW.capital_bucket_id AND a.claimed_at=NEW.reserved_at
      AND ((o.request->>'Side'='BUY' AND NEW.resource_type='CASH' AND NEW.resource_asset='USD' AND NEW.quantity=(o.request->>'MaximumDebitUSD')::numeric)
        OR (o.request->>'Side'='SELL' AND NEW.resource_type='ASSET' AND NEW.resource_asset=split_part(o.request->>'ProductID','-',1) AND NEW.quantity=(o.request->>'BaseSize')::numeric))) THEN
    RAISE EXCEPTION 'execution capital binding mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_capital_binding BEFORE INSERT ON execution_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION guard_execution_capital_binding();

-- Reciprocal fences: manual previews and non-Paper allocation cannot consume
-- resources after a dispatch claim. Paper remains isolated simulation money.
-- +goose StatementBegin
CREATE FUNCTION guard_other_execution_capital() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME='strategy_capital_reservations' THEN
    IF NEW.execution_mode='PAPER' THEN RETURN NEW; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text || ':' || NEW.financial_account_id::text || ':strategy-capital-reservations',0));
  ELSE
    PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text || ':' || NEW.financial_account_id::text || ':manual-order-reservations',0));
  END IF;
  PERFORM lock_execution_capital_fence(NEW.financial_account_id);
  IF EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'account capital is reserved' USING ERRCODE='23514',CONSTRAINT='execution_capital_held';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_manual_capital_fence BEFORE INSERT ON capital_reservations
  FOR EACH ROW EXECUTE FUNCTION guard_other_execution_capital();
CREATE TRIGGER execution_strategy_capital_fence BEFORE INSERT ON strategy_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION guard_other_execution_capital();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_capital_reservations) THEN
    RAISE EXCEPTION 'cannot remove reserved execution capital';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER execution_strategy_capital_fence ON strategy_capital_reservations;
DROP TRIGGER execution_manual_capital_fence ON capital_reservations;
DROP FUNCTION guard_other_execution_capital();
DROP TRIGGER execution_capital_reserve ON execution_dispatch_attempts;
DROP FUNCTION reserve_execution_capital();
DROP TRIGGER execution_capital_claim_guard ON execution_dispatch_attempts;
DROP FUNCTION guard_execution_capital_claim();
DROP FUNCTION lock_execution_claim_controls(uuid);
DROP TABLE execution_capital_reservations;
DROP FUNCTION guard_execution_capital_binding();
DROP FUNCTION lock_execution_capital_fence(uuid);
DROP TABLE execution_capital_fences;
