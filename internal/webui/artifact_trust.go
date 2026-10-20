package webui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

// artifactRecordsTrust reports whether another user could have decided what
// a run's artifact listing names. A job's definition (queue.json), its
// recorded working directory, and its recorded candidates come from files
// under the project, so a user who can write there can point a listing at
// any path, including this Web server user's files. The listing's roots
// may include its working directory and the basedir's parent only when
// every element below is owned by this user or root and is not writable by
// group or others. Only permission bits are read; write access granted by
// an ACL is not seen.
func artifactRecordsTrust(baseDir, projectName, runID string) (bool, string) {
	paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return false, err.Error()
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return false, err.Error()
	}
	uid := os.Getuid()
	for _, path := range []string{paths.BaseDir, filepath.Dir(paths.ProjectDir), paths.ProjectDir,
		paths.QueueFile, paths.MetaFile, paths.RunsDir, runDir} {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) && (path == paths.QueueFile || path == paths.MetaFile) {
			continue
		}
		if err != nil {
			return false, err.Error()
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return false, path + ": cannot read its owner"
		}
		if owner := int(stat.Uid); owner != uid && owner != 0 {
			return false, fmt.Sprintf("%s is owned by another user (uid %d)", path, owner)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return false, fmt.Sprintf("%s is writable by group or others", path)
		}
	}
	return true, ""
}

// basedirParentRoot is the directory holding a basedir, which is served for
// its trusted listings so outputs beside the state directory can be opened.
// The filesystem root is never one.
func basedirParentRoot(baseDir string) (string, bool) {
	if !filepath.IsAbs(baseDir) {
		return "", false
	}
	parent := filepath.Dir(filepath.Clean(baseDir))
	return parent, parent != filepath.Dir(parent)
}

// artifactRootsNote explains, for /api/artifacts, why an untrusted listing
// is served only from --artifact-root.
func artifactRootsNote(reason string) string {
	return "Only --artifact-root directories are served for this job, not its working directory or the basedir's parent, because another user could have chosen its paths: " + reason + "."
}
