package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5"
)

// ScheduledProposal is untrusted fresh LIVE proposal input, not a persisted
// PAPER/SHADOW outcome or permission to send. Financial scope, product, pilot
// limits, expiry and stable client identity are derived by the control plane.
// No runtime, scheduler, model or provider caller is wired to this intake.
type ScheduledProposal struct {
	MandateApprovalID, MandateID     string
	MandateVersion                   int
	ScheduledFor                     time.Time
	SourceMode                       string
	AIConnectionID, AIModelID        string
	Side, BaseSize, LimitPrice       string
	FeeAllowanceUSD, MaximumDebitUSD string
}

func canonicalScheduledSlot(t time.Time) bool {
	_, offset := t.Zone()
	return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 && offset == 0 && t.Nanosecond() == 0
}

// Slots use the immutable effective-from instant, rounded up to UTC seconds, as
// their anchor. A delay or renewable consent cannot shift the cadence or slot.
func scheduledProposalDeadline(slot, now, effectiveFrom time.Time, intervalMinutes int, limits ...time.Time) (time.Time, error) {
	if !canonicalScheduledSlot(slot) || now.IsZero() || effectiveFrom.IsZero() || intervalMinutes < 30 || intervalMinutes > 1440 {
		return time.Time{}, ErrNotAuthorized
	}
	anchor := effectiveFrom.UTC().Truncate(time.Second)
	if anchor.Before(effectiveFrom) {
		anchor = anchor.Add(time.Second)
	}
	seconds := slot.Unix() - anchor.Unix()
	if seconds < 0 || seconds%int64(intervalMinutes*60) != 0 || slot.After(now) || effectiveFrom.After(now) {
		return time.Time{}, ErrNotAuthorized
	}
	until := slot.Add(2 * time.Minute)
	if next := slot.Add(time.Duration(intervalMinutes) * time.Minute); next.Before(until) {
		until = next
	}
	for _, limit := range limits {
		if limit.Before(until) {
			until = limit
		}
	}
	if !until.After(now) {
		return time.Time{}, ErrNotAuthorized
	}
	return until, nil
}

