package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/supervisor"
)

const maxServerLogSize = 1 << 20

// serverIdleTimeout stops a supervisor whose client never sent its run
// request.
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

// startSupervisor starts the supervisor of one run of paths' project. Tests
// replace it to serve the run in their own process.
var startSupervisor = startSupervisorProcess

// startSupervisorProcess starts the supervisor of one run of paths' project
// as a child of this process, so it and the run's jobs inherit this
// command's working directory and environment; a supervisor is never reused.
func startSupervisorProcess(paths state.ProjectPaths) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to detect executable path: %w", err)
	}
	command := []string{exe, "__server", "--basedir", paths.BaseDir, "--project-name", paths.ProjectName}
	_, err = serverinternal.Start(paths.ProjectDir, command, serverLogPath(paths.BaseDir))
	if errors.Is(err, serverinternal.ErrAlreadyRunning) {
		return fmt.Errorf("project %q already has a run starting or running", paths.ProjectName)
	}
	return err
}

// runningSupervisor is a supervisor that answered a ping.
type runningSupervisor struct {
	paths state.ProjectPaths
	pid   int
}

// runningSupervisors returns the supervisors of baseDir's projects that
// answer a ping, in project order.
func runningSupervisors(baseDir string) ([]runningSupervisor, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var supervisors []runningSupervisor
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, err := state.ResolveProjectPaths(baseDir, entry.Name())
		if err != nil {
			continue
		}
		response, err := serverinternal.SendRequest(paths.ProjectDir, serverinternal.Request{Op: serverinternal.OpPing})
		if err == nil && response.OK {
			supervisors = append(supervisors, runningSupervisor{paths: paths, pid: response.PID})
		}
	}
	return supervisors, nil
}

// cmdServerStatus reports the running supervisors of the selected base
// directory, one per project with an active run.
func cmdServerStatus(args []string) int {
	fs := flag.NewFlagSet("server status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	supervisors, err := runningSupervisors(baseDir)
	if err != nil {
		printErrorf("failed to list projects: %v", err)
		return 1
	}
	if len(supervisors) == 0 {
		printError("server is not running")
		return 1
	}
	for _, supervisor := range supervisors {
		fmt.Printf("server is running pid=%d project=%s state=%s\n", supervisor.pid, supervisor.paths.ProjectName, baseDir)
	}
	return 0
}

// cmdServerList lists registered servers from the master registry.
func cmdServerList(args []string) int {
	fs := flag.NewFlagSet("server list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	masterDir, err := state.ResolveMasterDir(*masterdir)
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

// cmdServerRequest sends a lifecycle operation to every running supervisor
// of the selected base directory.
func cmdServerRequest(args []string, op string) int {
	fs := flag.NewFlagSet("server request", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	supervisors, err := runningSupervisors(baseDir)
	if err != nil {
		printErrorf("failed to list projects: %v", err)
		return 1
	}
	if len(supervisors) == 0 {
		printError("server is not running")
		return 1
	}
	code := 0
	for _, supervisor := range supervisors {
		response, err := serverinternal.SendRequest(supervisor.paths.ProjectDir, serverinternal.Request{Op: op})
		if err != nil {
			printErrorf("failed to contact server of project %q: %v", supervisor.paths.ProjectName, err)
			code = 1
			continue
		}
		if !response.OK {
			printError(response.Message)
			code = 1
			continue
		}
		fmt.Print(colorMessage(response.Message + " (project " + supervisor.paths.ProjectName + ")\n"))
	}
	return code
}

// cmdServerProcess runs the hidden supervisor process of one run of one
// project.
func cmdServerProcess(args []string) int {
	fs := flag.NewFlagSet("__server", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	paths, err := state.ResolveProjectPaths(*basedir, *projectName)
	if err != nil {
		printErrorf("failed to resolve project: %v", err)
		return 1
	}
	return runServer(paths)
}

func runServer(paths state.ProjectPaths) int {
	logger := newServerLogger(paths)
	// The project exists: the run client checked it before starting this
	// process, and the supervisor must not create one.
	listener, release, err := serverinternal.Listen(paths.ProjectDir, state.FileMode())
	if err != nil {
		// A detached server's stderr is discarded, so record why it stopped.
		if !errors.Is(err, serverinternal.ErrAlreadyRunning) {
			logger.Writef("start failed: %v", err)
		}
		printError(err)
		return 1
	}
	defer release()
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	record := serverRecord{
		BaseDir: paths.BaseDir, Project: paths.ProjectName, Socket: serverinternal.SocketPath(paths.ProjectDir), PID: os.Getpid(),
		StartedAt: nowRFC3339(), LastSeen: nowRFC3339(),
	}
	if err := registerServer(masterDir, record); err != nil {
		printErrorf("failed to register server: %v", err)
		return 1
	}
	defer unregisterServer(masterDir, paths.BaseDir, paths.ProjectName)

	server := serverinternal.New(listener, supervisorOperations(paths, logger), logger)
	logger.Writef("started pid=%d", os.Getpid())
	defer logger.Writef("stopped")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		server.Stop()
	}()
	go server.WatchIdle(5*time.Second, serverIdleTimeout, func() { _ = touchServerRecord(masterDir, paths.BaseDir, paths.ProjectName) })
	server.Serve()
	return 0
}

func serverLogPath(baseDir string) string {
	return filepath.Join(baseDir, "server.log")
}

// newServerLogger returns the log of the supervisor of paths' project, which
// shares the base directory's server.log with other projects' supervisors.
func newServerLogger(paths state.ProjectPaths) *serverinternal.Logger {
	return &serverinternal.Logger{Path: serverLogPath(paths.BaseDir), FileMode: state.FileMode(), MaxBytes: maxServerLogSize, Prefix: "project=" + paths.ProjectName + " "}
}

// newRotariServer returns a supervisor for paths' project that is served
// through Handle.
func newRotariServer(paths state.ProjectPaths) *serverinternal.Server {
	logger := newServerLogger(paths)
	return serverinternal.New(nil, supervisorOperations(paths, logger), logger)
}

// supervisorOperations performs the run of paths' project.
func supervisorOperations(paths state.ProjectPaths, logger *serverinternal.Logger) supervisor.Operations {
	return supervisor.Operations{
		BaseDir: paths.BaseDir, Project: paths.ProjectName, Controller: jobController(), Runner: projectRunner(),
		NewRunID: makeRunID, Logf: logger.Writef,
	}
}
