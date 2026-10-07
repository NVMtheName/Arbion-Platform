package execution

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/credential"
	"github.com/arbion/platform/services/api/internal/financial"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reuse only the shared test transport/observation shape. No owner approval is
// created or fabricated: each autonomous request has only real mandate consent.
func newMandateSendOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c mandateConsentFixture, side string, funding ...string) sendFixture {
	t.Helper()
	r := c.request
	r.Side = side
	if side == "SELL" {
		r.MaximumDebitUSD = "0"
	}
	o, err := NewPostgresStore(pool).PrepareScheduledProposal(ctx, r.OwnerID, newScheduledProposalInput(t, ctx, pool, c, r))
	if err != nil {
		t.Fatal(err)
	}
	r = o.Request
	cash, base := "1000", "1"
	if len(funding) == 2 {
		cash, base = funding[0], funding[1]
	}
	var rec string
	err = pool.QueryRow(ctx, `INSERT INTO portfolio_reconciliations(user_id,financial_account_id,provider_name,comparison_status,balances_status,positions_status,performance_status,realized_performance_status,autonomy_signal,observed_position_count,performance_position_count,change_count,evidence_hash,observed_at,cash_amount,cash_currency,available_cash_amount,available_cash_currency)
	 VALUES($1,$2,'coinbase','MATCHED','READY','READY','UNAVAILABLE','UNAVAILABLE','CLEAR',1,0,0,decode(repeat('55',32),'hex'),clock_timestamp(),$3,'USD',$3,'USD') RETURNING id::text`, r.OwnerID, r.AccountID, cash).Scan(&rec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO portfolio_reconciliation_positions(reconciliation_id,user_id,financial_account_id,symbol,instrument_type,direction,quantity,available_quantity,performance_status) VALUES($1,$2,$3,'BTC','CRYPTO','long',$4,$4,'UNAVAILABLE')`, rec, r.OwnerID, r.AccountID, base); err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	p := providerPreflightFixture(o, r.AccountID, now)
	quantity, _ := decimal(r.BaseSize, true)
	price, _ := decimal(r.LimitPrice, true)
	fee, _ := decimal(r.FeeAllowanceUSD, false)
	gross := new(big.Rat).Mul(quantity, price)
	total := new(big.Rat).Add(gross, fee)
	if side == "SELL" {
		total.Sub(gross, fee)
	}
	p.PreviewBaseSize, p.PreviewQuoteSize, p.PreviewFeeUSD, p.PreviewTotalUSD = r.BaseSize, canonical(gross), r.FeeAllowanceUSD, canonical(total)
	p.CashUSD, p.AvailableCashUSD, p.TotalBase, p.AvailableBase = cash, cash, base, base
	id, err := NewPostgresStore(pool).savePreflight(ctx, o, c.consent.CredentialGeneration, p.PortfolioID, p)
	if err != nil {
		t.Fatal(err)
	}
	vault := fixturePreflightVault{generation: c.consent.CredentialGeneration, retrieve: func(_ context.Context, l credential.Locator) ([]byte, error) {
		if l.ConnectionID != r.ConnectionID || l.UserID != r.OwnerID || l.Class != credential.Financial {
			return nil, ErrInvalid
		}
		return json.Marshal(financial.Credentials{APIKeyName: "synthetic-autonomous-key", APIPrivateKey: "synthetic-not-real-key", PortfolioID: p.PortfolioID})
	}}
	var providerID string
	if err = pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	return sendFixture{authorityFixture: authorityFixture{order: o, principal: c.principal}, preflight: p, evidence: id, vault: vault,
		ack: SubmissionAcknowledgement{providerID, r.ClientOrderID, r.ProductID, r.Side}}
}

func sendMandateOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c mandateConsentFixture, f sendFixture) Attempt {
	t.Helper()
	calls := 0
	a, err := NewPostgresStore(pool).SendAutonomous(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(callCtx context.Context, _ *financial.Credentials, submission ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		calls++
		deadline, bounded := callCtx.Deadline()
		if !bounded || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second || submission.Order.Request.MandateApprovalID != c.consent.ID {
			t.Error("autonomous callback changed exact consent or deadline")
		}
		assertSendHeld(t, ctx, pool, f.order, "")
		return f.ack, nil
	}))
	if err != nil || calls != 1 || a.ProviderOrderID != f.ack.ProviderOrderID {
		t.Fatal("exact autonomous send did not enter once", err, calls, a)
	}
	var exact bool
	err = pool.QueryRow(ctx, `SELECT approval_id IS NULL AND mandate_approval_id=$2 AND capital_bucket_id=$3
	 AND risk_evaluation->>'Source'='AI' AND risk_evaluation->>'Mode'='LIVE'
	 AND risk_evaluation->>'ApprovalRequired'='false' AND risk_evaluation->>'PlatformExecutionAvailable'='false'
	 AND risk_evaluation->>'MandateID'=$4 AND (risk_evaluation->>'MandateVersion')::int=$5
	 AND NOT EXISTS(SELECT 1 FROM execution_owner_approvals WHERE order_id=$1)
	 FROM execution_authorizations WHERE order_id=$1`, f.order.ID, c.consent.ID, f.order.Request.CapitalBucketID, c.mandateID, c.consent.MandateVersion).Scan(&exact)
	if err != nil || !exact {
		t.Fatal("autonomous authority manufactured owner evidence or lost mandate identity", err, exact)
	}
	return a
}

func settleMandateOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f sendFixture) AccountSettlementEvidence {
	t.Helper()
	e := pilotAllocationTerminal(t, ctx, pool, f, "FILLED", f.order.Request.BaseSize, f.preflight.PreviewQuoteSize, f.order.Request.FeeAllowanceUSD)
	if _, err := NewPostgresStore(pool).SettleBrokerAccount(ctx, f.order.Request.OwnerID, f.order.ID, f.vault, savedSettlement(e)); err != nil {
		t.Fatal(err)
	}
	r, err := NewPostgresStore(pool).ReadCapitalReservation(ctx, f.order.Request.OwnerID, f.order.ID)
	if err != nil || r.ReleasedAt == nil {
		t.Fatal("exact autonomous settlement did not release original reservation", err)
	}
	return e
}

func TestPostgresMandateSendSettledBuySellAcrossFreshScheduledVersions(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	first := newMandateSendOrder(t, ctx, pool, c, "BUY")
	sendMandateOrder(t, ctx, pool, c, first)
	e := settleMandateOrder(t, ctx, pool, first)
	assertPilotAllocationBalance(t, ctx, pool, c.request, "93.94", "0.0001", 1, false)
	firstConsent := c.consent.ID
	c = renewScheduledMandateFixture(t, ctx, pool, c, time.Time{})
	second := newMandateSendOrder(t, ctx, pool, c, "SELL", e.CashUSD, e.TotalBase)
	// MaxTradesPerDay=2: the final checker must exclude exactly this second
	// durable claim, while still counting the already settled first order.
	sendMandateOrder(t, ctx, pool, c, second)
	settleMandateOrder(t, ctx, pool, second)
	assertPilotAllocationBalance(t, ctx, pool, c.request, "99.88", "0", 2, false)
	var authorizations, approvals int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM execution_authorizations WHERE mandate_approval_id IN ($1,$2)),(SELECT count(*) FROM execution_mandate_approvals WHERE id IN ($1,$2))`, firstConsent, c.consent.ID).Scan(&authorizations, &approvals); err != nil || authorizations != 2 || approvals != 2 {
		t.Fatal("fresh scheduled versions lost exact consent and authorization history", err, authorizations, approvals)
	}
	for _, f := range []sendFixture{first, second} {
		if _, err := NewPostgresStore(pool).SendAutonomous(ctx, f.order.Request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
			t.Error("settled autonomous order retried")
			return f.ack, nil
		})); !errors.Is(err, ErrAlreadyAttempted) {
			t.Fatal("settlement reopened original send", err)
		}
	}
}

