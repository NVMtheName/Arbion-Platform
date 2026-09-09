package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationProgressVerificationReportVersion = "live-safety-evidence-gap-remediation-progress-verification-v1"

const safetyEvidenceGapRemediationProgressVerificationReportDigestDomain = "arbion:live-safety:evidence-gap-remediation-progress-verification:v1"

var ErrSafetyEvidenceGapRemediationProgressVerificationReport = errors.New("live safety evidence gap remediation progress verification report is invalid")

type SafetyEvidenceGapRemediationProgressVerificationFailure string

const (
	SafetyEvidenceGapRemediationProgressVerificationFailureNone              SafetyEvidenceGapRemediationProgressVerificationFailure = "NONE"
	SafetyEvidenceGapRemediationProgressVerificationFailureSecret            SafetyEvidenceGapRemediationProgressVerificationFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationProgressVerificationFailureAuthority         SafetyEvidenceGapRemediationProgressVerificationFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationProgressVerificationFailureObservedAt        SafetyEvidenceGapRemediationProgressVerificationFailure = "OBSERVATION_TIME_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailurePreviousChain     SafetyEvidenceGapRemediationProgressVerificationFailure = "PREVIOUS_CHAIN_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailureCurrentChain      SafetyEvidenceGapRemediationProgressVerificationFailure = "CURRENT_CHAIN_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailureComparisonVersion SafetyEvidenceGapRemediationProgressVerificationFailure = "COMPARISON_VERSION_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailureComparisonStatus  SafetyEvidenceGapRemediationProgressVerificationFailure = "COMPARISON_STATUS_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailureComparable        SafetyEvidenceGapRemediationProgressVerificationFailure = "CHAINS_NOT_COMPARABLE"
	SafetyEvidenceGapRemediationProgressVerificationFailureTimeOrder         SafetyEvidenceGapRemediationProgressVerificationFailure = "REPORT_TIME_ORDER_INVALID"
	SafetyEvidenceGapRemediationProgressVerificationFailureDigestEvidence    SafetyEvidenceGapRemediationProgressVerificationFailure = "UPSTREAM_DIGEST_EVIDENCE_MISMATCH"
	SafetyEvidenceGapRemediationProgressVerificationFailureRequirement       SafetyEvidenceGapRemediationProgressVerificationFailure = "REQUIREMENT_EVIDENCE_MISMATCH"
	SafetyEvidenceGapRemediationProgressVerificationFailureCount             SafetyEvidenceGapRemediationProgressVerificationFailure = "CHANGE_COUNT_MISMATCH"
	SafetyEvidenceGapRemediationProgressVerificationFailureDigestMalformed   SafetyEvidenceGapRemediationProgressVerificationFailure = "COMPARISON_DIGEST_MALFORMED"
	SafetyEvidenceGapRemediationProgressVerificationFailureDigestMismatch    SafetyEvidenceGapRemediationProgressVerificationFailure = "COMPARISON_DIGEST_MISMATCH"
	SafetyEvidenceGapRemediationProgressVerificationFailureContent           SafetyEvidenceGapRemediationProgressVerificationFailure = "COMPARISON_CONTENT_MISMATCH"
	SafetyEvidenceGapRemediationProgressVerificationFailureCompilation       SafetyEvidenceGapRemediationProgressVerificationFailure = "REPORT_COMPILATION_FAILED"
)

type SafetyEvidenceGapRemediationProgressRequirementVerification struct {
	Claimed    SafetyEvidenceGapRemediationProgressRequirement `json:"claimed"`
	Recomputed SafetyEvidenceGapRemediationProgressRequirement `json:"recomputed"`
	Verified   bool                                            `json:"verified"`
}

