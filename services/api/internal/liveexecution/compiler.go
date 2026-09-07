package liveexecution

import (
	"errors"
	"strings"
	"time"
)

var ErrSafetyCaseCompilation = errors.New("live safety case cannot be compiled into registry evidence")

const assessmentKeyPrefix = "live-safety-"

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
