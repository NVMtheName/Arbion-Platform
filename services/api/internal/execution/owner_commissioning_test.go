package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/authorization"
)

func ownerCommissioningTermsFixture() OwnerCommissioningTerms {
	r := requestFixture()
	from := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	return OwnerCommissioningTerms{
		Pilot: PilotAllocation{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID, CapitalBucketID: r.CapitalBucketID,
			ProductID: "BTC-USD", InitialCashUSD: "100", Limits: OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: from.Add(time.Hour)}},
		AIConnectionID: r.ClientOrderID, AIModelID: "gpt-5.4", Objective: "Synthetic bounded spot decisions",
		IntervalMinutes: 30, MaxTradesPerDay: 2, MaxCapitalDeployedUSD: "100", MaxSinglePositionUSD: "25", MinimumCashReserveUSD: "0", EffectiveFrom: from,
	}
}

func changedCommissioningTerms() map[string]func(*OwnerCommissioningTerms) {
	return map[string]func(*OwnerCommissioningTerms){
		"owner":         func(v *OwnerCommissioningTerms) { v.Pilot.OwnerID = v.AIConnectionID },
		"account":       func(v *OwnerCommissioningTerms) { v.Pilot.AccountID = v.AIConnectionID },
		"connection":    func(v *OwnerCommissioningTerms) { v.Pilot.ConnectionID = v.AIConnectionID },
		"bucket":        func(v *OwnerCommissioningTerms) { v.Pilot.CapitalBucketID = v.AIConnectionID },
		"product":       func(v *OwnerCommissioningTerms) { v.Pilot.ProductID = "ETH-USD" },
		"allocation":    func(v *OwnerCommissioningTerms) { v.Pilot.InitialCashUSD = "100.0" },
		"order cap":     func(v *OwnerCommissioningTerms) { v.Pilot.Limits.MaximumOrderUSD = "24" },
		"expiry":        func(v *OwnerCommissioningTerms) { v.Pilot.Limits.ExpiresAt = v.Pilot.Limits.ExpiresAt.Add(time.Second) },
		"AI connection": func(v *OwnerCommissioningTerms) { v.AIConnectionID = v.Pilot.ConnectionID },
		"AI model":      func(v *OwnerCommissioningTerms) { v.AIModelID = "different-model" },
		"objective":     func(v *OwnerCommissioningTerms) { v.Objective += "." },
		"cadence":       func(v *OwnerCommissioningTerms) { v.IntervalMinutes = 60 },
		"trade count":   func(v *OwnerCommissioningTerms) { v.MaxTradesPerDay = 3 },
		"capital cap":   func(v *OwnerCommissioningTerms) { v.MaxCapitalDeployedUSD = "99" },
		"position cap":  func(v *OwnerCommissioningTerms) { v.MaxSinglePositionUSD = "24" },
		"cash reserve":  func(v *OwnerCommissioningTerms) { v.MinimumCashReserveUSD = "1" },
		"anchor":        func(v *OwnerCommissioningTerms) { v.EffectiveFrom = v.EffectiveFrom.Add(-time.Second) },
	}
}

func commissioningCommands(w *OwnerCommissioning, p authorization.Principal) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"review":       func(c context.Context) error { _, e := w.Review(c, p); return e },
		"prepare":      func(c context.Context) error { _, e := w.Prepare(c, p, "stale"); return e },
		"read":         func(c context.Context) error { _, e := w.Read(c, p); return e },
		"approve":      func(c context.Context) error { _, e := w.Approve(c, p, "stale", "stale", "synthetic"); return e },
		"read consent": func(c context.Context) error { _, e := w.ReadConsent(c, p, requestFixture().ClientOrderID); return e },
		"revoke":       func(c context.Context) error { return w.Revoke(c, p, requestFixture().ClientOrderID) },
	}
}

