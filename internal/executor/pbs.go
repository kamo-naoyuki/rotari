package executor

import (
	"context"
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

const pbsAccountingWait = 60 * time.Second
const pbsCommandTimeout = 30 * time.Second

type pbsJobMetadata struct {
	Executor    string   `json:"executor"`
	JobID       string   `json:"job_id"`
	AttemptID   string   `json:"attempt_id,omitempty"`
	Command     []string `json:"command"`
	PBSJobID    string   `json:"pbs_job_id"`
	SubmittedAt string   `json:"submitted_at"`
}

// PBS submits jobs to a PBS/Torque scheduler via qsub and tracks them
// through qstat, mirroring the Slurm executor's submit/poll/accounting model.
type PBS struct {
	Store              state.Store
	Logf               func(string, ...any)
	SubmissionRetry    schedulerSubmissionRetryPolicy
	SubmissionSpacing  *schedulerSubmissionGate
	SubmissionInterval time.Duration
}

func NewPBS(store state.Store, logf func(string, ...any)) PBS {
	return PBS{Store: store, Logf: logf, SubmissionRetry: schedulerSubmissionRetries, SubmissionSpacing: schedulerSubmissionSpacing}
}

func (PBS) Name() string { return "pbs" }

func (pbs PBS) WithRunSettings(settings RunSettings) JobExecutor {
	if settings.SubmitRetryLimit > 0 {
		pbs.SubmissionRetry.RetryLimit = settings.SubmitRetryLimit
	}
	if settings.SubmitInterval > 0 {
		pbs.SubmissionInterval = settings.SubmitInterval
	}
	return pbs
}

func (pbs PBS) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	metadata, err := submitPBSJobWithPolicies(pbs.Store, pbs.Logf, runDir, job, options, pbs.SubmissionRetry, pbs.SubmissionSpacing, pbs.SubmissionInterval)
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{Job: job, Native: metadata.PBSJobID}, nil
}

func (pbs PBS) SubmitArray(runDir string, jobs []model.JobSpec, options []string) ([]JobHandle, error) {
	return submitPBSArrayWithPolicies(pbs.Store, pbs.Logf, runDir, jobs, options, pbs.SubmissionRetry, pbs.SubmissionSpacing, pbs.SubmissionInterval)
}

func (pbs PBS) Wait(runDir string, handle JobHandle) model.JobResult {
	metadata := pbsJobMetadata{
		Executor:  "pbs",
		JobID:     handle.Job.ID,
		AttemptID: handle.Job.AttemptID,
		Command:   handle.Job.Command,
		PBSJobID:  handle.Native,
	}
	return waitPBSJob(pbs.Store, runDir, metadata)
}

func (pbs PBS) Suspend(jobDir string) error {
	return pbs.qsig(jobDir, "suspend")
}

func (pbs PBS) Resume(jobDir string) error {
	return pbs.qsig(jobDir, "resume")
}

func (pbs PBS) qsig(jobDir, signal string) error {
	metadata, err := readPBSMetadata(pbs.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runPBSCommand("qsig", "-s", signal, metadata.PBSJobID); err != nil {
		return fmt.Errorf("qsig -s %s %s: %w", signal, metadata.PBSJobID, SchedulerCommandHint("qsig", output, err))
	}
	return nil
}

func (pbs PBS) Cancel(jobDir string) error {
	metadata, err := readPBSMetadata(pbs.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runPBSCommand("qdel", metadata.PBSJobID); err != nil {
		return fmt.Errorf("qdel %s: %w", metadata.PBSJobID, SchedulerCommandHint("qdel", output, err))
	}
	return nil
}

func readPBSMetadata(store state.Store, jobDir string) (pbsJobMetadata, error) {
	var metadata pbsJobMetadata
	if err := store.ReadJSON(filepath.Join(jobDir, "job.json"), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pbsJobMetadata{}, fmt.Errorf("job is not running")
		}
		return pbsJobMetadata{}, fmt.Errorf("invalid PBS metadata: %w", err)
	}
	if metadata.PBSJobID == "" {
		return pbsJobMetadata{}, fmt.Errorf("job is not running")
	}
	return metadata, nil
}

func submitPBSJob(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, options []string) (pbsJobMetadata, error) {
	return submitPBSJobWithPolicies(store, logf, runDir, job, options, schedulerSubmissionRetries, schedulerSubmissionSpacing, 0)
}

func submitPBSJobWithPolicies(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, options []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate, interval time.Duration) (pbsJobMetadata, error) {
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return pbsJobMetadata{}, err
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return pbsJobMetadata{}, err
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return pbsJobMetadata{}, err
	}
	wrapperPath := filepath.Join(jobDir, "pbs-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(StatusWrapperScript(job.Command, jobDir, job.Environment, job.WorkingDirectory, job.Timeout)), store.ScriptMode); err != nil {
		return pbsJobMetadata{}, err
	}
	outputPath := filepath.Join(jobDir, "output")
	args := []string{"-j", "oe", "-o", outputPath}
	expandedOptions, err := ExpandShellOptions(options)
	if err != nil {
		return pbsJobMetadata{}, err
	}
	args = append(args, expandedOptions...)
	args = append(args, wrapperPath)
	output, err := retryPolicy.submit(logf, "pbs", func() ([]byte, error) {
		spacing.wait("pbs", interval)
		return runPBSCommand("qsub", args...)
	})
	if err != nil {
		return pbsJobMetadata{}, fmt.Errorf("qsub: %w: %s", err, strings.TrimSpace(string(output)))
	}
	pbsJobID := strings.TrimSpace(string(output))
	if pbsJobID == "" {
		return pbsJobMetadata{}, errors.New("qsub returned an empty job id")
	}
	metadata := pbsJobMetadata{
		Executor: "pbs", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command,
		PBSJobID: pbsJobID, SubmittedAt: nowRFC3339(),
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		return pbsJobMetadata{}, err
	}
	logf("[%s] submit job=%s pbs_job_id=%s command=%s\n", metadata.SubmittedAt, job.ID, pbsJobID, strings.Join(job.Command, " "))
	return metadata, nil
}

