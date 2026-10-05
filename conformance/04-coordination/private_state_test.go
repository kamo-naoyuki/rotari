package coordination

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

func TestPrivateStateModes(t *testing.T) {
	covers(t, "COORD-5")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "public", "--", "true")
	publicDir := filepath.Join(e.Base, "projects", "public")
	before := fileMode(t, publicDir)

	private := e.WithVar("ROTARI_PRIVATE_STATE", "true")
	private.MustRotari("add", "-p", "private", "--", "true")
	private.MustRotari("add", "-p", "public", "--", "true")
	err := filepath.WalkDir(filepath.Join(e.Base, "projects", "private"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if mode := fileMode(t, path); mode&0o077 != 0 {
			t.Errorf("%s: mode %v, want owner-only", path, mode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if after := fileMode(t, publicDir); after != before {
		t.Errorf("an existing project directory changed mode from %v to %v", before, after)
	}

	static := filepath.Join(e.Root, "static")
	private.MustRotari("web", "--static-dir", static)
	if fileMode(t, static)&0o044 != 0o044 {
		t.Skip("the umask removes group read, so the export's modes cannot show")
	}
	err = filepath.WalkDir(static, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if mode := fileMode(t, path); mode&0o044 != 0o044 {
			t.Errorf("%s: mode %v, want publishable", path, mode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectStates(t *testing.T) {
	covers(t, "CORE-4", "SAFE-1")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "idle", "--", "true")
	if state := e.CheckState("idle"); state != "ready" {
		t.Errorf("a project with a queue: state %q, want ready", state)
	}
	if runID := shownRunID(t, e, "idle"); runID != "" {
		t.Errorf("idle project is not queue-first: show selected run %q", runID)
	}
	run := e.StartRun("live", 1, false)
	if state := e.CheckState("live"); state != "running" {
		t.Errorf("a project with a run: state %q, want running", state)
	}
	if runID := shownRunID(t, e, "live"); runID != run.RunID {
		t.Errorf("running project is not run-first: show selected run %q, want %q", runID, run.RunID)
	}
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	if out := e.MustRotari("show", "-p", "live").Stdout; !strings.Contains(out, "Project state: interrupted") {
		t.Errorf("show does not report the interrupted state:\n%s", out)
	}
}

// shownRunID returns the run that show selects for project, or "" when it shows the queue.
func shownRunID(t *testing.T, e *support.Env, project string) string {
	t.Helper()
	var shown struct {
		RunID string `json:"run_id"`
	}
	out := e.MustRotari("show", "-p", project, "--json").Stdout
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("show --json for %s: %v\n%s", project, err, out)
	}
	return shown.RunID
}

func TestControlFromAnotherHost(t *testing.T) {
	covers(t, "COORD-1", "COORD-2", "COORD-3", "SAFE-1", "SAFE-4")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, run.Jobs[0]) == 1, "the job did not start"
	})
	project := filepath.Join(e.Base, "projects", "live")
	setJSONField(t, filepath.Join(project, "runs", run.RunID, "context.json"), "hostname", "elsewhere")
	for _, command := range []string{"cancel", "suspend", "resume"} {
		if r := e.Rotari(command, "-p", "live", run.Jobs[0]); r.Code == 0 || !strings.Contains(r.Stderr, `runs on host "elsewhere"`) {
			t.Errorf("%s accepted: %s", command, r)
		}
	}
	base := e.StartWeb()
	if got := e.HTTPPostJSON(base+"/api/cancel-job", map[string]any{"project_name": "live", "run_id": run.RunID, "job_id": run.Jobs[0]}); got.Status == 200 || !strings.Contains(got.Body, `runs on host "elsewhere"`) {
		t.Errorf("Web cancel accepted: %d %s", got.Status, got.Body)
	}
	if alive := support.JobProcesses(t, e.Root, run.Jobs[0]); alive != 1 {
		t.Fatalf("rejected request stopped job: %d", alive)
	}
	setJSONField(t, filepath.Join(project, "running.lock"), "host", "elsewhere")
	if state := e.CheckState("live"); state != "locked" {
		t.Errorf("state %q, want locked", state)
	}
	if r := e.Rotari("cancel", "-p", "live"); r.Code == 0 || !strings.Contains(r.Stderr, `owned by host "elsewhere"`) {
		t.Errorf("whole cancel accepted: %s", r)
	}
	if r := e.Rotari("run", "-p", "live"); r.Code == 0 || !strings.Contains(r.Stderr, "is running") {
		t.Errorf("run accepted: %s", r)
	}
	e.MustRotari("unlock", "live")
	if state := e.CheckState("live"); state != "empty" {
		t.Errorf("after unlock state %q", state)
	}
}

func TestSuspendAndResumePreflightAllSelectedJobs(t *testing.T) {
	covers(t, "COORD-6")
	e := support.NewEnv(t)
	run := e.StartRun("live", 2, false)
	project := filepath.Join(e.Base, "projects", "live")
	jobMeta := findAttemptJobMetadata(t, filepath.Join(project, "runs", run.RunID, run.Jobs[1]))
	writeFile(t, jobMeta, `{"executor":"ssh"}`)
	base := e.StartWeb("--allow-control")
	for _, operation := range []string{"suspend", "resume"} {
		for _, call := range []string{"cli", "web"} {
			t.Run(operation+"/"+call, func(t *testing.T) {
				assertControlPreflightFailure(t, e, base, project, run, operation, call)
			})
		}
	}
	for _, jobID := range run.Jobs {
		if alive := support.JobProcesses(t, e.Root, jobID); alive != 1 {
			t.Errorf("preflight failure stopped job %s: process count %d", jobID, alive)
		}
	}
}

func assertControlPreflightFailure(t *testing.T, e *support.Env, webBase, project string, run support.ActiveRun, operation, call string) {
	t.Helper()
	want := `executor "ssh" does not support ` + operation
	if call == "cli" {
		got := e.Rotari(operation, "-p", "live", "--job-id", run.Jobs[0], "--job-id", run.Jobs[1])
		if got.Code == 0 || !strings.Contains(got.Stderr, want) {
			t.Fatalf("CLI %s should reject the unsupported target: %s", operation, got)
		}
	} else {
		got := e.HTTPPostJSON(webBase+"/api/"+operation+"-job", map[string]any{"project_name": "live", "run_id": run.RunID, "job_ids": run.Jobs})
		if got.Status == 200 || !strings.Contains(got.Body, want) {
			t.Fatalf("Web %s should reject the unsupported target: %d %s", operation, got.Status, got.Body)
		}
	}
	firstJobMeta := findAttemptJobMetadata(t, filepath.Join(project, "runs", run.RunID, run.Jobs[0]))
	statusPath := filepath.Join(filepath.Dir(firstJobMeta), "scheduler_status.json")
	if _, err := os.Stat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("failed %s partially changed the first job's scheduler status: stat error %v", operation, err)
	}
}

func findAttemptJobMetadata(t *testing.T, jobDir string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(jobDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "pid" {
			found = filepath.Join(filepath.Dir(path), "job.json")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatalf("no job.json found under %s", jobDir)
	}
	return found
}

func setJSONField(t *testing.T, path, field, value string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	fields[field] = value
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}

func TestUnlockRefusesLiveRun(t *testing.T) {
	covers(t, "SAFE-4", "CORE-5")
	e := support.NewEnv(t)
	e.StartRun("live", 1, false)
	if r := e.Rotari("unlock", "live"); r.Code == 0 {
		t.Errorf("unlock of a run whose coordinator is alive succeeded: %s", r)
	}
	if state := e.CheckState("live"); state != "running" {
		t.Errorf("after unlock: state %q, want running", state)
	}
	if r := e.Rotari("run", "-p", "live", "--async"); r.Code == 0 {
		t.Errorf("a second run of the project started: %s", r)
	}
}

func TestCopyIntoQueueWithoutTerminal(t *testing.T) {
	covers(t, "SAFE-6")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("a")
	e.MustRotari("add", "-p", "a", "--", "true")
	r := e.Rotari("copy", "-p", "a", "--run-id", runID)
	out := r.Stderr + r.Stdout
	if r.Code == 0 || !strings.Contains(out, "--append") || !strings.Contains(out, "--overwrite") {
		t.Errorf("copy into a non-empty queue without a terminal should fail naming --append and --overwrite: %s", r)
	}
}

func TestRunningProjectRejectsSecondRunAndDelete(t *testing.T) {
	covers(t, "CORE-3", "SAFE-2", "CORE-5")
	e := support.NewEnv(t)
	e.FinishedJobRun("live")
	run := e.StartRun("live", 1, false)
	// The run took the queue when it started (CORE-3).
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "queued=0") {
		t.Errorf("queue of a running project is not empty: %s", r)
	}
	for name, args := range map[string][]string{
		"run":    {"run", "-p", "live", "--async"},
		"delete": {"delete", "-p", "live", "--all"},
	} {
		if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "is running") {
			t.Errorf("%s of a running project was not rejected: %s", name, r)
		}
	}
	if r := e.Rotari("copy", "-p", "live", "--run-id", run.RunID); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "is still running") {
		t.Errorf("copy of the active run was not rejected: %s", r)
	}
	if runs, _ := filepath.Glob(filepath.Join(e.Base, "projects", "live", "runs", "*")); len(runs) != 2 {
		t.Errorf("project has %d runs, want two", len(runs))
	}
	if state := e.CheckState("live"); state != "running" {
		t.Errorf("after rejected commands: state %q", state)
	}
	e.MustRotari("add", "-p", "other", "--", "true")
	e.MustRotari("run", "-p", "other", "--quiet")
}

