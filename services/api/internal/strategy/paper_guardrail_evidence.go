package strategy

import (
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
)

const PaperGuardrailEvidenceMethod = "IMMUTABLE_PAPER_PROPOSAL_RISK_AND_SIMULATION_ATTRIBUTION"

const PaperGuardrailCoverageChangeMethod = "IMMUTABLE_24_HOUR_AND_SEVEN_DAY_GUARDRAIL_COVERAGE_COMPARISON"

const PaperDenialEligibilityMethod = "IMMUTABLE_PAPER_DETERMINISTIC_DENIAL_AND_LATER_ELIGIBILITY"

const (
	paperGuardrailCoverageComplete         = "COMPLETE"
	paperGuardrailCoverageDriftDetected    = "DRIFT_DETECTED"
	paperGuardrailCoverageFullEvaluation   = "FULL_EVALUATION"
	paperGuardrailCoverageFailClosedPrefix = "FAIL_CLOSED_PREFIX"
	paperGuardrailShareIncreased           = "INCREASED"
	paperGuardrailShareDecreased           = "DECREASED"
	paperGuardrailShareUnchanged           = "UNCHANGED"
	paperDenialLaterAllowed                = "LATER_ALLOWED"
	paperDenialLaterDenied                 = "LATER_DENIED"
	paperDenialNoLaterComparable           = "NO_LATER_COMPARABLE_PROPOSAL"
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
	ExecutionQuantity          string
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

func unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays PaperGuardrailEvidenceWindow) PaperGuardrailCoverageChange {
	return PaperGuardrailCoverageChange{
		Status:               PaperActivityCadenceUnavailable,
		BaselineHorizonHours: sevenDays.HorizonHours, CurrentHorizonHours: twentyFour.HorizonHours,
		BaselineWindowStartedAt: sevenDays.WindowStartedAt, BaselineWindowEndedAt: sevenDays.WindowEndedAt,
		CurrentWindowStartedAt: twentyFour.WindowStartedAt, CurrentWindowEndedAt: twentyFour.WindowEndedAt,
		FinancialProviders: []string{}, CoverageMetrics: []PaperGuardrailCoverageMetricChange{},
		CheckChanges: []PaperGuardrailCheckChange{}, SymbolChanges: []PaperGuardrailSymbolChange{},
	}
}

func paperGuardrailShareChange(currentCount, currentTotal, baselineCount, baselineTotal int) string {
	current := int64(currentCount) * int64(baselineTotal)
	baseline := int64(baselineCount) * int64(currentTotal)
	switch {
	case current > baseline:
		return paperGuardrailShareIncreased
	case current < baseline:
		return paperGuardrailShareDecreased
	default:
		return paperGuardrailShareUnchanged
	}
}

func paperGuardrailCoverageMetric(metric string, currentCount, currentTotal, baselineCount, baselineTotal int) (PaperGuardrailCoverageMetricChange, bool) {
	currentRate := paperDispositionRate(currentCount, currentTotal)
	baselineRate := paperDispositionRate(baselineCount, baselineTotal)
	if currentRate == nil || baselineRate == nil {
		return PaperGuardrailCoverageMetricChange{}, false
	}
	return PaperGuardrailCoverageMetricChange{
		Metric: metric, BaselineCount: baselineCount, CurrentCount: currentCount, CountDelta: currentCount - baselineCount,
		BaselineSharePercent: *baselineRate, CurrentSharePercent: *currentRate,
		ShareChange: paperGuardrailShareChange(currentCount, currentTotal, baselineCount, baselineTotal),
	}, true
}

