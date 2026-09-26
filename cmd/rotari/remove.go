package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdRemove removes jobs from the current queue or prepares a filtered
// follow-up run from historical commands.
func cmdRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	jobName := cliString(fs, "job-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if (len(fs.Args()) > 0 && (*jobName != "" || len(jobIDs) > 0)) ||
		(*jobName == "" && len(jobIDs) == 0 && len(fs.Args()) == 0) ||
		(*jobName != "" && len(jobIDs) > 0) {
		printError("usage: " + cliUsage("remove"))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}

	baseDir, queueName, err := resolve.ExistingRun(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := removeBatch(baseDir, queueName, *runID, jobIDs, *jobName)
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}

func removeBatch(baseDir, queueName, requestedRunID string, requestedJobIDs []string, requestedJobName string) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	var removed []model.QueuedCommand
	err = project.EditQueue(paths, "remove", func(queue *model.Queue) error {
		if len(queue.Commands) == 0 || requestedRunID != "" {
			snapshot, err := loadChangeSnapshot(paths, requestedRunID)
			if err != nil {
				return err
			}
			*queue = snapshot
		}

		removeIDs := make(map[string]bool, len(requestedJobIDs))
		for _, id := range requestedJobIDs {
			removeIDs[id] = true
		}
		foundIDs := make(map[string]bool, len(requestedJobIDs))
		removed = make([]model.QueuedCommand, 0, len(queue.Commands))
		for _, job := range queue.Commands {
			if removeIDs[job.ID] {
				foundIDs[job.ID] = true
				removed = append(removed, job)
			} else if requestedJobName != "" && job.Name == requestedJobName {
				removed = append(removed, job)
			}
		}
		if len(removed) == 0 {
			return fmt.Errorf("job not found")
		}
		if len(requestedJobIDs) > 0 && len(foundIDs) != len(removeIDs) {
			return fmt.Errorf("one or more jobs not found")
		}
		removedNames := make(map[string]bool, len(removed))
		for _, job := range removed {
			if job.Name != "" {
				removedNames[job.Name] = true
			}
		}
		remaining := make([]model.QueuedCommand, 0, len(queue.Commands)-len(removed))
		for _, job := range queue.Commands {
			if removeIDs[job.ID] || (requestedJobName != "" && job.Name == requestedJobName) {
				continue
			}
			for _, dependency := range job.AllDependencies() {
				if removedNames[dependency] {
					return fmt.Errorf("job %q is referenced by dependency; remove is not allowed", dependency)
				}
			}
			remaining = append(remaining, job)
		}
		queue.Commands = remaining
		model.ClearIncompleteMatrixGroups(queue.Commands)
		if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
			return fmt.Errorf("invalid dependencies: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("removed %d job(s) from queue=%s", len(removed), queueName), nil
}
