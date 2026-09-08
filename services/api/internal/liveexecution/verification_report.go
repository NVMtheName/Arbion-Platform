package liveexecution

import (
	"reflect"
	"time"
)

const SafetyCaseVerificationReportVersion = "live-safety-envelope-verification-v1"

type VerificationStatus string

const (
	VerificationVerified VerificationStatus = "VERIFIED"
	VerificationRejected VerificationStatus = "REJECTED"
)

type VerificationFailure string

const (
	VerificationFailureNone                    VerificationFailure = "NONE"
	VerificationFailureSecretLikeInput         VerificationFailure = "SECRET_LIKE_INPUT"
	VerificationFailureCompilerVersion         VerificationFailure = "COMPILER_VERSION_MISMATCH"
	VerificationFailureContractVersion         VerificationFailure = "CONTRACT_VERSION_MISMATCH"
	VerificationFailureEvaluationTimeMissing   VerificationFailure = "EVALUATION_TIME_MISSING"
	VerificationFailureEvaluationTimeCanonical VerificationFailure = "EVALUATION_TIME_NOT_CANONICAL"
	VerificationFailureEvaluationTimeMismatch  VerificationFailure = "EVALUATION_TIME_MISMATCH"
	VerificationFailureSafetyDigestMalformed   VerificationFailure = "SAFETY_CASE_DIGEST_MALFORMED"
	VerificationFailureRegistryDigestMalformed VerificationFailure = "REGISTRY_INPUT_DIGEST_MALFORMED"
	VerificationFailureEnvelopeDigestMalformed VerificationFailure = "ENVELOPE_DIGEST_MALFORMED"
	VerificationFailureCompilationRejected     VerificationFailure = "SAFETY_CASE_COMPILATION_REJECTED"
	VerificationFailureSafetyDigestMismatch    VerificationFailure = "SAFETY_CASE_DIGEST_MISMATCH"
	VerificationFailureRegistryDigestMismatch  VerificationFailure = "REGISTRY_INPUT_DIGEST_MISMATCH"
	VerificationFailureRegistryInputMismatch   VerificationFailure = "REGISTRY_INPUT_MISMATCH"
	VerificationFailureEnvelopeDigestMismatch  VerificationFailure = "ENVELOPE_DIGEST_MISMATCH"
	VerificationFailureEnvelopeMismatch        VerificationFailure = "ENVELOPE_MISMATCH"
)

type VersionVerification struct {
	Expected string `json:"expected"`
	Claimed  string `json:"claimed"`
	Verified bool   `json:"verified"`
}

type DigestVerification struct {
	Claimed    string `json:"claimed"`
	Recomputed string `json:"recomputed"`
	Expected   string `json:"expected"`
	Verified   bool   `json:"verified"`
}

type SafetyCaseDigestVerification struct {
	SafetyCase    DigestVerification `json:"safety_case"`
	RegistryInput DigestVerification `json:"registry_input"`
	Envelope      DigestVerification `json:"envelope"`
}

type SafetyCaseVerificationIdentity struct {
	ActionID             string `json:"action_id"`
	AssessmentKey        string `json:"assessment_key"`
	UserID               string `json:"user_id"`
	FinancialAccountID   string `json:"financial_account_id"`
	ProviderConnectionID string `json:"provider_connection_id"`
	ProviderName         string `json:"provider_name"`
	StrategyInstanceID   string `json:"strategy_instance_id"`
	MandateID            string `json:"mandate_id"`
	MandateVersion       int    `json:"mandate_version"`
	CapitalBucketID      string `json:"capital_bucket_id"`
	CapitalReservationID string `json:"capital_reservation_id"`
	ActionDigest         string `json:"action_digest_sha256"`
	BindingsVerified     bool   `json:"bindings_verified"`
}

type SafetyCaseEvidenceVerification struct {
	Category        string         `json:"category"`
	ID              string         `json:"id"`
	Digest          string         `json:"digest_sha256"`
	Source          EvidenceSource `json:"source"`
	ObservedAt      time.Time      `json:"observed_at"`
	MatchesEnvelope bool           `json:"matches_envelope"`
}