// SafetyEvidenceGapRemediationProgressVerificationReport independently
// verifies a claimed progress comparison against both complete owner-review
// chains. VERIFIED means structural integrity only and never means that a gap
// is remediated, live trading is ready, or execution is authorized.
type SafetyEvidenceGapRemediationProgressVerificationReport struct {
	ReportVersion                string                                                        `json:"report_version"`
	Status                       VerificationStatus                                            `json:"status"`
	Failure                      SafetyEvidenceGapRemediationProgressVerificationFailure       `json:"failure"`
	ExecutionAuthority           bool                                                          `json:"execution_authority"`
	ComparisonVersion            VersionVerification                                           `json:"comparison_version"`
	ObservedAt                   time.Time                                                     `json:"observed_at"`
	PreviousBaselineAt           time.Time                                                     `json:"previous_baseline_evaluated_at"`
	PreviousCurrentAt            time.Time                                                     `json:"previous_current_evaluated_at"`
	CurrentBaselineAt            time.Time                                                     `json:"current_baseline_evaluated_at"`
	CurrentCurrentAt             time.Time                                                     `json:"current_current_evaluated_at"`
	PreviousReportDigest         DigestVerification                                            `json:"previous_report_digest"`
	CurrentReportDigest          DigestVerification                                            `json:"current_report_digest"`
	PreviousArtifactDigest       DigestVerification                                            `json:"previous_artifact_digest"`
	CurrentArtifactDigest        DigestVerification                                            `json:"current_artifact_digest"`
	PreviousMatrixDigest         DigestVerification                                            `json:"previous_matrix_digest"`
	CurrentMatrixDigest          DigestVerification                                            `json:"current_matrix_digest"`
	ExpectedCategoryCount        int                                                           `json:"expected_category_count"`
	ClaimedUnchangedCount        int                                                           `json:"claimed_unchanged_count"`
	RecomputedUnchangedCount     int                                                           `json:"recomputed_unchanged_count"`
	ClaimedNewlyRequiredCount    int                                                           `json:"claimed_newly_required_count"`
	RecomputedNewlyRequiredCount int                                                           `json:"recomputed_newly_required_count"`
	ClaimedResolvedCount         int                                                           `json:"claimed_resolved_count"`
	RecomputedResolvedCount      int                                                           `json:"recomputed_resolved_count"`
	Requirements                 []SafetyEvidenceGapRemediationProgressRequirementVerification `json:"requirements"`
	ComparisonDigest             DigestVerification                                            `json:"comparison_digest"`
	ReportDigest                 string                                                        `json:"report_sha256"`
}

