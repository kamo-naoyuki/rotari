package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type localIdentity struct {
	Executor     string `json:"executor"`
	Host         string `json:"host"`
	PID          int    `json:"pid"`
	PIDNamespace string `json:"pid_namespace"`
	ProcessStart string `json:"process_start,omitempty"`
}

// ExecutionProber is a bounded, read-only batch. It caches only SGE's whole
// queue listing, never the verdict for a particular attempt.
type ExecutionProber struct {
	store     state.Store
	ctx       context.Context
	run       func(context.Context, []string) ([]byte, error)
	runInput  func(context.Context, []string, []byte) ([]byte, error)
	sgeRead   bool
	sgeOutput []byte
	sgeErr    error
}

func NewExecutionProber(store state.Store, ctx context.Context) *ExecutionProber {
	return &ExecutionProber{store: store, ctx: ctx, run: runExecutionQuery, runInput: runExecutionQueryInput}
}

// Assess never treats an outcome file as proof of termination. Missing launch
// identities (including the acceptance/publication crash window) stay unknown.
func (probe *ExecutionProber) Assess(dir, backend string) model.RecoveryExecution {
	result := model.RecoveryExecution{Backend: backend, State: "unknown", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: "no termination evidence available"}
	if err := probe.ctx.Err(); err != nil {
		result.Source = "assessment_budget"
		result.Reason = "execution assessment budget is unavailable: " + err.Error()
		return result
	}
	if backend == "" || backend == "unknown" {
		var recorded struct {
			Executor string `json:"executor"`
		}
		if err := probe.store.ReadJSON(filepath.Join(dir, "job.json"), &recorded); err != nil {
			result.Source = "launch_identity"
			result.Reason = "cannot determine launch backend: " + err.Error()
			return result
		}
		backend = recorded.Executor
		result.Backend = backend
	}
	switch backend {
	case "local":
		result = probe.local(dir, result)
	case "ssh":
		result = probe.ssh(dir, result)
	case "slurm", "pbs", "sge", "lsf":
		result = probe.scheduler(dir, result)
	default:
		result.Source = "unsupported"
		result.Reason = "termination probing is not supported for this backend; independently confirm stopped work"
	}
	return result
}

func (probe *ExecutionProber) local(dir string, result model.RecoveryExecution) model.RecoveryExecution {
	result.Source = "local_process_group"
	result.Scope = "recorded process group in the recorded host and PID namespace; descendants that leave the group are outside this scope"
	var identity localIdentity
	if err := probe.store.ReadJSON(filepath.Join(dir, "job.json"), &identity); err != nil {
		result.Reason = "cannot read local process identity: " + err.Error()
		return result
	}
	host, err := os.Hostname()
	if err != nil {
		result.Reason = "cannot identify inspection host: " + err.Error()
		return result
	}
	ns := state.PIDNamespace()
	switch {
	case identity.Executor != "local" || identity.PID <= 1 || identity.PID > (1<<31)-1:
		result.Reason = "local launch identity is incomplete; a process may have started before identity publication"
	case identity.Host == "" || identity.Host != host:
		result.Reason = "local process belongs to another or unrecorded host"
	case ns == "" || identity.PIDNamespace == "" || ns != identity.PIDNamespace:
		result.Reason = "local PID namespace does not match or cannot be verified"
	default:
		result.NativeID = strconv.Itoa(identity.PID)
		err := syscall.Kill(-identity.PID, 0)
		result.State, result.Reason, result.PossiblyActive = localGroupEvidence(identity.ProcessStart, state.ProcessStart(identity.PID), err)
	}
	return result
}

func localGroupEvidence(recordedStart, currentStart string, err error) (string, string, bool) {
	switch {
	case errors.Is(err, syscall.ESRCH):
		return "stopped", "recorded process group no longer exists in the verified host/PID namespace", false
	case err != nil:
		return "unknown", "cannot inspect recorded process group: " + err.Error(), errors.Is(err, syscall.EPERM)
	case recordedStart == "" || currentStart == "":
		return "unknown", "process group exists but its leader start time cannot be verified; possibly live descendants or a reused group", true
	case recordedStart != currentStart:
		return "unknown", "process group exists but leader identity differs; group reuse cannot establish the original execution stopped", true
	default:
		return "active", "process group exists and its leader start time matches the recorded execution", false
	}
}

