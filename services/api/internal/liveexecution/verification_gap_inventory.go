package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapInventoryVersion = "live-safety-evidence-gap-inventory-v1"

const safetyEvidenceGapInventoryDigestDomain = "arbion:live-safety:evidence-gap-inventory:v1"

var ErrSafetyEvidenceGapInventory = errors.New("live safety evidence gap inventory is invalid")

type SafetyEvidenceGapInventoryStatus string

const (
	SafetyEvidenceGapInventoryCompiled SafetyEvidenceGapInventoryStatus = "COMPILED"
	SafetyEvidenceGapInventoryRejected SafetyEvidenceGapInventoryStatus = "REJECTED"
)

type SafetyEvidenceGapInventoryFailure string

const (
	SafetyEvidenceGapInventoryFailureNone               SafetyEvidenceGapInventoryFailure = "NONE"
	SafetyEvidenceGapInventoryFailureSecretInput        SafetyEvidenceGapInventoryFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapInventoryFailureReviewVersion      SafetyEvidenceGapInventoryFailure = "REVIEW_VERSION_INVALID"
	SafetyEvidenceGapInventoryFailureReviewStatus       SafetyEvidenceGapInventoryFailure = "REVIEW_STATUS_INVALID"
	SafetyEvidenceGapInventoryFailureAuthority          SafetyEvidenceGapInventoryFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapInventoryFailureReviewMismatch     SafetyEvidenceGapInventoryFailure = "REVIEW_EVIDENCE_MISMATCH"
	SafetyEvidenceGapInventoryFailureComparisonRejected SafetyEvidenceGapInventoryFailure = "COMPARISON_REJECTED"
	SafetyEvidenceGapInventoryFailureReport             SafetyEvidenceGapInventoryFailure = "VERIFICATION_REPORT_INVALID"
	SafetyEvidenceGapInventoryFailureCategory           SafetyEvidenceGapInventoryFailure = "EVIDENCE_CATEGORY_INVALID"
	SafetyEvidenceGapInventoryFailureReason             SafetyEvidenceGapInventoryFailure = "ASSESSMENT_REASON_INVALID"
	SafetyEvidenceGapInventoryFailureCompilation        SafetyEvidenceGapInventoryFailure = "INVENTORY_COMPILATION_FAILED"
)

type SafetyEvidenceGapState string

const (
	SafetyEvidenceGapUnchanged        SafetyEvidenceGapState = "UNCHANGED"
	SafetyEvidenceGapChanged          SafetyEvidenceGapState = "CHANGED"
	SafetyEvidenceGapNewlyUnavailable SafetyEvidenceGapState = "NEWLY_UNAVAILABLE"
	SafetyEvidenceGapStillUnavailable SafetyEvidenceGapState = "STILL_UNAVAILABLE"
	SafetyEvidenceGapResolved         SafetyEvidenceGapState = "RESOLVED"
)

// SafetyEvidenceGap preserves one exact canonical evidence category and only
// attributes unavailability when the saved assessment contains that
// category's explicit reason code.
type SafetyEvidenceGap struct {
	Category            string                         `json:"category"`
	Source              EvidenceSource                 `json:"source"`
	State               SafetyEvidenceGapState         `json:"state"`
	BaselineUnavailable bool                           `json:"baseline_unavailable"`
	CurrentUnavailable  bool                           `json:"current_unavailable"`
	BaselineReasonCodes []ReasonCode                   `json:"baseline_reason_codes"`
	CurrentReasonCodes  []ReasonCode                   `json:"current_reason_codes"`
	Baseline            SafetyCaseEvidenceVerification `json:"baseline"`
	Current             SafetyCaseEvidenceVerification `json:"current"`
}

