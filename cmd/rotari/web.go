package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

const (
	webDefaultPort    = 8787
	webConfigFileName = "config.toml"
)

// webNotificationsDefault controls whether the served web UI's desktop
// notification toggle defaults to on or off; set once by cmdWeb.
var webNotificationsDefault = true

type webRun = webprojection.Run
type webJob = webprojection.Job
type webAttempt = webprojection.Attempt
type webTimelinePoint = webprojection.TimelinePoint

type webQueueState = webprojection.QueueState
type webServerState = webprojection.ServerState
type webConfigFile = webprojection.ConfigFile
type webState = webprojection.State

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
}

type webRemoveRequest struct {
	QueueName string `json:"project_name"`
	JobID     string `json:"job_id"`
}

type webCancelRequest struct {
	QueueName string `json:"project_name"`
	JobID     string `json:"job_id"`
}

type webJobControlRequest struct {
	QueueName string `json:"project_name"`
	JobID     string `json:"job_id"`
}

type webCancelRunRequest struct {
	QueueName string `json:"project_name"`
	RunID     string `json:"run_id"`
}

type webClearRequest struct {
	QueueName string `json:"project_name"`
	RunID     string `json:"run_id"`
}

// cmdWeb serves the embedded Web UI or writes a static export of project state.
func cmdWeb(args []string) int {
	fs := newFlagSet("web")
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	host := cliString(fs, "host", "127.0.0.1")
	port := cliInt(fs, "port", webDefaultPort)
	staticDir := cliString(fs, "static-dir", "")
	allowControl := cliBool(fs, "allow-control", true)
	authToken := cliString(fs, "auth-token", "")
	notifications := cliBool(fs, "notifications", true)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 || *port < 0 || *port > 65535 {
		printError("usage: " + cliUsage("web"))
		return 1
	}
	portExplicit := false
	fs.Visit(func(flag *flag.Flag) {
		portExplicit = portExplicit || flag.Name == "port"
	})
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	webNotificationsDefault = *notifications
	if *staticDir != "" {
		if err := generateStaticWeb(*staticDir, baseDir, *queueNameOption); err != nil {
			printErrorf("failed to generate static web: %v", err)
			return 1
		}
		return 0
	}
	if !isLoopbackWebHost(*host) && *authToken == "" {
		controlWarning := "job logs and environment variable names"
		if *allowControl {
			controlWarning = "job logs, environment variable names, and job control (cancel/suspend/resume/change/remove/copy) operations"
		}
		printErrorf("WARNING: --host %s exposes %s over unauthenticated HTTP.", *host, controlWarning)
	}
	handler := newWebHandler(baseDir, *queueNameOption, *allowControl)
	if *authToken != "" {
		handler = withWebAuthToken(handler, *authToken)
	}
	listener, err := listenWeb(*host, *port, !portExplicit)
	if err != nil {
		printErrorf("web server failed: %v", err)
		return 1
	}
	server := &http.Server{Handler: handler}
	go func() {
		<-interruptSignal()
		_ = server.Close()
	}()
	fmt.Printf("rotari web listening at http://%s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		printErrorf("web server failed: %v", err)
		return 1
	}
	return 0
}

