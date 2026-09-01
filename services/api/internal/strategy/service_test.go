package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

type journalPersistenceFake struct {
	entries                 []JournalActivity
	scheduleRuns            []ScheduleRun
	requestedUser           string
	requestedLimit          int
	requestedAfter          *JournalCursor
	requestedRunAfter       *ScheduleRunCursor
	lifecycleID             string
	lifecycle               LifecycleCommand
	lifecycleAt             time.Time
	lifecycleResult         LifecycleResult
	lifecycleError          error
	instance                Instance
	getError                error
	portfolio               PaperPortfolio
	portfolioError          error
	portfolioCalls          int
	scorecard               ShadowScorecard
	scorecardError          error
	scorecardCalls          int
	latestReview            *ShadowEvidenceReview
	latestReviewError       error
	evidenceReviews         []ShadowEvidenceReview
	reviewHistoryError      error
	reviewLimit             int
	reviewAfter             *ShadowEvidenceReviewCursor
	pageDecisions           []DecisionJournalEntry
	pageOutcomes            []ShadowOutcome
	pageError               error
	pageOutcomeError        error
	pageLimit               int
	pageAfter               *StrategyDecisionCursor
	pageExecutionIDs        []string
	runtimeTransitions      []StrategyTransitionEvidence
	runtimeExecutions       []StrategyExecutionEvidence
	transitionError         error
	executionError          error
	transitionLimit         int
	executionLimit          int
	transitionAfter         *StrategyTransitionCursor
	executionAfter          *StrategyExecutionCursor
	createdReview           ShadowEvidenceReview
	createReviewError       error
	createReviewCalls       int
	latestPaperReview       *PaperEvidenceReview
	latestPaperReviewError  error
	paperEvidenceReviews    []PaperEvidenceReview
	paperReviewHistoryError error
	paperReviewLimit        int
	paperReviewAfter        *PaperEvidenceReviewCursor
	createdPaperReview      PaperEvidenceReview
	createPaperReviewError  error
	createPaperReviewCalls  int
	requestedID             string
	initializedUser         string
	initializedCash         string
	initializedMandate      automation.Mandate
	pausedUser              string
	pausedID                string
	pausedVersion           int
	pausedAt                time.Time
	pauseResult             Instance
	pauseError              error
	resumedUser             string
	resumedID               string
	resumedVersion          int
	resumedAt               time.Time
	resumeResult            Instance
	resumeError             error
	finishedUser            string
	finishedID              string
	finishedVersion         int
	finishedAt              time.Time
	finishResult            Instance
	finishError             error
	reservation             CapitalReservation
	reservationError        error
	reservations            []CapitalReservation
	reservationsError       error
	paperFills              []AIPaperSpotFill
	paperFillError          error
	paperFillLimit          int
	paperFillAfter          *AIPaperSpotFillCursor
}

func (f *journalPersistenceFake) Initialize(_ context.Context, userID string, mandate automation.Mandate, cash string, state State) (Instance, error) {
	f.initializedUser = userID
	f.initializedCash = cash
	f.initializedMandate = mandate
	return Instance{UserID: userID, AutomationMandateID: mandate.ID, FinancialAccountID: mandate.FinancialAccountID, CapitalBucketID: mandate.CapitalBucketID, CurrentState: state}, nil
}
func (f *journalPersistenceFake) Pause(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.pausedUser = userID
	f.pausedID = instanceID
	f.pausedVersion = expectedVersion
	f.pausedAt = at
	return f.pauseResult, f.pauseError
}
func (f *journalPersistenceFake) Resume(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.resumedUser = userID
	f.resumedID = instanceID
	f.resumedVersion = expectedVersion
	f.resumedAt = at
	return f.resumeResult, f.resumeError
}
func (f *journalPersistenceFake) Finish(_ context.Context, userID, instanceID string, expectedVersion int, at time.Time) (Instance, error) {
	f.finishedUser = userID
	f.finishedID = instanceID
	f.finishedVersion = expectedVersion
	f.finishedAt = at
	return f.finishResult, f.finishError
}
func (*journalPersistenceFake) List(context.Context, string) ([]Instance, error) { return nil, nil }
func (f *journalPersistenceFake) Get(_ context.Context, userID, instanceID string) (Instance, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	return f.instance, f.getError
}
func (f *journalPersistenceFake) CapitalReservation(_ context.Context, userID, instanceID string) (CapitalReservation, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	return f.reservation, f.reservationError
}
func (f *journalPersistenceFake) CapitalReservations(_ context.Context, userID string) ([]CapitalReservation, error) {
	f.requestedUser = userID
	return f.reservations, f.reservationsError
}
func (f *journalPersistenceFake) AIPaperSpotFills(_ context.Context, userID, instanceID string, limit int, after *AIPaperSpotFillCursor) ([]AIPaperSpotFill, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.paperFillLimit = limit
	f.paperFillAfter = after
	if f.paperFillError != nil {
		return nil, f.paperFillError
	}
	if len(f.paperFills) < limit {
		limit = len(f.paperFills)
	}
	return f.paperFills[:limit], nil
}
func (f *journalPersistenceFake) StrategyTransitionEntries(_ context.Context, userID, instanceID string, limit int, after *StrategyTransitionCursor) ([]StrategyTransitionEvidence, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.transitionLimit = limit
	f.transitionAfter = after
	if f.transitionError != nil {
		return nil, f.transitionError
	}
	if len(f.runtimeTransitions) < limit {
		limit = len(f.runtimeTransitions)
	}
	return f.runtimeTransitions[:limit], nil
}
func (f *journalPersistenceFake) StrategyExecutionEntries(_ context.Context, userID, instanceID string, limit int, after *StrategyExecutionCursor) ([]StrategyExecutionEvidence, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.executionLimit = limit
	f.executionAfter = after
	if f.executionError != nil {
		return nil, f.executionError
	}
	if len(f.runtimeExecutions) < limit {
		limit = len(f.runtimeExecutions)
	}
	return f.runtimeExecutions[:limit], nil
}
func (f *journalPersistenceFake) StrategyDecisionEntries(_ context.Context, userID, instanceID string, limit int, after *StrategyDecisionCursor) ([]DecisionJournalEntry, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.pageLimit = limit
	f.pageAfter = after
	if f.pageError != nil {
		return nil, f.pageError
	}
	if len(f.pageDecisions) < limit {
		limit = len(f.pageDecisions)
	}
	return f.pageDecisions[:limit], nil
}
func (f *journalPersistenceFake) ShadowOutcomesForExecutions(_ context.Context, userID, instanceID string, executionIDs []string) ([]ShadowOutcome, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.pageExecutionIDs = append([]string(nil), executionIDs...)
	return f.pageOutcomes, f.pageOutcomeError
}
func (*journalPersistenceFake) ShadowOutcomes(context.Context, string, string) ([]ShadowOutcome, error) {
	return nil, nil
}
func (f *journalPersistenceFake) ShadowScorecard(_ context.Context, userID, instanceID string) (ShadowScorecard, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.scorecardCalls++
	return f.scorecard, f.scorecardError
}
func (f *journalPersistenceFake) LatestShadowEvidenceReview(_ context.Context, userID, instanceID string) (*ShadowEvidenceReview, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	return f.latestReview, f.latestReviewError
}
func (f *journalPersistenceFake) CreateShadowEvidenceReview(_ context.Context, userID string, review ShadowEvidenceReview) (ShadowEvidenceReview, error) {
	f.requestedUser = userID
	f.createReviewCalls++
	f.createdReview = review
	if f.createReviewError != nil {
		return ShadowEvidenceReview{}, f.createReviewError
	}
	if review.ID == "" {
		review.ID = "review-1"
	}
	if review.CreatedAt.IsZero() {
		review.CreatedAt = review.ReviewedAt
	}
	return review, nil
}
func (f *journalPersistenceFake) ShadowEvidenceReviews(_ context.Context, userID, instanceID string, limit int, after *ShadowEvidenceReviewCursor) ([]ShadowEvidenceReview, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.reviewLimit = limit
	f.reviewAfter = after
	if f.reviewHistoryError != nil {
		return nil, f.reviewHistoryError
	}
	if len(f.evidenceReviews) < limit {
		limit = len(f.evidenceReviews)
	}
	return f.evidenceReviews[:limit], nil
}
func (f *journalPersistenceFake) LatestPaperEvidenceReview(_ context.Context, userID, instanceID string) (*PaperEvidenceReview, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	return f.latestPaperReview, f.latestPaperReviewError
}
func (f *journalPersistenceFake) CreatePaperEvidenceReview(_ context.Context, userID string, review PaperEvidenceReview) (PaperEvidenceReview, error) {
	f.requestedUser = userID
	f.createPaperReviewCalls++
	f.createdPaperReview = review
	if f.createPaperReviewError != nil {
		return PaperEvidenceReview{}, f.createPaperReviewError
	}
	if review.ID == "" {
		review.ID = "paper-review-1"
	}
	if review.CreatedAt.IsZero() {
		review.CreatedAt = review.ReviewedAt
	}
	return review, nil
}
func (f *journalPersistenceFake) PaperEvidenceReviews(_ context.Context, userID, instanceID string, limit int, after *PaperEvidenceReviewCursor) ([]PaperEvidenceReview, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.paperReviewLimit = limit
	f.paperReviewAfter = after
	if f.paperReviewHistoryError != nil {
		return nil, f.paperReviewHistoryError
	}
	if len(f.paperEvidenceReviews) < limit {
		limit = len(f.paperEvidenceReviews)
	}
	return f.paperEvidenceReviews[:limit], nil
}
func (f *journalPersistenceFake) PaperPortfolio(_ context.Context, userID, instanceID string) (PaperPortfolio, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.portfolioCalls++
	return f.portfolio, f.portfolioError
}
func (f *journalPersistenceFake) Journal(_ context.Context, userID string, limit int, after *JournalCursor) ([]JournalActivity, error) {
	f.requestedUser = userID
	f.requestedLimit = limit
	f.requestedAfter = after
	if len(f.entries) < limit {
		limit = len(f.entries)
	}
	return f.entries[:limit], nil
}
func (*journalPersistenceFake) Schedule(context.Context, string, string) (ScheduleStatus, error) {
	return ScheduleStatus{}, nil
}
func (f *journalPersistenceFake) ScheduleRuns(_ context.Context, userID, instanceID string, limit int, after *ScheduleRunCursor) ([]ScheduleRun, error) {
	f.requestedUser = userID
	f.requestedID = instanceID
	f.requestedLimit = limit
	f.requestedRunAfter = after
	if len(f.scheduleRuns) < limit {
		limit = len(f.scheduleRuns)
	}
	return f.scheduleRuns[:limit], nil
}
func (f *journalPersistenceFake) RecordLifecycle(_ context.Context, userID, instanceID string, command LifecycleCommand, occurredAt time.Time) (LifecycleResult, error) {
	f.requestedUser = userID
	f.lifecycleID = instanceID
	f.lifecycle = command
	f.lifecycleAt = occurredAt
	return f.lifecycleResult, f.lifecycleError
}

