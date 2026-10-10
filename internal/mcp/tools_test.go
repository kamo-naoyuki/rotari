package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// toolFixture is a master directory with two basedirs that both hold a
// project "exp": the first has two runs of a three-task array, the second
// one healthy run.
type toolFixture struct {
	masterDir, firstBaseDir, secondBaseDir string
	firstRun, secondRun, healthyRun        string
}

func newToolFixture(t *testing.T) toolFixture {
	t.Helper()
	first, _ := filepath.Abs(t.TempDir())
	second, _ := filepath.Abs(t.TempDir())
	f := toolFixture{masterDir: t.TempDir(), firstBaseDir: first, secondBaseDir: second,
		firstRun: "20260101-000000-bbbbbbbb", secondRun: "20260101-000000-aaaaaaaa", healthyRun: "20260101-000000-cccccccc"}
	oom := func(exitCode int, evidence string) model.JobResult {
		return model.JobResult{ExitCode: exitCode, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: []model.RuleDiagnosis{
			{Name: "CUDA/GPU memory exhausted", Evidence: evidence, Suggestion: "Reduce batch size."}}}
	}
	timeout := model.JobResult{ExitCode: model.TimeoutExitCode, Error: "timed out after 5s"}
	// The first run's tasks: OOM with exit 1, OOM with exit 3, timeout. The
	// second run fixes task 1, and task 2 now times out instead.
	f.writeRun(t, first, f.firstRun, "2026-01-01T00:00:00.1Z", []model.JobResult{
		oom(1, "CUDA out of memory reading /home/alice/data/shard-1.bin"), oom(3, "CUDA out of memory"), timeout,
	})
	f.writeRun(t, first, f.secondRun, "2026-01-01T00:00:00.9Z", []model.JobResult{{ExitCode: 0}, timeout, timeout})
	f.writeRun(t, second, f.healthyRun, "2026-01-01T00:00:00.5Z", []model.JobResult{{ExitCode: 0}, {ExitCode: 0}, {ExitCode: 0}})
	return f
}

