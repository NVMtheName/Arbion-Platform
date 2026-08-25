-- +goose Up
ALTER TABLE strategy_instances DROP CONSTRAINT strategy_instances_strategy_identifier_check;
ALTER TABLE strategy_instances ADD CONSTRAINT strategy_instances_strategy_identifier_check
  CHECK (strategy_identifier IN ('wheel','covered_call','cash_secured_put','ai_shadow'));

ALTER TABLE decision_journal_entries DROP CONSTRAINT decision_journal_entries_source_check;
ALTER TABLE decision_journal_entries ADD CONSTRAINT decision_journal_entries_source_check
  CHECK (source IN ('STRATEGY','LIFECYCLE','AI'));

ALTER TABLE nonlive_strategy_schedules DROP CONSTRAINT nonlive_strategy_schedules_session_check;
ALTER TABLE nonlive_strategy_schedules ADD CONSTRAINT nonlive_strategy_schedules_session_check
  CHECK (session IN ('US_EQUITIES_REGULAR','CONTINUOUS'));

-- +goose Down
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM strategy_instances WHERE strategy_identifier='ai_shadow') OR
     EXISTS (SELECT 1 FROM decision_journal_entries WHERE source='AI') OR
     EXISTS (SELECT 1 FROM nonlive_strategy_schedules WHERE session='CONTINUOUS') THEN
    RAISE EXCEPTION 'cannot remove AI shadow schema while AI shadow history exists';
  END IF;
END $$;

ALTER TABLE nonlive_strategy_schedules DROP CONSTRAINT nonlive_strategy_schedules_session_check;
ALTER TABLE nonlive_strategy_schedules ADD CONSTRAINT nonlive_strategy_schedules_session_check
  CHECK (session = 'US_EQUITIES_REGULAR');

ALTER TABLE decision_journal_entries DROP CONSTRAINT decision_journal_entries_source_check;
ALTER TABLE decision_journal_entries ADD CONSTRAINT decision_journal_entries_source_check
  CHECK (source IN ('STRATEGY','LIFECYCLE'));

ALTER TABLE strategy_instances DROP CONSTRAINT strategy_instances_strategy_identifier_check;
ALTER TABLE strategy_instances ADD CONSTRAINT strategy_instances_strategy_identifier_check
  CHECK (strategy_identifier IN ('wheel','covered_call','cash_secured_put'));