type instanceMandatesFake struct {
	mandate automation.Mandate
	bucket  automation.CapitalBucket
}

type strategyAuditFake struct {
	userID   *string
	action   string
	metadata map[string]any
}

type evidenceReviewStepUpFake struct {
	userID     string
	code       string
	method     string
	verifiedAt time.Time
	err        error
	calls      int
}

func (f *evidenceReviewStepUpFake) VerifyShadowEvidenceReviewStepUp(_ context.Context, userID, code string) (string, time.Time, error) {
	f.userID = userID
	f.code = code
	f.calls++
	return f.method, f.verifiedAt, f.err
}

func (f *evidenceReviewStepUpFake) VerifyPaperEvidenceReviewStepUp(_ context.Context, userID, code string) (string, time.Time, error) {
	f.userID = userID
	f.code = code
	f.calls++
	return f.method, f.verifiedAt, f.err
}

func (f *strategyAuditFake) Record(_ context.Context, userID *string, action string, metadata map[string]any) error {
	f.userID = userID
	f.action = action
	f.metadata = metadata
	return nil
}

func (f *instanceMandatesFake) Get(context.Context, authorization.Principal, string) (automation.Mandate, error) {
	return f.mandate, nil
}

func (f *instanceMandatesFake) GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error) {
	return f.bucket, nil
}

func TestPaperInitializationIsBoundToProtectedBucketCapacity(t *testing.T) {
	wheel := "wheel"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, Status: "READY", ExecutionMode: "PAPER"},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "100", Currency: "USD", ProtectedAmount: "20", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, mandates)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	instance, err := service.Initialize(context.Background(), principal, "mandate", "80")
	if err != nil || store.initializedCash != "80" || instance.CapitalBucketID != "bucket" {
		t.Fatalf("authorized paper capacity was not bound: instance=%#v store=%#v err=%v", instance, store, err)
	}
	store.initializedCash = ""
	if _, err = service.Initialize(context.Background(), principal, "mandate", "80.0000000001"); !errors.Is(err, ErrCapitalLimit) || store.initializedCash != "" {
		t.Fatalf("over-capacity paper cash reached persistence: cash=%q err=%v", store.initializedCash, err)
	}

	limit := "75"
	mandates.bucket.AllocationLimit = &limit
	if _, err = service.Initialize(context.Background(), principal, "mandate", "55.0000000001"); !errors.Is(err, ErrCapitalLimit) {
		t.Fatalf("absolute bucket limit was not enforced: %v", err)
	}
	mandates.bucket.AllocationType = "PERCENT_OF_AVAILABLE_CASH"
	mandates.bucket.AllocationLimit = nil
	if _, err = service.Initialize(context.Background(), principal, "mandate", "1"); !errors.Is(err, ErrCapitalLimit) {
		t.Fatalf("unbounded percentage bucket initialized PAPER: %v", err)
	}
}

