-- +goose Up
ALTER TABLE capital_buckets
  ADD CONSTRAINT capital_buckets_id_user_account_unique
  UNIQUE (id,user_id,financial_account_id);

ALTER TABLE order_intents ADD COLUMN capital_bucket_id uuid;
ALTER TABLE order_intents
  ADD CONSTRAINT order_intents_capital_bucket_owner_account_fkey
  FOREIGN KEY (capital_bucket_id,user_id,financial_account_id)
  REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT;
ALTER TABLE order_intents
  ADD CONSTRAINT order_intents_id_owner_account_capital_bucket_unique
  UNIQUE (id,user_id,financial_account_id,capital_bucket_id);

ALTER TABLE risk_evaluations
  ADD CONSTRAINT risk_evaluations_id_owner_account_unique
  UNIQUE (id,user_id,financial_account_id);

ALTER TABLE risk_evaluations
  ADD COLUMN warnings jsonb NOT NULL DEFAULT '[]'::jsonb
  CHECK (jsonb_typeof(warnings)='array' AND jsonb_array_length(warnings) <= 20);

CREATE TABLE order_intent_risk_evaluations (
  order_intent_id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  capital_bucket_id uuid NOT NULL,
  risk_evaluation_id uuid NOT NULL UNIQUE,
  policy_version text NOT NULL CHECK (policy_version='manual_coinbase_spot.v1'),
  capital_bucket_name text NOT NULL CHECK (length(capital_bucket_name) BETWEEN 1 AND 100),
  allocation_type text NOT NULL CHECK (allocation_type IN ('FIXED_AMOUNT','PERCENT_OF_AVAILABLE_CASH','PERCENT_OF_BUYING_POWER')),
  allocation_value numeric(30,10) NOT NULL CHECK (allocation_value > 0),
  protected_amount numeric(30,10) NOT NULL CHECK (protected_amount >= 0),
  allocation_limit numeric(30,10) CHECK (allocation_limit > 0),
  account_available_cash numeric(38,18) NOT NULL CHECK (account_available_cash >= 0),
  target_available_quantity numeric(38,18) NOT NULL CHECK (target_available_quantity >= 0),
  proposed_notional numeric(38,18) NOT NULL CHECK (proposed_notional > 0),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (order_intent_id,user_id,financial_account_id,capital_bucket_id)
    REFERENCES order_intents(id,user_id,financial_account_id,capital_bucket_id) ON DELETE RESTRICT,
  FOREIGN KEY (risk_evaluation_id,user_id,financial_account_id)
    REFERENCES risk_evaluations(id,user_id,financial_account_id) ON DELETE RESTRICT
);

ALTER TABLE order_intent_previews
  DROP CONSTRAINT order_intent_previews_block_reasons_check;
ALTER TABLE order_intent_previews
  ADD CONSTRAINT order_intent_previews_block_reasons_check CHECK (
    jsonb_typeof(block_reasons)='array' AND jsonb_array_length(block_reasons) <= 20 AND
    block_reasons <@ '["ACCOUNT_RESTRICTED","INSUFFICIENT_FUNDS","INVALID_SIZE","MARKET_UNAVAILABLE","PRODUCT_AUCTION_MODE","PRODUCT_CANCEL_ONLY","PRODUCT_DISABLED","PRODUCT_LIMIT_ONLY","PRODUCT_POST_ONLY","PROVIDER_REJECTED","PROVIDER_TRADE_PERMISSION_REQUIRED","SIZE_ABOVE_MAXIMUM","SIZE_BELOW_MINIMUM","SIZE_INCREMENT_MISMATCH"]'::jsonb
  );

-- +goose StatementBegin
CREATE FUNCTION reject_order_intent_risk_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'order intent risk evidence is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER risk_evaluations_immutable
  BEFORE UPDATE OR DELETE ON risk_evaluations
  FOR EACH ROW EXECUTE FUNCTION reject_order_intent_risk_mutation();
CREATE TRIGGER order_intent_risk_evaluations_immutable
  BEFORE UPDATE OR DELETE ON order_intent_risk_evaluations
  FOR EACH ROW EXECUTE FUNCTION reject_order_intent_risk_mutation();

-- +goose StatementBegin
CREATE FUNCTION enforce_order_intent_capital_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' AND NEW.capital_bucket_id IS NULL THEN
    RAISE EXCEPTION 'new order intent requires a capital policy';
  END IF;
  IF TG_OP='UPDATE' AND NEW.capital_bucket_id IS DISTINCT FROM OLD.capital_bucket_id THEN
    RAISE EXCEPTION 'order intent capital policy is immutable';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER order_intent_capital_policy_guard
  BEFORE INSERT OR UPDATE ON order_intents
  FOR EACH ROW EXECUTE FUNCTION enforce_order_intent_capital_policy();

-- +goose Down
DROP TRIGGER IF EXISTS order_intent_capital_policy_guard ON order_intents;
DROP FUNCTION IF EXISTS enforce_order_intent_capital_policy();
DROP TRIGGER IF EXISTS order_intent_risk_evaluations_immutable ON order_intent_risk_evaluations;
DROP TRIGGER IF EXISTS risk_evaluations_immutable ON risk_evaluations;
DROP FUNCTION IF EXISTS reject_order_intent_risk_mutation();
DROP TABLE IF EXISTS order_intent_risk_evaluations;
ALTER TABLE order_intent_previews DROP CONSTRAINT IF EXISTS order_intent_previews_block_reasons_check;
ALTER TABLE order_intent_previews ADD CONSTRAINT order_intent_previews_block_reasons_check CHECK (jsonb_typeof(block_reasons)='array' AND jsonb_array_length(block_reasons) <= 20 AND block_reasons <@ '["ACCOUNT_RESTRICTED","INSUFFICIENT_FUNDS","INVALID_SIZE","MARKET_UNAVAILABLE","PROVIDER_REJECTED","PROVIDER_TRADE_PERMISSION_REQUIRED"]'::jsonb);
ALTER TABLE risk_evaluations DROP COLUMN IF EXISTS warnings;
ALTER TABLE risk_evaluations DROP CONSTRAINT IF EXISTS risk_evaluations_id_owner_account_unique;
ALTER TABLE order_intents DROP CONSTRAINT IF EXISTS order_intents_id_owner_account_capital_bucket_unique;
ALTER TABLE order_intents DROP CONSTRAINT IF EXISTS order_intents_capital_bucket_owner_account_fkey;
ALTER TABLE order_intents DROP COLUMN IF EXISTS capital_bucket_id;
ALTER TABLE capital_buckets DROP CONSTRAINT IF EXISTS capital_buckets_id_user_account_unique;
