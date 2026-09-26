package queueops

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ValidateJobs checks every queued command on its own and the job IDs they
// expand to: matrix groups, stage names, IDs as path elements, commands,
// environment, working directory, timeout, retry settings, arrays, and
// duplicate expanded IDs. Dependencies are checked separately by
// model.ValidateQueueDependencies, because add accepts dependencies on jobs
// added later.
func ValidateJobs(queue model.Queue) error {
	if err := model.ValidateMatrixGroups(queue.Commands); err != nil {
		return err
	}
	if err := model.ValidateStageNames(queue.Commands); err != nil {
		return err
	}
	expandedIDs := make(map[string]bool)
	for _, command := range queue.Commands {
		if !state.IsValidPathElement(command.ID) {
			return fmt.Errorf("invalid job ID %q", command.ID)
		}
		if len(command.Command) == 0 || command.Command[0] == "" {
			return fmt.Errorf("job %q has an empty command", command.ID)
		}
		for _, argument := range command.Command {
			if strings.ContainsRune(argument, '\x00') {
				return fmt.Errorf("job %q command contains a NUL byte", command.ID)
			}
		}
		if err := model.ValidateEnvironment(command.Environment); err != nil {
			return fmt.Errorf("job %q has invalid environment: %w", command.ID, err)
		}
		if strings.ContainsRune(command.WorkingDirectory, '\x00') {
			return fmt.Errorf("job %q working directory contains a NUL byte", command.ID)
		}
		if command.Timeout != "" {
			if _, err := model.ParseTimeout(command.Timeout); err != nil {
				return fmt.Errorf("job %q: %w", command.ID, err)
			}
		}
		if command.Retry != nil && *command.Retry < 0 {
			return fmt.Errorf("job %q has a negative retry limit", command.ID)
		}
		if err := model.ValidateRetryBackoff(command.RetryDelay, command.RetryBackoff, command.RetryMaxDelay); err != nil {
			return fmt.Errorf("job %q: %w", command.ID, err)
		}
		if command.Array != nil {
			if err := model.ValidateArraySpec(command.Array); err != nil {
				return fmt.Errorf("job %q has invalid array: %w", command.ID, err)
			}
		}
		jobIDs := []string{command.ID}
		if command.Array != nil {
			jobIDs = make([]string, 0, len(model.ArrayTaskIDs(command.Array)))
			for _, task := range model.ArrayTaskIDs(command.Array) {
				jobIDs = append(jobIDs, fmt.Sprintf("%s-%d", command.ID, task))
			}
		}
		for _, jobID := range jobIDs {
			if expandedIDs[jobID] {
				return fmt.Errorf("duplicate job ID %q", jobID)
			}
			expandedIDs[jobID] = true
		}
	}
	return nil
}