func TestPostgresMandateSendFinalControlChangesCloseWithoutCallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, kind := range []string{"version", "revocation", "expired mandate", "AUTOMATION", "GLOBAL", "USER", "ACCOUNT"} {
		t.Run(kind, func(t *testing.T) {
			c := newMandateConsentFixture(t, ctx, pool)
			f := newMandateSendOrder(t, ctx, pool, c, "BUY")
			var stopID string
			switch kind {
			case "AUTOMATION", "GLOBAL", "USER", "ACCOUNT":
				// Register exact-ID cleanup on this subtest before inserting.
				// Even GLOBAL is closed before t.Run returns to the next case.
				stopID = newSendStopID(t, ctx, pool)
			}
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(callCtx context.Context, n int) error {
				if n != 3 {
					return nil
				}
				var err error
				switch kind {
				case "version":
					_, err = pool.Exec(callCtx, `UPDATE automation_mandates SET current_version=current_version+1 WHERE id=$1`, c.mandateID)
				case "revocation":
					err = NewPostgresStore(pool).RevokeMandateConsent(callCtx, c.request.OwnerID, c.consent.ID)
				case "expired mandate":
					_, err = pool.Exec(callCtx, `UPDATE automation_mandates SET effective_until=clock_timestamp()-interval '1 second' WHERE id=$1`, c.mandateID)
				default:
					var scopeID any
					switch kind {
					case "AUTOMATION":
						scopeID = c.mandateID
					case "USER":
						scopeID = c.request.OwnerID
					case "ACCOUNT":
						scopeID = c.request.AccountID
					}
					_, err = pool.Exec(callCtx, `INSERT INTO risk_circuit_breakers(id,scope,scope_id,state,reason,source) VALUES($1,$2,$3,'OPEN','synthetic autonomous stop','SYSTEM')`, stopID, kind, scopeID)
				}
				return err
			}}
			called := false
			_, err := NewPostgresStore(db).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				return f.ack, nil
			}))
			if !errors.Is(err, ErrNotAuthorized) || !errors.Is(err, ErrSubmissionNotSent) || called {
				t.Fatal("changed authority reached autonomous callback", err, called)
			}
			assertNoSendClosed(t, ctx, pool, f)
		})
	}
}

