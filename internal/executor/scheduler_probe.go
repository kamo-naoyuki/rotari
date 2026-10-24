package executor

import (
	"github.com/rotari-dev/rotari/internal/state"
)

const schedulerProbeOutputLimit = 64 << 10

// SchedulerProbe is the output of one read-only scheduler query.
type SchedulerProbe struct {
	Command []string `json:"command"`
	Output  string   `json:"output"`
	Error   string   `json:"error,omitempty"`
}

// ProbeSchedulerJob runs the queue and accounting queries rotari polls with
// for the scheduler job recorded in attemptDir's job.json, and returns their
// raw output. A job without a scheduler job ID has none.
func ProbeSchedulerJob(store state.Store, attemptDir string) []SchedulerProbe {
	var metadata struct {
		SlurmJobID string `json:"slurm_job_id"`
		PBSJobID   string `json:"pbs_job_id"`
		SGEJobID   string `json:"sge_job_id"`
		LSFJobID   string `json:"lsf_job_id"`
	}
	path, err := state.ValidatedStateFile(attemptDir, "job.json")
	if err != nil || store.ReadJSON(path, &metadata) != nil {
		return nil
	}
	var commands [][]string
	var run func(...string) ([]byte, error)
	switch {
	case metadata.SlurmJobID != "":
		commands = [][]string{slurmQueueCommand(metadata.SlurmJobID), slurmAccountingCommand(metadata.SlurmJobID)}
		run = func(command ...string) ([]byte, error) { return runSlurmCommand(command[0], command[1:]...) }
	case metadata.PBSJobID != "":
		commands = [][]string{pbsQueueCommand(metadata.PBSJobID), pbsAccountingCommand(metadata.PBSJobID)}
		run = func(command ...string) ([]byte, error) { return runPBSCommand(command[0], command[1:]...) }
	case metadata.SGEJobID != "":
		commands = [][]string{sgeQueueCommand(), sgeAccountingCommand(metadata.SGEJobID)}
		run = func(command ...string) ([]byte, error) { return runSGECommand(command[0], command[1:]...) }
	case metadata.LSFJobID != "":
		commands = append([][]string{lsfQueueCommand(metadata.LSFJobID)}, lsfAccountingCommands(metadata.LSFJobID)...)
		run = func(command ...string) ([]byte, error) { return runLSFCommandWithInput(nil, command...) }
	default:
		return nil
	}
	probes := make([]SchedulerProbe, 0, len(commands))
	for _, command := range commands {
		schedulerQueryGate.wait("query", 0)
		output, err := run(command...)
		if len(output) > schedulerProbeOutputLimit {
			output = append(output[:schedulerProbeOutputLimit:schedulerProbeOutputLimit], "\n... (output cut)\n"...)
		}
		probe := SchedulerProbe{Command: command, Output: string(output)}
		if err != nil {
			probe.Error = err.Error()
		}
		probes = append(probes, probe)
	}
	return probes
}
