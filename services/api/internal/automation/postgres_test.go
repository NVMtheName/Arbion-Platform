package automation

import (
	"encoding/json"
	"testing"
)

func TestVersionChangeSummaryIsValidJSON(t *testing.T) {
	encoded, err := versionChangeSummary(7)
	if err != nil {
		t.Fatalf("encode version change summary: %v", err)
	}
	var summary map[string]int
	if err := json.Unmarshal(encoded, &summary); err != nil {
		t.Fatalf("version change summary is not valid JSON: %v", err)
	}
	if summary["previous_version"] != 7 {
		t.Fatalf("unexpected version change summary: %s", encoded)
	}
}
