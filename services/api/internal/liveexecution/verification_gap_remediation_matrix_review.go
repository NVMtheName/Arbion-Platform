package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationMatrixVerificationReviewVersion = "live-safety-evidence-gap-remediation-matrix-verification-review-v1"

const safetyEvidenceGapRemediationMatrixVerificationReviewDigestDomain = "arbion:live-safety:evidence-gap-remediation-matrix-verification-review:v1"

var ErrSafetyEvidenceGapRemediationMatrixVerificationReview = errors.New("live safety evidence gap remediation matrix verification review is invalid")

type SafetyEvidenceGapRemediationMatrixVerificationReviewStatus string

const (
	SafetyEvidenceGapRemediationMatrixVerificationReviewReady    SafetyEvidenceGapRemediationMatrixVerificationReviewStatus = "REVIEW_READY"
	SafetyEvidenceGapRemediationMatrixVerificationReviewRejected SafetyEvidenceGapRemediationMatrixVerificationReviewStatus = "REJECTED"
)

type SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition string

const (
	SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition = "NO_REMEDIATION_REQUIRED"
	SafetyEvidenceGapRemediationMatrixVerificationReviewRequired      SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition = "EVIDENCE_REMEDIATION_REVIEW_REQUIRED"
)

type SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction string

const (
	SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone   SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction = "NONE"
	SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction = "REVIEW_EXACT_REMEDIATION_REQUIREMENTS"
)

type SafetyEvidenceGapRemediationMatrixVerificationReviewFailure string

const (
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone        SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "NONE"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureSecret      SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureAuthority   SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureReport      SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "VERIFICATION_REPORT_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureConclusion  SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "REVIEW_CONCLUSION_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureEvidence    SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "REQUIREMENT_EVIDENCE_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewFailureCompilation SafetyEvidenceGapRemediationMatrixVerificationReviewFailure = "REVIEW_COMPILATION_FAILED"
)

