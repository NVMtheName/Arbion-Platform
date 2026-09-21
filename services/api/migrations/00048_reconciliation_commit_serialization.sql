-- +goose Up
-- Non-live commits hold the owner/account row FOR SHARE until their write is
-- committed. Every reconciliation insert must conflict with that lock before
-- it can publish new evidence. A foreign-key KEY SHARE lock alone does not.
-- This covers application persistence and direct SQL without rewriting reports.
-- +goose StatementBegin
CREATE FUNCTION serialize_portfolio_reconciliation_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM 1 FROM financial_accounts
  WHERE id=NEW.financial_account_id
    AND user_id=NEW.user_id
    AND provider_name=NEW.provider_name
  FOR NO KEY UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'portfolio reconciliation account/provider mismatch'
      USING ERRCODE='23514', CONSTRAINT='portfolio_reconciliation_account_commit_guard';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- PostgreSQL runs same-event triggers alphabetically. Account locking precedes
-- existing additive, change-impact, and source checks and lasts until commit.
-- Do not reuse the service's session-level reconciliation advisory lock here:
-- its callback persists using a different pooled database connection.
CREATE TRIGGER portfolio_reconciliation_account_commit_lock
  BEFORE INSERT ON portfolio_reconciliations
  FOR EACH ROW EXECUTE FUNCTION serialize_portfolio_reconciliation_insert();

-- +goose Down
-- Roll back application releases relying on this guard before this schema.
DROP TRIGGER portfolio_reconciliation_account_commit_lock ON portfolio_reconciliations;
DROP FUNCTION serialize_portfolio_reconciliation_insert();
