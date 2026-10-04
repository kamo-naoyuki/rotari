package queueops

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestDeleteRunRejectsUnsafeRunIDs(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	for _, runID := range []string{"", "../run-1", "nested/run-1", "run/..", "run/."} {
		if err := (Editor{}).deleteRun(paths, runID, false); err == nil {
			t.Fatalf("deleteRun accepted unsafe run ID %q", runID)
		}
	}
}

// deleteFixture writes runs whose modification times follow runIDs' order
// and a meta naming the last of them, and returns the project paths and an
// editor that records the runs it unregisters.
func deleteFixture(t *testing.T, runIDs ...string) (string, state.ProjectPaths, Editor, *[]string) {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	for i, runID := range runIDs {
		writeCarryStateRun(t, paths, runID, model.Queue{}, nil)
		modTime := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(filepath.Join(paths.RunsDir, runID), modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
	meta := model.Meta{Phase: "finished", LastRunID: runIDs[len(runIDs)-1], LastRunExitCode: 3, UpdatedAt: "2026-09-27T00:00:00Z"}
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}
	var unregistered []string
	editor := testEditor()
	editor.UnregisterRun = func(runID string) error {
		unregistered = append(unregistered, runID)
		return nil
	}
	return baseDir, paths, editor, &unregistered
}

func loadDeleteMeta(t *testing.T, paths state.ProjectPaths) model.Meta {
	t.Helper()
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

func TestDeleteRunMovesLastRunToLatestRemaining(t *testing.T) {
	baseDir, paths, editor, unregistered := deleteFixture(t, "run-a", "run-b", "run-c")
	if err := editor.DeleteRun(baseDir, "default", "run-c"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-c")); !os.IsNotExist(err) {
		t.Fatalf("deleted run directory still exists: %v", err)
	}
	meta := loadDeleteMeta(t, paths)
	if meta.LastRunID != "run-b" || meta.Phase != "collecting" {
		t.Fatalf("meta = %#v, want last run run-b in phase collecting", meta)
	}
	if !reflect.DeepEqual(*unregistered, []string{"run-c"}) {
		t.Fatalf("unregistered = %v, want [run-c]", *unregistered)
	}
}

func TestDeleteRunKeepsLastRunWhenAnotherRunIsDeleted(t *testing.T) {
	baseDir, paths, editor, _ := deleteFixture(t, "run-a", "run-b")
	if err := editor.DeleteRun(baseDir, "default", "run-a"); err != nil {
		t.Fatal(err)
	}
	if meta := loadDeleteMeta(t, paths); meta.LastRunID != "run-b" {
		t.Fatalf("last run = %q, want run-b", meta.LastRunID)
	}
}

func TestDeleteRunRejectsMissingRun(t *testing.T) {
	baseDir, _, editor, unregistered := deleteFixture(t, "run-a")
	err := editor.DeleteRun(baseDir, "default", "run-missing")
	if err == nil || !strings.Contains(err.Error(), `run "run-missing" not found`) {
		t.Fatalf("DeleteRun(missing) error = %v", err)
	}
	if len(*unregistered) != 0 {
		t.Fatalf("missing run unregistered %v", *unregistered)
	}
}

func TestDeleteHistoryOneRun(t *testing.T) {
	baseDir, paths, editor, unregistered := deleteFixture(t, "run-a", "run-b")
	message, err := editor.DeleteHistory(baseDir, "default", "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if message != "cleared logs project=default run=run-a" {
		t.Fatalf("message = %q", message)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-b")); err != nil {
		t.Fatalf("other run was removed: %v", err)
	}
	if !reflect.DeepEqual(*unregistered, []string{"run-a"}) {
		t.Fatalf("unregistered = %v, want [run-a]", *unregistered)
	}
	if _, err := editor.DeleteHistory(baseDir, "default", "run-a"); err == nil || !strings.Contains(err.Error(), `failed to clear run "run-a"`) {
		t.Fatalf("deleting a deleted run error = %v", err)
	}
}

func TestDeleteHistoryAllRuns(t *testing.T) {
	baseDir, paths, editor, unregistered := deleteFixture(t, "run-a", "run-b")
	message, err := editor.DeleteHistory(baseDir, "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := "cleared logs project=default directory=" + filepath.Join(paths.ProjectDir, "runs"); message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
	if _, err := os.Stat(paths.RunsDir); !os.IsNotExist(err) {
		t.Fatalf("runs directory still exists: %v", err)
	}
	meta := loadDeleteMeta(t, paths)
	if meta.LastRunID != "" || meta.LastRunExitCode != 0 || meta.Phase != "collecting" {
		t.Fatalf("meta = %#v, want reset", meta)
	}
	sort.Strings(*unregistered)
	if !reflect.DeepEqual(*unregistered, []string{"run-a", "run-b"}) {
		t.Fatalf("unregistered = %v, want both runs", *unregistered)
	}
}

func TestDeleteHistoryDryRunWritesNothing(t *testing.T) {
	baseDir, paths, editor, unregistered := deleteFixture(t, "run-a", "run-b")
	var outcome project.Outcome
	editor.Guard = project.Guard{DryRun: true, Report: func(o project.Outcome) { outcome = o }}

	message, err := editor.DeleteHistory(baseDir, "default", "")
	if err != nil {
		t.Fatal(err)
	}
	if message != "cleared logs project=default runs=2" {
		t.Fatalf("message = %q", message)
	}
	if _, err := editor.DeleteHistory(baseDir, "default", "run-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := editor.DeleteHistory(baseDir, "default", "run-missing"); err == nil {
		t.Fatal("dry run accepted a missing run")
	}
	for _, runID := range []string{"run-a", "run-b"} {
		if _, err := os.Stat(filepath.Join(paths.RunsDir, runID)); err != nil {
			t.Fatalf("dry run removed %s: %v", runID, err)
		}
	}
	if meta := loadDeleteMeta(t, paths); meta.LastRunID != "run-b" || meta.Phase != "finished" {
		t.Fatalf("dry run changed meta: %#v", meta)
	}
	if len(*unregistered) != 0 || outcome.Applied {
		t.Fatalf("dry run unregistered %v, outcome %#v", *unregistered, outcome)
	}
}

func TestDeleteHistoryRejectsInvalidProject(t *testing.T) {
	if _, err := testEditor().DeleteHistory(t.TempDir(), "bad/name", ""); err == nil {
		t.Fatal("DeleteHistory accepted a project name with a separator")
	}
	if err := testEditor().DeleteRun(t.TempDir(), `bad\name`, "run-a"); err == nil {
		t.Fatal("DeleteRun accepted a project name with a separator")
	}
}
