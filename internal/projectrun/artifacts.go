package projectrun

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/artifactsource"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// artifactRecorder discovers each attempt's artifact candidates when the
// attempt is prepared, before it starts, and records them in the attempt
// directory once it has started. An attempt that never starts gets no
// record. Discovery never changes the job's execution or result.
type artifactRecorder struct {
	runDir string
	// environments holds each job's own environment entries (add --env and
	// matrix values), without the variables the run adds.
	environments map[string][]string
	// variables names the run variables whose attempt values shell source
	// may expand (PATH-E1): the array task ID and the attempt directory.
	variables []string
	store     state.Store
	logf      func(string, ...any)
	// sources parses each configuration file once per version for the run.
	sources *artifactsource.Cache
	pending sync.Map // attempt ID -> artifact.Record
}

func newArtifactRecorder(runDir string, jobs []model.JobSpec, variables []string, store state.Store, logf func(string, ...any)) *artifactRecorder {
	environments := make(map[string][]string, len(jobs))
	for _, job := range jobs {
		environments[job.ID] = append([]string(nil), job.Environment...)
	}
	return &artifactRecorder{runDir: runDir, environments: environments, variables: variables, store: store, logf: logf, sources: artifactsource.NewCache()}
}

// prepare discovers the candidates of job's attempt, whose working directory
// is resolved and whose attempt ID is assigned. Configuration files are read
// on the supervisor's host for every executor, assuming the execution host
// shares the filesystem; a file the supervisor cannot read is a diagnostic.
func (recorder *artifactRecorder) prepare(job model.JobSpec) {
	// Discovery is best-effort; a defect in it must not stop the run.
	defer func() {
		if recovered := recover(); recovered != nil {
			recorder.logf("WARNING: artifact discovery failed for job %s: %v", job.ID, recovered)
		}
	}()
	variables := map[string]string{}
	for _, name := range recorder.variables {
		if entry, ok := run.EnvironmentEntry(job.Environment, name); name != "" && ok {
			variables[name] = strings.TrimPrefix(entry, name+"=")
		}
	}
	result := artifact.Discover(artifact.Job{
		Command:          job.Command,
		Environment:      recorder.environments[job.ID],
		Output:           job.Output,
		Error:            job.Error,
		WorkingDirectory: job.WorkingDirectory,
		Variables:        variables,
	}, artifact.Sources{Config: recorder.sources.References, Script: recorder.sources.Script})
	recorder.pending.Store(job.AttemptID, artifact.Record{Version: artifact.DiscoveryVersion, WorkingDirectory: job.WorkingDirectory, Result: result})
}

// started writes the record prepared for job's attempt.
func (recorder *artifactRecorder) started(job model.JobSpec) {
	value, ok := recorder.pending.LoadAndDelete(job.AttemptID)
	if !ok {
		return
	}
	attemptDir, err := state.AttemptJobDir(recorder.runDir, job)
	if err == nil {
		err = recorder.store.WriteJSON(filepath.Join(attemptDir, state.ArtifactsFileName), value)
	}
	if err != nil {
		recorder.logf("WARNING: failed to record artifact candidates for job %s: %v", job.ID, err)
	}
}
