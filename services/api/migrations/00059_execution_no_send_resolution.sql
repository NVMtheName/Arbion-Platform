-- +goose Up
-- A private fresh-claim coordinator may record this fact only after its
-- synchronous post-claim helper exits without entering the sender and finishes
-- transaction cleanup. Missing broker history is never proof of no send.
-- SQL binds scope, mutual exclusion and release; it cannot independently prove
-- that trusted application code did not call a provider. This is not a public
-- unlock command or an adversarial-SQL privilege boundary.
CREATE TABLE execution_no_send_resolutions (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  capital_bucket_id uuid NOT NULL,
  authorization_id uuid NOT NULL REFERENCES execution_authorizations(id) ON DELETE RESTRICT,
  request_digest text NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
  credential_generation bigint NOT NULL CHECK(credential_generation>0),
  claimed_at timestamptz NOT NULL,
  recorded_at timestamptz NOT NULL DEFAULT clock_timestamp() CHECK(recorded_at>=claimed_at),
  evidence_kind text NOT NULL DEFAULT 'SENDER_NOT_ENTERED' CHECK(evidence_kind='SENDER_NOT_ENTERED'),
  FOREIGN KEY(order_id,owner_id,financial_account_id) REFERENCES execution_orders(id,owner_id,financial_account_id) ON DELETE RESTRICT,
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id) ON DELETE RESTRICT,
  FOREIGN KEY(capital_bucket_id,owner_id,financial_account_id) REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT
);
CREATE TRIGGER execution_no_send_resolution_immutable BEFORE UPDATE OR DELETE ON execution_no_send_resolutions
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Historical no-callback evidence needs no renewed access, approval or breaker
-- permission. Keep the existing capital lock order, then serialize with both
-- acknowledgement/rejection writers on the original immutable dispatch attempt.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_no_send_resolution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o execution_orders%ROWTYPE; a execution_dispatch_attempts%ROWTYPE; z execution_authorizations%ROWTYPE;
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'no-send resolution requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_no_send_evidence';
  END IF;
  SELECT * INTO STRICT o FROM execution_orders WHERE id=NEW.order_id;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text||':'||o.financial_account_id::text||':manual-order-reservations',0));
  PERFORM 1 FROM financial_accounts WHERE id=o.financial_account_id AND user_id=o.owner_id FOR UPDATE;
  PERFORM pg_advisory_xact_lock(hashtextextended(o.owner_id::text||':'||o.financial_account_id::text||':strategy-capital-reservations',0));
  PERFORM lock_execution_capital_fence(o.financial_account_id);
  SELECT * INTO STRICT a FROM execution_dispatch_attempts WHERE order_id=o.id AND owner_id=o.owner_id FOR UPDATE;
  SELECT * INTO STRICT z FROM execution_authorizations WHERE id=a.authorization_id;
  NEW.recorded_at=clock_timestamp();
  IF NEW.owner_id IS DISTINCT FROM o.owner_id OR NEW.financial_account_id IS DISTINCT FROM o.financial_account_id
    OR NEW.capital_bucket_id IS DISTINCT FROM o.capital_bucket_id OR NEW.authorization_id IS DISTINCT FROM a.authorization_id
    OR NEW.request_digest IS DISTINCT FROM o.request_digest OR NEW.credential_generation IS DISTINCT FROM a.credential_generation
    OR NEW.claimed_at IS DISTINCT FROM a.claimed_at OR a.financial_account_id IS DISTINCT FROM o.financial_account_id
    OR z.order_id IS DISTINCT FROM o.id OR z.owner_id IS DISTINCT FROM o.owner_id OR z.financial_account_id IS DISTINCT FROM o.financial_account_id
    OR z.request_digest IS DISTINCT FROM o.request_digest OR z.credential_generation IS DISTINCT FROM a.credential_generation
    OR z.expires_at IS DISTINCT FROM a.expires_at OR z.checked_at>a.claimed_at
    OR NOT EXISTS(SELECT 1 FROM execution_account_holds WHERE order_id=o.id AND owner_id=o.owner_id AND financial_account_id=o.financial_account_id)
    OR NOT EXISTS(SELECT 1 FROM execution_capital_reservations WHERE order_id=o.id AND owner_id=o.owner_id
      AND financial_account_id=o.financial_account_id AND capital_bucket_id=o.capital_bucket_id AND reserved_at=a.claimed_at AND released_at IS NULL)
    OR EXISTS(SELECT 1 FROM execution_broker_acknowledgements WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_submission_rejections WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_fills WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_cancellation_attempts WHERE order_id=o.id)
    OR EXISTS(SELECT 1 FROM execution_account_settlements WHERE order_id=o.id) THEN
    RAISE EXCEPTION 'no-send resolution lacks exact unobserved claim' USING ERRCODE='23514',CONSTRAINT='execution_no_send_evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_no_send_resolution_guard BEFORE INSERT ON execution_no_send_resolutions
  FOR EACH ROW EXECUTE FUNCTION guard_execution_no_send_resolution();

-- Reciprocal exclusion is serialized on the same original attempt. Fills,
-- terminals, cancellation attempts and account settlements require an exact
-- acknowledgement FK, so none can be introduced after this closure either.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_submission_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'submission receipt requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  PERFORM 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id AND owner_id=NEW.owner_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'submission receipt scope mismatch' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  IF EXISTS(SELECT 1 FROM execution_no_send_resolutions WHERE order_id=NEW.order_id)
    OR (TG_TABLE_NAME='execution_submission_rejections' AND EXISTS(SELECT 1 FROM execution_broker_acknowledgements WHERE order_id=NEW.order_id))
    OR (TG_TABLE_NAME='execution_broker_acknowledgements' AND EXISTS(SELECT 1 FROM execution_submission_rejections WHERE order_id=NEW.order_id)) THEN
    RAISE EXCEPTION 'contradictory submission receipt' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Preserve the terminal path verbatim. Only an exact no-send receipt adds a
