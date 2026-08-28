package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/arbion/platform/services/api/internal/mailer"
	"github.com/redis/go-redis/v9"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h := NewPasswordHasher()
	encoded, err := h.Hash("a sufficiently long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == "a sufficiently long passphrase" || !h.Verify("a sufficiently long passphrase", encoded) || h.Verify("wrong password", encoded) {
		t.Fatal("password hashing contract failed")
	}
	if _, err = h.Hash("short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatal("weak password accepted")
	}
	if _, err = h.Hash(string(make([]byte, MaxPasswordBytes+1))); !errors.Is(err, ErrInvalidPassword) {
		t.Fatal("oversize password accepted")
	}
}
func TestRedisSessionsExpiryAndRevocation(t *testing.T) {
	m := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: m.Addr()})
	s := NewRedisStore(c)
	token, session, err := s.Create(context.Background(), "user-1", time.Hour)
	if err != nil || token == "" || session.UserID != "user-1" {
		t.Fatal("session creation failed")
	}
	if _, err = s.Get(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	m.FastForward(2 * time.Hour)
	if _, err = s.Get(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired session accepted")
	}
	a, _, _ := s.Create(context.Background(), "u", time.Hour)
	b, _, _ := s.Create(context.Background(), "u", time.Hour)
	challenge, _ := s.CreateMFAChallenge(context.Background(), "u", 5*time.Minute)
	if err = s.RevokeUser(context.Background(), "u"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{a, b} {
		if _, err = s.Get(context.Background(), v); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("session not revoked")
		}
	}
	if _, err = s.GetMFAChallenge(context.Background(), challenge); !errors.Is(err, ErrInvalidMFAChallenge) {
		t.Fatal("session revocation retained an MFA login challenge")
	}
}