func withWebAuthToken(next http.Handler, token string) http.Handler {
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

func isLoopbackWebHost(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func listenWeb(host string, port int, fallback bool) (net.Listener, error) {
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

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func interruptSignal() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	return signals
}

func newWebHandler(baseDir, queueFilter string, allowControl bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(webHTML()))
	})
	mux.HandleFunc("/web_styles.css", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = writer.Write([]byte(webStylesCSS))
	})
	mux.HandleFunc("/docs/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(cliDocsHTML("/")))
	})
	mux.HandleFunc("/docs", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/docs/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/jobs/", func(writer http.ResponseWriter, request *http.Request) {
		sinceText := request.URL.Query().Get("since")
		window, err := parseJobsSince(sinceText)
		if err != nil {
			writeWebError(writer, fmt.Errorf("invalid since duration %q", sinceText))
			return
		}
		if sinceText == "" {
			sinceText = defaultJobsSinceText
		}
		projects, err := jobsProjects(baseDir, queueFilter)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		rows, err := collectJobs(baseDir, projects, time.Now(), window)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		sortJobsRows(rows)
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(jobsHTML("/", rows, sinceText, true)))
	})
	mux.HandleFunc("/jobs", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/jobs/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/environment/", func(writer http.ResponseWriter, request *http.Request) {
		state, err := loadWebState(baseDir, queueFilter)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writer.Header().Set(headerContentType, "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(environmentHTML("/", state.Environments)))
	})
	mux.HandleFunc("/environment", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/environment/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/api/state", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		state, err := loadWebState(baseDir, queueFilter)
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
		files, err := loadWebConfigFiles(baseDir, request.URL.Query().Get("project_name"), request.URL.Query().Get("run_id"))
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
		path, err := generateWebConfig(baseDir, generate.QueueName, generate.Location)
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
		if !validWebID(projectName) || !validWebID(runID) {
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
			if !validWebID(jobID) {
				writeWebError(writer, fmt.Errorf("project_name and run_id are required; job_id must be valid when supplied"))
				return
			}
		}
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		if request.URL.Query().Get("job_ids") != "" {
			report, err := buildAIReportForJobs(paths, runID, jobIDs)
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
		report, err := buildAIReport(paths, runID, singleJobID, false)
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
		if !validWebID(queueName) || !validWebID(runID) || !validWebID(jobID) {
			writeWebError(writer, fmt.Errorf("project_name, run_id and job_id are required"))
			return
		}
		projectDir, err := stateinternal.SafeJoin(filepath.Join(baseDir, "projects"), queueName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		path, err := webLogPath(filepath.Join(projectDir, "runs"), runID, jobID, request.URL.Query().Get("attempt_id"))
		if err != nil {
			writeWebError(writer, err)
			return
		}
		// codeql[go/path-injection]: path is restricted by webLogPath to the validated output file.
		data, err := os.ReadFile(path) // NOSONAR: path is restricted by validatedStateFile to output.log
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
		if !validWebID(copyRequest.QueueName) || !validWebID(copyRequest.RunID) || (copyRequest.JobID != "" && !validWebID(copyRequest.JobID)) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		if copyRequest.JobID != "" {
			copyRequest.JobIDs = append(copyRequest.JobIDs, copyRequest.JobID)
		}
		for _, jobID := range copyRequest.JobIDs {
			if !validWebID(jobID) {
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
		message, err := copyRunToQueue(baseDir, copyRequest.QueueName, copyRequest.RunID, copyRequest.Selection, copyRequest.JobIDs, copyRequest.Append, copyRequest.Overwrite)
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
		if !validWebID(change.QueueName) || !validWebID(change.JobID) || len(change.Command) == 0 {
			writeWebError(writer, fmt.Errorf("project_name, job_id, and command are required"))
			return
		}
		message, err := changeBatchWithWorkingDirectory(baseDir, change.QueueName, "", change.JobID, "", change.Executor,
			change.ExecutorOptions, change.ClearExecutorOptions, change.Environment, change.ClearEnvironment, change.WorkingDirectory, change.ClearWorkingDirectory, change.SetJobName, change.DependsOn, change.ClearDependsOn, change.Command)
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
		if !validWebID(remove.QueueName) || !validWebID(remove.JobID) {
			writeWebError(writer, fmt.Errorf("project_name and job_id are required"))
			return
		}
		message, err := removeBatch(baseDir, remove.QueueName, "", []string{remove.JobID}, "")
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
		if !validWebID(clearRequest.QueueName) || !validWebID(clearRequest.RunID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		if err := clearRunHistory(baseDir, clearRequest.QueueName, clearRequest.RunID); err != nil {
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
		if !validWebID(cancel.QueueName) || !validWebID(cancel.JobID) {
			writeWebError(writer, fmt.Errorf("project_name and job_id are required"))
			return
		}
		message, err := cancelQueueJobs(baseDir, cancel.QueueName, []string{cancel.JobID}, false)
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
		if !validWebID(control.QueueName) || !validWebID(control.JobID) {
			writeWebError(writer, fmt.Errorf("project_name and job_id are required"))
			return
		}
		message, err := controlQueueJobs(baseDir, control.QueueName, []string{control.JobID}, "suspend")
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
		if !validWebID(control.QueueName) || !validWebID(control.JobID) {
			writeWebError(writer, fmt.Errorf("project_name and job_id are required"))
			return
		}
		message, err := controlQueueJobs(baseDir, control.QueueName, []string{control.JobID}, "resume")
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
		if !validWebID(cancel.QueueName) || !validWebID(cancel.RunID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		paths, err := resolvePaths(baseDir, cancel.QueueName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		lock, err := stateinternal.LoadLock(paths.LockFile)
		if err != nil {
			writeWebError(writer, fmt.Errorf("project %q is not running", cancel.QueueName))
			return
		}
		if lock.RunID != cancel.RunID {
			writeWebError(writer, fmt.Errorf("run %q is no longer running", cancel.RunID))
			return
		}
		message, err := cancelQueueJobs(baseDir, cancel.QueueName, nil, false)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{"message": message})
	})
	return mux
}

func loadWebConfigFiles(baseDir, projectName, runID string) ([]webConfigFile, error) {
	var paths []string
	if projectName == "" {
		if runID != "" {
			return nil, fmt.Errorf("project_name is required with run_id")
		}
		if path := effectiveConfigPath(baseDir, ""); path != "" {
			paths = []string{path}
		}
	} else {
		if !validWebID(projectName) {
			return nil, fmt.Errorf("invalid project_name %q", projectName)
		}
		if runID == "" {
			if path := effectiveConfigPath(baseDir, projectName); path != "" {
				paths = []string{path}
			}
		} else {
			if !validWebID(runID) {
				return nil, fmt.Errorf("invalid run_id %q", runID)
			}
			return loadRunConfigFiles(baseDir, projectName, runID)
		}
	}
	files := make([]webConfigFile, 0, len(paths))
	for _, path := range paths {
		// codeql[go/path-injection]: paths contain only resolved config files or validated run context entries.
		data, err := os.ReadFile(path) // NOSONAR: paths contain only the resolved global/project config files or validated run context entries.
		if err != nil {
			return nil, err
		}
		files = append(files, webConfigFile{Path: path, Content: string(data)})
	}
	return files, nil
}

func saveWebConfig(baseDir, projectName, content string) (string, error) {
	if projectName != "" && !validWebID(projectName) {
		return "", fmt.Errorf("invalid project_name %q", projectName)
	}
	path := effectiveConfigPath(baseDir, projectName)
	if path == "" {
		return "", fmt.Errorf("no config file exists to edit")
	}
	if _, err := parseConfigContent(path, []byte(content)); err != nil {
		return "", fmt.Errorf("invalid %s config: %w", strings.TrimPrefix(filepath.Ext(path), "."), err)
	}
	// codeql[go/path-injection]: path is returned by the allow-listed config resolver.
	if err := os.WriteFile(path, []byte(content), stateFileMode()); err != nil {
		return "", err
	}
	return path, nil
}

func loadRunConfigFiles(baseDir, projectName, runID string) ([]webConfigFile, error) {
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return nil, err
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return nil, err
	}
	context, err := stateinternal.LoadContext(jsonStore(), runDir)
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
	files := make([]webConfigFile, 0, len(context.ConfigSnapshotFiles))
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
		files = append(files, webConfigFile{Path: context.ConfigPaths[index], Content: string(data)})
	}
	return files, nil
}

func webConfigTargets(baseDir, projectName string) ([]webConfigTarget, error) {
	configHome, err := configHomeDir()
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
	if !validWebID(projectName) {
		return nil, fmt.Errorf("invalid project_name %q", projectName)
	}
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return nil, err
	}
	return append(targets, webConfigTarget{Location: "project", Path: filepath.Join(paths.ProjectDir, webConfigFileName)}), nil
}

func generateWebConfig(baseDir, projectName, location string) (string, error) {
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
	existing := configFilePaths(directory)
	if len(existing) > 1 {
		return "", fmt.Errorf("multiple config files found in %s: %s", directory, strings.Join(existing, ", "))
	}
	if len(existing) == 1 && filepath.Clean(existing[0]) != filepath.Clean(target) {
		return "", fmt.Errorf("config file %s already exists; remove it before generating config.toml", existing[0])
	}
	data, err := configTemplate("toml")
	if err != nil {
		return "", err
	}
	// codeql[go/path-injection]: directory is the resolved config directory.
	if err := os.MkdirAll(directory, stateDirMode()); err != nil {
		return "", err
	}
	// codeql[go/path-injection]: target is the resolved config file path.
	if err := os.WriteFile(target, data, stateFileMode()); err != nil {
		return "", err
	}
	return target, nil
}

func loadLegacyRunConfigFiles(baseDir, projectName string, configPaths []string) ([]webConfigFile, error) {
	allowed := make(map[string]bool)
	for _, path := range configPathsForRun(baseDir, projectName) {
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
	return []webConfigFile{{Path: path, Content: string(data)}}, nil
}

// loadWebState projects persisted server and project state into the Web API
// model consumed by the embedded and static Web UIs.
func loadWebState(baseDir, queueFilter string) (webState, error) {
	state := webState{BaseDir: baseDir, ConfigPath: effectiveConfigPath(baseDir, ""), Server: loadWebServerState(baseDir), Environments: environmentDefinitions(), UpdatedAt: nowRFC3339()}
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
			return webState{}, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				queueNames = append(queueNames, entry.Name())
			}
		}
		sort.Strings(queueNames)
	}
	for _, queueName := range queueNames {
		paths, err := resolvePaths(baseDir, queueName)
		if err != nil {
			return webState{}, err
		}
		queueState, err := loadWebQueueState(paths)
		if err != nil {
			return webState{}, err
		}
		queueState.ConfigPath = effectiveConfigPath(baseDir, queueName)
		state.Queues = append(state.Queues, queueState)
	}
	state.UpdatedAt = model.FormatDisplayTimestamp(state.UpdatedAt)
	return state, nil
}

