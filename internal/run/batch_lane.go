package run

import (
	"strings"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type BatchLaneCallbacks struct {
	// ValidatedJobDir resolves a job directory for scheduler submission checks.
	ValidatedJobDir func(runDir, jobID string) (string, error)
	JobCancelled    func(jobDir string) bool
	RecordCancelled func(jobDir string, job model.JobSpec) model.JobResult
	Logf            func(string, ...any)
}

func RunBatchLane(workers *sync.WaitGroup, runDir string, queue model.Queue, jobExecutor executor.JobExecutor, jobs []model.JobSpec, maxActive int, executorOptions []string, results chan<- model.JobResult, callbacks BatchLaneCallbacks, onStart func(model.JobSpec)) {
	defer workers.Done()
	for start := 0; start < len(jobs); {
		if jobs[start].ArrayGroup != "" {
			end := start + 1
			for end < len(jobs) && jobs[end].ArrayGroup == jobs[start].ArrayGroup {
				end++
			}
			completeArray := CompleteArrayGroup(jobs[start:end], jobs[start].ArrayFirst, jobs[start].ArrayLast)
			sparseArray := false
			if supporter, ok := jobExecutor.(executor.SparseArraySupporter); ok {
				sparseArray = supporter.SupportsSparseArray()
			}
			if submitter, ok := jobExecutor.(executor.ArraySubmitter); ok && (completeArray || sparseArray) {
				options := jobs[start].ExecutorOptions
				if len(options) == 0 {
					options = executorOptions
				}
				if len(options) == 0 {
					options = queue.DefaultExecutorOptions
				}
				handles, err := submitter.SubmitArray(runDir, jobs[start:end], options)
				if err != nil {
					for _, job := range jobs[start:end] {
						results <- model.JobResult{ID: job.ID, AttemptID: job.AttemptID, ExitCode: 1, Error: err.Error()}
					}
				} else {
					if onStart != nil {
						for _, job := range jobs[start:end] {
							onStart(job)
						}
					}
					for _, handle := range handles {
						result := jobExecutor.Wait(runDir, handle)
						result.AttemptID = handle.Job.AttemptID
						results <- result
					}
				}
				start = end
				continue
			}
		}
		end := start + maxActive
		if end > len(jobs) {
			end = len(jobs)
		}
		handles := make([]executor.JobHandle, 0, end-start)
		for _, job := range jobs[start:end] {
			jobDir, err := callbacks.ValidatedJobDir(runDir, job.ID)
			if err != nil {
				results <- model.JobResult{ID: job.ID, AttemptID: job.AttemptID, ExitCode: 1, Error: err.Error()}
				continue
			}
			if callbacks.JobCancelled(jobDir) {
				results <- callbacks.RecordCancelled(jobDir, job)
				continue
			}
			options := job.ExecutorOptions
			if len(options) == 0 {
				options = executorOptions
			}
			if len(options) == 0 {
				options = queue.DefaultExecutorOptions
			}
			handle, err := jobExecutor.Submit(runDir, job, options)
			if err != nil {
				results <- model.JobResult{ID: job.ID, ExitCode: 1, Error: err.Error()}
				continue
			}
			if onStart != nil {
				onStart(job)
			}
			handles = append(handles, handle)
		}
		for _, handle := range handles {
			result := jobExecutor.Wait(runDir, handle)
			result.AttemptID = handle.Job.AttemptID
			if result.ExitCode != 0 && callbacks.Logf != nil {
				callbacks.Logf("fail job=%s exit=%d command=%s\n", result.ID, result.ExitCode, strings.Join(result.Command, " "))
			}
			results <- result
		}
		start = end
	}
}