func TestSessionInventoryIsBoundedMetadataFreeAndRevokesOnlyOtherSessions(t *testing.T) {
	m := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: m.Addr()})
	sessions := NewRedisStore(client)
	now := time.Date(2026, 8, 28, 2, 30, 0, 0, time.UTC)
	sessions.now = func() time.Time { return now }
	currentToken, current, err := sessions.Create(context.Background(), "owner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	otherToken, _, err := sessions.Create(context.Background(), "owner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	foreignToken, _, err := sessions.Create(context.Background(), "foreign", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.SAdd(context.Background(), sessions.prefix+"user_sessions:owner", sessions.prefix+"session:stale-reference").Err(); err != nil {
		t.Fatal(err)
	}

	audit := &fakeAudit{}
	service := NewService(nil, sessions, sessions, audit, time.Hour)
	inventory, err := service.SessionInventory(context.Background(), "owner", currentToken)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.ActiveCount != 2 || inventory.OtherCount != 1 || !inventory.Current.CreatedAt.Equal(current.CreatedAt) || !inventory.Current.ExpiresAt.Equal(current.ExpiresAt) {
		t.Fatalf("unexpected session inventory: %#v", inventory)
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{currentToken, tokenKey(currentToken), "user_id", "last_activity_at", "address", "user_agent", "device"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("session inventory exposed prohibited data %q: %s", prohibited, encoded)
		}
	}
	if client.SIsMember(context.Background(), sessions.prefix+"user_sessions:owner", sessions.prefix+"session:stale-reference").Val() {
		t.Fatal("session inventory retained a stale index reference")
	}
	if err = client.SAdd(context.Background(), sessions.prefix+"user_sessions:owner", sessions.prefix+"session:"+tokenKey(foreignToken)).Err(); err != nil {
		t.Fatal(err)
	}

	revoked, err := service.LogoutOtherSessions(context.Background(), "owner", currentToken)
	if err != nil || revoked != 1 {
		t.Fatalf("other-session revocation returned count=%d error=%v", revoked, err)
	}
	if _, err = sessions.Get(context.Background(), currentToken); err != nil {
		t.Fatal("other-session revocation removed the current session")
	}
	if _, err = sessions.Get(context.Background(), otherToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("other-session revocation retained an owner session")
	}
	if _, err = sessions.Get(context.Background(), foreignToken); err != nil {
		t.Fatal("other-session revocation crossed the owner boundary")
	}
	if audit.actions[len(audit.actions)-1] != "auth.logout_others" {
		t.Fatalf("other-session revocation was not audited: %#v", audit.actions)
	}
	updated, err := service.SessionInventory(context.Background(), "owner", currentToken)
	if err != nil || updated.ActiveCount != 1 || updated.OtherCount != 0 {
		t.Fatalf("session inventory did not converge after revocation: %#v %v", updated, err)
	}
}
func TestRedisRateLimit(t *testing.T) {
	m := miniredis.RunT(t)
	s := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	for i := 0; i < 2; i++ {
		ok, e := s.Allow(context.Background(), "key", 2, time.Minute)
		if e != nil || !ok {
			t.Fatal("unexpected throttle")
		}
	}
	ok, _ := s.Allow(context.Background(), "key", 2, time.Minute)
	if ok {
		t.Fatal("expected throttle")
	}
}

func TestRedisWeightedRateLimitIsAtomic(t *testing.T) {
	m := miniredis.RunT(t)
	s := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))

	if ok, err := s.AllowWeighted(context.Background(), "weighted", 7, 12, time.Hour); err != nil || !ok {
		t.Fatalf("initial weighted request failed: %v", err)
	}
	if ok, err := s.AllowWeighted(context.Background(), "weighted", 6, 12, time.Hour); err != nil || ok {
		t.Fatalf("over-budget request was not rejected atomically: allowed=%v error=%v", ok, err)
	}
	if ok, err := s.AllowWeighted(context.Background(), "weighted", 5, 12, time.Hour); err != nil || !ok {
		t.Fatalf("rejected request consumed partial credits: allowed=%v error=%v", ok, err)
	}
	if ok, err := s.AllowWeighted(context.Background(), "weighted", 1, 12, time.Hour); err != nil || ok {
		t.Fatalf("credit limit was not enforced: allowed=%v error=%v", ok, err)
	}
}

type fakeUsers struct {
	user                                                  User
	connectionExists, automationExists, automationEnabled bool
}

func (f *fakeUsers) Create(_ context.Context, email, n, h, name, status string) (User, error) {
	if f.user.ID != "" {
		return User{}, ErrConflict
	}
	f.user = User{ID: "u1", Email: email, NormalizedEmail: n, PasswordHash: h, DisplayName: name, Status: status}
	return f.user, nil
}
func (f *fakeUsers) ByNormalizedEmail(_ context.Context, n string) (User, error) {
	if f.user.NormalizedEmail != n {
		return User{}, errors.New("not found")
	}
	return f.user, nil
}
func (f *fakeUsers) ByID(_ context.Context, id string) (User, error) {
	if f.user.ID != id {
		return User{}, errors.New("not found")
	}
	return f.user, nil
}
func (f *fakeUsers) RecordLogin(context.Context, string, time.Time) error { return nil }
func (f *fakeUsers) UpdatePassword(_ context.Context, id, currentHash, nextHash string, _ time.Time) (bool, error) {
	if f.user.ID != id || f.user.PasswordHash != currentHash {
		return false, nil
	}
	f.user.PasswordHash = nextHash
	return true, nil
}

type fakeAudit struct{ actions []string }

func (a *fakeAudit) Record(_ context.Context, _ *string, action string, _ map[string]any) error {
	a.actions = append(a.actions, action)
	return nil
}

type fakeEmailTokens struct {
	users    *fakeUsers
	userID   string
	purpose  EmailTokenPurpose
	hash     []byte
	expires  time.Time
	consumed bool
}

