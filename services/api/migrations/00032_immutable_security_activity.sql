-- +goose Up
CREATE INDEX audit_events_owner_activity_time_idx
  ON audit_events(user_id,occurred_at DESC,id DESC)
  WHERE user_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION reject_audit_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'platform audit history is append-only';
END $$;
-- +goose StatementEnd

CREATE TRIGGER audit_events_append_only
  BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_append_only ON audit_events;
DROP FUNCTION IF EXISTS reject_audit_event_mutation();
DROP INDEX IF EXISTS audit_events_owner_activity_time_idx;
