package strategy

import (
	"encoding/json"
	"math/big"
	"sort"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

const PaperGuardrailEvidenceMethod = "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION"

const (
	paperGuardrailCoverageComplete         = "COMPLETE"
	paperGuardrailCoverageDriftDetected    = "DRIFT_DETECTED"
	paperGuardrailCoverageFullEvaluation   = "FULL_EVALUATION"
	paperGuardrailCoverageFailClosedPrefix = "FAIL_CLOSED_PREFIX"
)

type paperGuardrailProposalRow struct {
	DecisionJournalEntryID     string
	CreatedAt                  time.Time
	DecisionType               string
	ProposedActionID           string
	RiskEvaluationID           string
	ExecutionRecordID          string
	Rationale                  json.RawMessage
	RiskDecision               string
	ApprovalRequired           bool
	RiskExecutionMode          string
	PlatformExecutionAvailable bool
	ReasonCodes                json.RawMessage
	Checks                     json.RawMessage
	ExecutionMode              string
	ExecutionStatus            string
	Symbol                     string
	Instrument                 string
	Side                       string
	ExecutionNotional          string
}

type paperGuardrailRationale struct {
	Decision         string `json:"decision"`
	Symbol           string `json:"symbol"`
	Side             string `json:"side"`
	ProposedNotional string `json:"proposed_notional"`
	InputEvidence    struct {
		Provider string `json:"provider"`
		Markets  []struct {
			Symbol     string    `json:"symbol"`
			Feed       string    `json:"feed"`
			Quality    string    `json:"quality"`
			ObservedAt time.Time `json:"observed_at"`
		} `json:"markets"`
	} `json:"input_evidence"`
}

type paperGuardrailCheck struct {
	Code    string `json:"code"`
	Result  string `json:"result"`
	Message string `json:"message"`
}

func unavailablePaperGuardrailWindow(window PaperDispositionFunnelWindow) PaperGuardrailEvidenceWindow {
	return PaperGuardrailEvidenceWindow{
		Status: PaperActivityCadenceUnavailable, CoverageStatus: PaperActivityCadenceUnavailable, HorizonHours: window.HorizonHours,
		WindowStartedAt: window.WindowStartedAt, WindowEndedAt: window.WindowEndedAt,
		DenialReasonCodes: []PaperGuardrailCodeCount{}, FailedCheckCodes: []PaperGuardrailCodeCount{},
		ExpectedCheckCodes: []string{}, CheckResults: []PaperGuardrailCheckCount{},
		Symbols: []PaperGuardrailSymbol{}, Proposals: []PaperGuardrailProposalFact{},
	}
}

func paperGuardrailDecimal(value string) (*big.Rat, bool) {
	parsed, ok := new(big.Rat).SetString(value)
	return parsed, ok && parsed.Sign() > 0
}

func paperGuardrailMedian(values []*big.Rat) string {
	sort.Slice(values, func(i, j int) bool { return values[i].Cmp(values[j]) < 0 })
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return paperDecimal(values[middle])
	}
	return paperDecimal(new(big.Rat).Quo(new(big.Rat).Add(values[middle-1], values[middle]), big.NewRat(2, 1)))
}

func sortedPaperGuardrailCounts(values map[string]int) []PaperGuardrailCodeCount {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]PaperGuardrailCodeCount, 0, len(keys))
	for _, key := range keys {
		result = append(result, PaperGuardrailCodeCount{Code: key, Count: values[key]})
	}
	return result
}

func rememberPaperGuardrailIdentity(seen map[string]struct{}, value string) bool {
	if value == "" {
		return false
	}
	if _, exists := seen[value]; exists {
		return false
	}
	seen[value] = struct{}{}
	return true
}

func paperGuardrailExpectedCheckCodes() []string {
	plan := risk.AIAutonomousPaperCheckPlan()
	result := make([]string, len(plan))
	for index, stage := range plan {
		result[index] = string(stage.CanonicalCode)
	}
	return result
}

func paperGuardrailStageAccepts(stage risk.DeterministicCheckStage, code string) bool {
	for _, accepted := range stage.AcceptedCodes {
		if string(accepted) == code {
			return true
		}
	}
	return false
}

