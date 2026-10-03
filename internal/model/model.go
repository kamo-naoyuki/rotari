package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StateVersion is the format version of queue.json, commands.json, and
// summary.json. Files written before versioning have no version (0) and are
// read as version 1. Bump it only for a change that older readers would
// misread, such as a renamed, removed, or reinterpreted field, and keep
// decoding every older version; adding an optional field does not need a new
// version.
const StateVersion = 2

type Queue struct {
	// StateVersion is set by the state package when the queue is written.
	StateVersion           int             `json:"state_version,omitempty"`
	DefaultExecutor        string          `json:"default_executor,omitempty"`
	DefaultExecutorOptions []string        `json:"default_executor_options,omitempty"`
	Commands               []QueuedCommand `json:"commands"`
}

// OriginOf returns where jobID's carried or marked result came from: the
// whole command's Origin, or an array task's entry in TaskOrigins. It returns
// nil for a job that executes in this run.
func (queue Queue) OriginOf(jobID string) *JobOrigin {
	for _, command := range queue.Commands {
		if command.ID == jobID {
			return command.Origin
		}
		if origin, ok := command.TaskOrigins[jobID]; ok {
			return origin
		}
	}
	return nil
}

type QueuedCommand struct {
	ID               string   `json:"id"`
	Command          []string `json:"command"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	Executor         string   `json:"executor,omitempty"`
	ExecutorOptions  []string `json:"executor_options,omitempty"`
	Environment      []string `json:"environment,omitempty"`
	Output           []string `json:"output,omitempty"`
	Error            []string `json:"error,omitempty"`
	LogMode          string   `json:"log_mode,omitempty"`
	OpenMode         string   `json:"open_mode,omitempty"`
	Name             string   `json:"name,omitempty"`
	Stage            string   `json:"stage,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	// DependsOnFinished names prerequisites that must finish, whatever their
	// result, before the job starts (Slurm's afterany).
	DependsOnFinished []string `json:"depends_on_finished,omitempty"`
	// Timeout limits how long the job may run once it starts, as a Go
	// duration such as "2h"; empty means no limit.
	Timeout string `json:"timeout,omitempty"`
	// Retry overrides the run's --retry limit for this job when set; 0
	// disables retries.
	Retry *int `json:"retry,omitempty"`
	// RetryDelay, RetryBackoff, and RetryMaxDelay space out the job's retries;
	// see JobSpec.RetryDelayFor.
	RetryDelay    string                `json:"retry_delay,omitempty"`
	RetryBackoff  float64               `json:"retry_backoff,omitempty"`
	RetryMaxDelay string                `json:"retry_max_delay,omitempty"`
	Origin        *JobOrigin            `json:"origin,omitempty"`
	Array         *ArraySpec            `json:"array,omitempty"`
	TaskOrigins   map[string]*JobOrigin `json:"task_origins,omitempty"`
	Matrix        *MatrixSpec           `json:"matrix,omitempty"`
	// MarkedStatus and TaskMarkedStatus hold a status the user gave the
	// command, or tasks of it, with change --status or a workflow manifest.
	// A filtered run reads it in place of the recorded result's status; see
	// MarkResult.
	MarkedStatus     string            `json:"marked_status,omitempty"`
	TaskMarkedStatus map[string]string `json:"task_marked_status,omitempty"`
}

type ArraySpec struct {
	First int   `json:"first"`
	Last  int   `json:"last"`
	Tasks []int `json:"tasks,omitempty"`
}

type MatrixDimension struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

type MatrixValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type MatrixExclusion struct {
	Values []MatrixValue `json:"values"`
}

type MatrixSpec struct {
	GroupID         string            `json:"group_id"`
	Dimensions      []MatrixDimension `json:"dimensions"`
	Values          []MatrixValue     `json:"values"`
	Exclusions      []MatrixExclusion `json:"exclusions,omitempty"`
	BaseName        string            `json:"base_name,omitempty"`
	BaseEnvironment []string          `json:"base_environment,omitempty"`
}