func (f *fakeEmailTokens) ReplaceEmailToken(_ context.Context, userID string, purpose EmailTokenPurpose, hash []byte, expires, _ time.Time) error {
	f.userID, f.purpose, f.hash, f.expires, f.consumed = userID, purpose, append([]byte(nil), hash...), expires, false
	return nil
}
func (f *fakeEmailTokens) ActiveEmailTokenUser(_ context.Context, purpose EmailTokenPurpose, hash []byte, now time.Time) (string, error) {
	if f.consumed || f.purpose != purpose || !bytes.Equal(f.hash, hash) || !f.expires.After(now) {
		return "", ErrInvalidEmailToken
	}
	return f.userID, nil
}
func (f *fakeEmailTokens) ConsumeVerificationToken(ctx context.Context, hash []byte, now time.Time) (string, error) {
	userID, err := f.ActiveEmailTokenUser(ctx, VerifyEmailToken, hash, now)
	if err != nil {
		return "", err
	}
	f.consumed = true
	f.users.user.Status = "active"
	f.users.user.EmailVerifiedAt = &now
	return userID, nil
}
func (f *fakeEmailTokens) ConsumePasswordResetToken(ctx context.Context, hash []byte, passwordHash string, now time.Time) (string, error) {
	userID, err := f.ActiveEmailTokenUser(ctx, ResetPasswordToken, hash, now)
	if err != nil {
		return "", err
	}
	f.consumed = true
	f.users.user.PasswordHash = passwordHash
	return userID, nil
}

type fakeSender struct{ messages []mailer.Message }

func (f *fakeSender) Send(_ context.Context, message mailer.Message) error {
	f.messages = append(f.messages, message)
	return nil
}

