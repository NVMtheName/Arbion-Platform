package strategy

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrCommitAccessRevoked         = errors.New("current non-live automation access is unavailable")
	ErrCommitConnectionUnavailable = errors.New("current non-live connection access is unavailable")
)

type nonLiveCommitAccess struct {
	startsAt               time.Time
	expiresAt              *time.Time
	authorizationExpiresAt *time.Time
	mandateWindow          *aiCommitMandateWindow
}

// lockNonLiveCommitAccess repeats the existing founder-only automation policy
// against current persisted state, not the principal captured before evaluation.
// It holds shared row locks through commit in owner -> entitlement -> account ->
// sorted connections order, before mandate/bucket/runtime/portfolio locks. Normal
// account sync and retirement already write accounts before their connection.
// All reads are credential-free. No browser session or provider call belongs here.
func lockNonLiveCommitAccess(ctx context.Context, tx pgx.Tx, instance Instance, expectedProvider string) (nonLiveCommitAccess, error) {
	var access nonLiveCommitAccess
	var aiConnectionID *string
	if instance.StrategyIdentifier == "ai_shadow" {
		var from, until []byte
		err := tx.QueryRow(ctx, `SELECT v.snapshot->>'ai_provider_connection_id',
			v.snapshot->'effective_from',v.snapshot->'effective_until'
			FROM automation_mandate_versions v JOIN automation_mandates m ON m.id=v.mandate_id
			WHERE m.id=$1 AND m.user_id=$2 AND v.version_number=$3`,
			instance.AutomationMandateID, instance.UserID, instance.MandateVersion).Scan(&aiConnectionID, &from, &until)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (aiConnectionID == nil || *aiConnectionID == "")) {
			return access, ErrEvaluationConfigurationChanged
		}
		if err != nil {
			return access, err
		}
		access.mandateWindow, err = parseAICommitMandateWindow(from, until)
		if err != nil {
			return access, err
		}
	}
	if err := access.lockOwner(ctx, tx, instance.UserID); err != nil {
		return access, err
	}

	var financialConnectionID, provider, status string
	err := tx.QueryRow(ctx, `SELECT provider_connection_id::text,provider_name,status
		FROM financial_accounts WHERE id=$1 AND user_id=$2 FOR SHARE`, instance.FinancialAccountID, instance.UserID).Scan(&financialConnectionID, &provider, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (status != "active" || (expectedProvider != "" && provider != expectedProvider))) {
		return access, ErrCommitConnectionUnavailable
	}
	if err != nil {
		return access, err
	}
	ids := []string{financialConnectionID}
	if aiConnectionID != nil {
		if *aiConnectionID == financialConnectionID {
			return access, ErrCommitConnectionUnavailable
		}
		ids = append(ids, *aiConnectionID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var category, connectionProvider string
		var expires *time.Time
		err = tx.QueryRow(ctx, `SELECT provider_category,provider_name,status,authorization_expires_at
			FROM provider_connections WHERE id=$1 AND user_id=$2 FOR SHARE`, id, instance.UserID).Scan(&category, &connectionProvider, &status, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return access, ErrCommitConnectionUnavailable
		}
		if err != nil {
			return access, err
		}
		if status != "active" || (id == financialConnectionID && (category != "financial" || connectionProvider != provider)) || (id != financialConnectionID && category != "ai") {
			return access, ErrCommitConnectionUnavailable
		}
		if expires != nil && (access.authorizationExpiresAt == nil || expires.Before(*access.authorizationExpiresAt)) {
			access.authorizationExpiresAt = expires
		}
	}
	return access, access.checkTime(ctx, tx)
}

// lockOwner repeats the existing active-owner/founder policy without relying
// on the caller's earlier principal. Initialization and accepted evaluations
// take these shared locks before account/provider locks and retain them through
// their transaction.
// It deliberately does not grant access from role or other product tiers.
func (access *nonLiveCommitAccess) lockOwner(ctx context.Context, tx pgx.Tx, userID string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
		return ErrCommitAccessRevoked
	}
	if err != nil {
		return err
	}
	// Missing authority fails closed; a concurrent new grant cannot authorize a
	// decision that this transaction has already rejected. An existing grant is
	// locked against both revocation and deletion, including direct SQL updates.
	err = tx.QueryRow(ctx, `SELECT status,starts_at,expires_at FROM user_entitlements
		WHERE user_id=$1 AND entitlement_key='founder' FOR SHARE`, userID).Scan(&status, &access.startsAt, &access.expiresAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
		return ErrCommitAccessRevoked
	}
	return err
}

// now() is the transaction start time in PostgreSQL; using it would extend an
// authorization across a lock wait. Use the database wall clock after locks and
// again immediately before commit. OAuth access-token expiry is intentionally
// not an authorization revocation: existing refresh handling remains separate.
func (a nonLiveCommitAccess) checkTime(ctx context.Context, tx pgx.Tx) error {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if a.startsAt.IsZero() || now.Before(a.startsAt) || (a.expiresAt != nil && !now.Before(*a.expiresAt)) {
		return ErrCommitAccessRevoked
	}
	if a.authorizationExpiresAt != nil && !now.Before(*a.authorizationExpiresAt) {
		return ErrCommitConnectionUnavailable
	}
	if a.mandateWindow != nil && !a.mandateWindow.current(now) {
		return ErrCommitMandateWindowClosed
	}
	return nil
}
