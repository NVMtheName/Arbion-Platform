package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/automation"
)

var (
	ErrForbidden               = errors.New("strategy entitlement required")
	ErrNotFound                = errors.New("strategy instance not found")
	ErrConflict                = errors.New("strategy instance conflict")
	ErrCapitalLimit            = errors.New("paper starting cash exceeds capital bucket capacity")
	ErrAccountInUse            = errors.New("requested capital overlaps an active non-live reservation")
	ErrCapitalReservation      = errors.New("capital bucket cannot establish an exact non-live reservation")
	ErrOpenExposure            = errors.New("paper strategy still has open simulated positions")
	ErrMandateStale            = errors.New("strategy mandate is not current and ready")
	ErrEvidenceNotReviewable   = errors.New("non-live evidence is not reviewable")
	ErrEvidenceSnapshotChanged = errors.New("non-live evidence snapshot changed")
	ErrEvidenceReviewStepUp    = errors.New("fresh authenticator code required for non-live evidence review")
)

type Persistence interface {
	Initialize(context.Context, string, automation.Mandate, string, State) (Instance, error)
	Pause(context.Context, string, string, int, time.Time) (Instance, error)
	Resume(context.Context, string, string, int, time.Time) (Instance, error)
	Finish(context.Context, string, string, int, time.Time) (Instance, error)
	List(context.Context, string) ([]Instance, error)
	Get(context.Context, string, string) (Instance, error)
	PaperPortfolio(context.Context, string, string) (PaperPortfolio, error)
	Journal(context.Context, string, int, *JournalCursor) ([]JournalActivity, error)
	Schedule(context.Context, string, string) (ScheduleStatus, error)
	RecordLifecycle(context.Context, string, string, LifecycleCommand, time.Time) (LifecycleResult, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
type Mandates interface {
	Get(context.Context, authorization.Principal, string) (automation.Mandate, error)
	GetBucket(context.Context, authorization.Principal, string) (automation.CapitalBucket, error)
}

type ShadowOutcomeReader interface {
	ShadowOutcomes(context.Context, string, string) ([]ShadowOutcome, error)
}
type ShadowScorecardReader interface {
	ShadowScorecard(context.Context, string, string) (ShadowScorecard, error)
}
type ShadowEvidenceReviewStore interface {
	CreateShadowEvidenceReview(context.Context, string, ShadowEvidenceReview) (ShadowEvidenceReview, error)
	LatestShadowEvidenceReview(context.Context, string, string) (*ShadowEvidenceReview, error)
}
type ShadowEvidenceReviewReader interface {
	ShadowEvidenceReviews(context.Context, string, string, int, *ShadowEvidenceReviewCursor) ([]ShadowEvidenceReview, error)
}
type ShadowEvidenceReviewStepUp interface {
	VerifyShadowEvidenceReviewStepUp(context.Context, string, string) (string, time.Time, error)
	VerifyPaperEvidenceReviewStepUp(context.Context, string, string) (string, time.Time, error)
}
type PaperEvidenceReviewStore interface {
	CreatePaperEvidenceReview(context.Context, string, PaperEvidenceReview) (PaperEvidenceReview, error)
	LatestPaperEvidenceReview(context.Context, string, string) (*PaperEvidenceReview, error)
}
type PaperEvidenceReviewReader interface {
	PaperEvidenceReviews(context.Context, string, string, int, *PaperEvidenceReviewCursor) ([]PaperEvidenceReview, error)
}
type ScheduleRunReader interface {
	ScheduleRuns(context.Context, string, string, int, *ScheduleRunCursor) ([]ScheduleRun, error)
}
type StrategyTransitionEvidence struct {
	ID                 string    `json:"id"`
	StrategyInstanceID string    `json:"-"`
	PreviousState      State     `json:"previous_state"`
	NewState           State     `json:"new_state"`
	StateVersion       int       `json:"state_version"`
	Trigger            string    `json:"trigger"`
	OccurredAt         time.Time `json:"occurred_at"`
}
type StrategyTransitionCursor struct {
	StateVersion int
	ID           string
}
type StrategyTransitionPage struct {
	Transitions []StrategyTransitionEvidence
	NextCursor  *StrategyTransitionCursor
}
type StrategyExecutionEvidence struct {
	ID                 string          `json:"id"`
	StrategyInstanceID string          `json:"-"`
	MandateVersion     int             `json:"mandate_version"`
	Mode               ExecutionMode   `json:"mode"`
	Status             ExecutionStatus `json:"status"`
	Symbol             string          `json:"symbol"`
	Instrument         string          `json:"instrument"`
	Side               string          `json:"side"`
	Quantity           string          `json:"quantity"`
	Price              *string         `json:"price,omitempty"`
	Notional           *string         `json:"notional,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
}
type StrategyExecutionCursor struct {
	CreatedAt time.Time
	ID        string
}
type StrategyExecutionPage struct {
	Executions []StrategyExecutionEvidence
	NextCursor *StrategyExecutionCursor
}
type StrategyRuntimeHistoryReader interface {
	StrategyTransitionEntries(context.Context, string, string, int, *StrategyTransitionCursor) ([]StrategyTransitionEvidence, error)
	StrategyExecutionEntries(context.Context, string, string, int, *StrategyExecutionCursor) ([]StrategyExecutionEvidence, error)
}
type CapitalReservationReader interface {
	CapitalReservation(context.Context, string, string) (CapitalReservation, error)
}
type CapitalReservationListReader interface {
	CapitalReservations(context.Context, string) ([]CapitalReservation, error)
}
type AIPaperSpotFillReader interface {
	AIPaperSpotFills(context.Context, string, string, int, *AIPaperSpotFillCursor) ([]AIPaperSpotFill, error)
}
type DecisionJournalEntry struct {
	ID, StrategyInstanceID, StrategyState, Source, DecisionType           string
	StructuredRationale                                                   json.RawMessage
	ProposedActionID, RiskEvaluationID, ExecutionRecordID, ResultingState *string
	RiskDecision                                                          *string
	ApprovalRequired                                                      *bool
	RiskReasonCodes, RiskChecks                                           json.RawMessage
	ExecutionStatus, Symbol, Instrument, Side, Quantity, Price, Notional  *string
	CreatedAt                                                             time.Time
}
type StrategyDecisionCursor struct {
	CreatedAt time.Time
	ID        string
}
type StrategyDecisionPage struct {
	Decisions  []DecisionJournalEntry
	Outcomes   []ShadowOutcome
	NextCursor *StrategyDecisionCursor
}
type StrategyDecisionPageReader interface {
	StrategyDecisionEntries(context.Context, string, string, int, *StrategyDecisionCursor) ([]DecisionJournalEntry, error)
	ShadowOutcomesForExecutions(context.Context, string, string, []string) ([]ShadowOutcome, error)
}
type InstanceService struct {
	store                Persistence
	mandates             Mandates
	audit                Auditor
	evidenceReviewStepUp ShadowEvidenceReviewStepUp
	now                  func() time.Time
}

func (s *InstanceService) ConfigureEvidenceReview(stepUp ShadowEvidenceReviewStepUp) {
	s.evidenceReviewStepUp = stepUp
}

func NewInstanceService(s Persistence, m Mandates, auditors ...Auditor) *InstanceService {
	var audit Auditor
	if len(auditors) > 0 {
		audit = auditors[0]
	}
	return &InstanceService{store: s, mandates: m, audit: audit, now: func() time.Time { return time.Now().UTC() }}
}
func entitled(p authorization.Principal) bool {
	return authorization.CanUseAutomation(p) && authorization.CanConnectFinancialAccounts(p)
}

var paperCashPattern = regexp.MustCompile(`^\d+(\.\d{1,10})?$`)

func paperCashCapacity(bucket automation.CapitalBucket) (*big.Rat, bool) {
	if bucket.Status != "ACTIVE" || bucket.IsReserve {
		return nil, false
	}
	allocation, ok := new(big.Rat).SetString(bucket.AllocationValue)
	if !ok || allocation.Sign() <= 0 {
		return nil, false
	}
	capacity := new(big.Rat).Set(allocation)
	if bucket.AllocationType != "FIXED_AMOUNT" {
		if bucket.AllocationLimit == nil {
			return nil, false
		}
		capacity, ok = new(big.Rat).SetString(*bucket.AllocationLimit)
		if !ok || capacity.Sign() <= 0 {
			return nil, false
		}
	} else if bucket.AllocationLimit != nil {
		limit, valid := new(big.Rat).SetString(*bucket.AllocationLimit)
		if !valid || limit.Sign() <= 0 {
			return nil, false
		}
		if limit.Cmp(capacity) < 0 {
			capacity = limit
		}
	}
	protected, ok := new(big.Rat).SetString(bucket.ProtectedAmount)
	if !ok || protected.Sign() < 0 {
		return nil, false
	}
	capacity = new(big.Rat).Sub(capacity, protected)
	return capacity, capacity.Sign() > 0
}

func reservationClaim(bucket automation.CapitalBucket, mode ExecutionMode, startingCash string) (capitalReservationClaim, error) {
	claim := capitalReservationClaim{Currency: bucket.Currency}
	if bucket.Status != "ACTIVE" || bucket.IsReserve || len(bucket.Currency) != 3 {
		return claim, ErrCapitalReservation
	}
	if bucket.AllocationType == "FIXED_AMOUNT" && bucket.AllocationLimit != nil {
		limit, ok := new(big.Rat).SetString(*bucket.AllocationLimit)
		if !ok || limit.Sign() <= 0 {
			return claim, ErrCapitalReservation
		}
		canonical := limit.FloatString(10)
		claim.AccountAllocationLimit = &canonical
	}
	if mode == Paper {
		amount, ok := new(big.Rat).SetString(startingCash)
		if !ok || amount.Sign() <= 0 {
			return claim, ErrCapitalReservation
		}
		claim.Amount = amount.FloatString(10)
		claim.Basis = "PAPER_STARTING_CASH"
		return claim, nil
	}
	if mode != Shadow {
		return claim, ErrCapitalReservation
	}
	capacity, available := paperCashCapacity(bucket)
	if !available {
		return claim, ErrCapitalReservation
	}
	claim.Amount = capacity.FloatString(10)
	if bucket.AllocationType == "FIXED_AMOUNT" {
		claim.Basis = "BUCKET_FIXED_CAPACITY"
	} else {
		claim.Basis = "BUCKET_ABSOLUTE_LIMIT"
	}
	return claim, nil
}

func (s *InstanceService) Initialize(ctx context.Context, p authorization.Principal, mandateID, startingCash string) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	m, e := s.mandates.Get(ctx, p, mandateID)
	if e != nil {
		return Instance{}, ErrNotFound
	}
	if m.AutomationType == "HYBRID" {
		return Instance{}, ErrInvalid
	}
	if m.AutomationType == "AI_AUTONOMOUS" {
		if m.StrategyIdentifier != nil || m.AIProviderConnectionID == nil || m.AIModelID == nil || m.Status != "READY" || (m.ExecutionMode != "PAPER" && m.ExecutionMode != "SHADOW") || m.AutonomyLevel != "FULL_AUTONOMOUS" {
			return Instance{}, ErrInvalid
		}
		if _, e = automation.ParseAIShadowParameters(m.StrategyParameters); e != nil {
			return Instance{}, ErrInvalid
		}
		bucket, bucketErr := s.mandates.GetBucket(ctx, p, m.CapitalBucketID)
		if bucketErr != nil || bucket.UserID != p.UserID || bucket.FinancialAccountID != m.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
			return Instance{}, ErrInvalid
		}
		if m.ExecutionMode == "PAPER" {
			if !paperCashPattern.MatchString(startingCash) {
				return Instance{}, ErrInvalid
			}
			amount, ok := new(big.Rat).SetString(startingCash)
			if !ok || amount.Sign() <= 0 {
				return Instance{}, ErrInvalid
			}
			capacity, available := paperCashCapacity(bucket)
			if !available || amount.Cmp(capacity) > 0 {
				return Instance{}, ErrCapitalLimit
			}
		} else {
			startingCash = ""
		}
		if _, claimErr := reservationClaim(bucket, ExecutionMode(m.ExecutionMode), startingCash); claimErr != nil {
			return Instance{}, claimErr
		}
		return s.store.Initialize(ctx, p.UserID, m, startingCash, AIMonitoring)
	}
	if m.AutomationType != "STRATEGY" || m.StrategyIdentifier == nil || m.Status != "READY" || (*m.StrategyIdentifier != "wheel" && *m.StrategyIdentifier != "covered_call" && *m.StrategyIdentifier != "cash_secured_put") {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode != "PAPER" && m.ExecutionMode != "SHADOW" {
		return Instance{}, ErrInvalid
	}
	bucket, e := s.mandates.GetBucket(ctx, p, m.CapitalBucketID)
	if e != nil || bucket.UserID != p.UserID || bucket.FinancialAccountID != m.FinancialAccountID || bucket.Status != "ACTIVE" || bucket.IsReserve {
		return Instance{}, ErrInvalid
	}
	if m.ExecutionMode == "PAPER" {
		if !paperCashPattern.MatchString(startingCash) {
			return Instance{}, ErrInvalid
		}
		x, ok := new(big.Rat).SetString(startingCash)
		if !ok || x.Sign() <= 0 {
			return Instance{}, ErrInvalid
		}
		capacity, available := paperCashCapacity(bucket)
		if !available || x.Cmp(capacity) > 0 {
			return Instance{}, ErrCapitalLimit
		}
	} else {
		startingCash = ""
	}
	if _, claimErr := reservationClaim(bucket, ExecutionMode(m.ExecutionMode), startingCash); claimErr != nil {
		return Instance{}, claimErr
	}
	definition := automation.Strategies[*m.StrategyIdentifier]
	return s.store.Initialize(ctx, p.UserID, m, startingCash, State(definition.InitialState))
}
func (s *InstanceService) List(c context.Context, p authorization.Principal) ([]Instance, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	return s.store.List(c, p.UserID)
}
func (s *InstanceService) Get(c context.Context, p authorization.Principal, id string) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	return s.store.Get(c, p.UserID, id)
}

func (s *InstanceService) CapitalReservation(c context.Context, p authorization.Principal, id string) (CapitalReservation, error) {
	if !entitled(p) {
		return CapitalReservation{}, ErrForbidden
	}
	if id == "" {
		return CapitalReservation{}, ErrInvalid
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil || instance.ID != id || instance.UserID != p.UserID {
		return CapitalReservation{}, ErrNotFound
	}
	reader, ok := s.store.(CapitalReservationReader)
	if !ok {
		return CapitalReservation{}, ErrNotFound
	}
	reservation, err := reader.CapitalReservation(c, p.UserID, id)
	if err != nil {
		return CapitalReservation{}, ErrNotFound
	}
	return reservation, nil
}

func (s *InstanceService) CapitalReservations(c context.Context, p authorization.Principal) ([]CapitalReservation, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	reader, ok := s.store.(CapitalReservationListReader)
	if !ok {
		return nil, ErrNotFound
	}
	return reader.CapitalReservations(c, p.UserID)
}

func (s *InstanceService) Pause(c context.Context, p authorization.Principal, id string, expectedStateVersion int) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Pause(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "paused")
	return instance, nil
}

func (s *InstanceService) Resume(c context.Context, p authorization.Principal, id string, expectedStateVersion int, confirmed bool) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 || !confirmed {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Resume(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "resumed")
	return instance, nil
}

func (s *InstanceService) Finish(c context.Context, p authorization.Principal, id string, expectedStateVersion int, confirmed bool) (Instance, error) {
	if !entitled(p) {
		return Instance{}, ErrForbidden
	}
	if id == "" || expectedStateVersion < 1 || !confirmed {
		return Instance{}, ErrInvalid
	}
	instance, err := s.store.Finish(c, p.UserID, id, expectedStateVersion, s.now().UTC())
	if err != nil {
		return Instance{}, err
	}
	s.auditInstanceStatus(c, p.UserID, instance, "completed")
	return instance, nil
}

func (s *InstanceService) auditInstanceStatus(c context.Context, userID string, instance Instance, action string) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(c, &userID, "strategy_instance."+action, map[string]any{
		"strategy_instance_id": instance.ID,
		"mandate_id":           instance.AutomationMandateID,
		"account_id":           instance.FinancialAccountID,
		"mode":                 instance.ExecutionMode,
		"source":               "UI",
	})
}
func (s *InstanceService) TransitionPage(c context.Context, p authorization.Principal, id string, limit int, after *StrategyTransitionCursor) (StrategyTransitionPage, error) {
	if !entitled(p) {
		return StrategyTransitionPage{}, ErrForbidden
	}
	if id == "" || limit < 1 || limit > 50 || (after != nil && (after.StateVersion < 1 || after.ID == "")) {
		return StrategyTransitionPage{}, ErrInvalid
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil || instance.ID != id {
		return StrategyTransitionPage{}, ErrNotFound
	}
	reader, ok := s.store.(StrategyRuntimeHistoryReader)
	if !ok {
		return StrategyTransitionPage{}, ErrNotFound
	}
	transitions, err := reader.StrategyTransitionEntries(c, p.UserID, id, limit+1, after)
	if err != nil {
		return StrategyTransitionPage{}, err
	}
	if len(transitions) > limit+1 {
		return StrategyTransitionPage{}, ErrInvalid
	}
	page := StrategyTransitionPage{Transitions: transitions}
	if len(transitions) > limit {
		page.Transitions = transitions[:limit]
		last := page.Transitions[len(page.Transitions)-1]
		page.NextCursor = &StrategyTransitionCursor{StateVersion: last.StateVersion, ID: last.ID}
	}
	for _, transition := range page.Transitions {
		if transition.ID == "" || transition.StrategyInstanceID != id || transition.StateVersion < 1 || transition.PreviousState == "" || transition.NewState == "" || transition.Trigger == "" || transition.OccurredAt.IsZero() {
			return StrategyTransitionPage{}, ErrInvalid
		}
	}
	return page, nil
}
func (s *InstanceService) DecisionPage(c context.Context, p authorization.Principal, id string, limit int, after *StrategyDecisionCursor) (StrategyDecisionPage, error) {
	if !entitled(p) {
		return StrategyDecisionPage{}, ErrForbidden
	}
	if id == "" || limit < 1 || limit > 50 || (after != nil && (after.CreatedAt.IsZero() || after.ID == "")) {
		return StrategyDecisionPage{}, ErrInvalid
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil || instance.ID != id {
		return StrategyDecisionPage{}, ErrNotFound
	}
	reader, ok := s.store.(StrategyDecisionPageReader)
	if !ok {
		return StrategyDecisionPage{}, ErrNotFound
	}
	decisions, err := reader.StrategyDecisionEntries(c, p.UserID, id, limit+1, after)
	if err != nil {
		return StrategyDecisionPage{}, err
	}
	if len(decisions) > limit+1 {
		return StrategyDecisionPage{}, ErrInvalid
	}
	page := StrategyDecisionPage{Decisions: decisions, Outcomes: []ShadowOutcome{}}
	if len(decisions) > limit {
		page.Decisions = decisions[:limit]
		last := page.Decisions[len(page.Decisions)-1]
		page.NextCursor = &StrategyDecisionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	executionIDs := make([]string, 0, len(page.Decisions))
	allowedExecutions := make(map[string]struct{}, len(page.Decisions))
	for _, decision := range page.Decisions {
		if decision.ID == "" || decision.StrategyInstanceID != id || decision.CreatedAt.IsZero() {
			return StrategyDecisionPage{}, ErrInvalid
		}
		if decision.ExecutionRecordID != nil && *decision.ExecutionRecordID != "" {
			if _, exists := allowedExecutions[*decision.ExecutionRecordID]; !exists {
				allowedExecutions[*decision.ExecutionRecordID] = struct{}{}
				executionIDs = append(executionIDs, *decision.ExecutionRecordID)
			}
		}
	}
	if len(executionIDs) == 0 {
		return page, nil
	}
	page.Outcomes, err = reader.ShadowOutcomesForExecutions(c, p.UserID, id, executionIDs)
	if err != nil {
		return StrategyDecisionPage{}, err
	}
	for _, outcome := range page.Outcomes {
		if _, allowed := allowedExecutions[outcome.ExecutionRecordID]; !allowed {
			return StrategyDecisionPage{}, ErrInvalid
		}
	}
	return page, nil
}
func (s *InstanceService) ExecutionPage(c context.Context, p authorization.Principal, id string, limit int, after *StrategyExecutionCursor) (StrategyExecutionPage, error) {
	if !entitled(p) {
		return StrategyExecutionPage{}, ErrForbidden
	}
	if id == "" || limit < 1 || limit > 50 || (after != nil && (after.CreatedAt.IsZero() || after.ID == "")) {
		return StrategyExecutionPage{}, ErrInvalid
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil || instance.ID != id {
		return StrategyExecutionPage{}, ErrNotFound
	}
	reader, ok := s.store.(StrategyRuntimeHistoryReader)
	if !ok {
		return StrategyExecutionPage{}, ErrNotFound
	}
	executions, err := reader.StrategyExecutionEntries(c, p.UserID, id, limit+1, after)
	if err != nil {
		return StrategyExecutionPage{}, err
	}
	if len(executions) > limit+1 {
		return StrategyExecutionPage{}, ErrInvalid
	}
	page := StrategyExecutionPage{Executions: executions}
	if len(executions) > limit {
		page.Executions = executions[:limit]
		last := page.Executions[len(page.Executions)-1]
		page.NextCursor = &StrategyExecutionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	for _, execution := range page.Executions {
		if execution.ID == "" || execution.StrategyInstanceID != id || execution.MandateVersion < 1 || (execution.Mode != Paper && execution.Mode != Shadow) || !isNonLiveExecutionStatus(execution.Status) || execution.Symbol == "" || execution.Instrument == "" || execution.Side == "" || execution.Quantity == "" || execution.CreatedAt.IsZero() {
			return StrategyExecutionPage{}, ErrInvalid
		}
	}
	return page, nil
}

func isNonLiveExecutionStatus(status ExecutionStatus) bool {
	switch status {
	case ExecutionProposed, RiskDenied, SimulatedFilled, SimulatedRejected, WouldHaveSubmitted, ExecutionCanceled, ExecutionError:
		return true
	default:
		return false
	}
}
func (s *InstanceService) ShadowOutcomes(c context.Context, p authorization.Principal, id string) ([]ShadowOutcome, error) {
	if !entitled(p) {
		return nil, ErrForbidden
	}
	reader, ok := s.store.(ShadowOutcomeReader)
	if !ok {
		return nil, ErrInvalid
	}
	return reader.ShadowOutcomes(c, p.UserID, id)
}
func (s *InstanceService) PaperPortfolio(c context.Context, p authorization.Principal, id string) (PaperPortfolio, error) {
	if !entitled(p) {
		return PaperPortfolio{}, ErrForbidden
	}
	_, portfolio, err := s.loadPaperPortfolio(c, p.UserID, id)
	return portfolio, err
}

func (s *InstanceService) AIPaperSpotFills(c context.Context, p authorization.Principal, id string, limit int, cursor *AIPaperSpotFillCursor) (AIPaperSpotFillPage, error) {
	if !entitled(p) {
		return AIPaperSpotFillPage{}, ErrForbidden
	}
	if limit < 1 || limit > 100 || (cursor != nil && (cursor.SimulatedAt.IsZero() || cursor.ID == "")) {
		return AIPaperSpotFillPage{}, ErrInvalid
	}
	instance, err := s.store.Get(c, p.UserID, id)
	if err != nil {
		return AIPaperSpotFillPage{}, ErrNotFound
	}
	if instance.ExecutionMode != Paper || instance.StrategyIdentifier != "ai_shadow" {
		return AIPaperSpotFillPage{}, ErrInvalid
	}
	reader, ok := s.store.(AIPaperSpotFillReader)
	if !ok {
		return AIPaperSpotFillPage{}, ErrInvalid
	}
	fills, err := reader.AIPaperSpotFills(c, p.UserID, id, limit+1, cursor)
	if err != nil {
		return AIPaperSpotFillPage{}, err
	}
	page := AIPaperSpotFillPage{Fills: fills}
	if len(page.Fills) > limit {
		last := page.Fills[limit-1]
		page.Fills = page.Fills[:limit]
		page.NextCursor = &AIPaperSpotFillCursor{SimulatedAt: last.SimulatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *InstanceService) Journal(c context.Context, p authorization.Principal, limit int, cursor *JournalCursor) (JournalPage, error) {
	if !entitled(p) {
		return JournalPage{}, ErrForbidden
	}
	if limit < 1 || limit > 100 || (cursor != nil && (cursor.CreatedAt.IsZero() || cursor.ID == "")) {
		return JournalPage{}, ErrInvalid
	}
	entries, err := s.store.Journal(c, p.UserID, limit+1, cursor)
	if err != nil {
		return JournalPage{}, err
	}
	page := JournalPage{Entries: entries}
	if len(entries) > limit {
		page.Entries = entries[:limit]
		last := page.Entries[len(page.Entries)-1]
		page.NextCursor = &JournalCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *InstanceService) Schedule(c context.Context, p authorization.Principal, id string) (ScheduleStatus, error) {
	if !entitled(p) {
		return ScheduleStatus{}, ErrForbidden
	}
	status, err := s.store.Schedule(c, p.UserID, id)
	if err != nil {
		return ScheduleStatus{}, ErrNotFound
	}
	return status, nil
}

func (s *InstanceService) ScheduleRuns(c context.Context, p authorization.Principal, id string, limit int, cursor *ScheduleRunCursor) (ScheduleRunPage, error) {
	if !entitled(p) {
		return ScheduleRunPage{}, ErrForbidden
	}
	if id == "" || limit < 1 || limit > 100 || (cursor != nil && (cursor.ScheduledFor.IsZero() || cursor.ID == "")) {
		return ScheduleRunPage{}, ErrInvalid
	}
	if _, err := s.store.Get(c, p.UserID, id); err != nil {
		return ScheduleRunPage{}, ErrNotFound
	}
	reader, ok := s.store.(ScheduleRunReader)
	if !ok {
		return ScheduleRunPage{}, ErrNotFound
	}
	runs, err := reader.ScheduleRuns(c, p.UserID, id, limit+1, cursor)
	if err != nil {
		return ScheduleRunPage{}, err
	}
	page := ScheduleRunPage{Runs: runs}
	if len(runs) > limit {
		page.Runs = runs[:limit]
		last := page.Runs[len(page.Runs)-1]
		page.NextCursor = &ScheduleRunCursor{ScheduledFor: last.ScheduledFor, ID: last.ID}
	}
	return page, nil
}

func (s *InstanceService) RecordLifecycle(c context.Context, p authorization.Principal, id string, command LifecycleCommand) (LifecycleResult, error) {
	if !entitled(p) {
		return LifecycleResult{}, ErrForbidden
	}
	if id == "" || !evaluationEventID.MatchString(command.EventID) || command.ExpectedStateVersion < 1 || !command.ConfirmPaperSimulation {
		return LifecycleResult{}, ErrInvalid
	}
	switch command.EventType {
	case ExpireWorthless, Assignment, CallAway:
	default:
		return LifecycleResult{}, ErrInvalid
	}
	result, err := s.store.RecordLifecycle(c, p.UserID, id, command, s.now().UTC())
	if err != nil {
		return LifecycleResult{}, err
	}
	return result, nil
}
