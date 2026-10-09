package executor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
//
// A job can end between the moment a caller selects it and the signal, so a
// process group that is already gone is not an error: the recorded
// cancellation is all that is left to do.
func (local Local) Cancel(jobDir string) error {
	pid, err := local.recordedPID(jobDir)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: jobDir is a validated job directory and cancelled is a fixed file name.
	if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(nowRFC3339()+"\n"), local.Store.FileMode); err != nil { // NOSONAR: jobDir comes from validated job path helpers.
		return fmt.Errorf("record job cancellation: %w", err)
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func (local Local) signal(jobDir string, sig syscall.Signal) error {
	pid, err := local.recordedPID(jobDir)
	if err != nil {
		return err
	}
	if !processAlive(pid) {
		return fmt.Errorf("job is not running")
	}
	return syscall.Kill(-pid, sig)
}

func (Local) recordedPID(jobDir string) (int, error) {
	// codeql[go/path-injection]: jobDir is a validated job directory and pid is a fixed file name.
	pidData, err := os.ReadFile(filepath.Join(jobDir, "pid"))
	if err != nil {
		return 0, fmt.Errorf("job is not running")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return 0, fmt.Errorf("job is not running")
	}
	return pid, nil
}

// LocalProcessGroupAlive reports whether the process group recorded by a local
// attempt exists. known is false when the PID record cannot be inspected.
func LocalProcessGroupAlive(jobDir string) (alive, known bool) {
	path, err := state.ValidatedStateFile(jobDir, "pid")
	if err != nil {
		return false, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return false, false
	}
	err = syscall.Kill(-pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, true
	}
	return false, false
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func RunLocalJob(runDir string, job model.JobSpec, store state.Store, logf func(string, ...any)) model.JobResult {
	hostname, _ := os.Hostname()
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return failedLocalResult(job, err)
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return failedLocalResult(job, err)
	}
	if err := store.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return failedLocalResult(job, err)
	}
	if job.Name != "" {
		_ = os.WriteFile(filepath.Join(jobDir, "name"), []byte(job.Name+"\n"), store.FileMode)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte(nowRFC3339()+"\n"), store.FileMode); err != nil {
		return failedLocalResult(job, err)
	}
	if _, err := os.Stat(filepath.Join(jobDir, "cancelled")); err == nil {
		return RecordCancelledJob(jobDir, job, store)
	}
	if len(job.Command) == 0 {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "empty command"}
	}

	logJob := job
	useHelper := len(job.Output) > 0 || len(job.Error) > 0
	if useHelper {
		logJob.Output, logJob.Error = nil, nil
	}
	logs, err := openJobLogs(jobDir, logJob, store)
	if err != nil {
		return failedLocalResult(job, err)
	}
	defer logs.Close()

	var wrapper string
	if useHelper {
		helper, err := os.Executable()
		if err != nil {
			return failedLocalResult(job, fmt.Errorf("locate rotari log helper: %w", err))
		}
		wrapper = StatusWrapperScriptWithLogForwarder(job.Command, jobDir, jobEnvironment(job), job.WorkingDirectory, job.Timeout, job.OpenMode, job.Output, job.Error, helper, job.EnvMode)
	} else {
		wrapper = StatusWrapperScript(job.Command, jobDir, jobEnvironment(job), job.WorkingDirectory, job.Timeout, job.EnvMode)
	}
	wrapperPath := filepath.Join(jobDir, "local-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), store.ScriptMode); err != nil {
		return failedLocalResult(job, err)
	}
	process := exec.Command("/bin/sh", wrapperPath)
	if job.EnvMode == model.EnvModeNone {
		process.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	}
	process.Stdout, process.Stderr = logs.stdout, logs.stderr
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte("1\n"), store.FileMode)
		_ = os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
		logf("fail job=%s command=%s error=%v\n", job.ID, strings.Join(job.Command, " "), err)
		return failedLocalResult(job, err)
	}
	_ = os.WriteFile(filepath.Join(jobDir, "pid"), []byte(strconv.Itoa(process.Process.Pid)+"\n"), store.FileMode)
	logf("[%s] submit job=%s pid=%d command=%s\n", nowRFC3339(), job.ID, process.Process.Pid, strings.Join(job.Command, " "))
	err = process.Wait()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}
	errorMessage := ""
	if _, err := os.Stat(filepath.Join(jobDir, "status.json.timed_out")); err == nil {
		exitCode = TimeoutExitCode
		errorMessage = TimeoutMessage(model.TimeoutSeconds(job.Timeout))
	}
	_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte(strconv.Itoa(exitCode)+"\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
	logf("%s\n", formatJobResult(job.ID, job.Command, exitCode))
	return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: exitCode, Error: errorMessage, Hosts: []string{hostname}}
}

func failedLocalResult(job model.JobSpec, err error) model.JobResult {
	return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
}

type jobLogs struct {
	stdout, stderr *os.File
	files          []*os.File
}

func (logs *jobLogs) Close() error {
	var firstErr error
	for _, file := range logs.files {
		if err := file.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
func openJobLogs(jobDir string, job model.JobSpec, store state.Store) (*jobLogs, error) {
	logs := &jobLogs{}
	open := func(name string) (*os.File, error) {
		file, err := os.OpenFile(filepath.Join(jobDir, name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, store.FileMode)
		if err == nil {
			logs.files = append(logs.files, file)
		}
		return file, err
	}
	if job.EffectiveLogMode() == model.LogModeSeparate {
		var err error
		logs.stdout, err = open(state.StdoutFileName)
		if err != nil {
			return nil, err
		}
		logs.stderr, err = open(state.StderrFileName)
		if err != nil {
			_ = logs.Close()
			return nil, err
		}
		return logs, nil
	}
	merged, err := open("output")
	if err != nil {
		return nil, err
	}
	logs.stdout, logs.stderr = merged, merged
	return logs, nil
}
func RecordCancelledJob(jobDir string, job model.JobSpec, store state.Store) model.JobResult {
	message := "cancelled before start"
	_ = os.MkdirAll(jobDir, store.DirectoryMode)
	_ = store.WriteJSON(filepath.Join(jobDir, "command.json"), job)
	if job.Name != "" {
		_ = os.WriteFile(filepath.Join(jobDir, "name"), []byte(job.Name+"\n"), store.FileMode)
	}
	_ = os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
	name := "output"
	if job.EffectiveLogMode() == model.LogModeSeparate {
		name = state.StderrFileName
	}
	_ = os.WriteFile(filepath.Join(jobDir, name), []byte(message+"\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte("143\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
	return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: message}
}
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
func formatJobResult(jobID string, command []string, exitCode int) string {
	if exitCode == 0 {
		return fmt.Sprintf("success job=%s", jobID)
	}
	return fmt.Sprintf("fail job=%s exit=%d command=%s", jobID, exitCode, strings.Join(command, " "))
}