type executionMetadata struct {
	Executor    string   `json:"executor"`
	Host        string   `json:"host"`
	SSHOptions  []string `json:"ssh_options"`
	RemoteToken string   `json:"remote_token"`
	SlurmID     string   `json:"slurm_job_id"`
	PBSID       string   `json:"pbs_job_id"`
	SGEID       string   `json:"sge_job_id"`
	LSFID       string   `json:"lsf_job_id"`
}

func (probe *ExecutionProber) ssh(dir string, result model.RecoveryExecution) model.RecoveryExecution {
	result.Source = "ssh_remote_token"
	var metadata executionMetadata
	if err := probe.store.ReadJSON(filepath.Join(dir, "job.json"), &metadata); err != nil {
		result.Reason = "cannot read SSH launch identity: " + err.Error()
		return result
	}
	if metadata.Executor != "ssh" || metadata.Host == "" || !validSSHRemoteToken(metadata.RemoteToken) {
		result.Reason = "SSH host or remote execution token is missing or invalid"
		return result
	}
	result.Scope = "remote process group on " + metadata.Host + " identified by its private token; descendants that leave the group are outside this scope"
	command := append([]string{"-o", "BatchMode=yes", "-o", "LogLevel=ERROR"}, metadata.SSHOptions...)
	command = append(command, "--", metadata.Host, "sh", "-s")
	output, err := probe.queryInput(command, []byte(sshExecutionProbeScript(metadata.RemoteToken)), false)
	if err != nil {
		result.Reason = "bounded remote SSH execution query failed: " + err.Error()
		return result
	}
	state, possiblyActive, reason := parseSSHExecutionProbe(output)
	result.State, result.PossiblyActive, result.Reason = state, possiblyActive, reason
	return result
}

