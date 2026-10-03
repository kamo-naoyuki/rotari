package run

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Plan describes how a rerun selects work and carries finished results forward.
// It is deliberately independent of filesystem paths; see PlanRerun.
type Plan struct {
	Execute        map[string]bool
	CarriedResults map[string]model.JobResult
	CarriedOrigins map[string]*model.JobOrigin
	// RerunDependencies names, for a job that executes only because a job it
	// depends on executes, that dependency, so a preview can say why.
	RerunDependencies map[string]string
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}
