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

func requestFixture() Request {
	return Request{OwnerID: "11111111-1111-4111-8111-111111111111", AccountID: "22222222-2222-4222-8222-222222222222",
		ConnectionID: "33333333-3333-4333-8333-333333333333", CapitalBucketID: "44444444-4444-4444-8444-444444444444",
		ClientOrderID: "55555555-5555-4555-8555-555555555555", ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.001",
		LimitPrice: "60000", FeeAllowanceUSD: "0.60", MaximumDebitUSD: "60.60",
		PilotLimits: &OwnerPilotLimits{MaximumOrderUSD: "100000000", ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}}
}

func TestRequestBindsExactBoundedSpotTerms(t *testing.T) {
	r := requestFixture()
	digest, err := requestDigest(r)
	if err != nil || len(digest) != 64 {
		t.Fatalf("valid request: %s %v", digest, err)
	}
	for name, mutate := range map[string]func(*Request){
		"cross account": func(r *Request) { r.AccountID = r.OwnerID },
		"side":          func(r *Request) { r.Side = "SELL"; r.MaximumDebitUSD = "0" },
		"quantity":      func(r *Request) { r.BaseSize = "0.0009" },
		"fee":           func(r *Request) { r.FeeAllowanceUSD = "0.59" },
		"price":         func(r *Request) { r.LimitPrice = "59999" },
		"client id":     func(r *Request) { r.ClientOrderID = r.OwnerID },
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			mutate(&changed)
			d, e := requestDigest(changed)
			if e != nil || d == digest {
				t.Fatalf("terms not bound: %v", e)
			}
		})
	}
	for name, mutate := range map[string]func(*Request){
		"zero id":                func(r *Request) { r.OwnerID = "00000000-0000-0000-0000-000000000000" },
		"non USD":                func(r *Request) { r.ProductID = "BTC-EUR" },
		"not a pair":             func(r *Request) { r.ProductID = "USD-USD" },
		"side":                   func(r *Request) { r.Side = "SHORT" },
		"zero":                   func(r *Request) { r.BaseSize = "0" },
		"negative":               func(r *Request) { r.LimitPrice = "-1" },
		"fraction":               func(r *Request) { r.BaseSize = "1/1000" },
		"exponent":               func(r *Request) { r.BaseSize = "1e-3" },
		"precision":              func(r *Request) { r.BaseSize = "0.0000000000000000001" },
		"negative fee":           func(r *Request) { r.FeeAllowanceUSD = "-1" },
		"fee omitted from bound": func(r *Request) { r.MaximumDebitUSD = "60" },
		"exact tiny overage":     func(r *Request) { r.MaximumDebitUSD = "60.599999999999999999" },
		"sell debit":             func(r *Request) { r.Side = "SELL" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			mutate(&changed)
			if _, e := requestDigest(changed); !errors.Is(e, ErrInvalid) {
				t.Fatalf("accepted invalid terms: %v", e)
			}
		})
	}
}

func TestRequestMandateConsentBindsDigestWithoutChangingLegacyBytes(t *testing.T) {
	r := requestFixture()
	legacy := struct {
		OwnerID, AccountID, ConnectionID, CapitalBucketID, ClientOrderID        string
		ProductID, Side, BaseSize, LimitPrice, FeeAllowanceUSD, MaximumDebitUSD string
		PilotLimits                                                             *OwnerPilotLimits `json:",omitempty"`
	}{r.OwnerID, r.AccountID, r.ConnectionID, r.CapitalBucketID, r.ClientOrderID, r.ProductID, r.Side, r.BaseSize, r.LimitPrice, r.FeeAllowanceUSD, r.MaximumDebitUSD, r.PilotLimits}
	for _, withPilot := range []bool{false, true} {
		if !withPilot {
			r.PilotLimits, legacy.PilotLimits = nil, nil
		} else {
			r.PilotLimits = requestFixture().PilotLimits
			legacy.PilotLimits = r.PilotLimits
		}
		body, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(append([]byte("arbion-coinbase-limit-ioc-v1\x00"), body...))
		got, err := requestDigest(r)
		if err != nil || got != hex.EncodeToString(hash[:]) {
			t.Fatal("empty mandate consent changed historical digest", withPilot, got, err)
		}
	}
	legacyDigest, err := requestDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	r.MandateApprovalID = r.OwnerID
	first, err := requestDigest(r)
	if err != nil || first == legacyDigest {
		t.Fatal("autonomous consent not bound", err)
	}
	r.MandateApprovalID = r.AccountID
	second, err := requestDigest(r)
	if err != nil || second == first {
		t.Fatal("changed consent not bound", err)
	}
	for _, invalid := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", "11111111-1111-4111-8111-11111111111Z"} {
		r.MandateApprovalID = invalid
		if _, err := requestDigest(r); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid mandate consent accepted", err)
		}
	}
}

func TestMissingAuthorityCannotClaimEvenWithNoDatabase(t *testing.T) {
	_, err := NewPostgresStore(nil).Claim(context.Background(), requestFixture().OwnerID, requestFixture().ClientOrderID, nil)
	if !errors.Is(err, ErrNotAuthorized) {
		t.Fatal(err)
	}
}
