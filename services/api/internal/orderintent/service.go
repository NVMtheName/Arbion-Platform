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
)

var (
	intentSymbolPattern        = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,15}$`)
	intentSizePattern          = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$`)
	storedDecimalPattern       = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$`)
	storedSignedDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$`)
	storedProductStatusPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,31}$`)
	idempotencyPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
)

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
)

type Store interface {
	ByIdempotency(context.Context, string, string) (*Intent, []byte, error)
	Create(context.Context, draft) (Intent, []byte, error)
	List(context.Context, string, string, int) ([]Intent, error)
	Get(context.Context, string, string) (Intent, []byte, error)
	Review(context.Context, reviewEvidence) (Intent, error)
}

type FinancialService interface {
	GetAccount(context.Context, authorization.Principal, string) (financial.FinancialAccount, error)
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
	now       func() time.Time
}

func NewService(store Store, financialService FinancialService, stepUp StepUpVerifier, audit Auditor) *Service {
	return &Service{store: store, financial: financialService, stepUp: stepUp, audit: audit, now: func() time.Time { return time.Now().UTC() }}
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
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	quantity, quantityOK := new(big.Rat).SetString(string(command.Size))
	if (source != SourceUI && source != SourceAI) || !intentSymbolPattern.MatchString(command.Symbol) || command.Symbol == "USD" || (command.Side != "BUY" && command.Side != "SELL") || !intentSizePattern.MatchString(string(command.Size)) || !quantityOK || quantity.Sign() <= 0 || !idempotencyPattern.MatchString(command.IdempotencyKey) {
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
	evidence, evidenceHash, err := normalizedEvidence(preview, command, now)
	if err != nil {
		return Intent{}, err
	}
	status := ReviewRequired
	if evidence.PreviewState == "BLOCKED" || !evidence.ProviderTradingAuthorized {
		status = Blocked
		if !evidence.ProviderTradingAuthorized {
			evidence.BlockReasons = appendUnique(evidence.BlockReasons, "PROVIDER_TRADE_PERMISSION_REQUIRED")
			evidenceHash = hashEvidence(evidence)
		}
	}
	intent := Intent{
		FinancialAccountID: accountID, Source: source, Provider: "coinbase", ProductID: preview.ProductID,
		BaseAsset: preview.BaseAsset, QuoteCurrency: "USD", Side: command.Side, OrderType: "MARKET_IOC",
		RequestedSize: preview.RequestedSize, Status: status, Version: 1, Preview: evidence, ReviewScope: ProposalReviewOnly,
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
	service.record(ctx, principal.UserID, "order_intent.proposed", created, map[string]any{"source": source, "outcome": strings.ToLower(created.Status)})
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
	if intent.Preview.PreviewState != "READY" || !intent.Preview.ProviderTradingAuthorized || len(intent.Preview.BlockReasons) != 0 || intent.Preview.ProductRules == nil || !intent.Preview.ProductRules.MarketIOCEnabled {
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
	digest := sha256.Sum256([]byte(accountID + "\x00" + source + "\x00" + command.Symbol + "\x00" + command.Side + "\x00" + string(command.Size)))
	return digest[:]
}

func hashEvidence(evidence PreviewEvidence) []byte {
	encoded, _ := json.Marshal(evidence)
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
