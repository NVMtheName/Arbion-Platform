-- +goose Up
-- Exact machine-code evidence only. A reported rejection is NOT settlement and
-- cannot release either account/capital holds or authorize a replacement send.
CREATE TABLE execution_submission_rejections (
  order_id uuid PRIMARY KEY,
  owner_id uuid NOT NULL,
  error_code text NOT NULL CHECK(error_code ~ '^[A-Z][A-Z0-9_]{0,127}$'),
  received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  FOREIGN KEY(order_id,owner_id) REFERENCES execution_dispatch_attempts(order_id,owner_id) ON DELETE RESTRICT
);
CREATE TRIGGER execution_rejection_immutable BEFORE UPDATE OR DELETE ON execution_submission_rejections
  FOR EACH ROW EXECUTE FUNCTION keep_execution_evidence();

-- Both receipt writers serialize on the same immutable attempt. Contradictory
-- acknowledgement/rejection evidence fails closed, including direct SQL writes.
-- +goose StatementBegin
CREATE FUNCTION guard_execution_submission_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER execution_rejection_guard BEFORE INSERT ON execution_submission_rejections
  FOR EACH ROW EXECUTE FUNCTION guard_execution_submission_receipt();
CREATE TRIGGER execution_ack_receipt_guard BEFORE INSERT ON execution_broker_acknowledgements
  FOR EACH ROW EXECUTE FUNCTION guard_execution_submission_receipt();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_submission_rejections) THEN
    RAISE EXCEPTION 'cannot remove immutable submission rejection evidence';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER execution_ack_receipt_guard ON execution_broker_acknowledgements;
DROP TABLE execution_submission_rejections;
DROP FUNCTION guard_execution_submission_receipt();