func ParseMatrixDimension(value string) (MatrixDimension, error) {
	name, valuesText, ok := strings.Cut(value, "=")
	if !ok || !ValidEnvironmentName(name) {
		return MatrixDimension{}, fmt.Errorf("want KEY=VALUE[,VALUE...]")
	}
	parts := strings.Split(valuesText, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		if part == "" {
			return MatrixDimension{}, fmt.Errorf("matrix %q has an empty value", name)
		}
		if seen[part] {
			return MatrixDimension{}, fmt.Errorf("matrix %q has duplicate value %q", name, part)
		}
		seen[part] = true
		values = append(values, part)
	}
	return MatrixDimension{Name: name, Values: values}, nil
}

func ExpandMatrix(dimensions []MatrixDimension) [][]MatrixValue {
	if len(dimensions) == 0 {
		return nil
	}
	combinations := [][]MatrixValue{{}}
	for _, dimension := range dimensions {
		next := make([][]MatrixValue, 0, len(combinations)*len(dimension.Values))
		for _, combination := range combinations {
			for _, value := range dimension.Values {
				values := append([]MatrixValue(nil), combination...)
				values = append(values, MatrixValue{Name: dimension.Name, Value: value})
				next = append(next, values)
			}
		}
		combinations = next
	}
	return combinations
}

// ExpandMatrixWithExclusions expands dimensions in their declared order and
// removes combinations matching any partial exclusion. Exclusion assignments
// are normalized to dimension order for stable provenance and export.
func ExpandMatrixWithExclusions(dimensions []MatrixDimension, exclusions []MatrixExclusion) ([][]MatrixValue, []MatrixExclusion, error) {
	if len(dimensions) == 0 {
		if len(exclusions) > 0 {
			return nil, nil, fmt.Errorf("matrix_exclude requires matrix dimensions")
		}
		return nil, nil, nil
	}
	dimensionValues, err := matrixDimensionValueLookup(dimensions)
	if err != nil {
		return nil, nil, err
	}
	normalized, err := normalizeMatrixExclusions(dimensions, dimensionValues, exclusions)
	if err != nil {
		return nil, nil, err
	}
	remaining := filterMatrixCombinations(ExpandMatrix(dimensions), normalized)
	if len(remaining) == 0 {
		return nil, nil, fmt.Errorf("matrix_exclude removes every matrix combination")
	}
	return remaining, normalized, nil
}

func matrixDimensionValueLookup(dimensions []MatrixDimension) (map[string]map[string]bool, error) {
	dimensionValues := make(map[string]map[string]bool, len(dimensions))
	for _, dimension := range dimensions {
		if !ValidEnvironmentName(dimension.Name) || len(dimension.Values) == 0 || dimensionValues[dimension.Name] != nil {
			return nil, fmt.Errorf("invalid matrix dimension %q", dimension.Name)
		}
		values := make(map[string]bool, len(dimension.Values))
		for _, value := range dimension.Values {
			if value == "" || values[value] {
				return nil, fmt.Errorf("invalid value in matrix dimension %q", dimension.Name)
			}
			values[value] = true
		}
		dimensionValues[dimension.Name] = values
	}
	return dimensionValues, nil
}

func normalizeMatrixExclusions(dimensions []MatrixDimension, dimensionValues map[string]map[string]bool, exclusions []MatrixExclusion) ([]MatrixExclusion, error) {
	normalized := make([]MatrixExclusion, 0, len(exclusions))
	seenRules := make(map[string]bool, len(exclusions))
	for _, exclusion := range exclusions {
		ordered, err := normalizeMatrixExclusion(dimensions, dimensionValues, exclusion)
		if err != nil {
			return nil, err
		}
		key := matrixValuesKey(ordered.Values)
		if seenRules[key] {
			return nil, fmt.Errorf("matrix_exclude repeats an exclusion rule")
		}
		seenRules[key] = true
		normalized = append(normalized, ordered)
	}
	if len(normalized) == 0 {
		normalized = nil
	}
	return normalized, nil
}

