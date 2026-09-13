package executionsim

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func openTestJournal(t *testing.T, path string) *Journal {
	t.Helper()
	j, err := OpenJournal(path, fixtureConfig(), fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}
func appendTest(t *testing.T, j *Journal, v Event) {
	t.Helper()
	ok, err := j.Append(v, fixtureTime)
	if err != nil || !ok {
		t.Fatalf("append %s: %v %v", v.Kind, ok, err)
	}
}
func snapshotTest(t *testing.T, j *Journal) Snapshot {
	t.Helper()
	s, err := j.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProviderFixtureScenariosRestartAndMatch(t *testing.T) {
	for _, provider := range []string{"coinbase", "schwab"} {
		t.Run(provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal.jsonl")
			s, err := RunScenario(path, provider, fixtureTime)
			if err != nil {
				t.Fatal(err)
			}
			if !s.Scope.SimulationOnly || s.Scope.Provider != provider || s.MatchedTransactions != 6 || s.AppliedEvents != 17 {
				t.Fatal(s)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(bytes.Split(bytes.TrimSpace(data), []byte{'\n'})) != 20 {
				t.Fatal("missing genesis, economic events, or duplicate delivery evidence")
			}
			if _, err := RunScenario(path, provider, fixtureTime); !errors.Is(err, ErrJournal) {
				t.Fatal("scenario overwrote an existing journal", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(data, after) {
				t.Fatal("scenario changed prior journal")
			}
		})
	}
}

func TestJournalReplayMaintainsEconomicDuplicateAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j := openTestJournal(t, path)
	v := Event{Scope: fixtureConfig().Scope, ID: "delivery", Kind: Deposit, TransactionID: "deposit-a", At: fixtureTime, Amount: "100"}
	appendTest(t, j, v)
	v.ID = "alias"
	if applied, err := j.Append(v, fixtureTime); err != nil || applied {
		t.Fatal(applied, err)
	}
	before := snapshotTest(t, j)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = openTestJournal(t, path)
	if !reflect.DeepEqual(before, snapshotTest(t, j)) {
		t.Fatal("replay changed ledger")
	}
	if applied, err := j.Append(v, fixtureTime); err != nil || applied {
		t.Fatal("redelivery applied after restart", applied, err)
	}
	v.TransactionID = "different-transaction"
	if _, err := j.Append(v, fixtureTime); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate alias lost after restart", err)
	}
}

func TestJournalRefusesSecondWriterAndDifferentScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j := openTestJournal(t, path)
	if other, err := OpenJournal(path, fixtureConfig(), fixtureTime); err == nil {
		_ = other.Close()
		t.Fatal("two writers acquired journal")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	c := fixtureConfig()
	c.Scope.Account = "other-account"
	if other, err := OpenJournal(path, c, fixtureTime); err == nil {
		_ = other.Close()
		t.Fatal("foreign account replayed journal")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("foreign scope changed evidence")
	}
}

func TestJournalConcurrentRedeliveryIsExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j := openTestJournal(t, path)
	v := Event{Scope: fixtureConfig().Scope, ID: "delivery", Kind: Deposit, TransactionID: "deposit-a", At: fixtureTime, Amount: "0.0000000001"}
	var wg sync.WaitGroup
	results := make(chan bool, 20)
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); applied, err := j.Append(v, fixtureTime); results <- applied; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	count := 0
	for applied := range results {
		if applied {
			count++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 || snapshotTest(t, j).Cash != "1000.0000000001" {
		t.Fatal("duplicate money", count)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	j = openTestJournal(t, path)
	if snapshotTest(t, j).MatchedTransactions != 1 {
		t.Fatal("replay duplicate money")
	}
}

func TestJournalCorruptionFailsClosedWithoutRepair(t *testing.T) {
	for _, name := range []string{"torn-tail", "changed-value", "duplicate-row", "unknown-field", "missing-row"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "journal.jsonl")
			j := openTestJournal(t, path)
			appendTest(t, j, opening())
			appendTest(t, j, fixtureEvent("send", Send, 2))
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			switch name {
			case "torn-tail":
				data = data[:len(data)-4]
			case "changed-value":
				data = bytes.Replace(data, []byte(`"quantity":"2"`), []byte(`"quantity":"3"`), 1)
			case "duplicate-row":
				rows := bytes.SplitAfter(data, []byte{'\n'})
				data = append(data, rows[1]...)
			case "unknown-field":
				data = bytes.Replace(data, []byte(`"sequence":1`), []byte(`"extra":true,"sequence":1`), 1)
			case "missing-row":
				rows := bytes.SplitAfter(data, []byte{'\n'})
				data = append(append([]byte{}, rows[0]...), rows[2]...)
			}
			// Deliberate fixture corruption; never run against production evidence.
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if other, err := OpenJournal(path, fixtureConfig(), fixtureTime); err == nil {
				_ = other.Close()
				t.Fatal("corrupt evidence replayed")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(data, after) {
				t.Fatal("corrupt evidence was silently repaired")
			}
		})
	}
}

func TestJournalPersistenceFailureDoesNotPublishState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j := openTestJournal(t, path)
	appendTest(t, j, opening())
	before := snapshotTest(t, j)
	// Inject a storage failure before the next append.
	if err := j.file.Close(); err != nil {
		t.Fatal(err)
	}
	if applied, err := j.Append(fixtureEvent("send", Send, 2), fixtureTime); applied || !errors.Is(err, ErrJournal) {
		t.Fatal(applied, err)
	}
	if _, err := j.Snapshot(); !errors.Is(err, ErrJournal) {
		t.Fatal("failed handle still serving possibly stale state")
	}
	if !reflect.DeepEqual(before, j.engine.Snapshot()) {
		t.Fatal("speculative state published")
	}
	j = openTestJournal(t, path)
	if !reflect.DeepEqual(before, snapshotTest(t, j)) {
		t.Fatal("failed write changed durable state")
	}
}

