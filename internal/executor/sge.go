package executor

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const sgeAccountingWait = 60 * time.Second
const sgeCommandTimeout = 30 * time.Second

type sgeJobMetadata struct {
	Executor    string   `json:"executor"`
	JobID       string   `json:"job_id"`
	AttemptID   string   `json:"attempt_id,omitempty"`
	Command     []string `json:"command"`
	SGEJobID    string   `json:"sge_job_id"`
	SubmittedAt string   `json:"submitted_at"`
}

type sgeSubmissionPolicy struct {
	retry    schedulerSubmissionRetryPolicy
	spacing  *schedulerSubmissionGate
	interval time.Duration
}

// SGE submits jobs through the common Grid Engine qsub/qstat/qacct commands.
// Array tasks are submitted independently so this does not depend on the
// differing native array support and job-id syntax across Grid Engine forks.
type SGE struct {
	Store              state.Store
	Logf               func(string, ...any)
	SubmissionRetry    schedulerSubmissionRetryPolicy
	SubmissionSpacing  *schedulerSubmissionGate
	SubmissionInterval time.Duration
}

func NewSGE(store state.Store, logf func(string, ...any)) SGE {
	return SGE{Store: store, Logf: logf, SubmissionRetry: schedulerSubmissionRetries, SubmissionSpacing: schedulerSubmissionSpacing}
}

func (SGE) Name() string { return "sge" }

func (sge SGE) WithRunSettings(settings RunSettings) JobExecutor {
	if settings.SubmitRetryLimit > 0 {
		sge.SubmissionRetry.RetryLimit = settings.SubmitRetryLimit
	}
	if settings.SubmitInterval > 0 {
		sge.SubmissionInterval = settings.SubmitInterval
	}
	return sge
}

func (sge SGE) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	policy := sgeSubmissionPolicy{retry: sge.SubmissionRetry, spacing: sge.SubmissionSpacing, interval: sge.SubmissionInterval}
	metadata, err := submitSGEJobWithPolicies(sge.Store, sge.Logf, runDir, job, options, policy)
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{Job: job, Native: metadata.SGEJobID}, nil
}

func (sge SGE) Wait(runDir string, handle JobHandle) model.JobResult {
	metadata := sgeJobMetadata{Executor: "sge", JobID: handle.Job.ID, AttemptID: handle.Job.AttemptID, Command: handle.Job.Command, SGEJobID: handle.Native}
	return waitSGEJob(sge.Store, runDir, metadata)
}

func (sge SGE) Cancel(jobDir string) error {
	metadata, err := readSGEMetadata(sge.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runSGECommand("qdel", metadata.SGEJobID); err != nil {
		return fmt.Errorf("qdel %s: %w", metadata.SGEJobID, SchedulerCommandHint("qdel", output, err))
	}
	return nil
}

func (sge SGE) Suspend(jobDir string) error { return sge.qmod(jobDir, "-sj") }

func (sge SGE) Resume(jobDir string) error { return sge.qmod(jobDir, "-usj") }

func (sge SGE) qmod(jobDir, action string) error {
	metadata, err := readSGEMetadata(sge.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runSGECommand("qmod", action, metadata.SGEJobID); err != nil {
		return fmt.Errorf("qmod %s %s: %w", action, metadata.SGEJobID, SchedulerCommandHint("qmod", output, err))
	}
	return nil
}

func readSGEMetadata(store state.Store, jobDir string) (sgeJobMetadata, error) {
	var metadata sgeJobMetadata
	if err := store.ReadJSON(filepath.Join(jobDir, "job.json"), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sgeJobMetadata{}, errors.New("job is not running")
		}
		return sgeJobMetadata{}, fmt.Errorf("invalid SGE metadata: %w", err)
	}
	if metadata.SGEJobID == "" {
		return sgeJobMetadata{}, errors.New("job is not running")
	}
	return metadata, nil
}

func submitSGEJobWithPolicies(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, options []string, policy sgeSubmissionPolicy) (sgeJobMetadata, error) {
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return sgeJobMetadata{}, err
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return sgeJobMetadata{}, err
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return sgeJobMetadata{}, err
	}
	wrapperPath := filepath.Join(jobDir, "sge-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(StatusWrapperScriptWithDestinations(job.Command, jobDir, jobEnvironment(job), job.WorkingDirectory, job.Timeout, job.EffectiveLogMode(), job.OpenMode, job.Output, job.Error, job.EnvMode)), store.ScriptMode); err != nil {
		return sgeJobMetadata{}, err
	}
	stdoutPath := filepath.Join(jobDir, state.StdoutFileName)
	stderrPath := filepath.Join(jobDir, state.StderrFileName)
	if job.EffectiveLogMode() == model.LogModeMerge {
		stdoutPath, stderrPath = filepath.Join(jobDir, "output"), filepath.Join(jobDir, "output")
	}
	args := []string{"-terse", "-o", stdoutPath, "-e", stderrPath}
	expandedOptions, err := ExpandShellOptions(options)
	if err != nil {
		return sgeJobMetadata{}, err
	}
	args = append(args, expandedOptions...)
	if job.EnvMode != model.EnvModeNone {
		args = append(args, "-V")
	}
	args = append(args, wrapperPath)
	output, err := policy.retry.submit(logf, "sge", func() ([]byte, error) {
		policy.spacing.wait("sge", policy.interval)
		return runSGECommand("qsub", args...)
	})
	if err != nil {
		return sgeJobMetadata{}, fmt.Errorf("qsub: %w: %s", err, strings.TrimSpace(string(output)))
	}
	jobID, err := parseSGEJobID(string(output))
	if err != nil {
		return sgeJobMetadata{}, err
	}
	metadata := sgeJobMetadata{Executor: "sge", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command, SGEJobID: jobID, SubmittedAt: nowRFC3339()}
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		return sgeJobMetadata{}, err
	}
	if logf != nil {
		logf("[%s] submit job=%s sge_job_id=%s command=%s\n", metadata.SubmittedAt, job.ID, jobID, strings.Join(job.Command, " "))
	}
	return metadata, nil
}