func normalizeMatrixExclusion(dimensions []MatrixDimension, dimensionValues map[string]map[string]bool, exclusion MatrixExclusion) (MatrixExclusion, error) {
	if len(exclusion.Values) == 0 {
		return MatrixExclusion{}, fmt.Errorf("matrix_exclude entries must not be empty")
	}
	assignments, err := matrixExclusionAssignments(dimensionValues, exclusion.Values)
	if err != nil {
		return MatrixExclusion{}, err
	}
	ordered := make([]MatrixValue, 0, len(assignments))
	for _, dimension := range dimensions {
		if value, exists := assignments[dimension.Name]; exists {
			ordered = append(ordered, MatrixValue{Name: dimension.Name, Value: value})
		}
	}
	return MatrixExclusion{Values: ordered}, nil
}

func matrixExclusionAssignments(dimensionValues map[string]map[string]bool, values []MatrixValue) (map[string]string, error) {
	assignments := make(map[string]string, len(values))
	for _, assignment := range values {
		declaredValues, exists := dimensionValues[assignment.Name]
		if !exists {
			return nil, fmt.Errorf("matrix_exclude names unknown dimension %q", assignment.Name)
		}
		if !declaredValues[assignment.Value] {
			return nil, fmt.Errorf("matrix_exclude has undeclared value %q for dimension %q", assignment.Value, assignment.Name)
		}
		if _, exists := assignments[assignment.Name]; exists {
			return nil, fmt.Errorf("matrix_exclude repeats dimension %q", assignment.Name)
		}
		assignments[assignment.Name] = assignment.Value
	}
	return assignments, nil
}

func filterMatrixCombinations(combinations [][]MatrixValue, exclusions []MatrixExclusion) [][]MatrixValue {
	remaining := make([][]MatrixValue, 0, len(combinations))
	for _, combination := range combinations {
		if !matrixCombinationExcluded(combination, exclusions) {
			remaining = append(remaining, combination)
		}
	}
	return remaining
}

func matrixCombinationExcluded(combination []MatrixValue, exclusions []MatrixExclusion) bool {
	for _, exclusion := range exclusions {
		if matrixCombinationMatches(combination, exclusion.Values) {
			return true
		}
	}
	return false
}

func matrixCombinationMatches(combination, assignments []MatrixValue) bool {
	values := make(map[string]string, len(combination))
	for _, value := range combination {
		values[value.Name] = value.Value
	}
	for _, assignment := range assignments {
		if values[assignment.Name] != assignment.Value {
			return false
		}
	}
	return true
}

func MatrixJobName(baseName string, values []MatrixValue) string {
	name := baseName
	for _, value := range values {
		if name != "" {
			name += "-" + value.Name + SanitizeMatrixName(value.Value)
		}
	}
	return name
}

func MatrixEnvironment(base []string, values []MatrixValue) []string {
	environment := append([]string(nil), base...)
	for _, value := range values {
		environment = append(environment, value.Name+"="+value.Value)
	}
	return environment
}

func SanitizeMatrixName(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '.' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func ParseArrayRange(value string) (ArraySpec, error) {
	values := strings.Split(value, ",")
	if len(values) == 1 && strings.TrimSpace(values[0]) == "" {
		return ArraySpec{}, fmt.Errorf("want FIRST-LAST or TASK[,TASK...]")
	}
	tasks := make([]int, 0, len(values))
	seen := make(map[int]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return ArraySpec{}, fmt.Errorf("empty task index")
		}
		parts := strings.Split(value, "-")
		if len(parts) > 2 {
			return ArraySpec{}, fmt.Errorf("invalid task range %q", value)
		}
		first, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return ArraySpec{}, fmt.Errorf("invalid task index: %w", err)
		}
		last := first
		if len(parts) == 2 {
			last, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return ArraySpec{}, fmt.Errorf("invalid last index: %w", err)
			}
			if first > last {
				return ArraySpec{}, errors.New("first index must not be greater than last index")
			}
		}
		for task := first; task <= last; task++ {
			if seen[task] {
				return ArraySpec{}, fmt.Errorf("duplicate task index: %d", task)
			}
			seen[task] = true
			tasks = append(tasks, task)
		}
	}
	sort.Ints(tasks)
	array := ArraySpec{First: tasks[0], Last: tasks[len(tasks)-1]}
	if len(tasks) != array.Last-array.First+1 {
		array.Tasks = tasks
	}
	return array, nil
}

