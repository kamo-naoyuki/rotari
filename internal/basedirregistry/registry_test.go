package basedirregistry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/runregistry"
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

func TestMissingAndRemove(t *testing.T) {
	masterDir := t.TempDir()
	registry := Open(masterDir)
	missing := filepath.Join(t.TempDir(), "removed")
	if err := registry.Register(missing); err != nil {
		t.Fatal(err)
	}
	missingDirs, err := registry.Missing()
	if err != nil {
		t.Fatal(err)
	}
	if len(missingDirs) != 1 || missingDirs[0] != missing {
		t.Fatalf("Missing() = %v, want [%s]", missingDirs, missing)
	}
	removed, err := registry.Remove(missing)
	if err != nil || !removed {
		t.Fatalf("Remove() = (%v, %v), want (true, nil)", removed, err)
	}
	missingDirs, err = registry.Missing()
	if err != nil {
		t.Fatal(err)
	}
	if len(missingDirs) != 0 {
		t.Fatalf("Missing() after Remove = %v, want empty", missingDirs)
	}
}

func TestRefNamesTheRecordWithoutThePath(t *testing.T) {
	masterDir := t.TempDir()
	baseDir, _ := filepath.Abs(t.TempDir())
	if err := Open(masterDir).Register(baseDir); err != nil {
		t.Fatal(err)
	}
	ref := Ref(baseDir)
	if _, err := os.Stat(filepath.Join(masterDir, "basedirs", ref+".json")); err != nil {
		t.Fatalf("Ref(%q) = %q does not name the registry record: %v", baseDir, ref, err)
	}
	if len(ref) != 32 || Ref(baseDir) != ref || Ref(baseDir+"-other") == ref {
		t.Fatalf("Ref(%q) = %q, want a stable 32-character key distinct per basedir", baseDir, ref)
	}
}

func TestDiscoverFallsBackToRunRecordsWithoutWriting(t *testing.T) {
	masterDir := t.TempDir()
	baseDir, _ := filepath.Abs(t.TempDir())
	// A master directory from before the basedir registry: one run record.
	if err := runregistry.Open(masterDir).Register(runregistry.Location{BaseDir: baseDir, ProjectName: "demo", RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	baseDirs, fromRuns, err := Discover(masterDir)
	if err != nil || !fromRuns || len(baseDirs) != 1 || baseDirs[0] != baseDir {
		t.Fatalf("Discover() = %v, %v, %v; want [%s] from run records", baseDirs, fromRuns, err, baseDir)
	}
	if _, err := os.Stat(filepath.Join(masterDir, "basedirs")); !os.IsNotExist(err) {
		t.Fatalf("Discover wrote the basedir registry: %v", err)
	}
	if err := Open(masterDir).Register(baseDir); err != nil {
		t.Fatal(err)
	}
	if baseDirs, fromRuns, err := Discover(masterDir); err != nil || fromRuns || len(baseDirs) != 1 {
		t.Fatalf("Discover() after registering = %v, %v, %v; want the registry", baseDirs, fromRuns, err)
	}
}

func TestFindResolvesARef(t *testing.T) {
	masterDir := t.TempDir()
	baseDir, _ := filepath.Abs(t.TempDir())
	if err := Open(masterDir).Register(baseDir); err != nil {
		t.Fatal(err)
	}
	if got, err := Find(masterDir, Ref(baseDir)); err != nil || got != baseDir {
		t.Fatalf("Find(Ref) = %q, %v; want %q", got, err, baseDir)
	}
	if _, err := Find(masterDir, Ref(baseDir+"-other")); err == nil {
		t.Fatal("Find accepted the ref of an unregistered basedir")
	}
}
