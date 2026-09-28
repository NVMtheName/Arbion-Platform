-- +goose Up
-- Preserve every attempt; move uniqueness to a guarded active account slot.
ALTER TABLE execution_broker_acknowledgements ADD CONSTRAINT execution_ack_binding_unique UNIQUE(order_id,owner_id,provider_order_id);
CREATE TABLE execution_account_holds (
  financial_account_id uuid PRIMARY KEY,
  order_id uuid NOT NULL UNIQUE,
  owner_id uuid NOT NULL,
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id),
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id)
);
INSERT INTO execution_account_holds(financial_account_id,order_id,owner_id)
  SELECT financial_account_id,order_id,owner_id FROM execution_dispatch_attempts;

CREATE TABLE execution_fills (
  order_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_order_id uuid NOT NULL,
  trade_id text NOT NULL CHECK(trade_id ~ '^[a-zA-Z0-9_-]{1,128}$'),
  base_quantity numeric NOT NULL CHECK(base_quantity>0 AND base_quantity<1e18 AND scale(base_quantity)<=18),
  price_usd numeric NOT NULL CHECK(price_usd>0 AND price_usd<1e18 AND scale(price_usd)<=18),
  gross_usd numeric NOT NULL CHECK(gross_usd>0 AND gross_usd<1e36 AND scale(gross_usd)<=36 AND gross_usd=base_quantity*price_usd),
  fee_usd numeric NOT NULL CHECK(fee_usd>=0 AND fee_usd<1e18 AND scale(fee_usd)<=18),
  traded_at timestamptz NOT NULL,
  observed_at timestamptz NOT NULL CHECK(observed_at>=traded_at),
  payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=8192),
  PRIMARY KEY(order_id,trade_id),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id),
  FOREIGN KEY(order_id,owner_id,provider_order_id) REFERENCES execution_broker_acknowledgements(order_id,owner_id,provider_order_id)
);

CREATE TABLE execution_order_terminals (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_order_id uuid NOT NULL,
  status text NOT NULL CHECK(status IN ('FILLED','CANCELLED','REJECTED','EXPIRED')),
  fill_count bigint NOT NULL CHECK(fill_count>=0),
  base_quantity numeric NOT NULL CHECK(base_quantity>=0 AND base_quantity<1e18 AND scale(base_quantity)<=18),
  gross_usd numeric NOT NULL CHECK(gross_usd>=0 AND gross_usd<1e36 AND scale(gross_usd)<=36),
  fee_usd numeric NOT NULL CHECK(fee_usd>=0 AND fee_usd<1e18 AND scale(fee_usd)<=18),
  completed_at timestamptz NOT NULL,
  observed_at timestamptz NOT NULL CHECK(observed_at>=completed_at),
  payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=8192),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id),
  FOREIGN KEY(order_id,owner_id,provider_order_id) REFERENCES execution_broker_acknowledgements(order_id,owner_id,provider_order_id)
);

CREATE TABLE execution_reconciliation_blocks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id uuid NOT NULL,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  reason text NOT NULL CHECK(reason IN ('INVALID_FILL','FILL_CONFLICT','LATE_FILL','FILL_LIMIT_BREACH','TERMINAL_CONFLICT')),
  payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object' AND octet_length(payload::text)<=8192),
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id)
);
CREATE INDEX execution_reconciliation_blocks_account_idx ON execution_reconciliation_blocks(financial_account_id);

-- +goose StatementBegin
CREATE FUNCTION guard_execution_attempt_slot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id AND user_id=NEW.owner_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'execution account reconciliation blocked' USING ERRCODE='23514';
  END IF;
  INSERT INTO execution_account_holds(financial_account_id,order_id,owner_id)
    VALUES(NEW.financial_account_id,NEW.order_id,NEW.owner_id);
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_attempt_slot AFTER INSERT ON execution_dispatch_attempts FOR EACH ROW EXECUTE FUNCTION guard_execution_attempt_slot();
ALTER TABLE execution_dispatch_attempts DROP CONSTRAINT execution_dispatch_attempts_financial_account_id_key;

