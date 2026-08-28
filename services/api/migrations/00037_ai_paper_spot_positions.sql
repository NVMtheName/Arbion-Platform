-- +goose Up
ALTER TABLE paper_positions DROP CONSTRAINT paper_positions_instrument_check;
ALTER TABLE paper_positions ADD CONSTRAINT paper_positions_instrument_check
  CHECK (instrument IN ('EQUITY','OPTION','CRYPTO'));

CREATE UNIQUE INDEX paper_positions_one_spot_symbol_idx
  ON paper_positions(paper_portfolio_id,symbol,instrument)
  WHERE instrument IN ('EQUITY','CRYPTO')
    AND option_type IS NULL AND strike IS NULL AND expiration IS NULL;

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM paper_positions WHERE instrument='CRYPTO') THEN
    RAISE EXCEPTION 'cannot remove AI paper crypto position projection';
  END IF;
END $$;

DROP INDEX IF EXISTS paper_positions_one_spot_symbol_idx;
ALTER TABLE paper_positions DROP CONSTRAINT paper_positions_instrument_check;
ALTER TABLE paper_positions ADD CONSTRAINT paper_positions_instrument_check
  CHECK (instrument IN ('EQUITY','OPTION'));