// BuildSafetyEvidenceGapRemediationProgressVerificationReport reconstructs
// the progress comparison without calling its builder, verifier, or digest
// helper. Both complete owner-review chains are verified first.
func BuildSafetyEvidenceGapRemediationProgressVerificationReport(
	previous, current SafetyEvidenceGapRemediationOwnerReviewChain,
	observedAt time.Time,
	comparison SafetyEvidenceGapRemediationProgressComparison,
) SafetyEvidenceGapRemediationProgressVerificationReport {
	result := SafetyEvidenceGapRemediationProgressVerificationReport{
		ReportVersion: SafetyEvidenceGapRemediationProgressVerificationReportVersion,
		Status:        VerificationRejected,
		Failure:       SafetyEvidenceGapRemediationProgressVerificationFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationProgressVerificationFailure) SafetyEvidenceGapRemediationProgressVerificationReport {
		result.Status = VerificationRejected
		result.Failure = failure
		result.ReportDigest = ""
		return result
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(previous)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(comparison)) {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureSecret)
	}
	if remediationProgressChainHasAuthority(previous) || remediationProgressChainHasAuthority(current) || comparison.ExecutionAuthority {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureAuthority)
	}
	if observedAt.IsZero() || observedAt.Location() != time.UTC {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureObservedAt)
	}
	if verifyRemediationProgressChain(previous) != nil {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailurePreviousChain)
	}
	if verifyRemediationProgressChain(current) != nil {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureCurrentChain)
	}

	result.ComparisonVersion = VersionVerification{
		Expected: SafetyEvidenceGapRemediationProgressVersion,
		Claimed:  comparison.ComparisonVersion,
		Verified: comparison.ComparisonVersion == SafetyEvidenceGapRemediationProgressVersion,
	}
	if !result.ComparisonVersion.Verified {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureComparisonVersion)
	}
	if comparison.Status != VerificationVerified || comparison.Failure != SafetyEvidenceGapRemediationProgressFailureNone {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureComparisonStatus)
	}

	expected, failure := independentlyRecomputeRemediationProgress(previous, current, observedAt)
	if failure != SafetyEvidenceGapRemediationProgressVerificationFailureNone {
		return reject(failure)
	}
	result.ObservedAt = expected.ObservedAt
	result.PreviousBaselineAt = expected.PreviousBaselineAt
	result.PreviousCurrentAt = expected.PreviousCurrentAt
	result.CurrentBaselineAt = expected.CurrentBaselineAt
	result.CurrentCurrentAt = expected.CurrentCurrentAt
	result.PreviousReportDigest = exactRemediationProgressDigestVerification(comparison.PreviousReportDigest, expected.PreviousReportDigest)
	result.CurrentReportDigest = exactRemediationProgressDigestVerification(comparison.CurrentReportDigest, expected.CurrentReportDigest)
	result.PreviousArtifactDigest = exactRemediationProgressDigestVerification(comparison.PreviousArtifactDigest, expected.PreviousArtifactDigest)
	result.CurrentArtifactDigest = exactRemediationProgressDigestVerification(comparison.CurrentArtifactDigest, expected.CurrentArtifactDigest)
	result.PreviousMatrixDigest = exactRemediationProgressDigestVerification(comparison.PreviousMatrixDigest, expected.PreviousMatrixDigest)
	result.CurrentMatrixDigest = exactRemediationProgressDigestVerification(comparison.CurrentMatrixDigest, expected.CurrentMatrixDigest)
	for _, digest := range []DigestVerification{
		result.PreviousReportDigest, result.CurrentReportDigest,
		result.PreviousArtifactDigest, result.CurrentArtifactDigest,
		result.PreviousMatrixDigest, result.CurrentMatrixDigest,
	} {
		if !digest.Verified {
			return reject(SafetyEvidenceGapRemediationProgressVerificationFailureDigestEvidence)
		}
	}

	result.ExpectedCategoryCount = expected.ExpectedCategoryCount
	result.ClaimedUnchangedCount = comparison.UnchangedCount
	result.RecomputedUnchangedCount = expected.UnchangedCount
	result.ClaimedNewlyRequiredCount = comparison.NewlyRequiredCount
	result.RecomputedNewlyRequiredCount = expected.NewlyRequiredCount
	result.ClaimedResolvedCount = comparison.ResolvedCount
	result.RecomputedResolvedCount = expected.ResolvedCount
	if comparison.ExpectedCategoryCount != expected.ExpectedCategoryCount ||
		result.ClaimedUnchangedCount != result.RecomputedUnchangedCount ||
		result.ClaimedNewlyRequiredCount != result.RecomputedNewlyRequiredCount ||
		result.ClaimedResolvedCount != result.RecomputedResolvedCount ||
		len(comparison.Requirements) != expected.ExpectedCategoryCount ||
		len(expected.Requirements) != expected.ExpectedCategoryCount {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureCount)
	}

	result.Requirements = make([]SafetyEvidenceGapRemediationProgressRequirementVerification, 0, expected.ExpectedCategoryCount)
	for index, recomputed := range expected.Requirements {
		claimed := comparison.Requirements[index]
		verified := reflect.DeepEqual(claimed, recomputed)
		result.Requirements = append(result.Requirements, SafetyEvidenceGapRemediationProgressRequirementVerification{
			Claimed: claimed, Recomputed: recomputed, Verified: verified,
		})
		if !verified {
			return reject(SafetyEvidenceGapRemediationProgressVerificationFailureRequirement)
		}
	}

	recomputedClaimedDigest, err := independentlyCanonicalRemediationProgressDigest(comparison)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureCompilation)
	}
	expectedDigest, err := independentlyCanonicalRemediationProgressDigest(expected)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureCompilation)
	}
	expected.ComparisonDigest = expectedDigest
	result.ComparisonDigest = DigestVerification{
		Claimed: comparison.ComparisonDigest, Recomputed: recomputedClaimedDigest, Expected: expectedDigest,
		Verified: comparison.ComparisonDigest == recomputedClaimedDigest && comparison.ComparisonDigest == expectedDigest,
	}
	if !digestPattern.MatchString(comparison.ComparisonDigest) {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureDigestMalformed)
	}
	if !result.ComparisonDigest.Verified {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureDigestMismatch)
	}
	if !reflect.DeepEqual(comparison, expected) {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureContent)
	}

	result.Status = VerificationVerified
	result.Failure = SafetyEvidenceGapRemediationProgressVerificationFailureNone
	result.ReportDigest, err = canonicalSafetyEvidenceGapRemediationProgressVerificationReportDigest(result)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationProgressVerificationFailureCompilation)
	}
	return result
}

