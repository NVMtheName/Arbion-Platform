package auth

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// Embedding only the legacy interface deliberately hides the exact-factor
// operation, even though the underlying fixture can implement it.
type legacyExecutionMFAStore struct{ MFAStore }

type replacingExecutionMFAStore struct {
	*fakeMFAStore
	changeEnabledAt bool
}

func (s replacingExecutionMFAStore) AdvanceExecutionTOTPStep(ctx context.Context, userID string, expected TOTPFactor, step int64, now time.Time) (bool, error) {
	if s.changeEnabledAt {
		replacement := s.factor.EnabledAt.Add(time.Microsecond)
		s.factor.EnabledAt = &replacement
	} else {
		s.factor.SecretCiphertext = bytes.Clone(s.factor.SecretCiphertext)
		s.factor.SecretCiphertext[0] ^= 1
	}
	return s.fakeMFAStore.AdvanceExecutionTOTPStep(ctx, userID, expected, step, now)
}

func executionStepUpFixture(t *testing.T) (*Service, *fakeMFAStore, string, time.Time) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := NewRedisStore(client)
	protector, err := NewMFASecretProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("execution-fixture-key")
	ciphertext, err := protector.Seal("user-1", secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	enabledAt := now.Add(-time.Hour)
	store := &fakeMFAStore{factor: TOTPFactor{SecretCiphertext: ciphertext, EnabledAt: &enabledAt}, lastUsedStep: now.Unix()/30 - 1}
	service := NewService(&fakeUsers{}, sessions, sessions, &fakeAudit{}, time.Hour)
	service.now = func() time.Time { return now }
	service.ConfigureMFA(store, sessions, protector)
	return service, store, totpCode(secret, now.Unix()/30), now
}

func TestExecutionStepUpRequiresExactFactorAndFreshTOTP(t *testing.T) {
	t.Run("one use and no recovery", func(t *testing.T) {
		service, store, code, now := executionStepUpFixture(t)
		codes, hashes, err := generateRecoveryCodes()
		if err != nil {
			t.Fatal(err)
		}
		recoveryKey := hex.EncodeToString(hashes[0])
		store.recovery = map[string]bool{recoveryKey: false}
		if _, _, err := service.VerifyExecutionStepUp(context.Background(), "user-1", codes[0]); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("execution accepted a recovery code: %v", err)
		}
		if store.recovery[recoveryKey] {
			t.Fatal("execution verification consumed a recovery code")
		}
		method, verified, err := service.VerifyExecutionStepUp(context.Background(), "user-1", code)
		if err != nil || method != "totp" || !verified.Equal(now) {
			t.Fatalf("exact execution step-up failed: method=%q verified=%v err=%v", method, verified, err)
		}
		if _, _, err = service.VerifyExecutionStepUp(context.Background(), "user-1", code); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("replayed execution step-up was accepted: %v", err)
		}
	})
	t.Run("legacy store cannot fall back", func(t *testing.T) {
		service, store, code, _ := executionStepUpFixture(t)
		before := store.lastUsedStep
		service.mfaStore = legacyExecutionMFAStore{store}
		if _, _, err := service.VerifyExecutionStepUp(context.Background(), "user-1", code); !errors.Is(err, ErrMFAUnavailable) {
			t.Fatalf("execution fell back to user-only advance: %v", err)
		}
		if store.lastUsedStep != before {
			t.Fatal("unsupported execution store consumed a TOTP step")
		}
	})
	t.Run("missing store", func(t *testing.T) {
		service, _, code, _ := executionStepUpFixture(t)
		service.mfaStore = nil
		if _, _, err := service.VerifyExecutionStepUp(context.Background(), "user-1", code); !errors.Is(err, ErrMFAUnavailable) {
			t.Fatalf("missing execution MFA store was accepted: %v", err)
		}
	})
	for _, changeEnabledAt := range []bool{false, true} {
		name := "ciphertext replaced after verification"
		if changeEnabledAt {
			name = "factor re-enrolled after verification"
		}
		t.Run(name, func(t *testing.T) {
			service, store, code, _ := executionStepUpFixture(t)
			before := store.lastUsedStep
			service.mfaStore = replacingExecutionMFAStore{store, changeEnabledAt}
			if _, _, err := service.VerifyExecutionStepUp(context.Background(), "user-1", code); !errors.Is(err, ErrInvalidMFACode) {
				t.Fatalf("replaced factor authorized execution: %v", err)
			}
			if store.lastUsedStep != before {
				t.Fatal("stale verification consumed the replacement factor")
			}
		})
	}
}

type executionFactorDB struct {
	query string
	args  []any
	rows  int64
}

func (d *executionFactorDB) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	d.query, d.args = query, args
	if d.rows == 1 {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}
func (*executionFactorDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("unexpected query")
}
func (*executionFactorDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected query")
}

func TestPostgresExecutionFactorAdvanceBindsCiphertextAndEnrollment(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	enabled := now.Add(-time.Hour)
	factor := TOTPFactor{SecretCiphertext: make([]byte, 32), EnabledAt: &enabled}
	db := &executionFactorDB{rows: 1}
	advanced, err := NewPostgresStore(db).AdvanceExecutionTOTPStep(context.Background(), "user-1", factor, 42, now)
	if err != nil || !advanced {
		t.Fatalf("exact factor did not advance: %v %v", advanced, err)
	}
	for _, predicate := range []string{"user_id=$1", "last_used_step<$2", "secret_ciphertext=$4", "enabled_at=$5"} {
		if !strings.Contains(db.query, predicate) {
			t.Fatalf("execution factor write lacks %s", predicate)
		}
	}
	if len(db.args) != 5 || !bytes.Equal(db.args[3].([]byte), factor.SecretCiphertext) || !db.args[4].(time.Time).Equal(enabled) {
		t.Fatal("execution factor write did not bind both opaque factor identities")
	}
	db.rows = 0
	if advanced, err = NewPostgresStore(db).AdvanceExecutionTOTPStep(context.Background(), "user-1", factor, 43, now); err != nil || advanced {
		t.Fatalf("factor mismatch was not rejected: %v %v", advanced, err)
	}
}
