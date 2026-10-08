package web

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestLoadQueueStateBuildsRunsFromCallbacks(t *testing.T) {
	state, err := LoadQueueState(QueueLoader{
		ProjectName: "demo",
		Queue: func() (model.Queue, error) {
			return model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo"}}}}, nil
		},
		Lock: func() (model.LockInfo, error) {
			return model.LockInfo{RunID: "run-2", PID: 42, StartedAt: "lock-start"}, nil
		},
		Runs: func() ([]string, error) { return []string{"run-1", "run-2"}, nil },
		Summary: func(runID string) (model.RunSummary, error) {
			if runID == "run-1" {
				return model.RunSummary{RunID: runID, StartedAt: "start-1", FinishedAt: "finish-1", ExitCode: 0}, nil
			}
			return model.RunSummary{}, assertNotFound{}
		},
		Jobs: func(runID string, summary model.RunSummary) ([]Job, error) {
			return []Job{{ID: runID + "-job", Result: &model.JobResult{ID: runID + "-job", ExitCode: 0}}}, nil
		},
		Context: func(string) (model.RunContext, error) { return model.RunContext{CWD: "/work"}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.QueueName != "demo" || state.RunningRunID != "run-2" || state.RunnerPID != 42 {
		t.Fatalf("state = %#v", state)
	}
	if len(state.Runs) != 2 || state.Runs[0].RunID != "run-2" || state.Runs[0].Status != "running" || !state.Runs[0].Running {
		t.Fatalf("runs = %#v", state.Runs)
	}
	if state.Runs[1].RunID != "run-1" || len(state.Runs[1].Jobs) != 1 {
		t.Fatalf("sorted runs = %#v", state.Runs)
	}
	if got := state.Runs[1].LineageSummary.Counts.Succeeded; got != 1 {
		t.Fatalf("lineage summary counts = %#v", state.Runs[1].LineageSummary)
	}
}

func TestBuildLineageSummaryCountsBlockedJobsSeparately(t *testing.T) {
	summary := buildLineageSummary(model.RunSummary{RunID: "run-1"}, []Job{
		{ID: "failed", Result: &model.JobResult{ID: "failed", ExitCode: 1, Error: "command failed"}},
		{ID: "blocked", Result: &model.JobResult{ID: "blocked", ExitCode: 1, Error: "blocked by failed dependency"}},
	})
	if summary.Counts.Failed != 1 || summary.Counts.Blocked != 1 {
		t.Fatalf("counts = %+v, want one failed and one blocked", summary.Counts)
	}
}

func TestBuildLineageSummaryGroupsFailuresByCause(t *testing.T) {
	task := func(number int) *int { return &number }
	oom := []model.RuleDiagnosis{{Name: "CUDA/GPU memory exhausted", Evidence: "CUDA out of memory"}}
	summary := buildLineageSummary(model.RunSummary{RunID: "run-2"}, []Job{
		{ID: "tr-1", Name: "train[1]", ArrayTaskID: task(1), Result: &model.JobResult{ID: "tr-1", ExitCode: 1, Diagnoses: oom}},
		{ID: "tr-2", Name: "train[2]", ArrayTaskID: task(2), Result: &model.JobResult{ID: "tr-2", ExitCode: 0}},
		{ID: "tr-3", Name: "train[3]", ArrayTaskID: task(3), Carried: true, Result: &model.JobResult{ID: "tr-3", ExitCode: 2, Diagnoses: oom}},
		{ID: "ev", Name: "eval", Result: &model.JobResult{ID: "ev", ExitCode: 124, Error: "timed out after 5s"}},
	})
	if len(summary.Failures) != 2 {
		t.Fatalf("failures = %+v, want OOM and timeout groups", summary.Failures)
	}
	group := summary.Failures[0]
	if group.Cause != "CUDA/GPU memory exhausted" || group.Count != 2 || group.Carried != 1 ||
		len(group.Jobs) != 2 || *group.Jobs[0].ArrayTaskID != 1 || *group.Jobs[1].ArrayTaskID != 3 || !group.Jobs[1].Carried {
		t.Fatalf("OOM group = %+v", group)
	}
	if timeout := summary.Failures[1]; timeout.Kind != model.FailureKindTimeout || timeout.Jobs[0].ID != "ev" {
		t.Fatalf("timeout group = %+v", timeout)
	}
}

func TestLoadQueueStateMarksRunsFromNewerRotariUnreadable(t *testing.T) {
	newer := fmt.Errorf("%w: summary.json has state version 99", state.ErrNewerStateVersion)
	loaded, err := LoadQueueState(QueueLoader{
		ProjectName: "demo",
		Queue:       func() (model.Queue, error) { return model.Queue{}, nil },
		Lock:        func() (model.LockInfo, error) { return model.LockInfo{}, assertNotFound{} },
		Runs:        func() ([]string, error) { return []string{"run-1", "run-2", "run-3"}, nil },
		Summary: func(runID string) (model.RunSummary, error) {
			if runID == "run-2" {
				return model.RunSummary{}, newer
			}
			return model.RunSummary{RunID: runID, Status: "finished"}, nil
		},
		Jobs: func(runID string, summary model.RunSummary) ([]Job, error) {
			if runID == "run-3" {
				return nil, fmt.Errorf("%w: commands.json has state version 99", state.ErrNewerStateVersion)
			}
			return []Job{{ID: runID + "-job"}}, nil
		},
		Context: func(string) (model.RunContext, error) { return model.RunContext{}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil {
		t.Fatalf("a run from a newer rotari failed the whole state: %v", err)
	}
	for _, run := range loaded.Runs {
		unreadable := run.RunID == "run-2" || run.RunID == "run-3"
		if unreadable != (run.Status == "unreadable" && run.Unreadable != "" && len(run.Jobs) == 0) {
			t.Fatalf("run %s = %#v", run.RunID, run)
		}
	}
}
func TestLoadJobsProjectsSummaryAndOrigin(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-1")
	writeTestFile(t, filepath.Join(runsDir, "run-0", "job-1", "submitted_at"), "submitted")
	writeTestFile(t, filepath.Join(runsDir, "run-0", "job-1", "finished_at"), "finished")
	origin := &model.JobOrigin{RunID: "run-0", JobID: "job-1", Status: "success"}
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "demo", Stage: "build", Command: []string{"echo", "ok"}, Origin: origin}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" || jobs[0].Result == nil || jobs[0].Result.ExitCode != 0 {
		t.Fatalf("jobs = %#v, want one finished job", jobs)
	}
	if jobs[0].Stage != "build" || jobs[0].Origin != origin || jobs[0].SubmittedAt != "submitted" || jobs[0].FinishedAt != "finished" {
		t.Fatalf("job projection = %#v, want origin and timestamps", jobs[0])
	}
}

func TestLoadJobsProjectsRecordedExecutionStatus(t *testing.T) {
	runID := "20261009-120000-12345678"
	runDir := filepath.Join(t.TempDir(), runID)
	attemptID := state.MakeAttemptID(runID, "running", 0)
	writeTestFile(t, filepath.Join(runDir, "running", "attempts", attemptID, "status.json"), `{"phase":"running","hosts":["node1"]}`)
	writeTestFile(t, filepath.Join(runDir, "unknown", "attempts", state.MakeAttemptID(runID, "unknown", 0), "command.json"), `{}`)
	jobs, err := LoadJobs(state.NewStore(0o700, 0o600), runDir, model.Queue{Commands: []model.QueuedCommand{
		{ID: "running", Command: []string{"sleep", "10"}},
		{ID: "unknown", Command: []string{"true"}},
		{ID: "not-started", Command: []string{"true"}},
	}}, model.RunSummary{}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"running (recorded)", "unknown", "unknown"}
	for index, status := range want {
		if jobs[index].ExecutionStatus != status {
			t.Errorf("job %s execution status = %q, want %q", jobs[index].ID, jobs[index].ExecutionStatus, status)
		}
	}
}

func TestBuildTimelineIncludesRerunAttemptEventsWhenJobHasOrigin(t *testing.T) {
	runID := "run-1"
	runDir := filepath.Join(t.TempDir(), runID)
	attemptID := state.MakeAttemptID(runID, "job-1", 1)
	attemptDir := filepath.Join(runDir, "job-1", "attempts", attemptID)
	writeTestFile(t, filepath.Join(attemptDir, "submitted_at"), "2026-10-02T10:00:00Z")
	writeTestFile(t, filepath.Join(attemptDir, "finished_at"), "2026-10-02T10:00:05Z")
	writeTestFile(t, filepath.Join(attemptDir, "status"), "0")

	origin := &model.JobOrigin{RunID: "run-0", JobID: "job-1", AttemptID: "previous-attempt", Status: "success"}
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"echo", "ok"}, Origin: origin}}},
		model.RunSummary{RunID: runID, StartedAt: "2026-10-02T09:59:00Z", FinishedAt: "2026-10-02T10:00:05Z", Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	points := buildTimeline(model.RunSummary{StartedAt: "2026-10-02T09:59:00Z"}, jobs, nil)
	if len(points) != 3 {
		t.Fatalf("timeline = %#v, want start, submission, and completion points", points)
	}
	if points[0].Pending != 1 || points[1].Running != 1 || points[2].Success != 1 {
		t.Fatalf("timeline = %#v, want the rerun job to move pending → running → success", points)
	}
}

func TestBuildTimelineDoesNotUseOriginTimesForBlockedJobs(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-2")
	writeTestFile(t, filepath.Join(runsDir, "run-1", "job-1", "submitted_at"), "2026-10-02T09:00:00Z")
	writeTestFile(t, filepath.Join(runsDir, "run-1", "job-1", "finished_at"), "2026-10-02T09:00:01Z")

	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1", AttemptID: "old-attempt", SubmittedAt: "2026-10-02T09:00:00Z", FinishedAt: "2026-10-02T09:00:01Z"}
	startedAt := "2026-10-02T10:00:00Z"
	finishedAt := "2026-10-02T10:00:02Z"
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"echo", "blocked"}, Origin: origin}}},
		model.RunSummary{RunID: "run-2", StartedAt: startedAt, FinishedAt: finishedAt, Results: []model.JobResult{{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].SubmittedAt != "" || jobs[0].FinishedAt != "" {
		t.Fatalf("blocked job times = %q, %q; want no timestamps inherited from its origin", jobs[0].SubmittedAt, jobs[0].FinishedAt)
	}

	points := buildTimeline(model.RunSummary{StartedAt: startedAt, FinishedAt: finishedAt}, jobs, nil)
	if len(points) != 2 || points[0].At != startedAt || points[1].At != finishedAt {
		t.Fatalf("timeline = %#v, want monotonic run-start and run-finish points", points)
	}
}

func TestBuildTimelineUsesRunStartSampleWhenSummaryStartedAtIsLate(t *testing.T) {
	startedAt := "2026-10-02T09:00:27.123456789Z"
	jobs := []Job{{
		SubmittedAt: "2026-10-02T09:00:27Z",
		FinishedAt:  "2026-10-02T09:00:39Z",
		Result:      &model.JobResult{ID: "job-1", ExitCode: 0},
	}}
	points := buildTimeline(
		model.RunSummary{StartedAt: "2026-10-02T09:00:39Z", FinishedAt: "2026-10-02T09:00:39Z"},
		jobs,
		[]model.LoadSample{{At: startedAt}},
	)
	if points[0].At != "2026-10-02T09:00:27Z" {
		t.Fatalf("initial point time = %q, want run-start sample normalized to seconds", points[0].At)
	}
}

func TestLoadJobsPrefersAttemptStatusOverSummary(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260925-000000-00000000")
	writeTestFile(t, filepath.Join(runDir, "job-1", "status"), "3")
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"false"}}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0, Hosts: []string{"node1"}}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || jobs[0].Result.ExitCode != 3 || len(jobs[0].Result.Hosts) != 1 {
		t.Fatalf("jobs = %#v, want attempt exit code with summary metadata", jobs)
	}
}

