package orderintent

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/arbion/platform/services/api/internal/neural"
	"github.com/arbion/platform/services/api/internal/risk"
)

const testCapitalBucketID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

func ptrDecimal(value financial.Decimal) *financial.Decimal { return &value }

type memoryStore struct {
	intent       *Intent
	requestHash  []byte
	evidenceHash []byte
	reviews      int
	reservations ReservationSnapshot
}

func (store *memoryStore) ByIdempotency(_ context.Context, userID, key string) (*Intent, []byte, error) {
	if store.intent == nil || userID != "owner" || key != "intent-key-123456789" {
		return nil, nil, nil
	}
	copy := *store.intent
	return &copy, bytes.Clone(store.requestHash), nil
}

func (store *memoryStore) Create(_ context.Context, input draft) (Intent, []byte, error) {
	if store.intent != nil {
		return *store.intent, bytes.Clone(store.requestHash), nil
	}
	created := input.Intent
	created.ID = "intent-1"
	store.intent = &created
	store.requestHash = bytes.Clone(input.RequestHash)
	store.evidenceHash = bytes.Clone(input.EvidenceHash)
	return created, bytes.Clone(store.requestHash), nil
}

func (store *memoryStore) List(_ context.Context, userID, accountID string, limit int) ([]Intent, error) {
	if store.intent == nil || userID != "owner" || accountID != "account-1" || limit != maxOrderIntentsPerResponse {
		return []Intent{}, nil
	}
	return []Intent{*store.intent}, nil
}

func (store *memoryStore) Get(_ context.Context, userID, intentID string) (Intent, []byte, error) {
	if store.intent == nil || userID != "owner" || intentID != store.intent.ID {
		return Intent{}, nil, ErrNotFound
	}
	return *store.intent, bytes.Clone(store.evidenceHash), nil
}

func (store *memoryStore) Review(_ context.Context, evidence reviewEvidence) (Intent, error) {
	if store.intent == nil || evidence.IntentID != store.intent.ID || evidence.UserID != "owner" || evidence.ExpectedVersion != 1 || evidence.MFAMethod != "totp" || !bytes.Equal(evidence.EvidenceHash, store.evidenceHash) {
		return Intent{}, ErrConflict
	}
	store.reviews++
	updated := *store.intent
	updated.Status = UserApprovedNonExecutable
	updated.Version = 2
	updated.UpdatedAt = evidence.ReviewedAt
	store.intent = &updated
	return updated, nil
}

func (store *memoryStore) ManualPolicy(_ context.Context, userID, accountID, bucketID string) (risk.CapitalBucket, []risk.CircuitBreaker, error) {
	if userID != "owner" || accountID != "account-1" || bucketID != testCapitalBucketID {
		return risk.CapitalBucket{}, nil, ErrNotFound
	}
	return risk.CapitalBucket{
		ID: testCapitalBucketID, UserID: "owner", AccountID: "account-1", Name: "Coinbase manual",
		AllocationType: "FIXED_AMOUNT", AllocationValue: "100", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE",
	}, []risk.CircuitBreaker{}, nil
}

func (store *memoryStore) ActiveReservations(_ context.Context, userID, accountID, bucketID, symbol string, observedAt time.Time) (ReservationSnapshot, error) {
	if userID != "owner" || accountID != "account-1" || bucketID != testCapitalBucketID || symbol != "BTC" {
		return ReservationSnapshot{}, ErrNotFound
	}
	snapshot := store.reservations
	if snapshot.AccountReservedCash == "" {
		snapshot.AccountReservedCash = "0"
	}
	if snapshot.BucketReservedCash == "" {
		snapshot.BucketReservedCash = "0"
	}
	if snapshot.TargetReservedQuantity == "" {
		snapshot.TargetReservedQuantity = "0"
	}
	snapshot.ObservedAt = observedAt
	return snapshot, nil
}

type financialFake struct {
	account      financial.FinancialAccount
	preview      financial.SpotOrderPreview
	balances     financial.Balances
	positions    []financial.Position
	previewCalls int
}

func (fake *financialFake) GetAccount(_ context.Context, principal authorization.Principal, id string) (financial.FinancialAccount, error) {
	if principal.UserID != "owner" || id != "account-1" {
		return financial.FinancialAccount{}, errors.New("unexpected account lookup")
	}
	return fake.account, nil
}

