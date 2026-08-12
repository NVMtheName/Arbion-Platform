package auth

import (
	"context"
	"errors"
	"net/mail"
	"time"
)

type Service struct {
	users    UserStore
	sessions SessionStore
	limiter  RateLimiter
	audit    Auditor
	hasher   PasswordHasher
	ttl      time.Duration
	now      func() time.Time
}

func NewService(users UserStore, sessions SessionStore, limiter RateLimiter, audit Auditor, ttl time.Duration) *Service {
	return &Service{users: users, sessions: sessions, limiter: limiter, audit: audit, hasher: NewPasswordHasher(), ttl: ttl, now: time.Now}
}
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
	if !validEmail(n) || len(name) > 100 {
		return SafeUser{}, "", errors.New("invalid registration")
	}
	h, err := s.hasher.Hash(password)
	if err != nil {
		return SafeUser{}, "", err
	}
	u, err := s.users.Create(ctx, email, n, h, name)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return SafeUser{}, "", ErrConflict
		}
		return SafeUser{}, "", err
	}
	token, _, err := s.sessions.Create(ctx, u.ID, s.ttl)
	if err != nil {
		return SafeUser{}, "", err
	}
	id := u.ID
	_ = s.audit.Record(ctx, &id, "auth.registration", map[string]any{"outcome": "success"})
	return u.Safe(), token, nil
}
func (s *Service) Login(ctx context.Context, email, password, rateKey string) (SafeUser, string, error) {
	n := NormalizeEmail(email)
	ok, err := s.limiter.Allow(ctx, "login:"+rateKey+":"+n, 10, 15*time.Minute)
	if err != nil {
		return SafeUser{}, "", err
	}
	if !ok {
		return SafeUser{}, "", ErrRateLimited
	}
	u, err := s.users.ByNormalizedEmail(ctx, n)
	if err != nil || u.Status != "active" || !s.hasher.Verify(password, u.PasswordHash) {
		_ = s.audit.Record(ctx, nil, "auth.login_failed", map[string]any{"outcome": "rejected"})
		return SafeUser{}, "", ErrInvalidCredentials
	}
	now := s.now().UTC()
	if err = s.users.RecordLogin(ctx, u.ID, now); err != nil {
		return SafeUser{}, "", err
	}
	token, _, err := s.sessions.Create(ctx, u.ID, s.ttl)
	if err != nil {
		return SafeUser{}, "", err
	}
	id := u.ID
	_ = s.audit.Record(ctx, &id, "auth.login", map[string]any{"outcome": "success"})
	return u.Safe(), token, nil
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
