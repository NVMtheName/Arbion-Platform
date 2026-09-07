package liveexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	MaxRegistryReasons  = 32
	MaxRegistryEvidence = 32
	MaxRegistryList     = 50
)

var (
	ErrRegistryInvalid   = errors.New("live safety case evidence is invalid")
	ErrRegistryConflict  = errors.New("live safety case assessment key conflicts with immutable evidence")
	ErrRegistryIntegrity = errors.New("live safety case evidence integrity check failed")

	assessmentKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	evidenceKindPattern  = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
)

type EvidenceSource string

const (
	EvidenceDatabase             EvidenceSource = "DATABASE"
	EvidenceProviderVerified     EvidenceSource = "PROVIDER_VERIFIED"
	EvidenceOwnerMFA             EvidenceSource = "OWNER_MFA"
	EvidenceDeterministicControl EvidenceSource = "DETERMINISTIC_CONTROL"
	EvidenceDesignContract       EvidenceSource = "DESIGN_CONTRACT"
)

type EvidenceItem struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Digest     string         `json:"digest_sha256"`
	Source     EvidenceSource `json:"source"`
	ObservedAt time.Time      `json:"observed_at"`
}

type EvidenceManifest struct {
	Items []EvidenceItem `json:"items"`
}

type RegistryInput struct {
	AssessmentKey        string
	FinancialAccountID   string
	ProviderConnectionID string
	ProviderName         string
	StrategyInstanceID   string
	MandateID            string
	MandateVersion       int
	CapitalBucketID      string
	CapitalReservationID string
	ActionDigest         string
	Assessment           Assessment
	Evidence             EvidenceManifest
	ObservedAt           time.Time
}

type RegistryRecord struct {
	ID                   string
	UserID               string
	AssessmentKey        string
	FinancialAccountID   string
	ProviderConnectionID string
	ProviderName         string
	StrategyInstanceID   string
	MandateID            string
	MandateVersion       int
	CapitalBucketID      string
	CapitalReservationID string
	ActionDigest         string
	ContractVersion      string
	Assessment           Assessment
	Evidence             EvidenceManifest
	EvidenceDigest       string
	ContentDigest        string
	ObservedAt           time.Time
	CreatedAt            time.Time
}

type RegistryDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type RegistryStore struct{ db RegistryDB }

func NewRegistryStore(db RegistryDB) *RegistryStore { return &RegistryStore{db: db} }

// Record appends a credential-free, non-executable assessment. An exact replay
// returns the original immutable row; the same assessment key with different
// content fails closed.
func (store *RegistryStore) Record(ctx context.Context, userID string, input RegistryInput) (RegistryRecord, error) {
	prepared, reasonsJSON, manifestJSON, err := prepareRegistryRecord(userID, input, time.Now().UTC())
	if err != nil {
		return RegistryRecord{}, err
	}
	row := store.db.QueryRow(ctx, `
INSERT INTO live_safety_case_evidence(
  user_id,assessment_key,financial_account_id,provider_connection_id,provider_name,
  strategy_instance_id,mandate_id,mandate_version,capital_bucket_id,capital_reservation_id,
  action_digest_sha256,contract_version,assessment_state,structural_blocker,reason_codes,
  evidence_manifest,evidence_sha256,content_sha256,observed_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,true,$14,$15,$16,$17,$18)
ON CONFLICT (user_id,assessment_key) DO NOTHING
RETURNING `+registryColumns,
		userID, prepared.AssessmentKey, prepared.FinancialAccountID, prepared.ProviderConnectionID,
		prepared.ProviderName, prepared.StrategyInstanceID, prepared.MandateID, prepared.MandateVersion,
		prepared.CapitalBucketID, prepared.CapitalReservationID, prepared.ActionDigest,
		ContractVersion, prepared.Assessment.Availability, reasonsJSON, manifestJSON,
		prepared.EvidenceDigest, prepared.ContentDigest, prepared.ObservedAt)
	record, err := scanRegistryRecord(row)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RegistryRecord{}, err
	}

	record, err = scanRegistryRecord(store.db.QueryRow(ctx, `SELECT `+registryColumns+`
FROM live_safety_case_evidence WHERE user_id=$1 AND assessment_key=$2`, userID, prepared.AssessmentKey))
	if err != nil {
		return RegistryRecord{}, err
	}
	if record.ContentDigest != prepared.ContentDigest || record.EvidenceDigest != prepared.EvidenceDigest {
		return RegistryRecord{}, ErrRegistryConflict
	}
	return record, nil
}

