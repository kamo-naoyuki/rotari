package pairruns

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func pairRunSample(t *testing.T, f pairMutationFixture, flag pairFlag) []string {
	t.Helper()
	if flag.Name == "partial-array" {
		return []string{"--partial-array=false"}
	}
	values := map[string]string{
		"basedir": f.E.Base, "project-name": f.Project, "config": f.Config,
		"run-id": f.Run, "run-name": "pair-preview",
		"local-concurrency": "2", "batch-concurrency": "2", "retry": "2",
		"job-id": f.Bad, "job-name": "bad", "stage": "training", "matrix": "train",
		"executor": "local", "env": "ALL", "match-by": "id-and-fingerprint", "executor-option": "--debug", "disconnect-action": "cancel",
		"ssh-concurrency": "2", "ssh-options": "ConnectTimeout=1",
		"slurm-concurrency": "2", "slurm-options": "--partition=debug", "slurm-submit-interval": "1s", "slurm-submit-retry-limit": "2",
		"pbs-concurrency": "2", "pbs-options": "-q debug", "pbs-submit-interval": "1s", "pbs-submit-retry-limit": "2",
		"lsf-concurrency": "2", "lsf-options": "-q debug", "lsf-submit-interval": "1s", "lsf-submit-retry-limit": "2",
		"sge-concurrency": "2", "sge-options": "-q debug", "sge-submit-interval": "1s", "sge-submit-retry-limit": "2",
		"filter-result": "failed", "filter-exit-code": "3", "filter-failure-kind": "error",
		"filter-diagnosis": "CUDA/GPU memory exhausted", "filter-host": "*",
		"filter-started-after": "2000-01-01T00:00:00Z", "filter-started-before": "2000-01-01T00:00:00Z",
		"filter-finished-after": "2000-01-01T00:00:00Z", "filter-finished-before": "2000-01-01T00:00:00Z",
		"filter-longer-than": "1h", "filter-shorter-than": "1h", "filter-command": "exit 3",
		"filter-stage": "training", "filter-not-stage": "setup", "filter-matrix": "train", "filter-not-matrix": "train",
	}
	if flag.Name == "if-revision" {
		return []string{"--if-revision", f.Revision}
	}
	if flag.ValueName == "" {
		return []string{"--" + flag.Name + "=true"}
	}
	value, ok := values[flag.Name]
	if !ok {
		t.Fatalf("no safe --%s sample for %s", flag.Name, flag.Name)
	}
	for _, allowed := range flag.Values {
		if allowed == value {
			return []string{"--" + flag.Name, value}
		}
	}
	if len(flag.Values) > 0 {
		t.Fatalf("sample %q outside --%s values %q", value, flag.Name, flag.Values)
	}
	return []string{"--" + flag.Name, value}
}

func pairRunArgs(t *testing.T, f pairMutationFixture, command string, flags []pairFlag) []string {
	t.Helper()
	args := []string{command}
	for _, flag := range flags {
		args = append(args, pairRunSample(t, f, flag)...)
	}
	if !pairHasFlag(flags, "dry-run") {
		args = append(args, "--dry-run")
	}
	return args
}

func assertPairPreviewOutcome(t *testing.T, r support.Result) {
	t.Helper()
	if r.Code == 0 {
		return
	}
	if strings.Contains(r.Stderr, "panic:") || strings.Contains(r.Stderr, "fatal error:") || strings.Contains(r.Stderr, "internal error") {
		t.Fatalf("internal failure: %s", r)
	}
	if r.Code != 1 {
		t.Fatalf("unexpected preview exit: %s", r)
	}
	for _, message := range []string{
		"usage: rotari run ", "usage: rotari retry ", "cannot be combined",
		"cannot be combined with", "job IDs cannot be combined", "--stage, --matrix",
		"project changed since the planned revision", "no jobs matching selection",
		"no queued commands", "no previous run", "not found", "invalid match mode",
		"executor command", "unsupported executor", "invalid scheduler", "invalid dependencies:",
	} {
		if strings.Contains(r.Stderr, message) {
			return
		}
	}
	t.Fatalf("unclassified run-preview failure: %s", r)
}

