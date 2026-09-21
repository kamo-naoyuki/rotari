package main

import (
	"bytes"
	"crypto/subtle"
	_ "embed"
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
)

const webDefaultPort = 8787

//go:embed web_template.html
var webTemplateHTML string

//go:embed web_app_core.js
var webAppCoreJS string

//go:embed web_app_actions.js
var webAppActionsJS string

//go:embed web_app_logs.js
var webAppLogsJS string

//go:embed web_app_tables.js
var webAppTablesJS string

//go:embed web_app_charts.js
var webAppChartsJS string

//go:embed web_app_bootstrap.js
var webAppBootstrapJS string

//go:embed web_styles.css
var webStylesCSS string

type webRun struct {
	RunSummary
	Jobs     []webJob           `json:"jobs"`
	CWD      string             `json:"cwd,omitempty"`
	Context  RunContext         `json:"context,omitempty"`
	Timeline []webTimelinePoint `json:"timeline,omitempty"`
	Running  bool               `json:"running"`
}

type webJob struct {
	ID               string     `json:"id"`
	ArrayTaskID      *int       `json:"array_task_id,omitempty"`
	ArrayFirst       int        `json:"array_first,omitempty"`
	ArrayLast        int        `json:"array_last,omitempty"`
	Name             string     `json:"name,omitempty"`
	Command          []string   `json:"command"`
	WorkingDirectory string     `json:"working_directory,omitempty"`
	Executor         string     `json:"executor,omitempty"`
	ExecutorOptions  []string   `json:"executor_options,omitempty"`
	DependsOn        []string   `json:"depends_on,omitempty"`
	Result           *JobResult `json:"result,omitempty"`
	Origin           *JobOrigin `json:"origin,omitempty"`
	SubmittedAt      string     `json:"submitted_at,omitempty"`
	FinishedAt       string     `json:"finished_at,omitempty"`
	SchedulerState   string     `json:"scheduler_state,omitempty"`
}

type webTimelinePoint struct {
	At       string `json:"at"`
	Pending  int    `json:"pending"`
	Running  int    `json:"running"`
	Finished int    `json:"finished"`
	Success  int    `json:"success"`
	Failed   int    `json:"failed"`
}

type webQueueState struct {
	QueueName       string   `json:"project_name"`
	ConfigPath      string   `json:"config_path,omitempty"`
	Queue           Queue    `json:"queue"`
	Runs            []webRun `json:"runs"`
	RunnerPID       int      `json:"runner_pid,omitempty"`
	RunningRunID    string   `json:"running_run_id,omitempty"`
	RunnerHost      string   `json:"runner_host,omitempty"`
	RunnerStartedAt string   `json:"runner_started_at,omitempty"`
}

type webServerState struct {
	PID           int  `json:"pid,omitempty"`
	PIDFileExists bool `json:"pid_file_exists"`
	SocketExists  bool `json:"socket_exists"`
}

type webConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type webState struct {
	BaseDir      string                  `json:"base_dir"`
	ConfigPath   string                  `json:"config_path,omitempty"`
	Queues       []webQueueState         `json:"projects"`
	Server       webServerState          `json:"server"`
	Environments []environmentDefinition `json:"environments"`
	UpdatedAt    string                  `json:"updated_at"`
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

func cmdWeb(args []string) int {
	fs := newFlagSet("web")
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	host := cliString(fs, "host", "127.0.0.1")
	port := cliInt(fs, "port", webDefaultPort)
	staticDir := cliString(fs, "static-dir", "")
	allowControl := cliBool(fs, "allow-control", true)
	authToken := cliString(fs, "auth-token", "")
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
	mux.HandleFunc("/api/report", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		projectName := request.URL.Query().Get("project_name")
		runID := request.URL.Query().Get("run_id")
		jobID := request.URL.Query().Get("job_id")
		if !validWebID(projectName) || !validWebID(runID) || (jobID != "" && !validWebID(jobID)) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required; job_id must be valid when supplied"))
			return
		}
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		report, err := buildAIReport(paths, runID, jobID, false)
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
		projectDir, err := joinValidatedPath(filepath.Join(baseDir, "projects"), queueName)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		runDir, err := joinValidatedPath(filepath.Join(projectDir, "runs"), runID)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		jobDir, err := validatedJobDir(runDir, jobID)
		if err != nil {
			writeWebError(writer, err)
			return
		}
		path, err := validatedStateFile(jobDir, "output")
		if err != nil {
			writeWebError(writer, err)
			return
		}
		data, err := os.ReadFile(path)
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
		var clear webClearRequest
		if err := json.NewDecoder(request.Body).Decode(&clear); err != nil {
			writeWebError(writer, err)
			return
		}
		if !validWebID(clear.QueueName) || !validWebID(clear.RunID) {
			writeWebError(writer, fmt.Errorf("project_name and run_id are required"))
			return
		}
		if err := clearRunHistory(baseDir, clear.QueueName, clear.RunID); err != nil {
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
		data, err := os.ReadFile(paths.lockFile) // NOSONAR: paths comes from resolvePaths after validWebID validation.
		if err != nil {
			writeWebError(writer, fmt.Errorf("project %q is not running", cancel.QueueName))
			return
		}
		var lock LockInfo
		if err := json.Unmarshal(data, &lock); err != nil {
			writeWebError(writer, fmt.Errorf("invalid running lock: %w", err))
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
		if path := globalConfigPath(); path != "" {
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
			var err error
			paths, err = loadRunConfigPaths(baseDir, projectName, runID)
			if err != nil {
				return nil, err
			}
		}
	}
	files := make([]webConfigFile, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path) // NOSONAR: paths contain only the resolved global/project config files or validated run context entries.
		if err != nil {
			return nil, err
		}
		files = append(files, webConfigFile{Path: path, Content: string(data)})
	}
	return files, nil
}

func loadRunConfigPaths(baseDir, projectName, runID string) ([]string, error) {
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return nil, err
	}
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(runDir, "context.json")) // NOSONAR: runDir is produced by validatedRunDir.
	if err != nil {
		return nil, err
	}
	var context RunContext
	if err := json.Unmarshal(data, &context); err != nil {
		return nil, err
	}
	allowed := make(map[string]bool)
	for _, path := range configPathsForRun(baseDir, projectName) {
		allowed[filepath.Clean(path)] = true
	}
	result := make([]string, 0, len(context.ConfigPaths))
	for _, path := range context.ConfigPaths {
		cleanPath := filepath.Clean(path)
		if allowed[cleanPath] {
			result = append(result, cleanPath)
		}
	}
	return result, nil
}