// SafetyEvidenceGapInventory is a pure, canonical review of exact saved
// evidence changes. It does not infer readiness, cause, or evidence that the
// verified reports did not contain.
type SafetyEvidenceGapInventory struct {
	InventoryVersion          string                                  `json:"inventory_version"`
	Status                    SafetyEvidenceGapInventoryStatus        `json:"status"`
	Failure                   SafetyEvidenceGapInventoryFailure       `json:"failure"`
	ExecutionAuthority        bool                                    `json:"execution_authority"`
	ReviewDigest              string                                  `json:"review_sha256"`
	BaselineEvaluatedAt       time.Time                               `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt        time.Time                               `json:"current_evaluated_at"`
	ComparisonStatus          VerificationComparisonStatus            `json:"comparison_status"`
	ComparisonFailureInput    VerificationComparisonInput             `json:"comparison_failure_input"`
	ComparisonFailure         VerificationComparisonFailure           `json:"comparison_failure"`
	ReviewDisposition         VerificationComparisonReviewDisposition `json:"review_disposition"`
	OwnerAction               VerificationComparisonOwnerAction       `json:"owner_action"`
	Assessment                VerificationAssessmentChange            `json:"assessment"`
	Categories                []SafetyEvidenceGap                     `json:"categories"`
	CurrentGapCount           int                                     `json:"current_gap_count"`
	NonUnchangedCategoryCount int                                     `json:"non_unchanged_category_count"`
	InventoryDigest           string                                  `json:"inventory_sha256"`
}

// CompileSafetyEvidenceGapInventory accepts only the exact independently
// verified review chain. An accurately recorded rejected comparison remains
// visible as a closed rejection because it cannot support a seven-category
// before/after inventory.
func CompileSafetyEvidenceGapInventory(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
) SafetyEvidenceGapInventory {
	inventory := SafetyEvidenceGapInventory{
		InventoryVersion: SafetyEvidenceGapInventoryVersion,
		Status:           SafetyEvidenceGapInventoryRejected,
		Failure:          SafetyEvidenceGapInventoryFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapInventoryFailure) SafetyEvidenceGapInventory {
		inventory.Status = SafetyEvidenceGapInventoryRejected
		inventory.Failure = failure
		return inventory
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(review)) {
		return reject(SafetyEvidenceGapInventoryFailureSecretInput)
	}
	if review.ArtifactVersion != SafetyCaseVerificationComparisonReviewArtifactVersion {
		return reject(SafetyEvidenceGapInventoryFailureReviewVersion)
	}
	if review.Status != VerificationComparisonReviewReady || review.Failure != VerificationComparisonReviewFailureNone {
		return reject(SafetyEvidenceGapInventoryFailureReviewStatus)
	}
	if review.ExecutionAuthority || baseline.ExecutionAuthority || current.ExecutionAuthority ||
		envelope.Comparison.ExecutionAuthority || audit.ExecutionAuthority {
		return reject(SafetyEvidenceGapInventoryFailureAuthority)
	}
	if VerifySafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit, review) != nil {
		return reject(SafetyEvidenceGapInventoryFailureReviewMismatch)
	}
	inventory.ReviewDigest = review.ReviewDigest
	inventory.BaselineEvaluatedAt = review.BaselineEvaluatedAt
	inventory.CurrentEvaluatedAt = review.CurrentEvaluatedAt
	inventory.ComparisonStatus = review.ComparisonStatus
	inventory.ComparisonFailureInput = review.ComparisonFailureInput
	inventory.ComparisonFailure = review.ComparisonFailure
	inventory.ReviewDisposition = review.Disposition
	inventory.OwnerAction = review.OwnerAction
	if review.ComparisonStatus == VerificationComparisonRejected {
		return reject(SafetyEvidenceGapInventoryFailureComparisonRejected)
	}
	if validateComparableVerificationReport(baseline) != VerificationComparisonFailureNone ||
		validateComparableVerificationReport(current) != VerificationComparisonFailureNone {
		return reject(SafetyEvidenceGapInventoryFailureReport)
	}
	if current.EvaluatedAt.Before(baseline.EvaluatedAt) {
		return reject(SafetyEvidenceGapInventoryFailureReport)
	}

	comparison := envelope.Comparison
	if len(comparison.Evidence) != len(verificationEvidenceExpectations()) {
		return reject(SafetyEvidenceGapInventoryFailureCategory)
	}
	inventory.Assessment = comparison.Assessment
	inventory.Categories = make([]SafetyEvidenceGap, 0, len(comparison.Evidence))
	for index, expected := range verificationEvidenceExpectations() {
		change := comparison.Evidence[index]
		if change.Category != expected.category || change.Before.Category != expected.category ||
			change.After.Category != expected.category || change.Before.Source != expected.source ||
			change.After.Source != expected.source {
			return reject(SafetyEvidenceGapInventoryFailureCategory)
		}
		baselineReasons, ok := categoryUnavailableReasons(expected.category, baseline.Assessment.ReasonCodes)
		if !ok {
			return reject(SafetyEvidenceGapInventoryFailureReason)
		}
		currentReasons, ok := categoryUnavailableReasons(expected.category, current.Assessment.ReasonCodes)
		if !ok {
			return reject(SafetyEvidenceGapInventoryFailureReason)
		}
		gap := SafetyEvidenceGap{
			Category: expected.category, Source: expected.source,
			BaselineUnavailable: len(baselineReasons) > 0, CurrentUnavailable: len(currentReasons) > 0,
			BaselineReasonCodes: baselineReasons, CurrentReasonCodes: currentReasons,
			Baseline: change.Before, Current: change.After,
		}
		gap.State = classifySafetyEvidenceGap(gap.BaselineUnavailable, gap.CurrentUnavailable, change.State)
		if gap.CurrentUnavailable {
			inventory.CurrentGapCount++
		}
		if gap.State != SafetyEvidenceGapUnchanged {
			inventory.NonUnchangedCategoryCount++
		}
		inventory.Categories = append(inventory.Categories, gap)
	}
	inventory.Status = SafetyEvidenceGapInventoryCompiled
	inventory.Failure = SafetyEvidenceGapInventoryFailureNone
	digest, err := canonicalSafetyEvidenceGapInventoryDigest(inventory)
	if err != nil {
		return reject(SafetyEvidenceGapInventoryFailureCompilation)
	}
	inventory.InventoryDigest = digest
	return inventory
}

// VerifySafetyEvidenceGapInventory recomputes the complete inventory. Changed
// inputs, ordering, classifications, counts, or digests fail closed.
func VerifySafetyEvidenceGapInventory(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
) error {
	if inventory.InventoryVersion != SafetyEvidenceGapInventoryVersion ||
		inventory.Status != SafetyEvidenceGapInventoryCompiled ||
		inventory.Failure != SafetyEvidenceGapInventoryFailureNone ||
		inventory.ExecutionAuthority ||
		!digestPattern.MatchString(inventory.ReviewDigest) ||
		!digestPattern.MatchString(inventory.InventoryDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(inventory)) {
		return ErrSafetyEvidenceGapInventory
	}
	expected := CompileSafetyEvidenceGapInventory(baseline, current, envelope, audit, review)
	if expected.Status != SafetyEvidenceGapInventoryCompiled || !reflect.DeepEqual(inventory, expected) {
		return ErrSafetyEvidenceGapInventory
	}
	return nil
}

func categoryUnavailableReasons(category string, reasons []ReasonCode) ([]ReasonCode, bool) {
	allowed := map[ReasonCode]struct{}{}
	switch category {
	case "PROVIDER_CAPABILITY":
		allowed[ReasonProviderCapabilityUnavailable] = struct{}{}
		allowed[ReasonTransferPermissionPresent] = struct{}{}
	case "OWNER_AUTHORIZATION":
		allowed[ReasonOwnerAuthorizationUnavailable] = struct{}{}
	case "DETERMINISTIC_RISK":
		allowed[ReasonDeterministicRiskUnavailable] = struct{}{}
	case "ACCOUNT_RECONCILIATION":
		allowed[ReasonReconciliationUnavailable] = struct{}{}
	case "BROKER_KILL_SWITCH":
		allowed[ReasonKillSwitchUnavailable] = struct{}{}
	case "IDEMPOTENCY_RESERVATION":
		allowed[ReasonIdempotencyUnavailable] = struct{}{}
	case "LIVE_LIFECYCLE_CONTRACT":
		allowed[ReasonLifecycleContractUnavailable] = struct{}{}
	default:
		return nil, false
	}
	result := make([]ReasonCode, 0, len(allowed))
	for _, reason := range reasons {
		if _, exists := allowed[reason]; exists {
			result = append(result, reason)
		}
	}
	return result, true
}

func classifySafetyEvidenceGap(baselineUnavailable, currentUnavailable bool, evidenceState VerificationChangeState) SafetyEvidenceGapState {
	switch {
	case !baselineUnavailable && currentUnavailable:
		return SafetyEvidenceGapNewlyUnavailable
	case baselineUnavailable && currentUnavailable:
		return SafetyEvidenceGapStillUnavailable
	case baselineUnavailable && !currentUnavailable:
		return SafetyEvidenceGapResolved
	case evidenceState == VerificationChangeChanged:
		return SafetyEvidenceGapChanged
	default:
		return SafetyEvidenceGapUnchanged
	}
}

func canonicalSafetyEvidenceGapInventoryDigest(inventory SafetyEvidenceGapInventory) (string, error) {
	inventory.InventoryDigest = ""
	payload, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapInventoryDigestDomain, payload), nil
}
