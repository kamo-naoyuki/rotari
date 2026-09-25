package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const maxServerLogSize = 1 << 20

const serverIdleTimeout = time.Minute

// cmdServer dispatches server lifecycle subcommands such as status, list, and
// shutdown.
func cmdServer(args []string) int {
	if len(args) == 0 {
		printError("usage: " + cliUsage("server"))
		return 1
	}

	if isHelpArgument(args[0]) {
		printSubcommandHelp("server")
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
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to detect executable path: %w", err)
	}
	return serverinternal.Ensure(baseDir, []string{exe, "__server", "--basedir", baseDir})
}

// cmdServerStatus reports whether the selected base directory has a reachable
// compatible background server.
func cmdServerStatus(args []string) int {
	fs := flag.NewFlagSet("server status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	response, err := serverinternal.SendRequest(baseDir, serverinternal.Request{Op: serverinternal.OpPing})
	if err != nil || !response.OK {
		printError("server is not running")
		return 1
	}
	fmt.Printf("server is running pid=%d state=%s\n", response.PID, baseDir)
	return 0
}

// cmdServerList lists registered servers from the master registry.
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

// cmdServerRequest sends a simple lifecycle operation to the selected server.
func cmdServerRequest(args []string, op string) int {
	fs := flag.NewFlagSet("server request", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	response, err := serverinternal.SendRequest(baseDir, serverinternal.Request{Op: op})
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

// cmdServerProcess runs the hidden background server process for one base
// directory.
func cmdServerProcess(args []string) int {
	fs := flag.NewFlagSet("__server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	return runServer(baseDir)
}

func runServer(baseDir string) int {
	if err := os.MkdirAll(baseDir, state.DirectoryMode()); err != nil {
		printErrorf("failed to create state directory: %v", err)
		return 1
	}
	listener, release, err := serverinternal.Listen(baseDir, state.FileMode())
	if err != nil {
		printError(err)
		return 1
	}
	defer release()
	masterDir, err := resolveMasterDir("")
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	record := serverRecord{
		BaseDir: baseDir, Socket: serverinternal.SocketPath(baseDir), PID: os.Getpid(),
		StartedAt: nowRFC3339(), LastSeen: nowRFC3339(),
	}
	if err := registerServer(masterDir, record); err != nil {
		printErrorf("failed to register server: %v", err)
		return 1
	}
	defer unregisterServer(masterDir, baseDir)

	logger := newServerLogger(baseDir)
	server := serverinternal.New(listener, serverOperations{baseDir: baseDir}, logger)
	logger.Writef("started pid=%d", os.Getpid())
	defer logger.Writef("stopped")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		server.Stop()
	}()
	go server.WatchIdle(5*time.Second, serverIdleTimeout, func() { _ = touchServerRecord(masterDir, baseDir) })
	server.Serve()
	return 0
}

func newServerLogger(baseDir string) *serverinternal.Logger {
	return &serverinternal.Logger{Path: filepath.Join(baseDir, "server.log"), FileMode: state.FileMode(), MaxBytes: maxServerLogSize}
}

// newRotariServer returns a server for baseDir that is served through Handle.
func newRotariServer(baseDir string) *serverinternal.Server {
	return serverinternal.New(nil, serverOperations{baseDir: baseDir}, newServerLogger(baseDir))
}

// serverOperations performs the project work behind background server
// requests for one base directory.
type serverOperations struct {
	baseDir string
}

func (ops serverOperations) Submit(request serverinternal.Request) (string, error) {
	command := model.QueuedCommand{
		Command: request.Command, Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Environment: request.Environment,
		WorkingDirectory: request.WorkingDirectory, Name: request.JobName, Stage: request.Stage, DependsOn: request.DependsOn,
	}
	return enqueueCommands(ops.baseDir, request.QueueName, []model.QueuedCommand{command}, request.Array)
}

func (ops serverOperations) Cancel(request serverinternal.Request) (string, error) {
	return cancelQueueJobs(ops.baseDir, request.QueueName, request.JobIDs, request.Wait)
}

func (ops serverOperations) Control(request serverinternal.Request) (string, error) {
	return controlQueueJobs(ops.baseDir, request.QueueName, request.JobIDs, request.Op)
}

func (ops serverOperations) StartRun(request serverinternal.Request, onDone func()) (string, error) {
	return startServerRun(ops.baseDir, request, onDone)
}

func (ops serverOperations) Run(request serverinternal.Request, progress func(serverinternal.Response)) (string, int, error) {
	return runServerSync(ops.baseDir, request, progress)
}

func (ops serverOperations) CancelRun(request serverinternal.Request) {
	_, _ = cancelQueue(ops.baseDir, request.QueueName, false)
}
