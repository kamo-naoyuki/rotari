package pairruns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/conformance/support"
)

func TestCLIFlagPairUnlockSamples(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	for _, command := range readPairSchema(t, f.E) {
		if command.Name != "unlock" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(flag.Name, func(t *testing.T) {
				seed := seedPairInterruptedUnlock(t, f)
				args := append([]string{"unlock"}, f.Sample(t, flag)...)
				if flag.Name != "assume-stopped" {
					args = append(args, "--assume-stopped")
				}
				result := pairInvoke(t, f.E, args...)
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
	f.Initial.Restore(t, f.E.Root, *f.Current)
	projectDir := filepath.Join(f.E.Base, "projects", f.Project)
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
	meta["last_run_id"] = f.Run
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
	lockBytes, err := json.Marshal(map[string]any{"pid": -1, "run_id": f.Run, "host": host})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "running.lock"), lockBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	*f.Current = savePairTree(t, f.E.Root)
	return *f.Current
}

func TestCLIFlagPairUnlock(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.E) {
		if candidate.Name == "unlock" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("unlock is missing from the schema")
	}
	for _, pair := range commandFlagPairs(command) {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			ab := invokePairUnlock(t, f, []pairFlag{pair.A, pair.B})
			ba := invokePairUnlock(t, f, []pairFlag{pair.B, pair.A})
			if !reflect.DeepEqual(ab, ba) {
				t.Fatalf("order-dependent unlock:\n%s\n%s\nstate equal=%t", ab.Process, ba.Process, reflect.DeepEqual(ab.State, ba.State))
			}
		})
	}
}

func invokePairUnlock(t *testing.T, f pairMutationFixture, flags []pairFlag) pairMutationResult {
	t.Helper()
	seed := seedPairInterruptedUnlock(t, f)
	args := []string{"unlock"}
	for _, flag := range flags {
		args = append(args, f.Sample(t, flag)...)
	}
	if !pairHasFlag(flags, "assume-stopped") {
		args = append(args, "--assume-stopped")
	}
	result := pairInvoke(t, f.E, args...)
	assertPairUnlockRecovered(t, f, result, seed)
	if pairHasFlag(flags, "json") {
		var output map[string]any
		if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
			t.Fatal(err)
		}
		verification := pairInvoke(t, f.E, "unlock", "--project-name", f.Project, "--dry-run", "--json")
		var current struct {
			Revision string `json:"revision"`
		}
		if err := json.Unmarshal([]byte(verification.Stdout), &current); err != nil {
			t.Fatal(err)
		}
		if verification.Code != 0 || current.Revision == "" || output["revision"] != current.Revision {
			t.Fatalf("incorrect revision: %s", result)
		}
		output["revision"] = "<verified-revision>"
		coordinator := output["coordinator"].(map[string]any)
		delete(coordinator, "observed_at")
		pairUnlockExecutionTimes(t, output["repair"].(map[string]any))
		data, err := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		result.Stdout = string(data)
	} else {
		result.Stdout = regexp.MustCompile(`observed_at=\S+`).ReplaceAllStringFunc(result.Stdout, func(field string) string {
			if _, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(field, "observed_at=")); err != nil {
				t.Fatalf("invalid execution observation: %s", field)
			}
			return "observed_at=<execution-time>"
		})
	}
	result.Args = nil
	return pairMutationResult{Process: result, State: pairUnlockObservation(t, *f.Current)}
}

