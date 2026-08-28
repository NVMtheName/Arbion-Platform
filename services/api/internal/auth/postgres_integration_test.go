package auth

import (
	"bytes"
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

func TestPostgresTOTPMFALifecycle(t *testing.T) {
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
	email := fmt.Sprintf("mfa-%d@example.com", time.Now().UnixNano())
	user, err := store.Create(ctx, email, email, "password-hash", "MFA Test", "active")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, user.ID) }()

	now := time.Now().UTC().Truncate(time.Microsecond)
	ciphertext := make([]byte, 48)
	if err = store.SetPendingTOTP(ctx, user.ID, ciphertext, now.Add(10*time.Minute), now); err != nil {
		t.Fatal(err)
	}
	factor, err := store.TOTPFactor(ctx, user.ID)
	if err != nil || factor.EnabledAt != nil || factor.PendingExpiresAt == nil || !bytes.Equal(factor.SecretCiphertext, ciphertext) {
		t.Fatalf("pending factor mismatch: %#v %v", factor, err)
	}
	firstHash := sha256.Sum256([]byte("first recovery code"))
	secondHash := sha256.Sum256([]byte("second recovery code"))
	if err = store.ActivateTOTP(ctx, user.ID, [][]byte{firstHash[:], secondHash[:]}, 100, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	status, err := store.MFAStatus(ctx, user.ID)
	if err != nil || !status.Enabled || status.RecoveryCodesRemaining != 2 {
		t.Fatalf("active factor status mismatch: %#v %v", status, err)
	}
	if advanced, err := store.AdvanceTOTPStep(ctx, user.ID, 100, now.Add(2*time.Second)); err != nil || advanced {
		t.Fatalf("replayed TOTP step advanced: %v %v", advanced, err)
	}
	if advanced, err := store.AdvanceTOTPStep(ctx, user.ID, 101, now.Add(2*time.Second)); err != nil || !advanced {
		t.Fatalf("new TOTP step did not advance: %v %v", advanced, err)
	}
	if consumed, err := store.ConsumeRecoveryCode(ctx, user.ID, firstHash[:], now.Add(3*time.Second)); err != nil || !consumed {
		t.Fatalf("recovery code was not consumed: %v %v", consumed, err)
	}
	if consumed, err := store.ConsumeRecoveryCode(ctx, user.ID, firstHash[:], now.Add(4*time.Second)); err != nil || consumed {
		t.Fatalf("recovery code replay was accepted: %v %v", consumed, err)
	}
	status, err = store.MFAStatus(ctx, user.ID)
	if err != nil || status.RecoveryCodesRemaining != 1 {
		t.Fatalf("recovery count mismatch: %#v %v", status, err)
	}
	thirdHash := sha256.Sum256([]byte("replacement recovery code"))
	if err = store.ReplaceRecoveryCodes(ctx, user.ID, [][]byte{thirdHash[:]}, now.Add(5*time.Second)); err != nil {
		t.Fatalf("recovery replacement failed: %v", err)
	}
	if consumed, err := store.ConsumeRecoveryCode(ctx, user.ID, secondHash[:], now.Add(6*time.Second)); err != nil || consumed {
		t.Fatalf("replaced recovery code remained active: %v %v", consumed, err)
	}
	if consumed, err := store.ConsumeRecoveryCode(ctx, user.ID, thirdHash[:], now.Add(6*time.Second)); err != nil || !consumed {
		t.Fatalf("replacement recovery code was unavailable: %v %v", consumed, err)
	}
	if disabled, err := store.DisableTOTP(ctx, user.ID); err != nil || !disabled {
		t.Fatalf("factor was not disabled: %v %v", disabled, err)
	}
	status, err = store.MFAStatus(ctx, user.ID)
	if err != nil || status.Enabled || status.RecoveryCodesRemaining != 0 {
		t.Fatalf("disabled factor remained: %#v %v", status, err)
	}
}

func TestPostgresSecurityActivityIsOwnerScopedFilteredAndAppendOnly(t *testing.T) {
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
	suffix := time.Now().UnixNano()
	ownerEmail := fmt.Sprintf("security-owner-%d@example.com", suffix)
	owner, err := store.Create(ctx, ownerEmail, ownerEmail, "password-hash", "Security Owner", "active")
	if err != nil {
		t.Fatal(err)
	}
	foreignEmail := fmt.Sprintf("security-foreign-%d@example.com", suffix)
	foreign, err := store.Create(ctx, foreignEmail, foreignEmail, "password-hash", "Security Foreign", "active")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	insert := func(userID, action string, occurredAt time.Time) string {
		var id string
		if insertErr := pool.QueryRow(ctx, `INSERT INTO audit_events(user_id,actor_type,actor_id,action,target_type,target_id,occurred_at,metadata) VALUES($1,'user',$2,$3,'authentication',$2,$4,'{}') RETURNING id::text`, userID, userID, action, occurredAt).Scan(&id); insertErr != nil {
			t.Fatal(insertErr)
		}
		return id
	}
	newestID := insert(owner.ID, "auth.login", now)
	secondID := insert(owner.ID, "auth.mfa_enabled", now.Add(-time.Minute))
	thirdID := insert(owner.ID, "financial.connection_disabled", now.Add(-2*time.Minute))
	insert(owner.ID, "neural_shadow_decision.completed", now.Add(time.Minute))
	insert(foreign.ID, "auth.login", now.Add(2*time.Minute))

	firstPage, err := store.SecurityActivities(ctx, owner.ID, 2, nil)
	if err != nil || len(firstPage) != 2 || firstPage[0].ID != newestID || firstPage[1].ID != secondID {
		t.Fatalf("owner security activity projection was not bounded and filtered: %#v %v", firstPage, err)
	}
	secondPage, err := store.SecurityActivities(ctx, owner.ID, 2, &SecurityActivityCursor{OccurredAt: firstPage[1].OccurredAt, ID: firstPage[1].ID})
	if err != nil || len(secondPage) != 1 || secondPage[0].ID != thirdID {
		t.Fatalf("security activity cursor was unstable: %#v %v", secondPage, err)
	}
	foreignPage, err := store.SecurityActivities(ctx, foreign.ID, 10, nil)
	if err != nil || len(foreignPage) != 1 {
		t.Fatalf("foreign security activity projection failed: %#v %v", foreignPage, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE audit_events SET action='auth.logout' WHERE id=$1`, newestID); err == nil {
		t.Fatal("append-only security activity was updated")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM audit_events WHERE id=$1`, newestID); err == nil {
		t.Fatal("append-only security activity was deleted")
	}
}
