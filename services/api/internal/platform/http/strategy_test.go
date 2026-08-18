package http

import (
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/strategy"
)

func TestDecisionJournalCursorRoundTrips(t *testing.T) {
	want := &strategy.JournalCursor{
		CreatedAt: time.Date(2026, 8, 18, 18, 12, 30, 123, time.UTC),
		ID:        "11111111-1111-4111-8111-111111111111",
	}
	encoded := encodeJournalCursor(want)
	got, err := decodeJournalCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("cursor changed during round trip: %#v", got)
	}
}

func TestDecisionJournalCursorRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{"not-base64", "e30", encodeJournalCursor(&strategy.JournalCursor{CreatedAt: time.Now(), ID: "not-a-uuid"})} {
		if _, err := decodeJournalCursor(input); err == nil {
			t.Fatalf("malformed cursor was accepted: %q", input)
		}
	}
}