func TestShadowInitializationBindsBucketWithoutCreatingPaperCash(t *testing.T) {
	wheel := "wheel"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "STRATEGY", StrategyIdentifier: &wheel, Status: "READY", ExecutionMode: "SHADOW"},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "PERCENT_OF_AVAILABLE_CASH", AllocationValue: "25", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, mandates)
	if _, err := service.Initialize(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "mandate", "untrusted"); !errors.Is(err, ErrCapitalReservation) {
		t.Fatalf("unbounded percentage shadow bucket established a dollar reservation: %v", err)
	}
	limit := "500"
	mandates.bucket.AllocationLimit = &limit
	instance, err := service.Initialize(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "mandate", "untrusted")
	if err != nil || store.initializedCash != "" || instance.CapitalBucketID != "bucket" {
		t.Fatalf("shadow bucket binding changed: instance=%#v cash=%q err=%v", instance, store.initializedCash, err)
	}
}

func TestAIShadowInitializationCreatesMonitoringStateWithoutPaperCash(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "ai-mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &connection, AIModelID: &model, Status: "READY", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "SHADOW", StrategyParameters: []byte(`{"objective":"Preserve capital.","max_proposal_notional":"1"}`)},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "10", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	instance, err := NewInstanceService(store, mandates).Initialize(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "ai-mandate", "999")
	if err != nil || instance.CurrentState != AIMonitoring || store.initializedCash != "" {
		t.Fatalf("AI shadow initialization was not isolated: instance=%#v cash=%q err=%v", instance, store.initializedCash, err)
	}
}

func TestAIPaperInitializationCreatesIsolatedCashAndMonitoringState(t *testing.T) {
	connection, model := "ai", "gpt-5.6-sol"
	mandates := &instanceMandatesFake{
		mandate: automation.Mandate{ID: "ai-paper-mandate", UserID: "owner", FinancialAccountID: "account", CapitalBucketID: "bucket", AutomationType: "AI_AUTONOMOUS", AIProviderConnectionID: &connection, AIModelID: &model, Status: "READY", AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "PAPER", StrategyParameters: []byte(`{"objective":"Preserve simulated capital.","max_proposal_notional":"100"}`)},
		bucket:  automation.CapitalBucket{ID: "bucket", UserID: "owner", FinancialAccountID: "account", AllocationType: "FIXED_AMOUNT", AllocationValue: "1000", Currency: "USD", ProtectedAmount: "100", Status: "ACTIVE"},
	}
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, mandates)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	instance, err := service.Initialize(context.Background(), principal, "ai-paper-mandate", "900.0000000000")
	if err != nil || instance.CurrentState != AIMonitoring || store.initializedCash != "900.0000000000" {
		t.Fatalf("AI Paper initialization was not isolated: instance=%#v cash=%q err=%v", instance, store.initializedCash, err)
	}
	if _, err = service.Initialize(context.Background(), principal, "ai-paper-mandate", "900.0000000001"); !errors.Is(err, ErrCapitalLimit) {
		t.Fatalf("AI Paper starting cash exceeded protected bucket capacity: %v", err)
	}
}

func TestCapitalReservationIsOwnerScopedAndDurable(t *testing.T) {
	amount := "1000.0000000000"
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance", UserID: "owner"},
		reservation: CapitalReservation{
			ID: "reservation", StrategyInstanceID: "instance", FinancialAccountID: "account",
			CapitalBucketID: "bucket", ExecutionMode: Shadow, ReservationAmount: &amount,
			Currency: "USD", ReservationBasis: "BUCKET_FIXED_CAPACITY", Status: "ACTIVE",
		},
	}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	reservation, err := service.CapitalReservation(context.Background(), principal, "instance")
	if err != nil || reservation.ID != "reservation" || reservation.ReservationAmount == nil || *reservation.ReservationAmount != amount || store.requestedUser != "owner" {
		t.Fatalf("owner reservation was not preserved: reservation=%#v store=%#v err=%v", reservation, store, err)
	}
	store.instance.UserID = "different-owner"
	if _, err = service.CapitalReservation(context.Background(), principal, "instance"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner reservation lookup did not fail closed: %v", err)
	}
}

func TestCapitalReservationsAreOwnerScopedAndRequireEntitlement(t *testing.T) {
	amount := "25.0000000000"
	store := &journalPersistenceFake{reservations: []CapitalReservation{{
		ID: "reservation", StrategyInstanceID: "instance", FinancialAccountID: "account",
		CapitalBucketID: "bucket", ExecutionMode: Shadow, ReservationAmount: &amount,
		Currency: "USD", ReservationBasis: "BUCKET_FIXED_CAPACITY", Status: "ACTIVE",
	}}}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	reservations, err := service.CapitalReservations(context.Background(), principal)
	if err != nil || len(reservations) != 1 || reservations[0].ID != "reservation" || store.requestedUser != "owner" {
		t.Fatalf("owner reservation inventory was not preserved: reservations=%#v store=%#v err=%v", reservations, store, err)
	}
	principal.Entitlement = authorization.EntitlementFree
	if _, err = service.CapitalReservations(context.Background(), principal); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled reservation inventory was not rejected: %v", err)
	}
}

func TestFinishRequiresExplicitConfirmationAndAuditsReleasedClaim(t *testing.T) {
	now := time.Date(2026, 8, 19, 13, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{finishResult: Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "COMPLETED", StateVersion: 4}}
	audit := &strategyAuditFake{}
	service := NewInstanceService(store, nil, audit)
	service.now = func() time.Time { return now }
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	if _, err := service.Finish(context.Background(), principal, "instance", 3, false); !errors.Is(err, ErrInvalid) || store.finishedID != "" {
		t.Fatalf("unconfirmed finish reached persistence: id=%q err=%v", store.finishedID, err)
	}
	instance, err := service.Finish(context.Background(), principal, "instance", 3, true)
	if err != nil || instance.Status != "COMPLETED" || store.finishedUser != "owner" || store.finishedID != "instance" || store.finishedVersion != 3 || !store.finishedAt.Equal(now) {
		t.Fatalf("confirmed finish was not owner-scoped: instance=%#v store=%#v err=%v", instance, store, err)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.completed" || audit.metadata["strategy_instance_id"] != "instance" || audit.metadata["account_id"] != "account" {
		t.Fatalf("finish audit evidence is incomplete: %#v", audit)
	}
}

func TestPauseAndResumeAreOwnerScopedAndAudited(t *testing.T) {
	now := time.Date(2026, 8, 19, 13, 30, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		pauseResult:  Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "PAUSED", StateVersion: 4},
		resumeResult: Instance{ID: "instance", UserID: "owner", AutomationMandateID: "mandate", FinancialAccountID: "account", ExecutionMode: Paper, Status: "ACTIVE", StateVersion: 5},
	}
	audit := &strategyAuditFake{}
	service := NewInstanceService(store, nil, audit)
	service.now = func() time.Time { return now }
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	paused, err := service.Pause(context.Background(), principal, "instance", 3)
	if err != nil || paused.Status != "PAUSED" || store.pausedUser != "owner" || store.pausedID != "instance" || store.pausedVersion != 3 || !store.pausedAt.Equal(now) {
		t.Fatalf("pause was not owner-scoped: instance=%#v store=%#v err=%v", paused, store, err)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.paused" || audit.metadata["strategy_instance_id"] != "instance" {
		t.Fatalf("pause audit evidence is incomplete: %#v", audit)
	}
	if _, err = service.Resume(context.Background(), principal, "instance", 4, false); !errors.Is(err, ErrInvalid) || store.resumedID != "" {
		t.Fatalf("unconfirmed resume reached persistence: id=%q err=%v", store.resumedID, err)
	}
	resumed, err := service.Resume(context.Background(), principal, "instance", 4, true)
	if err != nil || resumed.Status != "ACTIVE" || store.resumedUser != "owner" || store.resumedID != "instance" || store.resumedVersion != 4 || !store.resumedAt.Equal(now) {
		t.Fatalf("resume was not owner-scoped: instance=%#v store=%#v err=%v", resumed, store, err)
	}
	if audit.action != "strategy_instance.resumed" || audit.metadata["account_id"] != "account" {
		t.Fatalf("resume audit evidence is incomplete: %#v", audit)
	}
}

