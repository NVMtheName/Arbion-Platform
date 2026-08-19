-- +goose Up
ALTER TABLE automation_mandates
  ADD COLUMN paper_options_simulation_attested boolean NOT NULL DEFAULT false,
  ADD CONSTRAINT automation_mandates_paper_options_simulation_attested_check CHECK (
    NOT paper_options_simulation_attested OR (
      automation_type = 'STRATEGY'
      AND execution_mode = 'PAPER'
      AND strategy_identifier IS NOT NULL
      AND options_allowed
    )
  );

-- +goose Down
ALTER TABLE automation_mandates
  DROP CONSTRAINT IF EXISTS automation_mandates_paper_options_simulation_attested_check,
  DROP COLUMN IF EXISTS paper_options_simulation_attested;
