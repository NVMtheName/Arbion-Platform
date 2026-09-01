package strategy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

const paperEvidenceFingerprintSchema = "arbion.paper-evidence-review.v1"

var paperEvidenceFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type paperEvidenceReviewSnapshot struct {
	Schema             string                    `json:"schema"`
	StrategyInstanceID string                    `json:"strategy_instance_id"`
	FinancialAccountID string                    `json:"financial_account_id"`
	MandateID          string                    `json:"mandate_id"`
	MandateVersion     int                       `json:"mandate_version"`
	PortfolioVersion   int64                     `json:"portfolio_version"`
	PortfolioUpdatedAt time.Time                 `json:"portfolio_updated_at"`
	EvidenceReadiness  PaperAutonomyEvidenceGate `json:"evidence_readiness"`
}

func paperEvidenceFingerprint(instance Instance, portfolio PaperPortfolio) (string, error) {
	payload, err := json.Marshal(paperEvidenceReviewSnapshot{
		Schema:             paperEvidenceFingerprintSchema,
		StrategyInstanceID: instance.ID,
		FinancialAccountID: instance.FinancialAccountID,
		MandateID:          instance.AutomationMandateID,
		MandateVersion:     instance.MandateVersion,
		PortfolioVersion:   portfolio.Version,
		PortfolioUpdatedAt: portfolio.UpdatedAt,
		EvidenceReadiness:  portfolio.EvidenceReadiness,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func paperEvidenceReviewCheckpoint(instance Instance, gate PaperAutonomyEvidenceGate) (PaperAutonomyEvidenceThresholdCheckpoint, bool) {
	packet := gate.ReviewPacket
	ledger := packet.ThresholdChangeLedger
	if instance.StrategyIdentifier != "ai_shadow" || instance.ExecutionMode != Paper || instance.CurrentState != AIMonitoring ||
		instance.ID == "" || instance.FinancialAccountID == "" || instance.AutomationMandateID == "" || instance.MandateVersion < 1 ||
		gate.Status != PaperAutonomyEvidenceReviewable || gate.CalculationMethod != PaperAutonomyEvidenceGateMethod || gate.AsOf == nil ||
		gate.ReviewScope != PaperAutonomyEvidenceReviewScope || gate.ExecutionBoundary != PaperAutonomyEvidenceBoundary ||
		gate.MinimumDecisionCount != PaperAutonomyEvidenceMinimumDecisions || gate.MinimumEvidenceWindowHours != PaperAutonomyEvidenceMinimumHours ||
		gate.EvidenceWindowHours < PaperAutonomyEvidenceMinimumHours || gate.DecisionCount < PaperAutonomyEvidenceMinimumDecisions ||
		gate.AttributedDecisionCount != gate.DecisionCount || gate.TelemetryCompleteCount != gate.DecisionCount || gate.BoundedMemoryCount != gate.DecisionCount ||
		gate.LastScheduleStatus != "SUCCEEDED" || gate.ConsecutiveScheduleFailures != 0 || !gate.LedgerContractsReconciled ||
		gate.Safety.Status != "CLEAR" || gate.Safety.LiveMandateCount != 0 || gate.Safety.AIOrderIntentCount != 0 ||
		gate.Safety.InvalidStrategyModeCount != 0 || gate.Safety.InvalidExecutionModeCount != 0 || gate.Safety.PlatformExecutableRiskCount != 0 || gate.Safety.NonSimulationFillCount != 0 ||
		len(gate.Blockers) != 0 || gate.LiveExecutionAvailable ||
		packet.Status != PaperAutonomyEvidenceReviewable || packet.CalculationMethod != PaperAutonomyEvidenceReviewPacketMethod ||
		packet.EvidenceStartedAt == nil || packet.EvidenceEligibleAt == nil || packet.AsOf == nil || !packet.AsOf.Equal(*gate.AsOf) ||
		!packet.EvidenceEligibleAt.Equal(packet.EvidenceStartedAt.Add(time.Duration(PaperAutonomyEvidenceMinimumHours)*time.Hour)) || packet.AsOf.Before(*packet.EvidenceEligibleAt) ||
		packet.RemainingSeconds != 0 || packet.SchedulerSampleCount < PaperAutonomyEvidenceMinimumDecisions || packet.SchedulerSuccessCount < PaperAutonomyEvidenceMinimumDecisions ||
		packet.InputCoverageStatus != "COMPLETE" || packet.InputFreshnessStatus != "CURRENT_AT_DECISION" || packet.LedgerContractStatus != "RECONCILED" || packet.NoLiveSafetyStatus != "CLEAR" ||
		!packet.EvidenceReadyForHumanReview || packet.GrantsAuthority || packet.LivePromotionAvailable ||
		ledger.Status != PaperActivityCadenceAvailable || ledger.CalculationMethod != PaperAutonomyThresholdChangeLedgerMethod ||
		ledger.StrategyInstanceID != instance.ID || ledger.ExecutionMode != Paper || ledger.CheckpointCount != len(ledger.Checkpoints) || ledger.CheckpointCount == 0 ||
		ledger.GrantsAuthority || ledger.LivePromotionAvailable {
		return PaperAutonomyEvidenceThresholdCheckpoint{}, false
	}
	latest := ledger.Checkpoints[len(ledger.Checkpoints)-1]
	if latest.ScheduleRunID == "" || latest.AsOf.IsZero() || !latest.AsOf.Equal(*gate.AsOf) || latest.EvidenceStatus != PaperAutonomyEvidenceReviewable ||
		latest.SchedulerStatus != "SUCCEEDED" || latest.ConsecutiveFailures != 0 || latest.DecisionCount != gate.DecisionCount ||
		latest.InputCoverageStatus != packet.InputCoverageStatus || latest.InputFreshnessStatus != packet.InputFreshnessStatus || len(latest.Blockers) != 0 {
		return PaperAutonomyEvidenceThresholdCheckpoint{}, false
	}
	return latest, true
}

func (s *InstanceService) loadPaperPortfolio(ctx context.Context, userID, instanceID string) (Instance, PaperPortfolio, error) {
	instance, err := s.store.Get(ctx, userID, instanceID)
	if err != nil {
		return Instance{}, PaperPortfolio{}, ErrNotFound
	}
	if instance.ExecutionMode != Paper {
		return Instance{}, PaperPortfolio{}, ErrInvalid
	}
	portfolio, err := s.store.PaperPortfolio(ctx, userID, instanceID)
	if err != nil {
		return Instance{}, PaperPortfolio{}, ErrNotFound
	}
	if portfolio.StrategyInstanceID != instance.ID || portfolio.Version < 0 {
		return Instance{}, PaperPortfolio{}, ErrInvalid
	}
	fingerprint, err := paperEvidenceFingerprint(instance, portfolio)
	if err != nil {
		return Instance{}, PaperPortfolio{}, err
	}
	portfolio.EvidenceReviewFingerprint = fingerprint
	if reviews, available := s.store.(PaperEvidenceReviewStore); available {
		latest, reviewErr := reviews.LatestPaperEvidenceReview(ctx, userID, instanceID)
		if reviewErr != nil {
			return Instance{}, PaperPortfolio{}, reviewErr
		}
		portfolio.LatestEvidenceReview = latest
		portfolio.CurrentEvidenceReviewed = latest != nil && latest.EvidenceFingerprint == fingerprint
	}
	return instance, portfolio, nil
}

func (s *InstanceService) PaperEvidenceReviews(ctx context.Context, principal authorization.Principal, instanceID string, limit int, cursor *PaperEvidenceReviewCursor) (PaperEvidenceReviewPage, error) {
	if !entitled(principal) {
		return PaperEvidenceReviewPage{}, ErrForbidden
	}
	if instanceID == "" || limit < 1 || limit > 50 || (cursor != nil && (cursor.ReviewedAt.IsZero() || cursor.ID == "")) {
		return PaperEvidenceReviewPage{}, ErrInvalid
	}
	instance, err := s.store.Get(ctx, principal.UserID, instanceID)
	if err != nil {
		return PaperEvidenceReviewPage{}, ErrNotFound
	}
	if instance.StrategyIdentifier != "ai_shadow" || instance.ExecutionMode != Paper {
		return PaperEvidenceReviewPage{}, ErrInvalid
	}
	reader, ok := s.store.(PaperEvidenceReviewReader)
	if !ok {
		return PaperEvidenceReviewPage{}, ErrNotFound
	}
	reviews, err := reader.PaperEvidenceReviews(ctx, principal.UserID, instanceID, limit+1, cursor)
	if err != nil {
		return PaperEvidenceReviewPage{}, err
	}
	page := PaperEvidenceReviewPage{Reviews: reviews}
	if len(reviews) > limit {
		page.Reviews = reviews[:limit]
		last := page.Reviews[len(page.Reviews)-1]
		page.NextCursor = &PaperEvidenceReviewCursor{ReviewedAt: last.ReviewedAt, ID: last.ID}
	}
	return page, nil
}

// RecordPaperEvidenceReview appends an owner acknowledgment of one exact Paper
// evidence packet. It does not mutate any strategy, schedule, portfolio,
// account, order, or execution boundary.
func (s *InstanceService) RecordPaperEvidenceReview(ctx context.Context, principal authorization.Principal, instanceID string, command PaperEvidenceReviewCommand) (PaperEvidenceReview, error) {
	if !entitled(principal) {
		return PaperEvidenceReview{}, ErrForbidden
	}
	if instanceID == "" || !command.ConfirmPaperReview || !paperEvidenceFingerprintPattern.MatchString(command.EvidenceFingerprint) {
		return PaperEvidenceReview{}, ErrInvalid
	}
	instance, portfolio, err := s.loadPaperPortfolio(ctx, principal.UserID, instanceID)
	if err != nil {
		return PaperEvidenceReview{}, err
	}
	checkpoint, reviewable := paperEvidenceReviewCheckpoint(instance, portfolio.EvidenceReadiness)
	if !reviewable {
		return PaperEvidenceReview{}, ErrEvidenceNotReviewable
	}
	if command.EvidenceFingerprint != portfolio.EvidenceReviewFingerprint {
		return PaperEvidenceReview{}, ErrEvidenceSnapshotChanged
	}
	if portfolio.CurrentEvidenceReviewed && portfolio.LatestEvidenceReview != nil {
		return *portfolio.LatestEvidenceReview, nil
	}
	reviews, ok := s.store.(PaperEvidenceReviewStore)
	if !ok || s.evidenceReviewStepUp == nil {
		return PaperEvidenceReview{}, ErrEvidenceReviewStepUp
	}
	method, verifiedAt, err := s.evidenceReviewStepUp.VerifyPaperEvidenceReviewStepUp(ctx, principal.UserID, command.MFACode)
	if err != nil || method != "totp" || verifiedAt.IsZero() {
		return PaperEvidenceReview{}, ErrEvidenceReviewStepUp
	}
	gate, packet := portfolio.EvidenceReadiness, portfolio.EvidenceReadiness.ReviewPacket
	review, err := reviews.CreatePaperEvidenceReview(ctx, principal.UserID, PaperEvidenceReview{
		StrategyInstanceID: instance.ID, FinancialAccountID: instance.FinancialAccountID,
		MandateID: instance.AutomationMandateID, MandateVersion: instance.MandateVersion,
		EvidenceFingerprint: portfolio.EvidenceReviewFingerprint, GateStatus: gate.Status,
		EvidenceStartedAt: *packet.EvidenceStartedAt, EvidenceEligibleAt: *packet.EvidenceEligibleAt, EvidenceAsOf: *gate.AsOf,
		EvidenceWindowHours: gate.EvidenceWindowHours, DecisionCount: gate.DecisionCount,
		PortfolioVersion: portfolio.Version, PortfolioUpdatedAt: portfolio.UpdatedAt,
		LatestCheckpointRunID: checkpoint.ScheduleRunID, LatestCheckpointAsOf: checkpoint.AsOf,
		SchedulerSampleCount: packet.SchedulerSampleCount, SchedulerSuccessCount: packet.SchedulerSuccessCount, SchedulerFailureCount: packet.SchedulerFailureCount,
		LastScheduleStatus: gate.LastScheduleStatus, ConsecutiveScheduleFailures: gate.ConsecutiveScheduleFailures,
		RouteContinuityStatus: packet.RouteContinuityStatus, InputCoverageStatus: packet.InputCoverageStatus,
		InputFreshnessStatus: packet.InputFreshnessStatus, LedgerContractStatus: packet.LedgerContractStatus,
		NoLiveSafetyStatus: packet.NoLiveSafetyStatus, ExecutionBoundary: PaperAutonomyEvidenceBoundary,
		ReviewScope: PaperEvidenceReviewScope, GrantsAuthority: false, LivePromotionAvailable: false,
		MFAMethod: method, ReviewedAt: verifiedAt.UTC(),
	})
	if err != nil {
		return PaperEvidenceReview{}, err
	}
	if s.audit != nil {
		_ = s.audit.Record(ctx, &principal.UserID, "strategy_instance.paper_evidence_reviewed", map[string]any{
			"strategy_instance_id": instance.ID, "financial_account_id": instance.FinancialAccountID,
			"mandate_id": instance.AutomationMandateID, "mandate_version": instance.MandateVersion,
			"evidence_fingerprint": review.EvidenceFingerprint, "latest_checkpoint_run_id": review.LatestCheckpointRunID,
			"decision_count": review.DecisionCount, "evidence_window_hours": review.EvidenceWindowHours,
			"execution_boundary": PaperAutonomyEvidenceBoundary, "review_scope": PaperEvidenceReviewScope,
			"grants_authority": false, "live_promotion_available": false, "broker_order_created": false,
		})
	}
	return review, nil
}
