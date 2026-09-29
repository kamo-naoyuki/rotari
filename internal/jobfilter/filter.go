// Package jobfilter holds the conditions that --filter-* options add to a
// job selection. A filter only narrows: a job is selected when the result
// filter and the scope select it and every condition set here holds. The
// package reads no files; callers supply what a condition needs to know
// about each job.
package jobfilter

import (
	"slices"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Filter narrows a job selection. A negated list excludes a job that
// matches any of its values.
type Filter struct {
	NotStages   []string `json:"not_stages,omitempty"`
	NotMatrices []string `json:"not_matrices,omitempty"`
}

// Empty reports whether no condition is set.
func (filter Filter) Empty() bool {
	return len(filter.NotStages) == 0 && len(filter.NotMatrices) == 0
}

// MatchesCommand reports whether command's definition passes the filter. A
// command without a stage or matrix passes the negated stage and matrix
// conditions.
func (filter Filter) MatchesCommand(command model.QueuedCommand) bool {
	if command.Stage != "" && slices.Contains(filter.NotStages, command.Stage) {
		return false
	}
	if command.Matrix != nil && slices.Contains(filter.NotMatrices, command.Matrix.BaseName) {
		return false
	}
	return true
}