// SafetyEvidenceGapRemediationMatrixVerificationReview is an owner-facing
// conclusion over one exact independently verified remediation matrix. A
// review-ready artifact confirms structural integrity only: it does not claim
// live readiness, remediation completion, approval, or execution authority.
type SafetyEvidenceGapRemediationMatrixVerificationReview struct {
	ArtifactVersion           string                                                          `json:"artifact_version"`
	Status                    SafetyEvidenceGapRemediationMatrixVerificationReviewStatus      `json:"status"`
	Failure                   SafetyEvidenceGapRemediationMatrixVerificationReviewFailure     `json:"failure"`
	ExecutionAuthority        bool                                                            `json:"execution_authority"`
	Disposition               SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition `json:"disposition"`
	OwnerAction               SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction `json:"owner_action"`
	VerificationReportStatus  VerificationStatus                                              `json:"verification_report_status"`
	VerificationReportFailure SafetyEvidenceGapRemediationMatrixVerificationFailure           `json:"verification_report_failure"`
	BaselineEvaluatedAt       time.Time                                                       `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt        time.Time                                                       `json:"current_evaluated_at"`
	CurrentEvidenceAvailable  bool                                                            `json:"current_evidence_available"`
	ExpectedCategoryCount     int                                                             `json:"expected_category_count"`
	CurrentGapCount           int                                                             `json:"current_gap_count"`
	RequirementCount          int                                                             `json:"requirement_count"`
	VerificationReportDigest  string                                                          `json:"verification_report_sha256"`
	MatrixDigest              DigestVerification                                              `json:"matrix_digest"`
	Requirements              []SafetyEvidenceGapRemediationRequirement                       `json:"requirements"`
	ArtifactDigest            string                                                          `json:"artifact_sha256"`
}

// BuildSafetyEvidenceGapRemediationMatrixVerificationReview accepts only the
// exact complete matrix-verification chain. It can request owner review of
// exact evidence requirements but cannot approve or execute remediation.
func BuildSafetyEvidenceGapRemediationMatrixVerificationReview(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	matrix SafetyEvidenceGapRemediationMatrix,
	verificationReport SafetyEvidenceGapRemediationMatrixVerificationReport,
) SafetyEvidenceGapRemediationMatrixVerificationReview {
	artifact := SafetyEvidenceGapRemediationMatrixVerificationReview{
		ArtifactVersion: SafetyEvidenceGapRemediationMatrixVerificationReviewVersion,
		Status:          SafetyEvidenceGapRemediationMatrixVerificationReviewRejected,
		Failure:         SafetyEvidenceGapRemediationMatrixVerificationReviewFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationMatrixVerificationReviewFailure) SafetyEvidenceGapRemediationMatrixVerificationReview {
		artifact.Status = SafetyEvidenceGapRemediationMatrixVerificationReviewRejected
		artifact.Failure = failure
		return artifact
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(review)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(inventory)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(verificationReview)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(matrix)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(verificationReport)) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureSecret)
	}
	if baseline.ExecutionAuthority || current.ExecutionAuthority || envelope.Comparison.ExecutionAuthority ||
		audit.ExecutionAuthority || review.ExecutionAuthority || inventory.ExecutionAuthority || report.ExecutionAuthority ||
		verificationReview.ExecutionAuthority || matrix.ExecutionAuthority || verificationReport.ExecutionAuthority {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureAuthority)
	}
	if verificationReport.ReportVersion != SafetyEvidenceGapRemediationMatrixVerificationReportVersion ||
		verificationReport.Status != VerificationVerified ||
		verificationReport.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureNone ||
		VerifySafetyEvidenceGapRemediationMatrixVerificationReport(
			baseline, current, envelope, audit, review, inventory, report, verificationReview, matrix, verificationReport,
		) != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureReport)
	}
	if !digestPattern.MatchString(verificationReport.ReportDigest) || !verificationReport.MatrixDigest.Verified ||
		!digestPattern.MatchString(verificationReport.MatrixDigest.Claimed) ||
		verificationReport.MatrixDigest.Claimed != matrix.MatrixDigest {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureEvidence)
	}
	if verificationReport.ClaimedCurrentGapCount != verificationReport.RecomputedCurrentGapCount ||
		verificationReport.ClaimedRequirementCount != verificationReport.ExpectedRequirementCount ||
		verificationReport.RecomputedCurrentGapCount != verificationReport.ExpectedRequirementCount ||
		len(verificationReport.Requirements) != verificationReport.ExpectedRequirementCount ||
		len(matrix.Requirements) != verificationReport.ExpectedRequirementCount {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureEvidence)
	}
	for index, requirement := range verificationReport.Requirements {
		if !requirement.Verified || !reflect.DeepEqual(requirement.Claimed, requirement.Recomputed) ||
			!reflect.DeepEqual(requirement.Claimed, matrix.Requirements[index]) {
			return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureEvidence)
		}
	}

	disposition, ownerAction, ok := remediationMatrixVerificationReviewConclusion(verificationReport)
	if !ok {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureConclusion)
	}
	artifact.Status = SafetyEvidenceGapRemediationMatrixVerificationReviewReady
	artifact.Failure = SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone
	artifact.Disposition = disposition
	artifact.OwnerAction = ownerAction
	artifact.VerificationReportStatus = verificationReport.Status
	artifact.VerificationReportFailure = verificationReport.Failure
	artifact.BaselineEvaluatedAt = verificationReport.BaselineEvaluatedAt
	artifact.CurrentEvaluatedAt = verificationReport.CurrentEvaluatedAt
	artifact.CurrentEvidenceAvailable = verificationReport.CurrentEvidenceAvailable
	artifact.ExpectedCategoryCount = verificationReport.ExpectedCategoryCount
	artifact.CurrentGapCount = verificationReport.RecomputedCurrentGapCount
	artifact.RequirementCount = verificationReport.ExpectedRequirementCount
	artifact.VerificationReportDigest = verificationReport.ReportDigest
	artifact.MatrixDigest = verificationReport.MatrixDigest
	artifact.Requirements = append([]SafetyEvidenceGapRemediationRequirement(nil), matrix.Requirements...)
	digest, err := canonicalSafetyEvidenceGapRemediationMatrixVerificationReviewDigest(artifact)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewFailureCompilation)
	}
	artifact.ArtifactDigest = digest
	return artifact
}

func VerifySafetyEvidenceGapRemediationMatrixVerificationReview(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	matrix SafetyEvidenceGapRemediationMatrix,
	verificationReport SafetyEvidenceGapRemediationMatrixVerificationReport,
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
) error {
	if artifact.ArtifactVersion != SafetyEvidenceGapRemediationMatrixVerificationReviewVersion ||
		artifact.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewReady ||
		artifact.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone ||
		artifact.ExecutionAuthority || !digestPattern.MatchString(artifact.VerificationReportDigest) ||
		!digestPattern.MatchString(artifact.ArtifactDigest) || safetyCaseHasSecretLikeValue(reflect.ValueOf(artifact)) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReview
	}
	expected := BuildSafetyEvidenceGapRemediationMatrixVerificationReview(
		baseline, current, envelope, audit, review, inventory, report, verificationReview, matrix, verificationReport,
	)
	if expected.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewReady || !reflect.DeepEqual(artifact, expected) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReview
	}
	return nil
}

func remediationMatrixVerificationReviewConclusion(report SafetyEvidenceGapRemediationMatrixVerificationReport) (
	SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition,
	SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction,
	bool,
) {
	if report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount == 0 && report.ExpectedRequirementCount == 0 {
		return SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation,
			SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone, true
	}
	if !report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount > 0 &&
		report.ExpectedRequirementCount == report.RecomputedCurrentGapCount {
		return SafetyEvidenceGapRemediationMatrixVerificationReviewRequired,
			SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview, true
	}
	return "", "", false
}

func canonicalSafetyEvidenceGapRemediationMatrixVerificationReviewDigest(
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
) (string, error) {
	artifact.ArtifactDigest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationMatrixVerificationReviewDigestDomain, payload), nil
}
