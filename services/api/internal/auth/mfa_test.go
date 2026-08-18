package auth

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeMFAStore struct {
	factor       TOTPFactor
	lastUsedStep int64
	recovery     map[string]bool
}

func (f *fakeMFAStore) MFAStatus(context.Context, string) (MFAStatus, error) {
	remaining := 0
	for _, used := range f.recovery {
		if !used {
			remaining++
		}
	}
	return MFAStatus{Enabled: f.factor.EnabledAt != nil, RecoveryCodesRemaining: remaining}, nil
}

func (f *fakeMFAStore) SetPendingTOTP(_ context.Context, _ string, ciphertext []byte, expiresAt, _ time.Time) error {
	if f.factor.EnabledAt != nil {
		return ErrMFAAlreadyEnabled
	}
	f.factor = TOTPFactor{SecretCiphertext: bytes.Clone(ciphertext), PendingExpiresAt: &expiresAt}
	return nil
}

func (f *fakeMFAStore) TOTPFactor(context.Context, string) (TOTPFactor, error) {
	if len(f.factor.SecretCiphertext) == 0 {
		return TOTPFactor{}, ErrMFANotEnabled
	}
	return f.factor, nil
}

func (f *fakeMFAStore) ActivateTOTP(_ context.Context, _ string, hashes [][]byte, initialStep int64, now time.Time) error {
	if f.factor.PendingExpiresAt == nil || !f.factor.PendingExpiresAt.After(now) {
		return ErrMFAEnrollmentExpired
	}
	f.factor.PendingExpiresAt = nil
	f.factor.EnabledAt = &now
	f.lastUsedStep = initialStep
	f.recovery = map[string]bool{}
	for _, hash := range hashes {
		f.recovery[hex.EncodeToString(hash)] = false
	}
	return nil
}

func (f *fakeMFAStore) AdvanceTOTPStep(_ context.Context, _ string, step int64, _ time.Time) (bool, error) {
	if f.factor.EnabledAt == nil || step <= f.lastUsedStep {
		return false, nil
	}
	f.lastUsedStep = step
	return true, nil
}

func (f *fakeMFAStore) ConsumeRecoveryCode(_ context.Context, _ string, hash []byte, _ time.Time) (bool, error) {
	key := hex.EncodeToString(hash)
	used, found := f.recovery[key]
	if !found || used {
		return false, nil
	}
	f.recovery[key] = true
	return true, nil
}

func (f *fakeMFAStore) ReplaceRecoveryCodes(_ context.Context, _ string, hashes [][]byte, _ time.Time) error {
	if f.factor.EnabledAt == nil {
		return ErrMFANotEnabled
	}
	f.recovery = map[string]bool{}
	for _, hash := range hashes {
		f.recovery[hex.EncodeToString(hash)] = false
	}
	return nil
}

func (f *fakeMFAStore) DisableTOTP(context.Context, string) (bool, error) {
	if f.factor.EnabledAt == nil {
		return false, nil
	}
	f.factor = TOTPFactor{}
	f.recovery = nil
	return true, nil
}

func TestTOTPMatchesRFC6238AndRejectsAdjacentGarbage(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(59, 0).UTC()
	if code := totpCode(secret, now.Unix()/30); code != "287082" {
		t.Fatalf("RFC 6238 code mismatch: %s", code)
	}
	if step, ok := matchingTOTPStep(secret, "287082", now); !ok || step != 1 {
		t.Fatalf("valid TOTP rejected: step=%d valid=%v", step, ok)
	}
	for _, invalid := range []string{"28708", "2870820", "28A082", " 28 7082 "} {
		if _, ok := matchingTOTPStep(secret, invalid, now); ok {
			t.Fatalf("invalid TOTP accepted: %q", invalid)
		}
	}
}

