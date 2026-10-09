package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestInfoReportsContextAndLeavesStaleLockUntouched(t *testing.T) {
	covers(t, "CLI-21")
	e := support.NewEnv(t)
	if err := os.MkdirAll(e.Base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Base, "config.json"), []byte(`{"executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("add", "--project-name", "demo", "--", "true")
	lockPath := filepath.Join(e.Base, "projects", "demo", "running.lock")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	lock := []byte(`{"pid":999999999,"run_id":"stale-run","host":"` + host + `"}`)
	if err := os.WriteFile(lockPath, lock, 0o644); err != nil {
		t.Fatal(err)
	}

	result := e.MustRotari("info", "--json")
	var report struct {
		MasterDir string `json:"masterdir"`
		BaseDir   string `json:"basedir"`
		Project   string `json:"project"`
		RunLocks  []struct {
			State string `json:"state"`
			RunID string `json:"run_id"`
		} `json:"run_locks"`
		JobLivenessNote string `json:"job_liveness_note"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, result.Stdout)
	}
	if report.MasterDir != e.Master || report.BaseDir != e.Base || report.Project != "demo" {
		t.Fatalf("resolved context = master %q, basedir %q, project %q", report.MasterDir, report.BaseDir, report.Project)
	}
	if len(report.RunLocks) != 1 || report.RunLocks[0].State != "stale" || report.RunLocks[0].RunID != "stale-run" {
		t.Fatalf("run locks = %#v, want stale-run", report.RunLocks)
	}
	if report.JobLivenessNote == "" {
		t.Fatal("info did not clarify the limit of job-process liveness checks")
	}
	gotLock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("info removed the stale lock: %v", err)
	}
	if string(gotLock) != string(lock) {
		t.Fatalf("info changed stale lock: got %s, want %s", gotLock, lock)
	}
}
