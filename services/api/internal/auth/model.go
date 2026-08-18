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
var ErrEmailVerificationRequired = errors.New("email verification required")
var ErrInvalidEmailToken = errors.New("email link is invalid or expired")
var ErrMFARequired = errors.New("multi-factor authentication required")
var ErrMFAUnavailable = errors.New("multi-factor authentication unavailable")
var ErrMFAAlreadyEnabled = errors.New("multi-factor authentication already enabled")
var ErrMFANotEnabled = errors.New("multi-factor authentication is not enabled")
var ErrInvalidMFACode = errors.New("multi-factor authentication code is invalid")
var ErrInvalidMFAChallenge = errors.New("multi-factor authentication challenge is invalid or expired")
var ErrMFAEnrollmentExpired = errors.New("multi-factor authentication enrollment is invalid or expired")

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
	Create(context.Context, string, string, string, string, string) (User, error)
	ByNormalizedEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
	RecordLogin(context.Context, string, time.Time) error
	UpdatePassword(context.Context, string, string, string, time.Time) (bool, error)
}

type EmailTokenPurpose string

const (
	VerifyEmailToken   EmailTokenPurpose = "verify_email"
	ResetPasswordToken EmailTokenPurpose = "reset_password"
)

type EmailTokenStore interface {
	ReplaceEmailToken(context.Context, string, EmailTokenPurpose, []byte, time.Time, time.Time) error
	ActiveEmailTokenUser(context.Context, EmailTokenPurpose, []byte, time.Time) (string, error)
	ConsumeVerificationToken(context.Context, []byte, time.Time) (string, error)
	ConsumePasswordResetToken(context.Context, []byte, string, time.Time) (string, error)
}

type MFAStatus struct {
	Enabled                bool `json:"enabled"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

type TOTPFactor struct {
	SecretCiphertext []byte
	PendingExpiresAt *time.Time
	EnabledAt        *time.Time
}

type MFAStore interface {
	MFAStatus(context.Context, string) (MFAStatus, error)
	SetPendingTOTP(context.Context, string, []byte, time.Time, time.Time) error
	TOTPFactor(context.Context, string) (TOTPFactor, error)
	ActivateTOTP(context.Context, string, [][]byte, int64, time.Time) error
	AdvanceTOTPStep(context.Context, string, int64, time.Time) (bool, error)
	ConsumeRecoveryCode(context.Context, string, []byte, time.Time) (bool, error)
	ReplaceRecoveryCodes(context.Context, string, [][]byte, time.Time) error
	DisableTOTP(context.Context, string) (bool, error)
}

type MFAChallenge struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MFAChallengeStore interface {
	CreateMFAChallenge(context.Context, string, time.Duration) (string, error)
	GetMFAChallenge(context.Context, string) (MFAChallenge, error)
	ConsumeMFAChallenge(context.Context, string) (MFAChallenge, error)
	DeleteMFAChallenge(context.Context, string) error
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
