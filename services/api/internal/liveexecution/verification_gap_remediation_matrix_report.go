package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationMatrixVerificationReportVersion = "live-safety-evidence-gap-remediation-matrix-verification-v1"

const safetyEvidenceGapRemediationMatrixVerificationReportDigestDomain = "arbion:live-safety:evidence-gap-remediation-matrix-verification:v1"

var ErrSafetyEvidenceGapRemediationMatrixVerificationReport = errors.New("live safety evidence gap remediation matrix verification report is invalid")

type SafetyEvidenceGapRemediationMatrixVerificationFailure string

const (
	SafetyEvidenceGapRemediationMatrixVerificationFailureNone            SafetyEvidenceGapRemediationMatrixVerificationFailure = "NONE"
	SafetyEvidenceGapRemediationMatrixVerificationFailureSecretInput     SafetyEvidenceGapRemediationMatrixVerificationFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationMatrixVerificationFailureUpstream        SafetyEvidenceGapRemediationMatrixVerificationFailure = "UPSTREAM_REVIEW_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixVersion   SafetyEvidenceGapRemediationMatrixVerificationFailure = "MATRIX_VERSION_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixStatus    SafetyEvidenceGapRemediationMatrixVerificationFailure = "MATRIX_STATUS_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationFailureAuthority       SafetyEvidenceGapRemediationMatrixVerificationFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationMatrixVerificationFailureDisposition     SafetyEvidenceGapRemediationMatrixVerificationFailure = "REVIEW_DISPOSITION_INVALID"
	SafetyEvidenceGapRemediationMatrixVerificationFailureCount           SafetyEvidenceGapRemediationMatrixVerificationFailure = "REQUIREMENT_COUNT_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationFailureCategory        SafetyEvidenceGapRemediationMatrixVerificationFailure = "REQUIREMENT_CONTENT_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMalformed SafetyEvidenceGapRemediationMatrixVerificationFailure = "MATRIX_DIGEST_MALFORMED"
	SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMismatch  SafetyEvidenceGapRemediationMatrixVerificationFailure = "MATRIX_DIGEST_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch  SafetyEvidenceGapRemediationMatrixVerificationFailure = "MATRIX_CONTENT_MISMATCH"
	SafetyEvidenceGapRemediationMatrixVerificationFailureCompilation     SafetyEvidenceGapRemediationMatrixVerificationFailure = "REPORT_COMPILATION_FAILED"
)

// SafetyEvidenceGapRemediationRequirementVerification preserves the claimed
// row beside its independently reconstructed contract row.
type SafetyEvidenceGapRemediationRequirementVerification struct {
	Claimed    SafetyEvidenceGapRemediationRequirement `json:"claimed"`
	Recomputed SafetyEvidenceGapRemediationRequirement `json:"recomputed"`
	Verified   bool                                    `json:"verified"`
}

