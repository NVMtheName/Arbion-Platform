package execution

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReadReconciliation recovers one consistent database snapshot. It grants no
// dispatch authority and does not represent a cash/position balance ledger.
func (s *PostgresStore) ReadReconciliation(ctx context.Context, ownerID, orderID string) (Reconciliation, error) {
	if !validUUID(ownerID) || !validUUID(orderID) {
		return Reconciliation{}, ErrInvalid
	}
	var r Reconciliation
	err := s.db.QueryRow(ctx, `SELECT f.n,f.q::text,f.g::text,f.fee::text,COALESCE(t.status,''),
		EXISTS(SELECT 1 FROM execution_account_holds h WHERE h.financial_account_id=o.financial_account_id),
		EXISTS(SELECT 1 FROM execution_reconciliation_blocks b WHERE b.financial_account_id=o.financial_account_id)
		FROM execution_orders o LEFT JOIN execution_order_terminals t ON t.order_id=o.id
		CROSS JOIN LATERAL (SELECT count(*) n,COALESCE(sum(base_quantity),0) q,COALESCE(sum(gross_usd),0) g,COALESCE(sum(fee_usd),0) fee
		FROM execution_fills WHERE order_id=o.id) f WHERE o.id=$1 AND o.owner_id=$2`, orderID, ownerID).
		Scan(&r.FillCount, &r.BaseQuantity, &r.GrossUSD, &r.FeeUSD, &r.TerminalStatus, &r.AccountHeld, &r.AccountBlocked)
	if err != nil {
		return Reconciliation{}, mapError(err)
	}
	for _, p := range []*string{&r.BaseQuantity, &r.GrossUSD, &r.FeeUSD} {
		n, ok := amount(*p)
		if !ok {
			return Reconciliation{}, ErrInvalid
		}
		*p = canonical(n)
	}
	return r, nil
}

// reconciliationLock uses the same account-first serialization as Claim.
// It also checks broker correlation before accepting any financial evidence.
func reconciliationLock(ctx context.Context, tx pgx.Tx, b BrokerIdentity) (Order, Attempt, time.Time, error) {
	if !validUUID(b.OwnerID) || !validUUID(b.OrderID) {
		return Order{}, Attempt{}, time.Time{}, ErrInvalid
	}
	o, err := readOrder(ctx, tx, b.OwnerID, "id", b.OrderID)
	if err != nil {
		return o, Attempt{}, time.Time{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR UPDATE`, o.Request.AccountID, b.OwnerID).Scan(&id)
	if err != nil {
		return o, Attempt{}, time.Time{}, mapError(err)
	}
	var a Attempt
	err = tx.QueryRow(ctx, `SELECT a.claimed_at,b.provider_order_id::text FROM execution_dispatch_attempts a
		JOIN execution_broker_acknowledgements b ON b.order_id=a.order_id WHERE a.order_id=$1 AND a.owner_id=$2`, o.ID, b.OwnerID).Scan(&a.ClaimedAt, &a.ProviderOrderID)
	if err != nil {
		return o, a, time.Time{}, mapError(err)
	}
	if !identityMatches(o, a, b) {
		return o, a, time.Time{}, ErrInvalid
	}
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_reconciliation_blocks WHERE financial_account_id=$1)`, id).Scan(&blocked); err != nil {
		return o, a, time.Time{}, err
	}
	if blocked {
		return o, a, time.Time{}, ErrReconciliationBlocked
	}
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return o, a, now, err
}

// RecordFill never mutates real holdings. It records one immutable normalized
// broker fill. A conflicting economic identity or late new fill quarantines
// the account durably, even when a previous order already released its hold.
func (s *PostgresStore) RecordFill(ctx context.Context, f Fill) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = s.recordFill(ctx, tx, f); err != nil && !errors.Is(err, ErrReconciliationBlocked) {
		return err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return ErrCommitUnknown
	}
	return err
}

