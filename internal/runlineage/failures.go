package runlineage

import (
	"slices"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// FailureKindDiagnosis marks a failure group whose cause is a saved rule
// diagnosis. Other groups use the model.FailureKind* values.
const FailureKindDiagnosis = "diagnosis"

// FailureGroup is the failed and blocked jobs of a run that share one cause.
type FailureGroup struct {
	// Kind is FailureKindDiagnosis or a model.FailureKind* value.
	Kind string `json:"kind"`
	// Cause names the diagnosis rule for FailureKindDiagnosis, and repeats
	// Kind otherwise.
	Cause      string          `json:"cause"`
	Suggestion string          `json:"suggestion,omitempty"`
	Count      int             `json:"count"`
	Carried    int             `json:"carried,omitempty"`
	ExitCodes  []int           `json:"exit_codes"`
	Example    FailureExample  `json:"example"`
	Jobs       []FailureMember `json:"jobs"`
	// JobsOmitted counts the members a presentation left out of Jobs; see
	// LimitMembers.
	JobsOmitted int `json:"jobs_omitted,omitempty"`
}

// LimitMembers keeps the first limit members of each group in Jobs and
// counts the rest in JobsOmitted. Count, ExitCodes, and Example still
// describe the whole group.
func LimitMembers(groups []FailureGroup, limit int) {
	for index := range groups {
		if rest := len(groups[index].Jobs) - limit; rest > 0 {
			groups[index].Jobs = groups[index].Jobs[:limit]
			groups[index].JobsOmitted = rest
		}
	}
}

// FailureExample is the group's first job, with the line that shows its
// cause.
type FailureExample struct {
	JobID     string `json:"job_id"`
	AttemptID string `json:"attempt_id,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
}

// FailureMember is one job of a failure group.
type FailureMember struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ArrayTaskID *int   `json:"array_task_id,omitempty"`
	Carried     bool   `json:"carried,omitempty"`
}

// FailureGroups groups a run's failed and blocked jobs by cause, largest
// group first and otherwise in job order. Each job is classified on its own
// result, so the tasks of an array and the members of a matrix fall into
// their own causes.
func FailureGroups(run Run) []FailureGroup {
	groups := make([]FailureGroup, 0)
	index := make(map[failureCause]int)
	for _, job := range run.Jobs {
		if !failed(job) {
			continue
		}
		cause, suggestion, evidence := classifyFailure(job)
		position, ok := index[cause]
		if !ok {
			position = len(groups)
			index[cause] = position
			groups = append(groups, FailureGroup{
				Kind: cause.kind, Cause: cause.name, Suggestion: suggestion, ExitCodes: []int{},
				Example: FailureExample{JobID: job.Spec.ID, AttemptID: job.Result.AttemptID, Evidence: evidence},
			})
		}
		group := &groups[position]
		group.Count++
		if job.Carried {
			group.Carried++
		}
		if !slices.Contains(group.ExitCodes, job.Result.ExitCode) {
			group.ExitCodes = append(group.ExitCodes, job.Result.ExitCode)
		}
		group.Jobs = append(group.Jobs, FailureMember{ID: job.Spec.ID, Name: job.Spec.Name, ArrayTaskID: job.Spec.ArrayTaskID, Carried: job.Carried})
	}
	for index := range groups {
		slices.Sort(groups[index].ExitCodes)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	return groups
}

// FailureCause names the cause FailureGroups groups a failed or blocked job
// under, and is empty for any other job.
func FailureCause(job Job) string {
	if !failed(job) {
		return ""
	}
	cause, _, _ := classifyFailure(job)
	return cause.name
}

// failed reports whether job counts as a failure for grouping.
func failed(job Job) bool {
	return job.Status == StatusFailed || job.Status == StatusBlocked
}

type failureCause struct {
	kind string
	name string
}

// classifyFailure returns a failed job's cause, the suggestion for it, and
// the line that shows it. Causes rotari records itself (blocked, cancelled,
// timeout) come first because they are certain; then the latest saved rule
// diagnosis; then the remaining failure kinds.
func classifyFailure(job Job) (failureCause, string, string) {
	result := job.Result
	kinds := model.FailureKinds(result)
	for _, kind := range []string{model.FailureKindBlocked, model.FailureKindCancelled, model.FailureKindTimeout} {
		if slices.Contains(kinds, kind) {
			return failureCause{kind: kind, name: kind}, "", result.Error
		}
	}
	if len(result.Diagnoses) > 0 {
		diagnosis := result.Diagnoses[0]
		return failureCause{kind: FailureKindDiagnosis, name: diagnosis.Name}, diagnosis.Suggestion, diagnosis.Evidence
	}
	kind := model.FailureKindError
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	return failureCause{kind: kind, name: kind}, "", result.Error
}
