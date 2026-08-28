package strategy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"

	"github.com/arbion/platform/services/api/internal/authorization"
)

const shadowEvidenceFingerprintSchema = "arbion.shadow-evidence-scorecard.v1"

var shadowEvidenceFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type shadowEvidenceSnapshot struct {
	Schema             string               `json:"schema"`
	StrategyInstanceID string               `json:"strategy_instance_id"`
	MandateID          string               `json:"mandate_id"`
	MandateVersion     int                  `json:"mandate_version"`
	TotalMarks         int                  `json:"total_marks"`
	Horizons           []ShadowHorizonScore `json:"horizons"`
	Behavior           ShadowBehaviorScore  `json:"behavior"`
	EvidenceGate       ShadowEvidenceGate   `json:"evidence_gate"`
}

func shadowEvidenceFingerprint(instance Instance, scorecard ShadowScorecard) (string, error) {
	payload, err := json.Marshal(shadowEvidenceSnapshot{
		Schema:             shadowEvidenceFingerprintSchema,
		StrategyInstanceID: instance.ID,
		MandateID:          instance.AutomationMandateID,
		MandateVersion:     instance.MandateVersion,
		TotalMarks:         scorecard.TotalMarks,
		Horizons:           scorecard.Horizons,
		Behavior:           scorecard.Behavior,
		EvidenceGate:       scorecard.EvidenceGate,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func (s *InstanceService) loadShadowScorecard(ctx context.Context, userID, instanceID string) (Instance, ShadowScorecard, error) {
	instance, err := s.store.Get(ctx, userID, instanceID)
	if err != nil {
		return Instance{}, ShadowScorecard{}, ErrNotFound
	}
	if instance.StrategyIdentifier != "ai_shadow" || instance.ExecutionMode != Shadow || instance.CurrentState != AIMonitoring {
		return Instance{}, ShadowScorecard{}, ErrInvalid
	}
	reader, ok := s.store.(ShadowScorecardReader)
	if !ok {
		return Instance{}, ShadowScorecard{}, ErrInvalid
	}
	scorecard, err := reader.ShadowScorecard(ctx, userID, instanceID)
	if err != nil {
		return Instance{}, ShadowScorecard{}, err
	}
	if scorecard.StrategyInstanceID != instance.ID {
		return Instance{}, ShadowScorecard{}, ErrInvalid
	}
	fingerprint, err := shadowEvidenceFingerprint(instance, scorecard)
	if err != nil {
		return Instance{}, ShadowScorecard{}, err
	}
	scorecard.EvidenceReviewFingerprint = fingerprint
	if reviews, available := s.store.(ShadowEvidenceReviewStore); available {
		latest, reviewErr := reviews.LatestShadowEvidenceReview(ctx, userID, instanceID)
		if reviewErr != nil {
			return Instance{}, ShadowScorecard{}, reviewErr
		}
		scorecard.LatestEvidenceReview = latest
		scorecard.CurrentEvidenceReviewed = latest != nil && latest.EvidenceFingerprint == fingerprint
	}
	return instance, scorecard, nil
}

func (s *InstanceService) ShadowScorecard(ctx context.Context, principal authorization.Principal, instanceID string) (ShadowScorecard, error) {
	if !entitled(principal) {
		return ShadowScorecard{}, ErrForbidden
	}
	_, scorecard, err := s.loadShadowScorecard(ctx, principal.UserID, instanceID)
	return scorecard, err
}

func shadowEvidenceReviewable(instance Instance, gate ShadowEvidenceGate) bool {
	return instance.StrategyIdentifier == "ai_shadow" &&
		instance.ExecutionMode == Shadow &&
		instance.CurrentState == AIMonitoring &&
		gate.Status == ShadowEvidenceReviewable &&
		len(gate.Blockers) == 0 &&
		gate.OneHourSampleSize >= ShadowScorecardMinimumSample &&
		gate.TwentyFourHourSampleSize >= ShadowScorecardMinimumSample &&
		gate.MinimumSamplePerHorizon == ShadowScorecardMinimumSample &&
		gate.EvidenceWindowHours >= ShadowEvidenceMinimumWindowHours &&
		gate.MinimumEvidenceWindowHours == ShadowEvidenceMinimumWindowHours &&
		gate.ScheduleHealthy &&
		gate.LastScheduleStatus == "SUCCEEDED" &&
		gate.ConsecutiveScheduleFailures == 0 &&
		gate.ExecutionBoundary == ShadowExecutionBoundary &&
		!gate.LiveExecutionAvailable
}

// RecordShadowEvidenceReview appends an owner acknowledgment of one exact
// non-live scorecard. It cannot mutate a mandate, instance, schedule, order,
// portfolio, or execution boundary.
func (s *InstanceService) RecordShadowEvidenceReview(ctx context.Context, principal authorization.Principal, instanceID string, command ShadowEvidenceReviewCommand) (ShadowEvidenceReview, error) {
	if !entitled(principal) {
		return ShadowEvidenceReview{}, ErrForbidden
	}
	if instanceID == "" || !command.ConfirmNonLiveReview || !shadowEvidenceFingerprintPattern.MatchString(command.EvidenceFingerprint) {
		return ShadowEvidenceReview{}, ErrInvalid
	}
	instance, scorecard, err := s.loadShadowScorecard(ctx, principal.UserID, instanceID)
	if err != nil {
		return ShadowEvidenceReview{}, err
	}
	if !shadowEvidenceReviewable(instance, scorecard.EvidenceGate) {
		return ShadowEvidenceReview{}, ErrEvidenceNotReviewable
	}
	if command.EvidenceFingerprint != scorecard.EvidenceReviewFingerprint {
		return ShadowEvidenceReview{}, ErrEvidenceSnapshotChanged
	}
	if scorecard.CurrentEvidenceReviewed && scorecard.LatestEvidenceReview != nil {
		return *scorecard.LatestEvidenceReview, nil
	}
	reviews, ok := s.store.(ShadowEvidenceReviewStore)
	if !ok || s.evidenceReviewStepUp == nil {
		return ShadowEvidenceReview{}, ErrEvidenceReviewStepUp
	}
	method, verifiedAt, err := s.evidenceReviewStepUp.VerifyShadowEvidenceReviewStepUp(ctx, principal.UserID, command.MFACode)
	if err != nil || method != "totp" || verifiedAt.IsZero() {
		return ShadowEvidenceReview{}, ErrEvidenceReviewStepUp
	}
	gate := scorecard.EvidenceGate
	review, err := reviews.CreateShadowEvidenceReview(ctx, principal.UserID, ShadowEvidenceReview{
		StrategyInstanceID:          instance.ID,
		MandateID:                   instance.AutomationMandateID,
		MandateVersion:              instance.MandateVersion,
		EvidenceFingerprint:         scorecard.EvidenceReviewFingerprint,
		GateStatus:                  gate.Status,
		OneHourSampleSize:           gate.OneHourSampleSize,
		TwentyFourHourSampleSize:    gate.TwentyFourHourSampleSize,
		EvidenceWindowHours:         gate.EvidenceWindowHours,
		ScheduleHealthy:             gate.ScheduleHealthy,
		LastScheduleStatus:          gate.LastScheduleStatus,
		ConsecutiveScheduleFailures: gate.ConsecutiveScheduleFailures,
		ExecutionBoundary:           gate.ExecutionBoundary,
		LiveExecutionAvailable:      false,
		ReviewScope:                 ShadowEvidenceReviewScope,
		MFAMethod:                   method,
		ReviewedAt:                  verifiedAt.UTC(),
	})
	if err != nil {
		return ShadowEvidenceReview{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, &principal.UserID, "strategy_instance.shadow_evidence_reviewed", map[string]any{
			"strategy_instance_id":         instance.ID,
			"mandate_id":                   instance.AutomationMandateID,
			"mandate_version":              instance.MandateVersion,
			"evidence_fingerprint":         review.EvidenceFingerprint,
			"one_hour_sample_size":         review.OneHourSampleSize,
			"twenty_four_hour_sample_size": review.TwentyFourHourSampleSize,
			"evidence_window_hours":        review.EvidenceWindowHours,
			"execution_boundary":           ShadowExecutionBoundary,
			"review_scope":                 ShadowEvidenceReviewScope,
			"live_execution_available":     false,
			"broker_order_created":         false,
			"execution_authority_granted":  false,
		})
	}
	return review, nil
}
