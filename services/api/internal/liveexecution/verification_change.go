package liveexecution

import (
	"reflect"
	"sort"
	"strconv"
	"time"
)

const SafetyCaseVerificationComparisonVersion = "live-safety-verification-comparison-v1"

type VerificationComparisonStatus string

const (
	VerificationComparisonSame     VerificationComparisonStatus = "SAME"
	VerificationComparisonChanged  VerificationComparisonStatus = "CHANGED"
	VerificationComparisonRejected VerificationComparisonStatus = "REJECTED"
)

type VerificationComparisonInput string

const (
	VerificationComparisonInputNone     VerificationComparisonInput = "NONE"
	VerificationComparisonInputBaseline VerificationComparisonInput = "BASELINE"
	VerificationComparisonInputCurrent  VerificationComparisonInput = "CURRENT"
)

type VerificationComparisonFailure string

const (
	VerificationComparisonFailureNone          VerificationComparisonFailure = "NONE"
	VerificationComparisonFailureSecretInput   VerificationComparisonFailure = "SECRET_LIKE_INPUT"
	VerificationComparisonFailureReportVersion VerificationComparisonFailure = "REPORT_VERSION_INVALID"
	VerificationComparisonFailureStatus        VerificationComparisonFailure = "VERIFICATION_STATUS_INVALID"
	VerificationComparisonFailureAuthority     VerificationComparisonFailure = "EXECUTION_AUTHORITY_PRESENT"
	VerificationComparisonFailureTime          VerificationComparisonFailure = "EVALUATION_TIME_INVALID"
	VerificationComparisonFailureTimeOrder     VerificationComparisonFailure = "EVALUATION_TIME_REGRESSION"
	VerificationComparisonFailureVersion       VerificationComparisonFailure = "VERIFIED_VERSION_INVALID"
	VerificationComparisonFailureIdentity      VerificationComparisonFailure = "IDENTITY_INVALID"
	VerificationComparisonFailureAssessment    VerificationComparisonFailure = "ASSESSMENT_INVALID"
	VerificationComparisonFailureEvidence      VerificationComparisonFailure = "EVIDENCE_INVALID"
	VerificationComparisonFailureDigest        VerificationComparisonFailure = "DIGEST_INVALID"
)

type VerificationChangeState string

const (
	VerificationChangeSame    VerificationChangeState = "SAME"
	VerificationChangeChanged VerificationChangeState = "CHANGED"
)

type VerificationValueChange struct {
	Field  string                  `json:"field"`
	Before string                  `json:"before"`
	After  string                  `json:"after"`
	State  VerificationChangeState `json:"state"`
}

type VerificationAssessmentChange struct {
	Before Assessment              `json:"before"`
	After  Assessment              `json:"after"`
	State  VerificationChangeState `json:"state"`
}

type VerificationEvidenceChange struct {
	Category string                         `json:"category"`
	Before   SafetyCaseEvidenceVerification `json:"before"`
	After    SafetyCaseEvidenceVerification `json:"after"`
	State    VerificationChangeState        `json:"state"`
}

type verificationEvidenceExpectation struct {
	category string
	source   EvidenceSource
}

