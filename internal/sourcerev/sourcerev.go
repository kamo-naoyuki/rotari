// Package sourcerev reads the version-control revision of the directory a
// job runs from, so a run can record which code it executed. It supports
// jj and git; a jj repository colocated with git is read as jj, because
// git's HEAD there is the working-copy commit's parent.
package sourcerev

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Timeout bounds each version-control command.
const Timeout = 30 * time.Second

// Read returns the revision of the repository containing dir, and false when
// dir is in no jj or git repository. Reading a jj repository snapshots its
// working copy, which records one jj operation, so the commit ID covers the
// files as they are now. A failure to read the revision is reported in the
// result's Error rather than dropped, so the record says the code is unknown.
func Read(dir string) (model.SourceRevision, bool) {
	root, vcs, ok := findRoot(dir)
	if !ok {
		return model.SourceRevision{}, false
	}
	revision := model.SourceRevision{Root: root, VCS: vcs}
	var err error
	switch vcs {
	case "jj":
		err = readJJ(&revision)
	default:
		err = readGit(&revision)
	}
	if err != nil {
		revision = model.SourceRevision{Root: root, VCS: vcs, Error: err.Error()}
	}
	return revision, true
}

// findRoot walks up from dir to the nearest directory holding .jj or .git.
// A directory holding both is jj's.
func findRoot(dir string) (string, string, bool) {
	dir = filepath.Clean(dir)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".jj")); err == nil && info.IsDir() {
			return dir, "jj", true
		}
		// .git is a file in a git worktree or submodule.
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, "git", true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}
		dir = parent
	}
}

func readJJ(revision *model.SourceRevision) error {
	output, err := command(revision.Root, "jj", "--repository", revision.Root, "--no-pager", "--color", "never",
		"log", "--no-graph", "-r", "@", "-T", `commit_id ++ " " ++ change_id ++ "\n"`)
	if err != nil {
		return err
	}
	fields := strings.Fields(output)
	if len(fields) != 2 {
		return fmt.Errorf("unexpected jj output %q", output)
	}
	revision.CommitID, revision.ChangeID = fields[0], fields[1]
	return nil
}

func readGit(revision *model.SourceRevision) error {
	head, err := command(revision.Root, "git", "-C", revision.Root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	status, err := command(revision.Root, "git", "-C", revision.Root, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	revision.CommitID = strings.TrimSpace(head)
	revision.Dirty = strings.TrimSpace(status) != ""
	return nil
}

func command(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		if line, _, found := strings.Cut(message, "\n"); found {
			message = line
		}
		return "", fmt.Errorf("%s: %s", name, message)
	}
	return stdout.String(), nil
}

// ReadAll reads the repositories containing dirs, once per repository, in
// the order their first directory appears.
func ReadAll(dirs []string) model.RunSources {
	sources := model.RunSources{Sources: []model.SourceRevision{}}
	seen := make(map[string]bool)
	for _, dir := range dirs {
		if _, ok := sources.SourceFor(dir); ok {
			continue
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if revision, ok := Read(dir); ok {
			sources.Sources = append(sources.Sources, revision)
		}
	}
	return sources
}
