package launcher

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trustedDir returns a directory whose ancestors pass the checks, so each
// test controls the only insecure element.
func trustedDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "probe")
	writeFile(t, probe, 0o600)
	file, err := OpenTrustedFile(probe)
	if err != nil {
		t.Skipf("the test directory's ancestors are not trusted: %v", err)
	}
	file.Close()
	return dir
}

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("[launcher]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestOpenTrustedFileAcceptsPrivateFiles(t *testing.T) {
	dir := trustedDir(t)
	mkdir(t, filepath.Join(dir, "config"), 0o755)
	path := filepath.Join(dir, "config", "launchers.toml")
	writeFile(t, path, 0o644)
	if err := os.Symlink("config/launchers.toml", filepath.Join(dir, "link.toml")); err != nil {
		t.Fatal(err)
	}
	mkdir(t, filepath.Join(dir, "sticky"), 0o777|fs.ModeSticky)
	writeFile(t, filepath.Join(dir, "sticky", "launchers.toml"), 0o600)
	for _, path := range []string{path, filepath.Join(dir, "link.toml"), filepath.Join(dir, "sticky", "launchers.toml")} {
		file, err := OpenTrustedFile(path)
		if err != nil {
			t.Errorf("OpenTrustedFile(%s) = %v", path, err)
			continue
		}
		file.Close()
	}
}

func TestOpenTrustedFileRejectsWritableElements(t *testing.T) {
	dir := trustedDir(t)
	mkdir(t, filepath.Join(dir, "open"), 0o775)
	writeFile(t, filepath.Join(dir, "open", "launchers.toml"), 0o600)
	writeFile(t, filepath.Join(dir, "group.toml"), 0o620)
	writeFile(t, filepath.Join(dir, "other.toml"), 0o602)
	// A trusted link to a file in a writable directory, and a link in a
	// writable directory to a trusted file.
	writeFile(t, filepath.Join(dir, "target.toml"), 0o600)
	if err := os.Symlink(filepath.Join(dir, "open", "launchers.toml"), filepath.Join(dir, "to-open.toml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "target.toml"), filepath.Join(dir, "open", "link.toml")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"open/launchers.toml": "open is writable by group or others",
		"group.toml":          "group.toml is writable by group or others",
		"other.toml":          "other.toml is writable by group or others",
		"to-open.toml":        "open is writable by group or others",
		"open/link.toml":      "open is writable by group or others",
	} {
		file, err := OpenTrustedFile(filepath.Join(dir, name))
		if err == nil {
			file.Close()
			t.Errorf("OpenTrustedFile(%s) succeeded", name)
			continue
		}
		if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "chmod go-w") {
			t.Errorf("OpenTrustedFile(%s) = %v; want %q with a fix", name, err, want)
		}
	}
}

func TestOpenTrustedFileReportsMissingFile(t *testing.T) {
	dir := trustedDir(t)
	_, err := OpenTrustedFile(filepath.Join(dir, "launchers.toml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("OpenTrustedFile() = %v, want fs.ErrNotExist", err)
	}
}

func TestOpenTrustedFileRejectsOtherFiles(t *testing.T) {
	dir := trustedDir(t)
	mkdir(t, filepath.Join(dir, "directory"), 0o700)
	if err := os.Symlink("loop", filepath.Join(dir, "loop")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"relative.toml", filepath.Join(dir, "directory"), filepath.Join(dir, "loop")} {
		if file, err := OpenTrustedFile(path); err == nil {
			file.Close()
			t.Errorf("OpenTrustedFile(%s) succeeded", path)
		}
	}
}
