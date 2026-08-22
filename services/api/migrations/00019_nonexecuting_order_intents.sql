-- +goose Up
CREATE TABLE order_intents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  financial_account_id uuid NOT NULL,
  source text NOT NULL CHECK (source IN ('UI','AI','HYBRID')),
  idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 128),
  request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
  provider_name text NOT NULL CHECK (provider_name = 'coinbase'),
  product_id text NOT NULL CHECK (product_id ~ '^[A-Z][A-Z0-9]{0,15}-USD$'),
  base_asset text NOT NULL CHECK (base_asset ~ '^[A-Z][A-Z0-9]{0,15}$'),
  quote_currency char(3) NOT NULL CHECK (quote_currency = 'USD'),
  side text NOT NULL CHECK (side IN ('BUY','SELL')),
  order_type text NOT NULL CHECK (order_type = 'MARKET_IOC'),
  requested_size numeric(38,18) NOT NULL CHECK (requested_size > 0),
  requested_size_currency text NOT NULL,
  status text NOT NULL CHECK (status IN ('REVIEW_REQUIRED','BLOCKED','USER_APPROVED_NONEXECUTABLE')),
  version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id,idempotency_key),
  UNIQUE (id,user_id),
  FOREIGN KEY (financial_account_id,user_id) REFERENCES financial_accounts(id,user_id) ON DELETE RESTRICT,
  CHECK (base_asset <> 'USD' AND product_id=base_asset || '-USD'),
  CHECK ((side='BUY' AND requested_size_currency='USD') OR (side='SELL' AND requested_size_currency=base_asset)),
  CHECK ((status IN ('REVIEW_REQUIRED','BLOCKED') AND version=1) OR (status='USER_APPROVED_NONEXECUTABLE' AND version=2))
);
CREATE INDEX order_intents_owner_account_time_idx ON order_intents(user_id,financial_account_id,created_at DESC,id DESC);

CREATE TABLE order_intent_previews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_intent_id uuid NOT NULL REFERENCES order_intents(id) ON DELETE RESTRICT,
  intent_version bigint NOT NULL CHECK (intent_version = 1),
  provider_name text NOT NULL CHECK (provider_name = 'coinbase'),
  feed text NOT NULL CHECK (feed = 'advanced_trade_order_preview'),
  preview_state text NOT NULL CHECK (preview_state IN ('READY','BLOCKED')),
  base_size numeric(38,18) NOT NULL CHECK (base_size >= 0),
  quote_size numeric(38,18) NOT NULL CHECK (quote_size >= 0),
  order_total numeric(38,18) NOT NULL CHECK (order_total >= 0),
  commission_total numeric(38,18) NOT NULL CHECK (commission_total >= 0),
  best_bid numeric(38,18) CHECK (best_bid > 0),
  best_ask numeric(38,18) CHECK (best_ask > 0),
  estimated_average_filled_price numeric(38,18) CHECK (estimated_average_filled_price > 0),
  slippage numeric(38,18),
  provider_trading_authorized boolean NOT NULL,
  block_reasons jsonb NOT NULL CHECK (jsonb_typeof(block_reasons) = 'array' AND jsonb_array_length(block_reasons) <= 20 AND block_reasons <@ '["ACCOUNT_RESTRICTED","INSUFFICIENT_FUNDS","INVALID_SIZE","MARKET_UNAVAILABLE","PROVIDER_REJECTED","PROVIDER_TRADE_PERMISSION_REQUIRED"]'::jsonb),
  warnings jsonb NOT NULL CHECK (jsonb_typeof(warnings) = 'array' AND jsonb_array_length(warnings) <= 20 AND warnings <@ '["LARGE_ORDER","PROVIDER_WARNING","SMALL_ORDER"]'::jsonb),
  evidence_hash bytea NOT NULL CHECK (octet_length(evidence_hash) = 32),
  previewed_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK (expires_at > previewed_at),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (order_intent_id,intent_version),
  CHECK (expires_at = previewed_at + interval '1 minute'),
  CHECK ((preview_state='READY' AND order_total > 0 AND (base_size > 0 OR quote_size > 0)) OR (preview_state='BLOCKED' AND jsonb_array_length(block_reasons) > 0)),
  CHECK (preview_state='BLOCKED' OR jsonb_array_length(block_reasons)=0 OR (provider_trading_authorized=false AND block_reasons='["PROVIDER_TRADE_PERMISSION_REQUIRED"]'::jsonb))
);