func (fake *financialFake) PreviewSpotOrder(_ context.Context, principal authorization.Principal, id string, input financial.SpotOrderPreviewRequest) (financial.SpotOrderPreview, error) {
	if principal.UserID != "owner" || id != "account-1" || input.Symbol != "BTC" || input.Side != "BUY" || input.Size != "25.50" {
		return financial.SpotOrderPreview{}, errors.New("unexpected preview request")
	}
	fake.previewCalls++
	return fake.preview, nil
}

func (fake *financialFake) GetBalances(_ context.Context, principal authorization.Principal, id string) (financial.Balances, error) {
	if principal.UserID != "owner" || id != "account-1" {
		return financial.Balances{}, errors.New("unexpected balance lookup")
	}
	return fake.balances, nil
}

func (fake *financialFake) GetPositions(_ context.Context, principal authorization.Principal, id string) ([]financial.Position, error) {
	if principal.UserID != "owner" || id != "account-1" {
		return nil, errors.New("unexpected position lookup")
	}
	return fake.positions, nil
}

type stepUpFake struct {
	verifiedAt time.Time
	calls      int
}

func (fake *stepUpFake) VerifyOrderIntentStepUp(_ context.Context, userID, code string) (string, time.Time, error) {
	fake.calls++
	if userID != "owner" || code != "123456" {
		return "", time.Time{}, errors.New("invalid step-up")
	}
	return "totp", fake.verifiedAt, nil
}

type auditFake struct{ actions []string }

func (audit *auditFake) Record(_ context.Context, _ *string, action string, _ map[string]any) error {
	audit.actions = append(audit.actions, action)
	return nil
}

type proposalGeneratorFake struct {
	proposal neural.TradeProposal
	request  neural.TradeProposalRequest
	err      error
}

func (fake *proposalGeneratorFake) GenerateTradeProposal(_ context.Context, principal authorization.Principal, request neural.TradeProposalRequest) (neural.TradeProposal, error) {
	if principal.UserID != "owner" {
		return neural.TradeProposal{}, errors.New("unexpected proposal principal")
	}
	fake.request = request
	return fake.proposal, fake.err
}

func founder() authorization.Principal {
	return authorization.Principal{UserID: "owner", Role: authorization.RoleSuperadmin, Entitlement: authorization.EntitlementFounder}
}

func fixture(now time.Time, tradingAuthorized bool) (*memoryStore, *financialFake, *stepUpFake, *auditFake, *Service) {
	store := &memoryStore{}
	account := financial.FinancialAccount{ID: "account-1", Provider: "coinbase", Status: "active", Capabilities: financial.Capabilities{"order_preview": financial.Supported, "provider_trade_authorization": financial.Supported, "transfers": financial.Unsupported}}
	provider := &financialFake{account: account, preview: financial.SpotOrderPreview{
		Provider: "coinbase", Feed: "advanced_trade_order_preview", ProductID: "BTC-USD", BaseAsset: "BTC", QuoteCurrency: "USD", Side: "BUY", OrderType: "MARKET_IOC",
		RequestedSize: financial.Money{Amount: "25.50", Currency: "USD"}, BaseSize: "0.0004249", QuoteSize: "25.50",
		OrderTotal: financial.Money{Amount: "25.50", Currency: "USD"}, CommissionTotal: financial.Money{Amount: "0.15", Currency: "USD"},
		EstimatedAverageFilledPrice: &financial.Money{Amount: "60000.45", Currency: "USD"}, PreviewState: "READY",
		BlockReasons: []string{}, Warnings: []string{"SMALL_ORDER"}, ProviderTradingAuthorized: tradingAuthorized, PreviewedAt: now,
		ProductRules: &financial.SpotProductRules{
			Provider: "coinbase", Feed: "advanced_trade_product", ProductID: "BTC-USD", ProductType: "SPOT", BaseAsset: "BTC", QuoteCurrency: "USD",
			BaseIncrement: "0.00000001", QuoteIncrement: "0.01", BaseMinSize: "0.00000001", BaseMaxSize: "1000", QuoteMinSize: "1", QuoteMaxSize: "1000000",
			Status: "ONLINE", MarketIOCEnabled: true, BlockReasons: []string{}, ObservedAt: now,
		},
	}, balances: financial.Balances{AvailableCash: &financial.Money{Amount: "200", Currency: "USD"}}, positions: []financial.Position{{AccountID: "account-1", InstrumentType: "CRYPTO", Symbol: "BTC", Quantity: "0.012", AvailableQuantity: ptrDecimal("0.01"), Direction: "LONG"}}}
	stepUp := &stepUpFake{verifiedAt: now.Add(10 * time.Second)}
	audit := &auditFake{}
	service := NewService(store, provider, stepUp, audit)
	service.now = func() time.Time { return now }
	return store, provider, stepUp, audit, service
}

