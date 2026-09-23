package executor

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type sshJobMetadata struct {
	Executor    string   `json:"executor"`
	JobID       string   `json:"job_id"`
	Command     []string `json:"command"`
	Host        string   `json:"host"`
	PID         int      `json:"pid"`
	SSHOptions  []string `json:"ssh_options,omitempty"`
	RemoteToken string   `json:"remote_token,omitempty"`
	SubmittedAt string   `json:"submitted_at"`
}

// SSHCommandPath is the ssh binary invoked to run remote jobs; overridable
// (e.g. in tests, or validateExecutorCommand's availability check) since it
// is not always at a fixed path.
var SSHCommandPath = "/usr/bin/ssh"

// SSH runs jobs on a remote host over ssh, tracking the local ssh client
// process for the job's lifetime.
type SSH struct {
	Store state.Store
}

func NewSSH(store state.Store) SSH {
	return SSH{Store: store}
}

type sshProcess struct {
	command *exec.Cmd
	output  *os.File
}

var sshProcesses = struct {
	sync.Mutex
	commands map[int]sshProcess
}{commands: make(map[int]sshProcess)}

func (SSH) Name() string { return "ssh" }

func (ssh SSH) Submit(runDir string, job model.JobSpec, options []string) (JobHandle, error) {
	host, sshOptions, err := SSHTarget(options)
	if err != nil {
		return JobHandle{}, err
	}
	remoteToken, err := makeSSHRemoteToken()
	if err != nil {
		return JobHandle{}, err
	}
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		return JobHandle{}, err
	}
	if err := os.MkdirAll(jobDir, ssh.Store.DirectoryMode); err != nil {
		return JobHandle{}, err
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		return JobHandle{}, err
	}
	output, err := os.Create(filepath.Join(jobDir, "output"))
	if err != nil {
		return JobHandle{}, err
	}
	cmd := exec.Command(SSHCommandPath, append(sshOptions, "--", host, "sh", "-s")...)
	cmd.Stdin = strings.NewReader(sshWrapperScript(job.Command, job.Environment, job.WorkingDirectory, remoteToken))
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		return JobHandle{}, fmt.Errorf("ssh %s: %w", host, err)
	}
	metadata := sshJobMetadata{Executor: "ssh", JobID: job.ID, Command: job.Command, Host: host, PID: cmd.Process.Pid, SSHOptions: sshOptions, RemoteToken: remoteToken, SubmittedAt: nowRFC3339()}
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = output.Close()
		return JobHandle{}, err
	}
	sshProcesses.Lock()
	sshProcesses.commands[cmd.Process.Pid] = sshProcess{command: cmd, output: output}
	sshProcesses.Unlock()
	return JobHandle{Job: job, Native: strconv.Itoa(cmd.Process.Pid)}, nil
}

func (ssh SSH) Wait(runDir string, handle JobHandle) model.JobResult {
	jobDir, err := state.SafeJoin(runDir, handle.Job.ID)
	if err != nil {
		return model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command, ExitCode: 1, Error: err.Error()}
	}
	metadata, err := readSSHMetadata(ssh.Store, jobDir)
	if err != nil {
		return model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command, ExitCode: 1, Error: err.Error()}
	}
	sshProcesses.Lock()
	process, ok := sshProcesses.commands[metadata.PID]
	sshProcesses.Unlock()
	if !ok {
		return model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command, ExitCode: 1, Error: "SSH job is no longer managed by this process"}
	}
	err = process.command.Wait()
	_ = process.output.Close()
	sshProcesses.Lock()
	delete(sshProcesses.commands, metadata.PID)
	sshProcesses.Unlock()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}
	status := WrapperStatus{Phase: "finished", ExitCode: exitCode, FinishedAt: nowRFC3339(), Hosts: []string{metadata.Host}}
	if err != nil {
		status.Error = err.Error()
	}
	_ = state.WriteJSON(filepath.Join(jobDir, "status.json"), status)
	return jobResultFromStatus(handle.Job.ID, handle.Job.Command, status)
}

func (ssh SSH) Cancel(jobDir string) error {
	metadata, err := readSSHMetadata(ssh.Store, jobDir)
	if err != nil {
		return err
	}
	if metadata.RemoteToken != "" {
		args := append(append([]string{}, metadata.SSHOptions...), "--", metadata.Host, "sh", "-s")
		cmd := exec.Command(SSHCommandPath, args...)
		cmd.Stdin = strings.NewReader(sshCancelScript(metadata.RemoteToken))
		if output, err := cmd.CombinedOutput(); err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = err.Error()
			}
			return fmt.Errorf("cancel remote SSH job: %s", message)
		}
		return nil
	}
	sshProcesses.Lock()
	process, ok := sshProcesses.commands[metadata.PID]
	sshProcesses.Unlock()
	if !ok || process.command.Process == nil {
		return fmt.Errorf("SSH job is not managed by this process")
	}
	return process.command.Process.Signal(syscall.SIGTERM)
}

