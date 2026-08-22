-- +goose Up
ALTER TABLE order_intent_risk_evaluations
  ADD COLUMN account_reserved_cash numeric(38,18) NOT NULL DEFAULT 0 CHECK (account_reserved_cash >= 0),
  ADD COLUMN bucket_reserved_cash numeric(38,18) NOT NULL DEFAULT 0 CHECK (bucket_reserved_cash >= 0),
  ADD COLUMN target_reserved_quantity numeric(38,18) NOT NULL DEFAULT 0 CHECK (target_reserved_quantity >= 0);

CREATE TABLE capital_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  capital_bucket_id uuid NOT NULL,
  order_intent_id uuid NOT NULL UNIQUE,
  source_type text NOT NULL CHECK (source_type='ORDER_INTENT'),
  resource_type text NOT NULL CHECK (resource_type IN ('CASH','ASSET')),
  resource_asset text NOT NULL CHECK (resource_asset ~ '^[A-Z][A-Z0-9]{0,15}$'),
  quantity numeric(38,18) NOT NULL CHECK (quantity > 0),
  reserved_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK (expires_at > reserved_at),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (order_intent_id,user_id,financial_account_id,capital_bucket_id)
    REFERENCES order_intents(id,user_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT,
  CHECK ((resource_type='CASH' AND resource_asset='USD') OR (resource_type='ASSET' AND resource_asset<>'USD'))
);
CREATE INDEX capital_reservations_account_resource_expiry_idx
  ON capital_reservations(user_id,financial_account_id,resource_type,resource_asset,expires_at);
CREATE INDEX capital_reservations_bucket_resource_expiry_idx
  ON capital_reservations(user_id,financial_account_id,capital_bucket_id,resource_type,resource_asset,expires_at);

-- The trigger serializes every manual reservation for one account, verifies that
-- the immutable risk snapshot still matches all currently relevant reservations,
-- and binds the reservation to the normalized proposal terms. A concurrent
-- proposal therefore cannot silently consume the same cash or asset quantity.
-- +goose StatementBegin
CREATE FUNCTION enforce_order_intent_capital_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  intent_row order_intents%ROWTYPE;
  preview_row order_intent_previews%ROWTYPE;
  risk_observed_at timestamptz;
  risk_proposed_notional numeric(38,18);
  risk_account_reserved_cash numeric(38,18);
  risk_bucket_reserved_cash numeric(38,18);
  risk_target_reserved_quantity numeric(38,18);
  decision_value text;
  current_account_cash numeric(38,18);
  current_bucket_cash numeric(38,18);
  current_target_quantity numeric(38,18);
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text || ':' || NEW.financial_account_id::text || ':manual-order-reservations', 0));

  SELECT * INTO STRICT intent_row FROM order_intents WHERE id=NEW.order_intent_id;
  SELECT * INTO STRICT preview_row FROM order_intent_previews WHERE order_intent_id=NEW.order_intent_id AND intent_version=1;
  SELECT l.observed_at,l.proposed_notional,l.account_reserved_cash,l.bucket_reserved_cash,l.target_reserved_quantity,r.decision
    INTO STRICT risk_observed_at,risk_proposed_notional,risk_account_reserved_cash,risk_bucket_reserved_cash,risk_target_reserved_quantity,decision_value
    FROM order_intent_risk_evaluations l
    JOIN risk_evaluations r ON r.id=l.risk_evaluation_id
    WHERE l.order_intent_id=NEW.order_intent_id;

  SELECT
    COALESCE(sum(quantity) FILTER (WHERE resource_type='CASH' AND resource_asset='USD'),0),
    COALESCE(sum(quantity) FILTER (WHERE resource_type='CASH' AND resource_asset='USD' AND capital_bucket_id=NEW.capital_bucket_id),0),
    COALESCE(sum(quantity) FILTER (WHERE resource_type='ASSET' AND resource_asset=intent_row.base_asset),0)
  INTO current_account_cash,current_bucket_cash,current_target_quantity
  FROM capital_reservations
  WHERE user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id AND expires_at>risk_observed_at;

  IF intent_row.status<>'REVIEW_REQUIRED' OR decision_value<>'ALLOW' OR
     NEW.reserved_at<>risk_observed_at OR NEW.expires_at<>preview_row.expires_at OR
     current_account_cash<>risk_account_reserved_cash OR
     current_bucket_cash<>risk_bucket_reserved_cash OR
     current_target_quantity<>risk_target_reserved_quantity THEN
    RAISE EXCEPTION 'capital reservation snapshot is stale or not reviewable';
  END IF;

  IF intent_row.side='BUY' AND
     (NEW.resource_type<>'CASH' OR NEW.resource_asset<>'USD' OR NEW.quantity<>risk_proposed_notional) THEN
    RAISE EXCEPTION 'buy reservation does not match proposal notional';
  END IF;
  IF intent_row.side='SELL' AND
     (NEW.resource_type<>'ASSET' OR NEW.resource_asset<>intent_row.base_asset OR NEW.quantity<>intent_row.requested_size) THEN
    RAISE EXCEPTION 'sell reservation does not match proposal quantity';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER capital_reservation_guard
  BEFORE INSERT ON capital_reservations
  FOR EACH ROW EXECUTE FUNCTION enforce_order_intent_capital_reservation();

-- +goose StatementBegin
CREATE FUNCTION reject_capital_reservation_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'capital reservation evidence is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER capital_reservation_immutable
  BEFORE UPDATE OR DELETE ON capital_reservations
  FOR EACH ROW EXECUTE FUNCTION reject_capital_reservation_mutation();

-- New reviewable proposals must commit their reservation in the same
-- transaction; blocked proposals must not reserve anything. Existing rows are
-- left intact, but any future transition is checked.
-- +goose StatementBegin
CREATE FUNCTION enforce_reviewable_intent_has_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE reservation_exists boolean;
BEGIN
  IF NEW.capital_bucket_id IS NULL THEN
    RETURN NEW;
  END IF;
  SELECT EXISTS(SELECT 1 FROM capital_reservations WHERE order_intent_id=NEW.id) INTO reservation_exists;
  IF NEW.status='BLOCKED' AND reservation_exists THEN
    RAISE EXCEPTION 'blocked order intent cannot reserve capital';
  END IF;
  IF NEW.status<>'BLOCKED' AND NOT reservation_exists THEN
    RAISE EXCEPTION 'reviewable order intent requires a capital reservation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER reviewable_order_intent_reservation_guard
  AFTER INSERT OR UPDATE ON order_intents
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION enforce_reviewable_intent_has_reservation();

-- +goose Down
DROP TRIGGER IF EXISTS reviewable_order_intent_reservation_guard ON order_intents;
DROP FUNCTION IF EXISTS enforce_reviewable_intent_has_reservation();
DROP TRIGGER IF EXISTS capital_reservation_immutable ON capital_reservations;
DROP FUNCTION IF EXISTS reject_capital_reservation_mutation();
DROP TRIGGER IF EXISTS capital_reservation_guard ON capital_reservations;
DROP FUNCTION IF EXISTS enforce_order_intent_capital_reservation();
DROP TABLE IF EXISTS capital_reservations;
ALTER TABLE order_intent_risk_evaluations
  DROP COLUMN IF EXISTS target_reserved_quantity,
  DROP COLUMN IF EXISTS bucket_reserved_cash,
  DROP COLUMN IF EXISTS account_reserved_cash;
