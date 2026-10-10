package model

// SourceRevision is the version-control state of a repository that a run's
// jobs executed from, recorded when the run starts executing.
type SourceRevision struct {
	// Root is the repository's top directory.
	Root string `json:"root"`
	// VCS is "git" or "jj".
	VCS string `json:"vcs"`
	// CommitID identifies the code. For jj it is the working-copy commit,
	// taken after a snapshot, so it includes changes not yet described or
	// committed; for git it is HEAD.
	CommitID string `json:"commit_id,omitempty"`
	// ChangeID is jj's change ID of the working-copy commit, which survives
	// rewrites of the same change.
	ChangeID string `json:"change_id,omitempty"`
	// Dirty reports, for git, tracked changes not committed to HEAD; the
	// code that ran then differs from CommitID.
	Dirty bool `json:"dirty,omitempty"`
	// Error says why the revision could not be read; the other fields beyond
	// Root and VCS are then empty.
	Error string `json:"error,omitempty"`
}

// RunSources lists the repositories a run's executed jobs ran from.
type RunSources struct {
	Sources []SourceRevision `json:"sources"`
}

// SourceFor returns the recorded repository containing dir, the one with the
// longest root when repositories nest.
func (sources RunSources) SourceFor(dir string) (SourceRevision, bool) {
	var found SourceRevision
	ok := false
	for _, source := range sources.Sources {
		if pathWithin(dir, source.Root) && (!ok || len(source.Root) > len(found.Root)) {
			found, ok = source, true
		}
	}
	return found, ok
}

func pathWithin(path, root string) bool {
	if path == root {
		return true
	}
	if root == "" || len(path) <= len(root) || path[:len(root)] != root {
		return false
	}
	return root[len(root)-1] == '/' || path[len(root)] == '/'
}

// shortRevisionLength is how many characters of a commit or change ID views
// print.
const shortRevisionLength = 12

func shortRevision(id string) string {
	if len(id) > shortRevisionLength {
		return id[:shortRevisionLength]
	}
	return id
}

// FormatSourceRevision describes a revision for display, such as
// "git 89281f8c3a1b (uncommitted changes)" or
// "jj 1a2b3c4d5e6f (change kxqzmwuotpls)".
func FormatSourceRevision(revision SourceRevision) string {
	if revision.Error != "" {
		return revision.VCS + " unknown (" + revision.Error + ")"
	}
	text := revision.VCS + " " + shortRevision(revision.CommitID)
	if revision.ChangeID != "" {
		text += " (change " + shortRevision(revision.ChangeID) + ")"
	}
	if revision.Dirty {
		text += " (uncommitted changes)"
	}
	return text
}

// SourceLabel describes a revision with its repository root, as run views
// print it: "git 89281f8c3a1b (uncommitted changes) in /home/me/project".
func SourceLabel(revision SourceRevision) string {
	return FormatSourceRevision(revision) + " in " + revision.Root
}
