package attachment

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestClosePreservesFinalClientStatus(t *testing.T) {
	for _, clientState := range []string{model.RunClientAttached, model.RunClientCompleted, model.RunClientCancelling} {
		for _, summary := range []string{"", `{}`, `invalid`} {
			t.Run(fmt.Sprintf("state=%s/summary=%q", clientState, summary), func(t *testing.T) {
				checkCloseClientStatus(t, clientState, summary)
			})
		}
	}
}

func checkCloseClientStatus(t *testing.T, clientState, summary string) {
	t.Helper()
	paths, session, store := newInitiatingSession(t)
	runDir := filepath.Join(paths.RunsDir, "run-1")
	status := model.RunClientStatus{Mode: model.RunClientModeSync, State: clientState, UpdatedAt: "original"}
	if err := state.WriteRunClientStatus(store, runDir, status); err != nil {
		t.Fatal(err)
	}
	if summary != "" {
		if err := os.WriteFile(filepath.Join(runDir, "summary.json"), []byte(summary), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := session.Close(model.RunClientReasonCtrlD)
	wantErr := clientState == model.RunClientAttached && summary == "invalid"
	if (err != nil) != wantErr {
		t.Fatalf("Close error = %v, want error %v", err, wantErr)
	}
	got, err := state.LoadRunClientStatus(store, runDir)
	if err != nil {
		t.Fatal(err)
	}
	if clientState != model.RunClientAttached || summary != "" {
		if got != status {
			t.Fatalf("final status changed: got %#v, want %#v", got, status)
		}
	} else if got.State != model.RunClientDetached || got.Reason != model.RunClientReasonCtrlD {
		t.Fatalf("detached status = %#v", got)
	}
}

func TestCloseWaitsForStateLock(t *testing.T) {
	paths, session, store := newInitiatingSession(t)
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { release() }()
	dir := filepath.Join(paths.ProjectDir, directoryName)
	lock, err := os.OpenFile(filepath.Join(dir, session.id+".lock"), os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	done := make(chan error, 1)
	go func() { done <- session.Close(model.RunClientReasonCtrlD) }()
	select {
	case err := <-done:
		t.Fatalf("Close bypassed held state lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	for _, suffix := range []string{".json", ".lock"} {
		if _, err := os.Stat(filepath.Join(dir, session.id+suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("explicit session %s remains while Close waits: %v", suffix, err)
		}
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
		t.Fatalf("session lock released before Close finished: %v", err)
	}
	status := model.RunClientStatus{State: model.RunClientCompleted, Reason: model.RunClientReasonCancel, UpdatedAt: "final"}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := state.WriteRunClientStatus(store, runDir, status); err != nil {
		t.Fatal(err)
	}
	release()
	release = func() {}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish after releasing state lock")
	}
	got, err := state.LoadRunClientStatus(store, runDir)
	if err != nil || got != status {
		t.Fatalf("final status = %#v, %v; want %#v", got, err, status)
	}
}

func newInitiatingSession(t *testing.T) (state.ProjectPaths, *Session, state.Store) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	session, err := OpenPending(paths, "initiator", "cancel")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close("completed") })
	if err := Bind(paths, session.id, "run-1"); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{}); err != nil {
		t.Fatal(err)
	}
	return paths, session, state.NewStore(state.DirectoryMode(), state.FileMode())
}

func TestScanDoesNotUseRecordReadBeforeExplicitClose(t *testing.T) {
	for _, reason := range []string{model.RunClientReasonCtrlD, "completed"} {
		t.Run(reason, func(t *testing.T) {
			checkScanExplicitClose(t, reason)
		})
	}
}

