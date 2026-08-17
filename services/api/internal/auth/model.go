package auth

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrConflict = errors.New("account already exists")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInvalidCurrentPassword = errors.New("current password is incorrect")
var ErrPasswordUnchanged = errors.New("new password must be different")
var ErrUnauthenticated = errors.New("authentication required")
var ErrRateLimited = errors.New("too many attempts")
var ErrRegistrationUnavailable = errors.New("registration unavailable")

type User struct {
	ID              string
	Email           string
	NormalizedEmail string
	PasswordHash    string
	DisplayName     string
	Status          string
	EmailVerifiedAt *time.Time
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	Role            string
	Entitlement     string
	BillingRequired bool
}

type SafeUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
	Role          string `json:"role"`
	Entitlement   string `json:"entitlement"`
}

func (u User) Safe() SafeUser {
	return SafeUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Status: u.Status, EmailVerified: u.EmailVerifiedAt != nil, Role: u.Role, Entitlement: u.Entitlement}
}
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

type UserStore interface {
	Create(context.Context, string, string, string, string) (User, error)
	ByNormalizedEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
	RecordLogin(context.Context, string, time.Time) error
	UpdatePassword(context.Context, string, string, string, time.Time) (bool, error)
}
type Session struct {
	UserID         string    `json:"user_id"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}
type SessionStore interface {
	Create(context.Context, string, time.Duration) (string, Session, error)
	Get(context.Context, string) (Session, error)
	Delete(context.Context, string) error
	RevokeUser(context.Context, string) error
}
type RateLimiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
}
type Auditor interface {
	Record(context.Context, *string, string, map[string]any) error
}
