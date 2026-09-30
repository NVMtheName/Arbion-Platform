package execution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func ownerStatusFixture(state string) (OwnerOrder, ownerOrderFacts) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	o := OwnerOrder{ID: "safe-order", RequestDigest: strings.Repeat("a", 64), ProductID: "BTC-USD", Side: "BUY", BaseSize: "0.001", LimitPrice: "60000", FeeAllowanceUSD: "0.6", MaximumDebitUSD: "60.6", CreatedAt: now.Add(-time.Minute), BaseFilled: "0", GrossUSD: "0", FeeUSD: "0"}
	f := ownerOrderFacts{now: now, termsValid: true, terminalMatches: true, settlementMatches: true}
	if state == "PREPARED" {
		return o, f
	}
	expires := now.Add(time.Minute)
	o.ApprovalExpiresAt, f.approval = &expires, true
	if state == "APPROVED" {
		return o, f
	}
	o.Attempted, o.AccountHeld, f.capital = true, true, true
	switch state {
	case "SUBMISSION_UNKNOWN":
		return o, f
	case "REJECTED_HELD":
		f.rejected = true
		return o, f
	case "NOT_SENT":
		o.AccountHeld, f.noSend, f.capitalReleased = false, &now, &now
		return o, f
	}
	f.acknowledged = true
	if state == "BROKER_ACKNOWLEDGED" {
		return o, f
	}
	o.FillCount, o.BaseFilled, o.GrossUSD, o.FeeUSD = 1, "0.0004", "24.000", "0.240"
	if state == "PARTIALLY_FILLED" {
		return o, f
	}
	o.TerminalStatus, o.AccountHeld = "CANCELLED", false
	if state == "SETTLED" {
		f.capitalReleased = &now
		o.Accounting = &OwnerOrderAccounting{OpeningCashUSD: "1000.00", OpeningBase: "1", ClosingCashUSD: "975.76", ClosingBase: "1.0004", RecordedAt: now}
	}
	return o, f
}

func TestOwnerOrderStatesAreInformationalAndKeepWarnings(t *testing.T) {
	for _, state := range []string{"PREPARED", "APPROVED", "SUBMISSION_UNKNOWN", "REJECTED_HELD", "BROKER_ACKNOWLEDGED", "PARTIALLY_FILLED", "AWAITING_ACCOUNT_SETTLEMENT", "SETTLED", "NOT_SENT"} {
		t.Run(state, func(t *testing.T) {
			o, f := ownerStatusFixture(state)
			got := projectOwnerOrder(o, f)
			if got.State != state || got.Summary == "" || got.Attempted != o.Attempted {
				t.Fatal("incorrect historical projection", got)
			}
			o.AccountBlocked = true
			blocked := projectOwnerOrder(o, f)
			if blocked.State != state || !blocked.AccountBlocked || !strings.Contains(blocked.Summary, "blocked for reconciliation review") {
				t.Fatal("historical state hid account quarantine", blocked)
			}
		})
	}
	for _, status := range []string{"", "UNKNOWN", "ACCEPTED", "NOT_ACCEPTED"} {
		o, f := ownerStatusFixture("PARTIALLY_FILLED")
		f.cancellation, f.cancellationReceipt = true, status
		got := projectOwnerOrder(o, f)
		want := status
		if want == "" {
			want = "UNKNOWN"
		}
		if got.State != "PARTIALLY_FILLED" || got.CancellationStatus != want || !got.CapitalHeld || !got.AccountHeld || !strings.Contains(got.Summary, "not proof of final order status") {
			t.Fatal("cancellation result claimed finality", got)
		}
	}
	for _, revoked := range []bool{false, true} {
		o, f := ownerStatusFixture("APPROVED")
		f.now, f.revoked = *o.ApprovalExpiresAt, revoked
		got := projectOwnerOrder(o, f)
		want := "EXPIRED"
		if revoked {
			want = "REVOKED"
		}
		if got.State != "PREPARED" || got.ApprovalStatus != want {
			t.Fatal("stale approval was presented as active", got)
		}
	}
	buy, f := ownerStatusFixture("SETTLED")
	buy.Side, buy.MaximumDebitUSD = "SELL", "0"
	buy.Accounting.ClosingCashUSD, buy.Accounting.ClosingBase = "1023.76", "0.9996"
	if got := projectOwnerOrder(buy, f); got.State != "SETTLED" {
		t.Fatal("exact SELL accounting was not recognized", got)
	}
}

