package conformance

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Contracts COORD-1 to COORD-5: what happens across hosts, with scheduler
// commands missing, and to state permissions. See "Shared-state
// coordination" in contracts/04-coordination-and-safety.md. Another host is
// simulated by rewriting the host recorded in context.json and running.lock.

// setJSONField rewrites one top-level string field of the JSON file at path.
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

func TestControlFromAnotherHost(t *testing.T) {
	covers(t, "COORD-1", "COORD-2", "COORD-3", "SAFE-1", "SAFE-4")
	e := newEnv(t)
	run := e.startActiveRun("live", 1)
	job := run.jobs[0]
	project := filepath.Join(e.base, "projects", "live")

	// COORD-1: the run's jobs are recorded as running on another host.
	setJSONField(t, filepath.Join(project, "runs", run.runID, "context.json"), "hostname", "elsewhere")
	for _, command := range []string{"cancel", "suspend", "resume"} {
		if r := e.rotari(command, "-p", "live", job); r.code == 0 || !strings.Contains(r.stderr, `runs on host "elsewhere"`) {
			t.Errorf("%s of a job on another host: %s", command, r)
		}
	}
	base := e.startWeb()
	if got := e.httpPostJSON(base+"/api/cancel-job", map[string]any{"project_name": "live", "run_id": run.runID, "job_id": job}); got.status == 200 || !strings.Contains(got.body, `runs on host "elsewhere"`) {
		t.Errorf("Web cancel-job of a job on another host: status %d: %s", got.status, got.body)
	}
	if alive := jobProcesses(t, e.root, job); alive != 1 {
		t.Fatalf("a rejected request stopped the job: %d processes", alive)
	}

	// COORD-2 and COORD-3: the run's coordinator is on another host.
	setJSONField(t, filepath.Join(project, "running.lock"), "host", "elsewhere")
	if state := checkState(e, "live"); state != "locked" {
		t.Errorf("a project locked by another host: state %q, want locked", state)
	}
	if r := e.rotari("cancel", "-p", "live"); r.code == 0 || !strings.Contains(r.stderr, `owned by host "elsewhere"`) {
		t.Errorf("whole-run cancel of a run on another host: %s", r)
	}
	if r := e.rotari("run", "-p", "live"); r.code == 0 || !strings.Contains(r.stderr, "is running") {
		t.Errorf("run of a project locked by another host was not rejected: %s", r)
	}
	// SAFE-4: the operator's unlock removes a lock that cannot be checked.
	e.mustRotari("unlock", "live")
	if state := checkState(e, "live"); state != "ready" {
		t.Errorf("after unlock of a lock from another host: state %q, want ready", state)
	}
}

func TestMissingSchedulerCommand(t *testing.T) {
	covers(t, "COORD-4")
	requireUnixSockets(t)
	path := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only the tools a local job wrapper needs; no sbatch.
	for _, tool := range []string{"sh", "true"} {
		found, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not found: %v", tool, err)
		}
		if err := os.Symlink(found, filepath.Join(path, tool)); err != nil {
			t.Fatal(err)
		}
	}
	e := newEnv(t).withVar("PATH", path)
	job := addedJobID(t, e.mustRotari("add", "-p", "s", "--executor", "slurm", "--", "true"))
	if r := e.rotari("run", "-p", "s", "--quiet"); r.code == 0 {
		t.Fatalf("a slurm job ran without sbatch: %s", r)
	}
	if out := e.mustRotari("show", "-p", "s", "--run-id", "latest", "--job-id", job).stdout; !strings.Contains(out, "sbatch") {
		t.Errorf("show of the failed job does not name the missing command:\n%s", out)
	}
}

func TestPrivateStateModes(t *testing.T) {
	covers(t, "COORD-5")
	e := newEnv(t)
	e.mustRotari("add", "-p", "public", "--", "true")
	publicDir := filepath.Join(e.base, "projects", "public")
	before := fileMode(t, publicDir)

	private := e.withVar("ROTARI_PRIVATE_STATE", "true")
	private.mustRotari("add", "-p", "private", "--", "true")
	private.mustRotari("add", "-p", "public", "--", "true")
	err := filepath.WalkDir(filepath.Join(e.base, "projects", "private"), func(path string, entry fs.DirEntry, err error) error {
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

	static := filepath.Join(e.root, "static")
	private.mustRotari("web", "--static-dir", static)
	if !umaskKeepsGroupRead(t) {
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

func fileMode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// umaskKeepsGroupRead reports whether a file created 0644 keeps its group
// and other read bits under the current umask.
func umaskKeepsGroupRead(t *testing.T) bool {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe")
	writeFile(t, path, "")
	return fileMode(t, path)&0o044 == 0o044
}
