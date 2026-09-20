package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/arbion/platform/services/api/internal/risk"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testAICommitMandateWindow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, mode := range []ExecutionMode{Paper, Shadow} {
		t.Run(string(mode), func(t *testing.T) {
			commit := func(f paperBindingFixture) error {
				if mode == Paper {
					return f.commit(ctx)
				}
				return f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
			}
			fixture := func(from time.Time, until *time.Time) paperBindingFixture {
				t.Helper()
				snapshot, err := json.Marshal(map[string]any{"effective_from": from, "effective_until": until})
				if err != nil {
					t.Fatal(err)
				}
				return newNonLiveBindingFixture(t, ctx, pool, mode, snapshot)
			}
			t.Run("expired and not yet effective", func(t *testing.T) {
				now := time.Now().UTC()
				end := now.Add(-time.Minute)
				for _, f := range []paperBindingFixture{fixture(now.Add(-time.Hour), &end), fixture(now.Add(time.Hour), nil)} {
					if err := commit(f); !errors.Is(err, ErrCommitMandateWindowClosed) {
						t.Fatal("closed mandate window accepted", err)
					}
					f.assertEmpty(t, ctx)
				}
			})
			t.Run("pinned window is independent of newer draft", func(t *testing.T) {
				for _, expiredPinned := range []bool{false, true} {
					now := time.Now().UTC()
					end := now.Add(time.Hour)
					if expiredPinned {
						end = now.Add(-time.Minute)
					}
					f := fixture(now.Add(-time.Hour), &end)
					// The mutable draft has the opposite timing policy. Never repin.
					if _, err := pool.Exec(ctx, `UPDATE automation_mandates SET status='DRAFT',current_version=2,effective_from=now()-interval '2 hours',effective_until=CASE WHEN $2 THEN now()+interval '1 hour' ELSE now()-interval '1 hour' END WHERE id=$1`, f.instance.AutomationMandateID, expiredPinned); err != nil {
						t.Fatal(err)
					}
					if _, err := pool.Exec(ctx, `INSERT INTO automation_mandate_versions(mandate_id,version_number,created_by_user_id,source,snapshot,change_summary) SELECT id,2,user_id,'UI',to_jsonb(m),'{}' FROM automation_mandates m WHERE id=$1`, f.instance.AutomationMandateID); err != nil {
						t.Fatal(err)
					}
					err := commit(f)
					if expiredPinned {
						if !errors.Is(err, ErrCommitMandateWindowClosed) {
							t.Fatal("new draft rescued expired pinned window", err)
						}
						f.assertEmpty(t, ctx)
					} else if err != nil {
						t.Fatal("new draft interrupted valid pinned window", err)
					}
				}
			})
			t.Run("window expires during final row wait", func(t *testing.T) {
				var now time.Time
				if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
					t.Fatal(err)
				}
				end := now.Add(2 * time.Second)
				f := fixture(now.Add(-time.Hour), &end)
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				query := `SELECT id FROM paper_portfolios WHERE strategy_instance_id=$1 FOR UPDATE`
				if mode == Shadow {
					// Compatible with the event foreign-key lock; blocks only the
					// final runtime update, after the first window check passed.
					query = `SELECT id FROM strategy_instances WHERE id=$1 FOR NO KEY UPDATE`
				}
				if _, err = tx.Exec(ctx, query, f.instance.ID); err != nil {
					t.Fatal(err)
				}
				result := make(chan error, 1)
				go func() { result <- commit(f) }()
				waitForPaperBindingLock(t, ctx, pool, tx.Conn().PgConn().PID())
				waitForMandateWindowEnd(t, ctx, pool, end)
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if err = <-result; !errors.Is(err, ErrCommitMandateWindowClosed) {
					t.Fatal("row wait extended pinned authorization", err)
				}
				f.assertEmpty(t, ctx)
				assertCount(t, pool, `SELECT count(*) FROM strategy_instances WHERE id='`+f.instance.ID+`' AND last_evaluated_at IS NULL AND state_version=1 AND status='ACTIVE'`, 1)
			})
			t.Run("exact duplicate survives expiry", func(t *testing.T) {
				end := time.Now().UTC().Add(2 * time.Second)
				f := fixture(end.Add(-time.Hour), &end)
				if err := commit(f); err != nil {
					t.Fatal(err)
				}
				waitForMandateWindowEnd(t, ctx, pool, end)
				if err := commit(f); !errors.Is(err, ErrDuplicate) {
					t.Fatal("expiry hid committed duplicate", err)
				}
				assertCount(t, pool, `SELECT count(*) FROM nonlive_execution_records WHERE strategy_instance_id='`+f.instance.ID+`'`, 1)
			})
			t.Run("denial and abstention stay immutable evidence", func(t *testing.T) {
				end := time.Now().UTC().Add(-time.Minute)
				f := fixture(end.Add(-time.Hour), &end)
				f.evaluation.Decision = risk.Deny
				if err := f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: RiskDenied, ExpectedState: AIMonitoring}, f.now); err != nil {
					t.Fatal(err)
				}
				if err := f.store.CommitAIAbstention(ctx, f.instance, "abstain:"+f.instance.ID, json.RawMessage(`{"decision":"ABSTAIN"}`), f.now); err != nil {
					t.Fatal(err)
				}
				assertCount(t, pool, `SELECT count(*) FROM decision_journal_entries WHERE strategy_instance_id='`+f.instance.ID+`'`, 2)
				assertCount(t, pool, `SELECT count(*) FROM ai_paper_spot_fills WHERE strategy_instance_id='`+f.instance.ID+`'`, 0)
			})
			t.Run("malformed window is unavailable not unbounded", func(t *testing.T) {
				for _, snapshot := range []string{
					`{"effective_until":"bad-time"}`,
					`{"effective_from":null}`,
					`{"effective_from":"0001-01-01T00:00:00Z"}`,
					`{"effective_from":"2026-01-02T00:00:00Z","effective_until":"2026-01-01T00:00:00Z"}`,
				} {
					f := newNonLiveBindingFixture(t, ctx, pool, mode, json.RawMessage(snapshot))
					if err := commit(f); !errors.Is(err, ErrEvaluationConfigurationChanged) {
						t.Fatal("malformed window became unbounded authority", err)
					}
					f.assertEmpty(t, ctx)
				}
			})
			t.Run("another account's expired window cannot stop this account", func(t *testing.T) {
				end := time.Now().UTC().Add(-time.Minute)
				expired := fixture(end.Add(-time.Hour), &end)
				valid := newNonLiveBindingFixture(t, ctx, pool, mode, json.RawMessage(`{}`), expired.instance.UserID)
				if err := commit(expired); !errors.Is(err, ErrCommitMandateWindowClosed) {
					t.Fatal(err)
				}
				expired.assertEmpty(t, ctx)
				if err := commit(valid); err != nil {
					t.Fatal("independent same-owner account interrupted", err)
				}
			})
		})
	}
}

func waitForMandateWindowEnd(t *testing.T, ctx context.Context, pool *pgxpool.Pool, end time.Time) {
	t.Helper()
	for {
		var expired bool
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()>=$1`, end).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
