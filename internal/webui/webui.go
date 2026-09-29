package webui

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/report"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

const (
	// DefaultPort is the port the Web UI listens on unless told otherwise.
	DefaultPort       = 8787
	webConfigFileName = "config.toml"
	authBearerPrefix  = "Bearer "
	headerContentType = "Content-Type"
)

type webGenerateConfigRequest struct {
	QueueName string `json:"project_name"`
	Location  string `json:"location"`
}

type webConfigTarget struct {
	Location string `json:"location"`
	Path     string `json:"path"`
}

type webSaveConfigRequest struct {
	QueueName string `json:"project_name"`
	Content   string `json:"content"`
}

type webCopyRequest struct {
	QueueName string   `json:"project_name"`
	RunID     string   `json:"run_id"`
	JobID     string   `json:"job_id,omitempty"`
	JobIDs    []string `json:"job_ids,omitempty"`
	Selection string   `json:"selection"`
	Append    bool     `json:"append"`
	Overwrite bool     `json:"overwrite"`
}

type webChangeRequest struct {
	QueueName             string   `json:"project_name"`
	JobID                 string   `json:"job_id"`
	SetJobName            string   `json:"set_job_name,omitempty"`
	Command               []string `json:"command,omitempty"`
	Executor              string   `json:"executor,omitempty"`
	ExecutorOptions       []string `json:"executor_options,omitempty"`
	ClearExecutorOptions  bool     `json:"clear_executor_options"`
	WorkingDirectory      string   `json:"working_directory,omitempty"`
	ClearWorkingDirectory bool     `json:"clear_working_directory"`
	Environment           []string `json:"environment,omitempty"`
	ClearEnvironment      bool     `json:"clear_environment"`
	DependsOn             []string `json:"depends_on,omitempty"`
	ClearDependsOn        bool     `json:"clear_depends_on"`
	// DependsOnFinished and ClearDependsOnFinished mirror change
	// --depends-on-finished and --clear-depends-on-finished.
	DependsOnFinished      []string `json:"depends_on_finished,omitempty"`
	ClearDependsOnFinished bool     `json:"clear_depends_on_finished"`
}

type webRemoveRequest struct {
	QueueName string `json:"project_name"`
	JobID     string `json:"job_id"`
}

type webCancelRequest struct {
	QueueName string   `json:"project_name"`
	RunID     string   `json:"run_id"`
	JobID     string   `json:"job_id"`
	JobIDs    []string `json:"job_ids,omitempty"`
}

type webJobControlRequest struct {
	QueueName string   `json:"project_name"`
	RunID     string   `json:"run_id"`
	JobID     string   `json:"job_id"`
	JobIDs    []string `json:"job_ids,omitempty"`
}

type webCancelRunRequest struct {
	QueueName string `json:"project_name"`
	RunID     string `json:"run_id"`
}

type webClearRequest struct {
	QueueName string `json:"project_name"`
	RunID     string `json:"run_id"`
}

// WithAuthToken requires token from every request, as an X-Rotari-Token
// header, a bearer token, or the Basic auth password of user "rotari".
func WithAuthToken(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		provided := request.Header.Get("X-Rotari-Token")
		if provided == "" {
			const prefix = authBearerPrefix
			authorization := request.Header.Get("Authorization")
			if strings.HasPrefix(authorization, prefix) {
				provided = strings.TrimPrefix(authorization, prefix)
			}
		}
		if provided == "" {
			if username, password, ok := request.BasicAuth(); ok && username == "rotari" {
				provided = password
			}
		}
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			writer.Header().Add("WWW-Authenticate", `Basic realm="rotari web"`)
			writer.Header().Add("WWW-Authenticate", `Bearer realm="rotari web"`)
			http.Error(writer, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// IsLoopbackHost reports whether host only accepts local connections.
func IsLoopbackHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Listen listens on host and port. With fallback, a busy port is followed by
// the next free one.
func Listen(host string, port int, fallback bool) (net.Listener, error) {
	for candidate := port; ; candidate++ {
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(candidate)))
		if err == nil {
			return listener, nil
		}
		if !fallback || port == 0 || candidate == 65535 {
			return nil, err
		}
	}
}

func webJobIDs(single string, multiple []string) ([]string, error) {
	if single != "" && len(multiple) > 0 {
		return nil, fmt.Errorf("job_id and job_ids cannot both be provided")
	}
	if single != "" {
		multiple = []string{single}
	}
	if len(multiple) == 0 {
		return nil, fmt.Errorf("job_id or job_ids are required")
	}
	for _, jobID := range multiple {
		if !stateinternal.IsValidPathElement(jobID) {
			return nil, fmt.Errorf("invalid job_id %q", jobID)
		}
	}
	return multiple, nil
}