// SafetyCaseVerificationReport is a pure, credential-free explanation of an
// envelope verification result. It cannot grant execution authority and never
// contains provider payloads, credentials, or broker commands.
type SafetyCaseVerificationReport struct {
	ReportVersion      string                           `json:"report_version"`
	Status             VerificationStatus               `json:"status"`
	Failure            VerificationFailure              `json:"failure"`
	ExecutionAuthority bool                             `json:"execution_authority"`
	EvaluatedAt        time.Time                        `json:"evaluated_at"`
	CompilerVersion    VersionVerification              `json:"compiler_version"`
	ContractVersion    VersionVerification              `json:"contract_version"`
	Identity           SafetyCaseVerificationIdentity   `json:"identity"`
	Assessment         Assessment                       `json:"assessment"`
	AssessmentVerified bool                             `json:"assessment_verified"`
	Evidence           []SafetyCaseEvidenceVerification `json:"evidence"`
	Digests            SafetyCaseDigestVerification     `json:"digests"`
}

// BuildSafetyCaseVerificationReport verifies every envelope layer and returns
// a closed result with one exact failure category. A VERIFIED report remains
// review evidence only and never represents permission to execute.
func BuildSafetyCaseVerificationReport(safetyCase SafetyCase, envelope SafetyCaseCompilationEnvelope) SafetyCaseVerificationReport {
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(safetyCase)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) {
		return SafetyCaseVerificationReport{
			ReportVersion: SafetyCaseVerificationReportVersion,
			Status:        VerificationRejected,
			Failure:       VerificationFailureSecretLikeInput,
		}
	}
	report := SafetyCaseVerificationReport{
		ReportVersion: SafetyCaseVerificationReportVersion,
		Status:        VerificationRejected,
		Failure:       VerificationFailureEnvelopeMismatch,
		EvaluatedAt:   envelope.EvaluatedAt,
		CompilerVersion: VersionVerification{
			Expected: SafetyCaseCompilerVersion, Claimed: envelope.CompilerVersion,
			Verified: envelope.CompilerVersion == SafetyCaseCompilerVersion,
		},
		ContractVersion: VersionVerification{
			Expected: ContractVersion, Claimed: envelope.ContractVersion,
			Verified: envelope.ContractVersion == ContractVersion,
		},
		Digests: SafetyCaseDigestVerification{
			SafetyCase:    DigestVerification{Claimed: envelope.SafetyCaseDigest},
			RegistryInput: DigestVerification{Claimed: envelope.RegistryInputDigest},
			Envelope:      DigestVerification{Claimed: envelope.EnvelopeDigest},
		},
	}
	reject := func(failure VerificationFailure) SafetyCaseVerificationReport {
		report.Status = VerificationRejected
		report.Failure = failure
		return report
	}

	if !report.CompilerVersion.Verified {
		return reject(VerificationFailureCompilerVersion)
	}
	if !report.ContractVersion.Verified {
		return reject(VerificationFailureContractVersion)
	}
	if envelope.EvaluatedAt.IsZero() {
		return reject(VerificationFailureEvaluationTimeMissing)
	}
	if envelope.EvaluatedAt.Location() != time.UTC {
		return reject(VerificationFailureEvaluationTimeCanonical)
	}
	if !envelope.RegistryInput.ObservedAt.Equal(envelope.EvaluatedAt) {
		return reject(VerificationFailureEvaluationTimeMismatch)
	}
	if !digestPattern.MatchString(envelope.SafetyCaseDigest) {
		return reject(VerificationFailureSafetyDigestMalformed)
	}
	if !digestPattern.MatchString(envelope.RegistryInputDigest) {
		return reject(VerificationFailureRegistryDigestMalformed)
	}
	if !digestPattern.MatchString(envelope.EnvelopeDigest) {
		return reject(VerificationFailureEnvelopeDigestMalformed)
	}

	expected, err := CompileSafetyCaseEnvelope(safetyCase, envelope.EvaluatedAt)
	if err != nil {
		return reject(VerificationFailureCompilationRejected)
	}
	report.Identity = verificationIdentity(safetyCase, expected.RegistryInput)
	report.Assessment = expected.RegistryInput.Assessment
	report.AssessmentVerified = reflect.DeepEqual(envelope.RegistryInput.Assessment, expected.RegistryInput.Assessment)
	report.Evidence = verificationEvidence(expected.RegistryInput.Evidence, envelope.RegistryInput.Evidence)
	report.Digests.SafetyCase.Recomputed = expected.SafetyCaseDigest
	report.Digests.SafetyCase.Expected = expected.SafetyCaseDigest
	if envelope.SafetyCaseDigest != expected.SafetyCaseDigest {
		return reject(VerificationFailureSafetyDigestMismatch)
	}
	report.Digests.SafetyCase.Verified = true

	recomputedRegistryDigest, err := canonicalRegistryInputDigest(envelope.RegistryInput)
	if err != nil {
		return reject(VerificationFailureRegistryDigestMismatch)
	}
	report.Digests.RegistryInput.Recomputed = recomputedRegistryDigest
	report.Digests.RegistryInput.Expected = expected.RegistryInputDigest
	if envelope.RegistryInputDigest != recomputedRegistryDigest {
		return reject(VerificationFailureRegistryDigestMismatch)
	}
	if !reflect.DeepEqual(envelope.RegistryInput, expected.RegistryInput) ||
		envelope.RegistryInputDigest != expected.RegistryInputDigest {
		return reject(VerificationFailureRegistryInputMismatch)
	}
	report.Digests.RegistryInput.Verified = true
	report.Identity.BindingsVerified = true

	recomputedEnvelopeDigest, err := canonicalEnvelopeDigest(
		envelope.CompilerVersion,
		envelope.ContractVersion,
		envelope.EvaluatedAt,
		envelope.SafetyCaseDigest,
		envelope.RegistryInputDigest,
	)
	if err != nil {
		return reject(VerificationFailureEnvelopeDigestMismatch)
	}
	report.Digests.Envelope.Recomputed = recomputedEnvelopeDigest
	report.Digests.Envelope.Expected = expected.EnvelopeDigest
	if envelope.EnvelopeDigest != recomputedEnvelopeDigest {
		return reject(VerificationFailureEnvelopeDigestMismatch)
	}
	if envelope.EnvelopeDigest != expected.EnvelopeDigest || !reflect.DeepEqual(envelope, expected) {
		return reject(VerificationFailureEnvelopeMismatch)
	}
	report.Digests.Envelope.Verified = true
	report.Status = VerificationVerified
	report.Failure = VerificationFailureNone
	return report
}

