package execution

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5"
)

// OwnerCommissioningTerms are selected by trusted composition, never a model,
// request body or browser session. No default portfolio, pair, budget, model,
// objective, cadence, risk policy or expiry is chosen by this boundary.
// Preparing these terms is not owner consent or permission to execute.
type OwnerCommissioningTerms struct {
	Pilot                                                              PilotAllocation
	AIConnectionID, AIModelID, Objective                               string
	IntervalMinutes, MaxTradesPerDay                                   int
	MaxCapitalDeployedUSD, MaxSinglePositionUSD, MinimumCashReserveUSD string
	EffectiveFrom                                                      time.Time
}

type OwnerCommissioningReview struct {
	Terms          OwnerCommissioningTerms
	TermsDigest    string
	MandateID      string
	MandateVersion int
}

// A receipt is immutable recovery evidence, not proof of current authorization.
// SnapshotDigest is the database's exact digest used by existing fresh consent.
type OwnerCommissioningReceipt struct {
	OwnerCommissioningReview
	SnapshotDigest string
}

type OwnerCommissioning struct {
	store    *PostgresStore
	terms    OwnerCommissioningTerms
	review   OwnerCommissioningReview
	snapshot []byte
	stepUp   ExecutionStepUp
}

func NewOwnerCommissioning(store *PostgresStore, terms OwnerCommissioningTerms, stepUp ExecutionStepUp) (*OwnerCommissioning, error) {
	terms.EffectiveFrom = terms.EffectiveFrom.UTC()
	terms.Pilot.Limits.ExpiresAt = terms.Pilot.Limits.ExpiresAt.UTC()
	if store == nil || store.db == nil || stepUp == nil || !validOwnerCommissioningTerms(terms) {
		return nil, ErrInvalid
	}
	body, err := json.Marshal(terms)
	if err != nil {
		return nil, ErrInvalid
	}
	digest := sha256.Sum256(append([]byte("arbion-owner-commissioning-terms-v1\x00"), body...))
	review := OwnerCommissioningReview{Terms: terms, TermsDigest: fmt.Sprintf("%x", digest), MandateID: ownerCommissioningMandateID(terms.Pilot.OwnerID, terms.Pilot.AccountID), MandateVersion: 1}
	snapshot, err := json.Marshal(ownerCommissioningSnapshot(terms))
	if err != nil {
		return nil, ErrInvalid
	}
	return &OwnerCommissioning{store: store, terms: terms, review: review, snapshot: snapshot, stepUp: stepUp}, nil
}

func validOwnerCommissioningTerms(t OwnerCommissioningTerms) bool {
	if !validPilotAllocation(t.Pilot) || !validUUID(t.AIConnectionID) || !executionModelIDPattern.MatchString(t.AIModelID) ||
		strings.TrimSpace(t.Objective) == "" || len(t.Objective) > 2000 || t.EffectiveFrom.IsZero() || t.EffectiveFrom.Year() < 1 || t.EffectiveFrom.Year() > 9999 ||
		t.EffectiveFrom.Nanosecond() != 0 || t.Pilot.Limits.ExpiresAt.Nanosecond() != 0 ||
		!t.Pilot.Limits.ExpiresAt.After(t.EffectiveFrom) || t.Pilot.Limits.ExpiresAt.Sub(t.EffectiveFrom) > 24*time.Hour ||
		t.MaxTradesPerDay < 1 || t.MaxTradesPerDay > 48 {
		return false
	}
	schedule := automation.ScheduleConditions{Enabled: true, IntervalMinutes: t.IntervalMinutes, Session: "CONTINUOUS"}
	if automation.ValidateScheduleConditions(schedule) != nil {
		return false
	}
	initial, _ := decimal(t.Pilot.InitialCashUSD, true)
	for i, value := range []string{t.MaxCapitalDeployedUSD, t.MaxSinglePositionUSD, t.MinimumCashReserveUSD} {
		n, ok := decimal(value, i != 2)
		if !ok || n.Cmp(initial) > 0 {
			return false
		}
	}
	return true
}

