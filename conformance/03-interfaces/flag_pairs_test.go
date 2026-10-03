package interfaces

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type pairFlag struct {
	Name      string   `json:"name"`
	ValueName string   `json:"value_name"`
	Values    []string `json:"values"`
}

type pairCommand struct {
	Name  string     `json:"name"`
	Flags []pairFlag `json:"flags"`
}

type flagPair struct{ a, b pairFlag }

func commandFlagPairs(c pairCommand) []flagPair {
	var pairs []flagPair
	for i, a := range c.Flags {
		for _, b := range c.Flags[i+1:] {
			pairs = append(pairs, flagPair{a, b})
		}
	}
	return pairs
}

// Deferred commands remain visible, and a schema change requires an explicit
// coverage review. The fingerprint is of sorted flag names, not implementation.
var pairInventory = map[string]struct {
	count       int
	fingerprint string
}{
	"config":     {6, "cf7bcb9626d67065"},
	"check":      {6, "bc250836712a03be"},
	"reset":      {7, "e0dafcf28dbed25f"},
	"cancel":     {20, "65aeaf4b54d48db3"},
	"suspend":    {19, "4b2e628041f2b2eb"},
	"resume":     {19, "4b2e628041f2b2eb"},
	"delete":     {7, "cf61505b0fa4c0e7"},
	"gc":         {3, "4f8007fd85e880d5"},
	"unlock":     {4, "2d6ab0e22e6da38c"},
	"change":     {38, "9a6cb3c9eb91ec94"},
	"export":     {7, "36180bb4d09c2f4f"},
	"import":     {7, "32eb87a68c1692c7"},
	"remove":     {17, "83215b4518cebafa"},
	"show":       {39, "6ad0836313f8cf21"},
	"lineage":    {4, "03f8539f8880af76"},
	"jobs":       {7, "1e1d7aac6605a457"},
	"diagnose":   {11, "35f246a94a4dc9c5"},
	"wait":       {7, "9b5b3531f64908b2"},
	"add":        {25, "b5634affd2752931"},
	"copy":       {32, "25806ce2d014cab0"},
	"run":        {61, "e862f91f6923f288"},
	"retry":      {55, "625765893217c167"},
	"server":     {3, "1bc5f18bd9dd276d"},
	"web":        {8, "a4bffa924ab6363c"},
	"mcp":        {2, "c90df8ffe2985354"},
	"completion": {0, "e3b0c44298fc1c14"},
	"schema":     {1, "02bd175f32972037"},
	"guide":      {0, "e3b0c44298fc1c14"},
	"version":    {0, "e3b0c44298fc1c14"},
	"env":        {0, "e3b0c44298fc1c14"},
}

func readPairSchema(t *testing.T, e *support.Env) []pairCommand {
	t.Helper()
	var schema struct {
		Version  int           `json:"version"`
		Commands []pairCommand `json:"commands"`
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

func TestCLIFlagPairInventory(t *testing.T) {
	e := support.NewEnv(t)
	commands := readPairSchema(t, e)
	seen := map[string]bool{}
	total, executable := 0, 0
	for _, c := range commands {
		seen[c.Name] = true
		var names []string
		for _, f := range c.Flags {
			names = append(names, f.Name)
		}
		fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(names, " "))))[:16]
		want, ok := pairInventory[c.Name]
		if !ok || want.count != len(names) || want.fingerprint != fingerprint {
			t.Errorf("%s: schema changed; review samples/coverage: count=%d fingerprint=%s", c.Name, len(names), fingerprint)
		}
		pairs := len(commandFlagPairs(c))
		total += pairs
		coverage := "deferred: needs isolated command/subcommand adapter; see flag-pair coverage document"
		if c.Name == "show" || c.Name == "jobs" {
			executable += pairs
			coverage = "robustness/order; semantic witnesses separately"
		}
		t.Logf("%s: flags=%d pairs=%d %s", c.Name, len(names), pairs, coverage)
	}
	for name := range pairInventory {
		if !seen[name] {
			t.Errorf("stale inventory command %s", name)
		}
	}
	validatePairEquivalences(t, commands)
	t.Logf("pairs: generated=%d executable=%d deferred=%d", total, executable, total-executable)
}

