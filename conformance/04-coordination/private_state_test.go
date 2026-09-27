package coordination

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	support.RequireUnixSockets(t)
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
		{"diff", first, second},
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

func setStateVersion(t *testing.T, path string, version int) {
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
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