func readSSHMetadata(store state.Store, jobDir string) (sshJobMetadata, error) {
	var metadata sshJobMetadata
	if err := store.ReadJSON(filepath.Join(jobDir, "job.json"), &metadata); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sshJobMetadata{}, fmt.Errorf("job is not running")
		}
		if errors.Is(err, state.ErrInvalidJSON) {
			return sshJobMetadata{}, fmt.Errorf("invalid SSH metadata")
		}
		return sshJobMetadata{}, fmt.Errorf("invalid SSH metadata: %w", err)
	}
	if metadata.Executor != "ssh" {
		return sshJobMetadata{}, fmt.Errorf("invalid SSH metadata")
	}
	if metadata.RemoteToken != "" && !validSSHRemoteToken(metadata.RemoteToken) {
		return sshJobMetadata{}, fmt.Errorf("invalid SSH metadata")
	}
	return metadata, nil
}

// SSHTarget parses the SSH executor's options into a target host and the
// remaining ssh(1) options, shared by both job submission and
// cmd/rotari's up-front queue validation.
func SSHTarget(options []string) (string, []string, error) {
	expanded, err := ExpandShellOptions(options)
	if err != nil {
		return "", nil, err
	}
	if len(expanded) == 0 || expanded[0] == "" {
		return "", nil, fmt.Errorf("SSH executor requires its first executor option to be the target host")
	}
	if strings.HasPrefix(expanded[0], "-") {
		return "", nil, fmt.Errorf("invalid SSH target host %q", expanded[0])
	}
	return expanded[0], expanded[1:], nil
}

func makeSSHRemoteToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate SSH remote token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func validSSHRemoteToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func sshWrapperScript(command []string, environment []string, workingDirectory, remoteToken string) string {
	exports := make([]string, 0, len(environment))
	for _, entry := range environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			exports = append(exports, "export "+parts[0]+"="+ShellQuote(parts[1]))
		}
	}
	quoted := make([]string, 0, len(command))
	for _, arg := range command {
		quoted = append(quoted, ShellQuote(arg))
	}
	changeDirectory := ""
	if workingDirectory != "" {
		changeDirectory = "cd " + ShellQuote(workingDirectory) + " || exit 1\n"
	}
	return `#!/bin/sh
set +e
umask 077
runtime_base=${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}
state_root="$runtime_base/rotari-$(id -u)"
mkdir "$state_root" 2>/dev/null || { [ -d "$state_root" ] && [ ! -L "$state_root" ]; } || exit 1
[ "$(stat -c %u "$state_root" 2>/dev/null)" = "$(id -u)" ] || exit 1
[ "$(stat -c %a "$state_root" 2>/dev/null)" = "700" ] || exit 1
state_dir="$state_root/` + remoteToken + `"
mkdir "$state_dir" || exit 1
read_start_time() {
	process_stat=$(cat "/proc/$1/stat" 2>/dev/null) || return 1
	process_stat=${process_stat##*) }
	set -- $process_stat
	[ "$#" -ge 20 ] || return 1
	shift 19
	printf '%s\n' "$1"
}
` + strings.Join(exports, "\n") + "\n" + changeDirectory + `setsid sh -c 'exec "$@"' sh ` + strings.Join(quoted, " ") + ` &
remote_pid=$!
if remote_start=$(read_start_time "$remote_pid"); then
	printf '%s %s\n' "$remote_pid" "$remote_start" > "$state_dir/process.tmp" && mv "$state_dir/process.tmp" "$state_dir/process"
fi
wait "$remote_pid"
exit_code=$?
rm -rf "$state_dir"
exit "$exit_code"
`
}

func sshCancelScript(remoteToken string) string {
	return `#!/bin/sh
set -eu
runtime_base=${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}
state_root="$runtime_base/rotari-$(id -u)"
state_dir="$state_root/` + remoteToken + `"
attempt=0
while [ ! -f "$state_dir/process" ]; do
	attempt=$((attempt + 1))
	[ "$attempt" -lt 50 ] || { echo 'SSH job is not running' >&2; exit 1; }
	sleep 0.1
done
[ "$(stat -c %u "$state_root" 2>/dev/null)" = "$(id -u)" ] || { echo 'invalid SSH job state owner' >&2; exit 1; }
[ ! -L "$state_root" ] && [ "$(stat -c %a "$state_root" 2>/dev/null)" = "700" ] || { echo 'invalid SSH job state permissions' >&2; exit 1; }
[ -f "$state_dir/process" ] || { echo 'SSH job is not running' >&2; exit 1; }
read remote_pid recorded_start extra < "$state_dir/process"
case "$remote_pid:$recorded_start" in
	*[!0-9:]*|:*|*:) echo 'invalid SSH job state' >&2; exit 1 ;;
esac
[ -z "${extra:-}" ] || { echo 'invalid SSH job state' >&2; exit 1; }
process_stat=$(cat "/proc/$remote_pid/stat" 2>/dev/null) || { echo 'SSH job is not running' >&2; exit 1; }
process_stat=${process_stat##*) }
set -- $process_stat
[ "$#" -ge 20 ] || { echo 'invalid remote process state' >&2; exit 1; }
shift 19
[ "$1" = "$recorded_start" ] || { echo 'remote PID has been reused; refusing to signal it' >&2; exit 1; }
remote_pgid=$(ps -o pgid= -p "$remote_pid" | tr -d ' ')
[ "$remote_pgid" = "$remote_pid" ] || { echo 'remote process group does not match recorded PID' >&2; exit 1; }
kill -TERM "-$remote_pid"
`
}
