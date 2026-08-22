package orderintent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/risk"
)

var (
	ErrForbidden              = errors.New("order intent entitlement required")
	ErrInvalid                = errors.New("order intent input is invalid")
	ErrNotFound               = errors.New("order intent not found")
	ErrConflict               = errors.New("order intent state conflict")
	ErrIdempotencyConflict    = errors.New("order intent idempotency conflict")
	ErrExpired                = errors.New("order intent preview expired")
	ErrBlocked                = errors.New("order intent is blocked")
	ErrUnsafeProviderEvidence = errors.New("unsafe provider preview evidence")
	ErrUnsafeRiskEvidence     = errors.New("unsafe deterministic risk evidence")
)

var (
	intentSymbolPattern        = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
	intentSizePattern          = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
	storedDecimalPattern       = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$`)
	storedSignedDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$`)
	storedProductStatusPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)
	idempotencyPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	uuidPattern                = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

const manualPolicyVersion = "manual_coinbase_spot.v1"

var (
	allowedBlockReasons = map[string]struct{}{
		"ACCOUNT_RESTRICTED":      {},
		"INSUFFICIENT_FUNDS":      {},
		"INVALID_SIZE":            {},
		"MARKET_UNAVAILABLE":      {},
		"PRODUCT_AUCTION_MODE":    {},
		"PRODUCT_CANCEL_ONLY":     {},
		"PRODUCT_DISABLED":        {},
		"PRODUCT_LIMIT_ONLY":      {},
		"PRODUCT_POST_ONLY":       {},
		"PROVIDER_REJECTED":       {},
		"SIZE_ABOVE_MAXIMUM":      {},
		"SIZE_BELOW_MINIMUM":      {},
		"SIZE_INCREMENT_MISMATCH": {},
	}
	allowedWarnings = map[string]struct{}{
		"LARGE_ORDER":      {},
		"PROVIDER_WARNING": {},
		"SMALL_ORDER":      {},
	}
	allowedProductBlockReasons = map[string]struct{}{
		"PRODUCT_AUCTION_MODE":    {},
		"PRODUCT_CANCEL_ONLY":     {},
		"PRODUCT_DISABLED":        {},
		"PRODUCT_LIMIT_ONLY":      {},
		"PRODUCT_POST_ONLY":       {},
		"SIZE_ABOVE_MAXIMUM":      {},
		"SIZE_BELOW_MINIMUM":      {},
		"SIZE_INCREMENT_MISMATCH": {},
	}
	allowedManualRiskReasons = map[risk.ReasonCode]struct{}{
		risk.Allowed: {}, risk.AuthorizationDenied: {}, risk.AccountOwnershipMismatch: {}, risk.ConnectionUnavailable: {},
		risk.CircuitBreakerActive: {}, risk.MandateNotReady: {}, risk.CapitalPolicyRequired: {}, risk.AutonomyDenied: {}, risk.AutonomyRequiresApproval: {},
		risk.StaleAccountData: {}, risk.SymbolNotAllowed: {}, risk.OptionsNotAllowed: {}, risk.MarginNotAllowed: {},
		risk.InsufficientPosition: {}, risk.CapitalLimitExceeded: {}, risk.ReserveViolation: {}, risk.InsufficientBuyingPower: {},
		risk.PositionLimitExceeded: {}, risk.DailyLossLimitExceeded: {}, risk.InvalidAction: {},
	}
)

type Store interface {
	ByIdempotency(context.Context, string, string) (*Intent, []byte, error)
	Create(context.Context, draft) (Intent, []byte, error)
	List(context.Context, string, string, int) ([]Intent, error)
	Get(context.Context, string, string) (Intent, []byte, error)
	Review(context.Context, reviewEvidence) (Intent, error)
	ManualPolicy(context.Context, string, string, string) (risk.CapitalBucket, []risk.CircuitBreaker, error)
}

type FinancialService interface {
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
	GetBalances(context.Context, authorization.Principal, string) (financial.Balances, error)
	GetPositions(context.Context, authorization.Principal, string) ([]financial.Position, error)
	PreviewSpotOrder(context.Context, authorization.Principal, string, financial.SpotOrderPreviewRequest) (financial.SpotOrderPreview, error)
}

type StepUpVerifier interface {
	VerifyOrderIntentStepUp(context.Context, string, string) (string, time.Time, error)
}

