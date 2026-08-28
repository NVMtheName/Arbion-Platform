package http

import (
	"encoding/json"
	"testing"
)

func TestAutomationCollectionsSerializeAsArrays(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"automations":          nonNil[map[string]any](nil),
		"capital_buckets":      nonNil[map[string]any](nil),
		"capital_reservations": nonNil[map[string]any](nil),
		"versions":             nonNil[map[string]any](nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"automations":[],"capital_buckets":[],"capital_reservations":[],"versions":[]}` {
		t.Fatalf("empty collections must be JSON arrays: %s", payload)
	}
}
