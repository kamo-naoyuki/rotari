package run

import (
	"errors"
	"fmt"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// ErrNoReferenceRun reports that a command without an origin needs a
// reference run's result, but none was given.
var ErrNoReferenceRun = errors.New("no previous run")

// OriginResults reads the source results that queued commands' origins name.
// Implementations own the filesystem access and error wording for loading.
type OriginResults interface {
	// RunResults returns a run's summary results by job ID.
	RunResults(runID string) (map[string]model.JobResult, error)
	// AttemptResult returns the result recorded by the attempt named by
	// origin.AttemptID.
	AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error)
	// Origin describes a result carried from runID for a job that has no
	// origin of its own.
	Origin(runID, jobID string, result model.JobResult) *model.JobOrigin
}

// OriginAttributes optionally supplies execution attributes for an origin.
// Older in-memory sources may omit it; filters requiring these attributes then
// simply do not match.
type OriginAttributes interface {
	Attributes(origin model.JobOrigin) (jobfilter.Attributes, error)
}

// OriginDiagnoses optionally reports whether an origin's result, diagnosed
// with the current rules from its latest attempt's log, matches any of
// selectors. Without it a diagnosis filter matches no job.
type OriginDiagnoses interface {
	DiagnosisMatches(origin model.JobOrigin, result model.JobResult, selectors []string) (bool, error)
}

// PlanRerun decides which of the queue's jobs a new run executes.
//
// Without a selection every command executes. With a selection only
// matching jobs execute, together with every job that depends on an
// executing job; a job that does not match but has a finished result
// carries that result forward (Origin recorded, no re-execution), and one
// without a result is left untouched. Each command's result comes from its
// origin; a command without one falls back to referenceRunID, and planning
// fails with ErrNoReferenceRun when that is empty. Callers resolve the
// reference run before the new run is recorded, so it is never the run being
// planned. A job's marked status replaces its result's; see model.MarkResult.
//
// When partialArray is true, array jobs are evaluated per task, so only
// matching tasks execute and the rest carry their own results. When false,
// the whole array executes if its aggregate result matches.
//
// A scope, when set, narrows a result selection to one stage or matrix, and
// filter narrows it further: jobs outside them do not execute and carry
// their results like non-matching jobs.
// jobIDs are the whole selection, and only with selection "job-id": jobs
// named directly are not combined with a result selection or a filter.
func PlanRerun(queue model.Queue, selection string, jobIDs []string, scope model.CommandSelector, filter jobfilter.Filter, referenceRunID string, partialArray bool, source OriginResults) (Plan, error) {
	if len(jobIDs) > 0 && selection != "job-id" {
		return Plan{}, fmt.Errorf("job IDs cannot be combined with result selection %q", selection)
	}
	if len(jobIDs) > 0 && !filter.Empty() {
		return Plan{}, errors.New("job IDs cannot be combined with a job filter")
	}
	if selection == "" && scope.Kinds() == 0 && filter.Empty() {
		plan := Plan{Execute: make(map[string]bool, len(queue.Commands))}
		for _, command := range queue.Commands {
			plan.Execute[command.ID] = true
		}
		return plan, nil
	}
	inScope := filter.MatchesCommand
	if scope.Kinds() > 0 {
		if _, err := model.SelectCommands(queue.Commands, scope); err != nil {
			return Plan{}, err
		}
		inScope = func(command model.QueuedCommand) bool {
			return scope.Matches(command) && filter.MatchesCommand(command)
		}
	}
	plan, err := planByOrigin(queue, selection, jobIDs, inScope, filter, referenceRunID, partialArray, source)
	if err != nil {
		return Plan{}, err
	}
	expandDownstream(queue, &plan)
	return plan, nil
}

func forceExecution(jobID string, plan *Plan) {
	plan.Execute[jobID] = true
	delete(plan.CarriedResults, jobID)
	delete(plan.CarriedOrigins, jobID)
}

