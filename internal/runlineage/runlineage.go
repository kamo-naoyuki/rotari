// Package runlineage compares two runs of a project: which jobs were added or
// removed, how each job's definition changed, and how its result moved, such
// as a failure that was fixed. It works on already loaded runs and does not
// read state files itself.
package runlineage

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
	Spec            model.JobSpec
	Status          string
	Origin          *model.JobOrigin
	DiagnosisStatus string
	Diagnoses       []string
	// Result is the job's resolved result once it has finished.
	Result model.JobResult
	// Final reports that Result will not change in this run: the run has
	// finished, or the job reached its final result with no retry left.
	Final bool
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
	// Sources is the run's recorded source revisions, nil when it recorded
	// none.
	Sources *model.RunSources
	// Notes are the notes on the run and its job attempts, oldest first.
	Notes []model.RunNote
}

// IsCarried reports whether a job's result came from its origin rather than
// an attempt executed in the current run. recorded reports that the run has
// recorded a result for the job (jobstatus.RecordedResults): while a run is
// active, a job it will still execute has an origin and no attempt too, but
// no recorded result.
func IsCarried(origin *model.JobOrigin, latestAttemptID string, blocked, recorded bool) bool {
	return recorded && origin != nil && !blocked && (latestAttemptID == "" || latestAttemptID == origin.AttemptID)
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
	Name       string `json:"name"`
	FromID     string `json:"from_job_id,omitempty"`
	ToID       string `json:"to_job_id,omitempty"`
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
	Transition string `json:"transition"`
	// FromCause and ToCause name each side's failure cause, as
	// FailureCause does; CauseChanged marks a still-failing job whose cause
	// differs between the runs.
	FromCause    string   `json:"from_cause,omitempty"`
	ToCause      string   `json:"to_cause,omitempty"`
	CauseChanged bool     `json:"cause_changed,omitempty"`
	Carried      bool     `json:"carried,omitempty"`
	Changes      []Change `json:"changes,omitempty"`
}

// Notable reports whether a comparison lists job by default: its result or
// its definition changed. `rotari lineage FROM TO` and rotari_compare_runs
// list only these unless asked for every job.
func (job JobDiff) Notable() bool {
	return job.Transition != TransitionUnchanged || len(job.Changes) > 0
}

// RunInfo identifies a compared run.
type RunInfo struct {
	ID      string                 `json:"run_id"`
	Name    string                 `json:"run_name,omitempty"`
	Elapsed string                 `json:"elapsed,omitempty"`
	Sources []model.SourceRevision `json:"sources,omitempty"`
	Notes   []model.RunNote        `json:"notes,omitempty"`
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
	// CauseChanged counts still-failing jobs whose cause changed.
	CauseChanged int `json:"cause_changed"`
}

// Result is the comparison of two runs.
type Result struct {
	From    RunInfo   `json:"from"`
	To      RunInfo   `json:"to"`
	Summary Summary   `json:"summary"`
	Jobs    []JobDiff `json:"jobs"`
	// Sources compares the code each run executed, per repository.
	Sources []SourceChange `json:"sources,omitempty"`
}

// GridResult compares three or more runs as a job-by-run result grid.
type GridResult struct {
	Runs []RunInfo `json:"runs"`
	Jobs []GridJob `json:"jobs"`
}

// GridJob contains one job's status in each selected run. DefinitionChanged
// marks columns whose definition differs from the previous occurrence.
type GridJob struct {
	Name              string   `json:"name"`
	Statuses          []string `json:"statuses"`
	DefinitionChanged []bool   `json:"definition_changed,omitempty"`
}

