package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const repairRunID = "20261016-020229-e87d4799"

func TestUnlockRepairsCompleteSummary(t *testing.T) {
	paths := repairFixture(t)
	runDir := filepath.Join(paths.RunsDir, repairRunID)
	summaryBefore, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Unlock(paths, "", Guard{}, true); err != nil {
		t.Fatal(err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "finished" || meta.LastRunID != repairRunID || meta.LastRunExitCode != 0 {
		t.Fatalf("recovered metadata = %+v; want finished with the summary's exit code", meta)
	}
	for path, want := range map[string][]byte{filepath.Join(runDir, "summary.json"): summaryBefore, paths.QueueFile: queueBefore} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("recovery changed %s: %v", path, err)
		}
	}
}

func repairFixture(t *testing.T) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, repairRunID)
	job := model.QueuedCommand{ID: "training", Command: []string{"true"}}
	for path, value := range map[string]any{
		paths.MetaFile:                                    model.Meta{Phase: "running", LastRunID: repairRunID},
		paths.QueueFile:                                   model.Queue{Commands: []model.QueuedCommand{{ID: "next", Command: []string{"false"}}}},
		filepath.Join(runDir, "commands.json"):            model.Queue{Commands: []model.QueuedCommand{job}},
		filepath.Join(runDir, "context.json"):             model.RunContext{},
		filepath.Join(runDir, "training", "command.json"): model.JobSpec{ID: job.ID, Command: job.Command, Executor: "local"},
		filepath.Join(runDir, "summary.json"): model.RunSummary{
			RunID: repairRunID, Status: "finished", StartedAt: "2026-10-16T02:02:29Z", FinishedAt: "2026-10-16T21:29:52Z",
			Results: []model.JobResult{{ID: job.ID, Command: job.Command}},
		},
	} {
		if err := state.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "training", "status"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return paths
}

func repairWrite(t *testing.T, path string, value any) {
	t.Helper()
	if err := state.WriteJSON(path, value); err != nil {
		t.Fatal(err)
	}
}

