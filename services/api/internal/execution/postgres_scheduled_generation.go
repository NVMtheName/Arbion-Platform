package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func validScheduledGenerationSlot(ownerID string, slot ScheduledGenerationSlot, requireConsent bool) bool {
	return validUUID(ownerID) && validUUID(slot.MandateID) && slot.MandateVersion > 0 && canonicalScheduledSlot(slot.ScheduledFor) &&
		(!requireConsent || validUUID(slot.MandateApprovalID))
}

// ReadScheduledGeneration is historical recovery, not permission to invoke a
// model. Consent renewal cannot change the immutable mandate/version/slot key.
func (s *PostgresStore) ReadScheduledGeneration(ctx context.Context, ownerID string, slot ScheduledGenerationSlot) (ScheduledGenerationReceipt, error) {
	if !validScheduledGenerationSlot(ownerID, slot, false) {
		return ScheduledGenerationReceipt{}, ErrInvalid
	}
	return readScheduledGeneration(ctx, s.db, ownerID, slot, "")
}

type generationReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readScheduledGeneration(ctx context.Context, db generationReader, ownerID string, slot ScheduledGenerationSlot, claimID string) (ScheduledGenerationReceipt, error) {
	var receipt ScheduledGenerationReceipt
	var facts, market, result []byte
	var input string
	err := db.QueryRow(ctx, `SELECT c.id::text,c.mandate_approval_id::text,c.mandate_id::text,c.mandate_version,c.scheduled_for,
	 c.facts,c.market,c.model_input,c.input_digest,r.result
	 FROM execution_generation_claims c LEFT JOIN execution_generation_results r ON r.claim_id=c.id
	 WHERE c.owner_id=$1 AND (($2::uuid IS NOT NULL AND c.id=$2) OR
	 ($2::uuid IS NULL AND c.mandate_id=$3 AND c.mandate_version=$4 AND c.scheduled_for=$5))`,
		ownerID, nullableGenerationID(claimID), nullableGenerationID(slot.MandateID), slot.MandateVersion, slot.ScheduledFor).
		Scan(&receipt.Claim.ID, &receipt.Claim.Slot.MandateApprovalID, &receipt.Claim.Slot.MandateID, &receipt.Claim.Slot.MandateVersion,
			&receipt.Claim.Slot.ScheduledFor, &facts, &market, &input, &receipt.Claim.InputDigest, &result)
	if err != nil {
		return ScheduledGenerationReceipt{}, mapError(err)
	}
	digest := sha256.Sum256([]byte(input))
	if hex.EncodeToString(digest[:]) != receipt.Claim.InputDigest || !decodeMandateJSON(facts, &receipt.Claim.Facts) ||
		!decodeMandateJSON(market, &receipt.Claim.Market) || !decodeMandateJSON([]byte(input), &receipt.Claim.Input) || receipt.Claim.Facts.Pilot.OwnerID != ownerID {
		return ScheduledGenerationReceipt{}, ErrConflict
	}
	// Reconstruct against the frozen observation time, never today's authority
	// or prices. Historical input must remain the exact pilot-only projection;
	// a self-consistent digest alone cannot validate caller-authored JSON.
	expectedInput, expectedDigest, err := buildScheduledGenerationInput(receipt.Claim.Facts, receipt.Claim.Market, receipt.Claim.Facts.ObservedAt)
	if err != nil || expectedDigest != receipt.Claim.InputDigest || !sameGenerationInput(expectedInput, receipt.Claim.Input) {
		return ScheduledGenerationReceipt{}, ErrConflict
	}
	receipt.Claim.Slot.ScheduledFor = receipt.Claim.Slot.ScheduledFor.UTC()
	if len(result) != 0 {
		receipt.Result = new(ScheduledGenerationResult)
		if !decodeMandateJSON(result, receipt.Result) {
			return ScheduledGenerationReceipt{}, ErrConflict
		}
	}
	return receipt, nil
}

