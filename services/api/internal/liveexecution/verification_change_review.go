package liveexecution

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

const SafetyCaseVerificationComparisonReviewArtifactVersion = "live-safety-verification-comparison-review-v1"

const (
	verificationComparisonAuditReportDigestDomain = "arbion:live-safety:verification-comparison-envelope-report:v1"
	verificationComparisonReviewDigestDomain      = "arbion:live-safety:verification-comparison-review-artifact:v1"
)

var ErrSafetyCaseVerificationComparisonReviewArtifact = errors.New("live safety verification comparison review artifact is invalid")

type VerificationComparisonReviewStatus string

const (
	VerificationComparisonReviewReady    VerificationComparisonReviewStatus = "REVIEW_READY"
	VerificationComparisonReviewRejected VerificationComparisonReviewStatus = "REJECTED"
)

type VerificationComparisonReviewDisposition string

const (
	VerificationComparisonReviewNoChange      VerificationComparisonReviewDisposition = "NO_CHANGE"
	VerificationComparisonReviewChanged       VerificationComparisonReviewDisposition = "CHANGE_REVIEW_REQUIRED"
	VerificationComparisonReviewInputRejected VerificationComparisonReviewDisposition = "INPUT_REJECTION_REVIEW_REQUIRED"
)

type VerificationComparisonOwnerAction string

const (
	VerificationComparisonOwnerActionNone            VerificationComparisonOwnerAction = "NONE"
	VerificationComparisonOwnerActionReviewChanges   VerificationComparisonOwnerAction = "REVIEW_EXACT_CHANGES"
	VerificationComparisonOwnerActionReviewRejection VerificationComparisonOwnerAction = "REVIEW_REJECTED_INPUT"
)

type VerificationComparisonReviewFailure string

const (
	VerificationComparisonReviewFailureNone             VerificationComparisonReviewFailure = "NONE"
	VerificationComparisonReviewFailureSecretInput      VerificationComparisonReviewFailure = "SECRET_LIKE_INPUT"
	VerificationComparisonReviewFailureReportVersion    VerificationComparisonReviewFailure = "AUDIT_REPORT_VERSION_INVALID"
	VerificationComparisonReviewFailureReportStatus     VerificationComparisonReviewFailure = "AUDIT_REPORT_STATUS_INVALID"
	VerificationComparisonReviewFailureAuthority        VerificationComparisonReviewFailure = "EXECUTION_AUTHORITY_PRESENT"
	VerificationComparisonReviewFailureReportMismatch   VerificationComparisonReviewFailure = "AUDIT_REPORT_MISMATCH"
	VerificationComparisonReviewFailureComparisonStatus VerificationComparisonReviewFailure = "COMPARISON_STATUS_INVALID"
	VerificationComparisonReviewFailureIdentity         VerificationComparisonReviewFailure = "IDENTITY_EVIDENCE_INVALID"
	VerificationComparisonReviewFailureDigest           VerificationComparisonReviewFailure = "DIGEST_EVIDENCE_INVALID"
	VerificationComparisonReviewFailureCompilation      VerificationComparisonReviewFailure = "REVIEW_COMPILATION_FAILED"
)

