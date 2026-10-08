package execution

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

func generationFixture(now time.Time) ScheduledGenerationClaim {
	owner := "11111111-1111-4111-8111-111111111111"
	p := PilotAllocation{OwnerID: owner, AccountID: "22222222-2222-4222-8222-222222222222", ConnectionID: "33333333-3333-4333-8333-333333333333",
		CapitalBucketID: "44444444-4444-4444-8444-444444444444", ProductID: "BTC-USD", InitialCashUSD: "100",
		Limits: OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: now.Add(time.Hour).Truncate(time.Second)}}
	c := ScheduledGenerationClaim{ID: "55555555-5555-4555-8555-555555555555",
		Slot: ScheduledGenerationSlot{MandateApprovalID: "66666666-6666-4666-8666-666666666666", MandateID: "77777777-7777-4777-8777-777777777777", MandateVersion: 1, ScheduledFor: now.Truncate(time.Minute)},
		Facts: ScheduledGenerationFacts{Pilot: p, CashUSD: "100", AcquiredBase: "0", Objective: "Preserve capital; abstain without evidence.", MaxProposalNotional: "25",
			AIConnectionID: "88888888-8888-4888-8888-888888888888", AIModelID: "gpt-5.6-luna", Profile: "fast", ObservedAt: now, ExpiresAt: now.Add(time.Minute)},
		Market: generationMarket(now)}
	c.Input, c.InputDigest, _ = buildScheduledGenerationInput(c.Facts, c.Market, now)
	return c
}

func generationMarket(now time.Time) ScheduledGenerationMarket {
	return ScheduledGenerationMarket{ProductID: "BTC-USD", ProductType: "SPOT", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "online",
		BaseIncrement: "0.00000001", PriceIncrement: "0.01", BaseMinSize: "0.00000001", BaseMaxSize: "10", QuoteMinSize: "1", QuoteMaxSize: "1000000",
		BestBid: "59999", BestAsk: "60000", FeeAllowanceUSD: "1", StartedAt: now, CompletedAt: now, ObservedAt: now}
}

func generationDecision(side string) neural.LivePilotDecision {
	d := neural.LivePilotDecision{Decision: "PROPOSE", Symbol: "BTC", Side: side, ProposedNotional: "25", Confidence: "LOW", Thesis: "Synthetic test decision, not performance evidence.",
		RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: "gpt-5.6-luna", Profile: "fast"}}
	if side == "NONE" {
		d.Decision, d.Symbol, d.ProposedNotional = "ABSTAIN", "NONE", "0"
	}
	return d
}