func ValidateArraySpec(array *ArraySpec) error {
	if array.First < 0 || array.Last < 0 {
		return errors.New("task indexes must not be negative")
	}
	if array.First > array.Last {
		return errors.New("first index must not be greater than last index")
	}
	if len(array.Tasks) == 0 {
		return nil
	}
	if array.Tasks[0] != array.First || array.Tasks[len(array.Tasks)-1] != array.Last {
		return errors.New("first and last indexes must match the selected tasks")
	}
	previous := array.First - 1
	for _, task := range array.Tasks {
		if task < array.First || task > array.Last {
			return fmt.Errorf("task index %d is outside %d-%d", task, array.First, array.Last)
		}
		if task <= previous {
			return fmt.Errorf("task indexes must be strictly increasing: %d", task)
		}
		previous = task
	}
	return nil
}

type JobOrigin struct {
	RunID       string `json:"run_id"`
	JobID       string `json:"job_id"`
	AttemptID   string `json:"attempt_id,omitempty"`
	Status      string `json:"status,omitempty"`
	CWD         string `json:"cwd,omitempty"`
	SubmittedAt string `json:"submitted_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
}

type Meta struct {
	Phase           string `json:"phase"`
	LastRunID       string `json:"last_run_id,omitempty"`
	LastRunExitCode int    `json:"last_run_exit_code,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}

type LockInfo struct {
	PID       int    `json:"pid"`
	RunID     string `json:"run_id"`
	RunName   string `json:"run_name,omitempty"`
	StartedAt string `json:"started_at"`
	Host      string `json:"host,omitempty"`
}

// EnvJobDir is the environment variable name used to carry a job's working
// state directory into scheduler array wrapper scripts. It is a wire-format
// contract shared between cmd/rotari (which sets it) and internal/executor
// (which reads it back out of JobSpec.Environment) and must stay in sync
// with cmd/rotari's envJobDir constant.
const EnvJobDir = "ROTARI_JOB_DIR"

type JobSpec struct {
	ID               string   `json:"id"`
	AttemptID        string   `json:"attempt_id,omitempty"`
	EnvMode          string   `json:"env_mode,omitempty"`
	Command          []string `json:"command"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	Executor         string   `json:"executor,omitempty"`
	ExecutorOptions  []string `json:"executor_options,omitempty"`
	Name             string   `json:"name,omitempty"`
	Stage            string   `json:"stage,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	// DependsOnFinished names prerequisites that must finish, whatever their
	// result, before the job starts.
	DependsOnFinished []string `json:"depends_on_finished,omitempty"`
	// Timeout limits how long the job may run once it starts.
	Timeout string `json:"timeout,omitempty"`
	// Retry overrides the run's --retry limit for this job when set.
	Retry *int `json:"retry,omitempty"`
	// RetryDelay, RetryBackoff, and RetryMaxDelay space out the job's retries.
	RetryDelay    string   `json:"retry_delay,omitempty"`
	RetryBackoff  float64  `json:"retry_backoff,omitempty"`
	RetryMaxDelay string   `json:"retry_max_delay,omitempty"`
	ArrayGroup    string   `json:"array_group,omitempty"`
	ArrayTaskID   *int     `json:"array_task_id,omitempty"`
	ArrayFirst    int      `json:"array_first,omitempty"`
	ArrayLast     int      `json:"array_last,omitempty"`
	ArraySize     int      `json:"array_size,omitempty"`
	Environment   []string `json:"environment,omitempty"`
	// InheritedEnvironment carries run-scoped caller values to executor adapters
	// without persisting them in per-attempt command snapshots.
	InheritedEnvironment []string `json:"-"`
	Output               []string `json:"output,omitempty"`
	Error                []string `json:"error,omitempty"`
	LogMode              string   `json:"log_mode,omitempty"`
	OpenMode             string   `json:"open_mode,omitempty"`
}

const (
	EnvModeAll       = "ALL"
	EnvModeNone      = "NONE"
	LogModeMerge     = "merge"
	LogModeSeparate  = "separate"
	OpenModeAppend   = "append"
	OpenModeTruncate = "truncate"
)

func (job JobSpec) EffectiveLogMode() string {
	if job.LogMode == LogModeSeparate {
		return LogModeSeparate
	}
	return LogModeMerge
}

type RuleDiagnosis struct {
	Name       string `json:"name"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion"`
}

// JobResult is one job's outcome in a run summary. A failed result carries
// its saved rule-based analysis: DiagnosisStatus is DiagnosisMatched,
// DiagnosisNoMatch, or DiagnosisUnavailable (empty when never analyzed);
// Diagnoses lists only recognized diagnoses; DiagnosisNote says why an
// unavailable analysis could not run; and DiagnosisRules identifies the rule
// set that produced the analysis.
type JobResult struct {
	ID string `json:"id"`
	// Name is the job's name, such as train[3], in a run summary; results
	// written before it was recorded have none.
	Name            string          `json:"name,omitempty"`
	AttemptID       string          `json:"attempt_id,omitempty"`
	ExitCode        int             `json:"exit_code"`
	Accepted        bool            `json:"accepted,omitempty"`
	Error           string          `json:"error,omitempty"`
	Command         []string        `json:"command,omitempty"`
	Hosts           []string        `json:"hosts,omitempty"`
	Diagnoses       []RuleDiagnosis `json:"diagnoses,omitempty"`
	DiagnosisStatus string          `json:"diagnosis_status,omitempty"`
	DiagnosisNote   string          `json:"diagnosis_note,omitempty"`
	DiagnosisRules  string          `json:"diagnosis_rules,omitempty"`
}

// Rule-based analysis outcomes saved in JobResult.DiagnosisStatus.
const (
	DiagnosisMatched     = "matched"
	DiagnosisNoMatch     = "no_match"
	DiagnosisUnavailable = "unavailable"
)

// Names of the entries that recorded a no-match or unavailable analysis in
// Diagnoses before DiagnosisStatus existed.
const (
	legacyNoMatchDiagnosisName     = "No known rule-based diagnosis matched"
	legacyUnavailableDiagnosisName = "Rule-based diagnosis unavailable"
)

// ClearDiagnosis removes the saved rule-based analysis.
func (result *JobResult) ClearDiagnosis() {
	result.Diagnoses = nil
	result.DiagnosisStatus = ""
	result.DiagnosisNote = ""
	result.DiagnosisRules = ""
}

// UnmarshalJSON decodes a job result and converts an analysis saved before
// DiagnosisStatus existed, whose outcome was stored as a Diagnoses entry.
func (result *JobResult) UnmarshalJSON(data []byte) error {
	type plainJobResult JobResult
	if err := json.Unmarshal(data, (*plainJobResult)(result)); err != nil {
		return err
	}
	if result.DiagnosisStatus != "" || len(result.Diagnoses) == 0 {
		return nil
	}
	if len(result.Diagnoses) == 1 {
		switch entry := result.Diagnoses[0]; entry.Name {
		case legacyNoMatchDiagnosisName:
			result.DiagnosisStatus, result.Diagnoses = DiagnosisNoMatch, nil
			return nil
		case legacyUnavailableDiagnosisName:
			result.DiagnosisStatus, result.DiagnosisNote, result.Diagnoses = DiagnosisUnavailable, entry.Evidence, nil
			return nil
		}
	}
	result.DiagnosisStatus = DiagnosisMatched
	return nil
}

type RunSummary struct {
	// StateVersion is set by the state package when the summary is written.
	StateVersion int         `json:"state_version,omitempty"`
	RunID        string      `json:"run_id"`
	RunName      string      `json:"run_name,omitempty"`
	Status       string      `json:"status"`
	StartedAt    string      `json:"started_at"`
	FinishedAt   string      `json:"finished_at"`
	ExitCode     int         `json:"exit_code"`
	Results      []JobResult `json:"results"`
}

type RunContext struct {
	CWD                 string       `json:"cwd"`
	ConfigPaths         []string     `json:"config_paths,omitempty"`
	ConfigSnapshotFiles []string     `json:"config_snapshot_files,omitempty"`
	ConfigSnapshotPaths []string     `json:"config_snapshot_paths,omitempty"`
	Hostname            string       `json:"hostname,omitempty"`
	StartedLoad         *LoadAverage `json:"started_load,omitempty"`
	FinishedLoad        *LoadAverage `json:"finished_load,omitempty"`
	LoadSamples         []LoadSample `json:"load_samples,omitempty"`
}

type LoadAverage struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type LoadSample struct {
	At string `json:"at"`
	LoadAverage
}

func QueueToJobs(commands []QueuedCommand) []JobSpec {
	jobs := make([]JobSpec, 0, len(commands))
	stageJobs := make(map[string][]string)
	matrixJobs := make(map[string][]string)
	// A named array's name stands for its tasks, as a stage or matrix name
	// stands for its members.
	arrayJobs := make(map[string][]string)
	for _, queued := range commands {
		commandJobs := queueCommandToJobs(queued)
		jobs = append(jobs, commandJobs...)
		if queued.Array != nil && queued.Name != "" {
			for _, job := range commandJobs {
				arrayJobs[queued.Name] = append(arrayJobs[queued.Name], job.Name)
			}
		}
		if queued.Stage != "" {
			for _, job := range commandJobs {
				stageJobs[queued.Stage] = append(stageJobs[queued.Stage], job.Name)
			}
		}
		if queued.Matrix != nil && queued.Matrix.BaseName != "" {
			for _, job := range commandJobs {
				matrixJobs[queued.Matrix.BaseName] = append(matrixJobs[queued.Matrix.BaseName], job.Name)
			}
		}
	}
	expandStageDependencies(jobs, stageJobs)
	expandStageDependencies(jobs, matrixJobs)
	expandStageDependencies(jobs, arrayJobs)
	return jobs
}

func queueCommandToJobs(queued QueuedCommand) []JobSpec {
	if len(queued.Command) == 0 {
		return nil
	}
	if queued.Array == nil {
		return []JobSpec{queueCommandJob(queued, queued.ID, queued.Name, nil)}
	}
	tasks := ArrayTaskIDs(queued.Array)
	jobs := make([]JobSpec, 0, len(tasks))
	for _, task := range tasks {
		id := fmt.Sprintf("%s-%d", queued.ID, task)
		name := queued.Name
		if name != "" {
			name = fmt.Sprintf("%s[%d]", name, task)
		}
		taskID := task
		jobs = append(jobs, queueCommandJob(queued, id, name, &taskID))
	}
	return jobs
}

func queueCommandJob(queued QueuedCommand, id, name string, taskID *int) JobSpec {
	if name == "" && queued.Stage != "" {
		name = id
	}
	job := JobSpec{
		ID: id, Command: queued.Command, WorkingDirectory: queued.WorkingDirectory, Name: name,
		Executor: queued.Executor, ExecutorOptions: queued.ExecutorOptions, Environment: queued.Environment, Stage: queued.Stage, DependsOn: queued.DependsOn,
		DependsOnFinished: queued.DependsOnFinished, Timeout: queued.Timeout, Retry: queued.Retry,
		RetryDelay: queued.RetryDelay, RetryBackoff: queued.RetryBackoff, RetryMaxDelay: queued.RetryMaxDelay,
		Output: queued.Output, Error: queued.Error, LogMode: queued.LogMode,
		OpenMode: queued.OpenMode,
	}
	if taskID != nil {
		job.ArrayGroup = queued.ID
		job.ArrayTaskID = taskID
		job.ArrayFirst = queued.Array.First
		job.ArrayLast = queued.Array.Last
		job.ArraySize = len(ArrayTaskIDs(queued.Array))
	}
	return job
}

func expandStageDependencies(jobs []JobSpec, stageJobs map[string][]string) {
	for index := range jobs {
		jobs[index].DependsOn = expandGroupNames(jobs[index].DependsOn, stageJobs)
		if jobs[index].DependsOnFinished != nil {
			jobs[index].DependsOnFinished = expandGroupNames(jobs[index].DependsOnFinished, stageJobs)
		}
	}
}

func expandGroupNames(names []string, groups map[string][]string) []string {
	expanded := make([]string, 0, len(names))
	for _, name := range names {
		if members, ok := groups[name]; ok {
			expanded = append(expanded, members...)
		} else {
			expanded = append(expanded, name)
		}
	}
	return expanded
}

// ParseTimeout parses a job timeout, a Go duration such as "90m" or "2h".
// It must be at least one second; the wrapper enforces whole seconds.
func ParseTimeout(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: want a duration such as 90m or 2h", value)
	}
	if duration < time.Second {
		return 0, fmt.Errorf("invalid timeout %q: must be at least 1s", value)
	}
	return duration, nil
}

