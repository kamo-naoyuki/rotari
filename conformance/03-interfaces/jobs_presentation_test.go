package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestJobsTableKeepsVisibleColumnsAligned(t *testing.T) {
	covers(t, "CLI-2")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	output := e.MustRotari("jobs", "--basedir", e.Base, run.Project, "--format", "%s %a").Stdout
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Fatalf("jobs table has too few rows: %q", output)
	}

	columnStart := strings.Index(lines[0], "ATTEMPT_ID")
	if columnStart < 0 {
		t.Fatalf("jobs table header has no ATTEMPT_ID column: %q", lines[0])
	}
	for _, line := range lines[1:] {
		attemptStart := strings.Index(line, "att_")
		if attemptStart < 0 {
			t.Fatalf("jobs table row has no attempt ID: %q", line)
		}
		if attemptStart != columnStart {
			t.Errorf("jobs table row starts ATTEMPT_ID at %d, want %d: %q", attemptStart, columnStart, line)
		}
	}
}

func TestJobsWindowAcceptsDays(t *testing.T) {
	covers(t, "CLI-6")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	if output := e.MustRotari("jobs", "--basedir", e.Base, run.Project, "--since", "7d").Stdout; !strings.Contains(output, run.BadAttempt) {
		t.Fatalf("jobs --since 7d does not list the finished job %s:\n%s", run.BadAttempt, output)
	}
	if result := e.Rotari("jobs", "--basedir", e.Base, run.Project, "--since", "1.5d"); result.Code == 0 {
		t.Fatalf("jobs --since 1.5d was accepted: %s", result)
	}
	web := e.StartWeb()
	if got := e.HTTPGet(web + "/jobs/?since=7d"); got.Status != 200 || !strings.Contains(got.Body, run.BadAttempt) {
		t.Fatalf("GET /jobs/?since=7d: status %d, want 200 listing %s", got.Status, run.BadAttempt)
	}
}

func TestRunsWindowFiltersSettledHistoryButKeepsActiveRun(t *testing.T) {
	covers(t, "CLI-6")
	e := support.NewEnv(t)
	oldRun, _ := e.FinishedJobRun("history")
	summaryPath := filepath.Join(e.Base, "projects", "history", "runs", oldRun, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	summary["started_at"] = time.Now().Add(-73 * time.Hour).UTC().Format(time.RFC3339Nano)
	summary["finished_at"] = time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339Nano)
	data, err = json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(summaryPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Base, "projects", "history", "meta.json"), []byte(`{"phase":"finished","last_run_id":"`+oldRun+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("add", "-p", "history", "--", "sh", "-c", "sleep 3")
	started := e.MustRotari("run", "-p", "history", "--async").Stdout
	activeRun := ""
	for _, line := range strings.Split(started, "\n") {
		if strings.HasPrefix(line, "  Run: ") {
			activeRun = strings.TrimPrefix(line, "  Run: ")
			break
		}
	}
	if activeRun == "" {
		t.Fatalf("async run output has no run ID: %s", started)
	}
	projectPaths := filepath.Join(e.Base, "projects", "history")
	if err := os.MkdirAll(projectPaths, 0o755); err != nil {
		t.Fatal(err)
	}
	activeSummary := filepath.Join(projectPaths, "runs", activeRun, "summary.json")
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(activeSummary); os.IsNotExist(err) {
			break
		}
		if i == 49 {
			t.Fatalf("async run completed before active status could be checked")
		}
		time.Sleep(10 * time.Millisecond)
	}
	active := e.MustRotari("runs", "--since", "0").Stdout
	if !strings.Contains(active, activeRun) || strings.Contains(active, oldRun) {
		t.Fatalf("runs --since 0 should retain active and omit old settled history:\n%s", active)
	}
	e.MustRotari("wait", "history")
	settled := e.MustRotari("runs").Stdout
	if strings.Contains(settled, oldRun) {
		t.Fatalf("runs default window included old settled history:\n%s", settled)
	}
	wider := e.MustRotari("runs", "--since", "7d").Stdout
	if !strings.Contains(wider, oldRun) {
		t.Fatalf("runs --since 7d omitted old completed run %s:\n%s", oldRun, wider)
	}
}
