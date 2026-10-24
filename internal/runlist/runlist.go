// Package runlist collects the runs that `rotari runs` lists, across state
// directories: which runs are listed, their lifecycle and client status, and
// their order. It returns recorded values; callers format them.
package runlist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/project"
	"github.com/rotari-dev/rotari/internal/runview"
	"github.com/rotari-dev/rotari/internal/state"
)

// Run is one listed run.
type Run struct {
	BaseDir string
	Project string
	RunID   string
	RunName string
	// Lifecycle is runview.RunLifecycleLabel: running, interrupted,
	// incomplete, finished, or failed.
	Lifecycle    string
	ClientStatus model.RunClientStatus
	ClientLabel  string
	// ExitCode is nil until the run has a summary.
	ExitCode *int
	// StartedAt and FinishedAt are as recorded (RFC 3339), empty when
	// unknown; Started and Finished are their parsed times.
	StartedAt  string
	FinishedAt string
	Started    time.Time
	Finished   time.Time
	// Order sorts runs newest first: the start time, or the run
	// directory's modification time when the start is unknown.
	Order int64
}

// Collect lists the runs of every project in baseDirs, or of projectFilter
// only, newest first. A projectFilter found in none of them is an error,
// unless no basedir has any project.
func Collect(baseDirs []string, projectFilter string) ([]Run, error) {
	return collect(baseDirs, projectFilter, nil)
}

// CollectBetween is Collect for the runs whose span may meet [from, to),
// as Overlapping decides; callers still apply Overlapping. It skips, without
// reading their files, runs whose run ID says they started at or after to
// and runs whose directory last changed before from (a run writes its
// summary there when it ends), except the project's running or interrupted
// run. A run whose ID is not a timestamp is judged by its directory's time
// alone.
func CollectBetween(baseDirs []string, projectFilter string, from, to time.Time) ([]Run, error) {
	return collect(baseDirs, projectFilter, &window{from: from, to: to})
}

type window struct{ from, to time.Time }

func collect(baseDirs []string, projectFilter string, bounds *window) ([]Run, error) {
	if projectFilter != "" && !state.IsValidPathElement(projectFilter) {
		return nil, fmt.Errorf("invalid project name %q", projectFilter)
	}
	runs := make([]Run, 0)
	hasProjects, foundProject := false, false
	for _, baseDir := range baseDirs {
		entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list projects in %q: %w", baseDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
				continue
			}
			hasProjects = true
			if projectFilter != "" && entry.Name() != projectFilter {
				continue
			}
			foundProject = true
			projectRuns, err := collectProject(baseDir, entry.Name(), bounds)
			if err != nil {
				return nil, err
			}
			runs = append(runs, projectRuns...)
		}
	}
	// Keep the empty-state-directory result, but distinguish a misspelled
	// selector from an existing project that simply has no runs yet.
	if projectFilter != "" && hasProjects && !foundProject {
		return nil, fmt.Errorf("project %q not found in %s", projectFilter, DescribeScope(baseDirs, ""))
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Order != runs[j].Order {
			return runs[i].Order > runs[j].Order
		}
		if runs[i].BaseDir != runs[j].BaseDir {
			return runs[i].BaseDir < runs[j].BaseDir
		}
		if runs[i].Project != runs[j].Project {
			return runs[i].Project < runs[j].Project
		}
		return runs[i].RunID < runs[j].RunID
	})
	return runs, nil
}

func collectProject(baseDir, projectName string, bounds *window) ([]Run, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project %q: %w", projectName, err)
	}
	entries, err := os.ReadDir(paths.RunsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read runs for project %q: %w", projectName, err)
	}
	// The project is inspected once, before any run's summary is read, as
	// project.RunPhaseOf reads them for one run.
	inspection, err := project.Inspect(paths)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect project %q: %w", projectName, err)
	}
	current := ""
	if inspection.State == project.Running || inspection.State == project.Interrupted {
		current = inspection.RunID
	}
	selected := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if bounds != nil && entry.Name() != current && !bounds.mayMeet(entry) {
			continue
		}
		selected = append(selected, entry)
	}
	runs := make([]Run, len(selected))
	errs := make([]error, len(selected))
	state.ReadEach(len(selected), func(index int) {
		runs[index], errs[index] = readRun(paths, projectName, selected[index], inspection)
	})
	for index, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("failed to read run %q for project %q: %w", selected[index].Name(), projectName, err)
		}
	}
	return runs, nil
}