func TestInterruptedProjectNeedsRecovery(t *testing.T) {
	covers(t, "SAFE-3", "SAFE-4")
	e := support.NewEnv(t)
	e.FinishedJobRun("live")
	run := e.StartRun("live", 1, false)
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	if r := e.Rotari("copy", "-p", "live", "--run-id", run.RunID); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "was interrupted") || !strings.Contains(r.Stderr+r.Stdout, "rotari unlock") {
		t.Errorf("copy of the interrupted run did not point to unlock: %s", r)
	}
	for name, args := range support.GuardedCommands("live") {
		r := e.Rotari(args...)
		out := r.Stdout + r.Stderr
		if r.Code == 0 || !strings.Contains(out, "has interrupted run") || !strings.Contains(out, "rotari unlock") || !strings.Contains(out, "rotari show") || !strings.Contains(out, "rotari retry") {
			t.Errorf("%s did not point to show, unlock, and retry: %s", name, r)
		}
	}
	if r := e.Rotari("unlock", "live", "--run-id", "20990101-000000-deadbeef"); r.Code == 0 {
		t.Errorf("unlock accepted wrong run: %s", r)
	}
	r := e.MustRotari("unlock", "live", "--run-id", run.RunID)
	if !strings.Contains(r.Stdout, "rotari retry") || !strings.Contains(r.Stdout, run.RunID) {
		t.Errorf("unlock did not name the retry of the run: %s", r)
	}
	// The run took its jobs from the queue when it started (CORE-3).
	if state := e.CheckState("live"); state != "empty" {
		t.Errorf("after unlock: state %q", state)
	}
	e.MustRotari("add", "-p", "live", "--", "true")
}