func paperGuardrailStageCounts(window PaperGuardrailEvidenceWindow) (map[string]PaperGuardrailCheckCount, bool) {
	plan := risk.AIAutonomousPaperCheckPlan()
	if len(window.ExpectedCheckCodes) != len(plan) {
		return nil, false
	}
	counts := make(map[string]PaperGuardrailCheckCount, len(plan))
	for index, stage := range plan {
		code := string(stage.CanonicalCode)
		if window.ExpectedCheckCodes[index] != code {
			return nil, false
		}
		counts[code] = PaperGuardrailCheckCount{Code: code}
	}
	for _, proposal := range window.Proposals {
		if len(proposal.Checks) == 0 || len(proposal.Checks) > len(plan) {
			return nil, false
		}
		for index, check := range proposal.Checks {
			if !paperGuardrailStageAccepts(plan[index], check.Code) {
				return nil, false
			}
			code := string(plan[index].CanonicalCode)
			stage := counts[code]
			stage.EvaluationCount++
			switch check.Result {
			case "PASS":
				stage.PassCount++
			case "FAIL":
				stage.FailCount++
			case "WARN":
				stage.WarnCount++
			default:
				return nil, false
			}
			counts[code] = stage
		}
	}
	return counts, true
}

func paperGuardrailCoverageChange(twentyFour, sevenDays PaperGuardrailEvidenceWindow) PaperGuardrailCoverageChange {
	result := unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
	if twentyFour.Status != PaperActivityCadenceAvailable || sevenDays.Status != PaperActivityCadenceAvailable ||
		twentyFour.HorizonHours != 24 || sevenDays.HorizonHours != 168 || twentyFour.WindowStartedAt == nil || twentyFour.WindowEndedAt == nil ||
		sevenDays.WindowStartedAt == nil || sevenDays.WindowEndedAt == nil || twentyFour.ProposalCount <= 0 || sevenDays.ProposalCount <= 0 ||
		!twentyFour.WindowEndedAt.Equal(*sevenDays.WindowEndedAt) || twentyFour.WindowStartedAt.Before(*sevenDays.WindowStartedAt) ||
		len(twentyFour.Proposals) != twentyFour.ProposalCount || len(sevenDays.Proposals) != sevenDays.ProposalCount {
		return result
	}
	baselineFacts := make(map[string]PaperGuardrailProposalFact, len(sevenDays.Proposals))
	providers := map[string]struct{}{}
	for index, proposal := range sevenDays.Proposals {
		if proposal.DecisionJournalEntryID == "" || proposal.RiskEvaluationID == "" || proposal.ExecutionRecordID == "" || proposal.FinancialProvider == "" ||
			proposal.CreatedAt.Before(*sevenDays.WindowStartedAt) || proposal.CreatedAt.After(*sevenDays.WindowEndedAt) || proposal.MarketObservedAt.After(proposal.CreatedAt) {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
		if _, duplicate := baselineFacts[proposal.DecisionJournalEntryID]; duplicate {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
		baselineFacts[proposal.DecisionJournalEntryID] = proposal
		providers[proposal.FinancialProvider] = struct{}{}
		if index == 0 || proposal.CreatedAt.Before(*result.FirstEvidenceAt) {
			value := proposal.CreatedAt
			result.FirstEvidenceAt = &value
		}
		if index == 0 || proposal.CreatedAt.After(*result.LatestEvidenceAt) {
			value := proposal.CreatedAt
			result.LatestEvidenceAt = &value
		}
		if index == 0 || proposal.MarketObservedAt.Before(*result.FirstMarketObservedAt) {
			value := proposal.MarketObservedAt
			result.FirstMarketObservedAt = &value
		}
		if index == 0 || proposal.MarketObservedAt.After(*result.LatestMarketObservedAt) {
			value := proposal.MarketObservedAt
			result.LatestMarketObservedAt = &value
		}
		if proposal.CoverageStatus == paperGuardrailCoverageDriftDetected {
			if result.FirstCheckSetDriftAt == nil || proposal.CreatedAt.Before(*result.FirstCheckSetDriftAt) {
				value := proposal.CreatedAt
				result.FirstCheckSetDriftAt = &value
			}
			if result.LatestCheckSetDriftAt == nil || proposal.CreatedAt.After(*result.LatestCheckSetDriftAt) {
				value := proposal.CreatedAt
				result.LatestCheckSetDriftAt = &value
			}
		}
	}
	for _, proposal := range twentyFour.Proposals {
		baseline, exists := baselineFacts[proposal.DecisionJournalEntryID]
		if !exists || !reflect.DeepEqual(baseline, proposal) {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
	}
	for provider := range providers {
		result.FinancialProviders = append(result.FinancialProviders, provider)
	}
	sort.Strings(result.FinancialProviders)

	for _, values := range []struct {
		metric                      string
		currentCount, baselineCount int
	}{
		{"FULL_EVALUATION", twentyFour.FullyEvaluatedCount, sevenDays.FullyEvaluatedCount},
		{"FAIL_CLOSED_PREFIX", twentyFour.FailClosedPrefixCount, sevenDays.FailClosedPrefixCount},
		{"CHECK_SET_DRIFT", twentyFour.CheckSetDriftCount, sevenDays.CheckSetDriftCount},
	} {
		metric, ok := paperGuardrailCoverageMetric(values.metric, values.currentCount, twentyFour.ProposalCount, values.baselineCount, sevenDays.ProposalCount)
		if !ok {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
		result.CoverageMetrics = append(result.CoverageMetrics, metric)
	}

	currentStages, currentOK := paperGuardrailStageCounts(twentyFour)
	baselineStages, baselineOK := paperGuardrailStageCounts(sevenDays)
	if !currentOK || !baselineOK {
		return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
	}
	for _, code := range paperGuardrailExpectedCheckCodes() {
		current, baseline := currentStages[code], baselineStages[code]
		currentRate := paperDispositionRate(current.EvaluationCount, twentyFour.ProposalCount)
		baselineRate := paperDispositionRate(baseline.EvaluationCount, sevenDays.ProposalCount)
		if currentRate == nil || baselineRate == nil {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
		result.CheckChanges = append(result.CheckChanges, PaperGuardrailCheckChange{
			Code:                    code,
			BaselineEvaluationCount: baseline.EvaluationCount, CurrentEvaluationCount: current.EvaluationCount, EvaluationCountDelta: current.EvaluationCount - baseline.EvaluationCount,
			BaselinePassCount: baseline.PassCount, CurrentPassCount: current.PassCount, PassCountDelta: current.PassCount - baseline.PassCount,
			BaselineFailCount: baseline.FailCount, CurrentFailCount: current.FailCount, FailCountDelta: current.FailCount - baseline.FailCount,
			BaselineWarnCount: baseline.WarnCount, CurrentWarnCount: current.WarnCount, WarnCountDelta: current.WarnCount - baseline.WarnCount,
			BaselineEvaluationPercent: *baselineRate, CurrentEvaluationPercent: *currentRate,
			EvaluationShareChange: paperGuardrailShareChange(current.EvaluationCount, twentyFour.ProposalCount, baseline.EvaluationCount, sevenDays.ProposalCount),
		})
	}

	type symbolWindow struct {
		value    PaperGuardrailSymbol
		notional *big.Rat
	}
	currentSymbols, baselineSymbols := map[string]symbolWindow{}, map[string]symbolWindow{}
	for _, pair := range []struct {
		window PaperGuardrailEvidenceWindow
		target map[string]symbolWindow
	}{{twentyFour, currentSymbols}, {sevenDays, baselineSymbols}} {
		for _, symbol := range pair.window.Symbols {
			notional, ok := new(big.Rat).SetString(symbol.ProposedNotional)
			key := symbol.Instrument + ":" + symbol.Symbol
			if !ok || notional.Sign() <= 0 || key == ":" {
				return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
			}
			if _, duplicate := pair.target[key]; duplicate {
				return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
			}
			pair.target[key] = symbolWindow{value: symbol, notional: notional}
		}
	}
	keys := make([]string, 0, len(baselineSymbols))
	for key := range baselineSymbols {
		keys = append(keys, key)
	}
	for key := range currentSymbols {
		if _, exists := baselineSymbols[key]; !exists {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		baseline := baselineSymbols[key]
		current := currentSymbols[key]
		currentRate := paperDispositionRate(current.value.ProposalCount, twentyFour.ProposalCount)
		baselineRate := paperDispositionRate(baseline.value.ProposalCount, sevenDays.ProposalCount)
		if currentRate == nil || baselineRate == nil {
			return unavailablePaperGuardrailCoverageChange(twentyFour, sevenDays)
		}
		currentNotional := new(big.Rat)
		if current.notional != nil {
			currentNotional.Set(current.notional)
		}
		result.SymbolChanges = append(result.SymbolChanges, PaperGuardrailSymbolChange{
			Symbol: baseline.value.Symbol, Instrument: baseline.value.Instrument,
			BaselineProposalCount: baseline.value.ProposalCount, CurrentProposalCount: current.value.ProposalCount, ProposalCountDelta: current.value.ProposalCount - baseline.value.ProposalCount,
			BaselineProposalPercent: *baselineRate, CurrentProposalPercent: *currentRate,
			ProposalShareChange: paperGuardrailShareChange(current.value.ProposalCount, twentyFour.ProposalCount, baseline.value.ProposalCount, sevenDays.ProposalCount),
			BaselineAllowCount:  baseline.value.AllowCount, CurrentAllowCount: current.value.AllowCount,
			BaselineDenyCount: baseline.value.DenyCount, CurrentDenyCount: current.value.DenyCount,
			BaselineSimulatedFillCount: baseline.value.SimulatedFillCount, CurrentSimulatedFillCount: current.value.SimulatedFillCount,
			BaselineProposedNotional: paperDecimal(baseline.notional), CurrentProposedNotional: paperDecimal(currentNotional),
			ProposedNotionalDelta: paperDecimal(new(big.Rat).Sub(currentNotional, baseline.notional)),
		})
	}

	result.Status, result.CalculationMethod = PaperActivityCadenceAvailable, PaperGuardrailCoverageChangeMethod
	result.BaselineProposalCount, result.CurrentProposalCount = sevenDays.ProposalCount, twentyFour.ProposalCount
	result.ProposalCountDelta = result.CurrentProposalCount - result.BaselineProposalCount
	return result
}

func unavailablePaperDenialEligibility(window PaperGuardrailEvidenceWindow) PaperDenialEligibility {
	return PaperDenialEligibility{
		Status: PaperActivityCadenceUnavailable, HorizonHours: window.HorizonHours,
		WindowStartedAt: window.WindowStartedAt, WindowEndedAt: window.WindowEndedAt,
		FinancialProviders: []string{}, Denials: []PaperDenialEligibilityFact{},
	}
}

func paperDenialRiskChanges(previous, later PaperGuardrailProposalFact) ([]PaperDenialRiskResultChange, bool) {
	plan := risk.AIAutonomousPaperCheckPlan()
	if len(previous.Checks) == 0 || len(previous.Checks) > len(plan) || len(later.Checks) == 0 || len(later.Checks) > len(plan) {
		return nil, false
	}
	limit := len(previous.Checks)
	if len(later.Checks) < limit {
		limit = len(later.Checks)
	}
	changes := []PaperDenialRiskResultChange{}
	for index := 0; index < limit; index++ {
		before, after := previous.Checks[index], later.Checks[index]
		if !paperGuardrailStageAccepts(plan[index], before.Code) || !paperGuardrailStageAccepts(plan[index], after.Code) {
			return nil, false
		}
		if before.Code != after.Code || before.Result != after.Result {
			changes = append(changes, PaperDenialRiskResultChange{
				Stage: string(plan[index].CanonicalCode), PreviousCode: before.Code, LaterCode: after.Code,
				PreviousResult: before.Result, LaterResult: after.Result,
			})
		}
	}
	return changes, true
}

func paperDenialEligibility(window PaperGuardrailEvidenceWindow) PaperDenialEligibility {
	result := unavailablePaperDenialEligibility(window)
	if window.Status != PaperActivityCadenceAvailable || window.WindowStartedAt == nil || window.WindowEndedAt == nil ||
		window.HorizonHours <= 0 || len(window.Proposals) != window.ProposalCount || window.AllowCount+window.DenyCount != window.ProposalCount {
		return result
	}
	providers := map[string]struct{}{}
	seen := map[string]struct{}{}
	for index, proposal := range window.Proposals {
		if proposal.DecisionJournalEntryID == "" || proposal.ProposedActionID == "" || proposal.RiskEvaluationID == "" || proposal.ExecutionRecordID == "" ||
			proposal.Symbol == "" || (proposal.Instrument != "EQUITY" && proposal.Instrument != "CRYPTO") || (proposal.Side != "BUY" && proposal.Side != "SELL") ||
			proposal.FinancialProvider == "" || proposal.MarketFeed == "" || proposal.MarketQuality == "" || proposal.CreatedAt.Before(*window.WindowStartedAt) ||
			proposal.CreatedAt.After(*window.WindowEndedAt) || proposal.MarketObservedAt.IsZero() || proposal.MarketObservedAt.After(proposal.CreatedAt) ||
			proposal.CoverageStatus == paperGuardrailCoverageDriftDetected {
			return unavailablePaperDenialEligibility(window)
		}
		if _, ok := paperGuardrailDecimal(proposal.ProposedQuantity); !ok {
			return unavailablePaperDenialEligibility(window)
		}
		if _, ok := paperGuardrailDecimal(proposal.ProposedNotional); !ok {
			return unavailablePaperDenialEligibility(window)
		}
		if _, duplicate := seen[proposal.DecisionJournalEntryID]; duplicate {
			return unavailablePaperDenialEligibility(window)
		}
		seen[proposal.DecisionJournalEntryID] = struct{}{}
		if index > 0 && !proposal.CreatedAt.After(window.Proposals[index-1].CreatedAt) {
			return unavailablePaperDenialEligibility(window)
		}
	}

	for index, denial := range window.Proposals {
		if denial.RiskDecision != "DENY" {
			continue
		}
		if denial.ExecutionStatus != "RISK_DENIED" || denial.CoverageStatus != paperGuardrailCoverageFailClosedPrefix ||
			len(denial.DenialReasonCodes) == 0 || len(denial.FailedCheckCodes) == 0 || denial.TerminalCheckStage == "" {
			return unavailablePaperDenialEligibility(window)
		}
		fact := PaperDenialEligibilityFact{
			DecisionJournalEntryID: denial.DecisionJournalEntryID, ProposedActionID: denial.ProposedActionID,
			RiskEvaluationID: denial.RiskEvaluationID, ExecutionRecordID: denial.ExecutionRecordID,
			CreatedAt: denial.CreatedAt, Symbol: denial.Symbol, Instrument: denial.Instrument, Side: denial.Side,
			ProposedQuantity: denial.ProposedQuantity, ProposedNotional: denial.ProposedNotional,
			DenialReasonCodes: append([]string(nil), denial.DenialReasonCodes...), FailedCheckCodes: append([]string(nil), denial.FailedCheckCodes...),
			TerminalCheckStage: denial.TerminalCheckStage, FinancialProvider: denial.FinancialProvider,
			MarketFeed: denial.MarketFeed, MarketQuality: denial.MarketQuality, MarketObservedAt: denial.MarketObservedAt,
			LaterDisposition: paperDenialNoLaterComparable, ChangedRiskResults: []PaperDenialRiskResultChange{},
		}
		providers[denial.FinancialProvider] = struct{}{}
		if result.FirstDenialAt == nil || denial.CreatedAt.Before(*result.FirstDenialAt) {
			value := denial.CreatedAt
			result.FirstDenialAt = &value
		}
		if result.LatestDenialAt == nil || denial.CreatedAt.After(*result.LatestDenialAt) {
			value := denial.CreatedAt
			result.LatestDenialAt = &value
		}

		for laterIndex := index + 1; laterIndex < len(window.Proposals); laterIndex++ {
			later := window.Proposals[laterIndex]
			if later.Symbol != denial.Symbol || later.Instrument != denial.Instrument || later.Side != denial.Side {
				continue
			}
			if !later.CreatedAt.After(denial.CreatedAt) {
				return unavailablePaperDenialEligibility(window)
			}
			changes, ok := paperDenialRiskChanges(denial, later)
			if !ok {
				return unavailablePaperDenialEligibility(window)
			}
			fact.LaterDecisionJournalEntryID, fact.LaterProposedActionID = later.DecisionJournalEntryID, later.ProposedActionID
			fact.LaterRiskEvaluationID, fact.LaterExecutionRecordID = later.RiskEvaluationID, later.ExecutionRecordID
			laterCreatedAt, laterMarketObservedAt := later.CreatedAt, later.MarketObservedAt
			fact.LaterCreatedAt, fact.LaterMarketObservedAt = &laterCreatedAt, &laterMarketObservedAt
			fact.LaterProposedQuantity, fact.LaterProposedNotional = later.ProposedQuantity, later.ProposedNotional
			fact.LaterRiskDecision, fact.LaterExecutionStatus = later.RiskDecision, later.ExecutionStatus
			fact.LaterFinancialProvider, fact.LaterMarketFeed, fact.LaterMarketQuality = later.FinancialProvider, later.MarketFeed, later.MarketQuality
			elapsed := int64(later.CreatedAt.Sub(denial.CreatedAt) / time.Second)
			if elapsed <= 0 {
				return unavailablePaperDenialEligibility(window)
			}
			fact.ElapsedSeconds, fact.ChangedRiskResults = &elapsed, changes
			providers[later.FinancialProvider] = struct{}{}
			switch {
			case later.RiskDecision == "ALLOW" && later.ExecutionStatus == "SIMULATED_FILLED" && later.CoverageStatus == paperGuardrailCoverageFullEvaluation:
				fact.LaterDisposition = paperDenialLaterAllowed
				result.LaterAllowedCount++
			case later.RiskDecision == "DENY" && later.ExecutionStatus == "RISK_DENIED" && later.CoverageStatus == paperGuardrailCoverageFailClosedPrefix:
				fact.LaterDisposition = paperDenialLaterDenied
				result.LaterDeniedCount++
			default:
				return unavailablePaperDenialEligibility(window)
			}
			break
		}
		if fact.LaterDisposition == paperDenialNoLaterComparable {
			result.NoLaterComparableProposalCount++
		}
		result.Denials = append(result.Denials, fact)
	}
	if len(result.Denials) != window.DenyCount || result.LaterAllowedCount+result.LaterDeniedCount+result.NoLaterComparableProposalCount != len(result.Denials) {
		return unavailablePaperDenialEligibility(window)
	}
	for provider := range providers {
		result.FinancialProviders = append(result.FinancialProviders, provider)
	}
	sort.Strings(result.FinancialProviders)
	result.Status, result.CalculationMethod, result.DenialCount = PaperActivityCadenceAvailable, PaperDenialEligibilityMethod, len(result.Denials)
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
	quantity, ok := paperGuardrailDecimal(row.ExecutionQuantity)
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
		ProposedQuantity: paperDecimal(quantity), ProposedNotional: paperDecimal(notional), RiskDecision: row.RiskDecision, ExecutionStatus: row.ExecutionStatus,
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
			!fill.SimulatedAt.Equal(row.CreatedAt) || !sameAIPaperDecimal(fill.Quantity, row.ExecutionQuantity) || !sameAIPaperDecimal(fill.RequestedNotional, rationale.ProposedNotional) ||
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
	denialWindow := twentyFour
	if sevenDays.Status == PaperActivityCadenceAvailable {
		denialWindow = sevenDays
	}
	result := PaperGuardrailEvidence{Status: PaperActivityCadenceUnavailable, TwentyFourHours: twentyFour, SevenDays: sevenDays,
		CoverageChange: paperGuardrailCoverageChange(twentyFour, sevenDays), DenialEligibility: paperDenialEligibility(denialWindow)}
	if cadence.AsOf != nil {
		value := *cadence.AsOf
		result.AsOf = &value
	}
	if twentyFour.Status == PaperActivityCadenceAvailable {
		result.Status, result.CalculationMethod = PaperActivityCadenceAvailable, PaperGuardrailEvidenceMethod
	}
	return result
}