func tokenFromMessage(t *testing.T, message mailer.Message) string {
	t.Helper()
	const marker = "#token="
	index := strings.Index(message.Text, marker)
	if index < 0 {
		t.Fatalf("message did not use a URL fragment: %q", message.Text)
	}
	token := strings.Fields(message.Text[index+len(marker):])[0]
	if len(token) != 43 {
		t.Fatalf("unexpected token length %d", len(token))
	}
	return token
}
func TestRegistrationLoginLogoutPreservesDurableResources(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{connectionExists: true, automationExists: true, automationEnabled: true}
	audit := &fakeAudit{}
	svc := NewService(users, sessions, sessions, audit, time.Hour)
	u, registeredToken, err := svc.Register(context.Background(), "Person@Example.com", "correct horse battery staple", "Person", "ip")
	if err != nil || u.Email != "Person@Example.com" || users.user.PasswordHash == "correct horse battery staple" {
		t.Fatal("registration failed")
	}
	if _, _, err = svc.Register(context.Background(), "person@example.com", "another long password", "", "ip2"); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate normalized email accepted")
	}
	if _, _, err = svc.Login(context.Background(), "person@example.com", "wrong password", "ip"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("invalid login accepted")
	}
	_, loginToken, err := svc.Login(context.Background(), "PERSON@example.com", "correct horse battery staple", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Logout(context.Background(), loginToken, &u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.Get(context.Background(), loginToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("logout retained session")
	}
	if _, err = sessions.Get(context.Background(), registeredToken); err != nil {
		t.Fatal("logout removed a different browser session")
	}
	if !users.connectionExists || !users.automationExists || !users.automationEnabled {
		t.Fatal("logout modified persistent integration or automation state")
	}
	for _, want := range []string{"auth.registration", "auth.login_failed", "auth.login", "auth.logout"} {
		found := false
		for _, got := range audit.actions {
			found = found || got == want
		}
		if !found {
			t.Fatalf("missing audit %s", want)
		}
	}
}

func TestRegistrationAllowlistDefaultsClosedWhenRestricted(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{}
	audit := &fakeAudit{}
	svc := NewService(users, sessions, sessions, audit, time.Hour, RegistrationPolicy{Restricted: true, AllowedEmails: []string{"Founder@Example.com"}})

	if _, _, err := svc.Register(context.Background(), "outsider@example.com", "correct horse battery staple", "", "ip-1"); !errors.Is(err, ErrRegistrationUnavailable) {
		t.Fatalf("non-allowlisted registration returned %v", err)
	}
	if users.user.ID != "" {
		t.Fatal("non-allowlisted account was created")
	}
	if _, _, err := svc.Register(context.Background(), " FOUNDER@example.com ", "correct horse battery staple", "Founder", "ip-2"); err != nil {
		t.Fatalf("normalized allowlisted registration failed: %v", err)
	}
	if len(audit.actions) < 2 || audit.actions[0] != "auth.registration_rejected" || audit.actions[1] != "auth.registration" {
		t.Fatalf("unexpected registration audit actions: %#v", audit.actions)
	}
}

func TestPasswordChangeVerifiesCurrentPasswordAndRevokesEverySession(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{}
	audit := &fakeAudit{}
	svc := NewService(users, sessions, sessions, audit, time.Hour)

	_, registeredToken, err := svc.Register(context.Background(), "person@example.com", "correct horse battery staple", "Person", "ip")
	if err != nil {
		t.Fatal(err)
	}
	_, secondToken, err := svc.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ChangePassword(context.Background(), users.user.ID, "wrong current password", "a different secure passphrase"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("wrong current password returned %v", err)
	}
	if _, err = sessions.Get(context.Background(), registeredToken); err != nil {
		t.Fatal("failed password change revoked an existing session")
	}
	if err = svc.ChangePassword(context.Background(), users.user.ID, "correct horse battery staple", "correct horse battery staple"); !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("unchanged password returned %v", err)
	}
	if err = svc.ChangePassword(context.Background(), users.user.ID, "correct horse battery staple", "a different secure passphrase"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{registeredToken, secondToken} {
		if _, err = sessions.Get(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("password change retained an existing session")
		}
	}
	if _, _, err = svc.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip-2"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password remained valid")
	}
	if _, _, err = svc.Login(context.Background(), "person@example.com", "a different secure passphrase", "ip-2"); err != nil {
		t.Fatalf("new password was not usable: %v", err)
	}
	for _, want := range []string{"auth.password_change_failed", "auth.password_changed"} {
		found := false
		for _, got := range audit.actions {
			found = found || got == want
		}
		if !found {
			t.Fatalf("missing audit %s", want)
		}
	}
}

func TestLogoutEverywhereRevokesEverySession(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{}
	audit := &fakeAudit{}
	svc := NewService(users, sessions, sessions, audit, time.Hour)

	_, first, err := svc.Register(context.Background(), "person@example.com", "correct horse battery staple", "Person", "ip")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := svc.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.LogoutEverywhere(context.Background(), users.user.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if _, err = sessions.Get(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("logout everywhere retained a session")
		}
	}
	if audit.actions[len(audit.actions)-1] != "auth.logout_all" {
		t.Fatalf("logout everywhere was not audited: %#v", audit.actions)
	}
}
func TestSafeSerializationOmitsSecrets(t *testing.T) {
	u := User{ID: "1", Email: "a@b.co", PasswordHash: "secret", NormalizedEmail: "a@b.co", Status: "active"}.Safe()
	if u.Email != "a@b.co" {
		t.Fatal("unsafe conversion")
	}
}