func loadWebServerState(baseDir string) webServerState {
	state := webServerState{}
	if _, err := os.Stat(serverSocketPath(baseDir)); err == nil {
		state.SocketExists = true
	}
	data, err := os.ReadFile(serverPIDPath(baseDir))
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

func generateStaticWeb(outputDir, baseDir, queueFilter string) error {
	state, err := loadWebState(baseDir, queueFilter)
	if err != nil {
		return err
	}
	logs := map[string]string{}
	reports := map[string]string{}
	configTargets := map[string][]webConfigTarget{}
	configs := map[string][]webConfigFile{}
	allTargets, err := webConfigTargets(baseDir, "")
	if err != nil {
		return err
	}
	configTargets[""] = allTargets
	if files, configErr := loadWebConfigFiles(baseDir, "", ""); configErr == nil {
		configs[staticConfigKey("", "")] = files
	}
	for _, queue := range state.Queues {
		targets, targetErr := webConfigTargets(baseDir, queue.QueueName)
		if targetErr != nil {
			return targetErr
		}
		configTargets[queue.QueueName] = targets
		if files, configErr := loadWebConfigFiles(baseDir, queue.QueueName, ""); configErr == nil {
			configs[staticConfigKey(queue.QueueName, "")] = files
		}
		paths, pathErr := resolvePaths(baseDir, queue.QueueName)
		if pathErr != nil {
			continue
		}
		for _, run := range queue.Runs {
			if files, configErr := loadWebConfigFiles(baseDir, queue.QueueName, run.RunID); configErr == nil {
				configs[staticConfigKey(queue.QueueName, run.RunID)] = files
			}
			if report, reportErr := buildAIReport(paths, run.RunID, "", false); reportErr == nil {
				reports[staticReportKey(queue.QueueName, run.RunID, "")] = report
			}
			for _, job := range run.Jobs {
				path, pathErr := webLogPath(paths.RunsDir, run.RunID, job.ID, "")
				if pathErr != nil {
					continue
				}
				data, readErr := os.ReadFile(path)
				if readErr == nil {
					logs[staticLogKey(queue.QueueName, run.RunID, job.ID)] = string(data)
				}
				if job.AttemptID != "" {
					attemptPath, attemptErr := webLogPath(paths.RunsDir, run.RunID, job.ID, job.AttemptID)
					if attemptErr == nil {
						if attemptData, attemptReadErr := os.ReadFile(attemptPath); attemptReadErr == nil {
							logs[staticLogKey(queue.QueueName, run.RunID, job.ID, job.AttemptID)] = string(attemptData)
						}
					}
				}
				for _, attempt := range job.Attempts {
					attemptPath, attemptErr := webLogPath(paths.RunsDir, run.RunID, job.ID, attempt.ID)
					if attemptErr != nil {
						continue
					}
					attemptData, attemptReadErr := os.ReadFile(attemptPath)
					if attemptReadErr == nil {
						logs[staticLogKey(queue.QueueName, run.RunID, job.ID, attempt.ID)] = string(attemptData)
					}
				}
				if report, reportErr := buildAIReport(paths, run.RunID, job.ID, false); reportErr == nil {
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
	baseTemplate := webHTMLWithStaticBootstrap(bootstrap)
	template := strings.Replace(baseTemplate, `href="/web_styles.css"`, `href="web_styles.css"`, 1)
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
	if err := writeStaticWebPage(filepath.Join(outputDir, "docs", "index.html"), cliDocsHTML("../")); err != nil {
		return err
	}
	if err := writeStaticStylesheet(filepath.Join(outputDir, "docs")); err != nil {
		return err
	}
	projects, err := jobsProjects(baseDir, queueFilter)
	if err != nil {
		return err
	}
	jobs, err := collectJobs(baseDir, projects, time.Now(), defaultJobsSince)
	if err != nil {
		return err
	}
	sortJobsRows(jobs)
	if err := writeStaticWebPage(filepath.Join(outputDir, "jobs", "index.html"), jobsHTML("../", jobs, defaultJobsSinceText, false)); err != nil {
		return err
	}
	if err := writeStaticStylesheet(filepath.Join(outputDir, "jobs")); err != nil {
		return err
	}
	if err := writeStaticWebPage(filepath.Join(outputDir, "environment", "index.html"), environmentHTML("../", state.Environments)); err != nil {
		return err
	}
	if err := writeStaticStylesheet(filepath.Join(outputDir, "environment")); err != nil {
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

func staticLogKey(queueName, runID, jobID string, attemptIDs ...string) string {
	attemptID := ""
	if len(attemptIDs) > 0 {
		attemptID = attemptIDs[0]
	}
	return queueName + "/" + runID + "/" + jobID + "/" + attemptID
}

func staticConfigKey(projectName, runID string) string {
	return projectName + "/" + runID
}

func webLogPath(runsDir, runID, jobID, attemptID string) (string, error) {
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
		return stateinternal.ValidatedStateFile(jobDir, "output")
	}
	runDir, resolvedJobID, err := resolveWebLogJob(runsDir, runID, jobID)
	if err != nil {
		return "", err
	}
	jobDir, err := stateinternal.LatestAttemptJobDir(runDir, resolvedJobID)
	if err != nil {
		return "", err
	}
	return stateinternal.ValidatedStateFile(jobDir, "output")
}

func resolveWebLogJob(runsDir, runID, jobID string) (string, string, error) {
	runDir, err := stateinternal.SafeJoin(runsDir, runID)
	if err != nil {
		return "", "", err
	}
	jobDir, err := stateinternal.SafeJoin(runDir, jobID)
	if err != nil {
		return "", "", err
	}
	outputPath, err := stateinternal.ValidatedStateFile(jobDir, "output")
	if err != nil {
		return "", "", err
	}
	// codeql[go/path-injection]: outputPath is under the validated run/job directory.
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		return runDir, jobID, nil
	}
	origin := loadRunOrigin(runDir, jobID)
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
	return os.WriteFile(filepath.Join(directory, "web_styles.css"), []byte(webStylesCSS), 0o644)
}

func loadWebQueueState(paths pathSet) (webQueueState, error) {
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
			return loadWebJobs(filepath.Join(paths.RunsDir, runID), summary)
		},
		Context: func(runID string) (model.RunContext, error) {
			return stateinternal.LoadContext(jsonStore(), filepath.Join(paths.RunsDir, runID))
		},
		Samples: func(runID string) []model.LoadSample {
			return stateinternal.ReadLoadSamples(loadSamplesPath(paths, runID))
		},
	})
	if err != nil {
		return webQueueState{}, err
	}
	formatWebQueueDisplayTimes(&state)
	return state, nil
}

func formatWebQueueDisplayTimes(state *webQueueState) {
	projection := webprojection.QueueState{QueueName: state.QueueName, Queue: state.Queue, Runs: state.Runs, RunnerStartedAt: state.RunnerStartedAt}
	webprojection.FormatQueueDisplayTimes(&projection)
	state.RunnerStartedAt = projection.RunnerStartedAt
	state.Queue = projection.Queue
	state.Runs = projection.Runs
}

func loadWebJobs(runDir string, summary RunSummary, attemptIDs ...string) ([]webJob, error) {
	commands, err := stateinternal.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, err
	}
	selectedAttemptID := ""
	if len(attemptIDs) > 0 {
		selectedAttemptID = attemptIDs[0]
	}
	return webprojection.LoadJobs(jsonStore(), runDir, commands, summary, selectedAttemptID)
}

func validWebID(value string) bool {
	return stateinternal.IsValidPathElement(value)
}

func writeWebJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func writeWebError(writer http.ResponseWriter, err error) {
	http.Error(writer, err.Error(), http.StatusBadRequest)
}

func webHTML() string {
	return webHTMLWithStaticBootstrap("")
}

func webHTMLWithStaticBootstrap(bootstrap string) string {
	return composeWebHTML(executorNames(), bootstrap)
}

func methodNotAllowed(writer http.ResponseWriter) {
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func forbiddenReadOnly(writer http.ResponseWriter) {
	http.Error(writer, "the web UI is read-only; restart with --allow-control to enable job control", http.StatusForbidden)
}

func cliDocsHTML(homePath string) string {
	var builder strings.Builder
	for _, command := range cliCommandSpecs {
		builder.WriteString(`<section><h2 id="`)
		builder.WriteString(html.EscapeString(command.Name))
		builder.WriteString(`">rotari `)
		builder.WriteString(html.EscapeString(command.Name))
		builder.WriteString(`</h2><p>`)
		builder.WriteString(html.EscapeString(command.Description))
		builder.WriteString(`</p><pre>`)
		builder.WriteString(html.EscapeString(cliUsage(command.Name)))
		builder.WriteString(`</pre>`)
		if len(command.Flags) > 0 {
			builder.WriteString(`<h3>Options</h3><table><thead><tr><th>Option</th><th>Description</th><th>Values</th></tr></thead><tbody>`)
			for _, flagSpec := range command.Flags {
				value := flagSpec.ValueName
				if len(flagSpec.Values) > 0 {
					value = strings.Join(flagSpec.Values, ", ")
				}
				builder.WriteString(`<tr><td><code>--`)
				builder.WriteString(html.EscapeString(flagSpec.Name))
				builder.WriteString(`</code></td><td>`)
				builder.WriteString(html.EscapeString(flagSpec.Description))
				builder.WriteString(`</td><td>`)
				builder.WriteString(html.EscapeString(value))
				builder.WriteString(`</td></tr>`)
			}
			builder.WriteString(`</tbody></table>`)
		}
		if len(command.Subcommands) > 0 {
			builder.WriteString(`<h3>Subcommands</h3><table><thead><tr><th>Name</th><th>Description</th></tr></thead><tbody>`)
			for _, subcommand := range command.Subcommands {
				builder.WriteString(`<tr><td><code>`)
				builder.WriteString(html.EscapeString(subcommand.Name))
				builder.WriteString(`</code></td><td>`)
				builder.WriteString(html.EscapeString(subcommand.Description))
				builder.WriteString(`</td></tr>`)
			}
			builder.WriteString(`</tbody></table>`)
		}
		builder.WriteString(`</section>`)
	}
	return composeInfoHTML(cliDocsTemplateHTML, homePath, builder.String())
}

func environmentHTML(homePath string, environments []environmentDefinition) string {
	var builder strings.Builder
	builder.WriteString(`<section><table><thead><tr><th>Variable</th><th>Set</th><th>CLI</th><th>Job</th><th>Array</th><th>Description</th></tr></thead><tbody>`)
	for _, environment := range environments {
		value := "-"
		if environment.Set {
			value = "set"
		}
		builder.WriteString(`<tr><td><code>`)
		builder.WriteString(html.EscapeString(environment.Name))
		builder.WriteString(`</code></td><td>`)
		builder.WriteString(html.EscapeString(value))
		builder.WriteString(`</td><td>`)
		builder.WriteString(yesNo(environment.CLIDefault))
		builder.WriteString(`</td><td>`)
		builder.WriteString(yesNo(environment.Job))
		builder.WriteString(`</td><td>`)
		builder.WriteString(yesNo(environment.Array))
		builder.WriteString(`</td><td>`)
		builder.WriteString(html.EscapeString(environment.Description))
		builder.WriteString(`</td></tr>`)
	}
	builder.WriteString(`</tbody></table></section>`)
	return composeInfoHTML(environmentTemplateHTML, homePath, builder.String())
}