// CompareGrid compares three or more runs in the supplied order. Jobs are
// matched by name, or by ID for unnamed jobs.
func CompareGrid(runs []Run) GridResult {
	result := GridResult{Runs: make([]RunInfo, 0, len(runs)), Jobs: []GridJob{}}
	jobIndexes := make(map[string]int)
	originIndexes := make(map[string]int)
	lastJobs := make([]*Job, 0)
	lastRunIndexes := make([]int, 0)
	for _, run := range runs {
		result.Runs = append(result.Runs, runInfo(run))
	}
	for runIndex, run := range runs {
		for _, job := range run.Jobs {
			key := jobKey(job.Spec)
			jobIndex := -1
			if job.Origin != nil {
				if index, ok := originIndexes[job.Origin.RunID+"\x00"+job.Origin.JobID]; ok {
					jobIndex = index
				}
			}
			if jobIndex < 0 {
				if index, ok := jobIndexes[key]; ok {
					jobIndex = index
				}
			}
			exists := jobIndex >= 0
			if !exists {
				jobIndex = len(result.Jobs)
				jobIndexes[key] = jobIndex
				result.Jobs = append(result.Jobs, GridJob{
					Name: displayName(job.Spec), Statuses: make([]string, len(runs)), DefinitionChanged: make([]bool, len(runs)),
				})
				lastJobs = append(lastJobs, nil)
				lastRunIndexes = append(lastRunIndexes, -1)
			}
			gridJob := &result.Jobs[jobIndex]
			gridJob.Statuses[runIndex] = job.Status
			if runIndex > 0 && lastRunIndexes[jobIndex] == runIndex-1 && lastJobs[jobIndex] != nil && len(SpecChanges(lastJobs[jobIndex].Spec, job.Spec)) > 0 {
				gridJob.DefinitionChanged[runIndex] = true
			}
			gridJob.Name = displayName(job.Spec)
			jobCopy := job
			lastJobs[jobIndex] = &jobCopy
			lastRunIndexes[jobIndex] = runIndex
			jobIndexes[key] = jobIndex
			originIndexes[run.ID+"\x00"+job.Spec.ID] = jobIndex
		}
	}
	return result
}