func (f toolFixture) writeRun(t *testing.T, baseDir, runID, startedAt string, results []model.JobResult) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, "exp")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	for _, write := range []struct {
		path  string
		value any
	}{
		{paths.QueueFile, model.Queue{}},
		{paths.MetaFile, model.Meta{Phase: "finished", LastRunID: runID}},
		{filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
			{ID: "tr", Name: "train", Command: []string{"./train.sh"}, Array: &model.ArraySpec{First: 1, Last: len(results)}},
		}}},
	} {
		if err := state.WriteJSON(write.path, write.value); err != nil {
			t.Fatal(err)
		}
	}
	summary := model.RunSummary{RunID: runID, Status: "finished"}
	for index, result := range results {
		result.ID = "tr-" + strconv.Itoa(index+1)
		result.AttemptID = "att-" + runID + "-" + result.ID
		if result.ExitCode != 0 {
			summary.Status, summary.ExitCode = "failed", 1
		}
		summary.Results = append(summary.Results, result)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, state.LoadSamplesFileName), []byte(`{"at":"`+startedAt+`","one":1,"five":1,"fifteen":1}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runregistry.Open(f.masterDir).Register(runregistry.Location{BaseDir: baseDir, ProjectName: "exp", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := basedirregistry.Open(f.masterDir).Register(baseDir); err != nil {
		t.Fatal(err)
	}
}

func TestListProjectsNamesBaseDirsWithoutPaths(t *testing.T) {
	f := newToolFixture(t)
	output, err := listProjects(f.masterDir)
	if err != nil {
		t.Fatal(err)
	}
	byRef := map[string]ProjectOverview{}
	for _, project := range output.Projects {
		byRef[project.BaseDirRef] = project
	}
	first, second := byRef[basedirregistry.Ref(f.firstBaseDir)], byRef[basedirregistry.Ref(f.secondBaseDir)]
	if len(output.Projects) != 2 || first.Project != "exp" || second.Project != "exp" {
		t.Fatalf("projects = %+v, want exp in each basedir", output.Projects)
	}
	if first.LastRunID != f.secondRun || first.LastStatus != "failed" || first.LastFailed != 2 || first.LastJobs != 3 || first.Runs != 2 || first.State != "idle" {
		t.Errorf("first basedir's exp = %+v, want its second run with 2 of 3 failed", first)
	}
	if second.LastRunID != f.healthyRun || second.LastStatus != "finished" || second.LastFailed != 0 {
		t.Errorf("second basedir's exp = %+v, want its healthy run", second)
	}
	encoded, _ := json.Marshal(output)
	for _, path := range []string{f.firstBaseDir, f.secondBaseDir, f.masterDir} {
		if strings.Contains(string(encoded), path) {
			t.Fatalf("list_projects reveals path %q: %s", path, encoded)
		}
	}
}

func TestRunSummaryGroupsFailuresAndRedactsEvidence(t *testing.T) {
	f := newToolFixture(t)
	output, err := runSummary(f.masterDir, RunSummaryInput{RunID: f.firstRun})
	if err != nil {
		t.Fatal(err)
	}
	if output.Project != "exp" || output.BaseDirRef != basedirregistry.Ref(f.firstBaseDir) || output.Summary.Counts.Failed != 3 {
		t.Fatalf("summary = %+v, want the first basedir's run with 3 failures", output)
	}
	var causes []string
	for _, group := range output.Summary.Failures {
		causes = append(causes, group.Cause)
	}
	if want := []string{"CUDA/GPU memory exhausted", model.FailureKindTimeout}; !reflect.DeepEqual(causes, want) {
		t.Fatalf("causes = %q, want %q", causes, want)
	}
	oom := output.Summary.Failures[0]
	if !reflect.DeepEqual(oom.ExitCodes, []int{1, 3}) || oom.Example.AttemptID != "att-"+f.firstRun+"-tr-1" {
		t.Errorf("OOM group = %+v, want exit codes 1 and 3 and task 1's attempt", oom)
	}
	if strings.Contains(oom.Example.Evidence, "/home/alice") || !strings.Contains(oom.Example.Evidence, "[REDACTED_PATH]") {
		t.Errorf("evidence %q is not redacted", oom.Example.Evidence)
	}
}

func TestCompareRunsDefaultsToThePreviousRun(t *testing.T) {
	f := newToolFixture(t)
	for _, input := range []CompareRunsInput{{RunID: f.secondRun}, {RunID: f.secondRun, PreviousRunID: f.firstRun}} {
		output, err := compareRuns(f.masterDir, input)
		if err != nil {
			t.Fatalf("compareRuns(%+v): %v", input, err)
		}
		comparison := output.Comparison
		if comparison.From.ID != f.firstRun || comparison.To.ID != f.secondRun || comparison.Summary.Fixed != 1 || comparison.Summary.StillFailing != 2 || comparison.Summary.CauseChanged != 1 {
			t.Fatalf("compareRuns(%+v) = %+v", input, comparison.Summary)
		}
		jobs := map[string]runlineage.JobDiff{}
		for _, job := range comparison.Jobs {
			jobs[job.ToID] = job
		}
		if task := jobs["tr-2"]; !task.CauseChanged || task.FromCause != "CUDA/GPU memory exhausted" || task.ToCause != model.FailureKindTimeout {
			t.Errorf("task 2 = %+v, want its cause to change from OOM to timeout", task)
		}
	}
}

func TestCompareRunsCountsUnchangedJobsUnlessAllAreAsked(t *testing.T) {
	f := newToolFixture(t)
	again := "20260101-000000-dddddddd"
	f.writeRun(t, f.secondBaseDir, again, "2026-01-01T00:00:01Z", []model.JobResult{{ExitCode: 0}, {ExitCode: 0}, {ExitCode: 0}})
	output, err := compareRuns(f.masterDir, CompareRunsInput{RunID: again})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Comparison.Jobs) != 0 || output.HiddenUnchanged != 3 {
		t.Fatalf("default comparison lists %+v, hides %d; want the three unchanged tasks hidden", output.Comparison.Jobs, output.HiddenUnchanged)
	}
	output, err = compareRuns(f.masterDir, CompareRunsInput{RunID: again, AllJobs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Comparison.Jobs) != 3 || output.HiddenUnchanged != 0 {
		t.Fatalf("all_jobs comparison lists %d, hides %d", len(output.Comparison.Jobs), output.HiddenUnchanged)
	}
}

func TestCompareRunsRejectsRunsOfDifferentProjects(t *testing.T) {
	f := newToolFixture(t)
	tests := []struct {
		name  string
		input CompareRunsInput
		want  string
	}{
		{"other basedir", CompareRunsInput{RunID: f.secondRun, PreviousRunID: f.healthyRun}, "belong to different projects"},
		{"first run", CompareRunsInput{RunID: f.firstRun}, "has no earlier run"},
		{"unregistered", CompareRunsInput{RunID: "20990101-000000-deadbeef"}, "is not registered"},
		{"unsafe previous", CompareRunsInput{RunID: f.secondRun, PreviousRunID: "../x"}, "invalid run id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := compareRuns(f.masterDir, test.input); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compareRuns(%+v) error = %v, want %q", test.input, err, test.want)
			}
		})
	}
}

func TestCheckProjectReportsReadinessWithoutPaths(t *testing.T) {
	f := newToolFixture(t)
	secondRef := basedirregistry.Ref(f.secondBaseDir)
	paths, err := state.ResolveProjectPaths(f.secondBaseDir, "exp")
	if err != nil {
		t.Fatal(err)
	}
	if output, err := checkProject(f.masterDir, CheckProjectInput{BaseDirRef: secondRef, Project: "exp"}); err != nil || output.State != "empty" || output.Runnable || *output.Queued != 0 {
		t.Fatalf("check of an empty queue = %+v, %v", output, err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "a", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if output, err := checkProject(f.masterDir, CheckProjectInput{BaseDirRef: secondRef, Project: "exp"}); err != nil || output.State != "ready" || !output.Runnable || *output.Queued != 1 {
		t.Fatalf("check of a ready queue = %+v, %v", output, err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "a", Command: []string{"true"}, Executor: "nosuch"}}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		input CheckProjectInput
		want  string
	}{
		{"invalid queue", CheckProjectInput{BaseDirRef: secondRef, Project: "exp"}, "unsupported executor: nosuch"},
		{"unknown ref", CheckProjectInput{BaseDirRef: "0123", Project: "exp"}, "is not registered"},
		{"unknown project", CheckProjectInput{BaseDirRef: secondRef, Project: "nope"}, "nope"},
		{"unsafe project", CheckProjectInput{BaseDirRef: secondRef, Project: "../exp"}, "../exp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkProject(f.masterDir, test.input)
			err = hideStateDirs(f.masterDir, err)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("checkProject(%+v) error = %v, want %q", test.input, err, test.want)
			}
			if strings.Contains(err.Error(), f.secondBaseDir) {
				t.Fatalf("error reveals the basedir path: %v", err)
			}
		})
	}
}

func TestLimitComparedJobsKeepsResultChangesFirstInOrder(t *testing.T) {
	jobs := make([]runlineage.JobDiff, 0, 25)
	for index := 0; index < 25; index++ {
		job := runlineage.JobDiff{Name: fmt.Sprintf("job-%02d", index), Transition: runlineage.TransitionUnchanged, Changes: []runlineage.Change{{Field: "timeout"}}}
		// Two results change beyond the limit, where they would be cut
		// without their priority.
		if index == 21 || index == 23 {
			job.Transition = runlineage.TransitionFixed
		}
		jobs = append(jobs, job)
	}
	shown, hidden := limitComparedJobs(jobs, 20)
	if len(shown) != 20 || hidden != 5 {
		t.Fatalf("shown %d, hidden %d; want 20 and 5", len(shown), hidden)
	}
	fixed := 0
	for index, job := range shown {
		if job.Transition == runlineage.TransitionFixed {
			fixed++
		}
		if index > 0 && shown[index-1].Name >= job.Name {
			t.Fatalf("jobs out of order: %s before %s", shown[index-1].Name, job.Name)
		}
	}
	if fixed != 2 || shown[17].Name != "job-17" || shown[19].Name != "job-23" {
		t.Fatalf("kept %d fixed jobs, %s and %s at 17 and 19; want both fixed jobs after job-00 to job-17", fixed, shown[17].Name, shown[19].Name)
	}
	if shown, hidden := limitComparedJobs(jobs[:5], 20); len(shown) != 5 || hidden != 0 {
		t.Fatalf("under the limit: shown %d, hidden %d", len(shown), hidden)
	}
}

func TestRunSummaryListsTenJobsPerGroupUnlessAllAreAsked(t *testing.T) {
	f := newToolFixture(t)
	results := make([]model.JobResult, 12)
	for index := range results {
		results[index] = model.JobResult{ExitCode: 3}
	}
	many := "20260101-000000-eeeeeeee"
	f.writeRun(t, f.secondBaseDir, many, "2026-01-01T00:00:02Z", results)
	output, err := runSummary(f.masterDir, RunSummaryInput{RunID: many})
	if err != nil {
		t.Fatal(err)
	}
	if group := output.Summary.Failures[0]; group.Count != 12 || len(group.Jobs) != 10 || group.JobsOmitted != 2 {
		t.Fatalf("default group = count %d, %d jobs, %d omitted", group.Count, len(group.Jobs), group.JobsOmitted)
	}
	output, err = runSummary(f.masterDir, RunSummaryInput{RunID: many, AllJobs: true})
	if err != nil {
		t.Fatal(err)
	}
	if group := output.Summary.Failures[0]; len(group.Jobs) != 12 || group.JobsOmitted != 0 {
		t.Fatalf("all_jobs group = %d jobs, %d omitted", len(group.Jobs), group.JobsOmitted)
	}
}

// TestRunToolsNameSourcesWithoutPaths records a repository for both runs and
// a note with a path; the run summary and the comparison name the repository
// by its last path element, and redact paths in a read error and the note.
func TestRunToolsNameSourcesWithoutPaths(t *testing.T) {
	f := newToolFixture(t)
	paths, err := state.ResolveProjectPaths(f.firstBaseDir, "exp")
	if err != nil {
		t.Fatal(err)
	}
	root := "/home/alice/project"
	for runID, revision := range map[string]model.SourceRevision{
		f.firstRun:  {Root: root, VCS: "git", CommitID: "1111"},
		f.secondRun: {Root: root, VCS: "git", Error: "git: cannot read /home/alice/project/.git"},
	} {
		if err := state.SaveRunSources(filepath.Join(paths.RunsDir, runID), model.RunSources{Sources: []model.SourceRevision{revision}}); err != nil {
			t.Fatal(err)
		}
	}
	runDir := filepath.Join(paths.RunsDir, f.firstRun)
	if err := state.AppendRunNote(runDir, model.RunNote{At: "2026-01-01T00:00:00Z", Text: "data from /home/alice/data"}); err != nil {
		t.Fatal(err)
	}
	summary, err := runSummary(f.masterDir, RunSummaryInput{RunID: f.firstRun})
	if err != nil {
		t.Fatal(err)
	}
	if sources := summary.Summary.Run.Sources; len(sources) != 1 || sources[0].Root != "project" || sources[0].CommitID != "1111" {
		t.Fatalf("run summary sources = %+v, want project at 1111", sources)
	}
	if notes := summary.Summary.Run.Notes; len(notes) != 1 || strings.Contains(notes[0].Text, "/home/alice") {
		t.Fatalf("run summary notes = %+v, want one with its path redacted", notes)
	}
	output, err := compareRuns(f.masterDir, CompareRunsInput{RunID: f.secondRun, PreviousRunID: f.firstRun})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/home/alice") {
		t.Fatalf("comparison returns an absolute path: %s", encoded)
	}
	changes := output.Comparison.Sources
	if len(changes) != 1 || changes[0].Root != "project" || changes[0].Change != runlineage.SourceUnknown || changes[0].From.CommitID != "1111" || !strings.Contains(changes[0].To.Error, "[REDACTED_PATH]") {
		t.Fatalf("comparison sources = %+v", changes)
	}
}
