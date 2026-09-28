package executor

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// TimeoutExitCode is the exit code recorded for a job stopped by its timeout,
// matching GNU timeout.
const TimeoutExitCode = 124

// timeoutGraceSeconds is how long a timed-out job may take to exit after
// SIGTERM, for example to save a checkpoint, before it is killed. Tests
// shorten it.
var timeoutGraceSeconds = 30

// TimeoutMessage describes a job stopped by a timeout of seconds.
func TimeoutMessage(seconds int) string {
	return fmt.Sprintf("timed out after %s", time.Duration(seconds)*time.Second)
}

// StatusWrapperScript runs a job's command and records its progress in
// status.json. A positive timeout (a model.ParseTimeout duration) stops the
// command that long after it starts.
func StatusWrapperScript(command []string, jobDir string, environment []string, workingDirectory, timeout string, envMode ...string) string {
	return statusWrapperScript(shellCommandLine(command), jobDir, environment, workingDirectory, timeout, envMode...)
}

func statusWrapperScript(commandLine, jobDir string, environment []string, workingDirectory, timeout string, envMode ...string) string {
	statusPath := filepath.Join(jobDir, "status.json")
	exports := make([]string, 0, len(environment))
	for _, entry := range environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			exports = append(exports, "export "+parts[0]+"="+ShellQuote(parts[1]))
		}
	}
	changeDirectory := ""
	if workingDirectory != "" {
		changeDirectory = "cd " + ShellQuote(workingDirectory) + " || exit 1\n"
	}
	changeDirectory += "export PWD=\"$PWD\"\n"
	seconds := model.TimeoutSeconds(timeout)
	if len(envMode) > 0 && envMode[0] == model.EnvModeNone {
		cleanEnvironment := []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
		cleanEnvironment = append(cleanEnvironment, environment...)
		assignments := make([]string, 0, len(cleanEnvironment))
		for _, entry := range cleanEnvironment {
			assignments = append(assignments, ShellQuote(entry))
		}
		commandLine = "env -i " + strings.Join(assignments, " ") + " " + commandLine
	}
	return "#!/bin/sh\nset +e\n" + processGroupLeaderShell(seconds) + "status_path=" + ShellQuote(statusPath) + "\n" + strings.Join(exports, "\n") + "\n" + changeDirectory +
		statusWrapperBody(commandLine, seconds)
}

func StatusWrapperScriptWithDestinations(command []string, jobDir string, environment []string, workingDirectory, timeout, logMode, openMode string, outputPaths, errorPaths []string, envMode ...string) string {
	return StatusWrapperScriptWithLogForwarder(command, jobDir, environment, workingDirectory, timeout, openMode, outputPaths, errorPaths, "rotari", envMode...)
}

func StatusWrapperScriptWithLogForwarder(command []string, jobDir string, environment []string, workingDirectory, timeout, openMode string, outputPaths, errorPaths []string, helper string, envMode ...string) string {
	commandLine := shellCommandLine(command)
	if len(outputPaths) > 0 || len(errorPaths) > 0 {
		if openMode == "" {
			openMode = model.OpenModeAppend
		}
		commandLine = logForwardCommandLine(helper, openMode, outputPaths, errorPaths, command)
	}
	return statusWrapperScript(commandLine, jobDir, environment, workingDirectory, timeout, envMode...)
}

func logForwardCommandLine(helper, openMode string, outputPaths, errorPaths, command []string) string {
	if openMode == "" {
		openMode = model.OpenModeAppend
	}
	parts := []string{ShellQuote(helper), "__log-forward", "--open-mode", ShellQuote(openMode)}
	for _, path := range outputPaths {
		parts = append(parts, "--output", ShellQuote(path))
	}
	for _, path := range errorPaths {
		parts = append(parts, "--error", ShellQuote(path))
	}
	parts = append(parts, "--", shellCommandLine(command))
	return strings.Join(parts, " ")
}

// processGroupLeaderShell makes a wrapper with a timeout the leader of its
// own process group, so the watchdog's "kill 0" reaches only the wrapper and
// the job. The local executor already starts wrappers that way, and
// schedulers normally do; otherwise the wrapper re-executes itself under
// setsid, which keeps its PID. It sets group_leader when that holds. The
// process group is read from /proc, because some ps implementations, such as
// BusyBox's, do not accept -p; ps is the fallback without /proc.
func processGroupLeaderShell(timeoutSeconds int) string {
	if timeoutSeconds <= 0 {
		return ""
	}
	return `wrapper_pgid() {
    pgid=
    [ -r /proc/$$/stat ] && pgid=$(sed 's/.*) //' /proc/$$/stat 2>/dev/null | awk '{print $3}')
    [ -n "$pgid" ] || pgid=$(ps -o pgid= -p $$ 2>/dev/null | tr -d ' ')
    echo "$pgid"
}
if [ "$(wrapper_pgid)" != "$$" ] && [ -z "${ROTARI_WRAPPER_SETSID:-}" ] && [ -f "$0" ] && command -v setsid >/dev/null 2>&1; then
    ROTARI_WRAPPER_SETSID=1 exec setsid /bin/sh "$0" "$@"
fi
unset ROTARI_WRAPPER_SETSID
group_leader=
[ "$(wrapper_pgid)" = "$$" ] && group_leader=1
`
}

