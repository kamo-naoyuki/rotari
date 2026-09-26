package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Latest selects a project's latest run wherever a run can be given. It is
// reserved: new projects, runs, jobs, stages, and matrices cannot be named
// after it, so a positional "latest" is never ambiguous.
const Latest = "latest"

// ValidateReservedName rejects a new name of kind, such as "project" or "run
// name", that is reserved.
func ValidateReservedName(kind, name string) error {
	if name == Latest {
		return fmt.Errorf("%s %q is reserved for the latest run", kind, name)
	}
	return nil
}

// ValidateReservedNames rejects job, stage, and matrix names of commands
// that are reserved.
func ValidateReservedNames(commands []QueuedCommand) error {
	for _, command := range commands {
		for kind, name := range map[string]string{"job name": command.Name, "stage": command.Stage} {
			if err := ValidateReservedName(kind, name); err != nil {
				return err
			}
		}
		if command.Matrix != nil {
			if err := ValidateReservedName("matrix name", command.Matrix.BaseName); err != nil {
				return err
			}
		}
	}
	return nil
}

// CommandSelector selects queue commands. Callers set one kind of selector:
// job IDs, a job name, a stage, a matrix base name, or all. An array command
// is selected as a whole; its task IDs and names do not select it.
type CommandSelector struct {
	IDs    []string
	Name   string
	Stage  string
	Matrix string
	All    bool
}

// Kinds returns how many kinds of selector are set.
func (selector CommandSelector) Kinds() int {
	count := 0
	for _, set := range []bool{len(selector.IDs) > 0, selector.Name != "", selector.Stage != "", selector.Matrix != "", selector.All} {
		if set {
			count++
		}
	}
	return count
}

// Group reports whether the selector names a group of jobs (a stage, a
// matrix, or all) rather than individual jobs.
func (selector CommandSelector) Group() bool {
	return selector.Stage != "" || selector.Matrix != "" || selector.All
}

// Matches reports whether command is selected.
func (selector CommandSelector) Matches(command QueuedCommand) bool {
	switch {
	case selector.All:
		return true
	case selector.Stage != "":
		return command.Stage == selector.Stage
	case selector.Matrix != "":
		return command.Matrix != nil && command.Matrix.BaseName == selector.Matrix
	case selector.Name != "":
		return command.Name == selector.Name
	}
	for _, id := range selector.IDs {
		if command.ID == id {
			return true
		}
	}
	return false
}

// SelectCommands returns the indexes of the selected commands in queue order.
// It fails when nothing is selected, or when a requested job ID is missing.
func SelectCommands(commands []QueuedCommand, selector CommandSelector) ([]int, error) {
	var indexes []int
	found := make(map[string]bool, len(selector.IDs))
	for index, command := range commands {
		if selector.Matches(command) {
			indexes = append(indexes, index)
			found[command.ID] = true
		}
	}
	var missing []string
	for _, id := range selector.IDs {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(indexes) > 0 && len(missing) == 0 {
		return indexes, nil
	}
	for _, requested := range append(missing, selector.Name) {
		if err := arrayTaskError(commands, requested); err != nil {
			return nil, err
		}
	}
	switch {
	case len(missing) > 0 && len(missing) < len(selector.IDs):
		sort.Strings(missing)
		return nil, fmt.Errorf("one or more jobs not found: %s", strings.Join(missing, ", "))
	case selector.Stage != "":
		return nil, fmt.Errorf("no jobs in stage %q", selector.Stage)
	case selector.Matrix != "":
		return nil, fmt.Errorf("no matrix named %q", selector.Matrix)
	case selector.All:
		return nil, errors.New("no jobs to select")
	default:
		return nil, errors.New("job not found")
	}
}

// arrayTaskError explains that requested names a task of an array command,
// which shares its command's settings and cannot be selected alone.
func arrayTaskError(commands []QueuedCommand, requested string) error {
	if requested == "" {
		return nil
	}
	for _, command := range commands {
		if command.Array == nil {
			continue
		}
		for _, task := range QueueToJobs([]QueuedCommand{command}) {
			if task.ID == requested || task.Name == requested {
				return fmt.Errorf("%s is a task of array job %s; select the array job instead", requested, command.ID)
			}
		}
	}
	return nil
}
