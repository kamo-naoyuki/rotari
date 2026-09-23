package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestLoadMetaDefaultsMissingAndEmptyFields(t *testing.T) {
	missing, err := LoadMeta(filepath.Join(t.TempDir(), "meta.json"))
	if err != nil || missing.Phase != "collecting" || missing.UpdatedAt == "" {
		t.Fatalf("missing meta = %#v, %v", missing, err)
	}

	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMeta(path)
	if err != nil || loaded.Phase != "collecting" || loaded.UpdatedAt == "" {
		t.Fatalf("empty meta = %#v, %v", loaded, err)
	}
}

func TestLoadStateReturnsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadQueue(path)
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("LoadQueue error = %v, want ErrInvalidJSON", err)
	}
}

func TestWriteJSONCreatesParentAndReadableDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "value.json")
	if err := WriteJSON(path, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") || !strings.Contains(string(data), "job-1") {
		t.Fatalf("written JSON = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("written JSON mode = %o, want 600", got)
	}
}

func TestRequireRunStateFileRequiresRegularValidatedFiles(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequireRunStateFile(runDir, "context.json", "run-1"); err != nil {
		t.Fatalf("RequireRunStateFile() error = %v", err)
	}
	if err := RequireRunStateFile(runDir, "missing.json", "run-1"); err == nil {
		t.Fatal("RequireRunStateFile() accepted an invalid state filename")
	}
}

func TestValidateRunDirectoryChecksRequiredSnapshots(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRunDirectory(runsDir, "run-1", false); err != nil {
		t.Fatalf("ValidateRunDirectory() error = %v", err)
	}
	if _, err := ValidateRunDirectory(runsDir, "run-1", true); err == nil {
		t.Fatal("ValidateRunDirectory() accepted interrupted run without commands snapshot")
	}
}

func TestListRunJobDirsIncludesOnlyJobSnapshots(t *testing.T) {
	runDir := t.TempDir()
	for _, name := range []string{"job-2", "job-1", "without-command"} {
		if err := os.MkdirAll(filepath.Join(runDir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"job-1", "job-2"} {
		if err := os.WriteFile(filepath.Join(runDir, name, "command.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListRunJobDirs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(runDir, "job-1"), filepath.Join(runDir, "job-2")}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ListRunJobDirs() = %#v, want %#v", got, want)
	}
}

func TestLoadQueueMissingReturnsEmptyQueue(t *testing.T) {
	queue, err := LoadQueue(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(queue.Commands) != 0 {
		t.Fatalf("missing queue = %#v, %v", queue, err)
	}
}
