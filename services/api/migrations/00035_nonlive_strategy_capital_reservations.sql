-- +goose Up
CREATE TABLE strategy_capital_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL,
  financial_account_id uuid NOT NULL,
  capital_bucket_id uuid NOT NULL,
  strategy_instance_id uuid NOT NULL UNIQUE,
  execution_mode text NOT NULL CHECK (execution_mode IN ('PAPER','SHADOW')),
  reservation_amount numeric(30,10),
  currency char(3) NOT NULL,
  reservation_basis text NOT NULL CHECK (reservation_basis IN ('PAPER_STARTING_CASH','BUCKET_FIXED_CAPACITY','BUCKET_ABSOLUTE_LIMIT','UNRESOLVED_LEGACY')),
  account_allocation_limit numeric(30,10),
  reserved_at timestamptz NOT NULL,
  released_at timestamptz,
  release_reason text CHECK (release_reason IN ('COMPLETED','INACTIVE_LEGACY')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id,user_id,strategy_instance_id),
  FOREIGN KEY (strategy_instance_id,user_id)
    REFERENCES strategy_instances(id,user_id) ON DELETE RESTRICT,
  FOREIGN KEY (capital_bucket_id,user_id,financial_account_id)
    REFERENCES capital_buckets(id,user_id,financial_account_id) ON DELETE RESTRICT,
  CHECK ((reservation_amount IS NULL) = (reservation_basis='UNRESOLVED_LEGACY')),
  CHECK (reservation_amount IS NULL OR reservation_amount > 0),
  CHECK (account_allocation_limit IS NULL OR account_allocation_limit > 0),
  CHECK ((released_at IS NULL) = (release_reason IS NULL)),
  CHECK (released_at IS NULL OR released_at >= reserved_at)
);

-- Backfill every historical strategy without inventing missing percentage-based
-- dollar authority. Unresolved legacy rows remain exclusive at the account level
-- and can never participate in a shared aggregate reservation.
INSERT INTO strategy_capital_reservations(
  user_id,financial_account_id,capital_bucket_id,strategy_instance_id,
  execution_mode,reservation_amount,currency,reservation_basis,
  account_allocation_limit,reserved_at,released_at,release_reason
)
SELECT
  i.user_id,i.financial_account_id,i.capital_bucket_id,i.id,i.execution_mode,
  CASE
    WHEN i.execution_mode='PAPER' THEN p.starting_cash
    WHEN b.allocation_type='FIXED_AMOUNT'
      AND LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))>b.protected_amount
      THEN LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))-b.protected_amount
    WHEN b.allocation_type<>'FIXED_AMOUNT'
      AND b.allocation_limit IS NOT NULL AND b.allocation_limit>b.protected_amount
      THEN b.allocation_limit-b.protected_amount
    ELSE NULL
  END,
  b.currency,
  CASE
    WHEN i.execution_mode='PAPER' THEN 'PAPER_STARTING_CASH'
    WHEN b.allocation_type='FIXED_AMOUNT'
      AND LEAST(b.allocation_value,COALESCE(b.allocation_limit,b.allocation_value))>b.protected_amount
      THEN 'BUCKET_FIXED_CAPACITY'
    WHEN b.allocation_type<>'FIXED_AMOUNT'
      AND b.allocation_limit IS NOT NULL AND b.allocation_limit>b.protected_amount
      THEN 'BUCKET_ABSOLUTE_LIMIT'
    ELSE 'UNRESOLVED_LEGACY'
  END,
  CASE WHEN b.allocation_type='FIXED_AMOUNT' THEN b.allocation_limit ELSE NULL END,
  i.started_at,
  CASE WHEN i.status IN ('ACTIVE','PAUSED') THEN NULL ELSE COALESCE(i.completed_at,i.updated_at) END,
  CASE WHEN i.status IN ('ACTIVE','PAUSED') THEN NULL WHEN i.status='COMPLETED' THEN 'COMPLETED' ELSE 'INACTIVE_LEGACY' END
FROM strategy_instances i
JOIN capital_buckets b ON b.id=i.capital_bucket_id AND b.user_id=i.user_id
LEFT JOIN paper_portfolios p ON p.strategy_instance_id=i.id AND p.user_id=i.user_id;

DROP INDEX IF EXISTS strategy_one_active_account_idx;
DROP INDEX IF EXISTS strategy_one_active_bucket_idx;

CREATE UNIQUE INDEX strategy_one_active_reservation_bucket_idx
  ON strategy_capital_reservations(user_id,capital_bucket_id)
  WHERE released_at IS NULL;
CREATE INDEX strategy_active_reservations_account_idx
  ON strategy_capital_reservations(user_id,financial_account_id,reserved_at)
  WHERE released_at IS NULL;

-- A reservation is serialized per owner/account. A second active strategy is
-- admitted only when every participant has an exact amount, the same currency,
-- and the same explicit fixed-allocation account ceiling.
-- +goose StatementBegin
CREATE FUNCTION enforce_strategy_capital_reservation_insert() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE TRIGGER strategy_capital_reservation_insert_guard
  BEFORE INSERT ON strategy_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION enforce_strategy_capital_reservation_insert();

