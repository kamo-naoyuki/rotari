package supervisor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestProgressObserverPreservesResultResponses(t *testing.T) {
	for _, test := range []struct {
		name    string
		retry   int
		result  model.JobResult
		message string
	}{
		{
			name:   "success",
			result: model.JobResult{ID: "job", Command: []string{"true"}},
		},
		{
			name: "retry", retry: 2,
			result:  model.JobResult{ID: "job", Command: []string{"false"}, Error: "retry:2"},
			message: "Retrying job: attempt=2 job=job command=[false]",
		},
		{
			name:    "failure with attempt",
			result:  model.JobResult{ID: "job", AttemptID: "attempt", Command: []string{"exit", "7"}, ExitCode: 7, Error: "final-failure"},
			message: "Job failed:\n  ID: job\n  Attempt ID: attempt\n  Command: exit 7\n  Show output:\n    rotari show --run-id run-1 --job-id attempt",
		},
		{
			name: "failure after retry without attempt", retry: 1,
			result:  model.JobResult{ID: "job", Command: []string{"exit", "9"}, ExitCode: 9, Error: "final-failure"},
			message: "Job failed after retry:\n  ID: job\n  Attempt ID: job\n  Command: exit 9\n  Show output:\n    rotari show --run-id run-1 --job-id job",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ops, paths := progressTestOperations(t)
			runDir := filepath.Join(paths.RunsDir, "run-1")
			if err := os.MkdirAll(runDir, state.DirectoryMode()); err != nil {
				t.Fatal(err)
			}
			var forwarded []server.Response
			observer := ops.progressObserver(server.Request{QueueName: "demo", Retry: test.retry, Quiet: true}, startedRun{paths: paths, runID: "run-1", submitted: 3, total: 5}, func(event server.Response) {
				forwarded = append(forwarded, event)
				// Every forwarded response is already visible on disk, even
				// while the run is active, with no buffering until completion.
				if got := progressResponses(t, runDir); !reflect.DeepEqual(got, forwarded) {
					t.Fatalf("live journal = %+v, want %+v", got, forwarded)
				}
			})
			observer.Progress(test.result, 3, 5, 2, 1)
			want := server.Response{OK: true, Progress: true, Message: test.message, JobID: "job", Completed: 3, Total: 5, Succeeded: 2, Failed: 1}
			if len(forwarded) != 2 || forwarded[1] != want {
				t.Fatalf("response = %+v, want %+v", forwarded, want)
			}
		})
	}
}
