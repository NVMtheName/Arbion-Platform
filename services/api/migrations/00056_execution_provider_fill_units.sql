-- +goose Up
-- Preserve legacy exact payloads; permit only bounded provider-source metadata.
-- No runtime dispatch, account balance update, or capital release is introduced.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_observation_payload() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; e jsonb; keys text[];
BEGIN
  SELECT request INTO STRICT r FROM execution_orders WHERE id=NEW.order_id;
  keys:=ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','BaseQuantity','GrossUSD','FeeUSD','ObservedAt'];
  IF TG_TABLE_NAME='execution_fills' THEN keys:=keys||ARRAY['TradeID','PriceUSD','TradedAt'];
  ELSE keys:=keys||ARRAY['Status','CompleteFills','FillCount','CompletedAt']; END IF;
  -- Source attributes are optional only for existing legacy reports. New
  -- provider collections carry exact entry/sequence/unit and fee-rule evidence.
  IF TG_TABLE_NAME='execution_fills' AND NEW.payload ? 'ProviderEvidence' THEN
    keys:=keys||ARRAY['ProviderEvidence'];
    e:=NEW.payload->'ProviderEvidence';
    IF jsonb_typeof(e) IS DISTINCT FROM 'object'
      OR NOT (e ?& ARRAY['EntryID','SequenceAt','Size','SizeInQuote','FeeCurrency','FeeCurrencyBasis'])
      OR e-ARRAY['EntryID','SequenceAt','Size','SizeInQuote','FeeCurrency','FeeCurrencyBasis']<>'{}'::jsonb
      OR jsonb_typeof(e->'EntryID') IS DISTINCT FROM 'string'
      OR jsonb_typeof(e->'SequenceAt') IS DISTINCT FROM 'string'
      OR jsonb_typeof(e->'Size') IS DISTINCT FROM 'string'
      OR (e->>'EntryID') IS NULL OR (e->>'EntryID') !~ '^[a-zA-Z0-9_-]{1,128}$'
      OR jsonb_typeof(e->'SizeInQuote') IS DISTINCT FROM 'boolean'
      OR (e->>'SequenceAt') IS NULL OR (e->>'SequenceAt')::timestamptz<='0001-01-01'::timestamptz
      OR (e->>'SequenceAt')::timestamptz>NEW.observed_at
      OR (e->>'Size') IS NULL OR (e->>'Size') !~ '^(0|[1-9][0-9]{0,35})(\.[0-9]{1,36})?$'
      OR (e->>'Size')::numeric IS DISTINCT FROM CASE WHEN (e->>'SizeInQuote')::boolean THEN NEW.gross_usd ELSE NEW.base_quantity END
      OR e->>'FeeCurrency' IS DISTINCT FROM 'USD'
      OR e->>'FeeCurrencyBasis' IS DISTINCT FROM 'COINBASE_ADVANCED_QUOTE_ASSET_1_91' THEN
      RAISE EXCEPTION 'execution fill source units mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  IF TG_TABLE_NAME='execution_order_terminals' AND NEW.payload ? 'CompletionTimeBasis' THEN
    keys:=keys||ARRAY['CompletionTimeBasis'];
    IF NEW.payload->>'CompletionTimeBasis' IS DISTINCT FROM 'OBSERVED_TERMINAL_STATUS' OR NEW.completed_at<>NEW.observed_at THEN
      RAISE EXCEPTION 'execution terminal observation time mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  IF NOT (NEW.payload ?& keys) OR NEW.payload-keys<>'{}'::jsonb
    OR NEW.payload->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.payload->>'OrderID' IS DISTINCT FROM NEW.order_id::text
    OR NEW.payload->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.payload->>'ProviderOrderID' IS DISTINCT FROM NEW.provider_order_id::text
    OR NEW.payload->>'ConnectionID' IS DISTINCT FROM r->>'ConnectionID'
    OR NEW.payload->>'ClientOrderID' IS DISTINCT FROM r->>'ClientOrderID'
    OR NEW.payload->>'ProductID' IS DISTINCT FROM r->>'ProductID'
    OR NEW.payload->>'Side' IS DISTINCT FROM r->>'Side'
    OR (NEW.payload->>'BaseQuantity')::numeric IS DISTINCT FROM NEW.base_quantity
    OR (NEW.payload->>'GrossUSD')::numeric IS DISTINCT FROM NEW.gross_usd
    OR (NEW.payload->>'FeeUSD')::numeric IS DISTINCT FROM NEW.fee_usd
    OR (NEW.payload->>'ObservedAt')::timestamptz IS DISTINCT FROM NEW.observed_at THEN
    RAISE EXCEPTION 'execution observation payload mismatch' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='execution_fills' THEN
    IF NEW.payload->>'TradeID' IS DISTINCT FROM NEW.trade_id OR (NEW.payload->>'PriceUSD')::numeric IS DISTINCT FROM NEW.price_usd
      OR (NEW.payload->>'TradedAt')::timestamptz IS DISTINCT FROM NEW.traded_at THEN
      RAISE EXCEPTION 'execution fill payload mismatch' USING ERRCODE='23514';
    END IF;
  ELSE
    IF NEW.payload->>'Status' IS DISTINCT FROM NEW.status OR (NEW.payload->>'FillCount')::bigint IS DISTINCT FROM NEW.fill_count
      OR (NEW.payload->>'CompletedAt')::timestamptz IS DISTINCT FROM NEW.completed_at THEN
      RAISE EXCEPTION 'execution terminal payload mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Never strip the meaning of saved source-unit or observation-time evidence.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM execution_fills WHERE payload ? 'ProviderEvidence')
    OR EXISTS(SELECT 1 FROM execution_order_terminals WHERE payload ? 'CompletionTimeBasis') THEN
    RAISE EXCEPTION 'cannot remove exact execution provider provenance';
  END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION guard_execution_observation_payload() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r jsonb; keys text[];
