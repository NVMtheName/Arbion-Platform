package executionsim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var ErrJournal = errors.New("simulation journal unavailable or inconsistent; original evidence retained")

type record struct {
	Sequence int     `json:"sequence"`
	Previous string  `json:"previous"`
	Config   *Config `json:"config,omitempty"`
	Event    *Event  `json:"event,omitempty"`
	Hash     string  `json:"hash"`
}

// Journal is a single-writer local simulation log. Records are hash-linked,
// append-only and fsynced before acknowledgment. This detects accidental
// corruption, not malicious rewriting or rollback by a filesystem owner.
// It is not a production database, account ledger, or broker event consumer.
type Journal struct {
	mu       sync.Mutex
	file     *os.File
	engine   *Engine
	sequence int
	previous string
	poisoned bool
}

// OpenJournal replays every saved event. It never truncates or repairs a torn
// write. A malformed tail fails closed and preserves the original for review.
// flock excludes other processes; the mutex excludes concurrent goroutines.
func OpenJournal(path string, config Config, now time.Time) (*Journal, error) {
	engine, err := New(config, now)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, ErrJournal
	}
	f := os.NewFile(uintptr(fd), path)
	fail := func() (*Journal, error) { _ = f.Close(); return nil, ErrJournal }
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fail()
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 || info.Mode().Perm()&0077 != 0 {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return fail()
	}
	j := &Journal{file: f, engine: engine}
	if len(data) == 0 {
		if err := j.persist(record{Config: &config}); err != nil {
			return fail()
		}
		// Persist the newly created directory entry as well as file contents.
		dir, err := os.Open(filepath.Dir(path))
		if err != nil {
			return fail()
		}
		err = dir.Sync()
		_ = dir.Close()
		if err != nil {
			return fail()
		}
		return j, nil
	}
	if data[len(data)-1] != '\n' {
		return fail()
	}
	for index, line := range bytes.Split(data[:len(data)-1], []byte{'\n'}) {
		if index > 10000 || len(line) > 64<<10 {
			return fail()
		}
		r, err := decodeRecord(line)
		if err != nil || r.Sequence != index+1 || r.Previous != j.previous {
			return fail()
		}
		if index == 0 {
			want, _ := json.Marshal(config)
			got, _ := json.Marshal(r.Config)
			if r.Config == nil || r.Event != nil || !bytes.Equal(want, got) {
				return fail()
			}
		} else {
			if r.Config != nil || r.Event == nil {
				return fail()
			}
			if _, ok := engine.events[r.Event.ID]; ok {
				return fail()
			}
			if _, err := engine.Apply(*r.Event, now); err != nil {
				return fail()
			}
		}
		j.sequence, j.previous = r.Sequence, r.Hash
	}
	return j, nil
}

// Append persists new delivery IDs, including economic duplicate aliases.
// A durable record whose response was lost is safe to redeliver on restart.
func (j *Journal) Append(event Event, now time.Time) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.poisoned || j.file == nil {
		return false, ErrJournal
	}
	_, known := j.engine.events[event.ID]
	// Replay into a separate candidate before committing. A disk error never
	// publishes speculative state, and poisons this handle until reopened.
	candidate := *j.engine
	candidate.events = clone(j.engine.events)
	candidate.transactions = clone(j.engine.transactions)
	applied, err := candidate.Apply(event, now)
	if err != nil {
		return false, err
	}
	if known {
		return applied, nil
	}
	if err := j.persist(record{Event: &event}); err != nil {
		return false, err
	}
	j.engine = &candidate
	return applied, nil
}

func (j *Journal) Snapshot() (Snapshot, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.poisoned || j.file == nil {
		return Snapshot{}, ErrJournal
	}
	return j.engine.Snapshot(), nil
}

func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

func (j *Journal) persist(r record) error {
	r.Sequence, r.Previous = j.sequence+1, j.previous
	r.Hash = recordHash(r)
	data, err := json.Marshal(r)
	if err != nil {
		return ErrJournal
	}
	data = append(data, '\n')
	info, err := j.file.Stat()
	if err != nil || info.Size()+int64(len(data)) > 16<<20 {
		j.poisoned = true
		return ErrJournal
	}
	count, err := j.file.Write(data)
	if err != nil || count != len(data) {
		j.poisoned = true
		return ErrJournal
	}
	if err := j.file.Sync(); err != nil {
		j.poisoned = true
		return ErrJournal
	}
	j.sequence, j.previous = r.Sequence, r.Hash
	return nil
}

func recordHash(r record) string {
	r.Hash = ""
	data, _ := json.Marshal(r)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// Shared by file and PostgreSQL replay. Canonical encoding rejects duplicate
// JSON keys and alternate encodings instead of allowing decoder ambiguity.
func decodeRecord(data []byte) (record, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return record{}, ErrJournal
	}
	var r record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil || decoder.Decode(new(any)) != io.EOF {
		return record{}, ErrJournal
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(canonical, data) || r.Hash != recordHash(r) {
		return record{}, ErrJournal
	}
	return r, nil
}

func clone(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}

// Describe never includes raw event payloads or any credential-bearing source.
func Describe(s Snapshot) string {
	return fmt.Sprintf("SIMULATION ONLY | %s | cash USD %s | matched transactions %d | orders %d | funding review %t", s.Scope.Provider, s.Cash, s.MatchedTransactions, len(s.Orders), s.FundingReviewRequired)
}