func TestPostgresMandateSendConcurrentClaimAndLostCommit(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent", true: "lost claim commit"}[lost], func(t *testing.T) {
			c := newMandateConsentFixture(t, ctx, pool)
			f := newMandateSendOrder(t, ctx, pool, c, "BUY")
			var calls atomic.Int32
			sender := sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				calls.Add(1)
				return f.ack, nil
			})
			if lost {
				db := &sendBoundaryDB{Database: pool, wrap: func(n int, tx pgx.Tx) pgx.Tx {
					if n == 2 {
						return lostCommitTx{tx}
					}
					return tx
				}}
				if _, err := NewPostgresStore(db).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sender); !errors.Is(err, ErrCommitUnknown) || calls.Load() != 0 {
					t.Fatal("lost claim commit reached sender", err, calls.Load())
				}
				assertSendHeld(t, ctx, pool, f.order, "")
			} else {
				var wait sync.WaitGroup
				results := make(chan error, 6)
				for i := 0; i < 6; i++ {
					wait.Add(1)
					go func() {
						defer wait.Done()
						_, err := NewPostgresStore(pool).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sender)
						results <- err
					}()
				}
				wait.Wait()
				close(results)
				wins := 0
				for err := range results {
					if err == nil {
						wins++
					} else if !errors.Is(err, ErrAlreadyAttempted) {
						t.Fatal("unexpected concurrent result", err)
					}
				}
				if wins != 1 || calls.Load() != 1 {
					t.Fatal("concurrent autonomous order sent more than once", wins, calls.Load())
				}
				assertSendHeld(t, ctx, pool, f.order, f.ack.ProviderOrderID)
			}
			before := calls.Load()
			if _, err := NewPostgresStore(pool).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sender); !errors.Is(err, ErrAlreadyAttempted) || calls.Load() != before {
				t.Fatal("recovered attempt resent", err)
			}
		})
	}
}

func TestPostgresMandateSendConsentExpirationBoundsFinalAdmissionAndCallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	for _, beforeFinal := range []bool{true, false} {
		t.Run(map[bool]string{true: "before final guard", false: "during bounded callback"}[beforeFinal], func(t *testing.T) {
			c := newMandateConsentFixture(t, ctx, pool, pilotTestDeadline(t, ctx, pool))
			f := newMandateSendOrder(t, ctx, pool, c, "BUY")
			db := &sendBoundaryDB{Database: pool, beforeBegin: func(callCtx context.Context, n int) error {
				if n == 3 && beforeFinal {
					waitPastPilotDeadline(t, callCtx, pool, c.consent.ExpiresAt)
				}
				return nil
			}}
			called, bounded := false, false
			_, err := NewPostgresStore(db).SendAutonomous(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(callCtx context.Context, _ *financial.Credentials, _ ConfirmedSubmission) (SubmissionAcknowledgement, error) {
				called = true
				deadline, ok := callCtx.Deadline()
				bounded = ok && time.Until(deadline) > 0 && time.Until(deadline) <= 3*time.Second
				<-callCtx.Done()
				return f.ack, nil
			}))
			if beforeFinal {
				if !errors.Is(err, ErrNotAuthorized) || !errors.Is(err, ErrSubmissionNotSent) || called {
					t.Fatal("expired immutable consent reached callback", err, called)
				}
				assertNoSendClosed(t, ctx, pool, f)
			} else {
				if !errors.Is(err, ErrSubmissionUnknown) || !called || !bounded {
					t.Fatal("consent expiry did not bound synchronous callback", err, called, bounded)
				}
				assertSendHeld(t, ctx, pool, f.order, "")
			}
		})
	}
}

