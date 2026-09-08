package liveexecution

import (
	"reflect"
	"time"
)

const SafetyCaseVerificationComparisonEnvelopeReportVersion = "live-safety-verification-comparison-envelope-report-v1"

type VerificationComparisonEnvelopeFailure string

const (
	VerificationComparisonEnvelopeFailureNone                VerificationComparisonEnvelopeFailure = "NONE"
	VerificationComparisonEnvelopeFailureSecretInput         VerificationComparisonEnvelopeFailure = "SECRET_LIKE_INPUT"
	VerificationComparisonEnvelopeFailureEnvelopeVersion     VerificationComparisonEnvelopeFailure = "ENVELOPE_VERSION_MISMATCH"
	VerificationComparisonEnvelopeFailureComparisonVersion   VerificationComparisonEnvelopeFailure = "COMPARISON_VERSION_MISMATCH"
	VerificationComparisonEnvelopeFailureAuthority           VerificationComparisonEnvelopeFailure = "EXECUTION_AUTHORITY_PRESENT"
	VerificationComparisonEnvelopeFailureBaselineMalformed   VerificationComparisonEnvelopeFailure = "BASELINE_REPORT_DIGEST_MALFORMED"
	VerificationComparisonEnvelopeFailureCurrentMalformed    VerificationComparisonEnvelopeFailure = "CURRENT_REPORT_DIGEST_MALFORMED"
	VerificationComparisonEnvelopeFailureComparisonMalformed VerificationComparisonEnvelopeFailure = "COMPARISON_DIGEST_MALFORMED"
	VerificationComparisonEnvelopeFailureEnvelopeMalformed   VerificationComparisonEnvelopeFailure = "ENVELOPE_DIGEST_MALFORMED"
	VerificationComparisonEnvelopeFailureBaselineMismatch    VerificationComparisonEnvelopeFailure = "BASELINE_REPORT_DIGEST_MISMATCH"
	VerificationComparisonEnvelopeFailureCurrentMismatch     VerificationComparisonEnvelopeFailure = "CURRENT_REPORT_DIGEST_MISMATCH"
	VerificationComparisonEnvelopeFailureComparisonMismatch  VerificationComparisonEnvelopeFailure = "COMPARISON_MISMATCH"
	VerificationComparisonEnvelopeFailureComparisonDigest    VerificationComparisonEnvelopeFailure = "COMPARISON_DIGEST_MISMATCH"
	VerificationComparisonEnvelopeFailureEnvelopeDigest      VerificationComparisonEnvelopeFailure = "ENVELOPE_DIGEST_MISMATCH"
	VerificationComparisonEnvelopeFailureEnvelopeMismatch    VerificationComparisonEnvelopeFailure = "ENVELOPE_MISMATCH"
	VerificationComparisonEnvelopeFailureCompilationRejected VerificationComparisonEnvelopeFailure = "COMPILATION_REJECTED"
)

type SafetyCaseVerificationComparisonEnvelopeDigests struct {
	BaselineReport DigestVerification `json:"baseline_report"`
	CurrentReport  DigestVerification `json:"current_report"`
	Comparison     DigestVerification `json:"comparison"`
	Envelope       DigestVerification `json:"envelope"`
}

// SafetyCaseVerificationComparisonEnvelopeReport independently explains
// whether every layer of a comparison envelope matches its exact two input
// reports. A VERIFIED report remains review evidence only, including when the
// enclosed comparison correctly records a REJECTED input pair.
type SafetyCaseVerificationComparisonEnvelopeReport struct {
	ReportVersion       string                                          `json:"report_version"`
	Status              VerificationStatus                              `json:"status"`
	Failure             VerificationComparisonEnvelopeFailure           `json:"failure"`
	ExecutionAuthority  bool                                            `json:"execution_authority"`
	BaselineEvaluatedAt time.Time                                       `json:"baseline_evaluated_at"`
	CurrentEvaluatedAt  time.Time                                       `json:"current_evaluated_at"`
	ComparisonStatus    VerificationComparisonStatus                    `json:"comparison_status"`
	EnvelopeVersion     VersionVerification                             `json:"envelope_version"`
	ComparisonVersion   VersionVerification                             `json:"comparison_version"`
	Digests             SafetyCaseVerificationComparisonEnvelopeDigests `json:"digests"`
}

