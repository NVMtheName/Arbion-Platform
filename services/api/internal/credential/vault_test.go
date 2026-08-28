package credential

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEncryptedVaultRoundTripAndReplace(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	vault, err := NewEncryptedVault(make([]byte, 32), store)
	if err != nil {
		t.Fatal(err)
	}
	l := Locator{ConnectionID: "c", UserID: "u", Class: AI}
	if err = vault.Store(ctx, l, []byte(`{"access_token":"private"}`)); err != nil {
		t.Fatal(err)
	}
	if string(store.data[l]) == `{"access_token":"private"}` {
		t.Fatal("plaintext was stored")
	}
	got, err := vault.Retrieve(ctx, l)
	if err != nil || string(got) != `{"access_token":"private"}` {
		t.Fatalf("retrieve: %q, %v", got, err)
	}
	if err = vault.Replace(ctx, l, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, err = vault.Retrieve(ctx, l)
	if err != nil || string(got) != "new" {
		t.Fatalf("replace: %q, %v", got, err)
	}
}
func TestEncryptedVaultDetectsTamperingAndContextSwap(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	vault, _ := NewEncryptedVault(make([]byte, 32), store)
	l := Locator{ConnectionID: "c", UserID: "u", Class: Financial}
	if err := vault.Store(ctx, l, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	store.data[l][len(store.data[l])-1] ^= 1
	if _, err := vault.Retrieve(ctx, l); err == nil {
		t.Fatal("expected tamper detection")
	}
	store2 := newMemoryStore()
	vault2, _ := NewEncryptedVault(make([]byte, 32), store2)
	_ = vault2.Store(ctx, l, []byte("secret"))
	other := Locator{ConnectionID: "c", UserID: "u", Class: AI}
	store2.data[other] = store2.data[l]
	if _, err := vault2.Retrieve(ctx, other); err == nil {
		t.Fatal("expected credential class binding")
	}
}

func TestEncryptedVaultStagesEncryptedTokenBoundCandidate(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	vault, err := NewEncryptedVault(make([]byte, 32), store)
	if err != nil {
		t.Fatal(err)
	}
	locator := Locator{ConnectionID: "c", UserID: "u", Class: AI}
	if err = vault.Store(ctx, locator, []byte("current")); err != nil {
		t.Fatal(err)
	}
	token, err := vault.Stage(ctx, locator, []byte("candidate"))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 || store.pending[locator].token != token {
		t.Fatalf("staging token is not a bound 256-bit value: %q", token)
	}
	if string(store.pending[locator].payload) == "candidate" || string(store.data[locator]) == "candidate" {
		t.Fatal("candidate was persisted in plaintext or replaced the current credential")
	}
	current, err := vault.Retrieve(ctx, locator)
	if err != nil || string(current) != "current" {
		t.Fatalf("staging changed the runtime credential: %q %v", current, err)
	}
	if err = vault.DiscardStaged(ctx, locator, strings.Repeat("0", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched staging token discarded another candidate: %v", err)
	}
	if err = vault.DiscardStaged(ctx, locator, token); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.pending[locator]; ok {
		t.Fatal("matching staged candidate was not discarded")
	}
}
