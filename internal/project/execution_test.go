package project

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestRepairExecutionEvidencePolicy(t *testing.T) {
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	start := state.ProcessStart(process.Process.Pid)
	if start == "" || state.PIDNamespace() == "" {
		t.Skip("local process identity unavailable")
	}
	for _, test := range []struct {
		name, start, state string
		allowed            bool
		summaryOnly        bool
	}{
		{"verified live", start, "active", false, false},
		{"summary-only live", start, "active", false, true},
		{"legacy live", "", "unknown", true, false},
		{"reused group", "other", "unknown", true, false},
	} {
		for _, attest := range []bool{false, true} {
			t.Run(test.name+map[bool]string{false: "/normal", true: "/attested"}[attest], func(t *testing.T) {
				paths := repairFixture(t)
				stoppedRepairCoordinator(t, paths)
				dir := filepath.Join(paths.RunsDir, repairRunID, "training")
				if test.summaryOnly {
					if err := os.Remove(filepath.Join(dir, "status")); err != nil {
						t.Fatal(err)
					}
				}
				repairWrite(t, filepath.Join(dir, "job.json"), map[string]any{
					"executor": "local", "host": host, "pid": process.Process.Pid, "pid_namespace": state.PIDNamespace(), "process_start": test.start,
				})
				preview, err := Unlock(paths, "", Guard{DryRun: true}, attest)
				if err != nil {
					t.Fatal(err)
				}
				wantAllowed := attest && test.allowed
				if preview.Allowed != wantAllowed || preview.Repair.Executions[0].State != test.state || !preview.JobsMayBeRunning {
					t.Fatalf("assessment=%+v", preview)
				}
				if test.state == "active" && (!preview.Repair.Executions[0].Conflict || preview.RefusalCode != "execution_active" || !preview.ExecutionChecked) {
					t.Fatalf("terminal outcome overrode positive activity: %+v", preview)
				}
				shared, err := AssessExecutions(paths, repairRunID)
				if err != nil || len(shared.Executions) != 1 || shared.Executions[0].State != test.state || shared.Executions[0].Reason != preview.Repair.Executions[0].Reason {
					t.Fatalf("info/repair drift: %+v %v", shared, err)
				}
				result, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, attest)
				if wantAllowed {
					if err != nil || !result.Applied {
						t.Fatalf("attested apply=%+v %v", result, err)
					}
				} else {
					var refused *UnlockRefusedError
					if !errors.As(err, &refused) || result.Applied {
						t.Fatalf("unsafe apply=%+v %v", result, err)
					}
					context, err := state.LoadContext(state.NewStore(0o755, 0o644), filepath.Dir(dir))
					if err != nil || context.Recovery != nil {
						t.Fatalf("refusal mutated context: %+v %v", context, err)
					}
				}
			})
		}
	}
}

func TestExecutionInventoryIncludesPartialAndEarlierAttempts(t *testing.T) {
	paths := repairFixture(t)
	runDir := filepath.Join(paths.RunsDir, repairRunID)
	removeLegacyRepairJob(t, runDir)
	jobDir := filepath.Join(runDir, "training")
	for number := range 3 {
		id := state.MakeAttemptID(repairRunID, "training", number)
		dir := filepath.Join(jobDir, "attempts", id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		if number == 1 {
			continue
		} // Accepted/launched work with incomplete publication.
		repairWrite(t, filepath.Join(dir, "command.json"), map[string]any{
			"id": "training", "command": []string{"true"}, "executor": "local", "attempt_id": id,
		})
	}
	// There is deliberately no summary: an ongoing run still needs the same
	// complete attempt census, including the directory without a command.
	if err := os.Remove(filepath.Join(runDir, "summary.json")); err != nil {
		t.Fatal(err)
	}
	assessment, err := AssessExecutions(paths, repairRunID)
	if err != nil || len(assessment.Executions) != 3 || assessment.Unknown != 3 {
		t.Fatalf("inventory=%+v %v", assessment, err)
	}
}

func TestRepairVerifiedStoppedExecutionNeedsNoAttestation(t *testing.T) {
	paths := repairFixture(t)
	stoppedRepairCoordinator(t, paths)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	repairWrite(t, filepath.Join(paths.RunsDir, repairRunID, "training", "job.json"), map[string]any{
		"executor": "local", "host": host, "pid": 1 << 30,
		"pid_namespace": state.PIDNamespace(), "process_start": "1",
	})
	preview, err := Unlock(paths, "", Guard{DryRun: true}, false)
	if err != nil || !preview.Allowed || preview.Action != "finalize" {
		t.Fatalf("verified stopped execution still requires attestation: %+v %v", preview, err)
	}
	if len(preview.Repair.Executions) != 1 || preview.Repair.Executions[0].State != "stopped" {
		t.Fatalf("missing termination evidence: %+v", preview.Repair)
	}
	_, err = Unlock(paths, "", Guard{}, false)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepairChecksLiveEarlierAttempt(t *testing.T) {
	paths := repairFixture(t)
	stoppedRepairCoordinator(t, paths)
	runDir := filepath.Join(paths.RunsDir, repairRunID)
	removeLegacyRepairJob(t, runDir)
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for number := range 2 {
		id := state.MakeAttemptID(repairRunID, "training", number)
		dir := filepath.Join(runDir, "training", "attempts", id)
		repairWrite(t, filepath.Join(dir, "command.json"), model.JobSpec{ID: "training", AttemptID: id, Executor: "local", Command: []string{"true"}})
		pid := process.Process.Pid
		start := state.ProcessStart(pid)
		if number == 1 {
			pid = 1 << 30
			start = "1"
		}
		repairWrite(t, filepath.Join(dir, "job.json"), map[string]any{
			"executor": "local", "host": host, "pid": pid, "pid_namespace": state.PIDNamespace(), "process_start": start,
		})
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte("0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	summary := repairSummary(t, paths)
	summary.Results[0].AttemptID = state.MakeAttemptID(repairRunID, "training", 1)
	repairWrite(t, filepath.Join(runDir, "summary.json"), summary)
	preview, err := Unlock(paths, "", Guard{DryRun: true}, true)
	if err != nil || !preview.Repair.Complete || preview.Allowed || preview.RefusalCode != "execution_active" || len(preview.Repair.Executions) != 2 {
		t.Fatalf("latest stopped attempt hid older execution: %+v %v", preview, err)
	}
	shared, err := AssessExecutions(paths, repairRunID)
	if err != nil || shared.Active != 1 || shared.Stopped != 1 {
		t.Fatalf("shared inventory=%+v %v", shared, err)
	}
}

func TestExecutionAssessmentReportsMalformedOutcomeInventory(t *testing.T) {
	for _, name := range []string{"summary.json", "carried.json"} {
		t.Run(name, func(t *testing.T) {
			paths := repairFixture(t)
			if err := os.WriteFile(filepath.Join(paths.RunsDir, repairRunID, name), []byte("{bad"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := AssessExecutions(paths, repairRunID); err == nil {
				t.Fatal("silently omitted unreadable outcome inventory")
			}
		})
	}
}
