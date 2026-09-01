package strategy

import (
	"encoding/json"
	"sort"
	"time"
)

const (
	PaperAutonomyThresholdChangeLedgerMethod = "IMMUTABLE_PAPER_EVIDENCE_THRESHOLD_CHANGE_LEDGER"
	PaperAutonomyThresholdChangeLedgerLimit  = 6
)

type paperAutonomyThresholdSnapshot struct {
	run                   ScheduleRun
	elapsedSeconds        int64
	remainingSeconds      int64
	evidenceWindowHours   int64
	decisionCount         int
	routeContinuityStatus string
	inputCoverageStatus   string
	inputFreshnessStatus  string
	evidenceStatus        string
	routes                []PaperAutonomyEvidenceRoute
	blockers              []PaperAutonomyEvidenceBlocker
}

func unavailablePaperAutonomyThresholdChangeLedger(instanceID string, sourceRunCount int) PaperAutonomyEvidenceThresholdChangeLedger {
	return PaperAutonomyEvidenceThresholdChangeLedger{
		Status: PaperAutonomyEvidenceUnavailable, CalculationMethod: PaperAutonomyThresholdChangeLedgerMethod,
		StrategyInstanceID: instanceID, ExecutionMode: Paper, SourceRunCount: sourceRunCount,
		Checkpoints: []PaperAutonomyEvidenceThresholdCheckpoint{}, GrantsAuthority: false, LivePromotionAvailable: false,
	}
}

func projectPaperAutonomyThresholdChangeLedger(instanceStartedAt time.Time, runs []ScheduleRun, decisions []paperAutonomyDecisionRow) PaperAutonomyEvidenceThresholdChangeLedger {
	instanceID := ""
	if len(runs) > 0 {
		instanceID = runs[0].StrategyInstanceID
	}
	result := unavailablePaperAutonomyThresholdChangeLedger(instanceID, len(runs))
	if instanceStartedAt.IsZero() || instanceID == "" || len(runs) == 0 {
		return result
	}
	seenRuns := map[string]struct{}{}
	for index, run := range runs {
		if run.ID == "" || run.StrategyInstanceID != instanceID || run.ExecutionMode != Paper || run.ScheduledFor.IsZero() || run.CompletedAt.IsZero() || run.CompletedAt.Before(run.ScheduledFor) || run.ConsecutiveFailures < 0 {
			return result
		}
		if _, exists := seenRuns[run.ID]; exists {
			return result
		}
		seenRuns[run.ID] = struct{}{}
		if index > 0 && (run.ScheduledFor.Before(runs[index-1].ScheduledFor) || !run.CompletedAt.After(runs[index-1].CompletedAt)) {
			return result
		}
	}

	snapshots := make([]paperAutonomyThresholdSnapshot, 0, len(runs))
	for index := range runs {
		snapshot, ok := paperAutonomyThresholdSnapshotAt(instanceStartedAt, runs[:index+1], decisions)
		if !ok {
			return result
		}
		snapshots = append(snapshots, snapshot)
	}

	start := 0
	if len(snapshots) > PaperAutonomyThresholdChangeLedgerLimit {
		start = len(snapshots) - PaperAutonomyThresholdChangeLedgerLimit
		result.Capped = true
	}
	for index := start; index < len(snapshots); index++ {
		current := snapshots[index]
		checkpoint := PaperAutonomyEvidenceThresholdCheckpoint{
			ScheduleRunID: current.run.ID, AsOf: current.run.CompletedAt,
			ElapsedSeconds: current.elapsedSeconds, RemainingSeconds: current.remainingSeconds,
			EvidenceWindowHours: current.evidenceWindowHours, DecisionCount: current.decisionCount,
			RouteContinuityStatus: current.routeContinuityStatus, RouteContinuityChange: "BASELINE",
			InputCoverageStatus: current.inputCoverageStatus, InputCoverageChange: "BASELINE",
			InputFreshnessStatus: current.inputFreshnessStatus, SchedulerStatus: current.run.Status,
			ConsecutiveFailures: current.run.ConsecutiveFailures, SchedulerChange: "BASELINE",
			EvidenceStatus: current.evidenceStatus, ProgressClassification: "BASELINE",
			Routes:            append([]PaperAutonomyEvidenceRoute(nil), current.routes...),
			Blockers:          append([]PaperAutonomyEvidenceBlocker(nil), current.blockers...),
			AddedBlockerCodes: []string{}, ResolvedBlockerCodes: []string{},
		}
		if index > 0 {
			previous := snapshots[index-1]
			checkpoint.PreviousScheduleRunID = previous.run.ID
			checkpoint.DecisionDelta = current.decisionCount - previous.decisionCount
			checkpoint.RouteContinuityChange = paperEvidenceCoverageChange(previous.routeContinuityStatus, current.routeContinuityStatus)
			checkpoint.InputCoverageChange = paperEvidenceCoverageChange(previous.inputCoverageStatus, current.inputCoverageStatus)
			checkpoint.SchedulerChange = paperEvidenceSchedulerChange(previous.run, current.run)
			checkpoint.AddedBlockerCodes, checkpoint.ResolvedBlockerCodes = paperEvidenceBlockerChanges(previous.blockers, current.blockers)
			checkpoint.ProgressClassification = paperEvidenceProgressClassification(previous, current, checkpoint)
		}
		result.Checkpoints = append(result.Checkpoints, checkpoint)
	}
	result.Status = "AVAILABLE"
	result.CheckpointCount = len(result.Checkpoints)
	return result
}