func verificationIdentity(safetyCase SafetyCase, input RegistryInput) SafetyCaseVerificationIdentity {
	return SafetyCaseVerificationIdentity{
		ActionID: safetyCase.Action.ID, AssessmentKey: input.AssessmentKey,
		UserID: safetyCase.Action.UserID, FinancialAccountID: input.FinancialAccountID,
		ProviderConnectionID: input.ProviderConnectionID, ProviderName: input.ProviderName,
		StrategyInstanceID: input.StrategyInstanceID, MandateID: input.MandateID,
		MandateVersion: input.MandateVersion, CapitalBucketID: input.CapitalBucketID,
		CapitalReservationID: input.CapitalReservationID, ActionDigest: input.ActionDigest,
	}
}

func verificationEvidence(expected, claimed EvidenceManifest) []SafetyCaseEvidenceVerification {
	result := make([]SafetyCaseEvidenceVerification, 0, len(expected.Items))
	for index, item := range expected.Items {
		matches := index < len(claimed.Items) && reflect.DeepEqual(item, claimed.Items[index])
		result = append(result, SafetyCaseEvidenceVerification{
			Category: item.Kind, ID: item.ID, Digest: item.Digest,
			Source: item.Source, ObservedAt: item.ObservedAt, MatchesEnvelope: matches,
		})
	}
	return result
}