func nullableGenerationID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// GenerationFacts is a read-only, current control-plane projection. Claim
// rebuilds it under the same locks; this earlier read cannot authorize a call.
func (s *PostgresStore) GenerationFacts(ctx context.Context, ownerID string, slot ScheduledGenerationSlot) (ScheduledGenerationFacts, error) {
	if !validScheduledGenerationSlot(ownerID, slot, true) {
		return ScheduledGenerationFacts{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ScheduledGenerationFacts{}, err
	}
	defer rollback(tx)
	return currentScheduledGenerationFacts(ctx, tx, ownerID, slot)
}

func currentScheduledGenerationFacts(ctx context.Context, tx pgx.Tx, ownerID string, slot ScheduledGenerationSlot) (ScheduledGenerationFacts, error) {
	var facts ScheduledGenerationFacts
	var mandateID string
	var version int
	p := &facts.Pilot
	err := tx.QueryRow(ctx, `SELECT a.mandate_id::text,a.mandate_version,p.owner_id::text,p.financial_account_id::text,
	 p.provider_connection_id::text,p.capital_bucket_id::text,p.product_id,p.initial_cash_usd,p.maximum_order_usd,p.expires_at
	 FROM execution_mandate_approvals a JOIN execution_pilot_allocations p
	 ON p.owner_id=a.owner_id AND p.financial_account_id=a.financial_account_id AND p.capital_bucket_id=a.capital_bucket_id
	 WHERE a.id=$1 AND a.owner_id=$2`, slot.MandateApprovalID, ownerID).
		Scan(&mandateID, &version, &p.OwnerID, &p.AccountID, &p.ConnectionID, &p.CapitalBucketID, &p.ProductID, &p.InitialCashUSD, &p.Limits.MaximumOrderUSD, &p.Limits.ExpiresAt)
	if err != nil {
		return facts, mapError(err)
	}
	if mandateID != slot.MandateID || version != slot.MandateVersion {
		return facts, ErrNotAuthorized
	}
	p.Limits.ExpiresAt = p.Limits.ExpiresAt.UTC()
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, ownerID, p.CapitalBucketID).Scan(&generation); err != nil {
		return facts, mapError(err)
	}
	_, err = loadMandateConsent(ctx, tx, Order{Request: Request{OwnerID: ownerID, AccountID: p.AccountID, ConnectionID: p.ConnectionID,
		CapitalBucketID: p.CapitalBucketID, MandateApprovalID: slot.MandateApprovalID}}, generation)
	if err != nil {
		return facts, err
	}
	err = tx.QueryRow(ctx, `SELECT snapshot->>'ai_provider_connection_id',snapshot->>'ai_model_id',
	 snapshot->'strategy_parameters'->>'objective',snapshot->'strategy_parameters'->>'max_proposal_notional'
	 FROM automation_mandate_versions WHERE mandate_id=$1 AND version_number=$2`, slot.MandateID, slot.MandateVersion).
		Scan(&facts.AIConnectionID, &facts.AIModelID, &facts.Objective, &facts.MaxProposalNotional)
	if err != nil {
		return facts, mapError(err)
	}
	facts.Profile, err = scheduledGenerationProfile(facts.AIModelID)
	if err != nil {
		return facts, err
	}
	if err = tx.QueryRow(ctx, `SELECT execution_scheduled_proposal_deadline($1,$2,$3,$4,$5,$6,$7,$8)`, slot.MandateApprovalID,
		ownerID, p.CapitalBucketID, slot.MandateID, slot.MandateVersion, slot.ScheduledFor, facts.AIConnectionID, facts.AIModelID).Scan(&facts.ExpiresAt); err != nil {
		return facts, mapError(err)
	}
	if _, err = tx.Exec(ctx, `SELECT check_execution_generation_funding($1,$2)`, ownerID, p.AccountID); err != nil {
		return facts, mapError(err)
	}
	err = tx.QueryRow(ctx, `SELECT cash_usd::text,base_quantity::text,clock_timestamp() FROM execution_pilot_balance($1)`, p.AccountID).
		Scan(&facts.CashUSD, &facts.AcquiredBase, &facts.ObservedAt)
	if err != nil {
		return facts, mapError(err)
	}
	facts.ExpiresAt, facts.ObservedAt = facts.ExpiresAt.UTC(), facts.ObservedAt.UTC()
	if !facts.ExpiresAt.After(facts.ObservedAt) {
		return facts, ErrNotAuthorized
	}
	return facts, nil
}

