// Package jobfilter holds the conditions that --filter-* options add to a
// job selection. A filter only narrows: a job is selected when the result
// filter and the scope select it and every condition set here holds. The
// package reads no files; callers supply what a condition needs to know
// about each job.
package jobfilter

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Filter narrows a job selection. A negated list excludes a job that
// matches any of its values.
type Filter struct {
	NotStages      []string        `json:"not_stages,omitempty"`
	NotMatrices    []string        `json:"not_matrices,omitempty"`
	Command        string          `json:"command,omitempty"`
	ExitCodes      []int           `json:"exit_codes,omitempty"`
	FailureKinds   []string        `json:"failure_kinds,omitempty"`
	Diagnoses      []string        `json:"diagnoses,omitempty"`
	Changed        bool            `json:"changed,omitempty"`
	New            bool            `json:"new,omitempty"`
	ChangedIDs     map[string]bool `json:"-"`
	NewIDs         map[string]bool `json:"-"`
	Hosts          []string        `json:"hosts,omitempty"`
	StartedAfter   *time.Time      `json:"started_after,omitempty"`
	StartedBefore  *time.Time      `json:"started_before,omitempty"`
	FinishedAfter  *time.Time      `json:"finished_after,omitempty"`
	FinishedBefore *time.Time      `json:"finished_before,omitempty"`
	LongerThan     time.Duration   `json:"longer_than,omitempty"`
	ShorterThan    time.Duration   `json:"shorter_than,omitempty"`
}

// Attributes are the execution details used by host and time filters.
type Attributes struct {
	Hosts      []string
	StartedAt  time.Time
	FinishedAt time.Time
	Now        time.Time
}

// ParseTimestamp parses the timestamp formats accepted by time filters.
func ParseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}

// Empty reports whether no condition is set.
func (filter Filter) Empty() bool {
	return len(filter.NotStages) == 0 && len(filter.NotMatrices) == 0 && filter.Command == "" && len(filter.ExitCodes) == 0 && len(filter.FailureKinds) == 0 && len(filter.Diagnoses) == 0 && !filter.Changed && !filter.New && len(filter.Hosts) == 0 && filter.StartedAfter == nil && filter.StartedBefore == nil && filter.FinishedAfter == nil && filter.FinishedBefore == nil && filter.LongerThan == 0 && filter.ShorterThan == 0
}

// MatchesDefinition reports whether an ID satisfies changed/new conditions.
func (filter Filter) MatchesDefinition(id string) bool {
	if filter.Changed && !filter.ChangedIDs[id] {
		return false
	}
	if filter.New && !filter.NewIDs[id] {
		return false
	}
	return true
}

// MatchesAttributes reports whether execution attributes satisfy the filter.
// Missing attributes never match a positive condition.
func (filter Filter) MatchesAttributes(attributes Attributes) bool {
	if len(filter.Hosts) > 0 {
		matched := false
		for _, host := range attributes.Hosts {
			for _, pattern := range filter.Hosts {
				if ok, err := path.Match(pattern, host); err == nil && ok {
					matched = true
				}
			}
		}
		if !matched {
			return false
		}
	}
	if filter.StartedAfter != nil && (attributes.StartedAt.IsZero() || attributes.StartedAt.Before(*filter.StartedAfter)) {
		return false
	}
	if filter.StartedBefore != nil && (attributes.StartedAt.IsZero() || !attributes.StartedAt.Before(*filter.StartedBefore)) {
		return false
	}
	if filter.FinishedAfter != nil && (attributes.FinishedAt.IsZero() || attributes.FinishedAt.Before(*filter.FinishedAfter)) {
		return false
	}
	if filter.FinishedBefore != nil && (attributes.FinishedAt.IsZero() || !attributes.FinishedAt.Before(*filter.FinishedBefore)) {
		return false
	}
	if filter.LongerThan > 0 || filter.ShorterThan > 0 {
		end := attributes.FinishedAt
		if end.IsZero() {
			end = attributes.Now
		}
		if attributes.StartedAt.IsZero() || end.Before(attributes.StartedAt) {
			return false
		}
		duration := end.Sub(attributes.StartedAt)
		if filter.LongerThan > 0 && duration < filter.LongerThan {
			return false
		}
		if filter.ShorterThan > 0 && duration >= filter.ShorterThan {
			return false
		}
	}
	return true
}

