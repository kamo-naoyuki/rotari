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

func RunLocalJob(runDir string, job model.JobSpec, store state.Store, logf func(string, ...any)) model.JobResult {
	hostname, _ := os.Hostname()
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if err := store.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if job.Name != "" {
		_ = os.WriteFile(filepath.Join(jobDir, "name"), []byte(job.Name+"\n"), store.FileMode)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte(nowRFC3339()+"\n"), store.FileMode); err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if _, err := os.Stat(filepath.Join(jobDir, "cancelled")); err == nil {
		return RecordCancelledJob(jobDir, job, store)
	}

	logPath := filepath.Join(jobDir, "output")
	logFile, err := os.Create(logPath)
	if err != nil {
		return model.JobResult{ID: job.ID, ExitCode: 1, Error: err.Error()}
	}
	defer logFile.Close()
	if len(job.Command) == 0 {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "empty command"}
	}

	wrapperPath := filepath.Join(jobDir, "local-wrapper.sh")
	wrapper := StatusWrapperScript(job.Command, jobDir, job.Environment, job.WorkingDirectory, job.Timeout)
	if err := os.WriteFile(wrapperPath, []byte(wrapper), store.ScriptMode); err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}

	command := exec.Command("/bin/sh", wrapperPath)
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte("1\n"), store.FileMode)
		_ = os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
		logf("fail job=%s command=%s error=%v\n", job.ID, strings.Join(job.Command, " "), err)
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}

	_ = os.WriteFile(filepath.Join(jobDir, "pid"), []byte(strconv.Itoa(command.Process.Pid)+"\n"), store.FileMode)
	logf("[%s] submit job=%s pid=%d command=%s\n", nowRFC3339(), job.ID, command.Process.Pid, strings.Join(job.Command, " "))

	err = command.Wait()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}
	// The wrapper records a timeout itself unless the grace period ran out
	// and the watchdog killed it; either way report the timeout.
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

func RecordCancelledJob(jobDir string, job model.JobSpec, store state.Store) model.JobResult {
	message := "cancelled before start"
	_ = os.MkdirAll(jobDir, store.DirectoryMode)
	_ = store.WriteJSON(filepath.Join(jobDir, "command.json"), job)
	if job.Name != "" {
		_ = os.WriteFile(filepath.Join(jobDir, "name"), []byte(job.Name+"\n"), store.FileMode)
	}
	_ = os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "output"), []byte(message+"\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte("143\n"), store.FileMode)
	_ = os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte(nowRFC3339()+"\n"), store.FileMode)
	return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: message}
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func formatJobResult(jobID string, command []string, exitCode int) string {
	if exitCode == 0 {
		return fmt.Sprintf("success job=%s", jobID)
	}
	return fmt.Sprintf("fail job=%s exit=%d command=%s", jobID, exitCode, strings.Join(command, " "))
}
