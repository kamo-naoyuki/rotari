package interfaces

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestInfoReportsContextAndRunLocks(t *testing.T) {
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
	finishedJobDir := filepath.Join(runDir, "job-done")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(finishedJobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"hostname":"`+host+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "command.json"), []byte(`{"id":"job-a","executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(finishedJobDir, "command.json"), []byte(`{"id":"job-done","executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(finishedJobDir, "status"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(finishedJobDir, state.FinalResultFileName), []byte(`{"id":"job-done","exit_code":0}`), 0o644); err != nil {
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
				Finished int `json:"finished"`
				Alive    int `json:"alive"`
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
	if len(report.ActiveRuns) != 1 || report.ActiveRuns[0].RunID != runID || report.ActiveRuns[0].Jobs.Finished != 1 || report.ActiveRuns[0].Jobs.Alive != 1 {
		t.Fatalf("active runs = %#v, want one finished and one live local job in %s", report.ActiveRuns, runID)
	}
}

func TestInfoCountsFailedJobsOfActiveRuns(t *testing.T) {
	covers(t, "CLI-21")
	e := support.NewEnv(t)
	project := "info-failed"
	for _, code := range []string{"0", "3", "4"} {
		e.MustRotari("add", "-p", project, "--", "sh", "-c", "exit "+code)
	}
	e.StartRun(project, 1, true)
	var counts struct {
		Finished int `json:"finished"`
		Failed   int `json:"failed"`
	}
	support.WaitUntil(t, 10*time.Second, func() (bool, string) {
		out := e.MustRotari("info", "-p", project, "--json").Stdout
		var report struct {
			ActiveRuns []struct {
				Jobs json.RawMessage `json:"jobs"`
			} `json:"active_runs"`
		}
		if json.Unmarshal([]byte(out), &report) != nil || len(report.ActiveRuns) != 1 || json.Unmarshal(report.ActiveRuns[0].Jobs, &counts) != nil {
			return false, out
		}
		return counts.Finished == 3, out
	})
	if counts.Failed != 2 {
		t.Fatalf("info counts = %+v, want two of three finished jobs failed", counts)
	}
	if text := e.MustRotari("info", "-p", project).Stdout; !strings.Contains(text, "jobs=finished:3 failed:2 ") {
		t.Fatalf("info text lacks the failed count:\n%s", text)
	}
}

func TestInfoCoordinatorIdentityWithoutRunLock(t *testing.T) {
	covers(t, "CLI-21")
	e := support.NewEnv(t)
	project := "coordinator"
	runID, _ := e.FinishedJobRun(project)
	projectDir := filepath.Join(e.Base, "projects", project)
	contextPath := filepath.Join(projectDir, "runs", runID, "context.json")
	var data []byte
	var context struct {
		Coordinator struct {
			Host                 string `json:"host"`
			PID                  int    `json:"pid"`
			PIDNamespace         string `json:"pid_namespace"`
			ProcessStart         string `json:"process_start"`
			RecordedAt           string `json:"recorded_at"`
			ManagementFinishedAt string `json:"management_finished_at"`
		} `json:"coordinator"`
	}
	support.WaitUntil(t, 10*time.Second, func() (bool, string) {
		var err error
		data, err = os.ReadFile(contextPath)
		if err != nil {
			return false, err.Error()
		}
		if err := json.Unmarshal(data, &context); err != nil {
			return false, err.Error()
		}
		identity := context.Coordinator
		return identity.ManagementFinishedAt != "" && syscall.Kill(identity.PID, 0) == syscall.ESRCH, string(data)
	})
	identity := context.Coordinator
	if identity.Host == "" || identity.PID <= 0 || identity.RecordedAt == "" || identity.ManagementFinishedAt == "" {
		t.Fatalf("new run did not preserve completed coordinator identity: %s", data)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "running.lock")); !os.IsNotExist(err) {
		t.Fatalf("completed run still has a lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "meta.json"), []byte(`{"phase":"running","last_run_id":"`+runID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var report struct {
		ActiveRuns []struct {
			Phase       string `json:"phase"`
			Coordinator struct {
				State      string `json:"state"`
				Reason     string `json:"reason"`
				ObservedAt string `json:"observed_at"`
			} `json:"coordinator"`
		} `json:"active_runs"`
	}
	out := e.MustRotari("info", "--json").Stdout
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.ActiveRuns) != 1 || report.ActiveRuns[0].Phase != "interrupted" {
		t.Fatalf("missing interrupted run: %s", out)
	}
	assessment := report.ActiveRuns[0].Coordinator
	want := "stopped"
	if identity.PIDNamespace == "" {
		want = "unknown"
	}
	if assessment.State != want || assessment.Reason == "" || assessment.ObservedAt == "" {
		t.Fatalf("coordinator after lock removal: %s", out)
	}
	after, err := os.ReadFile(contextPath)
	if err != nil || string(after) != string(data) {
		t.Fatalf("info changed coordinator context: %v", err)
	}
	text := e.MustRotari("info").Stdout
	if !strings.Contains(text, "coordinator="+want) || !strings.Contains(text, "reason=") {
		t.Fatalf("text lacks coordinator evidence: %s", text)
	}
}