// Used by the collection coordinator under its current-access transaction.
// A proven conflict stages quarantine; the caller owns the final commit guard.
func (s *PostgresStore) recordFill(ctx context.Context, tx pgx.Tx, f Fill) error {
	o, a, now, err := reconciliationLock(ctx, tx, f.BrokerIdentity)
	if err != nil {
		return err
	}
	input := f
	f, err = normalizeFill(o, a, f, now)
	if err != nil {
		return s.block(ctx, tx, o, "INVALID_FILL", input)
	}
	var body []byte
	err = tx.QueryRow(ctx, `SELECT payload FROM execution_fills WHERE order_id=$1 AND trade_id=$2`, o.ID, f.TradeID).Scan(&body)
	if err == nil {
		var saved Fill
		if json.Unmarshal(body, &saved) != nil || !sameFill(saved, f) {
			return s.block(ctx, tx, o, "FILL_CONFLICT", input)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var terminal bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_order_terminals WHERE order_id=$1)`, o.ID).Scan(&terminal); err != nil {
		return err
	}
	if terminal {
		return s.block(ctx, tx, o, "LATE_FILL", input)
	}
	totals, _, err := readTotals(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	q, _ := amount(totals.BaseQuantity)
	fq, _ := amount(f.BaseQuantity)
	g, _ := amount(totals.GrossUSD)
	fg, _ := amount(f.GrossUSD)
	fees, _ := amount(totals.FeeUSD)
	ff, _ := amount(f.FeeUSD)
	next := Totals{totals.FillCount + 1, canonical(new(big.Rat).Add(q, fq)), canonical(new(big.Rat).Add(g, fg)), canonical(new(big.Rat).Add(fees, ff))}
	if !withinBounds(o.Request, next) {
		return s.block(ctx, tx, o, "FILL_LIMIT_BREACH", input)
	}
	body, _ = json.Marshal(f)
	// Use the database's projection of the exact payload timestamps. Go's
	// half-microsecond rounding can differ from PostgreSQL's timestamp parser.
	// Original nanoseconds remain immutable in the payload.
	_, err = tx.Exec(ctx, `INSERT INTO execution_fills(order_id,owner_id,financial_account_id,provider_order_id,trade_id,base_quantity,price_usd,gross_usd,fee_usd,traded_at,observed_at,payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,($10::jsonb->>'TradedAt')::timestamptz,($10::jsonb->>'ObservedAt')::timestamptz,$10)`, o.ID, f.OwnerID, f.AccountID, f.ProviderOrderID, f.TradeID, f.BaseQuantity, f.PriceUSD, f.GrossUSD, f.FeeUSD, body)
	if err != nil {
		return mapError(err)
	}
	return nil
}

// ReconcileTerminal releases only the submission slot, not financial capital.
// Capital release requires the separate exact account-settlement gate.
// Incomplete pagination or unmatched totals leave the slot held.
func (s *PostgresStore) ReconcileTerminal(ctx context.Context, t TerminalReport) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = s.reconcileTerminal(ctx, tx, t); err != nil && !errors.Is(err, ErrReconciliationBlocked) {
		return err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return ErrCommitUnknown
	}
	return err
}

func (s *PostgresStore) reconcileTerminal(ctx context.Context, tx pgx.Tx, t TerminalReport) error {
	o, a, now, err := reconciliationLock(ctx, tx, t.BrokerIdentity)
	if err != nil {
		return err
	}
	input := t
	var body []byte
	savedErr := tx.QueryRow(ctx, `SELECT payload FROM execution_order_terminals WHERE order_id=$1`, o.ID).Scan(&body)
	if savedErr != nil && !errors.Is(savedErr, pgx.ErrNoRows) {
		return savedErr
	}
	t, err = normalizeTerminal(o, a, t, now)
	if err != nil {
		if errors.Is(err, ErrInvalid) || (savedErr == nil && input.CompleteFills) {
			return s.block(ctx, tx, o, "TERMINAL_CONFLICT", input)
		}
		return err
	}
	if savedErr == nil {
		var saved TerminalReport
		if json.Unmarshal(body, &saved) != nil || !sameTerminal(saved, t) {
			return s.block(ctx, tx, o, "TERMINAL_CONFLICT", input)
		}
		return nil
	}
	totals, latest, err := readTotals(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	if totals.FillCount != t.FillCount || totals.BaseQuantity != t.BaseQuantity || totals.GrossUSD != t.GrossUSD || totals.FeeUSD != t.FeeUSD ||
		(latest != nil && latest.After(t.CompletedAt)) {
		return ErrUnreconciled
	}
	body, _ = json.Marshal(t)
	_, err = tx.Exec(ctx, `INSERT INTO execution_order_terminals(order_id,owner_id,financial_account_id,provider_order_id,status,fill_count,base_quantity,gross_usd,fee_usd,completed_at,observed_at,payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,($10::jsonb->>'CompletedAt')::timestamptz,($10::jsonb->>'ObservedAt')::timestamptz,$10)`, o.ID, t.OwnerID, t.AccountID, t.ProviderOrderID, t.Status, t.FillCount, t.BaseQuantity, t.GrossUSD, t.FeeUSD, body)
	if err != nil {
		return mapError(err)
	}
	// The schema validates totals and deletes this order's hold in the same tx.
	return nil
}

func readTotals(ctx context.Context, tx pgx.Tx, orderID string) (Totals, *time.Time, error) {
	var t Totals
	var latest *time.Time
	err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(base_quantity),0)::text,COALESCE(sum(gross_usd),0)::text,COALESCE(sum(fee_usd),0)::text,max(traded_at)
		FROM execution_fills WHERE order_id=$1`, orderID).Scan(&t.FillCount, &t.BaseQuantity, &t.GrossUSD, &t.FeeUSD, &latest)
	if err != nil {
		return t, latest, err
	}
	for _, p := range []*string{&t.BaseQuantity, &t.GrossUSD, &t.FeeUSD} {
		n, ok := amount(*p)
		if !ok {
			return Totals{}, nil, ErrInvalid
		}
		*p = canonical(n)
	}
	// PostgreSQL timestamps have microsecond precision, while provider evidence
	// may be finer. Recover exact original times for the latest timestamp bucket
	// so a terminal report cannot precede a fill by an invisible nanosecond.
	if latest != nil {
		rows, err := tx.Query(ctx, `SELECT payload->>'TradedAt' FROM execution_fills WHERE order_id=$1 AND traded_at=$2`, orderID, *latest)
		if err != nil {
			return Totals{}, nil, err
		}
		defer rows.Close()
		var exact time.Time
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				return Totals{}, nil, err
			}
			at, parseErr := time.Parse(time.RFC3339Nano, raw)
			if parseErr != nil {
				return Totals{}, nil, ErrInvalid
			}
			if exact.IsZero() || at.After(exact) {
				exact = at
			}
		}
		if err = rows.Err(); err != nil {
			return Totals{}, nil, err
		}
		if exact.IsZero() {
			return Totals{}, nil, ErrInvalid
		}
		latest = &exact
	}
	return t, latest, nil
}

// Commit the stop before returning an error. A rollback here would forget the
// discrepancy and could let another order dispatch after a prior settlement.
func (s *PostgresStore) block(ctx context.Context, tx pgx.Tx, o Order, reason string, input any) error {
	body, err := json.Marshal(input)
	// Malformed/oversized observations must not prevent the durable stop.
	if err != nil || len(body) > 4096 {
		body = []byte(`{}`)
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_reconciliation_blocks(order_id,owner_id,financial_account_id,reason,payload) VALUES($1,$2,$3,$4,$5)`, o.ID, o.Request.OwnerID, o.Request.AccountID, reason, body)
	if err != nil {
		return mapError(err)
	}
	// Caller commits only after any required current-access recheck. Legacy
	// single-evidence entry points also commit this durable quarantine.
	return ErrReconciliationBlocked
}