// Changing terms, clock time or caller request keys cannot mint another pilot
// mandate on the same account. Conflicting terms require an explicit later
// reviewed lifecycle, not replacement/renewal through commissioning.
func ownerCommissioningMandateID(ownerID, accountID string) string {
	h := sha256.Sum256([]byte("arbion-owner-commissioning-mandate-v1\x00" + ownerID + "\x00" + accountID))
	h[6] = (h[6] & 0x0f) | 0x80
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func ownerCommissioningSnapshot(t OwnerCommissioningTerms) mandatePolicySnapshot {
	params, _ := json.Marshal(spotPilotParameters{Profile: coinbaseSpotPilotProfile, Objective: t.Objective, MaxProposalNotional: t.Pilot.Limits.MaximumOrderUSD})
	schedule, _ := json.Marshal(automation.ScheduleConditions{Enabled: true, IntervalMinutes: t.IntervalMinutes, Session: "CONTINUOUS"})
	return mandatePolicySnapshot{FinancialAccountID: t.Pilot.AccountID, AutomationType: "AI_AUTONOMOUS",
		AIProviderConnectionID: &t.AIConnectionID, AIModelID: &t.AIModelID, CapitalBucketID: t.Pilot.CapitalBucketID,
		AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "LIVE", Status: "READY", StrategyParameters: params,
		Risk: automation.RiskPolicy{MaxCapitalDeployed: &t.MaxCapitalDeployedUSD, MaxSinglePositionAmount: &t.MaxSinglePositionUSD,
			MinimumCashReserve: &t.MinimumCashReserveUSD, MaxTradesPerDay: &t.MaxTradesPerDay},
		AllowedUniverse:    automation.Universe{Symbols: []string{strings.TrimSuffix(t.Pilot.ProductID, "-USD")}},
		ProhibitedUniverse: automation.Universe{Symbols: []string{}}, ScheduleConditions: schedule,
		EffectiveFrom: t.EffectiveFrom, EffectiveUntil: &t.Pilot.Limits.ExpiresAt}
}

func (w *OwnerCommissioning) checkOwner(ctx context.Context, p authorization.Principal) error {
	if w == nil || w.store == nil || p.UserID != w.terms.Pilot.OwnerID || p.Entitlement != authorization.EntitlementFounder {
		return ErrNotAuthorized
	}
	var allowed bool
	err := w.store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_entitlements e ON e.user_id=u.id
	 WHERE u.id=$1 AND u.status='active' AND e.entitlement_key='founder' AND e.status='active'
	 AND e.starts_at<=clock_timestamp() AND (e.expires_at IS NULL OR e.expires_at>clock_timestamp()))`, p.UserID).Scan(&allowed)
	if err != nil || !allowed {
		return ErrNotAuthorized
	}
	return nil
}

func (w *OwnerCommissioning) Review(ctx context.Context, p authorization.Principal) (OwnerCommissioningReview, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerCommissioningReview{}, err
	}
	return w.review, nil
}

func (w *OwnerCommissioning) Read(ctx context.Context, p authorization.Principal) (OwnerCommissioningReceipt, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	return w.readReceipt(ctx, w.store.db)
}

// readReceipt deliberately reads original version 1, even if current policy is
// stopped/changed/expired. It never restores that version or infers consent.
func (w *OwnerCommissioning) readReceipt(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (OwnerCommissioningReceipt, error) {
	var owner, digest string
	var matches bool
	err := db.QueryRow(ctx, `SELECT m.user_id::text,
	 COALESCE(encode(sha256(convert_to(v.snapshot::text,'UTF8')),'hex'),''),COALESCE(v.snapshot=$2::jsonb,false)
	 FROM automation_mandates m LEFT JOIN automation_mandate_versions v ON v.mandate_id=m.id AND v.version_number=1
	 WHERE m.id=$1`, w.review.MandateID, w.snapshot).Scan(&owner, &digest, &matches)
	if err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	p := w.terms.Pilot
	if owner != p.OwnerID || !matches {
		return OwnerCommissioningReceipt{}, ErrConflict
	}
	saved, err := readPilotAllocation(ctx, db, p.OwnerID, p.AccountID)
	if err != nil || !samePilotAllocation(saved, p) {
		if err != nil && !errors.Is(err, ErrNotFound) {
			return OwnerCommissioningReceipt{}, err
		}
		return OwnerCommissioningReceipt{}, ErrConflict
	}
	return OwnerCommissioningReceipt{OwnerCommissioningReview: w.review, SnapshotDigest: digest}, nil
}

// lockOwner precedes the account fence and remains held through a new write.
func (w *OwnerCommissioning) lockOwner(ctx context.Context, tx pgx.Tx) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id=$1 FOR SHARE`, w.terms.Pilot.OwnerID).Scan(&status); err != nil || status != "active" {
		return ErrNotAuthorized
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT status='active' AND starts_at<=clock_timestamp()
	 AND (expires_at IS NULL OR expires_at>clock_timestamp()) FROM user_entitlements
	 WHERE user_id=$1 AND entitlement_key='founder' FOR SHARE`, w.terms.Pilot.OwnerID).Scan(&active); err != nil || !active {
		return ErrNotAuthorized
	}
	return nil
}

func (w *OwnerCommissioning) Prepare(ctx context.Context, p authorization.Principal, expectedTermsDigest string) (OwnerCommissioningReceipt, error) {
	if err := w.checkOwner(ctx, p); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	if expectedTermsDigest != w.review.TermsDigest {
		return OwnerCommissioningReceipt{}, ErrConflict
	}
	tx, err := w.store.db.Begin(ctx)
	if err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	defer rollback(tx)
	if err = w.lockOwner(ctx, tx); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	// New dependency attachment shares lifecycle fences with existing connection
	// writers, BEFORE account locking. Do not use LockActive's account SHARE and
	// later upgrade to UPDATE: concurrent commissioners could then deadlock.
	ids := []string{w.terms.Pilot.ConnectionID, w.terms.AIConnectionID}
	sort.Strings(ids)
	for i, id := range ids {
		if i > 0 && id == ids[i-1] {
			continue
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, id); err != nil {
			return OwnerCommissioningReceipt{}, err
		}
	}
	pilot := w.terms.Pilot
	if _, err = tx.Exec(ctx, `SELECT lock_execution_pilot_account($1,$2)`, pilot.OwnerID, pilot.AccountID); err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	// Post-wait exact recovery observes the winning commit. It must precede
	// current expiry/status checks and must never consume another MFA code.
	if receipt, readErr := w.readReceipt(ctx, tx); !errors.Is(readErr, ErrNotFound) {
		return receipt, readErr
	}
	var hasOrders bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution_orders WHERE financial_account_id=$1)`, pilot.AccountID).Scan(&hasOrders); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	if hasOrders {
		return OwnerCommissioningReceipt{}, ErrNotAuthorized
	}
	if _, _, err = registerPilotAllocation(ctx, tx, pilot); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, pilot.OwnerID, pilot.CapitalBucketID).Scan(&generation); err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	var aiActive bool
	if err = tx.QueryRow(ctx, `SELECT status='active' AND provider_category='ai' AND provider_name='openai'
	 AND (authorization_expires_at IS NULL OR authorization_expires_at>clock_timestamp())
	 FROM provider_connections WHERE id=$1 AND user_id=$2 FOR SHARE`, w.terms.AIConnectionID, pilot.OwnerID).Scan(&aiActive); err != nil || !aiActive {
		return OwnerCommissioningReceipt{}, ErrNotAuthorized
	}
	_, err = tx.Exec(ctx, `INSERT INTO automation_mandates(id,user_id,financial_account_id,automation_type,
	 strategy_identifier,ai_provider_connection_id,ai_model_id,capital_bucket_id,autonomy_level,execution_mode,status,current_version,
	 strategy_parameters,risk_parameters,allowed_universe,prohibited_universe,margin_allowed,options_allowed,
	 schedule_conditions,capability_unverified,paper_options_simulation_attested,effective_from,effective_until)
	 SELECT $1,$2,(s->>'financial_account_id')::uuid,s->>'automation_type',NULL,(s->>'ai_provider_connection_id')::uuid,
	 s->>'ai_model_id',(s->>'capital_bucket_id')::uuid,s->>'autonomy_level',s->>'execution_mode',s->>'status',1,
	 s->'strategy_parameters',s->'risk_parameters',s->'allowed_universe',s->'prohibited_universe',false,false,
	 s->'schedule_conditions',false,false,(s->>'effective_from')::timestamptz,(s->>'effective_until')::timestamptz
	 FROM (SELECT $3::jsonb AS s) fixed`, w.review.MandateID, pilot.OwnerID, w.snapshot)
	if err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary)
	 VALUES($1,1,$2,'UI',$3,'{"change":"owner_coinbase_pilot_commissioning_v1"}')`, w.review.MandateID, pilot.OwnerID, w.snapshot)
	if err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	if _, _, err = readCurrentMandate(ctx, tx, pilot.OwnerID, pilot.CapitalBucketID, w.review.MandateID, 1); err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	// AI/policy row locks can wait after the first financial check. Recheck
	// wall-clock financial/owner expiry with all locks held before committing;
	// the earlier check cannot authorize a dependency that expired while waiting.
	if err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, pilot.OwnerID, pilot.CapitalBucketID).Scan(&generation); err != nil {
		return OwnerCommissioningReceipt{}, mapError(err)
	}
	receipt, err := w.readReceipt(ctx, tx)
	if err != nil {
		return OwnerCommissioningReceipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return receipt, ErrCommitUnknown
	}
	return receipt, nil
}

// Approve is deliberately a second command, after immutable preparation has
// committed. The existing consent boundary requires fresh MFA after both pilot
// registration and mandate creation and repeats all current controls itself.
func (w *OwnerCommissioning) Approve(ctx context.Context, p authorization.Principal, expectedTermsDigest, expectedSnapshotDigest, code string) (MandateConsent, error) {
	receipt, err := w.Read(ctx, p)
	if err != nil {
		return MandateConsent{}, err
	}
	if expectedTermsDigest != receipt.TermsDigest || expectedSnapshotDigest != receipt.SnapshotDigest {
		return MandateConsent{}, ErrConflict
	}
	// Avoid consuming MFA for already unavailable policy. This read transaction
	// is not authority; ApproveMandate repeats the same locks after the step-up.
	tx, err := w.store.db.Begin(ctx)
	if err != nil {
		return MandateConsent{}, err
	}
	pilot := w.terms.Pilot
	var generation int64
	err = tx.QueryRow(ctx, `SELECT lock_execution_consent_controls($1,$2)`, pilot.OwnerID, pilot.CapitalBucketID).Scan(&generation)
	if err == nil {
		var digest string
		_, digest, err = readCurrentMandate(ctx, tx, pilot.OwnerID, pilot.CapitalBucketID, receipt.MandateID, 1)
		if err == nil && digest != expectedSnapshotDigest {
			err = ErrConflict
		}
	}
	rollback(tx)
	if err != nil {
		return MandateConsent{}, mapError(err)
	}
	return w.store.ApproveMandate(ctx, p, pilot.CapitalBucketID, receipt.MandateID, 1, receipt.SnapshotDigest, code, w.stepUp)
}

func (w *OwnerCommissioning) ReadConsent(ctx context.Context, p authorization.Principal, consentID string) (MandateConsent, error) {
	receipt, err := w.Read(ctx, p)
	if err != nil {
		return MandateConsent{}, err
	}
	a, err := w.store.ReadMandateConsent(ctx, p.UserID, consentID)
	if err != nil {
		return MandateConsent{}, err
	}
	pilot := w.terms.Pilot
	if a.OwnerID != pilot.OwnerID || a.AccountID != pilot.AccountID || a.ConnectionID != pilot.ConnectionID ||
		a.CapitalBucketID != pilot.CapitalBucketID || a.MandateID != receipt.MandateID || a.MandateVersion != 1 || a.SnapshotDigest != receipt.SnapshotDigest {
		return MandateConsent{}, ErrNotFound
	}
	return a, nil
}

func (w *OwnerCommissioning) Revoke(ctx context.Context, p authorization.Principal, consentID string) error {
	if _, err := w.ReadConsent(ctx, p, consentID); err != nil {
		return err
	}
	return w.store.RevokeMandateConsent(ctx, p.UserID, consentID)
}
