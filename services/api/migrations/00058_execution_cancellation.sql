-- +goose Up
-- One durable, bounded cancellation attempt per known original broker order.
-- An outcome is operation evidence only: it cannot settle or release anything.
-- This migration adds no runtime caller or provider writer.
CREATE TABLE execution_cancellation_attempts (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  provider_connection_id uuid NOT NULL,
  provider_order_id uuid NOT NULL,
  portfolio_id uuid NOT NULL,
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  claimed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  expires_at timestamptz NOT NULL DEFAULT (clock_timestamp()+interval '5 seconds'),
  CHECK(expires_at=claimed_at+interval '5 seconds'),
  UNIQUE(order_id,owner_id,provider_order_id),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id,provider_order_id) REFERENCES execution_broker_acknowledgements(order_id,owner_id,provider_order_id) ON DELETE RESTRICT,
  FOREIGN KEY(provider_connection_id) REFERENCES provider_connections(id) ON DELETE RESTRICT
);
CREATE TABLE execution_cancellation_receipts (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  provider_order_id uuid NOT NULL,
  outcome text NOT NULL CHECK(outcome IN ('ACCEPTED','NOT_ACCEPTED','UNKNOWN')),
  received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(order_id,owner_id,provider_order_id) REFERENCES execution_cancellation_attempts(order_id,owner_id,provider_order_id) ON DELETE RESTRICT
);
CREATE TRIGGER execution_cancellation_attempt_immutable BEFORE UPDATE OR DELETE ON execution_cancellation_attempts
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();
CREATE TRIGGER execution_cancellation_receipt_immutable BEFORE UPDATE OR DELETE ON execution_cancellation_receipts
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Reuse owner -> entitlement -> manual fence -> account -> connection ->
-- strategy fence -> capital fence, with READ COMMITTED current snapshots.
-- Stop state, expired/revoked original approval, disabled allocation policy and
-- reconciliation quarantine do not prevent this bounded risk-reducing request.
-- The current owner/account/connection/key and original portfolio still must fit.
-- +goose StatementBegin
CREATE FUNCTION lock_execution_cancellation_controls(target uuid, portfolio uuid) RETURNS bigint LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; generation bigint;
BEGIN
  generation=lock_execution_settlement_controls(target,portfolio);
  SELECT * INTO STRICT o FROM execution_orders WHERE id=target;
  IF NOT EXISTS(
      SELECT 1 FROM execution_dispatch_attempts a
      JOIN execution_broker_acknowledgements b ON b.order_id=a.order_id AND b.owner_id=a.owner_id
      JOIN execution_authorizations z ON z.id=a.authorization_id AND z.order_id=a.order_id AND z.owner_id=a.owner_id
        AND z.financial_account_id=a.financial_account_id AND z.credential_generation=a.credential_generation
      JOIN execution_provider_preflights p ON p.id::text=(z.preflight->>'EvidenceID') AND p.order_id=a.order_id
        AND p.owner_id=a.owner_id AND p.financial_account_id=a.financial_account_id
        AND p.credential_generation=a.credential_generation AND p.request_digest=z.request_digest
      WHERE a.order_id=o.id AND a.owner_id=o.owner_id AND a.financial_account_id=o.financial_account_id
        AND p.provider_connection_id=o.provider_connection_id AND p.request_digest=o.request_digest AND p.portfolio_id=portfolio)
    OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_account_settlements WHERE order_id=o.id)
    OR NOT EXISTS(SELECT 1 FROM execution_account_holds WHERE order_id=o.id AND owner_id=o.owner_id
      AND financial_account_id=o.financial_account_id)
    OR NOT EXISTS(SELECT 1 FROM execution_capital_reservations WHERE order_id=o.id AND owner_id=o.owner_id
      AND financial_account_id=o.financial_account_id AND capital_bucket_id=o.capital_bucket_id AND released_at IS NULL) THEN
    RAISE EXCEPTION 'exact outstanding cancellation target unavailable' USING ERRCODE='23514',CONSTRAINT='execution_cancellation_controls';
  END IF;
  RETURN generation;
