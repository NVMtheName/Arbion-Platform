package strategy

import (
	"encoding/json"
	"sort"
	"time"
)

const (
	PaperAutonomyEvidenceGateMethod               = "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_READINESS_GATE"
	PaperAutonomyEvidenceCollecting               = "COLLECTING_EVIDENCE"
	PaperAutonomyEvidenceReviewable               = "EVIDENCE_REVIEWABLE"
	PaperAutonomyEvidenceReviewRequired           = "REVIEW_REQUIRED"
	PaperAutonomyEvidenceUnavailable              = "UNAVAILABLE"
	PaperAutonomyEvidenceMinimumDecisions         = 20
	PaperAutonomyEvidenceMinimumHours             = 168
	PaperAutonomyEvidenceReviewScope              = "OWNER_REVIEW_EVIDENCE_ONLY"
	PaperAutonomyEvidenceBoundary                 = "PAPER_SIMULATION_ONLY"
	PaperAutonomyEvidenceReviewPacketMethod       = "IMMUTABLE_PAPER_AUTONOMY_EVIDENCE_REVIEW_PACKET"
	PaperAutonomyEvidenceFreshnessSeconds   int64 = 300
)

type paperAutonomyDecisionRow struct {
	ID           string
	DecisionType string
	Rationale    json.RawMessage
	CreatedAt    time.Time
}

type paperAutonomyDecisionRationale struct {
	Decision      string `json:"decision"`
	AIProvider    string `json:"ai_provider"`
	ModelID       string `json:"model_id"`
	Profile       string `json:"profile"`
	ExecutionMode string `json:"execution_mode"`
	LatencyMS     *int   `json:"latency_ms"`
	InputUsage    *int   `json:"input_usage"`
	OutputUsage   *int   `json:"output_usage"`
	InputEvidence struct {
		Provider string `json:"provider"`
		Markets  []struct {
			Symbol     string    `json:"symbol"`
			Feed       string    `json:"feed"`
			Quality    string    `json:"quality"`
			ObservedAt time.Time `json:"observed_at"`
		} `json:"markets"`
		RecentDecisions *[]json.RawMessage `json:"recent_decisions"`
	} `json:"input_evidence"`
}

type paperAutonomySafetyCounts struct {
	LiveMandates            int
	AIOrderIntents          int
	InvalidStrategyModes    int
	InvalidExecutionModes   int
	PlatformExecutableRisks int
	NonSimulationFills      int
}

func paperAutonomyBlocker(code, category, detail string) PaperAutonomyEvidenceBlocker {
	return PaperAutonomyEvidenceBlocker{Code: code, Category: category, Detail: detail}
}

func paperAutonomyDecisionSemantics(decisionType, decision string) bool {
	switch decisionType {
	case "ABSTAIN":
		return decision == "ABSTAIN"
	case "ALLOW_SIMULATED_FILLED", "ALLOW_SIMULATED_REJECTED", "DENY_RISK_DENIED":
		return decision == "PROPOSE"
	default:
		return false
	}
}

func finalizePaperAutonomyEvidenceGate(result PaperAutonomyEvidenceGate) PaperAutonomyEvidenceGate {
	packet := &result.ReviewPacket
	packet.Status = result.Status
	packet.AsOf = result.AsOf
	packet.NoLiveSafetyStatus = result.Safety.Status
	packet.EvidenceReadyForHumanReview = result.Status == PaperAutonomyEvidenceReviewable
	packet.GrantsAuthority = false
	packet.LivePromotionAvailable = false
	switch result.Status {
	case PaperAutonomyEvidenceCollecting:
		packet.OwnerGuidance = "No owner action is required while the exact time and decision evidence window continues collecting automatically."
	case PaperAutonomyEvidenceReviewable:
		packet.OwnerGuidance = "The bounded non-live evidence packet is ready for human review; reviewability does not authorize promotion or live execution."
	case PaperAutonomyEvidenceReviewRequired:
		packet.OwnerGuidance = "Review the exact saved integrity or scheduler blocker before relying on this non-live evidence packet."
	default:
		packet.OwnerGuidance = "The exact saved evidence packet is unavailable; Arbion does not infer missing facts or readiness."
	}
	return result
}

