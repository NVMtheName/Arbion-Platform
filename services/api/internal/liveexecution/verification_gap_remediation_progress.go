package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationProgressVersion = "live-safety-evidence-gap-remediation-progress-v1"

const safetyEvidenceGapRemediationProgressDigestDomain = "arbion:live-safety:evidence-gap-remediation-progress:v1"

var ErrSafetyEvidenceGapRemediationProgress = errors.New("live safety evidence gap remediation progress comparison is invalid")

type SafetyEvidenceGapRemediationProgressFailure string

const (
	SafetyEvidenceGapRemediationProgressFailureNone          SafetyEvidenceGapRemediationProgressFailure = "NONE"
	SafetyEvidenceGapRemediationProgressFailureSecret        SafetyEvidenceGapRemediationProgressFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationProgressFailureAuthority     SafetyEvidenceGapRemediationProgressFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationProgressFailureObservedAt    SafetyEvidenceGapRemediationProgressFailure = "OBSERVATION_TIME_INVALID"
	SafetyEvidenceGapRemediationProgressFailurePreviousChain SafetyEvidenceGapRemediationProgressFailure = "PREVIOUS_CHAIN_INVALID"
	SafetyEvidenceGapRemediationProgressFailureCurrentChain  SafetyEvidenceGapRemediationProgressFailure = "CURRENT_CHAIN_INVALID"
	SafetyEvidenceGapRemediationProgressFailureComparable    SafetyEvidenceGapRemediationProgressFailure = "CHAINS_NOT_COMPARABLE"
	SafetyEvidenceGapRemediationProgressFailureTimeOrder     SafetyEvidenceGapRemediationProgressFailure = "REPORT_TIME_ORDER_INVALID"
	SafetyEvidenceGapRemediationProgressFailureDigest        SafetyEvidenceGapRemediationProgressFailure = "REPORT_DIGEST_INVALID"
	SafetyEvidenceGapRemediationProgressFailureRequirement   SafetyEvidenceGapRemediationProgressFailure = "REQUIREMENT_EVIDENCE_INVALID"
	SafetyEvidenceGapRemediationProgressFailureCount         SafetyEvidenceGapRemediationProgressFailure = "CHANGE_COUNT_MISMATCH"
	SafetyEvidenceGapRemediationProgressFailureCompilation   SafetyEvidenceGapRemediationProgressFailure = "COMPARISON_COMPILATION_FAILED"
)

type SafetyEvidenceGapRemediationChange string

const (
	SafetyEvidenceGapRemediationUnchanged     SafetyEvidenceGapRemediationChange = "UNCHANGED"
	SafetyEvidenceGapRemediationNewlyRequired SafetyEvidenceGapRemediationChange = "NEWLY_REQUIRED"
	SafetyEvidenceGapRemediationResolved      SafetyEvidenceGapRemediationChange = "RESOLVED"
)

// SafetyEvidenceGapRemediationOwnerReviewChain binds one owner-review report
// to every input needed to verify that report independently. It is evidence
// only and contains no execution method or authority.
type SafetyEvidenceGapRemediationOwnerReviewChain struct {
	BaselineReport     SafetyCaseVerificationReport
	CurrentReport      SafetyCaseVerificationReport
	ComparisonEnvelope SafetyCaseVerificationComparisonEnvelope
	ComparisonReport   SafetyCaseVerificationComparisonEnvelopeReport
	ComparisonReview   SafetyCaseVerificationComparisonReviewArtifact
	GapInventory       SafetyEvidenceGapInventory
	GapInventoryReport SafetyEvidenceGapInventoryVerificationReport
	GapInventoryReview SafetyEvidenceGapInventoryVerificationReview
	RemediationMatrix  SafetyEvidenceGapRemediationMatrix
	MatrixReport       SafetyEvidenceGapRemediationMatrixVerificationReport
	OwnerReview        SafetyEvidenceGapRemediationMatrixVerificationReview
	OwnerReviewReport  SafetyEvidenceGapRemediationMatrixVerificationReviewReport
}

// SafetyEvidenceGapRemediationProgressRequirement preserves the exact prior
// and current evidence contract for one canonical category. Change describes
// only whether evidence is required; it does not claim cause or completion.
type SafetyEvidenceGapRemediationProgressRequirement struct {
	Category                   string                                   `json:"category"`
	RequiredSource             EvidenceSource                           `json:"required_source"`
	ResponsibleBoundary        SafetyEvidenceGapResponsibleBoundary     `json:"responsible_boundary"`
	SafeFollowUp               SafetyEvidenceGapFollowUp                `json:"safe_follow_up"`
	Change                     SafetyEvidenceGapRemediationChange       `json:"change"`
	PreviousRequired           bool                                     `json:"previous_required"`
	CurrentRequired            bool                                     `json:"current_required"`
	RequirementEvidenceChanged bool                                     `json:"requirement_evidence_changed"`
	PreviousRequirement        *SafetyEvidenceGapRemediationRequirement `json:"previous_requirement"`
	CurrentRequirement         *SafetyEvidenceGapRemediationRequirement `json:"current_requirement"`
}

