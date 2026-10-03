package diagnose

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type Job struct {
	JobID    string
	ExitCode *int
	Error    string
	Log      string
}

func DiagnoseDefault(job Job) []model.RuleDiagnosis {
	return Diagnose(job.Error, job.Log, DefaultRules())
}

func TailLog(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return "[earlier log output omitted]\n" + value[len(value)-limit:]
}
