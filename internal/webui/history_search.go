package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

type historySearchAPIRequest struct {
	webprojection.HistorySearchRequest
	BasedirIDs []string `json:"basedir_ids"`
}

func (s site) handleHistorySearch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	var input historySearchAPIRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeWebError(writer, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWebError(writer, fmt.Errorf("history search request must contain one JSON value"))
		return
	}
	if err := webprojection.ValidateHistorySearchRequest(input.HistorySearchRequest); err != nil {
		writeWebError(writer, err)
		return
	}
	if len(input.BasedirIDs) == 0 || len(input.BasedirIDs) > 100 {
		writeWebError(writer, fmt.Errorf("history search requires between 1 and 100 basedirs"))
		return
	}
	entries := make(map[string]webBaseDir)
	for _, entry := range s.basedirEntries() {
		entries[entry.ID] = entry
	}
	selected := make(map[string]bool, len(input.BasedirIDs))
	for _, id := range input.BasedirIDs {
		_, ok := entries[id]
		if !ok || selected[id] {
			writeWebError(writer, fmt.Errorf("invalid basedir_id %q", id))
			return
		}
		selected[id] = true
	}
	records := make([]webprojection.HistorySearchRecord, 0)
	for _, id := range input.BasedirIDs {
		entry := entries[id]
		loaded, err := s.loadHistorySearchRecords(entry)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		records = append(records, loaded...)
	}
	result, err := webprojection.SearchHistory(records, input.HistorySearchRequest)
	if err != nil {
		writeWebError(writer, err)
		return
	}
	writeWebJSON(writer, result)
}

func (s site) loadHistorySearchRecords(entry webBaseDir) ([]webprojection.HistorySearchRecord, error) {
	projects, err := joblist.Projects(entry.Path, "")
	if err != nil {
		return nil, err
	}
	records := make([]webprojection.HistorySearchRecord, 0)
	for _, projectName := range projects {
		projectRecords, err := s.loadHistorySearchProject(entry, projectName)
		if err != nil {
			return nil, err
		}
		records = append(records, projectRecords...)
	}
	return records, nil
}

func (s site) loadHistorySearchProject(entry webBaseDir, projectName string) ([]webprojection.HistorySearchRecord, error) {
	paths, err := stateinternal.ResolveProjectPaths(entry.Path, projectName)
	if err != nil {
		return nil, err
	}
	runIDs, err := webRunIDs(paths.RunsDir)
	if err != nil {
		return nil, err
	}
	lock, lockErr := stateinternal.LoadLock(paths.LockFile)
	if lockErr != nil {
		lock = model.LockInfo{}
	}
	records := make([]webprojection.HistorySearchRecord, 0)
	if len(runIDs) == 0 {
		records = append(records, webprojection.HistorySearchRecord{
			BaseDirID: entry.ID, BaseDirPath: entry.Path, ProjectName: projectName,
		})
	}
	for _, runID := range runIDs {
		runningStartedAt := ""
		if lock.RunID == runID {
			runningStartedAt = lock.StartedAt
		}
		runRecords, err := s.loadHistorySearchRun(entry, projectName, paths.RunsDir, runID, runningStartedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, runRecords...)
	}
	return records, nil
}

func (s site) loadHistorySearchRun(entry webBaseDir, projectName, runsDir, runID, runningStartedAt string) ([]webprojection.HistorySearchRecord, error) {
	runDir, err := stateinternal.SafeJoin(runsDir, runID)
	if err != nil {
		return nil, err
	}
	summary, summaryErr := stateinternal.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if errors.Is(summaryErr, stateinternal.ErrNewerStateVersion) {
		summary = model.RunSummary{RunID: runID, Status: "unreadable"}
	} else if summaryErr != nil {
		summary = model.RunSummary{RunID: runID, Status: "running", StartedAt: runningStartedAt}
	}
	if summary.RunID == "" {
		summary.RunID = runID
	}
	base := historySearchBaseRecord(entry, projectName, summary)
	records := []webprojection.HistorySearchRecord{base}
	if errors.Is(summaryErr, stateinternal.ErrNewerStateVersion) {
		return records, nil
	}
	jobs, err := webprojection.LoadRunJobs(s.Store, runDir, summary, "")
	if errors.Is(err, stateinternal.ErrNewerStateVersion) {
		return records, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load jobs for project %q run %q: %w", projectName, runID, err)
	}
	for _, job := range jobs {
		records = append(records, historySearchJobRecord(base, job))
	}
	return records, nil
}

func historySearchBaseRecord(entry webBaseDir, projectName string, summary model.RunSummary) webprojection.HistorySearchRecord {
	status := summary.Status
	if status == "" {
		if summary.FinishedAt != "" {
			status = model.RunStatus(summary.ExitCode)
		} else {
			status = "running"
		}
	}
	timestamp := summary.FinishedAt
	if timestamp == "" {
		timestamp = summary.StartedAt
	}
	var exitCode *int
	if summary.FinishedAt != "" || summary.Status == "success" || summary.Status == "failed" {
		value := summary.ExitCode
		exitCode = &value
	}
	return webprojection.HistorySearchRecord{
		BaseDirID: entry.ID, BaseDirPath: entry.Path, ProjectName: projectName,
		RunID: summary.RunID, RunName: summary.RunName, RunStatus: status,
		RunExitCode: exitCode, RunStarted: summary.StartedAt,
		RunFinished: summary.FinishedAt, Timestamp: timestamp,
	}
}

func historySearchJobRecord(base webprojection.HistorySearchRecord, job webprojection.Job) webprojection.HistorySearchRecord {
	jobStatus := "pending"
	if base.RunStatus == "running" {
		jobStatus = "running"
	}
	if job.SchedulerState != "" {
		jobStatus = job.SchedulerState
	}
	var exitCode *int
	if job.Result != nil {
		switch {
		case job.Result.Accepted:
			jobStatus = "success (accepted)"
		case job.Result.Error == "blocked by failed dependency":
			jobStatus = "blocked"
		default:
			jobStatus = model.ResultStatus(*job.Result, true)
		}
		value := job.Result.ExitCode
		exitCode = &value
	}
	timestamp := job.FinishedAt
	if timestamp == "" {
		timestamp = job.SubmittedAt
	}
	if timestamp == "" {
		timestamp = base.Timestamp
	}
	base.JobID, base.JobName = job.ID, job.Name
	base.JobStatus, base.JobStage = jobStatus, job.Stage
	base.Command, base.Executor, base.AttemptID = strings.Join(job.Command, " "), job.Executor, job.AttemptID
	base.JobExitCode, base.Timestamp = exitCode, timestamp
	return base
}
