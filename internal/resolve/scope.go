package resolve

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rotari-dev/rotari/internal/basedirregistry"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

// Scope is where a selector is looked for. Pinned, for --basedir or a
// command that runs or changes jobs, keeps the search in BaseDir, the
// command's state directory value. Otherwise BaseDir, from ROTARI_BASEDIR or
// a config file, is only searched first: when nothing matches there, the
// local default state directory and those registered in the master
// directory are searched alike, so a selector matching in more than one of
// them is ambiguous. ExplicitProject reports a project named on the command
// line, which keeps a selector in that project; a project from
// ROTARI_PROJECT_NAME or a config file is only searched first.
type Scope struct {
	BaseDir         string
	Pinned          bool
	ExplicitProject bool
}

// search applies find to scope's preferred state directory and, when that
// finds nothing and scope is not pinned, to every other one alike,
// returning what they find together.
func search[T any](scope Scope, find func(baseDir string) ([]T, error)) ([]T, error) {
	first := ""
	if scope.Pinned || scope.BaseDir != "" {
		baseDir, _, err := state.ResolveBaseDir(scope.BaseDir)
		if err != nil {
			return nil, err
		}
		found, err := find(baseDir)
		if err != nil || len(found) > 0 || scope.Pinned {
			return found, err
		}
		first = baseDir
	}
	baseDirs, err := otherBaseDirs(first)
	if err != nil {
		return nil, err
	}
	var found []T
	for _, baseDir := range baseDirs {
		more, err := find(baseDir)
		if err != nil {
			return nil, err
		}
		found = append(found, more...)
	}
	return found, nil
}

// searchProjects applies find to the projects scope selects. A project named
// on the command line is looked for as ProjectBaseDir does, and find gets
// only it. A project from ROTARI_PROJECT_NAME or a config file is tried
// first in its state directory; when find has nothing there, or without a
// project, find gets every project of the state directories search covers.
func searchProjects[T any](scope Scope, cliProjectName string, find func(baseDir string, projects []string) ([]T, error)) ([]T, error) {
	if scope.Pinned {
		baseDir, _, err := state.ResolveBaseDir(scope.BaseDir)
		if err != nil {
			return nil, err
		}
		projects, err := ProjectNames(baseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		return find(baseDir, projects)
	}
	if state.ProjectNameGiven(cliProjectName) {
		projectName, err := state.ResolveProjectName("", cliProjectName)
		if err != nil {
			return nil, err
		}
		if scope.ExplicitProject {
			baseDir, found, err := ProjectBaseDir(scope, projectName)
			if err != nil {
				return nil, err
			}
			if !found {
				baseDir, _, err := state.ResolveBaseDir(scope.BaseDir)
				if err != nil {
					return nil, err
				}
				return nil, RequireProject(baseDir, projectName)
			}
			return find(baseDir, []string{projectName})
		}
		baseDir, _, err := state.ResolveBaseDir(scope.BaseDir)
		if err != nil {
			return nil, err
		}
		if ProjectExists(baseDir, projectName) {
			found, err := find(baseDir, []string{projectName})
			if err != nil || len(found) > 0 {
				return found, err
			}
		}
	}
	return search(scope, func(baseDir string) ([]T, error) {
		projects, err := ExistingProjectNames(baseDir)
		if err != nil {
			return nil, err
		}
		return find(baseDir, projects)
	})
}

// otherBaseDirs lists, sorted, the local default state directory and those
// registered in the master directory, other than except.
func otherBaseDirs(except string) ([]string, error) {
	local, err := state.ResolveBaseDirDefault()
	if err != nil {
		return nil, err
	}
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		return nil, err
	}
	registered, _, err := basedirregistry.Discover(masterDir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	if except != "" {
		seen[filepath.Clean(except)] = true
	}
	baseDirs := make([]string, 0, len(registered)+1)
	for _, baseDir := range append(registered, local) {
		baseDir = filepath.Clean(baseDir)
		if !seen[baseDir] {
			seen[baseDir] = true
			baseDirs = append(baseDirs, baseDir)
		}
	}
	sort.Strings(baseDirs)
	return baseDirs, nil
}

// ProjectBaseDir finds the state directory of scope that has projectName. It
// reports false when none has it, and fails when several have it.
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
	fmt.Fprintf(&message, "project %q exists in more than one state directory; pass --basedir:", projectName)
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
