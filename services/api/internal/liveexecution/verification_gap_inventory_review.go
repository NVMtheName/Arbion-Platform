package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapInventoryVerificationReviewVersion = "live-safety-evidence-gap-inventory-verification-review-v1"

const safetyEvidenceGapInventoryVerificationReviewDigestDomain = "arbion:live-safety:evidence-gap-inventory-verification-review:v1"

var ErrSafetyEvidenceGapInventoryVerificationReview = errors.New("live safety evidence gap inventory verification review is invalid")

type SafetyEvidenceGapInventoryVerificationReviewStatus string

const (
	SafetyEvidenceGapInventoryVerificationReviewReady    SafetyEvidenceGapInventoryVerificationReviewStatus = "REVIEW_READY"
	SafetyEvidenceGapInventoryVerificationReviewRejected SafetyEvidenceGapInventoryVerificationReviewStatus = "REJECTED"
)

type SafetyEvidenceGapInventoryVerificationReviewDisposition string

const (
	SafetyEvidenceGapInventoryVerificationReviewNoGaps      SafetyEvidenceGapInventoryVerificationReviewDisposition = "NO_GAPS"
	SafetyEvidenceGapInventoryVerificationReviewOwnerReview SafetyEvidenceGapInventoryVerificationReviewDisposition = "OWNER_REVIEW_REQUIRED"
)

type SafetyEvidenceGapInventoryVerificationReviewOwnerAction string

const (
	SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone       SafetyEvidenceGapInventoryVerificationReviewOwnerAction = "NONE"
	SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps SafetyEvidenceGapInventoryVerificationReviewOwnerAction = "REVIEW_UNAVAILABLE_EVIDENCE"
)

type SafetyEvidenceGapInventoryVerificationReviewFailure string

const (
	SafetyEvidenceGapInventoryVerificationReviewFailureNone          SafetyEvidenceGapInventoryVerificationReviewFailure = "NONE"
	SafetyEvidenceGapInventoryVerificationReviewFailureSecretInput   SafetyEvidenceGapInventoryVerificationReviewFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapInventoryVerificationReviewFailureReportVersion SafetyEvidenceGapInventoryVerificationReviewFailure = "REPORT_VERSION_INVALID"
	SafetyEvidenceGapInventoryVerificationReviewFailureReportStatus  SafetyEvidenceGapInventoryVerificationReviewFailure = "REPORT_STATUS_INVALID"
	SafetyEvidenceGapInventoryVerificationReviewFailureAuthority     SafetyEvidenceGapInventoryVerificationReviewFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapInventoryVerificationReviewFailureReport        SafetyEvidenceGapInventoryVerificationReviewFailure = "REPORT_VERIFICATION_FAILED"
	SafetyEvidenceGapInventoryVerificationReviewFailureDigest        SafetyEvidenceGapInventoryVerificationReviewFailure = "DIGEST_EVIDENCE_INVALID"
	SafetyEvidenceGapInventoryVerificationReviewFailureCount         SafetyEvidenceGapInventoryVerificationReviewFailure = "COUNT_EVIDENCE_INVALID"
	SafetyEvidenceGapInventoryVerificationReviewFailureCategory      SafetyEvidenceGapInventoryVerificationReviewFailure = "CATEGORY_EVIDENCE_INVALID"
	SafetyEvidenceGapInventoryVerificationReviewFailureCompilation   SafetyEvidenceGapInventoryVerificationReviewFailure = "REVIEW_COMPILATION_FAILED"
)

