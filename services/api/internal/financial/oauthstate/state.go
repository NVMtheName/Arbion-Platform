package oauthstate

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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

// Take consumes a state and returns the user bound to it. It is used by OAuth
// callbacks, which must not depend on the initiating browser session surviving.
func (m *Manager) Take(ctx context.Context, state string) (Record, error) {
	if state == "" {
		return Record{}, ErrInvalidState
	}
	r, err := m.store.Take(ctx, state)
	if err != nil || r.UserID == "" || time.Now().After(r.ExpiresAt) {
		return Record{}, ErrInvalidState
	}
	return r, nil
}

type RedisStore struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client, prefix: "oauth:financial:"}
}
func (s *RedisStore) Put(ctx context.Context, key string, r Record) error {
	ttl := time.Until(r.ExpiresAt)
	if ttl <= 0 {
		return ErrInvalidState
	}
	return s.client.Set(ctx, s.prefix+key, r.UserID, ttl).Err()
}

var takeScript = redis.NewScript(`local v=redis.call('GET',KEYS[1]); if v then redis.call('DEL',KEYS[1]) end; return v`)

func (s *RedisStore) Take(ctx context.Context, key string) (Record, error) {
	v, err := takeScript.Run(ctx, s.client, []string{s.prefix + key}).Text()
	if err == redis.Nil || v == "" {
		return Record{}, ErrInvalidState
	}
	if err != nil {
		return Record{}, err
	}
	return Record{UserID: v, ExpiresAt: time.Now().Add(time.Minute)}, nil
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
