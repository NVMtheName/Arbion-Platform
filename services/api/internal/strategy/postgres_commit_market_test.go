package strategy

import (
	"testing"
	"time"
)

func TestAICommitMarketTimeUsesUnchangedFreshnessBounds(t *testing.T) {
	now := time.Date(2026, 9, 20, 21, 0, 0, 0, time.UTC)
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		maxAge, futureOffset := marketDataMaxAge, marketDataMaxFutureOffset
		if mode == Paper {
			maxAge, futureOffset = aiPaperMaximumMarketAge, aiPaperMaximumFutureOffset
		}
		for _, tc := range []struct {
			name string
			at   time.Time
			want bool
		}{
			{"current", now, true},
			{"maximum age", now.Add(-maxAge), true},
			{"expired", now.Add(-maxAge - time.Nanosecond), false},
			{"future tolerance", now.Add(futureOffset), true},
			{"future beyond tolerance", now.Add(futureOffset + time.Nanosecond), false},
			{"missing observation", time.Time{}, false},
		} {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				if got := currentAICommitMarketTime(&AIProposalQuoteReference{ObservedAt: tc.at}, mode, now); got != tc.want {
					t.Fatal("unexpected saved quote freshness", got)
				}
			})
		}
	}
	if currentAICommitMarketTime(nil, Paper, now) || currentAICommitMarketTime(&AIProposalQuoteReference{ObservedAt: now}, Paper, time.Time{}) || currentAICommitMarketTime(&AIProposalQuoteReference{ObservedAt: now}, ExecutionMode("LIVE"), now) {
		t.Fatal("missing clock or quote accepted")
	}
	if classifyScheduleError(ErrCommitMarketDataStale) != "COMMIT_MARKET_DATA_STALE" {
		t.Fatal("late quote expiry lost its failure stage")
	}
}