// SafetyCaseVerificationComparisonReviewArtifact is a canonical, owner-facing
// summary of an independently verified comparison envelope. It preserves exact
// comparison semantics and evidence but cannot accept, approve, or execute an
// action.
type SafetyCaseVerificationComparisonReviewArtifact struct {
	ArtifactVersion        string                                          `json:"artifact_version"`
	Status                 VerificationComparisonReviewStatus              `json:"status"`
	Failure                VerificationComparisonReviewFailure             `json:"failure"`
	ExecutionAuthority     bool                                            `json:"execution_authority"`
	Disposition            VerificationComparisonReviewDisposition         `json:"disposition"`
	OwnerAction            VerificationComparisonOwnerAction               `json:"owner_action"`
	AuditReportVersion     string                                          `json:"audit_report_version"`
	EnvelopeVersion        string                                          `json:"envelope_version"`
	ComparisonVersion      string                                          `json:"comparison_version"`
	BaselineEvaluatedAt    time.Time                                       `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt     time.Time                                       `json:"current_evaluated_at"`
	ComparisonStatus       VerificationComparisonStatus                    `json:"comparison_status"`
	ComparisonFailureInput VerificationComparisonInput                     `json:"comparison_failure_input"`
	ComparisonFailure      VerificationComparisonFailure                   `json:"comparison_failure"`
	ChangeCount            int                                             `json:"change_count"`
	Identities             []VerificationValueChange                       `json:"identities"`
	Digests                SafetyCaseVerificationComparisonEnvelopeDigests `json:"digests"`
	AuditReportDigest      string                                          `json:"audit_report_sha256"`
	ReviewDigest           string                                          `json:"review_sha256"`
}

// BuildSafetyCaseVerificationComparisonReviewArtifact accepts only the exact
// independently VERIFIED audit report for the supplied inputs and envelope.
// It rejects protected or inconsistent input before copying it into output.
func BuildSafetyCaseVerificationComparisonReviewArtifact(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
) SafetyCaseVerificationComparisonReviewArtifact {
	artifact := SafetyCaseVerificationComparisonReviewArtifact{
		ArtifactVersion: SafetyCaseVerificationComparisonReviewArtifactVersion,
		Status:          VerificationComparisonReviewRejected,
		Failure:         VerificationComparisonReviewFailureCompilation,
	}
	reject := func(failure VerificationComparisonReviewFailure) SafetyCaseVerificationComparisonReviewArtifact {
		artifact.Status = VerificationComparisonReviewRejected
		artifact.Failure = failure
		return artifact
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(audit)) {
		return reject(VerificationComparisonReviewFailureSecretInput)
	}
	if audit.ReportVersion != SafetyCaseVerificationComparisonEnvelopeReportVersion {
		return reject(VerificationComparisonReviewFailureReportVersion)
	}
	if audit.Status != VerificationVerified || audit.Failure != VerificationComparisonEnvelopeFailureNone {
		return reject(VerificationComparisonReviewFailureReportStatus)
	}
	if audit.ExecutionAuthority || envelope.Comparison.ExecutionAuthority {
		return reject(VerificationComparisonReviewFailureAuthority)
	}
	if !validVerificationComparisonReviewStatus(envelope.Comparison) {
		return reject(VerificationComparisonReviewFailureComparisonStatus)
	}
	if !validVerificationComparisonReviewIdentities(envelope.Comparison) {
		return reject(VerificationComparisonReviewFailureIdentity)
	}
	if !exactVerifiedDigest(audit.Digests.BaselineReport) ||
		!exactVerifiedDigest(audit.Digests.CurrentReport) ||
		!exactVerifiedDigest(audit.Digests.Comparison) ||
		!exactVerifiedDigest(audit.Digests.Envelope) {
		return reject(VerificationComparisonReviewFailureDigest)
	}
	expectedAudit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, envelope)
	if expectedAudit.Status != VerificationVerified || !reflect.DeepEqual(audit, expectedAudit) {
		return reject(VerificationComparisonReviewFailureReportMismatch)
	}

	disposition, ownerAction := verificationComparisonReviewConclusion(envelope.Comparison.Status)
	auditDigest, err := canonicalVerificationComparisonAuditReportDigest(audit)
	if err != nil {
		return reject(VerificationComparisonReviewFailureCompilation)
	}
	artifact.Status = VerificationComparisonReviewReady
	artifact.Failure = VerificationComparisonReviewFailureNone
	artifact.Disposition = disposition
	artifact.OwnerAction = ownerAction
	artifact.AuditReportVersion = audit.ReportVersion
	artifact.EnvelopeVersion = envelope.EnvelopeVersion
	artifact.ComparisonVersion = envelope.ComparisonVersion
	artifact.BaselineEvaluatedAt = audit.BaselineEvaluatedAt
	artifact.CurrentEvaluatedAt = audit.CurrentEvaluatedAt
	artifact.ComparisonStatus = envelope.Comparison.Status
	artifact.ComparisonFailureInput = envelope.Comparison.FailureInput
	artifact.ComparisonFailure = envelope.Comparison.Failure
	artifact.ChangeCount = envelope.Comparison.ChangeCount
	artifact.Identities = append([]VerificationValueChange(nil), envelope.Comparison.Identities...)
	artifact.Digests = audit.Digests
	artifact.AuditReportDigest = auditDigest
	artifact.ReviewDigest, err = canonicalVerificationComparisonReviewArtifactDigest(artifact)
	if err != nil {
		return reject(VerificationComparisonReviewFailureCompilation)
	}
	return artifact
}

// VerifySafetyCaseVerificationComparisonReviewArtifact recomputes the complete
// artifact. Any changed input or summary fact fails closed.
func VerifySafetyCaseVerificationComparisonReviewArtifact(
	baseline, current SafetyCaseVerificationReport,
	envelope SafetyCaseVerificationComparisonEnvelope,
	audit SafetyCaseVerificationComparisonEnvelopeReport,
	artifact SafetyCaseVerificationComparisonReviewArtifact,
) error {
	if artifact.ArtifactVersion != SafetyCaseVerificationComparisonReviewArtifactVersion ||
		artifact.Status != VerificationComparisonReviewReady ||
		artifact.Failure != VerificationComparisonReviewFailureNone ||
		artifact.ExecutionAuthority ||
		!digestPattern.MatchString(artifact.AuditReportDigest) ||
		!digestPattern.MatchString(artifact.ReviewDigest) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(artifact)) {
		return ErrSafetyCaseVerificationComparisonReviewArtifact
	}
	expected := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit)
	if expected.Status != VerificationComparisonReviewReady || !reflect.DeepEqual(artifact, expected) {
		return ErrSafetyCaseVerificationComparisonReviewArtifact
	}
	return nil
}

func verificationComparisonReviewConclusion(status VerificationComparisonStatus) (VerificationComparisonReviewDisposition, VerificationComparisonOwnerAction) {
	switch status {
	case VerificationComparisonSame:
		return VerificationComparisonReviewNoChange, VerificationComparisonOwnerActionNone
	case VerificationComparisonChanged:
		return VerificationComparisonReviewChanged, VerificationComparisonOwnerActionReviewChanges
	default:
		return VerificationComparisonReviewInputRejected, VerificationComparisonOwnerActionReviewRejection
	}
}

func validVerificationComparisonReviewStatus(comparison SafetyCaseVerificationComparison) bool {
	switch comparison.Status {
	case VerificationComparisonSame:
		return comparison.FailureInput == VerificationComparisonInputNone &&
			comparison.Failure == VerificationComparisonFailureNone && comparison.ChangeCount == 0
	case VerificationComparisonChanged:
		return comparison.FailureInput == VerificationComparisonInputNone &&
			comparison.Failure == VerificationComparisonFailureNone && comparison.ChangeCount > 0
	case VerificationComparisonRejected:
		return comparison.FailureInput != VerificationComparisonInputNone &&
			comparison.Failure != VerificationComparisonFailureNone && comparison.ChangeCount == 0
	default:
		return false
	}
}

func validVerificationComparisonReviewIdentities(comparison SafetyCaseVerificationComparison) bool {
	if comparison.Status == VerificationComparisonRejected {
		return len(comparison.Identities) == 0
	}
	expectedFields := []string{
		"action_id", "assessment_key", "user_id", "financial_account_id", "provider_connection_id",
		"provider_name", "strategy_instance_id", "mandate_id", "mandate_version", "capital_bucket_id",
		"capital_reservation_id", "action_digest_sha256",
	}
	if len(comparison.Identities) != len(expectedFields) {
		return false
	}
	for index, change := range comparison.Identities {
		if change.Field != expectedFields[index] ||
			(change.State != VerificationChangeSame && change.State != VerificationChangeChanged) ||
			(change.State == VerificationChangeSame) != (change.Before == change.After) {
			return false
		}
	}
	return true
}

func canonicalVerificationComparisonAuditReportDigest(report SafetyCaseVerificationComparisonEnvelopeReport) (string, error) {
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(verificationComparisonAuditReportDigestDomain, payload), nil
}

func canonicalVerificationComparisonReviewArtifactDigest(artifact SafetyCaseVerificationComparisonReviewArtifact) (string, error) {
	artifact.ReviewDigest = ""
	payload, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(verificationComparisonReviewDigestDomain, payload), nil
}
