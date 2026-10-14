package execution

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func validScheduledWorkerScope(s ScheduledScope) bool {
	o := s.Owner
	return validUUID(o.OwnerID) && validUUID(o.AccountID) && validUUID(o.ConnectionID) && validUUID(o.CapitalBucketID) &&
		productPattern.MatchString(o.ProductID) && o.ProductID != "USD-USD" && validPilotLimits(&o.PilotLimits) &&
		validUUID(s.MandateApprovalID) && validUUID(s.MandateID) && s.MandateVersion > 0
}

// claimScheduledWork chooses one historical lifecycle item OR commits one new
// admission. An admission is never a lease: only a positively acknowledged new
// commit allows RunSlot, and no later caller retries an admitted slot. All
// locks end before the worker enters any market, model, or broker callback.
func (s *PostgresStore) claimScheduledWork(ctx context.Context, scope ScheduledScope) (scheduledWorkerWork, error) {
	if s == nil || s.db == nil || !validScheduledWorkerScope(scope) {
		return scheduledWorkerWork{}, ErrNotAuthorized
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return scheduledWorkerWork{}, err
	}
	defer rollback(tx)
	o := scope.Owner
	if _, err = tx.Exec(ctx, `SELECT lock_execution_scheduled_proposal_account($1,$2)`, o.OwnerID, o.AccountID); err != nil {
		return scheduledWorkerWork{}, mapError(err)
	}
	// Admission itself independently requires present owner access. Recovery is
	// deliberately independent of current mandate validity and pilot expiry.
	var ownerAllowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_entitlements e ON e.user_id=u.id
	 WHERE u.id=$1 AND u.status='active' AND e.entitlement_key='founder' AND e.status='active'
	 AND e.starts_at<=clock_timestamp() AND (e.expires_at IS NULL OR e.expires_at>clock_timestamp()))`, o.OwnerID).Scan(&ownerAllowed); err != nil {
		return scheduledWorkerWork{}, err
	}
	if !ownerAllowed {
		return scheduledWorkerWork{}, ErrNotAuthorized
	}
	if orderID, err := scheduledWorkerHistoricalOrder(ctx, tx, scope); err != nil || orderID != "" {
		return scheduledWorkerWork{OrderID: orderID}, err
	}
	var due *time.Time
	err = tx.QueryRow(ctx, `SELECT execution_scheduled_worker_slot($1,$2,$3,$4,$5,$6,$7)`, o.OwnerID, o.AccountID,
		o.ConnectionID, o.CapitalBucketID, scope.MandateApprovalID, scope.MandateID, scope.MandateVersion).Scan(&due)
	if err != nil {
		return scheduledWorkerWork{}, mapError(err)
	}
	if due == nil {
		return scheduledWorkerWork{}, nil
	}
	work := scheduledWorkerWork{ScheduledFor: due.UTC()}
	slot := ScheduledGenerationSlot{MandateApprovalID: scope.MandateApprovalID, MandateID: scope.MandateID,
		MandateVersion: scope.MandateVersion, ScheduledFor: work.ScheduledFor}
	facts, err := currentScheduledGenerationFacts(ctx, tx, o.OwnerID, slot)
	if err != nil {
		return scheduledWorkerWork{}, err
	}
	if !(scopedGenerationStore{scope: scope}).matches(facts, slot) {
		return scheduledWorkerWork{}, ErrNotAuthorized
	}
	// Slot identity excludes renewable consent. A renewed consent cannot turn
	// an old admission into another model or provider attempt.
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_scheduled_runs
	 WHERE owner_id=$1 AND mandate_id=$2 AND mandate_version=$3 AND scheduled_for=$4)`, o.OwnerID, scope.MandateID, scope.MandateVersion, work.ScheduledFor).Scan(&exists); err != nil {
		return scheduledWorkerWork{}, err
	}
	if exists {
		return work, nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_scheduled_runs(owner_id,financial_account_id,provider_connection_id,
	 capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for,expires_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, o.OwnerID, o.AccountID, o.ConnectionID, o.CapitalBucketID,
		scope.MandateApprovalID, scope.MandateID, scope.MandateVersion, work.ScheduledFor, facts.ExpiresAt)
	if err != nil {
		return scheduledWorkerWork{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return work, ErrCommitUnknown // false, even if the server actually committed
	}
	work.Admitted = true
	return work, nil
}

// Validate every historical candidate using the existing complete projection,
// including immutable source, request digest, exact holds and closure receipts.
// A terminal order without account settlement is still unresolved. Neither an
// absent broker record nor an expired consent releases a claim.
func scheduledWorkerHistoricalOrder(ctx context.Context, tx pgx.Tx, scope ScheduledScope) (string, error) {
	type candidate struct {
		id      string
		bound   bool
		expired bool
	}
	rows, err := tx.Query(ctx, `SELECT o.id::text,COALESCE(sp.owner_id=$2 AND sp.financial_account_id=$1
	 AND sp.provider_connection_id=$3 AND sp.capital_bucket_id=$4 AND sp.mandate_approval_id=$5
	 AND sp.mandate_id=$6 AND sp.mandate_version=$7 AND sp.request_digest=o.request_digest AND sp.source_mode='LIVE',false),
	 COALESCE(sp.expires_at<=clock_timestamp(),false)
	 FROM execution_orders o LEFT JOIN execution_scheduled_proposals sp ON sp.order_id=o.id
	 WHERE o.financial_account_id=$1 ORDER BY EXISTS(SELECT 1 FROM execution_dispatch_attempts a WHERE a.order_id=o.id) DESC,o.created_at,o.id`,
		scope.Owner.AccountID, scope.Owner.OwnerID, scope.Owner.ConnectionID, scope.Owner.CapitalBucketID,
		scope.MandateApprovalID, scope.MandateID, scope.MandateVersion)
	if err != nil {
		return "", err
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.bound, &c.expired); err != nil {
			rows.Close()
			return "", err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return "", err
	}
	reader := NewPostgresStore(tx)
	var pending string
	for _, c := range candidates {
		order, err := reader.ReadOwnerOrder(ctx, scope.Owner.OwnerID, c.id)
		if err != nil || order.State == "UNAVAILABLE" {
			return "", ErrUnreconciled
		}
		if order.State == "SETTLED" || order.State == "NOT_SENT" {
			continue // exact verified closure, not merely a missing active hold
		}
		if !c.bound || order.AccountID != scope.Owner.AccountID || order.ConnectionID != scope.Owner.ConnectionID ||
			order.CapitalBucketID != scope.Owner.CapitalBucketID || order.ProductID != scope.Owner.ProductID ||
			!samePilotLimits(order.PilotLimits, scope.Owner.PilotLimits) {
			return "", ErrNotAuthorized
		}
		if !order.Attempted && order.State == "PREPARED" && c.expired {
			// PREPARED's verified projection also proves no authorization,
			// reservation, hold, broker fact or closure. Never submit it later.
			continue
		}
		if pending != "" {
			return "", ErrUnreconciled
		}
		pending = order.ID
	}
	// Orphan/foreign active holds are never assumed safe just because no scoped
	// order was found. The original account fence is held throughout this read.
	var foreignHold bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_account_holds WHERE financial_account_id=$1 AND order_id::text<>$2)
	 OR EXISTS(SELECT 1 FROM execution_capital_reservations WHERE financial_account_id=$1 AND released_at IS NULL AND order_id::text<>$2)`, scope.Owner.AccountID, pending).Scan(&foreignHold)
	if err != nil {
		return "", err
	}
	if foreignHold {
		return "", ErrUnreconciled
	}
	return pending, nil
}