func TestCLIFlagPairPreviewSamples(t *testing.T) {
	f := newPairMutationFixture(t)
	for _, command := range readPairSchema(t, f.E) {
		if command.Name != "run" && command.Name != "retry" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(command.Name+"/"+flag.Name, func(t *testing.T) {
				f.Initial.Restore(t, f.E.Root, *f.Current)
				r := pairInvoke(t, f.E, pairRunArgs(t, f, command.Name, []pairFlag{flag})...)
				assertPairPreviewOutcome(t, r)
				if after := savePairTree(t, f.E.Root); !reflect.DeepEqual(f.Initial.Raw(), after.Raw()) {
					t.Fatalf("preview changed fixture: %s", r)
				}
				*f.Current = savePairTree(t, f.E.Root)
			})
		}
	}
}

// These pairs check parse, validation, plan output and order independence only.
// Every invocation is dry-run; no scheduler or job process is launched.
func TestCLIFlagPairPreviews(t *testing.T) {
	start := time.Now()
	f := newPairMutationFixture(t)
	state := f.Initial.Raw()
	var outcomes [2]atomic.Int64
	t.Cleanup(func() {
		t.Logf("run/retry preview pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", outcomes[0].Load(), outcomes[1].Load(), 2*(outcomes[0].Load()+outcomes[1].Load()), time.Since(start))
	})
	semaphore := make(chan struct{}, 4)
	for _, command := range readPairSchema(t, f.E) {
		if isPreviewCommand(command.Name) {
			runPreviewCommandPairs(t, f, state, semaphore, &outcomes, command)
		}
	}
	async, dryRun := pairFlag{Name: "async"}, pairFlag{Name: "dry-run"}
	for _, command := range []string{"run", "retry"} {
		for _, pair := range []flagPair{{A: async, B: dryRun}, {A: dryRun, B: async}} {
			args := append([]string{command}, pairRunSample(t, f, pair.A)...)
			args = append(args, pairRunSample(t, f, pair.B)...)
			result := pairInvoke(t, f.E, args...)
			assertPairPreviewOutcome(t, result)
			if result.Code != 1 || !strings.Contains(result.Stderr, "--async cannot be combined with --dry-run") {
				t.Fatalf("dry-run pair accepted async mode: %s", result)
			}
		}
	}
}

func isPreviewCommand(command string) bool { return command == "run" || command == "retry" }

func runPreviewCommandPairs(t *testing.T, f pairMutationFixture, state map[string]string, semaphore chan struct{}, outcomes *[2]atomic.Int64, command pairCommand) {
	t.Helper()
	t.Run(command.Name, func(t *testing.T) {
		for _, pair := range commandFlagPairs(command) {
			t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
				t.Parallel()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				assertRunPreviewOrder(t, f, state, command.Name, pair, outcomes)
			})
		}
	})
}

func assertRunPreviewOrder(t *testing.T, f pairMutationFixture, state map[string]string, command string, pair flagPair, outcomes *[2]atomic.Int64) {
	t.Helper()
	a := pairRunInvoke(t, f, command, []pairFlag{pair.A, pair.B})
	b := pairRunInvoke(t, f, command, []pairFlag{pair.B, pair.A})
	if a.Code < 0 || a.Code > 1 {
		t.Fatalf("unexpected preview code %d", a.Code)
	}
	outcomes[a.Code].Add(1)
	if a.Code != b.Code || a.Stdout != b.Stdout || a.Stderr != b.Stderr {
		t.Fatalf("order-dependent preview:\n%s\n%s", a, b)
	}
	if got := savePairTree(t, f.E.Root); !reflect.DeepEqual(state, got.Raw()) {
		t.Fatalf("preview mutated fixture: %s", a)
	}
}