func (probe *ExecutionProber) scheduler(dir string, result model.RecoveryExecution) model.RecoveryExecution {
	result.Source = "scheduler"
	result.Scope = "native job/task in the scheduler selected by the current command environment; no compute-node process-group inspection"
	var metadata executionMetadata
	if err := probe.store.ReadJSON(filepath.Join(dir, "job.json"), &metadata); err != nil {
		result.Reason = "cannot read scheduler launch identity: " + err.Error()
		return result
	}
	if metadata.Executor != result.Backend {
		result.Reason = "scheduler identity backend does not match the attempt"
		return result
	}
	var queue, accounting []string
	switch result.Backend {
	case "slurm":
		result.NativeID = metadata.SlurmID
		queue = []string{"squeue", "--noheader", "--jobs", result.NativeID, "--format=%i|%T"}
		accounting = []string{"sacct", "--noheader", "--parsable2", "--allocations", "--jobs", result.NativeID, "--format=JobID,State,ExitCode"}
	case "pbs":
		result.NativeID = metadata.PBSID
		queue = pbsQueueCommand(result.NativeID)
		accounting = pbsAccountingCommand(result.NativeID)
	case "sge":
		result.NativeID = metadata.SGEID
		queue = sgeQueueCommand()
		accounting = sgeAccountingCommand(result.NativeID)
	case "lsf":
		result.NativeID = metadata.LSFID
		queue = []string{"bjobs", "-noheader", "-o", "jobid stat", result.NativeID}
		accounting = []string{"bjobs", "-a", "-noheader", "-o", "jobid stat exit_code", result.NativeID}
	}
	if !validNativeIdentity(result.Backend, result.NativeID) {
		result.Reason = "native job/task identity is missing or invalid; submission may have succeeded before publication"
		return result
	}
	queueOutput, queueErr := probe.query(queue, result.Backend == "sge")
	active, queueKnown := executionQueueState(result.Backend, result.NativeID, queueOutput)
	if queueErr == nil && queueKnown && active {
		result.State = "active"
		result.Source = "scheduler_queue"
		result.Reason = "fresh queue query reports the recorded job/task active, queued, held or suspended"
		return result
	}
	accountingOutput, accountingErr := probe.query(accounting, false)
	terminal, accountingActive := executionAccountingState(result.Backend, result.NativeID, accountingOutput)
	if accountingErr == nil && accountingActive {
		result.State = "active"
		result.Source = "scheduler_accounting"
		result.Reason = "fresh accounting query reports the recorded job/task still active"
		if queueErr != nil {
			result.Reason += "; queue query failed: " + queueErr.Error()
		}
		if queueErr == nil && !queueKnown {
			result.Reason += "; queue output could not be classified"
		}
		if queueErr == nil && queueKnown && strings.TrimSpace(string(queueOutput)) != "" {
			result.Conflict = true
			result.Reason += "; conflicts with a terminal queue observation, which does not override activity"
		}
		return result
	}
	// A failed/unparseable queue query cannot exclude a requeue that conflicts
	// with an old accounting record.
	if queueErr == nil && queueKnown && accountingErr == nil && terminal {
		result.State = "stopped"
		result.Source = "scheduler_accounting"
		result.Reason = "fresh terminal accounting record and no active queue observation for the recorded native job/task"
		return result
	}
	result.Reason = "queue absence or terminal wrapper outcome alone does not establish scheduler termination"
	if queueErr != nil {
		result.Reason += "; queue query failed: " + queueErr.Error()
	}
	if !queueKnown && queueErr == nil {
		result.Reason += "; queue output could not be classified"
	}
	if accountingErr != nil {
		result.Reason += "; accounting query failed: " + accountingErr.Error()
	}
	if accountingErr == nil && !terminal {
		result.Reason += "; matching terminal accounting record is unavailable"
	}
	return result
}

var executionNativeIDs = map[string]*regexp.Regexp{
	"slurm": regexp.MustCompile(`^[1-9][0-9]*(?:_[0-9]+)?$`),
	"sge":   regexp.MustCompile(`^[1-9][0-9]*$`),
	"pbs":   regexp.MustCompile(`^[1-9][0-9]*(?:\[[0-9]+\])?(?:\.[A-Za-z0-9_.-]+)?$`),
	"lsf":   regexp.MustCompile(`^[1-9][0-9]*(?:\[[0-9]+\])?$`),
}

func validNativeIdentity(backend, id string) bool {
	pattern, ok := executionNativeIDs[backend]
	return ok && pattern.MatchString(id)
}

func sshExecutionProbeScript(token string) string {
	return `#!/bin/sh
set -eu
runtime_base=${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}
state_root="$runtime_base/rotari-$(id -u)"
state_dir="$state_root/` + token + `"
unknown() { printf 'ROTARI_EXECUTION:unknown\n'; }
if [ ! -d "$state_root" ] || [ -L "$state_root" ] ||
   [ "$(stat -c %u "$state_root" 2>/dev/null)" != "$(id -u)" ] ||
   [ "$(stat -c %a "$state_root" 2>/dev/null)" != "700" ]; then
	unknown
	exit 0
fi
[ -r /proc/self/stat ] || { unknown; exit 0; }
if [ ! -d "$state_dir" ]; then
	unknown
	exit 0
fi
[ ! -L "$state_dir" ] || { unknown; exit 0; }
if [ ! -f "$state_dir/process" ] || [ -L "$state_dir/process" ]; then
	unknown
	exit 0
fi
read remote_pid recorded_start extra < "$state_dir/process" || { unknown; exit 0; }
case "$remote_pid:$recorded_start" in
	*[!0-9:]*|:*|*:) unknown; exit 0 ;;
esac
[ -z "${extra:-}" ] || { unknown; exit 0; }
if process_stat=$(cat "/proc/$remote_pid/stat" 2>/dev/null); then
	process_fields=${process_stat##*) }
	set -- $process_fields
	if [ "$#" -ge 20 ]; then
		remote_state=$1
		remote_pgid=$3
		shift 19
		remote_start=$1
		if [ "$remote_pgid" = "$remote_pid" ] && [ "$remote_start" = "$recorded_start" ] && [ "$remote_state" != Z ] && [ "$remote_state" != X ]; then
			printf 'ROTARI_EXECUTION:active\n'
			exit 0
		fi
	fi
fi
process_groups=$(ps -eo pgid=,stat= 2>/dev/null) || { unknown; exit 0; }
if printf '%s\n' "$process_groups" | awk -v pgid="$remote_pid" '$1 == pgid && $2 !~ /^[ZX]/ { found=1 } END { exit !found }'; then
	unknown
	exit 0
fi
if [ -f "$state_dir/finished" ] && [ ! -L "$state_dir/finished" ]; then
	printf 'ROTARI_EXECUTION:stopped\n'
else
	unknown
fi
`
}

