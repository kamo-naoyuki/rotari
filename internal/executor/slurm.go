package executor

import (
	"bufio"
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

type slurmJobMetadata struct {
	Executor    string   `json:"executor"`
	JobID       string   `json:"job_id"`
	AttemptID   string   `json:"attempt_id,omitempty"`
	Command     []string `json:"command"`
	SlurmJobID  string   `json:"slurm_job_id"`
	SubmittedAt string   `json:"submitted_at"`
}

const slurmCommandTimeout = 30 * time.Second

var slurmAccountingWait = 60 * time.Second
var slurmPollInterval = time.Second

// Slurm submits jobs to Slurm via sbatch and tracks them through
// squeue/sacct. See submitSlurmJob and waitSlurmJob for the details.
type Slurm struct {
	Store             state.Store
	Logf              func(string, ...any)
	SubmissionRetry   schedulerSubmissionRetryPolicy
	SubmissionSpacing *schedulerSubmissionGate
}

func NewSlurm(store state.Store, logf func(string, ...any)) Slurm {
	return Slurm{Store: store, Logf: logf, SubmissionRetry: schedulerSubmissionRetries, SubmissionSpacing: schedulerSubmissionSpacing}
}

func (Slurm) Name() string { return "slurm" }

func (slurm Slurm) WithRunSettings(settings RunSettings) JobExecutor {
	if settings.SubmitRetryLimit > 0 {
		slurm.SubmissionRetry.RetryLimit = settings.SubmitRetryLimit
	}
	if settings.SubmitInterval > 0 {
		slurm.SubmissionSpacing = newSchedulerSubmissionGate(settings.SubmitInterval)
	}
	return slurm
}

func (slurm Slurm) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	metadata, err := submitSlurmJobWithPolicies(slurm.Store, slurm.Logf, runDir, job, options, slurm.SubmissionRetry, slurm.SubmissionSpacing)
	if err != nil {
		return JobHandle{}, err
	}
	return JobHandle{Job: job, Native: metadata.SlurmJobID}, nil
}

func (slurm Slurm) SubmitArray(runDir string, jobs []model.JobSpec, options []string) ([]JobHandle, error) {
	return submitSlurmArrayWithPolicies(slurm.Store, slurm.Logf, runDir, jobs, options, slurm.SubmissionRetry, slurm.SubmissionSpacing)
}

func (Slurm) SupportsSparseArray() bool { return true }

func (slurm Slurm) Wait(runDir string, handle JobHandle) model.JobResult {
	metadata := slurmJobMetadata{
		Executor:   "slurm",
		JobID:      handle.Job.ID,
		AttemptID:  handle.Job.AttemptID,
		Command:    handle.Job.Command,
		SlurmJobID: handle.Native,
	}
	return waitSlurmJob(slurm.Store, runDir, metadata)
}

func (slurm Slurm) Suspend(jobDir string) error {
	return slurm.scontrol(jobDir, "suspend")
}

func (slurm Slurm) Resume(jobDir string) error {
	return slurm.scontrol(jobDir, "resume")
}

func (slurm Slurm) Cancel(jobDir string) error {
	metadata, err := readSlurmMetadata(slurm.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runSlurmCommand("scancel", metadata.SlurmJobID); err != nil {
		return fmt.Errorf("scancel %s: %w", metadata.SlurmJobID, SchedulerCommandHint("scancel", output, err))
	}
	return nil
}

func (slurm Slurm) scontrol(jobDir, command string) error {
	metadata, err := readSlurmMetadata(slurm.Store, jobDir)
	if err != nil {
		return err
	}
	if output, err := runSlurmCommand("scontrol", command, metadata.SlurmJobID); err != nil {
		return fmt.Errorf("scontrol %s %s: %w", command, metadata.SlurmJobID, SchedulerCommandHint("scontrol", output, err))
	}
	return nil
}

func readSlurmMetadata(store state.Store, jobDir string) (slurmJobMetadata, error) {
	var metadata slurmJobMetadata
	if err := store.ReadJSON(filepath.Join(jobDir, "job.json"), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return slurmJobMetadata{}, fmt.Errorf("job is not running")
		}
		return slurmJobMetadata{}, fmt.Errorf("invalid Slurm metadata: %w", err)
	}
	if metadata.SlurmJobID == "" {
		return slurmJobMetadata{}, fmt.Errorf("job is not running")
	}
	return metadata, nil
}

