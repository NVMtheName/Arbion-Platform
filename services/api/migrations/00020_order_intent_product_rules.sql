-- +goose Up
ALTER TABLE order_intent_previews
  ADD COLUMN product_rules_feed text,
  ADD COLUMN product_type text,
  ADD COLUMN product_status text,
  ADD COLUMN base_increment numeric(38,18),
  ADD COLUMN quote_increment numeric(38,18),
  ADD COLUMN base_min_size numeric(38,18),
  ADD COLUMN base_max_size numeric(38,18),
  ADD COLUMN quote_min_size numeric(38,18),
  ADD COLUMN quote_max_size numeric(38,18),
  ADD COLUMN product_market_ioc_enabled boolean,
  ADD COLUMN product_block_reasons jsonb,
  ADD COLUMN product_rules_observed_at timestamptz;

ALTER TABLE order_intent_previews ADD CONSTRAINT order_intent_product_rules_all_or_none CHECK (
  (product_rules_feed IS NULL AND product_type IS NULL AND product_status IS NULL AND
   base_increment IS NULL AND quote_increment IS NULL AND base_min_size IS NULL AND base_max_size IS NULL AND
   quote_min_size IS NULL AND quote_max_size IS NULL AND product_market_ioc_enabled IS NULL AND
   product_block_reasons IS NULL AND product_rules_observed_at IS NULL)
  OR
  (product_rules_feed='advanced_trade_product' AND product_type='SPOT' AND product_status ~ '^[A-Z][A-Z0-9_-]{0,31}$' AND
   base_increment > 0 AND quote_increment > 0 AND base_min_size > 0 AND base_max_size >= base_min_size AND
   quote_min_size > 0 AND quote_max_size >= quote_min_size AND
   jsonb_typeof(product_block_reasons)='array' AND jsonb_array_length(product_block_reasons) <= 20 AND
   product_block_reasons <@ '["PRODUCT_AUCTION_MODE","PRODUCT_CANCEL_ONLY","PRODUCT_DISABLED","PRODUCT_LIMIT_ONLY","PRODUCT_POST_ONLY","SIZE_ABOVE_MAXIMUM","SIZE_BELOW_MINIMUM","SIZE_INCREMENT_MISMATCH"]'::jsonb AND
   (product_status='ONLINE' OR product_block_reasons @> '["PRODUCT_DISABLED"]'::jsonb) AND
   product_market_ioc_enabled=(jsonb_array_length(product_block_reasons)=0) AND product_rules_observed_at=previewed_at)
);

-- +goose Down
ALTER TABLE order_intent_previews DROP CONSTRAINT IF EXISTS order_intent_product_rules_all_or_none;
ALTER TABLE order_intent_previews
  DROP COLUMN IF EXISTS product_rules_observed_at,
  DROP COLUMN IF EXISTS product_block_reasons,
  DROP COLUMN IF EXISTS product_market_ioc_enabled,
  DROP COLUMN IF EXISTS quote_max_size,
  DROP COLUMN IF EXISTS quote_min_size,
  DROP COLUMN IF EXISTS base_max_size,
  DROP COLUMN IF EXISTS base_min_size,
  DROP COLUMN IF EXISTS quote_increment,
  DROP COLUMN IF EXISTS base_increment,
  DROP COLUMN IF EXISTS product_status,
  DROP COLUMN IF EXISTS product_type,
  DROP COLUMN IF EXISTS product_rules_feed;