func VerifySafetyEvidenceGapRemediationProgressVerificationReport(
	previous, current SafetyEvidenceGapRemediationOwnerReviewChain,
	observedAt time.Time,
	comparison SafetyEvidenceGapRemediationProgressComparison,
	report SafetyEvidenceGapRemediationProgressVerificationReport,
) error {
	if report.ReportVersion != SafetyEvidenceGapRemediationProgressVerificationReportVersion ||
		report.Status != VerificationVerified ||
		report.Failure != SafetyEvidenceGapRemediationProgressVerificationFailureNone ||
		report.ExecutionAuthority || !digestPattern.MatchString(report.ReportDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) {
		return ErrSafetyEvidenceGapRemediationProgressVerificationReport
	}
	expected := BuildSafetyEvidenceGapRemediationProgressVerificationReport(previous, current, observedAt, comparison)
	if !reflect.DeepEqual(report, expected) {
		return ErrSafetyEvidenceGapRemediationProgressVerificationReport
	}
	return nil
}

func independentlyRecomputeRemediationProgress(
	previous, current SafetyEvidenceGapRemediationOwnerReviewChain,
	observedAt time.Time,
) (SafetyEvidenceGapRemediationProgressComparison, SafetyEvidenceGapRemediationProgressVerificationFailure) {
	previousReport := previous.OwnerReviewReport
	currentReport := current.OwnerReviewReport
	expectations := independentRemediationExpectations()
	if !reflect.DeepEqual(previous.CurrentReport.Identity, current.CurrentReport.Identity) ||
		previousReport.ExpectedCategoryCount != currentReport.ExpectedCategoryCount ||
		previousReport.ExpectedCategoryCount != len(expectations) {
		return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureComparable
	}
	if previousReport.BaselineEvaluatedAt.IsZero() || previousReport.CurrentEvaluatedAt.IsZero() ||
		currentReport.BaselineEvaluatedAt.IsZero() || currentReport.CurrentEvaluatedAt.IsZero() ||
		previousReport.BaselineEvaluatedAt.Location() != time.UTC || previousReport.CurrentEvaluatedAt.Location() != time.UTC ||
		currentReport.BaselineEvaluatedAt.Location() != time.UTC || currentReport.CurrentEvaluatedAt.Location() != time.UTC ||
		!previousReport.CurrentEvaluatedAt.Before(currentReport.CurrentEvaluatedAt) ||
		previousReport.BaselineEvaluatedAt.After(observedAt) || previousReport.CurrentEvaluatedAt.After(observedAt) ||
		currentReport.BaselineEvaluatedAt.After(observedAt) || currentReport.CurrentEvaluatedAt.After(observedAt) {
		return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureTimeOrder
	}
	for _, digest := range []string{
		previousReport.ReportDigest, currentReport.ReportDigest,
		previous.OwnerReview.ArtifactDigest, current.OwnerReview.ArtifactDigest,
		previous.RemediationMatrix.MatrixDigest, current.RemediationMatrix.MatrixDigest,
	} {
		if !digestPattern.MatchString(digest) {
			return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureDigestEvidence
		}
	}
	previousRequirements, ok := independentlyExactRemediationProgressRequirements(previousReport)
	if !ok {
		return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureRequirement
	}
	currentRequirements, ok := independentlyExactRemediationProgressRequirements(currentReport)
	if !ok {
		return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureRequirement
	}

	result := SafetyEvidenceGapRemediationProgressComparison{
		ComparisonVersion:      SafetyEvidenceGapRemediationProgressVersion,
		Status:                 VerificationVerified,
		Failure:                SafetyEvidenceGapRemediationProgressFailureNone,
		ObservedAt:             observedAt,
		PreviousBaselineAt:     previousReport.BaselineEvaluatedAt,
		PreviousCurrentAt:      previousReport.CurrentEvaluatedAt,
		CurrentBaselineAt:      currentReport.BaselineEvaluatedAt,
		CurrentCurrentAt:       currentReport.CurrentEvaluatedAt,
		PreviousReportDigest:   previousReport.ReportDigest,
		CurrentReportDigest:    currentReport.ReportDigest,
		PreviousArtifactDigest: previous.OwnerReview.ArtifactDigest,
		CurrentArtifactDigest:  current.OwnerReview.ArtifactDigest,
		PreviousMatrixDigest:   previous.RemediationMatrix.MatrixDigest,
		CurrentMatrixDigest:    current.RemediationMatrix.MatrixDigest,
		ExpectedCategoryCount:  len(expectations),
		Requirements:           make([]SafetyEvidenceGapRemediationProgressRequirement, 0, len(expectations)),
	}
	for _, expectation := range expectations {
		previousRequirement, previousRequired := previousRequirements[expectation.category]
		currentRequirement, currentRequired := currentRequirements[expectation.category]
		change := SafetyEvidenceGapRemediationUnchanged
		switch {
		case !previousRequired && currentRequired:
			change = SafetyEvidenceGapRemediationNewlyRequired
			result.NewlyRequiredCount++
		case previousRequired && !currentRequired:
			change = SafetyEvidenceGapRemediationResolved
			result.ResolvedCount++
		default:
			result.UnchangedCount++
		}
		result.Requirements = append(result.Requirements, SafetyEvidenceGapRemediationProgressRequirement{
			Category: expectation.category, RequiredSource: expectation.source,
			ResponsibleBoundary: expectation.boundary, SafeFollowUp: expectation.followUp,
			Change: change, PreviousRequired: previousRequired, CurrentRequired: currentRequired,
			RequirementEvidenceChanged: !reflect.DeepEqual(previousRequirement, currentRequirement),
			PreviousRequirement:        cloneOptionalRemediationRequirement(previousRequirement),
			CurrentRequirement:         cloneOptionalRemediationRequirement(currentRequirement),
		})
	}
	if len(result.Requirements) != result.ExpectedCategoryCount ||
		result.UnchangedCount+result.NewlyRequiredCount+result.ResolvedCount != result.ExpectedCategoryCount {
		return SafetyEvidenceGapRemediationProgressComparison{}, SafetyEvidenceGapRemediationProgressVerificationFailureCount
	}
	return result, SafetyEvidenceGapRemediationProgressVerificationFailureNone
}

