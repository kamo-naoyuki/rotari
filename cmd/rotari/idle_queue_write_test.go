package main

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeIdleQueueFixture(t *testing.T) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
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