func shellCommandLine(command []string) string {
	quoted := make([]string, 0, len(command))
	for _, arg := range command {
		quoted = append(quoted, ShellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

// statusWrapperBody records status around commandLine for the local and
// scheduler wrappers; the caller sets status_path. Signals mark the job
// cancelled unless the timeout watchdog stopped it, which records a failure
// with TimeoutExitCode.
func statusWrapperBody(commandLine string, timeoutSeconds int) string {
	message := TimeoutMessage(timeoutSeconds)
	watchdog := ""
	if timeoutSeconds > 0 {
		// Signalling the process group is only safe when the wrapper leads
		// it; otherwise the timeout is reported as not enforced.
		watchdog = "if [ -z \"$group_leader\" ]; then\n    echo \"rotari: timeout not enforced: the job wrapper could not get its own process group\" >&2\nelse\n" + watchdogShell(timeoutSeconds,
			": > \"$timed_out_marker\"\n    trap '' TERM\n    kill -TERM 0\n    trap 'kill \"$sleep_pid\" 2>/dev/null; exit 0' TERM",
			"if kill -0 $$ 2>/dev/null; then\n        write_status finished "+fmt.Sprint(TimeoutExitCode)+" \""+message+"\"\n        kill -KILL 0\n    fi") + "fi\n"
	}
	return `hostname=$(hostname 2>/dev/null || true)
finish_logs() { :; }
write_status() {
    phase=$1
    code=$2
    error=${3:-}
    tmp="${status_path}.tmp.$$"
    now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    if [ "$phase" = "running" ]; then
	printf '{"phase":"running","hosts":["%s"],"started_at":"%s"}\n' "$hostname" "$now" > "$tmp"
    elif [ -n "$error" ]; then
	printf '{"phase":"%s","hosts":["%s"],"exit_code":%s,"error":"%s","finished_at":"%s"}\n' "$phase" "$hostname" "$code" "$error" "$now" > "$tmp"
    else
	printf '{"phase":"%s","hosts":["%s"],"exit_code":%s,"finished_at":"%s"}\n' "$phase" "$hostname" "$code" "$now" > "$tmp"
    fi
    mv -f "$tmp" "$status_path"
}
write_status running 0
timed_out_marker="${status_path}.timed_out"
cancelled_marker="${status_path%/*}/cancelled"
on_signal() {
	signal=$1
    if [ -f "$timed_out_marker" ]; then
        echo "rotari: job ` + message + `" >&2
		return 0
    fi
    if [ -n "${job_pid:-}" ] && kill -0 "$job_pid" 2>/dev/null; then
        kill -TERM "$job_pid" 2>/dev/null || true
    fi
	finish_logs || true
	write_status cancelled "$signal"
    exit "$signal"
}
trap 'on_signal 143' TERM
trap 'on_signal 130' INT
trap 'on_signal 131' QUIT
watchdog_pid=
` + watchdog + `
if [ -f "$cancelled_marker" ]; then
    on_signal 143
fi
job_pid=
` + commandLine + ` &
job_pid=$!
if [ -f "$cancelled_marker" ]; then
    on_signal 143
fi
wait "$job_pid"
code=$?
if [ -f "$timed_out_marker" ]; then
	while kill -0 "$job_pid" 2>/dev/null; do
		wait "$job_pid"
		code=$?
	done
	code=` + fmt.Sprint(TimeoutExitCode) + `
	[ -z "$watchdog_pid" ] || kill "$watchdog_pid" 2>/dev/null
	write_status finished "$code" "` + message + `"
else
	[ -z "$watchdog_pid" ] || kill "$watchdog_pid" 2>/dev/null
	finish_logs || code=1
	write_status finished "$code"
fi
exit "$code"
`
}

// watchdogShell starts a background watchdog that sleeps seconds, runs
// onTimeout, waits timeoutGraceSeconds for the job to exit, then runs
// escalate. Terminating the watchdog, as the wrapper does when the command
// finishes first, stops it at either sleep. It sets watchdog_pid.
func watchdogShell(seconds int, onTimeout, escalate string) string {
	return fmt.Sprintf(`(
    trap 'kill "$sleep_pid" 2>/dev/null; exit 0' TERM
    sleep %d &
    sleep_pid=$!
    wait "$sleep_pid"
    %s
    sleep %d &
    sleep_pid=$!
    wait "$sleep_pid"
    %s
) </dev/null >/dev/null 2>&1 &
watchdog_pid=$!
`, seconds, onTimeout, timeoutGraceSeconds, escalate)
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
