-- +goose Up
-- Financial reconnect/refresh uses the common encrypted vault, not the AI
-- staged-rotation writer. Bind execution evidence to changes of the effective
-- stored credential without reading, hashing, or exposing credential material.
-- Explicit forward-only invalidation remains available to control-plane writers.
-- +goose StatementBegin
CREATE FUNCTION advance_financial_credential_generation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.provider_category<>'financial' AND NEW.provider_category<>'financial' THEN
    RETURN NEW;
  END IF;
  IF NEW.credential_generation<OLD.credential_generation THEN
    RAISE EXCEPTION 'financial credential generation cannot decrease'
      USING ERRCODE='23514',CONSTRAINT='financial_credential_generation_monotonic';
  END IF;
  IF NEW.encrypted_credential_payload IS DISTINCT FROM OLD.encrypted_credential_payload
    OR NEW.credential_reference IS DISTINCT FROM OLD.credential_reference
    OR NEW.provider_category IS DISTINCT FROM OLD.provider_category THEN
    -- One effective credential change is one generation, even when a writer
    -- also supplied an increment. Staging and unrelated metadata do not rotate.
    NEW.credential_generation=OLD.credential_generation+1;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER financial_credential_generation_guard BEFORE UPDATE ON provider_connections
  FOR EACH ROW EXECUTE FUNCTION advance_financial_credential_generation();

-- +goose Down
-- Removing this guard while execution evidence exists could make a financial
-- reconnect reuse a generation already bound to a durable claim.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_dispatch_attempts) THEN
    RAISE EXCEPTION 'cannot remove financial credential generation protection after execution claims';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER financial_credential_generation_guard ON provider_connections;
DROP FUNCTION advance_financial_credential_generation();
