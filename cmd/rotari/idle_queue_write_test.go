package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeIdleQueueFixture(t *testing.T) state.ProjectPaths {
	t.Helper()
	paths, err := resolvePaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "previous", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	return paths
}

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
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" || meta.LastRunID != "run-1" {
		t.Fatalf("meta = %#v", meta)
	}
	if queue := loadCarryStateQueue(t, paths); len(queue.Commands) != 1 || queue.Commands[0].ID != "next" {
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
	if err := ensureProjectIdleForPaths(paths, "add"); err != nil {
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

func TestMarkProjectCollectingLeavesQueueUntouched(t *testing.T) {
	paths := writeIdleQueueFixture(t)
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := markProjectCollecting(paths); err != nil {
		t.Fatal(err)
	}
	if queueAfter, _ := os.ReadFile(paths.QueueFile); string(queueAfter) != string(queueBefore) {
		t.Fatalf("queue changed:\n%s", queueAfter)
	}
	if meta, err := loadMeta(paths.MetaFile); err != nil || meta.Phase != "collecting" {
		t.Fatalf("meta = %#v, %v", meta, err)
	}
}

func TestIdleQueueCommandsMarkProjectCollecting(t *testing.T) {
	tests := map[string]func(*testing.T, string, state.ProjectPaths){
		"add": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			if _, err := enqueueCommand(baseDir, "default", []string{"added"}, "", nil, nil, "", nil); err != nil {
				t.Fatal(err)
			}
		},
		"change": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			if _, err := changeBatch(baseDir, "default", "", "previous", "", "", nil, false, nil, false, "", nil, false, []string{"changed"}); err != nil {
				t.Fatal(err)
			}
		},
		"remove": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "previous", Command: []string{"true"}}, {ID: "other", Command: []string{"true"}}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := removeBatch(baseDir, "default", "", []string{"previous"}, ""); err != nil {
				t.Fatal(err)
			}
		},
		"reset": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			if _, err := resetQueueCommands(paths); err != nil {
				t.Fatal(err)
			}
		},
		"copy": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			writeCarryStateRun(t, paths, "run-1", model.Queue{Commands: []model.QueuedCommand{{ID: "copied", Command: []string{"true"}}}}, []model.JobResult{{ID: "copied"}})
			if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, false, true); err != nil {
				t.Fatal(err)
			}
		},
		"import": func(t *testing.T, baseDir string, paths state.ProjectPaths) {
			manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [imported]\n")
			if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "default", "--overwrite", manifest}); code != 0 {
				t.Fatalf("cmdImport exit code = %d", code)
			}
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			paths := writeIdleQueueFixture(t)
			run(t, paths.BaseDir, paths)
			meta, err := loadMeta(paths.MetaFile)
			if err != nil {
				t.Fatal(err)
			}
			if meta.Phase != "collecting" || meta.UpdatedAt == "" {
				t.Fatalf("meta after %s = %#v", name, meta)
			}
		})
	}
}
