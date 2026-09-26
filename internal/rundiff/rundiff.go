// Package rundiff compares two runs of a project: which jobs were added or
// removed, how each job's definition changed, and how its result moved, such
// as a failure that was fixed. It works on already loaded runs and does not
// read state files itself.
package rundiff

import (
	"sort"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Job status values used by Job.Status and JobDiff.
const (
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusBlocked    = "blocked"
	StatusUnfinished = "unfinished"
)

// Transition values describe how a job's result moved between the runs.
const (
	TransitionFixed        = "fixed"
	TransitionStillFailing = "still_failing"
	TransitionNewlyFailing = "newly_failing"
	TransitionAdded        = "added"
	TransitionRemoved      = "removed"
	TransitionUnchanged    = "unchanged"
	// TransitionOther covers moves involving an unfinished job.
	TransitionOther = "other"
)

// Job is one expanded job of a run with its resolved result.
type Job struct {
	Spec   model.JobSpec
	Status string
	// Carried reports that the run reused an earlier result instead of
	// executing the job.
	Carried bool
}

// Run is a loaded run to compare.
type Run struct {
	ID         string
	Name       string
	StartedAt  string
	FinishedAt string
	Jobs       []Job
}

// Change is one changed field of a job definition.
type Change struct {
	Field string   `json:"field"`
	From  string   `json:"from,omitempty"`
	To    string   `json:"to,omitempty"`
	Added []string `json:"added,omitempty"`
	// Removed lists entries of a list field that only the older run has.
	Removed []string `json:"removed,omitempty"`
}

// JobDiff compares one job across the runs. A job is matched by name, or by
// job ID when it has no name.
type JobDiff struct {
	Name       string   `json:"name"`
	FromID     string   `json:"from_job_id,omitempty"`
	ToID       string   `json:"to_job_id,omitempty"`
	FromStatus string   `json:"from_status,omitempty"`
	ToStatus   string   `json:"to_status,omitempty"`
	Transition string   `json:"transition"`
	Carried    bool     `json:"carried,omitempty"`
	Changes    []Change `json:"changes,omitempty"`
}

// RunInfo identifies a compared run.
type RunInfo struct {
	ID      string `json:"run_id"`
	Name    string `json:"run_name,omitempty"`
	Elapsed string `json:"elapsed,omitempty"`
}

// Summary counts the transitions and changes.
type Summary struct {
	Fixed        int `json:"fixed"`
	StillFailing int `json:"still_failing"`
	NewlyFailing int `json:"newly_failing"`
	Added        int `json:"added"`
	Removed      int `json:"removed"`
	Changed      int `json:"changed"`
	Carried      int `json:"carried"`
}

// Result is the comparison of two runs.
type Result struct {
	From    RunInfo   `json:"from"`
	To      RunInfo   `json:"to"`
	Summary Summary   `json:"summary"`
	Jobs    []JobDiff `json:"jobs"`
}

// Compare compares from with to. Jobs are listed in to's order, followed by
// removed jobs in from's order.
func Compare(from, to Run) Result {
	result := Result{From: runInfo(from), To: runInfo(to), Jobs: []JobDiff{}}
	fromByKey := make(map[string]Job, len(from.Jobs))
	for _, job := range from.Jobs {
		fromByKey[jobKey(job.Spec)] = job
	}
	seen := make(map[string]bool, len(to.Jobs))
	for _, job := range to.Jobs {
		key := jobKey(job.Spec)
		seen[key] = true
		diff := JobDiff{Name: displayName(job.Spec), ToID: job.Spec.ID, ToStatus: job.Status, Carried: job.Carried}
		if old, ok := fromByKey[key]; ok {
			diff.FromID = old.Spec.ID
			diff.FromStatus = old.Status
			diff.Changes = specChanges(old.Spec, job.Spec)
			diff.Transition = transition(old.Status, job.Status)
		} else {
			diff.Transition = TransitionAdded
		}
		result.Jobs = append(result.Jobs, diff)
	}
	for _, job := range from.Jobs {
		if seen[jobKey(job.Spec)] {
			continue
		}
		result.Jobs = append(result.Jobs, JobDiff{
			Name: displayName(job.Spec), FromID: job.Spec.ID, FromStatus: job.Status, Transition: TransitionRemoved,
		})
	}
	for _, diff := range result.Jobs {
		switch diff.Transition {
		case TransitionFixed:
			result.Summary.Fixed++
		case TransitionStillFailing:
			result.Summary.StillFailing++
		case TransitionNewlyFailing:
			result.Summary.NewlyFailing++
		case TransitionAdded:
			result.Summary.Added++
		case TransitionRemoved:
			result.Summary.Removed++
		}
		if len(diff.Changes) > 0 {
			result.Summary.Changed++
		}
		if diff.Carried {
			result.Summary.Carried++
		}
	}
	return result
}

func runInfo(run Run) RunInfo {
	info := RunInfo{ID: run.ID, Name: run.Name}
	started, startErr := time.Parse(time.RFC3339, run.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339, run.FinishedAt)
	if startErr == nil && finishErr == nil && !finished.Before(started) {
		info.Elapsed = finished.Sub(started).String()
	}
	return info
}

func jobKey(spec model.JobSpec) string {
	if spec.Name != "" {
		return "name:" + spec.Name
	}
	return "id:" + spec.ID
}

func displayName(spec model.JobSpec) string {
	if spec.Name != "" {
		return spec.Name
	}
	return spec.ID
}

func failing(status string) bool {
	return status == StatusFailed || status == StatusBlocked
}

func transition(from, to string) string {
	switch {
	case failing(from) && to == StatusSuccess:
		return TransitionFixed
	case failing(from) && failing(to):
		return TransitionStillFailing
	case from == StatusSuccess && failing(to):
		return TransitionNewlyFailing
	case from == to:
		return TransitionUnchanged
	default:
		return TransitionOther
	}
}

// specChanges lists the definition fields that differ. Lists whose order
// does not matter report added and removed entries.
func specChanges(from, to model.JobSpec) []Change {
	var changes []Change
	scalar := func(field, left, right string) {
		if left != right {
			changes = append(changes, Change{Field: field, From: left, To: right})
		}
	}
	list := func(field string, left, right []string) {
		added, removed := setDifference(right, left), setDifference(left, right)
		if len(added) > 0 || len(removed) > 0 {
			changes = append(changes, Change{Field: field, Added: added, Removed: removed})
		}
	}
	scalar("command", shellJoin(from.Command), shellJoin(to.Command))
	scalar("executor", from.Executor, to.Executor)
	if strings.Join(from.ExecutorOptions, "\x00") != strings.Join(to.ExecutorOptions, "\x00") {
		changes = append(changes, Change{Field: "executor_options", From: shellJoin(from.ExecutorOptions), To: shellJoin(to.ExecutorOptions)})
	}
	list("environment", from.Environment, to.Environment)
	scalar("working_directory", from.WorkingDirectory, to.WorkingDirectory)
	scalar("stage", from.Stage, to.Stage)
	list("depends_on", from.DependsOn, to.DependsOn)
	list("depends_on_finished", from.DependsOnFinished, to.DependsOnFinished)
	scalar("timeout", from.Timeout, to.Timeout)
	scalar("retry", model.FormatRetryPolicy(from), model.FormatRetryPolicy(to))
	return changes
}

// shellJoin joins arguments for display, single-quoting those that a shell
// would split or expand, so "sh -c 'exit 1'" stays readable.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		if arg != "" && strings.IndexFunc(arg, needsQuote) < 0 {
			quoted[index] = arg
			continue
		}
		quoted[index] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

