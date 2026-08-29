-- +goose Up
-- Paper starting cash belongs only to an isolated simulated ledger. It must
-- remain exact and bucket-bounded, but it must not consume or dilute the
-- account-level capital authority reserved by Shadow strategies.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_strategy_capital_reservation_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  bucket_row capital_buckets%ROWTYPE;
  paper_starting_cash numeric(30,10);
  expected_amount numeric(30,10);
  active_count bigint;
  active_amount numeric(30,10);
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text || ':' || NEW.financial_account_id::text || ':strategy-capital-reservations', 0));

  IF NEW.released_at IS NOT NULL OR NEW.release_reason IS NOT NULL OR NEW.reservation_basis='UNRESOLVED_LEGACY' THEN
    RAISE EXCEPTION 'new strategy reservation must begin active and resolved'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_resolved_guard';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM strategy_instances i
    WHERE i.id=NEW.strategy_instance_id AND i.user_id=NEW.user_id
      AND i.financial_account_id=NEW.financial_account_id
      AND i.capital_bucket_id=NEW.capital_bucket_id
      AND i.execution_mode=NEW.execution_mode
      AND i.status IN ('ACTIVE','PAUSED')
  ) THEN
    RAISE EXCEPTION 'strategy reservation does not match an active owner-bound instance'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_instance_guard';
  END IF;

  SELECT * INTO STRICT bucket_row FROM capital_buckets
  WHERE id=NEW.capital_bucket_id AND user_id=NEW.user_id
    AND financial_account_id=NEW.financial_account_id
    AND status='ACTIVE' AND NOT is_reserve;
  IF NEW.currency<>bucket_row.currency THEN
    RAISE EXCEPTION 'strategy reservation currency does not match its bucket'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
  END IF;

  IF NEW.reservation_basis='PAPER_STARTING_CASH' THEN
    IF NEW.execution_mode<>'PAPER' THEN
      RAISE EXCEPTION 'paper reservation basis requires paper mode'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
    SELECT starting_cash INTO STRICT paper_starting_cash FROM paper_portfolios
      WHERE strategy_instance_id=NEW.strategy_instance_id AND user_id=NEW.user_id;
    expected_amount=LEAST(bucket_row.allocation_value,COALESCE(bucket_row.allocation_limit,bucket_row.allocation_value))-bucket_row.protected_amount;
    IF expected_amount<=0 OR NEW.reservation_amount<>paper_starting_cash OR NEW.reservation_amount>expected_amount THEN
      RAISE EXCEPTION 'paper reservation does not match starting cash'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSIF NEW.reservation_basis='BUCKET_FIXED_CAPACITY' THEN
    expected_amount=LEAST(bucket_row.allocation_value,COALESCE(bucket_row.allocation_limit,bucket_row.allocation_value))-bucket_row.protected_amount;
    IF NEW.execution_mode<>'SHADOW' OR bucket_row.allocation_type<>'FIXED_AMOUNT' OR expected_amount<=0 OR NEW.reservation_amount<>expected_amount THEN
      RAISE EXCEPTION 'fixed reservation does not match its bucket capacity'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSIF NEW.reservation_basis='BUCKET_ABSOLUTE_LIMIT' THEN
    expected_amount=bucket_row.allocation_limit-bucket_row.protected_amount;
    IF NEW.execution_mode<>'SHADOW' OR bucket_row.allocation_type='FIXED_AMOUNT' OR bucket_row.allocation_limit IS NULL OR expected_amount<=0 OR NEW.reservation_amount<>expected_amount THEN
      RAISE EXCEPTION 'percentage reservation does not match its absolute cap'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSE
    RAISE EXCEPTION 'strategy reservation basis is unsupported'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
  END IF;

  IF bucket_row.allocation_type='FIXED_AMOUNT' THEN
    IF NEW.account_allocation_limit IS DISTINCT FROM bucket_row.allocation_limit THEN
      RAISE EXCEPTION 'fixed reservation account ceiling does not match its bucket'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
    END IF;
  ELSIF NEW.account_allocation_limit IS NOT NULL THEN
    RAISE EXCEPTION 'percentage reservation cannot define a shared account ceiling'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
  END IF;

  -- The exact paper claim remains durable and immutable, but it is simulated
  -- authority and therefore never participates in Shadow account aggregation.
  IF NEW.execution_mode='PAPER' THEN
    RETURN NEW;
  END IF;

  SELECT count(*),COALESCE(sum(reservation_amount),0)
    INTO active_count,active_amount
  FROM strategy_capital_reservations
  WHERE user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id
    AND execution_mode='SHADOW' AND released_at IS NULL;

  IF active_count>0 THEN
    IF NEW.account_allocation_limit IS NULL OR NEW.reservation_amount IS NULL OR EXISTS(
      SELECT 1 FROM strategy_capital_reservations r
      WHERE r.user_id=NEW.user_id AND r.financial_account_id=NEW.financial_account_id
        AND r.execution_mode='SHADOW' AND r.released_at IS NULL
        AND (r.reservation_amount IS NULL OR r.currency<>NEW.currency OR r.account_allocation_limit IS DISTINCT FROM NEW.account_allocation_limit)
    ) OR active_amount+NEW.reservation_amount>NEW.account_allocation_limit THEN
      RAISE EXCEPTION 'active Shadow strategy reservations do not fit one explicit account ceiling'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_account_guard';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- A downgrade is safe only when it would not strand a coexistence pattern that
