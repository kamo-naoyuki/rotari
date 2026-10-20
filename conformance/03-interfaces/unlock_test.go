package interfaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type unlockAssessment struct {
	Allowed          bool   `json:"allowed"`
	Applied          bool   `json:"applied"`
	RefusalCode      string `json:"refusal_code"`
	ExecutionChecked bool   `json:"execution_checked"`
	Revision         string `json:"revision"`
	Coordinator      struct {
		State    string         `json:"state"`
		Reason   string         `json:"reason"`
		Identity map[string]any `json:"identity"`
	} `json:"coordinator"`
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
						filepath.Join(runDir, "commands.json"): map[string]any{"commands": []any{}},
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
					if preview.Allowed != allowed || preview.Applied || preview.ExecutionChecked || preview.Coordinator.State != test.want || preview.Revision == "" {
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
					if failed == allowed || applied.Allowed != allowed || applied.Applied != allowed || applied.ExecutionChecked {
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
