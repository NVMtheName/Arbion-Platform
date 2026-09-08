package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyEvidenceGapRemediationMatrixVersion = "live-safety-evidence-gap-remediation-matrix-v1"

const safetyEvidenceGapRemediationMatrixDigestDomain = "arbion:live-safety:evidence-gap-remediation-matrix:v1"

var ErrSafetyEvidenceGapRemediationMatrix = errors.New("live safety evidence gap remediation matrix is invalid")

type SafetyEvidenceGapRemediationMatrixStatus string

const (
	SafetyEvidenceGapRemediationMatrixCompiled SafetyEvidenceGapRemediationMatrixStatus = "COMPILED"
	SafetyEvidenceGapRemediationMatrixRejected SafetyEvidenceGapRemediationMatrixStatus = "REJECTED"
)

type SafetyEvidenceGapRemediationMatrixFailure string

const (
	SafetyEvidenceGapRemediationMatrixFailureNone        SafetyEvidenceGapRemediationMatrixFailure = "NONE"
	SafetyEvidenceGapRemediationMatrixFailureSecretInput SafetyEvidenceGapRemediationMatrixFailure = "SECRET_LIKE_INPUT"
	SafetyEvidenceGapRemediationMatrixFailureReview      SafetyEvidenceGapRemediationMatrixFailure = "VERIFICATION_REVIEW_INVALID"
	SafetyEvidenceGapRemediationMatrixFailureAuthority   SafetyEvidenceGapRemediationMatrixFailure = "EXECUTION_AUTHORITY_PRESENT"
	SafetyEvidenceGapRemediationMatrixFailureDisposition SafetyEvidenceGapRemediationMatrixFailure = "REVIEW_DISPOSITION_INVALID"
	SafetyEvidenceGapRemediationMatrixFailureCount       SafetyEvidenceGapRemediationMatrixFailure = "GAP_COUNT_INVALID"
	SafetyEvidenceGapRemediationMatrixFailureCategory    SafetyEvidenceGapRemediationMatrixFailure = "GAP_CATEGORY_INVALID"
	SafetyEvidenceGapRemediationMatrixFailureCompilation SafetyEvidenceGapRemediationMatrixFailure = "MATRIX_COMPILATION_FAILED"
)

type SafetyEvidenceGapResponsibleBoundary string

const (
	SafetyEvidenceGapBoundaryFinancialProvider  SafetyEvidenceGapResponsibleBoundary = "FINANCIAL_PROVIDER_CONFIGURATION"
	SafetyEvidenceGapBoundaryOwnerAuthorization SafetyEvidenceGapResponsibleBoundary = "OWNER_AUTHORIZATION"
	SafetyEvidenceGapBoundaryRiskControl        SafetyEvidenceGapResponsibleBoundary = "RISK_CONTROL_PLANE"
	SafetyEvidenceGapBoundaryReconciliation     SafetyEvidenceGapResponsibleBoundary = "ACCOUNT_RECONCILIATION"
	SafetyEvidenceGapBoundaryBrokerControl      SafetyEvidenceGapResponsibleBoundary = "BROKER_CONTROL"
	SafetyEvidenceGapBoundaryIdempotency        SafetyEvidenceGapResponsibleBoundary = "EXECUTION_IDEMPOTENCY"
	SafetyEvidenceGapBoundaryLifecycleContract  SafetyEvidenceGapResponsibleBoundary = "LIVE_LIFECYCLE_DESIGN"
)

type SafetyEvidenceGapFollowUp string

const (
	SafetyEvidenceGapFollowUpVerifyProvider       SafetyEvidenceGapFollowUp = "VERIFY_PROVIDER_CAPABILITY_EVIDENCE"
	SafetyEvidenceGapFollowUpRecordAuthorization  SafetyEvidenceGapFollowUp = "RECORD_OWNER_AUTHORIZATION_EVIDENCE"
	SafetyEvidenceGapFollowUpVerifyRisk           SafetyEvidenceGapFollowUp = "VERIFY_DETERMINISTIC_RISK_EVIDENCE"
	SafetyEvidenceGapFollowUpVerifyReconciliation SafetyEvidenceGapFollowUp = "VERIFY_ACCOUNT_RECONCILIATION_EVIDENCE"
	SafetyEvidenceGapFollowUpVerifyKillSwitch     SafetyEvidenceGapFollowUp = "VERIFY_BROKER_KILL_SWITCH_EVIDENCE"
	SafetyEvidenceGapFollowUpVerifyIdempotency    SafetyEvidenceGapFollowUp = "VERIFY_IDEMPOTENCY_RESERVATION_EVIDENCE"
	SafetyEvidenceGapFollowUpVerifyLifecycle      SafetyEvidenceGapFollowUp = "VERIFY_LIVE_LIFECYCLE_CONTRACT_EVIDENCE"
)