-- the former all-mode account aggregate rejects.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS(
    SELECT 1
    FROM strategy_capital_reservations paper
    JOIN strategy_capital_reservations other
      ON other.user_id=paper.user_id
     AND other.financial_account_id=paper.financial_account_id
     AND other.id<>paper.id
     AND other.released_at IS NULL
    WHERE paper.execution_mode='PAPER' AND paper.released_at IS NULL
  ) THEN
    RAISE EXCEPTION 'cannot restore the legacy aggregate while an active Paper reservation shares an account';
  END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_strategy_capital_reservation_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  bucket_row capital_buckets%ROWTYPE;
  paper_starting_cash numeric(30,10);
  expected_amount numeric(30,10);
  active_count bigint;
  active_amount numeric(30,10);
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended(NEW.user_id::text || ':' || NEW.financial_account_id::text || ':strategy-capital-reservations', 0));

  IF NEW.released_at IS NOT NULL OR NEW.release_reason IS NOT NULL OR NEW.reservation_basis='UNRESOLVED_LEGACY' THEN
    RAISE EXCEPTION 'new strategy reservation must begin active and resolved'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_resolved_guard';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM strategy_instances i
    WHERE i.id=NEW.strategy_instance_id AND i.user_id=NEW.user_id
      AND i.financial_account_id=NEW.financial_account_id
      AND i.capital_bucket_id=NEW.capital_bucket_id
      AND i.execution_mode=NEW.execution_mode
      AND i.status IN ('ACTIVE','PAUSED')
  ) THEN
    RAISE EXCEPTION 'strategy reservation does not match an active owner-bound instance'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_instance_guard';
  END IF;

  SELECT * INTO STRICT bucket_row FROM capital_buckets
  WHERE id=NEW.capital_bucket_id AND user_id=NEW.user_id
    AND financial_account_id=NEW.financial_account_id
    AND status='ACTIVE' AND NOT is_reserve;
  IF NEW.currency<>bucket_row.currency THEN
    RAISE EXCEPTION 'strategy reservation currency does not match its bucket'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
  END IF;

  IF NEW.reservation_basis='PAPER_STARTING_CASH' THEN
    IF NEW.execution_mode<>'PAPER' THEN
      RAISE EXCEPTION 'paper reservation basis requires paper mode'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
    SELECT starting_cash INTO STRICT paper_starting_cash FROM paper_portfolios
      WHERE strategy_instance_id=NEW.strategy_instance_id AND user_id=NEW.user_id;
    expected_amount=LEAST(bucket_row.allocation_value,COALESCE(bucket_row.allocation_limit,bucket_row.allocation_value))-bucket_row.protected_amount;
    IF expected_amount<=0 OR NEW.reservation_amount<>paper_starting_cash OR NEW.reservation_amount>expected_amount THEN
      RAISE EXCEPTION 'paper reservation does not match starting cash'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSIF NEW.reservation_basis='BUCKET_FIXED_CAPACITY' THEN
    expected_amount=LEAST(bucket_row.allocation_value,COALESCE(bucket_row.allocation_limit,bucket_row.allocation_value))-bucket_row.protected_amount;
    IF NEW.execution_mode<>'SHADOW' OR bucket_row.allocation_type<>'FIXED_AMOUNT' OR expected_amount<=0 OR NEW.reservation_amount<>expected_amount THEN
      RAISE EXCEPTION 'fixed reservation does not match its bucket capacity'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSIF NEW.reservation_basis='BUCKET_ABSOLUTE_LIMIT' THEN
    expected_amount=bucket_row.allocation_limit-bucket_row.protected_amount;
    IF NEW.execution_mode<>'SHADOW' OR bucket_row.allocation_type='FIXED_AMOUNT' OR bucket_row.allocation_limit IS NULL OR expected_amount<=0 OR NEW.reservation_amount<>expected_amount THEN
      RAISE EXCEPTION 'percentage reservation does not match its absolute cap'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
    END IF;
  ELSE
    RAISE EXCEPTION 'strategy reservation basis is unsupported'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_basis_guard';
  END IF;

  IF bucket_row.allocation_type='FIXED_AMOUNT' THEN
    IF NEW.account_allocation_limit IS DISTINCT FROM bucket_row.allocation_limit THEN
      RAISE EXCEPTION 'fixed reservation account ceiling does not match its bucket'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
    END IF;
  ELSIF NEW.account_allocation_limit IS NOT NULL THEN
    RAISE EXCEPTION 'percentage reservation cannot define a shared account ceiling'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_bucket_guard';
  END IF;

  SELECT count(*),COALESCE(sum(reservation_amount),0)
    INTO active_count,active_amount
  FROM strategy_capital_reservations
  WHERE user_id=NEW.user_id AND financial_account_id=NEW.financial_account_id
    AND released_at IS NULL;

  IF active_count>0 THEN
    IF NEW.account_allocation_limit IS NULL OR NEW.reservation_amount IS NULL OR EXISTS(
      SELECT 1 FROM strategy_capital_reservations r
      WHERE r.user_id=NEW.user_id AND r.financial_account_id=NEW.financial_account_id
        AND r.released_at IS NULL
        AND (r.reservation_amount IS NULL OR r.currency<>NEW.currency OR r.account_allocation_limit IS DISTINCT FROM NEW.account_allocation_limit)
    ) OR active_amount+NEW.reservation_amount>NEW.account_allocation_limit THEN
      RAISE EXCEPTION 'active strategy reservations do not fit one explicit account ceiling'
        USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_account_guard';
    END IF;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