func submitPBSArray(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string) ([]JobHandle, error) {
	return submitPBSArrayWithPolicies(store, logf, runDir, jobs, executorOptions, schedulerSubmissionRetries, schedulerSubmissionSpacing, 0)
}

func submitPBSArrayWithPolicies(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate, interval time.Duration) ([]JobHandle, error) {
	if len(jobs) == 0 || jobs[0].ArrayTaskID == nil {
		return nil, errors.New("empty PBS array")
	}
	command := jobs[0].Command
	first, last := jobs[0].ArrayFirst, jobs[0].ArrayLast
	for _, job := range jobs {
		if job.ArrayTaskID == nil || job.ArrayFirst != first || job.ArrayLast != last || !sameStrings(job.Command, command) {
			return nil, errors.New("PBS array tasks must share one command and range")
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
	if err := RejectArraySchedulerOptions(executorOptions, "-J", "-t"); err != nil {
		return nil, err
	}
	wrapperPath := filepath.Join(runDir, jobs[0].ArrayGroup+"-pbs-array-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(schedulerArrayWrapperScript(jobs, "PBS_ARRAY_INDEX")), store.ScriptMode); err != nil {
		return nil, err
	}
	expandedOptions, err := ExpandShellOptions(executorOptions)
	if err != nil {
		return nil, err
	}
	args := []string{"-j", "oe", "-o", "/dev/null", "-J", fmt.Sprintf("%d-%d", first, last)}
	args = append(args, expandedOptions...)
	args = append(args, wrapperPath)
	output, err := retryPolicy.submit(logf, "pbs", func() ([]byte, error) {
		spacing.wait("pbs", interval)
		return runPBSCommand("qsub", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("qsub array: %w: %s", err, strings.TrimSpace(string(output)))
	}
	masterID := strings.TrimSpace(string(output))
	if bracket := strings.Index(masterID, "["); bracket >= 0 {
		masterID = masterID[:bracket] + strings.TrimPrefix(masterID[strings.Index(masterID, "]")+1:], "]")
	}
	if masterID == "" {
		return nil, errors.New("qsub returned an empty array job id")
	}
	handles := make([]JobHandle, 0, len(jobs))
	for _, job := range jobs {
		taskID := *job.ArrayTaskID
		nativeID := fmt.Sprintf("%s[%d]", masterID, taskID)
		metadata := pbsJobMetadata{Executor: "pbs", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command, PBSJobID: nativeID, SubmittedAt: nowRFC3339()}
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

func waitPBSJob(store state.Store, runDir string, job pbsJobMetadata) model.JobResult {
	jobDir, err := state.AttemptJobDir(runDir, model.JobSpec{ID: job.JobID, AttemptID: job.AttemptID})
	if err != nil {
		return model.JobResult{ID: job.JobID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	return waitForSchedulerResult(store, jobDir, job.JobID, job.Command, schedulerPollingPolicy{
		AccountingWait: pbsAccountingWait, PollInterval: time.Second,
		UnavailableError: "PBS accounting result and wrapper status are unavailable",
		JobState: func() schedulerQuery {
			state, err := pbsJobState(job.PBSJobID)
			return schedulerQuery{State: state, Failed: err != nil}
		},
		Accounting: func() schedulerAccounting {
			exitCode, ok, err := pbsAccounting(job.PBSJobID)
			return schedulerAccounting{Status: WrapperStatus{Phase: "finished", ExitCode: exitCode, FinishedAt: nowRFC3339()}, Resolved: ok, Failed: err != nil}
		},
	})
}

// pbsJobActive reports whether the scheduler still tracks the job. qstat
// exits non-zero once a job has been purged from its queue view.
func pbsJobActive(jobID string) (bool, error) {
	schedulerState, err := pbsJobState(jobID)
	return schedulerState != "", err
}

func pbsJobState(jobID string) (string, error) {
	output, err := runPBSCommand("qstat", "-f", jobID)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != "job_state" {
			continue
		}
		switch strings.TrimSpace(parts[1]) {
		case "Q":
			return "pending", nil
		case "R", "E":
			return "running", nil
		case "H":
			return "held", nil
		case "S":
			return "suspended", nil
		case "W":
			return "waiting", nil
		default:
			return strings.ToLower(strings.TrimSpace(parts[1])), nil
		}
	}
	return "", nil
}

// pbsAccounting parses "exit_status = N" out of qstat's full/history output,
// PBS's equivalent of Slurm's sacct.
func pbsAccounting(jobID string) (int, bool, error) {
	output, err := runPBSCommand("qstat", "-xf", jobID)
	if err != nil {
		return 0, false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "exit_status") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		code, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		return code, true, nil
	}
	return 0, false, nil
}

func runPBSCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pbsCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", name, pbsCommandTimeout)
	}
	return output, err
}
