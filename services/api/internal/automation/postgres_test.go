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

func TestPaperOptionsSimulationAttestationIsVersionedAndPersisted(t *testing.T) {
	command := MandateCommand{PaperOptionsSimulationAttested: true}
	values := args("user", command, true)
	if len(values) != 20 || values[17] != true {
		t.Fatalf("attestation is not in the expected persisted argument position: %#v", values)
	}
	encoded, err := snapshot(Mandate{PaperOptionsSimulationAttested: true})
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["paper_options_simulation_attested"] != true {
		t.Fatalf("immutable snapshot omitted attestation: %s", encoded)
	}
}
