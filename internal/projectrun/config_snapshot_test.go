package projectrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestMergedConfigSnapshotNeverRereadsSources(t *testing.T) {
	base := t.TempDir()
	paths, err := state.ResolveProjectPaths(base, "demo")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "deleted.toml")
	snapshot := &model.FileConfigSnapshot{Content: "[run]\nretry = 2\n", Sources: []model.ConfigSource{{Scope: "global", Path: source}, {Scope: "workspace", Path: source + "-workspace"}}}
	runner := Runner{Store: state.NewStore(0o755, 0o644)}
	if err := runner.WriteContext(paths, "run-1", "/work", "", nil, snapshot); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.RunsDir, "run-1", "configs", "config.toml"))
	if err != nil || string(data) != snapshot.Content {
		t.Fatalf("snapshot = %q, %v", data, err)
	}
	context, err := state.LoadContext(runner.Store, filepath.Join(paths.RunsDir, "run-1"))
	if err != nil || len(context.ConfigSources) != 2 || len(context.ConfigSnapshotFiles) != 1 {
		t.Fatalf("context = %#v, %v", context, err)
	}
	if err := runner.WriteContext(paths, "empty", "/work", "", nil, &model.FileConfigSnapshot{}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(paths.RunsDir, "empty", "configs", "config.toml")); err != nil || len(data) != 0 {
		t.Fatalf("empty = %q, %v", data, err)
	}
}

func TestMergedSnapshotFailureLeavesQueueIdle(t *testing.T) {
	runner, paths := testRunner(t)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"true"}}}}
	if err := runner.Store.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(paths.RunsDir, "run-1", "configs", "config.toml")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	registered := false
	runner.RegisterRun = func(state.ProjectPaths, string) error { registered = true; return nil }
	err := runner.Begin(paths, Start{RunID: "run-1", FileConfig: &model.FileConfigSnapshot{Content: "quiet = false\n"}})
	if err == nil || !strings.Contains(err.Error(), "failed to save run context") {
		t.Fatalf("Begin = %v", err)
	}
	if registered {
		t.Fatal("registered a run before snapshot succeeded")
	}
	if _, err := os.Stat(paths.LockFile); !os.IsNotExist(err) {
		t.Fatalf("run lock: %v", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || meta.Phase == "running" {
		t.Fatalf("meta=%#v,%v", meta, err)
	}
	remaining, err := state.LoadQueue(paths.QueueFile)
	if err != nil || len(remaining.Commands) != 1 || remaining.Commands[0].ID != "queued" {
		t.Fatalf("queue=%#v,%v", remaining, err)
	}
}
