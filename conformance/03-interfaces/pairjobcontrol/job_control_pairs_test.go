package pairjobcontrol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

const controlProject = "pairs"

// controlKeys are the five live jobs of a control run: a two-task array in
// stage batch, a single job in stage single, and a two-member matrix grid in
// stage matrix. Every selector sample selects a distinguishing subset.
var controlKeys = []string{"grid-SEED1", "grid-SEED2", "hold-1", "hold-2", "idle"}

// controlRun is a live run whose jobs sleep until the test ends.
type controlRun struct {
	e        *support.Env
	config   string
	runID    string
	ids      map[string]string // key -> job ID, plus "hold" for the array
	attempts map[string]string // key -> attempt directory
}

func startControlRun(t *testing.T) controlRun {
	t.Helper()
	e := support.NewEnv(t).WithVar("ROTARI_PROJECT_NAME", controlProject).WithVar("NO_COLOR", "1")
	r := controlRun{e: e, config: filepath.Join(e.Root, "pairs.yaml"), ids: map[string]string{}, attempts: map[string]string{}}
	if err := os.WriteFile(r.config, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.ids["hold"] = support.AddedJobID(t, e.MustRotari("add", "--job-name", "hold", "--stage", "batch", "--array", "1-2", "--", "sleep", "300"))
	e.MustRotari("add", "--job-name", "idle", "--stage", "single", "--", "sleep", "301")
	e.MustRotari("add", "--job-name", "grid", "--stage", "matrix", "--matrix", "SEED=1,2", "--", "sleep", "302")
	client := e.Command("run", "--quiet")
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A suspended job cannot act on the cancel, so resume first.
		_ = e.Command("resume").Run()
		// Disconnecting the synchronous client cancels its run.
		_ = client.Process.Kill()
		_ = client.Wait()
		_ = e.Command("wait", "--timeout", "30s").Run()
		support.KillStrays(t, e.Root)
	})
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		running := strings.Count(e.Rotari("jobs", controlProject, "--format", "%a %s").Stdout, " running")
		return running == len(controlKeys), fmt.Sprintf("running=%d, want %d", running, len(controlKeys))
	})
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "--json").Stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("live run ID: %v", err)
	}
	r.runID = shown.RunID
	dirs, err := filepath.Glob(filepath.Join(e.Base, "projects", controlProject, "runs", r.runID, "*", "attempts", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		var spec struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		data, err := os.ReadFile(filepath.Join(dir, "command.json"))
		if err != nil || json.Unmarshal(data, &spec) != nil {
			t.Fatalf("attempt %s: %v", dir, err)
		}
		key := strings.NewReplacer("[", "-", "]", "").Replace(spec.Name)
		r.attempts[key], r.ids[key] = dir, spec.ID
	}
	if len(r.attempts) != len(controlKeys) {
		t.Fatalf("live attempts = %v, want %v", r.attempts, controlKeys)
	}
	return r
}

func (r controlRun) sample(t *testing.T, flag pairFlag) []string {
	t.Helper()
	if flag.ValueName == "" {
		return []string{"--" + flag.Name + "=true"}
	}
	values := map[string]string{
		"config": r.config, "basedir": r.e.Base, "project-name": controlProject,
		"job-id": r.ids["idle"], "job-name": "idle",
		"stage": "single", "filter-stage": "single", "filter-not-stage": "batch",
		"matrix": "grid", "filter-matrix": "grid", "filter-not-matrix": "grid",
		"filter-state": "running", "filter-host": "*", "filter-command": "sleep",
		"filter-started-after": "2000-01-01T00:00:00Z", "filter-started-before": "2099-01-01T00:00:00Z",
		"filter-longer-than": "1ns", "filter-shorter-than": "8760h",
	}
	value, ok := values[flag.Name]
	if !ok {
		t.Fatalf("no job-control sample for --%s", flag.Name)
	}
	if len(flag.Values) > 0 && !slices.Contains(flag.Values, value) {
		t.Fatalf("sample %q outside --%s values %q", value, flag.Name, flag.Values)
	}
	return []string{"--" + flag.Name, value}
}

// args injects --yes unless the pair supplies it: without a terminal a
// filtered selection otherwise stops at confirmation, which the selector
// table covers separately.
func (r controlRun) args(t *testing.T, command string, flags []pairFlag) []string {
	t.Helper()
	args := []string{command}
	for _, flag := range flags {
		args = append(args, r.sample(t, flag)...)
	}
	if !hasFlag(flags, "yes") {
		args = append(args, "--yes")
	}
	return args
}

func hasFlag(flags []pairFlag, name string) bool {
	return slices.ContainsFunc(flags, func(flag pairFlag) bool { return flag.Name == name })
}

