package support

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type PairFlag struct {
	Name      string   `json:"name"`
	ValueName string   `json:"value_name"`
	Values    []string `json:"values"`
}

type PairCommand struct {
	Name  string     `json:"name"`
	Flags []PairFlag `json:"flags"`
}

type FlagPair struct{ A, B PairFlag }

// One execution classification shared by the inventory and runners. Missing
// adapters are deferred explicitly rather than counted as passing tests.
func PairAdapter(command string) string {
	switch command {
	case "show", "jobs", "check", "lineage":
		return "read"
	case "config", "export":
		return "file"
	case "remove", "reset", "delete":
		return "mutation"
	case "add", "change", "copy", "import":
		return "edit"
	case "run", "retry":
		return "preview"
	case "unlock":
		return "control"
	case "wait":
		return "wait"
	case "gc", "server":
		return "registry"
	case "mcp":
		return "mcp"
	case "web":
		return "static"
	case "diagnose":
		return "diagnose"
	case "cancel", "suspend", "resume":
		return "jobcontrol"
	default:
		return ""
	}
}

func CommandFlagPairs(c PairCommand) []FlagPair {
	var pairs []FlagPair
	for i, a := range c.Flags {
		for _, b := range c.Flags[i+1:] {
			pairs = append(pairs, FlagPair{a, b})
		}
	}
	return pairs
}

