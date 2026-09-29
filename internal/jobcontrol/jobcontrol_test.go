package jobcontrol

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type fakeExecutor struct {
	name      string
	cancelled []string
}

func (fake *fakeExecutor) Name() string { return fake.name }

func (fake *fakeExecutor) Submit(string, model.JobSpec, []string) (executor.JobHandle, error) {
	return executor.JobHandle{}, nil
}

func (fake *fakeExecutor) Wait(string, executor.JobHandle) model.JobResult { return model.JobResult{} }

func (fake *fakeExecutor) Cancel(jobDir string) error {
	fake.cancelled = append(fake.cancelled, filepath.Base(jobDir))
	return nil
}

func testController() (Controller, *fakeExecutor) {
	slurm := &fakeExecutor{name: "slurm"}
	return Controller{Store: state.NewStore(0o700, 0o600), Executors: executor.Registry{"slurm": slurm}}, slurm
}

const testRunID = "20260925-000000-00000000"

func writeJobJSON(t *testing.T, dir, executorName string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "job.json"), []byte(fmt.Sprintf(`{"executor":%q}`, executorName)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeRunningAttemptIDsRequiresCurrentRunningAttempt(t *testing.T) {
	controller, _ := testController()
	runDir := filepath.Join(t.TempDir(), testRunID)
	jobID := "job-1"
	oldAttempt := state.MakeAttemptID(testRunID, jobID, 0)
	currentAttempt := state.MakeAttemptID(testRunID, jobID, 1)
	oldDir := filepath.Join(runDir, jobID, "attempts", oldAttempt)
	currentDir := filepath.Join(runDir, jobID, "attempts", currentAttempt)
	writeJobJSON(t, oldDir, "slurm")
	writeJobJSON(t, currentDir, "slurm")

	got, err := controller.normalizeRunningAttemptIDs(runDir, testRunID, []string{currentAttempt, "plain"})
	if err != nil || len(got) != 2 || got[0] != jobID || got[1] != "plain" {
		t.Fatalf("current attempt normalization = %#v, %v", got, err)
	}
	if _, err := controller.normalizeRunningAttemptIDs(runDir, testRunID, []string{oldAttempt}); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("Latest attempt: %q (running)", currentAttempt)) {
		t.Fatalf("stale attempt error = %v", err)
	}
	if _, err := controller.normalizeRunningAttemptIDs(runDir, "other-run", []string{currentAttempt}); err == nil || !strings.Contains(err.Error(), "belongs to run") {
		t.Fatalf("foreign attempt error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "finished_at"), []byte(now()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.normalizeRunningAttemptIDs(runDir, testRunID, []string{currentAttempt}); err == nil || !strings.Contains(err.Error(), "is finished") {
		t.Fatalf("finished attempt error = %v", err)
	}
	pendingID := state.MakeAttemptID(testRunID, "pending", 0)
	if err := os.MkdirAll(filepath.Join(runDir, "pending", "attempts", pendingID), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.normalizeRunningAttemptIDs(runDir, testRunID, []string{pendingID}); err == nil || !strings.Contains(err.Error(), "is pending") {
		t.Fatalf("pending attempt error = %v", err)
	}
}

func TestCancelJobsCancelsOwnedAndMarksUnsubmittedJobs(t *testing.T) {
	controller, slurm := testController()
	runDir := filepath.Join(t.TempDir(), testRunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte(`{"commands":[{"id":"submitted","command":["true"]},{"id":"waiting","command":["true"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeJobJSON(t, filepath.Join(runDir, "submitted"), "slurm")
	message, err := controller.CancelJobs(runDir, "demo", testRunID, []string{"submitted", "waiting"})
	if err != nil || !strings.Contains(message, "Jobs: 2") {
		t.Fatalf("CancelJobs() = %q, %v", message, err)
	}
	if len(slurm.cancelled) != 1 || slurm.cancelled[0] != "submitted" {
		t.Fatalf("cancelled = %#v, want the submitted job", slurm.cancelled)
	}
	if _, err := os.Stat(filepath.Join(runDir, "waiting", "cancelled")); err != nil {
		t.Fatalf("unsubmitted job was not marked cancelled: %v", err)
	}
	if _, err := controller.CancelJobs(runDir, "demo", testRunID, []string{"unknown"}); err == nil || err.Error() != `job "unknown" is not found` {
		t.Fatalf("unknown job error = %v", err)
	}
}

func TestSelectJobsFiltersUnfinishedJobs(t *testing.T) {
	controller, _ := testController()
	runDir := filepath.Join(t.TempDir(), testRunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte(`{"commands":[
		{"id":"train-a","name":"train","stage":"fit","command":["python","train.py"]},
		{"id":"train-b","name":"train","stage":"fit","command":["python","train.py"]},
		{"id":"eval","name":"eval","stage":"score","command":["python","eval.py"]},
		{"id":"done","name":"done","stage":"fit","command":["true"]},
		{"id":"waiting","name":"waiting","stage":"fit","command":["sh","wait.sh"]}
	]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	writeRunning := func(jobID, host string, start time.Time) {
		dir := filepath.Join(runDir, jobID)
		writeJobJSON(t, dir, "slurm")
		status := fmt.Sprintf(`{"phase":"running","hosts":[%q],"started_at":%q}`, host, start.Format(time.RFC3339))
		if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(status), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeRunning("train-a", "gpu-1", started)
	writeRunning("train-b", "cpu-1", started.Add(time.Hour))
	writeRunning("eval", "gpu-2", started)
	writeRunning("done", "gpu-1", started)
	if err := os.WriteFile(filepath.Join(runDir, "done", "finished_at"), []byte(now()), 0o600); err != nil {
		t.Fatal(err)
	}
	now := started.Add(3 * time.Hour)
	for _, test := range []struct {
		name      string
		selection Selection
		want      string
	}{
		{"by name", Selection{Names: []string{"train"}}, "train-a,train-b"},
		{"by stage includes pending", Selection{Scope: model.CommandSelector{Stage: "fit"}}, "train-a,train-b,waiting"},
		{"running only", Selection{Scope: model.CommandSelector{Stage: "fit"}, States: []string{StateRunning}}, "train-a,train-b"},
		{"pending only", Selection{States: []string{StatePending}}, "waiting"},
		{"command", Selection{Filter: jobfilter.Filter{Command: `eval\.py`}}, "eval"},
		{"host", Selection{Filter: jobfilter.Filter{Hosts: []string{"gpu-*"}}}, "eval,train-a"},
		{"longer than", Selection{Filter: jobfilter.Filter{LongerThan: 150 * time.Minute}}, "eval,train-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := controller.selectJobs(runDir, test.selection, now)
			if err != nil || strings.Join(got, ",") != test.want {
				t.Fatalf("selectJobs() = %v, %v, want %s", got, err, test.want)
			}
		})
	}
	if _, err := controller.selectJobs(runDir, Selection{Names: []string{"missing"}}, now); err == nil || err.Error() != `job name "missing" not found` {
		t.Fatalf("unknown name error = %v", err)
	}
	if _, err := controller.selectJobs(runDir, Selection{Names: []string{"done"}}, now); err == nil || err.Error() != "no unfinished jobs match the selection" {
		t.Fatalf("finished-only selection error = %v", err)
	}
}

func TestControlRejectsUnsupportedOperationAndIdleProject(t *testing.T) {
	controller, _ := testController()
	baseDir := t.TempDir()
	if _, err := controller.Control(baseDir, "demo", "", nil, "pause"); err == nil || err.Error() != "unsupported job operation: pause" {
		t.Fatalf("pause error = %v", err)
	}
	if _, err := controller.Control(baseDir, "demo", "", nil, "suspend"); err == nil || err.Error() != `project "demo" is not running` {
		t.Fatalf("idle project error = %v", err)
	}
}

func TestFinishCancelMessage(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	message, err := finishCancelMessage("Cancel requested", paths, "run-1", false)
	if err != nil || !strings.Contains(message, "Inspect status") || !strings.Contains(message, "rotari show --run-id run-1") || strings.Contains(message, "Cancellation complete") {
		t.Fatalf("message = %q, %v, want inspect hint without waiting", message, err)
	}

	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), Host: host, RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.Remove(paths.LockFile)
	}()
	message, err = finishCancelMessage("Cancel requested", paths, "run-1", true)
	if err != nil || !strings.Contains(message, "Cancellation complete") {
		t.Fatalf("message = %q, %v, want cancellation complete", message, err)
	}
}

func TestRunnerHostMismatch(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if _, mismatch := runnerHostMismatch(model.LockInfo{Host: strings.ToUpper(host)}); mismatch {
		t.Fatal("same host with different case was a mismatch")
	}
	if recorded, mismatch := runnerHostMismatch(model.LockInfo{Host: host + "-other"}); !mismatch || recorded != host+"-other" {
		t.Fatalf("other host = %q, %v", recorded, mismatch)
	}
}
