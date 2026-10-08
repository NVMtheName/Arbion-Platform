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
	"github.com/jackc/pgx/v5/pgxpool"
)

func newScheduledGenerationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (mandateConsentFixture, ScheduledGenerationSlot, ScheduledGenerationMarket) {
	t.Helper()
	c := newMandateConsentSetup(t, ctx, pool)
	// Use an existing explicit model route, without changing the legacy fixture
	// or treating its historical version as the new current policy.
	if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET ai_model_id='gpt-5.6-sol' WHERE id=$1`, c.mandateID); err != nil {
		t.Fatal(err)
	}
	c = renewScheduledMandateFixture(t, ctx, pool, c, time.Time{})
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	slot := ScheduledGenerationSlot{MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: c.consent.MandateVersion, ScheduledFor: p.ScheduledFor}
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return c, slot, generationPostgresMarket(now.UTC())
}

func generationPostgresMarket(now time.Time) ScheduledGenerationMarket {
	return ScheduledGenerationMarket{ProductID: "BTC-USD", ProductType: "SPOT", BaseCurrency: "BTC", QuoteCurrency: "USD", Status: "online",
		BaseIncrement: "0.00000001", PriceIncrement: "0.01", BaseMinSize: "0.00000001", BaseMaxSize: "100", QuoteMinSize: "1", QuoteMaxSize: "1000000",
		BestBid: "59999", BestAsk: "60000", FeeAllowanceUSD: "0.06", StartedAt: now.Add(-time.Millisecond), CompletedAt: now, ObservedAt: now}
}

func scheduledGenerationTestDecision(c ScheduledGenerationClaim, outcome string) neural.LivePilotDecision {
	d := neural.LivePilotDecision{Decision: outcome, Symbol: "NONE", Side: "NONE", ProposedNotional: "0", Confidence: "LOW", Thesis: "Synthetic bounded test decision",
		RiskFlags: []string{}, Limitations: []string{}, Metadata: neural.InsightMetadata{Provider: "openai", Model: c.Facts.AIModelID, Profile: c.Facts.Profile, RequestID: "synthetic-generation"}}
	if outcome == "PROPOSE" {
		d.Symbol, d.Side, d.ProposedNotional = "BTC", "BUY", "6.06"
	}
	return d
}

func TestPostgresScheduledGenerationConcurrentClaimAndRenewalCannotReroll(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
	type outcome struct {
		claim ScheduledGenerationClaim
		err   error
	}
	results := make(chan outcome, 6)
	var callbacks atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, admitted, err := NewPostgresStore(pool).ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
			if admitted && err == nil {
				callbacks.Add(1) // stand-in only; no real model or provider call
			}
			results <- outcome{claim, err}
		}()
	}
	wg.Wait()
	close(results)
	var first ScheduledGenerationClaim
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if first.ID == "" {
			first = got.claim
		} else if !reflect.DeepEqual(first, got.claim) {
			t.Fatal("same slot returned different frozen claims")
		}
	}
	if callbacks.Load() != 1 || first.Facts.CashUSD != "100" || first.Facts.AcquiredBase != "0" || len(first.Input.Positions) != 0 || first.Input.AvailableCashUSD != "100" {
		t.Fatal("claim admission or isolated pilot input mismatch", callbacks.Load(), first)
	}
	input, _ := json.Marshal(first.Input)
	for _, private := range []string{c.request.OwnerID, c.request.AccountID, c.request.ConnectionID, c.request.CapitalBucketID, c.aiConnectionID, c.consent.ID} {
		if strings.Contains(string(input), private) {
			t.Fatal("internal identity leaked into model input")
		}
	}
	s := NewPostgresStore(pool)
	renewed, err := s.ApproveMandate(ctx, c.principal, c.request.CapitalBucketID, c.mandateID, slot.MandateVersion, c.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	changed := slot
	changed.MandateApprovalID = renewed.ID
	if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, changed, market); admitted || !errors.Is(err, ErrConflict) {
		t.Fatal("renewable consent rerolled immutable slot", admitted, err)
	}
	if err := s.RevokeMandateConsent(ctx, c.request.OwnerID, c.consent.ID); err != nil {
		t.Fatal(err)
	}
	got, err := NewPostgresStore(pool).ReadScheduledGeneration(ctx, c.request.OwnerID, changed)
	if err != nil || !reflect.DeepEqual(got.Claim, first) || got.Result != nil {
		t.Fatal("historical claim recovery changed after consent renewal/revocation", err)
	}
	if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, ScheduledGenerationMarket{}); err != nil || admitted {
		t.Fatal("missing result or changed quote admitted another callback", admitted, err)
	}
}

func TestPostgresScheduledGenerationLostClaimCommitNeverAdmitsCallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
	claim, admitted, err := NewPostgresStore(lostCommitDB{pool}).ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
	if admitted || !errors.Is(err, ErrCommitUnknown) || claim.ID == "" {
		t.Fatal("ambiguous claim commit admitted a model call", admitted, err)
	}
	s := NewPostgresStore(pool)
	recovered, err := s.ReadScheduledGeneration(ctx, c.request.OwnerID, slot)
	if err != nil || !reflect.DeepEqual(recovered.Claim, claim) || recovered.Result != nil {
		t.Fatal("lost claim recovery changed identity", err)
	}
	if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market); err != nil || admitted {
		t.Fatal("restart reclaimed a missing-result claim", admitted, err)
	}
	var claims, orders, attempts int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_generation_claims WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_orders WHERE owner_id=$1),(SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1)`, c.request.OwnerID).Scan(&claims, &orders, &attempts); err != nil || claims != 1 || orders != 0 || attempts != 0 {
		t.Fatal("claim created execution state", claims, orders, attempts, err)
	}
}

