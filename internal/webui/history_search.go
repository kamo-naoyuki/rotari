package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

type historySearchAPIRequest struct {
	webprojection.HistorySearchRequest
	Scopes []webprojection.HistorySearchScope `json:"scopes"`
}

type historySearchOptionsResponse struct {
	Projects []string                 `json:"projects,omitempty"`
	Runs     []historySearchRunOption `json:"runs,omitempty"`
}

type historySearchRunOption struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

func (s site) handleHistorySearchOptions(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer)
		return
	}
	entry, ok := s.registeredBasedir(request.URL.Query().Get("basedir_id"))
	if !ok {
		writeWebError(writer, fmt.Errorf("invalid basedir_id"))
		return
	}
	projectName := request.URL.Query().Get("project_name")
	if projectName == "" {
		projects, err := joblist.Projects(entry.Path, "")
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, historySearchOptionsResponse{Projects: projects})
		return
	}
	projects, err := joblist.Projects(entry.Path, "")
	if err != nil {
		writeWebError(writer, err)
		return
	}
	if !containsHistorySearchValue(projects, projectName) {
		writeWebError(writer, fmt.Errorf("project_name %q not found", projectName))
		return
	}
	paths, err := stateinternal.ResolveProjectPaths(entry.Path, projectName)
	if err != nil {
		writeWebError(writer, err)
		return
	}
	runIDs, err := webRunIDs(paths.RunsDir)
	if err != nil {
		writeWebError(writer, err)
		return
	}
	runs := make([]historySearchRunOption, 0, len(runIDs))
	for _, runID := range runIDs {
		runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		summary, err := stateinternal.LoadRunSummary(filepath.Join(runDir, "summary.json"))
		if errors.Is(err, stateinternal.ErrNewerStateVersion) {
			runs = append(runs, historySearchRunOption{ID: runID, Status: "unreadable"})
			continue
		}
		if err != nil {
			runs = append(runs, historySearchRunOption{ID: runID, Status: "running"})
			continue
		}
		status := summary.Status
		if status == "" && summary.FinishedAt != "" {
			status = model.RunStatus(summary.ExitCode)
		}
		runs = append(runs, historySearchRunOption{ID: runID, Name: summary.RunName, Status: status})
	}
	writeWebJSON(writer, historySearchOptionsResponse{Runs: runs})
}

func (s site) handleHistorySearchDiagnoses(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer)
		return
	}
	names := map[string]bool{"Python exception": true}
	for _, rule := range diagnose.DefaultRules() {
		names[rule.Name] = true
	}
	options := make([]string, 0, len(names))
	for name := range names {
		options = append(options, name)
	}
	sort.Strings(options)
	writeWebJSON(writer, map[string][]string{"diagnoses": options})
}

func containsHistorySearchValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s site) registeredBasedir(id string) (webBaseDir, bool) {
	for _, entry := range s.basedirEntries() {
		if entry.ID == id {
			return entry, true
		}
	}
	return webBaseDir{}, false
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
	if len(input.Scopes) == 0 {
		writeWebError(writer, fmt.Errorf("history search requires at least one search range"))
		return
	}
	selectedScopes := make(map[string]bool, len(input.Scopes))
	records := make([]webprojection.HistorySearchRecord, 0)
	for _, scope := range input.Scopes {
		entry, ok := s.registeredBasedir(scope.BaseDirID)
		if !ok {
			writeWebError(writer, fmt.Errorf("invalid basedir_id %q", scope.BaseDirID))
			return
		}
		if scope.RunID != "" && scope.ProjectName == "" {
			writeWebError(writer, fmt.Errorf("project_name is required with run_id"))
			return
		}
		key := scope.BaseDirID + "\x00" + scope.ProjectName + "\x00" + scope.RunID
		if selectedScopes[key] {
			continue
		}
		selectedScopes[key] = true
		loaded, err := s.loadHistorySearchRecords(entry, scope.ProjectName, scope.RunID)
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

func (s site) loadHistorySearchRecords(entry webBaseDir, projectName, runID string) ([]webprojection.HistorySearchRecord, error) {
	projects, err := joblist.Projects(entry.Path, "")
	if err != nil {
		return nil, err
	}
	if projectName != "" {
		if !stateinternal.IsValidPathElement(projectName) {
			return nil, fmt.Errorf("invalid project_name %q", projectName)
		}
		if !containsHistorySearchValue(projects, projectName) {
			return nil, fmt.Errorf("project_name %q not found", projectName)
		}
		projects = []string{projectName}
	}
	records := make([]webprojection.HistorySearchRecord, 0)
	for _, projectName := range projects {
		projectRecords, err := s.loadHistorySearchProject(entry, projectName, runID)
		if err != nil {
			return nil, err
		}
		records = append(records, projectRecords...)
	}
	return records, nil
}

func (s site) loadHistorySearchProject(entry webBaseDir, projectName, selectedRunID string) ([]webprojection.HistorySearchRecord, error) {
	paths, err := stateinternal.ResolveProjectPaths(entry.Path, projectName)
	if err != nil {
		return nil, err
	}
	runIDs, err := webRunIDs(paths.RunsDir)
	if err != nil {
		return nil, err
	}
	if selectedRunID != "" {
		if _, err := stateinternal.SafeJoin(paths.RunsDir, selectedRunID); err != nil {
			return nil, err
		}
		if !containsHistorySearchValue(runIDs, selectedRunID) {
			return nil, fmt.Errorf("run_id %q not found for project %q", selectedRunID, projectName)
		}
		runIDs = []string{selectedRunID}
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
	runContext, contextErr := stateinternal.LoadContext(s.Store, runDir)
	if contextErr != nil {
		runContext = model.RunContext{}
	}
	base := historySearchBaseRecord(entry, projectName, summary, runContext)
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

func historySearchBaseRecord(entry webBaseDir, projectName string, summary model.RunSummary, runContext model.RunContext) webprojection.HistorySearchRecord {
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
		RunFinished: summary.FinishedAt, RunHost: runContext.Hostname,
		RunWorkingDirectory: runContext.CWD, Timestamp: timestamp,
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
	base.WorkingDirectory = job.WorkingDirectory
	if base.WorkingDirectory == "" {
		base.WorkingDirectory = base.RunWorkingDirectory
	}
	if job.Result != nil {
		base.Host = strings.Join(job.Result.Hosts, " ")
	}
	base.JobExitCode, base.Timestamp = exitCode, timestamp
	if job.Result != nil {
		names := make([]string, 0, len(job.Result.Diagnoses))
		for _, diagnosis := range job.Result.Diagnoses {
			if diagnosis.Name != "" {
				names = append(names, diagnosis.Name)
			}
		}
		base.Diagnosis = strings.Join(names, "\n")
	}
	return base
}