func TestScheduledGenerationInputContainsOnlyAttributedFacts(t *testing.T) {
	now := time.Now().UTC()
	c := generationFixture(now)
	c.Facts.CashUSD, c.Facts.AcquiredBase = "12.30", "0.0002"
	in, digest, err := buildScheduledGenerationInput(c.Facts, c.Market, now)
	if err != nil || len(digest) != 64 || in.AvailableCashUSD != "12.3" || in.BuyingPowerUSD != "12.3" ||
		len(in.Positions) != 1 || in.Positions[0].Quantity != "0.0002" || in.Positions[0].AvailableQuantity != "0.0002" || in.Positions[0].MarketValueUSD != "11.9998" ||
		len(in.AllowedSymbols) != 1 || in.AllowedSymbols[0] != "BTC" || len(in.Markets) != 1 || in.Markets[0].Mark != "" || in.Markets[0].Last != "" ||
		in.Markets[0].HistoryStatus != "UNAVAILABLE" || len(in.RecentDecisions) != 0 {
		t.Fatal("incorrect isolated input", in, err)
	}
	body, _ := json.Marshal(in)
	for _, private := range []string{c.ID, c.Facts.Pilot.OwnerID, c.Facts.Pilot.AccountID, c.Facts.Pilot.ConnectionID, c.Facts.Pilot.CapitalBucketID,
		c.Slot.MandateID, c.Slot.MandateApprovalID, c.Facts.AIConnectionID} {
		if strings.Contains(string(body), private) {
			t.Fatal("private identity in provider facts")
		}
	}
	for name, change := range map[string]func(*ScheduledGenerationFacts){
		"unsupported model": func(f *ScheduledGenerationFacts) { f.AIModelID = "future-alias" },
		"profile mismatch":  func(f *ScheduledGenerationFacts) { f.Profile = "deep" },
		"over pilot cap":    func(f *ScheduledGenerationFacts) { f.MaxProposalNotional = "26" },
		"negative holdings": func(f *ScheduledGenerationFacts) { f.AcquiredBase = "-1" },
		"expired":           func(f *ScheduledGenerationFacts) { f.ExpiresAt = now },
		"future facts":      func(f *ScheduledGenerationFacts) { f.ObservedAt = now.Add(time.Second) },
		"blank objective":   func(f *ScheduledGenerationFacts) { f.Objective = " " },
	} {
		t.Run(name, func(t *testing.T) {
			f := c.Facts
			change(&f)
			if _, _, err := buildScheduledGenerationInput(f, c.Market, now); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
}

func TestScheduledGenerationMarketRejectsUnusableBooks(t *testing.T) {
	now := time.Now().UTC()
	for name, change := range map[string]func(*ScheduledGenerationMarket){
		"other pair":        func(m *ScheduledGenerationMarket) { m.ProductID = "ETH-USD" },
		"nonspot":           func(m *ScheduledGenerationMarket) { m.ProductType = "FUTURE" },
		"nonUSD":            func(m *ScheduledGenerationMarket) { m.QuoteCurrency = "USDC" },
		"wrong base":        func(m *ScheduledGenerationMarket) { m.BaseCurrency = "ETH" },
		"offline":           func(m *ScheduledGenerationMarket) { m.Status = "offline" },
		"disabled":          func(m *ScheduledGenerationMarket) { m.Disabled = true },
		"trading disabled":  func(m *ScheduledGenerationMarket) { m.TradingDisabled = true },
		"cancel only":       func(m *ScheduledGenerationMarket) { m.CancelOnly = true },
		"post only":         func(m *ScheduledGenerationMarket) { m.PostOnly = true },
		"auction":           func(m *ScheduledGenerationMarket) { m.AuctionMode = true },
		"view only":         func(m *ScheduledGenerationMarket) { m.ViewOnly = true },
		"crossed":           func(m *ScheduledGenerationMarket) { m.BestBid = "60001" },
		"missing ask":       func(m *ScheduledGenerationMarket) { m.BestAsk = "" },
		"zero bid":          func(m *ScheduledGenerationMarket) { m.BestBid = "0" },
		"unknown fees":      func(m *ScheduledGenerationMarket) { m.FeeAllowanceUSD = "" },
		"negative fees":     func(m *ScheduledGenerationMarket) { m.FeeAllowanceUSD = "-1" },
		"zero step":         func(m *ScheduledGenerationMarket) { m.BaseIncrement = "0" },
		"inverted limits":   func(m *ScheduledGenerationMarket) { m.BaseMinSize = "11" },
		"quote limits":      func(m *ScheduledGenerationMarket) { m.QuoteMinSize = "1000001" },
		"stale":             func(m *ScheduledGenerationMarket) { m.ObservedAt = now.Add(-11 * time.Second) },
		"future":            func(m *ScheduledGenerationMarket) { m.ObservedAt = now.Add(time.Second) },
		"future completion": func(m *ScheduledGenerationMarket) { m.CompletedAt = now.Add(time.Second) },
		"inverted time":     func(m *ScheduledGenerationMarket) { m.StartedAt = now.Add(time.Second) },
		"slow collection":   func(m *ScheduledGenerationMarket) { m.StartedAt = now.Add(-31 * time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			m := generationMarket(now)
			change(&m)
			if err := validateGenerationMarket("BTC-USD", m, now); err == nil {
				t.Fatal("unusable book accepted")
			}
		})
	}
}

func TestScheduledGenerationDeterministicSizingUsesAllInCapAndAttributedInventory(t *testing.T) {
	now := time.Now().UTC()
	c := generationFixture(now)
	d := generationDecision("BUY")
	r, err := deriveScheduledGenerationResult(c, d, generationMarket(now.Add(time.Second)))
	if err != nil || r.Proposal.BaseSize != "0.0004" || r.Proposal.LimitPrice != "60000" || r.Proposal.MaximumDebitUSD != "25" || r.Proposal.FeeAllowanceUSD != "1" ||
		r.Proposal.SourceMode != "LIVE" || validateScheduledGenerationResult(c, r) != nil {
		t.Fatal("BUY cap/digest/price", r, err)
	}
	c.Facts.CashUSD = "10"
	r, err = deriveScheduledGenerationResult(c, d, generationMarket(now.Add(time.Second)))
	if err != nil || r.Proposal.BaseSize != "0.00015" || r.Proposal.MaximumDebitUSD != "10" {
		t.Fatal("overspent attributed cash", r, err)
	}
	c.Facts.AcquiredBase = "0.0001"
	r, err = deriveScheduledGenerationResult(c, generationDecision("SELL"), generationMarket(now.Add(time.Second)))
	if err != nil || r.Proposal.BaseSize != "0.0001" || r.Proposal.LimitPrice != "59999" || r.Proposal.MaximumDebitUSD != "0" {
		t.Fatal("sold beyond pilot inventory", r, err)
	}
	c.Facts.AcquiredBase = "0"
	if _, err = deriveScheduledGenerationResult(c, generationDecision("SELL"), generationMarket(now.Add(time.Second))); err == nil {
		t.Fatal("sold pre-existing account holdings")
	}
}

func TestScheduledGenerationBoundsPriceAndRejectsInvalidDecisionOrTerms(t *testing.T) {
	now := time.Now().UTC()
	c := generationFixture(now)
	fresh := generationMarket(now.Add(time.Second))
	r, err := deriveScheduledGenerationResult(c, generationDecision("BUY"), fresh)
	if err != nil || r.Proposal.LimitPrice != "60000" || r.Proposal.BaseSize != "0.0004" {
		t.Fatal("exact book bound", r, err)
	}
	for _, side := range []string{"BUY", "SELL"} {
		x := c
		x.Facts.AcquiredBase = "0.0004"
		m := generationMarket(now.Add(time.Second))
		if side == "BUY" {
			x.Market.BestAsk = "60000.001"
		} else {
			x.Market.BestBid = "59998.999"
		}
		if _, err := deriveScheduledGenerationResult(x, generationDecision(side), m); err == nil {
			t.Fatal("off-grid original book silently rounded", side)
		}
		x.Market = c.Market
		if side == "BUY" {
			m.BestAsk = "60000.01"
		} else {
			m.BestBid = "59998.99"
		}
		if _, err := deriveScheduledGenerationResult(x, generationDecision(side), m); err == nil {
			t.Fatal("worsened book silently repriced", side)
		}
	}
	for name, change := range map[string]func(*neural.LivePilotDecision){
		"model":           func(d *neural.LivePilotDecision) { d.Metadata.Model = "gpt-5.6-sol" },
		"provider":        func(d *neural.LivePilotDecision) { d.Metadata.Provider = "other" },
		"profile":         func(d *neural.LivePilotDecision) { d.Metadata.Profile = "deep" },
		"pair":            func(d *neural.LivePilotDecision) { d.Symbol = "ETH" },
		"side":            func(d *neural.LivePilotDecision) { d.Side = "SHORT" },
		"overcap":         func(d *neural.LivePilotDecision) { d.ProposedNotional = "25.01" },
		"numeric":         func(d *neural.LivePilotDecision) { d.ProposedNotional = "1e1" },
		"zero":            func(d *neural.LivePilotDecision) { d.ProposedNotional = "0" },
		"bad abstention":  func(d *neural.LivePilotDecision) { d.Decision = "ABSTAIN" },
		"empty reason":    func(d *neural.LivePilotDecision) { d.Thesis = " " },
		"duplicate risks": func(d *neural.LivePilotDecision) { d.RiskFlags = []string{"risk", "risk"} },
	} {
		t.Run(name, func(t *testing.T) {
			d := generationDecision("BUY")
			change(&d)
			if err := validateGenerationDecision(c, d); err == nil {
				t.Fatal("untrusted decision accepted")
			}
		})
	}
	for name, change := range map[string]func(*ScheduledGenerationMarket){
		"fee exceeds budget":    func(m *ScheduledGenerationMarket) { m.FeeAllowanceUSD = "25" },
		"new incompatible tick": func(m *ScheduledGenerationMarket) { m.PriceIncrement = "7" },
		"base minimum":          func(m *ScheduledGenerationMarket) { m.BaseMinSize = "1" },
		"base maximum":          func(m *ScheduledGenerationMarket) { m.BaseMaxSize = "0.0000001" },
		"quote minimum":         func(m *ScheduledGenerationMarket) { m.QuoteMinSize = "100" },
		"quote maximum":         func(m *ScheduledGenerationMarket) { m.QuoteMaxSize = "2" },
		"predates claim":        func(m *ScheduledGenerationMarket) { m.StartedAt = now.Add(-time.Second) },
		"past fixed expiry":     func(m *ScheduledGenerationMarket) { m.CompletedAt = c.Facts.ExpiresAt },
	} {
		t.Run(name, func(t *testing.T) {
			m := generationMarket(now.Add(time.Second))
			change(&m)
			if _, err := deriveScheduledGenerationResult(c, generationDecision("BUY"), m); err == nil {
				t.Fatal("unsafe derived order")
			}
		})
	}
	// Frozen result recovery cannot replace terms even with another valid price.
	r.Proposal.LimitPrice = "60000.02"
	if validateScheduledGenerationResult(c, r) == nil {
		t.Fatal("changed saved terms accepted")
	}
}

type generationMemoryStore struct {
	mu                             sync.Mutex
	facts                          ScheduledGenerationFacts
	receipt                        *ScheduledGenerationReceipt
	order                          *Order
	claimErr, resultErr, intakeErr error
	denyFacts                      bool
	intakes                        int
}

func (s *generationMemoryStore) ReadScheduledGeneration(_ context.Context, owner string, slot ScheduledGenerationSlot) (ScheduledGenerationReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.receipt == nil || s.receipt.Claim.Facts.Pilot.OwnerID != owner || s.receipt.Claim.Slot.MandateID != slot.MandateID || s.receipt.Claim.Slot.MandateVersion != slot.MandateVersion || !s.receipt.Claim.Slot.ScheduledFor.Equal(slot.ScheduledFor) {
		return ScheduledGenerationReceipt{}, ErrNotFound
	}
	return *s.receipt, nil
}

func (s *generationMemoryStore) GenerationFacts(context.Context, string, ScheduledGenerationSlot) (ScheduledGenerationFacts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.denyFacts {
		return ScheduledGenerationFacts{}, ErrNotAuthorized
	}
	return s.facts, nil
}

func (s *generationMemoryStore) ClaimScheduledGeneration(_ context.Context, _ string, slot ScheduledGenerationSlot, m ScheduledGenerationMarket) (ScheduledGenerationClaim, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.receipt != nil {
		return s.receipt.Claim, false, nil
	}
	f := s.facts
	f.ObservedAt = time.Now().UTC()
	in, digest, err := buildScheduledGenerationInput(f, m, f.ObservedAt)
	if err != nil {
		return ScheduledGenerationClaim{}, false, err
	}
	c := ScheduledGenerationClaim{ID: "55555555-5555-4555-8555-555555555555", Slot: slot, Facts: f, Market: m, Input: in, InputDigest: digest}
	s.receipt = &ScheduledGenerationReceipt{Claim: c}
	return c, s.claimErr == nil, s.claimErr
}

func (s *generationMemoryStore) RecordScheduledGenerationResult(_ context.Context, owner, id string, result ScheduledGenerationResult) (ScheduledGenerationReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.receipt == nil || s.receipt.Claim.ID != id || s.receipt.Claim.Facts.Pilot.OwnerID != owner {
		return ScheduledGenerationReceipt{}, ErrNotFound
	}
	if err := validateScheduledGenerationResult(s.receipt.Claim, result); err != nil {
		return ScheduledGenerationReceipt{}, err
	}
	if s.receipt.Result != nil && !reflect.DeepEqual(*s.receipt.Result, result) {
		return ScheduledGenerationReceipt{}, ErrConflict
	}
	s.receipt.Result = &result
	return *s.receipt, s.resultErr
}

func (s *generationMemoryStore) PrepareScheduledProposal(_ context.Context, _ string, p ScheduledProposal) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.receipt == nil || s.receipt.Result == nil || s.receipt.Result.Proposal == nil || *s.receipt.Result.Proposal != p {
		return Order{}, ErrConflict
	}
	if s.intakeErr != nil {
		return Order{}, s.intakeErr
	}
	if s.order == nil {
		r := generationRequest(s.receipt.Claim, p)
		digest, err := requestDigest(r)
		if err != nil {
			return Order{}, err
		}
		s.order = &Order{ID: "99999999-9999-4999-8999-999999999999", Request: r, RequestDigest: digest}
		s.intakes++
	}
	return *s.order, nil
}

type generationMarketFunc func(context.Context, string, string, string) (ScheduledGenerationMarket, error)

func (f generationMarketFunc) CollectScheduledGenerationMarket(ctx context.Context, owner, account, product string) (ScheduledGenerationMarket, error) {
	return f(ctx, owner, account, product)
}

type generationDecisionFunc func(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error)

func (f generationDecisionFunc) GenerateLivePilotDecision(ctx context.Context, p authorization.Principal, conn, model string, in neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
	return f(ctx, p, conn, model, in)
}

func generationHarness(side string) (*ScheduledGenerator, *generationMemoryStore, authorization.Principal, ScheduledGenerationSlot, *atomic.Int32, *atomic.Int32) {
	c := generationFixture(time.Now().UTC())
	s := &generationMemoryStore{facts: c.Facts}
	models, markets := new(atomic.Int32), new(atomic.Int32)
	g := &ScheduledGenerator{store: s, market: generationMarketFunc(func(_ context.Context, owner, account, product string) (ScheduledGenerationMarket, error) {
		markets.Add(1)
		if owner != c.Facts.Pilot.OwnerID || account != c.Facts.Pilot.AccountID || product != c.Facts.Pilot.ProductID {
			return ScheduledGenerationMarket{}, ErrNotAuthorized
		}
		return generationMarket(time.Now().UTC()), nil
	}), model: generationDecisionFunc(func(ctx context.Context, p authorization.Principal, conn, model string, in neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		models.Add(1)
		if _, ok := ctx.Deadline(); !ok || p.UserID != c.Facts.Pilot.OwnerID || conn != c.Facts.AIConnectionID || model != c.Facts.AIModelID || in.BudgetScope != "live-pilot:"+c.Slot.MandateID {
			return neural.LivePilotDecision{}, ErrInvalid
		}
		return generationDecision(side), nil
	})}
	return g, s, authorization.Principal{UserID: c.Facts.Pilot.OwnerID, Entitlement: authorization.EntitlementFounder}, c.Slot, models, markets
}

func TestScheduledGeneratorConcurrentWorkersAndRestartReuseOneSavedDecision(t *testing.T) {
	g, s, p, slot, models, markets := generationHarness("BUY")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := g.Generate(context.Background(), p, slot); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	before := markets.Load()
	// New coordinator has no local attempt state. A renewable consent cannot
	// create another model entry or replace the original stored proposal.
	slot.MandateApprovalID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	restarted := &ScheduledGenerator{store: s, market: g.market, model: g.model}
	saved, order, err := restarted.Generate(context.Background(), p, slot)
	if err != nil || order == nil || saved.Result == nil || models.Load() != 1 || s.intakes != 1 || markets.Load() != before {
		t.Fatal("duplicate call/intake or nonhistorical recovery", err, models.Load(), s.intakes)
	}
}

func TestScheduledGeneratorLostCommitAndCrashNeverRecallModel(t *testing.T) {
	for _, where := range []string{"claim", "result", "panic", "error", "abstain"} {
		t.Run(where, func(t *testing.T) {
			g, s, p, slot, models, _ := generationHarness("BUY")
			switch where {
			case "claim":
				s.claimErr = ErrCommitUnknown
			case "result":
				s.resultErr = ErrCommitUnknown
			case "panic", "error", "abstain":
				g.model = generationDecisionFunc(func(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
					models.Add(1)
					if where == "panic" {
						panic("synthetic process failure")
					}
					if where == "error" {
						return neural.LivePilotDecision{}, errors.New("private provider error")
					}
					return generationDecision("NONE"), nil
				})
			}
			func() {
				defer func() {
					if recover() != nil && where != "panic" {
						t.Error("unexpected panic")
					}
				}()
				_, _, _ = g.Generate(context.Background(), p, slot)
			}()
			before := models.Load()
			if s.intakes != 0 {
				t.Fatal("intake before confirmed result")
			}
			for range 3 {
				_, o, err := g.Generate(context.Background(), p, slot)
				if err != nil {
					t.Fatal("historical recovery", err)
				}
				if where != "result" && o != nil {
					t.Fatal("order without saved proposal")
				}
			}
			if models.Load() != before || (where == "claim" && before != 0) || (where != "claim" && before != 1) {
				t.Fatal("model recalled", before, models.Load())
			}
			if where == "result" && s.intakes != 1 {
				t.Fatal("saved result not recovered exactly")
			}
		})
	}
}

func TestScheduledGeneratorDeniesAuthorityDriftAndBadPostmodelEvidence(t *testing.T) {
	for _, where := range []string{"before", "after", "stale", "cached", "wrong model", "timeout"} {
		t.Run(where, func(t *testing.T) {
			g, s, p, slot, models, _ := generationHarness("BUY")
			if where == "before" {
				s.denyFacts = true
			}
			original := g.model
			g.model = generationDecisionFunc(func(ctx context.Context, p authorization.Principal, conn, model string, in neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
				d, err := original.GenerateLivePilotDecision(ctx, p, conn, model, in)
				switch where {
				case "after":
					s.intakeErr = ErrNotAuthorized
				case "wrong model":
					d.Metadata.Model = "not-pinned"
				case "timeout":
					return d, context.DeadlineExceeded
				case "stale", "cached":
					g.market = generationMarketFunc(func(context.Context, string, string, string) (ScheduledGenerationMarket, error) {
						now := time.Now().UTC()
						m := generationMarket(now)
						if where == "stale" {
							m.ObservedAt = now.Add(-11 * time.Second)
						} else {
							m.StartedAt = now.Add(-time.Minute)
						}
						return m, nil
					})
				}
				return d, err
			})
			_, order, err := g.Generate(context.Background(), p, slot)
			if err == nil || order != nil || s.intakes != 0 {
				t.Fatal("unsafe admission", err)
			}
			if where == "before" && models.Load() != 0 {
				t.Fatal("called model despite authority denial")
			}
			if where != "before" {
				_, _, _ = g.Generate(context.Background(), p, slot)
				if models.Load() != 1 {
					t.Fatal("failed slot rerolled")
				}
			}
		})
	}
}