// SafetyEvidenceGapRemediationMatrixVerificationReport independently verifies
// the exact matrix, its complete upstream binding, and every remediation row.
// It reports evidence integrity only and grants no readiness or authority.
type SafetyEvidenceGapRemediationMatrixVerificationReport struct {
	ReportVersion             string                                                  `json:"report_version"`
	Status                    VerificationStatus                                      `json:"status"`
	Failure                   SafetyEvidenceGapRemediationMatrixVerificationFailure   `json:"failure"`
	ExecutionAuthority        bool                                                    `json:"execution_authority"`
	MatrixVersion             VersionVerification                                     `json:"matrix_version"`
	ReviewDisposition         SafetyEvidenceGapInventoryVerificationReviewDisposition `json:"review_disposition"`
	ReviewOwnerAction         SafetyEvidenceGapInventoryVerificationReviewOwnerAction `json:"review_owner_action"`
	BaselineEvaluatedAt       time.Time                                               `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt        time.Time                                               `json:"current_evaluated_at"`
	VerificationReportDigest  DigestVerification                                      `json:"verification_report_digest"`
	VerificationReviewDigest  DigestVerification                                      `json:"verification_review_digest"`
	InventoryDigest           DigestVerification                                      `json:"inventory_digest"`
	UpstreamReviewDigest      DigestVerification                                      `json:"upstream_review_digest"`
	MatrixDigest              DigestVerification                                      `json:"matrix_digest"`
	ExpectedCategoryCount     int                                                     `json:"expected_category_count"`
	ClaimedCurrentGapCount    int                                                     `json:"claimed_current_gap_count"`
	RecomputedCurrentGapCount int                                                     `json:"recomputed_current_gap_count"`
	ClaimedRequirementCount   int                                                     `json:"claimed_requirement_count"`
	ExpectedRequirementCount  int                                                     `json:"expected_requirement_count"`
	CurrentEvidenceAvailable  bool                                                    `json:"current_evidence_available"`
	Requirements              []SafetyEvidenceGapRemediationRequirementVerification   `json:"requirements"`
	ReportDigest              string                                                  `json:"report_sha256"`
}

// BuildSafetyEvidenceGapRemediationMatrixVerificationReport reconstructs the
// matrix without calling its compiler, verifier, mapping, or digest helper.
func BuildSafetyEvidenceGapRemediationMatrixVerificationReport(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	matrix SafetyEvidenceGapRemediationMatrix,
) SafetyEvidenceGapRemediationMatrixVerificationReport {
	result := SafetyEvidenceGapRemediationMatrixVerificationReport{
		ReportVersion: SafetyEvidenceGapRemediationMatrixVerificationReportVersion,
		Status:        VerificationRejected,
		Failure:       SafetyEvidenceGapRemediationMatrixVerificationFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationMatrixVerificationFailure) SafetyEvidenceGapRemediationMatrixVerificationReport {
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
		safetyCaseHasSecretLikeValue(reflect.ValueOf(matrix)) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureSecretInput)
	}
	if matrix.ExecutionAuthority || verificationReview.ExecutionAuthority || report.ExecutionAuthority || inventory.ExecutionAuthority ||
		baseline.ExecutionAuthority || current.ExecutionAuthority || envelope.Comparison.ExecutionAuthority ||
		audit.ExecutionAuthority || review.ExecutionAuthority {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureAuthority)
	}
	if VerifySafetyEvidenceGapInventoryVerificationReview(
		baseline, current, envelope, audit, review, inventory, report, verificationReview,
	) != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureUpstream)
	}
	result.MatrixVersion = VersionVerification{
		Expected: SafetyEvidenceGapRemediationMatrixVersion,
		Claimed:  matrix.MatrixVersion,
		Verified: matrix.MatrixVersion == SafetyEvidenceGapRemediationMatrixVersion,
	}
	if !result.MatrixVersion.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixVersion)
	}
	if matrix.Status != SafetyEvidenceGapRemediationMatrixCompiled ||
		matrix.Failure != SafetyEvidenceGapRemediationMatrixFailureNone {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixStatus)
	}
	if !independentlyValidRemediationReviewDisposition(verificationReview) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureDisposition)
	}

	expected, ok := independentlyRecomputeSafetyEvidenceGapRemediationMatrix(verificationReview, inventory)
	if !ok {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCategory)
	}
	result.ReviewDisposition = verificationReview.Disposition
	result.ReviewOwnerAction = verificationReview.OwnerAction
	result.BaselineEvaluatedAt = verificationReview.BaselineEvaluatedAt
	result.CurrentEvaluatedAt = verificationReview.CurrentEvaluatedAt
	result.VerificationReportDigest = exactMatrixDigestVerification(matrix.VerificationReportDigest, verificationReview.VerificationReportDigest)
	result.VerificationReviewDigest = exactMatrixDigestVerification(matrix.VerificationReviewDigest, verificationReview.ArtifactDigest)
	result.InventoryDigest = exactMatrixDigestVerification(matrix.InventoryDigest, inventory.InventoryDigest)
	result.UpstreamReviewDigest = exactMatrixDigestVerification(matrix.UpstreamReviewDigest, review.ReviewDigest)
	if !result.VerificationReportDigest.Verified || !result.VerificationReviewDigest.Verified ||
		!result.InventoryDigest.Verified || !result.UpstreamReviewDigest.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch)
	}
	result.ExpectedCategoryCount = len(independentRemediationExpectations())
	result.ClaimedCurrentGapCount = matrix.CurrentGapCount
	result.RecomputedCurrentGapCount = inventory.CurrentGapCount
	result.ClaimedRequirementCount = len(matrix.Requirements)
	result.ExpectedRequirementCount = len(expected.Requirements)
	if matrix.ExpectedCategoryCount != result.ExpectedCategoryCount ||
		result.ClaimedCurrentGapCount != result.RecomputedCurrentGapCount ||
		result.ClaimedRequirementCount != result.ExpectedRequirementCount ||
		result.ClaimedRequirementCount != result.RecomputedCurrentGapCount {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCount)
	}
	result.Requirements = make([]SafetyEvidenceGapRemediationRequirementVerification, 0, len(expected.Requirements))
	for index, recomputed := range expected.Requirements {
		claimed := matrix.Requirements[index]
		verified := reflect.DeepEqual(claimed, recomputed)
		result.Requirements = append(result.Requirements, SafetyEvidenceGapRemediationRequirementVerification{
			Claimed: claimed, Recomputed: recomputed, Verified: verified,
		})
		if !verified {
			return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCategory)
		}
	}

	recomputedClaimedDigest, err := independentlyCanonicalSafetyEvidenceGapRemediationMatrixDigest(matrix)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCompilation)
	}
	expectedDigest, err := independentlyCanonicalSafetyEvidenceGapRemediationMatrixDigest(expected)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCompilation)
	}
	result.MatrixDigest = DigestVerification{
		Claimed: matrix.MatrixDigest, Recomputed: recomputedClaimedDigest, Expected: expectedDigest,
		Verified: matrix.MatrixDigest == recomputedClaimedDigest && matrix.MatrixDigest == expectedDigest,
	}
	if !digestPattern.MatchString(matrix.MatrixDigest) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMalformed)
	}
	if !result.MatrixDigest.Verified {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureDigestMismatch)
	}
	if !reflect.DeepEqual(matrix, expected) {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureMatrixMismatch)
	}
	result.CurrentEvidenceAvailable = result.RecomputedCurrentGapCount == 0
	result.Status = VerificationVerified
	result.Failure = SafetyEvidenceGapRemediationMatrixVerificationFailureNone
	result.ReportDigest, err = canonicalSafetyEvidenceGapRemediationMatrixVerificationReportDigest(result)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixVerificationFailureCompilation)
	}
	return result
}

func VerifySafetyEvidenceGapRemediationMatrixVerificationReport(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	matrix SafetyEvidenceGapRemediationMatrix,
	verificationReport SafetyEvidenceGapRemediationMatrixVerificationReport,
) error {
	if verificationReport.ReportVersion != SafetyEvidenceGapRemediationMatrixVerificationReportVersion ||
		verificationReport.Status != VerificationVerified ||
		verificationReport.Failure != SafetyEvidenceGapRemediationMatrixVerificationFailureNone ||
		verificationReport.ExecutionAuthority || !digestPattern.MatchString(verificationReport.ReportDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(verificationReport)) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReport
	}
	expected := BuildSafetyEvidenceGapRemediationMatrixVerificationReport(
		baseline, current, envelope, audit, review, inventory, report, verificationReview, matrix,
	)
	if !reflect.DeepEqual(verificationReport, expected) {
		return ErrSafetyEvidenceGapRemediationMatrixVerificationReport
	}
	return nil
}

type independentRemediationExpectation struct {
	category string
	source   EvidenceSource
	boundary SafetyEvidenceGapResponsibleBoundary
	followUp SafetyEvidenceGapFollowUp
}

func independentRemediationExpectations() []independentRemediationExpectation {
	return []independentRemediationExpectation{
		{"PROVIDER_CAPABILITY", EvidenceProviderVerified, SafetyEvidenceGapBoundaryFinancialProvider, SafetyEvidenceGapFollowUpVerifyProvider},
		{"OWNER_AUTHORIZATION", EvidenceOwnerMFA, SafetyEvidenceGapBoundaryOwnerAuthorization, SafetyEvidenceGapFollowUpRecordAuthorization},
		{"DETERMINISTIC_RISK", EvidenceDeterministicControl, SafetyEvidenceGapBoundaryRiskControl, SafetyEvidenceGapFollowUpVerifyRisk},
		{"ACCOUNT_RECONCILIATION", EvidenceDatabase, SafetyEvidenceGapBoundaryReconciliation, SafetyEvidenceGapFollowUpVerifyReconciliation},
		{"BROKER_KILL_SWITCH", EvidenceDeterministicControl, SafetyEvidenceGapBoundaryBrokerControl, SafetyEvidenceGapFollowUpVerifyKillSwitch},
		{"IDEMPOTENCY_RESERVATION", EvidenceDatabase, SafetyEvidenceGapBoundaryIdempotency, SafetyEvidenceGapFollowUpVerifyIdempotency},
		{"LIVE_LIFECYCLE_CONTRACT", EvidenceDesignContract, SafetyEvidenceGapBoundaryLifecycleContract, SafetyEvidenceGapFollowUpVerifyLifecycle},
	}
}

func independentlyRecomputeSafetyEvidenceGapRemediationMatrix(
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	inventory SafetyEvidenceGapInventory,
) (SafetyEvidenceGapRemediationMatrix, bool) {
	expectations := independentRemediationExpectations()
	if len(inventory.Categories) != len(expectations) || verificationReview.ExpectedCategoryCount != len(expectations) ||
		verificationReview.CurrentGapCount != inventory.CurrentGapCount {
		return SafetyEvidenceGapRemediationMatrix{}, false
	}
	expected := SafetyEvidenceGapRemediationMatrix{
		MatrixVersion:            SafetyEvidenceGapRemediationMatrixVersion,
		Status:                   SafetyEvidenceGapRemediationMatrixCompiled,
		Failure:                  SafetyEvidenceGapRemediationMatrixFailureNone,
		ReviewDisposition:        verificationReview.Disposition,
		ReviewOwnerAction:        verificationReview.OwnerAction,
		BaselineEvaluatedAt:      verificationReview.BaselineEvaluatedAt,
		CurrentEvaluatedAt:       verificationReview.CurrentEvaluatedAt,
		ExpectedCategoryCount:    verificationReview.ExpectedCategoryCount,
		CurrentGapCount:          verificationReview.CurrentGapCount,
		VerificationReportDigest: verificationReview.VerificationReportDigest,
		VerificationReviewDigest: verificationReview.ArtifactDigest,
		InventoryDigest:          verificationReview.InventoryDigest.Claimed,
		UpstreamReviewDigest:     verificationReview.ReviewDigest.Claimed,
		Requirements:             make([]SafetyEvidenceGapRemediationRequirement, 0, verificationReview.CurrentGapCount),
	}
	for index, expectation := range expectations {
		gap := inventory.Categories[index]
		if gap.Category != expectation.category || gap.Source != expectation.source {
			return SafetyEvidenceGapRemediationMatrix{}, false
		}
		if !gap.CurrentUnavailable {
			continue
		}
		if len(gap.CurrentReasonCodes) == 0 {
			return SafetyEvidenceGapRemediationMatrix{}, false
		}
		expected.Requirements = append(expected.Requirements, SafetyEvidenceGapRemediationRequirement{
			Category: gap.Category, RequiredSource: gap.Source, GapState: gap.State,
			ReasonCodes:         append([]ReasonCode(nil), gap.CurrentReasonCodes...),
			ResponsibleBoundary: expectation.boundary, SafeFollowUp: expectation.followUp,
		})
	}
	if len(expected.Requirements) != expected.CurrentGapCount {
		return SafetyEvidenceGapRemediationMatrix{}, false
	}
	digest, err := independentlyCanonicalSafetyEvidenceGapRemediationMatrixDigest(expected)
	if err != nil {
		return SafetyEvidenceGapRemediationMatrix{}, false
	}
	expected.MatrixDigest = digest
	return expected, true
}

func independentlyValidRemediationReviewDisposition(review SafetyEvidenceGapInventoryVerificationReview) bool {
	if review.Disposition == SafetyEvidenceGapInventoryVerificationReviewNoGaps {
		return review.OwnerAction == SafetyEvidenceGapInventoryVerificationReviewOwnerActionNone &&
			review.CurrentEvidenceAvailable && review.CurrentGapCount == 0
	}
	if review.Disposition == SafetyEvidenceGapInventoryVerificationReviewOwnerReview {
		return review.OwnerAction == SafetyEvidenceGapInventoryVerificationReviewOwnerActionReviewGaps &&
			!review.CurrentEvidenceAvailable && review.CurrentGapCount > 0
	}
	return false
}

func exactMatrixDigestVerification(claimed, expected string) DigestVerification {
	return DigestVerification{Claimed: claimed, Recomputed: expected, Expected: expected, Verified: claimed == expected && digestPattern.MatchString(claimed)}
}

type safetyEvidenceGapRemediationMatrixDigestProjection struct {
	MatrixVersion            string                                                  `json:"matrix_version"`
	Status                   SafetyEvidenceGapRemediationMatrixStatus                `json:"status"`
	Failure                  SafetyEvidenceGapRemediationMatrixFailure               `json:"failure"`
	ExecutionAuthority       bool                                                    `json:"execution_authority"`
	ReviewDisposition        SafetyEvidenceGapInventoryVerificationReviewDisposition `json:"review_disposition"`
	ReviewOwnerAction        SafetyEvidenceGapInventoryVerificationReviewOwnerAction `json:"review_owner_action"`
	BaselineEvaluatedAt      time.Time                                               `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt       time.Time                                               `json:"current_evaluated_at"`
	ExpectedCategoryCount    int                                                     `json:"expected_category_count"`
	CurrentGapCount          int                                                     `json:"current_gap_count"`
	VerificationReportDigest string                                                  `json:"verification_report_sha256"`
	VerificationReviewDigest string                                                  `json:"verification_review_sha256"`
	InventoryDigest          string                                                  `json:"inventory_sha256"`
	UpstreamReviewDigest     string                                                  `json:"upstream_review_sha256"`
	Requirements             []SafetyEvidenceGapRemediationRequirement               `json:"requirements"`
	MatrixDigest             string                                                  `json:"matrix_sha256"`
}

