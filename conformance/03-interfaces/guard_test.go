package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

var revisionLine = regexp.MustCompile(`(?m)^revision=([0-9a-f]+)$`)

// TestGuardedCommandsPreviewAndCheckTheRevision gives every command that
// changes a project the same three calls: --dry-run must change nothing and
// print the revision `check` reports; --if-revision with that revision must
// apply and print a new one; and the old revision must then be refused
// without changing anything.
func TestGuardedCommandsPreviewAndCheckTheRevision(t *testing.T) {
	covers(t, "CLI-7")
	for _, test := range []struct {
		name string
		// args builds the command from the run and the queued job's ID.
		args func(e *support.Env, run support.FinishedRun, queued string) []string
	}{
		{"add", func(*support.Env, support.FinishedRun, string) []string {
			return []string{"add", "-p", "p1", "--", "true"}
		}},
		{"change", func(_ *support.Env, _ support.FinishedRun, queued string) []string {
			return []string{"change", "-p", "p1", "-j", queued, "--timeout", "1m"}
		}},
		{"remove", func(_ *support.Env, _ support.FinishedRun, queued string) []string {
			return []string{"remove", "-p", "p1", queued}
		}},
		{"copy", func(_ *support.Env, run support.FinishedRun, _ string) []string {
			return []string{"copy", "-p", "p1", "-r", run.RunID, "--append"}
		}},
		{"delete", func(_ *support.Env, run support.FinishedRun, _ string) []string {
			return []string{"delete", "-p", "p1", "-r", run.RunID}
		}},
		{"reset", func(*support.Env, support.FinishedRun, string) []string { return []string{"reset", "p1"} }},
		{"import", func(e *support.Env, _ support.FinishedRun, _ string) []string {
			manifest := filepath.Join(e.Root, "manifest.json")
			if err := os.WriteFile(manifest, []byte(`{"version":1,"jobs":[{"name":"imported","command":["true"]}]}`), 0o600); err != nil {
				e.T.Fatal(err)
			}
			return []string{"import", manifest, "p1", "--overwrite"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.CreateFinishedRun()
			queued := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "queued", "--", "true"))
			args := test.args(e, run, queued)
			before := projectSnapshot(t, e)
			checked := checkRevision(t, e)

			// Options go before the command's own arguments, which for add
			// follow "--" and belong to the job.
			guarded := func(options ...string) []string {
				return append(append([]string{args[0]}, options...), args[1:]...)
			}
			preview := revisionOf(t, e.MustRotari(guarded("--dry-run")...))
			if preview != checked {
				t.Fatalf("--dry-run revision %s, check reports %s", preview, checked)
			}
			if after := projectSnapshot(t, e); after != before {
				t.Fatalf("--dry-run changed the project:\nbefore %s\nafter  %s", before, after)
			}

			applied := revisionOf(t, e.MustRotari(guarded("--if-revision", preview)...))
			if applied == preview || applied != checkRevision(t, e) {
				t.Fatalf("applied revision %s, previewed %s, check reports %s", applied, preview, checkRevision(t, e))
			}
			changed := projectSnapshot(t, e)
			if changed == before {
				t.Fatal("--if-revision with the previewed revision did not apply")
			}

			// The repeated delete names a run that is gone, so it may fail
			// for that reason before the revision is compared.
			result := e.Rotari(guarded("--if-revision", preview)...)
			if result.Code == 0 || (test.name != "delete" && !strings.Contains(result.Stderr, "project changed since the planned revision")) {
				t.Fatalf("stale --if-revision = %s, want refused", result)
			}
			if after := projectSnapshot(t, e); after != changed {
				t.Fatal("a refused --if-revision changed the project")
			}
		})
	}
}

func revisionOf(t *testing.T, result support.Result) string {
	t.Helper()
	match := revisionLine.FindStringSubmatch(result.Stdout)
	if match == nil {
		t.Fatalf("no revision line: %s", result)
	}
	return match[1]
}

