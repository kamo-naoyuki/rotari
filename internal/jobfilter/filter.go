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

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Filter narrows a job selection. A negated list excludes a job that
// matches any of its values.
type Filter struct {
	NotStages      []string      `json:"not_stages,omitempty"`
	NotMatrices    []string      `json:"not_matrices,omitempty"`
	Command        string        `json:"command,omitempty"`
	ExitCodes      []int         `json:"exit_codes,omitempty"`
	FailureKinds   []string      `json:"failure_kinds,omitempty"`
	Hosts          []string      `json:"hosts,omitempty"`
	StartedAfter   *time.Time    `json:"started_after,omitempty"`
	StartedBefore  *time.Time    `json:"started_before,omitempty"`
	FinishedAfter  *time.Time    `json:"finished_after,omitempty"`
	FinishedBefore *time.Time    `json:"finished_before,omitempty"`
	LongerThan     time.Duration `json:"longer_than,omitempty"`
	ShorterThan    time.Duration `json:"shorter_than,omitempty"`
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
	return len(filter.NotStages) == 0 && len(filter.NotMatrices) == 0 && filter.Command == "" && len(filter.ExitCodes) == 0 && len(filter.FailureKinds) == 0 && len(filter.Hosts) == 0 && filter.StartedAfter == nil && filter.StartedBefore == nil && filter.FinishedAfter == nil && filter.FinishedBefore == nil && filter.LongerThan == 0 && filter.ShorterThan == 0
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

// MatchesResult reports whether a finished job's result satisfies the value
// filters. An unfinished result never matches any exit-code or failure-kind
// filter, even when the zero value would otherwise satisfy one.
func (filter Filter) MatchesResult(result model.JobResult, finished bool) bool {
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
		for _, kind := range jobstatus.FailureKinds(result) {
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