// SafetyEvidenceGapRemediationRequirement identifies only the exact evidence
// contract for one current unavailable category. It is not an instruction to
// execute, contact a provider, or mutate an account.
type SafetyEvidenceGapRemediationRequirement struct {
	Category            string                               `json:"category"`
	RequiredSource      EvidenceSource                       `json:"required_source"`
	GapState            SafetyEvidenceGapState               `json:"gap_state"`
	ReasonCodes         []ReasonCode                         `json:"reason_codes"`
	ResponsibleBoundary SafetyEvidenceGapResponsibleBoundary `json:"responsible_boundary"`
	SafeFollowUp        SafetyEvidenceGapFollowUp            `json:"safe_follow_up"`
}

// SafetyEvidenceGapRemediationMatrix translates an exact canonical review
// conclusion into contract-defined evidence requirements. It never claims
// readiness, completion, causality, or permission to execute.
type SafetyEvidenceGapRemediationMatrix struct {
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

// CompileSafetyEvidenceGapRemediationMatrix accepts only the exact canonical
// verification review and its complete input chain.
func CompileSafetyEvidenceGapRemediationMatrix(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
) SafetyEvidenceGapRemediationMatrix {
	matrix := SafetyEvidenceGapRemediationMatrix{
		MatrixVersion: SafetyEvidenceGapRemediationMatrixVersion,
		Status:        SafetyEvidenceGapRemediationMatrixRejected,
		Failure:       SafetyEvidenceGapRemediationMatrixFailureCompilation,
	}
	reject := func(failure SafetyEvidenceGapRemediationMatrixFailure) SafetyEvidenceGapRemediationMatrix {
		matrix.Status = SafetyEvidenceGapRemediationMatrixRejected
		matrix.Failure = failure
		return matrix
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(review)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(inventory)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(verificationReview)) {
		return reject(SafetyEvidenceGapRemediationMatrixFailureSecretInput)
	}
	if verificationReview.ExecutionAuthority || report.ExecutionAuthority || inventory.ExecutionAuthority ||
		baseline.ExecutionAuthority || current.ExecutionAuthority || envelope.Comparison.ExecutionAuthority ||
		audit.ExecutionAuthority || review.ExecutionAuthority {
		return reject(SafetyEvidenceGapRemediationMatrixFailureAuthority)
	}
	if VerifySafetyEvidenceGapInventoryVerificationReview(
		baseline, current, envelope, audit, review, inventory, report, verificationReview,
	) != nil {
		return reject(SafetyEvidenceGapRemediationMatrixFailureReview)
	}
	if !validSafetyEvidenceGapRemediationDisposition(verificationReview) {
		return reject(SafetyEvidenceGapRemediationMatrixFailureDisposition)
	}
	if verificationReview.ExpectedCategoryCount != len(verificationEvidenceExpectations()) ||
		verificationReview.CurrentGapCount != inventory.CurrentGapCount ||
		verificationReview.CurrentGapCount < 0 || verificationReview.CurrentGapCount > verificationReview.ExpectedCategoryCount {
		return reject(SafetyEvidenceGapRemediationMatrixFailureCount)
	}

	matrix.ReviewDisposition = verificationReview.Disposition
	matrix.ReviewOwnerAction = verificationReview.OwnerAction
	matrix.BaselineEvaluatedAt = verificationReview.BaselineEvaluatedAt
	matrix.CurrentEvaluatedAt = verificationReview.CurrentEvaluatedAt
	matrix.ExpectedCategoryCount = verificationReview.ExpectedCategoryCount
	matrix.CurrentGapCount = verificationReview.CurrentGapCount
	matrix.VerificationReportDigest = verificationReview.VerificationReportDigest
	matrix.VerificationReviewDigest = verificationReview.ArtifactDigest
	matrix.InventoryDigest = verificationReview.InventoryDigest.Claimed
	matrix.UpstreamReviewDigest = verificationReview.ReviewDigest.Claimed
	matrix.Requirements = make([]SafetyEvidenceGapRemediationRequirement, 0, matrix.CurrentGapCount)
	for index, gap := range inventory.Categories {
		expected := verificationEvidenceExpectations()[index]
		if gap.Category != expected.category || gap.Source != expected.source {
			return reject(SafetyEvidenceGapRemediationMatrixFailureCategory)
		}
		if !gap.CurrentUnavailable {
			continue
		}
		boundary, followUp, ok := safetyEvidenceGapRemediationContract(gap.Category)
		if !ok || len(gap.CurrentReasonCodes) == 0 {
			return reject(SafetyEvidenceGapRemediationMatrixFailureCategory)
		}
		matrix.Requirements = append(matrix.Requirements, SafetyEvidenceGapRemediationRequirement{
			Category: gap.Category, RequiredSource: gap.Source, GapState: gap.State,
			ReasonCodes:         append([]ReasonCode(nil), gap.CurrentReasonCodes...),
			ResponsibleBoundary: boundary, SafeFollowUp: followUp,
		})
	}
	if len(matrix.Requirements) != matrix.CurrentGapCount {
		return reject(SafetyEvidenceGapRemediationMatrixFailureCount)
	}
	matrix.Status = SafetyEvidenceGapRemediationMatrixCompiled
	matrix.Failure = SafetyEvidenceGapRemediationMatrixFailureNone
	digest, err := canonicalSafetyEvidenceGapRemediationMatrixDigest(matrix)
	if err != nil {
		return reject(SafetyEvidenceGapRemediationMatrixFailureCompilation)
	}
	matrix.MatrixDigest = digest
	return matrix
}

// VerifySafetyEvidenceGapRemediationMatrix recomputes the exact matrix. Any
// changed input, ordering, contract mapping, count, or digest fails closed.
func VerifySafetyEvidenceGapRemediationMatrix(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	review SafetyCaseVerificationComparisonReviewArtifact,
	inventory SafetyEvidenceGapInventory,
	report SafetyEvidenceGapInventoryVerificationReport,
	verificationReview SafetyEvidenceGapInventoryVerificationReview,
	matrix SafetyEvidenceGapRemediationMatrix,
) error {
	if matrix.MatrixVersion != SafetyEvidenceGapRemediationMatrixVersion ||
		matrix.Status != SafetyEvidenceGapRemediationMatrixCompiled ||
		matrix.Failure != SafetyEvidenceGapRemediationMatrixFailureNone || matrix.ExecutionAuthority ||
		!digestPattern.MatchString(matrix.VerificationReportDigest) ||
		!digestPattern.MatchString(matrix.VerificationReviewDigest) ||
		!digestPattern.MatchString(matrix.InventoryDigest) ||
		!digestPattern.MatchString(matrix.UpstreamReviewDigest) ||
		!digestPattern.MatchString(matrix.MatrixDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(matrix)) {
		return ErrSafetyEvidenceGapRemediationMatrix
	}
	expected := CompileSafetyEvidenceGapRemediationMatrix(
		baseline, current, envelope, audit, review, inventory, report, verificationReview,
	)
	if expected.Status != SafetyEvidenceGapRemediationMatrixCompiled || !reflect.DeepEqual(matrix, expected) {
		return ErrSafetyEvidenceGapRemediationMatrix
	}
	return nil
}

func validSafetyEvidenceGapRemediationDisposition(review SafetyEvidenceGapInventoryVerificationReview) bool {
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

func safetyEvidenceGapRemediationContract(category string) (SafetyEvidenceGapResponsibleBoundary, SafetyEvidenceGapFollowUp, bool) {
	switch category {
	case "PROVIDER_CAPABILITY":
		return SafetyEvidenceGapBoundaryFinancialProvider, SafetyEvidenceGapFollowUpVerifyProvider, true
	case "OWNER_AUTHORIZATION":
		return SafetyEvidenceGapBoundaryOwnerAuthorization, SafetyEvidenceGapFollowUpRecordAuthorization, true
	case "DETERMINISTIC_RISK":
		return SafetyEvidenceGapBoundaryRiskControl, SafetyEvidenceGapFollowUpVerifyRisk, true
	case "ACCOUNT_RECONCILIATION":
		return SafetyEvidenceGapBoundaryReconciliation, SafetyEvidenceGapFollowUpVerifyReconciliation, true
	case "BROKER_KILL_SWITCH":
		return SafetyEvidenceGapBoundaryBrokerControl, SafetyEvidenceGapFollowUpVerifyKillSwitch, true
	case "IDEMPOTENCY_RESERVATION":
		return SafetyEvidenceGapBoundaryIdempotency, SafetyEvidenceGapFollowUpVerifyIdempotency, true
	case "LIVE_LIFECYCLE_CONTRACT":
		return SafetyEvidenceGapBoundaryLifecycleContract, SafetyEvidenceGapFollowUpVerifyLifecycle, true
	default:
		return "", "", false
	}
}

func canonicalSafetyEvidenceGapRemediationMatrixDigest(matrix SafetyEvidenceGapRemediationMatrix) (string, error) {
	matrix.MatrixDigest = ""
	payload, err := json.Marshal(matrix)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(safetyEvidenceGapRemediationMatrixDigestDomain, payload), nil
}
