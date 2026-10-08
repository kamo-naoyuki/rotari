package pairruns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func pairWaitSample(t *testing.T, f pairFixture, flag pairFlag) []string {
	t.Helper()
	if flag.Name == "timeout" {
		return []string{"--timeout", "2s"}
	}
	return f.Sample(t, flag)
}

func pairWaitArgs(t *testing.T, f pairFixture, flags []pairFlag) []string {
	t.Helper()
	args := []string{"wait"}
	for _, flag := range flags {
		args = append(args, pairWaitSample(t, f, flag)...)
	}
	if !pairHasFlag(flags, "timeout") {
		args = append(args, "--timeout", "2s")
	}
	return withPairWaitTarget(f, flags, args)
}

// withPairWaitTarget names the completed fixture run unless a flag selects
// it; wait without a target returns at once when no detached run is active.
func withPairWaitTarget(f pairFixture, flags []pairFlag, args []string) []string {
	if pairHasFlag(flags, "run-id") || pairHasFlag(flags, "project-name") {
		return args
	}
	return append(args, f.Run)
}

func TestCLIFlagPairWaitSamples(t *testing.T) {
	covers(t, "CLI-12")
	f := newPairFixture(t)
	stopPairWaitFixture(t, f)
	for _, command := range readPairSchema(t, f.E) {
		if command.Name != "wait" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(flag.Name, func(t *testing.T) {
				args := append([]string{"wait"}, pairWaitSample(t, f, flag)...)
				if !pairHasFlag([]pairFlag{flag}, "timeout") {
					args = append(args, "--timeout", "2s")
				}
				args = withPairWaitTarget(f, []pairFlag{flag}, args)
				before := savePairTree(t, f.E.Root)
				result := pairInvoke(t, f.E, args...)
				assertPairWaitOutcome(t, f, []pairFlag{flag}, result)
				assertPairWaitStateUnchanged(t, f, before)
			})
		}
	}
}

func TestCLIFlagPairWait(t *testing.T) {
	start := time.Now()
	f := newPairFixture(t)
	stopPairWaitFixture(t, f)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.E) {
		if candidate.Name == "wait" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("wait is missing from the schema")
	}
	pairs := commandFlagPairs(command)
	for _, pair := range pairs {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			ab := invokePairWait(t, f, []pairFlag{pair.A, pair.B})
			ba := invokePairWait(t, f, []pairFlag{pair.B, pair.A})
			if !reflect.DeepEqual(ab, ba) {
				t.Fatalf("order-dependent wait:\n%s\n%s", ab.Process, ba.Process)
			}
		})
	}
	t.Logf("wait pairs with completed failed-run outcome=%d invocations=%d elapsed=%s", len(pairs), 2*len(pairs), time.Since(start))
}

func invokePairWait(t *testing.T, f pairFixture, flags []pairFlag) pairMutationResult {
	t.Helper()
	before := savePairTree(t, f.E.Root)
	result := pairInvoke(t, f.E, pairWaitArgs(t, f, flags)...)
	assertPairWaitOutcome(t, f, flags, result)
	assertPairWaitStateUnchanged(t, f, before)
	result.Args = nil
	return pairMutationResult{Process: result, State: before.Observation(t)}
}

func assertPairWaitOutcome(t *testing.T, f pairFixture, flags []pairFlag, result support.Result) {
	t.Helper()
	if result.Code != 1 || result.Stderr != "" {
		t.Fatalf("completed failed fixture should return its run exit code without diagnostics: %s", result)
	}
	if pairHasFlag(flags, "json") {
		assertPairWaitJSON(t, result, f.Run)
		return
	}
	if pairHasFlag(flags, "quiet") {
		if result.Stdout != "" {
			t.Fatalf("wait --quiet did not suppress completion: %s", result)
		}
		return
	}
	if !strings.Contains(result.Stdout, "=== Run failed ===") || !strings.Contains(result.Stdout, "Run: "+f.Run) {
		t.Fatalf("wait did not report the selected failed run: %s", result)
	}
}