func TestJournalIsOwnerScopedAndBuildsStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 18, 18, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{entries: []JournalActivity{
		{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: now},
		{ID: "22222222-2222-4222-8222-222222222222", CreatedAt: now.Add(-time.Minute)},
		{ID: "33333333-3333-4333-8333-333333333333", CreatedAt: now.Add(-2 * time.Minute)},
	}}
	service := NewInstanceService(store, nil)
	page, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedLimit != 3 {
		t.Fatalf("owner or lookahead was not preserved: user=%q limit=%d", store.requestedUser, store.requestedLimit)
	}
	if len(page.Entries) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Entries[1].ID || !page.NextCursor.CreatedAt.Equal(page.Entries[1].CreatedAt) {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestJournalRequiresAutomationEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	_, err := service.Journal(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, 25, nil)
	if !errors.Is(err, ErrForbidden) || store.requestedUser != "" {
		t.Fatalf("unentitled journal request was not rejected: %v", err)
	}
}

func TestStrategyTransitionPageIsOwnerScopedAndStable(t *testing.T) {
	now := time.Date(2026, 8, 28, 7, 0, 0, 0, time.UTC)
	after := &StrategyTransitionCursor{StateVersion: 6, ID: "99999999-9999-4999-8999-999999999999"}
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		runtimeTransitions: []StrategyTransitionEvidence{
			{ID: "11111111-1111-4111-8111-111111111111", StrategyInstanceID: "instance-1", PreviousState: AIMonitoring, NewState: AIMonitoring, StateVersion: 5, Trigger: "SCHEDULED_EVALUATION", OccurredAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", StrategyInstanceID: "instance-1", PreviousState: AIMonitoring, NewState: AIMonitoring, StateVersion: 4, Trigger: "SCHEDULED_EVALUATION", OccurredAt: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", StrategyInstanceID: "instance-1", PreviousState: AIMonitoring, NewState: AIMonitoring, StateVersion: 3, Trigger: "SCHEDULED_EVALUATION", OccurredAt: now.Add(-2 * time.Hour)},
		},
	}
	page, err := NewInstanceService(store, nil).TransitionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, after)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.transitionLimit != 3 || store.transitionAfter != after {
		t.Fatalf("transition owner boundary or lookahead changed: %#v", store)
	}
	if len(page.Transitions) != 2 || page.NextCursor == nil || page.NextCursor.StateVersion != 4 || page.NextCursor.ID != page.Transitions[1].ID {
		t.Fatalf("unexpected transition page: %#v", page)
	}
}

func TestStrategyExecutionPageIsOwnerScopedAndStable(t *testing.T) {
	now := time.Date(2026, 8, 28, 7, 0, 0, 0, time.UTC)
	after := &StrategyExecutionCursor{CreatedAt: now.Add(time.Minute), ID: "99999999-9999-4999-8999-999999999999"}
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		runtimeExecutions: []StrategyExecutionEvidence{
			{ID: "11111111-1111-4111-8111-111111111111", StrategyInstanceID: "instance-1", MandateVersion: 4, Mode: Shadow, Status: WouldHaveSubmitted, Symbol: "XRP", Instrument: "CRYPTO_SPOT", Side: "SELL", Quantity: "1", CreatedAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", StrategyInstanceID: "instance-1", MandateVersion: 4, Mode: Shadow, Status: RiskDenied, Symbol: "XRP", Instrument: "CRYPTO_SPOT", Side: "SELL", Quantity: "1", CreatedAt: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", StrategyInstanceID: "instance-1", MandateVersion: 4, Mode: Shadow, Status: WouldHaveSubmitted, Symbol: "XRP", Instrument: "CRYPTO_SPOT", Side: "BUY", Quantity: "1", CreatedAt: now.Add(-2 * time.Hour)},
		},
	}
	page, err := NewInstanceService(store, nil).ExecutionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, after)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.executionLimit != 3 || store.executionAfter != after {
		t.Fatalf("execution owner boundary or lookahead changed: %#v", store)
	}
	if len(page.Executions) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Executions[1].ID || !page.NextCursor.CreatedAt.Equal(page.Executions[1].CreatedAt) {
		t.Fatalf("unexpected execution page: %#v", page)
	}
}

func TestStrategyRuntimePagesFailClosedBeforeAndAcrossOwnerBoundary(t *testing.T) {
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	store := &journalPersistenceFake{instance: Instance{ID: "instance-1", UserID: "owner"}}
	service := NewInstanceService(store, nil)
	if _, err := service.TransitionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 16, nil); !errors.Is(err, ErrForbidden) || store.requestedID != "" {
		t.Fatalf("unentitled transition history reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.ExecutionPage(context.Background(), principal, "instance-1", 51, nil); !errors.Is(err, ErrInvalid) || store.requestedID != "" {
		t.Fatalf("invalid execution limit reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.TransitionPage(context.Background(), principal, "instance-1", 16, &StrategyTransitionCursor{}); !errors.Is(err, ErrInvalid) || store.requestedID != "" {
		t.Fatalf("invalid transition cursor reached persistence: id=%q err=%v", store.requestedID, err)
	}
	store.instance.ID = "different-instance"
	if _, err := service.ExecutionPage(context.Background(), principal, "instance-1", 16, nil); !errors.Is(err, ErrNotFound) || store.executionLimit != 0 {
		t.Fatalf("foreign execution history crossed its owner boundary: limit=%d err=%v", store.executionLimit, err)
	}
}

func TestStrategyExecutionPageRejectsUnknownPersistedStatus(t *testing.T) {
	now := time.Date(2026, 8, 28, 7, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		runtimeExecutions: []StrategyExecutionEvidence{{
			ID:                 "11111111-1111-4111-8111-111111111111",
			StrategyInstanceID: "instance-1",
			MandateVersion:     4,
			Mode:               Shadow,
			Status:             ExecutionStatus("LIVE_FILLED"),
			Symbol:             "XRP",
			Instrument:         "CRYPTO_SPOT",
			Side:               "SELL",
			Quantity:           "1",
			CreatedAt:          now,
		}},
	}

	_, err := NewInstanceService(store, nil).ExecutionPage(
		context.Background(),
		authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder},
		"instance-1",
		16,
		nil,
	)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown execution status did not fail closed: %v", err)
	}
}

