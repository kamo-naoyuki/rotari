package interfaces

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type unlockAssessment struct {
	Allowed          bool   `json:"allowed"`
	Applied          bool   `json:"applied"`
	RefusalCode      string `json:"refusal_code"`
	ExecutionChecked bool   `json:"execution_checked"`
	Revision         string `json:"revision"`
	Action           string `json:"action"`
	Repair           struct {
		Complete   bool             `json:"complete"`
		Reason     string           `json:"reason"`
		Executions []map[string]any `json:"executions"`
	} `json:"repair"`
	Coordinator struct {
		State    string         `json:"state"`
		Reason   string         `json:"reason"`
		Identity map[string]any `json:"identity"`
	} `json:"coordinator"`
}

func TestUnlockExecutionEvidenceOverridesTerminalOutcome(t *testing.T) {
	covers(t, "SAFE-16", "SAFE-14", "CLI-21")
	for _, transport := range []string{"CLI", "MCP"} {
		for _, incomplete := range []bool{false, true} {
			name := transport + "/complete"
			if incomplete {
				name = transport + "/incomplete"
			}
			t.Run(name, func(t *testing.T) {
				e := support.NewEnv(t)
				e.MustRotari("add", "-p", "live-execution", "--", "true")
				e.MustRotari("run", "-p", "live-execution", "--quiet")
				projectDir := filepath.Join(e.Base, "projects", "live-execution")
				metaPath := filepath.Join(projectDir, "meta.json")
				meta := readUnlockJSON(t, metaPath)
				runDir := filepath.Join(projectDir, "runs", meta["last_run_id"].(string))
				summary := readUnlockJSON(t, filepath.Join(runDir, "summary.json"))
				result := summary["results"].([]any)[0].(map[string]any)
				if incomplete {
					summary["results"] = []any{}
					writeUnlockJSON(t, filepath.Join(runDir, "summary.json"), summary)
				}
				attemptDir := filepath.Join(runDir, result["id"].(string), "attempts", result["attempt_id"].(string))
				if incomplete {
					for _, name := range []string{"status", "status.json"} {
						if err := os.Remove(filepath.Join(attemptDir, name)); err != nil && !os.IsNotExist(err) {
							t.Fatal(err)
						}
					}
				}
				process := exec.Command("sleep", "30")
				process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				if err := process.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = process.Process.Kill(); _ = process.Wait() }()
				execution := support.ProcessIdentity(t, process.Process.Pid)
				writeUnlockJSON(t, filepath.Join(attemptDir, "job.json"), map[string]any{
					"executor": "local", "host": execution.Host, "pid": execution.PID, "pid_namespace": execution.PIDNamespace, "process_start": execution.ProcessStart,
				})
				contextPath := filepath.Join(runDir, "context.json")
				context := readUnlockJSON(t, contextPath)
				coordinator := support.CurrentCoordinator(t)
				coordinator.PID = 1 << 30
				context["coordinator"] = coordinator
				writeUnlockJSON(t, contextPath, context)
				meta["phase"] = "running"
				writeUnlockJSON(t, metaPath, meta)
				before := support.SavePairTree(t, projectDir)
				var info struct {
					Runs []struct {
						Executions struct {
							Active int              `json:"active"`
							Units  []map[string]any `json:"executions"`
						} `json:"execution_assessment"`
					} `json:"active_runs"`
				}
				if err := json.Unmarshal([]byte(e.MustRotari("info", "-p", "live-execution", "--json").Stdout), &info); err != nil {
					t.Fatal(err)
				}
				if len(info.Runs) != 1 || info.Runs[0].Executions.Active != 1 || len(info.Runs[0].Executions.Units) != 1 {
					t.Fatalf("info lost execution assessment: %+v", info)
				}
				conflict := info.Runs[0].Executions.Units[0]["conflict"]
				if (!incomplete && conflict != true) || (incomplete && conflict == true) {
					t.Fatalf("info lost terminal/live conflict: %+v", info)
				}
				var session *mcpSession
				target := map[string]any{"project": "live-execution"}
				if transport == "MCP" {
					session = startMCP(t, e)
					target["basedir_ref"] = baseDirRef(t, session, "live-execution")
				}
				for _, attest := range []bool{false, true} {
					var preview unlockAssessment
					if transport == "CLI" {
						args := []string{"unlock", "-p", "live-execution", "--dry-run", "--json"}
						if attest {
							args = append(args, "--assume-stopped")
						}
						if err := json.Unmarshal([]byte(e.MustRotari(args...).Stdout), &preview); err != nil {
							t.Fatal(err)
						}
						args = []string{"unlock", "-p", "live-execution", "--json"}
						if attest {
							args = append(args, "--assume-stopped")
						}
						applied := e.Rotari(args...)
						if applied.Code != 1 || !strings.Contains(applied.Stderr, "execution_active") {
							t.Fatalf("live apply=%s", applied)
						}
					} else {
						delete(target, "if_revision")
						target["assume_stopped"] = attest
						if message := session.call("rotari_preview_unlock", target, &preview); message != "" {
							t.Fatal(message)
						}
						target["if_revision"] = preview.Revision
						var refused struct {
							IsError bool             `json:"isError"`
							Output  unlockAssessment `json:"structuredContent"`
						}
						if err := json.Unmarshal(session.request("tools/call", map[string]any{"name": "rotari_unlock", "arguments": target}), &refused); err != nil {
							t.Fatal(err)
						}
						if !refused.IsError || refused.Output.RefusalCode != "execution_active" {
							t.Fatalf("live MCP apply=%+v", refused)
						}
					}
					if len(preview.Repair.Executions) != 1 || preview.Allowed || preview.RefusalCode != "execution_active" || !preview.ExecutionChecked {
						t.Fatalf("live preview=%+v", preview)
					}
					conflict := preview.Repair.Executions[0]["conflict"]
					if (!incomplete && conflict != true) || (incomplete && conflict == true) {
						t.Fatalf("unexpected terminal conflict: %+v", preview)
					}
					if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, projectDir).Raw()) {
						t.Fatal("live refusal mutated state")
					}
				}
			})
		}
	}
}