func paperGuardrailCheckCoverage(checks []paperGuardrailCheck, decision string) (string, string) {
	plan := risk.AIAutonomousPaperCheckPlan()
	if len(checks) == 0 || len(checks) > len(plan) {
		return paperGuardrailCoverageDriftDetected, ""
	}
	for index, check := range checks {
		if !paperGuardrailStageAccepts(plan[index], check.Code) {
			return paperGuardrailCoverageDriftDetected, ""
		}
		if index < len(checks)-1 && check.Result == "FAIL" {
			return paperGuardrailCoverageDriftDetected, ""
		}
	}
	last := checks[len(checks)-1]
	if decision == "ALLOW" {
		if len(checks) != len(plan) || last.Result == "FAIL" {
			return paperGuardrailCoverageDriftDetected, ""
		}
		return paperGuardrailCoverageFullEvaluation, "ALL_REQUIRED_CHECKS"
	}
	if decision == "DENY" && last.Result == "FAIL" {
		return paperGuardrailCoverageFailClosedPrefix, string(plan[len(checks)-1].CanonicalCode)
	}
	return paperGuardrailCoverageDriftDetected, ""
}

func sortedPaperGuardrailCheckCounts(values map[string]PaperGuardrailCheckCount) []PaperGuardrailCheckCount {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]PaperGuardrailCheckCount, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func paperGuardrailProposalFact(row paperGuardrailProposalRow, fills map[string]paperRealizedFill) (PaperGuardrailProposalFact, *big.Rat, bool) {
	if row.DecisionJournalEntryID == "" || row.ProposedActionID == "" || row.RiskEvaluationID == "" || row.ExecutionRecordID == "" || row.CreatedAt.IsZero() ||
		row.ApprovalRequired || row.RiskExecutionMode != "PAPER" || row.PlatformExecutionAvailable || row.ExecutionMode != "PAPER" ||
		(row.Instrument != "EQUITY" && row.Instrument != "CRYPTO") || (row.Side != "BUY" && row.Side != "SELL") {
		return PaperGuardrailProposalFact{}, nil, false
	}
	var rationale paperGuardrailRationale
	var reasonCodes []string
	var checks []paperGuardrailCheck
	if json.Unmarshal(row.Rationale, &rationale) != nil || json.Unmarshal(row.ReasonCodes, &reasonCodes) != nil || json.Unmarshal(row.Checks, &checks) != nil ||
		rationale.Decision != "PROPOSE" || rationale.Symbol != row.Symbol || rationale.Side != row.Side || len(reasonCodes) == 0 || len(checks) == 0 || rationale.InputEvidence.Provider == "" {
		return PaperGuardrailProposalFact{}, nil, false
	}
	notional, ok := paperGuardrailDecimal(rationale.ProposedNotional)
	if !ok {
		return PaperGuardrailProposalFact{}, nil, false
	}
	var marketFeed, marketQuality string
	var marketObservedAt time.Time
	marketMatches := 0
	for _, market := range rationale.InputEvidence.Markets {
		if market.Symbol == row.Symbol {
			marketMatches++
			marketFeed, marketQuality, marketObservedAt = market.Feed, market.Quality, market.ObservedAt
		}
	}
	if marketMatches != 1 || marketFeed == "" || marketQuality == "" || marketObservedAt.IsZero() || marketObservedAt.After(row.CreatedAt) {
		return PaperGuardrailProposalFact{}, nil, false
	}
	failed := []string{}
	checkCodes := map[string]struct{}{}
	for _, check := range checks {
		if check.Code == "" || check.Message == "" || (check.Result != "PASS" && check.Result != "FAIL" && check.Result != "WARN") {
			return PaperGuardrailProposalFact{}, nil, false
		}
		if _, exists := checkCodes[check.Code]; exists {
			return PaperGuardrailProposalFact{}, nil, false
		}
		checkCodes[check.Code] = struct{}{}
		if check.Result == "FAIL" {
			failed = append(failed, check.Code)
		}
	}
	sort.Strings(failed)
	reasons := append([]string(nil), reasonCodes...)
	sort.Strings(reasons)
	for index := 1; index < len(reasons); index++ {
		if reasons[index] == reasons[index-1] {
			return PaperGuardrailProposalFact{}, nil, false
		}
	}
	fact := PaperGuardrailProposalFact{
		DecisionJournalEntryID: row.DecisionJournalEntryID, ProposedActionID: row.ProposedActionID,
		RiskEvaluationID: row.RiskEvaluationID, ExecutionRecordID: row.ExecutionRecordID,
		CreatedAt: row.CreatedAt, Symbol: row.Symbol, Instrument: row.Instrument, Side: row.Side,
		ProposedNotional: paperDecimal(notional), RiskDecision: row.RiskDecision, ExecutionStatus: row.ExecutionStatus,
		FinancialProvider: rationale.InputEvidence.Provider, MarketFeed: marketFeed, MarketQuality: marketQuality, MarketObservedAt: marketObservedAt,
	}
	for _, check := range checks {
		fact.Checks = append(fact.Checks, PaperGuardrailCheckFact{Code: check.Code, Result: check.Result})
	}
	fact.CoverageStatus, fact.TerminalCheckStage = paperGuardrailCheckCoverage(checks, row.RiskDecision)
	switch {
	case row.DecisionType == "ALLOW_SIMULATED_FILLED" && row.RiskDecision == "ALLOW" && row.ExecutionStatus == "SIMULATED_FILLED":
		if len(failed) != 0 || len(reasons) != 1 || reasons[0] != "ALLOWED" {
			return PaperGuardrailProposalFact{}, nil, false
		}
		fill, exists := fills[row.ExecutionRecordID]
		if !exists || fill.ProposedActionID != row.ProposedActionID || fill.RiskEvaluationID != row.RiskEvaluationID || fill.Symbol != row.Symbol || fill.Instrument != row.Instrument || fill.Side != row.Side ||
			!fill.SimulatedAt.Equal(row.CreatedAt) || !sameAIPaperDecimal(fill.RequestedNotional, rationale.ProposedNotional) ||
			fill.MarketProvider != fact.FinancialProvider || fill.MarketFeed != fact.MarketFeed || fill.MarketQuality != fact.MarketQuality || !fill.MarketObservedAt.Equal(fact.MarketObservedAt) || !fill.SimulationOnly {
			return PaperGuardrailProposalFact{}, nil, false
		}
		fact.DenialReasonCodes, fact.FailedCheckCodes = []string{}, []string{}
	case row.DecisionType == "DENY_RISK_DENIED" && row.RiskDecision == "DENY" && row.ExecutionStatus == "RISK_DENIED":
		if len(failed) == 0 || len(reasons) != len(failed) || !sameAIPaperDecimal(row.ExecutionNotional, rationale.ProposedNotional) {
			return PaperGuardrailProposalFact{}, nil, false
		}
		if _, exists := fills[row.ExecutionRecordID]; exists {
			return PaperGuardrailProposalFact{}, nil, false
		}
		for index := range reasons {
			if reasons[index] != failed[index] {
				return PaperGuardrailProposalFact{}, nil, false
			}
		}
		fact.DenialReasonCodes, fact.FailedCheckCodes = reasons, failed
	default:
		return PaperGuardrailProposalFact{}, nil, false
	}
	return fact, notional, true
}