func checkRevision(t *testing.T, e *support.Env) string {
	t.Helper()
	var checked struct {
		Revision string `json:"revision"`
	}
	result := e.Rotari("check", "p1", "--json")
	if err := json.Unmarshal([]byte(result.Stdout), &checked); err != nil || checked.Revision == "" {
		t.Fatalf("check --json has no revision: %s", result)
	}
	return checked.Revision
}

// projectSnapshot describes the project's queue, metadata, and runs.
func projectSnapshot(t *testing.T, e *support.Env) string {
	t.Helper()
	dir := filepath.Join(e.Base, "projects", "p1")
	var parts []string
	for _, name := range []string{"queue.json", "meta.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		parts = append(parts, string(data))
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "runs"))
	var runs []string
	for _, entry := range entries {
		runs = append(runs, entry.Name())
	}
	sort.Strings(runs)
	return strings.Join(append(parts, strings.Join(runs, ",")), "\n")
}

// TestRunPreviewMatchesTheRun checks that `retry --dry-run` changes nothing
// and lists the jobs the run then executes, and that the run starts only at
// the previewed revision.
func TestRunPreviewMatchesTheRun(t *testing.T) {
	covers(t, "CLI-7")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	before := projectSnapshot(t, e)

	preview := e.MustRotari("retry", "-p", "p1", "--dry-run")
	revision := revisionOf(t, preview)
	if !strings.Contains(preview.Stdout, "would execute 1 of 2 job(s)") || !strings.Contains(preview.Stdout, "execute job_id="+run.BadJob) || strings.Contains(preview.Stdout, "execute job_id="+run.OKJob) {
		t.Fatalf("retry --dry-run does not plan only the failed job:\n%s", preview.Stdout)
	}
	if revision != checkRevision(t, e) || projectSnapshot(t, e) != before {
		t.Fatal("retry --dry-run changed the project")
	}

	if result := e.Rotari("retry", "-p", "p1", "--if-revision", "0000000000000000"); result.Code == 0 || !strings.Contains(result.Stderr, "project changed since the planned revision") {
		t.Fatalf("retry with a stale revision = %s, want refused", result)
	}
	if projectSnapshot(t, e) != before {
		t.Fatal("a refused retry changed the project")
	}

	e.Rotari("retry", "-p", "p1", "--if-revision", revision, "--quiet")
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "p1", "--json").Stdout), &shown); err != nil || shown.RunID == run.RunID {
		t.Fatalf("retry at the previewed revision did not start a run: %v", err)
	}
	for _, result := range shown.Summary.Results {
		executed := strings.Contains(result.AttemptID, shown.RunID)
		if executed != (result.ID == run.BadJob) {
			t.Errorf("job %s executed=%v in the retry, but the preview planned only %s", result.ID, executed, run.BadJob)
		}
	}

	// Without a copy, the preview plans the queue as it is.
	queued := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--", "true"))
	if output := e.MustRotari("run", "-p", "p1", "--dry-run").Stdout; !strings.Contains(output, "would execute 1 of 1 job(s)") || !strings.Contains(output, "execute job_id="+queued) {
		t.Fatalf("run --dry-run of a queued job:\n%s", output)
	}
}