// SafetyEvidenceGapRemediationProgressComparison compares two complete,
// independently verified owner-review chains. VERIFIED means structural
// integrity only and never means remediation completion, live readiness,
// approval, or execution authority.
type SafetyEvidenceGapRemediationProgressComparison struct {
	ComparisonVersion      string                                            `json:"comparison_version"`
	Status                 VerificationStatus                                `json:"status"`
	Failure                SafetyEvidenceGapRemediationProgressFailure       `json:"failure"`
	ExecutionAuthority     bool                                              `json:"execution_authority"`
	ObservedAt             time.Time                                         `json:"observed_at"`
	PreviousBaselineAt     time.Time                                         `json:"previous_baseline_evaluated_at"`
	PreviousCurrentAt      time.Time                                         `json:"previous_current_evaluated_at"`
	CurrentBaselineAt      time.Time                                         `json:"current_baseline_evaluated_at"`
	CurrentCurrentAt       time.Time                                         `json:"current_current_evaluated_at"`
	PreviousReportDigest   string                                            `json:"previous_report_sha256"`
	CurrentReportDigest    string                                            `json:"current_report_sha256"`
	PreviousArtifactDigest string                                            `json:"previous_artifact_sha256"`
	CurrentArtifactDigest  string                                            `json:"current_artifact_sha256"`
	PreviousMatrixDigest   string                                            `json:"previous_matrix_sha256"`
	CurrentMatrixDigest    string                                            `json:"current_matrix_sha256"`
	ExpectedCategoryCount  int                                               `json:"expected_category_count"`
	UnchangedCount         int                                               `json:"unchanged_count"`
	NewlyRequiredCount     int                                               `json:"newly_required_count"`
	ResolvedCount          int                                               `json:"resolved_count"`
	Requirements           []SafetyEvidenceGapRemediationProgressRequirement `json:"requirements"`
	ComparisonDigest       string                                            `json:"comparison_sha256"`
}

// BuildSafetyEvidenceGapRemediationProgressComparison verifies both complete
// chains before comparing them. It never trusts a self-asserted report.
func BuildSafetyEvidenceGapRemediationProgressComparison(
	previous, current SafetyEvidenceGapRemediationOwnerReviewChain,
	observedAt time.Time,
) SafetyEvidenceGapRemediationProgressComparison {
	result := SafetyEvidenceGapRemediationProgressComparison{
		ComparisonVersion: SafetyEvidenceGapRemediationProgressVersion,
		Status:            VerificationRejected,
		Failure:           SafetyEvidenceGapRemediationProgressFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationProgressFailure) SafetyEvidenceGapRemediationProgressComparison {
		result.Status = VerificationRejected
		result.Failure = failure
		result.ComparisonDigest = ""
		return result
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(previous)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) {
		return reject(SafetyEvidenceGapRemediationProgressFailureSecret)
	}
	if remediationProgressChainHasAuthority(previous) || remediationProgressChainHasAuthority(current) {
		return reject(SafetyEvidenceGapRemediationProgressFailureAuthority)
	}
	if observedAt.IsZero() || observedAt.Location() != time.UTC {
		return reject(SafetyEvidenceGapRemediationProgressFailureObservedAt)
	}
	if verifyRemediationProgressChain(previous) != nil {
		return reject(SafetyEvidenceGapRemediationProgressFailurePreviousChain)
	}
	if verifyRemediationProgressChain(current) != nil {
		return reject(SafetyEvidenceGapRemediationProgressFailureCurrentChain)
	}

	previousReport := previous.OwnerReviewReport
	currentReport := current.OwnerReviewReport
	if !reflect.DeepEqual(previous.CurrentReport.Identity, current.CurrentReport.Identity) ||
		previousReport.ExpectedCategoryCount != currentReport.ExpectedCategoryCount ||
		previousReport.ExpectedCategoryCount != len(independentRemediationExpectations()) {
		return reject(SafetyEvidenceGapRemediationProgressFailureComparable)
	}
	if previousReport.BaselineEvaluatedAt.IsZero() || previousReport.CurrentEvaluatedAt.IsZero() ||
		currentReport.BaselineEvaluatedAt.IsZero() || currentReport.CurrentEvaluatedAt.IsZero() ||
		previousReport.BaselineEvaluatedAt.Location() != time.UTC || previousReport.CurrentEvaluatedAt.Location() != time.UTC ||
		currentReport.BaselineEvaluatedAt.Location() != time.UTC || currentReport.CurrentEvaluatedAt.Location() != time.UTC ||
		!previousReport.CurrentEvaluatedAt.Before(currentReport.CurrentEvaluatedAt) ||
		previousReport.BaselineEvaluatedAt.After(observedAt) || previousReport.CurrentEvaluatedAt.After(observedAt) ||
		currentReport.BaselineEvaluatedAt.After(observedAt) || currentReport.CurrentEvaluatedAt.After(observedAt) {
		return reject(SafetyEvidenceGapRemediationProgressFailureTimeOrder)
	}
	for _, digest := range []string{
		previousReport.ReportDigest, currentReport.ReportDigest,
		previous.OwnerReview.ArtifactDigest, current.OwnerReview.ArtifactDigest,
		previous.RemediationMatrix.MatrixDigest, current.RemediationMatrix.MatrixDigest,
	} {
		if !digestPattern.MatchString(digest) {
			return reject(SafetyEvidenceGapRemediationProgressFailureDigest)
		}
	}

	previousRequirements, ok := exactRemediationProgressRequirements(previousReport)
	if !ok {
		return reject(SafetyEvidenceGapRemediationProgressFailureRequirement)
	}
	currentRequirements, ok := exactRemediationProgressRequirements(currentReport)
	if !ok {
		return reject(SafetyEvidenceGapRemediationProgressFailureRequirement)
	}

	result.ObservedAt = observedAt
	result.PreviousBaselineAt = previousReport.BaselineEvaluatedAt
	result.PreviousCurrentAt = previousReport.CurrentEvaluatedAt
	result.CurrentBaselineAt = currentReport.BaselineEvaluatedAt
	result.CurrentCurrentAt = currentReport.CurrentEvaluatedAt
	result.PreviousReportDigest = previousReport.ReportDigest
	result.CurrentReportDigest = currentReport.ReportDigest
	result.PreviousArtifactDigest = previous.OwnerReview.ArtifactDigest
	result.CurrentArtifactDigest = current.OwnerReview.ArtifactDigest
	result.PreviousMatrixDigest = previous.RemediationMatrix.MatrixDigest
	result.CurrentMatrixDigest = current.RemediationMatrix.MatrixDigest
	result.ExpectedCategoryCount = len(independentRemediationExpectations())
	result.Requirements = make([]SafetyEvidenceGapRemediationProgressRequirement, 0, result.ExpectedCategoryCount)
	for _, expectation := range independentRemediationExpectations() {
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
			PreviousRequirement:        previousRequirement, CurrentRequirement: currentRequirement,
		})
	}
	if len(result.Requirements) != result.ExpectedCategoryCount ||
		result.UnchangedCount+result.NewlyRequiredCount+result.ResolvedCount != result.ExpectedCategoryCount {
		return reject(SafetyEvidenceGapRemediationProgressFailureCount)
	}
	result.Status = VerificationVerified
	result.Failure = SafetyEvidenceGapRemediationProgressFailureNone
	digest, err := canonicalSafetyEvidenceGapRemediationProgressDigest(result)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationProgressFailureCompilation)
	}
	result.ComparisonDigest = digest
	return result
}

