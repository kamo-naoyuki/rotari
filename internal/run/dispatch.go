package run

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// DispatchOptions configures a Dispatcher.
type DispatchOptions struct {
	LocalConcurrency  int
	BatchMaxActive    int
	RequestedExecutor string
	ExecutorOptions   []string
	Settings          executor.RunSettingsMap
	ResolveExecutor   func(name string) (executor.JobExecutor, bool)
	Callbacks         BatchLaneCallbacks
}

// BatchLaneCallbacks lets scheduler lanes check and record cancellation of
// jobs that have not been submitted yet.
type BatchLaneCallbacks struct {
	// ValidatedJobDir resolves a job directory for scheduler submission checks.
	ValidatedJobDir func(runDir, jobID string) (string, error)
	JobCancelled    func(jobDir string) bool
	RecordCancelled func(jobDir string, job model.JobSpec) model.JobResult
	Logf            func(string, ...any)
}

// Dispatcher runs jobs on one lane per executor for the whole run. Each lane
// holds a concurrency slot only while a job is submitted or running, so a
// finished job frees its slot for the next one at once.
type Dispatcher struct {
	runDir          string
	queue           model.Queue
	options         DispatchOptions
	onStart         func(model.JobSpec)
	defaultExecutor string

	mu    sync.Mutex
	lanes map[string]*lane
}

type lane struct {
	executor executor.JobExecutor
	local    bool
	slots    chan struct{}
	options  []string
}

// NewDispatcher returns a Dispatcher for a run. onStart, when set, is called
// once a job has been submitted.
func NewDispatcher(runDir string, queue model.Queue, options DispatchOptions, onStart func(model.JobSpec)) *Dispatcher {
	defaultExecutor := options.RequestedExecutor
	if defaultExecutor == "" {
		defaultExecutor = queue.DefaultExecutor
	}
	if defaultExecutor == "" {
		defaultExecutor = "local"
	}
	return &Dispatcher{runDir: runDir, queue: queue, options: options, onStart: onStart, defaultExecutor: defaultExecutor, lanes: make(map[string]*lane)}
}

func (dispatcher *Dispatcher) lane(name string) (*lane, bool) {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if existing, ok := dispatcher.lanes[name]; ok {
		return existing, true
	}
	jobExecutor, ok := dispatcher.options.ResolveExecutor(name)
	if !ok {
		return nil, false
	}
	created := &lane{executor: configuredExecutor(jobExecutor, dispatcher.options.Settings[name]), local: name == "local"}
	concurrency := effectiveConcurrency(dispatcher.options.Settings, name, dispatcher.options.BatchMaxActive)
	if created.local {
		concurrency = effectiveConcurrency(dispatcher.options.Settings, name, dispatcher.options.LocalConcurrency)
	} else {
		created.options = effectiveOptions(dispatcher.options.Settings, name, dispatcher.options.ExecutorOptions)
	}
	if concurrency < 1 {
		concurrency = 1
	}
	created.slots = make(chan struct{}, concurrency)
	dispatcher.lanes[name] = created
	return created, true
}

// Start runs jobs that became ready together and calls done once per job,
// from another goroutine, with its result. Array tasks that are ready
// together go to the scheduler as one native array when it supports that.
func (dispatcher *Dispatcher) Start(jobs []model.JobSpec, done func(model.JobResult)) {
	order := make([]string, 0)
	grouped := make(map[string][]model.JobSpec)
	for _, job := range jobs {
		name := job.Executor
		if name == "" {
			name = dispatcher.defaultExecutor
		}
		if _, ok := grouped[name]; !ok {
			order = append(order, name)
		}
		grouped[name] = append(grouped[name], job)
	}
	for _, name := range order {
		executorJobs := grouped[name]
		selected, ok := dispatcher.lane(name)
		if !ok {
			for _, job := range executorJobs {
				result := model.JobResult{ID: job.ID, AttemptID: job.AttemptID, Command: job.Command, ExitCode: 1, Error: fmt.Sprintf("unsupported executor: %s", name)}
				go done(result)
			}
			continue
		}
		if selected.local {
			for _, job := range executorJobs {
				go dispatcher.runLocal(selected, job, done)
			}
			continue
		}
		dispatcher.startBatch(selected, executorJobs, done)
	}
}

