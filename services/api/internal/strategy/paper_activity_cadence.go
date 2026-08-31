package strategy

import (
	"math/big"
	"sort"
	"time"
)

const (
	PaperActivityCadenceAvailable   = "AVAILABLE"
	PaperActivityCadenceUnavailable = "UNAVAILABLE"
	PaperActivityCadenceMethod      = "IMMUTABLE_SCHEDULE_AND_SIMULATION_CHRONOLOGY"
	PaperFillTimingNoFills          = "NO_FILLS"
	PaperFillTimingInsufficient     = "INSUFFICIENT_INTERVALS"
)

func paperDurationSeconds(duration time.Duration) string {
	return paperDecimal(new(big.Rat).SetFrac(big.NewInt(duration.Nanoseconds()), big.NewInt(int64(time.Second))))
}

func paperDurationSummary(values []time.Duration) (minimum, median, maximum string) {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	minimum = paperDurationSeconds(values[0])
	maximum = paperDurationSeconds(values[len(values)-1])
	middle := len(values) / 2
	if len(values)%2 == 1 {
		median = paperDurationSeconds(values[middle])
		return
	}
	total := new(big.Int).Add(big.NewInt(values[middle-1].Nanoseconds()), big.NewInt(values[middle].Nanoseconds()))
	median = paperDecimal(new(big.Rat).SetFrac(total, big.NewInt(2*int64(time.Second))))
	return
}

func unavailablePaperActivityWindow(hours int, start, end time.Time, runs []ScheduleRun) PaperActivityWindow {
	window := paperActivityWindow(hours, start, end, runs)
	window.Status = PaperActivityCadenceUnavailable
	return window
}

func paperActivityWindow(hours int, start, end time.Time, runs []ScheduleRun) PaperActivityWindow {
	window := PaperActivityWindow{Status: PaperActivityCadenceAvailable, HorizonHours: hours, WindowStartedAt: &start, WindowEndedAt: &end}
	if len(runs) == 0 {
		return window
	}
	window.ObservedStartedAt = &runs[0].ScheduledFor
	window.ObservedEndedAt = &runs[len(runs)-1].CompletedAt
	window.ScheduledCycleCount = len(runs)
	for _, run := range runs {
		switch run.Status {
		case "FAILED":
			window.FailedCycleCount++
		case "SKIPPED":
			window.SafeWaitCycleCount++
		case "SUCCEEDED":
			window.SucceededCycleCount++
			switch {
			case run.AIDecision != nil && *run.AIDecision == "ABSTAIN":
				window.AbstentionCount++
			case run.ExecutionStatus != nil && *run.ExecutionStatus == "RISK_DENIED":
				window.DeterministicDenyCount++
			case run.ExecutionStatus != nil && *run.ExecutionStatus == "SIMULATED_FILLED":
				window.SimulatedFillCount++
			default:
				window.OtherSucceededCount++
			}
		}
	}
	return window
}

func paperActivityWindowEvidence(instanceStartedAt time.Time, interval time.Duration, hours int, asOf time.Time, allRuns []ScheduleRun) PaperActivityWindow {
	start := asOf.Add(-time.Duration(hours) * time.Hour)
	runs := make([]ScheduleRun, 0, len(allRuns))
	for _, run := range allRuns {
		if !run.ScheduledFor.Before(start) && !run.CompletedAt.After(asOf) {
			runs = append(runs, run)
		}
	}
	if instanceStartedAt.After(start) || len(runs) == 0 || runs[0].ScheduledFor.Sub(start) > interval || runs[len(runs)-1].CompletedAt != asOf {
		return unavailablePaperActivityWindow(hours, start, asOf, runs)
	}
	for index := 1; index < len(runs); index++ {
		if runs[index-1].NextRunAt != runs[index].ScheduledFor {
			return unavailablePaperActivityWindow(hours, start, asOf, runs)
		}
	}
	return paperActivityWindow(hours, start, asOf, runs)
}

func paperFillTiming(fills []paperRealizedFill, complete bool) PaperFillTimingEvidence {
	result := PaperFillTimingEvidence{Status: PaperActivityCadenceUnavailable, FillCount: len(fills), Symbols: []PaperFillTimingSymbol{}}
	if !complete {
		return result
	}
	result.HistoricalCoverage = PaperCoverageCompleteGenesis
	if len(fills) == 0 {
		result.Status = PaperFillTimingNoFills
		return result
	}
	result.FirstFillAt, result.LastFillAt = &fills[0].SimulatedAt, &fills[len(fills)-1].SimulatedAt
	bySymbol := map[string][]paperRealizedFill{}
	intervals := make([]time.Duration, 0, len(fills)-1)
	for index, fill := range fills {
		if index > 0 {
			intervals = append(intervals, fill.SimulatedAt.Sub(fills[index-1].SimulatedAt))
		}
		bySymbol[fill.Instrument+":"+fill.Symbol] = append(bySymbol[fill.Instrument+":"+fill.Symbol], fill)
	}
	if len(intervals) == 0 {
		result.Status = PaperFillTimingInsufficient
	} else {
		result.Status = PaperActivityCadenceAvailable
		result.MinimumInterFillSeconds, result.MedianInterFillSeconds, result.MaximumInterFillSeconds = paperDurationSummary(intervals)
	}
	keys := make([]string, 0, len(bySymbol))
	for key := range bySymbol {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		symbolFills := bySymbol[key]
		symbol := PaperFillTimingSymbol{Status: PaperFillTimingInsufficient, Symbol: symbolFills[0].Symbol, Instrument: symbolFills[0].Instrument, FillCount: len(symbolFills), FirstFillAt: &symbolFills[0].SimulatedAt, LastFillAt: &symbolFills[len(symbolFills)-1].SimulatedAt}
		if len(symbolFills) > 1 {
			symbolIntervals := make([]time.Duration, 0, len(symbolFills)-1)
			for index := 1; index < len(symbolFills); index++ {
				symbolIntervals = append(symbolIntervals, symbolFills[index].SimulatedAt.Sub(symbolFills[index-1].SimulatedAt))
			}
			symbol.Status = PaperActivityCadenceAvailable
			symbol.MinimumInterFillSeconds, symbol.MedianInterFillSeconds, symbol.MaximumInterFillSeconds = paperDurationSummary(symbolIntervals)
		}
		result.Symbols = append(result.Symbols, symbol)
	}
	return result
}