// readRun reads one run, its summary once, deciding its lifecycle and client
// status with the shared rules (project.PhaseFrom, runview.LifecycleLabel,
// runview.ClientStatusIn) from the project's inspection.
func readRun(paths state.ProjectPaths, projectName string, entry os.DirEntry, inspection project.Inspection) (Run, error) {
	runID := entry.Name()
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return Run{}, err
	}
	run := Run{BaseDir: paths.BaseDir, Project: projectName, RunID: runID}
	if info, _ := entry.Info(); info != nil {
		run.Order = info.ModTime().UnixNano()
	}
	summary, err := state.LoadRunSummaryHead(filepath.Join(runDir, "summary.json"))
	if err != nil && !os.IsNotExist(err) {
		return Run{}, err
	}
	if err == nil {
		run.RunName = summary.RunName
		run.ExitCode = &summary.ExitCode
		run.StartedAt, run.FinishedAt = summary.StartedAt, summary.FinishedAt
	} else if lock, lockErr := state.LoadLock(paths.LockFile); lockErr == nil && lock.RunID == runID {
		// An active or interrupted run has no summary yet; its lock records
		// the name and start time.
		run.RunName = lock.RunName
		run.StartedAt = lock.StartedAt
	}
	run.Started, _ = time.Parse(time.RFC3339Nano, run.StartedAt)
	run.Finished, _ = time.Parse(time.RFC3339Nano, run.FinishedAt)
	if !run.Started.IsZero() {
		run.Order = run.Started.UnixNano()
	}
	phase := project.PhaseFrom(inspection, runID, run.ExitCode != nil)
	run.Lifecycle = runview.LifecycleLabel(phase, summary)
	run.ClientStatus, err = runview.ClientStatusIn(paths, runID, phase)
	if err != nil {
		run.ClientStatus = model.RunClientStatus{State: "unknown"}
	}
	run.ClientLabel = runview.ClientStatusLabel(run.ClientStatus)
	return run, nil
}

// Recent keeps the runs `rotari runs` shows: running, interrupted, and
// incomplete runs, runs with no recorded finish, and runs that finished at
// or after cutoff.
func Recent(runs []Run, cutoff time.Time) []Run {
	recent := make([]Run, 0, len(runs))
	for _, run := range runs {
		if run.Lifecycle == "running" || run.Lifecycle == "interrupted" || run.Lifecycle == "incomplete" || run.Finished.IsZero() || !run.Finished.Before(cutoff) {
			recent = append(recent, run)
		}
	}
	return recent
}

// DescribeScope names the listed state directories, and the project when
// one was selected, for messages.
func DescribeScope(baseDirs []string, projectFilter string) string {
	scope := fmt.Sprintf("%d state directories", len(baseDirs))
	if len(baseDirs) == 1 {
		scope = "state directory " + baseDirs[0]
	}
	if projectFilter != "" {
		scope = fmt.Sprintf("project %q in %s", projectFilter, scope)
	}
	return scope
}

// Details are the facts shown beside a listed run, read from its context,
// sources, and notes files: where it ran, which code it ran, and what was
// noted about it. A file the run did not write leaves its facts empty.
type Details struct {
	WorkingDirectory string
	Host             string
	// Sources are the recorded repositories' labels (model.SourceLabel).
	Sources []string
	// NoteCount counts the run's own notes, not its jobs'; FirstNote is the
	// first line of the earliest.
	NoteCount int
	FirstNote string
	// StartedBy is how the run was started, nil when not recorded.
	StartedBy *model.StartedBy
}

