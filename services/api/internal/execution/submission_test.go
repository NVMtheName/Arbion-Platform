package execution

import (
	"testing"
	"time"
)

func TestSubmissionValidationSeparatesArchivedRecoveryFromSendAuthority(t *testing.T) {
	now := time.Now().UTC()
	r := requestFixture()
	digest, _ := requestDigest(r)
	o := Order{ID: r.ClientOrderID, Request: r, RequestDigest: digest, CreatedAt: now.Add(-time.Second)}
	p := providerPreflightFixture(o, r.AccountID, now)
	s := ConfirmedSubmission{Order: o, PortfolioID: p.PortfolioID, PreviewID: p.PreviewID, Preflight: p}
	a := Attempt{OrderID: o.ID, ClientOrderID: r.ClientOrderID, RequestDigest: digest, AuthorizationID: r.CapitalBucketID, CredentialGeneration: 1, ClaimedAt: now, ExpiresAt: now.Add(5 * time.Second)}
	if ValidateSubmission(s, now) != nil || ValidateSubmission(s, now.Add(time.Hour)) == nil || ValidateRecoverySubmission(s, a, now.Add(time.Hour)) != nil {
		t.Fatal("archived proof became send authority or blocked read recovery")
	}
	for name, mutate := range map[string]func(*ConfirmedSubmission, *Attempt){
		"preview":          func(s *ConfirmedSubmission, _ *Attempt) { s.PreviewID = r.OwnerID },
		"order":            func(s *ConfirmedSubmission, _ *Attempt) { s.Order.ID = "" },
		"digest":           func(s *ConfirmedSubmission, _ *Attempt) { s.Order.RequestDigest = "bad" },
		"attempt":          func(_ *ConfirmedSubmission, a *Attempt) { a.OrderID = r.OwnerID },
		"client":           func(_ *ConfirmedSubmission, a *Attempt) { a.ClientOrderID = r.OwnerID },
		"before evidence":  func(_ *ConfirmedSubmission, a *Attempt) { a.ClaimedAt = now.Add(-time.Millisecond) },
		"future claim":     func(_ *ConfirmedSubmission, a *Attempt) { a.ClaimedAt = now.Add(2 * time.Hour) },
		"empty authority":  func(_ *ConfirmedSubmission, a *Attempt) { a.AuthorizationID = "" },
		"expired at claim": func(_ *ConfirmedSubmission, a *Attempt) { a.ExpiresAt = now },
		"excess authority": func(_ *ConfirmedSubmission, a *Attempt) { a.ExpiresAt = now.Add(2 * time.Minute) },
		"provider":         func(_ *ConfirmedSubmission, a *Attempt) { a.ProviderOrderID = "bad" },
		"generation":       func(_ *ConfirmedSubmission, a *Attempt) { a.CredentialGeneration = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			ss, aa := s, a
			mutate(&ss, &aa)
			if ValidateRecoverySubmission(ss, aa, now.Add(time.Hour)) == nil {
				t.Fatal("invalid archive accepted")
			}
		})
	}
}