BEGIN
  SELECT request INTO STRICT r FROM execution_orders WHERE id=NEW.order_id;
  keys:=ARRAY['OwnerID','OrderID','AccountID','ConnectionID','ClientOrderID','ProviderOrderID','ProductID','Side','BaseQuantity','GrossUSD','FeeUSD','ObservedAt'];
  IF TG_TABLE_NAME='execution_fills' THEN keys:=keys||ARRAY['TradeID','PriceUSD','TradedAt'];
  ELSE keys:=keys||ARRAY['Status','CompleteFills','FillCount','CompletedAt']; END IF;
  IF NOT (NEW.payload ?& keys) OR NEW.payload-keys<>'{}'::jsonb
    OR NEW.payload->>'OwnerID' IS DISTINCT FROM NEW.owner_id::text
    OR NEW.payload->>'OrderID' IS DISTINCT FROM NEW.order_id::text
    OR NEW.payload->>'AccountID' IS DISTINCT FROM NEW.financial_account_id::text
    OR NEW.payload->>'ProviderOrderID' IS DISTINCT FROM NEW.provider_order_id::text
    OR NEW.payload->>'ConnectionID' IS DISTINCT FROM r->>'ConnectionID'
    OR NEW.payload->>'ClientOrderID' IS DISTINCT FROM r->>'ClientOrderID'
    OR NEW.payload->>'ProductID' IS DISTINCT FROM r->>'ProductID'
    OR NEW.payload->>'Side' IS DISTINCT FROM r->>'Side'
    OR (NEW.payload->>'BaseQuantity')::numeric IS DISTINCT FROM NEW.base_quantity
    OR (NEW.payload->>'GrossUSD')::numeric IS DISTINCT FROM NEW.gross_usd
    OR (NEW.payload->>'FeeUSD')::numeric IS DISTINCT FROM NEW.fee_usd
    OR (NEW.payload->>'ObservedAt')::timestamptz IS DISTINCT FROM NEW.observed_at THEN
    RAISE EXCEPTION 'execution observation payload mismatch' USING ERRCODE='23514';
  END IF;
  IF TG_TABLE_NAME='execution_fills' THEN
    IF NEW.payload->>'TradeID' IS DISTINCT FROM NEW.trade_id OR (NEW.payload->>'PriceUSD')::numeric IS DISTINCT FROM NEW.price_usd
      OR (NEW.payload->>'TradedAt')::timestamptz IS DISTINCT FROM NEW.traded_at THEN
      RAISE EXCEPTION 'execution fill payload mismatch' USING ERRCODE='23514';
    END IF;
  ELSE
    IF NEW.payload->>'Status' IS DISTINCT FROM NEW.status OR (NEW.payload->>'FillCount')::bigint IS DISTINCT FROM NEW.fill_count
      OR (NEW.payload->>'CompletedAt')::timestamptz IS DISTINCT FROM NEW.completed_at THEN
      RAISE EXCEPTION 'execution terminal payload mismatch' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
