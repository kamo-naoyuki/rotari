package runview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestClientStatusSeparatesModeReasonAndLiveness(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20261009-120000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: runID, Host: host, ClientAttached: true}); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o700, 0o600)
	if err := state.WriteRunClientStatus(store, runDir, model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientAttached}); err != nil {
		t.Fatal(err)
	}
	status, err := ClientStatus(paths, runID)
	if err != nil || ClientStatusLabel(status) != "attached" {
		t.Fatalf("active status = %+v, %v; want attached", status, err)
	}
	if err := state.WriteRunClientStatus(store, runDir, model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientDetached, Reason: model.RunClientReasonCtrlD}); err != nil {
		t.Fatal(err)
	}
	status, err = ClientStatus(paths, runID)
	if err != nil || ClientStatusLabel(status) != "detached (Ctrl-D)" {
		t.Fatalf("detached status = %+v, %v; want Ctrl-D reason", status, err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: -1, RunID: runID, Host: host}); err != nil {
		t.Fatal(err)
	}
	status, err = ClientStatus(paths, runID)
	if err != nil || ClientStatusLabel(status) != "unknown (last detached by Ctrl-D)" {
		t.Fatalf("interrupted status = %+v, %v; want unknown with last transition", status, err)
	}
}

func TestClientStatusLabelsAsyncSeparatelyFromCtrlD(t *testing.T) {
	async := model.RunClientStatus{Mode: model.RunClientModeAsync, State: model.RunClientDetached, Reason: model.RunClientReasonAsync}
	ctrlD := model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientDetached, Reason: model.RunClientReasonCtrlD}
	if got := ClientStatusLabel(async); got != "detached (async)" {
		t.Fatalf("async label = %q", got)
	}
	if got := ClientStatusLabel(ctrlD); got != "detached (Ctrl-D)" {
		t.Fatalf("Ctrl-D label = %q", got)
	}
}
