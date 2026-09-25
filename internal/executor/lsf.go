package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const lsfAccountingWait = 60 * time.Second
const lsfCommandTimeout = 30 * time.Second

type lsfJobMetadata struct {
	Executor    string   `json:"executor"`
	JobID       string   `json:"job_id"`
	AttemptID   string   `json:"attempt_id,omitempty"`
	Command     []string `json:"command"`
	LSFJobID    string   `json:"lsf_job_id"`
	SubmittedAt string   `json:"submitted_at"`
}

// LSF submits jobs to IBM LSF via bsub and tracks them with bjobs/bhist.
type LSF struct {
	Store              state.Store
	Logf               func(string, ...any)
	SubmissionRetry    schedulerSubmissionRetryPolicy
	SubmissionSpacing  *schedulerSubmissionGate
	SubmissionInterval time.Duration
}

func NewLSF(store state.Store, logf func(string, ...any)) LSF {
	return LSF{Store: store, Logf: logf, SubmissionRetry: schedulerSubmissionRetries, SubmissionSpacing: schedulerSubmissionSpacing}
}

func (LSF) Name() string { return "lsf" }

func (lsf LSF) WithRunSettings(settings RunSettings) JobExecutor {
	if settings.SubmitRetryLimit > 0 {
		lsf.SubmissionRetry.RetryLimit = settings.SubmitRetryLimit
	}
	if settings.SubmitInterval > 0 {
		lsf.SubmissionInterval = settings.SubmitInterval
	}
	return lsf
}

func (lsf LSF) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	metadata, err := submitLSFJobWithPolicies(lsf.Store, lsf.Logf, runDir, job, options, lsf.SubmissionRetry, lsf.SubmissionSpacing, lsf.SubmissionInterval)
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{Job: job, Native: metadata.LSFJobID}, nil
}

func (lsf LSF) SubmitArray(runDir string, jobs []model.JobSpec, options []string) ([]JobHandle, error) {
	return submitLSFArrayWithPolicies(lsf.Store, lsf.Logf, runDir, jobs, options, lsf.SubmissionRetry, lsf.SubmissionSpacing, lsf.SubmissionInterval)
}

func (lsf LSF) Wait(runDir string, handle JobHandle) model.JobResult {
	metadata := lsfJobMetadata{
		Executor: "lsf",
		JobID:    handle.Job.ID,
		Command:  handle.Job.Command,
		LSFJobID: handle.Native,
	}
	return waitLSFJob(lsf.Store, runDir, metadata)
}

func (lsf LSF) Suspend(jobDir string) error {
	return lsf.runControl(jobDir, "bstop")
}

func (lsf LSF) Resume(jobDir string) error {
	return lsf.runControl(jobDir, "bresume")
}