func TestPostgresScheduledGenerationTerminalResultsAreImmutableAndOwnerScoped(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, outcome := range []string{"ABSTAIN", "FAILED", "UNKNOWN"} {
		t.Run(outcome, func(t *testing.T) {
			c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
			s := NewPostgresStore(pool)
			claim, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
			if err != nil || !admitted {
				t.Fatal(admitted, err)
			}
			result := ScheduledGenerationResult{Outcome: outcome}
			if outcome == "ABSTAIN" {
				result.Decision = scheduledGenerationTestDecision(claim, outcome)
			}
			first, err := s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, result)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.RevokeMandateConsent(ctx, c.request.OwnerID, c.consent.ID); err != nil {
				t.Fatal(err)
			}
			replayed, err := s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, result)
			if err != nil || !reflect.DeepEqual(first, replayed) {
				t.Fatal("terminal result did not replay exactly", err)
			}
			changed := ScheduledGenerationResult{Outcome: "FAILED"}
			if outcome == "FAILED" {
				changed.Outcome = "UNKNOWN"
			}
			if _, err = s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, changed); !errors.Is(err, ErrConflict) {
				t.Fatal("terminal outcome could be overwritten", err)
			}
			foreign := newExecutionFixture(t, ctx, pool)
			if _, err = s.ReadScheduledGeneration(ctx, foreign.OwnerID, slot); !errors.Is(err, ErrNotFound) {
				t.Fatal("foreign owner read generation", err)
			}
			if _, err = s.RecordScheduledGenerationResult(ctx, foreign.OwnerID, claim.ID, result); !errors.Is(err, ErrNotFound) {
				t.Fatal("foreign owner wrote generation result", err)
			}
			for _, query := range []string{
				`UPDATE execution_generation_claims SET expires_at=expires_at+interval '1 second' WHERE id=$1`,
				`DELETE FROM execution_generation_claims WHERE id=$1`,
				`UPDATE execution_generation_results SET outcome='FAILED' WHERE claim_id=$1`,
				`DELETE FROM execution_generation_results WHERE claim_id=$1`,
			} {
				if _, err = pool.Exec(ctx, query, claim.ID); err == nil {
					t.Fatal("immutable generation history could be changed", query)
				}
			}
			if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market); err != nil || admitted {
				t.Fatal("terminal outcome admitted another callback", admitted, err)
			}
		})
	}
	for _, query := range []string{`TRUNCATE execution_generation_results`, `TRUNCATE execution_generation_claims CASCADE`} {
		if _, err := pool.Exec(ctx, query); err == nil {
			t.Fatal("generation history could be truncated", query)
		}
	}
}