// BuildSafetyCaseVerificationComparisonEnvelopeReport independently
// recomputes the reports, comparison, and outer envelope. Secret-like input is
// rejected before any supplied value is copied into the output.
func BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current SafetyCaseVerificationReport, envelope SafetyCaseVerificationComparisonEnvelope) SafetyCaseVerificationComparisonEnvelopeReport {
	report := SafetyCaseVerificationComparisonEnvelopeReport{
		ReportVersion: SafetyCaseVerificationComparisonEnvelopeReportVersion,
		Status:        VerificationRejected,
		Failure:       VerificationComparisonEnvelopeFailureEnvelopeMismatch,
	}
	reject := func(failure VerificationComparisonEnvelopeFailure) SafetyCaseVerificationComparisonEnvelopeReport {
		report.Status = VerificationRejected
		report.Failure = failure
		return report
	}
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) {
		return reject(VerificationComparisonEnvelopeFailureSecretInput)
	}

	report.BaselineEvaluatedAt = baseline.EvaluatedAt
	report.CurrentEvaluatedAt = current.EvaluatedAt
	report.ComparisonStatus = envelope.Comparison.Status
	report.EnvelopeVersion = VersionVerification{
		Expected: SafetyCaseVerificationComparisonEnvelopeVersion,
		Claimed:  envelope.EnvelopeVersion,
		Verified: envelope.EnvelopeVersion == SafetyCaseVerificationComparisonEnvelopeVersion,
	}
	report.ComparisonVersion = VersionVerification{
		Expected: SafetyCaseVerificationComparisonVersion,
		Claimed:  envelope.ComparisonVersion,
		Verified: envelope.ComparisonVersion == SafetyCaseVerificationComparisonVersion,
	}
	report.Digests.BaselineReport.Claimed = envelope.BaselineReportDigest
	report.Digests.CurrentReport.Claimed = envelope.CurrentReportDigest
	report.Digests.Comparison.Claimed = envelope.ComparisonDigest
	report.Digests.Envelope.Claimed = envelope.EnvelopeDigest

	if !report.EnvelopeVersion.Verified {
		return reject(VerificationComparisonEnvelopeFailureEnvelopeVersion)
	}
	if !report.ComparisonVersion.Verified || envelope.Comparison.ComparisonVersion != SafetyCaseVerificationComparisonVersion {
		return reject(VerificationComparisonEnvelopeFailureComparisonVersion)
	}
	if envelope.Comparison.ExecutionAuthority {
		return reject(VerificationComparisonEnvelopeFailureAuthority)
	}
	if !digestPattern.MatchString(envelope.BaselineReportDigest) {
		return reject(VerificationComparisonEnvelopeFailureBaselineMalformed)
	}
	if !digestPattern.MatchString(envelope.CurrentReportDigest) {
		return reject(VerificationComparisonEnvelopeFailureCurrentMalformed)
	}
	if !digestPattern.MatchString(envelope.ComparisonDigest) {
		return reject(VerificationComparisonEnvelopeFailureComparisonMalformed)
	}
	if !digestPattern.MatchString(envelope.EnvelopeDigest) {
		return reject(VerificationComparisonEnvelopeFailureEnvelopeMalformed)
	}

	baselineDigest, err := canonicalVerificationReportDigest(baseline)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	report.Digests.BaselineReport.Recomputed = baselineDigest
	report.Digests.BaselineReport.Expected = baselineDigest
	if envelope.BaselineReportDigest != baselineDigest {
		return reject(VerificationComparisonEnvelopeFailureBaselineMismatch)
	}
	report.Digests.BaselineReport.Verified = true

	currentDigest, err := canonicalVerificationReportDigest(current)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	report.Digests.CurrentReport.Recomputed = currentDigest
	report.Digests.CurrentReport.Expected = currentDigest
	if envelope.CurrentReportDigest != currentDigest {
		return reject(VerificationComparisonEnvelopeFailureCurrentMismatch)
	}
	report.Digests.CurrentReport.Verified = true

	expectedComparison := CompareSafetyCaseVerificationReports(baseline, current)
	recomputedComparisonDigest, err := canonicalVerificationComparisonDigest(envelope.Comparison)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	expectedComparisonDigest, err := canonicalVerificationComparisonDigest(expectedComparison)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	report.Digests.Comparison.Recomputed = recomputedComparisonDigest
	report.Digests.Comparison.Expected = expectedComparisonDigest
	if !reflect.DeepEqual(envelope.Comparison, expectedComparison) {
		return reject(VerificationComparisonEnvelopeFailureComparisonMismatch)
	}
	if envelope.ComparisonDigest != recomputedComparisonDigest || envelope.ComparisonDigest != expectedComparisonDigest {
		return reject(VerificationComparisonEnvelopeFailureComparisonDigest)
	}
	report.Digests.Comparison.Verified = true

	recomputedEnvelopeDigest, err := canonicalVerificationComparisonEnvelopeDigest(
		envelope.EnvelopeVersion,
		envelope.ComparisonVersion,
		envelope.BaselineReportDigest,
		envelope.CurrentReportDigest,
		envelope.ComparisonDigest,
	)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	expectedEnvelopeDigest, err := canonicalVerificationComparisonEnvelopeDigest(
		SafetyCaseVerificationComparisonEnvelopeVersion,
		SafetyCaseVerificationComparisonVersion,
		baselineDigest,
		currentDigest,
		expectedComparisonDigest,
	)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	report.Digests.Envelope.Recomputed = recomputedEnvelopeDigest
	report.Digests.Envelope.Expected = expectedEnvelopeDigest
	if envelope.EnvelopeDigest != recomputedEnvelopeDigest || envelope.EnvelopeDigest != expectedEnvelopeDigest {
		return reject(VerificationComparisonEnvelopeFailureEnvelopeDigest)
	}
	report.Digests.Envelope.Verified = true

	expectedEnvelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		return reject(VerificationComparisonEnvelopeFailureCompilationRejected)
	}
	if !reflect.DeepEqual(envelope, expectedEnvelope) {
		return reject(VerificationComparisonEnvelopeFailureEnvelopeMismatch)
	}
	report.Status = VerificationVerified
	report.Failure = VerificationComparisonEnvelopeFailureNone
	return report
}