// TestQueueEditsBesideAnActiveRun checks that the queue, which holds only the
// next run's jobs, can be edited while a run is active or interrupted, and
// that the next run executes those jobs and not the earlier run's.
func TestQueueEditsBesideAnActiveRun(t *testing.T) {
	covers(t, "CORE-3", "SAFE-8")
	e := support.NewEnv(t)
	finished, _ := e.FinishedJobRun("live")
	manifest := filepath.Join(e.Root, "manifest.yaml")
	e.MustRotari("export", finished, manifest)
	run := e.StartRun("live", 1, false)

	e.MustRotari("import", manifest, "live")
	next := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--job-name", "next", "--", "true"))
	e.MustRotari("change", "-p", "live", "--job-id", next, "--timeout", "1m")
	e.MustRotari("copy", "-p", "live", "--run-id", finished, "--append")
	dropped := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--", "false"))
	e.MustRotari("remove", "-p", "live", dropped)
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "state=running") || !strings.Contains(r.Stdout, "queued=3") {
		t.Fatalf("after queue edits beside the active run: %s", r)
	}

	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	late := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--job-name", "late", "--", "true"))
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "state=interrupted") || !strings.Contains(r.Stdout, "queued=4") {
		t.Fatalf("after an add beside the interrupted run: %s", r)
	}

	e.MustRotari("unlock", "live", "--run-id", run.RunID)
	e.MustRotari("run", "-p", "live", "--quiet")
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "live", "--run-id", "latest", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	ran := map[string]bool{}
	for _, result := range shown.Summary.Results {
		ran[result.ID] = true
	}
	if shown.RunID == run.RunID || len(ran) != 4 || !ran[next] || !ran[late] || ran[run.Jobs[0]] || ran[dropped] {
		t.Errorf("next run %s executed %v; want the four queued jobs only", shown.RunID, ran)
	}
}