func TestCreateUIIsDurableIdempotentAndNonExecuting(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, provider, _, audit, service := fixture(now, true)
	command := CreateCommand{Symbol: "btc", Side: "buy", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"}
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if intent.ID != "intent-1" || intent.Status != ReviewRequired || intent.Version != 1 || intent.Source != SourceUI || intent.ProductID != "BTC-USD" || intent.Preview.OrderTotal.Amount != "25.50" || intent.CapitalBucketID != testCapitalBucketID || intent.Risk == nil || intent.Risk.Decision != risk.Allow || intent.Risk.ProposedNotional.Amount != "25.65" || intent.Risk.AccountReservedCash.Amount != "0" || intent.Risk.BucketReservedCash.Amount != "0" || intent.CapitalReservation == nil || intent.CapitalReservation.ResourceType != "CASH" || intent.CapitalReservation.Asset != "USD" || intent.CapitalReservation.Quantity != "25.65" || !intent.Risk.ApprovalRequired || intent.Risk.PlatformExecution || intent.ReviewScope != ProposalReviewOnly || intent.SubmissionAvailable || intent.RiskApprovalAvailable || intent.AIExecutionAuthority || intent.LiveExecutionAvailable {
		t.Fatalf("unsafe or incomplete intent: %#v", intent)
	}
	if provider.previewCalls != 1 || store.intent == nil || len(store.requestHash) != 32 || len(store.evidenceHash) != 32 || len(audit.actions) != 1 {
		t.Fatalf("proposal evidence was not stored once: calls=%d store=%#v audit=%#v", provider.previewCalls, store, audit.actions)
	}
	replayed, err := service.CreateUI(context.Background(), founder(), "account-1", command)
	if err != nil || replayed.ID != intent.ID || provider.previewCalls != 1 {
		t.Fatalf("idempotent replay called the provider or changed identity: %#v %v calls=%d", replayed, err, provider.previewCalls)
	}
	command.Size = "30"
	if _, err = service.CreateUI(context.Background(), founder(), "account-1", command); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed payload reused an idempotency key: %v", err)
	}
}

