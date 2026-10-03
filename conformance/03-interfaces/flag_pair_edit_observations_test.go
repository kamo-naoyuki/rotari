package interfaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func pairReportedRevision(t *testing.T, command string, flags []pairFlag, r support.Result) string {
	t.Helper()
	if command != "import" || !pairHasFlag(flags, "json") {
		return revisionOf(t, r)
	}
	var plan struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &plan); err != nil || plan.Revision == "" {
		t.Fatalf("import plan missing revision: %s", r)
	}
	return plan.Revision
}

// Assign a separate stable token to each NEW command/group ID in traversal
// order. Existing fixture IDs remain literal, so copying the wrong source or
// merging two IDs cannot be hidden by normalization. Job definitions, task
// selections and provenance are otherwise unchanged.
func pairNormalizeEdit(t *testing.T, f pairMutationFixture, r support.Result, after pairSavedTree) pairMutationResult {
	t.Helper()
	known := map[string]bool{}
	for _, id := range pairTreeIDs(t, f.initial) {
		known[id] = true
	}
	ids := pairTreeIDs(t, after)
	// A preview does not save its proposed IDs. Add reports its one job_id,
	// and import reports a structured plan; both are externally observable.
	for _, match := range regexp.MustCompile(`(?:job_id=|"id"\s*:\s*"|job ")([0-9a-f]{9})`).FindAllStringSubmatch(r.Stdout+r.Stderr, -1) {
		ids = append(ids, match[1])
	}
	replacements := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if known[id] || seen[id] {
			continue
		}
		seen[id] = true
		replacements = append(replacements, id, fmt.Sprintf("new-id-%d", len(seen)))
	}
	replacer := strings.NewReplacer(replacements...)
	normalized := make(pairSavedTree, len(after))
	copy(normalized, after)
	for i, file := range normalized {
		if filepath.Base(file.path) == "queue.json" {
			normalized[i].data = []byte(replacer.Replace(string(file.data)))
		}
	}
	r.Stdout, r.Stderr = replacer.Replace(r.Stdout), replacer.Replace(r.Stderr)
	r.Args = nil
	return pairMutationResult{process: r, state: normalized.observation(t)}
}

func pairTreeIDs(t *testing.T, tree pairSavedTree) []string {
	t.Helper()
	var ids []string
	for _, file := range tree {
		if filepath.Base(file.path) != "queue.json" && filepath.Base(file.path) != "commands.json" {
			continue
		}
		var queue struct {
			Commands []struct {
				ID     string `json:"id"`
				Matrix *struct {
					GroupID string `json:"group_id"`
				} `json:"matrix"`
			} `json:"commands"`
		}
		if err := json.Unmarshal(file.data, &queue); err != nil {
			t.Fatal(err)
		}
		for _, command := range queue.Commands {
			ids = append(ids, command.ID)
			if command.Matrix != nil {
				ids = append(ids, command.Matrix.GroupID)
			}
		}
	}
	return ids
}

func pairEditQueue(t *testing.T, f pairMutationFixture) []map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.e.Base, "projects", f.project, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var queue struct {
		Commands []map[string]json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		t.Fatal(err)
	}
	return queue.Commands
}

func pairAddDefinition(t *testing.T, f pairMutationFixture, flags []pairFlag) map[string]json.RawMessage {
	t.Helper()
	f.invoke(t, "add", flags)
	commands := pairEditQueue(t, f)
	if len(commands) == 0 {
		t.Fatal("add produced no commands")
	}
	command := commands[len(commands)-1]
	if matrix, ok := command["matrix"]; ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(matrix, &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, "group_id") // Allocation is checked by the pair normalizer.
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		command["matrix"] = data
	}
	return command
}

// Non-default settings and output destinations must survive an output-mode
// option. A accepted-but-ignored setting leaves the added definition unchanged.
func TestCLIFlagPairAddObservability(t *testing.T) {
	f := newPairMutationFixture(t)
	base := pairAddDefinition(t, f, nil)
	fields := map[string]string{
		"job-name": "name", "stage": "stage", "array": "array", "matrix": "matrix",
		"env": "environment", "output": "output", "error": "error", "log-mode": "log_mode", "open-mode": "open_mode",
		"working-directory": "working_directory", "executor": "executor", "executor-option": "executor_options",
		"depends-on": "depends_on", "depends-on-finished": "depends_on_finished", "timeout": "timeout", "retry": "retry",
		"retry-delay": "retry_delay", "retry-backoff": "retry_backoff", "retry-max-delay": "retry_max_delay",
	}
	for _, command := range readPairSchema(t, f.e) {
		if command.Name != "add" {
			continue
		}
		for _, flag := range command.Flags {
			field, ok := fields[flag.Name]
			if !ok {
				continue
			}
			t.Run(flag.Name+"+quiet", func(t *testing.T) {
				alone := pairAddDefinition(t, f, []pairFlag{flag})
				combined := pairAddDefinition(t, f, []pairFlag{flag, {Name: "quiet"}})
				if len(alone[field]) == 0 || reflect.DeepEqual(alone[field], base[field]) || !reflect.DeepEqual(alone[field], combined[field]) {
					t.Fatalf("--%s effect missing/changed with quiet: baseline=%s alone=%s combined=%s", flag.Name, base[field], alone[field], combined[field])
				}
			})
		}
	}
}

