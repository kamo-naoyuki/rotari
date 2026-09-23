package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveTerminalExitCodePrefersStatusFiles(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "status"), []byte("7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := ResolveTerminalExitCode(store, jobDir); !ok || got != 7 {
		t.Fatalf("ResolveTerminalExitCode() = %d, %t, want 7, true", got, ok)
	}
}
