package oauthstate

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestRedisStateIsUserBoundExpiringAndSingleUse(t *testing.T) {
	m := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: m.Addr()})
	manager := New(NewRedisStore(client), time.Minute)
	state, err := manager.Start(context.Background(), "user-a")
	if err != nil || len(state) < 43 {
		t.Fatalf("state entropy: %q %v", state, err)
	}
	if err := manager.Consume(context.Background(), state, "user-b"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("user swap accepted: %v", err)
	}
	if err := manager.Consume(context.Background(), state, "user-a"); !errors.Is(err, ErrInvalidState) {
		t.Fatal("mismatched attempt did not consume state")
	}
	expired, _ := manager.Start(context.Background(), "user-a")
	m.FastForward(2 * time.Minute)
	if _, err := manager.Take(context.Background(), expired); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expired state accepted: %v", err)
	}
}
