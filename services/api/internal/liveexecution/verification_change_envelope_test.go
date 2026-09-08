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

func TestVerificationComparisonEnvelopeIsDeterministicAndFixed(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 0, 0, 123, time.UTC)
	report := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	first, err := CompileSafetyCaseVerificationComparisonEnvelope(report, report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileSafetyCaseVerificationComparisonEnvelope(report, report)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("comparison envelope replay changed: %#v %#v %v", first, second, err)
	}
	if first.EnvelopeVersion != SafetyCaseVerificationComparisonEnvelopeVersion ||
		first.ComparisonVersion != SafetyCaseVerificationComparisonVersion ||
		first.Comparison.Status != VerificationComparisonSame || first.Comparison.ExecutionAuthority ||
		!digestPattern.MatchString(first.BaselineReportDigest) ||
		first.BaselineReportDigest != first.CurrentReportDigest ||
		!digestPattern.MatchString(first.ComparisonDigest) ||
		!digestPattern.MatchString(first.EnvelopeDigest) {
		t.Fatalf("comparison envelope is incomplete or authoritative: %#v", first)
	}
	if first.BaselineReportDigest != "ac5ce740c9b81bd7df778fa898ca537c9392f72da0509d3deb18f25411a35e59" ||
		first.ComparisonDigest != "efdc1bbbab42833204ee725c4824aaee49631bfbe5b96c403b897188d0da11ad" ||
		first.EnvelopeDigest != "027ec0a81fd73762600d992c81ecea3ddcad1bc754fe764d3c89ac8e90eb51c7" {
		t.Fatalf("fixed comparison envelope changed: baseline=%s comparison=%s envelope=%s", first.BaselineReportDigest, first.ComparisonDigest, first.EnvelopeDigest)
	}
	if err = VerifySafetyCaseVerificationComparisonEnvelope(report, report, first); err != nil {
		t.Fatalf("exact comparison envelope did not verify: %v", err)
	}
}

func TestVerificationComparisonEnvelopePreservesChangedAndRejectedSemantics(t *testing.T) {
	baselineTime := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	currentTime := baselineTime.Add(time.Minute)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(baselineTime), baselineTime)
	currentCase := validSafetyCase(currentTime)
	currentCase.Action.ID = "11111111-1111-4111-8111-111111111112"
	current := verifiedSafetyCaseReport(t, currentCase, currentTime)
	changed, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil || changed.Comparison.Status != VerificationComparisonChanged ||
		changed.Comparison.ChangeCount == 0 || changed.Comparison.ExecutionAuthority {
		t.Fatalf("changed semantics were not preserved: %#v %v", changed, err)
	}

	rejectedInput := cloneSafetyCaseReport(t, current)
	rejectedInput.Status = VerificationRejected
	rejected, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, rejectedInput)
	if err != nil || rejected.Comparison.Status != VerificationComparisonRejected ||
		rejected.Comparison.FailureInput != VerificationComparisonInputCurrent ||
		rejected.Comparison.Failure != VerificationComparisonFailureStatus ||
		rejected.Comparison.ExecutionAuthority {
		t.Fatalf("rejected semantics were not preserved: %#v %v", rejected, err)
	}
	if err = VerifySafetyCaseVerificationComparisonEnvelope(baseline, rejectedInput, rejected); err != nil {
		t.Fatalf("exact rejected comparison envelope did not verify: %v", err)
	}
}

func TestVerificationComparisonEnvelopeRejectsSecretLikeInputWithoutOutput(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	current.Identity.ProviderName = "access_token_do_not_hash_or_echo"
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if !errors.Is(err, ErrSafetyCaseVerificationComparisonEnvelope) ||
		!reflect.DeepEqual(envelope, SafetyCaseVerificationComparisonEnvelope{}) {
		t.Fatalf("secret-like input produced an envelope: %#v %v", envelope, err)
	}
	payload, marshalErr := json.Marshal(envelope)
	if marshalErr != nil || strings.Contains(string(payload), "access_token_do_not_hash_or_echo") {
		t.Fatalf("secret-like input was echoed: %s %v", payload, marshalErr)
	}
}