// expandDownstream executes every job that depends on an executing job,
// directly or through other such jobs, with DependsOn or DependsOnFinished.
// Its recorded result was computed from the prerequisite's earlier output,
// so a result filter alone would carry forward a result the new run no
// longer supports.
func expandDownstream(queue model.Queue, plan *Plan) {
	jobs := model.QueueToJobs(queue.Commands)
	executing := func(job model.JobSpec) bool {
		return plan.Execute[job.ID] || (job.ArrayGroup != "" && plan.Execute[job.ArrayGroup])
	}
	executingNames := make(map[string]bool)
	for _, job := range jobs {
		if executing(job) && job.Name != "" {
			executingNames[job.Name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, job := range jobs {
			if executing(job) || !dependsOnAny(job, executingNames) {
				continue
			}
			forceExecution(job.ID, plan)
			if job.Name != "" {
				executingNames[job.Name] = true
			}
			changed = true
		}
	}
}

func dependsOnAny(job model.JobSpec, names map[string]bool) bool {
	return anyName(job.AllDependencies(), names)
}

func anyName(candidates []string, names map[string]bool) bool {
	for _, candidate := range candidates {
		if names[candidate] {
			return true
		}
	}
	return false
}

// originResolver looks up the result each queued job's origin names, caching
// run summaries.
type originResolver struct {
	source        OriginResults
	fallbackRunID string
	resultsByRun  map[string]map[string]model.JobResult
}

func (resolver *originResolver) result(origin *model.JobOrigin, fallbackJobID string) (model.JobResult, bool, error) {
	if origin == nil || origin.RunID == "" || origin.JobID == "" {
		if resolver.fallbackRunID == "" {
			return model.JobResult{}, false, ErrNoReferenceRun
		}
		origin = &model.JobOrigin{RunID: resolver.fallbackRunID, JobID: fallbackJobID}
	}
	results, ok := resolver.resultsByRun[origin.RunID]
	if !ok {
		loaded, err := resolver.source.RunResults(origin.RunID)
		if err != nil {
			return model.JobResult{}, false, fmt.Errorf("failed to load run summary for origin run %q: %w", origin.RunID, err)
		}
		results = loaded
		resolver.resultsByRun[origin.RunID] = results
	}
	result, ok := results[origin.JobID]
	if origin.AttemptID != "" && (!ok || result.AttemptID != origin.AttemptID) {
		return resolver.source.AttemptResult(*origin)
	}
	return result, ok, nil
}

func (resolver *originResolver) fallbackOrigin(jobID string, result model.JobResult) *model.JobOrigin {
	return resolver.source.Origin(resolver.fallbackRunID, jobID, result)
}

func (resolver *originResolver) attributes(origin *model.JobOrigin, jobID string) (jobfilter.Attributes, error) {
	provider, ok := resolver.source.(OriginAttributes)
	if !ok {
		return jobfilter.Attributes{}, nil
	}
	if origin == nil {
		origin = &model.JobOrigin{RunID: resolver.fallbackRunID, JobID: jobID}
	}
	return provider.Attributes(*origin)
}

func (resolver *originResolver) diagnosisMatches(filter jobfilter.Filter, result model.JobResult, origin *model.JobOrigin, jobID string) bool {
	if len(filter.Diagnoses) == 0 {
		return true
	}
	provider, ok := resolver.source.(OriginDiagnoses)
	if !ok {
		return false
	}
	if origin == nil {
		origin = &model.JobOrigin{RunID: resolver.fallbackRunID, JobID: jobID}
	}
	matched, err := provider.DiagnosisMatches(*origin, result, filter.Diagnoses)
	return err == nil && matched
}

// jobResult returns the result of a job or task of command: the one its
// origin or the reference run records, as its marked status leaves it.
func (resolver *originResolver) jobResult(command model.QueuedCommand, origin *model.JobOrigin, id string) (model.JobResult, bool, error) {
	marked := command.MarkedStatusOf(id)
	if marked == model.StatusUnfinished {
		return model.JobResult{}, false, nil
	}
	result, finished, err := resolver.result(origin, id)
	if err != nil {
		return model.JobResult{}, false, err
	}
	return model.MarkResult(id, result, finished, marked)
}

func (resolver *originResolver) commandResult(command model.QueuedCommand) (model.JobResult, bool, error) {
	if command.Array == nil {
		return resolver.jobResult(command, command.Origin, command.ID)
	}
	results := make(map[string]model.JobResult)
	for _, task := range model.ArrayTaskIDs(command.Array) {
		id := taskID(command.ID, task)
		result, finished, err := resolver.jobResult(command, TaskOrigin(command, task), id)
		if err != nil {
			return model.JobResult{}, false, err
		}
		if finished {
			results[id] = result
		}
	}
	result, finished := model.AggregatedJobResult(command.ID, command.Array, results)
	return result, finished, nil
}

// planByOrigin applies a selection to the result recorded by each command's
// origin. Commands added directly to the queue have no origin and fall back
// to the reference run. Commands outside inScope never match the selection.
func planByOrigin(queue model.Queue, selection string, jobIDs []string, inScope func(model.QueuedCommand) bool, filter jobfilter.Filter, referenceRunID string, partialArray bool, source OriginResults) (Plan, error) {
	hasResultFilters := len(filter.ExitCodes) > 0 || len(filter.FailureKinds) > 0
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	plan := Plan{
		Execute:        make(map[string]bool, len(queue.Commands)),
		CarriedResults: make(map[string]model.JobResult),
		CarriedOrigins: make(map[string]*model.JobOrigin),
	}
	resolver := &originResolver{source: source, fallbackRunID: referenceRunID, resultsByRun: make(map[string]map[string]model.JobResult)}
	for _, command := range queue.Commands {
		scoped := inScope(command)
		// A requested job executes whole; a requested task of an array
		// executes alone.
		requestedTask := false
		if command.Array != nil {
			for _, task := range model.ArrayTaskIDs(command.Array) {
				requestedTask = requestedTask || requested[taskID(command.ID, task)]
			}
		}
		matchTasks := partialArray && selection != "job-id"
		if command.Array != nil && (matchTasks || requestedTask) && !requested[command.ID] {
			for _, task := range model.ArrayTaskIDs(command.Array) {
				id := taskID(command.ID, task)
				origin := TaskOrigin(command, task)
				result, finished, err := resolver.jobResult(command, origin, id)
				if err != nil {
					return Plan{}, err
				}
				attributes, err := resolver.attributes(origin, id)
				if err != nil {
					return Plan{}, err
				}
				if requested[id] {
					delete(requested, id)
					plan.Execute[id] = true
					continue
				}
				matchSelection := false
				switch selection {
				case "":
					matchSelection = !hasResultFilters || filter.MatchesResult(result, finished)
				default:
					matchSelection = model.ResultSelectionMatches(selection, finished, result.ExitCode)
				}
				if hasResultFilters && !filter.MatchesResult(result, finished) {
					matchSelection = false
				}
				if matchTasks && scoped && matchSelection && filter.MatchesDefinition(id) && filter.MatchesAttributes(attributes) && resolver.diagnosisMatches(filter, result, origin, id) {
					plan.Execute[id] = true
					continue
				}
				if finished {
					result.ID = id
					plan.CarriedResults[id] = result
					if origin == nil {
						plan.CarriedOrigins[id] = resolver.fallbackOrigin(id, result)
					}
				}
			}
			continue
		}

		result, finished, err := resolver.commandResult(command)
		if err != nil {
			return Plan{}, err
		}
		attributes, err := resolver.attributes(command.Origin, command.ID)
		if err != nil {
			return Plan{}, err
		}
		include := requested[command.ID]
		if selection != "job-id" {
			matchSelection := false
			switch selection {
			case "":
				matchSelection = !hasResultFilters || filter.MatchesResult(result, finished)
			default:
				matchSelection = model.ResultSelectionMatches(selection, finished, result.ExitCode)
			}
			include = include || (scoped && matchSelection && filter.MatchesDefinition(command.ID) && filter.MatchesAttributes(attributes) && resolver.diagnosisMatches(filter, result, command.Origin, command.ID))
		}
		if hasResultFilters && !filter.MatchesResult(result, finished) {
			include = false
		}
		delete(requested, command.ID)
		if include {
			plan.Execute[command.ID] = true
			continue
		}
		if !finished {
			continue
		}
		if command.Array == nil {
			result.ID = command.ID
			plan.CarriedResults[command.ID] = result
			if command.Origin == nil {
				plan.CarriedOrigins[command.ID] = resolver.fallbackOrigin(command.ID, result)
			}
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			id := taskID(command.ID, task)
			origin := TaskOrigin(command, task)
			taskResult, taskFinished, err := resolver.jobResult(command, origin, id)
			if err != nil {
				return Plan{}, err
			}
			if taskFinished {
				taskResult.ID = id
				plan.CarriedResults[id] = taskResult
				if origin == nil {
					plan.CarriedOrigins[id] = resolver.fallbackOrigin(id, taskResult)
				}
			}
		}
		if command.Origin == nil {
			plan.CarriedOrigins[command.ID] = resolver.fallbackOrigin(command.ID, result)
		}
	}
	if len(requested) > 0 {
		missing := make([]string, 0, len(requested))
		for jobID := range requested {
			missing = append(missing, jobID)
		}
		sort.Strings(missing)
		return Plan{}, fmt.Errorf("job IDs not found in queue: %s", joinStrings(missing, ", "))
	}
	return plan, nil
}

// TaskOrigin returns the origin of one task of an array command: its own
// task origin, or the command's origin narrowed to the task.
func TaskOrigin(command model.QueuedCommand, task int) *model.JobOrigin {
	if origin := command.TaskOrigins[taskID(command.ID, task)]; origin != nil {
		return origin
	}
	if command.Origin == nil {
		return nil
	}
	origin := *command.Origin
	origin.JobID = taskID(command.Origin.JobID, task)
	return &origin
}

func taskID(commandID string, task int) string {
	return fmt.Sprintf("%s-%d", commandID, task)
}
