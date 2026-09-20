package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/arbion/platform/services/api/internal/automation"
	"github.com/jackc/pgx/v5/pgxpool"
)

func commitShadowBindingFixture(ctx context.Context, f paperBindingFixture) error {
	return f.store.CommitEvaluation(ctx, f.instance, f.instance.StateVersion, f.decision, f.evaluation, ExecutionResult{Status: WouldHaveSubmitted, ExpectedState: AIMonitoring}, f.now)
}

func testAIShadowCommitBindings(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	for _, status := range []string{"PAUSED", "DISABLED", "ARCHIVED"} {
		t.Run("current mandate "+status+" blocks prepared Shadow action", func(t *testing.T) {
			f := newNonLiveBindingFixture(t, ctx, pool, Shadow, json.RawMessage(`{}`))
			if _, err := automation.NewPostgresStore(pool).Transition(ctx, f.instance.UserID, f.instance.AutomationMandateID, 1, status, "UI"); err != nil {
				t.Fatal(err)
			}
			if err := commitShadowBindingFixture(ctx, f); !errors.Is(err, ErrEvaluationConfigurationChanged) {
				t.Fatal("revoked mandate accepted a prepared Shadow action", err)
			}
			f.assertEmpty(t, ctx)
		})
	}
}