func TestUnlockFinalizationRepair(t *testing.T) {
	covers(t, "SAFE-15", "SAFE-16", "SAFE-4", "SAFE-7", "RES-7")
	for _, variant := range []string{"plain", "legacy", "array", "carried-array", "matrix", "incomplete", "incomplete-legacy", "duplicate"} {
		for _, transport := range []string{"CLI", "MCP"} {
			t.Run(variant+"/"+transport, func(t *testing.T) {
				e := support.NewEnv(t)
				args := []string{"add", "-p", "repair"}
				switch variant {
				case "array", "carried-array":
					args = append(args, "--array", "1-3")
				case "matrix":
					args = append(args, "--matrix", "SEED=1,2")
				}
				e.MustRotari(append(args, "--", "true")...)
				e.MustRotari("run", "-p", "repair", "--quiet")
				projectDir := filepath.Join(e.Base, "projects", "repair")
				metaPath := filepath.Join(projectDir, "meta.json")
				meta := readUnlockJSON(t, metaPath)
				runID := meta["last_run_id"].(string)
				runDir := filepath.Join(projectDir, "runs", runID)
				summaryPath := filepath.Join(runDir, "summary.json")
				summary := readUnlockJSON(t, summaryPath)
				if variant == "legacy" {
					if err := filepath.WalkDir(runDir, func(path string, entry os.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if !entry.IsDir() && entry.Name() == "job.json" {
							return os.Remove(path)
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "carried-array" {
					sourceID := runID
					commands := readUnlockJSON(t, filepath.Join(runDir, "commands.json"))
					context := readUnlockJSON(t, filepath.Join(runDir, "context.json"))
					runID = "20261020-130000-00000000"
					runDir = filepath.Join(projectDir, "runs", runID)
					if err := os.MkdirAll(runDir, 0o755); err != nil {
						t.Fatal(err)
					}
					for _, value := range commands["commands"].([]any) {
						command := value.(map[string]any)
						command["origin"] = map[string]any{"run_id": sourceID, "job_id": command["id"]}
						delete(command, "task_origins")
					}
					summary["run_id"] = runID
					summaryPath = filepath.Join(runDir, "summary.json")
					writeUnlockJSON(t, summaryPath, summary)
					writeUnlockJSON(t, filepath.Join(runDir, "commands.json"), commands)
					writeUnlockJSON(t, filepath.Join(runDir, "context.json"), context)
					writeUnlockJSON(t, filepath.Join(runDir, "carried.json"), map[string]any{"run_id": runID, "results": summary["results"]})
					meta["last_run_id"] = runID
				}
				if variant == "incomplete" || variant == "incomplete-legacy" {
					summary["results"] = []any{}
					writeUnlockJSON(t, summaryPath, summary)
				}
				if variant == "incomplete-legacy" {
					if err := filepath.WalkDir(runDir, func(path string, entry os.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if !entry.IsDir() && entry.Name() == "job.json" {
							return os.Remove(path)
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				if variant == "duplicate" {
					results := summary["results"].([]any)
					summary["results"] = append(results, results[0])
					writeUnlockJSON(t, summaryPath, summary)
				}
				meta["phase"] = "running"
				writeUnlockJSON(t, metaPath, meta)
				contextPath := filepath.Join(runDir, "context.json")
				context := readUnlockJSON(t, contextPath)
				identity := support.CurrentCoordinator(t)
				identity.PID = 1 << 30
				context["coordinator"] = identity
				writeUnlockJSON(t, contextPath, context)
				e.MustRotari("add", "-p", "repair", "--", "false")
				before := support.SavePairTree(t, projectDir)
				var session *mcpSession
				target := map[string]any{"project": "repair"}
				if transport == "MCP" {
					session = startMCP(t, e)
					target["basedir_ref"] = baseDirRef(t, session, "repair")
				}
				var preview unlockAssessment
				if transport == "CLI" {
					result := e.MustRotari("unlock", "-p", "repair", "--dry-run", "--json")
					if err := json.Unmarshal([]byte(result.Stdout), &preview); err != nil {
						t.Fatal(err)
					}
				} else if message := session.call("rotari_preview_unlock", target, &preview); message != "" {
					t.Fatal(message)
				}
				complete := variant != "incomplete" && variant != "incomplete-legacy" && variant != "duplicate"
				unknown := variant == "legacy" || variant == "incomplete-legacy"
				if preview.Repair.Complete != complete || preview.ExecutionChecked != !unknown || preview.Applied || preview.Allowed == unknown {
					t.Fatalf("preview=%+v", preview)
				}
				if complete && (preview.Action != "finalize" || len(preview.Repair.Executions) == 0) {
					t.Fatalf("missing execution assessment: %+v", preview)
				}
				if complete && !unknown {
					for _, execution := range preview.Repair.Executions {
						if execution["state"] != "stopped" || execution["source"] != "local_process_group" || execution["observed_at"] == "" {
							t.Fatalf("unverified termination: %v", execution)
						}
					}
				}
				if !complete && (preview.Action != "release" || preview.Repair.Reason == "") {
					t.Fatalf("unexplained release: %+v", preview)
				}
				if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, projectDir).Raw()) {
					t.Fatal("preview changed state")
				}
				if unknown {
					if transport == "CLI" {
						refused := e.Rotari("unlock", "-p", "repair", "--json")
						if refused.Code != 1 || !strings.Contains(refused.Stderr, "execution_unknown") {
							t.Fatalf("refusal=%s", refused)
						}
					} else {
						target["if_revision"] = preview.Revision
						var refused struct {
							IsError bool             `json:"isError"`
							Output  unlockAssessment `json:"structuredContent"`
						}
						if err := json.Unmarshal(session.request("tools/call", map[string]any{"name": "rotari_unlock", "arguments": target}), &refused); err != nil {
							t.Fatal(err)
						}
						if !refused.IsError || refused.Output.RefusalCode != "execution_unknown" {
							t.Fatalf("refusal=%+v", refused)
						}
					}
					if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, projectDir).Raw()) {
						t.Fatal("refusal changed state")
					}
				}
				var applied unlockAssessment
				if transport == "CLI" {
					args := []string{"unlock", "-p", "repair", "--json"}
					if unknown {
						args = append(args, "--assume-stopped")
					}
					result := e.MustRotari(args...)
					if err := json.Unmarshal([]byte(result.Stdout), &applied); err != nil {
						t.Fatal(err)
					}
				} else {
					target["assume_stopped"], target["if_revision"] = unknown, preview.Revision
					if message := session.call("rotari_unlock", target, &applied); message != "" {
						t.Fatal(message)
					}
				}
				meta = readUnlockJSON(t, metaPath)
				wantPhase := "collecting"
				if complete {
					wantPhase = "finished"
				}
				if !applied.Applied || meta["phase"] != wantPhase || meta["last_run_id"] != runID {
					t.Fatalf("applied=%+v meta=%v", applied, meta)
				}
				if complete {
					var completion struct {
						ExitCode int `json:"last_run_exit_code"`
					}
					data, err := os.ReadFile(metaPath)
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(data, &completion); err != nil {
						t.Fatal(err)
					}
					if float64(completion.ExitCode) != summary["exit_code"] || meta["updated_at"] != summary["finished_at"] {
						t.Fatalf("recorded completion not preserved: %v", meta)
					}
				}
				for _, name := range []string{"summary.json", "commands.json", "carried.json"} {
					beforePath := filepath.Join("runs", runID, name)
					after := support.SavePairTree(t, projectDir)
					if before.Observation(t)[beforePath] != after.Observation(t)[beforePath] {
						t.Fatalf("rewrote recorded %s", name)
					}
				}
				if got := e.MustRotari("check", "repair"); !strings.Contains(got.Stdout, "queued=1") {
					t.Fatalf("consumed queue: %s", got)
				}
				context = readUnlockJSON(t, contextPath)
				recovery := context["recovery"].(map[string]any)
				if recovery["action"] != applied.Action || recovery["assume_stopped"] != unknown {
					t.Fatalf("recovery evidence=%v", recovery)
				}
				e.MustRotari("unlock", "-p", "repair", "--assume-stopped")
			})
		}
	}
}

func readUnlockJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func writeUnlockJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnlockOwnershipAndDryRun(t *testing.T) {
	covers(t, "SAFE-4", "SAFE-13", "SAFE-14", "COORD-3", "SAFE-7")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	identity := support.CurrentCoordinator(t)
	for _, test := range []struct {
		name, host, namespace, start, want string
		pid                                int
	}{
		{"legacy", "", "", "", "unknown", 0},
		{"other-host", host + "-remote", identity.PIDNamespace, "1", "unknown", os.Getpid()},
		{"other-namespace", host, identity.PIDNamespace + "-remote", "1", "unknown", os.Getpid()},
		{"unverified-PID", host, identity.PIDNamespace, "", "unknown", os.Getpid()},
		{"live", host, identity.PIDNamespace, identity.ProcessStart, "live", os.Getpid()},
		{"stopped", host, identity.PIDNamespace, "1", "stopped", 1 << 30},
	} {
		for _, transport := range []string{"CLI", "MCP"} {
			for _, attest := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/attest=%t", test.name, transport, attest), func(t *testing.T) {
					e := support.NewEnv(t)
					e.MustRotari("add", "-p", "demo", "--", "true")
					projectDir := filepath.Join(e.Base, "projects", "demo")
					runDir := filepath.Join(projectDir, "runs", "20261020-120000-00000000")
					if err := os.MkdirAll(runDir, 0o755); err != nil {
						t.Fatal(err)
					}
					context := map[string]any{"hostname": host}
					if test.pid != 0 {
						context["coordinator"] = map[string]any{"host": test.host, "pid": test.pid, "pid_namespace": test.namespace, "process_start": test.start}
					}
					for path, value := range map[string]any{
						filepath.Join(runDir, "context.json"):  context,
						filepath.Join(runDir, "commands.json"): map[string]any{"commands": []any{map[string]any{"id": "training", "command": []string{"true"}, "executor": "local"}}},
						filepath.Join(projectDir, "meta.json"): map[string]any{"phase": "running", "last_run_id": filepath.Base(runDir)},
					} {
						data, err := json.Marshal(value)
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, data, 0o644); err != nil {
							t.Fatal(err)
						}
					}
					jobDir := filepath.Join(runDir, "training")
					if err := os.MkdirAll(jobDir, 0o755); err != nil {
						t.Fatal(err)
					}
					writeUnlockJSON(t, filepath.Join(jobDir, "command.json"), map[string]any{"id": "training", "command": []string{"true"}, "executor": "local"})
					writeUnlockJSON(t, filepath.Join(jobDir, "job.json"), map[string]any{
						"executor": "local", "host": host, "pid": 1 << 30,
						"pid_namespace": identity.PIDNamespace, "process_start": "1",
					})
					var session *mcpSession
					target := map[string]any{"project": "demo", "assume_stopped": attest}
					if transport == "MCP" {
						session = startMCP(t, e)
						target["basedir_ref"] = baseDirRef(t, session, "demo")
					}
					before := support.SavePairTree(t, projectDir)
					var preview, applied unlockAssessment
					args := []string{"unlock", "-p", "demo", "--json"}
					if attest {
						args = append(args, "--assume-stopped")
					}
					if transport == "CLI" {
						result := e.MustRotari(append(args, "--dry-run")...)
						if err := json.Unmarshal([]byte(result.Stdout), &preview); err != nil {
							t.Fatal(err)
						}
					} else if message := session.call("rotari_preview_unlock", target, &preview); message != "" {
						t.Fatal(message)
					}
					allowed := test.want == "stopped" || (test.want == "unknown" && attest)
					if preview.Allowed != allowed || preview.Applied || !preview.ExecutionChecked || preview.Coordinator.State != test.want || preview.Revision == "" {
						t.Fatalf("preview = %+v, want allowed=%t state=%s", preview, allowed, test.want)
					}
					if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, projectDir).Raw()) {
						t.Fatal("preview mutated project")
					}
					var failed bool
					var message string
					if transport == "CLI" {
						result := e.Rotari(args...)
						failed, message = result.Code != 0, result.Stderr
						if err := json.Unmarshal([]byte(result.Stdout), &applied); err != nil {
							t.Fatalf("invalid JSON: %s: %v", result, err)
						}
					} else {
						target["if_revision"] = preview.Revision
						var result struct {
							IsError bool             `json:"isError"`
							Output  unlockAssessment `json:"structuredContent"`
							Content []struct {
								Text string `json:"text"`
							} `json:"content"`
						}
						if err := json.Unmarshal(session.request("tools/call", map[string]any{"name": "rotari_unlock", "arguments": target}), &result); err != nil {
							t.Fatal(err)
						}
						failed, applied = result.IsError, result.Output
						for _, content := range result.Content {
							message += content.Text
						}
					}
					if failed == allowed || applied.Allowed != allowed || applied.Applied != allowed || !applied.ExecutionChecked {
						t.Fatalf("apply = %+v failed=%t message=%s", applied, failed, message)
					}
					if !allowed {
						if applied.RefusalCode != "coordinator_"+test.want || !strings.Contains(message, applied.RefusalCode) {
							t.Fatalf("missing machine-readable refusal: %+v, %s", applied, message)
						}
						if test.want == "unknown" && !strings.Contains(message, "--assume-stopped") {
							t.Fatalf("no explicit recovery guidance: %s", message)
						}
						if test.pid != 0 && (!strings.Contains(message, "host="+test.host) || !strings.Contains(message, "namespace="+test.namespace)) {
							t.Fatalf("missing inspection location: %s", message)
						}
						if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, projectDir).Raw()) {
							t.Fatal("refusal mutated project")
						}
						return
					}
					data, err := os.ReadFile(filepath.Join(runDir, "context.json"))
					if err != nil {
						t.Fatal(err)
					}
					var saved struct {
						Recovery struct {
							AssumeStopped bool   `json:"assume_stopped"`
							State         string `json:"coordinator_state"`
						} `json:"recovery"`
					}
					if err := json.Unmarshal(data, &saved); err != nil {
						t.Fatal(err)
					}
					if saved.Recovery.AssumeStopped != attest || saved.Recovery.State != test.want {
						t.Fatalf("wrong attestation evidence: %s", data)
					}
					if after := e.MustRotari("check", "demo"); !strings.Contains(after.Stdout, "queued=1") {
						t.Fatalf("queue was not preserved: %s", after)
					}
				})
			}
		}
	}
}