func TestLoadJobsSelectedAttemptUsesItsOwnOutcome(t *testing.T) {
	runID := "20260925-000000-00000000"
	runDir := filepath.Join(t.TempDir(), runID)
	first := state.MakeAttemptID(runID, "job-1", 1)
	second := state.MakeAttemptID(runID, "job-1", 2)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", first, "status.json"), `{"phase":"finished","exit_code":2,"finished_at":"first-finish"}`)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", second, "status"), "0")
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", AttemptID: second, ExitCode: 0}}},
		first,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].AttemptID != first || jobs[0].Result == nil || jobs[0].Result.ExitCode != 2 || jobs[0].Result.AttemptID != first {
		t.Fatalf("jobs = %#v, want selected attempt outcome", jobs)
	}
	if jobs[0].FinishedAt != "first-finish" {
		t.Fatalf("finished at = %q, want selected attempt wrapper time", jobs[0].FinishedAt)
	}
	if len(jobs[0].Attempts) != 2 || jobs[0].Attempts[0].ID != second || jobs[0].Attempts[1].Result == nil || jobs[0].Attempts[1].Result.ExitCode != 2 {
		t.Fatalf("attempts = %#v, want newest first with results", jobs[0].Attempts)
	}
}

func TestLoadJobsSelectedOlderAttemptIgnoresSummary(t *testing.T) {
	runID := "20260925-000000-00000000"
	runDir := filepath.Join(t.TempDir(), runID)
	first := state.MakeAttemptID(runID, "job-1", 1)
	second := state.MakeAttemptID(runID, "job-1", 2)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", first, "status.json"), `{"phase":"running"}`)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", second, "status"), "0")
	summary := model.RunSummary{Results: []model.JobResult{{ID: "job-1", AttemptID: second, ExitCode: 0, Hosts: []string{"node2"}}}}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	jobs, err := LoadJobs(state.NewStore(0o700, 0o600), runDir, queue, summary, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result != nil {
		t.Fatalf("jobs = %#v, want unfinished selected older attempt without summary result", jobs)
	}
	jobs, err = LoadJobs(state.NewStore(0o700, 0o600), runDir, queue, summary, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || jobs[0].Result.AttemptID != second || len(jobs[0].Result.Hosts) != 1 {
		t.Fatalf("jobs = %#v, want selected latest attempt with summary metadata", jobs)
	}
}

func TestLoadJobsMarksOutdatedDiagnosis(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260925-000000-00000000")
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "old", Command: []string{"false"}}, {ID: "current", Command: []string{"false"}}}}
	summary := model.RunSummary{Results: []model.JobResult{
		{ID: "old", ExitCode: 1, DiagnosisStatus: model.DiagnosisNoMatch, DiagnosisRules: "old"},
		{ID: "current", ExitCode: 1, DiagnosisStatus: model.DiagnosisNoMatch, DiagnosisRules: diagnose.RulesVersion()},
	}}
	jobs, err := LoadJobs(state.NewStore(0o700, 0o600), runDir, queue, summary, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || !jobs[0].DiagnosisOutdated || jobs[1].DiagnosisOutdated {
		t.Fatalf("jobs = %#v, want only the old analysis marked outdated", jobs)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadQueueStateFallsBackToRunningSummaryWhenMissing(t *testing.T) {
	state, err := LoadQueueState(QueueLoader{
		ProjectName: "demo",
		Queue:       func() (model.Queue, error) { return model.Queue{}, nil },
		Lock:        func() (model.LockInfo, error) { return model.LockInfo{RunID: "run-1", StartedAt: "started"}, nil },
		Runs:        func() ([]string, error) { return []string{"run-1"}, nil },
		Summary:     func(string) (model.RunSummary, error) { return model.RunSummary{}, assertNotFound{} },
		Jobs: func(runID string, summary model.RunSummary) ([]Job, error) {
			if summary.Status != "running" || summary.RunID != runID || summary.StartedAt != "started" {
				t.Fatalf("fallback summary = %#v", summary)
			}
			return nil, nil
		},
		Context: func(string) (model.RunContext, error) { return model.RunContext{}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Runs) != 1 || state.Runs[0].Status != "running" || !state.Runs[0].Running {
		t.Fatalf("state = %#v, want one running fallback run", state)
	}
}

func TestLoadQueueStateProjectsLifecycleAndClientStatus(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20261009-120000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "finished", ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteRunClientStatus(state.NewStore(0o700, 0o600), runDir, model.RunClientStatus{Mode: model.RunClientModeAsync, State: model.RunClientCompleted, Reason: model.RunClientReasonAsync}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadQueueState(QueueLoader{
		ProjectName: "demo", Paths: paths,
		Queue: func() (model.Queue, error) { return model.Queue{}, nil },
		Lock:  func() (model.LockInfo, error) { return model.LockInfo{}, assertNotFound{} },
		Runs:  func() ([]string, error) { return []string{runID}, nil },
		Summary: func(string) (model.RunSummary, error) {
			return state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
		},
		Jobs:    func(string, model.RunSummary) ([]Job, error) { return nil, nil },
		Context: func(string) (model.RunContext, error) { return model.RunContext{}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil || len(loaded.Runs) != 1 {
		t.Fatalf("LoadQueueState() = %#v, %v", loaded, err)
	}
	if run := loaded.Runs[0]; run.Lifecycle != "finished" || run.ClientStatus.Mode != model.RunClientModeAsync || run.ClientStatus.Reason != model.RunClientReasonAsync {
		t.Fatalf("run projection = %#v; want finished async history", run)
	}
}

type assertNotFound struct{}

func (assertNotFound) Error() string { return "not found" }