// Custom UUIDv8 identity: the slot, not a renewable consent or supplied client
// key, owns this value. The storage guard uses the identical domain and format.
func scheduledProposalClientID(bucketID, mandateID string, version int, slot time.Time) string {
	key := "arbion-live-scheduled-proposal-v1|" + bucketID + "|" + mandateID + "|" + strconv.Itoa(version) + "|" + slot.UTC().Format("2006-01-02T15:04:05Z")
	h := sha256.Sum256([]byte(key))
	h[6], h[8] = (h[6]&0x0f)|0x80, (h[8]&0x0f)|0xa0
	encoded := hex.EncodeToString(h[:16])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

// PrepareScheduledProposal atomically freezes one order and its LIVE source per
// immutable cadence slot. Exact replay is historical recovery even after expiry
// or revocation; changed terms, consent or source never replace the first record.
func (s *PostgresStore) PrepareScheduledProposal(ctx context.Context, ownerID string, p ScheduledProposal) (Order, error) {
	if !validUUID(ownerID) || !validUUID(p.MandateApprovalID) || !validUUID(p.MandateID) || p.MandateVersion < 1 || !canonicalScheduledSlot(p.ScheduledFor) {
		return Order{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer rollback(tx)
	var pilot PilotAllocation
	var mandateID string
	var version int
	err = tx.QueryRow(ctx, `SELECT a.mandate_id::text,a.mandate_version,p.owner_id::text,p.financial_account_id::text,
	 p.provider_connection_id::text,p.capital_bucket_id::text,p.product_id,p.initial_cash_usd,p.maximum_order_usd,p.expires_at
	 FROM execution_mandate_approvals a JOIN execution_pilot_allocations p
	 ON p.owner_id=a.owner_id AND p.financial_account_id=a.financial_account_id AND p.capital_bucket_id=a.capital_bucket_id
	 WHERE a.id=$1 AND a.owner_id=$2`, p.MandateApprovalID, ownerID).
		Scan(&mandateID, &version, &pilot.OwnerID, &pilot.AccountID, &pilot.ConnectionID, &pilot.CapitalBucketID, &pilot.ProductID, &pilot.InitialCashUSD, &pilot.Limits.MaximumOrderUSD, &pilot.Limits.ExpiresAt)
	if err != nil {
		return Order{}, mapError(err)
	}
	pilot.Limits.ExpiresAt = pilot.Limits.ExpiresAt.UTC()
	if saved, err := readScheduledProposal(ctx, tx, ownerID, pilot.CapitalBucketID, p); !errors.Is(err, ErrNotFound) {
		return saved, err
	}
	// Lock only the standard owner/entitlement/account prefix before rereading
	// the slot. A concurrent exact winner remains recoverable after a lock wait,
	// without renewing or revalidating its historical authority.
	if _, err = tx.Exec(ctx, `SELECT lock_execution_scheduled_proposal_account($1,$2)`, ownerID, pilot.AccountID); err != nil {
		return Order{}, mapError(err)
	}
	if saved, err := readScheduledProposal(ctx, tx, ownerID, pilot.CapitalBucketID, p); !errors.Is(err, ErrNotFound) {
		return saved, err
	}
	if mandateID != p.MandateID || version != p.MandateVersion || p.SourceMode != "LIVE" || !validUUID(p.AIConnectionID) || !executionModelIDPattern.MatchString(p.AIModelID) {
		return Order{}, ErrNotAuthorized
	}
	r := Request{OwnerID: ownerID, AccountID: pilot.AccountID, ConnectionID: pilot.ConnectionID, CapitalBucketID: pilot.CapitalBucketID,
		ClientOrderID: scheduledProposalClientID(pilot.CapitalBucketID, mandateID, version, p.ScheduledFor), ProductID: pilot.ProductID,
		Side: p.Side, BaseSize: p.BaseSize, LimitPrice: p.LimitPrice, FeeAllowanceUSD: p.FeeAllowanceUSD, MaximumDebitUSD: p.MaximumDebitUSD,
		PilotLimits: &pilot.Limits, MandateApprovalID: p.MandateApprovalID}
	if _, err = requestDigest(r); err != nil {
		return Order{}, err
	}
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, ownerID, pilot.CapitalBucketID).Scan(&generation); err != nil {
		return Order{}, mapError(err)
	}
	consent, err := loadMandateConsent(ctx, tx, Order{Request: r}, generation)
	if err != nil {
		return Order{}, err
	}
	var body []byte
	if err = tx.QueryRow(ctx, `SELECT snapshot FROM automation_mandate_versions WHERE mandate_id=$1 AND version_number=$2`, mandateID, version).Scan(&body); err != nil {
		return Order{}, mapError(err)
	}
	var snapshot mandatePolicySnapshot
	if !decodeMandateJSON(body, &snapshot) || snapshot.AIProviderConnectionID == nil || *snapshot.AIProviderConnectionID != p.AIConnectionID || snapshot.AIModelID == nil || *snapshot.AIModelID != p.AIModelID {
		return Order{}, ErrNotAuthorized
	}
	schedule, err := automation.ParseScheduleConditions(snapshot.ScheduleConditions)
	if err != nil || !schedule.Enabled || schedule.Session != "CONTINUOUS" {
		return Order{}, ErrNotAuthorized
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Order{}, err
	}
	limits := []time.Time{consent.expiresAt, pilot.Limits.ExpiresAt}
	if consent.mandate.EffectiveUntil != nil {
		limits = append(limits, *consent.mandate.EffectiveUntil)
	}
	until, err := scheduledProposalDeadline(p.ScheduledFor, now, snapshot.EffectiveFrom, schedule.IntervalMinutes, limits...)
	if err != nil {
		return Order{}, err
	}
	o, err := insertPreparedOrder(ctx, tx, r)
	if err != nil {
		return Order{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_scheduled_proposals(order_id,owner_id,financial_account_id,provider_connection_id,
	 capital_bucket_id,mandate_approval_id,mandate_id,mandate_version,scheduled_for,source_mode,ai_provider_connection_id,ai_model_id,
	 request_digest,client_order_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		o.ID, ownerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, p.MandateApprovalID, mandateID, version, p.ScheduledFor,
		p.SourceMode, p.AIConnectionID, p.AIModelID, o.RequestDigest, r.ClientOrderID, until)
	if err != nil {
		return Order{}, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Order{}, ErrCommitUnknown
	}
	return o, nil
}

func readScheduledProposal(ctx context.Context, tx pgx.Tx, ownerID, bucketID string, p ScheduledProposal) (Order, error) {
	var orderID, consent, mode, connection, model, digest string
	err := tx.QueryRow(ctx, `SELECT order_id::text,mandate_approval_id::text,source_mode,ai_provider_connection_id::text,ai_model_id,request_digest
	 FROM execution_scheduled_proposals WHERE owner_id=$1 AND capital_bucket_id=$2 AND mandate_id=$3 AND mandate_version=$4 AND scheduled_for=$5`,
		ownerID, bucketID, p.MandateID, p.MandateVersion, p.ScheduledFor).Scan(&orderID, &consent, &mode, &connection, &model, &digest)
	if err != nil {
		return Order{}, mapError(err)
	}
	o, err := readOrder(ctx, tx, ownerID, "id", orderID)
	if err != nil {
		return Order{}, err
	}
	r := o.Request
	if consent != p.MandateApprovalID || mode != p.SourceMode || connection != p.AIConnectionID || model != p.AIModelID || digest != o.RequestDigest || r.MandateApprovalID != p.MandateApprovalID ||
		r.Side != p.Side || r.BaseSize != p.BaseSize || r.LimitPrice != p.LimitPrice || r.FeeAllowanceUSD != p.FeeAllowanceUSD || r.MaximumDebitUSD != p.MaximumDebitUSD {
		return Order{}, ErrConflict
	}
	return o, nil
}

func checkScheduledProposal(ctx context.Context, tx pgx.Tx, o Order) (time.Time, error) {
	var expires time.Time
	if err := tx.QueryRow(ctx, `SELECT check_execution_scheduled_proposal($1)`, o.ID).Scan(&expires); err != nil {
		return time.Time{}, mapError(err)
	}
	return expires, nil
}
