-- +goose Up
-- Migration 42 is already deployed. The dormant registry is intentionally
-- empty because no production route calls it, so chain genesis must be explicit
-- rather than reconstructed from evidence that was never chained.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM live_safety_case_evidence) THEN
    RAISE EXCEPTION 'cannot invent chain history for existing live safety case evidence';
  END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE live_safety_case_evidence
  ADD COLUMN chain_sequence bigint NOT NULL CHECK (chain_sequence > 0),
  ADD COLUMN previous_chain_sha256 char(64) NOT NULL CHECK (previous_chain_sha256 ~ '^[0-9a-f]{64}$'),
  ADD COLUMN chain_sha256 char(64) NOT NULL CHECK (chain_sha256 ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT live_safety_case_evidence_chain_position_unique
    UNIQUE (user_id,strategy_instance_id,chain_sequence),
  ADD CONSTRAINT live_safety_case_evidence_chain_digest_unique
    UNIQUE (user_id,strategy_instance_id,chain_sha256);

CREATE INDEX live_safety_case_evidence_chain_read_idx
  ON live_safety_case_evidence(user_id,strategy_instance_id,chain_sequence DESC);

-- Owner-wide serialization also protects the owner-scoped assessment-key
-- namespace when two different strategy chains append concurrently.
-- +goose StatementBegin
CREATE FUNCTION enforce_live_safety_case_evidence_chain() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  latest_sequence bigint;
  latest_digest text;
  genesis_digest constant text := repeat('0',64);
BEGIN
  PERFORM pg_advisory_xact_lock(
    hashtextextended(NEW.user_id::text || ':live-safety-case-evidence-chain',0)
  );

  SELECT chain_sequence,chain_sha256
    INTO latest_sequence,latest_digest
  FROM live_safety_case_evidence
  WHERE user_id=NEW.user_id AND strategy_instance_id=NEW.strategy_instance_id
  ORDER BY chain_sequence DESC
  LIMIT 1;

  IF latest_sequence IS NULL THEN
    IF NEW.chain_sequence<>1 OR NEW.previous_chain_sha256<>genesis_digest THEN
      RAISE EXCEPTION 'live safety evidence chain genesis is invalid';
    END IF;
  ELSIF NEW.chain_sequence<>latest_sequence+1 OR NEW.previous_chain_sha256<>latest_digest THEN
    RAISE EXCEPTION 'live safety evidence chain linkage is invalid';
  END IF;

  IF NEW.chain_sha256=NEW.previous_chain_sha256 OR NEW.chain_sha256=NEW.content_sha256 THEN
    RAISE EXCEPTION 'live safety evidence chain digest is not domain separated';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER live_safety_case_evidence_chain_guard
  BEFORE INSERT ON live_safety_case_evidence
  FOR EACH ROW EXECUTE FUNCTION enforce_live_safety_case_evidence_chain();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM live_safety_case_evidence) THEN
    RAISE EXCEPTION 'cannot remove immutable live safety evidence chain';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS live_safety_case_evidence_chain_guard ON live_safety_case_evidence;
DROP FUNCTION IF EXISTS enforce_live_safety_case_evidence_chain();
DROP INDEX IF EXISTS live_safety_case_evidence_chain_read_idx;
ALTER TABLE live_safety_case_evidence
  DROP CONSTRAINT IF EXISTS live_safety_case_evidence_chain_digest_unique,
  DROP CONSTRAINT IF EXISTS live_safety_case_evidence_chain_position_unique,
  DROP COLUMN IF EXISTS chain_sha256,
  DROP COLUMN IF EXISTS previous_chain_sha256,
  DROP COLUMN IF EXISTS chain_sequence;
