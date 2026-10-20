package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/launcher"
)

// The launch wrapper runs before configuration is loaded, so a broken
// workspace config cannot stop a launched command's wrapper. Without a
// command it only reports its usage; its behavior is tested in
// internal/launcher.
func TestLaunchWrapperRunsBeforeConfiguration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".rotari.toml"), []byte("not toml ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	code, stderr := captureStderr(t, func() int { return run([]string{launcher.WrapperCommand}) })
	if code != 2 || !strings.Contains(stderr, "rotari "+launcher.WrapperCommand+": usage:") || strings.Contains(stderr, "config") {
		t.Fatalf("run(%s) = %d, stderr %q", launcher.WrapperCommand, code, stderr)
	}
}
