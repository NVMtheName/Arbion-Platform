-- +goose Up
-- Inert durable submission foundation. No production caller or broker writer.
-- Existing non-executable order_intents/reviews cannot become live approvals.
CREATE TABLE execution_orders (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL REFERENCES provider_connections(id) ON DELETE RESTRICT,
  capital_bucket_id uuid NOT NULL,
  client_order_id uuid NOT NULL UNIQUE CHECK(client_order_id<>'00000000-0000-0000-0000-000000000000'),
  request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
  request jsonb NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=4096),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE(id,owner_id),
  UNIQUE(id,owner_id,financial_account_id),
  FOREIGN KEY(financial_account_id,owner_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT
);

CREATE TABLE execution_dispatch_attempts (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  -- Intentionally one unresolved attempt per account. There is no automatic
  -- expiry, lease reclaim or release path. Terminal settlement is a later gate.
  financial_account_id uuid NOT NULL UNIQUE,
  authorization_id uuid NOT NULL UNIQUE CHECK(authorization_id<>'00000000-0000-0000-0000-000000000000'),
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  claimed_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK(expires_at>claimed_at AND expires_at<=claimed_at+interval '1 minute'),
  UNIQUE(order_id,owner_id),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT
);

CREATE TABLE execution_broker_acknowledgements (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  provider_order_id uuid NOT NULL UNIQUE CHECK(provider_order_id<>'00000000-0000-0000-0000-000000000000'),
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id) ON DELETE RESTRICT
);

-- +goose StatementBegin
CREATE FUNCTION guard_execution_order() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'execution evidence is immutable' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM financial_accounts a JOIN provider_connections c ON c.id=a.provider_connection_id
    WHERE a.id=NEW.financial_account_id AND a.user_id=NEW.owner_id AND a.provider_connection_id=NEW.provider_connection_id
      AND a.provider_name='coinbase' AND c.user_id=NEW.owner_id AND c.provider_category='financial' AND c.provider_name='coinbase') THEN
    RAISE EXCEPTION 'execution account binding mismatch' USING ERRCODE='23514';
  END IF;
  IF NOT (NEW.request ?& ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD'])
    OR NEW.request - ARRAY['OwnerID','AccountID','ConnectionID','CapitalBucketID','ClientOrderID','ProductID','Side','BaseSize','LimitPrice','FeeAllowanceUSD','MaximumDebitUSD'] <> '{}'::jsonb
    OR NEW.request->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.request->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.request->>'ConnectionID' IS DISTINCT FROM NEW.provider_connection_id::text
    OR NEW.request->>'CapitalBucketID' IS DISTINCT FROM NEW.capital_bucket_id::text
    OR NEW.request->>'ClientOrderID' IS DISTINCT FROM NEW.client_order_id::text THEN
    RAISE EXCEPTION 'execution request binding mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_order_guard BEFORE INSERT OR UPDATE OR DELETE ON execution_orders FOR EACH ROW EXECUTE FUNCTION guard_execution_order();

-- +goose StatementBegin
CREATE FUNCTION keep_execution_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'execution evidence is immutable' USING ERRCODE='23514';
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_attempt_immutable BEFORE UPDATE OR DELETE ON execution_dispatch_attempts FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_ack_immutable BEFORE UPDATE OR DELETE ON execution_broker_acknowledgements FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- +goose Down
-- Never erase submission history or uncertainty during a rollback.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_orders) THEN
    RAISE EXCEPTION 'cannot remove durable execution evidence';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE execution_broker_acknowledgements;
DROP TABLE execution_dispatch_attempts;
DROP TABLE execution_orders;
DROP FUNCTION keep_execution_evidence();
DROP FUNCTION guard_execution_order();