// ReadDetails reads run's details. Callers read them only for the runs they
// show, so listing many old runs stays cheap.
func ReadDetails(run Run) (Details, error) {
	paths, err := state.ResolveProjectPaths(run.BaseDir, run.Project)
	if err != nil {
		return Details{}, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, run.RunID)
	if err != nil {
		return Details{}, err
	}
	var details Details
	context, err := state.LoadContext(state.NewStore(state.DirectoryMode(), state.FileMode()), runDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Details{}, fmt.Errorf("failed to read the context of run %q: %w", run.RunID, err)
	}
	details.WorkingDirectory, details.Host = context.CWD, context.Hostname
	details.StartedBy = context.StartedBy
	sources, _, err := state.LoadRunSources(runDir)
	if err != nil {
		return Details{}, fmt.Errorf("failed to read the sources of run %q: %w", run.RunID, err)
	}
	for _, source := range sources.Sources {
		details.Sources = append(details.Sources, model.SourceLabel(source))
	}
	notes, err := state.LoadRunNotes(runDir)
	if err != nil {
		return Details{}, fmt.Errorf("failed to read the notes of run %q: %w", run.RunID, err)
	}
	for _, note := range notes {
		if note.JobID != "" {
			continue
		}
		if details.NoteCount == 0 {
			details.FirstNote, _, _ = strings.Cut(note.Text, "\n")
		}
		details.NoteCount++
	}
	return details, nil
}

// Span is when run occupied its project: from its start to its finish, or
// to now while it is running. A run that is not running and recorded no
// finish (interrupted or incomplete) ends where it started. ok is false
// when the run recorded no start.
func (run Run) Span(now time.Time) (start, end time.Time, ok bool) {
	if run.Started.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	switch {
	case run.Lifecycle == "running":
		end = now
	case !run.Finished.IsZero():
		end = run.Finished
	default:
		end = run.Started
	}
	if end.Before(run.Started) {
		end = run.Started
	}
	return run.Started, end, true
}

// Overlapping keeps the runs whose span meets the range [from, to).
func Overlapping(runs []Run, from, to, now time.Time) []Run {
	kept := make([]Run, 0, len(runs))
	for _, run := range runs {
		start, end, ok := run.Span(now)
		if ok && start.Before(to) && !end.Before(from) {
			kept = append(kept, run)
		}
	}
	return kept
}

// mayMeet reports whether the run in entry may have a span meeting the
// window, judged from its ID and its directory's modification time only.
func (bounds window) mayMeet(entry os.DirEntry) bool {
	if started, ok := RunIDStart(entry.Name()); ok && !started.Before(bounds.to) {
		return false
	}
	if info, err := entry.Info(); err == nil && info.ModTime().Before(bounds.from) {
		return false
	}
	return true
}

// RunIDStart is the start time a run ID records: rotari names a run by its
// UTC start, YYYYMMDD-HHMMSS-XXXXXXXX. ok is false for an ID in another form.
func RunIDStart(runID string) (time.Time, bool) {
	if len(runID) < len("20060102-150405") {
		return time.Time{}, false
	}
	started, err := time.Parse("20060102-150405", runID[:len("20060102-150405")])
	return started, err == nil
}

// LatestStart is the latest run start, by run ID, among the runs in
// baseDirs that started before now, read from directory names alone; ok is
// false when there is none.
func LatestStart(baseDirs []string, now time.Time) (latest time.Time, ok bool) {
	for _, baseDir := range baseDirs {
		projects, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if err != nil {
			continue
		}
		for _, projectEntry := range projects {
			if !projectEntry.IsDir() || !state.IsValidPathElement(projectEntry.Name()) {
				continue
			}
			runs, err := os.ReadDir(filepath.Join(baseDir, "projects", projectEntry.Name(), "runs"))
			if err != nil {
				continue
			}
			for _, run := range runs {
				if started, found := RunIDStart(run.Name()); found && run.IsDir() && started.Before(now) && started.After(latest) {
					latest, ok = started, true
				}
			}
		}
	}
	return latest, ok
}
