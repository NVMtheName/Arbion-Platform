package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresEmailTokenLifecycle(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewPostgresStore(pool)
	email := fmt.Sprintf("email-token-%d@example.com", time.Now().UnixNano())
	user, err := store.Create(ctx, email, email, "original-hash", "Token Test", "pending_verification")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, user.ID) }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	verificationHash := sha256.Sum256([]byte("verification-token"))
	if err = store.ReplaceEmailToken(ctx, user.ID, VerifyEmailToken, verificationHash[:], now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	verifiedUserID, err := store.ConsumeVerificationToken(ctx, verificationHash[:], now.Add(time.Second))
	if err != nil || verifiedUserID != user.ID {
		t.Fatalf("verification failed: user=%q error=%v", verifiedUserID, err)
	}
	verified, err := store.ByID(ctx, user.ID)
	if err != nil || verified.Status != "active" || verified.EmailVerifiedAt == nil {
		t.Fatalf("verification did not activate user: user=%#v error=%v", verified, err)
	}
	if _, err = store.ConsumeVerificationToken(ctx, verificationHash[:], now.Add(2*time.Second)); !errors.Is(err, ErrInvalidEmailToken) {
		t.Fatalf("verification token reuse returned %v", err)
	}

	resetHash := sha256.Sum256([]byte("reset-token"))
	if err = store.ReplaceEmailToken(ctx, user.ID, ResetPasswordToken, resetHash[:], now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}
	if found, err := store.ActiveEmailTokenUser(ctx, ResetPasswordToken, resetHash[:], now.Add(time.Second)); err != nil || found != user.ID {
		t.Fatalf("active reset token was not found: user=%q error=%v", found, err)
	}
	resetUserID, err := store.ConsumePasswordResetToken(ctx, resetHash[:], "replacement-hash", now.Add(2*time.Second))
	if err != nil || resetUserID != user.ID {
		t.Fatalf("password reset failed: user=%q error=%v", resetUserID, err)
	}
	resetUser, err := store.ByID(ctx, user.ID)
	if err != nil || resetUser.PasswordHash != "replacement-hash" {
		t.Fatalf("password hash was not replaced: user=%#v error=%v", resetUser, err)
	}
	if _, err = store.ConsumePasswordResetToken(ctx, resetHash[:], "second-hash", now.Add(3*time.Second)); !errors.Is(err, ErrInvalidEmailToken) {
		t.Fatalf("reset token reuse returned %v", err)
	}
}
