package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

func TestConcreteAuthoritiesRejectWrongConsentBranchBeforeDatabase(t *testing.T) {
	r := requestFixture()
	owner := NewOwnerAuthority(preflightFunc(nil))
	mandate := NewMandateAuthority(preflightFunc(nil))
	for _, claimed := range []bool{false, true} {
		if _, err := mandate.check(context.Background(), nil, Order{Request: r}, claimed); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("mandate authority accepted owner order", err)
		}
		r.MandateApprovalID = r.OwnerID
		if _, err := owner.check(context.Background(), nil, Order{Request: r}, claimed); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("owner authority accepted autonomous order", err)
		}
		r.MandateApprovalID = "invalid"
		if _, err := mandate.check(context.Background(), nil, Order{Request: r}, claimed); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("mandate authority accepted invalid consent", err)
		}
		r.MandateApprovalID = ""
	}
	for _, authority := range []Authority{(*OwnerAuthority)(nil), NewOwnerAuthority(nil), (*MandateAuthority)(nil), NewMandateAuthority(nil)} {
		if _, err := authority.AuthorizeDispatch(context.Background(), nil, Order{Request: r}, time.Now()); !errors.Is(err, ErrNotAuthorized) {
			t.Fatal("missing concrete authority dependency accepted", err)
		}
	}
}

func TestOwnerFundingRequiresExactAvailableFunds(t *testing.T) {
	now := time.Now().UTC()
	r := requestFixture()
	d, _ := requestDigest(r)
	o := Order{ID: r.ClientOrderID, Request: r, RequestDigest: d, CreatedAt: now.Add(-time.Second)}
	p := VerifiedPreflight{EvidenceID: r.ClientOrderID, RequestDigest: d, AccountID: r.AccountID, ConnectionID: r.ConnectionID, ReconciliationID: r.OwnerID, CredentialGeneration: 2, ObservedAt: now, ExpiresAt: now.Add(30 * time.Second), CashUSD: "1000", AvailableCashUSD: "60.60", TotalBase: "1", AvailableBase: "0.001"}
	b := risk.CapitalBucket{ID: r.CapitalBucketID, UserID: r.OwnerID, AccountID: r.AccountID, AllocationType: "FIXED_AMOUNT", AllocationValue: "100", Currency: "USD", ProtectedAmount: "0", Status: "ACTIVE"}
	for _, side := range []string{"BUY", "SELL"} {
		o.Request.Side = side
		if side == "SELL" {
			o.Request.MaximumDebitUSD = "0"
		}
		x := evaluateOwnerFunding(o, p, b, now)
		if x.Decision != risk.Allow || !x.ApprovalRequired || x.PlatformExecutionAvailable || x.Mode != "MANUAL_PROPOSAL" {
			t.Fatalf("owner risk boundary: %#v", x)
		}
		limited := p
		if side == "BUY" {
			limited.AvailableCashUSD = "60.599999999999999999"
		} else {
			limited.AvailableBase = "0.000999999999999999"
		}
		if x = evaluateOwnerFunding(o, limited, b, now); x.Decision != risk.Deny {
			t.Fatal("total funds substituted for available funds", side)
		}
	}
	o.Request = r
	for name, mutate := range map[string]func(*VerifiedPreflight){
		"stale":                   func(p *VerifiedPreflight) { p.ObservedAt = now.Add(-31 * time.Second) },
		"future":                  func(p *VerifiedPreflight) { p.ObservedAt = now.Add(time.Nanosecond) },
		"expired":                 func(p *VerifiedPreflight) { p.ExpiresAt = now },
		"long lifetime":           func(p *VerifiedPreflight) { p.ExpiresAt = now.Add(time.Minute) },
		"wrong generation":        func(p *VerifiedPreflight) { p.CredentialGeneration++ },
		"wrong account":           func(p *VerifiedPreflight) { p.AccountID = r.OwnerID },
		"wrong connection":        func(p *VerifiedPreflight) { p.ConnectionID = r.OwnerID },
		"wrong order":             func(p *VerifiedPreflight) { p.RequestDigest = "changed" },
		"wrong reconciliation":    func(p *VerifiedPreflight) { p.ReconciliationID = r.AccountID },
		"cash exceeds total":      func(p *VerifiedPreflight) { p.AvailableCashUSD = "1001" },
		"inventory exceeds total": func(p *VerifiedPreflight) { p.AvailableBase = "2" },
		"NaN":                     func(p *VerifiedPreflight) { p.AvailableCashUSD = "NaN" },
		"negative":                func(p *VerifiedPreflight) { p.AvailableBase = "-1" },
		"fraction expression":     func(p *VerifiedPreflight) { p.CashUSD = "1/2" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := p
			mutate(&bad)
			if validPreflight(bad, o, 2, r.OwnerID, now) {
				t.Fatal("invalid proof accepted")
			}
		})
	}
	if !validPreflight(p, o, 2, r.OwnerID, now) {
		t.Fatal("valid proof rejected")
	}
}
