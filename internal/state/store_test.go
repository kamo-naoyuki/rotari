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
}

func TestLoadQueueMissingReturnsEmptyQueue(t *testing.T) {
	queue, err := LoadQueue(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(queue.Commands) != 0 {
		t.Fatalf("missing queue = %#v, %v", queue, err)
	}
}
