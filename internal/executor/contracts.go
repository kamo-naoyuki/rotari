package executor

import (
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type JobHandle struct {
	Job    model.JobSpec
	Native string
}

type JobExecutor interface {
	Name() string
	Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error)
	Wait(runDir string, handle JobHandle) model.JobResult
}

// RunSettingsConfigurer returns an executor copy configured for one run.
// Implementations must not mutate a shared registry instance.
type RunSettingsConfigurer interface {
	WithRunSettings(RunSettings) JobExecutor
}

type ArraySubmitter interface {
	SubmitArray(runDir string, jobs []model.JobSpec, options []string) ([]JobHandle, error)
}

type SparseArraySupporter interface {
	SupportsSparseArray() bool
}

type Suspender interface {
	Suspend(jobDir string) error
	Resume(jobDir string) error
}

type Canceller interface {
	Cancel(jobDir string) error
}

type RunSettings struct {
	Concurrency      int           `json:"concurrency,omitempty"`
	Options          []string      `json:"options,omitempty"`
	SubmitInterval   time.Duration `json:"submit_interval,omitempty"`
	SubmitRetryLimit int           `json:"submit_retry_limit,omitempty"`
}

// RunSettingsMap holds per-run settings by executor name.
type RunSettingsMap map[string]RunSettings

// RunSettingNames lists the executors that take per-run settings, such as
// --slurm-concurrency.
var RunSettingNames = []string{"ssh", "slurm", "pbs", "lsf", "sge"}

// Concurrency returns the concurrency set for name, or fallback when none is.
func (settings RunSettingsMap) Concurrency(name string, fallback int) int {
	if concurrency := settings[name].Concurrency; concurrency > 0 {
		return concurrency
	}
	return fallback
}

// Options returns the options set for name, or fallback when none are.
func (settings RunSettingsMap) Options(name string, fallback []string) []string {
	if options := settings[name].Options; len(options) > 0 {
		return options
	}
	return fallback
}

func MergeEnvironment(base, overrides []string) []string {
	values := make(map[string]string)
	order := make([]string, 0, len(base)+len(overrides))
	for _, entry := range append(append([]string(nil), base...), overrides...) {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		if _, exists := values[parts[0]]; !exists {
			order = append(order, parts[0])
		}
		values[parts[0]] = parts[1]
	}
	merged := make([]string, 0, len(order))
	for _, name := range order {
		merged = append(merged, name+"="+values[name])
	}
	return merged
}

func jobEnvironment(job model.JobSpec) []string {
	return MergeEnvironment(job.InheritedEnvironment, job.Environment)
}

// LSFExplicitEnvironmentVariable reports variables omitted by LSF's
// documented `-env all` transfer rules. The run environment adapter adds
// caller values for these names to the job wrapper as a targeted correction.
func LSFExplicitEnvironmentVariable(name string) bool {
	switch name {
	case "HOME", "LS_JOBPID", "LSB_ACCT_MAP", "LSB_EXIT_PRE_ABORT", "LSB_EXIT_REQUEUE",
		"LSB_EVENT_ATTRIB", "LSB_HOSTS", "LSB_INTERACTIVE", "LSB_INTERACTIVE_SSH",
		"LSB_INTERACTIVE_TTY", "LSB_JOBFILENAME", "LSB_JOBGROUP", "LSB_JOBID",
		"LSB_JOBNAME", "LSB_JOB_STARTER", "LSB_QUEUE", "LSB_RESTART", "LSB_TRAPSIGS",
		"LSB_XJOB_SSH", "LSF_VERSION", "PWD", "USER", "VIRTUAL_HOSTNAME", "TERM", "TERMCAP":
		return true
	default:
		return strings.HasPrefix(name, "LSB_SUB_")
	}
}
