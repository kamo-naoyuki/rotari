package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type serverRequest = serverinternal.Request
type serverResponse = serverinternal.Response

const runDetachControl byte = 0x04

const serverProtocolVersion = 2

const maxServerLogSize = 1 << 20

type serverLogger struct {
	path string
	log  serverinternal.Logger
}

func (logger *serverLogger) writef(format string, args ...interface{}) {
	if logger == nil {
		return
	}
	logger.log.Path = logger.path
	logger.log.FileMode = stateFileMode()
	logger.log.MaxBytes = maxServerLogSize
	logger.log.Writef(format, args...)
}

type rotariServer struct {
	listener   net.Listener
	stopped    chan struct{}
	stopOnce   sync.Once
	logger     *serverLogger
	accessMu   sync.Mutex
	lastAccess time.Time
	activeRuns int
}

const serverIdleTimeout = time.Minute

func serverSocketPath(baseDir string) string {
	return serverinternal.SocketPath(baseDir)
}

func serverLockPath(baseDir string) string {
	return serverinternal.LockPath(baseDir)
}

func serverPIDPath(baseDir string) string {
	return serverinternal.PIDPath(baseDir)
}

func cmdServer(args []string) int {
	if len(args) == 0 {
		printError("usage: " + cliUsage("server"))
		return 1
	}

	switch args[0] {
	case serverinternal.OpShutdown:
		return cmdServerRequest(args[1:], "shutdown")
	case "status":
		return cmdServerStatus(args[1:])
	case "list":
		return cmdServerList(args[1:])
	default:
		printErrorf("unknown server command: %s", args[0])
		return 1
	}
}

func ensureServer(baseDir string) error {
	if response, err := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpPing}); err == nil && response.OK {
		if response.Protocol == serverProtocolVersion {
			return nil
		}
		if _, shutdownErr := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpShutdown}); shutdownErr != nil {
			return fmt.Errorf("server protocol mismatch (got %d, want %d); failed to stop old server: %w", response.Protocol, serverProtocolVersion, shutdownErr)
		}
		for range 40 {
			if _, pingErr := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpPing}); pingErr != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to detect executable path: %w", err)
	}
	child := exec.Command(exe, "__server", "--basedir", baseDir)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to detach server stdio: %w", err)
	}
	defer devNull.Close()
	child.Stdin = devNull
	child.Stdout = devNull
	child.Stderr = devNull
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	for range 40 {
		if response, err := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpPing}); err == nil && response.OK {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready (pid=%d)", child.Process.Pid)
}

