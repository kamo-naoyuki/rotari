package basedirregistry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterIsIdempotentAndListsBasedir(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	registry := Open(masterDir)
	if err := registry.Register(baseDir); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(baseDir); err != nil {
		t.Fatal(err)
	}
	baseDirs, err := registry.BaseDirs()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(baseDirs) != 1 || baseDirs[0] != absolute {
		t.Fatalf("BaseDirs() = %v, want [%s]", baseDirs, absolute)
	}
}

func TestBaseDirsSkipsMalformedRecords(t *testing.T) {
	masterDir := t.TempDir()
	registry := Open(masterDir)
	if err := os.MkdirAll(registry.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registry.dir, "broken.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registry.dir, "empty.json"), []byte(`{"base_dir":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDirs, err := registry.BaseDirs()
	if err != nil {
		t.Fatal(err)
	}
	if len(baseDirs) != 0 {
		t.Fatalf("BaseDirs() = %v, want empty", baseDirs)
	}
}
