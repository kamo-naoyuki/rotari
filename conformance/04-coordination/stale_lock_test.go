package coordination

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

const staleLockRunID = "20261015-120000-00000000"

// writeStaleLockProject leaves project demo interrupted by a coordinator that
// died on this host: metadata names the run and running.lock names lockRunID
// with a PID that is not alive.
func writeStaleLockProject(t *testing.T, e *support.Env, lockRunID string) string {
	t.Helper()
	e.MustRotari("add", "-p", "demo", "--", "true")
	projectDir := filepath.Join(e.Base, "projects", "demo")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(projectDir, "runs", staleLockRunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(runDir, "context.json"):     `{"hostname":"` + host + `"}`,
		filepath.Join(runDir, "commands.json"):    `{"commands":[{"id":"job-a","command":["true"]}]}`,
		filepath.Join(projectDir, "meta.json"):    `{"phase":"running","last_run_id":"` + staleLockRunID + `"}`,
		filepath.Join(projectDir, "running.lock"): `{"pid":999999999,"run_id":"` + lockRunID + `","host":"` + host + `"}`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(projectDir, "running.lock")
}

// lockReaders read a project's run lock through the binary and the Web API.
var lockReaders = map[string]func(e *support.Env){
	"show":     func(e *support.Env) { e.Rotari("show", "-p", "demo") },
	"show run": func(e *support.Env) { e.Rotari("show", "-p", "demo", "-r", staleLockRunID) },
	"jobs":     func(e *support.Env) { e.Rotari("jobs") },
	"runs":     func(e *support.Env) { e.Rotari("runs") },
	"projects": func(e *support.Env) { e.Rotari("projects") },
	"info":     func(e *support.Env) { e.Rotari("info", "-p", "demo") },
	"check":    func(e *support.Env) { e.Rotari("check", "demo") },
	"lineage":  func(e *support.Env) { e.Rotari("lineage", "-p", "demo") },
	"wait":     func(e *support.Env) { e.Rotari("wait", "-p", "demo", "--timeout", "1s") },
	"web api": func(e *support.Env) {
		url := e.StartWeb()
		e.HTTPGet(url + "/api/state")
	},
}

func TestCommandsRemoveStaleRunLock(t *testing.T) {
	covers(t, "SAFE-11")
	for name, read := range lockReaders {
		t.Run(name, func(t *testing.T) {
			t.Run("consistent", func(t *testing.T) {
				e := support.NewEnv(t)
				lockPath := writeStaleLockProject(t, e, staleLockRunID)
				read(e)
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("%s left the stale lock: %v", name, err)
				}
				if state := e.CheckState("demo"); state != "interrupted" {
					t.Fatalf("after %s: state %q, want the run still interrupted", name, state)
				}
			})
			t.Run("lock names another run", func(t *testing.T) {
				e := support.NewEnv(t)
				lockPath := writeStaleLockProject(t, e, "20261015-110000-11111111")
				read(e)
				data, err := os.ReadFile(lockPath)
				if err != nil || !strings.Contains(string(data), "20261015-110000-11111111") {
					t.Fatalf("%s changed a lock that fails the consistency checks: %q, %v", name, data, err)
				}
			})
		})
	}
}

// A lock written in another PID namespace on this host, such as by a run
// started outside a sandbox that cannot see its coordinator, is not stale.
func TestLockFromAnotherPIDNamespaceIsLocked(t *testing.T) {
	covers(t, "COORD-2", "COORD-3", "SAFE-11")
	writeLock := func(t *testing.T, e *support.Env) string {
		lockPath := writeStaleLockProject(t, e, staleLockRunID)
		setJSONField(t, lockPath, "pid_namespace", "pid:[1]")
		return lockPath
	}
	for name, read := range lockReaders {
		t.Run(name, func(t *testing.T) {
			e := support.NewEnv(t)
			lockPath := writeLock(t, e)
			read(e)
			if _, err := os.Stat(lockPath); err != nil {
				t.Fatalf("%s removed a lock from another PID namespace: %v", name, err)
			}
			if state := e.CheckState("demo"); state != "locked" {
				t.Fatalf("after %s: state %q, want locked", name, state)
			}
		})
	}
	e := support.NewEnv(t)
	writeLock(t, e)
	if r := e.Rotari("cancel", "-p", "demo"); r.Code == 0 || !strings.Contains(r.Stderr, "PID namespace pid:[1]") {
		t.Errorf("whole cancel accepted: %s", r)
	}
	if r := e.Rotari("run", "-p", "demo"); r.Code == 0 || !strings.Contains(r.Stderr, "is running") {
		t.Errorf("run accepted: %s", r)
	}
	if result := e.Rotari("unlock", "demo"); result.Code == 0 || !strings.Contains(result.Stderr, "coordinator_unknown") {
		t.Fatalf("unverified ownership did not need attestation: %s", result)
	}
	e.MustRotari("unlock", "demo", "--assume-stopped")
	if state := e.CheckState("demo"); state != "ready" {
		t.Errorf("after unlock state %q, want ready", state)
	}
}
