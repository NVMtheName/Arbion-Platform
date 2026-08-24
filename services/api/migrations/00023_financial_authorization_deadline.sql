-- +goose Up
ALTER TABLE provider_connections
  ADD COLUMN authorization_expires_at timestamptz;

-- Schwab's user authorization must be repeated on a weekly cycle. Preserve a
-- best-effort deadline for an existing connection from its latest completed
-- authorization event; new authorizations set the deadline explicitly.
UPDATE provider_connections connection
SET authorization_expires_at = COALESCE(
  (
    SELECT max(event.occurred_at) + interval '7 days'
    FROM audit_events event
    WHERE event.action = 'financial.authorization_completed'
      AND event.metadata->>'provider' = 'schwab'
      AND event.metadata->>'connection_id' = connection.id::text
  ),
  connection.created_at + interval '7 days'
)
WHERE connection.provider_category = 'financial'
  AND connection.provider_name = 'schwab';

-- +goose Down
ALTER TABLE provider_connections
  DROP COLUMN IF EXISTS authorization_expires_at;
