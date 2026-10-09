package interfaces

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
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
	projectDir := filepath.Dir(lockPath)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	runID := "20261009-120000-00000000"
	lock := []byte(`{"pid":999999999,"run_id":"` + runID + `","host":"` + host + `"}`)
	if err := os.WriteFile(lockPath, lock, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "meta.json"), []byte(`{"phase":"running","last_run_id":"`+runID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(projectDir, "runs", runID)
	jobDir := filepath.Join(runDir, "job-a")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"hostname":"`+host+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "command.json"), []byte(`{"id":"job-a","executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		_ = process.Wait()
	})
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte(strconv.Itoa(process.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := e.MustRotari("info", "--json")
	var report struct {
		MasterDir     string `json:"masterdir"`
		BaseDir       string `json:"basedir"`
		Project       string `json:"project"`
		ProjectExists bool   `json:"project_exists"`
		RunLocks      []struct {
			State string `json:"state"`
			RunID string `json:"run_id"`
		} `json:"run_locks"`
		ActiveRuns []struct {
			RunID string `json:"run_id"`
			Jobs  struct {
				Alive int `json:"alive"`
			} `json:"jobs"`
		} `json:"active_runs"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, result.Stdout)
	}
	if report.MasterDir != e.Master || report.BaseDir != e.Base || report.Project != "demo" || !report.ProjectExists {
		t.Fatalf("resolved context = master %q, basedir %q, project %q exists=%t", report.MasterDir, report.BaseDir, report.Project, report.ProjectExists)
	}
	var missingProject struct {
		Project       string `json:"project"`
		ProjectExists bool   `json:"project_exists"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("info", "--project-name", "not-created", "--json").Stdout), &missingProject); err != nil {
		t.Fatalf("decode missing-project info: %v", err)
	}
	if missingProject.Project != "not-created" || missingProject.ProjectExists {
		t.Fatalf("missing project = %+v, want not-created and false", missingProject)
	}
	if len(report.RunLocks) != 1 || report.RunLocks[0].State != "stale" || report.RunLocks[0].RunID != runID {
		t.Fatalf("run locks = %#v, want %s", report.RunLocks, runID)
	}
	if len(report.ActiveRuns) != 1 || report.ActiveRuns[0].RunID != runID || report.ActiveRuns[0].Jobs.Alive != 1 {
		t.Fatalf("active runs = %#v, want one live local job in %s", report.ActiveRuns, runID)
	}
	gotLock, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("info removed the stale lock: %v", err)
	}
	if string(gotLock) != string(lock) {
		t.Fatalf("info changed stale lock: got %s, want %s", gotLock, lock)
	}
}