func independentlyExactRemediationProgressRequirements(
	report SafetyEvidenceGapRemediationMatrixVerificationReviewReport,
) (map[string]*SafetyEvidenceGapRemediationRequirement, bool) {
	result := make(map[string]*SafetyEvidenceGapRemediationRequirement, len(report.Requirements))
	expectations := independentRemediationExpectations()
	next := 0
	for _, expectation := range expectations {
		if next >= len(report.Requirements) || report.Requirements[next].Recomputed.Category != expectation.category {
			continue
		}
		verification := report.Requirements[next]
		if !verification.Verified || !reflect.DeepEqual(verification.Claimed, verification.Recomputed) ||
			verification.Recomputed.RequiredSource != expectation.source ||
			verification.Recomputed.ResponsibleBoundary != expectation.boundary ||
			verification.Recomputed.SafeFollowUp != expectation.followUp ||
			len(verification.Recomputed.ReasonCodes) == 0 {
			return nil, false
		}
		requirement := cloneRemediationRequirement(verification.Recomputed)
		result[expectation.category] = &requirement
		next++
	}
	if next != len(report.Requirements) || len(result) != report.RecomputedRequirementCount {
		return nil, false
	}
	return result, true
}

func cloneOptionalRemediationRequirement(value *SafetyEvidenceGapRemediationRequirement) *SafetyEvidenceGapRemediationRequirement {
	if value == nil {
		return nil
	}
	cloned := cloneRemediationRequirement(*value)
	return &cloned
}

func exactRemediationProgressDigestVerification(claimed, expected string) DigestVerification {
	return DigestVerification{
		Claimed: claimed, Recomputed: expected, Expected: expected,
		Verified: digestPattern.MatchString(claimed) && claimed == expected,
	}
}

func independentlyCanonicalRemediationProgressDigest(
	comparison SafetyEvidenceGapRemediationProgressComparison,
) (string, error) {
	comparison.ComparisonDigest = ""
	payload, err := json.Marshal(comparison)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256("arbion:live-safety:evidence-gap-remediation-progress:v1", payload), nil
}

func canonicalSafetyEvidenceGapRemediationProgressVerificationReportDigest(
	report SafetyEvidenceGapRemediationProgressVerificationReport,
) (string, error) {
	report.ReportDigest = ""
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationProgressVerificationReportDigestDomain, payload), nil
}