-- second deletion basis, which does not erase or bypass account quarantine.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_account_hold() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'execution account hold cannot be rewritten' USING ERRCODE='23514'; END IF;
  IF TG_OP='DELETE' THEN
    PERFORM 1 FROM financial_accounts WHERE id=OLD.financial_account_id FOR UPDATE;
    IF EXISTS(SELECT 1 FROM execution_no_send_resolutions WHERE order_id=OLD.order_id AND owner_id=OLD.owner_id AND financial_account_id=OLD.financial_account_id) THEN
      RETURN OLD;
    END IF;
    IF NOT EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=OLD.order_id AND owner_id=OLD.owner_id AND financial_account_id=OLD.financial_account_id)
      OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=OLD.financial_account_id) THEN
      RAISE EXCEPTION 'execution hold requires exact terminal reconciliation' USING ERRCODE='23514';
    END IF;
    RETURN OLD;
  END IF;
  PERFORM 1 FROM financial_accounts WHERE id=NEW.financial_account_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM execution_no_send_resolutions WHERE order_id=NEW.order_id)
    OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=NEW.order_id)
    OR EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=NEW.financial_account_id) THEN
    RAISE EXCEPTION 'execution hold cannot reopen a closed or blocked account' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Both release bases are immutable and exact. Reservation identity, amount and
-- original time remain untouched; the only permitted change is one-way release.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_capital_release() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.released_at IS NOT NULL THEN RAISE EXCEPTION 'capital cannot be born released' USING ERRCODE='23514'; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'capital history is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.released_at IS NOT NULL OR NEW.released_at IS NULL
    OR (to_jsonb(NEW)-'released_at') IS DISTINCT FROM (to_jsonb(OLD)-'released_at')
    OR NOT (
      EXISTS(SELECT 1 FROM execution_account_settlements s WHERE s.order_id=OLD.order_id
        AND s.owner_id=OLD.owner_id AND s.financial_account_id=OLD.financial_account_id
        AND s.capital_bucket_id=OLD.capital_bucket_id AND s.recorded_at=NEW.released_at)
      OR EXISTS(SELECT 1 FROM execution_no_send_resolutions n WHERE n.order_id=OLD.order_id
        AND n.owner_id=OLD.owner_id AND n.financial_account_id=OLD.financial_account_id
        AND n.capital_bucket_id=OLD.capital_bucket_id AND n.claimed_at=OLD.reserved_at AND n.recorded_at=NEW.released_at)) THEN
    RAISE EXCEPTION 'capital release requires exact immutable resolution' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION release_execution_no_send_holds() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM execution_account_holds WHERE order_id=NEW.order_id AND owner_id=NEW.owner_id AND financial_account_id=NEW.financial_account_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'no-send resolution lost original account hold' USING ERRCODE='23514'; END IF;
  UPDATE execution_capital_reservations SET released_at=NEW.recorded_at WHERE order_id=NEW.order_id AND owner_id=NEW.owner_id
    AND financial_account_id=NEW.financial_account_id AND capital_bucket_id=NEW.capital_bucket_id AND reserved_at=NEW.claimed_at AND released_at IS NULL;
  IF NOT FOUND THEN RAISE EXCEPTION 'no-send resolution lost original capital hold' USING ERRCODE='23514'; END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER execution_no_send_resolution_release AFTER INSERT ON execution_no_send_resolutions
  FOR EACH ROW EXECUTE FUNCTION release_execution_no_send_holds();

-- +goose Down
-- Never erase the evidence supporting a release or recreate consumed authority.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_no_send_resolutions) THEN
    RAISE EXCEPTION 'cannot remove immutable no-send resolution history';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_submission_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('transaction_isolation')<>'read committed' THEN
    RAISE EXCEPTION 'submission receipt requires current snapshots' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  PERFORM 1 FROM execution_dispatch_attempts WHERE order_id=NEW.order_id AND owner_id=NEW.owner_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'submission receipt scope mismatch' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  IF (TG_TABLE_NAME='execution_submission_rejections' AND EXISTS(SELECT 1 FROM execution_broker_acknowledgements WHERE order_id=NEW.order_id))
    OR (TG_TABLE_NAME='execution_broker_acknowledgements' AND EXISTS(SELECT 1 FROM execution_submission_rejections WHERE order_id=NEW.order_id)) THEN
    RAISE EXCEPTION 'contradictory submission receipt' USING ERRCODE='23514',CONSTRAINT='execution_receipt_conflict';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_account_hold() RETURNS trigger LANGUAGE plpgsql AS $$
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_capital_release() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.released_at IS NOT NULL THEN RAISE EXCEPTION 'capital cannot be born released' USING ERRCODE='23514'; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='DELETE' THEN RAISE EXCEPTION 'capital history is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.released_at IS NOT NULL OR NEW.released_at IS NULL
    OR (to_jsonb(NEW)-'released_at') IS DISTINCT FROM (to_jsonb(OLD)-'released_at')
    OR NOT EXISTS(SELECT 1 FROM execution_account_settlements s WHERE s.order_id=OLD.order_id
      AND s.owner_id=OLD.owner_id AND s.financial_account_id=OLD.financial_account_id
      AND s.capital_bucket_id=OLD.capital_bucket_id AND s.recorded_at=NEW.released_at) THEN
    RAISE EXCEPTION 'capital release requires exact immutable settlement' USING ERRCODE='23514',CONSTRAINT='execution_settlement_evidence';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
DROP TABLE execution_no_send_resolutions;
DROP FUNCTION release_execution_no_send_holds();
DROP FUNCTION guard_execution_no_send_resolution();
