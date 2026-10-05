package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestPilotLimitsBindExactCapAndExpiry(t *testing.T) {
	r := requestFixture()
	r.PilotLimits.MaximumOrderUSD = "60.60"
	digest, err := requestDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*OwnerPilotLimits){
		func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "61" },
		func(l *OwnerPilotLimits) { l.ExpiresAt = l.ExpiresAt.Add(time.Microsecond) },
	} {
		copy := *r.PilotLimits
		change(&copy)
		changed := r
		changed.PilotLimits = &copy
		got, err := requestDigest(changed)
		if err != nil || got == digest || samePilotLimits(changed.PilotLimits, *r.PilotLimits) {
			t.Fatal("limits not included in immutable approval terms", err)
		}
	}
}

func TestPilotCapIncludesFeesAndExactDecimalBoundary(t *testing.T) {
	for _, side := range []string{"BUY", "SELL"} {
		t.Run(side, func(t *testing.T) {
			r := requestFixture()
			r.Side = side
			if side == "SELL" {
				r.MaximumDebitUSD = "0"
			}
			r.PilotLimits.MaximumOrderUSD = "60.60"
			if _, err := requestDigest(r); err != nil {
				t.Fatal("exact inclusive boundary denied", err)
			}
			for _, cap := range []string{"60", "60.599999999999999999"} {
				r.PilotLimits.MaximumOrderUSD = cap
				if _, err := requestDigest(r); !errors.Is(err, ErrInvalid) {
					t.Fatal("fee or precise overage escaped cap", cap, err)
				}
			}
		})
	}
	r := requestFixture()
	r.PilotLimits.MaximumOrderUSD = "60.60"
	r.MaximumDebitUSD = "60.600000000000000001"
	if _, err := requestDigest(r); !errors.Is(err, ErrInvalid) {
		t.Fatal("BUY tested only notional rather than maximum debit", err)
	}
}

func TestPilotLimitsDenyMissingOrMalformedConfigurationWithoutIO(t *testing.T) {
	for name, change := range map[string]func(*OwnerPilotLimits){
		"missing cap":    func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "" },
		"zero cap":       func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "0" },
		"negative":       func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "-1" },
		"exponent":       func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "1e2" },
		"leading zero":   func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "060.60" },
		"nonfinite":      func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "NaN" },
		"overprecision":  func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "100.0000000000000000001" },
		"oversized":      func(l *OwnerPilotLimits) { l.MaximumOrderUSD = "1000000000000000000" },
		"missing expiry": func(l *OwnerPilotLimits) { l.ExpiresAt = time.Time{} },
		"submicrosecond": func(l *OwnerPilotLimits) { l.ExpiresAt = l.ExpiresAt.Add(time.Nanosecond) },
		"non UTC":        func(l *OwnerPilotLimits) { l.ExpiresAt = l.ExpiresAt.In(time.FixedZone("offset", 3600)) },
		"invalid year":   func(l *OwnerPilotLimits) { l.ExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
	} {
		t.Run(name, func(t *testing.T) {
			r := requestFixture()
			change(r.PilotLimits)
			if validPilotLimits(r.PilotLimits) {
				t.Fatal("accepted malformed limits")
			}
			if _, err := NewPostgresStore(ownerNoDatabase{}).Prepare(context.Background(), r); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("invalid new order touched database", err)
			}
			if _, err := NewOwnerWorkflow(NewPostgresStore(ownerNoDatabase{}), ownerScopeFor(r), ownerNoIODependencies()); !errors.Is(err, ErrNotAuthorized) {
				t.Fatal("invalid trusted composition accepted", err)
			}
		})
	}
	r := requestFixture()
	r.PilotLimits = nil
	if _, err := NewPostgresStore(ownerNoDatabase{}).Prepare(context.Background(), r); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("absent new-order limits accepted", err)
	}
}

func TestHistoricalRequestDigestRemainsUnchangedForRecovery(t *testing.T) {
	r := requestFixture()
	r.PilotLimits = nil
	// Exact original v1 request encoding, before PilotLimits existed.
	const legacy = `{"OwnerID":"11111111-1111-4111-8111-111111111111","AccountID":"22222222-2222-4222-8222-222222222222","ConnectionID":"33333333-3333-4333-8333-333333333333","CapitalBucketID":"44444444-4444-4444-8444-444444444444","ClientOrderID":"55555555-5555-4555-8555-555555555555","ProductID":"BTC-USD","Side":"BUY","BaseSize":"0.001","LimitPrice":"60000","FeeAllowanceUSD":"0.60","MaximumDebitUSD":"60.60"}`
	body, err := json.Marshal(r)
	if err != nil || string(body) != legacy {
		t.Fatal("historical encoding changed", err)
	}
	expected := sha256.Sum256([]byte("arbion-coinbase-limit-ioc-v1\x00" + legacy))
	digest, err := requestDigest(r)
	if err != nil || digest != hex.EncodeToString(expected[:]) {
		t.Fatal("historical approval digest changed", err)
	}
	// Expiry is an admission check, not a reason to lose immutable history or
	// prevent constructing the workflow needed for recovery/cancellation.
	r = requestFixture()
	r.PilotLimits.ExpiresAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := requestDigest(r); err != nil {
		t.Fatal("expired history became unreadable", err)
	}
	if _, err := NewOwnerWorkflow(NewPostgresStore(ownerNoDatabase{}), ownerScopeFor(r), ownerNoIODependencies()); err != nil {
		t.Fatal("expired scope prevented recovery composition", err)
	}
}
