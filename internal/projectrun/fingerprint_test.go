package projectrun

import (
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestMatchFingerprintQueuePrefersJobIDAndMatchesNewIDs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	sourceRunID := "source-run"
	sourceDir := filepath.Join(paths.RunsDir, sourceRunID)
	sourceQueue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"old"}},
		{ID: "old", Command: []string{"echo", "same"}},
	}}
	if err := state.WriteJSON(filepath.Join(sourceDir, "commands.json"), sourceQueue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(sourceDir, "summary.json"), model.RunSummary{RunID: sourceRunID, Results: []model.JobResult{
		{ID: "same", AttemptID: "same-attempt", ExitCode: 0},
		{ID: "old", AttemptID: "old-attempt", ExitCode: 0},
	}}); err != nil {
		t.Fatal(err)
	}
	current := model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"changed"}},
		{ID: "new", Command: []string{"echo", "same"}},
	}}
	runner := Runner{Store: state.NewStore(state.DirectoryMode(), state.FileMode())}
	matched, err := runner.MatchFingerprintQueue(paths, current, model.MatchByIDAndFingerprint, sourceRunID)
	if err != nil {
		t.Fatal(err)
	}
	if matched.Commands[0].Origin == nil || matched.Commands[0].Origin.JobID != "same" {
		t.Fatalf("Job ID match was not preserved: %#v", matched.Commands[0])
	}
	if matched.Commands[1].Origin == nil || matched.Commands[1].Origin.JobID != "old" {
		t.Fatalf("fingerprint match was not applied: %#v", matched.Commands[1])
	}
}

func TestMatchFingerprintQueueLeavesCountMismatchNew(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	sourceRunID := "source-run"
	sourceDir := filepath.Join(paths.RunsDir, sourceRunID)
	if err := state.WriteJSON(filepath.Join(sourceDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "old", Command: []string{"echo", "same"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(sourceDir, "summary.json"), model.RunSummary{RunID: sourceRunID, Results: []model.JobResult{{ID: "old", AttemptID: "old-attempt", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	current := model.Queue{Commands: []model.QueuedCommand{
		{ID: "new-1", Command: []string{"echo", "same"}},
		{ID: "new-2", Command: []string{"echo", "same"}},
	}}
	runner := Runner{Store: state.NewStore(state.DirectoryMode(), state.FileMode())}
	matched, err := runner.MatchFingerprintQueue(paths, current, model.MatchByFingerprint, sourceRunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range matched.Commands {
		if command.Origin != nil {
			t.Fatalf("count-mismatched command was matched: %#v", command)
		}
	}
}

func TestMatchFingerprintQueueProtectsExistingOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	sourceRunID := "source-run"
	sourceDir := filepath.Join(paths.RunsDir, sourceRunID)
	if err := state.WriteJSON(filepath.Join(sourceDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "source", Command: []string{"echo", "same"}},
		{ID: "other", Command: []string{"echo", "same"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(sourceDir, "summary.json"), model.RunSummary{RunID: sourceRunID, Results: []model.JobResult{
		{ID: "source", AttemptID: "source-attempt", ExitCode: 0},
		{ID: "other", AttemptID: "other-attempt", ExitCode: 0},
	}}); err != nil {
		t.Fatal(err)
	}
	current := model.Queue{Commands: []model.QueuedCommand{
		{ID: "protected", Command: []string{"changed"}, Origin: &model.JobOrigin{RunID: sourceRunID, JobID: "source"}},
		{ID: "new", Command: []string{"echo", "same"}},
	}}
	runner := Runner{Store: state.NewStore(state.DirectoryMode(), state.FileMode())}
	matched, err := runner.MatchFingerprintQueue(paths, current, model.MatchByFingerprint, sourceRunID)
	if err != nil {
		t.Fatal(err)
	}
	if matched.Commands[0].Origin.JobID != "source" || matched.Commands[1].Origin == nil || matched.Commands[1].Origin.JobID != "other" {
		t.Fatalf("origins = %#v", matched.Commands)
	}
}

func TestClassifyDefinitionsComparesQueueWithReferenceRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "ref", "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"true"}},
		{ID: "edited", Command: []string{"old"}},
	}}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"true"}},
		{ID: "edited", Command: []string{"new"}},
		{ID: "added", Command: []string{"added"}},
	}}
	filter, err := ClassifyDefinitions(paths, queue, jobfilter.Filter{Changed: true}, "ref", "")
	if err != nil {
		t.Fatal(err)
	}
	if !filter.MatchesDefinition("edited") || filter.MatchesDefinition("same") || filter.MatchesDefinition("added") {
		t.Fatalf("changed = %#v", filter.ChangedIDs)
	}
	filter, err = ClassifyDefinitions(paths, queue, jobfilter.Filter{New: true}, "ref", "")
	if err != nil {
		t.Fatal(err)
	}
	if !filter.MatchesDefinition("added") || filter.MatchesDefinition("same") || filter.MatchesDefinition("edited") {
		t.Fatalf("new = %#v", filter.NewIDs)
	}
	filter, err = ClassifyDefinitions(paths, queue, jobfilter.Filter{New: true}, "", "")
	if err != nil || !filter.MatchesDefinition("same") {
		t.Fatalf("without a reference run every job is new: %#v, %v", filter.NewIDs, err)
	}
}