func paperGuardrailWindow(window PaperDispositionFunnelWindow, rows []paperGuardrailProposalRow, fills []paperRealizedFill) PaperGuardrailEvidenceWindow {
	result := unavailablePaperGuardrailWindow(window)
	if window.Status != PaperActivityCadenceAvailable || window.WindowStartedAt == nil || window.WindowEndedAt == nil || window.OtherProposalOutcomeCount != 0 {
		return result
	}
	fillMap := map[string]paperRealizedFill{}
	for _, fill := range fills {
		if fill.ExecutionRecordID == "" {
			return result
		}
		if _, exists := fillMap[fill.ExecutionRecordID]; exists {
			return result
		}
		fillMap[fill.ExecutionRecordID] = fill
	}
	result.Status = PaperActivityCadenceAvailable
	reasonCounts, failedCheckCounts := map[string]int{}, map[string]int{}
	checkCounts := map[string]PaperGuardrailCheckCount{}
	result.CoverageStatus = paperGuardrailCoverageComplete
	result.ExpectedCheckCodes = paperGuardrailExpectedCheckCodes()
	type symbolAggregate struct {
		value    PaperGuardrailSymbol
		notional *big.Rat
	}
	symbols := map[string]*symbolAggregate{}
	notionals := []*big.Rat{}
	seenDecision, seenAction, seenRisk, seenExecution := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, row := range rows {
		if row.CreatedAt.Before(*window.WindowStartedAt) || row.CreatedAt.After(*window.WindowEndedAt) {
			continue
		}
		fact, notional, ok := paperGuardrailProposalFact(row, fillMap)
		if !ok {
			return unavailablePaperGuardrailWindow(window)
		}
		if !rememberPaperGuardrailIdentity(seenDecision, fact.DecisionJournalEntryID) ||
			!rememberPaperGuardrailIdentity(seenAction, fact.ProposedActionID) ||
			!rememberPaperGuardrailIdentity(seenRisk, fact.RiskEvaluationID) ||
			!rememberPaperGuardrailIdentity(seenExecution, fact.ExecutionRecordID) {
			return unavailablePaperGuardrailWindow(window)
		}
		result.ProposalCount++
		for _, check := range fact.Checks {
			counts := checkCounts[check.Code]
			counts.Code = check.Code
			counts.EvaluationCount++
			switch check.Result {
			case "PASS":
				counts.PassCount++
			case "FAIL":
				counts.FailCount++
			case "WARN":
				counts.WarnCount++
			}
			checkCounts[check.Code] = counts
		}
		switch fact.CoverageStatus {
		case paperGuardrailCoverageFullEvaluation:
			result.FullyEvaluatedCount++
		case paperGuardrailCoverageFailClosedPrefix:
			result.FailClosedPrefixCount++
		default:
			result.CheckSetDriftCount++
			result.CoverageStatus = paperGuardrailCoverageDriftDetected
		}
		notionals = append(notionals, new(big.Rat).Set(notional))
		key := fact.Instrument + ":" + fact.Symbol
		aggregate := symbols[key]
		if aggregate == nil {
			aggregate = &symbolAggregate{value: PaperGuardrailSymbol{Symbol: fact.Symbol, Instrument: fact.Instrument}, notional: new(big.Rat)}
			symbols[key] = aggregate
		}
		aggregate.value.ProposalCount++
		aggregate.notional.Add(aggregate.notional, notional)
		if fact.RiskDecision == "ALLOW" {
			result.AllowCount++
			result.SimulatedFillCount++
			aggregate.value.AllowCount++
			aggregate.value.SimulatedFillCount++
		} else {
			result.DenyCount++
			aggregate.value.DenyCount++
			for _, code := range fact.DenialReasonCodes {
				reasonCounts[code]++
			}
			for _, code := range fact.FailedCheckCodes {
				failedCheckCounts[code]++
			}
		}
		result.Proposals = append(result.Proposals, fact)
	}
	if result.ProposalCount != window.ProposalCount || result.DenyCount != window.DeterministicDenyCount || result.SimulatedFillCount != window.SimulatedFillCount || result.AllowCount != result.SimulatedFillCount || result.ProposalCount != result.AllowCount+result.DenyCount {
		return unavailablePaperGuardrailWindow(window)
	}
	if result.FullyEvaluatedCount+result.FailClosedPrefixCount+result.CheckSetDriftCount != result.ProposalCount {
		return unavailablePaperGuardrailWindow(window)
	}
	if len(notionals) > 0 {
		sort.Slice(notionals, func(i, j int) bool { return notionals[i].Cmp(notionals[j]) < 0 })
		result.MinimumProposedNotional = paperDecimal(notionals[0])
		result.MedianProposedNotional = paperGuardrailMedian(notionals)
		result.MaximumProposedNotional = paperDecimal(notionals[len(notionals)-1])
	}
	result.DenialReasonCodes = sortedPaperGuardrailCounts(reasonCounts)
	result.FailedCheckCodes = sortedPaperGuardrailCounts(failedCheckCounts)
	result.CheckResults = sortedPaperGuardrailCheckCounts(checkCounts)
	keys := make([]string, 0, len(symbols))
	for key := range symbols {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		aggregate := symbols[key]
		aggregate.value.ProposedNotional = paperDecimal(aggregate.notional)
		result.Symbols = append(result.Symbols, aggregate.value)
	}
	sort.Slice(result.Proposals, func(i, j int) bool {
		if result.Proposals[i].CreatedAt.Equal(result.Proposals[j].CreatedAt) {
			return result.Proposals[i].DecisionJournalEntryID < result.Proposals[j].DecisionJournalEntryID
		}
		return result.Proposals[i].CreatedAt.Before(result.Proposals[j].CreatedAt)
	})
	return result
}

func projectPaperGuardrailEvidence(cadence PaperActivityCadence, rows []paperGuardrailProposalRow, fills []paperRealizedFill) PaperGuardrailEvidence {
	twentyFour := paperGuardrailWindow(cadence.DispositionFunnel.TwentyFourHours, rows, fills)
	sevenDays := paperGuardrailWindow(cadence.DispositionFunnel.SevenDays, rows, fills)
	result := PaperGuardrailEvidence{Status: PaperActivityCadenceUnavailable, TwentyFourHours: twentyFour, SevenDays: sevenDays}
	if cadence.AsOf != nil {
		value := *cadence.AsOf
		result.AsOf = &value
	}
	if twentyFour.Status == PaperActivityCadenceAvailable {
		result.Status, result.CalculationMethod = PaperActivityCadenceAvailable, PaperGuardrailEvidenceMethod
	}
	return result
}
