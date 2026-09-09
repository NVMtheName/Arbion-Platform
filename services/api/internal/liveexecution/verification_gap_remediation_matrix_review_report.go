package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationMatrixVerificationReviewReportVersion = "live-safety-evidence-gap-remediation-matrix-verification-review-report-v1"

const safetyEvidenceGapRemediationMatrixVerificationReviewReportDigestDomain = "arbion:live-safety:evidence-gap-remediation-matrix-verification-review-report:v1"

var ErrSafetyEvidenceGapRemediationMatrixVerificationReviewReport = errors.New("live safety evidence gap remediation matrix verification review report is invalid")

type SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure string

const (
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureNone            SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "NONE"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureSecret          SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureUpstream        SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "UPSTREAM_REPORT_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactVersion SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "ARTIFACT_VERSION_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactStatus  SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "ARTIFACT_STATUS_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureAuthority       SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureConclusion      SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "REVIEW_CONCLUSION_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCount           SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "COUNT_EVIDENCE_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureRequirement     SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "REQUIREMENT_EVIDENCE_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureReportDigest    SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "VERIFICATION_REPORT_DIGEST_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureMatrixDigest    SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "MATRIX_DIGEST_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMalformed SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "ARTIFACT_DIGEST_MALFORMED"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMismatch  SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "ARTIFACT_DIGEST_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureContent         SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "ARTIFACT_CONTENT_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCompilation     SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure = "REPORT_COMPILATION_FAILED"
)

