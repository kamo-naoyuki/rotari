package queueops

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Remove removes the selected jobs from the current queue, or from the batch
// restored from requestedRunID. A job that a remaining job depends on is not
// removed.
func (Editor) Remove(baseDir, projectName, requestedRunID string, selector model.CommandSelector) (string, error) {
	return Editor{}.RemoveWithFilter(baseDir, projectName, requestedRunID, selector, jobfilter.Filter{})
}

// RemoveWithFilter removes the jobs selected by selector and further narrowed by
// filter from the current queue or from the batch restored from requestedRunID.
func (Editor) RemoveWithFilter(baseDir, projectName, requestedRunID string, selector model.CommandSelector, filter jobfilter.Filter) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var removed []model.QueuedCommand
	err = project.EditQueue(paths, "remove", func(queue *model.Queue) error {
		if err := restoreSnapshot(paths, requestedRunID, queue); err != nil {
			return err
		}
		if err := checkEditable(*queue, projectName, selector); err != nil {
			return err
		}

		indexes, err := model.SelectCommands(queue.Commands, selector)
		if err != nil {
			return err
		}
		if !filter.Empty() {
			filtered := make([]int, 0, len(indexes))
			for _, index := range indexes {
				if filter.MatchesCommand(queue.Commands[index]) {
					filtered = append(filtered, index)
				}
			}
			indexes = filtered
		}
		removed = make([]model.QueuedCommand, 0, len(indexes))
		for _, index := range indexes {
			removed = append(removed, queue.Commands[index])
		}
		removedNames := make(map[string]bool, len(removed))
		for _, job := range removed {
			if job.Name != "" {
				removedNames[job.Name] = true
			}
		}
		selected := make(map[int]bool, len(indexes))
		for _, index := range indexes {
			selected[index] = true
		}
		remaining := make([]model.QueuedCommand, 0, len(queue.Commands)-len(removed))
		for index, job := range queue.Commands {
			if selected[index] {
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
	return fmt.Sprintf("removed %d job(s) from queue=%s", len(removed), projectName), nil
}
