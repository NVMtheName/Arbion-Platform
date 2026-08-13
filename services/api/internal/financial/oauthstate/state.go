package oauthstate

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var ErrInvalidState = errors.New("invalid, expired, or already used authorization state")

type Record struct {
	UserID    string
	ExpiresAt time.Time
}
type Store interface {
	Put(context.Context, string, Record) error
	Take(context.Context, string) (Record, error)
}
type Manager struct {
	store Store
	ttl   time.Duration
}

func New(store Store, ttl time.Duration) *Manager { return &Manager{store: store, ttl: ttl} }
func (m *Manager) Start(ctx context.Context, userID string) (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	return state, m.store.Put(ctx, state, Record{UserID: userID, ExpiresAt: time.Now().Add(m.ttl)})
}
func (m *Manager) Consume(ctx context.Context, state, userID string) error {
	if state == "" || userID == "" {
		return ErrInvalidState
	}
	r, e := m.store.Take(ctx, state)
	if e != nil || r.UserID != userID || time.Now().After(r.ExpiresAt) {
		return ErrInvalidState
	}
	return nil
}

type MemoryStore struct {
	mu     sync.Mutex
	values map[string]Record
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{values: map[string]Record{}} }
func (s *MemoryStore) Put(_ context.Context, k string, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[k] = r
	return nil
}
func (s *MemoryStore) Take(_ context.Context, k string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.values[k]
	delete(s.values, k)
	if !ok {
		return Record{}, ErrInvalidState
	}
	return r, nil
}