type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}

type Service struct {
	store     Store
	financial FinancialService
	stepUp    StepUpVerifier
	audit     Auditor
	gate      *risk.Engine
	now       func() time.Time
}

func NewService(store Store, financialService FinancialService, stepUp StepUpVerifier, audit Auditor) *Service {
	return &Service{store: store, financial: financialService, stepUp: stepUp, audit: audit, gate: risk.NewEngine(), now: func() time.Time { return time.Now().UTC() }}
}

func (service *Service) CreateUI(ctx context.Context, principal authorization.Principal, accountID string, command CreateCommand) (Intent, error) {
	return service.create(ctx, principal, accountID, SourceUI, command)
}

// CreateAIProposal is the only AI-source entry point. It creates the same
// non-executing record as the UI and grants no approval or provider authority.
func (service *Service) CreateAIProposal(ctx context.Context, principal authorization.Principal, accountID string, command CreateCommand) (Intent, error) {
	return service.create(ctx, principal, accountID, SourceAI, command)
}

func (service *Service) create(ctx context.Context, principal authorization.Principal, accountID, source string, command CreateCommand) (Intent, error) {
	if !authorization.CanConnectFinancialAccounts(principal) {
		return Intent{}, ErrForbidden
	}
	command.Symbol = strings.ToUpper(strings.TrimSpace(command.Symbol))
	command.Side = strings.ToUpper(strings.TrimSpace(command.Side))
	command.Size = financial.Decimal(strings.TrimSpace(string(command.Size)))
	command.CapitalBucketID = strings.ToLower(strings.TrimSpace(command.CapitalBucketID))
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	quantity, quantityOK := new(big.Rat).SetString(string(command.Size))
	if (source != SourceUI && source != SourceAI) || !intentSymbolPattern.MatchString(command.Symbol) || command.Symbol == "USD" || (command.Side != "BUY" && command.Side != "SELL") || !intentSizePattern.MatchString(string(command.Size)) || !quantityOK || quantity.Sign() <= 0 || !uuidPattern.MatchString(command.CapitalBucketID) || !idempotencyPattern.MatchString(command.IdempotencyKey) {
		return Intent{}, ErrInvalid
	}
	requestHash := hashRequest(accountID, source, command)
	existing, existingHash, err := service.store.ByIdempotency(ctx, principal.UserID, command.IdempotencyKey)
	if err != nil {
		return Intent{}, err
	}
	if existing != nil {
		if !equalHash(existingHash, requestHash) {
			return Intent{}, ErrIdempotencyConflict
		}
		return *existing, nil
	}
	account, err := service.financial.GetAccount(ctx, principal, accountID)
	if err != nil {
		return Intent{}, err
	}
	if account.Provider != "coinbase" || account.Status != "active" || account.Capabilities["order_preview"] != financial.Supported || account.Capabilities["transfers"] != financial.Unsupported {
		return Intent{}, ErrBlocked
	}
	preview, err := service.financial.PreviewSpotOrder(ctx, principal, accountID, financial.SpotOrderPreviewRequest{Symbol: command.Symbol, Side: command.Side, Size: command.Size})
	if err != nil {
		return Intent{}, err
	}
	now := service.now().UTC()
	evidence, _, err := normalizedEvidence(preview, command, now)
	if err != nil {
		return Intent{}, err
	}
	bucket, breakers, err := service.store.ManualPolicy(ctx, principal.UserID, accountID, command.CapitalBucketID)
	if err != nil {
		return Intent{}, ErrBlocked
	}
	balances, err := service.financial.GetBalances(ctx, principal, accountID)
	if err != nil {
		return Intent{}, err
	}
	positions, err := service.financial.GetPositions(ctx, principal, accountID)
	if err != nil {
		return Intent{}, err
	}
	evaluatedAt := service.now().UTC()
	if !evaluatedAt.Before(evidence.ExpiresAt) {
		return Intent{}, ErrExpired
	}
	riskEvidence, err := service.evaluateManualRisk(principal, accountID, source, command, evidence, bucket, breakers, balances, positions, evaluatedAt)
	if err != nil {
		return Intent{}, err
	}
	status := ReviewRequired
	if evidence.PreviewState == "BLOCKED" || !evidence.ProviderTradingAuthorized || riskEvidence.Decision != risk.Allow {
		status = Blocked
		if !evidence.ProviderTradingAuthorized {
			evidence.BlockReasons = appendUnique(evidence.BlockReasons, "PROVIDER_TRADE_PERMISSION_REQUIRED")
		}
	}
	evidenceHash := hashIntentEvidence(evidence, riskEvidence)
	intent := Intent{
		FinancialAccountID: accountID, CapitalBucketID: command.CapitalBucketID, Source: source, Provider: "coinbase", ProductID: preview.ProductID,
		BaseAsset: preview.BaseAsset, QuoteCurrency: "USD", Side: command.Side, OrderType: "MARKET_IOC",
		RequestedSize: preview.RequestedSize, Status: status, Version: 1, Preview: evidence, Risk: &riskEvidence, ReviewScope: ProposalReviewOnly,
		SubmissionAvailable: false, RiskApprovalAvailable: false, AIExecutionAuthority: false, LiveExecutionAvailable: false,
		CreatedAt: now, UpdatedAt: now,
	}
	created, storedRequestHash, err := service.store.Create(ctx, draft{Intent: intent, UserID: principal.UserID, IdempotencyKey: command.IdempotencyKey, RequestHash: requestHash, EvidenceHash: evidenceHash})
	if err != nil {
		return Intent{}, err
	}
	if !equalHash(storedRequestHash, requestHash) {
		return Intent{}, ErrIdempotencyConflict
	}
	service.record(ctx, principal.UserID, "order_intent.proposed", created, map[string]any{"source": source, "outcome": strings.ToLower(created.Status), "risk_decision": strings.ToLower(string(riskEvidence.Decision)), "capital_bucket_id": command.CapitalBucketID})
	return created, nil
}