func TestStrategyDecisionPageIsOwnerScopedAndReturnsOnlyMatchedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	firstExecution := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	secondExecution := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	after := &StrategyDecisionCursor{CreatedAt: now.Add(time.Minute), ID: "99999999-9999-4999-8999-999999999999"}
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		pageDecisions: []DecisionJournalEntry{
			{ID: "11111111-1111-4111-8111-111111111111", StrategyInstanceID: "instance-1", ExecutionRecordID: &firstExecution, CreatedAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", StrategyInstanceID: "instance-1", ExecutionRecordID: &secondExecution, CreatedAt: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", StrategyInstanceID: "instance-1", CreatedAt: now.Add(-2 * time.Hour)},
		},
		pageOutcomes: []ShadowOutcome{
			{ID: "44444444-4444-4444-8444-444444444444", ExecutionRecordID: firstExecution},
			{ID: "55555555-5555-4555-8555-555555555555", ExecutionRecordID: secondExecution},
		},
	}
	service := NewInstanceService(store, nil)
	page, err := service.DecisionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, after)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.pageLimit != 3 || store.pageAfter != after {
		t.Fatalf("decision-page owner boundary or lookahead changed: %#v", store)
	}
	if len(page.Decisions) != 2 || len(page.Outcomes) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Decisions[1].ID || !page.NextCursor.CreatedAt.Equal(page.Decisions[1].CreatedAt) {
		t.Fatalf("unexpected decision page: %#v", page)
	}
	if len(store.pageExecutionIDs) != 2 || store.pageExecutionIDs[0] != firstExecution || store.pageExecutionIDs[1] != secondExecution {
		t.Fatalf("outcomes were not restricted to the selected decision page: %#v", store.pageExecutionIDs)
	}
}

func TestStrategyDecisionPageFailsClosedBeforeOrAcrossItsOwnerBoundary(t *testing.T) {
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	store := &journalPersistenceFake{instance: Instance{ID: "instance-1", UserID: "owner"}}
	service := NewInstanceService(store, nil)
	if _, err := service.DecisionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 24, nil); !errors.Is(err, ErrForbidden) || store.requestedID != "" {
		t.Fatalf("unentitled decision history reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.DecisionPage(context.Background(), principal, "instance-1", 0, nil); !errors.Is(err, ErrInvalid) || store.requestedID != "" {
		t.Fatalf("invalid decision limit reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.DecisionPage(context.Background(), principal, "instance-1", 24, &StrategyDecisionCursor{}); !errors.Is(err, ErrInvalid) || store.requestedID != "" {
		t.Fatalf("invalid decision cursor reached persistence: id=%q err=%v", store.requestedID, err)
	}
	store.instance.ID = "different-instance"
	if _, err := service.DecisionPage(context.Background(), principal, "instance-1", 24, nil); !errors.Is(err, ErrNotFound) || store.pageLimit != 0 {
		t.Fatalf("foreign decision history crossed its owner boundary: limit=%d err=%v", store.pageLimit, err)
	}
}

func TestStrategyDecisionPageRejectsOutcomeOutsideSelectedExecutions(t *testing.T) {
	now := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	executionID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	store := &journalPersistenceFake{
		instance:      Instance{ID: "instance-1", UserID: "owner"},
		pageDecisions: []DecisionJournalEntry{{ID: "11111111-1111-4111-8111-111111111111", StrategyInstanceID: "instance-1", ExecutionRecordID: &executionID, CreatedAt: now}},
		pageOutcomes:  []ShadowOutcome{{ID: "22222222-2222-4222-8222-222222222222", ExecutionRecordID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}},
	}
	_, err := NewInstanceService(store, nil).DecisionPage(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 24, nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unmatched outcome evidence was accepted: %v", err)
	}
}

func TestScheduleRunsAreOwnerScopedAndBuildStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner"},
		scheduleRuns: []ScheduleRun{
			{ID: "11111111-1111-4111-8111-111111111111", ScheduledFor: now},
			{ID: "22222222-2222-4222-8222-222222222222", ScheduledFor: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", ScheduledFor: now.Add(-2 * time.Hour)},
		},
	}
	service := NewInstanceService(store, nil)
	page, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.requestedLimit != 3 {
		t.Fatalf("schedule-run owner boundary changed: %#v", store)
	}
	if len(page.Runs) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Runs[1].ID || !page.NextCursor.ScheduledFor.Equal(page.Runs[1].ScheduledFor) {
		t.Fatalf("unexpected schedule-run page: %#v", page)
	}
}

func TestScheduleRunsRequireAutomationEntitlementAndValidBounds(t *testing.T) {
	store := &journalPersistenceFake{instance: Instance{ID: "instance-1"}}
	service := NewInstanceService(store, nil)
	if _, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 20, nil); !errors.Is(err, ErrForbidden) || store.requestedID != "" {
		t.Fatalf("unentitled schedule-run request reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.ScheduleRuns(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 0, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid schedule-run limit was accepted: %v", err)
	}
}

