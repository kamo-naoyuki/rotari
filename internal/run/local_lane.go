package run

import (
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

func RunLocalLane(workers *sync.WaitGroup, runDir string, jobExecutor executor.JobExecutor, jobs []model.JobSpec, concurrency int, results chan<- model.JobResult, onStart func(model.JobSpec)) {
	defer workers.Done()
	semaphore := make(chan struct{}, concurrency)
	var jobsWait sync.WaitGroup
	for _, job := range jobs {
		jobsWait.Add(1)
		go func(job model.JobSpec) {
			defer jobsWait.Done()
			semaphore <- struct{}{}
			handle, err := jobExecutor.Submit(runDir, job, nil)
			if err != nil {
				results <- model.JobResult{ID: job.ID, AttemptID: job.AttemptID, Command: job.Command, ExitCode: 1, Error: err.Error()}
			} else {
				if onStart != nil {
					onStart(job)
				}
				result := jobExecutor.Wait(runDir, handle)
				result.AttemptID = job.AttemptID
				results <- result
			}
			<-semaphore
		}(job)
	}
	jobsWait.Wait()
}