func (s site) handler() http.Handler {
	baseDir, queueFilter, allowControl := s.BaseDir, s.ProjectFilter, s.AllowControl
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(s.webHTML()))
	})
	mux.HandleFunc("/web_styles.css", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = writer.Write([]byte(webStylesCSS))
	})
	mux.HandleFunc("/web_sidebar_styles.css", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = writer.Write([]byte(webSidebarStylesCSS))
	})
	mux.HandleFunc("/jobs/", func(writer http.ResponseWriter, request *http.Request) {
		sinceText := request.URL.Query().Get("since")
		window, err := joblist.ParseSince(sinceText)
		if err != nil {
			writeWebError(writer, fmt.Errorf("invalid since duration %q", sinceText))
			return
		}
		if sinceText == "" {
			sinceText = joblist.DefaultSinceText
		}
		projects, err := joblist.Projects(baseDir, queueFilter)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		rows, err := joblist.Collect(s.Store, baseDir, projects, time.Now(), window)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		joblist.Sort(rows)
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(jobsHTML("/", projects, rows, sinceText, true, s.Notifications)))
	})
	mux.HandleFunc("/jobs", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/jobs/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/api/state", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		state, err := s.loadWebState(baseDir, queueFilter)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, state)
	})
	mux.HandleFunc("/api/config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		files, err := s.loadWebConfigFiles(baseDir, request.URL.Query().Get("project_name"), request.URL.Query().Get("run_id"))
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]any{"configs": files})
	})
	mux.HandleFunc("/api/save-config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var save webSaveConfigRequest
		if err := json.NewDecoder(request.Body).Decode(&save); err != nil {
			writeWebError(writer, err)
			return
		}
		path, err := saveWebConfig(baseDir, save.QueueName, save.Content)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": "config saved", "path": path})
	})
	mux.HandleFunc("/api/generate-config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var generate webGenerateConfigRequest
		if err := json.NewDecoder(request.Body).Decode(&generate); err != nil {
			writeWebError(writer, err)
			return
		}
		path, err := s.generateWebConfig(baseDir, generate.QueueName, generate.Location)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": "config generated", "path": path})
	})
	mux.HandleFunc("/api/config-targets", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		targets, err := webConfigTargets(baseDir, request.URL.Query().Get("project_name"))
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]any{"targets": targets})
	})
	mux.HandleFunc("/api/report", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		projectName := request.URL.Query().Get("project_name")
		runID := request.URL.Query().Get("run_id")
		jobID := request.URL.Query().Get("job_id")
		jobIDs := request.URL.Query()["job_ids"]
		if !stateinternal.IsValidPathElement(projectName) || !stateinternal.IsValidPathElement(runID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required; job_id must be valid when supplied"))
			return
		}
		if jobID != "" && len(jobIDs) > 0 {
			writeWebError(writer, fmt.Errorf("job_id and job_ids cannot be combined"))
			return
		}
		if jobID != "" {
			jobIDs = []string{jobID}
		}
		for _, jobID := range jobIDs {
			if !stateinternal.IsValidPathElement(jobID) {
				writeWebError(writer, fmt.Errorf("project_name and run_id are required; job_id must be valid when supplied"))
				return
			}
		}
		paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		redact := request.URL.Query().Get("redact") != "false"
		if request.URL.Query().Get("job_ids") != "" {
			report, err := report.BuildForJobs(s.Store, paths, runID, jobIDs, redact)
			if err != nil {
				writeWebError(writer, err)
				return
			}
			writer.Header().Set(headerContentType, "text/markdown; charset=utf-8")
			_, _ = writer.Write([]byte(report))
			return
		}
		singleJobID := ""
		if request.URL.Query().Get("job_id") != "" {
			singleJobID = jobIDs[0]
		}
		report, err := report.Build(s.Store, paths, runID, singleJobID, false, "", redact)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writer.Header().Set(headerContentType, "text/markdown; charset=utf-8")
		_, _ = writer.Write([]byte(report))
	})
	mux.HandleFunc("/api/log", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		queueName := request.URL.Query().Get("project_name")
		runID, jobID := request.URL.Query().Get("run_id"), request.URL.Query().Get("job_id")
		stream := request.URL.Query().Get("stream")
		if stream == "" {
			stream = stateinternal.StdoutFileName
		}
		if !stateinternal.IsValidPathElement(queueName) || !stateinternal.IsValidPathElement(runID) || !stateinternal.IsValidPathElement(jobID) {
			writeWebError(writer, fmt.Errorf("project_name, run_id and job_id are required"))
			return
		}
		if stream != stateinternal.StdoutFileName && stream != stateinternal.StderrFileName {
			writeWebError(writer, fmt.Errorf("stream must be stdout or stderr"))
			return
		}
		projectDir, err := stateinternal.SafeJoin(filepath.Join(baseDir, "projects"), queueName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		path, err := webLogPath(filepath.Join(projectDir, "runs"), runID, jobID, request.URL.Query().Get("attempt_id"), stream)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		// codeql[go/path-injection]: path is restricted by webLogPath to a validated stream file.
		data, err := os.ReadFile(path) // NOSONAR: path is restricted by validatedStateFile to stdout or stderr.
		if err != nil {
			writeWebError(writer, err)
			return
		}
		lines := strings.Split(string(data), "\n")
		if request.URL.Query().Get("tail") != "" {
			count, parseErr := strconv.Atoi(request.URL.Query().Get("tail"))
			if parseErr != nil || count < 1 {
				writeWebError(writer, fmt.Errorf("tail must be a positive integer"))
				return
			}
			before := 0
			if value := request.URL.Query().Get("before"); value != "" {
				before, parseErr = strconv.Atoi(value)
				if parseErr != nil || before < 0 {
					writeWebError(writer, fmt.Errorf("before must be a non-negative integer"))
					return
				}
			}
			end := len(lines) - before
			if end < 0 {
				end = 0
			}
			start := end - count
			if start < 0 {
				start = 0
			}
			if start < end {
				lines = lines[start:end]
			} else {
				lines = nil
			}
			data = []byte(strings.Join(lines, "\n"))
		}
		writer.Header().Set(headerContentType, "text/plain; charset=utf-8")
		_, _ = writer.Write(data)
	})
	mux.HandleFunc("/api/copy", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var copyRequest webCopyRequest
		if err := json.NewDecoder(request.Body).Decode(&copyRequest); err != nil {
			writeWebError(writer, err)
			return
		}
		if !stateinternal.IsValidPathElement(copyRequest.QueueName) || !stateinternal.IsValidPathElement(copyRequest.RunID) || (copyRequest.JobID != "" && !stateinternal.IsValidPathElement(copyRequest.JobID)) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		if copyRequest.JobID != "" {
			copyRequest.JobIDs = append(copyRequest.JobIDs, copyRequest.JobID)
		}
		for _, jobID := range copyRequest.JobIDs {
			if !stateinternal.IsValidPathElement(jobID) {
				writeWebError(writer, fmt.Errorf("job_ids must contain valid job IDs"))
				return
			}
		}
		if copyRequest.JobID != "" {
			if copyRequest.Selection != "" && copyRequest.Selection != "job-id" {
				writeWebError(writer, fmt.Errorf("job_id cannot be combined with selection %q", copyRequest.Selection))
				return
			}
			copyRequest.Selection = "job-id"
			copyRequest.Append = true
		}
		if copyRequest.Append && copyRequest.Overwrite {
			writeWebError(writer, fmt.Errorf("append and overwrite cannot be used together"))
			return
		}
		if copyRequest.Selection == "" {
			copyRequest.Selection = "failed"
		}
		if copyRequest.Selection != "all" && copyRequest.Selection != "failed" && copyRequest.Selection != "unfinished" && copyRequest.Selection != "success" && copyRequest.Selection != "job-id" {
			writeWebError(writer, fmt.Errorf("unsupported copy selection %q", copyRequest.Selection))
			return
		}
		message, err := s.Editor.Copy(baseDir, copyRequest.QueueName, copyRequest.RunID, queueedit.CopyRequest{Selection: copyRequest.Selection, JobIDs: copyRequest.JobIDs, Append: copyRequest.Append, Overwrite: copyRequest.Overwrite})
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/change", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var change webChangeRequest
		if err := json.NewDecoder(request.Body).Decode(&change); err != nil {
			writeWebError(writer, err)
			return
		}
		if !stateinternal.IsValidPathElement(change.QueueName) || !stateinternal.IsValidPathElement(change.JobID) || len(change.Command) == 0 {
			writeWebError(writer, fmt.Errorf("project_name, job_id, and command are required"))
			return
		}
		message, err := s.Editor.Change(baseDir, change.QueueName, "", model.CommandSelector{IDs: []string{change.JobID}}, queueops.Mutation{
			Executor: change.Executor, ExecutorOptions: change.ExecutorOptions, ClearExecutorOptions: change.ClearExecutorOptions,
			Environment: change.Environment, ClearEnvironment: change.ClearEnvironment,
			WorkingDirectory: change.WorkingDirectory, ClearWorkingDirectory: change.ClearWorkingDirectory, SetJobName: change.SetJobName,
			DependsOn: change.DependsOn, ClearDependsOn: change.ClearDependsOn,
			DependsOnFinished: change.DependsOnFinished, ClearDependsOnFinished: change.ClearDependsOnFinished, Command: change.Command,
		})
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/remove", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var remove webRemoveRequest
		if err := json.NewDecoder(request.Body).Decode(&remove); err != nil {
			writeWebError(writer, err)
			return
		}
		if !stateinternal.IsValidPathElement(remove.QueueName) || !stateinternal.IsValidPathElement(remove.JobID) {
			writeWebError(writer, fmt.Errorf("project_name and job_id are required"))
			return
		}
		message, err := s.Editor.Remove(baseDir, remove.QueueName, "", model.CommandSelector{IDs: []string{remove.JobID}})
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/clear-run", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var clearRequest webClearRequest
		if err := json.NewDecoder(request.Body).Decode(&clearRequest); err != nil {
			writeWebError(writer, err)
			return
		}
		if !stateinternal.IsValidPathElement(clearRequest.QueueName) || !stateinternal.IsValidPathElement(clearRequest.RunID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		if err := s.Editor.DeleteRun(baseDir, clearRequest.QueueName, clearRequest.RunID); err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": "run history deleted"})
	})
	mux.HandleFunc("/api/cancel-job", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var cancel webCancelRequest
		if err := json.NewDecoder(request.Body).Decode(&cancel); err != nil {
			writeWebError(writer, err)
			return
		}
		jobIDs, err := webJobIDs(cancel.JobID, cancel.JobIDs)
		if err != nil || !stateinternal.IsValidPathElement(cancel.QueueName) || !stateinternal.IsValidPathElement(cancel.RunID) {
			if err == nil {
				err = fmt.Errorf("project_name and run_id are required")
			}
			writeWebError(writer, err)
			return
		}
		message, err := s.Controller.Cancel(baseDir, cancel.QueueName, cancel.RunID, jobIDs, false)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/suspend-job", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var control webJobControlRequest
		if err := json.NewDecoder(request.Body).Decode(&control); err != nil {
			writeWebError(writer, err)
			return
		}
		jobIDs, err := webJobIDs(control.JobID, control.JobIDs)
		if err != nil || !stateinternal.IsValidPathElement(control.QueueName) || !stateinternal.IsValidPathElement(control.RunID) {
			if err == nil {
				err = fmt.Errorf("project_name and run_id are required")
			}
			writeWebError(writer, err)
			return
		}
		message, err := s.Controller.Control(baseDir, control.QueueName, control.RunID, jobIDs, "suspend")
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/resume-job", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var control webJobControlRequest
		if err := json.NewDecoder(request.Body).Decode(&control); err != nil {
			writeWebError(writer, err)
			return
		}
		jobIDs, err := webJobIDs(control.JobID, control.JobIDs)
		if err != nil || !stateinternal.IsValidPathElement(control.QueueName) || !stateinternal.IsValidPathElement(control.RunID) {
			if err == nil {
				err = fmt.Errorf("project_name and run_id are required")
			}
			writeWebError(writer, err)
			return
		}
		message, err := s.Controller.Control(baseDir, control.QueueName, control.RunID, jobIDs, "resume")
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	mux.HandleFunc("/api/cancel-run", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !allowControl {
			forbiddenReadOnly(writer)
			return
		}
		var cancel webCancelRunRequest
		if err := json.NewDecoder(request.Body).Decode(&cancel); err != nil {
			writeWebError(writer, err)
			return
		}
		if !stateinternal.IsValidPathElement(cancel.QueueName) || !stateinternal.IsValidPathElement(cancel.RunID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		message, err := s.Controller.Cancel(baseDir, cancel.QueueName, cancel.RunID, nil, false)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	return mux
}

func (s site) loadWebConfigFiles(baseDir, projectName, runID string) ([]webprojection.ConfigFile, error) {
	var paths []string
	if projectName == "" {
		if runID != "" {
			return nil, fmt.Errorf("project_name is required with run_id")
		}
		if path := config.EffectivePath(baseDir, ""); path != "" {
			paths = []string{path}
		}
	} else {
		if !stateinternal.IsValidPathElement(projectName) {
			return nil, fmt.Errorf("invalid project_name %q", projectName)
		}
		if runID == "" {
			if path := config.EffectivePath(baseDir, projectName); path != "" {
				paths = []string{path}
			}
		} else {
			if !stateinternal.IsValidPathElement(runID) {
				return nil, fmt.Errorf("invalid run_id %q", runID)
			}
			return s.loadRunConfigFiles(baseDir, projectName, runID)
		}
	}
	files := make([]webprojection.ConfigFile, 0, len(paths))
	for _, path := range paths {
		// codeql[go/path-injection]: paths contain only resolved config files or validated run context entries.
		data, err := os.ReadFile(path) // NOSONAR: paths contain only the resolved global/project config files or validated run context entries.
		if err != nil {
			return nil, err
		}
		files = append(files, webprojection.ConfigFile{Path: path, Content: string(data)})
	}
	return files, nil
}

func saveWebConfig(baseDir, projectName, content string) (string, error) {
	if projectName != "" && !stateinternal.IsValidPathElement(projectName) {
		return "", fmt.Errorf("invalid project_name %q", projectName)
	}
	path := config.EffectivePath(baseDir, projectName)
	if path == "" {
		return "", fmt.Errorf("no config file exists to edit")
	}
	if _, err := config.Parse(path, []byte(content)); err != nil {
		return "", fmt.Errorf("invalid %s config: %w", strings.TrimPrefix(filepath.Ext(path), "."), err)
	}
	// codeql[go/path-injection]: path is returned by the allow-listed config resolver.
	if err := os.WriteFile(path, []byte(content), stateinternal.FileMode()); err != nil {
		return "", err
	}
	return path, nil
}

func (s site) loadRunConfigFiles(baseDir, projectName, runID string) ([]webprojection.ConfigFile, error) {
	paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return nil, err
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return nil, err
	}
	context, err := stateinternal.LoadContext(s.Store, runDir)
	if err != nil {
		return nil, err
	}
	if len(context.ConfigSnapshotFiles) == 0 {
		return loadLegacyRunConfigFiles(baseDir, projectName, context.ConfigPaths)
	}
	if len(context.ConfigPaths) != len(context.ConfigSnapshotFiles) {
		return nil, fmt.Errorf("run %q has inconsistent config snapshots", runID)
	}
	snapshotDir, err := stateinternal.SafeJoin(runDir, "configs")
	if err != nil {
		return nil, err
	}
	files := make([]webprojection.ConfigFile, 0, len(context.ConfigSnapshotFiles))
	for index, fileName := range context.ConfigSnapshotFiles {
		snapshotPath, err := stateinternal.SafeJoin(snapshotDir, fileName)
		if err != nil {
			return nil, err
		}
		// codeql[go/path-injection]: snapshotPath is safely joined below the validated run directory.
		data, err := os.ReadFile(snapshotPath) // NOSONAR: snapshotPath is safely joined below the validated run directory.
		if err != nil {
			return nil, err
		}
		files = append(files, webprojection.ConfigFile{Path: context.ConfigPaths[index], Content: string(data)})
	}
	return files, nil
}

func webConfigTargets(baseDir, projectName string) ([]webConfigTarget, error) {
	configHome, err := config.HomeDir()
	if err != nil {
		return nil, err
	}
	targets := []webConfigTarget{
		{Location: "global", Path: filepath.Join(configHome, webConfigFileName)},
		{Location: "basedir", Path: filepath.Join(baseDir, webConfigFileName)},
	}
	if projectName == "" {
		return targets, nil
	}
	if !stateinternal.IsValidPathElement(projectName) {
		return nil, fmt.Errorf("invalid project_name %q", projectName)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return nil, err
	}
	return append(targets, webConfigTarget{Location: "project", Path: filepath.Join(paths.ProjectDir, webConfigFileName)}), nil
}

func (s site) generateWebConfig(baseDir, projectName, location string) (string, error) {
	targets, err := webConfigTargets(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var target string
	for _, candidate := range targets {
		if candidate.Location == location {
			target = candidate.Path
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("invalid config location %q", location)
	}
	directory := filepath.Dir(target)
	existing := config.FilePaths(directory)
	if len(existing) > 1 {
		return "", fmt.Errorf("multiple config files found in %s: %s", directory, strings.Join(existing, ", "))
	}
	if len(existing) == 1 && filepath.Clean(existing[0]) != filepath.Clean(target) {
		return "", fmt.Errorf("config file %s already exists; remove it before generating config.toml", existing[0])
	}
	data, err := s.ConfigTemplate()
	if err != nil {
		return "", err
	}
	// codeql[go/path-injection]: directory is the resolved config directory.
	if err := os.MkdirAll(directory, stateinternal.DirectoryMode()); err != nil {
		return "", err
	}
	// codeql[go/path-injection]: target is the resolved config file path.
	if err := os.WriteFile(target, data, stateinternal.FileMode()); err != nil {
		return "", err
	}
	return target, nil
}

func loadLegacyRunConfigFiles(baseDir, projectName string, configPaths []string) ([]webprojection.ConfigFile, error) {
	allowed := make(map[string]bool)
	for _, path := range config.PathsForRun(baseDir, projectName) {
		allowed[filepath.Clean(path)] = true
	}
	allowedPaths := make([]string, 0, len(configPaths))
	for _, path := range configPaths {
		cleanPath := filepath.Clean(path)
		if allowed[cleanPath] {
			allowedPaths = append(allowedPaths, cleanPath)
		}
	}
	if len(allowedPaths) == 0 {
		return nil, nil
	}
	path := allowedPaths[len(allowedPaths)-1]
	data, err := os.ReadFile(path) // NOSONAR: path is one of the current allowed config locations.
	if err != nil {
		return nil, err
	}
	return []webprojection.ConfigFile{{Path: path, Content: string(data)}}, nil
}

// loadWebState projects persisted server and project state into the Web API
// model consumed by the embedded and static Web UIs.
func (s site) loadWebState(baseDir, queueFilter string) (webprojection.State, error) {
	state := webprojection.State{BaseDir: baseDir, ConfigPath: config.EffectivePath(baseDir, ""), Environments: s.environments(), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	for index := range state.Environments {
		// Only expose whether the variable is set, never its value: it may hold secrets (API keys, tokens).
		_, state.Environments[index].Set = os.LookupEnv(state.Environments[index].Name)
	}
	queueNames := []string{}
	if queueFilter != "" {
		queueNames = append(queueNames, queueFilter)
	} else {
		entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if err != nil && !os.IsNotExist(err) {
			return webprojection.State{}, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				queueNames = append(queueNames, entry.Name())
			}
		}
		sort.Strings(queueNames)
	}
	for _, queueName := range queueNames {
		paths, err := stateinternal.ResolveProjectPaths(baseDir, queueName)
		if err != nil {
			return webprojection.State{}, err
		}
		queueState, err := s.loadWebQueueState(paths)
		if err != nil {
			return webprojection.State{}, err
		}
		queueState.ConfigPath = config.EffectivePath(baseDir, queueName)
		queueState.Server = loadWebServerState(paths.ProjectDir)
		state.Queues = append(state.Queues, queueState)
	}
	state.UpdatedAt = model.FormatDisplayTimestamp(state.UpdatedAt)
	return state, nil
}

// loadWebServerState reads the files of the supervisor of projectDir without
// contacting it.
func loadWebServerState(projectDir string) webprojection.ServerState {
	state := webprojection.ServerState{}
	data, err := os.ReadFile(serverinternal.PIDPath(projectDir))
	if err != nil {
		return state
	}
	state.PIDFileExists = true
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil && pid > 0 {
		state.PID = pid
	}
	return state
}

func (s site) generateStaticWeb(outputDir string) error {
	baseDir, queueFilter := s.BaseDir, s.ProjectFilter
	state, err := s.loadWebState(baseDir, queueFilter)
	if err != nil {
		return err
	}
	logs := map[string]string{}
	reports := map[string]string{}
	configTargets := map[string][]webConfigTarget{}
	configs := map[string][]webprojection.ConfigFile{}
	allTargets, err := webConfigTargets(baseDir, "")
	if err != nil {
		return err
	}
	configTargets[""] = allTargets
	if files, configErr := s.loadWebConfigFiles(baseDir, "", ""); configErr == nil {
		configs[staticConfigKey("", "")] = files
	}
	for _, queue := range state.Queues {
		targets, targetErr := webConfigTargets(baseDir, queue.QueueName)
		if targetErr != nil {
			return targetErr
		}
		configTargets[queue.QueueName] = targets
		if files, configErr := s.loadWebConfigFiles(baseDir, queue.QueueName, ""); configErr == nil {
			configs[staticConfigKey(queue.QueueName, "")] = files
		}
		paths, pathErr := stateinternal.ResolveProjectPaths(baseDir, queue.QueueName)
		if pathErr != nil {
			continue
		}
		for _, run := range queue.Runs {
			if files, configErr := s.loadWebConfigFiles(baseDir, queue.QueueName, run.RunID); configErr == nil {
				configs[staticConfigKey(queue.QueueName, run.RunID)] = files
			}
			if report, reportErr := report.Build(s.Store, paths, run.RunID, "", false, "", true); reportErr == nil {
				reports[staticReportKey(queue.QueueName, run.RunID, "")] = report
			}
			for _, job := range run.Jobs {
				for _, stream := range []string{stateinternal.StdoutFileName, stateinternal.StderrFileName} {
					path, pathErr := webLogPath(paths.RunsDir, run.RunID, job.ID, "", stream)
					if pathErr == nil {
						if data, readErr := os.ReadFile(path); readErr == nil {
							logs[staticLogKey(queue.QueueName, run.RunID, job.ID, stream, "")] = string(data)
						}
					}
					if job.AttemptID != "" {
						attemptPath, attemptErr := webLogPath(paths.RunsDir, run.RunID, job.ID, job.AttemptID, stream)
						if attemptErr == nil {
							if attemptData, attemptReadErr := os.ReadFile(attemptPath); attemptReadErr == nil {
								logs[staticLogKey(queue.QueueName, run.RunID, job.ID, stream, job.AttemptID)] = string(attemptData)
							}
						}
					}
					for _, attempt := range job.Attempts {
						attemptPath, attemptErr := webLogPath(paths.RunsDir, run.RunID, job.ID, attempt.ID, stream)
						if attemptErr != nil {
							continue
						}
						if attemptData, attemptReadErr := os.ReadFile(attemptPath); attemptReadErr == nil {
							logs[staticLogKey(queue.QueueName, run.RunID, job.ID, stream, attempt.ID)] = string(attemptData)
						}
					}
				}
				if report, reportErr := report.Build(s.Store, paths, run.RunID, job.ID, false, "", true); reportErr == nil {
					reports[staticReportKey(queue.QueueName, run.RunID, job.ID)] = report
				}
			}
		}
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	logsJSON, err := json.Marshal(logs)
	if err != nil {
		return err
	}
	reportsJSON, err := json.Marshal(reports)
	if err != nil {
		return err
	}
	configTargetsJSON, err := json.Marshal(configTargets)
	if err != nil {
		return err
	}
	configsJSON, err := json.Marshal(configs)
	if err != nil {
		return err
	}
	var escapedState, escapedLogs, escapedReports, escapedConfigTargets, escapedConfigs bytes.Buffer
	json.HTMLEscape(&escapedState, stateJSON)
	json.HTMLEscape(&escapedLogs, logsJSON)
	json.HTMLEscape(&escapedReports, reportsJSON)
	json.HTMLEscape(&escapedConfigTargets, configTargetsJSON)
	json.HTMLEscape(&escapedConfigs, configsJSON)
	bootstrap := "<script>\n" + composeStaticBootstrap(escapedState.String(), escapedLogs.String(), escapedReports.String(), escapedConfigTargets.String(), escapedConfigs.String()) + "\n</script>"
	baseTemplate := s.webHTMLWithStaticBootstrap(bootstrap)
	template := strings.Replace(baseTemplate, `href="/web_styles.css"`, `href="web_styles.css"`, 1)
	template = strings.Replace(template, `href="/web_sidebar_styles.css"`, `href="web_sidebar_styles.css"`, 1)
	if template == baseTemplate {
		return errors.New("web HTML static stylesheet marker not found")
	}
	if bootstrap != "" && !strings.Contains(template, bootstrap) {
		return errors.New("web HTML static bootstrap marker not found")
	}
	if err := os.RemoveAll(outputDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if err := writeStaticWebPage(filepath.Join(outputDir, "index.html"), template); err != nil {
		return err
	}
	if err := writeStaticStylesheet(outputDir); err != nil {
		return err
	}
	projects, err := joblist.Projects(baseDir, queueFilter)
	if err != nil {
		return err
	}
	jobs, err := joblist.Collect(s.Store, baseDir, projects, time.Now(), joblist.DefaultSince)
	if err != nil {
		return err
	}
	joblist.Sort(jobs)
	if err := writeStaticWebPage(filepath.Join(outputDir, "jobs", "index.html"), jobsHTML("../", projects, jobs, joblist.DefaultSinceText, false, false)); err != nil {
		return err
	}
	if err := writeStaticStylesheet(filepath.Join(outputDir, "jobs")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, ".nojekyll"), nil, 0o644); err != nil {
		return err
	}
	for _, queue := range state.Queues {
		queuePath := filepath.Join(outputDir, "project", url.PathEscape(queue.QueueName))
		if err := writeStaticWebPage(filepath.Join(queuePath, "index.html"), template); err != nil {
			return err
		}
		if err := writeStaticStylesheet(queuePath); err != nil {
			return err
		}
		for _, run := range queue.Runs {
			runPath := filepath.Join(queuePath, "run", url.PathEscape(run.RunID))
			if err := writeStaticWebPage(filepath.Join(runPath, "index.html"), template); err != nil {
				return err
			}
			if err := writeStaticStylesheet(runPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func staticLogKey(queueName, runID, jobID, stream, attemptID string) string {
	return queueName + "/" + runID + "/" + jobID + "/" + stream + "/" + attemptID
}

func staticConfigKey(projectName, runID string) string {
	return projectName + "/" + runID
}

func webLogPath(runsDir, runID, jobID, attemptID, stream string) (string, error) {
	if stream != stateinternal.StdoutFileName && stream != stateinternal.StderrFileName {
		return "", fmt.Errorf("stream must be stdout or stderr")
	}
	if attemptID != "" {
		runDir, err := stateinternal.SafeJoin(runsDir, runID)
		if err != nil {
			return "", err
		}
		payload, decodeErr := stateinternal.DecodeAttemptID(attemptID)
		if decodeErr != nil || payload.RunID != runID || payload.JobID != jobID {
			return "", fmt.Errorf("attempt_id must identify this run and job")
		}
		jobDir, err := stateinternal.SpecificAttemptJobDir(runDir, jobID, attemptID)
		if err != nil {
			return "", err
		}
		return validatedLogPath(jobDir, stream)
	}
	runDir, resolvedJobID, err := resolveWebLogJob(runsDir, runID, jobID)
	if err != nil {
		return "", err
	}
	jobDir, err := stateinternal.LatestAttemptJobDir(runDir, resolvedJobID)
	if err != nil {
		return "", err
	}
	return validatedLogPath(jobDir, stream)
}

func validatedLogPath(jobDir, stream string) (string, error) {
	merged, err := stateinternal.ValidatedStateFile(jobDir, "output")
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(merged); statErr == nil {
		return merged, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	return stateinternal.ValidatedStateFile(jobDir, stream)
}

func resolveWebLogJob(runsDir, runID, jobID string) (string, string, error) {
	runDir, err := stateinternal.SafeJoin(runsDir, runID)
	if err != nil {
		return "", "", err
	}
	attemptID, attemptErr := stateinternal.LatestAttemptID(runDir, jobID)
	if attemptErr == nil && attemptID != "" {
		return runDir, jobID, nil
	}
	origin := stateinternal.LoadRunOrigin(runDir, jobID)
	if origin == nil {
		return runDir, jobID, nil
	}
	originRunDir, err := stateinternal.SafeJoin(runsDir, origin.RunID)
	if err != nil {
		return "", "", err
	}
	return originRunDir, origin.JobID, nil
}

func staticReportKey(projectName, runID, jobID string) string {
	return projectName + "/" + runID + "/" + jobID
}

func writeStaticWebPage(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

func writeStaticStylesheet(directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "web_styles.css"), []byte(webStylesCSS), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "web_sidebar_styles.css"), []byte(webSidebarStylesCSS), 0o644)
}

func (s site) loadWebQueueState(paths stateinternal.ProjectPaths) (webprojection.QueueState, error) {
	state, err := webprojection.LoadQueueState(webprojection.QueueLoader{
		ProjectName: paths.ProjectName,
		Queue:       func() (model.Queue, error) { return stateinternal.LoadQueue(paths.QueueFile) },
		Lock:        func() (model.LockInfo, error) { return stateinternal.LoadLock(paths.LockFile) },
		Runs: func() ([]string, error) {
			entries, err := os.ReadDir(paths.RunsDir)
			if os.IsNotExist(err) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(entries))
			for _, entry := range entries {
				if entry.IsDir() {
					ids = append(ids, entry.Name())
				}
			}
			return ids, nil
		},
		Summary: func(runID string) (model.RunSummary, error) {
			return stateinternal.LoadRunSummary(filepath.Join(paths.RunsDir, runID, "summary.json"))
		},
		Jobs: func(runID string, summary model.RunSummary) ([]webprojection.Job, error) {
			return webprojection.LoadRunJobs(s.Store, filepath.Join(paths.RunsDir, runID), summary, "")
		},
		Context: func(runID string) (model.RunContext, error) {
			return stateinternal.LoadContext(s.Store, filepath.Join(paths.RunsDir, runID))
		},
		Samples: func(runID string) []model.LoadSample {
			return stateinternal.ReadLoadSamples(loadSamplesPath(paths, runID))
		},
	})
	if err != nil {
		return webprojection.QueueState{}, err
	}
	formatWebQueueDisplayTimes(&state)
	return state, nil
}

func formatWebQueueDisplayTimes(state *webprojection.QueueState) {
	projection := webprojection.QueueState{QueueName: state.QueueName, Queue: state.Queue, Runs: state.Runs, RunnerStartedAt: state.RunnerStartedAt}
	webprojection.FormatQueueDisplayTimes(&projection)
	state.RunnerStartedAt = projection.RunnerStartedAt
	state.Queue = projection.Queue
	state.Runs = projection.Runs
}

func writeWebJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func writeWebError(writer http.ResponseWriter, err error) {
	http.Error(writer, err.Error(), http.StatusBadRequest)
}

func (s site) webHTML() string {
	return s.webHTMLWithStaticBootstrap("")
}

func (s site) webHTMLWithStaticBootstrap(bootstrap string) string {
	return composeWebHTML(s.Executors, s.Notifications, bootstrap)
}

func methodNotAllowed(writer http.ResponseWriter) {
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func forbiddenReadOnly(writer http.ResponseWriter) {
	http.Error(writer, "the web UI is read-only; restart with --allow-control to enable job control", http.StatusForbidden)
}

func loadSamplesPath(paths stateinternal.ProjectPaths, runID string) string {
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return ""
	}
	return filepath.Join(runDir, stateinternal.LoadSamplesFileName)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "-"
}
