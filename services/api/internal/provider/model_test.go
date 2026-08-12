package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConnectionSerializationContainsNoSecretFields(t *testing.T) {
	data, err := json.Marshal(Connection{ID: "connection", CredentialMetadata: map[string]any{"credential_type": "oauth"}})
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, word := range []string{"encrypted_credential", "access_token", "refresh_token", "secret"} {
		if strings.Contains(body, word) {
			t.Fatalf("serialized model contains %q: %s", word, body)
		}
	}
}
