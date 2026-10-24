package interfaces

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
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

// TestFailureGroupRetryHintsWork runs a project whose jobs fail for four
// causes and runs each failure group's retry hint that lineage prints, as
// printed: each previews a rerun of only that group's job. The causes
// overlap the way a job's facts do: the out-of-memory job's failure kind is
// error, like the plain error exit's, and the job that runs out of memory
// and then of disk space is grouped by its later diagnosis, so a hint that
// selected by failure kind or by any diagnosis would take another group's
// job too.
func TestFailureGroupRetryHintsWork(t *testing.T) {
	covers(t, "CLI-4")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "hints", "--job-name", "slow", "--timeout", "1s", "--", "sleep", "10")
	e.MustRotari("add", "-p", "hints", "--job-name", "broken", "--", "sh", "-c", "exit 3")
	e.MustRotari("add", "-p", "hints", "--job-name", "oom", "--", "sh", "-c", `echo "torch.OutOfMemoryError: CUDA out of memory" >&2; exit 1`)
	e.MustRotari("add", "-p", "hints", "--job-name", "full", "--", "sh", "-c", `echo "torch.OutOfMemoryError: CUDA out of memory" >&2; echo "OSError: [Errno 28] No space left on device" >&2; exit 4`)
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
	if len(hints) != 4 {
		t.Fatalf("lineage prints %d retry hints, want one per failure group:\n%s", len(hints), out)
	}
	for _, hint := range hints {
		preview := e.MustRotari(hint...).Stdout
		if !strings.Contains(preview, "would execute 1 of 5 job(s)") {
			t.Errorf("hint %v previews more or less than its group's job:\n%s", hint, preview)
		}
	}
	// show lists each group's job alone when it selects by that cause; it
	// groups only the jobs its table lists.
	var lineage struct {
		Failures failureGroupsView `json:"failures"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("lineage", "-p", "hints", "--json", shown.RunID).Stdout), &lineage); err != nil {
		t.Fatal(err)
	}
	for _, group := range lineage.Failures {
		text := e.MustRotari("show", "-p", "hints", "--filter-failure-cause", group.Cause).Stdout
		_, section, _ := strings.Cut(text, "Failures by cause:\n")
		section, _, _ = strings.Cut(section, "\n\n")
		var entries []string
		for _, line := range strings.Split(section, "\n") {
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
				entries = append(entries, strings.TrimSpace(line))
			}
		}
		if len(entries) != 1 || !strings.HasPrefix(entries[0], "1 "+group.Cause+" (") {
			t.Errorf("show --filter-failure-cause %q lists causes %q, want only its own:\n%s", group.Cause, entries, text)
		}
	}
}

// TestPrintedHintsNameOnlyANonImplicitBaseDir runs a failing project in the
// implicit state directory (ROTARI_BASEDIR) and one in another directory.
// The retry hints of lineage and the failure-summary hint of show name
// --basedir only for the other directory, and each works as printed.
func TestPrintedHintsNameOnlyANonImplicitBaseDir(t *testing.T) {
	covers(t, "CLI-22")
	e := support.NewEnv(t)
	other := filepath.Join(e.Root, "other")
	for _, test := range []struct {
		name     string
		location []string
		wantBase bool
	}{
		{name: "implicit", location: nil},
		{name: "other", location: []string{"--basedir", other}, wantBase: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := []string{"-p", "hints-" + test.name}
			args := func(command string, rest ...string) []string {
				return append(append(append([]string{command}, test.location...), project...), rest...)
			}
			e.MustRotari(args("add", "--job-name", "broken", "--", "sh", "-c", "exit 3")...)
			e.MustRotari(args("add", "--job-name", "fine", "--", "true")...)
			e.Rotari(args("run", "--quiet")...)
			var shown struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari(args("show", "--json")...).Stdout), &shown); err != nil {
				t.Fatal(err)
			}

			var hints [][]string
			for _, line := range strings.Split(e.MustRotari(args("lineage", shown.RunID)...).Stdout, "\n") {
				if hint, ok := strings.CutPrefix(strings.TrimSpace(line), "retry: rotari "); ok {
					hints = append(hints, shellFields(hint))
				}
			}
			for _, line := range strings.Split(e.MustRotari(args("show", "--run-id", shown.RunID)...).Stdout, "\n") {
				if hint, ok := strings.CutPrefix(line, "Failure summary: rotari "); ok {
					hints = append(hints, shellFields(hint))
				}
			}
			if len(hints) != 2 {
				t.Fatalf("got hints %q, want a lineage retry hint and a show failure summary", hints)
			}
			for _, hint := range hints {
				if got := strings.Contains(strings.Join(hint, " "), "--basedir"); got != test.wantBase {
					t.Errorf("hint %q names --basedir = %v, want %v", hint, got, test.wantBase)
				}
				if out := e.MustRotari(hint...).Stdout; !strings.Contains(out, "broken") {
					t.Errorf("hint %q does not reach the failed job:\n%s", hint, out)
				}
			}
		})
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
