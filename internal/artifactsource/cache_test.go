package artifactsource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
)

func referenceValues(references []artifact.ConfigReference) string {
	var values []string
	for _, reference := range references {
		values = append(values, reference.Location+"="+reference.Value)
	}
	return strings.Join(values, ",")
}

// TestCacheParsesAFileOncePerVersion checks that a file is parsed once for
// a given path, size, and modification time, and again when any changes.
func TestCacheParsesAFileOncePerVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.yaml")
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	write := func(content string, modified time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	cache := NewCache()
	write("out_dir: aaa\n", stamp)
	references, err := cache.References(path)
	if err != nil || referenceValues(references) != "out_dir=aaa" {
		t.Fatalf("first References = %q, %v", referenceValues(references), err)
	}

	// Same size and time: the cached result is returned without parsing.
	write("out_dir: bbb\n", stamp)
	if references, _ := cache.References(path); referenceValues(references) != "out_dir=aaa" {
		t.Fatalf("References with unchanged size and time = %q, want the cached out_dir=aaa", referenceValues(references))
	}
	// A new modification time is a new version.
	write("out_dir: bbb\n", stamp.Add(time.Second))
	if references, _ := cache.References(path); referenceValues(references) != "out_dir=bbb" {
		t.Fatalf("References after modification = %q, want out_dir=bbb", referenceValues(references))
	}
	// So is a new size.
	write("out_dir: cccc\n", stamp.Add(time.Second))
	if references, _ := cache.References(path); referenceValues(references) != "out_dir=cccc" {
		t.Fatalf("References after resize = %q, want out_dir=cccc", referenceValues(references))
	}
	// A cache belongs to one run: another cache parses again.
	if references, _ := NewCache().References(path); referenceValues(references) != "out_dir=cccc" {
		t.Fatalf("new cache References = %q", referenceValues(references))
	}
}

func TestCacheKeepsParseErrorsAndRejectsLargeFilesWithoutReading(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte(`{"a": `), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := NewCache()
	for range 2 {
		if _, err := cache.References(broken); err == nil || !strings.Contains(err.Error(), "cannot parse JSON") {
			t.Fatalf("References(broken) error = %v", err)
		}
	}

	large := filepath.Join(dir, "large.yaml")
	if err := os.WriteFile(large, make([]byte, artifact.MaxSourceBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	// Unreadable: the size alone must reject it, before any read.
	if err := os.Chmod(large, 0o000); err != nil {
		t.Fatal(err)
	}
	_, err := cache.References(large)
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("References(large) error = %v, want a size error before reading", err)
	}

	if _, err := cache.References(filepath.Join(dir, "missing.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("References(missing) error = %v", err)
	}
	if _, err := cache.References(dir); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("References(directory) error = %v", err)
	}
}
