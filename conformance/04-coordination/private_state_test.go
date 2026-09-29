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
	if state := e.CheckState("live"); state != "ready" {
		t.Errorf("after unlock state %q", state)
	}
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

func TestRunningProjectRejectsChanges(t *testing.T) {
	covers(t, "CORE-3", "SAFE-2", "SAFE-5", "CORE-5")
	e := support.NewEnv(t)
	manifest := e.ExportFinishedRun("live")
	run := e.StartRun("live", 1, false)
	commands := support.GuardedCommands("live", manifest, run)
	commands["reset"] = []string{"reset", "live", "--recover"}
	for name, args := range commands {
		if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "is running") {
			t.Errorf("%s of a running project was not rejected: %s", name, r)
		}
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
	manifest := e.ExportFinishedRun("live")
	run := e.StartRun("live", 1, false)
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	for name, args := range support.GuardedCommands("live", manifest, run) {
		r := e.Rotari(args...)
		out := r.Stdout + r.Stderr
		if r.Code == 0 || !strings.Contains(out, "has interrupted run") || !strings.Contains(out, "rotari unlock") || !strings.Contains(out, "rotari show") {
			t.Errorf("%s did not point to show and unlock: %s", name, r)
		}
	}
	if r := e.Rotari("unlock", "live", "--run-id", "20990101-000000-deadbeef"); r.Code == 0 {
		t.Errorf("unlock accepted wrong run: %s", r)
	}
	e.MustRotari("unlock", "live", "--run-id", run.RunID)
	if state := e.CheckState("live"); state != "ready" {
		t.Errorf("after unlock: state %q", state)
	}
	e.MustRotari("add", "-p", "live", "--", "true")
}

func TestResetOfInterruptedProject(t *testing.T) {
	covers(t, "SAFE-5", "SAFE-6")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	r := e.Rotari("reset", "live")
	if r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "--recover") {
		t.Errorf("reset did not require --recover: %s", r)
	}
	e.MustRotari("reset", "live", "--recover")
	if state := e.CheckState("live"); state != "empty" {
		t.Errorf("after reset: state %q", state)
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
		{"diagnose", "--rules", attemptID},
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
			got := e.HTTPGet(e.StartWeb() + "/api/state")
			if got.Status != 200 {
				t.Fatalf("Web state: status %d: %s", got.Status, got.Body)
			}
			var state struct {
				Projects []struct {
					Runs []struct {
						RunID      string            `json:"run_id"`
						Status     string            `json:"status"`
						Unreadable string            `json:"unreadable"`
						Jobs       []json.RawMessage `json:"jobs"`
					} `json:"runs"`
				} `json:"projects"`
			}
			if err := json.Unmarshal([]byte(got.Body), &state); err != nil || len(state.Projects) != 1 {
				t.Fatalf("Web state: %v: %s", err, got.Body)
			}
			for _, run := range state.Projects[0].Runs {
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
