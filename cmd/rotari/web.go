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

//go:embed web_app.js
var webAppJS string

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
	baseTemplate := webHTML()
	staticTemplate := strings.ReplaceAll(baseTemplate, "location.pathname.split('/').filter(Boolean)", "routeParts()")
	staticTemplate = strings.ReplaceAll(staticTemplate, "location.pathname!=='/'&&location.pathname!==''", "routeParts().length")
	staticTemplate = strings.ReplaceAll(staticTemplate, "const nextState=await r.json();const nextStateJSON=JSON.stringify(nextState);if(nextStateJSON===stateJSON)return;stateJSON=nextStateJSON;state=nextState;render()", "const nextState=await r.json();const nextStateJSON=JSON.stringify(nextState);if(nextStateJSON===stateJSON)return;stateJSON=nextStateJSON;state=nextState;render();rewriteStaticLinks()")
	template := strings.Replace(staticTemplate, "<script>\nconst executorNames=", bootstrap+"<script>\nconst executorNames=", 1)
	if template == baseTemplate {
		return errors.New("web HTML script marker not found")
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
	if err := writeStaticWebPage(filepath.Join(outputDir, "docs", "index.html"), cliDocsHTML("../")); err != nil {
		return err
	}
	if err := writeStaticWebPage(filepath.Join(outputDir, "environment", "index.html"), environmentHTML("../", state.Environments)); err != nil {
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
		for _, run := range queue.Runs {
			runPath := filepath.Join(queuePath, "run", url.PathEscape(run.RunID))
			if err := writeStaticWebPage(filepath.Join(runPath, "index.html"), template); err != nil {
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
	executorJSON, _ := json.Marshal(executorNames())
	template := strings.Replace(webTemplateHTML, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "<title>rotari</title>", "<title>rotari</title>"+faviconLinks(), 1)
	template = strings.Replace(template, "<h1><!--brand-icon-->rotari Web</h1>", "<h1>"+brandIcon()+"rotari Web</h1>", 1)
	template = strings.Replace(template,
		`<header><strong>Output</strong><button onclick="closeOutputModal()">Close</button></header>`,
		`<header><strong>Output</strong><div class="modal-actions"><button id="copy-modal" onclick="copyModalOutput(this)">Copy</button><button id="copy-tail" onclick="copyLogTail(this)" hidden>Copy last 100 lines</button><button id="open-chatgpt" onclick="openAI('https://chatgpt.com/',this)" hidden>Open ChatGPT</button><button id="open-gemini" onclick="openAI('https://gemini.google.com/app',this)" hidden>Open Gemini</button><button id="open-claude" onclick="openAI('https://claude.ai/new',this)" hidden>Open Claude</button><button onclick="closeOutputModal()">Close</button></div></header><p id="report-note" class="meta" hidden>Markdown report for pasting into an AI assistant. Nothing is sent to external services automatically.</p>`, 1)
	template = strings.Replace(template, "<script>\nlet state;", "<script>\nconst executorNames="+string(executorJSON)+";\nlet state;", 1)
	template = strings.Replace(template, "rotari copy", "rotari retry", -1)
	template = strings.Replace(template, "\\nrotari rerun'+basedir+' --project-name '+shellQuote(queueName)+' --job-id JOB_ID", "", -1)
	template = strings.Replace(template, "rotari rerun", "rotari retry", -1)
	// "retry" already means --failed --unfinished, so drop the now-redundant flag.
	template = strings.Replace(template, " --failed", "", -1)
	template = strings.Replace(template, "rerun failed jobs", "retry failed or unfinished jobs", -1)
	template = strings.NewReplacer(
		"rotari retry --queue-name '+shellQuote(q.project_name)+' --run-id '+shellQuote(runID)",
		"rotari retry --run-id '+shellQuote(runID)",
		"rotari retry'+basedir+' --queue-name '+shellQuote(queueName)+' --run-id '+shellQuote(runID)",
		"rotari retry --run-id '+shellQuote(runID)",
		"rotari retry'+basedir+' --queue-name '+shellQuote(queueName)",
		"rotari retry --run-id '+shellQuote(runID)",
		"retry failed or unfinished jobs from the latest run",
		"retry failed or unfinished jobs from this run",
		"retry failed or unfinished jobs from this older run",
		"retry failed or unfinished jobs from this run",
	).Replace(template)
	template = strings.Replace(template,
		`<select class="executor-input"><option value="local">local</option><option value="slurm">slurm</option></select>`,
		`<select class="executor-input">'+executorNames.map(name=>'<option value="'+esc(name)+'">'+esc(name)+'</option>').join('')+'</select>`, 1)
	template = strings.Replace(template,
		`row.children[5].innerHTML='<input class="command-input" value="'+esc(JSON.stringify(job.command))+'">';`,
		`rowCell(row,'command').innerHTML='<input class="command-input" value="'+esc(JSON.stringify(job.command))+'">';rowCell(row,'working_directory').innerHTML='<input class="working-directory-input" placeholder="working directory" value="'+esc(job.working_directory||'')+'">';`, 1)
	template = strings.Replace(template,
		`function addQueueEditors(queue,commands){`,
		`function rowCell(row,key){const headers=[...row.closest('table').querySelectorAll('thead th')];const index=headers.findIndex(header=>header.dataset.sort===key);return index<0?null:row.children[index]}
function ensureQueueWorkingDirectoryColumn(commands){const table=document.querySelector('.web-queue-commands table');if(!table)return;const headerRow=table.querySelector('thead tr');if(!headerRow||headerRow.querySelector('[data-sort="working_directory"]'))return;const header=document.createElement('th');header.dataset.sort='working_directory';header.textContent='Working directory';const commandHeader=headerRow.querySelector('[data-sort="command"]');if(commandHeader)headerRow.insertBefore(header,commandHeader);else headerRow.append(header);table.querySelectorAll('tbody tr').forEach((row,index)=>{const cell=document.createElement('td');cell.textContent=commands[index]&&commands[index].working_directory||'-';const commandCell=rowCell(row,'command');if(commandCell)row.insertBefore(cell,commandCell);else row.append(cell)})}
function addQueueEditors(queue,commands){`, 1)
	template = strings.Replace(template,
		`depends_on:depends,clear_depends_on:depends.length===0`,
		`depends_on:depends,clear_depends_on:depends.length===0,working_directory:row.querySelector('.working-directory-input').value,clear_working_directory:row.querySelector('.working-directory-input').value===''`, 1)
	template = strings.Replace(template,
		`addQueueEditors(queue,commands);enhanceQueueSourceContext(commands)`,
		`ensureQueueWorkingDirectoryColumn(commands);addQueueEditors(queue,commands);enhanceQueueSourceContext(commands)`, 1)
	template = strings.Replace(template,
		`row.children[0].innerHTML='<input class="job-name-input" value="'+esc(job.name||'')+'"><div class="meta">'+esc(job.id)+'</div>';`,
		`rowCell(row,'name').innerHTML='<input class="job-name-input" value="'+esc(job.name||'')+'"><div class="meta">'+esc(job.id)+'</div>';`, 1)
	template = strings.Replace(template,
		`row.children[2].innerHTML='<select class="executor-input"><option value="local">local</option><option value="slurm">slurm</option></select>';row.children[2].querySelector('select').value=job.executor||'local';`,
		`rowCell(row,'executor').innerHTML='<select class="executor-input"><option value="local">local</option><option value="slurm">slurm</option></select>';rowCell(row,'executor').querySelector('select').value=job.executor||'local';`, 1)
	template = strings.Replace(template,
		`row.children[2].innerHTML='<select class="executor-input">'+executorNames.map(name=>'<option value="'+esc(name)+'">'+esc(name)+'</option>').join('')+'</select>';row.children[2].querySelector('select').value=job.executor||'local';`,
		`rowCell(row,'executor').innerHTML='<select class="executor-input">'+executorNames.map(name=>'<option value="'+esc(name)+'">'+esc(name)+'</option>').join('')+'</select>';rowCell(row,'executor').querySelector('select').value=job.executor||'local';`, 1)
	template = strings.Replace(template,
		`row.children[3].innerHTML='<input class="executor-option-input" value="'+esc(JSON.stringify(job.executor_options||[]))+'">';row.children[4].innerHTML='<input class="depends-input" value="'+esc(JSON.stringify(job.depends_on||[]))+'">';`,
		`rowCell(row,'options').innerHTML='<input class="executor-option-input" value="'+esc(JSON.stringify(job.executor_options||[]))+'">';rowCell(row,'depends').innerHTML='<input class="depends-input" value="'+esc(JSON.stringify(job.depends_on||[]))+'">';`, 1)
	template = strings.Replace(template,
		`<td>'+esc(dependencies||'-')+'</td><td class="command">`,
		`<td>'+esc(dependencies||'-')+'</td><td>'+esc(j.working_directory||'-')+'</td><td class="command">`, 1)
	template = strings.Replace(template,
		`<th>Job name / ID</th><th>Executor</th><th>Executor options</th><th>Dependencies</th><th>Command</th><th>Started</th><th>Finished</th><th>Exit / error</th><th></th>`,
		`<th data-sort="name"><input id="select-all-jobs" type="checkbox" aria-label="Select all jobs"> Job name / ID</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="depends">Dependencies</th><th data-sort="working_directory">Working directory</th><th data-sort="command">Command</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="exit">Exit / error</th><th data-sort="output"></th>`, 1)
	template = strings.Replace(template,
		`return '<tr><td><strong>'+jobName+'</strong><div class="meta">'+esc(j.id)+'</div></td>`,
		`return '<tr data-job-id="'+esc(j.id)+'"><td><input class="job-selection" type="checkbox" aria-label="Select '+esc(j.id)+'"> <strong>'+jobName+'</strong><div class="meta">'+esc(j.id)+'</div></td>`, 1)
	template = strings.Replace(template,
		`controls.innerHTML='<button onclick="copyRun(\'`+"'"+`'+esc(queue.queue_name)+\'`+"'"+`\',\'`+"'"+`'+esc(runID)+\'`+"'"+`\',\'failed\')">Copy failed jobs</button><button onclick="copyRun(\'`+"'"+`'+esc(queue.queue_name)+\'`+"'"+`\',\'`+"'"+`'+esc(runID)+\'`+"'"+`\',\'all\')">Copy all jobs</button>'`,
		`controls.innerHTML='<button class="create-selected" disabled onclick="copySelectedJobs(\'`+"'"+`'+esc(queue.queue_name)+\'`+"'"+`\',\'`+"'"+`'+esc(runID)+\'`+"'"+`\',false)">Create</button><button class="append-selected" disabled onclick="copySelectedJobs(\'`+"'"+`'+esc(queue.queue_name)+\'`+"'"+`\',\'`+"'"+`'+esc(runID)+\'`+"'"+`\',true)">Append</button>'`, 1)
	template = strings.Replace(template,
		`async function copyRun(queue,run,selection){`,
		`function selectedRunJobIDs(){return [...document.querySelectorAll('.job-selection:checked')].map(input=>input.closest('tr').dataset.jobId)}
function updateSelectedRunJobs(){const selected=selectedRunJobIDs();const all=[...document.querySelectorAll('.job-selection')];const selectAll=document.getElementById('select-all-jobs');if(selectAll)selectAll.checked=all.length>0&&selected.length===all.length;document.querySelectorAll('.create-selected,.append-selected').forEach(button=>button.disabled=selected.length===0)}
function addRunJobSelection(){const selectAll=document.getElementById('select-all-jobs');if(!selectAll)return;selectAll.onchange=()=>{document.querySelectorAll('.job-selection').forEach(input=>input.checked=selectAll.checked);updateSelectedRunJobs()};document.querySelectorAll('.job-selection').forEach(input=>input.onchange=updateSelectedRunJobs);updateSelectedRunJobs()}
async function copySelectedJobs(queue,run,append){const jobIDs=selectedRunJobIDs();if(!jobIDs.length)return;const q=state.projects.find(item=>item.project_name===queue);const existing=(q&&q.queue.commands||[]).length;if(!append&&existing&&!confirm('This will replace '+existing+' queued jobs. Continue?'))return;const response=await fetch('/api/copy',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({project_name:queue,run_id:run,job_ids:jobIDs,selection:'job-id',append:append,overwrite:!append&&existing>0})});const text=await response.text();if(!response.ok){alert(text);return}alert(JSON.parse(text).message);window.location.href='/project/'+encodeURIComponent(queue)}
function selectFailedUnfinishedJobs(){const parts=location.pathname.split('/').filter(Boolean);const project=state.projects.find(item=>item.project_name===decodeURIComponent(parts[1]));const run=project&&project.runs.find(item=>item.run_id===decodeURIComponent(parts[3]));if(!run)return;(run.jobs||[]).forEach(job=>{const row=[...document.querySelectorAll('tr[data-job-id]')].find(item=>item.dataset.jobId===job.id);const status=jobDisplayStatus(job,run);if(row)row.querySelector('.job-selection').checked=status==='failed'||status==='pending'||status==='running'||status==='suspended'});updateSelectedRunJobs()}
function clearSelectedJobs(){document.querySelectorAll('.job-selection,#select-all-jobs').forEach(input=>input.checked=false);updateSelectedRunJobs()}
function syncRunControls(){const controls=document.querySelector('.web-copy-controls');if(!controls)return;const parts=location.pathname.split('/').filter(Boolean);const project=state.projects.find(item=>item.project_name===decodeURIComponent(parts[1]));const runID=decodeURIComponent(parts[3]);const buttons=controls.querySelectorAll('button');if(buttons.length>=2){buttons[0].outerHTML='<button class="create-selected" disabled onclick="copySelectedJobs(\''+esc(project.project_name)+'\',\''+esc(runID)+'\',false)">Create</button>';buttons[1].outerHTML='<button class="append-selected" disabled onclick="copySelectedJobs(\''+esc(project.project_name)+'\',\''+esc(runID)+'\',true)">Append</button>'}if(!controls.querySelector('.select-failed-unfinished')){const select=document.createElement('button');select.className='select-failed-unfinished';select.textContent='Select failed + unfinished';select.onclick=selectFailedUnfinishedJobs;const clear=document.createElement('button');clear.textContent='Clear selection';clear.onclick=clearSelectedJobs;controls.append(select,clear)}}
async function copyRun(queue,run,selection){`, 1)
	template = strings.Replace(template, `document.getElementById('app').prepend(controls);return}`, `document.getElementById('app').prepend(controls);syncRunControls();return}`, 1)
	template = strings.Replace(template,
		`function markJobHeaders(){const parts=location.pathname.split('/').filter(Boolean);if(parts[0]!=='project'||parts[2]!=='run')return;const table=document.querySelector('#app table.runs');if(!table)return;const keys=['name','status','executor','slurm','depends','command','exit'];table.querySelectorAll('thead th').forEach((header,index)=>{if(index<keys.length)header.dataset.sort=keys[index]})}`,
		`function markJobHeaders(){}`, 1)
	template = strings.Replace(template,
		`queue_name:queue,job_id:jobID`,
		`project_name:queue,job_id:jobID`, 1)
	template = strings.Replace(template, `let state;`, `let state;let projectRuntimeDetailsOpen=false;`, 1)
	template = strings.Replace(template, `<details><summary>Internal state</summary>`, `<details'+(projectRuntimeDetailsOpen?' open':'')+'><summary>Internal state</summary>`, 1)
	template = strings.Replace(template, `const originalRender=render;render=function(){originalRender();`, `const originalRender=render;render=function(){const runtimeDetails=document.querySelector('.project-runtime details');if(runtimeDetails)projectRuntimeDetailsOpen=runtimeDetails.open;originalRender();`, 1)
	template = strings.Replace(template, "--queue-name", "--project-name", -1)
	template = strings.NewReplacer(
		"const basedir=state&&state.base_dir?(' --basedir '+shellQuote(state.base_dir)):' '",
		"const basedir=state&&state.base_dir?(' -b '+shellQuote(state.base_dir)):' '",
		"rotari retry --run-id '+shellQuote(runID)",
		"rotari retry -r '+shellQuote(runID)",
		"rotari retry'+basedir+' --project-name '+shellQuote(queueName)+' --job-id JOB_ID",
		"rotari retry'+basedir+' -p '+shellQuote(queueName)+' -j JOB_ID",
		"rotari retry'+basedir+' --project-name '+shellQuote(queueName)",
		"rotari retry'+basedir+' -p '+shellQuote(queueName)",
	).Replace(template)
	template = strings.Replace(template, "function renderOverview(queues){", "function configText(paths){return paths&&paths.length?'\\nConfig: '+esc(paths.join(', ')):''}function renderOverview(queues){", 1)
	template = strings.Replace(template, "function configText(paths){return paths&&paths.length?'\\nConfig: '+esc(paths.join(', ')):''}function renderOverview(queues){", "function configText(paths){return paths&&paths.length?'\\nConfig: '+esc(paths.join(', ')):''}function pageConfigPaths(){const parts=location.pathname.split('/').filter(Boolean);if(parts[0]!=='project')return state.config_path?[state.config_path]:[];const project=state.projects.find(item=>item.project_name===decodeURIComponent(parts[1]));if(!project)return [];if(parts[2]==='run'){const run=project.runs.find(item=>item.run_id===decodeURIComponent(parts[3]));return run&&run.context&&run.context.config_paths||[]}return project.config_path?[project.config_path]:[]}async function showConfig(){const parts=location.pathname.split('/').filter(Boolean);const params=new URLSearchParams();if(parts[0]==='project')params.set('project_name',decodeURIComponent(parts[1]));if(parts[2]==='run')params.set('run_id',decodeURIComponent(parts[3]));const response=await fetch('/api/config?'+params);const text=await response.text();if(!response.ok){alert(text);return}const payload=JSON.parse(text);const output=ensureModalOutput();output.textContent=(payload.configs||[]).map(item=>'# '+item.path+'\\n'+item.content).join('\\n\\n');document.querySelector('#output-modal strong').textContent='Config';openOutputModal(false)}function addConfigButton(){document.querySelectorAll('.config-button').forEach(button=>button.remove());const paths=pageConfigPaths();const button=document.createElement('button');button.className='config-button';button.textContent='Config';button.disabled=!paths.length;button.title=paths.length?'View config':'No config file';if(paths.length)button.onclick=showConfig;document.querySelector('.toolbar').append(button)}function renderOverview(queues){", 1)
	template = strings.Replace(template, "state.base_dir+' / all projects'", "state.base_dir+' / all projects'+configText(state.config_path?[state.config_path]:[])", 1)
	template = strings.Replace(template, "state.base_dir+' / '+q.project_name", "state.base_dir+' / '+q.project_name+configText(q.config_path?[q.config_path]:[])", 1)
	template = strings.Replace(template, "state.base_dir+' / '+q.project_name+' / '+runID", "state.base_dir+' / '+q.project_name+' / '+runID+configText(run.context&&run.context.config_paths)", 1)
	template = strings.Replace(template, "document.querySelector('#output-modal strong').textContent='Config';openOutputModal(false)", "document.querySelector('#output-modal strong').textContent='Config';document.getElementById('output-modal').dataset.view='config';openOutputModal(false)", 1)
	template = strings.Replace(template, "async function showLog(queue,run,job){if(followTimer", "async function showLog(queue,run,job){document.getElementById('output-modal').dataset.view='log';if(followTimer", 1)
	template = strings.Replace(template, "function closeOutputModal(){document.getElementById('output-modal').style.display='none';", "function closeOutputModal(){document.getElementById('output-modal').style.display='none';delete document.getElementById('output-modal').dataset.view;", 1)
	template = strings.Replace(template, "function clarifyLogControls(){document.querySelector('#output-modal strong').textContent='Job log';", "function clarifyLogControls(){if(document.getElementById('output-modal').dataset.view!=='config')document.querySelector('#output-modal strong').textContent='Job log';", 1)
	template = strings.Replace(template, `const output=result?`, `const diagnoses=result&&result.diagnoses||[];const canDiagnose=!!(result&&result.exit_code!==0&&diagnoses.length);const diagnosisControl=canDiagnose?' <button class="diagnosis" data-diagnoses="'+esc(JSON.stringify(diagnoses))+'" onclick="showDiagnosis(this)">Diagnosis</button>':' <button class="diagnosis" disabled title="Available after a finalized failed result with saved analysis">Diagnosis</button>';const output=result?`, 1)
	template = strings.Replace(template, `Output</button>':'-';const jobName=`, `Output</button>'+diagnosisControl:diagnosisControl;const jobName=`, 1)
	template = strings.Replace(template, `async function followOutput(){`, `function showDiagnosis(trigger){if(followTimer)clearInterval(followTimer);followTimer=null;selectedLog=null;const diagnoses=JSON.parse(trigger.dataset.diagnoses||'[]');selectedOutput=diagnoses.map(item=>item.name+'\nEvidence: '+item.evidence+'\nNext: '+item.suggestion).join('\n\n');const output=ensureModalOutput();output.textContent=selectedOutput;const modal=document.getElementById('output-modal');modal.dataset.view='diagnosis';modal.querySelector('strong').textContent='Diagnosis';openOutputModal(true)}
async function followOutput(){`, 1)
	template = strings.Replace(template, `dataset.view!=='config'`, `dataset.view!=='config'&&document.getElementById('output-modal').dataset.view!=='diagnosis'&&document.getElementById('output-modal').dataset.view!=='path'&&document.getElementById('output-modal').dataset.view!=='ai'`, 1)
	template = strings.Replace(template,
		`const button=logCell.querySelector('button');if(button){const firstChild=actionCell.firstChild;actionCell.insertBefore(button,firstChild);actionCell.insertBefore(document.createTextNode(' '),firstChild)}logCell.remove()`,
		`const buttons=[...logCell.querySelectorAll('button')];if(buttons.length){const controls=document.createDocumentFragment();buttons.forEach((button,index)=>{if(index)controls.append(' ');controls.append(button)});actionCell.insertBefore(controls,actionCell.firstChild)}logCell.remove()`, 1)
	template = strings.Replace(template, `function shellQuote(v){`, `function lastLogLines(value,count){const lines=String(value||'').split('\n');return lines.slice(Math.max(0,lines.length-count)).join('\n')}
async function copyText(value){if(navigator.clipboard&&navigator.clipboard.writeText){await navigator.clipboard.writeText(value);return}const area=document.createElement('textarea');area.value=value;area.style.position='fixed';area.style.opacity='0';document.body.append(area);area.select();document.execCommand('copy');area.remove()}
function copied(button){const label=button.textContent;button.textContent='Copied';setTimeout(()=>button.textContent=label,1200)}
async function fetchSelectedLog(tail){if(!selectedLog)return selectedOutput;const suffix=tail?'&tail='+tail:'';const response=await fetch('/api/log?project_name='+encodeURIComponent(selectedLog.queue)+'&run_id='+encodeURIComponent(selectedLog.run)+'&job_id='+encodeURIComponent(selectedLog.job)+suffix);if(!response.ok)throw new Error(await response.text());const value=await response.text();return tail?lastLogLines(value,tail):value}
async function copyModalOutput(button){try{await copyText(document.getElementById('output-modal').dataset.view==='log'?await fetchSelectedLog(0):selectedOutput);copied(button)}catch(error){alert(error.message)}}
async function copyLogTail(button){try{await copyText(await fetchSelectedLog(100));copied(button)}catch(error){alert(error.message)}}
function updateModalActions(){const view=document.getElementById('output-modal').dataset.view;document.getElementById('copy-tail').hidden=view!=='log';document.getElementById('report-note').hidden=view!=='ai';['open-gemini','open-chatgpt','open-claude'].forEach(id=>document.getElementById(id).hidden=view!=='ai');document.getElementById('copy-modal').textContent=view==='log'?'Copy log':view==='ai'?'Copy':'Copy'}
async function openAI(url,button){window.open(url,'_blank','noopener');await copyText(selectedOutput);copied(button)}
async function showAIReport(project,run,job){const modal=document.getElementById('output-modal');modal.dataset.view='ai';modal.querySelector('strong').textContent=job?'Job report':'Run report';selectedLog=null;selectedOutput='Preparing...';ensureModalOutput().textContent=selectedOutput;openOutputModal(false);const params=new URLSearchParams({project_name:project.project_name,run_id:run.run_id});if(job)params.set('job_id',job.id);const response=await fetch('/api/report?'+params);selectedOutput=await response.text();if(!response.ok)selectedOutput='Failed to prepare report: '+selectedOutput;ensureModalOutput().textContent=selectedOutput;openOutputModal(false)}
function addAIButtons(){const parts=location.pathname.split('/').filter(Boolean);if(parts[0]!=='project'||parts[2]!=='run')return;const project=state.projects.find(item=>item.project_name===decodeURIComponent(parts[1]));const run=project&&project.runs.find(item=>item.run_id===decodeURIComponent(parts[3]));if(!run)return;const controls=document.querySelector('.web-copy-controls');if(controls&&!controls.querySelector('.run-ai')){const button=document.createElement('button');button.className='run-ai';button.textContent='Report';button.title='Prepare run report';button.onclick=()=>showAIReport(project,run,null);controls.append(button)}const table=document.querySelector('#app table.runs');if(!table)return;const actionIndex=[...table.querySelectorAll('thead th')].findIndex(header=>header.textContent.trim()==='Actions');if(actionIndex<0)return;table.querySelectorAll('tbody tr').forEach((row,index)=>{const actions=row.children[actionIndex];const job=run.jobs[index];if(!actions||!job||actions.querySelector('.job-ai'))return;const button=document.createElement('button');button.className='job-ai';button.textContent='Report';button.title='Prepare job report';button.onclick=()=>showAIReport(project,run,job);actions.append(' ',button)})}
function shellQuote(v){`, 1)
	template = strings.Replace(template, `function shellQuote(v){`, `function arrangeRunControls(){const parts=location.pathname.split('/').filter(Boolean);if(parts[0]!=='project'||parts[2]!=='run')return;const controls=document.querySelector('.web-copy-controls');const table=document.querySelector('#app table.runs');if(!controls||!table)return;const selectAll=document.createElement('button');selectAll.textContent='Select all';selectAll.title='Select all jobs, or clear the current selection';selectAll.onclick=()=>{const inputs=[...document.querySelectorAll('.job-selection')];const select=inputs.some(input=>!input.checked);inputs.forEach(input=>input.checked=select);updateSelectedRunJobs()};const failed=controls.querySelector('.select-failed-unfinished');const create=controls.querySelector('.create-selected');const append=controls.querySelector('.append-selected');const report=controls.querySelector('.run-ai');const deleteButton=[...controls.querySelectorAll('button')].find(button=>button.textContent.trim()==='Delete');const cancel=[...controls.querySelectorAll('button')].find(button=>button.textContent.trim()==='Cancel run');if(create)create.textContent='Create queue';if(append)append.textContent='Append to queue';controls.querySelectorAll('button').forEach(button=>{if(button.textContent.trim()==='Clear selection')button.remove()});controls.replaceChildren(...[selectAll,failed,create,append,report,deleteButton,cancel].filter(Boolean));const load=document.querySelector('.run-environment');if(load)load.after(controls);else table.before(controls);const headerSelect=document.getElementById('select-all-jobs');if(headerSelect)headerSelect.remove()}
function shellQuote(v){`, 1)
	template = strings.Replace(template, `addRunningCancelButtons();mergeActionColumns();`, `addRunningCancelButtons();addRunJobSelection();arrangeRunControls();mergeActionColumns();`, 1)
	template = strings.Replace(template, `function openOutputModal(compact){const modal=document.getElementById('output-modal');modal.style.display='flex';`, `function openOutputModal(compact){const modal=document.getElementById('output-modal');updateModalActions();modal.style.display='flex';`, 1)
	template = strings.Replace(template, "fixTimelineLegendColors()};window.addEventListener", "fixTimelineLegendColors();addConfigButton();addAIButtons();arrangeRunControls();orderJobActions()};window.addEventListener", 1)
	template = strings.Replace(template, `button.textContent='Delete';`, `button.textContent='Delete run';`, 1)
	template = strings.Replace(template, `const button=document.createElement('button');button.textContent='Delete';`, `const button=document.createElement('button');button.className='delete-run';button.textContent='Delete';`, 1)
	template = strings.Replace(template, `textContent.trim()==='Delete'`, `textContent.trim()==='Delete'||button.textContent.trim()==='Delete run'`, 1)
	template = strings.Replace(template, `button.textContent.trim()==='Delete'||button.textContent.trim()==='Delete run'`, `button.classList.contains('delete-run')`, 1)
	template = strings.Replace(template, `function shellQuote(v){`, `function orderJobActions(){document.querySelectorAll('#app table.runs tbody tr').forEach(row=>{const cell=row.firstElementChild;if(!cell)return;const buttons=[...cell.querySelectorAll('button')];const order=['view-log','job-ai','diagnosis','show-path','suspend-job','resume-job','cancel-job'];const ordered=[];order.forEach(className=>{buttons.filter(button=>button.classList.contains(className)).forEach(button=>ordered.push(button))});buttons.filter(button=>!ordered.includes(button)).forEach(button=>ordered.push(button));if(!ordered.length)return;cell.replaceChildren();ordered.forEach((button,index)=>{if(index)cell.append(' ');cell.append(button)})})}
function shellQuote(v){`, 1)
	template = strings.Replace(template, `const button=document.createElement('button');button.textContent='Path';`, `const button=document.createElement('button');button.className='show-path';button.textContent='Path';`, 1)
	template = strings.Replace(template, `const button=document.createElement('button');button.textContent='View log';`, `const button=document.createElement('button');button.className='view-log';button.textContent='View log';`, 1)
	template = strings.Replace(template, `const button=document.createElement('button');button.textContent='Delete run';`, `const button=document.createElement('button');button.className='delete-run';button.textContent='Delete run';`, 1)
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
