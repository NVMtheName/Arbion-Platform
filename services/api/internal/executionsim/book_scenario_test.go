package executionsim

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBookScenarioDurableDepthDerivedSettlement(t *testing.T) {
	for _, provider := range []string{"coinbase", "schwab"} {
		t.Run(provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "book.jsonl")
			s, err := RunBookScenario(path, provider, fixtureTime)
			if err != nil {
				t.Fatal(err)
			}
			if !s.Scope.SimulationOnly || s.Scope.Provider != provider || s.Cash != "977.5050000000" || s.MatchedTransactions != 4 || s.AppliedEvents != 14 {
				t.Fatal(s)
			}
			// Independent runs produce exactly the same projection. Neither
			// can overwrite a retained journal or silently reuse its liquidity.
			other, err := RunBookScenario(filepath.Join(t.TempDir(), "book.jsonl"), provider, fixtureTime)
			if err != nil || !reflect.DeepEqual(s, other) {
				t.Fatal("nondeterministic fixture", other, err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := RunBookScenario(path, provider, fixtureTime); !errors.Is(err, ErrJournal) {
				t.Fatal("existing journal was not refused", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("existing evidence changed", err)
			}
		})
	}
}