func assertPairWaitJSON(t *testing.T, result support.Result, runID string) {
	t.Helper()
	var summary struct {
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		Results  []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil {
		t.Fatalf("wait --json did not emit one run-summary object: %v\n%s", err, result.Stdout)
	}
	if summary.RunID != runID || summary.Status != "failed" || summary.ExitCode != 1 || len(summary.Results) != 7 {
		t.Fatalf("wait --json summary = %+v, want failed run %s, exit 1, seven results", summary, runID)
	}
}

func assertPairWaitStateUnchanged(t *testing.T, f pairFixture, before pairSavedTree) {
	t.Helper()
	after := savePairTree(t, f.E.Root)
	if !reflect.DeepEqual(before.Raw(), after.Raw()) {
		t.Fatal("wait changed fixture state")
	}
}

func stopPairWaitFixture(t *testing.T, f pairFixture) {
	t.Helper()
	result := f.E.Rotari("server", "shutdown")
	if result.Code != 0 && !(result.Code == 1 && strings.TrimSpace(result.Stderr) == "server is not running") {
		t.Fatalf("could not stop fixture supervisor: %s", result)
	}
}

func TestCLIFlagPairWaitJSONAndEarlyFailure(t *testing.T) {
	covers(t, "CLI-12", "RUN-6")
	f := newPairFixture(t)
	stopPairWaitFixture(t, f)
	text := pairInvoke(t, f.E, "wait", "--run-id", f.Run, "--timeout", "2s")
	machine := pairInvoke(t, f.E, "wait", "--run-id", f.Run, "--timeout", "2s", "--json")
	if text.Code != machine.Code || machine.Code != 1 || machine.Stderr != "" || strings.Contains(text.Stdout, "\"run_id\"") {
		t.Fatalf("text and JSON completion modes disagree:\n%s\n%s", text, machine)
	}
	assertPairWaitJSON(t, machine, f.Run)

	seedPairWaitRunning(t, f)
	if err := os.WriteFile(f.Config, []byte("timeout: 15s\nwait:\n  timeout: 12s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitEnv := f.E.WithVar("ROTARI_WAIT_TIMEOUT", "10s")
	before := savePairTree(t, f.E.Root)
	started := time.Now()
	timedOut := pairInvoke(t, waitEnv, "wait", "--config", f.Config, "--run-id", f.Run, "--timeout", "1s")
	elapsed := time.Since(started)
	if timedOut.Code != 1 || !strings.Contains(timedOut.Stderr, "timed out waiting for run "+f.Run) || !strings.Contains(timedOut.Stdout, "=== Run attached ===") || strings.Contains(timedOut.Stdout, "=== Run started ===") || strings.Contains(timedOut.Stdout, "=== Run failed ===") || elapsed < time.Second || elapsed >= 5*time.Second {
		t.Fatalf("--timeout did not bound the wait as requested (elapsed %s): %s", elapsed, timedOut)
	}
	assertPairWaitStateUnchanged(t, f, before)

	started = time.Now()
	earlyText := pairInvoke(t, waitEnv, "wait", "--config", f.Config, "--run-id", f.Run, "--until-failure", "--timeout", "3s")
	earlyElapsed := time.Since(started)
	if earlyText.Code != 1 || earlyText.Stderr != "" || !strings.Contains(earlyText.Stdout, "is still running, and jobs have failed") || earlyElapsed >= 3*time.Second {
		t.Fatalf("--until-failure did not return before the timeout (%s): %s", earlyElapsed, earlyText)
	}
	if strings.Contains(earlyText.Stdout, "job_name=ok") {
		t.Fatalf("early-failure output included a successful job: %s", earlyText.Stdout)
	}
	earlyJSON := pairInvoke(t, waitEnv, "wait", "--config", f.Config, "--run-id", f.Run, "--until-failure", "--timeout", "3s", "--json")
	if earlyJSON.Code != 1 || earlyJSON.Stderr != "" {
		t.Fatalf("JSON early-failure wait failed: %s", earlyJSON)
	}
	var early struct {
		RunID    string `json:"run_id"`
		Status   string `json:"status"`
		Failures []struct {
			Jobs []struct {
				ID string `json:"id"`
			} `json:"jobs"`
		} `json:"failures"`
	}
	if err := json.Unmarshal([]byte(earlyJSON.Stdout), &early); err != nil {
		t.Fatalf("--until-failure --json did not emit structured failures: %v\n%s", err, earlyJSON.Stdout)
	}
	foundBad := false
	for _, group := range early.Failures {
		for _, job := range group.Jobs {
			foundBad = foundBad || job.ID == f.Bad
		}
	}
	if early.RunID != f.Run || early.Status != "running" || len(early.Failures) == 0 || !foundBad {
		t.Fatalf("early-failure JSON = %+v, want running run %s and failures", early, f.Run)
	}
	assertPairWaitStateUnchanged(t, f, before)
}

func seedPairWaitRunning(t *testing.T, f pairFixture) {
	t.Helper()
	projectDir := filepath.Join(f.E.Base, "projects", f.Project)
	metaPath := filepath.Join(projectDir, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	meta["phase"], meta["last_run_id"] = "running", f.Run
	data, err = json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	lock := map[string]any{"pid": 1, "run_id": f.Run, "host": "pair-remote.invalid"}
	data, err = json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "running.lock"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(projectDir, "runs", f.Run, "summary.json")
	if err := os.Remove(summary); err != nil {
		t.Fatal(err)
	}
}
