package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCancelAcceptsAnExitedJob(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte(fmt.Sprintf("%d\n", command.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewLocal(testStore(), testLogf).Cancel(jobDir); err != nil {
		t.Fatalf("cancel of a job that already ended: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(jobDir, "cancelled"))
	if err != nil || strings.TrimSpace(string(marker)) == "" {
		t.Fatalf("cancellation marker: %v %q", err, marker)
	}
}
