package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/mailer"
)

type Service struct {
	users                  UserStore
	sessions               SessionStore
	limiter                RateLimiter
	audit                  Auditor
	hasher                 PasswordHasher
	ttl                    time.Duration
	now                    func() time.Time
	registrationRestricted bool
	registrationAllowlist  map[string]struct{}
	emailTokens            EmailTokenStore
	emailSender            mailer.Sender
	emailPolicy            EmailPolicy
	mfaStore               MFAStore
	mfaChallenges          MFAChallengeStore
	mfaProtector           *MFASecretProtector
}

type RegistrationPolicy struct {
	Restricted    bool
	AllowedEmails []string
}

type EmailPolicy struct {
	VerificationRequired bool
	PublicBaseURL        string
	VerificationTTL      time.Duration
	PasswordResetTTL     time.Duration
}

func NewService(users UserStore, sessions SessionStore, limiter RateLimiter, audit Auditor, ttl time.Duration, policies ...RegistrationPolicy) *Service {
	s := &Service{users: users, sessions: sessions, limiter: limiter, audit: audit, hasher: NewPasswordHasher(), ttl: ttl, now: time.Now, registrationAllowlist: map[string]struct{}{}}
	if len(policies) > 0 {
		s.registrationRestricted = policies[0].Restricted
		for _, email := range policies[0].AllowedEmails {
			s.registrationAllowlist[NormalizeEmail(email)] = struct{}{}
		}
	}
	return s
}

func (s *Service) ConfigureEmail(tokens EmailTokenStore, sender mailer.Sender, policy EmailPolicy) {
	s.emailTokens = tokens
	s.emailSender = sender
	s.emailPolicy = policy
}

func (s *Service) RequiresEmailVerification() bool { return s.emailPolicy.VerificationRequired }
func validEmail(v string) bool {
	a, e := mail.ParseAddress(v)
	return e == nil && a.Address == v && len(v) <= 320
}
func (s *Service) Register(ctx context.Context, email, password, name, rateKey string) (SafeUser, string, error) {
	ok, err := s.limiter.Allow(ctx, "register:"+rateKey, 5, time.Hour)
	if err != nil {
		return SafeUser{}, "", err
	}
	if !ok {
		return SafeUser{}, "", ErrRateLimited
	}
	n := NormalizeEmail(email)
	if _, allowed := s.registrationAllowlist[n]; s.registrationRestricted && !allowed {
		_ = s.audit.Record(ctx, nil, "auth.registration_rejected", map[string]any{"reason": "not_allowlisted"})
		return SafeUser{}, "", ErrRegistrationUnavailable
	}
	if !validEmail(n) || len(name) > 100 {
		return SafeUser{}, "", errors.New("invalid registration")
	}
	h, err := s.hasher.Hash(password)
	if err != nil {
		return SafeUser{}, "", err
	}
	status := "active"
	if s.emailPolicy.VerificationRequired {
		status = "pending_verification"
	}
	u, err := s.users.Create(ctx, email, n, h, name, status)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return SafeUser{}, "", ErrConflict
		}
		return SafeUser{}, "", err
	}
	id := u.ID
	if s.emailPolicy.VerificationRequired {
		delivery := "sent"
		if err = s.sendEmailToken(ctx, u, VerifyEmailToken); err != nil {
			delivery = "failed"
		}
		_ = s.audit.Record(ctx, &id, "auth.registration", map[string]any{"outcome": "verification_pending", "email_delivery": delivery})
		return u.Safe(), "", nil
	}
	token, _, err := s.sessions.Create(ctx, u.ID, s.ttl)
	if err != nil {
		return SafeUser{}, "", err
	}
	_ = s.audit.Record(ctx, &id, "auth.registration", map[string]any{"outcome": "success"})
	return u.Safe(), token, nil
}
func (s *Service) Login(ctx context.Context, email, password, rateKey string) (SafeUser, string, error) {
	result, err := s.BeginLogin(ctx, email, password, rateKey)
	if err != nil {
		return SafeUser{}, "", err
	}
	if result.MFARequired {
		return result.User, "", ErrMFARequired
	}
	return result.User, result.SessionToken, nil
}

