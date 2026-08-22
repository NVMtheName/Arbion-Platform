package orderintent

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/financial"
)

type memoryStore struct {
	intent       *Intent
	requestHash  []byte
	evidenceHash []byte
	reviews      int
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

type financialFake struct {
	account      financial.FinancialAccount
	preview      financial.SpotOrderPreview
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
	}}
	stepUp := &stepUpFake{verifiedAt: now.Add(10 * time.Second)}
	audit := &auditFake{}
	service := NewService(store, provider, stepUp, audit)
	service.now = func() time.Time { return now }
	return store, provider, stepUp, audit, service
}

func TestCreateUIIsDurableIdempotentAndNonExecuting(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, provider, _, audit, service := fixture(now, true)
	command := CreateCommand{Symbol: "btc", Side: "buy", Size: "25.50", IdempotencyKey: "intent-key-123456789"}
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if intent.ID != "intent-1" || intent.Status != ReviewRequired || intent.Version != 1 || intent.Source != SourceUI || intent.ProductID != "BTC-USD" || intent.Preview.OrderTotal.Amount != "25.50" || intent.ReviewScope != ProposalReviewOnly || intent.SubmissionAvailable || intent.RiskApprovalAvailable || intent.AIExecutionAuthority || intent.LiveExecutionAvailable {
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

func TestViewOnlyProviderCreatesBlockedProposalAndCannotReview(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	_, _, stepUp, _, service := fixture(now, false)
	intent, err := service.CreateAIProposal(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", IdempotencyKey: "intent-key-123456789"})
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

func TestReviewConsumesStepUpAndRemainsNonExecutable(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	store, _, stepUp, audit, service := fixture(now, true)
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", IdempotencyKey: "intent-key-123456789"})
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

func TestExpiredPreviewFailsBeforeMFA(t *testing.T) {
	now := time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC)
	_, _, stepUp, _, service := fixture(now, true)
	intent, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", IdempotencyKey: "intent-key-123456789"})
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, provider, _, _, service := fixture(now, true)
			test.mutate(&provider.preview)
			_, err := service.CreateUI(context.Background(), founder(), "account-1", CreateCommand{Symbol: "BTC", Side: "BUY", Size: "25.50", IdempotencyKey: "intent-key-123456789"})
			if !errors.Is(err, ErrUnsafeProviderEvidence) {
				t.Fatalf("unsafe evidence returned %v", err)
			}
			if store.intent != nil {
				t.Fatal("unsafe provider evidence was persisted")
			}
		})
	}
}