END $$;
-- +goose StatementEnd

-- Database time owns this nonrenewable deadline. A duplicate never changes the
-- original attempt or creates a retry entitlement, even after expiry/uncertainty.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_cancellation_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; b execution_broker_acknowledgements%ROWTYPE; generation bigint;
BEGIN
  generation=lock_execution_cancellation_controls(NEW.order_id,NEW.portfolio_id);
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  SELECT * INTO STRICT b FROM execution_broker_acknowledgements WHERE order_id=o.id AND owner_id=o.owner_id;
  IF NEW.owner_id IS DISTINCT FROM o.owner_id OR NEW.financial_account_id IS DISTINCT FROM o.financial_account_id
    OR NEW.provider_connection_id IS DISTINCT FROM o.provider_connection_id OR NEW.provider_order_id IS DISTINCT FROM b.provider_order_id
    OR NEW.credential_generation IS DISTINCT FROM generation THEN
    RAISE EXCEPTION 'cancellation attempt binding mismatch' USING ERRCODE='23514',CONSTRAINT='execution_cancellation_controls';
  END IF;
  NEW.claimed_at=clock_timestamp();
  NEW.expires_at=NEW.claimed_at+interval '5 seconds';
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_cancellation_attempt_guard BEFORE INSERT ON execution_cancellation_attempts
  FOR EACH ROW EXECUTE FUNCTION guard_execution_cancellation_attempt();

-- Default-deferred control-plane commits cannot extend a claim by holding the
-- transaction open. Callers must still recheck immediately before the provider
-- call; a committed attempt alone is not current cancellation authority.
-- This is not a privilege boundary against SQL callers that force IMMEDIATE.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_cancellation_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF lock_execution_cancellation_controls(NEW.order_id,NEW.portfolio_id) IS DISTINCT FROM NEW.credential_generation
    OR NEW.expires_at<=clock_timestamp() THEN
    RAISE EXCEPTION 'cancellation claim is stale at commit' USING ERRCODE='23514',CONSTRAINT='execution_cancellation_controls';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER execution_cancellation_attempt_commit AFTER INSERT ON execution_cancellation_attempts
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_execution_cancellation_commit();

-- Historical operation evidence remains writable after permission/deadline
-- loss or terminal reconciliation. It grants no permission and changes no hold,
-- fill, terminal, account settlement, or original attempt.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_cancellation_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a execution_cancellation_attempts%ROWTYPE;
BEGIN
  SELECT * INTO a FROM execution_cancellation_attempts WHERE order_id=NEW.order_id;
  IF NOT FOUND OR NEW.owner_id IS DISTINCT FROM a.owner_id OR NEW.provider_order_id IS DISTINCT FROM a.provider_order_id
    OR NEW.received_at IS NULL OR NEW.received_at<a.claimed_at OR NEW.received_at>clock_timestamp() THEN
    RAISE EXCEPTION 'cancellation receipt binding mismatch' USING ERRCODE='23514',CONSTRAINT='execution_cancellation_receipt';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_cancellation_receipt_guard BEFORE INSERT ON execution_cancellation_receipts
  FOR EACH ROW EXECUTE FUNCTION guard_execution_cancellation_receipt();

-- +goose Down
-- Never erase an attempted provider operation or its uncertainty on rollback.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_cancellation_attempts) OR EXISTS(SELECT 1 FROM execution_cancellation_receipts) THEN
    RAISE EXCEPTION 'cannot remove immutable cancellation history';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE execution_cancellation_receipts;
DROP TABLE execution_cancellation_attempts;
DROP FUNCTION guard_execution_cancellation_receipt();
DROP FUNCTION guard_execution_cancellation_commit();
DROP FUNCTION guard_execution_cancellation_attempt();
DROP FUNCTION lock_execution_cancellation_controls(uuid,uuid);
