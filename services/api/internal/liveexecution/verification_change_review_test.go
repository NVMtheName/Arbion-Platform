package liveexecution

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVerificationComparisonReviewArtifactIsDeterministicAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 0, 0, 123, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(input, input)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(input, input, envelope)
	first := BuildSafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit)
	second := BuildSafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("review artifact replay changed: %#v %#v", first, second)
	}
	if first.ArtifactVersion != SafetyCaseVerificationComparisonReviewArtifactVersion ||
		first.Status != VerificationComparisonReviewReady ||
		first.Failure != VerificationComparisonReviewFailureNone || first.ExecutionAuthority ||
		first.Disposition != VerificationComparisonReviewNoChange ||
		first.OwnerAction != VerificationComparisonOwnerActionNone ||
		first.ComparisonStatus != VerificationComparisonSame || first.ChangeCount != 0 ||
		len(first.Identities) != 12 ||
		!digestPattern.MatchString(first.AuditReportDigest) || !digestPattern.MatchString(first.ReviewDigest) {
		t.Fatalf("review artifact is incomplete or authoritative: %#v", first)
	}
	if first.AuditReportDigest != "80528858b3380b6ad053c597393e3751a701a95865685761a14edf19343a3a56" ||
		first.ReviewDigest != "4e63fdf21219c54c6d5b9cfe519574cebbf37c3d91a46ac4e49d1dbd9fd5012c" {
		t.Fatalf("fixed review artifact changed: audit=%s review=%s", first.AuditReportDigest, first.ReviewDigest)
	}
	if err = VerifySafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit, first); err != nil {
		t.Fatalf("exact review artifact did not verify: %v", err)
	}
}

func TestVerificationComparisonReviewArtifactPreservesChangedAndRejectedSemantics(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.Action.ID = "11111111-1111-4111-8111-111111111112"
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, envelope)
	changed := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit)
	if changed.Status != VerificationComparisonReviewReady ||
		changed.Disposition != VerificationComparisonReviewChanged ||
		changed.OwnerAction != VerificationComparisonOwnerActionReviewChanges ||
		changed.ComparisonStatus != VerificationComparisonChanged || changed.ChangeCount == 0 ||
		changed.ExecutionAuthority {
		t.Fatalf("changed comparison was not preserved: %#v", changed)
	}

	rejectedInput := cloneSafetyCaseReport(t, current)
	rejectedInput.Status = VerificationRejected
	rejectedEnvelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, rejectedInput)
	if err != nil {
		t.Fatal(err)
	}
	rejectedAudit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, rejectedInput, rejectedEnvelope)
	rejected := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, rejectedInput, rejectedEnvelope, rejectedAudit)
	if rejected.Status != VerificationComparisonReviewReady ||
		rejected.Disposition != VerificationComparisonReviewInputRejected ||
		rejected.OwnerAction != VerificationComparisonOwnerActionReviewRejection ||
		rejected.ComparisonStatus != VerificationComparisonRejected ||
		rejected.ComparisonFailureInput != VerificationComparisonInputCurrent ||
		rejected.ComparisonFailure != VerificationComparisonFailureStatus ||
		len(rejected.Identities) != 0 || rejected.ExecutionAuthority {
		t.Fatalf("rejected comparison was not preserved: %#v", rejected)
	}
}

func TestVerificationComparisonReviewArtifactRejectsSecretWithoutEcho(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(input, input)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(input, input, envelope)
	audit.ReportVersion = "authorization_header_do_not_copy"
	artifact := BuildSafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit)
	if artifact.Status != VerificationComparisonReviewRejected ||
		artifact.Failure != VerificationComparisonReviewFailureSecretInput ||
		artifact.ExecutionAuthority || artifact.AuditReportVersion != "" ||
		artifact.AuditReportDigest != "" || artifact.ReviewDigest != "" || len(artifact.Identities) != 0 {
		t.Fatalf("secret-like input produced attributable output: %#v", artifact)
	}
	payload, marshalErr := json.Marshal(artifact)
	if marshalErr != nil || strings.Contains(string(payload), "authorization_header_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, marshalErr)
	}
}

func TestVerificationComparisonReviewArtifactClassifiesInvalidEvidence(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	originalEnvelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	originalAudit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, originalEnvelope)
	tests := []struct {
		name    string
		failure VerificationComparisonReviewFailure
		mutate  func(*SafetyCaseVerificationComparisonEnvelope, *SafetyCaseVerificationComparisonEnvelopeReport)
	}{
		{"report version", VerificationComparisonReviewFailureReportVersion, func(_ *SafetyCaseVerificationComparisonEnvelope, audit *SafetyCaseVerificationComparisonEnvelopeReport) {
			audit.ReportVersion = "audit-v2"
		}},
		{"report status", VerificationComparisonReviewFailureReportStatus, func(_ *SafetyCaseVerificationComparisonEnvelope, audit *SafetyCaseVerificationComparisonEnvelopeReport) {
			audit.Status = VerificationRejected
		}},
		{"authority", VerificationComparisonReviewFailureAuthority, func(envelope *SafetyCaseVerificationComparisonEnvelope, _ *SafetyCaseVerificationComparisonEnvelopeReport) {
			envelope.Comparison.ExecutionAuthority = true
		}},
		{"comparison status", VerificationComparisonReviewFailureComparisonStatus, func(envelope *SafetyCaseVerificationComparisonEnvelope, _ *SafetyCaseVerificationComparisonEnvelopeReport) {
			envelope.Comparison.Status = VerificationComparisonStatus("UNKNOWN")
		}},
		{"identity shape", VerificationComparisonReviewFailureIdentity, func(envelope *SafetyCaseVerificationComparisonEnvelope, _ *SafetyCaseVerificationComparisonEnvelopeReport) {
			envelope.Comparison.Identities[0].Field = "unexpected_identity"
		}},
		{"digest evidence", VerificationComparisonReviewFailureDigest, func(_ *SafetyCaseVerificationComparisonEnvelope, audit *SafetyCaseVerificationComparisonEnvelopeReport) {
			audit.Digests.BaselineReport.Verified = false
		}},
		{"report mismatch", VerificationComparisonReviewFailureReportMismatch, func(_ *SafetyCaseVerificationComparisonEnvelope, audit *SafetyCaseVerificationComparisonEnvelopeReport) {
			audit.CurrentEvaluatedAt = audit.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := cloneVerificationComparisonEnvelope(t, originalEnvelope)
			audit := cloneVerificationComparisonEnvelopeReport(t, originalAudit)
			test.mutate(&envelope, &audit)
			artifact := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit)
			if artifact.Status != VerificationComparisonReviewRejected || artifact.Failure != test.failure ||
				artifact.ExecutionAuthority || artifact.ReviewDigest != "" {
				t.Fatalf("invalid evidence was not classified exactly: %#v", artifact)
			}
		})
	}
}

