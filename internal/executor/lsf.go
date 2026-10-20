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
		Executor:  "lsf",
		JobID:     handle.Job.ID,
		AttemptID: handle.Job.AttemptID,
		Command:   handle.Job.Command,
		LSFJobID:  handle.Native,
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
	stdoutPath := filepath.Join(jobDir, state.StdoutFileName)
	stderrPath := filepath.Join(jobDir, state.StderrFileName)
	if job.EffectiveLogMode() == model.LogModeMerge {
		stdoutPath, stderrPath = filepath.Join(jobDir, "output"), filepath.Join(jobDir, "output")
	}
	wrapperPath := filepath.Join(jobDir, "lsf-wrapper.sh")
	wrapper := lsfWrapperScript(job.Command, jobDir, stdoutPath, stderrPath, jobEnvironment(job), job.WorkingDirectory, job.Timeout, job.EffectiveLogMode(), job.OpenMode, job.Output, job.Error, job.EnvMode)
	if err := os.WriteFile(wrapperPath, []byte(wrapper), store.ScriptMode); err != nil {
		return lsfJobMetadata{}, err
	}
	expandedOptions, err := ExpandShellOptions(options)
	if err != nil {
		return lsfJobMetadata{}, err
	}
	args := append([]string{"bsub"}, expandedOptions...)
	if job.EnvMode == model.EnvModeNone {
		args = append(args, "-env", "none")
	} else {
		args = append(args, "-env", "all")
	}
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
	if jobs[0].EnvMode == model.EnvModeNone {
		args = append(args, "-env", "none")
	} else {
		args = append(args, "-env", "all")
	}
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

func lsfWrapperScript(command []string, jobDir, stdoutPath, stderrPath string, environment []string, workingDirectory, timeout, logMode, openMode string, outputPaths, errorPaths []string, envMode ...string) string {
	return "#BSUB -o " + ShellQuote(stdoutPath) + "\n#BSUB -e " + ShellQuote(stderrPath) + "\n" + StatusWrapperScriptWithDestinations(command, jobDir, environment, workingDirectory, timeout, logMode, openMode, outputPaths, errorPaths, envMode...)
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
			return schedulerQuery{State: state, Err: err}
		},
		Accounting: func() schedulerAccounting {
			exitCode, state, ok, err := lsfAccounting(job.LSFJobID)
			accounting := schedulerAccounting{Status: WrapperStatus{Phase: "finished", ExitCode: exitCode, FinishedAt: nowRFC3339()}, Resolved: ok, Err: err}
			if !ok {
				accounting.State = state
			}
			return accounting
		},
	})
}

func lsfJobActive(jobID string) (bool, error) {
	schedulerState, err := lsfJobState(jobID)
	return schedulerState != "", err
}

func lsfQueueCommand(jobID string) []string {
	return []string{"bjobs", "-noheader", "-o", "stat", jobID}
}

// lsfAccountingCommands are bjobs's record of a finished job and the bhist
// history read when bjobs no longer has it.
func lsfAccountingCommands(jobID string) [][]string {
	return [][]string{{"bjobs", "-a", "-noheader", "-o", "stat exit_code", jobID}, {"bhist", "-l", jobID}}
}

func lsfJobState(jobID string) (string, error) {
	output, err := runLSFCommandWithInput(nil, lsfQueueCommand(jobID)...)
	if err != nil {
		return "", SchedulerCommandHint("bjobs", output, err)
	}
	stat := strings.ToUpper(strings.TrimSpace(string(output)))
	if stat == "DONE" || stat == "EXIT" {
		// bjobs lists finished jobs until CLEAN_PERIOD passes; accounting reads their result.
		return "", nil
	}
	if state := lsfStateName(stat); state != "" {
		return state, nil
	}
	return strings.ToLower(stat), nil
}

// lsfStateName names the bjobs STAT of a job that has not ended, or returns "".
func lsfStateName(stat string) string {
	switch stat {
	case "PEND":
		return "pending"
	case "RUN":
		return "running"
	case "PSUSP", "USUSP", "SSUSP":
		return "suspended"
	case "WAIT":
		return "waiting"
	case "PROV", "UNKWN", "ZOMBI":
		return strings.ToLower(stat)
	}
	return ""
}

// lsfHistoryResult matches the event that ended a job in bhist -l output. Only
// event lines start with a time stamp; the command and the time summary,
// which hold arbitrary text and numbers, do not.
var lsfHistoryResult = regexp.MustCompile(`(?m)^[A-Z][a-z]{2} [A-Z][a-z]{2} +[0-9]+ [0-9:]+(?: [0-9]{4})?: (Done successfully|Exited)(.*)$`)

var lsfExitCode = regexp.MustCompile(`exit code ([0-9]+)`)

// lsfAccounting reads a job's result from bjobs, or from bhist once bjobs no
// longer lists it. ok reports an ended job; otherwise a non-empty state is
// the state of a job that has not ended.
func lsfAccounting(jobID string) (int, string, bool, error) {
	commands := lsfAccountingCommands(jobID)
	if output, err := runLSFCommandWithInput(nil, commands[0]...); err == nil {
		code, state, ended := parseLSFAccounting(output)
		if state != "" {
			return code, state, ended, nil
		}
	}
	output, err := runLSFCommandWithInput(nil, commands[1]...)
	if err != nil {
		return 0, "", false, SchedulerCommandHint("bhist", output, err)
	}
	matches := lsfHistoryResult.FindAllStringSubmatch(string(output), -1)
	if len(matches) == 0 {
		return 0, "", false, nil
	}
	last := matches[len(matches)-1]
	if last[1] == "Done successfully" {
		return 0, "done", true, nil
	}
	code := 1
	if match := lsfExitCode.FindStringSubmatch(last[2]); match != nil {
		if parsed, err := strconv.Atoi(match[1]); err == nil && parsed != 0 {
			code = parsed
		}
	}
	return code, "exit", true, nil
}

func parseLSFAccounting(output []byte) (int, string, bool) {
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return 0, "", false
	}
	stat := strings.ToUpper(fields[0])
	switch stat {
	case "DONE":
		return 0, "done", true
	case "EXIT":
		code := 0
		if len(fields) > 1 {
			code, _ = strconv.Atoi(fields[1])
		}
		if code == 0 {
			code = 1
		}
		return code, "exit", true
	default:
		return 0, lsfStateName(stat), false
	}
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
