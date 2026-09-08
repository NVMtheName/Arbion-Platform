package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapInventoryVerificationReportVersion = "live-safety-evidence-gap-inventory-verification-v1"

const safetyEvidenceGapInventoryVerificationReportDigestDomain = "arbion:live-safety:evidence-gap-inventory-verification:v1"

var ErrSafetyEvidenceGapInventoryVerificationReport = errors.New("live safety evidence gap inventory verification report is invalid")

type SafetyEvidenceGapInventoryVerificationFailure string

const (
	SafetyEvidenceGapInventoryVerificationFailureNone               SafetyEvidenceGapInventoryVerificationFailure = "NONE"
	SafetyEvidenceGapInventoryVerificationFailureSecretInput        SafetyEvidenceGapInventoryVerificationFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapInventoryVerificationFailureUpstream           SafetyEvidenceGapInventoryVerificationFailure = "UPSTREAM_REVIEW_INVALID"
	SafetyEvidenceGapInventoryVerificationFailureComparisonRejected SafetyEvidenceGapInventoryVerificationFailure = "COMPARISON_REJECTED"
	SafetyEvidenceGapInventoryVerificationFailureInventoryVersion   SafetyEvidenceGapInventoryVerificationFailure = "INVENTORY_VERSION_INVALID"
	SafetyEvidenceGapInventoryVerificationFailureInventoryStatus    SafetyEvidenceGapInventoryVerificationFailure = "INVENTORY_STATUS_INVALID"
	SafetyEvidenceGapInventoryVerificationFailureAuthority          SafetyEvidenceGapInventoryVerificationFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapInventoryVerificationFailureCategory           SafetyEvidenceGapInventoryVerificationFailure = "EVIDENCE_CATEGORY_MISMATCH"
	SafetyEvidenceGapInventoryVerificationFailureCount              SafetyEvidenceGapInventoryVerificationFailure = "EVIDENCE_COUNT_MISMATCH"
	SafetyEvidenceGapInventoryVerificationFailureDigestMalformed    SafetyEvidenceGapInventoryVerificationFailure = "INVENTORY_DIGEST_MALFORMED"
	SafetyEvidenceGapInventoryVerificationFailureDigestMismatch     SafetyEvidenceGapInventoryVerificationFailure = "INVENTORY_DIGEST_MISMATCH"
	SafetyEvidenceGapInventoryVerificationFailureInventoryMismatch  SafetyEvidenceGapInventoryVerificationFailure = "INVENTORY_CONTENT_MISMATCH"
	SafetyEvidenceGapInventoryVerificationFailureUnavailable        SafetyEvidenceGapInventoryVerificationFailure = "CURRENT_EVIDENCE_UNAVAILABLE"
	SafetyEvidenceGapInventoryVerificationFailureCompilation        SafetyEvidenceGapInventoryVerificationFailure = "REPORT_COMPILATION_FAILED"
)

// SafetyEvidenceGapCategoryVerification records the claimed and independently
// recomputed state for one canonical evidence category.
type SafetyEvidenceGapCategoryVerification struct {
	Category                     string                 `json:"category"`
	Source                       EvidenceSource         `json:"source"`
	ClaimedState                 SafetyEvidenceGapState `json:"claimed_state"`
	RecomputedState              SafetyEvidenceGapState `json:"recomputed_state"`
	ClaimedCurrentUnavailable    bool                   `json:"claimed_current_unavailable"`
	RecomputedCurrentUnavailable bool                   `json:"recomputed_current_unavailable"`
	Verified                     bool                   `json:"verified"`
}

