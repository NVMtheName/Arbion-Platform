package liveexecution

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	testOwnerID      = "11111111-1111-4111-8111-111111111111"
	testAccountID    = "22222222-2222-4222-8222-222222222222"
	testConnectionID = "33333333-3333-4333-8333-333333333333"
	testInstanceID   = "44444444-4444-4444-8444-444444444444"
	testMandateID    = "55555555-5555-4555-8555-555555555555"
	testBucketID     = "66666666-6666-4666-8666-666666666666"
	testReservation  = "77777777-7777-4777-8777-777777777777"
	testEvidenceOne  = "88888888-8888-4888-8888-888888888888"
	testEvidenceTwo  = "99999999-9999-4999-8999-999999999999"
	testDigest       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestPrepareRegistryRecordIsCanonicalAndNonExecutable(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 123, time.UTC)
	input := validRegistryInput(now)
	input.Assessment.ReasonCodes = []ReasonCode{ReasonLiveRuntimeUnimplemented, ReasonOwnerAuthorizationUnavailable}
	input.Evidence.Items = []EvidenceItem{input.Evidence.Items[1], input.Evidence.Items[0]}

	first, _, _, err := prepareRegistryRecord(testOwnerID, input, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	input.Assessment.ReasonCodes[0], input.Assessment.ReasonCodes[1] = input.Assessment.ReasonCodes[1], input.Assessment.ReasonCodes[0]
	input.Evidence.Items[0], input.Evidence.Items[1] = input.Evidence.Items[1], input.Evidence.Items[0]
	second, _, _, err := prepareRegistryRecord(testOwnerID, input, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentDigest != second.ContentDigest || first.EvidenceDigest != second.EvidenceDigest {
		t.Fatalf("semantic replay was not canonical: %#v %#v", first, second)
	}
	if first.Assessment.Availability != BlockedUnimplemented || !first.Assessment.Structural {
		t.Fatalf("registry accepted an executable assessment: %#v", first.Assessment)
	}
	if first.ContractVersion != ContractVersion || len(first.ContentDigest) != 64 || len(first.EvidenceDigest) != 64 {
		t.Fatalf("missing contract or deterministic fingerprints: %#v", first)
	}
}

func TestPrepareRegistryRecordFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	tests := map[string]func(*RegistryInput){
		"ready state":            func(input *RegistryInput) { input.Assessment.Availability = "READY" },
		"not structural":         func(input *RegistryInput) { input.Assessment.Structural = false },
		"runtime reason missing": func(input *RegistryInput) { input.Assessment.ReasonCodes = []ReasonCode{ReasonStaleEvidence} },
		"duplicate reason": func(input *RegistryInput) {
			input.Assessment.ReasonCodes = []ReasonCode{ReasonLiveRuntimeUnimplemented, ReasonLiveRuntimeUnimplemented}
		},
		"unsupported reason":   func(input *RegistryInput) { input.Assessment.ReasonCodes = []ReasonCode{"SUBMITTABLE"} },
		"future assessment":    func(input *RegistryInput) { input.ObservedAt = now.Add(time.Second) },
		"future evidence":      func(input *RegistryInput) { input.Evidence.Items[0].ObservedAt = now.Add(time.Second) },
		"duplicate evidence":   func(input *RegistryInput) { input.Evidence.Items[1].ID = input.Evidence.Items[0].ID },
		"secret-like evidence": func(input *RegistryInput) { input.Evidence.Items[0].Kind = "API_KEY_PROOF" },
		"malformed digest":     func(input *RegistryInput) { input.ActionDigest = "not-a-digest" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validRegistryInput(now)
			mutate(&input)
			_, _, _, err := prepareRegistryRecord(testOwnerID, input, now)
			if !errors.Is(err, ErrRegistryInvalid) {
				t.Fatalf("expected fail-closed validation, got %v", err)
			}
		})
	}
}

