package execution

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/arbion/platform/services/api/internal/risk"
)

func spotPolicyFixture() (PilotAllocation, mandatePolicySnapshot, time.Time) {
	r := requestFixture()
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	p := PilotAllocation{OwnerID: r.OwnerID, AccountID: r.AccountID, ConnectionID: r.ConnectionID, CapitalBucketID: r.CapitalBucketID,
		ProductID: r.ProductID, InitialCashUSD: "100", Limits: OwnerPilotLimits{MaximumOrderUSD: "25", ExpiresAt: now.Add(time.Hour)}}
	model, ai, trades := "gpt-5.6-sol", "77777777-7777-4777-8777-777777777777", 4
	s := mandatePolicySnapshot{FinancialAccountID: p.AccountID, CapitalBucketID: p.CapitalBucketID, AutomationType: "AI_AUTONOMOUS",
		AutonomyLevel: "FULL_AUTONOMOUS", ExecutionMode: "LIVE", Status: "READY", AIProviderConnectionID: &ai, AIModelID: &model,
		StrategyParameters: json.RawMessage(`{"profile":"COINBASE_SPOT_PILOT_V1","objective":"Evaluate the approved spot pilot","max_proposal_notional":"25"}`),
		Risk:               automation.RiskPolicy{MaxTradesPerDay: &trades}, AllowedUniverse: automation.Universe{Symbols: []string{"BTC"}},
		ScheduleConditions: json.RawMessage(`{"enabled":false}`), EffectiveFrom: now.Add(-time.Minute)}
	return p, s, now
}

func TestSpotPilotProfileCannotPromoteShadowOrExpandScope(t *testing.T) {
	p, original, now := spotPolicyFixture()
	str := func(s string) *string { return &s }
	for name, mutate := range map[string]func(*mandatePolicySnapshot){
		"paper":                     func(s *mandatePolicySnapshot) { s.ExecutionMode = "PAPER" },
		"shadow":                    func(s *mandatePolicySnapshot) { s.ExecutionMode = "SHADOW" },
		"paused":                    func(s *mandatePolicySnapshot) { s.Status = "PAUSED" },
		"manual":                    func(s *mandatePolicySnapshot) { s.AutonomyLevel = "CONFIRM_EACH" },
		"hybrid":                    func(s *mandatePolicySnapshot) { s.AutomationType = "HYBRID" },
		"options strategy":          func(s *mandatePolicySnapshot) { s.StrategyIdentifier = str("wheel") },
		"margin":                    func(s *mandatePolicySnapshot) { s.MarginAllowed = true },
		"options":                   func(s *mandatePolicySnapshot) { s.OptionsAllowed = true },
		"unknown capability":        func(s *mandatePolicySnapshot) { s.CapabilityUnverified = true },
		"paper options attestation": func(s *mandatePolicySnapshot) { s.PaperOptionsSimulationAttested = true },
		"execution flag promotion":  func(s *mandatePolicySnapshot) { s.ExecutionCapable = true },
		"wrong account":             func(s *mandatePolicySnapshot) { s.FinancialAccountID = p.OwnerID },
		"wrong bucket":              func(s *mandatePolicySnapshot) { s.CapitalBucketID = p.OwnerID },
		"missing AI":                func(s *mandatePolicySnapshot) { s.AIProviderConnectionID = nil },
		"model override":            func(s *mandatePolicySnapshot) { s.AIModelID = str("https://untrusted.example/") },
		"multiple pairs":            func(s *mandatePolicySnapshot) { s.AllowedUniverse.Symbols = []string{"BTC", "ETH"} },
		"wrong pair":                func(s *mandatePolicySnapshot) { s.AllowedUniverse.Symbols = []string{"ETH"} },
		"universe bypass":           func(s *mandatePolicySnapshot) { s.AllowedUniverse.UniverseIDs = []string{"all"} },
		"prohibited universe":       func(s *mandatePolicySnapshot) { s.ProhibitedUniverse.UniverseIDs = []string{"all"} },
		"prohibited target":         func(s *mandatePolicySnapshot) { s.ProhibitedUniverse.Symbols = []string{"btc"} },
		"not yet effective":         func(s *mandatePolicySnapshot) { s.EffectiveFrom = now.Add(time.Second) },
		"expired":                   func(s *mandatePolicySnapshot) { s.EffectiveUntil = &now },
		"unknown profile": func(s *mandatePolicySnapshot) {
			s.StrategyParameters = json.RawMessage(`{"profile":"shadow","objective":"test","max_proposal_notional":"25"}`)
		},
		"unrecognized parameter": func(s *mandatePolicySnapshot) {
			s.StrategyParameters = json.RawMessage(`{"profile":"COINBASE_SPOT_PILOT_V1","objective":"test","max_proposal_notional":"25","ignore_stops":true}`)
		},
		"oversized proposal": func(s *mandatePolicySnapshot) {
			s.StrategyParameters = json.RawMessage(`{"profile":"COINBASE_SPOT_PILOT_V1","objective":"test","max_proposal_notional":"26"}`)
		},
		"no daily count":          func(s *mandatePolicySnapshot) { s.Risk.MaxTradesPerDay = nil },
		"zero daily count":        func(s *mandatePolicySnapshot) { x := 0; s.Risk.MaxTradesPerDay = &x },
		"oversized daily count":   func(s *mandatePolicySnapshot) { x := 49; s.Risk.MaxTradesPerDay = &x },
		"unavailable realized PL": func(s *mandatePolicySnapshot) { s.Risk.MaxDailyLoss = str("5") },
		"invalid decimal":         func(s *mandatePolicySnapshot) { s.Risk.MaxCapitalDeployed = str("NaN") },
		"expanded deployment":     func(s *mandatePolicySnapshot) { s.Risk.MaxCapitalDeployed = str("101") },
		"invalid position":        func(s *mandatePolicySnapshot) { s.Risk.MaxSinglePositionAmount = str("0") },
		"invalid concentration":   func(s *mandatePolicySnapshot) { s.Risk.MaxSinglePositionPercentage = str("101") },
		"negative reserve":        func(s *mandatePolicySnapshot) { s.Risk.MinimumCashReserve = str("-1") },
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(original)
			var s mandatePolicySnapshot
			_ = json.Unmarshal(body, &s)
			mutate(&s)
			body, _ = json.Marshal(s)
			if _, err := parseSpotPilotMandate(body, p, p.OwnerID, 1, now); err == nil {
				t.Fatal("unsafe profile accepted")
			}
		})
	}
	body, _ := json.Marshal(original)
	if _, err := parseSpotPilotMandate(body, p, p.OwnerID, 1, now); err != nil {
		t.Fatal("valid explicit profile rejected", err)
	}
	var unknown map[string]any
	_ = json.Unmarshal(body, &unknown)
	unknown["risk_parameters"].(map[string]any)["unimplemented_policy"] = "1"
	body, _ = json.Marshal(unknown)
	if _, err := parseSpotPilotMandate(body, p, p.OwnerID, 1, now); err == nil {
		t.Fatal("unsupported risk field ignored")
	}
}