func cmdServerStatus(args []string) int {
	fs := flag.NewFlagSet("server status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpPing})
	if err != nil || !response.OK {
		printError("server is not running")
		return 1
	}
	fmt.Printf("server is running pid=%d state=%s\n", response.PID, baseDir)
	return 0
}

func cmdServerList(args []string) int {
	fs := flag.NewFlagSet("server list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	masterDir, err := resolveMasterDir(*masterdir)
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	servers, err := listServers(masterDir)
	if err != nil {
		printErrorf("failed to list servers: %v", err)
		return 1
	}
	fmt.Printf("master=%s\n%s\n", masterDir, formatServerList(servers))
	return 0
}

func cmdServerRequest(args []string, op string) int {
	fs := flag.NewFlagSet("server request", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: op})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Print(colorMessage(response.Message))
	return 0
}

func cmdAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	executor := cliString(fs, "executor", "")
	workingDirectory := cliString(fs, "working-directory", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	var environment stringSliceFlag
	cliValue(fs, &environment, "env")
	jobName := cliString(fs, "job-name", "")
	var dependsOn stringSliceFlag
	cliValue(fs, &dependsOn, "depends-on")
	arrayRange := cliString(fs, "array", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	left := fs.Args()
	if len(left) < 1 {
		printError("usage: " + cliUsage("add"))
		return 1
	}
	var array *ArraySpec
	if *arrayRange != "" {
		parsed, parseErr := model.ParseArrayRange(*arrayRange)
		if parseErr != nil {
			printErrorf("invalid --array: %v", parseErr)
			return 1
		}
		array = &parsed
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := resolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		printErrorf("invalid --env: %v", err)
		return 1
	}
	message, err := enqueueCommandWithWorkingDirectory(baseDir, queueName, left, *executor, executorOptions, environment, *workingDirectory, *jobName, dependsOn, array)
	if err != nil {
		printError(err)
		return 1
	}
	fmt.Println(colorKeyValueMessage(message, green))
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobNameOption := cliString(fs, "job-name", "")
	overwriteQueue := cliBool(fs, "overwrite", false)
	runName := cliString(fs, "run-name", "")
	localConcurrency := cliInt(fs, "local-concurrency", 8)
	batchConcurrency := cliInt(fs, "batch-concurrency", 8)
	executorSettings := cliExecutorRunSettings(fs)
	retry := cliInt(fs, "retry", 0)
	failed := cliBool(fs, "failed", false)
	unfinished := cliBool(fs, "unfinished", false)
	success := cliBool(fs, "success", false)
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	partialArray := cliBool(fs, "partial-array", true)
	async := cliBool(fs, "async", false)
	executor := cliString(fs, "executor", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	left := fs.Args()
	selection := model.ResultSelection(*failed, *unfinished, *success)
	if len(jobIDs) > 0 && selection == "" {
		selection = "job-id"
	}
	if len(left) != 0 || *localConcurrency < 1 || *batchConcurrency < 1 || *retry < -1 {
		printError("usage: " + cliUsage("run"))
		return 1
	}
	if *jobNameOption != "" && len(jobIDs) > 0 {
		printError("--job-name cannot be combined with --job-id")
		return 1
	}
	if *jobNameOption != "" {
		if *runIDOption != "" {
			baseDir, projectName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
			if err != nil {
				printError(err)
				return 1
			}
			paths, err := resolvePaths(baseDir, projectName)
			if err != nil {
				printError(err)
				return 1
			}
			target, found, err := findShowJobInRun(paths, *runIDOption, *jobNameOption, true)
			if err != nil || !found {
				printErrorf("job name %q not found in run %q", *jobNameOption, *runIDOption)
				return 1
			}
			jobIDs = stringSliceFlag{target.jobID}
		} else {
			targets, err := resolveJobTargets(*basedir, *queueNameOption, *jobNameOption, true, false)
			if err != nil {
				printError(err)
				return 1
			}
			if len(targets) != 1 {
				printErrorf("job name %q is %s", *jobNameOption, map[bool]string{true: "ambiguous across latest runs", false: "not found"}[len(targets) > 1])
				return 1
			}
			*basedir, *queueNameOption, *runIDOption = targets[0].baseDir, targets[0].projectName, targets[0].runID
			jobIDs = stringSliceFlag{targets[0].jobID}
		}
	}
	attemptSelection := false
	for _, jobID := range jobIDs {
		if !strings.HasPrefix(jobID, "att_") {
			continue
		}
		payload, decodeErr := state.DecodeAttemptID(jobID)
		if decodeErr != nil {
			printError(decodeErr)
			return 1
		}
		if *runIDOption != "" && *runIDOption != payload.RunID {
			printErrorf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, *runIDOption)
			return 1
		}
		if !attemptSelection {
			*runIDOption = payload.RunID
			attemptSelection = true
		}
	}
	if *runIDOption == "" && len(jobIDs) > 0 && !attemptSelection {
		target, resolveErr := resolveLatestJobIDSelection(*basedir, *queueNameOption, jobIDs)
		if resolveErr != nil {
			printError(resolveErr)
			return 1
		}
		*basedir, *queueNameOption, *runIDOption = target.baseDir, target.projectName, target.runID
	}
	if *overwriteQueue && *runIDOption == "" {
		printError("usage: " + cliUsage("run"))
		return 1
	}
	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureProjectIdle(baseDir, queueName, "run"); err != nil {
		printError(err)
		return 1
	}

	// A source run is needed whenever a selection filter is used, so the
	// filter can be matched against that run's results. An explicit
	// --run-id always repopulates the queue first ("run --run-id X"
	// behaves like "copy --run-id X --overwrite" followed by "run"). When
	// --run-id is omitted, the queue is only repopulated from the latest
	// run if it is currently empty; a non-empty queue (e.g. already
	// restored and edited via "change") is used as-is.
	sourceRunID := *runIDOption
	forceCopy := sourceRunID != ""
	if attemptSelection {
		selection = "job-id"
		forceCopy = true
	}
	if sourceRunID == "" && selection != "" {
		paths, pathErr := resolvePaths(baseDir, queueName)
		if pathErr != nil {
			printError(pathErr)
			return 1
		}
		meta, metaErr := state.LoadMeta(paths.MetaFile)
		if metaErr != nil {
			printErrorf("failed to load metadata: %v", metaErr)
			return 1
		}
		if meta.LastRunID == "" {
			printErrorf("queue %q has no previous run", queueName)
			return 1
		}
		sourceRunID = meta.LastRunID
		queue, queueErr := state.LoadQueue(paths.QueueFile)
		if queueErr != nil {
			printErrorf("failed to load queue: %v", queueErr)
			return 1
		}
		// Explicit job selection is equivalent to copying the selected job
		// from the latest run and then running it. Other filtered runs keep
		// the current queue when it is already populated.
		forceCopy = len(queue.Commands) == 0 || selection == "job-id"
	}
	if forceCopy {
		// Only prompts when the queue actually has jobs to lose; an empty
		// queue (the common "auto-copy" case) is overwritten silently.
		overwriteConfirmed, confirmErr := confirmQueueOverwrite(baseDir, queueName, false, *overwriteQueue)
		if confirmErr != nil {
			printError(confirmErr)
			return 1
		}
		copySelection := "all"
		copyJobIDs := []string(nil)
		if selection != "" {
			copySelection = selection
			copyJobIDs = jobIDs
		}
		message, copyErr := copyRunToQueue(baseDir, queueName, sourceRunID, copySelection, copyJobIDs, false, overwriteConfirmed)
		if copyErr != nil {
			printError(copyErr)
			return 1
		}
		fmt.Println(colorKeyValueMessage(message, green))
	}
	if selection != "" {
		selection = ""
		jobIDs = nil
	}

	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		printErrorf("failed to determine working directory: %v", err)
		return 1
	}
	request := serverRequest{
		Op: serverinternal.OpRun, QueueName: queueName, LocalConcurrency: *localConcurrency, BatchMaxActive: *batchConcurrency, ExecutorSettings: executorSettings, Retry: *retry, Async: *async,
		RunName: *runName, Executor: *executor, ExecutorOptions: executorOptions, CWD: cwd,
		Selection: selection, JobIDs: jobIDs, SourceRunID: sourceRunID, PartialArray: *partialArray,
	}
	var response serverResponse
	if *async {
		response, err = sendServerRequest(baseDir, request)
	} else {
		response, err = sendRunRequest(baseDir, request)
	}
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Print(colorMessage(response.Message))
	return response.ExitCode
}

func cmdRetry(args []string) int {
	return cmdRun(append([]string{"--failed", "--unfinished"}, args...))
}

func cmdServerProcess(args []string) int {
	fs := flag.NewFlagSet("__server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	return runServer(baseDir)
}

func runServer(baseDir string) int {
	if err := os.MkdirAll(baseDir, stateDirMode()); err != nil {
		printErrorf("failed to create state directory: %v", err)
		return 1
	}
	lease, err := os.OpenFile(serverLockPath(baseDir), os.O_CREATE|os.O_RDWR, stateFileMode())
	if err != nil {
		printErrorf("failed to open server lock: %v", err)
		return 1
	}
	defer lease.Close()
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		printError("server is already running")
		return 1
	}

	socketPath := serverSocketPath(baseDir)
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		printErrorf("failed to listen on server socket: %v", err)
		return 1
	}
	// net.Listen applies the process umask; enforce owner-only access explicitly
	// regardless of --shared-state, since this is the control-plane socket.
	if err := os.Chmod(socketPath, 0o600); err != nil {
		printErrorf("failed to set server socket permissions: %v", err)
		return 1
	}
	defer os.Remove(socketPath)
	defer os.Remove(serverPIDPath(baseDir))
	if err := os.WriteFile(serverPIDPath(baseDir), []byte(strconv.Itoa(os.Getpid())+"\n"), stateFileMode()); err != nil {
		printErrorf("failed to write server pid: %v", err)
		return 1
	}
	masterDir, err := resolveMasterDir("")
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	record := serverRecord{
		BaseDir: baseDir, Socket: socketPath, PID: os.Getpid(),
		StartedAt: nowRFC3339(), LastSeen: nowRFC3339(),
	}
	if err := registerServer(masterDir, record); err != nil {
		printErrorf("failed to register server: %v", err)
		return 1
	}
	defer unregisterServer(masterDir, baseDir)

	server := &rotariServer{
		listener: listener, stopped: make(chan struct{}), lastAccess: time.Now(),
		logger: &serverLogger{path: filepath.Join(baseDir, "server.log")},
	}
	server.logger.writef("started pid=%d", os.Getpid())
	defer server.logger.writef("stopped")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		server.stop()
	}()
	go server.idleChecker(masterDir, baseDir)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if server.isStopped() {
				return 0
			}
			continue
		}
		if err := verifyPeerCredential(conn); err != nil {
			_ = conn.Close()
			continue
		}
		go server.handle(baseDir, conn)
	}
}