func paperAutonomyThresholdSnapshotAt(instanceStartedAt time.Time, runs []ScheduleRun, decisions []paperAutonomyDecisionRow) (paperAutonomyThresholdSnapshot, bool) {
	run := runs[len(runs)-1]
	asOf := run.CompletedAt.UTC()
	if asOf.Before(instanceStartedAt) {
		return paperAutonomyThresholdSnapshot{}, false
	}
	eligibleAt := instanceStartedAt.Add(PaperAutonomyEvidenceMinimumHours * time.Hour)
	windowStart := asOf.Add(-PaperAutonomyEvidenceMinimumHours * time.Hour)
	if instanceStartedAt.After(windowStart) {
		windowStart = instanceStartedAt
	}
	snapshot := paperAutonomyThresholdSnapshot{
		run: run, elapsedSeconds: int64(asOf.Sub(instanceStartedAt) / time.Second),
		evidenceWindowHours:   int64(asOf.Sub(windowStart) / time.Hour),
		routeContinuityStatus: "UNAVAILABLE", inputCoverageStatus: "UNAVAILABLE", inputFreshnessStatus: "UNAVAILABLE",
	}
	if asOf.Before(eligibleAt) {
		snapshot.remainingSeconds = int64(eligibleAt.Sub(asOf) / time.Second)
	}

	expectedDecisionCount := 0
	for _, candidate := range runs {
		if candidate.ScheduledFor.Before(windowStart) || candidate.CompletedAt.After(asOf) {
			continue
		}
		if candidate.Status == "SUCCEEDED" {
			expectedDecisionCount++
		}
	}
	routeCounts := map[string]*PaperAutonomyEvidenceRoute{}
	seenDecisions := map[string]struct{}{}
	attributedCount := 0
	freshCount := 0
	telemetryCount := 0
	boundedMemoryCount := 0
	malformed := false
	for _, row := range decisions {
		if row.CreatedAt.Before(windowStart) || row.CreatedAt.After(asOf) {
			continue
		}
		if row.ID == "" || row.CreatedAt.IsZero() {
			malformed = true
			continue
		}
		if _, exists := seenDecisions[row.ID]; exists {
			malformed = true
			continue
		}
		seenDecisions[row.ID] = struct{}{}
		snapshot.decisionCount++
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
			if row.CreatedAt.Sub(market.ObservedAt) > time.Duration(PaperAutonomyEvidenceFreshnessSeconds)*time.Second {
				marketsFresh = false
			}
		}
		if rationale.AIProvider != "" && rationale.ModelID != "" && rationale.Profile != "" && rationale.InputEvidence.Provider != "" && marketsAttributed {
			attributedCount++
			key := rationale.AIProvider + "\x00" + rationale.ModelID + "\x00" + rationale.Profile + "\x00" + rationale.InputEvidence.Provider
			route := routeCounts[key]
			if route == nil {
				route = &PaperAutonomyEvidenceRoute{AIProvider: rationale.AIProvider, ModelID: rationale.ModelID, Profile: rationale.Profile, FinancialProvider: rationale.InputEvidence.Provider}
				routeCounts[key] = route
			}
			route.DecisionCount++
		}
		if marketsFresh {
			freshCount++
		}
		if rationale.LatencyMS != nil && rationale.InputUsage != nil && rationale.OutputUsage != nil && *rationale.LatencyMS >= 0 && *rationale.InputUsage >= 0 && *rationale.OutputUsage >= 0 {
			telemetryCount++
		}
		if rationale.InputEvidence.RecentDecisions != nil && len(*rationale.InputEvidence.RecentDecisions) <= 6 {
			boundedMemoryCount++
		}
	}
	for _, route := range routeCounts {
		snapshot.routes = append(snapshot.routes, *route)
	}
	sort.Slice(snapshot.routes, func(i, j int) bool {
		left, right := snapshot.routes[i], snapshot.routes[j]
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
	if snapshot.decisionCount > 0 && attributedCount == snapshot.decisionCount {
		snapshot.inputCoverageStatus = "COMPLETE"
		if len(snapshot.routes) == 1 {
			snapshot.routeContinuityStatus = "STABLE"
		} else {
			snapshot.routeContinuityStatus = "CONTEXT_CHANGED"
		}
	} else if snapshot.decisionCount > 0 {
		snapshot.inputCoverageStatus = "REVIEW_REQUIRED"
		snapshot.routeContinuityStatus = "REVIEW_REQUIRED"
	}
	if snapshot.decisionCount > 0 && freshCount == snapshot.decisionCount {
		snapshot.inputFreshnessStatus = "CURRENT_AT_DECISION"
	} else if snapshot.decisionCount > 0 {
		snapshot.inputFreshnessStatus = "REVIEW_REQUIRED"
	}

	if malformed || snapshot.decisionCount != expectedDecisionCount {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("DECISION_EVIDENCE_INCONSISTENT", "UNAVAILABLE", "Saved decisions do not reconcile with the successful scheduler history at this checkpoint."))
	}
	if attributedCount != snapshot.decisionCount {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("DECISION_PROVENANCE_INCOMPLETE", "REVIEW", "One or more saved decisions lack exact route, provider, or market attribution at this checkpoint."))
	}
	if freshCount != snapshot.decisionCount {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("MARKET_INPUT_FRESHNESS_REVIEW_REQUIRED", "REVIEW", "One or more saved decisions lack provider-market evidence inside the five-minute policy at this checkpoint."))
	}
	if telemetryCount != snapshot.decisionCount {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("DECISION_TELEMETRY_INCOMPLETE", "REVIEW", "One or more saved decisions lack nonnegative latency or usage telemetry at this checkpoint."))
	}
	if boundedMemoryCount != snapshot.decisionCount {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("DECISION_MEMORY_UNBOUNDED", "REVIEW", "One or more saved decisions exceed the six-decision memory boundary at this checkpoint."))
	}
	if run.Status != "SUCCEEDED" || run.ConsecutiveFailures != 0 {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("SCHEDULER_NOT_HEALTHY", "REVIEW", "This immutable scheduler checkpoint is not a zero-failure success."))
	}
	if snapshot.evidenceWindowHours < PaperAutonomyEvidenceMinimumHours {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("EVIDENCE_WINDOW_INCOMPLETE", "COLLECTION", "Seven days of automatic Paper evidence had not elapsed at this checkpoint."))
	}
	if snapshot.decisionCount < PaperAutonomyEvidenceMinimumDecisions {
		snapshot.blockers = append(snapshot.blockers, paperAutonomyBlocker("DECISION_SAMPLE_INCOMPLETE", "COLLECTION", "Fewer than 20 automatic Paper decisions were saved at this checkpoint."))
	}
	sort.Slice(snapshot.blockers, func(i, j int) bool { return snapshot.blockers[i].Code < snapshot.blockers[j].Code })
	snapshot.evidenceStatus = PaperAutonomyEvidenceReviewable
	for _, blocker := range snapshot.blockers {
		if blocker.Category == "UNAVAILABLE" {
			snapshot.evidenceStatus = PaperAutonomyEvidenceUnavailable
			return snapshot, true
		}
	}
	for _, blocker := range snapshot.blockers {
		if blocker.Category == "REVIEW" {
			snapshot.evidenceStatus = PaperAutonomyEvidenceReviewRequired
			return snapshot, true
		}
	}
	if len(snapshot.blockers) > 0 {
		snapshot.evidenceStatus = PaperAutonomyEvidenceCollecting
	}
	return snapshot, true
}

