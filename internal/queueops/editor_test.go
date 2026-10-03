package queueops

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
)

// TestAddRegistersTheBaseDirOnlyWhenApplied adds a job to a new project as a
// dry run and then for real; only the applied add registers its basedir.
func TestAddRegistersTheBaseDirOnlyWhenApplied(t *testing.T) {
	baseDir := t.TempDir()
	var registered []string
	editor := testEditor()
	editor.RegisterBaseDir = func(baseDir string) error {
		registered = append(registered, baseDir)
		return nil
	}
	add := func(guard project.Guard) {
		t.Helper()
		editor.Guard = guard
		if _, err := editor.Add(baseDir, "fresh", []model.QueuedCommand{{Command: []string{"true"}}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	add(project.Guard{DryRun: true})
	if len(registered) != 0 {
		t.Fatalf("a dry run registered %q", registered)
	}
	add(project.Guard{})
	if len(registered) != 1 || registered[0] != baseDir {
		t.Fatalf("registered %q, want %q", registered, baseDir)
	}
}