// TimeoutSeconds returns a job's timeout rounded up to whole seconds, or 0
// when it has none or it is invalid.
func TimeoutSeconds(value string) int {
	if value == "" {
		return 0
	}
	duration, err := ParseTimeout(value)
	if err != nil {
		return 0
	}
	return int((duration + time.Second - 1) / time.Second)
}

// RetryLimit returns how many times a failed job may be retried: its own
// Retry when set, otherwise the run's limit, where -1 means no limit.
func (job JobSpec) RetryLimit(runRetry int) int {
	if job.Retry != nil {
		return *job.Retry
	}
	return runRetry
}

// ValidateRetryBackoff checks a job's retry spacing: delays are Go durations
// of zero or more, and the backoff factor is at least 1 when set.
func ValidateRetryBackoff(delay string, backoff float64, maxDelay string) error {
	for _, field := range []struct{ name, value string }{{"retry delay", delay}, {"retry max delay", maxDelay}} {
		if field.value == "" {
			continue
		}
		if duration, err := time.ParseDuration(field.value); err != nil || duration < 0 {
			return fmt.Errorf("invalid %s %q: want a duration such as 30s or 5m", field.name, field.value)
		}
	}
	if backoff != 0 && backoff < 1 {
		return fmt.Errorf("invalid retry backoff %v: must be 1 or more", backoff)
	}
	return nil
}

