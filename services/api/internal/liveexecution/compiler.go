package liveexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
)

var (
	ErrSafetyCaseCompilation = errors.New("live safety case cannot be compiled into registry evidence")
	ErrSafetyCaseEnvelope    = errors.New("live safety case compilation envelope is invalid")
)

const (
	assessmentKeyPrefix       = "live-safety-"
	SafetyCaseCompilerVersion = "live-safety-case-compiler-v1"
)

// SafetyCaseCompilationEnvelope binds the complete typed input and the exact
// canonical registry projection without granting authority or persisting a
// record. Its digests are review evidence only.
type SafetyCaseCompilationEnvelope struct {
	CompilerVersion     string        `json:"compiler_version"`
	ContractVersion     string        `json:"contract_version"`
	EvaluatedAt         time.Time     `json:"evaluated_at"`
	SafetyCaseDigest    string        `json:"safety_case_sha256"`
	RegistryInputDigest string        `json:"registry_input_sha256"`
	EnvelopeDigest      string        `json:"envelope_sha256"`
	RegistryInput       RegistryInput `json:"registry_input"`
}

type compilerEvidence struct {
	reference ImmutableEvidenceRef
	kind      string
	source    EvidenceSource
}

// CompileSafetyCaseAssessment is a pure, credential-free adapter between the
// typed safety contract and the dormant evidence registry. It does not persist
// anything and cannot return executable authority.
func CompileSafetyCaseAssessment(safetyCase SafetyCase, evaluatedAt time.Time) (RegistryInput, error) {
	if evaluatedAt.IsZero() {
		return RegistryInput{}, ErrSafetyCaseCompilation
	}
	evaluatedAt = evaluatedAt.UTC()
	assessment := (UnavailableBoundary{}).Assess(safetyCase, evaluatedAt)
	if assessment.Availability != Unavailable && assessment.Availability != BlockedUnimplemented {
		return RegistryInput{}, ErrSafetyCaseCompilation
	}
	for _, reason := range assessment.ReasonCodes {
		switch reason {
		case ReasonInvalidEvidence, ReasonDuplicateEvidence, ReasonFutureEvidence,
			ReasonBindingMismatch, ReasonActionDigestMismatch:
			return RegistryInput{}, ErrSafetyCaseCompilation
		}
	}

	evidence := []compilerEvidence{
		{safetyCase.ProviderCapability.Evidence, "PROVIDER_CAPABILITY", EvidenceProviderVerified},
		{safetyCase.OwnerAuthorization.Evidence, "OWNER_AUTHORIZATION", EvidenceOwnerMFA},
		{safetyCase.Risk.Evidence, "DETERMINISTIC_RISK", EvidenceDeterministicControl},
		{safetyCase.PreTradeReconciliation.Evidence, "ACCOUNT_RECONCILIATION", EvidenceDatabase},
		{safetyCase.KillSwitch.Evidence, "BROKER_KILL_SWITCH", EvidenceDeterministicControl},
		{safetyCase.Idempotency.Evidence, "IDEMPOTENCY_RESERVATION", EvidenceDatabase},
		{safetyCase.Lifecycle.Evidence, "LIVE_LIFECYCLE_CONTRACT", EvidenceDesignContract},
	}
	items := make([]EvidenceItem, 0, len(evidence))
	seen := make(map[string]struct{}, len(evidence))
	for _, expected := range evidence {
		ref := expected.reference
		if !validEvidenceRef(ref) || ref.Kind != expected.kind ||
			!evidenceKindPattern.MatchString(ref.Kind) || containsSecretLikeValue(ref.Kind) ||
			ref.RecordedAt.After(evaluatedAt) {
			return RegistryInput{}, ErrSafetyCaseCompilation
		}
		if _, exists := seen[ref.ID]; exists {
			return RegistryInput{}, ErrSafetyCaseCompilation
		}
		seen[ref.ID] = struct{}{}
		items = append(items, EvidenceItem{
			ID: ref.ID, Kind: ref.Kind, Digest: ref.Digest,
			Source: expected.source, ObservedAt: ref.RecordedAt.UTC(),
		})
	}

	action := safetyCase.Action
	input := RegistryInput{
		AssessmentKey:        assessmentKeyPrefix + strings.ReplaceAll(action.ID, "-", ""),
		FinancialAccountID:   action.FinancialAccountID,
		ProviderConnectionID: action.ProviderConnectionID,
		ProviderName:         safetyCase.ProviderCapability.Provider,
		StrategyInstanceID:   action.StrategyInstanceID,
		MandateID:            action.MandateID,
		MandateVersion:       action.MandateVersion,
		CapitalBucketID:      action.CapitalBucketID,
		CapitalReservationID: action.CapitalReservationID,
		ActionDigest:         action.Digest,
		Assessment:           assessment,
		Evidence:             EvidenceManifest{Items: items},
		ObservedAt:           evaluatedAt,
	}
	prepared, _, _, err := prepareRegistryRecord(action.UserID, input, evaluatedAt)
	if err != nil {
		return RegistryInput{}, ErrSafetyCaseCompilation
	}
	input.Assessment = prepared.Assessment
	input.Evidence = prepared.Evidence
	input.ObservedAt = prepared.ObservedAt
	return input, nil
}