func TestGenerateAIProposalUsesNormalizedFactsAndDeterministicOrderIntentPath(t *testing.T) {
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	_, provider, _, audit, service := fixture(now, true)
	provider.positions = append(provider.positions, financial.Position{AccountID: "account-1", InstrumentType: "CRYPTO", Symbol: "BTC", Quantity: "0.003", AvailableQuantity: ptrDecimal("0.002"), Direction: "LONG"})
	generator := &proposalGeneratorFake{proposal: neural.TradeProposal{
		Decision: "PROPOSE", RequestedSize: "25.50", Confidence: "LOW", Thesis: "Keep the proposed allocation below the fixed user ceiling.",
		RiskFlags: []string{"Crypto prices can move sharply."}, Limitations: []string{"No external news feed was supplied."},
		Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-terra", Profile: "core"},
	}}
	service.ai = generator
	proposal, intent, err := service.GenerateAIProposal(context.Background(), founder(), "account-1", AIProposalCommand{
		Symbol: "btc", Side: "buy", MaxSize: "50", CapitalBucketID: testCapitalBucketID,
		Objective: " Keep a small long-term allocation. ", IdempotencyKey: "intent-key-123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Decision != "PROPOSE" || intent == nil || intent.Source != SourceAI || intent.RequestedSize.Amount != "25.50" || intent.SubmissionAvailable || intent.AIExecutionAuthority || intent.LiveExecutionAvailable {
		t.Fatalf("unsafe AI proposal result: proposal=%#v intent=%#v", proposal, intent)
	}
	if generator.request.Profile != "core" || generator.request.Symbol != "BTC" || generator.request.Side != "BUY" || generator.request.MaxSize != "50" || generator.request.MaxSizeUnit != "USD" || generator.request.AvailableCash != "200" || generator.request.PositionQuantity != "0.015" || generator.request.PositionAvailableQuantity != "0.012" || generator.request.Objective != "Keep a small long-term allocation." || !generator.request.ObservedAt.Equal(now) {
		t.Fatalf("proposal generator received unsafe or incomplete facts: %#v", generator.request)
	}
	if provider.previewCalls != 1 || len(audit.actions) != 1 || audit.actions[0] != "order_intent.proposed" {
		t.Fatalf("AI output did not use the durable proposal path: calls=%d audit=%#v", provider.previewCalls, audit.actions)
	}
}

func TestGenerateAIProposalAbstainsWithoutProviderPreview(t *testing.T) {
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	_, provider, _, audit, service := fixture(now, true)
	service.ai = &proposalGeneratorFake{proposal: neural.TradeProposal{
		Decision: "ABSTAIN", RequestedSize: "0", Confidence: "LOW", Thesis: "The supplied facts do not support a cautious proposal.",
		RiskFlags: []string{}, Limitations: []string{"No external market evidence was supplied."},
		Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-terra", Profile: "core"},
	}}
	proposal, intent, err := service.GenerateAIProposal(context.Background(), founder(), "account-1", AIProposalCommand{
		Symbol: "BTC", Side: "BUY", MaxSize: "50", CapitalBucketID: testCapitalBucketID,
		Objective: "Keep risk low.", IdempotencyKey: "intent-key-123456789",
	})
	if err != nil || proposal.Decision != "ABSTAIN" || intent != nil {
		t.Fatalf("safe abstention failed: proposal=%#v intent=%#v err=%v", proposal, intent, err)
	}
	if provider.previewCalls != 0 || len(audit.actions) != 1 || audit.actions[0] != "order_intent.ai_abstained" {
		t.Fatalf("abstention reached provider or was not audited: calls=%d audit=%#v", provider.previewCalls, audit.actions)
	}
}

func TestGenerateAIProposalRejectsConstraintViolationBeforeProviderPreview(t *testing.T) {
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	_, provider, _, _, service := fixture(now, true)
	service.ai = &proposalGeneratorFake{proposal: neural.TradeProposal{
		Decision: "PROPOSE", RequestedSize: "51", Confidence: "HIGH", Thesis: "Exceeds the user ceiling.",
		RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-terra", Profile: "core"},
	}}
	_, _, err := service.GenerateAIProposal(context.Background(), founder(), "account-1", AIProposalCommand{
		Symbol: "BTC", Side: "BUY", MaxSize: "50", CapitalBucketID: testCapitalBucketID,
		Objective: "Keep risk low.", IdempotencyKey: "intent-key-123456789",
	})
	if !errors.Is(err, ErrUnsafeAIProposal) || provider.previewCalls != 0 {
		t.Fatalf("unsafe model size reached provider: err=%v calls=%d", err, provider.previewCalls)
	}
}

func TestViewOnlyProviderCreatesBlockedProposalAndCannotReview(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	_, _, stepUp, _, service := fixture(now, false)
	intent, err := service.CreateAIProposal(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Source != SourceAI || intent.Status != Blocked || len(intent.Preview.BlockReasons) != 1 || intent.Preview.BlockReasons[0] != "PROVIDER_TRADE_PERMISSION_REQUIRED" {
		t.Fatalf("view-only proposal was not blocked: %#v", intent)
	}
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("blocked proposal review returned %v", err)
	}
	if stepUp.calls != 0 {
		t.Fatal("blocked proposal consumed MFA")
	}
}

func TestProductControlsCreateBlockedProposalAndCannotReview(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	_, provider, stepUp, _, service := fixture(now, true)
	provider.preview.PreviewState = "BLOCKED"
	provider.preview.BlockReasons = []string{"PRODUCT_LIMIT_ONLY"}
	provider.preview.ProductRules.MarketIOCEnabled = false
	provider.preview.ProductRules.BlockReasons = []string{"PRODUCT_LIMIT_ONLY"}
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Status != Blocked || intent.Preview.ProductRules == nil || intent.Preview.ProductRules.MarketIOCEnabled || len(intent.Preview.BlockReasons) != 1 || intent.Preview.BlockReasons[0] != "PRODUCT_LIMIT_ONLY" {
		t.Fatalf("product controls were not persisted as a block: %#v", intent)
	}
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("product-blocked proposal review returned %v", err)
	}
	if stepUp.calls != 0 {
		t.Fatal("product-blocked proposal consumed MFA")
	}
}

func TestInsufficientConnectedCashCreatesImmutableBlockedRiskEvidence(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, provider, stepUp, _, service := fixture(now, true)
	provider.balances.AvailableCash.Amount = "20"
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Status != Blocked || intent.Risk == nil || intent.Risk.Decision != risk.Deny || intent.Risk.ApprovalRequired || intent.Risk.PlatformExecution || len(intent.Risk.ReasonCodes) != 1 || intent.Risk.ReasonCodes[0] != risk.InsufficientBuyingPower || store.intent == nil {
		t.Fatalf("cash policy did not fail closed with evidence: %#v", intent)
	}
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("risk-blocked proposal review returned %v", err)
	}
	if stepUp.calls != 0 {
		t.Fatal("risk-blocked proposal consumed MFA")
	}
}

