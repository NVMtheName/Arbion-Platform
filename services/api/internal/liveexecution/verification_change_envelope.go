package liveexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

const SafetyCaseVerificationComparisonEnvelopeVersion = "live-safety-verification-comparison-envelope-v1"

const (
	verificationReportDigestDomain     = "arbion:live-safety:verification-report:v1"
	verificationComparisonDigestDomain = "arbion:live-safety:verification-comparison:v1"
	verificationEnvelopeDigestDomain   = "arbion:live-safety:verification-comparison-envelope:v1"
)

var ErrSafetyCaseVerificationComparisonEnvelope = errors.New("live safety verification comparison envelope is invalid")

// SafetyCaseVerificationComparisonEnvelope cryptographically binds the two
// exact input reports and their exact comparison. It is review evidence only
// and cannot grant execution authority.
type SafetyCaseVerificationComparisonEnvelope struct {
	EnvelopeVersion      string                           `json:"envelope_version"`
	ComparisonVersion    string                           `json:"comparison_version"`
	BaselineReportDigest string                           `json:"baseline_report_sha256"`
	CurrentReportDigest  string                           `json:"current_report_sha256"`
	ComparisonDigest     string                           `json:"comparison_sha256"`
	EnvelopeDigest       string                           `json:"envelope_sha256"`
	Comparison           SafetyCaseVerificationComparison `json:"comparison"`
}

// CompileSafetyCaseVerificationComparisonEnvelope is deterministic and
// credential-free. Secret-like input is rejected before hashing or output.
func CompileSafetyCaseVerificationComparisonEnvelope(baseline, current SafetyCaseVerificationReport) (SafetyCaseVerificationComparisonEnvelope, error) {
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(baseline)) ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(current)) {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	baselineDigest, err := canonicalVerificationReportDigest(baseline)
	if err != nil {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	currentDigest, err := canonicalVerificationReportDigest(current)
	if err != nil {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	comparison := CompareSafetyCaseVerificationReports(baseline, current)
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(comparison)) || comparison.ExecutionAuthority {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	comparisonDigest, err := canonicalVerificationComparisonDigest(comparison)
	if err != nil {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	envelopeDigest, err := canonicalVerificationComparisonEnvelopeDigest(
		SafetyCaseVerificationComparisonEnvelopeVersion,
		SafetyCaseVerificationComparisonVersion,
		baselineDigest,
		currentDigest,
		comparisonDigest,
	)
	if err != nil {
		return SafetyCaseVerificationComparisonEnvelope{}, ErrSafetyCaseVerificationComparisonEnvelope
	}
	return SafetyCaseVerificationComparisonEnvelope{
		EnvelopeVersion:      SafetyCaseVerificationComparisonEnvelopeVersion,
		ComparisonVersion:    SafetyCaseVerificationComparisonVersion,
		BaselineReportDigest: baselineDigest,
		CurrentReportDigest:  currentDigest,
		ComparisonDigest:     comparisonDigest,
		EnvelopeDigest:       envelopeDigest,
		Comparison:           comparison,
	}, nil
}

// VerifySafetyCaseVerificationComparisonEnvelope recomputes every input and
// output layer. Any changed fact, order, time, version, status, failure,
// evidence item, or digest fails closed.
func VerifySafetyCaseVerificationComparisonEnvelope(baseline, current SafetyCaseVerificationReport, envelope SafetyCaseVerificationComparisonEnvelope) error {
	if envelope.EnvelopeVersion != SafetyCaseVerificationComparisonEnvelopeVersion ||
		envelope.ComparisonVersion != SafetyCaseVerificationComparisonVersion ||
		!digestPattern.MatchString(envelope.BaselineReportDigest) ||
		!digestPattern.MatchString(envelope.CurrentReportDigest) ||
		!digestPattern.MatchString(envelope.ComparisonDigest) ||
		!digestPattern.MatchString(envelope.EnvelopeDigest) ||
		envelope.Comparison.ExecutionAuthority ||
		safetyCaseHasSecretLikeValue(reflect.ValueOf(envelope)) {
		return ErrSafetyCaseVerificationComparisonEnvelope
	}
	expected, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil || !reflect.DeepEqual(expected, envelope) {
		return ErrSafetyCaseVerificationComparisonEnvelope
	}
	return nil
}

func canonicalVerificationReportDigest(report SafetyCaseVerificationReport) (string, error) {
	payload, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(verificationReportDigestDomain, payload), nil
}

func canonicalVerificationComparisonDigest(comparison SafetyCaseVerificationComparison) (string, error) {
	payload, err := json.Marshal(comparison)
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(verificationComparisonDigestDomain, payload), nil
}

func canonicalVerificationComparisonEnvelopeDigest(envelopeVersion, comparisonVersion, baselineDigest, currentDigest, comparisonDigest string) (string, error) {
	payload, err := json.Marshal(struct {
		EnvelopeVersion      string `json:"envelope_version"`
		ComparisonVersion    string `json:"comparison_version"`
		BaselineReportDigest string `json:"baseline_report_sha256"`
		CurrentReportDigest  string `json:"current_report_sha256"`
		ComparisonDigest     string `json:"comparison_sha256"`
	}{
		EnvelopeVersion: envelopeVersion, ComparisonVersion: comparisonVersion,
		BaselineReportDigest: baselineDigest, CurrentReportDigest: currentDigest,
		ComparisonDigest: comparisonDigest,
	})
	if err != nil {
		return "", err
	}
	return verificationComparisonSHA256(verificationEnvelopeDigestDomain, payload), nil
}

func verificationComparisonSHA256(domain string, payload []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(domain))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(payload)
	return hex.EncodeToString(digest.Sum(nil))
}
