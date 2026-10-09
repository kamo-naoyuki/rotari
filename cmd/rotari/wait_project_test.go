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
		name   string
		active bool
	}{
		{"finished", false},
		{"active", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envProjectName, "")
			paths, wantID := waitProjectFixture(t, test.active)
			got, _, err := resolveActiveWaitTargets(paths.BaseDir, "demo", nil)
			want := resolve.Run{BaseDir: paths.BaseDir, ProjectName: "demo", RunID: wantID}
			if err != nil || len(got) != 1 || got[0] != want {
				t.Fatalf("wait targets = %#v, err=%v; want %#v", got, err, want)
			}
		})
	}
}

func TestResolveWaitWithoutProjectOptionIgnoresProjectEnvironment(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	t.Setenv(envProjectName, "beta")
	baseDir := t.TempDir()
	for _, projectName := range []string{"alpha", "beta"} {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: projectName + "-run"}); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := resolveActiveWaitTargets(baseDir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []resolve.Run{
		{BaseDir: baseDir, ProjectName: "alpha", RunID: "alpha-run"},
		{BaseDir: baseDir, ProjectName: "beta", RunID: "beta-run"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("wait targets = %#v, want all active projects %#v", got, want)
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
