-- +goose Up
CREATE INDEX decision_journal_owner_feed_idx
  ON decision_journal_entries(user_id,created_at DESC,id DESC);

-- +goose Down
DROP INDEX IF EXISTS decision_journal_owner_feed_idx;