-- +goose StatementBegin
CREATE FUNCTION guard_execution_account_hold() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'execution account hold cannot be rewritten' USING ERRCODE='23514'; END IF;
  IF TG_OP='DELETE' THEN
    PERFORM 1 FROM financial_accounts WHERE id=OLD.financial_account_id FOR UPDATE;
    IF NOT EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=OLD.order_id AND owner_id=OLD.owner_id AND financial_account_id=OLD.financial_account_id)
      OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=OLD.financial_account_id) THEN
      RAISE EXCEPTION 'execution hold requires exact terminal reconciliation' USING ERRCODE='23514';
    END IF;
    RETURN OLD;
  END IF;
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=NEW.order_id)
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'execution hold cannot reopen a terminal or blocked account' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_account_hold_guard BEFORE INSERT OR UPDATE OR DELETE ON execution_account_holds FOR EACH ROW EXECUTE FUNCTION guard_execution_account_hold();

-- +goose StatementBegin
CREATE FUNCTION guard_execution_fill() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; q numeric; g numeric; f numeric; claimed timestamptz;
BEGIN
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id AND user_id=NEW.owner_id FOR UPDATE;
  SELECT o.request,a.claimed_at INTO STRICT r,claimed FROM execution_orders o JOIN execution_dispatch_attempts a ON a.order_id=o.id
    WHERE o.id=NEW.order_id AND o.owner_id=NEW.owner_id AND o.financial_account_id=NEW.financial_account_id;
  IF NEW.traded_at<claimed OR NEW.observed_at>clock_timestamp()
    OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=NEW.order_id)
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'execution fill timing or lifecycle mismatch' USING ERRCODE='23514';
  END IF;
  SELECT COALESCE(sum(base_quantity),0)+NEW.base_quantity,COALESCE(sum(gross_usd),0)+NEW.gross_usd,COALESCE(sum(fee_usd),0)+NEW.fee_usd
    INTO q,g,f FROM execution_fills WHERE order_id=NEW.order_id;
  IF q>(r->>'BaseSize')::numeric OR f>(r->>'FeeAllowanceUSD')::numeric
    OR (r->>'Side'='BUY' AND (NEW.price_usd>(r->>'LimitPrice')::numeric OR g+f>(r->>'MaximumDebitUSD')::numeric))
    OR (r->>'Side'='SELL' AND (NEW.price_usd<(r->>'LimitPrice')::numeric OR g<f)) THEN
    RAISE EXCEPTION 'execution fill exceeds exact order bounds' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_fill_guard BEFORE INSERT ON execution_fills FOR EACH ROW EXECUTE FUNCTION guard_execution_fill();
CREATE TRIGGER execution_fill_immutable BEFORE UPDATE OR DELETE ON execution_fills FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- +goose StatementBegin
CREATE FUNCTION guard_execution_terminal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; n bigint; q numeric; g numeric; f numeric; latest timestamptz; claimed timestamptz;
BEGIN
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id AND user_id=NEW.owner_id FOR UPDATE;
  SELECT o.request,a.claimed_at INTO STRICT r,claimed FROM execution_orders o JOIN execution_dispatch_attempts a ON a.order_id=o.id
    WHERE o.id=NEW.order_id AND o.owner_id=NEW.owner_id AND o.financial_account_id=NEW.financial_account_id;
  SELECT count(*),COALESCE(sum(base_quantity),0),COALESCE(sum(gross_usd),0),COALESCE(sum(fee_usd),0),max(traded_at)
    INTO n,q,g,f,latest FROM execution_fills WHERE order_id=NEW.order_id;
  IF (NEW.payload->'CompleteFills') IS DISTINCT FROM 'true'::jsonb
    OR NEW.fill_count<>n OR NEW.base_quantity<>q OR NEW.gross_usd<>g OR NEW.fee_usd<>f
    OR NEW.completed_at<claimed OR NEW.completed_at<latest OR NEW.observed_at>clock_timestamp()
    OR (NEW.status='FILLED' AND q<>(r->>'BaseSize')::numeric)
    OR (NEW.status='REJECTED' AND n<>0)
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id)
    OR NOT EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=NEW.financial_account_id AND order_id=NEW.order_id) THEN
    RAISE EXCEPTION 'execution terminal totals or lifecycle mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_terminal_guard BEFORE INSERT ON execution_order_terminals FOR EACH ROW EXECUTE FUNCTION guard_execution_terminal();
CREATE TRIGGER execution_terminal_immutable BEFORE UPDATE OR DELETE ON execution_order_terminals FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
-- +goose StatementBegin
CREATE FUNCTION release_execution_terminal_slot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM execution_account_holds WHERE financial_account_id=NEW.financial_account_id AND order_id=NEW.order_id;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_terminal_release AFTER INSERT ON execution_order_terminals FOR EACH ROW EXECUTE FUNCTION release_execution_terminal_slot();