// SafetyEvidenceGapInventoryVerificationReport independently recomputes the
// complete seven-category inventory and its canonical digest. It is review
// evidence only. A structurally exact inventory with current evidence gaps is
// preserved as a verifiable, closed CURRENT_EVIDENCE_UNAVAILABLE result.
type SafetyEvidenceGapInventoryVerificationReport struct {
	ReportVersion                       string                                        `json:"report_version"`
	Status                              VerificationStatus                            `json:"status"`
	Failure                             SafetyEvidenceGapInventoryVerificationFailure `json:"failure"`
	ExecutionAuthority                  bool                                          `json:"execution_authority"`
	InventoryVersion                    VersionVerification                           `json:"inventory_version"`
	BaselineEvaluatedAt                 time.Time                                     `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt                  time.Time                                     `json:"current_evaluated_at"`
	ReviewDigest                        DigestVerification                            `json:"review_digest"`
	InventoryDigest                     DigestVerification                            `json:"inventory_digest"`
	ExpectedCategoryCount               int                                           `json:"expected_category_count"`
	ClaimedCategoryCount                int                                           `json:"claimed_category_count"`
	RecomputedCurrentGapCount           int                                           `json:"recomputed_current_gap_count"`
	ClaimedCurrentGapCount              int                                           `json:"claimed_current_gap_count"`
	RecomputedNonUnchangedCategoryCount int                                           `json:"recomputed_non_unchanged_category_count"`
	ClaimedNonUnchangedCategoryCount    int                                           `json:"claimed_non_unchanged_category_count"`
	CurrentEvidenceAvailable            bool                                          `json:"current_evidence_available"`
	Categories                          []SafetyEvidenceGapCategoryVerification       `json:"categories"`
	ReportDigest                        string                                        `json:"report_sha256"`
}

// BuildSafetyEvidenceGapInventoryVerificationReport independently rebuilds
// the expected inventory from the complete verified review chain. It never
// calls the inventory compiler or verifier.
func BuildSafetyEvidenceGapInventoryVerificationReport(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
) SafetyEvidenceGapInventoryVerificationReport {
	report := SafetyEvidenceGapInventoryVerificationReport{
		ReportVersion: SafetyEvidenceGapInventoryVerificationReportVersion,
		Status:        VerificationRejected,
		Failure:       SafetyEvidenceGapInventoryVerificationFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapInventoryVerificationFailure) SafetyEvidenceGapInventoryVerificationReport {
		report.Status = VerificationRejected
		report.Failure = failure
		return report
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(review)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(inventory)) {
		return reject(SafetyEvidenceGapInventoryVerificationFailureSecretInput)
	}
	if VerifySafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit, review) != nil ||
		validateComparableVerificationReport(baseline) != VerificationComparisonFailureNone ||
		validateComparableVerificationReport(current) != VerificationComparisonFailureNone {
		return reject(SafetyEvidenceGapInventoryVerificationFailureUpstream)
	}
	if review.ComparisonStatus == VerificationComparisonRejected {
		return reject(SafetyEvidenceGapInventoryVerificationFailureComparisonRejected)
	}
	report.InventoryVersion = VersionVerification{
		Expected: SafetyEvidenceGapInventoryVersion,
		Claimed:  inventory.InventoryVersion,
		Verified: inventory.InventoryVersion == SafetyEvidenceGapInventoryVersion,
	}
	if !report.InventoryVersion.Verified {
		return reject(SafetyEvidenceGapInventoryVerificationFailureInventoryVersion)
	}
	if inventory.Status != SafetyEvidenceGapInventoryCompiled || inventory.Failure != SafetyEvidenceGapInventoryFailureNone {
		return reject(SafetyEvidenceGapInventoryVerificationFailureInventoryStatus)
	}
	if inventory.ExecutionAuthority || baseline.ExecutionAuthority || current.ExecutionAuthority ||
		envelope.Comparison.ExecutionAuthority || audit.ExecutionAuthority || review.ExecutionAuthority {
		return reject(SafetyEvidenceGapInventoryVerificationFailureAuthority)
	}

	report.BaselineEvaluatedAt = baseline.EvaluatedAt
	report.CurrentEvaluatedAt = current.EvaluatedAt
	report.ReviewDigest = DigestVerification{
		Claimed: inventory.ReviewDigest, Recomputed: review.ReviewDigest,
		Expected: review.ReviewDigest, Verified: inventory.ReviewDigest == review.ReviewDigest,
	}
	if !report.ReviewDigest.Verified || !digestPattern.MatchString(inventory.ReviewDigest) {
		return reject(SafetyEvidenceGapInventoryVerificationFailureInventoryMismatch)
	}

	expected, ok := independentlyRecomputeSafetyEvidenceGapInventory(baseline, current, envelope, review)
	if !ok {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCategory)
	}
	report.ExpectedCategoryCount = len(verificationEvidenceExpectations())
	report.ClaimedCategoryCount = len(inventory.Categories)
	if report.ClaimedCategoryCount != report.ExpectedCategoryCount {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCount)
	}
	report.RecomputedCurrentGapCount = expected.CurrentGapCount
	report.ClaimedCurrentGapCount = inventory.CurrentGapCount
	report.RecomputedNonUnchangedCategoryCount = expected.NonUnchangedCategoryCount
	report.ClaimedNonUnchangedCategoryCount = inventory.NonUnchangedCategoryCount
	if report.ClaimedCurrentGapCount != report.RecomputedCurrentGapCount ||
		report.ClaimedNonUnchangedCategoryCount != report.RecomputedNonUnchangedCategoryCount {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCount)
	}

	report.Categories = make([]SafetyEvidenceGapCategoryVerification, 0, report.ExpectedCategoryCount)
	for index := range expected.Categories {
		claimed := inventory.Categories[index]
		recomputed := expected.Categories[index]
		verified := reflect.DeepEqual(claimed, recomputed)
		report.Categories = append(report.Categories, SafetyEvidenceGapCategoryVerification{
			Category: recomputed.Category, Source: recomputed.Source,
			ClaimedState: claimed.State, RecomputedState: recomputed.State,
			ClaimedCurrentUnavailable:    claimed.CurrentUnavailable,
			RecomputedCurrentUnavailable: recomputed.CurrentUnavailable,
			Verified:                     verified,
		})
		if !verified {
			return reject(SafetyEvidenceGapInventoryVerificationFailureCategory)
		}
	}

	recomputedClaimedDigest, err := independentlyCanonicalSafetyEvidenceGapInventoryDigest(inventory)
	if err != nil {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCompilation)
	}
	expectedDigest, err := independentlyCanonicalSafetyEvidenceGapInventoryDigest(expected)
	if err != nil {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCompilation)
	}
	report.InventoryDigest = DigestVerification{
		Claimed: inventory.InventoryDigest, Recomputed: recomputedClaimedDigest,
		Expected: expectedDigest,
		Verified: inventory.InventoryDigest == recomputedClaimedDigest && inventory.InventoryDigest == expectedDigest,
	}
	if !digestPattern.MatchString(inventory.InventoryDigest) {
		return reject(SafetyEvidenceGapInventoryVerificationFailureDigestMalformed)
	}
	if !report.InventoryDigest.Verified {
		return reject(SafetyEvidenceGapInventoryVerificationFailureDigestMismatch)
	}
	if !reflect.DeepEqual(inventory, expected) {
		return reject(SafetyEvidenceGapInventoryVerificationFailureInventoryMismatch)
	}

	report.CurrentEvidenceAvailable = report.RecomputedCurrentGapCount == 0
	if report.CurrentEvidenceAvailable {
		report.Status = VerificationVerified
		report.Failure = SafetyEvidenceGapInventoryVerificationFailureNone
	} else {
		report.Status = VerificationRejected
		report.Failure = SafetyEvidenceGapInventoryVerificationFailureUnavailable
	}
	reportDigest, err := canonicalSafetyEvidenceGapInventoryVerificationReportDigest(report)
	if err != nil {
		return reject(SafetyEvidenceGapInventoryVerificationFailureCompilation)
	}
	report.ReportDigest = reportDigest
	return report
}

// VerifySafetyEvidenceGapInventoryVerificationReport accepts an exact VERIFIED
// report or an exact closed CURRENT_EVIDENCE_UNAVAILABLE report. Integrity or
// content failures never verify.
func VerifySafetyEvidenceGapInventoryVerificationReport(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
) error {
	if report.ReportVersion != SafetyEvidenceGapInventoryVerificationReportVersion ||
		report.ExecutionAuthority || !digestPattern.MatchString(report.ReportDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) {
		return ErrSafetyEvidenceGapInventoryVerificationReport
	}
	validConclusion := report.Status == VerificationVerified && report.Failure == SafetyEvidenceGapInventoryVerificationFailureNone && report.CurrentEvidenceAvailable
	validUnavailable := report.Status == VerificationRejected && report.Failure == SafetyEvidenceGapInventoryVerificationFailureUnavailable && !report.CurrentEvidenceAvailable
	if !validConclusion && !validUnavailable {
		return ErrSafetyEvidenceGapInventoryVerificationReport
	}
	expected := BuildSafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory)
	if !reflect.DeepEqual(report, expected) {
		return ErrSafetyEvidenceGapInventoryVerificationReport
	}
	return nil
}

func independentlyRecomputeSafetyEvidenceGapInventory(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	review SafetyCaseVerificationComparisonReviewArtifact,
) (SafetyEvidenceGapInventory, bool) {
	expected := SafetyEvidenceGapInventory{
		InventoryVersion:       SafetyEvidenceGapInventoryVersion,
		Status:                 SafetyEvidenceGapInventoryCompiled,
		Failure:                SafetyEvidenceGapInventoryFailureNone,
		ReviewDigest:           review.ReviewDigest,
		BaselineEvaluatedAt:    review.BaselineEvaluatedAt,
		CurrentEvaluatedAt:     review.CurrentEvaluatedAt,
		ComparisonStatus:       review.ComparisonStatus,
		ComparisonFailureInput: review.ComparisonFailureInput,
		ComparisonFailure:      review.ComparisonFailure,
		ReviewDisposition:      review.Disposition,
		OwnerAction:            review.OwnerAction,
		Assessment:             envelope.Comparison.Assessment,
	}
	if current.EvaluatedAt.Before(baseline.EvaluatedAt) || len(envelope.Comparison.Evidence) != len(verificationEvidenceExpectations()) {
		return SafetyEvidenceGapInventory{}, false
	}
	expected.Categories = make([]SafetyEvidenceGap, 0, len(verificationEvidenceExpectations()))
	for index, expectation := range verificationEvidenceExpectations() {
		change := envelope.Comparison.Evidence[index]
		if change.Category != expectation.category || change.Before.Category != expectation.category ||
			change.After.Category != expectation.category || change.Before.Source != expectation.source || change.After.Source != expectation.source {
			return SafetyEvidenceGapInventory{}, false
		}
		baselineReasons, ok := independentlyMapGapReasons(expectation.category, baseline.Assessment.ReasonCodes)
		if !ok {
			return SafetyEvidenceGapInventory{}, false
		}
		currentReasons, ok := independentlyMapGapReasons(expectation.category, current.Assessment.ReasonCodes)
		if !ok {
			return SafetyEvidenceGapInventory{}, false
		}
		gap := SafetyEvidenceGap{
			Category: expectation.category, Source: expectation.source,
			BaselineUnavailable: len(baselineReasons) > 0, CurrentUnavailable: len(currentReasons) > 0,
			BaselineReasonCodes: baselineReasons, CurrentReasonCodes: currentReasons,
			Baseline: change.Before, Current: change.After,
		}
		gap.State = independentlyClassifyGap(gap.BaselineUnavailable, gap.CurrentUnavailable, change.State)
		if gap.CurrentUnavailable {
			expected.CurrentGapCount++
		}
		if gap.State != SafetyEvidenceGapUnchanged {
			expected.NonUnchangedCategoryCount++
		}
		expected.Categories = append(expected.Categories, gap)
	}
	digest, err := independentlyCanonicalSafetyEvidenceGapInventoryDigest(expected)
	if err != nil {
		return SafetyEvidenceGapInventory{}, false
	}
	expected.InventoryDigest = digest
	return expected, true
}

func independentlyMapGapReasons(category string, reasons []ReasonCode) ([]ReasonCode, bool) {
	var candidates []ReasonCode
	switch category {
	case "PROVIDER_CAPABILITY":
		candidates = []ReasonCode{ReasonProviderCapabilityUnavailable, ReasonTransferPermissionPresent}
	case "OWNER_AUTHORIZATION":
		candidates = []ReasonCode{ReasonOwnerAuthorizationUnavailable}
	case "DETERMINISTIC_RISK":
		candidates = []ReasonCode{ReasonDeterministicRiskUnavailable}
	case "ACCOUNT_RECONCILIATION":
		candidates = []ReasonCode{ReasonReconciliationUnavailable}
	case "BROKER_KILL_SWITCH":
		candidates = []ReasonCode{ReasonKillSwitchUnavailable}
	case "IDEMPOTENCY_RESERVATION":
		candidates = []ReasonCode{ReasonIdempotencyUnavailable}
	case "LIVE_LIFECYCLE_CONTRACT":
		candidates = []ReasonCode{ReasonLifecycleContractUnavailable}
	default:
		return nil, false
	}
	result := make([]ReasonCode, 0, len(candidates))
	for _, reason := range reasons {
		for _, candidate := range candidates {
			if reason == candidate {
				result = append(result, reason)
				break
			}
		}
	}
	return result, true
}

func independentlyClassifyGap(baselineUnavailable, currentUnavailable bool, state VerificationChangeState) SafetyEvidenceGapState {
	if !baselineUnavailable && currentUnavailable {
		return SafetyEvidenceGapNewlyUnavailable
	}
	if baselineUnavailable && currentUnavailable {
		return SafetyEvidenceGapStillUnavailable
	}
	if baselineUnavailable && !currentUnavailable {
		return SafetyEvidenceGapResolved
	}
	if state == VerificationChangeChanged {
		return SafetyEvidenceGapChanged
	}
	return SafetyEvidenceGapUnchanged
}

// This projection deliberately duplicates the public inventory wire order. It
// provides an independent canonical encoding rather than delegating to the
// inventory compiler's digest helper.
type safetyEvidenceGapInventoryDigestProjection struct {
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

func independentlyCanonicalSafetyEvidenceGapInventoryDigest(inventory SafetyEvidenceGapInventory) (string, error) {
	projection := safetyEvidenceGapInventoryDigestProjection{
		InventoryVersion: inventory.InventoryVersion, Status: inventory.Status, Failure: inventory.Failure,
		ExecutionAuthority: inventory.ExecutionAuthority, ReviewDigest: inventory.ReviewDigest,
		BaselineEvaluatedAt: inventory.BaselineEvaluatedAt, CurrentEvaluatedAt: inventory.CurrentEvaluatedAt,
		ComparisonStatus: inventory.ComparisonStatus, ComparisonFailureInput: inventory.ComparisonFailureInput,
		ComparisonFailure: inventory.ComparisonFailure, ReviewDisposition: inventory.ReviewDisposition,
		OwnerAction: inventory.OwnerAction, Assessment: inventory.Assessment, Categories: inventory.Categories,
		CurrentGapCount: inventory.CurrentGapCount, NonUnchangedCategoryCount: inventory.NonUnchangedCategoryCount,
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapInventoryDigestDomain, payload), nil
}

func canonicalSafetyEvidenceGapInventoryVerificationReportDigest(report SafetyEvidenceGapInventoryVerificationReport) (string, error) {
	report.ReportDigest = ""
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapInventoryVerificationReportDigestDomain, payload), nil
}