type pairFixture struct {
	e                                *support.Env
	project, run, bad, array, config string
	jobs                             []string
}

func newPairFixture(t *testing.T) pairFixture {
	t.Helper()
	e := support.NewEnv(t).WithVar("ROTARI_PROJECT_NAME", "pairs").WithVar("NO_COLOR", "1")
	f := pairFixture{e: e, project: "pairs", config: filepath.Join(e.Root, "pairs.yaml")}
	if err := os.WriteFile(f.config, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := func(name string, args ...string) string {
		return support.AddedJobID(t, e.MustRotari(append([]string{"add", "--job-name", name}, args...)...))
	}
	add("ok", "--stage", "setup", "--", "sh", "-c", "echo ok; echo ok-error >&2")
	f.bad = add("bad", "--stage", "training", "--", "sh", "-c", "echo bad; echo bad-error >&2; exit 3")
	f.array = add("array", "--stage", "evaluation", "--array", "1-3", "--", "sh", "-c", `echo task-$ROTARI_ARRAY_TASK_ID; case "$ROTARI_ARRAY_TASK_ID" in 2) exit 7;; 3) exit 9;; esac`)
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
	f.run = shown.RunID
	for _, r := range shown.Summary.Results {
		f.jobs = append(f.jobs, r.ID)
	}
	if f.run == "" || len(f.jobs) != 7 {
		t.Fatalf("incomplete fixture: %+v", shown)
	}
	return f
}

func (f pairFixture) sample(t *testing.T, flag pairFlag) []string {
	t.Helper()
	values := map[string]string{
		"config": f.config, "basedir": f.e.Base, "project-name": f.project,
		"masterdir": f.e.Master, "run-id": f.run, "job-id": f.bad, "job-name": "bad",
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

// Only read-only commands use this helper. Finished runs ensure --follow exits;
// the deadline is a failure bound, never an expected result.
func pairInvoke(t *testing.T, e *support.Env, args ...string) support.Result {
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
	return support.Result{Args: args, Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func assertPairOutcome(t *testing.T, r support.Result) {
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

func TestCLIFlagPairs(t *testing.T) {
	start := time.Now()
	f := newPairFixture(t)
	// Supply a real queue as well as history, so queue-scoped samples resolve
	// existing stages/matrices rather than failing on missing fixture data.
	f.e.MustRotari("copy", "--run-id", f.run, "--quiet")
	invocations := 0
	outcomes := [2]int{} // assertPairOutcome permits only success (0) or rejection (1).
	for _, c := range readPairSchema(t, f.e) {
		if c.Name != "show" && c.Name != "jobs" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			for _, pair := range commandFlagPairs(c) {
				t.Run(pair.a.Name+"+"+pair.b.Name, func(t *testing.T) {
					aArgs, bArgs := f.sample(t, pair.a), f.sample(t, pair.b)
					ab := pairInvoke(t, f.e, append(append([]string{c.Name}, aArgs...), bArgs...)...)
					ba := pairInvoke(t, f.e, append(append([]string{c.Name}, bArgs...), aArgs...)...)
					invocations += 2
					assertPairOutcome(t, ab)
					assertPairOutcome(t, ba)
					outcomes[ab.Code]++
					if ab.Code != ba.Code || ab.Stdout != ba.Stdout || ab.Stderr != ba.Stderr {
						t.Fatalf("order-dependent result:\n%s\n%s", ab, ba)
					}
				})
			}
		})
	}
	t.Logf("pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s (includes fixture/schema)", outcomes[0], outcomes[1], invocations, time.Since(start))
}

func pairShownIDs(t *testing.T, f pairFixture, r support.Result, mode string) []string {
	t.Helper()
	var ids []string
	if mode == "json" {
		ids = pairJSONIDs(t, r)
	} else {
		ids = pairTextIDs(f, r.Stdout, mode)
	}
	sort.Strings(ids)
	return ids
}

func pairJSONIDs(t *testing.T, r support.Result) []string {
	t.Helper()
	type jobID struct {
		ID string `json:"id"`
	}
	var shown struct {
		Summary *struct {
			Results []jobID `json:"results"`
		} `json:"summary"`
		Jobs []struct {
			Job jobID `json:"job"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &shown); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, r)
	}
	if shown.Summary == nil && shown.Jobs == nil {
		t.Fatalf("JSON has no summary or jobs projection: %s", r)
	}
	ids := []string{}
	if shown.Summary != nil {
		for _, result := range shown.Summary.Results {
			ids = append(ids, result.ID)
		}
	}
	for _, job := range shown.Jobs {
		ids = append(ids, job.Job.ID)
	}
	return ids
}

func pairTextIDs(f pairFixture, text, mode string) []string {
	known := map[string]bool{}
	for _, id := range f.jobs {
		known[id] = true
	}
	ids := []string{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(pairIDLine(line, mode))
		if len(fields) > 0 && (mode != "table" || known[fields[0]]) {
			ids = append(ids, fields[0])
		}
	}
	return ids
}

func pairIDLine(line, mode string) string {
	switch mode {
	case "logs", "failed-logs":
		if rest, ok := strings.CutPrefix(line, "=== Job: "); ok {
			return rest
		}
		return ""
	case "report":
		if rest, ok := strings.CutPrefix(line, "- Job ID: `"); ok {
			return strings.TrimSuffix(rest, "`")
		}
		return ""
	default:
		return line
	}
}

type pairEquivalence struct{ mode, selector, reason string }

// Narrow to this fixture's failed value; these entries do NOT skip selection
// equality, rejection, or robustness checks, only baseline-effect detection.
var pairEquivalences = []pairEquivalence{
	{"failed-logs", "failed", "--failed-logs already selects failed jobs (docs/INSPECT.md); --failed is redundant"},
	{"failed-logs", "filter-result", "sample is failed: the documented long form of --failed is redundant with --failed-logs"},
}

func TestCLIFlagPairObservability(t *testing.T) {
	covers(t, "SEL-11")
	f := newPairFixture(t)
	// Output modes x selectors: the table is an independent semantic witness. Most
	// combinations reject explicitly today; do not allow equal/empty output to
	// count as evidence that a selector works.
	for _, c := range readPairSchema(t, f.e) {
		if c.Name != "show" {
			continue
		}
		selectors := slices.DeleteFunc(slices.Clone(c.Flags), func(f pairFlag) bool { return !pairObservableSelector(f.Name) })
		for _, selector := range selectors {
			for _, mode := range []string{"json", "logs", "failed-logs", "report"} {
				t.Run("show/"+mode+"+"+selector.Name, func(t *testing.T) {
					assertPairObservable(t, f, selector, mode)
				})
			}
		}
	}
	t.Run("jobs/format+since", func(t *testing.T) {
		all := pairInvoke(t, f.e, "jobs", "--format", "%a %n", "--since", "7d")
		running := pairInvoke(t, f.e, "jobs", "--format", "%a %n", "--since", "0")
		if all.Code != 0 || running.Code != 0 || !strings.Contains(all.Stdout, "att_") || strings.Contains(running.Stdout, "att_") {
			t.Fatalf("--since has no effect across formatted output:\n%s\n%s", all, running)
		}
		if !strings.HasPrefix(all.Stdout, "ATTEMPT_ID") {
			t.Fatalf("--format ignored: %s", all)
		}
	})
}

func pairObservableSelector(name string) bool {
	switch name {
	case "failed", "success", "unfinished", "stage", "matrix":
		return true
	}
	return strings.HasPrefix(name, "filter-")
}

func pairEquivalenceReason(mode, selector string) string {
	for _, entry := range pairEquivalences {
		if entry.mode == mode && entry.selector == selector {
			return entry.reason
		}
	}
	return ""
}

func assertPairObservable(t *testing.T, f pairFixture, selector pairFlag, mode string) {
	t.Helper()
	reason := pairEquivalenceReason(mode, selector.Name)
	base := []string{"show", "--run-id", f.run}
	args := f.sample(t, selector)
	plain := pairInvoke(t, f.e, append(append([]string{}, base...), args...)...)
	assertPairOutcome(t, plain)
	combined := pairInvoke(t, f.e, append(append(append([]string{}, base...), "--"+mode), args...)...)
	assertPairOutcome(t, combined)
	if combined.Code != 0 {
		if reason != "" {
			t.Fatalf("stale equivalence: pair now rejects: %s", combined)
		}
		t.Logf("explicit rejection: %s", strings.TrimSpace(combined.Stderr))
		return // diagnosed rejection is not silent ignore
	}
	if plain.Code != 0 {
		t.Fatalf("pair accepted a selector without a valid table witness: %s", plain)
	}
	all := pairShownIDs(t, f, pairInvoke(t, f.e, base...), "table")
	selected := pairShownIDs(t, f, plain, "table")
	if len(all) == 0 {
		t.Fatal("fixture has no table jobs to select")
	}
	if reflect.DeepEqual(all, selected) {
		t.Fatalf("observation gap: --%s has no distinguishing table witness", selector.Name)
	}
	got := pairShownIDs(t, f, combined, mode)
	if !reflect.DeepEqual(got, selected) {
		t.Fatalf("silent-ignore/selection mismatch: table=%q %s=%q\n%s\n%s", selected, mode, got, plain, combined)
	}
	unfiltered := pairInvoke(t, f.e, append(append([]string{}, base...), "--"+mode)...)
	assertPairOutcome(t, unfiltered)
	assertPairBaselineEffect(t, mode, selector.Name, reason, reflect.DeepEqual(got, pairShownIDs(t, f, unfiltered, mode)))
}

func validatePairEquivalences(t *testing.T, commands []pairCommand) {
	t.Helper()
	flags := map[string]bool{}
	for _, c := range commands {
		if c.Name == "show" {
			for _, f := range c.Flags {
				flags[f.Name] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, entry := range pairEquivalences {
		key := entry.mode + "+" + entry.selector
		if seen[key] || entry.reason == "" || !flags[entry.mode] || !flags[entry.selector] || !pairObservableSelector(entry.selector) {
			t.Errorf("invalid/stale equivalence entry: %+v", entry)
		}
		seen[key] = true
	}
}

func TestCLIFlagPairSamples(t *testing.T) {
	f := newPairFixture(t)
	f.e.MustRotari("copy", "--run-id", f.run, "--quiet")
	for _, c := range readPairSchema(t, f.e) {
		if c.Name != "show" && c.Name != "jobs" {
			continue
		}
		for _, flag := range c.Flags {
			t.Run(c.Name+"/"+flag.Name, func(t *testing.T) {
				assertPairOutcome(t, pairInvoke(t, f.e, append([]string{c.Name}, f.sample(t, flag)...)...))
				if flag.ValueName == "" {
					assertPairOutcome(t, pairInvoke(t, f.e, c.Name, "--"+flag.Name+"=false"))
				}
			})
		}
	}
	for _, name := range []string{"filter-result", "filter-exit-code"} {
		t.Run("show/repeated-"+name, func(t *testing.T) {
			values := []string{"7", "9"}
			if name == "filter-result" {
				values = []string{"failed", "success"}
			}
			assertPairOutcome(t, pairInvoke(t, f.e, "show", "--"+name, values[0], "--"+name, values[1]))
		})
	}
}

func assertPairBaselineEffect(t *testing.T, mode, selector, reason string, equal bool) {
	t.Helper()
	if equal {
		if reason == "" {
			t.Fatalf("observation gap: %s+%s equals mode baseline without a reason", mode, selector)
		}
		t.Log(reason)
	} else if reason != "" {
		t.Fatalf("stale equivalence: %s+%s no longer equals baseline", mode, selector)
	}
}
