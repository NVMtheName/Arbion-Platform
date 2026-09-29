package credential

import (
	"context"
	"errors"
	"testing"
)

type financialVersionFixture struct {
	*memoryStore
	generation                int64
	versionReads, legacyReads int
	err                       error
}

func (s *financialVersionFixture) Get(ctx context.Context, l Locator) ([]byte, error) {
	s.legacyReads++
	return s.memoryStore.Get(ctx, l)
}

func (s *financialVersionFixture) GetFinancialVersion(ctx context.Context, l Locator) ([]byte, int64, error) {
	s.versionReads++
	if s.err != nil {
		return nil, 0, s.err
	}
	material, err := s.memoryStore.Get(ctx, l)
	return material, s.generation, err
}

func TestEncryptedVaultFinancialVersionRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := &financialVersionFixture{memoryStore: newMemoryStore(), generation: 7}
	vault, err := NewEncryptedVault(make([]byte, 32), store)
	if err != nil {
		t.Fatal(err)
	}
	l := Locator{ConnectionID: "fixture-connection", UserID: "fixture-owner", Class: Financial}
	if err = vault.Store(ctx, l, []byte("synthetic-financial-credential")); err != nil {
		t.Fatal(err)
	}
	plaintext, generation, err := vault.RetrieveFinancialVersion(ctx, l)
	defer clear(plaintext)
	if err != nil || string(plaintext) != "synthetic-financial-credential" || generation != 7 || store.versionReads != 1 || store.legacyReads != 0 {
		t.Fatalf("versioned credential mismatch: generation=%d versionReads=%d legacyReads=%d err=%v", generation, store.versionReads, store.legacyReads, err)
	}
}

func TestEncryptedVaultFinancialVersionFailsClosed(t *testing.T) {
	ctx := context.Background()
	l := Locator{ConnectionID: "fixture-connection", UserID: "fixture-owner", Class: Financial}
	for _, name := range []string{"wrong class", "legacy store", "zero generation", "negative generation", "read failure", "tampered material", "short material", "foreign owner", "foreign connection", "AI material"} {
		t.Run(name, func(t *testing.T) {
			store := &financialVersionFixture{memoryStore: newMemoryStore(), generation: 7}
			vault, err := NewEncryptedVault(make([]byte, 32), store)
			if err != nil {
				t.Fatal(err)
			}
			if err = vault.Store(ctx, l, []byte("synthetic-financial-credential")); err != nil {
				t.Fatal(err)
			}
			requested := l
			switch name {
			case "wrong class":
				requested.Class = AI
			case "legacy store":
				vault, err = NewEncryptedVault(make([]byte, 32), store.memoryStore)
				if err != nil {
					t.Fatal(err)
				}
			case "zero generation":
				store.generation = 0
			case "negative generation":
				store.generation = -1
			case "read failure":
				store.err = errors.New("synthetic read failure")
			case "tampered material":
				store.data[l][len(store.data[l])-1] ^= 1
			case "short material":
				store.data[l] = []byte{1}
			case "foreign owner":
				requested.UserID = "another-owner"
				store.data[requested] = store.data[l]
			case "foreign connection":
				requested.ConnectionID = "another-connection"
				store.data[requested] = store.data[l]
			case "AI material":
				ai := l
				ai.Class = AI
				if err = vault.Store(ctx, ai, []byte("synthetic-ai-credential")); err != nil {
					t.Fatal(err)
				}
				store.data[l] = store.data[ai]
			}
			plaintext, generation, err := vault.RetrieveFinancialVersion(ctx, requested)
			defer clear(plaintext)
			if err == nil || plaintext != nil || generation != 0 || store.legacyReads != 0 {
				t.Fatalf("unsafe credential returned: generation=%d legacyReads=%d err=%v", generation, store.legacyReads, err)
			}
			if (name == "wrong class" || name == "legacy store") && store.versionReads != 0 {
				t.Fatal("unsupported retrieval reached storage")
			}
		})
	}
	if plaintext, generation, err := (*EncryptedVault)(nil).RetrieveFinancialVersion(ctx, l); err == nil || plaintext != nil || generation != 0 {
		t.Fatal("nil vault accepted")
	}
	// Wrong-class rejection must happen before a database call.
	if material, generation, err := (&PostgresStore{}).GetFinancialVersion(ctx, Locator{Class: AI}); err == nil || material != nil || generation != 0 {
		t.Fatal("non-financial database lookup accepted")
	}
}