func (service *Service) List(ctx context.Context, principal authorization.Principal, accountID string) ([]Intent, error) {
	if !authorization.CanConnectFinancialAccounts(principal) {
		return nil, ErrForbidden
	}
	account, err := service.financial.GetAccount(ctx, principal, accountID)
	if err != nil {
		return nil, err
	}
	if account.Provider != "coinbase" {
		return nil, ErrNotFound
	}
	return service.store.List(ctx, principal.UserID, accountID, maxOrderIntentsPerResponse)
}

func (service *Service) Review(ctx context.Context, principal authorization.Principal, intentID string, command ReviewCommand) (Intent, error) {
	if !authorization.CanConnectFinancialAccounts(principal) {
		return Intent{}, ErrForbidden
	}
	intent, evidenceHash, err := service.store.Get(ctx, principal.UserID, intentID)
	if err != nil {
		return Intent{}, err
	}
	if command.ExpectedVersion <= 0 || command.ExpectedVersion != intent.Version || intent.Status != ReviewRequired {
		return Intent{}, ErrConflict
	}
	now := service.now().UTC()
	if !now.Before(intent.Preview.ExpiresAt) {
		return Intent{}, ErrExpired
	}
	if intent.Preview.PreviewState != "READY" || !intent.Preview.ProviderTradingAuthorized || len(intent.Preview.BlockReasons) != 0 || intent.Preview.ProductRules == nil || !intent.Preview.ProductRules.MarketIOCEnabled || !validManualRiskEvidence(intent.Risk, intent.CapitalBucketID, intent.Preview.PreviewedAt, intent.Preview.ExpiresAt, true) {
		return Intent{}, ErrBlocked
	}
	account, err := service.financial.GetAccount(ctx, principal, intent.FinancialAccountID)
	if err != nil {
		return Intent{}, err
	}
	if account.Provider != "coinbase" || account.Status != "active" || account.Capabilities["provider_trade_authorization"] != financial.Supported {
		return Intent{}, ErrBlocked
	}
	method, verifiedAt, err := service.stepUp.VerifyOrderIntentStepUp(ctx, principal.UserID, command.MFACode)
	if err != nil {
		return Intent{}, err
	}
	if method != "totp" || verifiedAt.IsZero() || verifiedAt.Before(intent.CreatedAt) || !verifiedAt.Before(intent.Preview.ExpiresAt) {
		return Intent{}, ErrConflict
	}
	reviewed, err := service.store.Review(ctx, reviewEvidence{IntentID: intent.ID, UserID: principal.UserID, ExpectedVersion: intent.Version, EvidenceHash: evidenceHash, MFAMethod: method, ReviewedAt: verifiedAt})
	if err != nil {
		return Intent{}, err
	}
	service.record(ctx, principal.UserID, "order_intent.user_reviewed_nonexecuting", reviewed, map[string]any{"approval_scope": ProposalReviewOnly, "mfa": method})
	return reviewed, nil
}

