package strategy

import (
	"testing"
	"time"
)

func easternTime(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, easternLocation)
}

func TestPublishedNYSESessionWindows(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		open bool
	}{
		{name: "regular session", at: easternTime(2026, time.August, 18, 11, 0), open: true},
		{name: "before conservative open", at: easternTime(2026, time.August, 18, 9, 34), open: false},
		{name: "after conservative close", at: easternTime(2026, time.August, 18, 15, 55), open: false},
		{name: "good friday", at: easternTime(2026, time.April, 3, 11, 0), open: false},
		{name: "observed independence day", at: easternTime(2026, time.July, 3, 11, 0), open: false},
		{name: "early close before cutoff", at: easternTime(2026, time.November, 27, 12, 54), open: true},
		{name: "early close at cutoff", at: easternTime(2026, time.November, 27, 12, 55), open: false},
		{name: "outside reviewed calendar", at: easternTime(2029, time.January, 2, 11, 0), open: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := inRegularSession(testCase.at.UTC()); got != testCase.open {
				t.Fatalf("inRegularSession(%s)=%v, want %v", testCase.at, got, testCase.open)
			}
		})
	}
}

func TestNextRegularSessionUsesPublishedCalendar(t *testing.T) {
	tests := []struct {
		name  string
		after time.Time
		want  time.Time
		ok    bool
	}{
		{
			name:  "thanksgiving skips to early close day",
			after: easternTime(2026, time.November, 26, 10, 0),
			want:  easternTime(2026, time.November, 27, 9, 35),
			ok:    true,
		},
		{
			name:  "after early close skips weekend",
			after: easternTime(2026, time.November, 27, 13, 0),
			want:  easternTime(2026, time.November, 30, 9, 35),
			ok:    true,
		},
		{
			name:  "outside reviewed calendar",
			after: easternTime(2029, time.January, 2, 10, 0),
			ok:    false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := nextRegularSession(testCase.after.UTC())
			if ok != testCase.ok {
				t.Fatalf("nextRegularSession(%s) ok=%v, want %v", testCase.after, ok, testCase.ok)
			}
			if ok && !got.Equal(testCase.want.UTC()) {
				t.Fatalf("nextRegularSession(%s)=%s, want %s", testCase.after, got, testCase.want.UTC())
			}
		})
	}
}
