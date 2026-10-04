package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// shownArtifacts is how many artifact candidates `show` of one job lists
// before pointing to --artifacts.
const shownArtifacts = 20

// showJobArtifacts prints every artifact candidate of one job attempt and
// the discovery diagnostics, for `show -j JOB --artifacts`. A carried job's
// candidates are those of the attempt that produced its result.
func showJobArtifacts(writer io.Writer, paths state.ProjectPaths, runID, jobID, attemptID string) int {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printError(err)
		return 1
	}
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		printErrorf("failed to read commands: %v", err)
		return 1
	}
	found := false
	for _, job := range model.QueueToJobs(queue.Commands) {
		found = found || job.ID == jobID
	}
	if !found {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	writeShowTargetHeaderWithMode(writer, paths, "run job")
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Run:"), runID, cyan("Job:"), jobID)
	if attemptID != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Attempt ID:"), attemptID)
	}
	listing := jobstatus.ListArtifacts(jsonStore(), paths.RunsDir, model.JobOrigin{RunID: runID, JobID: jobID, AttemptID: attemptID})
	writeArtifactListing(writer, listing, 0, "")
	return 0
}

// writeArtifactListing prints a listing: one line per candidate with what
// is at its path now, the path, and where it was found. With a limit, the
// rest are counted and more names the command that lists them; without
// one, the discovery diagnostics follow.
func writeArtifactListing(writer io.Writer, listing jobstatus.ArtifactListing, limit int, more string) {
	switch {
	case !listing.Recorded:
		fmt.Fprintf(writer, "%s (not recorded)\n", cyan("Artifacts:"))
		return
	case len(listing.Entries) == 0:
		fmt.Fprintf(writer, "%s none found\n", cyan("Artifacts:"))
	default:
		if listing.WorkingDirectory != "" {
			fmt.Fprintf(writer, "%s relative to %s\n", cyan("Artifacts:"), listing.WorkingDirectory)
		} else {
			fmt.Fprintln(writer, cyan("Artifacts:"))
		}
		shown := listing.Entries
		if limit > 0 && len(shown) > limit {
			shown = shown[:limit]
		}
		for _, entry := range shown {
			fmt.Fprintf(writer, "  %-9s  %s  (%s)\n", entry.Type, entry.DisplayPath, entry.Origin)
		}
		if hidden := len(listing.Entries) - len(shown); hidden > 0 {
			fmt.Fprintf(writer, "  ... and %d more: %s\n", hidden, more)
		}
	}
	if limit == 0 && len(listing.Diagnostics) > 0 {
		fmt.Fprintln(writer, cyan("Discovery notes:"))
		for _, diagnostic := range listing.Diagnostics {
			if diagnostic.Source != "" {
				fmt.Fprintf(writer, "  %s: %s\n", diagnostic.Source, diagnostic.Message)
			} else {
				fmt.Fprintf(writer, "  %s\n", diagnostic.Message)
			}
		}
	}
}