-- Reservation identity and amounts are immutable. The sole allowed mutation is
-- a one-way release that exactly accompanies completion of its strategy.
-- +goose StatementBegin
CREATE FUNCTION enforce_strategy_capital_reservation_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION 'strategy capital reservation evidence cannot be deleted'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_immutable';
  END IF;
  IF OLD.released_at IS NOT NULL OR NEW.released_at IS NULL OR NEW.release_reason<>'COMPLETED'
     OR NEW.id<>OLD.id OR NEW.user_id<>OLD.user_id
     OR NEW.financial_account_id<>OLD.financial_account_id
     OR NEW.capital_bucket_id<>OLD.capital_bucket_id
     OR NEW.strategy_instance_id<>OLD.strategy_instance_id
     OR NEW.execution_mode<>OLD.execution_mode
     OR NEW.reservation_amount IS DISTINCT FROM OLD.reservation_amount
     OR NEW.currency<>OLD.currency OR NEW.reservation_basis<>OLD.reservation_basis
     OR NEW.account_allocation_limit IS DISTINCT FROM OLD.account_allocation_limit
     OR NEW.reserved_at<>OLD.reserved_at OR NEW.created_at<>OLD.created_at
     OR NOT EXISTS(
       SELECT 1 FROM strategy_instances i
       WHERE i.id=OLD.strategy_instance_id AND i.user_id=OLD.user_id
         AND i.status='COMPLETED' AND i.completed_at=NEW.released_at
     ) THEN
    RAISE EXCEPTION 'strategy capital reservation evidence is immutable except for exact completion release'
      USING ERRCODE='23514', CONSTRAINT='strategy_capital_reservation_immutable';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER strategy_capital_reservation_immutable
  BEFORE UPDATE OR DELETE ON strategy_capital_reservations
  FOR EACH ROW EXECUTE FUNCTION enforce_strategy_capital_reservation_mutation();

-- Active and paused instances must own one active reservation; every other
-- instance status must have released it. Deferred checking keeps initialization
-- and completion atomic inside one transaction.
-- +goose StatementBegin
CREATE FUNCTION enforce_strategy_instance_capital_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE active_reservation boolean;
BEGIN
  SELECT EXISTS(
    SELECT 1 FROM strategy_capital_reservations r
    WHERE r.strategy_instance_id=NEW.id AND r.user_id=NEW.user_id AND r.released_at IS NULL
  ) INTO active_reservation;
  IF (NEW.status IN ('ACTIVE','PAUSED'))<>active_reservation THEN
    RAISE EXCEPTION 'strategy instance and capital reservation lifecycle do not match'
      USING ERRCODE='23514', CONSTRAINT='strategy_instance_capital_reservation_guard';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER strategy_instance_capital_reservation_guard
  AFTER INSERT OR UPDATE OF status ON strategy_instances
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION enforce_strategy_instance_capital_reservation();

-- Active reservations freeze the financial policy fields that established the
-- exact claim. Cosmetic bucket renames remain possible.
-- +goose StatementBegin
CREATE FUNCTION enforce_reserved_capital_bucket_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS(
    SELECT 1 FROM strategy_capital_reservations r
    WHERE r.capital_bucket_id=OLD.id AND r.user_id=OLD.user_id AND r.released_at IS NULL
  ) AND (
    NEW.financial_account_id<>OLD.financial_account_id OR
    NEW.allocation_type<>OLD.allocation_type OR
    NEW.allocation_value<>OLD.allocation_value OR
    NEW.currency<>OLD.currency OR
    NEW.is_reserve<>OLD.is_reserve OR
    NEW.protected_amount<>OLD.protected_amount OR
    NEW.allocation_limit IS DISTINCT FROM OLD.allocation_limit OR
    NEW.status<>OLD.status
  ) THEN
    RAISE EXCEPTION 'active strategy reservation freezes capital bucket policy'
      USING ERRCODE='23514', CONSTRAINT='reserved_capital_bucket_policy_guard';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER reserved_capital_bucket_policy_guard
  BEFORE UPDATE ON capital_buckets
  FOR EACH ROW EXECUTE FUNCTION enforce_reserved_capital_bucket_policy();

-- +goose Down
DROP TRIGGER IF EXISTS reserved_capital_bucket_policy_guard ON capital_buckets;
DROP FUNCTION IF EXISTS enforce_reserved_capital_bucket_policy();
DROP TRIGGER IF EXISTS strategy_instance_capital_reservation_guard ON strategy_instances;
DROP FUNCTION IF EXISTS enforce_strategy_instance_capital_reservation();
DROP TRIGGER IF EXISTS strategy_capital_reservation_immutable ON strategy_capital_reservations;
DROP FUNCTION IF EXISTS enforce_strategy_capital_reservation_mutation();
DROP TRIGGER IF EXISTS strategy_capital_reservation_insert_guard ON strategy_capital_reservations;
DROP FUNCTION IF EXISTS enforce_strategy_capital_reservation_insert();
DROP INDEX IF EXISTS strategy_active_reservations_account_idx;
DROP INDEX IF EXISTS strategy_one_active_reservation_bucket_idx;
DROP TABLE IF EXISTS strategy_capital_reservations;
CREATE UNIQUE INDEX strategy_one_active_bucket_idx
  ON strategy_instances(user_id,capital_bucket_id)
  WHERE status IN ('ACTIVE','PAUSED');
CREATE UNIQUE INDEX strategy_one_active_account_idx
  ON strategy_instances(user_id,financial_account_id)
  WHERE status IN ('ACTIVE','PAUSED');
