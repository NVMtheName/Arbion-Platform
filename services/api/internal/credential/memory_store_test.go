package credential

import (
	"context"
	"sync"
)

type memoryStore struct {
	mu   sync.Mutex
	data map[Locator][]byte
}

func newMemoryStore() *memoryStore { return &memoryStore{data: map[Locator][]byte{}} }
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

type stringError string

func (e stringError) Error() string { return string(e) }
func errorsNew(s string) error      { return stringError(s) }
