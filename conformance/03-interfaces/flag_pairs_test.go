package interfaces

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// Deferred commands remain visible, and a schema change requires an explicit
// coverage review. The fingerprint is of sorted flag names, not implementation.
var pairInventory = map[string]struct {
	count       int
	fingerprint string
}{
	"init":       {0, "e3b0c44298fc1c14"},
	"config":     {6, "cf7bcb9626d67065"},
	"check":      {6, "bc250836712a03be"},
	"reset":      {6, "0f2241eed8e6f8d4"},
	"cancel":     {20, "65aeaf4b54d48db3"},
	"suspend":    {19, "4b2e628041f2b2eb"},
	"resume":     {19, "4b2e628041f2b2eb"},
	"delete":     {7, "cf61505b0fa4c0e7"},
	"basedirs":   {1, "9c7ab83027410026"},
	"projects":   {2, "264d74c73d80792f"},
	"gc":         {3, "4f8007fd85e880d5"},
	"unlock":     {4, "2d6ab0e22e6da38c"},
	"change":     {40, "39d6e21a49c89e7d"},
	"export":     {7, "36180bb4d09c2f4f"},
	"import":     {7, "32eb87a68c1692c7"},
	"remove":     {17, "83215b4518cebafa"},
	"show":       {38, "e443b4918c6ed8ce"},
	"lineage":    {4, "03f8539f8880af76"},
	"jobs":       {6, "768d746bfd3b0ba3"},
	"runs":       {5, "049db737a3960cca"},
	"wait":       {9, "8c0920a50fe2e5fe"},
	"add":        {27, "804acd7f5887f012"},
	"copy":       {32, "25806ce2d014cab0"},
	"run":        {61, "f2925eab4bb9c0db"},
	"retry":      {61, "f2925eab4bb9c0db"},
	"server":     {3, "1bc5f18bd9dd276d"},
	"web":        {10, "db8a2826bc39f7f8"},
	"mcp":        {2, "c90df8ffe2985354"},
	"completion": {0, "e3b0c44298fc1c14"},
	"schema":     {1, "02bd175f32972037"},
	"guide":      {0, "e3b0c44298fc1c14"},
	"version":    {0, "e3b0c44298fc1c14"},
	"env":        {0, "e3b0c44298fc1c14"},
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
		if pairAdapter(c.Name) != "" {
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

func TestCLIFlagPairs(t *testing.T) {
	start := time.Now()
	f := newPairFixture(t)
	// Supply a real queue as well as history, so queue-scoped samples resolve
	// existing stages/matrices rather than failing on missing fixture data.
	f.e.MustRotari("copy", "--run-id", f.run, "--quiet")
	invocations := 0
	outcomes := [2]int{} // assertPairOutcome permits only success (0) or rejection (1).
	for _, c := range readPairSchema(t, f.e) {
		if pairAdapter(c.Name) != "read" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			for _, pair := range commandFlagPairs(c) {
				t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
					aArgs, bArgs := f.sample(t, pair.A), f.sample(t, pair.B)
					base := pairReadBase(c.Name, f, pair.A.Name == "basedir" || pair.B.Name == "basedir")
					ab := pairInvoke(t, f.e, append(append(base, aArgs...), bArgs...)...)
					ba := pairInvoke(t, f.e, append(append(base, bArgs...), aArgs...)...)
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

func pairReadBase(command string, f pairFixture, explicitBasedir bool) []string {
	if command == "jobs" || command == "runs" {
		if explicitBasedir {
			return []string{command}
		}
		return []string{command, "--basedir", f.e.Base}
	}
	if command == "lineage" {
		return []string{command, f.run}
	}
	return []string{command}
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
		all := pairInvoke(t, f.e, "jobs", "--basedir", f.e.Base, "--format", "%a %n", "--since", "7d")
		running := pairInvoke(t, f.e, "jobs", "--basedir", f.e.Base, "--format", "%a %n", "--since", "0")
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
		if pairAdapter(c.Name) != "read" {
			continue
		}
		for _, flag := range c.Flags {
			t.Run(c.Name+"/"+flag.Name, func(t *testing.T) {
				assertPairOutcome(t, pairInvoke(t, f.e, append(pairReadBase(c.Name, f, flag.Name == "basedir"), f.sample(t, flag)...)...))
				if flag.ValueName == "" {
					assertPairOutcome(t, pairInvoke(t, f.e, append(pairReadBase(c.Name, f, flag.Name == "basedir"), "--"+flag.Name+"=false")...))
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