func TestShadowEvidenceReviewsAreOwnerScopedAndBuildStableNextCursor(t *testing.T) {
	now := time.Date(2026, 8, 28, 5, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner", StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow},
		evidenceReviews: []ShadowEvidenceReview{
			{ID: "11111111-1111-4111-8111-111111111111", ReviewedAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", ReviewedAt: now.Add(-time.Hour)},
			{ID: "33333333-3333-4333-8333-333333333333", ReviewedAt: now.Add(-2 * time.Hour)},
		},
	}
	service := NewInstanceService(store, nil)
	page, err := service.ShadowEvidenceReviews(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.reviewLimit != 3 {
		t.Fatalf("review-ledger owner boundary changed: %#v", store)
	}
	if len(page.Reviews) != 2 || page.NextCursor == nil || page.NextCursor.ID != page.Reviews[1].ID || !page.NextCursor.ReviewedAt.Equal(page.Reviews[1].ReviewedAt) {
		t.Fatalf("unexpected review-ledger page: %#v", page)
	}
}

func TestShadowEvidenceReviewsRequireEntitlementAndAIShadowInstance(t *testing.T) {
	store := &journalPersistenceFake{instance: Instance{ID: "instance-1", StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow}}
	service := NewInstanceService(store, nil)
	if _, err := service.ShadowEvidenceReviews(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 8, nil); !errors.Is(err, ErrForbidden) || store.requestedID != "" {
		t.Fatalf("unentitled review-ledger request reached persistence: id=%q err=%v", store.requestedID, err)
	}
	if _, err := service.ShadowEvidenceReviews(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 0, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid review-ledger limit was accepted: %v", err)
	}
	store.instance.StrategyIdentifier = "wheel"
	if _, err := service.ShadowEvidenceReviews(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", 8, nil); !errors.Is(err, ErrInvalid) || store.reviewLimit != 0 {
		t.Fatalf("non-AI Shadow history was exposed: limit=%d err=%v", store.reviewLimit, err)
	}
}

func TestPaperPortfolioIsOwnerScopedAndPaperOnly(t *testing.T) {
	store := &journalPersistenceFake{
		instance:  Instance{ID: "instance-1", ExecutionMode: Paper},
		portfolio: PaperPortfolio{StrategyInstanceID: "instance-1", Currency: "USD", Cash: "20125.0000000000", Positions: []PaperPosition{}},
	}
	service := NewInstanceService(store, nil)
	portfolio, err := service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.StrategyInstanceID != "instance-1" || store.requestedUser != "owner" || store.requestedID != "instance-1" || store.portfolioCalls != 1 {
		t.Fatalf("paper portfolio boundary changed: portfolio=%#v store=%#v", portfolio, store)
	}

	store.instance.ExecutionMode = Shadow
	if _, err = service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("shadow portfolio was not rejected: %v", err)
	}
	if store.portfolioCalls != 1 {
		t.Fatal("shadow portfolio request reached the PAPER ledger")
	}

	if _, err = service.PaperPortfolio(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled portfolio request was accepted: %v", err)
	}
}

func TestAIPaperSpotFillsAreOwnerScopedPaperOnlyAndPaginated(t *testing.T) {
	now := time.Date(2026, 8, 28, 19, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", StrategyIdentifier: "ai_shadow", ExecutionMode: Paper},
		paperFills: []AIPaperSpotFill{
			{ID: "11111111-1111-4111-8111-111111111111", StrategyInstanceID: "instance-1", Symbol: "BTC", SimulationOnly: true, SimulatedAt: now},
			{ID: "22222222-2222-4222-8222-222222222222", StrategyInstanceID: "instance-1", Symbol: "ETH", SimulationOnly: true, SimulatedAt: now.Add(-time.Minute)},
		},
	}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	page, err := service.AIPaperSpotFills(context.Background(), principal, "instance-1", 1, nil)
	if err != nil || len(page.Fills) != 1 || page.Fills[0].Symbol != "BTC" || page.NextCursor == nil || page.NextCursor.ID != page.Fills[0].ID {
		t.Fatalf("AI Paper fill page changed: page=%#v err=%v", page, err)
	}
	if store.requestedUser != "owner" || store.requestedID != "instance-1" || store.paperFillLimit != 2 {
		t.Fatalf("AI Paper fill owner boundary changed: %#v", store)
	}

	store.instance.ExecutionMode = Shadow
	if _, err = service.AIPaperSpotFills(context.Background(), principal, "instance-1", 25, nil); !errors.Is(err, ErrInvalid) || store.paperFillLimit != 2 {
		t.Fatalf("Shadow instance reached AI Paper fills: limit=%d err=%v", store.paperFillLimit, err)
	}
	store.instance.ExecutionMode = Paper
	store.instance.StrategyIdentifier = "wheel"
	if _, err = service.AIPaperSpotFills(context.Background(), principal, "instance-1", 25, nil); !errors.Is(err, ErrInvalid) || store.paperFillLimit != 2 {
		t.Fatalf("non-AI Paper instance reached AI Paper fills: limit=%d err=%v", store.paperFillLimit, err)
	}
	if _, err = service.AIPaperSpotFills(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", 25, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled AI Paper fill request was accepted: %v", err)
	}
}

func TestShadowScorecardIsOwnerScopedAndEntitled(t *testing.T) {
	store := &journalPersistenceFake{
		instance: Instance{ID: "instance-1", UserID: "owner", AutomationMandateID: "mandate-1", MandateVersion: 4, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow, CurrentState: AIMonitoring},
		scorecard: ShadowScorecard{
			StrategyInstanceID: "instance-1", TotalMarks: 1,
			Horizons: []ShadowHorizonScore{{Horizon: ShadowOutcomeOneHour, SampleSize: 1}},
		}}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}
	scorecard, err := service.ShadowScorecard(context.Background(), principal, "instance-1")
	if err != nil || scorecard.TotalMarks != 1 || !shadowEvidenceFingerprintPattern.MatchString(scorecard.EvidenceReviewFingerprint) || store.requestedUser != "owner" || store.requestedID != "instance-1" || store.scorecardCalls != 1 {
		t.Fatalf("shadow scorecard owner boundary changed: scorecard=%#v store=%#v err=%v", scorecard, store, err)
	}
	if _, err = service.ShadowScorecard(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1"); !errors.Is(err, ErrForbidden) || store.scorecardCalls != 1 {
		t.Fatalf("unentitled shadow scorecard request reached persistence: calls=%d err=%v", store.scorecardCalls, err)
	}
}

func reviewableShadowScorecard(instanceID string) ShadowScorecard {
	first := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	last := first.Add(8 * 24 * time.Hour)
	return ShadowScorecard{
		StrategyInstanceID: instanceID,
		TotalMarks:         40,
		Horizons: []ShadowHorizonScore{
			{Horizon: ShadowOutcomeOneHour, SampleSize: 20, FirstEvaluatedAt: &first, LastEvaluatedAt: &last, Interpretation: "observational", MinimumSampleForObservationalLabel: 20},
			{Horizon: ShadowOutcomeTwentyFourHours, SampleSize: 20, FirstEvaluatedAt: &first, LastEvaluatedAt: &last, Interpretation: "observational", MinimumSampleForObservationalLabel: 20},
		},
		Behavior: ShadowBehaviorScore{TotalAIDecisions: 24, Abstentions: 4, ProposedDecisions: 20, Routes: []ShadowRouteBehavior{}, Symbols: []ShadowSymbolBehavior{}},
		EvidenceGate: ShadowEvidenceGate{
			Status:                      ShadowEvidenceReviewable,
			Blockers:                    []string{},
			OneHourSampleSize:           20,
			TwentyFourHourSampleSize:    20,
			MinimumSamplePerHorizon:     ShadowScorecardMinimumSample,
			EvidenceWindowHours:         192,
			MinimumEvidenceWindowHours:  ShadowEvidenceMinimumWindowHours,
			ScheduleHealthy:             true,
			LastScheduleStatus:          "SUCCEEDED",
			ConsecutiveScheduleFailures: 0,
			ExecutionBoundary:           ShadowExecutionBoundary,
			LiveExecutionAvailable:      false,
		},
	}
}

func TestRecordShadowEvidenceReviewRequiresExactReviewableSnapshotAndFreshTOTP(t *testing.T) {
	verifiedAt := time.Date(2026, 8, 28, 4, 30, 0, 0, time.UTC)
	instance := Instance{ID: "instance-1", UserID: "owner", AutomationMandateID: "mandate-1", MandateVersion: 4, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow, CurrentState: AIMonitoring, Status: "ACTIVE"}
	scorecard := reviewableShadowScorecard(instance.ID)
	fingerprint, err := shadowEvidenceFingerprint(instance, scorecard)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, scorecard: scorecard}
	audit := &strategyAuditFake{}
	stepUp := &evidenceReviewStepUpFake{method: "totp", verifiedAt: verifiedAt}
	service := NewInstanceService(store, nil, audit)
	service.ConfigureEvidenceReview(stepUp)

	review, err := service.RecordShadowEvidenceReview(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, instance.ID, ShadowEvidenceReviewCommand{
		EvidenceFingerprint:  fingerprint,
		ConfirmNonLiveReview: true,
		MFACode:              "123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.ID != "review-1" || review.EvidenceFingerprint != fingerprint || review.ReviewScope != ShadowEvidenceReviewScope || review.ExecutionBoundary != ShadowExecutionBoundary || review.LiveExecutionAvailable || !review.ReviewedAt.Equal(verifiedAt) {
		t.Fatalf("unexpected immutable review: %#v", review)
	}
	if stepUp.calls != 1 || stepUp.userID != "owner" || stepUp.code != "123456" || store.createReviewCalls != 1 || store.createdReview.MandateID != "mandate-1" || store.createdReview.MandateVersion != 4 {
		t.Fatalf("review boundary was not preserved: step-up=%#v store=%#v", stepUp, store)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.shadow_evidence_reviewed" || audit.metadata["evidence_fingerprint"] != fingerprint || audit.metadata["live_execution_available"] != false || audit.metadata["broker_order_created"] != false || audit.metadata["execution_authority_granted"] != false {
		t.Fatalf("review audit evidence is incomplete: %#v", audit)
	}
}