func TestOwnerCommissioningRejectsForeignPrincipalWithoutIO(t *testing.T) {
	terms := ownerCommissioningTermsFixture()
	w, err := NewOwnerCommissioning(NewPostgresStore(ownerNoDatabase{}), terms, ownerNoIO{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []authorization.Principal{{}, {UserID: terms.Pilot.AccountID, Entitlement: authorization.EntitlementFounder}, {UserID: terms.Pilot.OwnerID}} {
		for name, call := range commissioningCommands(w, p) {
			t.Run(name, func(t *testing.T) {
				if err := call(context.Background()); !errors.Is(err, ErrNotAuthorized) {
					t.Fatal("foreign principal reached commissioning", err)
				}
			})
		}
	}
}

func TestOwnerCommissioningReviewPinsEveryTermWithoutMutation(t *testing.T) {
	terms := ownerCommissioningTermsFixture()
	read := func(v OwnerCommissioningTerms) OwnerCommissioningReview {
		t.Helper()
		w, err := NewOwnerCommissioning(NewPostgresStore(ownerPresentationDatabase{true}), v, ownerNoIO{})
		if err != nil {
			t.Fatal(err)
		}
		review, err := w.Review(context.Background(), authorization.Principal{UserID: v.Pilot.OwnerID, Entitlement: authorization.EntitlementFounder})
		if err != nil {
			t.Fatal(err)
		}
		return review
	}
	first := read(terms)
	if !reflect.DeepEqual(first.Terms, terms) || len(first.TermsDigest) != 64 || !validUUID(first.MandateID) || first.MandateVersion != 1 || first != read(terms) {
		t.Fatal("review omitted terms or lacked stable identity", first)
	}
	for name, mutate := range changedCommissioningTerms() {
		t.Run(name, func(t *testing.T) {
			changed := terms
			mutate(&changed)
			got := read(changed)
			if got.TermsDigest == first.TermsDigest {
				t.Fatal("review did not bind changed term")
			}
			if name != "owner" && name != "account" && got.MandateID != first.MandateID {
				t.Fatal("changed terms minted replacement identity")
			}
		})
	}
	local := terms
	zone := time.FixedZone("fixture", -4*60*60)
	local.EffectiveFrom = local.EffectiveFrom.In(zone)
	local.Pilot.Limits.ExpiresAt = local.Pilot.Limits.ExpiresAt.In(zone)
	if first != read(local) {
		t.Fatal("timezone representation changed immutable terms")
	}
}

func TestOwnerCommissioningConstructorRejectsIncompleteOrInvalidPolicy(t *testing.T) {
	valid := ownerCommissioningTermsFixture()
	for name, mutate := range map[string]func(*OwnerCommissioningTerms){
		"unset owner":         func(v *OwnerCommissioningTerms) { v.Pilot.OwnerID = "" },
		"wrong pair":          func(v *OwnerCommissioningTerms) { v.Pilot.ProductID = "BTC-EUR" },
		"USD pair":            func(v *OwnerCommissioningTerms) { v.Pilot.ProductID = "USD-USD" },
		"zero budget":         func(v *OwnerCommissioningTerms) { v.Pilot.InitialCashUSD = "0" },
		"cap over budget":     func(v *OwnerCommissioningTerms) { v.Pilot.Limits.MaximumOrderUSD = "101" },
		"AI connection":       func(v *OwnerCommissioningTerms) { v.AIConnectionID = "bad" },
		"AI model":            func(v *OwnerCommissioningTerms) { v.AIModelID = "" },
		"missing objective":   func(v *OwnerCommissioningTerms) { v.Objective = "  " },
		"oversized objective": func(v *OwnerCommissioningTerms) { v.Objective = strings.Repeat("a", 2001) },
		"missing anchor":      func(v *OwnerCommissioningTerms) { v.EffectiveFrom = time.Time{} },
		"fractional anchor":   func(v *OwnerCommissioningTerms) { v.EffectiveFrom = v.EffectiveFrom.Add(time.Millisecond) },
		"fractional expiry": func(v *OwnerCommissioningTerms) {
			v.Pilot.Limits.ExpiresAt = v.Pilot.Limits.ExpiresAt.Add(time.Millisecond)
		},
		"expired window": func(v *OwnerCommissioningTerms) { v.Pilot.Limits.ExpiresAt = v.EffectiveFrom },
		"unbounded window": func(v *OwnerCommissioningTerms) {
			v.Pilot.Limits.ExpiresAt = v.EffectiveFrom.Add(24*time.Hour + time.Second)
		},
		"fast cadence":             func(v *OwnerCommissioningTerms) { v.IntervalMinutes = 29 },
		"unbounded cadence":        func(v *OwnerCommissioningTerms) { v.IntervalMinutes = 1441 },
		"missing trade limit":      func(v *OwnerCommissioningTerms) { v.MaxTradesPerDay = 0 },
		"trade limit":              func(v *OwnerCommissioningTerms) { v.MaxTradesPerDay = 49 },
		"missing capital cap":      func(v *OwnerCommissioningTerms) { v.MaxCapitalDeployedUSD = "" },
		"position over allocation": func(v *OwnerCommissioningTerms) { v.MaxSinglePositionUSD = "101" },
		"negative reserve":         func(v *OwnerCommissioningTerms) { v.MinimumCashReserveUSD = "-1" },
		"reserve over allocation":  func(v *OwnerCommissioningTerms) { v.MinimumCashReserveUSD = "101" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := valid
			mutate(&bad)
			if _, err := NewOwnerCommissioning(NewPostgresStore(ownerNoDatabase{}), bad, ownerNoIO{}); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid commissioning terms accepted", err)
			}
		})
	}
	if _, err := NewOwnerCommissioning(nil, valid, ownerNoIO{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil store accepted", err)
	}
	if _, err := NewOwnerCommissioning(NewPostgresStore(ownerNoDatabase{}), valid, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing step-up accepted", err)
	}
}
