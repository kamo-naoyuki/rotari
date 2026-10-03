package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveWaitProjectOptionsUseActiveOrLatestRun(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	for _, test := range []struct {
		name    string
		active  bool
		fromEnv bool
	}{
		{"finished/option", false, false},
		{"finished/environment", false, true},
		{"active/option", true, false},
		{"active/environment", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envProjectName, "")
			paths, wantID := waitProjectFixture(t, test.active)
			projectName := "demo"
			if test.fromEnv {
				t.Setenv(envProjectName, projectName)
				projectName = ""
			}
			got, err := resolveActiveWaitTargets(paths.BaseDir, projectName)
			want := resolve.Run{BaseDir: paths.BaseDir, ProjectName: "demo", RunID: wantID}
			if err != nil || len(got) != 1 || got[0] != want {
				t.Fatalf("wait targets = %#v, err=%v; want %#v", got, err, want)
			}
		})
	}
}

func waitProjectFixture(t *testing.T, active bool) (state.ProjectPaths, string) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "finished-run"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, meta.LastRunID), 0o755); err != nil {
		t.Fatal(err)
	}
	if !active {
		return paths, meta.LastRunID
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	return paths, "active-run"
}