func TestPostgresMandateOrderHasNoOwnerAuthorityFallback(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	f := newMandateSendOrder(t, ctx, pool, c, "BUY")
	s := NewPostgresStore(pool)
	if _, err := s.Claim(ctx, c.request.OwnerID, f.order.ID, NewOwnerAuthority(NewSavedPreflightVerifier(f.evidence))); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("owner authority accepted autonomous request", err)
	}
	if _, err := s.SendConfirmed(ctx, c.request.OwnerID, f.order.ID, f.evidence, f.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		t.Error("owner sender entered")
		return f.ack, nil
	})); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("owner send admitted autonomous request", err)
	}
	if _, err := s.ApproveOrder(ctx, c.principal, f.order.ID, f.order.RequestDigest, "synthetic", stepUpFunc(func(context.Context, string, string) (string, time.Time, error) {
		t.Error("autonomous request consumed owner MFA")
		return "totp", time.Now(), nil
	})); !errors.Is(err, ErrNotAuthorized) {
		t.Fatal("owner approval admitted autonomous request", err)
	}
	if _, err := s.ReadAttempt(ctx, c.request.OwnerID, f.order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("fallback created attempt", err)
	}
}

func TestPostgresMandateDailyAttemptsSurviveNewConsentAndVersion(t *testing.T) {
	ctx, pool := setupExecutionTest(t)
	c := newMandateConsentFixture(t, ctx, pool)
	first := newMandateSendOrder(t, ctx, pool, c, "BUY")
	sendMandateOrder(t, ctx, pool, c, first)
	e := settleMandateOrder(t, ctx, pool, first)
	// A fresh, explicitly consented version can lower its quota, never reset
	// previous account attempts. The next SELL is not a repeated BUY cooldown.
	var version int
	if err := pool.QueryRow(ctx, `UPDATE automation_mandates SET current_version=current_version+1,risk_parameters='{"max_trades_per_day":1}' WHERE id=$1 RETURNING current_version`, c.mandateID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot)
	 SELECT m.id,m.current_version,m.user_id,'SYSTEM',(to_jsonb(m)-ARRAY['id','user_id','current_version','created_at','updated_at'])||'{"execution_capable":false}'::jsonb FROM automation_mandates m WHERE id=$1
	 RETURNING encode(sha256(convert_to(snapshot::text,'UTF8')),'hex')`, c.mandateID).Scan(&c.snapshotDigest); err != nil {
		t.Fatal(err)
	}
	var err error
	c.consent, err = NewPostgresStore(pool).ApproveMandate(ctx, c.principal, c.request.CapitalBucketID, c.mandateID, version, c.snapshotDigest, "synthetic", mandateConsentStepUp(pool))
	if err != nil {
		t.Fatal(err)
	}
	c.request.MandateApprovalID = c.consent.ID
	second := newMandateSendOrder(t, ctx, pool, c, "SELL", e.CashUSD, e.TotalBase)
	called := false
	_, err = NewPostgresStore(pool).SendAutonomous(ctx, c.request.OwnerID, second.order.ID, second.evidence, second.vault, sendFunc(func(context.Context, *financial.Credentials, ConfirmedSubmission) (SubmissionAcknowledgement, error) {
		called = true
		return second.ack, nil
	}))
	if !errors.Is(err, ErrNotAuthorized) || called {
		t.Fatal("new consent reset daily attempt budget", err, called)
	}
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_dispatch_attempts WHERE financial_account_id=$1`, c.request.AccountID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("daily quota denial persisted another attempt", err, attempts)
	}
}
