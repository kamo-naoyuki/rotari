package lifecycle

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestRunNotes records why a run and a retry were made with --note, adds a
// run note and a job note afterwards, and checks that show, show -j,
// lineage, and the Web API report each note where it belongs. A dry run
// shows the note and records nothing.
func TestRunNotes(t *testing.T) {
	covers(t, "RUN-16")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "notes", "--job-name", "train", "--array", "1-2", "--", "sh", "-c", `exit "$ROTARI_ARRAY_TASK_ID"`)
	preview := e.MustRotari("run", "-p", "notes", "--dry-run", "--note", "first sweep").Stdout
	if !strings.Contains(preview, "note: first sweep") {
		t.Fatalf("run --dry-run does not show its note:\n%s", preview)
	}
	if r := e.Rotari("run", "-p", "notes", "--quiet", "--note", "first sweep"); r.Code == 0 {
		t.Fatalf("the first run should fail: %s", r)
	}
	first := latestRun(t, e, "notes")
	var shown struct {
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-r", first, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	attempt := ""
	for _, result := range shown.Summary.Results {
		if strings.HasSuffix(result.ID, "-1") {
			attempt = result.AttemptID
		}
	}
	if attempt == "" {
		t.Fatalf("no attempt for task 1 in %+v", shown.Summary.Results)
	}
	e.MustRotari("note", attempt, "exit", "1", "is", "expected")
	e.MustRotari("note", first, "both tasks fail on purpose")
	for _, bad := range [][]string{{"note", first, "  "}, {"note", "20990101-000000-ffffffff", "x"}, {"note", first}} {
		if r := e.Rotari(bad...); r.Code == 0 {
			t.Errorf("%v succeeded: %s", bad, r)
		}
	}

	runView := e.MustRotari("show", "-r", first, "--no-pager").Stdout
	for _, want := range []string{" first sweep\n", " [train[1]] exit 1 is expected\n", " both tasks fail on purpose\n"} {
		if !strings.Contains(runView, want) {
			t.Errorf("show for the run lacks note %q:\n%s", want, runView)
		}
	}
	jobView := e.MustRotari("show", "-j", attempt, "--no-pager").Stdout
	if !strings.Contains(jobView, " exit 1 is expected\n") || strings.Contains(jobView, "both tasks fail") {
		t.Errorf("show -j does not show only the job's note:\n%s", jobView)
	}

	e.Rotari("retry", "-p", "notes", "--quiet", "--note", "retry to see the same failures")
	second := latestRun(t, e, "notes")
	history := e.MustRotari("lineage", "-p", "notes").Stdout
	for _, want := range []string{"NOTE", "first sweep (+1 more)", "retry to see the same failures"} {
		if !strings.Contains(history, want) {
			t.Errorf("lineage history lacks %q:\n%s", want, history)
		}
	}
	comparison := e.MustRotari("lineage", first, second).Stdout
	for _, want := range []string{"Note (from): ", "[train[1]] exit 1 is expected", "Note (to): ", "retry to see the same failures"} {
		if !strings.Contains(comparison, want) {
			t.Errorf("lineage comparison lacks %q:\n%s", want, comparison)
		}
	}

	web := e.StartWeb()
	// The run page shows the run's notes in its report, each job's notes in
	// that job's section.
	report := e.HTTPGet(web + "/api/report?project_name=notes&run_id=" + url.QueryEscape(first))
	if report.Status != 200 {
		t.Fatalf("GET report: status %d: %s", report.Status, report.Body)
	}
	runNotes, jobSections, _ := strings.Cut(report.Body, "\n## Jobs\n")
	if !strings.Contains(runNotes, "\n## Notes\n") || !strings.Contains(runNotes, "\n\nfirst sweep\n") || !strings.Contains(runNotes, "\n\nboth tasks fail on purpose\n") || strings.Contains(runNotes, "exit 1 is expected") ||
		!strings.Contains(jobSections, "\n### Notes\n") || !strings.Contains(jobSections, "\n\nexit 1 is expected\n") {
		t.Fatalf("Web run report does not place the notes:\n%s", report.Body)
	}

	response := e.HTTPGet(web + "/api/run?project_name=notes&run_id=" + url.QueryEscape(first))
	if response.Status != 200 {
		t.Fatalf("GET run: status %d: %s", response.Status, response.Body)
	}
	var detail struct {
		Notes []struct {
			JobID     string `json:"job_id"`
			AttemptID string `json:"attempt_id"`
			Text      string `json:"text"`
		} `json:"notes"`
		Jobs []struct {
			ID         string   `json:"id"`
			NoteLabels []string `json:"note_labels"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Notes) != 3 || detail.Notes[0].Text != "first sweep" || detail.Notes[1].AttemptID != attempt || detail.Notes[1].Text != "exit 1 is expected" {
		t.Fatalf("Web run notes = %+v", detail.Notes)
	}
	// Each job carries its own notes, for the run page's Notes button.
	if len(detail.Jobs) != 2 {
		t.Fatalf("Web run has %d jobs, want the 2 array tasks", len(detail.Jobs))
	}
	for _, job := range detail.Jobs {
		noted := strings.HasSuffix(job.ID, "-1")
		if noted != (len(job.NoteLabels) == 1) || noted && !strings.HasSuffix(job.NoteLabels[0], " exit 1 is expected") {
			t.Fatalf("Web job %s note labels = %q", job.ID, job.NoteLabels)
		}
	}
}