func pairCopyNames(t *testing.T, f pairMutationFixture, args ...string) []string {
	t.Helper()
	f.initial.restore(t, f.e.Root, *f.current)
	r := pairInvoke(t, f.e, append([]string{"copy", "--run-id", f.run, "--overwrite", "--quiet"}, args...)...)
	*f.current = savePairTree(t, f.e.Root)
	if r.Code != 0 {
		t.Fatalf("copy witness failed: %s", r)
	}
	names := []string{}
	for _, command := range pairEditQueue(t, f) {
		var name string
		if err := json.Unmarshal(command["name"], &name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestCLIFlagPairCopyObservability(t *testing.T) {
	covers(t, "SEL-4")
	f := newPairMutationFixture(t)
	all := pairCopyNames(t, f)
	failed := pairCopyNames(t, f, "--failed")
	training := pairCopyNames(t, f, "--stage", "training")
	want := pairIntersect(training, failed)
	if len(want) == 0 || reflect.DeepEqual(want, failed) || reflect.DeepEqual(want, training) {
		t.Fatal("copy fixture cannot distinguish stage and result selection")
	}
	for _, flags := range [][]string{{"--stage", "training", "--failed"}, {"--failed", "--stage", "training"}, {"--filter-result", "failed", "--filter-stage", "training"}} {
		got := pairCopyNames(t, f, flags...)
		if !reflect.DeepEqual(got, want) || reflect.DeepEqual(got, all) {
			t.Fatalf("copy %v selected %v, want %v", flags, got, want)
		}
	}
}

func TestCLIFlagPairImportObservability(t *testing.T) {
	covers(t, "CLI-7")
	f := newPairMutationFixture(t)
	before := f.initial.raw()
	preview := f.invoke(t, "import", []pairFlag{{Name: "json"}, {Name: "dry-run"}})
	var plan struct {
		Jobs []struct {
			Name string `json:"name"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(preview.process.Stdout), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 1 || plan.Jobs[0].Name != "import-added" || !reflect.DeepEqual(before, savePairTree(t, f.e.Root).raw()) {
		t.Fatal("JSON preview lost jobs or mutated state")
	}
	applied := f.invoke(t, "import", []pairFlag{{Name: "json"}, {Name: "overwrite"}})
	queue := pairEditQueue(t, f)
	if applied.process.Code != 0 || len(queue) != 1 || string(queue[0]["name"]) != `"import-added"` {
		t.Fatalf("import JSON/overwrite lost effect: %s", applied.process)
	}
}

func pairChangeDefinition(t *testing.T, f pairMutationFixture, options ...string) map[string]json.RawMessage {
	t.Helper()
	f.initial.restore(t, f.e.Root, *f.current)
	r := pairInvoke(t, f.e, append([]string{"change", "--job-id", f.bad}, options...)...)
	*f.current = savePairTree(t, f.e.Root)
	if r.Code != 0 {
		t.Fatalf("change witness failed: %s", r)
	}
	for _, command := range pairEditQueue(t, f) {
		if string(command["id"]) == `"`+f.bad+`"` {
			return command
		}
	}
	t.Fatal("change lost the selected job")
	return nil
}

func TestCLIFlagPairChangeObservability(t *testing.T) {
	f := newPairMutationFixture(t)
	fields := map[string]string{"timeout": "timeout", "retry": "retry", "env": "environment", "executor-option": "executor_options", "working-directory": "working_directory", "depends-on": "depends_on", "depends-on-finished": "depends_on_finished", "status": "marked_status", "set-job-name": "name", "executor": "executor"}
	base := pairChangeDefinition(t, f, "--quiet", "--timeout", "1m")
	for flag, field := range fields {
		t.Run(flag+"+quiet", func(t *testing.T) {
			sample := pairEditSample(t, f, "change", pairFlag{Name: flag, ValueName: "VALUE"})
			alone := pairChangeDefinition(t, f, sample...)
			combined := pairChangeDefinition(t, f, append(sample, "--quiet")...)
			if len(alone[field]) == 0 || !reflect.DeepEqual(alone[field], combined[field]) {
				t.Fatalf("change --%s lost effect with quiet: %s / %s", flag, alone[field], combined[field])
			}
			if flag != "executor" && reflect.DeepEqual(alone[field], base[field]) {
				t.Fatalf("no distinguishing --%s witness", flag)
			}
		})
	}
}

func TestCLIFlagPairIDNormalization(t *testing.T) {
	queue := func(ids ...string) pairSavedTree {
		var commands []map[string]string
		for _, id := range ids {
			commands = append(commands, map[string]string{"id": id, "name": "same"})
		}
		data, _ := json.Marshal(map[string]any{"commands": commands})
		return pairSavedTree{{path: "queue.json", data: data}}
	}
	f := pairMutationFixture{initial: queue("existing")}
	a := pairNormalizeEdit(t, f, support.Result{}, queue("existing", "111111111", "222222222"))
	b := pairNormalizeEdit(t, f, support.Result{}, queue("existing", "333333333", "444444444"))
	c := pairNormalizeEdit(t, f, support.Result{}, queue("existing", "333333333", "333333333"))
	d := pairNormalizeEdit(t, f, support.Result{}, queue("wrong-source", "333333333", "444444444"))
	if !reflect.DeepEqual(a, b) || reflect.DeepEqual(a, c) || reflect.DeepEqual(a, d) {
		t.Fatal("ID normalization masked source identity or collision")
	}
}