func loadWebState(baseDir, queueFilter string) (webState, error) {
	state := webState{BaseDir: baseDir, ConfigPath: globalConfigPath(), Server: loadWebServerState(baseDir), Environments: environmentDefinitions(), UpdatedAt: nowRFC3339()}
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
	state.UpdatedAt = formatDisplayTimestamp(state.UpdatedAt)
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
	for _, queue := range state.Queues {
		paths, pathErr := resolvePaths(baseDir, queue.QueueName)
		if pathErr != nil {
			continue
		}
		for _, run := range queue.Runs {
			if report, reportErr := buildAIReport(paths, run.RunID, "", false); reportErr == nil {
				reports[staticReportKey(queue.QueueName, run.RunID, "")] = report
			}
			for _, job := range run.Jobs {
				jobDir, pathErr := validatedJobDir(filepath.Join(baseDir, "projects", queue.QueueName, "runs", run.RunID), job.ID)
				if pathErr != nil {
					continue
				}
				path := filepath.Join(jobDir, "output")
				data, readErr := os.ReadFile(path)
				if readErr == nil {
					logs[staticLogKey(queue.QueueName, run.RunID, job.ID)] = string(data)
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
	var escapedState, escapedLogs, escapedReports bytes.Buffer
	json.HTMLEscape(&escapedState, stateJSON)
	json.HTMLEscape(&escapedLogs, logsJSON)
	json.HTMLEscape(&escapedReports, reportsJSON)
	bootstrap := fmt.Sprintf(`<script>
window.__ROTARI_STATIC_STATE__=%s;
window.__ROTARI_STATIC_LOGS__=%s;
window.__ROTARI_STATIC_REPORTS__=%s;
window.fetch=async function(input, init){
  const request=new URL(input, window.location.href);
  if(request.pathname.endsWith('/api/state')) return new Response(JSON.stringify(window.__ROTARI_STATIC_STATE__), {headers:{'Content-Type':'application/json'}});
  if(request.pathname.endsWith('/api/log')) {
	const key=staticLogKey(request.searchParams.get('project_name'), request.searchParams.get('run_id'), request.searchParams.get('job_id'));
    return new Response(window.__ROTARI_STATIC_LOGS__[key] || '', {headers:{'Content-Type':'text/plain'}});
  }
	if(request.pathname.endsWith('/api/report')) {
	const key=staticReportKey(request.searchParams.get('project_name'), request.searchParams.get('run_id'), request.searchParams.get('job_id'));
		return new Response(window.__ROTARI_STATIC_REPORTS__[key] || 'Report not found', {status:window.__ROTARI_STATIC_REPORTS__[key]?200:404, headers:{'Content-Type':'text/markdown'}});
	}
  return new Response('This is a read-only static demo.', {status:405});
};
function staticLogKey(queue, run, job){return [queue, run, job].join('/');}
function staticReportKey(project, run, job){return [project, run, job || ''].join('/');}
function staticRootPath(){const pathname=window.location.pathname;const parts=pathname.split('/').filter(Boolean);const projectIndex=parts.indexOf('project');if(projectIndex>=0)return '/'+parts.slice(0,projectIndex).join('/');if(pathname.endsWith('/index.html'))return '/'+parts.slice(0,-1).join('/');if(pathname.endsWith('/'))return parts.length?'/'+parts.join('/'):'';return '/'+parts.slice(0,-1).join('/')}
function routeParts(){const root=staticRootPath().split('/').filter(Boolean);return window.location.pathname.split('/').filter(Boolean).slice(root.length)}
function staticPath(path){const root=staticRootPath().replace(/\/$/,'');return path===root||path.startsWith(root+'/')?path:root+path}
function rewriteStaticLinks(){document.querySelectorAll('a[href^="/"]').forEach(link=>{link.setAttribute('href',staticPath(link.getAttribute('href')))})}
rewriteStaticLinks();
new MutationObserver(rewriteStaticLinks).observe(document.body,{childList:true,subtree:true});
</script>`, escapedState.String(), escapedLogs.String(), escapedReports.String())
	baseTemplate := webHTMLWithStaticBootstrap(bootstrap)
	template := baseTemplate
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

func staticLogKey(queueName, runID, jobID string) string {
	return queueName + "/" + runID + "/" + jobID
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
	queue, err := loadQueue(paths.queueFile)
	if err != nil {
		return webQueueState{}, err
	}
	state := webQueueState{QueueName: paths.queueName, Queue: queue, Runs: make([]webRun, 0)}
	runningStartedAt := ""
	if data, err := os.ReadFile(paths.lockFile); err == nil {
		var lock LockInfo
		if json.Unmarshal(data, &lock) == nil {
			state.RunningRunID = lock.RunID
			state.RunnerPID = lock.PID
			state.RunnerHost = lock.Host
			state.RunnerStartedAt = lock.StartedAt
			runningStartedAt = lock.StartedAt
		}
	}
	entries, err := os.ReadDir(paths.runsDir)
	if err != nil && !os.IsNotExist(err) {
		return webQueueState{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		summary, err := loadRunSummary(filepath.Join(paths.runsDir, runID, "summary.json"))
		if err != nil {
			summary = RunSummary{RunID: runID, Status: "running", StartedAt: runningStartedAt}
		}
		if summary.RunID == "" {
			summary.RunID = runID
		}
		jobs, err := loadWebJobs(filepath.Join(paths.runsDir, runID), summary)
		if err != nil {
			return webQueueState{}, err
		}
		context := RunContext{}
		if data, contextErr := os.ReadFile(filepath.Join(paths.runsDir, runID, "context.json")); contextErr == nil {
			_ = json.Unmarshal(data, &context)
		}
		context.LoadSamples = readLoadSamples(loadSamplesPath(paths, runID))
		state.Runs = append(state.Runs, webRun{RunSummary: summary, Jobs: jobs, CWD: context.CWD, Context: context, Timeline: buildWebTimeline(summary, jobs), Running: runID == state.RunningRunID})
	}
	sort.Slice(state.Runs, func(i, j int) bool { return state.Runs[i].RunID > state.Runs[j].RunID })
	formatWebQueueDisplayTimes(&state)
	return state, nil
}

func formatWebQueueDisplayTimes(state *webQueueState) {
	state.RunnerStartedAt = formatDisplayTimestamp(state.RunnerStartedAt)
	for index := range state.Queue.Commands {
		origin := state.Queue.Commands[index].Origin
		if origin == nil {
			continue
		}
		origin.SubmittedAt = formatDisplayTimestamp(origin.SubmittedAt)
		origin.FinishedAt = formatDisplayTimestamp(origin.FinishedAt)
	}
	for index := range state.Runs {
		run := &state.Runs[index]
		run.StartedAt = formatDisplayTimestamp(run.StartedAt)
		run.FinishedAt = formatDisplayTimestamp(run.FinishedAt)
		for jobIndex := range run.Jobs {
			job := &run.Jobs[jobIndex]
			job.SubmittedAt = formatDisplayTimestamp(job.SubmittedAt)
			job.FinishedAt = formatDisplayTimestamp(job.FinishedAt)
			if job.Origin != nil {
				job.Origin.SubmittedAt = formatDisplayTimestamp(job.Origin.SubmittedAt)
				job.Origin.FinishedAt = formatDisplayTimestamp(job.Origin.FinishedAt)
			}
		}
	}
}

func loadWebJobs(runDir string, summary RunSummary) ([]webJob, error) {
	results := make(map[string]JobResult, len(summary.Results))
	for _, result := range summary.Results {
		results[result.ID] = result
	}
	commands, err := loadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, err
	}
	origins := make(map[string]*JobOrigin, len(commands.Commands))
	for _, command := range commands.Commands {
		origins[command.ID] = command.Origin
		for taskID, origin := range command.TaskOrigins {
			origins[taskID] = origin
		}
	}
	taskJobs := queueToJobs(commands.Commands)
	webJobs := make([]webJob, 0, len(taskJobs))
	for _, jobSpec := range taskJobs {
		jobDir, pathErr := validatedJobDir(runDir, jobSpec.ID)
		if pathErr != nil {
			return nil, fmt.Errorf("invalid job ID %q: %w", jobSpec.ID, pathErr)
		}
		origin := origins[jobSpec.ID]
		submittedAt, finishedAt := webJobTimestamps(runDir, jobSpec.ID, origin)
		job := webJob{ID: jobSpec.ID, Name: jobSpec.Name, Command: jobSpec.Command, WorkingDirectory: jobSpec.WorkingDirectory, Executor: jobSpec.Executor, ExecutorOptions: jobSpec.ExecutorOptions, DependsOn: jobSpec.DependsOn, Origin: origin, ArrayTaskID: jobSpec.ArrayTaskID, ArrayFirst: jobSpec.ArrayFirst, ArrayLast: jobSpec.ArrayLast, SubmittedAt: submittedAt, FinishedAt: finishedAt, SchedulerState: loadSchedulerStatus(jobDir)}
		if result, ok := results[jobSpec.ID]; ok {
			job.Result = &result
		} else if status, ok := loadSlurmStatus(filepath.Join(jobDir, stateFileStatusJSON)); ok && jobStatusTerminal(status) {
			job.Result = &JobResult{ID: jobSpec.ID, Command: jobSpec.Command, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
			if job.FinishedAt == "" {
				job.FinishedAt = status.FinishedAt
			}
		} else if result, ok := loadLocalJobResult(jobDir, jobSpec); ok {
			job.Result = &result
		} else if exitCode, ok := loadTerminalSchedulerState(jobDir); ok {
			job.Result = &JobResult{ID: jobSpec.ID, Command: jobSpec.Command, ExitCode: exitCode}
		}
		webJobs = append(webJobs, job)
		delete(results, jobSpec.ID)
	}
	for _, result := range summary.Results {
		if _, exists := results[result.ID]; !exists {
			continue
		}
		resultCopy := result
		webJobs = append(webJobs, webJob{ID: result.ID, Command: result.Command, Result: &resultCopy, SubmittedAt: readJobTimestamp(runDir, result.ID, "submitted_at"), FinishedAt: readJobTimestamp(runDir, result.ID, "finished_at")})
	}
	return webJobs, nil
}

func loadLocalJobResult(jobDir string, job JobSpec) (JobResult, bool) {
	finishedPath, err := validatedStateFile(jobDir, stateFileFinishedAt)
	if err != nil {
		return JobResult{}, false
	}
	if _, err := os.Stat(finishedPath); err != nil {
		return JobResult{}, false
	}
	statusPath, err := validatedStateFile(jobDir, stateFileStatus)
	if err != nil {
		return JobResult{}, false
	}
	data, err := os.ReadFile(statusPath) // NOSONAR: statusPath is restricted by validatedStateFile to status.
	if err != nil {
		return JobResult{}, false
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return JobResult{}, false
	}
	return JobResult{ID: job.ID, Command: job.Command, ExitCode: exitCode}, true
}

func webJobTimestamps(runDir, jobID string, origin *JobOrigin) (string, string) {
	submittedAt := readJobTimestamp(runDir, jobID, "submitted_at")
	finishedAt := readJobTimestamp(runDir, jobID, "finished_at")
	if finishedAt == "" {
		if status, ok := loadSlurmStatus(filepath.Join(runDir, jobID, "status.json")); ok {
			finishedAt = status.FinishedAt
		}
	}
	if origin == nil || (submittedAt != "" && finishedAt != "") {
		return submittedAt, finishedAt
	}
	if submittedAt == "" {
		submittedAt = origin.SubmittedAt
	}
	if finishedAt == "" {
		finishedAt = origin.FinishedAt
	}
	sourceRunDir, err := validatedRunDir(pathSet{runsDir: filepath.Dir(runDir)}, origin.RunID)
	if err == nil {
		if submittedAt == "" {
			submittedAt = readJobTimestamp(sourceRunDir, origin.JobID, "submitted_at")
		}
		if finishedAt == "" {
			finishedAt = readJobTimestamp(sourceRunDir, origin.JobID, "finished_at")
		}
	}
	return submittedAt, finishedAt
}

func readJobTimestamp(runDir, jobID, name string) string {
	if name != "submitted_at" && name != "finished_at" {
		return ""
	}
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(jobDir, name)) // NOSONAR: jobDir is produced by validatedJobDir and name is allowlisted above.
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func buildWebTimeline(summary RunSummary, jobs []webJob) []webTimelinePoint {
	type event struct {
		at       string
		pending  int
		running  int
		finished int
		success  int
		failed   int
	}
	events := make([]event, 0, len(jobs)*2)
	initial := webTimelinePoint{At: summary.StartedAt}
	for _, job := range jobs {
		if job.Result != nil && (job.Origin != nil || (job.SubmittedAt == "" && job.FinishedAt == "")) {
			initial.Finished++
			if job.Result.ExitCode == 0 {
				initial.Success++
			} else {
				initial.Failed++
			}
			continue
		}
		initial.Pending++
		if job.SubmittedAt != "" {
			events = append(events, event{at: job.SubmittedAt, pending: -1, running: 1})
		}
		if job.FinishedAt != "" {
			finished := event{at: job.FinishedAt, running: -1, finished: 1}
			if job.Result != nil && job.Result.ExitCode == 0 {
				finished.success = 1
			} else {
				finished.failed = 1
			}
			events = append(events, finished)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	points := []webTimelinePoint{initial}
	pending, running, finished, success, failed := initial.Pending, initial.Running, initial.Finished, initial.Success, initial.Failed
	for i := 0; i < len(events); {
		at := events[i].at
		event := event{at: at}
		for i < len(events) && events[i].at == at {
			event.pending += events[i].pending
			event.running += events[i].running
			event.finished += events[i].finished
			event.success += events[i].success
			event.failed += events[i].failed
			i++
		}
		pending += event.pending
		running += event.running
		finished += event.finished
		success += event.success
		failed += event.failed
		points = append(points, webTimelinePoint{At: event.at, Pending: pending, Running: running, Finished: finished, Success: success, Failed: failed})
	}
	return points
}

func validWebID(value string) bool {
	return isValidPathElement(value)
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
	executorJSON, _ := json.Marshal(executorNames())
	webAppJS := strings.Join([]string{webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppTablesJS, webAppChartsJS, webAppBootstrapJS}, "\n")
	template := strings.Replace(webTemplateHTML, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "__ROTARI_EXECUTORS__", string(executorJSON), 1)
	template = strings.Replace(template, "__ROTARI_STATIC_BOOTSTRAP__", bootstrap, 1)
	return template
}

func methodNotAllowed(writer http.ResponseWriter) {
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

func forbiddenReadOnly(writer http.ResponseWriter) {
	http.Error(writer, "the web UI is read-only; restart with --allow-control to enable job control", http.StatusForbidden)
}

func cliDocsHTML(homePath string) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>rotari CLI documentation</title><style>
:root{color-scheme:dark;--bg:#10151b;--panel:#18212b;--line:#2d3a47;--text:#e8eef4;--muted:#94a3b3;--accent:#b8d9f2}*{box-sizing:border-box}body{margin:0;background:linear-gradient(135deg,#10151b,#182733);color:var(--text);font:15px/1.5 ui-sans-serif,system-ui,sans-serif}main{max-width:1100px;margin:0 auto;padding:36px 22px}header{display:flex;justify-content:space-between;align-items:end;border-bottom:1px solid var(--line);padding-bottom:20px;margin-bottom:24px}h1{margin:0;font-size:32px;letter-spacing:.04em;display:flex;align-items:center;gap:10px}.brand-icon{width:.85em;height:.85em}h2{margin:0 0 8px;color:var(--accent)}h3{margin:22px 0 8px}.meta{color:var(--muted)}a{color:var(--accent)}section{background:rgba(24,33,43,.9);border:1px solid var(--line);padding:18px;margin-bottom:16px}pre{white-space:pre-wrap;background:#0b1015;border:1px solid var(--line);padding:12px;overflow:auto}table{width:100%;border-collapse:collapse}th,td{text-align:left;border-bottom:1px solid var(--line);padding:8px}th{color:var(--muted);font-size:12px;text-transform:uppercase}code{color:var(--accent)}
</style></head><body><main><header><div><h1>` + brandIcon() + `rotari CLI</h1><div class="meta">Generated from the command metadata used by the binary</div></div><a href="`)
	builder.WriteString(html.EscapeString(homePath))
	builder.WriteString(`">Web UI</a></header><p class="meta">Every command below is available from <code>rotari</code>. The flag descriptions and usage lines are shared with shell completion and command help.</p>`)
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
	builder.WriteString(`</main></body></html>`)
	return strings.Replace(builder.String(), `<title>rotari CLI documentation</title>`, `<title>rotari CLI documentation</title>`+faviconLinks(), 1)
}

func environmentHTML(homePath string, environments []environmentDefinition) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>rotari environment variables</title><style>
:root{color-scheme:dark;--bg:#10151b;--panel:#18212b;--line:#2d3a47;--text:#e8eef4;--muted:#94a3b3;--accent:#b8d9f2}*{box-sizing:border-box}body{margin:0;background:linear-gradient(135deg,#10151b,#182733);color:var(--text);font:15px/1.5 ui-sans-serif,system-ui,sans-serif}main{max-width:1100px;margin:0 auto;padding:36px 22px}header{display:flex;justify-content:space-between;align-items:end;border-bottom:1px solid var(--line);padding-bottom:20px;margin-bottom:24px}h1{margin:0;font-size:32px;letter-spacing:.04em;display:flex;align-items:center;gap:10px}.brand-icon{width:.85em;height:.85em}.meta{color:var(--muted)}a{color:var(--accent)}section{background:rgba(24,33,43,.9);border:1px solid var(--line);padding:18px;margin-bottom:16px}table{width:100%;border-collapse:collapse}th,td{text-align:left;border-bottom:1px solid var(--line);padding:8px}th{color:var(--muted);font-size:12px;text-transform:uppercase}code{color:var(--accent)}
</style></head><body><main><header><div><h1>` + brandIcon() + `rotari environment variables</h1><div class="meta">Variables read by the CLI, jobs, and array tasks</div></div><a href="`)
	builder.WriteString(html.EscapeString(homePath))
	builder.WriteString(`">Web UI</a></header><section><table><thead><tr><th>Variable</th><th>Set</th><th>CLI</th><th>Job</th><th>Array</th><th>Description</th></tr></thead><tbody>`)
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
	builder.WriteString(`</tbody></table></section></main></body></html>`)
	return strings.Replace(builder.String(), `<title>rotari environment variables</title>`, `<title>rotari environment variables</title>`+faviconLinks(), 1)
}