func TestUnavailableAssessmentRequiresEvidenceReason(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	input := validRegistryInput(now)
	input.Assessment.Availability = Unavailable
	input.Assessment.ReasonCodes = []ReasonCode{ReasonProviderCapabilityUnavailable}
	record, _, _, err := prepareRegistryRecord(testOwnerID, input, now)
	if err != nil || record.Assessment.Availability != Unavailable {
		t.Fatalf("valid unavailable assessment rejected: %#v %v", record, err)
	}
	input.Assessment.ReasonCodes = []ReasonCode{ReasonLiveRuntimeUnimplemented}
	if _, _, _, err = prepareRegistryRecord(testOwnerID, input, now); !errors.Is(err, ErrRegistryInvalid) {
		t.Fatalf("runtime-only unavailable assessment should fail, got %v", err)
	}
}

func TestStoredRegistryRecordFailsClosedOnFingerprintMismatch(t *testing.T) {
	now := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	prepared, _, _, err := prepareRegistryRecord(testOwnerID, validRegistryInput(now), now)
	if err != nil {
		t.Fatal(err)
	}
	prepared.ID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	prepared.CreatedAt = now
	prepared.ChainSequence = 1
	prepared.PreviousChainDigest = GenesisChainDigest
	prepared.ChainDigest = computeChainDigest(prepared.UserID, prepared.StrategyInstanceID, prepared.ChainSequence, prepared.PreviousChainDigest, prepared.ContentDigest)
	if err = validateStoredRegistryRecord(prepared, now); err != nil {
		t.Fatalf("valid stored fingerprint rejected: %v", err)
	}
	prepared.ContentDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err = validateStoredRegistryRecord(prepared, now); !errors.Is(err, ErrRegistryIntegrity) {
		t.Fatalf("fingerprint mismatch did not fail closed: %v", err)
	}
}

func TestChainDigestIsDomainSeparatedAndBindsEveryLink(t *testing.T) {
	first := computeChainDigest(testOwnerID, testInstanceID, 1, GenesisChainDigest, testDigest)
	if len(first) != 64 || first == GenesisChainDigest || first == testDigest {
		t.Fatalf("invalid genesis link: %q", first)
	}
	tests := []struct {
		name     string
		owner    string
		instance string
		sequence int64
		previous string
		content  string
	}{
		{"owner", testAccountID, testInstanceID, 1, GenesisChainDigest, testDigest},
		{"instance", testOwnerID, testAccountID, 1, GenesisChainDigest, testDigest},
		{"sequence", testOwnerID, testInstanceID, 2, GenesisChainDigest, testDigest},
		{"previous", testOwnerID, testInstanceID, 1, strings.Repeat("b", 64), testDigest},
		{"content", testOwnerID, testInstanceID, 1, GenesisChainDigest, strings.Repeat("b", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := computeChainDigest(test.owner, test.instance, test.sequence, test.previous, test.content); got == first {
				t.Fatalf("chain digest did not bind %s", test.name)
			}
		})
	}
}

func validRegistryInput(observedAt time.Time) RegistryInput {
	return RegistryInput{
		AssessmentKey:        "future-live-case-0001",
		FinancialAccountID:   testAccountID,
		ProviderConnectionID: testConnectionID,
		ProviderName:         "coinbase",
		StrategyInstanceID:   testInstanceID,
		MandateID:            testMandateID,
		MandateVersion:       1,
		CapitalBucketID:      testBucketID,
		CapitalReservationID: testReservation,
		ActionDigest:         testDigest,
		Assessment: Assessment{
			Availability: BlockedUnimplemented,
			ReasonCodes:  []ReasonCode{ReasonLiveRuntimeUnimplemented},
			Structural:   true,
		},
		Evidence: EvidenceManifest{Items: []EvidenceItem{
			{ID: testEvidenceOne, Kind: "DESIGN_CONTRACT", Digest: testDigest, Source: EvidenceDesignContract, ObservedAt: observedAt},
			{ID: testEvidenceTwo, Kind: "RISK_EVALUATION", Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Source: EvidenceDeterministicControl, ObservedAt: observedAt},
		}},
		ObservedAt: observedAt,
	}
}