// Compare compares from with to. Jobs are listed in to's order, followed by
// removed jobs in from's order.
func Compare(from, to Run) Result {
	result := Result{From: runInfo(from), To: runInfo(to), Jobs: []JobDiff{}, Sources: CompareSources(from.Sources, to.Sources)}
	fromByKey := make(map[string]Job, len(from.Jobs))
	fromByID := make(map[string]Job, len(from.Jobs))
	for _, job := range from.Jobs {
		fromByKey[jobKey(job.Spec)] = job
		fromByID[job.Spec.ID] = job
	}
	seen := make(map[string]bool, len(to.Jobs))
	for _, job := range to.Jobs {
		key := jobKey(job.Spec)
		old, matched := matchingJob(from, fromByKey, fromByID, job)
		if matched {
			seen[old.Spec.ID] = true
		} else if job.Origin == nil {
			seen[key] = true
		}
		diff := JobDiff{Name: displayName(job.Spec), ToID: job.Spec.ID, ToStatus: job.Status, ToCause: FailureCause(job), Carried: job.Carried}
		if matched {
			diff.FromID = old.Spec.ID
			diff.FromStatus = old.Status
			diff.FromCause = FailureCause(old)
			diff.Changes = SpecChanges(old.Spec, job.Spec)
			diff.Transition = transition(old.Status, job.Status)
			diff.CauseChanged = diff.Transition == TransitionStillFailing && diff.FromCause != diff.ToCause
		} else {
			diff.Transition = TransitionAdded
		}
		result.Jobs = append(result.Jobs, diff)
	}
	for _, job := range from.Jobs {
		if seen[job.Spec.ID] || seen[jobKey(job.Spec)] {
			continue
		}
		result.Jobs = append(result.Jobs, JobDiff{
			Name: displayName(job.Spec), FromID: job.Spec.ID, FromStatus: job.Status, FromCause: FailureCause(job), Transition: TransitionRemoved,
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
		if diff.CauseChanged {
			result.Summary.CauseChanged++
		}
	}
	return result
}

func matchingJob(from Run, byKey map[string]Job, byID map[string]Job, job Job) (Job, bool) {
	if job.Origin != nil {
		if job.Origin.RunID != from.ID {
			return Job{}, false
		}
		old, ok := byID[job.Origin.JobID]
		return old, ok
	}
	old, ok := byKey[jobKey(job.Spec)]
	return old, ok
}

func runInfo(run Run) RunInfo {
	info := RunInfo{ID: run.ID, Name: run.Name}
	if run.Sources != nil {
		info.Sources = run.Sources.Sources
	}
	info.Notes = run.Notes
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

// SpecChanges lists the definition fields that differ. Lists whose order does
// not matter report added and removed entries.
func SpecChanges(from, to model.JobSpec) []Change {
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
	list("output", from.Output, to.Output)
	list("artifact", from.Artifacts, to.Artifacts)
	list("error", from.Error, to.Error)
	scalar("log_mode", from.LogMode, to.LogMode)
	scalar("open_mode", from.OpenMode, to.OpenMode)
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

// RunSummary describes one run without comparing it to another run.
type RunSummary struct {
	Run       RunInfo          `json:"run"`
	Counts    Counts           `json:"counts"`
	Diagnoses []DiagnosisCount `json:"diagnoses,omitempty"`
	Failures  []FailureGroup   `json:"failures,omitempty"`
	Origins   []OriginCount    `json:"origins,omitempty"`
}

// DiagnosisCount counts jobs grouped by their saved diagnosis name or status.
type DiagnosisCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// OriginCount describes how many jobs came from one source run.
type OriginCount struct {
	RunID string `json:"run_id,omitempty"`
	Count int    `json:"count"`
}

// SummarizeOrigins groups jobs by their recorded source run. Jobs without an
// origin are reported under the explicit "new" category.
func SummarizeOrigins(run Run) []OriginCount {
	counts := make(map[string]int)
	for _, job := range run.Jobs {
		name := "new"
		if job.Origin != nil && job.Origin.RunID != "" {
			name = job.Origin.RunID
		}
		counts[name]++
	}
	result := make([]OriginCount, 0, len(counts))
	for runID, count := range counts {
		entry := OriginCount{Count: count}
		if runID != "new" {
			entry.RunID = runID
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].RunID, result[j].RunID
		if left == "" {
			left = "new"
		}
		if right == "" {
			right = "new"
		}
		return left < right
	})
	return result
}

// SummarizeDiagnoses groups failed-job diagnoses, preserving no-match and
// unavailable as explicit categories.
func SummarizeDiagnoses(run Run) []DiagnosisCount {
	counts := make(map[string]int)
	for _, job := range run.Jobs {
		if job.Status != StatusFailed && job.Status != StatusBlocked {
			continue
		}
		// A cause rotari recorded itself is the job's failure group; rules
		// did not analyze it, so it is not a diagnosis or a no-match.
		if cause, _, _ := classifyFailure(job); recordedCauseSuggestions[cause.kind] != "" {
			continue
		}
		if len(job.Diagnoses) > 0 {
			for _, diagnosis := range job.Diagnoses {
				counts[diagnosis]++
			}
			continue
		}
		if job.DiagnosisStatus != "" {
			counts[job.DiagnosisStatus]++
		}
	}
	result := make([]DiagnosisCount, 0, len(counts))
	for name, count := range counts {
		result = append(result, DiagnosisCount{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// LineageEntry describes one run in a project's run sequence. Changes
// compares it with the run before it and is nil for the first run.
type LineageEntry struct {
	Run     RunInfo  `json:"run"`
	Counts  Counts   `json:"counts"`
	Changes *Summary `json:"changes_from_previous,omitempty"`
	// CodeChange is whether the code changed since the run before, combined
	// over repositories by CombineSourceChanges; empty for the first run and
	// when neither run recorded sources.
	CodeChange string `json:"code_change_from_previous,omitempty"`
}

// Lineage describes runs, given oldest first, as the version history of an
// experiment: each run's result counts and what changed since the run
// before it.
func Lineage(runs []Run) []LineageEntry {
	entries := make([]LineageEntry, 0, len(runs))
	for index, run := range runs {
		entry := LineageEntry{Run: runInfo(run), Counts: Summarize(run)}
		if index > 0 {
			comparison := Compare(runs[index-1], run)
			entry.Changes = &comparison.Summary
			entry.CodeChange = CombineSourceChanges(comparison.Sources)
		}
		entries = append(entries, entry)
	}
	return entries
}

// Summarize tallies one run's jobs by resolved status.
func Summarize(run Run) Counts {
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
