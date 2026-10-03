package interfaces

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// failureGroupsView is the part of a failure group every view must agree on.
type failureGroupsView []struct {
	Kind      string `json:"kind"`
	Cause     string `json:"cause"`
	Count     int    `json:"count"`
	ExitCodes []int  `json:"exit_codes"`
	Jobs      []struct {
		ID string `json:"id"`
	} `json:"jobs"`
}

func TestFailureGroupsAgreeAcrossViews(t *testing.T) {
	covers(t, "CLI-4")
	e := support.NewEnv(t)
	project := "causes"
	// Tasks 1 and 3 run out of memory with different exit codes, task 2
	// raises a ValueError, task 4 succeeds, and a separate job times out.
	e.MustRotari("add", "-p", project, "--job-name", "train", "--array", "1-4", "--", "sh", "-c",
		`case $ROTARI_ARRAY_TASK_ID in
1) echo "torch.OutOfMemoryError: CUDA out of memory" >&2; exit 1;;
3) echo "torch.OutOfMemoryError: CUDA out of memory" >&2; exit 3;;
2) echo "ValueError: bad learning rate" >&2; exit 2;;
esac`)
	e.MustRotari("add", "-p", project, "--job-name", "slow", "--timeout", "1s", "--", "sleep", "5")
	if result := e.Rotari("run", "-p", project, "--quiet"); result.Code == 0 {
		t.Fatalf("run with failing jobs should exit 1: %s", result)
	}
	var shown struct {
		RunID    string            `json:"run_id"`
		Failures failureGroupsView `json:"failures"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("show --json did not describe the run: %v", err)
	}
	var lineage struct {
		Failures failureGroupsView `json:"failures"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("lineage", "-p", project, "--json", shown.RunID).Stdout), &lineage); err != nil {
		t.Fatal(err)
	}
	var web struct {
		LineageSummary struct {
			Failures failureGroupsView `json:"failures"`
		} `json:"lineage_summary"`
	}
	body := e.HTTPGet(e.StartWeb() + "/api/run?project_name=" + url.QueryEscape(project) + "&run_id=" + url.QueryEscape(shown.RunID)).Body
	if err := json.Unmarshal([]byte(body), &web); err != nil {
		t.Fatal(err)
	}

	var causes []string
	for _, group := range shown.Failures {
		causes = append(causes, group.Kind+":"+group.Cause)
	}
	if want := []string{"diagnosis:CUDA/GPU memory exhausted", "diagnosis:Python type or value error", "timeout:timeout"}; !reflect.DeepEqual(causes, want) {
		t.Fatalf("show --json causes = %q, want %q\n%+v", causes, want, shown.Failures)
	}
	if oom := shown.Failures[0]; oom.Count != 2 || !reflect.DeepEqual(oom.ExitCodes, []int{1, 3}) || len(oom.Jobs) != 2 {
		t.Fatalf("OOM group = %+v, want tasks 1 and 3 with exit codes 1 and 3", oom)
	}
	if !reflect.DeepEqual(lineage.Failures, shown.Failures) {
		t.Errorf("lineage --json failures differ from show --json:\nlineage %+v\nshow    %+v", lineage.Failures, shown.Failures)
	}
	if !reflect.DeepEqual(web.LineageSummary.Failures, shown.Failures) {
		t.Errorf("Web API failures differ from show --json:\nweb  %+v\nshow %+v", web.LineageSummary.Failures, shown.Failures)
	}
	for name, text := range map[string]string{
		"show":    e.MustRotari("show", "-p", project, "--run-id", shown.RunID).Stdout,
		"lineage": e.MustRotari("lineage", "-p", project, shown.RunID).Stdout,
	} {
		for _, want := range []string{"Failures by cause:", "2 CUDA/GPU memory exhausted (exit 1,3): train[1,3]", "1 timeout (exit 124): slow"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s output does not contain %q:\n%s", name, want, text)
			}
		}
	}
}

// TestFailureGroupRetryHintsWork runs a project whose jobs fail for two
// causes, a timeout and an error exit, and runs each failure group's retry
// hint that lineage prints, as printed: each previews a rerun of only that
// group's job.
func TestFailureGroupRetryHintsWork(t *testing.T) {
	covers(t, "CLI-4")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "hints", "--job-name", "slow", "--timeout", "1s", "--", "sleep", "10")
	e.MustRotari("add", "-p", "hints", "--job-name", "broken", "--", "sh", "-c", "exit 3")
	e.MustRotari("add", "-p", "hints", "--job-name", "fine", "--", "true")
	e.Rotari("run", "-p", "hints", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "hints", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	out := e.MustRotari("lineage", "-p", "hints", shown.RunID).Stdout
	var hints [][]string
	for _, line := range strings.Split(out, "\n") {
		if hint, ok := strings.CutPrefix(strings.TrimSpace(line), "retry: rotari "); ok {
			hints = append(hints, shellFields(hint))
		}
	}
	if len(hints) != 2 {
		t.Fatalf("lineage prints %d retry hints, want one per failure group:\n%s", len(hints), out)
	}
	for _, hint := range hints {
		preview := e.MustRotari(hint...).Stdout
		if !strings.Contains(preview, "would execute 1 of 3 job(s)") {
			t.Errorf("hint %v previews more or less than its group's job:\n%s", hint, preview)
		}
	}
}

// shellFields splits a command line as a shell does for the quoting rotari
// prints: words separated by spaces, with single quotes around a word.
func shellFields(line string) []string {
	var fields []string
	var current strings.Builder
	quoted, started := false, false
	for _, r := range line {
		switch {
		case r == '\'':
			quoted, started = !quoted, true
		case r == ' ' && !quoted:
			if started {
				fields = append(fields, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if started {
		fields = append(fields, current.String())
	}
	return fields
}