func normalizedEvidence(preview financial.SpotOrderPreview, command CreateCommand, now time.Time) (PreviewEvidence, []byte, error) {
	requestedCurrency := command.Symbol
	if command.Side == "BUY" {
		requestedCurrency = "USD"
	}
	if preview.Provider != "coinbase" || preview.Feed != "advanced_trade_order_preview" || preview.ProductID != command.Symbol+"-USD" || preview.BaseAsset != command.Symbol || preview.QuoteCurrency != "USD" || preview.Side != command.Side || preview.OrderType != "MARKET_IOC" || preview.RequestedSize.Amount != command.Size || preview.RequestedSize.Currency != requestedCurrency || (preview.PreviewState != "READY" && preview.PreviewState != "BLOCKED") || preview.PreviewedAt.IsZero() || preview.PreviewedAt.After(now.Add(5*time.Second)) || now.Sub(preview.PreviewedAt) > 30*time.Second {
		return PreviewEvidence{}, nil, ErrUnsafeProviderEvidence
	}
	for _, value := range []financial.Decimal{preview.BaseSize, preview.QuoteSize, preview.OrderTotal.Amount, preview.CommissionTotal.Amount} {
		if !storedDecimalPattern.MatchString(string(value)) {
			return PreviewEvidence{}, nil, ErrUnsafeProviderEvidence
		}
	}
	if preview.OrderTotal.Currency != "USD" || preview.CommissionTotal.Currency != "USD" || !validOptionalMoney(preview.BestBid) || !validOptionalMoney(preview.BestAsk) || !validOptionalMoney(preview.EstimatedAverageFilledPrice) || (preview.Slippage != nil && !storedSignedDecimalPattern.MatchString(string(*preview.Slippage))) || !validMessages(preview.BlockReasons, allowedBlockReasons) || !validMessages(preview.Warnings, allowedWarnings) || !validProductRules(preview.ProductRules, command, preview.PreviewedAt, now) {
		return PreviewEvidence{}, nil, ErrUnsafeProviderEvidence
	}
	if preview.PreviewState == "READY" && (len(preview.BlockReasons) != 0 || !positive(preview.OrderTotal.Amount) || (!positive(preview.BaseSize) && !positive(preview.QuoteSize))) {
		return PreviewEvidence{}, nil, ErrUnsafeProviderEvidence
	}
	if preview.PreviewState == "BLOCKED" && len(preview.BlockReasons) == 0 {
		return PreviewEvidence{}, nil, ErrUnsafeProviderEvidence
	}
	productRules := *preview.ProductRules
	productRules.BlockReasons = append([]string(nil), preview.ProductRules.BlockReasons...)
	evidence := PreviewEvidence{
		Provider: preview.Provider, Feed: preview.Feed, PreviewState: preview.PreviewState,
		BaseSize: preview.BaseSize, QuoteSize: preview.QuoteSize, OrderTotal: preview.OrderTotal, CommissionTotal: preview.CommissionTotal,
		BestBid: preview.BestBid, BestAsk: preview.BestAsk, EstimatedAverageFilledPrice: preview.EstimatedAverageFilledPrice,
		Slippage: preview.Slippage, ProviderTradingAuthorized: preview.ProviderTradingAuthorized,
		BlockReasons: append([]string(nil), preview.BlockReasons...), Warnings: append([]string(nil), preview.Warnings...),
		PreviewedAt: preview.PreviewedAt.UTC(), ExpiresAt: preview.PreviewedAt.UTC().Add(previewEvidenceLifetime), ProductRules: &productRules,
	}
	return evidence, hashEvidence(evidence), nil
}