func TestVerificationComparisonEnvelopeRejectsEveryEnvelopeTamper(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	report := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	original, err := CompileSafetyCaseVerificationComparisonEnvelope(report, report)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*SafetyCaseVerificationComparisonEnvelope)
	}{
		{"envelope version", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.EnvelopeVersion = "comparison-envelope-v2"
		}},
		{"comparison version", func(value *SafetyCaseVerificationComparisonEnvelope) { value.ComparisonVersion = "comparison-v2" }},
		{"baseline digest", func(value *SafetyCaseVerificationComparisonEnvelope) { value.BaselineReportDigest = digestB }},
		{"current digest", func(value *SafetyCaseVerificationComparisonEnvelope) { value.CurrentReportDigest = digestB }},
		{"comparison digest", func(value *SafetyCaseVerificationComparisonEnvelope) { value.ComparisonDigest = digestB }},
		{"envelope digest", func(value *SafetyCaseVerificationComparisonEnvelope) { value.EnvelopeDigest = digestB }},
		{"comparison status", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Status = VerificationComparisonChanged
		}},
		{"comparison failure", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Failure = VerificationComparisonFailureDigest
		}},
		{"execution authority", func(value *SafetyCaseVerificationComparisonEnvelope) { value.Comparison.ExecutionAuthority = true }},
		{"evaluation time", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.CurrentEvaluatedAt = value.Comparison.CurrentEvaluatedAt.Add(time.Nanosecond)
		}},
		{"version order", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Versions[0], value.Comparison.Versions[1] = value.Comparison.Versions[1], value.Comparison.Versions[0]
		}},
		{"identity fact", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Identities[0].After = accountID
		}},
		{"digest fact", func(value *SafetyCaseVerificationComparisonEnvelope) { value.Comparison.Digests[0].After = digestB }},
		{"assessment fact", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Assessment.After.Availability = Unavailable
		}},
		{"evidence fact", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Evidence[0].After.Digest = digestA
		}},
		{"evidence order", func(value *SafetyCaseVerificationComparisonEnvelope) {
			value.Comparison.Evidence[0], value.Comparison.Evidence[1] = value.Comparison.Evidence[1], value.Comparison.Evidence[0]
		}},
		{"change count", func(value *SafetyCaseVerificationComparisonEnvelope) { value.Comparison.ChangeCount++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneVerificationComparisonEnvelope(t, original)
			test.mutate(&mutated)
			if !errors.Is(VerifySafetyCaseVerificationComparisonEnvelope(report, report, mutated), ErrSafetyCaseVerificationComparisonEnvelope) {
				t.Fatalf("tampered comparison envelope verified: %#v", mutated)
			}
		})
	}
}

func TestVerificationComparisonEnvelopeBindsEveryInputReportFactAndOrder(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 0, 0, 0, time.UTC)
	baseline := verifiedSafetyCaseReport(t, validSafetyCase(now), now)
	current := cloneSafetyCaseReport(t, baseline)
	envelope, err := CompileSafetyCaseVerificationComparisonEnvelope(baseline, current)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*SafetyCaseVerificationReport)
	}{
		{"identity", func(value *SafetyCaseVerificationReport) {
			value.Identity.ActionID = "11111111-1111-4111-8111-111111111112"
		}},
		{"digest", func(value *SafetyCaseVerificationReport) { value.Digests.SafetyCase.Claimed = digestB }},
		{"time", func(value *SafetyCaseVerificationReport) { value.EvaluatedAt = value.EvaluatedAt.Add(-time.Nanosecond) }},
		{"evidence fact", func(value *SafetyCaseVerificationReport) { value.Evidence[0].Digest = digestA }},
		{"evidence order", func(value *SafetyCaseVerificationReport) {
			value.Evidence[0], value.Evidence[1] = value.Evidence[1], value.Evidence[0]
		}},
		{"reason", func(value *SafetyCaseVerificationReport) { value.Assessment.ReasonCodes[0] = ReasonInvalidEvidence }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneSafetyCaseReport(t, current)
			mutation.mutate(&changed)
			if !errors.Is(VerifySafetyCaseVerificationComparisonEnvelope(baseline, changed, envelope), ErrSafetyCaseVerificationComparisonEnvelope) {
				t.Fatalf("altered input report verified: %#v", changed)
			}
		})
	}
}

func TestVerificationComparisonEnvelopeHasNoPersistenceOrNetworkSurface(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("comparison envelope test path unavailable")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "verification_change_envelope.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{
		"context.Context", "NewRegistryStore(", ".Record(", ".Query(", ".Exec(", ".Begin(",
		"net/http", "pgx", "ProviderClient", "BrokerClient", "SubmitOrder", "CancelOrder",
	} {
		if strings.Contains(string(source), prohibited) {
			t.Fatalf("comparison envelope unexpectedly contains persistence, provider, or broker surface %q", prohibited)
		}
	}
}

func cloneVerificationComparisonEnvelope(t *testing.T, value SafetyCaseVerificationComparisonEnvelope) SafetyCaseVerificationComparisonEnvelope {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned SafetyCaseVerificationComparisonEnvelope
	if err = json.Unmarshal(payload, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
