package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Contracts SAFE-1 to SAFE-6 and CORE-5: how a project that is running or
// interrupted guards its state, and how the operator recovers it. See
// "Concurrency and safety" in contracts/04-coordination-and-safety.md.

// interruptRun starts a run of project whose coordinator and job are then
// killed with SIGKILL, as a crashed host would leave them.
func (e *env) interruptRun(project string) activeRun {
	e.t.Helper()
	run := e.startActiveRun(project, 1)
	killStrays(e.t, e.root)
	return run
}

// checkState returns the state `rotari check` reports for project.
func checkState(e *env, project string) string {
	e.t.Helper()
	for _, field := range strings.Fields(e.rotari("check", project).stdout) {
		if state, ok := strings.CutPrefix(field, "state="); ok {
			return state
		}
	}
	return ""
}

// exportFinishedRun runs one job in project and exports that run, for the
// import that guardedCommands tries. An active or interrupted run cannot be
// exported, so this comes first.
func (e *env) exportFinishedRun(project string) string {
	e.t.Helper()
	runID, _ := e.finishedJobRun(project)
	manifest := filepath.Join(e.root, "manifest.yaml")
	e.mustRotari("export", runID, manifest)
	return manifest
}

// guardedCommands are the commands a running or interrupted project rejects,
// for project with run, and manifest exported from project.
func guardedCommands(project, manifest string, run activeRun) map[string][]string {
	return map[string][]string{
		"run":    {"run", "-p", project},
		"add":    {"add", "-p", project, "--", "true"},
		"copy":   {"copy", "-p", project, "--run-id", run.runID, "--overwrite"},
		"change": {"change", "-p", project, "--job-id", run.jobs[0], "--timeout", "1m"},
		"delete": {"delete", "-p", project, "--all"},
		"remove": {"remove", "-p", project, run.jobs[0]},
		"import": {"import", manifest, project, "--overwrite"},
	}
}

func TestProjectStates(t *testing.T) {
	covers(t, "SAFE-1")
	e := newEnv(t)
	e.mustRotari("add", "-p", "idle", "--", "true")
	if state := checkState(e, "idle"); state != "ready" {
		t.Errorf("a project with a queue: state %q, want ready", state)
	}
	e.startActiveRun("live", 1)
	if state := checkState(e, "live"); state != "running" {
		t.Errorf("a project with a run: state %q, want running", state)
	}
	killStrays(t, e.root)
	if state := checkState(e, "live"); state != "interrupted" {
		t.Errorf("a project whose coordinator was killed: state %q, want interrupted", state)
	}
	if out := e.mustRotari("show", "-p", "live").stdout; !strings.Contains(out, "Project state: interrupted") {
		t.Errorf("show does not report the interrupted state:\n%s", out)
	}
}

func TestRunningProjectRejectsChanges(t *testing.T) {
	covers(t, "SAFE-2", "SAFE-5", "CORE-5")
	e := newEnv(t)
	manifest := e.exportFinishedRun("live")
	run := e.startActiveRun("live", 1)
	commands := guardedCommands("live", manifest, run)
	commands["reset"] = []string{"reset", "live", "--recover"}
	for name, args := range commands {
		if r := e.rotari(args...); r.code == 0 || !strings.Contains(r.stderr+r.stdout, "is running") {
			t.Errorf("%s of a running project was not rejected: %s", name, r)
		}
	}
	if runs, _ := filepath.Glob(filepath.Join(e.base, "projects", "live", "runs", "*")); len(runs) != 2 {
		t.Errorf("project has %d runs, want the finished and the running one", len(runs))
	}
	if state := checkState(e, "live"); state != "running" {
		t.Errorf("after the rejected commands: state %q, want running", state)
	}
	// Another project runs meanwhile.
	e.mustRotari("add", "-p", "other", "--", "true")
	e.mustRotari("run", "-p", "other", "--quiet")
}

func TestInterruptedProjectNeedsRecovery(t *testing.T) {
	covers(t, "SAFE-3", "SAFE-4")
	e := newEnv(t)
	manifest := e.exportFinishedRun("live")
	run := e.interruptRun("live")
	for name, args := range guardedCommands("live", manifest, run) {
		r := e.rotari(args...)
		out := r.stdout + r.stderr
		if r.code == 0 || !strings.Contains(out, "has interrupted run") || !strings.Contains(out, "rotari unlock") || !strings.Contains(out, "rotari show") {
			t.Errorf("%s of an interrupted project did not point to show and unlock: %s", name, r)
		}
	}
	if r := e.rotari("unlock", "live", "--run-id", "20990101-000000-deadbeef"); r.code == 0 {
		t.Errorf("unlock accepted a --run-id that is not the interrupted run: %s", r)
	}
	e.mustRotari("unlock", "live", "--run-id", run.runID)
	if state := checkState(e, "live"); state != "ready" {
		t.Errorf("after unlock: state %q, want ready with the retained queue", state)
	}
	e.mustRotari("add", "-p", "live", "--", "true")
}

func TestUnlockRefusesLiveRun(t *testing.T) {
	covers(t, "SAFE-4", "CORE-5")
	e := newEnv(t)
	e.startActiveRun("live", 1)
	if r := e.rotari("unlock", "live"); r.code == 0 {
		t.Errorf("unlock of a run whose coordinator is alive succeeded: %s", r)
	}
	if state := checkState(e, "live"); state != "running" {
		t.Errorf("after unlock: state %q, want running", state)
	}
	// --async returns at once, so a second runner cannot hang the test.
	if r := e.rotari("run", "-p", "live", "--async"); r.code == 0 {
		t.Errorf("a second run of the project started: %s", r)
	}
}

func TestResetOfInterruptedProject(t *testing.T) {
	covers(t, "SAFE-5", "SAFE-6")
	e := newEnv(t)
	run := e.interruptRun("live")
	r := e.rotari("reset", "live")
	if r.code == 0 || !strings.Contains(r.stderr+r.stdout, "--recover") {
		t.Errorf("reset of an interrupted project without a terminal should fail naming --recover: %s", r)
	}
	e.mustRotari("reset", "live", "--recover")
	if state := checkState(e, "live"); state != "empty" {
		t.Errorf("after reset --recover: state %q, want empty", state)
	}
	if _, err := os.Stat(filepath.Join(e.base, "projects", "live", "runs", run.runID)); err != nil {
		t.Errorf("reset removed the run history: %v", err)
	}
}

func TestCopyIntoQueueWithoutTerminal(t *testing.T) {
	covers(t, "SAFE-6")
	e := newEnv(t)
	runID, _ := e.finishedJobRun("a")
	e.mustRotari("add", "-p", "a", "--", "true")
	r := e.rotari("copy", "-p", "a", "--run-id", runID)
	out := r.stderr + r.stdout
	if r.code == 0 || !strings.Contains(out, "--append") || !strings.Contains(out, "--overwrite") {
		t.Errorf("copy into a non-empty queue without a terminal should fail naming --append and --overwrite: %s", r)
	}
}
