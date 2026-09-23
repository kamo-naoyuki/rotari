package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Queue struct {
	DefaultExecutor        string          `json:"default_executor,omitempty"`
	DefaultExecutorOptions []string        `json:"default_executor_options,omitempty"`
	Commands               []QueuedCommand `json:"commands"`
}

type QueuedCommand struct {
	ID               string                `json:"id"`
	Command          []string              `json:"command"`
	WorkingDirectory string                `json:"working_directory,omitempty"`
	Executor         string                `json:"executor,omitempty"`
	ExecutorOptions  []string              `json:"executor_options,omitempty"`
	Environment      []string              `json:"environment,omitempty"`
	Name             string                `json:"name,omitempty"`
	DependsOn        []string              `json:"depends_on,omitempty"`
	Origin           *JobOrigin            `json:"origin,omitempty"`
	Array            *ArraySpec            `json:"array,omitempty"`
	TaskOrigins      map[string]*JobOrigin `json:"task_origins,omitempty"`
}

type ArraySpec struct {
	First int   `json:"first"`
	Last  int   `json:"last"`
	Tasks []int `json:"tasks,omitempty"`
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

type JobSpec struct {
	ID               string   `json:"id"`
	AttemptID        string   `json:"attempt_id,omitempty"`
	Command          []string `json:"command"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	Executor         string   `json:"executor,omitempty"`
	ExecutorOptions  []string `json:"executor_options,omitempty"`
	Name             string   `json:"name,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	ArrayGroup       string   `json:"array_group,omitempty"`
	ArrayTaskID      *int     `json:"array_task_id,omitempty"`
	ArrayFirst       int      `json:"array_first,omitempty"`
	ArrayLast        int      `json:"array_last,omitempty"`
	ArraySize        int      `json:"array_size,omitempty"`
	Environment      []string `json:"environment,omitempty"`
}

type RuleDiagnosis struct {
	Name       string `json:"name"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion"`
}

type JobResult struct {
	ID        string          `json:"id"`
	AttemptID string          `json:"attempt_id,omitempty"`
	ExitCode  int             `json:"exit_code"`
	Error     string          `json:"error,omitempty"`
	Command   []string        `json:"command,omitempty"`
	Hosts     []string        `json:"hosts,omitempty"`
	Diagnoses []RuleDiagnosis `json:"diagnoses,omitempty"`
}

type RunSummary struct {
	RunID      string      `json:"run_id"`
	RunName    string      `json:"run_name,omitempty"`
	Status     string      `json:"status"`
	StartedAt  string      `json:"started_at"`
	FinishedAt string      `json:"finished_at"`
	ExitCode   int         `json:"exit_code"`
	Results    []JobResult `json:"results"`
}

type RunContext struct {
	CWD          string       `json:"cwd"`
	ConfigPaths  []string     `json:"config_paths,omitempty"`
	Hostname     string       `json:"hostname,omitempty"`
	StartedLoad  *LoadAverage `json:"started_load,omitempty"`
	FinishedLoad *LoadAverage `json:"finished_load,omitempty"`
	LoadSamples  []LoadSample `json:"load_samples,omitempty"`
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
	for _, queued := range commands {
		if len(queued.Command) == 0 {
			continue
		}
		if queued.Array == nil {
			jobs = append(jobs, JobSpec{
				ID: queued.ID, Command: queued.Command, WorkingDirectory: queued.WorkingDirectory, Name: queued.Name,
				Executor: queued.Executor, ExecutorOptions: queued.ExecutorOptions, Environment: queued.Environment, DependsOn: queued.DependsOn,
			})
			continue
		}
		for _, task := range ArrayTaskIDs(queued.Array) {
			id := fmt.Sprintf("%s-%d", queued.ID, task)
			name := queued.Name
			if name != "" {
				name = fmt.Sprintf("%s[%d]", name, task)
			}
			taskID := task
			jobs = append(jobs, JobSpec{
				ID: id, Command: queued.Command, WorkingDirectory: queued.WorkingDirectory, Name: name,
				Executor: queued.Executor, ExecutorOptions: queued.ExecutorOptions, Environment: queued.Environment, DependsOn: queued.DependsOn,
				ArrayGroup: queued.ID, ArrayTaskID: &taskID, ArrayFirst: queued.Array.First, ArrayLast: queued.Array.Last, ArraySize: len(ArrayTaskIDs(queued.Array)),
			})
		}
	}
	return jobs
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
