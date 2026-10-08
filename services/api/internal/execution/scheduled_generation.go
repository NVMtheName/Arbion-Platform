package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/aiconnection"
	"github.com/arbion/platform/services/api/internal/authorization"
	"github.com/arbion/platform/services/api/internal/neural"
)

// These are private-server domain inputs, not browser commands or model tools.
// Neither a slot nor a generation receipt grants permission to submit an order.
type ScheduledGenerationSlot struct {
	MandateApprovalID, MandateID string
	MandateVersion               int
	ScheduledFor                 time.Time
}

type ScheduledGenerationFacts struct {
	Pilot                                                                                     PilotAllocation
	CashUSD, AcquiredBase, Objective, MaxProposalNotional, AIConnectionID, AIModelID, Profile string
	ExpiresAt, ObservedAt                                                                     time.Time
}

// Market contains normalized read-only evidence from a trusted collector. The
// allowance is an explicit all-in fee budget, not a fee invented by the model.
// Final broker preview independently checks actual fees and available funds.
type ScheduledGenerationMarket struct {
	ProductID, ProductType, BaseCurrency, QuoteCurrency, Status                         string
	BaseIncrement, PriceIncrement, BaseMinSize, BaseMaxSize, QuoteMinSize, QuoteMaxSize string
	BestBid, BestAsk, FeeAllowanceUSD                                                   string
	Disabled, TradingDisabled, CancelOnly, PostOnly, AuctionMode, ViewOnly              bool
	StartedAt, CompletedAt, ObservedAt                                                  time.Time
}

type ScheduledGenerationClaim struct {
	ID          string
	Slot        ScheduledGenerationSlot
	Facts       ScheduledGenerationFacts
	Market      ScheduledGenerationMarket
	Input       neural.LivePilotDecisionRequest
	InputDigest string
}

type ScheduledGenerationResult struct {
	Outcome                      string
	Decision                     neural.LivePilotDecision
	Proposal                     *ScheduledProposal
	RequestDigest, ClientOrderID string
	Market                       *ScheduledGenerationMarket
}

type ScheduledGenerationReceipt struct {
	Claim  ScheduledGenerationClaim
	Result *ScheduledGenerationResult
}

type scheduledGenerationStore interface {
	ReadScheduledGeneration(context.Context, string, ScheduledGenerationSlot) (ScheduledGenerationReceipt, error)
	GenerationFacts(context.Context, string, ScheduledGenerationSlot) (ScheduledGenerationFacts, error)
	ClaimScheduledGeneration(context.Context, string, ScheduledGenerationSlot, ScheduledGenerationMarket) (ScheduledGenerationClaim, bool, error)
	RecordScheduledGenerationResult(context.Context, string, string, ScheduledGenerationResult) (ScheduledGenerationReceipt, error)
	PrepareScheduledProposal(context.Context, string, ScheduledProposal) (Order, error)
}

type ScheduledGenerationMarketCollector interface {
	CollectScheduledGenerationMarket(context.Context, string, string, string) (ScheduledGenerationMarket, error)
}

type ScheduledDecisionGenerator interface {
	GenerateLivePilotDecision(context.Context, authorization.Principal, string, string, neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error)
}

// ScheduledGenerator has no sender, preflight, credential or execution-claim
// dependency. No production runtime constructs it. Only a newly committed
// generation claim permits one model entry; history can never recreate that
// permission, even when no result exists after a crash.
type ScheduledGenerator struct {
	store  scheduledGenerationStore
	market ScheduledGenerationMarketCollector
	model  ScheduledDecisionGenerator
}

func NewScheduledGenerator(store *PostgresStore, market ScheduledGenerationMarketCollector, model ScheduledDecisionGenerator) (*ScheduledGenerator, error) {
	if store == nil || store.db == nil || market == nil || model == nil {
		return nil, ErrInvalid
	}
	return &ScheduledGenerator{store: store, market: market, model: model}, nil
}

