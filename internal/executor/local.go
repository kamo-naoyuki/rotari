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
func (local Local) Cancel(jobDir string) error  { return local.signal(jobDir, syscall.SIGTERM) }

func (Local) signal(jobDir string, sig syscall.Signal) error {
	pidData, err := os.ReadFile(filepath.Join(jobDir, "pid"))
	if err != nil {
		return fmt.Errorf("job is not running")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || !processAlive(pid) {
		return fmt.Errorf("job is not running")
	}
	return syscall.Kill(-pid, sig)
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