func independentlyCanonicalSafetyEvidenceGapRemediationMatrixDigest(matrix SafetyEvidenceGapRemediationMatrix) (string, error) {
	projection := safetyEvidenceGapRemediationMatrixDigestProjection{
		MatrixVersion: matrix.MatrixVersion, Status: matrix.Status, Failure: matrix.Failure,
		ExecutionAuthority: matrix.ExecutionAuthority, ReviewDisposition: matrix.ReviewDisposition,
		ReviewOwnerAction: matrix.ReviewOwnerAction, BaselineEvaluatedAt: matrix.BaselineEvaluatedAt,
		CurrentEvaluatedAt: matrix.CurrentEvaluatedAt, ExpectedCategoryCount: matrix.ExpectedCategoryCount,
		CurrentGapCount: matrix.CurrentGapCount, VerificationReportDigest: matrix.VerificationReportDigest,
		VerificationReviewDigest: matrix.VerificationReviewDigest, InventoryDigest: matrix.InventoryDigest,
		UpstreamReviewDigest: matrix.UpstreamReviewDigest, Requirements: matrix.Requirements,
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationMatrixDigestDomain, payload), nil
}

func canonicalSafetyEvidenceGapRemediationMatrixVerificationReportDigest(report SafetyEvidenceGapRemediationMatrixVerificationReport) (string, error) {
	report.ReportDigest = ""
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationMatrixVerificationReportDigestDomain, payload), nil
}
