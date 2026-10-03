package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWaitRunReturnsAtOnceForASettledRun(t *testing.T) {
	f := newToolFixture(t)
	started := time.Now()
	output, err := waitRun(context.Background(), f.masterDir, WaitRunInput{RunID: f.firstRun, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	if output.Reason != "settled" || output.State != "finished" || output.Summary.Counts.Failed != 3 || time.Since(started) > 2*time.Second {
		t.Fatalf("wait for a finished run = %+v after %s", output, time.Since(started))
	}
}

func TestWaitRunTimesOutOnARunningRunAndHonorsCancellation(t *testing.T) {
	f := newToolFixture(t)
	paths, err := state.ResolveProjectPaths(f.firstBaseDir, "exp")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// The second run is still running: it holds the lock and has no summary.
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: f.secondRun, Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: f.secondRun}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(paths.RunsDir, f.secondRun, "summary.json")); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	output, err := waitRun(context.Background(), f.masterDir, WaitRunInput{RunID: f.secondRun, TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); output.Reason != "timeout" || output.State != "running" || elapsed < time.Second {
		t.Fatalf("wait for a running run = %+v after %s", output, elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := waitRun(ctx, f.masterDir, WaitRunInput{RunID: f.secondRun, TimeoutSeconds: 60}); err == nil || time.Since(started) > 10*time.Second {
		t.Fatalf("a cancelled wait returned %v", err)
	}
}

func TestWaitRunRejectsTimeoutsOutOfRange(t *testing.T) {
	f := newToolFixture(t)
	for _, seconds := range []int{-1, maxWaitSeconds + 1} {
		if _, err := waitRun(context.Background(), f.masterDir, WaitRunInput{RunID: f.firstRun, TimeoutSeconds: seconds}); err == nil || !strings.Contains(err.Error(), "timeout_seconds") {
			t.Errorf("timeout_seconds %d: error %v", seconds, err)
		}
	}
}
