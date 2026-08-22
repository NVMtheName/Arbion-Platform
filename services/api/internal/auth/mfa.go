package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- RFC 6238 requires HMAC-SHA-1 interoperability.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	totpDigits           = 6
	totpPeriod           = 30 * time.Second
	totpSecretBytes      = 20
	mfaEnrollmentTTL     = 10 * time.Minute
	mfaChallengeTTL      = 5 * time.Minute
	recoveryCodeCount    = 10
	recoveryCodeRawBytes = 10
)

var noPaddingBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

type LoginResult struct {
	User           SafeUser
	SessionToken   string
	MFARequired    bool
	ChallengeToken string
}

type MFAEnrollment struct {
	Secret     string    `json:"secret"`
	OTPAuthURI string    `json:"otpauth_uri"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type MFASecretProtector struct{ aead cipher.AEAD }

func NewMFASecretProtector(key []byte) (*MFASecretProtector, error) {
	if len(key) != 32 {
		return nil, errors.New("MFA secret protection requires a 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create MFA cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create MFA AEAD: %w", err)
	}
	return &MFASecretProtector{aead: aead}, nil
}

func mfaAAD(userID string) []byte { return []byte("arbion-auth-totp\x00" + userID) }

func (p *MFASecretProtector) Seal(userID string, plaintext []byte) ([]byte, error) {
	if p == nil || p.aead == nil || userID == "" || len(plaintext) == 0 {
		return nil, ErrMFAUnavailable
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return p.aead.Seal(nonce, nonce, plaintext, mfaAAD(userID)), nil
}

func (p *MFASecretProtector) Open(userID string, ciphertext []byte) ([]byte, error) {
	if p == nil || p.aead == nil || userID == "" || len(ciphertext) < p.aead.NonceSize() {
		return nil, ErrMFAUnavailable
	}
	nonceSize := p.aead.NonceSize()
	plaintext, err := p.aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], mfaAAD(userID))
	if err != nil {
		return nil, errors.New("MFA secret authentication failed")
	}
	return plaintext, nil
}

func (s *Service) ConfigureMFA(store MFAStore, challenges MFAChallengeStore, protector *MFASecretProtector) {
	s.mfaStore = store
	s.mfaChallenges = challenges
	s.mfaProtector = protector
}

func (s *Service) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	if s.mfaStore == nil {
		return MFAStatus{}, ErrMFAUnavailable
	}
	return s.mfaStore.MFAStatus(ctx, userID)
}

func (s *Service) BeginTOTPEnrollment(ctx context.Context, userID, currentPassword string) (MFAEnrollment, error) {
	if s.mfaStore == nil || s.mfaProtector == nil {
		return MFAEnrollment{}, ErrMFAUnavailable
	}
	allowed, err := s.limiter.Allow(ctx, "mfa_enroll:"+userID, 5, time.Hour)
	if err != nil {
		return MFAEnrollment{}, err
	}
	if !allowed {
		return MFAEnrollment{}, ErrRateLimited
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil || user.Status != "active" || !s.hasher.Verify(currentPassword, user.PasswordHash) {
		return MFAEnrollment{}, ErrInvalidCurrentPassword
	}
	status, err := s.mfaStore.MFAStatus(ctx, userID)
	if err != nil {
		return MFAEnrollment{}, err
	}
	if status.Enabled {
		return MFAEnrollment{}, ErrMFAAlreadyEnabled
	}
	secret := make([]byte, totpSecretBytes)
	if _, err = io.ReadFull(rand.Reader, secret); err != nil {
		return MFAEnrollment{}, err
	}
	ciphertext, err := s.mfaProtector.Seal(userID, secret)
	if err != nil {
		return MFAEnrollment{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(mfaEnrollmentTTL)
	if err = s.mfaStore.SetPendingTOTP(ctx, userID, ciphertext, expiresAt, now); err != nil {
		return MFAEnrollment{}, err
	}
	encoded := noPaddingBase32.EncodeToString(secret)
	label := "Arbion:" + user.NormalizedEmail
	query := url.Values{"algorithm": {"SHA1"}, "digits": {"6"}, "issuer": {"Arbion"}, "period": {"30"}, "secret": {encoded}}
	uri := (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawQuery: query.Encode()}).String()
	_ = s.audit.Record(ctx, &userID, "auth.mfa_enrollment_started", map[string]any{"factor": "totp"})
	return MFAEnrollment{Secret: encoded, OTPAuthURI: uri, ExpiresAt: expiresAt}, nil
}

func (s *Service) ConfirmTOTPEnrollment(ctx context.Context, userID, code string) ([]string, error) {
	if s.mfaStore == nil || s.mfaProtector == nil {
		return nil, ErrMFAUnavailable
	}
	allowed, err := s.limiter.Allow(ctx, "mfa_enroll_confirm:"+userID, 10, mfaEnrollmentTTL)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrRateLimited
	}
	factor, err := s.mfaStore.TOTPFactor(ctx, userID)
	now := s.now().UTC()
	if err != nil || factor.EnabledAt != nil || factor.PendingExpiresAt == nil || !factor.PendingExpiresAt.After(now) {
		return nil, ErrMFAEnrollmentExpired
	}
	secret, err := s.mfaProtector.Open(userID, factor.SecretCiphertext)
	if err != nil {
		return nil, err
	}
	initialStep, ok := matchingTOTPStep(secret, code, now)
	if !ok {
		return nil, ErrInvalidMFACode
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	// Revoke first so activation can never leave a password-only session alive.
	if err = s.sessions.RevokeUser(ctx, userID); err != nil {
		return nil, err
	}
	if err = s.mfaStore.ActivateTOTP(ctx, userID, hashes, initialStep, now); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, &userID, "auth.mfa_enabled", map[string]any{"factor": "totp", "sessions_revoked": true, "recovery_code_count": len(codes)})
	return codes, nil
}

func (s *Service) DisableTOTP(ctx context.Context, userID, currentPassword, code string) error {
	if s.mfaStore == nil || s.mfaProtector == nil {
		return ErrMFAUnavailable
	}
	allowed, err := s.limiter.Allow(ctx, "mfa_disable:"+userID, 5, time.Hour)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil || user.Status != "active" || !s.hasher.Verify(currentPassword, user.PasswordHash) {
		return ErrInvalidCurrentPassword
	}
	factor, err := s.mfaStore.TOTPFactor(ctx, userID)
	if err != nil || factor.EnabledAt == nil {
		return ErrMFANotEnabled
	}
	if _, err = s.verifySecondFactor(ctx, userID, factor, code, s.now().UTC()); err != nil {
		return err
	}
	if err = s.sessions.RevokeUser(ctx, userID); err != nil {
		return err
	}
	disabled, err := s.mfaStore.DisableTOTP(ctx, userID)
	if err != nil {
		return err
	}
	if !disabled {
		return ErrMFANotEnabled
	}
	_ = s.audit.Record(ctx, &userID, "auth.mfa_disabled", map[string]any{"factor": "totp", "sessions_revoked": true})
	return nil
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, currentPassword, code string) ([]string, error) {
	if s.mfaStore == nil || s.mfaProtector == nil {
		return nil, ErrMFAUnavailable
	}
	allowed, err := s.limiter.Allow(ctx, "mfa_recovery_regenerate:"+userID, 5, time.Hour)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrRateLimited
	}
	user, err := s.users.ByID(ctx, userID)
	if err != nil || user.Status != "active" || !s.hasher.Verify(currentPassword, user.PasswordHash) {
		return nil, ErrInvalidCurrentPassword
	}
	factor, err := s.mfaStore.TOTPFactor(ctx, userID)
	if err != nil || factor.EnabledAt == nil {
		return nil, ErrMFANotEnabled
	}
	if _, err = s.verifySecondFactor(ctx, userID, factor, code, s.now().UTC()); err != nil {
		return nil, err
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err = s.mfaStore.ReplaceRecoveryCodes(ctx, userID, hashes, s.now().UTC()); err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, &userID, "auth.mfa_recovery_codes_replaced", map[string]any{"factor": "totp", "recovery_code_count": len(codes)})
	return codes, nil
}

func (s *Service) CompleteMFALogin(ctx context.Context, challengeToken, code, _ string) (SafeUser, string, error) {
	if s.mfaStore == nil || s.mfaChallenges == nil || s.mfaProtector == nil {
		return SafeUser{}, "", ErrMFAUnavailable
	}
	digest := sha256.Sum256([]byte(challengeToken))
	allowed, err := s.limiter.Allow(ctx, "mfa_login:"+base64.RawURLEncoding.EncodeToString(digest[:]), 8, 10*time.Minute)
	if err != nil {
		return SafeUser{}, "", err
	}
	if !allowed {
		_ = s.mfaChallenges.DeleteMFAChallenge(ctx, challengeToken)
		return SafeUser{}, "", ErrRateLimited
	}
	challenge, err := s.mfaChallenges.GetMFAChallenge(ctx, challengeToken)
	if err != nil || !s.now().Before(challenge.ExpiresAt) {
		return SafeUser{}, "", ErrInvalidMFAChallenge
	}
	user, err := s.users.ByID(ctx, challenge.UserID)
	if err != nil || user.Status != "active" {
		return SafeUser{}, "", ErrInvalidMFAChallenge
	}
	factor, err := s.mfaStore.TOTPFactor(ctx, user.ID)
	if err != nil || factor.EnabledAt == nil {
		return SafeUser{}, "", ErrInvalidMFAChallenge
	}
	method, err := s.verifySecondFactor(ctx, user.ID, factor, code, s.now().UTC())
	if err != nil {
		_ = s.audit.Record(ctx, &user.ID, "auth.mfa_login_failed", map[string]any{"outcome": "rejected"})
		return SafeUser{}, "", err
	}
	// Atomically consume the challenge before creating a durable authenticated
	// session. Durable TOTP/recovery-code consumption prevents concurrent factor
	// replay; this prevents different valid recovery codes from sharing one
	// password challenge.
	consumedChallenge, err := s.mfaChallenges.ConsumeMFAChallenge(ctx, challengeToken)
	if err != nil || consumedChallenge.UserID != user.ID {
		return SafeUser{}, "", ErrInvalidMFAChallenge
	}
	token, _, err := s.sessions.Create(ctx, user.ID, s.ttl)
	if err != nil {
		return SafeUser{}, "", err
	}
	if err = s.users.RecordLogin(ctx, user.ID, s.now().UTC()); err != nil {
		_ = s.sessions.Delete(ctx, token)
		return SafeUser{}, "", err
	}
	_ = s.audit.Record(ctx, &user.ID, "auth.login", map[string]any{"outcome": "success", "mfa": method})
	return user.Safe(), token, nil
}

// VerifyOrderIntentStepUp consumes one fresh authenticator step for a single
// order-intent review. Recovery codes are deliberately excluded from this
// higher-risk operation and no reusable step-up token is issued.
func (s *Service) VerifyOrderIntentStepUp(ctx context.Context, userID, code string) (string, time.Time, error) {
	if s.mfaStore == nil || s.mfaProtector == nil || userID == "" {
		return "", time.Time{}, ErrMFAUnavailable
	}
	allowed, err := s.limiter.Allow(ctx, "order_intent_step_up:"+userID, 8, 10*time.Minute)
	if err != nil {
		return "", time.Time{}, err
	}
	if !allowed {
		return "", time.Time{}, ErrRateLimited
	}
	factor, err := s.mfaStore.TOTPFactor(ctx, userID)
	if err != nil || factor.EnabledAt == nil {
		return "", time.Time{}, ErrMFANotEnabled
	}
	now := s.now().UTC()
	step, valid := matchingTOTPStepFromCiphertext(s.mfaProtector, userID, factor.SecretCiphertext, code, now)
	if !valid {
		_ = s.audit.Record(ctx, &userID, "auth.order_intent_step_up_failed", map[string]any{"outcome": "rejected"})
		return "", time.Time{}, ErrInvalidMFACode
	}
	advanced, err := s.mfaStore.AdvanceTOTPStep(ctx, userID, step, now)
	if err != nil {
		return "", time.Time{}, err
	}
	if !advanced {
		return "", time.Time{}, ErrInvalidMFACode
	}
	_ = s.audit.Record(ctx, &userID, "auth.order_intent_step_up_verified", map[string]any{"factor": "totp"})
	return "totp", now, nil
}

func (s *Service) verifySecondFactor(ctx context.Context, userID string, factor TOTPFactor, code string, now time.Time) (string, error) {
	if step, ok := matchingTOTPStepFromCiphertext(s.mfaProtector, userID, factor.SecretCiphertext, code, now); ok {
		advanced, err := s.mfaStore.AdvanceTOTPStep(ctx, userID, step, now)
		if err != nil {
			return "", err
		}
		if !advanced {
			return "", ErrInvalidMFACode
		}
		return "totp", nil
	}
	hash, ok := recoveryCodeHash(code)
	if !ok {
		return "", ErrInvalidMFACode
	}
	consumed, err := s.mfaStore.ConsumeRecoveryCode(ctx, userID, hash, now)
	if err != nil {
		return "", err
	}
	if !consumed {
		return "", ErrInvalidMFACode
	}
	return "recovery_code", nil
}

func matchingTOTPStepFromCiphertext(protector *MFASecretProtector, userID string, ciphertext []byte, code string, now time.Time) (int64, bool) {
	secret, err := protector.Open(userID, ciphertext)
	if err != nil {
		return 0, false
	}
	return matchingTOTPStep(secret, code, now)
}

func matchingTOTPStep(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	current := now.Unix() / int64(totpPeriod/time.Second)
	for _, step := range []int64{current, current - 1, current + 1} {
		if step >= 0 && subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func totpCode(secret []byte, step int64) string {
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(step))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func generateRecoveryCodes() ([]string, [][]byte, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([][]byte, 0, recoveryCodeCount)
	seen := map[string]struct{}{}
	for len(codes) < recoveryCodeCount {
		raw := make([]byte, recoveryCodeRawBytes)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return nil, nil, err
		}
		normalized := noPaddingBase32.EncodeToString(raw)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		hash := sha256.Sum256([]byte("arbion-mfa-recovery\x00" + normalized))
		codes = append(codes, strings.Join([]string{normalized[0:4], normalized[4:8], normalized[8:12], normalized[12:16]}, "-"))
		hashes = append(hashes, hash[:])
	}
	return codes, hashes, nil
}

func recoveryCodeHash(code string) ([]byte, bool) {
	normalized := strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, code)
	if len(normalized) != 16 {
		return nil, false
	}
	decoded, err := noPaddingBase32.DecodeString(normalized)
	if err != nil || len(decoded) != recoveryCodeRawBytes {
		return nil, false
	}
	hash := sha256.Sum256([]byte("arbion-mfa-recovery\x00" + normalized))
	return hash[:], true
}