func TestRecordShadowEvidenceReviewFailsClosedBeforeConsumingTOTP(t *testing.T) {
	instance := Instance{ID: "instance-1", UserID: "owner", AutomationMandateID: "mandate-1", MandateVersion: 4, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow, CurrentState: AIMonitoring, Status: "ACTIVE"}
	scorecard := reviewableShadowScorecard(instance.ID)
	fingerprint, err := shadowEvidenceFingerprint(instance, scorecard)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, scorecard: scorecard}
	stepUp := &evidenceReviewStepUpFake{method: "totp", verifiedAt: time.Now().UTC()}
	service := NewInstanceService(store, nil)
	service.ConfigureEvidenceReview(stepUp)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	if _, err = service.RecordShadowEvidenceReview(context.Background(), principal, instance.ID, ShadowEvidenceReviewCommand{EvidenceFingerprint: "b" + fingerprint[1:], ConfirmNonLiveReview: true, MFACode: "123456"}); !errors.Is(err, ErrEvidenceSnapshotChanged) {
		t.Fatalf("stale snapshot returned %v", err)
	}
	if stepUp.calls != 0 || store.createReviewCalls != 0 {
		t.Fatal("stale snapshot consumed MFA or reached persistence")
	}
	store.scorecard.EvidenceGate.Status = "COLLECTING_EVIDENCE"
	store.scorecard.EvidenceGate.Blockers = []string{"ONE_HOUR_SAMPLE_INCOMPLETE"}
	if _, err = service.RecordShadowEvidenceReview(context.Background(), principal, instance.ID, ShadowEvidenceReviewCommand{EvidenceFingerprint: fingerprint, ConfirmNonLiveReview: true, MFACode: "123456"}); !errors.Is(err, ErrEvidenceNotReviewable) {
		t.Fatalf("collecting evidence returned %v", err)
	}
	if stepUp.calls != 0 || store.createReviewCalls != 0 {
		t.Fatal("collecting evidence consumed MFA or reached persistence")
	}
	store.scorecard = scorecard
	stepUp.err = errors.New("rejected")
	if _, err = service.RecordShadowEvidenceReview(context.Background(), principal, instance.ID, ShadowEvidenceReviewCommand{EvidenceFingerprint: fingerprint, ConfirmNonLiveReview: true, MFACode: "000000"}); !errors.Is(err, ErrEvidenceReviewStepUp) {
		t.Fatalf("rejected MFA returned %v", err)
	}
	if stepUp.calls != 1 || store.createReviewCalls != 0 {
		t.Fatal("rejected MFA reached review persistence")
	}
	if _, err = service.RecordShadowEvidenceReview(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, instance.ID, ShadowEvidenceReviewCommand{EvidenceFingerprint: fingerprint, ConfirmNonLiveReview: true, MFACode: "123456"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled review returned %v", err)
	}
}

func TestShadowScorecardMarksOnlyTheExactCurrentSnapshotReviewed(t *testing.T) {
	instance := Instance{ID: "instance-1", UserID: "owner", AutomationMandateID: "mandate-1", MandateVersion: 4, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow, CurrentState: AIMonitoring}
	scorecard := reviewableShadowScorecard(instance.ID)
	fingerprint, err := shadowEvidenceFingerprint(instance, scorecard)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, scorecard: scorecard, latestReview: &ShadowEvidenceReview{ID: "review-1", EvidenceFingerprint: fingerprint}}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	current, err := service.ShadowScorecard(context.Background(), principal, instance.ID)
	if err != nil || !current.CurrentEvidenceReviewed || current.LatestEvidenceReview == nil {
		t.Fatalf("current review was not attached: %#v %v", current, err)
	}
	store.scorecard.TotalMarks++
	changed, err := service.ShadowScorecard(context.Background(), principal, instance.ID)
	if err != nil || changed.CurrentEvidenceReviewed || changed.EvidenceReviewFingerprint == fingerprint || changed.LatestEvidenceReview == nil {
		t.Fatalf("changed evidence reused an old review: %#v %v", changed, err)
	}
}

func TestRecordShadowEvidenceReviewIsIdempotentForAnAlreadyReviewedFingerprint(t *testing.T) {
	instance := Instance{ID: "instance-1", UserID: "owner", AutomationMandateID: "mandate-1", MandateVersion: 4, StrategyIdentifier: "ai_shadow", ExecutionMode: Shadow, CurrentState: AIMonitoring}
	scorecard := reviewableShadowScorecard(instance.ID)
	fingerprint, err := shadowEvidenceFingerprint(instance, scorecard)
	if err != nil {
		t.Fatal(err)
	}
	existing := &ShadowEvidenceReview{ID: "review-1", StrategyInstanceID: instance.ID, EvidenceFingerprint: fingerprint, ReviewScope: ShadowEvidenceReviewScope}
	store := &journalPersistenceFake{instance: instance, scorecard: scorecard, latestReview: existing}
	stepUp := &evidenceReviewStepUpFake{err: errors.New("must not be called")}
	service := NewInstanceService(store, nil)
	service.ConfigureEvidenceReview(stepUp)

	review, err := service.RecordShadowEvidenceReview(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, instance.ID, ShadowEvidenceReviewCommand{EvidenceFingerprint: fingerprint, ConfirmNonLiveReview: true})
	if err != nil || review.ID != existing.ID || stepUp.calls != 0 || store.createReviewCalls != 0 {
		t.Fatalf("current review was not idempotent: review=%#v step-up=%#v store=%#v err=%v", review, stepUp, store, err)
	}
}

func reviewablePaperEvidenceFixture() (Instance, PaperPortfolio) {
	asOf := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	startedAt := asOf.Add(-169 * time.Hour)
	runs, decisions := paperEvidenceFixture(asOf, 20, startedAt)
	portfolio := paperEvidencePortfolio()
	portfolio.StrategyInstanceID = "instance"
	portfolio.Version = 9
	portfolio.UpdatedAt = asOf
	portfolio.EvidenceReadiness = projectPaperAutonomyEvidenceGate(startedAt, "SUCCEEDED", 0, runs, decisions, portfolio, paperAutonomySafetyCounts{})
	instance := Instance{
		ID: "instance", UserID: "owner", AutomationMandateID: "mandate-1", FinancialAccountID: "account-1",
		MandateVersion: 6, StrategyIdentifier: "ai_shadow", ExecutionMode: Paper, CurrentState: AIMonitoring, Status: "ACTIVE", StartedAt: startedAt,
	}
	return instance, portfolio
}