func TestExistingReservationCreatesBlockedRiskEvidence(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, _, stepUp, _, service := fixture(now, true)
	store.reservations = ReservationSnapshot{AccountReservedCash: "175", BucketReservedCash: "70", TargetReservedQuantity: "0"}
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	if intent.Status != Blocked || intent.Risk == nil || intent.Risk.Decision != risk.Deny || intent.CapitalReservation != nil || intent.Risk.AccountReservedCash.Amount != "175" || intent.Risk.BucketReservedCash.Amount != "70" {
		t.Fatalf("reserved capital was reused: %#v", intent)
	}
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrConflict) || stepUp.calls != 0 {
		t.Fatalf("reservation-blocked proposal reached MFA: err=%v calls=%d", err, stepUp.calls)
	}
}

func TestReviewConsumesStepUpAndRemainsNonExecutable(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, _, stepUp, audit, service := fixture(now, true)
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"})
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != UserApprovedNonExecutable || reviewed.Version != 2 || reviewed.SubmissionAvailable || reviewed.RiskApprovalAvailable || reviewed.LiveExecutionAvailable || store.reviews != 1 || stepUp.calls != 1 || len(audit.actions) != 2 {
		t.Fatalf("review crossed the non-execution boundary: %#v reviews=%d stepups=%d audit=%#v", reviewed, store.reviews, stepUp.calls, audit.actions)
	}
}

func TestReviewRequiresBoundCapitalReservationBeforeMFA(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, _, stepUp, _, service := fixture(now, true)
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	store.intent.CapitalReservation = nil
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrBlocked) {
		t.Fatalf("proposal without reservation returned %v", err)
	}
	if stepUp.calls != 0 {
		t.Fatal("proposal without reservation consumed MFA")
	}
}

func TestExpiredPreviewFailsBeforeMFA(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	_, _, stepUp, _, service := fixture(now, true)
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.Add(previewEvidenceLifetime) }
	if _, err = service.Review(context.Background(), founder(), intent.ID, ReviewCommand{ExpectedVersion: 1, MFACode: "123456"}); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired proposal review returned %v", err)
	}
	if stepUp.calls != 0 {
		t.Fatal("expired proposal consumed MFA")
	}
}

func TestCreateRejectsUnsafeProviderEvidence(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*financial.SpotOrderPreview)
	}{
		{name: "wrong requested currency", mutate: func(preview *financial.SpotOrderPreview) { preview.RequestedSize.Currency = "BTC" }},
		{name: "unknown warning", mutate: func(preview *financial.SpotOrderPreview) { preview.Warnings = []string{"RAW_PROVIDER_TEXT"} }},
		{name: "ready with block", mutate: func(preview *financial.SpotOrderPreview) { preview.BlockReasons = []string{"PROVIDER_REJECTED"} }},
		{name: "ready without value", mutate: func(preview *financial.SpotOrderPreview) { preview.OrderTotal.Amount = "0" }},
		{name: "blocked without reason", mutate: func(preview *financial.SpotOrderPreview) {
			preview.PreviewState = "BLOCKED"
			preview.BlockReasons = nil
		}},
		{name: "missing product rules", mutate: func(preview *financial.SpotOrderPreview) { preview.ProductRules = nil }},
		{name: "mismatched product increment", mutate: func(preview *financial.SpotOrderPreview) { preview.ProductRules.QuoteIncrement = "0.04" }},
		{name: "non-online product without block", mutate: func(preview *financial.SpotOrderPreview) { preview.ProductRules.Status = "OFFLINE" }},
		{name: "unsafe product reason", mutate: func(preview *financial.SpotOrderPreview) {
			preview.ProductRules.BlockReasons = []string{"RAW_PRODUCT_TEXT"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, provider, _, _, service := fixture(now, true)
			test.mutate(&provider.preview)
			_, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", CapitalBucketID: testCapitalBucketID, IdempotencyKey: "intent-key-123456789"})
			if !errors.Is(err, ErrUnsafeProviderEvidence) {
				t.Fatalf("unsafe evidence returned %v", err)
			}
			if store.intent != nil {
				t.Fatal("unsafe provider evidence was persisted")
			}
		})
	}
}
