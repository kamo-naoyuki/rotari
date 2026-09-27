package supervisor

import (
	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/server"
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

// CancelRun cancels the project's whole active run.
func (ops Operations) CancelRun(request server.Request) {
	_, _ = ops.Controller.Cancel(ops.BaseDir, request.QueueName, "", nil, false)
}

func (ops Operations) logf(format string, args ...any) {
	if ops.Logf != nil {
		ops.Logf(format, args...)
	}
}