// Job is what a selection needs to know about one job or array task.
type Job struct {
	ID         string
	Result     model.JobResult
	Finished   bool
	Attributes Attributes
	// Diagnosis reports whether the job's diagnosis matches any selector. It
	// is called only for a filter with diagnosis conditions; nil never matches.
	Diagnosis func(selectors []string) bool
}

// Selects reports whether job is selected by a result selection and every
// per-job condition of the filter. selection is a comma-separated list such as
// "failed,unfinished"; "" and "all" select every result. Definition
// conditions are checked separately with MatchesCommand, because they apply
// to a whole command.
func (filter Filter) Selects(selection string, job Job) bool {
	if selection != "" && selection != "all" && !model.ResultSelectionMatches(selection, job.Finished, job.Result.ExitCode) {
		return false
	}
	if (len(filter.ExitCodes) > 0 || len(filter.FailureKinds) > 0) && !filter.matchesResult(job.Result, job.Finished) {
		return false
	}
	if !filter.MatchesDefinition(job.ID) || !filter.MatchesAttributes(job.Attributes) {
		return false
	}
	if len(filter.Diagnoses) > 0 && (job.Diagnosis == nil || !job.Diagnosis(filter.Diagnoses)) {
		return false
	}
	return true
}

// SelectsArray reports whether an array job decided as a whole is selected:
// its aggregate result, finished when every task is and failed when any task
// is, matches selection, and some task satisfies every per-job condition.
func (filter Filter) SelectsArray(selection string, tasks []Job) bool {
	whole := Job{Finished: true}
	for _, task := range tasks {
		whole.Finished = whole.Finished && task.Finished
		if whole.Result.ExitCode == 0 {
			whole.Result.ExitCode = task.Result.ExitCode
		}
	}
	if !(Filter{}).Selects(selection, whole) {
		return false
	}
	return slices.ContainsFunc(tasks, func(task Job) bool { return filter.Selects("", task) })
}

// HasRunConditions reports whether the filter has a condition on a job's
// result or execution, which only a run records.
func (filter Filter) HasRunConditions() bool {
	return len(filter.ExitCodes) > 0 || len(filter.FailureKinds) > 0 || len(filter.Diagnoses) > 0 || len(filter.Hosts) > 0 || filter.StartedAfter != nil || filter.StartedBefore != nil || filter.FinishedAfter != nil || filter.FinishedBefore != nil || filter.LongerThan > 0 || filter.ShorterThan > 0
}

// matchesResult reports whether a finished job's result satisfies the value
// filters. An unfinished result never matches any exit-code or failure-kind
// filter, even when the zero value would otherwise satisfy one.
func (filter Filter) matchesResult(result model.JobResult, finished bool) bool {
	if !finished {
		return false
	}
	if len(filter.ExitCodes) > 0 {
		matched := false
		for _, exitCode := range filter.ExitCodes {
			if result.ExitCode == exitCode {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(filter.FailureKinds) > 0 {
		matched := false
		for _, kind := range model.FailureKinds(result) {
			if slices.Contains(filter.FailureKinds, kind) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// MatchesCommand reports whether command's definition passes the filter. A
// command without a stage or matrix passes the negated stage and matrix
// conditions.
func (filter Filter) MatchesCommand(command model.QueuedCommand) bool {
	if filter.Command != "" {
		re, err := regexp.Compile(filter.Command)
		if err != nil || !re.MatchString(strings.Join(command.Command, " ")) {
			return false
		}
	}
	if command.Stage != "" && slices.Contains(filter.NotStages, command.Stage) {
		return false
	}
	if command.Matrix != nil && slices.Contains(filter.NotMatrices, command.Matrix.BaseName) {
		return false
	}
	return true
}