func parseSGEJobID(output string) (string, error) {
	jobID := strings.TrimSpace(output)
	if jobID == "" {
		return "", errors.New("qsub returned an empty job id")
	}
	// qsub -terse should return only the numeric ID. Some forks append task
	// ranges or other terse metadata; the leading job ID remains the first token.
	jobID = strings.Fields(jobID)[0]
	if dot := strings.IndexByte(jobID, '.'); dot >= 0 {
		jobID = jobID[:dot]
	}
	if jobID == "" {
		return "", errors.New("qsub returned an invalid job id")
	}
	if _, err := strconv.ParseUint(jobID, 10, 64); err != nil {
		return "", fmt.Errorf("qsub returned an invalid job id %q", jobID)
	}
	return jobID, nil
}

func waitSGEJob(store state.Store, runDir string, job sgeJobMetadata) model.JobResult {
	jobDir, err := state.AttemptJobDir(runDir, model.JobSpec{ID: job.JobID, AttemptID: job.AttemptID})
	if err != nil {
		return model.JobResult{ID: job.JobID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	return waitForSchedulerResult(store, jobDir, job.JobID, job.Command, schedulerPollingPolicy{
		AccountingWait: sgeAccountingWait, PollInterval: time.Second,
		UnavailableError: "Grid Engine accounting result and wrapper status are unavailable",
		JobState: func() schedulerQuery {
			state, err := sgeJobState(job.SGEJobID)
			return schedulerQuery{State: state, Failed: err != nil}
		},
		Accounting: func() schedulerAccounting {
			exitCode, ok, err := sgeAccounting(job.SGEJobID)
			return schedulerAccounting{Status: WrapperStatus{Phase: "finished", ExitCode: exitCode, FinishedAt: nowRFC3339()}, Resolved: ok, Failed: err != nil}
		},
	})
}

type sgeQueueXML struct {
	QueueInfo struct {
		Jobs []struct {
			JobNumber string `xml:"JB_job_number"`
			State     string `xml:"state,attr"`
		} `xml:"job_list"`
	} `xml:"queue_info"`
}

func sgeJobActive(jobID string) (bool, error) {
	state, err := sgeJobState(jobID)
	return state != "", err
}

func sgeJobState(jobID string) (string, error) {
	output, err := runSGECommand("qstat", "-xml")
	if err != nil {
		return "", err
	}
	var listing sgeQueueXML
	if err := xml.Unmarshal(output, &listing); err != nil {
		return "", fmt.Errorf("parse qstat XML: %w", err)
	}
	for _, job := range listing.QueueInfo.Jobs {
		if job.JobNumber != jobID {
			continue
		}
		switch strings.ToLower(job.State) {
		case "qw", "hqw", "h", "w", "swr":
			return "pending", nil
		case "r", "t", "rr":
			return "running", nil
		case "s", "suspended", "ts":
			return "suspended", nil
		case "eqw":
			return "error", nil
		default:
			return strings.ToLower(job.State), nil
		}
	}
	return "", nil
}

func sgeAccounting(jobID string) (int, bool, error) {
	output, err := runSGECommand("qacct", "-j", jobID)
	if err != nil {
		return 0, false, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			values[fields[0]] = fields[1]
		}
	}
	exitStatus, exitErr := strconv.Atoi(values["exit_status"])
	failed, failedErr := strconv.Atoi(values["failed"])
	if exitErr != nil || failedErr != nil {
		return 0, false, nil
	}
	if failed != 0 && exitStatus == 0 {
		exitStatus = failed
	}
	return exitStatus, true, nil
}

func runSGECommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sgeCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", name, sgeCommandTimeout)
	}
	return output, err
}
