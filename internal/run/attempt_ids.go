package run

import (
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type AttemptIDCallbacks struct {
	MakeAttemptID func(runID, jobID string, number int) string
	AttemptJobDir func(runDir string, job model.JobSpec) (string, error)
	AttemptIDName string
	RunDirName    string
	JobDirName    string
}

func AssignAttemptIDs(jobs []model.JobSpec, runID string, attempt int, callbacks AttemptIDCallbacks) {
	for index := range jobs {
		job := &jobs[index]
		job.AttemptID = callbacks.MakeAttemptID(runID, job.ID, attempt)
		environment := []string{callbacks.AttemptIDName + "=" + job.AttemptID}
		if runDirEntry, ok := EnvironmentEntry(job.Environment, callbacks.RunDirName); ok {
			runDir := strings.TrimPrefix(runDirEntry, callbacks.RunDirName+"=")
			if jobDir, err := callbacks.AttemptJobDir(runDir, *job); err == nil {
				environment = append(environment, callbacks.JobDirName+"="+jobDir)
			}
		}
		job.Environment = executor.MergeEnvironment(job.Environment, environment)
	}
}

func EnvironmentEntry(environment []string, name string) (string, bool) {
	for _, entry := range environment {
		if strings.HasPrefix(entry, name+"=") {
			return entry, true
		}
	}
	return "", false
}