// SafetyCaseVerificationComparison is a credential-free, non-authoritative
// comparison of two already-verified reports. It preserves exact facts and
// never infers the cause or quality of a change.
type SafetyCaseVerificationComparison struct {
	ComparisonVersion   string                        `json:"comparison_version"`
	Status              VerificationComparisonStatus  `json:"status"`
	FailureInput        VerificationComparisonInput   `json:"failure_input"`
	Failure             VerificationComparisonFailure `json:"failure"`
	ExecutionAuthority  bool                          `json:"execution_authority"`
	BaselineEvaluatedAt time.Time                     `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt  time.Time                     `json:"current_evaluated_at"`
	EvaluationTime      VerificationValueChange       `json:"evaluation_time"`
	Versions            []VerificationValueChange     `json:"versions"`
	Identities          []VerificationValueChange     `json:"identities"`
	Digests             []VerificationValueChange     `json:"digests"`
	Assessment          VerificationAssessmentChange  `json:"assessment"`
	Evidence            []VerificationEvidenceChange  `json:"evidence"`
	ChangeCount         int                           `json:"change_count"`
}

// CompareSafetyCaseVerificationReports produces an exact SAME, CHANGED, or
// REJECTED result. Inputs that are not complete current-version VERIFIED
// reports fail closed, and no result can represent execution authority.
func CompareSafetyCaseVerificationReports(baseline, current SafetyCaseVerificationReport) SafetyCaseVerificationComparison {
	comparison := SafetyCaseVerificationComparison{
		ComparisonVersion: SafetyCaseVerificationComparisonVersion,
		Status:            VerificationComparisonRejected,
		FailureInput:      VerificationComparisonInputNone,
		Failure:           VerificationComparisonFailureNone,
	}
	reject := func(input VerificationComparisonInput, failure VerificationComparisonFailure) SafetyCaseVerificationComparison {
		comparison.Status = VerificationComparisonRejected
		comparison.FailureInput = input
		comparison.Failure = failure
		return comparison
	}
	if failure := validateComparableVerificationReport(baseline); failure != VerificationComparisonFailureNone {
		return reject(VerificationComparisonInputBaseline, failure)
	}
	if failure := validateComparableVerificationReport(current); failure != VerificationComparisonFailureNone {
		return reject(VerificationComparisonInputCurrent, failure)
	}
	if current.EvaluatedAt.Before(baseline.EvaluatedAt) {
		return reject(VerificationComparisonInputCurrent, VerificationComparisonFailureTimeOrder)
	}

	comparison.BaselineEvaluatedAt = baseline.EvaluatedAt
	comparison.CurrentEvaluatedAt = current.EvaluatedAt
	comparison.EvaluationTime = verificationValueChange(
		"evaluated_at",
		baseline.EvaluatedAt.Format(time.RFC3339Nano),
		current.EvaluatedAt.Format(time.RFC3339Nano),
	)
	comparison.Versions = []VerificationValueChange{
		verificationValueChange("report_version", baseline.ReportVersion, current.ReportVersion),
		verificationValueChange("compiler_version", baseline.CompilerVersion.Claimed, current.CompilerVersion.Claimed),
		verificationValueChange("contract_version", baseline.ContractVersion.Claimed, current.ContractVersion.Claimed),
	}
	comparison.Identities = verificationIdentityChanges(baseline.Identity, current.Identity)
	comparison.Digests = []VerificationValueChange{
		verificationValueChange("safety_case_sha256", baseline.Digests.SafetyCase.Claimed, current.Digests.SafetyCase.Claimed),
		verificationValueChange("registry_input_sha256", baseline.Digests.RegistryInput.Claimed, current.Digests.RegistryInput.Claimed),
		verificationValueChange("envelope_sha256", baseline.Digests.Envelope.Claimed, current.Digests.Envelope.Claimed),
	}
	comparison.Assessment = VerificationAssessmentChange{
		Before: baseline.Assessment,
		After:  current.Assessment,
		State:  verificationChangeState(reflect.DeepEqual(baseline.Assessment, current.Assessment)),
	}
	baselineEvidence := verificationEvidenceByCategory(baseline.Evidence)
	currentEvidence := verificationEvidenceByCategory(current.Evidence)
	comparison.Evidence = make([]VerificationEvidenceChange, 0, len(baseline.Evidence))
	for _, expected := range verificationEvidenceExpectations() {
		before := baselineEvidence[expected.category]
		after := currentEvidence[expected.category]
		comparison.Evidence = append(comparison.Evidence, VerificationEvidenceChange{
			Category: before.Category,
			Before:   before,
			After:    after,
			State:    verificationChangeState(reflect.DeepEqual(before, after)),
		})
	}
	comparison.ChangeCount = countVerificationChanges(comparison)
	comparison.Status = VerificationComparisonSame
	if comparison.ChangeCount > 0 {
		comparison.Status = VerificationComparisonChanged
	}
	return comparison
}

func validateComparableVerificationReport(report SafetyCaseVerificationReport) VerificationComparisonFailure {
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(report)) {
		return VerificationComparisonFailureSecretInput
	}
	if report.ReportVersion != SafetyCaseVerificationReportVersion {
		return VerificationComparisonFailureReportVersion
	}
	if report.Status != VerificationVerified || report.Failure != VerificationFailureNone {
		return VerificationComparisonFailureStatus
	}
	if report.ExecutionAuthority {
		return VerificationComparisonFailureAuthority
	}
	if report.EvaluatedAt.IsZero() || report.EvaluatedAt.Location() != time.UTC {
		return VerificationComparisonFailureTime
	}
	if !exactVerifiedVersion(report.CompilerVersion, SafetyCaseCompilerVersion) ||
		!exactVerifiedVersion(report.ContractVersion, ContractVersion) {
		return VerificationComparisonFailureVersion
	}
	if !validVerificationIdentity(report.Identity) {
		return VerificationComparisonFailureIdentity
	}
	if !validVerificationAssessment(report) {
		return VerificationComparisonFailureAssessment
	}
	if !validVerificationEvidence(report) {
		return VerificationComparisonFailureEvidence
	}
	if !exactVerifiedDigest(report.Digests.SafetyCase) ||
		!exactVerifiedDigest(report.Digests.RegistryInput) ||
		!exactVerifiedDigest(report.Digests.Envelope) {
		return VerificationComparisonFailureDigest
	}
	return VerificationComparisonFailureNone
}

func exactVerifiedVersion(value VersionVerification, expected string) bool {
	return value.Verified && value.Expected == expected && value.Claimed == expected
}

func exactVerifiedDigest(value DigestVerification) bool {
	return value.Verified && digestPattern.MatchString(value.Claimed) &&
		value.Claimed == value.Recomputed && value.Claimed == value.Expected
}

func validVerificationIdentity(value SafetyCaseVerificationIdentity) bool {
	return value.BindingsVerified && uuidPattern.MatchString(value.ActionID) &&
		assessmentKeyPattern.MatchString(value.AssessmentKey) && uuidPattern.MatchString(value.UserID) &&
		uuidPattern.MatchString(value.FinancialAccountID) && uuidPattern.MatchString(value.ProviderConnectionID) &&
		providerPattern.MatchString(value.ProviderName) && uuidPattern.MatchString(value.StrategyInstanceID) &&
		uuidPattern.MatchString(value.MandateID) && value.MandateVersion > 0 &&
		uuidPattern.MatchString(value.CapitalBucketID) && uuidPattern.MatchString(value.CapitalReservationID) &&
		digestPattern.MatchString(value.ActionDigest)
}

func validVerificationAssessment(report SafetyCaseVerificationReport) bool {
	assessment := report.Assessment
	if !report.AssessmentVerified || !assessment.Structural ||
		(assessment.Availability != Unavailable && assessment.Availability != BlockedUnimplemented) ||
		len(assessment.ReasonCodes) == 0 || len(assessment.ReasonCodes) > MaxRegistryReasons {
		return false
	}
	reasons := append([]ReasonCode(nil), assessment.ReasonCodes...)
	for index, reason := range reasons {
		if !isRegistryReason(reason) || (index > 0 && reasons[index-1] >= reason) {
			return false
		}
	}
	return sort.SliceIsSorted(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
}

func validVerificationEvidence(report SafetyCaseVerificationReport) bool {
	expected := verificationEvidenceExpectations()
	if len(report.Evidence) != len(expected) {
		return false
	}
	expectedSources := make(map[string]EvidenceSource, len(expected))
	for _, item := range expected {
		expectedSources[item.category] = item.source
	}
	seenIDs := make(map[string]struct{}, len(expected))
	seenCategories := make(map[string]struct{}, len(expected))
	for _, item := range report.Evidence {
		expectedSource, categoryExists := expectedSources[item.Category]
		if !categoryExists || item.Source != expectedSource ||
			!item.MatchesEnvelope || !uuidPattern.MatchString(item.ID) ||
			!digestPattern.MatchString(item.Digest) || item.ObservedAt.IsZero() ||
			item.ObservedAt.Location() != time.UTC || item.ObservedAt.After(report.EvaluatedAt) {
			return false
		}
		if _, exists := seenIDs[item.ID]; exists {
			return false
		}
		if _, exists := seenCategories[item.Category]; exists {
			return false
		}
		seenIDs[item.ID] = struct{}{}
		seenCategories[item.Category] = struct{}{}
	}
	return len(seenCategories) == len(expected)
}

func verificationEvidenceExpectations() []verificationEvidenceExpectation {
	return []verificationEvidenceExpectation{
		{"PROVIDER_CAPABILITY", EvidenceProviderVerified},
		{"OWNER_AUTHORIZATION", EvidenceOwnerMFA},
		{"DETERMINISTIC_RISK", EvidenceDeterministicControl},
		{"ACCOUNT_RECONCILIATION", EvidenceDatabase},
		{"BROKER_KILL_SWITCH", EvidenceDeterministicControl},
		{"IDEMPOTENCY_RESERVATION", EvidenceDatabase},
		{"LIVE_LIFECYCLE_CONTRACT", EvidenceDesignContract},
	}
}

func verificationEvidenceByCategory(items []SafetyCaseEvidenceVerification) map[string]SafetyCaseEvidenceVerification {
	result := make(map[string]SafetyCaseEvidenceVerification, len(items))
	for _, item := range items {
		result[item.Category] = item
	}
	return result
}

func verificationIdentityChanges(before, after SafetyCaseVerificationIdentity) []VerificationValueChange {
	return []VerificationValueChange{
		verificationValueChange("action_id", before.ActionID, after.ActionID),
		verificationValueChange("assessment_key", before.AssessmentKey, after.AssessmentKey),
		verificationValueChange("user_id", before.UserID, after.UserID),
		verificationValueChange("financial_account_id", before.FinancialAccountID, after.FinancialAccountID),
		verificationValueChange("provider_connection_id", before.ProviderConnectionID, after.ProviderConnectionID),
		verificationValueChange("provider_name", before.ProviderName, after.ProviderName),
		verificationValueChange("strategy_instance_id", before.StrategyInstanceID, after.StrategyInstanceID),
		verificationValueChange("mandate_id", before.MandateID, after.MandateID),
		verificationValueChange("mandate_version", strconv.Itoa(before.MandateVersion), strconv.Itoa(after.MandateVersion)),
		verificationValueChange("capital_bucket_id", before.CapitalBucketID, after.CapitalBucketID),
		verificationValueChange("capital_reservation_id", before.CapitalReservationID, after.CapitalReservationID),
		verificationValueChange("action_digest_sha256", before.ActionDigest, after.ActionDigest),
	}
}

func verificationValueChange(field, before, after string) VerificationValueChange {
	return VerificationValueChange{Field: field, Before: before, After: after, State: verificationChangeState(before == after)}
}

func verificationChangeState(same bool) VerificationChangeState {
	if same {
		return VerificationChangeSame
	}
	return VerificationChangeChanged
}

func countVerificationChanges(comparison SafetyCaseVerificationComparison) int {
	count := 0
	if comparison.EvaluationTime.State == VerificationChangeChanged {
		count++
	}
	for _, changes := range [][]VerificationValueChange{comparison.Versions, comparison.Identities, comparison.Digests} {
		for _, change := range changes {
			if change.State == VerificationChangeChanged {
				count++
			}
		}
	}
	if comparison.Assessment.State == VerificationChangeChanged {
		count++
	}
	for _, change := range comparison.Evidence {
		if change.State == VerificationChangeChanged {
			count++
		}
	}
	return count
}