func (s *Service) BeginLogin(ctx context.Context, email, password, rateKey string) (LoginResult, error) {
	n := NormalizeEmail(email)
	ok, err := s.limiter.Allow(ctx, "login:"+rateKey+":"+n, 10, 15*time.Minute)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, ErrRateLimited
	}
	u, err := s.users.ByNormalizedEmail(ctx, n)
	if err != nil || !s.hasher.Verify(password, u.PasswordHash) {
		_ = s.audit.Record(ctx, nil, "auth.login_failed", map[string]any{"outcome": "rejected"})
		return LoginResult{}, ErrInvalidCredentials
	}
	if u.Status == "pending_verification" {
		return LoginResult{}, ErrEmailVerificationRequired
	}
	if u.Status != "active" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if s.mfaStore != nil {
		status, statusErr := s.mfaStore.MFAStatus(ctx, u.ID)
		if statusErr != nil {
			return LoginResult{}, statusErr
		}
		if status.Enabled {
			if s.mfaChallenges == nil {
				return LoginResult{}, ErrMFAUnavailable
			}
			challenge, challengeErr := s.mfaChallenges.CreateMFAChallenge(ctx, u.ID, mfaChallengeTTL)
			if challengeErr != nil {
				return LoginResult{}, challengeErr
			}
			_ = s.audit.Record(ctx, &u.ID, "auth.login_mfa_required", map[string]any{"outcome": "challenge_issued"})
			return LoginResult{User: u.Safe(), MFARequired: true, ChallengeToken: challenge}, nil
		}
	}
	now := s.now().UTC()
	if err = s.users.RecordLogin(ctx, u.ID, now); err != nil {
		return LoginResult{}, err
	}
	token, _, err := s.sessions.Create(ctx, u.ID, s.ttl)
	if err != nil {
		return LoginResult{}, err
	}
	id := u.ID
	_ = s.audit.Record(ctx, &id, "auth.login", map[string]any{"outcome": "success"})
	return LoginResult{User: u.Safe(), SessionToken: token}, nil
}

func (s *Service) RequestEmailVerification(ctx context.Context, email, rateKey string) error {
	return s.requestEmailToken(ctx, email, rateKey, VerifyEmailToken)
}

func (s *Service) RequestPasswordReset(ctx context.Context, email, rateKey string) error {
	return s.requestEmailToken(ctx, email, rateKey, ResetPasswordToken)
}

func (s *Service) requestEmailToken(ctx context.Context, email, rateKey string, purpose EmailTokenPurpose) error {
	normalized := NormalizeEmail(email)
	digest := sha256.Sum256([]byte(normalized))
	allowed, err := s.limiter.Allow(ctx, "email_token:"+string(purpose)+":"+rateKey+":"+hex.EncodeToString(digest[:]), 5, time.Hour)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	if !validEmail(normalized) || s.emailTokens == nil || s.emailSender == nil {
		return nil
	}
	user, err := s.users.ByNormalizedEmail(ctx, normalized)
	if err != nil || user.Status == "disabled" || (purpose == VerifyEmailToken && user.EmailVerifiedAt != nil) || (purpose == ResetPasswordToken && user.Status != "active") {
		return nil
	}
	if err = s.sendEmailToken(ctx, user, purpose); err != nil {
		id := user.ID
		_ = s.audit.Record(ctx, &id, "auth.email_delivery_failed", map[string]any{"purpose": string(purpose)})
		return nil
	}
	id := user.ID
	_ = s.audit.Record(ctx, &id, "auth.email_token_requested", map[string]any{"purpose": string(purpose), "outcome": "sent"})
	return nil
}