func TestRequiredEmailVerificationCreatesNoSessionAndUsesHashedSingleUseToken(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{}
	tokens := &fakeEmailTokens{users: users}
	sender := &fakeSender{}
	service := NewService(users, sessions, sessions, &fakeAudit{}, time.Hour)
	service.ConfigureEmail(tokens, sender, EmailPolicy{VerificationRequired: true, PublicBaseURL: "https://www.arbion.ai", VerificationTTL: 24 * time.Hour, PasswordResetTTL: 30 * time.Minute})

	user, sessionToken, err := service.Register(context.Background(), "Person@Example.com", "correct horse battery staple", "Person", "ip")
	if err != nil || sessionToken != "" || user.Status != "pending_verification" || len(sender.messages) != 1 {
		t.Fatalf("pending registration failed: user=%#v token=%q messages=%d error=%v", user, sessionToken, len(sender.messages), err)
	}
	if !strings.Contains(sender.messages[0].HTML, "https://www.arbion.ai/brand/arbion-wordmark.svg") || !strings.Contains(sender.messages[0].HTML, "Verify email") {
		t.Fatal("verification email did not include the Arbion-branded HTML alternative")
	}
	rawToken := tokenFromMessage(t, sender.messages[0])
	if bytes.Contains(tokens.hash, []byte(rawToken)) || len(tokens.hash) != sha256.Size {
		t.Fatal("token store retained a raw or malformed email token")
	}
	if _, _, err = service.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip"); !errors.Is(err, ErrEmailVerificationRequired) {
		t.Fatalf("pending login returned %v", err)
	}
	if err = service.VerifyEmail(context.Background(), rawToken, "ip"); err != nil {
		t.Fatal(err)
	}
	if err = service.VerifyEmail(context.Background(), rawToken, "ip"); !errors.Is(err, ErrInvalidEmailToken) {
		t.Fatalf("verification token reuse returned %v", err)
	}
	if _, loginToken, err := service.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip"); err != nil || loginToken == "" {
		t.Fatalf("verified user could not log in: %v", err)
	}
}

func TestPasswordResetIsGenericSingleUseAndRevokesSessions(t *testing.T) {
	m := miniredis.RunT(t)
	sessions := NewRedisStore(redis.NewClient(&redis.Options{Addr: m.Addr()}))
	users := &fakeUsers{}
	tokens := &fakeEmailTokens{users: users}
	sender := &fakeSender{}
	service := NewService(users, sessions, sessions, &fakeAudit{}, time.Hour)
	service.ConfigureEmail(tokens, sender, EmailPolicy{PublicBaseURL: "https://www.arbion.ai", VerificationTTL: 24 * time.Hour, PasswordResetTTL: 30 * time.Minute})
	_, firstSession, err := service.Register(context.Background(), "person@example.com", "correct horse battery staple", "Person", "ip")
	if err != nil {
		t.Fatal(err)
	}
	_, secondSession, err := service.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.RequestPasswordReset(context.Background(), "missing@example.com", "ip-missing"); err != nil || len(sender.messages) != 0 {
		t.Fatalf("unknown-account request was distinguishable: messages=%d error=%v", len(sender.messages), err)
	}
	if err = service.RequestPasswordReset(context.Background(), "PERSON@example.com", "ip-user"); err != nil || len(sender.messages) != 1 {
		t.Fatalf("password reset request failed: messages=%d error=%v", len(sender.messages), err)
	}
	if !strings.Contains(sender.messages[0].HTML, "https://www.arbion.ai/brand/arbion-wordmark.svg") || !strings.Contains(sender.messages[0].HTML, "Reset password") {
		t.Fatal("password reset email did not include the Arbion-branded HTML alternative")
	}
	rawToken := tokenFromMessage(t, sender.messages[0])
	if err = service.ResetPassword(context.Background(), rawToken, "a different secure passphrase", "ip"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{firstSession, secondSession} {
		if _, err = sessions.Get(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("password reset retained an existing session")
		}
	}
	if err = service.ResetPassword(context.Background(), rawToken, "yet another secure passphrase", "ip"); !errors.Is(err, ErrInvalidEmailToken) {
		t.Fatalf("password reset token reuse returned %v", err)
	}
	if _, _, err = service.Login(context.Background(), "person@example.com", "correct horse battery staple", "ip-2"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password remained valid")
	}
	if _, _, err = service.Login(context.Background(), "person@example.com", "a different secure passphrase", "ip-2"); err != nil {
		t.Fatalf("new password was not usable: %v", err)
	}
}
