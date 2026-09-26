package queueops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCopyPreservesSourceJobIDs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
		{ID: "test-id", Name: "test", Command: []string{"test"}, DependsOn: []string{"train"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "prepare-id", ExitCode: 0},
		{ID: "train-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	message, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if message == "" {
		t.Fatal("copy message is empty")
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("copied commands = %#v, want one command", queue.Commands)
	}
	copied := queue.Commands[0]
	if copied.ID != "train-id" || copied.Name != "train" || len(copied.DependsOn) != 0 {
		t.Fatalf("copied command = %#v, want the source ID preserved and no excluded dependency", copied)
	}
	if copied.Origin == nil || copied.Origin.RunID != "run-1" || copied.Origin.JobID != "train-id" || copied.Origin.Status != "failed" {
		t.Fatalf("copied origin = %#v, want source run/job and failed status", copied.Origin)
	}
}

func TestCopyPreservesCompleteStageDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-a", Stage: "prepare", Command: []string{"prepare-a"}},
		{ID: "prepare-b", Stage: "prepare", Command: []string{"prepare-b"}},
		{ID: "train", Name: "train", DependsOn: []string{"prepare"}, Command: []string{"train"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "all"}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := queue.Commands[2].DependsOn; len(got) != 1 || got[0] != "prepare" {
		t.Fatalf("copied dependencies = %v, want [prepare]", got)
	}
}

func TestCopyRejectsExcludedFailedDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "prepare-id", ExitCode: 1},
		{ID: "train-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	_, err = testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{"train-id"}})
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "prepare" did not succeed`) {
		t.Fatalf("copy error = %v, want excluded failed dependency error", err)
	}
}

func TestCopyRejectsExcludedUnfinishedDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{{ID: "train-id", ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	_, err = testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{"train-id"}})
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "prepare" did not succeed`) {
		t.Fatalf("copy error = %v, want excluded unfinished dependency error", err)
	}
}

func TestCopyPreservesExplicitAttemptID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := testRunID()
	attemptID := state.MakeAttemptID(runID, "train-id", 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "train-id", Name: "train", Command: []string{"train"}}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, "train-id", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{{ID: "train-id", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", runID, queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{attemptID}}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.AttemptID != attemptID {
		t.Fatalf("copied command = %#v, want explicit attempt %q", queue.Commands, attemptID)
	}
}

func TestCopyExplicitArrayAttemptSelectsOnlyTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := testRunID()
	attemptID := state.MakeAttemptID(runID, "train-2", 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	snapshot := model.Queue{Commands: []model.QueuedCommand{{ID: "train", Command: []string{"train"}, Array: &model.ArraySpec{First: 1, Last: 3}}}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{{ID: "train-1", AttemptID: state.MakeAttemptID(runID, "train-1", 0), ExitCode: 0}, {ID: "train-2", AttemptID: attemptID, ExitCode: 1}, {ID: "train-3", AttemptID: state.MakeAttemptID(runID, "train-3", 0), ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, "train-2", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", runID, queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{attemptID}}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Array == nil || len(queue.Commands[0].Array.Tasks) != 1 || queue.Commands[0].Array.Tasks[0] != 2 {
		t.Fatalf("copied array = %#v, want only task 2", queue.Commands)
	}
	if origin := queue.Commands[0].TaskOrigins["train-2"]; origin == nil || origin.AttemptID != attemptID {
		t.Fatalf("task origin = %#v, want attempt %q", origin, attemptID)
	}
}

func TestCopyReassignsIDOnCollision(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "train-id", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "train-id", Name: "train", Command: []string{"train"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "all", Append: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("commands after append = %#v, want two commands", queue.Commands)
	}
	if queue.Commands[1].ID == "train-id" {
		t.Fatal("colliding job ID was not reassigned")
	}
}

func TestCopyCombinesResultSelections(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success-id", Name: "success", Command: []string{"success"}},
		{ID: "failed-id", Name: "failed", Command: []string{"failed"}},
		{ID: "unfinished-id", Name: "unfinished", Command: []string{"unfinished"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "success-id", ExitCode: 0},
		{ID: "failed-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed,unfinished"}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 || queue.Commands[0].Name != "failed" || queue.Commands[1].Name != "unfinished" {
		t.Fatalf("combined selection = %#v, want failed or unfinished jobs", queue.Commands)
	}
	// A job named directly is not combined with a result filter.
	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed,unfinished", JobIDs: []string{"success-id"}, Overwrite: true}); err == nil || !strings.Contains(err.Error(), "job IDs cannot be combined") {
		t.Fatalf("copy with a filter and a job ID error = %v, want a rejection", err)
	}
}

func TestCopyRequiresAppendForNonEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "all"}); err == nil {
		t.Fatal("copy succeeded without --append")
	}
	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "all", Append: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("commands after append = %#v, want two commands", queue.Commands)
	}
}

func TestCopyCanOverwriteNonEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "all", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Command[0] != "source" {
		t.Fatalf("commands after overwrite = %#v, want only copied command", queue.Commands)
	}
}

func TestCopyAggregatesArrayTaskResults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success-array", Name: "success-array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}},
		{ID: "failed-array", Name: "failed-array", Command: []string{"false"}, Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "success-array-1", AttemptID: state.MakeAttemptID("20260922-000000-00000000", "success-array-1", 0), ExitCode: 0},
		{ID: "success-array-2", AttemptID: state.MakeAttemptID("20260922-000000-00000000", "success-array-2", 0), ExitCode: 0},
		{ID: "failed-array-1", AttemptID: state.MakeAttemptID("20260922-000000-00000000", "failed-array-1", 0), ExitCode: 0},
		{ID: "failed-array-2", AttemptID: state.MakeAttemptID("20260922-000000-00000000", "failed-array-2", 0), ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed"}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "failed-array" {
		t.Fatalf("--failed selection = %#v, want only failed-array (finished array jobs must not be treated as unfinished)", queue.Commands)
	}
	if queue.Commands[0].Origin == nil || queue.Commands[0].Origin.Status != "failed" {
		t.Fatalf("failed-array origin = %#v, want status=failed", queue.Commands[0].Origin)
	}
	if got := queue.Commands[0].TaskOrigins["failed-array-2"].AttemptID; got == "" {
		t.Fatalf("failed-array task origin has no attempt ID")
	}

	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "success", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	queue, err = state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "success-array" || queue.Commands[0].Origin.Status != "success" {
		t.Fatalf("--success selection = %#v, want only success-array with status=success", queue.Commands)
	}
}

func TestCopyRetriesFailedStageMemberWithDependent(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writePartialStageRun(t, paths, []model.JobResult{
		{ID: "a-id", ExitCode: 0},
		{ID: "b-id", ExitCode: 1},
		{ID: "evaluate-id", ExitCode: 1, Error: "blocked by failed dependency"},
	})
	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed"}); err != nil {
		t.Fatalf("copy --failed of a partial stage: %v", err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 || queue.Commands[0].ID != "b-id" || queue.Commands[1].ID != "evaluate-id" {
		t.Fatalf("copied queue = %#v", queue.Commands)
	}
	if got := queue.Commands[1].DependsOn; len(got) != 1 || got[0] != "compute" {
		t.Fatalf("copied dependencies = %v, want [compute]", got)
	}
	// The stage name now resolves to the copied member only, so evaluate
	// waits for the re-executed b.
	jobs := model.QueueToJobs(queue.Commands)
	if got := jobs[1].DependsOn; len(got) != 1 || got[0] != "b" {
		t.Fatalf("resolved dependencies = %v, want [b]", got)
	}
}

func TestCopyRejectsFailedExcludedStageMember(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writePartialStageRun(t, paths, []model.JobResult{
		{ID: "a-id", ExitCode: 1},
		{ID: "b-id", ExitCode: 1},
		{ID: "evaluate-id", ExitCode: 1, Error: "blocked by failed dependency"},
	})
	_, err = testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{"b-id", "evaluate-id"}})
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "a" (stage "compute") did not succeed`) {
		t.Fatalf("copy error = %v, want excluded failed stage member", err)
	}
}

func TestCopyDropsFullyExcludedSuccessfulStage(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writePartialStageRun(t, paths, []model.JobResult{
		{ID: "a-id", ExitCode: 0},
		{ID: "b-id", ExitCode: 0},
		{ID: "evaluate-id", ExitCode: 1},
	})
	if _, err := testEditor().Copy(baseDir, "default", "run-1", queueedit.CopyRequest{Selection: "failed"}); err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || len(queue.Commands[0].DependsOn) != 0 {
		t.Fatalf("copied queue = %#v", queue.Commands)
	}
}

func writePartialStageRun(t *testing.T, paths state.ProjectPaths, results []model.JobResult) {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "a-id", Name: "a", Stage: "compute", Command: []string{"a"}},
		{ID: "b-id", Name: "b", Stage: "compute", Command: []string{"b"}},
		{ID: "evaluate-id", Name: "evaluate", DependsOn: []string{"compute"}, Command: []string{"evaluate"}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: results}); err != nil {
		t.Fatal(err)
	}
}
