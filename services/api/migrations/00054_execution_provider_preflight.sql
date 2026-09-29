-- +goose Up
-- Private server-collected evidence. No browser/model API writes this table.
CREATE TABLE execution_provider_preflights (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 order_id uuid NOT NULL, owner_id uuid NOT NULL, financial_account_id uuid NOT NULL, provider_connection_id uuid NOT NULL,
 credential_generation bigint NOT NULL CHECK(credential_generation>0),
 request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 portfolio_id uuid NOT NULL, reconciliation_id uuid NOT NULL,
 observed_at timestamptz NOT NULL, expires_at timestamptz NOT NULL CHECK(expires_at>observed_at AND expires_at<=observed_at+interval '30 seconds'),
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=8192),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
 FOREIGN KEY(reconciliation_id,owner_id,financial_account_id) REFERENCES portfolio_reconciliations(id,user_id,financial_account_id) ON DELETE RESTRICT,
 FOREIGN KEY(provider_connection_id) REFERENCES provider_connections(id) ON DELETE RESTRICT
);
CREATE INDEX execution_provider_preflights_order ON execution_provider_preflights(order_id,created_at DESC);
CREATE TRIGGER execution_provider_preflight_immutable BEFORE UPDATE OR DELETE ON execution_provider_preflights FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
-- +goose StatementBegin
CREATE FUNCTION guard_execution_provider_preflight() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation bigint; o execution_orders%ROWTYPE; account_ref text;
BEGIN
 generation=lock_execution_claim_controls(NEW.order_id);
 SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
 SELECT provider_account_id INTO STRICT account_ref FROM financial_accounts WHERE id=o.financial_account_id;
 IF NEW.credential_generation<>generation OR NEW.request_digest<>o.request_digest OR NEW.provider_connection_id<>o.provider_connection_id
 OR account_ref<>'portfolio:'||NEW.portfolio_id::text OR NEW.observed_at<o.created_at OR NEW.observed_at>clock_timestamp() OR NEW.expires_at<=clock_timestamp()
 OR NEW.evidence->>'RequestDigest' IS DISTINCT FROM NEW.request_digest OR NEW.evidence->>'PortfolioID' IS DISTINCT FROM NEW.portfolio_id::text
 OR (NEW.evidence->>'StartedAt')::timestamptz IS DISTINCT FROM NEW.observed_at
 OR EXISTS(SELECT 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id) THEN
 RAISE EXCEPTION 'execution provider evidence unavailable' USING ERRCODE='23514',CONSTRAINT='execution_current_controls';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_provider_preflight_guard BEFORE INSERT ON execution_provider_preflights FOR EACH ROW EXECUTE FUNCTION guard_execution_provider_preflight();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_provider_preflights) THEN RAISE EXCEPTION 'cannot remove execution provider evidence'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE execution_provider_preflights;
DROP FUNCTION guard_execution_provider_preflight();