// sampleKeys is the subset of live jobs a flag's sample selects on its own.
func sampleKeys(name string) []string {
	switch name {
	case "job-id", "job-name", "stage", "filter-stage":
		return []string{"idle"}
	case "filter-not-stage":
		return []string{"grid-SEED1", "grid-SEED2", "idle"}
	case "matrix", "filter-matrix":
		return []string{"grid-SEED1", "grid-SEED2"}
	case "filter-not-matrix":
		return []string{"hold-1", "hold-2", "idle"}
	}
	return controlKeys
}

func isFilter(name string) bool {
	return name == "stage" || name == "matrix" || strings.HasPrefix(name, "filter-")
}

// expectedControl is an independent model of the contract (SEL-8, SEL-12):
// the jobs the command acts on, or a substring of its diagnosed rejection.
func expectedControl(command string, flags []pairFlag) ([]string, string) {
	direct := hasFlag(flags, "job-id") || hasFlag(flags, "job-name")
	filtered := slices.ContainsFunc(flags, func(flag pairFlag) bool { return isFilter(flag.Name) })
	switch {
	case hasFlag(flags, "job-id") && hasFlag(flags, "job-name"),
		direct && filtered,
		(hasFlag(flags, "stage") || hasFlag(flags, "filter-stage")) && (hasFlag(flags, "matrix") || hasFlag(flags, "filter-matrix")):
		return nil, "cannot be combined"
	case command == "cancel" && hasFlag(flags, "wait") && (direct || filtered):
		return nil, "--wait may not be used with a job selection"
	}
	keys := controlKeys
	for _, flag := range flags {
		keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return !slices.Contains(sampleKeys(flag.Name), key) })
	}
	if len(keys) == 0 {
		return nil, "no unfinished jobs match the selection"
	}
	return keys, ""
}

