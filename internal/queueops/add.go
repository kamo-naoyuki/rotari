package queueops

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Add appends commands to the project queue with new job IDs. A non-nil
// array makes every command an array job.
func (editor Editor) Add(baseDir, projectName string, commands []model.QueuedCommand, array *model.ArraySpec) (string, error) {
	if projectName == "" || len(commands) == 0 {
		return "", errors.New("project name and command are required")
	}
	for index := range commands {
		if array != nil {
			commands[index].Array = array
		}
		if err := model.ValidateEnvironment(commands[index].Environment); err != nil {
			return "", fmt.Errorf("invalid environment: %w", err)
		}
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var warnings []string
	err = project.CreateQueueGuarded(paths, editor.Guard, func(queue *model.Queue) error {
		for index := range commands {
			if commands[index].Executor != "" && !editor.Executors.Known(commands[index].Executor) {
				return fmt.Errorf("unsupported executor: %s", commands[index].Executor)
			}
			commands[index].ID = editor.NewJobID()
		}
		if err := model.ValidateReservedNames(commands); err != nil {
			return err
		}
		if err := ValidateJobs(model.Queue{Commands: append(append([]model.QueuedCommand(nil), queue.Commands...), commands...)}); err != nil {
			return err
		}
		// Dependencies may refer to jobs added later, so only duplicate names are checked here.
		for _, command := range commands {
			if command.Name != "" {
				for _, existing := range queue.Commands {
					if existing.Name == command.Name {
						return fmt.Errorf("invalid dependencies: duplicate job name: %s", command.Name)
					}
				}
			}
		}
		if editor.Warn != nil {
			var err error
			warnings, err = duplicateAddWarnings(*queue, commands)
			if err != nil {
				return err
			}
			warnings = append(warnings, unexpandedVariableWarning(commands)...)
		}
		queue.Commands = append(queue.Commands, commands...)
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := editor.registerBaseDir(paths.BaseDir); err != nil {
		return "", err
	}
	for _, warning := range warnings {
		editor.Warn(warning)
	}
	message := fmt.Sprintf("added project=%s jobs=%d", projectName, len(commands))
	if len(commands) == 1 {
		message = fmt.Sprintf("added project=%s job_id=%s", projectName, commands[0].ID)
		if commands[0].Name != "" {
			message += fmt.Sprintf(" job_name=%s", commands[0].Name)
		}
	}
	return fmt.Sprintf("%s command=%v", message, commands[0].Command), nil
}

// duplicateAddWarnings compares execution units using the same fingerprints
// as run matching. Only groups touched by this add are reported, once each,
// in added-unit order. Existing, unrelated duplicates are left alone.
func duplicateAddWarnings(queue model.Queue, commands []model.QueuedCommand) ([]string, error) {
	added, err := model.QueueFingerprintJobs(model.Queue{Commands: commands})
	if err != nil {
		return nil, err
	}
	combined := model.Queue{Commands: append(append([]model.QueuedCommand(nil), queue.Commands...), commands...)}
	jobs, err := model.QueueFingerprintJobs(combined)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string)
	for _, job := range model.QueueToJobs(combined.Commands) {
		names[job.ID] = job.Name
	}
	groups := make(map[string][]string)
	for _, job := range jobs {
		label := fmt.Sprintf("job_id=%s", job.ID)
		if names[job.ID] != "" {
			label += fmt.Sprintf(" job_name=%q", names[job.ID])
		}
		groups[job.Fingerprint] = append(groups[job.Fingerprint], label)
	}
	var warnings []string
	seen := make(map[string]bool)
	for _, job := range added {
		if group := groups[job.Fingerprint]; len(group) > 1 && !seen[job.Fingerprint] {
			warnings = append(warnings, "warning: the same command is queued more than once. Did you accidentally add it twice? "+strings.Join(group, "; "))
			seen[job.Fingerprint] = true
		}
	}
	return warnings, nil
}
