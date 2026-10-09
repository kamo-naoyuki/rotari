package attachment

import (
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// CurrentLaunchOrigin is the process that ran this rotari command: its
// parent, such as the shell or script, on this host. Like a shell's wait,
// wait without a selector follows the runs that the same process started.
func CurrentLaunchOrigin() model.LaunchOrigin {
	host, _ := os.Hostname()
	parent := os.Getppid()
	return model.LaunchOrigin{Host: host, PID: parent, ProcessStart: state.ProcessStart(parent)}
}

// SameLaunchOrigin reports whether a run's recorded origin is current. A
// start time known on both sides must match, so a reused PID never matches.
func SameLaunchOrigin(recorded *model.LaunchOrigin, current model.LaunchOrigin) bool {
	if recorded == nil || recorded.Host != current.Host || recorded.PID != current.PID {
		return false
	}
	return recorded.ProcessStart == "" || current.ProcessStart == "" || recorded.ProcessStart == current.ProcessStart
}
