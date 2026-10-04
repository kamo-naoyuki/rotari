package jobstatus

import (
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Artifacts returns the artifact candidates recorded for the attempt that
// origin names under runsDir. A job carried into origin's run is read from
// the attempt that produced its result, as FilterJob reads it. ok is false
// when that attempt has no record, as in state written before discovery
// existed or for an attempt that never started: discovery information is
// unavailable, which does not mean the job had no associated files.
func Artifacts(store state.Store, runsDir string, origin model.JobOrigin) (artifact.Record, bool) {
	attemptDir, _ := locateAttempt(runsDir, origin, 0)
	if attemptDir == "" {
		return artifact.Record{}, false
	}
	var record artifact.Record
	if err := store.ReadJSON(filepath.Join(attemptDir, state.ArtifactsFileName), &record); err != nil {
		return artifact.Record{}, false
	}
	return record, true
}

// Types an ArtifactEntry observes at its path on the viewing host.
const (
	ArtifactFile      = "file"
	ArtifactDirectory = "directory"
	ArtifactOther     = "other"
	ArtifactMissing   = "missing"
	// ArtifactUnknown is a relative path whose base was never known.
	ArtifactUnknown = "unknown"
)

// ArtifactListing is an attempt's artifact candidates as the CLI, JSON, and
// Web UI show them.
type ArtifactListing struct {
	// Recorded is false when the attempt has no record: discovery
	// information is unavailable, unlike a record with no candidates.
	Recorded bool `json:"recorded"`
	Version  int  `json:"version,omitempty"`
	// WorkingDirectory is the directory relative references were resolved
	// on, when the record names it.
	WorkingDirectory string                `json:"working_directory,omitempty"`
	Entries          []ArtifactEntry       `json:"entries,omitempty"`
	Diagnostics      []artifact.Diagnostic `json:"diagnostics,omitempty"`
}

// ArtifactEntry is one candidate with what the viewing host finds at its
// path now. A missing path may exist on another host, such as an SSH job's.
type ArtifactEntry struct {
	Path  string `json:"path"`
	Basis string `json:"basis"`
	Type  string `json:"type"`
	// Origin is a one-line account of where the candidate was found.
	Origin  string            `json:"origin"`
	Sources []artifact.Source `json:"sources"`
}

// ListArtifacts returns the listing of the attempt origin names, read like
// Artifacts, with each path observed now. It follows symlinks and never
// opens a path.
func ListArtifacts(store state.Store, runsDir string, origin model.JobOrigin) ArtifactListing {
	record, ok := Artifacts(store, runsDir, origin)
	if !ok {
		return ArtifactListing{}
	}
	listing := ArtifactListing{Recorded: true, Version: record.Version, WorkingDirectory: record.WorkingDirectory, Diagnostics: record.Diagnostics}
	for _, candidate := range record.Candidates {
		listing.Entries = append(listing.Entries, ArtifactEntry{
			Path: candidate.Path, Basis: candidate.Basis, Type: observeArtifact(candidate),
			Origin: artifact.Describe(candidate.Sources), Sources: candidate.Sources,
		})
	}
	return listing
}

func observeArtifact(candidate artifact.Candidate) string {
	if candidate.Basis == artifact.BasisUnresolved {
		return ArtifactUnknown
	}
	info, err := os.Stat(candidate.Path)
	switch {
	case err != nil:
		return ArtifactMissing
	case info.Mode().IsRegular():
		return ArtifactFile
	case info.IsDir():
		return ArtifactDirectory
	}
	return ArtifactOther
}