func (s *Service) sendEmailToken(ctx context.Context, user User, purpose EmailTokenPurpose) error {
	if s.emailTokens == nil || s.emailSender == nil {
		return errors.New("email delivery is not configured")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	ttl, route, subject, intro, action := s.emailPolicy.VerificationTTL, "/verify-email", "Verify your Arbion email", "Verify your email address to activate your Arbion account.", "Verify email"
	if purpose == ResetPasswordToken {
		ttl, route, subject, intro, action = s.emailPolicy.PasswordResetTTL, "/reset-password", "Reset your Arbion password", "A password reset was requested for your Arbion account.", "Reset password"
	}
	if ttl <= 0 {
		return errors.New("email token lifetime is not configured")
	}
	if err := s.emailTokens.ReplaceEmailToken(ctx, user.ID, purpose, hash[:], now.Add(ttl), now); err != nil {
		return err
	}
	link := strings.TrimRight(s.emailPolicy.PublicBaseURL, "/") + route + "#token=" + token
	detail := fmt.Sprintf("This link expires in %s and can be used once. If you did not request this, you can safely ignore this email.", ttl)
	html, err := mailer.RenderBrandedHTML(mailer.BrandedEmailContent{
		Preheader:   subject,
		LogoURL:     strings.TrimRight(s.emailPolicy.PublicBaseURL, "/") + "/brand/arbion-wordmark.svg",
		Heading:     subject,
		Intro:       intro,
		ActionLabel: action,
		ActionURL:   link,
		Detail:      detail,
	})
	if err != nil {
		return err
	}
	message := mailer.Message{To: user.NormalizedEmail, Subject: subject, Text: fmt.Sprintf("%s\n\nOpen this secure link:\n%s\n\n%s\n", intro, link, detail), HTML: html}
	return s.emailSender.Send(ctx, message)
}

func (s *Service) VerifyEmail(ctx context.Context, token, rateKey string) error {
	if s.emailTokens == nil {
		return ErrInvalidEmailToken
	}
	hash, err := emailTokenHash(token)
	if err != nil {
		return err
	}
	allowed, err := s.limiter.Allow(ctx, "email_verify:"+rateKey+":"+hex.EncodeToString(hash), 10, time.Hour)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	userID, err := s.emailTokens.ConsumeVerificationToken(ctx, hash, s.now().UTC())
	if err != nil {
		return err
	}
	_ = s.audit.Record(ctx, &userID, "auth.email_verified", map[string]any{"outcome": "success"})
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword, rateKey string) error {
	if s.emailTokens == nil {
		return ErrInvalidEmailToken
	}
	hash, err := emailTokenHash(token)
	if err != nil {
		return err
	}
	allowed, err := s.limiter.Allow(ctx, "password_reset_confirm:"+rateKey+":"+hex.EncodeToString(hash), 10, time.Hour)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	nextHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	userID, err := s.emailTokens.ActiveEmailTokenUser(ctx, ResetPasswordToken, hash, now)
	if err != nil {
		return err
	}
	if err = s.sessions.RevokeUser(ctx, userID); err != nil {
		return err
	}
	consumedUserID, err := s.emailTokens.ConsumePasswordResetToken(ctx, hash, nextHash, now)
	if err != nil {
		return err
	}
	_ = s.audit.Record(ctx, &consumedUserID, "auth.password_reset", map[string]any{"outcome": "success", "sessions_revoked": true})
	return nil
}

func emailTokenHash(token string) ([]byte, error) {
	if len(token) != 43 {
		return nil, ErrInvalidEmailToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrInvalidEmailToken
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (SafeUser, error) {
	sess, err := s.sessions.Get(ctx, token)
	if err != nil {
		return SafeUser{}, ErrUnauthenticated
	}
	u, err := s.users.ByID(ctx, sess.UserID)
	if err != nil || u.Status != "active" {
		return SafeUser{}, ErrUnauthenticated
	}
	return u.Safe(), nil
}
func (s *Service) Logout(ctx context.Context, token string, userID *string) error {
	if token != "" {
		if err := s.sessions.Delete(ctx, token); err != nil {
			return err
		}
	}
	_ = s.audit.Record(ctx, userID, "auth.logout", map[string]any{"outcome": "success"})
	return nil
}

func (s *Service) LogoutEverywhere(ctx context.Context, userID string) error {
	if err := s.sessions.RevokeUser(ctx, userID); err != nil {
		return err
	}
	_ = s.audit.Record(ctx, &userID, "auth.logout_all", map[string]any{"outcome": "success"})
	return nil
}

func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	ok, err := s.limiter.Allow(ctx, "password_change:"+userID, 5, time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRateLimited
	}
	u, err := s.users.ByID(ctx, userID)
	if err != nil || u.Status != "active" || !s.hasher.Verify(currentPassword, u.PasswordHash) {
		_ = s.audit.Record(ctx, &userID, "auth.password_change_failed", map[string]any{"outcome": "rejected"})
		return ErrInvalidCurrentPassword
	}
	if s.hasher.Verify(newPassword, u.PasswordHash) {
		return ErrPasswordUnchanged
	}
	nextHash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	// Revoke first so a Redis failure cannot leave old sessions valid after the
	// durable password has changed. A later database failure may require login
	// again, but it cannot silently weaken session security.
	if err = s.sessions.RevokeUser(ctx, userID); err != nil {
		return err
	}
	updated, err := s.users.UpdatePassword(ctx, userID, u.PasswordHash, nextHash, s.now().UTC())
	if err != nil {
		return err
	}
	if !updated {
		return ErrInvalidCurrentPassword
	}
	_ = s.audit.Record(ctx, &userID, "auth.password_changed", map[string]any{"outcome": "success", "sessions_revoked": true})
	return nil
}
