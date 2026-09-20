package strategy

import (
	"errors"
	"testing"
	"time"
)

func TestAICommitMandateWindow(t *testing.T) {
	from := time.Date(2026, 9, 20, 21, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	w := aiCommitMandateWindow{from: from, until: &until}
	for _, tc := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{"missing clock", time.Time{}, false},
		{"before start", from.Add(-time.Nanosecond), false},
		{"inclusive start", from, true},
		{"before end", until.Add(-time.Nanosecond), true},
		{"exclusive end", until, false},
		{"after end", until.Add(time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := w.current(tc.now); got != tc.want {
				t.Fatalf("current=%v, want %v", got, tc.want)
			}
		})
	}
	if (aiCommitMandateWindow{}).current(from) || !(aiCommitMandateWindow{from: from}).current(until) {
		t.Fatal("missing start or explicit open end handled incorrectly")
	}
}

func TestParseAICommitMandateWindow(t *testing.T) {
	start := `"2026-09-20T21:00:00Z"`
	for _, tc := range []struct{ from, until string }{
		{start, "null"},
		{start, `"2026-09-20T22:00:00+00:00"`},
	} {
		if _, err := parseAICommitMandateWindow([]byte(tc.from), []byte(tc.until)); err != nil {
			t.Fatal("valid saved window rejected", err)
		}
	}
	for _, tc := range []struct{ from, until string }{
		{"", "null"}, {"null", "null"}, {start, ""},
		{`"0001-01-01T00:00:00Z"`, "null"},
		{start, `"0001-01-01T00:00:00Z"`},
		{start, start}, {start, `"2026-09-20T20:00:00Z"`},
		{`"2026-09-20"`, "null"}, {start, `"bad-time"`},
		{`123`, "null"}, {start, `{}`},
	} {
		if _, err := parseAICommitMandateWindow([]byte(tc.from), []byte(tc.until)); !errors.Is(err, ErrEvaluationConfigurationChanged) {
			t.Fatal("incomplete or malformed saved window accepted", err)
		}
	}
}
