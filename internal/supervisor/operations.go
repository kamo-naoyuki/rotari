package supervisor

import (
	"errors"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Operations performs the requests of the supervisor of Project in BaseDir.
type Operations struct {
	BaseDir string
	// Project is the only project this supervisor runs; empty accepts any,
	// for tests that drive Operations directly.
	Project    string
	Controller jobcontrol.Controller
	Runner     projectrun.Runner
	// NewRunID returns a fresh run ID.
	NewRunID func() string
	// Logf receives events, such as an async run that failed, that have no
	// client to report them to. Nil drops them.
	Logf func(format string, args ...any)
}

var _ server.Operations = Operations{}

// UpdateRunClientStatus records the synchronous client's connection transition.
func (ops Operations) UpdateRunClientStatus(request server.Request, status model.RunClientStatus) {
	paths, err := state.ResolveProjectPaths(ops.BaseDir, request.QueueName)
	if err == nil {
		var lock model.LockInfo
		lock, err = state.LoadLock(paths.LockFile)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err == nil {
			err = ops.Runner.SetRunClientStatus(paths, lock.RunID, status)
		}
	}
	if err != nil {
		ops.logf("failed to update project %s run client status: %v", request.QueueName, err)
	}
}

// CancelRun cancels the project's whole active run.
func (ops Operations) CancelRun(request server.Request) {
	_, _ = ops.Controller.Cancel(ops.BaseDir, request.QueueName, "", nil, false)
}

func (ops Operations) logf(format string, args ...any) {
	if ops.Logf != nil {
		ops.Logf(format, args...)
	}
}
