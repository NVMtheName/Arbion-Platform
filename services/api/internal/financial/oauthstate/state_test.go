package oauthstate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStateEntropyBindingExpiryAndSingleUse(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	m := New(s, time.Minute)
	a, e := m.Start(ctx, "user-a")
	if e != nil || len(a) < 43 {
		t.Fatalf("weak state: %q %v", a, e)
	}
	b, _ := m.Start(ctx, "user-a")
	if a == b {
		t.Fatal("states repeated")
	}
	if !errors.Is(m.Consume(ctx, b, "user-b"), ErrInvalidState) {
		t.Fatal("state was not user-bound")
	}
	if !errors.Is(m.Consume(ctx, b, "user-a"), ErrInvalidState) {
		t.Fatal("mismatched attempt did not consume state")
	}
	if e = m.Consume(ctx, a, "user-a"); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(m.Consume(ctx, a, "user-a"), ErrInvalidState) {
		t.Fatal("state reused")
	}
	expired := "expired"
	_ = s.Put(ctx, expired, Record{UserID: "user-a", ExpiresAt: time.Now().Add(-time.Second)})
	if !errors.Is(m.Consume(ctx, expired, "user-a"), ErrInvalidState) {
		t.Fatal("expired state accepted")
	}
}