// CompileSafetyCaseEnvelope is a pure, deterministic review boundary. The
// returned envelope contains no executable authority and has no persistence,
// provider, credential, scheduler, or broker capability.
func CompileSafetyCaseEnvelope(safetyCase SafetyCase, evaluatedAt time.Time) (SafetyCaseCompilationEnvelope, error) {
	if safetyCaseHasSecretLikeValue(reflect.ValueOf(safetyCase)) {
		return SafetyCaseCompilationEnvelope{}, ErrSafetyCaseEnvelope
	}
	input, err := CompileSafetyCaseAssessment(safetyCase, evaluatedAt)
	if err != nil {
		return SafetyCaseCompilationEnvelope{}, ErrSafetyCaseEnvelope
	}
	evaluatedAt = evaluatedAt.UTC()
	safetyDigest, err := canonicalSafetyCaseDigest(safetyCase)
	if err != nil {
		return SafetyCaseCompilationEnvelope{}, ErrSafetyCaseEnvelope
	}
	registryDigest, err := canonicalRegistryInputDigest(input)
	if err != nil {
		return SafetyCaseCompilationEnvelope{}, ErrSafetyCaseEnvelope
	}
	envelopePayload, err := json.Marshal(struct {
		CompilerVersion     string `json:"compiler_version"`
		ContractVersion     string `json:"contract_version"`
		EvaluatedAt         string `json:"evaluated_at"`
		SafetyCaseDigest    string `json:"safety_case_sha256"`
		RegistryInputDigest string `json:"registry_input_sha256"`
	}{
		CompilerVersion: SafetyCaseCompilerVersion, ContractVersion: ContractVersion,
		EvaluatedAt: evaluatedAt.Format(time.RFC3339Nano), SafetyCaseDigest: safetyDigest,
		RegistryInputDigest: registryDigest,
	})
	if err != nil {
		return SafetyCaseCompilationEnvelope{}, ErrSafetyCaseEnvelope
	}
	return SafetyCaseCompilationEnvelope{
		CompilerVersion: SafetyCaseCompilerVersion, ContractVersion: ContractVersion,
		EvaluatedAt: evaluatedAt, SafetyCaseDigest: safetyDigest,
		RegistryInputDigest: registryDigest, EnvelopeDigest: compilerSHA256(envelopePayload),
		RegistryInput: input,
	}, nil
}

// VerifySafetyCaseCompilationEnvelope recomputes the complete canonical
// envelope. Any changed fact, digest, timestamp, assessment, evidence item, or
// ordering fails closed; successful verification still grants no authority.
func VerifySafetyCaseCompilationEnvelope(safetyCase SafetyCase, envelope SafetyCaseCompilationEnvelope) error {
	if envelope.CompilerVersion != SafetyCaseCompilerVersion || envelope.ContractVersion != ContractVersion ||
		envelope.EvaluatedAt.IsZero() || envelope.EvaluatedAt.Location() != time.UTC ||
		!digestPattern.MatchString(envelope.SafetyCaseDigest) ||
		!digestPattern.MatchString(envelope.RegistryInputDigest) ||
		!digestPattern.MatchString(envelope.EnvelopeDigest) {
		return ErrSafetyCaseEnvelope
	}
	expected, err := CompileSafetyCaseEnvelope(safetyCase, envelope.EvaluatedAt)
	if err != nil || !reflect.DeepEqual(expected, envelope) {
		return ErrSafetyCaseEnvelope
	}
	return nil
}