func assertPairUnlockRecovered(t *testing.T, f pairMutationFixture, result support.Result, seed pairSavedTree) {
	t.Helper()
	dryRun, jsonOutput := false, false
	for _, arg := range result.Args {
		dryRun = dryRun || strings.HasPrefix(arg, "--dry-run")
		jsonOutput = jsonOutput || strings.HasPrefix(arg, "--json")
	}
	recovered := strings.Contains(result.Stdout, "repaired finalization project="+f.Project+" run_id="+f.Run)
	if jsonOutput {
		var output struct {
			Allowed bool   `json:"allowed"`
			Applied bool   `json:"applied"`
			RunID   string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
			t.Fatal(err)
		}
		recovered = output.Allowed && output.Applied == !dryRun && output.RunID == f.Run
	} else if dryRun {
		recovered = strings.Contains(result.Stdout, "allowed=true") && strings.Contains(result.Stdout, "run_id="+f.Run)
	}
	if result.Code != 0 || !recovered {
		assertPairOutcome(t, result)
		t.Fatalf("unlock did not recover the seeded interrupted run: %s", result)
	}
	if dryRun {
		after := savePairTree(t, f.E.Root)
		if !reflect.DeepEqual(seed.Raw(), after.Raw()) {
			t.Fatal("unlock preview changed state")
		}
		*f.Current = after
		return
	}
	projectDir := filepath.Join(f.E.Base, "projects", f.Project)
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
	metaRelative, err := filepath.Rel(f.E.Root, filepath.Join(f.E.Base, "projects", f.Project, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seededMeta map[string]any
	for _, file := range seed {
		if file.Path == metaRelative {
			if err := json.Unmarshal(file.Data, &seededMeta); err != nil {
				t.Fatal(err)
			}
		}
	}
	if meta.Phase != "finished" || meta.LastRunID != f.Run {
		t.Fatalf("unlocked metadata = %+v, want finished with retained run ID %q", meta, f.Run)
	}
	var actualMeta map[string]any
	if err := json.Unmarshal(data, &actualMeta); err != nil {
		t.Fatal(err)
	}
	summaryPath := filepath.Join(projectDir, "runs", f.Run, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatal(err)
	}
	seededMeta["phase"] = "finished"
	seededMeta["last_run_exit_code"] = summary["exit_code"]
	seededMeta["updated_at"] = summary["finished_at"]
	if !reflect.DeepEqual(seededMeta, actualMeta) {
		t.Fatalf("unlock changed metadata beyond phase/updated_at: got %v, want %v", actualMeta, seededMeta)
	}

	afterTree := savePairTree(t, f.E.Root)
	contextPath, err := filepath.Rel(f.E.Root, filepath.Join(projectDir, "runs", f.Run, "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	seedContext, savedContext := pairUnlockContext(t, seed, contextPath), pairUnlockContext(t, afterTree, contextPath)
	recovery, ok := savedContext["recovery"].(map[string]any)
	if !ok || recovery["at"] == "" || recovery["coordinator_state"] != "stopped" || recovery["action"] != "finalize" || recovery["assume_stopped"] != true {
		t.Fatalf("missing recovery evidence: %v", savedContext)
	}
	delete(savedContext, "recovery")
	if !reflect.DeepEqual(seedContext, savedContext) {
		t.Fatalf("unlock changed run context beyond recovery evidence: %v", savedContext)
	}
	before, after := seed.Observation(t), afterTree.Observation(t)
	delete(before, contextPath)
	delete(after, contextPath)
	lockRelative, err := filepath.Rel(f.E.Root, filepath.Join(f.E.Base, "projects", f.Project, "running.lock"))
	if err != nil {
		t.Fatal(err)
	}
	delete(before, lockRelative)
	delete(after, metaRelative)
	delete(before, metaRelative)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("unlock changed project state beyond its lock, metadata and recovery evidence")
	}

	*f.Current = savePairTree(t, f.E.Root)
}

func pairUnlockContext(t *testing.T, tree pairSavedTree, path string) map[string]any {
	t.Helper()
	for _, file := range tree {
		if file.Path == path {
			var context map[string]any
			if err := json.Unmarshal(file.Data, &context); err != nil {
				t.Fatal(err)
			}
			return context
		}
	}
	t.Fatalf("missing run context %s", path)
	return nil
}

func pairUnlockObservation(t *testing.T, tree pairSavedTree) map[string]string {
	t.Helper()
	normalized := append(pairSavedTree(nil), tree...)
	for index, file := range normalized {
		if filepath.Base(file.Path) != "context.json" || file.Dir {
			continue
		}
		context := pairUnlockContext(t, normalized, file.Path)
		if recovery, ok := context["recovery"].(map[string]any); ok {
			recovery["at"] = "<recovery-time>"
			pairUnlockExecutionTimes(t, recovery)
			data, err := json.Marshal(context)
			if err != nil {
				t.Fatal(err)
			}
			normalized[index].Data = data
		}
	}
	return normalized.Observation(t)
}

func pairUnlockExecutionTimes(t *testing.T, record map[string]any) {
	t.Helper()
	executions, ok := record["executions"].([]any)
	if !ok || len(executions) == 0 {
		t.Fatal("missing execution evidence in completed fixture")
	}
	for _, value := range executions {
		execution := value.(map[string]any)
		observed, ok := execution["observed_at"].(string)
		if !ok {
			t.Fatalf("missing execution observation: %v", execution)
		}
		if _, err := time.Parse(time.RFC3339Nano, observed); err != nil {
			t.Fatalf("invalid execution observation: %v", execution)
		}
		execution["observed_at"] = "<execution-time>"
	}
}

func TestCLIFlagPairUnlockSafety(t *testing.T) {
	covers(t, "SAFE-4")
	f := newPairMutationFixture(t)
	for _, mode := range []string{"live-local", "wrong-run", "remote", "without-lock"} {
		t.Run(mode, func(t *testing.T) {
			for _, reversed := range []bool{false, true} {
				seed, runID := seedPairUnlockSafety(t, f, mode)
				args := []string{"unlock", "--project-name", f.Project, "--run-id", runID}
				if reversed {
					args = []string{"unlock", "--run-id", runID, "--project-name", f.Project}
				}
				if mode == "without-lock" {
					args = append(args, "--assume-stopped")
				}
				result := pairInvoke(t, f.E, args...)
				after := savePairTree(t, f.E.Root)
				*f.Current = after
				assertPairUnlockSafety(t, f, mode, result, seed, after)
			}
		})
	}
}

func assertPairUnlockSafety(t *testing.T, f pairMutationFixture, mode string, result support.Result, seed, after pairSavedTree) {
	t.Helper()
	if mode != "live-local" && mode != "wrong-run" && mode != "remote" {
		assertPairUnlockRecovered(t, f, result, seed)
		return
	}
	message := "is running"
	if mode == "wrong-run" {
		message = "run lock belongs to"
	} else if mode == "remote" {
		message = "coordinator_unknown"
	}
	if result.Code != 1 || !strings.Contains(result.Stderr, message) || !reflect.DeepEqual(seed.Raw(), after.Raw()) {
		t.Fatalf("unsafe unlock was not rejected without mutation: %s", result)
	}
}

func seedPairUnlockSafety(t *testing.T, f pairMutationFixture, mode string) (pairSavedTree, string) {
	t.Helper()
	seedPairInterruptedUnlock(t, f)
	lockPath := filepath.Join(f.E.Base, "projects", f.Project, "running.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	runID := f.Run
	switch mode {
	case "live-local":
		lock["pid"] = os.Getpid()
	case "wrong-run":
		runID = f.SecondRun // Existing sibling run; resolution must reach the lock check.
	case "remote":
		lock["host"] = lock["host"].(string) + "-pair-remote"
	}
	contextPath := filepath.Join(f.E.Base, "projects", f.Project, "runs", f.Run, "context.json")
	if mode == "live-local" || mode == "remote" {
		contextData, err := os.ReadFile(contextPath)
		if err != nil {
			t.Fatal(err)
		}
		var context map[string]any
		if err := json.Unmarshal(contextData, &context); err != nil {
			t.Fatal(err)
		}
		identity := support.CurrentCoordinator(t)
		identity.Host = lock["host"].(string)
		context["coordinator"] = identity
		contextData, err = json.Marshal(context)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contextPath, contextData, 0o644); err != nil {
			t.Fatal(err)
		}
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
	return savePairTree(t, f.E.Root), runID
}