// SafetyEvidenceGapInventoryVerificationReview is a canonical owner-facing
// conclusion over one exact independently verified gap-inventory report. It
// can request review of unavailable evidence but cannot approve or execute an
// action.
type SafetyEvidenceGapInventoryVerificationReview struct {
	ArtifactVersion           string                                                  `json:"artifact_version"`
	Status                    SafetyEvidenceGapInventoryVerificationReviewStatus      `json:"status"`
	Failure                   SafetyEvidenceGapInventoryVerificationReviewFailure     `json:"failure"`
	ExecutionAuthority        bool                                                    `json:"execution_authority"`
	Disposition               SafetyEvidenceGapInventoryVerificationReviewDisposition `json:"disposition"`
	OwnerAction               SafetyEvidenceGapInventoryVerificationReviewOwnerAction `json:"owner_action"`
	VerificationReportVersion string                                                  `json:"verification_report_version"`
	VerificationReportStatus  VerificationStatus                                      `json:"verification_report_status"`
	VerificationReportFailure SafetyEvidenceGapInventoryVerificationFailure           `json:"verification_report_failure"`
	BaselineEvaluatedAt       time.Time                                               `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt        time.Time                                               `json:"current_evaluated_at"`
	CurrentEvidenceAvailable  bool                                                    `json:"current_evidence_available"`
	ExpectedCategoryCount     int                                                     `json:"expected_category_count"`
	CurrentGapCount           int                                                     `json:"current_gap_count"`
	NonUnchangedCategoryCount int                                                     `json:"non_unchanged_category_count"`
	ReviewDigest              DigestVerification                                      `json:"review_digest"`
	InventoryDigest           DigestVerification                                      `json:"inventory_digest"`
	Categories                []SafetyEvidenceGapCategoryVerification                 `json:"categories"`
	VerificationReportDigest  string                                                  `json:"verification_report_sha256"`
	ArtifactDigest            string                                                  `json:"artifact_sha256"`
}

// BuildSafetyEvidenceGapInventoryVerificationReview accepts only an exact
// report over the supplied immutable review chain and inventory. The only
// review-ready conclusions are NO_GAPS and OWNER_REVIEW_REQUIRED.
func BuildSafetyEvidenceGapInventoryVerificationReview(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
) SafetyEvidenceGapInventoryVerificationReview {
	artifact := SafetyEvidenceGapInventoryVerificationReview{
		ArtifactVersion: SafetyEvidenceGapInventoryVerificationReviewVersion,
		Status:          SafetyEvidenceGapInventoryVerificationReviewRejected,
		Failure:         SafetyEvidenceGapInventoryVerificationReviewFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapInventoryVerificationReviewFailure) SafetyEvidenceGapInventoryVerificationReview {
		artifact.Status = SafetyEvidenceGapInventoryVerificationReviewRejected
		artifact.Failure = failure
		return artifact
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(review)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(inventory)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureSecretInput)
	}
	if report.ReportVersion != SafetyEvidenceGapInventoryVerificationReportVersion {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureReportVersion)
	}
	if report.ExecutionAuthority || inventory.ExecutionAuthority || baseline.ExecutionAuthority || current.ExecutionAuthority ||
		envelope.Comparison.ExecutionAuthority || audit.ExecutionAuthority || review.ExecutionAuthority {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureAuthority)
	}
	if !validGapInventoryVerificationReportConclusion(report) {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureReportStatus)
	}
	if VerifySafetyEvidenceGapInventoryVerificationReport(baseline, current, envelope, audit, review, inventory, report) != nil {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureReport)
	}
	if !exactVerifiedDigest(report.ReviewDigest) || !exactVerifiedDigest(report.InventoryDigest) ||
		!digestPattern.MatchString(report.ReportDigest) {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureDigest)
	}
	if report.ExpectedCategoryCount != len(verificationEvidenceExpectations()) ||
		report.ClaimedCategoryCount != report.ExpectedCategoryCount ||
		report.ClaimedCurrentGapCount != report.RecomputedCurrentGapCount ||
		report.ClaimedNonUnchangedCategoryCount != report.RecomputedNonUnchangedCategoryCount {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureCount)
	}
	if len(report.Categories) != report.ExpectedCategoryCount {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureCategory)
	}
	for index, category := range report.Categories {
		expected := verificationEvidenceExpectations()[index]
		if category.Category != expected.category || category.Source != expected.source || !category.Verified ||
			category.ClaimedState != category.RecomputedState ||
			category.ClaimedCurrentUnavailable != category.RecomputedCurrentUnavailable {
			return reject(SafetyEvidenceGapInventoryVerificationReviewFailureCategory)
		}
	}

	disposition, ownerAction := gapInventoryVerificationReviewConclusion(report)
	artifact.Status = SafetyEvidenceGapInventoryVerificationReviewReady
	artifact.Failure = SafetyEvidenceGapInventoryVerificationReviewFailureNone
	artifact.Disposition = disposition
	artifact.OwnerAction = ownerAction
	artifact.VerificationReportVersion = report.ReportVersion
	artifact.VerificationReportStatus = report.Status
	artifact.VerificationReportFailure = report.Failure
	artifact.BaselineEvaluatedAt = report.BaselineEvaluatedAt
	artifact.CurrentEvaluatedAt = report.CurrentEvaluatedAt
	artifact.CurrentEvidenceAvailable = report.CurrentEvidenceAvailable
	artifact.ExpectedCategoryCount = report.ExpectedCategoryCount
	artifact.CurrentGapCount = report.RecomputedCurrentGapCount
	artifact.NonUnchangedCategoryCount = report.RecomputedNonUnchangedCategoryCount
	artifact.ReviewDigest = report.ReviewDigest
	artifact.InventoryDigest = report.InventoryDigest
	artifact.Categories = append([]SafetyEvidenceGapCategoryVerification(nil), report.Categories...)
	artifact.VerificationReportDigest = report.ReportDigest
	digest, err := canonicalSafetyEvidenceGapInventoryVerificationReviewDigest(artifact)
	if err != nil {
		return reject(SafetyEvidenceGapInventoryVerificationReviewFailureCompilation)
	}
	artifact.ArtifactDigest = digest
	return artifact
}

// VerifySafetyEvidenceGapInventoryVerificationReview recomputes the complete
// artifact. Any changed input, conclusion, count, category, or digest fails
// closed.
func VerifySafetyEvidenceGapInventoryVerificationReview(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	artifact SafetyEvidenceGapInventoryVerificationReview,
) error {
	if artifact.ArtifactVersion != SafetyEvidenceGapInventoryVerificationReviewVersion ||
		artifact.Status != SafetyEvidenceGapInventoryVerificationReviewReady ||
		artifact.Failure != SafetyEvidenceGapInventoryVerificationReviewFailureNone ||
		artifact.ExecutionAuthority || !digestPattern.MatchString(artifact.VerificationReportDigest) ||
		!digestPattern.MatchString(artifact.ArtifactDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(artifact)) {
		return ErrSafetyEvidenceGapInventoryVerificationReview
	}
	expected := BuildSafetyEvidenceGapInventoryVerificationReview(baseline, current, envelope, audit, review, inventory, report)
	if expected.Status != SafetyEvidenceGapInventoryVerificationReviewReady || !reflect.DeepEqual(artifact, expected) {
		return ErrSafetyEvidenceGapInventoryVerificationReview
	}
	return nil
}

func validGapInventoryVerificationReportConclusion(report SafetyEvidenceGapInventoryVerificationReport) bool {
	if report.Status == VerificationVerified && report.Failure == SafetyEvidenceGapInventoryVerificationFailureNone {
		return report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount == 0
	}
	if report.Status == VerificationRejected && report.Failure == SafetyEvidenceGapInventoryVerificationFailureUnavailable {
		return !report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount > 0
	}
	return false
}

func gapInventoryVerificationReviewConclusion(report SafetyEvidenceGapInventoryVerificationReport) (
	SafetyEvidenceGapInventoryVerificationReviewDisposition,
	SafetyEvidenceGapInventoryVerificationReviewOwnerAction,
) {
	if report.CurrentEvidenceAvailable {
		return SafetyEvidenceGapInventoryVerificationReviewNoGaps, SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone
	}
	return SafetyEvidenceGapInventoryVerificationReviewOwnerReview, SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps
}

func canonicalSafetyEvidenceGapInventoryVerificationReviewDigest(artifact SafetyEvidenceGapInventoryVerificationReview) (string, error) {
	artifact.ArtifactDigest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapInventoryVerificationReviewDigestDomain, payload), nil
}