func pairRunInvoke(t *testing.T, f pairMutationFixture, command string, flags []pairFlag) support.Result {
	t.Helper()
	args := pairRunArgs(t, f, command, flags)
	r := pairInvoke(t, f.E, args...)
	assertPairPreviewOutcome(t, r)
	return r
}

func TestCLIFlagPairAsyncDryRunIsRejected(t *testing.T) {
	covers(t, "CLI-8")
	f := newPairMutationFixture(t)
	for _, command := range []string{"run", "retry"} {
		for _, flags := range [][]string{{"--async", "--dry-run"}, {"--dry-run", "--async"}} {
			t.Run(command+"/"+strings.Join(flags, "+"), func(t *testing.T) {
				f.Initial.Restore(t, f.E.Root, *f.Current)
				result := pairInvoke(t, f.E, append([]string{command}, flags...)...)
				if result.Code != 1 || !strings.Contains(result.Stderr, "--async cannot be combined with --dry-run") {
					t.Fatalf("async dry-run must fail explicitly: %s", result)
				}
				if !reflect.DeepEqual(f.Initial.Raw(), savePairTree(t, f.E.Root).Raw()) {
					t.Fatalf("rejected async dry-run changed state: %s", result)
				}
			})
		}
	}
}

var runPlanJobID = regexp.MustCompile(`(?m)^  execute job_id=(\S+)`)

func pairRunPlanIDs(t *testing.T, f pairMutationFixture, command string, flags ...string) []string {
	t.Helper()
	f.Initial.Restore(t, f.E.Root, *f.Current)
	args := append([]string{command, "--run-id", f.Run, "--dry-run"}, flags...)
	r := pairInvoke(t, f.E, args...)
	assertPairPreviewOutcome(t, r)
	if r.Code != 0 {
		t.Fatalf("selection witness rejected: %s", r)
	}
	ids := []string{}
	for _, match := range runPlanJobID.FindAllStringSubmatch(r.Stdout, -1) {
		ids = append(ids, match[1])
	}
	sort.Strings(ids)
	return ids
}

func TestCLIFlagPairRunSelectionEffects(t *testing.T) {
	covers(t, "SEL-5")
	covers(t, "CLI-9")
	f := newPairMutationFixture(t)
	failed := pairRunPlanIDs(t, f, "run", "--failed")
	training := pairRunPlanIDs(t, f, "run", "--stage", "training")
	want := pairIntersect(failed, training)
	if len(want) == 0 || reflect.DeepEqual(want, failed) || reflect.DeepEqual(want, training) {
		t.Fatal("run fixture does not distinguish failed result from stage selection")
	}
	for _, flags := range [][]string{{"--failed", "--stage", "training"}, {"--stage", "training", "--failed"}} {
		if got := pairRunPlanIDs(t, f, "run", flags...); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %v executes %v, want intersection %v", flags, got, want)
		}
	}
	partial := pairRunPlanIDs(t, f, "run", "--failed")
	whole := pairRunPlanIDs(t, f, "run", "--failed", "--partial-array=false")
	if reflect.DeepEqual(partial, whole) || len(whole) <= len(partial) {
		t.Fatalf("partial-array did not alter failed-task plan: partial=%v whole=%v", partial, whole)
	}
}

func TestCLIFlagPairRunNameInPreview(t *testing.T) {
	covers(t, "CLI-10")
	f := newPairMutationFixture(t)
	plain := pairInvoke(t, f.E, "run", "--run-id", f.Run, "--dry-run")
	named := pairInvoke(t, f.E, "run", "--run-id", f.Run, "--run-name", "pair-preview-witness", "--dry-run")
	if plain.Code != 0 || named.Code != 0 || strings.Contains(plain.Stdout, "pair-preview-witness") || !strings.Contains(named.Stdout, "run_name=pair-preview-witness") {
		t.Fatalf("run name was ignored in the dry-run preview:\n%s\n%s", plain, named)
	}
	if after := savePairTree(t, f.E.Root); !reflect.DeepEqual(f.Initial.Raw(), after.Raw()) {
		t.Fatal("--run-name dry-run changed project state")
	}
}