func TestVerificationComparisonReviewArtifactRejectsEveryOutputTamper(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(input, input)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(input, input, envelope)
	original := BuildSafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit)
	mutations := []struct {
		name   string
		mutate func(*SafetyCaseVerificationComparisonReviewArtifact)
	}{
		{"artifact version", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.ArtifactVersion = "review-v2" }},
		{"status", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.Status = VerificationComparisonReviewRejected
		}},
		{"failure", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.Failure = VerificationComparisonReviewFailureDigest
		}},
		{"authority", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.ExecutionAuthority = true }},
		{"disposition", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.Disposition = VerificationComparisonReviewChanged
		}},
		{"owner action", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.OwnerAction = VerificationComparisonOwnerActionReviewChanges
		}},
		{"report version", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.AuditReportVersion = "audit-v2" }},
		{"envelope version", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.EnvelopeVersion = "envelope-v2" }},
		{"comparison version", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.ComparisonVersion = "comparison-v2" }},
		{"baseline time", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.BaselineEvaluatedAt = value.BaselineEvaluatedAt.Add(time.Nanosecond)
		}},
		{"current time", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.CurrentEvaluatedAt = value.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"comparison status", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.ComparisonStatus = VerificationComparisonChanged
		}},
		{"comparison failure input", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.ComparisonFailureInput = VerificationComparisonInputCurrent
		}},
		{"comparison failure", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.ComparisonFailure = VerificationComparisonFailureDigest
		}},
		{"change count", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.ChangeCount++ }},
		{"identity fact", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.Identities[0].After = accountID }},
		{"identity order", func(value *SafetyCaseVerificationComparisonReviewArtifact) {
			value.Identities[0], value.Identities[1] = value.Identities[1], value.Identities[0]
		}},
		{"digest evidence", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.Digests.Envelope.Verified = false }},
		{"audit digest", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.AuditReportDigest = digestB }},
		{"review digest", func(value *SafetyCaseVerificationComparisonReviewArtifact) { value.ReviewDigest = digestB }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			artifact := cloneVerificationComparisonReviewArtifact(t, original)
			mutation.mutate(&artifact)
			if !errors.Is(VerifySafetyCaseVerificationComparisonReviewArtifact(input, input, envelope, audit, artifact), ErrSafetyCaseVerificationComparisonReviewArtifact) {
				t.Fatalf("tampered review artifact verified: %#v", artifact)
			}
		})
	}
}

func TestVerificationComparisonReviewArtifactBindsEveryInput(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	audit := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, envelope)
	artifact := BuildSafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, audit)
	changed := cloneSafetyCaseReport(t, current)
	changed.Identity.ActionID = "11111111-1111-4111-8111-111111111112"
	if !errors.Is(VerifySafetyCaseVerificationComparisonReviewArtifact(baseline, changed, envelope, audit, artifact), ErrSafetyCaseVerificationComparisonReviewArtifact) {
		t.Fatal("changed current report verified against the prior review artifact")
	}
	changedEnvelope := cloneVerificationComparisonEnvelope(t, envelope)
	changedEnvelope.EnvelopeDigest = digestB
	if !errors.Is(VerifySafetyCaseVerificationComparisonReviewArtifact(baseline, current, changedEnvelope, audit, artifact), ErrSafetyCaseVerificationComparisonReviewArtifact) {
		t.Fatal("changed comparison envelope verified against the prior review artifact")
	}
	changedAudit := cloneVerificationComparisonEnvelopeReport(t, audit)
	changedAudit.CurrentEvaluatedAt = changedAudit.CurrentEvaluatedAt.Add(time.Nanosecond)
	if !errors.Is(VerifySafetyCaseVerificationComparisonReviewArtifact(baseline, current, envelope, changedAudit, artifact), ErrSafetyCaseVerificationComparisonReviewArtifact) {
		t.Fatal("changed audit report verified against the prior review artifact")
	}
}

func TestVerificationComparisonReviewArtifactHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("comparison review test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_change_review.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("comparison review unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}

func cloneVerificationComparisonEnvelopeReport(t *testing.T, value SafetyCaseVerificationComparisonEnvelopeReport) SafetyCaseVerificationComparisonEnvelopeReport {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyCaseVerificationComparisonEnvelopeReport
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func cloneVerificationComparisonReviewArtifact(t *testing.T, value SafetyCaseVerificationComparisonReviewArtifact) SafetyCaseVerificationComparisonReviewArtifact {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyCaseVerificationComparisonReviewArtifact
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