func VerifySafetyEvidenceGapRemediationProgressComparison(
	previous, current SafetyEvidenceGapRemediationOwnerReviewChain,
	observedAt time.Time,
	result SafetyEvidenceGapRemediationProgressComparison,
) error {
	if result.ComparisonVersion != SafetyEvidenceGapRemediationProgressVersion ||
		result.Status != VerificationVerified || result.Failure != SafetyEvidenceGapRemediationProgressFailureNone ||
		result.ExecutionAuthority || !digestPattern.MatchString(result.ComparisonDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(result)) {
		return ErrSafetyEvidenceGapRemediationProgress
	}
	expected := BuildSafetyEvidenceGapRemediationProgressComparison(previous, current, observedAt)
	if !reflect.DeepEqual(result, expected) {
		return ErrSafetyEvidenceGapRemediationProgress
	}
	return nil
}

func verifyRemediationProgressChain(chain SafetyEvidenceGapRemediationOwnerReviewChain) error {
	return VerifySafetyEvidenceGapRemediationMatrixVerificationReviewReport(
		chain.BaselineReport, chain.CurrentReport, chain.ComparisonEnvelope, chain.ComparisonReport,
		chain.ComparisonReview, chain.GapInventory, chain.GapInventoryReport, chain.GapInventoryReview,
		chain.RemediationMatrix, chain.MatrixReport, chain.OwnerReview, chain.OwnerReviewReport,
	)
}

func remediationProgressChainHasAuthority(chain SafetyEvidenceGapRemediationOwnerReviewChain) bool {
	return chain.BaselineReport.ExecutionAuthority || chain.CurrentReport.ExecutionAuthority ||
		chain.ComparisonEnvelope.Comparison.ExecutionAuthority || chain.ComparisonReport.ExecutionAuthority ||
		chain.ComparisonReview.ExecutionAuthority || chain.GapInventory.ExecutionAuthority ||
		chain.GapInventoryReport.ExecutionAuthority || chain.GapInventoryReview.ExecutionAuthority ||
		chain.RemediationMatrix.ExecutionAuthority || chain.MatrixReport.ExecutionAuthority ||
		chain.OwnerReview.ExecutionAuthority || chain.OwnerReviewReport.ExecutionAuthority
}

func exactRemediationProgressRequirements(
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

func canonicalSafetyEvidenceGapRemediationProgressDigest(
	result SafetyEvidenceGapRemediationProgressComparison,
) (string, error) {
	result.ComparisonDigest = ""
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationProgressDigestDomain, payload), nil
}
