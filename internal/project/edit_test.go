package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWriteIdleQueueWritesMetadataBeforeQueue(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	var order []string
	writer := func(path string, value any) error {
		order = append(order, path)
		return state.WriteJSON(path, value)
	}
	if err := writeIdleQueueWith(writer, paths, &model.Queue{Commands: []model.QueuedCommand{{ID: "next", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{paths.MetaFile, paths.QueueFile}) {
		t.Fatalf("write order = %#v", order)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" || meta.LastRunID != "run-1" {
		t.Fatalf("meta = %#v", meta)
	}
	if queue := loadQueueForTest(t, paths); len(queue.Commands) != 1 || queue.Commands[0].ID != "next" {
		t.Fatalf("queue = %#v", queue)
	}
}

func TestWriteIdleQueueFailureKeepsPreviousQueue(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	writer := func(path string, value any) error {
		if path == paths.QueueFile {
			return errors.New("disk full")
		}
		return state.WriteJSON(path, value)
	}
	err = writeIdleQueueWith(writer, paths, &model.Queue{Commands: []model.QueuedCommand{{ID: "next", Command: []string{"true"}}}})
	if err == nil || !strings.Contains(err.Error(), "failed to write queue") {
		t.Fatalf("writeIdleQueueWith error = %v", err)
	}
	if queueAfter, _ := os.ReadFile(paths.QueueFile); string(queueAfter) != string(queueBefore) {
		t.Fatalf("failed write replaced the queue:\n%s", queueAfter)
	}
	// The metadata was already updated, which leaves a valid idle project.
	if err := EnsureIdle(paths, "add"); err != nil {
		t.Fatalf("project is not idle after a failed queue write: %v", err)
	}
}

func TestWriteIdleQueueMetadataFailureSkipsQueue(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	var wrote []string
	writer := func(path string, value any) error {
		wrote = append(wrote, path)
		return errors.New("disk full")
	}
	err := writeIdleQueueWith(writer, paths, &model.Queue{})
	if err == nil || !strings.Contains(err.Error(), "failed to update metadata") {
		t.Fatalf("writeIdleQueueWith error = %v", err)
	}
	if !reflect.DeepEqual(wrote, []string{paths.MetaFile}) {
		t.Fatalf("attempted writes = %#v", wrote)
	}
}

func TestMarkCollectingLeavesQueueUntouched(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := MarkCollecting(paths); err != nil {
		t.Fatal(err)
	}
	if queueAfter, _ := os.ReadFile(paths.QueueFile); string(queueAfter) != string(queueBefore) {
		t.Fatalf("queue changed:\n%s", queueAfter)
	}
	if meta, err := state.LoadMeta(paths.MetaFile); err != nil || meta.Phase != "collecting" {
		t.Fatalf("meta = %#v, %v", meta, err)
	}
}

func writeIdleQueueFixture(t *testing.T) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "previous", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestRecoverInterruptedKeepsQueueOnlyWithoutDiscarding(t *testing.T) {
	for _, discard := range []bool{false, true} {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, MarkedStatus: model.StatusUnfinished}}}); err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		writeTestRunStateFiles(t, paths, "run-1")
		if err := RecoverInterrupted(paths, "run-1", discard); err != nil {
			t.Fatalf("RecoverInterrupted(discard=%v): %v", discard, err)
		}
		queue := loadQueueForTest(t, paths)
		if discard && len(queue.Commands) != 0 {
			t.Fatalf("discarded queue = %#v", queue)
		}
		if !discard && (len(queue.Commands) != 1 || queue.Commands[0].MarkedStatus != model.StatusUnfinished) {
			t.Fatalf("retained queue = %#v", queue)
		}
	}
}

func loadQueueForTest(t *testing.T, paths state.ProjectPaths) model.Queue {
	t.Helper()
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	return queue
}

func writeTestRunStateFiles(t *testing.T, paths state.ProjectPaths, runID string) {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{}); err != nil {
		t.Fatal(err)
	}
}