// Generate returns saved history first. A saved PROPOSE may recover only its
// exact immutable intake request; expiry/revocation still deny new intake.
// FAILED, ABSTAIN and missing results are terminal for model entry in this slot.
func (g *ScheduledGenerator) Generate(ctx context.Context, p authorization.Principal, slot ScheduledGenerationSlot) (ScheduledGenerationReceipt, *Order, error) {
	if g == nil || g.store == nil || g.market == nil || g.model == nil || !validUUID(p.UserID) || p.Entitlement != authorization.EntitlementFounder ||
		!validUUID(slot.MandateApprovalID) || !validUUID(slot.MandateID) || slot.MandateVersion < 1 || !canonicalScheduledSlot(slot.ScheduledFor) {
		return ScheduledGenerationReceipt{}, nil, ErrNotAuthorized
	}
	if saved, err := g.store.ReadScheduledGeneration(ctx, p.UserID, slot); !errors.Is(err, ErrNotFound) {
		if err != nil {
			return ScheduledGenerationReceipt{}, nil, err
		}
		return g.recover(ctx, p.UserID, saved)
	}
	facts, err := g.store.GenerationFacts(ctx, p.UserID, slot)
	if err != nil {
		return ScheduledGenerationReceipt{}, nil, err
	}
	market, err := g.market.CollectScheduledGenerationMarket(ctx, p.UserID, facts.Pilot.AccountID, facts.Pilot.ProductID)
	if err != nil {
		return ScheduledGenerationReceipt{}, nil, ErrNotAuthorized
	}
	started := time.Now() // includes database/commit response latency in the budget
	claim, admitted, err := g.store.ClaimScheduledGeneration(ctx, p.UserID, slot, market)
	if err != nil {
		return ScheduledGenerationReceipt{}, nil, err // including ambiguous commit: zero calls
	}
	if !admitted {
		saved, err := g.store.ReadScheduledGeneration(ctx, p.UserID, slot)
		if err != nil {
			return ScheduledGenerationReceipt{}, nil, err
		}
		return g.recover(ctx, p.UserID, saved)
	}
	receipt := ScheduledGenerationReceipt{Claim: claim}
	input, digest, err := buildScheduledGenerationInput(claim.Facts, claim.Market, claim.Facts.ObservedAt)
	if err != nil || digest != claim.InputDigest || !sameGenerationInput(input, claim.Input) || claim.Facts.Pilot.OwnerID != p.UserID || claim.Slot != slot {
		return receipt, nil, ErrNotAuthorized
	}
	// A committed claim is not an indefinitely renewable model-call lease.
	remaining := claim.Facts.ExpiresAt.Sub(claim.Facts.ObservedAt)
	if remaining > time.Minute {
		remaining = time.Minute
	}
	deadline := started.Add(remaining)
	if !time.Now().Before(deadline) || validateGenerationMarket(claim.Facts.Pilot.ProductID, claim.Market, time.Now().UTC()) != nil {
		return g.fail(ctx, p.UserID, claim, ErrNotAuthorized)
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	input.BudgetScope = "live-pilot:" + slot.MandateID // private, stable across slots/consent renewals
	decision, err := g.model.GenerateLivePilotDecision(callCtx, p, claim.Facts.AIConnectionID, claim.Facts.AIModelID, input)
	completed := time.Now()
	if err != nil || callCtx.Err() != nil || !completed.Before(deadline) || validateGenerationDecision(claim, decision) != nil {
		return g.fail(ctx, p.UserID, claim, ErrNotAuthorized)
	}
	result := ScheduledGenerationResult{Outcome: decision.Decision, Decision: decision}
	if decision.Decision == "PROPOSE" {
		fresh, err := g.market.CollectScheduledGenerationMarket(callCtx, p.UserID, claim.Facts.Pilot.AccountID, claim.Facts.Pilot.ProductID)
		if err != nil || callCtx.Err() != nil || fresh.StartedAt.Before(completed) || validateGenerationMarket(claim.Facts.Pilot.ProductID, fresh, time.Now().UTC()) != nil {
			return g.fail(ctx, p.UserID, claim, ErrNotAuthorized)
		}
		result, err = deriveScheduledGenerationResult(claim, decision, fresh)
		if err != nil {
			return g.fail(ctx, p.UserID, claim, err)
		}
	}
	// Persist before intake. Lost result commits are read-only recovery, never
	// generation retries or use of the in-memory result to bypass persistence.
	saved, err := g.store.RecordScheduledGenerationResult(callCtx, p.UserID, claim.ID, result)
	if err != nil {
		return receipt, nil, err
	}
	return g.recover(ctx, p.UserID, saved)
}

func (g *ScheduledGenerator) fail(ctx context.Context, owner string, c ScheduledGenerationClaim, reason error) (ScheduledGenerationReceipt, *Order, error) {
	saved, err := g.store.RecordScheduledGenerationResult(ctx, owner, c.ID, ScheduledGenerationResult{Outcome: "FAILED"})
	if err != nil {
		return ScheduledGenerationReceipt{Claim: c}, nil, errors.Join(reason, err)
	}
	return saved, nil, reason
}

func (g *ScheduledGenerator) recover(ctx context.Context, owner string, saved ScheduledGenerationReceipt) (ScheduledGenerationReceipt, *Order, error) {
	if saved.Claim.Facts.Pilot.OwnerID != owner {
		return ScheduledGenerationReceipt{}, nil, ErrNotAuthorized
	}
	if saved.Result == nil {
		return saved, nil, nil // unknown, not evidence the model was never invoked
	}
	if err := validateScheduledGenerationResult(saved.Claim, *saved.Result); err != nil {
		return saved, nil, err
	}
	if saved.Result.Outcome != "PROPOSE" {
		return saved, nil, nil
	}
	o, err := g.store.PrepareScheduledProposal(ctx, owner, *saved.Result.Proposal)
	if err != nil {
		return saved, nil, err
	}
	if o.RequestDigest != saved.Result.RequestDigest || o.Request.ClientOrderID != saved.Result.ClientOrderID {
		return saved, nil, ErrConflict
	}
	return saved, &o, nil
}

func scheduledGenerationProfile(modelID string) (string, error) {
	route, err := aiconnection.LivePilotModelRoute(modelID)
	if err != nil {
		return "", ErrNotAuthorized
	}
	return string(route.Profile), nil
}

func validateGenerationMarket(product string, m ScheduledGenerationMarket, now time.Time) error {
	if !productPattern.MatchString(product) || product == "USD-USD" || m.ProductID != product || m.ProductType != "SPOT" ||
		m.BaseCurrency != strings.TrimSuffix(product, "-USD") || m.QuoteCurrency != "USD" || m.Status != "online" ||
		m.Disabled || m.TradingDisabled || m.CancelOnly || m.PostOnly || m.AuctionMode || m.ViewOnly ||
		m.StartedAt.IsZero() || m.StartedAt.After(m.CompletedAt) || m.CompletedAt.After(now) || m.StartedAt.Before(now.Add(-30*time.Second)) ||
		m.ObservedAt.IsZero() || m.ObservedAt.After(m.CompletedAt) || m.ObservedAt.Before(now.Add(-10*time.Second)) {
		return ErrNotAuthorized
	}
	values := []string{m.BaseIncrement, m.PriceIncrement, m.BaseMinSize, m.BaseMaxSize, m.QuoteMinSize, m.QuoteMaxSize, m.BestBid, m.BestAsk}
	n := make([]*big.Rat, len(values))
	for i, value := range values {
		var ok bool
		if n[i], ok = decimal(value, true); !ok {
			return ErrInvalid
		}
	}
	if n[2].Cmp(n[3]) > 0 || n[4].Cmp(n[5]) > 0 || n[6].Cmp(n[7]) > 0 ||
		!new(big.Rat).Quo(n[6], n[1]).IsInt() || !new(big.Rat).Quo(n[7], n[1]).IsInt() {
		return ErrNotAuthorized
	}
	if _, ok := decimal(m.FeeAllowanceUSD, false); !ok {
		return ErrInvalid
	}
	return nil
}

func buildScheduledGenerationInput(f ScheduledGenerationFacts, m ScheduledGenerationMarket, now time.Time) (neural.LivePilotDecisionRequest, string, error) {
	var input neural.LivePilotDecisionRequest
	profile, err := scheduledGenerationProfile(f.AIModelID)
	if err != nil || f.Profile != profile || !validPilotAllocation(f.Pilot) || !validUUID(f.AIConnectionID) ||
		strings.TrimSpace(f.Objective) == "" || len(f.Objective) > 2000 || f.ObservedAt.IsZero() || f.ObservedAt.After(now) ||
		!f.ExpiresAt.After(now) || f.ExpiresAt.After(f.Pilot.Limits.ExpiresAt) || validateGenerationMarket(f.Pilot.ProductID, m, now) != nil {
		return input, "", ErrNotAuthorized
	}
	cash, cok := decimal(f.CashUSD, false)
	base, bok := decimal(f.AcquiredBase, false)
	cap, capok := decimal(f.MaxProposalNotional, true)
	pilotCap, _ := decimal(f.Pilot.Limits.MaximumOrderUSD, true)
	if !cok || !bok || !capok || cap.Cmp(pilotCap) > 0 {
		return input, "", ErrNotAuthorized
	}
	symbol := strings.TrimSuffix(f.Pilot.ProductID, "-USD")
	input = neural.LivePilotDecisionRequest{Profile: profile, Objective: f.Objective, AllowedSymbols: []string{symbol},
		MaxProposalNotional: f.MaxProposalNotional, AvailableCashUSD: canonical(cash), BuyingPowerUSD: canonical(cash),
		Positions: []neural.ShadowPositionFact{}, Markets: []neural.ShadowMarketFact{{Symbol: symbol, AssetClass: "CRYPTO", Currency: "USD",
			Bid: m.BestBid, Ask: m.BestAsk, Feed: "COINBASE_ORDER_BOOK", Quality: "LIVE", ObservedAt: m.ObservedAt,
			HistoryStatus: "UNAVAILABLE", LiquidityStatus: "UNAVAILABLE"}},
		MarketEventCoverage: []neural.ShadowMarketEventCoverage{}, MarketEvents: []neural.ShadowMarketEventFact{},
		RecentDecisions: []neural.ShadowRecentDecision{}, ObservedAt: f.ObservedAt}
	if base.Sign() > 0 {
		bid, _ := decimal(m.BestBid, true)
		value := canonical(new(big.Rat).Mul(base, bid))
		if _, ok := decimal(value, false); !ok {
			return neural.LivePilotDecisionRequest{}, "", ErrInvalid
		}
		input.Positions = append(input.Positions, neural.ShadowPositionFact{Symbol: symbol, Instrument: "CRYPTO", Quantity: canonical(base),
			AvailableQuantity: canonical(base), MarketValueUSD: value, PerformanceStatus: "UNAVAILABLE"})
	}
	if neural.ValidateLivePilotDecisionRequest(input) != nil {
		return neural.LivePilotDecisionRequest{}, "", ErrInvalid
	}
	body, err := json.Marshal(input)
	if err != nil {
		return neural.LivePilotDecisionRequest{}, "", ErrInvalid
	}
	digest := sha256.Sum256(body)
	return input, hex.EncodeToString(digest[:]), nil
}

func sameGenerationInput(a, b neural.LivePilotDecisionRequest) bool {
	// BudgetScope is private runtime metadata, not part of the frozen input.
	a.BudgetScope, b.BudgetScope = "", ""
	return reflect.DeepEqual(a, b)
}

func validateGenerationDecision(c ScheduledGenerationClaim, d neural.LivePilotDecision) error {
	if d.Metadata.Provider != "openai" || d.Metadata.Model != c.Facts.AIModelID || d.Metadata.Profile != c.Facts.Profile ||
		len(d.Metadata.RequestID) > 200 {
		return ErrInvalid
	}
	for _, n := range []*int{d.Metadata.InputUsage, d.Metadata.OutputUsage, d.Metadata.LatencyMS} {
		if n != nil && *n < 0 {
			return ErrInvalid
		}
	}
	input := neural.LivePilotDecisionRequest{AllowedSymbols: []string{strings.TrimSuffix(c.Facts.Pilot.ProductID, "-USD")}, MaxProposalNotional: c.Facts.MaxProposalNotional}
	if neural.ValidateLivePilotDecision(d, input) != nil {
		return ErrInvalid
	}
	return nil
}

// Derive from fresh trusted facts, not a model price/quantity/fee. The original
// book fixes the price bound; a later worsened quote cannot silently reprice the
// decision. Quantity always rounds down, fees come out of the all-in budget,
// and SELL inventory is capped at pilot-acquired units only.
func deriveScheduledGenerationResult(c ScheduledGenerationClaim, d neural.LivePilotDecision, fresh ScheduledGenerationMarket) (ScheduledGenerationResult, error) {
	if validateGenerationDecision(c, d) != nil || d.Decision != "PROPOSE" ||
		validateGenerationMarket(c.Facts.Pilot.ProductID, c.Market, c.Facts.ObservedAt) != nil ||
		validateGenerationMarket(c.Facts.Pilot.ProductID, fresh, fresh.CompletedAt) != nil ||
		fresh.StartedAt.Before(c.Facts.ObservedAt) || !fresh.CompletedAt.Before(c.Facts.ExpiresAt) {
		return ScheduledGenerationResult{}, ErrNotAuthorized
	}
	priceText := c.Market.BestAsk
	if d.Side == "SELL" {
		priceText = c.Market.BestBid
	}
	price, _ := decimal(priceText, true)
	newStep, _ := decimal(fresh.PriceIncrement, true)
	ask, _ := decimal(fresh.BestAsk, true)
	bid, _ := decimal(fresh.BestBid, true)
	if price.Sign() <= 0 || !new(big.Rat).Quo(price, newStep).IsInt() ||
		(d.Side == "BUY" && ask.Cmp(price) > 0) || (d.Side == "SELL" && bid.Cmp(price) < 0) {
		return ScheduledGenerationResult{}, ErrNotAuthorized
	}
	budget, _ := decimal(d.ProposedNotional, true)
	fee, _ := decimal(fresh.FeeAllowanceUSD, false)
	if d.Side == "BUY" {
		cash, ok := decimal(c.Facts.CashUSD, false)
		if !ok {
			return ScheduledGenerationResult{}, ErrInvalid
		}
		if cash.Cmp(budget) < 0 {
			budget = cash
		}
	}
	budget = new(big.Rat).Sub(budget, fee)
	if budget.Sign() <= 0 {
		return ScheduledGenerationResult{}, ErrNotAuthorized
	}
	qty := new(big.Rat).Quo(budget, price)
	if d.Side == "SELL" {
		base, ok := decimal(c.Facts.AcquiredBase, false)
		if !ok {
			return ScheduledGenerationResult{}, ErrInvalid
		}
		if base.Cmp(qty) < 0 {
			qty = base
		}
	}
	baseStep, _ := decimal(fresh.BaseIncrement, true)
	qty = generationStep(qty, baseStep)
	gross := new(big.Rat).Mul(qty, price)
	baseMin, _ := decimal(fresh.BaseMinSize, true)
	baseMax, _ := decimal(fresh.BaseMaxSize, true)
	quoteMin, _ := decimal(fresh.QuoteMinSize, true)
	quoteMax, _ := decimal(fresh.QuoteMaxSize, true)
	if qty.Sign() <= 0 || qty.Cmp(baseMin) < 0 || qty.Cmp(baseMax) > 0 || gross.Cmp(quoteMin) < 0 || gross.Cmp(quoteMax) > 0 ||
		(d.Side == "SELL" && gross.Cmp(fee) <= 0) {
		return ScheduledGenerationResult{}, ErrNotAuthorized
	}
	p := ScheduledProposal{MandateApprovalID: c.Slot.MandateApprovalID, MandateID: c.Slot.MandateID, MandateVersion: c.Slot.MandateVersion,
		ScheduledFor: c.Slot.ScheduledFor, SourceMode: "LIVE", AIConnectionID: c.Facts.AIConnectionID, AIModelID: c.Facts.AIModelID,
		Side: d.Side, BaseSize: canonical(qty), LimitPrice: canonical(price), FeeAllowanceUSD: fresh.FeeAllowanceUSD, MaximumDebitUSD: "0"}
	if d.Side == "BUY" {
		p.MaximumDebitUSD = canonical(new(big.Rat).Add(gross, fee))
	}
	r := generationRequest(c, p)
	digest, err := requestDigest(r)
	if err != nil {
		return ScheduledGenerationResult{}, err
	}
	return ScheduledGenerationResult{Outcome: "PROPOSE", Decision: d, Proposal: &p, RequestDigest: digest, ClientOrderID: r.ClientOrderID, Market: &fresh}, nil
}

func generationRequest(c ScheduledGenerationClaim, p ScheduledProposal) Request {
	pilot := c.Facts.Pilot
	return Request{OwnerID: pilot.OwnerID, AccountID: pilot.AccountID, ConnectionID: pilot.ConnectionID, CapitalBucketID: pilot.CapitalBucketID,
		ClientOrderID: scheduledProposalClientID(pilot.CapitalBucketID, c.Slot.MandateID, c.Slot.MandateVersion, c.Slot.ScheduledFor), ProductID: pilot.ProductID,
		Side: p.Side, BaseSize: p.BaseSize, LimitPrice: p.LimitPrice, FeeAllowanceUSD: p.FeeAllowanceUSD, MaximumDebitUSD: p.MaximumDebitUSD,
		PilotLimits: &pilot.Limits, MandateApprovalID: c.Slot.MandateApprovalID}
}

func generationStep(value, step *big.Rat) *big.Rat {
	units := new(big.Rat).Quo(value, step)
	n := new(big.Int).Quo(units.Num(), units.Denom())
	return new(big.Rat).Mul(new(big.Rat).SetInt(n), step)
}

func validateScheduledGenerationResult(c ScheduledGenerationClaim, r ScheduledGenerationResult) error {
	if r.Outcome == "FAILED" || r.Outcome == "UNKNOWN" {
		if r.Proposal != nil || r.Market != nil || r.RequestDigest != "" || r.ClientOrderID != "" || !reflect.DeepEqual(r.Decision, neural.LivePilotDecision{}) {
			return ErrInvalid
		}
		return nil
	}
	if validateGenerationDecision(c, r.Decision) != nil || r.Outcome != r.Decision.Decision {
		return ErrInvalid
	}
	if r.Outcome == "ABSTAIN" {
		if r.Proposal != nil || r.Market != nil || r.RequestDigest != "" || r.ClientOrderID != "" {
			return ErrInvalid
		}
		return nil
	}
	if r.Market == nil || r.Proposal == nil {
		return ErrInvalid
	}
	expected, err := deriveScheduledGenerationResult(c, r.Decision, *r.Market)
	if err != nil || !reflect.DeepEqual(expected, r) {
		return ErrInvalid
	}
	return nil
}
