package executor

import (
	"strings"

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
	Concurrency int      `json:"concurrency,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type RunSettingsMap map[string]RunSettings

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