-- +goose StatementBegin
CREATE FUNCTION lock_execution_block_account() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id AND user_id=NEW.owner_id FOR UPDATE;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_block_account BEFORE INSERT ON execution_reconciliation_blocks FOR EACH ROW EXECUTE FUNCTION lock_execution_block_account();
CREATE TRIGGER execution_block_immutable BEFORE UPDATE OR DELETE ON execution_reconciliation_blocks FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Bind the replay payload to the exact relational facts used for settlement.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_observation_payload() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; keys text[];
BEGIN
  SELECT request INTO STRICT r FROM execution_orders WHERE id=NEW.order_id;
  keys:=ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','BaseQuantity','GrossUSD','FeeUSD','ObservedAt'];
  IF TG_TABLE_NAME='execution_fills' THEN keys:=keys||ARRAY['TradeID','PriceUSD','TradedAt'];
  ELSE keys:=keys||ARRAY['Status','CompleteFills','FillCount','CompletedAt']; END IF;
  IF NOT (NEW.payload ?& keys) OR NEW.payload-keys<>'{}'::jsonb
    OR NEW.payload->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.payload->>'OrderID' IS DISTINCT FROM NEW.order_id::text
    OR NEW.payload->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.payload->>'ProviderOrderID' IS DISTINCT FROM NEW.provider_order_id::text
    OR NEW.payload->>'ConnectionID' IS DISTINCT FROM r->>'ConnectionID'
    OR NEW.payload->>'ClientOrderID' IS DISTINCT FROM r->>'ClientOrderID'
    OR NEW.payload->>'ProductID' IS DISTINCT FROM r->>'ProductID'
    OR NEW.payload->>'Side' IS DISTINCT FROM r->>'Side'
    OR (NEW.payload->>'BaseQuantity')::numeric IS DISTINCT FROM NEW.base_quantity
    OR (NEW.payload->>'GrossUSD')::numeric IS DISTINCT FROM NEW.gross_usd
    OR (NEW.payload->>'FeeUSD')::numeric IS DISTINCT FROM NEW.fee_usd
    OR (NEW.payload->>'ObservedAt')::timestamptz IS DISTINCT FROM NEW.observed_at THEN
    RAISE EXCEPTION 'execution observation payload mismatch' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='execution_fills' THEN
    IF NEW.payload->>'TradeID' IS DISTINCT FROM NEW.trade_id OR (NEW.payload->>'PriceUSD')::numeric IS DISTINCT FROM NEW.price_usd
      OR (NEW.payload->>'TradedAt')::timestamptz IS DISTINCT FROM NEW.traded_at THEN
      RAISE EXCEPTION 'execution fill payload mismatch' USING ERRCODE='23514';
    END IF;
  ELSE
    IF NEW.payload->>'Status' IS DISTINCT FROM NEW.status OR (NEW.payload->>'FillCount')::bigint IS DISTINCT FROM NEW.fill_count
      OR (NEW.payload->>'CompletedAt')::timestamptz IS DISTINCT FROM NEW.completed_at THEN
      RAISE EXCEPTION 'execution terminal payload mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_fill_payload BEFORE INSERT ON execution_fills FOR EACH ROW EXECUTE FUNCTION guard_execution_observation_payload();
CREATE TRIGGER execution_terminal_payload BEFORE INSERT ON execution_order_terminals FOR EACH ROW EXECUTE FUNCTION guard_execution_observation_payload();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_fills) OR EXISTS(SELECT 1 FROM execution_order_terminals) OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks) THEN
    RAISE EXCEPTION 'cannot remove execution reconciliation evidence';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE execution_dispatch_attempts ADD UNIQUE(financial_account_id);
DROP TRIGGER execution_attempt_slot ON execution_dispatch_attempts;
DROP TABLE execution_reconciliation_blocks;
DROP TABLE execution_order_terminals;
DROP TABLE execution_fills;
DROP TABLE execution_account_holds;
DROP FUNCTION lock_execution_block_account();
DROP FUNCTION release_execution_terminal_slot();
DROP FUNCTION guard_execution_terminal();
DROP FUNCTION guard_execution_fill();
DROP FUNCTION guard_execution_account_hold();
DROP FUNCTION guard_execution_attempt_slot();
DROP FUNCTION guard_execution_observation_payload();
ALTER TABLE execution_broker_acknowledgements DROP CONSTRAINT execution_ack_binding_unique;
