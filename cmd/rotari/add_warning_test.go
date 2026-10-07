package main

import (
	"os"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdAddWarnsOnDuplicateFingerprint(t *testing.T) {
	for _, mode := range []string{"normal", "quiet", "dry-run", "refused"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ROTARI_MASTERDIR", t.TempDir())
			baseDir := t.TempDir()
			args := []string{"--basedir", baseDir, "--project-name", "demo"}
			mustAddWarningFixture(t, args)
			paths, err := state.ResolveProjectPaths(baseDir, "demo")
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(paths.QueueFile)
			if err != nil {
				t.Fatal(err)
			}
			queue, err := loadQueue(paths.QueueFile)
			if err != nil {
				t.Fatal(err)
			}
			firstID := queue.Commands[0].ID
			switch mode {
			case "quiet":
				args = append(args, "--quiet")
			case "dry-run":
				args = append(args, "--dry-run")
			case "refused":
				args = append(args, "--if-revision", "stale")
			}
			args = append(args, "--job-name", "second", "--", "true")
			code, stderr := captureStderr(t, func() int { return cmdAdd(args) })
			assertDuplicateAddWarning(t, mode, code, stderr, firstID)
			assertDuplicateAddQueue(t, mode, paths.QueueFile, before, stderr)
		})
	}
}

func mustAddWarningFixture(t *testing.T, args []string) {
	t.Helper()
	if code := cmdAdd(append(append([]string(nil), args...), "--job-name", "first", "--", "true")); code != 0 {
		t.Fatalf("first add exit code = %d", code)
	}
}

func assertDuplicateAddWarning(t *testing.T, mode string, code int, stderr, firstID string) {
	t.Helper()
	if mode == "refused" {
		if code == 0 || strings.Contains(stderr, "same fingerprint") {
			t.Fatalf("refused add = %d, %q", code, stderr)
		}
		return
	}
	if code != 0 || strings.Count(stderr, "warning: jobs have the same fingerprint:") != 1 {
		t.Fatalf("duplicate add = %d, %q", code, stderr)
	}
	for _, label := range []string{"job_id=" + firstID, `job_name="first"`, `job_name="second"`} {
		if !strings.Contains(stderr, label) {
			t.Fatalf("warning %q omitted %q", stderr, label)
		}
	}
}

func assertDuplicateAddQueue(t *testing.T, mode, queueFile string, before []byte, stderr string) {
	t.Helper()
	after, err := os.ReadFile(queueFile)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "dry-run" || mode == "refused" {
		if string(after) != string(before) {
			t.Fatal("preview or refused add changed the queue")
		}
		return
	}
	queue, err := loadQueue(queueFile)
	if err != nil || len(queue.Commands) != 2 {
		t.Fatalf("queue = %+v, err = %v", queue, err)
	}
	if !strings.Contains(stderr, "job_id="+queue.Commands[1].ID) {
		t.Fatalf("warning %q omitted the added job", stderr)
	}
}
