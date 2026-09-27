package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type Local struct {
	Store state.Store
	Logf  func(string, ...any)
}

func NewLocal(store state.Store, logf func(string, ...any)) Local {
	return Local{Store: store, Logf: logf}
}

func (Local) Name() string { return "local" }

func (local Local) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	return JobHandle{Job: job}, nil
}

func (local Local) Wait(runDir string, handle JobHandle) model.JobResult {
	return RunLocalJob(runDir, handle.Job, local.Store, local.Logf)
}

func (local Local) Suspend(jobDir string) error { return local.signal(jobDir, syscall.SIGSTOP) }
func (local Local) Resume(jobDir string) error  { return local.signal(jobDir, syscall.SIGCONT) }

// Cancel records the request before signalling. A SIGTERM that reaches the
// wrapper between installing its traps and spawning the command can be
// deferred until that command finishes, so the wrapper also reads this marker
// around the command launch and stops there.
func (local Local) Cancel(jobDir string) error {
	pid, err := local.runningPID(jobDir)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: jobDir is a validated job directory and cancelled is a fixed file name.
	if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(nowRFC3339()+"\n"), local.Store.FileMode); err != nil { // NOSONAR: jobDir comes from validated job path helpers.
		return fmt.Errorf("record job cancellation: %w", err)
	}
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func (local Local) signal(jobDir string, sig syscall.Signal) error {
	pid, err := local.runningPID(jobDir)
	if err != nil {
		return err
	}
	return syscall.Kill(-pid, sig)
}

func (Local) runningPID(jobDir string) (int, error) {
	// codeql[go/path-injection]: jobDir is a validated job directory and pid is a fixed file name.
	pidData, err := os.ReadFile(filepath.Join(jobDir, "pid"))
	if err != nil {
		return 0, fmt.Errorf("job is not running")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || !processAlive(pid) {
		return 0, fmt.Errorf("job is not running")
	}
	return pid, nil
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
