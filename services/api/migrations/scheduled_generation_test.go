package migrations

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestScheduledGenerationJSONSubtractionIsExplicitlyGrouped(t *testing.T) {
	body, err := fs.ReadFile(Files, "00064_execution_scheduled_generation.sql")
	if err != nil {
		t.Fatal(err)
	}
	// PostgreSQL gives subtraction higher precedence than JSON extraction.
	// Without parentheses it tries to subtract two unknown string literals,
	// instead of removing a key from the extracted JSON object (SQLSTATE 42725).
	for _, expression := range []string{
		"(NEW.facts->'Pilot')-'Limits'",
		"(NEW.facts->'Pilot'->'Limits')-'ExpiresAt'",
	} {
		if !strings.Contains(string(body), expression) {
			t.Errorf("missing explicitly grouped JSON subtraction: %s", expression)
		}
	}
	if regexp.MustCompile(`->\s*'[^']+'\s*-\s*'`).Match(body) {
		t.Fatal("JSON extraction followed by ambiguous ungrouped key subtraction")
	}
}
