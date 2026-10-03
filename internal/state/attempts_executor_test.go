package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAttemptExecutorReadsTheSubmittedCommand(t *testing.T) {
	attemptDir := t.TempDir()
	if got := ReadAttemptExecutor(attemptDir); got != "" {
		t.Fatalf("an attempt without command.json = %q", got)
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "command.json"), []byte(`{"id":"job","executor":"slurm"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadAttemptExecutor(attemptDir); got != "slurm" {
		t.Fatalf("ReadAttemptExecutor = %q, want slurm", got)
	}
}