func (service *Service) evaluateManualRisk(principal authorization.Principal, accountID, source string, command CreateCommand, preview PreviewEvidence, bucket risk.CapitalBucket, breakers []risk.CircuitBreaker, balances financial.Balances, positions []financial.Position, evaluatedAt time.Time) (ManualRiskEvidence, error) {
	availableCash := financial.Decimal("0")
	if balances.AvailableCash != nil {
		if balances.AvailableCash.Currency != "USD" || !storedDecimalPattern.MatchString(string(balances.AvailableCash.Amount)) {
			return ManualRiskEvidence{}, ErrUnsafeRiskEvidence
		}
		availableCash = balances.AvailableCash.Amount
	}
	targetQuantity := financial.Decimal("0")
	riskPositions := make([]risk.Position, 0, len(positions))
	for _, position := range positions {
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol))
		if !intentSymbolPattern.MatchString(symbol) || !storedDecimalPattern.MatchString(string(position.Quantity)) || position.AvailableQuantity == nil || !storedDecimalPattern.MatchString(string(*position.AvailableQuantity)) {
			return ManualRiskEvidence{}, ErrUnsafeRiskEvidence
		}
		riskPositions = append(riskPositions, risk.Position{Instrument: symbol, AvailableQuantity: string(*position.AvailableQuantity)})
		if symbol == command.Symbol {
			var ok bool
			targetQuantity, ok = addExactDecimals(targetQuantity, *position.AvailableQuantity)
			if !ok {
				return ManualRiskEvidence{}, ErrUnsafeRiskEvidence
			}
		}
	}
	proposedNotional, ok := addExactDecimals(preview.OrderTotal.Amount, preview.CommissionTotal.Amount)
	if !ok || !positive(proposedNotional) {
		return ManualRiskEvidence{}, ErrUnsafeRiskEvidence
	}
	actionType := risk.ActionBuy
	actionQuantity := preview.BaseSize
	if command.Side == "SELL" {
		actionType = risk.ActionSell
		actionQuantity = command.Size
	}
	actionSource := risk.SourceUI
	if source == SourceAI {
		actionSource = risk.SourceAI
	}
	action := risk.ProposedAction{
		ID: "proposal:" + command.IdempotencyKey, CorrelationID: command.IdempotencyKey, FinancialAccountID: accountID,
		Source: actionSource, ActionType: actionType, Instrument: command.Symbol, Side: command.Side,
		Quantity: string(actionQuantity), Notional: string(proposedNotional), CreatedAt: evaluatedAt,
	}
	context := risk.EvaluationContext{
		UserID: principal.UserID, AccountOwned: true, FinancialEntitled: true, ConnectionUsable: true,
		Bucket: &bucket, Breakers: append([]risk.CircuitBreaker(nil), breakers...), Now: evaluatedAt, MaxStaleness: 15 * time.Second,
		Account: &risk.AccountRiskSnapshot{
			AccountID: accountID, Currency: "USD", Timestamp: evaluatedAt, Cash: string(availableCash), AvailableCash: string(availableCash), BuyingPower: string(availableCash), CurrentExposure: "0", Positions: riskPositions,
		},
	}
	evaluation := service.gate.Evaluate(context, action)
	limit := (*financial.Decimal)(nil)
	if bucket.AllocationLimit != nil {
		value := financial.Decimal(*bucket.AllocationLimit)
		limit = &value
	}
	evidence := ManualRiskEvidence{
		PolicyVersion: manualPolicyVersion, EvaluationID: evaluation.ID, CapitalBucketID: bucket.ID, CapitalBucketName: bucket.Name,
		AllocationType: bucket.AllocationType, AllocationValue: financial.Decimal(bucket.AllocationValue), ProtectedAmount: financial.Decimal(bucket.ProtectedAmount), AllocationLimit: limit,
		AccountAvailableCash: financial.Money{Amount: availableCash, Currency: "USD"}, TargetAvailableQuantity: targetQuantity,
		ProposedNotional: financial.Money{Amount: proposedNotional, Currency: "USD"}, Decision: evaluation.Decision,
		ReasonCodes: append([]risk.ReasonCode(nil), evaluation.ReasonCodes...), Warnings: append([]risk.ReasonCode(nil), evaluation.Warnings...), Checks: append([]risk.RiskCheck(nil), evaluation.Checks...),
		ApprovalRequired: evaluation.ApprovalRequired, PlatformExecution: evaluation.PlatformExecutionAvailable, ObservedAt: evaluatedAt,
	}
	if !validManualRiskEvidence(&evidence, command.CapitalBucketID, preview.PreviewedAt, preview.ExpiresAt, false) {
		return ManualRiskEvidence{}, ErrUnsafeRiskEvidence
	}
	return evidence, nil
}

