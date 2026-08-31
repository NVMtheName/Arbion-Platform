package risk

import "testing"

func TestAIAutonomousPaperCheckPlanMatchesEngineOrder(t *testing.T) {
	plan := AIAutonomousPaperCheckPlan()
	engine := NewEngine()
	if len(plan) != len(engine.rules)+1 {
		t.Fatalf("paper plan has %d stages, want %d", len(plan), len(engine.rules)+1)
	}
	seenCanonical := map[ReasonCode]struct{}{}
	for index, stage := range plan {
		if stage.CanonicalCode == "" || len(stage.AcceptedCodes) == 0 {
			t.Fatalf("stage %d is incomplete: %#v", index, stage)
		}
		if _, duplicate := seenCanonical[stage.CanonicalCode]; duplicate {
			t.Fatalf("duplicate canonical stage %q", stage.CanonicalCode)
		}
		seenCanonical[stage.CanonicalCode] = struct{}{}
		accepted := map[ReasonCode]struct{}{}
		for _, code := range stage.AcceptedCodes {
			if code == "" {
				t.Fatalf("stage %q includes an empty accepted code", stage.CanonicalCode)
			}
			accepted[code] = struct{}{}
		}
		if _, ok := accepted[stage.CanonicalCode]; !ok {
			t.Fatalf("stage %q does not accept its canonical code", stage.CanonicalCode)
		}
		if index < len(engine.rules) && engine.rules[index].Code() != stage.CanonicalCode {
			t.Fatalf("stage %d is %q, engine rule is %q", index, stage.CanonicalCode, engine.rules[index].Code())
		}
	}
	if plan[len(plan)-1].CanonicalCode != RepeatActionCooldownActive {
		t.Fatalf("paper plan does not end with the repeat-action guard: %#v", plan[len(plan)-1])
	}
}