func schedulerState(dir string) string {
	var status struct {
		State string `json:"state"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "scheduler_status.json"))
	if err != nil || json.Unmarshal(data, &status) != nil {
		return ""
	}
	return status.State
}

// keysWhere lists the live jobs whose attempt satisfies match.
func (r controlRun) keysWhere(match func(dir string) bool) []string {
	var keys []string
	for key, dir := range r.attempts {
		if match(dir) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// controlOutcome is what one invocation did: its process result, made
// independent of the run's identifiers, and the jobs it acted on.
type controlOutcome struct {
	result   support.Result
	affected []string
}

func (r controlRun) symbolic(text string) string {
	pairs := []string{r.runID, "{run}", r.e.Base, "{base}", r.e.Root, "{root}"}
	for _, key := range append([]string{"hold"}, controlKeys...) {
		pairs = append(pairs, r.ids[key], "{"+key+"}")
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

func assertControlOutcome(t *testing.T, command string, flags []pairFlag, outcome controlOutcome) {
	t.Helper()
	r := outcome.result
	if strings.Contains(r.Stderr, "panic:") || strings.Contains(r.Stderr, "fatal error:") {
		t.Fatalf("internal failure: %s", r)
	}
	want, reject := expectedControl(command, flags)
	if reject != "" {
		if r.Code != 1 || !strings.Contains(r.Stderr, reject) || len(outcome.affected) != 0 {
			t.Fatalf("want a rejection containing %q acting on no job; acted on %v: %s", reject, outcome.affected, r)
		}
		return
	}
	if r.Code != 0 || r.Stderr != "" || !slices.Equal(outcome.affected, want) {
		t.Fatalf("acted on %v, want %v (an option was ignored or misapplied): %s", outcome.affected, want, r)
	}
}

func controlCommand(t *testing.T, name string) pairCommand {
	t.Helper()
	for _, command := range readPairSchema(t, support.NewEnv(t)) {
		if command.Name == name {
			return command
		}
	}
	t.Fatalf("%s is missing from schema", name)
	return pairCommand{}
}

// signalOutcome resets every live job to the opposite state, runs one
// suspend or resume, and reports the jobs that it moved to its state.
func (r controlRun) signalOutcome(t *testing.T, command string, flags []pairFlag) controlOutcome {
	t.Helper()
	reset, target := "resume", "suspended"
	if command == "resume" {
		reset, target = "suspend", "running"
	}
	r.e.MustRotari(reset)
	if moved := r.keysWhere(func(dir string) bool { return schedulerState(dir) == target }); len(moved) != 0 {
		t.Fatalf("%s left %v %s", reset, moved, target)
	}
	result := pairInvoke(t, r.e, r.args(t, command, flags)...)
	return controlOutcome{result: result, affected: r.keysWhere(func(dir string) bool { return schedulerState(dir) == target })}
}

func testSignalPairs(t *testing.T, command string) {
	start := time.Now()
	c := controlCommand(t, command)
	r := startControlRun(t)
	var outcomes [2]int
	for _, pair := range commandFlagPairs(c) {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			ab := r.signalOutcome(t, command, []pairFlag{pair.A, pair.B})
			ba := r.signalOutcome(t, command, []pairFlag{pair.B, pair.A})
			assertControlOutcome(t, command, []pairFlag{pair.A, pair.B}, ab)
			assertControlOutcome(t, command, []pairFlag{pair.B, pair.A}, ba)
			if ab.result.Code != ba.result.Code || ab.result.Stdout != ba.result.Stdout || ab.result.Stderr != ba.result.Stderr || !slices.Equal(ab.affected, ba.affected) {
				t.Fatalf("order-dependent %s:\n%s affected=%v\n%s affected=%v", command, ab.result, ab.affected, ba.result, ba.affected)
			}
			outcomes[ab.result.Code]++
		})
	}
	t.Logf("%s pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", command, outcomes[0], outcomes[1], 2*(outcomes[0]+outcomes[1]), time.Since(start))
}

func TestCLIFlagPairSuspend(t *testing.T) {
	covers(t, "SEL-12")
	testSignalPairs(t, "suspend")
}

func TestCLIFlagPairResume(t *testing.T) {
	covers(t, "SEL-12")
	testSignalPairs(t, "resume")
}

// cancelOutcome cancels on r and reports the jobs that finished. Cancelling
// is not reversible, so an invocation expected to act gets its own run.
func (r controlRun) cancelOutcome(t *testing.T, flags []pairFlag, want []string) controlOutcome {
	t.Helper()
	result := pairInvoke(t, r.e, r.args(t, "cancel", flags)...)
	finished := func(dir string) bool {
		_, err := os.Stat(filepath.Join(dir, "finished_at"))
		return err == nil
	}
	var affected []string
	if len(want) > 0 && result.Code == 0 {
		// A cancelled job finishes asynchronously.
		deadline := time.Now().Add(10 * time.Second)
		for affected = r.keysWhere(finished); !slices.Equal(affected, want) && time.Now().Before(deadline); affected = r.keysWhere(finished) {
			time.Sleep(50 * time.Millisecond)
		}
	} else {
		affected = r.keysWhere(finished)
	}
	result.Stdout, result.Stderr = r.symbolic(result.Stdout), r.symbolic(result.Stderr)
	return controlOutcome{result: result, affected: affected}
}

func TestCLIFlagPairCancel(t *testing.T) {
	covers(t, "SEL-12")
	start := time.Now()
	c := controlCommand(t, "cancel")
	// Rejections must act on nothing, so they share one run, checked after each.
	shared := startControlRun(t)
	semaphore := make(chan struct{}, 2)
	var accepted, rejected atomic.Int64
	t.Cleanup(func() {
		t.Logf("cancel pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", accepted.Load(), rejected.Load(), 2*(accepted.Load()+rejected.Load()), time.Since(start))
	})
	t.Run("pairs", func(t *testing.T) {
		for _, pair := range commandFlagPairs(c) {
			ab, ba := []pairFlag{pair.A, pair.B}, []pairFlag{pair.B, pair.A}
			want, reject := expectedControl("cancel", ab)
			if reject != "" {
				t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
					first, second := shared.cancelOutcome(t, ab, nil), shared.cancelOutcome(t, ba, nil)
					assertControlOutcome(t, "cancel", ab, first)
					assertControlOutcome(t, "cancel", ba, second)
					assertSameCancel(t, first, second)
					rejected.Add(1)
				})
				continue
			}
			// At most two pairs (four live runs) at a time: the semaphore is
			// released by the first-registered cleanup, which runs after the
			// runs' own cleanups have reaped them.
			t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
				t.Parallel()
				semaphore <- struct{}{}
				t.Cleanup(func() { <-semaphore })
				first := startControlRun(t).cancelOutcome(t, ab, want)
				second := startControlRun(t).cancelOutcome(t, ba, want)
				assertControlOutcome(t, "cancel", ab, first)
				assertControlOutcome(t, "cancel", ba, second)
				assertSameCancel(t, first, second)
				accepted.Add(1)
			})
		}
	})
}

func assertSameCancel(t *testing.T, first, second controlOutcome) {
	t.Helper()
	if first.result.Code != second.result.Code || first.result.Stdout != second.result.Stdout || first.result.Stderr != second.result.Stderr || !slices.Equal(first.affected, second.affected) {
		t.Fatalf("order-dependent cancel:\n%s affected=%v\n%s affected=%v", first.result, first.affected, second.result, second.affected)
	}
}
