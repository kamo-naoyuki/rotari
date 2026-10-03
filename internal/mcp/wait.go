package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	defaultWaitSeconds = 30
	maxWaitSeconds     = 300
	waitPoll           = 500 * time.Millisecond
)

type WaitRunInput struct {
	RunID          string `json:"run_id" jsonschema:"exact run ID, such as rotari_start_run returns"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"how long to wait, 1 to 300 seconds; default 30. Call again to keep waiting."`
	UntilFailure   bool   `json:"until_failure,omitempty" jsonschema:"also return as soon as a job has failed with no retry left, as rotari wait --until-failure does"`
	AllJobs        bool   `json:"all_jobs,omitempty" jsonschema:"list every job of each failure group, as rotari_run_summary does with all_jobs"`
}

type WaitRunOutput struct {
	RunSummaryOutput
	Reason string `json:"reason" jsonschema:"why the wait returned: settled (the run is no longer running), failure (with until_failure, a job failed while the run goes on), or timeout (still running)"`
}

// waitRun waits, as `rotari wait` does but for at most a bounded time, until
// runID is no longer running or, with UntilFailure, one of its jobs has
// failed with no retry left. It then returns the run's summary, as
// rotari_run_summary does, and why it returned.
func waitRun(ctx context.Context, masterDir string, input WaitRunInput) (WaitRunOutput, error) {
	seconds := input.TimeoutSeconds
	if seconds == 0 {
		seconds = defaultWaitSeconds
	}
	if seconds < 1 || seconds > maxWaitSeconds {
		return WaitRunOutput{}, fmt.Errorf("timeout_seconds must be 1 to %d", maxWaitSeconds)
	}
	location, paths, err := registeredRun(masterDir, input.RunID)
	if err != nil {
		return WaitRunOutput{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for {
		phase, err := project.RunPhaseOf(paths, location.RunID)
		if err != nil {
			return WaitRunOutput{}, err
		}
		reason := ""
		switch {
		case phase != project.RunPhaseRunning:
			reason = "settled"
		case input.UntilFailure:
			// A run that has not written its jobs yet has no failures.
			if groups, err := runview.FinalFailureGroups(paths, location.RunID, store); err == nil && len(groups) > 0 {
				reason = "failure"
			}
		}
		if reason == "" && !time.Now().Before(deadline) {
			reason = "timeout"
		}
		if reason != "" {
			summary, err := runSummary(masterDir, RunSummaryInput{RunID: location.RunID, AllJobs: input.AllJobs})
			return WaitRunOutput{RunSummaryOutput: summary, Reason: reason}, err
		}
		select {
		case <-ctx.Done():
			return WaitRunOutput{}, ctx.Err()
		case <-time.After(waitPoll):
		}
	}
}
