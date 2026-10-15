package resolve

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Scope is where a selector without a project is looked for. BaseDir is the
// command's state directory value, from --basedir, ROTARI_BASEDIR, or a
// config file, or "" for the local default. A selector is looked for there
// first and, when nothing matches there and --basedir did not pin it, in the
// other state directories registered in the master directory.
type Scope struct {
	BaseDir string
	Pinned  bool
}

// forProject pins scope when a project is chosen: only a selector without
// one leaves the state directory.
func (scope Scope) forProject(cliProjectName string) Scope {
	if state.ProjectNameGiven(cliProjectName) {
		scope.Pinned = true
	}
	return scope
}

// search applies find to scope's state directory and, when that finds
// nothing and scope is not pinned, to every other registered state
// directory, returning what they find together.
func search[T any](scope Scope, find func(baseDir string) ([]T, error)) ([]T, error) {
	baseDir, _, err := state.ResolveBaseDir(scope.BaseDir)
	if err != nil {
		return nil, err
	}
	found, err := find(baseDir)
	if err != nil || len(found) > 0 || scope.Pinned {
		return found, err
	}
	others, err := otherBaseDirs(baseDir)
	if err != nil {
		return nil, err
	}
	for _, other := range others {
		more, err := find(other)
		if err != nil {
			return nil, err
		}
		found = append(found, more...)
	}
	return found, nil
}

// otherBaseDirs lists, sorted, the state directories registered in the
// master directory other than except.
func otherBaseDirs(except string) ([]string, error) {
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		return nil, err
	}
	registered, _, err := basedirregistry.Discover(masterDir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{filepath.Clean(except): true}
	others := make([]string, 0, len(registered))
	for _, baseDir := range registered {
		baseDir = filepath.Clean(baseDir)
		if !seen[baseDir] {
			seen[baseDir] = true
			others = append(others, baseDir)
		}
	}
	sort.Strings(others)
	return others, nil
}

// ProjectBaseDir finds the state directory of scope that has projectName,
// for a positional selector that may name a project. It reports false when
// none has it, and fails when several registered ones, but not scope's own,
// have it.
func ProjectBaseDir(scope Scope, projectName string) (string, bool, error) {
	if !state.IsValidPathElement(projectName) {
		return "", false, nil
	}
	found, err := search(scope, func(baseDir string) ([]string, error) {
		if ProjectExists(baseDir, projectName) {
			return []string{baseDir}, nil
		}
		return nil, nil
	})
	switch {
	case err != nil || len(found) == 0:
		return "", false, err
	case len(found) == 1:
		return found[0], true, nil
	}
	var message strings.Builder
	fmt.Fprintf(&message, "project %q exists in more than one registered state directory; pass --basedir:", projectName)
	for _, baseDir := range found {
		fmt.Fprintf(&message, "\n  basedir=%s%s", strconv.Quote(baseDir), lastRunNote(baseDir, projectName))
	}
	return "", false, errors.New(message.String())
}

// QueueWithJobs finds the project whose non-empty queue holds the commands
// selector names by job ID or name, for queue edits without a project. It
// reports false when selector names a group or no queue holds them, leaving
// project resolution to report it.
func QueueWithJobs(scope Scope, selector model.CommandSelector) (Run, bool, error) {
	if len(selector.IDs) == 0 && selector.Name == "" {
		return Run{}, false, nil
	}
	targets, err := search(scope, func(baseDir string) ([]Job, error) {
		return queuesWithJobs(baseDir, selector)
	})
	if err != nil {
		return Run{}, false, err
	}
	switch len(targets) {
	case 0:
		return Run{}, false, nil
	case 1:
		return targets[0].Run, true, nil
	}
	what := fmt.Sprintf("job %q", strings.Join(selector.IDs, ","))
	if selector.Name != "" {
		what = fmt.Sprintf("job name %q", selector.Name)
	}
	return Run{}, false, AmbiguousError(what, targets)
}

func queuesWithJobs(baseDir string, selector model.CommandSelector) ([]Job, error) {
	projects, err := ExistingProjectNames(baseDir)
	if err != nil {
		return nil, err
	}
	var targets []Job
	for _, projectName := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil || len(queue.Commands) == 0 {
			continue
		}
		if indexes, err := model.SelectCommands(queue.Commands, selector); err == nil {
			targets = append(targets, Job{Run: Run{BaseDir: baseDir, ProjectName: projectName}, JobID: queue.Commands[indexes[0]].ID, FromQueue: true})
		}
	}
	return targets, nil
}