func TestResetClearsQueueBesideActiveRun(t *testing.T) {
	covers(t, "SAFE-2", "SAFE-5", "SAFE-8")
	e := support.NewEnv(t)
	e.StartRun("live", 1, true)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	e.MustRotari("add", "-p", "live", "--", "true")
	e.MustRotari("reset", "live")
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "state=running") || !strings.Contains(r.Stdout, "queued=0") {
		t.Fatalf("reset changed the active run or kept queued work: %s", r)
	}
}

func TestShowActiveRunIncludesNextQueue(t *testing.T) {
	covers(t, "SAFE-10")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, true)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the active job did not start"
	})
	next := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--job-name", "next", "--", "true"))
	text := e.MustRotari("show", "-p", "live", "--run-id", run.RunID).Stdout
	for _, want := range []string{run.RunID, "=== Next queue ===", next, "next"} {
		if !strings.Contains(text, want) {
			t.Errorf("show output missing %q:\n%s", want, text)
		}
	}
	var shown struct {
		RunID    string `json:"run_id"`
		Commands struct {
			Commands []struct {
				ID string `json:"id"`
			} `json:"commands"`
		} `json:"commands"`
		NextQueue struct {
			Commands []struct {
				ID string `json:"id"`
			} `json:"commands"`
		} `json:"next_queue"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "live", "--run-id", run.RunID, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.RunID != run.RunID || len(shown.Commands.Commands) != 1 || shown.Commands.Commands[0].ID == next || len(shown.NextQueue.Commands) != 1 || shown.NextQueue.Commands[0].ID != next {
		t.Fatalf("show JSON = %+v; want active run snapshot and separate next queue", shown)
	}
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	text = e.MustRotari("show", "-p", "live", "--run-id", run.RunID).Stdout
	if !strings.Contains(text, "=== Next queue ===") || !strings.Contains(text, next) {
		t.Fatalf("interrupted show omitted the next queue:\n%s", text)
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "live", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.RunID != run.RunID || len(shown.NextQueue.Commands) != 1 || shown.NextQueue.Commands[0].ID != next {
		t.Fatalf("interrupted show JSON = %+v; want run plus separate next queue", shown)
	}
}

func TestResetOfInterruptedProject(t *testing.T) {
	covers(t, "SAFE-5", "SAFE-8")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	e.MustRotari("add", "-p", "live", "--", "true")
	e.MustRotari("reset", "live")
	if state := e.CheckState("live"); state != "interrupted" {
		t.Errorf("after reset: state %q, want interrupted", state)
	}
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "queued=0") {
		t.Errorf("reset left queued work: %s", r)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "live", "runs", run.RunID)); err != nil {
		t.Errorf("reset removed run history: %v", err)
	}
}

func TestUnlockWithoutInterruptedRunIsNoOp(t *testing.T) {
	covers(t, "SAFE-4")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "idle", "--", "true")
	e.MustRotari("unlock", "idle")
	if state := e.CheckState("idle"); state != "ready" {
		t.Errorf("after unlock of idle project: state %q, want ready", state)
	}
	e.MustRotari("run", "-p", "idle", "--quiet")
	e.MustRotari("unlock", "idle")
	if state := e.CheckState("idle"); state != "empty" {
		t.Errorf("after unlock of finished project: state %q, want empty", state)
	}
}

func fileMode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestMissingSchedulerCommand(t *testing.T) {
	covers(t, "COORD-4")
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"sh", "true"} {
		found, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not found: %v", tool, err)
		}
		if err := os.Symlink(found, filepath.Join(path, tool)); err != nil {
			t.Fatal(err)
		}
	}
	e := support.NewEnv(t).WithVar("PATH", path)
	job := support.AddedJobID(t, e.MustRotari("add", "-p", "s", "--executor", "slurm", "--", "true"))
	if r := e.Rotari("run", "-p", "s", "--quiet"); r.Code == 0 {
		t.Fatalf("a slurm job ran without sbatch: %s", r)
	}
	if out := e.MustRotari("show", "-p", "s", "--run-id", "latest", "--job-id", job).Stdout; !strings.Contains(out, "sbatch") {
		t.Errorf("show of the failed job does not name the missing command:\n%s", out)
	}
}

func TestUnversionedStateIsVersionOne(t *testing.T) {
	covers(t, "STATE-2")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("p")
	runDir := filepath.Join(e.Base, "projects", "p", "runs", runID)
	setStateVersion(t, filepath.Join(runDir, "summary.json"), 0)
	setStateVersion(t, filepath.Join(runDir, "commands.json"), 0)
	e.MustRotari("add", "-p", "p", "--", "true")
	setStateVersion(t, filepath.Join(e.Base, "projects", "p", "queue.json"), 0)
	e.MustRotari("show", "-p", "p", "--run-id", runID)
	e.MustRotari("show", "-p", "p", "--queue")
	e.MustRotari("copy", "-p", "p", "--run-id", runID, "--append")
	e.MustRotari("run", "-p", "p", "--quiet")
}

func TestMalformedLoadSamplesAreSkipped(t *testing.T) {
	covers(t, "STATE-4")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("p")
	samples := filepath.Join(e.Base, "projects", "p", "runs", runID, "load_samples.jsonl")
	data, err := os.ReadFile(samples)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, samples, "not json\n\n{\"at\":\n"+string(data))
	e.MustRotari("show", "-p", "p", "--run-id", runID)
	base := e.StartWeb()
	if got := e.HTTPGet(base + "/api/state"); got.Status != 200 {
		t.Errorf("Web API state with malformed load samples: status %d: %s", got.Status, got.Body)
	}
}

func TestReadingHistoryDoesNotRewriteIt(t *testing.T) {
	covers(t, "STATE-3")
	e := support.NewEnv(t)
	first, attemptID := e.FinishedJobRun("p")
	second, _ := e.FinishedJobRun("p")
	runDir := filepath.Join(e.Base, "projects", "p", "runs", first)
	setStateVersion(t, filepath.Join(runDir, "summary.json"), 0)
	before := snapshotTree(t, runDir)
	base := e.StartWeb()
	for _, args := range [][]string{
		{"show", "-p", "p", "--run-id", first},
		{"show", attemptID},
		{"show", "-p", "p", "--run-id", first, "--report"},
		{"jobs", "p"},
		{"lineage", first, second},
		{"export", first},
		{"wait", first},
		{"copy", "-p", "p", "--run-id", first, "--overwrite"},
	} {
		e.Rotari(args...)
	}
	e.HTTPGet(base + "/api/state")
	after := snapshotTree(t, runDir)
	for path, content := range before {
		if after[path] != content {
			t.Errorf("reading the run rewrote %s", strings.TrimPrefix(path, runDir))
		}
	}
}

func TestNewerStateVersionIsRejected(t *testing.T) {
	covers(t, "STATE-1")
	type readCommand struct{ args []string }
	readers := func(file, runID, jobID string) []readCommand {
		switch file {
		case "queue.json":
			return []readCommand{{[]string{"show", "-p", "p", "--queue"}}, {[]string{"check", "p"}}, {[]string{"add", "-p", "p", "--", "true"}}, {[]string{"run", "-p", "p"}}}
		case "commands.json":
			return []readCommand{{[]string{"show", "-p", "p", "--run-id", runID}}, {[]string{"copy", "-p", "p", "--run-id", runID, "--overwrite"}}, {[]string{"export", runID}}}
		default:
			return []readCommand{{[]string{"show", "-p", "p", "--run-id", runID}}, {[]string{"show", "-p", "p", "--run-id", runID, "--job-id", jobID}}, {[]string{"jobs", "p"}}, {[]string{"copy", "-p", "p", "--run-id", runID, "--overwrite"}}, {[]string{"copy", "-p", "p", "--run-id", runID, "--failed", "--overwrite"}}, {[]string{"export", runID}}}
		}
	}
	for _, file := range []string{"queue.json", "commands.json", "summary.json"} {
		t.Run(file, func(t *testing.T) {
			e := support.NewEnv(t)
			runID, _ := e.FinishedJobRun("p")
			var shown struct {
				Summary struct {
					Results []struct {
						ID string `json:"id"`
					} `json:"results"`
				} `json:"summary"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "p", "--json").Stdout), &shown); err != nil || len(shown.Summary.Results) == 0 {
				t.Fatal(err)
			}
			jobID := shown.Summary.Results[0].ID
			e.MustRotari("add", "-p", "p", "--", "true")
			path := filepath.Join(e.Base, "projects", "p", "queue.json")
			if file != "queue.json" {
				path = filepath.Join(e.Base, "projects", "p", "runs", runID, file)
			}
			written := setStateVersion(t, path, 99)
			for i, command := range readers(file, runID, jobID) {
				t.Run(command.args[0]+fmt.Sprint(i), func(t *testing.T) {
					got := e.Rotari(command.args...)
					if got.Code == 0 || !strings.Contains(got.Stderr+got.Stdout, "upgrade rotari") {
						t.Errorf("read newer %s without upgrade message: %s", file, got)
					}
				})
			}
			if data, _ := os.ReadFile(path); string(data) != written {
				t.Errorf("a command rewrote the newer %s", file)
			}
		})
	}
}