func (lsf LSF) Cancel(jobDir string) error {
	metadata, err := readLSFMetadata(lsf.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runLSFCommand("bkill", metadata.LSFJobID); err != nil {
		return fmt.Errorf("bkill %s: %w", metadata.LSFJobID, SchedulerCommandHint("bkill", output, err))
	}
	return nil
}

func (lsf LSF) runControl(jobDir, command string) error {
	metadata, err := readLSFMetadata(lsf.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runLSFCommand(command, metadata.LSFJobID); err != nil {
		return fmt.Errorf("%s %s: %w", command, metadata.LSFJobID, SchedulerCommandHint(command, output, err))
	}
	return nil
}

func readLSFMetadata(store state.Store, jobDir string) (lsfJobMetadata, error) {
	var metadata lsfJobMetadata
	if err := store.ReadJSON(filepath.Join(jobDir, "job.json"), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lsfJobMetadata{}, errors.New("job is not running")
		}
		return lsfJobMetadata{}, fmt.Errorf("invalid LSF metadata: %w", err)
	}
	if metadata.LSFJobID == "" {
		return lsfJobMetadata{}, errors.New("job is not running")
	}
	return metadata, nil
}

func submitLSFJob(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, options []string) (lsfJobMetadata, error) {
	return submitLSFJobWithPolicies(store, logf, runDir, job, options, schedulerSubmissionRetries, schedulerSubmissionSpacing, 0)
}

func submitLSFJobWithPolicies(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, options []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate, interval time.Duration) (lsfJobMetadata, error) {
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return lsfJobMetadata{}, err
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return lsfJobMetadata{}, err
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return lsfJobMetadata{}, err
	}
	outputPath := filepath.Join(jobDir, "output")
	wrapperPath := filepath.Join(jobDir, "lsf-wrapper.sh")
	wrapper := lsfWrapperScript(job.Command, jobDir, outputPath, job.Environment, job.WorkingDirectory, job.Timeout)
	if err := os.WriteFile(wrapperPath, []byte(wrapper), store.ScriptMode); err != nil {
		return lsfJobMetadata{}, err
	}
	expandedOptions, err := ExpandShellOptions(options)
	if err != nil {
		return lsfJobMetadata{}, err
	}
	args := append([]string{"bsub"}, expandedOptions...)
	output, err := retryPolicy.submit(logf, "lsf", func() ([]byte, error) {
		spacing.wait("lsf", interval)
		return runLSFCommandWithInput(bytes.NewReader([]byte(wrapper)), args...)
	})
	if err != nil {
		return lsfJobMetadata{}, fmt.Errorf("bsub: %w: %s", err, strings.TrimSpace(string(output)))
	}
	jobID, err := parseLSFJobID(string(output))
	if err != nil {
		return lsfJobMetadata{}, err
	}
	metadata := lsfJobMetadata{
		Executor: "lsf", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command,
		LSFJobID: jobID, SubmittedAt: nowRFC3339(),
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		return lsfJobMetadata{}, err
	}
	logf("[%s] submit job=%s lsf_job_id=%s command=%s\n", metadata.SubmittedAt, job.ID, jobID, strings.Join(job.Command, " "))
	return metadata, nil
}

func submitLSFArray(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string) ([]JobHandle, error) {
	return submitLSFArrayWithPolicies(store, logf, runDir, jobs, executorOptions, schedulerSubmissionRetries, schedulerSubmissionSpacing, 0)
}

func submitLSFArrayWithPolicies(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate, interval time.Duration) ([]JobHandle, error) {
	if len(jobs) == 0 || jobs[0].ArrayTaskID == nil {
		return nil, errors.New("empty LSF array")
	}
	command := jobs[0].Command
	first, last := jobs[0].ArrayFirst, jobs[0].ArrayLast
	for _, job := range jobs {
		if job.ArrayTaskID == nil || job.ArrayFirst != first || job.ArrayLast != last || !sameStrings(job.Command, command) {
			return nil, errors.New("LSF array tasks must share one command and range")
		}
		jobDir, err := state.AttemptJobDir(runDir, job)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
			return nil, err
		}
		if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
			return nil, err
		}
	}
	if err := RejectArraySchedulerOptions(executorOptions, "-J"); err != nil {
		return nil, err
	}
	wrapper := "#BSUB -o /dev/null\n#BSUB -e /dev/null\n" + schedulerArrayWrapperScript(jobs, "LSB_JOBINDEX")
	expandedOptions, err := ExpandShellOptions(executorOptions)
	if err != nil {
		return nil, err
	}
	args := []string{"bsub", "-J", fmt.Sprintf("rotari[%d-%d]", first, last)}
	args = append(args, expandedOptions...)
	output, err := retryPolicy.submit(logf, "lsf", func() ([]byte, error) {
		spacing.wait("lsf", interval)
		return runLSFCommandWithInput(bytes.NewReader([]byte(wrapper)), args...)
	})
	if err != nil {
		return nil, fmt.Errorf("bsub array: %w: %s", err, strings.TrimSpace(string(output)))
	}
	masterID, err := parseLSFJobID(string(output))
	if err != nil {
		return nil, err
	}
	handles := make([]JobHandle, 0, len(jobs))
	for _, job := range jobs {
		taskID := *job.ArrayTaskID
		nativeID := fmt.Sprintf("%s[%d]", masterID, taskID)
		metadata := lsfJobMetadata{Executor: "lsf", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command, LSFJobID: nativeID, SubmittedAt: nowRFC3339()}
		jobDir, err := state.AttemptJobDir(runDir, job)
		if err != nil {
			return nil, err
		}
		if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
			return nil, err
		}
		handles = append(handles, JobHandle{Job: job, Native: nativeID})
	}
	return handles, nil
}