// List returns only the caller's newest immutable safety assessments.
func (store *RegistryStore) List(ctx context.Context, userID string, limit int) ([]RegistryRecord, error) {
	if !uuidPattern.MatchString(userID) || limit < 1 || limit > MaxRegistryList {
		return nil, ErrRegistryInvalid
	}
	rows, err := store.db.Query(ctx, `SELECT `+registryColumns+`
FROM live_safety_case_evidence
WHERE user_id=$1
ORDER BY created_at DESC,id DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []RegistryRecord{}
	for rows.Next() {
		record, scanErr := scanRegistryRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

const registryColumns = `
id::text,user_id::text,assessment_key,financial_account_id::text,provider_connection_id::text,
provider_name,strategy_instance_id::text,mandate_id::text,mandate_version,capital_bucket_id::text,
capital_reservation_id::text,action_digest_sha256,contract_version,assessment_state,
structural_blocker,reason_codes,evidence_manifest,evidence_sha256,content_sha256,observed_at,created_at`

type rowScanner interface{ Scan(...any) error }

func scanRegistryRecord(row rowScanner) (RegistryRecord, error) {
	var record RegistryRecord
	var state string
	var structural bool
	var reasonsJSON, manifestJSON []byte
	err := row.Scan(
		&record.ID, &record.UserID, &record.AssessmentKey, &record.FinancialAccountID,
		&record.ProviderConnectionID, &record.ProviderName, &record.StrategyInstanceID,
		&record.MandateID, &record.MandateVersion, &record.CapitalBucketID,
		&record.CapitalReservationID, &record.ActionDigest, &record.ContractVersion, &state,
		&structural, &reasonsJSON, &manifestJSON, &record.EvidenceDigest, &record.ContentDigest,
		&record.ObservedAt, &record.CreatedAt,
	)
	if err != nil {
		return RegistryRecord{}, err
	}
	record.Assessment.Availability = Availability(state)
	record.Assessment.Structural = structural
	if err = json.Unmarshal(reasonsJSON, &record.Assessment.ReasonCodes); err != nil {
		return RegistryRecord{}, fmt.Errorf("decode live safety reasons: %w", err)
	}
	if err = json.Unmarshal(manifestJSON, &record.Evidence); err != nil {
		return RegistryRecord{}, fmt.Errorf("decode live safety manifest: %w", err)
	}
	if err = validateStoredRegistryRecord(record, time.Now().UTC()); err != nil {
		return RegistryRecord{}, err
	}
	return record, nil
}

func validateStoredRegistryRecord(record RegistryRecord, now time.Time) error {
	if !uuidPattern.MatchString(record.ID) || record.ContractVersion != ContractVersion ||
		record.CreatedAt.Before(record.ObservedAt) || record.CreatedAt.After(now) {
		return ErrRegistryIntegrity
	}
	prepared, _, _, err := prepareRegistryRecord(record.UserID, RegistryInput{
		AssessmentKey:        record.AssessmentKey,
		FinancialAccountID:   record.FinancialAccountID,
		ProviderConnectionID: record.ProviderConnectionID,
		ProviderName:         record.ProviderName,
		StrategyInstanceID:   record.StrategyInstanceID,
		MandateID:            record.MandateID,
		MandateVersion:       record.MandateVersion,
		CapitalBucketID:      record.CapitalBucketID,
		CapitalReservationID: record.CapitalReservationID,
		ActionDigest:         record.ActionDigest,
		Assessment:           record.Assessment,
		Evidence:             record.Evidence,
		ObservedAt:           record.ObservedAt,
	}, now)
	if err != nil || prepared.EvidenceDigest != record.EvidenceDigest || prepared.ContentDigest != record.ContentDigest {
		return ErrRegistryIntegrity
	}
	return nil
}

func prepareRegistryRecord(userID string, input RegistryInput, now time.Time) (RegistryRecord, []byte, []byte, error) {
	if !uuidPattern.MatchString(userID) || !assessmentKeyPattern.MatchString(input.AssessmentKey) ||
		!uuidPattern.MatchString(input.FinancialAccountID) || !uuidPattern.MatchString(input.ProviderConnectionID) ||
		!providerPattern.MatchString(input.ProviderName) || !uuidPattern.MatchString(input.StrategyInstanceID) ||
		!uuidPattern.MatchString(input.MandateID) || input.MandateVersion < 1 ||
		!uuidPattern.MatchString(input.CapitalBucketID) || !uuidPattern.MatchString(input.CapitalReservationID) ||
		!digestPattern.MatchString(input.ActionDigest) || input.ObservedAt.IsZero() || input.ObservedAt.After(now) ||
		!input.Assessment.Structural || len(input.Assessment.ReasonCodes) < 1 || len(input.Assessment.ReasonCodes) > MaxRegistryReasons ||
		len(input.Evidence.Items) < 1 || len(input.Evidence.Items) > MaxRegistryEvidence {
		return RegistryRecord{}, nil, nil, ErrRegistryInvalid
	}

	reasons := append([]ReasonCode(nil), input.Assessment.ReasonCodes...)
	seenReasons := make(map[ReasonCode]struct{}, len(reasons))
	hasRuntimeBlocker := false
	hasEvidenceReason := false
	for _, reason := range reasons {
		if !isRegistryReason(reason) {
			return RegistryRecord{}, nil, nil, ErrRegistryInvalid
		}
		if _, exists := seenReasons[reason]; exists {
			return RegistryRecord{}, nil, nil, ErrRegistryInvalid
		}
		seenReasons[reason] = struct{}{}
		hasRuntimeBlocker = hasRuntimeBlocker || reason == ReasonLiveRuntimeUnimplemented
		hasEvidenceReason = hasEvidenceReason || reason != ReasonLiveRuntimeUnimplemented
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
	if (input.Assessment.Availability == BlockedUnimplemented && !hasRuntimeBlocker) ||
		(input.Assessment.Availability == Unavailable && !hasEvidenceReason) ||
		(input.Assessment.Availability != BlockedUnimplemented && input.Assessment.Availability != Unavailable) {
		return RegistryRecord{}, nil, nil, ErrRegistryInvalid
	}

	items := append([]EvidenceItem(nil), input.Evidence.Items...)
	seenItems := make(map[string]struct{}, len(items))
	for index := range items {
		item := &items[index]
		if !uuidPattern.MatchString(item.ID) || !evidenceKindPattern.MatchString(item.Kind) ||
			containsSecretLikeValue(item.Kind) ||
			!digestPattern.MatchString(item.Digest) || !isEvidenceSource(item.Source) ||
			item.ObservedAt.IsZero() || item.ObservedAt.After(input.ObservedAt) || item.ObservedAt.After(now) {
			return RegistryRecord{}, nil, nil, ErrRegistryInvalid
		}
		if _, exists := seenItems[item.ID]; exists {
			return RegistryRecord{}, nil, nil, ErrRegistryInvalid
		}
		seenItems[item.ID] = struct{}{}
		item.ObservedAt = item.ObservedAt.UTC()
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	manifest := EvidenceManifest{Items: items}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil || containsSecretLikeKey(string(manifestJSON)) {
		return RegistryRecord{}, nil, nil, ErrRegistryInvalid
	}
	reasonsJSON, err := json.Marshal(reasons)
	if err != nil {
		return RegistryRecord{}, nil, nil, ErrRegistryInvalid
	}
	evidenceDigest := sha256Hex(manifestJSON)
	canonical := struct {
		UserID               string       `json:"user_id"`
		AssessmentKey        string       `json:"assessment_key"`
		FinancialAccountID   string       `json:"financial_account_id"`
		ProviderConnectionID string       `json:"provider_connection_id"`
		ProviderName         string       `json:"provider_name"`
		StrategyInstanceID   string       `json:"strategy_instance_id"`
		MandateID            string       `json:"mandate_id"`
		MandateVersion       int          `json:"mandate_version"`
		CapitalBucketID      string       `json:"capital_bucket_id"`
		CapitalReservationID string       `json:"capital_reservation_id"`
		ActionDigest         string       `json:"action_digest_sha256"`
		ContractVersion      string       `json:"contract_version"`
		AssessmentState      Availability `json:"assessment_state"`
		StructuralBlocker    bool         `json:"structural_blocker"`
		ReasonCodes          []ReasonCode `json:"reason_codes"`
		EvidenceDigest       string       `json:"evidence_sha256"`
		ObservedAt           string       `json:"observed_at"`
	}{
		userID, input.AssessmentKey, input.FinancialAccountID, input.ProviderConnectionID,
		input.ProviderName, input.StrategyInstanceID, input.MandateID, input.MandateVersion,
		input.CapitalBucketID, input.CapitalReservationID, input.ActionDigest, ContractVersion,
		input.Assessment.Availability, true, reasons, evidenceDigest,
		input.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
	contentJSON, err := json.Marshal(canonical)
	if err != nil {
		return RegistryRecord{}, nil, nil, ErrRegistryInvalid
	}
	return RegistryRecord{
		UserID: userID, AssessmentKey: input.AssessmentKey,
		FinancialAccountID: input.FinancialAccountID, ProviderConnectionID: input.ProviderConnectionID,
		ProviderName: input.ProviderName, StrategyInstanceID: input.StrategyInstanceID,
		MandateID: input.MandateID, MandateVersion: input.MandateVersion,
		CapitalBucketID: input.CapitalBucketID, CapitalReservationID: input.CapitalReservationID,
		ActionDigest: input.ActionDigest, ContractVersion: ContractVersion,
		Assessment: Assessment{Availability: input.Assessment.Availability, ReasonCodes: reasons, Structural: true},
		Evidence:   manifest, EvidenceDigest: evidenceDigest, ContentDigest: sha256Hex(contentJSON),
		ObservedAt: input.ObservedAt.UTC(),
	}, reasonsJSON, manifestJSON, nil
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func isEvidenceSource(source EvidenceSource) bool {
	switch source {
	case EvidenceDatabase, EvidenceProviderVerified, EvidenceOwnerMFA, EvidenceDeterministicControl, EvidenceDesignContract:
		return true
	default:
		return false
	}
}

func isRegistryReason(reason ReasonCode) bool {
	switch reason {
	case ReasonInvalidEvidence, ReasonDuplicateEvidence, ReasonFutureEvidence, ReasonStaleEvidence,
		ReasonBindingMismatch, ReasonActionDigestMismatch, ReasonNonLiveMode,
		ReasonProviderCapabilityUnavailable, ReasonTransferPermissionPresent,
		ReasonOwnerAuthorizationUnavailable, ReasonDeterministicRiskUnavailable,
		ReasonReconciliationUnavailable, ReasonKillSwitchUnavailable, ReasonIdempotencyUnavailable,
		ReasonLifecycleContractUnavailable, ReasonLiveRuntimeUnimplemented, ReasonLifecycleEvidenceInvalid,
		ReasonPostTradeReconciliationMissing:
		return true
	default:
		return false
	}
}

func containsSecretLikeKey(value string) bool {
	lower := strings.ToLower(value)
	for _, key := range []string{"api_key", "secret", "credential", "password", "private_key", "access_token", "refresh_token", "authorization_header"} {
		if strings.Contains(lower, `"`+key+`":`) {
			return true
		}
	}
	return false
}

func containsSecretLikeValue(value string) bool {
	upper := strings.ToUpper(value)
	for _, term := range []string{"API_KEY", "SECRET", "CREDENTIAL", "PASSWORD", "PRIVATE_KEY", "ACCESS_TOKEN", "REFRESH_TOKEN", "AUTHORIZATION_HEADER"} {
		if strings.Contains(upper, term) {
			return true
		}
	}
	return false
}