func (server *rotariServer) isStopped() bool {
	select {
	case <-server.stopped:
		return true
	default:
		return false
	}
}

func (server *rotariServer) stop() {
	server.stopOnce.Do(func() {
		server.logger.writef("stopping")
		close(server.stopped)
		_ = server.listener.Close()
	})
}

func (server *rotariServer) touch() {
	server.accessMu.Lock()
	server.lastAccess = time.Now()
	server.accessMu.Unlock()
}

func (server *rotariServer) idleChecker(masterDir, baseDir string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		_ = touchServerRecord(masterDir, baseDir)
		server.accessMu.Lock()
		idle := time.Since(server.lastAccess) >= serverIdleTimeout && server.activeRuns == 0
		server.accessMu.Unlock()
		if idle {
			server.stop()
			return
		}
		if server.isStopped() {
			return
		}
	}
}

func (server *rotariServer) handle(baseDir string, conn net.Conn) {
	defer conn.Close()
	server.touch()
	var request serverRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		server.logger.writef("request decode failed: %v", err)
		_ = json.NewEncoder(conn).Encode(serverResponse{Message: err.Error()})
		return
	}
	server.logger.writef("request op=%s", request.Op)
	response := serverResponse{}
	encoder := json.NewEncoder(conn)
	if !serverinternal.IsKnownOperation(request.Op) {
		response.Message = "unknown server operation: " + request.Op
		_ = encoder.Encode(response)
		return
	}
	switch request.Op {
	case serverinternal.OpPing:
		response = serverResponse{OK: true, PID: os.Getpid(), Protocol: serverProtocolVersion}
	case serverinternal.OpSubmit:
		message, err := enqueueCommandWithWorkingDirectory(baseDir, request.QueueName, request.Command, request.Executor, request.ExecutorOptions, request.Environment, request.WorkingDirectory, request.JobName, request.DependsOn, request.Array)
		response = serverResponse{OK: err == nil, Message: message}
		if err != nil {
			response.Message = err.Error()
		}
	case serverinternal.OpCancel:
		message, err := cancelQueueJobs(baseDir, request.QueueName, request.JobIDs, request.Wait)
		response = serverResponse{OK: err == nil, Message: message}
		if err != nil {
			response.Message = err.Error()
		}
	case serverinternal.OpSuspend, serverinternal.OpResume:
		message, err := controlQueueJobs(baseDir, request.QueueName, request.JobIDs, request.Op)
		response = serverResponse{OK: err == nil, Message: message}
		if err != nil {
			response.Message = err.Error()
		}
	case serverinternal.OpRun:
		var message string
		var exitCode int
		var err error
		var onDone func()
		if request.Async {
			server.beginRun()
			onDone = server.endRun
			message, err = startServerRun(baseDir, request.QueueName, request.RunName, request.LocalConcurrency, request.BatchMaxActive, request.Retry, request.Executor, request.ExecutorOptions, request.Selection, request.JobIDs, request.SourceRunID, request.PartialArray, onDone, request.CWD, request.ExecutorSettings)
			if err != nil {
				onDone()
			}
		} else {
			server.beginRun()
			runDetached := false
			defer func() {
				if !runDetached {
					server.endRun()
				}
			}()
			message, exitCode, err, runDetached = runServerSyncWithDisconnectAndDoneWithSettings(conn, baseDir, request.QueueName, request.RunName, request.LocalConcurrency, request.BatchMaxActive, request.Retry, request.Executor, request.ExecutorOptions, request.Selection, request.JobIDs, request.SourceRunID, request.PartialArray, func(progress serverResponse) {
				_ = encoder.Encode(progress)
			}, server.endRun, request.ExecutorSettings, request.CWD)
		}
		response = serverResponse{OK: err == nil, Message: message, ExitCode: exitCode}
		if err != nil {
			response.Message = err.Error()
		}
	case serverinternal.OpShutdown:
		response = serverResponse{OK: true, Message: "server stopped"}
		server.stop()
	default:
		response.Message = "unsupported server operation: " + request.Op
	}
	_ = encoder.Encode(response)
}