func lsfWrapperScript(command []string, jobDir, outputPath string, environment []string, workingDirectory, timeout string) string {
	return "#BSUB -o " + ShellQuote(outputPath) + "\n#BSUB -e " + ShellQuote(outputPath) + "\n" + StatusWrapperScript(command, jobDir, environment, workingDirectory, timeout)
}

var lsfJobIDPattern = regexp.MustCompile(`<([0-9]+)>`)

func parseLSFJobID(output string) (string, error) {
	match := lsfJobIDPattern.FindStringSubmatch(output)
	if len(match) == 2 {
		return match[1], nil
	}
	return "", errors.New("bsub returned no job id")
}

func waitLSFJob(store state.Store, runDir string, job lsfJobMetadata) model.JobResult {
	jobDir, err := state.AttemptJobDir(runDir, model.JobSpec{ID: job.JobID, AttemptID: job.AttemptID})
	if err != nil {
		return model.JobResult{ID: job.JobID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	return waitForSchedulerResult(store, jobDir, job.JobID, job.Command, schedulerPollingPolicy{
		AccountingWait: lsfAccountingWait, PollInterval: time.Second,
		UnavailableError: "LSF accounting result and wrapper status are unavailable",
		JobState: func() schedulerQuery {
			state, err := lsfJobState(job.LSFJobID)
			return schedulerQuery{State: state, Failed: err != nil}
		},
		Accounting: func() schedulerAccounting {
			exitCode, ok, err := lsfAccounting(job.LSFJobID)
			return schedulerAccounting{Status: WrapperStatus{Phase: "finished", ExitCode: exitCode, FinishedAt: nowRFC3339()}, Resolved: ok, Failed: err != nil}
		},
	})
}

func lsfJobActive(jobID string) (bool, error) {
	schedulerState, err := lsfJobState(jobID)
	return schedulerState != "", err
}

func lsfJobState(jobID string) (string, error) {
	output, err := runLSFCommand("bjobs", "-noheader", "-o", "stat", jobID)
	if err != nil {
		return "", err
	}
	switch strings.ToUpper(strings.TrimSpace(string(output))) {
	case "PEND":
		return "pending", nil
	case "RUN":
		return "running", nil
	case "PSUSP", "USUSP", "SSUSP":
		return "suspended", nil
	case "WAIT":
		return "waiting", nil
	default:
		return strings.ToLower(strings.TrimSpace(string(output))), nil
	}
}

func lsfAccounting(jobID string) (int, bool, error) {
	output, err := runLSFCommand("bjobs", "-a", "-noheader", "-o", "stat exit_code", jobID)
	if err != nil {
		output, err = runLSFCommand("bhist", "-l", jobID)
		if err != nil {
			return 0, false, err
		}
	}
	text := string(output)
	if strings.Contains(text, "DONE") || strings.Contains(text, "Done successfully") {
		return 0, true, nil
	}
	if match := regexp.MustCompile(`(?:EXIT|Exited)\s*\(?([0-9]+)\)?`).FindStringSubmatch(text); len(match) == 2 {
		code, parseErr := strconv.Atoi(match[1])
		return code, parseErr == nil, parseErr
	}
	if match := regexp.MustCompile(`\s([0-9]+)\s*$`).FindStringSubmatch(strings.TrimSpace(text)); len(match) == 2 {
		code, parseErr := strconv.Atoi(match[1])
		return code, parseErr == nil, parseErr
	}
	return 0, false, nil
}

func runLSFCommand(name string, args ...string) ([]byte, error) {
	return runLSFCommandWithInput(nil, append([]string{name}, args...)...)
}

func runLSFCommandWithInput(input *bytes.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lsfCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	if input != nil {
		command.Stdin = input
	}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", args[0], lsfCommandTimeout)
	}
	return output, err
}
