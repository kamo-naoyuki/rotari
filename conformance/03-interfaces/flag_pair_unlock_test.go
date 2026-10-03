package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestCLIFlagPairUnlockSamples(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	for _, command := range readPairSchema(t, f.e) {
		if command.Name != "unlock" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(flag.Name, func(t *testing.T) {
				seed := seedPairInterruptedUnlock(t, f)
				args := append([]string{"unlock"}, f.sample(t, flag)...)
				result := pairInvoke(t, f.e, args...)
				assertPairUnlockRecovered(t, f, result, seed)
			})
		}
	}
}

// The synthetic stale local lock lets the public CLI recovery path run without
// starting a supervisor or job. The referenced run directory comes from the
// finished fixture and remains available for --run-id resolution.
func seedPairInterruptedUnlock(t *testing.T, f pairMutationFixture) pairSavedTree {
	t.Helper()
	f.initial.restore(t, f.e.Root, *f.current)
	projectDir := filepath.Join(f.e.Base, "projects", f.project)
	metaPath := filepath.Join(projectDir, "meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatal(err)
	}
	meta["phase"] = "running"
	meta["last_run_id"] = f.run
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	lockBytes, err := json.Marshal(map[string]any{"pid": -1, "run_id": f.run, "host": host})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "running.lock"), lockBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	*f.current = savePairTree(t, f.e.Root)
	return *f.current
}

func TestCLIFlagPairUnlock(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.e) {
		if candidate.Name == "unlock" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("unlock is missing from the schema")
	}
	for _, pair := range commandFlagPairs(command) {
		t.Run(pair.a.Name+"+"+pair.b.Name, func(t *testing.T) {
			ab := invokePairUnlock(t, f, []pairFlag{pair.a, pair.b})
			ba := invokePairUnlock(t, f, []pairFlag{pair.b, pair.a})
			if !reflect.DeepEqual(ab, ba) {
				t.Fatalf("order-dependent unlock:\n%s\n%s\nstate equal=%t", ab.process, ba.process, reflect.DeepEqual(ab.state, ba.state))
			}
		})
	}
}

func invokePairUnlock(t *testing.T, f pairMutationFixture, flags []pairFlag) pairMutationResult {
	t.Helper()
	seed := seedPairInterruptedUnlock(t, f)
	args := []string{"unlock"}
	for _, flag := range flags {
		args = append(args, f.sample(t, flag)...)
	}
	result := pairInvoke(t, f.e, args...)
	assertPairUnlockRecovered(t, f, result, seed)
	result.Args = nil
	return pairMutationResult{process: result, state: f.current.observation(t)}
}

func assertPairUnlockRecovered(t *testing.T, f pairMutationFixture, result support.Result, seed pairSavedTree) {
	t.Helper()
	if result.Code != 0 || !strings.Contains(result.Stdout, "recovered queue project="+f.project+" run_id="+f.run) {
		assertPairOutcome(t, result)
		t.Fatalf("unlock did not recover the seeded interrupted run: %s", result)
	}
	projectDir := filepath.Join(f.e.Base, "projects", f.project)
	if _, err := os.Stat(filepath.Join(projectDir, "running.lock")); !os.IsNotExist(err) {
		t.Fatalf("stale run lock remains after successful unlock: %v", err)
	}
	var meta struct {
		Phase     string `json:"phase"`
		LastRunID string `json:"last_run_id"`
	}
	data, err := os.ReadFile(filepath.Join(projectDir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	metaRelative, err := filepath.Rel(f.e.Root, filepath.Join(f.e.Base, "projects", f.project, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seededMeta map[string]any
	for _, file := range seed {
		if file.path == metaRelative {
			if err := json.Unmarshal(file.data, &seededMeta); err != nil {
				t.Fatal(err)
			}
		}
	}
	if meta.Phase != "collecting" || meta.LastRunID != f.run {
		t.Fatalf("unlocked metadata = %+v, want collecting with retained run ID %q", meta, f.run)
	}
	var actualMeta map[string]any
	if err := json.Unmarshal(data, &actualMeta); err != nil {
		t.Fatal(err)
	}
	seededMeta["phase"] = "collecting"
	delete(seededMeta, "updated_at")
	delete(actualMeta, "updated_at")
	if !reflect.DeepEqual(seededMeta, actualMeta) {
		t.Fatalf("unlock changed metadata beyond phase/updated_at: got %v, want %v", actualMeta, seededMeta)
	}

	before, after := seed.observation(t), savePairTree(t, f.e.Root).observation(t)
	lockRelative, err := filepath.Rel(f.e.Root, filepath.Join(f.e.Base, "projects", f.project, "running.lock"))
	if err != nil {
		t.Fatal(err)
	}
	delete(before, lockRelative)
	delete(after, metaRelative)
	delete(before, metaRelative)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("unlock changed project state other than its stale lock and recovery metadata")
	}
	*f.current = savePairTree(t, f.e.Root)
}

func TestCLIFlagPairUnlockSafety(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	for _, mode := range []string{"live-local", "wrong-run", "remote", "without-lock"} {
		t.Run(mode, func(t *testing.T) {
			for _, reversed := range []bool{false, true} {
				seed, runID := seedPairUnlockSafety(t, f, mode)
				args := []string{"unlock", "--project-name", f.project, "--run-id", runID}
				if reversed {
					args = []string{"unlock", "--run-id", runID, "--project-name", f.project}
				}
				result := pairInvoke(t, f.e, args...)
				after := savePairTree(t, f.e.Root)
				*f.current = after
				assertPairUnlockSafety(t, f, mode, result, seed, after)
			}
		})
	}
}

func assertPairUnlockSafety(t *testing.T, f pairMutationFixture, mode string, result support.Result, seed, after pairSavedTree) {
	t.Helper()
	if mode != "live-local" && mode != "wrong-run" {
		assertPairUnlockRecovered(t, f, result, seed)
		return
	}
	message := "is running"
	if mode == "wrong-run" {
		message = "run lock belongs to"
	}
	if result.Code != 1 || !strings.Contains(result.Stderr, message) || !reflect.DeepEqual(seed.raw(), after.raw()) {
		t.Fatalf("unsafe unlock was not rejected without mutation: %s", result)
	}
}

func seedPairUnlockSafety(t *testing.T, f pairMutationFixture, mode string) (pairSavedTree, string) {
	t.Helper()
	seedPairInterruptedUnlock(t, f)
	lockPath := filepath.Join(f.e.Base, "projects", f.project, "running.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	runID := f.run
	switch mode {
	case "live-local":
		lock["pid"] = os.Getpid()
	case "wrong-run":
		runID = f.secondRun // Existing sibling run; resolution must reach the lock check.
	case "remote":
		lock["host"] = lock["host"].(string) + "-pair-remote"
	}
	data, err = json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if mode == "without-lock" {
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
	}
	return savePairTree(t, f.e.Root), runID
}