func TestOwnerOrderInconsistentFactsAreUnavailable(t *testing.T) {
	cases := []struct {
		name, state string
		mutate      func(*OwnerOrder, *ownerOrderFacts)
	}{
		{"ack without claim", "BROKER_ACKNOWLEDGED", func(o *OwnerOrder, _ *ownerOrderFacts) { o.Attempted = false }},
		{"claim without approval", "SUBMISSION_UNKNOWN", func(o *OwnerOrder, f *ownerOrderFacts) { f.approval, o.ApprovalExpiresAt = false, nil }},
		{"claim without capital", "SUBMISSION_UNKNOWN", func(_ *OwnerOrder, f *ownerOrderFacts) { f.capital = false }},
		{"unknown without slot", "SUBMISSION_UNKNOWN", func(o *OwnerOrder, _ *ownerOrderFacts) { o.AccountHeld = false }},
		{"unproven release", "SUBMISSION_UNKNOWN", func(_ *OwnerOrder, f *ownerOrderFacts) { f.capitalReleased = &f.now }},
		{"conflicting ack and rejection", "REJECTED_HELD", func(_ *OwnerOrder, f *ownerOrderFacts) { f.acknowledged = true }},
		{"no-send with ack", "NOT_SENT", func(_ *OwnerOrder, f *ownerOrderFacts) { f.acknowledged = true }},
		{"no-send retained slot", "NOT_SENT", func(o *OwnerOrder, _ *ownerOrderFacts) { o.AccountHeld = true }},
		{"no-send unreleased capital", "NOT_SENT", func(_ *OwnerOrder, f *ownerOrderFacts) { f.capitalReleased = nil }},
		{"no-send with rejection", "NOT_SENT", func(_ *OwnerOrder, f *ownerOrderFacts) { f.rejected = true }},
		{"cancel without claim", "PREPARED", func(_ *OwnerOrder, f *ownerOrderFacts) { f.cancellationReceipt = "ACCEPTED" }},
		{"cancel without known broker order", "SUBMISSION_UNKNOWN", func(_ *OwnerOrder, f *ownerOrderFacts) { f.cancellation = true }},
		{"unknown cancel code", "BROKER_ACKNOWLEDGED", func(_ *OwnerOrder, f *ownerOrderFacts) {
			f.cancellation, f.cancellationReceipt = true, "private provider body"
		}},
		{"terminal totals disagree", "AWAITING_ACCOUNT_SETTLEMENT", func(_ *OwnerOrder, f *ownerOrderFacts) { f.terminalMatches = false }},
		{"terminal still held", "AWAITING_ACCOUNT_SETTLEMENT", func(o *OwnerOrder, _ *ownerOrderFacts) { o.AccountHeld = true }},
		{"FILLED with partial quantity", "AWAITING_ACCOUNT_SETTLEMENT", func(o *OwnerOrder, _ *ownerOrderFacts) { o.TerminalStatus = "FILLED" }},
		{"rejected terminal with fills", "AWAITING_ACCOUNT_SETTLEMENT", func(o *OwnerOrder, _ *ownerOrderFacts) { o.TerminalStatus = "REJECTED" }},
		{"settlement missing terminal", "SETTLED", func(o *OwnerOrder, _ *ownerOrderFacts) { o.TerminalStatus = "" }},
		{"settlement receipt mismatch", "SETTLED", func(_ *OwnerOrder, f *ownerOrderFacts) { f.settlementMatches = false }},
		{"settlement cash discrepancy", "SETTLED", func(o *OwnerOrder, _ *ownerOrderFacts) { o.Accounting.ClosingCashUSD = "999" }},
		{"settlement unreleased capital", "SETTLED", func(_ *OwnerOrder, f *ownerOrderFacts) { f.capitalReleased = nil }},
		{"settlement and no-send", "SETTLED", func(_ *OwnerOrder, f *ownerOrderFacts) { f.noSend = &f.now }},
		{"malformed amount", "PARTIALLY_FILLED", func(o *OwnerOrder, _ *ownerOrderFacts) { o.GrossUSD = "secret-invalid-amount" }},
		{"unverified count", "PARTIALLY_FILLED", func(o *OwnerOrder, _ *ownerOrderFacts) { o.FillCount = 0 }},
		{"invalid saved terms or digest", "PREPARED", func(_ *OwnerOrder, f *ownerOrderFacts) { f.termsValid = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, f := ownerStatusFixture(tc.state)
			tc.mutate(&o, &f)
			got := projectOwnerOrder(o, f)
			if got.State != "UNAVAILABLE" || got.Accounting != nil || got.Attempted != o.Attempted {
				t.Fatal("inconsistent records presented as complete or reusable", got)
			}
			if tc.name == "malformed amount" && got.GrossUSD != "" {
				t.Fatal("malformed amount became a plausible exact value", got.GrossUSD)
			}
			if !f.termsValid && (got.RequestDigest != "" || got.BaseSize != "" || got.ProductID != "") {
				t.Fatal("unverified public terms were returned", got)
			}
		})
	}
}

func TestOwnerOrderJSONContainsOnlySafeFixedFields(t *testing.T) {
	o, f := ownerStatusFixture("SETTLED")
	o.AccountID, o.ConnectionID, o.CapitalBucketID = "private-account", "private-connection", "private-bucket"
	o = projectOwnerOrder(o, f)
	body, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-", "CanSend", "can_send", "Attempted", "attempted", "owner_id", "client_order_id", "provider_order_id", "preview_id", "credential", "evidence", "authorization_id"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatal("private identity or authority leaked", forbidden)
		}
	}
	var decoded map[string]any
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	allowed := strings.Fields("id request_digest product_id side base_size limit_price fee_allowance_usd maximum_debit_usd created_at state summary approval_status approval_expires_at cancellation_status account_held capital_held account_blocked fill_count base_filled gross_usd fee_usd terminal_status accounting")
	if len(decoded) != len(allowed) {
		t.Fatal("JSON contract gained or lost fields", string(body))
	}
	for _, field := range allowed {
		if _, ok := decoded[field]; !ok {
			t.Error("missing safe explicit field", field)
		}
	}
	if o.AccountID == "" || o.ConnectionID == "" || o.CapitalBucketID == "" || !o.Attempted {
		t.Fatal("projection discarded private workflow scope")
	}
}