func checkScanExplicitClose(t *testing.T, reason string) {
	t.Helper()
	paths, session, _ := newInitiatingSession(t)
	dir := filepath.Join(paths.ProjectDir, directoryName)
	path := filepath.Join(dir, session.id+".json")
	info, err := readRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	info.PID, info.ProcessStart = 2147483647, ""
	data := replaceRecordWithFIFO(t, path, info)
	done := make(chan []Info, 1)
	scanErr := make(chan error, 1)
	go func() {
		_, stale, err := Scan(paths, "run-1")
		done <- stale
		scanErr <- err
	}()
	writer, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := writeRecord(dir, info); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(reason); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case stale := <-done:
		if err := <-scanErr; err != nil || len(stale) != 0 {
			t.Fatalf("explicit close became stale: %#v, %v", stale, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Scan did not finish")
	}
	for _, suffix := range []string{".json", ".lock"} {
		if _, err := os.Stat(filepath.Join(dir, session.id+suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("session %s remains: %v", suffix, err)
		}
	}
}

func replaceRecordWithFIFO(t *testing.T, path string, info record) []byte {
	t.Helper()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSessionsAreIndependent(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	first, err := Open(paths, "run-1", "first", "detach")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close("")
	second, err := Open(paths, "run-1", "second", "detach")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close("")
	if attached, err := Attached(paths, "run-1"); err != nil || !attached {
		t.Fatalf("Attached with two sessions = %v, %v", attached, err)
	}
	if err := first.Close(""); err != nil {
		t.Fatal(err)
	}
	if attached, err := Attached(paths, "run-1"); err != nil || !attached {
		t.Fatalf("Attached after closing one session = %v, %v, want remaining session", attached, err)
	}
	if err := second.Close(""); err != nil {
		t.Fatal(err)
	}
	if attached, err := Attached(paths, "run-1"); err != nil || attached {
		t.Fatalf("Attached after closing both sessions = %v, %v", attached, err)
	}
}

func TestWaitSessionKeepsLegacyRunLockFallback(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", ClientAttached: true}); err != nil {
		t.Fatal(err)
	}
	session, err := Open(paths, "run-1", "waiter", "detach")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(""); err != nil {
		t.Fatal(err)
	}
	if Enabled(paths, "run-1") {
		t.Fatal("wait session enabled session-only attachment for a legacy run")
	}
	if attached, err := IsAttached(paths, "run-1"); err != nil || !attached {
		t.Fatalf("IsAttached after wait detached = %v, %v, want legacy sync client attached", attached, err)
	}
}

func TestSessionLivenessSurvivesSuspendAndDetectsSIGKILL(t *testing.T) {
	if os.Getenv("ROTARI_ATTACHMENT_HELPER") == "1" {
		paths, err := state.ResolveProjectPaths(os.Getenv("ROTARI_ATTACHMENT_BASE"), "demo")
		if err != nil {
			os.Exit(2)
		}
		session, err := Open(paths, "run-1", "suspended-child", "cancel")
		if err != nil {
			os.Exit(3)
		}
		_, _ = os.Stdout.WriteString("ready\n")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_ = session.Close("")
		os.Exit(0)
	}
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSessionLivenessSurvivesSuspendAndDetectsSIGKILL$")
	cmd.Env = append(os.Environ(), "ROTARI_ATTACHMENT_HELPER=1", "ROTARI_ATTACHMENT_BASE="+baseDir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("helper readiness = %q, %v", line, err)
	}

	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	if attached, err := Attached(paths, "run-1"); err != nil || !attached {
		t.Fatalf("suspended session = %v, %v, want attached", attached, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("SIGKILLed helper exited successfully")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		attached, err := Attached(paths, "run-1")
		if err == nil && !attached {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	entries, _ := os.ReadDir(filepath.Join(paths.ProjectDir, directoryName))
	t.Fatalf("SIGKILLed session still appears attached: %#v", entries)
}

func TestRemoteSessionIsConservativelyAttached(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	info := record{ID: "remote-client", RunID: "run-1", PID: 1, Host: "another-host", Disconnect: "cancel"}
	if err := writeRecord(dir, info); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, info.ID+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()

	live, stale, err := Scan(paths, "run-1")
	if err != nil || len(live) != 1 || len(stale) != 0 {
		t.Fatalf("remote scan = live %#v stale %#v err %v; want uncertain live session", live, stale, err)
	}
	if attached, err := Attached(paths, "run-1"); err != nil || !attached {
		t.Fatalf("remote Attached = %v, %v; want conservative true", attached, err)
	}
}

func TestBoundSessionSurvivesLostAcceptanceForSupervisorPolicy(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	session, err := OpenPending(paths, id, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	if err := Bind(paths, id, "run-1"); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(""); err != nil {
		t.Fatal(err)
	}
	info, err := readRecord(filepath.Join(paths.ProjectDir, directoryName, id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	info.PID = 2147483647
	info.ProcessStart = ""
	if err := writeRecord(filepath.Join(paths.ProjectDir, directoryName), info); err != nil {
		t.Fatal(err)
	}
	live, stale, err := Scan(paths, "run-1")
	if err != nil || len(live) != 0 || len(stale) != 1 || stale[0].ID != id || stale[0].Disconnect != "cancel" || !stale[0].Initiator {
		t.Fatalf("lost-acceptance scan = live %#v stale %#v err %v", live, stale, err)
	}
	if err := Forget(paths, id); err != nil {
		t.Fatal(err)
	}
	if live, stale, err = Scan(paths, "run-1"); err != nil || len(live) != 0 || len(stale) != 0 {
		t.Fatalf("acknowledged stale scan = live %#v stale %#v err %v", live, stale, err)
	}
}

func TestProcessStartIdentityDetectsPIDReuse(t *testing.T) {
	identity := processStart(os.Getpid())
	if identity == "" {
		t.Skip("process start identity is not available on this platform")
	}
	if processMayBeSame(os.Getpid(), "not-the-current-start-time") {
		t.Fatal("different process start time was treated as the same process")
	}
	if !processMayBeSame(os.Getpid(), identity) {
		t.Fatal("matching process start time was treated as a reused PID")
	}
}
