package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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
	if err = s.RevokeUser(context.Background(), "u"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{a, b} {
		if _, err = s.Get(context.Background(), v); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("session not revoked")
		}
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

func (f *fakeUsers) Create(_ context.Context, email, n, h, name string) (User, error) {
	if f.user.ID != "" {
		return User{}, ErrConflict
	}
	f.user = User{ID: "u1", Email: email, NormalizedEmail: n, PasswordHash: h, DisplayName: name, Status: "active"}
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

type fakeAudit struct{ actions []string }

func (a *fakeAudit) Record(_ context.Context, _ *string, action string, _ map[string]any) error {
	a.actions = append(a.actions, action)
	return nil
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
func TestSafeSerializationOmitsSecrets(t *testing.T) {
	u := User{ID: "1", Email: "a@b.co", PasswordHash: "secret", NormalizedEmail: "a@b.co", Status: "active"}.Safe()
	if u.Email != "a@b.co" {
		t.Fatal("unsafe conversion")
	}
}