func TestRecordPaperEvidenceReviewPinsExactGateCheckpointAndFreshTOTP(t *testing.T) {
	verifiedAt := time.Date(2026, 9, 5, 12, 5, 0, 0, time.UTC)
	instance, portfolio := reviewablePaperEvidenceFixture()
	fingerprint, err := paperEvidenceFingerprint(instance, portfolio)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, portfolio: portfolio}
	audit := &strategyAuditFake{}
	stepUp := &evidenceReviewStepUpFake{method: "totp", verifiedAt: verifiedAt}
	service := NewInstanceService(store, nil, audit)
	service.ConfigureEvidenceReview(stepUp)

	review, err := service.RecordPaperEvidenceReview(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, instance.ID, PaperEvidenceReviewCommand{
		EvidenceFingerprint: fingerprint, ConfirmPaperReview: true, MFACode: "123456",
	})
	if err != nil {
		t.Fatal(err)
	}
	latestCheckpoint := portfolio.EvidenceReadiness.ReviewPacket.ThresholdChangeLedger.Checkpoints[len(portfolio.EvidenceReadiness.ReviewPacket.ThresholdChangeLedger.Checkpoints)-1]
	if review.ID != "paper-review-1" || review.EvidenceFingerprint != fingerprint || review.LatestCheckpointRunID != latestCheckpoint.ScheduleRunID || !review.LatestCheckpointAsOf.Equal(latestCheckpoint.AsOf) || review.FinancialAccountID != instance.FinancialAccountID || review.ReviewScope != PaperEvidenceReviewScope || review.ExecutionBoundary != PaperAutonomyEvidenceBoundary || review.GrantsAuthority || review.LivePromotionAvailable || !review.ReviewedAt.Equal(verifiedAt) {
		t.Fatalf("unexpected immutable Paper review: %#v", review)
	}
	if stepUp.calls != 1 || stepUp.userID != "owner" || stepUp.code != "123456" || store.createPaperReviewCalls != 1 || store.createdPaperReview.MandateVersion != 6 {
		t.Fatalf("Paper review boundary was not preserved: step-up=%#v store=%#v", stepUp, store)
	}
	if audit.userID == nil || *audit.userID != "owner" || audit.action != "strategy_instance.paper_evidence_reviewed" || audit.metadata["latest_checkpoint_run_id"] != latestCheckpoint.ScheduleRunID || audit.metadata["grants_authority"] != false || audit.metadata["live_promotion_available"] != false || audit.metadata["broker_order_created"] != false {
		t.Fatalf("Paper review audit evidence is incomplete: %#v", audit)
	}
}

func TestRecordPaperEvidenceReviewFailsClosedBeforeConsumingTOTP(t *testing.T) {
	instance, portfolio := reviewablePaperEvidenceFixture()
	fingerprint, err := paperEvidenceFingerprint(instance, portfolio)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, portfolio: portfolio}
	stepUp := &evidenceReviewStepUpFake{method: "totp", verifiedAt: time.Now().UTC()}
	service := NewInstanceService(store, nil)
	service.ConfigureEvidenceReview(stepUp)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	if _, err = service.RecordPaperEvidenceReview(context.Background(), principal, instance.ID, PaperEvidenceReviewCommand{EvidenceFingerprint: "b" + fingerprint[1:], ConfirmPaperReview: true, MFACode: "123456"}); !errors.Is(err, ErrEvidenceSnapshotChanged) {
		t.Fatalf("stale Paper snapshot returned %v", err)
	}
	if stepUp.calls != 0 || store.createPaperReviewCalls != 0 {
		t.Fatal("stale Paper snapshot consumed MFA or reached persistence")
	}
	store.portfolio.EvidenceReadiness.Status = PaperAutonomyEvidenceCollecting
	store.portfolio.EvidenceReadiness.ReviewPacket.Status = PaperAutonomyEvidenceCollecting
	store.portfolio.EvidenceReadiness.ReviewPacket.EvidenceReadyForHumanReview = false
	if _, err = service.RecordPaperEvidenceReview(context.Background(), principal, instance.ID, PaperEvidenceReviewCommand{EvidenceFingerprint: fingerprint, ConfirmPaperReview: true, MFACode: "123456"}); !errors.Is(err, ErrEvidenceNotReviewable) {
		t.Fatalf("collecting Paper evidence returned %v", err)
	}
	if stepUp.calls != 0 || store.createPaperReviewCalls != 0 {
		t.Fatal("collecting Paper evidence consumed MFA or reached persistence")
	}
}

func TestPaperPortfolioMarksOnlyExactCurrentEvidenceReviewed(t *testing.T) {
	instance, portfolio := reviewablePaperEvidenceFixture()
	fingerprint, err := paperEvidenceFingerprint(instance, portfolio)
	if err != nil {
		t.Fatal(err)
	}
	store := &journalPersistenceFake{instance: instance, portfolio: portfolio, latestPaperReview: &PaperEvidenceReview{ID: "review-1", EvidenceFingerprint: fingerprint}}
	service := NewInstanceService(store, nil)
	principal := authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}

	current, err := service.PaperPortfolio(context.Background(), principal, instance.ID)
	if err != nil || !current.CurrentEvidenceReviewed || current.LatestEvidenceReview == nil {
		t.Fatalf("current Paper review was not attached: %#v %v", current, err)
	}
	store.portfolio.Version++
	changed, err := service.PaperPortfolio(context.Background(), principal, instance.ID)
	if err != nil || changed.CurrentEvidenceReviewed || changed.EvidenceReviewFingerprint == fingerprint || changed.LatestEvidenceReview == nil {
		t.Fatalf("changed Paper evidence reused an old review: %#v %v", changed, err)
	}
}

func TestRecordLifecycleRequiresExplicitPaperConfirmationAndOwnerEntitlement(t *testing.T) {
	store := &journalPersistenceFake{}
	service := NewInstanceService(store, nil)
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: Assignment, ExpectedStateVersion: 2}
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unconfirmed paper event was accepted: %v", err)
	}
	command.ConfirmPaperSimulation = true
	if _, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFree}, "instance-1", command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unentitled paper event was accepted: %v", err)
	}
	if store.lifecycleID != "" {
		t.Fatal("rejected lifecycle command reached persistence")
	}
}

func TestRecordLifecycleIsOwnerScopedAndUsesServerTime(t *testing.T) {
	now := time.Date(2026, 8, 18, 20, 0, 0, 0, time.UTC)
	store := &journalPersistenceFake{lifecycleResult: LifecycleResult{ID: "event-record-1", EventType: ExpireWorthless}}
	service := NewInstanceService(store, nil)
	service.now = func() time.Time { return now }
	command := LifecycleCommand{EventID: "manual-lifecycle:event-1", EventType: ExpireWorthless, ExpectedStateVersion: 2, ConfirmPaperSimulation: true}
	result, err := service.RecordLifecycle(context.Background(), authorization.Principal{UserID: "owner", Entitlement: authorization.EntitlementFounder}, "instance-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "event-record-1" || store.requestedUser != "owner" || store.lifecycleID != "instance-1" || store.lifecycle != command || !store.lifecycleAt.Equal(now) {
		t.Fatalf("lifecycle command boundary changed: result=%#v store=%#v", result, store)
	}
}