func TestAutonomousRiskUsesOnlyPilotFundsAndActualInventoryExposure(t *testing.T) {
	p, s, now := spotPolicyFixture()
	body, _ := json.Marshal(s)
	m, err := parseSpotPilotMandate(body, p, p.OwnerID, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	r := requestFixture()
	r.BaseSize, r.LimitPrice, r.FeeAllowanceUSD, r.MaximumDebitUSD = "0.0002", "60000", "0.12", "12.12"
	o := Order{ID: r.ClientOrderID, Request: r, CreatedAt: now.Add(-time.Second)}
	proof := VerifiedPreflight{ObservedAt: now, AvailableCashUSD: "1000", AvailableBase: "5"}
	bucket := risk.CapitalBucket{ID: p.CapitalBucketID, UserID: p.OwnerID, AccountID: p.AccountID, AllocationType: "FIXED_AMOUNT", AllocationValue: "1000", ProtectedAmount: "0", Currency: "USD", Status: "ACTIVE"}
	eval := func(o Order, m risk.Mandate, cash, base, ask string, count int) risk.RiskEvaluation {
		t.Helper()
		x, e := evaluateSpotPilotFunding(o, proof, bucket, m, cash, base, "100", "25", ask, count, now)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	x := eval(o, m, "100", "0", "60000", 0)
	if x.Decision != risk.Allow || x.ApprovalRequired || x.PlatformExecutionAvailable || x.Mode != "LIVE" || x.MandateID == nil || *x.MandateID != m.ID || x.MandateVersion == nil || *x.MandateVersion != 1 {
		t.Fatalf("wrong autonomous evidence: %#v", x)
	}
	if eval(o, m, "12.119999999999999999", "0", "60000", 0).Decision != risk.Deny {
		t.Fatal("borrowed brokerage cash beyond attributed pilot cash")
	}
	if eval(o, m, "100", "0", "60000", 4).Decision != risk.Deny {
		t.Fatal("daily attempt ceiling ignored")
	}
	limited := m
	reserve := "90"
	limited.MinimumCashReserve = &reserve
	if eval(o, limited, "100", "0", "60000", 0).Decision != risk.Deny {
		t.Fatal("cash reserve ignored")
	}
	cap := "20"
	limited = m
	limited.MaxSinglePositionAmount = &cap
	if eval(o, limited, "80", "0.0002", "60000", 0).Decision != risk.Deny {
		t.Fatal("existing attributed exposure treated as zero")
	}
	// A very low SELL price must not understate remaining inventory exposure.
	sell := o
	sell.Request.Side = "SELL"
	sell.Request.MaximumDebitUSD = "0"
	sell.Request.LimitPrice = "1"
	if eval(sell, m, "100", "0", "60000", 0).Decision != risk.Deny {
		t.Fatal("sold pre-existing brokerage holdings")
	}
	if eval(sell, m, "80", "0.0002", "60000", 0).Decision != risk.Allow {
		t.Fatal("attributed acquired inventory unavailable")
	}
	cap = "5"
	limited.MaxSinglePositionAmount = &cap
	if eval(sell, limited, "80", "0.0004", "60000", 0).Decision != risk.Deny {
		t.Fatal("low owner limit erased market exposure")
	}
	oversized := o
	oversized.Request.MaximumDebitUSD = "25.000000000000000001"
	if _, err := evaluateSpotPilotFunding(oversized, proof, bucket, m, "100", "0", "100", "25", "60000", 0, now); err == nil {
		t.Fatal("all-in proposal cap rounded")
	}
}