func repairSummary(t *testing.T, paths state.ProjectPaths) model.RunSummary {
	t.Helper()
	summary, err := state.LoadRunSummary(filepath.Join(paths.RunsDir, repairRunID, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func stoppedRepairCoordinator(t *testing.T, paths state.ProjectPaths) {
	t.Helper()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	repairWrite(t, filepath.Join(paths.RunsDir, repairRunID, "context.json"), model.RunContext{
		Coordinator: &model.RunCoordinator{Host: host, PID: 1 << 30, PIDNamespace: state.PIDNamespace(), ProcessStart: "1"},
	})
}

func removeLegacyRepairJob(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{"command.json", "status"} {
		if err := os.Remove(filepath.Join(runDir, "training", name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(runDir, "training")); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockRepairInventory(t *testing.T) {
	for _, test := range []struct {
		name       string
		change     func(*testing.T, state.ProjectPaths, *model.RunSummary)
		complete   bool
		executions int
		reason     string
	}{
		{name: "terminal legacy outcome is not termination", complete: true, executions: 1},
		{name: "summary only is not termination", complete: true, executions: 1, change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			removeLegacyRepairJob(t, filepath.Join(p.RunsDir, repairRunID))
		}},
		{name: "blocked never launched", complete: true, change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			removeLegacyRepairJob(t, filepath.Join(p.RunsDir, repairRunID))
			s.ExitCode = 1
			s.Status = "failed"
			s.Results[0].ExitCode = 1
			s.Results[0].Error = "blocked by failed dependency"
		}},
		{name: "cancelled before launch", complete: true, change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			runDir := filepath.Join(p.RunsDir, repairRunID)
			removeLegacyRepairJob(t, runDir)
			if err := os.MkdirAll(filepath.Join(runDir, "training"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runDir, "training", "cancelled"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			s.ExitCode = 1
			s.Status = model.StatusCancelled
			s.Results[0].ExitCode = 130
			s.Results[0].Error = "cancelled before start"
			s.Results[0].AttemptID = state.MakeAttemptID(repairRunID, "training", 0)
		}},
		{name: "missing result", reason: "cover every", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) { s.Results = nil }},
		{name: "duplicate result", reason: "duplicates", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			s.Results = append(s.Results, s.Results[0])
		}},
		{name: "wrong unit", reason: "missing job", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) { s.Results[0].ID = "other" }},
		{name: "wrong run", reason: "different run", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) { s.RunID = "other" }},
		{name: "wrong exit", reason: "exit code", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) { s.ExitCode = 1; s.Status = "failed" }},
		{name: "invalid finish", reason: "finish time", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) { s.FinishedAt = "" }},
		{name: "wrong command", reason: "command does not match", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			s.Results[0].Command = []string{"false"}
		}},
		{name: "foreign attempt", reason: "attempt is missing", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			s.Results[0].AttemptID = state.MakeAttemptID(repairRunID, "other", 0)
		}},
		{name: "unexpected job directory", reason: "unexpected directory", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			if err := os.MkdirAll(filepath.Join(p.RunsDir, repairRunID, "orphan"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "configs cannot hide an orphan execution", reason: "unrecognized config", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			if err := os.MkdirAll(filepath.Join(p.RunsDir, repairRunID, "configs"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "recognized config snapshots", complete: true, executions: 1, change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			runDir := filepath.Join(p.RunsDir, repairRunID)
			store := state.NewStore(state.DirectoryMode(), state.FileMode())
			context, err := state.LoadContext(store, runDir)
			if err != nil {
				t.Fatal(err)
			}
			context.ConfigSnapshotFiles = []string{"config.toml"}
			if err := state.SaveContext(store, runDir, context); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(runDir, "configs"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runDir, "configs", "config.toml"), []byte("[run]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "duplicate expanded commands", reason: "duplicates", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			repairWrite(t, filepath.Join(p.RunsDir, repairRunID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "training", Command: []string{"true"}}, {ID: "training", Command: []string{"true"}}}})
		}},
		{name: "incomplete launch and earlier attempt", complete: true, executions: 2, change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			runDir := filepath.Join(p.RunsDir, repairRunID)
			removeLegacyRepairJob(t, runDir)
			for i := 0; i < 2; i++ {
				id := state.MakeAttemptID(repairRunID, "training", i)
				dir := filepath.Join(runDir, "training", "attempts", id)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if i == 1 {
					repairWrite(t, filepath.Join(dir, "command.json"), model.JobSpec{ID: "training", AttemptID: id, Command: []string{"true"}, Executor: "local"})
					if err := os.WriteFile(filepath.Join(dir, "status"), []byte("0"), 0o644); err != nil {
						t.Fatal(err)
					}
					s.Results[0].AttemptID = id
				}
			}
		}},
		{name: "newer attempt omitted", reason: "newer attempt", change: func(t *testing.T, p state.ProjectPaths, s *model.RunSummary) {
			runDir := filepath.Join(p.RunsDir, repairRunID)
			removeLegacyRepairJob(t, runDir)
			for i := 0; i < 2; i++ {
				if err := os.MkdirAll(filepath.Join(runDir, "training", "attempts", state.MakeAttemptID(repairRunID, "training", i)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			s.Results[0].AttemptID = state.MakeAttemptID(repairRunID, "training", 0)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := repairFixture(t)
			stoppedRepairCoordinator(t, paths)
			summary := repairSummary(t, paths)
			if test.change != nil {
				test.change(t, paths, &summary)
			}
			repairWrite(t, filepath.Join(paths.RunsDir, repairRunID, "summary.json"), summary)
			attestPreview := !test.complete || test.executions == 0
			preview, err := Unlock(paths, "", Guard{DryRun: true}, attestPreview)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Repair.Complete != test.complete || (test.complete && len(preview.Repair.Executions) != test.executions) || (!test.complete && len(preview.Repair.Executions) == 0) || !strings.Contains(preview.Repair.Reason, test.reason) {
				t.Fatalf("assessment=%+v", preview)
			}
			if test.complete && test.executions > 0 {
				if preview.Allowed || preview.RefusalCode != "execution_unknown" {
					t.Fatalf("unattested repair allowed: %+v", preview)
				}
				contextBefore, _ := os.ReadFile(filepath.Join(paths.RunsDir, repairRunID, "context.json"))
				result, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, false)
				var refused *UnlockRefusedError
				if !errors.As(err, &refused) || result.Applied {
					t.Fatalf("apply=%+v, %v", result, err)
				}
				contextAfter, _ := os.ReadFile(filepath.Join(paths.RunsDir, repairRunID, "context.json"))
				if string(contextBefore) != string(contextAfter) {
					t.Fatal("refusal wrote recovery evidence")
				}
			} else if !preview.Allowed {
				t.Fatalf("unexpected refusal: %+v", preview)
			}
			attest := !test.complete || test.executions > 0
			applied, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, attest)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := state.LoadMeta(paths.MetaFile)
			if err != nil {
				t.Fatal(err)
			}
			want := "collecting"
			if test.complete {
				want = "finished"
			}
			if meta.Phase != want || !applied.Applied {
				t.Fatalf("metadata=%+v result=%+v", meta, applied)
			}
		})
	}
}

func TestUnlockRepairExpandedInventory(t *testing.T) {
	for _, variant := range []string{"array", "sparse-array", "matrix"} {
		t.Run(variant, func(t *testing.T) {
			paths := repairFixture(t)
			runDir := filepath.Join(paths.RunsDir, repairRunID)
			removeLegacyRepairJob(t, runDir)
			stoppedRepairCoordinator(t, paths)
			commands := []model.QueuedCommand{{ID: "tasks", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 3}}}
			if variant == "sparse-array" {
				commands[0].Array.Tasks = []int{1, 3}
			}
			if variant == "matrix" {
				commands = []model.QueuedCommand{{ID: "member-a", Command: []string{"true"}, Matrix: &model.MatrixSpec{GroupID: "matrix"}}, {ID: "member-b", Command: []string{"true"}, Matrix: &model.MatrixSpec{GroupID: "matrix"}}}
				for i, value := range []string{"a", "b"} {
					commands[i].Matrix.Dimensions = []model.MatrixDimension{{Name: "member", Values: []string{"a", "b"}}}
					commands[i].Matrix.Values = []model.MatrixValue{{Name: "member", Value: value}}
					commands[i].Name = model.MatrixJobName("", commands[i].Matrix.Values)
					commands[i].Environment = model.MatrixEnvironment(nil, commands[i].Matrix.Values)
				}
			}
			repairWrite(t, filepath.Join(runDir, "commands.json"), model.Queue{Commands: commands})
			summary := repairSummary(t, paths)
			summary.Results = nil
			summary.ExitCode = 1
			summary.Status = "failed"
			for i, job := range model.QueueToJobs(commands) {
				summary.Results = append(summary.Results, model.JobResult{ID: job.ID, Command: job.Command, ExitCode: i + 1, Error: "blocked by failed dependency"})
			}
			repairWrite(t, filepath.Join(runDir, "summary.json"), summary)
			preview, err := Unlock(paths, "", Guard{DryRun: true}, false)
			if err != nil || !preview.Repair.Complete || !preview.Allowed || len(preview.Repair.Executions) != 0 {
				t.Fatalf("never-launched inventory=%+v, %v", preview, err)
			}
			// Same cardinality, but one unit is replaced by a duplicate.
			summary.Results[len(summary.Results)-1] = summary.Results[0]
			repairWrite(t, filepath.Join(runDir, "summary.json"), summary)
			preview, err = Unlock(paths, "", Guard{DryRun: true}, true)
			if err != nil || preview.Repair.Complete {
				t.Fatalf("duplicate inventory=%+v, %v", preview, err)
			}
		})
	}
}

func TestUnlockRepairCleanupRetryAndReassessment(t *testing.T) {
	paths := repairFixture(t)
	runDir := filepath.Join(paths.RunsDir, repairRunID)
	stoppedRepairCoordinator(t, paths)
	preview, err := Unlock(paths, "", Guard{DryRun: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientPath := filepath.Join(runDir, state.RunClientStatusFileName)
	if err := os.WriteFile(clientPath, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, true); err == nil {
		t.Fatal("ignored failed client cleanup")
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || meta.Phase != "running" {
		t.Fatalf("premature finalization=%+v %v", meta, err)
	}
	repairWrite(t, clientPath, model.RunClientStatus{State: model.RunClientAttached, Mode: model.RunClientModeSync})
	if _, err := Unlock(paths, "", Guard{}, true); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	client, err := state.LoadRunClientStatus(store, runDir)
	if err != nil || client.State != model.RunClientCompleted || client.UpdatedAt != repairSummary(t, paths).FinishedAt {
		t.Fatalf("client=%+v %v", client, err)
	}
	before, err := state.LoadContext(store, runDir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Recovery.Action != "finalize" || !before.Recovery.AssumeStopped || len(before.Recovery.Executions) != 1 {
		t.Fatalf("evidence=%+v", before.Recovery)
	}
	if _, err := Unlock(paths, "", Guard{}, true); err != nil {
		t.Fatal(err)
	}
	after, err := state.LoadContext(store, runDir)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("repeat recovery rewrote completed evidence")
	}

	paths = repairFixture(t)
	stoppedRepairCoordinator(t, paths)
	preview, err = Unlock(paths, "", Guard{DryRun: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	summary := repairSummary(t, paths)
	summary.Results = nil
	repairWrite(t, filepath.Join(paths.RunsDir, repairRunID, "summary.json"), summary)
	result, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, true)
	if !errors.Is(err, ErrRevisionChanged) || result.Applied {
		t.Fatalf("apply accepted stale execution inventory: %+v %v", result, err)
	}
}

func TestUnlockRepairRejectsUnreadableRecords(t *testing.T) {
	for _, file := range []string{"summary.json", "commands.json", "carried.json", "training/command.json"} {
		t.Run(file, func(t *testing.T) {
			paths := repairFixture(t)
			path := filepath.Join(paths.RunsDir, repairRunID, file)
			if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := Unlock(paths, "", Guard{DryRun: true}, true); err == nil {
				t.Fatal("accepted malformed repair record")
			}
			meta, err := state.LoadMeta(paths.MetaFile)
			if err != nil || meta.Phase != "running" {
				t.Fatalf("mutated metadata: %+v %v", meta, err)
			}
		})
	}
}

func TestUnlockRepairCarriedOrigins(t *testing.T) {
	for _, variant := range []string{"plain", "array-task", "accepted", "marked-failed", "pinned-older", "unpinned", "missing-origin", "mismatched-origin", "cycle"} {
		t.Run(variant, func(t *testing.T) {
			paths := repairFixture(t)
			runDir := filepath.Join(paths.RunsDir, repairRunID)
			stoppedRepairCoordinator(t, paths)
			removeLegacyRepairJob(t, runDir)
			sourceID := "20261015-020229-00000000"
			sourceDir := filepath.Join(paths.RunsDir, sourceID)
			summary := repairSummary(t, paths)
			source := summary
			source.RunID = sourceID
			source.Results = []model.JobResult{{ID: "original", Command: []string{"true"}, ExitCode: 7}}
			source.ExitCode, source.Status = 1, "failed"
			status := []byte("7")
			if variant == "marked-failed" {
				source.Results[0].ExitCode, source.ExitCode, source.Status = 0, 0, "finished"
				status = []byte("0")
			}
			attemptID := state.MakeAttemptID(sourceID, "original", 0)
			source.Results[0].AttemptID = attemptID
			repairWrite(t, filepath.Join(sourceDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "original", Command: []string{"true"}}}})
			for i := 0; i < 2; i++ {
				id := state.MakeAttemptID(sourceID, "original", i)
				dir := filepath.Join(sourceDir, "original", "attempts", id)
				repairWrite(t, filepath.Join(dir, "command.json"), model.JobSpec{ID: "original", AttemptID: id, Command: []string{"true"}, Executor: "ssh"})
				if err := os.WriteFile(filepath.Join(dir, "status"), status, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if variant == "pinned-older" || variant == "unpinned" {
				source.Results[0].AttemptID = state.MakeAttemptID(sourceID, "original", 1)
			}
			repairWrite(t, filepath.Join(sourceDir, "summary.json"), source)
			summary.Results[0].AttemptID = attemptID
			summary.Results[0].ExitCode = 7
			summary.ExitCode, summary.Status = 1, "failed"
			origin := &model.JobOrigin{RunID: sourceID, JobID: "original", AttemptID: attemptID}
			command := model.QueuedCommand{ID: "training", Command: []string{"true"}, Origin: origin}
			switch variant {
			case "unpinned":
				origin.AttemptID = ""
				summary.Results[0].AttemptID = source.Results[0].AttemptID
			case "array-task":
				command.Origin = nil
				command.Array = &model.ArraySpec{First: 1, Last: 2}
				command.TaskOrigins = map[string]*model.JobOrigin{"training-1": origin}
				summary.Results[0].ID = "training-1"
				summary.Results = append(summary.Results, model.JobResult{ID: "training-2", Command: []string{"true"}, ExitCode: 9, Error: "blocked by failed dependency"})
			case "accepted":
				command.MarkedStatus = model.StatusSuccess
				summary.Results[0].Accepted = true
				summary.Results[0].ExitCode = 0
				summary.ExitCode, summary.Status = 0, "finished"
			case "marked-failed":
				command.MarkedStatus = model.StatusFailed
				summary.Results[0].ExitCode = 1
				summary.Results[0].Error = model.MarkedFailedError
			case "missing-origin":
				origin.RunID = "absent"
			case "mismatched-origin":
				summary.Results[0].ExitCode = 13
			case "cycle":
				origin.RunID, origin.JobID = repairRunID, "training"
			}
			repairWrite(t, filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{command}})
			repairWrite(t, filepath.Join(runDir, "summary.json"), summary)
			carried := summary.Results
			if variant == "array-task" {
				carried = carried[:1]
			}
			repairWrite(t, filepath.Join(runDir, "carried.json"), model.RunSummary{RunID: repairRunID, Results: carried})
			preview, err := Unlock(paths, "", Guard{DryRun: true}, true)
			if err != nil {
				t.Fatal(err)
			}
			valid := variant != "missing-origin" && variant != "mismatched-origin" && variant != "cycle"
			if preview.Repair.Complete != valid {
				t.Fatalf("assessment=%+v", preview)
			}
			if valid {
				if len(preview.Repair.Executions) != 2 {
					t.Fatalf("lost origin attempts: %+v", preview.Repair)
				}
				for _, execution := range preview.Repair.Executions {
					if execution.RunID != sourceID || execution.JobID != "original" || execution.Backend != "ssh" || execution.State != "unknown" {
						t.Fatalf("origin evidence=%+v", execution)
					}
				}
			}
		})
	}
}

func TestUnlockRepairTerminalRecordsDoNotProveTermination(t *testing.T) {
	for _, backend := range []string{"local", "ssh", "slurm", "sge"} {
		for _, outcome := range []struct {
			name, phase string
			exit        int
			message     string
		}{
			{"normal", "finished", 0, ""},
			{"cancel", "cancelled", 143, "cancelled"},
			{"timeout", "finished", 124, "timeout"},
		} {
			t.Run(backend+"/"+outcome.name, func(t *testing.T) {
				paths := repairFixture(t)
				stoppedRepairCoordinator(t, paths)
				dir := filepath.Join(paths.RunsDir, repairRunID, "training")
				if err := os.Remove(filepath.Join(dir, "status")); err != nil {
					t.Fatal(err)
				}
				repairWrite(t, filepath.Join(dir, "command.json"), model.JobSpec{ID: "training", Command: []string{"true"}, Executor: backend})
				summary := repairSummary(t, paths)
				summary.Results[0].ExitCode, summary.Results[0].Error = outcome.exit, outcome.message
				if outcome.exit != 0 {
					summary.ExitCode, summary.Status = 1, "failed"
				}
				repairWrite(t, filepath.Join(dir, "status.json"), map[string]any{"phase": outcome.phase, "exit_code": outcome.exit, "finished_at": summary.FinishedAt, "error": outcome.message})
				repairWrite(t, filepath.Join(paths.RunsDir, repairRunID, "summary.json"), summary)
				preview, err := Unlock(paths, "", Guard{DryRun: true}, false)
				if err != nil || !preview.Repair.Complete || preview.Allowed || preview.RefusalCode != "execution_unknown" || preview.ExecutionChecked {
					t.Fatalf("terminal wrapper treated as termination proof: %+v %v", preview, err)
				}
			})
		}
	}
}