func TestJournalRestartAfterProcessExit(t *testing.T) {
	if path := os.Getenv("ARBION_SIM_EXIT_FIXTURE"); path != "" {
		j, err := OpenJournal(path, fixtureConfig(), fixtureTime)
		if err != nil {
			os.Exit(2)
		}
		if _, err = j.Append(opening(), fixtureTime); err != nil {
			os.Exit(3)
		}
		if _, err = j.Append(fixtureEvent("send", Send, 2), fixtureTime); err != nil {
			os.Exit(4)
		}
		// No Close/defer: OS releases the writer lock after durable send.
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "crash.jsonl")
	command := exec.Command(os.Args[0], "-test.run=^TestJournalRestartAfterProcessExit$")
	command.Env = append(os.Environ(), "ARBION_SIM_EXIT_FIXTURE="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture subprocess: %v %s", err, output)
	}
	j := openTestJournal(t, path)
	s := snapshotTest(t, j)
	if s.Orders["order-a"].State != "OUTCOME_UNKNOWN" || s.ReservedCash != "85.0000000000" {
		t.Fatal(s)
	}
	if _, err := j.Append(fixtureEvent("repeat-send", Send, 3), fixtureTime); !errors.Is(err, ErrTransition) {
		t.Fatal("restart enabled blind resend", err)
	}
	v := fixtureEvent("ack", Acknowledge, 3)
	v.SimulatedOrderID = "recovered-order"
	appendTest(t, j, v)
	if snapshotTest(t, j).Orders["order-a"].State != "ACKNOWLEDGED" {
		t.Fatal("matching did not recover unknown outcome")
	}
}

func TestJournalRefusesSymlinksAndPublicFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.jsonl")
	j := openTestJournal(t, path)
	_ = j.Close()
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenJournal(link, fixtureConfig(), fixtureTime); err == nil {
		_ = other.Close()
		t.Fatal("symlink accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenJournal(path, fixtureConfig(), fixtureTime); err == nil {
		_ = other.Close()
		t.Fatal("public-readable journal accepted")
	}
}