func paperEvidenceCoverageChange(previous, current string) string {
	if previous == current {
		return "UNCHANGED"
	}
	if current == "UNAVAILABLE" || current == "REVIEW_REQUIRED" {
		return "REGRESSED"
	}
	if previous == "UNAVAILABLE" || previous == "REVIEW_REQUIRED" {
		return "IMPROVED"
	}
	return "CONTEXT_CHANGED"
}

func paperEvidenceSchedulerChange(previous, current ScheduleRun) string {
	if current.Status == "FAILED" {
		if previous.Status == "FAILED" {
			return "INCIDENT_CONTINUES"
		}
		return "INCIDENT_OPENED"
	}
	if current.Status == "SUCCEEDED" && previous.Status == "FAILED" {
		return "RECOVERED"
	}
	if current.Status == "SKIPPED" && current.ErrorCode != nil && *current.ErrorCode == "OUTSIDE_SESSION" {
		return "SAFE_WAIT"
	}
	if current.Status == previous.Status && current.ConsecutiveFailures == previous.ConsecutiveFailures {
		return "UNCHANGED"
	}
	return "CONTEXT_CHANGED"
}

func paperEvidenceBlockerChanges(previous, current []PaperAutonomyEvidenceBlocker) ([]string, []string) {
	previousCodes := map[string]struct{}{}
	currentCodes := map[string]struct{}{}
	for _, blocker := range previous {
		previousCodes[blocker.Code] = struct{}{}
	}
	for _, blocker := range current {
		currentCodes[blocker.Code] = struct{}{}
	}
	added, resolved := []string{}, []string{}
	for code := range currentCodes {
		if _, exists := previousCodes[code]; !exists {
			added = append(added, code)
		}
	}
	for code := range previousCodes {
		if _, exists := currentCodes[code]; !exists {
			resolved = append(resolved, code)
		}
	}
	sort.Strings(added)
	sort.Strings(resolved)
	return added, resolved
}