func runServerSyncWithDisconnect(conn net.Conn, baseDir, queueName, runName string, localConcurrency, batchMaxActive, retry int, executor string, executorOptions []string, selection string, jobIDs []string, sourceRunID string, partialArray bool, progress func(serverResponse), cwdOverride ...string) (string, int, error) {
	message, exitCode, err, _ := runServerSyncWithDisconnectAndDoneWithSettings(conn, baseDir, queueName, runName, localConcurrency, batchMaxActive, retry, executor, executorOptions, selection, jobIDs, sourceRunID, partialArray, progress, nil, nil, cwdOverride...)
	return message, exitCode, err
}

func runServerSyncWithDisconnectAndDone(conn net.Conn, baseDir, queueName, runName string, localConcurrency, batchMaxActive, retry int, executor string, executorOptions []string, selection string, jobIDs []string, sourceRunID string, partialArray bool, progress func(serverResponse), onDone func(), cwdOverride ...string) (string, int, error, bool) {
	return runServerSyncWithDisconnectAndDoneWithSettings(conn, baseDir, queueName, runName, localConcurrency, batchMaxActive, retry, executor, executorOptions, selection, jobIDs, sourceRunID, partialArray, progress, onDone, nil, cwdOverride...)
}

func runServerSyncWithDisconnectAndDoneWithSettings(conn net.Conn, baseDir, queueName, runName string, localConcurrency, batchMaxActive, retry int, executor string, executorOptions []string, selection string, jobIDs []string, sourceRunID string, partialArray bool, progress func(serverResponse), onDone func(), executorSettings executorRunSettingsMap, cwdOverride ...string) (string, int, error, bool) {
	cwd := ""
	if len(cwdOverride) > 0 {
		cwd = cwdOverride[0]
	}
	type result struct {
		message  string
		exitCode int
		err      error
	}
	done := make(chan result, 1)
	go func() {
		message, exitCode, err := runServerSync(baseDir, queueName, runName, localConcurrency, batchMaxActive, retry, executor, executorOptions, selection, jobIDs, sourceRunID, partialArray, progress, cwd, executorSettings)
		done <- result{message: message, exitCode: exitCode, err: err}
	}()
	disconnected := make(chan bool, 1)
	go func() {
		var buffer [1]byte
		n, err := conn.Read(buffer[:])
		if n > 0 && buffer[0] == runDetachControl {
			disconnected <- true
			return
		}
		if err != nil && (err == io.EOF || !errors.Is(err, os.ErrDeadlineExceeded)) {
			disconnected <- false
		}
	}()
	select {
	case result := <-done:
		return result.message, result.exitCode, result.err, false
	case detached := <-disconnected:
		if detached {
			if onDone != nil {
				go func() {
					<-done
					onDone()
				}()
			}
			return "Run detached; it continues in the background.", 0, nil, true
		}
		_, _ = cancelQueue(baseDir, queueName, false)
		result := <-done
		return result.message, result.exitCode, result.err, false
	}
}

func cmdCancel(args []string) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	wait := cliBool(fs, "wait", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("cancel"))
		return 1
	}
	if len(jobIDs) > 0 && *wait {
		printError("--wait may not be used with --job-id")
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if len(jobIDs) > 0 && strings.HasPrefix(jobIDs[0], "att_") {
		resolvedBaseDir, resolvedProject, _, _, resolveErr := resolveAttemptTarget(jobIDs[0], *basedir, *queueNameOption, "")
		if resolveErr != nil {
			printError(resolveErr)
			return 1
		}
		baseDir, *queueNameOption = resolvedBaseDir, resolvedProject
		err = nil
	}
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := resolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpCancel, QueueName: queueName, JobIDs: jobIDs, Wait: *wait})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

func cmdJobSignal(args []string, operation string) int {
	fs := flag.NewFlagSet(operation, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage(operation))
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if len(jobIDs) > 0 && strings.HasPrefix(jobIDs[0], "att_") {
		resolvedBaseDir, resolvedProject, _, _, resolveErr := resolveAttemptTarget(jobIDs[0], *basedir, *queueNameOption, "")
		if resolveErr != nil {
			printError(resolveErr)
			return 1
		}
		baseDir, *queueNameOption = resolvedBaseDir, resolvedProject
		err = nil
	}
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := resolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: operation, QueueName: queueName, JobIDs: jobIDs})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

func (server *rotariServer) beginRun() {
	server.accessMu.Lock()
	server.activeRuns++
	server.lastAccess = time.Now()
	server.accessMu.Unlock()
}

func (server *rotariServer) endRun() {
	server.accessMu.Lock()
	server.activeRuns--
	server.lastAccess = time.Now()
	shouldStop := server.activeRuns == 0
	server.accessMu.Unlock()
	if shouldStop {
		server.stop()
	}
}