func projectPaperAutonomyEvidenceGate(
	instanceStartedAt time.Time,
	lastScheduleStatus string,
	consecutiveScheduleFailures int,
	runs []ScheduleRun,
	decisions []paperAutonomyDecisionRow,
	portfolio PaperPortfolio,
	safetyCounts paperAutonomySafetyCounts,
) PaperAutonomyEvidenceGate {
	result := PaperAutonomyEvidenceGate{
		Status: PaperAutonomyEvidenceUnavailable, CalculationMethod: PaperAutonomyEvidenceGateMethod,
		ReviewScope: PaperAutonomyEvidenceReviewScope, ExecutionBoundary: PaperAutonomyEvidenceBoundary,
		MinimumDecisionCount: PaperAutonomyEvidenceMinimumDecisions, MinimumEvidenceWindowHours: PaperAutonomyEvidenceMinimumHours,
		LastScheduleStatus: lastScheduleStatus, ConsecutiveScheduleFailures: consecutiveScheduleFailures,
		Routes: []PaperAutonomyEvidenceRoute{}, Blockers: []PaperAutonomyEvidenceBlocker{}, LiveExecutionAvailable: false,
	}
	result.ReviewPacket = PaperAutonomyEvidenceReviewPacket{
		Status: PaperAutonomyEvidenceUnavailable, CalculationMethod: PaperAutonomyEvidenceReviewPacketMethod,
		FreshnessThresholdSeconds: PaperAutonomyEvidenceFreshnessSeconds,
		RouteContinuityStatus:     "UNAVAILABLE", InputCoverageStatus: "UNAVAILABLE", InputFreshnessStatus: "UNAVAILABLE",
		LedgerContractStatus: "UNAVAILABLE", NoLiveSafetyStatus: "CLEAR", GrantsAuthority: false, LivePromotionAvailable: false,
		ThresholdChangeLedger: projectPaperAutonomyThresholdChangeLedger(instanceStartedAt, runs, decisions),
	}
	result.Safety = PaperNoLiveSafetyEvidence{
		Status: "CLEAR", LiveMandateCount: safetyCounts.LiveMandates, AIOrderIntentCount: safetyCounts.AIOrderIntents,
		InvalidStrategyModeCount: safetyCounts.InvalidStrategyModes, InvalidExecutionModeCount: safetyCounts.InvalidExecutionModes,
		PlatformExecutableRiskCount: safetyCounts.PlatformExecutableRisks, NonSimulationFillCount: safetyCounts.NonSimulationFills,
	}
	if instanceStartedAt.IsZero() || len(runs) == 0 {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("EVIDENCE_TIMELINE_UNAVAILABLE", "UNAVAILABLE", "The immutable Paper schedule timeline is not available."))
		return finalizePaperAutonomyEvidenceGate(result)
	}
	result.ReviewPacket.EvidenceStartedAt = &instanceStartedAt
	eligibleAt := instanceStartedAt.Add(PaperAutonomyEvidenceMinimumHours * time.Hour)
	result.ReviewPacket.EvidenceEligibleAt = &eligibleAt

	asOf := runs[len(runs)-1].CompletedAt
	if asOf.IsZero() || asOf.Before(instanceStartedAt) {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("EVIDENCE_TIMELINE_INVALID", "UNAVAILABLE", "The saved Paper schedule timestamps are inconsistent."))
		return finalizePaperAutonomyEvidenceGate(result)
	}
	result.AsOf = &asOf
	result.ReviewPacket.ElapsedSeconds = int64(asOf.Sub(instanceStartedAt) / time.Second)
	if asOf.Before(eligibleAt) {
		result.ReviewPacket.RemainingSeconds = int64(eligibleAt.Sub(asOf) / time.Second)
	}
	windowStart := asOf.Add(-PaperAutonomyEvidenceMinimumHours * time.Hour)
	if instanceStartedAt.After(windowStart) {
		windowStart = instanceStartedAt
	}
	result.EvidenceWindowHours = int64(asOf.Sub(windowStart) / time.Hour)

	expectedDecisionCount := 0
	for _, run := range runs {
		if run.ScheduledFor.Before(windowStart) || run.CompletedAt.After(asOf) {
			continue
		}
		result.ReviewPacket.SchedulerSampleCount++
		switch run.Status {
		case "SUCCEEDED":
			expectedDecisionCount++
			result.ReviewPacket.SchedulerSuccessCount++
		case "FAILED":
			result.ReviewPacket.SchedulerFailureCount++
		case "SKIPPED":
			if run.ErrorCode != nil && *run.ErrorCode == "OUTSIDE_SESSION" {
				result.ReviewPacket.SchedulerSafeWaitCount++
			} else {
				result.ReviewPacket.SchedulerFailureCount++
			}
		default:
			result.ReviewPacket.SchedulerFailureCount++
		}
	}

	routeCounts := map[string]*PaperAutonomyEvidenceRoute{}
	seen := map[string]struct{}{}
	malformed := false
	for _, row := range decisions {
		if row.CreatedAt.Before(windowStart) || row.CreatedAt.After(asOf) {
			continue
		}
		if row.ID == "" || row.CreatedAt.IsZero() {
			malformed = true
			continue
		}
		if _, exists := seen[row.ID]; exists {
			malformed = true
			continue
		}
		seen[row.ID] = struct{}{}
		result.DecisionCount++
		if result.FirstDecisionAt == nil || row.CreatedAt.Before(*result.FirstDecisionAt) {
			value := row.CreatedAt
			result.FirstDecisionAt = &value
		}
		if result.LatestDecisionAt == nil || row.CreatedAt.After(*result.LatestDecisionAt) {
			value := row.CreatedAt
			result.LatestDecisionAt = &value
		}
		switch row.DecisionType {
		case "ABSTAIN":
			result.AbstentionCount++
		case "DENY_RISK_DENIED":
			result.ProposalCount++
			result.DeterministicDenyCount++
		case "ALLOW_SIMULATED_FILLED":
			result.ProposalCount++
			result.SimulatedFillCount++
		case "ALLOW_SIMULATED_REJECTED":
			result.ProposalCount++
		default:
			malformed = true
		}

		var rationale paperAutonomyDecisionRationale
		if len(row.Rationale) == 0 || json.Unmarshal(row.Rationale, &rationale) != nil || !paperAutonomyDecisionSemantics(row.DecisionType, rationale.Decision) || rationale.ExecutionMode != string(Paper) {
			malformed = true
			continue
		}
		marketsAttributed := len(rationale.InputEvidence.Markets) > 0
		marketsFresh := marketsAttributed
		for _, market := range rationale.InputEvidence.Markets {
			if market.Symbol == "" || market.Feed == "" || market.Quality == "" || market.ObservedAt.IsZero() || market.ObservedAt.After(row.CreatedAt) {
				marketsAttributed = false
				marketsFresh = false
				break
			}
			result.ReviewPacket.MarketObservationCount++
			if result.ReviewPacket.FirstMarketObservedAt == nil || market.ObservedAt.Before(*result.ReviewPacket.FirstMarketObservedAt) {
				value := market.ObservedAt
				result.ReviewPacket.FirstMarketObservedAt = &value
			}
			if result.ReviewPacket.LatestMarketObservedAt == nil || market.ObservedAt.After(*result.ReviewPacket.LatestMarketObservedAt) {
				value := market.ObservedAt
				result.ReviewPacket.LatestMarketObservedAt = &value
			}
			ageSeconds := int64(row.CreatedAt.Sub(market.ObservedAt) / time.Second)
			if ageSeconds > result.ReviewPacket.MaximumMarketAgeSeconds {
				result.ReviewPacket.MaximumMarketAgeSeconds = ageSeconds
			}
			if ageSeconds > PaperAutonomyEvidenceFreshnessSeconds {
				marketsFresh = false
			}
		}
		if rationale.AIProvider != "" && rationale.ModelID != "" && rationale.Profile != "" && rationale.InputEvidence.Provider != "" && marketsAttributed {
			result.AttributedDecisionCount++
			key := rationale.AIProvider + "\x00" + rationale.ModelID + "\x00" + rationale.Profile + "\x00" + rationale.InputEvidence.Provider
			route := routeCounts[key]
			if route == nil {
				route = &PaperAutonomyEvidenceRoute{AIProvider: rationale.AIProvider, ModelID: rationale.ModelID, Profile: rationale.Profile, FinancialProvider: rationale.InputEvidence.Provider}
				routeCounts[key] = route
			}
			route.DecisionCount++
		}
		if rationale.LatencyMS != nil && rationale.InputUsage != nil && rationale.OutputUsage != nil && *rationale.LatencyMS >= 0 && *rationale.InputUsage >= 0 && *rationale.OutputUsage >= 0 {
			result.TelemetryCompleteCount++
		}
		if marketsFresh {
			result.ReviewPacket.FreshMarketDecisionCount++
		}
		if rationale.InputEvidence.RecentDecisions != nil && len(*rationale.InputEvidence.RecentDecisions) <= 6 {
			result.BoundedMemoryCount++
		}
	}
	for _, route := range routeCounts {
		result.Routes = append(result.Routes, *route)
	}
	sort.Slice(result.Routes, func(i, j int) bool {
		left, right := result.Routes[i], result.Routes[j]
		if left.AIProvider != right.AIProvider {
			return left.AIProvider < right.AIProvider
		}
		if left.ModelID != right.ModelID {
			return left.ModelID < right.ModelID
		}
		if left.Profile != right.Profile {
			return left.Profile < right.Profile
		}
		return left.FinancialProvider < right.FinancialProvider
	})
	if result.DecisionCount > 0 && result.AttributedDecisionCount == result.DecisionCount {
		result.ReviewPacket.InputCoverageStatus = "COMPLETE"
		if len(result.Routes) == 1 {
			result.ReviewPacket.RouteContinuityStatus = "STABLE"
		} else {
			result.ReviewPacket.RouteContinuityStatus = "CONTEXT_CHANGED"
		}
	} else if result.DecisionCount > 0 {
		result.ReviewPacket.InputCoverageStatus = "REVIEW_REQUIRED"
		result.ReviewPacket.RouteContinuityStatus = "REVIEW_REQUIRED"
	}
	if result.DecisionCount > 0 && result.ReviewPacket.FreshMarketDecisionCount == result.DecisionCount {
		result.ReviewPacket.InputFreshnessStatus = "CURRENT_AT_DECISION"
	} else if result.DecisionCount > 0 {
		result.ReviewPacket.InputFreshnessStatus = "REVIEW_REQUIRED"
	}

	result.LedgerContractsReconciled = portfolio.RealizedOutcome.Status != PaperRealizedUnavailable &&
		portfolio.ExecutionCosts.Status != PaperExecutionCostsUnavailable &&
		portfolio.ActivityCadence.Status == PaperActivityCadenceAvailable &&
		portfolio.GuardrailEvidence.Status == PaperActivityCadenceAvailable &&
		portfolio.RealizedOutcome.FillCount == portfolio.ExecutionCosts.FillCount
	if result.LedgerContractsReconciled {
		result.ReviewPacket.LedgerContractStatus = "RECONCILED"
	} else {
		result.ReviewPacket.LedgerContractStatus = "REVIEW_REQUIRED"
	}

	if malformed || result.DecisionCount != expectedDecisionCount || result.DecisionCount != result.AbstentionCount+result.ProposalCount {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("DECISION_EVIDENCE_INCONSISTENT", "UNAVAILABLE", "Saved Paper decisions do not reconcile exactly with the successful scheduler history."))
	}
	if result.AttributedDecisionCount != result.DecisionCount {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("DECISION_PROVENANCE_INCOMPLETE", "REVIEW", "One or more saved decisions lack exact AI-route, financial-provider, or market provenance."))
	}
	if result.ReviewPacket.FreshMarketDecisionCount != result.DecisionCount {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("MARKET_INPUT_FRESHNESS_REVIEW_REQUIRED", "REVIEW", "One or more saved decisions lack provider-market evidence observed within five minutes of that decision."))
	}
	if result.TelemetryCompleteCount != result.DecisionCount {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("DECISION_TELEMETRY_INCOMPLETE", "REVIEW", "One or more saved decisions lack nonnegative latency or usage telemetry."))
	}
	if result.BoundedMemoryCount != result.DecisionCount {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("DECISION_MEMORY_UNBOUNDED", "REVIEW", "One or more saved decisions exceed the six-decision memory boundary."))
	}
	if lastScheduleStatus != "SUCCEEDED" || consecutiveScheduleFailures != 0 {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("SCHEDULER_NOT_HEALTHY", "REVIEW", "The newest automatic Paper cycle is not a zero-failure success."))
	}
	if !result.LedgerContractsReconciled {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("LEDGER_CONTRACT_REVIEW_REQUIRED", "REVIEW", "The isolated Paper ledger or one of its immutable evidence contracts is unavailable or inconsistent."))
	}
	if safetyCounts.LiveMandates != 0 || safetyCounts.AIOrderIntents != 0 || safetyCounts.InvalidStrategyModes != 0 || safetyCounts.InvalidExecutionModes != 0 || safetyCounts.PlatformExecutableRisks != 0 || safetyCounts.NonSimulationFills != 0 {
		result.Safety.Status = "REVIEW_REQUIRED"
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("NO_LIVE_INVARIANT_BREACH", "REVIEW", "An owner-scoped no-live safety counter is nonzero."))
	}
	if result.EvidenceWindowHours < PaperAutonomyEvidenceMinimumHours {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("EVIDENCE_WINDOW_INCOMPLETE", "COLLECTION", "Seven days of automatic Paper evidence have not elapsed yet."))
	}
	if result.DecisionCount < PaperAutonomyEvidenceMinimumDecisions {
		result.Blockers = append(result.Blockers, paperAutonomyBlocker("DECISION_SAMPLE_INCOMPLETE", "COLLECTION", "Fewer than 20 automatic Paper decisions are saved in the evidence window."))
	}

	result.Status = PaperAutonomyEvidenceReviewable
	for _, blocker := range result.Blockers {
		if blocker.Category == "UNAVAILABLE" {
			result.Status = PaperAutonomyEvidenceUnavailable
			return finalizePaperAutonomyEvidenceGate(result)
		}
	}
	for _, blocker := range result.Blockers {
		if blocker.Category == "REVIEW" {
			result.Status = PaperAutonomyEvidenceReviewRequired
			return finalizePaperAutonomyEvidenceGate(result)
		}
	}
	if len(result.Blockers) > 0 {
		result.Status = PaperAutonomyEvidenceCollecting
	}
	return finalizePaperAutonomyEvidenceGate(result)
}
