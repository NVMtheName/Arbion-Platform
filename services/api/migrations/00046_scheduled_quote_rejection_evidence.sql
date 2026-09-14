-- +goose Up
-- Nullable, forward-only metadata. Existing immutable rows are not rewritten.
ALTER TABLE nonlive_schedule_runs ADD COLUMN quote_rejection jsonb;

-- +goose StatementBegin
CREATE FUNCTION enforce_scheduled_quote_rejection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  e jsonb := NEW.quote_rejection;
  keys text[] := ARRAY['schema_version','provider','financial_account_id','requested_symbol',
    'quote_type','realtime','provider_observed_at','evaluated_at','rejection_code','before_model'];
BEGIN
  IF e IS NULL THEN RETURN NEW; END IF;
  IF NOT COALESCE(
    jsonb_typeof(e)='object' AND octet_length(e::text)<=1536
    AND e ?& keys AND e - keys = '{}'::jsonb
    AND e->'schema_version'='1'::jsonb
    AND e->>'provider'='schwab' AND e->'before_model'='true'::jsonb
    AND jsonb_typeof(e->'requested_symbol')='string'
    AND e->>'requested_symbol' ~ '^[A-Z0-9][A-Z0-9./$^-]{0,31}$'
    AND e->>'quote_type' IN ('NBBO','NFL','UNAVAILABLE','UNRECOGNIZED')
    AND jsonb_typeof(e->'realtime') IN ('boolean','null')
    AND jsonb_typeof(e->'financial_account_id')='string'
    AND e->>'financial_account_id' ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
    AND NEW.status='FAILED' AND NEW.strategy_state='AI_MONITORING'
    AND NEW.ai_decision IS NULL AND NEW.execution_status IS NULL AND NOT NEW.duplicate_recovered
    AND e->>'rejection_code'=NEW.error_code
    AND NEW.error_code IN ('MARKET_DATA_INVALID','MARKET_DATA_STALE','MARKET_DATA_DELAYED','MARKET_DATA_REALTIME_UNCONFIRMED')
    AND (NEW.error_code<>'MARKET_DATA_DELAYED' OR e->'realtime'='false'::jsonb)
    AND (NEW.error_code<>'MARKET_DATA_REALTIME_UNCONFIRMED' OR e->'realtime'='null'::jsonb)
    AND jsonb_typeof(e->'evaluated_at')='string'
    AND e->>'evaluated_at' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$'
    AND (e->'provider_observed_at'='null'::jsonb OR
      (jsonb_typeof(e->'provider_observed_at')='string'
        AND e->>'provider_observed_at' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?Z$'))
  ,false) THEN
    RAISE EXCEPTION 'invalid bounded scheduled quote rejection evidence';
  END IF;
  IF (e->>'evaluated_at')::timestamptz < NEW.started_at OR
     (e->>'evaluated_at')::timestamptz > NEW.completed_at THEN
    RAISE EXCEPTION 'quote rejection evaluation time is outside its scheduled run';
  END IF;
  -- Validate parseability, but retain future provider times as rejected evidence.
  IF e->'provider_observed_at'<>'null'::jsonb THEN
    PERFORM (e->>'provider_observed_at')::timestamptz;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM strategy_instances i
    JOIN financial_accounts a ON a.id=i.financial_account_id AND a.user_id=i.user_id
    WHERE i.id=NEW.strategy_instance_id AND i.user_id=NEW.user_id
      AND i.financial_account_id=(e->>'financial_account_id')::uuid
      AND a.provider_name=e->>'provider'
  ) THEN
    RAISE EXCEPTION 'quote rejection evidence does not match its owner account';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER scheduled_quote_rejection_guard
  BEFORE INSERT ON nonlive_schedule_runs
  FOR EACH ROW EXECUTE FUNCTION enforce_scheduled_quote_rejection();
-- The existing UPDATE/DELETE immutability trigger covers the new column too.

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM nonlive_schedule_runs WHERE quote_rejection IS NOT NULL) THEN
    RAISE EXCEPTION 'cannot remove immutable scheduled quote rejection evidence';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER scheduled_quote_rejection_guard ON nonlive_schedule_runs;
DROP FUNCTION enforce_scheduled_quote_rejection();
ALTER TABLE nonlive_schedule_runs DROP COLUMN quote_rejection;