func canonicalSafetyCaseDigest(safetyCase SafetyCase) (string, error) {
	canonical := safetyCase
	canonical.Action.CreatedAt = canonical.Action.CreatedAt.UTC()
	canonical.ProviderCapability.Evidence.RecordedAt = canonical.ProviderCapability.Evidence.RecordedAt.UTC()
	canonical.ProviderCapability.ObservedAt = canonical.ProviderCapability.ObservedAt.UTC()
	canonical.ProviderCapability.ExpiresAt = canonical.ProviderCapability.ExpiresAt.UTC()
	canonical.OwnerAuthorization.Evidence.RecordedAt = canonical.OwnerAuthorization.Evidence.RecordedAt.UTC()
	canonical.OwnerAuthorization.ApprovedAt = canonical.OwnerAuthorization.ApprovedAt.UTC()
	canonical.OwnerAuthorization.ExpiresAt = canonical.OwnerAuthorization.ExpiresAt.UTC()
	canonical.Risk.Evidence.RecordedAt = canonical.Risk.Evidence.RecordedAt.UTC()
	canonical.Risk.EvaluatedAt = canonical.Risk.EvaluatedAt.UTC()
	canonical.PreTradeReconciliation.Evidence.RecordedAt = canonical.PreTradeReconciliation.Evidence.RecordedAt.UTC()
	canonical.PreTradeReconciliation.ObservedAt = canonical.PreTradeReconciliation.ObservedAt.UTC()
	canonical.KillSwitch.Evidence.RecordedAt = canonical.KillSwitch.Evidence.RecordedAt.UTC()
	canonical.KillSwitch.ObservedAt = canonical.KillSwitch.ObservedAt.UTC()
	canonical.Idempotency.Evidence.RecordedAt = canonical.Idempotency.Evidence.RecordedAt.UTC()
	canonical.Idempotency.ReservedAt = canonical.Idempotency.ReservedAt.UTC()
	canonical.Lifecycle.Evidence.RecordedAt = canonical.Lifecycle.Evidence.RecordedAt.UTC()
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return compilerSHA256(payload), nil
}

func canonicalRegistryInputDigest(input RegistryInput) (string, error) {
	reasons := append([]ReasonCode(nil), input.Assessment.ReasonCodes...)
	sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
	items := append([]EvidenceItem(nil), input.Evidence.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for index := range items {
		items[index].ObservedAt = items[index].ObservedAt.UTC()
	}
	payload, err := json.Marshal(struct {
		AssessmentKey        string           `json:"assessment_key"`
		FinancialAccountID   string           `json:"financial_account_id"`
		ProviderConnectionID string           `json:"provider_connection_id"`
		ProviderName         string           `json:"provider_name"`
		StrategyInstanceID   string           `json:"strategy_instance_id"`
		MandateID            string           `json:"mandate_id"`
		MandateVersion       int              `json:"mandate_version"`
		CapitalBucketID      string           `json:"capital_bucket_id"`
		CapitalReservationID string           `json:"capital_reservation_id"`
		ActionDigest         string           `json:"action_digest_sha256"`
		Availability         Availability     `json:"availability"`
		ReasonCodes          []ReasonCode     `json:"reason_codes"`
		Structural           bool             `json:"structural_blocker"`
		Evidence             EvidenceManifest `json:"evidence"`
		ObservedAt           string           `json:"observed_at"`
	}{
		AssessmentKey: input.AssessmentKey, FinancialAccountID: input.FinancialAccountID,
		ProviderConnectionID: input.ProviderConnectionID, ProviderName: input.ProviderName,
		StrategyInstanceID: input.StrategyInstanceID, MandateID: input.MandateID,
		MandateVersion: input.MandateVersion, CapitalBucketID: input.CapitalBucketID,
		CapitalReservationID: input.CapitalReservationID, ActionDigest: input.ActionDigest,
		Availability: input.Assessment.Availability, ReasonCodes: reasons,
		Structural: input.Assessment.Structural, Evidence: EvidenceManifest{Items: items},
		ObservedAt: input.ObservedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return "", err
	}
	return compilerSHA256(payload), nil
}

func safetyCaseHasSecretLikeValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return false
		}
		return safetyCaseHasSecretLikeValue(value.Elem())
	case reflect.String:
		return containsSecretLikeValue(value.String())
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return false
		}
		for index := 0; index < value.NumField(); index++ {
			if safetyCaseHasSecretLikeValue(value.Field(index)) {
				return true
			}
		}
	case reflect.Array, reflect.Slice:
		for index := 0; index < value.Len(); index++ {
			if safetyCaseHasSecretLikeValue(value.Index(index)) {
				return true
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if safetyCaseHasSecretLikeValue(iterator.Key()) || safetyCaseHasSecretLikeValue(iterator.Value()) {
				return true
			}
		}
	}
	return false
}

func compilerSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
