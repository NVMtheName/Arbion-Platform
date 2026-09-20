package strategy

import (
	"errors"
	"testing"
	"time"
)

func TestAICommitActivityDay(t *testing.T) {
	now := time.Date(2026, 9, 20, 0, 0, 1, 0, time.UTC)
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{
		{now, true}, {now.Add(-time.Second), true},
		{now.Add(-2 * time.Second), false}, {now.Add(time.Microsecond), false},
		{now.AddDate(-1, 0, 0), false}, {time.Time{}, false},
		{now.In(time.FixedZone("offset", -4*60*60)), true},
	} {
		if sameAICommitActivityDay(tc.at, now) != tc.want {
			t.Fatal("wrong UTC activity boundary", tc.at)
		}
	}
	if sameAICommitActivityDay(now, time.Time{}) {
		t.Fatal("missing clock accepted")
	}
}

func TestParseAICommitActionLimit(t *testing.T) {
	for _, raw := range []string{``, `null`, `[]`, `{"max_trades_per_day":-1}`, `{"max_trades_per_day":"1"}`, `{"max_trades_per_day":1.5}`, `{"max_trades_per_day":1} trailing`} {
		if _, err := parseAICommitActionLimit([]byte(raw)); !errors.Is(err, ErrEvaluationConfigurationChanged) {
			t.Fatal("invalid limit accepted", raw, err)
		}
	}
	for _, raw := range []string{`{}`, `{"max_trades_per_day":null}`} {
		if limit, err := parseAICommitActionLimit([]byte(raw)); err != nil || limit != nil {
			t.Fatal("legacy optional limit changed", raw, err)
		}
	}
	if limit, err := parseAICommitActionLimit([]byte(`{"max_trades_per_day":1}`)); err != nil || limit == nil || *limit != 1 {
		t.Fatal("pinned limit lost", err)
	}
}
