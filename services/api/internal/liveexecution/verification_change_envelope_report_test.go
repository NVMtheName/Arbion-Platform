package liveexecution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVerificationComparisonEnvelopeReportVerifiesEveryLayer(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 0, 0, 123, time.UTC)
	input := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(input, input)
	if err != nil {
		t.Fatal(err)
	}
	first := BuildSafetyCaseVerificationComparisonEnvelopeReport(input, input, envelope)
	second := BuildSafetyCaseVerificationComparisonEnvelopeReport(input, input, envelope)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("verification report replay changed: %#v %#v", first, second)
	}
	if first.ReportVersion != SafetyCaseVerificationComparisonEnvelopeReportVersion ||
		first.Status != VerificationVerified ||
		first.Failure != VerificationComparisonEnvelopeFailureNone ||
		first.ExecutionAuthority ||
		first.ComparisonStatus != VerificationComparisonSame ||
		!first.BaselineEvaluatedAt.Equal(now) || !first.CurrentEvaluatedAt.Equal(now) ||
		!first.EnvelopeVersion.Verified || !first.ComparisonVersion.Verified ||
		!exactVerifiedDigest(first.Digests.BaselineReport) ||
		!exactVerifiedDigest(first.Digests.CurrentReport) ||
		!exactVerifiedDigest(first.Digests.Comparison) ||
		!exactVerifiedDigest(first.Digests.Envelope) {
		t.Fatalf("verification report is incomplete or authoritative: %#v", first)
	}
	if first.Digests.BaselineReport.Claimed != envelope.BaselineReportDigest ||
		first.Digests.CurrentReport.Claimed != envelope.CurrentReportDigest ||
		first.Digests.Comparison.Claimed != envelope.ComparisonDigest ||
		first.Digests.Envelope.Claimed != envelope.EnvelopeDigest {
		t.Fatalf("verification report lost an exact digest: %#v", first.Digests)
	}
}

func TestVerificationComparisonEnvelopeReportPreservesComparisonSemantics(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 15, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.Action.ID = "11111111-1111-4111-8111-111111111112"
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	changedEnvelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	changed := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, changedEnvelope)
	if changed.Status != VerificationVerified || changed.ComparisonStatus != VerificationComparisonChanged || changed.ExecutionAuthority {
		t.Fatalf("changed comparison envelope was not independently verified: %#v", changed)
	}

	rejectedInput := cloneSafetyCaseReport(t, current)
	rejectedInput.Status = VerificationRejected
	rejectedEnvelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, rejectedInput)
	if err != nil {
		t.Fatal(err)
	}
	rejected := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, rejectedInput, rejectedEnvelope)
	if rejected.Status != VerificationVerified || rejected.ComparisonStatus != VerificationComparisonRejected || rejected.ExecutionAuthority {
		t.Fatalf("exact rejected comparison was not independently verified: %#v", rejected)
	}
}

func TestVerificationComparisonEnvelopeReportRejectsSecretWithoutEcho(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, baseline)
	if err != nil {
		t.Fatal(err)
	}
	secret := cloneSafetyCaseReport(t, baseline)
	secret.Identity.ProviderName = "client_secret_do_not_copy"
	report := BuildSafetyCaseVerificationComparisonEnvelopeReport(secret, baseline, envelope)
	if report.Status != VerificationRejected || report.Failure != VerificationComparisonEnvelopeFailureSecretInput ||
		report.ExecutionAuthority || report.BaselineEvaluatedAt != (time.Time{}) ||
		report.CurrentEvaluatedAt != (time.Time{}) || report.EnvelopeVersion.Claimed != "" ||
		report.Digests.BaselineReport.Claimed != "" {
		t.Fatalf("secret-like input produced attributable output: %#v", report)
	}
	payload, marshalErr := json.Marshal(report)
	if marshalErr != nil || strings.Contains(string(payload), "client_secret_do_not_copy") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, marshalErr)
	}
}

func TestVerificationComparisonEnvelopeReportClassifiesTampering(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	original, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		failure VerificationComparisonEnvelopeFailure
		mutate  func(*SafetyCaseVerificationComparisonEnvelope)
	}{
		{"envelope version", VerificationComparisonEnvelopeFailureEnvelopeVersion, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.EnvelopeVersion = "comparison-envelope-v2"
		}},
		{"comparison version", VerificationComparisonEnvelopeFailureComparisonVersion, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.ComparisonVersion = "comparison-v2"
		}},
		{"nested comparison version", VerificationComparisonEnvelopeFailureComparisonVersion, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.ComparisonVersion = "comparison-v2"
		}},
		{"execution authority", VerificationComparisonEnvelopeFailureAuthority, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.ExecutionAuthority = true
		}},
		{"baseline malformed", VerificationComparisonEnvelopeFailureBaselineMalformed, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.BaselineReportDigest = "not-a-digest"
		}},
		{"current malformed", VerificationComparisonEnvelopeFailureCurrentMalformed, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.CurrentReportDigest = "not-a-digest"
		}},
		{"comparison malformed", VerificationComparisonEnvelopeFailureComparisonMalformed, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.ComparisonDigest = "not-a-digest"
		}},
		{"envelope malformed", VerificationComparisonEnvelopeFailureEnvelopeMalformed, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.EnvelopeDigest = "not-a-digest"
		}},
		{"baseline mismatch", VerificationComparisonEnvelopeFailureBaselineMismatch, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.BaselineReportDigest = digestB
		}},
		{"current mismatch", VerificationComparisonEnvelopeFailureCurrentMismatch, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.CurrentReportDigest = digestB
		}},
		{"comparison mismatch", VerificationComparisonEnvelopeFailureComparisonMismatch, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Status = VerificationComparisonChanged
		}},
		{"comparison digest", VerificationComparisonEnvelopeFailureComparisonDigest, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.ComparisonDigest = digestB
		}},
		{"envelope digest", VerificationComparisonEnvelopeFailureEnvelopeDigest, func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.EnvelopeDigest = digestB
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneVerificationComparisonEnvelope(t, original)
			test.mutate(&mutated)
			report := BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, current, mutated)
			if report.Status != VerificationRejected || report.Failure != test.failure || report.ExecutionAuthority {
				t.Fatalf("tamper was not classified exactly: %#v", report)
			}
		})
	}
}

func TestVerificationComparisonEnvelopeReportBindsBothInputReports(t *testing.T) {
	now := time.Date(2026, 9, 8, 15, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	changedBaseline := cloneSafetyCaseReport(t, baseline)
	changedBaseline.Identity.ActionID = "11111111-1111-4111-8111-111111111112"
	report := BuildSafetyCaseVerificationComparisonEnvelopeReport(changedBaseline, current, envelope)
	if report.Failure != VerificationComparisonEnvelopeFailureBaselineMismatch {
		t.Fatalf("changed baseline was not rejected: %#v", report)
	}
	changedCurrent := cloneSafetyCaseReport(t, current)
	changedCurrent.EvaluatedAt = changedCurrent.EvaluatedAt.Add(time.Nanosecond)
	report = BuildSafetyCaseVerificationComparisonEnvelopeReport(baseline, changedCurrent, envelope)
	if report.Failure != VerificationComparisonEnvelopeFailureCurrentMismatch {
		t.Fatalf("changed current report was not rejected: %#v", report)
	}
}

func TestVerificationComparisonEnvelopeReportHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("comparison envelope report test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_change_envelope_report.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("comparison envelope report unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}
