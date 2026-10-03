package interfaces

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type pairSavedFile struct {
	path string
	mode fs.FileMode
	at   time.Time
	data []byte
	dir  bool
}

// A byte-for-byte initial tree, including directory mtimes used by latest-run
// lookup. All paths are relative to the harness-owned temporary root. No active
// supervisor/job is allowed while this snapshot is captured or restored.
type pairSavedTree []pairSavedFile

func savePairTree(t *testing.T, root string) pairSavedTree {
	t.Helper()
	var tree pairSavedTree
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
		file := pairSavedFile{path: relative, mode: info.Mode().Perm(), at: info.ModTime(), dir: entry.IsDir()}
		if !file.dir {
			file.data, err = os.ReadFile(path)
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

// restore returns root to tree, given current, the snapshot taken after the
// last invocation. It rewrites only entries that differ, which matters on
// network filesystems where rewriting the whole tree per invocation dominated
// the runtime. Every entry's path, type, mode, and modification time is then
// checked against tree; contents are exact because unchanged entries had equal
// contents in current and changed ones are rewritten from tree.
func (tree pairSavedTree) restore(t *testing.T, root string, current pairSavedTree) {
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
		if old, ok := have[file.path]; ok && !touched[file.path] && old.at.Equal(file.at) {
			continue
		}
		if err := os.Chtimes(filepath.Join(root, file.path), file.at, file.at); err != nil {
			t.Fatal(err)
		}
	}
	if got, wanted := statPairTree(t, root), tree.stats(); !reflect.DeepEqual(got, wanted) {
		t.Fatalf("fixture restoration differs:\n got %v\nwant %v", got, wanted)
	}
}

// removeAddedPairEntries removes the entries of have that want lacks, deepest
// first, and marks their parents touched.
func removeAddedPairEntries(root string, want, have map[string]pairSavedFile, touched map[string]bool) error {
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
func (file pairSavedFile) rewriteIfChanged(root string, have map[string]pairSavedFile, touched map[string]bool) error {
	old, ok := have[file.path]
	if ok && old.dir == file.dir && old.mode == file.mode && string(old.data) == string(file.data) {
		return nil
	}
	path := filepath.Join(root, file.path)
	if ok && old.dir != file.dir {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	var err error
	if file.dir {
		err = os.MkdirAll(path, file.mode)
	} else {
		err = os.WriteFile(path, file.data, file.mode)
	}
	if err == nil {
		err = os.Chmod(path, file.mode)
	}
	touched[file.path], touched[filepath.Dir(file.path)] = true, true
	return err
}

func (tree pairSavedTree) byPath() map[string]pairSavedFile {
	files := make(map[string]pairSavedFile, len(tree))
	for _, file := range tree {
		files[file.path] = file
	}
	return files
}

// stats describes each entry by type, mode, size, and modification time.
func (tree pairSavedTree) stats() map[string]string {
	files := make(map[string]string, len(tree))
	for _, file := range tree {
		size := len(file.data)
		if file.dir {
			size = 0
		}
		files[file.path] = fmt.Sprintf("%t:%o:%d:%d", file.dir, file.mode, size, file.at.UnixNano())
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

func (tree pairSavedTree) raw() map[string]string {
	files := map[string]string{}
	for _, file := range tree {
		files[file.path] = fmt.Sprintf("%t:%o:%x", file.dir, file.mode, sha256.Sum256(file.data))
	}
	return files
}

// Only the project's newly written meta.updated_at is nondeterministic. All
// other content (IDs, queue definitions, results, registry, modes) stays exact.
func (tree pairSavedTree) observation(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, file := range tree {
		data := file.data
		if filepath.Base(file.path) == "meta.json" {
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
		files[file.path] = fmt.Sprintf("%t:%o:%x", file.dir, file.mode, sha256.Sum256(data))
	}
	return files
}

type pairMutationFixture struct {
	pairFixture
	initial pairSavedTree
	// current is the tree as last observed after an invocation; restore
	// diffs against it.
	current   *pairSavedTree
	revision  string
	secondRun string
	manifest  string
}

func newPairMutationFixture(t *testing.T) pairMutationFixture {
	t.Helper()
	f := newPairFixture(t)
	if r := f.e.Rotari("retry", "--quiet"); r.Code != 1 {
		t.Fatalf("expected failing retry: %s", r)
	}
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(f.e.MustRotari("show", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	f.e.MustRotari("copy", "--run-id", shown.RunID, "--quiet")
	f.e.MustRotari("add", "--job-name", "queue-only", "--stage", "setup", "--", "true")
	// Ensure all supervisors stopped before touching their state directories.
	stopped := f.e.Rotari("server", "shutdown")
	if stopped.Code != 0 && !(stopped.Code == 1 && strings.TrimSpace(stopped.Stderr) == "server is not running") {
		t.Fatalf("could not stop fixture supervisor: %s", stopped)
	}
	manifest := filepath.Join(f.e.Root, "pair-manifest.json")
	manifestBytes := []byte(`{"version":1,"jobs":[{"name":"import-added","command":["true"]}]}`)
	if err := os.WriteFile(manifest, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	initial := savePairTree(t, f.e.Root)
	current := initial
	return pairMutationFixture{pairFixture: f, initial: initial, current: &current, revision: pairMutationRevision(t, f.e), secondRun: shown.RunID, manifest: manifest}
}

func pairMutationRevision(t *testing.T, e *support.Env) string {
	t.Helper()
	r := pairInvoke(t, e, "check", "--json")
	var check struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &check); err != nil || check.Revision == "" || r.Code < 0 || r.Code > 1 {
		t.Fatalf("check did not report project revision: %s", r)
	}
	return check.Revision
}

func (f pairMutationFixture) args(t *testing.T, command string, flags []pairFlag) []string {
	t.Helper()
	if pairAdapter(command) == "edit" {
		return f.editArgs(t, command, flags)
	}
	args := []string{command}
	for _, flag := range flags {
		if flag.Name == "if-revision" {
			args = append(args, "--if-revision", f.revision)
		} else {
			args = append(args, f.sample(t, flag)...)
		}
	}
	// Supply a selector only if none is under test. Filters narrow --all;
	// filter-stage/filter-matrix already select their own scope.
	if command == "remove" && !pairMutationHasSelector(flags) {
		args = append(args, "--all")
	}
	if command == "delete" && !pairHasFlag(flags, "all") && !pairHasFlag(flags, "run-id") {
		args = append(args, "--run-id", f.run)
	}
	return args
}

func (f pairMutationFixture) editArgs(t *testing.T, command string, flags []pairFlag) []string {
	t.Helper()
	args := []string{command}
	for _, flag := range flags {
		args = append(args, pairEditSample(t, f, command, flag)...)
	}
	selector := pairMutationHasSelector(flags)
	modified := false
	for _, flag := range flags {
		modified = modified || pairEditMutationFlag(command, flag.Name)
	}
	switch command {
	case "add":
		args = append(args, "--", "true")
	case "change":
		if !selector {
			args = append(args, "--all")
		}
		if !modified {
			args = append(args, "--timeout", "17m")
		}
	case "copy":
		if !pairHasFlag(flags, "run-id") {
			args = append(args, "--run-id", f.run)
		}
		if !pairHasFlag(flags, "append") && !pairHasFlag(flags, "overwrite") {
			args = append(args, "--overwrite")
		}
	case "import":
		args = append(args[:1], append([]string{f.manifest}, args[1:]...)...)
		if !pairHasFlag(flags, "overwrite") {
			args = append(args, "--overwrite")
		}
	}
	return args
}

func pairEditMutationFlag(command, name string) bool {
	if command == "add" {
		return name != "basedir" && name != "project-name" && name != "config" && name != "quiet" && name != "dry-run" && name != "if-revision"
	}
	if command == "change" {
		if strings.HasPrefix(name, "filter-") {
			return false
		}
		switch name {
		case "basedir", "project-name", "config", "run-id", "job-id", "job-name", "stage", "matrix", "all", "quiet", "dry-run", "if-revision":
			return false
		default:
			return true
		}
	}
	return false
}

func pairEditSample(t *testing.T, f pairMutationFixture, command string, flag pairFlag) []string {
	t.Helper()
	values := map[string]string{
		"basedir": f.e.Base, "project-name": f.project, "config": f.config,
		"if-revision": f.revision,
		"run-id":      f.run, "job-id": f.bad, "job-name": "bad",
		"stage": "training", "matrix": "train", "filter-stage": "training", "filter-matrix": "train",
		"filter-command": "exit 3", "filter-not-stage": "training", "filter-not-matrix": "train",
		"executor": "local", "executor-option": "--partition=debug", "working-directory": f.e.Root,
		"env": "PAIR_VALUE=1", "output": filepath.Join(f.e.Root, "job-stdout.log"),
		"error": filepath.Join(f.e.Root, "job-stderr.log"), "log-mode": "separate", "open-mode": "truncate",
		"depends-on": "ok", "depends-on-finished": "ok", "timeout": "17m", "retry": "2",
		"retry-delay": "5s", "retry-backoff": "2", "retry-max-delay": "1m",
		"array": "4-6", "set-job-name": "renamed-pair", "status": "failed",
		"format": "json", "run-name": "pair-copy", "overwrite": "true", "partial-array": "false",
		"filter-exit-code": "3", "filter-failure-kind": "error", "filter-result": "failed",
		"filter-diagnosis": "CUDA/GPU memory exhausted", "filter-host": "*",
		"filter-started-after": "2000-01-01T00:00:00Z", "filter-started-before": "2000-01-01T00:00:00Z",
		"filter-finished-after": "2000-01-01T00:00:00Z", "filter-finished-before": "2000-01-01T00:00:00Z",
		"filter-longer-than": "1h", "filter-shorter-than": "1h",
	}
	if command == "add" && flag.Name == "matrix" {
		values[flag.Name] = "SEED=4,6"
	}
	if command == "add" && flag.Name == "job-name" {
		values[flag.Name] = "pair-added"
	}
	if flag.Name == "retry" && command == "change" {
		values[flag.Name] = "2"
	}
	if flag.ValueName == "" {
		return []string{"--" + flag.Name + "=true"}
	}
	value, ok := values[flag.Name]
	if !ok {
		t.Fatalf("no --%s sample for %s", flag.Name, command)
	}
	if len(flag.Values) > 0 {
		valid := false
		for _, option := range flag.Values {
			valid = valid || option == value
		}
		if !valid {
			t.Fatalf("sample %q not allowed for --%s (%q)", value, flag.Name, flag.Values)
		}
	}
	return []string{"--" + flag.Name, value}
}

func pairMutationHasSelector(flags []pairFlag) bool {
	for _, name := range []string{"all", "job-id", "job-name", "stage", "matrix", "filter-stage", "filter-matrix"} {
		if pairHasFlag(flags, name) {
			return true
		}
	}
	return false
}

type pairMutationResult struct {
	process support.Result
	state   map[string]string
}

func (f pairMutationFixture) invoke(t *testing.T, command string, flags []pairFlag) pairMutationResult {
	t.Helper()
	f.initial.restore(t, f.e.Root, *f.current)
	r := pairInvoke(t, f.e, f.args(t, command, flags)...)
	after := savePairTree(t, f.e.Root)
	*f.current = after
	if r.Code != 0 || pairHasFlag(flags, "dry-run") {
		if !reflect.DeepEqual(f.initial.raw(), after.raw()) {
			t.Fatalf("rejection/preview changed state: %s", r)
		}
	}
	assertPairMutationOutcome(t, command, flags, r)
	if r.Code == 0 && (command == "import" || pairHasFlag(flags, "if-revision") || pairHasFlag(flags, "dry-run")) {
		reported := pairReportedRevision(t, command, flags, r)
		if reported != pairMutationRevision(t, f.e) {
			t.Fatalf("guard revision differs from check: %s", r)
		}
		// Revision hashes incorporate meta.updated_at. Verify them against the
		// actual state first, then normalize only that line for order parity.
		r.Stdout = strings.ReplaceAll(r.Stdout, reported, "<verified-revision>")
	}
	if pairAdapter(command) == "edit" {
		return pairNormalizeEdit(t, f, r, after)
	}
	r.Args = nil
	return pairMutationResult{process: r, state: after.observation(t)}
}

func assertPairMutationOutcome(t *testing.T, command string, flags []pairFlag, r support.Result) {
	t.Helper()
	if pairAdapter(command) == "edit" {
		assertPairEditOutcome(t, command, flags, r)
		return
	}
	if r.Code == 1 && command == "remove" && pairMutationSelectorCount(flags) > 1 && strings.HasPrefix(r.Stderr, "usage: rotari remove ") {
		return
	}
	if r.Code == 1 && command == "delete" && pairHasFlag(flags, "all") && pairHasFlag(flags, "run-id") &&
		strings.TrimSpace(r.Stderr) == "pass a run ID to delete one run, or --all to delete every run of the project" {
		return
	}
	assertPairOutcome(t, r)
}

func assertPairEditOutcome(t *testing.T, command string, flags []pairFlag, r support.Result) {
	t.Helper()
	if strings.Contains(r.Stderr, "panic:") || strings.Contains(r.Stderr, "fatal error:") || strings.Contains(r.Stderr, "internal error") {
		t.Fatalf("internal failure: %s", r)
	}
	if r.Code == 0 {
		return
	}
	message := strings.TrimSpace(r.Stderr)
	if r.Code != 1 {
		t.Fatalf("unexpected exit status: %s", r)
	}
	if command == "change" && pairMutationSelectorCount(flags) > 1 && strings.HasPrefix(message, "usage: rotari change ") {
		return
	}
	if command == "copy" && pairHasFlag(flags, "append") && pairHasFlag(flags, "overwrite") && strings.HasPrefix(message, "usage: rotari copy ") {
		return
	}
	for _, expected := range []string{
		"cannot be combined",
		"depends on itself", "dependency cycle", "both depends_on and depends_on_finished",
		"a new command or --set-job-name needs a single job",
		"invalid dependencies: duplicate job name:", "invalid dependencies: duplicate matrix name:",
	} {
		if strings.Contains(message, expected) {
			return
		}
	}
	if command == "copy" && strings.HasPrefix(message, "run ") && strings.HasSuffix(message, " has no jobs matching selection") {
		return
	}
	t.Fatalf("unclassified edit-command failure: %s", r)
}

func pairMutationSelectorCount(flags []pairFlag) int {
	kinds := 0
	for _, group := range [][]string{{"all"}, {"job-id"}, {"job-name"}, {"stage", "filter-stage"}, {"matrix", "filter-matrix"}} {
		for _, name := range group {
			if pairHasFlag(flags, name) {
				kinds++
				break
			}
		}
	}
	return kinds
}

func TestCLIFlagPairMutations(t *testing.T) {
	start := time.Now()
	f := newPairMutationFixture(t)
	outcomes := [2]int{}
	for _, command := range readPairSchema(t, f.e) {
		if pairAdapter(command.Name) != "mutation" {
			continue
		}
		t.Run(command.Name, func(t *testing.T) {
			for _, pair := range commandFlagPairs(command) {
				t.Run(pair.a.Name+"+"+pair.b.Name, func(t *testing.T) {
					ab := f.invoke(t, command.Name, []pairFlag{pair.a, pair.b})
					ba := f.invoke(t, command.Name, []pairFlag{pair.b, pair.a})
					outcomes[ab.process.Code]++
					if !reflect.DeepEqual(ab, ba) {
						t.Fatalf("order-dependent mutation: %s\n%s\nstate equal=%t", ab.process, ba.process, reflect.DeepEqual(ab.state, ba.state))
					}
				})
			}
		})
	}
	t.Logf("mutation pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", outcomes[0], outcomes[1], 2*(outcomes[0]+outcomes[1]), time.Since(start))
}

func TestCLIFlagPairEditSamples(t *testing.T) {
	f := newPairMutationFixture(t)
	for _, command := range readPairSchema(t, f.e) {
		if pairAdapter(command.Name) != "edit" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(command.Name+"/"+flag.Name, func(t *testing.T) {
				f.invoke(t, command.Name, []pairFlag{flag})
			})
		}
	}
}

func TestCLIFlagPairEdits(t *testing.T) {
	start := time.Now()
	f := newPairMutationFixture(t)
	outcomes := [2]int{}
	for _, command := range readPairSchema(t, f.e) {
		if pairAdapter(command.Name) != "edit" {
			continue
		}
		t.Run(command.Name, func(t *testing.T) {
			for _, pair := range commandFlagPairs(command) {
				t.Run(pair.a.Name+"+"+pair.b.Name, func(t *testing.T) {
					ab := f.invoke(t, command.Name, []pairFlag{pair.a, pair.b})
					ba := f.invoke(t, command.Name, []pairFlag{pair.b, pair.a})
					outcomes[ab.process.Code]++
					if !reflect.DeepEqual(ab, ba) {
						t.Fatalf("order-dependent edit: %s\n%s\nstate equal=%t", ab.process, ba.process, reflect.DeepEqual(ab.state, ba.state))
					}
				})
			}
		})
	}
	t.Logf("edit pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", outcomes[0], outcomes[1], 2*(outcomes[0]+outcomes[1]), time.Since(start))
}

func pairQueuedIDs(t *testing.T, f pairMutationFixture) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.e.Base, "projects", f.project, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var queue struct {
		Commands []struct {
			ID string `json:"id"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, command := range queue.Commands {
		ids = append(ids, command.ID)
	}
	sort.Strings(ids)
	return ids
}

// pairApplied is the project as an explicit command leaves it, starting from
// the restored fixture.
type pairApplied struct {
	result support.Result
	queue  []string
	runs   []string
}

func (f pairMutationFixture) apply(t *testing.T, args ...string) pairApplied {
	t.Helper()
	f.initial.restore(t, f.e.Root, *f.current)
	r := pairInvoke(t, f.e, args...)
	*f.current = savePairTree(t, f.e.Root)
	if r.Code != 0 {
		t.Fatalf("witness command failed: %s", r)
	}
	entries, err := os.ReadDir(filepath.Join(f.e.Base, "projects", f.project, "runs"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	runs := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			runs = append(runs, entry.Name())
		}
	}
	return pairApplied{result: r, queue: pairQueuedIDs(t, f), runs: runs}
}

// removed lists the IDs of initial that are absent from after.
func pairRemoved(initial, after []string) []string {
	kept := map[string]bool{}
	for _, id := range after {
		kept[id] = true
	}
	removed := []string{}
	for _, id := range initial {
		if !kept[id] {
			removed = append(removed, id)
		}
	}
	return removed
}

func pairIntersect(a, b []string) []string {
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

// TestCLIFlagPairMutationObservability checks that each remove selector and
// definition filter has an effect, and that a selector and a filter combine as
// an intersection: the jobs removed by S plus F are those S removes that
// --all plus F also removes. An ignored selector or filter breaks the
// relation. It also checks the effects of reset and delete options.
func TestCLIFlagPairMutationObservability(t *testing.T) {
	covers(t, "SEL-7")
	f := newPairMutationFixture(t)
	initial := f.apply(t, "check", "--json").queue
	if len(initial) < 4 {
		t.Fatalf("fixture queue too small to distinguish selections: %v", initial)
	}
	r := pairRemoveWitness{f: f, initial: initial, all: pairRemoved(initial, f.apply(t, "remove", "--all", "--quiet").queue)}
	selectors := map[string][]string{
		"job-id": {"--job-id", f.bad}, "job-name": {"--job-name", "bad"},
		"stage": {"--stage", "training"}, "matrix": {"--matrix", "train"},
	}
	// --filter-stage and --filter-matrix are the long forms of --stage and
	// --matrix (SEL-11): selectors, not narrowing filters.
	for alias, short := range map[string]string{"filter-stage": "stage", "filter-matrix": "matrix"} {
		if got, want := r.removed(t, "--"+alias, selectors[short][1]), r.removed(t, selectors[short]...); !reflect.DeepEqual(got, want) {
			t.Errorf("--%s removed %v, its short form --%s removed %v", alias, got, short, want)
		}
	}
	filters := map[string][]string{
		"filter-command":    {"--filter-command", "exit 3"},
		"filter-not-stage":  {"--filter-not-stage", "training"},
		"filter-not-matrix": {"--filter-not-matrix", "train"},
	}
	byFilter := r.effects(t, filters, "--all")
	bySelector := r.effects(t, selectors)
	r.assertIntersections(t, selectors, filters, bySelector, byFilter)
	t.Run("remove/run-id", func(t *testing.T) {
		if got, current := r.removed(t, "--run-id", f.run, "--job-name", "bad"), r.removed(t, "--job-name", "bad"); reflect.DeepEqual(got, current) {
			t.Fatalf("--run-id did not restore that run's snapshot before removing: %v", got)
		}
	})
	t.Run("reset/quiet", func(t *testing.T) {
		loud, quiet := f.apply(t, "reset"), f.apply(t, "reset", "--quiet")
		if len(loud.queue) != 0 || len(quiet.queue) != 0 || loud.result.Stdout == "" || quiet.result.Stdout != "" ||
			!reflect.DeepEqual(loud.runs, quiet.runs) || len(loud.runs) == 0 {
			t.Fatalf("reset/quiet effects: loud %+v quiet %+v", loud, quiet)
		}
	})
	t.Run("delete/run-id-vs-all", func(t *testing.T) {
		before := f.apply(t, "check", "--json").runs
		one := f.apply(t, "delete", "--run-id", f.run).runs
		every := f.apply(t, "delete", "--all").runs
		if len(before) < 2 || len(one) != len(before)-1 || slices.Contains(one, f.run) || len(every) != 0 {
			t.Fatalf("delete selection: before %v, --run-id %v, --all %v", before, one, every)
		}
	})
}

// pairRemoveWitness observes which queued jobs `remove` takes out.
type pairRemoveWitness struct {
	f       pairMutationFixture
	initial []string
	all     []string
}

// removed applies remove with args from the restored fixture. It does not use
// the --run-id snapshot unless args name one, so it reports removals from the
// fixture's queue.
func (r pairRemoveWitness) removed(t *testing.T, args ...string) []string {
	t.Helper()
	return pairRemoved(r.initial, r.f.apply(t, append([]string{"remove", "--quiet"}, args...)...).queue)
}

// effects records what each option removes, with extra appended, and fails
// an option that removes nothing or everything --all removes.
func (r pairRemoveWitness) effects(t *testing.T, options map[string][]string, extra ...string) map[string][]string {
	t.Helper()
	removed := map[string][]string{}
	for name, args := range options {
		removed[name] = r.removed(t, append(append([]string{}, args...), extra...)...)
		if len(removed[name]) == 0 || reflect.DeepEqual(removed[name], r.all) {
			t.Errorf("--%s has no distinguishing effect: removed %v of %v", name, removed[name], r.all)
		}
	}
	return removed
}

// assertIntersections checks every selector with every filter, in both orders.
// Each option must be able to change some combination's result; otherwise the
// fixture cannot tell an ignored option from a working one.
func (r pairRemoveWitness) assertIntersections(t *testing.T, selectors, filters, bySelector, byFilter map[string][]string) {
	t.Helper()
	filterWitness, selectorWitness := map[string]bool{}, map[string]bool{}
	for sName, sArgs := range selectors {
		for fName, fArgs := range filters {
			want := pairIntersect(bySelector[sName], byFilter[fName])
			filterWitness[fName] = filterWitness[fName] || !reflect.DeepEqual(want, bySelector[sName])
			selectorWitness[sName] = selectorWitness[sName] || !reflect.DeepEqual(want, byFilter[fName])
			t.Run("remove/"+sName+"+"+fName, func(t *testing.T) {
				assertPairRemoveIntersection(t, r.f, r.initial, sArgs, fArgs, want)
			})
		}
	}
	assertPairWitnesses(t, "selector", filterWitness, filters)
	assertPairWitnesses(t, "filter", selectorWitness, selectors)
}

func assertPairWitnesses(t *testing.T, other string, witnessed map[string]bool, options map[string][]string) {
	t.Helper()
	for name := range options {
		if !witnessed[name] {
			t.Errorf("no %s combination would detect --%s being ignored", other, name)
		}
	}
}

func assertPairRemoveIntersection(t *testing.T, f pairMutationFixture, initial, sArgs, fArgs, want []string) {
	t.Helper()
	for _, args := range [][]string{append(append([]string{}, sArgs...), fArgs...), append(append([]string{}, fArgs...), sArgs...)} {
		got := pairRemoved(initial, f.apply(t, append([]string{"remove", "--quiet"}, args...)...).queue)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("remove %v removed %v, want selector ∩ filter %v", args, got, want)
		}
	}
}