func (server *rotariServer) isBusy() bool {
	server.accessMu.Lock()
	defer server.accessMu.Unlock()
	return server.activeRuns > 0
}

func sendServerRequest(baseDir string, request serverRequest) (serverResponse, error) {
	return serverinternal.SendRequest(baseDir, request)
}

func sendRunRequest(baseDir string, request serverRequest) (serverResponse, error) {
	conn, err := net.DialTimeout("unix", serverSocketPath(baseDir), time.Second)
	if err != nil {
		return serverResponse{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return serverResponse{}, err
	}
	decoder := json.NewDecoder(conn)
	responses := make(chan serverResponse, 1)
	decodeErrors := make(chan error, 1)
	go func() {
		for {
			var response serverResponse
			if err := decoder.Decode(&response); err != nil {
				decodeErrors <- err
				return
			}
			responses <- response
			if !response.Progress {
				return
			}
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	input := make(chan bool, 1)
	if isTerminal(os.Stdin) {
		go func() {
			var buffer [1]byte
			n, err := os.Stdin.Read(buffer[:])
			if (n == 0 && err == nil) || (n > 0 && buffer[0] == runDetachControl) {
				input <- true
			}
		}()
	}
	lastCompleted, lastSucceeded, lastFailed := -1, -1, -1
	for {
		var response serverResponse
		select {
		case <-input:
			if _, err := conn.Write([]byte{runDetachControl}); err != nil {
				return serverResponse{}, err
			}
			_ = conn.Close()
			fmt.Println(cyan("Run detached; it continues in the background."))
			return serverResponse{OK: true}, nil
		case <-signals:
			fmt.Println(yellow("Cancellation requested; stopping running jobs..."))
			_ = conn.Close()
			return serverResponse{OK: true, ExitCode: 130}, nil
		case err := <-decodeErrors:
			return serverResponse{}, err
		case response = <-responses:
		}
		if response.Progress {
			if response.Message != "" {
				if strings.HasPrefix(response.Message, "Job failed") {
					title, details, _ := strings.Cut(response.Message, "\n")
					fmt.Printf("%s\n%s\n", red(title), colorLabeledDetails(details, true))
				} else if strings.HasPrefix(response.Message, "Retrying job") {
					fmt.Printf("%s\n", colorKeyValueMessage(response.Message, yellow))
				} else if strings.HasPrefix(response.Message, "=== Run started ===") {
					fmt.Printf("%s\n", colorMessage(response.Message))
					fmt.Println(cyan("Press Ctrl-D to detach; Ctrl-C to cancel."))
				} else if strings.HasPrefix(response.Message, "Job running:") {
					title, details, _ := strings.Cut(response.Message, "\n")
					fmt.Printf("%s\n%s\n", cyan(title), colorLabeledDetails(details, false))
				} else {
					fmt.Printf("%s\n", yellow(response.Message))
				}
			} else {
				if response.Completed == lastCompleted && response.Succeeded == lastSucceeded && response.Failed == lastFailed {
					continue
				}
				fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("progress: %d/%d completed=%d failed=%d", response.Completed, response.Total, response.Succeeded, response.Failed), cyan))
				lastCompleted, lastSucceeded, lastFailed = response.Completed, response.Succeeded, response.Failed
			}
			continue
		}
		return response, nil
	}
}

func resolveQueueExecutor(baseDir, queueName, requested string) (string, error) {
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", err
	}
	if requested == "" {
		requested = queue.DefaultExecutor
		if requested == "" {
			requested = "local"
		}
	}
	if !isKnownExecutor(requested) {
		return "", fmt.Errorf("unsupported executor: %s", requested)
	}
	resolved := requested
	for _, queued := range queue.Commands {
		executor := queued.Executor
		if executor == "" {
			executor = requested
		}
		if !isKnownExecutor(executor) {
			return "", fmt.Errorf("unsupported executor: %s", executor)
		}
	}
	return resolved, nil
}

func cancelQueue(baseDir, queueName string, wait bool) (string, error) {
	return cancelQueueJobs(baseDir, queueName, nil, wait)
}

func controlQueueJobs(baseDir, queueName string, jobIDs []string, operation string) (string, error) {
	if operation != "suspend" && operation != "resume" {
		return "", fmt.Errorf("unsupported job operation: %s", operation)
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", queueName)
		}
		return "", fmt.Errorf("invalid running lock: %w", err)
	}
	if !validWebID(lock.RunID) {
		return "", fmt.Errorf("invalid run ID %q", lock.RunID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, lock.RunID)
	if err != nil {
		return "", err
	}
	jobIDs, err = normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
	if err != nil {
		return "", err
	}
	allJobs := len(jobIDs) == 0
	targets := append([]string(nil), jobIDs...)
	if allJobs {
		entries, err := os.ReadDir(runDir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				targets = append(targets, entry.Name())
			}
		}
	}
	controlled := 0
	for _, jobID := range targets {
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			return "", err
		}
		if jobFinished(jobDir) {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		jobExecutor, err := jobOwnerExecutor(jobDir)
		if err != nil {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		if host, mismatch := localExecutorHostMismatch(jobExecutor, runDir); mismatch {
			return "", fmt.Errorf("job %q runs on host %q; run %s from that host", jobID, host, operation)
		}
		suspender, ok := jobExecutor.(Suspender)
		if !ok {
			return "", fmt.Errorf("executor %q does not support %s", jobExecutor.Name(), operation)
		}
		if operation == "resume" {
			err = suspender.Resume(jobDir)
		} else {
			err = suspender.Suspend(jobDir)
		}
		if err != nil {
			return "", fmt.Errorf("%s job %s: %w", operation, jobID, err)
		}
		if operation == "resume" {
			executor.WriteSchedulerStatus(jsonStore(), jobDir, "running", time.Now())
		} else {
			executor.WriteSchedulerStatus(jsonStore(), jobDir, "suspended", time.Now())
		}
		controlled++
	}
	if controlled == 0 {
		return "", fmt.Errorf("no running jobs found in queue %q", queueName)
	}
	return fmt.Sprintf("%s requested\n  Project: %s\n  Run: %s\n  Jobs: %d", strings.Title(operation), queueName, lock.RunID, controlled), nil
}