func TestPostgresScheduledGenerationProposalResultAndIntakeRecoverExactly(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	claim, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
	if err != nil || !admitted {
		t.Fatal(admitted, err)
	}
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	fresh := generationPostgresMarket(now.UTC())
	fresh.StartedAt = fresh.CompletedAt
	result, err := deriveScheduledGenerationResult(claim, scheduledGenerationTestDecision(claim, "PROPOSE"), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewPostgresStore(lostCommitDB{pool}).RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, result); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("lost result commit treated as certain", err)
	}
	saved, err := s.ReadScheduledGeneration(ctx, c.request.OwnerID, slot)
	if err != nil || saved.Result == nil || !reflect.DeepEqual(*saved.Result, result) {
		t.Fatal("persisted proposal result could not be recovered", err)
	}
	if _, err = NewPostgresStore(lostCommitDB{pool}).PrepareScheduledProposal(ctx, c.request.OwnerID, *saved.Result.Proposal); !errors.Is(err, ErrCommitUnknown) {
		t.Fatal("lost intake commit treated as certain", err)
	}
	first, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, *saved.Result.Proposal)
	if err != nil || first.RequestDigest != result.RequestDigest || first.Request.ClientOrderID != result.ClientOrderID {
		t.Fatal("recovered intake changed frozen proposal terms", err)
	}
	if err = s.RevokeMandateConsent(ctx, c.request.OwnerID, c.consent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, result); err != nil {
		t.Fatal("current revocation blocked exact terminal history replay", err)
	}
	second, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, *saved.Result.Proposal)
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatal("intake recovery replaced historical order", err)
	}
	var orders, attempts, consents int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_orders WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1),(SELECT count(*) FROM execution_mandate_approvals WHERE owner_id=$1)`, c.request.OwnerID).Scan(&orders, &attempts, &consents); err != nil || orders != 1 || attempts != 0 || consents != 1 {
		t.Fatal("proposal recovery changed execution or consent counts", orders, attempts, consents, err)
	}
}

func TestPostgresScheduledGenerationRejectsCurrentDriftAndInvalidEvidenceBeforeClaim(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, test := range []string{"revoke", "version", "off-grid", "stale quote", "missing fee", "existing proposal"} {
		t.Run(test, func(t *testing.T) {
			c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
			s := NewPostgresStore(pool)
			switch test {
			case "revoke":
				if err := s.RevokeMandateConsent(ctx, c.request.OwnerID, c.consent.ID); err != nil {
					t.Fatal(err)
				}
			case "version":
				renewScheduledMandateFixture(t, ctx, pool, c, time.Time{})
			case "off-grid":
				slot.ScheduledFor = slot.ScheduledFor.Add(time.Second)
			case "stale quote":
				market.ObservedAt = market.ObservedAt.Add(-time.Minute)
			case "missing fee":
				market.FeeAllowanceUSD = ""
			case "existing proposal":
				p := newScheduledProposalInput(t, ctx, pool, c, c.request)
				if _, err := s.PrepareScheduledProposal(ctx, c.request.OwnerID, p); err != nil {
					t.Fatal(err)
				}
			}
			if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market); err == nil || admitted {
				t.Fatal("invalid generation admitted a callback", admitted, err)
			}
			var claims int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_generation_claims WHERE owner_id=$1`, c.request.OwnerID).Scan(&claims); err != nil || claims != 0 {
				t.Fatal("denied generation persisted a claim", claims, err)
			}
		})
	}
}