func (dispatcher *Dispatcher) runLocal(selected *lane, job model.JobSpec, done func(model.JobResult)) {
	selected.slots <- struct{}{}
	var result model.JobResult
	handle, err := selected.executor.Submit(dispatcher.runDir, job, nil)
	if err != nil {
		result = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	} else {
		if dispatcher.onStart != nil {
			dispatcher.onStart(job)
		}
		result = selected.executor.Wait(dispatcher.runDir, handle)
	}
	<-selected.slots
	result.AttemptID = job.AttemptID
	done(result)
}

func (dispatcher *Dispatcher) startBatch(selected *lane, jobs []model.JobSpec, done func(model.JobResult)) {
	for start := 0; start < len(jobs); {
		if jobs[start].ArrayGroup != "" {
			end := start + 1
			for end < len(jobs) && jobs[end].ArrayGroup == jobs[start].ArrayGroup {
				end++
			}
			if dispatcher.canSubmitArray(selected, jobs[start:end]) {
				go dispatcher.runArray(selected, jobs[start:end], done)
				start = end
				continue
			}
		}
		go dispatcher.runBatchJob(selected, jobs[start], done)
		start++
	}
}

func (dispatcher *Dispatcher) canSubmitArray(selected *lane, tasks []model.JobSpec) bool {
	if _, ok := selected.executor.(executor.ArraySubmitter); !ok {
		return false
	}
	if CompleteArrayGroup(tasks, tasks[0].ArrayFirst, tasks[0].ArrayLast) {
		return true
	}
	supporter, ok := selected.executor.(executor.SparseArraySupporter)
	return ok && supporter.SupportsSparseArray()
}

func (dispatcher *Dispatcher) jobOptions(selected *lane, job model.JobSpec) []string {
	if len(job.ExecutorOptions) > 0 {
		return job.ExecutorOptions
	}
	if len(selected.options) > 0 {
		return selected.options
	}
	return dispatcher.queue.DefaultExecutorOptions
}

// runArray submits tasks as one native array. Like before, an array does not
// take concurrency slots; the scheduler limits its tasks.
func (dispatcher *Dispatcher) runArray(selected *lane, tasks []model.JobSpec, done func(model.JobResult)) {
	submitter := selected.executor.(executor.ArraySubmitter)
	handles, err := submitter.SubmitArray(dispatcher.runDir, tasks, dispatcher.jobOptions(selected, tasks[0]))
	if err != nil {
		for _, job := range tasks {
			done(model.JobResult{ID: job.ID, AttemptID: job.AttemptID, Command: job.Command, ExitCode: 1, Error: err.Error()})
		}
		return
	}
	if dispatcher.onStart != nil {
		for _, job := range tasks {
			dispatcher.onStart(job)
		}
	}
	var waiters sync.WaitGroup
	for _, handle := range handles {
		waiters.Add(1)
		go func(handle executor.JobHandle) {
			defer waiters.Done()
			result := selected.executor.Wait(dispatcher.runDir, handle)
			result.AttemptID = handle.Job.AttemptID
			dispatcher.logFailure(result)
			done(result)
		}(handle)
	}
	waiters.Wait()
}

func (dispatcher *Dispatcher) runBatchJob(selected *lane, job model.JobSpec, done func(model.JobResult)) {
	selected.slots <- struct{}{}
	result := dispatcher.submitAndWait(selected, job)
	<-selected.slots
	result.AttemptID = job.AttemptID
	dispatcher.logFailure(result)
	done(result)
}

func (dispatcher *Dispatcher) submitAndWait(selected *lane, job model.JobSpec) model.JobResult {
	callbacks := dispatcher.options.Callbacks
	jobDir, err := callbacks.ValidatedJobDir(dispatcher.runDir, job.ID)
	if err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if callbacks.JobCancelled(jobDir) {
		return callbacks.RecordCancelled(jobDir, job)
	}
	handle, err := selected.executor.Submit(dispatcher.runDir, job, dispatcher.jobOptions(selected, job))
	if err != nil {
		return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: err.Error()}
	}
	if dispatcher.onStart != nil {
		dispatcher.onStart(job)
	}
	return selected.executor.Wait(dispatcher.runDir, handle)
}

func (dispatcher *Dispatcher) logFailure(result model.JobResult) {
	if result.ExitCode != 0 && dispatcher.options.Callbacks.Logf != nil {
		dispatcher.options.Callbacks.Logf("fail job=%s exit=%d command=%s\n", result.ID, result.ExitCode, strings.Join(result.Command, " "))
	}
}
