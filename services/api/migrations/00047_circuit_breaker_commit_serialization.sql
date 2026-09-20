-- +goose Up
-- One stable namespace shared by breaker mutations and non-live commits.
-- Scope IDs are UUIDs so alternative textual UUID spellings cannot evade a lock.
-- +goose StatementBegin
CREATE FUNCTION risk_breaker_scope_lock_key(breaker_scope text, breaker_scope_id uuid)
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$
  SELECT hashtextextended('arbion:risk-breaker:1:' || breaker_scope || ':' || COALESCE(breaker_scope_id::text,''),0)
$$;
-- +goose StatementEnd

-- Lock the scope even when there is not yet an OPEN row. Locking only existing
-- rows cannot serialize the first stop against a concurrent fill transaction.
-- This trigger covers all current application writers and direct SQL mutations.
-- +goose StatementBegin
CREATE FUNCTION serialize_risk_breaker_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' THEN
    IF NEW.scope IS DISTINCT FROM OLD.scope OR NEW.scope_id IS DISTINCT FROM OLD.scope_id THEN
      RAISE EXCEPTION 'circuit breaker scope identity is immutable'
        USING ERRCODE='23514', CONSTRAINT='risk_breaker_scope_identity_guard';
    END IF;
  END IF;
  IF TG_OP='INSERT' THEN
    PERFORM pg_advisory_xact_lock(risk_breaker_scope_lock_key(NEW.scope,NEW.scope_id));
    RETURN NEW;
  END IF;
  PERFORM pg_advisory_xact_lock(risk_breaker_scope_lock_key(OLD.scope,OLD.scope_id));
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER risk_breaker_mutation_serialization
  BEFORE INSERT OR UPDATE OR DELETE ON risk_circuit_breakers
  FOR EACH ROW EXECUTE FUNCTION serialize_risk_breaker_mutation();

-- +goose Down
-- Application releases using this guard must be rolled back before this schema.
DROP TRIGGER risk_breaker_mutation_serialization ON risk_circuit_breakers;
DROP FUNCTION serialize_risk_breaker_mutation();
DROP FUNCTION risk_breaker_scope_lock_key(text,uuid);