func TestMFASecretProtectionBindsCiphertextToUser(t *testing.T) {
	protector, err := NewMFASecretProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := protector.Seal("user-1", []byte("private-totp-secret"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := protector.Open("user-1", ciphertext)
	if err != nil || string(plaintext) != "private-totp-secret" {
		t.Fatalf("MFA secret round trip failed: %q %v", plaintext, err)
	}
	if _, err = protector.Open("user-2", ciphertext); err == nil {
		t.Fatal("MFA ciphertext was not bound to its user")
	}
	tampered := bytes.Clone(ciphertext)
	tampered[len(tampered)-1] ^= 1
	if _, err = protector.Open("user-1", tampered); err == nil {
		t.Fatal("tampered MFA ciphertext was accepted")
	}
}

func TestRedisMFAChallengeIsHashedSinglePurposeAndExpires(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisStore(client)
	token, err := store.CreateMFAChallenge(context.Background(), "user-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range mini.Keys() {
		if bytes.Contains([]byte(key), []byte(token)) {
			t.Fatal("raw MFA challenge appeared in Redis key")
		}
	}
	challenge, err := store.GetMFAChallenge(context.Background(), token)
	if err != nil || challenge.UserID != "user-1" {
		t.Fatalf("challenge lookup failed: %#v %v", challenge, err)
	}
	consumed, err := store.ConsumeMFAChallenge(context.Background(), token)
	if err != nil || consumed.UserID != "user-1" {
		t.Fatalf("challenge consumption failed: %#v %v", consumed, err)
	}
	if _, err = store.GetMFAChallenge(context.Background(), token); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("consumed challenge returned %v", err)
	}
	token, err = store.CreateMFAChallenge(context.Background(), "user-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	mini.FastForward(6 * time.Minute)
	if _, err = store.GetMFAChallenge(context.Background(), token); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatalf("expired challenge returned %v", err)
	}
}

func TestMFALifecycleRequiresSecondFactorPreventsReplayAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	sessions := NewRedisStore(redisClient)
	users := &fakeUsers{}
	audit := &fakeAudit{}
	store := &fakeMFAStore{}
	protector, err := NewMFASecretProtector(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	sessions.now = func() time.Time { return now }
	service := NewService(users, sessions, sessions, audit, time.Hour)
	service.now = func() time.Time { return now }
	service.ConfigureMFA(store, sessions, protector)

	_, originalSession, err := service.Register(ctx, "person@example.com", "correct horse battery staple", "Person", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.BeginTOTPEnrollment(ctx, users.user.ID, "wrong password"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("wrong enrollment password returned %v", err)
	}
	enrollment, err := service.BeginTOTPEnrollment(ctx, users.user.ID, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := noPaddingBase32.DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	enrollmentCode := totpCode(secret, now.Unix()/30)
	if _, err = service.ConfirmTOTPEnrollment(ctx, users.user.ID, "000000"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("invalid enrollment code returned %v", err)
	}
	recoveryCodes, err := service.ConfirmTOTPEnrollment(ctx, users.user.ID, enrollmentCode)
	if err != nil || len(recoveryCodes) != recoveryCodeCount {
		t.Fatalf("MFA activation failed: codes=%d error=%v", len(recoveryCodes), err)
	}
	if _, err = sessions.Get(ctx, originalSession); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("MFA activation retained a password-only session")
	}

	now = now.Add(totpPeriod)
	login, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip")
	if err != nil || !login.MFARequired || login.SessionToken != "" || login.ChallengeToken == "" {
		t.Fatalf("password phase did not issue only an MFA challenge: %#v %v", login, err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, login.ChallengeToken, "000000", "ip"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("invalid MFA code returned %v", err)
	}
	loginCode := totpCode(secret, now.Unix()/30)
	_, sessionToken, err := service.CompleteMFALogin(ctx, login.ChallengeToken, loginCode, "ip")
	if err != nil || sessionToken == "" {
		t.Fatalf("MFA login failed: %v", err)
	}
	if _, err = sessions.Get(ctx, sessionToken); err != nil {
		t.Fatalf("MFA login did not create a session: %v", err)
	}

	replay, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, replay.ChallengeToken, loginCode, "ip-2"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replayed TOTP returned %v", err)
	}

	recoveryLogin, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip-3")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, recoveryLogin.ChallengeToken, recoveryCodes[0], "ip-3"); err != nil {
		t.Fatalf("recovery code login failed: %v", err)
	}
	recoveryReplay, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip-4")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, recoveryReplay.ChallengeToken, recoveryCodes[0], "ip-4"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replayed recovery code returned %v", err)
	}

	now = now.Add(totpPeriod)
	replacementCode := totpCode(secret, now.Unix()/30)
	replacementCodes, err := service.RegenerateRecoveryCodes(ctx, users.user.ID, "correct horse battery staple", replacementCode)
	if err != nil || len(replacementCodes) != recoveryCodeCount {
		t.Fatalf("recovery code regeneration failed: codes=%d error=%v", len(replacementCodes), err)
	}
	oldRecoveryLogin, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip-5")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, oldRecoveryLogin.ChallengeToken, recoveryCodes[1], "ip-5"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replaced recovery code returned %v", err)
	}
	newRecoveryLogin, err := service.BeginLogin(ctx, "person@example.com", "correct horse battery staple", "ip-6")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.CompleteMFALogin(ctx, newRecoveryLogin.ChallengeToken, replacementCodes[0], "ip-6"); err != nil {
		t.Fatalf("replacement recovery code login failed: %v", err)
	}

	now = now.Add(totpPeriod)
	disableCode := totpCode(secret, now.Unix()/30)
	if err = service.DisableTOTP(ctx, users.user.ID, "correct horse battery staple", disableCode); err != nil {
		t.Fatalf("MFA disable failed: %v", err)
	}
	status, err := service.MFAStatus(ctx, users.user.ID)
	if err != nil || status.Enabled {
		t.Fatalf("MFA remained enabled: %#v %v", status, err)
	}
}