func addExactDecimals(left, right financial.Decimal) (financial.Decimal, bool) {
	leftNumber, leftOK := new(big.Rat).SetString(string(left))
	rightNumber, rightOK := new(big.Rat).SetString(string(right))
	if !leftOK || !rightOK || leftNumber.Sign() < 0 || rightNumber.Sign() < 0 {
		return "", false
	}
	value := financial.Decimal(canonicalStoredDecimal(new(big.Rat).Add(leftNumber, rightNumber).FloatString(18)))
	return value, storedDecimalPattern.MatchString(string(value))
}

func validManualRiskEvidence(evidence *ManualRiskEvidence, capitalBucketID string, previewedAt, expiresAt time.Time, requireAllow bool) bool {
	if evidence == nil || evidence.PolicyVersion != manualPolicyVersion || !uuidPattern.MatchString(evidence.EvaluationID) || evidence.CapitalBucketID != capitalBucketID || !uuidPattern.MatchString(evidence.CapitalBucketID) || strings.TrimSpace(evidence.CapitalBucketName) == "" || len(evidence.CapitalBucketName) > 100 || (evidence.AllocationType != "FIXED_AMOUNT" && evidence.AllocationType != "PERCENT_OF_AVAILABLE_CASH" && evidence.AllocationType != "PERCENT_OF_BUYING_POWER") || evidence.AccountAvailableCash.Currency != "USD" || evidence.ProposedNotional.Currency != "USD" || !storedDecimalPattern.MatchString(string(evidence.AccountAvailableCash.Amount)) || !storedDecimalPattern.MatchString(string(evidence.TargetAvailableQuantity)) || !storedDecimalPattern.MatchString(string(evidence.ProposedNotional.Amount)) || !positive(evidence.ProposedNotional.Amount) || !storedDecimalPattern.MatchString(string(evidence.AllocationValue)) || !positive(evidence.AllocationValue) || !storedDecimalPattern.MatchString(string(evidence.ProtectedAmount)) || evidence.ObservedAt.Before(previewedAt) || !evidence.ObservedAt.Before(expiresAt) || evidence.PlatformExecution || len(evidence.ReasonCodes) == 0 || len(evidence.ReasonCodes) > 20 || len(evidence.Warnings) > 20 || len(evidence.Checks) == 0 || len(evidence.Checks) > 20 {
		return false
	}
	if evidence.AllocationLimit != nil && (!storedDecimalPattern.MatchString(string(*evidence.AllocationLimit)) || !positive(*evidence.AllocationLimit)) {
		return false
	}
	if evidence.Decision != risk.Allow && evidence.Decision != risk.Deny {
		return false
	}
	if evidence.ApprovalRequired != (evidence.Decision == risk.Allow) {
		return false
	}
	if requireAllow && evidence.Decision != risk.Allow {
		return false
	}
	for _, reasons := range [][]risk.ReasonCode{evidence.ReasonCodes, evidence.Warnings} {
		seen := map[risk.ReasonCode]struct{}{}
		for _, reason := range reasons {
			if _, allowed := allowedManualRiskReasons[reason]; !allowed {
				return false
			}
			if _, duplicate := seen[reason]; duplicate {
				return false
			}
			seen[reason] = struct{}{}
		}
	}
	for _, item := range evidence.Checks {
		if _, allowed := allowedManualRiskReasons[item.Code]; !allowed || (item.Result != risk.Pass && item.Result != risk.Fail && item.Result != risk.CheckWarn) || strings.TrimSpace(item.Message) == "" || len(item.Message) > 240 {
			return false
		}
	}
	return true
}

