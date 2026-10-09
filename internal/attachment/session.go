// Package attachment coordinates live CLI clients of one run.
package attachment

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const directoryName = ".rotari-attachments"

type record struct {
	ID           string `json:"id"`
	RunID        string `json:"run_id,omitempty"`
	PID          int    `json:"pid"`
	Host         string `json:"host"`
	ProcessStart string `json:"process_start,omitempty"`
	Disconnect   string `json:"disconnect_action,omitempty"`
	Initiator    bool   `json:"initiator,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

// Info describes one live or unexpectedly lost client session.
type Info struct {
	ID         string
	RunID      string
	PID        int
	Host       string
	Disconnect string
	Initiator  bool
	Live       bool
}

// Session owns a process-held advisory lock. The lock is released by the
// kernel on process death, including SIGKILL, and remains held while stopped.
type Session struct {
	paths  state.ProjectPaths
	id     string
	file   *os.File
	closed bool
}

// NewID returns a collision-resistant identifier safe as one path element.
func NewID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

// OpenPending reserves an initiating client session before run startup. Bind
// associates it with the accepted run without releasing the process lock.
func OpenPending(paths state.ProjectPaths, id, disconnectAction string) (*Session, error) {
	return open(paths, record{ID: id, PID: os.Getpid(), Disconnect: disconnectAction, Initiator: true})
}

// Open registers a client that attaches to an existing run.
func Open(paths state.ProjectPaths, runID, id, disconnectAction string) (*Session, error) {
	return open(paths, record{ID: id, RunID: runID, PID: os.Getpid(), Disconnect: disconnectAction})
}

func open(paths state.ProjectPaths, info record) (*Session, error) {
	if !state.IsValidPathElement(info.ID) {
		return nil, fmt.Errorf("invalid attachment ID %q", info.ID)
	}
	if info.RunID != "" && !state.IsValidPathElement(info.RunID) {
		return nil, fmt.Errorf("invalid run ID %q", info.RunID)
	}
	if info.Disconnect != "" && info.Disconnect != "detach" && info.Disconnect != "cancel" {
		return nil, fmt.Errorf("invalid disconnect action %q", info.Disconnect)
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	if err := os.MkdirAll(dir, state.DirectoryMode()); err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("determine attachment host: %w", err)
	}
	info.Host = host
	info.ProcessStart = state.ProcessStart(os.Getpid())
	info.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	lockPath := filepath.Join(dir, info.ID+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, state.FileMode())
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock attachment session %s: %w", info.ID, err)
	}
	session := &Session{paths: paths, id: info.ID, file: f}
	if err := writeRecord(dir, info); err != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		return nil, err
	}
	return session, nil
}

// Bind commits the pending initiating session to a run. It also marks that
// run as using session-derived attachment state, replacing the legacy boolean.
func Bind(paths state.ProjectPaths, id, runID string) error {
	if !state.IsValidPathElement(id) || !state.IsValidPathElement(runID) {
		return fmt.Errorf("invalid attachment or run ID")
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	info, err := readRecord(filepath.Join(dir, id+".json"))
	if err != nil {
		return fmt.Errorf("read pending attachment %s: %w", id, err)
	}
	if info.ID != id || !info.Initiator {
		return fmt.Errorf("attachment %s is not an initiating session", id)
	}
	info.RunID = runID
	info.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeRecord(dir, info); err != nil {
		return err
	}
	marker := filepath.Join(dir, runID+".enabled")
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_WRONLY, state.FileMode())
	if err != nil {
		return err
	}
	return f.Close()
}

// Enabled reports whether a run has session-derived attachment state.
func Enabled(paths state.ProjectPaths, runID string) bool {
	if !state.IsValidPathElement(runID) {
		return false
	}
	_, err := os.Stat(filepath.Join(paths.ProjectDir, directoryName, runID+".enabled"))
	return err == nil
}

// Attached reports whether a run has any live client sessions. Sessions
// whose liveness cannot be disproved (notably remote hosts) remain attached.
func Attached(paths state.ProjectPaths, runID string) (bool, error) {
	sessions, _, err := Scan(paths, runID)
	return len(sessions) != 0, err
}

// Forget removes a stale or handled session record after the caller has
// applied its disconnect policy. It does not remove a live session lock.
func Forget(paths state.ProjectPaths, id string) error {
	if !state.IsValidPathElement(id) {
		return fmt.Errorf("invalid attachment ID %q", id)
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	var removeErr error
	for _, suffix := range []string{".json", ".lock"} {
		err := os.Remove(filepath.Join(dir, id+suffix))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, err)
		}
	}
	return removeErr
}

// Scan returns live sessions and sessions proved stale on this host. A stale
// initiating session configured to cancel is returned so the supervisor can
// apply that policy; callers should treat scan errors as uncertainty.
func Scan(paths state.ProjectPaths, runID string) (live, stale []Info, err error) {
	dir := filepath.Join(paths.ProjectDir, directoryName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	localHost, hostErr := os.Hostname()
	if hostErr != nil {
		return nil, nil, hostErr
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !state.IsValidPathElement(id) {
			continue
		}
		info, scanErr := scanSession(dir, id, localHost)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		if info == nil || info.RunID != runID {
			continue
		}
		if info.Live {
			live = append(live, *info)
		} else {
			stale = append(stale, *info)
		}
	}
	return live, stale, nil
}

func scanSession(dir, id, localHost string) (*Info, error) {
	held, release := probeLock(dir, id)
	defer release()
	info, err := readRecord(filepath.Join(dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read attachment session %s: %w", id, err)
	}
	client := toInfo(info, held || info.Host != localHost || processMayBeSame(info.PID, info.ProcessStart))
	return &client, nil
}

// probeLock reports whether another process may hold the session's lock,
// holding it itself until release. It never creates the lock file, so a
// scan racing Forget cannot leave one behind; a missing lock is not held.
func probeLock(dir, id string) (held bool, release func()) {
	lock, err := os.OpenFile(filepath.Join(dir, id+".lock"), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, func() {}
	}
	if err != nil {
		return true, func() {}
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return true, func() {}
	}
	return false, func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}
}

// ForgetRun removes a deleted run's attachment marker and the session
// records bound to it whose locks are not held. A live client's session is
// left for that client to close.
func ForgetRun(paths state.ProjectPaths, runID string) error {
	if !state.IsValidPathElement(runID) {
		return fmt.Errorf("invalid run ID %q", runID)
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var forgetErr error
	for _, entry := range entries {
		id, isRecord := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !isRecord || !state.IsValidPathElement(id) {
			continue
		}
		info, err := readRecord(filepath.Join(dir, entry.Name()))
		if err != nil || info.RunID != runID {
			continue
		}
		held, release := probeLock(dir, id)
		release()
		if !held {
			forgetErr = errors.Join(forgetErr, Forget(paths, id))
		}
	}
	if err := os.Remove(filepath.Join(dir, runID+".enabled")); err != nil && !errors.Is(err, os.ErrNotExist) {
		forgetErr = errors.Join(forgetErr, err)
	}
	return forgetErr
}

// Close releases the session and removes its record. An empty reason is used
// by non-initiating wait clients, which do not alter launch history.
func (session *Session) Close(reason string) error {
	if session == nil || session.closed {
		return nil
	}
	session.closed = true
	dir := filepath.Join(session.paths.ProjectDir, directoryName)
	path := filepath.Join(dir, session.id+".json")
	info, err := readRecord(path)
	keepForSupervisor := err == nil && info.Initiator && info.RunID != "" && reason == ""
	if !keepForSupervisor {
		err = errors.Join(err, Forget(session.paths, session.id))
	}
	if info.Initiator && info.RunID != "" && reason != "" && reason != "completed" {
		err = errors.Join(err, session.detachInitiator(info.RunID, reason))
	}
	return errors.Join(err, syscall.Flock(int(session.file.Fd()), syscall.LOCK_UN), session.file.Close())
}

func (session *Session) detachInitiator(runID, reason string) error {
	release, err := state.AcquireStateLock(session.paths.StateLockFile)
	if err != nil {
		return err
	}
	defer release()
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	runDir, err := state.SafeJoin(session.paths.RunsDir, runID)
	if err != nil {
		return err
	}
	status, err := state.LoadRunClientStatus(store, runDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if status.State == model.RunClientCompleted || status.State == model.RunClientCancelling {
		return nil
	}
	if _, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	status.State = model.RunClientDetached
	status.Reason = reason
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state.WriteRunClientStatus(store, runDir, status)
}

func writeRecord(dir string, info record) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(state.FileMode()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath.Join(dir, info.ID+".json"))
}

func readRecord(path string) (record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}, err
	}
	var info record
	if err := json.Unmarshal(data, &info); err != nil {
		return record{}, err
	}
	return info, nil
}

func toInfo(info record, live bool) Info {
	return Info{ID: info.ID, RunID: info.RunID, PID: info.PID, Host: info.Host, Disconnect: info.Disconnect, Initiator: info.Initiator, Live: live}
}

func processMayBeSame(pid int, start string) bool {
	if pid <= 0 {
		return false
	}
	current := state.ProcessStart(pid)
	if start != "" && current != "" {
		return current == start
	}
	return state.ProcessAlive(pid)
}
