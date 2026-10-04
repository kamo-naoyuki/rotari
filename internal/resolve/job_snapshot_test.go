package resolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestJobInRunSnapshotErrors(t *testing.T) {
	for _, test := range []struct {
		name, data string
		wantError  bool
		wantNewer  bool
	}{
		{name: "missing snapshot"},
		{name: "missing name", data: `{"commands":[]}`},
		{name: "malformed snapshot", data: `{"commands":`, wantError: true},
		{name: "newer snapshot", data: `{"state_version":999,"commands":[]}`, wantError: true, wantNewer: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			runDir := filepath.Join(paths.RunsDir, "20261005-000000-00000000")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if test.data != "" {
				if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte(test.data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, found, err := JobInRun(paths, filepath.Base(runDir), "train", true)
			if found || (err != nil) != test.wantError || errors.Is(err, state.ErrNewerStateVersion) != test.wantNewer {
				t.Fatalf("JobInRun = found %v, error %v", found, err)
			}
		})
	}
}

func TestActiveRunWithJobsPropagatesSnapshotError(t *testing.T) {
	baseDir := t.TempDir()
	runID := "20261005-000000-00000000"
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: runID, Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte(`{"state_version":999}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = activeRunWithJobs(baseDir, []string{"job-1"})
	if !errors.Is(err, state.ErrNewerStateVersion) || !strings.Contains(err.Error(), "upgrade rotari") {
		t.Fatalf("activeRunWithJobs error = %v", err)
	}
}