// TestRunPreviewListsTheTasksOfAWholeArray previews a run of a fresh queue
// that holds an array, through `run --dry-run` and rotari_preview_run, and
// checks that both list every job the run then executes, each array task
// included.
func TestRunPreviewListsTheTasksOfAWholeArray(t *testing.T) {
	covers(t, "CLI-7", "MCP-1")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "p1", "--job-name", "prep", "--", "true")
	e.MustRotari("add", "-p", "p1", "--job-name", "train", "--array", "1-3", "--", "true")

	preview := e.MustRotari("run", "-p", "p1", "--dry-run")
	var want []string
	for _, match := range regexp.MustCompile(`(?m)^  execute job_id=(\S+)`).FindAllStringSubmatch(preview.Stdout, -1) {
		want = append(want, match[1])
	}
	sort.Strings(want)
	if len(want) != 4 || !strings.Contains(preview.Stdout, "would execute 4 of 4 job(s)") {
		t.Fatalf("run --dry-run lists %v, want prep and the three train tasks:\n%s", want, preview.Stdout)
	}

	session := startMCP(t, e)
	var planned struct {
		Execute []struct {
			ID string `json:"id"`
		} `json:"execute"`
	}
	if message := session.call("rotari_preview_run", map[string]any{"basedir_ref": baseDirRef(t, session, "p1"), "project": "p1"}, &planned); message != "" {
		t.Fatal(message)
	}
	var tool []string
	for _, job := range planned.Execute {
		tool = append(tool, job.ID)
	}
	sort.Strings(tool)
	if strings.Join(tool, ",") != strings.Join(want, ",") {
		t.Fatalf("rotari_preview_run lists %v, want %v", tool, want)
	}

	e.MustRotari("run", "-p", "p1", "--if-revision", revisionOf(t, preview), "--quiet")
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "p1", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	var executed []string
	for _, result := range shown.Summary.Results {
		if strings.Contains(result.AttemptID, shown.RunID) {
			executed = append(executed, result.ID)
		}
	}
	sort.Strings(executed)
	if strings.Join(executed, ",") != strings.Join(want, ",") {
		t.Fatalf("the run executed %v, the preview planned %v", executed, want)
	}
}

// TestRunPreviewSaysWhyADependentJobExecutes reruns one job by ID, which
// also executes the job that depends on it, and checks that the preview
// names the rerun dependency on that job's line only.
func TestRunPreviewSaysWhyADependentJobExecutes(t *testing.T) {
	covers(t, "CLI-7")
	e := support.NewEnv(t)
	prep := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "prep", "--", "true"))
	after := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "after", "--depends-on", "prep", "--", "true"))
	e.MustRotari("run", "-p", "p1", "--quiet")
	preview := e.MustRotari("retry", "-p", "p1", "--dry-run", "-j", prep).Stdout
	if !strings.Contains(preview, "execute job_id="+after+" job_name=after depends_on_rerun=prep\n") {
		t.Fatalf("the preview does not say why after executes:\n%s", preview)
	}
	if !strings.Contains(preview, "execute job_id="+prep+" job_name=prep\n") {
		t.Fatalf("the preview gives the selected job a reason:\n%s", preview)
	}
}

// TestAsyncDryRunRefusalShowsTheWayToPreview gives run and retry both
// --async and --dry-run, which is refused, and checks that the refusal
// says how to preview and then start the run.
func TestAsyncDryRunRefusalShowsTheWayToPreview(t *testing.T) {
	covers(t, "CLI-8")
	e := support.NewEnv(t)
	e.CreateFinishedRun()
	for _, command := range []string{"run", "retry"} {
		result := e.Rotari(command, "-p", "p1", "--async", "--dry-run")
		if result.Code == 0 || !strings.Contains(result.Stderr, "preview without --async") || !strings.Contains(result.Stderr, "--async --if-revision REVISION") {
			t.Errorf("%s --async --dry-run = %s, want a refusal that shows how to preview and start", command, result)
		}
	}
}

// TestChangePreviewNamesTheFieldsItChanges previews a change of a job's
// timeout and environment and checks that each changed job's line names the
// fields with their old and new values, before and when it applies.
func TestChangePreviewNamesTheFieldsItChanges(t *testing.T) {
	covers(t, "CLI-7")
	e := support.NewEnv(t)
	job := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "train", "--timeout", "5s", "--", "true"))
	for _, args := range [][]string{{"--dry-run"}, nil} {
		out := e.MustRotari(append([]string{"change", "-p", "p1", "-j", job, "--timeout", "60s", "--env", "LR=0.1"}, args...)...).Stdout
		if !strings.Contains(out, "job="+job+" environment+=LR=0.1 timeout=5s->60s") {
			t.Fatalf("change %v does not name the changed fields:\n%s", args, out)
		}
	}
}