func paperLongestNoFillInterval(window PaperActivityWindow, runs []ScheduleRun) PaperNoFillIntervalEvidence {
	result := PaperNoFillIntervalEvidence{Status: PaperActivityCadenceUnavailable}
	if window.Status != PaperActivityCadenceAvailable || window.WindowStartedAt == nil || window.WindowEndedAt == nil {
		return result
	}
	windowRuns := make([]ScheduleRun, 0, len(runs))
	for _, run := range runs {
		if !run.ScheduledFor.Before(*window.WindowStartedAt) && !run.CompletedAt.After(*window.WindowEndedAt) {
			windowRuns = append(windowRuns, run)
		}
	}
	var currentStart time.Time
	currentCount := 0
	var bestStart, bestEnd time.Time
	bestCount := 0
	bestDuration := time.Duration(-1)
	for index, run := range windowRuns {
		filled := run.ExecutionStatus != nil && *run.ExecutionStatus == "SIMULATED_FILLED"
		if !filled {
			if currentCount == 0 {
				currentStart = run.ScheduledFor
			}
			currentCount++
			duration := run.CompletedAt.Sub(currentStart)
			if duration > bestDuration {
				bestDuration, bestCount, bestStart, bestEnd = duration, currentCount, currentStart, run.CompletedAt
			}
		}
		if filled || index == len(windowRuns)-1 {
			currentCount = 0
		}
	}
	if bestCount == 0 {
		result.Status = PaperFillTimingNoFills
		result.IntervalSeconds = "0.0000000000"
		return result
	}
	result.Status, result.CycleCount, result.IntervalSeconds = PaperActivityCadenceAvailable, bestCount, paperDurationSeconds(bestDuration)
	result.ScheduledStartedAt, result.CompletedEndedAt = &bestStart, &bestEnd
	return result
}

func projectPaperActivityCadence(instanceID string, instanceStartedAt time.Time, intervalMinutes int, runs []ScheduleRun, fills []paperRealizedFill, fillsComplete bool) PaperActivityCadence {
	unavailable := PaperActivityCadence{Status: PaperActivityCadenceUnavailable, ScheduleIntervalMinutes: intervalMinutes,
		TwentyFourHours: PaperActivityWindow{Status: PaperActivityCadenceUnavailable, HorizonHours: 24}, SevenDays: PaperActivityWindow{Status: PaperActivityCadenceUnavailable, HorizonHours: 168},
		FillTiming: PaperFillTimingEvidence{Status: PaperActivityCadenceUnavailable, FillCount: len(fills), Symbols: []PaperFillTimingSymbol{}}, LongestNoFillInterval: PaperNoFillIntervalEvidence{Status: PaperActivityCadenceUnavailable}}
	if instanceID == "" || instanceStartedAt.IsZero() || intervalMinutes < 30 || intervalMinutes > 1440 || len(runs) == 0 {
		return unavailable
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ScheduledFor.Before(runs[j].ScheduledFor) })
	seen := map[string]struct{}{}
	for index, run := range runs {
		if run.ID == "" || run.StrategyInstanceID != instanceID || run.ExecutionMode != Paper || run.ScheduledFor.IsZero() || run.StartedAt.Before(run.ScheduledFor) || run.CompletedAt.Before(run.StartedAt) || !run.NextRunAt.After(run.ScheduledFor) || (run.Status != "SUCCEEDED" && run.Status != "FAILED" && run.Status != "SKIPPED") {
			return unavailable
		}
		if _, exists := seen[run.ID]; exists {
			return unavailable
		}
		seen[run.ID] = struct{}{}
		if index > 0 && !runs[index].ScheduledFor.After(runs[index-1].ScheduledFor) {
			return unavailable
		}
	}
	asOf := runs[len(runs)-1].CompletedAt
	interval := time.Duration(intervalMinutes) * time.Minute
	twentyFour := paperActivityWindowEvidence(instanceStartedAt, interval, 24, asOf, runs)
	sevenDays := paperActivityWindowEvidence(instanceStartedAt, interval, 168, asOf, runs)
	fillTiming := paperFillTiming(fills, fillsComplete)
	status := PaperActivityCadenceAvailable
	if twentyFour.Status != PaperActivityCadenceAvailable || fillTiming.Status == PaperActivityCadenceUnavailable {
		status = PaperActivityCadenceUnavailable
	}
	return PaperActivityCadence{Status: status, CalculationMethod: PaperActivityCadenceMethod, AsOf: &asOf, ScheduleIntervalMinutes: intervalMinutes,
		TwentyFourHours: twentyFour, SevenDays: sevenDays, FillTiming: fillTiming, LongestNoFillInterval: paperLongestNoFillInterval(twentyFour, runs)}
}