func submitSlurmJob(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, executorOptions []string) (slurmJobMetadata, error) {
	return submitSlurmJobWithPolicies(store, logf, runDir, job, executorOptions, schedulerSubmissionRetries, schedulerSubmissionSpacing)
}

func submitSlurmJobWithPolicies(store state.Store, logf func(string, ...any), runDir string, job model.JobSpec, executorOptions []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate) (slurmJobMetadata, error) {
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return slurmJobMetadata{}, err
	}
	if err := os.MkdirAll(jobDir, store.DirectoryMode); err != nil {
		return slurmJobMetadata{}, err
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return slurmJobMetadata{}, err
	}
	wrapperPath := filepath.Join(jobDir, "slurm-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(StatusWrapperScript(job.Command, jobDir, job.Environment, job.WorkingDirectory)), store.ScriptMode); err != nil {
		return slurmJobMetadata{}, err
	}
	outputPath := filepath.Join(jobDir, "output")
	showCommand := fmt.Sprintf("rotari show --job-id %s", ShellQuote(job.AttemptID))
	args := []string{"--parsable", "--job-name=" + showCommand, "--output=" + outputPath, "--error=" + outputPath}
	expandedOptions, err := ExpandShellOptions(executorOptions)
	if err != nil {
		return slurmJobMetadata{}, err
	}
	args = append(args, expandedOptions...)
	args = append(args, wrapperPath)
	output, err := retryPolicy.submit(logf, "slurm", func() ([]byte, error) {
		spacing.wait("slurm")
		return runSlurmCommand("sbatch", args...)
	})
	if err != nil {
		return slurmJobMetadata{}, fmt.Errorf("sbatch: %w: %s", err, strings.TrimSpace(string(output)))
	}
	slurmJobID := strings.TrimSpace(strings.SplitN(string(output), ";", 2)[0])
	if slurmJobID == "" {
		return slurmJobMetadata{}, errors.New("sbatch returned an empty job id")
	}
	metadata := slurmJobMetadata{
		Executor: "slurm", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command,
		SlurmJobID: slurmJobID, SubmittedAt: nowRFC3339(),
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		return slurmJobMetadata{}, err
	}
	logf("[%s] submit job=%s slurm_job_id=%s command=%s\n", metadata.SubmittedAt, job.ID, slurmJobID, strings.Join(job.Command, " "))
	return metadata, nil
}

func submitSlurmArray(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string) ([]JobHandle, error) {
	return submitSlurmArrayWithPolicies(store, logf, runDir, jobs, executorOptions, schedulerSubmissionRetries, schedulerSubmissionSpacing)
}

