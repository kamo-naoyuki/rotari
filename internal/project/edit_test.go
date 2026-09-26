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

func TestRecoverInterruptedClearsWorkflowImportOnlyWhenDiscarding(t *testing.T) {
	for _, discard := range []bool{false, true} {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.QueueFile, model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Force: true}}}); err != nil {
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
		if discard && (queue.WorkflowImport || len(queue.Commands) != 0) {
			t.Fatalf("discarded queue = %#v", queue)
		}
		if !discard && (!queue.WorkflowImport || len(queue.Commands) != 1 || !queue.Commands[0].Force) {
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