func TestWebShowsNewerRunAsUnreadable(t *testing.T) {
	covers(t, "STATE-1")
	for _, file := range []string{"summary.json", "commands.json"} {
		t.Run(file, func(t *testing.T) {
			e := support.NewEnv(t)
			readable, _ := e.FinishedJobRun("p")
			newer, _ := e.FinishedJobRun("p")
			setStateVersion(t, filepath.Join(e.Base, "projects", "p", "runs", newer, file), 99)
			base := e.StartWeb()
			got := e.HTTPGet(base + "/api/project?project_name=p")
			if got.Status != 200 {
				t.Fatalf("Web project: status %d: %s", got.Status, got.Body)
			}
			var project struct {
				Runs []struct {
					RunID      string `json:"run_id"`
					Status     string `json:"status"`
					Unreadable string `json:"unreadable"`
				} `json:"runs"`
			}
			if err := json.Unmarshal([]byte(got.Body), &project); err != nil || len(project.Runs) != 2 {
				t.Fatalf("Web project: %v: %s", err, got.Body)
			}
			for _, summary := range project.Runs {
				detailResponse := e.HTTPGet(base + "/api/run?project_name=p&run_id=" + summary.RunID)
				if detailResponse.Status != 200 {
					t.Fatalf("Web run %s: status %d: %s", summary.RunID, detailResponse.Status, detailResponse.Body)
				}
				var run struct {
					RunID      string            `json:"run_id"`
					Status     string            `json:"status"`
					Unreadable string            `json:"unreadable"`
					Jobs       []json.RawMessage `json:"jobs"`
				}
				if err := json.Unmarshal([]byte(detailResponse.Body), &run); err != nil {
					t.Fatal(err)
				}
				switch run.RunID {
				case newer:
					if run.Status != "unreadable" || !strings.Contains(run.Unreadable, "upgrade rotari") || len(run.Jobs) != 0 {
						t.Errorf("newer run: %#v", run)
					}
				case readable:
					if run.Status != "finished" || len(run.Jobs) != 1 {
						t.Errorf("readable run: %#v", run)
					}
				}
			}
		})
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[path] = string(data)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func setStateVersion(t *testing.T, path string, version int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if version == 0 {
		delete(fields, "state_version")
	} else {
		fields["state_version"] = version
	}
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestUnlockWarnsAboutRunningJobs stops only the supervisor of a run, so its
// job keeps running, and checks that unlock warns the operator before recovery.
func TestUnlockWarnsAboutRunningJobs(t *testing.T) {
	covers(t, "SAFE-7")
	e := support.NewEnv(t)
	e.StartRun("live", 1, true)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	t.Cleanup(func() { support.KillStrays(t, e.Root) })
	support.KillSupervisors(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	e.MustRotari("add", "-p", "live", "--", "true")
	e.MustRotari("reset", "live")
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("reset changed interrupted state to %q", state)
	}
	if r := e.Rotari("unlock", "live"); r.Code != 0 || !strings.Contains(r.Stderr, "1 of 1 job(s) appear to still be running") || !strings.Contains(r.Stderr, "make sure they have stopped") {
		t.Fatalf("unlock with a running job: %s", r)
	}
}

func TestResetRejectsRemovedRecoveryOptions(t *testing.T) {
	covers(t, "SAFE-9")
	e := support.NewEnv(t)
	for _, r := range []support.Result{
		e.Rotari("reset", "--recover"),
		e.WithVar("ROTARI_RESET_RECOVER", "true").Rotari("reset"),
	} {
		if r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "unlock") {
			t.Errorf("removed reset recovery option did not name unlock: %s", r)
		}
	}
}