func TestPostgresScheduledGeneratorPublicCoordinatorPersistsAndRecoversWithoutExecution(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, slot, _ := newScheduledGenerationFixture(t, ctx, pool)
	var marketCalls, modelCalls int
	market := generationMarketFunc(func(_ context.Context, owner, account, product string) (ScheduledGenerationMarket, error) {
		marketCalls++
		if owner != c.request.OwnerID || account != c.request.AccountID || product != c.request.ProductID {
			t.Fatal("collector received changed financial scope")
		}
		m := generationPostgresMarket(time.Now().UTC())
		m.StartedAt = m.CompletedAt
		return m, nil
	})
	model := generationDecisionFunc(func(_ context.Context, principal authorization.Principal, connection, model string, input neural.LivePilotDecisionRequest) (neural.LivePilotDecision, error) {
		modelCalls++
		if principal != c.principal || connection != c.aiConnectionID || model != "gpt-5.6-sol" || input.Profile != "deep" || input.AvailableCashUSD != "100" || len(input.Positions) != 0 {
			t.Fatal("generation received changed scope or non-pilot funding")
		}
		return neural.LivePilotDecision{Decision: "PROPOSE", Symbol: "BTC", Side: "BUY", ProposedNotional: "6.06", Confidence: "LOW",
			Thesis: "Synthetic test decision only", RiskFlags: []string{}, Limitations: []string{},
			Metadata: neural.InsightMetadata{Provider: "openai", Model: model, Profile: input.Profile, RequestID: "synthetic-joined-generation"}}, nil
	})
	g, err := NewScheduledGenerator(NewPostgresStore(pool), market, model)
	if err != nil {
		t.Fatal(err)
	}
	first, order, err := g.Generate(ctx, c.principal, slot)
	if err != nil || first.Result == nil || first.Result.Outcome != "PROPOSE" || order == nil || modelCalls != 1 || marketCalls != 2 {
		t.Fatal("joined generation did not persist one proposal/order", first, order, modelCalls, marketCalls, err)
	}
	restarted, err := NewScheduledGenerator(NewPostgresStore(pool), market, model)
	if err != nil {
		t.Fatal(err)
	}
	replayed, replayedOrder, err := restarted.Generate(ctx, c.principal, slot)
	if err != nil || !reflect.DeepEqual(first, replayed) || !reflect.DeepEqual(order, replayedOrder) || modelCalls != 1 || marketCalls != 2 {
		t.Fatal("restart replay regenerated or changed persisted output", modelCalls, marketCalls, err)
	}
	var got [7]int
	err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_generation_claims WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_generation_results WHERE owner_id=$1),(SELECT count(*) FROM execution_orders WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_dispatch_attempts WHERE owner_id=$1),(SELECT count(*) FROM execution_authorizations WHERE owner_id=$1),
	 (SELECT count(*) FROM execution_broker_acknowledgements WHERE owner_id=$1),(SELECT count(*) FROM execution_fills WHERE owner_id=$1)`, c.request.OwnerID).
		Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5], &got[6])
	if err != nil || got != [7]int{1, 1, 1, 0, 0, 0, 0} {
		t.Fatal("generation bridge crossed the no-execution boundary", got, err)
	}
}

func TestPostgresScheduledGenerationFrozenDeadlineBlocksNewProposalButAllowsFailureHistory(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, _, _ := newScheduledGenerationFixture(t, ctx, pool)
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	c = renewScheduledMandateFixture(t, ctx, pool, c, now.Add(-117*time.Second))
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	slot := ScheduledGenerationSlot{MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: c.consent.MandateVersion, ScheduledFor: p.ScheduledFor}
	market := generationPostgresMarket(time.Now().UTC())
	s := NewPostgresStore(pool)
	claim, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
	if err != nil || !admitted {
		t.Fatal(admitted, err)
	}
	fresh := generationPostgresMarket(time.Now().UTC())
	fresh.StartedAt = fresh.CompletedAt
	result, err := deriveScheduledGenerationResult(claim, scheduledGenerationTestDecision(claim, "PROPOSE"), fresh)
	if err != nil {
		t.Fatal(err)
	}
	waitPastPilotDeadline(t, ctx, pool, claim.Facts.ExpiresAt)
	if _, err = s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, result); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("new proposal result persisted after frozen deadline", err)
	}
	failure := ScheduledGenerationResult{Outcome: "FAILED"}
	if _, err = s.RecordScheduledGenerationResult(ctx, c.request.OwnerID, claim.ID, failure); err != nil {
		t.Fatal("expiry blocked terminal no-order failure evidence", err)
	}
	if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market); err != nil || admitted {
		t.Fatal("expired terminal generation was reclaimed", admitted, err)
	}
}

func TestPostgresScheduledGenerationUnresolvedCapitalCannotBePresentedAsAvailable(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, _, _ := newScheduledGenerationFixture(t, ctx, pool)
	f := newMandateSendOrder(t, ctx, pool, c, "BUY")
	s := NewPostgresStore(pool)
	if _, err := s.Claim(ctx, c.request.OwnerID, f.order.ID, NewMandateAuthority(NewSavedPreflightVerifier(f.evidence))); err != nil {
		t.Fatal(err)
	}
	// A fresh immutable version/consent does not free the prior account hold or
	// capital reservation. No broker callback is needed to create this fixture.
	c = renewScheduledMandateFixture(t, ctx, pool, c, time.Time{})
	p := newScheduledProposalInput(t, ctx, pool, c, c.request)
	slot := ScheduledGenerationSlot{MandateApprovalID: c.consent.ID, MandateID: c.mandateID, MandateVersion: c.consent.MandateVersion, ScheduledFor: p.ScheduledFor}
	if _, err := s.GenerationFacts(ctx, c.request.OwnerID, slot); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("pending execution capital was projected as available", err)
	}
	if _, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, generationPostgresMarket(time.Now().UTC())); !errors.Is(err, ErrNotAuthorized) || admitted {
		t.Fatal("pending execution capital admitted generation", admitted, err)
	}
}

func TestPostgresScheduledGenerationDirectSQLCannotForgePilotInputOrProposalScope(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c, slot, market := newScheduledGenerationFixture(t, ctx, pool)
	s := NewPostgresStore(pool)
	facts, err := s.GenerationFacts(ctx, c.request.OwnerID, slot)
	if err != nil {
		t.Fatal(err)
	}
	input, _, err := buildScheduledGenerationInput(facts, market, facts.ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	// Even a matching SHA256 cannot relabel whole-account cash as this pilot's
	// input. No existing claim/unique-key collision can cause this denial.
	input.AvailableCashUSD, input.BuyingPowerUSD = "1000000", "1000000"
	factsJSON, _ := json.Marshal(facts)
	marketJSON, _ := json.Marshal(market)
	inputJSON, _ := json.Marshal(input)
	_, err = pool.Exec(ctx, `INSERT INTO execution_generation_claims(owner_id,financial_account_id,provider_connection_id,capital_bucket_id,
	 mandate_approval_id,mandate_id,mandate_version,scheduled_for,ai_provider_connection_id,ai_model_id,facts,market,model_input,input_digest,expires_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,encode(sha256(convert_to($13,'UTF8')),'hex'),$14)`,
		c.request.OwnerID, facts.Pilot.AccountID, facts.Pilot.ConnectionID, facts.Pilot.CapitalBucketID, slot.MandateApprovalID,
		slot.MandateID, slot.MandateVersion, slot.ScheduledFor, facts.AIConnectionID, facts.AIModelID, factsJSON, marketJSON, string(inputJSON), facts.ExpiresAt)
	if err == nil {
		t.Fatal("direct SQL forged pilot model funding")
	}
	claim, admitted, err := s.ClaimScheduledGeneration(ctx, c.request.OwnerID, slot, market)
	if err != nil || !admitted {
		t.Fatal("forged input denial left a claimed slot", admitted, err)
	}
	fresh := generationPostgresMarket(time.Now().UTC())
	fresh.StartedAt = fresh.CompletedAt
	result, err := deriveScheduledGenerationResult(claim, scheduledGenerationTestDecision(claim, "PROPOSE"), fresh)
	if err != nil {
		t.Fatal(err)
	}
	result.Proposal.MandateApprovalID = c.aiConnectionID
	body, _ := json.Marshal(result)
	if _, err = pool.Exec(ctx, `INSERT INTO execution_generation_results(claim_id,owner_id,outcome,result) VALUES($1,$2,'PROPOSE',$3)`, claim.ID, c.request.OwnerID, body); err == nil {
		t.Fatal("direct SQL attached foreign consent to generated proposal")
	}
	read, err := s.ReadScheduledGeneration(ctx, c.request.OwnerID, slot)
	if err != nil || read.Result != nil {
		t.Fatal("rejected result left terminal evidence", err)
	}
}