// ClaimScheduledGeneration admits only a newly committed durable claim. The
// caller MUST NOT enter a model callback on false or ANY error, including an
// ambiguous commit. A saved claim without a result is not a lease to reclaim.
func (s *PostgresStore) ClaimScheduledGeneration(ctx context.Context, ownerID string, slot ScheduledGenerationSlot, market ScheduledGenerationMarket) (ScheduledGenerationClaim, bool, error) {
	if !validScheduledGenerationSlot(ownerID, slot, true) {
		return ScheduledGenerationClaim{}, false, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ScheduledGenerationClaim{}, false, err
	}
	defer rollback(tx)
	read := func() (ScheduledGenerationClaim, bool, error) {
		r, err := readScheduledGeneration(ctx, tx, ownerID, slot, "")
		if err == nil && r.Claim.Slot.MandateApprovalID != slot.MandateApprovalID {
			err = ErrConflict
		}
		return r.Claim, false, err
	}
	if claim, admitted, err := read(); !errors.Is(err, ErrNotFound) {
		return claim, admitted, err
	}
	var accountID string
	if err = tx.QueryRow(ctx, `SELECT financial_account_id::text FROM execution_mandate_approvals WHERE id=$1 AND owner_id=$2`, slot.MandateApprovalID, ownerID).Scan(&accountID); err != nil {
		return ScheduledGenerationClaim{}, false, mapError(err)
	}
	if _, err = tx.Exec(ctx, `SELECT lock_execution_scheduled_proposal_account($1,$2)`, ownerID, accountID); err != nil {
		return ScheduledGenerationClaim{}, false, mapError(err)
	}
	if claim, admitted, err := read(); !errors.Is(err, ErrNotFound) {
		return claim, admitted, err
	}
	facts, err := currentScheduledGenerationFacts(ctx, tx, ownerID, slot)
	if err != nil {
		return ScheduledGenerationClaim{}, false, err
	}
	input, digest, err := buildScheduledGenerationInput(facts, market, facts.ObservedAt)
	if err != nil {
		return ScheduledGenerationClaim{}, false, err
	}
	claim := ScheduledGenerationClaim{Slot: slot, Facts: facts, Market: market, Input: input, InputDigest: digest}
	factsJSON, _ := json.Marshal(facts)
	marketJSON, _ := json.Marshal(market)
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ScheduledGenerationClaim{}, false, ErrInvalid
	}
	err = tx.QueryRow(ctx, `INSERT INTO execution_generation_claims(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,
	 mandate_approval_id,mandate_id,mandate_version,scheduled_for,ai_provider_connection_id,ai_model_id,facts,market,model_input,input_digest,expires_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id::text`, ownerID, facts.Pilot.AccountID, facts.Pilot.ConnectionID,
		facts.Pilot.CapitalBucketID, slot.MandateApprovalID, slot.MandateID, slot.MandateVersion, slot.ScheduledFor, facts.AIConnectionID, facts.AIModelID,
		factsJSON, marketJSON, string(inputJSON), digest, facts.ExpiresAt).Scan(&claim.ID)
	if err != nil {
		return ScheduledGenerationClaim{}, false, mapError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return claim, false, ErrCommitUnknown
	}
	return claim, true, nil
}

// RecordScheduledGenerationResult saves terminal evidence even after authority
// expires. It cannot authorize intake or regenerate a missing result. PROPOSE
// still has to pass existing scheduled intake and all later execution controls.
func (s *PostgresStore) RecordScheduledGenerationResult(ctx context.Context, ownerID, claimID string, result ScheduledGenerationResult) (ScheduledGenerationReceipt, error) {
	if !validUUID(ownerID) || !validUUID(claimID) {
		return ScheduledGenerationReceipt{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ScheduledGenerationReceipt{}, err
	}
	defer rollback(tx)
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM execution_generation_claims WHERE id=$1 AND owner_id=$2 FOR UPDATE`, claimID, ownerID).Scan(&locked); err != nil {
		return ScheduledGenerationReceipt{}, mapError(err)
	}
	receipt, err := readScheduledGeneration(ctx, tx, ownerID, ScheduledGenerationSlot{}, claimID)
	if err != nil {
		return ScheduledGenerationReceipt{}, err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return ScheduledGenerationReceipt{}, ErrInvalid
	}
	if receipt.Result != nil {
		saved, _ := json.Marshal(receipt.Result)
		if string(saved) != string(body) {
			return ScheduledGenerationReceipt{}, ErrConflict
		}
		return receipt, nil
	}
	if err = validateScheduledGenerationResult(receipt.Claim, result); err != nil {
		return ScheduledGenerationReceipt{}, err
	}
	if result.Outcome == "PROPOSE" {
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return ScheduledGenerationReceipt{}, err
		}
		if !receipt.Claim.Facts.ExpiresAt.After(now) || result.Market == nil || validateGenerationMarket(receipt.Claim.Facts.Pilot.ProductID, *result.Market, now) != nil {
			return ScheduledGenerationReceipt{}, ErrNotAuthorized
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO execution_generation_results(claim_id,owner_id,outcome,result) VALUES($1,$2,$3,$4)`, claimID, ownerID, result.Outcome, body)
	if err != nil {
		return ScheduledGenerationReceipt{}, mapError(err)
	}
	receipt.Result = &result
	if err = tx.Commit(ctx); err != nil {
		return receipt, ErrCommitUnknown
	}
	return receipt, nil
}