func TestEditQueueSavesEditAndMarksCollecting(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	err := EditQueue(paths, "add", func(queue *model.Queue) error {
		queue.Commands = append(queue.Commands, model.QueuedCommand{ID: "next", Command: []string{"true"}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if queue := loadQueueForTest(t, paths); len(queue.Commands) != 2 || queue.Commands[1].ID != "next" {
		t.Fatalf("queue = %#v", queue)
	}
	if meta, err := state.LoadMeta(paths.MetaFile); err != nil || meta.Phase != "collecting" {
		t.Fatalf("meta = %#v, %v", meta, err)
	}
}

func TestEditQueueWritesNothingWhenEditFails(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	err = EditQueue(paths, "add", func(queue *model.Queue) error {
		queue.Commands = nil
		return errors.New("invalid job")
	})
	if err == nil || err.Error() != "invalid job" {
		t.Fatalf("EditQueue error = %v", err)
	}
	if queueAfter, _ := os.ReadFile(paths.QueueFile); string(queueAfter) != string(queueBefore) {
		t.Fatalf("failed edit replaced the queue:\n%s", queueAfter)
	}
	if meta, err := state.LoadMeta(paths.MetaFile); err != nil || meta.Phase != "finished" {
		t.Fatalf("failed edit changed metadata: %#v, %v", meta, err)
	}
}

func TestEditQueueRejectsInterruptedProjectWithoutEditing(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	called := false
	err := EditQueue(paths, "change", func(*model.Queue) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), `has interrupted run "run-1"; change is not allowed`) {
		t.Fatalf("EditQueue error = %v", err)
	}
	if called {
		t.Fatal("edit ran on an interrupted project")
	}
}

func TestEditQueueGuardedPreviewsAndChecksTheRevision(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	appendJob := func(queue *model.Queue) error {
		queue.Commands = append(queue.Commands, model.QueuedCommand{ID: "next", Command: []string{"true"}})
		return nil
	}
	before, err := Revision(paths)
	if err != nil {
		t.Fatal(err)
	}

	// A dry run reports the edited queue and writes nothing.
	var preview Outcome
	if err := EditQueueGuarded(paths, "add", Guard{DryRun: true, Report: func(outcome Outcome) { preview = outcome }}, appendJob); err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.Revision != before || preview.Queue == nil || len(preview.Queue.Commands) != 2 {
		t.Fatalf("dry run outcome = %+v", preview)
	}
	if after, _ := Revision(paths); after != before || len(loadQueueForTest(t, paths).Commands) != 1 {
		t.Fatalf("dry run wrote the project: revision %s -> %s", before, after)
	}

	// The previewed revision applies the edit and yields a new revision.
	var applied Outcome
	if err := EditQueueGuarded(paths, "add", Guard{IfRevision: preview.Revision, Report: func(outcome Outcome) { applied = outcome }}, appendJob); err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.NewRevision == before || len(loadQueueForTest(t, paths).Commands) != 2 {
		t.Fatalf("applied outcome = %+v", applied)
	}
	if now, _ := Revision(paths); now != applied.NewRevision {
		t.Fatalf("NewRevision = %s, project is at %s", applied.NewRevision, now)
	}

	// The old revision is refused, and nothing is written.
	err = EditQueueGuarded(paths, "add", Guard{IfRevision: before}, appendJob)
	if !errors.Is(err, ErrRevisionChanged) || !strings.Contains(err.Error(), before) {
		t.Fatalf("stale revision error = %v", err)
	}
	if len(loadQueueForTest(t, paths).Commands) != 2 {
		t.Fatal("a refused edit wrote the queue")
	}
}

func TestEditGuardedPassesDryRunAndChecksTheRevision(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	var sawDryRun []bool
	edit := func(dryRun bool) error { sawDryRun = append(sawDryRun, dryRun); return nil }
	if err := EditGuarded(paths, "delete", Guard{DryRun: true}, edit); err != nil {
		t.Fatal(err)
	}
	if err := EditGuarded(paths, "delete", Guard{IfRevision: "stale"}, edit); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("stale revision error = %v", err)
	}
	if !reflect.DeepEqual(sawDryRun, []bool{true}) {
		t.Fatalf("edit saw dry runs %v, want one dry run and no call for the refused edit", sawDryRun)
	}
}

func TestRecoverInterruptedGuardedPreviewsAndChecksTheRevision(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	before, err := Revision(paths)
	if err != nil {
		t.Fatal(err)
	}
	var preview Outcome
	if err := RecoverInterruptedGuarded(paths, "run-1", true, Guard{DryRun: true, Report: func(outcome Outcome) { preview = outcome }}); err != nil {
		t.Fatal(err)
	}
	if preview.Revision != before || preview.Applied {
		t.Fatalf("dry run outcome = %+v", preview)
	}
	if now, _ := Revision(paths); now != before || len(loadQueueForTest(t, paths).Commands) != 1 {
		t.Fatal("dry run recovered the interrupted run")
	}
	if err := RecoverInterruptedGuarded(paths, "run-1", true, Guard{IfRevision: "stale"}); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("stale revision error = %v", err)
	}
	if err := RecoverInterruptedGuarded(paths, "run-1", true, Guard{IfRevision: before}); err != nil {
		t.Fatal(err)
	}
	if len(loadQueueForTest(t, paths).Commands) != 0 {
		t.Fatal("recovery at the previewed revision did not discard the queue")
	}
}

func TestCreateQueueGuardedPreviewsANewProjectWithoutCreatingIt(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	appendJob := func(queue *model.Queue) error {
		queue.Commands = append(queue.Commands, model.QueuedCommand{ID: "first", Command: []string{"true"}})
		return nil
	}
	empty, err := Revision(paths)
	if err != nil {
		t.Fatal(err)
	}

	var preview Outcome
	if err := CreateQueueGuarded(paths, "add", Guard{DryRun: true, Report: func(outcome Outcome) { preview = outcome }}, appendJob); err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.Revision != empty || preview.Queue == nil || len(preview.Queue.Commands) != 1 {
		t.Fatalf("dry run outcome = %+v", preview)
	}
	if _, err := os.Stat(paths.ProjectDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry run created the project: %v", err)
	}

	var applied Outcome
	if err := CreateQueueGuarded(paths, "add", Guard{IfRevision: preview.Revision, Report: func(outcome Outcome) { applied = outcome }}, appendJob); err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.NewRevision == empty || len(loadQueueForTest(t, paths).Commands) != 1 {
		t.Fatalf("applied outcome = %+v", applied)
	}
}