func needsQuote(r rune) bool {
	return !(r == '-' || r == '_' || r == '.' || r == '/' || r == '=' || r == ':' || r == ',' || r == '+' || r == '@' || r == '%' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
}

func setDifference(left, right []string) []string {
	present := make(map[string]bool, len(right))
	for _, value := range right {
		present[value] = true
	}
	var difference []string
	for _, value := range left {
		if !present[value] {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
}

// Counts tallies a run's jobs by status.
type Counts struct {
	Jobs       int `json:"jobs"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Blocked    int `json:"blocked"`
	Unfinished int `json:"unfinished"`
}

// LineageEntry describes one run in a project's run sequence. Changes
// compares it with the run before it and is nil for the first run.
type LineageEntry struct {
	Run     RunInfo  `json:"run"`
	Counts  Counts   `json:"counts"`
	Changes *Summary `json:"changes_from_previous,omitempty"`
}

// Lineage describes runs, given oldest first, as the version history of an
// experiment: each run's result counts and what changed since the run
// before it.
func Lineage(runs []Run) []LineageEntry {
	entries := make([]LineageEntry, 0, len(runs))
	for index, run := range runs {
		entry := LineageEntry{Run: runInfo(run), Counts: countJobs(run)}
		if index > 0 {
			summary := Compare(runs[index-1], run).Summary
			entry.Changes = &summary
		}
		entries = append(entries, entry)
	}
	return entries
}

func countJobs(run Run) Counts {
	counts := Counts{Jobs: len(run.Jobs)}
	for _, job := range run.Jobs {
		switch job.Status {
		case StatusSuccess:
			counts.Succeeded++
		case StatusFailed:
			counts.Failed++
		case StatusBlocked:
			counts.Blocked++
		default:
			counts.Unfinished++
		}
	}
	return counts
}