func jobFinished(jobDir string) bool {
	if path, err := state.ValidatedStateFile(jobDir, stateFileFinishedAt); err == nil {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	// NOSONAR: jobDir is produced by validated run/job path helpers before reading the status file.
	path, err := state.ValidatedStateFile(jobDir, stateFileStatusJSON)
	if err != nil {
		return false
	}
	// NOSONAR: jobDir is produced by validated job and run path helpers.
	var status slurmStatus
	if err := jsonStore().ReadJSON(path, &status); err != nil {
		return false
	}
	return status.Phase != "" && status.Phase != "running"
}

func normalizeRunningAttemptIDs(runDir, runID string, jobIDs []string) ([]string, error) {
	normalized := make([]string, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		if !strings.HasPrefix(jobID, "att_") {
			normalized = append(normalized, jobID)
			continue
		}
		payload, err := state.DecodeAttemptID(jobID)
		if err != nil {
			return nil, err
		}
		if payload.RunID != runID {
			return nil, fmt.Errorf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, runID)
		}
		attemptDir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, jobID)
		if err != nil {
			return nil, err
		}
		latestDir, err := state.LatestAttemptJobDir(runDir, payload.JobID)
		if err != nil || filepath.Clean(attemptDir) != filepath.Clean(latestDir) {
			latestAttemptID, _ := state.LatestAttemptID(runDir, payload.JobID)
			if latestAttemptID != "" {
				return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q\nLatest attempt: %q (%s)", jobID, payload.JobID, latestAttemptID, attemptState(latestDir))
			}
			return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q", jobID, payload.JobID)
		}
		if jobFinished(attemptDir) {
			return nil, fmt.Errorf("attempt %q is finished", jobID)
		}
		if _, err := jobOwnerExecutor(attemptDir); err != nil {
			return nil, fmt.Errorf("attempt %q is pending", jobID)
		}
		normalized = append(normalized, payload.JobID)
	}
	return normalized, nil
}

func attemptState(jobDir string) string {
	if state := strings.ToLower(strings.TrimSpace(executor.LoadSchedulerStatus(jsonStore(), jobDir))); state != "" {
		return state
	}
	if path, err := state.ValidatedStateFile(jobDir, stateFileStatusJSON); err == nil { // NOSONAR: jobDir is validated run/job path
		var status slurmStatus
		if err := jsonStore().ReadJSON(path, &status); err == nil && status.Phase != "" {
			return strings.ToLower(status.Phase)
		}
	}
	if jobFinished(jobDir) {
		return "finished"
	}
	if _, err := jobOwnerExecutor(jobDir); err == nil {
		return "running"
	}
	return "pending"
}

func cancelQueueJobs(baseDir, queueName string, jobIDs []string, wait bool) (string, error) {
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", queueName)
		}
		return "", err
	}
	if !validWebID(lock.RunID) {
		return "", fmt.Errorf("invalid run ID %q", lock.RunID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, lock.RunID)
	if err != nil {
		return "", err
	}
	jobIDs, err = normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
	if err != nil {
		return "", err
	}
	if len(jobIDs) > 0 {
		return cancelJobs(runDir, queueName, lock.RunID, jobIDs)
	}
	if err := markQueueCancelling(paths); err != nil {
		return "", err
	}
	if lock.PID == os.Getpid() {
		// Cancel every still-running job directly through its owning
		// executor (job.json for schedulers, pid file for local), instead of
		// relying on an aggregate metadata file that schedulers only write
		// once the whole run finishes -- otherwise a cancel issued mid-run
		// never reaches an already-submitted Slurm/PBS/LSF job.
		commandSnapshot, err := loadCommandSnapshot(runDir)
		if err != nil {
			return "", err
		}
		targets := make([]string, 0, len(commandSnapshot.Commands))
		for _, job := range model.QueueToJobs(commandSnapshot.Commands) {
			jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
			if err != nil {
				return "", err
			}
			if jobFinished(jobDir) {
				continue
			}
			targets = append(targets, job.ID)
		}
		message, err := cancelJobs(runDir, queueName, lock.RunID, targets)
		if err != nil {
			return "", err
		}
		return finishCancelMessage(message, paths, queueName, lock.RunID, wait)
	}
	if host, mismatch := runningWorkerHostMismatch(lock); mismatch {
		return "", fmt.Errorf("run %q is owned by host %q; run cancel from that host", lock.RunID, host)
	}
	if err := syscall.Kill(-lock.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return "", fmt.Errorf("cancel local worker: %w", err)
	}
	return finishCancelMessage(fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Worker PID: %d", queueName, lock.RunID, lock.PID), paths, queueName, lock.RunID, wait)
}

func cancelJobs(runDir, queueName, runID string, jobIDs []string) (string, error) {
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	commandSnapshot, err := loadCommandSnapshot(runDir)
	if err != nil {
		return "", err
	}
	knownJobs := make(map[string]JobSpec)
	for _, job := range model.QueueToJobs(commandSnapshot.Commands) {
		knownJobs[job.ID] = job
	}
	cancelled := 0
	for jobID := range requested {
		if !state.IsValidPathElement(jobID) {
			return "", fmt.Errorf("invalid job ID %q", jobID)
		}
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			return "", err
		}
		if executor, err := jobOwnerExecutor(jobDir); err == nil {
			if host, mismatch := localExecutorHostMismatch(executor, runDir); mismatch {
				return "", fmt.Errorf("job %q runs on host %q; run cancel from that host", jobID, host)
			}
			canceller, ok := executor.(Canceller)
			if !ok {
				return "", fmt.Errorf("executor %q does not support cancel", executor.Name())
			}
			if err := canceller.Cancel(jobDir); err != nil {
				return "", fmt.Errorf("cancel job %s: %w", jobID, err)
			}
			cancelled++
			continue
		}
		// Job has no executor yet (Submit was never called): record the
		// cancellation so the scheduler skips it once it would be submitted.
		if _, ok := knownJobs[jobID]; !ok {
			return "", fmt.Errorf("job %q is not found", jobID)
		}
		if err := os.MkdirAll(jobDir, stateDirMode()); err != nil { // NOSONAR: jobDir comes from validatedJobDir.
			return "", fmt.Errorf("prepare cancellation for job %s: %w", jobID, err)
		}
		if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(nowRFC3339()+"\n"), stateFileMode()); err != nil { // NOSONAR: jobDir comes from validatedJobDir.
			return "", fmt.Errorf("record cancellation for job %s: %w", jobID, err)
		}
		cancelled++
	}
	return fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Jobs: %d", queueName, runID, cancelled), nil
}

