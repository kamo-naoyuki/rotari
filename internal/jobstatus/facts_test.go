package jobstatus

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestFilterJobFollowsCarriedOrigin(t *testing.T) {
	runsDir := t.TempDir()
	const executed, carried, later = "20260101-000000-aaaaaaaa", "20260102-000000-bbbbbbbb", "20260103-000000-cccccccc"
	attemptID := state.MakeAttemptID(executed, "job", 0)
	attemptDir := filepath.Join(runsDir, executed, "job", "attempts", attemptID)
	writeFile(t, filepath.Join(attemptDir, "status.json"), `{"phase":"finished","exit_code":3,"hosts":["node1"],"started_at":"2026-01-01T00:00:10Z"}`)
	writeFile(t, filepath.Join(attemptDir, "finished_at"), "2026-01-01T00:00:20Z\n")
	writeFile(t, filepath.Join(attemptDir, "output"), "CUDA out of memory\n")
	// Carried twice: run "carried" from "executed", and run "later" from "carried".
	writeQueue(t, filepath.Join(runsDir, carried), model.JobOrigin{RunID: executed, JobID: "job"})
	writeQueue(t, filepath.Join(runsDir, later), model.JobOrigin{RunID: carried, JobID: "job"})

	result := model.JobResult{ID: "job", ExitCode: 3}
	for name, origin := range map[string]model.JobOrigin{
		"executing run":  {RunID: executed, JobID: "job"},
		"carried once":   {RunID: carried, JobID: "job"},
		"carried twice":  {RunID: later, JobID: "job"},
		"attempt ID":     {RunID: later, JobID: "job", AttemptID: attemptID},
		"other job's ID": {RunID: later, JobID: "job", AttemptID: "att_bad"},
	} {
		t.Run(name, func(t *testing.T) {
			job := FilterJob(testStore(), runsDir, origin, "queued", result, true, time.Time{})
			if job.ID != "queued" || !reflect.DeepEqual(job.Attributes.Hosts, []string{"node1"}) {
				t.Fatalf("job = %+v, want ID queued on node1", job)
			}
			if got := job.Attributes.StartedAt.Format(time.RFC3339); got != "2026-01-01T00:00:10Z" {
				t.Fatalf("started = %s", got)
			}
			if got := job.Attributes.FinishedAt.Format(time.RFC3339); got != "2026-01-01T00:00:20Z" {
				t.Fatalf("finished = %s", got)
			}
			if !job.Diagnosis([]string{"cuda-gpu-memory-exhausted"}) {
				t.Fatal("diagnosis did not read the executing attempt's log")
			}
		})
	}
}

func TestFilterJobPrefersResultHostsAndCarriedTimes(t *testing.T) {
	runsDir := t.TempDir()
	origin := model.JobOrigin{RunID: "20260101-000000-aaaaaaaa", JobID: "gone", SubmittedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:01:00Z"}
	job := FilterJob(testStore(), runsDir, origin, "gone", model.JobResult{ExitCode: 1, Hosts: []string{"summary-host"}}, true, time.Time{})
	if !reflect.DeepEqual(job.Attributes.Hosts, []string{"summary-host"}) || job.Attributes.StartedAt.IsZero() || job.Attributes.FinishedAt.IsZero() {
		t.Fatalf("attributes = %+v, want summary hosts and the carry's times", job.Attributes)
	}
	if job.Diagnosis([]string{"cuda-gpu-memory-exhausted"}) {
		t.Fatal("diagnosis matched without a log")
	}
}

func writeQueue(t *testing.T, runDir string, origin model.JobOrigin) {
	t.Helper()
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Origin: &origin}}}
	data, err := json.Marshal(queue)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(runDir, "commands.json"), string(data))
}