func validProductRules(rules *financial.SpotProductRules, command CreateCommand, previewedAt, now time.Time) bool {
	if rules == nil || rules.Provider != "coinbase" || rules.Feed != "advanced_trade_product" || rules.ProductID != command.Symbol+"-USD" || rules.ProductType != "SPOT" || rules.BaseAsset != command.Symbol || rules.QuoteCurrency != "USD" || !storedProductStatusPattern.MatchString(rules.Status) || rules.ObservedAt.IsZero() || !rules.ObservedAt.Equal(previewedAt) || rules.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(rules.ObservedAt) > 30*time.Second || !validMessages(rules.BlockReasons, allowedProductBlockReasons) || rules.MarketIOCEnabled != (len(rules.BlockReasons) == 0) {
		return false
	}
	for _, value := range []financial.Decimal{rules.BaseIncrement, rules.QuoteIncrement, rules.BaseMinSize, rules.BaseMaxSize, rules.QuoteMinSize, rules.QuoteMaxSize} {
		if !storedDecimalPattern.MatchString(string(value)) || !positive(value) {
			return false
		}
	}
	if compare(rules.BaseMinSize, rules.BaseMaxSize) > 0 || compare(rules.QuoteMinSize, rules.QuoteMaxSize) > 0 {
		return false
	}
	if rules.Status != "ONLINE" && !hasReason(rules.BlockReasons, "PRODUCT_DISABLED") {
		return false
	}
	minimum, maximum, increment := rules.BaseMinSize, rules.BaseMaxSize, rules.BaseIncrement
	if command.Side == "BUY" {
		minimum, maximum, increment = rules.QuoteMinSize, rules.QuoteMaxSize, rules.QuoteIncrement
	}
	return hasReason(rules.BlockReasons, "SIZE_BELOW_MINIMUM") == (compare(command.Size, minimum) < 0) &&
		hasReason(rules.BlockReasons, "SIZE_ABOVE_MAXIMUM") == (compare(command.Size, maximum) > 0) &&
		hasReason(rules.BlockReasons, "SIZE_INCREMENT_MISMATCH") == !multiple(command.Size, increment)
}

func compare(left, right financial.Decimal) int {
	leftNumber, leftOK := new(big.Rat).SetString(string(left))
	rightNumber, rightOK := new(big.Rat).SetString(string(right))
	if !leftOK || !rightOK {
		return 0
	}
	return leftNumber.Cmp(rightNumber)
}

func multiple(value, increment financial.Decimal) bool {
	valueNumber, valueOK := new(big.Rat).SetString(string(value))
	incrementNumber, incrementOK := new(big.Rat).SetString(string(increment))
	return valueOK && incrementOK && incrementNumber.Sign() > 0 && new(big.Rat).Quo(valueNumber, incrementNumber).IsInt()
}

func hasReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}

func validMessages(values []string, allowed map[string]struct{}) bool {
	if len(values) > 20 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validOptionalMoney(value *financial.Money) bool {
	return value == nil || value.Currency == "USD" && storedDecimalPattern.MatchString(string(value.Amount)) && positive(value.Amount)
}

func positive(value financial.Decimal) bool {
	number, ok := new(big.Rat).SetString(string(value))
	return ok && number.Sign() > 0
}

func hashRequest(accountID, source string, command CreateCommand) []byte {
	digest := sha256.Sum256([]byte(accountID + "\x00" + source + "\x00" + command.Symbol + "\x00" + command.Side + "\x00" + string(command.Size) + "\x00" + command.CapitalBucketID))
	return digest[:]
}

func hashEvidence(evidence PreviewEvidence) []byte {
	encoded, _ := json.Marshal(evidence)
	digest := sha256.Sum256(encoded)
	return digest[:]
}

func hashIntentEvidence(preview PreviewEvidence, riskEvidence ManualRiskEvidence) []byte {
	encoded, _ := json.Marshal(struct {
		Preview PreviewEvidence    `json:"preview"`
		Risk    ManualRiskEvidence `json:"risk"`
	}{Preview: preview, Risk: riskEvidence})
	digest := sha256.Sum256(encoded)
	return digest[:]
}

func equalHash(left, right []byte) bool {
	if len(left) != sha256.Size || len(right) != sha256.Size {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (service *Service) record(ctx context.Context, userID, action string, intent Intent, metadata map[string]any) {
	if service.audit == nil {
		return
	}
	metadata["order_intent_id"] = intent.ID
	metadata["financial_account_id"] = intent.FinancialAccountID
	metadata["product_id"] = intent.ProductID
	metadata["side"] = intent.Side
	metadata["submission_available"] = false
	metadata["risk_approval_available"] = false
	metadata["live_execution_available"] = false
	_ = service.audit.Record(ctx, &userID, action, metadata)
}