func ReadPairSchema(t *testing.T, e *Env) []PairCommand {
	t.Helper()
	var schema struct {
		Version  int           `json:"version"`
		Commands []PairCommand `json:"commands"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("schema", "--json").Stdout), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Version != 1 || len(schema.Commands) == 0 {
		t.Fatalf("unsupported/empty schema: %+v", schema)
	}
	sort.Slice(schema.Commands, func(i, j int) bool { return schema.Commands[i].Name < schema.Commands[j].Name })
	for i := range schema.Commands {
		sort.Slice(schema.Commands[i].Flags, func(a, b int) bool { return schema.Commands[i].Flags[a].Name < schema.Commands[i].Flags[b].Name })
	}
	return schema.Commands
}

type PairFixture struct {
	E                                *Env
	Project, Run, Bad, Array, Config string
	Jobs                             []string
}

func NewPairFixture(t *testing.T) PairFixture {
	t.Helper()
	e := NewEnv(t).WithVar("ROTARI_PROJECT_NAME", "pairs").WithVar("NO_COLOR", "1")
	f := PairFixture{E: e, Project: "pairs", Config: filepath.Join(e.Root, "pairs.yaml")}
	if err := os.WriteFile(f.Config, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := func(name string, args ...string) string {
		return AddedJobID(t, e.MustRotari(append([]string{"add", "--job-name", name}, args...)...))
	}
	add("ok", "--stage", "setup", "--", "sh", "-c", "echo ok; echo ok-error >&2")
	f.Bad = add("bad", "--stage", "training", "--", "sh", "-c", "echo bad; echo bad-error >&2; exit 3")
	f.Array = add("array", "--stage", "evaluation", "--array", "1-3", "--", "sh", "-c", `echo task-$ROTARI_ARRAY_TASK_ID; case "$ROTARI_ARRAY_TASK_ID" in 2) exit 7;; 3) exit 9;; esac`)
	e.MustRotari("add", "--job-name", "train", "--matrix", "SEED=1,2", "--stage", "training", "--", "sh", "-c", `echo seed-$SEED; test "$SEED" = 1`)
	r := e.Rotari("run", "--quiet")
	if r.Code != 1 {
		t.Fatalf("fixture should have failed jobs: %s", r)
	}
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	f.Run = shown.RunID
	for _, r := range shown.Summary.Results {
		f.Jobs = append(f.Jobs, r.ID)
	}
	if f.Run == "" || len(f.Jobs) != 7 {
		t.Fatalf("incomplete fixture: %+v", shown)
	}
	return f
}

func (f PairFixture) Sample(t *testing.T, flag PairFlag) []string {
	t.Helper()
	values := map[string]string{
		"config": f.Config, "basedir": f.E.Base, "project-name": f.Project,
		"masterdir": f.E.Master, "run-id": f.Run, "job-id": f.Bad, "job-name": "bad",
		"stage": "training", "filter-stage": "training", "matrix": "train", "filter-matrix": "train",
		"filter-not-stage": "setup", "filter-not-matrix": "train", "filter-command": "exit 3",
		"filter-exit-code": "7", "filter-failure-kind": "error", "filter-result": "failed",
		"filter-diagnosis": "CUDA/GPU memory exhausted", "filter-host": "*", "filter-started-after": "2000-01-01T00:00:00Z",
		"filter-started-before": "2000-01-01T00:00:00Z", "filter-finished-after": "2000-01-01T00:00:00Z",
		"filter-finished-before": "2000-01-01T00:00:00Z", "filter-longer-than": "1h", "filter-shorter-than": "1h",
		"stream": "stderr", "format": "%a %n", "since": "7d",
	}
	if flag.ValueName == "" {
		return []string{"--" + flag.Name + "=true"}
	}
	value, ok := values[flag.Name]
	if !ok {
		t.Fatalf("no valid sample for --%s", flag.Name)
	}
	if len(flag.Values) > 0 {
		valid := false
		for _, v := range flag.Values {
			valid = valid || value == v
		}
		if !valid {
			t.Fatalf("sample %q not in --%s values %q", value, flag.Name, flag.Values)
		}
	}
	return []string{"--" + flag.Name, value}
}

// Adapters own isolation and restoration for mutating commands. Finished runs
// ensure --follow exits; the deadline is a failure bound, never an expected result.
func PairInvoke(t *testing.T, e *Env, args ...string) Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := e.Command(args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var err error
	select {
	case err = <-done:
	case <-timer.C:
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("timeout: rotari %q\nstdout: %s\nstderr: %s", args, stdout.String(), stderr.String())
	}
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return Result{Args: args, Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func AssertPairOutcome(t *testing.T, r Result) {
	t.Helper()
	if strings.Contains(r.Stderr, "panic:") || strings.Contains(r.Stderr, "fatal error:") || strings.Contains(r.Stderr, "internal error") {
		t.Fatalf("internal failure: %s", r)
	}
	if r.Code == 0 {
		return
	}
	// Recognize only diagnosed option incompatibility, not arbitrary exit 1.
	text := r.Stderr
	if r.Code != 1 || (!strings.Contains(text, "cannot be combined") && !strings.Contains(text, "requires --") && !strings.Contains(text, "filter the job table") &&
		!strings.Contains(text, "--stream requires a job log") && !strings.Contains(text, "--filter-changed and --filter-new apply to the queue view")) {
		t.Fatalf("unclassified failure (not a usage rejection): %s", r)
	}
}

func PairHasFlag(flags []PairFlag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

type PairSavedFile struct {
	Path string
	Mode fs.FileMode
	At   time.Time
	Data []byte
	Dir  bool
}

// A byte-for-byte initial tree, including directory mtimes used by latest-run
// lookup. All paths are relative to the harness-owned temporary root. No active
// supervisor/job is allowed while this snapshot is captured or restored.
type PairSavedTree []PairSavedFile

func SavePairTree(t *testing.T, root string) PairSavedTree {
	t.Helper()
	var tree PairSavedTree
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !entry.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported fixture file %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file := PairSavedFile{Path: relative, Mode: info.Mode().Perm(), At: info.ModTime(), Dir: entry.IsDir()}
		if !file.Dir {
			file.Data, err = os.ReadFile(path)
		}
		if err == nil {
			tree = append(tree, file)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// Restore returns root to tree, given current, the snapshot taken after the
// last invocation. It rewrites only entries that differ, which matters on
// network filesystems where rewriting the whole tree per invocation dominated
// the runtime. Every entry's path, type, mode, and modification time is then
// checked against tree; contents are exact because unchanged entries had equal
// contents in current and changed ones are rewritten from tree.
func (tree PairSavedTree) Restore(t *testing.T, root string, current PairSavedTree) {
	t.Helper()
	want, have := tree.byPath(), current.byPath()
	touched := map[string]bool{}
	if err := removeAddedPairEntries(root, want, have, touched); err != nil {
		t.Fatal(err)
	}
	for _, file := range tree {
		if err := file.rewriteIfChanged(root, have, touched); err != nil {
			t.Fatal(err)
		}
	}
	// Reset times children first, so a parent's time is set after its last
	// child changed.
	for i := len(tree) - 1; i >= 0; i-- {
		file := tree[i]
		if old, ok := have[file.Path]; ok && !touched[file.Path] && old.At.Equal(file.At) {
			continue
		}
		if err := os.Chtimes(filepath.Join(root, file.Path), file.At, file.At); err != nil {
			t.Fatal(err)
		}
	}
	if got, wanted := statPairTree(t, root), tree.stats(); !reflect.DeepEqual(got, wanted) {
		t.Fatalf("fixture restoration differs:\n got %v\nwant %v", got, wanted)
	}
}

// removeAddedPairEntries removes the entries of have that want lacks, deepest
// first, and marks their parents touched.
func removeAddedPairEntries(root string, want, have map[string]PairSavedFile, touched map[string]bool) error {
	var extra []string
	for path := range have {
		if _, ok := want[path]; !ok {
			extra = append(extra, path)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(extra)))
	for _, path := range extra {
		if err := os.RemoveAll(filepath.Join(root, path)); err != nil {
			return err
		}
		touched[filepath.Dir(path)] = true
	}
	return nil
}

// rewriteIfChanged recreates file unless have holds it unchanged, and marks it
// and its parent touched. Callers pass entries parents first (walk order).
func (file PairSavedFile) rewriteIfChanged(root string, have map[string]PairSavedFile, touched map[string]bool) error {
	old, ok := have[file.Path]
	if ok && old.Dir == file.Dir && old.Mode == file.Mode && string(old.Data) == string(file.Data) {
		return nil
	}
	path := filepath.Join(root, file.Path)
	if ok && old.Dir != file.Dir {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	var err error
	if file.Dir {
		err = os.MkdirAll(path, file.Mode)
	} else {
		err = os.WriteFile(path, file.Data, file.Mode)
	}
	if err == nil {
		err = os.Chmod(path, file.Mode)
	}
	touched[file.Path], touched[filepath.Dir(file.Path)] = true, true
	return err
}

func (tree PairSavedTree) byPath() map[string]PairSavedFile {
	files := make(map[string]PairSavedFile, len(tree))
	for _, file := range tree {
		files[file.Path] = file
	}
	return files
}

// stats describes each entry by type, mode, size, and modification time.
func (tree PairSavedTree) stats() map[string]string {
	files := make(map[string]string, len(tree))
	for _, file := range tree {
		size := len(file.Data)
		if file.Dir {
			size = 0
		}
		files[file.Path] = fmt.Sprintf("%t:%o:%d:%d", file.Dir, file.Mode, size, file.At.UnixNano())
	}
	return files
}

func statPairTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		size := info.Size()
		if entry.IsDir() {
			size = 0
		}
		files[relative] = fmt.Sprintf("%t:%o:%d:%d", entry.IsDir(), info.Mode().Perm(), size, info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func (tree PairSavedTree) Raw() map[string]string {
	files := map[string]string{}
	for _, file := range tree {
		files[file.Path] = fmt.Sprintf("%t:%o:%x", file.Dir, file.Mode, sha256.Sum256(file.Data))
	}
	return files
}

// Only the project's newly written meta.updated_at is nondeterministic. All
// other content (IDs, queue definitions, results, registry, modes) stays exact.
func (tree PairSavedTree) Observation(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, file := range tree {
		data := file.Data
		if filepath.Base(file.Path) == "meta.json" {
			var meta map[string]json.RawMessage
			if err := json.Unmarshal(data, &meta); err != nil {
				t.Fatal(err)
			}
			delete(meta, "updated_at")
			var err error
			data, err = json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
		}
		files[file.Path] = fmt.Sprintf("%t:%o:%x", file.Dir, file.Mode, sha256.Sum256(data))
	}
	return files
}

type PairMutationFixture struct {
	PairFixture
	Initial PairSavedTree
	// Current is the tree as last observed after an invocation; Restore
	// diffs against it.
	Current   *PairSavedTree
	Revision  string
	SecondRun string
	Manifest  string
}

func NewPairMutationFixture(t *testing.T) PairMutationFixture {
	t.Helper()
	f := NewPairFixture(t)
	if r := f.E.Rotari("retry", "--quiet"); r.Code != 1 {
		t.Fatalf("expected failing retry: %s", r)
	}
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(f.E.MustRotari("show", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	f.E.MustRotari("copy", "--run-id", shown.RunID, "--quiet")
	f.E.MustRotari("add", "--job-name", "queue-only", "--stage", "setup", "--", "true")
	// Ensure all supervisors stopped before touching their state directories.
	stopped := f.E.Rotari("server", "shutdown")
	if stopped.Code != 0 && !(stopped.Code == 1 && strings.TrimSpace(stopped.Stderr) == "server is not running") {
		t.Fatalf("could not stop fixture supervisor: %s", stopped)
	}
	manifest := filepath.Join(f.E.Root, "pair-manifest.json")
	manifestBytes := []byte(`{"version":1,"jobs":[{"name":"import-added","command":["true"]}]}`)
	if err := os.WriteFile(manifest, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	initial := SavePairTree(t, f.E.Root)
	current := initial
	return PairMutationFixture{PairFixture: f, Initial: initial, Current: &current, Revision: PairMutationRevision(t, f.E), SecondRun: shown.RunID, Manifest: manifest}
}

func PairMutationRevision(t *testing.T, e *Env) string {
	t.Helper()
	r := PairInvoke(t, e, "check", "--json")
	var check struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &check); err != nil || check.Revision == "" || r.Code < 0 || r.Code > 1 {
		t.Fatalf("check did not report project revision: %s", r)
	}
	return check.Revision
}

type PairMutationResult struct {
	Process Result
	State   map[string]string
}

func PairIntersect(a, b []string) []string {
	in := map[string]bool{}
	for _, id := range b {
		in[id] = true
	}
	both := []string{}
	for _, id := range a {
		if in[id] {
			both = append(both, id)
		}
	}
	return both
}