func paperEvidenceProgressClassification(previous, current paperAutonomyThresholdSnapshot, checkpoint PaperAutonomyEvidenceThresholdCheckpoint) string {
	if current.evidenceStatus == PaperAutonomyEvidenceUnavailable || (current.evidenceStatus == PaperAutonomyEvidenceReviewRequired && previous.evidenceStatus != PaperAutonomyEvidenceReviewRequired) {
		return "REVIEW_REGRESSION"
	}
	if checkpoint.SchedulerChange == "RECOVERED" || ((previous.evidenceStatus == PaperAutonomyEvidenceUnavailable || previous.evidenceStatus == PaperAutonomyEvidenceReviewRequired) && current.evidenceStatus != PaperAutonomyEvidenceUnavailable && current.evidenceStatus != PaperAutonomyEvidenceReviewRequired) {
		return "RECOVERED"
	}
	if checkpoint.RouteContinuityChange == "CONTEXT_CHANGED" || checkpoint.InputCoverageChange == "CONTEXT_CHANGED" {
		return "CONTEXT_CHANGED"
	}
	if checkpoint.DecisionDelta > 0 || current.remainingSeconds < previous.remainingSeconds || len(checkpoint.ResolvedBlockerCodes) > 0 {
		return "NORMAL_COLLECTION"
	}
	return "HELD"
}
