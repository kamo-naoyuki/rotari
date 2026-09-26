package queueops

import (
	"errors"
	"fmt"
	"os"

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
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return "", err
	}
	err = project.EditQueue(paths, "add", func(queue *model.Queue) error {
		for index := range commands {
			if commands[index].Executor != "" && !editor.Executors.Known(commands[index].Executor) {
				return fmt.Errorf("unsupported executor: %s", commands[index].Executor)
			}
			commands[index].ID = editor.NewJobID()
			if queue.WorkflowImport {
				commands[index].Force = true
			}
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
		queue.Commands = append(queue.Commands, commands...)
		return nil
	})
	if err != nil {
		return "", err
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
