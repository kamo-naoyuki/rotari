package webui

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/runlist"
)

// activityMaxRange bounds one /api/activity request: a month view with the
// days of the neighbouring months it shows.
const activityMaxRange = 45 * 24 * time.Hour

// activityResponse is what /api/activity returns: every registered basedir,
// marked when it is missing or could not be read, and the runs across them
// whose span (runlist.Run.Span) meets the requested range.
type activityResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
	Now  string `json:"now"`
	// LatestStartedAt is the latest start of a run, in any basedir and at
	// any time, so the page can open on the latest day with runs without
	// searching month by month.
	LatestStartedAt string            `json:"latest_started_at,omitempty"`
	BaseDirs        []activityBaseDir `json:"basedirs"`
	Runs            []activityRun     `json:"runs"`
}

type activityBaseDir struct {
	webBaseDir
	Missing bool   `json:"missing,omitempty"`
	Error   string `json:"error,omitempty"`
}

type activityRun struct {
	BaseDirID  string `json:"basedir_id"`
	Project    string `json:"project_name"`
	RunID      string `json:"run_id"`
	RunName    string `json:"run_name,omitempty"`
	Lifecycle  string `json:"lifecycle"`
	ExitCode   *int   `json:"exit_code"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	// EndsAt is the end of the run's span (runlist.Run.Span): its finish,
	// the response's now while it runs, or its start when it recorded no
	// finish.
	EndsAt    string   `json:"ends_at"`
	CWD       string   `json:"cwd,omitempty"`
	Host      string   `json:"host,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	NoteCount int      `json:"note_count"`
	FirstNote string   `json:"first_note,omitempty"`
	// StartedBy is how the run was started, absent for older runs.
	StartedBy *model.StartedBy `json:"started_by,omitempty"`
}

func (s site) handleActivity(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer)
		return
	}
	from, to, err := parseActivityRange(request.URL.Query())
	if err != nil {
		writeWebError(writer, err)
		return
	}
	writeWebJSON(writer, loadActivity(s.basedirEntries(), from, to, time.Now()))
}

// parseActivityRange reads from and to (RFC 3339). A missing, malformed,
// reversed, or longer than activityMaxRange range is an error, never
// clamped.
func parseActivityRange(query url.Values) (from, to time.Time, err error) {
	parse := func(name string) (time.Time, error) {
		value := query.Get(name)
		if value == "" {
			return time.Time{}, fmt.Errorf("%s is required", name)
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid %s %q: want an RFC 3339 time", name, value)
		}
		return parsed, nil
	}
	if from, err = parse("from"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if to, err = parse("to"); err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("to must be after from")
	}
	if to.Sub(from) > activityMaxRange {
		return time.Time{}, time.Time{}, fmt.Errorf("the range may span at most %d days", int(activityMaxRange/(24*time.Hour)))
	}
	return from, to, nil
}

// loadActivity lists the runs of entries that meet [from, to). A missing
// basedir, or one whose runs cannot be read, is reported on its entry
// rather than failing the others.
func loadActivity(entries []webBaseDir, from, to, now time.Time) activityResponse {
	response := activityResponse{
		From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), Now: now.Format(time.RFC3339),
		BaseDirs: []activityBaseDir{}, Runs: []activityRun{},
	}
	var paths []string
	for _, entry := range entries {
		item := activityBaseDir{webBaseDir: entry}
		if _, err := os.Stat(entry.Path); errors.Is(err, os.ErrNotExist) {
			item.Missing = true
			response.BaseDirs = append(response.BaseDirs, item)
			continue
		}
		paths = append(paths, entry.Path)
		runs, err := runlist.CollectBetween([]string{entry.Path}, "", from, to)
		if err == nil {
			var listed []activityRun
			for _, run := range runlist.Overlapping(runs, from, to, now) {
				_, end, _ := run.Span(now)
				details, detailErr := runlist.ReadDetails(run)
				if detailErr != nil {
					err = detailErr
					break
				}
				listed = append(listed, activityRun{
					BaseDirID: entry.ID, Project: run.Project, RunID: run.RunID, RunName: run.RunName,
					Lifecycle: run.Lifecycle, ExitCode: run.ExitCode, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
					EndsAt: end.Format(time.RFC3339Nano),
					CWD:    details.WorkingDirectory, Host: details.Host, Sources: details.Sources,
					NoteCount: details.NoteCount, FirstNote: details.FirstNote, StartedBy: details.StartedBy,
				})
			}
			if err == nil {
				response.Runs = append(response.Runs, listed...)
			}
		}
		if err != nil {
			item.Error = err.Error()
		}
		response.BaseDirs = append(response.BaseDirs, item)
	}
	if latest, ok := runlist.LatestStart(paths, now); ok {
		response.LatestStartedAt = latest.Format(time.RFC3339)
	}
	return response
}

// staticActivity is a static export's activity: every run of the exported
// basedir, which the page sorts into days itself.
func staticActivity(entries []webBaseDir, now time.Time) activityResponse {
	return loadActivity(entries, time.Unix(0, 0), now.Add(24*time.Hour), now)
}