// RetryDelayFor returns how long to wait before the job's retryNumber-th
// retry (1 for the first): RetryDelay multiplied by RetryBackoff for each
// earlier retry, capped at RetryMaxDelay. Without RetryDelay the job is
// retried at once.
func (job JobSpec) RetryDelayFor(retryNumber int) time.Duration {
	delay, err := time.ParseDuration(job.RetryDelay)
	if err != nil || delay <= 0 {
		return 0
	}
	maxDelay, maxErr := time.ParseDuration(job.RetryMaxDelay)
	capped := maxErr == nil && maxDelay > 0
	factor := job.RetryBackoff
	if factor < 1 {
		factor = 1
	}
	for retry := 1; retry < retryNumber; retry++ {
		delay = time.Duration(float64(delay) * factor)
		if capped && delay >= maxDelay {
			break
		}
	}
	if capped && delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// FormatRetryPolicy describes a job's own retry settings, such as
// "3 (delay 30s, backoff x2, max 10m)", or "" when it has none.
func FormatRetryPolicy(job JobSpec) string {
	parts := make([]string, 0, 3)
	if job.RetryDelay != "" {
		parts = append(parts, "delay "+job.RetryDelay)
	}
	if job.RetryBackoff != 0 {
		parts = append(parts, "backoff x"+strconv.FormatFloat(job.RetryBackoff, 'g', -1, 64))
	}
	if job.RetryMaxDelay != "" {
		parts = append(parts, "max "+job.RetryMaxDelay)
	}
	limit := FormatRetry(job.Retry)
	if len(parts) == 0 {
		return limit
	}
	if limit == "" {
		limit = "run limit"
	}
	return limit + " (" + strings.Join(parts, ", ") + ")"
}

// FormatRetry formats an optional per-job retry limit, or "" when unset.
func FormatRetry(retry *int) string {
	if retry == nil {
		return ""
	}
	return strconv.Itoa(*retry)
}

// FormatDependencies joins dependsOn and dependsOnFinished for display,
// marking each name that only needs to finish with a "finished:" prefix.
func FormatDependencies(dependsOn, dependsOnFinished []string, separator string) string {
	names := joinArrayTasks(dependsOn)
	for _, name := range joinArrayTasks(dependsOnFinished) {
		names = append(names, "finished:"+name)
	}
	return strings.Join(names, separator)
}

var arrayTaskDependency = regexp.MustCompile(`^(.*)\[(\d+)\]$`)

// joinArrayTasks joins the tasks of one array among names, which a stage or
// array name expands to, into one name with task ranges, such as
// train[1-12] or train[1-2,4], placed where the first of them was.
func joinArrayTasks(names []string) []string {
	joined := make([]string, 0, len(names))
	tasks := make(map[string][]int)
	position := make(map[string]int)
	for _, name := range names {
		match := arrayTaskDependency.FindStringSubmatch(name)
		if match == nil {
			joined = append(joined, name)
			continue
		}
		task, _ := strconv.Atoi(match[2])
		if _, seen := position[match[1]]; !seen {
			position[match[1]] = len(joined)
			joined = append(joined, "")
		}
		tasks[match[1]] = append(tasks[match[1]], task)
	}
	for base, index := range position {
		ids := tasks[base]
		sort.Ints(ids)
		var ranges []string
		for start := 0; start < len(ids); {
			end := start
			for end+1 < len(ids) && ids[end+1] == ids[end]+1 {
				end++
			}
			if end == start {
				ranges = append(ranges, strconv.Itoa(ids[start]))
			} else {
				ranges = append(ranges, strconv.Itoa(ids[start])+"-"+strconv.Itoa(ids[end]))
			}
			start = end + 1
		}
		joined[index] = base + "[" + strings.Join(ranges, ",") + "]"
	}
	return joined
}

// AllDependencies returns the names in DependsOn followed by those in
// DependsOnFinished.
func (command QueuedCommand) AllDependencies() []string {
	return concatNames(command.DependsOn, command.DependsOnFinished)
}

// AllDependencies returns the names in DependsOn followed by those in
// DependsOnFinished.
func (job JobSpec) AllDependencies() []string {
	return concatNames(job.DependsOn, job.DependsOnFinished)
}

func concatNames(left, right []string) []string {
	if len(right) == 0 {
		return left
	}
	return append(append(make([]string, 0, len(left)+len(right)), left...), right...)
}

func ArrayTaskIDs(array *ArraySpec) []int {
	if array == nil {
		return nil
	}
	if len(array.Tasks) > 0 {
		return array.Tasks
	}
	tasks := make([]int, 0, array.Last-array.First+1)
	for task := array.First; task <= array.Last; task++ {
		tasks = append(tasks, task)
	}
	return tasks
}

// RunLabel names a run for display: its name with the ID in parentheses, or
// the ID alone when the run has no name.
func RunLabel(runID, runName string) string {
	if runName == "" {
		return runID
	}
	return fmt.Sprintf("%s (%s)", runName, runID)
}
