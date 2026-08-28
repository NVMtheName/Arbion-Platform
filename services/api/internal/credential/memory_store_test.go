package credential

import (
	"context"
	"sync"
)

type memoryStore struct {
	mu      sync.Mutex
	data    map[Locator][]byte
	pending map[Locator]pendingCredential
}

type pendingCredential struct {
	payload []byte
	token   string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: map[Locator][]byte{}, pending: map[Locator]pendingCredential{}}
}
func (s *memoryStore) Put(_ context.Context, l Locator, b []byte, create bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.data[l]
	if create && exists {
		return errorsNew("exists")
	}
	if !create && !exists {
		return ErrNotFound
	}
	s.data[l] = append([]byte(nil), b...)
	return nil
}
func (s *memoryStore) Get(_ context.Context, l Locator) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.data[l]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), b...), nil
}
func (s *memoryStore) Delete(_ context.Context, l Locator) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[l]; !ok {
		return ErrNotFound
	}
	delete(s.data, l)
	return nil
}

func (s *memoryStore) PutStaged(_ context.Context, l Locator, payload []byte, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[l]; !ok {
		return ErrNotFound
	}
	s.pending[l] = pendingCredential{payload: append([]byte(nil), payload...), token: token}
	return nil
}

func (s *memoryStore) DeleteStaged(_ context.Context, l Locator, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[l]
	if !ok || pending.token != token {
		return ErrNotFound
	}
	delete(s.pending, l)
	return nil
}

type stringError string

func (e stringError) Error() string { return string(e) }
func errorsNew(s string) error      { return stringError(s) }