// SafetyEvidenceGapRemediationMatrixVerificationReviewReport independently
// verifies the canonical owner-review artifact against its complete upstream
// chain. VERIFIED means structural integrity only and never means live
// readiness, remediation completion, approval, or execution authority.
type SafetyEvidenceGapRemediationMatrixVerificationReviewReport struct {
	ReportVersion              string                                                            `json:"report_version"`
	Status                     VerificationStatus                                                `json:"status"`
	Failure                    SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure `json:"failure"`
	ExecutionAuthority         bool                                                              `json:"execution_authority"`
	ArtifactVersion            VersionVerification                                               `json:"artifact_version"`
	ClaimedDisposition         SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition   `json:"claimed_disposition"`
	RecomputedDisposition      SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition   `json:"recomputed_disposition"`
	ClaimedOwnerAction         SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction   `json:"claimed_owner_action"`
	RecomputedOwnerAction      SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction   `json:"recomputed_owner_action"`
	BaselineEvaluatedAt        time.Time                                                         `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt         time.Time                                                         `json:"current_evaluated_at"`
	CurrentEvidenceAvailable   bool                                                              `json:"current_evidence_available"`
	ExpectedCategoryCount      int                                                               `json:"expected_category_count"`
	ClaimedCurrentGapCount     int                                                               `json:"claimed_current_gap_count"`
	RecomputedCurrentGapCount  int                                                               `json:"recomputed_current_gap_count"`
	ClaimedRequirementCount    int                                                               `json:"claimed_requirement_count"`
	RecomputedRequirementCount int                                                               `json:"recomputed_requirement_count"`
	VerificationReportDigest   DigestVerification                                                `json:"verification_report_digest"`
	MatrixDigest               DigestVerification                                                `json:"matrix_digest"`
	ArtifactDigest             DigestVerification                                                `json:"artifact_digest"`
	Requirements               []SafetyEvidenceGapRemediationRequirementVerification             `json:"requirements"`
	ReportDigest               string                                                            `json:"report_sha256"`
}

// BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport reconstructs
// the review conclusion, counts, requirements, and digests without calling the
// review compiler, verifier, conclusion helper, or digest helper.
func BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport(
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
) SafetyEvidenceGapRemediationMatrixVerificationReviewReport {
	result := SafetyEvidenceGapRemediationMatrixVerificationReviewReport{
		ReportVersion: SafetyEvidenceGapRemediationMatrixVerificationReviewReportVersion,
		Status:        VerificationRejected,
		Failure:       SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailure) SafetyEvidenceGapRemediationMatrixVerificationReviewReport {
		result.Status = VerificationRejected
		result.Failure = failure
		return result
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
		safetyCaseHasSecretLikeValue(reflect.ValueOf(verificationReport)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(artifact)) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureSecret)
	}
	if baseline.ExecutionAuthority || current.ExecutionAuthority || envelope.Comparison.ExecutionAuthority ||
		audit.ExecutionAuthority || review.ExecutionAuthority || inventory.ExecutionAuthority || report.ExecutionAuthority ||
		verificationReview.ExecutionAuthority || matrix.ExecutionAuthority || verificationReport.ExecutionAuthority || artifact.ExecutionAuthority {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureAuthority)
	}
	if VerifySafetyEvidenceGapRemediationMatrixVerificationReport(
		baseline, current, envelope, audit, review, inventory, report, verificationReview, matrix, verificationReport,
	) != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureUpstream)
	}
	result.ArtifactVersion = VersionVerification{
		Expected: SafetyEvidenceGapRemediationMatrixVerificationReviewVersion,
		Claimed:  artifact.ArtifactVersion,
		Verified: artifact.ArtifactVersion == SafetyEvidenceGapRemediationMatrixVerificationReviewVersion,
	}
	if !result.ArtifactVersion.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactVersion)
	}
	if artifact.Status != SafetyEvidenceGapRemediationMatrixVerificationReviewReady ||
		artifact.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureArtifactStatus)
	}

	disposition, ownerAction, ok := independentlyExpectedRemediationOwnerReviewConclusion(verificationReport)
	if !ok {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureConclusion)
	}
	result.ClaimedDisposition = artifact.Disposition
	result.RecomputedDisposition = disposition
	result.ClaimedOwnerAction = artifact.OwnerAction
	result.RecomputedOwnerAction = ownerAction
	if result.ClaimedDisposition != result.RecomputedDisposition || result.ClaimedOwnerAction != result.RecomputedOwnerAction {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureConclusion)
	}
	if artifact.VerificationReportStatus != verificationReport.Status ||
		artifact.VerificationReportFailure != verificationReport.Failure ||
		!artifact.BaselineEvaluatedAt.Equal(verificationReport.BaselineEvaluatedAt) ||
		!artifact.CurrentEvaluatedAt.Equal(verificationReport.CurrentEvaluatedAt) ||
		artifact.CurrentEvidenceAvailable != verificationReport.CurrentEvidenceAvailable {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureContent)
	}
	result.BaselineEvaluatedAt = verificationReport.BaselineEvaluatedAt
	result.CurrentEvaluatedAt = verificationReport.CurrentEvaluatedAt
	result.CurrentEvidenceAvailable = verificationReport.CurrentEvidenceAvailable
	result.ExpectedCategoryCount = verificationReport.ExpectedCategoryCount
	result.ClaimedCurrentGapCount = artifact.CurrentGapCount
	result.RecomputedCurrentGapCount = verificationReport.RecomputedCurrentGapCount
	result.ClaimedRequirementCount = artifact.RequirementCount
	result.RecomputedRequirementCount = verificationReport.ExpectedRequirementCount
	if artifact.ExpectedCategoryCount != verificationReport.ExpectedCategoryCount ||
		result.ClaimedCurrentGapCount != result.RecomputedCurrentGapCount ||
		result.ClaimedRequirementCount != result.RecomputedRequirementCount ||
		len(artifact.Requirements) != result.RecomputedRequirementCount ||
		len(verificationReport.Requirements) != result.RecomputedRequirementCount {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCount)
	}

	result.VerificationReportDigest = DigestVerification{
		Claimed: artifact.VerificationReportDigest, Recomputed: verificationReport.ReportDigest,
		Expected: verificationReport.ReportDigest,
		Verified: artifact.VerificationReportDigest == verificationReport.ReportDigest &&
			digestPattern.MatchString(artifact.VerificationReportDigest),
	}
	if !result.VerificationReportDigest.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureReportDigest)
	}
	result.MatrixDigest = DigestVerification{
		Claimed: artifact.MatrixDigest.Claimed, Recomputed: verificationReport.MatrixDigest.Recomputed,
		Expected: matrix.MatrixDigest,
		Verified: reflect.DeepEqual(artifact.MatrixDigest, verificationReport.MatrixDigest) &&
			artifact.MatrixDigest.Claimed == matrix.MatrixDigest && digestPattern.MatchString(matrix.MatrixDigest),
	}
	if !result.MatrixDigest.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureMatrixDigest)
	}

	result.Requirements = make([]SafetyEvidenceGapRemediationRequirementVerification, 0, result.RecomputedRequirementCount)
	var expectedRequirements []SafetyEvidenceGapRemediationRequirement
	if result.RecomputedRequirementCount > 0 {
		expectedRequirements = make([]SafetyEvidenceGapRemediationRequirement, 0, result.RecomputedRequirementCount)
	}
	for index, upstream := range verificationReport.Requirements {
		if !upstream.Verified || !reflect.DeepEqual(upstream.Claimed, upstream.Recomputed) ||
			!reflect.DeepEqual(upstream.Recomputed, matrix.Requirements[index]) {
			return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureRequirement)
		}
		expected := cloneRemediationRequirement(upstream.Recomputed)
		claimed := artifact.Requirements[index]
		verified := reflect.DeepEqual(claimed, expected)
		result.Requirements = append(result.Requirements, SafetyEvidenceGapRemediationRequirementVerification{
			Claimed: claimed, Recomputed: expected, Verified: verified,
		})
		if !verified {
			return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureRequirement)
		}
		expectedRequirements = append(expectedRequirements, expected)
	}

	expectedArtifact := SafetyEvidenceGapRemediationMatrixVerificationReview{
		ArtifactVersion:           SafetyEvidenceGapRemediationMatrixVerificationReviewVersion,
		Status:                    SafetyEvidenceGapRemediationMatrixVerificationReviewReady,
		Failure:                   SafetyEvidenceGapRemediationMatrixVerificationReviewFailureNone,
		Disposition:               disposition,
		OwnerAction:               ownerAction,
		VerificationReportStatus:  verificationReport.Status,
		VerificationReportFailure: verificationReport.Failure,
		BaselineEvaluatedAt:       verificationReport.BaselineEvaluatedAt,
		CurrentEvaluatedAt:        verificationReport.CurrentEvaluatedAt,
		CurrentEvidenceAvailable:  verificationReport.CurrentEvidenceAvailable,
		ExpectedCategoryCount:     verificationReport.ExpectedCategoryCount,
		CurrentGapCount:           verificationReport.RecomputedCurrentGapCount,
		RequirementCount:          verificationReport.ExpectedRequirementCount,
		VerificationReportDigest:  verificationReport.ReportDigest,
		MatrixDigest:              verificationReport.MatrixDigest,
		Requirements:              expectedRequirements,
	}
	recomputedClaimedDigest, err := independentlyCanonicalRemediationOwnerReviewDigest(artifact)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCompilation)
	}
	expectedDigest, err := independentlyCanonicalRemediationOwnerReviewDigest(expectedArtifact)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCompilation)
	}
	expectedArtifact.ArtifactDigest = expectedDigest
	result.ArtifactDigest = DigestVerification{
		Claimed: artifact.ArtifactDigest, Recomputed: recomputedClaimedDigest, Expected: expectedDigest,
		Verified: artifact.ArtifactDigest == recomputedClaimedDigest && artifact.ArtifactDigest == expectedDigest,
	}
	if !digestPattern.MatchString(artifact.ArtifactDigest) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMalformed)
	}
	if !result.ArtifactDigest.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureDigestMismatch)
	}
	if !reflect.DeepEqual(artifact, expectedArtifact) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureContent)
	}
	result.Status = VerificationVerified
	result.Failure = SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureNone
	result.ReportDigest, err = canonicalSafetyEvidenceGapRemediationMatrixVerificationReviewReportDigest(result)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureCompilation)
	}
	return result
}

func VerifySafetyEvidenceGapRemediationMatrixVerificationReviewReport(
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
	result SafetyEvidenceGapRemediationMatrixVerificationReviewReport,
) error {
	if result.ReportVersion != SafetyEvidenceGapRemediationMatrixVerificationReviewReportVersion ||
		result.Status != VerificationVerified ||
		result.Failure != SafetyEvidenceGapRemediationMatrixVerificationReviewReportFailureNone ||
		result.ExecutionAuthority || !digestPattern.MatchString(result.ReportDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(result)) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReviewReport
	}
	expected := BuildSafetyEvidenceGapRemediationMatrixVerificationReviewReport(
		baseline, current, envelope, audit, review, inventory, report, verificationReview,
		matrix, verificationReport, artifact,
	)
	if !reflect.DeepEqual(result, expected) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReviewReport
	}
	return nil
}

func independentlyExpectedRemediationOwnerReviewConclusion(
	report SafetyEvidenceGapRemediationMatrixVerificationReport,
) (SafetyEvidenceGapRemediationMatrixVerificationReviewDisposition, SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerAction, bool) {
	if report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount == 0 &&
		report.ExpectedRequirementCount == 0 && len(report.Requirements) == 0 {
		return SafetyEvidenceGapRemediationMatrixVerificationReviewNoRemediation,
			SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionNone, true
	}
	if !report.CurrentEvidenceAvailable && report.RecomputedCurrentGapCount > 0 &&
		report.ExpectedRequirementCount == report.RecomputedCurrentGapCount &&
		len(report.Requirements) == report.ExpectedRequirementCount {
		return SafetyEvidenceGapRemediationMatrixVerificationReviewRequired,
			SafetyEvidenceGapRemediationMatrixVerificationReviewOwnerActionReview, true
	}
	return "", "", false
}

func cloneRemediationRequirement(value SafetyEvidenceGapRemediationRequirement) SafetyEvidenceGapRemediationRequirement {
	value.ReasonCodes = append([]ReasonCode(nil), value.ReasonCodes...)
	return value
}

func independentlyCanonicalRemediationOwnerReviewDigest(
	artifact SafetyEvidenceGapRemediationMatrixVerificationReview,
) (string, error) {
	artifact.ArtifactDigest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256("arbion:live-safety:evidence-gap-remediation-matrix-verification-review:v1", payload), nil
}

func canonicalSafetyEvidenceGapRemediationMatrixVerificationReviewReportDigest(
	report SafetyEvidenceGapRemediationMatrixVerificationReviewReport,
) (string, error) {
	report.ReportDigest = ""
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationMatrixVerificationReviewReportDigestDomain, payload), nil
}