func submitSlurmArrayWithPolicies(store state.Store, logf func(string, ...any), runDir string, jobs []model.JobSpec, executorOptions []string, retryPolicy schedulerSubmissionRetryPolicy, spacing *schedulerSubmissionGate) ([]JobHandle, error) {
	if len(jobs) == 0 || jobs[0].ArrayTaskID == nil {
		return nil, errors.New("empty Slurm array")
	}
	command := jobs[0].Command
	first, last := jobs[0].ArrayFirst, jobs[0].ArrayLast
	for _, job := range jobs {
		if job.ArrayTaskID == nil || job.ArrayFirst != first || job.ArrayLast != last || !sameStrings(job.Command, command) {
			return nil, errors.New("Slurm array tasks must share one command and range")
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
	wrapperPath := filepath.Join(runDir, jobs[0].ArrayGroup+"-array-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(schedulerArrayWrapperScript(jobs, "SLURM_ARRAY_TASK_ID")), store.ScriptMode); err != nil {
		return nil, err
	}
	expandedOptions, err := ExpandShellOptions(executorOptions)
	if err != nil {
		return nil, err
	}
	for _, option := range expandedOptions {
		if option == "--array" || strings.HasPrefix(option, "--array=") {
			return nil, errors.New("executor options must not include --array when rotari --array is used")
		}
	}
	args := []string{
		"--parsable",
		"--array=" + slurmArrayOption(jobs, first, last),
		"--job-name=rotari-array",
		"--output=/dev/null",
		"--error=/dev/null",
	}
	args = append(args, expandedOptions...)
	args = append(args, wrapperPath)
	output, err := retryPolicy.submit(logf, "slurm", func() ([]byte, error) {
		spacing.wait("slurm")
		return runSlurmCommand("sbatch", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("sbatch array: %w: %s", err, strings.TrimSpace(string(output)))
	}
	masterID := strings.TrimSpace(strings.SplitN(string(output), ";", 2)[0])
	if masterID == "" {
		return nil, errors.New("sbatch returned an empty array job id")
	}
	handles := make([]JobHandle, 0, len(jobs))
	for _, job := range jobs {
		task := *job.ArrayTaskID
		nativeID := fmt.Sprintf("%s_%d", masterID, task)
		metadata := slurmJobMetadata{Executor: "slurm", JobID: job.ID, AttemptID: job.AttemptID, Command: job.Command, SlurmJobID: nativeID, SubmittedAt: nowRFC3339()}
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

func slurmArrayOption(jobs []model.JobSpec, first, last int) string {
	if completeTaskRange(jobs, first, last) {
		return fmt.Sprintf("%d-%d", first, last)
	}
	tasks := make([]string, 0, len(jobs))
	for _, job := range jobs {
		tasks = append(tasks, strconv.Itoa(*job.ArrayTaskID))
	}
	return strings.Join(tasks, ",")
}

func completeTaskRange(jobs []model.JobSpec, first, last int) bool {
	if len(jobs) != last-first+1 {
		return false
	}
	seen := make(map[int]bool, len(jobs))
	for _, job := range jobs {
		if job.ArrayTaskID == nil || *job.ArrayTaskID < first || *job.ArrayTaskID > last || seen[*job.ArrayTaskID] {
			return false
		}
		seen[*job.ArrayTaskID] = true
	}
	return len(seen) == len(jobs)
}

func waitSlurmJob(store state.Store, runDir string, job slurmJobMetadata) model.JobResult {
	jobDir, err := state.AttemptJobDir(runDir, model.JobSpec{ID: job.JobID, AttemptID: job.AttemptID})
	if err != nil {
		return model.JobResult{ID: job.JobID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	return waitForSchedulerResult(store, jobDir, job.JobID, job.Command, schedulerPollingPolicy{
		AccountingWait: slurmAccountingWait, PollInterval: slurmPollInterval,
		UnavailableError: "Slurm accounting result and wrapper status are unavailable",
		JobState: func() schedulerQuery {
			state, err := slurmJobState(job.SlurmJobID)
			return schedulerQuery{State: state, Failed: err != nil}
		},
		Accounting: func() schedulerAccounting {
			exitCode, state, ok, err := slurmAccounting(job.SlurmJobID)
			return schedulerAccounting{Status: WrapperStatus{Phase: state, ExitCode: exitCode, Error: state, FinishedAt: nowRFC3339()}, Resolved: ok, Failed: err != nil}
		},
	})
}

func slurmJobActive(jobID string) (bool, error) {
	schedulerState, err := slurmJobState(jobID)
	return schedulerState != "", err
}

func slurmJobState(jobID string) (string, error) {
	output, err := runSlurmCommand("squeue", "--noheader", "--jobs", jobID, "--format=%T")
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(string(output))), nil
}

func slurmAccounting(jobID string) (int, string, bool, error) {
	output, err := runSlurmCommand("sacct", "--noheader", "--parsable2", "--allocations", "--jobs", jobID, "--format=State,ExitCode")
	if err != nil {
		return 0, "", false, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Split(strings.TrimSpace(scanner.Text()), "|")
		if len(fields) < 2 || fields[0] == "" {
			continue
		}
		state := strings.SplitN(fields[0], "+", 2)[0]
		exitCode := parseSlurmExitCode(fields[1])
		if state == "COMPLETED" {
			exitCode = 0
		}
		return exitCode, strings.ToLower(state), true, nil
	}
	return 0, "", false, nil
}

func runSlurmCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), slurmCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s timed out after %s", name, slurmCommandTimeout)
	}
	return output, err
}

func parseSlurmExitCode(value string) int {
	value = strings.SplitN(value, ":", 2)[0]
	code, err := strconv.Atoi(value)
	if err != nil {
		return 1
	}
	return code
}