func parseSSHExecutionProbe(output []byte) (state string, possiblyActive bool, reason string) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 {
		switch strings.TrimSpace(lines[0]) {
		case "ROTARI_EXECUTION:active":
			return "active", false, "remote token query verified the recorded process identity and its process group is active"
		case "ROTARI_EXECUTION:stopped":
			return "stopped", false, "remote token query verified completion and found no active process in the recorded process group"
		case "ROTARI_EXECUTION:unknown":
			return "unknown", false, "remote token query could not verify process-group termination"
		}
	}
	return "unknown", possiblyActive, "remote token query returned missing, malformed or ambiguous evidence"
}

func (probe *ExecutionProber) query(command []string, sge bool) ([]byte, error) {
	return probe.queryInput(command, nil, sge)
}

func (probe *ExecutionProber) queryInput(command []string, input []byte, sge bool) ([]byte, error) {
	if sge && probe.sgeRead {
		return probe.sgeOutput, probe.sgeErr
	}
	var output []byte
	var err error
	if probe.ctx.Err() != nil {
		err = probe.ctx.Err()
	} else {
		delay := schedulerQueryGate.delay("query", 0)
		timer := time.NewTimer(max(delay, 0))
		select {
		case <-probe.ctx.Done():
			err = probe.ctx.Err()
		case <-timer.C:
			if input == nil {
				output, err = probe.run(probe.ctx, command)
			} else {
				output, err = probe.runInput(probe.ctx, command, input)
			}
		}
		timer.Stop()
		if probe.ctx.Err() != nil {
			err = probe.ctx.Err()
		}
	}
	if sge {
		probe.sgeRead = true
		probe.sgeOutput, probe.sgeErr = output, err
	}
	return output, err
}

type probeOutput struct {
	data     []byte
	overflow bool
}

func (output *probeOutput) Write(data []byte) (int, error) {
	n := len(data)
	remaining := schedulerProbeOutputLimit - len(output.data)
	if len(data) > remaining {
		data = data[:remaining]
		output.overflow = true
	}
	output.data = append(output.data, data...)
	return n, nil
}

func runExecutionQuery(parent context.Context, command []string) ([]byte, error) {
	return runExecutionQueryInput(parent, command, nil)
}

func runExecutionQueryInput(parent context.Context, command []string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 200 * time.Millisecond
	if input != nil {
		cmd.Stdin = strings.NewReader(string(input))
	}
	output := &probeOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	if ctx.Err() != nil {
		return output.data, fmt.Errorf("%s query: %w", command[0], ctx.Err())
	}
	if output.overflow {
		return nil, fmt.Errorf("%s output exceeded %d bytes", command[0], schedulerProbeOutputLimit)
	}
	if err != nil {
		return output.data, fmt.Errorf("%s: %w: %s", command[0], err, strings.TrimSpace(string(output.data)))
	}
	return output.data, nil
}