func loadCommandSnapshot(runDir string) (Queue, error) {
	var snapshot Queue
	if data, err := os.ReadFile(filepath.Join(runDir, "commands.json")); err == nil { // NOSONAR: runDir is produced by validatedRunDir.
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return Queue{}, fmt.Errorf("invalid command snapshot: %w", err)
		}
	}
	return snapshot, nil
}

func markQueueCancelling(paths pathSet) error {
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "cancelling"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to mark queue as cancelling: %w", err)
	}
	return nil
}

func finishCancelMessage(message string, paths pathSet, queueName, runID string, wait bool) (string, error) {
	if wait {
		deadline := time.Now().Add(5 * time.Minute)
		for {
			running, err := isRunning(paths.LockFile)
			if err != nil {
				return "", err
			}
			if !running {
				message += "\n\nCancellation complete"
				break
			}
			if time.Now().After(deadline) {
				return "", errors.New("timed out waiting for cancellation")
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	message += fmt.Sprintf("\n\nInspect status:\n  rotari show --run-id %s", runID)
	return message, nil
}

func enqueueCommand(baseDir, queueName string, command []string, executor string, executorOptions, environment []string, jobName string, dependsOn []string, arrays ...*ArraySpec) (string, error) {
	return enqueueCommandWithWorkingDirectory(baseDir, queueName, command, executor, executorOptions, environment, "", jobName, dependsOn, arrays...)
}

func enqueueCommandWithWorkingDirectory(baseDir, queueName string, command []string, executor string, executorOptions, environment []string, workingDirectory, jobName string, dependsOn []string, arrays ...*ArraySpec) (string, error) {
	if queueName == "" || len(command) == 0 {
		return "", errors.New("project name and command are required")
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		return "", fmt.Errorf("invalid environment: %w", err)
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return "", err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", err
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "add"); err != nil {
		return "", err
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", err
	}
	if executor != "" && !isKnownExecutor(executor) {
		return "", fmt.Errorf("unsupported executor: %s", executor)
	}
	var array *ArraySpec
	if len(arrays) > 0 {
		array = arrays[0]
	}
	job := QueuedCommand{
		ID: makeJobID(), Command: command, WorkingDirectory: workingDirectory, Executor: executor, ExecutorOptions: executorOptions, Environment: environment, Name: jobName, DependsOn: dependsOn, Array: array,
	}
	if err := validateQueueJobs(Queue{Commands: append(append([]QueuedCommand(nil), queue.Commands...), job)}); err != nil {
		return "", err
	}
	// Dependencies may refer to jobs added later, so only duplicate names are checked here.
	if job.Name != "" {
		for _, existing := range queue.Commands {
			if existing.Name == job.Name {
				return "", fmt.Errorf("invalid dependencies: duplicate job name: %s", job.Name)
			}
		}
	}
	queue.Commands = append(queue.Commands, job)
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		return "", err
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return "", err
	}
	message := fmt.Sprintf("submitted project=%s job_id=%s", queueName, job.ID)
	if job.Name != "" {
		message += fmt.Sprintf(" job_name=%s", job.Name)
	}
	return fmt.Sprintf("%s command=%s", message, joinCommand(command)), nil
}

func startServerRun(baseDir, queueName, runName string, localConcurrency, batchMaxActive, retry int, executor string, executorOptions []string, selection string, jobIDs []string, sourceRunID string, partialArray bool, onDone func(), cwd string, executorSettings executorRunSettingsMap) (string, error) {
	_, err := resolveQueueExecutor(baseDir, queueName, executor)
	if err != nil {
		return "", err
	}
	if localConcurrency < 1 {
		return "", errors.New("local concurrency must be >= 1")
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return "", err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", err
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "run"); err != nil {
		return "", err
	}
	queue, err := loadRunQueue(paths, executor, executorOptions, executorSettings)
	if err != nil {
		return "", err
	}
	if len(queue.Commands) == 0 {
		return "", fmt.Errorf("queue %q has no queued commands", queueName)
	}
	runID := makeRunID()
	if err := launchAsyncRun(paths, runOptions{
		QueueName: queueName, RunID: runID, RunName: runName,
		LocalConcurrency: localConcurrency, BatchMaxActive: batchMaxActive, Retry: retry,
		Executor: executor, ExecutorOptions: executorOptions, Selection: selection,
		JobIDs: jobIDs, SourceRunID: sourceRunID, PartialArray: partialArray,
		CWD: cwd, OnDone: onDone, ExecutorSettings: executorSettings,
	}); err != 0 {
		return "", errors.New("queue is already running")
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run: %s\n  Directory: %s\n\nCheck status:\n  rotari show --run-id %s\n\nCancel run:\n  rotari cancel --basedir %s --project-name %s",
		queueName, formatRunLabel(runID, runName), runDir, runID, paths.BaseDir, queueName), nil
}

func runServerSync(baseDir, queueName, runName string, localConcurrency, batchMaxActive, retry int, executor string, executorOptions []string, selection string, jobIDs []string, sourceRunID string, partialArray bool, progress func(serverResponse), cwd string, executorSettings executorRunSettingsMap) (string, int, error) {
	resolvedExecutor, err := resolveQueueExecutor(baseDir, queueName, executor)
	if err != nil {
		return "", 1, err
	}
	if localConcurrency < 1 {
		return "", 1, errors.New("local concurrency must be >= 1")
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", 1, err
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return "", 1, err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", 1, err
	}
	if err := ensureProjectIdleForPaths(paths, "run"); err != nil {
		release()
		return "", 1, err
	}
	queue, err := loadRunQueue(paths, executor, executorOptions, executorSettings)
	if err != nil {
		release()
		return "", 1, err
	}
	if len(queue.Commands) == 0 {
		release()
		return "", 1, fmt.Errorf("queue %q has no queued commands", queueName)
	}
	runID := makeRunID()
	if err := writeRunContext(paths, runID, cwd); err != nil {
		release()
		return "", 1, err
	}
	if err := acquireLock(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: runID, RunName: runName, StartedAt: nowRFC3339()}); err != nil {
		release()
		return "", 1, fmt.Errorf("project %q is already running", queueName)
	}
	if err := registerRun(paths, runID); err != nil {
		_ = os.Remove(paths.LockFile)
		if runDir, pathErr := state.SafeJoin(paths.RunsDir, runID); pathErr == nil {
			_ = os.RemoveAll(runDir)
		}
		release()
		return "", 1, fmt.Errorf("failed to register run: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	meta.Phase = "running"
	meta.LastRunID = runID
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	plan, err := planRerunSelection(paths, queue, selection, jobIDs, sourceRunID, partialArray)
	if err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	submitted := len(plan.Execute)
	excluded := len(queue.Commands) - submitted
	release()
	if progress != nil {
		progress(serverResponse{Progress: true, Message: fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run ID: %s\n  Submitted: %d\n  Excluded: %d\n  Total: %d", queueName, runID, submitted, excluded, len(queue.Commands))})
	}

	stopLoadSampling := startRunLoadSampling(paths, runID)
	exitCode := executeMixedRun(paths, runID, runName, localConcurrency, batchMaxActive, retry, resolvedExecutor, executorOptions, selection, jobIDs, sourceRunID, partialArray, func(result JobResult, completed, total, succeeded, failed int) {
		if progress != nil {
			message := ""
			if result.ExitCode != 0 && result.Error == "final-failure" {
				failureTitle := "Job failed:"
				if retry > 0 {
					failureTitle = "Job failed after retry:"
				}
				attemptID := result.AttemptID
				if attemptID == "" {
					attemptID = result.ID
				}
				message = fmt.Sprintf("%s\n  ID: %s\n  Attempt ID: %s\n  Command: %s\n  Show output:\n    rotari show --run-id %s --job-id %s",
					failureTitle,
					result.ID, result.AttemptID, strings.Join(result.Command, " "), runID, attemptID)
			} else if strings.HasPrefix(result.Error, "retry:") {
				message = fmt.Sprintf("Retrying job: attempt=%s job=%s command=%v", strings.TrimPrefix(result.Error, "retry:"), result.ID, result.Command)
			}
			progress(serverResponse{OK: true, Progress: true, Message: message, JobID: result.ID, Completed: completed, Total: total, Succeeded: succeeded, Failed: failed})
		}
	}, func(job JobSpec) {
		if progress == nil {
			return
		}
		name := job.Name
		if name == "" {
			name = "-"
		}
		message := fmt.Sprintf("Job running:\n  ID: %s\n  Attempt ID: %s\n  Name: %s\n  Show:\n    rotari show --run-id %s --job-id %s",
			job.ID, job.AttemptID, name, runID, job.AttemptID)
		progress(serverResponse{OK: true, Progress: true, Message: message, JobID: job.ID})
	}, executorSettings)
	stopLoadSampling()
	if err := finishRunContext(paths, runID); err != nil {
		_ = os.Remove(paths.LockFile)
		return "", 1, err
	}
	if err := finishRun(paths, runID, exitCode); err != nil {
		_ = os.Remove(paths.LockFile)
		return "", 1, err
	}
	if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 1, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", 1, err
	}
	data, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err == nil {
		var summary RunSummary
		if json.Unmarshal(data, &summary) == nil {
			return formatRunCompletion(paths, runID, summary), exitCode, nil
		}
	}
	return fmt.Sprintf("=== Run finished ===\n  Project: %s\n  Run: %s\n  Exit code: %d", queueName, runID, exitCode), exitCode, nil
}

func joinCommand(command []string) string {
	return fmt.Sprintf("%v", command)
}
