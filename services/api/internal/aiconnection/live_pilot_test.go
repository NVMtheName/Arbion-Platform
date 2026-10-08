package aiconnection

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

type livePilotNeural struct {
	shadowNeural
	calls *int
}

func (f livePilotNeural) ProposeLivePilotDecision(ctx context.Context, provider string, secret []byte, in neural.LivePilotDecisionRequest, safety string) (neural.LivePilotDecision, error) {
	*f.calls++
	return f.shadowNeural.ProposeShadow(ctx, provider, secret, in, safety)
}

func livePilotServiceInput() neural.LivePilotDecisionRequest {
	now := time.Now().UTC()
	return neural.LivePilotDecisionRequest{BudgetScope: "fixed-slot", Objective: "Private fixed pilot objective", AllowedSymbols: []string{"BTC"}, MaxProposalNotional: "24", AvailableCashUSD: "100", BuyingPowerUSD: "100", Positions: []neural.ShadowPositionFact{}, Markets: []neural.ShadowMarketFact{{Symbol: "BTC", AssetClass: "CRYPTO", Currency: "USD", Bid: "60000", Ask: "60001", ObservedAt: now, Feed: "Coinbase", Quality: "LIVE", HistoryStatus: "UNAVAILABLE"}}, ObservedAt: now}
}

func TestLivePilotServiceUsesOnlyExactPinnedModelAndClearsAICredential(t *testing.T) {
	s, ms, _ := setup(t)
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	s.neural = fakeNeural{}
	connection, err := s.Create(context.Background(), p, "openai", "Synthetic pilot", []byte("synthetic-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(context.Background(), p, connection.ID); err != nil {
		t.Fatal(err)
	}
	ms.preference = &Preference{ConnectionID: "never-this-connection", ModelID: "never-this-model"}
	calls := 0
	var seen neural.ShadowDecisionRequest
	var secret []byte
	var safety, key string
	decision := neural.ShadowDecision{Decision: "ABSTAIN", Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "Insufficient evidence", RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-sol", Profile: "deep"}}
	fake := livePilotNeural{shadowNeural: shadowNeural{decision: decision, request: &seen, seenSecret: &secret, seenSafetyID: &safety}, calls: &calls}
	s.neural = fake
	s.limiter = fakeLimiter{allowed: true, key: &key}
	got, err := s.GenerateLivePilotDecision(context.Background(), p, connection.ID, "gpt-5.6-sol", livePilotServiceInput())
	if err != nil || got.Decision != "ABSTAIN" || seen.Profile != "deep" || calls != 1 || len(safety) != 64 || key != "neural-live-pilot-decision:u:fixed-slot" {
		t.Fatal("exact fresh adapter failed", err, calls, key)
	}
	for _, b := range secret {
		if b != 0 {
			t.Fatal("AI credential not cleared")
		}
	}
	for _, model := range []string{" gpt-5.6-sol", "gpt-5.6-sol ", "unavailable-model", "claude-sonnet-5", "gemini-3.6-flash"} {
		if _, err := s.GenerateLivePilotDecision(context.Background(), p, connection.ID, model, livePilotServiceInput()); !errors.Is(err, ErrInvalid) {
			t.Fatal("fallback or unsupported model accepted", model, err)
		}
	}
	if calls != 1 {
		t.Fatal("unsupported route used provider")
	}
	fake.decision.Metadata.Model = "changed-model"
	s.neural = fake
	if _, err := s.GenerateLivePilotDecision(context.Background(), p, connection.ID, "gpt-5.6-sol", livePilotServiceInput()); neural.Code(err) != neural.InternalError {
		t.Fatal("metadata changed pinned model", err)
	}
	fake.decision = decision
	fake.decision.ProposedNotional = "1"
	s.neural = fake
	if _, err := s.GenerateLivePilotDecision(context.Background(), p, connection.ID, "gpt-5.6-sol", livePilotServiceInput()); neural.Code(err) != neural.DecisionContractInvalid {
		t.Fatal("invalid structured proposal accepted", err)
	}
	fake.decision = decision
	fake.err = errors.New("private provider failure")
	s.neural = fake
	if _, err := s.GenerateLivePilotDecision(context.Background(), p, connection.ID, "gpt-5.6-sol", livePilotServiceInput()); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("provider error exposed internals")
	}
}

func TestLivePilotServiceCannotUseShadowOnlyAdapter(t *testing.T) {
	s, ms, _ := setup(t)
	ms.items["id"] = Connection{ID: "id", Provider: "openai", Status: "active"}
	s.neural = shadowNeural{}
	s.limiter = fakeLimiter{allowed: true}
	p := authorization.Principal{UserID: "u", Entitlement: authorization.EntitlementFounder}
	if _, err := s.GenerateLivePilotDecision(context.Background(), p, "id", "gpt-5.6-sol", livePilotServiceInput()); !errors.Is(err, ErrProvider) {
		t.Fatal("shadow adapter was promoted", err)
	}
	if _, err := s.GenerateLivePilotDecision(context.Background(), authorization.Principal{}, "id", "gpt-5.6-sol", livePilotServiceInput()); !errors.Is(err, ErrForbidden) {
		t.Fatal("unauthenticated caller accepted", err)
	}
	c := ms.items["id"]
	c.Status = "disabled"
	ms.items["id"] = c
	if _, err := s.GenerateLivePilotDecision(context.Background(), p, "id", "gpt-5.6-sol", livePilotServiceInput()); !errors.Is(err, ErrInactive) {
		t.Fatal("disabled connection accepted", err)
	}
}
