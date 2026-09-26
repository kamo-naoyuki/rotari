package supervisor

import (
	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/server"
)

// Operations performs the requests of the background server for BaseDir.
type Operations struct {
	BaseDir    string
	Editor     queueops.Editor
	Controller jobcontrol.Controller
	Runner     projectrun.Runner
	// NewRunID returns a fresh run ID.
	NewRunID func() string
	// Executable returns the program started as an async run's worker; it
	// must accept the arguments of run.WorkerArgs. Nil means os.Executable.
	Executable func() (string, error)
	// Printf receives a line for each started async worker. Nil drops it.
	Printf func(format string, args ...any)
}

var _ server.Operations = Operations{}

// Submit adds the request's command to its project's queue.
func (ops Operations) Submit(request server.Request) (string, error) {
	command := model.QueuedCommand{
		Command: request.Command, Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Environment: request.Environment,
		WorkingDirectory: request.WorkingDirectory, Name: request.JobName, Stage: request.Stage, DependsOn: request.DependsOn,
		DependsOnFinished: request.DependsOnFinished, Timeout: request.Timeout, Retry: request.JobRetry,
		RetryDelay: request.RetryDelay, RetryBackoff: request.RetryBackoff, RetryMaxDelay: request.RetryMaxDelay,
	}
	return ops.Editor.Add(ops.BaseDir, request.QueueName, []model.QueuedCommand{command}, request.Array)
}

// Cancel cancels the requested jobs, or the whole active run.
func (ops Operations) Cancel(request server.Request) (string, error) {
	return ops.Controller.Cancel(ops.BaseDir, request.QueueName, request.RunID, request.JobIDs, request.Wait)
}

// Control suspends or resumes the requested jobs, as named by request.Op.
func (ops Operations) Control(request server.Request) (string, error) {
	return ops.Controller.Control(ops.BaseDir, request.QueueName, request.RunID, request.JobIDs, request.Op)
}

// CancelRun cancels the project's whole active run.
func (ops Operations) CancelRun(request server.Request) {
	_, _ = ops.Controller.Cancel(ops.BaseDir, request.QueueName, "", nil, false)
}

func (ops Operations) printf(format string, args ...any) {
	if ops.Printf != nil {
		ops.Printf(format, args...)
	}
}