CREATE TABLE order_intent_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_intent_id uuid NOT NULL,
  intent_version bigint NOT NULL CHECK (intent_version = 1),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  decision text NOT NULL CHECK (decision = 'APPROVE'),
  approval_scope text NOT NULL CHECK (approval_scope = 'PROPOSAL_REVIEW_ONLY'),
  mfa_method text NOT NULL CHECK (mfa_method = 'totp'),
  evidence_hash bytea NOT NULL CHECK (octet_length(evidence_hash) = 32),
  reviewed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (order_intent_id,intent_version),
  FOREIGN KEY (order_intent_id,user_id) REFERENCES order_intents(id,user_id) ON DELETE RESTRICT
);

CREATE TABLE order_intent_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_intent_id uuid NOT NULL REFERENCES order_intents(id) ON DELETE RESTRICT,
  sequence_number bigint NOT NULL CHECK (sequence_number > 0),
  event_type text NOT NULL CHECK (event_type IN ('PROPOSED','BLOCKED','USER_REVIEWED_NONEXECUTABLE')),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  occurred_at timestamptz NOT NULL,
  UNIQUE (order_intent_id,sequence_number)
);

-- +goose StatementBegin
CREATE FUNCTION reject_order_intent_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'order intent evidence is immutable';
END $$;
-- +goose StatementEnd

CREATE TRIGGER order_intent_preview_immutable BEFORE UPDATE OR DELETE ON order_intent_previews FOR EACH ROW EXECUTE FUNCTION reject_order_intent_evidence_mutation();
CREATE TRIGGER order_intent_review_immutable BEFORE UPDATE OR DELETE ON order_intent_reviews FOR EACH ROW EXECUTE FUNCTION reject_order_intent_evidence_mutation();
CREATE TRIGGER order_intent_event_immutable BEFORE UPDATE OR DELETE ON order_intent_events FOR EACH ROW EXECUTE FUNCTION reject_order_intent_evidence_mutation();

-- +goose StatementBegin
CREATE FUNCTION enforce_order_intent_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'order intents cannot be deleted';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF NEW.status NOT IN ('REVIEW_REQUIRED','BLOCKED') OR NEW.version <> 1 THEN
      RAISE EXCEPTION 'order intents must begin non-approved';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.user_id <> OLD.user_id OR NEW.financial_account_id <> OLD.financial_account_id OR
     NEW.source <> OLD.source OR NEW.idempotency_key <> OLD.idempotency_key OR
     NEW.request_hash <> OLD.request_hash OR NEW.provider_name <> OLD.provider_name OR
     NEW.product_id <> OLD.product_id OR NEW.base_asset <> OLD.base_asset OR
     NEW.quote_currency <> OLD.quote_currency OR NEW.side <> OLD.side OR
     NEW.order_type <> OLD.order_type OR NEW.requested_size <> OLD.requested_size OR
     NEW.requested_size_currency <> OLD.requested_size_currency OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'order intent proposal is immutable';
  END IF;
  IF OLD.status <> 'REVIEW_REQUIRED' OR NEW.status <> 'USER_APPROVED_NONEXECUTABLE' OR NEW.version <> OLD.version + 1 OR NEW.updated_at <= OLD.updated_at THEN
    RAISE EXCEPTION 'invalid order intent transition';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER order_intent_transition_guard BEFORE INSERT OR UPDATE OR DELETE ON order_intents FOR EACH ROW EXECUTE FUNCTION enforce_order_intent_transition();

-- +goose Down
DROP TRIGGER IF EXISTS order_intent_transition_guard ON order_intents;
DROP FUNCTION IF EXISTS enforce_order_intent_transition();
DROP TRIGGER IF EXISTS order_intent_event_immutable ON order_intent_events;
DROP TRIGGER IF EXISTS order_intent_review_immutable ON order_intent_reviews;
DROP TRIGGER IF EXISTS order_intent_preview_immutable ON order_intent_previews;
DROP FUNCTION IF EXISTS reject_order_intent_evidence_mutation();
DROP TABLE IF EXISTS order_intent_events;
DROP TABLE IF EXISTS order_intent_reviews;
DROP TABLE IF EXISTS order_intent_previews;
DROP TABLE IF EXISTS order_intents;
